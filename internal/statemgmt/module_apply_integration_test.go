package statemgmt

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// writeFakeCmd creates a fake command script that can be controlled via environment variables.
func writeFakeCmd(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0755); err != nil {
		t.Fatalf("Failed to write fake command %s: %v", name, err)
	}
}

// setupFakePath prepends the given directory to PATH for the duration of the test.
func setupFakePath(t *testing.T, fakeDir string) {
	t.Helper()
	origPath := os.Getenv("PATH")
	t.Setenv("PATH", fakeDir+string(os.PathListSeparator)+origPath)
}

// =============================================================================
// Service Module Apply/Test Integration Tests
// This is the primary module test demonstrating the Apply/Test pattern
// =============================================================================

func TestServiceModule_ApplyTest_FullLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Service module uses different backend on Windows")
	}

	tmpDir := t.TempDir()
	serviceState := filepath.Join(tmpDir, "service_state")

	writeFakeCmd(t, tmpDir, "systemctl", `#!/bin/sh
case "$1" in
    "is-active")
        if [ -f "`+serviceState+`" ] && grep -q "running" "`+serviceState+`"; then
            echo "active"
            exit 0
        fi
        echo "inactive"
        exit 3
        ;;
    "is-enabled")
        if [ -f "`+serviceState+`" ] && grep -q "enabled" "`+serviceState+`"; then
            echo "enabled"
            exit 0
        fi
        echo "disabled"
        exit 1
        ;;
    "start")
        echo "running" >> "`+serviceState+`"
        exit 0
        ;;
    "stop")
        sed -i '/running/d' "`+serviceState+`" 2>/dev/null || true
        exit 0
        ;;
    "enable")
        echo "enabled" >> "`+serviceState+`"
        exit 0
        ;;
    "disable")
        sed -i '/enabled/d' "`+serviceState+`" 2>/dev/null || true
        exit 0
        ;;
    "show")
        echo "LoadState=loaded"
        echo "ActiveState=inactive"
        echo "UnitFileState=disabled"
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
`)

	setupFakePath(t, tmpDir)

	module := NewServiceModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "nginx",
		Module: "service",
		State:  "running",
		Parameters: map[string]interface{}{
			"name": "nginx",
		},
	}

	// Step 1: Check initial state (not running)
	check1, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Initial check failed: %v", err)
	}
	if check1.Matches {
		t.Error("Expected initial state to not match (service not running)")
	}

	// Step 2: Apply state (start service)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}
	if !apply1.Changed {
		t.Error("Expected apply to report changes")
	}

	// Step 3: Test should return true (service is now running)
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true after apply")
	}

	// Step 4: Apply again (idempotency check)
	apply2, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
	if !apply2.Success {
		t.Errorf("Expected second apply to succeed: %s", apply2.Comment)
	}
	if apply2.Changed {
		t.Error("Expected second apply to report no changes (idempotent)")
	}

	// Step 5: Stop service
	decl.State = "stopped"
	apply3, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Stop apply failed: %v", err)
	}
	if !apply3.Success {
		t.Errorf("Expected stop to succeed: %s", apply3.Comment)
	}

	// Step 6: Test stopped state
	test2, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test stopped state failed: %v", err)
	}
	if !test2 {
		t.Error("Expected Test to return true for stopped state")
	}
}

func TestServiceModule_ApplyTest_EnableDisable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Service module uses different backend on Windows")
	}

	tmpDir := t.TempDir()
	serviceState := filepath.Join(tmpDir, "service_state")

	writeFakeCmd(t, tmpDir, "systemctl", `#!/bin/sh
case "$1" in
    "is-active")
        echo "inactive"
        exit 3
        ;;
    "is-enabled")
        if [ -f "`+serviceState+`" ] && grep -q "enabled" "`+serviceState+`"; then
            echo "enabled"
            exit 0
        fi
        echo "disabled"
        exit 1
        ;;
    "enable")
        echo "enabled" >> "`+serviceState+`"
        exit 0
        ;;
    "disable")
        sed -i '/enabled/d' "`+serviceState+`" 2>/dev/null || true
        exit 0
        ;;
    "show")
        echo "LoadState=loaded"
        echo "ActiveState=inactive"
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
`)

	setupFakePath(t, tmpDir)

	module := NewServiceModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "docker",
		Module: "service",
		State:  "enabled",
		Parameters: map[string]interface{}{
			"name": "docker",
		},
	}

	// Apply enable
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Enable apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected enable to succeed: %s", apply1.Comment)
	}

	// Test enabled
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test enabled failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true for enabled state")
	}

	// Disable
	decl.State = "disabled"
	apply2, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Disable apply failed: %v", err)
	}
	if !apply2.Success {
		t.Errorf("Expected disable to succeed: %s", apply2.Comment)
	}

	// Test disabled
	test2, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test disabled failed: %v", err)
	}
	if !test2 {
		t.Error("Expected Test to return true for disabled state")
	}
}

