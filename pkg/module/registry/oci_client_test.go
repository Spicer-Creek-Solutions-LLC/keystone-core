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
)

func TestOCIClient_Ping(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
		wantErr    bool
	}{
		{
			name:       "successful ping",
			statusCode: http.StatusOK,
			wantErr:    false,
		},
		{
			name:       "unauthorized",
			statusCode: http.StatusUnauthorized,
			wantErr:    true,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v2/" {
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
				w.WriteHeader(tt.statusCode)
			}))
			defer server.Close()

			config := &OCIRegistryConfig{
				Registry:  strings.TrimPrefix(server.URL, "http://"),
				PlainHTTP: true,
			}
			client, err := NewOCIClient(config)
			if err != nil {
				t.Fatalf("NewOCIClient failed: %v", err)
			}

			err = client.Ping()
			if (err != nil) != tt.wantErr {
				t.Errorf("Ping() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestOCIClient_ListTags(t *testing.T) {
	tests := []struct {
		name       string
		moduleName string
		tags       []string
		statusCode int
		wantTags   []string
		wantErr    bool
	}{
		{
			name:       "list tags",
			moduleName: "mymodule",
			tags:       []string{"1.0.0", "1.1.0", "2.0.0"},
			statusCode: http.StatusOK,
			wantTags:   []string{"1.0.0", "1.1.0", "2.0.0"},
			wantErr:    false,
		},
		{
			name:       "module not found",
			moduleName: "nonexistent",
			statusCode: http.StatusNotFound,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tt.statusCode == http.StatusNotFound {
					w.WriteHeader(http.StatusNotFound)
					return
				}

				resp := OCITagsList{
					Name: tt.moduleName,
					Tags: tt.tags,
				}
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(resp)
			}))
			defer server.Close()

			config := &OCIRegistryConfig{
				Registry:  strings.TrimPrefix(server.URL, "http://"),
				PlainHTTP: true,
			}
			client, err := NewOCIClient(config)
			if err != nil {
				t.Fatalf("NewOCIClient failed: %v", err)
			}

			tags, err := client.ListTags(tt.moduleName)
			if (err != nil) != tt.wantErr {
				t.Errorf("ListTags() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(tags) != len(tt.wantTags) {
					t.Errorf("ListTags() got %d tags, want %d", len(tags), len(tt.wantTags))
				}
			}
		})
	}
}

func TestOCIClient_GetManifest(t *testing.T) {
	manifest := OCIManifest{
		SchemaVersion: 2,
		MediaType:     OCIManifestMediaType,
		ArtifactType:  KscoreModuleMediaType,
		Config: OCIDescriptor{
			MediaType: OCIConfigMediaType,
			Digest:    "sha256:abc123",
			Size:      100,
		},
		Layers: []OCIDescriptor{
			{
				MediaType: KscoreModuleMediaType,
				Digest:    "sha256:module123",
				Size:      1000,
			},
			{
				MediaType: KscoreManifestMediaType,
				Digest:    "sha256:manifest123",
				Size:      200,
			},
		},
		Annotations: map[string]string{
			"io.kscore.module.name":    "test/module",
			"io.kscore.module.version": "1.0.0",
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v2/") {
			t.Errorf("unexpected path: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		w.Header().Set("Content-Type", OCIManifestMediaType)
		w.Header().Set("Docker-Content-Digest", "sha256:manifestdigest")
		json.NewEncoder(w).Encode(manifest)
	}))
	defer server.Close()

	config := &OCIRegistryConfig{
		Registry:  strings.TrimPrefix(server.URL, "http://"),
		PlainHTTP: true,
	}
	client, err := NewOCIClient(config)
	if err != nil {
		t.Fatalf("NewOCIClient failed: %v", err)
	}

	got, err := client.GetManifest("test/module", "1.0.0")
	if err != nil {
		t.Fatalf("GetManifest() error = %v", err)
	}

	if got.SchemaVersion != 2 {
		t.Errorf("GetManifest() SchemaVersion = %d, want 2", got.SchemaVersion)
	}
	if got.ArtifactType != KscoreModuleMediaType {
		t.Errorf("GetManifest() ArtifactType = %s, want %s", got.ArtifactType, KscoreModuleMediaType)
	}
	if len(got.Layers) != 2 {
		t.Errorf("GetManifest() got %d layers, want 2", len(got.Layers))
	}
}

