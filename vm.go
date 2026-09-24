package filo

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
)

// The machine that runs a unit, as the C runtime's does (docs/bytecode.md):
// one step per instruction, and every index, slot, jump and stack access
// checked, since a unit is untrusted input.

const (
	bcPushK = iota
	bcPushG
	bcStoreG
	bcPushL
	bcStoreL
	bcPushUp
	bcStoreUp
	bcPop
	bcJmp
	bcCall
	bcCallB
	bcRet
	bcClosure
	bcTuple
	bcUnpack
	bcTrap
	bcPushB
)

const (
	bcJumpAlways = iota
	bcJumpFalse
	bcJumpAnd
	bcJumpOr
	bcJumpCheck
)

// bcRun is what the machine keeps for a run: its calls and their operands, and
// where it failed (line 0 while nothing did, or the unit does not say).
type bcRun struct {
	stack     []Value
	calls     []bcAct
	line, col int
}

// bcAct is a call running: its operands are ev.bc.stack[base:].
type bcAct struct {
	fn      *bcFunc
	f       *Frame
	pc      int
	base    int
	escapes uint64
	root    bool // the call a loop runs until it returns: from a builtin, or the entry
}

// Run runs the entry point named entry, with globals, and returns its value
// and the globals after it. An import that is neither a builtin nor a
// function among globals refuses the run, as the C runtime refuses the load;
// an extern that is missing fails when it is read. A failure the unit's
// debug section places is a *PositionError.
func (u *Unit) Run(ctx context.Context, entry string, globals map[string]Value, cfg EvalConfig) (result Value, newGlobals map[string]Value, err error) {
	fn := u.export(entry)
	if fn == nil {
		return Value{}, nil, fmt.Errorf("bytecode: no entry point named %s", entry)
	}
	if fn.nparams != 0 {
		return Value{}, nil, fmt.Errorf("bytecode: an entry point takes no arguments")
	}
	var lack []string
	for _, imp := range u.imports {
		v, ok := globals[imp.name]
		if imp.builtin == nil && (!ok || v.Kind != KFunc) {
			lack = append(lack, imp.name)
		}
	}
	if len(lack) > 0 {
		return Value{}, nil, fmt.Errorf("missing (%d): %s", len(lack), strings.Join(lack, " "))
	}
	defer func() {
		r := recover()
		if r != nil {
			result, newGlobals, err = Value{}, nil, fmt.Errorf("panic in script: %v", r)
		}
	}()

	e := u.eng
	root := e.envPool.Get().(*GlobalEnv)
	root.Reset(e.symbols)
	defer e.envPool.Put(root)
	for k, v := range globals {
		root.Define(k, v)
	}
	// an extern the host did not give that is a builtin here is that builtin,
	// as a unit compiled without it reads it
	for _, g := range u.externs {
		_, held := root.GetByID(u.globals[g])
		_, builtin := e.builtins[u.names[g]]
		if !held && builtin {
			root.DefineID(u.globals[g], VFunc(e.builtinValue(u.names[g])))
		}
	}

	cfg = cfg.withDefaults()
	runCtx := ctx
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}
	ev := newEvaluator(runCtx, cfg, root, e.builtins)
	result, err = ev.runBC(fn, nil, nil)
	if err != nil {
		switch sig := err.(type) {
		case *exitSignal:
			result, err = sig.Value, nil
		case *returnSignal:
			result, err = sig.Value, nil
		default:
			if ev.bc.line > 0 {
				err = &PositionError{Line: ev.bc.line, Col: ev.bc.col, Err: err}
			}
			return Value{}, nil, err
		}
	}
	return result, root.ToMap(), nil
}

func (u *Unit) export(name string) *bcFunc {
	for _, x := range u.exports {
		if x.name == name {
			return &u.fns[x.fn]
		}
	}
	return nil
}

// callBC calls a function of a unit with args, from a builtin or from a
// program: the recursion counter was incremented by the caller and is
// released here, as runFunc releases it.
func (ev *evaluator) callBC(fn *Func, args []Value) (Value, error) {
	v, err := ev.runBC(fn.bc, fn.Frame, args)
	ev.recursion--
	return v, err
}

