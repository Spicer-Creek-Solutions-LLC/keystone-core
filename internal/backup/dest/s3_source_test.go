// SPDX-License-Identifier: Apache-2.0

package dest

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// newFakeS3ReadServer mimics enough of the S3 API for GetObject +
// ListObjectsV2 to succeed against an in-process server.
func newFakeS3ReadServer(t *testing.T, body []byte, listKeys []string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		// ListObjectsV2: GET /bucket/?list-type=2&prefix=...
		case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
			w.Header().Set("Content-Type", "application/xml")
			var b strings.Builder
			b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>`)
			b.WriteString(`<ListBucketResult xmlns="http://s3.amazonaws.com/doc/2006-03-01/">`)
			b.WriteString(`<Name>test-bucket</Name>`)
			b.WriteString(`<KeyCount>` + fmt.Sprintf("%d", len(listKeys)) + `</KeyCount>`)
			b.WriteString(`<IsTruncated>false</IsTruncated>`)
			for _, k := range listKeys {
				b.WriteString(`<Contents>`)
				b.WriteString(`<Key>` + k + `</Key>`)
				b.WriteString(`<Size>42</Size>`)
				b.WriteString(`<LastModified>2026-05-20T19:30:00.000Z</LastModified>`)
				b.WriteString(`<ETag>"abc"</ETag>`)
				b.WriteString(`</Contents>`)
			}
			b.WriteString(`</ListBucketResult>`)
			_, _ = w.Write([]byte(b.String()))

		// GetObject: GET /bucket/key (no list-type)
		case r.Method == http.MethodGet:
			w.Header().Set("Content-Type", "application/x-tar")
			w.Header().Set("Content-Length", fmt.Sprintf("%d", len(body)))
			w.Header().Set("ETag", `"test-etag"`)
			w.Header().Set("Last-Modified", "Wed, 20 May 2026 19:30:00 GMT")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(body)

		default:
			t.Logf("fakeS3 read: unexpected %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
}

func cfgFor(t *testing.T, srv *httptest.Server) Config {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return Config{
		AccessKey: "ak",
		SecretKey: "sk",
		Region:    "us-east-1",
		Endpoint:  u.Host,
		UseSSL:    false,
	}
}

func TestS3Source_Open(t *testing.T) {
	payload := []byte("artifact-bytes")
	srv := newFakeS3ReadServer(t, payload, nil)
	defer srv.Close()

	s := &S3Source{
		Bucket: "test-bucket",
		Key:    "test-key.tar",
		Config: cfgFor(t, srv),
	}
	rc, err := s.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer func() { _ = rc.Close() }()

	got, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("ReadAll: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("got %q, want %q", got, payload)
	}
}

func TestS3Source_MissingCreds(t *testing.T) {
	s := &S3Source{
		Bucket: "b",
		Key:    "k",
		Config: Config{Endpoint: "localhost:9000"},
	}
	if _, err := s.Open(context.Background()); err == nil {
		t.Fatal("want error for missing creds")
	}
}

func TestS3Lister_List(t *testing.T) {
	srv := newFakeS3ReadServer(t, nil, []string{
		"prefix/2026-05-20-001.tar",
		"prefix/2026-05-20-002.tar",
	})
	defer srv.Close()

	l := &S3Lister{
		Bucket: "test-bucket",
		Prefix: "prefix/",
		Config: cfgFor(t, srv),
	}
	entries, err := l.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("entries = %d, want 2: %+v", len(entries), entries)
	}
	wantNames := []string{
		"prefix/2026-05-20-001.tar",
		"prefix/2026-05-20-002.tar",
	}
	for i, e := range entries {
		if e.Name != wantNames[i] {
			t.Errorf("[%d].Name = %q, want %q", i, e.Name, wantNames[i])
		}
		if e.Size != 42 {
			t.Errorf("[%d].Size = %d, want 42", i, e.Size)
		}
		if e.LastModified.IsZero() {
			t.Errorf("[%d].LastModified is zero", i)
		}
	}
}

func TestS3Lister_MissingCreds(t *testing.T) {
	l := &S3Lister{
		Bucket: "b",
		Prefix: "p",
		Config: Config{Endpoint: "localhost:9000"},
	}
	if _, err := l.List(context.Background()); err == nil {
		t.Fatal("want error for missing creds")
	}
}
