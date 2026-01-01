package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
)

func main() {
	eng := filo.NewEngine()
	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 64, RecursionLimit: 8, Timeout: time.Second}

	// 'do' evaluates expressions in order and returns the last result.
	// Useful for grouping side effects (like 'set').
	script := `
(let ((x 10) (y 20))
  (do
    (set x (* x 2)) ; x becomes 20
    (set y (+ y 1)) ; y becomes 21
    (+ x y)))       ; returns 41
`

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	num, err := result.AsNumber()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Result: %.0f\n", num)
}

// Output: Result: 41
