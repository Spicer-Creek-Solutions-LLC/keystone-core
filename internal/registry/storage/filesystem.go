// SPDX-License-Identifier: Apache-2.0

package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
)

// Filesystem is the v1.0 filesystem-backed Storage. Keys map to
// files under root; every key is validated to stay within root
// (no traversal, no absolute paths).
type Filesystem struct {
	root string
}

// NewFilesystem returns a filesystem backend rooted at root,
// creating it if absent.
func NewFilesystem(root string) (*Filesystem, error) {
	if root == "" {
		return nil, fmt.Errorf("%w: empty root", ErrInvalidKey)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("storage: resolve root: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("storage: create root: %w", err)
	}
	return &Filesystem{root: abs}, nil
}

// resolve validates key and maps it to an absolute path guaranteed
// to be within root.
func (f *Filesystem) resolve(key string) (string, error) {
	if key == "" || strings.HasPrefix(key, "/") || strings.Contains(key, "\x00") {
		return "", fmt.Errorf("%w: %q", ErrInvalidKey, key)
	}
	// Clean (collapses internal `..` like a/b/../c → a/c). A result
	// that is "." or starts with ".." escapes root → reject. An
	// internal `..` that stays within root is fine.
	clean := path.Clean(key)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || path.IsAbs(clean) {
		return "", fmt.Errorf("%w: %q escapes root", ErrInvalidKey, key)
	}
	p := filepath.Join(f.root, filepath.FromSlash(clean))
	// Defence in depth: the joined path must stay under root.
	if p != f.root && !strings.HasPrefix(p, f.root+string(os.PathSeparator)) {
		return "", fmt.Errorf("%w: %q escapes root", ErrInvalidKey, key)
	}
	return p, nil
}

// Get opens the object at key.
func (f *Filesystem) Get(_ context.Context, key string) (io.ReadCloser, error) {
	p, err := f.resolve(key)
	if err != nil {
		return nil, err
	}
	file, err := os.Open(p) //nolint:gosec // p is validated to be within root
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("%w: %s", ErrNotExist, key)
		}
		return nil, err
	}
	return file, nil
}

// Put writes r to key atomically (temp + rename), creating parent
// directories.
func (f *Filesystem) Put(_ context.Context, key string, r io.Reader) error {
	p, err := f.resolve(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("storage: mkdir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(p), ".put-*")
	if err != nil {
		return fmt.Errorf("storage: temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := io.Copy(tmp, r); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage: close: %w", err)
	}
	if err := os.Rename(tmpName, p); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("storage: commit: %w", err)
	}
	return nil
}

// Delete removes key (idempotent — absent is not an error).
func (f *Filesystem) Delete(_ context.Context, key string) error {
	p, err := f.resolve(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("storage: delete: %w", err)
	}
	return nil
}

// List returns every object key under prefix (recursive), sorted.
func (f *Filesystem) List(_ context.Context, prefix string) ([]string, error) {
	base := f.root
	if prefix != "" {
		p, err := f.resolve(prefix)
		if err != nil {
			return nil, err
		}
		base = p
	}
	var keys []string
	err := filepath.WalkDir(base, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil // empty / missing prefix → no keys
			}
			return err
		}
		if d.IsDir() || strings.HasPrefix(d.Name(), ".put-") {
			return nil
		}
		rel, rerr := filepath.Rel(f.root, p)
		if rerr != nil {
			return rerr
		}
		keys = append(keys, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("storage: list: %w", err)
	}
	sort.Strings(keys)
	return keys, nil
}

// Exists reports whether key has a regular-file object.
func (f *Filesystem) Exists(_ context.Context, key string) (bool, error) {
	p, err := f.resolve(key)
	if err != nil {
		return false, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return fi.Mode().IsRegular(), nil
}

// Stat returns object metadata.
func (f *Filesystem) Stat(_ context.Context, key string) (Info, error) {
	p, err := f.resolve(key)
	if err != nil {
		return Info{}, err
	}
	fi, err := os.Stat(p)
	if err != nil {
		if os.IsNotExist(err) {
			return Info{}, fmt.Errorf("%w: %s", ErrNotExist, key)
		}
		return Info{}, err
	}
	return Info{Size: fi.Size(), ModTime: fi.ModTime()}, nil
}

// Health verifies the root is a writable directory.
func (f *Filesystem) Health(_ context.Context) error {
	fi, err := os.Stat(f.root)
	if err != nil {
		return fmt.Errorf("storage: health: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("storage: health: root %s is not a directory", f.root)
	}
	probe := filepath.Join(f.root, ".health")
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		return fmt.Errorf("storage: health: not writable: %w", err)
	}
	_ = os.Remove(probe)
	return nil
}
