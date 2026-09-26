package main

import (
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/crgimenes/filo/fbc"
)

func screenText(scr *screen, from, to int) string {
	var b strings.Builder
	for row := 0; row < scr.h; row++ {
		for col := from; col < to; col++ {
			b.WriteRune(scr.at(row, col).r)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// The help names every instruction the format has, so a new one cannot be
// left out of it.
func TestHelpNamesEveryInstruction(t *testing.T) {
	for op := range 32 {
		u := &fbc.Unit{Data: []byte{byte(op << 3), 0, 0, 0}, Code: fbc.Span{Len: 4}}
		in := u.Insn(0)
		if in.Op < 0 {
			continue
		}
		if !strings.Contains(debugHelp, "\n  "+in.Name+" ") {
			t.Errorf("the help does not explain %s", in.Name)
		}
	}
}

// h brings the page up, over the whole screen; space pages, the top stays
// in range, and h, q or Esc go back without quitting.
func TestHelpPage(t *testing.T) {
	s := testSession(t, "fail")
	s.key([]byte("h"))
	scr := &screen{}
	scr.resize(80, 20)
	draw(s, scr)
	if !s.help || !strings.Contains(screenText(scr, 0, scr.w), "A program runs here one step at a time") {
		t.Fatalf("h did not show the help:\n%s", screenText(scr, 0, scr.w))
	}
	for range 100 {
		s.key([]byte(" "))
		draw(s, scr)
	}
	if s.helpTop != len(helpLines)-(scr.h-1) {
		t.Fatalf("paged past the end: top %d of %d lines", s.helpTop, len(helpLines))
	}
	if !strings.Contains(screenText(scr, 0, scr.w), "docs/bytecode.md") {
		t.Fatalf("the last page:\n%s", screenText(scr, 0, scr.w))
	}
	for _, k := range []string{"q", "h", "\x1b"} {
		if !s.help {
			s.key([]byte("h"))
		}
		if !s.key([]byte(k)) || s.help {
			t.Fatalf("%q in the help quit, or stayed", k)
		}
	}
	if s.steps != 0 {
		t.Fatal("a key in the help moved the run")
	}
}

// x shows the unit's bytes on the right, region by region, the
// instruction the run is at marked; x again, the instructions.
func TestBytesView(t *testing.T) {
	s := testSession(t, "fail")
	s.key([]byte("x"))
	scr := &screen{}
	scr.resize(120, 100) // the whole unit, 399 bytes, in view
	draw(s, scr)
	right := screenText(scr, scr.w/2, scr.w)
	for _, want := range []string{"header", "constants", "code", "four", "x code"} {
		if !strings.Contains(right+screenText(scr, 0, scr.w/2), want) {
			t.Errorf("the bytes view lacks %q:\n%s", want, right)
		}
	}
	data, err := os.ReadFile("../../testdata/bytecode/prog.fbc")
	if err != nil {
		t.Fatal(err)
	}
	st := s.shown()
	at := s.listing.Code.Off + st.Frames[len(st.Frames)-1].PC
	marked := false
	for row := 0; row < scr.h; row++ {
		for col := scr.w / 2; col < scr.w-1; col++ {
			c := scr.at(row, col)
			if c.bg == colHereBg && string([]rune{c.r, scr.at(row, col+1).r}) == strings.ToLower(hex2(data[at])) {
				marked = true
			}
		}
	}
	if !marked {
		t.Fatalf("the byte %02x the run is at is not marked:\n%s", data[at], right)
	}
	s.key([]byte("x"))
	draw(s, scr)
	if !strings.Contains(screenText(scr, scr.w/2, scr.w), "CLOSURE") {
		t.Fatal("x again did not bring the instructions back")
	}
}

func hex2(b byte) string {
	const digits = "0123456789abcdef"
	return string([]byte{digits[b>>4], digits[b&15]})
}

// The operands the next instruction takes are marked: before (half "four")
// is called, CALL takes the function and "four", the top two.
func TestOperandsTheNextInstructionTakes(t *testing.T) {
	s := testSession(t, "fail")
	for range 5 { // CLOSURE, STORE_G, POP, PUSH_G, PUSH_K
		s.instruction()
	}
	scr := &screen{}
	scr.resize(100, 24)
	draw(s, scr)
	text := screenText(scr, 0, scr.w)
	if !strings.Contains(text, `operands (<fn> "four")  CALL takes 2`) {
		t.Fatalf("no CALL taking two:\n%s", text)
	}
	for row := 0; row < scr.h; row++ {
		var line strings.Builder
		for col := range scr.w {
			line.WriteRune(scr.at(row, col).r)
		}
		at := strings.Index(line.String(), `"four"`)
		if at < 0 || !strings.Contains(line.String(), "takes") {
			continue
		}
		col := len([]rune(line.String()[:at]))
		if scr.at(row, col).fg != colKeys || scr.at(row, col-2).fg != colKeys {
			t.Fatal("the operands CALL takes are not marked")
		}
		return
	}
	t.Fatal("no row with the operands")
}

func TestConsumes(t *testing.T) {
	u, err := fbc.Read(mustRead(t, "../../testdata/bytecode/prog.fbc"))
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for pc := 0; pc < u.Code.Len; {
		in := u.Insn(pc)
		got := consumes(in)
		argc, _ := strconv.Atoi(strings.Fields(in.Operands)[0])
		switch {
		case in.Op == fbc.OpCallB && got != argc,
			in.Op == fbc.OpPushK && got != 0,
			in.Op == fbc.OpRet && got != 1:
			t.Errorf("%s %s takes %d", in.Name, in.Operands, got)
		}
		seen[in.Name] = true
		pc += in.Len
	}
	if !seen["CALLB"] || !seen["PUSH_K"] || !seen["RET"] {
		t.Fatalf("prog.fbc lacks the instructions this test reads: %v", seen)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}
