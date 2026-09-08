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

Literals and lexical rules:

- Booleans are `#t` and `#f` (exactly those two characters; `#true` is a parse error).
- Strings are double-quoted UTF-8 text with escapes `\n \t \r \0 \a \b \f \v \\ \"`. A leading UTF-8 BOM is stripped.
- `;` starts a line comment, running to end of line.
- Numbers are all `float64` (integers are exact up to 2^53). A literal is `[+-]?digits[.digits][e[+-]digits]` or `.digits`; any other atom is a symbol, so `inf`, `nan`, `1_000` and `0x10` are undefined symbols, not numbers. `(number s)` accepts whatever the host's parser accepts.
- `(tuple a b ...)` builds a fixed multi-value tuple; `(list ...)` builds a list.
- A source file may hold several top-level expressions; they are evaluated in order and the last value is the result (as if wrapped in an implicit `(let () ...)`).

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
| `cond` | `(cond (test body...) ... (else body...))` | Multi-way branch: the first clause whose `test` is `#t` runs its body (an implicit `do`). An optional final `else` clause always matches. Each test must be a bool; no match and no `else` yields the empty list. |
| `do` | `(do expr1 expr2 ...)` | Evaluates expressions in order, returns the last result. |
| `let` | `(let ((n v) ...) body)` | Local variables scoped to the body. |
| `letv` | `(letv (n1 n2) (values v1 v2) body)` | Destructures multi-value returns (tuples). |
| `fn` | `(fn (args) body)` | Anonymous function. |
| `def` | `(def name expr)` | Global variable or function (always in the root scope). |
| `set` | `(set name expr)` | Assigns a variable in the nearest scope that binds it; if the name is unbound, it creates a global. |
| `and`, `or` | `(and a b ...)` | Boolean logic with **short-circuit**: arguments evaluate left to right and stop at the first `#f` (`and`) or `#t` (`or`); later arguments never run, so `(or found (expensive-check x))` is a valid guard. Each evaluated argument must be a bool. |
| `values` | `(values v1 v2 ...)` | Returns multiple values (a tuple). |
| `exit` | `(exit [value])` | Terminates execution immediately. |
| `return` | `(return [value])` | Returns from the current function. |

### Core builtins

Note: `NewEngine()` includes the core math/logic/list/type builtins by default. Some sets are intentionally **opt-in** and must be registered explicitly.

| Category | Function | Description |
|----------|----------|-------------|
| **Math** | `+`, `-`, `*`, `/`, `%` | Basic arithmetic. `(- x)` negates and `(/ x)` is the reciprocal. `%` is floored modulo as in Lua — the result takes the divisor's sign, so `(% -1 2)` is `1`. |
| | `pow` | `(pow x y)`; a negative base with a fractional exponent is NaN, as in IEEE. |
| **Logic** | `=`, `!=` | Equality. Comparing values of different kinds is an error — cast first: `(= (string 1) "1")`. |
| | `<`, `<=`, `>`, `>=` | Numeric comparison. |
| | `not` | Boolean negation (`and`/`or` are special forms, see above). |
| **Casts** | `string` | `(string x)` renders any value as text (`42` → `"42"`, `#t` → `"#t"`; strings pass through unchanged). |
| | `number` | `(number s)` parses a numeric string (`"1.5"` → `1.5`); a non-numeric string is an error. |
| **Types** | `type-of` | Returns "number", "string", "list", etc. |
| | `is-empty` | True for `""` or empty list. |
| | `is-nil` | True for an empty list (closest thing to nil in the current runtime). |
| **Lists** | `list` | Creates a list `(list 1 2 3)`. |
| | `length` | List length. |
| | `head`, `tail` | First element / rest of list. |
| | `nth` | `(nth list index)` 0-based access; a fractional index is an error. |
| | `list-append` | `(list-append list item)` Returns new list with item appended. |
| | `list-concat` | `(list-concat l1 l2 ...)` |
| | `map` | `(map fn list)` |
| | `filter` | `(filter fn list)` Keeps elements for which `fn` returns `#t`. |
| | `fold` | `(fold fn init list)` |
| | `range` | `(range end)` → `0..end-1`; `(range start end)` → `start..end-1` (empty when non-increasing). Bounds must be integers, and the list is capped at 2^20 elements. |
| | `reverse` | `(reverse list)` |
| **Errors** | `error` | `(error "message")` Raises a script error with the message — for validation rules that must fail clearly. |

