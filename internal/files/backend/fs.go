// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/files"
)

// FilesystemStore persists files under a single root directory:
//
//	<root>/data/<path>          file body
//	<root>/meta/<path>.json     marshalled [files.FileMetadata]
//
// Writes are atomic via temp-file + rename. A per-store mutex
// serialises Put so the version-assignment read-modify-write window
// is race-free within one process. Cross-process safety would need
// flock and is not a v1.0 concern (the file service runs in one
// process; cluster mode is post-v1.0 for files).
type FilesystemStore struct {
	root string
	mu   sync.Mutex
	now  func() time.Time
}

const (
	fsDataSubdir = "data"
	fsMetaSubdir = "meta"
	fsMetaSuffix = ".json"
	fsTempSuffix = ".tmp"
	// fsFileMode applies to file bodies + meta JSON. 0o600 keeps
	// the artifacts owner-only — matches the encrypted-secrets +
	// kscore-backup precedent (G302 mitigation).
	fsFileMode = 0o600
	// fsDirMode is the operator-only directory mode (gosec G301).
	fsDirMode = 0o750
)

// NewFilesystemStore returns a [FilesystemStore] rooted at root.
// The root and its data/meta subdirectories are created on first
// write; an empty root is valid. nowFunc lets tests inject
// deterministic timestamps (nil → [time.Now]).
func NewFilesystemStore(root string, nowFunc func() time.Time) (*FilesystemStore, error) {
	if root == "" {
		return nil, errors.New("backend: filesystem root must not be empty")
	}
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &FilesystemStore{root: root, now: nowFunc}, nil
}

