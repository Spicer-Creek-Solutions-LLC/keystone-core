// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// fakeModule satisfies Module with a configurable name + valid
// states; never registered globally so it cannot leak between tests.
type fakeModule struct {
	name        string
	validStates []string
}

func (m *fakeModule) Name() string          { return m.name }
func (m *fakeModule) ValidStates() []string { return m.validStates }
func (m *fakeModule) Check(context.Context, *Declaration) (*ModuleCheckResult, error) {
	return &ModuleCheckResult{Matches: true}, nil
}
func (m *fakeModule) Apply(context.Context, *Declaration) (*StateResult, error) {
	return &StateResult{Success: true}, nil
}
func (m *fakeModule) Test(context.Context, *Declaration) (bool, error) { return true, nil }

func fakeFactory(name string, validStates ...string) Factory {
	return func() Module { return &fakeModule{name: name, validStates: validStates} }
}

// validatableModule additionally implements ValidatableModule.
type validatableModule struct {
	fakeModule
	check func(*Declaration) error
}

func (m *validatableModule) Validate(d *Declaration) error { return m.check(d) }

func validatableFactory(name string, check func(*Declaration) error, validStates ...string) Factory {
	return func() Module {
		return &validatableModule{
			fakeModule: fakeModule{name: name, validStates: validStates},
			check:      check,
		}
	}
}

func newRegistryWith(t *testing.T, factories map[string]Factory) *Registry {
	t.Helper()
	r := NewRegistry()
	for name, f := range factories {
		if err := r.Register(name, f); err != nil {
			t.Fatalf("Register(%q): %v", name, err)
		}
	}
	return r
}

func mustValidateOK(t *testing.T, v *Validator, sf *StateFile) {
	t.Helper()
	if err := v.Validate(sf); err != nil {
		t.Fatalf("Validate: unexpected error: %v", err)
	}
}

func mustValidationErr(t *testing.T, v *Validator, sf *StateFile) *ValidationError {
	t.Helper()
	err := v.Validate(sf)
	if err == nil {
		t.Fatal("Validate: expected error, got nil")
	}
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("Validate: expected *ValidationError, got %T: %v", err, err)
	}
	return ve
}

func TestValidator_NilStateFile(t *testing.T) {
	t.Parallel()
	v := NewValidator(NewRegistry())
	if err := v.Validate(nil); err != nil {
		t.Errorf("Validate(nil) = %v, want nil", err)
	}
}

func TestValidator_EmptyStateFile(t *testing.T) {
	t.Parallel()
	v := NewValidator(NewRegistry())
	mustValidateOK(t, v, &StateFile{})
}

func TestValidator_SingleValidDeclaration(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"file": fakeFactory("file", "present", "absent"),
	})
	v := NewValidator(r)
	mustValidateOK(t, v, &StateFile{
		Declarations: []*Declaration{{
			ID:     "file:/etc/hosts",
			Module: "file",
			Name:   "/etc/hosts",
			State:  "present",
		}},
	})
}

func TestValidator_RequisiteResolves(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"package": fakeFactory("package", "installed"),
		"file":    fakeFactory("file", "present"),
	})
	v := NewValidator(r)
	sf := &StateFile{
		Declarations: []*Declaration{
			{ID: "package:nginx", Module: "package", Name: "nginx", State: "installed"},
			{
				ID:     "file:/etc/nginx.conf",
				Module: "file",
				Name:   "/etc/nginx.conf",
				State:  "present",
				Params: map[string]any{
					"require": []any{map[string]any{"package": "nginx"}},
					"watch":   []any{map[string]any{"package": "nginx"}},
				},
			},
		},
	}
	mustValidateOK(t, v, sf)
}

func TestValidator_EmptyFields(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"file": fakeFactory("file", "present"),
	})
	v := NewValidator(r)
	// Module empty, Name empty, State empty — and the ID
	// consequently does not match Module:Name.
	ve := mustValidationErr(t, v, &StateFile{
		Declarations: []*Declaration{{}},
	})
	wantFields := map[string]bool{"Module": false, "Name": false, "State": false}
	for _, iss := range ve.Issues {
		if _, ok := wantFields[iss.Field]; ok {
			wantFields[iss.Field] = true
		}
	}
	for f, hit := range wantFields {
		if !hit {
			t.Errorf("expected issue on field %q, none found in %v", f, ve.Issues)
		}
	}
}

func TestValidator_IDDoesNotMatchModuleName(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"file": fakeFactory("file", "present"),
	})
	v := NewValidator(r)
	ve := mustValidationErr(t, v, &StateFile{
		Declarations: []*Declaration{{
			ID:     "WRONG", // should be file:/etc/hosts
			Module: "file",
			Name:   "/etc/hosts",
			State:  "present",
		}},
	})
	if !hasFieldIssue(ve.Issues, "ID") {
		t.Errorf("expected ID issue, got %v", ve.Issues)
	}
}

