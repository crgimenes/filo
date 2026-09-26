package main

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/fbc"
)

// The edt's colours: text on the terminal's own ground, bars dark grey with
// keys in bold orange and words in light grey, the position in green, and
// the place the run is at in orange under black.
const (
	colBar    = 236
	colWords  = 252
	colKeys   = 214
	colDim    = 244
	colRule   = 240
	colWhere  = 2
	colHereFg = 0
	colHereBg = 214
	colBreak  = 196
	tabWidth  = 8
)

// the footer's keys, the ones a narrow screen drops last first: quit
// among them, as the edt keeps its ^Q
var hints = []struct{ key, rest string }{
	{"s", " step"}, {"n", " next"}, {"c", " continue"}, {"q", " quit"}, {"h", " help"},
	{"x", " bytes"}, {"space", " break"}, {"b", " back"}, {"i", " instruction"}, {"r", " restart"},
}

// draw paints the session: the source on the left, the unit's instructions
// on the right (all of them, around the one the run is at), the calls with
// their locals and operands below, and the footer.
func draw(s *session, scr *screen) {
	scr.clear()
	if scr.w < 40 || scr.h < 10 {
		scr.print(0, 0, "filo debug: the terminal is too small")
		return
	}
	if s.help {
		drawHelp(s, scr)
		return
	}
	st := s.shown()
	stackRows := min(max(scr.h/4, 3), 10)
	top := scr.h - 1 - stackRows
	left := scr.w / 2
	fn, pc := -1, -1
	if len(st.Frames) > 0 {
		f := st.Frames[len(st.Frames)-1]
		fn, pc = f.Fn, f.PC
	}
	drawSource(s, scr, fn, st.Line, st.Col, top, left)
	drawMarks(s, scr, fn, st.Line, top)
	scr.color(colRule, colorDefault)
	scr.fill(0, left, top, 1, '│')
	if s.bytes {
		drawBytes(s, scr, fn, pc, st.Line, top, left+1)
	} else {
		drawCode(s, scr, fn, pc, st.Line, top, left+1)
	}
	drawStack(s, scr, st, top, stackRows)
	drawFooter(s, scr, st)
}

func drawSource(s *session, scr *screen, fn, line, col, rows, width int) {
	file := s.fileOf[fn]
	scr.color(colDim, colorDefault)
	scr.print(0, 1, file)
	text, ok := s.sources[file]
	if !ok {
		if file != "" {
			for i, l := range wrap("(no source: "+file+" is not beside the unit; -src DIR says where)", width-2) {
				scr.print(2+i, 1, l)
			}
		}
		return
	}
	first := max(1, s.cursor-(rows-1)/2)
	// the text scrolls sideways, as the edt's, to keep the column in view
	shift := 0
	if line >= 1 && line <= len(text) && col > 0 {
		at := len([]rune(expandTabs(prefixBytes(text[line-1], col-1))))
		room := width - 7
		if at >= room-2 {
			shift = at - room/2
		}
	}
	inString := false // a string open at the start of the line, as the edt carries it
	for n := 1; n < first && n <= len(text); n++ {
		_, inString = hlClasses(text[n-1], inString)
	}
	for row := 1; row < rows; row++ {
		n := first + row - 1
		if n > len(text) {
			break
		}
		here := n == line
		bg := int16(colorDefault)
		if here {
			bg = colBar
			scr.color(colorDefault, colBar)
			scr.fill(row, 0, 1, width, ' ')
			scr.color(colKeys, colBar)
			scr.style(true, false)
		} else {
			scr.color(colDim, colorDefault)
		}
		scr.print(row, 0, fmt.Sprintf("%5d ", n))
		scr.style(false, false)
		expanded := expandTabs(text[n-1])
		var cls []byte
		cls, inString = hlClasses(expanded, inString)
		drawColoured(scr, row, 6, expanded, cls, shift, width-7, bg)
		shown := string(dropRunes(expanded, shift))
		if here && col > 0 {
			at := 6 + len([]rune(expandTabs(prefixBytes(text[n-1], col-1)))) - shift
			if at < width {
				scr.color(colHereFg, colHereBg)
				r := ' '
				rs := []rune(shown)
				if at-6 < len(rs) {
					r = rs[at-6]
				}
				scr.print(row, at, string(r))
			}
		}
	}
}

// drawColoured prints text from its rune skip on, at most cols runes, each
// run of one class in its colour over bg.
func drawColoured(scr *screen, row, col int, text string, cls []byte, skip, cols int, bg int16) {
	var run strings.Builder
	cur := byte(hlPlain)
	at, shown, k := col, 0, 0
	flush := func() {
		scr.color(hlColor[cur], bg)
		at += scr.print(row, at, run.String())
		run.Reset()
	}
	for i, r := range text {
		if k < skip {
			k++
			continue
		}
		if shown == cols {
			break
		}
		if cls[i] != cur && run.Len() > 0 {
			flush()
		}
		cur = cls[i]
		run.WriteRune(r)
		shown++
		k++
	}
	flush()
}

