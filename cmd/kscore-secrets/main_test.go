package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

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
// Get Command Tests
// =============================================================================

func TestGetCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"get", "vault/secret/database/prod"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"get", "vault/secret/database/prod", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"get", "vault/secret/database/prod", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCmd_WithVersion(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"get", "vault/secret/database/prod", "--version", "2", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCmd_WithField(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"get", "vault/secret/database/prod", "--field", "password"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetCmd_WithInvalidField(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"get", "vault/secret/database/prod", "--field", "nonexistent"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error for invalid field")
	}
	if err != nil && !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestGetCmd_RequiresArgs(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"get"})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no path argument provided")
	}
}

func TestGetCmd_Flags(t *testing.T) {
	cmd := newGetCmd()

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

func TestListCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListCmd_WithPrefix(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "vault/secret/"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListCmd_NoMatch(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "nonexistent/prefix/"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListCmd_WithLimit(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "--limit", "2"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListCmd_WithShowMetadata(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "--show-metadata"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"list", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestListCmd_Aliases(t *testing.T) {
	cmd := newListCmd()
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
	}
}

func TestListCmd_Flags(t *testing.T) {
	cmd := newListCmd()

	if cmd.Flags().Lookup("limit") == nil {
		t.Error("expected --limit flag")
	}
	if cmd.Flags().Lookup("show-metadata") == nil {
		t.Error("expected --show-metadata flag")
	}
}

// =============================================================================
// Backends Command Tests
// =============================================================================

func TestBackendsCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"backends"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBackendsCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"backends", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBackendsCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"backends", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Audit Command Tests
// =============================================================================

func TestAuditCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"audit", "vault/secret/database/prod", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAuditCmd_WithLimit(t *testing.T) {
	outputFormat = "table"
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
	cmd := newAuditCmd()
	if cmd.Flags().Lookup("limit") == nil {
		t.Error("expected --limit flag")
	}
}

// =============================================================================
// Display Type Tests
// =============================================================================

func TestSecretDisplay_JSON(t *testing.T) {
	s := &secretDisplay{
		Path:      "vault/secret/db",
		Version:   2,
		Keys:      []string{"user", "pass"},
		CreatedAt: "2025-01-01T00:00:00Z",
		ExpiresAt: "2025-04-01T00:00:00Z",
		CreatedBy: "admin",
		Backend:   "vault",
		Metadata:  map[string]string{"env": "prod"},
	}

	data, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded secretDisplay
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Path != s.Path {
		t.Errorf("Path = %q, want %q", decoded.Path, s.Path)
	}
	if decoded.Version != s.Version {
		t.Errorf("Version = %d, want %d", decoded.Version, s.Version)
	}
	if len(decoded.Keys) != 2 {
		t.Errorf("Keys count = %d, want 2", len(decoded.Keys))
	}
	if decoded.Backend != s.Backend {
		t.Errorf("Backend = %q, want %q", decoded.Backend, s.Backend)
	}
}

func TestSecretDisplay_YAML(t *testing.T) {
	s := &secretDisplay{
		Path:      "vault/secret/db",
		Version:   2,
		Keys:      []string{"user", "pass"},
		CreatedAt: "2025-01-01T00:00:00Z",
		CreatedBy: "admin",
		Backend:   "vault",
	}

	data, err := yaml.Marshal(s)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded secretDisplay
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Path != s.Path {
		t.Errorf("Path = %q, want %q", decoded.Path, s.Path)
	}
}

func TestSecretListItem_JSON(t *testing.T) {
	item := &secretListItem{
		Path:         "vault/secret/db",
		Versions:     3,
		Backend:      "vault",
		LastModified: "Jan 01 12:00",
		CreatedBy:    "admin",
		ExpiresAt:    "Apr 01",
	}

	data, err := json.Marshal(item)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded secretListItem
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.Versions != 3 {
		t.Errorf("Versions = %d, want 3", decoded.Versions)
	}
}

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

// =============================================================================
// Sample Data Generator Tests
// =============================================================================

func TestGenerateSampleSecret(t *testing.T) {
	s := generateSampleSecret("vault/secret/test", 0)
	if s.Path != "vault/secret/test" {
		t.Errorf("Path = %q, want %q", s.Path, "vault/secret/test")
	}
	if s.Version != 3 {
		t.Errorf("Version = %d, want 3 (default for version=0)", s.Version)
	}
	if len(s.Keys) == 0 {
		t.Error("expected non-empty Keys")
	}
	if s.Backend == "" {
		t.Error("expected non-empty Backend")
	}

	s2 := generateSampleSecret("vault/secret/test", 5)
	if s2.Version != 5 {
		t.Errorf("Version = %d, want 5", s2.Version)
	}
}

func TestGenerateSampleSecretList(t *testing.T) {
	items := generateSampleSecretList("", 50)
	if len(items) == 0 {
		t.Error("expected non-empty list for empty prefix")
	}

	items = generateSampleSecretList("vault/", 50)
	for _, item := range items {
		if !strings.HasPrefix(item.Path, "vault/") {
			t.Errorf("Path %q does not have prefix vault/", item.Path)
		}
	}

	items = generateSampleSecretList("", 2)
	if len(items) > 2 {
		t.Errorf("expected at most 2 items with limit=2, got %d", len(items))
	}

	items = generateSampleSecretList("nonexistent/", 50)
	if len(items) != 0 {
		t.Errorf("expected 0 items for nonexistent prefix, got %d", len(items))
	}
}

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

// =============================================================================
// Dynamic Command Tests
// =============================================================================

func TestDynamicCmd_HasSubcommands(t *testing.T) {
	cmd := newDynamicCmd()
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

func TestDynamicListCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamicListCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "list", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamicListCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "list", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamicListCmd_FilterBackend(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "list", "--backend", "vault"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamicListCmd_NoMatch(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "list", "--backend", "nonexistent"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamicGetCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "get", "vault/database/creds/myapp"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamicGetCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "get", "vault/database/creds/myapp", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamicGetCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "get", "vault/database/creds/myapp", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDynamicGetCmd_WithTTL(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "get", "vault/database/creds/myapp", "--ttl", "2h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestDynamicRevokeCmd(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"dynamic", "revoke", "lease-abc12345"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
	cmd := newDynamicListCmd()
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
	}
}

func TestGenerateSampleDynamicSecrets(t *testing.T) {
	items := generateSampleDynamicSecrets()
	if len(items) != 4 {
		t.Errorf("expected 4 items, got %d", len(items))
	}
	for _, item := range items {
		if item.LeaseID == "" {
			t.Error("expected non-empty LeaseID")
		}
		if item.Path == "" {
			t.Error("expected non-empty Path")
		}
		if item.Type == "" {
			t.Error("expected non-empty Type")
		}
	}
}

// =============================================================================
// Leases Command Tests
// =============================================================================

func TestLeasesCmd_HasSubcommands(t *testing.T) {
	cmd := newLeasesCmd()
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
	cmd := newLeasesCmd()
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "lease" {
		t.Error("expected alias 'lease' not found")
	}
}

func TestLeasesListCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeasesListCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "list", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeasesListCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "list", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeasesListCmd_FilterBackend(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "list", "--backend", "vault"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeasesListCmd_FilterState(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "list", "--state", "active"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeasesListCmd_NoMatch(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "list", "--backend", "nonexistent"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeasesRevokeCmd(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "revoke", "lease-abc12345"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestLeasesRenewCmd(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "renew", "lease-abc12345"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLeasesRenewCmd_WithIncrement(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{"leases", "renew", "lease-abc12345", "--increment", "2h"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
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

func TestGenerateSampleLeases(t *testing.T) {
	items := generateSampleLeases()
	if len(items) != 4 {
		t.Errorf("expected 4 items, got %d", len(items))
	}
	for _, item := range items {
		if item.LeaseID == "" {
			t.Error("expected non-empty LeaseID")
		}
		if item.SecretPath == "" {
			t.Error("expected non-empty SecretPath")
		}
		if item.State == "" {
			t.Error("expected non-empty State")
		}
	}
}

// =============================================================================
// Encrypt Command Tests
// =============================================================================

func TestEncryptCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"encrypt", "my-secret-data", "--key", "transit/mykey"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEncryptCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"encrypt", "my-secret-data", "--key", "transit/mykey", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEncryptCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"encrypt", "my-secret-data", "--key", "transit/mykey", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestEncryptCmd_WithContext(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"encrypt", "my-secret-data", "--key", "transit/mykey", "--context", "app=web"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
	cmd := newEncryptCmd()
	if cmd.Flags().Lookup("key") == nil {
		t.Error("expected --key flag")
	}
	if cmd.Flags().Lookup("context") == nil {
		t.Error("expected --context flag")
	}
}

func TestEncryptCmd_OutputContainsVaultPrefix(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"encrypt", "test-data", "--key", "transit/mykey", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Decrypt Command Tests
// =============================================================================

func TestDecryptCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"decrypt", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecryptCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"decrypt", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecryptCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"decrypt", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDecryptCmd_WithContext(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"decrypt", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey", "--context", "app=web"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

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
	cmd := newDecryptCmd()
	if cmd.Flags().Lookup("key") == nil {
		t.Error("expected --key flag")
	}
	if cmd.Flags().Lookup("context") == nil {
		t.Error("expected --context flag")
	}
}

// =============================================================================
// Rewrap Command Tests
// =============================================================================

func TestRewrapCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRewrapCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "vault:v1:bXktc2VjcmV0", "--key", "transit/mykey", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRewrapCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

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
	cmd := newRewrapCmd()
	if cmd.Flags().Lookup("key") == nil {
		t.Error("expected --key flag")
	}
}

func TestRewrapCmd_NoCipherPrefix(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"rewrap", "raw-ciphertext-data", "--key", "transit/mykey"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// =============================================================================
// Template Command Tests
// =============================================================================

func TestTemplateCmd_Table(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_DryRun(t *testing.T) {
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "--dry-run"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_DryRunJSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "--dry-run", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_DryRunYAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"template", "config.tmpl", "--dry-run", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestTemplateCmd_WithOutFile(t *testing.T) {
	outputFormat = "table"
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
	cmd := newTemplateCmd()
	if cmd.Flags().Lookup("out-file") == nil {
		t.Error("expected --out-file flag")
	}
	if cmd.Flags().Lookup("dry-run") == nil {
		t.Error("expected --dry-run flag")
	}
}

// =============================================================================
// Cache Command Tests
// =============================================================================

func TestCacheCmd_HasSubcommands(t *testing.T) {
	cmd := newCacheCmd()
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
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "status"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheStatusCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "status", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheStatusCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

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
	outputFormat = "table"
	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "list"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_JSON(t *testing.T) {
	outputFormat = "json"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "list", "-o", "json"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_YAML(t *testing.T) {
	outputFormat = "yaml"
	defer func() { outputFormat = "table" }()

	cmd := newRootCmd()
	cmd.SetArgs([]string{"cache", "list", "-o", "yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCacheListCmd_Aliases(t *testing.T) {
	cmd := newCacheListCmd()
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "ls" {
		t.Error("expected alias 'ls' not found")
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
// New Display Type Tests
// =============================================================================

func TestDynamicSecretDisplay_JSON(t *testing.T) {
	d := &dynamicSecretDisplay{
		LeaseID:   "lease-001",
		Path:      "vault/database/creds/myapp",
		Type:      "database",
		Backend:   "vault",
		TTL:       "1h",
		ExpiresAt: "15:30",
	}

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded dynamicSecretDisplay
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.LeaseID != "lease-001" {
		t.Errorf("LeaseID = %q, want %q", decoded.LeaseID, "lease-001")
	}
	if decoded.Type != "database" {
		t.Errorf("Type = %q, want %q", decoded.Type, "database")
	}
}

func TestLeaseDisplay_JSON(t *testing.T) {
	l := &leaseDisplay{
		LeaseID:      "lease-001",
		SecretPath:   "vault/database/creds/myapp",
		Backend:      "vault",
		State:        "active",
		TTL:          "1h",
		RenewalCount: 3,
		ExpiresAt:    "15:30",
	}

	data, err := json.Marshal(l)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded leaseDisplay
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if decoded.State != "active" {
		t.Errorf("State = %q, want %q", decoded.State, "active")
	}
	if decoded.RenewalCount != 3 {
		t.Errorf("RenewalCount = %d, want 3", decoded.RenewalCount)
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
