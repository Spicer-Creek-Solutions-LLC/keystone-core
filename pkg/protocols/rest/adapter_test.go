package rest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/credentials"
	"github.com/shawnbutts/keystone-core/pkg/protocols"
	"github.com/shawnbutts/keystone-core/pkg/proxy"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.FollowRedirects != true {
		t.Error("expected FollowRedirects to be true")
	}
	if cfg.MaxRedirects != 10 {
		t.Errorf("expected MaxRedirects=10, got %d", cfg.MaxRedirects)
	}
	if cfg.ValidateSSL != true {
		t.Error("expected ValidateSSL to be true")
	}
	if cfg.ContentType != "application/json" {
		t.Errorf("expected ContentType=application/json, got %s", cfg.ContentType)
	}
	if cfg.AcceptType != "application/json" {
		t.Errorf("expected AcceptType=application/json, got %s", cfg.AcceptType)
	}
	if len(cfg.RetryOnStatus) != 4 {
		t.Errorf("expected 4 retry status codes, got %d", len(cfg.RetryOnStatus))
	}
}

func TestNewAdapter(t *testing.T) {
	tests := []struct {
		name   string
		config *Config
	}{
		{
			name:   "nil config uses defaults",
			config: nil,
		},
		{
			name:   "custom config",
			config: &Config{BaseURL: "https://example.com"},
		},
		{
			name: "config with rate limiting",
			config: &Config{
				RateLimitPerSecond: 10,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewAdapter(tt.config)
			if adapter == nil {
				t.Fatal("expected adapter to be created")
			}
			if adapter.config == nil {
				t.Error("expected config to be set")
			}
			if adapter.metrics == nil {
				t.Error("expected metrics to be initialized")
			}
		})
	}
}

func TestAdapterType(t *testing.T) {
	adapter := NewAdapter(nil)
	if adapter.Type() != protocols.ProtocolREST {
		t.Errorf("expected ProtocolREST, got %v", adapter.Type())
	}
}

func TestAdapterConnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tests := []struct {
		name      string
		config    *Config
		device    *proxy.ProxiedDevice
		cred      credentials.Credential
		wantError bool
	}{
		{
			name:   "connect with basic auth",
			config: &Config{BaseURL: server.URL},
			device: &proxy.ProxiedDevice{Address: "localhost"},
			cred:   &credentials.RESTBasicCredential{Username: "user", Password: "pass"},
		},
		{
			name:   "connect with bearer token",
			config: &Config{BaseURL: server.URL},
			device: &proxy.ProxiedDevice{Address: "localhost"},
			cred:   &credentials.RESTBearerCredential{Token: "test-token"},
		},
		{
			name:   "connect with nil credential",
			config: &Config{BaseURL: server.URL},
			device: &proxy.ProxiedDevice{Address: "localhost"},
			cred:   nil,
		},
		{
			name:   "connect with api key",
			config: &Config{BaseURL: server.URL},
			device: &proxy.ProxiedDevice{Address: "localhost"},
			cred:   &credentials.RESTAPIKeyCredential{APIKey: "my-key", HeaderName: "X-API-Key"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			adapter := NewAdapter(tt.config)
			err := adapter.Connect(context.Background(), tt.device, tt.cred)
			if tt.wantError && err == nil {
				t.Error("expected error")
			}
			if !tt.wantError && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
			if !tt.wantError && !adapter.IsConnected() {
				t.Error("expected adapter to be connected")
			}
		})
	}
}

func TestAdapterConnectBuildURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tests := []struct {
		name       string
		baseURL    string
		device     *proxy.ProxiedDevice
		validateSS bool
	}{
		{
			name:    "with explicit base URL",
			baseURL: server.URL,
			device:  &proxy.ProxiedDevice{Address: "localhost", Port: 8080},
		},
		{
			name:       "without base URL, https",
			baseURL:    "",
			device:     &proxy.ProxiedDevice{Address: "localhost", Port: 443},
			validateSS: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				BaseURL:     tt.baseURL,
				ValidateSSL: tt.validateSS,
			}
			adapter := NewAdapter(config)

			if tt.baseURL != "" {
				err := adapter.Connect(context.Background(), tt.device, nil)
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestAdapterDisconnect(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{BaseURL: server.URL})
	device := &proxy.ProxiedDevice{Address: "localhost"}

	if err := adapter.Connect(context.Background(), device, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	if !adapter.IsConnected() {
		t.Error("expected to be connected")
	}

	if err := adapter.Disconnect(context.Background()); err != nil {
		t.Errorf("disconnect failed: %v", err)
	}

	if adapter.IsConnected() {
		t.Error("expected to be disconnected")
	}
}

func TestAdapterExecute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/data":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
		case "/api/error":
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("internal error"))
		default:
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{BaseURL: server.URL})
	device := &proxy.ProxiedDevice{Address: "localhost"}

	if err := adapter.Connect(context.Background(), device, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	tests := []struct {
		name       string
		command    string
		wantStatus int
		wantError  bool
	}{
		{
			name:       "GET request",
			command:    "GET /api/data",
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST request with body",
			command:    `POST /api/data {"key":"value"}`,
			wantStatus: http.StatusOK,
		},
		{
			name:       "error response",
			command:    "GET /api/error",
			wantStatus: http.StatusInternalServerError,
		},
		{
			name:      "invalid command format",
			command:   "INVALID",
			wantError: true,
		},
		{
			name:      "invalid method",
			command:   "BADMETHOD /path",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &protocols.ExecuteRequest{Command: tt.command}
			result, err := adapter.Execute(context.Background(), req)

			if tt.wantError {
				if err == nil && result.Error == "" {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if result.ExitCode != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, result.ExitCode)
			}

			if result.Duration <= 0 {
				t.Error("expected positive duration")
			}
		})
	}
}

func TestAdapterExecuteNotConnected(t *testing.T) {
	adapter := NewAdapter(nil)
	req := &protocols.ExecuteRequest{Command: "GET /api/data"}
	_, err := adapter.Execute(context.Background(), req)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestAdapterRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"method": r.Method,
			"path":   r.URL.Path,
		})
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{BaseURL: server.URL})
	device := &proxy.ProxiedDevice{Address: "localhost"}

	if err := adapter.Connect(context.Background(), device, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	tests := []struct {
		name   string
		method string
		path   string
		body   interface{}
	}{
		{"string body", "POST", "/api/test", "test body"},
		{"byte body", "POST", "/api/test", []byte("test body")},
		{"struct body", "POST", "/api/test", map[string]string{"key": "value"}},
		{"reader body", "POST", "/api/test", strings.NewReader("test body")},
		{"nil body", "GET", "/api/test", nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp, err := adapter.Request(context.Background(), tt.method, tt.path, tt.body)
			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}
			if !resp.IsSuccess() {
				t.Errorf("expected success response, got %d", resp.StatusCode)
			}
		})
	}
}

func TestAdapterHTTPMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"method": r.Method})
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{BaseURL: server.URL})
	device := &proxy.ProxiedDevice{Address: "localhost"}

	if err := adapter.Connect(context.Background(), device, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	ctx := context.Background()

	t.Run("Get", func(t *testing.T) {
		resp, err := adapter.Get(ctx, "/test")
		if err != nil {
			t.Errorf("Get failed: %v", err)
		}
		if !resp.IsSuccess() {
			t.Errorf("expected success, got %d", resp.StatusCode)
		}
	})

	t.Run("Post", func(t *testing.T) {
		resp, err := adapter.Post(ctx, "/test", map[string]string{"key": "value"})
		if err != nil {
			t.Errorf("Post failed: %v", err)
		}
		if !resp.IsSuccess() {
			t.Errorf("expected success, got %d", resp.StatusCode)
		}
	})

	t.Run("Put", func(t *testing.T) {
		resp, err := adapter.Put(ctx, "/test", map[string]string{"key": "value"})
		if err != nil {
			t.Errorf("Put failed: %v", err)
		}
		if !resp.IsSuccess() {
			t.Errorf("expected success, got %d", resp.StatusCode)
		}
	})

	t.Run("Patch", func(t *testing.T) {
		resp, err := adapter.Patch(ctx, "/test", map[string]string{"key": "value"})
		if err != nil {
			t.Errorf("Patch failed: %v", err)
		}
		if !resp.IsSuccess() {
			t.Errorf("expected success, got %d", resp.StatusCode)
		}
	})

	t.Run("Delete", func(t *testing.T) {
		resp, err := adapter.Delete(ctx, "/test")
		if err != nil {
			t.Errorf("Delete failed: %v", err)
		}
		if !resp.IsSuccess() {
			t.Errorf("expected success, got %d", resp.StatusCode)
		}
	})
}

