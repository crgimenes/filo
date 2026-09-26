// Lisp-style formatting for Filo code.
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
	"unicode/utf8"
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
	nodes, ok := node.([]Node)
	if ok {
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
	blanksBefore int  // number of blank lines before this token
	trailing     bool // a comment on the same line as the code before it
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
				trailing:     codeBefore(src, start),
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

// codeBefore reports whether something other than blanks precedes position
// i on its line. A comment there belongs to that code: `(def x 1) ; why`
// moved to a line of its own would read as the comment of the next form.
func codeBefore(src string, i int) bool {
	j := i - 1
	for j >= 0 && (src[j] == ' ' || src[j] == '\t' || src[j] == '\r') {
		j--
	}
	return j >= 0 && src[j] != '\n'
}

// Layout in one rule: a form that fits on the rest of the line stays on one
// line; one that does not keeps its leading children on the head line while
// they fit, and from the first that does not, every child takes a line of
// its own — so nothing ever trails behind a multi-line argument. Special
// forms keep a fixed head instead (see headInline), and so does a cond
// clause: its test. The close parens that follow a child land on its line,
// so they count toward whether it fits. Comments and blank lines stay where
// they are and rule out packing the form around them.
type frame struct {
	head     string // symbol right after the paren, "" until seen
	children int    // children laid out so far, the head not counted
	broken   bool   // a child took a line of its own: the rest follow
	align    int    // column the children of a broken frame start on
	inline   int    // children the head line keeps; -1 for an ordinary call
	clause   bool   // a clause of cond: the head line keeps the test
}

// headInline is how many children a form keeps on its head line once it has
// to break: the params of fn, the bindings of let, the name of def, the
// names and value of letv, the condition of if; none at all for the forms
// that read as a column — cond clauses, do steps, the items of a list.
// -1 is an ordinary call: leading children stay while they fit.
func headInline(head string) int {
	switch head {
	case "fn", "let", "def", "if":
		return 1
	case "letv":
		return 2
	case "cond", "do", "list":
		return 0
	default:
		return -1
	}
}

// span describes the parenthesised group opened at a token.
type span struct {
	end     int    // index of the matching close, -1 if unbalanced
	compact string // the group on one line, "" when it cannot be
}

func spans(tokens []token) []span {
	out := make([]span, len(tokens))
	var stack []int
	for i, tok := range tokens {
		switch tok.typ {
		case tokOpen:
			out[i].end = -1
			stack = append(stack, i)
		case tokClose:
			if len(stack) > 0 {
				open := stack[len(stack)-1]
				stack = stack[:len(stack)-1]
				out[open].end = i
				out[open].compact = compactSpan(tokens, open, i)
			}
		case tokAtom, tokComment, tokBlank:
		}
	}
	return out
}

// compactSpan writes tokens[open..close] on one line, or returns "" when a
// comment, a blank line or a multi-line string inside forbids it.
func compactSpan(tokens []token, open, close int) string {
	var b strings.Builder
	prevOpen := false
	for i := open; i <= close; i++ {
		tok := tokens[i]
		if tok.blanksBefore > 0 && i > open {
			return ""
		}
		switch tok.typ {
		case tokComment, tokBlank:
			return ""
		case tokOpen:
			if i > open && !prevOpen {
				b.WriteString(" ")
			}
			b.WriteString("(")
			prevOpen = true
		case tokClose:
			b.WriteString(")")
			prevOpen = false
		case tokAtom:
			if strings.Contains(tok.value, "\n") {
				return ""
			}
			if !prevOpen {
				b.WriteString(" ")
			}
			b.WriteString(tok.value)
			prevOpen = false
		}
	}
	return b.String()
}

// fitsHere says whether something `width` runes wide goes on the current
// line; -1 is a thing that cannot go on one line at all.
func fitsHere(width, col int, space bool, cfg FormatConfig) bool {
	if width < 0 {
		return false
	}
	avail := cfg.MaxLineWidth - col
	if space {
		avail--
	}
	return width <= avail
}

// endsWithOpen reports whether the last byte written is an open paren, the
// one place no space goes before the next token.
func endsWithOpen(b *strings.Builder) bool {
	s := b.String()
	return len(s) > 0 && s[len(s)-1] == '('
}

// layout is the state of one formatting pass: the text so far, where the
// cursor is on the line, and the stack of open forms.
type layout struct {
	b           strings.Builder
	cfg         FormatConfig
	tokens      []token
	groups      []span
	indentWidth int
	col         int
	atLineStart bool
	fresh       bool // a line was just started: nothing goes before the token
	stack       []frame
}

func (l *layout) write(s string) {
	l.b.WriteString(s)
	if nl := strings.LastIndexByte(s, '\n'); nl >= 0 {
		l.col = utf8.RuneCountInString(s[nl+1:])
	} else {
		l.col += utf8.RuneCountInString(s)
	}
	l.atLineStart = false
	l.fresh = false
}

func (l *layout) newline(indent int) {
	if !l.atLineStart {
		l.b.WriteString("\n")
	}
	l.b.WriteString(strings.Repeat(" ", indent))
	l.col = indent
	l.atLineStart = false
	l.fresh = true
}

// blankLine ends the current line and leaves one empty one.
func (l *layout) blankLine() {
	if !l.atLineStart {
		l.b.WriteString("\n")
	}
	l.b.WriteString("\n")
	l.col = 0
	l.atLineStart = true
	if p := l.parent(); p != nil {
		p.broken = true
	}
}

func (l *layout) needSpace() bool {
	return !l.atLineStart && !l.fresh && !endsWithOpen(&l.b)
}

func (l *layout) parent() *frame {
	if len(l.stack) == 0 {
		return nil
	}
	return &l.stack[len(l.stack)-1]
}

func (l *layout) childIndent() int {
	p := l.parent()
	if p == nil {
		return 0
	}
	return p.align
}

// stays says whether the next child of p, `width` runes wide, may go on the
// current line: always in the head slot of a special form, and in an
// ordinary call while nothing has broken yet and it fits.
func (l *layout) stays(p *frame, width int) bool {
	if p.inline >= 0 {
		return p.children < p.inline
	}
	return !p.broken && fitsHere(width, l.col, l.needSpace(), l.cfg)
}

// place puts the next child of p on the current line or on one of its own.
func (l *layout) place(p *frame, width int) {
	if p == nil {
		if !l.atLineStart {
			l.newline(0) // top-level forms never share a line
		}
		return
	}
	if !l.stays(p, width) {
		l.newline(l.childIndent())
		p.broken = true
	}
}

func (l *layout) comment(tok token) {
	sep := " "
	if !tok.trailing || l.atLineStart {
		l.newline(l.childIndent())
		sep = ""
	}
	l.write(sep + tok.value)
	l.b.WriteString("\n")
	l.col = 0
	l.atLineStart = true
	if p := l.parent(); p != nil {
		p.broken = true
	}
}

// open lays out the group starting at token i and returns the index of the
// first token after what it consumed: the whole group when it fit on the
// line, only the paren when it has to break inside.
func (l *layout) open(i int) int {
	p := l.parent()
	g := l.groups[i]
	width := -1
	if g.compact != "" && g.end >= 0 {
		width = utf8.RuneCountInString(g.compact)
	}
	headSlot := p != nil && p.inline >= 0 && p.children < p.inline
	if width >= 0 {
		width += l.closersAfter(g.end)
	}
	l.place(p, width)
	if fitsHere(width, l.col, l.needSpace(), l.cfg) {
		if l.needSpace() {
			l.write(" ")
		}
		l.write(g.compact)
		if p != nil {
			p.children++
		}
		return g.end + 1
	}
	if l.needSpace() {
		l.write(" ")
	}
	f := frame{align: (len(l.stack) + 1) * l.indentWidth, inline: -1}
	if p != nil && p.head == "cond" {
		f.clause, f.inline = true, 1
	}
	if headSlot {
		// bindings and params open on the head line and stack their own
		// children under the first one
		f.align = l.col + 1
		if p.head == "let" || p.head == "letv" {
			f.inline = 1
		}
	}
	l.write("(")
	if p != nil {
		p.children++
	}
	l.stack = append(l.stack, f)
	return i + 1
}

func (l *layout) closersAfter(i int) int {
	n := 0
	for j := i + 1; j < len(l.tokens) && l.tokens[j].typ == tokClose && l.tokens[j].blanksBefore == 0; j++ {
		n++
	}
	return n
}

func (l *layout) close() {
	l.write(")")
	if len(l.stack) > 0 {
		l.stack = l.stack[:len(l.stack)-1]
	}
}

func (l *layout) atom(i int) {
	tok := l.tokens[i]
	p := l.parent()
	if p != nil && p.head == "" && p.children == 0 && endsWithOpen(&l.b) {
		l.write(tok.value)
		p.head = tok.value
		p.inline = headInline(tok.value)
		if p.clause {
			p.inline = 0 // the test is the head
		}
		return
	}
	width := utf8.RuneCountInString(tok.value)
	if strings.Contains(tok.value, "\n") {
		width = -1
	} else {
		width += l.closersAfter(i)
	}
	if p != nil {
		l.place(p, width)
	} else if l.atLineStart {
		l.newline(0)
	}
	if l.needSpace() {
		l.write(" ")
	}
	l.write(tok.value)
	if p != nil {
		p.children++
	}
}

func formatTokens(tokens []token, cfg FormatConfig) string {
	l := layout{
		cfg:         cfg,
		tokens:      tokens,
		groups:      spans(tokens),
		indentWidth: utf8.RuneCountInString(cfg.Indent),
		atLineStart: true,
	}
	i := 0
	for i < len(tokens) {
		tok := tokens[i]
		if tok.blanksBefore > 0 && i > 0 {
			l.blankLine()
		}
		switch tok.typ {
		case tokComment:
			l.comment(tok)
			i++
		case tokOpen:
			i = l.open(i)
		case tokClose:
			l.close()
			i++
		case tokAtom:
			l.atom(i)
			i++
		case tokBlank:
			i++
		}
	}
	result := strings.TrimRight(l.b.String(), "\n ")
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
		// (list), never (): the evaluator rejects () as an empty
		// list expression, and Marshal output must round-trip.
		return formatListCompact(v.List)
	case KTuple:
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

// formatString is s as Filo reads it back: between double quotes, with the
// escapes the reader knows, and every other byte as it is — invalid UTF-8
// and control bytes included, since the reader takes them raw and has no
// escape for them. The C runtime writes a string the same way.
func formatString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		case '\r':
			b.WriteString(`\r`)
		case 0:
			b.WriteString(`\0`)
		case '\a':
			b.WriteString(`\a`)
		case '\b':
			b.WriteString(`\b`)
		case '\f':
			b.WriteString(`\f`)
		case '\v':
			b.WriteString(`\v`)
		default:
			b.WriteByte(c)
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
	sym, ok := items[0].(*Symbol)
	if ok {
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
		bindings, ok := items[1].(*List)
		if ok {
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
