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
			r := recover()
			if r != nil {
				t.Fatalf("parse panicked: %v", r)
			}
		}()
		_, _ = Parse(string(data))
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
		// Mock string builtins for fuzzing to avoid import cycle
		err := eng.RegisterBuiltin("str-len", func(_ context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return Value{}, nil
			}
			s, _ := args[0].AsString()
			return VNum(float64(len(s))), nil
		})
		if err != nil {
			t.Fatalf("register str-len: %v", err)
		}
		err = eng.RegisterBuiltin("str-sub", func(_ context.Context, args []Value) (Value, error) {
			return VString("mock"), nil
		})
		if err != nil {
			t.Fatalf("register str-sub: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
		defer cancel()

		cfg := EvalConfig{StepLimit: 200, RecursionLimit: 20, Timeout: 0}

		defer func() {
			r := recover()
			if r != nil {
				t.Fatalf("RunScript panicked: %v", r)
			}
		}()

		_, _, _ = eng.RunScript(ctx, string(data), nil, cfg)
	})
}
