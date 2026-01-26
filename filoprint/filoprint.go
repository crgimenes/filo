// Package filoprint provides simple print builtins for Filo scripts.
// These builtins echo output directly to stdout, useful for REPL debugging.
//
// Functions:
//   - print: Print all arguments
//   - printf: Print with format string (supports %T for Filo types)
package filoprint

import (
	"context"
	"fmt"
	"strings"

	"github.com/crgimenes/filo"
)

// RegisterPrintBuiltins adds print functions to the engine.
func RegisterPrintBuiltins(eng *filo.Engine) {
	eng.MustRegisterBuiltin("print", builtinPrint)
	eng.MustRegisterBuiltin("printf", builtinPrintf)
}

// builtinPrint prints all arguments to stdout.
// Usage: (print "Hello" name)
func builtinPrint(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) < 1 {
		return filo.Value{}, fmt.Errorf("print expects at least 1 argument")
	}

	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = valueToString(arg)
	}
	fmt.Println(strings.Join(parts, " "))

	return filo.VList(nil), nil
}

// builtinPrintf prints with format string.
// Supports %T to show the Filo type of a value.
// Usage: (printf "User %s has %d points" name points)
// Usage: (printf "x is %T with value %v" x x)
func builtinPrintf(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) < 1 {
		return filo.Value{}, fmt.Errorf("printf expects at least 1 argument (format string)")
	}

	format, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("printf: first argument must be string: %w", err)
	}

	result := formatWithFiloTypes(format, args[1:])
	fmt.Println(result)

	return filo.VBool(true), nil
}

// formatWithFiloTypes processes a format string, handling %T for Filo types.
func formatWithFiloTypes(format string, args []filo.Value) string {
	var result strings.Builder
	argIndex := 0

	for i := 0; i < len(format); i++ {
		if format[i] != '%' {
			result.WriteByte(format[i])
			continue
		}

		// Check for %%
		if i+1 < len(format) && format[i+1] == '%' {
			result.WriteString("%%")
			i++
			continue
		}

		// Look for format specifier
		if i+1 < len(format) {
			spec := format[i+1]
			if spec == 'T' {
				// Custom %T: show Filo type
				tValue := "%!T(MISSING)"
				if argIndex < len(args) {
					tValue = (filoTypeName(args[argIndex]))
					argIndex++
				}
				result.WriteString(tValue)
				i++
				continue
			}

			// Standard format specifier - find end of specifier
			j := i + 1
			for j < len(format) && !isFormatVerb(format[j]) {
				j++
			}
			if j < len(format) {
				specFull := format[i : j+1]
				if argIndex < len(args) {
					result.WriteString(fmt.Sprintf(specFull, valueToGo(args[argIndex])))
					argIndex++
				} else {
					result.WriteString(specFull)
					result.WriteString("(MISSING)")
				}
				i = j
				continue
			}
		}

		result.WriteByte(format[i])
	}

	return result.String()
}

// valueToString converts a Filo value to a readable string.
func valueToString(v filo.Value) string {
	switch v.Kind {
	case filo.KBool:
		if v.Bool {
			return "#t"
		}
		return "#f"
	case filo.KNumber:
		if v.Num == float64(int64(v.Num)) {
			return fmt.Sprintf("%d", int64(v.Num))
		}
		return fmt.Sprintf("%g", v.Num)
	case filo.KString:
		return v.Str
	default:
		return v.String()
	}
}

// valueToGo converts a Filo value to a Go value for formatting.
func valueToGo(v filo.Value) any {
	switch v.Kind {
	case filo.KBool:
		return v.Bool
	case filo.KNumber:
		if v.Num == float64(int64(v.Num)) {
			return int64(v.Num)
		}
		return v.Num
	case filo.KString:
		return v.Str
	default:
		return v.String()
	}
}

// filoTypeName returns the Filo type name for a value.
func filoTypeName(v filo.Value) string {
	switch v.Kind {
	case filo.KBool:
		return "bool"
	case filo.KNumber:
		return "number"
	case filo.KString:
		return "string"
	case filo.KList:
		return "list"
	case filo.KTuple:
		return "tuple"
	case filo.KFunc:
		return "function"
	default:
		return "unknown"
	}
}

// isFormatVerb returns true if c is a valid format verb character.
func isFormatVerb(c byte) bool {
	switch c {
	case 'v', 's', 'd', 'b', 'o', 'O', 'x', 'X', 'c', 'q', 'U',
		'e', 'E', 'f', 'F', 'g', 'G', 'p', 't':
		return true
	default:
		return false
	}
}
