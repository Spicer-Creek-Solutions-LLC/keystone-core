package registry

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
)

func TestNewHTTPClient(t *testing.T) {
	tests := []struct {
		name      string
		config    *RegistryConfig
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: true, // URL is required
		},
		{
			name:      "empty URL fails",
			config:    &RegistryConfig{},
			wantErr:   true,
			errSubstr: "URL is required",
		},
		{
			name: "valid config",
			config: &RegistryConfig{
				URL: "https://registry.example.com",
			},
			wantErr: false,
		},
		{
			name: "URL with trailing slash normalized",
			config: &RegistryConfig{
				URL: "https://registry.example.com/",
			},
			wantErr: false,
		},
		{
			name: "custom timeout",
			config: &RegistryConfig{
				URL:     "https://registry.example.com",
				Timeout: 60 * time.Second,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client, err := NewHTTPClient(tt.config)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				} else if tt.errSubstr != "" && !contains(err.Error(), tt.errSubstr) {
					t.Errorf("error %q should contain %q", err.Error(), tt.errSubstr)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if client == nil {
				t.Error("expected client, got nil")
			}

			if client.config.URL == "" {
				t.Error("URL should be set")
			}

			// Check defaults are applied
			if client.config.Timeout == 0 {
				t.Error("timeout should have default value")
			}
			if client.config.RetryAttempts == 0 {
				t.Error("retry attempts should have default value")
			}
			if client.config.Namespace == "" {
				t.Error("namespace should have default value")
			}
		})
	}
}

func TestDefaultRegistryConfig(t *testing.T) {
	config := DefaultRegistryConfig()

	if config.Timeout != 30*time.Second {
		t.Errorf("default timeout = %v, want 30s", config.Timeout)
	}
	if config.RetryAttempts != 3 {
		t.Errorf("default retry attempts = %d, want 3", config.RetryAttempts)
	}
	if config.RetryDelay != 1*time.Second {
		t.Errorf("default retry delay = %v, want 1s", config.RetryDelay)
	}
	if config.Namespace != "blueprints" {
		t.Errorf("default namespace = %q, want %q", config.Namespace, "blueprints")
	}
}

func TestHTTPClient_ListVersions(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blueprints/myorg/web-stack/@v/list" {
			w.Write([]byte("1.0.0\n1.1.0\n2.0.0\n"))
			return
		}
		if r.URL.Path == "/blueprints/notfound/bp/@v/list" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client, err := NewHTTPClient(&RegistryConfig{URL: server.URL})
	if err != nil {
		t.Fatalf("failed to create client: %v", err)
	}

	t.Run("success", func(t *testing.T) {
		versions, err := client.ListVersions("myorg/web-stack")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if len(versions) != 3 {
			t.Errorf("got %d versions, want 3", len(versions))
		}

		// Should be sorted newest first
		if versions[0] != "2.0.0" {
			t.Errorf("first version = %q, want %q", versions[0], "2.0.0")
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := client.ListVersions("notfound/bp")
		if err != ErrBlueprintNotFound {
			t.Errorf("error = %v, want ErrBlueprintNotFound", err)
		}
	})
}

