package main

import (
	"context"
	"fmt"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filojson"
	"github.com/crgimenes/filo/filostrings"
)

func main() {
	eng := filo.NewEngine()
	filostrings.RegisterBuiltins(eng)
	filojson.RegisterJSONBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 128, RecursionLimit: 16, Timeout: time.Second}

	// Build an object as list of pairs (list "key" value)
	scriptMarshal := `
(json-marshal
  (list
    (list "name" "Alice")
    (list "age" 30)
    (list "tags" (list "dev" "golang"))
    (list "active" #t)
    (list "extra" (json-null))))
`

	j, _, err := eng.RunScript(ctx, scriptMarshal, nil, cfg)
	if err != nil {
		panic(err)
	}
	js, _ := j.AsString()
	fmt.Println("JSON:", js)

	// Unmarshal back and inspect the structure (list of pairs)
	globals := map[string]filo.Value{"j": filo.VString(js)}
	v, _, err := eng.RunScript(ctx, "(json-unmarshal j)", globals, cfg)
	if err != nil {
		panic(err)
	}

	// Show Filo representation
	fmt.Println("Filo:", v.String())
}

// Example output (key order may vary):
// JSON: {"active":true,"age":30,"extra":null,"name":"Alice","tags":["dev","golang"]}
// Filo: (list (tuple "active" #t) (tuple "age" 30) (tuple "extra" (tuple)) (tuple "name" "Alice") (tuple "tags" (list "dev" "golang")))
