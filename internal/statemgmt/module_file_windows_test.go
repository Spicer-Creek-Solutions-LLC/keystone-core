// Copyright 2024 Spicer Creek Solutions LLC
// SPDX-License-Identifier: Apache-2.0

package statemgmt

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// Tests for Windows path handling (run on all platforms)
func TestNormalizePath_Basic(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows path test on non-Windows")
	}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "empty path",
			input:    "",
			expected: "",
		},
		{
			name:     "forward slashes",
			input:    "C:/Users/Test/file.txt",
			expected: "C:\\Users\\Test\\file.txt",
		},
		{
			name:     "already normalized",
			input:    "C:\\Users\\Test\\file.txt",
			expected: "C:\\Users\\Test\\file.txt",
		},
		{
			name:     "UNC path",
			input:    "\\\\server\\share\\file.txt",
			expected: "\\\\server\\share\\file.txt",
		},
		{
			name:     "long path prefix preserved",
			input:    "\\\\?\\C:\\Users\\Test\\file.txt",
			expected: "\\\\?\\C:\\Users\\Test\\file.txt",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := NormalizePath(tt.input)
			// For absolute paths, result depends on current directory
			if tt.input != "" && tt.expected != "" && !filepath.IsAbs(tt.input) {
				// Skip comparison for relative paths that get converted to absolute
				return
			}
			if result != tt.expected {
				t.Errorf("NormalizePath(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIsUNCPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows path test on non-Windows")
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"\\\\server\\share", true},
		{"\\\\server\\share\\file.txt", true},
		{"\\\\?\\C:\\path", false}, // Long path prefix, not UNC
		{"C:\\path", false},
		{"/unix/path", false},
	}

	for _, tt := range tests {
		result := IsUNCPath(tt.path)
		if result != tt.expected {
			t.Errorf("IsUNCPath(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestIsLongPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows path test on non-Windows")
	}

	tests := []struct {
		path     string
		expected bool
	}{
		{"\\\\?\\C:\\path", true},
		{"\\\\?\\UNC\\server\\share", true},
		{"\\\\server\\share", false},
		{"C:\\path", false},
	}

	for _, tt := range tests {
		result := IsLongPath(tt.path)
		if result != tt.expected {
			t.Errorf("IsLongPath(%q) = %v, want %v", tt.path, result, tt.expected)
		}
	}
}

func TestParseDriveLetter(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows path test on non-Windows")
	}

	tests := []struct {
		path          string
		expectedDrive string
		expectedRest  string
	}{
		{"C:\\path", "C:", "\\path"},
		{"D:\\Users\\Test", "D:", "\\Users\\Test"},
		{"\\\\server\\share", "", "\\\\server\\share"},
		{"/unix/path", "", "/unix/path"},
		{"relative/path", "", "relative/path"},
	}

	for _, tt := range tests {
		drive, rest := ParseDriveLetter(tt.path)
		if drive != tt.expectedDrive || rest != tt.expectedRest {
			t.Errorf("ParseDriveLetter(%q) = (%q, %q), want (%q, %q)",
				tt.path, drive, rest, tt.expectedDrive, tt.expectedRest)
		}
	}
}

func TestAccessMaskToString(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	tests := []struct {
		mask     uint32
		expected string
	}{
		{FILE_ALL_ACCESS, "FullControl"},
		{FILE_GENERIC_READ, "Read"},
		{FILE_GENERIC_WRITE, "Write"},
		{FILE_GENERIC_READ | FILE_GENERIC_WRITE, "Read,Write"},
		{0, "0x00000000"},
	}

	for _, tt := range tests {
		result := AccessMaskToString(tt.mask)
		if result != tt.expected {
			t.Errorf("AccessMaskToString(0x%08X) = %q, want %q", tt.mask, result, tt.expected)
		}
	}
}

func TestStringToAccessMask(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	tests := []struct {
		perm     string
		expected uint32
	}{
		{"fullcontrol", FILE_ALL_ACCESS},
		{"full", FILE_ALL_ACCESS},
		{"f", FILE_ALL_ACCESS},
		{"read", FILE_GENERIC_READ},
		{"r", FILE_GENERIC_READ},
		{"write", FILE_GENERIC_WRITE},
		{"w", FILE_GENERIC_WRITE},
		{"invalid", 0},
	}

	for _, tt := range tests {
		result := StringToAccessMask(tt.perm)
		if result != tt.expected {
			t.Errorf("StringToAccessMask(%q) = 0x%08X, want 0x%08X", tt.perm, result, tt.expected)
		}
	}
}

