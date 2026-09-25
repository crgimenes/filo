// Command filo is the Filo toolchain on the desktop. It has what the C
// runtime's filo command has, as the Go side gets it: for now, dump, the
// listing of a unit (.fbc) or a bundle (.fbb), which is the C one byte for
// byte.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/crgimenes/filo/fbc"
)

const usage = `usage: filo dump [FILE]

Lists Filo bytecode: a unit (.fbc) or a bundle (.fbb), as docs/bytecode.md
describes it — the header, the names it imports and the globals it uses
(the extern ones marked), its constants and entry points, and every
function, each instruction with its bytes and the line:column it came from.
FILE is read from standard input when absent or "-". Compiling is the C
runtime's filo, for now (filo build -o x.fbc x.filo).

Example:
  filo dump lib/msh/edt.fbb | less
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) > 0 && (args[0] == "-h" || args[0] == "--help") {
		_, _ = io.WriteString(stdout, usage)
		return 0
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

func complain(stderr io.Writer, err error) int {
	_, _ = fmt.Fprintf(stderr, "filo: %v\n", err)
	return 1
}
