package capabilities

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// FSReadCapability allows reading files from the filesystem
type FSReadCapability struct {
	AllowedPaths []string // Glob patterns for allowed paths
	DeniedPaths  []string // Glob patterns for denied paths
	MaxFileSize  int64    // Maximum file size in bytes (0 = unlimited)
}

// Name returns the capability name
func (c *FSReadCapability) Name() string {
	return "fs.read"
}

// Validate checks if the capability configuration is valid
func (c *FSReadCapability) Validate() error {
	if len(c.AllowedPaths) == 0 {
		return fmt.Errorf("%w: at least one allowed path required", ErrInvalidConfiguration)
	}

	// Validate glob patterns
	for _, pattern := range c.AllowedPaths {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("%w: invalid allowed path pattern %s: %v", ErrInvalidConfiguration, pattern, err)
		}
	}

	for _, pattern := range c.DeniedPaths {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("%w: invalid denied path pattern %s: %v", ErrInvalidConfiguration, pattern, err)
		}
	}

	if c.MaxFileSize < 0 {
		return fmt.Errorf("%w: max file size cannot be negative", ErrInvalidConfiguration)
	}

	return nil
}

// CheckPath validates if a path is allowed
func (c *FSReadCapability) CheckPath(path string) error {
	// Clean the path
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}

	// Check denied paths first
	for _, pattern := range c.DeniedPaths {
		matched, err := filepath.Match(pattern, cleanPath)
		if err == nil && matched {
			return fmt.Errorf("%w: %s matches denied pattern %s", ErrPathDenied, cleanPath, pattern)
		}
	}

	// Check allowed paths
	allowed := false
	for _, pattern := range c.AllowedPaths {
		// Handle ** wildcard for recursive matching
		if strings.Contains(pattern, "**") {
			if matchesRecursive(pattern, cleanPath) {
				allowed = true
				break
			}
		} else {
			matched, err := filepath.Match(pattern, cleanPath)
			if err == nil && matched {
				allowed = true
				break
			}
		}
	}

	if !allowed {
		return fmt.Errorf("%w: %s", ErrPathNotAllowed, cleanPath)
	}

	return nil
}

// ReadFile reads a file if the path is allowed
func (c *FSReadCapability) ReadFile(ctx *CapabilityContext, path string) ([]byte, error) {
	if err := c.CheckPath(path); err != nil {
		return nil, err
	}

	// Check file size if limit is set
	if c.MaxFileSize > 0 {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("failed to stat file: %w", err)
		}

		if info.Size() > c.MaxFileSize {
			return nil, fmt.Errorf("%w: file size %d exceeds limit %d", ErrMaxSizeExceeded, info.Size(), c.MaxFileSize)
		}
	}

	// Read the file
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}

	return data, nil
}

// OpenFile opens a file for reading if the path is allowed
func (c *FSReadCapability) OpenFile(ctx *CapabilityContext, path string) (*os.File, error) {
	if err := c.CheckPath(path); err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open file: %w", err)
	}

	return file, nil
}

// FSWriteCapability allows writing files to the filesystem
type FSWriteCapability struct {
	AllowedPaths []string // Glob patterns for allowed paths
	DeniedPaths  []string // Glob patterns for denied paths
	MaxFileSize  int64    // Maximum file size in bytes (0 = unlimited)
}

// Name returns the capability name
func (c *FSWriteCapability) Name() string {
	return "fs.write"
}

// Validate checks if the capability configuration is valid
func (c *FSWriteCapability) Validate() error {
	if len(c.AllowedPaths) == 0 {
		return fmt.Errorf("%w: at least one allowed path required", ErrInvalidConfiguration)
	}

	// Validate glob patterns
	for _, pattern := range c.AllowedPaths {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("%w: invalid allowed path pattern %s: %v", ErrInvalidConfiguration, pattern, err)
		}
	}

	for _, pattern := range c.DeniedPaths {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("%w: invalid denied path pattern %s: %v", ErrInvalidConfiguration, pattern, err)
		}
	}

	if c.MaxFileSize < 0 {
		return fmt.Errorf("%w: max file size cannot be negative", ErrInvalidConfiguration)
	}

	return nil
}

// CheckPath validates if a path is allowed
func (c *FSWriteCapability) CheckPath(path string) error {
	// Clean the path
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}

	// Check denied paths first
	for _, pattern := range c.DeniedPaths {
		matched, err := filepath.Match(pattern, cleanPath)
		if err == nil && matched {
			return fmt.Errorf("%w: %s matches denied pattern %s", ErrPathDenied, cleanPath, pattern)
		}
	}

	// Check allowed paths
	allowed := false
	for _, pattern := range c.AllowedPaths {
		// Handle ** wildcard for recursive matching
		if strings.Contains(pattern, "**") {
			if matchesRecursive(pattern, cleanPath) {
				allowed = true
				break
			}
		} else {
			matched, err := filepath.Match(pattern, cleanPath)
			if err == nil && matched {
				allowed = true
				break
			}
		}
	}

	if !allowed {
		return fmt.Errorf("%w: %s", ErrPathNotAllowed, cleanPath)
	}

	return nil
}

