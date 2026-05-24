// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/s3client"
)

// fakeS3 is the in-memory bucket the httptest server reflects.
type fakeS3 struct {
	mu      sync.Mutex
	objects map[string][]byte
}

func newFakeS3() *fakeS3 {
	return &fakeS3{objects: make(map[string][]byte)}
}

func (s *fakeS3) handler(t *testing.T) http.HandlerFunc {
	t.Helper()
	return func(w http.ResponseWriter, r *http.Request) {
		// minio-go addresses objects as /<bucket>/<key...>; the
		// bucket name precedes the key.
		path := strings.TrimPrefix(r.URL.Path, "/")
		slash := strings.Index(path, "/")

		switch {
		case r.Method == http.MethodGet && r.URL.Query().Get("list-type") == "2":
			s.handleList(w, r)

		case r.Method == http.MethodPut && slash > 0:
			s.handlePut(w, r, path[slash+1:])

		case r.Method == http.MethodGet && slash > 0:
			s.handleGet(w, path[slash+1:])

		case r.Method == http.MethodHead && slash > 0:
			s.handleHead(w, path[slash+1:])

		case r.Method == http.MethodDelete && slash > 0:
			s.handleDelete(w, path[slash+1:])

		default:
			t.Logf("fakeS3: unexpected %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}
}

func (s *fakeS3) handlePut(w http.ResponseWriter, r *http.Request, key string) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// minio-go wraps PUT bodies in AWS chunked transfer encoding
	// (STREAMING-AWS4-HMAC-SHA256-PAYLOAD) even for known-size
	// single-PUT objects. Detect via the Content-Encoding /
	// X-Amz-Decoded-Content-Length headers and strip the framing.
	if r.Header.Get("Content-Encoding") == "aws-chunked" ||
		r.Header.Get("X-Amz-Decoded-Content-Length") != "" {
		body, err = decodeAWSChunked(body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	}
	s.mu.Lock()
	s.objects[key] = body
	s.mu.Unlock()
	w.Header().Set("ETag", `"deadbeef"`)
	w.WriteHeader(http.StatusOK)
}

// decodeAWSChunked strips AWS-chunked framing from a payload that
// looks like `<hex-size>;chunk-signature=...\r\n<bytes>\r\n`
// repeated until a zero-length size marker. Chunk signatures are
// not verified — the test asserts what the backend forwarded, not
// the signing. Adapted from internal/backup/dest/s3_test.go.
func decodeAWSChunked(body []byte) ([]byte, error) {
	var out bytes.Buffer
	pos := 0
	for pos < len(body) {
		eol := bytes.Index(body[pos:], []byte("\r\n"))
		if eol < 0 {
			return nil, fmt.Errorf("aws-chunked: missing size CRLF at pos %d", pos)
		}
		header := body[pos : pos+eol]
		pos += eol + 2

		if semi := bytes.IndexByte(header, ';'); semi >= 0 {
			header = header[:semi]
		}
		size, err := parseHex(string(header))
		if err != nil {
			return nil, fmt.Errorf("aws-chunked: parse size %q: %w", header, err)
		}
		if size == 0 {
			break
		}
		if pos+size > len(body) {
			return nil, fmt.Errorf("aws-chunked: chunk size %d > remaining %d", size, len(body)-pos)
		}
		out.Write(body[pos : pos+size])
		pos += size
		if pos+2 <= len(body) {
			pos += 2
		}
	}
	return out.Bytes(), nil
}

func parseHex(s string) (int, error) {
	v := 0
	for _, c := range s {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= int(c - '0')
		case c >= 'a' && c <= 'f':
			v |= int(c-'a') + 10
		case c >= 'A' && c <= 'F':
			v |= int(c-'A') + 10
		default:
			return 0, errors.New("not a hex char")
		}
	}
	return v, nil
}

func (s *fakeS3) handleGet(w http.ResponseWriter, key string) {
	s.mu.Lock()
	body, ok := s.objects[key]
	s.mu.Unlock()
	if !ok {
		writeS3NotFound(w)
		return
	}
	writeObjectHeaders(w, len(body))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(body)
}

func (s *fakeS3) handleHead(w http.ResponseWriter, key string) {
	s.mu.Lock()
	body, ok := s.objects[key]
	s.mu.Unlock()
	if !ok {
		writeS3NotFound(w)
		return
	}
	writeObjectHeaders(w, len(body))
	w.WriteHeader(http.StatusOK)
}

func writeObjectHeaders(w http.ResponseWriter, size int) {
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))
	w.Header().Set("ETag", `"deadbeef"`)
	w.Header().Set("Last-Modified", "Wed, 21 May 2026 12:00:00 GMT")
}

