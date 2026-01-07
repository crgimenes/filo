// Package filojson provides JSON marshal/unmarshal builtins for the Filo language.
//
// It enables converting between Filo values and JSON strings for debugging,
// logging, and interoperating with external systems.
//
// Mapping rules:
// - Filo number/bool/string -> JSON number/bool/string
// - Filo list -> JSON array (elements converted recursively)
// - Filo tuple -> JSON array (elements converted recursively)
//   - Special case: empty tuple encodes as JSON null
//
// - Filo list of 2-tuples (key, value), with key as string -> JSON object
// - Functions are not serializable and will return an error
//
// Unmarshal rules (JSON -> Filo):
// - number/bool/string -> corresponding Filo value
// - array -> Filo list
// - object -> Filo list of 2-tuples (key, value), sorted by key for determinism
// - null -> empty tuple (VTuple(nil)) for unambiguous round-trip
package filojson

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"

	"github.com/crgimenes/filo"
)

// RegisterJSONBuiltins adds json-marshal and json-unmarshal to a Filo engine.
//
// - (json-marshal value) -> string
// - (json-unmarshal string) -> value
// - (json-null) -> tuple() sentinel representing JSON null
func RegisterJSONBuiltins(eng *filo.Engine) {
	eng.MustRegisterBuiltin("json-marshal", builtinJSONMarshal)
	eng.MustRegisterBuiltin("json-unmarshal", builtinJSONUnmarshal)
	eng.MustRegisterBuiltin("json-null", builtinJSONNull)
}

func builtinJSONNull(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 0 {
		return filo.Value{}, fmt.Errorf("json-null expects no arguments")
	}
	return filo.VTuple(nil), nil
}

func builtinJSONMarshal(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("json-marshal expects 1 argument (value)")
	}
	goVal, err := toGoForMarshal(args[0])
	if err != nil {
		return filo.Value{}, fmt.Errorf("json-marshal: %w", err)
	}
	// JSON numbers cannot represent NaN/Inf; ensure we don't emit them.
	if err := validateNoNaNInf(goVal); err != nil {
		return filo.Value{}, fmt.Errorf("json-marshal: %w", err)
	}
	b, err := json.Marshal(goVal)
	if err != nil {
		return filo.Value{}, fmt.Errorf("json-marshal: %w", err)
	}
	return filo.VString(string(b)), nil
}

func builtinJSONUnmarshal(_ context.Context, args []filo.Value) (filo.Value, error) {
	if len(args) != 1 {
		return filo.Value{}, fmt.Errorf("json-unmarshal expects 1 argument (string)")
	}
	s, err := args[0].AsString()
	if err != nil {
		return filo.Value{}, fmt.Errorf("json-unmarshal: argument must be string: %w", err)
	}
	var v any
	if err := json.Unmarshal([]byte(s), &v); err != nil {
		return filo.Value{}, fmt.Errorf("json-unmarshal: %w", err)
	}
	fv, err := fromGoToFilo(v)
	if err != nil {
		return filo.Value{}, fmt.Errorf("json-unmarshal: %w", err)
	}
	return fv, nil
}

func toGoForMarshal(v filo.Value) (any, error) {
	switch v.Kind {
	case filo.KNumber:
		return v.Num, nil
	case filo.KBool:
		return v.Bool, nil
	case filo.KString:
		return v.Str, nil
	case filo.KList:
		// Detect object-like list: list of pairs (tuple or 2-element list)
		if isObjectLikeList(v.List) {
			m := make(map[string]any, len(v.List))
			for i, item := range v.List {
				var keyVal, valVal filo.Value
				switch item.Kind {
				case filo.KTuple:
					keyVal = item.Tup[0]
					valVal = item.Tup[1]
				case filo.KList:
					if len(item.List) != 2 {
						return nil, fmt.Errorf("object pair at %d must have 2 elements", i)
					}
					keyVal = item.List[0]
					valVal = item.List[1]
				default:
					return nil, fmt.Errorf("object pair at %d must be tuple or list", i)
				}
				key, err := keyVal.AsString()
				if err != nil {
					return nil, fmt.Errorf("object key at %d must be string: %w", i, err)
				}
				val, err := toGoForMarshal(valVal)
				if err != nil {
					return nil, err
				}
				m[key] = val
			}
			return m, nil
		}
		arr := make([]any, len(v.List))
		for i, el := range v.List {
			gv, err := toGoForMarshal(el)
			if err != nil {
				return nil, err
			}
			arr[i] = gv
		}
		return arr, nil
	case filo.KTuple:
		// Empty tuple encodes as JSON null.
		if len(v.Tup) == 0 {
			return nil, nil
		}
		arr := make([]any, len(v.Tup))
		for i, el := range v.Tup {
			gv, err := toGoForMarshal(el)
			if err != nil {
				return nil, err
			}
			arr[i] = gv
		}
		return arr, nil
	case filo.KFunc:
		return nil, errors.New("cannot marshal functions to JSON")
	default:
		return nil, fmt.Errorf("unsupported value kind: %v", v.Kind)
	}
}

func isObjectLikeList(list []filo.Value) bool {
	if len(list) == 0 {
		return false
	}
	for _, it := range list {
		switch it.Kind {
		case filo.KTuple:
			if len(it.Tup) != 2 || it.Tup[0].Kind != filo.KString {
				return false
			}
		case filo.KList:
			if len(it.List) != 2 || it.List[0].Kind != filo.KString {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func validateNoNaNInf(v any) error {
	switch x := v.(type) {
	case float64:
		if math.IsNaN(x) || math.IsInf(x, 0) {
			return fmt.Errorf("numbers cannot be NaN or Inf")
		}
		return nil
	case []any:
		for i := range x {
			if err := validateNoNaNInf(x[i]); err != nil {
				return err
			}
		}
		return nil
	case map[string]any:
		for k := range x {
			if err := validateNoNaNInf(x[k]); err != nil {
				return err
			}
		}
		return nil
	case nil, bool, string:
		return nil
	default:
		return nil
	}
}

func fromGoToFilo(v any) (filo.Value, error) {
	switch t := v.(type) {
	case nil:
		return filo.VTuple(nil), nil
	case bool:
		return filo.VBool(t), nil
	case float64:
		return filo.VNum(t), nil
	case string:
		return filo.VString(t), nil
	case []any:
		vals := make([]filo.Value, len(t))
		for i := range t {
			fv, err := fromGoToFilo(t[i])
			if err != nil {
				return filo.Value{}, err
			}
			vals[i] = fv
		}
		return filo.VList(vals), nil
	case map[string]any:
		// Emit as list of (key, value) tuples; sort keys for deterministic output
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		pairs := make([]filo.Value, 0, len(keys))
		for _, k := range keys {
			fv, err := fromGoToFilo(t[k])
			if err != nil {
				return filo.Value{}, err
			}
			pairs = append(pairs, filo.VTuple([]filo.Value{filo.VString(k), fv}))
		}
		return filo.VList(pairs), nil
	default:
		return filo.Value{}, fmt.Errorf("unsupported JSON value type: %T", v)
	}
}
