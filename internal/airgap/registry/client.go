package registry

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
)

// Ensure LocalClient implements resolver.RegistryClient at compile time.
var _ resolver.RegistryClient = (*LocalClient)(nil)

// LocalClient implements resolver.RegistryClient by reading directly from a
// FilesystemBackend, bypassing HTTP entirely. This is the primary way air-gapped
// environments resolve module dependencies.
type LocalClient struct {
	registry *Registry
}

// NewLocalClient creates a RegistryClient backed by a local offline registry.
func NewLocalClient(registry *Registry) *LocalClient {
	return &LocalClient{registry: registry}
}

// ListVersions returns all available versions for a module.
func (c *LocalClient) ListVersions(moduleName string) ([]string, error) {
	return c.registry.backend.ListVersions(context.Background(), moduleName)
}

// GetModuleInfo returns metadata for a specific module version.
func (c *LocalClient) GetModuleInfo(moduleName, version string) (*resolver.ModuleInfo, error) {
	stored, err := c.registry.backend.GetInfo(context.Background(), moduleName, version)
	if err != nil {
		return nil, err
	}
	return &resolver.ModuleInfo{
		Name:         stored.Name,
		Version:      stored.Version,
		Hash:         stored.Hash,
		PublishedAt:  stored.PublishedAt,
		Description:  stored.Description,
		Dependencies: stored.Dependencies,
		Size:         stored.Size,
	}, nil
}

// GetModuleManifest returns the parsed manifest for a specific module version.
func (c *LocalClient) GetModuleManifest(moduleName, version string) (*manifest.Manifest, error) {
	rc, _, err := c.registry.backend.GetManifest(context.Background(), moduleName, version)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	return manifest.Parse(data)
}

// DownloadModule downloads a module zip to the specified destination path.
func (c *LocalClient) DownloadModule(moduleName, version, destPath string) error {
	rc, _, err := c.registry.backend.GetZip(context.Background(), moduleName, version)
	if err != nil {
		return err
	}
	defer rc.Close()

	//nolint:gosec // G304: destPath is caller-provided output location
	out, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("create destination: %w", err)
	}
	defer out.Close()

	if _, err := io.Copy(out, rc); err != nil {
		return fmt.Errorf("write module: %w", err)
	}

	return nil
}
