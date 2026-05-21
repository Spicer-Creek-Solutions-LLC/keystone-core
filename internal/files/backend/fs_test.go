package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/files"
)

func TestFilesystemStore_Conformance(t *testing.T) {
	runConformance(t, func(t *testing.T) Store {
		s, err := NewFilesystemStore(t.TempDir(), nil)
		if err != nil {
			t.Fatalf("NewFilesystemStore: %v", err)
		}
		return s
	})
}

func TestNewFilesystemStore_RejectsEmptyRoot(t *testing.T) {
	if _, err := NewFilesystemStore("", nil); err == nil {
		t.Fatal("want error for empty root")
	}
}

func TestFilesystemStore_OnDiskLayout(t *testing.T) {
	// Verify the documented <root>/data/<path> + <root>/meta/<path>.json
	// layout — the rest of the suite should be impl-agnostic so this
	// check lives separately.
	root := t.TempDir()
	s, err := NewFilesystemStore(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	body := []byte("hello")
	if _, err := s.Put(context.Background(), files.FileMetadata{Path: "configs/app.yaml"}, bytes.NewReader(body)); err != nil {
		t.Fatal(err)
	}

	dataPath := filepath.Join(root, "data", "configs", "app.yaml")
	metaPath := filepath.Join(root, "meta", "configs", "app.yaml.json")
	if got, err := os.ReadFile(dataPath); err != nil || !bytes.Equal(got, body) {
		t.Errorf("data file = (%q, %v), want (%q, nil)", got, err, body)
	}
	mb, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta: %v", err)
	}
	var parsed files.FileMetadata
	if err := json.Unmarshal(mb, &parsed); err != nil {
		t.Fatalf("parse meta: %v", err)
	}
	if parsed.Path != "configs/app.yaml" {
		t.Errorf("parsed.Path = %q, want configs/app.yaml", parsed.Path)
	}
	if parsed.Hash != files.HashOf(body) {
		t.Errorf("parsed.Hash mismatch")
	}
}

func TestFilesystemStore_AtomicWrite_NoOrphanTmpOnSuccess(t *testing.T) {
	root := t.TempDir()
	s, _ := NewFilesystemStore(root, nil)
	if _, err := s.Put(context.Background(), files.FileMetadata{Path: "a/b/c"}, bytes.NewReader([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, _ error) error {
		if !d.IsDir() && strings.HasSuffix(path, fsTempSuffix) {
			t.Errorf("leftover tmp file: %s", path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestFilesystemStore_BodyWriteErrorIsClean(t *testing.T) {
	root := t.TempDir()
	s, _ := NewFilesystemStore(root, nil)
	_, err := s.Put(context.Background(), files.FileMetadata{Path: "x"}, errReader{})
	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want errBoom-wrapped", err)
	}
	// No persistent state should have landed.
	dataPath := filepath.Join(root, "data", "x")
	metaPath := filepath.Join(root, "meta", "x.json")
	if _, err := os.Stat(dataPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("body present after error: %v", err)
	}
	if _, err := os.Stat(metaPath); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("meta present after error: %v", err)
	}
}

func TestFilesystemStore_VersionPersistsAcrossInstances(t *testing.T) {
	root := t.TempDir()
	s1, _ := NewFilesystemStore(root, nil)
	for i := 0; i < 3; i++ {
		if _, err := s1.Put(context.Background(), files.FileMetadata{Path: "p"}, bytes.NewReader([]byte{byte('a' + i)})); err != nil {
			t.Fatal(err)
		}
	}
	// Reopen — version counter must come from disk, not from the
	// in-process state.
	s2, _ := NewFilesystemStore(root, nil)
	m, err := s2.Put(context.Background(), files.FileMetadata{Path: "p"}, bytes.NewReader([]byte("d")))
	if err != nil {
		t.Fatal(err)
	}
	if m.Version != 4 {
		t.Errorf("version after reopen = %d, want 4", m.Version)
	}
}

func TestFilesystemStore_PathTraversalRejected(t *testing.T) {
	// validatePath already catches "..", but exercise the safeJoin
	// defense for paths that contain only allowed runes yet would
	// resolve outside the root.
	root := t.TempDir()
	s, _ := NewFilesystemStore(root, nil)
	// Reaching outside via filepath.FromSlash mapping: the safeJoin
	// guard treats absolute leakage as an error. We cannot easily
	// construct a string that bypasses validatePath but trips
	// safeJoin (validatePath is strict); the explicit safeJoin test
	// below exercises the guard directly.
	_, err := safeJoin(root, "data", "../../etc/passwd")
	if err == nil {
		t.Error("safeJoin accepted traversal path")
	}
	_, err = s.Stat(context.Background(), "ok")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Stat after traversal probe: %v", err)
	}
}

func TestFilesystemStore_ListIgnoresNonMetaFiles(t *testing.T) {
	// A stray non-.json file under <root>/meta/ must not appear in
	// the listing — operators sometimes drop README or .DS_Store
	// files into directories.
	root := t.TempDir()
	s, _ := NewFilesystemStore(root, nil)
	ctx := context.Background()
	if _, err := s.Put(ctx, files.FileMetadata{Path: "real"}, bytes.NewReader([]byte("x"))); err != nil {
		t.Fatal(err)
	}
	stray := filepath.Join(root, "meta", "stray.txt")
	if err := os.WriteFile(stray, []byte("ignore me"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := s.List(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Path != "real" {
		t.Errorf("list = %+v, want [real]", got)
	}
}

func TestFilesystemStore_GetClosesReader(t *testing.T) {
	root := t.TempDir()
	s, _ := NewFilesystemStore(root, nil)
	ctx := context.Background()
	if _, err := s.Put(ctx, files.FileMetadata{Path: "p"}, bytes.NewReader([]byte("z"))); err != nil {
		t.Fatal(err)
	}
	_, rc, err := s.Get(ctx, "p")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(rc); err != nil {
		t.Fatal(err)
	}
	if err := rc.Close(); err != nil {
		t.Errorf("Close err = %v", err)
	}
}

func TestFilesystemStore_DeterministicTime(t *testing.T) {
	fixed := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	s, _ := NewFilesystemStore(t.TempDir(), func() time.Time { return fixed })
	m, err := s.Put(context.Background(), files.FileMetadata{Path: "p"}, bytes.NewReader([]byte("z")))
	if err != nil {
		t.Fatal(err)
	}
	if !m.CreatedAt.Equal(fixed) {
		t.Errorf("CreatedAt = %v, want %v", m.CreatedAt, fixed)
	}
}

func TestFilesystemStore_CorruptMetaSurfacesError(t *testing.T) {
	root := t.TempDir()
	s, _ := NewFilesystemStore(root, nil)
	ctx := context.Background()
	if _, err := s.Put(ctx, files.FileMetadata{Path: "p"}, bytes.NewReader([]byte("z"))); err != nil {
		t.Fatal(err)
	}
	metaPath := filepath.Join(root, "meta", "p.json")
	if err := os.WriteFile(metaPath, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat(ctx, "p"); err == nil {
		t.Error("Stat returned nil on corrupt meta")
	}
	if _, err := s.List(ctx, ""); err == nil {
		t.Error("List returned nil on corrupt meta")
	}
}
