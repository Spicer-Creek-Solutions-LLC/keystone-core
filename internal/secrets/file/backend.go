// SPDX-License-Identifier: Apache-2.0

package file

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/masterkey"
	"go.keystone-core.io/keystone-core/internal/secrets"
)

// DefaultBackendName is the [Backend.Name] when [Config.Name] is empty.
const DefaultBackendName = "file"

// Config drives [NewBackend]. Path is the canonical state-file
// location (the operator's `secrets.backends[].file.path`).
// MasterKeySource is the scheme-prefixed config value resolved by
// [masterkey.Resolve].
//
// Name overrides the backend's `Name()` so a multi-file-backend
// operator deployment (two encrypted files for two namespaces) has
// unambiguous names — defaults to [DefaultBackendName].
//
// Clock + Logger inject testable now-time + a logger. Both default
// to standard values when nil.
//
// EnsureParentDir, when true, creates the file's parent directory at
// Start (`0700` permissions). v1.0 default is false — operators are
// responsible for the directory in production deployments. Tests
// enable it for ergonomics.
type Config struct {
	Path            string
	MasterKeySource string
	Name            string
	Clock           func() time.Time
	Logger          *slog.Logger
	EnsureParentDir bool
}

// Backend is the encrypted-file [secrets.SecretBackend] — AES-256-GCM
// at rest, atomic-rename writes, in-memory cleartext while running.
// Implements [secrets.CapKV] + [secrets.CapList]; explicitly does NOT
// implement dynamic / transit / lease ops (those methods return
// [secrets.ErrInvalidBackend] for defense-in-depth against direct
// callers that bypass the broker's capability check).
//
// Concurrency model: a single `sync.RWMutex` guards the in-memory
// state map. Reads acquire R; writes acquire W and serialise the
// disk write under the same lock so post-mutation state stays
// consistent. Realistic deployments don't bottleneck on this.
type Backend struct {
	cfg    Config
	key    masterkey.Key
	name   string
	logger *slog.Logger
	clock  func() time.Time

	mu      sync.RWMutex
	state   *fileState
	started bool
	stopped bool
}

// NewBackend validates the config and resolves the master key. The
// backend is NOT live until [Backend.Start] runs.
func NewBackend(cfg Config) (*Backend, error) {
	if cfg.Path == "" {
		return nil, fmt.Errorf("%w: file backend: Path is required", secrets.ErrInvalidBackend)
	}
	if cfg.MasterKeySource == "" {
		return nil, fmt.Errorf("%w: file backend: MasterKeySource is required", secrets.ErrInvalidBackend)
	}
	key, err := masterkey.Resolve(cfg.MasterKeySource)
	if err != nil {
		// Re-wrap so the file backend's public contract still exposes
		// ErrInvalidBackend (the masterkey.ErrInvalidKey cause is kept
		// in the chain).
		return nil, fmt.Errorf("%w: %w", secrets.ErrInvalidBackend, err)
	}
	name := cfg.Name
	if name == "" {
		name = DefaultBackendName
	}
	logger := cfg.Logger
	if logger == nil {
		logger = slog.Default()
	}
	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}

	return &Backend{
		cfg:    cfg,
		key:    key,
		name:   name,
		logger: logger,
		clock:  clock,
	}, nil
}

// Name returns the backend's operator-facing name.
func (b *Backend) Name() string { return b.name }

// Capabilities reports [secrets.CapKV] + [secrets.CapList]. The file
// backend deliberately does NOT advertise dynamic / transit / lease
// caps; the broker (task 3) refuses routes for unsupported ops
// before dispatch.
func (b *Backend) Capabilities() []secrets.BackendCapability {
	return []secrets.BackendCapability{secrets.CapKV, secrets.CapList}
}

