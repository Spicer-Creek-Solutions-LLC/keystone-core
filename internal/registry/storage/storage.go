// Package storage provides pluggable storage backends for the module registry.
package storage

import (
	"context"
	"io"
	"time"
)

// Backend is the interface that registry storage backends must implement.
type Backend interface {
	// ListVersions returns all available versions for a module.
	ListVersions(ctx context.Context, moduleName string) ([]string, error)

	// GetInfo returns metadata for a specific module version.
	GetInfo(ctx context.Context, moduleName, version string) (*StoredModule, error)

	// GetManifest returns the manifest (module.yaml) for a module version.
	// The caller must close the returned reader.
	GetManifest(ctx context.Context, moduleName, version string) (io.ReadCloser, int64, error)

	// GetZip returns the module ZIP file for a module version.
	// The caller must close the returned reader.
	GetZip(ctx context.Context, moduleName, version string) (io.ReadCloser, int64, error)

	// Publish stores a new module version.
	Publish(ctx context.Context, req *PublishRequest) (*PublishResult, error)

	// Delete removes a module version.
	Delete(ctx context.Context, moduleName, version string) error

	// VersionExists checks if a specific module version exists.
	VersionExists(ctx context.Context, moduleName, version string) (bool, error)

	// Health checks if the storage backend is healthy.
	Health(ctx context.Context) error

	// Close releases any resources held by the backend.
	Close() error
}

// StoredModule represents module metadata persisted by the registry.
type StoredModule struct {
	Name         string            `json:"name"`
	Version      string            `json:"version"`
	Hash         string            `json:"hash"`
	PublishedAt  time.Time         `json:"published_at"`
	Size         int64             `json:"size"`
	Description  string            `json:"description,omitempty"`
	Dependencies map[string]string `json:"dependencies,omitempty"`
	Signature    string            `json:"signature,omitempty"`
	Tags         []string          `json:"tags,omitempty"`
	ReleaseNotes string            `json:"release_notes,omitempty"`
}

// PublishRequest bundles all data needed to publish a module version.
type PublishRequest struct {
	ModuleName   string
	Version      string
	ZipData      io.Reader
	ZipSize      int64
	Manifest     []byte
	Signature    string
	Hash         string
	Force        bool
	Description  string
	Dependencies map[string]string
	Tags         []string
	ReleaseNotes string
}

// PublishResult contains the outcome of a publish operation.
type PublishResult struct {
	Hash string
	Size int64
}
