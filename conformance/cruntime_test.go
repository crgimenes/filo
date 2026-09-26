package conformance

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/fbc"
	"github.com/crgimenes/filo/filomath"
	"github.com/crgimenes/filo/filostrings"
)

// The C runtime (clang_filo) is held to the Go engine from its side, with
// Go kept on this side: its Makefile runs these tests in this module, as it
// runs TestExportOracle, and they are skipped unless asked for.

// TestExportSteps writes the steps each corpus case takes on the Go engine,
// "file<TAB>case<TAB>steps" a line: the smallest step limit it runs under.
// Only cases that run, with no given globals and no packs. The C runtime's
// IR must take the same steps (corpus_runner --steps): a step limit is
// behavior a script can see. The corpus is this repository's, the same as
// the C one.
//
//	FILO_STEPS_OUT=/path/to/steps.txt go test -run TestExportSteps -count=1 .
func TestExportSteps(t *testing.T) {
	out := os.Getenv("FILO_STEPS_OUT")
	if out == "" {
		t.Skip("set FILO_STEPS_OUT to the file to write the steps to")
	}
	paths, err := filepath.Glob("../testdata/corpus/*.txt")
	if err != nil || len(paths) == 0 {
		t.Fatalf("no corpus: %v", err)
	}
	var b strings.Builder
	for _, path := range paths {
		cases, packs, err := stepCases(path)
		if err != nil {
			t.Fatal(err)
		}
		if packs {
			continue
		}
		for _, c := range cases {
			n, ok := stepsOf(c[1])
			if ok {
				fmt.Fprintf(&b, "%s\t%s\t%d\n", filepath.Base(path), c[0], n)
			}
		}
	}
	err = os.WriteFile(out, []byte(b.String()), 0o600)
	if err != nil {
		t.Fatal(err)
	}
}

// stepCases reads a corpus file's cases, name and script, leaving out those
// with given globals or needs; packs says the file asks for packs.
func stepCases(path string) (cases [][2]string, packs bool, err error) {
	f, err := os.Open(path) // #nosec G304 -- a corpus file of this repository
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = f.Close() }()
	name, script, skip, in := "", []string{}, false, false
	flush := func() {
		if name != "" && !skip {
			cases = append(cases, [2]string{name, strings.TrimRight(strings.Join(script, "\n"), " \t\n")})
		}
	}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		l := strings.TrimRight(sc.Text(), " \t\r")
		if strings.HasPrefix(l, "packs:") && name == "" {
			packs = true
		}
		if strings.HasPrefix(l, "=== ") {
			flush()
			name, script, skip, in = l[4:], nil, false, true
			continue
		}
		if strings.HasPrefix(l, "--- ") {
			in = false
			continue
		}
		if !in {
			continue
		}
		if len(script) == 0 && (strings.HasPrefix(l, "given ") || strings.HasPrefix(l, "needs ")) {
			skip = true
			continue
		}
		if len(script) == 0 && strings.HasPrefix(l, "limits ") {
			continue
		}
		script = append(script, l)
	}
	flush()
	return cases, packs, sc.Err()
}

// stepsOf is the steps script takes, the smallest step limit it runs under;
// false when it does not run at all.
func stepsOf(script string) (int, bool) {
	p, err := filo.NewEngine().Compile(script)
	if err != nil {
		return 0, false
	}
	var n int
	_, _, err = p.Execute(context.Background(), nil, filo.EvalConfig{StepLimit: 1 << 30, RecursionLimit: 128, Steps: &n})
	return n, err == nil
}

