package filo

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

type Engine struct {
	builtins map[string]builtinFunc
	symbols  *SymbolTable
	envPool  sync.Pool
	// each builtin as a value, made when a script first names one outside a
	// call: one per builtin, so every use of it is the same function
	valuesMu sync.Mutex
	values   map[string]*Func
}

type EvalConfig struct {
	StepLimit      int
	RecursionLimit int
	Timeout        time.Duration
}

const (
	defaultStepLimit = 100_000
	// range is the only builtin whose output is not bounded by its inputs;
	// without a ceiling (range 1e18) exhausts the host before the first step
	// check. 2^20 values is ~100 MB here and far more than a script can
	// iterate under the default step limit. The C runtime uses the same cap.
	rangeMax              = 1 << 20
	defaultRecursionLimit = 128
	defaultTimeout        = 30 * time.Second
)

func (cfg EvalConfig) withDefaults() EvalConfig {
	if cfg.StepLimit == 0 {
		cfg.StepLimit = defaultStepLimit
	}
	if cfg.RecursionLimit == 0 {
		cfg.RecursionLimit = defaultRecursionLimit
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = defaultTimeout
	}
	return cfg
}

func NewEngine() *Engine {
	return &Engine{
		builtins: defaultBuiltins(),
		symbols:  NewSymbolTable(),
		envPool: sync.Pool{
			New: func() any {
				// We don't allocate size here because we need SymbolTable size at runtime
				return &GlobalEnv{}
			},
		},
	}
}

func (e *Engine) RegisterBuiltin(name string, fn Builtin) error {
	if name == "" {
		return fmt.Errorf("builtin name cannot be empty")
	}
	if fn == nil {
		return fmt.Errorf("builtin %q cannot be nil", name)
	}
	_, exists := e.builtins[name]
	if exists {
		return fmt.Errorf("builtin %q already registered", name)
	}
	e.builtins[name] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		return fn(ctx, args)
	}
	return nil
}

func (e *Engine) MustRegisterBuiltin(name string, fn Builtin) {
	err := e.RegisterBuiltin(name, fn)
	if err != nil {
		panic(err)
	}
}

// Program represents a compiled Filo script bound to an Engine instance.
type Program struct {
	ir     *Instr
	eng    *Engine
	source *sourceMap // where it came from, for an error to say where; nil for a tree
}

// Compile parses and compiles the source code into a reuseable Program.
func (e *Engine) Compile(src string) (*Program, error) {
	ast, source, err := parseSource(src)
	if err != nil {
		return nil, err
	}
	return e.compile(ast, source)
}

// CompileAST compiles a parsed AST into a reusable Program bound to this Engine.
// Constant subexpressions are folded first; folding is semantics-preserving (it
// only replaces a call when evaluating it with constant arguments succeeds), so
// there is no knob to turn it off.
func (e *Engine) CompileAST(ast Node) (*Program, error) {
	return e.compile(ast, nil)
}

// compile is CompileAST keeping where each part came from in source, when
// the tree was parsed from one.
func (e *Engine) compile(ast Node, source *sourceMap) (*Program, error) {
	folded, _ := foldNode(ast, source)
	ir, err := e.lowerSource(folded, source)
	if err != nil {
		if at, ok := errors.AsType[*PositionError](err); ok {
			return nil, &PositionError{Line: at.Line, Col: at.Col, Err: fmt.Errorf("compile error: %w", at.Err)}
		}
		return nil, fmt.Errorf("compile error: %w", err)
	}
	return &Program{ir: ir, eng: e, source: source}, nil
}

// builtinValue is the builtin name as a function value, the same one every
// time it is asked for.
func (e *Engine) builtinValue(name string) *Func {
	e.valuesMu.Lock()
	defer e.valuesMu.Unlock()
	fn, ok := e.values[name]
	if !ok {
		if e.values == nil {
			e.values = map[string]*Func{}
		}
		fn = &Func{builtin: e.builtins[name], name: name}
		e.values[name] = fn
	}
	return fn
}

// lower turns a parse tree into IR bound to this engine's builtins and
// symbol table.
func (e *Engine) lower(ast Node) (*Instr, error) {
	return e.lowerSource(ast, nil)
}

func (e *Engine) lowerSource(ast Node, source *sourceMap) (*Instr, error) {
	compiled, err := compileSource(ast, e.builtins, e.symbols, source, e.builtinValue)
	if err != nil {
		return nil, err
	}
	ir, ok := compiled.(*Instr)
	if !ok {
		return nil, fmt.Errorf("compile produced %T, not IR", compiled)
	}
	return ir, nil
}

// Execute runs the compiled program. When one compiled from source fails,
// the error is a *PositionError saying where; its message is the same.
func (p *Program) Execute(ctx context.Context, globals map[string]Value, cfg EvalConfig) (result Value, newGlobals map[string]Value, err error) {
	result, newGlobals, at, err := p.eng.run(ctx, p.ir, globals, cfg)
	if err != nil && at > 0 && p.source != nil {
		line, col := p.source.lineCol(at)
		err = &PositionError{Line: line, Col: col, Err: err}
	}
	return result, newGlobals, err
}

func (e *Engine) RunScript(ctx context.Context, src string, globals map[string]Value, cfg EvalConfig) (result Value, newGlobals map[string]Value, err error) {
	prog, err := e.Compile(src)
	if err != nil {
		return Value{}, nil, err
	}
	return prog.Execute(ctx, globals, cfg)
}

// ExecuteAST executes a script with the given globals. ast is either the IR
// returned by Compile or a parse tree, which is lowered first (without
// constant folding). This is the core execution method used by both RunScript
// and Script.Execute.
func (e *Engine) ExecuteAST(ctx context.Context, ast Node, globals map[string]Value, cfg EvalConfig) (result Value, newGlobals map[string]Value, err error) {
	result, newGlobals, _, err = e.run(ctx, ast, globals, cfg)
	return result, newGlobals, err
}

// run is ExecuteAST, also saying where it failed: the pos of the innermost
// instruction that did, 0 when not known.
func (e *Engine) run(ctx context.Context, ast Node, globals map[string]Value, cfg EvalConfig) (result Value, newGlobals map[string]Value, failedAt int32, err error) {
	ir, ok := ast.(*Instr)
	if !ok {
		ir, err = e.lower(ast)
		if err != nil {
			return Value{}, nil, 0, fmt.Errorf("compile error: %w", err)
		}
	}
	defer func() {
		r := recover()
		if r != nil {
			result = Value{}
			newGlobals = nil
			failedAt = 0
			err = fmt.Errorf("panic in script: %v", r)
		}
	}()

	// Initialize the global environment linked to the Engine's SymbolTable. The
	// pool has New set, so Get never returns nil, and Reset also initializes a
	// virgin *GlobalEnv.
	root := e.envPool.Get().(*GlobalEnv)
	root.Reset(e.symbols)
	defer e.envPool.Put(root)

	for k, v := range globals {
		root.Define(k, v)
	}

	cfg = cfg.withDefaults()

	runCtx := ctx
	var cancel context.CancelFunc
	if cfg.Timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}

	ev := newEvaluator(runCtx, cfg, root, e.builtins)

	result, err = ev.eval(ir)
	if err != nil {
		switch sig := err.(type) {
		case *exitSignal:
			result = sig.Value
			err = nil
		case *returnSignal:
			// Catch returnSignal at top level - acts like exit
			result = sig.Value
			err = nil
		default:
			return Value{}, nil, ev.failedAt, err
		}
	}

	newGlobals = root.ToMap()
	return result, newGlobals, 0, nil
}
