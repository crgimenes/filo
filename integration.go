package filo

import (
	"context"
	"errors"
	"fmt"
	"log"
	"maps"
	"sort"
	"time"
)

// Script represents a pre-parsed Filo script that can be executed multiple times
// with different globals. This is similar to Go's html/template pattern.
type Script struct {
	name    string
	rawAST  Node     // Original AST (unbound)
	program *Program // Cached bound program (optimized)
}

// newScript creates a new named Script. The script must be parsed before execution.
func newScript(name string) *Script {
	return &Script{name: name}
}

// Name returns the script's name.
func (s *Script) Name() string {
	return s.name
}

// Parse parses the source code and stores the AST in the Script.
// Returns the Script for method chaining.
func (s *Script) Parse(src string) (*Script, error) {
	ast, err := Parse(src)
	if err != nil {
		return nil, err
	}
	s.rawAST = ast
	s.program = nil // Invalidate cache
	return s, nil
}

// Execute runs the pre-parsed script with the given engine, globals, and config.
func (s *Script) Execute(ctx context.Context, eng *Engine, globals map[string]Value, cfg EvalConfig) (result Value, newGlobals map[string]Value, err error) {
	if s.rawAST == nil {
		return Value{}, nil, &ParseError{Message: "script not parsed"}
	}

	// Lazy Compilation / JIT
	// If we haven't compiled for this engine yet, do it now.
	// NOTE: We assume Script is typically used with a single Engine instance.
	// If switched, we re-compile. This is safe but incurs a one-time cost per engine.
	if s.program == nil || s.program.eng != eng {
		prog, err := eng.CompileAST(s.rawAST)
		if err != nil {
			return Value{}, nil, fmt.Errorf("jit compile error: %w", err)
		}
		s.program = prog
	}

	// Delegate to optimized program
	return s.program.Execute(ctx, globals, cfg)
}

// ParseScript is a convenience function that creates a new Script and parses it.
func ParseScript(name, src string) (*Script, error) {
	return newScript(name).Parse(src)
}

// Must is a helper that wraps a call to ParseScript and panics if the error is non-nil.
// It is intended for use in variable initializations.
func Must(s *Script, err error) *Script {
	if err != nil {
		panic(err)
	}
	return s
}

type Filo struct {
	eng     *Engine
	globals map[string]Value
}

func must[T any](v T, err error) T {
	if err != nil {
		log.Panicf("%v", err)
	}
	return v
}

var ErrorFunctionNotFound = errors.New("function not found")

// integrationConfig bounds every execution made through the high-level Filo
// API. Tighter than the engine defaults on purpose: config files are small.
var integrationConfig = EvalConfig{
	StepLimit:      10000,
	RecursionLimit: 64,
	Timeout:        5 * time.Second,
}

// goValue converts the Go scalar types shared by SetGlobal and CallFunction to
// a Value; ok reports whether the type was handled.
func goValue(v any) (val Value, ok bool) {
	switch x := v.(type) {
	case string:
		return VString(x), true
	case int:
		return VNum(float64(x)), true
	case int64:
		return VNum(float64(x)), true
	case float32:
		return VNum(float64(x)), true
	case float64:
		return VNum(x), true
	case bool:
		return VBool(x), true
	}
	return Value{}, false
}

// New creates a new Filo instance for configuration loading.
func New() *Filo {
	return &Filo{
		eng:     NewEngine(),
		globals: make(map[string]Value),
	}
}

// Close is a no-op for Filo but provided for API compatibility.
func (f *Filo) Close() {
	// No cleanup needed for Filo
}

// RegisterBuiltin registers a custom builtin function in the Filo engine.
func (f *Filo) RegisterBuiltin(name string, fn Builtin) error {
	return f.eng.RegisterBuiltin(name, fn)
}

// GetEngine returns the underlying Filo engine for advanced operations.
func (f *Filo) GetEngine() *Engine {
	return f.eng
}