// drawMarks puts the breakpoints (red dots) and the cursor (an orange
// arrow, where it is not on the run's line) in the source's margin.
func drawMarks(s *session, scr *screen, fn, line, rows int) {
	file := s.fileOf[fn]
	text, ok := s.sources[file]
	if !ok {
		return
	}
	first := max(1, s.cursor-(rows-1)/2)
	for row := 1; row < rows; row++ {
		n := first + row - 1
		if n > len(text) {
			break
		}
		bg := int16(colorDefault)
		if n == line {
			bg = colBar
		}
		if s.breaks[file][n] {
			scr.color(colBreak, bg)
			scr.print(row, 0, "●")
		}
		if n == s.cursor && n != line {
			scr.color(colKeys, bg)
			scr.style(true, false)
			scr.print(row, 1, "▸")
			scr.style(false, false)
		}
	}
}

func drawCode(s *session, scr *screen, fn, pc, line, rows, col int) {
	width := scr.w - col
	if fn < 0 {
		return
	}
	scr.color(colDim, colorDefault)
	label := fmt.Sprintf(" at fn %d", fn)
	name := s.entryName(fn)
	if name != "" {
		label += ` (entry "` + name + `")`
	}
	scr.print(0, col, label)
	code, here := codeRows(s, pc)
	// the cursor moved off the run's line: its instructions, in view and
	// marked, so the arrows read the code the source makes
	file, cursorOff := s.fileOf[fn], s.cursor != line
	if cursorOff {
		for i, r := range code {
			if r.of >= 0 && r.line == s.cursor && s.fileOf[r.of] == file {
				here = i
				break
			}
		}
	}
	first := max(0, here-(rows-2)/2)
	for row := 1; row < rows && first+row-1 < len(code); row++ {
		r := code[first+row-1]
		switch {
		case r.of < 0:
			scr.color(colDim, colorDefault)
		case r.in.PC == pc:
			scr.color(colHereFg, colHereBg)
			scr.fill(row, col, 1, width, ' ')
		case cursorOff && r.line == s.cursor && s.fileOf[r.of] == file:
			scr.color(colKeys, colorDefault)
		default:
			scr.color(colorDefault, colorDefault)
		}
		scr.print(row, col, cutTo(r.text, width))
	}
}

// codeRow is a row of the listing: a function's heading (of is -1) or an
// instruction of function of, from line of its source.
type codeRow struct {
	of   int
	line int
	in   fbc.Insn
	text string
}

// codeRows is the whole unit's code, as filo dump lists it, every function
// under its heading, and the row of the instruction at pc: the functions
// the run has not reached yet are there to read too.
func codeRows(s *session, pc int) ([]codeRow, int) {
	var rows []codeRow
	here := 0
	for i, f := range s.listing.Fns {
		head := fmt.Sprintf(" fn %d: params %d, slots %d", i, f.Params, f.Slots)
		name := s.entryName(i)
		if name != "" {
			head = fmt.Sprintf(` fn %d (entry "%s"): params %d, slots %d`, i, name, f.Params, f.Slots)
		}
		rows = append(rows, codeRow{of: -1, text: head})
		for at := f.Off; at < f.Off+f.Len; {
			in := s.listing.Insn(at)
			if in.Len == 0 {
				break
			}
			if at == pc {
				here = len(rows)
			}
			place := ""
			line, c, ok := s.listing.Position(in.PC)
			if ok {
				place = fmt.Sprintf("%d:%d", line, c)
			}
			rows = append(rows, codeRow{of: i, line: line, in: in, text: fmt.Sprintf(" %04d %-6s %s", in.PC, place, in)})
			at += in.Len
		}
	}
	return rows, here
}

func drawStack(s *session, scr *screen, st filo.StepState, top, rows int) {
	scr.color(colRule, colorDefault)
	scr.fill(top, 0, 1, scr.w, '─')
	scr.color(colDim, colorDefault)
	scr.print(top, 1, " calls, innermost first ")
	row := top + 1
	for i := len(st.Frames) - 1; i >= 0 && row < top+rows; i-- {
		if row == top+rows-1 && i > 0 {
			scr.color(colDim, colorDefault)
			scr.print(row, 1, fmt.Sprintf("… %d more calls", i+1))
			break
		}
		f := st.Frames[i]
		label := fmt.Sprintf("#%d fn %d", len(st.Frames)-1-i, f.Fn)
		name := s.entryName(f.Fn)
		if name != "" {
			label += ` "` + name + `"`
		}
		if f.Via != "" {
			label += " via " + f.Via
		}
		scr.color(colDim, colorDefault)
		n := scr.print(row, 1, label+"  "+s.place(st.Frames, i)+"  ")
		scr.color(colorDefault, colorDefault)
		if i < len(st.Frames)-1 {
			scr.print(row, 1+n, cutTo("slots "+values(f.Slots)+"  operands "+values(f.Operands), scr.w-2-n))
			row++
			continue
		}
		drawOperands(s, scr, row, 1+n, f)
		row++
	}
	if len(st.Frames) == 0 && s.msg != "" {
		scr.color(colorDefault, colorDefault)
		scr.print(row, 1, cutTo(s.msg, scr.w-2))
	}
}

