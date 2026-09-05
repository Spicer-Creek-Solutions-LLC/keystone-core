// SPDX-License-Identifier: Apache-2.0

package runbook

import (
	"strings"
	"testing"
)

func TestRenderString(t *testing.T) {
	rr := renderRoot{
		inputs: map[string]any{"agent": "a1"},
		steps:  map[string]any{"s1": map[string]any{"outputs": map[string]any{"pid": 42}, "status": "succeeded"}},
	}
	out, err := renderString(`agent={{ .inputs.agent }} pid={{ .steps.s1.outputs.pid }} st={{ .steps.s1.status }}`, rr)
	if err != nil {
		t.Fatal(err)
	}
	if out != "agent=a1 pid=42 st=succeeded" {
		t.Fatalf("out=%q", out)
	}
}

func TestRenderString_MissingLoud(t *testing.T) {
	rr := renderRoot{inputs: map[string]any{}, steps: map[string]any{}}
	_, err := renderString(`{{ .steps.nope.outputs.x }}`, rr)
	if err == nil || !strings.Contains(err.Error(), "render") {
		t.Fatalf("expected loud missing error, got %v", err)
	}
}

func TestRenderConfig_Nested(t *testing.T) {
	rr := renderRoot{inputs: map[string]any{"host": "db1"}, steps: map[string]any{}}
	cfg := map[string]any{
		"cmd":    "restart {{ .inputs.host }}",
		"count":  3,
		"nested": map[string]any{"path": "/var/{{ .inputs.host }}"},
		"list":   []any{"{{ .inputs.host }}", 7},
	}
	out, err := renderConfig(cfg, rr)
	if err != nil {
		t.Fatal(err)
	}
	if out["cmd"] != "restart db1" || out["count"] != 3 {
		t.Fatalf("scalar render wrong: %#v", out)
	}
	if out["nested"].(map[string]any)["path"] != "/var/db1" {
		t.Fatalf("nested render wrong: %#v", out["nested"])
	}
	if out["list"].([]any)[0] != "db1" || out["list"].([]any)[1] != 7 {
		t.Fatalf("list render wrong: %#v", out["list"])
	}
}

func TestRenderConfig_ErrorPropagates(t *testing.T) {
	rr := renderRoot{inputs: map[string]any{}, steps: map[string]any{}}
	_, err := renderConfig(map[string]any{"k": "{{ .inputs.missing }}"}, rr)
	if err == nil || !strings.Contains(err.Error(), "config.k") {
		t.Fatalf("expected config.k error, got %v", err)
	}
}

func TestTruthy(t *testing.T) {
	for _, s := range []string{"", "false", "0", "no", "off", " FALSE "} {
		if truthy(s) {
			t.Errorf("truthy(%q) = true, want false", s)
		}
	}
	for _, s := range []string{"true", "1", "yes", "x"} {
		if !truthy(s) {
			t.Errorf("truthy(%q) = false, want true", s)
		}
	}
}
