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
	filomath.RegisterMathBuiltins(eng)

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

	list, _ := result.AsList()
	floatVal, _ := list[0].AsNumber()
	intVal, _ := list[1].AsNumber() // to-int returns a number (float64) holding an integer

	fmt.Printf("Sqrt(17): %f\n", floatVal)
	fmt.Printf("Truncated: %.0f\n", intVal)
}

// Output:
// Sqrt(17): 4.123106...
// Truncated: 4
