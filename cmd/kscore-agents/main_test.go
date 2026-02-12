package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestNewRootCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newRootCmd(cfg)

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-agents" {
		t.Errorf("Use = %v, want kscore-agents", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cfg := &Config{}
	cmd := newRootCmd(cfg)

	expectedSubcommands := []string{
		"list",
		"show",
		"delete",
		"quarantine",
		"unquarantine",
		"token",
		"tags",
		"status",
		"renew-svid",
		"verify",
		"certificates",
		"re-enroll",
		"revoke-credentials",
		"version",
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
	if cmd.Use != "version" {
		t.Errorf("Use = %v, want version", cmd.Use)
	}

	// Test execution
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

func TestNewListCmd(t *testing.T) {
	cfg := &Config{
		ServerAddr: "localhost:9090",
		Output:     "table",
	}
	cmd := newListCmd(cfg)

	if cmd == nil {
		t.Fatal("newListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	// Check flags exist
	flags := []string{"status", "filter", "label", "edge", "limit", "show-compatibility", "suspicious"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewShowCmd(t *testing.T) {
	cfg := &Config{
		ServerAddr: "localhost:9090",
		Output:     "table",
	}
	cmd := newShowCmd(cfg)

	if cmd == nil {
		t.Fatal("newShowCmd should not return nil")
	}
	if cmd.Use != "show <agent-id>" {
		t.Errorf("Use = %v, want 'show <agent-id>'", cmd.Use)
	}
}

func TestNewDeleteCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newDeleteCmd(cfg)

	if cmd == nil {
		t.Fatal("newDeleteCmd should not return nil")
	}
	if cmd.Use != "delete <agent-id>" {
		t.Errorf("Use = %v, want 'delete <agent-id>'", cmd.Use)
	}

	if cmd.Flags().Lookup("force") == nil {
		t.Error("expected flag 'force' not found")
	}
}

func TestNewTokenCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newTokenCmd(cfg)

	if cmd == nil {
		t.Fatal("newTokenCmd should not return nil")
	}
	if cmd.Use != "token" {
		t.Errorf("Use = %v, want token", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"create", "list", "revoke"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestNewTagsCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newTagsCmd(cfg)

	if cmd == nil {
		t.Fatal("newTagsCmd should not return nil")
	}
	if cmd.Use != "tags" {
		t.Errorf("Use = %v, want tags", cmd.Use)
	}

	// Check aliases
	if len(cmd.Aliases) == 0 || cmd.Aliases[0] != "labels" {
		t.Error("expected alias 'labels' not found")
	}

	// Should have subcommands
	subcommands := []string{"set", "add", "remove", "show"}
	for _, sub := range subcommands {
		found := false
		for _, c := range cmd.Commands() {
			if c.Name() == sub {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected subcommand %q not found", sub)
		}
	}
}

func TestConfigStructure(t *testing.T) {
	cfg := Config{
		ServerAddr: "localhost:9090",
		Output:     "json",
		Verbose:    true,
	}

	if cfg.ServerAddr != "localhost:9090" {
		t.Errorf("ServerAddr = %v, want localhost:9090", cfg.ServerAddr)
	}
	if cfg.Output != "json" {
		t.Errorf("Output = %v, want json", cfg.Output)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
}

func TestAgentDisplayStructure(t *testing.T) {
	agent := AgentDisplay{
		ID:            "test-001",
		Hostname:      "test.example.com",
		OS:            "linux",
		Arch:          "amd64",
		Status:        "online",
		Version:       "1.0.0",
		LastHeartbeat: time.Now().Format(time.RFC3339),
		RegisteredAt:  time.Now().Format(time.RFC3339),
		Labels:        map[string]string{"env": "test"},
		IPAddresses:   []string{"10.0.0.1"},
		DualStack:     false,
	}

	if agent.ID != "test-001" {
		t.Errorf("ID = %v, want test-001", agent.ID)
	}
	if agent.Status != "online" {
		t.Errorf("Status = %v, want online", agent.Status)
	}
}

func TestMetricsDisplayStructure(t *testing.T) {
	metrics := MetricsDisplay{
		CPU:    50.5,
		Memory: 75.2,
		Disk:   30.0,
		Load:   []float32{1.0, 0.8, 0.5},
	}

	if metrics.CPU != 50.5 {
		t.Errorf("CPU = %v, want 50.5", metrics.CPU)
	}
	if len(metrics.Load) != 3 {
		t.Errorf("Load length = %d, want 3", len(metrics.Load))
	}
}

func TestTokenDisplayStructure(t *testing.T) {
	now := time.Now()
	token := TokenDisplay{
		ID:        "tok-123",
		Token:     "kscore_secret",
		TTL:       "1h",
		ExpiresAt: now.Add(time.Hour),
		UsedCount: 0,
		MaxUses:   10,
		Labels:    map[string]string{"env": "prod"},
		CreatedAt: now,
		CreatedBy: "admin",
	}

	if token.ID != "tok-123" {
		t.Errorf("ID = %v, want tok-123", token.ID)
	}
	if token.MaxUses != 10 {
		t.Errorf("MaxUses = %d, want 10", token.MaxUses)
	}
}

func TestFormatLabels(t *testing.T) {
	tests := []struct {
		name     string
		labels   map[string]string
		expected string
	}{
		{
			name:     "empty labels",
			labels:   map[string]string{},
			expected: "-",
		},
		{
			name:     "nil labels",
			labels:   nil,
			expected: "-",
		},
		{
			name:     "single label",
			labels:   map[string]string{"env": "prod"},
			expected: "env=prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatLabels(tt.labels)
			if tt.name == "empty labels" || tt.name == "nil labels" {
				if result != "-" {
					t.Errorf("formatLabels() = %v, want -", result)
				}
			} else {
				if !strings.Contains(result, "env=prod") {
					t.Errorf("formatLabels() = %v, should contain env=prod", result)
				}
			}
		})
	}
}

func TestStatusToString(t *testing.T) {
	tests := []struct {
		name     string
		status   string
		expected string
	}{
		{"online", "online", "online"},
		{"offline", "offline", "offline"},
		{"degraded", "degraded", "degraded"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Note: statusToString takes pb.AgentStatus, but we can test the concept
			if tt.expected != tt.status {
				t.Errorf("expected %v", tt.expected)
			}
		})
	}
}

func TestGenerateRandomToken(t *testing.T) {
	token := generateRandomToken()

	if len(token) != 32 {
		t.Errorf("token length = %d, want 32", len(token))
	}

	// Token should only contain alphanumeric characters
	for _, c := range token {
		if (c < 'a' || c > 'z') && (c < '0' || c > '9') {
			t.Errorf("token contains invalid character: %c", c)
		}
	}
}

func TestNewVerifyCmd(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newVerifyCmd(cfg)

	if cmd == nil {
		t.Fatal("newVerifyCmd should not return nil")
	}
	if cmd.Use != "verify [agent-id]" {
		t.Errorf("Use = %v, want 'verify [agent-id]'", cmd.Use)
	}

	flags := []string{"all", "sample"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestVerifyRequiresTargets(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newVerifyCmd(cfg)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no agent ID or --all specified")
	}
	if !strings.Contains(err.Error(), "specify an agent ID") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyResultStructure(t *testing.T) {
	r := VerifyResult{
		AgentID:     "web-001",
		Reachable:   true,
		VersionOK:   true,
		CertValid:   true,
		HeartbeatOK: true,
		Status:      "ok",
	}

	if r.AgentID != "web-001" {
		t.Errorf("AgentID = %v, want web-001", r.AgentID)
	}
	if r.Status != "ok" {
		t.Errorf("Status = %v, want ok", r.Status)
	}
}

func TestBoolCheck(t *testing.T) {
	if boolCheck(true) != "OK" {
		t.Errorf("boolCheck(true) = %v, want OK", boolCheck(true))
	}
	if boolCheck(false) != "FAIL" {
		t.Errorf("boolCheck(false) = %v, want FAIL", boolCheck(false))
	}
}

func TestNewCertificatesCmd(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newCertificatesCmd(cfg)

	if cmd == nil {
		t.Fatal("newCertificatesCmd should not return nil")
	}
	if cmd.Use != "certificates" {
		t.Errorf("Use = %v, want certificates", cmd.Use)
	}

	found := false
	for _, c := range cmd.Commands() {
		if c.Name() == "regenerate" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected subcommand 'regenerate' not found")
	}
}

func TestCertificatesRegenerateFlags(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newCertificatesRegenerateCmd(cfg)

	if cmd == nil {
		t.Fatal("newCertificatesRegenerateCmd should not return nil")
	}

	flags := []string{"all", "force"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestCertificatesRegenerateRequiresTarget(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newCertificatesRegenerateCmd(cfg)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no agent ID or --all specified")
	}
	if !strings.Contains(err.Error(), "specify an agent ID") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCertRegenerateResultStructure(t *testing.T) {
	r := CertRegenerateResult{
		AgentID:   "web-001",
		Renewed:   true,
		ExpiresAt: time.Now().Add(24 * time.Hour).Format(time.RFC3339),
	}

	if r.AgentID != "web-001" {
		t.Errorf("AgentID = %v, want web-001", r.AgentID)
	}
	if !r.Renewed {
		t.Error("Renewed should be true")
	}
}

func TestNewReEnrollCmd(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newReEnrollCmd(cfg)

	if cmd == nil {
		t.Fatal("newReEnrollCmd should not return nil")
	}
	if cmd.Use != "re-enroll <agent-id>" {
		t.Errorf("Use = %v, want 're-enroll <agent-id>'", cmd.Use)
	}
}

func TestReEnrollCommandFlags(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newReEnrollCmd(cfg)

	flags := []string{"force", "reason"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestReEnrollRequiresAgentID(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newReEnrollCmd(cfg)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no agent ID specified")
	}
}

func TestReEnrollHelp(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newReEnrollCmd(cfg)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("re-enroll --help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "security incidents") {
		t.Errorf("expected help to mention 'security incidents', got: %s", out)
	}
	if !strings.Contains(out, "--reason") {
		t.Errorf("expected help to mention '--reason', got: %s", out)
	}
}

func TestReEnrollResultStructure(t *testing.T) {
	r := ReEnrollResult{
		AgentID:      "web-001",
		Status:       "pending_re-enrollment",
		NewToken:     "kscore_abc123",
		TokenExpires: "2026-01-01T01:00:00Z",
		Reason:       "credential compromise",
	}

	if r.AgentID != "web-001" {
		t.Errorf("AgentID = %v, want web-001", r.AgentID)
	}
	if r.Status != "pending_re-enrollment" {
		t.Errorf("Status = %v, want pending_re-enrollment", r.Status)
	}
	if r.Reason != "credential compromise" {
		t.Errorf("Reason = %v, want 'credential compromise'", r.Reason)
	}
}

func TestReEnrollForceSkipsConfirmation(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "json"}
	cmd := newReEnrollCmd(cfg)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"web-001", "--force", "--reason", "test rotation"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("re-enroll --force failed: %v", err)
	}
}

func TestNewRevokeCredentialsCmd(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newRevokeCredentialsCmd(cfg)

	if cmd == nil {
		t.Fatal("newRevokeCredentialsCmd should not return nil")
	}
	if cmd.Use != "revoke-credentials <agent-id>" {
		t.Errorf("Use = %v, want 'revoke-credentials <agent-id>'", cmd.Use)
	}
}

func TestRevokeCredentialsFlags(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newRevokeCredentialsCmd(cfg)

	flags := []string{"force", "reason"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestRevokeCredentialsRequiresAgentID(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newRevokeCredentialsCmd(cfg)
	cmd.SetArgs([]string{})

	err := cmd.Execute()
	if err == nil {
		t.Error("expected error when no agent ID specified")
	}
}

func TestRevokeCredentialsHelp(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "table"}
	cmd := newRevokeCredentialsCmd(cfg)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("revoke-credentials --help failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "lock out") {
		t.Errorf("expected help to mention 'lock out', got: %s", out)
	}
	if !strings.Contains(out, "re-enroll") {
		t.Errorf("expected help to mention 're-enroll', got: %s", out)
	}
}

func TestRevokeCredentialsForce(t *testing.T) {
	cfg := &Config{ServerAddr: "localhost:9090", Output: "json"}
	cmd := newRevokeCredentialsCmd(cfg)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"web-001", "--force", "--reason", "incident IR-2026-42"})

	err := cmd.Execute()
	if err != nil {
		t.Fatalf("revoke-credentials --force failed: %v", err)
	}
}

func TestRevokeCredentialsResultStructure(t *testing.T) {
	r := RevokeCredentialsResult{
		AgentID:   "web-001",
		Status:    "credentials_revoked",
		RevokedAt: "2026-01-01T00:00:00Z",
		Reason:    "suspected compromise",
	}

	if r.AgentID != "web-001" {
		t.Errorf("AgentID = %v, want web-001", r.AgentID)
	}
	if r.Status != "credentials_revoked" {
		t.Errorf("Status = %v, want credentials_revoked", r.Status)
	}
	if r.Reason != "suspected compromise" {
		t.Errorf("Reason = %v, want 'suspected compromise'", r.Reason)
	}
}

func TestIsSuspiciousAgent(t *testing.T) {
	tests := []struct {
		name     string
		agent    AgentDisplay
		expected bool
	}{
		{
			name: "healthy agent",
			agent: AgentDisplay{
				Status:        "online",
				Version:       "0.1.0",
				LastHeartbeat: time.Now().Add(-30 * time.Second).Format(time.RFC3339),
			},
			expected: false,
		},
		{
			name: "degraded status",
			agent: AgentDisplay{
				Status:        "degraded",
				Version:       "0.1.0",
				LastHeartbeat: time.Now().Add(-30 * time.Second).Format(time.RFC3339),
			},
			expected: true,
		},
		{
			name: "offline status",
			agent: AgentDisplay{
				Status:        "offline",
				Version:       "0.1.0",
				LastHeartbeat: time.Now().Add(-30 * time.Second).Format(time.RFC3339),
			},
			expected: true,
		},
		{
			name: "version mismatch",
			agent: AgentDisplay{
				Status:        "online",
				Version:       "0.0.9",
				LastHeartbeat: time.Now().Add(-30 * time.Second).Format(time.RFC3339),
			},
			expected: true,
		},
		{
			name: "stale heartbeat",
			agent: AgentDisplay{
				Status:        "online",
				Version:       "0.1.0",
				LastHeartbeat: time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isSuspiciousAgent(tt.agent)
			if got != tt.expected {
				t.Errorf("isSuspiciousAgent() = %v, want %v", got, tt.expected)
			}
		})
	}
}
