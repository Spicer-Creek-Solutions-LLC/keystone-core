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
	"time"
)

// StorageBackup writes the kscore-server state-store backup (pg_dump
// output for Postgres, online-backup bytes for SQLite) into w. The
// orchestrator computes size + SHA-256 as the bytes stream through.
type StorageBackup interface {
	Dump(ctx context.Context, w io.Writer) error
}

// JetStreamBackup writes a snapshot of every kscore JetStream stream
// into w. Concrete layout (single concatenation vs. nested tar) is
// the adapter's choice; the orchestrator treats it as opaque bytes.
type JetStreamBackup interface {
	Snapshot(ctx context.Context, w io.Writer) error
}

// EtcdBackup writes an etcd v3 snapshot ([clientv3.Snapshot]) into w.
// Only relevant when the server is configured with cluster.enabled =
// true; single-node deployments skip this seam.
type EtcdBackup interface {
	Snapshot(ctx context.Context, w io.Writer) error
}

// ConfigCollector enumerates the operator-supplied config files that
// should ride along in the backup. Each returned [ConfigFile] becomes
// its own tar entry under `components/config/`; restore writes them
// back to disk.
type ConfigCollector interface {
	Collect(ctx context.Context) ([]ConfigFile, error)
}

// ConfigFile is one entry returned by [ConfigCollector]. Name is the
// basename used to build the tar entry path; Body is the file
// contents (typically already-loaded YAML).
type ConfigFile struct {
	Name string
	Body []byte
}

// SecretsBackup copies the encrypted-file secrets backend's storage
// into w. The contents are *already encrypted at rest* by the
// secrets backend — the backup neither re-encrypts nor decrypts.
// Epic 18 task 4 adds a separate age envelope around the whole tar.
type SecretsBackup interface {
	Copy(ctx context.Context, w io.Writer) error
}

// ClusterMetadata writes the cluster membership + shard map snapshot
// (see internal/cluster.MarshalSnapshot) into w. Single-node servers
// pass a nil adapter.
type ClusterMetadata interface {
	Write(ctx context.Context, w io.Writer) error
}

// Option configures a [BackupManager].
type Option func(*managerOptions)

type managerOptions struct {
	storage     StorageBackup
	jetstream   JetStreamBackup
	etcd        EtcdBackup
	config      ConfigCollector
	secrets     SecretsBackup
	cluster     ClusterMetadata
	clock       func() time.Time
	logger      *slog.Logger
	clusterName string
}

// WithStorage wires the storage-backend backup seam.
func WithStorage(b StorageBackup) Option {
	return func(o *managerOptions) { o.storage = b }
}

// WithJetStream wires the JetStream backup seam.
func WithJetStream(b JetStreamBackup) Option {
	return func(o *managerOptions) { o.jetstream = b }
}

// WithEtcd wires the etcd backup seam.
func WithEtcd(b EtcdBackup) Option {
	return func(o *managerOptions) { o.etcd = b }
}

// WithConfig wires the config-file collector.
func WithConfig(c ConfigCollector) Option {
	return func(o *managerOptions) { o.config = c }
}

// WithSecrets wires the secrets-backend file copier.
func WithSecrets(s SecretsBackup) Option {
	return func(o *managerOptions) { o.secrets = s }
}

// WithCluster wires the cluster-metadata serializer.
func WithCluster(c ClusterMetadata) Option {
	return func(o *managerOptions) { o.cluster = c }
}

// WithClock overrides time.Now for deterministic tests.
func WithClock(fn func() time.Time) Option {
	return func(o *managerOptions) {
		if fn != nil {
			o.clock = fn
		}
	}
}

// WithLogger overrides slog.Default for backup-progress logs.
func WithLogger(l *slog.Logger) Option {
	return func(o *managerOptions) {
		if l != nil {
			o.logger = l
		}
	}
}

// WithClusterName stamps a human-readable cluster identifier into
// the manifest. Optional but recommended for multi-cluster operators.
func WithClusterName(name string) Option {
	return func(o *managerOptions) { o.clusterName = name }
}

// BackupManager orchestrates a portable backup artifact. Component
// adapters are injected via [Option]; an unconfigured component is
// skipped silently (the manifest records only what was written).
type BackupManager struct {
	opts managerOptions
}

// NewBackupManager constructs a manager with the supplied options.
// All component seams are independently optional; pass [WithStorage],
// [WithJetStream], etc. to wire the ones you want included.
func NewBackupManager(opts ...Option) (*BackupManager, error) {
	o := managerOptions{
		clock:  time.Now,
		logger: slog.Default(),
	}
	for _, opt := range opts {
		opt(&o)
	}
	return &BackupManager{opts: o}, nil
}

