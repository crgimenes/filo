package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/crgimenes/filo"
)

func TestCountParens(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOpen  int
		wantClose int
	}{
		{
			name:      "simple expression",
			input:     "(+ 1 2)",
			wantOpen:  1,
			wantClose: 1,
		},
		{
			name:      "nested expression",
			input:     "(+ (* 2 3) (- 5 1))",
			wantOpen:  3,
			wantClose: 3,
		},
		{
			name:      "unbalanced open",
			input:     "(if (> x 10)",
			wantOpen:  2,
			wantClose: 1,
		},
		{
			name:      "unbalanced close",
			input:     "(+ 1 2))",
			wantOpen:  1,
			wantClose: 2,
		},
		{
			name:      "parens in string ignored",
			input:     `(print "hello (world)")`,
			wantOpen:  1,
			wantClose: 1,
		},
		{
			name:      "escaped quote in string",
			input:     `(print "say \"hi\"")`,
			wantOpen:  1,
			wantClose: 1,
		},
		{
			name:      "empty string",
			input:     "",
			wantOpen:  0,
			wantClose: 0,
		},
		{
			name:      "no parens",
			input:     "hello world",
			wantOpen:  0,
			wantClose: 0,
		},
		{
			name:      "utf8 content",
			input:     "(print \"naïve café 世界\")",
			wantOpen:  1,
			wantClose: 1,
		},
		{
			name:      "multi-line",
			input:     "(if (> x 10)\n  \"big\"\n  \"small\")",
			wantOpen:  2,
			wantClose: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			open, close := countParens(tt.input)
			if open != tt.wantOpen {
				t.Errorf("open = %d, want %d", open, tt.wantOpen)
			}
			if close != tt.wantClose {
				t.Errorf("close = %d, want %d", close, tt.wantClose)
			}
		})
	}
}

func TestBatchMode(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		stdin    string
		wantCode int
		wantOut  string
	}{
		{
			name:     "simple addition",
			args:     []string{},
			stdin:    "(+ 1 2 3)",
			wantCode: 0,
			wantOut:  "6",
		},
		{
			name:     "multiplication",
			args:     []string{},
			stdin:    "(* 2 3 4)",
			wantCode: 0,
			wantOut:  "24",
		},
		{
			name:     "boolean true",
			args:     []string{},
			stdin:    "(= 1 1)",
			wantCode: 0,
			wantOut:  "#t",
		},
		{
			name:     "boolean false",
			args:     []string{},
			stdin:    "(= 1 2)",
			wantCode: 0,
			wantOut:  "#f",
		},
		{
			name:     "string",
			args:     []string{},
			stdin:    `"hello world"`,
			wantCode: 0,
			wantOut:  "hello world",
		},
		{
			name:     "with math package",
			args:     []string{"--filo-package", "math"},
			stdin:    "(sqrt 16)",
			wantCode: 0,
			wantOut:  "4",
		},
		{
			name:     "with str package",
			args:     []string{"--filo-package", "str"},
			stdin:    `(str-upper "hello")`,
			wantCode: 0,
			wantOut:  "HELLO",
		},
		{
			name:     "multi-line script",
			args:     []string{},
			stdin:    "(let ((x 10)\n      (y 20))\n  (+ x y))",
			wantCode: 0,
			wantOut:  "30",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdin := strings.NewReader(tt.stdin)
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			code := run(tt.args, stdin, stdout, stderr)

			if code != tt.wantCode {
				t.Errorf("exit code = %d, want %d; stderr: %s", code, tt.wantCode, stderr.String())
			}

			if tt.wantOut != "" && !strings.Contains(strings.TrimSpace(stdout.String()), tt.wantOut) {
				t.Errorf("stdout = %q, want to contain %q", stdout.String(), tt.wantOut)
			}
		})
	}
}

func TestBatchModeEmpty(t *testing.T) {
	stdin := strings.NewReader("")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{}, stdin, stdout, stderr)

	if code != 1 {
		t.Errorf("expected exit code 1 for empty script, got %d", code)
	}

	if !strings.Contains(stderr.String(), "empty script") {
		t.Errorf("expected 'empty script' error, got: %s", stderr.String())
	}
}

func TestBatchModeScriptError(t *testing.T) {
	stdin := strings.NewReader("(undefined-function)")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{}, stdin, stdout, stderr)

	if code != 1 {
		t.Errorf("expected exit code 1 for script error, got %d", code)
	}

	if !strings.Contains(stderr.String(), "filo: stdin:1:2: in call: undefined global: undefined-function") {
		t.Errorf("expected the error and where, got: %s", stderr.String())
	}
}

func TestUnknownPackage(t *testing.T) {
	stdin := strings.NewReader("(+ 1 2)")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{"--filo-package", "unknown-pkg"}, stdin, stdout, stderr)

	if code != 1 {
		t.Errorf("expected exit code 1 for unknown package, got %d", code)
	}

	if !strings.Contains(stderr.String(), "unknown filo package") {
		t.Errorf("expected unknown package error, got: %s", stderr.String())
	}
}

// The REPL writes a value as filo run does: a string as its text, anything
// else as Filo reads it back.
func TestValueText(t *testing.T) {
	tests := []struct {
		name string
		v    filo.Value
		want string
	}{
		{"integer", filo.VNum(42), "42"},
		{"float", filo.VNum(3.14), "3.14"},
		{"tiny", filo.VNum(1e-10), "1e-10"},
		{"bool true", filo.VBool(true), "#t"},
		{"bool false", filo.VBool(false), "#f"},
		{"string", filo.VString("hello"), "hello"},
		{"empty list", filo.VList([]filo.Value{}), "(list)"},
		{"list", filo.VList([]filo.Value{filo.VNum(1), filo.VString("a")}), `(list 1 "a")`},
	}
	for _, tt := range tests {
		got := valueText(tt.v)
		if got != tt.want {
			t.Errorf("%s: %q, want %q", tt.name, got, tt.want)
		}
	}
}

// A script that fails says where, as filo run says it; math and strings
// are there without asking; filo repl is the same REPL, and -h its help.
func TestReplAsFiloRunIs(t *testing.T) {
	var out, errs bytes.Buffer
	code := run(nil, strings.NewReader("(def x 1)\n(+ x \"a\")"), &out, &errs)
	if code != 1 || !strings.HasPrefix(errs.String(), "filo: stdin:2:1: ") {
		t.Fatalf("exit %d, %q", code, errs.String())
	}
	out.Reset()
	code = run([]string{"repl"}, strings.NewReader(`(str-upper (string (sqrt 16)))`), &out, &errs)
	if code != 0 || out.String() != "4\n" {
		t.Fatalf("exit %d, %q %q", code, out.String(), errs.String())
	}
	out.Reset()
	code = run([]string{"repl", "-h"}, nil, &out, &errs)
	if code != 0 || !strings.HasPrefix(out.String(), "usage: filo") {
		t.Fatalf("exit %d, %q", code, out.String())
	}
}