// =============================================================================
// Git Module Apply/Test Integration Tests
// =============================================================================

func TestGitModule_ApplyTest_Clone(t *testing.T) {
	tmpDir := t.TempDir()
	repoDir := filepath.Join(tmpDir, "repo")
	cloneMarker := filepath.Join(tmpDir, "clone_marker")

	writeFakeCmd(t, tmpDir, "git", `#!/bin/sh
case "$1" in
    "clone")
        mkdir -p "`+repoDir+`/.git"
        touch "`+cloneMarker+`"
        exit 0
        ;;
    "rev-parse")
        if [ -d "`+repoDir+`/.git" ]; then
            echo "abc123"
            exit 0
        fi
        exit 128
        ;;
    "remote")
        echo "origin"
        exit 0
        ;;
    "fetch")
        exit 0
        ;;
    "pull")
        exit 0
        ;;
    "config")
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
`)

	setupFakePath(t, tmpDir)

	module := NewGitModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "myrepo",
		Module: "git",
		State:  "present",
		Parameters: map[string]interface{}{
			"dest": repoDir,
			"repo": "https://github.com/example/repo.git",
		},
	}

	// Check initial state (repo doesn't exist)
	check1, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Initial check failed: %v", err)
	}
	if check1.Matches {
		t.Error("Expected initial state to not match (repo doesn't exist)")
	}

	// Apply state (clone repo)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify clone happened
	if _, err := os.Stat(cloneMarker); os.IsNotExist(err) {
		t.Error("Expected git clone to be called")
	}

	// Test should return success
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Errorf("Expected Test to match after clone")
	}
}

// =============================================================================
// Docker Container Module Apply/Test Integration Tests
// =============================================================================

func TestDockerContainerModule_ApplyTest_Lifecycle(t *testing.T) {
	tmpDir := t.TempDir()
	containerState := filepath.Join(tmpDir, "container_state")

	writeFakeCmd(t, tmpDir, "docker", `#!/bin/sh
case "$1" in
    "inspect")
        if [ -f "`+containerState+`" ] && grep -q "running" "`+containerState+`"; then
            echo '[{"State":{"Status":"running","Running":true},"Config":{"Image":"nginx:latest"}}]'
            exit 0
        elif [ -f "`+containerState+`" ] && grep -q "stopped" "`+containerState+`"; then
            echo '[{"State":{"Status":"exited","Running":false},"Config":{"Image":"nginx:latest"}}]'
            exit 0
        fi
        echo "Error: No such container: $2" >&2
        exit 1
        ;;
    "ps")
        if [ -f "`+containerState+`" ] && grep -q "running" "`+containerState+`"; then
            echo "mycontainer"
        fi
        exit 0
        ;;
    "run")
        echo "running" > "`+containerState+`"
        echo "container_id_123"
        exit 0
        ;;
    "start")
        echo "running" > "`+containerState+`"
        exit 0
        ;;
    "stop")
        echo "stopped" > "`+containerState+`"
        exit 0
        ;;
    "rm")
        rm -f "`+containerState+`"
        exit 0
        ;;
    "pull")
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
`)

	setupFakePath(t, tmpDir)

	module := NewDockerContainerModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "mycontainer",
		Module: "docker_container",
		State:  "running",
		Parameters: map[string]interface{}{
			"name":  "mycontainer",
			"image": "nginx:latest",
		},
	}

	// Check initial state (container doesn't exist)
	check1, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Initial check failed: %v", err)
	}
	if check1.Matches {
		t.Error("Expected initial state to not match (container doesn't exist)")
	}

	// Apply state (create and run container)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Test should return success (Docker Test returns *StateResult)
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Errorf("Expected Test to match after creating container")
	}

	// Stop container
	decl.State = "stopped"
	apply2, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Stop apply failed: %v", err)
	}
	if !apply2.Success {
		t.Errorf("Expected stop to succeed: %s", apply2.Comment)
	}

	// Test stopped state
	test2, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test stopped state failed: %v", err)
	}
	if !test2 {
		t.Errorf("Expected Test to match for stopped state")
	}

	// Remove container
	decl.State = "absent"
	apply3, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Remove apply failed: %v", err)
	}
	if !apply3.Success {
		t.Errorf("Expected remove to succeed: %s", apply3.Comment)
	}

	// Test absent state
	test3, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test absent state failed: %v", err)
	}
	if !test3 {
		t.Errorf("Expected Test to match for absent state")
	}
}

