// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"path"
	"strings"
)

// Restore seam interfaces. Each method receives the byte stream (or
// reconstructed [ConfigFile] list) extracted from one component of
// the artifact tar. Implementations are responsible for the actual
// destructive work — replacing SQLite files, calling pg_restore,
// invoking etcd snapshot restore, etc. Concrete adapters land
// independently under the gate-v1.0 ROADMAP entry "Backup + restore
// component adapters"; this package ships only the orchestrator.

type StorageRestore interface {
	Restore(ctx context.Context, r io.Reader) error
}

type JetStreamRestore interface {
	Restore(ctx context.Context, r io.Reader) error
}

type EtcdRestore interface {
	Restore(ctx context.Context, r io.Reader) error
}

type ConfigRestore interface {
	Restore(ctx context.Context, files []ConfigFile) error
}

type SecretsRestore interface {
	Restore(ctx context.Context, r io.Reader) error
}

type ClusterRestore interface {
	Restore(ctx context.Context, r io.Reader) error
}

// ClusterDetector reports whether the target server already holds
// data. When wired into a [RestoreManager], a populated target
// refuses [RestoreManager.Restore] unless [RestoreOptions.Force] is
// true. A nil detector skips the check entirely — appropriate for
// tests and dev paths; the production CLI wires a real one.
type ClusterDetector interface {
	IsPopulated(ctx context.Context) (bool, error)
}

// Error sentinels — use errors.Is to discriminate without parsing
// strings.
var (
	ErrIntegrityCheckFailed = errors.New("backup: integrity check failed")
	ErrSchemaIncompatible   = errors.New("backup: schema incompatible")
	ErrClusterPopulated     = errors.New("backup: target cluster is populated (pass Force=true to override)")
	ErrManifestMissing      = errors.New("backup: manifest.json missing from artifact")
)

// Selection picks which components Restore should restore. The zero
// value selects nothing; use [SelectAll] for the typical full-restore
// case, [SelectConfigOnly] / [SelectSecretsOnly] for the partial
// modes called out in Epic 18 Scope §.
type Selection struct {
	Storage   bool
	JetStream bool
	Etcd      bool
	Config    bool
	Secrets   bool
	Cluster   bool
}

func SelectAll() Selection {
	return Selection{Storage: true, JetStream: true, Etcd: true, Config: true, Secrets: true, Cluster: true}
}

func SelectConfigOnly() Selection  { return Selection{Config: true} }
func SelectSecretsOnly() Selection { return Selection{Secrets: true} }

// any reports whether at least one component flag is set.
func (s Selection) any() bool {
	return s.Storage || s.JetStream || s.Etcd || s.Config || s.Secrets || s.Cluster
}

// has reports whether the named component (storage/jetstream/etcd/
// config/secrets/cluster) is selected.
func (s Selection) has(name string) bool {
	switch name {
	case "storage":
		return s.Storage
	case "jetstream":
		return s.JetStream
	case "etcd":
		return s.Etcd
	case "config":
		return s.Config
	case "secrets":
		return s.Secrets
	case "cluster":
		return s.Cluster
	}
	return false
}

// RestoreOption configures a [RestoreManager] at construction.
type RestoreOption func(*restoreOpts)

type restoreOpts struct {
	storage   StorageRestore
	jetstream JetStreamRestore
	etcd      EtcdRestore
	config    ConfigRestore
	secrets   SecretsRestore
	cluster   ClusterRestore
	detector  ClusterDetector
	logger    *slog.Logger
}

func WithStorageRestore(s StorageRestore) RestoreOption {
	return func(o *restoreOpts) { o.storage = s }
}
func WithJetStreamRestore(j JetStreamRestore) RestoreOption {
	return func(o *restoreOpts) { o.jetstream = j }
}
func WithEtcdRestore(e EtcdRestore) RestoreOption {
	return func(o *restoreOpts) { o.etcd = e }
}
func WithConfigRestore(c ConfigRestore) RestoreOption {
	return func(o *restoreOpts) { o.config = c }
}
func WithSecretsRestore(s SecretsRestore) RestoreOption {
	return func(o *restoreOpts) { o.secrets = s }
}
func WithClusterRestore(c ClusterRestore) RestoreOption {
	return func(o *restoreOpts) { o.cluster = c }
}
func WithClusterDetector(d ClusterDetector) RestoreOption {
	return func(o *restoreOpts) { o.detector = d }
}
func WithRestoreLogger(l *slog.Logger) RestoreOption {
	return func(o *restoreOpts) {
		if l != nil {
			o.logger = l
		}
	}
}

// RestoreOptions controls a single [RestoreManager.Restore] call.
type RestoreOptions struct {
	// Force bypasses the populated-cluster guard. Equivalent to the
	// kscore-backup restore --force flag.
	Force bool
	// Selection picks which components to restore. An empty
	// Selection (the zero value) restores nothing — the caller
	// should pass [SelectAll] explicitly.
	Selection Selection
}