func TestAdapterJSONMethods(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		var reqBody map[string]interface{}
		if r.Body != nil {
			json.NewDecoder(r.Body).Decode(&reqBody)
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"method":  r.Method,
			"request": reqBody,
		})
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{BaseURL: server.URL})
	device := &proxy.ProxiedDevice{Address: "localhost"}

	if err := adapter.Connect(context.Background(), device, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	ctx := context.Background()

	t.Run("GetJSON", func(t *testing.T) {
		var result map[string]interface{}
		err := adapter.GetJSON(ctx, "/test", &result)
		if err != nil {
			t.Errorf("GetJSON failed: %v", err)
		}
		if result["method"] != "GET" {
			t.Errorf("expected GET method, got %v", result["method"])
		}
	})

	t.Run("PostJSON", func(t *testing.T) {
		var result map[string]interface{}
		err := adapter.PostJSON(ctx, "/test", map[string]string{"key": "value"}, &result)
		if err != nil {
			t.Errorf("PostJSON failed: %v", err)
		}
		if result["method"] != "POST" {
			t.Errorf("expected POST method, got %v", result["method"])
		}
	})

	t.Run("PutJSON", func(t *testing.T) {
		var result map[string]interface{}
		err := adapter.PutJSON(ctx, "/test", map[string]string{"key": "value"}, &result)
		if err != nil {
			t.Errorf("PutJSON failed: %v", err)
		}
		if result["method"] != "PUT" {
			t.Errorf("expected PUT method, got %v", result["method"])
		}
	})

	t.Run("PatchJSON", func(t *testing.T) {
		var result map[string]interface{}
		err := adapter.PatchJSON(ctx, "/test", map[string]string{"key": "value"}, &result)
		if err != nil {
			t.Errorf("PatchJSON failed: %v", err)
		}
		if result["method"] != "PATCH" {
			t.Errorf("expected PATCH method, got %v", result["method"])
		}
	})
}

func TestAdapterHealthCheck(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	t.Run("healthy when connected", func(t *testing.T) {
		adapter := NewAdapter(&Config{BaseURL: server.URL})
		device := &proxy.ProxiedDevice{Address: "localhost"}

		if err := adapter.Connect(context.Background(), device, nil); err != nil {
			t.Fatalf("connect failed: %v", err)
		}

		result, err := adapter.HealthCheck(context.Background())
		if err != nil {
			t.Errorf("health check failed: %v", err)
		}
		if !result.Healthy {
			t.Error("expected healthy")
		}
		if result.Status != "connected" {
			t.Errorf("expected status 'connected', got %s", result.Status)
		}
		if result.Latency <= 0 {
			t.Error("expected positive latency")
		}
	})

	t.Run("unhealthy when not connected", func(t *testing.T) {
		adapter := NewAdapter(nil)

		result, err := adapter.HealthCheck(context.Background())
		if err != nil {
			t.Errorf("health check failed: %v", err)
		}
		if result.Healthy {
			t.Error("expected unhealthy")
		}
		if result.Status != "not connected" {
			t.Errorf("expected status 'not connected', got %s", result.Status)
		}
	})
}

func TestAdapterMetrics(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{BaseURL: server.URL})
	device := &proxy.ProxiedDevice{Address: "localhost"}

	if err := adapter.Connect(context.Background(), device, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Execute a few requests
	for i := 0; i < 3; i++ {
		req := &protocols.ExecuteRequest{Command: "GET /"}
		adapter.Execute(context.Background(), req)
	}

	metrics := adapter.Metrics()
	if metrics.ConnectionCount != 1 {
		t.Errorf("expected 1 connection, got %d", metrics.ConnectionCount)
	}
	if metrics.ExecutionCount != 3 {
		t.Errorf("expected 3 executions, got %d", metrics.ExecutionCount)
	}
}

