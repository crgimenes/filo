package main

import (
	"context"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filoprint"
)

func main() {
	eng := filo.NewEngine()
	filoprint.RegisterPrintBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 1000, RecursionLimit: 32, Timeout: time.Second}

	script := `
;; Simple print examples
(print "Hello, World!")
(print "Sum:" (+ 1 2 3))
(print "List:" (list 1 2 3))

;; printf with format specifiers
(printf "User: %s, Age: %d" "Alice" 25)
(printf "Price: R$ %.2f" 19.99)

;; %T shows the Filo type
(printf "42 is a %T" 42)
(printf "\"hello\" is a %T" "hello")
(printf "#t is a %T" #t)
(printf "(list 1 2) is a %T" (list 1 2))

;; Combining %T with %v
(let ((x 3.14))
  (printf "x = %v (type: %T)" x x))

;; Multiple values
(printf "Types: %T, %T, %T" 42 "text" #f)

"done"
`

	_, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		panic(err)
	}
}

// Output:
// Hello, World!
// Sum: 6
// List: (1 2 3)
// User: Alice, Age: 25
// Price: R$ 19.99
// 42 is a number
// "hello" is a string
// #t is a bool
// (list 1 2) is a list
// x = 3.14 (type: number)
// Types: number, string, bool
