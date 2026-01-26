package blueprint

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/mod/semver"
)

// LocalStorage implements Storage using the local filesystem.
// Expected directory structure:
//
//	<basePath>/
//	├── <vendor>/
//	│   └── <name>/
//	│       └── blueprint.yaml
//	└── <vendor>/
//	    └── <name>-<version>/
//	        └── blueprint.yaml
//
// Versioned blueprints use the format: <name>-<version>/
// Unversioned blueprints are treated as development versions.
type LocalStorage struct {
	basePath string
	readOnly bool
}

// NewLocalStorage creates a new local filesystem storage.
func NewLocalStorage(basePath string, readOnly bool) (*LocalStorage, error) {
	// Ensure base path exists
	if _, err := os.Stat(basePath); os.IsNotExist(err) {
		if readOnly {
			return nil, fmt.Errorf("base path does not exist: %s", basePath)
		}
		if err := os.MkdirAll(basePath, 0755); err != nil {
			return nil, fmt.Errorf("failed to create base path: %w", err)
		}
	}

	absPath, err := filepath.Abs(basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to get absolute path: %w", err)
	}

	return &LocalStorage{
		basePath: absPath,
		readOnly: readOnly,
	}, nil
}

// Get retrieves a blueprint by name and optional version.
func (s *LocalStorage) Get(ctx context.Context, name string, version string) (*Blueprint, error) {
	// Parse the blueprint name to extract vendor/name
	vendor, bpName, err := parseBlueprintName(name)
	if err != nil {
		return nil, err
	}

	// Find the blueprint directory
	bpDir, err := s.findBlueprintDir(vendor, bpName, version)
	if err != nil {
		return nil, err
	}

	// Parse the manifest
	manifestPath := filepath.Join(bpDir, "blueprint.yaml")
	bp, err := ParseManifestFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to parse blueprint manifest: %w", err)
	}

	return bp, nil
}

// List returns all blueprints matching the optional filter.
func (s *LocalStorage) List(ctx context.Context, filter *ListFilter) ([]*BlueprintInfo, error) {
	var results []*BlueprintInfo

	// Walk the vendor directories
	vendors, err := os.ReadDir(s.basePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read base path: %w", err)
	}

	for _, vendorEntry := range vendors {
		if !vendorEntry.IsDir() {
			continue
		}

		vendor := vendorEntry.Name()

		// Skip if vendor filter doesn't match
		if filter != nil && filter.Vendor != "" && filter.Vendor != vendor {
			continue
		}

		vendorPath := filepath.Join(s.basePath, vendor)
		blueprints, err := os.ReadDir(vendorPath)
		if err != nil {
			continue
		}

		// Group blueprints by name (ignoring version suffix)
		bpGroups := make(map[string][]string)
		for _, bpEntry := range blueprints {
			if !bpEntry.IsDir() {
				continue
			}

			bpName, version := parseVersionedDir(bpEntry.Name())
			bpGroups[bpName] = append(bpGroups[bpName], version)
		}

		// Create BlueprintInfo for each blueprint
		for bpName, versions := range bpGroups {
			info, err := s.getBlueprintInfo(vendor, bpName, versions)
			if err != nil {
				continue
			}

			// Apply keyword filter
			if filter != nil && len(filter.Keywords) > 0 {
				if !matchesKeywords(info.Keywords, filter.Keywords) {
					continue
				}
			}

			// Apply category filter
			if filter != nil && len(filter.Categories) > 0 {
				if !matchesCategories(info.Categories, filter.Categories) {
					continue
				}
			}

			results = append(results, info)
		}
	}

	// Apply pagination
	if filter != nil {
		if filter.Offset > 0 && filter.Offset < len(results) {
			results = results[filter.Offset:]
		}
		if filter.Limit > 0 && filter.Limit < len(results) {
			results = results[:filter.Limit]
		}
	}

	return results, nil
}

// Versions returns all available versions for a blueprint.
func (s *LocalStorage) Versions(ctx context.Context, name string) ([]string, error) {
	vendor, bpName, err := parseBlueprintName(name)
	if err != nil {
		return nil, err
	}

	vendorPath := filepath.Join(s.basePath, vendor)
	entries, err := os.ReadDir(vendorPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrBlueprintNotFound
		}
		return nil, fmt.Errorf("failed to read vendor directory: %w", err)
	}

	var versions []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirBpName, version := parseVersionedDir(entry.Name())
		if dirBpName == bpName {
			if version != "" {
				versions = append(versions, version)
			} else {
				// Check if it has a blueprint.yaml with version
				manifestPath := filepath.Join(vendorPath, entry.Name(), "blueprint.yaml")
				bp, err := ParseManifestFile(manifestPath)
				if err == nil && bp.Metadata.Version != "" {
					versions = append(versions, bp.Metadata.Version)
				}
			}
		}
	}

	if len(versions) == 0 {
		return nil, ErrBlueprintNotFound
	}

	// Sort versions in descending order (newest first)
	sortVersionsDesc(versions)

	return versions, nil
}