// runBC runs fn in a new frame under parent until it returns.
func (ev *evaluator) runBC(fn *bcFunc, parent *Frame, args []Value) (Value, error) {
	if ev.bc == nil {
		ev.bc = &bcRun{stack: make([]Value, 0, fn.maxstack), calls: make([]bcAct, 0, 8)}
	}
	f := ev.takeFrame(fn.nslots, parent)
	copy(f.slots, args)
	escapes := ev.escapes
	base := len(ev.bc.stack)
	v, err := ev.vmLoop(bcAct{fn: fn, f: f, pc: fn.off, base: base, root: true})
	ev.bc.stack = ev.bc.stack[:base]
	if err == nil {
		ev.dropFrame(f, escapes)
	}
	return v, err
}

// vmLoop runs from base until base returns. A failure is placed at the
// instruction that failed, unless one further in already was.
func (ev *evaluator) vmLoop(base bcAct) (Value, error) {
	root := len(ev.bc.calls)
	ev.bc.calls = append(ev.bc.calls, base)
	var out Value
	for len(ev.bc.calls) > root {
		err := ev.vmStep(&ev.bc.calls[len(ev.bc.calls)-1], &out)
		if err != nil {
			ev.vmWhere(&ev.bc.calls[len(ev.bc.calls)-1], err)
			ev.bc.calls = ev.bc.calls[:root]
			return Value{}, err
		}
	}
	return out, nil
}

func (ev *evaluator) vmWhere(a *bcAct, err error) {
	if isSignal(err) || ev.bc.line != 0 {
		return
	}
	pc := a.fn.off
	if a.pc > a.fn.off {
		pc = a.pc - 1
	}
	line, col, ok := a.fn.unit.position(pc)
	if ok {
		ev.bc.line, ev.bc.col = line, col
	}
}

func (ev *evaluator) vmPush(a *bcAct, v Value) error {
	if len(ev.bc.stack)-a.base >= a.fn.maxstack {
		return errors.New("bytecode: operand stack overflow")
	}
	ev.bc.stack = append(ev.bc.stack, v)
	return nil
}

func (ev *evaluator) vmNeed(a *bcAct, n int) error {
	if len(ev.bc.stack)-a.base < n {
		return errors.New("bytecode: operand stack underflow")
	}
	return nil
}

func (ev *evaluator) vmTop() Value {
	return ev.bc.stack[len(ev.bc.stack)-1]
}

// vmUleb reads an operand inside the function.
func vmUleb(code []byte, end int, pc *int) (int, bool) {
	r := &bcReader{b: code[:end], p: *pc}
	v := r.uleb()
	*pc = r.p
	return v, !r.bad
}

var errTruncated = errors.New("bytecode: a truncated operand")

// vmStep runs one instruction of a, the call on top. When a root call
// returns, its value goes to out.
func (ev *evaluator) vmStep(a *bcAct, out *Value) error {
	err := ev.tick()
	if err != nil {
		return err
	}
	ev.steps++
	if ev.cfg.StepLimit > 0 && ev.steps > ev.cfg.StepLimit {
		return errors.New("step limit exceeded")
	}
	u := a.fn.unit
	end := a.fn.off + a.fn.length
	if a.pc >= end {
		return errors.New("bytecode: ran past the end of a function")
	}
	b := u.code[a.pc]
	a.pc++
	op := int(b >> 3)
	x := int(b & 7)
	if x == 7 && op != bcJmp {
		var ok bool
		x, ok = vmUleb(u.code, end, &a.pc)
		if !ok {
			return errTruncated
		}
	}
	switch op {
	case bcPushK:
		if x >= len(u.consts) {
			return errors.New("bytecode: a constant outside the unit")
		}
		return ev.vmPush(a, u.consts[x])
	case bcPushG:
		if x >= len(u.globals) {
			return errors.New("bytecode: a global outside the unit")
		}
		v, ok := ev.global.GetByID(u.globals[x])
		if !ok {
			return fmt.Errorf("undefined global: %s", u.names[x])
		}
		return ev.vmPush(a, v)
	case bcStoreG:
		if x >= len(u.globals) {
			return errors.New("bytecode: a global outside the unit")
		}
		err = ev.vmNeed(a, 1)
		if err != nil {
			return err
		}
		ev.global.DefineID(u.globals[x], ev.vmTop())
		return nil
	case bcPushL, bcStoreL, bcPushUp, bcStoreUp:
		return ev.vmLocal(a, op, x, end)
	case bcPop:
		err = ev.vmNeed(a, x)
		if err != nil {
			return err
		}
		ev.bc.stack = ev.bc.stack[:len(ev.bc.stack)-x]
		return nil
	case bcJmp:
		return ev.vmJump(a, x, end)
	case bcCall:
		err = ev.vmNeed(a, x+1)
		if err != nil {
			return err
		}
		return ev.vmInvoke(a, ev.bc.stack[len(ev.bc.stack)-x-1], x, x+1)
	case bcCallB:
		return ev.vmCallB(a, x, end)
	case bcRet:
		return ev.vmRet(a, x, out)
	case bcClosure:
		if x >= len(u.fns) {
			return errors.New("bytecode: a function outside the unit")
		}
		ev.escapes++ // it captures the frame, by reference
		return ev.vmPush(a, VFunc(&Func{Frame: a.f, bc: &u.fns[x]}))
	case bcTuple:
		err = ev.vmNeed(a, x)
		if err != nil {
			return err
		}
		top := len(ev.bc.stack)
		t := VTuple(append([]Value(nil), ev.bc.stack[top-x:]...))
		ev.bc.stack = ev.bc.stack[:top-x]
		return ev.vmPush(a, t)
	case bcUnpack:
		return ev.vmUnpack(a, x)
	case bcTrap:
		if x >= len(u.consts) {
			return errors.New("bytecode: a constant outside the unit")
		}
		if u.consts[x].Kind != KString {
			return errors.New("bytecode: a trap without a message")
		}
		return errors.New(u.consts[x].Str)
	case bcPushB:
		return ev.vmPushB(a, x)
	}
	return errors.New("bytecode: unknown instruction")
}

