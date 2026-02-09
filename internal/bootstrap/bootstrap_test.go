package bootstrap

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestConfigLoader tests the configuration loader
func TestConfigLoader(t *testing.T) {
	t.Run("NewConfigLoader", func(t *testing.T) {
		loader := NewConfigLoader()
		if loader == nil {
			t.Fatal("expected non-nil loader")
		}
		if loader.envPrefix != "KSCORE_" {
			t.Errorf("expected envPrefix KSCORE_, got %s", loader.envPrefix)
		}
	})

	t.Run("ParseSeedConfig_Valid", func(t *testing.T) {
		loader := NewConfigLoader()
		yaml := `
cluster:
  name: test-cluster
  domain: test.local
control_plane:
  replicas: 1
  nodes:
    - host: localhost
      port: 8080
nats:
  mode: embedded
database:
  type: sqlite
`
		config, err := loader.ParseSeedConfig([]byte(yaml))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if config.Cluster.Name != "test-cluster" {
			t.Errorf("expected cluster name test-cluster, got %s", config.Cluster.Name)
		}
		if config.NATS.Mode != NATSModeEmbedded {
			t.Errorf("expected NATS mode embedded, got %s", config.NATS.Mode)
		}
	})

	t.Run("ParseSeedConfig_Defaults", func(t *testing.T) {
		loader := NewConfigLoader()
		yaml := `
cluster:
  name: minimal
`
		config, err := loader.ParseSeedConfig([]byte(yaml))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Check defaults are applied
		if config.ControlPlane.Replicas != 1 {
			t.Errorf("expected default replicas 1, got %d", config.ControlPlane.Replicas)
		}
		if config.NATS.Mode != NATSModeEmbedded {
			t.Errorf("expected default NATS mode embedded, got %s", config.NATS.Mode)
		}
		if config.Database.Type != DatabaseTypeSQLite {
			t.Errorf("expected default database type sqlite, got %s", config.Database.Type)
		}
	})

	t.Run("ExpandEnvVars", func(t *testing.T) {
		loader := NewConfigLoader()

		// Set test env var
		os.Setenv("TEST_VAR", "test_value")
		defer os.Unsetenv("TEST_VAR")

		input := "value: ${TEST_VAR}"
		result := loader.expandEnvVars(input)
		if !strings.Contains(result, "test_value") {
			t.Errorf("expected env var to be expanded, got %s", result)
		}
	})

	t.Run("ExpandEnvVars_Default", func(t *testing.T) {
		loader := NewConfigLoader()

		// Ensure var doesn't exist
		os.Unsetenv("NONEXISTENT_VAR")

		input := "value: ${NONEXISTENT_VAR:-default_val}"
		result := loader.expandEnvVars(input)
		if !strings.Contains(result, "default_val") {
			t.Errorf("expected default value to be used, got %s", result)
		}
	})
}

