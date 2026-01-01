// Package filomath provides advanced math builtins for the Filo scripting language.
// These extend the basic arithmetic operations (+, -, *, /, %) with trigonometric,
// logarithmic, and other mathematical functions.
package filomath

import (
	"context"
	"fmt"
	"math"

	"github.com/crgimenes/filo"
)

// RegisterMathBuiltins adds advanced math builtins to a Filo engine.
// This can be called alongside other extension packages.
//
// Registered builtins:
//   - abs: Absolute value
//   - sqrt: Square root
//   - floor, ceil, round: Rounding functions
//   - sin, cos, tan: Trigonometric functions (radians)
//   - log, log10: Logarithms
//   - exp: Exponential (e^x)
//   - min, max: Multi-argument minimum/maximum
func RegisterMathBuiltins(eng *filo.Engine) {
	eng.MustRegisterBuiltin("abs", builtinAbs)
	eng.MustRegisterBuiltin("sqrt", builtinSqrt)
	eng.MustRegisterBuiltin("floor", builtinFloor)
	eng.MustRegisterBuiltin("ceil", builtinCeil)
	eng.MustRegisterBuiltin("round", builtinRound)
	eng.MustRegisterBuiltin("sin", builtinSin)
	eng.MustRegisterBuiltin("cos", builtinCos)
	eng.MustRegisterBuiltin("tan", builtinTan)
	eng.MustRegisterBuiltin("log", builtinLog)
	eng.MustRegisterBuiltin("log10", builtinLog10)
	eng.MustRegisterBuiltin("exp", builtinExp)
	eng.MustRegisterBuiltin("math-min", builtinMin) // prefixed to avoid conflicts with other packages
	eng.MustRegisterBuiltin("math-max", builtinMax) // prefixed to avoid conflicts with other packages
	eng.MustRegisterBuiltin("pi", builtinPi)
	eng.MustRegisterBuiltin("e", builtinE)
	eng.MustRegisterBuiltin("to-int", builtinToInt)
}

// builtinToInt converts a number to an integer by truncation (e.g., 3.9 -> 3.0).
func builtinToInt(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("to-int expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(float64(int64(n))), nil
}

// builtinAbs returns the absolute value of a number.
func builtinAbs(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("abs expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(math.Abs(n)), nil
}

// builtinSqrt returns the square root of a number.
func builtinSqrt(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("sqrt expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	if n < 0 {
		return filo.Value{}, fmt.Errorf("sqrt of negative number")
	}
	return filo.VNum(math.Sqrt(n)), nil
}

// builtinFloor returns the largest integer less than or equal to the argument.
func builtinFloor(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("floor expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(math.Floor(n)), nil
}

// builtinCeil returns the smallest integer greater than or equal to the argument.
func builtinCeil(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("ceil expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(math.Ceil(n)), nil
}

// builtinRound returns the nearest integer to the argument.
func builtinRound(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("round expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(math.Round(n)), nil
}

// builtinSin returns the sine of an angle in radians.
func builtinSin(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("sin expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(math.Sin(n)), nil
}

// builtinCos returns the cosine of an angle in radians.
func builtinCos(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("cos expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(math.Cos(n)), nil
}

// builtinTan returns the tangent of an angle in radians.
func builtinTan(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("tan expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(math.Tan(n)), nil
}

// builtinLog returns the natural logarithm of a number.
func builtinLog(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("log expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	if n <= 0 {
		return filo.Value{}, fmt.Errorf("log of non-positive number")
	}
	return filo.VNum(math.Log(n)), nil
}

// builtinLog10 returns the base-10 logarithm of a number.
func builtinLog10(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("log10 expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	if n <= 0 {
		return filo.Value{}, fmt.Errorf("log10 of non-positive number")
	}
	return filo.VNum(math.Log10(n)), nil
}

// builtinExp returns e raised to the power of the argument.
func builtinExp(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("exp expects 1 argument")
	}
	n, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	return filo.VNum(math.Exp(n)), nil
}

// builtinMin returns the minimum of multiple numbers.
func builtinMin(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) == 0 {
		return filo.Value{}, fmt.Errorf("math-min expects at least 1 argument")
	}
	result, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	for i := 1; i < len(args); i++ {
		n, err := args[i].AsNumber()
		if err != nil {
			return filo.Value{}, err
		}
		if n < result {
			result = n
		}
	}
	return filo.VNum(result), nil
}

// builtinMax returns the maximum of multiple numbers.
func builtinMax(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) == 0 {
		return filo.Value{}, fmt.Errorf("math-max expects at least 1 argument")
	}
	result, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}
	for i := 1; i < len(args); i++ {
		n, err := args[i].AsNumber()
		if err != nil {
			return filo.Value{}, err
		}
		if n > result {
			result = n
		}
	}
	return filo.VNum(result), nil
}

// builtinPi returns the mathematical constant pi.
func builtinPi(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 0 {
		return filo.Value{}, fmt.Errorf("pi expects no arguments")
	}
	return filo.VNum(math.Pi), nil
}

// builtinE returns Euler's number e.
func builtinE(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 0 {
		return filo.Value{}, fmt.Errorf("e expects no arguments")
	}
	return filo.VNum(math.E), nil
}
