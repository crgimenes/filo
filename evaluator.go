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
	root          *Env
	lastTickCheck time.Time
	startedAt     time.Time
}

func newEvaluator(ctx context.Context, cfg EvalConfig, root *Env, builtins map[string]builtinFunc) *evaluator {
	now := time.Now()
	return &evaluator{ctx: ctx, cfg: cfg, builtins: builtins, root: root, lastTickCheck: now, startedAt: now}
}

func (ev *evaluator) eval(node Node, env *Env) (Value, error) {
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
	case *Symbol:
		val, ok := env.Get(n.Name)
		if ok {
			return val, nil
		}
		return Value{}, fmt.Errorf("undefined symbol: %s", n.Name)
	case *List:
		return ev.evalList(n, env)
	default:
		return Value{}, fmt.Errorf("unknown node type: %T", node)
	}
}

func wrapIn(ctx string, err error) error {
	if err == nil {
		return nil
	}
	return fmt.Errorf("in %s: %w", ctx, err)
}

func (ev *evaluator) evalList(list *List, env *Env) (Value, error) {
	if len(list.Elems) == 0 {
		return Value{}, fmt.Errorf("empty list expression")
	}

	headSym, ok := list.Elems[0].(*Symbol)
	if ok {
		switch headSym.Name {
		case "if":
			v, err := ev.evalIf(list.Elems[1:], env)
			return v, wrapIn("if", err)
		case "do":
			v, err := ev.evalDo(list.Elems[1:], env)
			return v, wrapIn("do", err)
		case "let":
			v, err := ev.evalLet(list.Elems[1:], env)
			return v, wrapIn("let", err)
		case "letv":
			v, err := ev.evalLetv(list.Elems[1:], env)
			return v, wrapIn("letv", err)
		case "set":
			v, err := ev.evalSet(list.Elems[1:], env)
			return v, wrapIn("set", err)
		case "fn":
			v, err := ev.evalFn(list.Elems[1:], env)
			return v, wrapIn("fn", err)
		case "def":
			v, err := ev.evalDef(list.Elems[1:], env)
			return v, wrapIn("def", err)
		case "values":
			v, err := ev.evalValues(list.Elems[1:], env)
			return v, wrapIn("values", err)
		}
		builtin, okBuiltin := ev.builtins[headSym.Name]
		if okBuiltin {
			args, err := ev.evalArgs(list.Elems[1:], env)
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

	fnVal, err := ev.eval(list.Elems[0], env)
	if err != nil {
		return Value{}, wrapIn("call", err)
	}
	args, err := ev.evalArgs(list.Elems[1:], env)
	if err != nil {
		return Value{}, wrapIn("call arguments", err)
	}
	if fnVal.Kind != KFunc {
		return Value{}, fmt.Errorf("attempt to call non-function (got %s)", fnVal.describe())
	}
	v, callErr := ev.callFunc(ev.ctx, fnVal.Fn, args)
	return v, wrapIn("function call", callErr)
}

func (ev *evaluator) evalArgs(nodes []Node, env *Env) ([]Value, error) {
	result := make([]Value, len(nodes))
	for i, n := range nodes {
		val, err := ev.eval(n, env)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", i, err)
		}
		result[i] = val
	}
	return result, nil
}

func (ev *evaluator) evalIf(args []Node, env *Env) (Value, error) {
	if len(args) < 2 || len(args) > 3 {
		return Value{}, fmt.Errorf("if expects 2 or 3 arguments (condition then [else])")
	}
	cond, err := ev.eval(args[0], env)
	if err != nil {
		return Value{}, err
	}
	condVal, err := cond.AsBool()
	if err != nil {
		return Value{}, err
	}
	if condVal {
		return ev.eval(args[1], env)
	}
	// If no else branch provided, return empty list
	if len(args) == 2 {
		return VList([]Value{}), nil
	}
	return ev.eval(args[2], env)
}

func (ev *evaluator) evalDo(args []Node, env *Env) (Value, error) {
	if len(args) == 0 {
		return Value{}, fmt.Errorf("do requires at least one expression")
	}
	var result Value
	for _, arg := range args {
		val, err := ev.eval(arg, env)
		if err != nil {
			return Value{}, err
		}
		result = val
	}
	return result, nil
}

func (ev *evaluator) evalLet(args []Node, env *Env) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("let expects bindings and body")
	}
	bindingsList, ok := args[0].(*List)
	if !ok {
		return Value{}, fmt.Errorf("let expects binding list")
	}
	child := env.WithChild()
	for _, b := range bindingsList.Elems {
		pair, okPair := b.(*List)
		if !okPair || len(pair.Elems) != 2 {
			return Value{}, fmt.Errorf("invalid let binding")
		}
		nameSym, okSym := pair.Elems[0].(*Symbol)
		if !okSym {
			return Value{}, fmt.Errorf("let binding name must be symbol")
		}
		val, err := ev.eval(pair.Elems[1], child)
		if err != nil {
			return Value{}, err
		}
		child.Define(nameSym.Name, val)
	}
	return ev.evalBody(args[1:], child)
}

