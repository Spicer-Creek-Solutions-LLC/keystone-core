package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-secrets" {
		t.Errorf("Use = %v, want kscore-secrets", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expectedSubcommands := []string{
		"version",
		"get",
		"list",
		"backends",
		"audit",
		"rotate",
		"schedule",
		"policy",
		"dynamic",
		"leases",
		"encrypt",
		"decrypt",
		"rewrap",
		"template",
		"cache",
		"rotate-keys",
	}

	for _, expected := range expectedSubcommands {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", expected)
		}
	}
}

func TestRootCmdPersistentFlags(t *testing.T) {
	cmd := newRootCmd()

	flags := []string{
		"server", "output", "verbose",
		"tls", "tls-ca-cert", "tls-cert", "tls-key",
		"tls-skip-verify", "tls-server-name", "tls-min-version",
	}
	for _, name := range flags {
		if cmd.PersistentFlags().Lookup(name) == nil {
			t.Errorf("expected persistent flag %q not found", name)
		}
	}
}

func TestNewVersionCmd(t *testing.T) {
	cmd := newVersionCmd()
	if cmd == nil {
		t.Fatal("newVersionCmd should not return nil")
	}

	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{})

	if err := cmd.Execute(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if buf.Len() == 0 {
		t.Error("version command should produce output")
	}
}

// =============================================================================
// TLS Helper Tests
// =============================================================================

func TestParseTLSMinVersion(t *testing.T) {
	tests := []struct {
		input   string
		want    uint16
		wantErr bool
	}{
		{"", tls.VersionTLS13, false},
		{"1.3", tls.VersionTLS13, false},
		{"tls1.3", tls.VersionTLS13, false},
		{"tls13", tls.VersionTLS13, false},
		{"1.2", tls.VersionTLS12, false},
		{"tls1.2", tls.VersionTLS12, false},
		{"tls12", tls.VersionTLS12, false},
		{"TLS1.3", tls.VersionTLS13, false},
		{"1.1", 0, true},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		got, err := parseTLSMinVersion(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("parseTLSMinVersion(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("parseTLSMinVersion(%q) = %d, want %d", tt.input, got, tt.want)
		}
	}
}

func TestBuildTLSConfig_Defaults(t *testing.T) {
	cfg := &Config{TLS: true}
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg.MinVersion != tls.VersionTLS13 {
		t.Errorf("MinVersion = %d, want TLS 1.3 (%d)", tlsCfg.MinVersion, tls.VersionTLS13)
	}
}

func TestBuildTLSConfig_InvalidMinVersion(t *testing.T) {
	cfg := &Config{TLS: true, TLSMinVersion: "1.0"}
	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error for invalid min version")
	}
}

func TestBuildTLSConfig_SkipVerifyRequiresEnv(t *testing.T) {
	t.Setenv("KSCORE_ALLOW_INSECURE_TLS", "")
	cfg := &Config{TLS: true, TLSSkipVerify: true}
	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error when KSCORE_ALLOW_INSECURE_TLS not set")
	}
}

func TestBuildTLSConfig_SkipVerifyWithEnv(t *testing.T) {
	t.Setenv("KSCORE_ALLOW_INSECURE_TLS", "1")
	cfg := &Config{TLS: true, TLSSkipVerify: true}
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !tlsCfg.InsecureSkipVerify {
		t.Error("InsecureSkipVerify should be true")
	}
}

func TestBuildTLSConfig_ServerName(t *testing.T) {
	cfg := &Config{TLS: true, TLSServerName: "myserver"}
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg.ServerName != "myserver" {
		t.Errorf("ServerName = %q, want %q", tlsCfg.ServerName, "myserver")
	}
}

func TestBuildTLSConfig_CACert(t *testing.T) {
	// Generate a self-signed CA cert for testing
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{Organization: []string{"Test"}},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}
	caPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	tmpDir := t.TempDir()
	caFile := filepath.Join(tmpDir, "ca.pem")
	if err := os.WriteFile(caFile, caPEM, 0o600); err != nil {
		t.Fatalf("write ca: %v", err)
	}

	cfg := &Config{TLS: true, TLSCACert: caFile}
	tlsCfg, err := buildTLSConfig(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tlsCfg.RootCAs == nil {
		t.Error("RootCAs should not be nil")
	}
}

