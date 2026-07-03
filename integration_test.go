package filo

import (
	"context"
	"strings"
	"testing"
)

// TestDoString executes a simple Filo script that assigns a global variable
// and then verifies that the variable was correctly set.
func TestDoString(t *testing.T) {
	f := New()
	defer f.Close()

	// Execute Filo script setting global variable 'x' to 42.
	err := f.DoString("(set x 42)")
	if err != nil {
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

func TestGetNumber(t *testing.T) {
	f := New()
	defer f.Close()

	err := f.DoString("(set speed 2.5)")
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}

	speed, err := f.GetNumber("speed")
	if err != nil {
		t.Fatalf("GetNumber error: %v", err)
	}
	if speed != 2.5 {
		t.Fatalf("expected speed = 2.5, got %v", speed)
	}

	i := f.MustGetInt("speed")
	if i != 2 {
		t.Fatalf("expected GetInt to truncate to 2, got %d", i)
	}
}

func TestMustGetNumber(t *testing.T) {
	f := New()
	defer f.Close()

	f.SetGlobal("scale", 1.5)
	scale := f.MustGetNumber("scale")
	if scale != 1.5 {
		t.Fatalf("expected scale = 1.5, got %v", scale)
	}

	err := f.DoString("(set whole 3)")
	if err != nil {
		t.Fatalf("DoString error: %v", err)
	}
	whole := f.MustGetNumber("whole")
	if whole != 3.0 {
		t.Fatalf("expected whole = 3.0, got %v", whole)
	}
}

func TestGetFloatCompatibilityAliases(t *testing.T) {
	f := New()
	defer f.Close()

	f.SetGlobal("scale", 1.5)

	scale, err := f.GetFloat("scale")
	if err != nil {
		t.Fatalf("GetFloat error: %v", err)
	}
	if scale != 1.5 {
		t.Fatalf("expected scale = 1.5, got %v", scale)
	}
	scale = f.MustGetFloat("scale")
	if scale != 1.5 {
		t.Fatalf("expected scale = 1.5, got %v", scale)
	}
}

func TestGetNumberTypeMismatch(t *testing.T) {
	f := New()
	defer f.Close()

	f.SetGlobal("name", "neko")
	_, err := f.GetNumber("name")
	if err == nil {
		t.Fatal("expected error converting string to number")
	}
	_, err = f.GetFloat("name")
	if err == nil || !strings.Contains(err.Error(), `converting "name" to float`) {
		t.Fatalf("GetFloat compatibility error = %v", err)
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

	got := getKey(0)
	if got != "a" {
		t.Fatalf("expected first key to be %q, got %q", "a", got)
	}
	got = getKey(1)
	if got != "b" {
		t.Fatalf("expected second key to be %q, got %q", "b", got)
	}
	got = getKey(2)
	if got != "c" {
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
	err := f.DoString(script)
	if err != nil {
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
	err := f.DoString(script)
	if err != nil {
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
	err := f.DoString(script)
	if err != nil {
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

	got := getKey(0)
	if got != "a" {
		t.Fatalf("expected first key to be %q, got %q", "a", got)
	}
	got = getKey(1)
	if got != "b" {
		t.Fatalf("expected second key to be %q, got %q", "b", got)
	}
	got = getKey(2)
	if got != "c" {
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
	err := f.DoString(script)
	if err != nil {
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
	err := f.DoString(script)
	if err != nil {
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
	// RegisterStringBuiltins(f.eng)
	err := f.eng.RegisterBuiltin("str-concat", func(_ context.Context, args []Value) (Value, error) {
		return VString(args[0].Str + args[1].Str + args[2].Str), nil
	})
	if err != nil {
		t.Fatalf("register str-concat: %v", err)
	}

	// Define a greeting function
	script := `(def greet (fn (name) (str-concat "Hello, " name "!")))`
	err = f.DoString(script)
	if err != nil {
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
	// RegisterStringBuiltins(f.eng)
	err := f.eng.RegisterBuiltin("str-upper", func(_ context.Context, args []Value) (Value, error) {
		return VString(strings.ToUpper(args[0].Str)), nil
	})
	if err != nil {
		t.Fatalf("register str-upper: %v", err)
	}

	// Test with non-existent function (should return fallback)
	result := f.CallFunctionString("not-exists", "fallback", "arg")
	if result != "fallback" {
		t.Fatalf("Expected fallback, got %q", result)
	}

	// Define a function
	script := `(def process (fn (text) (str-upper text)))`
	err = f.DoString(script)
	if err != nil {
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

// TestClose verifies Filo.Close() no-op method.
func TestClose(t *testing.T) {
	t.Parallel()

	f := New()
	f.Close() // Should not panic or error
}

// TestRegisterBuiltinMethod tests Filo.RegisterBuiltin().
func TestRegisterBuiltinMethod(t *testing.T) {
	t.Parallel()

	f := New()
	err := f.RegisterBuiltin("my-test", func(ctx context.Context, args []Value) (Value, error) {
		return VNum(42), nil
	})
	if err != nil {
		t.Fatalf("RegisterBuiltin failed: %v", err)
	}

	err = f.DoString("(set result (my-test))")
	if err != nil {
		t.Fatalf("DoString failed: %v", err)
	}

	got := f.MustGetInt("result")
	if got != 42 {
		t.Fatalf("want 42, got %d", got)
	}
}

// TestGetEngine tests Filo.GetEngine().
func TestGetEngine(t *testing.T) {
	t.Parallel()

	f := New()
	eng := f.GetEngine()
	if eng == nil {
		t.Fatal("GetEngine returned nil")
	}
}

// TestSetGlobalVariousTypes tests SetGlobal with various Go types.
func TestSetGlobalVariousTypes(t *testing.T) {
	t.Parallel()

	f := New()

	// int64
	f.SetGlobal("i64", int64(100))
	err := f.DoString("(set x i64)")
	if err != nil {
		t.Fatalf("set int64 global: %v", err)
	}
	if f.MustGetInt("x") != 100 {
		t.Fatal("int64 failed")
	}

	// float32
	f.SetGlobal("f32", float32(3.14))
	err = f.DoString("(set y f32)")
	if err != nil {
		t.Fatalf("set float32 global: %v", err)
	}
	got, err := f.GetNumber("y")
	if err != nil {
		t.Fatalf("GetNumber float32 global: %v", err)
	}
	want := float64(float32(3.14))
	if got != want {
		t.Fatalf("float32 global = %v, want %v", got, want)
	}
}

// TestGetNotFound tests Get* methods when variable not found.
func TestGetNotFound(t *testing.T) {
	t.Parallel()

	f := New()

	_, err := f.GetString("missing")
	if err == nil {
		t.Fatal("expected error for missing variable")
	}

	_, err = f.GetInt("missing")
	if err == nil {
		t.Fatal("expected error for missing variable")
	}

	_, err = f.GetNumber("missing")
	if err == nil {
		t.Fatal("expected error for missing variable")
	}

	_, err = f.GetFloat("missing")
	if err == nil {
		t.Fatal("expected error for missing variable")
	}

	_, err = f.GetBool("missing")
	if err == nil {
		t.Fatal("expected error for missing variable")
	}

	_, err = f.GetTable("missing")
	if err == nil {
		t.Fatal("expected error for missing variable")
	}

	_, err = f.GetMap("missing")
	if err == nil {
		t.Fatal("expected error for missing variable")
	}

	_, err = f.GetMapOfLists("missing")
	if err == nil {
		t.Fatal("expected error for missing variable")
	}
}

// TestCallFunctionNotAFunction tests error when calling non-function.
func TestCallFunctionNotAFunction(t *testing.T) {
	t.Parallel()

	f := New()
	f.SetGlobal("notfn", "string value")

	_, err := f.CallFunction("notfn")
	if err == nil {
		t.Fatal("expected error for non-function")
	}
}

// TestCallFunctionWithTypedArgs tests CallFunction with various argument types.
func TestCallFunctionWithTypedArgs(t *testing.T) {
	t.Parallel()

	f := New()
	err := f.DoString("(def add (fn (a b) (+ a b)))")
	if err != nil {
		t.Fatalf("define add: %v", err)
	}

	result, err := f.CallFunction("add", 10, int64(5))
	if err != nil {
		t.Fatalf("CallFunction failed: %v", err)
	}

	num, err := result.AsNumber()
	if err != nil {
		t.Fatalf("integer result is not a number: %v", err)
	}
	if num != 15 {
		t.Fatalf("want 15, got %v", num)
	}

	result, err = f.CallFunction("add", float32(1.5), float64(2.25))
	if err != nil {
		t.Fatalf("CallFunction with floats failed: %v", err)
	}

	num, err = result.AsNumber()
	if err != nil {
		t.Fatalf("float result is not a number: %v", err)
	}
	if num != 3.75 {
		t.Fatalf("want 3.75, got %v", num)
	}
}

// TestFiloParseScript verifies that ParseScript works correctly.
func TestFiloParseScript(t *testing.T) {
	t.Parallel()

	script, err := ParseScript("test", "(+ 1 2)")
	if err != nil {
		t.Fatalf("ParseScript failed: %v", err)
	}
	if script == nil {
		t.Fatal("ParseScript returned nil script")
	}
	if script.Name() != "test" {
		t.Fatalf("want name 'test', got %q", script.Name())
	}
}

// TestFiloParseScriptError verifies that ParseScript returns error on invalid source.
func TestFiloParseScriptError(t *testing.T) {
	t.Parallel()

	_, err := ParseScript("bad", "(+ 1")
	if err == nil {
		t.Fatal("expected parse error")
	}
}

// TestFiloExecuteScript verifies basic ExecuteScript functionality.
func TestFiloExecuteScript(t *testing.T) {
	t.Parallel()

	f := New()
	script, _ := ParseScript("calc", "(set result (+ x y))")

	f.SetGlobal("x", 10)
	f.SetGlobal("y", 20)

	err := f.Execute(script, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	result := f.MustGetInt("result")
	if result != 30 {
		t.Fatalf("want 30, got %d", result)
	}
}

// TestFiloExecuteScriptMultipleTimes verifies that the same script can be executed multiple times.
func TestFiloExecuteScriptMultipleTimes(t *testing.T) {
	t.Parallel()

	f := New()
	script, _ := ParseScript("sum", "(set total (+ a b))")

	testCases := []struct {
		a, b, expected int
	}{
		{1, 2, 3},
		{10, 20, 30},
		{100, 200, 300},
	}

	for _, tc := range testCases {
		f.SetGlobal("a", tc.a)
		f.SetGlobal("b", tc.b)

		err := f.Execute(script, nil)
		if err != nil {
			t.Fatalf("Execute failed: %v", err)
		}

		total := f.MustGetInt("total")
		if total != tc.expected {
			t.Fatalf("want %d, got %d", tc.expected, total)
		}
	}
}

// TestFiloExecuteScriptWithOverrideGlobals verifies globals merge behavior.
func TestFiloExecuteScriptWithOverrideGlobals(t *testing.T) {
	t.Parallel()

	f := New()
	script, _ := ParseScript("calc", "(set result (+ x y))")

	// Set instance globals
	f.SetGlobal("x", 10)
	f.SetGlobal("y", 20)

	// Execute with override for x (should override instance global)
	overrides := map[string]Value{
		"x": VNum(100),
	}
	err := f.Execute(script, overrides)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// Result should be 100 + 20 = 120 (override x, keep instance y)
	result := f.MustGetInt("result")
	if result != 120 {
		t.Fatalf("want 120, got %d", result)
	}

	// Instance global x should still be 10 (override was temporary)
	// Note: After execution, globals are updated with script results
	// The instance x is preserved because overrides don't modify f.globals directly
}

// TestFiloExecuteScriptGlobalsUpdate verifies that globals are updated after script execution.
func TestFiloExecuteScriptGlobalsUpdate(t *testing.T) {
	t.Parallel()

	f := New()
	script, _ := ParseScript("setter", "(set newvar 42)")

	err := f.Execute(script, nil)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	// New variable should be set in instance globals
	newvar := f.MustGetInt("newvar")
	if newvar != 42 {
		t.Fatalf("want 42, got %d", newvar)
	}
}
