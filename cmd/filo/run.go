package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/fbc"
)

// cmdRun runs a program and writes its value, as the C runtime's filo run:
// a source on the tree the compiler lowers (or, with --vm, as bytecode
// compiled in memory); a unit's entries, in order, sharing their globals;
// a bundle's member. --both runs a source both ways, a line each.
func cmdRun(args []string, stdout, stderr io.Writer) int {
	vm, both, trace := false, false, false
	i := 0
	for ; i < len(args) && len(args[i]) > 0 && args[i][0] == '-'; i++ {
		switch args[i] {
		case "--vm":
			vm = true
		case "--both":
			both = true
		case "--trace":
			trace = true
		default:
			_, _ = fmt.Fprintf(stderr, "filo: unknown flag: %s\n", args[i])
			_, _ = io.WriteString(stderr, help("run"))
			return 2
		}
	}
	if i >= len(args) {
		_, _ = io.WriteString(stderr, help("run"))
		return 2
	}
	path, rest := args[i], args[i+1:]
	data, err := os.ReadFile(path) // #nosec G304 G703 -- the program the command was given
	if err != nil {
		return complain(stderr, fmt.Errorf("cannot open: %s", path))
	}
	if len(data) == 0 {
		return complain(stderr, fmt.Errorf("%s: is empty", path))
	}
	switch fbc.Kind(data) {
	case fbc.KindUnit:
		return runUnit(data, rest, trace, stdout, stderr)
	case fbc.KindBundle:
		return runMember(data, rest, trace, stdout, stderr)
	}
	if len(rest) > 0 {
		_, _ = fmt.Fprintf(stderr, "filo: entries are for units, and this is source: %s\n", path)
		return 2
	}
	if both {
		return runBoth(path, data, stdout, stderr)
	}
	if vm || trace {
		unit, err := compileFiles([]string{path})
		if err != nil {
			return complain(stderr, err)
		}
		return runUnit(unit, nil, trace, stdout, stderr)
	}
	p, err := engine().Compile(string(data))
	if err != nil {
		return complain(stderr, placed(path, data, err))
	}
	v, _, err := p.Execute(context.Background(), nil, filo.EvalConfig{})
	if err != nil {
		return complain(stderr, placed(path, data, err))
	}
	return writeValue(stdout, stderr, v)
}

// runUnit runs a unit's entries, in order, sharing their globals, and
// writes the last one's value: main, else the unit's first, when none is
// named.
func runUnit(data []byte, entries []string, trace bool, stdout, stderr io.Writer) int {
	u, err := load(data)
	if err != nil {
		return complain(stderr, err)
	}
	if len(entries) == 0 {
		entries = []string{firstEntry(u.Entries())}
	}
	var v filo.Value
	var globals map[string]filo.Value
	for _, entry := range entries {
		if trace {
			v, globals, err = traceEntry(u, data, entry, globals, stdout)
		} else {
			v, globals, err = u.Run(context.Background(), entry, globals, filo.EvalConfig{})
		}
		if err != nil {
			return complain(stderr, placed(entry, nil, err)) // an entry is named by its source
		}
	}
	return writeValue(stdout, stderr, v)
}

// load is a unit this command's VM can run, refused as the C runtime's
// load refuses one, naming everything it lacks: an extern too, which Run
// would take and fail on only when read.
func load(data []byte) (*filo.Unit, error) {
	u, err := engine().LoadUnit(data)
	if err != nil {
		return nil, err
	}
	lack := u.Missing(nil)
	if len(lack) > 0 {
		return nil, fmt.Errorf("missing (%d): %s", len(lack), strings.Join(lack, " "))
	}
	return u, nil
}

// firstEntry is main when there is one, else the first.
func firstEntry(names []string) string {
	for _, n := range names {
		if n == "main" {
			return n
		}
	}
	if len(names) > 0 {
		return names[0]
	}
	return "main"
}

// runMember runs a member of a bundle: the one named first in args, else
// main, else the first; the rest of args are its entries.
func runMember(data []byte, args []string, trace bool, stdout, stderr io.Writer) int {
	b, err := fbc.ReadBundle(data)
	if err != nil {
		return complain(stderr, err)
	}
	var names []string
	var units [][]byte
	for _, m := range b.Members {
		names = append(names, m.Name)
		units = append(units, data[m.Unit.Off:m.Unit.Off+m.Unit.Len])
	}
	name := firstEntry(names)
	if len(args) > 0 {
		name, args = args[0], args[1:]
	}
	for i, n := range names {
		if n == name {
			return runUnit(units[i], args, trace, stdout, stderr)
		}
	}
	return complain(stderr, fmt.Errorf("bytecode: no bundle member named %s", name))
}

