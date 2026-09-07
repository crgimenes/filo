package filo

// The IR is a tree of instructions: every runtime dispatches on Op as an
// integer and reads typed operands. docs/ir.md is the contract; this file is
// the Go shape of it. One struct for every opcode keeps the layout identical
// to the C runtime's tagged union, which is the point of sharing an IR.

// Opcode selects what an Instr does. Values are part of the contract only by
// name; the numbers are free to differ between runtimes.
type Opcode uint8

const (
	OpConst Opcode = iota
	OpLocal
	OpGlobal
	OpDynamic
	OpBuiltin
	OpEmpty
	OpInvalid
	OpIf
	OpCond
	OpDo
	OpAnd
	OpOr
	OpLet
	OpLetv
	OpSet
	OpFn
	OpDef
	OpTuple
	OpExit
	OpReturn
	OpCallB
	OpCall
)

// Instr is one instruction. Which fields are meaningful depends on Op:
//
//	OpConst   Val
//	OpLocal   A (depth), B (index), Name
//	OpGlobal  A (id), Name
//	OpDynamic Name
//	OpBuiltin Name
//	OpInvalid Name (context), Msg
//	OpIf, OpDo, OpAnd, OpOr, OpSet, OpExit, OpReturn   Args
//	OpCond    Clauses
//	OpLet     A (binding count), Args (values then body)
//	OpLetv    Names, Args (tuple then body)
//	OpFn      Names (params), Args (body)
//	OpDef     Name (empty when the name was not a symbol), Args (value)
//	OpTuple   Name (context: "values" or "tuple"), Args
//	OpCallB   Name, Fn, Args
//	OpCall    Args (head then arguments)
type Instr struct {
	Op      Opcode
	A, B    int
	Name    string
	Msg     string
	Val     Value
	Names   []string
	Args    []*Instr
	Clauses []Clause
	Fn      builtinFunc
}

// Clause is one arm of a cond. Exactly one of Invalid, Else or Test applies:
// an Invalid clause errors with Msg when reached; an Else clause runs Body;
// otherwise Test decides.
type Clause struct {
	Invalid bool
	Msg     string
	Else    bool
	Test    *Instr
	Body    []*Instr
}
