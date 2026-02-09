package statemgmt

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

// ============================================================================
// Nginx Site Module Tests
// ============================================================================

func TestNewNginxSiteModule(t *testing.T) {
	m := NewNginxSiteModule()

	if m.Name() != "nginx_site" {
		t.Errorf("expected name 'nginx_site', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"enabled", "disabled", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestNginxSiteModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_site module not supported on Windows")
	}

	m := NewNginxSiteModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "nginx_site",
		State:      "enabled",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestNginxSiteModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewNginxSiteModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_site",
		State:  "enabled",
		Parameters: map[string]interface{}{
			"name": "test-site",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "nginx_site module is not supported on Windows" {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

func TestNginxSiteModule_GetPaths(t *testing.T) {
	m := NewNginxSiteModule()
	paths := m.getNginxPaths()

	if paths.configDir == "" {
		t.Error("configDir should not be empty")
	}
	if paths.sitesAvailable == "" {
		t.Error("sitesAvailable should not be empty")
	}
	if paths.sitesEnabled == "" {
		t.Error("sitesEnabled should not be empty")
	}

	// Check platform-specific paths
	switch runtime.GOOS {
	case "darwin":
		if paths.configDir != "/usr/local/etc/nginx" {
			t.Errorf("unexpected macOS configDir: %s", paths.configDir)
		}
	case "linux":
		if paths.configDir != "/etc/nginx" {
			t.Errorf("unexpected Linux configDir: %s", paths.configDir)
		}
	}
}

func TestNginxSiteModule_SiteStateTransitions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_site module not supported on Windows")
	}

	// Create temp directory structure
	tmpDir := t.TempDir()
	sitesAvailable := filepath.Join(tmpDir, "sites-available")
	sitesEnabled := filepath.Join(tmpDir, "sites-enabled")
	if err := os.MkdirAll(sitesAvailable, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(sitesEnabled, 0755); err != nil {
		t.Fatal(err)
	}

	// Create a site configuration
	siteName := "test-site"
	siteConfig := `server { listen 80; server_name test.example.com; }`
	availablePath := filepath.Join(sitesAvailable, siteName)
	if err := os.WriteFile(availablePath, []byte(siteConfig), 0644); err != nil {
		t.Fatal(err)
	}

	// Verify paths created
	if _, err := os.Stat(availablePath); err != nil {
		t.Fatal("site config not created")
	}
	if _, err := os.Stat(sitesEnabled); err != nil {
		t.Fatal("sites-enabled not created")
	}

	// Note: Real testing would need to mock nginx paths
	// This test just verifies the temp directory setup works
	t.Logf("Created test site at: %s", availablePath)
}

// ============================================================================
// Nginx Config Module Tests
// ============================================================================

func TestNewNginxConfigModule(t *testing.T) {
	m := NewNginxConfigModule()

	if m.Name() != "nginx_config" {
		t.Errorf("expected name 'nginx_config', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestNginxConfigModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_config module not supported on Windows")
	}

	m := NewNginxConfigModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "nginx_config",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestNginxConfigModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewNginxConfigModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_config",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":    "test.conf",
			"content": "upstream backend { server 127.0.0.1:8080; }",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "nginx_config module is not supported on Windows" {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

// ============================================================================
// Apache Site Module Tests
// ============================================================================

func TestNewApacheSiteModule(t *testing.T) {
	m := NewApacheSiteModule()

	if m.Name() != "apache_site" {
		t.Errorf("expected name 'apache_site', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"enabled", "disabled", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestApacheSiteModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("apache_site module not supported on Windows")
	}

	m := NewApacheSiteModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "apache_site",
		State:      "enabled",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestApacheSiteModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewApacheSiteModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "apache_site",
		State:  "enabled",
		Parameters: map[string]interface{}{
			"name": "test-site",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "apache_site module is not supported on Windows" {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

func TestApacheSiteModule_GetPaths(t *testing.T) {
	m := NewApacheSiteModule()
	paths := m.getApachePaths()

	if paths.configDir == "" {
		t.Error("configDir should not be empty")
	}
	if paths.sitesAvailable == "" {
		t.Error("sitesAvailable should not be empty")
	}
	if paths.sitesEnabled == "" {
		t.Error("sitesEnabled should not be empty")
	}
	if paths.modsAvailable == "" {
		t.Error("modsAvailable should not be empty")
	}
	if paths.modsEnabled == "" {
		t.Error("modsEnabled should not be empty")
	}

	// Check platform-specific paths
	switch runtime.GOOS {
	case "darwin":
		if paths.configDir != "/usr/local/etc/httpd" {
			t.Errorf("unexpected macOS configDir: %s", paths.configDir)
		}
	case "linux":
		if paths.configDir != "/etc/apache2" {
			t.Errorf("unexpected Linux configDir: %s", paths.configDir)
		}
	}
}

// ============================================================================
// Apache Module Module Tests
// ============================================================================

func TestNewApacheModuleModule(t *testing.T) {
	m := NewApacheModuleModule()

	if m.Name() != "apache_module" {
		t.Errorf("expected name 'apache_module', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"enabled", "disabled"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestApacheModuleModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("apache_module not supported on Windows")
	}

	m := NewApacheModuleModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "apache_module",
		State:      "enabled",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestApacheModuleModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewApacheModuleModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "apache_module",
		State:  "enabled",
		Parameters: map[string]interface{}{
			"name": "rewrite",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "apache_module is not supported on Windows" {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

// ============================================================================
// Integration Tests (require Nginx/Apache)
// ============================================================================

func TestNginxSiteModule_Integration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_site module not supported on Windows")
	}

	// Check if nginx is available
	cmd := exec.CommandContext(context.Background(),"nginx", "-v")
	if err := cmd.Run(); err != nil {
		t.Skip("nginx is not available")
	}

	m := NewNginxSiteModule()

	// Test checking a non-existent site
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_site",
		State:  "absent",
		Parameters: map[string]interface{}{
			"name": "kscore-test-nonexistent-site-12345",
		},
	}

	result, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected site to not exist")
	}
	if !result.Matches {
		t.Error("expected state to match (absent)")
	}
}

func TestApacheSiteModule_Integration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("apache_site module not supported on Windows")
	}

	// Check if apache is available
	cmd := exec.CommandContext(context.Background(),"apachectl", "-v")
	if err := cmd.Run(); err != nil {
		cmd = exec.CommandContext(context.Background(),"apache2ctl", "-v")
		if err := cmd.Run(); err != nil {
			t.Skip("apache is not available")
		}
	}

	m := NewApacheSiteModule()

	// Test checking a non-existent site
	decl := &StateDeclaration{
		ID:     "test",
		Module: "apache_site",
		State:  "absent",
		Parameters: map[string]interface{}{
			"name": "kscore-test-nonexistent-site-12345",
		},
	}

	result, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if result.Present {
		t.Error("expected site to not exist")
	}
	if !result.Matches {
		t.Error("expected state to match (absent)")
	}
}

func TestApacheModuleModule_Integration(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("apache_module not supported on Windows")
	}

	// Check if apache is available
	cmd := exec.CommandContext(context.Background(),"apachectl", "-v")
	if err := cmd.Run(); err != nil {
		cmd = exec.CommandContext(context.Background(),"apache2ctl", "-v")
		if err := cmd.Run(); err != nil {
			t.Skip("apache is not available")
		}
	}

	m := NewApacheModuleModule()

	// Test checking a module
	decl := &StateDeclaration{
		ID:     "test",
		Module: "apache_module",
		State:  "enabled",
		Parameters: map[string]interface{}{
			"name": "rewrite",
		},
	}

	result, err := m.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	// Just verify Check works - module may or may not be enabled
	t.Logf("rewrite module enabled: %v", result.Metadata["enabled"])
}

// ============================================================================
// Nginx Upstream Module Tests
// ============================================================================

func TestNewNginxUpstreamModule(t *testing.T) {
	m := NewNginxUpstreamModule()

	if m.Name() != "nginx_upstream" {
		t.Errorf("expected name 'nginx_upstream', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
	for i, s := range expected {
		if states[i] != s {
			t.Errorf("expected state[%d] = '%s', got '%s'", i, s, states[i])
		}
	}
}

func TestNginxUpstreamModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_upstream module not supported on Windows")
	}

	m := NewNginxUpstreamModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "nginx_upstream",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestNginxUpstreamModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewNginxUpstreamModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_upstream",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":    "backend",
			"servers": []interface{}{"127.0.0.1:8080", "127.0.0.1:8081"},
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "nginx_upstream module is not supported on Windows" {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

func TestNginxUpstreamModule_Apply_Present(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_upstream module not supported on Windows")
	}

	// Check if nginx is available for config validation
	cmd := exec.CommandContext(context.Background(),"nginx", "-v")
	if err := cmd.Run(); err != nil {
		t.Skip("nginx is not available for config validation")
	}

	tmpDir := t.TempDir()
	upstreamDir := filepath.Join(tmpDir, "conf.d")
	if err := os.MkdirAll(upstreamDir, 0755); err != nil {
		t.Fatal(err)
	}

	m := &NginxUpstreamModule{
		BaseModule: NewBaseModule("nginx_upstream", []string{"present", "absent"}),
	}
	// Override upstream path for testing
	m.upstreamDir = upstreamDir

	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_upstream",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":      "backend",
			"servers":   []interface{}{"127.0.0.1:8080", "127.0.0.1:8081"},
			"method":    "least_conn",
			"keepalive": 32,
		},
	}

	result, err := m.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Changed {
		t.Error("expected changed=true")
	}

	// Verify file was created
	configPath := filepath.Join(upstreamDir, "upstream-backend.conf")
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read config: %v", err)
	}

	// Check configuration content
	config := string(content)
	if !filepath.IsAbs(configPath) {
		t.Error("expected absolute path")
	}
	if len(config) == 0 {
		t.Error("expected config content")
	}
}

func TestNginxUpstreamModule_BuildConfig(t *testing.T) {
	m := NewNginxUpstreamModule()

	tests := []struct {
		name     string
		decl     *StateDeclaration
		contains []string
	}{
		{
			name: "round_robin",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"servers": []interface{}{"127.0.0.1:8080", "127.0.0.1:8081"},
				},
			},
			contains: []string{"upstream test", "server 127.0.0.1:8080", "server 127.0.0.1:8081"},
		},
		{
			name: "least_conn",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"servers": []interface{}{"127.0.0.1:8080"},
					"method":  "least_conn",
				},
			},
			contains: []string{"least_conn"},
		},
		{
			name: "ip_hash",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"servers": []interface{}{"127.0.0.1:8080"},
					"method":  "ip_hash",
				},
			},
			contains: []string{"ip_hash"},
		},
		{
			name: "keepalive",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"servers":   []interface{}{"127.0.0.1:8080"},
					"keepalive": 32,
				},
			},
			contains: []string{"keepalive 32"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := m.buildUpstreamConfig(tt.decl, "test", []string{"127.0.0.1:8080", "127.0.0.1:8081"})
			for _, s := range tt.contains {
				if !containsSubstring(config, s) {
					t.Errorf("expected config to contain '%s', got: %s", s, config)
				}
			}
		})
	}
}

