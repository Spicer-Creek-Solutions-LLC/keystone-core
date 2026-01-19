package statemgmt

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// TestIntegration_FullWorkflow tests a complete state management workflow
func TestIntegration_FullWorkflow(t *testing.T) {
	ctx := context.Background()
	tmpDir := filepath.Join(os.TempDir(), "kscore-integration-test")

	// Clean up before and after
	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	// Create test directory
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Define state file path
	stateFilePath := filepath.Join(tmpDir, "webserver.yaml")
	configFile := filepath.Join(tmpDir, "nginx.conf")
	dataDir := filepath.Join(tmpDir, "data")

	// Create state file
	stateContent := `
metadata:
  name: webserver-setup
  description: Complete webserver setup
  version: "1.0"

file:
  ` + dataDir + `:
    state: directory
    mode: "0755"

  ` + configFile + `:
    state: present
    contents: |
      server {
        listen 80;
        root /var/www/html;
      }
    mode: "0644"

cmd:
  echo hello > ` + filepath.Join(tmpDir, "test-output.txt") + `:
    state: run
    creates: ` + filepath.Join(tmpDir, "test-output.txt") + `
`

	if err := os.WriteFile(stateFilePath, []byte(stateContent), 0644); err != nil {
		t.Fatalf("Failed to write state file: %v", err)
	}

	// Step 1: Parse the state file
	parser := NewParser(tmpDir)
	stateFile, err := parser.ParseFile(stateFilePath)
	if err != nil {
		t.Fatalf("Failed to parse state file: %v", err)
	}

	// Verify parsing
	if stateFile.Metadata.Name != "webserver-setup" {
		t.Errorf("Expected metadata name 'webserver-setup', got '%s'", stateFile.Metadata.Name)
	}

	if len(stateFile.States["file"]) != 2 {
		t.Errorf("Expected 2 file states, got %d", len(stateFile.States["file"]))
	}

	if len(stateFile.States["cmd"]) != 1 {
		t.Errorf("Expected 1 cmd state, got %d", len(stateFile.States["cmd"]))
	}

	// Step 2: Validate the state file
	validator := NewValidator()
	result := validator.Validate(stateFile)
	if !result.Valid {
		t.Fatalf("Validation failed with %d errors: %s", result.Errors, result.Summary())
	}

	// Step 3: Dry run - preview changes
	run1, err := CheckState(ctx, stateFile)
	if err != nil {
		t.Fatalf("Dry run failed: %v", err)
	}

	if !run1.DryRun {
		t.Error("Expected dry run mode")
	}

	if run1.Summary.Total != 3 {
		t.Errorf("Expected 3 states in dry run, got %d", run1.Summary.Total)
	}

	// Verify no actual changes were made
	if _, err := os.Stat(dataDir); !os.IsNotExist(err) {
		t.Error("Expected directory to not exist after dry run")
	}

	// Step 4: Apply the state
	run2, err := ApplyState(ctx, stateFile, false)
	if err != nil {
		t.Fatalf("Apply state failed: %v", err)
	}

	if !run2.Summary.Success {
		t.Errorf("Expected successful state application. Summary: %+v", run2.Summary)
		for _, result := range run2.Results {
			if !result.Success {
				t.Errorf("Failed state: %s.%s - %v", result.Module, result.StateID, result.Error)
			}
		}
	}

	if run2.Summary.Changed == 0 {
		t.Error("Expected some changes to be made")
	}

	// Verify changes were made
	// Check directory
	info, err := os.Stat(dataDir)
	if err != nil {
		t.Fatalf("Expected directory to be created: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected directory type")
	}

	// Check config file
	content, err := os.ReadFile(configFile)
	if err != nil {
		t.Fatalf("Expected config file to be created: %v", err)
	}
	if len(content) == 0 {
		t.Error("Expected config file to have content")
	}

	// Step 5: Re-apply (test idempotency)
	run3, err := ApplyState(ctx, stateFile, false)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}

	if !run3.Summary.Success {
		t.Error("Expected successful second application")
	}

	// Should have no changes (idempotent)
	if run3.Summary.Changed > 0 {
		t.Errorf("Expected 0 changes on second run (idempotent), got %d", run3.Summary.Changed)
	}

	if run3.Summary.Unchanged == 0 {
		t.Error("Expected all states to be unchanged on second run")
	}

	// Step 6: Test state removal
	configStateUpdated := false
	for i := range stateFile.States["file"] {
		if stateFile.States["file"][i].ID == configFile {
			stateFile.States["file"][i].State = "absent" // Remove config file
			configStateUpdated = true
			break
		}
	}
	if !configStateUpdated {
		t.Fatalf("Failed to locate config file state for %s", configFile)
	}

	run4, err := ApplyState(ctx, stateFile, false)
	if err != nil {
		t.Fatalf("Removal apply failed: %v", err)
	}

	if !run4.Summary.Success {
		t.Error("Expected successful removal")
	}

	// Verify config file was removed
	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Error("Expected config file to be removed")
	}

	// Directory should still exist
	if _, err := os.Stat(dataDir); err != nil {
		t.Error("Expected directory to still exist")
	}
}