func (ev *evaluator) evalLetv(args []Node, env *Env) (Value, error) {
	if len(args) < 2 {
		return Value{}, fmt.Errorf("letv expects bindings and body")
	}
	namesList, ok := args[0].(*List)
	if !ok {
		return Value{}, fmt.Errorf("letv expects name list")
	}
	tupleVal, err := ev.eval(args[1], env)
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
	child := env.WithChild()
	for i, n := range namesList.Elems {
		sym, okSym := n.(*Symbol)
		if !okSym {
			return Value{}, fmt.Errorf("letv names must be symbols")
		}
		child.Define(sym.Name, elements[i])
	}
	return ev.evalBody(args[2:], child)
}

func (ev *evaluator) evalSet(args []Node, env *Env) (Value, error) {
	if len(args) != 2 {
		return Value{}, fmt.Errorf("set expects name and expression")
	}
	nameSym, ok := args[0].(*Symbol)
	if !ok {
		return Value{}, fmt.Errorf("set name must be symbol")
	}
	val, err := ev.eval(args[1], env)
	if err != nil {
		return Value{}, err
	}
	env.Assign(ev.root, nameSym.Name, val)
	return val, nil
}

func (ev *evaluator) evalFn(args []Node, env *Env) (Value, error) {
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
	fn := &Func{Params: params, Body: args[1:], Env: env}
	return VFunc(fn), nil
}

func (ev *evaluator) evalDef(args []Node, env *Env) (Value, error) {
	if len(args) != 2 {
		return Value{}, fmt.Errorf("def expects name and expression")
	}
	nameSym, ok := args[0].(*Symbol)
	if !ok {
		return Value{}, fmt.Errorf("def name must be symbol")
	}
	val, err := ev.eval(args[1], env)
	if err != nil {
		return Value{}, err
	}
	ev.root.Define(nameSym.Name, val)
	return val, nil
}

func (ev *evaluator) evalValues(args []Node, env *Env) (Value, error) {
	vals, err := ev.evalArgs(args, env)
	if err != nil {
		return Value{}, err
	}
	return VTuple(vals), nil
}

func (ev *evaluator) evalBody(nodes []Node, env *Env) (Value, error) {
	if len(nodes) == 0 {
		return Value{}, fmt.Errorf("empty body")
	}
	var result Value
	for _, n := range nodes {
		val, err := ev.eval(n, env)
		if err != nil {
			return Value{}, err
		}
		result = val
	}
	return result, nil
}

func (ev *evaluator) callFunc(ctx context.Context, fn *Func, args []Value) (Value, error) {
	if len(args) != len(fn.Params) {
		return Value{}, fmt.Errorf("function expects %d args, got %d", len(fn.Params), len(args))
	}
	ev.recursion++
	if ev.cfg.RecursionLimit > 0 && ev.recursion > ev.cfg.RecursionLimit {
		ev.recursion--
		return Value{}, fmt.Errorf("recursion limit exceeded")
	}
	child := fn.Env.WithChild()
	for i, name := range fn.Params {
		child.Define(name, args[i])
	}
	result, err := ev.evalBody(fn.Body, child)
	ev.recursion--
	if err != nil {
		return Value{}, err
	}
	return result, nil
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
