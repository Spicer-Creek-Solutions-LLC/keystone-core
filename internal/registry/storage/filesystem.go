package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// FilesystemBackend stores modules on the local filesystem.
type FilesystemBackend struct {
	root string
	mu   sync.RWMutex
}

// NewFilesystemBackend creates a new filesystem-backed storage.
// It creates the root directory if it doesn't exist.
func NewFilesystemBackend(root string) (*FilesystemBackend, error) {
	//nolint:gosec // G301: registry directory needs to be accessible by service user
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("create root directory: %w", err)
	}
	return &FilesystemBackend{root: root}, nil
}

// ListVersions returns all versions for a module, sorted descending.
func (f *FilesystemBackend) ListVersions(_ context.Context, moduleName string) ([]string, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	moduleDir := filepath.Join(f.root, moduleName)
	entries, err := os.ReadDir(moduleDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrModuleNotFound
		}
		return nil, fmt.Errorf("list versions: %w", err)
	}

	var versions []string
	for _, entry := range entries {
		if entry.IsDir() {
			zipPath := filepath.Join(moduleDir, entry.Name(), "module.zip")
			if _, err := os.Stat(zipPath); err == nil {
				versions = append(versions, entry.Name())
			}
		}
	}

	sort.Sort(sort.Reverse(sort.StringSlice(versions)))
	return versions, nil
}

// GetInfo returns module metadata for a specific version.
func (f *FilesystemBackend) GetInfo(_ context.Context, moduleName, version string) (*StoredModule, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	infoPath := filepath.Join(f.root, moduleName, version, "module.info")
	data, err := os.ReadFile(infoPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrVersionNotFound
		}
		return nil, fmt.Errorf("read module info: %w", err)
	}

	var stored StoredModule
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal module info: %w", err)
	}
	return &stored, nil
}

// GetManifest returns the module manifest (module.yaml).
func (f *FilesystemBackend) GetManifest(_ context.Context, moduleName, version string) (io.ReadCloser, int64, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	manifestPath := filepath.Join(f.root, moduleName, version, "module.yaml")
	file, err := os.Open(manifestPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, ErrVersionNotFound
		}
		return nil, 0, fmt.Errorf("open manifest: %w", err)
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, 0, fmt.Errorf("stat manifest: %w", err)
	}

	return file, info.Size(), nil
}

// GetZip returns the module ZIP file.
func (f *FilesystemBackend) GetZip(_ context.Context, moduleName, version string) (io.ReadCloser, int64, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	zipPath := filepath.Join(f.root, moduleName, version, "module.zip")
	info, err := os.Stat(zipPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, 0, ErrVersionNotFound
		}
		return nil, 0, fmt.Errorf("stat zip: %w", err)
	}

	file, err := os.Open(zipPath)
	if err != nil {
		return nil, 0, fmt.Errorf("open zip: %w", err)
	}

	return file, info.Size(), nil
}

// Publish stores a new module version.
func (f *FilesystemBackend) Publish(_ context.Context, req *PublishRequest) (*PublishResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	versionDir := filepath.Join(f.root, req.ModuleName, req.Version)

	if !req.Force {
		if _, err := os.Stat(versionDir); err == nil {
			return nil, ErrVersionExists
		}
	}

	//nolint:gosec // G301: registry directory needs to be accessible by service user
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		return nil, fmt.Errorf("create version directory: %w", err)
	}

	// Write zip file
	zipPath := filepath.Join(versionDir, "module.zip")
	zipFile, err := os.Create(zipPath)
	if err != nil {
		os.RemoveAll(versionDir)
		return nil, fmt.Errorf("create zip file: %w", err)
	}

	size, err := io.Copy(zipFile, req.ZipData)
	zipFile.Close()
	if err != nil {
		os.RemoveAll(versionDir)
		return nil, fmt.Errorf("write zip file: %w", err)
	}

	// Write manifest
	manifestPath := filepath.Join(versionDir, "module.yaml")
	//nolint:gosec // G306: module manifest needs to be readable by the registry
	if err := os.WriteFile(manifestPath, req.Manifest, 0o644); err != nil {
		os.RemoveAll(versionDir)
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	// Write signature if provided
	if req.Signature != "" {
		sigPath := filepath.Join(versionDir, "module.sig")
		//nolint:gosec // G306: signature files need to be readable for verification
		if err := os.WriteFile(sigPath, []byte(req.Signature), 0o644); err != nil {
			os.RemoveAll(versionDir)
			return nil, fmt.Errorf("write signature: %w", err)
		}
	}

	// Write module info (last, so partial writes don't appear in listings)
	stored := StoredModule{
		Name:         req.ModuleName,
		Version:      req.Version,
		Hash:         req.Hash,
		PublishedAt:  time.Now(),
		Size:         size,
		Description:  req.Description,
		Dependencies: req.Dependencies,
		Signature:    req.Signature,
		Tags:         req.Tags,
		ReleaseNotes: req.ReleaseNotes,
	}

	infoData, _ := json.MarshalIndent(stored, "", "  ")
	infoPath := filepath.Join(versionDir, "module.info")
	//nolint:gosec // G306: module info needs to be readable by the registry
	if err := os.WriteFile(infoPath, infoData, 0o644); err != nil {
		os.RemoveAll(versionDir)
		return nil, fmt.Errorf("write info: %w", err)
	}

	return &PublishResult{
		Hash: req.Hash,
		Size: size,
	}, nil
}

// Delete removes a module version and cleans up empty parent directories.
func (f *FilesystemBackend) Delete(_ context.Context, moduleName, version string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	versionDir := filepath.Join(f.root, moduleName, version)
	if _, err := os.Stat(versionDir); os.IsNotExist(err) {
		return ErrVersionNotFound
	}

	if err := os.RemoveAll(versionDir); err != nil {
		return fmt.Errorf("delete version: %w", err)
	}

	// Clean up empty module directory
	moduleDir := filepath.Join(f.root, moduleName)
	entries, _ := os.ReadDir(moduleDir)
	if len(entries) == 0 {
		os.RemoveAll(moduleDir)
	}

	return nil
}

// VersionExists checks if a specific module version exists.
func (f *FilesystemBackend) VersionExists(_ context.Context, moduleName, version string) (bool, error) {
	f.mu.RLock()
	defer f.mu.RUnlock()

	zipPath := filepath.Join(f.root, moduleName, version, "module.zip")
	_, err := os.Stat(zipPath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, fmt.Errorf("stat version: %w", err)
	}
	return true, nil
}

// Health checks that the root directory is accessible.
func (f *FilesystemBackend) Health(_ context.Context) error {
	_, err := os.Stat(f.root)
	if err != nil {
		return fmt.Errorf("filesystem health check: %w", err)
	}
	return nil
}

// Close is a no-op for the filesystem backend.
func (f *FilesystemBackend) Close() error {
	return nil
}
