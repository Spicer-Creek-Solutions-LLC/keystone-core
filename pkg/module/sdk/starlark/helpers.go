// Package starlark provides Starlark SDK helpers for module development
// including type conversion, templates, and testing utilities.
package starlark

import (
	"fmt"

	"go.starlark.net/starlark"
)

// ConvertToGo converts a Starlark value to a Go value
func ConvertToGo(val starlark.Value) (interface{}, error) {
	switch v := val.(type) {
	case starlark.NoneType:
		return nil, nil
	case starlark.Bool:
		return bool(v), nil
	case starlark.Int:
		i, ok := v.Int64()
		if !ok {
			return nil, fmt.Errorf("integer too large")
		}
		return i, nil
	case starlark.Float:
		return float64(v), nil
	case starlark.String:
		return string(v), nil
	case *starlark.List:
		list := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := ConvertToGo(v.Index(i))
			if err != nil {
				return nil, err
			}
			list[i] = item
		}
		return list, nil
	case *starlark.Dict:
		dict := make(map[string]interface{})
		for _, item := range v.Items() {
			key, ok := item[0].(starlark.String)
			if !ok {
				return nil, fmt.Errorf("dictionary keys must be strings")
			}
			val, err := ConvertToGo(item[1])
			if err != nil {
				return nil, err
			}
			dict[string(key)] = val
		}
		return dict, nil
	case *starlark.Tuple:
		tuple := make([]interface{}, v.Len())
		for i := 0; i < v.Len(); i++ {
			item, err := ConvertToGo(v.Index(i))
			if err != nil {
				return nil, err
			}
			tuple[i] = item
		}
		return tuple, nil
	default:
		return nil, fmt.Errorf("unsupported type: %T", val)
	}
}

// ConvertFromGo converts a Go value to a Starlark value
func ConvertFromGo(val interface{}) (starlark.Value, error) {
	if val == nil {
		return starlark.None, nil
	}

	switch v := val.(type) {
	case bool:
		return starlark.Bool(v), nil
	case int:
		return starlark.MakeInt(v), nil
	case int32:
		return starlark.MakeInt(int(v)), nil
	case int64:
		return starlark.MakeInt64(v), nil
	case uint:
		return starlark.MakeUint(v), nil
	case uint32:
		return starlark.MakeUint(uint(v)), nil
	case uint64:
		return starlark.MakeUint64(v), nil
	case float32:
		return starlark.Float(v), nil
	case float64:
		return starlark.Float(v), nil
	case string:
		return starlark.String(v), nil
	case []interface{}:
		list := make([]starlark.Value, len(v))
		for i, item := range v {
			val, err := ConvertFromGo(item)
			if err != nil {
				return nil, err
			}
			list[i] = val
		}
		return starlark.NewList(list), nil
	case map[string]interface{}:
		dict := starlark.NewDict(len(v))
		for key, value := range v {
			k := starlark.String(key)
			val, err := ConvertFromGo(value)
			if err != nil {
				return nil, err
			}
			if err := dict.SetKey(k, val); err != nil {
				return nil, err
			}
		}
		return dict, nil
	default:
		return nil, fmt.Errorf("unsupported Go type: %T", val)
	}
}

// MakeBuiltin creates a Starlark builtin function from a Go function
func MakeBuiltin(name string, fn func(thread *starlark.Thread, fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error)) *starlark.Builtin {
	return starlark.NewBuiltin(name, fn)
}

// UnpackArgs is a helper to unpack positional arguments
func UnpackArgs(fnName string, args starlark.Tuple, minVal, maxVal int) ([]starlark.Value, error) {
	if len(args) < minVal {
		return nil, fmt.Errorf("%s: got %d arguments, want at least %d", fnName, len(args), minVal)
	}
	if maxVal >= 0 && len(args) > maxVal {
		return nil, fmt.Errorf("%s: got %d arguments, want at most %d", fnName, len(args), maxVal)
	}

	result := make([]starlark.Value, len(args))
	copy(result, args)
	return result, nil
}

// GetKwarg gets a keyword argument by name
func GetKwarg(kwargs []starlark.Tuple, name string, defaultVal starlark.Value) starlark.Value {
	for _, kw := range kwargs {
		if len(kw) != 2 {
			continue
		}
		key, ok := kw[0].(starlark.String)
		if !ok {
			continue
		}
		if string(key) == name {
			return kw[1]
		}
	}
	return defaultVal
}

// StringValue gets a string value or returns an error
func StringValue(val starlark.Value, name string) (string, error) {
	s, ok := val.(starlark.String)
	if !ok {
		return "", fmt.Errorf("%s must be a string, got %s", name, val.Type())
	}
	return string(s), nil
}

// IntValue gets an int value or returns an error
func IntValue(val starlark.Value, name string) (int64, error) {
	i, ok := val.(starlark.Int)
	if !ok {
		return 0, fmt.Errorf("%s must be an int, got %s", name, val.Type())
	}
	n, ok := i.Int64()
	if !ok {
		return 0, fmt.Errorf("%s is too large to fit in int64", name)
	}
	return n, nil
}

// BoolValue gets a bool value or returns an error
func BoolValue(val starlark.Value, name string) (bool, error) {
	b, ok := val.(starlark.Bool)
	if !ok {
		return false, fmt.Errorf("%s must be a bool, got %s", name, val.Type())
	}
	return bool(b), nil
}

// DictValue gets a dict value or returns an error
func DictValue(val starlark.Value, name string) (*starlark.Dict, error) {
	d, ok := val.(*starlark.Dict)
	if !ok {
		return nil, fmt.Errorf("%s must be a dict, got %s", name, val.Type())
	}
	return d, nil
}

// ListValue gets a list value or returns an error
func ListValue(val starlark.Value, name string) (*starlark.List, error) {
	l, ok := val.(*starlark.List)
	if !ok {
		return nil, fmt.Errorf("%s must be a list, got %s", name, val.Type())
	}
	return l, nil
}
