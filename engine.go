package filo

import (
	"context"
	"fmt"
	"sync"
	"time"
)

type Engine struct {
	builtins map[string]builtinFunc
	symbols  *SymbolTable
	envPool  sync.Pool
}

type EvalConfig struct {
	StepLimit      int
	RecursionLimit int
	Timeout        time.Duration
}

const (
	defaultStepLimit      = 100_000
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
	ast Node
	eng *Engine
}

// Compile parses and compiles the source code into a reuseable Program.
func (e *Engine) Compile(src string) (*Program, error) {
	ast, err := Parse(src)
	if err != nil {
		return nil, err
	}
	return e.CompileAST(ast)
}

// CompileAST compiles a parsed AST into a reusable Program bound to this Engine.
// Constant subexpressions are folded first; folding is semantics-preserving (it
// only replaces a call when evaluating it with constant arguments succeeds), so
// there is no knob to turn it off.
func (e *Engine) CompileAST(ast Node) (*Program, error) {
	folded, _ := FoldConstants(ast)
	compiled, err := Compile(folded, e.builtins, e.symbols)
	if err != nil {
		return nil, fmt.Errorf("compile error: %w", err)
	}
	return &Program{ast: compiled, eng: e}, nil
}

// Execute runs the compiled program.
func (p *Program) Execute(ctx context.Context, globals map[string]Value, cfg EvalConfig) (result Value, newGlobals map[string]Value, err error) {
	return p.eng.ExecuteAST(ctx, p.ast, globals, cfg)
}

func (e *Engine) RunScript(ctx context.Context, src string, globals map[string]Value, cfg EvalConfig) (result Value, newGlobals map[string]Value, err error) {
	prog, err := e.Compile(src)
	if err != nil {
		return Value{}, nil, err
	}
	return prog.Execute(ctx, globals, cfg)
}

// ExecuteAST executes a pre-parsed (and compiled) AST with the given globals.
// This is the core execution method used by both RunScript and Script.Execute.
func (e *Engine) ExecuteAST(ctx context.Context, ast Node, globals map[string]Value, cfg EvalConfig) (result Value, newGlobals map[string]Value, err error) {
	defer func() {
		r := recover()
		if r != nil {
			result = Value{}
			newGlobals = nil
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

	// Eval with single argument (state needed is inside ev)
	result, err = ev.eval(ast)
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
			return Value{}, nil, err
		}
	}

	newGlobals = root.ToMap()
	return result, newGlobals, nil
}
