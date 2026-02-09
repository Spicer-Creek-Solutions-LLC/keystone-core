package injection

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// =============================================================================
// Mock Secret Source
// =============================================================================

type mockSecretSource struct {
	secrets map[string]*Secret
}

func newMockSecretSource() *mockSecretSource {
	return &mockSecretSource{
		secrets: make(map[string]*Secret),
	}
}

func (m *mockSecretSource) AddSecret(path string, data map[string]interface{}) {
	m.secrets[path] = &Secret{
		Path:    path,
		Data:    data,
		Version: 1,
	}
}

func (m *mockSecretSource) GetSecret(ctx context.Context, path string) (*Secret, error) {
	if secret, ok := m.secrets[path]; ok {
		return secret, nil
	}
	return nil, nil
}

func (m *mockSecretSource) WatchSecret(ctx context.Context, path string) (<-chan *Secret, error) {
	ch := make(chan *Secret)
	close(ch)
	return ch, nil
}

// =============================================================================
// Secret Tests
// =============================================================================

func TestSecretGetString(t *testing.T) {
	secret := &Secret{
		Data: map[string]interface{}{
			"username": "admin",
			"port":     float64(5432),
		},
	}

	if got := secret.GetString("username"); got != "admin" {
		t.Errorf("GetString(username) = %q, want %q", got, "admin")
	}

	if got := secret.GetString("missing"); got != "" {
		t.Errorf("GetString(missing) = %q, want empty", got)
	}
}

func TestSecretGetBytes(t *testing.T) {
	secret := &Secret{
		Data: map[string]interface{}{
			"key":  "value",
			"cert": []byte("certificate data"),
		},
	}

	// String should convert to bytes
	if got := secret.GetBytes("key"); string(got) != "value" {
		t.Errorf("GetBytes(key) = %q, want %q", got, "value")
	}

	// Bytes should return as-is
	if got := secret.GetBytes("cert"); string(got) != "certificate data" {
		t.Errorf("GetBytes(cert) = %q, want %q", got, "certificate data")
	}
}

// =============================================================================
// File Injector Tests
// =============================================================================

func TestFileInjectorCreate(t *testing.T) {
	source := newMockSecretSource()

	_, err := NewFileInjector(nil, source, nil)
	if err == nil {
		t.Error("expected error for nil config")
	}

	config := &FileInjectionConfig{}
	_, err = NewFileInjector(config, nil, nil)
	if err == nil {
		t.Error("expected error for nil source")
	}
}

