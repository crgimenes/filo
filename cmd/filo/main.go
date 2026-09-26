// Command filo is the Filo toolchain on the desktop, the C runtime's filo
// command in Go: run, show, build, bundle and dump do and write what the C
// ones do and write; repl reads and evaluates (filo alone is the REPL); check
// and size look at what a unit asks for and weighs; debug steps an entry
// point on the terminal.
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

// Each command's help: filo CMD -h writes its own, filo -h all of them.
type command struct{ name, synopsis, text, examples string }

var commands = []command{
	{"repl", "filo [repl] [-filo-package LIST] [-step-limit N] [-recursion-limit N] [-timeout S] [-fold-const]", `filo alone, or filo repl, is the REPL: on a terminal it evaluates
expression by expression (.help lists its commands); with standard input a
pipe it runs what comes as one script and writes its value. The packages
are math and strings; -filo-package adds others (rand, print, json).`, `  echo '(str-upper "hello world")' | filo
  filo -filo-package rand,json`},
	{"run", "filo run [--vm | --trace | --both] FILE [MEMBER] [ENTRY...]", `run runs a program and writes its value: a source (FILE is told apart from
bytecode by the magic) on the tree the compiler lowers; --vm compiles it to
bytecode in memory first; --trace runs it as bytecode, writing each
instruction with the top of its operand stack; --both runs it both ways, a
line each, with their steps. For a unit, the ENTRY points run in order and
share their globals (default: main, or the first); for a bundle, MEMBER is
the unit (default: main, or the first).`, `  filo run --both fib.filo
  filo run prog.fbc main fail`},
	{"show", "filo show tree|folded|ir FILE", `show writes one stage of what the compiler makes of a source, each line
with the line:column it came from: the tree as read, the tree once
constants folded, or the IR, frames and slots named.`, `  filo show ir fib.filo`},
	{"build", "filo build [--strip] -o OUT FILE...", `build compiles programs into one unit of bytecode (docs/bytecode.md), each
an entry named by its file (lib/hello.filo is the entry "hello"): the same
bytes the C runtime's filo build writes. --strip leaves out the debug section
(the lines and columns errors say).`, `  filo build -o prog.fbc main.filo fail.filo`},
	{"bundle", "filo bundle -o OUT UNIT...", `bundle puts units into one bundle, each a member named by its file.`, `  filo bundle -o demo.fbb prog.fbc upper.fbc`},
	{"dump", "filo dump [FILE]", `dump lists Filo bytecode: a unit (.fbc) or a bundle (.fbb), as
docs/bytecode.md describes it — the header, the names it imports and the
globals it uses (the extern ones marked), its constants and entry points,
and every function, each instruction with its bytes and the line:column it
came from. FILE is read from standard input when absent or "-".`, `  filo dump lib/msh/edt.fbb | less`},
	{"check", "filo check [-vm PROFILE] [FILE]", `check says of each unit (a bundle's members, each) whether a VM gives what
it asks for: the functions it imports and the extern globals it reads. The
VM is this command's (the core, math and strings), or the one PROFILE lists:
the names it gives, one a line, "#" for a comment (msh's build writes the
BBS's). A line a unit: "NAME  runs: ..." or "NAME  lacks N: a, b"; the exit
status is 1 when one lacks something.`, `  filo check -vm bin.vm mine.fbb`},
	{"size", "filo size [FILE]", `size says where the bytes go: each unit's header and sections, in the
order the file has them.`, `  filo size screens.fbb`},
	{"debug", "filo debug [-src DIR] [-g NAME=EXPR]... FILE [MEMBER] [ENTRY]", `debug steps an entry point of a unit, a bundle or a source (compiled as
build compiles it): ENTRY, else main, else the first — of MEMBER, chosen the
same way, for a bundle. It runs on the terminal, in the edt's colours: the
source on the left, the function's instructions on the right, the calls
with their locals and operands below. Keys, as gdb's: s step a line (into
calls), n next line (over calls), i one instruction, c continue (to a
breakpoint, else to the end), b back (undo the last movement), r restart,
q quit; the arrows and PgUp/PgDn move a cursor in the source, and space
sets or clears a breakpoint on its line. The source of the entry "main" is
main.filo, beside FILE or in -src DIR. -g gives the run a global, the
expression in Filo: a value, or a function a unit imports and the VM lacks.`, `  filo debug fib.filo
  filo debug prog.fbc fail
  filo debug -g base=5 -g 'xs=(list 1 2 3)' prog.fbc`},
}

// help is the help of the command name, or of them all when name is "".
func help(name string) string {
	var syn, text, ex []string
	for _, c := range commands {
		if name == "" || c.name == name {
			syn = append(syn, c.synopsis)
			text = append(text, c.text)
			ex = append(ex, c.examples)
		}
	}
	return "usage: " + strings.Join(syn, "\n       ") + "\n\n" + strings.Join(text, "\n\n") +
		"\n\nExamples:\n" + strings.Join(ex, "\n") + "\n"
}

var usage = help("")

// asksHelp says whether a command's arguments ask for its help, anywhere.
func asksHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "-help" || a == "--help" {
			return true
		}
	}
	return false
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}
	if len(args) > 1 && asksHelp(args[1:]) {
		for _, c := range commands {
			if c.name == args[0] {
				_, _ = io.WriteString(stdout, help(c.name))
				return 0
			}
		}
	}
	if len(args) > 0 && args[0] == "repl" {
		return cmdRepl(args[1:], stdin, stdout, stderr)
	}
	if len(args) == 0 || strings.HasPrefix(args[0], "-") { // filo alone, or with the REPL's flags
		return cmdRepl(args, stdin, stdout, stderr)
	}
	if len(args) > 0 {
		switch args[0] {
		case "run":
			return cmdRun(args[1:], stdout, stderr)
		case "show":
			return cmdShow(args[1:], stdout, stderr)
		case "debug":
			return debug(args[1:], stderr)
		case "build":
			return cmdBuild(args[1:], stderr)
		case "bundle":
			return cmdBundle(args[1:], stderr)
		case "check":
			return cmdCheck(args[1:], stdin, stdout, stderr)
		case "size":
			return cmdSize(args[1:], stdin, stdout, stderr)
		}
	}
	if len(args) < 1 || args[0] != "dump" {
		_, _ = io.WriteString(stderr, usage)
		return 2
	}
	if len(args) > 2 {
		_, _ = io.WriteString(stderr, help("dump"))
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
	return complain(stderr, fmt.Errorf("not a unit or a bundle (filo build makes one from source)"))
}

func debug(args []string, stderr io.Writer) int {
	fs := flag.NewFlagSet("filo debug", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {}
	src := fs.String("src", "", "the directory of the sources (default: FILE's)")
	var defs repeated
	fs.Var(&defs, "g", "a global for the run, NAME=EXPR in Filo (repeatable)")
	err := fs.Parse(args)
	if err != nil || fs.NArg() < 1 || fs.NArg() > 3 {
		_, _ = io.WriteString(stderr, help("debug"))
		return 2
	}
	path := fs.Arg(0)
	data, err := os.ReadFile(path) // #nosec G304 G703 -- the file the command was given
	if err != nil {
		return complain(stderr, err)
	}
	if fbc.Kind(data) == 0 { // a source: compiled as build compiles it
		data, err = compileFiles([]string{path})
		if err != nil {
			return complain(stderr, err)
		}
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
