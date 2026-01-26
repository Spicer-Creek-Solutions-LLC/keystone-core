package policy

import (
	"context"
	"testing"
)

func TestNewTypeRegistry(t *testing.T) {
	tr := NewTypeRegistry()
	if tr == nil {
		t.Fatal("expected non-nil registry")
	}

	// Should have built-in types registered
	types := tr.List()
	if len(types) < 3 {
		t.Errorf("expected at least 3 built-in types, got %d", len(types))
	}

	// Check OPA is registered
	if _, ok := tr.Get(PolicyTypeOPA); !ok {
		t.Error("expected OPA type to be registered")
	}

	// Check CEL is registered
	if _, ok := tr.Get(PolicyTypeCEL); !ok {
		t.Error("expected CEL type to be registered")
	}

	// Check Builtin is registered
	if _, ok := tr.Get(PolicyTypeBuiltin); !ok {
		t.Error("expected Builtin type to be registered")
	}
}

func TestTypeRegistry_Register(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	handler := &testHandler{
		typeValue: PolicyType("test-type"),
		name:      "Test Handler",
	}

	err := tr.Register(handler)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify registration
	h, ok := tr.Get(PolicyType("test-type"))
	if !ok {
		t.Error("expected handler to be registered")
	}
	if h.Name() != "Test Handler" {
		t.Errorf("expected name 'Test Handler', got '%s'", h.Name())
	}

	// Duplicate registration should fail
	err = tr.Register(handler)
	if err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestTypeRegistry_Register_NilHandler(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	err := tr.Register(nil)
	if err == nil {
		t.Error("expected error for nil handler")
	}
}

func TestTypeRegistry_Register_EmptyType(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	handler := &testHandler{
		typeValue: PolicyType(""),
		name:      "Test Handler",
	}

	err := tr.Register(handler)
	if err == nil {
		t.Error("expected error for empty type")
	}
}

func TestTypeRegistry_Unregister(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	handler := &testHandler{
		typeValue: PolicyType("test-type"),
		name:      "Test Handler",
	}

	tr.Register(handler)

	err := tr.Unregister(PolicyType("test-type"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should not be found
	if _, ok := tr.Get(PolicyType("test-type")); ok {
		t.Error("expected handler to be unregistered")
	}

	// Unregistering again should fail
	err = tr.Unregister(PolicyType("test-type"))
	if err == nil {
		t.Error("expected error for unregistering non-existent type")
	}
}

func TestTypeRegistry_GetMetadata(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	handler := &testHandler{
		typeValue:   PolicyType("test-type"),
		name:        "Test Handler",
		description: "A test handler",
		features:    []PolicyFeature{FeatureDryRun, FeatureMetrics},
	}

	tr.Register(handler)

	metadata, ok := tr.GetMetadata(PolicyType("test-type"))
	if !ok {
		t.Fatal("expected metadata to exist")
	}

	if metadata.Name != "Test Handler" {
		t.Errorf("expected name 'Test Handler', got '%s'", metadata.Name)
	}

	if metadata.Description != "A test handler" {
		t.Errorf("expected description 'A test handler', got '%s'", metadata.Description)
	}

	if len(metadata.Features) != 2 {
		t.Errorf("expected 2 features, got %d", len(metadata.Features))
	}
}

func TestTypeRegistry_ListMetadata(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	tr.Register(&testHandler{typeValue: PolicyType("type1"), name: "Type 1"})
	tr.Register(&testHandler{typeValue: PolicyType("type2"), name: "Type 2"})

	metadata := tr.ListMetadata()
	if len(metadata) != 2 {
		t.Errorf("expected 2 metadata entries, got %d", len(metadata))
	}
}

func TestTypeRegistry_HasFeature(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	handler := &testHandler{
		typeValue: PolicyType("test-type"),
		features:  []PolicyFeature{FeatureDryRun, FeatureMetrics},
	}

	tr.Register(handler)

	if !tr.HasFeature(PolicyType("test-type"), FeatureDryRun) {
		t.Error("expected type to have FeatureDryRun")
	}

	if !tr.HasFeature(PolicyType("test-type"), FeatureMetrics) {
		t.Error("expected type to have FeatureMetrics")
	}

	if tr.HasFeature(PolicyType("test-type"), FeatureDebug) {
		t.Error("expected type to NOT have FeatureDebug")
	}

	if tr.HasFeature(PolicyType("non-existent"), FeatureDryRun) {
		t.Error("expected non-existent type to return false")
	}
}

func TestTypeRegistry_Evaluate(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	evaluateCalled := false
	handler := &testHandler{
		typeValue: PolicyType("test-type"),
		evaluateFunc: func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
			evaluateCalled = true
			return &EvaluationResult{
				PolicyID: policy.ID,
				Allowed:  true,
			}, nil
		},
	}

	tr.Register(handler)

	policy := &Policy{
		ID:   "test-policy",
		Type: PolicyType("test-type"),
	}

	result, err := tr.Evaluate(context.Background(), policy, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !evaluateCalled {
		t.Error("expected evaluate function to be called")
	}

	if !result.Allowed {
		t.Error("expected result to be allowed")
	}

	// Check metrics updated
	metadata, _ := tr.GetMetadata(PolicyType("test-type"))
	if metadata.EvaluationCount != 1 {
		t.Errorf("expected evaluation count 1, got %d", metadata.EvaluationCount)
	}
}

func TestTypeRegistry_Evaluate_UnknownType(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	policy := &Policy{
		ID:   "test-policy",
		Type: PolicyType("unknown-type"),
	}

	_, err := tr.Evaluate(context.Background(), policy, nil)
	if err == nil {
		t.Error("expected error for unknown type")
	}
}

func TestTypeRegistry_Validate(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	validateCalled := false
	handler := &testHandler{
		typeValue: PolicyType("test-type"),
		validateFunc: func(policy *Policy) error {
			validateCalled = true
			return nil
		},
	}

	tr.Register(handler)

	policy := &Policy{
		ID:   "test-policy",
		Type: PolicyType("test-type"),
	}

	err := tr.Validate(policy)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !validateCalled {
		t.Error("expected validate function to be called")
	}
}

func TestTypeRegistry_Callbacks(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	var registeredType PolicyType
	var unregisteredType PolicyType

	tr.OnRegister(func(pt PolicyType, h PolicyTypeHandler) {
		registeredType = pt
	})

	tr.OnUnregister(func(pt PolicyType) {
		unregisteredType = pt
	})

	handler := &testHandler{typeValue: PolicyType("test-type")}
	tr.Register(handler)

	if registeredType != PolicyType("test-type") {
		t.Errorf("expected registered type 'test-type', got '%s'", registeredType)
	}

	tr.Unregister(PolicyType("test-type"))

	if unregisteredType != PolicyType("test-type") {
		t.Errorf("expected unregistered type 'test-type', got '%s'", unregisteredType)
	}
}

func TestOPAHandler(t *testing.T) {
	h := &OPAHandler{}

	if h.Type() != PolicyTypeOPA {
		t.Errorf("expected type OPA, got %s", h.Type())
	}

	if h.Name() == "" {
		t.Error("expected non-empty name")
	}

	if h.Description() == "" {
		t.Error("expected non-empty description")
	}

	features := h.SupportedFeatures()
	if len(features) == 0 {
		t.Error("expected some features")
	}
}

func TestCELHandler(t *testing.T) {
	h := &CELHandler{}

	if h.Type() != PolicyTypeCEL {
		t.Errorf("expected type CEL, got %s", h.Type())
	}

	if h.Name() == "" {
		t.Error("expected non-empty name")
	}

	if h.Description() == "" {
		t.Error("expected non-empty description")
	}
}

func TestBuiltinHandler(t *testing.T) {
	h := &BuiltinHandler{}

	if h.Type() != PolicyTypeBuiltin {
		t.Errorf("expected type Builtin, got %s", h.Type())
	}

	if h.Name() == "" {
		t.Error("expected non-empty name")
	}
}

func TestNewCustomHandler(t *testing.T) {
	evaluateCalled := false
	validateCalled := false

	h := NewCustomHandler(CustomHandlerOptions{
		Type:        PolicyType("custom"),
		Name:        "Custom Handler",
		Description: "A custom handler",
		Features:    []PolicyFeature{FeatureDryRun},
		EvaluateFunc: func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
			evaluateCalled = true
			return &EvaluationResult{Allowed: true}, nil
		},
		ValidateFunc: func(policy *Policy) error {
			validateCalled = true
			return nil
		},
	})

	if h.Type() != PolicyType("custom") {
		t.Errorf("expected type 'custom', got '%s'", h.Type())
	}

	if h.Name() != "Custom Handler" {
		t.Errorf("expected name 'Custom Handler', got '%s'", h.Name())
	}

	if h.Description() != "A custom handler" {
		t.Errorf("expected description 'A custom handler', got '%s'", h.Description())
	}

	if len(h.SupportedFeatures()) != 1 {
		t.Errorf("expected 1 feature, got %d", len(h.SupportedFeatures()))
	}

	// Test evaluate
	h.Evaluate(context.Background(), &Policy{}, nil)
	if !evaluateCalled {
		t.Error("expected evaluate to be called")
	}

	// Test validate
	h.Validate(&Policy{})
	if !validateCalled {
		t.Error("expected validate to be called")
	}
}

func TestCustomHandler_NilFunctions(t *testing.T) {
	h := NewCustomHandler(CustomHandlerOptions{
		Type: PolicyType("custom"),
		Name: "Custom Handler",
	})

	// Evaluate without function should error
	_, err := h.Evaluate(context.Background(), &Policy{}, nil)
	if err == nil {
		t.Error("expected error for nil evaluate function")
	}

	// Validate without function should succeed
	err = h.Validate(&Policy{})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPluginLoader(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	pl := NewPluginLoader(tr)

	config := &PluginConfig{
		Name:        "Test Plugin",
		Type:        "test-plugin",
		Description: "A test plugin",
		Features:    []PolicyFeature{FeatureDryRun},
		EvaluateFunc: func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
			return &EvaluationResult{Allowed: true}, nil
		},
	}

	err := pl.LoadPlugin(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !pl.IsLoaded("Test Plugin") {
		t.Error("expected plugin to be loaded")
	}

	// Loading again should fail
	err = pl.LoadPlugin(config)
	if err == nil {
		t.Error("expected error for duplicate load")
	}

	loaded := pl.ListLoaded()
	if len(loaded) != 1 {
		t.Errorf("expected 1 loaded plugin, got %d", len(loaded))
	}
}

func TestPluginLoader_Unload(t *testing.T) {
	tr := &TypeRegistry{
		handlers: make(map[PolicyType]PolicyTypeHandler),
		metadata: make(map[PolicyType]*TypeMetadata),
	}

	pl := NewPluginLoader(tr)

	config := &PluginConfig{
		Name: "Test Plugin",
		Type: "test-plugin",
	}

	pl.LoadPlugin(config)

	err := pl.UnloadPlugin("Test Plugin")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if pl.IsLoaded("Test Plugin") {
		t.Error("expected plugin to be unloaded")
	}

	// Unloading non-existent should fail
	err = pl.UnloadPlugin("Test Plugin")
	if err == nil {
		t.Error("expected error for unloading non-existent plugin")
	}
}

// testHandler is a mock handler for testing
type testHandler struct {
	typeValue    PolicyType
	name         string
	description  string
	features     []PolicyFeature
	evaluateFunc func(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error)
	validateFunc func(policy *Policy) error
}

func (h *testHandler) Type() PolicyType        { return h.typeValue }
func (h *testHandler) Name() string            { return h.name }
func (h *testHandler) Description() string     { return h.description }
func (h *testHandler) SupportedFeatures() []PolicyFeature { return h.features }

func (h *testHandler) Evaluate(ctx context.Context, policy *Policy, input interface{}) (*EvaluationResult, error) {
	if h.evaluateFunc != nil {
		return h.evaluateFunc(ctx, policy, input)
	}
	return &EvaluationResult{Allowed: true}, nil
}

func (h *testHandler) Validate(policy *Policy) error {
	if h.validateFunc != nil {
		return h.validateFunc(policy)
	}
	return nil
}
