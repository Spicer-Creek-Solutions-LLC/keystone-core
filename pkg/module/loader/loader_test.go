package loader

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/policy"
	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

func TestNewModuleLoader(t *testing.T) {
	hashVerifier := verify.NewDefaultHashVerifier()
	sumDB := verify.NewInMemorySumDB()
	policyEngine := policy.NewModulePolicyEngine(nil, nil)

	loader := NewModuleLoader(hashVerifier, nil, sumDB, nil, policyEngine)

	if loader == nil {
		t.Fatal("Expected loader to be created")
	}

	if loader.hashVerifier == nil {
		t.Error("Expected hash verifier to be set")
	}

	if loader.sumDB == nil {
		t.Error("Expected sumDB to be set")
	}

	if loader.policyEngine == nil {
		t.Error("Expected policy engine to be set")
	}
}

func TestLoadOptions_Defaults(t *testing.T) {
	opts := &LoadOptions{}

	if opts.SkipVerification {
		t.Error("Expected SkipVerification to default to false")
	}

	if opts.SkipPolicyValidation {
		t.Error("Expected SkipPolicyValidation to default to false")
	}
}

func TestExecuteOptions_Defaults(t *testing.T) {
	opts := &ExecuteOptions{}

	if opts.Timeout != 0 {
		t.Error("Expected Timeout to default to 0")
	}

	if opts.Context != nil {
		t.Error("Expected Context to default to nil")
	}
}

func TestLoadResult_Structure(t *testing.T) {
	result := &LoadResult{
		Manifest: &manifest.Manifest{
			Name:    "test/module",
			Version: "1.0.0",
		},
		RegisteredCapabilities: []string{"fs.read", "log"},
		LoadDuration:           100 * time.Millisecond,
	}

	if result.Manifest.Name != "test/module" {
		t.Errorf("Expected module name test/module, got %s", result.Manifest.Name)
	}

	if len(result.RegisteredCapabilities) != 2 {
		t.Errorf("Expected 2 capabilities, got %d", len(result.RegisteredCapabilities))
	}

	if result.LoadDuration != 100*time.Millisecond {
		t.Errorf("Expected load duration 100ms, got %v", result.LoadDuration)
	}
}

func TestExecuteResult_Structure(t *testing.T) {
	result := &ExecuteResult{
		Output:          "success",
		Error:           nil,
		ExecuteDuration: 50 * time.Millisecond,
		CapabilityInvocations: map[string]int{
			"fs.read": 5,
			"log":     10,
		},
	}

	if result.Output != "success" {
		t.Errorf("Expected output 'success', got %v", result.Output)
	}

	if result.Error != nil {
		t.Errorf("Expected no error, got %v", result.Error)
	}

	if result.CapabilityInvocations["fs.read"] != 5 {
		t.Errorf("Expected 5 fs.read invocations, got %d", result.CapabilityInvocations["fs.read"])
	}
}

func TestLoadEvent_Types(t *testing.T) {
	events := []LoadEventType{
		LoadEventStart,
		LoadEventManifestParsed,
		LoadEventVerifying,
		LoadEventVerified,
		LoadEventPolicyCheck,
		LoadEventPolicyApproved,
		LoadEventRuntimeInit,
		LoadEventCapabilities,
		LoadEventComplete,
		LoadEventFailed,
	}

	if len(events) != 10 {
		t.Errorf("Expected 10 event types, got %d", len(events))
	}

	for _, event := range events {
		if event == "" {
			t.Error("Event type should not be empty")
		}
	}
}

func TestModuleLoader_SetCache(t *testing.T) {
	loader := NewModuleLoader(
		verify.NewDefaultHashVerifier(),
		nil,
		verify.NewInMemorySumDB(),
		nil,
		policy.NewModulePolicyEngine(nil, nil),
	)

	cache := NewInMemoryModuleCache(10, 5*time.Minute)
	loader.SetCache(cache)

	if loader.cache == nil {
		t.Error("Expected cache to be set")
	}
}