// ============================================================================
// Nginx Proxy Module Tests
// ============================================================================

func TestNewNginxProxyModule(t *testing.T) {
	m := NewNginxProxyModule()

	if m.Name() != "nginx_proxy" {
		t.Errorf("expected name 'nginx_proxy', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestNginxProxyModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_proxy module not supported on Windows")
	}

	m := NewNginxProxyModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "nginx_proxy",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestNginxProxyModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewNginxProxyModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_proxy",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":    "api",
			"backend": "http://127.0.0.1:8080",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "nginx_proxy module is not supported on Windows" {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

func TestNginxProxyModule_Apply_MissingBackend(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_proxy module not supported on Windows")
	}

	m := NewNginxProxyModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_proxy",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "api",
		},
	}

	_, err := m.Apply(context.Background(), decl)
	if err == nil || err.Error() != "backend parameter is required for present state" {
		t.Errorf("expected backend required error, got: %v", err)
	}
}

func TestNginxProxyModule_BuildConfig(t *testing.T) {
	m := NewNginxProxyModule()

	tests := []struct {
		name     string
		decl     *StateDeclaration
		contains []string
	}{
		{
			name: "basic_proxy",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"name":        "api",
					"backend":     "http://127.0.0.1:8080",
					"listen":      "80",
					"server_name": "api.example.com",
				},
			},
			contains: []string{"listen 80", "server_name api.example.com", "proxy_pass http://127.0.0.1:8080"},
		},
		{
			name: "websocket",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"name":      "ws",
					"backend":   "http://127.0.0.1:8080",
					"websocket": true,
				},
			},
			contains: []string{"proxy_http_version 1.1", "Upgrade $http_upgrade", "Connection \"upgrade\""},
		},
		{
			name: "custom_headers",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"name":            "api",
					"backend":         "http://127.0.0.1:8080",
					"proxy_headers":   true,
					"connect_timeout": "30s",
				},
			},
			contains: []string{"proxy_set_header Host $host", "proxy_connect_timeout 30s"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			backend := getStringParameter(tt.decl, "backend", "http://127.0.0.1:8080")
			config := m.buildProxyConfig(tt.decl, "test", backend)
			for _, s := range tt.contains {
				if !containsSubstring(config, s) {
					t.Errorf("expected config to contain '%s', got: %s", s, config)
				}
			}
		})
	}
}

