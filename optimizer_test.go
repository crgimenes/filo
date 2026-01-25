package filo

import (
	"testing"
)

func TestFoldConstants(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string // Formatted
		changed  bool
	}{
		{
			name:     "pure add",
			input:    "(+ 1 2)",
			expected: "3",
			changed:  true,
		},
		{
			name:     "nested pure",
			input:    "(+ 1 (* 2 3))",
			expected: "7",
			changed:  true,
		},
		{
			name:     "mixed pure/impure",
			input:    "(+ 1 (print \"x\"))",
			expected: "(+ 1 (print \"x\"))",
			changed:  false, // Inner (print) is not pure, outer + has non-literal arg
		},
		{
			name:     "list wrapper",
			input:    "(list (+ 1 2) 3)",
			expected: "(list 3 3)", // (+ 1 2) -> 3
			changed:  true,
		},
		{
			name:     "if constant true",
			input:    "(if #t 1 2)",
			expected: "1",
			changed:  true,
		},
		{
			name:     "if constant false",
			input:    "(if #f 1 2)",
			expected: "2",
			changed:  true,
		},
		{
			name:     "if constant true nested",
			input:    "(if #t (+ 1 2) 0)",
			expected: "3",
			changed:  true,
		},
		{
			name:     "nested expressions",
			input:    "(list (+ 1 2) (* 3 4))",
			expected: "(list 3 12)",
			changed:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := Parse(tt.input)
			if err != nil {
				t.Fatalf("Parse error: %v", err)
			}

			folded, changed := FoldConstants(ast)
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}

			if changed {
				formatted, err := FormatAST(folded, FormatConfig{MaxLineWidth: 80})
				if err != nil {
					t.Fatalf("FormatAST error: %v", err)
				}
				if formatted != tt.expected {
					t.Errorf("got %q, want %q", formatted, tt.expected)
				}
			}
		})
	}
}
