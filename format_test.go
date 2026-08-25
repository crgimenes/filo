package filo

import (
	"strings"
	"testing"
)

func TestFormatSimple(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"number", "42", "42"},
		{"negative", "-3.14", "-3.14"},
		{"string", `"hello"`, `"hello"`},
		{"bool true", "#t", "#t"},
		{"bool false", "#f", "#f"},
		{"empty list", "()", "()"},
		{"simple add", "(+ 1 2)", "(+ 1 2)"},
		{"nested", "((foo))", "(\n  (foo))"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Format(tt.input)
			if err != nil {
				t.Fatalf("Format error: %v", err)
			}
			result = strings.TrimSpace(result)
			if result != tt.expect {
				t.Errorf("got %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestFormatStringEscapes(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{"escaped quote", `"test \" test"`, `"test \" test"`},
		{"escaped backslash", `"a\\b"`, `"a\\b"`},
		{"newline", `"line1\nline2"`, `"line1\nline2"`},
		{"tab", `"col1\tcol2"`, `"col1\tcol2"`},
		{"mixed", `"say \"hi\"\n"`, `"say \"hi\"\n"`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Format(tt.input)
			if err != nil {
				t.Fatalf("Format error: %v", err)
			}
			result = strings.TrimSpace(result)
			if result != tt.expect {
				t.Errorf("got %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestFormatLet(t *testing.T) {
	input := "(let ((x 1)(y 2))(+ x y))"
	result, err := Format(input)
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	// Should format as multiline when appropriate
	if !strings.Contains(result, "let") {
		t.Errorf("result should contain 'let': %s", result)
	}
}

func TestFormatPreservesComments(t *testing.T) {
	input := `(do
  ; This is a comment
  (set x 1)

  ; Another comment
  (set y 2))`

	result, err := Format(input)
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	// Should preserve comments
	if !strings.Contains(result, "; This is a comment") {
		t.Errorf("comment not preserved: %s", result)
	}
	if !strings.Contains(result, "; Another comment") {
		t.Errorf("second comment not preserved: %s", result)
	}

	// Should preserve blank line
	if !strings.Contains(result, "\n\n") {
		t.Errorf("blank line not preserved: %s", result)
	}
}

func TestFormattingBlankLines(t *testing.T) {
	input := "(do\n  (set x 1)\n\n\n  (set y 2)\n  (set s \"multi\nline\nstring\"))"
	result, err := Format(input)
	if err != nil {
		t.Fatalf("Format error: %v", err)
	}

	// Should collapse multiple blank lines to one (so \n\n, not \n\n\n)
	if strings.Contains(result, "\n\n\n") {
		t.Errorf("multiple blank lines not collapsed: %s", result)
	}
	if !strings.Contains(result, "\n\n") {
		t.Errorf("single blank line expected: %s", result)
	}

	// Should preserve multi-line string exactly
	expectedString := "\"multi\nline\nstring\""
	if !strings.Contains(result, expectedString) {
		t.Errorf("multi-line string corrupted. Got:\n%s\nWant containing:\n%s", result, expectedString)
	}
}

func TestFormatIdempotent(t *testing.T) {
	inputs := []string{
		"(+ 1 2)",
		"(let ((x 1)) x)",
		"(if (> x 0) \"pos\" \"neg\")",
		"(fn (a b) (+ a b))",
		"(def foo 42)",
		"(do (set x 1) (set y 2) (+ x y))",
	}

	for _, input := range inputs {
		t.Run(input, func(t *testing.T) {
			first, err := Format(input)
			if err != nil {
				t.Fatalf("First format error: %v", err)
			}

			second, err := Format(first)
			if err != nil {
				t.Fatalf("Second format error: %v", err)
			}

			if first != second {
				t.Errorf("Format not idempotent:\nfirst:  %q\nsecond: %q", first, second)
			}
		})
	}
}

func TestFormatValue(t *testing.T) {
	tests := []struct {
		name   string
		val    Value
		expect string
	}{
		{"number", VNum(42), "42"},
		{"bool", VBool(true), "#t"},
		{"string", VString("hello"), `"hello"`},
		{"string with quote", VString(`say "hi"`), `"say \"hi\""`},
		{"empty list", VList(nil), "(list)"},
		{"simple list", VList([]Value{VNum(1), VNum(2)}), "(list 1 2)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatValue(tt.val)
			result = strings.TrimSpace(result)
			if result != tt.expect {
				t.Errorf("got %q, want %q", result, tt.expect)
			}
		})
	}
}

func TestMarshalIndent(t *testing.T) {
	type Config struct {
		Name  string `filo:"name"`
		Port  int    `filo:"port"`
		Debug bool   `filo:"debug"`
	}

	cfg := Config{Name: "app", Port: 8080, Debug: true}

	result, err := MarshalIndent(cfg, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent error: %v", err)
	}

	// Should contain the fields
	if !strings.Contains(result, "name") {
		t.Errorf("missing 'name' in result: %s", result)
	}
	if !strings.Contains(result, "8080") {
		t.Errorf("missing '8080' in result: %s", result)
	}
}

func TestMarshalIndentNested(t *testing.T) {
	type Inner struct {
		X int `filo:"x"`
		Y int `filo:"y"`
	}
	type Outer struct {
		Name  string `filo:"name"`
		Inner Inner  `filo:"inner"`
	}

	data := Outer{
		Name:  "test",
		Inner: Inner{X: 10, Y: 20},
	}

	result, err := MarshalIndent(data, "", "  ")
	if err != nil {
		t.Fatalf("MarshalIndent error: %v", err)
	}

	// Should contain nested structure
	if !strings.Contains(result, "inner") {
		t.Errorf("missing 'inner' in result: %s", result)
	}
	if !strings.Contains(result, "10") {
		t.Errorf("missing '10' in result: %s", result)
	}
}
