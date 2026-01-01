package filo

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// RegisterStringBuiltins adds string manipulation functions to the engine.
// These builtins are pure, deterministic functions that do not access
// external resources.
func RegisterStringBuiltins(eng *Engine) {
	eng.MustRegisterBuiltin("str-join", builtinStrJoin)
	eng.MustRegisterBuiltin("str-split", builtinStrSplit)
	eng.MustRegisterBuiltin("str-find", builtinStrFind)
	eng.MustRegisterBuiltin("str-trim", builtinStrTrim)
	eng.MustRegisterBuiltin("str-replace", builtinStrReplace)
	eng.MustRegisterBuiltin("str-upper", builtinStrUpper)
	eng.MustRegisterBuiltin("str-lower", builtinStrLower)
	eng.MustRegisterBuiltin("str-concat", builtinStrConcat)
	eng.MustRegisterBuiltin("str-len", builtinStrLen)
	eng.MustRegisterBuiltin("str-sub", builtinStrSub)
	eng.MustRegisterBuiltin("str-fmt", builtinStrFmt)
}

// builtinStrFmt formats a string according to a format specifier.
// Usage: (str-fmt format args...) -> string
// Example: (str-fmt "Hello %s" "World") -> "Hello World"
// Supports %s, %d, %f, %v, %t.
func builtinStrFmt(ctx context.Context, args []Value) (Value, error) {
	if len(args) < 1 {
		return Value{}, fmt.Errorf("str-fmt expects at least 1 argument (format string)")
	}
	format, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-fmt: format must be string: %w", err)
	}

	fmtArgs := make([]interface{}, len(args)-1)
	for i, arg := range args[1:] {
		switch arg.Kind {
		case KNumber:
			fmtArgs[i] = arg.Num
		case KString:
			fmtArgs[i] = arg.Str
		case KBool:
			fmtArgs[i] = arg.Bool
		case KList:
			fmtArgs[i] = arg.List
		case KTuple:
			fmtArgs[i] = arg.Tup
		default:
			fmtArgs[i] = arg
		}
	}
	return VString(fmt.Sprintf(format, fmtArgs...)), nil
}

// builtinStrJoin joins a list of strings with a separator.
// Usage: (str-join separator list) -> string
// Example: (str-join ", " (list "a" "b" "c")) -> "a, b, c"
func builtinStrJoin(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 {
		return Value{}, fmt.Errorf("str-join expects 2 arguments (separator, list)")
	}
	sep, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-join: separator must be string: %w", err)
	}
	list, err := args[1].AsList()
	if err != nil {
		return Value{}, fmt.Errorf("str-join: second argument must be list: %w", err)
	}
	parts := make([]string, len(list))
	for i, v := range list {
		s, convErr := v.AsString()
		if convErr != nil {
			return Value{}, fmt.Errorf("str-join: list element %d must be string: %w", i, convErr)
		}
		parts[i] = s
	}
	return VString(strings.Join(parts, sep)), nil
}

// builtinStrSplit splits a string by a separator into a list.
// Usage: (str-split separator string) -> list
// Example: (str-split ", " "a, b, c") -> (list "a" "b" "c")
func builtinStrSplit(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 {
		return Value{}, fmt.Errorf("str-split expects 2 arguments (separator, string)")
	}
	sep, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-split: separator must be string: %w", err)
	}
	str, err := args[1].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-split: second argument must be string: %w", err)
	}
	parts := strings.Split(str, sep)
	result := make([]Value, len(parts))
	for i, p := range parts {
		result[i] = VString(p)
	}
	return VList(result), nil
}

// builtinStrFind checks if a substring exists in a string.
// Usage: (str-find substring string) -> bool
// Example: (str-find "world" "hello world") -> #t
func builtinStrFind(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 {
		return Value{}, fmt.Errorf("str-find expects 2 arguments (substring, string)")
	}
	substr, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-find: substring must be string: %w", err)
	}
	str, err := args[1].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-find: second argument must be string: %w", err)
	}
	return VBool(strings.Contains(str, substr)), nil
}