// TestValidateSeedConfig tests configuration validation
func TestValidateSeedConfig(t *testing.T) {
	t.Run("ValidConfig", func(t *testing.T) {
		config := DefaultSeedConfig()
		err := ValidateSeedConfig(config)
		if err != nil {
			t.Errorf("expected valid config, got error: %v", err)
		}
	})

	t.Run("EmptyClusterName", func(t *testing.T) {
		config := DefaultSeedConfig()
		config.Cluster.Name = ""
		err := ValidateSeedConfig(config)
		if err == nil {
			t.Error("expected error for empty cluster name")
		}
	})

	t.Run("InvalidReplicas", func(t *testing.T) {
		config := DefaultSeedConfig()
		config.ControlPlane.Replicas = 0
		err := ValidateSeedConfig(config)
		if err == nil {
			t.Error("expected error for zero replicas")
		}
	})

	t.Run("ReplicasWithoutNodes", func(t *testing.T) {
		config := DefaultSeedConfig()
		config.ControlPlane.Replicas = 3
		config.ControlPlane.Nodes = []NodeConfig{
			{Host: "node1", Port: 8080},
		}
		err := ValidateSeedConfig(config)
		if err == nil {
			t.Error("expected error for insufficient nodes")
		}
	})

	t.Run("InvalidNATSMode", func(t *testing.T) {
		config := DefaultSeedConfig()
		config.NATS.Mode = "invalid"
		err := ValidateSeedConfig(config)
		if err == nil {
			t.Error("expected error for invalid NATS mode")
		}
	})

	t.Run("NATSClusterWithoutNodes", func(t *testing.T) {
		config := DefaultSeedConfig()
		config.NATS.Mode = NATSModeCluster
		config.NATS.Nodes = []string{"node1"}
		err := ValidateSeedConfig(config)
		if err == nil {
			t.Error("expected error for cluster mode with insufficient nodes")
		}
	})

	t.Run("PostgreSQLWithoutHost", func(t *testing.T) {
		config := DefaultSeedConfig()
		config.Database.Type = DatabaseTypePostgreSQL
		config.Database.Name = "kscore"
		config.Database.User = "kscore"
		err := ValidateSeedConfig(config)
		if err == nil {
			t.Error("expected error for PostgreSQL without host")
		}
	})

	t.Run("ExternalEtcdWithoutNodes", func(t *testing.T) {
		config := DefaultSeedConfig()
		config.Etcd.Mode = EtcdModeExternal
		err := ValidateSeedConfig(config)
		if err == nil {
			t.Error("expected error for external etcd without nodes")
		}
	})

	t.Run("InvalidHostname", func(t *testing.T) {
		config := DefaultSeedConfig()
		config.ControlPlane.Nodes[0].Host = "invalid..hostname"
		err := ValidateSeedConfig(config)
		if err == nil {
			t.Error("expected error for invalid hostname")
		}
	})
}

// TestIsValidHost tests hostname validation
func TestIsValidHost(t *testing.T) {
	tests := []struct {
		host  string
		valid bool
	}{
		{"localhost", true},
		{"example.com", true},
		{"sub.example.com", true},
		{"192.168.1.1", true},
		{"10.0.0.1", true},
		{"::1", true},
		{"2001:db8::1", true},
		{"host-name", true},
		{"host_name", false}, // underscore not valid in hostname
		{"", false},
		{"-hostname", false},
		{"hostname-", false},
		{"host..name", false},
	}

	for _, tc := range tests {
		t.Run(tc.host, func(t *testing.T) {
			result := isValidHost(tc.host)
			if result != tc.valid {
				t.Errorf("isValidHost(%q) = %v, want %v", tc.host, result, tc.valid)
			}
		})
	}
}

// TestDefaultSeedConfig tests default configuration generation
func TestDefaultSeedConfig(t *testing.T) {
	config := DefaultSeedConfig()

	if config.Cluster.Name != "default" {
		t.Errorf("expected default cluster name 'default', got %s", config.Cluster.Name)
	}
	if config.ControlPlane.Replicas != 1 {
		t.Errorf("expected 1 replica, got %d", config.ControlPlane.Replicas)
	}
	if config.NATS.Mode != NATSModeEmbedded {
		t.Errorf("expected embedded NATS mode, got %s", config.NATS.Mode)
	}
	if config.Database.Type != DatabaseTypeSQLite {
		t.Errorf("expected sqlite database type, got %s", config.Database.Type)
	}
	if config.Etcd.Mode != EtcdModeEmbedded {
		t.Errorf("expected embedded etcd mode, got %s", config.Etcd.Mode)
	}
}

