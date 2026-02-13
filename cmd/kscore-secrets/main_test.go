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
// Backends Command Tests (stub — still uses mock data)
// =============================================================================

func TestBackendsCmd_Table(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"backends"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBackendsCmd_JSON(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"backends", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBackendsCmd_YAML(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"backends", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Audit Command Tests (stub — still uses mock data)
// =============================================================================

func TestAuditCmd_Table(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_JSON(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_YAML(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_WithLimit(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod", "--limit", "3"})

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

func TestBackendDisplay_JSON(t *testing.T) {
	b := &backendDisplay{
		Name:        "vault",
		Type:        "hashicorp-vault",
		Status:      "healthy",
		SecretCount: 42,
		Address:     "https://vault.example.com",
		AuthMethod:  "approle",
	}

	data, err := json.Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded backendDisplay
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Name != "vault" {
		t.Errorf("Name = %q, want %q", decoded.Name, "vault")
	}
	if decoded.SecretCount != 42 {
		t.Errorf("SecretCount = %d, want 42", decoded.SecretCount)
	}
}

func TestAuditEntry_JSON(t *testing.T) {
	e := &auditEntry{
		Timestamp: "2025-01-01T00:00:00Z",
		Path:      "vault/secret/db",
		Action:    "read",
		Principal: "agent/web-01",
		SourceIP:  "10.0.1.15",
		Version:   3,
	}

	data, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded auditEntry
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Action != "read" {
		t.Errorf("Action = %q, want %q", decoded.Action, "read")
	}
	if decoded.Version != 3 {
		t.Errorf("Version = %d, want 3", decoded.Version)
	}
}

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

func TestTemplateResult_JSON(t *testing.T) {
	tr := &templateResult{
		TemplateFile: "config.tmpl",
		OutputFile:   "config.yaml",
		DryRun:       true,
		SecretRefs: []templateSecretRef{
			{Path: "vault/secret/db", Field: "password", Line: 5},
		},
		RefCount: 1,
	}

	data, err := json.Marshal(tr)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded templateResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.TemplateFile != "config.tmpl" {
		t.Errorf("TemplateFile = %q, want %q", decoded.TemplateFile, "config.tmpl")
	}
	if decoded.RefCount != 1 {
		t.Errorf("RefCount = %d, want 1", decoded.RefCount)
	}
	if len(decoded.SecretRefs) != 1 {
		t.Errorf("SecretRefs count = %d, want 1", len(decoded.SecretRefs))
	}
}

func TestCacheStatusDisplay_JSON(t *testing.T) {
	cs := &cacheStatusDisplay{
		Entries:      47,
		MaxEntries:   10000,
		HitRate:      87.3,
		Hits:         1523,
		Misses:       221,
		Evictions:    12,
		ExpiredCount: 34,
		MemoryBytes:  245760,
		DefaultTTL:   "5m",
		Encrypted:    true,
	}

	data, err := json.Marshal(cs)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded cacheStatusDisplay
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Entries != 47 {
		t.Errorf("Entries = %d, want 47", decoded.Entries)
	}
	if decoded.HitRate != 87.3 {
		t.Errorf("HitRate = %f, want 87.3", decoded.HitRate)
	}
	if !decoded.Encrypted {
		t.Error("Encrypted should be true")
	}
}

func TestCacheEntryDisplay_JSON(t *testing.T) {
	ce := &cacheEntryDisplay{
		Path:     "vault/secret/db",
		Backend:  "vault",
		CachedAt: "12:00:00",
		TTL:      "5m",
		Hits:     10,
	}

	data, err := json.Marshal(ce)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded cacheEntryDisplay
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Hits != 10 {
		t.Errorf("Hits = %d, want 10", decoded.Hits)
	}
}

// =============================================================================
// Sample Data Generator Tests (kept generators)
// =============================================================================

func TestGenerateSampleBackends(t *testing.T) {
	backends := generateSampleBackends()
	if len(backends) != 3 {
		t.Errorf("expected 3 backends, got %d", len(backends))
	}

	names := make(map[string]bool)
	for _, b := range backends {
		if b.Name == "" {
			t.Error("expected non-empty Name")
		}
		if b.Type == "" {
			t.Error("expected non-empty Type")
		}
		if b.Status == "" {
			t.Error("expected non-empty Status")
		}
		names[b.Name] = true
	}

	for _, expected := range []string{"vault", "aws-sm", "azure-kv"} {
		if !names[expected] {
			t.Errorf("expected backend %q not found", expected)
		}
	}
}

func TestGenerateSampleAuditEntries(t *testing.T) {
	entries := generateSampleAuditEntries("vault/secret/db", 20)
	if len(entries) != 7 {
		t.Errorf("expected 7 entries (sample set size), got %d", len(entries))
	}

	for _, e := range entries {
		if e.Path != "vault/secret/db" {
			t.Errorf("Path = %q, want %q", e.Path, "vault/secret/db")
		}
		if e.Action == "" {
			t.Error("expected non-empty Action")
		}
		if e.Principal == "" {
			t.Error("expected non-empty Principal")
		}
	}

	entries = generateSampleAuditEntries("vault/secret/db", 3)
	if len(entries) != 3 {
		t.Errorf("expected 3 entries with limit=3, got %d", len(entries))
	}
}

func TestGenerateSampleCacheEntries(t *testing.T) {
	items := generateSampleCacheEntries()
	if len(items) != 3 {
		t.Errorf("expected 3 items, got %d", len(items))
	}
	for _, item := range items {
		if item.Path == "" {
			t.Error("expected non-empty Path")
		}
		if item.Backend == "" {
			t.Error("expected non-empty Backend")
		}
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
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRewrapCmd_JSON(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRewrapCmd_YAML(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey", "-o", "yaml"})

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
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "raw-ciphertext-data", "--key", "transit/mykey"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Template Command Tests (stub — still uses mock data)
// =============================================================================

func TestTemplateCmd_Table(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_JSON(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_YAML(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_DryRun(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_DryRunJSON(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "--dry-run", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_DryRunYAML(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "--dry-run", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_WithOutFile(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "--out-file", "config.yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheStatusCmd_JSON(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "status", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheStatusCmd_YAML(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "status", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheClearCmd(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "clear"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_Table(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_JSON(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "list", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_YAML(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "list", "-o", "yaml"})

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

func TestRotateKeysForce(t *testing.T) {
	cmd := newRootCmd()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"rotate-keys", "--force"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	output := buf.String()
	if !strings.Contains(output, "Encryption keys rotated") {
		t.Errorf("expected rotation message, got: %s", output)
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

// Suppress unused import warnings for yaml
var _ = yaml.Marshal
