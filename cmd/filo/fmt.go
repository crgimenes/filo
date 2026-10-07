package main

import (
	"bytes"
	"io"
	"os"

	"github.com/crgimenes/filo"
)

// cmdFmt is C's filo fmt: filofmt's layout, on standard output or, with -w,
// back into each file; a file already formatted is left as it is.
func cmdFmt(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	write := len(args) > 0 && args[0] == "-w"
	if write {
		args = args[1:]
		if len(args) == 0 {
			_, _ = io.WriteString(stderr, help("fmt"))
			return 2
		}
	}
	if len(args) == 0 {
		args = []string{"-"}
	}
	code := 0
	for _, path := range args {
		data, err := read(path, stdin)
		if err != nil {
			code = complain(stderr, err)
			continue
		}
		text, err := filo.Format(string(data))
		if err != nil {
			code = complain(stderr, err)
			continue
		}
		if !write {
			_, _ = io.WriteString(stdout, text)
			continue
		}
		if bytes.Equal(data, []byte(text)) {
			continue
		}
		if err := os.WriteFile(path, []byte(text), 0o600); err != nil { // #nosec G703 -- the file the command was given
			code = complain(stderr, err)
		}
	}
	return code
}
