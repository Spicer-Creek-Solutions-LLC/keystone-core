package capabilities

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockCapability is a test capability
type mockCapability struct {
	name      string
	validateFn func() error
}

func (m *mockCapability) Name() string {
	return m.name
}

func (m *mockCapability) Validate() error {
	if m.validateFn != nil {
		return m.validateFn()
	}
	return nil
}

// mockAuditLogger is a test audit logger
type mockAuditLogger struct {
	entries []AuditEntry
}

func (m *mockAuditLogger) Log(entry AuditEntry) {
	m.entries = append(m.entries, entry)
}

func TestCapabilityContext(t *testing.T) {
	ctx := context.Background()
	capCtx := NewCapabilityContext(ctx, "test-module")

	if capCtx.ModuleName != "test-module" {
		t.Errorf("expected module name 'test-module', got %s", capCtx.ModuleName)
	}

	if capCtx.CorrelationID == "" {
		t.Error("expected non-empty correlation ID")
	}

	if capCtx.StartTime.IsZero() {
		t.Error("expected non-zero start time")
	}

	// Test WithCorrelationID
	capCtx.WithCorrelationID("custom-id")
	if capCtx.CorrelationID != "custom-id" {
		t.Errorf("expected correlation ID 'custom-id', got %s", capCtx.CorrelationID)
	}

	// Test WithMetadata
	capCtx.WithMetadata("key", "value")
	if capCtx.Metadata["key"] != "value" {
		t.Errorf("expected metadata value 'value', got %v", capCtx.Metadata["key"])
	}

	// Test Duration
	time.Sleep(10 * time.Millisecond)
	duration := capCtx.Duration()
	if duration < 10*time.Millisecond {
		t.Errorf("expected duration >= 10ms, got %v", duration)
	}
}

func TestCapabilityRegistry_Register(t *testing.T) {
	registry := NewCapabilityRegistry()

	cap := &mockCapability{name: "test.capability"}
	err := registry.Register(cap)
	if err != nil {
		t.Fatalf("failed to register capability: %v", err)
	}

	// Verify registered
	if !registry.Has("test.capability") {
		t.Error("capability not found after registration")
	}

	// Test duplicate registration
	err = registry.Register(cap)
	if !errors.Is(err, ErrCapabilityAlreadyRegistered) {
		t.Errorf("expected ErrCapabilityAlreadyRegistered, got %v", err)
	}
}

func TestCapabilityRegistry_RegisterNil(t *testing.T) {
	registry := NewCapabilityRegistry()

	err := registry.Register(nil)
	if !errors.Is(err, ErrNilCapability) {
		t.Errorf("expected ErrNilCapability, got %v", err)
	}
}

func TestCapabilityRegistry_RegisterEmptyName(t *testing.T) {
	registry := NewCapabilityRegistry()

	cap := &mockCapability{name: ""}
	err := registry.Register(cap)
	if !errors.Is(err, ErrEmptyCapabilityName) {
		t.Errorf("expected ErrEmptyCapabilityName, got %v", err)
	}
}

func TestCapabilityRegistry_RegisterInvalid(t *testing.T) {
	registry := NewCapabilityRegistry()

	validationErr := errors.New("validation failed")
	cap := &mockCapability{
		name: "test.invalid",
		validateFn: func() error {
			return validationErr
		},
	}

	err := registry.Register(cap)
	if err == nil {
		t.Fatal("expected error for invalid capability")
	}
	if !errors.Is(err, validationErr) {
		t.Errorf("expected validation error to be wrapped, got %v", err)
	}
}

func TestCapabilityRegistry_Get(t *testing.T) {
	registry := NewCapabilityRegistry()

	cap := &mockCapability{name: "test.capability"}
	registry.Register(cap)

	retrieved, err := registry.Get("test.capability")
	if err != nil {
		t.Fatalf("failed to get capability: %v", err)
	}

	if retrieved != cap {
		t.Error("retrieved capability does not match registered capability")
	}

	// Test non-existent capability
	_, err = registry.Get("nonexistent")
	if !errors.Is(err, ErrCapabilityNotFound) {
		t.Errorf("expected ErrCapabilityNotFound, got %v", err)
	}
}

func TestCapabilityRegistry_Has(t *testing.T) {
	registry := NewCapabilityRegistry()

	if registry.Has("test.capability") {
		t.Error("expected Has to return false for non-existent capability")
	}

	cap := &mockCapability{name: "test.capability"}
	registry.Register(cap)

	if !registry.Has("test.capability") {
		t.Error("expected Has to return true for registered capability")
	}
}

