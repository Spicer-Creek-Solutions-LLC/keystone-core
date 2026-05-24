// SPDX-License-Identifier: Apache-2.0

package blueprint

import (
	"errors"
	"strings"
	"testing"
)

func validManifest() *Manifest {
	return &Manifest{
		Metadata:    Metadata{Name: "demo", Version: "1.2.3"},
		Entrypoints: Entrypoints{Default: "demo.apply"},
	}
}

func TestValidate_OK(t *testing.T) {
	m := validManifest()
	m.Compatibility.MinKeystoneVersion = "0.9.0"
	m.Parameters = map[string]ParamSpec{
		"db": {Type: TypeString, Sensitive: true, Source: SourceSecret},
		"n":  {Type: TypeInteger, Min: f64(1), Max: f64(5)},
	}
	m.Dependencies = Dependencies{Requires: []string{"base"}, RequiresBefore: []string{"net"}}
	m.Hooks = Hooks{PreApply: []string{"warm"}}
	if err := m.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func f64(v float64) *float64 { return &v }

func TestValidate_Errors(t *testing.T) {
	tests := []struct {
		name string
		mut  func(*Manifest)
		want string
	}{
		{"missing name", func(m *Manifest) { m.Metadata.Name = "" }, "metadata.name is required"},
		{"bad name", func(m *Manifest) { m.Metadata.Name = "Demo!" }, "must match"},
		{"missing version", func(m *Manifest) { m.Metadata.Version = "" }, "metadata.version is required"},
		{"bad semver", func(m *Manifest) { m.Metadata.Version = "not-semver" }, "not valid semver"},
		{"bad min ks version", func(m *Manifest) { m.Compatibility.MinKeystoneVersion = "abc" }, "min_keystone_version"},
		{"missing entrypoint", func(m *Manifest) { m.Entrypoints.Default = "" }, "entrypoints.default is required"},
		{"bad param type", func(m *Manifest) {
			m.Parameters = map[string]ParamSpec{"x": {Type: "blob"}}
		}, "is not one of"},
		{"bad source", func(m *Manifest) {
			m.Parameters = map[string]ParamSpec{"x": {Type: TypeString, Source: "vault"}}
		}, "must be empty or"},
		{"secret without sensitive", func(m *Manifest) {
			m.Parameters = map[string]ParamSpec{"x": {Type: TypeString, Source: SourceSecret}}
		}, "requires sensitive: true"},
		{"min gt max", func(m *Manifest) {
			m.Parameters = map[string]ParamSpec{"x": {Type: TypeInteger, Min: f64(9), Max: f64(2)}}
		}, "min (9) > max (2)"},
		{"bad regex pattern", func(m *Manifest) {
			m.Parameters = map[string]ParamSpec{"x": {Type: TypeString, Pattern: "("}}
		}, "schema does not compile"},
		{"self dependency", func(m *Manifest) {
			m.Dependencies.Requires = []string{"demo"}
		}, "depends on itself"},
		{"dup dependency", func(m *Manifest) {
			m.Dependencies.Requires = []string{"a", "a"}
		}, "listed more than once"},
		{"empty dep", func(m *Manifest) {
			m.Dependencies.RequiresBefore = []string{""}
		}, "contains an empty name"},
		{"bad dep name", func(m *Manifest) {
			m.Dependencies.Requires = []string{"Bad"}
		}, "must match"},
		{"empty hook", func(m *Manifest) {
			m.Hooks.PostApply = []string{""}
		}, "empty runbook name"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := validManifest()
			tc.mut(m)
			err := m.Validate()
			if !errors.Is(err, ErrInvalidManifest) {
				t.Fatalf("err=%v want ErrInvalidManifest", err)
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err=%q want substring %q", err, tc.want)
			}
		})
	}
}
