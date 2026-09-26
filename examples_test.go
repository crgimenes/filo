package filo

import (
	"bufio"
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// The examples are built in one go build, which compiles the engine once,
// and only running each is held to a deadline: a go run apiece, all at
// once, compiled it for each, and on a slow runner compiling alone ran
// past the 30 seconds.
func TestExamples(t *testing.T) {
	bin := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	build := exec.CommandContext(ctx, "go", "build", "-trimpath", "-o", bin+string(os.PathSeparator), "./examples/...")
	build.Env = append(os.Environ(), "GOWORK=off")
	out, err := build.CombinedOutput()
	if err != nil {
		t.Fatalf("building examples: %v\n%s", err, out)
	}
	err = filepath.Walk("examples", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Base(path) != "main.go" {
			return nil
		}

		t.Run(filepath.Dir(path), func(t *testing.T) {
			t.Parallel()
			exe := filepath.Join(bin, filepath.Base(filepath.Dir(path)))
			if runtime.GOOS == "windows" {
				exe += ".exe"
			}
			verifyExample(t, path, exe)
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking examples: %v", err)
	}
}

func verifyExample(t *testing.T, sourcePath, exe string) {
	// 1. Parse expected output from source
	expectedOutput, hasOutput := parseExpectedOutput(t, sourcePath)

	// 2. Run the example
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, exe) // #nosec G204 -- a binary this test just built
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("execution timed out: %v", ctx.Err())
	}
	if err != nil {
		t.Fatalf("execution failed: %v\nStderr: %s", err, stderr.String())
	}
	if !hasOutput {
		return
	}

	// 3. Compare output
	actual := strings.TrimSpace(stdout.String())
	expected := strings.TrimSpace(expectedOutput)

	if actual != expected {
		t.Errorf("output mismatch for %s:\nGot:\n%s\nWant:\n%s", sourcePath, actual, expected)
	}
}

func parseExpectedOutput(t *testing.T, path string) (string, bool) {
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open file: %v", err)
	}
	defer func() { _ = f.Close() }()

	var expected strings.Builder
	var found bool
	var inOutput bool

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if !inOutput {
			if strings.HasPrefix(trimmed, "// Output:") {
				inOutput = true
				found = true
				// Handle content on the same line "d// Output: content"
				remainder := strings.TrimPrefix(trimmed, "// Output:")
				remainder = strings.TrimSpace(remainder)
				if remainder != "" {
					expected.WriteString(remainder)
				}
			}
		} else {
			// Inside output block, read contiguous comments
			after, ok := strings.CutPrefix(trimmed, "//")
			if ok {
				// Preserve indentation by removing only the first space (comment marker)
				content := after
				after, ok = strings.CutPrefix(content, " ")
				if ok {
					content = after
				}

				if expected.Len() > 0 {
					expected.WriteString("\n")
				}
				expected.WriteString(strings.TrimRight(content, " \t"))
			} else {
				// Block ended
				break
			}
		}
	}
	return expected.String(), found
}
