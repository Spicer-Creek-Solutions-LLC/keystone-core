package backend

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Verify interface compliance.
var _ Backend = (*HTTPBackend)(nil)

func newTestHTTPServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()

	mux.HandleFunc("/files/hello.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Header().Set("Content-Length", "13")
		w.Header().Set("ETag", `"abc123"`)
		w.Header().Set("Last-Modified", "Mon, 02 Jan 2006 15:04:05 GMT")

		if r.Header.Get("If-None-Match") == `"abc123"` {
			w.WriteHeader(http.StatusNotModified)
			return
		}

		if r.Method == http.MethodHead {
			return
		}

		// Handle range request
		if rng := r.Header.Get("Range"); rng != "" {
			w.Header().Set("Content-Range", "bytes 0-4/13")
			w.WriteHeader(http.StatusPartialContent)
			w.Write([]byte("Hello"))
			return
		}

		w.Write([]byte("Hello, World!"))
	})

	mux.HandleFunc("/files/secret.txt", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})

	mux.HandleFunc("/files/config.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-yaml")
		w.Header().Set("Content-Length", "10")
		if r.Method == http.MethodHead {
			return
		}
		w.Write([]byte("key: value"))
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	return httptest.NewServer(mux)
}

func TestNewHTTPBackend(t *testing.T) {
	tests := []struct {
		name    string
		config  *HTTPConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &HTTPConfig{
				Config:  Config{Name: "test-http", Type: TypeHTTP},
				BaseURL: "https://example.com/files",
			},
		},
		{
			name: "missing base_url",
			config: &HTTPConfig{
				Config: Config{Name: "test-http"},
			},
			wantErr: true,
		},
		{
			name: "invalid url",
			config: &HTTPConfig{
				BaseURL: "://bad",
			},
			wantErr: true,
		},
		{
			name: "non-http scheme",
			config: &HTTPConfig{
				BaseURL: "ftp://example.com/files",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, err := NewHTTPBackend(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if b == nil {
				t.Fatal("expected backend, got nil")
			}
			if !b.config.ReadOnly {
				t.Error("expected ReadOnly to be true")
			}
		})
	}
}

func TestHTTPBackend_NameAndType(t *testing.T) {
	b, err := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "my-http", Type: TypeHTTP},
		BaseURL: "https://example.com",
	})
	if err != nil {
		t.Fatal(err)
	}
	if b.Name() != "my-http" {
		t.Errorf("Name() = %q, want my-http", b.Name())
	}
	if b.Type() != TypeHTTP {
		t.Errorf("Type() = %q, want %q", b.Type(), TypeHTTP)
	}
	if b.BaseConfig().Name != "my-http" {
		t.Errorf("BaseConfig().Name = %q, want my-http", b.BaseConfig().Name)
	}
}

func TestHTTPBackend_Get(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, err := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test", Type: TypeHTTP},
		BaseURL: srv.URL + "/files",
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx := context.Background()

	result, err := b.Get(ctx, "/hello.txt", nil)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer result.Reader.Close()

	data, _ := io.ReadAll(result.Reader)
	if string(data) != "Hello, World!" {
		t.Errorf("body = %q, want Hello, World!", string(data))
	}
	if result.Info.ETag != `"abc123"` {
		t.Errorf("ETag = %q, want \"abc123\"", result.Info.ETag)
	}
	if result.Info.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", result.Info.ContentType)
	}
	if result.Info.Name != "hello.txt" {
		t.Errorf("Name = %q, want hello.txt", result.Info.Name)
	}
	if result.Info.ModifiedTime.IsZero() {
		t.Error("expected non-zero ModifiedTime")
	}
}

func TestHTTPBackend_Get_NotFound(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	_, err := b.Get(context.Background(), "/nonexistent.txt", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestHTTPBackend_Get_Forbidden(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	_, err := b.Get(context.Background(), "/secret.txt", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("expected access denied, got: %v", err)
	}
}

func TestHTTPBackend_Get_ConditionalMatch(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	result, err := b.Get(context.Background(), "/hello.txt", &GetOptions{
		IfNoneMatch: `"abc123"`,
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if !result.NotModified {
		t.Error("expected NotModified=true")
	}
}

func TestHTTPBackend_Get_Range(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	result, err := b.Get(context.Background(), "/hello.txt", &GetOptions{
		Range: &ByteRange{Start: 0, End: 5},
	})
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer result.Reader.Close()

	data, _ := io.ReadAll(result.Reader)
	if string(data) != "Hello" {
		t.Errorf("body = %q, want Hello", string(data))
	}
}

func TestHTTPBackend_Put_ReadOnly(t *testing.T) {
	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: "https://example.com",
	})

	_, err := b.Put(context.Background(), "/test.txt", bytes.NewReader([]byte("test")), nil)
	if err == nil {
		t.Error("expected error for Put on HTTP backend")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("expected read-only error, got: %v", err)
	}
}

func TestHTTPBackend_Delete_ReadOnly(t *testing.T) {
	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: "https://example.com",
	})

	err := b.Delete(context.Background(), "/test.txt")
	if err == nil {
		t.Error("expected error for Delete on HTTP backend")
	}
	if !strings.Contains(err.Error(), "read-only") {
		t.Errorf("expected read-only error, got: %v", err)
	}
}