// ============================================================================
// Nginx SSL Module Tests
// ============================================================================

func TestNewNginxSSLModule(t *testing.T) {
	m := NewNginxSSLModule()

	if m.Name() != "nginx_ssl" {
		t.Errorf("expected name 'nginx_ssl', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestNginxSSLModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_ssl module not supported on Windows")
	}

	m := NewNginxSSLModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "nginx_ssl",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestNginxSSLModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewNginxSSLModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_ssl",
		State:  "present",
		Parameters: map[string]interface{}{
			"name":            "secure",
			"certificate":     "/etc/ssl/certs/server.crt",
			"certificate_key": "/etc/ssl/private/server.key",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "nginx_ssl module is not supported on Windows" {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

func TestNginxSSLModule_Apply_MissingCertificate(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_ssl module not supported on Windows")
	}

	m := NewNginxSSLModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_ssl",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "secure",
		},
	}

	_, err := m.Apply(context.Background(), decl)
	if err == nil || !containsSubstring(err.Error(), "certificate") {
		t.Errorf("expected certificate required error, got: %v", err)
	}
}

func TestNginxSSLModule_BuildConfig(t *testing.T) {
	m := NewNginxSSLModule()

	tests := []struct {
		name     string
		decl     *StateDeclaration
		contains []string
	}{
		{
			name: "basic_ssl",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"certificate":     "/etc/ssl/certs/server.crt",
					"certificate_key": "/etc/ssl/private/server.key",
				},
			},
			contains: []string{"ssl_certificate /etc/ssl/certs/server.crt", "ssl_certificate_key /etc/ssl/private/server.key"},
		},
		{
			name: "protocols_and_ciphers",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"certificate":     "/etc/ssl/certs/server.crt",
					"certificate_key": "/etc/ssl/private/server.key",
					"protocols":       "TLSv1.2 TLSv1.3",
					"ciphers":         "ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256",
				},
			},
			contains: []string{"ssl_protocols TLSv1.2 TLSv1.3", "ssl_ciphers ECDHE-ECDSA-AES128-GCM-SHA256"},
		},
		{
			name: "session_settings",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"certificate":     "/etc/ssl/certs/server.crt",
					"certificate_key": "/etc/ssl/private/server.key",
					"session_cache":   "shared:SSL:10m",
					"session_timeout": "1d",
				},
			},
			contains: []string{"ssl_session_cache shared:SSL:10m", "ssl_session_timeout 1d"},
		},
		{
			name: "ocsp_stapling",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"certificate":     "/etc/ssl/certs/server.crt",
					"certificate_key": "/etc/ssl/private/server.key",
					"ocsp_stapling":   true,
				},
			},
			contains: []string{"ssl_stapling on", "ssl_stapling_verify on"},
		},
		{
			name: "hsts",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"certificate":     "/etc/ssl/certs/server.crt",
					"certificate_key": "/etc/ssl/private/server.key",
					"hsts":            true,
					"hsts_max_age":    31536000,
				},
			},
			contains: []string{"add_header Strict-Transport-Security"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := getStringParameter(tt.decl, "certificate", "/etc/ssl/certs/server.crt")
			key := getStringParameter(tt.decl, "certificate_key", "/etc/ssl/private/server.key")
			config := m.buildSSLConfig(tt.decl, "test", cert, key)
			for _, s := range tt.contains {
				if !containsSubstring(config, s) {
					t.Errorf("expected config to contain '%s', got: %s", s, config)
				}
			}
		})
	}
}

