package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
)

func greet(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("greet expects 1 argument")
	}

	name, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, err
	}

	return filo.VString("Hello, " + name + "!"), nil
}

func main() {
	eng := filo.NewEngine()
	eng.MustRegisterBuiltin("greet", greet)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 64, RecursionLimit: 8, Timeout: time.Second}

	result, _, err := eng.RunScript(ctx, "(greet \"Ada\")", nil, cfg)
	if err != nil {
		panic(err)
	}

	message, err := result.AsString()
	if err != nil {
		panic(err)
	}

	fmt.Println(message)
}

// Output: Hello, Ada!
