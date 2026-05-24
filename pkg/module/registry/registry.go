// SPDX-License-Identifier: Apache-2.0

// Package registry is the v1.0 filesystem-backed module registry
// (Epic 14 task 8): publish + the Go module-proxy HTTP endpoints,
// and an in-process implementation of resolver.Source so the
// resolver resolves directly against a registry.
//
// The standalone cmd/kscore-registry server binary is task 9; this
// package is the library + HTTP handler. The ZIP is opaque here —
// the loader (task 10) unzips.
//
// Composes pkg/module/{manifest,cas,resolver}, pkg/semver, and
// internal/registry/storage; no new dependency.
package registry

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"time"

	"go.keystone-core.io/keystone-core/internal/registry/storage"
	"go.keystone-core.io/keystone-core/pkg/module/cas"
	"go.keystone-core.io/keystone-core/pkg/module/manifest"
	"go.keystone-core.io/keystone-core/pkg/module/resolver"
	"go.keystone-core.io/keystone-core/pkg/semver"
)

var (
	// ErrInvalidModule — the published manifest is malformed or its
	// name/version disagree with the artifact.
	ErrInvalidModule = errors.New("registry: invalid module")
	// ErrVersionExists — that module@version is already published
	// (versions are immutable).
	ErrVersionExists = errors.New("registry: version already exists")
	// ErrNotFound — no such module / version.
	ErrNotFound = errors.New("registry: not found")
)

// Registry stores published modules in a storage backend and serves
// them. It implements resolver.Source.
type Registry struct {
	st storage.Storage
}

// New returns a Registry over st.
func New(st storage.Storage) *Registry { return &Registry{st: st} }

func manifestKey(mod, ver string) string { return path.Join(mod, ver, "manifest.yaml") }
func zipKey(mod, ver string) string      { return path.Join(mod, ver, "module.zip") }
func infoKey(mod, ver string) string     { return path.Join(mod, ver, "info.json") }
func sigKey(mod, ver string) string      { return path.Join(mod, ver, "module.sig") }

// versionInfo is the stored per-version metadata. The HTTP .info
// endpoint exposes only {Version, Time}; Hash is internal (the
// resolver.Source path reads it).
type versionInfo struct {
	Version string    `json:"version"`
	Time    time.Time `json:"time"`
	Hash    string    `json:"hash"`
}

// Publish stores an unsigned module (manifest + ZIP + metadata).
func (r *Registry) Publish(ctx context.Context, manifestYAML, zip []byte) error {
	return r.PublishSigned(ctx, manifestYAML, zip, nil)
}

// PublishSigned is Publish with an optional detached signature
// artifact (verify.MarshalSignature bytes). Re-publishing an
// existing version is rejected (immutability).
func (r *Registry) PublishSigned(ctx context.Context, manifestYAML, zip, sig []byte) error {
	m, err := manifest.UnmarshalManifest(manifestYAML)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidModule, err)
	}
	if err := m.Validate(); err != nil {
		// Validate enforces the namespaced name (squatting guard).
		return fmt.Errorf("%w: %v", ErrInvalidModule, err)
	}
	if len(zip) == 0 {
		return fmt.Errorf("%w: empty module zip", ErrInvalidModule)
	}
	mod, ver := m.Name, m.Version

	exists, err := r.st.Exists(ctx, infoKey(mod, ver))
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("%w: %s@%s", ErrVersionExists, mod, ver)
	}

	info := versionInfo{Version: ver, Time: time.Now().UTC(), Hash: cas.HashBytes(zip)}
	infoJSON, err := json.Marshal(info)
	if err != nil {
		return err
	}
	if err := r.st.Put(ctx, manifestKey(mod, ver), bytes.NewReader(manifestYAML)); err != nil {
		return err
	}
	if err := r.st.Put(ctx, zipKey(mod, ver), bytes.NewReader(zip)); err != nil {
		return err
	}
	if len(sig) > 0 {
		if err := r.st.Put(ctx, sigKey(mod, ver), bytes.NewReader(sig)); err != nil {
			return err
		}
	}
	return r.st.Put(ctx, infoKey(mod, ver), bytes.NewReader(infoJSON))
}