// SetGlobal sets a global variable that will be available in the Filo script.
func (f *Filo) SetGlobal(name string, value any) {
	val, ok := goValue(value)
	if ok {
		f.globals[name] = val
		return
	}
	switch v := value.(type) {
	case []string:
		vals := make([]Value, len(v))
		for i, s := range v {
			vals[i] = VString(s)
		}
		f.globals[name] = VList(vals)
	case map[string]string:
		// Convert map to list of key-value tuples for Filo
		pairs := make([]Value, 0, len(v))
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			pairs = append(pairs, VTuple([]Value{VString(k), VString(v[k])}))
		}
		f.globals[name] = VList(pairs)
	default:
		f.globals[name] = VString(fmt.Sprintf("%v", v))
	}
}

// Execute executes a pre-parsed script with optional override globals.
// Override globals take precedence over instance globals for this execution only.
// After execution, instance globals are updated with any (set ...) statements.
func (f *Filo) Execute(script *Script, overrideGlobals map[string]Value) error {
	ctx := context.Background()
	cfg := integrationConfig

	// Merge globals: start with instance globals, override with parameter
	mergedGlobals := make(map[string]Value, len(f.globals)+len(overrideGlobals))
	maps.Copy(mergedGlobals, f.globals)
	maps.Copy(mergedGlobals, overrideGlobals)

	_, updatedGlobals, err := script.Execute(ctx, f.eng, mergedGlobals, cfg)
	if err != nil {
		return err
	}

	// Update instance globals with any values set during script execution
	f.globals = updatedGlobals
	return nil
}

// DoString executes a Filo script and updates globals with any (set ...) statements.
// This is a convenience method that parses and executes in one call.
// For scripts executed multiple times, use ParseScript + Execute for better performance.
func (f *Filo) DoString(filoScript string) error {
	ctx := context.Background()
	cfg := integrationConfig

	_, updatedGlobals, err := f.eng.RunScript(ctx, filoScript, f.globals, cfg)
	if err != nil {
		return err
	}
	f.globals = updatedGlobals
	return nil
}

// MustGetString retrieves a global variable as a string or panics.
func (f *Filo) MustGetString(vGlobal string) string {
	return must(f.GetString(vGlobal))
}

func (f *Filo) GetString(vGlobal string) (string, error) {
	v, ok := f.globals[vGlobal]
	if !ok {
		return "", fmt.Errorf("global variable %q not found", vGlobal)
	}
	s, err := v.AsString()
	if err != nil {
		return "", fmt.Errorf("error converting %q to string: %w", vGlobal, err)
	}
	return s, nil
}

// MustGetInt retrieves a global variable as an int or panics.
func (f *Filo) MustGetInt(vGlobal string) int {
	return must(f.GetInt(vGlobal))
}

func (f *Filo) GetInt(vGlobal string) (int, error) {
	v, ok := f.globals[vGlobal]
	if !ok {
		return 0, fmt.Errorf("global variable %q not found", vGlobal)
	}
	n, err := v.AsNumber()
	if err != nil {
		return 0, fmt.Errorf("error converting %q to int: %w", vGlobal, err)
	}
	return int(n), nil
}

// MustGetNumber retrieves a global variable as a float64 or panics.
func (f *Filo) MustGetNumber(vGlobal string) float64 {
	return must(f.GetNumber(vGlobal))
}

// GetNumber retrieves a global Filo number as a float64.
func (f *Filo) GetNumber(vGlobal string) (float64, error) {
	return f.getNumber(vGlobal, "number")
}

func (f *Filo) getNumber(vGlobal, targetType string) (float64, error) {
	v, ok := f.globals[vGlobal]
	if !ok {
		return 0, fmt.Errorf("global variable %q not found", vGlobal)
	}
	n, err := v.AsNumber()
	if err != nil {
		return 0, fmt.Errorf("error converting %q to %s: %w", vGlobal, targetType, err)
	}
	return n, nil
}

