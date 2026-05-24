// SPDX-License-Identifier: Apache-2.0

package dest

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
)

// fakeS3Server mimics enough of the S3 multipart-upload protocol for
// minio-go's PutObject(size=-1) to complete. It records the body
// bytes received across one or more part PUTs so the test can assert
// the destination forwarded the writer payload faithfully.
type fakeS3Server struct {
	mu             sync.Mutex
	receivedBody   bytes.Buffer
	partsReceived  int
	failPart       bool
	failComplete   bool
	uploadIDIssued string
}

func newFakeS3Server(t *testing.T, failPart, failComplete bool) (*httptest.Server, *fakeS3Server) {
	t.Helper()
	state := &fakeS3Server{
		uploadIDIssued: "test-upload-id",
		failPart:       failPart,
		failComplete:   failComplete,
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.handle(t, w, r)
	}))
	return srv, state
}

func (s *fakeS3Server) handle(t *testing.T, w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	switch {
	// 1. Initiate multipart upload: POST /bucket/key?uploads=
	case r.Method == http.MethodPost && q.Has("uploads"):
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(
			`<?xml version="1.0" encoding="UTF-8"?>` +
				`<InitiateMultipartUploadResult>` +
				`<Bucket>test-bucket</Bucket>` +
				`<Key>test-key.tar</Key>` +
				`<UploadId>` + s.uploadIDIssued + `</UploadId>` +
				`</InitiateMultipartUploadResult>`,
		))

	// 2. Upload part: PUT /bucket/key?partNumber=1&uploadId=...
	case r.Method == http.MethodPut && q.Has("partNumber") && q.Has("uploadId"):
		s.mu.Lock()
		s.partsReceived++
		s.mu.Unlock()
		if s.failPart {
			http.Error(w, "InternalError", http.StatusInternalServerError)
			return
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("part read: %v", err)
		}
		// minio-go wraps unknown-size streams in AWS chunked
		// transfer encoding (STREAMING-AWS4-HMAC-SHA256-PAYLOAD).
		// Detect via Content-Encoding / x-amz-decoded-content-length
		// header and strip the chunk framing.
		if r.Header.Get("Content-Encoding") == "aws-chunked" ||
			r.Header.Get("X-Amz-Decoded-Content-Length") != "" {
			body = decodeAWSChunked(t, body)
		}
		s.mu.Lock()
		s.receivedBody.Write(body)
		s.mu.Unlock()
		w.Header().Set("ETag", `"abc123"`)
		w.WriteHeader(http.StatusOK)

	// 3. Complete multipart upload: POST /bucket/key?uploadId=...
	case r.Method == http.MethodPost && q.Has("uploadId"):
		if s.failComplete {
			http.Error(w, "InternalError", http.StatusInternalServerError)
			return
		}
		// Drain client's CompleteMultipartUpload XML body.
		_, _ = io.Copy(io.Discard, r.Body)
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(
			`<?xml version="1.0" encoding="UTF-8"?>` +
				`<CompleteMultipartUploadResult>` +
				`<Location>http://test/test-bucket/test-key.tar</Location>` +
				`<Bucket>test-bucket</Bucket>` +
				`<Key>test-key.tar</Key>` +
				`<ETag>"final-etag"</ETag>` +
				`</CompleteMultipartUploadResult>`,
		))

	default:
		t.Logf("fakeS3: unexpected %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
		http.Error(w, "unexpected", http.StatusBadRequest)
	}
}

func newS3Destination(t *testing.T, srv *httptest.Server) *S3Destination {
	t.Helper()
	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse server URL: %v", err)
	}
	return &S3Destination{
		Bucket: "test-bucket",
		Key:    "test-key.tar",
		Config: Config{
			AccessKey: "test-ak",
			SecretKey: "test-sk",
			Region:    "us-east-1",
			Endpoint:  u.Host,
			UseSSL:    false,
		},
	}
}

// decodeAWSChunked strips AWS-chunked framing from a payload that
// looks like `<hex-size>;chunk-signature=...\r\n<bytes>\r\n` repeated
// until a zero-length size marker. We do not verify chunk signatures
// — the test asserts what the destination forwarded, not the signing.
func decodeAWSChunked(t *testing.T, body []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	pos := 0
	for pos < len(body) {
		// Find end of size+signature line.
		eol := bytes.Index(body[pos:], []byte("\r\n"))
		if eol < 0 {
			t.Fatalf("aws-chunked: missing size CRLF at pos %d", pos)
		}
		header := body[pos : pos+eol]
		pos += eol + 2

		// Strip chunk-signature=...
		semi := bytes.IndexByte(header, ';')
		if semi >= 0 {
			header = header[:semi]
		}
		var size int
		if _, err := fmtSscanfHex(string(header), &size); err != nil {
			t.Fatalf("aws-chunked: parse size %q: %v", header, err)
		}
		if size == 0 {
			break
		}
		if pos+size > len(body) {
			t.Fatalf("aws-chunked: chunk size %d > remaining %d", size, len(body)-pos)
		}
		out.Write(body[pos : pos+size])
		pos += size
		// Skip trailing CRLF.
		if pos+2 <= len(body) {
			pos += 2
		}
	}
	return out.Bytes()
}

// fmtSscanfHex parses a hex string into n. Inlined to keep the
// dependency surface in this test file small (no extra fmt import).
func fmtSscanfHex(s string, n *int) (int, error) {
	v, err := parseHex(s)
	if err != nil {
		return 0, err
	}
	*n = v
	return 1, nil
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

func TestS3Destination_RoundTrip(t *testing.T) {
	srv, state := newFakeS3Server(t, false, false)
	defer srv.Close()

	d := newS3Destination(t, srv)
	wc, err := d.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	payload := []byte("kscore-backup-payload")
	if _, err := wc.Write(payload); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	state.mu.Lock()
	got := state.receivedBody.Bytes()
	state.mu.Unlock()
	if !bytes.Equal(got, payload) {
		t.Errorf("server got %q, want %q", got, payload)
	}
	if state.partsReceived < 1 {
		t.Errorf("partsReceived = %d, want >=1", state.partsReceived)
	}
}

func TestS3Destination_PartUploadError(t *testing.T) {
	srv, _ := newFakeS3Server(t, true, false)
	defer srv.Close()

	d := newS3Destination(t, srv)
	wc, err := d.Open(context.Background())
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	// Write may succeed or fail depending on when minio-go observes
	// the 500 — but Close MUST surface the error.
	_, _ = wc.Write([]byte("payload"))
	err = wc.Close()
	if err == nil {
		t.Fatal("Close: want error from failed part upload")
	}
}

func TestS3Destination_CtxCancellation(t *testing.T) {
	srv, _ := newFakeS3Server(t, false, false)
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	d := newS3Destination(t, srv)

	wc, err := d.Open(ctx)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}

	cancel()
	// Best-effort write; the goroutine may have already aborted.
	_, _ = wc.Write([]byte("payload"))

	err = wc.Close()
	if err == nil {
		t.Fatal("Close: want error after ctx cancel")
	}
	if !errors.Is(err, context.Canceled) && !strings.Contains(err.Error(), "context canceled") {
		t.Logf("Close err = %v (accepted any post-cancel error)", err)
	}
}

func TestS3Destination_MissingCreds(t *testing.T) {
	d := &S3Destination{
		Bucket: "b",
		Key:    "k",
		Config: Config{Endpoint: "localhost:9000"}, // no AccessKey/SecretKey
	}
	if _, err := d.Open(context.Background()); err == nil {
		t.Fatal("want error for missing creds")
	}
}
