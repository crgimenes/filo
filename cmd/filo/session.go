package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/fbc"
	"github.com/crgimenes/filo/filomath"
	"github.com/crgimenes/filo/filostrings"
)

// session is a debugging session: a unit's entry point stepped by a
// person, who may go back. Going back is starting again and stepping to
// where the last movement began: the machine is deterministic.
type session struct {
	unit    *filo.Unit
	listing *fbc.Unit
	entry   string
	run     *filo.Stepper
	steps   int   // Steps taken since the start
	marks   []int // where each movement began, the last one last
	sources map[string][]string
	fileOf  map[int]string // each function's source file, by the entry it belongs to
	msg     string         // how the run ended, or why a key did nothing
	cursor  int            // the source line the arrows move, from 1; it follows the run
	breaks  map[string]map[int]bool
	globals map[string]filo.Value // what the run is given, every time it starts again
}

// newSession loads a unit (a .fbc, or a member of a .fbb) for its entry:
// the one named, or "main", or the unit's first. Sources are read from
// srcDir as the entries name them: the entry "main" is main.filo.
func newSession(data []byte, member, entry, srcDir string, globals map[string]filo.Value) (*session, error) {
	e := engine()
	unitBytes := data
	var u *filo.Unit
	var err error
	switch fbc.Kind(data) {
	case fbc.KindBundle:
		unitBytes, err = bundleMember(data, member)
		if err != nil {
			return nil, err
		}
		u, err = e.LoadUnit(unitBytes)
	case fbc.KindUnit:
		u, err = e.LoadUnit(data)
	default:
		return nil, errors.New("not a unit or a bundle (filo build makes one from source)")
	}
	if err != nil {
		return nil, err
	}
	listing, err := fbc.Read(unitBytes)
	if err != nil {
		return nil, err
	}
	s := &session{
		unit: u, listing: listing, entry: pickEntry(u.Entries(), entry),
		sources: map[string][]string{}, breaks: map[string]map[int]bool{},
		globals: globals,
	}
	s.fileOf = sourceFiles(listing)
	for _, file := range s.fileOf {
		text, err := os.ReadFile(filepath.Join(srcDir, file)) // #nosec G304 -- a source beside the unit the command was given
		if err == nil {
			text = bytes.TrimSuffix(bytes.TrimPrefix(text, []byte("\ufeff")), []byte("\n"))
			s.sources[file] = strings.Split(string(text), "\n")
		}
	}
	return s, s.restart()
}

// engine has what the C runtime's filo command has: the core and the
// math and strings packs.
func engine() *filo.Engine {
	e := filo.NewEngine()
	filomath.RegisterBuiltins(e)
	filostrings.RegisterBuiltins(e)
	return e
}

// evalGlobals evaluates each "name=expression" into the global of that
// name, as a corpus case's given: the expression is Filo, so a global may
// be a value, a list, or a function a unit imports and the VM lacks.
func evalGlobals(defs []string) (map[string]filo.Value, error) {
	globals := map[string]filo.Value{}
	for _, d := range defs {
		name, expr, ok := strings.Cut(d, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.TrimSpace(expr) == "" {
			return nil, fmt.Errorf("-g %q: want NAME=EXPRESSION", d)
		}
		v, _, err := engine().RunScript(context.Background(), expr, nil, filo.EvalConfig{})
		if err != nil {
			return nil, fmt.Errorf("-g %s: %w", name, err)
		}
		globals[name] = v
	}
	return globals, nil
}

// bundleMember is the unit of the bundle named member, or when member is ""
// the one named "main", or the first.
func bundleMember(data []byte, member string) ([]byte, error) {
	b, err := fbc.ReadBundle(data)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(b.Members))
	for i, m := range b.Members {
		names[i] = m.Name
	}
	name := pickEntry(names, member)
	for _, m := range b.Members {
		if m.Name == name {
			return data[m.Unit.Off : m.Unit.Off+m.Unit.Len], nil
		}
	}
	return nil, fmt.Errorf("no bundle member named %s", member)
}

// pickEntry is want, or "main" when there is one, or the first.
func pickEntry(names []string, want string) string {
	if want != "" || len(names) == 0 {
		return want
	}
	for _, n := range names {
		if n == "main" {
			return n
		}
	}
	return names[0]
}

// sourceFiles is the file each function came from. The debug section says
// the line and the column, not the file; but every entry is compiled from
// the file it is named after, and every other function is made by a
// CLOSURE inside one, so walking them from each entry says whose it is.
func sourceFiles(u *fbc.Unit) map[int]string {
	owner := map[int]string{}
	var walk func(fn int, file string)
	walk = func(fn int, file string) {
		_, seen := owner[fn]
		if seen || fn < 0 || fn >= len(u.Fns) {
			return
		}
		owner[fn] = file
		f := u.Fns[fn]
		for pc := f.Off; pc < f.Off+f.Len; {
			in := u.Insn(pc)
			if in.Len == 0 {
				return
			}
			if in.Op == fbc.OpClosure {
				inner, err := strconv.Atoi(in.Operands)
				if err == nil {
					walk(inner, file)
				}
			}
			pc += in.Len
		}
	}
	for _, x := range u.Exports {
		walk(x.Fn, x.Name+".filo")
	}
	return owner
}

