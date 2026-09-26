package main

import (
	"bytes"
	"strings"
	"testing"
)

// Standard input formats to what -w would write: one final newline.
func TestStdinEndsWithOneNewline(t *testing.T) {
	for _, args := range [][]string{nil, {"-fold-const"}} {
		var out, errs bytes.Buffer
		code := run(args, strings.NewReader("(+ x (* 2 3))\n"), &out, &errs)
		if code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, errs.String())
		}
		want := "(+ x (* 2 3))\n"
		if args != nil {
			want = "(+ x 6)\n"
		}
		if out.String() != want {
			t.Errorf("%v: got %q, want %q", args, out.String(), want)
		}
	}
}
