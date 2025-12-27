package plugin

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewDiscovery(t *testing.T) {
	d := NewDiscovery()
	if d == nil {
		t.Fatal("NewDiscovery returned nil")
	}
	if d.plugins == nil {
		t.Error("plugins map not initialized")
	}
	if d.cached {
		t.Error("cached should be false initially")
	}
}

func TestDiscovery_Discover(t *testing.T) {
	// Create temporary directory for test plugins
	tmpDir := t.TempDir()

	// Create mock plugin binaries
	createMockPlugin(t, tmpDir, "kscore-exec")
	createMockPlugin(t, tmpDir, "kscore-state")
	createMockPlugin(t, tmpDir, "kscore-module")

	// Create a non-plugin binary (should be ignored)
	createMockPlugin(t, tmpDir, "other-tool")

	// Create a non-executable file (should be ignored)
	nonExecPath := filepath.Join(tmpDir, "kscore-nonexec")
	if err := os.WriteFile(nonExecPath, []byte("not executable"), 0644); err != nil {
		t.Fatal(err)
	}

	// Set PATH to our temp directory
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpDir)

	// Discover plugins
	d := NewDiscovery()
	if err := d.Discover(); err != nil {
		t.Fatalf("Discover failed: %v", err)
	}

	// Verify cached flag
	if !d.cached {
		t.Error("cached should be true after Discover")
	}

	// Verify correct number of plugins found
	plugins := d.List()
	if len(plugins) != 3 {
		t.Errorf("Expected 3 plugins, got %d", len(plugins))
	}

	// Verify specific plugins
	expectedPlugins := []string{"exec", "state", "module"}
	for _, name := range expectedPlugins {
		if !d.Has(name) {
			t.Errorf("Expected plugin %q not found", name)
		}

		plugin, err := d.Get(name)
		if err != nil {
			t.Errorf("Get(%q) failed: %v", name, err)
		}
		if plugin.Name != name {
			t.Errorf("Plugin name mismatch: got %q, want %q", plugin.Name, name)
		}
		if !strings.Contains(plugin.Path, tmpDir) {
			t.Errorf("Plugin path doesn't contain tmpDir: %s", plugin.Path)
		}
	}

	// Verify non-plugin not found
	if d.Has("other-tool") {
		t.Error("Non-plugin binary should not be discovered")
	}
	if d.Has("nonexec") {
		t.Error("Non-executable file should not be discovered")
	}
}

func TestDiscovery_Get_NotCached(t *testing.T) {
	d := NewDiscovery()
	_, err := d.Get("exec")
	if err == nil {
		t.Error("Get should fail when plugins not discovered")
	}
	if !strings.Contains(err.Error(), "not discovered") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestDiscovery_Get_NotFound(t *testing.T) {
	tmpDir := t.TempDir()
	createMockPlugin(t, tmpDir, "kscore-exec")

	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpDir)

	d := NewDiscovery()
	if err := d.Discover(); err != nil {
		t.Fatal(err)
	}

	_, err := d.Get("nonexistent")
	if err == nil {
		t.Error("Get should fail for non-existent plugin")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("Unexpected error message: %v", err)
	}
}

func TestDiscovery_List_Empty(t *testing.T) {
	// Set PATH to empty directory
	tmpDir := t.TempDir()
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpDir)

	d := NewDiscovery()
	if err := d.Discover(); err != nil {
		t.Fatal(err)
	}

	plugins := d.List()
	if len(plugins) != 0 {
		t.Errorf("Expected 0 plugins, got %d", len(plugins))
	}
}

func TestDiscovery_PathPriority(t *testing.T) {
	// Create two directories with same plugin name
	tmpDir1 := t.TempDir()
	tmpDir2 := t.TempDir()

	createMockPlugin(t, tmpDir1, "kscore-exec")
	createMockPlugin(t, tmpDir2, "kscore-exec")

	// Set PATH with tmpDir1 first (should take priority)
	oldPath := os.Getenv("PATH")
	defer os.Setenv("PATH", oldPath)
	os.Setenv("PATH", tmpDir1+string(filepath.ListSeparator)+tmpDir2)

	d := NewDiscovery()
	if err := d.Discover(); err != nil {
		t.Fatal(err)
	}

	plugin, err := d.Get("exec")
	if err != nil {
		t.Fatal(err)
	}

	// Should use plugin from tmpDir1 (first in PATH)
	if !strings.Contains(plugin.Path, tmpDir1) {
		t.Errorf("Expected plugin from tmpDir1, got: %s", plugin.Path)
	}
}

