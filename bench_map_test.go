package filo

import (
	"testing"
)

// BenchmarkMapAllocations compares standard map (allocating slice per call)
// vs optimized map (reusing slice, forcing copy in callFunc).
func BenchmarkMapAllocations(b *testing.B) {
	// Setup
	// Define a simple function (x) => x
	// We use a "native" builtin as the target function to avoid interpreter overhead noise
	// But callFunc logic is in evaluator.
	// We need to simulate the loop logic.

	// Target function (mock)
	fn := &Func{
		Params: []string{"x"},
		// Body is empty for this micro-bench
	}

	// 1. Current Implementation (Alloc per iteration)
	b.Run("AllocPerIter", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Simulate map loop
			for j := 0; j < 1000; j++ {
				arg := VNum(float64(j))
				// Allocates slice
				args := []Value{arg}
				// Simulate callFunc taking ownership
				_ = &Frame{slots: args, parent: fn.Frame}
			}
		}
	})

	// 2. Optimized Reuse Buffer (Needs Copy in callee)
	b.Run("ReuseBuffer_Copy", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Pre-allocate buffer
			buffer := make([]Value, 1)
			for j := 0; j < 1000; j++ {
				arg := VNum(float64(j))
				buffer[0] = arg

				// Simulate callFunc COPYING args
				// 1. Copy
				argsCopy := make([]Value, 1)
				copy(argsCopy, buffer)
				// 2. Use
				_ = &Frame{slots: argsCopy, parent: fn.Frame}
			}
		}
	})
}
