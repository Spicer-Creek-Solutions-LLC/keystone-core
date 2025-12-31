package capabilities

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Default security limits for filesystem capabilities
const (
	// DefaultMaxFileSize is 10MB - a reasonable default for most use cases
	DefaultMaxFileSize = 10 * 1024 * 1024
	// MaxAllowedFileSize is 100MB - the absolute maximum allowed
	MaxAllowedFileSize = 100 * 1024 * 1024
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

	// Validate glob patterns and check for dangerous patterns
	for _, pattern := range c.AllowedPaths {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("%w: invalid allowed path pattern %s: %v", ErrInvalidConfiguration, pattern, err)
		}
		// Check for overly broad patterns
		if err := validatePathPatternSecurity(pattern); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidConfiguration, err)
		}
	}

	for _, pattern := range c.DeniedPaths {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("%w: invalid denied path pattern %s: %v", ErrInvalidConfiguration, pattern, err)
		}
	}

	// MaxFileSize must be set and within bounds for security
	if c.MaxFileSize <= 0 {
		return fmt.Errorf("%w: max file size must be set (got %d, use DefaultMaxFileSize=%d or specify explicitly)",
			ErrInvalidConfiguration, c.MaxFileSize, DefaultMaxFileSize)
	}
	if c.MaxFileSize > MaxAllowedFileSize {
		return fmt.Errorf("%w: max file size %d exceeds maximum allowed %d",
			ErrInvalidConfiguration, c.MaxFileSize, MaxAllowedFileSize)
	}

	return nil
}

// CheckPath validates if a path is allowed
func (c *FSReadCapability) CheckPath(path string) error {
	// Clean the path and resolve symlinks to prevent escape attacks
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}

	// Resolve symlinks to get the real path - prevents symlink escape attacks
	// If the file doesn't exist yet, we can still validate the cleaned path
	realPath, err := filepath.EvalSymlinks(cleanPath)
	if err != nil {
		// If file doesn't exist, that's ok for path checking
		// But if it's another error, we should still use cleanPath
		if !os.IsNotExist(err) {
			// For other errors (e.g., permission denied), use the cleaned path
			realPath = cleanPath
		} else {
			realPath = cleanPath
		}
	}

	// Check denied paths first (against both cleaned and real paths)
	for _, pattern := range c.DeniedPaths {
		matched, err := filepath.Match(pattern, realPath)
		if err == nil && matched {
			return fmt.Errorf("%w: %s matches denied pattern %s", ErrPathDenied, realPath, pattern)
		}
		// Also check the original clean path in case symlink resolution changed it
		if cleanPath != realPath {
			matched, err = filepath.Match(pattern, cleanPath)
			if err == nil && matched {
				return fmt.Errorf("%w: %s matches denied pattern %s", ErrPathDenied, cleanPath, pattern)
			}
		}
	}

	// Check allowed paths (use real path for security)
	allowed := false
	for _, pattern := range c.AllowedPaths {
		// Handle ** wildcard for recursive matching
		if strings.Contains(pattern, "**") {
			if matchesRecursive(pattern, realPath) {
				allowed = true
				break
			}
		} else {
			matched, err := filepath.Match(pattern, realPath)
			if err == nil && matched {
				allowed = true
				break
			}
		}
	}

	if !allowed {
		return fmt.Errorf("%w: %s", ErrPathNotAllowed, realPath)
	}

	return nil
}

// ReadFile reads a file if the path is allowed
func (c *FSReadCapability) ReadFile(ctx *CapabilityContext, path string) ([]byte, error) {
	if err := c.CheckPath(path); err != nil {
		return nil, err
	}

	// Always check file size (MaxFileSize is now required)
	info, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("failed to stat file: %w", err)
	}

	if info.Size() > c.MaxFileSize {
		return nil, fmt.Errorf("%w: file size %d exceeds limit %d", ErrMaxSizeExceeded, info.Size(), c.MaxFileSize)
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

	// Validate glob patterns and check for dangerous patterns
	for _, pattern := range c.AllowedPaths {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("%w: invalid allowed path pattern %s: %v", ErrInvalidConfiguration, pattern, err)
		}
		// Check for overly broad patterns
		if err := validatePathPatternSecurity(pattern); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidConfiguration, err)
		}
	}

	for _, pattern := range c.DeniedPaths {
		if _, err := filepath.Match(pattern, "test"); err != nil {
			return fmt.Errorf("%w: invalid denied path pattern %s: %v", ErrInvalidConfiguration, pattern, err)
		}
	}

	// MaxFileSize must be set and within bounds for security
	if c.MaxFileSize <= 0 {
		return fmt.Errorf("%w: max file size must be set (got %d, use DefaultMaxFileSize=%d or specify explicitly)",
			ErrInvalidConfiguration, c.MaxFileSize, DefaultMaxFileSize)
	}
	if c.MaxFileSize > MaxAllowedFileSize {
		return fmt.Errorf("%w: max file size %d exceeds maximum allowed %d",
			ErrInvalidConfiguration, c.MaxFileSize, MaxAllowedFileSize)
	}

	return nil
}