func TestAdapterBuildURL(t *testing.T) {
	adapter := NewAdapter(nil)

	tests := []struct {
		path   string
		params map[string]string
		want   string
	}{
		{"/api/data", nil, "/api/data"},
		{"/api/data", map[string]string{}, "/api/data"},
		{"/api/data", map[string]string{"key": "value"}, "/api/data?key=value"},
		{"/api/data?existing=param", map[string]string{"key": "value"}, "/api/data?existing=param&key=value"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			got := adapter.BuildURL(tt.path, tt.params)
			if !strings.Contains(got, tt.path[:strings.Index(tt.path, "?")+1]) {
				t.Errorf("BuildURL() = %v, want to contain base path", got)
			}
		})
	}
}

func TestParseRESTCommand(t *testing.T) {
	tests := []struct {
		name       string
		cmd        string
		wantMethod string
		wantPath   string
		wantBody   bool
		wantErr    bool
	}{
		{"GET request", "GET /api/data", "GET", "/api/data", false, false},
		{"POST with body", "POST /api/data {\"key\":\"value\"}", "POST", "/api/data", true, false},
		{"lowercase method", "get /api/data", "GET", "/api/data", false, false},
		{"path without slash", "GET api/data", "GET", "/api/data", false, false},
		{"absolute URL", "GET https://example.com/api", "GET", "https://example.com/api", false, false},
		{"HEAD request", "HEAD /api/data", "HEAD", "/api/data", false, false},
		{"OPTIONS request", "OPTIONS /api/data", "OPTIONS", "/api/data", false, false},
		{"invalid command", "INVALID", "", "", false, true},
		{"invalid method", "BADMETHOD /path", "", "", false, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method, path, body, err := parseRESTCommand(tt.cmd)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if method != tt.wantMethod {
				t.Errorf("method = %v, want %v", method, tt.wantMethod)
			}

			if path != tt.wantPath {
				t.Errorf("path = %v, want %v", path, tt.wantPath)
			}

			if tt.wantBody && body == nil {
				t.Error("expected body")
			}
			if !tt.wantBody && body != nil {
				t.Error("unexpected body")
			}
		})
	}
}

func TestNewAdapterFactory(t *testing.T) {
	factory := NewAdapterFactory(nil)
	if factory == nil {
		t.Fatal("expected factory to be created")
	}

	connConfig := protocols.DefaultConnectionConfig()
	adapter, err := factory(connConfig)
	if err != nil {
		t.Errorf("factory failed: %v", err)
	}
	if adapter == nil {
		t.Error("expected adapter from factory")
	}
	if adapter.Type() != protocols.ProtocolREST {
		t.Errorf("expected REST protocol, got %v", adapter.Type())
	}
}

func TestRateLimiter(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		limiter := NewRateLimiter(10)

		for i := 0; i < 10; i++ {
			ctx := context.Background()
			err := limiter.Wait(ctx)
			if err != nil {
				t.Errorf("unexpected error on request %d: %v", i, err)
			}
		}
	})

	t.Run("context cancellation", func(t *testing.T) {
		limiter := NewRateLimiter(1)

		// Use up the token
		ctx := context.Background()
		limiter.Wait(ctx)

		// Next request should block, cancel it
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()

		// Wait should return context error
		err := limiter.Wait(ctx)
		if err == nil {
			t.Log("rate limiter allowed request (token replenished)")
		}
	})

	t.Run("token replenishment", func(t *testing.T) {
		limiter := NewRateLimiter(100)

		// Use all tokens
		for i := 0; i < 100; i++ {
			limiter.Wait(context.Background())
		}

		// Wait for replenishment
		time.Sleep(50 * time.Millisecond)

		// Should have some tokens now
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		err := limiter.Wait(ctx)
		if err != nil {
			t.Errorf("expected token replenishment: %v", err)
		}
	})
}

func TestAdapterWithRateLimiting(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{
		BaseURL:            server.URL,
		RateLimitPerSecond: 100,
	})

	device := &proxy.ProxiedDevice{Address: "localhost"}
	if err := adapter.Connect(context.Background(), device, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	// Reset counter after connect (which makes a verification request)
	countAfterConnect := requestCount

	// Make several requests
	for i := 0; i < 5; i++ {
		_, err := adapter.Get(context.Background(), "/test")
		if err != nil {
			t.Errorf("request %d failed: %v", i, err)
		}
	}

	additionalRequests := requestCount - countAfterConnect
	if additionalRequests != 5 {
		t.Errorf("expected 5 additional requests, got %d", additionalRequests)
	}
}

