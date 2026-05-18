package starlark

import (
	"fmt"
	"sort"

	star "go.starlark.net/starlark"
)

// toStarlark converts a JSON-ish Go value to a Starlark value.
// Supported: nil, bool, int/int8..int64/uint..., float64/float32,
// string, []any, map[string]any. Anything else errors (modules
// receive structured, JSON-shaped input only).
func toStarlark(v any) (star.Value, error) {
	switch x := v.(type) {
	case nil:
		return star.None, nil
	case bool:
		return star.Bool(x), nil
	case string:
		return star.String(x), nil
	case float64:
		return star.Float(x), nil
	case float32:
		return star.Float(float64(x)), nil
	case int:
		return star.MakeInt(x), nil
	case int8:
		return star.MakeInt(int(x)), nil
	case int16:
		return star.MakeInt(int(x)), nil
	case int32:
		return star.MakeInt(int(x)), nil
	case int64:
		return star.MakeInt64(x), nil
	case uint, uint8, uint16, uint32, uint64:
		return star.MakeInt64(int64(toUint64(x))), nil
	case []any:
		elems := make([]star.Value, 0, len(x))
		for _, e := range x {
			sv, err := toStarlark(e)
			if err != nil {
				return nil, err
			}
			elems = append(elems, sv)
		}
		return star.NewList(elems), nil
	case map[string]any:
		d := star.NewDict(len(x))
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys) // deterministic insertion order
		for _, k := range keys {
			sv, err := toStarlark(x[k])
			if err != nil {
				return nil, err
			}
			if err := d.SetKey(star.String(k), sv); err != nil {
				return nil, err
			}
		}
		return d, nil
	default:
		return nil, fmt.Errorf("unsupported input type %T", v)
	}
}

func toUint64(v any) uint64 {
	switch x := v.(type) {
	case uint:
		return uint64(x)
	case uint8:
		return uint64(x)
	case uint16:
		return uint64(x)
	case uint32:
		return uint64(x)
	case uint64:
		return x
	default:
		return 0
	}
}

// fromStarlark converts a Starlark value back to a JSON-ish Go
// value. None→nil, Bool→bool, Int→int64, Float→float64,
// String→string, List/Tuple→[]any, Dict→map[string]any (string
// keys). Other types fall back to their Starlark string form.
func fromStarlark(v star.Value) (any, error) {
	switch x := v.(type) {
	case star.NoneType:
		return nil, nil
	case star.Bool:
		return bool(x), nil
	case star.String:
		return string(x), nil
	case star.Int:
		n, _ := x.Int64()
		return n, nil
	case star.Float:
		return float64(x), nil
	case *star.List:
		return seqToSlice(x.Len(), x.Index)
	case star.Tuple:
		return seqToSlice(x.Len(), x.Index)
	case *star.Dict:
		out := make(map[string]any, x.Len())
		for _, item := range x.Items() {
			ks, ok := star.AsString(item[0])
			if !ok {
				return nil, fmt.Errorf("dict key %s is not a string", item[0].String())
			}
			ev, err := fromStarlark(item[1])
			if err != nil {
				return nil, err
			}
			out[ks] = ev
		}
		return out, nil
	default:
		return v.String(), nil
	}
}

func seqToSlice(n int, index func(int) star.Value) ([]any, error) {
	out := make([]any, 0, n)
	for i := 0; i < n; i++ {
		ev, err := fromStarlark(index(i))
		if err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, nil
}
