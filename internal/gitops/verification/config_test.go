// SPDX-License-Identifier: Apache-2.0

package verification

import (
	"errors"
	"testing"
)

func TestCfgString(t *testing.T) {
	t.Parallel()
	if _, err := cfgString(map[string]any{}, "url"); !errors.Is(err, ErrConfig) {
		t.Errorf("missing key err = %v, want ErrConfig", err)
	}
	if _, err := cfgString(map[string]any{"url": 1}, "url"); !errors.Is(err, ErrConfig) {
		t.Errorf("wrong type err = %v, want ErrConfig", err)
	}
	got, err := cfgString(map[string]any{"url": "x"}, "url")
	if err != nil || got != "x" {
		t.Errorf("cfgString = %q, %v", got, err)
	}
}

func TestCfgStringOptAndBoolOpt(t *testing.T) {
	t.Parallel()
	if v := cfgStringOpt(map[string]any{}, "m", "GET"); v != "GET" {
		t.Errorf("default not returned: %q", v)
	}
	if v := cfgStringOpt(map[string]any{"m": 1}, "m", "GET"); v != "GET" {
		t.Errorf("wrong-type falls back to default: %q", v)
	}
	if v := cfgBoolOpt(map[string]any{"tls": true}, "tls", false); !v {
		t.Error("cfgBoolOpt true not read")
	}
	if v := cfgBoolOpt(map[string]any{}, "tls", true); !v {
		t.Error("cfgBoolOpt default not returned")
	}
}

func TestCfgIntOpt(t *testing.T) {
	t.Parallel()
	cases := []struct {
		in      any
		want    int
		wantErr bool
	}{
		{nil, 9, false}, // absent → default
		{200, 200, false},
		{float64(204), 204, false},
		{int64(500), 500, false},
		{"200", 0, true},
	}
	for _, c := range cases {
		cfg := map[string]any{}
		if c.in != nil {
			cfg["expect_status"] = c.in
		}
		got, err := cfgIntOpt(cfg, "expect_status", 9)
		if c.wantErr {
			if !errors.Is(err, ErrConfig) {
				t.Errorf("in=%v err=%v, want ErrConfig", c.in, err)
			}
			continue
		}
		if err != nil || got != c.want {
			t.Errorf("in=%v → %d,%v want %d", c.in, got, err, c.want)
		}
	}
}

func TestCfgStringSliceAndMap(t *testing.T) {
	t.Parallel()
	sl, err := cfgStringSlice(map[string]any{"args": []any{"a", "b"}}, "args")
	if err != nil || len(sl) != 2 || sl[0] != "a" {
		t.Errorf("cfgStringSlice = %v, %v", sl, err)
	}
	if _, err := cfgStringSlice(map[string]any{"args": "nope"}, "args"); !errors.Is(err, ErrConfig) {
		t.Errorf("non-list err = %v, want ErrConfig", err)
	}
	if _, err := cfgStringSlice(map[string]any{"args": []any{1}}, "args"); !errors.Is(err, ErrConfig) {
		t.Errorf("non-string elem err = %v, want ErrConfig", err)
	}
	m, err := cfgStringMap(map[string]any{"h": map[string]any{"k": "v"}}, "h")
	if err != nil || m["k"] != "v" {
		t.Errorf("cfgStringMap = %v, %v", m, err)
	}
	if _, err := cfgStringMap(map[string]any{"h": map[string]any{"k": 1}}, "h"); !errors.Is(err, ErrConfig) {
		t.Errorf("non-string val err = %v, want ErrConfig", err)
	}
	if v, _ := cfgStringSlice(map[string]any{}, "absent"); v != nil {
		t.Errorf("absent slice = %v, want nil", v)
	}
}
