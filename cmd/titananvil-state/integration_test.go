package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIIntegration tests the end-to-end CLI workflow
func TestCLIIntegration(t *testing.T) {
	// Build the CLI binary
	binPath := filepath.Join(t.TempDir(), "titananvil-state")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
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
		cmd := exec.Command(binPath, "check", stateFile)
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
		cmd := exec.Command(binPath, "apply", stateFile)
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
		cmd := exec.Command(binPath, "drift", stateFile)
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

		cmd := exec.Command(binPath, "drift", stateFile)
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
		cmd1 := exec.Command(binPath, "apply", stateFile)
		if output, err := cmd1.CombinedOutput(); err != nil {
			t.Fatalf("First apply failed: %v\n%s", err, output)
		}

		// Second apply (should be idempotent)
		cmd2 := exec.Command(binPath, "apply", stateFile)
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

// TestCLIVersion tests the version command
func TestCLIVersion(t *testing.T) {
	// Build the CLI binary
	binPath := filepath.Join(t.TempDir(), "titananvil-state")
	buildCmd := exec.Command("go", "build", "-o", binPath, ".")
	if output, err := buildCmd.CombinedOutput(); err != nil {
		t.Fatalf("Failed to build CLI: %v\n%s", err, output)
	}

	cmd := exec.Command(binPath, "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("Version command failed: %v\n%s", err, output)
	}

	outputStr := string(output)
	if !strings.Contains(outputStr, "TitanAnvil") {
		t.Error("Expected TitanAnvil version information in output")
	}
}
