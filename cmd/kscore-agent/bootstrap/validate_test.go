package bootstrap

import "testing"

func TestValidateBootstrapConfig(t *testing.T) {
	cfg := &BootstrapConfig{
		Mode:        "demo",
		ClusterName: "keystone",
		NodeRole:    "both",
		Storage:     "sqlite",
		NATSMode:    "embedded",
	}

	if err := validateBootstrapConfig(cfg); err != nil {
		t.Fatalf("expected config to be valid: %v", err)
	}
}

func TestValidateBootstrapConfigErrors(t *testing.T) {
	cfg := &BootstrapConfig{}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for missing required fields")
	}

	cfg = &BootstrapConfig{
		Mode:        "demo",
		ClusterName: "keystone",
		NodeRole:    "invalid",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for invalid node role")
	}

	cfg = &BootstrapConfig{
		Mode:        "demo",
		ClusterName: "keystone",
		Storage:     "mysql",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for invalid storage backend")
	}

	cfg = &BootstrapConfig{
		Mode:        "demo",
		ClusterName: "keystone",
		NATSMode:    "invalid",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for invalid NATS mode")
	}

	cfg = &BootstrapConfig{
		Mode:        "production",
		ClusterName: "keystone",
		Storage:     "postgres",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for incomplete postgres config")
	}

	cfg = &BootstrapConfig{
		Mode:             "production",
		ClusterName:      "keystone",
		Storage:          "postgres",
		PostgresHost:     "db.example.com",
		PostgresPort:     5432,
		PostgresDatabase: "keystone",
		PostgresUser:     "kscore",
		PostgresSSLMode:  "invalid",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for invalid postgres ssl mode")
	}

	cfg = &BootstrapConfig{
		Mode:        "production",
		ClusterName: "keystone",
		NATSMode:    "external",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for missing nats urls")
	}

	cfg = &BootstrapConfig{
		Mode:        "production",
		ClusterName: "keystone",
		NATSMode:    "leaf",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for missing nats urls in leaf mode")
	}

	cfg = &BootstrapConfig{
		Mode:          "demo",
		ClusterName:   "keystone",
		NATSCredsFile: "/tmp/nats.creds",
		NATSUser:      "user",
		NATSPassword:  "pass",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for mixed nats auth methods")
	}

	cfg = &BootstrapConfig{
		Mode:        "demo",
		ClusterName: "keystone",
		NATSUser:    "user",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for missing nats password")
	}

	cfg = &BootstrapConfig{
		Mode:            "demo",
		ClusterName:     "keystone",
		ApplyBlueprints: []string{"blueprints/demo"},
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for missing blueprints dir")
	}

	cfg = &BootstrapConfig{
		Mode:        "demo",
		ClusterName: "keystone",
		BlueprintParams: map[string]map[string]interface{}{
			"blueprints/demo": {
				"replicas": 2,
			},
		},
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for missing blueprints dir with params")
	}

	cfg = &BootstrapConfig{
		Mode:        "production",
		ClusterName: "keystone",
		TLSCertFile: "/tmp/cert.pem",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for missing tls key")
	}

	cfg = &BootstrapConfig{
		Mode:          "production",
		ClusterName:   "keystone",
		GenerateCerts: true,
		TLSCSRFile:    "/tmp/kscore.csr",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for csr file with generated certs")
	}

	cfg = &BootstrapConfig{
		Mode:        "demo",
		ClusterName: "keystone",
		Join:        "https://example.com",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for join endpoint without token")
	}

	cfg = &BootstrapConfig{
		Mode:        "demo",
		ClusterName: "keystone",
		NodeLabels: map[string]string{
			"": "value",
		},
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for invalid node labels")
	}

	cfg = &BootstrapConfig{
		Mode:            "demo",
		ClusterName:     "keystone",
		ExportStatesDir: "/tmp/export",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for export states without blueprints")
	}

	cfg = &BootstrapConfig{
		Mode:              "demo",
		ClusterName:       "keystone",
		Storage:           "sqlite",
		MigrateFromSQLite: "/tmp/kscore.db",
	}
	if err := validateBootstrapConfig(cfg); err == nil {
		t.Fatal("expected error for migrate with non-postgres storage")
	}
}
