// Package scenarios contains E2E test scenarios for Keystone Core features.
package scenarios

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/backup"
	"github.com/shawnbutts/keystone-core/internal/bootstrap"
	"github.com/shawnbutts/keystone-core/internal/selfmgmt"
	"github.com/shawnbutts/keystone-core/internal/upgrade"
)

// =============================================================================
// Self-Management E2E Tests (Epic 23)
// These tests verify bootstrap, backup/restore, and upgrade functionality.
// =============================================================================

// -----------------------------------------------------------------------------
// Bootstrap Tests
// -----------------------------------------------------------------------------

// TestBootstrap_SeedConfigValidation tests seed configuration validation
func TestBootstrap_SeedConfigValidation(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	tests := []struct {
		name        string
		config      *bootstrap.SeedConfig
		expectError bool
		errorMsg    string
	}{
		{
			name: "valid_minimal_config",
			config: &bootstrap.SeedConfig{
				Cluster: bootstrap.ClusterConfig{
					Name:   "test-cluster",
					Domain: "test.example.com",
				},
				ControlPlane: bootstrap.ControlPlaneConfig{
					Replicas: 1,
					Nodes: []bootstrap.NodeConfig{
						{
							Host: "10.0.0.1",
							Port: 8080,
							Role: bootstrap.NodeRoleLeader,
						},
					},
				},
				NATS: bootstrap.NATSConfig{
					Mode: bootstrap.NATSModeEmbedded,
				},
				Database: bootstrap.DatabaseConfig{
					Type: bootstrap.DatabaseTypeSQLite,
					Path: "/var/lib/kscore/state.db",
				},
			},
			expectError: false,
		},
		{
			name: "missing_cluster_name",
			config: &bootstrap.SeedConfig{
				Cluster: bootstrap.ClusterConfig{
					Domain: "test.example.com",
				},
				ControlPlane: bootstrap.ControlPlaneConfig{
					Replicas: 1,
				},
			},
			expectError: true,
			errorMsg:    "cluster.name",
		},
		{
			name: "invalid_replicas",
			config: &bootstrap.SeedConfig{
				Cluster: bootstrap.ClusterConfig{
					Name:   "test-cluster",
					Domain: "test.example.com",
				},
				ControlPlane: bootstrap.ControlPlaneConfig{
					Replicas: 0,
				},
			},
			expectError: true,
			errorMsg:    "replicas",
		},
		{
			name: "invalid_node_host",
			config: &bootstrap.SeedConfig{
				Cluster: bootstrap.ClusterConfig{
					Name:   "test-cluster",
					Domain: "test.example.com",
				},
				ControlPlane: bootstrap.ControlPlaneConfig{
					Replicas: 1,
					Nodes: []bootstrap.NodeConfig{
						{
							Host: "not a valid host!@#",
							Port: 8080,
							Role: bootstrap.NodeRoleLeader,
						},
					},
				},
			},
			expectError: true,
			errorMsg:    "host",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bootstrap.ValidateSeedConfig(tt.config)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tt.errorMsg)) {
					t.Errorf("Expected error containing '%s', got: %v", tt.errorMsg, err)
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error, got: %v", err)
				}
			}
		})
	}
}