func TestValidator_UnknownModule(t *testing.T) {
	t.Parallel()
	v := NewValidator(NewRegistry())
	ve := mustValidationErr(t, v, &StateFile{
		Declarations: []*Declaration{{
			ID:     "ghost:/x",
			Module: "ghost",
			Name:   "/x",
			State:  "present",
		}},
	})
	if !issueMessageContains(ve.Issues, "Module", `module "ghost" not registered`) {
		t.Errorf("missing unknown-module issue: %v", ve.Issues)
	}
}

func TestValidator_InvalidStateForModule(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"service": fakeFactory("service", "running", "stopped"),
	})
	v := NewValidator(r)
	ve := mustValidationErr(t, v, &StateFile{
		Declarations: []*Declaration{{
			ID:     "service:nginx",
			Module: "service",
			Name:   "nginx",
			State:  "latest",
		}},
	})
	if !issueMessageContains(ve.Issues, "State", "latest") {
		t.Errorf("expected invalid-state issue, got %v", ve.Issues)
	}
	if !issueMessageContains(ve.Issues, "State", "running, stopped") {
		t.Errorf("expected valid-states listed in message, got %v", ve.Issues)
	}
}

func TestValidator_DuplicateDeclarations(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"file": fakeFactory("file", "present"),
	})
	v := NewValidator(r)
	dup := &Declaration{ID: "file:/x", Module: "file", Name: "/x", State: "present"}
	ve := mustValidationErr(t, v, &StateFile{
		Declarations: []*Declaration{dup, dup},
	})
	if !hasFieldIssue(ve.Issues, "ID") {
		t.Errorf("expected duplicate-ID issue, got %v", ve.Issues)
	}
}

func TestValidator_RequisiteRefNotFound(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"file":    fakeFactory("file", "present"),
		"package": fakeFactory("package", "installed"),
	})
	v := NewValidator(r)
	ve := mustValidationErr(t, v, &StateFile{
		Declarations: []*Declaration{{
			ID:     "file:/etc/nginx.conf",
			Module: "file",
			Name:   "/etc/nginx.conf",
			State:  "present",
			Params: map[string]any{
				"require": []any{map[string]any{"package": "ghost"}},
			},
		}},
	})
	if !issueMessageContains(ve.Issues, "Params.require", `"package:ghost" not found`) {
		t.Errorf("expected dangling-requisite issue, got %v", ve.Issues)
	}
}

func TestValidator_AllEightRequisiteKeys(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"file": fakeFactory("file", "present"),
	})
	v := NewValidator(r)
	for _, key := range RequisiteKeys {
		t.Run(key, func(t *testing.T) {
			ve := mustValidationErr(t, v, &StateFile{
				Declarations: []*Declaration{{
					ID:     "file:/x",
					Module: "file",
					Name:   "/x",
					State:  "present",
					Params: map[string]any{
						key: []any{map[string]any{"file": "ghost"}},
					},
				}},
			})
			if !hasFieldIssue(ve.Issues, "Params."+key) {
				t.Errorf("expected issue on Params.%s, got %v", key, ve.Issues)
			}
		})
	}
}

func TestValidator_MalformedRequisiteShape(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"file": fakeFactory("file", "present"),
	})
	v := NewValidator(r)
	cases := map[string]any{
		"not a list":        "package: nginx",
		"entry not a map":   []any{"package:nginx"},
		"multi-key map":     []any{map[string]any{"package": "nginx", "file": "/x"}},
		"non-string value":  []any{map[string]any{"package": 42}},
		"empty module name": []any{map[string]any{"": "nginx"}},
		"empty name":        []any{map[string]any{"package": ""}},
	}
	for name, value := range cases {
		t.Run(name, func(t *testing.T) {
			ve := mustValidationErr(t, v, &StateFile{
				Declarations: []*Declaration{{
					ID:     "file:/x",
					Module: "file",
					Name:   "/x",
					State:  "present",
					Params: map[string]any{
						"require": value,
					},
				}},
			})
			if !hasFieldIssue(ve.Issues, "Params.require") {
				t.Errorf("expected Params.require issue for %q, got %v", name, ve.Issues)
			}
		})
	}
}

func TestValidator_AggregatesMultipleIssues(t *testing.T) {
	t.Parallel()
	v := NewValidator(NewRegistry()) // nothing registered → every decl will produce issues
	ve := mustValidationErr(t, v, &StateFile{
		Declarations: []*Declaration{
			{ID: "file:/a", Module: "file", Name: "/a", State: "present"},
			{ID: "file:/b", Module: "file", Name: "/b", State: "present"},
			{ID: "service:s", Module: "service", Name: "s", State: "running"},
		},
	})
	if len(ve.Issues) < 3 {
		t.Errorf("expected at least 3 aggregated issues, got %d: %v", len(ve.Issues), ve.Issues)
	}
	// Ensure error string formats cleanly.
	msg := ve.Error()
	if !strings.HasPrefix(msg, "statemgmt: validation failed") {
		t.Errorf("Error() = %q, want \"statemgmt: validation failed\" prefix", msg)
	}
	if !strings.Contains(msg, "• ") {
		t.Errorf("Error() = %q, want bullet-list formatting", msg)
	}
}

