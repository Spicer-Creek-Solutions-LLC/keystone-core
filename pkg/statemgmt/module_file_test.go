package statemgmt

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestFileModule_Check_Absent(t *testing.T) {
	module := NewFileModule()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-file-absent.txt")
	// Ensure file doesn't exist
	os.Remove(tmpFile)

	decl := &StateDeclaration{
		ID:         tmpFile,
		Module:     "file",
		State:      "absent",
		Parameters: make(map[string]interface{}),
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("Expected file to not be present")
	}

	if !result.Matches {
		t.Error("Expected state to match (file absent, desired absent)")
	}
}

func TestFileModule_Check_Present(t *testing.T) {
	module := NewFileModule()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-file-present.txt")
	// Create the file
	if err := os.WriteFile(tmpFile, []byte("test content"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(tmpFile)

	decl := &StateDeclaration{
		ID:         tmpFile,
		Module:     "file",
		State:      "present",
		Parameters: make(map[string]interface{}),
	}

	result, err := module.Check(ctx, decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("Expected file to be present")
	}

	if !result.Matches {
		t.Error("Expected state to match (file present, desired present)")
	}
}

func TestFileModule_Apply_CreateFile(t *testing.T) {
	module := NewFileModule()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-file-create.txt")
	os.Remove(tmpFile)
	defer os.Remove(tmpFile)

	decl := &StateDeclaration{
		ID:     tmpFile,
		Module: "file",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "hello world",
			"mode":     "0644",
		},
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	if !result.Changed {
		t.Error("Expected changes")
	}

	// Verify file was created
	content, err := os.ReadFile(tmpFile)
	if err != nil {
		t.Fatalf("Failed to read created file: %v", err)
	}

	if string(content) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(content))
	}
}

func TestFileModule_Apply_Idempotent(t *testing.T) {
	module := NewFileModule()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-file-idempotent.txt")
	// Create the file with desired content
	if err := os.WriteFile(tmpFile, []byte("hello world"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(tmpFile)

	decl := &StateDeclaration{
		ID:     tmpFile,
		Module: "file",
		State:  "present",
		Parameters: map[string]interface{}{
			"contents": "hello world",
		},
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	if result.Changed {
		t.Error("Expected no changes (idempotent)")
	}

	if result.Comment != "Already in desired state" {
		t.Errorf("Expected 'Already in desired state', got '%s'", result.Comment)
	}
}

func TestFileModule_Apply_RemoveFile(t *testing.T) {
	module := NewFileModule()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-file-remove.txt")
	// Create the file
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	decl := &StateDeclaration{
		ID:         tmpFile,
		Module:     "file",
		State:      "absent",
		Parameters: make(map[string]interface{}),
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	if !result.Changed {
		t.Error("Expected changes")
	}

	// Verify file was removed
	if _, err := os.Stat(tmpFile); !os.IsNotExist(err) {
		t.Error("Expected file to be removed")
	}
}

func TestFileModule_Apply_CreateDirectory(t *testing.T) {
	module := NewFileModule()
	ctx := context.Background()

	tmpDir := filepath.Join(os.TempDir(), "test-dir-create")
	os.RemoveAll(tmpDir)
	defer os.RemoveAll(tmpDir)

	decl := &StateDeclaration{
		ID:         tmpDir,
		Module:     "file",
		State:      "directory",
		Parameters: map[string]interface{}{"mode": "0755"},
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	if !result.Changed {
		t.Error("Expected changes")
	}

	// Verify directory was created
	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Failed to stat directory: %v", err)
	}

	if !info.IsDir() {
		t.Error("Expected directory")
	}
}

func TestFileModule_Apply_CreateSymlink(t *testing.T) {
	module := NewFileModule()
	ctx := context.Background()

	tmpTarget := filepath.Join(os.TempDir(), "test-symlink-target.txt")
	tmpLink := filepath.Join(os.TempDir(), "test-symlink.txt")

	// Create target file
	if err := os.WriteFile(tmpTarget, []byte("target"), 0644); err != nil {
		t.Fatalf("Failed to create target file: %v", err)
	}
	defer os.Remove(tmpTarget)
	defer os.Remove(tmpLink)

	decl := &StateDeclaration{
		ID:     tmpLink,
		Module: "file",
		State:  "symlink",
		Parameters: map[string]interface{}{
			"target": tmpTarget,
		},
	}

	result, err := module.Apply(ctx, decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected success")
	}

	if !result.Changed {
		t.Error("Expected changes")
	}

	// Verify symlink was created
	info, err := os.Lstat(tmpLink)
	if err != nil {
		t.Fatalf("Failed to stat symlink: %v", err)
	}

	if info.Mode()&os.ModeSymlink == 0 {
		t.Error("Expected symlink")
	}

	target, err := os.Readlink(tmpLink)
	if err != nil {
		t.Fatalf("Failed to read symlink: %v", err)
	}

	if target != tmpTarget {
		t.Errorf("Expected target '%s', got '%s'", tmpTarget, target)
	}
}

func TestFileModule_Test(t *testing.T) {
	module := NewFileModule()
	ctx := context.Background()

	tmpFile := filepath.Join(os.TempDir(), "test-file-test.txt")
	if err := os.WriteFile(tmpFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}
	defer os.Remove(tmpFile)

	decl := &StateDeclaration{
		ID:         tmpFile,
		Module:     "file",
		State:      "present",
		Parameters: make(map[string]interface{}),
	}

	matches, err := module.Test(ctx, decl)
	if err != nil {
		t.Fatalf("Test failed: %v", err)
	}

	if !matches {
		t.Error("Expected state to match")
	}
}
