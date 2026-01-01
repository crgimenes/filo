# filomath Package

This package provides advanced mathematical builtins for the Filo scripting language.

## Installation

```go
import "github.com/crgimenes/filo/filomath"
```

## Usage

```go
engine := filo.NewEngine()
filomath.RegisterMathBuiltins(engine)

result, _, _ := engine.RunScript(ctx, "(sqrt 16)", nil, cfg)
// result = 4
```

## Available Builtins

### Constants

| Builtin | Description | Example |
|---------|-------------|---------|
| `pi` | Mathematical constant pi | `(pi)` -> 3.14159... |
| `e` | Euler's number | `(e)` -> 2.71828... |

### Absolute Value and Rounding

| Builtin | Description | Example |
|---------|-------------|---------|
| `abs` | Absolute value | `(abs -5)` -> 5 |
| `floor` | Round down | `(floor 3.7)` -> 3 |
| `ceil` | Round up | `(ceil 3.2)` -> 4 |
| `round` | Round to nearest | `(round 3.5)` -> 4 |
| `sqrt` | Square root | `(sqrt 16)` -> 4 |

### Trigonometric (radians)

| Builtin | Description | Example |
|---------|-------------|---------|
| `sin` | Sine | `(sin 0)` -> 0 |
| `cos` | Cosine | `(cos 0)` -> 1 |
| `tan` | Tangent | `(tan 0)` -> 0 |

### Logarithmic and Exponential

| Builtin | Description | Example |
|---------|-------------|---------|
| `log` | Natural log (ln) | `(log (e))` -> 1 |
| `log10` | Base-10 log | `(log10 100)` -> 2 |
| `exp` | e^x | `(exp 1)` -> 2.71828... |

### Min/Max

| Builtin | Description | Example |
|---------|-------------|---------|
| `math-min` | Minimum of N values | `(math-min 3 1 4)` -> 1 |
| `math-max` | Maximum of N values | `(math-max 3 1 4)` -> 4 |

## Using Multiple Packages

Multiple Filo extension packages can be loaded on the same engine:

```go
engine := filo.NewEngine()

// Load extension packages
filomath.RegisterMathBuiltins(engine)

// All builtins are now available
result, _, _ := engine.RunScript(ctx, `
    (let ((value 16)
          (rounded (round value)))
      (sqrt rounded))
`, globals, cfg)
```

## Testing

```bash
go test -v ./filomath/...
```
