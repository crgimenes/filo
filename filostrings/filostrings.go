package filostrings

import (
	"context"
	"fmt"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/crgimenes/filo"
)

// RegisterBuiltins adds string manipulation functions to the engine.
// These builtins are pure, deterministic functions that do not access
// external resources.
func RegisterBuiltins(eng *filo.Engine) {
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

// builtinStrFmt formats with Filo's own verbs so that a wrong verb is an
// error instead of Go's "%!d(float64=1)" leaking into the output:
// (str-fmt "%s=%d" "n" 3) -> "n=3". Verbs: %s any value as text, %v any value
// as a Filo literal, %d an integral number, %f a number, %% a percent sign.
// Flags -, 0, +, a width and a .precision are accepted before the verb.
func builtinStrFmt(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) < 1 {
		return filo.Value{}, fmt.Errorf("str-fmt expects at least 1 argument (format string)")
	}
	format, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-fmt: format must be string: %w", err)
	}
	rest := args[1:]
	var b strings.Builder
	for i := 0; i < len(format); i++ {
		c := format[i]
		if c != '%' {
			b.WriteByte(c)
			continue
		}
		i++
		specStart := i
		for i < len(format) && strings.IndexByte("-+0123456789.", format[i]) >= 0 {
			i++
		}
		if i >= len(format) {
			return filo.Value{}, fmt.Errorf("str-fmt: incomplete verb at end of format")
		}
		spec := format[specStart:i]
		verb := format[i]
		if verb == '%' && spec == "" {
			b.WriteByte('%')
			continue
		}
		if len(rest) == 0 {
			return filo.Value{}, fmt.Errorf("str-fmt: missing argument for %%%s%c", spec, verb)
		}
		piece, err := fmtVerb(spec, verb, rest[0])
		if err != nil {
			return filo.Value{}, fmt.Errorf("str-fmt: %w", err)
		}
		rest = rest[1:]
		b.WriteString(piece)
	}
	if len(rest) > 0 {
		return filo.Value{}, fmt.Errorf("str-fmt: %d extra arguments", len(rest))
	}
	return filo.VString(b.String()), nil
}

func fmtVerb(spec string, verb byte, arg filo.Value) (string, error) {
	if !specShape(spec) {
		return "", fmt.Errorf("bad verb %%%s%c", spec, verb)
	}
	switch verb {
	case 's':
		text := arg.String()
		if arg.Kind == filo.KString {
			text = arg.Str
		}
		return fmt.Sprintf("%"+spec+"s", text), nil
	case 'v':
		return fmt.Sprintf("%"+spec+"s", arg.String()), nil
	case 'd':
		n, err := arg.AsNumber()
		if err != nil {
			return "", err
		}
		if n != math.Trunc(n) || math.IsInf(n, 0) {
			return "", fmt.Errorf("%%d expects an integer, got %s", arg)
		}
		return fmt.Sprintf("%"+spec+"d", int64(n)), nil
	case 'f':
		n, err := arg.AsNumber()
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%"+spec+"f", n), nil
	}
	return "", fmt.Errorf("unknown verb %%%s%c", spec, verb)
}

// specShape is [-+0]*digits*(.digits*)?: what fmt renders without a BADWIDTH
// or BADPREC marker.
func specShape(spec string) bool {
	i := 0
	for i < len(spec) && strings.IndexByte("-+0", spec[i]) >= 0 {
		i++
	}
	for i < len(spec) && spec[i] >= '0' && spec[i] <= '9' {
		i++
	}
	if i < len(spec) && spec[i] == '.' {
		i++
		for i < len(spec) && spec[i] >= '0' && spec[i] <= '9' {
			i++
		}
	}
	return i == len(spec)
}

// builtinStrJoin joins a list of strings with a separator.
// Usage: (str-join separator list) -> string
// Example: (str-join ", " (list "a" "b" "c")) -> "a, b, c"
func builtinStrJoin(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 2 {
		return filo.Value{}, fmt.Errorf("str-join expects 2 arguments (separator, list)")
	}
	sep, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-join: separator must be string: %w", err)
	}
	list, err := args[1].AsList()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-join: second argument must be list: %w", err)
	}
	parts := make([]string, len(list))
	for i, v := range list {
		s, convErr := v.AsString()
		if convErr != nil {
			return filo.Value{}, fmt.Errorf("str-join: list element %d must be string: %w", i, convErr)
		}
		parts[i] = s
	}
	return filo.VString(strings.Join(parts, sep)), nil
}

