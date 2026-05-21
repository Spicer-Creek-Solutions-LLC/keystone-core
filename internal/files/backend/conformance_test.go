package backend

import (
	"bytes"
	"context"
	"errors"
	"io"
	"sort"
	"testing"

	"go.keystone-core.io/keystone-core/internal/files"
)

// runConformance is the shared conformance suite every [Store] impl
// is exercised against. Each impl wires this from its own test file
// with a factory closure (see memory_test.go, fs_test.go,
// s3_test.go).
func runConformance(t *testing.T, factory func(t *testing.T) Store) {
	t.Helper()

	t.Run("PutGet_RoundTrip", func(t *testing.T) {
		s := factory(t)
		ctx := context.Background()
		body := []byte("hello world")

		put, err := s.Put(ctx, files.FileMetadata{
			Path:        "configs/app/main.yaml",
			ContentType: "application/yaml",
			Tags:        map[string]string{"env": "prod"},
		}, bytes.NewReader(body))
		if err != nil {
			t.Fatalf("Put: %v", err)
		}
		if put.Version != 1 {
			t.Errorf("first version = %d, want 1", put.Version)
		}
		if put.Size != int64(len(body)) {
			t.Errorf("size = %d, want %d", put.Size, len(body))
		}
		if put.Hash != files.HashOf(body) {
			t.Errorf("hash = %q, want %q", put.Hash, files.HashOf(body))
		}
		if put.Path != "configs/app/main.yaml" || put.ContentType != "application/yaml" {
			t.Errorf("metadata not preserved: %+v", put)
		}
		if put.Tags["env"] != "prod" {
			t.Errorf("tags = %+v, want env=prod", put.Tags)
		}
		if put.CreatedAt.IsZero() {
			t.Error("CreatedAt is zero")
		}

		meta, rc, err := s.Get(ctx, "configs/app/main.yaml")
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer func() { _ = rc.Close() }()

		gotBody, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		if !bytes.Equal(gotBody, body) {
			t.Errorf("body = %q, want %q", gotBody, body)
		}
		if meta.Hash != put.Hash {
			t.Errorf("meta.Hash = %q, want %q", meta.Hash, put.Hash)
		}
		if meta.Version != 1 {
			t.Errorf("get version = %d, want 1", meta.Version)
		}
	})

	t.Run("Put_BumpsVersion", func(t *testing.T) {
		s := factory(t)
		ctx := context.Background()
		path := "blueprints/web.yaml"
		for i := 1; i <= 3; i++ {
			body := []byte{byte('a' + i)}
			m, err := s.Put(ctx, files.FileMetadata{Path: path}, bytes.NewReader(body))
			if err != nil {
				t.Fatalf("Put #%d: %v", i, err)
			}
			if m.Version != int64(i) {
				t.Errorf("Put #%d version = %d, want %d", i, m.Version, i)
			}
		}
		got, err := s.Stat(ctx, path)
		if err != nil {
			t.Fatalf("Stat: %v", err)
		}
		if got.Version != 3 {
			t.Errorf("final version = %d, want 3", got.Version)
		}
	})

	t.Run("Put_OverwritesBody", func(t *testing.T) {
		s := factory(t)
		ctx := context.Background()
		path := "secrets/api-token"
		_, _ = s.Put(ctx, files.FileMetadata{Path: path}, bytes.NewReader([]byte("v1")))
		_, _ = s.Put(ctx, files.FileMetadata{Path: path}, bytes.NewReader([]byte("v2-longer")))

		_, rc, err := s.Get(ctx, path)
		if err != nil {
			t.Fatalf("Get: %v", err)
		}
		defer func() { _ = rc.Close() }()
		got, _ := io.ReadAll(rc)
		if string(got) != "v2-longer" {
			t.Errorf("body = %q, want v2-longer", got)
		}
	})

	t.Run("Get_NotFound", func(t *testing.T) {
		s := factory(t)
		_, _, err := s.Get(context.Background(), "nope/missing.txt")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Stat_NotFound", func(t *testing.T) {
		s := factory(t)
		_, err := s.Stat(context.Background(), "nope/missing.txt")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Delete_RemovesFile", func(t *testing.T) {
		s := factory(t)
		ctx := context.Background()
		path := "tmp/file.txt"
		if _, err := s.Put(ctx, files.FileMetadata{Path: path}, bytes.NewReader([]byte("x"))); err != nil {
			t.Fatalf("Put: %v", err)
		}
		if err := s.Delete(ctx, path); err != nil {
			t.Fatalf("Delete: %v", err)
		}
		if _, err := s.Stat(ctx, path); !errors.Is(err, ErrNotFound) {
			t.Errorf("post-delete Stat err = %v, want ErrNotFound", err)
		}
	})

	t.Run("Delete_NotFound", func(t *testing.T) {
		s := factory(t)
		err := s.Delete(context.Background(), "never/existed")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("err = %v, want ErrNotFound", err)
		}
	})

	t.Run("List_Empty", func(t *testing.T) {
		s := factory(t)
		got, err := s.List(context.Background(), "")
		if err != nil {
			t.Fatalf("List: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("len = %d, want 0", len(got))
		}
	})

	t.Run("List_Prefix_Sorted", func(t *testing.T) {
		s := factory(t)
		ctx := context.Background()
		paths := []string{
			"configs/app/a.yaml",
			"configs/app/b.yaml",
			"configs/other/c.yaml",
			"blueprints/web.yaml",
		}
		for _, p := range paths {
			if _, err := s.Put(ctx, files.FileMetadata{Path: p}, bytes.NewReader([]byte("x"))); err != nil {
				t.Fatalf("Put %s: %v", p, err)
			}
		}

		all, err := s.List(ctx, "")
		if err != nil {
			t.Fatalf("List all: %v", err)
		}
		if len(all) != 4 {
			t.Errorf("len(all) = %d, want 4", len(all))
		}
		// sorted check
		wantAll := append([]string{}, paths...)
		sort.Strings(wantAll)
		for i, m := range all {
			if m.Path != wantAll[i] {
				t.Errorf("all[%d] = %s, want %s", i, m.Path, wantAll[i])
			}
		}

		appList, err := s.List(ctx, "configs/app/")
		if err != nil {
			t.Fatalf("List prefix: %v", err)
		}
		if len(appList) != 2 {
			t.Errorf("len(prefix) = %d, want 2: %+v", len(appList), appList)
		}
		for _, m := range appList {
			if m.Path != "configs/app/a.yaml" && m.Path != "configs/app/b.yaml" {
				t.Errorf("unexpected match: %s", m.Path)
			}
		}
	})

	t.Run("RejectsInvalidPath", func(t *testing.T) {
		s := factory(t)
		ctx := context.Background()
		bad := []string{"", "/leading", "trailing/", "with/../traverse", "double//slash", "with space"}
		for _, p := range bad {
			if _, err := s.Stat(ctx, p); err == nil || errors.Is(err, ErrNotFound) {
				t.Errorf("Stat(%q) accepted, want validation error (got %v)", p, err)
			}
			if _, _, err := s.Get(ctx, p); err == nil || errors.Is(err, ErrNotFound) {
				t.Errorf("Get(%q) accepted, want validation error (got %v)", p, err)
			}
			if err := s.Delete(ctx, p); err == nil || errors.Is(err, ErrNotFound) {
				t.Errorf("Delete(%q) accepted, want validation error (got %v)", p, err)
			}
			if _, err := s.Put(ctx, files.FileMetadata{Path: p}, bytes.NewReader(nil)); err == nil {
				t.Errorf("Put(%q) accepted, want validation error", p)
			}
		}
	})
}
