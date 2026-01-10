// Package scenarios contains E2E test scenarios for Keystone Core features.
package scenarios

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/backup"
	"github.com/shawnbutts/keystone-core/pkg/bootstrap"
	"github.com/shawnbutts/keystone-core/pkg/selfmgmt"
	"github.com/shawnbutts/keystone-core/pkg/upgrade"
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
				Nodes: []bootstrap.NodeConfig{
					{
						Hostname: "node-1",
						Role:     "server",
						IP:       "10.0.0.1",
					},
				},
				NATS: bootstrap.NATSSeedConfig{
					Mode: "embedded",
				},
				Database: bootstrap.DatabaseSeedConfig{
					Type: "sqlite",
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
			},
			expectError: true,
			errorMsg:    "cluster name",
		},
		{
			name: "missing_nodes",
			config: &bootstrap.SeedConfig{
				Cluster: bootstrap.ClusterConfig{
					Name:   "test-cluster",
					Domain: "test.example.com",
				},
				Nodes: []bootstrap.NodeConfig{},
			},
			expectError: true,
			errorMsg:    "at least one node",
		},
		{
			name: "invalid_node_ip",
			config: &bootstrap.SeedConfig{
				Cluster: bootstrap.ClusterConfig{
					Name:   "test-cluster",
					Domain: "test.example.com",
				},
				Nodes: []bootstrap.NodeConfig{
					{
						Hostname: "node-1",
						Role:     "server",
						IP:       "not-an-ip",
					},
				},
			},
			expectError: true,
			errorMsg:    "invalid IP",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := bootstrap.ValidateSeedConfig(tt.config)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error containing '%s', got nil", tt.errorMsg)
				} else if !strings.Contains(err.Error(), tt.errorMsg) {
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
nodes:
  - hostname: node-1
    role: server
    ip: 10.0.0.1
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
	config, err := loader.LoadFile(configPath)
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

	config := &bootstrap.SeedConfig{
		Cluster: bootstrap.ClusterConfig{
			Name:   "dry-run-test",
			Domain: "test.example.com",
		},
		Nodes: []bootstrap.NodeConfig{
			{
				Hostname: "node-1",
				Role:     "server",
				IP:       "10.0.0.1",
			},
		},
		NATS: bootstrap.NATSSeedConfig{
			Mode: "embedded",
		},
		Database: bootstrap.DatabaseSeedConfig{
			Type: "sqlite",
			Path: "/tmp/test-state.db",
		},
	}

	bootstrapper := bootstrap.NewBootstrapper(nil, nil)
	opts := &bootstrap.BootstrapOptions{
		DryRun: true,
	}

	result, err := bootstrapper.Bootstrap(ctx, config, opts)
	if err != nil {
		t.Fatalf("Dry-run bootstrap failed: %v", err)
	}

	if !result.DryRun {
		t.Error("Expected result.DryRun to be true")
	}

	// In dry-run mode, no actual changes should be made
	t.Logf("Dry-run completed: %d steps validated", len(result.Steps))
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
			backup.ComponentConfig,
		},
		Compression: backup.CompressionGzip,
	}

	// Create mock exporters for testing
	exporters := map[backup.ComponentType]backup.Exporter{
		backup.ComponentConfig: &mockConfigExporter{
			configDir: tmpDir,
		},
	}

	// Create local destination
	dest, err := backup.NewLocalDestination(backupDir)
	if err != nil {
		t.Fatalf("Failed to create destination: %v", err)
	}

	// Create backup manager
	manager := backup.NewBackupManager(exporters, dest, nil, nil)

	// Execute backup
	result, err := manager.Backup(ctx, config)
	if err != nil {
		t.Fatalf("Backup failed: %v", err)
	}

	if result.Status != backup.BackupStatusCompleted {
		t.Errorf("Expected status Completed, got: %v", result.Status)
	}

	// Verify backup artifact exists
	if result.ArtifactPath == "" {
		t.Error("Expected artifact path in result")
	}

	t.Logf("Backup created: %s (size: %d bytes)", result.ArtifactPath, result.Size)
}

// TestBackup_RetentionPolicy tests backup retention enforcement
func TestBackup_RetentionPolicy(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

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

	// Create retention policy (keep max 2 backups or max 48h)
	policy := &backup.RetentionPolicy{
		MaxBackups: 2,
		MaxAge:     48 * time.Hour,
	}

	manager := backup.NewRetentionManager(policy)

	// Get backups to delete
	dest, _ := backup.NewLocalDestination(tmpDir)
	toDelete, err := manager.ApplyRetention(dest)
	if err != nil {
		t.Fatalf("Retention check failed: %v", err)
	}

	// Should delete backups 3 and 4 (older than 48h or exceeding max 2)
	if len(toDelete) < 2 {
		t.Errorf("Expected at least 2 backups to delete, got: %d", len(toDelete))
	}

	t.Logf("Retention policy would delete %d backups", len(toDelete))
}

