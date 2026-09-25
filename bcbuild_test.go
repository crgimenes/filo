package filo

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"
)

// The units in testdata/bytecode are the C runtime's compiler's (the
// commands at the top of bytecode_test.go): the Go one writes the same
// bytes from the same sources.

func buildFrom(t *testing.T, e *Engine, names ...string) []byte {
	t.Helper()
	var entries []BuildEntry
	for _, n := range names {
		src, err := os.ReadFile("testdata/bytecode/" + n + ".filo")
		if err != nil {
			t.Fatal(err)
		}
		p, err := e.Compile(string(src))
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, BuildEntry{Name: n, Program: p})
	}
	unit, err := e.Build(entries)
	if err != nil {
		t.Fatal(err)
	}
	return unit
}

func sameAsFile(t *testing.T, got []byte, name string) {
	t.Helper()
	want, err := os.ReadFile("testdata/bytecode/" + name)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("%s: the Go compiler wrote %d bytes, the C one %d", name, len(got), len(want))
	}
}

func TestBuildIsTheCCompilers(t *testing.T) {
	e := NewEngine()
	prog := buildFrom(t, e, "main", "fail")
	sameAsFile(t, prog, "prog.fbc")
	sameAsFile(t, buildFrom(t, e, "fib"), "fib.fbc")
	stripped, err := StripDebug(buildFrom(t, e, "main"))
	if err != nil {
		t.Fatal(err)
	}
	sameAsFile(t, stripped, "stripped.fbc")
	again, err := StripDebug(stripped)
	if err != nil || !bytes.Equal(again, stripped) {
		t.Fatal("stripping a stripped unit changed it")
	}
	strs := NewEngine() // compiling needs str-upper to be a builtin, whatever it does
	strs.MustRegisterBuiltin("str-upper", func(_ context.Context, args []Value) (Value, error) { return args[0], nil })
	upper := buildFrom(t, strs, "upper")
	sameAsFile(t, upper, "upper.fbc")
	demo, err := BuildBundle([]BundleMember{{Name: "prog", Unit: prog}, {Name: "upper", Unit: upper}})
	if err != nil {
		t.Fatal(err)
	}
	sameAsFile(t, demo, "demo.fbb")
}

func TestBuildRefusals(t *testing.T) {
	e := NewEngine()
	p, err := e.Compile("(+ 1 2)")
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		entries []BuildEntry
		why     string
	}{
		{nil, "1 to 64 entry points"},
		{[]BuildEntry{{Name: "main", Program: nil}}, "did not compile"},
	} {
		_, err = e.Build(c.entries)
		if err == nil || !strings.Contains(err.Error(), c.why) {
			t.Errorf("got %v, want %q", err, c.why)
		}
	}
	_, err = NewEngine().Build([]BuildEntry{{Name: "main", Program: p}})
	if err == nil {
		t.Error("a program of another engine was built")
	}
	unit, err := e.Build([]BuildEntry{{Name: "main", Program: p}})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		members []BundleMember
		why     string
	}{
		{nil, "1 to 256 units"},
		{[]BundleMember{{Name: "", Unit: unit}}, "a name of 1 to 256 bytes"},
		{[]BundleMember{{Name: "a", Unit: unit}, {Name: "a", Unit: unit}}, "two bundle members named a"},
		{[]BundleMember{{Name: "a", Unit: []byte("(+ 1 2)")}}, "is not a unit"},
	} {
		_, err = BuildBundle(c.members)
		if err == nil || !strings.Contains(err.Error(), c.why) {
			t.Errorf("got %v, want %q", err, c.why)
		}
	}
	_, err = StripDebug([]byte("nope"))
	if err == nil {
		t.Error("stripped what is not a unit")
	}
}

// A NaN constant is written one way, whatever NaN the arithmetic gave.
func TestOneNaN(t *testing.T) {
	e := NewEngine()
	p, err := e.Compile(`(list (% (pow 2 2000) 3) (number "nan"))`)
	if err != nil {
		t.Fatal(err)
	}
	unit, err := e.Build([]BuildEntry{{Name: "main", Program: p}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(unit, []byte{1, 0, 0, 0, 0, 0, 0, 0xF8, 0x7F}) || bytes.Contains(unit, []byte{1, 1, 0, 0, 0, 0, 0, 0xF8, 0x7F}) {
		t.Fatal("a NaN constant is not 0x7FF8000000000000")
	}
}
