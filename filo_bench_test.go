package filo

import (
	"context"
	"fmt"
	"testing"
)

// BenchmarkTemplateReuse simulates the "Parse Once, Execute Many" pattern
// requested by the user, similar to Go HTML templates.
func BenchmarkTemplateReuse(b *testing.B) {
	// 1. Setup Phase (Parse Once)
	eng := NewEngine()
	// Simulating a moderate script:
	// - Some math
	// - String formatting
	// - Logic
	// - Uses both locals and globals
	src := `
	(let ((x in-x) (y in-y))
			(if (> x y)
			x
			(+ y 10)))
	`
	// RegisterStringBuiltins(eng) // Removed to avoid cycle

	prog, err := eng.Compile(src)
	if err != nil {
		b.Fatalf("compile failed: %v", err)
	}

	// 2. Execution Phase (Execute Many)
	b.ResetTimer()
	b.ReportAllocs()

	// Use parallel to verify thread safety and lock contention on SymbolTable
	b.RunParallel(func(pb *testing.PB) {
		ctx := context.Background()
		// Each usage has its own "context" / globals
		iter := 0
		for pb.Next() {
			iter++
			globals := map[string]Value{
				"in-x": VNum(float64(iter)),
				"in-y": VNum(float64(iter * 2)),
			}
			_, _, err := prog.Execute(ctx, globals, EvalConfig{})
			if err != nil {
				// Avoid t.Fatal in parallel loop, just panic
				panic(fmt.Sprintf("exec error: %v", err))
			}
		}
	})
}

// BenchmarkAllocAnalysis checks deeply specific allocations
func BenchmarkGlobalEnvAlloc(b *testing.B) {
	eng := NewEngine()
	// Pre-fill symbol table to simulate a larger application
	for i := 0; i < 1000; i++ {
		eng.symbols.Resolve(fmt.Sprintf("var-%d", i))
	}

	prog, _ := eng.Compile("(+ 1 1)")

	b.ResetTimer()
	b.ReportAllocs()

	ctx := context.Background()
	globals := map[string]Value{} // Empty globals

	for i := 0; i < b.N; i++ {
		_, _, _ = prog.Execute(ctx, globals, EvalConfig{})
	}
}
