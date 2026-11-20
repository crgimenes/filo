// Package filo implements Filo, a small embeddable Lisp for Go: a scripting and
// configuration language with minimal syntax, deterministic execution, and
// explicit limits on every dimension that could put a host process at risk.
//
// A host embeds Filo to let end users write short logic — validations,
// expressions, field rules, configuration — without ever exposing the machine.
// Scripts run under a StepLimit, a RecursionLimit, and a Timeout (honored via
// context.Context), the executor recovers from panics, and the only capabilities
// a script has are the Go functions the host explicitly registers as builtins:
// no file, network, or syscall access.
//
// The high-level entry point is New / DoString / MustGet* (see core/integration.go);
// for compiled reuse and full control over globals and limits, use Engine,
// Compile, and RunScript (see core/engine.go). Go values convert to and from Filo
// with Marshal / Unmarshal and their Value-level forms. Optional builtin sets
// live in the filostrings, filomath, filorand, filojson, and filoprint
// subpackages; source formatting lives in the filofmt command.
//
// The implementation, and the documentation of each symbol, live in package core.
package filo

import "github.com/crgimenes/filo/core"

type (
	BoolLit        = core.BoolLit
	BuildEntry     = core.BuildEntry
	Builtin        = core.Builtin
	BundleMember   = core.BundleMember
	Clause         = core.Clause
	Engine         = core.Engine
	EvalConfig     = core.EvalConfig
	Filo           = core.Filo
	FormatConfig   = core.FormatConfig
	Frame          = core.Frame
	Func           = core.Func
	GlobalEnv      = core.GlobalEnv
	Instr          = core.Instr
	Kind           = core.Kind
	List           = core.List
	MarshalOptions = core.MarshalOptions
	Node           = core.Node
	NumberLit      = core.NumberLit
	Opcode         = core.Opcode
	ParseError     = core.ParseError
	PositionError  = core.PositionError
	Program        = core.Program
	Scope          = core.Scope
	Script         = core.Script
	StepFrame      = core.StepFrame
	StepState      = core.StepState
	Stepper        = core.Stepper
	StringLit      = core.StringLit
	Symbol         = core.Symbol
	SymbolTable    = core.SymbolTable
	Unit           = core.Unit
	Value          = core.Value
)

const (
	KNumber = core.KNumber
	KBool   = core.KBool
	KString = core.KString
	KList   = core.KList
	KTuple  = core.KTuple
	KFunc   = core.KFunc
)

const (
	OpConst   = core.OpConst
	OpLocal   = core.OpLocal
	OpGlobal  = core.OpGlobal
	OpDynamic = core.OpDynamic
	OpBuiltin = core.OpBuiltin
	OpEmpty   = core.OpEmpty
	OpInvalid = core.OpInvalid
	OpIf      = core.OpIf
	OpCond    = core.OpCond
	OpDo      = core.OpDo
	OpAnd     = core.OpAnd
	OpOr      = core.OpOr
	OpLet     = core.OpLet
	OpLetv    = core.OpLetv
	OpSet     = core.OpSet
	OpFn      = core.OpFn
	OpDef     = core.OpDef
	OpTuple   = core.OpTuple
	OpExit    = core.OpExit
	OpReturn  = core.OpReturn
	OpCallB   = core.OpCallB
	OpCall    = core.OpCall
)

var ErrorFunctionNotFound = core.ErrorFunctionNotFound

// Compile is a variable because its builtins map has an unexported value type.
var Compile = core.Compile

func New() *Filo { return core.New() }

func NewEngine() *Engine { return core.NewEngine() }

func NewGlobalEnv(symbols *SymbolTable) *GlobalEnv { return core.NewGlobalEnv(symbols) }

func NewSymbolTable() *SymbolTable { return core.NewSymbolTable() }

func Parse(input string) (Node, error) { return core.Parse(input) }

func ParseScript(name, src string) (*Script, error) { return core.ParseScript(name, src) }

func Must(s *Script, err error) *Script { return core.Must(s, err) }

func FoldConstants(node Node) (Node, bool) { return core.FoldConstants(node) }

func BuildBundle(members []BundleMember) ([]byte, error) { return core.BuildBundle(members) }

func StripDebug(unit []byte) ([]byte, error) { return core.StripDebug(unit) }

func DefaultFormatConfig() FormatConfig { return core.DefaultFormatConfig() }

func Format(src string) (string, error) { return core.Format(src) }

func FormatAST(node Node, cfg FormatConfig) (string, error) { return core.FormatAST(node, cfg) }

func FormatWithConfig(src string, cfg FormatConfig) (string, error) {
	return core.FormatWithConfig(src, cfg)
}

func FormatValue(v Value) string { return core.FormatValue(v) }

func FormatValueIndent(v Value, prefix, indent string) string {
	return core.FormatValueIndent(v, prefix, indent)
}

func Marshal(v any) (string, error) { return core.Marshal(v) }

func MarshalIndent(v any, prefix, indent string) (string, error) {
	return core.MarshalIndent(v, prefix, indent)
}

func MarshalWithOptions(v any, opts MarshalOptions) (string, error) {
	return core.MarshalWithOptions(v, opts)
}

func MarshalToValue(v any) (Value, error) { return core.MarshalToValue(v) }

func Unmarshal(data string, target any) error { return core.Unmarshal(data, target) }

func UnmarshalFromValue(val Value, target any) error { return core.UnmarshalFromValue(val, target) }

func VBool(v bool) Value { return core.VBool(v) }

func VFunc(fn *Func) Value { return core.VFunc(fn) }

func VList(v []Value) Value { return core.VList(v) }

func VNum(v float64) Value { return core.VNum(v) }

func VString(v string) Value { return core.VString(v) }

func VTuple(v []Value) Value { return core.VTuple(v) }
