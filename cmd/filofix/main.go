// Command filofix modernizes and reduces Filo source files, in the spirit of
// `go fix`: it rewrites idioms the language has outgrown and applies safe
// source-level reductions, while preserving the file's formatting and comments.
//
// Fixes applied:
//
//   - root-let: a program whose single top-level form is `(let () body...)`
//     loses the wrapper. Early Filo required it; the interpreter has wrapped
//     multiple top-level forms implicitly for a long time.
//   - const-fold: a constant subexpression whose source span contains no
//     comment is replaced by its value — `(* 2 60)` becomes `120`. Only
//     expressions that fold to a single atom are touched, and only when
//     evaluating them succeeds, so behavior never changes.
//
// Usage:
//
//	filofix [flags] [path ...]
//
// Flags:
//
//	-w        write result to (source) file instead of stdout
//	-l        list files that would change
//	-d        display diffs instead of rewriting files
//
// Without an explicit path, it reads from standard input.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/crgimenes/filo"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("filofix", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		writeFlag bool
		listFlag  bool
		diffFlag  bool
		showVer   bool
	)

	fs.BoolVar(&writeFlag, "w", false, "write result to (source) file instead of stdout")
	fs.BoolVar(&listFlag, "l", false, "list files that would change")
	fs.BoolVar(&diffFlag, "d", false, "display diffs instead of rewriting files")
	fs.BoolVar(&showVer, "version", false, "print version and exit")

	err := fs.Parse(args)
	if err != nil {
		return 1
	}

	if showVer {
		_, _ = fmt.Fprintf(stdout, "filofix version %s\n", version)
		return 0
	}

	paths := fs.Args()

	// No paths: read from stdin
	if len(paths) == 0 {
		data, err := io.ReadAll(stdin)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error reading stdin: %v\n", err)
			return 1
		}
		fixed, _ := Fix(string(data))
		_, err = fmt.Fprint(stdout, fixed)
		if err != nil {
			return 1
		}
		return 0
	}

	exitCode := 0
	for _, path := range paths {
		err := processPath(path, writeFlag, listFlag, diffFlag, stdout)
		if err != nil {
			_, _ = fmt.Fprintf(stderr, "error: %v\n", err)
			exitCode = 1
		}
	}
	return exitCode
}

func processPath(path string, write, list, diff bool, stdout io.Writer) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				return nil
			}
			if !strings.HasSuffix(p, ".filo") {
				return nil
			}
			return processFile(p, write, list, diff, stdout)
		})
	}

	return processFile(path, write, list, diff, stdout)
}

func processFile(path string, write, list, diff bool, stdout io.Writer) error {
	// #nosec G304 -- filofix intentionally reads paths selected by the user.
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	original := string(data)
	fixed, changed := Fix(original)

	if !changed {
		if !write && !list && !diff {
			_, err = fmt.Fprint(stdout, original)
			return err
		}
		return nil
	}

	if list {
		_, err = fmt.Fprintln(stdout, path)
		return err
	}

	if diff {
		_, _ = fmt.Fprintf(stdout, "--- %s (original)\n", path)
		_, _ = fmt.Fprintf(stdout, "+++ %s (fixed)\n", path)
		printSimpleDiff(stdout, original, fixed)
		return nil
	}

	if write {
		// #nosec G703 -- filofix intentionally rewrites paths selected by the user.
		return os.WriteFile(path, []byte(fixed), 0600)
	}

	_, err = fmt.Fprint(stdout, fixed)
	return err
}

func printSimpleDiff(w io.Writer, original, fixed string) {
	origLines := strings.Split(original, "\n")
	fixedLines := strings.Split(fixed, "\n")

	maxLines := max(len(fixedLines), len(origLines))
	for i := range maxLines {
		var origLine, fixedLine string
		if i < len(origLines) {
			origLine = origLines[i]
		}
		if i < len(fixedLines) {
			fixedLine = fixedLines[i]
		}
		if origLine != fixedLine {
			if origLine != "" {
				_, _ = fmt.Fprintf(w, "-%s\n", origLine)
			}
			if fixedLine != "" {
				_, _ = fmt.Fprintf(w, "+%s\n", fixedLine)
			}
		}
	}
}

// Fix applies every filofix rewrite until the source stops changing, and
// reports whether anything changed.
func Fix(src string) (string, bool) {
	changed := false
	// Each fix can expose work for the other (unwrapping can reveal foldable
	// forms), so iterate to a fixed point with a hard cap as a safety net.
	for range 32 {
		out, c1 := unwrapRootLet(src)
		out, c2 := foldConstSpans(out)
		if !c1 && !c2 {
			break
		}
		changed = true
		src = out
	}
	return src, changed
}

// span is a balanced (...) group in the source, as byte offsets. start is the
// '(' and end the matching ')' (inclusive). depth 0 is top level.
type span struct {
	start, end, depth int
}

// scanGroups walks the source respecting strings, escapes, and ; comments, and
// returns every balanced group. A source with unbalanced parens returns nil —
// filofix leaves files it cannot understand alone.
func scanGroups(src string) []span {
	var groups []span
	var stack []int
	inString := false
	inComment := false
	escape := false

	for i := 0; i < len(src); i++ {
		c := src[i]
		if inComment {
			if c == '\n' {
				inComment = false
			}
			continue
		}
		if escape {
			escape = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case ';':
			inComment = true
		case '(':
			stack = append(stack, i)
		case ')':
			if len(stack) == 0 {
				return nil // unbalanced
			}
			start := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			groups = append(groups, span{start: start, end: i, depth: len(stack)})
		}
	}
	if len(stack) != 0 || inString {
		return nil // unbalanced or unterminated
	}
	return groups
}