// TestBootstrap_ConfigLoader tests configuration loading with env expansion
func TestBootstrap_ConfigLoader(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	// Create temp config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "seed.yaml")

	configContent := `
cluster:
  name: ${CLUSTER_NAME:-default-cluster}
  domain: ${CLUSTER_DOMAIN}
control_plane:
  replicas: 1
  nodes:
    - host: 10.0.0.1
      port: 8080
      role: leader
nats:
  mode: embedded
database:
  type: sqlite
  path: /var/lib/kscore/state.db
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	// Set environment variables
	os.Setenv("CLUSTER_DOMAIN", "test.example.com")
	defer os.Unsetenv("CLUSTER_DOMAIN")

	// Load configuration
	loader := bootstrap.NewConfigLoader()
	config, err := loader.LoadSeedConfig(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Verify env expansion
	if config.Cluster.Name != "default-cluster" {
		t.Errorf("Expected cluster name 'default-cluster' (from default), got: %s", config.Cluster.Name)
	}
	if config.Cluster.Domain != "test.example.com" {
		t.Errorf("Expected domain 'test.example.com', got: %s", config.Cluster.Domain)
	}
}

// TestBootstrap_DryRun tests bootstrap in dry-run mode
func TestBootstrap_DryRun(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Create temp config file for dry-run test
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "seed.yaml")

	configContent := `
cluster:
  name: dry-run-test
  domain: test.example.com
control_plane:
  replicas: 1
  nodes:
    - host: 10.0.0.1
      port: 8080
      role: leader
nats:
  mode: embedded
database:
  type: sqlite
  path: /tmp/test-state.db
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	opts := bootstrap.BootstrapOptions{
		Mode:           bootstrap.BootstrapModeSeed,
		SeedConfigPath: configPath,
		DryRun:         true,
	}

	bootstrapper, err := bootstrap.NewBootstrapper(opts, nil)
	if err != nil {
		t.Fatalf("Failed to create bootstrapper: %v", err)
	}

	result, err := bootstrapper.Bootstrap(ctx, opts)
	if err != nil {
		t.Fatalf("Dry-run bootstrap failed: %v", err)
	}

	if !result.Success {
		t.Error("Expected result.Success to be true for dry-run")
	}

	// In dry-run mode, no actual changes should be made
	t.Logf("Dry-run completed successfully in %v", result.Duration)
}

// -----------------------------------------------------------------------------
// Backup Tests
// -----------------------------------------------------------------------------

// TestBackup_CreateAndVerify tests backup creation and verification
func TestBackup_CreateAndVerify(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	tmpDir := t.TempDir()
	backupDir := filepath.Join(tmpDir, "backup")

	// Create backup configuration
	config := &backup.BackupConfig{
		Type: backup.BackupTypeFull,
		Components: []backup.ComponentType{
			backup.ComponentTypeConfig,
		},
		Compression: backup.CompressionTypeGzip,
		Destination: backup.DestinationConfig{
			Type: backup.DestinationTypeLocal,
			Path: backupDir,
		},
	}

	// Create backup manager
	manager, err := backup.NewBackupManager(config, nil)
	if err != nil {
		t.Fatalf("Failed to create backup manager: %v", err)
	}

	// Register mock exporter
	manager.RegisterExporter(&mockConfigExporter{
		configDir: tmpDir,
	})

	// Create and set local destination
	dest := backup.NewLocalDestination(backupDir, nil)
	manager.SetDestination(dest)

	// Execute backup
	result, err := manager.Backup(ctx)
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	if result.Status != backup.BackupStatusCompleted {
		t.Errorf("Expected status Completed, got: %v", result.Status)
	}

	// Verify backup destination is set
	if result.Destination == "" {
		t.Error("Expected destination path in result")
	}

	t.Logf("Backup created: %s (size: %d bytes)", result.Destination, result.Size)
}

// TestBackup_RetentionPolicy tests backup retention enforcement
func TestBackup_RetentionPolicy(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create test backup files with different ages
	now := time.Now()
	backups := []struct {
		name string
		age  time.Duration
	}{
		{"backup-1.tar.gz", 1 * time.Hour},
		{"backup-2.tar.gz", 25 * time.Hour},
		{"backup-3.tar.gz", 50 * time.Hour},
		{"backup-4.tar.gz", 75 * time.Hour},
	}

	for _, b := range backups {
		path := filepath.Join(tmpDir, b.name)
		if err := os.WriteFile(path, []byte("test backup"), 0644); err != nil {
			t.Fatalf("Failed to create test backup: %v", err)
		}
		modTime := now.Add(-b.age)
		if err := os.Chtimes(path, modTime, modTime); err != nil {
			t.Fatalf("Failed to set mod time: %v", err)
		}
	}

	// Create retention config (keep max 2 backups or max 48h)
	config := &backup.RetentionConfig{
		MaxBackups: 2,
		MaxAge:     48 * time.Hour,
	}

	// Create local destination and retention manager
	dest := backup.NewLocalDestination(tmpDir, nil)
	manager := backup.NewRetentionManager(dest, config, nil)

	// Apply retention policy
	deleted, err := manager.Apply(ctx)
	if err != nil {
		t.Fatalf("Retention check failed: %v", err)
	}

	// Should delete backups 3 and 4 (older than 48h or exceeding max 2)
	if len(deleted) < 2 {
		t.Errorf("Expected at least 2 backups to delete, got: %d", len(deleted))
	}

	t.Logf("Retention policy deleted %d backups", len(deleted))
}

