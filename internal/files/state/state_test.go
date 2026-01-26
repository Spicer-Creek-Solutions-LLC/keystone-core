package state

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/testing/mocks"
)

// TestParseKSCoreURL tests the ParseKSCoreURL function.
func TestParseKSCoreURL(t *testing.T) {
	tests := []struct {
		name      string
		url       string
		wantErr   bool
		namespace string
		path      string
		version   string
		checksum  string
	}{
		{
			name:      "simple_url",
			url:       "kscore://configs/nginx.conf",
			namespace: "configs",
			path:      "/nginx.conf",
		},
		{
			name:      "with_version",
			url:       "kscore://configs/nginx.conf?version=v1.0",
			namespace: "configs",
			path:      "/nginx.conf",
			version:   "v1.0",
		},
		{
			name:      "with_checksum",
			url:       "kscore://configs/nginx.conf?checksum=sha256:abc123",
			namespace: "configs",
			path:      "/nginx.conf",
			checksum:  "sha256:abc123",
		},
		{
			name:      "with_both",
			url:       "kscore://configs/nginx.conf?version=v1.0&checksum=sha256:abc123",
			namespace: "configs",
			path:      "/nginx.conf",
			version:   "v1.0",
			checksum:  "sha256:abc123",
		},
		{
			name:      "nested_path",
			url:       "kscore://configs/app/prod/config.yaml",
			namespace: "configs",
			path:      "/app/prod/config.yaml",
		},
		{
			name:    "not_kscore_url",
			url:     "http://example.com/file",
			wantErr: true,
		},
		{
			name:    "missing_namespace",
			url:     "kscore:///path/to/file",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			source, err := ParseKSCoreURL(tt.url)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if source.GetNamespace() != tt.namespace {
				t.Errorf("namespace = %q, want %q", source.GetNamespace(), tt.namespace)
			}

			if source.GetPath() != tt.path {
				t.Errorf("path = %q, want %q", source.GetPath(), tt.path)
			}

			if source.GetVersion() != tt.version {
				t.Errorf("version = %q, want %q", source.GetVersion(), tt.version)
			}

			if source.GetChecksum() != tt.checksum {
				t.Errorf("checksum = %q, want %q", source.GetChecksum(), tt.checksum)
			}
		})
	}
}

