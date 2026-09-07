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
	frame     *Frame // current local scope; nil at top level
	startedAt time.Time
}

func newEvaluator(ctx context.Context, cfg EvalConfig, global *GlobalEnv, builtins map[string]builtinFunc) *evaluator {
	return &evaluator{ctx: ctx, cfg: cfg, builtins: builtins, global: global, startedAt: time.Now()}
}

// eval runs one instruction. Every instruction costs one step, counted before
// it does anything, which keeps the count equal to the number of parse-tree
// nodes visited — the accounting docs/ir.md promises across runtimes.
func (ev *evaluator) eval(in *Instr) (Value, error) {
	err := ev.tick()
	if err != nil {
		return Value{}, err
	}
	ev.steps++
	if ev.cfg.StepLimit > 0 && ev.steps > ev.cfg.StepLimit {
		return Value{}, fmt.Errorf("step limit exceeded")
	}

	switch in.Op {
	case OpConst:
		return in.Val, nil
	case OpLocal:
		cur := ev.frame
		for i := 0; i < in.A; i++ {
			cur = cur.parent
		}
		return cur.slots[in.B], nil
	case OpGlobal:
		v, ok := ev.global.GetByID(in.A)
		if !ok {
			return Value{}, fmt.Errorf("undefined global: %s", in.Name)
		}
		return v, nil
	case OpDynamic:
		v, ok := ev.global.Get(in.Name)
		if !ok {
			return Value{}, fmt.Errorf("undefined symbol: %s", in.Name)
		}
		return v, nil
	case OpBuiltin:
		return Value{}, fmt.Errorf("builtin %q cannot be used as value", in.Name)
	case OpEmpty:
		return Value{}, fmt.Errorf("empty list expression")
	case OpInvalid:
		return Value{}, fmt.Errorf("in %s: %s", in.Name, in.Msg)
	case OpIf:
		v, err := ev.evalIf(in.Args)
		return v, wrapIn("if", err)
	case OpCond:
		v, err := ev.evalCond(in.Clauses)
		return v, wrapIn("cond", err)
	case OpDo:
		v, err := ev.evalDo(in.Args)
		return v, wrapIn("do", err)
	case OpAnd:
		v, err := ev.evalAnd(in.Args)
		return v, wrapIn("and", err)
	case OpOr:
		v, err := ev.evalOr(in.Args)
		return v, wrapIn("or", err)
	case OpLet:
		v, err := ev.evalLet(in)
		return v, wrapIn("let", err)
	case OpLetv:
		v, err := ev.evalLetv(in)
		return v, wrapIn("letv", err)
	case OpSet:
		v, err := ev.evalSet(in.Args)
		return v, wrapIn("set", err)
	case OpFn:
		v, err := ev.evalFn(in)
		return v, wrapIn("fn", err)
	case OpDef:
		v, err := ev.evalDef(in)
		return v, wrapIn("def", err)
	case OpTuple:
		vals, err := ev.evalArgs(in.Args)
		if err != nil {
			return Value{}, wrapIn(in.Name, err)
		}
		return VTuple(vals), nil
	case OpExit:
		return ev.evalSignal(in.Args, "exit")
	case OpReturn:
		return ev.evalSignal(in.Args, "return")
	case OpCallB:
		return ev.callBuiltin(in.Name, in.Fn, in.Args)
	case OpCall:
		return ev.evalCall(in.Args)
	default:
		return Value{}, fmt.Errorf("unknown instruction: %d", in.Op)
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

func (ev *evaluator) callBuiltin(name string, fn builtinFunc, argInstrs []*Instr) (Value, error) {
	args, err := ev.evalArgs(argInstrs)
	if err != nil {
		return Value{}, fmt.Errorf("while evaluating arguments for %q: %w", name, err)
	}
	v, callErr := fn(ev.ctx, ev, args)
	if callErr != nil {
		return Value{}, fmt.Errorf("in builtin %q: %w", name, callErr)
	}
	return v, nil
}

// evalCall runs a closure call: args[0] is the head, the rest its arguments. A
// by-name head that names a builtin takes the builtin path instead, which is
// how lowering without a builtin table keeps working.
func (ev *evaluator) evalCall(args []*Instr) (Value, error) {
	head := args[0]
	if head.Op == OpDynamic {
		builtin, ok := ev.builtins[head.Name]
		if ok {
			return ev.callBuiltin(head.Name, builtin, args[1:])
		}
	}
	fnVal, err := ev.eval(head)
	if err != nil {
		return Value{}, wrapIn("call", err)
	}
	if fnVal.Kind != KFunc {
		return Value{}, fmt.Errorf("attempt to call non-function (got %s)", fnVal.describe())
	}
	v, callErr := ev.callFuncInstrs(fnVal.Fn, args[1:])
	return v, wrapIn("function call", callErr)
}

// evalArgs evaluates instructions into values, naming the failing argument.
func (ev *evaluator) evalArgs(instrs []*Instr) ([]Value, error) {
	n := len(instrs)
	if n == 0 {
		return nil, nil
	}
	result := make([]Value, n)
	for i, in := range instrs {
		val, err := ev.eval(in)
		if err != nil {
			return nil, fmt.Errorf("argument %d: %w", i, err)
		}
		result[i] = val
	}
	return result, nil
}

// callFuncInstrs calls fn with unevaluated arguments, evaluating them straight
// into the new frame's slots.
func (ev *evaluator) callFuncInstrs(fn *Func, argInstrs []*Instr) (Value, error) {
	if len(fn.Params) != len(argInstrs) {
		return Value{}, fmt.Errorf("function expects %d arguments, got %d", len(fn.Params), len(argInstrs))
	}
	ev.recursion++
	if ev.cfg.RecursionLimit > 0 && ev.recursion > ev.cfg.RecursionLimit {
		ev.recursion--
		return Value{}, fmt.Errorf("recursion limit exceeded")
	}
	slots := make([]Value, len(fn.Params))
	for i, in := range argInstrs {
		val, err := ev.eval(in)
		if err != nil {
			ev.recursion--
			return Value{}, fmt.Errorf("in call arguments: argument %d: %w", i, err)
		}
		slots[i] = val
	}
	return ev.runFunc(fn, slots)
}

// callFunc calls fn with already evaluated arguments; builtins such as map and
// fold use it to run the closures they receive.
func (ev *evaluator) callFunc(ctx context.Context, fn *Func, args []Value) (Value, error) {
	_ = ctx // the evaluator's own context governs cancellation
	if len(args) != len(fn.Params) {
		return Value{}, fmt.Errorf("function expects %d arguments, got %d", len(fn.Params), len(args))
	}
	ev.recursion++
	if ev.cfg.RecursionLimit > 0 && ev.recursion > ev.cfg.RecursionLimit {
		ev.recursion--
		return Value{}, fmt.Errorf("recursion limit exceeded")
	}
	return ev.runFunc(fn, args)
}

// runFunc runs a function body in a frame of slots whose parent is the frame
// the closure captured. The recursion counter was already incremented by the
// caller and is released here. A return signal becomes the function's value.
func (ev *evaluator) runFunc(fn *Func, slots []Value) (Value, error) {
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

// evalAnd evaluates left to right and short-circuits: the first false stops
// evaluation and later arguments never run. Every evaluated argument must be a
// bool. (and) with no arguments is #t.
func (ev *evaluator) evalAnd(args []*Instr) (Value, error) {
	for _, in := range args {
		v, err := ev.eval(in)
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

// evalOr mirrors evalAnd: the first true stops evaluation. (or) is #f.
func (ev *evaluator) evalOr(args []*Instr) (Value, error) {
	for _, in := range args {
		v, err := ev.eval(in)
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

// evalCond tries the clauses in order; the first whose test is #t runs its
// body (an implicit do). An invalid clause errors only when it is reached, so
// an earlier match hides it. No match and no else: the empty list.
func (ev *evaluator) evalCond(clauses []Clause) (Value, error) {
	for _, c := range clauses {
		if c.Invalid {
			return Value{}, fmt.Errorf("%s", c.Msg)
		}
		if c.Else {
			return ev.evalBody(c.Body)
		}
		test, err := ev.eval(c.Test)
		if err != nil {
			return Value{}, err
		}
		match, err := test.AsBool()
		if err != nil {
			return Value{}, err
		}
		if match {
			return ev.evalBody(c.Body)
		}
	}
	return VList([]Value{}), nil
}

func (ev *evaluator) evalIf(args []*Instr) (Value, error) {
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

func (ev *evaluator) evalDo(args []*Instr) (Value, error) {
	if len(args) == 0 {
		return Value{}, fmt.Errorf("do expects at least 1 expression")
	}
	var result Value
	for _, in := range args {
		val, err := ev.eval(in)
		if err != nil {
			return Value{}, err
		}
		result = val
	}
	return result, nil
}

// evalLet: in.A binding values come first in Args, the body after. The new
// frame is active while the values are evaluated, so a binding may read the
// ones before it; reads of outer variables resolve through the parent.
func (ev *evaluator) evalLet(in *Instr) (Value, error) {
	n := in.A
	if len(in.Args) <= n {
		return Value{}, fmt.Errorf("let expects bindings and body")
	}
	newFrame := &Frame{
		slots:  make([]Value, n),
		parent: ev.frame,
	}
	oldFrame := ev.frame
	ev.frame = newFrame
	for i := range n {
		val, err := ev.eval(in.Args[i])
		if err != nil {
			ev.frame = oldFrame
			return Value{}, err
		}
		newFrame.slots[i] = val
	}
	res, err := ev.evalBody(in.Args[n:])
	ev.frame = oldFrame
	return res, err
}

// evalLetv: Args[0] is the tuple expression, evaluated in the OUTER scope;
// the rest is the body.
func (ev *evaluator) evalLetv(in *Instr) (Value, error) {
	tupleVal, err := ev.eval(in.Args[0])
	if err != nil {
		return Value{}, err
	}
	elements, err := tupleVal.AsTuple()
	if err != nil {
		return Value{}, fmt.Errorf("letv expects tuple expression")
	}
	if len(in.Names) != len(elements) {
		return Value{}, fmt.Errorf("letv arity mismatch")
	}
	// The slots must NOT alias the tuple: a later (set var ...) writes into
	// the frame, and sharing would mutate a tuple still reachable elsewhere.
	slots := make([]Value, len(elements))
	copy(slots, elements)
	newFrame := &Frame{
		slots:  slots,
		parent: ev.frame,
	}
	oldFrame := ev.frame
	ev.frame = newFrame
	res, err := ev.evalBody(in.Args[1:])
	ev.frame = oldFrame
	return res, err
}

func (ev *evaluator) evalSet(args []*Instr) (Value, error) {
	if len(args) != 2 {
		return Value{}, fmt.Errorf("set expects name and expression")
	}
	val, err := ev.eval(args[1])
	if err != nil {
		return Value{}, err
	}
	target := args[0]
	switch target.Op {
	case OpLocal:
		cur := ev.frame
		for i := 0; i < target.A; i++ {
			cur = cur.parent
		}
		cur.slots[target.B] = val
		return val, nil
	case OpGlobal:
		ev.global.DefineID(target.A, val)
		return val, nil
	case OpDynamic:
		ev.global.Define(target.Name, val)
		return val, nil
	default:
		return Value{}, fmt.Errorf("set name must be symbol")
	}
}

func (ev *evaluator) evalFn(in *Instr) (Value, error) {
	if len(in.Args) == 0 {
		return Value{}, fmt.Errorf("fn expects parameters and body")
	}
	fn := &Func{Params: in.Names, Body: in.Args, Frame: ev.frame}
	return VFunc(fn), nil
}

func (ev *evaluator) evalDef(in *Instr) (Value, error) {
	if in.Msg != "" {
		return Value{}, fmt.Errorf("%s", in.Msg)
	}
	val, err := ev.eval(in.Args[0])
	if err != nil {
		return Value{}, err
	}
	ev.global.Define(in.Name, val)
	return val, nil
}

func (ev *evaluator) evalBody(body []*Instr) (Value, error) {
	if len(body) == 0 {
		return Value{}, fmt.Errorf("empty body")
	}
	var result Value
	for _, in := range body {
		val, err := ev.eval(in)
		if err != nil {
			return Value{}, err
		}
		result = val
	}
	return result, nil
}

// evalSignal implements exit and return: an optional value, then the signal.
// Neither adds error context — a signal is not an error.
func (ev *evaluator) evalSignal(args []*Instr, form string) (Value, error) {
	if len(args) > 1 {
		return Value{}, fmt.Errorf("%s expects 0 or 1 argument", form)
	}
	val := VList([]Value{})
	if len(args) == 1 {
		var err error
		val, err = ev.eval(args[0])
		if err != nil {
			return Value{}, err
		}
	}
	if form == "exit" {
		return Value{}, &exitSignal{Value: val}
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
