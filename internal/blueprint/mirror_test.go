package blueprint

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestMirrorConfig_Structure(t *testing.T) {
	config := &MirrorConfig{
		StorageDir:        "/var/lib/keystone-core/blueprints",
		ListenAddr:        ":8080",
		UpstreamURL:       "https://registry.example.com",
		SyncInterval:      time.Hour,
		AllowPush:         true,
		TrustedKeys:       []string{"key1", "key2"},
		RequireSignatures: true,
	}

	if config.StorageDir != "/var/lib/keystone-core/blueprints" {
		t.Errorf("StorageDir = %q, want %q", config.StorageDir, "/var/lib/keystone-core/blueprints")
	}
	if config.ListenAddr != ":8080" {
		t.Errorf("ListenAddr = %q, want %q", config.ListenAddr, ":8080")
	}
	if !config.AllowPush {
		t.Error("AllowPush should be true")
	}
	if len(config.TrustedKeys) != 2 {
		t.Errorf("len(TrustedKeys) = %d, want 2", len(config.TrustedKeys))
	}
}

func TestNewMirrorServer(t *testing.T) {
	tempDir := t.TempDir()
	config := &MirrorConfig{
		StorageDir: tempDir,
		ListenAddr: ":8080",
	}

	server, err := NewMirrorServer(config)
	if err != nil {
		t.Fatalf("NewMirrorServer failed: %v", err)
	}

	if server == nil {
		t.Fatal("NewMirrorServer returned nil")
	}
	if server.config != config {
		t.Error("Config not set correctly")
	}
	if server.index == nil {
		t.Error("Index not initialized")
	}
}

func TestNewMirrorServer_NoStorageDir(t *testing.T) {
	config := &MirrorConfig{
		ListenAddr: ":8080",
	}

	_, err := NewMirrorServer(config)
	if err == nil {
		t.Error("Expected error for missing storage_dir")
	}
}

func TestNewMirrorServer_IndexBuilding(t *testing.T) {
	tempDir := t.TempDir()

	// Create blueprint structure
	bpDir := filepath.Join(tempDir, "myorg", "webapp", "1.0.0")
	if err := os.MkdirAll(bpDir, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to write blueprint: %v", err)
	}

	// Add another version
	bpDir2 := filepath.Join(tempDir, "myorg", "webapp", "2.0.0")
	if err := os.MkdirAll(bpDir2, 0755); err != nil {
		t.Fatalf("Failed to create blueprint dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(bpDir2, "blueprint.yaml"), []byte("test2"), 0644); err != nil {
		t.Fatalf("Failed to write blueprint: %v", err)
	}

	config := &MirrorConfig{
		StorageDir: tempDir,
	}

	server, err := NewMirrorServer(config)
	if err != nil {
		t.Fatalf("NewMirrorServer failed: %v", err)
	}

	// Check index
	versions, exists := server.index["myorg/webapp"]
	if !exists {
		t.Fatal("Blueprint not found in index")
	}
	if len(versions) != 2 {
		t.Errorf("len(versions) = %d, want 2", len(versions))
	}
	if versions[0] != "1.0.0" || versions[1] != "2.0.0" {
		t.Errorf("versions = %v, want [1.0.0, 2.0.0]", versions)
	}
}

func TestMirrorServer_HandleHealth(t *testing.T) {
	tempDir := t.TempDir()
	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	server.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if result["status"] != "healthy" {
		t.Errorf("status = %q, want %q", result["status"], "healthy")
	}
}

func TestMirrorServer_HandleListBlueprints(t *testing.T) {
	tempDir := t.TempDir()

	// Create blueprints
	for _, bp := range []string{"myorg/webapp/1.0.0", "myorg/db/1.0.0", "other/tool/2.0.0"} {
		parts := strings.Split(bp, "/")
		dir := filepath.Join(tempDir, parts[0], parts[1], parts[2])
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "blueprint.yaml"), []byte("test"), 0644)
	}

	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	req := httptest.NewRequest(http.MethodGet, "/v1/blueprints", nil)
	w := httptest.NewRecorder()

	server.handleListBlueprints(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var blueprints []map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &blueprints); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(blueprints) != 3 {
		t.Errorf("len(blueprints) = %d, want 3", len(blueprints))
	}
}

