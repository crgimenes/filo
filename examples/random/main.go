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
	filorand.RegisterRandomBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 64, RecursionLimit: 8, Timeout: time.Second}

	// Generate a float, and an integer ID
	script := `
(list (rand-float) (rand-int 100) (uuid-v4))
`
	// Seed for partial reproducibility in this run if we wanted,
	// but here we just show it running.
	eng.RunScript(ctx, "(rand-seed 1234)", nil, cfg)

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	list, _ := result.AsList()
	fVal, _ := list[0].AsNumber()
	iVal, _ := list[1].AsNumber()
	uuid, _ := list[2].AsString()

	fmt.Printf("Random float: %.4f (is between 0 and 1: %v)\n", fVal, fVal >= 0 && fVal < 1)
	fmt.Printf("Random int: %.0f\n", iVal)
	fmt.Printf("UUID: %s (len: %d)\n", uuid, len(uuid))
}
