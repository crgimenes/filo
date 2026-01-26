package filo

import (
	"fmt"
)

// ResolvedSymbol represents a symbol resolved to a static frame location.
type ResolvedSymbol struct {
	Name  string
	Depth int // How many frames up to look (0 = local)
	Index int // Index in the frame's slot array
}

// Scope represents a lexical scope during compilation.
type Scope struct {
	vars   map[string]int // Maps variable name to slot index
	next   int            // Next available slot index
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
		if idx, ok := cur.vars[name]; ok {
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

// Compile performs static analysis on the AST to resolve symbols.
// It returns a new AST with Symbol nodes replaced by ResolvedSymbol where possible,
// and ResolvedBuiltin for known builtins.
// Global symbols are resolved to ResolvedGlobal if a SymbolTable is provided.
func Compile(node Node, builtins map[string]builtinFunc, symbols *SymbolTable) (Node, error) {
	c := &compiler{
		scope:    newScope(nil), // Global scope (empty for now, serves as root)
		builtins: builtins,
		symbols:  symbols,
	}
	return c.walk(node)
}

func (c *compiler) walk(node Node) (Node, error) {
	switch n := node.(type) {
	case *List:
		return c.walkList(n)
	case *Symbol:
		depth, index, found := c.scope.resolve(n.Name)
		if found {
			return &ResolvedSymbol{
				Name:  n.Name,
				Depth: depth,
				Index: index,
			}, nil
		}
		// Not in local scope. Check builtins.
		if c.builtins != nil {
			if fn, ok := c.builtins[n.Name]; ok {
				return &ResolvedBuiltin{
					Name: n.Name,
					fn:   fn,
				}, nil
			}
		}
		// Not found in local or builtins -> Check Global Symbol Table
		if c.symbols != nil {
			id := c.symbols.Resolve(n.Name)
			return &ResolvedGlobal{
				Name: n.Name,
				ID:   id,
			}, nil
		}
		// Fallback: leave as Symbol (dynamic lookup)
		return n, nil
	default:
		// Literals pass through unchanged
		return n, nil
	}
}

func (c *compiler) walkList(list *List) (Node, error) {
	if len(list.Elems) == 0 {
		return list, nil
	}

	// Check for special forms that introduce scopes
	headSym, ok := list.Elems[0].(*Symbol)
	if ok {
		switch headSym.Name {
		case "let":
			return c.compileLet(list)
		case "letv":
			return c.compileLetv(list)
		case "fn":
			return c.compileFn(list)
		case "def":
			// def (name expr) - expr is compiled in current scope
			if len(list.Elems) != 3 {
				return list, nil // Let runtime handle error
			}
			val, err := c.walk(list.Elems[2])
			if err != nil {
				return nil, err
			}
			// Name remains a Symbol (global)
			return &List{Elems: []Node{list.Elems[0], list.Elems[1], val}}, nil
		case "if", "do", "set", "values", "tuple", "exit", "return":
			// Special forms: preserve head, walk arguments
			newElems := make([]Node, len(list.Elems))
			newElems[0] = list.Elems[0] // Preserve head Symbol
			for i := 1; i < len(list.Elems); i++ {
				res, err := c.walk(list.Elems[i])
				if err != nil {
					return nil, err
				}
				newElems[i] = res
			}
			return &List{Elems: newElems}, nil
		}
	}

	// Default: walk all elements
	newElems := make([]Node, len(list.Elems))
	for i, elem := range list.Elems {
		res, err := c.walk(elem)
		if err != nil {
			return nil, err
		}
		newElems[i] = res
	}
	return &List{Elems: newElems}, nil
}

func (c *compiler) compileLet(list *List) (Node, error) {
	// (let ((x 1) (y 2)) body...)
	if len(list.Elems) < 2 {
		return list, nil
	}
	bindingsList, ok := list.Elems[1].(*List)
	if !ok {
		return list, nil
	}

	// Filo let is sequential (like let*).
	// We must compile each binding's value in the scope that includes previous bindings?
	// Verified in evaluator.go: Yes, it uses 'child' scope which accumulates definitions.

	c.enterScope()
	defer c.leaveScope()

	newBindingsElems := make([]Node, len(bindingsList.Elems))

	for i, b := range bindingsList.Elems {
		pair, ok := b.(*List)
		if !ok || len(pair.Elems) != 2 {
			return nil, fmt.Errorf("invalid let binding")
		}
		nameSym, ok := pair.Elems[0].(*Symbol)
		if !ok {
			return nil, fmt.Errorf("let binding name must be symbol")
		}

		// Compile value in CURRENT scope (includes previous let bindings)
		valComp, err := c.walk(pair.Elems[1])
		if err != nil {
			return nil, err
		}

		// Define variable in scope
		// We don't need to change the Name symbol in the binding definition itself,
		// because 'let' runtime will set the slot.
		// BUT the runtime needs to know WHICH slot to set.
		// Implementation Detail: 'let' runtime will simply push values to Frame?
		// No, ResolvedSymbol is for usage. Definition might need info too?
		// Actually, if we use pure stack machine, 'let' pushes values.
		// But Filo keeps structure.
		// Optimization: We can annotate the binding symbol too!
		// But let's keep it simple: Define returns index.
		// We don't alter the binding name symbol in the AST,
		// but we might need to if the Runtime needs to know where to put it.
		// With Static Frames, Let just extends the frame.
		// The compiler calculates that 'x' is at index 0, 'y' at index 1.
		// The runtime 'let' starts at index K (inherited) and sets K, K+1...

		// Simple approach: The AST for let binding def doesn't need resolution index
		// if the runtime simply appends to the frame sequentially.
		// Since 'let' bindings are declared in order, the runtime just append values.
		// Accesses to 'x' will become ResolvedSymbol(depth=0, index=K).

		idx := c.scope.define(nameSym.Name)
		_ = idx // Used to track next slot

		newBindingsElems[i] = &List{Elems: []Node{nameSym, valComp}}
	}

	// Compile body
	newBody := make([]Node, 0, len(list.Elems)-2)
	for i := 2; i < len(list.Elems); i++ {
		res, err := c.walk(list.Elems[i])
		if err != nil {
			return nil, err
		}
		newBody = append(newBody, res)
	}

	// Reconstruct list
	resList := &List{Elems: make([]Node, 2+len(newBody))}
	resList.Elems[0] = list.Elems[0] // let
	resList.Elems[1] = &List{Elems: newBindingsElems}
	for i, b := range newBody {
		resList.Elems[2+i] = b
	}

	return resList, nil
}

func (c *compiler) compileLetv(list *List) (Node, error) {
	// (letv (x y) (tuple 1 2) body...)
	if len(list.Elems) < 3 {
		return list, nil
	}
	namesList, ok := list.Elems[1].(*List)
	if !ok {
		return list, nil
	}

	valExpr, err := c.walk(list.Elems[2])
	if err != nil {
		return nil, err
	}

	c.enterScope()
	defer c.leaveScope()

	for _, n := range namesList.Elems {
		sym, ok := n.(*Symbol)
		if !ok {
			return nil, fmt.Errorf("letv names must be symbols")
		}
		c.scope.define(sym.Name)
	}

	newBody := make([]Node, 0, len(list.Elems)-3)
	for i := 3; i < len(list.Elems); i++ {
		res, err := c.walk(list.Elems[i])
		if err != nil {
			return nil, err
		}
		newBody = append(newBody, res)
	}

	resList := &List{Elems: make([]Node, 3+len(newBody))}
	resList.Elems[0] = list.Elems[0]
	resList.Elems[1] = list.Elems[1] // names (left as symbols)
	resList.Elems[2] = valExpr
	for i, b := range newBody {
		resList.Elems[3+i] = b
	}
	return resList, nil
}

func (c *compiler) compileFn(list *List) (Node, error) {
	// (fn (a b) body...)
	if len(list.Elems) < 2 {
		return list, nil
	}
	paramsList, ok := list.Elems[1].(*List)
	if !ok {
		return list, nil
	}

	c.enterScope()
	defer c.leaveScope()

	for _, p := range paramsList.Elems {
		sym, ok := p.(*Symbol)
		if !ok {
			return nil, fmt.Errorf("fn params must be symbols")
		}
		c.scope.define(sym.Name)
	}

	newBody := make([]Node, 0, len(list.Elems)-2)
	for i := 2; i < len(list.Elems); i++ {
		res, err := c.walk(list.Elems[i])
		if err != nil {
			return nil, err
		}
		newBody = append(newBody, res)
	}

	resList := &List{Elems: make([]Node, 2+len(newBody))}
	resList.Elems[0] = list.Elems[0]
	resList.Elems[1] = list.Elems[1] // params
	for i, b := range newBody {
		resList.Elems[2+i] = b
	}

	// Annotate the function node with frame size info?
	// The runtime needs to know how big the frame should be?
	// Actually, for 'fn', the frame size is determined by args + locals inside.
	// But 'fn' creates a fresh frame.
	// If body contains 'let', 'let' extends that frame (or creates nested frame, but static frames usually imply one flat frame per function or nested frames).
	// Current Filo: Let creates NEW Env.
	// Static Strategy: Let creates NEW Frame linked to parent.
	// So each Scope = New Frame.
	// So 'fn' scope has N args. 'let' inside has M vars.
	// This matches.

	return resList, nil
}

func (c *compiler) enterScope() {
	c.scope = newScope(c.scope)
}

func (c *compiler) leaveScope() {
	c.scope = c.scope.parent
}
