package filo

import (
	"context"
	"fmt"
	"math"
)

type Builtin func(ctx context.Context, args []Value) (Value, error)

type builtinFunc func(ctx context.Context, ev *evaluator, args []Value) (Value, error)

func defaultBuiltins() map[string]builtinFunc {
	bi := map[string]builtinFunc{}

	bi["+"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		var sum float64
		for _, a := range args {
			val, err := a.AsNumber()
			if err != nil {
				return Value{}, err
			}
			sum += val
		}
		return VNum(sum), nil
	}

	bi["-"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) == 0 {
			return Value{}, fmt.Errorf("- expects at least one argument")
		}
		first, err := args[0].AsNumber()
		if err != nil {
			return Value{}, err
		}
		if len(args) == 1 {
			return VNum(-first), nil
		}
		result := first
		for i := 1; i < len(args); i++ {
			val, convErr := args[i].AsNumber()
			if convErr != nil {
				return Value{}, convErr
			}
			result -= val
		}
		return VNum(result), nil
	}

	bi["*"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		result := 1.0
		for _, a := range args {
			val, err := a.AsNumber()
			if err != nil {
				return Value{}, err
			}
			result *= val
		}
		return VNum(result), nil
	}

	bi["/"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) == 0 {
			return Value{}, fmt.Errorf("/ expects at least one argument")
		}
		first, err := args[0].AsNumber()
		if err != nil {
			return Value{}, err
		}
		result := first
		for i := 1; i < len(args); i++ {
			val, convErr := args[i].AsNumber()
			if convErr != nil {
				return Value{}, convErr
			}
			if val == 0 {
				return Value{}, fmt.Errorf("division by zero")
			}
			result /= val
		}
		return VNum(result), nil
	}

	bi["%"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 2 {
			return Value{}, fmt.Errorf("%% expects two arguments")
		}
		a, err := args[0].AsNumber()
		if err != nil {
			return Value{}, err
		}
		b, err := args[1].AsNumber()
		if err != nil {
			return Value{}, err
		}
		if b == 0 {
			return Value{}, fmt.Errorf("modulo by zero")
		}
		return VNum(math.Mod(a, b)), nil
	}

	bi["pow"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 2 {
			return Value{}, fmt.Errorf("pow expects two arguments")
		}
		a, err := args[0].AsNumber()
		if err != nil {
			return Value{}, err
		}
		b, err := args[1].AsNumber()
		if err != nil {
			return Value{}, err
		}
		return VNum(math.Pow(a, b)), nil
	}

	bi["="] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) < 2 {
			return Value{}, fmt.Errorf("= expects at least two arguments")
		}
		_, err := ensureSameKind(args)
		if err != nil {
			return Value{}, err
		}
		first := args[0]
		for i := 1; i < len(args); i++ {
			if !valueEqual(first, args[i]) {
				return VBool(false), nil
			}
		}
		return VBool(true), nil
	}

	bi["!="] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		eq, err := bi["="](ctx, nil, args)
		if err != nil {
			return Value{}, err
		}
		return VBool(!eq.Bool), nil
	}

	bi["<"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) < 2 {
			return Value{}, fmt.Errorf("< expects at least two arguments")
		}
		for i := 1; i < len(args); i++ {
			left, err := args[i-1].AsNumber()
			if err != nil {
				return Value{}, err
			}
			right, err := args[i].AsNumber()
			if err != nil {
				return Value{}, err
			}
			if !(left < right) {
				return VBool(false), nil
			}
		}
		return VBool(true), nil
	}

	bi["<="] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) < 2 {
			return Value{}, fmt.Errorf("<= expects at least two arguments")
		}
		for i := 1; i < len(args); i++ {
			left, err := args[i-1].AsNumber()
			if err != nil {
				return Value{}, err
			}
			right, err := args[i].AsNumber()
			if err != nil {
				return Value{}, err
			}
			if !(left <= right) {
				return VBool(false), nil
			}
		}
		return VBool(true), nil
	}

	bi[">"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) < 2 {
			return Value{}, fmt.Errorf("> expects at least two arguments")
		}
		for i := 1; i < len(args); i++ {
			left, err := args[i-1].AsNumber()
			if err != nil {
				return Value{}, err
			}
			right, err := args[i].AsNumber()
			if err != nil {
				return Value{}, err
			}
			if !(left > right) {
				return VBool(false), nil
			}
		}
		return VBool(true), nil
	}

	bi[">="] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) < 2 {
			return Value{}, fmt.Errorf(">= expects at least two arguments")
		}
		for i := 1; i < len(args); i++ {
			left, err := args[i-1].AsNumber()
			if err != nil {
				return Value{}, err
			}
			right, err := args[i].AsNumber()
			if err != nil {
				return Value{}, err
			}
			if !(left >= right) {
				return VBool(false), nil
			}
		}
		return VBool(true), nil
	}

	bi["not"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("not expects one argument")
		}
		v, err := args[0].AsBool()
		if err != nil {
			return Value{}, err
		}
		return VBool(!v), nil
	}

	bi["and"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		for _, a := range args {
			v, err := a.AsBool()
			if err != nil {
				return Value{}, err
			}
			if !v {
				return VBool(false), nil
			}
		}
		return VBool(true), nil
	}

	bi["or"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		for _, a := range args {
			v, err := a.AsBool()
			if err != nil {
				return Value{}, err
			}
			if v {
				return VBool(true), nil
			}
		}
		return VBool(false), nil
	}

	bi["type-of"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("type-of expects 1 argument")
		}
		switch args[0].Kind {
		case KNumber:
			return VString("number"), nil
		case KString:
			return VString("string"), nil
		case KBool:
			return VString("bool"), nil
		case KList:
			return VString("list"), nil
		case KTuple:
			return VString("tuple"), nil
		case KFunc:
			return VString("func"), nil
		default:
			return VString("unknown"), nil
		}
	}

	bi["is-empty"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("is-empty expects 1 argument")
		}
		val := args[0]
		switch val.Kind {
		case KString:
			return VBool(len(val.Str) == 0), nil
		case KList:
			return VBool(len(val.List) == 0), nil
		case KTuple:
			return VBool(len(val.Tup) == 0), nil
		default:
			return VBool(false), nil
		}
	}

	bi["is-nil"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("is-nil expects 1 argument")
		}
		// In Filo, we don't have a specific nil/null type yet, but empty list/string checks covers most "empty" cases.
		// For now, this behaves similarly to checking if something is "missing" or zero-value.
		// However, without a true NIL value, is-empty is often what users want.
		// Let's defer is-nil logic unless we introduce a real NIL type.
		// For now, checks for empty list which is the closest to NIL in Lisp.
		return VBool(args[0].Kind == KList && len(args[0].List) == 0), nil
	}

	bi["list-append"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 2 {
			return Value{}, fmt.Errorf("list-append expects 2 arguments (list, value)")
		}
		list, err := args[0].AsList()
		if err != nil {
			return Value{}, fmt.Errorf("list-append: first argument must be list: %w", err)
		}
		// Create new list to preserve immutability
		newList := make([]Value, len(list)+1)
		copy(newList, list)
		newList[len(list)] = args[1]
		return VList(newList), nil
	}

	bi["list-concat"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) < 2 {
			return Value{}, fmt.Errorf("list-concat expects at least 2 arguments")
		}
		totalLen := 0
		for i, arg := range args {
			l, err := arg.AsList()
			if err != nil {
				return Value{}, fmt.Errorf("list-concat: argument %d is not a list", i)
			}
			totalLen += len(l)
		}
		newList := make([]Value, 0, totalLen)
		for _, arg := range args {
			l, _ := arg.AsList() // already checked
			newList = append(newList, l...)
		}
		return VList(newList), nil
	}

	bi["list"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		return VList(args), nil
	}

	bi["length"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("length expects one argument")
		}
		list, err := args[0].AsList()
		if err == nil {
			return VNum(float64(len(list))), nil
		}
		tuple, tupleErr := args[0].AsTuple()
		if tupleErr != nil {
			return Value{}, fmt.Errorf("length expects list or tuple")
		}
		_ = err
		return VNum(float64(len(tuple))), nil
	}

	bi["head"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("head expects one argument")
		}
		list, err := args[0].AsList()
		if err != nil {
			return Value{}, err
		}
		if len(list) == 0 {
			return Value{}, fmt.Errorf("head of empty list")
		}
		return list[0], nil
	}

	bi["tail"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("tail expects one argument")
		}
		list, err := args[0].AsList()
		if err != nil {
			return Value{}, err
		}
		if len(list) == 0 {
			return Value{}, fmt.Errorf("tail of empty list")
		}
		return VList(list[1:]), nil
	}

	bi["nth"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 2 {
			return Value{}, fmt.Errorf("nth expects two arguments")
		}
		list, err := args[0].AsList()
		if err != nil {
			return Value{}, err
		}
		idx, err := args[1].AsNumber()
		if err != nil {
			return Value{}, err
		}
		i := int(idx)
		if i < 0 || i >= len(list) {
			return Value{}, fmt.Errorf("index out of range")
		}
		return list[i], nil
	}

	bi["map"] = func(ctx context.Context, ev *evaluator, args []Value) (Value, error) {
		if len(args) != 2 {
			return Value{}, fmt.Errorf("map expects function and list")
		}
		fn := args[0]
		if fn.Kind != KFunc {
			return Value{}, fmt.Errorf("map expects function as first argument")
		}
		list, err := args[1].AsList()
		if err != nil {
			return Value{}, err
		}
		result := make([]Value, len(list))
		for i, el := range list {
			val, callErr := ev.callFunc(ctx, fn.Fn, []Value{el})
			if callErr != nil {
				return Value{}, callErr
			}
			result[i] = val
		}
		return VList(result), nil
	}

	bi["fold"] = func(ctx context.Context, ev *evaluator, args []Value) (Value, error) {
		if len(args) != 3 {
			return Value{}, fmt.Errorf("fold expects function, initial value, and list")
		}
		fn := args[0]
		if fn.Kind != KFunc {
			return Value{}, fmt.Errorf("fold expects function as first argument")
		}
		acc := args[1]
		list, err := args[2].AsList()
		if err != nil {
			return Value{}, err
		}
		current := acc
		for _, el := range list {
			val, callErr := ev.callFunc(ctx, fn.Fn, []Value{current, el})
			if callErr != nil {
				return Value{}, callErr
			}
			current = val
		}
		return current, nil
	}

	return bi
}

func valueEqual(a, b Value) bool {
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case KNumber:
		return a.Num == b.Num
	case KBool:
		return a.Bool == b.Bool
	case KString:
		return a.Str == b.Str
	case KList:
		if len(a.List) != len(b.List) {
			return false
		}
		for i := range a.List {
			if !valueEqual(a.List[i], b.List[i]) {
				return false
			}
		}
		return true
	case KTuple:
		if len(a.Tup) != len(b.Tup) {
			return false
		}
		for i := range a.Tup {
			if !valueEqual(a.Tup[i], b.Tup[i]) {
				return false
			}
		}
		return true
	case KFunc:
		return a.Fn == b.Fn
	default:
		return false
	}
}
