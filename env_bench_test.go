package filo

import (
	"context"
	"testing"
)

// BenchmarkEnvCreateDefine measures cost of creating a child env and defining vars
// This simulates typical function call overhead.
func BenchmarkEnvCreateDefine(b *testing.B) {
	root := NewEnv()
	root.Define("global", VNum(1))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		child := root.WithChild()
		// Simulate defining arguments (typically 2-4)
		child.Define("a", VNum(10))
		child.Define("b", VNum(20))
		child.Define("c", VNum(30))
		// Simulate local var
		child.Define("local", VNum(40))
	}
}

// BenchmarkEnvGetLocal measures accessing variable in immediate scope
func BenchmarkEnvGetLocal(b *testing.B) {
	root := NewEnv()
	child := root.WithChild()
	child.Define("a", VNum(10))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = child.Get("a")
	}
}

// BenchmarkEnvGetParent measures accessing variable in parent scope
func BenchmarkEnvGetParent(b *testing.B) {
	root := NewEnv()
	root.Define("global", VNum(1))
	child1 := root.WithChild()
	child2 := child1.WithChild()
	child3 := child2.WithChild()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = child3.Get("global")
	}
}

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
		StepLimit:      1000000,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, err := script.Execute(ctx, f.GetEngine(), nil, cfg)
		if err != nil {
			b.Fatal(err)
		}
	}
}