// =============================================================================
// File Module Apply/Test Integration Tests (Real Filesystem)
// =============================================================================

func TestFileModule_ApplyTest_CreateFile(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "testfile.txt")

	module := NewFileModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     testFile,
		Module: "file",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "Hello, World!\n",
			"mode":     "0644",
		},
	}

	// Check initial state (file doesn't exist)
	check1, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Initial check failed: %v", err)
	}
	if check1.Matches {
		t.Error("Expected initial state to not match (file doesn't exist)")
	}

	// Apply state (create file)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}
	if !apply1.Changed {
		t.Error("Expected apply to report changes")
	}

	// Verify file was created with correct content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}
	if string(content) != "Hello, World!\n" {
		t.Errorf("File content mismatch: got %q", string(content))
	}

	// Test should return true
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true after creating file")
	}

	// Apply again (idempotency)
	apply2, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
	if !apply2.Success {
		t.Errorf("Expected second apply to succeed: %s", apply2.Comment)
	}
	if apply2.Changed {
		t.Error("Expected second apply to report no changes (idempotent)")
	}

	// Remove file
	decl.State = "absent"
	apply3, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Remove apply failed: %v", err)
	}
	if !apply3.Success {
		t.Errorf("Expected remove to succeed: %s", apply3.Comment)
	}

	// Verify file was removed
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("Expected file to be removed")
	}

	// Test absent state
	test2, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test absent state failed: %v", err)
	}
	if !test2 {
		t.Error("Expected Test to return true for absent state")
	}
}

func TestFileModule_ApplyTest_CreateDirectory(t *testing.T) {
	tmpDir := t.TempDir()
	testDir := filepath.Join(tmpDir, "testdir", "nested")

	module := NewFileModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     testDir,
		Module: "file",
		State:  "directory",
		Parameters: map[string]interface{}{
			"mode": "0755",
		},
	}

	// Apply state (create directory)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify directory was created
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("Failed to stat created directory: %v", err)
	}
	if !info.IsDir() {
		t.Error("Expected a directory")
	}

	// Test should return true
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true after creating directory")
	}
}

func TestFileModule_ApplyTest_CreateSymlink(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Symlinks require elevated privileges on Windows")
	}

	tmpDir := t.TempDir()
	targetFile := filepath.Join(tmpDir, "target.txt")
	linkFile := filepath.Join(tmpDir, "link.txt")

	// Create target file
	if err := os.WriteFile(targetFile, []byte("target content"), 0644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}

	module := NewFileModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     linkFile,
		Module: "file",
		State:  "symlink",
		Parameters: map[string]interface{}{
			"target": targetFile,
		},
	}

	// Apply state (create symlink)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify symlink was created
	target, err := os.Readlink(linkFile)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}
	if target != targetFile {
		t.Errorf("Symlink target mismatch: got %q, want %q", target, targetFile)
	}

	// Test should return true
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true after creating symlink")
	}
}

// =============================================================================
// Cmd Module Apply/Test Integration Tests
// =============================================================================

func TestCmdModule_ApplyTest_RunCommand(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "output.txt")

	module := NewCmdModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "echo hello > " + outputFile,
		Module: "cmd",
		State:  "run",
		Parameters: map[string]interface{}{
			"creates": outputFile,
		},
	}

	// First run should execute the command
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("First apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify output file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected output file to be created")
	}

	// Second run should skip (creates condition met)
	apply2, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
	if !apply2.Success {
		t.Errorf("Expected second apply to succeed: %s", apply2.Comment)
	}
	if apply2.Changed {
		t.Error("Expected second apply to skip (creates condition)")
	}
}

