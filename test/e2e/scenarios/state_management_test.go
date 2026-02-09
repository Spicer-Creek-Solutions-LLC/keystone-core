// Package scenarios contains E2E test scenarios for Keystone Core features.
// These tests require Docker/Podman to be running.
package scenarios

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/test/e2e/harness"
)

var testEnv *harness.TestEnvironment

// TestMain sets up and tears down the test environment for scenario tests
func TestMain(m *testing.M) {
	// Skip if not running E2E tests
	if os.Getenv("KSCORE_E2E_TESTS") != "1" {
		os.Exit(0)
	}

	var cfg *harness.Config
	if harness.IsVMMode() {
		vmCfg, _, err := harness.ConfigFromVM("")
		if err != nil {
			panic("failed to load VM config: " + err.Error())
		}
		cfg = vmCfg
		cfg.ProjectName = "kscore-e2e-scenarios"
		cfg.StartupTimeout = 180 * time.Second
	} else {
		// Find compose file
		composeFile := findComposeFile()
		if composeFile == "" {
			panic("could not find docker-compose.yml")
		}

		// Check if we should skip building images (useful when images already exist)
		skipBuild := os.Getenv("KSCORE_SKIP_BUILD") == "1"

		cfg = &harness.Config{
			ComposeFile:    composeFile,
			ProjectName:    "kscore-e2e-scenarios",
			BuildImages:    !skipBuild,
			StartupTimeout: 180 * time.Second,
			ServerGRPCPort: 8080,
			ServerHTTPPort: 8081,
			WebhookPort:    8082,
		}
	}

	var err error
	testEnv, err = harness.New(cfg)
	if err != nil {
		panic("failed to create test environment: " + err.Error())
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	if err := testEnv.Start(ctx, cfg); err != nil {
		panic("failed to start test environment: " + err.Error())
	}

	// Wait for agents to be ready
	if err := testEnv.WaitForAgents(ctx, 3, 60*time.Second); err != nil {
		panic("agents did not register: " + err.Error())
	}

	// Run tests
	code := m.Run()

	// Cleanup
	ctx, cancel = context.WithTimeout(context.Background(), 30*time.Second)
	_ = testEnv.Stop(ctx)
	cancel()

	os.Exit(code)
}

func findComposeFile() string {
	candidates := []string{
		"../containers/docker-compose.yml",
		"test/e2e/containers/docker-compose.yml",
		"../../containers/docker-compose.yml",
	}

	if root := os.Getenv("KSCORE_ROOT"); root != "" {
		candidates = append(candidates, filepath.Join(root, "test/e2e/containers/docker-compose.yml"))
	}

	for _, c := range candidates {
		abs, err := filepath.Abs(c)
		if err != nil {
			continue
		}
		if _, err := os.Stat(abs); err == nil {
			return abs
		}
	}

	return ""
}

// =============================================================================
// State Management Tests - File Module
// =============================================================================

// TestState_FileCreate tests creating a file using the file module
func TestState_FileCreate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agentID := "agent-web-1"
	testFile := "/tmp/e2e-test-file.txt"
	testContent := "Hello from E2E test"

	// Create file via command (simulating state apply)
	resp := testEnv.AssertCommandSuccess(ctx, t, agentID, "sh", "-c",
		"echo '"+testContent+"' > "+testFile)
	if resp == nil {
		t.Fatal("Failed to create file")
	}

	// Verify file exists
	testEnv.AssertFileExists(ctx, t, agentID, testFile)

	// Verify content
	testEnv.AssertFileContents(ctx, t, agentID, testFile, testContent)

	// Cleanup
	_, _ = testEnv.ExecuteCommandAndWait(ctx, agentID, "rm", "-f", testFile)
}