// drawOperands writes the running call's slots and operands, the operands
// its next instruction takes in orange, and which instruction that is: the
// stack is where the arguments of a call come from.
func drawOperands(s *session, scr *screen, row, col int, f filo.StepFrame) {
	in := s.listing.Insn(f.PC)
	takes := min(consumes(in), len(f.Operands))
	end := scr.w - 1
	put := func(text string, fg int16, bold bool) {
		if col >= end {
			return
		}
		scr.color(fg, colorDefault)
		scr.style(bold, false)
		col += scr.print(row, col, cutTo(text, end-col))
		scr.style(false, false)
	}
	put("slots "+values(f.Slots)+"  operands (", colorDefault, false)
	for k, v := range f.Operands {
		if k > 0 {
			put(" ", colorDefault, false)
		}
		if k >= len(f.Operands)-takes {
			put(v.String(), colKeys, true)
		} else {
			put(v.String(), colorDefault, false)
		}
	}
	put(")", colorDefault, false)
	if takes > 0 {
		put(fmt.Sprintf("  %s takes %d", in.Name, takes), colDim, false)
	}
}

// consumes is how many operands in takes from the top of the stack, as
// docs/bytecode.md's table has it: a store reads the top and keeps it, a
// conditional jump its test, a call its function and arguments.
func consumes(in fbc.Insn) int {
	count := 0
	fields := strings.Fields(in.Operands)
	if len(fields) > 0 {
		count, _ = strconv.Atoi(fields[0])
	}
	switch in.Op {
	case fbc.OpStoreG, fbc.OpStoreL, fbc.OpStoreUp, fbc.OpUnpack, fbc.OpRet:
		return 1
	case fbc.OpPop, fbc.OpTuple, fbc.OpCallB:
		return count
	case fbc.OpCall:
		return count + 1
	case fbc.OpJmp:
		if in.Operands == "always" {
			return 0
		}
		return 1
	}
	return 0
}

func values(vs []filo.Value) string {
	parts := make([]string, len(vs))
	for i, v := range vs {
		parts[i] = v.String()
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func drawFooter(s *session, scr *screen, st filo.StepState) {
	y := scr.h - 1
	scr.color(colWords, colBar)
	scr.fill(y, 0, 1, scr.w, ' ')
	where := fmt.Sprintf("%s  step %d", s.entry, s.steps)
	if st.Line > 0 {
		where += fmt.Sprintf("  %d:%d", st.Line, st.Col)
	}
	room := scr.w - len(where) - 2
	if s.msg != "" {
		scr.print(y, 1, cutTo(s.msg, room-1))
	} else {
		col := 1
		for _, h := range hints {
			rest := h.rest
			if h.key == "x" && s.bytes {
				rest = " code" // what x goes to
			}
			if col+len(h.key)+len(rest) > room {
				break
			}
			scr.color(colKeys, colBar)
			scr.style(true, false)
			col += scr.print(y, col, h.key)
			scr.color(colWords, colBar)
			scr.style(false, false)
			col += scr.print(y, col, rest) + 2
		}
	}
	scr.color(colWhere, colBar)
	scr.print(y, scr.w-len(where)-1, where)
}

func expandTabs(line string) string {
	var b strings.Builder
	col := 0
	for _, r := range strings.TrimRight(line, "\r") {
		if r == '\t' {
			for {
				b.WriteByte(' ')
				col++
				if col%tabWidth == 0 {
					break
				}
			}
			continue
		}
		b.WriteRune(r)
		col++
	}
	return b.String()
}

// prefixBytes is the first n bytes of line: a column in the debug section
// counts bytes.
func prefixBytes(line string, n int) string {
	return line[:min(n, len(line))]
}

func dropRunes(text string, n int) []rune {
	rs := []rune(text)
	if n >= len(rs) {
		return nil
	}
	return rs[max(n, 0):]
}

// wrap breaks text into lines of at most cols runes, at spaces.
func wrap(text string, cols int) []string {
	var lines []string
	line := ""
	for w := range strings.FieldsSeq(text) {
		if line != "" && len([]rune(line))+1+len([]rune(w)) > cols {
			lines = append(lines, line)
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += w
	}
	return append(lines, line)
}

func cutTo(text string, cols int) string {
	rs := []rune(text)
	if cols <= 0 {
		return ""
	}
	if len(rs) > cols {
		return string(rs[:cols])
	}
	return text
}
