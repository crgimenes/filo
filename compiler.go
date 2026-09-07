package filo

import (
	"fmt"
)

// Scope is a lexical scope during lowering: which names are bound here and
// which slot each one owns.
type Scope struct {
	vars   map[string]int
	next   int
	parent *Scope
}

func newScope(parent *Scope) *Scope {
	return &Scope{
		vars:   make(map[string]int),
		parent: parent,
	}
}

func (s *Scope) define(name string) int {
	idx := s.next
	s.vars[name] = idx
	s.next++
	return idx
}

func (s *Scope) resolve(name string) (depth int, index int, found bool) {
	depth = 0
	for cur := s; cur != nil; cur = cur.parent {
		idx, ok := cur.vars[name]
		if ok {
			return depth, idx, true
		}
		depth++
	}
	return -1, -1, false
}

type compiler struct {
	scope    *Scope
	builtins map[string]builtinFunc
	symbols  *SymbolTable
}

// Compile lowers a parse tree to the IR described in docs/ir.md and returns
// its root (an *Instr). Locals become frame slots, builtins direct references
// and globals ids in the symbol table — or, with a nil table, by-name lookups.
// A special form with the wrong shape lowers to an instruction that raises the
// form's own error when it is evaluated, never earlier; only the four shape
// errors named in the spec are reported here.
func Compile(node Node, builtins map[string]builtinFunc, symbols *SymbolTable) (Node, error) {
	c := &compiler{
		scope:    newScope(nil),
		builtins: builtins,
		symbols:  symbols,
	}
	return c.lower(node)
}

func (c *compiler) lower(node Node) (*Instr, error) {
	switch n := node.(type) {
	case *NumberLit:
		return &Instr{Op: OpConst, Val: VNum(n.Value)}, nil
	case *BoolLit:
		return &Instr{Op: OpConst, Val: VBool(n.Value)}, nil
	case *StringLit:
		return &Instr{Op: OpConst, Val: VString(n.Value)}, nil
	case *Symbol:
		return c.symbol(n.Name), nil
	case *List:
		return c.list(n)
	default:
		return nil, fmt.Errorf("unknown node type: %T", node)
	}
}

func (c *compiler) symbol(name string) *Instr {
	depth, index, found := c.scope.resolve(name)
	if found {
		return &Instr{Op: OpLocal, A: depth, B: index, Name: name}
	}
	if c.builtins != nil {
		_, ok := c.builtins[name]
		if ok {
			return &Instr{Op: OpBuiltin, Name: name}
		}
	}
	if c.symbols != nil {
		return &Instr{Op: OpGlobal, A: c.symbols.Resolve(name), Name: name}
	}
	return &Instr{Op: OpDynamic, Name: name}
}

func (c *compiler) lowerAll(nodes []Node) ([]*Instr, error) {
	out := make([]*Instr, len(nodes))
	for i, n := range nodes {
		in, err := c.lower(n)
		if err != nil {
			return nil, err
		}
		out[i] = in
	}
	return out, nil
}

func invalid(ctx, msg string) *Instr {
	return &Instr{Op: OpInvalid, Name: ctx, Msg: msg}
}

func (c *compiler) list(list *List) (*Instr, error) {
	if len(list.Elems) == 0 {
		return &Instr{Op: OpEmpty}, nil
	}
	// The head decides the form only when it is a plain symbol: special
	// forms are recognized by name in head position and nowhere else.
	head, ok := list.Elems[0].(*Symbol)
	if ok {
		switch head.Name {
		case "if":
			return c.simple(OpIf, "", list.Elems[1:])
		case "do":
			return c.simple(OpDo, "", list.Elems[1:])
		case "and":
			return c.simple(OpAnd, "", list.Elems[1:])
		case "or":
			return c.simple(OpOr, "", list.Elems[1:])
		case "set":
			return c.simple(OpSet, "", list.Elems[1:])
		case "exit":
			return c.simple(OpExit, "", list.Elems[1:])
		case "return":
			return c.simple(OpReturn, "", list.Elems[1:])
		case "values", "tuple":
			return c.simple(OpTuple, head.Name, list.Elems[1:])
		case "cond":
			return c.cond(list)
		case "let":
			return c.let(list)
		case "letv":
			return c.letv(list)
		case "fn":
			return c.fn(list)
		case "def":
			return c.def(list)
		}
	}
	elems, err := c.lowerAll(list.Elems)
	if err != nil {
		return nil, err
	}
	if elems[0].Op == OpBuiltin {
		return &Instr{Op: OpCallB, Name: elems[0].Name, Fn: c.builtins[elems[0].Name], Args: elems[1:]}, nil
	}
	return &Instr{Op: OpCall, Args: elems}, nil
}

// simple lowers the forms that keep their arguments as they are and validate
// them when evaluated.
func (c *compiler) simple(op Opcode, name string, args []Node) (*Instr, error) {
	lowered, err := c.lowerAll(args)
	if err != nil {
		return nil, err
	}
	return &Instr{Op: op, Name: name, Args: lowered}, nil
}

