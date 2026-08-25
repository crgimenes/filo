package filo

import (
	"reflect"
	"testing"
)

func TestMarshalBasicTypes(t *testing.T) {
	tests := []struct {
		name     string
		input    any
		wantKind Kind
		check    func(Value) bool
	}{
		{"bool true", true, KBool, func(v Value) bool { return v.Bool == true }},
		{"bool false", false, KBool, func(v Value) bool { return v.Bool == false }},
		{"int", 42, KNumber, func(v Value) bool { return v.Num == 42 }},
		{"int64", int64(123), KNumber, func(v Value) bool { return v.Num == 123 }},
		{"float64", 3.14, KNumber, func(v Value) bool { return v.Num == 3.14 }},
		{"string", "hello", KString, func(v Value) bool { return v.Str == "hello" }},
		{"nil", nil, KTuple, func(v Value) bool { return len(v.Tup) == 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := MarshalToValue(tt.input)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}
			if val.Kind != tt.wantKind {
				t.Errorf("got kind %v, want %v", val.Kind, tt.wantKind)
			}
			if !tt.check(val) {
				t.Errorf("value check failed for %v", val)
			}
		})
	}
}

func TestMarshalSlice(t *testing.T) {
	input := []int{1, 2, 3}
	val, err := MarshalToValue(input)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if val.Kind != KList {
		t.Fatalf("expected KList, got %v", val.Kind)
	}
	if len(val.List) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(val.List))
	}
	for i, v := range val.List {
		if v.Kind != KNumber || v.Num != float64(i+1) {
			t.Errorf("element %d: expected %d, got %v", i, i+1, v)
		}
	}
}

func TestMarshalEmptySliceRoundtrip(t *testing.T) {
	s, err := Marshal([]int{})
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if s != "(list)" {
		t.Fatalf("expected (list), got %q", s)
	}
	var out []int
	err = Unmarshal(s, &out)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(out) != 0 {
		t.Fatalf("expected empty slice, got %v", out)
	}
}

func TestMarshalMap(t *testing.T) {
	input := map[string]int{"a": 1, "b": 2}
	val, err := MarshalToValue(input)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if val.Kind != KList {
		t.Fatalf("expected KList, got %v", val.Kind)
	}
	// Should be sorted by key
	if len(val.List) != 2 {
		t.Fatalf("expected 2 pairs, got %d", len(val.List))
	}
	// First pair should be "a"
	if val.List[0].Tup[0].Str != "a" {
		t.Errorf("expected first key 'a', got %v", val.List[0].Tup[0])
	}
}

type SimpleStruct struct {
	Name  string `filo:"name"`
	Age   int    `filo:"age"`
	Skip  string `filo:"-"`
	NoTag string
}

func TestMarshalStruct(t *testing.T) {
	input := SimpleStruct{Name: "Alice", Age: 30, Skip: "ignored", NoTag: "value"}
	val, err := MarshalToValue(input)
	if err != nil {
		t.Fatalf("Marshal error: %v", err)
	}
	if val.Kind != KList {
		t.Fatalf("expected KList, got %v", val.Kind)
	}
	// Should have 3 fields (Skip is excluded)
	if len(val.List) != 3 {
		t.Fatalf("expected 3 pairs, got %d", len(val.List))
	}

	// Check field values
	fieldMap := make(map[string]Value)
	for _, pair := range val.List {
		key, _ := pair.Tup[0].AsString()
		fieldMap[key] = pair.Tup[1]
	}

	if fieldMap["name"].Str != "Alice" {
		t.Errorf("expected name=Alice, got %v", fieldMap["name"])
	}
	if fieldMap["age"].Num != 30 {
		t.Errorf("expected age=30, got %v", fieldMap["age"])
	}
	if fieldMap["notag"].Str != "value" {
		t.Errorf("expected notag=value, got %v", fieldMap["notag"])
	}
}

func TestUnmarshalBasicTypes(t *testing.T) {
	tests := []struct {
		name  string
		val   Value
		alloc func() any
		check func(any) bool
	}{
		{"bool", VBool(true), func() any { var b bool; return &b }, func(v any) bool { return *v.(*bool) == true }},
		{"int", VNum(42), func() any { var i int; return &i }, func(v any) bool { return *v.(*int) == 42 }},
		{"float64", VNum(3.14), func() any { var f float64; return &f }, func(v any) bool { return *v.(*float64) == 3.14 }},
		{"string", VString("hello"), func() any { var s string; return &s }, func(v any) bool { return *v.(*string) == "hello" }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := tt.alloc()
			err := UnmarshalFromValue(tt.val, target)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}
			if !tt.check(target) {
				t.Errorf("value check failed")
			}
		})
	}
}

