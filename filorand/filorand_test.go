package filorand

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/crgimenes/filo"
)

func TestRandomBuiltins(t *testing.T) {
	eng := filo.NewEngine()
	RegisterBuiltins(eng)
	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 1000, Timeout: time.Second}

	t.Run("rand-float", func(t *testing.T) {
		val, _, err := eng.RunScript(ctx, "(rand-float)", nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		num, err := val.AsNumber()
		if err != nil {
			t.Fatalf("expected number: %v", err)
		}
		if num < 0 || num >= 1.0 {
			t.Errorf("rand-float out of range: %v", num)
		}
	})

	t.Run("rand-int", func(t *testing.T) {
		val, _, err := eng.RunScript(ctx, "(rand-int 10)", nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		num, err := val.AsNumber()
		if err != nil {
			t.Fatalf("expected number: %v", err)
		}
		if num < 0 || num >= 10 {
			t.Errorf("rand-int out of range: %v", num)
		}
		// Check integer strictness if possible, but it's a float
		if num != float64(int64(num)) {
			t.Errorf("rand-int returned non-integer float: %v", num)
		}
	})

	t.Run("uuid-v4", func(t *testing.T) {
		val, _, err := eng.RunScript(ctx, "(uuid-v4)", nil, cfg)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		id, err := val.AsString()
		if err != nil {
			t.Fatalf("expected string: %v", err)
		}
		if len(id) != 36 {
			t.Errorf("uuid length mismatch: %d != 36", len(id))
		}
		parts := strings.Split(id, "-")
		if len(parts) != 5 {
			t.Errorf("uuid format invalid parts count: %d", len(parts))
		}
	})
}