func (c *compiler) cond(list *List) (*Instr, error) {
	clauses := make([]Clause, 0, len(list.Elems)-1)
	for _, node := range list.Elems[1:] {
		clause, ok := node.(*List)
		if !ok || len(clause.Elems) < 2 {
			clauses = append(clauses, Clause{Invalid: true, Msg: "cond clause must be a list of a test and a body"})
			continue
		}
		start := 0
		var test *Instr
		sym, isSym := clause.Elems[0].(*Symbol)
		isElse := isSym && sym.Name == "else"
		if isElse {
			start = 1
		} else {
			var err error
			test, err = c.lower(clause.Elems[0])
			if err != nil {
				return nil, err
			}
			start = 1
		}
		body, err := c.lowerAll(clause.Elems[start:])
		if err != nil {
			return nil, err
		}
		clauses = append(clauses, Clause{Else: isElse, Test: test, Body: body})
	}
	return &Instr{Op: OpCond, Clauses: clauses}, nil
}

func (c *compiler) let(list *List) (*Instr, error) {
	const ctx = "let"
	args := list.Elems[1:]
	if len(args) == 0 {
		return invalid(ctx, "let expects bindings and body"), nil
	}
	bindings, ok := args[0].(*List)
	if !ok {
		if len(args) < 2 {
			return invalid(ctx, "let expects bindings and body"), nil
		}
		return invalid(ctx, "let expects binding list"), nil
	}
	// Sequential like let*: each value is lowered in a scope that already
	// holds the bindings before it.
	c.enterScope()
	defer c.leaveScope()
	values := make([]*Instr, 0, len(bindings.Elems))
	for _, b := range bindings.Elems {
		pair, okPair := b.(*List)
		if !okPair || len(pair.Elems) != 2 {
			return nil, fmt.Errorf("invalid let binding")
		}
		name, okName := pair.Elems[0].(*Symbol)
		if !okName {
			return nil, fmt.Errorf("let binding name must be symbol")
		}
		value, err := c.lower(pair.Elems[1])
		if err != nil {
			return nil, err
		}
		c.scope.define(name.Name)
		values = append(values, value)
	}
	body, err := c.lowerAll(args[1:])
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return invalid(ctx, "let expects bindings and body"), nil
	}
	return &Instr{Op: OpLet, A: len(values), Args: append(values, body...)}, nil
}

func (c *compiler) letv(list *List) (*Instr, error) {
	const ctx = "letv"
	args := list.Elems[1:]
	if len(args) < 2 {
		return invalid(ctx, "letv expects bindings and body"), nil
	}
	namesList, ok := args[0].(*List)
	if !ok {
		return invalid(ctx, "letv expects name list"), nil
	}
	// The tuple is evaluated outside the new scope, so it is lowered there.
	tuple, err := c.lower(args[1])
	if err != nil {
		return nil, err
	}
	c.enterScope()
	defer c.leaveScope()
	names := make([]string, len(namesList.Elems))
	for i, n := range namesList.Elems {
		sym, okSym := n.(*Symbol)
		if !okSym {
			return nil, fmt.Errorf("letv names must be symbols")
		}
		c.scope.define(sym.Name)
		names[i] = sym.Name
	}
	body, err := c.lowerAll(args[2:])
	if err != nil {
		return nil, err
	}
	return &Instr{Op: OpLetv, Names: names, Args: append([]*Instr{tuple}, body...)}, nil
}

func (c *compiler) fn(list *List) (*Instr, error) {
	const ctx = "fn"
	args := list.Elems[1:]
	if len(args) == 0 {
		return invalid(ctx, "fn expects parameters and body"), nil
	}
	paramsList, ok := args[0].(*List)
	if !ok {
		if len(args) < 2 {
			return invalid(ctx, "fn expects parameters and body"), nil
		}
		return invalid(ctx, "fn expects parameter list"), nil
	}
	c.enterScope()
	defer c.leaveScope()
	params := make([]string, len(paramsList.Elems))
	for i, p := range paramsList.Elems {
		sym, okSym := p.(*Symbol)
		if !okSym {
			return nil, fmt.Errorf("fn params must be symbols")
		}
		c.scope.define(sym.Name)
		params[i] = sym.Name
	}
	body, err := c.lowerAll(args[1:])
	if err != nil {
		return nil, err
	}
	if len(body) == 0 {
		return invalid(ctx, "fn expects parameters and body"), nil
	}
	return &Instr{Op: OpFn, Names: params, Args: body}, nil
}

func (c *compiler) def(list *List) (*Instr, error) {
	if len(list.Elems) != 3 {
		return invalid("def", "def expects name and expression"), nil
	}
	value, err := c.lower(list.Elems[2])
	if err != nil {
		return nil, err
	}
	in := &Instr{Op: OpDef, Args: []*Instr{value}}
	name, ok := list.Elems[1].(*Symbol)
	if !ok {
		in.Msg = "def name must be symbol"
		return in, nil
	}
	in.Name = name.Name
	return in, nil
}

func (c *compiler) enterScope() {
	c.scope = newScope(c.scope)
}

func (c *compiler) leaveScope() {
	c.scope = c.scope.parent
}
