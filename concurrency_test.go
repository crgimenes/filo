package filo

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestConcurrencySafety stresses the engine with parallel execution, compilation, and dynamic symbol creation.
func TestConcurrencySafety(t *testing.T) {
	eng := NewEngine()
	RegisterStringBuiltins(eng)

	// 1. Compile a base program (Program reuse scenario)
	baseScript := `
	(let ((x in-x))
		(str-fmt "val: %g" x))`
	prog, err := eng.Compile(baseScript)
	if err != nil {
		t.Fatalf("compile failed: %v", err)
	}

	var wg sync.WaitGroup
	start := make(chan struct{})

	// Scenario A: Massive Parallel Execution of Pre-Compiled Program
	// Simulates HTTP handlers executing logic
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			// Run multiple times per goroutine
			for j := 0; j < 100; j++ {
				globals := map[string]Value{"in-x": VNum(float64(id*100 + j))}
				_, _, err := prog.Execute(context.Background(), globals, EvalConfig{})
				if err != nil {
					// Don't fatal in goroutine
					panic(fmt.Sprintf("exec error: %v", err))
				}
			}
		}(i)
	}

	// Scenario B: Concurrent Dynamic Compilation
	// Simulates lazy loading of new scripts
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			src := fmt.Sprintf("(+ 1 %d)", id)
			for j := 0; j < 50; j++ {
				_, err := eng.Compile(src)
				if err != nil {
					panic(fmt.Sprintf("concurrent compile error: %v", err))
				}
			}
		}(i)
	}

	// Scenario C: Dynamic Global Definition (Symbol Table Stress)
	// Scripts that define NEW unique globals at runtime: (set dynamic-var-X 1)
	// This forces SymbolTable.Resolve() to acquire locks and resize.
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			<-start
			// Each iteration defines a totally new global variable
			for j := 0; j < 50; j++ {
				varName := fmt.Sprintf("dyn-%d-%d", id, j)
				src := fmt.Sprintf("(set %s %d)", varName, j)
				_, _, err := eng.RunScript(context.Background(), src, nil, EvalConfig{})
				if err != nil {
					panic(fmt.Sprintf("dynamic set error: %v", err))
				}
			}
		}(i)
	}

	// Start all
	close(start)

	// Wait with timeout
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Success
	case <-time.After(5 * time.Second):
		t.Fatal("Test timed out - possible deadlock in SymbolTable locks?")
	}
}

// TestBuiltinRace simulates modifying builtins while compiling/executing.
// This is EXPECTED to fail if we don't protect builtins map, but we verify behavior.
// Filo usually assumes builtins are registered at startup.
func TestBuiltinReadRace(t *testing.T) {
	eng := NewEngine()
	// No writes, just concurrent reads via Compile

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := eng.Compile("(+ 1 1)")
			if err != nil {
				panic(err)
			}
		}()
	}
	wg.Wait()
}