func TestCapabilityRegistry_List(t *testing.T) {
	registry := NewCapabilityRegistry()

	cap1 := &mockCapability{name: "test.cap1"}
	cap2 := &mockCapability{name: "test.cap2"}

	registry.Register(cap1)
	registry.Register(cap2)

	names := registry.List()
	if len(names) != 2 {
		t.Fatalf("expected 2 capabilities, got %d", len(names))
	}

	// Check both names are present (order doesn't matter)
	found := make(map[string]bool)
	for _, name := range names {
		found[name] = true
	}

	if !found["test.cap1"] || !found["test.cap2"] {
		t.Errorf("unexpected capability names: %v", names)
	}
}

func TestCapabilityRegistry_Unregister(t *testing.T) {
	registry := NewCapabilityRegistry()

	cap := &mockCapability{name: "test.capability"}
	registry.Register(cap)

	err := registry.Unregister("test.capability")
	if err != nil {
		t.Fatalf("failed to unregister capability: %v", err)
	}

	if registry.Has("test.capability") {
		t.Error("capability still registered after unregister")
	}

	// Test unregistering non-existent capability
	err = registry.Unregister("nonexistent")
	if !errors.Is(err, ErrCapabilityNotFound) {
		t.Errorf("expected ErrCapabilityNotFound, got %v", err)
	}
}

func TestCapabilityRegistry_Clear(t *testing.T) {
	registry := NewCapabilityRegistry()

	cap1 := &mockCapability{name: "test.cap1"}
	cap2 := &mockCapability{name: "test.cap2"}

	registry.Register(cap1)
	registry.Register(cap2)

	registry.Clear()

	if len(registry.List()) != 0 {
		t.Errorf("expected 0 capabilities after clear, got %d", len(registry.List()))
	}
}

func TestCapabilityInvoker_Invoke(t *testing.T) {
	registry := NewCapabilityRegistry()
	auditor := &mockAuditLogger{}
	invoker := NewCapabilityInvoker(registry, auditor)

	cap := &mockCapability{name: "test.capability"}
	registry.Register(cap)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test successful invocation
	result, err := invoker.Invoke(ctx, "test.capability", func(c Capability) (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Fatalf("invocation failed: %v", err)
	}

	if result != "success" {
		t.Errorf("expected result 'success', got %v", result)
	}

	// Check audit log
	if len(auditor.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(auditor.entries))
	}

	entry := auditor.entries[0]
	if entry.ModuleName != "test-module" {
		t.Errorf("expected module name 'test-module', got %s", entry.ModuleName)
	}
	if entry.Capability != "test.capability" {
		t.Errorf("expected capability 'test.capability', got %s", entry.Capability)
	}
	if !entry.Success {
		t.Error("expected successful invocation")
	}
}

func TestCapabilityInvoker_InvokeError(t *testing.T) {
	registry := NewCapabilityRegistry()
	auditor := &mockAuditLogger{}
	invoker := NewCapabilityInvoker(registry, auditor)

	cap := &mockCapability{name: "test.capability"}
	registry.Register(cap)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	expectedErr := errors.New("invocation error")

	// Test failed invocation
	_, err := invoker.Invoke(ctx, "test.capability", func(c Capability) (interface{}, error) {
		return nil, expectedErr
	})

	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}

	// Check audit log
	if len(auditor.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(auditor.entries))
	}

	entry := auditor.entries[0]
	if entry.Success {
		t.Error("expected failed invocation")
	}
	if entry.Error != expectedErr.Error() {
		t.Errorf("expected error %v, got %v", expectedErr.Error(), entry.Error)
	}
}

func TestCapabilityInvoker_InvokeNotFound(t *testing.T) {
	registry := NewCapabilityRegistry()
	auditor := &mockAuditLogger{}
	invoker := NewCapabilityInvoker(registry, auditor)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test invocation of non-existent capability
	_, err := invoker.Invoke(ctx, "nonexistent", func(c Capability) (interface{}, error) {
		return nil, nil
	})

	if !errors.Is(err, ErrCapabilityNotFound) {
		t.Errorf("expected ErrCapabilityNotFound, got %v", err)
	}

	// Check audit log (should still log the attempt)
	if len(auditor.entries) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(auditor.entries))
	}

	entry := auditor.entries[0]
	if entry.Success {
		t.Error("expected failed invocation")
	}
}

func TestCapabilityInvoker_NoAuditor(t *testing.T) {
	registry := NewCapabilityRegistry()
	invoker := NewCapabilityInvoker(registry, nil)

	cap := &mockCapability{name: "test.capability"}
	registry.Register(cap)

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Should not panic with nil auditor
	_, err := invoker.Invoke(ctx, "test.capability", func(c Capability) (interface{}, error) {
		return "success", nil
	})

	if err != nil {
		t.Fatalf("invocation failed: %v", err)
	}
}