// runBoth runs a source on the tree and as bytecode, a line each: the same
// value, or the same error in the same place, in steps of their own.
func runBoth(path string, data []byte, stdout, stderr io.Writer) int {
	p, err := engine().Compile(string(data))
	if err != nil {
		return complain(stderr, placed(path, data, err))
	}
	steps := 0
	v, _, err := p.Execute(context.Background(), nil, filo.EvalConfig{Steps: &steps})
	sayEnd(stdout, "ir", v, err, steps, "steps, one a node")
	unit, err := compileFiles([]string{path})
	if err != nil {
		return complain(stderr, err)
	}
	u, err := load(unit)
	if err != nil {
		return complain(stderr, err)
	}
	steps = 0
	v, _, err = u.Run(context.Background(), entryName(path, ".filo"), nil, filo.EvalConfig{Steps: &steps})
	sayEnd(stdout, "vm", v, err, steps, "steps, one an instruction")
	return 0
}

// sayEnd is one run's end as a line: the value, or the error and where.
func sayEnd(w io.Writer, who string, v filo.Value, err error, steps int, unit string) {
	if err != nil {
		at := ""
		pe, ok := errors.AsType[*filo.PositionError](err)
		if ok {
			at = fmt.Sprintf(" at %d:%d", pe.Line, pe.Col)
		}
		_, _ = fmt.Fprintf(w, "%s  error%s: %v  (%d %s)\n", who, at, err, steps, unit)
		return
	}
	text := v.String()
	if len(text) >= 200 { // the C runtime's line holds 199 bytes of it
		text = text[:199] + "..."
	}
	_, _ = fmt.Fprintf(w, "%s  %s  (%d %s)\n", who, text, steps, unit)
}

func writeValue(stdout, stderr io.Writer, v filo.Value) int {
	err := v.Walkable()
	if err != nil {
		return complain(stderr, err)
	}
	_, _ = fmt.Fprintln(stdout, valueText(v))
	return 0
}

// traceEntry runs an entry as runUnit does, writing each instruction before
// it runs as the C runtime's --trace writes it: the listing's text,
// indented by how many calls deep the run is, and the top of the operand
// stack of the call running on the right; then how many steps it took.
func traceEntry(u *filo.Unit, data []byte, entry string, globals map[string]filo.Value, w io.Writer) (filo.Value, map[string]filo.Value, error) {
	listing, err := fbc.Read(data)
	if err != nil {
		return filo.Value{}, nil, err
	}
	s, err := u.Start(entry, globals, filo.EvalConfig{})
	if err != nil {
		return filo.Value{}, nil, err
	}
	defer s.Close()
	ctx := context.Background()
	for !s.Done() {
		st := s.State()
		top := st.Frames[len(st.Frames)-1]
		text := "(unreadable)"
		in := listing.Insn(top.PC)
		if in.Len > 0 {
			text = in.String()
		}
		stack := operandsText(top.Operands)
		sep := ""
		if stack != "" {
			sep = " "
		}
		_, _ = fmt.Fprintf(w, "%*s%04d  %s%*s |%s%s\n", 2*(len(st.Frames)-1), "", top.PC, text,
			max(34-utf8.RuneCountInString(text), 0), "", sep, stack)
		_ = s.Step(ctx)
	}
	v, g, err := s.Result()
	if err != nil {
		return v, g, err
	}
	_, _ = fmt.Fprintf(w, "-- %s: %d steps\n", entry, s.State().Steps)
	return v, g, nil
}

// operandsText is the top of an operand stack, four values at most, each
// cut to 16 bytes: a function has no source form, and shows as fn.
func operandsText(vs []filo.Value) string {
	from := max(len(vs)-4, 0)
	var b strings.Builder
	if from > 0 {
		b.WriteString("... ")
	}
	for i := from; i < len(vs); i++ {
		if i > from {
			b.WriteByte(' ')
		}
		text := "fn"
		if vs[i].Kind != filo.KFunc {
			text = vs[i].String()
		}
		if vs[i].Walkable() != nil {
			text = "?" // past the walk ceilings: the C runtime cannot write it either
		}
		if len(text) > 18 {
			text = text[:16] + ".."
		}
		b.WriteString(text)
	}
	return b.String()
}

// cmdShow writes one stage of what the compiler makes of a source, as the
// C runtime's filo show: the tree as read, the tree once constants folded,
// or the IR, each line with where it came from.
func cmdShow(args []string, stdout, stderr io.Writer) int {
	if len(args) != 2 {
		_, _ = io.WriteString(stderr, help("show"))
		return 2
	}
	stage, path := args[0], args[1]
	data, err := os.ReadFile(path) // #nosec G304 G703 -- the program the command was given
	if err != nil {
		return complain(stderr, fmt.Errorf("cannot open: %s", path))
	}
	if len(data) == 0 {
		return complain(stderr, fmt.Errorf("%s: is empty", path))
	}
	lines, err := engine().Show(string(data), stage)
	if err != nil {
		return complain(stderr, placed(path, data, err))
	}
	for _, l := range lines {
		_, _ = fmt.Fprintln(stdout, l)
	}
	return 0
}
