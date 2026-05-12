package statemgmt

import (
	"context"
	"fmt"
	"testing"
)

func TestDeclaration_moduleView_StripsRequisiteKeys(t *testing.T) {
	t.Parallel()
	d := &Declaration{
		ID:     "file:/etc/x",
		Module: "file",
		State:  "present",
		Name:   "/etc/x",
		Params: map[string]any{
			"content":      "hi",
			"severity":     "high",
			ReqRequire:     []any{map[string]any{"package": "nginx"}},
			ReqWatch:       []any{map[string]any{"package": "nginx"}},
			ReqOnChangesIn: []any{map[string]any{"service": "nginx"}},
		},
	}
	got := d.moduleView()
	if got == d {
		t.Fatal("moduleView returned the same Declaration despite requisite keys present")
	}
	for _, k := range RequisiteKeys {
		if _, ok := got.Params[k]; ok {
			t.Errorf("requisite key %q still present in moduleView Params", k)
		}
	}
	if got.Params["content"] != "hi" {
		t.Errorf("content param lost: %v", got.Params["content"])
	}
	if got.Params[ReservedSeverityParamKey] != "high" {
		t.Errorf("severity param should be retained, got %v", got.Params[ReservedSeverityParamKey])
	}
	// Identity fields carried over.
	if got.ID != d.ID || got.Module != d.Module || got.State != d.State || got.Name != d.Name {
		t.Errorf("moduleView mangled identity fields: %+v", got)
	}
	// Original untouched.
	if _, ok := d.Params[ReqRequire]; !ok {
		t.Error("moduleView mutated the original Declaration's Params")
	}
}

func TestDeclaration_moduleView_NoRequisites_ReturnsSame(t *testing.T) {
	t.Parallel()
	d := &Declaration{Params: map[string]any{"content": "x"}}
	if d.moduleView() != d {
		t.Error("moduleView allocated a copy for a Declaration with no requisite keys")
	}
	empty := &Declaration{}
	if empty.moduleView() != empty {
		t.Error("moduleView allocated a copy for a Declaration with no Params")
	}
	var nilDecl *Declaration
	if nilDecl.moduleView() != nil {
		t.Error("moduleView(nil) should be nil")
	}
}

// strictModule rejects any Params key it does not recognise — the
// typo-defense pattern every stdlib module uses. It is the canary for
// the requisite-keys-leak regression.
type strictModule struct{ allowed map[string]struct{} }

func (strictModule) Name() string          { return "strict" }
func (strictModule) ValidStates() []string { return []string{"present"} }
func (m strictModule) check(decl *Declaration) error {
	for k := range decl.Params {
		if _, ok := m.allowed[k]; !ok {
			return fmt.Errorf("unknown param %q", k)
		}
	}
	return nil
}
func (m strictModule) Check(_ context.Context, decl *Declaration) (*ModuleCheckResult, error) {
	if err := m.check(decl); err != nil {
		return nil, err
	}
	return &ModuleCheckResult{Matches: true}, nil
}
func (m strictModule) Apply(_ context.Context, decl *Declaration) (*StateResult, error) {
	if err := m.check(decl); err != nil {
		return nil, err
	}
	return &StateResult{Success: true}, nil
}
func (m strictModule) Test(_ context.Context, decl *Declaration) (bool, error) {
	return true, m.check(decl)
}
func (m strictModule) Validate(decl *Declaration) error { return m.check(decl) }

func newStrictModule(reg *Registry, allowed ...string) {
	set := map[string]struct{}{ReservedSeverityParamKey: {}}
	for _, a := range allowed {
		set[a] = struct{}{}
	}
	if err := reg.Register("strict", func() Module { return strictModule{allowed: set} }); err != nil {
		panic(err)
	}
}

func TestRunner_RequisiteKeysHiddenFromModule(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newStrictModule(reg, "content")
	a := &Declaration{ID: "strict:a", Module: "strict", State: "present", Name: "a"}
	b := &Declaration{
		ID: "strict:b", Module: "strict", State: "present", Name: "b",
		Params: map[string]any{
			"content":  "x",
			ReqRequire: []any{map[string]any{"strict": "a"}},
		},
	}
	r := NewRunner(reg, nil)
	rep, err := r.Run(context.Background(), []*Declaration{a, b})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for _, res := range rep.Results {
		if res.Outcome == OutcomeFailed {
			t.Fatalf("decl %s failed: %v", res.DeclID, res.Error)
		}
	}
}

func TestValidator_RequisiteKeysHiddenFromModuleValidate(t *testing.T) {
	t.Parallel()
	reg := NewRegistry()
	newStrictModule(reg, "content")
	v := NewValidator(reg)
	sf := &StateFile{Declarations: []*Declaration{
		{ID: "strict:a", Module: "strict", State: "present", Name: "a"},
		{
			ID: "strict:b", Module: "strict", State: "present", Name: "b",
			Params: map[string]any{
				"content":  "x",
				ReqRequire: []any{map[string]any{"strict": "a"}},
			},
		},
	}}
	if err := v.Validate(sf); err != nil {
		t.Fatalf("Validate rejected a requisite-bearing declaration: %v", err)
	}
}
