package filo

import "testing"

// TestStringAllTypes tests Value.String() for all types.
func TestStringAllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"number-int", VNum(42), "42"},
		{"number-float", VNum(3.14), "3.14"},
		{"bool-true", VBool(true), "#t"},
		{"bool-false", VBool(false), "#f"},
		{"string", VString("hello"), `"hello"`},
		{"list-empty", VList(nil), "(list)"},
		{"list-nums", VList([]Value{VNum(1), VNum(2)}), "(list 1 2)"},
		{"tuple-empty", VTuple(nil), "(tuple)"},
		{"tuple-vals", VTuple([]Value{VNum(1), VString("a")}), `(tuple 1 "a")`},
		{"func", VFunc(&Func{Params: []string{"x"}}), "<fn>"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.val.String()
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// TestDescribeAllTypes tests value.describe() for all types.
func TestDescribeAllTypes(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		val  Value
		want string
	}{
		{"number", VNum(42), "number"},
		{"bool", VBool(true), "bool"},
		{"string", VString("hello"), "string"},
		{"list", VList(nil), "list"},
		{"tuple", VTuple(nil), "tuple"},
		{"func", VFunc(nil), "function"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.val.describe()
			if got != tc.want {
				t.Fatalf("want %q, got %q", tc.want, got)
			}
		})
	}
}

// TestAsListAsTupleErrors tests error cases for AsList/AsTuple.
func TestAsListAsTupleErrors(t *testing.T) {
	t.Parallel()

	// AsList on non-list
	_, err := VNum(42).AsList()
	if err == nil {
		t.Fatal("expected error for AsList on number")
	}

	// AsTuple on non-tuple
	_, err = VNum(42).AsTuple()
	if err == nil {
		t.Fatal("expected error for AsTuple on number")
	}
}

// TestEnsureSameKindFunc tests ensureSameKind function.
func TestEnsureSameKindFunc(t *testing.T) {
	t.Parallel()

	// Empty list
	_, err := ensureSameKind([]Value{})
	if err == nil {
		t.Fatal("expected error for empty list")
	}

	// Same kind
	kind, err := ensureSameKind([]Value{VNum(1), VNum(2), VNum(3)})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if kind != KNumber {
		t.Fatalf("want KNumber, got %v", kind)
	}

	// Mixed kinds
	_, err = ensureSameKind([]Value{VNum(1), VString("a")})
	if err == nil {
		t.Fatal("expected error for mixed kinds")
	}
}