// MustGetFloat retrieves a global variable as a float64 or panics.
//
// Deprecated: use MustGetNumber.
func (f *Filo) MustGetFloat(vGlobal string) float64 {
	return must(f.GetFloat(vGlobal))
}

// GetFloat retrieves a global Filo number as a float64.
//
// Deprecated: use GetNumber.
func (f *Filo) GetFloat(vGlobal string) (float64, error) {
	return f.getNumber(vGlobal, "float")
}

// MustGetBool retrieves a global variable as a bool or panics.
func (f *Filo) MustGetBool(vGlobal string) bool {
	return must(f.GetBool(vGlobal))
}

func (f *Filo) GetBool(vGlobal string) (bool, error) {
	v, ok := f.globals[vGlobal]
	if !ok {
		return false, fmt.Errorf("global variable %q not found", vGlobal)
	}
	b, err := v.AsBool()
	if err != nil {
		return false, fmt.Errorf("error converting %q to bool: %w", vGlobal, err)
	}
	return b, nil
}

// MustGetTable retrieves a global variable as a []string or panics.
func (f *Filo) MustGetTable(vGlobal string) []string {
	return must(f.GetTable(vGlobal))
}

func (f *Filo) GetTable(vGlobal string) ([]string, error) {
	v, ok := f.globals[vGlobal]
	if !ok {
		return nil, fmt.Errorf("global variable %q not found", vGlobal)
	}
	list, err := v.AsList()
	if err != nil {
		return nil, fmt.Errorf("error converting %q to list: %w", vGlobal, err)
	}
	ret := make([]string, len(list))
	for i, item := range list {
		s, convErr := item.AsString()
		if convErr != nil {
			return nil, fmt.Errorf("error converting list item %d to string: %w", i, convErr)
		}
		ret[i] = s
	}
	return ret, nil
}

// MustGetMap retrieves a global variable as a map[string]string or panics.
func (f *Filo) MustGetMap(vGlobal string) map[string]string {
	return must(f.GetMap(vGlobal))
}

func (f *Filo) GetMap(vGlobal string) (map[string]string, error) {
	v, ok := f.globals[vGlobal]
	if !ok {
		return nil, fmt.Errorf("global variable %q not found", vGlobal)
	}
	list, err := v.AsList()
	if err != nil {
		return nil, fmt.Errorf("error converting %q to list: %w", vGlobal, err)
	}
	ret := make(map[string]string)
	for i, item := range list {
		tuple, tupleErr := item.AsTuple()
		if tupleErr != nil {
			return nil, fmt.Errorf("error converting map item %d to tuple: %w", i, tupleErr)
		}
		if len(tuple) != 2 {
			return nil, fmt.Errorf("map tuple item %d must have exactly 2 elements", i)
		}
		key, keyErr := tuple[0].AsString()
		if keyErr != nil {
			return nil, fmt.Errorf("error converting map key %d to string: %w", i, keyErr)
		}
		val, valErr := tuple[1].AsString()
		if valErr != nil {
			return nil, fmt.Errorf("error converting map value %d to string: %w", i, valErr)
		}
		ret[key] = val
	}
	return ret, nil
}

// SetGlobalMapOfLists sets a global variable from a map[string][]string.
// The structure is stored as a list of (key, list-of-values) tuples.
// Example: {"a": ["x", "y"]} becomes (list (list "a" (list "x" "y")))
func (f *Filo) SetGlobalMapOfLists(name string, m map[string][]string) {
	pairs := make([]Value, 0, len(m))
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		vals := m[k]
		valList := make([]Value, len(vals))
		for i, v := range vals {
			valList[i] = VString(v)
		}
		pairs = append(pairs, VList([]Value{VString(k), VList(valList)}))
	}
	f.globals[name] = VList(pairs)
}

// MustGetMapOfLists retrieves a global variable as a map[string][]string or panics.
// Expects the structure: (list (list "key" (list "val1" "val2")) ...)
func (f *Filo) MustGetMapOfLists(vGlobal string) map[string][]string {
	return must(f.GetMapOfLists(vGlobal))
}

