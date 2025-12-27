package statemgmt

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// FileModule implements file and directory management
type FileModule struct {
	*BaseModule
}

// NewFileModule creates a new file module
func NewFileModule() *FileModule {
	return &FileModule{
		BaseModule: NewBaseModule("file", []string{"present", "absent", "directory", "symlink"}),
	}
}

// normalizePath normalizes a path for the current operating system
func (m *FileModule) normalizePath(path string) string {
	// Convert forward slashes to backslashes on Windows
	if runtime.GOOS == "windows" {
		// Replace Unix-style paths with Windows-style
		path = strings.ReplaceAll(path, "/", "\\")
	}

	// Use filepath.Clean to normalize the path
	return filepath.Clean(path)
}

// isSymlinkSupported checks if symlinks are supported and practical
func (m *FileModule) isSymlinkSupported() bool {
	// Use platform-specific implementation
	// On Windows, symlinks require elevated privileges
	return isSymlinkFullySupported()
}

// Check checks the current state of a file/directory
func (m *FileModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	// Normalize path for the current OS
	normalizedPath := m.normalizePath(decl.ID)

	// Check if file exists
	info, err := os.Lstat(normalizedPath)
	if err != nil {
		if os.IsNotExist(err) {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (decl.State == "absent")
			return result, nil
		}
		return nil, fmt.Errorf("failed to stat %s: %w", normalizedPath, err)
	}

	result.Present = true
	result.Metadata["size"] = info.Size()
	result.Metadata["modified"] = info.ModTime()

	// Determine current state
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		result.CurrentState = "symlink"
		target, _ := os.Readlink(normalizedPath)
		result.Metadata["target"] = target
	case info.IsDir():
		result.CurrentState = "directory"
	default:
		result.CurrentState = "present"
	}

	// Check if state matches
	switch decl.State {
	case "absent":
		result.Matches = false
		result.Diff["state"] = map[string]string{"current": result.CurrentState, "desired": "absent"}

	case "present":
		result.Matches = true
		if info.IsDir() {
			result.Matches = false
			result.Diff["type"] = map[string]string{"current": "directory", "desired": "file"}
		}
		// Check mode, owner, group, content
		if !m.checkAttributes(normalizedPath, decl, info, result) {
			result.Matches = false
		}

	case "directory":
		result.Matches = info.IsDir()
		if !info.IsDir() {
			result.Diff["type"] = map[string]string{"current": "file", "desired": "directory"}
		} else {
			// Check mode, owner, group
			if !m.checkAttributes(normalizedPath, decl, info, result) {
				result.Matches = false
			}
		}

	case "symlink":
		// Check if symlinks are supported on this platform
		if !m.isSymlinkSupported() {
			result.Matches = false
			result.Diff["platform"] = "symlinks not supported on Windows without elevated privileges"
		} else {
			result.Matches = (info.Mode()&os.ModeSymlink != 0)
			if !result.Matches {
				result.Diff["type"] = map[string]string{"current": result.CurrentState, "desired": "symlink"}
			} else {
				// Check target
				target, _ := os.Readlink(normalizedPath)
				desiredTarget := getStringParameter(decl, "target", "")
				if desiredTarget != "" && target != desiredTarget {
					result.Matches = false
					result.Diff["target"] = map[string]string{"current": target, "desired": desiredTarget}
				}
			}
		}
	}

	return result, nil
}

// checkAttributes checks file attributes (mode, owner, content)
func (m *FileModule) checkAttributes(path string, decl *StateDeclaration, info os.FileInfo, result *ModuleCheckResult) bool {
	matches := true

	// Check mode
	if modeStr := getStringParameter(decl, "mode", ""); modeStr != "" {
		desiredMode, err := strconv.ParseUint(modeStr, 8, 32)
		if err == nil {
			currentMode := uint32(info.Mode().Perm())
			if currentMode != uint32(desiredMode) {
				matches = false
				result.Diff["mode"] = map[string]string{
					"current": fmt.Sprintf("%04o", currentMode),
					"desired": fmt.Sprintf("%04o", desiredMode),
				}
			}
		}
	}

	// Check owner/group (Unix only - skip on Windows)
	if isOwnershipSupported() {
		if uid, gid, ok := getFileOwnership(info); ok {
			// Check user
			if user := getStringParameter(decl, "user", ""); user != "" {
				result.Metadata["uid"] = uid
				// Would need to resolve username to UID for comparison
				// For now, just store it
			}

			// Check group
			if group := getStringParameter(decl, "group", ""); group != "" {
				result.Metadata["gid"] = gid
				// Would need to resolve group name to GID for comparison
			}
		}
	}

	// Check content hash for files
	if !info.IsDir() && decl.State == "present" {
		if contents := getStringParameter(decl, "contents", ""); contents != "" {
			currentHash, err := m.hashFile(path)
			if err == nil {
				desiredHash := m.hashString(contents)
				if currentHash != desiredHash {
					matches = false
					result.Diff["contents"] = "content differs"
				}
			}
		}

		// Check source file
		if source := getStringParameter(decl, "source", ""); source != "" {
			sourceType, sourcePath := ParseSource(source)
			if sourceType == SourceTypeFile {
				normalizedSourcePath := m.normalizePath(sourcePath)
				sourceHash, err := m.hashFile(normalizedSourcePath)
				if err == nil {
					currentHash, err := m.hashFile(path)
					if err == nil && currentHash != sourceHash {
						matches = false
						result.Diff["contents"] = "differs from source"
					}
				}
			}
		}
	}

	return matches
}

