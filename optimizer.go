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
	if boolLit, ok := cond.(*BoolLit); ok {
		if boolLit.Value {
			// Return 'then' branch
			return list.Elems[2], true
		} else {
			// Return 'else' branch
			if len(list.Elems) == 4 {
				return list.Elems[3], true
			}
			// No else branch -> void/nil?
			// Filo runtime returns empty list for missing else.
			// We should return empty list literal.
			// Currently empty list is modeled as empty *List.
			return &List{Elems: nil}, true
		}
	}
	return list, false
}

var pureFunctions = map[string]bool{
	"+": true, "-": true, "*": true, "/": true, "%": true, "pow": true,
	"=": true, "!=": true, "<": true, ">": true, "<=": true, ">=": true,
	"not": true, "and": true, "or": true,
	"type-of": true, "is-empty": true, "is-nil": true,
	"length": true, "head": true, "tail": true, "nth": true,
	"list": true, // (list 1 2) -> (1 2)? Value is List.
	// We want to reduce (list 1 2) to a Literal List?
	// AST has *List. Literal List is handled by parser as *List?
	// Note: (list 1 2) IS how we write a literal list in code if using 'list'.
	// But parser produces *List for (...) calls.
	// If I have (+ 1 2), parser gives List{+, 1, 2}. Fold -> 3.
	// If I have (list 1 2), parser gives List{list, 1, 2}.
	// If I evaluate it, I get VList{1, 2}.
	// Can I convert VList{1, 2} back to AST?
	// AST for list is just *List{Node, Node}.
	// But *List IS interpreted as call unless quoted.
	// So (list 1 2) -> evaluates to (1 2). Code for (1 2) is... (1 2).
	// If I replace (list 1 2) with (1 2)... (1 2) will be executed as "call 1".
	// So (list ...) should NOT be folded if it produces a list that would be executed!
	// Wait, constant folding runs on AST.
	// If source is `(list 1 2)`, it effectively IS a constant list.
	// But we can't replace it with a literal because there is no "Literal List" node type that isn't executed.
	// Unquoted lists are calls.
	// Quoted lists `'(1 2)` are literals.
	// So `(list 1 2)` -> `'(1 2)` (Quote with List).
	// Does parser support Quote node?
	// Let's check parser.go.
	// We might skipping folding 'list' for now to avoid complexity of introducing Quotes if not supported.
}

func isPureFunction(name string) bool {
	return pureFunctions[name]
}

func foldPureCall(name string, args []Node) (Node, bool) {
	// 1. Check if all args are literals
	values := make([]Value, len(args))
	for i, arg := range args {
		v, ok := nodeToValue(arg)
		if !ok {
			return nil, false // Not a literal
		}
		values[i] = v
	}

	// 2. Evaluate using a fresh engine/eval
	// We can't easily reuse the main engine because we are in 'filo' package.
	// We can create a temporary evaluator or reuse builtins.
	// `defaultBuiltins()` creates a map.
	// We can manually dispatch to pure implementations or use the real map.
	// Using real map ensures consistency.

	builtinFuncs := defaultBuiltins()
	fn, ok := builtinFuncs[name]
	if !ok {
		return nil, false
	}

	// Context? Background is fine for pure functions.
	// Evaluator? Pure functions shouldn't need a complex evaluator state unless they map/fold.
	// pureFunctions map avoids map/fold which take functions.
	// We only whitelisted simple math/logic.
	// So passing nil evaluator might crash if they use it?
	// Let's check builtins.go. simple math ignores evaluator.

	// Create dummy evaluator just in case.
	dummyEv := &evaluator{ctx: context.Background(), cfg: EvalConfig{}}

	val, err := fn(context.Background(), dummyEv, values)
	if err != nil {
		return nil, false // Evaluation failure (e.g. division by zero), don't fold.
	}

	// 3. Convert Value back to Node
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
		// List literals? Only if quoted?
		// For now, strict literal arguments.
	}
	return Value{}, false
}

func valueToNode(v Value) (Node, bool) {
	switch v.Kind {
	case KNumber:
		return &NumberLit{Value: v.Num}, true
	case KBool:
		return &BoolLit{Value: v.Bool}, true
	case KString:
		return &StringLit{Value: v.Str}, true
		// Tuple/List?
		// If result is List/Tuple, we need to return a Node that EVALUATES to that list/tuple.
		// E.g. (list 1 2).
		// For now, only support atom results to be safe.
	}
	return nil, false
}
