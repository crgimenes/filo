// Command cast demonstrates Filo's explicit type coercions: (string x) renders
// any value as text and (number s) parses a numeric string. Filo's = and !=
// refuse to compare values of different kinds, so casts make the intent
// explicit: (= (string 1) "1") instead of a silent implicit coercion.
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
	cfg := filo.EvalConfig{StepLimit: 1024, RecursionLimit: 16, Timeout: time.Second}

	script := `
; string renders any value as text
(set as-text (string 42))

; number parses a numeric string ((number "abc") would be an error)
(set as-number (number "1.5"))

; the casts make cross-kind comparison explicit
(set same (= (string 42) "42"))

(list as-text as-number same)`

	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}

	items, err := result.AsList()
	if err != nil {
		panic(err)
	}

	text, _ := items[0].AsString()
	num, _ := items[1].AsNumber()
	same, _ := items[2].AsBool()
	fmt.Printf("as-text=%q as-number=%v same=%v\n", text, num, same)
}

// Output: as-text="42" as-number=1.5 same=true
