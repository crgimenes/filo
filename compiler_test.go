package filo

import (
	"strings"
	"testing"
)

func mustLower(t *testing.T, src string, builtins map[string]builtinFunc, symbols *SymbolTable) *Instr {
	t.Helper()
	ast, err := Parse(src)
	if err != nil {
		t.Fatalf("parse %q: %v", src, err)
	}
	res, err := Compile(ast, builtins, symbols)
	if err != nil {
		t.Fatalf("compile %q: %v", src, err)
	}
	in, ok := res.(*Instr)
	if !ok {
		t.Fatalf("compile %q returned %T, want *Instr", src, res)
	}
	return in
}

func checkLocal(t *testing.T, in *Instr, name string, depth, index int) {
	t.Helper()
	if in.Op != OpLocal {
		t.Fatalf("%s: op = %d, want OpLocal", name, in.Op)
	}
	if in.Name != name || in.A != depth || in.B != index {
		t.Fatalf("%s: got {%s %d %d}, want {%s %d %d}", name, in.Name, in.A, in.B, name, depth, index)
	}
}

// Without a symbol table a free symbol stays a by-name lookup; with one it
// becomes an id.
func TestCompilerResolveGlobals(t *testing.T) {
	in := mustLower(t, "(do x y)", nil, nil)
	if in.Op != OpDo || len(in.Args) != 2 {
		t.Fatalf("root = %+v", in)
	}
	for i, name := range []string{"x", "y"} {
		if in.Args[i].Op != OpDynamic || in.Args[i].Name != name {
			t.Fatalf("arg %d = %+v, want dynamic %s", i, in.Args[i], name)
		}
	}

	syms := NewSymbolTable()
	in = mustLower(t, "(do x y)", nil, syms)
	for i, name := range []string{"x", "y"} {
		if in.Args[i].Op != OpGlobal || in.Args[i].Name != name || in.Args[i].A != i {
			t.Fatalf("arg %d = %+v, want global %s id %d", i, in.Args[i], name, i)
		}
	}
}

func TestCompilerResolveLocals(t *testing.T) {
	in := mustLower(t, "(let ((x 1) (y 2)) x y)", nil, nil)
	if in.Op != OpLet || in.A != 2 || len(in.Args) != 4 {
		t.Fatalf("let = %+v", in)
	}
	checkLocal(t, in.Args[2], "x", 0, 0)
	checkLocal(t, in.Args[3], "y", 0, 1)
}

func TestCompilerClosure(t *testing.T) {
	outer := mustLower(t, "(fn (a) (fn (b) a b))", nil, nil)
	if outer.Op != OpFn || len(outer.Names) != 1 || outer.Names[0] != "a" {
		t.Fatalf("outer = %+v", outer)
	}
	inner := outer.Args[0]
	if inner.Op != OpFn || len(inner.Names) != 1 || inner.Names[0] != "b" {
		t.Fatalf("inner = %+v", inner)
	}
	checkLocal(t, inner.Args[0], "a", 1, 0) // one frame up
	checkLocal(t, inner.Args[1], "b", 0, 0)
}

func TestCompilerShadowing(t *testing.T) {
	outer := mustLower(t, "(let ((x 1)) (let ((x 2)) x))", nil, nil)
	inner := outer.Args[1]
	if inner.Op != OpLet {
		t.Fatalf("inner = %+v", inner)
	}
	checkLocal(t, inner.Args[1], "x", 0, 0) // the inner x, not the outer
}

func TestCompilerBuiltinCall(t *testing.T) {
	bi := defaultBuiltins()
	in := mustLower(t, "(+ 1 2)", bi, nil)
	if in.Op != OpCallB || in.Name != "+" || in.Fn == nil || len(in.Args) != 2 {
		t.Fatalf("call = %+v", in)
	}
	if in.Args[0].Op != OpConst || in.Args[0].Val.Num != 1 {
		t.Fatalf("arg 0 = %+v", in.Args[0])
	}
	ref := mustLower(t, "+", bi, nil)
	if ref.Op != OpBuiltin || ref.Name != "+" {
		t.Fatalf("builtin as value = %+v", ref)
	}
	// A call whose head is not a builtin goes through the generic call.
	call := mustLower(t, "(f 1)", bi, nil)
	if call.Op != OpCall || call.Args[0].Op != OpDynamic {
		t.Fatalf("closure call = %+v", call)
	}
}

// A special form with the wrong shape lowers to an instruction that errors
// when evaluated, so a branch that is never taken never fails.
func TestCompilerMalformedIsLazy(t *testing.T) {
	in := mustLower(t, "(if #t 1 (let))", nil, nil)
	bad := in.Args[2]
	if bad.Op != OpInvalid || bad.Name != "let" || bad.Msg != "let expects bindings and body" {
		t.Fatalf("malformed let = %+v", bad)
	}
	cond := mustLower(t, "(cond (#t 1) 5)", nil, nil)
	if len(cond.Clauses) != 2 || !cond.Clauses[1].Invalid || cond.Clauses[0].Invalid {
		t.Fatalf("cond clauses = %+v", cond.Clauses)
	}
	def := mustLower(t, "(def 1 2)", nil, nil)
	if def.Op != OpDef || def.Msg != "def name must be symbol" {
		t.Fatalf("def = %+v", def)
	}
}

// The four shape errors the spec reports at lowering time.
func TestCompilerShapeErrors(t *testing.T) {
	cases := map[string]string{
		"(let (x) x)":               "invalid let binding",
		"(let ((1 2)) 1)":           "let binding name must be symbol",
		"(letv (1) (values 1) 1)":   "letv names must be symbols",
		"((fn (1) 1) 2)":            "fn params must be symbols",
		"(if #t 1 (let ((1 2)) 1))": "let binding name must be symbol", // even inside an untaken branch
	}
	for src, want := range cases {
		ast, err := Parse(src)
		if err != nil {
			t.Fatalf("parse %q: %v", src, err)
		}
		_, err = Compile(ast, nil, nil)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%q: error = %v, want %q", src, err, want)
		}
	}
}