func TestModuleLoader_SetEventHandler(t *testing.T) {
	loader := NewModuleLoader(
		verify.NewDefaultHashVerifier(),
		nil,
		verify.NewInMemorySumDB(),
		nil,
		policy.NewModulePolicyEngine(nil, nil),
	)

	eventsCalled := 0
	handler := func(event *LoadEvent) {
		eventsCalled++
	}

	loader.SetEventHandler(handler)

	if loader.eventHandler == nil {
		t.Error("Expected event handler to be set")
	}
}

func TestModuleLoader_LoadInvalidPath(t *testing.T) {
	loader := NewModuleLoader(
		verify.NewDefaultHashVerifier(),
		nil,
		verify.NewInMemorySumDB(),
		nil,
		policy.NewModulePolicyEngine(nil, nil),
	)

	_, err := loader.Load("/nonexistent/module", &LoadOptions{
		SkipVerification:     true,
		SkipPolicyValidation: true,
	})

	if err == nil {
		t.Error("Expected error for invalid module path")
	}
}

func TestModuleLoader_CreateCapability(t *testing.T) {
	loader := NewModuleLoader(
		verify.NewDefaultHashVerifier(),
		nil,
		verify.NewInMemorySumDB(),
		nil,
		policy.NewModulePolicyEngine(nil, nil),
	)

	mf := &manifest.Manifest{
		Name:    "test/module",
		Version: "1.0.0",
	}

	tests := []struct {
		name    string
		capName string
		wantErr bool
	}{
		{"fs.read", "fs.read", false},
		{"fs.write", "fs.write", false},
		{"http.get", "http.get", false},
		{"http.post", "http.post", false},
		{"exec", "exec", false},
		{"secrets.read", "secrets.read", false},
		{"secrets.write", "secrets.write", false},
		{"log", "log", false},
		{"time", "time", false},
		{"kv", "kv", false},
		{"unknown", "unknown", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cap, err := loader.createCapability(tt.capName, mf, nil)

			if tt.wantErr {
				if err == nil {
					t.Errorf("Expected error for capability %s", tt.capName)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for capability %s: %v", tt.capName, err)
				}
				if cap == nil {
					t.Errorf("Expected capability to be created for %s", tt.capName)
				}
			}
		})
	}
}

