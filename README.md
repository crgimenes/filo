# Filo

Filo is a small scripting language I built to be embedded in Go applications. It's a Lisp with minimal syntax, deterministic execution, and explicit limits on every dimension that could break a host server.

I made it for a problem that comes up every time I try to give end users scripting power: the obvious paths are either too weak to be useful or too powerful to be safe. Filo is the third option -- small enough that an ordinary user can write short rules (validations, expressions, field logic) and closed enough that nothing can go down because of it.

## Name and pronunciation

The name **Filo** comes from the Italian word *filo* ("thread").

It's pronounced like Italian **filo**:

- IPA: `/ˈfiː.lo/`
- Rough English approximation: **"FEE-lo"**

It is **not** "Filó", "FYE-lo" (`/ˈfaɪ.loʊ/`), or "fee-LOH" (`/fiːˈloʊ/`). Just **"FEE-lo"**.

## Why I built it

Three concrete cases that needed something like Filo:

- a RAD-style generic application builder where end users write logic into fields;
- an RPG platform with rules customizable per game and per character;
- regular applications configurable by administrators without redeploys.

In all three the requirement is the same: let the user write logic, and never let that logic put the server at risk.

## How it stays safe

The runtime ships with explicit constraints from day one:

- `StepLimit` -- bounds the number of evaluation steps.
- `RecursionLimit` -- bounds the call stack.
- `Timeout` -- execution gets cancelled.
- `context.Context` -- natural integration with Go cancellation.
- `recover()` around the executor -- no script can cause a `panic` on the host.

There is no file access, no network access, no syscall, no "dangerous" calls of any kind. The only things scripts can touch are the Go functions the host explicitly registers as builtins.

## Syntax

Lisp, minimal:

```lisp
(+ 1 2)
(if (< age 18) "minor" "adult")
(map (fn (x) (* x x)) (list 1 2 3))
```

I picked Lisp because it's the cheapest syntax to implement and the most predictable for someone who has never programmed. There's nowhere to hide logic in syntactic ornament.

## Go integration shape

- Builtins are written in Go.
- The global environment is a `map[string]Value`.
- Scripts run inside whatever sandbox the host configures.
- The host stays in control: only what you register is callable.

## Language reference

### Special forms

| Name | Syntax | Description |
|------|--------|-------------|
| `if` | `(if cond then [else])` | Conditional. Returns `list()` (empty/nil) if `else` is missing and `cond` is false. |
| `do` | `(do expr1 expr2 ...)` | Evaluates expressions in order, returns the last result. |
| `let` | `(let ((n v) ...) body)` | Local variables scoped to the body. |
| `letv` | `(letv (n1 n2) (values v1 v2) body)` | Destructures multi-value returns (tuples). |
| `fn` | `(fn (args) body)` | Anonymous function. |
| `def` | `(def name expr)` | Global variable or function (always in the root scope). |
| `set` | `(set name expr)` | Updates an existing variable in the nearest scope. |
| `values` | `(values v1 v2 ...)` | Returns multiple values (a tuple). |
| `exit` | `(exit [value])` | Terminates execution immediately. |
| `return` | `(return [value])` | Returns from the current function. |

### Core builtins

Note: `NewEngine()` includes the core math/logic/list/type builtins by default. Some sets are intentionally **opt-in** and must be registered explicitly.

| Category | Function | Description |
|----------|----------|-------------|
| **Math** | `+`, `-`, `*`, `/`, `%` | Basic arithmetic. |
| | `pow` | `(pow x y)` |
| **Logic** | `=`, `!=` | Equality. |
| | `<`, `<=`, `>`, `>=` | Numeric comparison. |
| | `and`, `or`, `not` | Boolean logic. |
| **Types** | `type-of` | Returns "number", "string", "list", etc. |
| | `is-empty` | True for `""` or empty list. |
| | `is-nil` | True for an empty list (closest thing to nil in the current runtime). |
| **Lists** | `list` | Creates a list `(list 1 2 3)`. |
| | `length` | List length. |
| | `head`, `tail` | First element / rest of list. |
| | `nth` | `(nth list index)` 0-based access. |
| | `list-append` | `(list-append list item)` Returns new list with item appended. |
| | `list-concat` | `(list-concat l1 l2 ...)` |
| | `map` | `(map fn list)` |
| | `fold` | `(fold fn init list)` |

