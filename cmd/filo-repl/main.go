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
	"github.com/crgimenes/filo/filostrings"
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
		foldConst      bool
	)

	fs.StringVar(&packages, "filo-package", "", "Comma-separated list of extension packages (math, rand, str)")
	fs.IntVar(&stepLimit, "step-limit", defaultStepLimit, "Maximum evaluation steps")
	fs.IntVar(&recursionLimit, "recursion-limit", defaultRecursionLimit, "Maximum recursion depth")
	fs.IntVar(&timeoutSeconds, "timeout", defaultTimeoutSeconds, "Script execution timeout in seconds")
	fs.BoolVar(&foldConst, "fold-const", false, "Enable interactive constant folding")

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

	stdinFd, err := terminalFileDescriptor(os.Stdin)
	if err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return 1
	}
	if !term.IsTerminal(stdinFd) {
		return runBatchMode(engine, stdin, stdout, stderr, cfg)
	}

	return runREPL(engine, stdinFd, stdout, stderr, cfg, foldConst)
}

func terminalFileDescriptor(file *os.File) (int, error) {
	fd := file.Fd()
	if fd > uintptr(^uint(0)>>1) {
		return 0, fmt.Errorf("file descriptor %d exceeds int range", fd)
	}
	// #nosec G115 -- the architecture-sized bound above makes this conversion safe.
	return int(fd), nil
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

func runREPL(engine *filo.Engine, stdinFd int, stdout, stderr io.Writer, cfg filo.EvalConfig, foldConst bool) int {
	// Set terminal to raw mode
	oldState, err := term.MakeRaw(stdinFd)
	if err != nil {
		fmt.Fprintf(stderr, "error: failed to set raw mode: %v\n", err)
		return 1
	}

	defer func() {
		if err := term.Restore(stdinFd, oldState); err != nil {
			fmt.Fprintf(stderr, "error: failed to restore terminal: %v\n", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigChan
		if err := term.Restore(stdinFd, oldState); err != nil {
			fmt.Fprintf(stderr, "error: failed to restore terminal: %v\n", err)
		}
		os.Exit(0)
	}()

	// Create terminal with VT100 support
	// Wrap stdin with CRLF output converter for raw mode
	crlfOut := &crlfWriter{os.Stdout}
	t := term.NewTerminal(&crlfReadWriter{os.Stdin, crlfOut}, promptMain)

	// Configure filoprint to use CRLF output for raw terminal mode
	filoprint.SetOutput(crlfOut)

	var buffer strings.Builder
	globals := make(map[string]filo.Value)

	// State for Ctrl+X prefix
	ctrlXPressed := false
	exitRequested := false

	// AutoCompleteCallback to handle Ctrl+X, E, ESC, and constant folding
	t.AutoCompleteCallback = func(line string, pos int, key rune) (newLine string, newPos int, ok bool) {
		// ESC key (0x1B) - request exit
		if key == 0x1B {
			exitRequested = true
			return "", 0, true
		}

		// Folding hook
		if foldConst && key == ')' {
			balance := 1
			startIdx := -1
			// Scan backwards for matching '(' in current line
			for i := len(line) - 1; i >= 0; i-- {
				if line[i] == ')' {
					balance++
				} else if line[i] == '(' {
					balance--
				}
				if balance == 0 {
					startIdx = i
					break
				}
			}

			if startIdx != -1 {
				snippet := line[startIdx:] + ")"
				ast, err := filo.Parse(snippet)
				if err == nil {
					folded, changed := filo.FoldConstants(ast)
					if changed {
						replacement, err := filo.FormatAST(folded, filo.FormatConfig{})
						if err == nil {
							replacement = strings.TrimSpace(replacement)
							newLine = line[:startIdx] + replacement
							newPos = len(newLine)
							return newLine, newPos, true // consume the ')'
						}
					}
				}
			}
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

				editedContent, err := openEditor(stdinFd, oldState, fullContent)
				if err != nil {
					fmt.Fprintf(t, "error: %v\n", err)
					return line, pos, true
				}
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
				if err := showHelp(stdinFd, oldState); err != nil {
					fmt.Fprintf(t, "error: %v\n", err)
				}
				continue
			case ".c", ".clear":
				buffer.Reset()
				t.SetPrompt(promptMain)
				continue
			case ".e", ".edit":
				content, err := openEditor(stdinFd, oldState, buffer.String())
				if err != nil {
					fmt.Fprintf(t, "error: %v\n", err)
					continue
				}
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

func showHelp(stdinFd int, oldState *term.State) error {
	pager := os.Getenv("PAGER")
	if pager == "" {
		pager = "less"
	}

	// Create temp file with help content
	tmpFile, err := os.CreateTemp("", "filo-help-*.txt")
	if err != nil {
		return fmt.Errorf("create help file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := tmpFile.WriteString(helpContent); err != nil {
		return fmt.Errorf("write help file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("close help file: %w", err)
	}

	if err := term.Restore(stdinFd, oldState); err != nil {
		return fmt.Errorf("restore terminal for pager: %w", err)
	}

	// #nosec G204,G702 -- PAGER intentionally selects the executable without invoking a shell.
	cmd := exec.Command(pager, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()

	_, rawErr := term.MakeRaw(stdinFd)
	if runErr != nil {
		return fmt.Errorf("run pager: %w", runErr)
	}
	if rawErr != nil {
		return fmt.Errorf("restore raw terminal mode: %w", rawErr)
	}
	return nil
}

func openEditor(stdinFd int, oldState *term.State, content string) (string, error) {
	editor := os.Getenv("EDITOR")
	if editor == "" {
		editor = "vi"
	}

	// Create temp file
	tmpFile, err := os.CreateTemp("", "filo-repl-*.filo")
	if err != nil {
		return content, fmt.Errorf("create editor file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if content != "" {
		if _, err := tmpFile.WriteString(content); err != nil {
			return content, fmt.Errorf("write editor file: %w", err)
		}
	}
	if err := tmpFile.Close(); err != nil {
		return content, fmt.Errorf("close editor file: %w", err)
	}

	if err := term.Restore(stdinFd, oldState); err != nil {
		return content, fmt.Errorf("restore terminal for editor: %w", err)
	}

	// #nosec G204,G702 -- EDITOR intentionally selects the executable without invoking a shell.
	cmd := exec.Command(editor, tmpPath)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	runErr := cmd.Run()
	_, rawErr := term.MakeRaw(stdinFd)
	if runErr != nil {
		return content, fmt.Errorf("run editor: %w", runErr)
	}
	if rawErr != nil {
		return content, fmt.Errorf("restore raw terminal mode: %w", rawErr)
	}

	// #nosec G304 -- tmpPath was created by os.CreateTemp in this function.
	data, err := os.ReadFile(tmpPath)
	if err != nil {
		return content, fmt.Errorf("read editor file: %w", err)
	}

	return string(data), nil
}

// crlfReadWriter wraps an io.ReadWriter and converts \n to \r\n on output.
// This is necessary because in raw terminal mode, \n only moves the cursor
// down without returning to column 0. The \r\n sequence properly returns
// the cursor to the first column.

// crlfWriter wraps an io.Writer and converts \n to \r\n on output.
type crlfWriter struct {
	w io.Writer
}

func (c *crlfWriter) Write(p []byte) (int, error) {
	// Convert \n to \r\n for proper terminal output in raw mode
	var out []byte
	for _, b := range p {
		if b == '\n' {
			out = append(out, '\r', '\n')
			continue
		}
		out = append(out, b)
	}
	_, err := c.w.Write(out)
	// Return original length to satisfy io.Writer contract
	return len(p), err
}

// crlfReadWriter combines a Reader with a crlfWriter for use with term.Terminal.
type crlfReadWriter struct {
	r io.Reader
	w *crlfWriter
}

func (c *crlfReadWriter) Read(p []byte) (int, error) {
	return c.r.Read(p)
}

func (c *crlfReadWriter) Write(p []byte) (int, error) {
	return c.w.Write(p)
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
		filomath.RegisterBuiltins(engine)
	case "rand":
		filorand.RegisterBuiltins(engine)
	case "str":
		filostrings.RegisterBuiltins(engine)
	case "print":
		filoprint.RegisterBuiltins(engine)
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
