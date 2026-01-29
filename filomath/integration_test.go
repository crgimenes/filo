package filomath

import (
	"context"
	"testing"
	"time"

	"github.com/crgimenes/filo"
)

// TestMultiplePackagesIntegration demonstrates loading multiple Filo extension
// packages on the same engine instance. This is the canonical pattern for
// combining builtins from different packages.
func TestMultiplePackagesIntegration(t *testing.T) {
	t.Parallel()

	// Create a single Filo engine
	eng := filo.NewEngine()

	// Register multiple extension packages on the same engine
	// Each package adds its builtins to the same builtins map
	RegisterBuiltins(eng)
	// In a real scenario, you would also call:
	// Load multiple extension packages on the same engine
	// filostrings.RegisterStringBuiltins(eng)
	// etc.

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 1000, RecursionLimit: 32, Timeout: 5 * time.Second}

	// Test that both core builtins and extension builtins work together
	script := `
		(let ((base 16)
		      (root (sqrt base))
		      (doubled (* root 2)))
		  (+ doubled (abs -10)))
	`
	// sqrt(16) = 4, * 2 = 8, + abs(-10) = 8 + 10 = 18

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		t.Fatalf("RunScript error: %v", err)
	}

	num, err := result.AsNumber()
	if err != nil {
		t.Fatalf("expected number: %v", err)
	}

	if num != 18 {
		t.Errorf("expected 18, got %v", num)
	}
}

// TestBuiltinNamespacing shows how to avoid naming conflicts between packages
// by using prefixed builtin names.
func TestBuiltinNamespacing(t *testing.T) {
	t.Parallel()

	eng := filo.NewEngine()
	RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 1000, RecursionLimit: 32, Timeout: 5 * time.Second}

	// math-min and math-max are prefixed to avoid potential conflicts with other packages
	script := `(math-min (math-max 1 2 3) 5 6)`
	// math-max(1,2,3) = 3, math-min(3, 5, 6) = 3

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		t.Fatalf("RunScript error: %v", err)
	}

	num, _ := result.AsNumber()
	if num != 3 {
		t.Errorf("expected 3, got %v", num)
	}
}
