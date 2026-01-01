# Filo — A Safe, Minimalist Scripting Language for Go Applications

Filo is a **lean**, **secure**, and **deterministic** scripting language designed to be embedded directly into Go applications. It was built for real-world scenarios where end users — including non-programmers — must write small rules, expressions, and validations that influence application behavior **without compromising stability, security, or performance**.

This README explains **why** the language exists, **which problems it solves**, how it works, and presents **practical examples**.

---

## Name and pronunciation

The name **Filo** comes from the Italian word *filo* (“thread”).

It is pronounced like Italian **filo**:

- IPA: `/ˈfiː.lo/`
- Rough English approximation: **“FEE-lo”**

Examples in Italian:

- *Un filo di lana.* – “A thread of wool.”
- *Tirare un filo dal maglione.* – “To pull a thread from the sweater.”
- *C’è un filo che pende dalla manica.* – “There is a thread hanging from the sleeve.”

Please note: it is **not** pronounced like “Filó” or “FYE-lo” (`/ˈfaɪ.loʊ/`) or “fee-LOH” (`/fiːˈloʊ/`), but simply **“FEE-lo”**.

---

## Motivation

When building complex systems such as:

- a RAD-style generic application builder,
- an RPG platform with customizable rules,
- applications configurable by users or administrators,

a clear need appears:

> **Allow the user to write custom logic without ever putting the server at risk.**

Using Lua was considered, but it has issues:

### Problems when using Lua/gopher-lua

- No granular execution control (`SetHook` does not exist in the library).
- It is difficult to guarantee:
  - there are no *infinite loops*,
  - there is no *excessive CPU consumption*,
  - there is no *sandbox escape* (filesystem, network, fragile globals).
- Lua syntax is not ideal for short expressions used as calculated fields.
- Non-technical users struggle more with procedural syntax.

Therefore, something **simpler, controlled, and safe** was necessary.

---

## Why create the Filo language?

### 1. **Absolute security**

The Filo runtime is designed with:

- `StepLimit` — prevents infinite loops.
- `RecursionLimit` — blocks stack explosions.
- `Timeout` — execution is automatically aborted.
- `context.Context` — natural integration with Go.
- `recover()` — no script can cause a server `panic`.

### 2. **Determinism**

Scripts must always produce the same results, with no unexpected side effects.

### 3. **Simplicity**

Filo uses a minimalist Lisp-like syntax:

```lisp
(+ 1 2)
(if (< idade 18) "minor" "adult")
(map (fn (x) (* x x)) (list 1 2 3))
```

Small, easy to teach, easy to understand, and extremely predictable.

### 4. **Smooth Go integration**

- Builtins written directly in Go.
- Global environment passed as `map[string]Value`.
- Safe calls made in the backend.
- Ideal for validations, RPG rules, and configuration scripts.

### 5. **Extensible**

- Go functions can be registered as Filo commands — from simple sums to database queries.

---

## Design and Philosophy

Filo's philosophy is summarized in four principles:

### **1. Short, declarative scripts**

Users should write:

- mathematical expressions,
- validations,
- small business rules.

No modules, long loops, or complex structures.

### **2. Small, powerful, testable language**

Step by step, Filo provides:

- essential operations (`+`, `-`, `*`, `/`, `%`, `pow`);
- comparisons (`=`, `!=`, `<`, `<=`, `>`, `>=`);
- boolean logic (`and`, `or`, `not`);
- lists and higher-order functions (`map`, `fold`, `list`, `length`, `head`, `tail`, `nth`, `append`, `concat`);
- string operations (`str-fmt`, `str-concat`, `str-join`, `str-split`, `str-find`, `str-trim`, `str-replace`, `str-upper`, `str-lower`, `str-len`, `str-sub`);
- basic control flow (`if`, `do`);
- introspection (`type-of`);
- validation (`is-empty`, `is-nil`);
- local scope (`let`, `letv`);
- multiple returns (`values`);
- functions (`fn`, `def`) with recursion limits.

### **Extension Packages**

Filo can be extended with specialized packages:

- **filomath**: Advanced math functions (`sin`, `cos`, `log`, `to-int`, etc).
- **filorand**: Non-deterministic functions (`rand-float`, `rand-int`, `uuid-v4`).

### **3. Restricted environment**

No:

- file access,
- network access,
- “dangerous” calls.

All advanced integration happens only through explicitly registered Go functions.

### **4. Interpreter over AST**

For now, Filo:

- compiles to an AST,
- executes directly,
- can optionally cache AST or serialized IR.

Bytecode may be added in the future, but only when there is a real need.

---

## Language Reference

### Special Forms