func TestMirrorServer_HandleListBlueprints_MethodNotAllowed(t *testing.T) {
	tempDir := t.TempDir()
	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	req := httptest.NewRequest(http.MethodPost, "/v1/blueprints", nil)
	w := httptest.NewRecorder()

	server.handleListBlueprints(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusMethodNotAllowed)
	}
}

func TestMirrorServer_HandleIndex(t *testing.T) {
	tempDir := t.TempDir()

	bpDir := filepath.Join(tempDir, "test", "bp", "1.0.0")
	os.MkdirAll(bpDir, 0755)
	os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte("test"), 0644)

	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	req := httptest.NewRequest(http.MethodGet, "/v1/index", nil)
	w := httptest.NewRecorder()

	server.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var index map[string][]string
	if err := json.Unmarshal(w.Body.Bytes(), &index); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if len(index) != 1 {
		t.Errorf("len(index) = %d, want 1", len(index))
	}
	if _, exists := index["test/bp"]; !exists {
		t.Error("test/bp not in index")
	}
}

func TestMirrorServer_HandleGetBlueprint_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	req := httptest.NewRequest(http.MethodGet, "/v1/blueprints/nonexistent/bp", nil)
	w := httptest.NewRecorder()

	server.handleBlueprintRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestMirrorServer_HandleGetBlueprint_VersionList(t *testing.T) {
	tempDir := t.TempDir()

	// Create blueprint with multiple versions
	for _, ver := range []string{"1.0.0", "1.1.0", "2.0.0"} {
		dir := filepath.Join(tempDir, "test", "bp", ver)
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "blueprint.yaml"), []byte("test"), 0644)
	}

	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	req := httptest.NewRequest(http.MethodGet, "/v1/blueprints/test/bp", nil)
	w := httptest.NewRecorder()

	server.handleBlueprintRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)

	versions := result["versions"].([]interface{})
	if len(versions) != 3 {
		t.Errorf("len(versions) = %d, want 3", len(versions))
	}
}

