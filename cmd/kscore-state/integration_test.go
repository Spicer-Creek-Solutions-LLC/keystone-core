// Package main implements the kscore-state CLI for declarative state management.
package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIIntegration tests the end-to-end CLI workflow
func TestCLIIntegration(t *testing.T) {
	ctx := context.Background()

	// Build the CLI binary
	binPath := filepath.Join(t.TempDir(), "kscore-state")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\n%s", err, output)
	}

	// Create test state file
	testDir := t.TempDir()
	stateFile := filepath.Join(testDir, "test.yaml")
	testFile := filepath.Join(testDir, "test.txt")

	stateContent := `metadata:
  description: Integration test state file

file:
  ` + testFile + `:
    state: present
    contents: "Integration test content"
    mode: "0644"
`

	if err := os.WriteFile(stateFile, []byte(stateContent), 0644); err != nil {
		t.Fatalf("Failed to create state file: %v", err)
	}

	// Test 1: Check command (dry-run)
	t.Run("check_command", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "check", stateFile)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Check command failed: %v\n%s", err, output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "would change") {
			t.Error("Expected 'would change' in check output")
		}

		if !strings.Contains(outputStr, "✓ Success!") {
			t.Error("Expected success message in check output")
		}

		// File should not exist yet
		if _, err := os.Stat(testFile); !os.IsNotExist(err) {
			t.Error("File should not exist after check command")
		}
	})

	// Test 2: Apply command
	t.Run("apply_command", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "apply", stateFile)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Apply command failed: %v\n%s", err, output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "changed") {
			t.Error("Expected 'changed' in apply output")
		}

		if !strings.Contains(outputStr, "✓ Success!") {
			t.Error("Expected success message in apply output")
		}

		// File should exist now
		content, err := os.ReadFile(testFile)
		if err != nil {
			t.Fatalf("File was not created: %v", err)
		}

		if string(content) != "Integration test content" {
			t.Errorf("Expected 'Integration test content', got '%s'", string(content))
		}
	})

	// Test 3: Drift command (no drift)
	t.Run("drift_no_drift", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "drift", stateFile)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Drift command failed: %v\n%s", err, output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "No drift detected") {
			t.Error("Expected 'No drift detected' in output")
		}

		if !strings.Contains(outputStr, "Overall severity: none") {
			t.Error("Expected 'Overall severity: none' in output")
		}
	})

	// Test 4: Drift command (with drift)
	t.Run("drift_with_drift", func(t *testing.T) {
		// Modify the file to create drift
		if err := os.WriteFile(testFile, []byte("Modified!"), 0644); err != nil {
			t.Fatalf("Failed to modify file: %v", err)
		}

		cmd := exec.CommandContext(ctx, binPath, "drift", stateFile)
		output, err := cmd.CombinedOutput()
		// Exit code should be 1 when drift is detected
		if err == nil {
			t.Error("Expected non-zero exit code when drift is detected")
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "medium") {
			t.Error("Expected 'medium' severity in output")
		}

		if strings.Contains(outputStr, "No drift detected") {
			t.Error("Should not say 'No drift detected' when drift exists")
		}
	})

	// Test 5: Apply idempotency
	t.Run("apply_idempotent", func(t *testing.T) {
		// First apply
		cmd1 := exec.CommandContext(ctx, binPath, "apply", stateFile)
		if output, err := cmd1.CombinedOutput(); err != nil {
			t.Fatalf("First apply failed: %v\n%s", err, output)
		}

		// Second apply (should be idempotent)
		cmd2 := exec.CommandContext(ctx, binPath, "apply", stateFile)
		output, err := cmd2.CombinedOutput()
		if err != nil {
			t.Fatalf("Second apply failed: %v\n%s", err, output)
		}

		outputStr := string(output)
		if !strings.Contains(outputStr, "unchanged") {
			t.Error("Expected 'unchanged' in second apply output (idempotency)")
		}
	})
}

// TestCLICompile tests the compile command end-to-end
func TestCLICompile(t *testing.T) {
	ctx := context.Background()

	binPath := filepath.Join(t.TempDir(), "kscore-state")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\n%s", err, output)
	}

	testDir := t.TempDir()
	testFile := filepath.Join(testDir, "managed.txt")
	stateFile := filepath.Join(testDir, "compile-test.yaml")
	stateContent := `metadata:
  name: compile-test
  description: Compile integration test

file:
  ` + testFile + `:
    state: present
    contents: "compiled content"
    mode: "0644"
`
	if err := os.WriteFile(stateFile, []byte(stateContent), 0644); err != nil {
		t.Fatalf("Failed to create state file: %v", err)
	}

	t.Run("compile_yaml", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "compile", stateFile)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Compile command failed: %v\n%s", err, output)
		}
		outputStr := string(output)
		if !strings.Contains(outputStr, "compile-test") {
			t.Error("Expected state name in YAML compile output")
		}
		if !strings.Contains(outputStr, "present") {
			t.Error("Expected state value in YAML compile output")
		}
	})

	t.Run("compile_json", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "compile", stateFile, "--output", "json")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Compile JSON command failed: %v\n%s", err, output)
		}
		outputStr := string(output)
		if !strings.Contains(outputStr, `"compile-test"`) {
			t.Error("Expected state name in JSON compile output")
		}
	})
}

