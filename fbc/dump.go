package fbc

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// lister writes the listing a line at a time, each cut to what the C one
// holds (255 bytes), and keeps the first error.
type lister struct {
	w   *bufio.Writer
	err error
}

func (l *lister) say(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	if len(line) > lineMost-1 {
		line = line[:lineMost-1]
	}
	if l.err == nil {
		_, l.err = l.w.WriteString(line + "\n")
	}
}

func (l *lister) flush() error {
	if l.err != nil {
		return l.err
	}
	return l.w.Flush()
}

func okText(ok bool) string {
	if ok {
		return "ok"
	}
	return "WRONG"
}

// Dump writes the unit's listing: its header, the names it imports and the
// globals it uses, its constants and entry points, then every function,
// each instruction with its bytes and, where it changes, the line and
// column it came from.
func (u *Unit) Dump(w io.Writer) error {
	l := &lister{w: bufio.NewWriter(w)}
	u.dump(l)
	return l.flush()
}

func (u *Unit) dump(l *lister) {
	debug := "none (stripped)"
	if u.Debug.Len > 0 {
		debug = "line and column of each instruction"
	}
	l.say("unit: %d bytes, format %d, checksum %08x %s", len(u.Data), u.Version, u.Checksum, okText(u.ChecksumOK))
	l.say("widest stack %d, widest frame %d", u.WidestStack, u.WidestFrame)
	l.say("debug: %s", debug)
	l.say("")
	l.say("imports (%d): functions the loading VM must provide", len(u.Imports))
	for i, name := range u.Imports {
		l.say("  %4d  %s", i, cut(name, nameMost))
	}
	note := ""
	if len(u.Externs) > 0 {
		note = ": the extern ones, read and never written, the VM provides"
	}
	l.say("globals (%d)%s", len(u.Globals), note)
	for i, name := range u.Globals {
		extern := ""
		if u.Externs[i] {
			extern = "  extern"
		}
		l.say("  %4d  %s%s", i, cut(name, nameMost), extern)
	}
	l.say("constants (%d)", len(u.Consts))
	for i := range u.Consts {
		l.say("  %4d  %s", i, u.ConstText(i, nameMost))
	}
	l.say("exports (%d)", len(u.Exports))
	for _, x := range u.Exports {
		l.say("  %s -> fn %d", cut(x.Name, nameMost), x.Fn)
	}
	for i := range u.Fns {
		u.dumpFn(l, i)
	}
}

// entry is the name of the last export that runs fn, "" for none.
func (u *Unit) entry(fn int) string {
	name := ""
	for _, x := range u.Exports {
		if x.Fn == fn {
			name = cut(x.Name, nameMost)
		}
	}
	return name
}

func (u *Unit) dumpFn(l *lister, i int) {
	f := u.Fns[i]
	label := ""
	name := u.entry(i)
	if name != "" {
		label = ` (entry "` + name + `")`
	}
	l.say("")
	l.say("fn %d%s: params %d, slots %d, stack %d, %d bytes", i, label, f.Params, f.Slots, f.Stack, f.Len)
	pc, end := f.Off, f.Off+f.Len
	lastLine, lastCol := 0, 0
	for pc < end {
		in := u.Insn(pc)
		if in.Len == 0 {
			l.say("  %04d  (an instruction cut short)", pc)
			return
		}
		var hex strings.Builder
		for k := 0; k < in.Len && k < 5; k++ {
			fmt.Fprintf(&hex, "%02x ", u.Data[u.Code.Off+pc+k])
		}
		// where it came from, shown when that changes: a map of the source
		where := ""
		line, col, ok := u.Position(pc)
		if ok && (line != lastLine || col != lastCol) {
			where = fmt.Sprintf("%d:%d", line, col)
			lastLine, lastCol = line, col
		}
		l.say("  %04d  %s %s %s", pc, pad(hex.String(), 15), pad(where, 6), in)
		pc += in.Len
	}
	if pc > end {
		l.say("  (the last instruction runs past the function)")
	}
}

// Dump writes the bundle's listing: its table, then every member's.
func (b *Bundle) Dump(w io.Writer) error {
	l := &lister{w: bufio.NewWriter(w)}
	l.say("bundle: %d bytes, format %d, checksum %08x %s", len(b.Data), b.Version, b.Checksum, okText(b.ChecksumOK))
	l.say("widest stack %d, widest frame %d", b.WidestStack, b.WidestFrame)
	l.say("members (%d)", len(b.Members))
	for _, m := range b.Members {
		l.say("  %s %6d bytes at %d", pad(cut(m.Name, len(m.Name)+1), 16), m.Unit.Len, m.Unit.Off)
	}
	for _, m := range b.Members {
		l.say("")
		l.say("== %s", cut(m.Name, len(m.Name)+1))
		u, err := Read(b.Data[m.Unit.Off : m.Unit.Off+m.Unit.Len])
		if err != nil {
			l.say("(not a unit: %s)", err)
			continue
		}
		u.dump(l)
	}
	return l.flush()
}
