package filo

import (
	"context"
)

// FoldConstants recursively folds constant expressions in the AST.
// It returns the folded node and true if any change was made, or false otherwise.
//
// Rules:
// 1. Recursively fold children.
// 2. If node is a pure function call with all-literal arguments, evaluate it.
// 3. If node is (if cond then else) and cond is literal bool:
//   - If true, replace with folded 'then'.
//   - If false, replace with folded 'else' (or empty if missing).
func FoldConstants(node Node) (Node, bool) {
	changed := false

	switch n := node.(type) {
	case []Node:
		// Fold slice elements
		newElems := make([]Node, len(n))
		nodesChanged := false
		for i, elem := range n {
			folded, c := FoldConstants(elem)
			newElems[i] = folded
			if c {
				nodesChanged = true
			}
		}
		if nodesChanged {
			return newElems, true
		}
		return n, false

	case *List:
		// Fold elements first
		newElems := make([]Node, len(n.Elems))
		nodesChanged := false
		for i, elem := range n.Elems {
			folded, c := FoldConstants(elem)
			newElems[i] = folded
			if c {
				nodesChanged = true
			}
		}

		if nodesChanged {
			n = &List{Elems: newElems}
			changed = true
		}

		// Try to fold the list itself
		folded, c := foldList(n)
		if c {
			return folded, true
		}
		return n, changed

	default:
		return node, false
	}
}

func foldList(list *List) (Node, bool) {
	if len(list.Elems) == 0 {
		return list, false
	}

	head, ok := list.Elems[0].(*Symbol)
	if !ok {
		return list, false
	}

	// Handle 'if' special form
	if head.Name == "if" {
		return foldIf(list)
	}

	// Handle pure functions
	if isPureFunction(head.Name) {
		return foldPureCall(head.Name, list.Elems[1:])
	}

	return list, false
}

func foldIf(list *List) (Node, bool) {
	// (if cond then [else])
	if len(list.Elems) < 3 || len(list.Elems) > 4 {
		return list, false // Invalid arity, don't fold (let runtime error handle it?)
	}

	cond := list.Elems[1]

	// Check if cond is a boolean literal
	boolLit, ok := cond.(*BoolLit)
	if !ok {
		return list, false
	}
	if boolLit.Value {
		return list.Elems[2], true
	}
	// Condition is #f — return 'else' branch if present.
	if len(list.Elems) == 4 {
		return list.Elems[3], true
	}
	// No else branch: the interpreter's (if #f X) yields an empty list VALUE, so
	// fold to a (list) call, not a bare &List{} node — an empty list node is a
	// runtime error ("empty list expression"), which would make the optimized
	// form (and filofmt -fold-const output) diverge from the interpreter.
	return &List{Elems: []Node{&Symbol{Name: "list"}}}, true
}

// pureFunctions whitelists the default builtins that are safe to evaluate at
// fold time: pure, atom-in/atom-out. "list" is deliberately absent — an
// unquoted list node is a call, and the language has no quoted-literal node to
// fold a list result into (valueToNode only emits atoms for the same reason).
// "and"/"or" are special forms with short-circuit evaluation, not builtins, so
// they cannot fold through this path.
var pureFunctions = map[string]bool{
	"+": true, "-": true, "*": true, "/": true, "%": true, "pow": true,
	"=": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
	"not": true, "string": true, "number": true,
	"type-of": true, "is-empty": true, "is-nil": true,
	"length": true, "head": true, "tail": true, "nth": true,
}

func isPureFunction(name string) bool {
	return pureFunctions[name]
}

// foldBuiltins is the builtin table shared by every fold; pureFunctions only
// names default builtins, so the table is read-only after init.
var foldBuiltins = defaultBuiltins()

func foldPureCall(name string, args []Node) (Node, bool) {
	values := make([]Value, len(args))
	for i, arg := range args {
		v, ok := nodeToValue(arg)
		if !ok {
			return nil, false // not all arguments are literals
		}
		values[i] = v
	}

	fn, ok := foldBuiltins[name]
	if !ok {
		return nil, false
	}

	// The whitelisted builtins ignore the evaluator state, so a placeholder is
	// enough here.
	dummyEv := &evaluator{ctx: context.Background(), cfg: EvalConfig{}}
	val, err := fn(context.Background(), dummyEv, values)
	if err != nil {
		return nil, false // e.g. division by zero: leave it to fail at runtime
	}

	return valueToNode(val)
}

func nodeToValue(n Node) (Value, bool) {
	switch n := n.(type) {
	case *NumberLit:
		return VNum(n.Value), true
	case *BoolLit:
		return VBool(n.Value), true
	case *StringLit:
		return VString(n.Value), true
	}
	return Value{}, false
}

func valueToNode(v Value) (Node, bool) {
	// Only atom results fold: a List/Tuple result would need a node that
	// EVALUATES to it, and unquoted list nodes are calls.
	switch v.Kind {
	case KNumber:
		return &NumberLit{Value: v.Num}, true
	case KBool:
		return &BoolLit{Value: v.Bool}, true
	case KString:
		return &StringLit{Value: v.Str}, true
	}
	return nil, false
}
