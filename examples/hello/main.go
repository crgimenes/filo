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
	cfg := filo.EvalConfig{StepLimit: 64, RecursionLimit: 16, Timeout: time.Second}

	result, _, err := eng.RunScript(ctx, "\"Hello, World\"", nil, cfg)
	if err != nil {
		panic(err)
	}

	message, err := result.AsString()
	if err != nil {
		panic(err)
	}

	fmt.Println(message)
}

// Output: Hello, World
