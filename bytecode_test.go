package filo

import (
	"context"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"
)

// The units in testdata/bytecode are built by the C runtime's filo command
// from the .filo beside them (the compiler is C only):
//
//	filo build -o prog.fbc main.filo fail.filo
//	filo build -o upper.fbc upper.filo
//	filo bundle -o demo.fbb prog.fbc upper.fbc

func loadUnit(t *testing.T, e *Engine, name string) *Unit {
	t.Helper()
	data, err := os.ReadFile("testdata/bytecode/" + name)
	if err != nil {
		t.Fatal(err)
	}
	u, err := e.LoadUnit(data)
	if err != nil {
		t.Fatalf("LoadUnit(%s): %v", name, err)
	}
	return u
}

func TestUnitRuns(t *testing.T) {
	e := NewEngine()
	u := loadUnit(t, e, "prog.fbc")
	got := u.Entries()
	if !slices.Equal(got, []string{"main", "fail"}) {
		t.Fatalf("Entries() = %v", got)
	}
	got = u.Missing(nil)
	if !slices.Equal(got, []string{"base"}) {
		t.Fatalf("Missing(nil) = %v, want [base]", got)
	}
	v, globals, err := u.Run(context.Background(), "main", map[string]Value{"base": VNum(5)}, EvalConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "19" || globals["total"].String() != "14" {
		t.Fatalf("got %s and total %s, want 19 and 14", v, globals["total"])
	}

	// a function of the unit, called by a program and by a builtin there
	p, err := e.Compile("(list (square 7) (map square (list 2 3)))")
	if err != nil {
		t.Fatal(err)
	}
	v, _, err = p.Execute(context.Background(), map[string]Value{"square": globals["square"]}, EvalConfig{})
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "(list 49 (list 4 9))" {
		t.Fatalf("got %s", v)
	}
}

func TestUnitErrorsSayWhere(t *testing.T) {
	u := loadUnit(t, NewEngine(), "prog.fbc")
	_, _, err := u.Run(context.Background(), "fail", nil, EvalConfig{})
	pe, ok := errors.AsType[*PositionError](err)
	if !ok || pe.Line != 2 || pe.Col != 3 {
		t.Fatalf("got %v, want an error at 2:3", err)
	}
	if !strings.Contains(err.Error(), `in builtin "/"`) {
		t.Fatalf("got %q", err)
	}
	_, _, err = u.Run(context.Background(), "main", nil, EvalConfig{})
	if err == nil || err.Error() != "undefined global: base" {
		t.Fatalf("got %v, want undefined global: base", err)
	}
	_, _, err = u.Run(context.Background(), "nope", nil, EvalConfig{})
	if err == nil || err.Error() != "bytecode: no entry point named nope" {
		t.Fatalf("got %v", err)
	}
}

func TestUnitRefusedForWhatTheEngineLacks(t *testing.T) {
	e := NewEngine()
	u := loadUnit(t, e, "upper.fbc")
	got := u.Missing(nil)
	if !slices.Equal(got, []string{"str-upper"}) {
		t.Fatalf("Missing(nil) = %v", got)
	}
	_, _, err := u.Run(context.Background(), "upper", nil, EvalConfig{})
	if err == nil || err.Error() != "missing (1): str-upper" {
		t.Fatalf("got %v", err)
	}
	// a host that has it, as a builtin or as a function in Filo
	e.MustRegisterBuiltin("str-upper", func(_ context.Context, args []Value) (Value, error) {
		return VString(strings.ToUpper(args[0].Str)), nil
	})
	u = loadUnit(t, e, "upper.fbc")
	v, _, err := u.Run(context.Background(), "upper", nil, EvalConfig{})
	if err != nil || v.Str != "OLA" {
		t.Fatalf("got %v, %v", v, err)
	}
}

func TestBundleMembers(t *testing.T) {
	data, err := os.ReadFile("testdata/bytecode/demo.fbb")
	if err != nil {
		t.Fatal(err)
	}
	e := NewEngine()
	u, err := e.LoadBundle(data, "prog")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(u.Entries(), []string{"main", "fail"}) {
		t.Fatalf("Entries() = %v", u.Entries())
	}
	_, err = e.LoadBundle(data, "nope")
	if err == nil || err.Error() != "bytecode: no bundle member named nope" {
		t.Fatalf("got %v", err)
	}
	_, err = e.LoadUnit(data)
	if err == nil || err.Error() != "bytecode: a kind or version this runtime does not read" {
		t.Fatalf("a bundle as a unit: got %v", err)
	}
}

func TestUnitDamageRefused(t *testing.T) {
	data, err := os.ReadFile("testdata/bytecode/prog.fbc")
	if err != nil {
		t.Fatal(err)
	}
	for _, at := range []int{30, len(data) / 2, len(data) - 1} {
		bad := slices.Clone(data)
		bad[at] ^= 0x40
		_, err = NewEngine().LoadUnit(bad)
		if err == nil || !strings.Contains(err.Error(), "checksum") {
			t.Errorf("byte %d flipped: got %v", at, err)
		}
	}
	_, err = NewEngine().LoadUnit(data[:len(data)-1])
	if err == nil {
		t.Error("a cut unit loaded")
	}
}

// FuzzLoadUnitDoesNotPanic feeds units, the checksum put right so a
// mutation reaches the sections and the machine: a damaged one is refused or
// fails, and never takes the host down.
func FuzzLoadUnitDoesNotPanic(f *testing.F) {
	for _, name := range []string{"prog.fbc", "upper.fbc", "demo.fbb"} {
		data, err := os.ReadFile("testdata/bytecode/" + name)
		if err != nil {
			f.Fatal(err)
		}
		f.Add(data)
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) >= bcHeader {
			sum := bcChecksum(data)
			data[8], data[9], data[10], data[11] = byte(sum), byte(sum>>8), byte(sum>>16), byte(sum>>24)
		}
		e := NewEngine()
		u, err := e.LoadUnit(data)
		if err != nil {
			u, err = e.LoadBundle(data, "prog")
		}
		if err != nil {
			return
		}
		cfg := EvalConfig{StepLimit: 10_000, RecursionLimit: 64}
		for _, entry := range u.Entries() {
			_, _, err = u.Run(context.Background(), entry, map[string]Value{"base": VNum(1)}, cfg)
			if err != nil && strings.HasPrefix(err.Error(), "panic in script") {
				t.Fatal(err)
			}
		}
	})
}

// A unit is read only once loaded: one may run on many goroutines at once.
func TestUnitRunsConcurrently(t *testing.T) {
	u := loadUnit(t, NewEngine(), "prog.fbc")
	errs := make(chan error, 8)
	for range 8 {
		go func() {
			v, _, err := u.Run(context.Background(), "main", map[string]Value{"base": VNum(1)}, EvalConfig{})
			if err == nil && v.String() != "15" {
				err = errors.New("got " + v.String())
			}
			errs <- err
		}()
	}
	for range 8 {
		err := <-errs
		if err != nil {
			t.Fatal(err)
		}
	}
}