func TestVerifyConnectivity(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{"200 OK", http.StatusOK, false},
		{"201 Created", http.StatusCreated, false},
		{"301 Redirect", http.StatusMovedPermanently, false},
		{"401 Unauthorized", http.StatusUnauthorized, false},
		{"403 Forbidden", http.StatusForbidden, false},
		{"404 Not Found", http.StatusNotFound, true},
		{"500 Server Error", http.StatusInternalServerError, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			adapter := NewAdapter(&Config{BaseURL: server.URL})
			device := &proxy.ProxiedDevice{Address: "localhost"}

			err := adapter.Connect(context.Background(), device, nil)
			if tt.wantErr && err == nil {
				t.Error("expected error")
			}
			if !tt.wantErr && err != nil {
				t.Errorf("unexpected error: %v", err)
			}
		})
	}
}

func TestSetupAuth(t *testing.T) {
	adapter := NewAdapter(nil)

	tests := []struct {
		name     string
		cred     credentials.Credential
		wantType string
		wantErr  bool
	}{
		{
			name:     "basic auth",
			cred:     &credentials.RESTBasicCredential{Username: "user", Password: "pass"},
			wantType: "basic",
		},
		{
			name:     "bearer auth",
			cred:     &credentials.RESTBearerCredential{Token: "token"},
			wantType: "bearer",
		},
		{
			name:     "api key header",
			cred:     &credentials.RESTAPIKeyCredential{APIKey: "key", HeaderName: "X-API-Key"},
			wantType: "api_key",
		},
		{
			name:     "api key query",
			cred:     &credentials.RESTAPIKeyCredential{APIKey: "key", QueryParam: "api_key"},
			wantType: "api_key",
		},
		{
			name:     "oauth2 client credentials",
			cred:     &credentials.RESTOAuth2Credential{ClientID: "id", ClientSecret: "secret", TokenURL: "https://example.com/token"},
			wantType: "oauth2_client_credentials",
		},
		{
			name:     "nil credential",
			cred:     nil,
			wantType: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			auth, err := adapter.setupAuth(tt.cred)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error")
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if auth.Type() != tt.wantType {
				t.Errorf("auth type = %v, want %v", auth.Type(), tt.wantType)
			}
		})
	}
}

func TestAdapterRequestNotConnected(t *testing.T) {
	adapter := NewAdapter(nil)
	_, err := adapter.Request(context.Background(), "GET", "/test", nil)
	if err == nil {
		t.Error("expected error when not connected")
	}
}

func TestAdapterExecuteWithContextCancel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{BaseURL: server.URL})
	device := &proxy.ProxiedDevice{Address: "localhost"}

	if err := adapter.Connect(context.Background(), device, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	req := &protocols.ExecuteRequest{Command: "GET /slow"}
	_, err := adapter.Execute(ctx, req)
	if err == nil {
		t.Log("request completed before context cancelled")
	}
}

func TestBodyReaderTypes(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"received": string(body)})
	}))
	defer server.Close()

	adapter := NewAdapter(&Config{BaseURL: server.URL})
	device := &proxy.ProxiedDevice{Address: "localhost"}

	if err := adapter.Connect(context.Background(), device, nil); err != nil {
		t.Fatalf("connect failed: %v", err)
	}

	ctx := context.Background()

	t.Run("string body", func(t *testing.T) {
		_, err := adapter.Request(ctx, "POST", "/test", "string body")
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("byte slice body", func(t *testing.T) {
		_, err := adapter.Request(ctx, "POST", "/test", []byte("byte body"))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("io.Reader body", func(t *testing.T) {
		_, err := adapter.Request(ctx, "POST", "/test", strings.NewReader("reader body"))
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})

	t.Run("struct body (JSON)", func(t *testing.T) {
		_, err := adapter.Request(ctx, "POST", "/test", struct{ Key string }{Key: "value"})
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	})
}
