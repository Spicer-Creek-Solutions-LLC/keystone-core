// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"errors"
	"reflect"
	"testing"
)

func paramManifest() *Manifest {
	return &Manifest{
		Metadata:    Metadata{Name: "demo", Version: "1.0.0"},
		Entrypoints: Entrypoints{Default: "x"},
		Parameters: map[string]ParamSpec{
			"name":     {Type: TypeString, Required: true},
			"replicas": {Type: TypeInteger, Default: 2, Min: f64(1), Max: f64(5)},
			"ratio":    {Type: TypeNumber},
			"enabled":  {Type: TypeBoolean, Default: false},
			"tags":     {Type: TypeArray},
			"opts":     {Type: TypeObject},
			"tier":     {Type: TypeString, Enum: []any{"a", "b"}},
			"db":       {Type: TypeString, Sensitive: true, Source: SourceSecret},
		},
	}
}

func TestResolveParams_Success(t *testing.T) {
	m := paramManifest()
	got, err := m.ResolveParams(map[string]string{
		"name":     "web",
		"replicas": "3",
		"ratio":    "0.5",
		"enabled":  "true",
		"tags":     `["x","y"]`,
		"opts":     `{"k":1}`,
		"tier":     "b",
	})
	if err != nil {
		t.Fatalf("ResolveParams: %v", err)
	}
	if got.Values["replicas"] != int64(3) || got.Values["ratio"] != 0.5 || got.Values["enabled"] != true {
		t.Errorf("coercion wrong: %#v", got.Values)
	}
	if !reflect.DeepEqual(got.Secret, []string{"db"}) {
		t.Errorf("Secret=%v want [db]", got.Secret)
	}
	// Absent param with a default is populated.
	if _, ok := got.Values["enabled"]; !ok {
		t.Error("expected enabled present")
	}
}

func TestResolveParams_DefaultsAndRedaction(t *testing.T) {
	m := paramManifest()
	got, err := m.ResolveParams(map[string]string{"name": "web", "db": "supersecret"})
	if err != nil {
		t.Fatalf("ResolveParams: %v", err)
	}
	if got.Values["replicas"] != 2 {
		t.Errorf("default replicas=%v want 2", got.Values["replicas"])
	}
	red := got.Redacted(m)
	if red["db"] != "***" {
		t.Errorf("db not redacted: %v", red["db"])
	}
	if red["name"] != "web" {
		t.Errorf("name should not be redacted: %v", red["name"])
	}
}

func TestResolveParams_Errors(t *testing.T) {
	m := paramManifest()
	tests := []struct {
		name   string
		inputs map[string]string
		want   error
	}{
		{"unknown param", map[string]string{"name": "w", "ghost": "1"}, ErrUnknownParam},
		{"bad integer", map[string]string{"name": "w", "replicas": "ten"}, ErrParamCoercion},
		{"bad number", map[string]string{"name": "w", "ratio": "x"}, ErrParamCoercion},
		{"bad bool", map[string]string{"name": "w", "enabled": "yesno"}, ErrParamCoercion},
		{"bad json array", map[string]string{"name": "w", "tags": "[1,"}, ErrParamCoercion},
		{"missing required", map[string]string{"replicas": "2"}, ErrParamValidation},
		{"below minimum", map[string]string{"name": "w", "replicas": "0"}, ErrParamValidation},
		{"above maximum", map[string]string{"name": "w", "replicas": "9"}, ErrParamValidation},
		{"enum violation", map[string]string{"name": "w", "tier": "z"}, ErrParamValidation},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := m.ResolveParams(tc.inputs)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err=%v want %v", err, tc.want)
			}
		})
	}
}

func TestResolveParams_UnsupportedTypeCoercion(t *testing.T) {
	m := &Manifest{
		Metadata:    Metadata{Name: "demo", Version: "1.0.0"},
		Entrypoints: Entrypoints{Default: "x"},
		Parameters:  map[string]ParamSpec{"weird": {Type: "blob"}},
	}
	_, err := m.ResolveParams(map[string]string{"weird": "v"})
	if !errors.Is(err, ErrParamCoercion) {
		t.Fatalf("err=%v want ErrParamCoercion", err)
	}
}