// TestLocalFileCache tests the LocalFileCache.
func TestLocalFileCache(t *testing.T) {
	// Create temp directory.
	tmpDir, err := os.MkdirTemp("", "cache-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create cache.
	cache, err := NewLocalFileCache(tmpDir)
	if err != nil {
		t.Fatalf("failed to create cache: %v", err)
	}

	// Test Get for non-existent entry.
	if _, ok := cache.Get("nonexistent"); ok {
		t.Error("expected Get to return false for nonexistent entry")
	}

	// Test Put.
	content := "test content"
	entry, err := cache.Put("key1", strings.NewReader(content), "", "v1")
	if err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	if entry.Version != "v1" {
		t.Errorf("version = %q, want %q", entry.Version, "v1")
	}

	if entry.Size != int64(len(content)) {
		t.Errorf("size = %d, want %d", entry.Size, len(content))
	}

	// Test Get for existing entry.
	retrieved, ok := cache.Get("key1")
	if !ok {
		t.Error("expected Get to return true for existing entry")
	}

	if retrieved.Checksum != entry.Checksum {
		t.Errorf("checksum = %q, want %q", retrieved.Checksum, entry.Checksum)
	}

	// Test checksum verification.
	_, err = cache.Put("key2", strings.NewReader("content"), "sha256:wrong", "v2")
	if err == nil {
		t.Error("expected error for checksum mismatch")
	}

	// Test Remove.
	if err := cache.Remove("key1"); err != nil {
		t.Fatalf("Remove failed: %v", err)
	}

	if _, ok := cache.Get("key1"); ok {
		t.Error("expected Get to return false after Remove")
	}

	// Test Clear.
	cache.Put("key3", strings.NewReader("content3"), "", "")
	cache.Put("key4", strings.NewReader("content4"), "", "")

	if err := cache.Clear(); err != nil {
		t.Fatalf("Clear failed: %v", err)
	}

	if _, ok := cache.Get("key3"); ok {
		t.Error("expected key3 to be cleared")
	}
}

// TestVerifyChecksum tests the VerifyChecksum function.
func TestVerifyChecksum(t *testing.T) {
	// Create temp file.
	tmpFile, err := os.CreateTemp("", "checksum-test-*")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	content := "test content for checksum"
	if _, err := tmpFile.WriteString(content); err != nil {
		t.Fatalf("failed to write content: %v", err)
	}
	tmpFile.Close()

	// Compute expected checksum.
	expectedChecksum, err := ComputeChecksum(tmpFile.Name())
	if err != nil {
		t.Fatalf("ComputeChecksum failed: %v", err)
	}

	// Test valid checksum.
	if err := VerifyChecksum(tmpFile.Name(), expectedChecksum); err != nil {
		t.Errorf("VerifyChecksum failed for valid checksum: %v", err)
	}

	// Test checksum without prefix.
	checksumWithoutPrefix := strings.TrimPrefix(expectedChecksum, "sha256:")
	if err := VerifyChecksum(tmpFile.Name(), checksumWithoutPrefix); err != nil {
		t.Errorf("VerifyChecksum failed for checksum without prefix: %v", err)
	}

	// Test invalid checksum.
	if err := VerifyChecksum(tmpFile.Name(), "sha256:invalid"); err == nil {
		t.Error("expected error for invalid checksum")
	}

	// Test empty checksum (should pass).
	if err := VerifyChecksum(tmpFile.Name(), ""); err != nil {
		t.Errorf("VerifyChecksum should pass for empty checksum: %v", err)
	}
}

// TestTemplateRenderer tests the TemplateRenderer.
func TestTemplateRenderer(t *testing.T) {
	renderer := NewTemplateRenderer(nil)

	tests := []struct {
		name     string
		template string
		vars     map[string]interface{}
		want     string
		wantErr  bool
	}{
		{
			name:     "simple_substitution",
			template: "Hello, {{ .name }}!",
			vars:     map[string]interface{}{"name": "World"},
			want:     "Hello, World!",
		},
		{
			name:     "multiple_vars",
			template: "{{ .greeting }}, {{ .name }}!",
			vars:     map[string]interface{}{"greeting": "Hi", "name": "Test"},
			want:     "Hi, Test!",
		},
		{
			name:     "upper_function",
			template: "{{ upper .name }}",
			vars:     map[string]interface{}{"name": "test"},
			want:     "TEST",
		},
		{
			name:     "lower_function",
			template: "{{ lower .name }}",
			vars:     map[string]interface{}{"name": "TEST"},
			want:     "test",
		},
		{
			name:     "default_function_with_empty",
			template: "{{ default \"default_value\" .value }}",
			vars:     map[string]interface{}{"value": ""},
			want:     "default_value",
		},
		{
			name:     "default_function_with_value",
			template: "{{ default \"default_value\" .value }}",
			vars:     map[string]interface{}{"value": "actual"},
			want:     "actual",
		},
		{
			name:     "ternary_function",
			template: "{{ ternary \"yes\" \"no\" .flag }}",
			vars:     map[string]interface{}{"flag": true},
			want:     "yes",
		},
		{
			name:     "quote_function",
			template: `{{ quote .value }}`,
			vars:     map[string]interface{}{"value": "test"},
			want:     `"test"`,
		},
		{
			name:     "contains_function",
			template: `{{ if contains .haystack "needle" }}found{{ else }}not found{{ end }}`,
			vars:     map[string]interface{}{"haystack": "needle in haystack"},
			want:     "found",
		},
		{
			name:     "missing_key_error",
			template: "{{ .missing }}",
			vars:     map[string]interface{}{},
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := renderer.Render(tt.template, tt.vars)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if result != tt.want {
				t.Errorf("result = %q, want %q", result, tt.want)
			}
		})
	}
}

