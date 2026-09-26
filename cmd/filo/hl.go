package main

import "strings"

// Filo's colours in the source, the edt's (filo-term's hl.c, HL_FILO, and
// the edt's class-fg): the same classes found the same way, so a line looks
// the same in both.
const (
	hlPlain = iota
	hlComment
	hlString
	hlNumber
	hlKeyword
)

var hlColor = [...]int16{colorDefault, 244, 108, 139, 74}

const hlKeywords = " def fn let letv if cond else do set and or return exit "

func hlWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_' ||
		b != 0 && strings.IndexByte("-?!*<>=/+%", b) >= 0
}

// hlStringEnd is the end of a string that opened before i, or the line's;
// closed says which. An escaped quote does not close it.
func hlStringEnd(line string, i int) (end int, closed bool) {
	for i < len(line) && line[i] != '"' {
		if line[i] == '\\' && i+1 < len(line) {
			i++
		}
		i++
	}
	if i < len(line) {
		return i + 1, true
	}
	return i, false
}

// hlClasses is the class of each byte of line, and whether a string is
// still open at its end (a Filo string may span lines); inString says one
// was open at its start.
func hlClasses(line string, inString bool) ([]byte, bool) {
	cls := make([]byte, len(line))
	mark := func(c byte, from, to int) {
		for k := from; k < to; k++ {
			cls[k] = c
		}
	}
	i := 0
	if inString {
		end, closed := hlStringEnd(line, 0)
		mark(hlString, 0, end)
		if !closed {
			return cls, true
		}
		i = end
	}
	start := i // where the pending plain run starts: a number may begin there
	for i < len(line) {
		b := line[i]
		switch {
		case b == ';':
			mark(hlComment, i, len(line))
			return cls, false
		case b == '"':
			end, closed := hlStringEnd(line, i+1)
			mark(hlString, i, end)
			if !closed {
				return cls, true
			}
			i, start = end, end
		case b >= '0' && b <= '9' && (i == start || !hlWordByte(line[i-1])):
			from := i
			for i < len(line) && (hlWordByte(line[i]) || line[i] == '.') {
				i++
			}
			mark(hlNumber, from, i)
			start = i
		case hlWordByte(b) && (i == 0 || !hlWordByte(line[i-1])):
			from := i
			for i < len(line) && hlWordByte(line[i]) {
				i++
			}
			if strings.Contains(hlKeywords, " "+line[from:i]+" ") {
				mark(hlKeyword, from, i)
				start = i
			}
		default:
			i++
		}
	}
	return cls, false
}