func TestHTTPClient_GetBlueprintInfo(t *testing.T) {
	info := &BlueprintInfo{
		Name:      "myorg/web-stack",
		Version:   "1.0.0",
		Published: time.Now(),
		Checksum:  "sha256:abc123",
		Size:      1024,
		Metadata: &blueprint.Metadata{
			Name:        "web-stack",
			Description: "A web stack blueprint",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blueprints/myorg/web-stack/@v/1.0.0.info" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(info)
			return
		}
		if r.URL.Path == "/blueprints/myorg/web-stack/@v/9.9.9.info" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	t.Run("success", func(t *testing.T) {
		result, err := client.GetBlueprintInfo("myorg/web-stack", "1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Name != info.Name {
			t.Errorf("name = %q, want %q", result.Name, info.Name)
		}
		if result.Version != info.Version {
			t.Errorf("version = %q, want %q", result.Version, info.Version)
		}
		if result.Checksum != info.Checksum {
			t.Errorf("checksum = %q, want %q", result.Checksum, info.Checksum)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := client.GetBlueprintInfo("myorg/web-stack", "9.9.9")
		if err != ErrVersionNotFound {
			t.Errorf("error = %v, want ErrVersionNotFound", err)
		}
	})
}

func TestHTTPClient_GetLatestVersion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blueprints/myorg/web-stack/@latest" {
			w.Write([]byte("2.0.0"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	version, err := client.GetLatestVersion("myorg/web-stack")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if version != "2.0.0" {
		t.Errorf("version = %q, want %q", version, "2.0.0")
	}
}

func TestHTTPClient_DownloadBlueprint(t *testing.T) {
	archiveData := []byte("mock archive content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blueprints/myorg/web-stack/@v/1.0.0.zip" {
			w.Write(archiveData)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	t.Run("success", func(t *testing.T) {
		data, err := client.DownloadBlueprint("myorg/web-stack", "1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if string(data) != string(archiveData) {
			t.Errorf("data = %q, want %q", data, archiveData)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := client.DownloadBlueprint("myorg/web-stack", "9.9.9")
		if err != ErrVersionNotFound {
			t.Errorf("error = %v, want ErrVersionNotFound", err)
		}
	})
}

func TestHTTPClient_GetManifest(t *testing.T) {
	manifestYAML := `apiVersion: blueprints.kscore.io/v1
kind: Blueprint
metadata:
  name: web-stack
  version: 1.0.0
  description: A web stack blueprint
`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/blueprints/myorg/web-stack/@v/1.0.0.mod" {
			w.Write([]byte(manifestYAML))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	bp, err := client.GetManifest("myorg/web-stack", "1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if bp.Metadata.Name != "web-stack" {
		t.Errorf("name = %q, want %q", bp.Metadata.Name, "web-stack")
	}
	if bp.Metadata.Description != "A web stack blueprint" {
		t.Errorf("description = %q, want %q", bp.Metadata.Description, "A web stack blueprint")
	}
}

func TestHTTPClient_PublishBlueprint(t *testing.T) {
	publishResult := &PublishResult{
		Name:      "myorg/web-stack",
		Version:   "1.0.0",
		Published: time.Now(),
		URL:       "https://registry.example.com/blueprints/myorg/web-stack/@v/1.0.0.zip",
		Checksum:  "sha256:abc123",
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}

		// Check content type is multipart
		ct := r.Header.Get("Content-Type")
		if !contains(ct, "multipart/form-data") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Parse multipart form
		if err := r.ParseMultipartForm(10 << 20); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		// Check archive is present
		if _, _, err := r.FormFile("archive"); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(publishResult)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	t.Run("success", func(t *testing.T) {
		req := &PublishRequest{
			Name:     "myorg/web-stack",
			Version:  "1.0.0",
			Archive:  []byte("archive content"),
			Manifest: []byte("manifest content"),
		}

		result, err := client.PublishBlueprint(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result.Name != publishResult.Name {
			t.Errorf("name = %q, want %q", result.Name, publishResult.Name)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		req := &PublishRequest{
			Version: "1.0.0",
			Archive: []byte("archive content"),
		}

		_, err := client.PublishBlueprint(req)
		if err == nil {
			t.Error("expected error for missing name")
		}
	})

	t.Run("missing archive", func(t *testing.T) {
		req := &PublishRequest{
			Name:    "myorg/web-stack",
			Version: "1.0.0",
		}

		_, err := client.PublishBlueprint(req)
		if err == nil {
			t.Error("expected error for missing archive")
		}
	})
}

func TestHTTPClient_Search(t *testing.T) {
	searchResult := &SearchResult{
		Blueprints: []*BlueprintInfo{
			{Name: "myorg/web-stack", Version: "1.0.0"},
			{Name: "myorg/api-gateway", Version: "2.0.0"},
		},
		Total:  2,
		Limit:  10,
		Offset: 0,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/blueprints/@search" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		// Check query parameters
		query := r.URL.Query()
		if query.Get("vendor") != "myorg" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(searchResult)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	result, err := client.Search(&SearchQuery{Vendor: "myorg"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.Blueprints) != 2 {
		t.Errorf("got %d blueprints, want 2", len(result.Blueprints))
	}
	if result.Total != 2 {
		t.Errorf("total = %d, want 2", result.Total)
	}
}

func TestHTTPClient_GetIndex(t *testing.T) {
	index := []*IndexEntry{
		{Name: "myorg/web-stack", LatestVersion: "2.0.0", Downloads: 100},
		{Name: "myorg/api-gateway", LatestVersion: "1.5.0", Downloads: 50},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/blueprints/@index" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(index)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	entries, err := client.GetIndex()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("got %d entries, want 2", len(entries))
	}
}

func TestHTTPClient_GetVendors(t *testing.T) {
	vendors := []string{"myorg", "acme", "example"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/blueprints/@vendors" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(vendors)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	result, err := client.GetVendors()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("got %d vendors, want 3", len(result))
	}
}

func TestHTTPClient_GetTags(t *testing.T) {
	tags := []string{"web", "database", "kubernetes"}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/blueprints/@tags" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tags)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	result, err := client.GetTags()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result) != 3 {
		t.Errorf("got %d tags, want 3", len(result))
	}
}

func TestHTTPClient_Authentication(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.Write([]byte("2.0.0"))
	}))
	defer server.Close()

	t.Run("basic auth", func(t *testing.T) {
		client, _ := NewHTTPClient(&RegistryConfig{
			URL: server.URL,
			Auth: &AuthConfig{
				Type:     AuthTypeBasic,
				Username: "user",
				Password: "pass",
			},
		})

		client.GetLatestVersion("myorg/test")

		if receivedAuth == "" {
			t.Error("no auth header received")
		}
		if !contains(receivedAuth, "Basic") {
			t.Errorf("auth = %q, want Basic auth", receivedAuth)
		}
	})

	t.Run("bearer auth", func(t *testing.T) {
		client, _ := NewHTTPClient(&RegistryConfig{
			URL: server.URL,
			Auth: &AuthConfig{
				Type:  AuthTypeBearer,
				Token: "mytoken",
			},
		})

		client.GetLatestVersion("myorg/test")

		if receivedAuth != "Bearer mytoken" {
			t.Errorf("auth = %q, want %q", receivedAuth, "Bearer mytoken")
		}
	})

	t.Run("api key auth", func(t *testing.T) {
		var receivedAPIKey string
		apiKeyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedAPIKey = r.Header.Get("X-API-Key")
			w.Write([]byte("2.0.0"))
		}))
		defer apiKeyServer.Close()

		client, _ := NewHTTPClient(&RegistryConfig{
			URL: apiKeyServer.URL,
			Auth: &AuthConfig{
				Type:  AuthTypeAPIKey,
				Token: "myapikey",
			},
		})

		client.GetLatestVersion("myorg/test")

		if receivedAPIKey != "myapikey" {
			t.Errorf("api key = %q, want %q", receivedAPIKey, "myapikey")
		}
	})
}

func TestHTTPClient_ErrorHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/blueprints/unauthorized/@latest":
			w.WriteHeader(http.StatusUnauthorized)
		case "/blueprints/forbidden/@latest":
			w.WriteHeader(http.StatusForbidden)
		case "/blueprints/conflict/@v/1.0.0":
			w.WriteHeader(http.StatusConflict)
		case "/blueprints/error/@latest":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(&RegistryError{
				Code:    "invalid_request",
				Message: "Invalid request parameters",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{URL: server.URL})

	t.Run("unauthorized", func(t *testing.T) {
		_, err := client.GetLatestVersion("unauthorized")
		if err != ErrUnauthorized {
			t.Errorf("error = %v, want ErrUnauthorized", err)
		}
	})

	t.Run("forbidden", func(t *testing.T) {
		_, err := client.GetLatestVersion("forbidden")
		if err != ErrForbidden {
			t.Errorf("error = %v, want ErrForbidden", err)
		}
	})

	t.Run("conflict on publish", func(t *testing.T) {
		_, err := client.PublishBlueprint(&PublishRequest{
			Name:    "conflict",
			Version: "1.0.0",
			Archive: []byte("data"),
		})
		if err != ErrVersionExists {
			t.Errorf("error = %v, want ErrVersionExists", err)
		}
	})

	t.Run("registry error", func(t *testing.T) {
		_, err := client.GetLatestVersion("error")
		regErr, ok := err.(*RegistryError)
		if !ok {
			t.Fatalf("error type = %T, want *RegistryError", err)
		}
		if regErr.Code != "invalid_request" {
			t.Errorf("code = %q, want %q", regErr.Code, "invalid_request")
		}
	})
}

func TestHTTPClient_RetryRespectsTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	client, _ := NewHTTPClient(&RegistryConfig{
		URL:           server.URL,
		RetryAttempts: 3,
		RetryDelay:    time.Second,
		Timeout:       50 * time.Millisecond,
	})

	start := time.Now()
	_, err := client.GetIndex()

	if err == nil {
		t.Fatal("expected error from timeout")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("expected retry delay to be bounded by timeout")
	}
}

func TestRegistryError(t *testing.T) {
	t.Run("Error method", func(t *testing.T) {
		err := &RegistryError{Message: "test error"}
		if err.Error() != "test error" {
			t.Errorf("Error() = %q, want %q", err.Error(), "test error")
		}

		err = &RegistryError{Code: "test_code"}
		if err.Error() != "test_code" {
			t.Errorf("Error() = %q, want %q", err.Error(), "test_code")
		}
	})

	t.Run("IsNotFound", func(t *testing.T) {
		err := &RegistryError{StatusCode: 404}
		if !err.IsNotFound() {
			t.Error("expected IsNotFound() = true")
		}

		err = &RegistryError{Code: "not_found"}
		if !err.IsNotFound() {
			t.Error("expected IsNotFound() = true")
		}
	})

	t.Run("IsUnauthorized", func(t *testing.T) {
		err := &RegistryError{StatusCode: 401}
		if !err.IsUnauthorized() {
			t.Error("expected IsUnauthorized() = true")
		}
	})

	t.Run("IsForbidden", func(t *testing.T) {
		err := &RegistryError{StatusCode: 403}
		if !err.IsForbidden() {
			t.Error("expected IsForbidden() = true")
		}
	})

	t.Run("IsConflict", func(t *testing.T) {
		err := &RegistryError{StatusCode: 409}
		if !err.IsConflict() {
			t.Error("expected IsConflict() = true")
		}
	})
}

func TestBlueprintPath(t *testing.T) {
	path := BlueprintPath("blueprints", "myorg/web-stack", "1.0.0")
	expected := "blueprints/myorg/web-stack/@v/1.0.0"
	if path != expected {
		t.Errorf("BlueprintPath() = %q, want %q", path, expected)
	}
}

func TestParseBlueprintName(t *testing.T) {
	tests := []struct {
		name       string
		fullName   string
		wantVendor string
		wantName   string
		wantErr    bool
	}{
		{
			name:       "valid name",
			fullName:   "myorg/web-stack",
			wantVendor: "myorg",
			wantName:   "web-stack",
		},
		{
			name:     "no slash",
			fullName: "web-stack",
			wantErr:  true,
		},
		{
			name:       "multiple slashes",
			fullName:   "myorg/sub/web-stack",
			wantVendor: "myorg",
			wantName:   "sub/web-stack",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vendor, name, err := ParseBlueprintName(tt.fullName)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if vendor != tt.wantVendor {
				t.Errorf("vendor = %q, want %q", vendor, tt.wantVendor)
			}
			if name != tt.wantName {
				t.Errorf("name = %q, want %q", name, tt.wantName)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsAt(s, substr, 0))
}

func containsAt(s, substr string, start int) bool {
	for i := start; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
