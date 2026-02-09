package bootstrap

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBootstrapConfigLoadWrite(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bootstrap.yaml")

	cfg := &Config{
		Mode:                   "demo",
		ClusterName:            "keystone",
		NodeRole:               "agent",
		NodeName:               "node-1",
		NodeLabels:             map[string]string{"role": "agent"},
		Regions:                []string{"us-east-1", "us-west-2"},
		HAEnabled:              true,
		HAReplicas:             3,
		ObservabilityBackend:   "prometheus",
		ObservabilityEndpoint:  "https://obs.example.com",
		IdentityProvider:       "oidc",
		IdentityEndpoint:       "https://id.example.com",
		Join:                   "https://example.com",
		JoinToken:              "token",
		Storage:                "sqlite",
		NATSMode:               "embedded",
		BindAddress:            "0.0.0.0",
		Advertise:              "10.0.0.1",
		PostgresHost:           "db.example.com",
		PostgresPort:           5432,
		PostgresDatabase:       "keystone",
		PostgresUser:           "kscore",
		PostgresPassword:       "secret",
		PostgresSSLMode:        "prefer",
		NATSURLs:               []string{"nats://nats1:4222", "nats://nats2:4222"},
		GenerateCerts:          true,
		TLSCertFile:            "/etc/keystone-core/tls.crt",
		TLSKeyFile:             "/etc/keystone-core/tls.key",
		TLSCAFile:              "/etc/keystone-core/ca.crt",
		TLSCSRFile:             "/etc/keystone-core/tls/kscore.csr",
		TLSRenewalCommand:      "/usr/local/bin/renew-tls",
		TLSRenewalScriptPath:   "/etc/keystone-core/tls/renew.sh",
		NATSCredsFile:          "/etc/keystone-core/nats.creds",
		NATSUser:               "nats-user",
		NATSPassword:           "nats-pass",
		PackageChannel:         "stable",
		PackageVersion:         "1.2.3",
		MigrateFromSQLite:      "/var/lib/keystone-core/keystone-core.db",
		MigrateBatchSize:       250,
		MigrateContinueOnError: true,
		MigrateSkipExisting:    true,
		BlueprintsDir:          "/etc/keystone-core/blueprints",
		ApplyBlueprints:        []string{"blueprints/demo", "blueprints/metrics@1.0.0"},
		BlueprintParams: map[string]map[string]interface{}{
			"blueprints/demo": {
				"replicas": 2,
			},
		},
		BlueprintFeatures: map[string]map[string]bool{
			"blueprints/demo": {
				"monitoring": true,
			},
		},
		BlueprintEntrypoints: map[string]string{
			"blueprints/demo": "states/primary.yaml",
		},
		ExportStatesDir: "/tmp/kscore-export",
	}

	if err := WriteBootstrapConfig(path, cfg); err != nil {
		t.Fatalf("WriteBootstrapConfig failed: %v", err)
	}

	loaded, err := LoadBootstrapConfig(path)
	if err != nil {
		t.Fatalf("LoadBootstrapConfig failed: %v", err)
	}

	if loaded.Mode != cfg.Mode {
		t.Fatalf("expected mode %s, got %s", cfg.Mode, loaded.Mode)
	}
	if loaded.ClusterName != cfg.ClusterName {
		t.Fatalf("expected cluster name %s, got %s", cfg.ClusterName, loaded.ClusterName)
	}
	if loaded.NodeRole != cfg.NodeRole {
		t.Fatalf("expected node role %s, got %s", cfg.NodeRole, loaded.NodeRole)
	}
	if loaded.NodeName != cfg.NodeName {
		t.Fatalf("expected node name %s, got %s", cfg.NodeName, loaded.NodeName)
	}
	if loaded.NodeLabels["role"] != cfg.NodeLabels["role"] {
		t.Fatalf("expected node label role %s, got %s", cfg.NodeLabels["role"], loaded.NodeLabels["role"])
	}
	if len(loaded.Regions) != len(cfg.Regions) {
		t.Fatalf("expected %d regions, got %d", len(cfg.Regions), len(loaded.Regions))
	}
	if loaded.HAEnabled != cfg.HAEnabled {
		t.Fatalf("expected ha enabled %v, got %v", cfg.HAEnabled, loaded.HAEnabled)
	}
	if loaded.HAReplicas != cfg.HAReplicas {
		t.Fatalf("expected ha replicas %d, got %d", cfg.HAReplicas, loaded.HAReplicas)
	}
	if loaded.ObservabilityBackend != cfg.ObservabilityBackend {
		t.Fatalf("expected observability backend %s, got %s", cfg.ObservabilityBackend, loaded.ObservabilityBackend)
	}
	if loaded.ObservabilityEndpoint != cfg.ObservabilityEndpoint {
		t.Fatalf("expected observability endpoint %s, got %s", cfg.ObservabilityEndpoint, loaded.ObservabilityEndpoint)
	}
	if loaded.IdentityProvider != cfg.IdentityProvider {
		t.Fatalf("expected identity provider %s, got %s", cfg.IdentityProvider, loaded.IdentityProvider)
	}
	if loaded.IdentityEndpoint != cfg.IdentityEndpoint {
		t.Fatalf("expected identity endpoint %s, got %s", cfg.IdentityEndpoint, loaded.IdentityEndpoint)
	}
	if loaded.Join != cfg.Join {
		t.Fatalf("expected join %s, got %s", cfg.Join, loaded.Join)
	}
	if loaded.JoinToken != cfg.JoinToken {
		t.Fatalf("expected join token %s, got %s", cfg.JoinToken, loaded.JoinToken)
	}
	if loaded.Storage != cfg.Storage {
		t.Fatalf("expected storage %s, got %s", cfg.Storage, loaded.Storage)
	}
	if loaded.NATSMode != cfg.NATSMode {
		t.Fatalf("expected nats mode %s, got %s", cfg.NATSMode, loaded.NATSMode)
	}
	if loaded.BindAddress != cfg.BindAddress {
		t.Fatalf("expected bind address %s, got %s", cfg.BindAddress, loaded.BindAddress)
	}
	if loaded.Advertise != cfg.Advertise {
		t.Fatalf("expected advertise address %s, got %s", cfg.Advertise, loaded.Advertise)
	}
	if loaded.PostgresHost != cfg.PostgresHost {
		t.Fatalf("expected postgres host %s, got %s", cfg.PostgresHost, loaded.PostgresHost)
	}
	if loaded.PostgresPort != cfg.PostgresPort {
		t.Fatalf("expected postgres port %d, got %d", cfg.PostgresPort, loaded.PostgresPort)
	}
	if loaded.PostgresDatabase != cfg.PostgresDatabase {
		t.Fatalf("expected postgres database %s, got %s", cfg.PostgresDatabase, loaded.PostgresDatabase)
	}
	if loaded.PostgresUser != cfg.PostgresUser {
		t.Fatalf("expected postgres user %s, got %s", cfg.PostgresUser, loaded.PostgresUser)
	}
	if loaded.PostgresPassword != cfg.PostgresPassword {
		t.Fatalf("expected postgres password %s, got %s", cfg.PostgresPassword, loaded.PostgresPassword)
	}
	if loaded.PostgresSSLMode != cfg.PostgresSSLMode {
		t.Fatalf("expected postgres ssl mode %s, got %s", cfg.PostgresSSLMode, loaded.PostgresSSLMode)
	}
	if len(loaded.NATSURLs) != len(cfg.NATSURLs) {
		t.Fatalf("expected %d NATS URLs, got %d", len(cfg.NATSURLs), len(loaded.NATSURLs))
	}
	if loaded.GenerateCerts != cfg.GenerateCerts {
		t.Fatalf("expected generate certs %v, got %v", cfg.GenerateCerts, loaded.GenerateCerts)
	}
	if loaded.TLSCertFile != cfg.TLSCertFile {
		t.Fatalf("expected tls cert file %s, got %s", cfg.TLSCertFile, loaded.TLSCertFile)
	}
	if loaded.TLSKeyFile != cfg.TLSKeyFile {
		t.Fatalf("expected tls key file %s, got %s", cfg.TLSKeyFile, loaded.TLSKeyFile)
	}
	if loaded.TLSCAFile != cfg.TLSCAFile {
		t.Fatalf("expected tls ca file %s, got %s", cfg.TLSCAFile, loaded.TLSCAFile)
	}
	if loaded.TLSCSRFile != cfg.TLSCSRFile {
		t.Fatalf("expected tls csr file %s, got %s", cfg.TLSCSRFile, loaded.TLSCSRFile)
	}
	if loaded.TLSRenewalCommand != cfg.TLSRenewalCommand {
		t.Fatalf("expected tls renewal command %s, got %s", cfg.TLSRenewalCommand, loaded.TLSRenewalCommand)
	}
	if loaded.TLSRenewalScriptPath != cfg.TLSRenewalScriptPath {
		t.Fatalf("expected tls renewal script %s, got %s", cfg.TLSRenewalScriptPath, loaded.TLSRenewalScriptPath)
	}
	if loaded.NATSCredsFile != cfg.NATSCredsFile {
		t.Fatalf("expected nats creds file %s, got %s", cfg.NATSCredsFile, loaded.NATSCredsFile)
	}
	if loaded.NATSUser != cfg.NATSUser {
		t.Fatalf("expected nats user %s, got %s", cfg.NATSUser, loaded.NATSUser)
	}
	if loaded.NATSPassword != cfg.NATSPassword {
		t.Fatalf("expected nats password %s, got %s", cfg.NATSPassword, loaded.NATSPassword)
	}
	if loaded.PackageChannel != cfg.PackageChannel {
		t.Fatalf("expected package channel %s, got %s", cfg.PackageChannel, loaded.PackageChannel)
	}
	if loaded.PackageVersion != cfg.PackageVersion {
		t.Fatalf("expected package version %s, got %s", cfg.PackageVersion, loaded.PackageVersion)
	}
	if loaded.MigrateFromSQLite != cfg.MigrateFromSQLite {
		t.Fatalf("expected migrate from sqlite %s, got %s", cfg.MigrateFromSQLite, loaded.MigrateFromSQLite)
	}
	if loaded.MigrateBatchSize != cfg.MigrateBatchSize {
		t.Fatalf("expected migrate batch size %d, got %d", cfg.MigrateBatchSize, loaded.MigrateBatchSize)
	}
	if loaded.MigrateContinueOnError != cfg.MigrateContinueOnError {
		t.Fatalf("expected migrate continue on error %v, got %v", cfg.MigrateContinueOnError, loaded.MigrateContinueOnError)
	}
	if loaded.MigrateSkipExisting != cfg.MigrateSkipExisting {
		t.Fatalf("expected migrate skip existing %v, got %v", cfg.MigrateSkipExisting, loaded.MigrateSkipExisting)
	}
	if loaded.BlueprintsDir != cfg.BlueprintsDir {
		t.Fatalf("expected blueprints dir %s, got %s", cfg.BlueprintsDir, loaded.BlueprintsDir)
	}
	if len(loaded.ApplyBlueprints) != len(cfg.ApplyBlueprints) {
		t.Fatalf("expected %d apply blueprints, got %d", len(cfg.ApplyBlueprints), len(loaded.ApplyBlueprints))
	}
	if len(loaded.BlueprintParams) != len(cfg.BlueprintParams) {
		t.Fatalf("expected %d blueprint params, got %d", len(cfg.BlueprintParams), len(loaded.BlueprintParams))
	}
	if loaded.ExportStatesDir != cfg.ExportStatesDir {
		t.Fatalf("expected export states dir %s, got %s", cfg.ExportStatesDir, loaded.ExportStatesDir)
	}
	if len(loaded.BlueprintFeatures) != len(cfg.BlueprintFeatures) {
		t.Fatalf("expected %d blueprint features, got %d", len(cfg.BlueprintFeatures), len(loaded.BlueprintFeatures))
	}
	if len(loaded.BlueprintEntrypoints) != len(cfg.BlueprintEntrypoints) {
		t.Fatalf("expected %d blueprint entrypoints, got %d", len(cfg.BlueprintEntrypoints), len(loaded.BlueprintEntrypoints))
	}
}

func TestLoadBootstrapConfigMissingFile(t *testing.T) {
	_, err := LoadBootstrapConfig(filepath.Join(t.TempDir(), "missing.yaml"))
	if err == nil {
		t.Fatal("expected error for missing config")
	}
}

func TestWriteBootstrapConfigPermissions(t *testing.T) {
	tmpDir := t.TempDir()
	path := filepath.Join(tmpDir, "bootstrap.yaml")

	cfg := &Config{Mode: "demo"}
	if err := WriteBootstrapConfig(path, cfg); err != nil {
		t.Fatalf("WriteBootstrapConfig failed: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat failed: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("expected 0600 permissions, got %v", info.Mode().Perm())
	}
}