func TestOCIClient_Push(t *testing.T) {
	// Track what was uploaded
	var uploadedBlobs []string
	var pushedManifest *OCIManifest

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Check blob existence
		if r.Method == "HEAD" && strings.Contains(path, "/blobs/") {
			w.WriteHeader(http.StatusNotFound) // Blob doesn't exist
			return
		}

		// Start upload
		if r.Method == "POST" && strings.HasSuffix(path, "/blobs/uploads/") {
			w.Header().Set("Location", "/v2/test/mymodule/blobs/uploads/uuid-123")
			w.WriteHeader(http.StatusAccepted)
			return
		}

		// Complete upload
		if r.Method == "PUT" && strings.Contains(path, "/blobs/uploads/") {
			digest := r.URL.Query().Get("digest")
			uploadedBlobs = append(uploadedBlobs, digest)
			w.WriteHeader(http.StatusCreated)
			return
		}

		// Push manifest
		if r.Method == "PUT" && strings.Contains(path, "/manifests/") {
			body, _ := io.ReadAll(r.Body)
			var m OCIManifest
			json.Unmarshal(body, &m)
			pushedManifest = &m
			w.Header().Set("Docker-Content-Digest", "sha256:manifestdigest")
			w.WriteHeader(http.StatusCreated)
			return
		}

		t.Logf("Unhandled request: %s %s", r.Method, path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create temp files
	tmpDir, err := os.MkdirTemp("", "oci-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	modulePath := filepath.Join(tmpDir, "module.zip")
	if err := os.WriteFile(modulePath, []byte("module content"), 0644); err != nil {
		t.Fatal(err)
	}

	manifestPath := filepath.Join(tmpDir, "module.yaml")
	if err := os.WriteFile(manifestPath, []byte("name: test/mymodule\nversion: 1.0.0\n"), 0644); err != nil {
		t.Fatal(err)
	}

	config := &OCIRegistryConfig{
		Registry:  strings.TrimPrefix(server.URL, "http://"),
		Namespace: "test",
		PlainHTTP: true,
	}
	client, err := NewOCIClient(config)
	if err != nil {
		t.Fatalf("NewOCIClient failed: %v", err)
	}

	result, err := client.Push(&OCIPushRequest{
		ModulePath:   modulePath,
		ManifestPath: manifestPath,
		ModuleName:   "mymodule",
		Version:      "1.0.0",
	})

	if err != nil {
		t.Fatalf("Push() error = %v", err)
	}

	// Verify result
	if result.ModuleName != "mymodule" {
		t.Errorf("Push() ModuleName = %s, want mymodule", result.ModuleName)
	}
	if result.Version != "1.0.0" {
		t.Errorf("Push() Version = %s, want 1.0.0", result.Version)
	}
	if result.Digest != "sha256:manifestdigest" {
		t.Errorf("Push() Digest = %s, want sha256:manifestdigest", result.Digest)
	}

	// Verify blobs were uploaded (module, manifest, config = 3 blobs)
	if len(uploadedBlobs) != 3 {
		t.Errorf("Push() uploaded %d blobs, want 3", len(uploadedBlobs))
	}

	// Verify manifest was pushed
	if pushedManifest == nil {
		t.Fatal("Push() manifest was not pushed")
	}
	if pushedManifest.SchemaVersion != 2 {
		t.Errorf("Push() manifest SchemaVersion = %d, want 2", pushedManifest.SchemaVersion)
	}
	if len(pushedManifest.Layers) != 2 {
		t.Errorf("Push() manifest has %d layers, want 2", len(pushedManifest.Layers))
	}
}

func TestOCIClient_Pull(t *testing.T) {
	moduleContent := []byte("module zip content")
	manifestContent := []byte("name: test/mymodule\nversion: 1.0.0\n")

	manifest := OCIManifest{
		SchemaVersion: 2,
		MediaType:     OCIManifestMediaType,
		Layers: []OCIDescriptor{
			{
				MediaType: KscoreModuleMediaType,
				Digest:    "sha256:moduledigest",
				Size:      int64(len(moduleContent)),
			},
			{
				MediaType: KscoreManifestMediaType,
				Digest:    "sha256:manifestdigest",
				Size:      int64(len(manifestContent)),
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Get manifest
		if r.Method == "GET" && strings.Contains(path, "/manifests/") {
			w.Header().Set("Content-Type", OCIManifestMediaType)
			w.Header().Set("Docker-Content-Digest", "sha256:fullmanifest")
			json.NewEncoder(w).Encode(manifest)
			return
		}

		// Get blob
		if r.Method == "GET" && strings.Contains(path, "/blobs/") {
			if strings.Contains(path, "moduledigest") {
				w.Write(moduleContent)
			} else if strings.Contains(path, "manifestdigest") {
				w.Write(manifestContent)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "oci-pull-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	config := &OCIRegistryConfig{
		Registry:  strings.TrimPrefix(server.URL, "http://"),
		Namespace: "test",
		PlainHTTP: true,
	}
	client, err := NewOCIClient(config)
	if err != nil {
		t.Fatalf("NewOCIClient failed: %v", err)
	}

	result, err := client.Pull("mymodule", "1.0.0", tmpDir)
	if err != nil {
		t.Fatalf("Pull() error = %v", err)
	}

	// Verify result
	if result.Digest != "sha256:fullmanifest" {
		t.Errorf("Pull() Digest = %s, want sha256:fullmanifest", result.Digest)
	}

	// Verify files were downloaded
	if result.ModulePath == "" {
		t.Error("Pull() ModulePath is empty")
	}
	if result.ManifestPath == "" {
		t.Error("Pull() ManifestPath is empty")
	}

	// Verify content
	got, err := os.ReadFile(result.ModulePath)
	if err != nil {
		t.Fatalf("failed to read module: %v", err)
	}
	if string(got) != string(moduleContent) {
		t.Errorf("Pull() module content = %s, want %s", got, moduleContent)
	}

	got, err = os.ReadFile(result.ManifestPath)
	if err != nil {
		t.Fatalf("failed to read manifest: %v", err)
	}
	if string(got) != string(manifestContent) {
		t.Errorf("Pull() manifest content = %s, want %s", got, manifestContent)
	}
}

func TestOCIClient_Delete(t *testing.T) {
	deleted := false

	manifest := OCIManifest{
		SchemaVersion: 2,
		MediaType:     OCIManifestMediaType,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Get manifest
		if r.Method == "GET" && strings.Contains(path, "/manifests/") {
			w.Header().Set("Docker-Content-Digest", "sha256:todelete")
			json.NewEncoder(w).Encode(manifest)
			return
		}

		// Delete manifest
		if r.Method == "DELETE" && strings.Contains(path, "/manifests/") {
			deleted = true
			w.WriteHeader(http.StatusAccepted)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	config := &OCIRegistryConfig{
		Registry:  strings.TrimPrefix(server.URL, "http://"),
		Namespace: "test",
		PlainHTTP: true,
	}
	client, err := NewOCIClient(config)
	if err != nil {
		t.Fatalf("NewOCIClient failed: %v", err)
	}

	err = client.Delete("mymodule", "1.0.0")
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if !deleted {
		t.Error("Delete() did not send delete request")
	}
}

func TestOCIClient_Auth(t *testing.T) {
	var receivedAuth string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	tests := []struct {
		name     string
		auth     *AuthConfig
		wantAuth string
	}{
		{
			name:     "no auth",
			auth:     nil,
			wantAuth: "",
		},
		{
			name: "basic auth",
			auth: &AuthConfig{
				Type:     AuthTypeBasic,
				Username: "user",
				Password: "pass",
			},
			wantAuth: "Basic dXNlcjpwYXNz", // base64(user:pass)
		},
		{
			name: "bearer token",
			auth: &AuthConfig{
				Type:  AuthTypeBearer,
				Token: "mytoken",
			},
			wantAuth: "Bearer mytoken",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			receivedAuth = ""

			config := &OCIRegistryConfig{
				Registry:  strings.TrimPrefix(server.URL, "http://"),
				PlainHTTP: true,
				Auth:      tt.auth,
			}
			client, err := NewOCIClient(config)
			if err != nil {
				t.Fatalf("NewOCIClient failed: %v", err)
			}

			client.Ping()

			if receivedAuth != tt.wantAuth {
				t.Errorf("Auth header = %s, want %s", receivedAuth, tt.wantAuth)
			}
		})
	}
}

func TestOCIDescriptor_MediaTypes(t *testing.T) {
	// Verify media type constants are properly defined
	if OCIManifestMediaType != "application/vnd.oci.image.manifest.v1+json" {
		t.Errorf("OCIManifestMediaType = %s, want application/vnd.oci.image.manifest.v1+json", OCIManifestMediaType)
	}
	if KscoreModuleMediaType != "application/vnd.kscore.module.v1+zip" {
		t.Errorf("KscoreModuleMediaType = %s, want application/vnd.kscore.module.v1+zip", KscoreModuleMediaType)
	}
	if KscoreManifestMediaType != "application/vnd.kscore.module.manifest.v1+yaml" {
		t.Errorf("KscoreManifestMediaType = %s, want application/vnd.kscore.module.manifest.v1+yaml", KscoreManifestMediaType)
	}
}

func TestDefaultOCIRegistryConfig(t *testing.T) {
	config := DefaultOCIRegistryConfig("ghcr.io", "myorg")

	if config.Registry != "ghcr.io" {
		t.Errorf("Registry = %s, want ghcr.io", config.Registry)
	}
	if config.Namespace != "myorg" {
		t.Errorf("Namespace = %s, want myorg", config.Namespace)
	}
	if config.Timeout == 0 {
		t.Error("Timeout should not be zero")
	}
}