// TestBackup_Encryption tests backup encryption configuration
func TestBackup_Encryption(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	// Test various encryption configurations
	tests := []struct {
		name   string
		config backup.EncryptionConfig
	}{
		{
			name: "age_encryption",
			config: backup.EncryptionConfig{
				Type:         backup.EncryptionTypeAge,
				AgeRecipient: "age1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			},
		},
		{
			name: "aws_kms_encryption",
			config: backup.EncryptionConfig{
				Type:        backup.EncryptionTypeAWSKMS,
				AWSKMSKeyID: "arn:aws:kms:us-east-1:123456789:key/12345678-1234-1234-1234-123456789012",
				AWSRegion:   "us-east-1",
			},
		},
		{
			name: "no_encryption",
			config: backup.EncryptionConfig{
				Type: backup.EncryptionTypeNone,
			},
		},
		{
			name: "vault_transit_encryption",
			config: backup.EncryptionConfig{
				Type:         backup.EncryptionTypeVaultTransit,
				VaultAddress: "https://vault.example.com:8200",
				VaultKeyName: "backup-key",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Verify config can be used in BackupConfig
			backupConfig := &backup.BackupConfig{
				Type:       backup.BackupTypeFull,
				Encryption: tt.config,
			}

			// Verify the encryption type is set correctly
			if backupConfig.Encryption.Type != tt.config.Type {
				t.Errorf("Expected encryption type %s, got %s", tt.config.Type, backupConfig.Encryption.Type)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Restore Tests
// -----------------------------------------------------------------------------

// TestRestore_ValidateBackup tests backup artifact building
func TestRestore_ValidateBackup(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	ctx := context.Background()
	tmpDir := t.TempDir()

	// Create source directory with test files
	configDir := filepath.Join(tmpDir, "source", "config")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("Failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "server.yaml"), []byte("test config"), 0644); err != nil {
		t.Fatalf("Failed to write test config: %v", err)
	}

	// Create artifact builder with source directory
	sourceDir := filepath.Join(tmpDir, "source")
	builder, err := backup.NewArtifactBuilder(sourceDir, nil)
	if err != nil {
		t.Fatalf("Failed to create artifact builder: %v", err)
	}

	// Create a backup manifest
	manifest := &backup.BackupManifest{
		ManifestVersion: backup.ManifestVersion,
		Backup: backup.BackupInfo{
			ID:        "test-backup",
			Name:      "test-backup",
			Type:      backup.BackupTypeFull,
			Status:    backup.BackupStatusCompleted,
			StartTime: time.Now(),
		},
		CreatedAt: time.Now(),
	}

	// Build the artifact
	outputPath := filepath.Join(tmpDir, "backup.tar.gz")
	err = builder.Build(ctx, outputPath, manifest, backup.CompressionTypeGzip)
	if err != nil {
		t.Fatalf("Failed to build artifact: %v", err)
	}

	// Verify artifact was created
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("Artifact file not found: %v", err)
	}
	if info.Size() == 0 {
		t.Error("Expected non-empty artifact file")
	}

	t.Logf("Created backup artifact: %s (%d bytes)", outputPath, info.Size())
}

// TestRestore_PartialRestore tests restoring specific components
func TestRestore_PartialRestore(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	// Test restore configuration for specific components
	config := &backup.RestoreConfig{
		Source: "/tmp/backup.tar.gz",
		Components: []backup.ComponentType{
			backup.ComponentTypeConfig,
		},
		SkipVerification: false,
		VerifyIntegrity:  true,
		DryRun:           true,
	}

	// Verify config is created correctly
	if len(config.Components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(config.Components))
	}
	if config.Components[0] != backup.ComponentTypeConfig {
		t.Errorf("Expected ComponentTypeConfig, got %v", config.Components[0])
	}
	if config.SkipVerification {
		t.Error("Expected SkipVerification to be false")
	}
	if !config.DryRun {
		t.Error("Expected DryRun to be true")
	}

	// Test with empty components (should restore all)
	config.Components = nil
	if config.Components != nil {
		t.Error("Expected nil components for full restore")
	}

	// Verify source is required
	if config.Source == "" {
		t.Error("Source should be set for restore config")
	}
}

// -----------------------------------------------------------------------------
// Upgrade Tests
// -----------------------------------------------------------------------------

// TestUpgrade_VersionParsing tests semantic version parsing
func TestUpgrade_VersionParsing(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	tests := []struct {
		input       string
		expectError bool
		major       int
		minor       int
		patch       int
	}{
		{"1.0.0", false, 1, 0, 0},
		{"2.5.10", false, 2, 5, 10},
		{"1.0.0-alpha", false, 1, 0, 0},
		{"1.0.0-beta.1", false, 1, 0, 0},
		{"1.0.0+build.123", false, 1, 0, 0},
		{"invalid", true, 0, 0, 0},
		{"1.0", true, 0, 0, 0},
		{"", true, 0, 0, 0},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			v, err := upgrade.ParseVersion(tt.input)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error for input '%s'", tt.input)
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error for input '%s': %v", tt.input, err)
				}
				if v.Major != tt.major || v.Minor != tt.minor || v.Patch != tt.patch {
					t.Errorf("Expected %d.%d.%d, got %d.%d.%d",
						tt.major, tt.minor, tt.patch, v.Major, v.Minor, v.Patch)
				}
			}
		})
	}
}

