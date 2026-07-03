package filo

import (
	"context"
	"testing"
)

func TestScriptName(t *testing.T) {
	s, _ := ParseScript("test", "(+ 1 2)")
	if s.Name() != "test" {
		t.Errorf("Name() = %q, want %q", s.Name(), "test")
	}
}

func TestScriptParse(t *testing.T) {
	s, err := ParseScript("calc", "(+ 1 2)")
	if err != nil {
		t.Fatalf("Parse error: %v", err)
	}
	if s.Name() != "calc" {
		t.Errorf("Name() = %q, want %q", s.Name(), "calc")
	}
}

func TestScriptParseError(t *testing.T) {
	_, err := ParseScript("bad", "(+ 1")
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestParseScript(t *testing.T) {
	s, err := ParseScript("calc", "(+ 1 2)")
	if err != nil {
		t.Fatalf("ParseScript error: %v", err)
	}
	if s.Name() != "calc" {
		t.Errorf("Name() = %q, want %q", s.Name(), "calc")
	}
}

func TestScriptExecute(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()
	cfg := EvalConfig{}

	script, err := ParseScript("add", "(+ x y)")
	if err != nil {
		t.Fatalf("ParseScript error: %v", err)
	}

	globals := map[string]Value{
		"x": VNum(10),
		"y": VNum(32),
	}

	result, _, err := script.Execute(ctx, eng, globals, cfg)
	if err != nil {
		t.Fatalf("Execute error: %v", err)
	}

	if result.Kind != KNumber || result.Num != 42 {
		t.Errorf("result = %v, want 42", result)
	}
}

func TestScriptExecuteMultipleTimes(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()
	cfg := EvalConfig{}

	script := Must(ParseScript("sum", "(+ a b)"))

	testCases := []struct {
		a, b, expected float64
	}{
		{1, 2, 3},
		{10, 20, 30},
		{100, 200, 300},
		{-5, 5, 0},
	}

	for _, tc := range testCases {
		globals := map[string]Value{
			"a": VNum(tc.a),
			"b": VNum(tc.b),
		}

		result, _, err := script.Execute(ctx, eng, globals, cfg)
		if err != nil {
			t.Fatalf("Execute error: %v", err)
		}

		if result.Kind != KNumber || result.Num != tc.expected {
			t.Errorf("(%v + %v) = %v, want %v", tc.a, tc.b, result.Num, tc.expected)
		}
	}
}

func TestScriptExecuteNotParsed(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()
	cfg := EvalConfig{}

	// Create a script but don't parse it properly (this tests nil AST)
	script := &Script{name: "empty"}
	_, _, err := script.Execute(ctx, eng, nil, cfg)
	if err == nil {
		t.Fatal("expected error for unparsed script, got nil")
	}
}

func TestMust(t *testing.T) {
	// Should not panic
	script := Must(ParseScript("ok", "(+ 1 2)"))
	if script == nil {
		t.Fatal("Must returned nil")
	}
}

func TestMustPanic(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected Must to panic on error")
		}
	}()

	Must(ParseScript("bad", "(+ 1"))
}

func TestRunScriptBackwardCompatibility(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()
	cfg := EvalConfig{}

	result, _, err := eng.RunScript(ctx, "(+ 10 20)", nil, cfg)
	if err != nil {
		t.Fatalf("RunScript error: %v", err)
	}

	if result.Kind != KNumber || result.Num != 30 {
		t.Errorf("result = %v, want 30", result)
	}
}

func TestRunScriptWithGlobals(t *testing.T) {
	ctx := context.Background()
	eng := NewEngine()
	cfg := EvalConfig{}

	globals := map[string]Value{
		"x": VNum(5),
	}

	result, _, err := eng.RunScript(ctx, "(* x x)", globals, cfg)
	if err != nil {
		t.Fatalf("RunScript error: %v", err)
	}

	if result.Kind != KNumber || result.Num != 25 {
		t.Errorf("result = %v, want 25", result)
	}
}

// BenchmarkRunScript benchmarks parsing + executing each time
func BenchmarkRunScript(b *testing.B) {
	ctx := context.Background()
	eng := NewEngine()
	cfg := EvalConfig{}
	globals := map[string]Value{
		"x": VNum(10),
		"y": VNum(20),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = eng.RunScript(ctx, "(+ x y)", globals, cfg)
	}
}

// BenchmarkPreParsed benchmarks executing a pre-parsed script
func BenchmarkPreParsed(b *testing.B) {
	ctx := context.Background()
	eng := NewEngine()
	cfg := EvalConfig{}
	script := Must(ParseScript("add", "(+ x y)"))
	globals := map[string]Value{
		"x": VNum(10),
		"y": VNum(20),
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = script.Execute(ctx, eng, globals, cfg)
	}
}
