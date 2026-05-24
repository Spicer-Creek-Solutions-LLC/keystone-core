// SPDX-License-Identifier: Apache-2.0

package manifest

import (
	"testing"
	"time"
)

// The verbatim PROJECT-DETAILS §4.18 example manifest.
const exampleYAML = `name: vendor/pkg_apt
version: 1.2.3
type: starlark
entrypoint: main.star
description: APT package management
author: vendor
license: Apache-2.0
capabilities:
  fs.read:
    paths: [/etc/apt/**, /var/lib/apt/**]
    max_file_size: 10MB
  fs.write:
    paths: [/etc/apt/sources.list.d/**]
  exec:
    commands: [apt-get, dpkg]
    timeout: 60s
  log: {rate_limit: 100/s}
limits:
  timeout: 5m
  memory: 64MB
  cpu: 0.5
dependencies:
  vendor/pkg_common: ^1.0.0
`

func TestManifest_ExampleParsesAndValidates(t *testing.T) {
	m, err := UnmarshalManifest([]byte(exampleYAML))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if err := m.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	if m.Name != "vendor/pkg_apt" || m.Version != "1.2.3" || m.Type != TypeStarlark {
		t.Fatalf("head = %+v", m)
	}
	if got := m.Capabilities[CapFSRead].Paths; len(got) != 2 || got[0] != "/etc/apt/**" {
		t.Fatalf("fs.read paths = %v", got)
	}
	if m.Capabilities[CapExec].Timeout != "60s" {
		t.Fatalf("exec timeout = %q", m.Capabilities[CapExec].Timeout)
	}
	if m.Limits.CPU != 0.5 || m.Limits.Memory != "64MB" {
		t.Fatalf("limits = %+v", m.Limits)
	}
	if m.Dependencies["vendor/pkg_common"] != "^1.0.0" {
		t.Fatalf("deps = %v", m.Dependencies)
	}
}

func TestManifest_RoundTripStable(t *testing.T) {
	m, err := UnmarshalManifest([]byte(exampleYAML))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out1, err := MarshalManifest(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	m2, err := UnmarshalManifest(out1)
	if err != nil {
		t.Fatalf("re-unmarshal: %v", err)
	}
	out2, err := MarshalManifest(m2)
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	if string(out1) != string(out2) {
		t.Fatalf("round-trip not stable:\n--- 1 ---\n%s\n--- 2 ---\n%s", out1, out2)
	}
}

func validBase() *Manifest {
	return &Manifest{
		Name:       "acme/widget",
		Version:    "1.0.0",
		Type:       TypeStarlark,
		Entrypoint: "main.star",
	}
}

func TestManifest_ValidateErrors(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Manifest)
	}{
		{"bad name", func(m *Manifest) { m.Name = "noslash" }},
		{"upper name", func(m *Manifest) { m.Name = "Acme/Widget" }},
		{"bad version", func(m *Manifest) { m.Version = "notsemver" }},
		{"wasm rejected", func(m *Manifest) { m.Type = TypeWASM }},
		{"unknown type", func(m *Manifest) { m.Type = "lua" }},
		{"no entrypoint", func(m *Manifest) { m.Entrypoint = " " }},
		{"unknown capability", func(m *Manifest) {
			m.Capabilities = map[string]CapabilityConfig{"fs.delete": {}}
		}},
		{"bad max_file_size", func(m *Manifest) {
			m.Capabilities = map[string]CapabilityConfig{CapFSRead: {MaxFileSize: "10 wombats"}}
		}},
		{"bad rate_limit", func(m *Manifest) {
			m.Capabilities = map[string]CapabilityConfig{CapLog: {RateLimit: "lots"}}
		}},
		{"bad cap timeout", func(m *Manifest) {
			m.Capabilities = map[string]CapabilityConfig{CapExec: {Timeout: "soon"}}
		}},
		{"bad limits memory", func(m *Manifest) { m.Limits = Limits{Memory: "huge"} }},
		{"bad limits timeout", func(m *Manifest) { m.Limits = Limits{Timeout: "5 fortnights"} }},
		{"negative cpu", func(m *Manifest) { m.Limits = Limits{CPU: -1} }},
		{"bad dep name", func(m *Manifest) {
			m.Dependencies = map[string]string{"bare": "^1.0.0"}
		}},
		{"bad dep constraint", func(m *Manifest) {
			m.Dependencies = map[string]string{"acme/common": "not a constraint"}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := validBase()
			tc.mut(m)
			if err := m.Validate(); err == nil {
				t.Fatalf("%s: want validation error, got nil", tc.name)
			}
		})
	}
}

func TestManifest_ValidateOK(t *testing.T) {
	m := validBase()
	m.Capabilities = map[string]CapabilityConfig{
		CapFSWrite:     {Paths: []string{"/etc/**"}, DeniedPaths: []string{"/etc/shadow"}, MaxFileSize: "1MiB"},
		CapHTTPGet:     {Domains: []string{"example.com"}, MaxResponseSize: "512KB", Timeout: "10s"},
		CapSecretsRead: {SecretPaths: []string{"kv/data/app/*"}},
		CapKV:          {},
		CapLog:         {RateLimit: "50/m"},
	}
	m.Limits = Limits{Timeout: "2m", Memory: "128MB", CPU: 1.5}
	m.Dependencies = map[string]string{"acme/common": ">=1.0.0 <2.0.0"}
	if err := m.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestManifest_NilSafety(t *testing.T) {
	var m *Manifest
	if err := m.Validate(); err == nil {
		t.Fatal("nil Validate: want error")
	}
	if _, err := MarshalManifest(nil); err == nil {
		t.Fatal("nil Marshal: want error")
	}
}

func TestParseSize(t *testing.T) {
	ok := map[string]int64{
		"512":    512,
		"1KB":    1024,
		"10MB":   10 * 1 << 20,
		"64MiB":  64 * 1 << 20,
		"1.5GB":  int64(1.5 * float64(1<<30)),
		" 2 mb ": 2 << 20,
		"0":      0,
	}
	for in, want := range ok {
		got, err := ParseSize(in)
		if err != nil || got != want {
			t.Errorf("ParseSize(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "MB", "-5MB", "abc", "10 wombats", "-3"} {
		if _, err := ParseSize(bad); err == nil {
			t.Errorf("ParseSize(%q): want error", bad)
		}
	}
}

func TestParseRate(t *testing.T) {
	n, per, err := ParseRate("100/s")
	if err != nil || n != 100 || per != time.Second {
		t.Fatalf("100/s = %d,%v,%v", n, per, err)
	}
	if _, p, _ := ParseRate("5/m"); p != time.Minute {
		t.Errorf("5/m unit wrong")
	}
	if _, p, _ := ParseRate("9/hour"); p != time.Hour {
		t.Errorf("9/hour unit wrong")
	}
	for _, bad := range []string{"100", "100/", "/s", "x/s", "-1/s", "100/day"} {
		if _, _, err := ParseRate(bad); err == nil {
			t.Errorf("ParseRate(%q): want error", bad)
		}
	}
}
