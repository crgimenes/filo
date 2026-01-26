package filo

import (
	"context"
	"fmt"
	"time"
)

type evaluator struct {
	ctx           context.Context
	cfg           EvalConfig
	builtins      map[string]builtinFunc
	steps         int
	recursion     int
	global        *GlobalEnv
	frame         *Frame // Current local scope (can be nil if at top level)
	lastTickCheck time.Time
	startedAt     time.Time
}

func newEvaluator(ctx context.Context, cfg EvalConfig, global *GlobalEnv, builtins map[string]builtinFunc) *evaluator {
	now := time.Now()
	return &evaluator{ctx: ctx, cfg: cfg, builtins: builtins, global: global, lastTickCheck: now, startedAt: now}
}

func (ev *evaluator) eval(node Node) (Value, error) {
	err := ev.tick()
	if err != nil {
		return Value{}, err
	}
	ev.steps++
	if ev.cfg.StepLimit > 0 && ev.steps > ev.cfg.StepLimit {
		return Value{}, fmt.Errorf("step limit exceeded")
	}

	switch n := node.(type) {
	case *NumberLit:
		return VNum(n.Value), nil
	case *BoolLit:
		return VBool(n.Value), nil
	case *StringLit:
		return VString(n.Value), nil

	case *ResolvedSymbol:
		// Static resolution: O(1) access
		cur := ev.frame
		for i := 0; i < n.Depth; i++ {
			cur = cur.parent
		}
		return cur.slots[n.Index], nil

	case *ResolvedBuiltin:
		return Value{}, fmt.Errorf("builtin %q cannot be used as value", n.Name)

	case *ResolvedGlobal:
		// Global resolution by ID: O(1) access
		v, ok := ev.global.GetByID(n.ID)
		if !ok {
			// This can happen if symbol was resolved at compile time but global never set at runtime
			// Return default or error?
			// To mimic legacy behavior (undefined symbol -> error), we should check if it was set.
			// But GetByID returning !ok means ID out of bounds.
			// If ID is in bounds but value is zero-value?
			// Current GetByID implementations checks bounds.
			// If bounds OK, returns value.
			// If value is empty Value{}, it returns it.
			// If we want error for undefined, we need IsDefined check.
			// For now, assume consistent state.
			// If GetByID fails (bounds), it's definitely error.
			return Value{}, fmt.Errorf("undefined global: %s", n.Name)
		}
		return v, nil

	case *Symbol:
		// Dynamic/Global resolution
		val, ok := ev.global.Get(n.Name)
		if ok {
			return val, nil
		}
		return Value{}, fmt.Errorf("undefined symbol: %s", n.Name)

	case *List:
		return ev.evalList(n)
	default:
		return Value{}, fmt.Errorf("unknown node type: %T", node)
	}
}

func wrapIn(ctx string, err error) error {
	if err == nil {
		return nil
	}
	if _, ok := err.(*exitSignal); ok {
		return err
	}
	if _, ok := err.(*returnSignal); ok {
		return err
	}
	return fmt.Errorf("in %s: %w", ctx, err)
}

