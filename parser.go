package filo

import (
	"fmt"
	"strconv"
	"strings"
)

type Node any

type NumberLit struct {
	Value float64
}

type BoolLit struct {
	Value bool
}

type StringLit struct {
	Value string
}

type Symbol struct {
	Name string
}

type List struct {
	Elems []Node
}

type ResolvedBuiltin struct {
	Name string
	fn   builtinFunc
}

type ResolvedGlobal struct {
	Name string
	ID   int
}

// ParseError represents a syntax/lexing error with a byte position in the source.
// Position is 0-based and refers to a byte offset in the original string.
// Near contains a small snippet around the error location to aid debugging.
type ParseError struct {
	Pos     int
	Near    string
	Message string
}

func (e *ParseError) Error() string {
	if e == nil {
		return "parse error"
	}
	if e.Near != "" {
		return fmt.Sprintf("parse error at position %d near %q: %s", e.Pos, e.Near, e.Message)
	}
	return fmt.Sprintf("parse error at position %d: %s", e.Pos, e.Message)
}

type lexer struct {
	src string
	i   int
}

// Parse parses the input string and returns the AST (List of expressions)
func Parse(input string) (Node, error) {
	lx := &lexer{src: input}
	var nodes []Node

	// Read all top-level expressions
	for {
		lx.skipWS()
		if lx.i >= len(lx.src) {
			break
		}
		node, err := lx.readNode()
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}

	// If no nodes, return error
	if len(nodes) == 0 {
		return nil, &ParseError{Pos: 0, Near: snippetNear(input, 0), Message: "empty script"}
	}

	// If single node, return it directly
	if len(nodes) == 1 {
		return nodes[0], nil
	}

	// Multiple nodes: wrap in implicit (let () ...) block
	// This allows sequential evaluation with the last value returned
	return &List{Elems: append([]Node{&Symbol{Name: "let"}, &List{Elems: []Node{}}}, nodes...)}, nil
}

func (l *lexer) readNode() (Node, error) {
	l.skipWS()
	if l.i >= len(l.src) {
		return nil, l.errAt(l.i, "unexpected end of input")
	}

	c := l.src[l.i]
	switch c {
	case '(':
		openPos := l.i
		l.i++
		return l.readList(openPos)
	case '"':
		return l.readString(l.i)
	case '#':
		return l.readBool()
	default:
		return l.readAtom()
	}
}

func (l *lexer) skipWS() {
	for l.i < len(l.src) {
		switch l.src[l.i] {
		case ' ', '\t', '\n', '\r':
			l.i++
		case ';':
			for l.i < len(l.src) && l.src[l.i] != '\n' {
				l.i++
			}
		default:
			return
		}
	}
}

func (l *lexer) readList(openPos int) (Node, error) {
	var elems []Node
	for {
		l.skipWS()
		if l.i >= len(l.src) {
			return nil, l.errAt(openPos, "unterminated list")
		}
		if l.src[l.i] == ')' {
			l.i++
			break
		}
		node, err := l.readNode()
		if err != nil {
			return nil, err
		}
		elems = append(elems, node)
	}
	return &List{Elems: elems}, nil
}

func (l *lexer) readString(startPos int) (Node, error) {
	l.i++ // consume opening quote
	var b strings.Builder
	for l.i < len(l.src) {
		c := l.src[l.i]
		if c == '"' {
			l.i++
			return &StringLit{Value: b.String()}, nil
		}
		if c == '\\' {
			l.i++
			if l.i >= len(l.src) {
				return nil, l.errAt(startPos, "unterminated escape sequence")
			}
			escaped := l.src[l.i]
			switch escaped {
			case '"': // double quote
				b.WriteByte('"')
			case 'n': // newline (line feed)
				b.WriteByte('\n')
			case 't': // horizontal tab
				b.WriteByte('\t')
			case 'r': // carriage return
				b.WriteByte('\r')
			case '\\': // backslash
				b.WriteByte('\\')
			case '0': // null character
				b.WriteByte('\x00')
			case 'a': // alert (bell)
				b.WriteByte('\a')
			case 'b': // backspace
				b.WriteByte('\b')
			case 'f': // form feed
				b.WriteByte('\f')
			case 'v': // vertical tab
				b.WriteByte('\v')
			default:
				return nil, l.errAt(l.i, fmt.Sprintf("unsupported escape: \\%c", escaped))
			}
			l.i++
			continue
		}
		b.WriteByte(c)
		l.i++
	}
	return nil, l.errAt(startPos, "unterminated string literal")
}

func (l *lexer) readBool() (Node, error) {
	if strings.HasPrefix(l.src[l.i:], "#t") {
		l.i += 2
		return &BoolLit{Value: true}, nil
	}
	if strings.HasPrefix(l.src[l.i:], "#f") {
		l.i += 2
		return &BoolLit{Value: false}, nil
	}
	return nil, l.errAt(l.i, "invalid boolean literal")
}

func (l *lexer) readAtom() (Node, error) {
	start := l.i
	for l.i < len(l.src) {
		switch l.src[l.i] {
		case ' ', '\t', '\n', '\r', '(', ')':
			goto done
		default:
			l.i++
		}
	}
done:
	text := l.src[start:l.i]
	if text == "" {
		return nil, l.errAt(start, "expected token")
	}
	num, err := strconv.ParseFloat(text, 64)
	if err == nil {
		return &NumberLit{Value: num}, nil
	}
	return &Symbol{Name: text}, nil
}

func (l *lexer) errAt(pos int, msg string) error {
	return &ParseError{Pos: pos, Near: snippetNear(l.src, pos), Message: msg}
}

func snippetNear(src string, pos int) string {
	pos = min(max(pos, 0), len(src))
	const radius = 20
	start := max(pos-radius, 0)
	end := min(pos+radius, len(src))
	snippet := src[start:end]
	snippet = strings.ReplaceAll(snippet, "\n", " ")
	snippet = strings.ReplaceAll(snippet, "\r", " ")
	snippet = strings.ReplaceAll(snippet, "\t", " ")
	snippet = strings.TrimSpace(snippet)
	return snippet
}