func TestBuildTLSConfig_CACertNotFound(t *testing.T) {
	cfg := &Config{TLS: true, TLSCACert: "/nonexistent/ca.pem"}
	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error for missing CA cert")
	}
}

func TestBuildTLSConfig_MutualTLSRequiresBoth(t *testing.T) {
	cfg := &Config{TLS: true, TLSCert: "/some/cert.pem"}
	_, err := buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error when only cert is provided")
	}

	cfg = &Config{TLS: true, TLSKey: "/some/key.pem"}
	_, err = buildTLSConfig(cfg)
	if err == nil {
		t.Error("expected error when only key is provided")
	}
}

func TestCreateSecretsClient_Insecure(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090"}
	client, err := createSecretsClient(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer client.Close()
}

// =============================================================================
// Get Command Tests
// =============================================================================

func TestGetCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"get"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no path argument provided")
	}
}

func TestGetCmd_Flags(t *testing.T) {
	cfg := &Config{}
	cmd := newGetCmd(cfg)

	if cmd.Flags().Lookup("version") == nil {
		t.Error("expected --version flag")
	}
	if cmd.Flags().Lookup("field") == nil {
		t.Error("expected --field flag")
	}
}

// =============================================================================
// List Command Tests
// =============================================================================

func TestListCmd_Aliases(t *testing.T) {
	cfg := &Config{}
	cmd := newListCmd(cfg)
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
	}
}

func TestListCmd_Flags(t *testing.T) {
	cfg := &Config{}
	cmd := newListCmd(cfg)

	if cmd.Flags().Lookup("limit") == nil {
		t.Error("expected --limit flag")
	}
	if cmd.Flags().Lookup("show-metadata") == nil {
		t.Error("expected --show-metadata flag")
	}
}

// =============================================================================
// Backends Command Tests (wired to REST API)
// =============================================================================

func setupSecretsTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/secrets/backends", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"backends": []map[string]interface{}{
				{"name": "vault", "type": "vault", "healthy": true},
			},
			"total": 1,
		})
	})
	mux.HandleFunc("/api/v1/audit/logs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"events": []map[string]interface{}{
				{"secret_path": "vault/secret/db", "action": "read", "success": true, "timestamp": "2025-01-01T00:00:00Z"},
			},
			"total": 1,
		})
	})
	mux.HandleFunc("/api/v1/secrets/cache/stats", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"entries": 5, "max_entries": 100, "hits": 50, "misses": 10,
			"evictions": 2, "expired_count": 1, "memory_bytes": 1024,
		})
	})
	mux.HandleFunc("/api/v1/secrets/cache", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"message": "cache cleared", "cleared": 5,
		})
	})
	mux.HandleFunc("/api/v1/rotations/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/rotations/")
		parts := strings.SplitN(path, "/", 2)
		rotID := parts[0]
		if len(parts) == 2 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"rotation_id": rotID, "action": parts[1], "success": true,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": rotID, "secret_path": "vault/secret/db", "state": "in_progress",
			"strategy": "rolling", "total_targets": 10, "updated_targets": 6,
			"failed_targets": 0, "started_at": "2025-01-01T00:00:00Z",
		})
	})
	mux.HandleFunc("/api/v1/rotations", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusAccepted)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "rot-new123", "secret_path": "vault/secret/db", "state": "pending",
				"strategy": "rolling", "total_targets": 1, "started_at": "2025-01-01T00:00:00Z",
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"rotations": []map[string]interface{}{
				{"id": "rot-abc123", "secret_path": "vault/secret/db", "state": "in_progress",
					"strategy": "rolling", "total_targets": 10, "updated_targets": 6,
					"started_at": "2025-01-01T00:00:00Z"},
			},
			"total": 1,
		})
	})
	mux.HandleFunc("/api/v1/transit/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ciphertext": "vault:v2:rewrapped", "key_version": 2,
		})
	})
	mux.HandleFunc("/api/v1/secrets/rotation/policies/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		path := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets/rotation/policies/")
		parts := strings.SplitN(path, "/", 2)
		policyID := parts[0]
		if len(parts) == 2 {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"policy_id": policyID, "action": parts[1], "success": true,
			})
			return
		}
		if r.Method == http.MethodDelete {
			json.NewEncoder(w).Encode(map[string]interface{}{
				"policy_id": policyID, "action": "delete", "success": true,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id": policyID, "name": "test-policy", "max_age": "90d",
			"schedule": "0 2 * * *", "enabled": true, "auto_rotate": true,
			"credential_types": []string{"password"},
		})
	})
	mux.HandleFunc("/api/v1/secrets/rotation/policies", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": "pol-new123", "name": "new-policy", "max_age": "90d",
				"schedule": "0 2 * * *", "enabled": true,
			})
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"policies": []map[string]interface{}{
				{"id": "pol-001", "name": "db-policy", "max_age": "90d",
					"schedule": "0 2 * * *", "enabled": true, "auto_rotate": true},
			},
			"total": 1,
		})
	})
	return httptest.NewServer(mux)
}

