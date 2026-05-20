package backup_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	upstream "filippo.io/age"

	bkp "go.keystone-core.io/keystone-core/internal/backup"
	"go.keystone-core.io/keystone-core/internal/backup/age"
	cli "go.keystone-core.io/keystone-core/internal/cli/backup"
)

// buildArtifact writes a populated tar via the real BackupManager to
// outPath. enc is optional — when non-nil it wraps the file in an
// age envelope. Returns the manifest so tests can assert.
func buildArtifact(t *testing.T, outPath string, enc bkp.Encrypter) *bkp.Manifest {
	t.Helper()
	f, err := os.Create(outPath) //nolint:gosec // test temp file
	if err != nil {
		t.Fatalf("create %q: %v", outPath, err)
	}
	defer func() { _ = f.Close() }()

	wc, err := bkp.NewEncryptingWriter(f, enc)
	if err != nil {
		t.Fatalf("NewEncryptingWriter: %v", err)
	}

	clock := func() time.Time { return time.Unix(1700000000, 0).UTC() }
	mgr, err := bkp.NewBackupManager(
		bkp.WithConfig(&staticConfigCollector{files: []bkp.ConfigFile{
			{Name: "server.yaml", Body: []byte("server: {}")},
		}}),
		bkp.WithClusterName("test-cluster"),
		bkp.WithClock(clock),
	)
	if err != nil {
		t.Fatalf("NewBackupManager: %v", err)
	}
	manifest, err := mgr.CreateBackup(context.Background(), wc)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if err := wc.Close(); err != nil {
		t.Fatalf("Close encrypting writer: %v", err)
	}
	return manifest
}

type staticConfigCollector struct {
	files []bkp.ConfigFile
}

func (c *staticConfigCollector) Collect(_ context.Context) ([]bkp.ConfigFile, error) {
	return c.files, nil
}

func runRoot(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	root := cli.NewBackupCommand()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(args)
	err := root.ExecuteContext(context.Background())
	return stdout.String(), stderr.String(), err
}

func TestVerify_Happy_Plain(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.tar")
	buildArtifact(t, path, nil)

	stdout, _, err := runRoot(t, "verify", "--src", path)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("stdout missing OK: %q", stdout)
	}
	if !strings.Contains(stdout, "test-cluster") {
		t.Errorf("stdout missing cluster_name: %q", stdout)
	}
	if !strings.Contains(stdout, "server.yaml") {
		t.Errorf("stdout missing config entry: %q", stdout)
	}
}

func TestVerify_AgeEncrypted_RoundTrip(t *testing.T) {
	// Generate a real age keypair, write a recipients-derived
	// artifact, then verify with the matching identity.
	id, err := upstream.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity: %v", err)
	}

	dir := t.TempDir()
	keyPath := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(keyPath, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	artifactPath := filepath.Join(dir, "backup.tar")
	enc := &age.Encrypter{Recipients: []upstream.Recipient{id.Recipient()}}
	buildArtifact(t, artifactPath, enc)

	stdout, _, err := runRoot(t, "verify", "--src", artifactPath, "--age-identity", keyPath)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("stdout missing OK: %q", stdout)
	}
}

func TestVerify_Corrupted(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "backup.tar")
	buildArtifact(t, path, nil)

	// Flip a byte in the middle of the file to corrupt a tar entry
	// without disturbing the header.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 1024 {
		t.Fatalf("artifact unexpectedly small: %d", len(data))
	}
	// Find the manifest entry's body region by searching for "test-
	// cluster" (manifest is plaintext JSON). Skip past it and flip a
	// byte AFTER the manifest to avoid corrupting the manifest itself.
	// Easier: flip a byte at offset 200, which is inside the first
	// tar entry's body (manifest is last).
	data[200] ^= 0xFF
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}

	_, _, err = runRoot(t, "verify", "--src", path)
	if err == nil {
		t.Fatal("want error from corrupted artifact")
	}
	if !errors.Is(err, bkp.ErrIntegrityCheckFailed) &&
		!strings.Contains(err.Error(), "integrity") &&
		!strings.Contains(err.Error(), "tar") {
		t.Errorf("err = %v, want integrity / tar error", err)
	}
}