func (ev *evaluator) evalList(list *List) (Value, error) {
	if len(list.Elems) == 0 {
		return Value{}, fmt.Errorf("empty list expression")
	}

	if rb, ok := list.Elems[0].(*ResolvedBuiltin); ok {
		args, err := ev.evalArgs(list.Elems[1:])
		if err != nil {
			return Value{}, fmt.Errorf("while evaluating arguments for %q: %w", rb.Name, err)
		}
		v, callErr := rb.fn(ev.ctx, ev, args)
		if callErr != nil {
			return Value{}, fmt.Errorf("in builtin %q: %w", rb.Name, callErr)
		}
		return v, nil
	}

	headSym, ok := list.Elems[0].(*Symbol)
	// Note: Head could be ResolvedSymbol if 'fn' or 'let' was somehow resolved?
	// But special forms (if, let, fn) are usually plain Symbols in AST root.
	// Compiler might have resolved "if" if it matched a local?
	// No, special forms are keywords, compiler shouldn't resolve them as vars if they are in head pos?
	// ACTUALLY, my compiler 'resolve' checks scope. "if" is not in scope. So it returns Symbol("if"). Correct.

	if ok {
		switch headSym.Name {
		case "if":
			v, err := ev.evalIf(list.Elems[1:])
			return v, wrapIn("if", err)
		case "do":
			v, err := ev.evalDo(list.Elems[1:])
			return v, wrapIn("do", err)
		case "let":
			v, err := ev.evalLet(list.Elems[1:])
			return v, wrapIn("let", err)
		case "letv":
			v, err := ev.evalLetv(list.Elems[1:])
			return v, wrapIn("letv", err)
		case "set":
			v, err := ev.evalSet(list.Elems[1:])
			return v, wrapIn("set", err)
		case "fn":
			v, err := ev.evalFn(list.Elems[1:])
			return v, wrapIn("fn", err)
		case "def":
			v, err := ev.evalDef(list.Elems[1:])
			return v, wrapIn("def", err)
		case "values":
			v, err := ev.evalValues(list.Elems[1:])
			return v, wrapIn("values", err)
		case "tuple":
			v, err := ev.evalValues(list.Elems[1:])
			return v, wrapIn("tuple", err)
		case "exit":
			return ev.evalExit(list.Elems[1:])
		case "return":
			return ev.evalReturn(list.Elems[1:])
		}
		builtin, okBuiltin := ev.builtins[headSym.Name]
		if okBuiltin {
			args, err := ev.evalArgs(list.Elems[1:])
			if err != nil {
				return Value{}, fmt.Errorf("while evaluating arguments for %q: %w", headSym.Name, err)
			}
			v, callErr := builtin(ev.ctx, ev, args)
			if callErr != nil {
				return Value{}, fmt.Errorf("in builtin %q: %w", headSym.Name, callErr)
			}
			return v, nil
		}
	}

	// Function call
	fnVal, err := ev.eval(list.Elems[0])
	if err != nil {
		return Value{}, wrapIn("call", err)
	}

	if fnVal.Kind != KFunc {
		return Value{}, fmt.Errorf("attempt to call non-function (got %s)", fnVal.describe())
	}
	// Optimization: Pass AST nodes directly to avoid intermediate slice alloc
	v, callErr := ev.callFuncAST(ev.ctx, fnVal.Fn, list.Elems[1:])
	return v, wrapIn("function call", callErr)
}

// evalArgs creates a slice of values from nodes
func (ev *evaluator) evalArgs(nodes []Node) ([]Value, error) {
	n := len(nodes)
	if n == 0 {
		return nil, nil
	}

	result := make([]Value, n)
	for i, node := range nodes {
		val, err := ev.eval(node)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", i, err)
		}
		result[i] = val
	}
	return result, nil
}

// Logic for callFuncAST (add to end or replace callFunc)
func (ev *evaluator) callFuncAST(ctx context.Context, fn *Func, argNodes []Node) (Value, error) {
	if len(argNodes) != len(fn.Params) {
		return Value{}, fmt.Errorf("function expects %d args, got %d", len(fn.Params), len(argNodes))
	}
	ev.recursion++
	if ev.cfg.RecursionLimit > 0 && ev.recursion > ev.cfg.RecursionLimit {
		ev.recursion--
		return Value{}, fmt.Errorf("recursion limit exceeded")
	}

	// Create Frame with size of params
	// Evaluate args directly into slots (avoiding intermediate slice)
	// We allocate the backing array for slots here. 1 allocation for Frame+Array?
	// Slice 'make' allocates array. Frame struct allocates struct.
	// To minimize allocs, we'd need Frame to embed array.
	// But this is already better than make([]Value) + Frame{slots: slice}.
	// Because evalArgs did make(), then callFunc did Frame{}. -> 2 allocs.
	// Here: make([]Value) inside Frame creation? No, Frame creation takes existing slice usually.

	slots := make([]Value, len(fn.Params))
	for i, node := range argNodes {
		val, err := ev.eval(node)
		if err != nil {
			ev.recursion--
			return Value{}, fmt.Errorf("in call arguments: argument %d: %w", i, err)
		}
		slots[i] = val
	}

	newFrame := &Frame{
		slots:  slots,
		parent: fn.Frame,
	}

	oldFrame := ev.frame
	ev.frame = newFrame

	result, err := ev.evalBody(fn.Body)

	ev.frame = oldFrame
	ev.recursion--

	if err != nil {
		if ret, ok := err.(*returnSignal); ok {
			return ret.Value, nil
		}
		return Value{}, err
	}
	return result, nil
}