// builtinStrSplit splits a string by a separator into a list.
// Usage: (str-split separator string) -> list
// Example: (str-split ", " "a, b, c") -> (list "a" "b" "c")
func builtinStrSplit(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 2 {
		return filo.Value{}, fmt.Errorf("str-split expects 2 arguments (separator, string)")
	}
	sep, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-split: separator must be string: %w", err)
	}
	str, err := args[1].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-split: second argument must be string: %w", err)
	}
	parts := strings.Split(str, sep)
	result := make([]filo.Value, len(parts))
	for i, p := range parts {
		result[i] = filo.VString(p)
	}
	return filo.VList(result), nil
}

// builtinStrFind checks if a substring exists in a string.
// Usage: (str-find substring string) -> bool
// Example: (str-find "world" "hello world") -> #t
func builtinStrFind(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 2 {
		return filo.Value{}, fmt.Errorf("str-find expects 2 arguments (substring, string)")
	}
	substr, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-find: substring must be string: %w", err)
	}
	str, err := args[1].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-find: second argument must be string: %w", err)
	}
	return filo.VBool(strings.Contains(str, substr)), nil
}

// builtinStrTrim removes leading and trailing whitespace from a string.
// Usage: (str-trim string) -> string
// Example: (str-trim "  hello  ") -> "hello"
func builtinStrTrim(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("str-trim expects 1 argument (string)")
	}
	str, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-trim: argument must be string: %w", err)
	}
	return filo.VString(strings.TrimSpace(str)), nil
}

// builtinStrReplace replaces all occurrences of old with new in a string.
// Usage: (str-replace old new string) -> string
// Example: (str-replace "world" "Filo" "hello world") -> "hello Filo"
func builtinStrReplace(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 3 {
		return filo.Value{}, fmt.Errorf("str-replace expects 3 arguments (old, new, string)")
	}
	old, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-replace: old must be string: %w", err)
	}
	newStr, err := args[1].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-replace: new must be string: %w", err)
	}
	str, err := args[2].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-replace: third argument must be string: %w", err)
	}
	return filo.VString(strings.ReplaceAll(str, old, newStr)), nil
}

// builtinStrUpper converts a string to uppercase.
// Usage: (str-upper string) -> string
// Example: (str-upper "hello") -> "HELLO"
func builtinStrUpper(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("str-upper expects 1 argument (string)")
	}
	str, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-upper: argument must be string: %w", err)
	}
	return filo.VString(strings.ToUpper(str)), nil
}

// builtinStrLower converts a string to lowercase.
// Usage: (str-lower string) -> string
// Example: (str-lower "HELLO") -> "hello"
func builtinStrLower(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("str-lower expects 1 argument (string)")
	}
	str, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-lower: argument must be string: %w", err)
	}
	return filo.VString(strings.ToLower(str)), nil
}

// builtinStrConcat concatenates multiple strings.
// Usage: (str-concat strings...) -> string
// Example: (str-concat "hello" " " "world") -> "hello world"
func builtinStrConcat(ctx context.Context, args []filo.Value) (filo.Value, error) {
	var b strings.Builder
	for i, a := range args {
		s, err := a.AsString()
		if err != nil {
			return filo.Value{}, fmt.Errorf("str-concat: argument %d must be string: %w", i, err)
		}
		b.WriteString(s)
	}
	return filo.VString(b.String()), nil
}

// builtinStrLen returns the length of a string in runes.
// Usage: (str-len string) -> number
// Example: (str-len "hello") -> 5
func builtinStrLen(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("str-len expects 1 argument (string)")
	}
	str, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-len: argument must be string: %w", err)
	}
	return filo.VNum(float64(utf8.RuneCountInString(str))), nil
}

// builtinStrSub extracts a substring from a string.
// Usage: (str-sub str start [end]) -> string
// Indices are 0-based, in runes. End is exclusive. If end is omitted, it slices to the end.
// Example: (str-sub "hello world" 0 5) -> "hello"
func builtinStrSub(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return filo.Value{}, fmt.Errorf("str-sub expects 2 or 3 arguments (string, start, [end])")
	}
	str, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-sub: first argument must be string: %w", err)
	}
	startF, err := args[1].AsNumber()
	if err != nil {
		return filo.Value{}, fmt.Errorf("str-sub: start must be number: %w", err)
	}
	if math.Trunc(startF) != startF {
		return filo.Value{}, fmt.Errorf("str-sub: start must be an integer")
	}
	start := int(startF)
	end := -1
	if len(args) == 3 {
		endF, err := args[2].AsNumber()
		if err != nil {
			return filo.Value{}, fmt.Errorf("str-sub: end must be number: %w", err)
		}
		if math.Trunc(endF) != endF {
			return filo.Value{}, fmt.Errorf("str-sub: end must be an integer")
		}
		end = int(endF)
	}

	runes := []rune(str)
	start = min(max(start, 0), len(runes))
	if end < 0 {
		end = len(runes)
	}
	end = min(end, len(runes))
	if start > end {
		return filo.VString(""), nil
	}
	return filo.VString(string(runes[start:end])), nil
}
