package filo

import (
	"context"
	"testing"
	"time"
)

func TestStringBuiltins(t *testing.T) {
	tests := []struct {
		name    string
		script  string
		want    string
		wantErr bool
	}{
		// str-join tests
		{
			name:   "str-join basic",
			script: `(str-join ", " (list "a" "b" "c"))`,
			want:   `"a, b, c"`,
		},
		{
			name:   "str-join empty separator",
			script: `(str-join "" (list "a" "b" "c"))`,
			want:   `"abc"`,
		},
		{
			name:   "str-join empty list",
			script: `(str-join ", " (list))`,
			want:   `""`,
		},
		{
			name:    "str-join wrong args count",
			script:  `(str-join ", ")`,
			wantErr: true,
		},
		{
			name:    "str-join non-string separator",
			script:  `(str-join 123 (list "a" "b"))`,
			wantErr: true,
		},

		// str-split tests
		{
			name:   "str-split basic",
			script: `(str-split ", " "a, b, c")`,
			want:   `(list "a" "b" "c")`,
		},
		{
			name:   "str-split empty separator",
			script: `(str-split "" "abc")`,
			want:   `(list "a" "b" "c")`,
		},
		{
			name:   "str-split no match",
			script: `(str-split "x" "abc")`,
			want:   `(list "abc")`,
		},
		{
			name:    "str-split wrong args",
			script:  `(str-split ", ")`,
			wantErr: true,
		},

		// str-find tests
		{
			name:   "str-find found",
			script: `(str-find "world" "hello world")`,
			want:   `#t`,
		},
		{
			name:   "str-find not found",
			script: `(str-find "xyz" "hello world")`,
			want:   `#f`,
		},
		{
			name:   "str-find empty substring",
			script: `(str-find "" "hello")`,
			want:   `#t`,
		},
		{
			name:    "str-find wrong args",
			script:  `(str-find "test")`,
			wantErr: true,
		},

		// str-trim tests
		{
			name:   "str-trim basic",
			script: `(str-trim "  hello  ")`,
			want:   `"hello"`,
		},
		{
			name:   "str-trim tabs and newlines",
			script: "(str-trim \"\t\nhello\t\n\")",
			want:   `"hello"`,
		},
		{
			name:   "str-trim no whitespace",
			script: `(str-trim "hello")`,
			want:   `"hello"`,
		},
		{
			name:    "str-trim wrong args",
			script:  `(str-trim)`,
			wantErr: true,
		},

		// str-replace tests
		{
			name:   "str-replace basic",
			script: `(str-replace "world" "Filo" "hello world")`,
			want:   `"hello Filo"`,
		},
		{
			name:   "str-replace multiple occurrences",
			script: `(str-replace "a" "X" "banana")`,
			want:   `"bXnXnX"`,
		},
		{
			name:   "str-replace no match",
			script: `(str-replace "x" "y" "hello")`,
			want:   `"hello"`,
		},
		{
			name:    "str-replace wrong args",
			script:  `(str-replace "a" "b")`,
			wantErr: true,
		},

		// str-upper tests
		{
			name:   "str-upper basic",
			script: `(str-upper "hello")`,
			want:   `"HELLO"`,
		},
		{
			name:   "str-upper mixed",
			script: `(str-upper "Hello World")`,
			want:   `"HELLO WORLD"`,
		},
		{
			name:    "str-upper wrong args",
			script:  `(str-upper)`,
			wantErr: true,
		},

		// str-lower tests
		{
			name:   "str-lower basic",
			script: `(str-lower "HELLO")`,
			want:   `"hello"`,
		},
		{
			name:   "str-lower mixed",
			script: `(str-lower "Hello World")`,
			want:   `"hello world"`,
		},
		{
			name:    "str-lower wrong args",
			script:  `(str-lower)`,
			wantErr: true,
		},

		// str-concat tests
		{
			name:   "str-concat basic",
			script: `(str-concat "hello" " " "world")`,
			want:   `"hello world"`,
		},
		{
			name:   "str-concat single",
			script: `(str-concat "hello")`,
			want:   `"hello"`,
		},
		{
			name:   "str-concat empty",
			script: `(str-concat)`,
			want:   `""`,
		},
		{
			name:    "str-concat non-string",
			script:  `(str-concat "hello" 123)`,
			wantErr: true,
		},

		// str-len tests
		{
			name:   "str-len basic",
			script: `(str-len "hello")`,
			want:   `5`,
		},
		{
			name:   "str-len empty",
			script: `(str-len "")`,
			want:   `0`,
		},
		{
			name:   "str-len unicode",
			script: `(str-len "café")`,
			want:   `4`, // rune count
		},
		{
			name:    "str-len wrong args",
			script:  `(str-len)`,
			wantErr: true,
		},

		// str-sub tests
		{
			name:   "str-sub basic",
			script: `(str-sub "hello world" 0 5)`,
			want:   `"hello"`,
		},
		{
			name:   "str-sub middle",
			script: `(str-sub "hello world" 6 11)`,
			want:   `"world"`,
		},
		{
			name:   "str-sub end beyond length",
			script: `(str-sub "hello world" 6 100)`,
			want:   `"world"`,
		},
		{
			name:   "str-sub start negative",
			script: `(str-sub "hello world" -5 5)`,
			want:   `"hello"`,
		},
		{
			name:   "str-sub start beyond end",
			script: `(str-sub "hello world" 10 5)`,
			want:   `""`,
		},
		{
			name:   "str-sub end omitted",
			script: `(str-sub "hello" 2)`,
			want:   `"llo"`,
		},
		{
			name:   "str-sub unicode runes",
			script: `(str-sub "café" 2 4)`,
			want:   `"fé"`,
		},
		{
			name:    "str-sub wrong args",
			script:  `(str-sub "x")`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			eng := NewEngine()
			RegisterStringBuiltins(eng)

			ctx := context.Background()
			cfg := EvalConfig{
				StepLimit:      1000,
				RecursionLimit: 64,
				Timeout:        time.Second,
			}

			result, _, err := eng.RunScript(ctx, tt.script, nil, cfg)
			if tt.wantErr {
				if err == nil {
					t.Errorf("expected error, got result: %v", result)
				}
				return
			}
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			got := result.String()
			if got != tt.want {
				t.Errorf("got %s, want %s", got, tt.want)
			}
		})
	}
}

func TestStringBuiltinsIntegration(t *testing.T) {
	// Test combining multiple string builtins
	eng := NewEngine()
	RegisterStringBuiltins(eng)

	ctx := context.Background()
	cfg := EvalConfig{
		StepLimit:      1000,
		RecursionLimit: 64,
		Timeout:        time.Second,
	}

	// Test: uppercase, split, join
	script := `(str-join "-" (str-split " " (str-upper "hello world")))`
	result, _, err := eng.RunScript(ctx, script, nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := `"HELLO-WORLD"`
	if result.String() != want {
		t.Errorf("got %s, want %s", result.String(), want)
	}

	// Test: find after replace
	script2 := `(str-find "Filo" (str-replace "Lua" "Filo" "I love Lua"))`
	result2, _, err := eng.RunScript(ctx, script2, nil, cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.String() != "#t" {
		t.Errorf("got %s, want #t", result2.String())
	}
}
