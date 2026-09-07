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

func (v Value) String() string {
	switch v.Kind {
	case KNumber:
		return strconv.FormatFloat(v.Num, 'g', -1, 64)
	case KBool:
		if v.Bool {
			return "#t"
		}
		return "#f"
	case KString:
		return fmt.Sprintf("%q", v.Str)
	case KList:
		var b strings.Builder
		b.WriteString("(list")
		for _, e := range v.List {
			b.WriteByte(' ')
			b.WriteString(e.String())
		}
		b.WriteByte(')')
		return b.String()
	case KTuple:
		var b strings.Builder
		b.WriteString("(tuple")
		for _, e := range v.Tup {
			b.WriteByte(' ')
			b.WriteString(e.String())
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
	return v.String(), nil
}

func ensureSameKind(values []Value) error {
	if len(values) == 0 {
		return errors.New("empty value list")
	}
	target := values[0].Kind
	for i := 1; i < len(values); i++ {
		if values[i].Kind != target {
			return fmt.Errorf("expected values of the same kind, got %v and %v", target, values[i].Kind)
		}
	}
	return nil
}
