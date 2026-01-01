package filo

import (
	"context"
	"testing"
	"time"
)

// TestMultipleTopLevelExpressions tests that Filo can execute multiple
// expressions in sequence and returns the value of the last one.
func TestMultipleTopLevelExpressions(t *testing.T) {
	eng := NewEngine()
	ctx := context.Background()
	cfg := EvalConfig{StepLimit: 1000, RecursionLimit: 32, Timeout: time.Second}

	// Test 1: Two simple arithmetic expressions
	t.Run("two_expressions", func(t *testing.T) {
		script := `(+ 1 1)
(+ 2 2)`
		result, _, err := eng.RunScript(ctx, script, nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		num, err := result.AsNumber()
		if err != nil {
			t.Fatalf("expected number: %v", err)
		}
		if num != 4 {
			t.Fatalf("expected 4 (result of last expression), got %v", num)
		}
	})

	// Test 2: Multiple expressions with set
	t.Run("with_set_commands", func(t *testing.T) {
		script := `(set x 10)
(set y 20)
(+ x y)`
		result, globals, err := eng.RunScript(ctx, script, nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		num, err := result.AsNumber()
		if err != nil {
			t.Fatalf("expected number: %v", err)
		}
		if num != 30 {
			t.Fatalf("expected 30, got %v", num)
		}

		// Verify globals were set
		x, ok := globals["x"]
		if !ok {
			t.Fatal("x should be in globals")
		}
		xNum, _ := x.AsNumber()
		if xNum != 10 {
			t.Fatalf("x should be 10, got %v", xNum)
		}
	})

	// Test 3: Three expressions
	t.Run("three_expressions", func(t *testing.T) {
		script := `(+ 1 1)
(* 3 3)
(- 10 5)`
		result, _, err := eng.RunScript(ctx, script, nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		num, err := result.AsNumber()
		if err != nil {
			t.Fatalf("expected number: %v", err)
		}
		if num != 5 {
			t.Fatalf("expected 5 (result of last expression), got %v", num)
		}
	})

	// Test 4: Expression with comments between
	t.Run("with_comments", func(t *testing.T) {
		script := `;; First calculation
(+ 1 2)
;; Second calculation
(* 4 5)`
		result, _, err := eng.RunScript(ctx, script, nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		num, err := result.AsNumber()
		if err != nil {
			t.Fatalf("expected number: %v", err)
		}
		if num != 20 {
			t.Fatalf("expected 20 (result of last expression), got %v", num)
		}
	})

	// Test 5: Single expression should still work
	t.Run("single_expression", func(t *testing.T) {
		script := `(+ 5 5)`
		result, _, err := eng.RunScript(ctx, script, nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		num, err := result.AsNumber()
		if err != nil {
			t.Fatalf("expected number: %v", err)
		}
		if num != 10 {
			t.Fatalf("expected 10, got %v", num)
		}
	})
}
