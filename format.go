// Package filo provides Lisp-style formatting for Filo code.
//
// The formatting follows common Lisp conventions for readability:
//   - Short expressions stay on one line: (+ 1 2)
//   - Function arguments align vertically when multiline
//   - Special forms (let, if, fn, def) have specific indentation rules
//   - Strings with escapes are preserved exactly: "test \" test"
//   - Comments and blank lines are preserved
package filo

import (
	"strconv"
	"strings"
)

// FormatConfig controls formatting behavior.
type FormatConfig struct {
	// Indent is the string used for each indentation level (default: 2 spaces).
	Indent string
	// MaxLineWidth is the preferred maximum line width (default: 80).
	MaxLineWidth int
}

// DefaultFormatConfig returns the default formatting configuration.
func DefaultFormatConfig() FormatConfig {
	return FormatConfig{
		Indent:       "  ",
		MaxLineWidth: 80,
	}
}

// FormatAST formatting an AST node back to string.
// This is used when the AST has been modified (e.g. by constant folding)
// and we can't use the token-based formatter.
func FormatAST(node Node, cfg FormatConfig) (string, error) {
	var b strings.Builder
	if nodes, ok := node.([]Node); ok {
		for i, n := range nodes {
			if i > 0 {
				b.WriteString("\n\n")
			}
			formatNode(&b, n, 0, cfg)
		}
	} else {
		formatNode(&b, node, 0, cfg)
	}
	return b.String(), nil
}

// Format formats Filo source code with Lisp-style indentation.
// It preserves comments and blank lines.
func Format(src string) (string, error) {
	return FormatWithConfig(src, DefaultFormatConfig())
}

// FormatWithConfig formats Filo source with custom configuration.
// Comments and intentional blank lines are preserved.
func FormatWithConfig(src string, cfg FormatConfig) (string, error) {
	tokens := tokenize(src)
	return formatTokens(tokens, cfg), nil
}

// Token types for formatting
type tokenType int

const (
	tokOpen    tokenType = iota // (
	tokClose                    // )
	tokAtom                     // symbol, number, bool, string
	tokComment                  // ; comment
	tokBlank                    // blank line(s)
)

type token struct {
	typ          tokenType
	value        string
	blanksBefore int // number of blank lines before this token
}

func tokenize(src string) []token {
	var tokens []token
	i := 0
	blankCount := 0

	for i < len(src) {
		c := src[i]

		// Count blank lines
		if c == '\n' {
			// Check if this is a blank line (just newline or whitespace before newline)
			lineStart := i
			i++
			for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
				i++
			}
			if i < len(src) && src[i] == '\n' {
				blankCount++
				continue
			}
			// Not a blank line, reset to after the newline
			i = lineStart + 1
			continue
		}

		// Skip whitespace (not newlines)
		if c == ' ' || c == '\t' || c == '\r' {
			i++
			continue
		}

		// Comment
		if c == ';' {
			start := i
			for i < len(src) && src[i] != '\n' {
				i++
			}
			tokens = append(tokens, token{
				typ:          tokComment,
				value:        src[start:i],
				blanksBefore: blankCount,
			})
			blankCount = 0
			continue
		}

		// Open paren
		if c == '(' {
			tokens = append(tokens, token{typ: tokOpen, value: "(", blanksBefore: blankCount})
			blankCount = 0
			i++
			continue
		}

		// Close paren
		if c == ')' {
			tokens = append(tokens, token{typ: tokClose, value: ")", blanksBefore: blankCount})
			blankCount = 0
			i++
			continue
		}

		// String
		if c == '"' {
			start := i
			i++ // skip opening quote
			for i < len(src) {
				if src[i] == '\\' && i+1 < len(src) {
					i += 2 // skip escape
					continue
				}
				if src[i] == '"' {
					i++ // include closing quote
					break
				}
				i++
			}
			tokens = append(tokens, token{
				typ:          tokAtom,
				value:        src[start:i],
				blanksBefore: blankCount,
			})
			blankCount = 0
			continue
		}

		// Atom (symbol, number, bool)
		start := i
		for i < len(src) {
			c := src[i]
			if c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '(' || c == ')' || c == ';' {
				break
			}
			i++
		}
		if i > start {
			tokens = append(tokens, token{
				typ:          tokAtom,
				value:        src[start:i],
				blanksBefore: blankCount,
			})
			blankCount = 0
		}
	}

	return tokens
}

