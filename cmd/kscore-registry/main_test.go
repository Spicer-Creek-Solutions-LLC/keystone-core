package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/module/registry"
)

// createTestZip creates a minimal valid ZIP file
func createTestZip(content string) []byte {
	buf := &bytes.Buffer{}
	w := zip.NewWriter(buf)
	f, _ := w.Create("content.txt")
	f.Write([]byte(content))
	w.Close()
	return buf.Bytes()
}

func TestServer_Health(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{
		DataDir: tmpDir,
	})

	req := httptest.NewRequest("GET", "/health", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Health check returned %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["status"] != "healthy" {
		t.Errorf("Health status = %v, want healthy", resp["status"])
	}
}

func TestServer_Index(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{
		DataDir:  tmpDir,
		ReadOnly: true,
	})

	req := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Index returned %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp["name"] != "kscore-registry" {
		t.Errorf("Name = %v, want kscore-registry", resp["name"])
	}
	if resp["readonly"] != true {
		t.Errorf("Readonly = %v, want true", resp["readonly"])
	}
}

func TestServer_ListVersions(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create module versions
	for _, v := range []string{"1.0.0", "1.1.0", "2.0.0"} {
		versionDir := filepath.Join(tmpDir, "test", "mymodule", v)
		if err := os.MkdirAll(versionDir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(versionDir, "module.zip"), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	server := NewServer(Config{DataDir: tmpDir})

	req := httptest.NewRequest("GET", "/test/mymodule/@v/list", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("ListVersions returned %d, want %d", rec.Code, http.StatusOK)
	}

	body := strings.TrimSpace(rec.Body.String())
	versions := strings.Split(body, "\n")
	if len(versions) != 3 {
		t.Errorf("ListVersions returned %d versions, want 3", len(versions))
	}
}

func TestServer_ListVersions_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{DataDir: tmpDir})

	req := httptest.NewRequest("GET", "/nonexistent/@v/list", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("ListVersions returned %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServer_GetInfo(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create module with info
	versionDir := filepath.Join(tmpDir, "test", "mymodule", "1.0.0")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}

	info := StoredModule{
		Name:        "test/mymodule",
		Version:     "1.0.0",
		Hash:        "sha256:abc123",
		PublishedAt: time.Now(),
		Size:        1000,
		Description: "Test module",
	}
	infoData, _ := json.Marshal(info)
	if err := os.WriteFile(filepath.Join(versionDir, "module.info"), infoData, 0644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(Config{DataDir: tmpDir})

	req := httptest.NewRequest("GET", "/test/mymodule/@v/1.0.0.info", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GetInfo returned %d, want %d", rec.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Note: resolver.ModuleInfo uses capitalized field names in JSON
	if resp["Name"] != "test/mymodule" {
		t.Errorf("Name = %v, want test/mymodule", resp["Name"])
	}
	if resp["Version"] != "1.0.0" {
		t.Errorf("Version = %v, want 1.0.0", resp["Version"])
	}
}

func TestServer_GetManifest(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create module with manifest
	versionDir := filepath.Join(tmpDir, "test", "mymodule", "1.0.0")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}

	manifest := "name: test/mymodule\nversion: 1.0.0\ndescription: Test module\n"
	if err := os.WriteFile(filepath.Join(versionDir, "module.yaml"), []byte(manifest), 0644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(Config{DataDir: tmpDir})

	req := httptest.NewRequest("GET", "/test/mymodule/@v/1.0.0.mod", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("GetManifest returned %d, want %d", rec.Code, http.StatusOK)
	}

	if rec.Body.String() != manifest {
		t.Errorf("GetManifest returned %s, want %s", rec.Body.String(), manifest)
	}
}

func TestServer_Download(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create module
	versionDir := filepath.Join(tmpDir, "test", "mymodule", "1.0.0")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}

	content := []byte("module zip content")
	if err := os.WriteFile(filepath.Join(versionDir, "module.zip"), content, 0644); err != nil {
		t.Fatal(err)
	}

	server := NewServer(Config{DataDir: tmpDir})

	req := httptest.NewRequest("GET", "/test/mymodule/@v/1.0.0.zip", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("Download returned %d, want %d", rec.Code, http.StatusOK)
	}

	if rec.Header().Get("Content-Type") != "application/zip" {
		t.Errorf("Content-Type = %s, want application/zip", rec.Header().Get("Content-Type"))
	}

	if rec.Body.String() != string(content) {
		t.Errorf("Download returned %s, want %s", rec.Body.String(), string(content))
	}
}

func TestServer_Publish(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{DataDir: tmpDir})

	// Create multipart form with valid ZIP
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add module file (valid ZIP)
	part, _ := writer.CreateFormFile("module", "module.zip")
	part.Write(createTestZip("module content"))

	// Add manifest (hash will be computed by server)
	writer.WriteField("manifest", "name: test/newmodule\nversion: 1.0.0\n")

	writer.Close()

	req := httptest.NewRequest("POST", "/test/newmodule/@v/1.0.0", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Publish returned %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	var resp registry.PublishResult
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	if resp.ModuleName != "test/newmodule" {
		t.Errorf("ModuleName = %s, want test/newmodule", resp.ModuleName)
	}
	if resp.Version != "1.0.0" {
		t.Errorf("Version = %s, want 1.0.0", resp.Version)
	}

	// Verify files were created
	if _, err := os.Stat(filepath.Join(tmpDir, "test/newmodule", "1.0.0", "module.zip")); err != nil {
		t.Errorf("Module file not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "test/newmodule", "1.0.0", "module.yaml")); err != nil {
		t.Errorf("Manifest file not created: %v", err)
	}
}

func TestServer_Publish_Conflict(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing version
	versionDir := filepath.Join(tmpDir, "test", "existing", "1.0.0")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(versionDir, "module.zip"), []byte("existing"), 0644)

	server := NewServer(Config{DataDir: tmpDir})

	// Try to publish same version
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("module", "module.zip")
	part.Write([]byte("new content"))
	writer.WriteField("manifest", "name: test/existing\nversion: 1.0.0\n")
	writer.Close()

	req := httptest.NewRequest("POST", "/test/existing/@v/1.0.0", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusConflict {
		t.Errorf("Publish returned %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestServer_Publish_Force(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create existing version with valid ZIP
	versionDir := filepath.Join(tmpDir, "test", "existing", "1.0.0")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(versionDir, "module.zip"), createTestZip("old"), 0644)

	server := NewServer(Config{DataDir: tmpDir})

	// Force overwrite with valid ZIP
	newZip := createTestZip("new content")
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("module", "module.zip")
	part.Write(newZip)
	writer.WriteField("manifest", "name: test/existing\nversion: 1.0.0\n")
	writer.WriteField("force", "true")
	writer.Close()

	req := httptest.NewRequest("POST", "/test/existing/@v/1.0.0", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Publish with force returned %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// Verify content was updated (compare size since ZIP content varies)
	content, _ := os.ReadFile(filepath.Join(versionDir, "module.zip"))
	if len(content) != len(newZip) {
		t.Errorf("Content length = %d, want %d", len(content), len(newZip))
	}
}

func TestServer_Publish_ReadOnly(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{
		DataDir:  tmpDir,
		ReadOnly: true,
	})

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("module", "module.zip")
	part.Write(createTestZip("content"))
	writer.WriteField("manifest", "name: test/mod\nversion: 1.0.0\n")
	writer.Close()

	req := httptest.NewRequest("POST", "/test/mod/@v/1.0.0", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Errorf("Publish in readonly mode returned %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestServer_Publish_AuthRequired(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{
		DataDir: tmpDir,
		APIKey:  "secret-key",
	})

	zipContent := createTestZip("content")

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("module", "module.zip")
	part.Write(zipContent)
	writer.WriteField("manifest", "name: test/mod\nversion: 1.0.0\n")
	writer.Close()

	// Without API key
	req := httptest.NewRequest("POST", "/test/mod/@v/1.0.0", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Publish without auth returned %d, want %d", rec.Code, http.StatusUnauthorized)
	}

	// With API key
	body = &bytes.Buffer{}
	writer = multipart.NewWriter(body)
	part, _ = writer.CreateFormFile("module", "module.zip")
	part.Write(zipContent)
	writer.WriteField("manifest", "name: test/mod\nversion: 1.0.0\n")
	writer.Close()

	req = httptest.NewRequest("POST", "/test/mod/@v/1.0.0", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-API-Key", "secret-key")
	rec = httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Publish with auth returned %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestServer_Delete(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create module
	versionDir := filepath.Join(tmpDir, "test", "todelete", "1.0.0")
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(versionDir, "module.zip"), []byte("content"), 0644)

	server := NewServer(Config{DataDir: tmpDir})

	req := httptest.NewRequest("DELETE", "/test/todelete/@v/1.0.0", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("Delete returned %d, want %d", rec.Code, http.StatusNoContent)
	}

	// Verify deleted
	if _, err := os.Stat(versionDir); !os.IsNotExist(err) {
		t.Error("Version directory was not deleted")
	}
}

func TestServer_Delete_NotFound(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{DataDir: tmpDir})

	req := httptest.NewRequest("DELETE", "/nonexistent/@v/1.0.0", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Delete returned %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServer_CORS(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{
		DataDir:     tmpDir,
		EnableCORS:  true,
		CORSOrigins: "https://example.com",
	})

	// Test OPTIONS preflight with Origin header
	req := httptest.NewRequest("OPTIONS", "/test/mod/@v/list", nil)
	req.Header.Set("Origin", "https://example.com")
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("CORS preflight returned %d, want %d", rec.Code, http.StatusOK)
	}

	if rec.Header().Get("Access-Control-Allow-Origin") != "https://example.com" {
		t.Errorf("CORS header not set correctly, got %q", rec.Header().Get("Access-Control-Allow-Origin"))
	}
}

func TestServer_InvalidPath(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{DataDir: tmpDir})

	// Path without /@v/
	req := httptest.NewRequest("GET", "/invalid/path/here", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("Invalid path returned %d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestServer_SignatureHandling(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "registry-test")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	server := NewServer(Config{DataDir: tmpDir})

	// Publish with signature using valid ZIP
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, _ := writer.CreateFormFile("module", "module.zip")
	part.Write(createTestZip("module content"))
	writer.WriteField("manifest", "name: test/signed\nversion: 1.0.0\n")
	writer.WriteField("signature", "base64signaturedata")
	writer.Close()

	req := httptest.NewRequest("POST", "/test/signed/@v/1.0.0", body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusCreated {
		t.Errorf("Publish returned %d, want %d: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}

	// Verify signature file was created
	sigPath := filepath.Join(tmpDir, "test/signed", "1.0.0", "module.sig")
	sig, err := os.ReadFile(sigPath)
	if err != nil {
		t.Fatalf("Signature file not created: %v", err)
	}
	if string(sig) != "base64signaturedata" {
		t.Errorf("Signature = %s, want base64signaturedata", string(sig))
	}
}

func TestStoredModule_JSON(t *testing.T) {
	now := time.Now()
	stored := StoredModule{
		Name:        "test/module",
		Version:     "1.0.0",
		Hash:        "sha256:abc",
		PublishedAt: now,
		Size:        1000,
		Description: "Test",
		Dependencies: map[string]string{
			"other/dep": ">=1.0.0",
		},
		Tags:         []string{"stable", "latest"},
		ReleaseNotes: "Initial release",
	}

	data, err := json.Marshal(stored)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded StoredModule
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.Name != stored.Name {
		t.Errorf("Name = %s, want %s", decoded.Name, stored.Name)
	}
	if decoded.Version != stored.Version {
		t.Errorf("Version = %s, want %s", decoded.Version, stored.Version)
	}
	if len(decoded.Dependencies) != 1 {
		t.Errorf("Dependencies count = %d, want 1", len(decoded.Dependencies))
	}
	if len(decoded.Tags) != 2 {
		t.Errorf("Tags count = %d, want 2", len(decoded.Tags))
	}
}

// Test helper to create a request with body (uses valid ZIP)
func createPublishRequest(t *testing.T, path string, moduleContent, manifestContent string) (*http.Request, string) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	part, err := writer.CreateFormFile("module", "module.zip")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write(createTestZip(moduleContent)); err != nil {
		t.Fatal(err)
	}

	if err := writer.WriteField("manifest", manifestContent); err != nil {
		t.Fatal(err)
	}

	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("POST", path, body)
	req.Header.Set("Content-Type", writer.FormDataContentType())

	return req, writer.FormDataContentType()
}