// TestCUnits runs the units the C runtime compiled (corpus_runner
// --write-units DIR) on the Go engine's machine. Each must give what it gave
// on the C one — the result, or the error and where it happened, and the
// globals the case checks — as device_test holds the C build without a
// compiler to it; whole, and stepped one instruction at a time as a
// debugger steps it. Where dump_test kept a unit's listing (NNNNN.dump), the
// Go listing (package fbc) must be the same, byte for byte: two readers of
// the format written apart. Where corpus_runner kept the source a unit was
// compiled from (NNNNN.filo, with its packs in NNNNN.packs), the Go
// compiler must write the same unit from it, byte for byte: two compilers.
//
//	FILO_C_UNITS=/path/to/clang_filo/build/units go test -run TestCUnits -count=1 .
func TestCUnits(t *testing.T) {
	dir := os.Getenv("FILO_C_UNITS")
	if dir == "" {
		t.Skip("set FILO_C_UNITS to the directory corpus_runner --write-units wrote")
	}
	passed, failed := 0, 0
	for no := 0; ; no++ {
		expect, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d.expect", no))) // #nosec G304 -- a unit of the directory the test was given
		if err != nil {
			break
		}
		why := unitCase(dir, no, string(expect))
		if why == "" {
			why = sameListing(dir, no)
		}
		if why == "" {
			why = sameBuild(dir, no)
		}
		if why == "" {
			why = sameShow(dir, no)
		}
		if why == "" {
			passed++
			continue
		}
		failed++
		if failed <= 20 {
			t.Errorf("%s/%05d: %s", dir, no, why)
		}
	}
	t.Logf("go vm: %d passed, %d failed", passed, failed)
	if passed == 0 {
		t.Fatal("no units")
	}
}

func loadUnit(e *filo.Engine, dir string, no int, suffix string) (*filo.Unit, error) {
	data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d%s", no, suffix))) // #nosec G304 -- a unit of the directory the test was given
	if err != nil {
		return nil, err
	}
	return e.LoadUnit(data)
}

// sameStepped is "" when the run stepped one instruction at a time (a
// debugger's) gives what the whole run gave: the value and the globals, or
// the error and its place.
func sameStepped(u *filo.Unit, globals map[string]filo.Value, cfg filo.EvalConfig, want filo.Value, wantGlobals map[string]filo.Value, wantErr error) string {
	s, err := u.Start("main", globals, cfg)
	if err != nil {
		if wantErr == nil || err.Error() != wantErr.Error() {
			return fmt.Sprintf("stepped: %v, the whole run: %v", err, wantErr)
		}
		return ""
	}
	for !s.Done() {
		_ = s.Step(context.Background())
	}
	got, gotGlobals, err := s.Result()
	if (err == nil) != (wantErr == nil) {
		return fmt.Sprintf("stepped: %v, the whole run: %v", err, wantErr)
	}
	if err != nil {
		if err.Error() != wantErr.Error() || place(err) != place(wantErr) {
			return fmt.Sprintf("stepped: %v at %s, the whole run: %v at %s", err, place(err), wantErr, place(wantErr))
		}
		return ""
	}
	if repr(got) != repr(want) {
		return fmt.Sprintf("stepped gives %s, the whole run %s", repr(got), repr(want))
	}
	for name, v := range wantGlobals {
		if repr(gotGlobals[name]) != repr(v) {
			return fmt.Sprintf("stepped: global %s is %s, the whole run %s", name, repr(gotGlobals[name]), repr(v))
		}
	}
	return ""
}

func place(err error) string {
	pe, ok := errors.AsType[*filo.PositionError](err)
	if !ok {
		return "-"
	}
	return fmt.Sprintf("%d:%d", pe.Line, pe.Col)
}

