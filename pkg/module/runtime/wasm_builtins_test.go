package runtime

import (
	"context"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/module/capabilities"
)

func TestNewWasmHostFunctions(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()

	whf := NewWasmHostFunctions(registry)
	if whf == nil {
		t.Fatal("WasmHostFunctions should not be nil")
	}
	if whf.registry != registry {
		t.Error("registry should be set correctly")
	}
}

func TestNewWasmHostFunctionsNilRegistry(t *testing.T) {
	whf := NewWasmHostFunctions(nil)
	if whf == nil {
		t.Fatal("WasmHostFunctions should not be nil even with nil registry")
	}
	if whf.registry != nil {
		t.Error("registry should be nil")
	}
}

func TestCreateWasmCapabilityContext(t *testing.T) {
	ctx := createWasmCapabilityContext("wasm-module")
	if ctx == nil {
		t.Fatal("CapabilityContext should not be nil")
	}
	if ctx.ModuleName != "wasm-module" {
		t.Errorf("ModuleName = %v, want wasm-module", ctx.ModuleName)
	}
}

func TestCreateWasmCapabilityContextEmpty(t *testing.T) {
	ctx := createWasmCapabilityContext("")
	if ctx == nil {
		t.Fatal("CapabilityContext should not be nil")
	}
	if ctx.ModuleName != "" {
		t.Errorf("ModuleName = %v, want empty", ctx.ModuleName)
	}
}

func TestWasmHostFunctionsWithRegistry(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()

	// Register a test capability using the correct API
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")
	fsCap := capabilities.NewFSReadCapability(ctx, []string{"/tmp/test/**"}, nil, capabilities.DefaultMaxFileSize)
	if err := registry.Register(fsCap); err != nil {
		t.Fatalf("failed to register capability: %v", err)
	}

	whf := NewWasmHostFunctions(registry)
	if whf == nil {
		t.Fatal("WasmHostFunctions should not be nil")
	}

	// Verify capability can be retrieved from registry
	cap, err := registry.Get("fs.read")
	if err != nil {
		t.Errorf("failed to get capability: %v", err)
	}
	if cap == nil {
		t.Error("capability should not be nil")
	}
}

func TestWasmHostFunctionsMultipleCapabilities(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")

	// Register multiple capabilities using correct constructors
	caps := []capabilities.Capability{
		capabilities.NewFSReadCapability(ctx, []string{"/tmp/test/**"}, nil, capabilities.DefaultMaxFileSize),
		capabilities.NewFSWriteCapability(ctx, []string{"/tmp/test/**"}, nil),
		capabilities.NewLogCapability(ctx, nil, 100),
		capabilities.NewTimeCapability(ctx),
		capabilities.NewKVCapability(ctx, "test-namespace", nil),
	}

	for _, cap := range caps {
		if err := registry.Register(cap); err != nil {
			t.Fatalf("failed to register %s: %v", cap.Name(), err)
		}
	}

	whf := NewWasmHostFunctions(registry)
	if whf == nil {
		t.Fatal("WasmHostFunctions should not be nil")
	}

	// Verify each capability is retrievable
	for _, c := range caps {
		cap, err := whf.registry.Get(c.Name())
		if err != nil {
			t.Errorf("failed to get %s: %v", c.Name(), err)
		}
		if cap == nil {
			t.Errorf("%s capability should not be nil", c.Name())
		}
	}
}

