// Package conformance cross-checks the Filo implementation against an
// independent executable specification of its semantics written in Prolog
// (github.com/crgimenes/prolog).
//
// The spec is a relational definition of evaluation: evn(E, N) holds when E
// evaluates to the number N, evb(E, B) when it evaluates to the bool B, and
// "no solution" means evaluating E is an error in Filo — so error behavior,
// short-circuit and/or, and cross-kind equality are all part of the oracle.
// The test enumerates every expression up to depth 2 exhaustively, samples
// deeper random trees, runs each one through the real engine (parser,
// constant folding, compiler, and evaluator are all in the loop), and demands
// the same outcome from both sides.
//
// This is a separate module (own go.mod) so the Filo library keeps its
// dependency-free go.mod; `go test ./...` from the repo root does not reach
// it. Run it from this directory: go test ./...
package conformance

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"strings"
	"testing"
	"time"

	"github.com/crgimenes/filo"
	"github.com/crgimenes/prolog"
)

// spec is Filo's semantics for the covered subset, as Prolog relations.
//
// Note % is specified with Prolog's `mod`, which is floored (the result takes
// the divisor's sign): (-1 % 2) is 1. That matches Lua — the language Filo
// replaces — and is Filo's settled semantics (decided 2026-07-03; the first
// conformance run surfaced the divergence when Filo still truncated).
const spec = `
evn(num(N), N).

evn(add(A,B), R) :- evn(A,X), evn(B,Y), R is X+Y.
evn(sub(A,B), R) :- evn(A,X), evn(B,Y), R is X-Y.
evn(mul(A,B), R) :- evn(A,X), evn(B,Y), R is X*Y.
evn(dvd(A,B), R) :- evn(A,X), evn(B,Y), Y =\= 0, R is float(X)/float(Y).
evn(mod(A,B), R) :- evn(A,X), evn(B,Y), Y =\= 0, R is X mod Y.

evn(if3(C,T,_), R) :- evb(C,true),  evn(T,R).
evn(if3(C,_,E), R) :- evb(C,false), evn(E,R).

evb(b(B), B).

evb(lt(A,B), R) :- evn(A,X), evn(B,Y), (X  <  Y -> R=true ; R=false).
evb(le(A,B), R) :- evn(A,X), evn(B,Y), (X =<  Y -> R=true ; R=false).
evb(gt(A,B), R) :- evn(A,X), evn(B,Y), (X  >  Y -> R=true ; R=false).
evb(ge(A,B), R) :- evn(A,X), evn(B,Y), (X  >= Y -> R=true ; R=false).

evb(eq(A,B), R) :- evn(A,X), evn(B,Y), (X =:= Y -> R=true ; R=false).
evb(eq(A,B), R) :- evb(A,X), evb(B,Y), (X ==  Y -> R=true ; R=false).
evb(ne(A,B), R) :- evb(eq(A,B), E), (E == true -> R=false ; R=true).

evb(not(A), R) :- evb(A,X), (X == true -> R=false ; R=true).

% Short-circuit: when the first argument decides, the second is NEVER
% evaluated — (or #t (/ 1 0)) is true even though the tail would error.
evb(and(A,_), false) :- evb(A,false).
evb(and(A,B), R)     :- evb(A,true), evb(B,R).
evb(or(A,_), true)   :- evb(A,true).
evb(or(A,B), R)      :- evb(A,false), evb(B,R).

evb(if3(C,T,_), R) :- evb(C,true),  evb(T,R).
evb(if3(C,_,E), R) :- evb(C,false), evb(E,R).
`

type expr struct {
	op   string // "num", "bool", or an operator name
	num  float64
	b    bool
	kids []expr
}

var binOps = []string{"add", "sub", "mul", "dvd", "mod", "lt", "le", "gt", "ge", "eq", "ne", "and", "or"}

var filoOp = map[string]string{
	"add": "+", "sub": "-", "mul": "*", "dvd": "/", "mod": "%",
	"lt": "<", "le": "<=", "gt": ">", "ge": ">=", "eq": "=", "ne": "!=",
	"and": "and", "or": "or", "not": "not", "if3": "if",
}

func leaves() []expr {
	return []expr{
		{op: "num", num: 0},
		{op: "num", num: 1},
		{op: "num", num: 2},
		{op: "num", num: -1},
		{op: "bool", b: true},
		{op: "bool", b: false},
	}
}

// enumerate builds every expression one level deeper than the given set.
func enumerate(all []expr) []expr {
	var out []expr
	for _, op := range binOps {
		for _, a := range all {
			for _, b := range all {
				out = append(out, expr{op: op, kids: []expr{a, b}})
			}
		}
	}
	for _, a := range all {
		out = append(out, expr{op: "not", kids: []expr{a}})
	}
	for _, c := range all {
		for _, t := range all {
			for _, e := range all {
				out = append(out, expr{op: "if3", kids: []expr{c, t, e}})
			}
		}
	}
	return out
}