// sameListing is "" when the unit lists in Go as it did in C, or when
// there is no C listing kept for it.
func sameListing(dir string, no int) string {
	want, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d.dump", no))) // #nosec G304 -- a listing of the directory the test was given
	if err != nil {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d.fbc", no))) // #nosec G304 -- a unit of the directory the test was given
	if err != nil {
		return err.Error()
	}
	u, err := fbc.Read(data)
	if err != nil {
		return "listing: " + err.Error()
	}
	var got bytes.Buffer
	err = u.Dump(&got)
	if err != nil {
		return "listing: " + err.Error()
	}
	if !bytes.Equal(got.Bytes(), want) {
		return "the Go listing differs from the C one: " + firstDifference(got.String(), string(want))
	}
	return ""
}

func firstDifference(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := 0; i < len(g) && i < len(w); i++ {
		if g[i] != w[i] {
			return fmt.Sprintf("line %d: %q, want %q", i+1, g[i], w[i])
		}
	}
	return fmt.Sprintf("%d lines, want %d", len(g), len(w))
}

// sameBuild is "" when the Go compiler writes, from the source the C one
// compiled, the unit it wrote — and the units of the given values — or
// when no source was kept.
func sameBuild(dir string, no int) string {
	packs, _ := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d.packs", no))) // #nosec G304 -- a file of the directory the test was given
	for k := -1; ; k++ {
		suffix := ""
		if k >= 0 {
			suffix = fmt.Sprintf(".g%d", k)
		}
		src, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d%s.filo", no, suffix))) // #nosec G304 -- a source of the directory the test was given
		if err != nil {
			return ""
		}
		want, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d%s.fbc", no, suffix))) // #nosec G304 -- a unit of the directory the test was given
		if err != nil {
			return err.Error()
		}
		e := filo.NewEngine()
		for p := range strings.FieldsSeq(string(packs)) {
			switch p {
			case "math":
				filomath.RegisterBuiltins(e)
			case "strings":
				filostrings.RegisterBuiltins(e)
			}
		}
		p, err := e.Compile(string(src))
		if err != nil {
			return fmt.Sprintf("build%s: %v", suffix, err)
		}
		got, err := e.Build([]filo.BuildEntry{{Name: "main", Program: p}})
		if err != nil {
			return fmt.Sprintf("build%s: %v", suffix, err)
		}
		if !bytes.Equal(got, want) {
			return fmt.Sprintf("build%s: the Go compiler wrote %d bytes, the C one %d; they differ at byte %d", suffix, len(got), len(want), firstByte(got, want))
		}
	}
}

// sameShow is "" when the Go engine shows the three stages of a unit's
// source (Engine.Show) as the C runtime's filo_show did (NNNNN.show, written
// by corpus_runner --write-units): "== tree", "== folded", "== ir", each
// stage's lines or "error".
func sameShow(dir string, no int) string {
	want, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d.show", no))) // #nosec G304 -- a file of the directory the test was given
	if err != nil {
		return ""
	}
	src, err := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d.filo", no))) // #nosec G304 -- a source of the directory the test was given
	if err != nil {
		return err.Error()
	}
	packs, _ := os.ReadFile(filepath.Join(dir, fmt.Sprintf("%05d.packs", no))) // #nosec G304 -- a file of the directory the test was given
	e := filo.NewEngine()
	for p := range strings.FieldsSeq(string(packs)) {
		switch p {
		case "math":
			filomath.RegisterBuiltins(e)
		case "strings":
			filostrings.RegisterBuiltins(e)
		}
	}
	var b strings.Builder
	for _, stage := range []string{"tree", "folded", "ir"} {
		b.WriteString("== " + stage + "\n")
		lines, err := e.Show(string(src), stage)
		if err != nil {
			b.WriteString("error\n")
			continue
		}
		for _, l := range lines {
			b.WriteString(l + "\n")
		}
	}
	if b.String() != string(want) {
		got := strings.Split(b.String(), "\n")
		kept := strings.Split(string(want), "\n")
		for i := 0; i < len(got) && i < len(kept); i++ {
			if got[i] != kept[i] {
				return fmt.Sprintf("show, line %d: %q, the C runtime %q", i+1, got[i], kept[i])
			}
		}
		return fmt.Sprintf("show: %d lines, the C runtime %d", len(got), len(kept))
	}
	return ""
}

func firstByte(a, b []byte) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return min(len(a), len(b))
}