func TestCmdModule_ApplyTest_EnvironmentVars(t *testing.T) {
	tmpDir := t.TempDir()
	outputFile := filepath.Join(tmpDir, "env_output.txt")

	module := NewCmdModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "sh -c 'echo $MY_VAR > " + outputFile + "'",
		Module: "cmd",
		State:  "run",
		Parameters: map[string]interface{}{
			"env": map[string]interface{}{
				"MY_VAR": "hello_world",
			},
			"creates": outputFile,
		},
	}

	// Run command with environment variable
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify output file was created
	if _, err := os.Stat(outputFile); os.IsNotExist(err) {
		t.Error("Expected output file to be created")
	}
}

// =============================================================================
// X509 Module Apply/Test Integration Tests
// =============================================================================

func TestX509Module_ApplyTest_SelfSignedCert(t *testing.T) {
	tmpDir := t.TempDir()
	certFile := filepath.Join(tmpDir, "cert.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")

	module := NewX509Module()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "test-cert",
		Module: "x509",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":            certFile,
			"privatekey_path": keyFile,
			"common_name":     "test.example.com",
			"days":            365,
			"selfsigned":      true,
		},
	}

	// Apply state (create self-signed cert)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify cert was created
	if _, err := os.Stat(certFile); os.IsNotExist(err) {
		t.Error("Expected certificate file to be created")
	}

	// Test should return success (X509 Test returns *StateResult)
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Errorf("Expected Test to match after creating cert")
	}
}

// =============================================================================
// File Module Content Validation Tests
// =============================================================================

func TestFileModule_ApplyTest_UpdateContent(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "config.txt")

	// Create initial file
	initialContent := "old content\n"
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	module := NewFileModule()
	ctx := context.Background()

	newContent := "new content\n"
	decl := &StateDeclaration{
		ID:     testFile,
		Module: "file",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": newContent,
			"mode":     "0644",
		},
	}

	// Check should show content difference
	check1, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}
	if check1.Matches {
		t.Error("Expected check to show content difference")
	}

	// Apply state (update content)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}
	if !apply1.Changed {
		t.Error("Expected apply to report changes")
	}

	// Verify content was updated
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if string(content) != newContent {
		t.Errorf("Content mismatch: got %q, want %q", string(content), newContent)
	}

	// Test should return true
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true after updating content")
	}
}

// =============================================================================
// IniFile Module Apply/Test Integration Tests
// =============================================================================

func TestIniFileModule_ApplyTest_SetValue(t *testing.T) {
	tmpDir := t.TempDir()
	iniFile := filepath.Join(tmpDir, "config.ini")

	// Create initial INI file
	initialContent := "[section1]\nkey1=value1\n\n[section2]\nkey2=value2\n"
	if err := os.WriteFile(iniFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create INI file: %v", err)
	}

	module := NewIniFileModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "set-key3",
		Module: "ini_file",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":    iniFile,
			"section": "section1",
			"option":  "key3",
			"value":   "value3",
		},
	}

	// Apply state (set value)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify value was set
	content, err := os.ReadFile(iniFile)
	if err != nil {
		t.Fatalf("Failed to read INI file: %v", err)
	}
	if !strings.Contains(string(content), "key3") {
		t.Error("Expected key3 to be in INI file")
	}

	// Test should return true
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true after setting value")
	}

	// Apply again (idempotency)
	apply2, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
	if !apply2.Success {
		t.Errorf("Expected second apply to succeed: %s", apply2.Comment)
	}
	if apply2.Changed {
		t.Error("Expected second apply to report no changes (idempotent)")
	}
}

// =============================================================================
// Archive Module Apply/Test Integration Tests
// =============================================================================

func TestArchiveModule_ApplyTest_ExtractArchive(t *testing.T) {
	tmpDir := t.TempDir()
	extractDir := filepath.Join(tmpDir, "extracted")

	// Create a simple tar archive
	archiveFile := filepath.Join(tmpDir, "test.tar")
	tarContent := createSimpleTarArchive(t, tmpDir)
	if err := os.WriteFile(archiveFile, tarContent, 0644); err != nil {
		t.Fatalf("Failed to create archive: %v", err)
	}

	module := NewArchiveModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "extract-test",
		Module: "archive",
		State:  "present",
		Parameters: map[string]interface{}{
			"src":  archiveFile,
			"dest": extractDir,
		},
	}

	// Apply state (extract archive)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify directory was created
	if _, err := os.Stat(extractDir); os.IsNotExist(err) {
		t.Error("Expected extraction directory to be created")
	}
}

