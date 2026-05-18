package storage_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.keystone-core.io/keystone-core/internal/registry/storage"
)

func newFS(t *testing.T) *storage.Filesystem {
	t.Helper()
	fs, err := storage.NewFilesystem(t.TempDir())
	if err != nil {
		t.Fatalf("NewFilesystem: %v", err)
	}
	return fs
}

func TestFilesystem_RoundTrip(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()
	const key = "vendor/pkg/1.2.3/module.zip"

	if ok, _ := fs.Exists(ctx, key); ok {
		t.Fatal("Exists before Put")
	}
	if err := fs.Put(ctx, key, strings.NewReader("ZIPBYTES")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	ok, err := fs.Exists(ctx, key)
	if err != nil || !ok {
		t.Fatalf("Exists = %v,%v", ok, err)
	}
	rc, err := fs.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	got, _ := io.ReadAll(rc)
	_ = rc.Close()
	if string(got) != "ZIPBYTES" {
		t.Fatalf("Get = %q", got)
	}
	st, err := fs.Stat(ctx, key)
	if err != nil || st.Size != 8 || st.ModTime.IsZero() {
		t.Fatalf("Stat = %+v,%v", st, err)
	}
	if err := fs.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	if err := fs.Delete(ctx, key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if err := fs.Delete(ctx, key); err != nil { // idempotent
		t.Fatalf("Delete idempotent: %v", err)
	}
	if ok, _ := fs.Exists(ctx, key); ok {
		t.Fatal("Exists after Delete")
	}
}

func TestFilesystem_GetStatMissing(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()
	if _, err := fs.Get(ctx, "no/such/key"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("Get missing = %v, want ErrNotExist", err)
	}
	if _, err := fs.Stat(ctx, "no/such/key"); !errors.Is(err, storage.ErrNotExist) {
		t.Fatalf("Stat missing = %v, want ErrNotExist", err)
	}
}

func TestFilesystem_List(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()
	for _, k := range []string{
		"v/a/1.0.0/info.json", "v/a/1.0.0/module.zip",
		"v/a/2.0.0/info.json", "v/b/1.0.0/info.json",
	} {
		if err := fs.Put(ctx, k, strings.NewReader("x")); err != nil {
			t.Fatal(err)
		}
	}
	all, err := fs.List(ctx, "v/a")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(all) != 3 {
		t.Fatalf("List(v/a) = %v, want 3", all)
	}
	// Sorted + scoped to the prefix (no v/b entries).
	for _, k := range all {
		if !strings.HasPrefix(k, "v/a/") {
			t.Fatalf("List leaked outside prefix: %q", k)
		}
	}
	// .put-* temp files are never listed.
	empty, err := fs.List(ctx, "does/not/exist")
	if err != nil || len(empty) != 0 {
		t.Fatalf("List missing prefix = %v,%v", empty, err)
	}
}

func TestFilesystem_TraversalRejected(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()
	for _, bad := range []string{
		"../escape", "a/../../etc/passwd", "/abs/path", "", "x\x00y",
	} {
		if err := fs.Put(ctx, bad, strings.NewReader("x")); !errors.Is(err, storage.ErrInvalidKey) {
			t.Errorf("Put(%q) = %v, want ErrInvalidKey", bad, err)
		}
		if _, err := fs.Get(ctx, bad); !errors.Is(err, storage.ErrInvalidKey) {
			t.Errorf("Get(%q) = %v, want ErrInvalidKey", bad, err)
		}
	}
	// A cleaned key that stays in root is fine.
	if err := fs.Put(ctx, "a/b/../c", bytes.NewReader([]byte("ok"))); err != nil {
		t.Fatalf("clean-in-root key: %v", err)
	}
}

func TestNewFilesystem_EmptyRoot(t *testing.T) {
	if _, err := storage.NewFilesystem(""); !errors.Is(err, storage.ErrInvalidKey) {
		t.Fatalf("NewFilesystem(\"\") = %v, want ErrInvalidKey", err)
	}
}

func TestNewFilesystem_RootIsAFile(t *testing.T) {
	f := filepath.Join(t.TempDir(), "afile")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := storage.NewFilesystem(filepath.Join(f, "sub")); err == nil {
		t.Fatal("NewFilesystem under a regular file: want error")
	}
}

func TestHealthFailsWhenRootGone(t *testing.T) {
	dir := t.TempDir()
	fs, err := storage.NewFilesystem(filepath.Join(dir, "r"))
	if err != nil {
		t.Fatal(err)
	}
	if err := fs.Health(context.Background()); err != nil {
		t.Fatalf("Health (healthy): %v", err)
	}
	if err := os.RemoveAll(filepath.Join(dir, "r")); err != nil {
		t.Fatal(err)
	}
	if err := fs.Health(context.Background()); err == nil {
		t.Fatal("Health after root removed: want error")
	}
}

func TestInvalidKeyOnEveryOp(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()
	const bad = "../escape"
	if err := fs.Delete(ctx, bad); !errors.Is(err, storage.ErrInvalidKey) {
		t.Errorf("Delete(bad) = %v", err)
	}
	if _, err := fs.Exists(ctx, bad); !errors.Is(err, storage.ErrInvalidKey) {
		t.Errorf("Exists(bad) = %v", err)
	}
	if _, err := fs.Stat(ctx, bad); !errors.Is(err, storage.ErrInvalidKey) {
		t.Errorf("Stat(bad) = %v", err)
	}
	if _, err := fs.List(ctx, bad); !errors.Is(err, storage.ErrInvalidKey) {
		t.Errorf("List(bad) = %v", err)
	}
}

func TestPut_ParentIsAFile(t *testing.T) {
	fs := newFS(t)
	ctx := context.Background()
	if err := fs.Put(ctx, "a", strings.NewReader("file")); err != nil {
		t.Fatalf("Put a: %v", err)
	}
	// "a" is now a regular file; using it as a directory must fail.
	if err := fs.Put(ctx, "a/b", strings.NewReader("x")); err == nil {
		t.Fatal("Put under a file parent: want error")
	}
}