// vmLocal reads or writes a slot of the running frame or of one further
// out, checked: a unit is untrusted input.
func (ev *evaluator) vmLocal(a *bcAct, op, x, end int) error {
	depth, slot := 0, x
	if op == bcPushUp || op == bcStoreUp {
		depth = x
		var ok bool
		slot, ok = vmUleb(a.fn.unit.code, end, &a.pc)
		if !ok {
			return errTruncated
		}
	}
	f := a.f
	for i := 0; i < depth && f != nil; i++ {
		f = f.parent
	}
	if f == nil || slot >= len(f.slots) {
		return errors.New("bytecode: a slot outside its frame")
	}
	if op == bcPushL || op == bcPushUp {
		return ev.vmPush(a, f.slots[slot])
	}
	err := ev.vmNeed(a, 1)
	if err != nil {
		return err
	}
	if depth > 0 {
		ev.escapes++ // a frame further out now holds it
	}
	f.slots[slot] = ev.vmTop()
	return nil
}

func (ev *evaluator) vmJump(a *bcAct, cond, end int) error {
	code := a.fn.unit.code
	if a.pc+2 > end {
		return errors.New("bytecode: truncated jump")
	}
	off := int(int16(binary.LittleEndian.Uint16(code[a.pc:]))) // #nosec G115 -- the offset is a signed 16-bit field (docs/bytecode.md)
	a.pc += 2
	take := cond == bcJumpAlways
	if cond != bcJumpAlways {
		if cond > bcJumpCheck {
			return errors.New("bytecode: unknown jump condition")
		}
		err := ev.vmNeed(a, 1)
		if err != nil {
			return err
		}
		b, err := ev.vmTop().AsBool()
		if err != nil {
			return err
		}
		if cond == bcJumpFalse {
			ev.bc.stack = ev.bc.stack[:len(ev.bc.stack)-1]
		}
		if cond == bcJumpFalse || cond == bcJumpAnd {
			take = !b // these jump on false
		}
		if cond == bcJumpOr {
			take = b
		}
		if !take && (cond == bcJumpAnd || cond == bcJumpOr) {
			ev.bc.stack = ev.bc.stack[:len(ev.bc.stack)-1]
		}
	}
	if !take {
		return nil
	}
	target := a.pc + off
	if target < a.fn.off || target > end {
		return errors.New("bytecode: a jump out of its function")
	}
	a.pc = target
	return nil
}

