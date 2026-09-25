package filo

import (
	"context"
	"errors"
	"reflect"
	"runtime"
	"testing"
	"time"
)

// stepAll steps s to the end and returns how many Steps it took.
func stepAll(t *testing.T, s *Stepper) int {
	t.Helper()
	n := 0
	for !s.Done() {
		_ = s.Step(context.Background())
		n++
		if n > 1_000_000 {
			t.Fatal("the run never ended")
		}
	}
	return n
}

func TestSteppingGivesWhatRunGives(t *testing.T) {
	u := loadUnit(t, NewEngine(), "prog.fbc")
	globals := map[string]Value{"base": VNum(5)}
	want, wantGlobals, err := u.Run(context.Background(), "main", globals, EvalConfig{})
	if err != nil {
		t.Fatal(err)
	}
	s, err := u.Start("main", globals, EvalConfig{})
	if err != nil {
		t.Fatal(err)
	}
	st := s.State()
	if len(st.Frames) != 1 || st.Frames[0].Fn != 0 || st.Line != 1 || st.Steps != 0 {
		t.Fatalf("before the first step: %+v", st)
	}
	stepAll(t, s)
	got, gotGlobals, err := s.Result()
	if err != nil || got.String() != want.String() || gotGlobals["total"].String() != wantGlobals["total"].String() {
		t.Fatalf("got %s %v (%v), want %s", got, gotGlobals, err, want)
	}
	if len(s.State().Frames) != 0 || s.Step(context.Background()) != nil {
		t.Fatal("a Step after the end did something")
	}
}

func TestSteppingIntoACall(t *testing.T) {
	u := loadUnit(t, NewEngine(), "prog.fbc")
	s, err := u.Start("fail", nil, EvalConfig{})
	if err != nil {
		t.Fatal(err)
	}
	for len(s.State().Frames) < 2 && !s.Done() {
		_ = s.Step(context.Background())
	}
	st := s.State()
	if len(st.Frames) != 2 || st.Frames[1].Fn != 4 || st.Line != 2 {
		t.Fatalf("inside half: %+v", st)
	}
	if st.Frames[1].Slots[0].Str != "four" || len(st.Frames[0].Operands) != 0 {
		t.Fatalf("frames: %+v", st.Frames)
	}
	stepAll(t, s)
	_, _, err = s.Result()
	_, _, runErr := u.Run(context.Background(), "fail", nil, EvalConfig{})
	pe, ok := errors.AsType[*PositionError](err)
	if !ok || err.Error() != runErr.Error() || pe.Line != 2 || pe.Col != 3 {
		t.Fatalf("got %v, Run gives %v", err, runErr)
	}
}

// Going back is starting again and stepping one fewer: the machine is
// deterministic, so the state is the same.
func TestGoingBackIsStartingAgain(t *testing.T) {
	u := loadUnit(t, NewEngine(), "prog.fbc")
	globals := map[string]Value{"base": VNum(1)}
	s, err := u.Start("main", globals, EvalConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var states []StepState
	for !s.Done() {
		states = append(states, s.State())
		_ = s.Step(context.Background())
	}
	for _, back := range []int{0, len(states) / 2, len(states) - 1} {
		again, err := u.Start("main", globals, EvalConfig{})
		if err != nil {
			t.Fatal(err)
		}
		for range back {
			_ = again.Step(context.Background())
		}
		got, want := again.State(), states[back]
		if got.Line != want.Line || !reflect.DeepEqual(frameShape(got), frameShape(want)) {
			t.Fatalf("step %d: %+v, want %+v", back, got, want)
		}
	}
}

// frameShape is a state's frames as text: values hold functions, which
// compare by identity.
func frameShape(st StepState) []string {
	var out []string
	for _, f := range st.Frames {
		out = append(out, VNum(float64(f.Fn)).String()+" "+VNum(float64(f.PC)).String()+" "+
			VList(f.Slots).String()+" "+VList(f.Operands).String())
	}
	return out
}

// buildUnit compiles src into a unit whose one entry is "main".
func buildUnit(t *testing.T, src string) *Unit {
	t.Helper()
	e := NewEngine()
	p, err := e.Compile(src)
	if err != nil {
		t.Fatal(err)
	}
	data, err := e.Build([]BuildEntry{{Name: "main", Program: p}})
	if err != nil {
		t.Fatal(err)
	}
	u, err := e.LoadUnit(data)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// A function map calls back runs a Step at a time too, as a call above the
// one that called map, which says so.
func TestSteppingIntoWhatABuiltinCalls(t *testing.T) {
	u := buildUnit(t, "(fold (fn (a x) (+ a x))\n  0\n  (map (fn (x) (* x 2)) (range 3)))")
	s, err := u.Start("main", nil, EvalConfig{})
	if err != nil {
		t.Fatal(err)
	}
	var vias []string
	seen := map[string]bool{}
	for !s.Done() {
		st := s.State()
		top := st.Frames[len(st.Frames)-1]
		if len(st.Frames) == 2 && !seen[top.Via+" "+VList(top.Slots).String()] {
			seen[top.Via+" "+VList(top.Slots).String()] = true
			vias = append(vias, top.Via+" "+VList(top.Slots).String())
			if st.Line != 3 && top.Via == "map" || st.Line != 1 && top.Via == "fold" {
				t.Fatalf("%s at line %d", top.Via, st.Line)
			}
		}
		_ = s.Step(context.Background())
	}
	want := []string{"map (list 0)", "map (list 1)", "map (list 2)",
		"fold (list 0 0)", "fold (list 0 2)", "fold (list 2 4)"}
	if !reflect.DeepEqual(vias, want) {
		t.Fatalf("calls back: %q, want %q", vias, want)
	}
	got, _, err := s.Result()
	if err != nil || got.String() != "6" {
		t.Fatalf("got %s (%v)", got, err)
	}
}

// Close ends a run in the middle, a callback's included, and a Stepper
// dropped without it is closed when collected: no run is left parked.
func TestClosingARunInTheMiddle(t *testing.T) {
	u := buildUnit(t, "(map (fn (x) (* x 2)) (range 100))")
	before := runtime.NumGoroutine()
	for range 50 {
		s, err := u.Start("main", nil, EvalConfig{})
		if err != nil {
			t.Fatal(err)
		}
		for range 20 {
			_ = s.Step(context.Background())
		}
		s.Close()
		if !s.Done() || s.Step(context.Background()) == nil || len(s.State().Frames) != 0 {
			t.Fatal("a closed run goes on")
		}
	}
	for range 50 {
		s, err := u.Start("main", nil, EvalConfig{})
		if err != nil {
			t.Fatal(err)
		}
		_ = s.Step(context.Background())
	}
	deadline := time.Now().Add(5 * time.Second)
	for runtime.NumGoroutine() > before+5 {
		if time.Now().After(deadline) {
			t.Fatalf("%d goroutines, %d before", runtime.NumGoroutine(), before)
		}
		runtime.GC()
		time.Sleep(10 * time.Millisecond)
	}
}
