package capabilities

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestFSReadCapability_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cap         *FSReadCapability
		expectError bool
	}{
		{
			name: "valid capability",
			cap: &FSReadCapability{
				AllowedPaths: []string{"/tmp/*"},
				MaxFileSize:  1024,
			},
			expectError: false,
		},
		{
			name: "no allowed paths",
			cap: &FSReadCapability{
				AllowedPaths: []string{},
			},
			expectError: true,
		},
		{
			name: "invalid allowed pattern",
			cap: &FSReadCapability{
				AllowedPaths: []string{"[invalid"},
			},
			expectError: true,
		},
		{
			name: "invalid denied pattern",
			cap: &FSReadCapability{
				AllowedPaths: []string{"/tmp/*"},
				DeniedPaths:  []string{"[invalid"},
			},
			expectError: true,
		},
		{
			name: "negative max file size",
			cap: &FSReadCapability{
				AllowedPaths: []string{"/tmp/*"},
				MaxFileSize:  -1,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestFSReadCapability_CheckPath(t *testing.T) {
	tmpDir := t.TempDir()

	cap := &FSReadCapability{
		AllowedPaths: []string{
			filepath.Join(tmpDir, "*"),
			filepath.Join(tmpDir, "subdir", "**"),
		},
		DeniedPaths: []string{
			filepath.Join(tmpDir, "denied.txt"),
		},
	}

	tests := []struct {
		name        string
		path        string
		expectError error
	}{
		{
			name:        "allowed path",
			path:        filepath.Join(tmpDir, "allowed.txt"),
			expectError: nil,
		},
		{
			name:        "denied path",
			path:        filepath.Join(tmpDir, "denied.txt"),
			expectError: ErrPathDenied,
		},
		{
			name:        "not allowed path",
			path:        "/etc/passwd",
			expectError: ErrPathNotAllowed,
		},
		{
			name:        "recursive allowed path",
			path:        filepath.Join(tmpDir, "subdir", "deep", "file.txt"),
			expectError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := cap.CheckPath(tt.path)
			if tt.expectError != nil {
				if err == nil {
					t.Errorf("expected error %v but got nil", tt.expectError)
				} else if !errors.Is(err, tt.expectError) {
					t.Errorf("expected error %v but got %v", tt.expectError, err)
				}
			} else if err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestFSReadCapability_ReadFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test data")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cap := &FSReadCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "*")},
		MaxFileSize:  1024,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test successful read
	data, err := cap.ReadFile(ctx, testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("expected data %q, got %q", testData, data)
	}
}

func TestFSReadCapability_ReadFileMaxSize(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("this is a longer test data string")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cap := &FSReadCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "*")},
		MaxFileSize:  10, // Smaller than file size
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test read with size limit
	_, err := cap.ReadFile(ctx, testFile)
	if !errors.Is(err, ErrMaxSizeExceeded) {
		t.Errorf("expected ErrMaxSizeExceeded, got %v", err)
	}
}

func TestFSReadCapability_OpenFile(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test data")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	cap := &FSReadCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "*")},
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Test successful open
	file, err := cap.OpenFile(ctx, testFile)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer file.Close()

	// Verify we can read from the file
	buf := make([]byte, len(testData))
	n, err := file.Read(buf)
	if err != nil {
		t.Fatalf("failed to read from file: %v", err)
	}

	if n != len(testData) || string(buf) != string(testData) {
		t.Errorf("expected data %q, got %q", testData, buf)
	}
}

func TestFSWriteCapability_Validate(t *testing.T) {
	tests := []struct {
		name        string
		cap         *FSWriteCapability
		expectError bool
	}{
		{
			name: "valid capability",
			cap: &FSWriteCapability{
				AllowedPaths: []string{"/tmp/*"},
				MaxFileSize:  1024,
			},
			expectError: false,
		},
		{
			name: "no allowed paths",
			cap: &FSWriteCapability{
				AllowedPaths: []string{},
			},
			expectError: true,
		},
		{
			name: "invalid allowed pattern",
			cap: &FSWriteCapability{
				AllowedPaths: []string{"[invalid"},
			},
			expectError: true,
		},
		{
			name: "negative max file size",
			cap: &FSWriteCapability{
				AllowedPaths: []string{"/tmp/*"},
				MaxFileSize:  -1,
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cap.Validate()
			if tt.expectError && err == nil {
				t.Error("expected error but got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestFSWriteCapability_WriteFile(t *testing.T) {
	tmpDir := t.TempDir()

	cap := &FSWriteCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "*")},
		MaxFileSize:  1024,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	testFile := filepath.Join(tmpDir, "output.txt")
	testData := []byte("test output")

	// Test successful write
	err := cap.WriteFile(ctx, testFile, testData, 0644)
	if err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Verify file was written
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("expected data %q, got %q", testData, data)
	}
}

func TestFSWriteCapability_WriteFileMaxSize(t *testing.T) {
	tmpDir := t.TempDir()

	cap := &FSWriteCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "*")},
		MaxFileSize:  10,
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	testFile := filepath.Join(tmpDir, "output.txt")
	testData := []byte("this is a longer test data string")

	// Test write with size limit
	err := cap.WriteFile(ctx, testFile, testData, 0644)
	if !errors.Is(err, ErrMaxSizeExceeded) {
		t.Errorf("expected ErrMaxSizeExceeded, got %v", err)
	}
}

func TestFSWriteCapability_AppendFile(t *testing.T) {
	tmpDir := t.TempDir()

	cap := &FSWriteCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "*")},
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	testFile := filepath.Join(tmpDir, "append.txt")

	// Write initial data
	initialData := []byte("initial\n")
	if err := cap.WriteFile(ctx, testFile, initialData, 0644); err != nil {
		t.Fatalf("failed to write initial data: %v", err)
	}

	// Append more data
	appendData := []byte("appended\n")
	if err := cap.AppendFile(ctx, testFile, appendData, 0644); err != nil {
		t.Fatalf("failed to append data: %v", err)
	}

	// Verify file contents
	data, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	expected := string(initialData) + string(appendData)
	if string(data) != expected {
		t.Errorf("expected data %q, got %q", expected, string(data))
	}
}

func TestFSWriteCapability_DeleteFile(t *testing.T) {
	tmpDir := t.TempDir()

	cap := &FSWriteCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "*")},
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	testFile := filepath.Join(tmpDir, "delete.txt")

	// Create file
	if err := os.WriteFile(testFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Delete file
	if err := cap.DeleteFile(ctx, testFile); err != nil {
		t.Fatalf("failed to delete file: %v", err)
	}

	// Verify file is gone
	if _, err := os.Stat(testFile); !os.IsNotExist(err) {
		t.Error("file still exists after deletion")
	}
}

