package verify

import (
	"archive/zip"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DefaultHashVerifier implements HashVerifier using standard crypto libraries
type DefaultHashVerifier struct {
	algorithm HashAlgorithm
}

// NewHashVerifier creates a new hash verifier with the specified algorithm
func NewHashVerifier(algorithm HashAlgorithm) *DefaultHashVerifier {
	return &DefaultHashVerifier{
		algorithm: algorithm,
	}
}

// NewDefaultHashVerifier creates a new hash verifier with SHA256
func NewDefaultHashVerifier() *DefaultHashVerifier {
	return NewHashVerifier(SHA256)
}

// VerifyHash checks if the module's hash matches the expected hash
func (v *DefaultHashVerifier) VerifyHash(modulePath, expectedHash string) (bool, error) {
	// Compute the actual hash
	actualHash, err := v.ComputeHash(modulePath)
	if err != nil {
		return false, fmt.Errorf("failed to compute hash: %w", err)
	}

	// Normalize hashes (remove algorithm prefix if present)
	expectedHash = normalizeHash(expectedHash)
	actualHash = normalizeHash(actualHash)

	// Compare hashes
	return actualHash == expectedHash, nil
}

// ComputeHash computes the hash of a module
func (v *DefaultHashVerifier) ComputeHash(modulePath string) (string, error) {
	// Check if path exists
	info, err := os.Stat(modulePath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrModuleNotFound
		}
		return "", fmt.Errorf("failed to stat module: %w", err)
	}

	var hashValue string

	// Handle directory vs file
	if info.IsDir() {
		hashValue, err = v.computeDirectoryHash(modulePath)
	} else if strings.HasSuffix(modulePath, ".zip") {
		hashValue, err = v.computeZipHash(modulePath)
	} else {
		hashValue, err = v.computeFileHash(modulePath)
	}

	if err != nil {
		return "", err
	}

	// Return hash with algorithm prefix
	return fmt.Sprintf("%s:%s", v.algorithm, hashValue), nil
}

// computeFileHash computes the hash of a single file
func (v *DefaultHashVerifier) computeFileHash(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hasher := v.newHasher()
	if _, err := io.Copy(hasher, file); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// computeZipHash computes the hash of a ZIP file's contents
func (v *DefaultHashVerifier) computeZipHash(zipPath string) (string, error) {
	// Open the ZIP file
	reader, err := zip.OpenReader(zipPath)
	if err != nil {
		return "", fmt.Errorf("failed to open zip: %w", err)
	}
	defer reader.Close()

	// Create a deterministic hash by sorting files and hashing their content
	hasher := v.newHasher()

	// Sort files by name for deterministic hashing
	files := make([]*zip.File, len(reader.File))
	copy(files, reader.File)
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	// Hash each file's name and content
	for _, file := range files {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Write file name to hash
		hasher.Write([]byte(file.Name))
		hasher.Write([]byte{0}) // Separator

		// Open and hash file content
		rc, err := file.Open()
		if err != nil {
			return "", fmt.Errorf("failed to open file in zip: %w", err)
		}

		if _, err := io.CopyN(hasher, rc, int64(file.UncompressedSize64)); err != nil {
			rc.Close()
			return "", fmt.Errorf("failed to hash file content: %w", err)
		}
		rc.Close()
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// computeDirectoryHash computes the hash of a directory's contents
func (v *DefaultHashVerifier) computeDirectoryHash(dirPath string) (string, error) {
	hasher := v.newHasher()

	// Collect all files
	var files []string
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path
		relPath, err := filepath.Rel(dirPath, path)
		if err != nil {
			return err
		}

		files = append(files, relPath)
		return nil
	})

	if err != nil {
		return "", fmt.Errorf("failed to walk directory: %w", err)
	}

	// Sort for deterministic hashing
	sort.Strings(files)

	// Hash each file
	for _, relPath := range files {
		fullPath := filepath.Join(dirPath, relPath)

		// Write file path to hash (using forward slashes for consistency)
		normalizedPath := filepath.ToSlash(relPath)
		hasher.Write([]byte(normalizedPath))
		hasher.Write([]byte{0}) // Separator

		// Hash file content
		file, err := os.Open(fullPath)
		if err != nil {
			return "", fmt.Errorf("failed to open file: %w", err)
		}

		if _, err := io.Copy(hasher, file); err != nil {
			file.Close()
			return "", fmt.Errorf("failed to hash file: %w", err)
		}
		file.Close()
	}

	return hex.EncodeToString(hasher.Sum(nil)), nil
}

// newHasher creates a new hash instance based on the algorithm
func (v *DefaultHashVerifier) newHasher() hash.Hash {
	switch v.algorithm {
	case SHA512:
		return sha512.New()
	case SHA256:
		fallthrough
	default:
		return sha256.New()
	}
}

// normalizeHash removes the algorithm prefix from a hash string
func normalizeHash(hashStr string) string {
	// Check for algorithm prefix (e.g., "sha256:abc123")
	if idx := strings.Index(hashStr, ":"); idx >= 0 {
		return hashStr[idx+1:]
	}
	return hashStr
}

// ParseHash parses a hash string into algorithm and value
func ParseHash(hashStr string) (HashAlgorithm, string, error) {
	parts := strings.SplitN(hashStr, ":", 2)
	if len(parts) != 2 {
		return "", "", ErrInvalidHash
	}

	algorithm := HashAlgorithm(parts[0])
	value := parts[1]

	// Validate algorithm
	switch algorithm {
	case SHA256, SHA512:
		// Valid
	default:
		return "", "", fmt.Errorf("%w: unsupported algorithm %s", ErrInvalidHash, algorithm)
	}

	// Validate hex encoding
	if _, err := hex.DecodeString(value); err != nil {
		return "", "", fmt.Errorf("%w: invalid hex encoding", ErrInvalidHash)
	}

	return algorithm, value, nil
}

// FormatHash formats a hash value with its algorithm prefix
func FormatHash(algorithm HashAlgorithm, value string) string {
	return fmt.Sprintf("%s:%s", algorithm, value)
}
