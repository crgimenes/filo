package conformance

import (
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/prolog"
)

// TestExportOracle writes what the Prolog spec answers for the cases the two
// tests above generate, in the corpus format, so a runtime that cannot run
// Prolog — the C one — answers to the same oracle. It runs only when asked:
//
//	FILO_ORACLE_OUT=/path/to/dir go test -run TestExportOracle -count=1 .
//
// A case is written only when the Go engine agrees with the spec exactly: the
// corpus compares without an epsilon, where the tests above allow 1e-9.
func TestExportOracle(t *testing.T) {
	dir := os.Getenv("FILO_ORACLE_OUT")
	if dir == "" {
		t.Skip("set FILO_ORACLE_OUT to a directory to write the oracle cases")
	}
	files := map[string]string{
		"arith.txt": exportArith(t),
		"scope.txt": exportScope(t),
	}
	for name, body := range files {
		err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600)
		if err != nil {
			t.Fatal(err)
		}
	}
}

const oracleHeader = `# Generated from the Prolog spec in filo/conformance (%s): do not edit.
# Regenerate in filo/conformance with
#   FILO_ORACLE_OUT=<this directory> go test -run TestExportOracle -count=1 .

`

// exportArith covers the cases of TestConformance: the same generators and
// the same seed.
func exportArith(t *testing.T) string {
	p := prolog.New(nil, nil)
	err := p.Exec(spec)
	if err != nil {
		t.Fatalf("loading spec: %v", err)
	}
	eng := filo.NewEngine()

	d1 := leaves()
	cases := append(append([]expr{}, d1...), enumerate(d1)...)
	rng := rand.New(rand.NewSource(1))
	for range 4000 {
		cases = append(cases, randExpr(rng, 3))
	}

	var b strings.Builder
	fmt.Fprintf(&b, oracleHeader, "conformance_test.go")
	for i, e := range cases {
		src := e.filo()
		want := prologOutcome(t, p, e.prolog())
		got := filoOutcome(eng, src)
		if !sameExactly(got, want) {
			t.Errorf("%s: filo=%s spec=%s", src, got, want)
			continue
		}
		fmt.Fprintf(&b, "=== a%04d\n%s\n", i, src)
		switch want.kind {
		case "num":
			fmt.Fprintf(&b, "--- want\n%s\n\n", strconv.FormatFloat(want.num, 'g', -1, 64))
		case "bool":
			fmt.Fprintf(&b, "--- want\n%s\n\n", filoBool(want.b))
		default:
			b.WriteString("--- error\n\n")
		}
	}
	return b.String()
}

// exportScope covers the cases of TestConformanceExtended.
func exportScope(t *testing.T) string {
	p := prolog.New(nil, nil)
	err := p.Exec(extSpec)
	if err != nil {
		t.Fatalf("loading spec: %v", err)
	}
	eng := filo.NewEngine()

	var b strings.Builder
	fmt.Fprintf(&b, oracleHeader, "conformance_ext_test.go")
	rng := rand.New(rand.NewSource(7))
	for i := range 12000 {
		counter := 0
		e := genExt(rng, 4, nil, &counter)
		src := e.filo()
		want := prologRender(t, p, e.prolog())
		got := filoRender(eng, src)
		if got != want {
			t.Errorf("%s: filo=%s spec=%s", src, got, want)
			continue
		}
		fmt.Fprintf(&b, "=== s%05d\n%s\n", i, src)
		if want == "ERR" {
			b.WriteString("--- error\n\n")
			continue
		}
		text, rest, err := termToFilo(want)
		if err != nil || rest != "" {
			t.Fatalf("%s: cannot read the spec's value %q", src, want)
		}
		fmt.Fprintf(&b, "--- want\n%s\n\n", text)
	}
	return b.String()
}

// sameExactly is agree without the epsilon.
func sameExactly(a, b outcome) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case "num":
		return a.num == b.num
	case "bool":
		return a.b == b.b
	}
	return true
}

func filoBool(b bool) string {
	if b {
		return "#t"
	}
	return "#f"
}

// termToFilo turns the spec's value term — num(N), bool(B), lst([...]) — into
// the Filo expression the corpus evaluates for the expected value, and returns
// what follows it.
func termToFilo(s string) (string, string, error) {
	switch {
	case strings.HasPrefix(s, "num("):
		end := strings.IndexByte(s, ')')
		if end < 0 {
			return "", "", fmt.Errorf("unterminated number in %q", s)
		}
		return s[len("num("):end], s[end+1:], nil
	case strings.HasPrefix(s, "bool(true)"):
		return "#t", s[len("bool(true)"):], nil
	case strings.HasPrefix(s, "bool(false)"):
		return "#f", s[len("bool(false)"):], nil
	case strings.HasPrefix(s, "lst(["):
		rest := s[len("lst(["):]
		parts := []string{"list"}
		for !strings.HasPrefix(rest, "])") {
			item, after, err := termToFilo(rest)
			if err != nil {
				return "", "", err
			}
			parts = append(parts, item)
			rest = strings.TrimPrefix(after, ",")
		}
		return "(" + strings.Join(parts, " ") + ")", rest[len("])"):], nil
	}
	return "", "", fmt.Errorf("unexpected term %q", s)
}
