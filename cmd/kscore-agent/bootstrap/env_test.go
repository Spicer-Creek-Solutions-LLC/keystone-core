package bootstrap

import (
	"os"
	"testing"
)

func TestApplyEnvOverrides(t *testing.T) {
	t.Setenv("KSCORE_BOOTSTRAP_MODE", "demo")
	t.Setenv("KSCORE_CLUSTER_NAME", "demo-cluster")
	t.Setenv("KSCORE_NODE_ROLE", "agent")
	t.Setenv("KSCORE_NODE_NAME", "node-1")
	t.Setenv("KSCORE_NODE_LABELS", "env=prod,role=agent")
	t.Setenv("KSCORE_JOIN_ENDPOINT", "https://example.com")
	t.Setenv("KSCORE_JOIN_TOKEN", "token")
	t.Setenv("KSCORE_STORAGE_BACKEND", "sqlite")
	t.Setenv("KSCORE_NATS_MODE", "embedded")
	t.Setenv("KSCORE_BIND_ADDRESS", "0.0.0.0")
	t.Setenv("KSCORE_ADVERTISE_ADDRESS", "10.0.0.1")
	t.Setenv("KSCORE_POSTGRES_HOST", "db.example.com")
	t.Setenv("KSCORE_POSTGRES_PORT", "5432")
	t.Setenv("KSCORE_POSTGRES_DATABASE", "keystone")
	t.Setenv("KSCORE_POSTGRES_USER", "kscore")
	t.Setenv("KSCORE_POSTGRES_PASSWORD", "secret")
	t.Setenv("KSCORE_POSTGRES_SSLMODE", "prefer")
	t.Setenv("KSCORE_NATS_URLS", "nats://nats1:4222,nats://nats2:4222")
	t.Setenv("KSCORE_GENERATE_CERTS", "true")
	t.Setenv("KSCORE_TLS_CERT_FILE", "/etc/keystone-core/tls.crt")
	t.Setenv("KSCORE_TLS_KEY_FILE", "/etc/keystone-core/tls.key")
	t.Setenv("KSCORE_TLS_CA_FILE", "/etc/keystone-core/ca.crt")
	t.Setenv("KSCORE_TLS_CSR_FILE", "/etc/keystone-core/kscore.csr")
	t.Setenv("KSCORE_TLS_RENEWAL_COMMAND", "/usr/local/bin/renew-tls")
	t.Setenv("KSCORE_TLS_RENEWAL_SCRIPT", "/etc/keystone-core/tls/renew.sh")
	t.Setenv("KSCORE_NATS_CREDS_FILE", "/etc/keystone-core/nats.creds")
	t.Setenv("KSCORE_NATS_USER", "nats-user")
	t.Setenv("KSCORE_NATS_PASSWORD", "nats-pass")
	t.Setenv("KSCORE_PACKAGE_CHANNEL", "stable")
	t.Setenv("KSCORE_PACKAGE_VERSION", "1.2.3")
	t.Setenv("KSCORE_MIGRATE_FROM_SQLITE", "/var/lib/keystone-core/keystone-core.db")
	t.Setenv("KSCORE_MIGRATE_BATCH_SIZE", "250")
	t.Setenv("KSCORE_MIGRATE_CONTINUE_ON_ERROR", "true")
	t.Setenv("KSCORE_MIGRATE_SKIP_EXISTING", "false")
	t.Setenv("KSCORE_BLUEPRINTS_DIR", "/etc/keystone-core/blueprints")
	t.Setenv("KSCORE_APPLY_BLUEPRINTS", "blueprints/demo,blueprints/metrics@1.0.0")
	t.Setenv("KSCORE_BLUEPRINT_PARAMS", "blueprints/demo:replicas=2")
	t.Setenv("KSCORE_BLUEPRINT_FEATURES", "blueprints/demo:monitoring=true")
	t.Setenv("KSCORE_BLUEPRINT_ENTRYPOINTS", "blueprints/demo:states/primary.yaml")
	t.Setenv("KSCORE_EXPORT_STATES_DIR", "/tmp/kscore-export")
	t.Setenv("KSCORE_BOOTSTRAP_NON_INTERACTIVE", "true")

	opts := &Options{}
	applyEnvOverrides(opts)

	if opts.Mode != "demo" {
		t.Fatalf("expected mode demo, got %s", opts.Mode)
	}
	if opts.ClusterName != "demo-cluster" {
		t.Fatalf("expected cluster name demo-cluster, got %s", opts.ClusterName)
	}
	if opts.NodeRole != "agent" {
		t.Fatalf("expected node role agent, got %s", opts.NodeRole)
	}
	if opts.NodeName != "node-1" {
		t.Fatalf("expected node name node-1, got %s", opts.NodeName)
	}
	if len(opts.NodeLabelArgs) != 2 {
		t.Fatalf("expected 2 node label args, got %d", len(opts.NodeLabelArgs))
	}
	if opts.JoinEndpoint != "https://example.com" {
		t.Fatalf("expected join endpoint https://example.com, got %s", opts.JoinEndpoint)
	}
	if opts.JoinToken != "token" {
		t.Fatalf("expected join token token, got %s", opts.JoinToken)
	}
	if opts.StorageBackend != "sqlite" {
		t.Fatalf("expected storage sqlite, got %s", opts.StorageBackend)
	}
	if opts.NATSMode != "embedded" {
		t.Fatalf("expected nats mode embedded, got %s", opts.NATSMode)
	}
	if opts.BindAddress != "0.0.0.0" {
		t.Fatalf("expected bind address 0.0.0.0, got %s", opts.BindAddress)
	}
	if opts.AdvertiseAddr != "10.0.0.1" {
		t.Fatalf("expected advertise address 10.0.0.1, got %s", opts.AdvertiseAddr)
	}
	if opts.PostgresHost != "db.example.com" {
		t.Fatalf("expected postgres host db.example.com, got %s", opts.PostgresHost)
	}
	if opts.PostgresPort != 5432 {
		t.Fatalf("expected postgres port 5432, got %d", opts.PostgresPort)
	}
	if opts.PostgresDatabase != "keystone" {
		t.Fatalf("expected postgres database keystone, got %s", opts.PostgresDatabase)
	}
	if opts.PostgresUser != "kscore" {
		t.Fatalf("expected postgres user kscore, got %s", opts.PostgresUser)
	}
	if opts.PostgresPassword != "secret" {
		t.Fatalf("expected postgres password secret, got %s", opts.PostgresPassword)
	}
	if opts.PostgresSSLMode != "prefer" {
		t.Fatalf("expected postgres ssl mode prefer, got %s", opts.PostgresSSLMode)
	}
	if len(opts.NATSURLs) != 2 {
		t.Fatalf("expected 2 nats urls, got %d", len(opts.NATSURLs))
	}
	if !opts.GenerateCerts {
		t.Fatal("expected generate certs true")
	}
	if opts.TLSCertFile != "/etc/keystone-core/tls.crt" {
		t.Fatalf("expected tls cert file /etc/keystone-core/tls.crt, got %s", opts.TLSCertFile)
	}
	if opts.TLSKeyFile != "/etc/keystone-core/tls.key" {
		t.Fatalf("expected tls key file /etc/keystone-core/tls.key, got %s", opts.TLSKeyFile)
	}
	if opts.TLSCAFile != "/etc/keystone-core/ca.crt" {
		t.Fatalf("expected tls ca file /etc/keystone-core/ca.crt, got %s", opts.TLSCAFile)
	}
	if opts.TLSCSRFile != "/etc/keystone-core/kscore.csr" {
		t.Fatalf("expected tls csr file /etc/keystone-core/kscore.csr, got %s", opts.TLSCSRFile)
	}
	if opts.TLSRenewalCommand != "/usr/local/bin/renew-tls" {
		t.Fatalf("expected tls renewal command /usr/local/bin/renew-tls, got %s", opts.TLSRenewalCommand)
	}
	if opts.TLSRenewalScriptPath != "/etc/keystone-core/tls/renew.sh" {
		t.Fatalf("expected tls renewal script /etc/keystone-core/tls/renew.sh, got %s", opts.TLSRenewalScriptPath)
	}
	if opts.NATSCredsFile != "/etc/keystone-core/nats.creds" {
		t.Fatalf("expected nats creds file /etc/keystone-core/nats.creds, got %s", opts.NATSCredsFile)
	}
	if opts.NATSUser != "nats-user" {
		t.Fatalf("expected nats user nats-user, got %s", opts.NATSUser)
	}
	if opts.NATSPassword != "nats-pass" {
		t.Fatalf("expected nats password nats-pass, got %s", opts.NATSPassword)
	}
	if opts.PackageChannel != "stable" {
		t.Fatalf("expected package channel stable, got %s", opts.PackageChannel)
	}
	if opts.PackageVersion != "1.2.3" {
		t.Fatalf("expected package version 1.2.3, got %s", opts.PackageVersion)
	}
	if opts.MigrateFromSQLite != "/var/lib/keystone-core/keystone-core.db" {
		t.Fatalf("expected migrate from sqlite /var/lib/keystone-core/keystone-core.db, got %s", opts.MigrateFromSQLite)
	}
	if opts.MigrateBatchSize != 250 {
		t.Fatalf("expected migrate batch size 250, got %d", opts.MigrateBatchSize)
	}
	if !opts.MigrateContinueOnError {
		t.Fatal("expected migrate continue on error true")
	}
	if opts.MigrateSkipExisting {
		t.Fatal("expected migrate skip existing false")
	}
	if opts.BlueprintsDir != "/etc/keystone-core/blueprints" {
		t.Fatalf("expected blueprints dir /etc/keystone-core/blueprints, got %s", opts.BlueprintsDir)
	}
	if len(opts.ApplyBlueprints) != 2 {
		t.Fatalf("expected 2 apply blueprints, got %d", len(opts.ApplyBlueprints))
	}
	if len(opts.BlueprintParamArgs) != 1 {
		t.Fatalf("expected 1 blueprint param arg, got %d", len(opts.BlueprintParamArgs))
	}
	if len(opts.BlueprintFeatureArgs) != 1 {
		t.Fatalf("expected 1 blueprint feature arg, got %d", len(opts.BlueprintFeatureArgs))
	}
	if len(opts.BlueprintEntrypointArgs) != 1 {
		t.Fatalf("expected 1 blueprint entrypoint arg, got %d", len(opts.BlueprintEntrypointArgs))
	}
	if opts.ExportStatesDir != "/tmp/kscore-export" {
		t.Fatalf("expected export states dir /tmp/kscore-export, got %s", opts.ExportStatesDir)
	}
	if !opts.NonInteractive {
		t.Fatal("expected non-interactive to be true")
	}
}

func TestApplyEnvOverridesRespectsExisting(t *testing.T) {
	t.Setenv("KSCORE_BOOTSTRAP_MODE", "demo")

	opts := &Options{Mode: "production"}
	applyEnvOverrides(opts)

	if opts.Mode != "production" {
		t.Fatalf("expected mode production, got %s", opts.Mode)
	}

	if os.Getenv("KSCORE_BOOTSTRAP_MODE") == "" {
		t.Fatal("expected environment variable to be set for test")
	}
}
