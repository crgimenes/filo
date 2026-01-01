// Command filo-cli runs Filo scripts with optional extension packages.
//
// Usage:
//
//	echo '(+ 1 2)' | filo-cli
//	filo-cli --script-file script.filo --filo-package math
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filomath"
)

const (
	defaultStepLimit      = 100000
	defaultRecursionLimit = 128
	defaultTimeoutSeconds = 30
)

// availablePackages lists all supported extension packages.
var availablePackages = []string{"math"}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// run executes the CLI logic and returns the exit code.
// Separated from main() for testability.
func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("filo-cli", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		scriptFile     string
		packages       string
		stepLimit      int
		recursionLimit int
		timeoutSeconds int
	)

	fs.StringVar(&scriptFile, "script-file", "", "Path to Filo script file (if omitted, reads from stdin)")
	fs.StringVar(&packages, "filo-package", "", "Comma-separated list of extension packages (math)")
	fs.IntVar(&stepLimit, "step-limit", defaultStepLimit, "Maximum evaluation steps")
	fs.IntVar(&recursionLimit, "recursion-limit", defaultRecursionLimit, "Maximum recursion depth")
	fs.IntVar(&timeoutSeconds, "timeout", defaultTimeoutSeconds, "Script execution timeout in seconds")

	if err := fs.Parse(args); err != nil {
		return 1
	}

	// Parse requested packages
	var requestedPackages []string
	if packages != "" {
		for p := range strings.SplitSeq(packages, ",") {
			p = strings.TrimSpace(p)
			if p != "" {
				requestedPackages = append(requestedPackages, p)
			}
		}
	}

	// Load script first (before potentially expensive DB operations)
	var script string
	if scriptFile != "" {
		data, err := os.ReadFile(scriptFile)
		if err != nil {
			fmt.Fprintf(stderr, "error: failed to read script file: %v\n", err)
			return 1
		}
		script = string(data)
	} else {
		data, err := io.ReadAll(stdin)
		if err != nil {
			fmt.Fprintf(stderr, "error: failed to read script from stdin: %v\n", err)
			return 1
		}
		script = string(data)
	}

	if strings.TrimSpace(script) == "" {
		fmt.Fprintln(stderr, "error: empty script")
		return 1
	}

	// Create Filo engine
	engine := filo.NewEngine()

	// Set up globals
	globals := make(map[string]filo.Value)

	// Register requested packages
	for _, pkg := range requestedPackages {
		if err := registerPackage(engine, pkg); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}

	// Configure evaluation
	cfg := filo.EvalConfig{
		StepLimit:      stepLimit,
		RecursionLimit: recursionLimit,
		Timeout:        time.Duration(timeoutSeconds) * time.Second,
	}

	// Execute script
	ctx := context.Background()
	result, _, err := engine.RunScript(ctx, script, globals, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: script execution failed: %v\n", err)
		return 1
	}

	// Print result
	fmt.Fprintln(stdout, formatResult(result))
	return 0
}

// registerPackage registers a Filo extension package by name.
func registerPackage(engine *filo.Engine, pkg string) error {
	switch pkg {
	case "math":
		filomath.RegisterMathBuiltins(engine)
	default:
		return fmt.Errorf("unknown filo package: %q (available: math)", pkg)
	}
	return nil
}

// formatResult converts a Filo value to a human-readable string.
func formatResult(v filo.Value) string {
	switch v.Kind {
	case filo.KNumber:
		// Remove trailing zeros for cleaner output
		return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%f", v.Num), "0"), ".")
	case filo.KBool:
		if v.Bool {
			return "#t"
		}
		return "#f"
	case filo.KString:
		return v.Str
	case filo.KList:
		var items []string
		for _, item := range v.List {
			items = append(items, formatResult(item))
		}
		return "(" + strings.Join(items, " ") + ")"
	case filo.KTuple:
		var items []string
		for _, item := range v.Tup {
			items = append(items, formatResult(item))
		}
		return "(values " + strings.Join(items, " ") + ")"
	case filo.KFunc:
		return "<fn>"
	default:
		return v.String()
	}
}