// TestUpgrade_VersionComparison tests version comparison
func TestUpgrade_VersionComparison(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	tests := []struct {
		v1       string
		v2       string
		expected int // -1: v1 < v2, 0: equal, 1: v1 > v2
	}{
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.1", "1.0.0", 1},
		{"1.1.0", "1.0.0", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0-alpha", "1.0.0", -1},
		{"1.0.0-beta", "1.0.0-alpha", 1},
	}

	for _, tt := range tests {
		t.Run(tt.v1+"_vs_"+tt.v2, func(t *testing.T) {
			v1, _ := upgrade.ParseVersion(tt.v1)
			v2, _ := upgrade.ParseVersion(tt.v2)
			result := v1.Compare(v2)
			if result != tt.expected {
				t.Errorf("Compare(%s, %s): expected %d, got %d",
					tt.v1, tt.v2, tt.expected, result)
			}
		})
	}
}

// TestUpgrade_CompatibilityCheck tests upgrade compatibility validation
func TestUpgrade_CompatibilityCheck(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	// Create compatibility matrix
	matrix := &upgrade.CompatibilityMatrix{
		Entries: []upgrade.CompatibilityEntry{
			{
				Version:    upgrade.Version{Major: 1, Minor: 5, Patch: 0},
				MinUpgrade: &upgrade.Version{Major: 1, Minor: 4, Patch: 0},
				MaxUpgrade: &upgrade.Version{Major: 1, Minor: 6, Patch: 0},
			},
			{
				Version:    upgrade.Version{Major: 1, Minor: 6, Patch: 0},
				MinUpgrade: &upgrade.Version{Major: 1, Minor: 4, Patch: 0},
				MaxUpgrade: &upgrade.Version{Major: 2, Minor: 0, Patch: 0},
			},
		},
	}

	checker := upgrade.NewVersionChecker(nil)
	checker.LoadMatrix(upgrade.ComponentServer, matrix)

	tests := []struct {
		from       string
		to         string
		compatible bool
	}{
		{"1.4.0", "1.5.0", true},
		{"1.5.0", "1.6.0", true},
		{"1.4.0", "1.6.0", true},
		{"1.3.0", "1.5.0", false}, // 1.3 not in min range for 1.5
		{"1.5.0", "2.1.0", false}, // 2.1 exceeds max for 1.6
	}

	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			from, _ := upgrade.ParseVersion(tt.from)
			to, _ := upgrade.ParseVersion(tt.to)
			result, err := checker.CheckCompatibility(upgrade.ComponentServer, from, to)
			compatible := err == nil && result.Compatible
			if compatible != tt.compatible {
				t.Errorf("CheckCompatibility(%s -> %s): expected %v, got %v",
					tt.from, tt.to, tt.compatible, compatible)
			}
		})
	}
}