// WriteFile writes data to a file if the path is allowed
func (c *FSWriteCapability) WriteFile(ctx *CapabilityContext, path string, data []byte, perm os.FileMode) error {
	if err := c.CheckPath(path); err != nil {
		return err
	}

	// Check file size if limit is set
	if c.MaxFileSize > 0 && int64(len(data)) > c.MaxFileSize {
		return fmt.Errorf("%w: data size %d exceeds limit %d", ErrMaxSizeExceeded, len(data), c.MaxFileSize)
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write the file
	if err := os.WriteFile(path, data, perm); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// CreateFile creates a file for writing if the path is allowed
func (c *FSWriteCapability) CreateFile(ctx *CapabilityContext, path string, perm os.FileMode) (*os.File, error) {
	if err := c.CheckPath(path); err != nil {
		return nil, err
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, perm)
	if err != nil {
		return nil, fmt.Errorf("failed to create file: %w", err)
	}

	return file, nil
}

// AppendFile appends data to a file if the path is allowed
func (c *FSWriteCapability) AppendFile(ctx *CapabilityContext, path string, data []byte, perm os.FileMode) error {
	if err := c.CheckPath(path); err != nil {
		return err
	}

	// Check existing file size if limit is set
	if c.MaxFileSize > 0 {
		info, err := os.Stat(path)
		if err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to stat file: %w", err)
		}

		existingSize := int64(0)
		if info != nil {
			existingSize = info.Size()
		}

		if existingSize+int64(len(data)) > c.MaxFileSize {
			return fmt.Errorf("%w: final size %d would exceed limit %d", ErrMaxSizeExceeded, existingSize+int64(len(data)), c.MaxFileSize)
		}
	}

	// Ensure parent directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Open file for appending
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, perm)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	// Write data
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("failed to write data: %w", err)
	}

	return nil
}

// DeleteFile deletes a file if the path is allowed
func (c *FSWriteCapability) DeleteFile(ctx *CapabilityContext, path string) error {
	if err := c.CheckPath(path); err != nil {
		return err
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	return nil
}

// Mkdir creates a directory if the path is allowed
func (c *FSWriteCapability) Mkdir(ctx *CapabilityContext, path string, perm os.FileMode) error {
	if err := c.CheckPath(path); err != nil {
		return err
	}

	if err := os.Mkdir(path, perm); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	return nil
}

// MkdirAll creates a directory and all parent directories if the path is allowed
func (c *FSWriteCapability) MkdirAll(ctx *CapabilityContext, path string, perm os.FileMode) error {
	if err := c.CheckPath(path); err != nil {
		return err
	}

	if err := os.MkdirAll(path, perm); err != nil {
		return fmt.Errorf("failed to create directories: %w", err)
	}

	return nil
}

// CopyFile copies a file from src to dst if both paths are allowed
func (c *FSWriteCapability) CopyFile(ctx *CapabilityContext, readCap *FSReadCapability, src, dst string) error {
	// Check read capability for source
	if readCap == nil {
		return fmt.Errorf("read capability required for source file")
	}

	if err := readCap.CheckPath(src); err != nil {
		return fmt.Errorf("source: %w", err)
	}

	// Check write capability for destination
	if err := c.CheckPath(dst); err != nil {
		return fmt.Errorf("destination: %w", err)
	}

	// Open source file
	srcFile, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("failed to open source: %w", err)
	}
	defer srcFile.Close()

	// Check file size
	if c.MaxFileSize > 0 || readCap.MaxFileSize > 0 {
		info, err := srcFile.Stat()
		if err != nil {
			return fmt.Errorf("failed to stat source: %w", err)
		}

		maxSize := c.MaxFileSize
		if readCap.MaxFileSize > 0 && (maxSize == 0 || readCap.MaxFileSize < maxSize) {
			maxSize = readCap.MaxFileSize
		}

		if maxSize > 0 && info.Size() > maxSize {
			return fmt.Errorf("%w: file size %d exceeds limit %d", ErrMaxSizeExceeded, info.Size(), maxSize)
		}
	}

	// Ensure destination directory exists
	dstDir := filepath.Dir(dst)
	if err := os.MkdirAll(dstDir, 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	// Create destination file
	dstFile, err := os.Create(dst)
	if err != nil {
		return fmt.Errorf("failed to create destination: %w", err)
	}
	defer dstFile.Close()

	// Copy data
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return fmt.Errorf("failed to copy data: %w", err)
	}

	return nil
}

// matchesRecursive checks if a path matches a pattern with ** wildcards
func matchesRecursive(pattern, path string) bool {
	// Split pattern and path
	patternParts := strings.Split(filepath.Clean(pattern), string(filepath.Separator))
	pathParts := strings.Split(filepath.Clean(path), string(filepath.Separator))

	return matchesRecursiveParts(patternParts, pathParts)
}

func matchesRecursiveParts(pattern, path []string) bool {
	if len(pattern) == 0 {
		return len(path) == 0
	}

	if pattern[0] == "**" {
		// ** matches zero or more path segments
		// Try matching rest of pattern with different amounts of path consumed
		for i := 0; i <= len(path); i++ {
			if matchesRecursiveParts(pattern[1:], path[i:]) {
				return true
			}
		}
		return false
	}

	if len(path) == 0 {
		return false
	}

	// Try to match current segment
	matched, err := filepath.Match(pattern[0], path[0])
	if err != nil || !matched {
		return false
	}

	// Recurse on remaining segments
	return matchesRecursiveParts(pattern[1:], path[1:])
}