func TestBackendsCmd_Table(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"backends", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBackendsCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"backends", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBackendsCmd_YAML(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"backends", "-o", "yaml", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Audit Command Tests (wired to REST API)
// =============================================================================

func TestAuditCmd_Table(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_YAML(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod", "-o", "yaml", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_WithLimit(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod", "--limit", "3", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no path argument provided")
	}
}

func TestAuditCmd_Flags(t *testing.T) {
	cfg := &Config{}
	cmd := newAuditCmd(cfg)
	if cmd.Flags().Lookup("limit") == nil {
		t.Error("expected --limit flag")
	}
}

// =============================================================================
// Display Type Tests (kept types)
// =============================================================================

func TestTransitResult_JSON(t *testing.T) {
	tr := &transitResult{
		KeyName:    "transit/mykey",
		Operation:  "encrypt",
		Ciphertext: "vault:v1:abc123",
		KeyVersion: 1,
		Context:    "app=web",
	}

	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded transitResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Operation != "encrypt" {
		t.Errorf("Operation = %q, want %q", decoded.Operation, "encrypt")
	}
	if decoded.KeyVersion != 1 {
		t.Errorf("KeyVersion = %d, want 1", decoded.KeyVersion)
	}
}

func TestTransitResult_YAML(t *testing.T) {
	tr := &transitResult{
		KeyName:    "transit/mykey",
		Operation:  "rewrap",
		Ciphertext: "vault:v2:abc123",
		KeyVersion: 2,
	}

	data, err := yaml.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded transitResult
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Operation != "rewrap" {
		t.Errorf("Operation = %q, want %q", decoded.Operation, "rewrap")
	}
}

// =============================================================================
// REST Client Helper Tests
// =============================================================================

func TestCreateRESTClient(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090"}
	client := createRESTClient(cfg)
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.baseURL != "http://localhost:8443" {
		t.Errorf("expected baseURL 'http://localhost:8443', got %q", client.baseURL)
	}
}

func TestCreateRESTClient_TLS(t *testing.T) {
	cfg := &Config{ServerAddr: "server.example.com:9090", TLS: true}
	client := createRESTClient(cfg)
	if client.baseURL != "https://server.example.com:8443" {
		t.Errorf("expected baseURL 'https://server.example.com:8443', got %q", client.baseURL)
	}
}

// =============================================================================
// Helper Tests
// =============================================================================

func TestTruncate(t *testing.T) {
	tests := []struct {
		input    string
		maxLen   int
		expected string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "he..."},
		{"", 5, ""},
		{"abc", 3, "abc"},
		{"abcdef", 6, "abcdef"},
		{"abcdefg", 6, "abc..."},
	}

	for _, tt := range tests {
		result := truncate(tt.input, tt.maxLen)
		if result != tt.expected {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.maxLen, result, tt.expected)
		}
	}
}

func TestRandomID(t *testing.T) {
	id := randomID(8)
	if len(id) != 8 {
		t.Errorf("randomID(8) length = %d, want 8", len(id))
	}
}

