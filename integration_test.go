package filo

import (
	"testing"
)

// TestDoString executes a simple Filo script that assigns a global variable
// and then verifies that the variable was correctly set.
func TestDoString(t *testing.T) {
	f := New()
	defer f.Close()

	// Execute Filo script setting global variable 'x' to 42.
	if err := f.DoString("(set x 42)"); err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	// Retrieve 'x' and verify its value.
	x := f.MustGetInt("x")
	if x != 42 {
		t.Fatalf("Expected x = 42, got %d", x)
	}
}

// TestSetGlobalInt verifies that an integer is correctly set as a global variable.
func TestSetGlobalInt(t *testing.T) {
	f := New()
	defer f.Close()

	f.SetGlobal("num", 100)
	num := f.MustGetInt("num")
	if num != 100 {
		t.Fatalf("Expected num = 100, got %d", num)
	}
}

// TestSetGlobalString verifies that a string is correctly set as a global variable.
func TestSetGlobalString(t *testing.T) {
	f := New()
	defer f.Close()

	f.SetGlobal("greeting", "hello")
	greeting := f.MustGetString("greeting")
	if greeting != "hello" {
		t.Fatalf("Expected greeting = 'hello', got %s", greeting)
	}
}

// TestSetGlobalBool verifies that a bool is correctly set as a global variable.
func TestSetGlobalBool(t *testing.T) {
	f := New()
	defer f.Close()

	f.SetGlobal("enabled", true)
	enabled := f.MustGetBool("enabled")
	if !enabled {
		t.Fatalf("Expected enabled = true, got false")
	}

	f.SetGlobal("disabled", false)
	disabled := f.MustGetBool("disabled")
	if disabled {
		t.Fatalf("Expected disabled = false, got true")
	}
}

// TestSetGlobalTable verifies that a []string is correctly set as a Filo list global.
func TestSetGlobalTable(t *testing.T) {
	f := New()
	defer f.Close()

	expected := []string{"one", "two", "three"}
	f.SetGlobal("list", expected)

	list := f.MustGetTable("list")
	if len(list) != len(expected) {
		t.Fatalf("Expected list length %d, got %d", len(expected), len(list))
	}
	for i, v := range expected {
		if list[i] != v {
			t.Fatalf("Expected list[%d] = %s, got %s", i, v, list[i])
		}
	}
}

// TestSetGlobalMap verifies that a map[string]string is correctly set as a Filo global.
func TestSetGlobalMap(t *testing.T) {
	f := New()
	defer f.Close()

	m := make(map[string]string)
	m["one"] = "uno"
	m["two"] = "dos"
	m["three"] = "tres"
	f.SetGlobal("map", m)

	mapTable := f.MustGetMap("map")
	if len(mapTable) != len(m) {
		t.Fatalf("Expected map length %d, got %d", len(m), len(mapTable))
	}
	for k, v := range m {
		if mapTable[k] != v {
			t.Fatalf("Expected map[%s] = %s, got %s", k, v, mapTable[k])
		}
	}
}

func TestSetGlobalMapDeterministicOrder(t *testing.T) {
	f := New()
	defer f.Close()

	// Use intentionally unsorted insertion order.
	f.SetGlobal("map", map[string]string{
		"b": "2",
		"a": "1",
		"c": "3",
	})

	v, ok := f.globals["map"]
	if !ok {
		t.Fatalf("expected global %q", "map")
	}
	list, err := v.AsList()
	if err != nil {
		t.Fatalf("expected list global: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 pairs, got %d", len(list))
	}

	getKey := func(i int) string {
		tuple, tupleErr := list[i].AsTuple()
		if tupleErr != nil {
			t.Fatalf("expected tuple at %d: %v", i, tupleErr)
		}
		if len(tuple) != 2 {
			t.Fatalf("expected tuple length 2 at %d, got %d", i, len(tuple))
		}
		k, keyErr := tuple[0].AsString()
		if keyErr != nil {
			t.Fatalf("expected string key at %d: %v", i, keyErr)
		}
		return k
	}

	if got := getKey(0); got != "a" {
		t.Fatalf("expected first key to be %q, got %q", "a", got)
	}
	if got := getKey(1); got != "b" {
		t.Fatalf("expected second key to be %q, got %q", "b", got)
	}
	if got := getKey(2); got != "c" {
		t.Fatalf("expected third key to be %q, got %q", "c", got)
	}
}

// TestScriptWithGlobals verifies that globals set before script execution are available in the script.
func TestScriptWithGlobals(t *testing.T) {
	f := New()
	defer f.Close()

	f.SetGlobal("base", 10)
	f.SetGlobal("multiplier", 5)

	script := "(set result (* base multiplier))"
	if err := f.DoString(script); err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	result := f.MustGetInt("result")
	if result != 50 {
		t.Fatalf("Expected result = 50, got %d", result)
	}
}

// TestScriptWithConditional verifies that conditional logic works in config scripts.
func TestScriptWithConditional(t *testing.T) {
	f := New()
	defer f.Close()

	f.SetGlobal("env", "prod")

	script := `(if (= env "prod")
		(set port 443)
		(set port 8080))`
	if err := f.DoString(script); err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	port := f.MustGetInt("port")
	if port != 443 {
		t.Fatalf("Expected port = 443, got %d", port)
	}
}

// TestMultipleStatements verifies that multiple set statements can be executed.
func TestMultipleStatements(t *testing.T) {
	f := New()
	defer f.Close()

	script := `(let ()
		(set host "localhost")
		(set port 3210)
		(set enabled #t))`
	if err := f.DoString(script); err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	host := f.MustGetString("host")
	if host != "localhost" {
		t.Fatalf("Expected host = 'localhost', got %s", host)
	}

	port := f.MustGetInt("port")
	if port != 3210 {
		t.Fatalf("Expected port = 3210, got %d", port)
	}

	enabled := f.MustGetBool("enabled")
	if !enabled {
		t.Fatalf("Expected enabled = true, got false")
	}
}

