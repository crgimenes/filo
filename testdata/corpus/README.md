# Language corpus

Plain-text cases that pin what a Filo script evaluates to. This directory is
the behavioral contract of the language: every runtime (the Go engine here, the
C port) runs the same files and must agree with them. Cases assert outcomes —
the resulting value, the fact that an error occurred, the globals left behind —
never the wording of an error message.

## Format

```
# comments and blank lines are allowed between cases
packs: math strings          # optional; builtin packs a runtime must register

=== case name                # unique within the file
given x = (list 1 2)         # optional; an input global, as a Filo expression
limits steps=100 recursion=5 # optional; evaluation limits (defaults otherwise)
(script lines ...)
--- want
(expected value, as a Filo expression)
--- globals                  # optional; globals expected after the run
x = 42

=== another case
(script)
--- error                    # any error; nothing may follow on this line
```

- The script is everything between the case header lines and the first
  `--- ` marker, trailing blank lines dropped.
- `--- want` holds an expression evaluated on a fresh engine; comparison is
  exact (no epsilon), `NaN` equals `NaN`, lists and tuples compare deeply.
  Functions cannot be expected. The expression ends at the first blank line;
  after it, only blank lines and `#` comments may appear until the next case.
- `--- error` and `--- want` are mutually exclusive and one is required.
- A runtime that does not implement a pack listed in `packs:` skips the file.

## What belongs here

Intended behavior only. A behavior that was noticed and not yet decided on
goes in a `quirks.txt` that pins what the engine does today, so a change is
deliberate and visible instead of accidental; when the decision is made, the
case moves to the proper file with the intended outcome. (The file does not
exist right now: every quirk found so far has been decided.)

Limits (parse depth, step counts) are implementation-defined; cases that use
`limits` keep a wide margin and never probe the exact boundary.