func TestNormalizeStrategy(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"blue-green", "blue_green"},
		{"rolling", "rolling"},
		{"canary", "canary"},
	}

	for _, tt := range tests {
		result := normalizeStrategy(tt.input)
		if string(result) != tt.expected {
			t.Errorf("normalizeStrategy(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{0, "0 B"},
		{512, "512 B"},
		{1024, "1.0 KB"},
		{245760, "240.0 KB"},
		{1048576, "1.0 MB"},
	}

	for _, tt := range tests {
		result := formatBytes(tt.input)
		if result != tt.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

func TestDataKeys(t *testing.T) {
	data := map[string]interface{}{
		"username": "admin",
		"password": "secret",
	}
	keys := dataKeys(data)
	if len(keys) != 2 {
		t.Errorf("expected 2 keys, got %d", len(keys))
	}

	keys = dataKeys(nil)
	if len(keys) != 0 {
		t.Errorf("expected 0 keys for nil map, got %d", len(keys))
	}
}

// =============================================================================
// Dynamic Command Tests
// =============================================================================

func TestDynamicCmd_HasSubcommands(t *testing.T) {
	cfg := &Config{}
	cmd := newDynamicCmd(cfg)
	expected := []string{"list", "get", "revoke"}

	for _, name := range expected {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestDynamicGetCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "get"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no path argument provided")
	}
}

func TestDynamicRevokeCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "revoke"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no lease-id argument provided")
	}
}

func TestDynamicListCmd_Aliases(t *testing.T) {
	cfg := &Config{}
	cmd := newDynamicListCmd(cfg)
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
	}
}

// =============================================================================
// Leases Command Tests
// =============================================================================

func TestLeasesCmd_HasSubcommands(t *testing.T) {
	cfg := &Config{}
	cmd := newLeasesCmd(cfg)
	expected := []string{"list", "revoke", "renew"}

	for _, name := range expected {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestLeasesCmd_Aliases(t *testing.T) {
	cfg := &Config{}
	cmd := newLeasesCmd(cfg)
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "lease" {
		t.Error("expected alias 'lease' not found")
	}
}

func TestLeasesRevokeCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "revoke"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no lease-id argument provided")
	}
}

func TestLeasesRenewCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "renew"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no lease-id argument provided")
	}
}

// =============================================================================
// Encrypt Command Tests
// =============================================================================

func TestEncryptCmd_RequiresKey(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"encrypt", "my-secret-data"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when --key flag is missing")
	}
}

func TestEncryptCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"encrypt", "--key", "transit/mykey"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no plaintext argument provided")
	}
}

func TestEncryptCmd_Flags(t *testing.T) {
	cfg := &Config{}
	cmd := newEncryptCmd(cfg)
	if cmd.Flags().Lookup("key") == nil {
		t.Error("expected --key flag")
	}
	if cmd.Flags().Lookup("context") == nil {
		t.Error("expected --context flag")
	}
}

// =============================================================================
// Decrypt Command Tests
// =============================================================================

func TestDecryptCmd_RequiresKey(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"decrypt", "vault:v1:bXktc2VjcmV0"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when --key flag is missing")
	}
}

func TestDecryptCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"decrypt", "--key", "transit/mykey"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no ciphertext argument provided")
	}
}

func TestDecryptCmd_Flags(t *testing.T) {
	cfg := &Config{}
	cmd := newDecryptCmd(cfg)
	if cmd.Flags().Lookup("key") == nil {
		t.Error("expected --key flag")
	}
	if cmd.Flags().Lookup("context") == nil {
		t.Error("expected --context flag")
	}
}

// =============================================================================
// Rewrap Command Tests (stub — still uses mock data)
// =============================================================================

func TestRewrapCmd_Table(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRewrapCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRewrapCmd_YAML(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey", "-o", "yaml", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRewrapCmd_RequiresKey(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "vault:v1:bXktc2VjcmV0"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when --key flag is missing")
	}
}

func TestRewrapCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "--key", "transit/mykey"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no ciphertext argument provided")
	}
}

func TestRewrapCmd_Flags(t *testing.T) {
	cfg := &Config{}
	cmd := newRewrapCmd(cfg)
	if cmd.Flags().Lookup("key") == nil {
		t.Error("expected --key flag")
	}
}

func TestRewrapCmd_NoCipherPrefix(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "raw-ciphertext-data", "--key", "transit/mykey", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Template Command Tests (stub — still uses mock data)
// =============================================================================

func TestTemplateCmd_NotYetAvailable(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for template (not yet available)")
	}
	if !strings.Contains(err.Error(), "not yet available") {
		t.Errorf("expected 'not yet available' error, got: %v", err)
	}
}

func TestTemplateCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no template file argument provided")
	}
}

