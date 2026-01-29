// Example: Pre-Parse/Execute pattern for efficient script reuse
//
// This example demonstrates how to parse a Filo script once and execute it
// multiple times with different data, similar to Go's html/template pattern.
package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filostrings"
)

func main() {
	ctx := context.Background()
	eng := filo.NewEngine()
	cfg := filo.EvalConfig{}

	// Example 1: Simple calculation script
	fmt.Println("=== Simple Pre-Parsed Script ===")
	simpleExample(ctx, eng, cfg)

	// Example 2: Multiple scripts stored by name
	fmt.Println("\n=== Named Script Collection ===")
	namedScriptsExample(ctx, eng, cfg)

	// Example 3: Performance comparison
	fmt.Println("\n=== Performance Comparison ===")
	performanceExample(ctx, eng, cfg)
}

func simpleExample(ctx context.Context, eng *filo.Engine, cfg filo.EvalConfig) {
	// Parse once at startup
	calcScript := filo.Must(filo.ParseScript("calc", "(+ (* x x) (* y y))"))

	// Execute multiple times with different data
	testCases := []struct {
		x, y float64
	}{
		{3, 4},  // 9 + 16 = 25
		{5, 12}, // 25 + 144 = 169
		{8, 15}, // 64 + 225 = 289
	}

	for _, tc := range testCases {
		globals := map[string]filo.Value{
			"x": filo.VNum(tc.x),
			"y": filo.VNum(tc.y),
		}

		result, _, err := calcScript.Execute(ctx, eng, globals, cfg)
		if err != nil {
			log.Fatal(err)
		}

		fmt.Printf("x²+y² for (%v,%v) = %v\n", tc.x, tc.y, result.Num)
	}
}

func namedScriptsExample(ctx context.Context, eng *filo.Engine, cfg filo.EvalConfig) {
	// Store multiple scripts by name (user manages the map)
	scripts := map[string]*filo.Script{
		"greet": filo.Must(filo.ParseScript("greet", `
			(do
				(def greeting (if formal "Dear" "Hello"))
				(str-concat greeting ", " name "!"))
		`)),
		"sum": filo.Must(filo.ParseScript("sum", "(+ a b c)")),
		"discount": filo.Must(filo.ParseScript("discount", `
			(if (> qty 10)
				(* price 0.9)
				price)
		`)),
	}

	// Register string builtins for the greet script
	filostrings.RegisterBuiltins(eng)

	// Use 'greet' script
	result, _, _ := scripts["greet"].Execute(ctx, eng, map[string]filo.Value{
		"name":   filo.VString("Alice"),
		"formal": filo.VBool(true),
	}, cfg)
	fmt.Println("Greeting:", result.Str)

	// Use 'sum' script
	result, _, _ = scripts["sum"].Execute(ctx, eng, map[string]filo.Value{
		"a": filo.VNum(10),
		"b": filo.VNum(20),
		"c": filo.VNum(30),
	}, cfg)
	fmt.Println("Sum:", result.Num)

	// Use 'discount' script
	result, _, _ = scripts["discount"].Execute(ctx, eng, map[string]filo.Value{
		"price": filo.VNum(100),
		"qty":   filo.VNum(15),
	}, cfg)
	fmt.Println("Discounted price:", result.Num)
}

func performanceExample(ctx context.Context, eng *filo.Engine, cfg filo.EvalConfig) {
	const iterations = 10000
	script := "(+ x y)"
	globals := map[string]filo.Value{
		"x": filo.VNum(10),
		"y": filo.VNum(20),
	}

	// Measure RunScript (parse + execute each time)
	start := time.Now()
	for i := 0; i < iterations; i++ {
		_, _, _ = eng.RunScript(ctx, script, globals, cfg)
	}
	runScriptDuration := time.Since(start)

	// Measure Pre-Parsed (parse once, execute many)
	parsedScript := filo.Must(filo.ParseScript("perf", script))
	start = time.Now()
	for i := 0; i < iterations; i++ {
		_, _, _ = parsedScript.Execute(ctx, eng, globals, cfg)
	}
	preParsedDuration := time.Since(start)

	fmt.Printf("RunScript (%d iterations): %v\n", iterations, runScriptDuration)
	fmt.Printf("Pre-Parsed (%d iterations): %v\n", iterations, preParsedDuration)
	fmt.Printf("Speedup: %.2fx faster\n", float64(runScriptDuration)/float64(preParsedDuration))
}
