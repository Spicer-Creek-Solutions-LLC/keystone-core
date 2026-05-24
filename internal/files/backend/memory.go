// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"bytes"
	"context"
	"errors"
	"io"
	"maps"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/files"
)

// MemoryStore is the in-memory [Store] used by service-layer unit
// tests. It is not goroutine-safe across processes (no flock); the
// single-process file service holds the only reference so the
// internal mutex suffices.
type MemoryStore struct {
	mu  sync.RWMutex
	now func() time.Time
	m   map[string]memoryEntry
}

type memoryEntry struct {
	meta files.FileMetadata
	body []byte
}

// NewMemoryStore returns an empty MemoryStore. The optional
// nowFunc lets tests inject deterministic timestamps; if nil,
// [time.Now] is used.
func NewMemoryStore(nowFunc func() time.Time) *MemoryStore {
	if nowFunc == nil {
		nowFunc = time.Now
	}
	return &MemoryStore{
		now: nowFunc,
		m:   make(map[string]memoryEntry),
	}
}

func (s *MemoryStore) Put(_ context.Context, meta files.FileMetadata, body io.Reader) (files.FileMetadata, error) {
	if err := validatePath(meta.Path); err != nil {
		return files.FileMetadata{}, err
	}
	buf, err := io.ReadAll(body)
	if err != nil {
		return files.FileMetadata{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	var nextVersion int64 = 1
	if prev, ok := s.m[meta.Path]; ok {
		nextVersion = prev.meta.Version + 1
	}

	final := files.FileMetadata{
		Path:        meta.Path,
		Size:        int64(len(buf)),
		Hash:        files.HashOf(buf),
		ContentType: meta.ContentType,
		CreatedAt:   s.now().UTC(),
		Version:     nextVersion,
		Tags:        maps.Clone(meta.Tags),
	}
	s.m[meta.Path] = memoryEntry{meta: final, body: buf}
	return final, nil
}

func (s *MemoryStore) Get(_ context.Context, path string) (files.FileMetadata, io.ReadCloser, error) {
	if err := validatePath(path); err != nil {
		return files.FileMetadata{}, nil, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.m[path]
	if !ok {
		return files.FileMetadata{}, nil, ErrNotFound
	}
	// Copy bytes so callers can't mutate our state through the slice.
	buf := bytes.NewReader(slices.Clone(e.body))
	return cloneMeta(e.meta), io.NopCloser(buf), nil
}

func (s *MemoryStore) Stat(_ context.Context, path string) (files.FileMetadata, error) {
	if err := validatePath(path); err != nil {
		return files.FileMetadata{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	e, ok := s.m[path]
	if !ok {
		return files.FileMetadata{}, ErrNotFound
	}
	return cloneMeta(e.meta), nil
}

func (s *MemoryStore) List(_ context.Context, prefix string) ([]files.FileMetadata, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]files.FileMetadata, 0, len(s.m))
	for p, e := range s.m {
		if prefix == "" || strings.HasPrefix(p, prefix) {
			out = append(out, cloneMeta(e.meta))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Path < out[j].Path })
	return out, nil
}

func (s *MemoryStore) Delete(_ context.Context, path string) error {
	if err := validatePath(path); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.m[path]; !ok {
		return ErrNotFound
	}
	delete(s.m, path)
	return nil
}

// cloneMeta returns a deep copy so callers don't share map state.
func cloneMeta(m files.FileMetadata) files.FileMetadata {
	out := m
	out.Tags = maps.Clone(m.Tags)
	return out
}

// validatePath enforces the same invariants as
// [files.FileMetadata.Validate]'s path check, scoped to a single
// argument. The backend revalidates because Get, Stat, Delete, and
// List accept a path directly rather than a full FileMetadata.
func validatePath(p string) error {
	if p == "" {
		return errors.New("backend: path must not be empty")
	}
	if strings.HasPrefix(p, "/") {
		return errors.New("backend: path must not start with '/'")
	}
	if strings.HasSuffix(p, "/") {
		return errors.New("backend: path must not end with '/'")
	}
	if strings.Contains(p, "//") {
		return errors.New("backend: path must not contain empty segments")
	}
	for _, tok := range strings.Split(p, "/") {
		if tok == ".." {
			return errors.New("backend: path must not contain '..' segments")
		}
		if tok == "" {
			return errors.New("backend: path must not contain empty segments")
		}
		if strings.ContainsAny(tok, " \t\r\n") {
			return errors.New("backend: path must not contain whitespace")
		}
	}
	return nil
}
