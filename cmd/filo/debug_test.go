package main

import (
	"bytes"
	"os"
	"regexp"
	"strings"
	"testing"
)

func testSession(t *testing.T, entry string) *session {
	t.Helper()
	data, err := os.ReadFile("../../testdata/bytecode/prog.fbc")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newSession(data, "", entry, "../../testdata/bytecode", nil)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// where is the running call's function and the line and column of the
// next instruction.
func where(s *session) (fn, line, col int) {
	st := s.run.State()
	if len(st.Frames) == 0 {
		return -1, 0, 0
	}
	return st.Frames[len(st.Frames)-1].Fn, st.Line, st.Col
}

func TestSourcesByEntry(t *testing.T) {
	s := testSession(t, "fail")
	// fail.filo makes half (fn 4); main.filo makes square and the adder
	want := map[int]string{0: "main.filo", 1: "fail.filo", 2: "main.filo", 3: "main.filo", 4: "fail.filo"}
	for fn, file := range want {
		if s.fileOf[fn] != file {
			t.Errorf("fn %d is in %q, want %q", fn, s.fileOf[fn], file)
		}
	}
	if len(s.sources) != 2 {
		t.Fatalf("sources read: %d", len(s.sources))
	}
}

func TestStepNextAndBack(t *testing.T) {
	s := testSession(t, "fail")
	fn, line, _ := where(s)
	if fn != 1 || line != 1 {
		t.Fatalf("at the start: fn %d line %d", fn, line)
	}
	s.line(false) // the def of half is line 1; the call is on line 3
	fn, line, _ = where(s)
	if fn != 1 || line != 3 {
		t.Fatalf("after s: fn %d line %d", fn, line)
	}
	s.line(false) // into half
	fn, line, col := where(s)
	if fn != 4 || line != 2 || col != 6 {
		t.Fatalf("stepped into half: fn %d at %d:%d", fn, line, col)
	}
	s.back()
	fn, line, _ = where(s)
	if fn != 1 || line != 3 {
		t.Fatalf("back: fn %d line %d", fn, line)
	}
	s.line(true) // over the call, which fails
	if !s.run.Done() || !strings.Contains(s.msg, `in builtin "/"`) {
		t.Fatalf("next over a failing call: done %v, msg %q", s.run.Done(), s.msg)
	}
	s.instruction()
	if !strings.Contains(s.msg, "has ended") {
		t.Fatalf("a movement past the end: %q", s.msg)
	}
	s.back()
	if s.run.Done() {
		t.Fatal("back from the end did not go back")
	}
	s.startOver()
	fn, line, _ = where(s)
	if fn != 1 || line != 1 || s.steps != 0 || len(s.marks) != 0 {
		t.Fatalf("restart: fn %d line %d steps %d", fn, line, s.steps)
	}
}

func TestContinueToTheEnd(t *testing.T) {
	s := testSession(t, "main")
	s.finish()
	if !s.run.Done() || s.msg != "error: undefined global: base" {
		t.Fatalf("continue: %q", s.msg) // prog's main reads base, which no one gave
	}
}

// The screen as the terminal would get it: the source line and the
// instruction the run is at are marked, the calls listed, the keys shown.
func TestDraw(t *testing.T) {
	s := testSession(t, "fail")
	s.line(false)
	s.line(false) // inside half, at (/ n 2)
	scr := &screen{}
	scr.resize(100, 24)
	draw(s, scr)
	text := func(row int) string {
		var b strings.Builder
		for col := 0; col < scr.w; col++ {
			b.WriteRune(scr.at(row, col).r)
		}
		return b.String()
	}
	var all strings.Builder
	for row := 0; row < scr.h; row++ {
		all.WriteString(text(row) + "\n")
	}
	for _, want := range []string{"fail.filo", "(/ n 2)", " fn 4", "CALLB", `"four"`, "s step", "fail  step"} {
		if !strings.Contains(all.String(), want) {
			t.Errorf("the screen lacks %q:\n%s", want, all.String())
		}
	}
	marked := 0
	for row := 0; row < scr.h; row++ {
		for col := 0; col < scr.w; col++ {
			c := scr.at(row, col)
			if c.bg == colHereBg {
				marked++
			}
		}
	}
	if marked == 0 {
		t.Fatal("nothing marks where the run is")
	}
	var out bytes.Buffer
	err := scr.flush(&out)
	plain := regexp.MustCompile("\x1b\\[[0-9;?]*[A-Za-z]").ReplaceAllString(out.String(), "")
	if err != nil || !strings.Contains(plain, "(/ n 2)") {
		t.Fatalf("flush: %v", err)
	}
	out.Reset()
	_ = scr.flush(&out)
	if out.Len() != 0 {
		t.Fatalf("an unchanged screen sent %d bytes", out.Len())
	}
}

func TestKeys(t *testing.T) {
	s := testSession(t, "fail")
	if !s.key([]byte("i")) || s.steps != 1 {
		t.Fatal("i did not step")
	}
	if !s.key([]byte("\x1b[A")) || s.steps != 1 {
		t.Fatal("an arrow did something")
	}
	if s.key([]byte("q")) || s.key([]byte("\x1b")) {
		t.Fatal("q or Esc did not quit")
	}
}

// Deep in fib: the calls that do not fit say how many are left, and a
// column far to the right scrolls the source sideways into view.
func TestDrawDeepAndWide(t *testing.T) {
	data, err := os.ReadFile("../../testdata/bytecode/fib.fbc")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newSession(data, "", "", "../../testdata/bytecode", nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 11 {
		s.line(false)
	}
	scr := &screen{}
	scr.resize(60, 20)
	draw(s, scr)
	var all strings.Builder
	for row := 0; row < scr.h; row++ {
		for col := 0; col < scr.w; col++ {
			all.WriteRune(scr.at(row, col).r)
		}
		all.WriteByte('\n')
	}
	if !strings.Contains(all.String(), "more calls") {
		t.Errorf("no count of the calls left out:\n%s", all.String())
	}
	s.line(false) // to (+ (fib ...) ...) far on line 3, past the 23 columns of text
	for s.run.State().Col < 40 && !s.run.Done() {
		s.instruction()
	}
	draw(s, scr)
	hereCol := -1
	for row := 0; row < scr.h; row++ {
		for col := 0; col < scr.w/2; col++ {
			if scr.at(row, col).bg == colHereBg {
				hereCol = col
			}
		}
	}
	if hereCol < 0 {
		t.Fatalf("the column %d is not in view", s.run.State().Col)
	}
}

func TestBreakpoints(t *testing.T) {
	data, err := os.ReadFile("../../testdata/bytecode/fib.fbc")
	if err != nil {
		t.Fatal(err)
	}
	s, err := newSession(data, "", "", "../../testdata/bytecode", nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.cursor != 3 { // (def fib ...) is where the run starts
		t.Fatalf("the cursor starts at %d", s.cursor)
	}
	s.key([]byte("\x1b[A"))
	s.key([]byte("\x1b[B"))
	s.key([]byte(" ")) // a breakpoint on line 3, fib's body
	if !s.breaks["fib.filo"][3] {
		t.Fatalf("breaks: %v", s.breaks)
	}
	depths := []int{}
	for range 3 {
		s.key([]byte("c"))
		st := s.run.State()
		if s.run.Done() || st.Line != 3 || !strings.HasPrefix(s.msg, "breakpoint at fib.filo:3") {
			t.Fatalf("continue did not stop at the breakpoint: %q, line %d", s.msg, st.Line)
		}
		depths = append(depths, len(st.Frames))
	}
	if depths[0] != 2 || depths[1] != 3 || depths[2] != 4 {
		t.Fatalf("each call of fib stops once: depths %v", depths)
	}
	s.key([]byte("b"))
	if len(s.run.State().Frames) != 3 {
		t.Fatalf("back did not undo a continue: %d calls", len(s.run.State().Frames))
	}
	s.key([]byte(" ")) // off again: the cursor followed the run to line 3
	s.key([]byte("c"))
	if !s.run.Done() || s.msg != "returned 55" {
		t.Fatalf("without breakpoints, continue ends the run: %q", s.msg)
	}
}

func TestGlobalsGiven(t *testing.T) {
	data, err := os.ReadFile("../../testdata/bytecode/prog.fbc")
	if err != nil {
		t.Fatal(err)
	}
	globals, err := evalGlobals([]string{"base=5", "unused = (list 1 (+ 1 1))"})
	if err != nil || globals["unused"].String() != "(list 1 2)" {
		t.Fatalf("globals %v, %v", globals, err)
	}
	s, err := newSession(data, "", "main", "../../testdata/bytecode", globals)
	if err != nil {
		t.Fatal(err)
	}
	s.finish()
	if s.msg != "returned 19" {
		t.Fatalf("with base given: %q", s.msg)
	}
	s.back() // starting again keeps what was given
	s.finish()
	if s.msg != "returned 19" {
		t.Fatalf("after back: %q", s.msg)
	}
	for _, bad := range []string{"base", "=5", "base=", "base=(+ 1"} {
		_, err = evalGlobals([]string{bad})
		if err == nil {
			t.Errorf("-g %q was taken", bad)
		}
	}
	upper, err := os.ReadFile("../../testdata/bytecode/upper.fbc")
	if err != nil {
		t.Fatal(err)
	}
	u, err := newSession(upper, "", "", "../../testdata/bytecode", nil)
	if err != nil {
		t.Fatal(err)
	}
	u.finish()
	if u.msg != `returned "OLA"` { // str-upper is the strings pack's, which filo has
		t.Fatalf("upper: %q", u.msg)
	}
}

// debug of a source compiles it as build does: the unit is the C
// compiler's, and the session runs it.
func TestDebugFromSource(t *testing.T) {
	unit, err := compileFiles([]string{"../../testdata/bytecode/fib.filo"})
	if err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile("../../testdata/bytecode/fib.fbc")
	if err != nil || !bytes.Equal(unit, want) {
		t.Fatalf("the unit is not the C compiler's: %v", err)
	}
	s, err := newSession(unit, "", "", "../../testdata/bytecode", nil)
	if err != nil {
		t.Fatal(err)
	}
	s.finish()
	if s.msg != "returned 55" {
		t.Fatalf("%q", s.msg)
	}
}

// s goes into the function map calls back, a line at a time, and the stack
// says map made the call; n goes over it, as over any call.
func TestStepIntoACallback(t *testing.T) {
	dir := t.TempDir()
	src := "(def sq (fn (x)\n  (* x x)))\n(map sq\n  (list 1 2 3))\n"
	err := os.WriteFile(dir+"/sq.filo", []byte(src), 0o600)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := compileFiles([]string{dir + "/sq.filo"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := newSession(unit, "", "", dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		s.line(false)
		st := s.run.State()
		if len(st.Frames) == 2 {
			break
		}
	}
	st := s.run.State()
	if len(st.Frames) != 2 || st.Frames[1].Via != "map" || st.Line != 2 || s.file() != "sq.filo" {
		t.Fatalf("not inside sq: %+v", st)
	}
	scr := &screen{}
	scr.resize(100, 24)
	draw(s, scr)
	var all strings.Builder
	for row := 0; row < scr.h; row++ {
		for col := 0; col < scr.w; col++ {
			all.WriteRune(scr.at(row, col).r)
		}
		all.WriteByte('\n')
	}
	if !strings.Contains(all.String(), "via map") {
		t.Fatalf("the stack does not say map called:\n%s", all.String())
	}

	s.startOver()
	for !s.run.Done() {
		s.line(true)
		if len(s.run.State().Frames) > 1 {
			t.Fatal("n went into the callback")
		}
	}
	if s.msg != "returned (list 1 4 9)" {
		t.Fatalf("%q", s.msg)
	}
}
