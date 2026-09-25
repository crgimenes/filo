package main

import (
	"bufio"
	"fmt"
	"io"
)

// A cell of the terminal: a rune and how it is drawn. A colour is one of
// the 256 of xterm, or colorDefault for the terminal's own.
type cell struct {
	r    rune
	fg   int16
	bg   int16
	bold bool
	rev  bool
}

const colorDefault = -1

// screen is a grid of cells drawn into, then sent as only what changed
// since the last frame: a debugger repaints on every key, and the terminal
// sees a few bytes.
type screen struct {
	w, h int
	cur  []cell
	prev []cell // what the terminal shows; nil sends the whole grid
	pen  cell
}

func (s *screen) resize(w, h int) {
	if w == s.w && h == s.h {
		return
	}
	s.w, s.h = w, h
	s.cur = make([]cell, w*h)
	s.prev = nil
}

func (s *screen) clear() {
	s.pen = cell{r: ' ', fg: colorDefault, bg: colorDefault}
	for i := range s.cur {
		s.cur[i] = s.pen
	}
}

func (s *screen) color(fg, bg int16) {
	s.pen.fg, s.pen.bg = fg, bg
}

func (s *screen) style(bold, rev bool) {
	s.pen.bold, s.pen.rev = bold, rev
}

// print writes text at row and col with the pen, cut at the right edge,
// and says how many columns it took.
func (s *screen) print(row, col int, text string) int {
	if row < 0 || row >= s.h {
		return 0
	}
	n := 0
	for _, r := range text {
		if col+n >= s.w {
			break
		}
		if col+n >= 0 {
			c := s.pen
			c.r = r
			s.cur[row*s.w+col+n] = c
		}
		n++
	}
	return n
}

// fill paints rows by cols from row and col with r in the pen.
func (s *screen) fill(row, col, rows, cols int, r rune) {
	for y := row; y < row+rows && y < s.h; y++ {
		for x := col; x < col+cols && x < s.w; x++ {
			if y >= 0 && x >= 0 {
				c := s.pen
				c.r = r
				s.cur[y*s.w+x] = c
			}
		}
	}
}

func (s *screen) at(row, col int) cell {
	return s.cur[row*s.w+col]
}

// flush sends what changed: a cursor move where a run of changes starts,
// the style only when it changes, and the runes.
func (s *screen) flush(out io.Writer) error {
	w := bufio.NewWriter(out)
	full := s.prev == nil
	if full {
		s.prev = make([]cell, len(s.cur))
		_, _ = w.WriteString("\x1b[0m\x1b[2J")
	}
	var pen cell
	penSet := false
	at := -1
	changed := false
	for i, c := range s.cur {
		if !full && c == s.prev[i] {
			continue
		}
		changed = true
		if at != i {
			_, _ = fmt.Fprintf(w, "\x1b[%d;%dH", i/s.w+1, i%s.w+1) // a write error sticks, and Flush returns it
		}
		style := c
		style.r = 0
		if !penSet || style != pen {
			_, _ = w.WriteString(sgr(c))
			pen, penSet = style, true
		}
		_, _ = w.WriteString(string(c.r))
		s.prev[i] = c
		at = i + 1
		if at%s.w == 0 {
			at = -1 // the terminal may wrap or not at the edge: move anew
		}
	}
	if changed {
		_, _ = w.WriteString("\x1b[0m")
	}
	return w.Flush()
}

func sgr(c cell) string {
	s := "\x1b[0"
	if c.bold {
		s += ";1"
	}
	if c.rev {
		s += ";7"
	}
	if c.fg != colorDefault {
		s += fmt.Sprintf(";38;5;%d", c.fg)
	}
	if c.bg != colorDefault {
		s += fmt.Sprintf(";48;5;%d", c.bg)
	}
	return s + "m"
}