// Start brings the backend up. On a fresh path, an empty encrypted
// file is written so the master key is round-tripped at boot. On an
// existing path, the file is decoded and the in-memory state
// populated. Any leftover `.tmp` from a crashed previous write is
// cleaned up first.
func (b *Backend) Start(ctx context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.stopped {
		return fmt.Errorf("%w: file backend: cannot Start after Stop", secrets.ErrInvalidBackend)
	}
	if b.started {
		return fmt.Errorf("%w: file backend: already started", secrets.ErrInvalidBackend)
	}

	if b.cfg.EnsureParentDir {
		if err := ensureParentDir(b.cfg.Path); err != nil {
			return err
		}
	}

	staleFound, err := cleanupStaleTemp(b.cfg.Path)
	if err != nil {
		return err
	}
	if staleFound {
		b.logger.LogAttrs(ctx, slog.LevelInfo, "file backend: cleaned up leftover .tmp from previous crashed write",
			slog.String("path", b.cfg.Path),
			slog.String("backend", b.name),
		)
	}

	raw, err := os.ReadFile(b.cfg.Path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			b.logger.LogAttrs(ctx, slog.LevelInfo, "file backend: initializing empty store",
				slog.String("path", b.cfg.Path),
				slog.String("backend", b.name),
				slog.String("key_fp", b.key.Fingerprint()),
			)
			b.state = newFileState()
			if err := b.persistLocked(); err != nil {
				return err
			}
			b.started = true
			return nil
		}
		return fmt.Errorf("%w: file backend: read %q: %v", secrets.ErrInvalidBackend, b.cfg.Path, err)
	}

	plain, err := decode(raw, b.key)
	if err != nil {
		return err
	}
	var state fileState
	if err := json.Unmarshal(plain, &state); err != nil {
		return fmt.Errorf("%w: file backend: parse state %q: %v", secrets.ErrInvalidBackend, b.cfg.Path, err)
	}
	if state.Version != stateSchemaVersion {
		return fmt.Errorf("%w: file backend: state schema version %d, want %d", secrets.ErrInvalidBackend, state.Version, stateSchemaVersion)
	}
	if state.Secrets == nil {
		state.Secrets = make(map[string]*storedSecret)
	}
	b.state = &state
	b.logger.LogAttrs(ctx, slog.LevelInfo, "file backend: started",
		slog.String("path", b.cfg.Path),
		slog.String("backend", b.name),
		slog.String("key_fp", b.key.Fingerprint()),
		slog.Int("secrets", len(state.Secrets)),
	)
	b.started = true
	return nil
}

// Stop clears the in-memory state and marks the backend stopped.
// Idempotent.
func (b *Backend) Stop(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.stopped {
		return nil
	}
	b.state = nil
	b.stopped = true
	return nil
}

// Health returns nil when the backend is started.
func (b *Backend) Health(_ context.Context) error {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if !b.started || b.stopped {
		return secrets.ErrBackendNotStarted
	}
	return nil
}

// GetSecret reads the secret at req.Path. Returns
// [secrets.ErrSecretNotFound] on miss. The file backend has no
// per-path history — `req.Version`, if non-zero, must match the
// current version or the call returns `ErrSecretNotFound`.
func (b *Backend) GetSecret(_ context.Context, req secrets.GetSecretRequest) (*secrets.Secret, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if err := b.ensureStartedLocked(); err != nil {
		return nil, err
	}
	rec, ok := b.state.Secrets[req.Path]
	if !ok {
		return nil, fmt.Errorf("%w: %q", secrets.ErrSecretNotFound, req.Path)
	}
	if req.Version != 0 && req.Version != rec.Version {
		return nil, fmt.Errorf("%w: %q version %d (current %d)", secrets.ErrSecretNotFound, req.Path, req.Version, rec.Version)
	}
	return rec.toSecret(req.Path), nil
}

