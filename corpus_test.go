package filo_test

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/filo/filomath"
	"github.com/crgimenes/filo/filostrings"
)

// The corpus under testdata/corpus pins what a script evaluates to. It is the
// oracle every Filo runtime must pass — the C port reads the very same files —
// so this runner touches only the public API and asserts outcomes (value, error,
// resulting globals), never the text of an error message. Message wording stays
// a concern of the Go-only tests.
//
// File format (see testdata/corpus/README.md):
//
//	packs: math strings          optional header, builtin packs to register
//	=== case name                 starts a case; the name is unique in the file
//	given x = (list 1 2)          optional, input global (a Filo expression)
//	limits steps=100 recursion=5  optional, evaluation limits
//	needs host-pow                optional, a capability the host must supply
//	<script lines>
//	--- want                      the expected value, as a Filo expression
//	<expression>
//	--- error                     or: any error is expected
//	--- globals                   optional, globals expected after the run
//	x = 42

type corpusCase struct {
	name    string
	line    int
	given   []binding
	needs   []string
	cfg     filo.EvalConfig
	script  string
	want    string
	wantErr bool
	globals []binding
}

type binding struct {
	name string
	expr string
}

type corpusFile struct {
	packs []string
	cases []corpusCase
}

func TestCorpus(t *testing.T) {
	files, err := filepath.Glob(filepath.Join("testdata", "corpus", "*.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no corpus files found")
	}
	for _, path := range files {
		cf, err := parseCorpusFile(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if len(cf.cases) == 0 {
			t.Fatalf("%s: no cases", path)
		}
		base := strings.TrimSuffix(filepath.Base(path), ".txt")
		for _, c := range cf.cases {
			t.Run(base+"/"+c.name, func(t *testing.T) {
				runCorpusCase(t, cf.packs, c)
			})
		}
	}
}

func newCorpusEngine(t *testing.T, packs []string) *filo.Engine {
	t.Helper()
	eng := filo.NewEngine()
	for _, p := range packs {
		switch p {
		case "math":
			filomath.RegisterBuiltins(eng)
		case "strings":
			filostrings.RegisterBuiltins(eng)
		default:
			t.Fatalf("unknown pack %q", p)
		}
	}
	return eng
}

// evalExpr evaluates a corpus expression (an expectation or a given) on a
// fresh engine, so it can never observe the case under test.
func evalExpr(t *testing.T, packs []string, what, expr string) filo.Value {
	t.Helper()
	eng := newCorpusEngine(t, packs)
	v, _, err := eng.RunScript(context.Background(), expr, nil, filo.EvalConfig{})
	if err != nil {
		t.Fatalf("%s %q does not evaluate: %v", what, expr, err)
	}
	return v
}

func runCorpusCase(t *testing.T, packs []string, c corpusCase) {
	t.Helper()
	globals := map[string]filo.Value{}
	for _, g := range c.given {
		globals[g.name] = evalExpr(t, packs, "given", g.expr)
	}
	eng := newCorpusEngine(t, packs)
	got, newGlobals, err := eng.RunScript(context.Background(), c.script, globals, c.cfg)

	if c.wantErr {
		if err == nil {
			t.Fatalf("line %d: expected an error, got %s", c.line, got.String())
		}
		return
	}
	if err != nil {
		t.Fatalf("line %d: unexpected error: %v", c.line, err)
	}
	want := evalExpr(t, packs, "want", c.want)
	if !sameValue(got, want) {
		t.Fatalf("line %d: got %s, want %s", c.line, got.String(), want.String())
	}
	for _, g := range c.globals {
		v, ok := newGlobals[g.name]
		if !ok {
			t.Fatalf("line %d: global %q not set after the run", c.line, g.name)
		}
		want := evalExpr(t, packs, "globals", g.expr)
		if !sameValue(v, want) {
			t.Fatalf("line %d: global %q is %s, want %s", c.line, g.name, v.String(), want.String())
		}
	}
}

// sameValue is exact: no epsilon, NaN equals NaN, and functions are never
// comparable (a case must not expect one).
func sameValue(a, b filo.Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case filo.KNumber:
		if math.IsNaN(a.Num) && math.IsNaN(b.Num) {
			return true
		}
		return a.Num == b.Num
	case filo.KBool:
		return a.Bool == b.Bool
	case filo.KString:
		return a.Str == b.Str
	case filo.KList:
		return sameValues(a.List, b.List)
	case filo.KTuple:
		return sameValues(a.Tup, b.Tup)
	default:
		return false
	}
}

