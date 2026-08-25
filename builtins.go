package filo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type Builtin func(ctx context.Context, args []Value) (Value, error)

type builtinFunc func(ctx context.Context, ev *evaluator, args []Value) (Value, error)

func checkContext(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("execution cancelled: %w", ctx.Err())
	default:
		return nil
	}
}

//nolint:gocognit // flat registry: the score is the sum of dozens of independent builtin closures with no shared control flow; once-called registrars would add indirection only
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
			return Value{}, fmt.Errorf("- expects at least 1 argument")
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
			return Value{}, fmt.Errorf("/ expects at least 1 argument")
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
			return Value{}, fmt.Errorf("%% expects 2 arguments")
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
		// Floored modulo, as in Lua: the result takes the divisor's sign, so
		// (% -1 2) is 1. Go's math.Mod truncates instead (sign of the
		// dividend); adjust when the signs disagree.
		r := math.Mod(a, b)
		if r != 0 && (r < 0) != (b < 0) {
			r += b
		}
		return VNum(r), nil
	}

	bi["pow"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 2 {
			return Value{}, fmt.Errorf("pow expects 2 arguments")
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
			return Value{}, fmt.Errorf("= expects at least 2 arguments")
		}
		err := ensureSameKind(args)
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
			return Value{}, fmt.Errorf("< expects at least 2 arguments")
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
			return Value{}, fmt.Errorf("<= expects at least 2 arguments")
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
			return Value{}, fmt.Errorf("> expects at least 2 arguments")
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
			return Value{}, fmt.Errorf(">= expects at least 2 arguments")
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
			return Value{}, fmt.Errorf("not expects 1 argument")
		}
		v, err := args[0].AsBool()
		if err != nil {
			return Value{}, err
		}
		return VBool(!v), nil
	}

	// "and" and "or" are special forms (see evaluator.go): they short-circuit,
	// so they cannot live here — a builtin receives its arguments already
	// evaluated.

	bi["string"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("string expects 1 argument")
		}
		s, err := valueToText(args[0])
		if err != nil {
			return Value{}, err
		}
		return VString(s), nil
	}

	bi["number"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("number expects 1 argument")
		}
		switch args[0].Kind {
		case KNumber:
			return args[0], nil
		case KString:
			n, err := strconv.ParseFloat(strings.TrimSpace(args[0].Str), 64)
			if err != nil {
				return Value{}, fmt.Errorf("number: cannot parse %q", args[0].Str)
			}
			return VNum(n), nil
		default:
			return Value{}, fmt.Errorf("number expects a number or a numeric string, got %s", args[0].describe())
		}
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
			return Value{}, fmt.Errorf("length expects 1 argument")
		}
		switch args[0].Kind {
		case KList:
			return VNum(float64(len(args[0].List))), nil
		case KTuple:
			return VNum(float64(len(args[0].Tup))), nil
		default:
			return Value{}, fmt.Errorf("length expects list or tuple")
		}
	}

	bi["head"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("head expects 1 argument")
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
			return Value{}, fmt.Errorf("tail expects 1 argument")
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
			return Value{}, fmt.Errorf("nth expects 2 arguments")
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
			err := checkContext(ctx)
			if err != nil {
				return Value{}, err
			}
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
			err := checkContext(ctx)
			if err != nil {
				return Value{}, err
			}
			val, callErr := ev.callFunc(ctx, fn.Fn, []Value{current, el})
			if callErr != nil {
				return Value{}, callErr
			}
			current = val
		}
		return current, nil
	}

	bi["filter"] = func(ctx context.Context, ev *evaluator, args []Value) (Value, error) {
		if len(args) != 2 {
			return Value{}, fmt.Errorf("filter expects function and list")
		}
		fn := args[0]
		if fn.Kind != KFunc {
			return Value{}, fmt.Errorf("filter expects function as first argument")
		}
		list, err := args[1].AsList()
		if err != nil {
			return Value{}, err
		}
		result := make([]Value, 0, len(list))
		for _, el := range list {
			err := checkContext(ctx)
			if err != nil {
				return Value{}, err
			}
			v, callErr := ev.callFunc(ctx, fn.Fn, []Value{el})
			if callErr != nil {
				return Value{}, callErr
			}
			keep, boolErr := v.AsBool()
			if boolErr != nil {
				return Value{}, fmt.Errorf("filter predicate must return a bool: %w", boolErr)
			}
			if keep {
				result = append(result, el)
			}
		}
		return VList(result), nil
	}

	bi["reverse"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		if len(args) != 1 {
			return Value{}, fmt.Errorf("reverse expects 1 argument")
		}
		list, err := args[0].AsList()
		if err != nil {
			return Value{}, err
		}
		result := make([]Value, len(list))
		for i, el := range list {
			result[len(list)-1-i] = el
		}
		return VList(result), nil
	}

	bi["range"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		// (range end) yields 0..end-1; (range start end) yields start..end-1.
		// Empty when the range is non-increasing, as in a for loop that never runs.
		if len(args) != 1 && len(args) != 2 {
			return Value{}, fmt.Errorf("range expects 1 or 2 arguments")
		}
		var start, end float64
		if len(args) == 1 {
			e, err := args[0].AsNumber()
			if err != nil {
				return Value{}, err
			}
			end = e
		} else {
			s, err := args[0].AsNumber()
			if err != nil {
				return Value{}, err
			}
			e, err := args[1].AsNumber()
			if err != nil {
				return Value{}, err
			}
			start, end = s, e
		}
		lo, hi := int(start), int(end)
		if hi <= lo {
			return VList([]Value{}), nil
		}
		result := make([]Value, 0, hi-lo)
		for i := lo; i < hi; i++ {
			err := checkContext(ctx)
			if err != nil {
				return Value{}, err
			}
			result = append(result, VNum(float64(i)))
		}
		return VList(result), nil
	}

	bi["error"] = func(ctx context.Context, _ *evaluator, args []Value) (Value, error) {
		// Lets a script raise a clear failure the host can present, e.g. in a
		// validation rule: (if (< age 0) (error "age must be non-negative") age).
		if len(args) != 1 {
			return Value{}, fmt.Errorf("error expects 1 argument (a message string)")
		}
		msg, err := args[0].AsString()
		if err != nil {
			return Value{}, fmt.Errorf("error expects a string message: %w", err)
		}
		return Value{}, errors.New(msg)
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