func TestMirrorServer_HandleGetBlueprint_SpecificVersion(t *testing.T) {
	tempDir := t.TempDir()

	dir := filepath.Join(tempDir, "test", "bp", "1.0.0")
	os.MkdirAll(dir, 0755)
	os.WriteFile(filepath.Join(dir, "blueprint.yaml"), []byte("metadata:\n  name: test/bp"), 0644)

	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	req := httptest.NewRequest(http.MethodGet, "/v1/blueprints/test/bp/1.0.0", nil)
	w := httptest.NewRecorder()

	server.handleBlueprintRequest(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	if !strings.Contains(w.Body.String(), "metadata:") {
		t.Error("Response should contain blueprint content")
	}
}

func TestMirrorServer_HandlePutBlueprint_NotAllowed(t *testing.T) {
	tempDir := t.TempDir()
	config := &MirrorConfig{
		StorageDir: tempDir,
		AllowPush:  false, // Push not allowed
	}
	server, _ := NewMirrorServer(config)

	body := strings.NewReader("metadata:\n  name: test/bp")
	req := httptest.NewRequest(http.MethodPut, "/v1/blueprints/test/bp/1.0.0", body)
	w := httptest.NewRecorder()

	server.handleBlueprintRequest(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestMirrorServer_HandlePutBlueprint_Success(t *testing.T) {
	tempDir := t.TempDir()
	config := &MirrorConfig{
		StorageDir: tempDir,
		AllowPush:  true,
	}
	server, _ := NewMirrorServer(config)

	body := strings.NewReader("metadata:\n  name: test/bp\n  version: 1.0.0")
	req := httptest.NewRequest(http.MethodPut, "/v1/blueprints/test/bp/1.0.0", body)
	w := httptest.NewRecorder()

	server.handleBlueprintRequest(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusCreated)
	}

	// Verify file was created
	bpPath := filepath.Join(tempDir, "test", "bp", "1.0.0", "blueprint.yaml")
	if _, err := os.Stat(bpPath); os.IsNotExist(err) {
		t.Error("Blueprint file was not created")
	}

	// Verify index was updated
	if _, exists := server.index["test/bp"]; !exists {
		t.Error("Blueprint not in index after push")
	}
}

func TestMirrorServer_HandleGetBundle_NotFound(t *testing.T) {
	tempDir := t.TempDir()
	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	req := httptest.NewRequest(http.MethodGet, "/v1/bundles/test/bp/1.0.0", nil)
	w := httptest.NewRecorder()

	server.handleBundleRequest(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestMirrorServer_HandleBundle_InvalidPath(t *testing.T) {
	tempDir := t.TempDir()
	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	req := httptest.NewRequest(http.MethodGet, "/v1/bundles/test", nil) // Missing parts
	w := httptest.NewRecorder()

	server.handleBundleRequest(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestNewMirrorClient(t *testing.T) {
	client := NewMirrorClient("http://localhost:8080")

	if client == nil {
		t.Fatal("NewMirrorClient returned nil")
	}
	if client.baseURL != "http://localhost:8080" {
		t.Errorf("baseURL = %q, want %q", client.baseURL, "http://localhost:8080")
	}
	if client.httpClient == nil {
		t.Error("httpClient is nil")
	}
}

func TestMirrorClient_ListBlueprints(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/blueprints" {
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"name": "test/bp1", "versions": []string{"1.0.0"}},
				{"name": "test/bp2", "versions": []string{"2.0.0"}},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewMirrorClient(server.URL)
	blueprints, err := client.ListBlueprints()
	if err != nil {
		t.Fatalf("ListBlueprints failed: %v", err)
	}

	if len(blueprints) != 2 {
		t.Errorf("len(blueprints) = %d, want 2", len(blueprints))
	}
}

func TestMirrorClient_GetVersions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/blueprints/test/bp" {
			json.NewEncoder(w).Encode(map[string][]string{
				"versions": {"1.0.0", "2.0.0", "3.0.0"},
			})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewMirrorClient(server.URL)
	versions, err := client.GetVersions("test/bp")
	if err != nil {
		t.Fatalf("GetVersions failed: %v", err)
	}

	if len(versions) != 3 {
		t.Errorf("len(versions) = %d, want 3", len(versions))
	}
}

func TestMirrorClient_GetVersions_InvalidName(t *testing.T) {
	client := NewMirrorClient("http://localhost:8080")
	_, err := client.GetVersions("invalid-name")
	if err == nil {
		t.Error("Expected error for invalid name")
	}
}

func TestMirrorClient_GetBlueprint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/blueprints/test/bp/1.0.0" {
			w.Write([]byte("metadata:\n  name: test/bp"))
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewMirrorClient(server.URL)
	content, err := client.GetBlueprint("test/bp", "1.0.0")
	if err != nil {
		t.Fatalf("GetBlueprint failed: %v", err)
	}

	if !strings.Contains(string(content), "metadata:") {
		t.Error("Content should contain metadata")
	}
}

func TestMirrorClient_UploadBlueprint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/v1/blueprints/test/bp/1.0.0" {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewMirrorClient(server.URL)
	err := client.UploadBlueprint([]byte("metadata:\n  name: test/bp"), "test/bp", "1.0.0")
	if err != nil {
		t.Fatalf("UploadBlueprint failed: %v", err)
	}
}

func TestMirrorClient_UploadBlueprint_PushNotAllowed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	defer server.Close()

	client := NewMirrorClient(server.URL)
	err := client.UploadBlueprint([]byte("test"), "test/bp", "1.0.0")
	if err == nil {
		t.Error("Expected error for forbidden push")
	}
	if !strings.Contains(err.Error(), "push not allowed") {
		t.Errorf("Error should mention push not allowed: %v", err)
	}
}

func TestNewMirrorSyncer(t *testing.T) {
	syncer := NewMirrorSyncer("http://source.example.com", "http://dest.example.com")

	if syncer == nil {
		t.Fatal("NewMirrorSyncer returned nil")
	}
	if syncer.source == nil {
		t.Error("source client is nil")
	}
	if syncer.dest == nil {
		t.Error("dest client is nil")
	}
}

func TestSyncResult_Structure(t *testing.T) {
	result := &SyncResult{
		StartedAt:   time.Now(),
		CompletedAt: time.Now().Add(time.Minute),
		Synced:      10,
		Skipped:     5,
		Failed:      2,
		Errors:      []string{"error1", "error2"},
	}

	if result.Synced != 10 {
		t.Errorf("Synced = %d, want 10", result.Synced)
	}
	if result.Skipped != 5 {
		t.Errorf("Skipped = %d, want 5", result.Skipped)
	}
	if result.Failed != 2 {
		t.Errorf("Failed = %d, want 2", result.Failed)
	}
	if len(result.Errors) != 2 {
		t.Errorf("len(Errors) = %d, want 2", len(result.Errors))
	}
}

func TestMirrorClient_ExportToDirectory(t *testing.T) {
	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/blueprints":
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"Name": "test/bp", "AvailableVersions": []string{"1.0.0"}},
			})
		case "/v1/blueprints/test/bp/1.0.0":
			w.Write([]byte("metadata:\n  name: test/bp"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	tempDir := t.TempDir()
	client := NewMirrorClient(server.URL)

	err := client.ExportToDirectory(tempDir)
	if err != nil {
		t.Fatalf("ExportToDirectory failed: %v", err)
	}

	// Verify export
	bpPath := filepath.Join(tempDir, "test", "bp", "1.0.0", "blueprint.yaml")
	if _, err := os.Stat(bpPath); os.IsNotExist(err) {
		t.Error("Blueprint was not exported")
	}
}

func TestMirrorClient_ImportFromDirectory(t *testing.T) {
	// Create source directory with blueprints
	srcDir := t.TempDir()
	bpDir := filepath.Join(srcDir, "test", "bp", "1.0.0")
	os.MkdirAll(bpDir, 0755)
	os.WriteFile(filepath.Join(bpDir, "blueprint.yaml"), []byte("metadata:\n  name: test/bp"), 0644)

	// Create test server that accepts uploads
	uploaded := make(map[string]bool)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut {
			uploaded[r.URL.Path] = true
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	client := NewMirrorClient(server.URL)
	err := client.ImportFromDirectory(srcDir)
	if err != nil {
		t.Fatalf("ImportFromDirectory failed: %v", err)
	}

	// Verify upload was called
	if !uploaded["/v1/blueprints/test/bp/1.0.0"] {
		t.Error("Blueprint was not uploaded")
	}
}

func TestMirrorServer_Handler(t *testing.T) {
	tempDir := t.TempDir()
	config := &MirrorConfig{StorageDir: tempDir}
	server, _ := NewMirrorServer(config)

	handler := server.Handler()
	if handler == nil {
		t.Fatal("Handler returned nil")
	}

	// Test that handler responds to health endpoint
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Health status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestMirrorClient_DownloadBundle(t *testing.T) {
	// Create test server with bundle
	bundleContent := []byte("fake bundle content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/bundles/test/bp/1.0.0" {
			w.Header().Set("Content-Type", "application/gzip")
			w.Write(bundleContent)
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	tempDir := t.TempDir()
	destPath := filepath.Join(tempDir, "bundle.tar.gz")

	client := NewMirrorClient(server.URL)
	err := client.DownloadBundle("test/bp", "1.0.0", destPath)
	if err != nil {
		t.Fatalf("DownloadBundle failed: %v", err)
	}

	// Verify download
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("Failed to read downloaded bundle: %v", err)
	}
	if string(content) != string(bundleContent) {
		t.Error("Downloaded content doesn't match")
	}
}

func TestMirrorClient_UploadBundle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPut && r.URL.Path == "/v1/bundles/test/bp/1.0.0" {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created"})
			return
		}
		http.NotFound(w, r)
	}))
	defer server.Close()

	// Create temp bundle file
	tempDir := t.TempDir()
	bundlePath := filepath.Join(tempDir, "bundle.tar.gz")
	os.WriteFile(bundlePath, []byte("fake bundle"), 0644)

	client := NewMirrorClient(server.URL)
	err := client.UploadBundle(bundlePath, "test/bp", "1.0.0")
	if err != nil {
		t.Fatalf("UploadBundle failed: %v", err)
	}
}

