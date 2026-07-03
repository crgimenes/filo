package filo

import (
	"context"
	"fmt"
	"time"
)

type evaluator struct {
	ctx       context.Context
	cfg       EvalConfig
	builtins  map[string]builtinFunc
	steps     int
	recursion int
	global    *GlobalEnv
	frame     *Frame // Current local scope (can be nil if at top level)
	startedAt time.Time
}

func newEvaluator(ctx context.Context, cfg EvalConfig, global *GlobalEnv, builtins map[string]builtinFunc) *evaluator {
	return &evaluator{ctx: ctx, cfg: cfg, builtins: builtins, global: global, startedAt: time.Now()}
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
			// The symbol resolved at compile time but the global was never set at
			// runtime (GetByID only fails on an out-of-bounds ID).
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
	_, ok := err.(*exitSignal)
	if ok {
		return err
	}
	_, ok = err.(*returnSignal)
	if ok {
		return err
	}
	return fmt.Errorf("in %s: %w", ctx, err)
}

func (ev *evaluator) evalList(list *List) (Value, error) {
	if len(list.Elems) == 0 {
		return Value{}, fmt.Errorf("empty list expression")
	}

	rb, ok := list.Elems[0].(*ResolvedBuiltin)
	if ok {
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
	// Special forms stay plain Symbols: the compiler only resolves names bound in
	// scope, and keywords like "if"/"let"/"fn" never are.

	if ok {
		switch headSym.Name {
		case "if":
			v, err := ev.evalIf(list.Elems[1:])
			return v, wrapIn("if", err)
		case "cond":
			v, err := ev.evalCond(list.Elems[1:])
			return v, wrapIn("cond", err)
		case "do":
			v, err := ev.evalDo(list.Elems[1:])
			return v, wrapIn("do", err)
		case "and":
			v, err := ev.evalAnd(list.Elems[1:])
			return v, wrapIn("and", err)
		case "or":
			v, err := ev.evalOr(list.Elems[1:])
			return v, wrapIn("or", err)
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
	if len(fn.Params) != len(argNodes) {
		return Value{}, fmt.Errorf("function expects %d arguments, got %d", len(fn.Params), len(argNodes))
	}
	ev.recursion++
	if ev.cfg.RecursionLimit > 0 && ev.recursion > ev.cfg.RecursionLimit {
		ev.recursion--
		return Value{}, fmt.Errorf("recursion limit exceeded")
	}

	// Evaluate the arguments directly into the frame's slots, skipping the
	// intermediate slice an evalArgs helper would allocate.
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
		ret, ok := err.(*returnSignal)
		if ok {
			return ret.Value, nil
		}
		return Value{}, err
	}
	return result, nil
}

// Keep callFunc for Builtins usage (takes []Value)
func (ev *evaluator) callFunc(ctx context.Context, fn *Func, args []Value) (Value, error) {
	if len(args) != len(fn.Params) {
		return Value{}, fmt.Errorf("function expects %d arguments, got %d", len(fn.Params), len(args))
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
		ret, ok := err.(*returnSignal)
		if ok {
			return ret.Value, nil
		}
		return Value{}, err
	}
	return result, nil
}

// evalAnd evaluates its arguments left to right and short-circuits: the first
// false stops evaluation and later arguments never run. Every evaluated
// argument must be a bool. (and) with no arguments is #t.
func (ev *evaluator) evalAnd(args []Node) (Value, error) {
	for _, node := range args {
		v, err := ev.eval(node)
		if err != nil {
			return Value{}, err
		}
		b, err := v.AsBool()
		if err != nil {
			return Value{}, err
		}
		if !b {
			return VBool(false), nil
		}
	}
	return VBool(true), nil
}

// evalOr evaluates its arguments left to right and short-circuits: the first
// true stops evaluation and later arguments never run. Every evaluated
// argument must be a bool. (or) with no arguments is #f.
func (ev *evaluator) evalOr(args []Node) (Value, error) {
	for _, node := range args {
		v, err := ev.eval(node)
		if err != nil {
			return Value{}, err
		}
		b, err := v.AsBool()
		if err != nil {
			return Value{}, err
		}
		if b {
			return VBool(true), nil
		}
	}
	return VBool(false), nil
}

// evalCond evaluates a multi-way branch:
//
//	(cond (test1 body1...) (test2 body2...) (else bodyN...))
//
// Clauses are tried in order; the first whose test evaluates to #t runs its
// body (an implicit do — the last expression is the value). An `else` clause,
// if present, must be last and always matches. Each evaluated test must be a
// bool. When no clause matches, the result is the empty list, like (if) with a
// false condition and no else branch.
func (ev *evaluator) evalCond(clauses []Node) (Value, error) {
	for _, c := range clauses {
		clause, ok := c.(*List)
		if !ok || len(clause.Elems) < 2 {
			return Value{}, fmt.Errorf("cond clause must be a list of a test and a body")
		}
		if sym, ok := clause.Elems[0].(*Symbol); ok && sym.Name == "else" {
			return ev.evalBody(clause.Elems[1:])
		}
		test, err := ev.eval(clause.Elems[0])
		if err != nil {
			return Value{}, err
		}
		match, err := test.AsBool()
		if err != nil {
			return Value{}, err
		}
		if match {
			return ev.evalBody(clause.Elems[1:])
		}
	}
	return VList([]Value{}), nil
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
		return Value{}, fmt.Errorf("do expects at least 1 expression")
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
	// A binding may read the ones before it — (let ((x 1) (y x)) ...) — and the
	// compiler resolves such reads into THIS frame, so newFrame must already be
	// active while the binding values are evaluated. Reads of outer variables
	// resolve through the parent frame and are unaffected.
	oldFrame := ev.frame
	ev.frame = newFrame

	for i, b := range bindingsList.Elems {
		pair, okPair := b.(*List)
		if !okPair || len(pair.Elems) != 2 {
			ev.frame = oldFrame
			return Value{}, fmt.Errorf("invalid let binding")
		}
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

	// Copy the tuple's elements into the frame. The slice must NOT be shared with
	// the source tuple: a later (set var ...) writes into the frame's slots, and
	// aliasing would mutate the original tuple in place (corrupting a value that
	// is still reachable from another binding or the Go host).
	slots := make([]Value, len(elements))
	copy(slots, elements)
	newFrame := &Frame{
		slots:  slots,
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
		return Value{}, fmt.Errorf("fn expects parameters and body")
	}
	paramsList, ok := args[0].(*List)
	if !ok {
		return Value{}, fmt.Errorf("fn expects parameter list")
	}
	params := make([]string, len(paramsList.Elems))
	for i, p := range paramsList.Elems {
		sym, okSym := p.(*Symbol)
		if !okSym {
			return Value{}, fmt.Errorf("fn parameters must be symbols")
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
	}
	select {
	case <-ev.ctx.Done():
		return fmt.Errorf("execution cancelled: %w", ev.ctx.Err())
	default:
	}
	return nil
}
