package filo

import (
	"bufio"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestExamples(t *testing.T) {
	// Walk examples directory
	err := filepath.Walk("examples", func(path string, info os.FileInfo, err error) error {
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
			verifyExample(t, path)
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking examples: %v", err)
	}
}

func verifyExample(t *testing.T, sourcePath string) {
	// 1. Parse expected output from source
	expectedOutput, hasOutput := parseExpectedOutput(t, sourcePath)
	if !hasOutput {
		t.Skipf("no // Output: comment found in %s", sourcePath)
	}

	// 2. Run the example
	// Assumption: running from module root so 'go run examples/foo/main.go' works.
	cmd := exec.Command("go", "run", sourcePath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err != nil {
		t.Fatalf("execution failed: %v\nStderr: %s", err, stderr.String())
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
	defer f.Close()

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
			if after, ok := strings.CutPrefix(trimmed, "//"); ok {
				// Preserve indentation by removing only the first space (comment marker)
				content := after
				if after, ok := strings.CutPrefix(content, " "); ok {
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
