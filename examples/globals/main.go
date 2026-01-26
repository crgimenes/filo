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
	cfg := filo.EvalConfig{StepLimit: 128, RecursionLimit: 16, Timeout: time.Second}

	globals := map[string]filo.Value{
		"strength": filo.VNum(14),
		"bonus":    filo.VNum(3),
	}

	script := "(+ (* strength 2) bonus)"
	result, updatedGlobals, err := eng.RunScript(ctx, script, globals, cfg)
	if err != nil {
		panic(err)
	}

	total, err := result.AsNumber()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Total attack: %.0f\n", total)
	fmt.Printf("Globals kept strength=%s and bonus=%s\n", updatedGlobals["strength"], updatedGlobals["bonus"])
}

// Output:
// Total attack: 31
// Globals kept strength=14 and bonus=3
