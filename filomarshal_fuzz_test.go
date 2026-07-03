package filo

import (
	"testing"
	"unicode/utf8"
)

// FuzzMarshalUnmarshalInt tests roundtrip stability for integers
func FuzzMarshalUnmarshalInt(f *testing.F) {
	f.Add(0)
	f.Add(1)
	f.Add(-1)
	f.Add(42)
	f.Add(int(1<<31 - 1))
	f.Add(int(-1 << 31))

	f.Fuzz(func(t *testing.T, i int) {
		val, err := Marshal(i)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var result int
		err = Unmarshal(val, &result)
		if err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if result != i {
			t.Errorf("roundtrip mismatch: input %d, output %d", i, result)
		}
	})
}

// FuzzMarshalUnmarshalFloat tests roundtrip stability for floats
func FuzzMarshalUnmarshalFloat(f *testing.F) {
	f.Add(0.0)
	f.Add(1.0)
	f.Add(-1.0)
	f.Add(3.14159)
	f.Add(1e10)
	f.Add(1e-10)

	f.Fuzz(func(t *testing.T, fl float64) {
		val, err := Marshal(fl)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var result float64
		err = Unmarshal(val, &result)
		if err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if result != fl {
			t.Errorf("roundtrip mismatch: input %v, output %v", fl, result)
		}
	})
}

// FuzzMarshalUnmarshalString tests roundtrip stability for strings
func FuzzMarshalUnmarshalString(f *testing.F) {
	f.Add("")
	f.Add("hello")
	f.Add("hello world")
	f.Add("unicode: 日本語")
	f.Add("special: \n\t\\\"")
	f.Add("emoji: 🎉🚀")

	f.Fuzz(func(t *testing.T, s string) {
		val, err := Marshal(s)
		if err != nil {
			// Filo strings are UTF-8 text: Marshal rejects non-UTF-8 input rather
			// than corrupting it. That is the only allowed error here.
			if utf8.ValidString(s) {
				t.Fatalf("Marshal error on valid UTF-8 %q: %v", s, err)
			}
			return
		}

		var result string
		err = Unmarshal(val, &result)
		if err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if result != s {
			t.Errorf("roundtrip mismatch: input %q, output %q", s, result)
		}
	})
}

// FuzzMarshalUnmarshalBool tests roundtrip stability for booleans
func FuzzMarshalUnmarshalBool(f *testing.F) {
	f.Add(true)
	f.Add(false)

	f.Fuzz(func(t *testing.T, b bool) {
		val, err := Marshal(b)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var result bool
		err = Unmarshal(val, &result)
		if err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if result != b {
			t.Errorf("roundtrip mismatch: input %v, output %v", b, result)
		}
	})
}

// FuzzMarshalUnmarshalBytes tests roundtrip stability for byte slices via string
func FuzzMarshalUnmarshalBytes(f *testing.F) {
	f.Add([]byte{})
	f.Add([]byte{0x00})
	f.Add([]byte{0x01, 0x02, 0x03})
	f.Add([]byte("hello"))

	f.Fuzz(func(t *testing.T, data []byte) {
		s := string(data)
		val, err := Marshal(s)
		if err != nil {
			// Bytes cast to a string can be invalid UTF-8; Filo strings are text,
			// so Marshal rejecting that is the contract, not a failure.
			if utf8.ValidString(s) {
				t.Fatalf("Marshal error on valid UTF-8: %v", err)
			}
			return
		}

		var result string
		err = Unmarshal(val, &result)
		if err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if result != s {
			t.Errorf("roundtrip mismatch for bytes")
		}
	})
}

// FuzzMarshalSliceInt tests roundtrip stability for int slices
func FuzzMarshalSliceInt(f *testing.F) {
	f.Add(0, 0, 0)
	f.Add(1, 2, 3)
	f.Add(-1, 0, 1)

	f.Fuzz(func(t *testing.T, a, b, c int) {
		input := []int{a, b, c}

		val, err := Marshal(input)
		if err != nil {
			t.Fatalf("Marshal error: %v", err)
		}

		var result []int
		err = Unmarshal(val, &result)
		if err != nil {
			t.Fatalf("Unmarshal error: %v", err)
		}

		if len(result) != len(input) {
			t.Fatalf("length mismatch: %d != %d", len(result), len(input))
		}
		for i := range input {
			if result[i] != input[i] {
				t.Errorf("element %d mismatch: %d != %d", i, result[i], input[i])
			}
		}
	})
}