### String builtins

Not enabled by default. To use them:

```go
import "github.com/crgimenes/filo/filostrings"

eng := filo.NewEngine()
filostrings.RegisterBuiltins(eng)
```

| Function | Description |
|----------|-------------|
| `str-fmt` | `(str-fmt format args...)` Filo's own verbs: `%s` any value as text, `%v` any value as a Filo literal, `%d` an integral number, `%f` a number, `%%`; flags `-`, `0`, `+`, a width and `.precision` before the verb. An unknown verb, a fractional `%d`, or a mismatched argument count is an error. |
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

Requires explicit registration (import `github.com/crgimenes/filo/filomath`): `filomath.RegisterBuiltins(eng)`.

| Function | Description |
|----------|-------------|
| `abs`, `sqrt` | Absolute value, square root. |
| `floor`, `ceil`, `round` | Rounding. |
| `to-int` | `(to-int n)` Truncates float to int. |
| `sin`, `cos`, `tan` | Trig (radians). |
| `log`, `log10`, `exp` | Logarithms / exponential. |
| `math-min`, `math-max` | Min / max of arguments. |
| `pi`, `e` | Zero-arg builtins: call them as `(pi)`, `(e)`. |

### Extension: filorand

Requires explicit registration (import `github.com/crgimenes/filo/filorand`): `filorand.RegisterBuiltins(eng)`. These are intentionally **non-deterministic**.

| Function | Description |
|----------|-------------|
| `rand-float` | Random number `[0.0, 1.0)`. |
| `rand-int` | `(rand-int n)` Random integer `[0, n)`. |
| `uuid-v4` | Generates a UUID string. |

### Extension: filojson

Requires explicit registration (import `github.com/crgimenes/filo/filojson`): `filojson.RegisterBuiltins(eng)`. JSON helpers for marshal/unmarshal: `json-marshal`, `json-unmarshal`, `json-null`.

### Extension: filoprint

Requires explicit registration (import `github.com/crgimenes/filo/filoprint`): `filoprint.RegisterBuiltins(eng)`. Adds `print`, `println`, and `printf` (the latter understands `%T` for Filo types). Output goes to stdout by default; redirect it with `filoprint.SetOutput(w)`.

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

Pre-parsing eliminates parsing overhead, roughly 1.5x faster for repeated executions (see the `BenchmarkRunScript` vs `BenchmarkPreParsed` benchmarks).

Compilation also folds constant subexpressions (`(* 2 60)` becomes `120` at
compile time). Folding is semantics-preserving — a call is only replaced when
evaluating it with constant arguments succeeds — so there is no switch to turn
it off, and `TestFoldingMatchesInterpreter` holds the two paths equal.

## Marshal / Unmarshal

Convert Go values to Filo values and back:

```go
type Config struct {
	Name string `filo:"name"`
	Port int    `filo:"port"`
}

cfg := Config{Name: "app", Port: 8080}
src, err := filo.Marshal(cfg)
// src is Filo source text: (list (tuple "name" "app") (tuple "port" 8080))

var cfg2 Config
err = filo.Unmarshal(src, &cfg2)
// cfg2 = {Name: "app", Port: 8080}
```

`Marshal` returns a **string** of Filo source and `Unmarshal` parses one back.
To work with a `Value` directly instead of text, use `MarshalToValue` and
`UnmarshalFromValue`. `Marshal` rejects a string field that is not valid UTF-8
(Filo strings are text) rather than corrupting it. The Kind column below is the
`Value` kind each Go type maps to:

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

## Command-line tools

Three small binaries live under `cmd/`; `make tools` builds all of them into
`./bin` (and `make install` puts them in `GOPATH/bin`):

