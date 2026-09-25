package filo

import (
	"context"
	"errors"
	"fmt"
	"time"
)

// Stepper is a run of a unit's entry point that stops between instructions,
// for a debugger: each Step runs one instruction of the entry's own loop, a
// call into a function of the unit included. A builtin runs whole within
// the Step that calls it, even one that calls back into the unit (map,
// fold), as the C runtime's paused runs do. The machine is deterministic,
// so going back a step is starting again and stepping one fewer.
type Stepper struct {
	unit   *Unit
	ev     *evaluator
	root   *GlobalEnv
	cfg    EvalConfig
	done   bool
	result Value
	err    error
}

// StepFrame is a call of the unit that is running, or waiting on the one
// above it.
type StepFrame struct {
	Fn       int     // the unit's function, numbered as a listing numbers them
	PC       int     // its next instruction, in the code section
	Slots    []Value // its locals, the parameters first
	Operands []Value // its operand stack, the bottom first
}

// StepState is where a Stepper stopped.
type StepState struct {
	Steps  int         // instructions run so far, those under builtins included
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
	ev := newEvaluator(context.Background(), cfg, root, u.eng.builtins)
	ev.bc = &bcRun{stack: make([]Value, 0, fn.maxstack), calls: make([]bcAct, 0, 8)}
	f := ev.takeFrame(fn.nslots, nil)
	ev.bc.calls = append(ev.bc.calls, bcAct{fn: fn, f: f, pc: fn.off, root: true})
	return &Stepper{unit: u, ev: ev, root: root, cfg: cfg}, nil
}

// Step runs the next instruction. The error is the run's, when that
// instruction ended it failing; a Step after the end changes nothing.
func (s *Stepper) Step(ctx context.Context) (err error) {
	if s.done {
		return s.err
	}
	defer func() {
		r := recover()
		if r != nil {
			s.end(Value{}, fmt.Errorf("panic in script: %v", r))
			err = s.err
		}
	}()
	stepCtx := ctx
	if s.cfg.Timeout > 0 {
		var cancel context.CancelFunc
		stepCtx, cancel = context.WithTimeout(ctx, s.cfg.Timeout)
		defer cancel()
	}
	ev := s.ev
	ev.ctx, ev.startedAt = stepCtx, time.Now()
	var out Value
	top := &ev.bc.calls[len(ev.bc.calls)-1]
	err = ev.vmStep(top, &out)
	if err != nil {
		ev.vmWhere(&ev.bc.calls[len(ev.bc.calls)-1], err)
		s.end(Value{}, err)
		return s.err
	}
	if len(ev.bc.calls) == 0 {
		s.end(out, nil)
	}
	return nil
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