func sameValues(a, b []filo.Value) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if !sameValue(a[i], b[i]) {
			return false
		}
	}
	return true
}

// ---- parser ----

type section int

const (
	secNone section = iota
	secScript
	secWant
	secGlobals
)

// corpusParser is the line-by-line state of one file: the case being built
// and which section its lines currently belong to.
type corpusParser struct {
	file   *corpusFile
	seen   map[string]bool
	cur    *corpusCase
	sec    section
	script []string
	want   []string
}

func parseCorpusFile(path string) (*corpusFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	p := &corpusParser{file: &corpusFile{}, seen: map[string]bool{}}
	for i, line := range strings.Split(strings.TrimSuffix(string(data), "\n"), "\n") {
		if err := p.line(i+1, line); err != nil {
			return nil, err
		}
	}
	if err := p.flush(); err != nil {
		return nil, err
	}
	return p.file, nil
}

func (p *corpusParser) line(n int, line string) error {
	if after, ok := strings.CutPrefix(line, "=== "); ok {
		return p.startCase(n, strings.TrimSpace(after))
	}
	if after, ok := strings.CutPrefix(line, "--- "); ok {
		return p.startSection(n, strings.TrimSpace(after))
	}
	if p.cur == nil {
		return p.outsideCase(n, line)
	}
	return p.bodyLine(n, line)
}

func (p *corpusParser) startCase(n int, name string) error {
	if err := p.flush(); err != nil {
		return err
	}
	if name == "" || p.seen[name] {
		return fmt.Errorf("line %d: missing or duplicate case name %q", n, name)
	}
	p.seen[name] = true
	p.cur = &corpusCase{name: name, line: n}
	p.sec = secScript
	p.script, p.want = nil, nil
	return nil
}

func (p *corpusParser) startSection(n int, name string) error {
	if p.cur == nil {
		return fmt.Errorf("line %d: section outside a case", n)
	}
	switch name {
	case "want":
		p.sec = secWant
	case "error":
		p.cur.wantErr = true
		p.sec = secNone
	case "globals":
		p.sec = secGlobals
	default:
		return fmt.Errorf("line %d: unknown section %q", n, name)
	}
	return nil
}

func (p *corpusParser) outsideCase(n int, line string) error {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return nil
	}
	if after, ok := strings.CutPrefix(trimmed, "packs:"); ok {
		p.file.packs = strings.Fields(after)
		return nil
	}
	return fmt.Errorf("line %d: text outside a case: %q", n, line)
}

func (p *corpusParser) bodyLine(n int, line string) error {
	switch p.sec {
	case secScript:
		return p.scriptLine(n, line)
	case secWant:
		if strings.TrimSpace(line) == "" && len(p.want) > 0 {
			p.sec = secNone // the expectation ended; only comments may follow
			return nil
		}
		if len(p.want) == 0 && strings.TrimSpace(line) == "" {
			return nil
		}
		p.want = append(p.want, line)
	case secGlobals:
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			return nil
		}
		b, err := parseBinding(line)
		if err != nil {
			return fmt.Errorf("line %d: %v", n, err)
		}
		p.cur.globals = append(p.cur.globals, b)
	case secNone:
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return fmt.Errorf("line %d: text after '--- error' in case %q", n, p.cur.name)
		}
	}
	return nil
}

// Capabilities a case may ask the host for. They exist because the runtime
// runs on hosts that have no libm: a case that needs one is skipped there
// instead of counting as a disagreement between runtimes.
func knownCapability(name string) bool {
	switch name {
	case "host-pow", "host-math":
		return true
	}
	return false
}