// TestMergeSeedConfig tests configuration merging
func TestMergeSeedConfig(t *testing.T) {
	base := DefaultSeedConfig()
	override := &SeedConfig{
		Cluster: ClusterConfig{
			Name: "override-cluster",
		},
		NATS: NATSConfig{
			Mode: NATSModeCluster,
		},
	}

	result := MergeSeedConfig(base, override)

	if result.Cluster.Name != "override-cluster" {
		t.Errorf("expected overridden cluster name, got %s", result.Cluster.Name)
	}
	if result.NATS.Mode != NATSModeCluster {
		t.Errorf("expected overridden NATS mode, got %s", result.NATS.Mode)
	}
	// Base values should be preserved where not overridden
	if result.Database.Type != DatabaseTypeSQLite {
		t.Errorf("expected base database type to be preserved, got %s", result.Database.Type)
	}
}

// TestExportSeedConfig tests configuration export
func TestExportSeedConfig(t *testing.T) {
	config := DefaultSeedConfig()
	data, err := ExportSeedConfig(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty export")
	}
	// Verify it can be parsed back
	loader := NewConfigLoader()
	_, err = loader.ParseSeedConfig(data)
	if err != nil {
		t.Errorf("exported config cannot be parsed: %v", err)
	}
}

// TestInstallerRegistry tests the installer registry
func TestInstallerRegistry(t *testing.T) {
	t.Run("RegisterAndGet", func(t *testing.T) {
		registry := NewInstallerRegistry()
		installer := &ServerInstaller{BaseInstaller: BaseInstaller{}}
		registry.Register(ComponentServer, installer)

		got, ok := registry.Get(ComponentServer)
		if !ok {
			t.Fatal("expected to find registered installer")
		}
		if got != installer {
			t.Error("got different installer than registered")
		}
	})

	t.Run("GetUnregistered", func(t *testing.T) {
		registry := NewInstallerRegistry()
		_, ok := registry.Get(ComponentServer)
		if ok {
			t.Error("expected not to find unregistered component")
		}
	})
}