// CreateBackup writes a tar artifact to w and returns the manifest.
//
// Layout — components are written in fixed order (storage, jetstream,
// etcd, config files, secrets, cluster); [ManifestFilename] is
// written last so a streaming tar reader can locate it deterministically
// by scanning to EOF. Each component's bytes pass through a SHA-256
// tee + byte counter so the manifest records integrity metadata
// without buffering the artifact a second time.
//
// Any component error aborts the backup: the caller still owns w and
// is responsible for discarding the partial output (an
// age-encrypted-file wrapper at the next epic task layer makes the
// partial artifact unreadable anyway). The returned error wraps the
// component error with the component name.
func (m *BackupManager) CreateBackup(ctx context.Context, w io.Writer) (*Manifest, error) {
	if w == nil {
		return nil, errors.New("backup: CreateBackup: writer must not be nil")
	}

	tw := tar.NewWriter(w)
	manifest := &Manifest{
		FormatVersion: ManifestFormatV1,
		TakenAt:       m.opts.clock().UTC(),
		ClusterName:   m.opts.clusterName,
	}

	writeComponent := func(name, path string, source func(io.Writer) error) error {
		buf := &bytes.Buffer{}
		h := sha256.New()
		mw := io.MultiWriter(buf, h)
		if err := source(mw); err != nil {
			return err
		}
		size := int64(buf.Len())
		hdr := &tar.Header{
			Name:    path,
			Mode:    0o600,
			Size:    size,
			ModTime: manifest.TakenAt,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("tar header %q: %w", path, err)
		}
		if _, err := io.Copy(tw, buf); err != nil {
			return fmt.Errorf("tar body %q: %w", path, err)
		}
		manifest.Components = append(manifest.Components, ComponentEntry{
			Name:      name,
			Path:      path,
			Size:      size,
			SHA256Hex: hex.EncodeToString(h.Sum(nil)),
		})
		return nil
	}

	if m.opts.storage != nil {
		if err := writeComponent("storage", "components/storage.bin", func(w io.Writer) error {
			return m.opts.storage.Dump(ctx, w)
		}); err != nil {
			return nil, fmt.Errorf("backup: storage: %w", err)
		}
	}
	if m.opts.jetstream != nil {
		if err := writeComponent("jetstream", "components/jetstream.tar", func(w io.Writer) error {
			return m.opts.jetstream.Snapshot(ctx, w)
		}); err != nil {
			return nil, fmt.Errorf("backup: jetstream: %w", err)
		}
	}
	if m.opts.etcd != nil {
		if err := writeComponent("etcd", "components/etcd.snapshot", func(w io.Writer) error {
			return m.opts.etcd.Snapshot(ctx, w)
		}); err != nil {
			return nil, fmt.Errorf("backup: etcd: %w", err)
		}
	}
	if m.opts.config != nil {
		files, err := m.opts.config.Collect(ctx)
		if err != nil {
			return nil, fmt.Errorf("backup: config collect: %w", err)
		}
		for _, f := range files {
			path := "components/config/" + f.Name
			body := f.Body
			if err := writeComponent("config", path, func(w io.Writer) error {
				_, err := w.Write(body)
				return err
			}); err != nil {
				return nil, fmt.Errorf("backup: config %q: %w", f.Name, err)
			}
		}
	}
	if m.opts.secrets != nil {
		if err := writeComponent("secrets", "components/secrets.bin", func(w io.Writer) error {
			return m.opts.secrets.Copy(ctx, w)
		}); err != nil {
			return nil, fmt.Errorf("backup: secrets: %w", err)
		}
	}
	if m.opts.cluster != nil {
		if err := writeComponent("cluster", "components/cluster.snapshot", func(w io.Writer) error {
			return m.opts.cluster.Write(ctx, w)
		}); err != nil {
			return nil, fmt.Errorf("backup: cluster: %w", err)
		}
	}

	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("backup: marshal manifest: %w", err)
	}
	mfHdr := &tar.Header{
		Name:    ManifestFilename,
		Mode:    0o600,
		Size:    int64(len(manifestBytes)),
		ModTime: manifest.TakenAt,
	}
	if err := tw.WriteHeader(mfHdr); err != nil {
		return nil, fmt.Errorf("backup: tar header manifest: %w", err)
	}
	if _, err := tw.Write(manifestBytes); err != nil {
		return nil, fmt.Errorf("backup: tar body manifest: %w", err)
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("backup: close tar: %w", err)
	}

	m.opts.logger.InfoContext(ctx, "backup: artifact written",
		"components", len(manifest.Components),
		"cluster_name", manifest.ClusterName,
	)
	return manifest, nil
}
