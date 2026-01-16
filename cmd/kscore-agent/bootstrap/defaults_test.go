package bootstrap

import "testing"

func TestApplyOptionDefaults(t *testing.T) {
	opts := &Options{}
	applyOptionDefaults(opts, DeploymentModeDemo)

	if opts.ClusterName != "keystone" {
		t.Fatalf("expected default cluster name keystone, got %s", opts.ClusterName)
	}
	if opts.NodeRole != "both" {
		t.Fatalf("expected demo default node role both, got %s", opts.NodeRole)
	}

	opts = &Options{JoinEndpoint: "https://example.com"}
	applyOptionDefaults(opts, DeploymentModeProduction)
	if opts.NodeRole != "agent" {
		t.Fatalf("expected join default node role agent, got %s", opts.NodeRole)
	}
	if opts.StorageBackend != "postgres" {
		t.Fatalf("expected storage backend postgres, got %s", opts.StorageBackend)
	}
	if opts.NATSMode != "cluster" {
		t.Fatalf("expected nats mode cluster, got %s", opts.NATSMode)
	}
	if opts.PostgresPort != 5432 {
		t.Fatalf("expected postgres port 5432, got %d", opts.PostgresPort)
	}
	if opts.PostgresSSLMode != "prefer" {
		t.Fatalf("expected postgres ssl mode prefer, got %s", opts.PostgresSSLMode)
	}
	if opts.PackageChannel != "stable" {
		t.Fatalf("expected package channel stable, got %s", opts.PackageChannel)
	}

	opts = &Options{GenerateCerts: true, TLSCertFile: "/tmp/cert", TLSKeyFile: "/tmp/key", TLSCAFile: "/tmp/ca"}
	applyOptionDefaults(opts, DeploymentModeDemo)
	if opts.TLSCertFile != "/tmp/cert" || opts.TLSKeyFile != "/tmp/key" || opts.TLSCAFile != "/tmp/ca" {
		t.Fatal("expected TLS files to be preserved when generating certs")
	}

	opts = &Options{GenerateCerts: true}
	applyOptionDefaults(opts, DeploymentModeDemo)
	if opts.TLSCertFile != defaultTLSCertPath || opts.TLSKeyFile != defaultTLSKeyPath || opts.TLSCAFile != defaultTLSCAPath {
		t.Fatal("expected TLS defaults to be set when generating certs")
	}

	opts = &Options{ApplyBlueprints: []string{"blueprints/demo"}}
	applyOptionDefaults(opts, DeploymentModeDemo)
	if opts.BlueprintsDir != defaultBlueprintsDir {
		t.Fatalf("expected blueprints dir %s, got %s", defaultBlueprintsDir, opts.BlueprintsDir)
	}
}
