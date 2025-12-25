package statemgmt

import (
	"context"
	"testing"
)

// MockModule is a mock module for testing
type MockModule struct {
	*BaseModule
	checkCalled bool
	applyCalled bool
	testCalled  bool
}

func NewMockModule() *MockModule {
	return &MockModule{
		BaseModule: NewBaseModule("mock", []string{"present", "absent"}),
	}
}

func (m *MockModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	m.checkCalled = true
	return &ModuleCheckResult{
		Present:      true,
		CurrentState: "present",
		Matches:      decl.State == "present",
		Diff:         make(map[string]interface{}),
		Metadata:     make(map[string]interface{}),
	}, nil
}

func (m *MockModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	m.applyCalled = true
	return &StateResult{
		StateID: decl.ID,
		Module:  m.Name(),
		Success: true,
		Changed: true,
		Comment: "Mock applied",
		Changes: make(map[string]interface{}),
	}, nil
}

func (m *MockModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	m.testCalled = true
	return true, nil
}

func TestModuleRegistry_Register(t *testing.T) {
	registry := NewModuleRegistry()
	module := NewMockModule()

	err := registry.Register(module)
	if err != nil {
		t.Fatalf("Failed to register module: %v", err)
	}

	// Try to register again (should fail)
	err = registry.Register(module)
	if err == nil {
		t.Fatal("Expected error when registering duplicate module")
	}
}

func TestModuleRegistry_Get(t *testing.T) {
	registry := NewModuleRegistry()
	module := NewMockModule()

	err := registry.Register(module)
	if err != nil {
		t.Fatalf("Failed to register module: %v", err)
	}

	retrieved, err := registry.Get("mock")
	if err != nil {
		t.Fatalf("Failed to get module: %v", err)
	}

	if retrieved.Name() != "mock" {
		t.Errorf("Expected module name 'mock', got '%s'", retrieved.Name())
	}

	// Try to get non-existent module
	_, err = registry.Get("nonexistent")
	if err == nil {
		t.Fatal("Expected error when getting non-existent module")
	}
}

func TestModuleRegistry_List(t *testing.T) {
	registry := NewModuleRegistry()
	module1 := NewMockModule()

	err := registry.Register(module1)
	if err != nil {
		t.Fatalf("Failed to register module: %v", err)
	}

	modules := registry.List()
	if len(modules) != 1 {
		t.Errorf("Expected 1 module, got %d", len(modules))
	}

	if modules[0] != "mock" {
		t.Errorf("Expected module name 'mock', got '%s'", modules[0])
	}
}

func TestBaseModule_Name(t *testing.T) {
	module := NewBaseModule("test", []string{"present", "absent"})
	if module.Name() != "test" {
		t.Errorf("Expected name 'test', got '%s'", module.Name())
	}
}

func TestBaseModule_ValidStates(t *testing.T) {
	states := []string{"present", "absent"}
	module := NewBaseModule("test", states)

	validStates := module.ValidStates()
	if len(validStates) != 2 {
		t.Errorf("Expected 2 valid states, got %d", len(validStates))
	}
}

func TestGetStringParameter(t *testing.T) {
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{
			"key": "value",
		},
	}

	value := getStringParameter(decl, "key", "default")
	if value != "value" {
		t.Errorf("Expected 'value', got '%s'", value)
	}

	value = getStringParameter(decl, "missing", "default")
	if value != "default" {
		t.Errorf("Expected 'default', got '%s'", value)
	}
}

func TestGetBoolParameter(t *testing.T) {
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{
			"enabled": true,
		},
	}

	value := getBoolParameter(decl, "enabled", false)
	if !value {
		t.Error("Expected true, got false")
	}

	value = getBoolParameter(decl, "missing", true)
	if !value {
		t.Error("Expected true (default), got false")
	}
}

func TestGetIntParameter(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected int
	}{
		{"int", 42, 42},
		{"int64", int64(42), 42},
		{"float64", float64(42), 42},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := &StateDeclaration{
				Parameters: map[string]interface{}{
					"num": tt.value,
				},
			}

			value := getIntParameter(decl, "num", 0)
			if value != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, value)
			}
		})
	}

	// Test default
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{},
	}
	value := getIntParameter(decl, "missing", 99)
	if value != 99 {
		t.Errorf("Expected 99 (default), got %d", value)
	}
}

func TestGetStringSliceParameter(t *testing.T) {
	decl := &StateDeclaration{
		Parameters: map[string]interface{}{
			"list": []interface{}{"a", "b", "c"},
		},
	}

	value := getStringSliceParameter(decl, "list")
	if len(value) != 3 {
		t.Errorf("Expected 3 items, got %d", len(value))
	}

	if value[0] != "a" || value[1] != "b" || value[2] != "c" {
		t.Errorf("Expected [a, b, c], got %v", value)
	}

	// Test missing parameter
	value = getStringSliceParameter(decl, "missing")
	if value != nil {
		t.Errorf("Expected nil, got %v", value)
	}
}

func TestDefaultRegistry(t *testing.T) {
	// Test that default modules are registered
	modules := []string{"file", "package", "service", "user", "group", "cmd"}

	for _, moduleName := range modules {
		_, err := GetModule(moduleName)
		if err != nil {
			t.Errorf("Expected module '%s' to be registered, got error: %v", moduleName, err)
		}
	}
}
