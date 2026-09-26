package main

import (
	_ "embed"
	"fmt"
	"sort"
	"strings"

	"github.com/crgimenes/filo/fbc"
)

//go:embed debug_help.txt
var debugHelp string

var helpLines = strings.Split(strings.TrimRight(debugHelp, "\n"), "\n")

// drawHelp is the h page, the whole screen: the language, the machine, its
// instructions and bytes, scrolled from s.helpTop; "# " starts a heading.
func drawHelp(s *session, scr *screen) {
	rows := scr.h - 1
	s.helpPage = rows
	s.helpTop = max(0, min(s.helpTop, len(helpLines)-rows))
	for row := 0; row < rows && s.helpTop+row < len(helpLines); row++ {
		line := helpLines[s.helpTop+row]
		heading, isHeading := strings.CutPrefix(line, "# ")
		if isHeading {
			scr.color(colKeys, colorDefault)
			scr.style(true, false)
			line = heading
		} else {
			scr.color(colorDefault, colorDefault)
		}
		scr.print(row, 1, cutTo(line, scr.w-2))
		scr.style(false, false)
	}
	y := scr.h - 1
	scr.color(colWords, colBar)
	scr.fill(y, 0, 1, scr.w, ' ')
	col := 1
	for _, h := range []struct{ key, rest string }{{"arrows", " scroll"}, {"space", " page"}, {"h", " back"}} {
		scr.color(colKeys, colBar)
		scr.style(true, false)
		col += scr.print(y, col, h.key)
		scr.color(colWords, colBar)
		scr.style(false, false)
		col += scr.print(y, col, h.rest) + 2
	}
	last := min(s.helpTop+rows, len(helpLines))
	where := fmt.Sprintf("help  %d-%d of %d", s.helpTop+1, last, len(helpLines))
	scr.color(colWhere, colBar)
	scr.print(y, scr.w-len(where)-1, where)
}

// byteRow is a row of the bytes view: a region's heading (n is 0) or n
// bytes of the unit from off.
type byteRow struct {
	off, n int
	text   string
}

// byteRows is the unit as the file holds it, region by region, perRow
// bytes a row, each region starting a row of its own under its heading.
func byteRows(s *session, perRow int) []byteRow {
	u := s.listing
	type region struct {
		name     string
		off, len int
	}
	regions := []region{{"header", 0, u.HeaderSize}}
	secs := append(u.Sections[:0:0], u.Sections...)
	sort.Slice(secs, func(i, j int) bool { return secs[i].Off < secs[j].Off })
	at := u.HeaderSize
	for _, sec := range secs {
		if sec.Off > at {
			regions = append(regions, region{"between", at, sec.Off - at})
		}
		name := fmt.Sprintf("kind %d", sec.Kind)
		if sec.Kind < len(fbc.SectionNames) && fbc.SectionNames[sec.Kind] != "" {
			name = fbc.SectionNames[sec.Kind]
		}
		regions = append(regions, region{name, sec.Off, sec.Len})
		at = max(at, sec.Off+sec.Len)
	}
	if at < len(u.Data) {
		regions = append(regions, region{"after", at, len(u.Data) - at})
	}
	var rows []byteRow
	for _, r := range regions {
		rows = append(rows, byteRow{off: r.off, text: fmt.Sprintf(" %-10s %04x  %d bytes", r.name, r.off, r.len)})
		for off := r.off; off < r.off+r.len; off += perRow {
			rows = append(rows, byteRow{off: off, n: min(perRow, r.off+r.len-off)})
		}
	}
	return rows
}

// drawBytes is the x view of the right side: the bytes of the unit, the
// instruction the run is at in orange under black, the cursor's line's in
// orange, kept in view as drawCode keeps its rows.
func drawBytes(s *session, scr *screen, fn, pc, line, rows, col int) {
	width := scr.w - col
	if fn < 0 {
		return
	}
	u := s.listing
	perRow := 4
	for perRow < 16 && 9+4*perRow*2 <= width { // " 0000  " hex, a space, text
		perRow *= 2
	}
	here, hereLen, target, marked := byteMarks(s, fn, pc, line)
	scr.color(colDim, colorDefault)
	scr.print(0, col, cutTo(fmt.Sprintf(" at fn %d, byte %04x of %d", fn, here, len(u.Data)), width))
	all := byteRows(s, perRow)
	at := 0
	for i, r := range all {
		if r.n > 0 && target >= r.off && target < r.off+r.n {
			at = i
			break
		}
	}
	first := max(0, at-(rows-2)/2)
	for row := 1; row < rows && first+row-1 < len(all); row++ {
		r := all[first+row-1]
		if r.n == 0 {
			scr.color(colDim, colorDefault)
			scr.print(row, col, cutTo(r.text, width))
			continue
		}
		scr.color(colorDefault, colorDefault)
		x := col + scr.print(row, col, fmt.Sprintf(" %04x  ", r.off))
		text := col + 8 + 3*perRow + 1
		for k := range r.n {
			off := r.off + k
			fg, bg := int16(colorDefault), int16(colorDefault)
			switch {
			case off >= here && off < here+hereLen:
				fg, bg = colHereFg, colHereBg
			case inSpans(marked, off):
				fg = colKeys
			}
			scr.color(fg, bg)
			scr.print(row, x, fmt.Sprintf("%02x", u.Data[off]))
			ch := rune(u.Data[off])
			if ch < 0x20 || ch > 0x7e {
				ch = '.'
			}
			scr.print(row, text+k, string(ch))
			x += 3
		}
	}
}

// byteMarks is where, in the file, the instruction at pc is and how long;
// the byte to keep in view (the cursor's line's first, when the cursor is
// off the run's line, else that instruction's); and the spans of the
// cursor's line's instructions.
func byteMarks(s *session, fn, pc, line int) (here, hereLen, target int, marked [][2]int) {
	off := s.listing.Code.Off
	here = off + pc
	target = here
	file, cursorOff := s.fileOf[fn], s.cursor != line
	code, _ := codeRows(s, pc)
	for _, r := range code {
		if r.of < 0 {
			continue
		}
		start := off + r.in.PC
		if r.in.PC == pc {
			hereLen = r.in.Len
		}
		if cursorOff && r.line == s.cursor && s.fileOf[r.of] == file {
			if len(marked) == 0 {
				target = start
			}
			marked = append(marked, [2]int{start, start + r.in.Len})
		}
	}
	return here, hereLen, target, marked
}

func inSpans(spans [][2]int, off int) bool {
	for _, sp := range spans {
		if off >= sp[0] && off < sp[1] {
			return true
		}
	}
	return false
}
