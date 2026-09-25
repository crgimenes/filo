package main

import (
	"bytes"
	"os"
	"strconv"
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
		{[]string{"check", "a", "b"}, "", 2, "", "usage: filo build"},
		{[]string{"check", "-"}, "(+ 1 2)", 1, "", "filo: not a unit or a bundle"},
		{[]string{"size", "a", "b"}, "", 2, "", "usage: filo build"},
		{[]string{"check", "-vm", "no-such.vm", "x.fbc"}, "", 1, "", "filo: open no-such.vm"},
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

// check says what a unit asks for and a VM lacks: this command's VM, or
// one a profile lists; the exit status says whether anything lacks.
func TestCheck(t *testing.T) {
	var out, errs bytes.Buffer
	code := run([]string{"check", "../../testdata/bytecode/demo.fbb"}, nil, &out, &errs)
	if code != 1 || out.String() != "prog  lacks 1: base\nupper  runs: 1 imports, 0 externs\n" {
		t.Fatalf("exit %d:\n%s%s", code, out.String(), errs.String())
	}

	vm := t.TempDir() + "/small.vm"
	_ = os.WriteFile(vm, []byte("# a VM with little\nlist\nmap\nfold\n+\n\n*\nbase\n"), 0o600)
	out.Reset()
	code = run([]string{"check", "-vm", vm, "../../testdata/bytecode/prog.fbc"}, nil, &out, &errs)
	if code != 1 || out.String() != "prog.fbc  lacks 1: /\n" {
		t.Fatalf("exit %d: %q", code, out.String())
	}
	_ = os.WriteFile(vm, []byte("list\nmap\nfold\n+\n*\n/\nbase\n"), 0o600)
	unit, _ := os.ReadFile("../../testdata/bytecode/prog.fbc")
	out.Reset()
	code = run([]string{"check", "-vm", vm}, bytes.NewReader(unit), &out, &errs)
	if code != 0 || out.String() != "-  runs: 6 imports, 1 externs\n" {
		t.Fatalf("from stdin: exit %d: %q", code, out.String())
	}
}

// size adds up: the header and the sections are the whole unit.
func TestSize(t *testing.T) {
	var out, errs bytes.Buffer
	code := run([]string{"size", "../../testdata/bytecode/demo.fbb"}, nil, &out, &errs)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if code != 0 || lines[0] != "demo.fbb  615 bytes: 2 members, 57 of header and table" {
		t.Fatalf("exit %d:\n%s%s", code, out.String(), errs.String())
	}
	sum, member := 0, 0
	for _, l := range lines[1:] {
		f := strings.Fields(l)
		if !strings.HasPrefix(l, "  ") {
			member += atoi(t, f[1])
			continue
		}
		sum += atoi(t, f[len(f)-1])
	}
	if sum != member || member != 615-57 || strings.Contains(out.String(), "between") {
		t.Fatalf("sections %d, members %d:\n%s", sum, member, out.String())
	}
	if !strings.Contains(out.String(), "  debug          102\n") {
		t.Fatalf("no debug section:\n%s", out.String())
	}
}

func atoi(t *testing.T, s string) int {
	t.Helper()
	n, err := strconv.Atoi(s)
	if err != nil {
		t.Fatal(err)
	}
	return n
}
