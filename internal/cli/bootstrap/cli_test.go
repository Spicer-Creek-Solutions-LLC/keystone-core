package bootstrap_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cli "go.keystone-core.io/keystone-core/internal/cli/bootstrap"
)

const validSeed = `mode: development
cluster_name: test-cluster
node_role: seed
storage:
  driver: sqlite
  dsn: ./data/keystone.db
nats:
  mode: embedded
  endpoints: []
tls_strategy: self-signed
blueprints:
  - base
`

const invalidSeedBadMode = `mode: staging
cluster_name: test-cluster
node_role: seed
storage:
  driver: sqlite
  dsn: ./data/keystone.db
nats:
  mode: embedded
tls_strategy: self-signed
`

const malformedSeed = `mode: development
cluster_name: [unterminated
`

func runRoot(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewBootstrapCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	return stdout.String(), err
}

func writeSeed(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "seed.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestBootstrap_HappyPath(t *testing.T) {
	stdout, err := runRoot(t, "--seed", writeSeed(t, validSeed))
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("stdout missing OK: %q", stdout)
	}
	if !strings.Contains(stdout, "state:        verified") {
		t.Errorf("stdout missing verified state: %q", stdout)
	}
	if !strings.Contains(stdout, "cluster_name: test-cluster") {
		t.Errorf("stdout missing cluster_name: %q", stdout)
	}
	if !strings.Contains(stdout, "transitions:  12") {
		// 6 phases × (start + done) = 12 transitions
		t.Errorf("stdout transition count != 12: %q", stdout)
	}
}

func TestBootstrap_MissingSeedFlag(t *testing.T) {
	_, err := runRoot(t)
	if err == nil {
		t.Fatal("want error for missing --seed")
	}
}

func TestBootstrap_SeedFileNotFound(t *testing.T) {
	_, err := runRoot(t, "--seed", filepath.Join(t.TempDir(), "nope.yaml"))
	if err == nil {
		t.Fatal("want error for missing seed file")
	}
}

func TestBootstrap_SeedValidationError(t *testing.T) {
	_, err := runRoot(t, "--seed", writeSeed(t, invalidSeedBadMode))
	if err == nil {
		t.Fatal("want validation error for bad mode")
	}
	if !strings.Contains(err.Error(), "mode") {
		t.Errorf("err = %v, want mode-related", err)
	}
}

func TestBootstrap_MalformedSeed(t *testing.T) {
	_, err := runRoot(t, "--seed", writeSeed(t, malformedSeed))
	if err == nil {
		t.Fatal("want parse error for malformed seed")
	}
}
