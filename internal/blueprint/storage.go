package blueprint

import (
	"context"
	"errors"
	"io"
)

// Common storage errors.
var (
	// ErrBlueprintNotFound is returned when a blueprint is not found.
	ErrBlueprintNotFound = errors.New("blueprint not found")

	// ErrVersionNotFound is returned when a specific version is not found.
	ErrVersionNotFound = errors.New("blueprint version not found")

	// ErrInvalidReference is returned when a blueprint reference is invalid.
	ErrInvalidReference = errors.New("invalid blueprint reference")

	// ErrStorageReadOnly is returned when trying to write to a read-only storage.
	ErrStorageReadOnly = errors.New("storage is read-only")
)

// Storage defines the interface for blueprint storage backends.
type Storage interface {
	// Get retrieves a blueprint by name and optional version.
	// If version is empty, returns the latest version.
	Get(ctx context.Context, name string, version string) (*Blueprint, error)

	// List returns all blueprints matching the optional filter.
	List(ctx context.Context, filter *ListFilter) ([]*Info, error)

	// Versions returns all available versions for a blueprint.
	Versions(ctx context.Context, name string) ([]string, error)

	// Exists checks if a blueprint exists.
	Exists(ctx context.Context, name string, version string) (bool, error)

	// Close releases any resources held by the storage.
	Close() error
}

// WritableStorage extends Storage with write operations.
type WritableStorage interface {
	Storage

	// Put stores a blueprint.
	Put(ctx context.Context, bp *Blueprint) error

	// Delete removes a blueprint version.
	Delete(ctx context.Context, name string, version string) error
}

// FileStorage extends Storage with file operations.
type FileStorage interface {
	Storage

	// GetFile retrieves a file from a blueprint.
	GetFile(ctx context.Context, name, version, path string) (io.ReadCloser, error)

	// ListFiles lists files in a blueprint directory.
	ListFiles(ctx context.Context, name, version, path string) ([]FileInfo, error)
}

// ListFilter defines filters for listing blueprints.
type ListFilter struct {
	// Vendor filters by vendor name (e.g., "community")
	Vendor string

	// Keywords filters by keywords
	Keywords []string

	// Categories filters by categories
	Categories []string

	// Limit limits the number of results
	Limit int

	// Offset for pagination
	Offset int
}

// Info contains summary information about a blueprint.
type Info struct {
	// Name is the full blueprint name (e.g., "blueprints/community/web-app-stack")
	Name string

	// Version is the latest version
	Version string

	// Description is a short description
	Description string

	// Keywords for search
	Keywords []string

	// Categories for classification
	Categories []string

	// Maintainers list
	Maintainers []Maintainer

	// License identifier
	License string

	// AvailableVersions lists all versions
	AvailableVersions []string
}

// FileInfo contains information about a file in a blueprint.
type FileInfo struct {
	// Name is the file name
	Name string

	// Path is the relative path within the blueprint
	Path string

	// Size is the file size in bytes
	Size int64

	// IsDir is true if this is a directory
	IsDir bool
}

// StorageConfig contains configuration for creating a storage backend.
type StorageConfig struct {
	// Type is the storage type (e.g., "local", "oci", "http")
	Type string

	// Path is the local filesystem path (for local storage)
	Path string

	// URL is the registry URL (for remote storage)
	URL string

	// ReadOnly makes the storage read-only
	ReadOnly bool

	// CacheDir is the local cache directory for remote storage
	CacheDir string

	// Auth contains authentication configuration
	Auth *StorageAuth
}

// StorageAuth contains authentication configuration.
type StorageAuth struct {
	// Token for bearer token authentication
	Token string

	// Username for basic authentication
	Username string

	// Password for basic authentication
	Password string

	// CertFile for mTLS
	CertFile string

	// KeyFile for mTLS
	KeyFile string

	// CAFile for CA verification
	CAFile string
}

// StorageFactory creates storage backends from configuration.
type StorageFactory struct {
	// factories maps storage types to factory functions
	factories map[string]func(*StorageConfig) (Storage, error)
}

// NewStorageFactory creates a new storage factory with default backends.
func NewStorageFactory() *StorageFactory {
	f := &StorageFactory{
		factories: make(map[string]func(*StorageConfig) (Storage, error)),
	}

	// Register default backends
	f.Register("local", func(cfg *StorageConfig) (Storage, error) {
		return NewLocalStorage(cfg.Path, cfg.ReadOnly)
	})

	return f
}

// Register registers a storage backend factory.
func (f *StorageFactory) Register(storageType string, factory func(*StorageConfig) (Storage, error)) {
	f.factories[storageType] = factory
}

// Create creates a storage backend from configuration.
func (f *StorageFactory) Create(cfg *StorageConfig) (Storage, error) {
	factory, ok := f.factories[cfg.Type]
	if !ok {
		return nil, errors.New("unknown storage type: " + cfg.Type)
	}
	return factory(cfg)
}

// MultiStorage combines multiple storage backends with priority.
type MultiStorage struct {
	// storages is ordered by priority (first is highest priority)
	storages []Storage
}

// NewMultiStorage creates a new multi-storage backend.
func NewMultiStorage(storages ...Storage) *MultiStorage {
	return &MultiStorage{storages: storages}
}

// Get retrieves a blueprint, trying each storage in order.
func (m *MultiStorage) Get(ctx context.Context, name, version string) (*Blueprint, error) {
	var lastErr error
	for _, s := range m.storages {
		bp, err := s.Get(ctx, name, version)
		if err == nil {
			return bp, nil
		}
		if !errors.Is(err, ErrBlueprintNotFound) && !errors.Is(err, ErrVersionNotFound) {
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	return nil, ErrBlueprintNotFound
}

// List combines results from all storages.
func (m *MultiStorage) List(ctx context.Context, filter *ListFilter) ([]*Info, error) {
	seen := make(map[string]bool)
	var result []*Info

	for _, s := range m.storages {
		infos, err := s.List(ctx, filter)
		if err != nil {
			continue // Skip failing storages
		}
		for _, info := range infos {
			if !seen[info.Name] {
				seen[info.Name] = true
				result = append(result, info)
			}
		}
	}

	return result, nil
}

// Versions returns versions from all storages.
func (m *MultiStorage) Versions(ctx context.Context, name string) ([]string, error) {
	seen := make(map[string]bool)
	var result []string

	for _, s := range m.storages {
		versions, err := s.Versions(ctx, name)
		if err != nil {
			continue
		}
		for _, v := range versions {
			if !seen[v] {
				seen[v] = true
				result = append(result, v)
			}
		}
	}

	if len(result) == 0 {
		return nil, ErrBlueprintNotFound
	}

	return result, nil
}

// Exists checks if a blueprint exists in any storage.
func (m *MultiStorage) Exists(ctx context.Context, name, version string) (bool, error) {
	for _, s := range m.storages {
		exists, err := s.Exists(ctx, name, version)
		if err != nil {
			continue
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

// Close closes all storages.
func (m *MultiStorage) Close() error {
	var lastErr error
	for _, s := range m.storages {
		if err := s.Close(); err != nil {
			lastErr = err
		}
	}
	return lastErr
}

// AddStorage adds a storage backend.
func (m *MultiStorage) AddStorage(s Storage) {
	m.storages = append(m.storages, s)
}
