// SPDX-License-Identifier: Apache-2.0

package dest

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveSource(t *testing.T) {
	cfg := Config{AccessKey: "ak", SecretKey: "sk"}
	cases := []struct {
		name     string
		uri      string
		wantType string
		wantSub  string
	}{
		{name: "abs path", uri: "/tmp/foo.tar", wantType: "local"},
		{name: "rel path", uri: "./foo.tar", wantType: "local"},
		{name: "file scheme", uri: "file:///tmp/foo.tar", wantType: "local"},
		{name: "s3 scheme", uri: "s3://bucket/key.tar", wantType: "s3"},
		{name: "empty", uri: "", wantSub: "empty URI"},
		{name: "s3 missing key", uri: "s3://bucket", wantSub: "source key must not be empty"},
		{name: "s3 missing bucket", uri: "s3:///key", wantSub: "bucket must not be empty"},
		{name: "unsupported", uri: "https://example.com/foo", wantSub: "unsupported source scheme"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			s, err := ResolveSource(tc.uri, cfg)
			if tc.wantSub != "" {
				if err == nil {
					t.Fatalf("want error, got source=%#v", s)
				}
				if !strings.Contains(err.Error(), tc.wantSub) {
					t.Errorf("err = %v, want %q", err, tc.wantSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			switch tc.wantType {
			case "local":
				if _, ok := s.(*LocalSource); !ok {
					t.Errorf("want LocalSource, got %T", s)
				}
			case "s3":
				if _, ok := s.(*S3Source); !ok {
					t.Errorf("want S3Source, got %T", s)
				}
			}
		})
	}
}

func TestResolveLister(t *testing.T) {
	cfg := Config{AccessKey: "ak", SecretKey: "sk"}
	cases := []struct {
		name     string
		uri      string
		wantType string
		wantSub  string
	}{
		{name: "local dir", uri: "/var/backups", wantType: "local"},
		{name: "s3 prefix", uri: "s3://bucket/prefix/", wantType: "s3"},
		{name: "s3 bucket only", uri: "s3://bucket", wantType: "s3"}, // empty prefix is valid
		{name: "empty", uri: "", wantSub: "empty URI"},
		{name: "https unsupported", uri: "https://x/y", wantSub: "unsupported lister scheme"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			l, err := ResolveLister(tc.uri, cfg)
			if tc.wantSub != "" {
				if err == nil {
					t.Fatalf("want error, got %#v", l)
				}
				if !strings.Contains(err.Error(), tc.wantSub) {
					t.Errorf("err = %v, want %q", err, tc.wantSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			switch tc.wantType {
			case "local":
				if _, ok := l.(*LocalLister); !ok {
					t.Errorf("want LocalLister, got %T", l)
				}
			case "s3":
				if _, ok := l.(*S3Lister); !ok {
					t.Errorf("want S3Lister, got %T", l)
				}
			}
		})
	}
}

func TestLocalSource_Open(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.tar")
	want := []byte("tar-bytes")
	if err := os.WriteFile(path, want, 0o600); err != nil {
		t.Fatal(err)
	}
	s := &LocalSource{Path: path}
	rc, err := s.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = rc.Close() }()
	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestLocalSource_Open_Missing(t *testing.T) {
	s := &LocalSource{Path: filepath.Join(t.TempDir(), "nope.tar")}
	if _, err := s.Open(context.Background()); err == nil {
		t.Fatal("want error")
	}
}

func TestLocalSource_EmptyPath(t *testing.T) {
	s := &LocalSource{}
	if _, err := s.Open(context.Background()); err == nil {
		t.Fatal("want error")
	}
}

func TestLocalLister_List(t *testing.T) {
	dir := t.TempDir()
	// Create three .tar files (out-of-order), one non-tar, one subdir.
	for _, name := range []string{"c.tar", "a.tar", "b.tar"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o700); err != nil {
		t.Fatal(err)
	}

	l := &LocalLister{Dir: dir}
	entries, err := l.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("entries = %d, want 3 (ignored non-tar + subdir): %+v", len(entries), entries)
	}
	wantNames := []string{"a.tar", "b.tar", "c.tar"}
	for i, e := range entries {
		if e.Name != wantNames[i] {
			t.Errorf("[%d].Name = %q, want %q (sorted)", i, e.Name, wantNames[i])
		}
		if e.Size != 1 {
			t.Errorf("[%d].Size = %d, want 1", i, e.Size)
		}
		if e.LastModified.IsZero() {
			t.Errorf("[%d].LastModified is zero", i)
		}
		// Sanity: LastModified is recent.
		if time.Since(e.LastModified) > time.Minute {
			t.Errorf("[%d].LastModified = %v, want recent", i, e.LastModified)
		}
	}
}

func TestLocalLister_List_Missing(t *testing.T) {
	l := &LocalLister{Dir: filepath.Join(t.TempDir(), "nope")}
	_, err := l.List(context.Background())
	if err == nil {
		t.Fatal("want error")
	}
	if !strings.Contains(err.Error(), "does not exist") {
		t.Errorf("err = %v, want 'does not exist'", err)
	}
}

func TestLocalLister_List_EmptyDir(t *testing.T) {
	l := &LocalLister{Dir: t.TempDir()}
	entries, err := l.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("entries = %+v, want empty", entries)
	}
}

func TestLocalLister_EmptyDir_Field(t *testing.T) {
	l := &LocalLister{}
	if _, err := l.List(context.Background()); err == nil {
		t.Fatal("want error for empty Dir")
	}
}
