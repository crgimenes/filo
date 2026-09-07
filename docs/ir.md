# Filo IR — the instruction set every runtime implements

This document is the contract between the language and its runtimes. Today
there are two: the Go engine in this repository and the C port under `c/`.
Both lower the same source to the same instructions and evaluate them by the
same rules, so a script's outcome — its value, whether it errors, the globals
it leaves behind, the number of steps it takes — is identical on every
runtime. The corpus under `testdata/corpus` and the Prolog conformance suite
are the checks; this document is what they check against.

The IR is a **tree of instructions**, not a linear byte stream. Each
instruction has an opcode and typed operands, and its children are
instructions. A runtime dispatches on the opcode as an integer; nothing is
looked up by name while a script runs except the two dynamic cases below.
There is deliberately no serialized file format: the source text is the only
artifact that crosses a boundary, and a runtime lowers it on load.

## Source to IR

Lowering happens after parsing and constant folding, on the parse tree:

1. **Head position decides the form.** A list whose first element is the
   plain symbol `if`, `cond`, `do`, `and`, `or`, `let`, `letv`, `set`, `fn`,
   `def`, `values`, `tuple`, `exit` or `return` is that special form, even
   if a local variable of the same name is in scope. The same names are
   ordinary variables anywhere else: `(let ((if 1)) if)` is `1`.
2. **Scope resolution.** `let`, `letv` and `fn` open a scope. A symbol bound
   in an enclosing scope becomes `LOCAL depth index` — how many scopes up,
   and which slot. Otherwise a symbol that names a registered builtin
   becomes `BUILTIN name`, and any other symbol becomes `GLOBAL id name`
   through the engine's symbol table (or `DYNAMIC name` when lowering
   without a table). `let` is sequential: each binding's value is resolved
   in a scope that already contains the bindings before it.
3. **Malformed forms lower lazily.** A special form whose shape is wrong
   does not fail at lowering; it becomes an `INVALID ctx message`
   instruction that raises exactly the error the form would raise when
   evaluated — and only when evaluated. `(if #t 1 (let))` is `1`. The
   messages and the checks that produce them are listed under each form.
   Four shape errors *are* lowering errors, reported as `compile error: …`:
   `invalid let binding`, `let binding name must be symbol`,
   `letv names must be symbols`, `fn params must be symbols`.
4. The empty list `()` lowers to `EMPTY`, which errors when evaluated.

## Execution model

- **Number literals** in source are `[+-]?(digits[.digits*]|.digits)
  ([eE][+-]?digits)?`; any other atom is a symbol. Only `(number s)` defers
  to the host's parser.
- **Values**: number (IEEE double), bool, string (bytes, UTF-8 by
  convention), list, tuple, func. Lists and tuples are immutable from the
  script's point of view — builtins return new ones.
- **Frames**: a frame is a fixed array of slots plus a parent link. `LET`,
  `LETV` and a closure call each push a frame; `FN` captures the frame in
  effect when it is evaluated (by reference: a later `set` on a captured
  variable is visible through the closure, and the closure can `set` it).
- **Globals** live in a table indexed by id; a global that was never set
  reads as an error. Every global set during a run is returned to the host
  when the run ends.
- **Steps**: evaluating an instruction costs exactly one step, counted
  before the instruction does anything, and the run errors with
  `step limit exceeded` when the count passes the limit. Children cost their
  own steps when they are evaluated; operands that are not instructions
  (names, binding counts, clause markers) cost nothing. This makes the step
  count equal to the number of parse-tree nodes the script visits, so a
  limit that held on one runtime holds on every runtime.
- **Recursion**: a closure call increments a depth counter for the duration
  of the call and errors with `recursion limit exceeded` beyond the limit.
- **Timeout / cancellation** are checked at every step; how they are
  measured is the host's business.
- **Signals**: `exit` and `return` unwind as signals, not errors. `return`
  is caught by the nearest closure call and becomes its value; at top level
  it acts like `exit`. `exit` ends the run with its value from wherever it
  is raised: inside a closure that a builtin calls, in a builtin's or a
  call's arguments, in a binding. Error context (below) is never added
  around a signal, so no layer can turn it into an error.

## Error context

Errors carry the chain of forms they crossed, innermost first, so that
`(if #t (+ 1 "a"))` fails with `in if: in builtin "+": expected number, got
string`. The rule per instruction is given in the table as *context*. Two
wrappers are used:

- `in <ctx>: <err>` — added by the form to any error from its children.
- for builtin calls, `while evaluating arguments for "<name>": argument
  <i>: <err>` when argument *i* (0-based) fails, and `in builtin "<name>":
  <err>` when the builtin itself fails.

The Go-only tests assert this wording; the corpus asserts only that an error
occurred. A runtime that reproduces the wording is a better runtime, but the
corpus is the contract.

## Instructions

Operands in *italics* are not instructions and cost no step. `args…` are
child instructions. "Errors" lists the checks in the order they run;
anything evaluated before a failing check has already been evaluated (and
counted, and may have had side effects).

