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
	cfg := filo.EvalConfig{StepLimit: 128, RecursionLimit: 32, Timeout: 2 * time.Second}

	globals := map[string]filo.Value{
		"age": filo.VNum(17),
	}

	script := "(if (< age 18) \"minor\" \"adult\")"
	result, _, err := eng.RunScript(ctx, script, globals, cfg)
	if err != nil {
		panic(err)
	}

	status, err := result.AsString()
	if err != nil {
		panic(err)
	}

	fmt.Printf("Age %s -> %s\n", globals["age"], status)
}

// Output: Age 17 -> minor