func TestFileInjectorBasic(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()
	source.AddSecret("db/password", map[string]interface{}{
		"password": "secret123",
	})

	config := &FileInjectionConfig{
		Config: Config{Enabled: true},
		BasePath:        dir,
		DefaultMode:     0600,
		AtomicWrite:     true,
		Files: []FileRule{
			{
				SecretPath: "db/password",
				SecretKey:  "password",
				FilePath:   filepath.Join(dir, "db_password"),
			},
		},
	}

	injector, err := NewFileInjector(config, source, nil)
	if err != nil {
		t.Fatalf("NewFileInjector failed: %v", err)
	}

	results, err := injector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if !results[0].Success {
		t.Errorf("injection failed: %v", results[0].Error)
	}

	// Verify file content
	content, err := os.ReadFile(filepath.Join(dir, "db_password"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(content) != "secret123" {
		t.Errorf("content = %q, want %q", content, "secret123")
	}

	// Verify permissions
	info, err := os.Stat(filepath.Join(dir, "db_password"))
	if err != nil {
		t.Fatalf("failed to stat file: %v", err)
	}

	if info.Mode().Perm() != 0600 {
		t.Errorf("mode = %o, want %o", info.Mode().Perm(), 0600)
	}
}

func TestFileInjectorAtomicWrite(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()
	source.AddSecret("test/data", map[string]interface{}{
		"value": "test content",
	})

	config := &FileInjectionConfig{
		Config: Config{Enabled: true},
		AtomicWrite:     true,
		Files: []FileRule{
			{
				SecretPath: "test/data",
				SecretKey:  "value",
				FilePath:   filepath.Join(dir, "test.txt"),
			},
		},
	}

	injector, err := NewFileInjector(config, source, nil)
	if err != nil {
		t.Fatalf("NewFileInjector failed: %v", err)
	}

	// Inject multiple times to test atomic overwrites
	for i := 0; i < 3; i++ {
		results, err := injector.Inject(context.Background())
		if err != nil {
			t.Fatalf("Inject %d failed: %v", i, err)
		}
		if !results[0].Success {
			t.Errorf("injection %d failed: %v", i, results[0].Error)
		}
	}

	// Verify final content
	content, err := os.ReadFile(filepath.Join(dir, "test.txt"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(content) != "test content" {
		t.Errorf("content = %q, want %q", content, "test content")
	}
}

func TestFileInjectorBinaryData(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()

	binaryData := []byte{0x00, 0x01, 0x02, 0xFF, 0xFE, 0xFD}
	source.secrets["binary/data"] = &Secret{
		Path:    "binary/data",
		Data:    map[string]interface{}{"content": binaryData},
		Version: 1,
	}

	config := &FileInjectionConfig{
		Config: Config{Enabled: true},
		Files: []FileRule{
			{
				SecretPath: "binary/data",
				SecretKey:  "content",
				FilePath:   filepath.Join(dir, "binary.bin"),
				Binary:     true,
			},
		},
	}

	injector, err := NewFileInjector(config, source, nil)
	if err != nil {
		t.Fatalf("NewFileInjector failed: %v", err)
	}

	results, err := injector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("injection failed: %v", results[0].Error)
	}

	content, err := os.ReadFile(filepath.Join(dir, "binary.bin"))
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}

	if string(content) != string(binaryData) {
		t.Errorf("binary content mismatch")
	}
}

func TestFileInjectorMissingSecret(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()

	config := &FileInjectionConfig{
		Config: Config{Enabled: true},
		Files: []FileRule{
			{
				SecretPath: "missing/secret",
				SecretKey:  "value",
				FilePath:   filepath.Join(dir, "missing.txt"),
			},
		},
	}

	injector, err := NewFileInjector(config, source, nil)
	if err != nil {
		t.Fatalf("NewFileInjector failed: %v", err)
	}

	results, err := injector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if results[0].Success {
		t.Error("expected failure for missing secret")
	}

	if results[0].Error == nil {
		t.Error("expected error for missing secret")
	}
}

func TestFileInjectorSubdirectory(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()
	source.AddSecret("test/secret", map[string]interface{}{"data": "value"})

	config := &FileInjectionConfig{
		Config: Config{Enabled: true},
		Files: []FileRule{
			{
				SecretPath: "test/secret",
				SecretKey:  "data",
				FilePath:   filepath.Join(dir, "deep", "nested", "file.txt"),
			},
		},
	}

	injector, err := NewFileInjector(config, source, nil)
	if err != nil {
		t.Fatalf("NewFileInjector failed: %v", err)
	}

	results, err := injector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("injection failed: %v", results[0].Error)
	}

	// Verify directory was created
	if _, err := os.Stat(filepath.Join(dir, "deep", "nested")); os.IsNotExist(err) {
		t.Error("expected directory to be created")
	}
}

// =============================================================================
// Environment Injector Tests
// =============================================================================

func TestEnvInjectorCreate(t *testing.T) {
	source := newMockSecretSource()

	_, err := NewEnvInjector(nil, source)
	if err == nil {
		t.Error("expected error for nil config")
	}

	config := &EnvInjectionConfig{}
	_, err = NewEnvInjector(config, nil)
	if err == nil {
		t.Error("expected error for nil source")
	}
}

func TestEnvInjectorBasic(t *testing.T) {
	source := newMockSecretSource()
	source.AddSecret("app/config", map[string]interface{}{
		"api_key":    "key123",
		"api_secret": "secret456",
	})

	config := &EnvInjectionConfig{
		Config: Config{Enabled: true},
		Prefix:          "MYAPP_",
		Uppercase:       true,
		Secrets: []EnvRule{
			{
				SecretPath: "app/config",
				SecretKey:  "api_key",
				EnvVar:     "api_key",
			},
			{
				SecretPath: "app/config",
				SecretKey:  "api_secret",
				EnvVar:     "api_secret",
			},
		},
	}

	injector, err := NewEnvInjector(config, source)
	if err != nil {
		t.Fatalf("NewEnvInjector failed: %v", err)
	}

	// Clean up env vars after test
	defer func() {
		os.Unsetenv("MYAPP_API_KEY")
		os.Unsetenv("MYAPP_API_SECRET")
	}()

	results, err := injector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if !r.Success {
			t.Errorf("injection failed for %s: %v", r.Target, r.Error)
		}
	}

	// Verify environment variables
	if val := os.Getenv("MYAPP_API_KEY"); val != "key123" {
		t.Errorf("MYAPP_API_KEY = %q, want %q", val, "key123")
	}

	if val := os.Getenv("MYAPP_API_SECRET"); val != "secret456" {
		t.Errorf("MYAPP_API_SECRET = %q, want %q", val, "secret456")
	}
}

