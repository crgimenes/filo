package filomath

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/crgimenes/filo"
)

func TestMathBuiltins(t *testing.T) {
	t.Parallel()

	eng := filo.NewEngine()
	RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 1000, RecursionLimit: 32, Timeout: 5 * time.Second}

	tests := []struct {
		name     string
		script   string
		expected float64
		epsilon  float64 // for floating point comparison
	}{
		{"abs positive", "(abs 5)", 5, 0},
		{"abs negative", "(abs -5)", 5, 0},
		{"sqrt", "(sqrt 16)", 4, 0},
		{"floor", "(floor 3.7)", 3, 0},
		{"ceil", "(ceil 3.2)", 4, 0},
		{"round down", "(round 3.4)", 3, 0},
		{"round up", "(round 3.6)", 4, 0},
		{"sin 0", "(sin 0)", 0, 1e-10},
		{"cos 0", "(cos 0)", 1, 1e-10},
		{"pi", "(pi)", math.Pi, 0},
		{"e", "(e)", math.E, 0},
		{"log e", "(log (e))", 1, 1e-10},
		{"log10 100", "(log10 100)", 2, 1e-10},
		{"exp 0", "(exp 0)", 1, 0},
		{"exp 1", "(exp 1)", math.E, 1e-10},
		{"math-min", "(math-min 5 3 8 1 4)", 1, 0},
		{"math-min", "(math-min 5 3 8 1 4)", 1, 0},
		{"math-max", "(math-max 5 3 8 1 4)", 8, 0},
		{"to-int basic", "(to-int 3.9)", 3, 0},
		{"to-int negative", "(to-int -3.9)", -3, 0},
		{"to-int exact", "(to-int 4.0)", 4, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _, err := eng.RunScript(ctx, tt.script, nil, cfg)
			if err != nil {
				t.Fatalf("RunScript error: %v", err)
			}

			num, err := result.AsNumber()
			if err != nil {
				t.Fatalf("expected number: %v", err)
			}

			diff := math.Abs(num - tt.expected)
			if tt.epsilon > 0 {
				if diff > tt.epsilon {
					t.Errorf("expected %.10f, got %.10f (diff %.10f > epsilon %.10f)",
						tt.expected, num, diff, tt.epsilon)
				}
			} else {
				if num != tt.expected {
					t.Errorf("expected %v, got %v", tt.expected, num)
				}
			}
		})
	}
}

func TestMathErrors(t *testing.T) {
	t.Parallel()

	eng := filo.NewEngine()
	RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 1000, RecursionLimit: 32, Timeout: 5 * time.Second}

	tests := []struct {
		name   string
		script string
	}{
		{"sqrt negative", "(sqrt -1)"},
		{"log zero", "(log 0)"},
		{"log negative", "(log -1)"},
		{"log10 zero", "(log10 0)"},
		{"abs no args", "(abs)"},
		{"sqrt two args", "(sqrt 4 5)"},
		{"pi with args", "(pi 1)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, _, err := eng.RunScript(ctx, tt.script, nil, cfg)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}
