# Language corpus

Plain-text cases that pin what a Filo script evaluates to. This directory is
the behavioral contract of the language: every runtime runs the same files and
must agree with them. Cases assert outcomes — the resulting value, the fact
that an error occurred, where, and how its message ends, the globals left
behind. The end of the message is the part that says what happened; the
contexts before it ("in let: ", "in builtin ..." ) differ between the tree
and the bytecode, and are not checked.

Because it is a contract between implementations that live in different
repositories, this directory is duplicated and the two copies must stay
identical: [filo](https://github.com/crgimenes/filo) runs it from
`corpus_test.go`, [clang_filo](https://github.com/crgimenes/clang_filo) from
`corpus_runner.c`. A case added or changed on either side belongs on both, and
`diff -ru` between the two checkouts must come back empty.

## Format

```
# comments and blank lines are allowed between cases
packs: math strings          # optional; builtin packs a runtime must register

=== case name                # unique within the file
given x = (list 1 2)         # optional; an input global, as a Filo expression
limits steps=100 recursion=5 # optional; evaluation limits (defaults otherwise)
needs host-pow               # optional; a capability the host has to supply
(script lines ...)
--- want
(expected value, as a Filo expression)
--- globals                  # optional; globals expected after the run
x = 42

=== another case
(script)
--- error                    # any error; nothing may follow on this line
--- at 1:1                   # optional; where the error happened, line:col
--- message division by zero # optional; how the error's message ends
```

- The script is everything between the case header lines and the first
  `--- ` marker, trailing blank lines dropped.
- `--- want` holds an expression evaluated on a fresh engine; comparison is
  exact (no epsilon), `NaN` equals `NaN`, lists and tuples compare deeply.
  Functions cannot be expected. The expression ends at the first blank line;
  after it, only blank lines and `#` comments may appear until the next case.
- `--- error` and `--- want` are mutually exclusive and one is required.
- `--- at L:C`, once, right after `--- error`: the line and column (both from
  1, the column in bytes, a leading BOM not counted) the runtime reports for
  the error. A run or compile error is the innermost expression that failed;
  a parse error is where the text stops making sense, or where a list or a
  string that never closes was opened. Every runtime and every way it runs
  (the tree, the VM) must report the same place; the one exception is a step
  limit on a VM, which counts steps differently from the tree.
- A runtime that does not implement a pack listed in `packs:` skips the file.
- `needs` names something the runtime can only do when its host supplies it,
  so a runtime running without it skips that case instead of counting as a
  disagreement. The capabilities are `host-pow` (a power with a fractional
  exponent, which needs libm) and `host-math` (square root, the logarithms
  and the trigonometric functions of the `math` pack). The C runtime on a
  target with no C library skips exactly these; everything else it answers
  identically.

## What belongs here

Intended behavior only. A behavior that was noticed and not yet decided on
goes in a `quirks.txt` that pins what the engine does today, so a change is
deliberate and visible instead of accidental; when the decision is made, the
case moves to the proper file with the intended outcome. (The file does not
exist right now: every quirk found so far has been decided.)

Limits (parse depth, step counts) are implementation-defined; cases that use
`limits` keep a wide margin and never probe the exact boundary.