func (s *session) restart() error {
	run, err := s.unit.Start(s.entry, s.globals, filo.EvalConfig{})
	if err != nil {
		return err
	}
	s.run, s.steps, s.msg = run, 0, ""
	s.follow()
	return nil
}

// follow brings the cursor to the line the run is at.
func (s *session) follow() {
	st := s.run.State()
	if st.Line > 0 {
		s.cursor = st.Line
	}
}

// file is the source file of the function running, "" once the run ended.
func (s *session) file() string {
	st := s.run.State()
	if len(st.Frames) == 0 {
		return ""
	}
	return s.fileOf[st.Frames[len(st.Frames)-1].Fn]
}

// moveCursor moves the cursor by n lines, within the file.
func (s *session) moveCursor(n int) {
	lines := len(s.sources[s.file()])
	s.cursor = min(max(s.cursor+n, 1), max(lines, 1))
}

// toggleBreak sets a breakpoint on the cursor's line, or takes it away.
func (s *session) toggleBreak() {
	file := s.file()
	if file == "" {
		return
	}
	if s.breaks[file] == nil {
		s.breaks[file] = map[int]bool{}
	}
	if s.breaks[file][s.cursor] {
		delete(s.breaks[file], s.cursor)
		return
	}
	s.breaks[file][s.cursor] = true
}

// hasBreaks says whether any line is marked.
func (s *session) hasBreaks() bool {
	for _, lines := range s.breaks {
		if len(lines) > 0 {
			return true
		}
	}
	return false
}

// step runs one instruction; false when the run had ended.
func (s *session) step() bool {
	if s.run.Done() {
		return false
	}
	_ = s.run.Step(context.Background())
	s.steps++
	if s.run.Done() {
		s.msg = s.ending()
	}
	return true
}

func (s *session) ending() string {
	v, _, err := s.run.Result()
	if err != nil {
		return "error: " + err.Error()
	}
	return "returned " + v.String()
}

// move runs a movement, remembering where it began so back can undo it.
func (s *session) move(how func()) {
	if s.run.Done() {
		s.msg = "the run has ended: r starts again, b goes back"
		return
	}
	s.marks = append(s.marks, s.steps)
	how()
	s.follow()
}

// instruction runs one instruction.
func (s *session) instruction() {
	s.move(func() { s.step() })
}

// line runs until the source line changes: into calls, or over them when
// over, as gdb's step and next.
func (s *session) line(over bool) {
	s.move(func() {
		start := s.run.State()
		depth := len(start.Frames)
		if start.Line == 0 { // a stripped unit has no lines: an instruction is the step
			s.step()
			return
		}
		for s.step() {
			st := s.run.State()
			if len(st.Frames) == 0 {
				return
			}
			if over && len(st.Frames) > depth {
				continue
			}
			if st.Line != 0 && (st.Line != start.Line || len(st.Frames) != depth) {
				return
			}
		}
	})
}

// finish runs to the end, or until the run enters a line with a
// breakpoint: comes to it from another line, or a call starts on it.
func (s *session) finish() {
	s.move(func() {
		if !s.hasBreaks() {
			for s.step() {
			}
			return
		}
		st := s.run.State()
		line, depth := st.Line, len(st.Frames)
		for s.step() {
			st = s.run.State()
			if len(st.Frames) == 0 {
				return
			}
			entered := st.Line != line || len(st.Frames) != depth
			line, depth = st.Line, len(st.Frames)
			top := st.Frames[len(st.Frames)-1]
			if entered && s.breaks[s.fileOf[top.Fn]][st.Line] {
				s.msg = fmt.Sprintf("breakpoint at %s:%d", s.fileOf[top.Fn], st.Line)
				return
			}
		}
	})
}

// back undoes the last movement.
func (s *session) back() {
	if len(s.marks) == 0 {
		s.msg = "at the start"
		return
	}
	target := s.marks[len(s.marks)-1]
	s.marks = s.marks[:len(s.marks)-1]
	err := s.restart()
	if err != nil {
		s.msg = err.Error()
		return
	}
	for s.steps < target && s.step() {
	}
	s.follow()
}

// entryName is the entry point fn is, "" for another function.
func (s *session) entryName(fn int) string {
	for _, x := range s.listing.Exports {
		if x.Fn == fn {
			return x.Name
		}
	}
	return ""
}

// place is where frame f of the state is in its source: the next
// instruction for the one running, the call it waits on for the others.
func (s *session) place(frames []filo.StepFrame, i int) string {
	f := frames[i]
	pc := f.PC
	if i < len(frames)-1 && pc > 0 {
		pc-- // after the call: the call's own place
	}
	line, col, ok := s.listing.Position(pc)
	if !ok {
		return s.fileOf[f.Fn]
	}
	return fmt.Sprintf("%s %d:%d", s.fileOf[f.Fn], line, col)
}

// startOver goes back to the first instruction, forgetting every movement.
func (s *session) startOver() {
	s.marks = nil
	err := s.restart()
	if err != nil {
		s.msg = err.Error()
	}
}
