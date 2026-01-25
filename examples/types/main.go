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
	cfg := filo.EvalConfig{StepLimit: 64, RecursionLimit: 8, Timeout: time.Second}

	// type-of returns the type string
	// is-empty checks for empty strings/lists
	script := `
(let ((n 42)
      (s "hello")
      (l (list 1 2)))
  (list
    (type-of n)        ; "number"
    (type-of s)        ; "string"
    (type-of l)        ; "list"
    (is-empty "")      ; #t
    (is-empty "a")     ; #f
    (is-empty (list))  ; #t
  ))
`

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	list, err := result.AsList()
	if err != nil {
		panic(err)
	}

	fmt.Println("Types and Checks:")
	for _, v := range list {
		switch v.Kind {
		case filo.KString:
			fmt.Printf("- %s\n", v.Str)
		case filo.KBool:
			fmt.Printf("- %v\n", v.Bool)
		}
	}
}

// Output:
// Types and Checks:
// - number
// - string
// - list
// - true
// - false
// - true
