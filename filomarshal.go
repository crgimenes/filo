// Package filo provides Marshal and Unmarshal functions for converting between
// Go values and Filo Value types.
//
// Marshal converts any Go value supported by Filo (bool, numbers, string, slices,
// structs, maps) into a Filo Value.
//
// Unmarshal converts a Filo Value into a Go value. The target must be a pointer
// to any Filo-supported type.
//
// Struct fields use the "filo" tag to specify the field name. If no tag is present,
// the lowercased field name is used. Fields with tag "-" are skipped.
//
// Example:
//
//	type Person struct {
//	    Name string `filo:"name"`
//	    Age  int    `filo:"age"`
//	}
//
//	p := Person{Name: "Alice", Age: 30}
//	val, err := filo.Marshal(p)
//	// val is a list of tuples: [("name", "Alice"), ("age", 30)]
//
//	var p2 Person
//	err = filo.Unmarshal(val, &p2)
//	// p2 is now {Name: "Alice", Age: 30}
package filo

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"
)

// Marshal converts a Go value to a Filo Value.
//
// Supported types:
//   - bool -> KBool
//   - int, int8, int16, int32, int64 -> KNumber
//   - uint, uint8, uint16, uint32, uint64 -> KNumber
//   - float32, float64 -> KNumber
//   - string -> KString
//   - []T -> KList (elements converted recursively)
//   - struct -> KList of (field_name, value) tuples
//   - map[K]V -> KList of (key, value) tuples (sorted by key for determinism)
//   - *T -> same as T (nil becomes empty tuple)
//
// Functions and channels are not supported and return an error.
// MarshalIndent converts a Go value to an indented Filo string.
// The prefix is prepended to each line (including the first),
// and indent is added per nesting level.
//
// This is similar to json.MarshalIndent but produces Filo syntax.
// MarshalOptions configures the marshaling process.
type MarshalOptions struct {
	// Prefix is prepended to each line.
	Prefix string
	// Indent is added per nesting level.
	Indent string
}

// MarshalIndent converts a Go value to an indented Filo string.
// The prefix is prepended to each line (including the first),
// and indent is added per nesting level.
//
// This is similar to json.MarshalIndent but produces Filo syntax.
func MarshalIndent(v any, prefix, indent string) (string, error) {
	return MarshalWithOptions(v, MarshalOptions{
		Prefix: prefix,
		Indent: indent,
	})
}

// Marshal converts a Go value to a compact Filo string.
// To modify the formatting, use MarshalIndent.
func Marshal(v any) (string, error) {
	return MarshalWithOptions(v, MarshalOptions{})
}

// MarshalWithOptions converts a Go value to a Filo string with options.
func MarshalWithOptions(v any, opts MarshalOptions) (string, error) {
	val, err := MarshalToValue(v)
	if err != nil {
		return "", err
	}

	// Default formatting path
	if opts.Indent != "" || opts.Prefix != "" {
		return FormatValueIndent(val, opts.Prefix, opts.Indent), nil
	}
	// Use compact formatting
	return formatValueCompact(val), nil
}

// MarshalToValue converts a Go value to a Filo Value.
//
// Supported types:
//   - bool -> KBool
//   - int, int8, int16, int32, int64 -> KNumber
//   - uint, uint8, uint16, uint32, uint64 -> KNumber
//   - float32, float64 -> KNumber
//   - string -> KString
//   - []T -> KList (elements converted recursively)
//   - struct -> KList of (field_name, value) tuples
//   - map[K]V -> KList of (key, value) tuples (sorted by key for determinism)
//   - *T -> same as T (nil becomes empty tuple)
//
// Functions and channels are not supported and return an error.
func MarshalToValue(v any) (Value, error) {
	if v == nil {
		return VTuple(nil), nil
	}
	return marshalValue(reflect.ValueOf(v))
}

