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

// This file extends the differential oracle beyond the arithmetic/logic subset
// in conformance_test.go to the parts of the language most prone to
// implementation bugs: lexical scope (let, variables, shadowing) and list
// operations (construction, head, tail, length, nth, structural equality).
//
// Values are tagged Prolog terms — num(N), bool(B), lst([...]) — so structural
// results compare exactly via prolog.TermString rendering, which the Go side
// mirrors in renderValue. To keep that rendering exact, this phase uses only
// integer numeric leaves and omits division/modulo (arithmetic over floats is
// already covered exhaustively in conformance_test.go); every value therefore
// stays an integer, a bool, or a list of those.
//
// eval(Env, E, V) relates an environment and an expression to a value; "no
// solution" is the spec's way of saying evaluating E is an error in Filo.
const extSpec = `
% Deterministic innermost-binding lookup (inner shadows outer, no backtracking
% into an outer binding).
lookup(Name, [Name-V|_], V) :- !.
lookup(Name, [_|T], V) :- lookup(Name, T, V).

% Equality requires the same kind (Filo's ensureSameKind); == is structural, so
% it matches valueEqual over ground num/bool/lst terms. Different kinds → no
% solution → error, as in Filo.
same_kind(num(_),  num(_)).
same_kind(bool(_), bool(_)).
same_kind(lst(_),  lst(_)).

eval(_, num(N), num(N)).
eval(_, bool(B), bool(B)).
eval(Env, var(Name), V) :- lookup(Name, Env, V).

% let binds one variable; the body sees it (and shadows any outer one).
eval(Env, let(Name, VE, Body), R) :-
	eval(Env, VE, V),
	eval([Name-V|Env], Body, R).

eval(Env, add(A,B), num(R)) :- eval(Env,A,num(X)), eval(Env,B,num(Y)), R is X+Y.
eval(Env, sub(A,B), num(R)) :- eval(Env,A,num(X)), eval(Env,B,num(Y)), R is X-Y.
eval(Env, mul(A,B), num(R)) :- eval(Env,A,num(X)), eval(Env,B,num(Y)), R is X*Y.

eval(Env, lt(A,B), bool(R)) :- eval(Env,A,num(X)), eval(Env,B,num(Y)), (X  <  Y -> R=true ; R=false).
eval(Env, le(A,B), bool(R)) :- eval(Env,A,num(X)), eval(Env,B,num(Y)), (X =<  Y -> R=true ; R=false).
eval(Env, gt(A,B), bool(R)) :- eval(Env,A,num(X)), eval(Env,B,num(Y)), (X  >  Y -> R=true ; R=false).
eval(Env, ge(A,B), bool(R)) :- eval(Env,A,num(X)), eval(Env,B,num(Y)), (X  >= Y -> R=true ; R=false).

eval(Env, eq(A,B), bool(R)) :-
	eval(Env,A,VA), eval(Env,B,VB), same_kind(VA,VB),
	(VA == VB -> R=true ; R=false).
eval(Env, ne(A,B), bool(R)) :-
	eval(Env, eq(A,B), bool(E)), (E == true -> R=false ; R=true).

eval(Env, not(A), bool(R)) :- eval(Env,A,bool(B)), (B == true -> R=false ; R=true).

% Short-circuit and/or: when the first argument decides, the second is never
% evaluated; every argument that IS evaluated must be a bool.
eval(Env, and(A,_), bool(false)) :- eval(Env,A,bool(false)).
eval(Env, and(A,B), bool(R))     :- eval(Env,A,bool(true)),  eval(Env,B,bool(R)).
eval(Env, or(A,_), bool(true))   :- eval(Env,A,bool(true)).
eval(Env, or(A,B), bool(R))      :- eval(Env,A,bool(false)), eval(Env,B,bool(R)).

eval(Env, if(C,T,_), R) :- eval(Env,C,bool(true)),  eval(Env,T,R).
eval(Env, if(C,_,E), R) :- eval(Env,C,bool(false)), eval(Env,E,R).

% Lists.
eval(_, list([]), lst([])).
eval(Env, list([E|Es]), lst([V|Vs])) :- eval(Env,E,V), eval(Env, list(Es), lst(Vs)).

eval(Env, head(E), V)      :- eval(Env,E,lst([V|_])).
eval(Env, tail(E), lst(T)) :- eval(Env,E,lst([_|T])).
eval(Env, length(E), num(N)) :- eval(Env,E,lst(L)), length(L, N).

% nth is 0-based and the index must evaluate to a number; this phase only ever
% produces integer indices (integer leaves, integer-preserving arithmetic), so
% no truncation is needed — Filo's int(idx) is the identity here. The
% range-check mirrors Filo's i < 0 || i >= len.
eval(Env, nth(E,I), V) :-
	eval(Env,E,lst(Xs)), eval(Env,I,num(Idx)),
	Idx >= 0, length(Xs, Len), Idx < Len, nth0(Idx, Xs, V).

eval(Env, reverse(E), lst(R)) :- eval(Env,E,lst(L)), myreverse(L, [], R).
eval(Env, range(N), lst(L))   :- eval(Env,N,num(End)), myrange(0, End, L).
eval(Env, range(A,B), lst(L)) :- eval(Env,A,num(S)), eval(Env,B,num(E)), myrange(S, E, L).

% cond: ordered clauses, first true test wins; else always matches; a non-bool
% test fails to unify with either true or false, so the whole cond has no
% solution — an error, as in Filo. No match and no else yields the empty list.
eval(_, cond([]), lst([])).
eval(Env, cond([clause(else,B)|_]), R) :- eval(Env, B, R).
eval(Env, cond([clause(T,B)|_]), R)    :- T \= else, eval(Env,T,bool(true)),  eval(Env,B,R).
eval(Env, cond([clause(T,_)|Rest]), R) :- T \= else, eval(Env,T,bool(false)), eval(Env, cond(Rest), R).

myreverse([], A, A).
myreverse([H|T], A, R) :- myreverse(T, [H|A], R).

myrange(I, End, [])       :- I >= End, !.
myrange(I, End, [num(I)|T]) :- I < End, I1 is I+1, myrange(I1, End, T).
`