func TestEnvInjectorSanitize(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with-dash", "with_dash"},
		{"with.dot", "with_dot"},
		{"with space", "with_space"},
		{"123start", "_23start"},
		{"UPPER", "UPPER"},
		{"MixedCase", "MixedCase"},
	}

	for _, tt := range tests {
		got := sanitizeEnvVar(tt.input)
		if got != tt.want {
			t.Errorf("sanitizeEnvVar(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestEnvInjectorClear(t *testing.T) {
	source := newMockSecretSource()
	source.AddSecret("test/env", map[string]interface{}{"value": "test"})

	config := &EnvInjectionConfig{
		Config: Config{Enabled: true},
		Secrets: []EnvRule{
			{
				SecretPath: "test/env",
				SecretKey:  "value",
				EnvVar:     "TEST_CLEAR_VAR",
			},
		},
	}

	injector, err := NewEnvInjector(config, source)
	if err != nil {
		t.Fatalf("NewEnvInjector failed: %v", err)
	}

	injector.Inject(context.Background())

	if os.Getenv("TEST_CLEAR_VAR") == "" {
		t.Error("expected env var to be set")
	}

	injector.Clear()

	if os.Getenv("TEST_CLEAR_VAR") != "" {
		t.Error("expected env var to be cleared")
	}
}

func TestProcessEnvBuilder(t *testing.T) {
	source := newMockSecretSource()
	source.AddSecret("app/creds", map[string]interface{}{
		"username": "dbuser",
		"password": "dbpass",
	})

	config := &EnvInjectionConfig{
		Prefix:    "DB_",
		Uppercase: true,
		Secrets: []EnvRule{
			{
				SecretPath: "app/creds",
				SecretKey:  "username",
				EnvVar:     "user",
			},
			{
				SecretPath: "app/creds",
				SecretKey:  "password",
				EnvVar:     "pass",
			},
		},
	}

	builder := NewProcessEnvBuilder(config, source).WithCleanEnv()

	env, err := builder.Build(context.Background())
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	// Check that our vars are present
	found := make(map[string]bool)
	for _, e := range env {
		if strings.HasPrefix(e, "DB_") {
			parts := strings.SplitN(e, "=", 2)
			found[parts[0]] = true

			switch parts[0] {
			case "DB_USER":
				if parts[1] != "dbuser" {
					t.Errorf("DB_USER = %q, want %q", parts[1], "dbuser")
				}
			case "DB_PASS":
				if parts[1] != "dbpass" {
					t.Errorf("DB_PASS = %q, want %q", parts[1], "dbpass")
				}
			}
		}
	}

	if !found["DB_USER"] {
		t.Error("DB_USER not found in env")
	}
	if !found["DB_PASS"] {
		t.Error("DB_PASS not found in env")
	}
}

// =============================================================================
// Template Injector Tests
// =============================================================================

func TestTemplateInjectorCreate(t *testing.T) {
	source := newMockSecretSource()

	_, err := NewTemplateInjector(nil, source, nil)
	if err == nil {
		t.Error("expected error for nil config")
	}

	config := &TemplateInjectionConfig{}
	_, err = NewTemplateInjector(config, nil, nil)
	if err == nil {
		t.Error("expected error for nil source")
	}
}

func TestTemplateInjectorBasic(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()
	source.AddSecret("db/config", map[string]interface{}{
		"host":     "localhost",
		"port":     "5432",
		"database": "myapp",
		"username": "appuser",
		"password": "secretpass",
	})

	// Create template file
	templateContent := `database:
  host: {{ secret "db/config" "host" }}
  port: {{ secret "db/config" "port" }}
  name: {{ secret "db/config" "database" }}
  user: {{ secret "db/config" "username" }}
  pass: {{ secret "db/config" "password" }}
`
	templatePath := filepath.Join(dir, "db.tpl")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	config := &TemplateInjectionConfig{
		Config: Config{Enabled: true},
		Templates: []TemplateRule{
			{
				Source:      templatePath,
				Destination: filepath.Join(dir, "database.yml"),
				Mode:        0644,
			},
		},
	}

	injector, err := NewTemplateInjector(config, source, nil)
	if err != nil {
		t.Fatalf("NewTemplateInjector failed: %v", err)
	}

	results, err := injector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if !results[0].Success {
		t.Errorf("injection failed: %v", results[0].Error)
	}

	// Verify rendered content
	content, err := os.ReadFile(filepath.Join(dir, "database.yml"))
	if err != nil {
		t.Fatalf("failed to read rendered file: %v", err)
	}

	expectedContent := `database:
  host: localhost
  port: 5432
  name: myapp
  user: appuser
  pass: secretpass
`
	if string(content) != expectedContent {
		t.Errorf("rendered content mismatch:\ngot:\n%s\nwant:\n%s", content, expectedContent)
	}
}

func TestTemplateInjectorFunctions(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()
	source.AddSecret("test/data", map[string]interface{}{
		"value":  "hello",
		"number": "42",
	})

	templateContent := `upper: {{ secret "test/data" "value" | toUpper }}
lower: {{ "WORLD" | toLower }}
default: {{ secret "test/data" "missing" | default "fallback" }}
`
	templatePath := filepath.Join(dir, "funcs.tpl")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	config := &TemplateInjectionConfig{
		Config: Config{Enabled: true},
		Templates: []TemplateRule{
			{
				Source:      templatePath,
				Destination: filepath.Join(dir, "output.txt"),
			},
		},
	}

	injector, err := NewTemplateInjector(config, source, nil)
	if err != nil {
		t.Fatalf("NewTemplateInjector failed: %v", err)
	}

	results, err := injector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("injection failed: %v", results[0].Error)
	}

	content, err := os.ReadFile(filepath.Join(dir, "output.txt"))
	if err != nil {
		t.Fatalf("failed to read rendered file: %v", err)
	}

	if !strings.Contains(string(content), "upper: HELLO") {
		t.Error("toUpper function not working")
	}
	if !strings.Contains(string(content), "lower: world") {
		t.Error("toLower function not working")
	}
	if !strings.Contains(string(content), "default: fallback") {
		t.Error("default function not working")
	}
}

func TestTemplateInjectorCustomDelimiters(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()
	source.AddSecret("test/data", map[string]interface{}{
		"value": "custom delimiters work",
	})

	templateContent := `result: [[ secret "test/data" "value" ]]`
	templatePath := filepath.Join(dir, "custom.tpl")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	config := &TemplateInjectionConfig{
		Config: Config{Enabled: true},
		Templates: []TemplateRule{
			{
				Source:      templatePath,
				Destination: filepath.Join(dir, "custom.txt"),
				LeftDelim:   "[[",
				RightDelim:  "]]",
			},
		},
	}

	injector, err := NewTemplateInjector(config, source, nil)
	if err != nil {
		t.Fatalf("NewTemplateInjector failed: %v", err)
	}

	results, err := injector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Inject failed: %v", err)
	}

	if !results[0].Success {
		t.Errorf("injection failed: %v", results[0].Error)
	}

	content, err := os.ReadFile(filepath.Join(dir, "custom.txt"))
	if err != nil {
		t.Fatalf("failed to read rendered file: %v", err)
	}

	if string(content) != "result: custom delimiters work" {
		t.Errorf("got %q, want %q", content, "result: custom delimiters work")
	}
}

