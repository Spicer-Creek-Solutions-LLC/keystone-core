package backend

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/files"
)

func TestMemoryStore_Conformance(t *testing.T) {
	runConformance(t, func(_ *testing.T) Store {
		return NewMemoryStore(nil)
	})
}

func TestMemoryStore_DeterministicTime(t *testing.T) {
	fixed := time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	s := NewMemoryStore(func() time.Time { return fixed })
	m, err := s.Put(context.Background(), files.FileMetadata{Path: "x"}, bytes.NewReader([]byte("z")))
	if err != nil {
		t.Fatal(err)
	}
	if !m.CreatedAt.Equal(fixed) {
		t.Errorf("CreatedAt = %v, want %v", m.CreatedAt, fixed)
	}
}

func TestMemoryStore_GetCopyIsIndependent(t *testing.T) {
	// Mutating the slice returned by Get must not corrupt the
	// stored body for later reads.
	s := NewMemoryStore(nil)
	ctx := context.Background()
	original := []byte("abc")
	if _, err := s.Put(ctx, files.FileMetadata{Path: "x"}, bytes.NewReader(original)); err != nil {
		t.Fatal(err)
	}
	_, rc, err := s.Get(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	first, _ := io.ReadAll(rc)
	_ = rc.Close()
	first[0] = 'Z'

	_, rc2, err := s.Get(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	second, _ := io.ReadAll(rc2)
	_ = rc2.Close()
	if string(second) != "abc" {
		t.Errorf("post-mutation body = %q, want %q", second, "abc")
	}
}

func TestMemoryStore_TagIsolation(t *testing.T) {
	// Mutating the tags map returned by Stat must not corrupt the
	// stored copy.
	s := NewMemoryStore(nil)
	ctx := context.Background()
	tags := map[string]string{"env": "prod"}
	if _, err := s.Put(ctx, files.FileMetadata{Path: "x", Tags: tags}, bytes.NewReader([]byte("z"))); err != nil {
		t.Fatal(err)
	}
	tags["env"] = "dev" // input mutation must not bleed through

	got, err := s.Stat(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	if got.Tags["env"] != "prod" {
		t.Errorf("input-mutation leak: tags = %+v", got.Tags)
	}

	got.Tags["env"] = "staging" // output mutation must not bleed back

	again, _ := s.Stat(ctx, "x")
	if again.Tags["env"] != "prod" {
		t.Errorf("output-mutation leak: tags = %+v", again.Tags)
	}
}

func TestMemoryStore_PutValidationErrors(t *testing.T) {
	s := NewMemoryStore(nil)
	_, err := s.Put(context.Background(), files.FileMetadata{Path: ""}, bytes.NewReader(nil))
	if err == nil {
		t.Fatal("want error for empty path")
	}
}

func TestMemoryStore_PutBodyReadError(t *testing.T) {
	s := NewMemoryStore(nil)
	_, err := s.Put(context.Background(), files.FileMetadata{Path: "x"}, errReader{})
	if !errors.Is(err, errBoom) {
		t.Errorf("err = %v, want errBoom", err)
	}
}

var errBoom = errors.New("boom")

type errReader struct{}

func (errReader) Read([]byte) (int, error) { return 0, errBoom }
