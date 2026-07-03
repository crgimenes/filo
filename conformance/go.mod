module github.com/crgimenes/filo/conformance

go 1.26

require (
	github.com/crgimenes/filo v0.0.0
	github.com/crgimenes/prolog v0.0.0
)

replace github.com/crgimenes/filo => ../

// The engine needs the mulF/divF float-overflow fix (2026-07-03); switch to a
// tagged version once it is released.
replace github.com/crgimenes/prolog => ../../prolog