- **`filofmt`** -- a formatter for `.filo` files, in the `gofmt` mold. Reads
  stdin or file/dir paths; `-w` rewrites in place, `-l` lists files that would
  change, `-d` shows a diff, `-indent N` sets the indent width, and
  `-fold-const` additionally folds constant expressions. Install with
  `go install github.com/crgimenes/filo/cmd/filofmt@latest`.
- **`filofix`** -- modernizes and reduces source, in the `go fix` mold, while
  preserving formatting and comments. It removes the legacy root `(let () ...)`
  wrapper (the interpreter has handled multiple top-level forms for a long
  time), folds comment-free constant subexpressions to their value
  (`(* 8 1000)` → `8000`), and simplifies redundant boolean forms —
  `(if C #t #f)` → `C`, `(if C #f #t)` → `(not C)`, `(not (not C))` → `C` —
  when `C` is provably a bool, so the shorter form keeps the same value and
  errors. Flags `-w`, `-l`, `-d` as filofmt, plus `-fmt` to run the formatter
  on the result. Install with
  `go install github.com/crgimenes/filo/cmd/filofix@latest`.
- **`filo-repl`** -- an interactive REPL (line editing, history, multi-line
  input) that falls back to batch mode when stdin is a pipe. Load extension
  packages with `-filo-package math,rand,str,print,json`, and bound the run with
  `-step-limit`, `-recursion-limit`, and `-timeout`. Install with
  `go install github.com/crgimenes/filo/cmd/filo-repl@latest`.

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
(def fact (fn (n)
  (if (<= n 1)
      1
      (* n (fact (- n 1))))))
(fact 5) ; 120
```

### 4. Auto-level helpers

```lisp
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
          (values lvl (- next xp)))))))

(auto-level-progress 1200 thresholds)
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

`min-max` takes a list of numbers and returns a two-element list `(min max)`:

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

---

## C runtime

`c/` holds the same language as a C library: `filo.h` and `filo.c`, no libc
beyond `memcpy`/`memcmp`/`strlen`, and no allocation after init — the host
hands over two memory blocks (a persistent arena for programs and globals, a
run arena reset on every run) and a script that exhausts them fails with an
error instead of corrupting anything. It compiles for a freestanding wasm32
target and for microcontrollers; `filo_libc.c` adds number formatting and
parsing for hosts that have a libc. The `math` and `strings` packs exist as
one opt-in file each (`filo_math.c`, `filo_strings.c`), freestanding too:
the transcendental functions and `%f` formatting come from the host through
a small table (`filo_libc.c` fills it from libm), and everything else is
self-contained. Case mapping covers ASCII and the Latin-1 letters. For a
target with no C library at all, `filo_nolibc.c` supplies the number text the
lexer and `(string n)` need; the whole corpus runs against it as well as
against the libc host, and the only cases it skips are the ones asking for a
power with a fractional exponent or the transcendental functions, which need
libm to exist. The QA gate also fuzzes the runtime with libFuzzer for a few
seconds (`make -C c fuzz`, seeded from the corpus), since any byte string is
a script and none may fault.

A C host may close the set of globals with `filo_seal_globals`, after which a
script naming a global the host never created fails to compile instead of
creating one silently. It is off unless asked for, and has no counterpart in
the Go engine, which no host has needed it for.

Both runtimes lower source to the instruction set in `docs/ir.md` and are
held to the same behavior by `testdata/corpus`: plain-text cases pinning what
a script evaluates to, run by `corpus_test.go` here and by `c/corpus_runner.c`
there (`make -C c qa` runs it under ASan/UBSan together with clang-tidy and
cppcheck). Error messages are a Go-side promise; the corpus asserts outcomes.

## More of my projects

- [kutta](https://github.com/crgimenes/kutta): a 2D wind tunnel; watch air misbehave around an airfoil.
- [glaze](https://github.com/crgimenes/glaze): WebView desktop apps in Go, cgo-free.
- [compterm](https://github.com/crgimenes/compterm): share your terminal over the network.
- [native](https://github.com/crgimenes/native): cgo-free Go bindings for OS APIs: clipboard, mmap, keep-awake, and friends.

More at [github.com/crgimenes](https://github.com/crgimenes) and [crg.eti.br](https://crg.eti.br).
