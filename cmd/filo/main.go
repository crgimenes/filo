// Command filo is the Filo toolchain on the desktop. It has what the C
// runtime's filo command has, as the Go side gets it: dump, the listing of a
// unit (.fbc) or a bundle (.fbb), which is the C one byte for byte; and
// debug, which steps a unit's entry point on the terminal.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/crgimenes/filo/fbc"
)

const usage = `usage: filo dump [FILE]
       filo debug [-src DIR] [-g NAME=EXPR]... FILE [MEMBER] [ENTRY]

dump lists Filo bytecode: a unit (.fbc) or a bundle (.fbb), as
docs/bytecode.md describes it — the header, the names it imports and the
globals it uses (the extern ones marked), its constants and entry points,
and every function, each instruction with its bytes and the line:column it
came from. FILE is read from standard input when absent or "-".

debug steps an entry point (ENTRY, else main, else the first; for a bundle,
of MEMBER, chosen the same way) on the terminal, in the edt's colours: the
source on the left, the function's instructions on the right, the calls
with their locals and operands below. Keys, as gdb's: s step a line (into
calls), n next line (over calls), i one instruction, c continue (to a
breakpoint, else to the end), b back (undo the last movement), r restart,
q quit; the arrows and PgUp/PgDn move a cursor in the source, and space
sets or clears a breakpoint on its line. The source of the entry "main" is
main.filo, beside FILE or in -src DIR. -g gives the run a global, the
expression in Filo: a value, or a function a unit imports and the VM lacks.

Compiling is the C runtime's filo, for now (filo build -o x.fbc x.filo).

Examples:
  filo dump lib/msh/edt.fbb | less
  filo debug prog.fbc fail
  filo debug -g base=5 -g 'xs=(list 1 2 3)' prog.fbc
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}
	if len(args) > 0 && args[0] == "debug" {
		return debug(args[1:], stderr)
	}
	if len(args) < 1 || args[0] != "dump" || len(args) > 2 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	path := "-"
	if len(args) == 2 {
		path = args[1]
	}
	data, err := read(path, stdin)
	if err != nil {
		return complain(stderr, err)
	}
	return dump(data, stdout, stderr)
}

func read(path string, stdin io.Reader) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(stdin)
	}
	return os.ReadFile(path) // #nosec G304 G703 -- the file the command was given
}

func dump(data []byte, stdout, stderr io.Writer) int {
	switch fbc.Kind(data) {
	case fbc.KindBundle:
		b, err := fbc.ReadBundle(data)
		if err != nil {
			return complain(stderr, err)
		}
		err = b.Dump(stdout)
		if err != nil {
			return complain(stderr, err)
		}
		return 0
	case fbc.KindUnit:
		u, err := fbc.Read(data)
		if err != nil {
			return complain(stderr, err)
		}
		err = u.Dump(stdout)
		if err != nil {
			return complain(stderr, err)
		}
		return 0
	}
	return complain(stderr, fmt.Errorf("not a unit or a bundle (to list source, build it first with the C runtime's filo)"))
}

func debug(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("filo debug", flag.ContinueOnError)
	fs.SetOutput(stderr)
	src := fs.String("src", "", "the directory of the sources (default: FILE's)")
	var defs repeated
	fs.Var(&defs, "g", "a global for the run, NAME=EXPR in Filo (repeatable)")
	err := fs.Parse(args)
	if err != nil || fs.NArg() < 1 || fs.NArg() > 3 {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	path := fs.Arg(0)
	data, err := os.ReadFile(path) // #nosec G304 G703 -- the file the command was given
	if err != nil {
		return complain(stderr, err)
	}
	member, entry := "", fs.Arg(1)
	if fbc.Kind(data) == fbc.KindBundle {
		member, entry = fs.Arg(1), fs.Arg(2)
	}
	dir := *src
	if dir == "" {
		dir = filepath.Dir(path)
	}
	globals, err := evalGlobals(defs)
	if err != nil {
		return complain(stderr, err)
	}
	s, err := newSession(data, member, entry, dir, globals)
	if err != nil {
		return complain(stderr, err)
	}
	err = debugTerminal(s, os.Stdin, os.Stdout)
	if err != nil {
		return complain(stderr, err)
	}
	return 0
}

// repeated is a flag given more than once, each value kept in order.
type repeated []string

func (r *repeated) String() string {
	return strings.Join(*r, " ")
}

func (r *repeated) Set(v string) error {
	*r = append(*r, v)
	return nil
}

func complain(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "filo: %v\n", err)
	return 1
}
