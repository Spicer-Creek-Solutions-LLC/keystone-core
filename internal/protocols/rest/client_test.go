package rest

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()

	if cfg.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", cfg.Timeout)
	}
	if cfg.FollowRedirects != true {
		t.Error("expected FollowRedirects to be true")
	}
	if cfg.MaxRedirects != 10 {
		t.Errorf("MaxRedirects = %d, want 10", cfg.MaxRedirects)
	}
	if cfg.ValidateSSL != true {
		t.Error("expected ValidateSSL to be true")
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("MaxRetries = %d, want 3", cfg.MaxRetries)
	}
	if cfg.RetryDelay != time.Second {
		t.Errorf("RetryDelay = %v, want 1s", cfg.RetryDelay)
	}
	if len(cfg.RetryOnStatus) != 4 {
		t.Errorf("RetryOnStatus has %d codes, want 4", len(cfg.RetryOnStatus))
	}
}

func TestNewClient(t *testing.T) {
	tests := []struct {
		name   string
		config *ClientConfig
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
		},
		{
			name: "custom config",
			config: &ClientConfig{
				BaseURL: "https://example.com",
				Timeout: 60 * time.Second,
			},
		},
		{
			name: "with proxy",
			config: &ClientConfig{
				ProxyURL: "http://proxy.example.com:8080",
			},
		},
		{
			name: "invalid base URL",
			config: &ClientConfig{
				BaseURL: "://invalid",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(tt.config)
			if client == nil {
				t.Fatal("expected client to be created")
			}
			if client.httpClient == nil {
				t.Error("expected httpClient to be set")
			}
		})
	}
}

func TestClientNewRequest(t *testing.T) {
	tests := []struct {
		name    string
		baseURL string
		method  string
		path    string
		wantURL string
	}{
		{
			name:    "with base URL",
			baseURL: "https://example.com/api",
			method:  "GET",
			path:    "/users",
			wantURL: "https://example.com/api/users",
		},
		{
			name:    "absolute path",
			baseURL: "https://example.com",
			method:  "GET",
			path:    "https://other.com/test",
			wantURL: "https://other.com/test",
		},
		{
			name:    "no base URL",
			baseURL: "",
			method:  "GET",
			path:    "/test",
			wantURL: "/test",
		},
		{
			name:    "base URL with trailing slash",
			baseURL: "https://example.com/",
			method:  "GET",
			path:    "/test",
			wantURL: "https://example.com/test",
		},
		{
			name:    "path without leading slash",
			baseURL: "https://example.com/api",
			method:  "GET",
			path:    "users",
			wantURL: "https://example.com/api/users",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := NewClient(&ClientConfig{
				BaseURL: tt.baseURL,
			})

			req, err := client.NewRequest(context.Background(), tt.method, tt.path, nil)
			if err != nil {
				t.Errorf("NewRequest() error = %v", err)
				return
			}

			if req.URL.String() != tt.wantURL {
				t.Errorf("URL = %v, want %v", req.URL.String(), tt.wantURL)
			}
			if req.Method != tt.method {
				t.Errorf("Method = %v, want %v", req.Method, tt.method)
			}
		})
	}
}

func TestClientNewRequestWithHeaders(t *testing.T) {
	client := NewClient(&ClientConfig{
		DefaultHeaders: map[string]string{
			"X-Custom":      "custom-value",
			"Authorization": "Bearer token",
		},
	})

	req, err := client.NewRequest(context.Background(), "GET", "/test", nil)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	if req.Header.Get("X-Custom") != "custom-value" {
		t.Errorf("X-Custom = %v, want 'custom-value'", req.Header.Get("X-Custom"))
	}
	if req.Header.Get("Authorization") != "Bearer token" {
		t.Errorf("Authorization = %v, want 'Bearer token'", req.Header.Get("Authorization"))
	}
}

func TestClientNewRequestWithBody(t *testing.T) {
	client := NewClient(nil)

	body := strings.NewReader("test body")
	req, err := client.NewRequest(context.Background(), "POST", "/test", body)
	if err != nil {
		t.Fatalf("NewRequest() error = %v", err)
	}

	if req.Body == nil {
		t.Error("expected body to be set")
	}

	content, _ := io.ReadAll(req.Body)
	if string(content) != "test body" {
		t.Errorf("Body = %v, want 'test body'", string(content))
	}
}

func TestClientDo(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{
		BaseURL: server.URL,
	})

	req, _ := client.NewRequest(context.Background(), "GET", "/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}
}