// ============================================================================
// Nginx Location Module Tests
// ============================================================================

func TestNewNginxLocationModule(t *testing.T) {
	m := NewNginxLocationModule()

	if m.Name() != "nginx_location" {
		t.Errorf("expected name 'nginx_location', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestNginxLocationModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_location module not supported on Windows")
	}

	m := NewNginxLocationModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "nginx_location",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestNginxLocationModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewNginxLocationModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_location",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "api",
			"path": "/api",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "nginx_location module is not supported on Windows" {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

func TestNginxLocationModule_Apply_MissingPath(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_location module not supported on Windows")
	}

	m := NewNginxLocationModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_location",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "api",
		},
	}

	_, err := m.Apply(context.Background(), decl)
	if err == nil || err.Error() != "path parameter is required for present state" {
		t.Errorf("expected path required error, got: %v", err)
	}
}

func TestNginxLocationModule_BuildConfig(t *testing.T) {
	m := NewNginxLocationModule()

	tests := []struct {
		name     string
		decl     *StateDeclaration
		contains []string
	}{
		{
			name: "basic_location",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"path": "/api",
				},
			},
			contains: []string{"location /api"},
		},
		{
			name: "exact_match",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"path":     "/health",
					"modifier": "=",
				},
			},
			contains: []string{"location = /health"},
		},
		{
			name: "regex_match",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"path":     "\\.php$",
					"modifier": "~",
				},
			},
			contains: []string{"location ~ \\.php$"},
		},
		{
			name: "proxy_pass",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"path":       "/api",
					"proxy_pass": "http://backend",
				},
			},
			contains: []string{"proxy_pass http://backend"},
		},
		{
			name: "root_and_try_files",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"path":      "/static",
					"root":      "/var/www/html",
					"try_files": "$uri $uri/ =404",
				},
			},
			contains: []string{"root /var/www/html", "try_files $uri $uri/ =404"},
		},
		{
			name: "access_control",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"path":  "/admin",
					"allow": []interface{}{"192.168.1.0/24", "10.0.0.0/8"},
					"deny":  []interface{}{"all"},
				},
			},
			contains: []string{"allow 192.168.1.0/24", "allow 10.0.0.0/8", "deny all"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := getStringParameter(tt.decl, "path", "/")
			config := m.buildLocationConfig(tt.decl, "test", path)
			for _, s := range tt.contains {
				if !containsSubstring(config, s) {
					t.Errorf("expected config to contain '%s', got: %s", s, config)
				}
			}
		})
	}
}