// TestBootstrapper tests the bootstrapper
func TestBootstrapper(t *testing.T) {
	t.Run("NewBootstrapper", func(t *testing.T) {
		opts := Options{
			Mode: BootstrapModeSeed,
		}
		b, err := NewBootstrapper(opts, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if b == nil {
			t.Fatal("expected non-nil bootstrapper")
		}
	})

	t.Run("DryRun", func(t *testing.T) {
		opts := Options{
			Mode:   BootstrapModeSeed,
			DryRun: true,
		}
		b, _ := NewBootstrapper(opts, nil)

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		result, err := b.Bootstrap(ctx, opts)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.Success {
			t.Error("expected success for dry run")
		}
	})

	t.Run("Status", func(t *testing.T) {
		opts := Options{}
		b, _ := NewBootstrapper(opts, nil)
		status := b.Status()
		if status == nil {
			t.Fatal("expected non-nil status")
		}
		if status.Phase != PhaseInitializing {
			t.Errorf("expected initializing phase, got %s", status.Phase)
		}
	})
}

// TestClusterFormation tests cluster formation
func TestClusterFormation(t *testing.T) {
	t.Run("NewClusterFormation", func(t *testing.T) {
		config := DefaultSeedConfig()
		registry := NewInstallerRegistry()
		cf := NewClusterFormation(config, registry, nil)
		if cf == nil {
			t.Fatal("expected non-nil cluster formation")
		}
	})

	t.Run("GetStatus", func(t *testing.T) {
		config := DefaultSeedConfig()
		registry := NewInstallerRegistry()
		cf := NewClusterFormation(config, registry, nil)
		status := cf.GetStatus()
		if status == nil {
			t.Fatal("expected non-nil status")
		}
		if status.Phase != "initializing" {
			t.Errorf("expected initializing phase, got %s", status.Phase)
		}
	})
}

// TestHandoffManager tests handoff management
func TestHandoffManager(t *testing.T) {
	t.Run("NewHandoffManager", func(t *testing.T) {
		config := DefaultSeedConfig()
		result := &Result{
			APIEndpoint: "localhost:8080",
		}
		hm := NewHandoffManager(config, result, nil)
		if hm == nil {
			t.Fatal("expected non-nil handoff manager")
		}
	})
}

// TestLoadAndSaveHandoffState tests handoff state persistence
func TestLoadAndSaveHandoffState(t *testing.T) {
	tempDir := t.TempDir()

	config := DefaultSeedConfig()
	result := &Result{
		APIEndpoint: "localhost:8080",
	}
	hm := NewHandoffManager(config, result, nil)
	hm.stateDir = tempDir

	// Create and save state
	state := &HandoffState{
		Phase:          "test",
		StartTime:      time.Now(),
		CompletedSteps: []string{"step1", "step2"},
		HealthVerified: true,
	}
	err := hm.saveState(state)
	if err != nil {
		t.Fatalf("failed to save state: %v", err)
	}

	// Load state
	loaded, err := LoadHandoffState(tempDir)
	if err != nil {
		t.Fatalf("failed to load state: %v", err)
	}
	if loaded.Phase != "test" {
		t.Errorf("expected phase 'test', got %s", loaded.Phase)
	}
	if len(loaded.CompletedSteps) != 2 {
		t.Errorf("expected 2 completed steps, got %d", len(loaded.CompletedSteps))
	}
}

// TestLoadSeedConfigFromFile tests loading config from file
func TestLoadSeedConfigFromFile(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.yaml")

	yaml := `
cluster:
  name: file-test
control_plane:
  replicas: 1
`
	if err := os.WriteFile(configPath, []byte(yaml), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	loader := NewConfigLoader()
	config, err := loader.LoadSeedConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}
	if config.Cluster.Name != "file-test" {
		t.Errorf("expected cluster name 'file-test', got %s", config.Cluster.Name)
	}
}

// TestBootstrapPhases tests bootstrap phase constants
func TestBootstrapPhases(t *testing.T) {
	phases := []Phase{
		PhaseInitializing,
		PhaseValidating,
		PhaseInstallingDeps,
		PhaseInstallingServer,
		PhaseConfiguringServer,
		PhaseStartingServer,
		PhaseFormingCluster,
		PhaseInstallingAgents,
		PhaseApplyingStates,
		PhaseVerifying,
		PhaseHandoff,
		PhaseComplete,
		PhaseFailed,
	}

	for _, phase := range phases {
		if phase == "" {
			t.Error("found empty phase constant")
		}
	}
}

// TestBootstrapModes tests bootstrap mode constants
func TestBootstrapModes(t *testing.T) {
	modes := []Mode{
		BootstrapModeSeed,
		BootstrapModeRestore,
		BootstrapModeImport,
	}

	for _, mode := range modes {
		if mode == "" {
			t.Error("found empty mode constant")
		}
	}
}

// TestComponentTypes tests component type constants
func TestComponentTypes(t *testing.T) {
	types := []ComponentType{
		ComponentServer,
		ComponentAgent,
		ComponentNATS,
		ComponentPostgres,
		ComponentEtcd,
	}

	for _, ct := range types {
		if ct == "" {
			t.Error("found empty component type constant")
		}
	}
}

// TestValidationError tests validation error formatting
func TestValidationError(t *testing.T) {
	err := &ValidationError{
		Field:   "cluster.name",
		Message: "required",
	}
	expected := "cluster.name: required"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}

// TestValidationErrors tests multiple validation errors
func TestValidationErrors(t *testing.T) {
	errs := ValidationErrors{
		{Field: "field1", Message: "error1"},
		{Field: "field2", Message: "error2"},
	}
	result := errs.Error()
	if !strings.Contains(result, "field1: error1") {
		t.Error("expected field1 error in message")
	}
	if !strings.Contains(result, "field2: error2") {
		t.Error("expected field2 error in message")
	}
}

// TestEmptyValidationErrors tests empty validation errors
func TestEmptyValidationErrors(t *testing.T) {
	errs := ValidationErrors{}
	if errs.Error() != "" {
		t.Errorf("expected empty string, got %q", errs.Error())
	}
}