// TestTemplateRendererCustomDelimiters tests custom delimiters.
func TestTemplateRendererCustomDelimiters(t *testing.T) {
	config := &TemplateConfig{
		LeftDelim:  "<%",
		RightDelim: "%>",
	}

	renderer := NewTemplateRenderer(config)

	result, err := renderer.Render("<% .name %>", map[string]interface{}{"name": "Test"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result != "Test" {
		t.Errorf("result = %q, want %q", result, "Test")
	}
}

// TestTemplateFileSource tests the TemplateFileSource.
func TestTemplateFileSource(t *testing.T) {
	source := &mocks.FileSource{
		Data:    []byte("Hello, {{ .name }}!"),
		Version: "v1",
	}

	renderer := NewTemplateRenderer(nil)
	vars := map[string]interface{}{"name": "World"}

	templateSource := NewTemplateFileSource(source, renderer, vars)

	reader, err := templateSource.Get(context.Background())
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	defer reader.Close()

	content, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(content) != "Hello, World!" {
		t.Errorf("content = %q, want %q", string(content), "Hello, World!")
	}

	if templateSource.GetVersion() != "v1" {
		t.Errorf("version = %q, want %q", templateSource.GetVersion(), "v1")
	}

	// Checksum should be empty for template source.
	if templateSource.GetChecksum() != "" {
		t.Errorf("checksum should be empty, got %q", templateSource.GetChecksum())
	}
}

// TestGenerateYumRepo tests yum repo file generation.
func TestGenerateYumRepo(t *testing.T) {
	repo := &PackageRepository{
		Name:     "myrepo",
		URL:      "http://example.com/repo",
		Enabled:  true,
		GPGCheck: true,
		GPGKey:   "http://example.com/key.gpg",
		Priority: 10,
	}

	content := GenerateYumRepo(repo)

	if !strings.Contains(content, "[myrepo]") {
		t.Error("expected repo section header")
	}

	if !strings.Contains(content, "baseurl=http://example.com/repo") {
		t.Error("expected baseurl")
	}

	if !strings.Contains(content, "enabled=1") {
		t.Error("expected enabled=1")
	}

	if !strings.Contains(content, "gpgcheck=1") {
		t.Error("expected gpgcheck=1")
	}

	if !strings.Contains(content, "gpgkey=http://example.com/key.gpg") {
		t.Error("expected gpgkey")
	}

	if !strings.Contains(content, "priority=10") {
		t.Error("expected priority")
	}
}

// TestGenerateAptSource tests apt source generation.
func TestGenerateAptSource(t *testing.T) {
	tests := []struct {
		name    string
		repo    *PackageRepository
		want    string
		wantNot string
	}{
		{
			name: "enabled",
			repo: &PackageRepository{
				Name:    "myrepo",
				URL:     "http://example.com/repo main",
				Enabled: true,
			},
			want: "deb http://example.com/repo main",
		},
		{
			name: "disabled",
			repo: &PackageRepository{
				Name:    "myrepo",
				URL:     "http://example.com/repo main",
				Enabled: false,
			},
			want: "# deb http://example.com/repo main",
		},
		{
			name: "with_gpg_key",
			repo: &PackageRepository{
				Name:    "myrepo",
				URL:     "http://example.com/repo main",
				Enabled: true,
				GPGKey:  "/usr/share/keyrings/myrepo.gpg",
			},
			want: "[signed-by=/usr/share/keyrings/myrepo.gpg]",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			content := GenerateAptSource(tt.repo)

			if !strings.Contains(content, tt.want) {
				t.Errorf("content = %q, want to contain %q", content, tt.want)
			}
		})
	}
}

// TestMirrorManager tests the MirrorManager.
func TestMirrorManager(t *testing.T) {
	manager := NewMirrorManager(nil)

	// Test AddMirror validation.
	err := manager.AddMirror(&RepositoryMirror{})
	if err == nil {
		t.Error("expected error for empty name")
	}

	err = manager.AddMirror(&RepositoryMirror{Name: "test"})
	if err == nil {
		t.Error("expected error for empty source URL")
	}

	err = manager.AddMirror(&RepositoryMirror{Name: "test", SourceURL: "http://example.com"})
	if err == nil {
		t.Error("expected error for empty local path")
	}

	// Test successful add.
	mirror := &RepositoryMirror{
		Name:      "test",
		SourceURL: "http://example.com/repo",
		LocalPath: "/var/mirror/test",
		Includes:  []string{"python-*"},
		Excludes:  []string{"*-debug"},
	}

	if err := manager.AddMirror(mirror); err != nil {
		t.Fatalf("AddMirror failed: %v", err)
	}

	// Test GetMirror.
	retrieved, ok := manager.GetMirror("test")
	if !ok {
		t.Error("expected GetMirror to return true")
	}

	if retrieved.Name != "test" {
		t.Errorf("name = %q, want %q", retrieved.Name, "test")
	}

	// Test ListMirrors.
	mirrors := manager.ListMirrors()
	if len(mirrors) != 1 {
		t.Errorf("expected 1 mirror, got %d", len(mirrors))
	}

	// Test RemoveMirror.
	manager.RemoveMirror("test")

	if _, ok := manager.GetMirror("test"); ok {
		t.Error("expected GetMirror to return false after Remove")
	}
}

// TestRepositoryMirror_ShouldInclude tests the ShouldInclude method.
func TestRepositoryMirror_ShouldInclude(t *testing.T) {
	tests := []struct {
		name     string
		includes []string
		excludes []string
		pkg      string
		want     bool
	}{
		{
			name:     "no_filters",
			includes: nil,
			excludes: nil,
			pkg:      "any-package",
			want:     true,
		},
		{
			name:     "include_match",
			includes: []string{"python-*"},
			excludes: nil,
			pkg:      "python-requests",
			want:     true,
		},
		{
			name:     "include_no_match",
			includes: []string{"python-*"},
			excludes: nil,
			pkg:      "nodejs",
			want:     false,
		},
		{
			name:     "exclude_match",
			includes: nil,
			excludes: []string{"*-debug"},
			pkg:      "python-debug",
			want:     false,
		},
		{
			name:     "exclude_no_match",
			includes: nil,
			excludes: []string{"*-debug"},
			pkg:      "python",
			want:     true,
		},
		{
			name:     "exclude_overrides_include",
			includes: []string{"python-*"},
			excludes: []string{"*-debug"},
			pkg:      "python-debug",
			want:     false,
		},
		{
			name:     "wildcard_middle",
			includes: []string{"*python*"},
			excludes: nil,
			pkg:      "libpython3",
			want:     true,
		},
		{
			name:     "exact_match",
			includes: []string{"python"},
			excludes: nil,
			pkg:      "python",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mirror := &RepositoryMirror{
				Includes: tt.includes,
				Excludes: tt.excludes,
			}

			got := mirror.ShouldInclude(tt.pkg)
			if got != tt.want {
				t.Errorf("ShouldInclude(%q) = %v, want %v", tt.pkg, got, tt.want)
			}
		})
	}
}