// extExpr is the AST shared by the Filo and Prolog renderers.
type extExpr struct {
	op   string
	num  int
	b    bool
	name string
	kids []extExpr
}

var (
	extNumOps  = []string{"add", "sub", "mul"}
	extCmpOps  = []string{"lt", "le", "gt", "ge"}
	extFiloOp  = map[string]string{"add": "+", "sub": "-", "mul": "*", "lt": "<", "le": "<=", "gt": ">", "ge": ">=", "eq": "=", "ne": "!=", "and": "and", "or": "or", "not": "not", "if": "if", "list": "list", "head": "head", "tail": "tail", "length": "length", "nth": "nth", "reverse": "reverse", "range": "range"}
	extLeafInt = []int{-1, 0, 1, 2, 3}
)

// genExt builds a random expression up to the given depth, tracking the
// variable names in scope so it can emit var references and lets that actually
// bind.
func genExt(rng *rand.Rand, depth int, scope []string, counter *int) extExpr {
	if depth <= 0 {
		switch k := rng.Intn(6); {
		case k < 3:
			return extExpr{op: "num", num: extLeafInt[rng.Intn(len(extLeafInt))]}
		case k < 4:
			return extExpr{op: "bool", b: rng.Intn(2) == 0}
		default:
			if len(scope) > 0 {
				return extExpr{op: "var", name: scope[rng.Intn(len(scope))]}
			}
			return extExpr{op: "num", num: extLeafInt[rng.Intn(len(extLeafInt))]}
		}
	}
	sub := func(s []string) extExpr { return genExt(rng, rng.Intn(depth), s, counter) }

	switch rng.Intn(24) {
	case 20:
		return extExpr{op: "reverse", kids: []extExpr{sub(scope)}}
	case 21:
		if rng.Intn(2) == 0 {
			return extExpr{op: "range", kids: []extExpr{sub(scope)}}
		}
		return extExpr{op: "range", kids: []extExpr{sub(scope), sub(scope)}}
	case 22, 23:
		// cond: 1-3 (test body) clauses, optionally an else clause last.
		n := 1 + rng.Intn(3)
		var clauses []extExpr
		for i := 0; i < n; i++ {
			clauses = append(clauses, extExpr{op: "clause", kids: []extExpr{sub(scope), sub(scope)}})
		}
		if rng.Intn(2) == 0 {
			clauses = append(clauses, extExpr{op: "clause", name: "else", kids: []extExpr{sub(scope)}})
		}
		return extExpr{op: "cond", kids: clauses}
	case 0, 1, 2:
		return extExpr{op: extNumOps[rng.Intn(len(extNumOps))], kids: []extExpr{sub(scope), sub(scope)}}
	case 3, 4:
		return extExpr{op: extCmpOps[rng.Intn(len(extCmpOps))], kids: []extExpr{sub(scope), sub(scope)}}
	case 5:
		return extExpr{op: "eq", kids: []extExpr{sub(scope), sub(scope)}}
	case 6:
		return extExpr{op: "ne", kids: []extExpr{sub(scope), sub(scope)}}
	case 7:
		return extExpr{op: "and", kids: []extExpr{sub(scope), sub(scope)}}
	case 8:
		return extExpr{op: "or", kids: []extExpr{sub(scope), sub(scope)}}
	case 9:
		return extExpr{op: "not", kids: []extExpr{sub(scope)}}
	case 10:
		return extExpr{op: "if", kids: []extExpr{sub(scope), sub(scope), sub(scope)}}
	case 11, 12:
		// let: bind a fresh name, evaluate the body with it in scope.
		name := fmt.Sprintf("v%d", *counter)
		*counter++
		val := sub(scope)
		body := sub(append(append([]string{}, scope...), name))
		return extExpr{op: "let", name: name, kids: []extExpr{val, body}}
	case 13, 14:
		n := rng.Intn(4)
		kids := make([]extExpr, n)
		for i := range kids {
			kids[i] = sub(scope)
		}
		return extExpr{op: "list", kids: kids}
	case 15:
		return extExpr{op: "head", kids: []extExpr{sub(scope)}}
	case 16:
		return extExpr{op: "tail", kids: []extExpr{sub(scope)}}
	case 17:
		return extExpr{op: "length", kids: []extExpr{sub(scope)}}
	default:
		return extExpr{op: "nth", kids: []extExpr{sub(scope), sub(scope)}}
	}
}