// SignatureBytes returns the stored detached signature artifact for
// a module version, or ErrNotFound if it was published unsigned.
func (r *Registry) SignatureBytes(ctx context.Context, mod, ver string) ([]byte, error) {
	if !manifest.ValidModuleName(mod) {
		return nil, fmt.Errorf("%w: bad module name %q", ErrNotFound, mod)
	}
	return r.getBytes(ctx, sigKey(mod, ver))
}

// versions returns the published version strings for mod (sorted
// semver ascending), or ErrNotFound if the module is unknown.
func (r *Registry) versions(ctx context.Context, mod string) ([]semver.Version, error) {
	if !manifest.ValidModuleName(mod) {
		return nil, fmt.Errorf("%w: bad module name %q", ErrNotFound, mod)
	}
	keys, err := r.st.List(ctx, mod)
	if err != nil {
		return nil, err
	}
	var vs []semver.Version
	suffix := "/info.json"
	prefix := mod + "/"
	for _, k := range keys {
		if !hasPrefixSuffix(k, prefix, suffix) {
			continue
		}
		verStr := k[len(prefix) : len(k)-len(suffix)]
		v, perr := semver.Parse(verStr)
		if perr != nil {
			continue // skip unparseable dirs defensively
		}
		vs = append(vs, v)
	}
	if len(vs) == 0 {
		return nil, fmt.Errorf("%w: module %q", ErrNotFound, mod)
	}
	semver.Sort(vs)
	return vs, nil
}

func (r *Registry) readInfo(ctx context.Context, mod, ver string) (versionInfo, error) {
	b, err := r.getBytes(ctx, infoKey(mod, ver))
	if err != nil {
		return versionInfo{}, err
	}
	var vi versionInfo
	if err := json.Unmarshal(b, &vi); err != nil {
		return versionInfo{}, fmt.Errorf("registry: corrupt info for %s@%s: %w", mod, ver, err)
	}
	return vi, nil
}

func (r *Registry) getBytes(ctx context.Context, key string) ([]byte, error) {
	rc, err := r.st.Get(ctx, key)
	if err != nil {
		if errors.Is(err, storage.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrNotFound, key)
		}
		return nil, err
	}
	defer func() { _ = rc.Close() }()
	return io.ReadAll(rc)
}

// --- resolver.Source ------------------------------------------------------

// ListVersions implements resolver.Source.
func (r *Registry) ListVersions(ctx context.Context, module string) ([]resolver.ModuleVersion, error) {
	vs, err := r.versions(ctx, module)
	if err != nil {
		return nil, err
	}
	out := make([]resolver.ModuleVersion, 0, len(vs))
	for _, v := range vs {
		vi, ierr := r.readInfo(ctx, module, v.String())
		if ierr != nil {
			return nil, ierr
		}
		out = append(out, resolver.ModuleVersion{Version: v, Hash: vi.Hash})
	}
	return out, nil
}

// GetManifest implements resolver.Source.
func (r *Registry) GetManifest(ctx context.Context, module string, v semver.Version) (*manifest.Manifest, error) {
	if !manifest.ValidModuleName(module) {
		return nil, fmt.Errorf("%w: bad module name %q", ErrNotFound, module)
	}
	b, err := r.getBytes(ctx, manifestKey(module, v.String()))
	if err != nil {
		return nil, err
	}
	return manifest.UnmarshalManifest(b)
}

func hasPrefixSuffix(s, prefix, suffix string) bool {
	return len(s) >= len(prefix)+len(suffix) &&
		s[:len(prefix)] == prefix && s[len(s)-len(suffix):] == suffix
}