func TestVerify_MissingSrcFlag(t *testing.T) {
	_, _, err := runRoot(t, "verify")
	if err == nil {
		t.Fatal("want error for missing --src")
	}
}

func TestList_Happy(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"c.tar", "a.tar", "b.tar"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "ignore.txt"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runRoot(t, "list", "--dest", dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(stdout, "NAME") {
		t.Errorf("stdout missing header: %q", stdout)
	}
	// Sorted output: a.tar < b.tar < c.tar.
	aIdx := strings.Index(stdout, "a.tar")
	bIdx := strings.Index(stdout, "b.tar")
	cIdx := strings.Index(stdout, "c.tar")
	if aIdx < 0 || bIdx < 0 || cIdx < 0 {
		t.Fatalf("not all entries in output: %q", stdout)
	}
	if aIdx >= bIdx || bIdx >= cIdx {
		t.Errorf("entries not sorted: a=%d b=%d c=%d (%q)", aIdx, bIdx, cIdx, stdout)
	}
	if strings.Contains(stdout, "ignore.txt") {
		t.Errorf("non-.tar leaked into output: %q", stdout)
	}
}

func TestList_Empty(t *testing.T) {
	stdout, _, err := runRoot(t, "list", "--dest", t.TempDir())
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(stdout, "no artifacts found") {
		t.Errorf("stdout = %q, want 'no artifacts found'", stdout)
	}
}

func TestList_MissingDir(t *testing.T) {
	_, _, err := runRoot(t, "list", "--dest", filepath.Join(t.TempDir(), "nope"))
	if err == nil {
		t.Fatal("want error for missing dir")
	}
}

func TestList_MissingDestFlag(t *testing.T) {
	_, _, err := runRoot(t, "list")
	if err == nil {
		t.Fatal("want error for missing --dest")
	}
}

// ---- create + restore + round-trip ----

func TestCreate_LocalPlain(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yaml")
	if err := os.WriteFile(cfgPath, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "backup.tar")

	stdout, _, err := runRoot(t, "create",
		"--dest", out,
		"--config", cfgPath,
		"--cluster-name", "test-cluster",
	)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("stdout missing OK: %q", stdout)
	}
	if !strings.Contains(stdout, "server.yaml") {
		t.Errorf("stdout missing config entry: %q", stdout)
	}
	if _, err := os.Stat(out); err != nil {
		t.Errorf("artifact missing: %v", err)
	}
}

func TestCreate_MissingDestFlag(t *testing.T) {
	_, _, err := runRoot(t, "create")
	if err == nil {
		t.Fatal("want error for missing --dest")
	}
}

func TestCreate_BadConfigPath(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "backup.tar")
	_, _, err := runRoot(t, "create",
		"--dest", out,
		"--config", filepath.Join(dir, "nope.yaml"),
	)
	if err == nil {
		t.Fatal("want error for missing config file")
	}
}

func TestRestore_LocalPlain_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yaml")
	if err := os.WriteFile(cfgPath, []byte("server: original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(dir, "backup.tar")

	if _, _, err := runRoot(t, "create",
		"--dest", artifactPath,
		"--config", cfgPath,
		"--cluster-name", "rt-cluster",
	); err != nil {
		t.Fatalf("create: %v", err)
	}

	outDir := filepath.Join(dir, "out")
	stdout, _, err := runRoot(t, "restore",
		"--src", artifactPath,
		"--config-out-dir", outDir,
	)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("restore stdout missing OK: %q", stdout)
	}

	restored, err := os.ReadFile(filepath.Join(outDir, "server.yaml"))
	if err != nil {
		t.Fatalf("read restored config: %v", err)
	}
	if string(restored) != "server: original\n" {
		t.Errorf("restored content = %q, want unchanged", restored)
	}
}

