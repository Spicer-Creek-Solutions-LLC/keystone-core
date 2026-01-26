package registry

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
)

func TestHTTPClient_ListVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/myorg/mymodule/@v/list" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("1.0.0\n1.1.0\n2.0.0\n"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewHTTPClient(DefaultRegistryConfig(server.URL))
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	versions, err := client.ListVersions("myorg/mymodule")
	if err != nil {
		t.Fatalf("ListVersions failed: %v", err)
	}

	if len(versions) != 3 {
		t.Errorf("expected 3 versions, got %d", len(versions))
	}
	if versions[0] != "1.0.0" {
		t.Errorf("expected first version 1.0.0, got %s", versions[0])
	}
}

func TestHTTPClient_ListVersions_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewHTTPClient(DefaultRegistryConfig(server.URL))
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	_, err = client.ListVersions("nonexistent/module")
	if err == nil {
		t.Fatal("expected error for nonexistent module")
	}
	if !IsNotFoundError(err) {
		t.Errorf("expected not found error, got: %v", err)
	}
}

func TestHTTPClient_GetModuleInfo(t *testing.T) {
	info := &resolver.ModuleInfo{
		Name:        "myorg/mymodule",
		Version:     "1.0.0",
		Hash:        "sha256:abc123",
		PublishedAt: time.Now(),
		Description: "Test module",
		Size:        1024,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/myorg/mymodule/@v/1.0.0.info" {
			json.NewEncoder(w).Encode(info)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewHTTPClient(DefaultRegistryConfig(server.URL))
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	result, err := client.GetModuleInfo("myorg/mymodule", "1.0.0")
	if err != nil {
		t.Fatalf("GetModuleInfo failed: %v", err)
	}

	if result.Name != info.Name {
		t.Errorf("expected name %s, got %s", info.Name, result.Name)
	}
	if result.Version != info.Version {
		t.Errorf("expected version %s, got %s", info.Version, result.Version)
	}
}

func TestHTTPClient_GetModuleManifest(t *testing.T) {
	manifestYAML := `name: myorg/mymodule
version: 1.0.0
type: starlark
entrypoint: main.star`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/myorg/mymodule/@v/1.0.0.mod" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(manifestYAML))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewHTTPClient(DefaultRegistryConfig(server.URL))
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	m, err := client.GetModuleManifest("myorg/mymodule", "1.0.0")
	if err != nil {
		t.Fatalf("GetModuleManifest failed: %v", err)
	}

	if m.Name != "myorg/mymodule" {
		t.Errorf("expected name myorg/mymodule, got %s", m.Name)
	}
	if m.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", m.Version)
	}
}

func TestHTTPClient_DownloadModule(t *testing.T) {
	moduleContent := []byte("fake zip content")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/myorg/mymodule/@v/1.0.0.zip" {
			w.WriteHeader(http.StatusOK)
			w.Write(moduleContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	tmpDir, err := os.MkdirTemp("", "registry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	destPath := filepath.Join(tmpDir, "module.zip")
	client, err := NewHTTPClient(DefaultRegistryConfig(server.URL))
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	err = client.DownloadModule("myorg/mymodule", "1.0.0", destPath)
	if err != nil {
		t.Fatalf("DownloadModule failed: %v", err)
	}

	// Verify file was downloaded
	downloaded, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("failed to read downloaded file: %v", err)
	}
	if string(downloaded) != string(moduleContent) {
		t.Errorf("downloaded content mismatch")
	}
}

func TestHTTPClient_PublishModule(t *testing.T) {
	var receivedRequest *http.Request
	var receivedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequest = r
		receivedBody, _ = io.ReadAll(r.Body)
		if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/myorg/mymodule/@v/1.0.0") {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(PublishResult{
				ModuleName:  "myorg/mymodule",
				Version:     "1.0.0",
				Hash:        "sha256:abc123",
				URL:         "https://registry.example.com/myorg/mymodule/@v/1.0.0.zip",
				PublishedAt: time.Now(),
				Size:        100,
			})
			return
		}
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	// Create a test module file
	tmpDir, err := os.MkdirTemp("", "registry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("fake module content"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	client, err := NewHTTPClient(DefaultRegistryConfig(server.URL))
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	result, err := client.PublishModule(&PublishRequest{
		ModulePath: modulePath,
		Manifest: &manifest.Manifest{
			Name:       "myorg/mymodule",
			Version:    "1.0.0",
			Type:       "starlark",
			Entrypoint: "main.star",
		},
		Hash:  "sha256:abcdef1234567890", // Provide hash directly to skip computation
		Force: false,
	})
	if err != nil {
		t.Fatalf("PublishModule failed: %v", err)
	}

	if receivedRequest.Method != "POST" {
		t.Errorf("expected POST request, got %s", receivedRequest.Method)
	}
	if result.ModuleName != "myorg/mymodule" {
		t.Errorf("expected module name myorg/mymodule, got %s", result.ModuleName)
	}
	if result.Version != "1.0.0" {
		t.Errorf("expected version 1.0.0, got %s", result.Version)
	}
	if !strings.Contains(string(receivedBody), "fake module content") {
		t.Error("request body should contain module content")
	}
}

func TestHTTPClient_PublishModule_AlreadyExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]string{
			"code":    ErrCodeVersionExists,
			"message": "version already exists",
		})
	}))
	defer server.Close()

	tmpDir, err := os.MkdirTemp("", "registry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	client, err := NewHTTPClient(DefaultRegistryConfig(server.URL))
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	_, err = client.PublishModule(&PublishRequest{
		ModulePath: modulePath,
		Manifest: &manifest.Manifest{
			Name:       "myorg/mymodule",
			Version:    "1.0.0",
			Type:       "starlark",
			Entrypoint: "main.star",
		},
		Hash: "sha256:abcdef1234567890", // Provide hash directly to skip computation
	})
	if err == nil {
		t.Fatal("expected error for existing version")
	}
	if !IsVersionExistsError(err) {
		t.Errorf("expected version exists error, got: %v", err)
	}
}