| Name | Syntax | Description |
|------|--------|-------------|
| `if` | `(if cond then [else])` | Conditional execution. Returns `list()` (empty/nil) if else is missing and cond is false. |
| `do` | `(do expr1 expr2 ...)` | Evaluates expressions in order, returns the last result. |
| `let` | `(let ((n v) ...) body)` | Defines local variables. Scope is limited to the body. |
| `letv` | `(letv (n1 n2) (values v1 v2) body)` | Destructures multi-value returns (tuples). |
| `fn` | `(fn (args) body)` | Creates an anonymous function. |
| `def` | `(def name expr)` | Defines a global variable or function in the current scope. |
| `set` | `(set name expr)` | Updates an existing variable in the nearest scope. |
| `values`| `(values v1 v2 ...)` | Returns multiple values (a tuple). |

### Core Builtins

| Category | Function | Description |
|----------|----------|-------------|
| **Math** | `+`, `-`, `*`, `/`, `%` | Basic arithmetic. |
| | `pow` | `(pow x y)` Power function. |
| **Logic** | `=`, `!=` | Equality checks. |
| | `<`, `<=`, `>`, `>=` | Numeric comparison. |
| | `and`, `or`, `not` | Boolean logic. |
| **Types** | `type-of` | Returns "number", "string", "list", etc. |
| | `is-empty` | Returns true for "" or empty list. |
| | `is-nil` | Returns true for nil/empty list (legacy alias). |
| **Lists** | `list` | Creates a list `(list 1 2 3)`. |
| | `length` | Returns list length. |
| | `head`, `tail` | First element / Rest of list. |
| | `nth` | `(nth list index)` Access element by index (0-based). |
| | `append` | `(append list item)` Returns new list with item appended. |
| | `concat` | `(concat l1 l2)` Concatenates two lists. |
| | `map` | `(map fn list)` Applies function to each element. |
| | `fold` | `(fold fn init list)` Reduces list with accumulator. |

### String Builtins

| Function | Description |
|----------|-------------|
| `str-fmt` | `(str-fmt format args...)` Safe implementation of `fmt.Sprintf`. |
| `str-concat` | Concatenates arguments into a string. |
| `str-join` | `(str-join sep list)` Joins list elements with separator. |
| `str-split` | `(str-split sep str)` Splits string into list. |
| `str-len` | Returns string length (runes). |
| `str-sub` | `(str-sub str start [end])` Substring operations. |
| `str-find` | `(str-find sub str)` Validation/Search. |
| `str-replace`| `(str-replace old new str)` Replaces occurrences. |
| `str-trim` | Trims whitespace. |
| `str-upper` | Converts to uppercase. |
| `str-lower` | Converts to lowercase. |

### Extension: filomath

| Function | Description |
|----------|-------------|
| `abs`, `sqrt` | Absolute value, Square root. |
| `floor`, `ceil`, `round`| Rounding operations. |
| `to-int` | `(to-int n)` Truncates float to integer. |
| `sin`, `cos`, `tan` | Trigonometric functions (radians). |
| `log`, `log10`, `exp` | Logarithmic functions. |
| `math-min`, `math-max` | Min/Max of arguments. |
| `pi`, `e` | Constants. |

### Extension: filorand

| Function | Description |
|----------|-------------|
| `rand-float` | Random number [0.0, 1.0). |
| `rand-int` | `(rand-int n)` Random integer [0, n). |
| `rand-seed` | `(rand-seed [n])` Reseed RNG. |
| `uuid-v4` | Generates a standard UUID string. |

---

## Practical examples

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

This script returns `(tuple 3 1500)`, meaning the character is at level three and needs 1,500 XP to reach the next tier.

---

## Integration example with Go

### Register a simple builtin

The following helper sums two numbers provided as positional parameters `a` and `b`.

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

`full-name` expects two string parameters: `first` and `last`. It trims blanks and returns a single string value.

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

### Register a custom aggregator

This builtin receives a list of numbers `xs` and returns a tuple with the minimum and maximum values.

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

### Example with globals

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

## Security

Filo was designed to **avoid compromising the server**, even if a user attempts malicious code:

- infinite loops → stopped by the step limit,
- infinite recursion → stopped by the recursion limit,
- slow scripts → timeout,
- attempts to access external resources → impossible,
- errors never crash the server → `recover()`.

This combination makes Filo **secure by construction**.

---

## How it will be used in projects

### RAD system

- calculated fields,
- validations,
- transformations,
- custom behaviors.

### RPG site

- rules,
- attribute calculations,
- modifiers,
- temporary effects,
- combat automations.

### System configuration

- allow administrators to configure the application using declarative logic.

---

## Current state and next steps

### Current state

- specification consolidated,
- Go integration defined,
- examples and tests included,
- `filo` package implemented and used by engine tools.

### Next steps

- keep expanding documentation and reference material,
- add more safety tests (limits, panic recovery, fuzzing),
- syntax highlighting for Neovim.

---

## Conclusion

Filo exists to solve a real problem:

> **give power to the user without giving up security.**

It is a small, elegant, deterministic language that integrates easily into the Go ecosystem. It lets users write helpful rules without ever putting the server at risk.

Filo is simple enough for anyone to learn, yet powerful enough to express complex rules.