// Keep callFunc for Builtins usage (takes []Value)
func (ev *evaluator) callFunc(ctx context.Context, fn *Func, args []Value) (Value, error) {
	if len(args) != len(fn.Params) {
		return Value{}, fmt.Errorf("function expects %d args, got %d", len(fn.Params), len(args))
	}
	ev.recursion++
	if ev.cfg.RecursionLimit > 0 && ev.recursion > ev.cfg.RecursionLimit {
		ev.recursion--
		return Value{}, fmt.Errorf("recursion limit exceeded")
	}

	newFrame := &Frame{
		slots:  args,
		parent: fn.Frame,
	}

	oldFrame := ev.frame
	ev.frame = newFrame

	result, err := ev.evalBody(fn.Body)

	ev.frame = oldFrame
	ev.recursion--

	if err != nil {
		if ret, ok := err.(*returnSignal); ok {
			return ret.Value, nil
		}
		return Value{}, err
	}
	return result, nil
}

func (ev *evaluator) evalIf(args []Node) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return Value{}, fmt.Errorf("if expects 2 or 3 arguments (condition then [else])")
	}
	cond, err := ev.eval(args[0])
	if err != nil {
		return Value{}, err
	}
	condVal, err := cond.AsBool()
	if err != nil {
		return Value{}, err
	}
	if condVal {
		return ev.eval(args[1])
	}
	if len(args) == 2 {
		return VList([]Value{}), nil
	}
	return ev.eval(args[2])
}

func (ev *evaluator) evalDo(args []Node) (Value, error) {
	if len(args) == 0 {
		return Value{}, fmt.Errorf("do requires at least one expression")
	}
	var result Value
	for _, arg := range args {
		val, err := ev.eval(arg)
		if err != nil {
			return Value{}, err
		}
		result = val
	}
	return result, nil
}

func (ev *evaluator) evalLet(args []Node) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("let expects bindings and body")
	}
	bindingsList, ok := args[0].(*List)
	if !ok {
		return Value{}, fmt.Errorf("let expects binding list")
	}

	// Create Frame
	numVars := len(bindingsList.Elems)
	newFrame := &Frame{
		slots:  make([]Value, numVars),
		parent: ev.frame,
	}

	// Temporarily switch to new frame?
	// Let bindings are evaluated in specific order.
	// Since `let` allows seeing previous bindings, we usually:
	// 1. Eval val in scope (parent + prev bindings).
	// 2. Define in scope.
	// Our Compiler treats `let` as ONE scope where variables are defined sequentially.
	// The `ResolvedSymbol` for `x` in `(let ((x 1) (y x)) ...)`:
	// `y`'s value `x` resolves to index 0.
	// So we need `newFrame` to be active availability WHILE evaluating bindings!

	oldFrame := ev.frame
	ev.frame = newFrame

	for i, b := range bindingsList.Elems {
		pair, okPair := b.(*List)
		if !okPair || len(pair.Elems) != 2 {
			ev.frame = oldFrame
			return Value{}, fmt.Errorf("invalid let binding")
		}
		// The compiler resolved access to previous bindings to point to THIS frame.
		// Access to outer vars points to PARENT frame (oldFrame).
		// So `ev.frame = newFrame` handles both correctly!

		val, err := ev.eval(pair.Elems[1])
		if err != nil {
			ev.frame = oldFrame
			return Value{}, err
		}
		newFrame.slots[i] = val
	}

	// Eval body
	res, err := ev.evalBody(args[1:])
	ev.frame = oldFrame // Restore
	return res, err
}

func (ev *evaluator) evalLetv(args []Node) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("letv expects bindings and body")
	}
	namesList, ok := args[0].(*List)
	if !ok {
		return Value{}, fmt.Errorf("letv expects name list")
	}

	// Eval tuple in OUTER scope (unlike let, letv values are one expression)
	tupleVal, err := ev.eval(args[1])
	if err != nil {
		return Value{}, err
	}
	elements, err := tupleVal.AsTuple()
	if err != nil {
		return Value{}, fmt.Errorf("letv expects tuple expression")
	}
	if len(namesList.Elems) != len(elements) {
		return Value{}, fmt.Errorf("letv arity mismatch")
	}

	// Create Frame
	newFrame := &Frame{
		slots:  elements, // Reuse the tuple slice as frame! Optimization!
		parent: ev.frame,
	}

	// Switch frame for body
	oldFrame := ev.frame
	ev.frame = newFrame
	res, err := ev.evalBody(args[2:])
	ev.frame = oldFrame
	return res, err
}

