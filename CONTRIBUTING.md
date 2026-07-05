# Contributing to Filo

Thanks for your interest. Filo is a small embeddable scripting language, and its
entire value is in what it refuses to do: it must never be able to take a host
application down. Every contribution is judged against that first.

## The prime directive: the host survives

Filo exists because giving end users scripting power usually means choosing
between too weak and too dangerous. Filo is the third option, and it only works
if the guarantees hold with no exceptions:

- **Deterministic execution.** Same program, same inputs, same result; no
  wall-clock, no randomness, no I/O unless the host explicitly wires it in.
- **Bounded everything.** Steps, recursion depth, memory, value sizes; every
  dimension that could run away has a limit, and the limit is enforced, not
  advisory.
- **Errors, never panics.** A hostile or broken script returns an error to the
  host; it does not crash, hang, or exhaust the process.

A change that makes a script faster or a feature nicer by weakening a limit will
be declined. That trade is the one thing this project does not make.

## YAGNI, applied to a language

A language grows forever if you let it. The default answer to "add a builtin" or
"add syntax" is no; it becomes yes when a real host case needs it and nothing
already in the language covers it. Convenience alone does not justify surface,
because every addition is permanent: programs written against it never break
retroactively.

- Semantics changes are the most expensive kind; existing scripts must keep
  working, so anything observable needs a strong reason and tests that pin it.
- Prefer a host-side function (the host controls its own API) over a new
  language builtin.

## Dependencies

Pure Go. The interpreter itself uses only the standard library;
`golang.org/x/term` exists for the REPL tool and that is the whole list. No cgo,
ever, and no new dependencies; an embeddable language that drags dependencies
into its hosts has failed at being embeddable.

## Code style

`gofmt`, US English everywhere (comments, identifiers, docs). House rules:

- No inline `if` init; assign on its own line, then `if`.
- No `else` after a terminal branch; return early. Prefer `switch` over long
  `else if` chains.
- Comments explain why, not what.

## Tests

Language work needs language tests:

- Every semantic change or new builtin comes with table-driven tests covering
  the normal case and the hostile one (limit hit, wrong type, bad arity).
- The fuzzers are first-class citizens; keep `go test -fuzz` targets building
  and never quiet a fuzz finding by loosening a check.
- Bound everything: `go test -timeout 30s ./...` is the baseline.

```sh
go fix ./...
gofmt -l .        # must print nothing
go vet ./...
golangci-lint run ./...
go test -timeout 30s -count 1 ./...
```

## Proposing a change

The default branch is `trunk`. Typos and doc fixes can go straight to a PR. For
anything that touches semantics, limits, or the host API, open an issue first;
agreeing on the design before the code exists saves you from writing something
that can't be merged.