// bareText reports whether the source outside groups and comments contains
// anything but whitespace between from and to (e.g. a top-level bare symbol).
func bareText(src string, from, to int, groups []span) bool {
	covered := func(i int) bool {
		for _, g := range groups {
			if g.depth == 0 && i >= g.start && i <= g.end {
				return true
			}
		}
		return false
	}
	inComment := false
	for i := from; i < to; i++ {
		c := src[i]
		if inComment {
			if c == '\n' {
				inComment = false
			}
			continue
		}
		if covered(i) {
			continue
		}
		switch c {
		case ' ', '\t', '\n', '\r':
			continue
		case ';':
			inComment = true
			continue
		}
		return true
	}
	return false
}

// unwrapRootLet removes a legacy `(let () ...)` wrapper when it is the only
// top-level form: the interpreter wraps multiple top-level forms implicitly,
// so the explicit wrapper is dead weight. Comments outside the wrapper and the
// body's own formatting are preserved; body lines lose the wrapper's one level
// of indentation.
func unwrapRootLet(src string) (string, bool) {
	groups := scanGroups(src)
	if groups == nil {
		return src, false
	}

	var top []span
	for _, g := range groups {
		if g.depth == 0 {
			top = append(top, g)
		}
	}
	if len(top) != 1 {
		return src, false
	}
	g := top[0]
	if bareText(src, 0, len(src), groups) {
		return src, false
	}

	inner := src[g.start+1 : g.end]
	rest := strings.TrimLeft(inner, " \t\r\n")
	if !strings.HasPrefix(rest, "let") {
		return src, false
	}
	rest = rest[len("let"):]
	if rest == "" || !isTokenBoundary(rest[0]) {
		return src, false
	}
	rest = strings.TrimLeft(rest, " \t\r\n")
	// The bindings group must be empty: (let () ...). Anything else is a real
	// let and stays.
	if !strings.HasPrefix(rest, "(") {
		return src, false
	}
	closeIdx := strings.IndexByte(rest, ')')
	if closeIdx < 0 || strings.TrimSpace(rest[1:closeIdx]) != "" {
		return src, false
	}
	body := rest[closeIdx+1:]
	if strings.TrimSpace(body) == "" {
		return src, false
	}

	// Re-indent: body lines carry the wrapper's indentation; strip the common
	// prefix of the full lines so the forms land at column 0, keeping their
	// relative structure. A fragment on the wrapper's own line starts a line.
	var out strings.Builder
	out.WriteString(src[:g.start])

	lines := strings.Split(body, "\n")
	fullLines := lines
	if len(lines) > 0 && strings.TrimSpace(lines[0]) != "" {
		fullLines = lines[1:]
	}
	indent := commonIndent(fullLines)

	first := strings.TrimSpace(lines[0])
	if first != "" {
		out.WriteString(first)
		if len(lines) > 1 {
			out.WriteString("\n")
		}
	}
	for i, line := range lines[1:] {
		out.WriteString(strings.TrimPrefix(line, indent))
		if i < len(lines)-2 {
			out.WriteString("\n")
		}
	}
	out.WriteString(src[g.end+1:])

	res := out.String()
	if !strings.HasSuffix(res, "\n") {
		res += "\n"
	}
	return res, true
}

func isTokenBoundary(c byte) bool {
	switch c {
	case ' ', '\t', '\n', '\r', '(', ')', ';', '"':
		return true
	}
	return false
}

// commonIndent returns the longest whitespace prefix shared by every non-empty
// line.
func commonIndent(lines []string) string {
	indent := ""
	first := true
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		ws := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		if first {
			indent = ws
			first = false
			continue
		}
		for !strings.HasPrefix(ws, indent) {
			indent = indent[:len(indent)-1]
		}
	}
	return indent
}

// foldConstSpans replaces constant subexpressions with their folded value, one
// splice at a time. A span is only touched when its text holds no comment,
// parses on its own, and folds all the way down to a single atom — the same
// semantics-preserving fold the engine applies at compile time, made visible
// in the source.
func foldConstSpans(src string) (string, bool) {
	for {
		groups := scanGroups(src)
		if groups == nil {
			return src, false
		}

		spliced := false
		for _, g := range groups {
			text := src[g.start : g.end+1]
			if strings.ContainsRune(text, ';') {
				continue // fold must not eat a comment
			}
			ast, err := filo.Parse(text)
			if err != nil {
				continue
			}
			folded, changed := filo.FoldConstants(ast)
			if !changed || !isAtomNode(folded) {
				continue
			}
			replacement, err := filo.FormatAST(folded, filo.FormatConfig{})
			if err != nil {
				continue
			}
			replacement = strings.TrimSpace(replacement)
			src = src[:g.start] + replacement + src[g.end+1:]
			spliced = true
			break // offsets shifted: rescan
		}
		if !spliced {
			return src, false
		}

		// Keep splicing until a full pass finds nothing; report change.
		out, _ := foldConstSpans(src)
		return out, true
	}
}

// isAtomNode reports whether the folded node is a single literal — the only
// shape foldConstSpans splices into the source.
func isAtomNode(n filo.Node) bool {
	switch n.(type) {
	case *filo.NumberLit, *filo.BoolLit, *filo.StringLit:
		return true
	}
	return false
}
