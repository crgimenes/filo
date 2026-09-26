package fbc_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/fbc"
	"github.com/crgimenes/filo/filomath"
	"github.com/crgimenes/filo/filostrings"
)

func engine() *filo.Engine {
	e := filo.NewEngine()
	filomath.RegisterBuiltins(e)
	filostrings.RegisterBuiltins(e)
	return e
}

func build(t *testing.T, names []string, sources []string) []byte {
	t.Helper()
	e := engine()
	var entries []filo.BuildEntry
	for i, src := range sources {
		p, err := e.Compile(src)
		if err != nil {
			t.Fatalf("%s: %v\n%s", names[i], err, src)
		}
		entries = append(entries, filo.BuildEntry{Name: names[i], Program: p})
	}
	data, err := e.Build(entries)
	if err != nil {
		t.Fatalf("%v: %v", names, err)
	}
	return data
}

// roundTrip decompiles unit and compiles what it gives: the same unit but
// for the debug section, or the test fails saying what it read.
func roundTrip(t *testing.T, label string, unit []byte) {
	t.Helper()
	u, err := fbc.Read(unit)
	if err != nil {
		t.Fatalf("%s: %v", label, err)
	}
	srcs, err := fbc.Decompile(u)
	if err != nil {
		t.Fatalf("%s: decompile: %v", label, err)
	}
	var names, texts []string
	for _, s := range srcs {
		names = append(names, s.Name)
		texts = append(texts, s.Text)
	}
	again := build(t, names, texts)
	want, _ := filo.StripDebug(unit)
	got, _ := filo.StripDebug(again)
	if !bytes.Equal(want, got) {
		var a, b bytes.Buffer
		wu, _ := fbc.Read(want)
		gu, _ := fbc.Read(got)
		_ = wu.Dump(&a)
		_ = gu.Dump(&b)
		t.Fatalf("%s: compiled again, not the same unit\nsource:\n%s\nwant:\n%s\ngot:\n%s", label, strings.Join(texts, "\n"), a.String(), b.String())
	}
}

func TestDecompileTestdata(t *testing.T) {
	for _, name := range []string{"prog.fbc", "fib.fbc", "upper.fbc", "stripped.fbc"} {
		data, err := os.ReadFile("../testdata/bytecode/" + name)
		if err != nil {
			t.Fatal(err)
		}
		roundTrip(t, name, data)
	}
}

func TestDecompileExamples(t *testing.T) {
	paths, _ := filepath.Glob("../examples/*.filo")
	for _, p := range paths {
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		name := strings.TrimSuffix(filepath.Base(p), ".filo")
		roundTrip(t, p, build(t, []string{name}, []string{string(src)}))
	}
}

// Each form the compiler has a shape for, and the shapes that meet.
var shapes = []string{
	`(+ 1 2)`,
	`(def f (fn (x) (* x x))) (f 3)`,
	`(if (< 1 x) "a" "b")`,
	`(if (< 1 x) "a")`,
	`(cond ((< x 0) "neg") ((= x 0) "zero") (else "pos"))`,
	`(cond ((< x 0) "neg") ((= x 0) "zero"))`,
	`(cond (#t 1) (else 2))`,
	`(and a b c)`, `(or a b)`, `(and)`, `(or)`, `(and a)`,
	`(and (or a b) (and c d) e)`, `(or (and a (or b c)) d)`,
	`(let ((a 1) (b (+ a 1))) (* a b))`,
	`(let ((a 1)) (let ((b 2)) (+ a b)))`,
	`(let ((a (let ((c 5)) c))) (let ((b 1)) (+ a b)))`,
	`(let ((a (let ((c 5)) c)) (b 1)) (+ a b))`,
	`(letv (a b) (tuple 1 2) (+ a b))`, `(letv () (tuple) 1)`, `(let ((a (letv () (tuple) 1))) a)`,
	`(def g (fn (n) (let ((m 2)) (fn (k) (+ n m k))))) ((g 1) 2)`,
	`(def h (fn (n) (fn () (set n (+ n 1)) n)))`,
	`(def c 0) (set c (+ c 1)) c`,
	`(def f (fn (x) (if (< x 0) (return "neg")) (exit x)))`,
	`(def f (fn (x) (return))) (exit)`,
	`(do (print-nothing) 1)`,
	`(+ (do (f) 1) 2)`,
	`(+ (let ((a 1)) a) 2)`,
	`(if (do (f) #t) 1 2)`,
	`(map - (list 1 2))`, `(def add +) (add 1 2)`, `((do +) 1 2)`,
	`(list "q\"uote" "back\\slash" "tab\tnl\n" "\0\a\b\f\v\r" "ünï")`,
	`(list 1e400 -1e400 (- 1e400 1e400) -0 0.1 1e21 1e-7 123456789012345680)`,
	`(/ 0 0)`, `(tuple)`, `(values 1 2)`, `(list)`,
	`()`, `(do)`, `(if)`, `(let)`, `(let x 1)`, `(letv)`, `(letv x 1)`, `(fn)`, `(fn x 1)`,
	`(def)`, `(def 1 2)`, `(set)`, `(set 1 (f))`, `(exit 1 2)`, `(return 1 2)`,
	`(cond ((f) 1) 2)`, `(1 2)`, `(str-upper (str-concat "a" "b"))`, `(floor 1.5)`,
	`(fold (fn (acc x) (+ acc x)) 0 (range 10))`,
	`(def x (let ((a 1)) (set a 2) a))`,
}

func TestDecompileShapes(t *testing.T) {
	for i, src := range shapes {
		roundTrip(t, src, build(t, []string{"s" + string(rune('a'+i%26))}, []string{src}))
	}
}

// Several entries in one unit share its globals and constants: compiled
// again in the same order, the same tables.
func TestDecompileEntries(t *testing.T) {
	roundTrip(t, "three entries", build(t, []string{"one", "two", "three"},
		[]string{`(def k 5) (def f (fn (x) (+ x k)))`, `(f 1)`, `(list k "k" (f k))`}))
}

// A unit is untrusted bytes: whatever reads, decompiles or says why not,
// and never panics.
func FuzzDecompileDoesNotPanic(f *testing.F) {
	for _, name := range []string{"prog.fbc", "fib.fbc", "upper.fbc", "stripped.fbc"} {
		data, err := os.ReadFile("../testdata/bytecode/" + name)
		if err == nil {
			f.Add(data)
		}
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		u, err := fbc.Read(data)
		if err != nil {
			return
		}
		_, _ = fbc.Decompile(u)
	})
}