// TestPackageManager tests the PackageManager.
func TestPackageManager(t *testing.T) {
	// Create temp directories.
	cacheDir, err := os.MkdirTemp("", "pkg-cache-*")
	if err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	repoDir, err := os.MkdirTemp("", "pkg-repo-*")
	if err != nil {
		t.Fatalf("failed to create repo dir: %v", err)
	}
	defer os.RemoveAll(repoDir)

	// Create package manager (without client for basic tests).
	config := &PackageManagerConfig{
		CacheDir: cacheDir,
		RepoDir:  repoDir,
	}

	manager, err := NewPackageManager(config)
	if err != nil {
		t.Fatalf("NewPackageManager failed: %v", err)
	}

	// Test WriteYumRepo.
	yumRepo := &PackageRepository{
		Name:     "testrepo",
		URL:      "http://example.com/repo",
		Enabled:  true,
		GPGCheck: false,
	}

	if err := manager.WriteYumRepo(yumRepo); err != nil {
		t.Fatalf("WriteYumRepo failed: %v", err)
	}

	// Verify file was created.
	repoFile := filepath.Join(repoDir, "testrepo.repo")
	if _, err := os.Stat(repoFile); os.IsNotExist(err) {
		t.Error("repo file was not created")
	}

	// Test WriteAptSource.
	aptRepo := &PackageRepository{
		Name:    "aptrepo",
		URL:     "http://example.com/debian stable main",
		Enabled: true,
	}

	if err := manager.WriteAptSource(aptRepo); err != nil {
		t.Fatalf("WriteAptSource failed: %v", err)
	}

	// Verify file was created.
	sourceFile := filepath.Join(repoDir, "aptrepo.list")
	if _, err := os.Stat(sourceFile); os.IsNotExist(err) {
		t.Error("source file was not created")
	}

	// Test RemoveRepo.
	if err := manager.RemoveRepo("testrepo", "yum"); err != nil {
		t.Fatalf("RemoveRepo failed: %v", err)
	}

	if _, err := os.Stat(repoFile); !os.IsNotExist(err) {
		t.Error("repo file was not removed")
	}

	// Test unsupported repo type.
	if err := manager.RemoveRepo("test", "unsupported"); err == nil {
		t.Error("expected error for unsupported repo type")
	}
}