// vmInvoke calls fnv with argc arguments on top of the stack, and drop
// values to take off when it returns (the arguments, and the function too
// when it was on the stack): bytecode gets a new call the machine moves
// into; anything else runs through callFunc.
func (ev *evaluator) vmInvoke(a *bcAct, fnv Value, argc, drop int) error {
	if fnv.Kind != KFunc {
		return fmt.Errorf("attempt to call non-function (got %s)", fnv.describe())
	}
	fn := fnv.Fn
	top := len(ev.bc.stack)
	args := ev.bc.stack[top-argc : top]
	if fn.bc == nil {
		v, err := ev.callFunc(ev.ctx, fn, args)
		if err != nil {
			return err
		}
		ev.bc.stack = ev.bc.stack[:top-drop]
		return ev.vmPush(a, v)
	}
	if fn.bc.nparams != argc {
		return fmt.Errorf("function expects %d arguments, got %d", fn.bc.nparams, argc)
	}
	ev.recursion++
	if ev.cfg.RecursionLimit > 0 && ev.recursion > ev.cfg.RecursionLimit {
		ev.recursion--
		return errors.New("recursion limit exceeded")
	}
	escapes := ev.escapes
	f := ev.takeFrame(fn.bc.nslots, fn.Frame)
	copy(f.slots, args)
	ev.bc.stack = ev.bc.stack[:top-drop]
	ev.bc.calls = append(ev.bc.calls, bcAct{fn: fn.bc, f: f, pc: fn.bc.off, base: len(ev.bc.stack), escapes: escapes})
	return nil
}

func (ev *evaluator) vmCallB(a *bcAct, argc, end int) error {
	u := a.fn.unit
	idx, ok := vmUleb(u.code, end, &a.pc)
	if !ok || idx >= len(u.imports) {
		return errors.New("bytecode: a builtin outside the imports")
	}
	err := ev.vmNeed(a, argc)
	if err != nil {
		return err
	}
	imp := &u.imports[idx]
	if imp.builtin == nil {
		fnv, ok := ev.global.GetByID(imp.sym)
		if !ok {
			return fmt.Errorf("undefined global: %s", imp.name)
		}
		return ev.vmInvoke(a, fnv, argc, argc)
	}
	top := len(ev.bc.stack)
	args := append([]Value(nil), ev.bc.stack[top-argc:]...) // the builtin may keep them: list does
	v, err := imp.builtin(ev.ctx, ev, args)
	if err != nil {
		return wrapf(err, "in builtin %q", imp.name)
	}
	ev.bc.stack = ev.bc.stack[:top-argc]
	return ev.vmPush(a, v)
}

func (ev *evaluator) vmRet(a *bcAct, x int, out *Value) error {
	err := ev.vmNeed(a, 1)
	if err != nil {
		return err
	}
	v := ev.vmTop()
	if x == 1 {
		return &exitSignal{Value: v} // ends the run from wherever it is
	}
	if x != 0 {
		return errors.New("bytecode: unknown return")
	}
	root, f, escapes, base := a.root, a.f, a.escapes, a.base
	ev.bc.calls = ev.bc.calls[:len(ev.bc.calls)-1]
	if root {
		*out = v
		return nil
	}
	ev.recursion--
	ev.dropFrame(f, escapes)
	ev.bc.stack = ev.bc.stack[:base]
	return ev.vmPush(&ev.bc.calls[len(ev.bc.calls)-1], v)
}

func (ev *evaluator) vmUnpack(a *bcAct, n int) error {
	err := ev.vmNeed(a, 1)
	if err != nil {
		return err
	}
	t := ev.vmTop()
	ev.bc.stack = ev.bc.stack[:len(ev.bc.stack)-1]
	if t.Kind != KTuple {
		return errors.New("letv expects tuple expression")
	}
	if len(t.Tup) != n {
		return errors.New("letv arity mismatch")
	}
	for _, v := range t.Tup {
		err = ev.vmPush(a, v)
		if err != nil {
			return err
		}
	}
	return nil
}

// vmPushB pushes a builtin as a value: the import resolved to one, or to
// the global that holds the function in Filo this engine has for its name.
func (ev *evaluator) vmPushB(a *bcAct, idx int) error {
	u := a.fn.unit
	if idx >= len(u.imports) {
		return errors.New("bytecode: a builtin outside the imports")
	}
	imp := &u.imports[idx]
	if imp.builtin == nil {
		v, ok := ev.global.GetByID(imp.sym)
		if !ok {
			return fmt.Errorf("undefined global: %s", imp.name)
		}
		return ev.vmPush(a, v)
	}
	return ev.vmPush(a, VFunc(u.eng.builtinValue(imp.name)))
}
