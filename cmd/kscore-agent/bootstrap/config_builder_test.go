package bootstrap

import (
	"testing"

	"gopkg.in/yaml.v3"
)

func TestBuildServerConfigPostgres(t *testing.T) {
	cfg := &Config{
		Storage:          "postgres",
		PostgresHost:     "db.example.com",
		PostgresPort:     5432,
		PostgresDatabase: "keystone",
		PostgresUser:     "kscore",
		PostgresPassword: "secret",
		PostgresSSLMode:  "require",
		NATSMode:         "external",
		NATSURLs:         []string{"nats://nats1:4222", "nats://nats2:4222"},
		NATSCredsFile:    "/etc/keystone-core/nats.creds",
		TLSCertFile:      "/etc/keystone-core/tls.crt",
		TLSKeyFile:       "/etc/keystone-core/tls.key",
	}

	data, err := buildServerConfig(cfg)
	if err != nil {
		t.Fatalf("buildServerConfig returned error: %v", err)
	}

	var decoded map[string]any
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	storage := decoded["storage"].(map[string]any)
	if storage["backend"] != "postgresql" {
		t.Fatalf("expected postgres backend, got %v", storage["backend"])
	}

	nats := decoded["nats"].(map[string]any)
	if nats["mode"] != "external" {
		t.Fatalf("expected external nats mode, got %v", nats["mode"])
	}
	if nats["credential"] != "/etc/keystone-core/nats.creds" {
		t.Fatalf("expected creds file, got %v", nats["credential"])
	}

	if _, ok := decoded["tls"]; !ok {
		t.Fatal("expected tls section to be set")
	}
}

func TestBuildAgentConfigDefaultURL(t *testing.T) {
	cfg := &Config{
		NATSMode:   "embedded",
		NodeName:   "node-1",
		NodeLabels: map[string]string{"role": "agent"},
	}

	data, err := buildAgentConfig(cfg)
	if err != nil {
		t.Fatalf("buildAgentConfig returned error: %v", err)
	}

	var decoded map[string]any
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	nats := decoded["nats"].(map[string]any)
	if nats["mode"] != "external" {
		t.Fatalf("expected external nats mode, got %v", nats["mode"])
	}
	if nats["url"] != "nats://127.0.0.1:4222" {
		t.Fatalf("unexpected nats url: %v", nats["url"])
	}

	agent := decoded["agent"].(map[string]any)
	if agent["id"] != "node-1" {
		t.Fatalf("expected agent id node-1, got %v", agent["id"])
	}
	labels := agent["labels"].(map[string]any)
	if labels["role"] != "agent" {
		t.Fatalf("expected agent label role=agent, got %v", labels["role"])
	}
}

func TestBuildNATSLeafConfig(t *testing.T) {
	cfg := &Config{
		NATSMode: "leaf",
		NATSURLs: []string{"nats://parent:4222"},
	}

	data, err := buildServerConfig(cfg)
	if err != nil {
		t.Fatalf("buildServerConfig returned error: %v", err)
	}

	var decoded map[string]any
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}

	nats := decoded["nats"].(map[string]any)
	if nats["mode"] != "leaf" {
		t.Fatalf("expected leaf nats mode, got %v", nats["mode"])
	}
	embedded := nats["embedded"].(map[string]any)
	urls := embedded["leafnodeurls"].([]any)
	if len(urls) != 1 || urls[0] != "nats://parent:4222" {
		t.Fatalf("unexpected leaf urls: %v", urls)
	}
}

func TestBuildServerConfigPreservesExportStates(t *testing.T) {
	cfg := &Config{
		Storage:         "sqlite",
		ExportStatesDir: "/tmp/exported",
	}
	data, err := buildServerConfig(cfg)
	if err != nil {
		t.Fatalf("buildServerConfig returned error: %v", err)
	}
	var decoded map[string]any
	if err := yaml.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal config: %v", err)
	}
	if _, ok := decoded["export_states_dir"]; ok {
		t.Fatal("export states dir should not be part of server config")
	}
}