func marshalValue(rv reflect.Value) (Value, error) {
	// Handle nil pointers
	if !rv.IsValid() {
		return VTuple(nil), nil
	}

	// Dereference pointers
	for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
		if rv.IsNil() {
			return VTuple(nil), nil
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Bool:
		return VBool(rv.Bool()), nil

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return VNum(float64(rv.Int())), nil

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return VNum(float64(rv.Uint())), nil

	case reflect.Float32, reflect.Float64:
		return VNum(rv.Float()), nil

	case reflect.String:
		return VString(rv.String()), nil

	case reflect.Slice, reflect.Array:
		return marshalSlice(rv)

	case reflect.Map:
		return marshalMap(rv)

	case reflect.Struct:
		return marshalStruct(rv)

	default:
		return Value{}, fmt.Errorf("marshal: unsupported type %s", rv.Type())
	}
}

func marshalSlice(rv reflect.Value) (Value, error) {
	if rv.IsNil() {
		return VList(nil), nil
	}
	vals := make([]Value, rv.Len())
	for i := 0; i < rv.Len(); i++ {
		v, err := marshalValue(rv.Index(i))
		if err != nil {
			return Value{}, fmt.Errorf("marshal: slice index %d: %w", i, err)
		}
		vals[i] = v
	}
	return VList(vals), nil
}

func marshalMap(rv reflect.Value) (Value, error) {
	if rv.IsNil() {
		return VList(nil), nil
	}

	// Collect and sort keys for deterministic output
	keys := rv.MapKeys()
	sortedKeys := make([]string, 0, len(keys))
	keyMap := make(map[string]reflect.Value, len(keys))

	for _, k := range keys {
		// Convert key to string representation
		ks, err := keyToString(k)
		if err != nil {
			return Value{}, fmt.Errorf("marshal: map key: %w", err)
		}
		sortedKeys = append(sortedKeys, ks)
		keyMap[ks] = k
	}
	sort.Strings(sortedKeys)

	pairs := make([]Value, 0, len(sortedKeys))
	for _, ks := range sortedKeys {
		k := keyMap[ks]
		v := rv.MapIndex(k)

		keyVal, err := marshalValue(k)
		if err != nil {
			return Value{}, fmt.Errorf("marshal: map key: %w", err)
		}
		valVal, err := marshalValue(v)
		if err != nil {
			return Value{}, fmt.Errorf("marshal: map value for key %q: %w", ks, err)
		}
		pairs = append(pairs, VTuple([]Value{keyVal, valVal}))
	}
	return VList(pairs), nil
}

func keyToString(k reflect.Value) (string, error) {
	switch k.Kind() {
	case reflect.String:
		return k.String(), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", k.Int()), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprintf("%d", k.Uint()), nil
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%g", k.Float()), nil
	case reflect.Bool:
		return fmt.Sprintf("%t", k.Bool()), nil
	default:
		return "", fmt.Errorf("unsupported map key type %s", k.Type())
	}
}

func marshalStruct(rv reflect.Value) (Value, error) {
	rt := rv.Type()
	pairs := make([]Value, 0, rt.NumField())

	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)

		// Skip unexported fields
		if !field.IsExported() {
			continue
		}

		// Get field name from tag or use lowercased name
		name := field.Tag.Get("filo")
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(field.Name)
		}

		val, err := marshalValue(rv.Field(i))
		if err != nil {
			return Value{}, fmt.Errorf("marshal: field %s: %w", field.Name, err)
		}
		pairs = append(pairs, VTuple([]Value{VString(name), val}))
	}
	return VList(pairs), nil
}

// Unmarshal converts a Filo string to a Go value.
// It parses the string, evaluates it, and unmarshals the result into target.
func Unmarshal(data string, target any) error {
	// Evaluate to get Value
	// Use a fresh engine instance
	eng := NewEngine()
	// We need 'tuple' support which is now in evaluator special forms or alias.

	val, _, err := eng.RunScript(context.Background(), data, nil, EvalConfig{})
	if err != nil {
		return err
	}

	return UnmarshalFromValue(val, target)
}

// UnmarshalFromValue converts a Filo Value to a Go value.
// The target must be a non-nil pointer to any Filo-supported type.
//
// Type mapping:
//   - KBool -> bool
//   - KNumber -> int, int64, float64, etc.
//   - KString -> string
//   - KList -> []T or struct (if list of tuples with string keys)
//   - KTuple (empty) -> nil pointer
//
// For structs, the list must contain tuples of (string_key, value).
// Field matching uses the "filo" tag or lowercased field name.
func UnmarshalFromValue(val Value, target any) error {
	rv := reflect.ValueOf(target)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		return errors.New("unmarshal: target must be a non-nil pointer")
	}
	return unmarshalValue(val, rv.Elem())
}

