package runtime

import (
	"context"
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/module/capabilities"
)

func TestNewCapabilityBuiltins(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()

	cb := NewCapabilityBuiltins(registry)
	if cb == nil {
		t.Fatal("CapabilityBuiltins should not be nil")
	}
	if cb.registry != registry {
		t.Error("registry should be set correctly")
	}
}

func TestNewCapabilityBuiltinsNilRegistry(t *testing.T) {
	cb := NewCapabilityBuiltins(nil)
	if cb == nil {
		t.Fatal("CapabilityBuiltins should not be nil even with nil registry")
	}
	if cb.registry != nil {
		t.Error("registry should be nil")
	}
}

func TestCreateCapabilityContext(t *testing.T) {
	ctx := createCapabilityContext("test-module", "1.0.0")
	if ctx == nil {
		t.Fatal("CapabilityContext should not be nil")
	}
	if ctx.ModuleName != "test-module" {
		t.Errorf("ModuleName = %v, want test-module", ctx.ModuleName)
	}
	if ctx.ModuleVersion != "1.0.0" {
		t.Errorf("ModuleVersion = %v, want 1.0.0", ctx.ModuleVersion)
	}
}

func TestCreateCapabilityContextEmpty(t *testing.T) {
	ctx := createCapabilityContext("", "")
	if ctx == nil {
		t.Fatal("CapabilityContext should not be nil")
	}
	if ctx.ModuleName != "" {
		t.Errorf("ModuleName = %v, want empty", ctx.ModuleName)
	}
}

func TestRegisterStarlarkBuiltinsEmptyRegistry(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	cb := NewCapabilityBuiltins(registry)

	// This should not panic - empty registry means no capabilities registered
	err := cb.RegisterStarlarkBuiltins(nil)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCapabilityBuiltinsWithRegistry(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()

	// Register a test capability using the correct API
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")
	fsCap := capabilities.NewFSReadCapability(ctx, []string{"/tmp/test/**"}, nil, capabilities.DefaultMaxFileSize)
	if err := registry.Register(fsCap); err != nil {
		t.Fatalf("failed to register capability: %v", err)
	}

	cb := NewCapabilityBuiltins(registry)
	if cb == nil {
		t.Fatal("CapabilityBuiltins should not be nil")
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

func TestCapabilityBuiltinsStructure(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")

	t.Run("fs.write capability", func(t *testing.T) {
		writeCap := capabilities.NewFSWriteCapability(ctx, []string{"/tmp/test/**"}, nil)
		if err := registry.Register(writeCap); err != nil {
			t.Fatalf("failed to register: %v", err)
		}
	})

	t.Run("log capability", func(t *testing.T) {
		logCap := capabilities.NewLogCapability(ctx, nil, 100)
		if err := registry.Register(logCap); err != nil {
			t.Fatalf("failed to register: %v", err)
		}
	})

	t.Run("time capability", func(t *testing.T) {
		timeCap := capabilities.NewTimeCapability(ctx)
		if err := registry.Register(timeCap); err != nil {
			t.Fatalf("failed to register: %v", err)
		}
	})

	cb := NewCapabilityBuiltins(registry)
	if cb == nil {
		t.Fatal("CapabilityBuiltins should not be nil")
	}
}

func TestCapabilityRegistryIntegration(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")

	// Register multiple capabilities using the correct constructors
	caps := []capabilities.Capability{
		capabilities.NewFSReadCapability(ctx, []string{"/tmp/test/**"}, nil, capabilities.DefaultMaxFileSize),
		capabilities.NewLogCapability(ctx, nil, 100),
		capabilities.NewTimeCapability(ctx),
	}

	for _, cap := range caps {
		if err := registry.Register(cap); err != nil {
			t.Fatalf("failed to register %s: %v", cap.Name(), err)
		}
	}

	cb := NewCapabilityBuiltins(registry)

	// Verify each capability is retrievable
	for _, c := range caps {
		cap, err := cb.registry.Get(c.Name())
		if err != nil {
			t.Errorf("failed to get %s: %v", c.Name(), err)
		}
		if cap == nil {
			t.Errorf("%s capability should not be nil", c.Name())
		}
	}
}

func TestCapabilityContextFields(t *testing.T) {
	tests := []struct {
		name    string
		module  string
		version string
	}{
		{
			name:    "standard values",
			module:  "myorg/mymodule",
			version: "2.0.0",
		},
		{
			name:    "empty values",
			module:  "",
			version: "",
		},
		{
			name:    "special characters",
			module:  "org/module-v2",
			version: "1.0.0-beta.1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := createCapabilityContext(tt.module, tt.version)
			if ctx.ModuleName != tt.module {
				t.Errorf("ModuleName = %v, want %v", ctx.ModuleName, tt.module)
			}
			if ctx.ModuleVersion != tt.version {
				t.Errorf("ModuleVersion = %v, want %v", ctx.ModuleVersion, tt.version)
			}
		})
	}
}

func TestCapabilityBuiltinsTypeAssertions(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")

	// Register capabilities
	fsCap := capabilities.NewFSReadCapability(ctx, []string{"/tmp/test/**"}, nil, capabilities.DefaultMaxFileSize)
	if err := registry.Register(fsCap); err != nil {
		t.Fatalf("failed to register fs.read: %v", err)
	}

	logCap := capabilities.NewLogCapability(ctx, nil, 100)
	if err := registry.Register(logCap); err != nil {
		t.Fatalf("failed to register log: %v", err)
	}

	timeCap := capabilities.NewTimeCapability(ctx)
	if err := registry.Register(timeCap); err != nil {
		t.Fatalf("failed to register time: %v", err)
	}

	cb := NewCapabilityBuiltins(registry)

	// Verify type assertions work correctly
	if cap, err := cb.registry.Get("fs.read"); err == nil && cap != nil {
		if _, ok := cap.(*capabilities.FSReadCapability); !ok {
			t.Error("fs.read should be *FSReadCapability")
		}
	}

	if cap, err := cb.registry.Get("log"); err == nil && cap != nil {
		if _, ok := cap.(*capabilities.LogCapability); !ok {
			t.Error("log should be *LogCapability")
		}
	}

	if cap, err := cb.registry.Get("time"); err == nil && cap != nil {
		if _, ok := cap.(*capabilities.TimeCapability); !ok {
			t.Error("time should be *TimeCapability")
		}
	}
}

func TestCapabilityBuiltinsRegistryMethods(t *testing.T) {
	registry := capabilities.NewCapabilityRegistry()
	ctx := capabilities.NewCapabilityContext(context.Background(), "test")

	// Register capabilities
	if err := registry.Register(capabilities.NewTimeCapability(ctx)); err != nil {
		t.Fatalf("failed to register time: %v", err)
	}

	cb := NewCapabilityBuiltins(registry)

	// Test Has method
	if !cb.registry.Has("time") {
		t.Error("registry should have time capability")
	}

	if cb.registry.Has("nonexistent") {
		t.Error("registry should not have nonexistent capability")
	}

	// Test List method
	names := cb.registry.List()
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

	// Test Get method for non-existent capability
	_, err := cb.registry.Get("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent capability")
	}
}