// TestCLIDriftFix tests the drift --fix command end-to-end
func TestCLIDriftFix(t *testing.T) {
	ctx := context.Background()

	binPath := filepath.Join(t.TempDir(), "kscore-state")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\n%s", err, output)
	}

	testDir := t.TempDir()
	testFile := filepath.Join(testDir, "drift-fix.txt")
	stateFile := filepath.Join(testDir, "drift-fix.yaml")
	stateContent := `metadata:
  description: Drift fix test

file:
  ` + testFile + `:
    state: present
    contents: "expected content"
    mode: "0644"
`
	if err := os.WriteFile(stateFile, []byte(stateContent), 0644); err != nil {
		t.Fatalf("Failed to create state file: %v", err)
	}

	// First apply to create the file
	applyCmd := exec.CommandContext(ctx, binPath, "apply", stateFile)
	if output, err := applyCmd.CombinedOutput(); err != nil {
		t.Fatalf("Initial apply failed: %v\n%s", err, output)
	}

	// Create drift by modifying the file
	if err := os.WriteFile(testFile, []byte("drifted content"), 0644); err != nil {
		t.Fatalf("Failed to create drift: %v", err)
	}

	// Run drift --fix to auto-remediate
	fixCmd := exec.CommandContext(ctx, binPath, "drift", stateFile, "--fix")
	output, err := fixCmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Drift --fix command failed: %v\n%s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Auto-Remediation") {
		t.Error("Expected 'Auto-Remediation' in drift --fix output")
	}

	// Verify the file was restored
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file after fix: %v", err)
	}
	if string(content) != "expected content" {
		t.Errorf("Expected 'expected content' after fix, got '%s'", string(content))
	}
}

// TestCLIExportRestore tests the export and restore commands
func TestCLIExportRestore(t *testing.T) {
	ctx := context.Background()

	binPath := filepath.Join(t.TempDir(), "kscore-state")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\n%s", err, output)
	}

	t.Run("export_runs", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "export")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Export command failed: %v\n%s", err, output)
		}
		outputStr := string(output)
		if !strings.Contains(outputStr, "Exporting current state") {
			t.Error("Expected export header in output")
		}
	})

	t.Run("restore_missing_input", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "restore")
		output, _ := cmd.CombinedOutput()
		outputStr := string(output)
		if !strings.Contains(outputStr, "required") {
			t.Error("Expected required flag error for missing --input")
		}
	})

	t.Run("restore_nonexistent_file", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "restore", "--input", "/nonexistent/file.yaml")
		output, _ := cmd.CombinedOutput()
		outputStr := string(output)
		if !strings.Contains(outputStr, "not found") {
			t.Errorf("Expected 'not found' error message, got: %s", outputStr)
		}
	})

	t.Run("restore_existing_file", func(t *testing.T) {
		tmpFile := filepath.Join(t.TempDir(), "export.yaml")
		if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}
		cmd := exec.CommandContext(ctx, binPath, "restore", "--input", tmpFile)
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Restore command failed: %v\n%s", err, output)
		}
		outputStr := string(output)
		if !strings.Contains(outputStr, "Restoring state from") {
			t.Error("Expected restore header in output")
		}
	})
}

// TestCLIVars tests the vars subcommands
func TestCLIVarsIntegration(t *testing.T) {
	ctx := context.Background()

	binPath := filepath.Join(t.TempDir(), "kscore-state")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\n%s", err, output)
	}

	t.Run("vars_get", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "vars", "get", "http_port")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Vars get command failed: %v\n%s", err, output)
		}
		outputStr := string(output)
		if !strings.Contains(outputStr, "http_port") {
			t.Error("Expected variable key in output")
		}
	})

	t.Run("vars_list", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "vars", "list")
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("Vars list command failed: %v\n%s", err, output)
		}
		outputStr := string(output)
		if !strings.Contains(outputStr, "KEY") {
			t.Error("Expected table header in output")
		}
	})

	t.Run("vars_get_agent_scope_missing_agent", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "vars", "get", "key", "--scope", "agent")
		output, _ := cmd.CombinedOutput()
		outputStr := string(output)
		if !strings.Contains(outputStr, "--agent is required") {
			t.Errorf("Expected '--agent is required' error, got: %s", outputStr)
		}
	})

	t.Run("vars_list_role_scope_missing_role", func(t *testing.T) {
		cmd := exec.CommandContext(ctx, binPath, "vars", "list", "--scope", "role")
		output, _ := cmd.CombinedOutput()
		outputStr := string(output)
		if !strings.Contains(outputStr, "--role is required") {
			t.Errorf("Expected '--role is required' error, got: %s", outputStr)
		}
	})
}

// TestCLIVersion tests the version command
func TestCLIVersion(t *testing.T) {
	ctx := context.Background()

	// Build the CLI binary
	binPath := filepath.Join(t.TempDir(), "kscore-state")
	buildCmd := exec.CommandContext(ctx, "go", "build", "-o", binPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\n%s", err, output)
	}

	cmd := exec.CommandContext(ctx, binPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Version command failed: %v\n%s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "Keystone Core") {
		t.Error("Expected Keystone Core version information in output")
	}
}
