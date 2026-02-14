// Package registry provides an offline module/blueprint registry for air-gapped environments.
// It wraps the existing FilesystemBackend with indexing, import/export, trust management,
// and a LocalClient that implements resolver.RegistryClient without HTTP.
package registry

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/shawnbutts/keystone-core/internal/registry/storage"
)

// Config configures an offline registry.
type Config struct {
	// RootDir is the root directory for registry data.
	// Modules are stored under RootDir/modules/ using FilesystemBackend layout.
	// Blueprints are stored under RootDir/blueprints/.
	// The index file lives at RootDir/index.json.
	RootDir string

	// AutoIndex rebuilds the registry index after mutations (import, delete, GC).
	AutoIndex bool
}

// Registry is a filesystem-based offline module/blueprint registry for air-gapped environments.
// It wraps FilesystemBackend for module storage and adds indexing, import/export, and trust management.
type Registry struct {
	config  Config
	backend *storage.FilesystemBackend
	index   *Index
}

// New creates a new offline registry. The root directory must already exist
// (call Init to create a fresh registry).
func New(cfg Config) (*Registry, error) {
	if cfg.RootDir == "" {
		return nil, fmt.Errorf("root directory is required")
	}

	info, err := os.Stat(cfg.RootDir)
	if err != nil {
		return nil, fmt.Errorf("root directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("root path is not a directory: %s", cfg.RootDir)
	}

	modulesDir := filepath.Join(cfg.RootDir, "modules")
	backend, err := storage.NewFilesystemBackend(modulesDir)
	if err != nil {
		return nil, fmt.Errorf("create backend: %w", err)
	}

	r := &Registry{
		config:  cfg,
		backend: backend,
	}

	// Load existing index if available
	indexPath := filepath.Join(cfg.RootDir, "index.json")
	if idx, err := LoadIndex(indexPath); err == nil {
		r.index = idx
	}

	return r, nil
}

// Init initializes a new empty offline registry at cfg.RootDir.
// Creates the directory structure and an empty index.
func Init(cfg Config) (*Registry, error) {
	if cfg.RootDir == "" {
		return nil, fmt.Errorf("root directory is required")
	}

	dirs := []string{
		cfg.RootDir,
		filepath.Join(cfg.RootDir, "modules"),
		filepath.Join(cfg.RootDir, "blueprints"),
	}
	for _, d := range dirs {
		//nolint:gosec // G301: registry directory needs to be accessible by service user
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	idx := &Index{
		SchemaVersion: indexSchemaVersion,
	}
	if err := idx.Save(filepath.Join(cfg.RootDir, "index.json")); err != nil {
		return nil, fmt.Errorf("write initial index: %w", err)
	}

	return New(cfg)
}

// Backend returns the underlying FilesystemBackend for module storage.
func (r *Registry) Backend() *storage.FilesystemBackend {
	return r.backend
}

// Index returns the current registry index, or nil if not loaded.
func (r *Registry) Index() *Index {
	return r.index
}

// ModulesDir returns the path to the modules directory.
func (r *Registry) ModulesDir() string {
	return filepath.Join(r.config.RootDir, "modules")
}

// BlueprintsDir returns the path to the blueprints directory.
func (r *Registry) BlueprintsDir() string {
	return filepath.Join(r.config.RootDir, "blueprints")
}

// Reindex regenerates the registry index from the filesystem.
func (r *Registry) Reindex() error {
	idx, err := Generate(r.config.RootDir)
	if err != nil {
		return fmt.Errorf("generate index: %w", err)
	}
	r.index = idx
	return idx.Save(filepath.Join(r.config.RootDir, "index.json"))
}

// Close releases resources held by the registry.
func (r *Registry) Close() error {
	return r.backend.Close()
}