func (f *Filo) GetMapOfLists(vGlobal string) (map[string][]string, error) {
	v, ok := f.globals[vGlobal]
	if !ok {
		return nil, fmt.Errorf("global variable %q not found", vGlobal)
	}
	list, err := v.AsList()
	if err != nil {
		return nil, fmt.Errorf("error converting %q to list: %w", vGlobal, err)
	}
	ret := make(map[string][]string)
	for i, item := range list {
		tuple, tupleErr := item.AsList()
		if tupleErr != nil {
			return nil, fmt.Errorf("error converting map item %d to list: %w", i, tupleErr)
		}
		if len(tuple) != 2 {
			return nil, fmt.Errorf("map list item %d must have exactly 2 elements", i)
		}
		key, keyErr := tuple[0].AsString()
		if keyErr != nil {
			return nil, fmt.Errorf("error converting map key %d to string: %w", i, keyErr)
		}
		valList, valErr := tuple[1].AsList()
		if valErr != nil {
			return nil, fmt.Errorf("error converting map value %d to list: %w", i, valErr)
		}
		vals := make([]string, len(valList))
		for j, val := range valList {
			s, sErr := val.AsString()
			if sErr != nil {
				return nil, fmt.Errorf("error converting value list item %d.%d to string: %w", i, j, sErr)
			}
			vals[j] = s
		}
		ret[key] = vals
	}
	return ret, nil
}

// HasFunction checks if a function with the given name is defined in globals.
func (f *Filo) HasFunction(name string) bool {
	v, ok := f.globals[name]
	if !ok {
		return false
	}
	return v.Kind == KFunc
}

// CallFunction invokes a Filo-defined function by name with the given arguments.
// Arguments are converted from Go types to Filo Values.
// Returns the result Value or an error if the function doesn't exist or fails.
func (f *Filo) CallFunction(name string, args ...any) (Value, error) {
	fnVal, ok := f.globals[name]
	if !ok {
		return Value{}, fmt.Errorf("%w: %s", ErrorFunctionNotFound, name)
	}
	if fnVal.Kind != KFunc {
		return Value{}, fmt.Errorf("%s is not a function", name)
	}

	// Convert Go args to Filo Values
	filoArgs := make([]Value, len(args))
	for i, arg := range args {
		val, ok := goValue(arg)
		if ok {
			filoArgs[i] = val
			continue
		}
		switch v := arg.(type) {
		case []byte:
			filoArgs[i] = VString(string(v))
		case Value:
			filoArgs[i] = v
		default:
			filoArgs[i] = VString(fmt.Sprintf("%v", v))
		}
	}

	// Build call expression
	var callExpr string
	callExpr = "(" + name
	for i := range filoArgs {
		callExpr += fmt.Sprintf(" arg%d", i)
	}
	callExpr += ")"

	// Set up globals with args
	callGlobals := make(map[string]Value, len(f.globals)+len(filoArgs))
	maps.Copy(callGlobals, f.globals)
	for i, v := range filoArgs {
		callGlobals[fmt.Sprintf("arg%d", i)] = v
	}

	ctx := context.Background()

	result, _, err := f.eng.RunScript(ctx, callExpr, callGlobals, integrationConfig)
	if err != nil {
		return Value{}, fmt.Errorf("error calling %s: %w", name, err)
	}

	return result, nil
}

// CallFunctionString is a convenience wrapper that calls a function and converts
// the result to a string. If the function doesn't exist or returns a non-string,
// the fallback string is returned.
func (f *Filo) CallFunctionString(name string, fallback string, args ...any) string {
	if !f.HasFunction(name) {
		return fallback
	}
	result, err := f.CallFunction(name, args...)
	if err != nil {
		return fallback
	}
	s, err := result.AsString()
	if err != nil {
		return fallback
	}
	return s
}
