package statemgmt

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// ============================================================================
// Git Module Tests
// ============================================================================

func TestNewGitModule(t *testing.T) {
	m := NewGitModule()

	if m.Name() != "git" {
		t.Errorf("expected name 'git', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent", "latest"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestGitModule_Check_MissingDest(t *testing.T) {
	m := NewGitModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "git",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "dest parameter is required" {
		t.Errorf("expected dest required error, got: %v", err)
	}
}

func TestGitModule_Check_MissingRepo(t *testing.T) {
	m := NewGitModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "git",
		State:  "present",
		Parameters: map[string]interface{}{
			"dest": "/tmp/test-repo",
		},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "repo parameter is required for state present" {
		t.Errorf("expected repo required error, got: %v", err)
	}
}

func TestGitModule_Check_AbsentDoesntNeedRepo(t *testing.T) {
	m := NewGitModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "git",
		State:  "absent",
		Parameters: map[string]interface{}{
			"dest": "/nonexistent/path",
		},
	}

	result, err := m.Check(nil, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected Present to be false")
	}
	if !result.Matches {
		t.Error("expected Matches to be true for absent state on non-existent path")
	}
}

func TestGitModule_Check_NonGitDirectory(t *testing.T) {
	// Create a temp directory that's not a git repo
	tmpDir := t.TempDir()

	m := NewGitModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "git",
		State:  "present",
		Parameters: map[string]interface{}{
			"dest": tmpDir,
			"repo": "https://example.com/repo.git",
		},
	}

	result, err := m.Check(nil, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected Present to be false for non-git directory")
	}
	if result.Metadata["exists"] != true {
		t.Error("expected exists metadata to be true")
	}
	if result.Metadata["is_git_repo"] != false {
		t.Error("expected is_git_repo metadata to be false")
	}
}

func TestGitModule_Test_Valid(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	m := NewGitModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "git",
		State:  "present",
		Parameters: map[string]interface{}{
			"dest": "/tmp/test-repo",
			"repo": "https://github.com/example/repo.git",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
}

func TestGitModule_Test_MissingGit(t *testing.T) {
	// This test is tricky since we can't easily remove git from PATH
	// Just verify the Test method exists and handles parameters
	m := NewGitModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "git",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if result.Success {
		t.Error("expected failure for missing dest parameter")
	}
}

func TestGitModule_Integration_CloneAndUpdate(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Create a bare repo to clone from
	tmpDir := t.TempDir()
	bareRepo := filepath.Join(tmpDir, "bare.git")
	workRepo := filepath.Join(tmpDir, "work")

	// Initialize bare repo
	cmd := exec.Command("git", "init", "--bare", bareRepo)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to create bare repo: %v", err)
	}

	// Create a work repo and push initial commit
	cmd = exec.Command("git", "clone", bareRepo, workRepo)
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to clone bare repo: %v", err)
	}

	// Configure git user in work repo
	cmd = exec.Command("git", "-C", workRepo, "config", "user.email", "test@test.com")
	cmd.Run()
	cmd = exec.Command("git", "-C", workRepo, "config", "user.name", "Test User")
	cmd.Run()

	// Create initial commit
	testFile := filepath.Join(workRepo, "README.md")
	if err := os.WriteFile(testFile, []byte("# Test"), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}
	cmd = exec.Command("git", "-C", workRepo, "add", ".")
	cmd.Run()
	cmd = exec.Command("git", "-C", workRepo, "commit", "-m", "Initial commit")
	if err := cmd.Run(); err != nil {
		t.Fatalf("Failed to commit: %v", err)
	}
	cmd = exec.Command("git", "-C", workRepo, "push", "origin", "master")
	// Push might fail if branch is main, try both
	if err := cmd.Run(); err != nil {
		cmd = exec.Command("git", "-C", workRepo, "push", "origin", "main")
		cmd.Run()
	}

	// Now test cloning to a new location
	cloneDest := filepath.Join(tmpDir, "clone")
	m := NewGitModule()

	decl := &StateDeclaration{
		ID:     "test-clone",
		Module: "git",
		State:  "present",
		Parameters: map[string]interface{}{
			"dest": cloneDest,
			"repo": bareRepo,
		},
	}

	// Check should show not present
	checkResult, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if checkResult.Present {
		t.Error("expected repository to not be present before clone")
	}

	// Apply should clone
	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Check should now show present
	checkResult, err = m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed after clone: %v", err)
	}
	if !checkResult.Present {
		t.Error("expected repository to be present after clone")
	}
	if !checkResult.Matches {
		t.Error("expected Matches to be true")
	}

	// Verify .git directory exists
	if _, err := os.Stat(filepath.Join(cloneDest, ".git")); os.IsNotExist(err) {
		t.Error("expected .git directory to exist")
	}
}

