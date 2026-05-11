package stdlib

import (
	"errors"
	"testing"

	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

func TestRegisterAll_IntoFreshRegistry(t *testing.T) {
	t.Parallel()
	reg := statemgmt.NewRegistry()
	if err := RegisterAll(reg); err != nil {
		t.Fatalf("RegisterAll: %v", err)
	}
	for _, name := range ModuleNames() {
		if !reg.Has(name) {
			t.Errorf("RegisterAll did not register %q", name)
		}
	}
}

func TestRegisterAll_RejectsDuplicateRegistration(t *testing.T) {
	t.Parallel()
	reg := statemgmt.NewRegistry()
	if err := RegisterAll(reg); err != nil {
		t.Fatalf("first RegisterAll: %v", err)
	}
	err := RegisterAll(reg)
	if err == nil {
		t.Fatal("second RegisterAll should fail; registry already populated")
	}
	if !errors.Is(err, statemgmt.ErrDuplicateModule) {
		t.Errorf("err = %v, want wrapping ErrDuplicateModule", err)
	}
}

// (DefaultRegistry coverage is implicit: cmd/kscore-server calls
// RegisterAll(nil) at boot and the gRPC server tests in
// internal/controlplane exercise the populated DefaultRegistry. A
// dedicated test here would mutate package-global state and order-
// couple with other tests in the package; not worth the fragility.)

func TestModuleNames_StableOrder(t *testing.T) {
	t.Parallel()
	a := ModuleNames()
	b := ModuleNames()
	if len(a) != len(b) {
		t.Fatalf("len mismatch: %d vs %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Errorf("ordering not stable at %d: %q vs %q", i, a[i], b[i])
		}
	}
}

func TestModuleNames_IncludesFile(t *testing.T) {
	t.Parallel()
	found := false
	for _, name := range ModuleNames() {
		if name == "file" {
			found = true
		}
	}
	if !found {
		t.Error("file module missing from stdlib")
	}
}

func TestModuleNames_IncludesCmd(t *testing.T) {
	t.Parallel()
	found := false
	for _, name := range ModuleNames() {
		if name == "cmd" {
			found = true
		}
	}
	if !found {
		t.Error("cmd module missing from stdlib")
	}
}

func TestModuleNames_IncludesGroup(t *testing.T) {
	t.Parallel()
	found := false
	for _, name := range ModuleNames() {
		if name == "group" {
			found = true
		}
	}
	if !found {
		t.Error("group module missing from stdlib")
	}
}

func TestModuleNames_IncludesUser(t *testing.T) {
	t.Parallel()
	found := false
	for _, name := range ModuleNames() {
		if name == "user" {
			found = true
		}
	}
	if !found {
		t.Error("user module missing from stdlib")
	}
}

func TestModuleNames_IncludesPackage(t *testing.T) {
	t.Parallel()
	found := false
	for _, name := range ModuleNames() {
		if name == "package" {
			found = true
		}
	}
	if !found {
		t.Error("package module missing from stdlib")
	}
}