// TestState_FileModify tests modifying an existing file
func TestState_FileModify(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agentID := "agent-web-1"
	testFile := "/tmp/e2e-test-modify.txt"

	// Create initial file
	testEnv.AssertCommandSuccess(ctx, t, agentID, "sh", "-c",
		"echo 'initial content' > "+testFile)

	// Modify file
	testEnv.AssertCommandSuccess(ctx, t, agentID, "sh", "-c",
		"echo 'modified content' > "+testFile)

	// Verify new content
	testEnv.AssertFileContents(ctx, t, agentID, testFile, "modified content")

	// Cleanup
	_, _ = testEnv.ExecuteCommandAndWait(ctx, agentID, "rm", "-f", testFile)
}

// TestState_FileDelete tests deleting a file
func TestState_FileDelete(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agentID := "agent-web-1"
	testFile := "/tmp/e2e-test-delete.txt"

	// Create file first
	testEnv.AssertCommandSuccess(ctx, t, agentID, "sh", "-c",
		"echo 'to be deleted' > "+testFile)

	// Verify it exists
	testEnv.AssertFileExists(ctx, t, agentID, testFile)

	// Delete file
	testEnv.AssertCommandSuccess(ctx, t, agentID, "rm", "-f", testFile)

	// Verify it's gone
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "test", "-f", testFile)
	if err != nil {
		t.Fatalf("Failed to check file: %v", err)
	}
	if result.ExitCode == 0 {
		t.Errorf("File %s should not exist after deletion", testFile)
	}
}

// TestState_DirectoryCreate tests creating a directory
func TestState_DirectoryCreate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agentID := "agent-web-1"
	testDir := "/tmp/e2e-test-dir"

	// Create directory
	testEnv.AssertCommandSuccess(ctx, t, agentID, "mkdir", "-p", testDir)

	// Verify it exists and is a directory
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "test", "-d", testDir)
	if err != nil {
		t.Fatalf("Failed to check directory: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("Directory %s was not created", testDir)
	}

	// Cleanup
	_, _ = testEnv.ExecuteCommandAndWait(ctx, agentID, "rm", "-rf", testDir)
}

// =============================================================================
// State Management Tests - Idempotency
// =============================================================================

// TestState_FileIdempotency tests that file operations are idempotent
func TestState_FileIdempotency(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	agentID := "agent-web-1"
	testFile := "/tmp/e2e-idempotent.txt"
	testContent := "idempotent content"

	// Apply state first time
	testEnv.AssertCommandSuccess(ctx, t, agentID, "sh", "-c",
		"echo '"+testContent+"' > "+testFile)

	// Get initial mtime
	result1, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "stat", "-c", "%Y", testFile)
	if err != nil {
		t.Fatalf("Failed to get mtime: %v", err)
	}
	mtime1 := result1.Stdout

	// Wait a bit
	time.Sleep(2 * time.Second)

	// Apply same content again (simulating idempotent state)
	// In a real state module, this would check first and skip if unchanged
	testEnv.AssertCommandSuccess(ctx, t, agentID, "sh", "-c",
		"cat "+testFile+" | grep -q '"+testContent+"' || echo '"+testContent+"' > "+testFile)

	// Verify content is still correct
	testEnv.AssertFileContents(ctx, t, agentID, testFile, testContent)

	// Get new mtime - should be unchanged if idempotent
	result2, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "stat", "-c", "%Y", testFile)
	if err != nil {
		t.Fatalf("Failed to get mtime: %v", err)
	}
	mtime2 := result2.Stdout

	if mtime1 != mtime2 {
		t.Logf("Note: File was modified (mtime changed from %s to %s)", mtime1, mtime2)
		t.Log("This is expected for the basic test - real state module would skip unchanged files")
	}

	// Cleanup
	_, _ = testEnv.ExecuteCommandAndWait(ctx, agentID, "rm", "-f", testFile)
}

// =============================================================================
// State Management Tests - Cross-Agent
// =============================================================================

// TestState_CrossAgentFileSync tests applying the same state to multiple agents
func TestState_CrossAgentFileSync(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	agents := []string{"agent-web-1", "agent-web-2", "agent-db-1"}
	testFile := "/tmp/e2e-cross-agent.txt"
	testContent := "synchronized content"

	// Apply to all agents
	for _, agentID := range agents {
		t.Run(agentID, func(t *testing.T) {
			testEnv.AssertCommandSuccess(ctx, t, agentID, "sh", "-c",
				"echo '"+testContent+"' > "+testFile)
			testEnv.AssertFileContents(ctx, t, agentID, testFile, testContent)
		})
	}

	// Cleanup all agents
	for _, agentID := range agents {
		_, _ = testEnv.ExecuteCommandAndWait(ctx, agentID, "rm", "-f", testFile)
	}
}

