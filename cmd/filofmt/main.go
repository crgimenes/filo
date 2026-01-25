// Command filofmt formats Filo source code files.
//
// Usage:
//
//	filofmt [flags] [path ...]
//
// Flags:
//
//	-w        write result to (source) file instead of stdout
//	-l        list files whose formatting differs from filofmt's
//	-d        display diffs instead of rewriting files
//	-indent N spaces per indent level (default: 2)
//
// Without an explicit path, it reads from standard input.
//
// Examples:
//
//	filofmt script.filo         # format and print to stdout
//	filofmt -w script.filo      # format in place
//	filofmt -l .                # list unformatted files
//	cat script.filo | filofmt   # format from stdin
package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/crgimenes/filo"
)

const version = "0.1.0"

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("filofmt", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		writeFlag  bool
		listFlag   bool
		diffFlag   bool
		indentSize int
		showVer    bool
	)

	fs.BoolVar(&writeFlag, "w", false, "write result to (source) file instead of stdout")
	fs.BoolVar(&listFlag, "l", false, "list files whose formatting differs from filofmt's")
	fs.BoolVar(&diffFlag, "d", false, "display diffs instead of rewriting files")
	fs.IntVar(&indentSize, "indent", 2, "spaces per indent level")
	fs.BoolVar(&showVer, "version", false, "print version and exit")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	if showVer {
		fmt.Fprintf(stdout, "filofmt version %s\n", version)
		return 0
	}

	cfg := filo.FormatConfig{
		Indent:       strings.Repeat(" ", indentSize),
		MaxLineWidth: 80,
	}

	paths := fs.Args()

	// No paths: read from stdin
	if len(paths) == 0 {
		data, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "error reading stdin: %v\n", err)
			return 1
		}
		formatted, err := filo.FormatWithConfig(string(data), cfg)
		if err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
		fmt.Fprintln(stdout, formatted)
		return 0
	}

	// Process files
	exitCode := 0
	for _, path := range paths {
		if err := processPath(path, cfg, writeFlag, listFlag, diffFlag, stdout, stderr); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			exitCode = 1
		}
	}

	return exitCode
}

func processPath(path string, cfg filo.FormatConfig, write, list, diff bool, stdout, stderr io.Writer) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if info.IsDir() {
		return filepath.Walk(path, func(p string, fi os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if fi.IsDir() {
				return nil
			}
			if !strings.HasSuffix(p, ".filo") {
				return nil
			}
			return processFile(p, cfg, write, list, diff, stdout, stderr)
		})
	}

	return processFile(path, cfg, write, list, diff, stdout, stderr)
}

func processFile(path string, cfg filo.FormatConfig, write, list, diff bool, stdout, stderr io.Writer) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	original := string(data)
	formatted, err := filo.FormatWithConfig(original, cfg)
	if err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}

	// Ensure trailing newline
	if !strings.HasSuffix(formatted, "\n") {
		formatted += "\n"
	}

	if original == formatted {
		// Already formatted
		if !write && !list && !diff {
			fmt.Fprint(stdout, formatted)
		}
		return nil
	}

	if list {
		fmt.Fprintln(stdout, path)
		return nil
	}

	if diff {
		// Simple diff: show before/after
		fmt.Fprintf(stdout, "--- %s (original)\n", path)
		fmt.Fprintf(stdout, "+++ %s (formatted)\n", path)
		printSimpleDiff(stdout, original, formatted)
		return nil
	}

	if write {
		// Write back to file
		return os.WriteFile(path, []byte(formatted), 0644)
	}

	// Default: print to stdout
	fmt.Fprint(stdout, formatted)
	return nil
}

func printSimpleDiff(w io.Writer, original, formatted string) {
	origLines := strings.Split(original, "\n")
	fmtLines := strings.Split(formatted, "\n")

	// Simple line-by-line diff
	maxLines := len(origLines)
	if len(fmtLines) > maxLines {
		maxLines = len(fmtLines)
	}

	for i := 0; i < maxLines; i++ {
		var origLine, fmtLine string
		if i < len(origLines) {
			origLine = origLines[i]
		}
		if i < len(fmtLines) {
			fmtLine = fmtLines[i]
		}

		if origLine != fmtLine {
			if origLine != "" {
				fmt.Fprintf(w, "-%s\n", origLine)
			}
			if fmtLine != "" {
				fmt.Fprintf(w, "+%s\n", fmtLine)
			}
		}
	}
}

// Ensure bytes imported (for future use)
var _ = bytes.Buffer{}
