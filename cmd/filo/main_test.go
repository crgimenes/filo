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
		{[]string{"-h"}, "", 0, "usage: filo build", ""},
		{nil, "", 2, "", "usage: filo build"},
		{[]string{"run", "x"}, "", 2, "", "usage: filo build"},
		{[]string{"build", "x.filo"}, "", 2, "", "usage: filo build"},
		{[]string{"bundle", "-o", "x.fbb"}, "", 2, "", "usage: filo build"},
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

// build and bundle write the C runtime's bytes: the units and the bundle in
// testdata/bytecode are its filo's.
func TestBuildAndBundle(t *testing.T) {
	dir := t.TempDir()
	src := "../../testdata/bytecode/"
	var errs bytes.Buffer
	for _, c := range []struct {
		args []string
		out  string
		want string
	}{
		{[]string{"build", "-o", dir + "/prog.fbc", src + "main.filo", src + "fail.filo"}, "prog.fbc", "prog.fbc"},
		{[]string{"build", "--strip", "-o", dir + "/stripped.fbc", src + "main.filo"}, "stripped.fbc", "stripped.fbc"},
		{[]string{"build", "-o", dir + "/upper.fbc", src + "upper.filo"}, "upper.fbc", "upper.fbc"},
		{[]string{"bundle", "-o", dir + "/demo.fbb", dir + "/prog.fbc", dir + "/upper.fbc"}, "demo.fbb", "demo.fbb"},
	} {
		code := run(c.args, nil, &bytes.Buffer{}, &errs)
		got, _ := os.ReadFile(dir + "/" + c.out)
		want, _ := os.ReadFile(src + c.want)
		if code != 0 || !bytes.Equal(got, want) {
			t.Fatalf("%v: exit %d, %q; not the C bytes", c.args, code, errs.String())
		}
	}
	bad := dir + "/bad.filo"
	_ = os.WriteFile(bad, []byte("(def x 1)\n(+ x 2"), 0o600)
	code := run([]string{"build", "-o", dir + "/bad.fbc", bad}, nil, &bytes.Buffer{}, &errs)
	if code != 1 || !strings.Contains(errs.String(), "bad.filo: parse error at line 2, col 1: unterminated list") {
		t.Fatalf("exit %d, %q", code, errs.String())
	}
}
