package filo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// Show is one stage of what the compiler makes of src, a line at a time,
// each with the line and column it came from: "tree" as the reader read it,
// "folded" once constants folded, "ir" as lowered. It is for learning how a
// program becomes something to run, and writes what the C runtime's
// filo_show writes, byte for byte; the bytecode has its listing in package
// fbc.
func (e *Engine) Show(src, stage string) ([]string, error) {
	if stage != "tree" && stage != "folded" && stage != "ir" {
		return nil, fmt.Errorf("no stage named %s", stage)
	}
	ast, source, err := parseSource(src)
	if err != nil {
		return nil, err
	}
	s := &shower{source: source}
	if stage == "tree" {
		s.tree(ast, 0)
		return s.lines, nil
	}
	folded, _ := foldNode(ast, source)
	if stage == "folded" {
		s.tree(folded, 0)
		return s.lines, nil
	}
	ir, err := e.lowerSource(folded, source)
	if err != nil {
		if at, ok := errors.AsType[*PositionError](err); ok {
			return nil, &PositionError{Line: at.Line, Col: at.Col, Err: fmt.Errorf("compile error: %w", at.Err)}
		}
		return nil, fmt.Errorf("compile error: %w", err)
	}
	s.ir(ir, 0)
	return s.lines, nil
}

// The C runtime's buffers, which cut what does not fit: a value's text at
// 159 bytes, a line's text at 255, a whole line at 319.
const (
	showValueMost = 159
	showTextMost  = 255
	showLineMost  = 319
)

var opNames = []string{
	"const", "local", "global", "dynamic", "builtin", "empty", "invalid", "if",
	"cond", "do", "and", "or", "let", "letv", "set", "fn",
	"def", "tuple", "exit", "return", "callb", "call",
}

type shower struct {
	source *sourceMap
	lines  []string
}

func cut(s string, most int) string {
	if len(s) > most {
		return s[:most]
	}
	return s
}

// line is "line:col" padded to 8, the text indented by depth.
func (s *shower) line(line, col, depth int, text string) {
	at := strconv.Itoa(line) + ":" + strconv.Itoa(col)
	b := []byte(at)
	for len(b) < 8 {
		b = append(b, ' ')
	}
	b = append(b, strings.Repeat("  ", depth)...)
	b = append(b, cut(text, showTextMost)...)
	s.lines = append(s.lines, cut(string(b), showLineMost))
}

func (s *shower) nodePos(n Node) (int, int) {
	at, ok := s.source.at[n]
	if !ok {
		return 0, 0
	}
	return s.source.lineCol(int32(min(at, 1<<30)) + 1) // #nosec G115 -- bounded above
}

func showValue(v Value) string {
	if v.Walkable() != nil {
		return "?"
	}
	return cut(v.text(), showValueMost)
}

func (s *shower) tree(n Node, depth int) {
	line, col := s.nodePos(n)
	switch t := n.(type) {
	case *List:
		s.line(line, col, depth, "list of "+strconv.Itoa(len(t.Elems)))
		for _, e := range t.Elems {
			s.tree(e, depth+1)
		}
	case *Symbol:
		s.line(line, col, depth, "symbol "+t.Name)
	case *NumberLit:
		s.line(line, col, depth, "number "+showValue(VNum(t.Value)))
	case *BoolLit:
		s.line(line, col, depth, "bool "+showValue(VBool(t.Value)))
	case *StringLit:
		s.line(line, col, depth, "string "+showValue(VString(t.Value)))
	}
}

func (s *shower) instrPos(in *Instr) (int, int) {
	if in.pos == 0 {
		return 0, 0
	}
	return s.source.lineCol(in.pos)
}

func (s *shower) ir(in *Instr, depth int) {
	text := "?"
	if int(in.Op) < len(opNames) {
		text = opNames[in.Op]
	}
	switch {
	case in.Op == OpConst:
		text += " " + showValue(in.Val)
	case in.Name != "":
		text += " " + in.Name
	case in.Op == OpExit || in.Op == OpReturn:
		text += " " + text // the C runtime's IR names the signal it raises
	}
	if in.Op == OpLocal {
		text += fmt.Sprintf("  (frame %d out, slot %d)", in.A, in.B)
	}
	if len(in.Names) > 0 {
		text += "  names " + strings.Join(in.Names, " ")
	}
	if in.Msg != "" {
		text += "  error: " + in.Msg
	}
	line, col := s.instrPos(in)
	s.line(line, col, depth, text)
	for _, a := range in.Args {
		s.ir(a, depth+1)
	}
	for _, c := range in.Clauses {
		label := "clause"
		if c.Else {
			label = "clause else"
		}
		s.line(line, col, depth+1, label)
		if c.Test != nil {
			s.ir(c.Test, depth+2)
		}
		for _, b := range c.Body {
			s.ir(b, depth+2)
		}
	}
}
