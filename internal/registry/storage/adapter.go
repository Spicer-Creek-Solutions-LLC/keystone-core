package storage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/files/backend"
)

// BackendAdapter wraps a files/backend.Backend to implement the registry storage.Backend interface.
// Path mapping: {moduleName}/{version}/module.zip, module.yaml, module.sig, module.info
type BackendAdapter struct {
	inner backend.Backend
}

// NewBackendAdapter creates a new adapter around a files backend.
func NewBackendAdapter(inner backend.Backend) *BackendAdapter {
	return &BackendAdapter{inner: inner}
}

// ListVersions lists version directories for a module by listing objects with the module prefix.
func (a *BackendAdapter) ListVersions(ctx context.Context, moduleName string) ([]string, error) {
	result, err := a.inner.List(ctx, moduleName, &backend.ListOptions{Recursive: true})
	if err != nil {
		if backend.IsNotFound(err) {
			return nil, ErrModuleNotFound
		}
		return nil, fmt.Errorf("list versions: %w", err)
	}

	// Extract unique version directories that contain module.zip
	versionSet := make(map[string]bool)
	prefix := moduleName + "/"
	for i := range result.Files {
		// Path is like /moduleName/version/module.zip
		relPath := strings.TrimPrefix(result.Files[i].Path, "/")
		if !strings.HasPrefix(relPath, prefix) {
			continue
		}
		rest := strings.TrimPrefix(relPath, prefix)
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) == 2 && parts[1] == "module.zip" {
			versionSet[parts[0]] = true
		}
	}

	if len(versionSet) == 0 {
		return nil, ErrModuleNotFound
	}

	versions := make([]string, 0, len(versionSet))
	for v := range versionSet {
		versions = append(versions, v)
	}
	return versions, nil
}

// GetInfo retrieves and unmarshals the module.info JSON file.
func (a *BackendAdapter) GetInfo(ctx context.Context, moduleName, version string) (*StoredModule, error) {
	key := path.Join(moduleName, version, "module.info")
	result, err := a.inner.Get(ctx, key, nil)
	if err != nil {
		if backend.IsNotFound(err) {
			return nil, ErrVersionNotFound
		}
		return nil, fmt.Errorf("get info: %w", err)
	}
	defer result.Reader.Close()

	data, err := io.ReadAll(result.Reader)
	if err != nil {
		return nil, fmt.Errorf("read info: %w", err)
	}

	var stored StoredModule
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("unmarshal info: %w", err)
	}
	return &stored, nil
}

// GetManifest returns the module.yaml file.
func (a *BackendAdapter) GetManifest(ctx context.Context, moduleName, version string) (io.ReadCloser, int64, error) {
	key := path.Join(moduleName, version, "module.yaml")
	result, err := a.inner.Get(ctx, key, nil)
	if err != nil {
		if backend.IsNotFound(err) {
			return nil, 0, ErrVersionNotFound
		}
		return nil, 0, fmt.Errorf("get manifest: %w", err)
	}
	var size int64
	if result.Info != nil {
		size = result.Info.Size
	}
	return result.Reader, size, nil
}

// GetZip returns the module.zip file.
func (a *BackendAdapter) GetZip(ctx context.Context, moduleName, version string) (io.ReadCloser, int64, error) {
	key := path.Join(moduleName, version, "module.zip")
	result, err := a.inner.Get(ctx, key, nil)
	if err != nil {
		if backend.IsNotFound(err) {
			return nil, 0, ErrVersionNotFound
		}
		return nil, 0, fmt.Errorf("get zip: %w", err)
	}
	var size int64
	if result.Info != nil {
		size = result.Info.Size
	}
	return result.Reader, size, nil
}