func TestRestore_FullRoundTrip_AgeEncrypted(t *testing.T) {
	// Generate an age keypair; write recipients + identity files.
	id, err := upstream.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity: %v", err)
	}
	dir := t.TempDir()
	identityPath := filepath.Join(dir, "key.txt")
	if err := os.WriteFile(identityPath, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	recipientsPath := filepath.Join(dir, "key.pub")
	if err := os.WriteFile(recipientsPath, []byte(id.Recipient().String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cfgA := filepath.Join(dir, "server.yaml")
	cfgB := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(cfgA, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgB, []byte("agent: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	artifactPath := filepath.Join(dir, "backup.tar")

	// create → list → verify → restore, all via root command.
	if _, _, err := runRoot(t, "create",
		"--dest", artifactPath,
		"--config", cfgA,
		"--config", cfgB,
		"--age-recipients", recipientsPath,
		"--cluster-name", "rt-cluster",
	); err != nil {
		t.Fatalf("create: %v", err)
	}

	listOut, _, err := runRoot(t, "list", "--dest", dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(listOut, "backup.tar") {
		t.Errorf("list missing artifact: %q", listOut)
	}

	verifyOut, _, err := runRoot(t, "verify",
		"--src", artifactPath,
		"--age-identity", identityPath,
	)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !strings.Contains(verifyOut, "OK") {
		t.Errorf("verify stdout missing OK: %q", verifyOut)
	}

	outDir := filepath.Join(dir, "out")
	restoreOut, _, err := runRoot(t, "restore",
		"--src", artifactPath,
		"--age-identity", identityPath,
		"--config-out-dir", outDir,
	)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !strings.Contains(restoreOut, "OK") {
		t.Errorf("restore stdout missing OK: %q", restoreOut)
	}

	for _, name := range []string{"server.yaml", "agent.yaml"} {
		b, err := os.ReadFile(filepath.Join(outDir, name))
		if err != nil {
			t.Errorf("restored %s missing: %v", name, err)
			continue
		}
		if len(b) == 0 {
			t.Errorf("restored %s empty", name)
		}
	}
}

func TestRestore_MissingSrcFlag(t *testing.T) {
	_, _, err := runRoot(t, "restore")
	if err == nil {
		t.Fatal("want error for missing --src")
	}
}

func TestRestore_NoConfigHandlerForConfigComponent(t *testing.T) {
	// Create an artifact with config, then restore WITHOUT
	// --config-out-dir. Per Task-6 semantics, Selection asking for a
	// present component without a wired handler is a loud error.
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "server.yaml")
	if err := os.WriteFile(cfgPath, []byte("server: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join(dir, "backup.tar")
	if _, _, err := runRoot(t, "create",
		"--dest", artifactPath,
		"--config", cfgPath,
	); err != nil {
		t.Fatalf("create: %v", err)
	}

	_, _, err := runRoot(t, "restore", "--src", artifactPath)
	if err == nil {
		t.Fatal("want error: artifact has config but no --config-out-dir wired")
	}
}

func TestRestore_NoConfigComponent_NoHandlerNeeded(t *testing.T) {
	// Empty artifact (no components) — restore should succeed with
	// no handlers wired.
	dir := t.TempDir()
	artifactPath := filepath.Join(dir, "backup.tar")
	if _, _, err := runRoot(t, "create",
		"--dest", artifactPath,
	); err != nil {
		t.Fatalf("create: %v", err)
	}

	stdout, _, err := runRoot(t, "restore", "--src", artifactPath)
	if err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !strings.Contains(stdout, "OK") {
		t.Errorf("stdout missing OK: %q", stdout)
	}
}

func TestRoot_LogLevelDefaultsToInfo(t *testing.T) {
	// Smoke: invoking the root with no args prints help, not an
	// error. The PersistentPreRunE that wires the logger should not
	// fail on the default level.
	stdout, _, err := runRoot(t)
	if err != nil {
		t.Fatalf("root: %v", err)
	}
	if !strings.Contains(stdout, "kscore-backup") {
		t.Errorf("help output missing binary name: %q", stdout)
	}
}

// silence unused-import warnings if io is only needed in some builds
var _ = io.Discard
