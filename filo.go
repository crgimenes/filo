// Package filo implements Filo, a small embeddable Lisp for Go: a scripting and
// configuration language with minimal syntax, deterministic execution, and
// explicit limits on every dimension that could put a host process at risk.
//
// A host embeds Filo to let end users write short logic — validations,
// expressions, field rules, configuration — without ever exposing the machine.
// Scripts run under a StepLimit, a RecursionLimit, and a Timeout (honored via
// context.Context), the executor recovers from panics, and the only capabilities
// a script has are the Go functions the host explicitly registers as builtins:
// no file, network, or syscall access.
//
// The high-level entry point is New / DoString / MustGet* (see integration.go);
// for compiled reuse and full control over globals and limits, use Engine,
// Compile, and RunScript (see engine.go). Go values convert to and from Filo
// with Marshal / Unmarshal and their Value-level forms. Optional builtin sets
// live in the filostrings, filomath, filorand, filojson, and filoprint
// subpackages; source formatting lives in the filofmt command.
package filo