func TestFSWriteCapability_Mkdir(t *testing.T) {
	tmpDir := t.TempDir()

	cap := &FSWriteCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "*")},
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	testDir := filepath.Join(tmpDir, "newdir")

	// Create directory
	if err := cap.Mkdir(ctx, testDir, 0755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}

	if !info.IsDir() {
		t.Error("created path is not a directory")
	}
}

func TestFSWriteCapability_MkdirAll(t *testing.T) {
	tmpDir := t.TempDir()

	cap := &FSWriteCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "**")},
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	testDir := filepath.Join(tmpDir, "a", "b", "c")

	// Create nested directories
	if err := cap.MkdirAll(ctx, testDir, 0755); err != nil {
		t.Fatalf("failed to create directories: %v", err)
	}

	// Verify directory exists
	info, err := os.Stat(testDir)
	if err != nil {
		t.Fatalf("failed to stat directory: %v", err)
	}

	if !info.IsDir() {
		t.Error("created path is not a directory")
	}
}

func TestFSWriteCapability_CopyFile(t *testing.T) {
	tmpDir := t.TempDir()

	readCap := &FSReadCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "src", "*")},
	}

	writeCap := &FSWriteCapability{
		AllowedPaths: []string{filepath.Join(tmpDir, "dst", "*")},
	}

	ctx := NewCapabilityContext(context.Background(), "test-module")

	// Create source file
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0755); err != nil {
		t.Fatalf("failed to create src dir: %v", err)
	}

	srcFile := filepath.Join(srcDir, "source.txt")
	testData := []byte("test data to copy")
	if err := os.WriteFile(srcFile, testData, 0644); err != nil {
		t.Fatalf("failed to create source file: %v", err)
	}

	// Copy file
	dstFile := filepath.Join(tmpDir, "dst", "dest.txt")
	if err := writeCap.CopyFile(ctx, readCap, srcFile, dstFile); err != nil {
		t.Fatalf("failed to copy file: %v", err)
	}

	// Verify destination file
	data, err := os.ReadFile(dstFile)
	if err != nil {
		t.Fatalf("failed to read destination file: %v", err)
	}

	if string(data) != string(testData) {
		t.Errorf("expected data %q, got %q", testData, data)
	}
}

func TestMatchesRecursive(t *testing.T) {
	tests := []struct {
		pattern  string
		path     string
		expected bool
	}{
		{
			pattern:  "/tmp/**",
			path:     "/tmp/a/b/c/file.txt",
			expected: true,
		},
		{
			pattern:  "/tmp/**/file.txt",
			path:     "/tmp/a/b/c/file.txt",
			expected: true,
		},
		{
			pattern:  "/tmp/**/file.txt",
			path:     "/tmp/file.txt",
			expected: true,
		},
		{
			pattern:  "/tmp/**/*.txt",
			path:     "/tmp/a/b/test.txt",
			expected: true,
		},
		{
			pattern:  "/tmp/**/*.txt",
			path:     "/tmp/a/b/test.log",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			result := matchesRecursive(tt.pattern, tt.path)
			if result != tt.expected {
				t.Errorf("matchesRecursive(%q, %q) = %v, expected %v", tt.pattern, tt.path, result, tt.expected)
			}
		})
	}
}