// builtinStrTrim removes leading and trailing whitespace from a string.
// Usage: (str-trim string) -> string
// Example: (str-trim "  hello  ") -> "hello"
func builtinStrTrim(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return Value{}, fmt.Errorf("str-trim expects 1 argument (string)")
	}
	str, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-trim: argument must be string: %w", err)
	}
	return VString(strings.TrimSpace(str)), nil
}

// builtinStrReplace replaces all occurrences of old with new in a string.
// Usage: (str-replace old new string) -> string
// Example: (str-replace "world" "Filo" "hello world") -> "hello Filo"
func builtinStrReplace(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 3 {
		return Value{}, fmt.Errorf("str-replace expects 3 arguments (old, new, string)")
	}
	old, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-replace: old must be string: %w", err)
	}
	newStr, err := args[1].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-replace: new must be string: %w", err)
	}
	str, err := args[2].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-replace: third argument must be string: %w", err)
	}
	return VString(strings.ReplaceAll(str, old, newStr)), nil
}

// builtinStrUpper converts a string to uppercase.
// Usage: (str-upper string) -> string
// Example: (str-upper "hello") -> "HELLO"
func builtinStrUpper(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return Value{}, fmt.Errorf("str-upper expects 1 argument (string)")
	}
	str, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-upper: argument must be string: %w", err)
	}
	return VString(strings.ToUpper(str)), nil
}

// builtinStrLower converts a string to lowercase.
// Usage: (str-lower string) -> string
// Example: (str-lower "HELLO") -> "hello"
func builtinStrLower(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return Value{}, fmt.Errorf("str-lower expects 1 argument (string)")
	}
	str, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-lower: argument must be string: %w", err)
	}
	return VString(strings.ToLower(str)), nil
}

// builtinStrConcat concatenates multiple strings.
// Usage: (str-concat strings...) -> string
// Example: (str-concat "hello" " " "world") -> "hello world"
func builtinStrConcat(ctx context.Context, args []Value) (Value, error) {
	var b strings.Builder
	for i, a := range args {
		s, err := a.AsString()
		if err != nil {
			return Value{}, fmt.Errorf("str-concat: argument %d must be string: %w", i, err)
		}
		b.WriteString(s)
	}
	return VString(b.String()), nil
}

// builtinStrLen returns the length of a string in runes.
// Usage: (str-len string) -> number
// Example: (str-len "hello") -> 5
func builtinStrLen(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 1 {
		return Value{}, fmt.Errorf("str-len expects 1 argument (string)")
	}
	str, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-len: argument must be string: %w", err)
	}
	return VNum(float64(utf8.RuneCountInString(str))), nil
}

// builtinStrSub extracts a substring from a string.
// Usage: (str-sub str start [end]) -> string
// Indices are 0-based, in runes. End is exclusive. If end is omitted, it slices to the end.
// Example: (str-sub "hello world" 0 5) -> "hello"
func builtinStrSub(ctx context.Context, args []Value) (Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return Value{}, fmt.Errorf("str-sub expects 2 or 3 arguments (string, start, [end])")
	}
	str, err := args[0].AsString()
	if err != nil {
		return Value{}, fmt.Errorf("str-sub: first argument must be string: %w", err)
	}
	startF, err := args[1].AsNumber()
	if err != nil {
		return Value{}, fmt.Errorf("str-sub: start must be number: %w", err)
	}
	if math.Trunc(startF) != startF {
		return Value{}, fmt.Errorf("str-sub: start must be an integer")
	}
	start := int(startF)
	end := -1
	if len(args) == 3 {
		endF, err := args[2].AsNumber()
		if err != nil {
			return Value{}, fmt.Errorf("str-sub: end must be number: %w", err)
		}
		if math.Trunc(endF) != endF {
			return Value{}, fmt.Errorf("str-sub: end must be an integer")
		}
		end = int(endF)
	}

	runes := []rune(str)
	if start < 0 {
		start = 0
	}
	if start > len(runes) {
		start = len(runes)
	}
	if end < 0 || end > len(runes) {
		end = len(runes)
	}
	if start > end {
		return VString(""), nil
	}
	return VString(string(runes[start:end])), nil
}