func TestHTTPBackend_Exists(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	ctx := context.Background()

	exists, err := b.Exists(ctx, "/hello.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if !exists {
		t.Error("expected hello.txt to exist")
	}

	exists, err = b.Exists(ctx, "/nonexistent.txt")
	if err != nil {
		t.Fatalf("Exists failed: %v", err)
	}
	if exists {
		t.Error("expected nonexistent.txt to not exist")
	}
}

func TestHTTPBackend_Exists_Forbidden(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	_, err := b.Exists(context.Background(), "/secret.txt")
	if err == nil {
		t.Fatal("expected error for forbidden file")
	}
	if !strings.Contains(err.Error(), "access denied") {
		t.Errorf("expected access denied, got: %v", err)
	}
}

func TestHTTPBackend_Stat(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	info, err := b.Stat(context.Background(), "/hello.txt")
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}
	if info.Name != "hello.txt" {
		t.Errorf("Name = %q, want hello.txt", info.Name)
	}
	if info.Size != 13 {
		t.Errorf("Size = %d, want 13", info.Size)
	}
	if info.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want text/plain", info.ContentType)
	}
	if info.ETag != `"abc123"` {
		t.Errorf("ETag = %q, want \"abc123\"", info.ETag)
	}
}

func TestHTTPBackend_Stat_NotFound(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	_, err := b.Stat(context.Background(), "/nonexistent.txt")
	if err == nil {
		t.Fatal("expected error")
	}
	if !IsNotFound(err) {
		t.Errorf("expected not-found error, got: %v", err)
	}
}

func TestHTTPBackend_List_Unsupported(t *testing.T) {
	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: "https://example.com",
	})

	_, err := b.List(context.Background(), "/", nil)
	if err == nil {
		t.Error("expected error for List on HTTP backend")
	}
	if !strings.Contains(err.Error(), "not supported") {
		t.Errorf("expected 'not supported' error, got: %v", err)
	}
}

func TestHTTPBackend_Health(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL,
	})

	err := b.Health(context.Background())
	if err != nil {
		t.Errorf("Health failed: %v", err)
	}
}

func TestHTTPBackend_Health_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL,
	})

	err := b.Health(context.Background())
	if err == nil {
		t.Error("expected error for unhealthy server")
	}
}

func TestHTTPBackend_Close(t *testing.T) {
	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: "https://example.com",
	})

	if err := b.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestHTTPBackend_BasicAuth(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:   Config{Name: "test"},
		BaseURL:  srv.URL,
		Username: "admin",
		Password: "secret",
	})

	b.Health(context.Background())

	if gotAuth == "" {
		t.Error("expected Authorization header")
	}
	if !strings.HasPrefix(gotAuth, "Basic ") {
		t.Errorf("expected Basic auth, got: %q", gotAuth)
	}
}

func TestHTTPBackend_BearerToken(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:      Config{Name: "test"},
		BaseURL:     srv.URL,
		BearerToken: "mytoken123",
	})

	b.Health(context.Background())

	if gotAuth != "Bearer mytoken123" {
		t.Errorf("Authorization = %q, want 'Bearer mytoken123'", gotAuth)
	}
}

func TestHTTPBackend_CustomHeaders(t *testing.T) {
	var gotHeader string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Get("X-Custom")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL,
		Headers: map[string]string{"X-Custom": "test-value"},
	})

	b.Health(context.Background())

	if gotHeader != "test-value" {
		t.Errorf("X-Custom = %q, want test-value", gotHeader)
	}
}

func TestHTTPBackend_GetWithChecksum(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	result, err := b.GetWithChecksum(context.Background(), "/hello.txt")
	if err != nil {
		t.Fatalf("GetWithChecksum failed: %v", err)
	}
	defer result.Reader.Close()

	if result.Info.Checksum == "" {
		t.Error("expected non-empty checksum")
	}
	if result.Info.Size != 13 {
		t.Errorf("Size = %d, want 13", result.Info.Size)
	}

	data, _ := io.ReadAll(result.Reader)
	if string(data) != "Hello, World!" {
		t.Errorf("body = %q, want Hello, World!", string(data))
	}
}

func TestHTTPBackend_ContextCanceled(t *testing.T) {
	srv := newTestHTTPServer(t)
	defer srv.Close()

	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: srv.URL + "/files",
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := b.Get(ctx, "/hello.txt", nil)
	if err == nil {
		t.Error("expected error for canceled context")
	}
}

func TestHTTPBackend_TrailingSlashNormalized(t *testing.T) {
	b, _ := NewHTTPBackend(&HTTPConfig{
		Config:  Config{Name: "test"},
		BaseURL: "https://example.com/files/",
	})
	got := b.fileURL("/test.txt")
	if got != "https://example.com/files/test.txt" {
		t.Errorf("fileURL = %q, want https://example.com/files/test.txt", got)
	}
}
