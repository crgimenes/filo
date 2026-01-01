package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunSimpleScriptNoDatabase(t *testing.T) {
	// Test that simple scripts work without database flags
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
			name:     "math package pi",
			args:     []string{"--filo-package", "math"},
			stdin:    "(floor (pi))",
			wantCode: 0,
			wantOut:  "3",
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

func TestRunEmptyScript(t *testing.T) {
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

func TestRunScriptError(t *testing.T) {
	stdin := strings.NewReader("(undefined-function)")
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}

	code := run([]string{}, stdin, stdout, stderr)

	if code != 1 {
		t.Errorf("expected exit code 1 for script error, got %d", code)
	}

	if !strings.Contains(stderr.String(), "script execution failed") {
		t.Errorf("expected script error message, got: %s", stderr.String())
	}
}

func TestRunUnknownPackage(t *testing.T) {
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
