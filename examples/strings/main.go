package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filostrings"
)

func main() {
	eng := filo.NewEngine()
	filostrings.RegisterBuiltins(eng) // Required for str-fmt, str-upper, etc.

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 64, RecursionLimit: 8, Timeout: time.Second}

	script := `
(let ((name "Alice")
      (points 100))
  (str-fmt "User %s has %g points" (str-upper name) points))
`

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	msg, err := result.AsString()
	if err != nil {
		panic(err)
	}

	fmt.Println(msg)
}

// Output: User ALICE has 100 points