func TestTemplateInjectorChangeDetection(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()
	source.AddSecret("test/data", map[string]interface{}{
		"value": "initial",
	})

	templateContent := `value: {{ secret "test/data" "value" }}`
	templatePath := filepath.Join(dir, "detect.tpl")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	destPath := filepath.Join(dir, "detect.txt")

	config := &TemplateInjectionConfig{
		Config: Config{Enabled: true},
		Templates: []TemplateRule{
			{
				Source:      templatePath,
				Destination: destPath,
			},
		},
	}

	injector, err := NewTemplateInjector(config, source, nil)
	if err != nil {
		t.Fatalf("NewTemplateInjector failed: %v", err)
	}

	// First injection
	results1, _ := injector.Inject(context.Background())
	if !results1[0].Success {
		t.Fatalf("first injection failed: %v", results1[0].Error)
	}

	// Read initial content
	content1, _ := os.ReadFile(destPath)
	if !strings.Contains(string(content1), "initial") {
		t.Error("expected initial content")
	}

	// Second injection with same content
	results2, _ := injector.Inject(context.Background())
	if !results2[0].Success {
		t.Fatalf("second injection failed: %v", results2[0].Error)
	}

	// Content should be unchanged
	content2, _ := os.ReadFile(destPath)
	if string(content1) != string(content2) {
		t.Error("content should be same for unchanged secret")
	}

	// Update secret
	source.AddSecret("test/data", map[string]interface{}{
		"value": "changed",
	})

	// Third injection should update content
	results3, _ := injector.Inject(context.Background())
	if !results3[0].Success {
		t.Fatalf("third injection failed: %v", results3[0].Error)
	}

	// Content should be different
	content3, _ := os.ReadFile(destPath)
	if !strings.Contains(string(content3), "changed") {
		t.Error("expected changed content")
	}
	if string(content1) == string(content3) {
		t.Error("content should differ for changed secret")
	}
}

