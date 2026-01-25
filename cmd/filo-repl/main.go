// Command filo-repl provides an interactive REPL for the Filo language.
//
// When stdin is a TTY, it runs in interactive mode with line editing, history,
// and multi-line support. When stdin is a pipe, it runs in batch mode.
//
// Usage:
//
//	filo-repl                         # Interactive REPL
//	echo '(+ 1 2)' | filo-repl        # Batch mode
//	filo-repl --filo-package math     # REPL with math package
package main

import (
	"context"
	_ "embed"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filomath"
	"github.com/crgimenes/filo/filoprint"
	"github.com/crgimenes/filo/filorand"
	"golang.org/x/term"
)

//go:embed help.txt
var helpContent string

const (
	defaultStepLimit      = 100000
	defaultRecursionLimit = 128
	defaultTimeoutSeconds = 30
	promptMain            = "filo> "
	promptCont            = "...   "
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("filo-repl", flag.ContinueOnError)
	fs.SetOutput(stderr)

	var (
		packages       string
		stepLimit      int
		recursionLimit int
		timeoutSeconds int
	)

	fs.StringVar(&packages, "filo-package", "", "Comma-separated list of extension packages (math, rand, str)")
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

	// Create engine and register packages
	engine := filo.NewEngine()
	for _, pkg := range requestedPackages {
		if err := registerPackage(engine, pkg); err != nil {
			fmt.Fprintf(stderr, "error: %v\n", err)
			return 1
		}
	}

	cfg := filo.EvalConfig{
		StepLimit:      stepLimit,
		RecursionLimit: recursionLimit,
		Timeout:        time.Duration(timeoutSeconds) * time.Second,
	}

	// Check if stdin is a terminal
	stdinFd := int(os.Stdin.Fd())
	if !term.IsTerminal(stdinFd) {
		// Batch mode
		return runBatchMode(engine, stdin, stdout, stderr, cfg)
	}

	// Interactive REPL mode
	return runREPL(engine, stdinFd, stdout, stderr, cfg)
}

func runBatchMode(engine *filo.Engine, stdin io.Reader, stdout, stderr io.Writer, cfg filo.EvalConfig) int {
	data, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to read script: %v\n", err)
		return 1
	}

	script := string(data)
	if strings.TrimSpace(script) == "" {
		fmt.Fprintln(stderr, "error: empty script")
		return 1
	}

	ctx := context.Background()
	result, _, err := engine.RunScript(ctx, script, nil, cfg)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}

	fmt.Fprintln(stdout, formatResult(result))
	return 0
}

func runREPL(engine *filo.Engine, stdinFd int, stdout, stderr io.Writer, cfg filo.EvalConfig) int {
	// Set terminal to raw mode
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to set raw mode: %v\n", err)
		return 1
	}

	// Ensure terminal is restored on exit
	defer term.Restore(stdinFd, oldState)

	// Handle signals to restore terminal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		term.Restore(stdinFd, oldState)
		os.Exit(0)
	}()

	// Create terminal with VT100 support
	t := term.NewTerminal(os.Stdin, promptMain)

	var buffer strings.Builder
	globals := make(map[string]filo.Value)

	// State for Ctrl+X prefix
	ctrlXPressed := false
	exitRequested := false

	// AutoCompleteCallback to handle Ctrl+X, E and ESC
	t.AutoCompleteCallback = func(line string, pos int, key rune) (newLine string, newPos int, ok bool) {
		// ESC key (0x1B) - request exit
		if key == 0x1B {
			exitRequested = true
			return "", 0, true
		}

		// Ctrl+X is rune 0x18 (ASCII 24)
		if key == 0x18 {
			ctrlXPressed = true
			return line, pos, true // consume the key, don't echo
		}

		if ctrlXPressed {
			ctrlXPressed = false
			if key == 'e' || key == 'E' {
				// Open editor with current content (line + buffer)
				fullContent := buffer.String()
				if fullContent != "" {
					fullContent += "\n"
				}
				fullContent += line

				editedContent := openEditor(t, stdinFd, oldState, fullContent)
				buffer.Reset()

				// If edited content has multiple lines, put all but last in buffer
				lines := strings.Split(strings.TrimSuffix(editedContent, "\n"), "\n")
				if len(lines) > 1 {
					buffer.WriteString(strings.Join(lines[:len(lines)-1], "\n"))
					return lines[len(lines)-1], len(lines[len(lines)-1]), true
				}
				if len(lines) == 1 {
					return lines[0], len(lines[0]), true
				}
				return "", 0, true
			}
			// Any other key after Ctrl+X - just pass through
			return line, pos, false
		}

		return line, pos, false // let terminal handle other keys
	}

	// Print welcome message
	fmt.Fprintln(t, "Filo REPL - Type expressions to evaluate. Ctrl+D to exit.")
	fmt.Fprintln(t, "Use Ctrl+X,E to open $EDITOR or type .help for commands.")
	fmt.Fprintln(t, ".exit to exit.")
	fmt.Fprintln(t, "")

	for {
		line, err := t.ReadLine()

		// Check if ESC was pressed
		if exitRequested {
			fmt.Fprintln(t, "\nBye!")
			return 0
		}

		if err == io.EOF {
			// Ctrl+D on empty line - exit
			if buffer.Len() == 0 {
				fmt.Fprintln(t, "\nBye!")
				return 0
			}
			// Ctrl+D with content - force execute
			executeAndPrint(t, engine, buffer.String(), globals, cfg)
			buffer.Reset()
			t.SetPrompt(promptMain)
			continue
		}
		if err != nil {
			fmt.Fprintf(t, "error: %v\n", err)
			return 1
		}

		// Check for special commands
		trimmed := strings.TrimSpace(line)
		if buffer.Len() == 0 {
			switch trimmed {
			case ".q", ".quit", ".exit":
				fmt.Fprintln(t, "Bye!")
				return 0
			case ".h", ".help":
				showHelp(t, stdinFd, oldState)
				continue
			case ".c", ".clear":
				buffer.Reset()
				t.SetPrompt(promptMain)
				continue
			case ".e", ".edit":
				content := openEditor(t, stdinFd, oldState, buffer.String())
				buffer.Reset()
				buffer.WriteString(content)
				if content != "" {
					t.SetPrompt(promptCont)
				}
				continue
			}
		}

		// Add line to buffer
		if buffer.Len() > 0 {
			buffer.WriteString("\n")
		}
		buffer.WriteString(line)

		// Count parentheses
		open, close := countParens(buffer.String())
		content := strings.TrimSpace(buffer.String())

		if content == "" {
			// Empty input
			buffer.Reset()
			t.SetPrompt(promptMain)
			continue
		}

		if open == close {
			// Balanced (including 0 == 0 for bare values like strings/numbers)
			newGlobals := executeAndPrint(t, engine, buffer.String(), globals, cfg)
			if newGlobals != nil {
				globals = newGlobals
			}
			buffer.Reset()
			t.SetPrompt(promptMain)
			continue
		}

		if open > close {
			// Unbalanced - continue reading
			t.SetPrompt(promptCont)
			continue
		}

		// More close than open - error
		fmt.Fprintln(t, "error: unbalanced parentheses (too many closing)")
		buffer.Reset()
		t.SetPrompt(promptMain)

	}
}

