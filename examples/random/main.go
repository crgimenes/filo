package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filorand"
)

func main() {
	eng := filo.NewEngine()
	filorand.RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 64, RecursionLimit: 8, Timeout: time.Second}

	// Generate a float, and an integer ID
	script := `
(list (rand-float) (rand-int 100) (uuid-v4))
`
	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	list, err := result.AsList()
	if err != nil {
		panic(err)
	}
	fVal, err := list[0].AsNumber()
	if err != nil {
		panic(err)
	}
	iVal, err := list[1].AsNumber()
	if err != nil {
		panic(err)
	}
	uuid, err := list[2].AsString()
	if err != nil {
		panic(err)
	}
	if fVal < 0 || fVal >= 1 {
		panic(fmt.Errorf("rand-float returned %v outside [0, 1)", fVal))
	}
	if iVal < 0 || iVal >= 100 {
		panic(fmt.Errorf("rand-int returned %v outside [0, 100)", iVal))
	}
	if len(uuid) != 36 {
		panic(fmt.Errorf("uuid-v4 returned length %d, want 36", len(uuid)))
	}

	fmt.Printf("Random float: %.4f (is between 0 and 1: %v)\n", fVal, fVal >= 0 && fVal < 1)
	fmt.Printf("Random int: %.0f\n", iVal)
	fmt.Printf("UUID: %s (len: %d)\n", uuid, len(uuid))
}
