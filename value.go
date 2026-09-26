package filo

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

type Kind int

const (
	KNumber Kind = iota
	KBool
	KString
	KList
	KTuple
	KFunc
)

type Func struct {
	Params []string
	Body   []*Instr
	Frame  *Frame
	// a builtin used as a value: it takes what its own checks take, and is
	// the same value everywhere it is named (so (= floor floor) is true)
	builtin builtinFunc
	name    string
	bc      *bcFunc // a function of a unit of bytecode
}

type Value struct {
	Kind Kind
	Num  float64
	Bool bool
	Str  string
	List []Value
	Tup  []Value
	Fn   *Func
}

func VNum(v float64) Value {
	return Value{Kind: KNumber, Num: v}
}

func VBool(v bool) Value {
	return Value{Kind: KBool, Bool: v}
}

func VString(v string) Value {
	return Value{Kind: KString, Str: v}
}

func VList(v []Value) Value {
	return Value{Kind: KList, List: v}
}

func VTuple(v []Value) Value {
	return Value{Kind: KTuple, Tup: v}
}

func VFunc(fn *Func) Value {
	return Value{Kind: KFunc, Fn: fn}
}

func (v Value) AsNumber() (float64, error) {
	if v.Kind != KNumber {
		return 0, fmt.Errorf("expected number, got %s", v.describe())
	}
	return v.Num, nil
}

func (v Value) AsBool() (bool, error) {
	if v.Kind != KBool {
		return false, fmt.Errorf("expected bool, got %s", v.describe())
	}
	return v.Bool, nil
}

func (v Value) AsString() (string, error) {
	if v.Kind != KString {
		return "", fmt.Errorf("expected string, got %s", v.describe())
	}
	return v.Str, nil
}

func (v Value) AsList() ([]Value, error) {
	if v.Kind != KList {
		return nil, fmt.Errorf("expected list, got %s", v.describe())
	}
	return v.List, nil
}

func (v Value) AsTuple() ([]Value, error) {
	if v.Kind != KTuple {
		return nil, fmt.Errorf("expected tuple, got %s", v.describe())
	}
	return v.Tup, nil
}

func (v Value) describe() string {
	switch v.Kind {
	case KNumber:
		return "number"
	case KBool:
		return "bool"
	case KString:
		return "string"
	case KList:
		return "list"
	case KTuple:
		return "tuple"
	case KFunc:
		return "func"
	default:
		return "unknown"
	}
}

// A list or a tuple may hold the same value more than once, so a value
// made in a few steps, (fold (fn (a x) (tuple a a)) 0 (range 60)), is small
// in memory and 2^60 parts to walk. Whatever walks a value stops at
// walkPartsMax parts and walkLevelsMax levels, the same ceilings as the C
// runtime, checked in the same order, so both fail on the same value:
// writing it and converting it check first (Walkable), comparing it counts
// as it goes (equalWalk).
const (
	// writing 2^18 numbers takes ~1.3 s on the C runtime with libc, the
	// number that sets it (measured 2026-09-26)
	walkPartsMax  = 1 << 18
	walkLevelsMax = 512 // the C runtime's evaluation depth: its stack
)

// Walkable says whether v can be walked: nil when it has at most 262,144
// parts (every value in it, itself included) and 512 levels of lists and
// tuples, or the error saying which ceiling it passed. Counting stops at the
// ceiling, so this costs at most that much. Code outside the engine that
// walks a value (a marshaler, a printer) calls it first.
func (v Value) Walkable() error {
	parts := 0
	return walk(v, 1, &parts)
}

func walk(v Value, level int, parts *int) error {
	*parts++
	if *parts > walkPartsMax {
		return fmt.Errorf("value too large: more than %d parts", walkPartsMax)
	}
	items := v.List
	if v.Kind == KTuple {
		items = v.Tup
	}
	if v.Kind != KList && v.Kind != KTuple {
		return nil
	}
	if level > walkLevelsMax {
		return fmt.Errorf("value too deep: more than %d levels", walkLevelsMax)
	}
	for _, item := range items {
		err := walk(item, level+1, parts)
		if err != nil {
			return err
		}
	}
	return nil
}

// String writes v as Filo reads it back; a value too large to walk is
// written as the reason, in angle brackets.
func (v Value) String() string {
	err := v.Walkable()
	if err != nil {
		return "<" + err.Error() + ">"
	}
	return v.text()
}

func (v Value) text() string {
	switch v.Kind {
	case KNumber:
		return strconv.FormatFloat(v.Num, 'g', -1, 64)
	case KBool:
		if v.Bool {
			return "#t"
		}
		return "#f"
	case KString:
		return formatString(v.Str)
	case KList:
		var b strings.Builder
		b.WriteString("(list")
		for _, e := range v.List {
			b.WriteByte(' ')
			b.WriteString(e.text())
		}
		b.WriteByte(')')
		return b.String()
	case KTuple:
		var b strings.Builder
		b.WriteString("(tuple")
		for _, e := range v.Tup {
			b.WriteByte(' ')
			b.WriteString(e.text())
		}
		b.WriteByte(')')
		return b.String()
	case KFunc:
		return "<fn>"
	default:
		return "<invalid>"
	}
}

// valueToText renders a value as plain text for the (string ...) cast: strings
// pass through unquoted; everything else uses the same textual form as
// Value.String. Functions have no textual value and error.
func valueToText(v Value) (string, error) {
	switch v.Kind {
	case KString:
		return v.Str, nil
	case KFunc:
		return "", errors.New("string: cannot convert a function")
	}
	err := v.Walkable()
	if err != nil {
		return "", err
	}
	return v.text(), nil
}

func ensureSameKind(values []Value) error {
	if len(values) == 0 {
		return errors.New("empty value list")
	}
	target := values[0].Kind
	for i := 1; i < len(values); i++ {
		if values[i].Kind != target {
			return fmt.Errorf("expected values of the same kind, got %s and %s", values[0].describe(), values[i].describe())
		}
	}
	return nil
}
