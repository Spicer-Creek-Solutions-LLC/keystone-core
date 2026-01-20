package state

import (
	"context"
	"testing"
)

type stubModule struct {
	name string
}

func (s *stubModule) Name() string {
	return s.name
}

func (s *stubModule) Execute(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	return &ModuleResult{Changed: false}, nil
}

func (s *stubModule) Check(ctx context.Context, mctx ModuleContext) (*ModuleResult, error) {
	return &ModuleResult{Changed: false}, nil
}

func TestProxyModuleRegistry(t *testing.T) {
	registry := NewProxyModuleRegistry()
	module := &stubModule{name: "stub"}

	registry.Register("stub", module)

	got, err := registry.Get("stub")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if got.Name() != "stub" {
		t.Fatalf("Unexpected module name: %s", got.Name())
	}

	if _, err := registry.Get("missing"); err == nil {
		t.Fatal("Expected error for missing module")
	}

	names := registry.List()
	if len(names) != 1 || names[0] != "stub" {
		t.Fatalf("Unexpected list result: %v", names)
	}
}

func TestBaseProxyModule_Getters(t *testing.T) {
	base := &BaseProxyModule{}
	params := map[string]interface{}{
		"str":   "value",
		"int":   3,
		"int64": int64(5),
		"float": float64(2),
		"bool":  true,
		"list":  []interface{}{"a", "b"},
	}

	if v, ok := base.GetString(params, "str"); !ok || v != "value" {
		t.Fatalf("GetString failed: %v %v", v, ok)
	}
	if v, ok := base.GetInt(params, "int"); !ok || v != 3 {
		t.Fatalf("GetInt failed: %v %v", v, ok)
	}
	if v, ok := base.GetInt(params, "int64"); !ok || v != 5 {
		t.Fatalf("GetInt int64 failed: %v %v", v, ok)
	}
	if v, ok := base.GetInt(params, "float"); !ok || v != 2 {
		t.Fatalf("GetInt float failed: %v %v", v, ok)
	}
	if v, ok := base.GetBool(params, "bool"); !ok || !v {
		t.Fatalf("GetBool failed: %v %v", v, ok)
	}
	if v, ok := base.GetStringSlice(params, "list"); !ok || len(v) != 2 {
		t.Fatalf("GetStringSlice failed: %v %v", v, ok)
	}
}
