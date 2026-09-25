package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestDump(t *testing.T) {
	want, err := os.ReadFile("../../testdata/bytecode/prog.dump")
	if err != nil {
		t.Fatal(err)
	}
	var out, errs bytes.Buffer
	code := run([]string{"dump", "../../testdata/bytecode/prog.fbc"}, nil, &out, &errs)
	if code != 0 || !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("exit %d, stderr %q", code, errs.String())
	}

	unit, err := os.ReadFile("../../testdata/bytecode/prog.fbc")
	if err != nil {
		t.Fatal(err)
	}
	out.Reset()
	code = run([]string{"dump"}, bytes.NewReader(unit), &out, &errs)
	if code != 0 || !bytes.Equal(out.Bytes(), want) {
		t.Fatalf("from stdin: exit %d, stderr %q", code, errs.String())
	}
}

func TestUsageAndRefusals(t *testing.T) {
	cases := []struct {
		args   []string
		stdin  string
		code   int
		stdout string
		stderr string
	}{
		{[]string{"-h"}, "", 0, "usage: filo dump", ""},
		{nil, "", 2, "", "usage: filo dump"},
		{[]string{"run", "x"}, "", 2, "", "usage: filo dump"},
		{[]string{"dump", "-"}, "(+ 1 2)", 1, "", "filo: not a unit or a bundle"},
		{[]string{"dump", "no-such-file.fbc"}, "", 1, "", "filo: open no-such-file.fbc"},
	}
	for _, c := range cases {
		var out, errs bytes.Buffer
		code := run(c.args, strings.NewReader(c.stdin), &out, &errs)
		if code != c.code || !strings.HasPrefix(out.String(), c.stdout) || !strings.HasPrefix(errs.String(), c.stderr) {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", c.args, code, out.String(), errs.String())
		}
	}
}
