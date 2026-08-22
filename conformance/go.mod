module github.com/crgimenes/filo/conformance

go 1.27

require (
	github.com/crgimenes/filo v0.0.0
	github.com/crgimenes/prolog v0.1.3
)

// filo is the module under development; always test against the working tree.
replace github.com/crgimenes/filo => ../