func TestHTTPClient_DeleteModule(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" && r.URL.Path == "/myorg/mymodule/@v/1.0.0" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, err := NewHTTPClient(DefaultRegistryConfig(server.URL))
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	err = client.DeleteModule("myorg/mymodule", "1.0.0")
	if err != nil {
		t.Fatalf("DeleteModule failed: %v", err)
	}
}

func TestHTTPClient_Authentication_Bearer(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1.0.0\n"))
	}))
	defer server.Close()

	config := DefaultRegistryConfig(server.URL)
	config.Auth = &AuthConfig{
		Type:  AuthTypeBearer,
		Token: "test-token-123",
	}
	client, err := NewHTTPClient(config)
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	_, err = client.ListVersions("myorg/mymodule")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if receivedAuth != "Bearer test-token-123" {
		t.Errorf("expected Bearer auth header, got: %s", receivedAuth)
	}
}

func TestHTTPClient_Authentication_Basic(t *testing.T) {
	var receivedAuth string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("1.0.0\n"))
	}))
	defer server.Close()

	config := DefaultRegistryConfig(server.URL)
	config.Auth = &AuthConfig{
		Type:     AuthTypeBasic,
		Username: "user",
		Password: "pass",
	}
	client, err := NewHTTPClient(config)
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}
	_, err = client.ListVersions("myorg/mymodule")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}

	if !strings.HasPrefix(receivedAuth, "Basic ") {
		t.Errorf("expected Basic auth header, got: %s", receivedAuth)
	}
}

func TestHTTPClient_RetryOnServerError(t *testing.T) {
	attempts := 0

	// Create temp module file
	tmpDir, err := os.MkdirTemp("", "registry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts < 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(PublishResult{
			ModuleName: "myorg/mymodule",
			Version:    "1.0.0",
		})
	}))
	defer server.Close()

	config := DefaultRegistryConfig(server.URL)
	config.RetryAttempts = 3
	config.RetryDelay = 10 * time.Millisecond
	client, err := NewHTTPClient(config)
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}

	// PublishModule uses retry logic
	_, err = client.PublishModule(&PublishRequest{
		ModulePath: modulePath,
		Manifest: &manifest.Manifest{
			Name:       "myorg/mymodule",
			Version:    "1.0.0",
			Type:       "starlark",
			Entrypoint: "main.star",
		},
		Hash: "sha256:test",
	})
	if err != nil {
		t.Fatalf("request failed after retries: %v", err)
	}

	if attempts != 3 {
		t.Errorf("expected 3 attempts, got %d", attempts)
	}
}

func TestHTTPClient_RetryRespectsTimeout(t *testing.T) {
	// Create temp module file
	tmpDir, err := os.MkdirTemp("", "registry-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "test-module.zip")
	if err := os.WriteFile(modulePath, []byte("content"), 0644); err != nil {
		t.Fatalf("failed to write test module: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultRegistryConfig(server.URL)
	config.RetryAttempts = 3
	config.RetryDelay = time.Second
	config.Timeout = 50 * time.Millisecond

	client, err := NewHTTPClient(config)
	if err != nil {
		t.Fatalf("NewHTTPClient failed: %v", err)
	}

	start := time.Now()
	_, err = client.PublishModule(&PublishRequest{
		ModulePath: modulePath,
		Manifest: &manifest.Manifest{
			Name:       "myorg/mymodule",
			Version:    "1.0.0",
			Type:       "starlark",
			Entrypoint: "main.star",
		},
		Hash: "sha256:test",
	})
	if err == nil {
		t.Fatal("expected error from timeout")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Fatal("expected retry delay to be bounded by timeout")
	}
}

func TestRegistryError_Error(t *testing.T) {
	tests := []struct {
		name     string
		err      *RegistryError
		expected string
	}{
		{
			name:     "with code",
			err:      &RegistryError{Code: "TEST_CODE", Message: "test message"},
			expected: "TEST_CODE: test message",
		},
		{
			name:     "without code",
			err:      &RegistryError{Message: "test message"},
			expected: "test message",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.err.Error() != tt.expected {
				t.Errorf("expected %q, got %q", tt.expected, tt.err.Error())
			}
		})
	}
}

func TestIsNotFoundError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "404 status",
			err:      &RegistryError{StatusCode: 404},
			expected: true,
		},
		{
			name:     "module not found code",
			err:      &RegistryError{Code: ErrCodeModuleNotFound},
			expected: true,
		},
		{
			name:     "version not found code",
			err:      &RegistryError{Code: ErrCodeVersionNotFound},
			expected: true,
		},
		{
			name:     "other error",
			err:      &RegistryError{StatusCode: 500},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsNotFoundError(tt.err) != tt.expected {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}

func TestIsAuthError(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		expected bool
	}{
		{
			name:     "401 status",
			err:      &RegistryError{StatusCode: 401},
			expected: true,
		},
		{
			name:     "403 status",
			err:      &RegistryError{StatusCode: 403},
			expected: true,
		},
		{
			name:     "unauthorized code",
			err:      &RegistryError{Code: ErrCodeUnauthorized},
			expected: true,
		},
		{
			name:     "forbidden code",
			err:      &RegistryError{Code: ErrCodeForbidden},
			expected: true,
		},
		{
			name:     "other error",
			err:      &RegistryError{StatusCode: 500},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if IsAuthError(tt.err) != tt.expected {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}