func (s *FilesystemStore) Put(_ context.Context, meta files.FileMetadata, body io.Reader) (files.FileMetadata, error) {
	if err := validatePath(meta.Path); err != nil {
		return files.FileMetadata{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var nextVersion int64 = 1
	if prev, err := s.statLocked(meta.Path); err == nil {
		nextVersion = prev.Version + 1
	} else if !errors.Is(err, ErrNotFound) {
		return files.FileMetadata{}, err
	}

	dataPath, err := s.dataPath(meta.Path)
	if err != nil {
		return files.FileMetadata{}, err
	}
	metaPath, err := s.metaPath(meta.Path)
	if err != nil {
		return files.FileMetadata{}, err
	}
	if err := os.MkdirAll(filepath.Dir(dataPath), fsDirMode); err != nil {
		return files.FileMetadata{}, fmt.Errorf("backend: mkdir data: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(metaPath), fsDirMode); err != nil {
		return files.FileMetadata{}, fmt.Errorf("backend: mkdir meta: %w", err)
	}

	size, hashHex, err := writeBodyAtomic(dataPath, body)
	if err != nil {
		return files.FileMetadata{}, err
	}

	final := files.FileMetadata{
		Path:        meta.Path,
		Size:        size,
		Hash:        hashHex,
		ContentType: meta.ContentType,
		CreatedAt:   s.now().UTC(),
		Version:     nextVersion,
		Tags:        maps.Clone(meta.Tags),
	}
	if err := writeMetaAtomic(metaPath, final); err != nil {
		// Body landed but meta did not — best-effort cleanup of the
		// orphan body so the store doesn't accumulate ghosts on
		// repeated failures.
		_ = os.Remove(dataPath)
		return files.FileMetadata{}, err
	}
	return final, nil
}

func (s *FilesystemStore) Get(_ context.Context, path string) (files.FileMetadata, io.ReadCloser, error) {
	if err := validatePath(path); err != nil {
		return files.FileMetadata{}, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	meta, err := s.statLocked(path)
	if err != nil {
		return files.FileMetadata{}, nil, err
	}
	dataPath, err := s.dataPath(path)
	if err != nil {
		return files.FileMetadata{}, nil, err
	}
	f, err := os.Open(dataPath) //nolint:gosec // path is post-validation + joined against root
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return files.FileMetadata{}, nil, ErrNotFound
		}
		return files.FileMetadata{}, nil, fmt.Errorf("backend: open body: %w", err)
	}
	return meta, f, nil
}

func (s *FilesystemStore) Stat(_ context.Context, path string) (files.FileMetadata, error) {
	if err := validatePath(path); err != nil {
		return files.FileMetadata{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.statLocked(path)
}

func (s *FilesystemStore) List(_ context.Context, prefix string) ([]files.FileMetadata, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	metaRoot := filepath.Join(s.root, fsMetaSubdir)
	out := []files.FileMetadata{}
	err := filepath.WalkDir(metaRoot, func(p string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			if errors.Is(walkErr, os.ErrNotExist) {
				return filepath.SkipAll
			}
			return walkErr
		}
		if d.IsDir() || !strings.HasSuffix(p, fsMetaSuffix) {
			return nil
		}
		rel, err := filepath.Rel(metaRoot, p)
		if err != nil {
			return err
		}
		filePath := filepath.ToSlash(strings.TrimSuffix(rel, fsMetaSuffix))
		if prefix != "" && !strings.HasPrefix(filePath, prefix) {
			return nil
		}
		m, err := readMeta(p)
		if err != nil {
			return err
		}
		out = append(out, m)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (s *FilesystemStore) Delete(_ context.Context, path string) error {
	if err := validatePath(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	metaPath, err := s.metaPath(path)
	if err != nil {
		return err
	}
	if _, err := os.Stat(metaPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("backend: stat meta: %w", err)
	}

	dataPath, err := s.dataPath(path)
	if err != nil {
		return err
	}
	// Remove meta first so a partial delete (data gone, meta
	// remains) cannot reappear in List with a missing body. The
	// inverse — meta gone, data orphan — is invisible to List.
	if err := os.Remove(metaPath); err != nil {
		return fmt.Errorf("backend: remove meta: %w", err)
	}
	if err := os.Remove(dataPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("backend: remove body: %w", err)
	}
	return nil
}

// statLocked reads metadata under the assumption that the caller
// holds s.mu.
func (s *FilesystemStore) statLocked(path string) (files.FileMetadata, error) {
	metaPath, err := s.metaPath(path)
	if err != nil {
		return files.FileMetadata{}, err
	}
	m, err := readMeta(metaPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return files.FileMetadata{}, ErrNotFound
		}
		return files.FileMetadata{}, err
	}
	return m, nil
}

// dataPath returns the absolute on-disk body path for p.
func (s *FilesystemStore) dataPath(p string) (string, error) {
	return safeJoin(s.root, fsDataSubdir, p)
}

// metaPath returns the absolute on-disk metadata path for p.
func (s *FilesystemStore) metaPath(p string) (string, error) {
	joined, err := safeJoin(s.root, fsMetaSubdir, p)
	if err != nil {
		return "", err
	}
	return joined + fsMetaSuffix, nil
}

// safeJoin maps a slash-delimited path under <root>/<sub>/ and
// verifies the result stays inside that subtree. It is the
// defense-in-depth path-traversal guard on top of [validatePath].
func safeJoin(root, sub, p string) (string, error) {
	target := filepath.Join(root, sub, filepath.FromSlash(p))
	cleanRoot, err := filepath.Abs(filepath.Join(root, sub))
	if err != nil {
		return "", err
	}
	cleanTarget, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	if cleanTarget != cleanRoot && !strings.HasPrefix(cleanTarget, cleanRoot+string(filepath.Separator)) {
		return "", fmt.Errorf("backend: path %q escapes root", p)
	}
	return cleanTarget, nil
}

// writeBodyAtomic streams r into <dataPath>.tmp, computing the
// SHA-256 hash + size as it copies, then renames into place. The
// rename is the commit point.
func writeBodyAtomic(dataPath string, r io.Reader) (int64, string, error) {
	tmp := dataPath + fsTempSuffix
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fsFileMode) //nolint:gosec // tmp is post-safeJoin

	if err != nil {
		return 0, "", fmt.Errorf("backend: open body tmp: %w", err)
	}
	hasher := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, hasher), r)
	closeErr := f.Close()
	if err != nil {
		_ = os.Remove(tmp)
		return 0, "", fmt.Errorf("backend: write body: %w", err)
	}
	if closeErr != nil {
		_ = os.Remove(tmp)
		return 0, "", fmt.Errorf("backend: close body tmp: %w", closeErr)
	}
	if err := os.Rename(tmp, dataPath); err != nil {
		_ = os.Remove(tmp)
		return 0, "", fmt.Errorf("backend: rename body: %w", err)
	}
	return n, hex.EncodeToString(hasher.Sum(nil)), nil
}

// writeMetaAtomic marshals meta to JSON and writes it atomically.
func writeMetaAtomic(metaPath string, meta files.FileMetadata) error {
	tmp := metaPath + fsTempSuffix
	b, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return fmt.Errorf("backend: marshal meta: %w", err)
	}
	if err := os.WriteFile(tmp, b, fsFileMode); err != nil {
		return fmt.Errorf("backend: write meta tmp: %w", err)
	}
	if err := os.Rename(tmp, metaPath); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("backend: rename meta: %w", err)
	}
	return nil
}

// readMeta loads + unmarshals a metadata file.
func readMeta(metaPath string) (files.FileMetadata, error) {
	b, err := os.ReadFile(metaPath) //nolint:gosec // path is post-safeJoin
	if err != nil {
		return files.FileMetadata{}, err
	}
	var m files.FileMetadata
	if err := json.Unmarshal(b, &m); err != nil {
		return files.FileMetadata{}, fmt.Errorf("backend: parse meta %s: %w", metaPath, err)
	}
	return m, nil
}