// CheckPath validates if a path is allowed
func (c *FSWriteCapability) CheckPath(path string) error {
	// Clean the path and resolve symlinks to prevent escape attacks
	cleanPath, err := filepath.Abs(path)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidPath, err)
	}

	// For write operations, we need to check the parent directory if file doesn't exist
	// Resolve symlinks to get the real path - prevents symlink escape attacks
	realPath := cleanPath
	if _, err := os.Stat(cleanPath); err == nil {
		// File exists, resolve symlinks
		resolved, err := filepath.EvalSymlinks(cleanPath)
		if err == nil {
			realPath = resolved
		}
	} else if !os.IsNotExist(err) {
		// Error other than "file not found"
		return fmt.Errorf("%w: cannot stat path: %v", ErrInvalidPath, err)
	} else {
		// File doesn't exist, check parent directory for symlinks
		parentDir := filepath.Dir(cleanPath)
		if resolved, err := filepath.EvalSymlinks(parentDir); err == nil {
			realPath = filepath.Join(resolved, filepath.Base(cleanPath))
		}
	}

	// Check denied paths first (against both cleaned and real paths)
	for _, pattern := range c.DeniedPaths {
		matched, err := filepath.Match(pattern, realPath)
		if err == nil && matched {
			return fmt.Errorf("%w: %s matches denied pattern %s", ErrPathDenied, realPath, pattern)
		}
		if cleanPath != realPath {
			matched, err = filepath.Match(pattern, cleanPath)
			if err == nil && matched {
				return fmt.Errorf("%w: %s matches denied pattern %s", ErrPathDenied, cleanPath, pattern)
			}
		}
	}

	// Check allowed paths (use real path for security)
	allowed := false
	for _, pattern := range c.AllowedPaths {
		// Handle ** wildcard for recursive matching
		if strings.Contains(pattern, "**") {
			if matchesRecursive(pattern, realPath) {
				allowed = true
				break
			}
		} else {
			matched, err := filepath.Match(pattern, realPath)
			if err == nil && matched {
				allowed = true
				break
			}
		}
	}

	if !allowed {
		return fmt.Errorf("%w: %s", ErrPathNotAllowed, realPath)
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

// Dangerous path patterns that should be blocked
var dangerousPathPatterns = []string{
	"/",          // Root filesystem
	"/*",         // Everything in root
	"/**",        // All files recursively from root
	"/etc",       // System configuration
	"/etc/*",     // All system config files
	"/etc/**",    // All system config recursively
	"/var",       // Variable data
	"/var/*",     // All variable data
	"/var/**",    // All variable data recursively
	"/usr",       // User programs
	"/usr/*",     // All user programs
	"/usr/**",    // All user programs recursively
	"/bin",       // Essential binaries
	"/bin/*",     // All essential binaries
	"/sbin",      // System binaries
	"/sbin/*",    // All system binaries
	"/root",      // Root home
	"/root/*",    // All root files
	"/root/**",   // All root files recursively
	"/home",      // All user homes
	"/home/*",    // All user home directories
	"/home/**",   // All user files recursively
	"C:\\",       // Windows root
	"C:\\*",      // Windows root all
	"C:\\**",     // Windows all files
	"C:\\Windows",          // Windows system
	"C:\\Windows\\*",       // Windows system files
	"C:\\Windows\\**",      // Windows system recursively
	"C:\\Windows\\System32", // Windows system32
	"C:\\Windows\\System32\\*",  // All system32 files
	"C:\\Windows\\System32\\**", // System32 recursively
}

// validatePathPatternSecurity checks if a path pattern is overly broad or dangerous
func validatePathPatternSecurity(pattern string) error {
	// Clean the pattern for comparison
	cleanPattern := filepath.Clean(pattern)

	// Check against dangerous patterns
	for _, dangerous := range dangerousPathPatterns {
		if cleanPattern == dangerous || pattern == dangerous {
			return fmt.Errorf("pattern %q is too broad and could access sensitive system areas", pattern)
		}
	}

	// Warn about patterns that are still risky
	// Count path components before **
	parts := strings.Split(cleanPattern, string(filepath.Separator))
	doubleStarIndex := -1
	for i, part := range parts {
		if part == "**" {
			doubleStarIndex = i
			break
		}
	}

	// If ** is used, require at least 2 path components before it for security
	// e.g., /app/** is risky but allowed, /** is not allowed
	if doubleStarIndex >= 0 && doubleStarIndex < 2 {
		// Count non-empty parts before **
		nonEmptyParts := 0
		for i := 0; i < doubleStarIndex; i++ {
			if parts[i] != "" {
				nonEmptyParts++
			}
		}
		if nonEmptyParts < 2 {
			return fmt.Errorf("pattern %q uses ** with insufficient path depth (need at least 2 path components before **)", pattern)
		}
	}

	return nil
}
