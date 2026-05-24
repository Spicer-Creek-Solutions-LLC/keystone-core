// SPDX-License-Identifier: Apache-2.0

package rollback

import "fmt"

// Typed accessors over a [Config] map[string]any, mirroring the
// verification package's cfg* helpers so the YAML/JSON config
// conventions stay consistent across the gitops domain.

func cfgString(cfg Config, key string) (string, error) {
	v, ok := cfg[key]
	if !ok {
		return "", fmt.Errorf("%w: %q is required", ErrConfig, key)
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("%w: %q must be a string, got %T", ErrConfig, key, v)
	}
	if s == "" {
		return "", fmt.Errorf("%w: %q must not be empty", ErrConfig, key)
	}
	return s, nil
}

func cfgStringOpt(cfg Config, key, def string) string {
	if v, ok := cfg[key]; ok {
		if s, ok := v.(string); ok && s != "" {
			return s
		}
	}
	return def
}