func (ev *evaluator) evalSet(args []Node) (Value, error) {
	if len(args) != 2 {
		return Value{}, fmt.Errorf("set expects name and expression")
	}
	val, err := ev.eval(args[1])
	if err != nil {
		return Value{}, err
	}

	switch sym := args[0].(type) {
	case *ResolvedSymbol:
		cur := ev.frame
		for i := 0; i < sym.Depth; i++ {
			cur = cur.parent
		}
		cur.slots[sym.Index] = val
		return val, nil
	case *ResolvedGlobal:
		ev.global.DefineID(sym.ID, val)
		return val, nil
	case *Symbol:
		ev.global.Define(sym.Name, val)
		return val, nil
	default:
		return Value{}, fmt.Errorf("set name must be symbol")
	}
}

func (ev *evaluator) evalFn(args []Node) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("fn expects params and body")
	}
	paramsList, ok := args[0].(*List)
	if !ok {
		return Value{}, fmt.Errorf("fn expects param list")
	}
	params := make([]string, len(paramsList.Elems))
	for i, p := range paramsList.Elems {
		sym, okSym := p.(*Symbol)
		if !okSym {
			return Value{}, fmt.Errorf("fn params must be symbols")
		}
		params[i] = sym.Name
	}
	fn := &Func{Params: params, Body: args[1:], Frame: ev.frame}
	return VFunc(fn), nil
}

func (ev *evaluator) evalDef(args []Node) (Value, error) {
	if len(args) != 2 {
		return Value{}, fmt.Errorf("def expects name and expression")
	}
	nameSym, ok := args[0].(*Symbol)
	if !ok {
		return Value{}, fmt.Errorf("def name must be symbol")
	}
	val, err := ev.eval(args[1])
	if err != nil {
		return Value{}, err
	}
	ev.global.Define(nameSym.Name, val)
	return val, nil
}

func (ev *evaluator) evalValues(args []Node) (Value, error) {
	vals, err := ev.evalArgs(args)
	if err != nil {
		return Value{}, err
	}
	return VTuple(vals), nil
}

func (ev *evaluator) evalBody(nodes []Node) (Value, error) {
	if len(nodes) == 0 {
		return Value{}, fmt.Errorf("empty body")
	}
	var result Value
	for _, n := range nodes {
		val, err := ev.eval(n)
		if err != nil {
			return Value{}, err
		}
		result = val
	}
	return result, nil
}

func (ev *evaluator) evalExit(args []Node) (Value, error) {
	if len(args) > 1 {
		return Value{}, fmt.Errorf("exit expects 0 or 1 argument")
	}
	val := VList([]Value{})
	if len(args) == 1 {
		var err error
		val, err = ev.eval(args[0])
		if err != nil {
			return Value{}, err
		}
	}
	return Value{}, &exitSignal{Value: val}
}

func (ev *evaluator) evalReturn(args []Node) (Value, error) {
	if len(args) > 1 {
		return Value{}, fmt.Errorf("return expects 0 or 1 argument")
	}
	val := VList([]Value{})
	if len(args) == 1 {
		var err error
		val, err = ev.eval(args[0])
		if err != nil {
			return Value{}, err
		}
	}
	return Value{}, &returnSignal{Value: val}
}

func (ev *evaluator) tick() error {
	if ev.cfg.Timeout > 0 {
		if time.Since(ev.startedAt) > ev.cfg.Timeout {
			return fmt.Errorf("execution timeout")
		}
		if time.Since(ev.lastTickCheck) >= time.Millisecond*5 {
			ev.lastTickCheck = time.Now()
		}
	}
	select {
	case <-ev.ctx.Done():
		return fmt.Errorf("execution cancelled: %w", ev.ctx.Err())
	default:
	}
	return nil
}
