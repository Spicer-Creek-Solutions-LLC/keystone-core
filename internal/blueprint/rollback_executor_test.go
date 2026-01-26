package blueprint

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRollbackExecutor(t *testing.T) {
	tests := []struct {
		name    string
		config  *RollbackExecutorConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "with custom config",
			config: &RollbackExecutorConfig{
				BlueprintPath: "/tmp/blueprints",
				DryRun:        true,
				Timeout:       10 * time.Minute,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor, err := NewRollbackExecutor(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewRollbackExecutor() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && executor == nil {
				t.Error("NewRollbackExecutor() returned nil executor")
			}
		})
	}
}

func TestDefaultRollbackExecutorConfig(t *testing.T) {
	config := DefaultRollbackExecutorConfig()

	if config == nil {
		t.Fatal("DefaultRollbackExecutorConfig() returned nil")
	}

	if config.DryRun != false {
		t.Errorf("Expected DryRun to be false, got %v", config.DryRun)
	}

	if config.Timeout != 5*time.Minute {
		t.Errorf("Expected Timeout to be 5 minutes, got %v", config.Timeout)
	}

	if config.Parameters == nil {
		t.Error("Expected Parameters to be initialized")
	}
}

func TestRollbackExecutor_HasRollbackEntrypoint(t *testing.T) {
	// Create a temp directory for blueprints
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a blueprint with a rollback entrypoint
	blueprintWithRollback := filepath.Join(tmpDir, "test-blueprint")
	if err := os.MkdirAll(blueprintWithRollback, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}

	manifestWithRollback := `
name: test-blueprint
namespace: test
version: "1.0.0"
entrypoints:
  default: states/main.yaml
  rollback: states/rollback.yaml
`
	if err := os.WriteFile(filepath.Join(blueprintWithRollback, "blueprint.yaml"), []byte(manifestWithRollback), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create a blueprint without a rollback entrypoint
	blueprintWithoutRollback := filepath.Join(tmpDir, "no-rollback-blueprint")
	if err := os.MkdirAll(blueprintWithoutRollback, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}

	manifestWithoutRollback := `
name: no-rollback-blueprint
namespace: test
version: "1.0.0"
entrypoints:
  default: states/main.yaml
`
	if err := os.WriteFile(filepath.Join(blueprintWithoutRollback, "blueprint.yaml"), []byte(manifestWithoutRollback), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	executor, err := NewRollbackExecutor(&RollbackExecutorConfig{
		BlueprintPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Test blueprint with rollback entrypoint
	if !executor.HasRollbackEntrypoint("test-blueprint") {
		t.Error("Expected HasRollbackEntrypoint to return true for test-blueprint")
	}

	// Test blueprint without rollback entrypoint
	if executor.HasRollbackEntrypoint("no-rollback-blueprint") {
		t.Error("Expected HasRollbackEntrypoint to return false for no-rollback-blueprint")
	}

	// Test non-existent blueprint
	if executor.HasRollbackEntrypoint("non-existent") {
		t.Error("Expected HasRollbackEntrypoint to return false for non-existent blueprint")
	}
}

func TestRollbackExecutor_GetRollbackEntrypoint(t *testing.T) {
	// Create a temp directory for blueprints
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a blueprint with a rollback entrypoint
	blueprintDir := filepath.Join(tmpDir, "test-blueprint")
	if err := os.MkdirAll(blueprintDir, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}

	manifest := `
name: test-blueprint
namespace: test
version: "1.0.0"
entrypoints:
  default: states/main.yaml
  rollback: states/rollback.yaml
`
	if err := os.WriteFile(filepath.Join(blueprintDir, "blueprint.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	executor, err := NewRollbackExecutor(&RollbackExecutorConfig{
		BlueprintPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	entrypoint, err := executor.GetRollbackEntrypoint("test-blueprint")
	if err != nil {
		t.Fatalf("GetRollbackEntrypoint failed: %v", err)
	}

	if entrypoint != "states/rollback.yaml" {
		t.Errorf("Expected entrypoint to be 'states/rollback.yaml', got '%s'", entrypoint)
	}
}

func TestRollbackExecutor_ExecuteStateRestore(t *testing.T) {
	executor, err := NewRollbackExecutor(&RollbackExecutorConfig{
		BlueprintPath: "/tmp/blueprints",
		DryRun:        true,
	})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Create a snapshot with various state captures
	snapshot := &Snapshot{
		ID:               "test-snapshot-123",
		AgentID:          "agent-1",
		BlueprintName:    "test/web-stack",
		BlueprintVersion: "1.0.0",
		Namespace:        "default",
		CreatedAt:        time.Now(),
		StateCapture: &StateCapture{
			Files: []FileCaptureEntry{
				{Path: "/etc/nginx/nginx.conf", Exists: true, Mode: 0644, Owner: "root", Group: "root"},
				{Path: "/tmp/old-file.txt", Exists: false},
				{Path: "/var/log/app", Exists: true, IsDir: true, Mode: 0755},
				{Path: "/etc/app/link", Exists: true, IsSymlink: true, LinkTarget: "/etc/app/current"},
			},
			Packages: []PackageCaptureEntry{
				{Name: "nginx", Installed: true, Version: "1.18.0"},
				{Name: "old-package", Installed: false},
			},
			Services: []ServiceCaptureEntry{
				{Name: "nginx", Running: true, Enabled: true},
				{Name: "old-service", Running: false, Enabled: false},
			},
			Users: []UserCaptureEntry{
				{Name: "appuser", Exists: true, UID: 1000, GID: 1000, Home: "/home/appuser", Shell: "/bin/bash"},
				{Name: "olduser", Exists: false},
			},
			Groups: []GroupCaptureEntry{
				{Name: "appgroup", Exists: true, GID: 1000},
				{Name: "oldgroup", Exists: false},
			},
		},
	}

	result, err := executor.ExecuteStateRestore(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("ExecuteStateRestore failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected Success to be true")
	}

	if result.Blueprint != "test/web-stack" {
		t.Errorf("Expected Blueprint to be 'test/web-stack', got '%s'", result.Blueprint)
	}

	// Check state counts
	expectedCounts := map[string]int{
		"file":    4,
		"package": 2,
		"service": 2,
		"user":    2,
		"group":   2,
	}

	for module, expectedCount := range expectedCounts {
		actualCount := len(result.ExpandedStates[module])
		if actualCount != expectedCount {
			t.Errorf("Expected %d %s states, got %d", expectedCount, module, actualCount)
		}
	}

	// Verify total states applied
	if result.StatesApplied != 12 {
		t.Errorf("Expected 12 states applied, got %d", result.StatesApplied)
	}
}

func TestRollbackExecutor_ExecuteStateRestore_EmptySnapshot(t *testing.T) {
	executor, err := NewRollbackExecutor(&RollbackExecutorConfig{
		BlueprintPath: "/tmp/blueprints",
	})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Test with nil StateCapture
	snapshot := &Snapshot{
		ID:               "empty-snapshot",
		BlueprintName:    "test/empty",
		BlueprintVersion: "1.0.0",
		StateCapture:     nil,
	}

	result, err := executor.ExecuteStateRestore(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("ExecuteStateRestore failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected Success to be true")
	}

	if result.StatesApplied != 0 {
		t.Errorf("Expected 0 states applied, got %d", result.StatesApplied)
	}
}

func TestRollbackExecutor_ValidateRollbackEntrypoint(t *testing.T) {
	// Create a temp directory for blueprints
	tmpDir, err := os.MkdirTemp("", "blueprint-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a blueprint with a valid rollback entrypoint
	blueprintDir := filepath.Join(tmpDir, "valid-blueprint")
	statesDir := filepath.Join(blueprintDir, "states")
	if err := os.MkdirAll(statesDir, 0755); err != nil {
		t.Fatalf("Failed to create states dir: %v", err)
	}

	manifest := `
name: valid-blueprint
namespace: test
version: "1.0.0"
entrypoints:
  default: states/main.yaml
  rollback: states/rollback.yaml
`
	if err := os.WriteFile(filepath.Join(blueprintDir, "blueprint.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatalf("Failed to write manifest: %v", err)
	}

	// Create the rollback state file
	rollbackState := `
file:
  cleanup_temp:
    path: /tmp/cleanup
    state: absent
`
	if err := os.WriteFile(filepath.Join(statesDir, "rollback.yaml"), []byte(rollbackState), 0644); err != nil {
		t.Fatalf("Failed to write rollback state: %v", err)
	}

	executor, err := NewRollbackExecutor(&RollbackExecutorConfig{
		BlueprintPath: tmpDir,
	})
	if err != nil {
		t.Fatalf("Failed to create executor: %v", err)
	}

	// Test valid rollback entrypoint
	if err := executor.ValidateRollbackEntrypoint("valid-blueprint"); err != nil {
		t.Errorf("ValidateRollbackEntrypoint failed for valid blueprint: %v", err)
	}

	// Test non-existent blueprint
	if err := executor.ValidateRollbackEntrypoint("non-existent"); err == nil {
		t.Error("Expected error for non-existent blueprint")
	}
}

func TestSanitizeID(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/etc/nginx/nginx.conf", "_etc_nginx_nginx_conf"},
		{"simple", "simple"},
		{"with-dash", "with_dash"},
		{"with.dot", "with_dot"},
		{"with spaces", "with_spaces"},
		{"MixedCase123", "MixedCase123"},
		{"under_score", "under_score"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := sanitizeID(tt.input)
			if result != tt.expected {
				t.Errorf("sanitizeID(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestRollbackResult_Fields(t *testing.T) {
	result := &RollbackResult{
		Success:            true,
		Blueprint:          "test/blueprint",
		FromVersion:        "2.0.0",
		ToVersion:          "1.0.0",
		EntrypointExecuted: "states/rollback.yaml",
		StatesApplied:      10,
		StatesChanged:      5,
		StatesFailed:       1,
		Duration:           time.Second * 30,
		Errors:             []string{"error 1", "error 2"},
	}

	if !result.Success {
		t.Error("Expected Success to be true")
	}

	if result.Blueprint != "test/blueprint" {
		t.Errorf("Expected Blueprint to be 'test/blueprint', got '%s'", result.Blueprint)
	}

	if result.FromVersion != "2.0.0" {
		t.Errorf("Expected FromVersion to be '2.0.0', got '%s'", result.FromVersion)
	}

	if result.ToVersion != "1.0.0" {
		t.Errorf("Expected ToVersion to be '1.0.0', got '%s'", result.ToVersion)
	}

	if result.StatesApplied != 10 {
		t.Errorf("Expected StatesApplied to be 10, got %d", result.StatesApplied)
	}

	if len(result.Errors) != 2 {
		t.Errorf("Expected 2 errors, got %d", len(result.Errors))
	}
}