func TestClientRetry(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{
		BaseURL:       server.URL,
		MaxRetries:    3,
		RetryDelay:    10 * time.Millisecond,
		RetryOnStatus: []int{503},
	})

	req, _ := client.NewRequest(context.Background(), "GET", "/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
	}
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestClientRetryExhausted(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{
		BaseURL:       server.URL,
		MaxRetries:    2,
		RetryDelay:    10 * time.Millisecond,
		RetryOnStatus: []int{503},
	})

	req, _ := client.NewRequest(context.Background(), "GET", "/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	// Should get the 503 after exhausting retries
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusServiceUnavailable)
	}
	// 1 initial + 2 retries = 3 attempts
	if attempts != 3 {
		t.Errorf("attempts = %d, want 3", attempts)
	}
}

func TestClientRetry_ContextCancelDuringDelay(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{
		BaseURL:       server.URL,
		MaxRetries:    1,
		RetryDelay:    time.Second,
		RetryOnStatus: []int{503},
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req, _ := client.NewRequest(ctx, "GET", "/test", nil)
	start := time.Now()
	_, err := client.Do(req)

	if err == nil {
		t.Fatal("expected error from context timeout")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected DeadlineExceeded, got %v", err)
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatalf("expected retry delay to be cancelable")
	}
}

func TestClientNoRetryOnSuccess(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{
		BaseURL:       server.URL,
		MaxRetries:    3,
		RetryOnStatus: []int{503},
	})

	req, _ := client.NewRequest(context.Background(), "GET", "/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (no retry)", attempts)
	}
}

func TestClientNoRetryOnNonRetryableStatus(t *testing.T) {
	attempts := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{
		BaseURL:       server.URL,
		MaxRetries:    3,
		RetryOnStatus: []int{503}, // 404 not in retry list
	})

	req, _ := client.NewRequest(context.Background(), "GET", "/test", nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do() error = %v", err)
	}
	defer resp.Body.Close()

	if attempts != 1 {
		t.Errorf("attempts = %d, want 1 (404 not retryable)", attempts)
	}
}

func TestClientContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{
		BaseURL: server.URL,
		Timeout: 5 * time.Second,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	req, _ := client.NewRequest(ctx, "GET", "/test", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Error("expected error from cancelled context")
	}
}

func TestClientRedirect(t *testing.T) {
	redirects := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/final" {
			w.WriteHeader(http.StatusOK)
			return
		}
		redirects++
		http.Redirect(w, r, "/final", http.StatusFound)
	}))
	defer server.Close()

	t.Run("follow redirects", func(t *testing.T) {
		redirects = 0
		client := NewClient(&ClientConfig{
			BaseURL:         server.URL,
			FollowRedirects: true,
		})

		req, _ := client.NewRequest(context.Background(), "GET", "/start", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusOK)
		}
	})

	t.Run("don't follow redirects", func(t *testing.T) {
		redirects = 0
		client := NewClient(&ClientConfig{
			BaseURL:         server.URL,
			FollowRedirects: false,
		})

		req, _ := client.NewRequest(context.Background(), "GET", "/start", nil)
		resp, err := client.Do(req)
		if err != nil {
			t.Fatalf("Do() error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusFound {
			t.Errorf("StatusCode = %d, want %d", resp.StatusCode, http.StatusFound)
		}
	})
}

func TestClientMaxRedirects(t *testing.T) {
	redirectCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirectCount++
		http.Redirect(w, r, "/redirect", http.StatusFound)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{
		BaseURL:         server.URL,
		FollowRedirects: true,
		MaxRedirects:    3,
	})

	req, _ := client.NewRequest(context.Background(), "GET", "/start", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Error("expected error from too many redirects")
	}
}

func TestClientHTTPMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Method", r.Method)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{BaseURL: server.URL})
	ctx := context.Background()

	tests := []struct {
		name   string
		fn     func(context.Context, string) (*http.Response, error)
		method string
	}{
		{"Get", client.Get, "GET"},
		{"Head", client.Head, "HEAD"},
		{"Delete", client.Delete, "DELETE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := tt.fn(ctx, "/test")
			if err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			defer resp.Body.Close()

			if resp.Header.Get("X-Method") != tt.method {
				t.Errorf("Method = %v, want %v", resp.Header.Get("X-Method"), tt.method)
			}
		})
	}
}

func TestClientPostPutPatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("X-Method", r.Method)
		w.Header().Set("X-Body", string(body))
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClient(&ClientConfig{BaseURL: server.URL})
	ctx := context.Background()

	tests := []struct {
		name   string
		fn     func(context.Context, string, io.Reader) (*http.Response, error)
		method string
	}{
		{"Post", client.Post, "POST"},
		{"Put", client.Put, "PUT"},
		{"Patch", client.Patch, "PATCH"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := strings.NewReader("test body")
			resp, err := tt.fn(ctx, "/test", body)
			if err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			defer resp.Body.Close()

			if resp.Header.Get("X-Method") != tt.method {
				t.Errorf("Method = %v, want %v", resp.Header.Get("X-Method"), tt.method)
			}
			if resp.Header.Get("X-Body") != "test body" {
				t.Errorf("Body = %v, want 'test body'", resp.Header.Get("X-Body"))
			}
		})
	}
}

func TestClientClose(t *testing.T) {
	client := NewClient(nil)
	// Should not panic
	client.Close()
}

func TestClientSetRemoveHeader(t *testing.T) {
	client := NewClient(nil)

	client.SetHeader("X-Test", "value")
	if client.config.DefaultHeaders["X-Test"] != "value" {
		t.Error("SetHeader failed")
	}

	client.RemoveHeader("X-Test")
	if _, ok := client.config.DefaultHeaders["X-Test"]; ok {
		t.Error("RemoveHeader failed")
	}
}

func TestJoinPaths(t *testing.T) {
	tests := []struct {
		base string
		path string
		want string
	}{
		{"", "", ""},
		{"", "/test", "/test"},
		{"/api", "", "/api"},
		{"/api", "/test", "/api/test"},
		{"/api/", "/test", "/api/test"},
		{"/api", "test", "/api/test"},
		{"/api/", "test", "/api/test"},
	}

	for _, tt := range tests {
		t.Run(tt.base+"+"+tt.path, func(t *testing.T) {
			got := joinPaths(tt.base, tt.path)
			if got != tt.want {
				t.Errorf("joinPaths(%q, %q) = %q, want %q", tt.base, tt.path, got, tt.want)
			}
		})
	}
}

func TestIsAbsoluteURL(t *testing.T) {
	tests := []struct {
		path string
		want bool
	}{
		{"/test", false},
		{"test", false},
		{"https://example.com", true},
		{"http://example.com/test", true},
		{"://invalid", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := isAbsoluteURL(tt.path)
			if got != tt.want {
				t.Errorf("isAbsoluteURL(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

func TestTrimSuffix(t *testing.T) {
	tests := []struct {
		s      string
		suffix string
		want   string
	}{
		{"hello/", "/", "hello"},
		{"hello", "/", "hello"},
		{"", "/", ""},
		{"/", "/", ""},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := trimSuffix(tt.s, tt.suffix)
			if got != tt.want {
				t.Errorf("trimSuffix(%q, %q) = %q, want %q", tt.s, tt.suffix, got, tt.want)
			}
		})
	}
}

func TestHasPrefix(t *testing.T) {
	tests := []struct {
		s      string
		prefix string
		want   bool
	}{
		{"/test", "/", true},
		{"test", "/", false},
		{"", "/", false},
		{"/", "/", true},
	}

	for _, tt := range tests {
		t.Run(tt.s, func(t *testing.T) {
			got := hasPrefix(tt.s, tt.prefix)
			if got != tt.want {
				t.Errorf("hasPrefix(%q, %q) = %v, want %v", tt.s, tt.prefix, got, tt.want)
			}
		})
	}
}

func TestClientShouldRetry(t *testing.T) {
	client := NewClient(&ClientConfig{
		RetryOnStatus: []int{429, 502, 503, 504},
	})

	tests := []struct {
		status int
		want   bool
	}{
		{200, false},
		{400, false},
		{401, false},
		{404, false},
		{429, true},
		{500, false},
		{502, true},
		{503, true},
		{504, true},
	}

	for _, tt := range tests {
		t.Run(http.StatusText(tt.status), func(t *testing.T) {
			got := client.shouldRetry(tt.status)
			if got != tt.want {
				t.Errorf("shouldRetry(%d) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestClientNetworkError(t *testing.T) {
	client := NewClient(&ClientConfig{
		BaseURL:    "http://localhost:99999",
		MaxRetries: 2,
		RetryDelay: 10 * time.Millisecond,
	})

	req, _ := client.NewRequest(context.Background(), "GET", "/test", nil)
	_, err := client.Do(req)
	if err == nil {
		t.Error("expected error from network failure")
	}
}
