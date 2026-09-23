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
		{"nested", "((foo))", "((foo))"},
		{"fits on one line", "(print-at row (floor (/ (- W (text-width text)) 2)) text)", "(print-at row (floor (/ (- W (text-width text)) 2)) text)"},
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

// The layout rule, pinned: a form that fits stays on one line; one that
// does not keeps its leading children while they fit and then gives every
// child a line; the special forms keep their heads.
func TestFormatLayout(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		expect string
	}{
		{
			"short arithmetic stays inline",
			"(centred\n  (+\n    (/ H 3) 2)\n  (str-fmt \"%d keys\" hits))",
			"(centred (+ (/ H 3) 2) (str-fmt \"%d keys\" hits))",
		},
		{
			"leading children stay, the first that does not fit breaks the rest",
			"(print-at (- H 1) 1 (str-fmt \"crg.eti.br BBS %s · %dx%d · %s · %02d:%02d:%02d\" VERSION W H USER h mn sec))",
			"(print-at (- H 1) 1\n  (str-fmt \"crg.eti.br BBS %s · %dx%d · %s · %02d:%02d:%02d\" VERSION W H USER h\n    mn\n    sec))",
		},
		{
			"fn keeps its params, the body breaks",
			"(def print-runs (fn (row col runs) (fold (fn (c run) (letv (colour text) run (do (fg colour) (+ c (print-at row c text))))) col runs)))",
			"(def print-runs\n  (fn (row col runs)\n    (fold\n      (fn (c run)\n        (letv (colour text) run (do (fg colour) (+ c (print-at row c text)))))\n      col\n      runs)))",
		},
		{
			"let bindings that do not fit stack under the first",
			"(let ((big (>= W 69)) (art (if big banner small)) (wide (if big 67 33)) (nart (length art))) (fg 2) (attr A_BOLD))",
			"(let ((big (>= W 69))\n      (art (if big banner small))\n      (wide (if big 67 33))\n      (nart (length art)))\n  (fg 2)\n  (attr A_BOLD))",
		},
		{
			"cond puts every clause on its own line",
			"(cond ((is-empty c) (set NOTE \"\")) ((chose c \"a\" \"articles\") (goto-screen \"area:/pub\" (list))) (else (set NOTE \"unknown choice (try ?)\")))",
			"(cond\n  ((is-empty c) (set NOTE \"\"))\n  ((chose c \"a\" \"articles\") (goto-screen \"area:/pub\" (list)))\n  (else (set NOTE \"unknown choice (try ?)\")))",
		},
		{
			"a list of long strings is a column",
			"(def lines (list \"Type the letter of a menu entry and press Enter.\" \"In an area, type the item number to read it.\"))",
			"(def lines\n  (list\n    \"Type the letter of a menu entry and press Enter.\"\n    \"In an area, type the item number to read it.\"))",
		},
		{
			"a comment keeps its place and rules out packing",
			"(do\n  ; first\n  (set x 1)\n  (set y 2))",
			"(do\n  ; first\n  (set x 1)\n  (set y 2))",
		},
		{
			// moved to the next line it would read as the comment of the
			// form below it
			"a comment after code stays on that line",
			"(def FOOD 9608) ; the code point of the food\n(def EMPTY 32)",
			"(def FOOD 9608) ; the code point of the food\n(def EMPTY 32)",
		},
		{
			"a trailing comment inside a form stays with its child",
			"(list 1 ; one\n  2)",
			"(list\n  1 ; one\n  2)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Format(tt.input)
			if err != nil {
				t.Fatalf("Format error: %v", err)
			}
			got = strings.TrimRight(got, "\n")
			if got != tt.expect {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.expect)
			}
			again, err := Format(got)
			if err != nil {
				t.Fatalf("second Format error: %v", err)
			}
			if strings.TrimRight(again, "\n") != got {
				t.Errorf("not idempotent:\n%s\n---\n%s", got, again)
			}
		})
	}
}

// Formatting rearranges whitespace and nothing else: the program parsed
// from the output is the program parsed from the input.
func TestFormatKeepsProgram(t *testing.T) {
	inputs := []string{
		"(def centred (fn (row text) (print-at row (floor (/ (- W (text-width text)) 2)) text)))",
		"(let ((c (str-lower (str-trim (input-text))))) (cond ((is-empty c) (set NOTE \"\")) (else (set NOTE \"unknown choice (try ?)\"))))",
		"(def single (list \"┌\" \"─\" \"┐\" \"│\" \"┘\" \"─\" \"└\" \"│\"))",
		"(if (>= W 69) banner small)\n\n(set s \"multi\nline\")",
		"(fn (i) (letv (key label) (nth items i) (key-label (+ top nart 2 i) left key label)))",
	}
	for _, input := range inputs {
		t.Run(input[:12], func(t *testing.T) {
			formatted, err := Format(input)
			if err != nil {
				t.Fatalf("Format error: %v", err)
			}
			before, err := Parse(input)
			if err != nil {
				t.Fatalf("Parse input: %v", err)
			}
			after, err := Parse(formatted)
			if err != nil {
				t.Fatalf("Parse formatted: %v\n%s", err, formatted)
			}
			cfg := DefaultFormatConfig()
			a, _ := FormatAST(before, cfg)
			b, _ := FormatAST(after, cfg)
			if a != b {
				t.Errorf("program changed by formatting:\n%s\n---\n%s", a, b)
			}
		})
	}
}