func TestUnmarshalSlice(t *testing.T) {
	val := VList([]Value{VNum(1), VNum(2), VNum(3)})
	var result []int
	err := UnmarshalFromValue(val, &result)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(result))
	}
	for i, v := range result {
		if v != i+1 {
			t.Errorf("element %d: expected %d, got %d", i, i+1, v)
		}
	}
}

func TestUnmarshalMap(t *testing.T) {
	val := VList([]Value{
		VTuple([]Value{VString("a"), VNum(1)}),
		VTuple([]Value{VString("b"), VNum(2)}),
	})
	var result map[string]int
	err := UnmarshalFromValue(val, &result)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if result["a"] != 1 || result["b"] != 2 {
		t.Errorf("unexpected result: %v", result)
	}
}

func TestUnmarshalStruct(t *testing.T) {
	val := VList([]Value{
		VTuple([]Value{VString("name"), VString("Bob")}),
		VTuple([]Value{VString("age"), VNum(25)}),
	})
	var result SimpleStruct
	err := UnmarshalFromValue(val, &result)
	if err != nil {
		t.Fatalf("Unmarshal error: %v", err)
	}
	if result.Name != "Bob" {
		t.Errorf("expected Name=Bob, got %s", result.Name)
	}
	if result.Age != 25 {
		t.Errorf("expected Age=25, got %d", result.Age)
	}
}

func TestRoundtripStability(t *testing.T) {
	type Nested struct {
		Items []int             `filo:"items"`
		Data  map[string]string `filo:"data"`
	}
	type Complex struct {
		Name   string  `filo:"name"`
		Value  float64 `filo:"value"`
		Active bool    `filo:"active"`
		Nested Nested  `filo:"nested"`
	}

	original := Complex{
		Name:   "test",
		Value:  123.456,
		Active: true,
		Nested: Nested{
			Items: []int{1, 2, 3, 4, 5},
			Data:  map[string]string{"key1": "val1", "key2": "val2"},
		},
	}

	// Multiple roundtrips
	for i := range 5 {
		val, err := MarshalToValue(original)
		if err != nil {
			t.Fatalf("roundtrip %d: Marshal error: %v", i, err)
		}

		var result Complex
		err = UnmarshalFromValue(val, &result)
		if err != nil {
			t.Fatalf("roundtrip %d: Unmarshal error: %v", i, err)
		}

		if !reflect.DeepEqual(original, result) {
			t.Errorf("roundtrip %d: mismatch\noriginal: %+v\nresult: %+v", i, original, result)
		}

		original = result // Use result for next iteration
	}
}

func TestMarshalUnmarshalRoundtrip(t *testing.T) {
	tests := []struct {
		name  string
		input any
		alloc func() any
	}{
		{"int", 42, func() any { var v int; return &v }},
		{"string", "hello world", func() any { var v string; return &v }},
		{"bool", true, func() any { var v bool; return &v }},
		{"float64", 3.14159, func() any { var v float64; return &v }},
		{"[]int", []int{1, 2, 3}, func() any { var v []int; return &v }},
		{"[]string", []string{"a", "b"}, func() any { var v []string; return &v }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, err := MarshalToValue(tt.input)
			if err != nil {
				t.Fatalf("Marshal error: %v", err)
			}

			target := tt.alloc()
			err = UnmarshalFromValue(val, target)
			if err != nil {
				t.Fatalf("Unmarshal error: %v", err)
			}

			// Second roundtrip
			val2, err := MarshalToValue(reflect.ValueOf(target).Elem().Interface())
			if err != nil {
				t.Fatalf("Marshal2 error: %v", err)
			}

			target2 := tt.alloc()
			err = UnmarshalFromValue(val2, target2)
			if err != nil {
				t.Fatalf("Unmarshal2 error: %v", err)
			}

			v1 := reflect.ValueOf(target).Elem().Interface()
			v2 := reflect.ValueOf(target2).Elem().Interface()
			if !reflect.DeepEqual(v1, v2) {
				t.Errorf("roundtrip mismatch: %v != %v", v1, v2)
			}
		})
	}
}

func TestUnmarshalErrors(t *testing.T) {
	tests := []struct {
		name   string
		val    Value
		target any
	}{
		{"non-pointer target", VNum(1), 42},
		{"nil target", VNum(1), (*int)(nil)},
		{"type mismatch bool->int", VBool(true), new(int)},
		{"type mismatch string->bool", VString("hi"), new(bool)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := UnmarshalFromValue(tt.val, tt.target)
			if err == nil {
				t.Error("expected error, got nil")
			}
		})
	}
}

func TestMarshalErrors(t *testing.T) {
	ch := make(chan int)
	_, err := MarshalToValue(ch)
	if err == nil {
		t.Error("expected error for channel, got nil")
	}

	fn := func() {}
	_, err = MarshalToValue(fn)
	if err == nil {
		t.Error("expected error for function, got nil")
	}
}