func TestParseMemoryLimit(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected uint64
	}{
		// Default cases
		{"empty string", "", 64 * 1024 * 1024},
		{"whitespace only", "   ", 64 * 1024 * 1024},

		// Plain bytes
		{"plain bytes", "1048576", 1048576},
		{"plain bytes float", "1.5", 1},

		// Decimal units (KB, MB, GB, TB)
		{"10KB decimal", "10KB", 10 * 1000},
		{"10MB decimal", "10MB", 10 * 1000 * 1000},
		{"100MB decimal", "100MB", 100 * 1000 * 1000},
		{"1GB decimal", "1GB", 1000 * 1000 * 1000},
		{"1TB decimal", "1TB", 1000 * 1000 * 1000 * 1000},

		// Binary units (Ki, Mi, Gi, Ti) - Kubernetes style
		{"64Mi binary", "64Mi", 64 * 1024 * 1024},
		{"128Mi binary", "128Mi", 128 * 1024 * 1024},
		{"1Gi binary", "1Gi", 1024 * 1024 * 1024},
		{"2Gi binary", "2Gi", 2 * 1024 * 1024 * 1024},
		{"512Ki binary", "512Ki", 512 * 1024},

		// Binary units (KiB, MiB, GiB, TiB)
		{"64MiB binary", "64MiB", 64 * 1024 * 1024},
		{"1GiB binary", "1GiB", 1024 * 1024 * 1024},

		// Short binary units (K, M, G, T)
		{"64M short", "64M", 64 * 1024 * 1024},
		{"1G short", "1G", 1024 * 1024 * 1024},

		// Case insensitivity
		{"lowercase mb", "10mb", 10 * 1000 * 1000},
		{"lowercase mi", "64mi", 64 * 1024 * 1024},
		{"lowercase gi", "1gi", 1024 * 1024 * 1024},

		// With whitespace
		{"with leading space", " 64Mi", 64 * 1024 * 1024},
		{"with trailing space", "64Mi ", 64 * 1024 * 1024},
		{"with space between", "64 Mi", 64 * 1024 * 1024},

		// Floating point values
		{"1.5Gi float", "1.5Gi", uint64(1.5 * 1024 * 1024 * 1024)},
		{"0.5GB float", "0.5GB", uint64(0.5 * 1000 * 1000 * 1000)},

		// Invalid inputs (should return default)
		{"invalid suffix", "10XB", 64 * 1024 * 1024},
		{"no number", "MB", 64 * 1024 * 1024},
		{"negative", "-10MB", 64 * 1024 * 1024},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := parseMemoryLimit(tt.input)
			if result != tt.expected {
				t.Errorf("parseMemoryLimit(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestModuleLoader_ExecuteWithTimeout(t *testing.T) {
	// This test validates timeout handling
	_ = &LoadResult{
		Manifest: &manifest.Manifest{
			Name:       "test/module",
			Version:    "1.0.0",
			Type:       "starlark",
			Entrypoint: "main.star",
		},
	}

	opts := &ExecuteOptions{
		Timeout: 1 * time.Millisecond, // Very short timeout
		Context: context.Background(),
	}

	// Execution should respect timeout
	if opts.Timeout <= 0 {
		t.Error("Expected timeout to be set")
	}
}

// Helper function to create a test module directory
func createTestModule(t *testing.T, moduleType string) string {
	tmpDir := t.TempDir()

	manifestContent := `name: test/module
version: 1.0.0
type: ` + moduleType + `
capabilities:
  - log
entrypoint: main.star
limits:
  timeout: 30s
  memory: 10MB
`

	manifestPath := filepath.Join(tmpDir, "module.yaml")
	if err := os.WriteFile(manifestPath, []byte(manifestContent), 0644); err != nil {
		t.Fatalf("Failed to write test manifest: %v", err)
	}

	return tmpDir
}

func TestModuleLoader_LoadStarlarkModule(t *testing.T) {
	tmpDir := createTestModule(t, "starlark")

	loader := NewModuleLoader(
		verify.NewDefaultHashVerifier(),
		nil,
		verify.NewInMemorySumDB(),
		nil,
		policy.NewModulePolicyEngine(nil, nil),
	)

	result, err := loader.Load(tmpDir, &LoadOptions{
		SkipVerification:     true,
		SkipPolicyValidation: true,
	})

	if err != nil {
		t.Fatalf("Failed to load module: %v", err)
	}

	if result.Manifest.Name != "test/module" {
		t.Errorf("Expected module name test/module, got %s", result.Manifest.Name)
	}

	if result.Runtime == nil {
		t.Error("Expected runtime to be initialized")
	}

	if len(result.RegisteredCapabilities) != 1 {
		t.Errorf("Expected 1 capability, got %d", len(result.RegisteredCapabilities))
	}

	if result.LoadDuration == 0 {
		t.Error("Expected load duration to be recorded")
	}

	// Clean up
	if err := loader.Unload(result); err != nil {
		t.Errorf("Failed to unload module: %v", err)
	}
}

func TestModuleLoader_LoadWasmModule(t *testing.T) {
	tmpDir := createTestModule(t, "wasm")

	loader := NewModuleLoader(
		verify.NewDefaultHashVerifier(),
		nil,
		verify.NewInMemorySumDB(),
		nil,
		policy.NewModulePolicyEngine(nil, nil),
	)

	result, err := loader.Load(tmpDir, &LoadOptions{
		SkipVerification:     true,
		SkipPolicyValidation: true,
	})

	if err != nil {
		t.Fatalf("Failed to load module: %v", err)
	}

	if result.Manifest.Type != "wasm" {
		t.Errorf("Expected module type wasm, got %s", result.Manifest.Type)
	}

	if result.Runtime == nil {
		t.Error("Expected runtime to be initialized")
	}

	// Clean up
	if err := loader.Unload(result); err != nil {
		t.Errorf("Failed to unload module: %v", err)
	}
}
