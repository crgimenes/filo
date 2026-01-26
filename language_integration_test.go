package filo_test

import (
	"context"
	"math"
	"strings"
	"testing"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filomath"
)

func TestLanguageFeatures(t *testing.T) {
	tests := []struct {
		name      string
		script    string
		want      filo.Value
		wantIsVal bool
		wantErr   string
	}{
		// --- Literals ---
		{
			name:      "number int",
			script:    "42",
			want:      filo.VNum(42),
			wantIsVal: true,
		},
		{
			name:      "number float",
			script:    "3.14",
			want:      filo.VNum(3.14),
			wantIsVal: true,
		},
		{
			name:      "string",
			script:    `"hello"`,
			want:      filo.VString("hello"),
			wantIsVal: true,
		},
		{
			name:      "bool true",
			script:    "#t",
			want:      filo.VBool(true),
			wantIsVal: true,
		},
		{
			name:      "bool false",
			script:    "#f",
			want:      filo.VBool(false),
			wantIsVal: true,
		},
		{
			name:      "list empty",
			script:    "(list)",
			want:      filo.VList([]filo.Value{}),
			wantIsVal: true,
		},
		{
			name:      "list values",
			script:    `(list 1 "a")`,
			want:      filo.VList([]filo.Value{filo.VNum(1), filo.VString("a")}),
			wantIsVal: true,
		},

		// --- Math Builtins (Core + filomath) ---
		{
			name:      "add",
			script:    "(+ 1 2)",
			want:      filo.VNum(3),
			wantIsVal: true,
		},
		{
			name:      "sub",
			script:    "(- 10 3)",
			want:      filo.VNum(7),
			wantIsVal: true,
		},
		{
			name:      "mul",
			script:    "(* 2 3 4)",
			want:      filo.VNum(24),
			wantIsVal: true,
		},
		{
			name:      "div",
			script:    "(/ 10 2)",
			want:      filo.VNum(5),
			wantIsVal: true,
		},
		{
			name:      "mod",
			script:    "(% 10 3)",
			want:      filo.VNum(1),
			wantIsVal: true,
		},
		{
			name:      "sqrt",
			script:    "(sqrt 16)",
			want:      filo.VNum(4),
			wantIsVal: true,
		},
		{
			name:      "abs negative",
			script:    "(abs -5)",
			want:      filo.VNum(5),
			wantIsVal: true,
		},

		// --- Logic & Comparison ---
		{
			name:      "equal numbers",
			script:    "(= 1 1)",
			want:      filo.VBool(true),
			wantIsVal: true,
		},
		{
			name:      "not equal numbers",
			script:    "(= 1 2)",
			want:      filo.VBool(false),
			wantIsVal: true,
		},
		{
			name:      "not equal builtin",
			script:    "(!= 1 2)",
			want:      filo.VBool(true),
			wantIsVal: true,
		},
		{
			name:      "less than",
			script:    "(< 1 2)",
			want:      filo.VBool(true),
			wantIsVal: true,
		},
		{
			name:      "greater than",
			script:    "(> 2 1)",
			want:      filo.VBool(true),
			wantIsVal: true,
		},
		{
			name:      "and true",
			script:    "(and #t #t)",
			want:      filo.VBool(true),
			wantIsVal: true,
		},
		{
			name:      "and false",
			script:    "(and #t #f)",
			want:      filo.VBool(false),
			wantIsVal: true,
		},
		{
			name:      "or true",
			script:    "(or #f #t)",
			want:      filo.VBool(true),
			wantIsVal: true,
		},
		{
			name:      "not",
			script:    "(not #f)",
			want:      filo.VBool(true),
			wantIsVal: true,
		},

		// --- String Builtins ---
		{
			name:      "str-len string",
			script:    `(str-len "abc")`,
			want:      filo.VNum(3),
			wantIsVal: true,
		},
		{
			name:      "str-find",
			script:    `(str-find "foo" "foobar")`,
			want:      filo.VBool(true),
			wantIsVal: true,
		},
		{
			name:      "str-lower",
			script:    `(str-lower "ABC")`,
			want:      filo.VString("abc"),
			wantIsVal: true,
		},
		{
			name:      "str-upper",
			script:    `(str-upper "abc")`,
			want:      filo.VString("ABC"),
			wantIsVal: true,
		},

		// --- List Builtins ---
		{
			name:      "length list",
			script:    `(length (list 1 2 3))`,
			want:      filo.VNum(3),
			wantIsVal: true,
		},
		{
			name:      "head",
			script:    `(head (list 1 2))`,
			want:      filo.VNum(1),
			wantIsVal: true,
		},
		{
			name:      "tail",
			script:    `(tail (list 1 2 3))`,
			want:      filo.VList([]filo.Value{filo.VNum(2), filo.VNum(3)}),
			wantIsVal: true,
		},
		{
			name:      "list-append",
			script:    `(list-append (list 1) 2)`,
			want:      filo.VList([]filo.Value{filo.VNum(1), filo.VNum(2)}),
			wantIsVal: true,
		},
		{
			name:      "map builtin",
			script:    `(map (fn (x) (* x 2)) (list 1 2 3))`,
			want:      filo.VList([]filo.Value{filo.VNum(2), filo.VNum(4), filo.VNum(6)}),
			wantIsVal: true,
		},
		{
			name:      "fold builtin",
			script:    `(fold (fn (acc x) (+ acc x)) 0 (list 1 2 3 4))`,
			want:      filo.VNum(10),
			wantIsVal: true,
		},

		// --- Control Flow ---
		{
			name:      "if true",
			script:    "(if #t 1 2)",
			want:      filo.VNum(1),
			wantIsVal: true,
		},
		{
			name:      "if false",
			script:    "(if #f 1 2)",
			want:      filo.VNum(2),
			wantIsVal: true,
		},
		{
			name:      "do chain",
			script:    "(do 1 2 3)",
			want:      filo.VNum(3),
			wantIsVal: true,
		},

		// --- Variables & Scopes ---
		{
			name:      "let local",
			script:    "(let ((x 10)) x)",
			want:      filo.VNum(10),
			wantIsVal: true,
		},
		{
			name:      "let sequential",
			script:    "(let ((x 1) (y (+ x 1))) y)",
			want:      filo.VNum(2),
			wantIsVal: true,
		},
		{
			name:      "let shadowing",
			script:    "(let ((x 1)) (let ((x 2)) x))",
			want:      filo.VNum(2),
			wantIsVal: true,
		},
		{
			name:      "let outer access",
			script:    "(let ((x 1)) (let ((y 2)) x))",
			want:      filo.VNum(1),
			wantIsVal: true,
		},
		{
			name:      "global def",
			script:    "(do (def g 100) g)",
			want:      filo.VNum(100),
			wantIsVal: true,
		},
		{
			name:      "set global",
			script:    "(do (def g 1) (set g 2) g)",
			want:      filo.VNum(2),
			wantIsVal: true,
		},
		{
			name:      "set local",
			script:    "(let ((x 1)) (do (set x 2) x))",
			want:      filo.VNum(2),
			wantIsVal: true,
		},
		{
			name:      "letvDestructuring",
			script:    "(letv (a b) (tuple 1 2) (+ a b))",
			want:      filo.VNum(3),
			wantIsVal: true,
		},

		// --- Functions & Closures ---
		{
			name:      "fn identity",
			script:    "((fn (x) x) 42)",
			want:      filo.VNum(42),
			wantIsVal: true,
		},
		{
			name:      "fn closure simple",
			script:    "(let ((x 10)) ((fn (y) (+ x y)) 5))",
			want:      filo.VNum(15),
			wantIsVal: true,
		},
		{
			name: "fn closure counter",
			script: `
				(do 
					(def make-adder (fn (k) (fn (x) (+ x k))))
					(def add5 (make-adder 5))
					(add5 10))`,
			want:      filo.VNum(15),
			wantIsVal: true,
		},
		{
			name: "recursion fib",
			script: `
				(do
					(def fib (fn (n) (if (< n 2) n (+ (fib (- n 1)) (fib (- n 2))))))
					(fib 10))`,
			want:      filo.VNum(55),
			wantIsVal: true,
		},

		// --- Values / Tuples ---
		{
			name:      "tuple create",
			script:    "(tuple 1 2)",
			want:      filo.VTuple([]filo.Value{filo.VNum(1), filo.VNum(2)}),
			wantIsVal: true,
		},
		{
			name:      "values alias",
			script:    "(values 1 2)",
			want:      filo.VTuple([]filo.Value{filo.VNum(1), filo.VNum(2)}),
			wantIsVal: true,
		},

		// --- Errors ---
		{
			name:    "undefined symbol",
			script:  `missing-var`,
			wantErr: "undefined global: missing-var",
		},
		{
			name:    "arity mismatch",
			script:  "((fn (x) x) 1 2)",
			wantErr: "function expects 1 args, got 2",
		},
		{
			name:    "type mismatch +",
			script:  `(+ 1 "a")`,
			wantErr: "expected number, got string",
		},
		// Divide by zero might panic or error depending on impl. Logic check:
		{
			name:    "divide by zero",
			script:  "(/ 1 0)",
			wantErr: "division by zero",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := filo.NewEngine()

			// Register Extensions
			filomath.RegisterMathBuiltins(eng)
			filo.RegisterStringBuiltins(eng)
			// filoprint/random not needed for these pure tests

			ctx := context.Background()

			// Use ParseScript -> Execute to ensure compilation
			script, err := filo.ParseScript(tt.name, tt.script)
			if err != nil {
				t.Fatalf("ParseScript failed: %v", err)
			}

			res, _, err := script.Execute(ctx, eng, nil, filo.EvalConfig{})

			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Errorf("expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected execution error: %v", err)
			}

			if tt.wantIsVal {
				// Compare values broadly
				if !valuesEqual(res, tt.want) {
					t.Errorf("got %v, want %v", res, tt.want)
				}
			}
		})
	}
}

// Helper to compare values (exported logic from filo_test, simplified for integration)
func valuesEqual(a, b filo.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case filo.KNumber:
		if math.IsNaN(a.Num) && math.IsNaN(b.Num) {
			return true
		}
		return math.Abs(a.Num-b.Num) < 1e-9
	case filo.KBool:
		return a.Bool == b.Bool
	case filo.KString:
		return a.Str == b.Str
	case filo.KList:
		if len(a.List) != len(b.List) {
			return false
		}
		for i := range a.List {
			if !valuesEqual(a.List[i], b.List[i]) {
				return false
			}
		}
		return true
	case filo.KTuple:
		if len(a.Tup) != len(b.Tup) {
			return false
		}
		for i := range a.Tup {
			if !valuesEqual(a.Tup[i], b.Tup[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}
