package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filomath"
)

func main() {
	eng := filo.NewEngine()
	filomath.RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 64, RecursionLimit: 8, Timeout: time.Second}

	// Uses sqrt and to-int
	script := `
(let ((root (sqrt 17)))
  (list root (to-int root)))
`

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	list, err := result.AsList()
	if err != nil {
		panic(err)
	}
	floatVal, err := list[0].AsNumber()
	if err != nil {
		panic(err)
	}
	intVal, err := list[1].AsNumber()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Sqrt(17): %f\n", floatVal)
	fmt.Printf("Truncated: %.0f\n", intVal)
}

// Output:
// Sqrt(17): 4.123106
// Truncated: 4