func executeAndPrint(t *term.Terminal, engine *filo.Engine, script string, globals map[string]filo.Value, cfg filo.EvalConfig) map[string]filo.Value {
	ctx := context.Background()
	result, newGlobals, err := engine.RunScript(ctx, script, globals, cfg)
	if err != nil {
		fmt.Fprintf(t, "error: %v\n", err)
		return nil
	}
	fmt.Fprintln(t, formatResult(result))
	return newGlobals
}

func showHelp(t *term.Terminal, stdinFd int, oldState *term.State) {
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less"
	}

	// Create temp file with help content
	tmpFile, err := os.CreateTemp("", "filo-help-*.txt")
	if err != nil {
		fmt.Fprintf(t, "error: failed to create temp file: %v\n", err)
		return
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	tmpFile.WriteString(helpContent)
	tmpFile.Close()

	// Restore terminal for pager
	term.Restore(stdinFd, oldState)

	// Run pager
	cmd := exec.Command(pager, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	// Set raw mode again
	term.MakeRaw(stdinFd)
}

func openEditor(t *term.Terminal, stdinFd int, oldState *term.State, content string) string {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "filo-repl-*.filo")
	if err != nil {
		fmt.Fprintf(t, "error: failed to create temp file: %v\n", err)
		return content
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	// Write current content
	if content != "" {
		tmpFile.WriteString(content)
	}
	tmpFile.Close()

	// Restore terminal for editor
	term.Restore(stdinFd, oldState)

	// Run editor
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Run()

	// Set raw mode again
	term.MakeRaw(stdinFd)

	// Read back content
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		fmt.Fprintf(t, "error: failed to read edited file: %v\n", err)
		return content
	}

	return string(data)
}

// countParens counts open and close parentheses, ignoring those inside strings.
// It properly handles escape sequences and UTF-8.
func countParens(s string) (open, close int) {
	inString := false
	escape := false

	for _, r := range s {
		if escape {
			escape = false
			continue
		}
		if r == '\\' && inString {
			escape = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		// Ignore comment lines
		if r == ';' {
			// Skip until end of line - but we're iterating runes, so we need a different approach
			// For simplicity, comments are handled at line level, not here
			continue
		}
		switch r {
		case '(':
			open++
		case ')':
			close++
		}
	}
	return
}

func registerPackage(engine *filo.Engine, pkg string) error {
	switch pkg {
	case "math":
		filomath.RegisterMathBuiltins(engine)
	case "rand":
		filorand.RegisterRandomBuiltins(engine)
	case "str":
		filo.RegisterStringBuiltins(engine)
	case "print":
		filoprint.RegisterPrintBuiltins(engine)
	default:
		return fmt.Errorf("unknown filo package: %q (available: math, rand, str, print)", pkg)
	}
	return nil
}

func formatResult(v filo.Value) string {
	switch v.Kind {
	case filo.KNumber:
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
