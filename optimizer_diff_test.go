package filo

import (
	"context"
	"testing"
)

// TestFoldingMatchesInterpreter runs a battery of scripts through the folding
// compile path (Engine.Compile) and through the raw interpreter (ExecuteAST on
// the unfolded parse tree), and requires identical outcomes. This is the
// contract that lets constant folding stay wired into Compile with no off
// switch: optimized and unoptimized execution must never diverge.
func TestFoldingMatchesInterpreter(t *testing.T) {
	scripts := []string{
		"(+ 1 2)",
		"(* (+ 1 2) (- 10 4))",
		"(if #t 1 2)",
		"(if #f 1 2)",
		"(if #f 42)",
		"(if (> 3 2) \"yes\" \"no\")",
		"(let ((x (+ 1 2))) (* x x))",
		"(def f (fn (n) (+ n (* 2 3)))) (f 4)",
		"(and #t #t)",
		"(or #f #t)",
		"(not #f)",
		"(pow 2 8)",
		"(str-id \"a\")",         // non-pure unknown builtin: must not fold
		"(set x (+ 40 2)) x",     // fold inside set
		"(if #f 1)",              // no-else false: empty list on both paths
		"(list (+ 1 1) (+ 2 2))", // list is not folded, args are
	}

	eng := NewEngine()
	err := eng.RegisterBuiltin("str-id", func(_ context.Context, args []Value) (Value, error) {
		return args[0], nil
	})
	if err != nil {
		t.Fatalf("register str-id: %v", err)
	}

	for _, src := range scripts {
		ast, err := Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}

		// Raw interpreter path: compile WITHOUT folding.
		compiled, err := Compile(ast, eng.builtins, eng.symbols)
		if err != nil {
			t.Fatalf("compile %q: %v", src, err)
		}
		rawVal, _, rawErr := eng.ExecuteAST(context.Background(), compiled, nil, EvalConfig{})

		// Folding path: the public Compile applies FoldConstants.
		prog, err := eng.Compile(src)
		if err != nil {
			t.Fatalf("Engine.Compile %q: %v", src, err)
		}
		foldVal, _, foldErr := prog.Execute(context.Background(), nil, EvalConfig{})

		if (rawErr == nil) != (foldErr == nil) {
			t.Errorf("%q: error divergence: raw=%v folded=%v", src, rawErr, foldErr)
			continue
		}
		if rawErr != nil {
			continue
		}
		if !valueEqual(rawVal, foldVal) {
			t.Errorf("%q: value divergence: raw=%v folded=%v", src, rawVal, foldVal)
		}
	}
}
