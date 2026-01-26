package filo

import (
	"context"
	"testing"
)

// BenchmarkRecursiveCall measures full engine overhead including Env creation
// Script: (def fib (fn (n) (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2))))))
// BenchmarkRecursiveCall measures full engine overhead including Env creation
// Script: (def fib (fn (n) (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2))))))
func BenchmarkRecursiveCall(b *testing.B) {
	ctx := context.Background()
	f := New()
	// Pre-parse the script
	script, err := ParseScript("fib", `
		(do 
			(def fib (fn (n) 
				(if (< n 2) 
					n 
					(+ (fib (- n 1)) (fib (- n 2))))))
			(fib 15))
	`)
	if err != nil {
		b.Fatal(err)
	}

	// Use script.Execute directly to customize limits
	cfg := EvalConfig{
		RecursionLimit: 10000,
		StepLimit:      10_000_000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := script.Execute(ctx, f.GetEngine(), nil, cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}