func formatTokens(tokens []token, cfg FormatConfig) string {
	var b strings.Builder
	depth := 0
	atLineStart := true
	lastWasOpen := false

	writeIndent := func() {
		b.WriteString(strings.Repeat(cfg.Indent, depth))
	}

	for i, tok := range tokens {
		// Handle blank lines before this token
		if tok.blanksBefore > 0 && i > 0 {
			if !atLineStart {
				b.WriteString("\n")
			}
			// Collapse multiple consecutive blank lines into just one
			b.WriteString("\n")
			atLineStart = true
		}

		switch tok.typ {
		case tokComment:
			// Comments go on their own line
			if !atLineStart {
				b.WriteString("\n")
			}
			writeIndent()
			b.WriteString(tok.value)
			b.WriteString("\n")
			atLineStart = true
			lastWasOpen = false

		case tokOpen:
			// Before every ( break line (except at start)
			if !atLineStart {
				b.WriteString("\n")
				atLineStart = true
			}
			writeIndent()
			b.WriteString("(")
			depth++
			atLineStart = false
			lastWasOpen = true

		case tokClose:
			// ) doesn't break line, just close
			b.WriteString(")")
			depth--
			if depth < 0 {
				depth = 0
			}
			atLineStart = false
			lastWasOpen = false

		case tokAtom:
			if atLineStart {
				writeIndent()
			} else if !lastWasOpen {
				b.WriteString(" ")
			}
			b.WriteString(tok.value)
			atLineStart = false
			lastWasOpen = false
		}
	}

	result := strings.TrimRight(b.String(), "\n ")
	if result != "" {
		result += "\n"
	}
	return result
}

// FormatValue formats a Filo Value as a readable string.
func FormatValue(v Value) string {
	return FormatValueIndent(v, "", "  ")
}

// FormatValueIndent formats a Value with prefix and indent strings.
// Similar to json.MarshalIndent behavior.
func FormatValueIndent(v Value, prefix, indent string) string {
	// 1. Convert value to valid source code (compact)
	src := formatValueCompact(v)

	// 2. Format using the robust token-based formatter
	cfg := FormatConfig{
		Indent:       indent,
		MaxLineWidth: 80,
	}
	formatted, err := FormatWithConfig(src, cfg)
	if err != nil {
		// Fallback to compact if formatting fails (shouldn't happen)
		return src
	}

	// 3. Apply prefix to every line if needed
	if prefix == "" {
		return formatted
	}

	var b strings.Builder
	lines := strings.Split(formatted, "\n")
	for i, line := range lines {
		if i > 0 {
			b.WriteString("\n")
		}
		if line != "" {
			b.WriteString(prefix)
			b.WriteString(line)
		}
	}
	return b.String()
}

// formatValueAtDepth is removed as it's superseded by the token-based formatter.

func formatListCompact(list []Value) string {
	var b strings.Builder
	b.WriteString("(list")
	for _, item := range list {
		b.WriteString(" ")
		b.WriteString(formatValueCompact(item))
	}
	b.WriteString(")")
	return b.String()
}

func formatTupleCompact(tup []Value) string {
	var b strings.Builder
	b.WriteString("(tuple")
	for _, item := range tup {
		b.WriteString(" ")
		b.WriteString(formatValueCompact(item))
	}
	b.WriteString(")")
	return b.String()
}

func formatValueCompact(v Value) string {
	switch v.Kind {
	case KNumber:
		return formatNumber(v.Num)
	case KBool:
		if v.Bool {
			return "#t"
		}
		return "#f"
	case KString:
		return formatString(v.Str)
	case KList:
		if len(v.List) == 0 {
			return "()"
		}
		return formatListCompact(v.List)
	case KTuple:
		if len(v.Tup) == 0 {
			return "()"
		}
		return formatTupleCompact(v.Tup)
	case KFunc:
		return "<fn>"
	default:
		return v.String()
	}
}

// formatNumber formats a float64 as a compact string.
func formatNumber(n float64) string {
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// formatString properly escapes a string for Filo output.
// Handles: \", \\, \n, \t
func formatString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return b.String()
}

// formatNode formats an AST node with Lisp-style indentation.
func formatNode(b *strings.Builder, node Node, depth int, cfg FormatConfig) {
	switch n := node.(type) {
	case *NumberLit:
		b.WriteString(formatNumber(n.Value))
	case *StringLit:
		b.WriteString(formatString(n.Value))
	case *BoolLit:
		if n.Value {
			b.WriteString("#t")
		} else {
			b.WriteString("#f")
		}
	case *Symbol:
		b.WriteString(n.Name)
	case *List:
		formatList(b, n.Elems, depth, cfg)
	}
}

// formatList handles Lisp-style list formatting with special form awareness.
func formatList(b *strings.Builder, items []Node, depth int, cfg FormatConfig) {
	if len(items) == 0 {
		b.WriteString("()")
		return
	}

	// Check for special forms
	if sym, ok := items[0].(*Symbol); ok {
		switch sym.Name {
		case "let", "letv":
			formatLetForm(b, items, depth, cfg)
			return
		case "fn":
			formatFnForm(b, items, depth, cfg)
			return
		case "def":
			formatDefForm(b, items, depth, cfg)
			return
		case "if":
			formatIfForm(b, items, depth, cfg)
			return
		case "do":
			formatDoForm(b, items, depth, cfg)
			return
		}
	}

	// Try compact format first
	compact := compactNode(items)
	if len(compact) <= cfg.MaxLineWidth && !strings.Contains(compact, "\n") {
		b.WriteString(compact)
		return
	}

	// Multiline: function call style - first arg on same line if short
	b.WriteString("(")
	formatNode(b, items[0], depth, cfg)

	indent := strings.Repeat(cfg.Indent, depth+1)
	for i := 1; i < len(items); i++ {
		b.WriteString("\n")
		b.WriteString(indent)
		formatNode(b, items[i], depth+1, cfg)
	}
	b.WriteString(")")
}

