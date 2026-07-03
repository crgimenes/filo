package filo

import (
	"context"
	"fmt"
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

// TestAndOrShortCircuit verifies and/or stop evaluating at the first deciding
// argument: the (/ 1 0) in the tail must never run.
func TestAndOrShortCircuit(t *testing.T) {
	b, err := eval(t, "(or #t (/ 1 0))").AsBool()
	if err != nil || !b {
		t.Fatalf("(or #t (/ 1 0)) = %v (err %v), want #t without evaluating the division", b, err)
	}
	b, err = eval(t, "(and #f (/ 1 0))").AsBool()
	if err != nil || b {
		t.Fatalf("(and #f (/ 1 0)) = %v (err %v), want #f without evaluating the division", b, err)
	}

	// Arguments that DO get evaluated still enforce bool typing and propagate
	// their errors.
	eng := NewEngine()
	_, _, rerr := eng.RunScript(context.Background(), "(and #t (/ 1 0))", nil, EvalConfig{})
	if rerr == nil {
		t.Fatal("(and #t (/ 1 0)) must fail: the second argument is evaluated")
	}
	_, _, rerr = eng.RunScript(context.Background(), "(or #f 42)", nil, EvalConfig{})
	if rerr == nil {
		t.Fatal("(or #f 42) must fail: evaluated arguments must be bool")
	}
}

// TestStringNumberCasts covers the explicit coercion builtins used to compare
// across kinds: (= (string 1) "1") and (= (number "1") 1).
func TestStringNumberCasts(t *testing.T) {
	tests := []struct {
		src  string
		want string
	}{
		{`(string 42)`, "42"},
		{`(string 1.5)`, "1.5"},
		{`(string #t)`, "#t"},
		{`(string "already")`, "already"},
		{`(string (list 1 2))`, "(list 1 2)"},
	}
	for _, tc := range tests {
		s, err := eval(t, tc.src).AsString()
		if err != nil || s != tc.want {
			t.Errorf("%s = %q (err %v), want %q", tc.src, s, err, tc.want)
		}
	}

	n, err := eval(t, `(number "1.5")`).AsNumber()
	if err != nil || n != 1.5 {
		t.Fatalf(`(number "1.5") = %v (err %v), want 1.5`, n, err)
	}
	n, err = eval(t, `(number " 42 ")`).AsNumber()
	if err != nil || n != 42 {
		t.Fatalf(`(number " 42 ") = %v (err %v), want 42 (whitespace trimmed)`, n, err)
	}

	// The cross-kind comparison idiom the casts exist for.
	b, err := eval(t, `(= (string 1) "1")`).AsBool()
	if err != nil || !b {
		t.Fatalf(`(= (string 1) "1") = %v (err %v), want #t`, b, err)
	}
	b, err = eval(t, `(= (number "2") 2)`).AsBool()
	if err != nil || !b {
		t.Fatalf(`(= (number "2") 2) = %v (err %v), want #t`, b, err)
	}

	// A failed numeric parse is a script error, same as any other builtin error.
	eng := NewEngine()
	_, _, rerr := eng.RunScript(context.Background(), `(number "abc")`, nil, EvalConfig{})
	if rerr == nil {
		t.Fatal(`(number "abc") must error`)
	}
	_, _, rerr = eng.RunScript(context.Background(), `(number #t)`, nil, EvalConfig{})
	if rerr == nil {
		t.Fatal(`(number #t) must error: booleans do not coerce implicitly`)
	}
}

// TestModuloFloored fixes the floored (Lua-compatible) semantics of %: the
// result takes the divisor's sign. Decided 2026-07-03 after the Prolog
// conformance spec surfaced the divergence (Go's math.Mod truncates).
func TestModuloFloored(t *testing.T) {
	tests := []struct {
		src  string
		want float64
	}{
		{"(% 10 3)", 1},
		{"(% -1 2)", 1},
		{"(% 1 -2)", -1},
		{"(% -1 -2)", -1},
		{"(% -7 3)", 2},
		{"(% 7 -3)", -2},
		{"(% 6 3)", 0},
		{"(% -6 3)", 0},
		{"(% 5.5 2)", 1.5},
		{"(% -5.5 2)", 0.5},
	}
	for _, tc := range tests {
		n, err := eval(t, tc.src).AsNumber()
		if err != nil || n != tc.want {
			t.Errorf("%s = %v (err %v), want %v", tc.src, n, err, tc.want)
		}
	}

	eng := NewEngine()
	_, _, err := eng.RunScript(context.Background(), "(% 1 0)", nil, EvalConfig{})
	if err == nil {
		t.Fatal("(% 1 0) must error")
	}
}

// TestCond covers the cond special form: ordered clauses, else, no-match, the
// bool-test requirement, and that clauses after the first match are not
// evaluated (so an erroring later clause is never reached).
func TestCond(t *testing.T) {
	num := func(src string) float64 {
		t.Helper()
		n, err := eval(t, src).AsNumber()
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		return n
	}

	if got := num(`(cond ((< 1 0) 10) ((> 1 0) 20) (else 30))`); got != 20 {
		t.Errorf("first matching clause = %v, want 20", got)
	}
	if got := num(`(cond ((< 1 0) 10) (else 30))`); got != 30 {
		t.Errorf("else clause = %v, want 30", got)
	}
	if got := num(`(cond (#t 1 2 3))`); got != 3 {
		t.Errorf("implicit-do body = %v, want 3 (last expr)", got)
	}

	// No clause matches and no else: empty list.
	l, err := eval(t, `(cond ((< 1 0) 1) ((> 0 1) 2))`).AsList()
	if err != nil || len(l) != 0 {
		t.Fatalf("no-match cond = %v (err %v), want empty list", l, err)
	}

	// First-match short-circuit: the second clause's test would error, but it
	// is never evaluated because the first clause matches.
	if got := num(`(cond (#t 42) ((/ 1 0) 0))`); got != 42 {
		t.Errorf("short-circuit = %v, want 42 (later clause not evaluated)", got)
	}

	// A non-bool test is an error.
	eng := NewEngine()
	_, _, rerr := eng.RunScript(context.Background(), `(cond (5 1))`, nil, EvalConfig{})
	if rerr == nil {
		t.Fatal("(cond (5 1)) must error: a test must be a bool")
	}
}

// TestListBuiltins covers filter, range, and reverse.
func TestListBuiltins(t *testing.T) {
	render := func(src string) string {
		t.Helper()
		l, err := eval(t, src).AsList()
		if err != nil {
			t.Fatalf("%s: %v", src, err)
		}
		parts := make([]string, len(l))
		for i, v := range l {
			n, _ := v.AsNumber()
			parts[i] = fmt.Sprintf("%g", n)
		}
		return strings.Join(parts, ",")
	}

	if got := render(`(range 5)`); got != "0,1,2,3,4" {
		t.Errorf("(range 5) = %q", got)
	}
	if got := render(`(range 2 5)`); got != "2,3,4" {
		t.Errorf("(range 2 5) = %q", got)
	}
	if got := render(`(range 0)`); got != "" {
		t.Errorf("(range 0) = %q, want empty", got)
	}
	if got := render(`(range 5 2)`); got != "" {
		t.Errorf("(range 5 2) = %q, want empty (non-increasing)", got)
	}
	if got := render(`(reverse (list 1 2 3 4))`); got != "4,3,2,1" {
		t.Errorf("reverse = %q", got)
	}
	if got := render(`(filter (fn (x) (> x 2)) (range 6))`); got != "3,4,5" {
		t.Errorf("filter = %q", got)
	}

	// filter predicate must return a bool.
	eng := NewEngine()
	_, _, rerr := eng.RunScript(context.Background(), `(filter (fn (x) x) (list 1 2))`, nil, EvalConfig{})
	if rerr == nil {
		t.Fatal("filter with non-bool predicate must error")
	}
}

// TestErrorBuiltin verifies (error msg) raises the message as a script error.
func TestErrorBuiltin(t *testing.T) {
	eng := NewEngine()
	_, _, err := eng.RunScript(context.Background(), `(error "age must be non-negative")`, nil, EvalConfig{})
	if err == nil {
		t.Fatal("(error ...) must produce an error")
	}
	if !strings.Contains(err.Error(), "age must be non-negative") {
		t.Fatalf("error message not propagated: %v", err)
	}

	// The message must be a string.
	_, _, err = eng.RunScript(context.Background(), `(error 42)`, nil, EvalConfig{})
	if err == nil {
		t.Fatal("(error 42) must error: message must be a string")
	}
}
