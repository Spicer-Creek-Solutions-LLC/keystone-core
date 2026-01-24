package main

import (
	"bytes"
	"testing"
)

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd()

	if cmd == nil {
		t.Fatal("newRootCmd should not return nil")
	}

	if cmd.Use != "kscore-backup" {
		t.Errorf("Use = %v, want kscore-backup", cmd.Use)
	}
}

func TestRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()

	expectedSubcommands := []string{
		"create",
		"list",
		"show",
		"verify",
		"restore",
		"delete",
		"replication-status",
		"schedule",
		"retention",
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

func TestNewCreateCmd(t *testing.T) {
	cfg := &Config{
		ServerAddr:   "localhost:9090",
		OutputFormat: "table",
	}
	cmd := newCreateCmd(cfg)

	if cmd == nil {
		t.Fatal("newCreateCmd should not return nil")
	}
	if cmd.Use != "create" {
		t.Errorf("Use = %v, want create", cmd.Use)
	}

	// Check important flags exist
	flags := []string{"type", "components", "destination", "encrypt", "compress", "compression", "async"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewListCmd(t *testing.T) {
	cfg := &Config{
		ServerAddr:   "localhost:9090",
		OutputFormat: "table",
	}
	cmd := newListCmd(cfg)

	if cmd == nil {
		t.Fatal("newListCmd should not return nil")
	}
	if cmd.Use != "list" {
		t.Errorf("Use = %v, want list", cmd.Use)
	}

	// Check flags exist
	flags := []string{"last", "type", "status", "limit"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewShowCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newShowCmd(cfg)

	if cmd == nil {
		t.Fatal("newShowCmd should not return nil")
	}
	if cmd.Use != "show <backup-id>" {
		t.Errorf("Use = %v, want 'show <backup-id>'", cmd.Use)
	}
}

func TestNewVerifyCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newVerifyCmd(cfg)

	if cmd == nil {
		t.Fatal("newVerifyCmd should not return nil")
	}
	if cmd.Use != "verify <backup-id>" {
		t.Errorf("Use = %v, want 'verify <backup-id>'", cmd.Use)
	}

	// Check flags exist
	flags := []string{"check-integrity", "check-restorable", "verbose"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewRestoreCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newRestoreCmd(cfg)

	if cmd == nil {
		t.Fatal("newRestoreCmd should not return nil")
	}
	if cmd.Use != "restore <backup-id>" {
		t.Errorf("Use = %v, want 'restore <backup-id>'", cmd.Use)
	}

	// Check flags exist
	flags := []string{"target", "components", "dry-run", "force", "async"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewDeleteCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newDeleteCmd(cfg)

	if cmd == nil {
		t.Fatal("newDeleteCmd should not return nil")
	}
	if cmd.Use != "delete <backup-id>" {
		t.Errorf("Use = %v, want 'delete <backup-id>'", cmd.Use)
	}

	// Check flags exist
	flags := []string{"force", "older-than"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil {
			t.Errorf("expected flag %q not found", flag)
		}
	}
}

func TestNewScheduleCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newScheduleCmd(cfg)

	if cmd == nil {
		t.Fatal("newScheduleCmd should not return nil")
	}
	if cmd.Use != "schedule" {
		t.Errorf("Use = %v, want schedule", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"list", "create", "delete", "enable", "disable"}
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

func TestNewRetentionCmd(t *testing.T) {
	cfg := &Config{}
	cmd := newRetentionCmd(cfg)

	if cmd == nil {
		t.Fatal("newRetentionCmd should not return nil")
	}
	if cmd.Use != "retention" {
		t.Errorf("Use = %v, want retention", cmd.Use)
	}

	// Should have subcommands
	subcommands := []string{"show", "set", "apply"}
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
		ServerAddr:   "localhost:9090",
		OutputFormat: "json",
		Verbose:      true,
	}

	if cfg.ServerAddr != "localhost:9090" {
		t.Errorf("ServerAddr = %v, want localhost:9090", cfg.ServerAddr)
	}
	if cfg.OutputFormat != "json" {
		t.Errorf("OutputFormat = %v, want json", cfg.OutputFormat)
	}
	if !cfg.Verbose {
		t.Error("Verbose should be true")
	}
}

func TestBackupInfoStructure(t *testing.T) {
	backup := BackupInfo{
		ID:          "backup-123",
		Type:        "full",
		Status:      "completed",
		Size:        "512 MB",
		SizeBytes:   536870912,
		Components:  []string{"database", "config"},
		Destination: "s3://bucket/path",
		Encrypted:   true,
		Compressed:  true,
		Checksum:    "sha256:abc123",
		CreatedAt:   "2024-01-15T06:00:00Z",
		CompletedAt: "2024-01-15T06:05:00Z",
		Duration:    "5m",
		Labels:      map[string]string{"env": "prod"},
	}

	if backup.ID != "backup-123" {
		t.Errorf("ID = %v, want backup-123", backup.ID)
	}
	if backup.Type != "full" {
		t.Errorf("Type = %v, want full", backup.Type)
	}
	if !backup.Encrypted {
		t.Error("Encrypted should be true")
	}
	if len(backup.Components) != 2 {
		t.Errorf("Components count = %d, want 2", len(backup.Components))
	}
}

func TestVerificationResultStructure(t *testing.T) {
	result := VerificationResult{
		BackupID:       "backup-123",
		Valid:          true,
		ChecksumMatch:  true,
		ComponentsOK:   map[string]bool{"database": true, "config": true},
		IntegrityOK:    true,
		Restorable:     true,
		Issues:         []string{},
		VerifiedAt:     "2024-01-15T07:00:00Z",
		VerificationID: "verify-123",
	}

	if !result.Valid {
		t.Error("Valid should be true")
	}
	if !result.ChecksumMatch {
		t.Error("ChecksumMatch should be true")
	}
	if len(result.Issues) != 0 {
		t.Errorf("Issues count = %d, want 0", len(result.Issues))
	}
}

func TestReplicationStatusStructure(t *testing.T) {
	status := ReplicationStatus{
		Enabled:      true,
		LastSync:     "2024-01-15T06:00:00Z",
		NextSync:     "2024-01-15T18:00:00Z",
		SyncInterval: "12h",
		Status:       "healthy",
		Destinations: []ReplicationDest{
			{
				Name:        "us-west-2",
				Type:        "s3",
				Status:      "synced",
				LastSync:    "2024-01-15T06:00:00Z",
				BackupCount: 30,
				TotalSize:   "15.2 GB",
			},
		},
	}

	if !status.Enabled {
		t.Error("Enabled should be true")
	}
	if status.Status != "healthy" {
		t.Errorf("Status = %v, want healthy", status.Status)
	}
	if len(status.Destinations) != 1 {
		t.Errorf("Destinations count = %d, want 1", len(status.Destinations))
	}
}

func TestRestoreResultStructure(t *testing.T) {
	result := RestoreResult{
		RestoreID:   "restore-123",
		BackupID:    "backup-123",
		Status:      "completed",
		Target:      "local",
		Components:  []string{"database", "config"},
		StartedAt:   "2024-01-15T08:00:00Z",
		CompletedAt: "2024-01-15T08:05:00Z",
		Duration:    "5m",
		DryRun:      false,
	}

	if result.RestoreID != "restore-123" {
		t.Errorf("RestoreID = %v, want restore-123", result.RestoreID)
	}
	if result.Status != "completed" {
		t.Errorf("Status = %v, want completed", result.Status)
	}
	if result.DryRun {
		t.Error("DryRun should be false")
	}
}

func TestScheduleInfoStructure(t *testing.T) {
	schedule := ScheduleInfo{
		Name:        "daily-full",
		Schedule:    "0 6 * * *",
		Type:        "full",
		Components:  []string{"database", "config"},
		Destination: "s3://bucket/backups",
		Enabled:     true,
		LastRun:     "2024-01-15T06:00:00Z",
		NextRun:     "2024-01-16T06:00:00Z",
		RetainCount: 7,
	}

	if schedule.Name != "daily-full" {
		t.Errorf("Name = %v, want daily-full", schedule.Name)
	}
	if schedule.Schedule != "0 6 * * *" {
		t.Errorf("Schedule = %v, want '0 6 * * *'", schedule.Schedule)
	}
	if schedule.RetainCount != 7 {
		t.Errorf("RetainCount = %d, want 7", schedule.RetainCount)
	}
}

func TestRetentionPolicyStructure(t *testing.T) {
	policy := RetentionPolicy{
		Name:        "default",
		MaxBackups:  30,
		MaxAge:      "30d",
		KeepDaily:   7,
		KeepWeekly:  4,
		KeepMonthly: 6,
		KeepYearly:  2,
		AppliesTo:   "all",
	}

	if policy.Name != "default" {
		t.Errorf("Name = %v, want default", policy.Name)
	}
	if policy.MaxBackups != 30 {
		t.Errorf("MaxBackups = %d, want 30", policy.MaxBackups)
	}
	if policy.KeepWeekly != 4 {
		t.Errorf("KeepWeekly = %d, want 4", policy.KeepWeekly)
	}
}

func TestBoolToStatus(t *testing.T) {
	tests := []struct {
		input    bool
		expected string
	}{
		{true, "✓ yes"},
		{false, "✗ no"},
	}

	for _, tt := range tests {
		result := boolToStatus(tt.input)
		if result != tt.expected {
			t.Errorf("boolToStatus(%v) = %v, want %v", tt.input, result, tt.expected)
		}
	}
}