func (e extExpr) filo() string {
	switch e.op {
	case "num":
		return fmt.Sprintf("%d", e.num)
	case "bool":
		if e.b {
			return "#t"
		}
		return "#f"
	case "var":
		return e.name
	case "let":
		return fmt.Sprintf("(let ((%s %s)) %s)", e.name, e.kids[0].filo(), e.kids[1].filo())
	case "cond":
		parts := []string{"cond"}
		for _, c := range e.kids {
			parts = append(parts, c.filo())
		}
		return "(" + strings.Join(parts, " ") + ")"
	case "clause":
		if e.name == "else" {
			return "(else " + e.kids[0].filo() + ")"
		}
		return "(" + e.kids[0].filo() + " " + e.kids[1].filo() + ")"
	}
	parts := []string{extFiloOp[e.op]}
	for _, k := range e.kids {
		parts = append(parts, k.filo())
	}
	return "(" + strings.Join(parts, " ") + ")"
}

func (e extExpr) prolog() string {
	switch e.op {
	case "num":
		return fmt.Sprintf("num(%d)", e.num)
	case "bool":
		if e.b {
			return "bool(true)"
		}
		return "bool(false)"
	case "var":
		return "var(" + e.name + ")"
	case "let":
		return fmt.Sprintf("let(%s,%s,%s)", e.name, e.kids[0].prolog(), e.kids[1].prolog())
	case "list":
		parts := make([]string, len(e.kids))
		for i, k := range e.kids {
			parts[i] = k.prolog()
		}
		return "list([" + strings.Join(parts, ",") + "])"
	case "cond":
		parts := make([]string, len(e.kids))
		for i, c := range e.kids {
			parts[i] = c.prolog()
		}
		return "cond([" + strings.Join(parts, ",") + "])"
	case "clause":
		if e.name == "else" {
			return "clause(else," + e.kids[0].prolog() + ")"
		}
		return "clause(" + e.kids[0].prolog() + "," + e.kids[1].prolog() + ")"
	}
	parts := make([]string, len(e.kids))
	for i, k := range e.kids {
		parts[i] = k.prolog()
	}
	return e.op + "(" + strings.Join(parts, ",") + ")"
}