func TestRegisterWithWasmRuntimeNilRuntime(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	whf := NewWasmHostFunctions(registry)

	// This should not panic - nil runtime is handled gracefully
	err := whf.RegisterWithWasmRuntime(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestWasmHostFunctionsEmptyRegistry(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	whf := NewWasmHostFunctions(registry)

	// Registering with empty registry should work
	err := whf.RegisterWithWasmRuntime(nil)
	if err != nil {
		t.Errorf("unexpected error with empty registry: %v", err)
	}
}

func TestWasmHostFunctionsCapabilityTypeAssertions(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")

	// Register capabilities
	if err := registry.Register(capabilities.NewFSReadCapability(ctx, []string{"/tmp/test/**"}, nil, capabilities.DefaultMaxFileSize)); err != nil {
		t.Fatalf("failed to register: %v", err)
	}
	if err := registry.Register(capabilities.NewFSWriteCapability(ctx, []string{"/tmp/test/**"}, nil)); err != nil {
		t.Fatalf("failed to register: %v", err)
	}
	if err := registry.Register(capabilities.NewLogCapability(ctx, nil, 100)); err != nil {
		t.Fatalf("failed to register: %v", err)
	}
	if err := registry.Register(capabilities.NewTimeCapability(ctx)); err != nil {
		t.Fatalf("failed to register: %v", err)
	}
	if err := registry.Register(capabilities.NewKVCapability(ctx, "test-namespace", nil)); err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	whf := NewWasmHostFunctions(registry)

	// Verify type assertions work correctly
	if cap, err := whf.registry.Get("fs.read"); err == nil && cap != nil {
		if _, ok := cap.(*capabilities.FSReadCapability); !ok {
			t.Error("fs.read should be *FSReadCapability")
		}
	}

	if cap, err := whf.registry.Get("fs.write"); err == nil && cap != nil {
		if _, ok := cap.(*capabilities.FSWriteCapability); !ok {
			t.Error("fs.write should be *FSWriteCapability")
		}
	}

	if cap, err := whf.registry.Get("log"); err == nil && cap != nil {
		if _, ok := cap.(*capabilities.LogCapability); !ok {
			t.Error("log should be *LogCapability")
		}
	}

	if cap, err := whf.registry.Get("time"); err == nil && cap != nil {
		if _, ok := cap.(*capabilities.TimeCapability); !ok {
			t.Error("time should be *TimeCapability")
		}
	}

	if cap, err := whf.registry.Get("kv"); err == nil && cap != nil {
		if _, ok := cap.(*capabilities.KVCapability); !ok {
			t.Error("kv should be *KVCapability")
		}
	}
}

func TestWasmCapabilityContextCreation(t *testing.T) {
	tests := []struct {
		name       string
		moduleName string
	}{
		{"standard name", "myorg/wasm-module"},
		{"empty name", ""},
		{"special chars", "org/module_v2.1"},
		{"unicode", "org/模块"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := createWasmCapabilityContext(tt.moduleName)
			if ctx == nil {
				t.Fatal("context should not be nil")
			}
			if ctx.ModuleName != tt.moduleName {
				t.Errorf("ModuleName = %v, want %v", ctx.ModuleName, tt.moduleName)
			}
		})
	}
}

func TestWasmHostFunctionsRegistryIntegration(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")

	// Test that all capability types can be registered without error
	caps := []capabilities.Capability{
		capabilities.NewFSReadCapability(ctx, []string{"/data/test/**"}, nil, capabilities.DefaultMaxFileSize),
		capabilities.NewFSWriteCapability(ctx, []string{"/output/test/**"}, nil),
		capabilities.NewLogCapability(ctx, nil, 50),
		capabilities.NewTimeCapability(ctx),
		capabilities.NewKVCapability(ctx, "test-namespace", nil),
	}

	for _, cap := range caps {
		if err := registry.Register(cap); err != nil {
			t.Fatalf("failed to register %s: %v", cap.Name(), err)
		}
	}

	whf := NewWasmHostFunctions(registry)

	// Verify all capabilities are accessible
	for _, c := range caps {
		cap, err := whf.registry.Get(c.Name())
		if err != nil {
			t.Errorf("Get(%s) failed: %v", c.Name(), err)
		}
		if cap == nil {
			t.Errorf("Get(%s) returned nil", c.Name())
		}
	}

	// Verify non-existent capability returns error
	_, err := whf.registry.Get("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent capability")
	}
}

func TestWasmHostFunctionsRegistryMethods(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")

	// Register capabilities
	if err := registry.Register(capabilities.NewTimeCapability(ctx)); err != nil {
		t.Fatalf("failed to register time: %v", err)
	}

	whf := NewWasmHostFunctions(registry)

	// Test Has method
	if !whf.registry.Has("time") {
		t.Error("registry should have time capability")
	}

	if whf.registry.Has("nonexistent") {
		t.Error("registry should not have nonexistent capability")
	}

	// Test List method
	names := whf.registry.List()
	found := false
	for _, name := range names {
		if name == "time" {
			found = true
			break
		}
	}
	if !found {
		t.Error("List should include time capability")
	}
}