func TestMirrorSyncer_SyncAll(t *testing.T) {
	// Create source server
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/blueprints":
			json.NewEncoder(w).Encode([]map[string]interface{}{
				{"Name": "test/bp1", "AvailableVersions": []string{"1.0.0"}},
				{"Name": "test/bp2", "AvailableVersions": []string{"1.0.0", "2.0.0"}},
			})
		case "/v1/blueprints/test/bp1/1.0.0", "/v1/blueprints/test/bp2/1.0.0", "/v1/blueprints/test/bp2/2.0.0":
			w.Write([]byte("metadata:\n  name: test"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer sourceServer.Close()

	// Create dest server
	uploaded := 0
	destServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/blueprints" {
			json.NewEncoder(w).Encode([]map[string]interface{}{}) // Empty
			return
		}
		if r.Method == http.MethodPut {
			uploaded++
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created"})
			return
		}
		http.NotFound(w, r)
	}))
	defer destServer.Close()

	syncer := NewMirrorSyncer(sourceServer.URL, destServer.URL)
	result, err := syncer.SyncAll()
	if err != nil {
		t.Fatalf("SyncAll failed: %v", err)
	}

	if result.Synced != 3 {
		t.Errorf("Synced = %d, want 3", result.Synced)
	}
	if uploaded != 3 {
		t.Errorf("uploaded = %d, want 3", uploaded)
	}
}