func TestGitModule_Apply_Absent(t *testing.T) {
	// Create a temp directory with a git repo
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	os.MkdirAll(filepath.Join(repoDir, ".git"), 0755)

	m := NewGitModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "git",
		State:  "absent",
		Parameters: map[string]interface{}{
			"dest": repoDir,
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Verify directory is removed
	if _, err := os.Stat(repoDir); !os.IsNotExist(err) {
		t.Error("expected repository directory to be removed")
	}
}

// ============================================================================
// Git Config Module Tests
// ============================================================================

func TestNewGitConfigModule(t *testing.T) {
	m := NewGitConfigModule()

	if m.Name() != "git_config" {
		t.Errorf("expected name 'git_config', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestGitConfigModule_Check_MissingName(t *testing.T) {
	m := NewGitConfigModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "git_config",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(nil, decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestGitConfigModule_Test_Valid(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	m := NewGitConfigModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "git_config",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":  "user.email",
			"value": "test@example.com",
			"scope": "global",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
}

func TestGitConfigModule_Test_InvalidScope(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	m := NewGitConfigModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "git_config",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":  "user.email",
			"value": "test@example.com",
			"scope": "invalid",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if result.Success {
		t.Error("expected failure for invalid scope")
	}
}

func TestGitConfigModule_Test_MissingValue(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	m := NewGitConfigModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "git_config",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":  "user.email",
			"scope": "global",
		},
	}

	result, err := m.Test(nil, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if result.Success {
		t.Error("expected failure for missing value")
	}
}

func TestGitConfigModule_Integration_SetAndUnset(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Use a temp file for config
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, ".gitconfig")

	m := NewGitConfigModule()

	// Set a config value
	setDecl := &StateDeclaration{
		ID:     "test-set",
		Module: "git_config",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":  "kscore.test.value",
			"value": "test123",
			"file":  configFile,
		},
	}

	result, err := m.Apply(context.Background(), setDecl)
	if err != nil {
		t.Fatalf("Apply set failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Check should show present and matching
	checkResult, err := m.Check(context.Background(), setDecl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !checkResult.Present {
		t.Error("expected config to be present")
	}
	if !checkResult.Matches {
		t.Error("expected config to match")
	}
	if checkResult.Metadata["current_value"] != "test123" {
		t.Errorf("expected current_value to be 'test123', got: %v", checkResult.Metadata["current_value"])
	}

	// Unset the config value
	unsetDecl := &StateDeclaration{
		ID:     "test-unset",
		Module: "git_config",
		State:  "absent",
		Parameters: map[string]interface{}{
			"name": "kscore.test.value",
			"file": configFile,
		},
	}

	result, err = m.Apply(context.Background(), unsetDecl)
	if err != nil {
		t.Fatalf("Apply unset failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Check should show not present
	checkResult, err = m.Check(context.Background(), unsetDecl)
	if err != nil {
		t.Fatalf("Check after unset failed: %v", err)
	}
	if checkResult.Present {
		t.Error("expected config to not be present after unset")
	}
}

func TestGitConfigModule_Integration_UpdateValue(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	// Use a temp file for config
	tmpDir := t.TempDir()
	configFile := filepath.Join(tmpDir, ".gitconfig")

	m := NewGitConfigModule()

	// Set initial value
	decl := &StateDeclaration{
		ID:     "test",
		Module: "git_config",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":  "kscore.update.test",
			"value": "initial",
			"file":  configFile,
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply initial failed: %v", err)
	}
	if !result.Success || !result.Changed {
		t.Errorf("expected success and changed, got: %s", result.Comment)
	}

	// Update the value
	decl.Parameters["value"] = "updated"

	// Check should show not matching
	checkResult, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if !checkResult.Present {
		t.Error("expected config to be present")
	}
	if checkResult.Matches {
		t.Error("expected config to not match")
	}

	// Apply should update
	result, err = m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply update failed: %v", err)
	}
	if !result.Success {
		t.Errorf("expected success, got: %s", result.Comment)
	}
	if !result.Changed {
		t.Error("expected Changed to be true")
	}

	// Check should now match
	checkResult, err = m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check after update failed: %v", err)
	}
	if !checkResult.Matches {
		t.Error("expected config to match after update")
	}
	if checkResult.Metadata["current_value"] != "updated" {
		t.Errorf("expected current_value to be 'updated', got: %v", checkResult.Metadata["current_value"])
	}
}

func TestGetGitConfigPath(t *testing.T) {
	// Test system path
	sysPath := getGitConfigPath("system")
	if sysPath == "" {
		t.Error("expected non-empty system config path")
	}

	// Test global path
	globalPath := getGitConfigPath("global")
	if globalPath == "" {
		t.Error("expected non-empty global config path")
	}

	// Test local path (should be empty as it depends on working directory)
	localPath := getGitConfigPath("local")
	if localPath != "" {
		t.Errorf("expected empty local config path, got: %s", localPath)
	}
}