func unmarshalValue(val Value, rv reflect.Value) error {
	// Handle nil (empty tuple)
	if val.Kind == KTuple && len(val.Tup) == 0 {
		if rv.Kind() == reflect.Pointer {
			rv.SetZero()
		}
		return nil
	}

	// Dereference and allocate pointers
	for rv.Kind() == reflect.Pointer {
		if rv.IsNil() {
			rv.Set(reflect.New(rv.Type().Elem()))
		}
		rv = rv.Elem()
	}

	switch rv.Kind() {
	case reflect.Bool:
		b, err := val.AsBool()
		if err != nil {
			return fmt.Errorf("unmarshal bool: %w", err)
		}
		rv.SetBool(b)

	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		n, err := val.AsNumber()
		if err != nil {
			return fmt.Errorf("unmarshal int: %w", err)
		}
		rv.SetInt(int64(n))

	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		n, err := val.AsNumber()
		if err != nil {
			return fmt.Errorf("unmarshal uint: %w", err)
		}
		rv.SetUint(uint64(n))

	case reflect.Float32, reflect.Float64:
		n, err := val.AsNumber()
		if err != nil {
			return fmt.Errorf("unmarshal float: %w", err)
		}
		rv.SetFloat(n)

	case reflect.String:
		s, err := val.AsString()
		if err != nil {
			return fmt.Errorf("unmarshal string: %w", err)
		}
		rv.SetString(s)

	case reflect.Slice:
		return unmarshalSlice(val, rv)

	case reflect.Map:
		return unmarshalMap(val, rv)

	case reflect.Struct:
		return unmarshalStruct(val, rv)

	case reflect.Interface:
		// Convert Filo Value to native Go value and set it
		goVal, err := valueToGo(val)
		if err != nil {
			return err
		}
		rv.Set(reflect.ValueOf(goVal))

	default:
		return fmt.Errorf("unmarshal: unsupported target type %s", rv.Type())
	}
	return nil
}

func unmarshalSlice(val Value, rv reflect.Value) error {
	list, err := val.AsList()
	if err != nil {
		return fmt.Errorf("unmarshal slice: %w", err)
	}

	slice := reflect.MakeSlice(rv.Type(), len(list), len(list))
	for i, item := range list {
		if err := unmarshalValue(item, slice.Index(i)); err != nil {
			return fmt.Errorf("unmarshal slice index %d: %w", i, err)
		}
	}
	rv.Set(slice)
	return nil
}

func unmarshalMap(val Value, rv reflect.Value) error {
	list, err := val.AsList()
	if err != nil {
		return fmt.Errorf("unmarshal map: %w", err)
	}

	m := reflect.MakeMap(rv.Type())
	keyType := rv.Type().Key()
	valType := rv.Type().Elem()

	for i, item := range list {
		tup, tupErr := item.AsTuple()
		if tupErr != nil {
			// Try list as fallback
			tup2, listErr := item.AsList()
			if listErr != nil {
				return fmt.Errorf("unmarshal map item %d: expected tuple or list of 2 elements", i)
			}
			if len(tup2) != 2 {
				return fmt.Errorf("unmarshal map item %d: expected 2 elements, got %d", i, len(tup2))
			}
			tup = tup2
		}
		if len(tup) != 2 {
			return fmt.Errorf("unmarshal map item %d: expected 2 elements, got %d", i, len(tup))
		}

		keyRv := reflect.New(keyType).Elem()
		if err := unmarshalValue(tup[0], keyRv); err != nil {
			return fmt.Errorf("unmarshal map key %d: %w", i, err)
		}

		valRv := reflect.New(valType).Elem()
		if err := unmarshalValue(tup[1], valRv); err != nil {
			return fmt.Errorf("unmarshal map value %d: %w", i, err)
		}

		m.SetMapIndex(keyRv, valRv)
	}
	rv.Set(m)
	return nil
}