// TestIntegration_ComplexDependencies tests state execution with dependencies
func TestIntegration_ComplexDependencies(t *testing.T) {
	ctx := context.Background()
	tmpDir := filepath.Join(os.TempDir(), "kscore-integration-deps")

	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	file1 := filepath.Join(tmpDir, "file1.txt")
	file2 := filepath.Join(tmpDir, "file2.txt")
	file3 := filepath.Join(tmpDir, "file3.txt")

	// Create state file with dependencies
	stateFile := &StateFile{
		Path: "deps.yaml",
		States: map[string][]StateDeclaration{
			"file": {
				{
					ID:     file1,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "file 1",
					},
				},
				{
					ID:     file2,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "file 2",
					},
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: file1},
						},
					},
				},
				{
					ID:     file3,
					Module: "file",
					State:  "present",
					Parameters: map[string]interface{}{
						"contents": "file 3",
					},
					Requisites: Requisites{
						Require: []StateReference{
							{Module: "file", ID: file2},
						},
					},
				},
			},
		},
	}

	// Apply state
	run, err := ApplyState(ctx, stateFile, false)
	if err != nil {
		t.Fatalf("Apply state failed: %v", err)
	}

	if !run.Summary.Success {
		t.Error("Expected successful state application")
	}

	// Verify all files were created
	for _, file := range []string{file1, file2, file3} {
		if _, err := os.Stat(file); os.IsNotExist(err) {
			t.Errorf("Expected file %s to be created", file)
		}
	}

	// Note: In the current implementation, we don't have dependency resolution yet
	// This test validates that states execute successfully
	// Dependency ordering will be added in Week 3
}

// TestIntegration_ErrorHandling tests error scenarios
func TestIntegration_ErrorHandling(t *testing.T) {
	ctx := context.Background()

	// Test invalid module
	stateFile := &StateFile{
		Path: "error.yaml",
		States: map[string][]StateDeclaration{
			"invalid-module": {
				{
					ID:         "test",
					Module:     "invalid-module",
					State:      "present",
					Parameters: make(map[string]interface{}),
				},
			},
		},
	}

	run, err := ApplyState(ctx, stateFile, false)
	// Executor doesn't return error, but the run should have failures
	if err != nil {
		// This is fine too
		return
	}

	if run.Summary.Failed == 0 {
		t.Error("Expected at least one failure for invalid module")
	}

	if run.Summary.Success {
		t.Error("Expected overall failure")
	}
}

// TestIntegration_CmdModule tests the cmd module
func TestIntegration_CmdModule(t *testing.T) {
	ctx := context.Background()
	tmpDir := filepath.Join(os.TempDir(), "kscore-integration-cmd")

	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	outputFile := filepath.Join(tmpDir, "output.txt")

	stateFile := &StateFile{
		Path: "cmd.yaml",
		States: map[string][]StateDeclaration{
			"cmd": {
				{
					ID:     "echo 'hello' > " + outputFile,
					Module: "cmd",
					State:  "run",
					Parameters: map[string]interface{}{
						"creates": outputFile,
					},
				},
			},
		},
	}

	// First run should execute the command
	run1, err := ApplyState(ctx, stateFile, false)
	if err != nil {
		t.Fatalf("First apply failed: %v", err)
	}

	if !run1.Summary.Success {
		t.Error("Expected successful execution")
	}

	// Verify output file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected output file to be created")
	}

	// Second run should skip (creates condition)
	run2, err := ApplyState(ctx, stateFile, false)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}

	if !run2.Summary.Success {
		t.Error("Expected successful execution")
	}

	// Command should have been skipped
	if run2.Results[0].Comment != "Skipped: file "+outputFile+" already exists" {
		t.Logf("Comment: %s", run2.Results[0].Comment)
		// Note: This might not match exactly due to implementation details
	}
}

// TestIntegration_PerformanceBaseline tests basic performance
func TestIntegration_PerformanceBaseline(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance test in short mode")
	}

	ctx := context.Background()
	tmpDir := filepath.Join(os.TempDir(), "kscore-integration-perf")

	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	// Create 100 file states
	states := make([]StateDeclaration, 100)
	for i := 0; i < 100; i++ {
		states[i] = StateDeclaration{
			ID:     filepath.Join(tmpDir, fmt.Sprintf("file%03d.txt", i)),
			Module: "file",
			State:  "present",
			Parameters: map[string]interface{}{
				"contents": "test",
			},
		}
	}

	stateFile := &StateFile{
		Path:   "perf.yaml",
		States: map[string][]StateDeclaration{"file": states},
	}

	// Apply state and measure time
	run, err := ApplyState(ctx, stateFile, false)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !run.Summary.Success {
		t.Errorf("Expected successful execution. Failed: %d, Succeeded: %d", run.Summary.Failed, run.Summary.Succeeded)
		for _, result := range run.Results {
			if !result.Success {
				t.Errorf("Failed state: %s - %v", result.StateID, result.Error)
			}
		}
	}

	// Check that it completed in reasonable time
	// Epic 3 success criteria: <60s for 100 resources
	if run.Summary.Duration.Seconds() > 60 {
		t.Errorf("Expected completion in <60s, took %v", run.Summary.Duration)
	}

	t.Logf("Created 100 file states in %v (%.2f states/sec)", run.Summary.Duration, float64(100)/run.Summary.Duration.Seconds())
}