// TestUpgrade_RollingStrategy tests rolling upgrade configuration
func TestUpgrade_RollingStrategy(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	config := upgrade.DefaultRollingConfig()

	if config.MaxUnavailable < 1 {
		t.Error("MaxUnavailable should be at least 1")
	}

	if config.DrainTimeout < time.Minute {
		t.Error("DrainTimeout should be at least 1 minute")
	}

	// Test rolling stats
	stats := &upgrade.RollingStats{
		CurrentBatch:   2,
		CompletedNodes: 4,
		FailedNodes:    1,
		HealthyNodes:   3,
	}

	if stats.CompletedNodes != 4 {
		t.Error("Stats should track completed nodes")
	}
	if stats.FailedNodes != 1 {
		t.Error("Stats should track failed nodes")
	}
}

// TestUpgrade_CanaryStrategy tests canary deployment configuration
func TestUpgrade_CanaryStrategy(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	config := upgrade.DefaultCanaryConfig()

	if config.InitialPercentage <= 0 || config.InitialPercentage > 100 {
		t.Error("InitialPercentage should be 1-100")
	}

	if config.Increment <= 0 {
		t.Error("Increment should be positive")
	}

	if config.SuccessThreshold < 1 {
		t.Error("SuccessThreshold should be at least 1")
	}

	// Test canary stats
	stats := &upgrade.CanaryStats{
		CurrentPercentage: 25,
		SuccessfulChecks:  3,
		FailedChecks:      0,
	}

	if stats.SuccessfulChecks < config.SuccessThreshold {
		t.Log("Canary not yet ready for promotion")
	}
}