// scriptLine handles the case header (given/limits/needs lines come before
// any script text) and then accumulates the script itself.
func (p *corpusParser) scriptLine(n int, line string) error {
	if len(p.script) == 0 && strings.HasPrefix(line, "given ") {
		b, err := parseBinding(strings.TrimPrefix(line, "given "))
		if err != nil {
			return fmt.Errorf("line %d: %v", n, err)
		}
		p.cur.given = append(p.cur.given, b)
		return nil
	}
	if len(p.script) == 0 && strings.HasPrefix(line, "needs ") {
		want := strings.TrimSpace(strings.TrimPrefix(line, "needs "))
		if !knownCapability(want) {
			return fmt.Errorf("line %d: unknown capability %q", n, want)
		}
		p.cur.needs = append(p.cur.needs, want)
		return nil
	}
	if len(p.script) == 0 && strings.HasPrefix(line, "limits ") {
		cfg, err := parseLimits(strings.TrimPrefix(line, "limits "))
		if err != nil {
			return fmt.Errorf("line %d: %v", n, err)
		}
		p.cur.cfg = cfg
		return nil
	}
	p.script = append(p.script, line)
	return nil
}

func (p *corpusParser) flush() error {
	if p.cur == nil {
		return nil
	}
	c := p.cur
	c.script = strings.TrimRight(strings.Join(p.script, "\n"), "\n")
	if strings.TrimSpace(c.script) == "" {
		return fmt.Errorf("line %d: case %q has no script", c.line, c.name)
	}
	c.want = strings.TrimSpace(strings.Join(p.want, "\n"))
	if c.wantErr == (c.want != "") {
		return fmt.Errorf("line %d: case %q needs exactly one of '--- want' or '--- error'", c.line, c.name)
	}
	p.file.cases = append(p.file.cases, *c)
	p.cur = nil
	return nil
}

func parseBinding(s string) (binding, error) {
	name, expr, ok := strings.Cut(s, "=")
	name = strings.TrimSpace(name)
	expr = strings.TrimSpace(expr)
	if !ok || name == "" || expr == "" {
		return binding{}, fmt.Errorf("binding must be 'name = expression', got %q", s)
	}
	return binding{name: name, expr: expr}, nil
}

func parseLimits(s string) (filo.EvalConfig, error) {
	var cfg filo.EvalConfig
	for field := range strings.FieldsSeq(s) {
		key, val, ok := strings.Cut(field, "=")
		if !ok {
			return cfg, fmt.Errorf("limit must be key=value, got %q", field)
		}
		n, err := strconv.Atoi(val)
		if err != nil || n <= 0 {
			return cfg, fmt.Errorf("limit %q must be a positive integer", field)
		}
		switch key {
		case "steps":
			cfg.StepLimit = n
		case "recursion":
			cfg.RecursionLimit = n
		default:
			return cfg, fmt.Errorf("unknown limit %q", key)
		}
	}
	return cfg, nil
}

// ---- the runner's own guard rails ----

func writeCorpus(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "case.txt")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// A corpus that could not fail would prove nothing: a wrong expectation, a
// missing error and a wrong global must each be caught.
func TestCorpusDetectsDisagreement(t *testing.T) {
	bad := []struct {
		name string
		body string
	}{
		{"wrong value", "=== a\n(+ 1 1)\n--- want\n3\n"},
		{"wrong kind", "=== a\n(+ 1 1)\n--- want\n\"2\"\n"},
		{"expected error did not happen", "=== a\n(+ 1 1)\n--- error\n"},
		{"unexpected error", "=== a\n(/ 1 0)\n--- want\n1\n"},
		{"wrong global", "=== a\n(set x 1)\n--- want\n1\n--- globals\nx = 2\n"},
		{"missing global", "=== a\n(+ 1 1)\n--- want\n2\n--- globals\nx = 2\n"},
		{"nan is not a number", "=== a\n(+ 1 1)\n--- want\n(number \"nan\")\n"},
	}
	for _, tc := range bad {
		t.Run(tc.name, func(t *testing.T) {
			cf, err := parseCorpusFile(writeCorpus(t, tc.body))
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			probe := &testing.T{}
			failed := runCaseCapturing(probe, cf)
			if !failed {
				t.Fatalf("runner accepted a case that must fail")
			}
		})
	}
}