// ============================================================================
// Nginx Rate Limit Module Tests
// ============================================================================

func TestNewNginxRateLimitModule(t *testing.T) {
	m := NewNginxRateLimitModule()

	if m.Name() != "nginx_rate_limit" {
		t.Errorf("expected name 'nginx_rate_limit', got '%s'", m.Name())
	}

	states := m.ValidStates()
	expected := []string{"present", "absent"}
	if len(states) != len(expected) {
		t.Errorf("expected %d states, got %d", len(expected), len(states))
	}
}

func TestNginxRateLimitModule_Check_MissingName(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_rate_limit module not supported on Windows")
	}

	m := NewNginxRateLimitModule()
	decl := &StateDeclaration{
		ID:         "test",
		Module:     "nginx_rate_limit",
		State:      "present",
		Parameters: map[string]interface{}{},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "name parameter is required" {
		t.Errorf("expected name required error, got: %v", err)
	}
}

func TestNginxRateLimitModule_Check_Windows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("test for Windows platform")
	}

	m := NewNginxRateLimitModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_rate_limit",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "api_limit",
			"zone": "api",
			"rate": "10r/s",
		},
	}

	_, err := m.Check(context.Background(), decl)
	if err == nil || err.Error() != "nginx_rate_limit module is not supported on Windows" {
		t.Errorf("expected Windows not supported error, got: %v", err)
	}
}

func TestNginxRateLimitModule_Apply_MissingZone(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("nginx_rate_limit module not supported on Windows")
	}

	m := NewNginxRateLimitModule()
	decl := &StateDeclaration{
		ID:     "test",
		Module: "nginx_rate_limit",
		State:  "present",
		Parameters: map[string]interface{}{
			"name": "api_limit",
		},
	}

	_, err := m.Apply(context.Background(), decl)
	if err == nil || !containsSubstring(err.Error(), "rate") {
		t.Errorf("expected rate required error, got: %v", err)
	}
}

func TestNginxRateLimitModule_BuildConfig(t *testing.T) {
	m := NewNginxRateLimitModule()

	tests := []struct {
		name     string
		decl     *StateDeclaration
		contains []string
	}{
		{
			name: "basic_rate_limit",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"zone": "api",
					"rate": "10r/s",
				},
			},
			contains: []string{"limit_req_zone", "rate=10r/s"},
		},
		{
			name: "custom_key",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"zone": "api",
					"rate": "10r/s",
					"key":  "$http_x_api_key",
				},
			},
			contains: []string{"$http_x_api_key"},
		},
		{
			name: "burst_and_nodelay",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"zone":    "api",
					"rate":    "10r/s",
					"burst":   20,
					"nodelay": true,
				},
			},
			contains: []string{"burst=20", "nodelay"},
		},
		{
			name: "connection_limit",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"zone":       "api",
					"rate":       "10r/s",
					"conn_zone":  "conn",
					"conn_limit": 10,
				},
			},
			contains: []string{"limit_conn_zone", "limit_conn conn 10"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			zone := getStringParameter(tt.decl, "zone", "default")
			rate := getStringParameter(tt.decl, "rate", "10r/s")
			config := m.buildRateLimitConfig(tt.decl, "test", zone, rate)
			for _, s := range tt.contains {
				if !containsSubstring(config, s) {
					t.Errorf("expected config to contain '%s', got: %s", s, config)
				}
			}
		})
	}
}