// WriteSecret writes a static KV secret. If `req.CAS` is non-nil, the
// supplied version must match the current version or
// [secrets.ErrInvalidBackend] is returned. On success the entire
// state is re-encrypted and atomically rewritten; an in-memory
// rollback restores the prior state if the disk write fails.
func (b *Backend) WriteSecret(_ context.Context, req secrets.WriteSecretRequest) (*secrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ensureStartedLocked(); err != nil {
		return nil, err
	}

	now := b.clock()
	existing, exists := b.state.Secrets[req.Path]

	var currentVersion uint64
	if exists {
		currentVersion = existing.Version
	}
	if req.CAS != nil && *req.CAS != currentVersion {
		return nil, fmt.Errorf("%w: CAS mismatch for %q (expected version %d, current %d)", secrets.ErrInvalidBackend, req.Path, *req.CAS, currentVersion)
	}

	next := &storedSecret{
		Data:      cloneAnyMap(req.Data),
		Metadata:  cloneStringMap(req.Metadata),
		Version:   currentVersion + 1,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if exists {
		next.CreatedAt = existing.CreatedAt
	}

	// Stage in a backup so a failed persist rolls back cleanly.
	var prior *storedSecret
	if exists {
		copy := *existing
		prior = &copy
	}
	b.state.Secrets[req.Path] = next

	if err := b.persistLocked(); err != nil {
		// Roll back the in-memory mutation.
		if prior == nil {
			delete(b.state.Secrets, req.Path)
		} else {
			b.state.Secrets[req.Path] = prior
		}
		return nil, err
	}
	return next.toSecret(req.Path), nil
}

// ListSecrets enumerates paths under req.Prefix. Metadata-only by
// contract — `Data` is never on the response. Pagination uses
// `Cursor` as the last path returned by the previous page;
// `req.Limit <= 0` means no limit.
func (b *Backend) ListSecrets(_ context.Context, req secrets.ListSecretsRequest) (*secrets.ListSecretsResponse, error) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	if err := b.ensureStartedLocked(); err != nil {
		return nil, err
	}

	matching := make([]string, 0, len(b.state.Secrets))
	for path := range b.state.Secrets {
		if strings.HasPrefix(path, req.Prefix) {
			matching = append(matching, path)
		}
	}
	sort.Strings(matching)

	// Apply cursor: drop everything <= cursor.
	if req.Cursor != "" {
		idx := sort.SearchStrings(matching, req.Cursor)
		if idx < len(matching) && matching[idx] == req.Cursor {
			idx++
		}
		matching = matching[idx:]
	}

	out := &secrets.ListSecretsResponse{}
	for _, path := range matching {
		if req.Limit > 0 && len(out.Entries) >= req.Limit {
			out.NextCursor = out.Entries[len(out.Entries)-1].Path
			break
		}
		rec := b.state.Secrets[path]
		out.Entries = append(out.Entries, secrets.ListEntry{
			Path:      path,
			Version:   rec.Version,
			Metadata:  cloneStringMap(rec.Metadata),
			UpdatedAt: rec.UpdatedAt,
		})
	}
	return out, nil
}

// DeleteSecret removes the secret at req.Path. Returns
// [secrets.ErrSecretNotFound] on miss. If `req.Version` is non-zero,
// it must match the current version or [secrets.ErrInvalidBackend]
// is returned (CAS-style).
func (b *Backend) DeleteSecret(_ context.Context, req secrets.DeleteSecretRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if err := b.ensureStartedLocked(); err != nil {
		return err
	}

	existing, ok := b.state.Secrets[req.Path]
	if !ok {
		return fmt.Errorf("%w: %q", secrets.ErrSecretNotFound, req.Path)
	}
	if req.Version != 0 && req.Version != existing.Version {
		return fmt.Errorf("%w: CAS mismatch for delete %q (expected version %d, current %d)", secrets.ErrInvalidBackend, req.Path, req.Version, existing.Version)
	}

	prior := *existing
	delete(b.state.Secrets, req.Path)

	if err := b.persistLocked(); err != nil {
		b.state.Secrets[req.Path] = &prior
		return err
	}
	return nil
}

// IssueDynamicSecret is unsupported on the file backend — returned
// for defense-in-depth against direct callers that bypass the
// broker's capability check.
func (b *Backend) IssueDynamicSecret(_ context.Context, _ secrets.IssueDynamicSecretRequest) (*secrets.Secret, error) {
	return nil, fmt.Errorf("%w: file backend does not support capability %s", secrets.ErrInvalidBackend, secrets.CapDynamic)
}

// RenewLease is unsupported — see [Backend.IssueDynamicSecret].
func (b *Backend) RenewLease(_ context.Context, _ secrets.RenewLeaseRequest) (*secrets.LeaseInfo, error) {
	return nil, fmt.Errorf("%w: file backend does not support capability %s", secrets.ErrInvalidBackend, secrets.CapLeaseRenew)
}

// RevokeLease is unsupported — see [Backend.IssueDynamicSecret].
func (b *Backend) RevokeLease(_ context.Context, _ secrets.RevokeLeaseRequest) error {
	return fmt.Errorf("%w: file backend does not support capability %s", secrets.ErrInvalidBackend, secrets.CapLeaseRevoke)
}

// ensureStartedLocked validates the lifecycle. Caller MUST hold the
// mutex.
func (b *Backend) ensureStartedLocked() error {
	if !b.started || b.stopped {
		return secrets.ErrBackendNotStarted
	}
	return nil
}

// persistLocked marshals the in-memory state, encrypts it, and
// rewrites the file atomically. Caller MUST hold the write mutex.
func (b *Backend) persistLocked() error {
	plain, err := json.Marshal(b.state)
	if err != nil {
		return fmt.Errorf("%w: file backend: marshal state: %v", secrets.ErrInvalidBackend, err)
	}
	framed, err := encode(plain, b.key)
	if err != nil {
		return err
	}
	return writeAtomic(b.cfg.Path, framed)
}

// Compile-time interface assertion.
var _ secrets.SecretBackend = (*Backend)(nil)
