// SPDX-License-Identifier: Apache-2.0

package files

import (
	"strings"
	"testing"
)

func TestFileOperation_Valid(t *testing.T) {
	for _, op := range []FileOperation{FileOpPut, FileOpGet, FileOpList, FileOpDelete} {
		if !op.Valid() {
			t.Errorf("%q.Valid() = false, want true", op)
		}
	}
	for _, op := range []FileOperation{"", "create", "PUT", "fetch"} {
		if FileOperation(op).Valid() {
			t.Errorf("%q.Valid() = true, want false", op)
		}
	}
}

func TestFileMetadata_Validate(t *testing.T) {
	good := FileMetadata{
		Path: "configs/server.yaml",
		Size: 1024,
		Hash: strings.Repeat("a", 64),
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("baseline: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*FileMetadata)
		wantSub string
	}{
		{name: "empty path", mutate: func(m *FileMetadata) { m.Path = "" }, wantSub: "must not be empty"},
		{name: "leading slash", mutate: func(m *FileMetadata) { m.Path = "/foo/bar" }, wantSub: "must not start"},
		{name: "trailing slash", mutate: func(m *FileMetadata) { m.Path = "foo/bar/" }, wantSub: "must not end"},
		{name: "double slash", mutate: func(m *FileMetadata) { m.Path = "foo//bar" }, wantSub: "empty segments"},
		{name: "path traversal", mutate: func(m *FileMetadata) { m.Path = "foo/../etc/passwd" }, wantSub: "'..'"},
		{name: "whitespace in segment", mutate: func(m *FileMetadata) { m.Path = "foo/bar baz.yaml" }, wantSub: "whitespace"},
		{name: "wildcard >", mutate: func(m *FileMetadata) { m.Path = "foo/>" }, wantSub: "wildcard"},
		{name: "wildcard *", mutate: func(m *FileMetadata) { m.Path = "foo/*.yaml" }, wantSub: "wildcard"},
		{name: "negative size", mutate: func(m *FileMetadata) { m.Size = -1 }, wantSub: "size"},
		{name: "bad hash uppercase", mutate: func(m *FileMetadata) { m.Hash = strings.Repeat("A", 64) }, wantSub: "hash"},
		{name: "bad hash length", mutate: func(m *FileMetadata) { m.Hash = "abcd" }, wantSub: "hash"},
		{name: "negative version", mutate: func(m *FileMetadata) { m.Version = -5 }, wantSub: "version"},
		{name: "empty hash ok on put", mutate: func(m *FileMetadata) { m.Hash = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := good
			tc.mutate(&m)
			err := m.Validate()
			if tc.wantSub == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q, got nil", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestFileRequest_Validate(t *testing.T) {
	cases := []struct {
		name    string
		req     FileRequest
		wantSub string
	}{
		{name: "put ok", req: FileRequest{Operation: FileOpPut, Path: "a/b.yaml"}},
		{name: "put with metadata ok", req: FileRequest{
			Operation: FileOpPut,
			Path:      "a/b.yaml",
			Metadata:  &FileMetadata{Path: "a/b.yaml", Size: 5, Hash: strings.Repeat("a", 64)},
		}},
		{name: "get ok", req: FileRequest{Operation: FileOpGet, Path: "a/b.yaml"}},
		{name: "delete ok", req: FileRequest{Operation: FileOpDelete, Path: "a/b.yaml"}},
		{name: "list empty path ok", req: FileRequest{Operation: FileOpList}},
		{name: "list with prefix slash ok", req: FileRequest{Operation: FileOpList, Path: "a/"}},
		{name: "list with prefix ok", req: FileRequest{Operation: FileOpList, Path: "a/b"}},
		{name: "list with traversal", req: FileRequest{Operation: FileOpList, Path: "../etc"}, wantSub: "'..'"},
		{name: "list with wildcard", req: FileRequest{Operation: FileOpList, Path: "a/*"}, wantSub: "wildcard"},
		{name: "list with metadata", req: FileRequest{Operation: FileOpList, Metadata: &FileMetadata{Path: "x"}}, wantSub: "metadata"},
		{name: "list with body", req: FileRequest{Operation: FileOpList, Body: []byte("x")}, wantSub: "body"},
		{name: "invalid op", req: FileRequest{Operation: "create", Path: "x"}, wantSub: "invalid"},
		{name: "put without path", req: FileRequest{Operation: FileOpPut}, wantSub: "path"},
		{name: "metadata invalid", req: FileRequest{
			Operation: FileOpPut, Path: "a",
			Metadata: &FileMetadata{Path: "/bad"}, // leading slash
		}, wantSub: "metadata"},
		{name: "get with from_chunk ok", req: FileRequest{Operation: FileOpGet, Path: "a", FromChunk: 5}},
		{name: "get with from_chunk zero ok", req: FileRequest{Operation: FileOpGet, Path: "a", FromChunk: 0}},
		{name: "from_chunk negative", req: FileRequest{Operation: FileOpGet, Path: "a", FromChunk: -1}, wantSub: "from_chunk"},
		{name: "from_chunk on put", req: FileRequest{Operation: FileOpPut, Path: "a", FromChunk: 1}, wantSub: "from_chunk"},
		{name: "from_chunk on list", req: FileRequest{Operation: FileOpList, FromChunk: 1}, wantSub: "from_chunk"},
		{name: "from_chunk on delete", req: FileRequest{Operation: FileOpDelete, Path: "a", FromChunk: 1}, wantSub: "from_chunk"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.req.Validate()
			if tc.wantSub == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestFileChunk_Validate(t *testing.T) {
	body := []byte("chunk-body")
	good := FileChunk{
		ID:     "transfer-uuid-123",
		FileID: "configs/server.yaml",
		Index:  0,
		Total:  1,
		Data:   body,
		Hash:   HashOf(body),
	}
	if err := good.Validate(); err != nil {
		t.Fatalf("baseline: %v", err)
	}

	cases := []struct {
		name    string
		mutate  func(*FileChunk)
		wantSub string
	}{
		{name: "empty id", mutate: func(c *FileChunk) { c.ID = "" }, wantSub: "id"},
		{name: "bad file id", mutate: func(c *FileChunk) { c.FileID = "/abs" }, wantSub: "file_id"},
		{name: "zero total", mutate: func(c *FileChunk) { c.Total = 0 }, wantSub: "total"},
		{name: "negative total", mutate: func(c *FileChunk) { c.Total = -1 }, wantSub: "total"},
		{name: "index >= total", mutate: func(c *FileChunk) { c.Index = 1; c.Total = 1 }, wantSub: "out of range"},
		{name: "negative index", mutate: func(c *FileChunk) { c.Index = -1 }, wantSub: "out of range"},
		{name: "nil data", mutate: func(c *FileChunk) { c.Data = nil }, wantSub: "data"},
		{name: "oversized data", mutate: func(c *FileChunk) {
			c.Data = make([]byte, ChunkSize+1)
			c.Hash = HashOf(c.Data)
		}, wantSub: "exceeds"},
		{name: "hash format bad", mutate: func(c *FileChunk) { c.Hash = "not-hex" }, wantSub: "hash"},
		{name: "hash mismatch", mutate: func(c *FileChunk) { c.Hash = HashOf([]byte("different")) }, wantSub: "does not match"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := good
			c.Data = append([]byte{}, body...)
			tc.mutate(&c)
			err := c.Validate()
			if tc.wantSub == "" {
				if err != nil {
					t.Fatalf("want nil, got %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("want error containing %q", tc.wantSub)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestHashOf(t *testing.T) {
	// Empty SHA-256: the well-known constant.
	const emptyHex = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if got := HashOf(nil); got != emptyHex {
		t.Errorf("HashOf(nil) = %s, want %s", got, emptyHex)
	}
	if got := HashOf([]byte{}); got != emptyHex {
		t.Errorf("HashOf([]) = %s, want %s", got, emptyHex)
	}
	a := HashOf([]byte("a"))
	b := HashOf([]byte("a"))
	if a != b {
		t.Errorf("HashOf deterministic: %s vs %s", a, b)
	}
	if len(a) != 64 {
		t.Errorf("hash len = %d, want 64", len(a))
	}
}

func TestNamespace(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"configs/app/main.yaml", "configs"},
		{"blueprints/web.yaml", "blueprints"},
		{"single", "single"},
		{"", ""},
		{"a/b", "a"},
		{"long-namespace/x/y/z", "long-namespace"},
	}
	for _, tc := range cases {
		if got := Namespace(tc.path); got != tc.want {
			t.Errorf("Namespace(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

// TestChunkSize_Constant guards against an accidental change to the
// PROJECT-DETAILS §4.20-fixed 1 MiB chunk size.
func TestChunkSize_Constant(t *testing.T) {
	const want = 1 << 20
	if ChunkSize != want {
		t.Errorf("ChunkSize = %d, want %d", ChunkSize, want)
	}
}
