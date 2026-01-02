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
			input:     "(print \"olá mundo 世界\")",
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

	if !strings.Contains(stderr.String(), "error") {
		t.Errorf("expected error message, got: %s", stderr.String())
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

func TestFormatResult(t *testing.T) {
	tests := []struct {
		name string
		v    func() filo.Value
		want string
	}{
		{"integer", func() filo.Value { return filo.VNum(42) }, "42"},
		{"float", func() filo.Value { return filo.VNum(3.14) }, "3.14"},
		{"bool true", func() filo.Value { return filo.VBool(true) }, "#t"},
		{"bool false", func() filo.Value { return filo.VBool(false) }, "#f"},
		{"string", func() filo.Value { return filo.VString("hello") }, "hello"},
		{"empty list", func() filo.Value { return filo.VList([]filo.Value{}) }, "()"},
		{"list", func() filo.Value {
			return filo.VList([]filo.Value{filo.VNum(1), filo.VNum(2)})
		}, "(1 2)"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := formatResult(tt.v())
			if got != tt.want {
				t.Errorf("formatResult() = %q, want %q", got, tt.want)
			}
		})
	}
}