func TestMirrorSyncer_SyncBlueprint(t *testing.T) {
	// Create source server
	sourceServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/blueprints/test/bp":
			json.NewEncoder(w).Encode(map[string][]string{
				"versions": {"1.0.0", "2.0.0"},
			})
		case "/v1/blueprints/test/bp/1.0.0", "/v1/blueprints/test/bp/2.0.0":
			w.Write([]byte("metadata:\n  name: test/bp"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer sourceServer.Close()

	// Create dest server (already has 1.0.0)
	destServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/blueprints/test/bp" && r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(map[string][]string{
				"versions": {"1.0.0"}, // Already has 1.0.0
			})
			return
		}
		if r.Method == http.MethodPut {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{"status": "created"})
			return
		}
		http.NotFound(w, r)
	}))
	defer destServer.Close()

	syncer := NewMirrorSyncer(sourceServer.URL, destServer.URL)
	result, err := syncer.SyncBlueprint("test/bp")
	if err != nil {
		t.Fatalf("SyncBlueprint failed: %v", err)
	}

	if result.Synced != 1 {
		t.Errorf("Synced = %d, want 1 (only 2.0.0)", result.Synced)
	}
	if result.Skipped != 1 {
		t.Errorf("Skipped = %d, want 1 (1.0.0 already exists)", result.Skipped)
	}
}