// =============================================================================
// Signal Tests
// =============================================================================

func TestParseSignal(t *testing.T) {
	tests := []struct {
		name    string
		wantErr bool
	}{
		{"SIGHUP", false},
		{"HUP", false},
		{"SIGTERM", false},
		{"TERM", false},
		{"SIGUSR1", false},
		{"USR1", false},
		{"1", false},
		{"INVALID", true},
	}

	for _, tt := range tests {
		_, err := parseSignal(tt.name)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseSignal(%q) error = %v, wantErr %v", tt.name, err, tt.wantErr)
		}
	}
}

// =============================================================================
// Helper Function Tests
// =============================================================================

func TestHashContent(t *testing.T) {
	// Same content should have same hash
	hash1 := hashContent([]byte("test content"))
	hash2 := hashContent([]byte("test content"))
	if hash1 != hash2 {
		t.Error("same content should have same hash")
	}

	// Different content should have different hash
	hash3 := hashContent([]byte("different content"))
	if hash1 == hash3 {
		t.Error("different content should have different hash")
	}
}

func TestEscapeJSON(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"simple", "simple"},
		{"with\"quote", `with\"quote`},
		{"with\\slash", `with\\slash`},
		{"with\nnewline", `with\nnewline`},
		{"with\ttab", `with\ttab`},
	}

	for _, tt := range tests {
		got := escapeJSON(tt.input)
		if got != tt.want {
			t.Errorf("escapeJSON(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// =============================================================================
// Integration Tests
// =============================================================================

func TestFullInjectionWorkflow(t *testing.T) {
	dir := t.TempDir()
	source := newMockSecretSource()

	// Add various secrets
	source.AddSecret("db/creds", map[string]interface{}{
		"host":     "db.example.com",
		"port":     "5432",
		"username": "app",
		"password": "s3cr3t",
	})
	source.AddSecret("api/keys", map[string]interface{}{
		"public":  "pk_123",
		"private": "sk_456",
	})
	source.AddSecret("tls/cert", map[string]interface{}{
		"certificate": "-----BEGIN CERTIFICATE-----\nMIIB...",
		"private_key": "-----BEGIN PRIVATE KEY-----\nMIIE...",
	})

	// Create template
	templateContent := `# Database Configuration
database:
  host: {{ secret "db/creds" "host" }}
  port: {{ secret "db/creds" "port" | default "5432" }}
  credentials:
    username: {{ secret "db/creds" "username" }}
    password: {{ secret "db/creds" "password" }}

# API Configuration
api:
  public_key: {{ secret "api/keys" "public" }}
  # private key is injected via file
`
	templatePath := filepath.Join(dir, "config.tpl")
	if err := os.WriteFile(templatePath, []byte(templateContent), 0644); err != nil {
		t.Fatalf("failed to write template: %v", err)
	}

	// 1. File injection for certificates
	fileConfig := &FileInjectionConfig{
		Config: Config{Enabled: true},
		AtomicWrite:     true,
		Files: []FileRule{
			{
				SecretPath: "tls/cert",
				SecretKey:  "certificate",
				FilePath:   filepath.Join(dir, "certs", "server.crt"),
				Mode:       0644,
			},
			{
				SecretPath: "tls/cert",
				SecretKey:  "private_key",
				FilePath:   filepath.Join(dir, "certs", "server.key"),
				Mode:       0600,
			},
		},
	}

	fileInjector, err := NewFileInjector(fileConfig, source, nil)
	if err != nil {
		t.Fatalf("NewFileInjector failed: %v", err)
	}

	fileResults, err := fileInjector.Inject(context.Background())
	if err != nil {
		t.Fatalf("File injection failed: %v", err)
	}

	for _, r := range fileResults {
		if !r.Success {
			t.Errorf("file injection failed for %s: %v", r.Target, r.Error)
		}
	}

	// 2. Environment injection for API keys
	envConfig := &EnvInjectionConfig{
		Config: Config{Enabled: true},
		Prefix:          "APP_",
		Uppercase:       true,
		Secrets: []EnvRule{
			{
				SecretPath: "api/keys",
				SecretKey:  "private",
				EnvVar:     "api_secret",
			},
		},
	}

	envInjector, err := NewEnvInjector(envConfig, source)
	if err != nil {
		t.Fatalf("NewEnvInjector failed: %v", err)
	}

	defer envInjector.Clear()

	envResults, err := envInjector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Env injection failed: %v", err)
	}

	for _, r := range envResults {
		if !r.Success {
			t.Errorf("env injection failed for %s: %v", r.Target, r.Error)
		}
	}

	// 3. Template injection for config
	templateConfig := &TemplateInjectionConfig{
		Config: Config{Enabled: true},
		Templates: []TemplateRule{
			{
				Source:      templatePath,
				Destination: filepath.Join(dir, "config.yml"),
				Mode:        0644,
			},
		},
	}

	templateInjector, err := NewTemplateInjector(templateConfig, source, nil)
	if err != nil {
		t.Fatalf("NewTemplateInjector failed: %v", err)
	}

	templateResults, err := templateInjector.Inject(context.Background())
	if err != nil {
		t.Fatalf("Template injection failed: %v", err)
	}

	for _, r := range templateResults {
		if !r.Success {
			t.Errorf("template injection failed for %s: %v", r.Target, r.Error)
		}
	}

	// Verify all files exist
	expectedFiles := []string{
		"certs/server.crt",
		"certs/server.key",
		"config.yml",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(dir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s to exist", f)
		}
	}

	// Verify env var
	if os.Getenv("APP_API_SECRET") != "sk_456" {
		t.Errorf("APP_API_SECRET not set correctly")
	}

	// Verify config content
	configContent, _ := os.ReadFile(filepath.Join(dir, "config.yml"))
	if !strings.Contains(string(configContent), "host: db.example.com") {
		t.Error("config does not contain expected host")
	}
	if !strings.Contains(string(configContent), "password: s3cr3t") {
		t.Error("config does not contain expected password")
	}
}
