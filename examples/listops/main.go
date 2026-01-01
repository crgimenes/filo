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
	cfg := filo.EvalConfig{StepLimit: 256, RecursionLimit: 32, Timeout: 2 * time.Second}

	script := "(fold (fn (acc x) (+ acc (* x x))) 0 (list 1 2 3 4))"
	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	sumOfSquares, err := result.AsNumber()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Sum of squares: %.0f\n", sumOfSquares)
}

// Output: Sum of squares: 30
