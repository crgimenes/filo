package filojson

import (
	"context"
	"testing"

	"github.com/crgimenes/filo"
)

func newEngine() *filo.Engine {
	eng := filo.NewEngine()
	filo.RegisterStringBuiltins(eng)
	RegisterJSONBuiltins(eng)
	return eng
}

func TestJSONNullBuiltin(t *testing.T) {
	eng := newEngine()
	v, _, err := eng.RunScript(context.Background(), "(json-null)", nil, filo.EvalConfig{})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if v.Kind != filo.KTuple || len(v.Tup) != 0 {
		t.Fatalf("expected empty tuple as json-null, got %v", v)
	}
}

func TestMarshalSimple(t *testing.T) {
	eng := newEngine()
	// number
	v, _, err := eng.RunScript(context.Background(), "(json-marshal 42)", nil, filo.EvalConfig{})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	s, _ := v.AsString()
	if s != "42" {
		t.Fatalf("expected 42, got %q", s)
	}

	// list
	v, _, err = eng.RunScript(context.Background(), "(json-marshal (list 1 2 3))", nil, filo.EvalConfig{})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	s, _ = v.AsString()
	if s != "[1,2,3]" {
		t.Fatalf("expected [1,2,3], got %q", s)
	}
}

func TestMarshalObjectLikeList(t *testing.T) {
	eng := newEngine()
	// (list (list "a" 1) (list "b" (list 2 3))) -> {"a":1,"b":[2,3]}
	src := "(json-marshal (list (list \"a\" 1) (list \"b\" (list 2 3))))"
	v, _, err := eng.RunScript(context.Background(), src, nil, filo.EvalConfig{})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	s, _ := v.AsString()
	if s != "{\"a\":1,\"b\":[2,3]}" && s != "{\"b\":[2,3],\"a\":1}" {
		// JSON maps may marshal in any order; both acceptable
		t.Fatalf("unexpected object json: %q", s)
	}
}

func TestUnmarshalSimple(t *testing.T) {
	eng := newEngine()
	v, _, err := eng.RunScript(context.Background(), "(json-unmarshal \"[1,2,3]\")", nil, filo.EvalConfig{})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	list, err := v.AsList()
	if err != nil || len(list) != 3 {
		t.Fatalf("expected list len 3, got %v err=%v", v, err)
	}
}

func TestUnmarshalObject(t *testing.T) {
	eng := newEngine()
	v, _, err := eng.RunScript(context.Background(), "(json-unmarshal \"{\\\"x\\\":1,\\\"y\\\":[2]}\")", nil, filo.EvalConfig{})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	pairs, err := v.AsList()
	if err != nil {
		t.Fatalf("expected list of pairs: %v", err)
	}
	if len(pairs) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(pairs))
	}
}

func TestRoundTrip(t *testing.T) {
	eng := newEngine()
	// Round-trip object
	src := "(let ((obj (json-unmarshal \"{\\\"a\\\":1,\\\"b\\\":null,\\\"c\\\":[1,2]}\"))) (json-marshal obj))"
	v, _, err := eng.RunScript(context.Background(), src, nil, filo.EvalConfig{})
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	s, _ := v.AsString()
	// Key order may vary; accept both sorted and non-sorted depending on map marshal order
	if s != "{\"a\":1,\"b\":null,\"c\":[1,2]}" && s != "{\"a\":1,\"c\":[1,2],\"b\":null}" && s != "{\"b\":null,\"a\":1,\"c\":[1,2]}" {
		t.Fatalf("unexpected roundtrip json: %q", s)
	}
}