// TestSetGlobalMapOfLists verifies that a map[string][]string is correctly set and retrieved.
func TestSetGlobalMapOfLists(t *testing.T) {
	f := New()
	defer f.Close()

	m := map[string][]string{
		"work": {"work", "secret"},
		"home": {"personal"},
		"blog": {"blog", "public", "writing"},
	}
	f.SetGlobalMapOfLists("DirectoryTags", m)

	result := f.MustGetMapOfLists("DirectoryTags")
	if len(result) != len(m) {
		t.Fatalf("Expected map length %d, got %d", len(m), len(result))
	}

	for k, expected := range m {
		got, ok := result[k]
		if !ok {
			t.Fatalf("Expected key %q not found in result", k)
		}
		if len(got) != len(expected) {
			t.Fatalf("Expected %d values for key %q, got %d", len(expected), k, len(got))
		}
		for i, v := range expected {
			if got[i] != v {
				t.Fatalf("Expected result[%q][%d] = %q, got %q", k, i, v, got[i])
			}
		}
	}
}

func TestSetGlobalMapOfListsDeterministicOrder(t *testing.T) {
	f := New()
	defer f.Close()

	f.SetGlobalMapOfLists("DirectoryTags", map[string][]string{
		"b": {"b"},
		"a": {"a"},
		"c": {"c"},
	})

	v, ok := f.globals["DirectoryTags"]
	if !ok {
		t.Fatalf("expected global %q", "DirectoryTags")
	}
	list, err := v.AsList()
	if err != nil {
		t.Fatalf("expected list global: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 items, got %d", len(list))
	}

	getKey := func(i int) string {
		pair, pairErr := list[i].AsList()
		if pairErr != nil {
			t.Fatalf("expected list pair at %d: %v", i, pairErr)
		}
		if len(pair) != 2 {
			t.Fatalf("expected pair length 2 at %d, got %d", i, len(pair))
		}
		k, keyErr := pair[0].AsString()
		if keyErr != nil {
			t.Fatalf("expected string key at %d: %v", i, keyErr)
		}
		return k
	}

	if got := getKey(0); got != "a" {
		t.Fatalf("expected first key to be %q, got %q", "a", got)
	}
	if got := getKey(1); got != "b" {
		t.Fatalf("expected second key to be %q, got %q", "b", got)
	}
	if got := getKey(2); got != "c" {
		t.Fatalf("expected third key to be %q, got %q", "c", got)
	}
}

// TestHasFunction verifies the HasFunction method.
func TestHasFunction(t *testing.T) {
	f := New()
	defer f.Close()

	// Initially no functions
	if f.HasFunction("my-func") {
		t.Fatal("Expected HasFunction to return false for undefined function")
	}

	// Define a function via script
	script := `(def my-func (fn (x) (* x 2)))`
	if err := f.DoString(script); err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	// Now it should exist
	if !f.HasFunction("my-func") {
		t.Fatal("Expected HasFunction to return true for defined function")
	}

	// Non-function global should return false
	f.SetGlobal("not-a-func", "hello")
	if f.HasFunction("not-a-func") {
		t.Fatal("Expected HasFunction to return false for non-function global")
	}
}

// TestCallFunction verifies calling a Filo-defined function from Go.
func TestCallFunction(t *testing.T) {
	f := New()
	defer f.Close()

	// Define a function
	script := `(def double (fn (x) (* x 2)))`
	if err := f.DoString(script); err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	// Call it
	result, err := f.CallFunction("double", 21)
	if err != nil {
		t.Fatalf("CallFunction error: %v", err)
	}

	n, err := result.AsNumber()
	if err != nil {
		t.Fatalf("Expected number result: %v", err)
	}
	if n != 42 {
		t.Fatalf("Expected 42, got %v", n)
	}
}

// TestCallFunctionWithStrings verifies calling a function with string arguments.
func TestCallFunctionWithStrings(t *testing.T) {
	f := New()
	defer f.Close()
	RegisterStringBuiltins(f.eng)

	// Define a greeting function
	script := `(def greet (fn (name) (str-concat "Hello, " name "!")))`
	if err := f.DoString(script); err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	// Call it
	result, err := f.CallFunction("greet", "World")
	if err != nil {
		t.Fatalf("CallFunction error: %v", err)
	}

	s, err := result.AsString()
	if err != nil {
		t.Fatalf("Expected string result: %v", err)
	}
	if s != "Hello, World!" {
		t.Fatalf("Expected 'Hello, World!', got %q", s)
	}
}

// TestCallFunctionString verifies the convenience wrapper.
func TestCallFunctionString(t *testing.T) {
	f := New()
	defer f.Close()
	RegisterStringBuiltins(f.eng)

	// Test with non-existent function (should return fallback)
	result := f.CallFunctionString("not-exists", "fallback", "arg")
	if result != "fallback" {
		t.Fatalf("Expected fallback, got %q", result)
	}

	// Define a function
	script := `(def process (fn (text) (str-upper text)))`
	if err := f.DoString(script); err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	// Call it
	result = f.CallFunctionString("process", "fallback", "hello")
	if result != "HELLO" {
		t.Fatalf("Expected 'HELLO', got %q", result)
	}
}

// TestCallFunctionNotFound verifies error handling for missing functions.
func TestCallFunctionNotFound(t *testing.T) {
	f := New()
	defer f.Close()

	_, err := f.CallFunction("not-exists", 1, 2, 3)
	if err == nil {
		t.Fatal("Expected error for non-existent function")
	}
}