// unitEngine has what the C runner registers: the core and the math and
// strings packs, which are packages of their own on the Go side.
func unitEngine() *filo.Engine {
	e := filo.NewEngine()
	filomath.RegisterBuiltins(e)
	filostrings.RegisterBuiltins(e)
	return e
}

// unitCase runs unit no of dir with what its .expect says it was given and
// checks what it says the run gave; "" when all agree.
func unitCase(dir string, no int, expect string) string {
	e := unitEngine()
	var cfg filo.EvalConfig
	globals := map[string]filo.Value{}
	lines := strings.Split(strings.TrimRight(expect, "\n"), "\n")
	given := 0
	i := 0
	for ; i < len(lines); i++ {
		l := lines[i]
		rest, ok := strings.CutPrefix(l, "limits ")
		if ok {
			f := strings.Fields(rest)
			if len(f) != 2 {
				return "bad limits: " + l
			}
			cfg.StepLimit, _ = strconv.Atoi(f[0])
			cfg.RecursionLimit, _ = strconv.Atoi(f[1])
			continue
		}
		name, ok := strings.CutPrefix(l, "given ")
		if !ok {
			break
		}
		u, err := loadUnit(e, dir, no, fmt.Sprintf(".g%d.fbc", given))
		if err != nil {
			return fmt.Sprintf("given %s: %v", name, err)
		}
		v, _, err := u.Run(context.Background(), "main", nil, filo.EvalConfig{})
		if err != nil {
			return fmt.Sprintf("given %s: %v", name, err)
		}
		globals[name] = v
		given++
	}
	u, err := loadUnit(e, dir, no, ".fbc")
	var got filo.Value
	var after map[string]filo.Value
	if err == nil {
		got, after, err = u.Run(context.Background(), "main", globals, cfg)
		why := sameStepped(u, globals, cfg, got, after, err)
		if why != "" {
			return why
		}
	}
	if i == len(lines) {
		return "no expectation"
	}
	for ; i < len(lines); i++ {
		why := checkExpect(lines[i], got, after, err)
		if why != "" {
			return why
		}
	}
	return ""
}

// checkExpect is "" when the run agrees with one line of an .expect:
// "error" (with "at L:C" when it says where), "result R" or "global N R".
func checkExpect(l string, got filo.Value, after map[string]filo.Value, err error) string {
	if strings.HasPrefix(l, "error") {
		if err == nil {
			return "expected an error, got " + repr(got)
		}
		at := "error"
		pe, ok := errors.AsType[*filo.PositionError](err)
		if ok {
			at = fmt.Sprintf("error at %d:%d", pe.Line, pe.Col)
		}
		if at != l {
			return fmt.Sprintf("%s, want %s (%v)", at, l, err)
		}
		return ""
	}
	if err != nil {
		return "unexpected error: " + err.Error()
	}
	want, ok := strings.CutPrefix(l, "result ")
	if ok {
		if repr(got) != want {
			return fmt.Sprintf("got %s, want %s", repr(got), want)
		}
		return ""
	}
	rest, ok := strings.CutPrefix(l, "global ")
	name, want, found := strings.Cut(rest, " ")
	if !ok || !found {
		return "bad expectation: " + l
	}
	v, held := after[name]
	if !held || repr(v) != want {
		return fmt.Sprintf("global %s is %s, want %s", name, repr(v), want)
	}
	return ""
}

// repr is how the C runtime writes a value: as Value.String does, but a
// control byte in a string is \xNN unless it is \n, \t or \r.
func repr(v filo.Value) string {
	switch v.Kind {
	case filo.KString:
		return v.String() // the one writer both engines share
	case filo.KList, filo.KTuple:
		head, items := "(list", v.List
		if v.Kind == filo.KTuple {
			head, items = "(tuple", v.Tup
		}
		var b strings.Builder
		b.WriteString(head)
		for _, e := range items {
			b.WriteByte(' ')
			b.WriteString(repr(e))
		}
		b.WriteByte(')')
		return b.String()
	}
	return v.String()
}