// renderValue renders a Filo result value into the same canonical text
// prolog.TermString produces for the matching value term.
func renderValue(v filo.Value) string {
	list, err := v.AsList()
	if err == nil {
		parts := make([]string, len(list))
		for i, e := range list {
			parts[i] = renderValue(e)
		}
		return "lst([" + strings.Join(parts, ",") + "])"
	}
	b, err := v.AsBool()
	if err == nil {
		if b {
			return "bool(true)"
		}
		return "bool(false)"
	}
	n, err := v.AsNumber()
	if err == nil {
		if n != math.Trunc(n) {
			// Phase B stays integer-valued; a fraction here means a real
			// divergence, so surface it instead of truncating silently.
			return fmt.Sprintf("nonint(%g)", n)
		}
		return fmt.Sprintf("num(%d)", int64(n))
	}
	return "ERR"
}

func filoRender(eng *filo.Engine, src string) string {
	cfg := filo.EvalConfig{StepLimit: 100000, RecursionLimit: 64, Timeout: time.Second}
	v, _, err := eng.RunScript(context.Background(), src, nil, cfg)
	if err != nil {
		return "ERR"
	}
	return renderValue(v)
}

func prologRender(t *testing.T, p *prolog.Interpreter, term string) string {
	t.Helper()
	sols, err := p.Query("eval([], " + term + ", V).")
	if err != nil {
		t.Fatalf("oracle query %s: %v", term, err)
	}
	defer func() { _ = sols.Close() }()
	if !sols.Next() {
		return "ERR"
	}
	var s struct{ V prolog.TermString }
	err = sols.Scan(&s)
	if err != nil {
		t.Fatalf("oracle scan %s: %v", term, err)
	}
	return string(s.V)
}

// TestConformanceExtended cross-checks scope (let/variables) and list
// operations against the Prolog oracle.
func TestConformanceExtended(t *testing.T) {
	p := prolog.New(nil, nil)
	err := p.Exec(extSpec)
	if err != nil {
		t.Fatalf("loading spec: %v", err)
	}
	eng := filo.NewEngine()

	rng := rand.New(rand.NewSource(7))
	const samples = 12000
	divergences := 0
	for range samples {
		counter := 0
		e := genExt(rng, 4, nil, &counter)
		src := e.filo()
		got := filoRender(eng, src)
		want := prologRender(t, p, e.prolog())
		if got != want {
			divergences++
			if divergences <= 20 {
				t.Errorf("%-42s filo=%-22s spec=%s", src, got, want)
			}
		}
	}
	if divergences > 0 {
		t.Fatalf("%d divergences out of %d cases", divergences, samples)
	}
	t.Logf("%d extended cases agree (let/scope + list ops)", samples)
}