// RestoreManager is the symmetric counterpart to [BackupManager].
type RestoreManager struct {
	opts restoreOpts
}

// NewRestoreManager constructs a RestoreManager. Pass
// [WithStorageRestore], [WithJetStreamRestore], etc. to wire
// component handlers; an unconfigured component handler causes a
// [Selection]-requested restore for that component to fail loudly at
// dispatch (operator asked us to restore a component we cannot
// restore).
func NewRestoreManager(opts ...RestoreOption) (*RestoreManager, error) {
	o := restoreOpts{logger: slog.Default()}
	for _, opt := range opts {
		opt(&o)
	}
	return &RestoreManager{opts: o}, nil
}

// Restore reads an artifact tar from r, verifies its manifest +
// per-component SHA-256s, optionally enforces the populated-cluster
// guard, and dispatches each selected component to its handler.
// Returns the verified [Manifest] so the caller can log taken-at,
// cluster name, and the component-count summary.
//
// The artifact is buffered into memory because the manifest sits at
// the end of the tar (Task 3 invariant) and integrity verification
// re-hashes every entry. v1.0 expects backups under a few GB;
// larger sizes are a v1.x concern (streaming-verify mode).
//
// Verify-only mode — passing an empty [RestoreOptions.Selection] (the
// zero value) runs every step EXCEPT handler dispatch: the manifest
// is parsed, the schema is checked, every component is re-hashed
// against its manifest entry, and the populated-cluster guard runs.
// No handler is invoked because no Selection flag is set. This is
// the contract kscore-backup verify relies on.
func (m *RestoreManager) Restore(ctx context.Context, r io.Reader, opts RestoreOptions) (*Manifest, error) {
	if r == nil {
		return nil, errors.New("backup: Restore: reader must not be nil")
	}

	entries, err := readTarEntries(r)
	if err != nil {
		return nil, fmt.Errorf("backup: read artifact: %w", err)
	}

	manifestBytes, ok := entries[ManifestFilename]
	if !ok {
		return nil, ErrManifestMissing
	}

	var manifest Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("backup: parse manifest: %w", err)
	}

	if manifest.FormatVersion != ManifestFormatV1 {
		return nil, fmt.Errorf("%w: artifact format %d, supported %d", ErrSchemaIncompatible, manifest.FormatVersion, ManifestFormatV1)
	}

	if err := verifyIntegrity(&manifest, entries); err != nil {
		return nil, err
	}

	if m.opts.detector != nil && !opts.Force {
		populated, err := m.opts.detector.IsPopulated(ctx)
		if err != nil {
			return nil, fmt.Errorf("backup: detect populated: %w", err)
		}
		if populated {
			return nil, ErrClusterPopulated
		}
	}

	if err := m.dispatch(ctx, &manifest, entries, opts.Selection); err != nil {
		return nil, err
	}

	m.opts.logger.InfoContext(ctx, "backup: restore complete",
		"components", len(manifest.Components),
		"cluster_name", manifest.ClusterName,
		"format_version", manifest.FormatVersion,
	)
	return &manifest, nil
}

// MaxTarEntryBytes caps each artifact tar entry's size during read.
// Protects against a malicious tar declaring a huge entry size and
// exhausting memory. v1.0 expects backup components below this size;
// operators with bigger backups need the v1.x streaming-restore
// mode tracked in ROADMAP.
const MaxTarEntryBytes = 4 << 30 // 4 GiB

// readTarEntries reads every entry into memory keyed by path. A
// duplicate path is an integrity-equivalent error; an entry larger
// than [MaxTarEntryBytes] is rejected.
func readTarEntries(r io.Reader) (map[string][]byte, error) {
	out := map[string][]byte{}
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("tar next: %w", err)
		}
		if hdr.Typeflag != tar.TypeReg && hdr.Typeflag != tar.TypeRegA { //nolint:staticcheck // TypeRegA is deprecated but still seen
			continue
		}
		if _, dup := out[hdr.Name]; dup {
			return nil, fmt.Errorf("%w: duplicate tar entry %q", ErrIntegrityCheckFailed, hdr.Name)
		}
		body := &bytes.Buffer{}
		// CopyN bounds the per-entry read at MaxTarEntryBytes; any
		// remaining bytes in the entry indicate an oversized entry
		// the artifact cannot legitimately contain.
		n, err := io.CopyN(body, tr, MaxTarEntryBytes+1)
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("tar body %q: %w", hdr.Name, err)
		}
		if n > MaxTarEntryBytes {
			return nil, fmt.Errorf("%w: entry %q exceeds %d bytes", ErrIntegrityCheckFailed, hdr.Name, MaxTarEntryBytes)
		}
		out[hdr.Name] = body.Bytes()
	}
	return out, nil
}