### String builtins

Not enabled by default. To use them:

```go
eng := filo.NewEngine()
filo.RegisterStringBuiltins(eng)
```

| Function | Description |
|----------|-------------|
| `str-fmt` | `(str-fmt format args...)` Safe `fmt.Sprintf`. |
| `str-concat` | Concatenates arguments. |
| `str-join` | `(str-join sep list)` |
| `str-split` | `(str-split sep str)` |
| `str-len` | String length (runes). |
| `str-sub` | `(str-sub str start [end])` |
| `str-find` | `(str-find sub str)` |
| `str-replace` | `(str-replace old new str)` |
| `str-trim` | Trims whitespace. |
| `str-upper` | Uppercase. |
| `str-lower` | Lowercase. |

### Extension: filomath

Requires explicit registration: `filomath.RegisterMathBuiltins(eng)`.

| Function | Description |
|----------|-------------|
| `abs`, `sqrt` | Absolute value, square root. |
| `floor`, `ceil`, `round` | Rounding. |
| `to-int` | `(to-int n)` Truncates float to int. |
| `sin`, `cos`, `tan` | Trig (radians). |
| `log`, `log10`, `exp` | Logarithms / exponential. |
| `math-min`, `math-max` | Min / max of arguments. |
| `pi`, `e` | Constants. |

### Extension: filorand

Requires explicit registration: `filorand.RegisterRandomBuiltins(eng)`. These are intentionally **non-deterministic**.

| Function | Description |
|----------|-------------|
| `rand-float` | Random number `[0.0, 1.0)`. |
| `rand-int` | `(rand-int n)` Random integer `[0, n)`. |
| `uuid-v4` | Generates a UUID string. |

### Extension: filojson

JSON helpers for marshal/unmarshal: `json-marshal`, `json-unmarshal`, `json-null`.

## Pre-parse / execute (template style)

For scripts run many times against different data, Filo supports pre-parsing, similar to Go's `html/template`:

```go
// Parse once at startup
script := filo.Must(filo.ParseScript("calc", "(+ x y)"))

// Execute many times with different globals
for _, data := range items {
	globals := map[string]filo.Value{
		"x": filo.VNum(data.X),
		"y": filo.VNum(data.Y),
	}
	result, _, err := script.Execute(ctx, engine, globals, cfg)
	fmt.Println(result.Num)
}
```

API:

| Function | Description |
|----------|-------------|
| `ParseScript(name, src)` | Creates and parses a reusable script. |
| `script.Execute(ctx, eng, globals, cfg)` | Executes with explicit engine/config. |
| `Must(script, err)` | Panics if error (for init). |
| `Filo.Execute(script, overrides)` | Executes with Filo's globals + optional overrides. |

Pre-parsing eliminates parsing overhead. Roughly 2x faster for repeated executions.

## Marshal / Unmarshal

Convert Go values to Filo values and back:

```go
type Config struct {
	Name string `filo:"name"`
	Port int    `filo:"port"`
}

cfg := Config{Name: "app", Port: 8080}
val, err := filo.Marshal(cfg)
// val = (list (tuple "name" "app") (tuple "port" 8080))

var cfg2 Config
err = filo.Unmarshal(val, &cfg2)
// cfg2 = {Name: "app", Port: 8080}
```

Type mapping:

| Go Type | Filo Kind |
|---------|-----------|
| `bool` | `KBool` |
| `int`, `float64`, etc | `KNumber` |
| `string` | `KString` |
| `[]T` | `KList` |
| `struct` | `KList` of `(key, value)` tuples |
| `map[K]V` | `KList` of `(key, value)` tuples |
| `nil` | `KTuple` (empty) |

Expressions are evaluated **before** reaching `Unmarshal`:

- `(list "port" (+ 8000 80))` → `port = 8080`
- `(list "port" "(+ 8000 80)")` → `port = "(+ 8000 80)"` (literal string)

Fuzz tests:

```bash
# A single fuzz target for 30 seconds
go test -fuzz=FuzzMarshalUnmarshalString -fuzztime=30s .

# Available targets:
# - FuzzMarshalUnmarshalInt
# - FuzzMarshalUnmarshalFloat
# - FuzzMarshalUnmarshalString
# - FuzzMarshalUnmarshalBool
# - FuzzMarshalUnmarshalBytes
# - FuzzMarshalSliceInt
```

