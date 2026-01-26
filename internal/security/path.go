package security

import (
	"fmt"
	"path/filepath"
	"strings"
)

// PathError is returned when path validation fails.
type PathError struct {
	Path   string
	Reason string
}

func (e *PathError) Error() string {
	return fmt.Sprintf("invalid path %q: %s", e.Path, e.Reason)
}

// ValidatePath checks if a path is safe to use within a given base directory.
// It prevents path traversal attacks by ensuring the resolved path stays within
// the base directory.
//
// Parameters:
//   - basePath: The trusted base directory (must be absolute)
//   - userPath: The user-provided path to validate
//
// Returns:
//   - The cleaned, absolute path if valid
//   - An error if the path is invalid or escapes the base directory
func ValidatePath(basePath, userPath string) (string, error) {
	// Ensure base path is absolute
	if !filepath.IsAbs(basePath) {
		return "", &PathError{Path: basePath, Reason: "base path must be absolute"}
	}

	// Clean the base path
	basePath = filepath.Clean(basePath)

	// Join and clean the full path
	fullPath := filepath.Join(basePath, userPath)
	fullPath = filepath.Clean(fullPath)

	// Check that the resulting path is within the base directory
	// Using HasPrefix after Clean() is safe because Clean() normalizes paths
	if !strings.HasPrefix(fullPath, basePath+string(filepath.Separator)) && fullPath != basePath {
		return "", &PathError{
			Path:   userPath,
			Reason: "path escapes base directory",
		}
	}

	return fullPath, nil
}

// ValidateFilename checks if a filename is safe to use.
// It rejects filenames containing path separators or that start with dots.
//
// Parameters:
//   - filename: The filename to validate
//
// Returns:
//   - An error if the filename is invalid
func ValidateFilename(filename string) error {
	if filename == "" {
		return &PathError{Path: filename, Reason: "filename is empty"}
	}

	// Reject path separators
	if strings.ContainsAny(filename, "/\\") {
		return &PathError{Path: filename, Reason: "filename contains path separators"}
	}

	// Reject directory traversal attempts
	if filename == "." || filename == ".." {
		return &PathError{Path: filename, Reason: "filename is a directory reference"}
	}

	// Reject hidden files (starting with .) - can be optionally relaxed
	if strings.HasPrefix(filename, ".") {
		return &PathError{Path: filename, Reason: "filename starts with dot"}
	}

	// Reject null bytes
	if strings.ContainsRune(filename, 0) {
		return &PathError{Path: filename, Reason: "filename contains null byte"}
	}

	return nil
}

// SanitizePathComponent removes potentially dangerous characters from a path component.
// This is useful for generating safe filenames from user input.
//
// Parameters:
//   - input: The user input to sanitize
//
// Returns:
//   - A sanitized string safe to use as a filename component
func SanitizePathComponent(input string) string {
	// Replace path separators with underscores
	result := strings.ReplaceAll(input, "/", "_")
	result = strings.ReplaceAll(result, "\\", "_")

	// Remove null bytes
	result = strings.ReplaceAll(result, "\x00", "")

	// Remove leading dots
	result = strings.TrimLeft(result, ".")

	// If empty after sanitization, return a safe default
	if result == "" {
		return "unnamed"
	}

	return result
}