func TestExecutor_Execute(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a simple echo plugin
	pluginPath := filepath.Join(tmpDir, "kscore-echo")
	pluginScript := `#!/bin/sh
echo "Hello from plugin"
echo "$@"
`
	if err := os.WriteFile(pluginPath, []byte(pluginScript), 0755); err != nil {
		t.Fatal(err)
	}

	plugin := &Plugin{
		Name: "echo",
		Path: pluginPath,
	}

	executor := NewExecutor(plugin)

	// Capture output
	var stdout, stderr strings.Builder

	err := executor.Execute(ExecuteOptions{
		Args:   []string{"arg1", "arg2"},
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	output := stdout.String()
	if !strings.Contains(output, "Hello from plugin") {
		t.Errorf("Expected 'Hello from plugin' in output, got: %s", output)
	}
	if !strings.Contains(output, "arg1 arg2") {
		t.Errorf("Expected 'arg1 arg2' in output, got: %s", output)
	}
}

func TestExecutor_ExecuteWithOutput(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a plugin that writes to both stdout and stderr
	pluginPath := filepath.Join(tmpDir, "kscore-both")
	pluginScript := `#!/bin/sh
echo "stdout message"
echo "stderr message" >&2
`
	if err := os.WriteFile(pluginPath, []byte(pluginScript), 0755); err != nil {
		t.Fatal(err)
	}

	plugin := &Plugin{
		Name: "both",
		Path: pluginPath,
	}

	executor := NewExecutor(plugin)

	stdout, stderr, err := executor.ExecuteWithOutput(context.Background())
	if err != nil {
		t.Fatalf("ExecuteWithOutput failed: %v", err)
	}

	if !strings.Contains(stdout, "stdout message") {
		t.Errorf("Expected 'stdout message' in stdout, got: %s", stdout)
	}
	if !strings.Contains(stderr, "stderr message") {
		t.Errorf("Expected 'stderr message' in stderr, got: %s", stderr)
	}
}

func TestExecutor_ExecuteWithContext_Cancellation(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a plugin that sleeps for a long time
	pluginPath := filepath.Join(tmpDir, "kscore-sleep")
	pluginScript := `#!/bin/sh
sleep 10
`
	if err := os.WriteFile(pluginPath, []byte(pluginScript), 0755); err != nil {
		t.Fatal(err)
	}

	plugin := &Plugin{
		Name: "sleep",
		Path: pluginPath,
	}

	executor := NewExecutor(plugin)

	// Create context with short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	var stdout, stderr strings.Builder

	err := executor.Execute(ExecuteOptions{
		Ctx:    ctx,
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err == nil {
		t.Error("Expected error due to context cancellation")
	}
	if !strings.Contains(err.Error(), "context deadline exceeded") && !strings.Contains(err.Error(), "signal: killed") {
		t.Errorf("Expected context cancellation error, got: %v", err)
	}
}

func TestExecutor_ExecuteWithError(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a plugin that exits with error
	pluginPath := filepath.Join(tmpDir, "kscore-fail")
	pluginScript := `#!/bin/sh
echo "error message" >&2
exit 1
`
	if err := os.WriteFile(pluginPath, []byte(pluginScript), 0755); err != nil {
		t.Fatal(err)
	}

	plugin := &Plugin{
		Name: "fail",
		Path: pluginPath,
	}

	executor := NewExecutor(plugin)

	var stdout, stderr strings.Builder

	err := executor.Execute(ExecuteOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	if err == nil {
		t.Error("Expected error from plugin execution")
	}

	stderrOutput := stderr.String()
	if !strings.Contains(stderrOutput, "error message") {
		t.Errorf("Expected 'error message' in stderr, got: %s", stderrOutput)
	}
}

// Helper function to create mock plugin binary
func createMockPlugin(t *testing.T, dir, name string) {
	t.Helper()

	pluginPath := filepath.Join(dir, name)
	script := `#!/bin/sh
echo "Mock plugin: ` + name + `"
`
	if err := os.WriteFile(pluginPath, []byte(script), 0755); err != nil {
		t.Fatalf("Failed to create mock plugin: %v", err)
	}
}