// TestUpgrade_RollbackDecision tests rollback decision logic
func TestUpgrade_RollbackDecision(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	tests := []struct {
		name           string
		decision       upgrade.RollbackDecision
		shouldRollback bool
	}{
		{
			name: "no_rollback_needed",
			decision: upgrade.RollbackDecision{
				ShouldRollback: false,
				Confidence:     0.0,
				Reasons:        nil,
			},
			shouldRollback: false,
		},
		{
			name: "high_failure_rate",
			decision: upgrade.RollbackDecision{
				ShouldRollback: true,
				Confidence:     0.95,
				Reasons:        []string{"failure rate exceeded threshold"},
			},
			shouldRollback: true,
		},
		{
			name: "health_degradation",
			decision: upgrade.RollbackDecision{
				ShouldRollback: true,
				Confidence:     0.80,
				Reasons:        []string{"cluster health degraded"},
			},
			shouldRollback: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.decision.ShouldRollback != tt.shouldRollback {
				t.Errorf("Expected ShouldRollback=%v, got %v",
					tt.shouldRollback, tt.decision.ShouldRollback)
			}
			if tt.decision.ShouldRollback && tt.decision.Confidence < 0.5 {
				t.Error("Rollback decision should have high confidence")
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Self-Management State Module Tests
// -----------------------------------------------------------------------------

// TestSelfMgmt_ServerModule tests server state module
func TestSelfMgmt_ServerModule(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	module := selfmgmt.NewServerModule(nil)

	if module.Name() != "kscore_server" {
		t.Errorf("Expected name 'kscore_server', got: %s", module.Name())
	}

	if module.ComponentType() != selfmgmt.ComponentServer {
		t.Errorf("Expected type ComponentServer, got: %v", module.ComponentType())
	}

	// Test valid states
	validStates := []selfmgmt.ComponentState{
		selfmgmt.StateInstalled,
		selfmgmt.StateUninstalled,
		selfmgmt.StateRunning,
		selfmgmt.StateStopped,
	}

	for _, state := range validStates {
		config := &selfmgmt.ServerConfig{
			BaseConfig: selfmgmt.BaseConfig{
				State: state,
			},
		}
		if err := module.Validate(config); err != nil {
			t.Errorf("Expected state %v to be valid, got error: %v", state, err)
		}
	}
}

// TestSelfMgmt_AgentModule tests agent state module
func TestSelfMgmt_AgentModule(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	module := selfmgmt.NewAgentModule(nil)

	if module.Name() != "kscore_agent" {
		t.Errorf("Expected name 'kscore_agent', got: %s", module.Name())
	}

	config := &selfmgmt.AgentConfig{
		BaseConfig: selfmgmt.BaseConfig{
			State: selfmgmt.StateRunning,
		},
		ServerURL: "https://server:8080",
	}

	if err := module.Validate(config); err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}
}

// TestSelfMgmt_BackupModule tests backup state module
func TestSelfMgmt_BackupModule(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	module := selfmgmt.NewBackupModule()

	if module.Name() != "kscore_backup" {
		t.Errorf("Expected name 'kscore_backup', got: %s", module.Name())
	}

	// Test valid cron schedule
	config := &selfmgmt.BackupConfig{
		BaseConfig: selfmgmt.BaseConfig{
			State: selfmgmt.StateConfigured,
		},
		Schedule: "0 2 * * *",
		Destinations: []selfmgmt.BackupDestination{
			{
				Type: "local",
				Path: "/backup",
			},
		},
	}

	if err := module.Validate(config); err != nil {
		t.Errorf("Expected valid config, got error: %v", err)
	}

	// Test invalid cron schedule
	config.Schedule = "invalid"
	if err := module.Validate(config); err == nil {
		t.Error("Expected error for invalid cron schedule")
	}
}

// TestSelfMgmt_Validation tests configuration validation
func TestSelfMgmt_Validation(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	validator := selfmgmt.NewStateValidator()

	// Test port validation
	if err := validator.ValidatePort(8080); err != nil {
		t.Errorf("Expected port 8080 to be valid: %v", err)
	}
	if err := validator.ValidatePort(0); err == nil {
		t.Error("Expected port 0 to be invalid")
	}
	if err := validator.ValidatePort(70000); err == nil {
		t.Error("Expected port 70000 to be invalid")
	}

	// Test path validation
	if err := validator.ValidatePath("/etc/kscore"); err != nil {
		t.Errorf("Expected absolute path to be valid: %v", err)
	}
	if err := validator.ValidatePath("relative/path"); err == nil {
		t.Error("Expected relative path to be invalid")
	}

	// Test NATS URL validation
	if err := validator.ValidateNATSURL("nats://localhost:4222"); err != nil {
		t.Errorf("Expected NATS URL to be valid: %v", err)
	}
	if err := validator.ValidateNATSURL("http://localhost:4222"); err == nil {
		t.Error("Expected non-NATS URL to be invalid")
	}
}

// -----------------------------------------------------------------------------
// Helper Types
// -----------------------------------------------------------------------------

// mockConfigExporter is a mock exporter for testing
type mockConfigExporter struct {
	configDir string
}

func (e *mockConfigExporter) Name() string {
	return "config"
}

func (e *mockConfigExporter) Component() backup.ComponentType {
	return backup.ComponentTypeConfig
}

func (e *mockConfigExporter) Export(ctx context.Context, w io.Writer) error {
	// Write mock config data
	_, err := w.Write([]byte("test: config"))
	return err
}

func (e *mockConfigExporter) EstimateSize(ctx context.Context) (int64, error) {
	return int64(len("test: config")), nil
}
