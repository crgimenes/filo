package filo

import (
	"context"
	"testing"
	"time"
)

// TestEscapeSequences tests all escape sequences in string parsing.
func TestEscapeSequences(t *testing.T) {
	t.Parallel()
	eng := NewEngine()
	cfg := EvalConfig{StepLimit: 100, RecursionLimit: 16, Timeout: time.Second}
	ctx := context.Background()

	tests := []struct {
		name   string
		script string
		want   string
	}{
		{"escaped-quote", `"hello \"world\""`, `hello "world"`},
		{"newline", `"line1\nline2"`, "line1\nline2"},
		{"tab", `"col1\tcol2"`, "col1\tcol2"},
		{"backslash", `"path\\file"`, `path\file`},
		{"combined", `"a\tb\nc\\d\"e"`, "a\tb\nc\\d\"e"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			val, _, err := eng.RunScript(ctx, tc.script, nil, cfg)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			got, convErr := val.AsString()
			if convErr != nil {
				t.Fatalf("expected string: %v", convErr)
			}
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// TestEscapeErrors tests error conditions in escape parsing.
func TestEscapeErrors(t *testing.T) {
	t.Parallel()
	eng := NewEngine()
	cfg := EvalConfig{StepLimit: 100, RecursionLimit: 16, Timeout: time.Second}
	ctx := context.Background()

	tests := []struct {
		name   string
		script string
	}{
		{"unterminated-string", `"hello`},
		{"unterminated-escape", `"hello\`},
		{"unsupported-escape", `"hello\x"`},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := eng.RunScript(ctx, tc.script, nil, cfg)
			if err == nil {
				t.Fatal("expected error")
			}
		})
	}
}

// TestParseErrorMethod tests ParseError.Error() method.
func TestParseErrorMethod(t *testing.T) {
	t.Parallel()

	pe := &ParseError{Pos: 10, Message: "test error", Near: "abc"}
	err := pe.Error()
	if err == "" {
		t.Fatal("expected non-empty error string")
	}

	// Test error without Near
	pe2 := &ParseError{Pos: 5, Message: "simple error"}
	err2 := pe2.Error()
	if err2 == "" {
		t.Fatal("expected non-empty error string")
	}
}