func TestValidator_OptInValidatableModule_Pass(t *testing.T) {
	t.Parallel()
	called := false
	r := newRegistryWith(t, map[string]Factory{
		"file": validatableFactory("file", func(d *Declaration) error {
			called = true
			return nil
		}, "present"),
	})
	v := NewValidator(r)
	mustValidateOK(t, v, &StateFile{
		Declarations: []*Declaration{{
			ID:     "file:/x",
			Module: "file",
			Name:   "/x",
			State:  "present",
		}},
	})
	if !called {
		t.Error("ValidatableModule.Validate was not called")
	}
}

func TestValidator_OptInValidatableModule_Fail(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"file": validatableFactory("file", func(d *Declaration) error {
			return fmt.Errorf("missing required param %q", "path")
		}, "present"),
	})
	v := NewValidator(r)
	ve := mustValidationErr(t, v, &StateFile{
		Declarations: []*Declaration{{
			ID:     "file:/x",
			Module: "file",
			Name:   "/x",
			State:  "present",
		}},
	})
	if !issueMessageContains(ve.Issues, "Params", `missing required param "path"`) {
		t.Errorf("expected opt-in validator issue surfaced, got %v", ve.Issues)
	}
}

func TestValidator_OptInSkippedWhenNotImplemented(t *testing.T) {
	t.Parallel()
	// Plain fakeModule does not implement ValidatableModule — no
	// per-module issue should fire even with intentionally weird
	// Params (still need a valid state).
	r := newRegistryWith(t, map[string]Factory{
		"file": fakeFactory("file", "present"),
	})
	v := NewValidator(r)
	mustValidateOK(t, v, &StateFile{
		Declarations: []*Declaration{{
			ID:     "file:/x",
			Module: "file",
			Name:   "/x",
			State:  "present",
			Params: map[string]any{"made_up": "value", "count": 7},
		}},
	})
}

func TestValidator_FallsBackToDefaultRegistry(t *testing.T) {
	// Not parallel: touches the global DefaultRegistry.
	name := fmt.Sprintf("validate-default-%d", testCounter.next())
	if err := RegisterModule(name, fakeFactory(name, "present")); err != nil {
		t.Fatalf("RegisterModule: %v", err)
	}
	t.Cleanup(func() {
		DefaultRegistry.mu.Lock()
		delete(DefaultRegistry.factories, name)
		DefaultRegistry.mu.Unlock()
	})
	v := NewValidator(nil) // nil → DefaultRegistry
	mustValidateOK(t, v, &StateFile{
		Declarations: []*Declaration{{
			ID:     name + ":/x",
			Module: name,
			Name:   "/x",
			State:  "present",
		}},
	})
}

func TestValidator_NilDeclarationsAreSkipped(t *testing.T) {
	t.Parallel()
	r := newRegistryWith(t, map[string]Factory{
		"file": fakeFactory("file", "present"),
	})
	v := NewValidator(r)
	// A nil entry shouldn't crash; only the real decl gets checked.
	mustValidateOK(t, v, &StateFile{
		Declarations: []*Declaration{
			nil,
			{ID: "file:/x", Module: "file", Name: "/x", State: "present"},
			nil,
		},
	})
}

func TestValidationIssue_StringFormatting(t *testing.T) {
	t.Parallel()
	cases := map[string]ValidationIssue{
		"file:/x (Module): cannot be empty": {DeclID: "file:/x", Field: "Module", Message: "cannot be empty"},
		"Module: cannot be empty":           {Field: "Module", Message: "cannot be empty"},
		"file:/x: dangling":                 {DeclID: "file:/x", Message: "dangling"},
	}
	for want, iss := range cases {
		if got := iss.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}

func TestValidationError_EmptyMessageStillSensible(t *testing.T) {
	t.Parallel()
	// Defensive: a *ValidationError with no issues should still
	// produce a non-empty error string. The Validator never returns
	// this shape (it returns nil on success), but the type is
	// exported and a caller could construct it manually.
	e := &ValidationError{}
	if e.Error() == "" {
		t.Error("empty ValidationError.Error() must still be non-empty")
	}
}

func hasFieldIssue(issues []ValidationIssue, field string) bool {
	for _, iss := range issues {
		if iss.Field == field {
			return true
		}
	}
	return false
}

func issueMessageContains(issues []ValidationIssue, field, substr string) bool {
	for _, iss := range issues {
		if iss.Field == field && strings.Contains(iss.Message, substr) {
			return true
		}
	}
	return false
}