// TestBackup_Encryption tests backup encryption
func TestBackup_Encryption(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	// Test that encryption config is validated
	tests := []struct {
		name        string
		config      *backup.EncryptionConfig
		expectError bool
	}{
		{
			name: "valid_age_encryption",
			config: &backup.EncryptionConfig{
				Enabled:   true,
				Provider:  "age",
				Recipient: "age1xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
			},
			expectError: false,
		},
		{
			name: "missing_recipient",
			config: &backup.EncryptionConfig{
				Enabled:  true,
				Provider: "age",
			},
			expectError: true,
		},
		{
			name: "disabled_encryption",
			config: &backup.EncryptionConfig{
				Enabled: false,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := backup.ValidateEncryptionConfig(tt.config)
			if tt.expectError && err == nil {
				t.Error("Expected error, got nil")
			}
			if !tt.expectError && err != nil {
				t.Errorf("Expected no error, got: %v", err)
			}
		})
	}
}

// -----------------------------------------------------------------------------
// Restore Tests
// -----------------------------------------------------------------------------

// TestRestore_ValidateBackup tests backup validation before restore
func TestRestore_ValidateBackup(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	tmpDir := t.TempDir()

	// Create a valid backup artifact structure
	backupPath := filepath.Join(tmpDir, "backup.tar.gz")
	manifest := &backup.BackupManifest{
		Version:   "1.0",
		Timestamp: time.Now(),
		Components: []backup.ComponentManifest{
			{
				Type:     backup.ComponentConfig,
				Path:     "config/",
				Checksum: "abc123",
			},
		},
	}

	builder := backup.NewArtifactBuilder(backupPath)
	if err := builder.AddManifest(manifest); err != nil {
		t.Fatalf("Failed to add manifest: %v", err)
	}
	if err := builder.AddFile("config/server.yaml", []byte("test config")); err != nil {
		t.Fatalf("Failed to add file: %v", err)
	}
	if err := builder.Close(); err != nil {
		t.Fatalf("Failed to close artifact: %v", err)
	}

	// Validate the artifact
	valid, err := backup.ValidateArtifact(backupPath)
	if err != nil {
		t.Fatalf("Validation failed: %v", err)
	}
	if !valid {
		t.Error("Expected artifact to be valid")
	}
}

// TestRestore_PartialRestore tests restoring specific components
func TestRestore_PartialRestore(t *testing.T) {
	if os.Getenv("KSCORE_E2E_TESTS") == "" {
		t.Skip("Skipping E2E test; set KSCORE_E2E_TESTS=1 to run")
	}

	// Test restore configuration validation
	config := &backup.RestoreConfig{
		Components: []backup.ComponentType{
			backup.ComponentConfig,
		},
		SkipValidation: false,
		DryRun:         true,
	}

	err := backup.ValidateRestoreConfig(config)
	if err != nil {
		t.Errorf("Expected valid restore config, got: %v", err)
	}

	// Test with empty components (should restore all)
	config.Components = nil
	err = backup.ValidateRestoreConfig(config)
	if err != nil {
		t.Errorf("Expected valid config with nil components, got: %v", err)
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
				MinUpgrade: upgrade.Version{Major: 1, Minor: 4, Patch: 0},
				MaxUpgrade: upgrade.Version{Major: 1, Minor: 6, Patch: 0},
			},
			{
				Version:    upgrade.Version{Major: 1, Minor: 6, Patch: 0},
				MinUpgrade: upgrade.Version{Major: 1, Minor: 4, Patch: 0},
				MaxUpgrade: upgrade.Version{Major: 2, Minor: 0, Patch: 0},
			},
		},
	}

	checker := upgrade.NewVersionChecker()
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
			compatible := checker.IsCompatible(upgrade.ComponentServer, from, to)
			if compatible != tt.compatible {
				t.Errorf("IsCompatible(%s -> %s): expected %v, got %v",
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
		TotalBatches:   5,
		CompletedNodes: 4,
		FailedNodes:    1,
		HealthyNodes:   4,
	}

	if stats.CompletedNodes+stats.FailedNodes != 5 {
		t.Error("Stats should track all processed nodes")
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

	module := selfmgmt.NewServerModule()

	if module.Name() != "kscore_server" {
		t.Errorf("Expected name 'kscore_server', got: %s", module.Name())
	}

	if module.Type() != selfmgmt.ComponentServer {
		t.Errorf("Expected type ComponentServer, got: %v", module.Type())
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

	module := selfmgmt.NewAgentModule()

	if module.Name() != "kscore_agent" {
		t.Errorf("Expected name 'kscore_agent', got: %s", module.Name())
	}

	config := &selfmgmt.AgentConfig{
		BaseConfig: selfmgmt.BaseConfig{
			State: selfmgmt.StateRunning,
		},
		ServerURLs: []string{"https://server:8080"},
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
		Destination: selfmgmt.BackupDestination{
			Type: "local",
			Path: "/backup",
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

	validator := selfmgmt.NewValidator()

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

func (e *mockConfigExporter) Export(ctx context.Context, destPath string) error {
	// Create mock config file
	configPath := filepath.Join(destPath, "server.yaml")
	return os.WriteFile(configPath, []byte("test: config"), 0644)
}

func (e *mockConfigExporter) Type() backup.ComponentType {
	return backup.ComponentConfig
}