func formatLetForm(b *strings.Builder, items []Node, depth int, cfg FormatConfig) {
	// (let ((x 1) (y 2)) body...)
	b.WriteString("(let ")

	indent := strings.Repeat(cfg.Indent, depth+1)
	bodyIndent := strings.Repeat(cfg.Indent, depth+1)

	if len(items) >= 2 {
		// Format bindings
		if bindings, ok := items[1].(*List); ok {
			compact := compactNode(bindings.Elems)
			if len(compact) <= 40 {
				b.WriteString(compact)
			} else {
				b.WriteString("(")
				bindIndent := indent + " "
				for i, bind := range bindings.Elems {
					if i > 0 {
						b.WriteString("\n")
						b.WriteString(bindIndent)
					}
					formatNode(b, bind, depth+2, cfg)
				}
				b.WriteString(")")
			}
		} else {
			formatNode(b, items[1], depth+1, cfg)
		}

		// Format body
		for i := 2; i < len(items); i++ {
			b.WriteString("\n")
			b.WriteString(bodyIndent)
			formatNode(b, items[i], depth+1, cfg)
		}
	}
	b.WriteString(")")
}

func formatFnForm(b *strings.Builder, items []Node, depth int, cfg FormatConfig) {
	// (fn (args) body...)
	b.WriteString("(fn ")

	indent := strings.Repeat(cfg.Indent, depth+1)

	if len(items) >= 2 {
		// Format params
		formatNode(b, items[1], depth+1, cfg)

		// Format body
		for i := 2; i < len(items); i++ {
			b.WriteString("\n")
			b.WriteString(indent)
			formatNode(b, items[i], depth+1, cfg)
		}
	}
	b.WriteString(")")
}

func formatDefForm(b *strings.Builder, items []Node, depth int, cfg FormatConfig) {
	// (def name expr)
	if len(items) <= 3 {
		compact := compactNode(items)
		if len(compact) <= cfg.MaxLineWidth {
			b.WriteString(compact)
			return
		}
	}

	b.WriteString("(def ")
	if len(items) >= 2 {
		formatNode(b, items[1], depth+1, cfg)
	}

	indent := strings.Repeat(cfg.Indent, depth+1)
	for i := 2; i < len(items); i++ {
		b.WriteString("\n")
		b.WriteString(indent)
		formatNode(b, items[i], depth+1, cfg)
	}
	b.WriteString(")")
}

func formatIfForm(b *strings.Builder, items []Node, depth int, cfg FormatConfig) {
	// (if cond then else)
	compact := compactNode(items)
	if len(compact) <= cfg.MaxLineWidth && !strings.Contains(compact, "\n") {
		b.WriteString(compact)
		return
	}

	b.WriteString("(if ")
	indent := strings.Repeat(cfg.Indent, depth+1)

	if len(items) >= 2 {
		formatNode(b, items[1], depth+1, cfg) // condition
	}
	for i := 2; i < len(items); i++ {
		b.WriteString("\n")
		b.WriteString(indent)
		formatNode(b, items[i], depth+1, cfg)
	}
	b.WriteString(")")
}

func formatDoForm(b *strings.Builder, items []Node, depth int, cfg FormatConfig) {
	// (do expr1 expr2 ...)
	compact := compactNode(items)
	if len(compact) <= cfg.MaxLineWidth && !strings.Contains(compact, "\n") {
		b.WriteString(compact)
		return
	}

	b.WriteString("(do")
	indent := strings.Repeat(cfg.Indent, depth+1)

	for i := 1; i < len(items); i++ {
		b.WriteString("\n")
		b.WriteString(indent)
		formatNode(b, items[i], depth+1, cfg)
	}
	b.WriteString(")")
}

func compactNode(items []Node) string {
	var b strings.Builder
	b.WriteString("(")
	for i, item := range items {
		if i > 0 {
			b.WriteString(" ")
		}
		compactSingleNode(&b, item)
	}
	b.WriteString(")")
	return b.String()
}

func compactSingleNode(b *strings.Builder, node Node) {
	switch n := node.(type) {
	case *NumberLit:
		b.WriteString(formatNumber(n.Value))
	case *StringLit:
		b.WriteString(formatString(n.Value))
	case *BoolLit:
		if n.Value {
			b.WriteString("#t")
		} else {
			b.WriteString("#f")
		}
	case *Symbol:
		b.WriteString(n.Name)
	case *List:
		b.WriteString(compactNode(n.Elems))
	}
}