func (s *fakeS3) handleDelete(w http.ResponseWriter, key string) {
	s.mu.Lock()
	delete(s.objects, key)
	s.mu.Unlock()
	w.WriteHeader(http.StatusNoContent)
}

type listBucketResult struct {
	XMLName     xml.Name        `xml:"ListBucketResult"`
	Xmlns       string          `xml:"xmlns,attr"`
	Name        string          `xml:"Name"`
	KeyCount    int             `xml:"KeyCount"`
	IsTruncated bool            `xml:"IsTruncated"`
	Contents    []listEntryXML  `xml:"Contents"`
}

type listEntryXML struct {
	Key          string `xml:"Key"`
	Size         int    `xml:"Size"`
	LastModified string `xml:"LastModified"`
	ETag         string `xml:"ETag"`
}

func (s *fakeS3) handleList(w http.ResponseWriter, r *http.Request) {
	prefix := r.URL.Query().Get("prefix")
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0)
	for k := range s.objects {
		if prefix == "" || strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	result := listBucketResult{
		Xmlns:       "http://s3.amazonaws.com/doc/2006-03-01/",
		Name:        "test-bucket",
		KeyCount:    len(keys),
		IsTruncated: false,
	}
	for _, k := range keys {
		result.Contents = append(result.Contents, listEntryXML{
			Key:          k,
			Size:         len(s.objects[k]),
			LastModified: "2026-05-21T12:00:00.000Z",
			ETag:         `"deadbeef"`,
		})
	}
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(xml.Header))
	_ = xml.NewEncoder(w).Encode(result)
}

func writeS3NotFound(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(http.StatusNotFound)
	_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<Error><Code>NoSuchKey</Code><Message>not found</Message></Error>`))
}

func cfgForS3(t *testing.T, srv *httptest.Server) s3client.Config {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return s3client.Config{
		AccessKey: "ak",
		SecretKey: "sk",
		Region:    "us-east-1",
		Endpoint:  u.Host,
		UseSSL:    false,
	}
}

func TestS3Store_Conformance(t *testing.T) {
	runConformance(t, func(t *testing.T) Store {
		fake := newFakeS3()
		srv := httptest.NewServer(fake.handler(t))
		t.Cleanup(srv.Close)
		s, err := NewS3Store("test-bucket", "files/", cfgForS3(t, srv), nil)
		if err != nil {
			t.Fatalf("NewS3Store: %v", err)
		}
		return s
	})
}

func TestNewS3Store_Validation(t *testing.T) {
	if _, err := NewS3Store("", "", s3client.Config{}, nil); err == nil {
		t.Fatal("want error for empty bucket")
	}
}

func TestNewS3Store_PrefixNormalisation(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"files", "files/"},
		{"files/", "files/"},
		{"/files/", "files/"},
		{"/files", "files/"},
	}
	for _, tc := range tests {
		s, err := NewS3Store("b", tc.in, s3client.Config{}, nil)
		if err != nil {
			t.Fatalf("NewS3Store(%q): %v", tc.in, err)
		}
		if s.prefix != tc.want {
			t.Errorf("prefix(%q) = %q, want %q", tc.in, s.prefix, tc.want)
		}
	}
}

func TestS3Store_KeyLayout(t *testing.T) {
	// Verify the documented data/meta object-key split.
	fake := newFakeS3()
	srv := httptest.NewServer(fake.handler(t))
	defer srv.Close()

	s, err := NewS3Store("test-bucket", "files/", cfgForS3(t, srv), nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Put(context.Background(),
		files.FileMetadata{Path: "configs/app.yaml"},
		strings.NewReader("hi")); err != nil {
		t.Fatalf("Put: %v", err)
	}
	fake.mu.Lock()
	defer fake.mu.Unlock()
	if _, ok := fake.objects["files/data/configs/app.yaml"]; !ok {
		t.Errorf("missing body key: %+v", keysOf(fake.objects))
	}
	if _, ok := fake.objects["files/meta/configs/app.yaml.json"]; !ok {
		t.Errorf("missing meta key: %+v", keysOf(fake.objects))
	}
}

func TestS3Store_MissingCreds(t *testing.T) {
	s, err := NewS3Store("b", "", s3client.Config{Endpoint: "x"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Stat(context.Background(), "any"); err == nil {
		t.Error("want error for missing creds")
	}
}

func keysOf(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
