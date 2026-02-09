package vm

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigExpandsEnvironmentVariables(t *testing.T) {
	t.Setenv("KSCORE_VM_TEST_HOST", "vm-demo.example.internal")
	t.Setenv("KSCORE_VM_TEST_KEY", "/tmp/test-key")

	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configData := `vm_provider: ssh
ssh:
  clean_nodes: true
  nodes:
    - name: demo
      host: ${KSCORE_VM_TEST_HOST}
      port: 22
      user: test
      key_file: ${KSCORE_VM_TEST_KEY}
      os: ubuntu-24.04
      role: both
`
	if err := os.WriteFile(configPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if len(cfg.SSH.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(cfg.SSH.Nodes))
	}

	node := cfg.SSH.Nodes[0]
	if node.Host != "vm-demo.example.internal" {
		t.Fatalf("host = %q, want %q", node.Host, "vm-demo.example.internal")
	}
	if node.KeyFile != "/tmp/test-key" {
		t.Fatalf("key_file = %q, want %q", node.KeyFile, "/tmp/test-key")
	}
}

func TestLoadConfigUnsetEnvironmentVariablesBecomeEmpty(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configData := `vm_provider: ssh
ssh:
  clean_nodes: true
  nodes:
    - name: demo
      host: ${KSCORE_VM_MISSING_HOST}
      port: 22
      user: test
      key_file: ~/.ssh/id_ed25519
      os: ubuntu-24.04
      role: both
`
	if err := os.WriteFile(configPath, []byte(configData), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}

	if len(cfg.SSH.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(cfg.SSH.Nodes))
	}

	if cfg.SSH.Nodes[0].Host != "" {
		t.Fatalf("host = %q, want empty string when env var is unset", cfg.SSH.Nodes[0].Host)
	}
}
