package verification

import "fmt"

// Typed accessors over a [Step.Config] map[string]any. They mirror
// the runbook steps' cfg* helpers so the YAML→config conventions are
// consistent across domains. Missing-required → ErrConfig; wrong-type
// → ErrConfig naming the key and the Go type seen.

func cfgString(cfg map[string]any, key string) (string, error) {
	v, ok := cfg[key]
	if !ok {
		return "", fmt.Errorf("%w: %q is required", ErrConfig, key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: %q must be a string, got %T", ErrConfig, key, v)
	}
	return s, nil
}

func cfgStringOpt(cfg map[string]any, key, def string) string {
	if v, ok := cfg[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return def
}

func cfgBoolOpt(cfg map[string]any, key string, def bool) bool {
	if v, ok := cfg[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return def
}

// cfgIntOpt accepts int and float64 (the JSON/YAML numeric decode
// default) so `expect_status: 200` works from either source.
func cfgIntOpt(cfg map[string]any, key string, def int) (int, error) {
	v, ok := cfg[key]
	if !ok {
		return def, nil
	}
	switch n := v.(type) {
	case int:
		return n, nil
	case int64:
		return int(n), nil
	case float64:
		return int(n), nil
	default:
		return 0, fmt.Errorf("%w: %q must be an integer, got %T", ErrConfig, key, v)
	}
}

func cfgStringSlice(cfg map[string]any, key string) ([]string, error) {
	v, ok := cfg[key]
	if !ok {
		return nil, nil
	}
	list, ok := v.([]any)
	if !ok {
		return nil, fmt.Errorf("%w: %q must be a list", ErrConfig, key)
	}
	out := make([]string, 0, len(list))
	for i, e := range list {
		s, ok := e.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %q[%d] must be a string, got %T", ErrConfig, key, i, e)
		}
		out = append(out, s)
	}
	return out, nil
}

func cfgStringMap(cfg map[string]any, key string) (map[string]string, error) {
	v, ok := cfg[key]
	if !ok {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: %q must be a map", ErrConfig, key)
	}
	out := make(map[string]string, len(m))
	for k, val := range m {
		s, ok := val.(string)
		if !ok {
			return nil, fmt.Errorf("%w: %q[%q] must be a string, got %T", ErrConfig, key, k, val)
		}
		out[k] = s
	}
	return out, nil
}
