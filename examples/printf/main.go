package main

import (
	"context"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filoprint"
)

func main() {
	eng := filo.NewEngine()
	filoprint.RegisterBuiltins(eng)

	ctx := context.Background()
	cfg := filo.EvalConfig{StepLimit: 1000, RecursionLimit: 32, Timeout: time.Second}

	script := `
;; Simple print examples
(println "Hello, World!")
(println "Sum:" (+ 1 2 3))
(println "List:" (list 1 2 3))

;; printf with format specifiers
(printf "User: %s, Age: %d\n" "Alice" 25)
(printf "Price: R$ %.2f\n" 19.99)

;; %T shows the Filo type
(printf "42 is a %T\n" 42)
(printf "\"hello\" is a %T\n" "hello")
(printf "#t is a %T\n" #t)
(printf "(list 1 2) is a %T\n" (list 1 2))

;; Combining %T with %v
(let ((x 3.14))
	(printf "x = %v (type: %T)\n" x x))

;; Multiple values
(printf "Types: %T, %T, %T\n" 42 "text" #f)

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
// List: (list 1 2 3)
// User: Alice, Age: 25
// Price: R$ 19.99
// 42 is a number
// "hello" is a string
// #t is a bool
// (list 1 2) is a list
// x = 3.14 (type: number)
// Types: number, string, bool
