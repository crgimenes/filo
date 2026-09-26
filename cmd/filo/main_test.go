package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/crgimenes/filo"
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
		{[]string{"-h"}, "", 0, "usage: filo", ""},
		{nil, "", 1, "", "error: empty script"},
		{[]string{"run"}, "", 2, "", "usage: filo"},
		{[]string{"run", "x"}, "", 1, "", "filo: cannot open: x"},
		{[]string{"build", "x.filo"}, "", 2, "", "usage: filo"},
		{[]string{"bundle", "-o", "x.fbb"}, "", 2, "", "usage: filo"},
		{[]string{"dump", "-"}, "(+ 1 2)", 1, "", "filo: not a unit or a bundle"},
		{[]string{"dump", "no-such-file.fbc"}, "", 1, "", "filo: open no-such-file.fbc"},
		{[]string{"check", "a", "b"}, "", 2, "", "usage: filo"},
		{[]string{"check", "-"}, "(+ 1 2)", 1, "", "filo: not a unit or a bundle"},
		{[]string{"size", "a", "b"}, "", 2, "", "usage: filo"},
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

// Asking a command for help, anywhere in its arguments, is using it: its own
// help, on standard output, and exit 0.
func TestEachCommandsHelp(t *testing.T) {
	for _, c := range commands {
		for _, args := range [][]string{{c.name, "-h"}, {c.name, "x", "--help"}} {
			var out, errs bytes.Buffer
			code := run(args, strings.NewReader(""), &out, &errs)
			if code != 0 || errs.Len() != 0 || !strings.HasPrefix(out.String(), "usage: "+c.synopsis+"\n") {
				t.Errorf("%v: exit %d, stdout %.60q, stderr %q", args, code, out.String(), errs.String())
			}
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
	if code != 0 || lines[0] != "demo.fbb  617 bytes: 2 members, 57 of header and table" {
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
	if sum != member || member != 617-57 || strings.Contains(out.String(), "between") {
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

// run does what the C runtime's filo run does, and writes what it writes:
// a source on the tree, as bytecode, both ways, traced; a bundle's member;
// a unit that lacks something is refused naming it.
func TestRun(t *testing.T) {
	dir := t.TempDir()
	src := dir + "/sq.filo"
	_ = os.WriteFile(src, []byte("(def sq (fn (x) (* x x)))\n; comment\n(map sq (list 1 (+ 1 1) 3))\n"), 0o600)
	bad := dir + "/bad.filo"
	_ = os.WriteFile(bad, []byte("(def x 1)\n(+ x \"a\")\n"), 0o600)
	cases := []struct {
		args   []string
		code   int
		stdout string
		stderr string
	}{
		{[]string{"run", src}, 0, "(list 1 4 9)\n", ""},
		{[]string{"run", "--vm", src}, 0, "(list 1 4 9)\n", ""},
		{[]string{"run", "--both", src}, 0,
			"ir  (list 1 4 9)  (18 steps, one a node)\nvm  (list 1 4 9)  (22 steps, one an instruction)\n", ""},
		{[]string{"run", bad}, 1, "", "filo: " + bad + ":2:1: in let: in builtin \"+\": expected number, got string\n"},
		{[]string{"run", "--both", bad}, 0,
			"ir  error at 2:1: in let: in builtin \"+\": expected number, got string  (6 steps, one a node)\n" +
				"vm  error at 2:1: in builtin \"+\": expected number, got string  (6 steps, one an instruction)\n", ""},
		{[]string{"run", "../../testdata/bytecode/demo.fbb", "upper"}, 0, "HELLO\n", ""},
		{[]string{"run", "../../testdata/bytecode/prog.fbc"}, 1, "", "filo: missing (1): base\n"},
		{[]string{"run", src, "main"}, 2, "", "filo: entries are for units, and this is source: " + src + "\n"},
		{[]string{"run", dir + "/none.filo"}, 1, "", "filo: cannot open: " + dir + "/none.filo\n"},
		{[]string{"show", "folded", src}, 0, "1:1     list of 4\n1:1       symbol let\n1:1       list of 0\n" +
			"1:1       list of 3\n1:2         symbol def\n1:6         symbol sq\n1:9         list of 3\n" +
			"1:10          symbol fn\n1:13          list of 1\n1:14            symbol x\n1:17          list of 3\n" +
			"1:18            symbol *\n1:20            symbol x\n1:22            symbol x\n3:1       list of 3\n" +
			"3:2         symbol map\n3:6         symbol sq\n3:9         list of 4\n3:10          symbol list\n" +
			"3:15          number 1\n3:17          number 2\n3:25          number 3\n", ""},
		{[]string{"show", "tokens", src}, 1, "", "filo: " + src + ": no stage named tokens\n"},
	}
	for _, c := range cases {
		var out, errs bytes.Buffer
		code := run(c.args, nil, &out, &errs)
		if code != c.code || out.String() != c.stdout || errs.String() != c.stderr {
			t.Errorf("%v: exit %d\nstdout %q\nstderr %q", c.args, code, out.String(), errs.String())
		}
	}
	var out, errs bytes.Buffer
	code := run([]string{"run", "--trace", src}, nil, &out, &errs)
	lines := strings.Split(out.String(), "\n")
	if code != 0 || lines[0] != "0000  CLOSURE  1         fn 1            |" ||
		!strings.Contains(out.String(), "\n  0012  PUSH_L   0         slot 0          |\n") ||
		!strings.Contains(out.String(), "-- sq: 22 steps\n(list 1 4 9)\n") {
		t.Fatalf("trace: exit %d\n%s", code, out.String())
	}
}

// Each program of examples/ says what it gives on its last line: "; Output:
// VALUE", on the tree and as bytecode alike, or "; Error: LINE:COL:
// MESSAGE". The C repository keeps the same files and holds its filo to
// the same lines.
func TestExamples(t *testing.T) {
	paths, err := filepath.Glob("../../examples/*.filo")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no examples: %v", err)
	}
	for _, path := range paths {
		src, _ := os.ReadFile(path)
		lines := strings.Split(strings.TrimRight(string(src), "\n"), "\n")
		last := lines[len(lines)-1]
		want, output := strings.CutPrefix(last, "; Output: ")
		if output {
			for _, mode := range [][]string{{"run", path}, {"run", "--vm", path}} {
				var out, errs bytes.Buffer
				code := run(mode, nil, &out, &errs)
				if code != 0 || out.String() != want+"\n" {
					t.Errorf("%v: exit %d, %q%q, want %q", mode, code, out.String(), errs.String(), want)
				}
			}
			continue
		}
		want, failure := strings.CutPrefix(last, "; Error: ")
		if !failure {
			t.Errorf("%s: the last line says neither Output nor Error", path)
			continue
		}
		var out, errs bytes.Buffer
		code := run([]string{"run", path}, nil, &out, &errs)
		if code != 1 || errs.String() != "filo: "+path+":"+want+"\n" {
			t.Errorf("%s: exit %d, %q, want %q", path, code, errs.String(), want)
		}
	}
}

// decompile -o writes the entry points as files and their paths in the
// unit's order: built from them, the same unit but for its debug section.
func TestDecompileBuildsBack(t *testing.T) {
	dir := t.TempDir()
	var out, errs bytes.Buffer
	code := run([]string{"decompile", "-o", dir, "../../testdata/bytecode/prog.fbc"}, nil, &out, &errs)
	paths := strings.Fields(out.String())
	if code != 0 || len(paths) != 2 || !strings.HasSuffix(paths[0], "main.filo") {
		t.Fatalf("decompile: exit %d, %q, %q", code, out.String(), errs.String())
	}
	again := filepath.Join(dir, "again.fbc")
	code = run(append([]string{"build", "-o", again}, paths...), nil, &out, &errs)
	if code != 0 {
		t.Fatalf("build: %s", errs.String())
	}
	a, _ := os.ReadFile("../../testdata/bytecode/prog.fbc")
	b, _ := os.ReadFile(again)
	sa, _ := filo.StripDebug(a)
	sb, _ := filo.StripDebug(b)
	if !bytes.Equal(sa, sb) {
		t.Fatal("built back, not the same unit")
	}
	code = run([]string{"decompile", "../../testdata/bytecode/fib.filo"}, nil, &out, &errs)
	if code != 1 || !strings.Contains(errs.String(), "a source is Filo already") {
		t.Fatalf("a source: exit %d, %q", code, errs.String())
	}
}