// TestGetPackagePath tests the GetPackagePath method.
func TestGetPackagePath(t *testing.T) {
	cacheDir, err := os.MkdirTemp("", "pkg-cache-*")
	if err != nil {
		t.Fatalf("failed to create cache dir: %v", err)
	}
	defer os.RemoveAll(cacheDir)

	manager := &PackageManager{
		cacheDir: cacheDir,
	}

	tests := []struct {
		name string
		pkg  *PackageFile
		want string
	}{
		{
			name: "name_only",
			pkg:  &PackageFile{Name: "nginx"},
			want: filepath.Join(cacheDir, "nginx"),
		},
		{
			name: "with_version",
			pkg:  &PackageFile{Name: "nginx", Version: "1.18.0"},
			want: filepath.Join(cacheDir, "nginx-1.18.0"),
		},
		{
			name: "with_arch",
			pkg:  &PackageFile{Name: "nginx", Architecture: "x86_64"},
			want: filepath.Join(cacheDir, "nginx.x86_64"),
		},
		{
			name: "full",
			pkg:  &PackageFile{Name: "nginx", Version: "1.18.0", Architecture: "x86_64"},
			want: filepath.Join(cacheDir, "nginx-1.18.0.x86_64"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := manager.GetPackagePath(tt.pkg)
			if got != tt.want {
				t.Errorf("GetPackagePath() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestIndentFunctions tests the indent and nindent template functions.
func TestIndentFunctions(t *testing.T) {
	renderer := NewTemplateRenderer(nil)

	// Test indent.
	result, err := renderer.Render(`{{ indent 4 "line1\nline2" }}`, nil)
	if err != nil {
		t.Fatalf("indent failed: %v", err)
	}

	expected := "    line1\n    line2"
	if result != expected {
		t.Errorf("indent result = %q, want %q", result, expected)
	}

	// Test nindent.
	result, err = renderer.Render(`before{{ nindent 2 "line1\nline2" }}`, nil)
	if err != nil {
		t.Fatalf("nindent failed: %v", err)
	}

	expected = "before\n  line1\n  line2"
	if result != expected {
		t.Errorf("nindent result = %q, want %q", result, expected)
	}
}