func TestTemplateCmd_Flags(t *testing.T) {
	cfg := &Config{}
	cmd := newTemplateCmd(cfg)
	if cmd.Flags().Lookup("out-file") == nil {
		t.Error("expected --out-file flag")
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("expected --dry-run flag")
	}
}

// =============================================================================
// Cache Command Tests (stub — still uses mock data)
// =============================================================================

func TestCacheCmd_HasSubcommands(t *testing.T) {
	cfg := &Config{}
	cmd := newCacheCmd(cfg)
	expected := []string{"status", "clear", "list"}

	for _, name := range expected {
		found := false
		for _, sub := range cmd.Commands() {
			if sub.Name() == name {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", name)
		}
	}
}

func TestCacheStatusCmd_Table(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "status", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheStatusCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "status", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheStatusCmd_YAML(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "status", "-o", "yaml", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheClearCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "clear", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_Table(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "list", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "list", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_YAML(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "list", "-o", "yaml", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_Aliases(t *testing.T) {
	cfg := &Config{}
	cmd := newCacheListCmd(cfg)
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
	}
}

// =============================================================================
// Rotate Command Tests (wired to REST API)
// =============================================================================

func TestRotateListCmd_Table(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "list", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateListCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "list", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateShowCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "show", "rot-abc123", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateStatusCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "status", "rot-abc123", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateStatusCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "status", "rot-abc123", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateStartCmd_DryRun(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "start", "--secret", "vault/secret/db", "--target", "agent-1", "--dry-run"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateStartCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "start", "--secret", "vault/secret/db", "--target", "agent-1", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateHistoryCmd(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "history"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for history (not yet available)")
	}
}

func TestRotateCancelCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "cancel", "rot-abc123", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateRollbackCmd_NoForce(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "rollback", "rot-abc123"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateRollbackCmd_Force(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "rollback", "rot-abc123", "--force", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotatePauseCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "pause", "rot-abc123", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateResumeCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "resume", "rot-abc123", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRotateTriggerCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate", "trigger", "rot-abc123", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Rotate Keys Command Tests (stub — still uses mock data)
// =============================================================================

func TestNewRotateKeysCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newRotateKeysCmd(cfg)
	if cmd == nil {
		t.Fatal("newRotateKeysCmd should not return nil")
	}
	if cmd.Use != "rotate-keys" {
		t.Errorf("Use = %v, want rotate-keys", cmd.Use)
	}

	if cmd.Flags().Lookup("force") == nil {
		t.Error("expected flag 'force' not found")
	}
}

func TestRotateKeysNotYetAvailable(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rotate-keys"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for rotate-keys (not yet available)")
	}
	if !strings.Contains(err.Error(), "not yet available") {
		t.Errorf("expected 'not yet available' error, got: %v", err)
	}
}

func TestRotateKeysHelp(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"rotate-keys", "--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "encryption keys") {
		t.Errorf("expected help text about encryption keys, got: %s", output)
	}
}

// =============================================================================
// Schedule Command Tests (wired to REST API)
// =============================================================================

func TestScheduleListCmd_Table(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"schedule", "list", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduleListCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"schedule", "list", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduleShowCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"schedule", "show", "pol-001", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduleShowCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"schedule", "show", "pol-001", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduleCreateCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"schedule", "create", "--secret", "vault/secret/db",
		"--schedule", "0 2 * * *", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduleEnableCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"schedule", "enable", "pol-001", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduleDisableCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"schedule", "disable", "pol-001", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduleDeleteCmd_NoForce(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"schedule", "delete", "pol-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestScheduleDeleteCmd_Force(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"schedule", "delete", "pol-001", "--force", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Policy Command Tests (wired to REST API)
// =============================================================================

func TestPolicyListCmd_Table(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"policy", "list", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyListCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"policy", "list", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyShowCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"policy", "show", "pol-001", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyShowCmd_JSON(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"policy", "show", "pol-001", "-o", "json", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyCreateCmd(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"policy", "create", "--name", "db-policy",
		"--pattern", "vault/secret/db/*", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyDeleteCmd_NoForce(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"policy", "delete", "pol-001"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPolicyDeleteCmd_Force(t *testing.T) {
	ts := setupSecretsTestServer(t)
	defer ts.Close()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"policy", "delete", "pol-001", "--force", "--rest-addr", ts.URL})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// Suppress unused import warnings for yaml
var _ = yaml.Marshal
