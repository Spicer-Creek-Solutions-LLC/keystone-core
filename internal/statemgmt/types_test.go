// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"testing"
	"time"
)

func TestDeclaration_ZeroValueIsUsable(t *testing.T) {
	t.Parallel()
	// A zero Declaration is meaningless to a module but must not
	// panic when inspected by the runner (which logs unresolved
	// declarations during validation failures).
	var d Declaration
	if d.ID != "" || d.Module != "" || d.State != "" || d.Name != "" {
		t.Errorf("zero Declaration has non-zero string field: %#v", d)
	}
	if d.Params != nil {
		t.Errorf("zero Declaration.Params = %v, want nil", d.Params)
	}
}

func TestDeclaration_ParamsRoundtrip(t *testing.T) {
	t.Parallel()
	d := Declaration{
		ID:     "files:/etc/hosts",
		Module: "file",
		State:  "present",
		Name:   "/etc/hosts",
		Params: map[string]any{"mode": "0644", "owner": "root"},
	}
	if d.Params["mode"] != "0644" {
		t.Errorf("Params[mode] = %v, want 0644", d.Params["mode"])
	}
	if d.Params["owner"] != "root" {
		t.Errorf("Params[owner] = %v, want root", d.Params["owner"])
	}
}

func TestModuleCheckResult_Fields(t *testing.T) {
	t.Parallel()
	r := ModuleCheckResult{Matches: false, Diff: "mode 0600 -> 0644"}
	if r.Matches {
		t.Error("Matches should be false")
	}
	if r.Diff == "" {
		t.Error("Diff should be populated")
	}
}

func TestStateResult_Fields(t *testing.T) {
	t.Parallel()
	r := StateResult{
		Success:  true,
		Changed:  true,
		Diff:     "mode 0600 -> 0644",
		Comment:  "chmod applied",
		Duration: 12 * time.Millisecond,
	}
	if !r.Success || !r.Changed {
		t.Errorf("Success/Changed = %v/%v, want true/true", r.Success, r.Changed)
	}
	if r.Duration <= 0 {
		t.Errorf("Duration = %v, want positive", r.Duration)
	}
}