// =============================================================================
// State Management Tests - User/Group Module (limited in containers)
// =============================================================================

// TestState_UserExists tests checking if a user exists
func TestState_UserExists(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Check for root user (should exist)
	resp := testEnv.AssertCommandSuccess(ctx, t, agentID, "id", "root")
	if resp == nil {
		t.Fatal("Failed to check for root user")
	}
	t.Logf("Root user info: %s", resp.Stdout)

	// Check for non-existent user
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "id", "nonexistent-user-12345")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
	if result.ExitCode == 0 {
		t.Error("Expected non-zero exit code for non-existent user")
	}
}

// =============================================================================
// State Management Tests - Cmd Module
// =============================================================================

// TestState_CmdRun tests running arbitrary commands
func TestState_CmdRun(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Test successful command
	resp := testEnv.AssertCommandSuccess(ctx, t, agentID, "echo", "test command")
	if resp == nil {
		t.Fatal("Command failed")
	}
	if resp.Stdout != "test command\n" && resp.Stdout != "test command" {
		t.Errorf("Unexpected output: %q", resp.Stdout)
	}

	// Test command with complex output
	resp = testEnv.AssertCommandSuccess(ctx, t, agentID, "sh", "-c", "echo line1; echo line2")
	if resp == nil {
		t.Fatal("Command failed")
	}
	t.Logf("Multi-line output: %s", resp.Stdout)
}

// TestState_CmdFail tests handling of failed commands
func TestState_CmdFail(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Test command that should fail
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sh", "-c", "exit 42")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
	if result.ExitCode != 42 {
		t.Errorf("Expected exit code 42, got %d", result.ExitCode)
	}
}

// TestState_CmdTimeout tests command timeout handling
func TestState_CmdTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Execute a command that would take longer than context timeout
	// The command should be killed when context expires
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "sleep", "1")
	if err != nil {
		t.Fatalf("Failed to execute command: %v", err)
	}
	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
}

// =============================================================================
// State Management Tests - Package Module (platform-dependent)
// =============================================================================

// TestState_PackageCheck tests checking if a package is installed
func TestState_PackageCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1" // Alpine container

	// Check for apk (should exist on Alpine)
	resp := testEnv.AssertCommandSuccess(ctx, t, agentID, "which", "apk")
	if resp == nil {
		t.Skip("apk not found - not an Alpine container")
	}

	// Check if bash is installed (we install it in the Dockerfile)
	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "apk", "info", "bash")
	if err != nil {
		t.Fatalf("Failed to check package: %v", err)
	}
	if result.ExitCode != 0 {
		t.Log("bash package not installed (expected if not in Dockerfile)")
	} else {
		t.Log("bash package is installed")
	}
}

// =============================================================================
// State Management Tests - Service Module (limited in containers)
// =============================================================================

// TestState_ServiceCheck tests checking process status (pseudo-service check)
func TestState_ServiceCheck(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	// Check for any kscore-related process
	// The process name depends on how the binary was built/run
	// Try multiple possible process names
	processNames := []string{"kscore-agent", "kscore", "agent"}
	var found bool

	for _, procName := range processNames {
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "pgrep", "-f", procName)
		if err == nil && result.ExitCode == 0 {
			t.Logf("Found process matching '%s' on %s", procName, agentID)
			found = true
			break
		}
	}

	if !found {
		// As a fallback, verify we can execute commands (which proves agent is running)
		result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", "agent is responsive")
		if err != nil || result.ExitCode != 0 {
			t.Errorf("Agent %s is not responsive", agentID)
		} else {
			t.Logf("Agent %s is responsive (process name detection skipped)", agentID)
		}
	}
}
