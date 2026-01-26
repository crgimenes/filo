package filo

import (
	"testing"
)

func TestCompilerResolveGlobals(t *testing.T) {
	// (do x y) - x and y are global
	src := "(do x y)"
	ast, _ := Parse(src)
	res, err := Compile(ast, nil, nil)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	// Helper to check if node remains Symbol
	checkGlobal := func(n Node, name string) {
		sym, ok := n.(*Symbol)
		if !ok {
			t.Errorf("expected Symbol for %s, got %T", name, n)
			return
		}
		if sym.Name != name {
			t.Errorf("expected Name %s, got %s", name, sym.Name)
		}
	}

	list := res.(*List) // (do x y) wrapped?
	// Parse returns (do x y) as List if input is just that.
	// Check Parse implementation:
	// Parse("(do x y)") returns List{do, x, y}

	// Wait, Parse returns single Node if only one expression.
	// "(do x y)" is one expression.

	// doList, ok := list.Elems[1].(*List) // Not needed for check
	// If src="(do x y)", Parse return List with 3 elems: do, x, y.
	// Oh wait, parser `Parse` wraps in `(let () ...)` ONLY if multiple top-level nodes.
	// "(do x y)" is a single top-level list. So it returns just that list.

	if list.Elems[0].(*Symbol).Name != "do" {
		t.Fatalf("expected do")
	}
	checkGlobal(list.Elems[1], "x")
	checkGlobal(list.Elems[2], "y")
}

func TestCompilerResolveLocals(t *testing.T) {
	// (let ((x 1) (y 2)) x y)
	src := "(let ((x 1) (y 2)) x y)"
	ast, _ := Parse(src)
	res, err := Compile(ast, nil, nil)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	// Structure: (let ((x 1) (y 2)) x y)
	list := res.(*List)
	// Body starts at index 2
	xUse := list.Elems[2]
	yUse := list.Elems[3]

	checkResolved := func(n Node, name string, depth, index int) {
		rs, ok := n.(*ResolvedSymbol)
		if !ok {
			t.Errorf("expected ResolvedSymbol for %s, got %T", name, n)
			return
		}
		if rs.Name != name {
			t.Errorf("expected Name %s, got %s", name, rs.Name)
		}
		if rs.Depth != depth {
			t.Errorf("expected Depth %d for %s, got %d", depth, name, rs.Depth)
		}
		if rs.Index != index {
			t.Errorf("expected Index %d for %s, got %d", index, name, rs.Index)
		}
	}

	checkResolved(xUse, "x", 0, 0)
	checkResolved(yUse, "y", 0, 1) // y is 2nd binding
}

func TestCompilerClosure(t *testing.T) {
	// (fn (a) (fn (b) a b))
	src := "(fn (a) (fn (b) a b))"
	ast, _ := Parse(src)
	res, err := Compile(ast, nil, nil)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	// fn -> list
	fn1 := res.(*List)
	// body of fn1 is at index 2
	fn2 := fn1.Elems[2].(*List)
	// body of fn2 is at index 2, 3
	aUse := fn2.Elems[2]
	bUse := fn2.Elems[3]

	checkResolved := func(n Node, name string, depth, index int) {
		rs, ok := n.(*ResolvedSymbol)
		if !ok {
			t.Errorf("expected ResolvedSymbol for %s, got %T", name, n)
			return
		}
		if rs.Name != name || rs.Depth != depth || rs.Index != index {
			t.Errorf("for %s: got {%s %d %d}, want {%s %d %d}", name, rs.Name, rs.Depth, rs.Index, name, depth, index)
		}
	}

	// a is from outer scope (depth 1), index 0 (param a)
	checkResolved(aUse, "a", 1, 0)
	// b is from inner scope (depth 0), index 0 (param b)
	checkResolved(bUse, "b", 0, 0)
}

func TestCompilerShadowing(t *testing.T) {
	// (let ((x 1)) (let ((x 2)) x))
	src := "(let ((x 1)) (let ((x 2)) x))"
	ast, _ := Parse(src)
	res, err := Compile(ast, nil, nil)
	if err != nil {
		t.Fatalf("Compile error: %v", err)
	}

	let1 := res.(*List)
	let2 := let1.Elems[2].(*List)
	xUse := let2.Elems[2]

	checkResolved := func(n Node, name string, depth, index int) {
		rs, ok := n.(*ResolvedSymbol)
		if !ok {
			t.Errorf("expected ResolvedSymbol for %s, got %T", name, n)
			return
		}
		if rs.Depth != depth || rs.Index != index {
			t.Errorf("shadowing failed: got {%d %d}, want {%d %d}", rs.Depth, rs.Index, depth, index)
		}
	}

	// Should resolve to inner x (depth 0, index 0)
	checkResolved(xUse, "x", 0, 0)
}
