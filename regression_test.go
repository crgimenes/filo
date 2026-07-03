package filo

import (
	"context"
	"strings"
	"testing"
)

// eval runs a script through the public Engine path with default limits and
// returns the resulting value, failing the test on any error.
func eval(t *testing.T, src string) Value {
	t.Helper()
	eng := NewEngine()
	v, _, err := eng.RunScript(context.Background(), src, nil, EvalConfig{})
	if err != nil {
		t.Fatalf("RunScript(%q) error: %v", src, err)
	}
	return v
}

// TestParseDepthLimit guards the parser against unbounded recursion: a
// pathological stream of open parens must return a ParseError, not crash the
// host with an unrecoverable stack overflow.
func TestParseDepthLimit(t *testing.T) {
	src := strings.Repeat("(", maxParseDepth+50)
	_, err := Parse(src)
	if err == nil {
		t.Fatal("expected a parse error for over-deep nesting, got nil")
	}
	if !strings.Contains(err.Error(), "nesting too deep") {
		t.Fatalf("expected a nesting-depth error, got: %v", err)
	}

	// A nesting depth comfortably under the limit must still parse.
	ok := strings.Repeat("(list ", 100) + "1" + strings.Repeat(")", 100)
	_, err = Parse(ok)
	if err != nil {
		t.Fatalf("moderate nesting should parse, got: %v", err)
	}
}

// TestParseStripsBOM verifies a leading UTF-8 BOM is stripped before tokenizing,
// so a file saved with a BOM still parses.
func TestParseStripsBOM(t *testing.T) {
	const bom = "\ufeff"
	v := eval(t, bom+"(+ 1 2)")
	n, err := v.AsNumber()
	if err != nil || n != 3 {
		t.Fatalf("BOM-prefixed script = %v (err %v), want 3", n, err)
	}
}

// TestBoolLiteralBoundary verifies #t / #f are only recognized as the exact
// two-character literals; #true must not be read as #t followed by a symbol.
func TestBoolLiteralBoundary(t *testing.T) {
	b, err := eval(t, "#t").AsBool()
	if err != nil || !b {
		t.Fatalf("#t should be true, got %v (err %v)", b, err)
	}
	n, err := eval(t, "(if #f 1 2)").AsNumber()
	if err != nil || n != 2 {
		t.Fatalf("(if #f 1 2) should be 2, got %v (err %v)", n, err)
	}
	eng := NewEngine()
	_, _, err = eng.RunScript(context.Background(), "#true", nil, EvalConfig{})
	if err == nil {
		t.Fatal("#true must be a parse/eval error, not silently #t + symbol")
	}
}

// TestLetvNoTupleAliasing verifies that mutating a letv binding does not corrupt
// the source tuple it was destructured from.
func TestLetvNoTupleAliasing(t *testing.T) {
	src := `(let ((tup (values 5 6)))
		(letv (a b) tup (set a 99))
		(letv (c d) tup c))`
	v := eval(t, src)
	n, err := v.AsNumber()
	if err != nil || n != 5 {
		t.Fatalf("source tuple was mutated through letv: got %v (err %v), want 5", n, err)
	}
}

// TestFoldIfNoElseMatchesInterpreter verifies constant-folding (if #f X) yields
// the same empty-list value the interpreter produces, rather than an
// empty-list node that errors at runtime.
func TestFoldIfNoElseMatchesInterpreter(t *testing.T) {
	ast, err := Parse("(if #f 42)")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	folded, changed := FoldConstants(ast)
	if !changed {
		t.Fatal("expected (if #f 42) to fold")
	}

	eng := NewEngine()
	prog, err := eng.CompileAST(folded)
	if err != nil {
		t.Fatalf("compile folded: %v", err)
	}
	v, _, err := prog.Execute(context.Background(), nil, EvalConfig{})
	if err != nil {
		t.Fatalf("folded (if #f 42) must evaluate cleanly, got: %v", err)
	}
	list, err := v.AsList()
	if err != nil || len(list) != 0 {
		t.Fatalf("folded (if #f 42) = %v (err %v), want empty list", v, err)
	}
}