// verifyIntegrity asserts the three integrity invariants:
//   1. every manifest entry resolves to a tar entry,
//   2. every tar entry (except manifest.json) is referenced by some
//      manifest entry, and
//   3. SHA-256 of each tar entry matches the manifest record.
func verifyIntegrity(manifest *Manifest, entries map[string][]byte) error {
	referenced := map[string]bool{ManifestFilename: true}

	for _, c := range manifest.Components {
		body, ok := entries[c.Path]
		if !ok {
			return fmt.Errorf("%w: %s entry %q missing from tar", ErrIntegrityCheckFailed, c.Name, c.Path)
		}
		referenced[c.Path] = true

		if int64(len(body)) != c.Size {
			return fmt.Errorf("%w: %s size mismatch (tar=%d manifest=%d)", ErrIntegrityCheckFailed, c.Name, len(body), c.Size)
		}
		h := sha256.Sum256(body)
		got := hex.EncodeToString(h[:])
		if got != c.SHA256Hex {
			return fmt.Errorf("%w: %s SHA-256 mismatch", ErrIntegrityCheckFailed, c.Name)
		}
	}

	for name := range entries {
		if !referenced[name] {
			return fmt.Errorf("%w: unexpected tar entry %q", ErrIntegrityCheckFailed, name)
		}
	}
	return nil
}

// dispatch routes each selected component to its handler in the
// fixed write order. A handler error aborts immediately. A component
// selected but unhandled (no With*Restore option) is also an error.
func (m *RestoreManager) dispatch(ctx context.Context, manifest *Manifest, entries map[string][]byte, sel Selection) error {
	for _, c := range manifest.Components {
		if !sel.has(c.Name) {
			continue
		}
		body := entries[c.Path]
		switch c.Name {
		case "storage":
			if m.opts.storage == nil {
				return fmt.Errorf("backup: storage selected but no StorageRestore wired")
			}
			if err := m.opts.storage.Restore(ctx, bytes.NewReader(body)); err != nil {
				return fmt.Errorf("backup: restore storage: %w", err)
			}
		case "jetstream":
			if m.opts.jetstream == nil {
				return fmt.Errorf("backup: jetstream selected but no JetStreamRestore wired")
			}
			if err := m.opts.jetstream.Restore(ctx, bytes.NewReader(body)); err != nil {
				return fmt.Errorf("backup: restore jetstream: %w", err)
			}
		case "etcd":
			if m.opts.etcd == nil {
				return fmt.Errorf("backup: etcd selected but no EtcdRestore wired")
			}
			if err := m.opts.etcd.Restore(ctx, bytes.NewReader(body)); err != nil {
				return fmt.Errorf("backup: restore etcd: %w", err)
			}
		case "config":
			// Config dispatch is buffered: collect every config entry,
			// then call ConfigRestore once with the full list. This
			// matches the BackupManager side where ConfigCollector
			// returns []ConfigFile in one shot.
			// Handled after the loop to avoid invoking it once per
			// file; see configBatch below.
		case "secrets":
			if m.opts.secrets == nil {
				return fmt.Errorf("backup: secrets selected but no SecretsRestore wired")
			}
			if err := m.opts.secrets.Restore(ctx, bytes.NewReader(body)); err != nil {
				return fmt.Errorf("backup: restore secrets: %w", err)
			}
		case "cluster":
			if m.opts.cluster == nil {
				return fmt.Errorf("backup: cluster selected but no ClusterRestore wired")
			}
			if err := m.opts.cluster.Restore(ctx, bytes.NewReader(body)); err != nil {
				return fmt.Errorf("backup: restore cluster: %w", err)
			}
		}
	}

	if sel.Config {
		files := configBatch(manifest, entries)
		if len(files) > 0 {
			if m.opts.config == nil {
				return fmt.Errorf("backup: config selected but no ConfigRestore wired")
			}
			if err := m.opts.config.Restore(ctx, files); err != nil {
				return fmt.Errorf("backup: restore config: %w", err)
			}
		}
	}
	return nil
}

// configBatch reconstructs the [ConfigFile] list from the manifest.
// Each manifest "config" entry's Path is components/config/<name>;
// strip the prefix to recover the original basename.
func configBatch(manifest *Manifest, entries map[string][]byte) []ConfigFile {
	var out []ConfigFile
	const prefix = "components/config/"
	for _, c := range manifest.Components {
		if c.Name != "config" {
			continue
		}
		name := strings.TrimPrefix(c.Path, prefix)
		if name == c.Path {
			// Manifest path did not have the expected prefix; fall
			// back to the basename to keep the restore robust against
			// a hand-crafted manifest.
			name = path.Base(c.Path)
		}
		out = append(out, ConfigFile{Name: name, Body: entries[c.Path]})
	}
	return out
}