// valueToGo converts a Filo Value to a native Go value.
// This is used when the target type is interface{}.
func valueToGo(val Value) (any, error) {
	switch val.Kind {
	case KBool:
		return val.Bool, nil
	case KNumber:
		return val.Num, nil
	case KString:
		return val.Str, nil
	case KList:
		// Check if it looks like a map (list of 2-element tuples/lists with string keys)
		if isMapLike(val.List) {
			return listToMap(val.List)
		}
		// Otherwise, convert to []any
		result := make([]any, len(val.List))
		for i, item := range val.List {
			v, err := valueToGo(item)
			if err != nil {
				return nil, fmt.Errorf("list item %d: %w", i, err)
			}
			result[i] = v
		}
		return result, nil
	case KTuple:
		if len(val.Tup) == 0 {
			return nil, nil
		}
		// Convert tuple to []any
		result := make([]any, len(val.Tup))
		for i, item := range val.Tup {
			v, err := valueToGo(item)
			if err != nil {
				return nil, fmt.Errorf("tuple item %d: %w", i, err)
			}
			result[i] = v
		}
		return result, nil
	default:
		return nil, fmt.Errorf("valueToGo: unsupported value kind %v", val.Kind)
	}
}

// isMapLike checks if a list looks like a map representation (list of 2-element tuples/lists with string keys)
func isMapLike(list []Value) bool {
	for _, item := range list {
		var elements []Value
		switch item.Kind {
		case KTuple:
			elements = item.Tup
		case KList:
			elements = item.List
		default:
			return false
		}
		if len(elements) != 2 {
			return false
		}
		if elements[0].Kind != KString {
			return false
		}
	}
	return len(list) > 0
}

// listToMap converts a list of 2-element tuples/lists with string keys to a map[string]any
func listToMap(list []Value) (map[string]any, error) {
	result := make(map[string]any, len(list))
	for i, item := range list {
		var elements []Value
		switch item.Kind {
		case KTuple:
			elements = item.Tup
		case KList:
			elements = item.List
		default:
			return nil, fmt.Errorf("map item %d: expected tuple or list", i)
		}
		if len(elements) != 2 {
			return nil, fmt.Errorf("map item %d: expected 2 elements", i)
		}
		key, err := elements[0].AsString()
		if err != nil {
			return nil, fmt.Errorf("map item %d key: %w", i, err)
		}
		val, err := valueToGo(elements[1])
		if err != nil {
			return nil, fmt.Errorf("map item %d value: %w", i, err)
		}
		result[key] = val
	}
	return result, nil
}

func unmarshalStruct(val Value, rv reflect.Value) error {
	list, err := val.AsList()
	if err != nil {
		return fmt.Errorf("unmarshal struct: expected list of tuples: %w", err)
	}

	// Build field index by tag/name
	rt := rv.Type()
	fieldMap := make(map[string]int, rt.NumField())
	for i := 0; i < rt.NumField(); i++ {
		field := rt.Field(i)
		if !field.IsExported() {
			continue
		}
		name := field.Tag.Get("filo")
		if name == "-" {
			continue
		}
		if name == "" {
			name = strings.ToLower(field.Name)
		}
		fieldMap[name] = i
	}

	// Process tuples
	for i, item := range list {
		var tup []Value
		switch item.Kind {
		case KTuple:
			tup = item.Tup
		case KList:
			tup = item.List
		default:
			return fmt.Errorf("unmarshal struct item %d: expected tuple or list", i)
		}

		if len(tup) != 2 {
			return fmt.Errorf("unmarshal struct item %d: expected 2 elements, got %d", i, len(tup))
		}

		key, keyErr := tup[0].AsString()
		if keyErr != nil {
			return fmt.Errorf("unmarshal struct item %d: key must be string: %w", i, keyErr)
		}

		fieldIdx, ok := fieldMap[key]
		if !ok {
			// Skip unknown fields
			continue
		}

		if err := unmarshalValue(tup[1], rv.Field(fieldIdx)); err != nil {
			return fmt.Errorf("unmarshal struct field %s: %w", key, err)
		}
	}
	return nil
}