// Helper to create a simple tar archive
func createSimpleTarArchive(t *testing.T, tmpDir string) []byte {
	t.Helper()
	// Create a minimal tar archive with just a header
	// This is a simple 512-byte tar header for a file
	header := make([]byte, 512)
	copy(header[0:], "testfile.txt")  // filename
	copy(header[100:], "0000644")     // mode
	copy(header[108:], "0000000")     // uid
	copy(header[116:], "0000000")     // gid
	copy(header[124:], "00000000000") // size
	copy(header[136:], "00000000000") // mtime
	copy(header[156:], "0")           // typeflag
	copy(header[257:], "ustar")       // magic
	// Checksum calculation would go here for a real tar
	return header
}

// =============================================================================
// Sysctl Module Apply/Test Integration Tests
// =============================================================================

func TestSysctlModule_ApplyTest_SetValue(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Sysctl module only supported on Linux")
	}

	tmpDir := t.TempDir()
	sysctlState := filepath.Join(tmpDir, "sysctl_state")

	writeFakeCmd(t, tmpDir, "sysctl", `#!/bin/sh
case "$1" in
    "-n")
        param="$2"
        if [ -f "`+sysctlState+`" ] && grep -q "^$param=" "`+sysctlState+`"; then
            grep "^$param=" "`+sysctlState+`" | cut -d= -f2
        else
            echo "0"
        fi
        exit 0
        ;;
    "-w")
        echo "$2" >> "`+sysctlState+`"
        exit 0
        ;;
    *)
        exit 0
        ;;
esac
`)

	setupFakePath(t, tmpDir)

	module := NewSysctlModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "net.ipv4.ip_forward",
		Module: "sysctl",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":  "net.ipv4.ip_forward",
			"value": "1",
		},
	}

	// Apply state
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Test should return true
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true after setting sysctl")
	}
}

// =============================================================================
// Lineinfile Module Apply/Test Integration Tests
// =============================================================================

func TestLineinfileModule_ApplyTest_AddLine(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "config.txt")

	// Create initial file
	initialContent := "# Configuration\nkey1=value1\nkey2=value2\n"
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	module := NewLineinfileModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "add-key3",
		Module: "lineinfile",
		State:  "present",
		Parameters: map[string]interface{}{
			"path": testFile,
			"line": "key3=value3",
		},
	}

	// Apply state (add line)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify line was added
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if !strings.Contains(string(content), "key3=value3") {
		t.Error("Expected line to be added")
	}

	// Test should return true
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true after adding line")
	}

	// Apply again (idempotency)
	apply2, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Second apply failed: %v", err)
	}
	if !apply2.Success {
		t.Errorf("Expected second apply to succeed: %s", apply2.Comment)
	}
	if apply2.Changed {
		t.Error("Expected second apply to report no changes (idempotent)")
	}
}

func TestLineinfileModule_ApplyTest_ReplaceLine(t *testing.T) {
	tmpDir := t.TempDir()
	testFile := filepath.Join(tmpDir, "config.txt")

	// Create initial file
	initialContent := "# Configuration\nkey1=oldvalue\nkey2=value2\n"
	if err := os.WriteFile(testFile, []byte(initialContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	module := NewLineinfileModule()
	ctx := context.Background()

	decl := &StateDeclaration{
		ID:     "update-key1",
		Module: "lineinfile",
		State:  "present",
		Parameters: map[string]interface{}{
			"path":   testFile,
			"regexp": "^key1=",
			"line":   "key1=newvalue",
		},
	}

	// Apply state (replace line)
	apply1, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}
	if !apply1.Success {
		t.Errorf("Expected apply to succeed: %s", apply1.Comment)
	}

	// Verify line was replaced
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}
	if !strings.Contains(string(content), "key1=newvalue") {
		t.Error("Expected line to be replaced")
	}
	if strings.Contains(string(content), "key1=oldvalue") {
		t.Error("Expected old line to be removed")
	}

	// Test should return true
	test1, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}
	if !test1 {
		t.Error("Expected Test to return true after replacing line")
	}
}
