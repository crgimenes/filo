package filo

import (
	"context"
	"testing"
	"time"
)

func FuzzParseDoesNotPanic(f *testing.F) {
	seeds := []string{
		"",
		"   ",
		"; comment\n(+ 1 2)",
		"()",
		"(+ 1 2)",
		"(if #t 1 2)",
		"(let ((x 1)) x)",
		"\"unterminated",
		"(#x)",
		"(list 1 2 3)",
		"(def f (fn (x) x)) (f 1)",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("parse panicked: %v", r)
			}
		}()
		_, _ = parse(string(data))
	})
}

func FuzzRunScriptDoesNotPanic(f *testing.F) {
	seeds := []string{
		"(+ 1 2)",
		"(do (def x 1) (set x 2) x)",
		"(letv (a b) (values 1 2) (+ a b))",
		"(let () (def loop (fn (n) (loop n))) (loop 0))",
		"(str-len \"café\")",
		"(str-sub \"café\" 1 3)",
		"(str-sub \"hello\" 2)",
	}
	for _, s := range seeds {
		f.Add([]byte(s))
	}

	f.Fuzz(func(t *testing.T, data []byte) {
		eng := NewEngine()
		RegisterStringBuiltins(eng)

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()

		cfg := EvalConfig{StepLimit: 200, RecursionLimit: 20, Timeout: 0}

		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("RunScript panicked: %v", r)
			}
		}()

		_, _, _ = eng.RunScript(ctx, string(data), nil, cfg)
	})
}