## Examples

### 1. Calculated field

```lisp
(let ((forca field:for) (bonus field:bonus))
  (+ (* forca 2) bonus))
```

### 2. Dynamic configuration

```lisp
(let ((env ENV))
  (set Address "http://localhost:3210")
  (if (= env "prod")
      (set Address "https://app.example.com")
      (set Address "http://localhost:3210")))
```

### 3. Controlled recursion

```lisp
(let ()
  (def fact (fn (n)
    (if (<= n 1)
        1
        (* n (fact (- n 1))))))
  (fact 5)) ; 120
```

### 4. Auto-level helpers

```lisp
(let ()
  (def thresholds (list 0 300 900 2700 6500 15000))

  (def auto-level (fn (xp thresholds)
    (fold (fn (lvl threshold)
            (if (>= xp threshold) (+ lvl 1) lvl))
         0
         thresholds)))

  (def auto-level-progress (fn (xp thresholds)
    (let ((lvl (auto-level xp thresholds))
          (total (length thresholds)))
      (if (>= lvl total)
          (values lvl 0)
          (let ((next (nth thresholds lvl)))
            (values lvl (- next xp))))))

  (auto-level-progress 1200 thresholds))
```

Returns `(tuple 3 1500)` -- character is at level three and needs 1,500 XP to reach the next tier.

## Integrating with Go

### Register a builtin

`add-two` sums two numbers passed positionally:

```go
func addTwo(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 2 {
		return filo.Value{}, errors.New("expected two numbers")
	}

	a, err := args[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}

	b, err := args[1].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}

	return filo.VNum(a + b), nil
}

func registerMathBuiltins(eng *filo.Engine) {
	eng.MustRegisterBuiltin("add-two", addTwo)
}

eng := filo.NewEngine()
registerMathBuiltins(eng)

res, _, err := eng.RunScript(ctx, "(add-two 10 32)", nil, cfg)
if err != nil {
	return err
}
// res = 42
```

### Register a string formatter

`full-name` takes two strings, trims, joins:

```go
func fullName(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 2 {
		return filo.Value{}, errors.New("expected first and last name")
	}

	first, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, err
	}

	last, err := args[1].AsString()
	if err != nil {
		return filo.Value{}, err
	}

	combined := strings.TrimSpace(first + " " + last)
	return filo.VString(combined), nil
}

func registerStringBuiltins(eng *filo.Engine) {
	eng.MustRegisterBuiltin("full-name", fullName)
}

eng := filo.NewEngine()
registerStringBuiltins(eng)

res, _, err := eng.RunScript(ctx, "(full-name \"Ada\" \"Lovelace\")", nil, cfg)
if err != nil {
	return err
}
// res = "Ada Lovelace"
```

### Register an aggregator

`min-max` takes a list of numbers and returns a tuple `(min max)`:

```go
func minMax(ctx context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, errors.New("expected one list")
	}

	list, err := args[0].AsList()
	if err != nil {
		return filo.Value{}, err
	}

	if len(list) == 0 {
		return filo.Value{}, errors.New("list cannot be empty")
	}

	minVal, err := list[0].AsNumber()
	if err != nil {
		return filo.Value{}, err
	}

	maxVal := minVal
	for i := 1; i < len(list); i++ {
		current, convErr := list[i].AsNumber()
		if convErr != nil {
			return filo.Value{}, convErr
		}

		if current < minVal {
			minVal = current
		}

		if current > maxVal {
			maxVal = current
		}
	}

	return filo.VList([]filo.Value{filo.VNum(minVal), filo.VNum(maxVal)}), nil
}

func registerAggregatorBuiltins(eng *filo.Engine) {
	eng.MustRegisterBuiltin("min-max", minMax)
}

eng := filo.NewEngine()
registerAggregatorBuiltins(eng)

res, _, err := eng.RunScript(ctx, "(min-max (list 4 7 1 9))", nil, cfg)
if err != nil {
	return err
}
// res = (list 1 9)
```

### Passing globals

```go
globals := map[string]filo.Value{
	"field:a": filo.VNum(10),
	"field:b": filo.VNum(5),
}

res, _, err := eng.RunScript(ctx, "(+ field:a field:b)", globals, cfg)
if err != nil {
	return err
}
// res = 15
```