| Op | Operands | Evaluation |
|----|----------|------------|
| `CONST` | *value* | The value. |
| `LOCAL` | *depth*, *index*, *name* | The slot *index* of the frame *depth* levels up. |
| `GLOBAL` | *id*, *name* | The global *id*. Errors `undefined global: <name>` when unset. |
| `DYNAMIC` | *name* | The global named *name*, looked up by name. Errors `undefined symbol: <name>` when unset. Only produced when lowering without a symbol table. |
| `BUILTIN` | *name* | Always errors `builtin "<name>" cannot be used as value`. A builtin is callable, never a value. |
| `EMPTY` | — | Always errors `empty list expression`. |
| `INVALID` | *ctx*, *message* | Always errors `in <ctx>: <message>`. |
| `IF` | args… | Errors `if expects 2 or 3 arguments (condition then [else])` unless 2 or 3 args. Evaluates args[0]; it must be a bool (`expected bool, got <kind>`). True: evaluates args[1]. False: evaluates args[2], or yields the empty list with 2 args. Context `if`. |
| `COND` | *clauses…* | Each clause is *(test, body…)*, *(ELSE, body…)* or *INVALID(message)*. Clauses run in order; an INVALID clause errors when reached with its message; ELSE runs its body; otherwise evaluate test (must be a bool), and on true run the body as a sequence. No clause taken: the empty list. Context `cond`. Messages: `cond clause must be a list of a test and a body` for a clause that is not a list or has fewer than two elements (checked before ELSE is recognized, so `(else)` alone is an error). |
| `DO` | args… | Errors `do expects at least 1 expression` with no args. Evaluates all, yields the last. Context `do`. |
| `AND` | args… | Evaluates left to right; each must be a bool; stops at the first false. No args: true. Context `and`. |
| `OR` | args… | Same, stops at the first true. No args: false. Context `or`. |
| `LET` | *n*, values…(n), body… | Errors `let expects bindings and body` with an empty body. Pushes a frame of *n* slots, evaluates each value in order **with the new frame already active** (so a value may read the bindings before it), then runs the body as a sequence. Context `let`. |
| `LETV` | *names*, tuple, body… | Evaluates tuple **in the outer frame**; it must be a tuple (`letv expects tuple expression`) of exactly len(*names*) elements (`letv arity mismatch`). Pushes a frame holding a **copy** of the elements (never aliasing the tuple), runs the body as a sequence. Context `letv`. |
| `SET` | args… | Errors `set expects name and expression` unless exactly 2 args. Evaluates args[1]. Then args[0] must be a `LOCAL` (write its slot), a `GLOBAL` (write the id), or a `DYNAMIC` (define by name); anything else errors `set name must be symbol`. Yields the value. Context `set`. |
| `FN` | *params*, body… | Errors `fn expects parameters and body` with an empty body. Yields a func capturing *params*, the body and the current frame. Context `fn`. |
| `DEF` | *name*, value | *name* is the symbol or absent when the form's second element was not a symbol; then errors `def name must be symbol` **before** evaluating. Evaluates value, defines the global by name, yields the value. Context `def`. |
| `TUPLE` | *ctx*, args… | Evaluates all, yields a tuple. Context is *ctx*: `values` or `tuple`, whichever was written. |
| `EXIT` | args… | Errors `exit expects 0 or 1 argument` beyond one (no context). Evaluates the arg if present (else the empty list) and raises the exit signal with it. |
| `RETURN` | args… | Same with `return expects 0 or 1 argument` and the return signal. |
| `CALLB` | *name*, *fn*, args… | Evaluates args left to right (argument context on failure), then calls the builtin (builtin context on failure). Yields its result. A signal from an argument or from a closure the builtin calls passes through untouched. |
| `CALL` | head, args… | Evaluates head (context `call`). It must be a func, else errors `attempt to call non-function (got <kind>)` with no context. Then, with context `function call`: increments recursion depth (`recursion limit exceeded`), checks arity (`function expects <n> arguments, got <m>`), evaluates args left to right into a new frame whose parent is the func's captured frame (`in call arguments: argument <i>: <err>`), runs the body as a sequence, catches a return signal as the value, decrements depth. |

A `CALL` whose head is a `DYNAMIC` first checks whether *name* is a
registered builtin and, if so, behaves as `CALLB` — this preserves the
by-name path for lowering done without a builtin table.

**Sequence** (`body…` above): evaluates each instruction in order and yields
the last; an empty sequence errors `empty body`. It can only be empty for
`LETV`.

## Malformed-form messages

Produced by `INVALID` (raised lazily, in the form's context):

| Form | Shape | Message |
|------|-------|---------|
| `let` | fewer than 2 elements after `let`, or bindings not a list | `let expects bindings and body` / `let expects binding list` |
| `letv` | fewer than 2 elements after `letv`, or names not a list | `letv expects bindings and body` / `letv expects name list` |
| `fn` | fewer than 1 element after `fn`, or params not a list | `fn expects parameters and body` / `fn expects parameter list` |
| `def` | not exactly 2 elements after `def` | `def expects name and expression` |

`if`, `do`, `and`, `or`, `set`, `values`, `tuple`, `exit` and `return` keep
their arguments and validate them when evaluated, as listed in the table.

## What a runtime may choose

- Memory: the Go engine relies on its garbage collector; the C port uses an
  arena reset after every run, copying surviving globals out. Neither is
  visible to a script.
- Limits (parse depth, default step/recursion limits) are configuration.
- Number formatting for `(string n)` must be the shortest representation
  that round-trips; how it is computed is the runtime's business.
- The wording of error messages: see *Error context*.
