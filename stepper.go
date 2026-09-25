package filo

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"runtime"
	"time"
)

// Stepper is a run of a unit's entry point that stops between instructions,
// for a debugger: each Step runs one instruction, those of a function a
// builtin calls back (map, fold) included, which show as calls above the
// one that called the builtin. The machine is deterministic, so going back
// a step is starting again and stepping one fewer.
//
// The run is a coroutine (iter.Pull) parked before each instruction: Close
// ends one that has not ended, and a Stepper dropped without it is closed
// when it is collected.
type Stepper struct {
	ctx    *stepContext
	ev     *evaluator
	root   *GlobalEnv
	cfg    EvalConfig
	run    *stepRun
	next   func() (struct{}, bool)
	stop   func()
	done   bool
	result Value
	err    error
}

// stepRun is what the coroutine leaves: kept apart from the Stepper, which
// it must not hold, or the Stepper would never be collected.
type stepRun struct {
	out Value
	err error
}

// stepAborted unwinds a run closed before its end.
type stepAborted struct{}

// stepContext is the run's context, the current Step's: a builtin keeps the
// one it was called with while the Steps that run its callbacks go by, and
// each of those has a context and a time limit of its own.
type stepContext struct {
	cur context.Context
}

func (c *stepContext) Deadline() (time.Time, bool) { return c.cur.Deadline() }
func (c *stepContext) Done() <-chan struct{}       { return c.cur.Done() }
func (c *stepContext) Err() error                  { return c.cur.Err() }
func (c *stepContext) Value(key any) any           { return c.cur.Value(key) }

// StepFrame is a call of the unit that is running, or waiting on the one
// above it.
type StepFrame struct {
	Fn       int     // the unit's function, numbered as a listing numbers them
	PC       int     // its next instruction, in the code section
	Slots    []Value // its locals, the parameters first
	Operands []Value // its operand stack, the bottom first
	Via      string  // the builtin that called it (map, fold); "" when the unit did
}

// StepState is where a Stepper stopped.
type StepState struct {
	Steps  int         // instructions run so far
	Frames []StepFrame // outermost first; the last one runs next
	Line   int         // where the next instruction came from; 0 when the
	Col    int         // unit does not say (a stripped one) or the run ended
}

// Start begins a run of the entry point named entry, stopped before its
// first instruction; the limits are Run's, except that the time limit
// counts per Step, since a person decides when the next one runs.
func (u *Unit) Start(entry string, globals map[string]Value, cfg EvalConfig) (*Stepper, error) {
	fn, err := u.entry(entry, globals)
	if err != nil {
		return nil, err
	}
	root := &GlobalEnv{}
	u.globalsFor(root, globals)
	cfg = cfg.withDefaults()
	sc := &stepContext{cur: context.Background()}
	ev := newEvaluator(sc, cfg, root, u.eng.builtins)
	ev.bc = &bcRun{stack: make([]Value, 0, fn.maxstack), calls: make([]bcAct, 0, 8)}
	run := &stepRun{}
	next, stop := iter.Pull(func(yield func(struct{}) bool) {
		aborted := false
		ev.bc.pause = func() {
			if aborted || !yield(struct{}{}) {
				aborted = true // and again at the next instruction, should a builtin recover
				panic(stepAborted{})
			}
		}
		defer func() {
			r := recover()
			_, closed := r.(stepAborted)
			if r != nil && !closed {
				run.out, run.err = Value{}, fmt.Errorf("panic in script: %v", r)
			}
		}()
		run.out, run.err = ev.runBC(fn, nil, nil)
	})
	s := &Stepper{ctx: sc, ev: ev, root: root, cfg: cfg, run: run, next: next, stop: stop}
	runtime.AddCleanup(s, func(stop func()) { stop() }, stop)
	_, ok := next() // to the first instruction
	if !ok {
		s.end(run.out, run.err)
	}
	return s, nil
}

// Step runs the next instruction. The error is the run's, when that
// instruction ended it failing; a Step after the end changes nothing.
func (s *Stepper) Step(ctx context.Context) error {
	if s.done {
		return s.err
	}
	stepCtx := ctx
	if s.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, s.cfg.Timeout)
		defer cancel()
	}
	s.ctx.cur, s.ev.startedAt = stepCtx, time.Now()
	_, ok := s.next()
	if !ok {
		s.end(s.run.out, s.run.err)
	}
	return s.err
}

// Close ends a run that has not ended; one that has is left as it is.
func (s *Stepper) Close() {
	s.stop()
	if !s.done {
		s.end(Value{}, errors.New("bytecode: the run was closed"))
	}
}

func (s *Stepper) end(v Value, err error) {
	s.done = true
	switch sig := err.(type) {
	case nil:
		s.result = v
	case *exitSignal:
		s.result = sig.Value
	case *returnSignal:
		s.result = sig.Value
	default:
		s.err = err
		if s.ev.bc.line > 0 {
			s.err = &PositionError{Line: s.ev.bc.line, Col: s.ev.bc.col, Err: err}
		}
	}
}

// Done says whether the run has ended, returning or failing.
func (s *Stepper) Done() bool {
	return s.done
}

// State is where the run stopped: its calls, each with its locals and
// operands, and the place of the next instruction.
func (s *Stepper) State() StepState {
	ev := s.ev
	st := StepState{Steps: ev.steps}
	calls := ev.bc.calls
	if s.done {
		calls = nil
	}
	for i, a := range calls {
		end := len(ev.bc.stack)
		if i+1 < len(calls) {
			end = calls[i+1].base
		}
		st.Frames = append(st.Frames, StepFrame{
			Fn:       a.fn.index,
			PC:       a.pc,
			Slots:    append([]Value(nil), a.f.slots...),
			Operands: append([]Value(nil), ev.bc.stack[a.base:end]...),
			Via:      a.via,
		})
	}
	if len(calls) > 0 {
		a := calls[len(calls)-1]
		line, col, ok := a.fn.unit.position(a.pc)
		if ok {
			st.Line, st.Col = line, col
		}
	}
	return st
}

// Result is what the run gave once it has ended, as Run gives it.
func (s *Stepper) Result() (Value, map[string]Value, error) {
	if !s.done {
		return Value{}, nil, errors.New("bytecode: the run has not ended")
	}
	if s.err != nil {
		return Value{}, nil, s.err
	}
	return s.result, s.root.ToMap(), nil
}