// Exists checks if a blueprint exists.
func (s *LocalStorage) Exists(ctx context.Context, name string, version string) (bool, error) {
	vendor, bpName, err := parseBlueprintName(name)
	if err != nil {
		return false, nil // Invalid name doesn't exist
	}

	_, err = s.findBlueprintDir(vendor, bpName, version)
	return err == nil, nil
}

// Close releases any resources held by the storage.
func (s *LocalStorage) Close() error {
	return nil // Nothing to close for local storage
}

// Put stores a blueprint.
func (s *LocalStorage) Put(ctx context.Context, bp *Blueprint) error {
	if s.readOnly {
		return ErrStorageReadOnly
	}

	// Validate the blueprint first
	validator := NewValidator()
	result := validator.Validate(bp)
	if !result.Valid {
		return result.Error()
	}

	// Extract vendor from source path or use "local"
	vendor := "local"
	if bp.SourcePath != "" {
		parts := strings.Split(filepath.ToSlash(bp.SourcePath), "/")
		for i, part := range parts {
			if part == "blueprints" && i+1 < len(parts) {
				vendor = parts[i+1]
				break
			}
		}
	}

	// Create the directory
	dirName := fmt.Sprintf("%s-%s", bp.Metadata.Name, bp.Metadata.Version)
	bpDir := filepath.Join(s.basePath, vendor, dirName)

	if err := os.MkdirAll(bpDir, 0755); err != nil {
		return fmt.Errorf("failed to create blueprint directory: %w", err)
	}

	// Write the manifest
	data, err := bp.Marshal()
	if err != nil {
		return fmt.Errorf("failed to marshal blueprint: %w", err)
	}

	manifestPath := filepath.Join(bpDir, "blueprint.yaml")
	if err := os.WriteFile(manifestPath, data, 0644); err != nil {
		return fmt.Errorf("failed to write manifest: %w", err)
	}

	return nil
}

// Delete removes a blueprint version.
func (s *LocalStorage) Delete(ctx context.Context, name string, version string) error {
	if s.readOnly {
		return ErrStorageReadOnly
	}

	vendor, bpName, err := parseBlueprintName(name)
	if err != nil {
		return err
	}

	bpDir, err := s.findBlueprintDir(vendor, bpName, version)
	if err != nil {
		return err
	}

	return os.RemoveAll(bpDir)
}

// GetFile retrieves a file from a blueprint.
func (s *LocalStorage) GetFile(ctx context.Context, name, version, path string) (io.ReadCloser, error) {
	vendor, bpName, err := parseBlueprintName(name)
	if err != nil {
		return nil, err
	}

	bpDir, err := s.findBlueprintDir(vendor, bpName, version)
	if err != nil {
		return nil, err
	}

	// Security: ensure path doesn't escape blueprint directory
	fullPath := filepath.Join(bpDir, filepath.Clean(path))
	if !strings.HasPrefix(fullPath, bpDir) {
		return nil, fmt.Errorf("path traversal detected: %s", path)
	}

	return os.Open(fullPath)
}

// ListFiles lists files in a blueprint directory.
func (s *LocalStorage) ListFiles(ctx context.Context, name, version, path string) ([]FileInfo, error) {
	vendor, bpName, err := parseBlueprintName(name)
	if err != nil {
		return nil, err
	}

	bpDir, err := s.findBlueprintDir(vendor, bpName, version)
	if err != nil {
		return nil, err
	}

	// Security: ensure path doesn't escape blueprint directory
	dirPath := filepath.Join(bpDir, filepath.Clean(path))
	if !strings.HasPrefix(dirPath, bpDir) {
		return nil, fmt.Errorf("path traversal detected: %s", path)
	}

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory: %w", err)
	}

	var files []FileInfo
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		files = append(files, FileInfo{
			Name:  entry.Name(),
			Path:  filepath.Join(path, entry.Name()),
			Size:  info.Size(),
			IsDir: entry.IsDir(),
		})
	}

	return files, nil
}