// Publish stores all module files. module.info is written last so partially-written
// versions don't appear in listings.
func (a *BackendAdapter) Publish(ctx context.Context, req *PublishRequest) (*PublishResult, error) {
	base := path.Join(req.ModuleName, req.Version)

	// Check for existing version (unless force)
	if !req.Force {
		exists, err := a.inner.Exists(ctx, path.Join(base, "module.zip"))
		if err != nil && !backend.IsNotFound(err) {
			return nil, fmt.Errorf("check existing: %w", err)
		}
		if exists {
			return nil, ErrVersionExists
		}
	}

	// Write zip
	zipKey := path.Join(base, "module.zip")
	zipResult, err := a.inner.Put(ctx, zipKey, req.ZipData, &backend.PutOptions{
		ContentType: "application/zip",
		Overwrite:   req.Force,
	})
	if err != nil {
		return nil, fmt.Errorf("put zip: %w", err)
	}

	// Write manifest
	manifestKey := path.Join(base, "module.yaml")
	if _, err := a.inner.Put(ctx, manifestKey, bytes.NewReader(req.Manifest), &backend.PutOptions{
		ContentType: "application/x-yaml",
		Overwrite:   true,
	}); err != nil {
		a.inner.Delete(ctx, zipKey) //nolint:errcheck // best-effort cleanup
		return nil, fmt.Errorf("put manifest: %w", err)
	}

	// Write signature if provided
	if req.Signature != "" {
		sigKey := path.Join(base, "module.sig")
		if _, err := a.inner.Put(ctx, sigKey, bytes.NewReader([]byte(req.Signature)), &backend.PutOptions{
			ContentType: "text/plain",
			Overwrite:   true,
		}); err != nil {
			a.inner.Delete(ctx, zipKey)       //nolint:errcheck // best-effort cleanup
			a.inner.Delete(ctx, manifestKey)   //nolint:errcheck // best-effort cleanup
			return nil, fmt.Errorf("put signature: %w", err)
		}
	}

	// Write info last
	stored := StoredModule{
		Name:         req.ModuleName,
		Version:      req.Version,
		Hash:         req.Hash,
		Size:         zipResult.Size,
		Description:  req.Description,
		Dependencies: req.Dependencies,
		Signature:    req.Signature,
		Tags:         req.Tags,
		ReleaseNotes: req.ReleaseNotes,
	}
	infoData, _ := json.MarshalIndent(stored, "", "  ")
	infoKey := path.Join(base, "module.info")
	if _, err := a.inner.Put(ctx, infoKey, bytes.NewReader(infoData), &backend.PutOptions{
		ContentType: "application/json",
		Overwrite:   true,
	}); err != nil {
		a.inner.Delete(ctx, zipKey)       //nolint:errcheck // best-effort cleanup
		a.inner.Delete(ctx, manifestKey)   //nolint:errcheck // best-effort cleanup
		return nil, fmt.Errorf("put info: %w", err)
	}

	return &PublishResult{
		Hash: req.Hash,
		Size: zipResult.Size,
	}, nil
}

// Delete removes all files for a module version.
func (a *BackendAdapter) Delete(ctx context.Context, moduleName, version string) error {
	base := path.Join(moduleName, version)

	// Check existence first
	exists, err := a.inner.Exists(ctx, path.Join(base, "module.zip"))
	if err != nil && !backend.IsNotFound(err) {
		return fmt.Errorf("check existing: %w", err)
	}
	if !exists {
		return ErrVersionNotFound
	}

	for _, file := range []string{"module.info", "module.sig", "module.yaml", "module.zip"} {
		if err := a.inner.Delete(ctx, path.Join(base, file)); err != nil && !backend.IsNotFound(err) {
			return fmt.Errorf("delete %s: %w", file, err)
		}
	}
	return nil
}

// VersionExists checks if a module version exists by probing module.zip.
func (a *BackendAdapter) VersionExists(ctx context.Context, moduleName, version string) (bool, error) {
	exists, err := a.inner.Exists(ctx, path.Join(moduleName, version, "module.zip"))
	if err != nil {
		if backend.IsNotFound(err) {
			return false, nil
		}
		return false, fmt.Errorf("version exists: %w", err)
	}
	return exists, nil
}

// Health delegates to the inner backend.
func (a *BackendAdapter) Health(ctx context.Context) error {
	return a.inner.Health(ctx)
}

// Close delegates to the inner backend.
func (a *BackendAdapter) Close() error {
	return a.inner.Close()
}

// Verify interface compliance.
var _ Backend = (*BackendAdapter)(nil)
