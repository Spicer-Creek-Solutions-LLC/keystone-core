package blueprint

import (
	"context"
	"testing"
)

func TestDefaultStateResolverConfig(t *testing.T) {
	config := DefaultStateResolverConfig()

	if config == nil {
		t.Fatal("DefaultStateResolverConfig() returned nil")
	}

	if len(config.LocalBlueprintPaths) == 0 {
		t.Error("LocalBlueprintPaths should not be empty")
	}

	if config.CachePath == "" {
		t.Error("CachePath should not be empty")
	}

	if !config.AllowRemote {
		t.Error("AllowRemote should be true by default")
	}
}

func TestNewStateResolver(t *testing.T) {
	tests := []struct {
		name    string
		config  *StateResolverConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "custom config",
			config: &StateResolverConfig{
				LocalBlueprintPaths: []string{"./test-blueprints"},
				CachePath:           "/tmp/test-cache",
				AllowRemote:         false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resolver, err := NewStateResolver(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewStateResolver() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && resolver == nil {
				t.Error("NewStateResolver() returned nil resolver")
			}
		})
	}
}

func TestNewStateResolverWithOptions(t *testing.T) {
	resolver, err := NewStateResolverWithOptions(
		WithLocalPaths("./blueprints", "/etc/blueprints"),
		WithCachePath("/tmp/bp-cache"),
		WithRemoteDisabled(),
		WithStrictVersions(),
	)
	if err != nil {
		t.Fatalf("NewStateResolverWithOptions() error = %v", err)
	}

	if resolver == nil {
		t.Fatal("NewStateResolverWithOptions() returned nil")
	}

	// Verify options were applied
	if resolver.config.AllowRemote {
		t.Error("AllowRemote should be false after WithRemoteDisabled()")
	}

	if !resolver.config.StrictVersions {
		t.Error("StrictVersions should be true after WithStrictVersions()")
	}
}

func TestWithLocalPaths(t *testing.T) {
	config := DefaultStateResolverConfig()
	opt := WithLocalPaths("./path1", "./path2")
	opt(config)

	if len(config.LocalBlueprintPaths) != 2 {
		t.Errorf("LocalBlueprintPaths length = %d, want 2", len(config.LocalBlueprintPaths))
	}
	if config.LocalBlueprintPaths[0] != "./path1" {
		t.Errorf("LocalBlueprintPaths[0] = %s, want ./path1", config.LocalBlueprintPaths[0])
	}
}

func TestWithCachePath(t *testing.T) {
	config := DefaultStateResolverConfig()
	opt := WithCachePath("/custom/cache")
	opt(config)

	if config.CachePath != "/custom/cache" {
		t.Errorf("CachePath = %s, want /custom/cache", config.CachePath)
	}
}

func TestWithRemoteDisabled(t *testing.T) {
	config := DefaultStateResolverConfig()
	if !config.AllowRemote {
		t.Fatal("Default should have AllowRemote = true")
	}

	opt := WithRemoteDisabled()
	opt(config)

	if config.AllowRemote {
		t.Error("AllowRemote should be false after WithRemoteDisabled()")
	}
}

func TestWithStrictVersions(t *testing.T) {
	config := DefaultStateResolverConfig()
	if config.StrictVersions {
		t.Fatal("Default should have StrictVersions = false")
	}

	opt := WithStrictVersions()
	opt(config)

	if !config.StrictVersions {
		t.Error("StrictVersions should be true after WithStrictVersions()")
	}
}

func TestStateResolver_GetExecutor(t *testing.T) {
	resolver, err := NewStateResolver(nil)
	if err != nil {
		t.Fatalf("NewStateResolver() error = %v", err)
	}

	executor := resolver.GetExecutor()
	if executor == nil {
		t.Error("GetExecutor() returned nil")
	}
}

func TestStateResolver_GetLoader(t *testing.T) {
	resolver, err := NewStateResolver(nil)
	if err != nil {
		t.Fatalf("NewStateResolver() error = %v", err)
	}

	loader := resolver.GetLoader()
	if loader == nil {
		t.Error("GetLoader() returned nil")
	}
}

func TestStateResolver_Close(t *testing.T) {
	resolver, err := NewStateResolver(nil)
	if err != nil {
		t.Fatalf("NewStateResolver() error = %v", err)
	}

	err = resolver.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// After close, applied blueprints should be cleared
	applied := resolver.GetAppliedBlueprints()
	if len(applied) != 0 {
		t.Errorf("GetAppliedBlueprints() after close = %d, want 0", len(applied))
	}
}

func TestStateResolver_GetAppliedBlueprints(t *testing.T) {
	resolver, err := NewStateResolver(nil)
	if err != nil {
		t.Fatalf("NewStateResolver() error = %v", err)
	}

	// Initially empty
	applied := resolver.GetAppliedBlueprints()
	if len(applied) != 0 {
		t.Errorf("GetAppliedBlueprints() initially = %d, want 0", len(applied))
	}
}

func TestStateResolver_ValidateBlueprints_NoIncludes(t *testing.T) {
	resolver, err := NewStateResolver(nil)
	if err != nil {
		t.Fatalf("NewStateResolver() error = %v", err)
	}
	defer resolver.Close()

	ctx := context.Background()

	// Create a mock state file with no blueprint includes
	// This would normally come from parsing, but for testing we create directly
	// Since StateFile is from statemgmt, we'd need to import and create one
	// For now, we test that the resolver was created successfully
	_ = ctx
}

func TestStateResolver_ResolveDependencies_NotFound(t *testing.T) {
	resolver, err := NewStateResolver(nil)
	if err != nil {
		t.Fatalf("NewStateResolver() error = %v", err)
	}
	defer resolver.Close()

	ctx := context.Background()

	// Try to resolve dependencies for a non-existent blueprint
	_, err = resolver.ResolveDependencies(ctx, "nonexistent/blueprint", "1.0.0")
	if err == nil {
		t.Error("ResolveDependencies() should return error for non-existent blueprint")
	}
}
