package verify

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

func TestNewHashVerifier(t *testing.T) {
	verifier := NewHashVerifier(SHA256)
	if verifier.algorithm != SHA256 {
		t.Errorf("expected SHA256, got %s", verifier.algorithm)
	}

	defaultVerifier := NewDefaultHashVerifier()
	if defaultVerifier.algorithm != SHA256 {
		t.Errorf("expected default SHA256, got %s", defaultVerifier.algorithm)
	}
}

func TestComputeFileHash(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test data for hashing")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	verifier := NewDefaultHashVerifier()

	// Compute hash
	hash, err := verifier.ComputeHash(testFile)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	// Hash should have algorithm prefix
	if hash[:7] != "sha256:" {
		t.Errorf("expected sha256: prefix, got %s", hash[:7])
	}

	// Computing again should give same hash
	hash2, err := verifier.ComputeHash(testFile)
	if err != nil {
		t.Fatalf("failed to compute hash again: %v", err)
	}

	if hash != hash2 {
		t.Errorf("hashes don't match: %s vs %s", hash, hash2)
	}
}

func TestComputeDirectoryHash(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test directory structure
	os.WriteFile(filepath.Join(tmpDir, "file1.txt"), []byte("content1"), 0644)
	os.WriteFile(filepath.Join(tmpDir, "file2.txt"), []byte("content2"), 0644)

	subdir := filepath.Join(tmpDir, "subdir")
	os.Mkdir(subdir, 0755)
	os.WriteFile(filepath.Join(subdir, "file3.txt"), []byte("content3"), 0644)

	verifier := NewDefaultHashVerifier()

	// Compute hash
	hash, err := verifier.ComputeHash(tmpDir)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	// Hash should be deterministic
	hash2, err := verifier.ComputeHash(tmpDir)
	if err != nil {
		t.Fatalf("failed to compute hash again: %v", err)
	}

	if hash != hash2 {
		t.Errorf("directory hashes don't match: %s vs %s", hash, hash2)
	}
}

func TestComputeZipHash(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a ZIP file
	zipPath := filepath.Join(tmpDir, "test.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("failed to create zip file: %v", err)
	}

	zipWriter := zip.NewWriter(zipFile)

	// Add files to ZIP
	files := map[string]string{
		"file1.txt": "content1",
		"file2.txt": "content2",
		"dir/file3.txt": "content3",
	}

	for name, content := range files {
		writer, err := zipWriter.Create(name)
		if err != nil {
			t.Fatalf("failed to create zip entry: %v", err)
		}
		writer.Write([]byte(content))
	}

	zipWriter.Close()
	zipFile.Close()

	verifier := NewDefaultHashVerifier()

	// Compute hash
	hash, err := verifier.ComputeHash(zipPath)
	if err != nil {
		t.Fatalf("failed to compute zip hash: %v", err)
	}

	// Hash should be deterministic
	hash2, err := verifier.ComputeHash(zipPath)
	if err != nil {
		t.Fatalf("failed to compute hash again: %v", err)
	}

	if hash != hash2 {
		t.Errorf("zip hashes don't match: %s vs %s", hash, hash2)
	}
}

func TestVerifyHash(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test data")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	verifier := NewDefaultHashVerifier()

	// Compute the expected hash
	expectedHash, err := verifier.ComputeHash(testFile)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	// Verify with correct hash
	valid, err := verifier.VerifyHash(testFile, expectedHash)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if !valid {
		t.Error("expected hash to be valid")
	}

	// Verify with incorrect hash
	wrongHash := "sha256:0000000000000000000000000000000000000000000000000000000000000000"
	valid, err = verifier.VerifyHash(testFile, wrongHash)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if valid {
		t.Error("expected hash to be invalid")
	}
}

func TestVerifyHashWithoutPrefix(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test data")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	verifier := NewDefaultHashVerifier()

	// Compute hash
	fullHash, err := verifier.ComputeHash(testFile)
	if err != nil {
		t.Fatalf("failed to compute hash: %v", err)
	}

	// Extract hash without prefix
	hashOnly := normalizeHash(fullHash)

	// Verify should work with or without prefix
	valid, err := verifier.VerifyHash(testFile, hashOnly)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if !valid {
		t.Error("expected hash to be valid without prefix")
	}
}

func TestComputeHashNonexistent(t *testing.T) {
	verifier := NewDefaultHashVerifier()

	_, err := verifier.ComputeHash("/nonexistent/path")
	if err != ErrModuleNotFound {
		t.Errorf("expected ErrModuleNotFound, got %v", err)
	}
}

func TestDifferentAlgorithms(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	testData := []byte("test data")
	if err := os.WriteFile(testFile, testData, 0644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	// Test SHA256
	sha256Verifier := NewHashVerifier(SHA256)
	hash256, err := sha256Verifier.ComputeHash(testFile)
	if err != nil {
		t.Fatalf("failed to compute SHA256: %v", err)
	}

	if hash256[:7] != "sha256:" {
		t.Errorf("expected sha256: prefix, got %s", hash256[:7])
	}

	// Test SHA512
	sha512Verifier := NewHashVerifier(SHA512)
	hash512, err := sha512Verifier.ComputeHash(testFile)
	if err != nil {
		t.Fatalf("failed to compute SHA512: %v", err)
	}

	if hash512[:7] != "sha512:" {
		t.Errorf("expected sha512: prefix, got %s", hash512[:7])
	}

	// Hashes should be different
	if normalizeHash(hash256) == normalizeHash(hash512) {
		t.Error("SHA256 and SHA512 hashes should be different")
	}
}

func TestParseHash(t *testing.T) {
	tests := []struct {
		name        string
		hashStr     string
		expectAlg   HashAlgorithm
		expectError bool
	}{
		{
			name:        "valid SHA256",
			hashStr:     "sha256:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			expectAlg:   SHA256,
			expectError: false,
		},
		{
			name:        "valid SHA512",
			hashStr:     "sha512:abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890",
			expectAlg:   SHA512,
			expectError: false,
		},
		{
			name:        "missing algorithm",
			hashStr:     "abcdef1234567890",
			expectError: true,
		},
		{
			name:        "invalid algorithm",
			hashStr:     "md5:abcdef1234567890",
			expectError: true,
		},
		{
			name:        "invalid hex",
			hashStr:     "sha256:xyz",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			alg, value, err := ParseHash(tt.hashStr)

			if tt.expectError {
				if err == nil {
					t.Error("expected error but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				if alg != tt.expectAlg {
					t.Errorf("expected algorithm %s, got %s", tt.expectAlg, alg)
				}
				if value == "" {
					t.Error("expected non-empty hash value")
				}
			}
		})
	}
}

func TestFormatHash(t *testing.T) {
	formatted := FormatHash(SHA256, "abcdef1234567890")
	expected := "sha256:abcdef1234567890"

	if formatted != expected {
		t.Errorf("expected %s, got %s", expected, formatted)
	}
}

func TestNormalizeHash(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "sha256:abcdef",
			expected: "abcdef",
		},
		{
			input:    "abcdef",
			expected: "abcdef",
		},
		{
			input:    "sha512:123456",
			expected: "123456",
		},
	}

	for _, tt := range tests {
		result := normalizeHash(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeHash(%q) = %q, expected %q", tt.input, result, tt.expected)
		}
	}
}