// Apply applies the file state
func (m *FileModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		Success:   false,
		Changed:   false,
		Changes:   make(map[string]interface{}),
		StartTime: startTime,
	}

	// Normalize path for the current OS
	normalizedPath := m.normalizePath(decl.ID)

	// Check current state first
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Error = err
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// If already in desired state, no changes needed
	if checkResult.Matches {
		result.Success = true
		result.Changed = false
		result.Comment = "Already in desired state"
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	var applyErr error
	switch decl.State {
	case "absent":
		applyErr = m.applyAbsent(normalizedPath, decl, result)
	case "present":
		applyErr = m.applyPresent(normalizedPath, decl, result)
	case "directory":
		applyErr = m.applyDirectory(normalizedPath, decl, result)
	case "symlink":
		applyErr = m.applySymlink(normalizedPath, decl, result)
	default:
		applyErr = fmt.Errorf("unsupported state: %s", decl.State)
	}

	if applyErr != nil {
		result.Error = applyErr
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to apply state: %v", applyErr)
	} else {
		result.Success = true
		result.Changed = true
		result.Changes = checkResult.Diff
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(startTime)
	return result, nil
}

// applyAbsent removes a file/directory
func (m *FileModule) applyAbsent(path string, decl *StateDeclaration, result *StateResult) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			result.Comment = "Already absent"
			return nil
		}
		return err
	}

	if info.IsDir() {
		if err := os.RemoveAll(path); err != nil {
			return fmt.Errorf("failed to remove directory: %w", err)
		}
		result.Comment = "Directory removed"
	} else {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove file: %w", err)
		}
		result.Comment = "File removed"
	}

	return nil
}

// applyPresent creates or updates a file
func (m *FileModule) applyPresent(path string, decl *StateDeclaration, result *StateResult) error {
	// Create parent directories if needed
	if getBoolParameter(decl, "makedirs", false) {
		dir := filepath.Dir(path)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create parent directories: %w", err)
		}
	}

	// Determine content source
	var content []byte
	var err error

	if contents := getStringParameter(decl, "contents", ""); contents != "" {
		content = []byte(contents)
	} else if source := getStringParameter(decl, "source", ""); source != "" {
		sourceType, sourcePath := ParseSource(source)
		if sourceType == SourceTypeFile {
			normalizedSourcePath := m.normalizePath(sourcePath)
			content, err = os.ReadFile(normalizedSourcePath)
			if err != nil {
				return fmt.Errorf("failed to read source file: %w", err)
			}
		} else {
			return fmt.Errorf("unsupported source type: %s", sourceType)
		}
	}

	// Write file
	if content != nil {
		// Backup existing file if requested
		if getBoolParameter(decl, "backup", false) {
			if _, err := os.Stat(path); err == nil {
				backupPath := path + ".backup"
				if err := os.Rename(path, backupPath); err != nil {
					return fmt.Errorf("failed to backup file: %w", err)
				}
			}
		}

		if err := os.WriteFile(path, content, 0644); err != nil {
			return fmt.Errorf("failed to write file: %w", err)
		}
		result.Comment = "File created/updated"
	} else {
		// Just create empty file if it doesn't exist
		if _, err := os.Stat(path); os.IsNotExist(err) {
			if err := os.WriteFile(path, []byte{}, 0644); err != nil {
				return fmt.Errorf("failed to create file: %w", err)
			}
			result.Comment = "Empty file created"
		}
	}

	// Set permissions
	if modeStr := getStringParameter(decl, "mode", ""); modeStr != "" {
		mode, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid mode: %w", err)
		}
		if err := os.Chmod(path, os.FileMode(mode)); err != nil {
			return fmt.Errorf("failed to set mode: %w", err)
		}
	}

	return nil
}

// applyDirectory creates a directory
func (m *FileModule) applyDirectory(path string, decl *StateDeclaration, result *StateResult) error {
	// Get desired mode
	mode := os.FileMode(0755)
	if modeStr := getStringParameter(decl, "mode", ""); modeStr != "" {
		modeInt, err := strconv.ParseUint(modeStr, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid mode: %w", err)
		}
		mode = os.FileMode(modeInt)
	}

	// Create directory
	if err := os.MkdirAll(path, mode); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	result.Comment = "Directory created"
	return nil
}

// applySymlink creates a symlink
func (m *FileModule) applySymlink(path string, decl *StateDeclaration, result *StateResult) error {
	// Check if symlinks are supported
	if !m.isSymlinkSupported() {
		return fmt.Errorf("symlinks are not supported on Windows without elevated privileges")
	}

	target := getStringParameter(decl, "target", "")
	if target == "" {
		return fmt.Errorf("symlink requires 'target' parameter")
	}

	// Normalize the target path as well
	normalizedTarget := m.normalizePath(target)

	// Remove existing file if it exists
	if _, err := os.Lstat(path); err == nil {
		if err := os.Remove(path); err != nil {
			return fmt.Errorf("failed to remove existing file: %w", err)
		}
	}

	// Create symlink
	if err := os.Symlink(normalizedTarget, path); err != nil {
		return fmt.Errorf("failed to create symlink: %w", err)
	}

	result.Comment = fmt.Sprintf("Symlink created to %s", normalizedTarget)
	return nil
}

// Test tests if the file state is correct
func (m *FileModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return checkResult.Matches, nil
}

// hashFile calculates SHA256 hash of a file
func (m *FileModule) hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// hashString calculates SHA256 hash of a string
func (m *FileModule) hashString(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return fmt.Sprintf("%x", h.Sum(nil))
}

func init() {
	RegisterModule(NewFileModule())
}