// findBlueprintDir finds the directory for a blueprint version.
func (s *LocalStorage) findBlueprintDir(vendor, name, version string) (string, error) {
	vendorPath := filepath.Join(s.basePath, vendor)

	// If version is specified, look for exact match
	if version != "" {
		// Try versioned directory first
		versionedDir := filepath.Join(vendorPath, fmt.Sprintf("%s-%s", name, version))
		if _, err := os.Stat(filepath.Join(versionedDir, "blueprint.yaml")); err == nil {
			return versionedDir, nil
		}

		// Try unversioned directory and check manifest version
		unversionedDir := filepath.Join(vendorPath, name)
		if bp, err := ParseManifestFile(filepath.Join(unversionedDir, "blueprint.yaml")); err == nil {
			if bp.Metadata.Version == version {
				return unversionedDir, nil
			}
		}

		return "", ErrVersionNotFound
	}

	// No version specified, find the latest
	entries, err := os.ReadDir(vendorPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", ErrBlueprintNotFound
		}
		return "", fmt.Errorf("failed to read vendor directory: %w", err)
	}

	var latestDir string
	var latestVersion string

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		dirName := entry.Name()
		dirBpName, dirVersion := parseVersionedDir(dirName)

		if dirBpName != name {
			continue
		}

		// Check if this directory has a blueprint.yaml
		manifestPath := filepath.Join(vendorPath, dirName, "blueprint.yaml")
		if _, err := os.Stat(manifestPath); err != nil {
			continue
		}

		// If no version in dir name, read from manifest
		if dirVersion == "" {
			bp, err := ParseManifestFile(manifestPath)
			if err == nil {
				dirVersion = bp.Metadata.Version
			}
		}

		// Compare versions
		if latestVersion == "" || compareVersions(dirVersion, latestVersion) > 0 {
			latestVersion = dirVersion
			latestDir = filepath.Join(vendorPath, dirName)
		}
	}

	if latestDir == "" {
		return "", ErrBlueprintNotFound
	}

	return latestDir, nil
}

// getBlueprintInfo creates a BlueprintInfo from a blueprint.
func (s *LocalStorage) getBlueprintInfo(vendor, name string, versions []string) (*BlueprintInfo, error) {
	// Sort versions descending
	sortVersionsDesc(versions)

	// Use latest version for metadata
	latestVersion := ""
	if len(versions) > 0 {
		latestVersion = versions[0]
	}

	bpDir, err := s.findBlueprintDir(vendor, name, latestVersion)
	if err != nil {
		return nil, err
	}

	bp, err := ParseManifestFile(filepath.Join(bpDir, "blueprint.yaml"))
	if err != nil {
		return nil, err
	}

	return &BlueprintInfo{
		Name:              fmt.Sprintf("blueprints/%s/%s", vendor, name),
		Version:           bp.Metadata.Version,
		Description:       bp.Metadata.Description,
		Keywords:          bp.Metadata.Keywords,
		Categories:        bp.Metadata.Categories,
		Maintainers:       bp.Metadata.Maintainers,
		License:           bp.Metadata.License,
		AvailableVersions: versions,
	}, nil
}

// parseBlueprintName parses a blueprint name into vendor and name.
// Expected format: blueprints/<vendor>/<name> or <vendor>/<name>
func parseBlueprintName(fullName string) (vendor, name string, err error) {
	// Remove "blueprints/" prefix if present
	fullName = strings.TrimPrefix(fullName, "blueprints/")

	parts := strings.Split(fullName, "/")
	if len(parts) != 2 {
		return "", "", ErrInvalidReference
	}

	return parts[0], parts[1], nil
}

// parseVersionedDir extracts name and version from a directory name.
// Format: <name>-<version> or <name>
func parseVersionedDir(dirName string) (name, version string) {
	// Look for semver pattern at the end
	parts := strings.Split(dirName, "-")
	if len(parts) >= 2 {
		// Check if last part(s) look like a version
		for i := len(parts) - 1; i >= 1; i-- {
			potentialVersion := strings.Join(parts[i:], "-")
			if semver.IsValid("v" + potentialVersion) {
				return strings.Join(parts[:i], "-"), potentialVersion
			}
		}
	}
	return dirName, ""
}

// compareVersions compares two version strings.
// Returns: -1 if a < b, 0 if a == b, 1 if a > b
func compareVersions(a, b string) int {
	// Ensure "v" prefix for semver package
	va := a
	if !strings.HasPrefix(va, "v") {
		va = "v" + va
	}
	vb := b
	if !strings.HasPrefix(vb, "v") {
		vb = "v" + vb
	}

	return semver.Compare(va, vb)
}

// sortVersionsDesc sorts versions in descending order (newest first).
func sortVersionsDesc(versions []string) {
	sort.Slice(versions, func(i, j int) bool {
		return compareVersions(versions[i], versions[j]) > 0
	})
}

// matchesKeywords checks if any of the haystack keywords match the needle keywords.
func matchesKeywords(haystack, needles []string) bool {
	for _, needle := range needles {
		needle = strings.ToLower(needle)
		for _, kw := range haystack {
			if strings.Contains(strings.ToLower(kw), needle) {
				return true
			}
		}
	}
	return false
}

// matchesCategories checks if any of the haystack categories match the needle categories.
func matchesCategories(haystack, needles []string) bool {
	haystackSet := make(map[string]bool)
	for _, cat := range haystack {
		haystackSet[strings.ToLower(cat)] = true
	}

	for _, needle := range needles {
		if haystackSet[strings.ToLower(needle)] {
			return true
		}
	}
	return false
}