func randExpr(rng *rand.Rand, depth int) expr {
	if depth == 0 {
		l := leaves()
		return l[rng.Intn(len(l))]
	}
	sub := func() expr { return randExpr(rng, rng.Intn(depth)) }
	k := rng.Intn(15)
	switch {
	case k < 13:
		return expr{op: binOps[k], kids: []expr{sub(), sub()}}
	case k == 13:
		return expr{op: "not", kids: []expr{sub()}}
	default:
		return expr{op: "if3", kids: []expr{sub(), sub(), sub()}}
	}
}

func (e expr) filo() string {
	switch e.op {
	case "num":
		return fmt.Sprintf("%g", e.num)
	case "bool":
		if e.b {
			return "#t"
		}
		return "#f"
	}
	parts := make([]string, 0, len(e.kids)+1)
	parts = append(parts, filoOp[e.op])
	for _, k := range e.kids {
		parts = append(parts, k.filo())
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func (e expr) prolog() string {
	switch e.op {
	case "num":
		return fmt.Sprintf("num(%g)", e.num)
	case "bool":
		if e.b {
			return "b(true)"
		}
		return "b(false)"
	}
	parts := make([]string, len(e.kids))
	for i, k := range e.kids {
		parts[i] = k.prolog()
	}
	return e.op + "(" + strings.Join(parts, ",") + ")"
}

type outcome struct {
	kind string // "num", "bool", "err"
	num  float64
	b    bool
}

func (o outcome) String() string {
	switch o.kind {
	case "num":
		return fmt.Sprintf("num %g", o.num)
	case "bool":
		return fmt.Sprintf("bool %v", o.b)
	}
	return "error"
}

func filoOutcome(eng *filo.Engine, src string) outcome {
	cfg := filo.EvalConfig{StepLimit: 10000, RecursionLimit: 32, Timeout: time.Second}
	v, _, err := eng.RunScript(context.Background(), src, nil, cfg)
	if err != nil {
		return outcome{kind: "err"}
	}
	n, err := v.AsNumber()
	if err == nil {
		return outcome{kind: "num", num: n}
	}
	b, err := v.AsBool()
	if err == nil {
		return outcome{kind: "bool", b: b}
	}
	return outcome{kind: "err"} // other kinds are outside the covered subset
}

func prologOutcome(t *testing.T, p *prolog.Interpreter, term string) outcome {
	t.Helper()

	sols, err := p.Query("evn(" + term + ", V).")
	if err != nil {
		t.Fatalf("oracle query %s: %v", term, err)
	}
	if sols.Next() {
		// Integer math yields integers, division yields floats; Scan does not
		// convert between them, so try both.
		var f struct{ V float64 }
		err = sols.Scan(&f)
		if err == nil {
			_ = sols.Close()
			return outcome{kind: "num", num: f.V}
		}
		var i struct{ V int64 }
		err = sols.Scan(&i)
		_ = sols.Close()
		if err != nil {
			t.Fatalf("oracle scan %s: %v", term, err)
		}
		return outcome{kind: "num", num: float64(i.V)}
	}
	_ = sols.Close()

	sols, err = p.Query("evb(" + term + ", V).")
	if err != nil {
		t.Fatalf("oracle query %s: %v", term, err)
	}
	defer func() { _ = sols.Close() }()
	if sols.Next() {
		var s struct{ V string }
		err = sols.Scan(&s)
		if err != nil {
			t.Fatalf("oracle scan %s: %v", term, err)
		}
		return outcome{kind: "bool", b: s.V == "true"}
	}
	return outcome{kind: "err"}
}

func agree(a, b outcome) bool {
	if a.kind != b.kind {
		return false
	}
	switch a.kind {
	case "num":
		return math.Abs(a.num-b.num) < 1e-9
	case "bool":
		return a.b == b.b
	}
	return true
}

func TestConformance(t *testing.T) {
	p := prolog.New(nil, nil)
	err := p.Exec(spec)
	if err != nil {
		t.Fatalf("loading spec: %v", err)
	}
	eng := filo.NewEngine()

	d1 := leaves()
	cases := append(append([]expr{}, d1...), enumerate(d1)...)

	// Random deep trees on top of the exhaustive shallow set. The seed is
	// fixed so a failure is reproducible.
	rng := rand.New(rand.NewSource(1))
	const deepSamples = 4000
	for range deepSamples {
		cases = append(cases, randExpr(rng, 3))
	}

	divergences := 0
	for _, e := range cases {
		src := e.filo()
		fo := filoOutcome(eng, src)
		po := prologOutcome(t, p, e.prolog())
		if !agree(fo, po) {
			divergences++
			if divergences <= 20 {
				t.Errorf("%-30s filo=%-12s spec=%s", src, fo, po)
			}
		}
	}
	if divergences > 0 {
		t.Fatalf("%d divergences out of %d cases", divergences, len(cases))
	}
	t.Logf("%d cases agree (exhaustive depth<=2 plus %d random deep trees)", len(cases), deepSamples)
}