func TestAttributesToString(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	tests := []struct {
		attrs    *WindowsFileAttributes
		expected string
	}{
		{
			attrs:    &WindowsFileAttributes{},
			expected: "Normal",
		},
		{
			attrs:    &WindowsFileAttributes{ReadOnly: true},
			expected: "ReadOnly",
		},
		{
			attrs:    &WindowsFileAttributes{Hidden: true, System: true},
			expected: "Hidden,System",
		},
		{
			attrs:    &WindowsFileAttributes{ReadOnly: true, Hidden: true, Archive: true},
			expected: "ReadOnly,Hidden,Archive",
		},
	}

	for _, tt := range tests {
		result := AttributesToString(tt.attrs)
		if result != tt.expected {
			t.Errorf("AttributesToString() = %q, want %q", result, tt.expected)
		}
	}
}

// Windows-specific integration tests
func TestGetFileAttributes_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	// Create a temp file
	tmpFile, err := os.CreateTemp("", "kscore-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	attrs, err := GetFileAttributes(tmpFile.Name())
	if err != nil {
		t.Fatalf("GetFileAttributes failed: %v", err)
	}

	// Default file should have Archive attribute
	if !attrs.Archive {
		t.Log("Note: Archive attribute not set on new file")
	}

	// Should not be hidden, system, or read-only by default
	if attrs.Hidden {
		t.Error("New file should not be hidden")
	}
	if attrs.System {
		t.Error("New file should not be system")
	}
	if attrs.ReadOnly {
		t.Error("New file should not be read-only")
	}
}

func TestSetFileAttributes_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	// Create a temp file
	tmpFile, err := os.CreateTemp("", "kscore-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Set read-only attribute
	err = SetFileReadOnly(tmpFile.Name(), true)
	if err != nil {
		t.Fatalf("SetFileReadOnly failed: %v", err)
	}

	// Verify
	attrs, err := GetFileAttributes(tmpFile.Name())
	if err != nil {
		t.Fatalf("GetFileAttributes failed: %v", err)
	}
	if !attrs.ReadOnly {
		t.Error("Expected ReadOnly to be true")
	}

	// Clear read-only so we can delete the file
	err = SetFileReadOnly(tmpFile.Name(), false)
	if err != nil {
		t.Fatalf("SetFileReadOnly(false) failed: %v", err)
	}
}

func TestGetFileOwnerName_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	// Create a temp file
	tmpFile, err := os.CreateTemp("", "kscore-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	owner, err := getFileOwnerName(tmpFile.Name())
	if err != nil {
		t.Fatalf("getFileOwnerName failed: %v", err)
	}

	if owner == "" {
		t.Error("Expected non-empty owner name")
	}

	t.Logf("File owner: %s", owner)
}

func TestGetWindowsACL_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	// Create a temp file
	tmpFile, err := os.CreateTemp("", "kscore-test-*.txt")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	acl, err := GetWindowsACL(tmpFile.Name())
	if err != nil {
		t.Fatalf("GetWindowsACL failed: %v", err)
	}

	if acl.Owner == "" {
		t.Error("Expected non-empty owner")
	}

	t.Logf("Owner: %s", acl.Owner)
	t.Logf("Group: %s", acl.Group)
	t.Logf("ACE count: %d", len(acl.Entries))

	for i, ace := range acl.Entries {
		t.Logf("ACE[%d]: Type=%s, Principal=%s, Access=%s, Inherited=%v",
			i, ace.Type, ace.Principal, AccessMaskToString(ace.Access), ace.Inherited)
	}
}

func TestGetLongPathName_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	// Use Windows directory which should exist
	path := "C:\\Windows"
	longPath, err := GetLongPathName(path)
	if err != nil {
		t.Fatalf("GetLongPathName failed: %v", err)
	}

	// Should return the path (possibly unchanged)
	if longPath == "" {
		t.Error("Expected non-empty long path")
	}

	t.Logf("Long path: %s", longPath)
}

func TestGetFullPath_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Skipping Windows test on non-Windows")
	}

	// Test with relative path
	fullPath, err := GetFullPath(".")
	if err != nil {
		t.Fatalf("GetFullPath failed: %v", err)
	}

	if fullPath == "" {
		t.Error("Expected non-empty full path")
	}

	// Should be absolute path
	if !filepath.IsAbs(fullPath) && !strings.HasPrefix(fullPath, "\\\\") {
		t.Errorf("Expected absolute path, got: %s", fullPath)
	}

	t.Logf("Full path: %s", fullPath)
}