// runCaseCapturing runs the single case of cf under a throwaway *testing.T and
// reports whether it failed, without failing the caller.
func runCaseCapturing(probe *testing.T, cf *corpusFile) bool {
	failed := false
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() {
			// t.Fatalf on a throwaway T calls runtime.Goexit, which is why the
			// case runs on its own goroutine.
			failed = probe.Failed()
		}()
		runCorpusCase(probe, cf.packs, cf.cases[0])
	}()
	<-done
	return failed
}

func TestCorpusFormatRejects(t *testing.T) {
	bad := map[string]string{
		"both want and error":    "=== a\n1\n--- want\n1\n--- error\n",
		"neither want nor error": "=== a\n1\n",
		"empty script":           "=== a\n--- want\n1\n",
		"duplicate name":         "=== a\n1\n--- want\n1\n=== a\n2\n--- want\n2\n",
		"empty name":             "=== \n1\n--- want\n1\n",
		"text outside a case":    "hello\n=== a\n1\n--- want\n1\n",
		"section outside a case": "--- want\n1\n",
		"unknown section":        "=== a\n1\n--- expect\n1\n",
		"text after error":       "=== a\n1\n--- error\nsome message\n",
		"text after want":        "=== a\n1\n--- want\n1\n\nstray text\n",
		"bad given":              "=== a\ngiven x\n1\n--- want\n1\n",
		"bad limit":              "=== a\nlimits steps=abc\n1\n--- want\n1\n",
		"unknown limit":          "=== a\nlimits time=5\n1\n--- want\n1\n",
		"unknown capability":     "=== a\nneeds host-gpu\n1\n--- want\n1\n",
		"bad globals line":       "=== a\n1\n--- want\n1\n--- globals\nx\n",
	}
	for name, body := range bad {
		t.Run(name, func(t *testing.T) {
			_, err := parseCorpusFile(writeCorpus(t, body))
			if err == nil {
				t.Fatalf("format accepted: %q", body)
			}
		})
	}
}

func TestCorpusFormatAccepts(t *testing.T) {
	body := "# comment\npacks: math\n\n=== one\ngiven x = 2\nlimits steps=10 recursion=3\n(* x 2)\n\n--- want\n4\n\n# a comment after the expectation\n--- globals\n\n# and inside globals\nx = 2\n\n=== two\n(/ 1 0)\n--- error\n\n# trailing comment\n"
	cf, err := parseCorpusFile(writeCorpus(t, body))
	if err != nil {
		t.Fatal(err)
	}
	if len(cf.packs) != 1 || cf.packs[0] != "math" {
		t.Fatalf("packs = %v", cf.packs)
	}
	if len(cf.cases) != 2 {
		t.Fatalf("cases = %d", len(cf.cases))
	}
	one := cf.cases[0]
	if one.script != "(* x 2)" || one.want != "4" || one.wantErr {
		t.Fatalf("case one = %+v", one)
	}
	if len(one.given) != 1 || one.given[0].name != "x" || one.given[0].expr != "2" {
		t.Fatalf("given = %+v", one.given)
	}
	if one.cfg.StepLimit != 10 || one.cfg.RecursionLimit != 3 {
		t.Fatalf("limits = %+v", one.cfg)
	}
	if len(one.globals) != 1 || one.globals[0].name != "x" {
		t.Fatalf("globals = %+v", one.globals)
	}
	if !cf.cases[1].wantErr || cf.cases[1].script != "(/ 1 0)" {
		t.Fatalf("case two = %+v", cf.cases[1])
	}
}
