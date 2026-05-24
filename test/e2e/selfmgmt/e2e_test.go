// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Package selfmgmte2e is the Epic 18 task-8 end-to-end integration
// suite for the self-management subsection (tasks 1-7). It wires the
// REAL shipped binaries' cobra commands (kscore-bootstrap and
// kscore-backup) and drives them against a temp-dir-staged
// "cluster" — a directory of config files standing in for the
// kscore-server's persistent footprint. Per the Epic-13/16/17
// precedent, "full cluster → backup → fresh cluster → restore"
// is met when the mechanism is proven in-process even if real
// per-component adapter wiring is deferred (gate-v1.0 ROADMAP
// entry "Backup + restore component adapters").
//
// Build-tagged `integration` (run via `make test-integration`), so
// it is excluded from the default `go test ./...`.
package selfmgmte2e

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	upstream "filippo.io/age"

	bkp "go.keystone-core.io/keystone-core/internal/backup"
	bkpage "go.keystone-core.io/keystone-core/internal/backup/age"
	bkpcli "go.keystone-core.io/keystone-core/internal/cli/backup"
	bootcli "go.keystone-core.io/keystone-core/internal/cli/bootstrap"
)

// ---- helpers ----------------------------------------------------------

func runBackup(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := bkpcli.NewBackupCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Logf("backup stderr: %s", stderr.String())
	}
	return stdout.String(), err
}

func runBootstrap(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := bootcli.NewBootstrapCommand()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.ExecuteContext(context.Background())
	if err != nil {
		t.Logf("bootstrap stderr: %s", stderr.String())
	}
	return stdout.String(), err
}

// stageCluster writes the named files to dir with deterministic
// payloads of the given sizes. Returns the {name -> sha256-hex} map
// so the round-trip can assert byte-equal after restore.
func stageCluster(t *testing.T, dir string, files map[string]int) map[string]string {
	t.Helper()
	hashes := make(map[string]string, len(files))
	for name, size := range files {
		body := bytes.Repeat([]byte{byte(len(name) % 256)}, size)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, body, 0o600); err != nil {
			t.Fatalf("stage %s: %v", name, err)
		}
		h := sha256.Sum256(body)
		hashes[name] = hex.EncodeToString(h[:])
	}
	return hashes
}

// dirHashes hashes every regular file in dir keyed by basename.
func dirHashes(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}
	out := make(map[string]string, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name())
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("ReadFile %s: %v", path, err)
		}
		h := sha256.Sum256(b)
		out[e.Name()] = hex.EncodeToString(h[:])
	}
	return out
}

func writeSeedFile(t *testing.T, dir string) string {
	t.Helper()
	body := `mode: development
cluster_name: e2e-cluster
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
	path := filepath.Join(dir, "seed.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// configPathsFromStage flattens a stageCluster map into a sorted
// slice of absolute paths suitable for repeated --config flags.
func configPathsFromStage(dir string, files map[string]int) []string {
	out := make([]string, 0, len(files))
	for name := range files {
		out = append(out, filepath.Join(dir, name))
	}
	return out
}

// ---- bootstrap --------------------------------------------------------

func TestBootstrap_FSM_RunsToVerified(t *testing.T) {
	dir := t.TempDir()
	seedPath := writeSeedFile(t, dir)

	stdout, err := runBootstrap(t, "--seed", seedPath)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	want := []string{
		"OK",
		"state:        verified",
		"cluster_name: e2e-cluster",
		"node_role:    seed",
		"transitions:  12",
	}
	for _, line := range want {
		if !strings.Contains(stdout, line) {
			t.Errorf("stdout missing %q\nfull output:\n%s", line, stdout)
		}
	}
}

// ---- plain round-trip ------------------------------------------------

func TestBackup_Restore_RoundTrip_Plain(t *testing.T) {
	clusterDir := t.TempDir()
	files := map[string]int{
		"kscore-server.yaml": 1 << 10,  // 1 KB
		"kscore-agent.yaml":  100 << 10, // 100 KB
		"kscore-cluster.yaml": 1 << 20, // 1 MB
		"audit.yaml":         2 << 10,
		"policy.yaml":        2 << 10,
	}
	want := stageCluster(t, clusterDir, files)
	configPaths := configPathsFromStage(clusterDir, files)

	artifactDir := t.TempDir()
	artifactPath := filepath.Join(artifactDir, "backup.tar")

	createArgs := append([]string{"create",
		"--dest", artifactPath,
		"--cluster-name", "e2e-plain",
	}, addConfigFlags(configPaths)...)
	stdout, err := runBackup(t, createArgs...)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if !strings.Contains(stdout, "OK") || !strings.Contains(stdout, "e2e-plain") {
		t.Errorf("create stdout: %s", stdout)
	}

	if _, err := runBackup(t, "list", "--dest", artifactDir); err != nil {
		t.Fatalf("list: %v", err)
	}

	if _, err := runBackup(t, "verify", "--src", artifactPath); err != nil {
		t.Fatalf("verify: %v", err)
	}

	freshDir := t.TempDir()
	if _, err := runBackup(t, "restore",
		"--src", artifactPath,
		"--config-out-dir", freshDir,
	); err != nil {
		t.Fatalf("restore: %v", err)
	}

	got := dirHashes(t, freshDir)
	assertHashMapEqual(t, got, want)
}

// ---- age-encrypted round-trip ----------------------------------------

func TestBackup_Restore_RoundTrip_AgeEncrypted(t *testing.T) {
	clusterDir := t.TempDir()
	files := map[string]int{
		"a.yaml": 4 << 10,
		"b.yaml": 64 << 10,
		"c.yaml": 512 << 10,
	}
	want := stageCluster(t, clusterDir, files)
	configPaths := configPathsFromStage(clusterDir, files)

	// Generate keypair, write identity + recipients files.
	id, err := upstream.GenerateX25519Identity()
	if err != nil {
		t.Fatalf("GenerateX25519Identity: %v", err)
	}
	keyDir := t.TempDir()
	identityPath := filepath.Join(keyDir, "key.txt")
	recipientsPath := filepath.Join(keyDir, "key.pub")
	if err := os.WriteFile(identityPath, []byte(id.String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(recipientsPath, []byte(id.Recipient().String()+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	artifactDir := t.TempDir()
	artifactPath := filepath.Join(artifactDir, "backup.tar")

	createArgs := append([]string{"create",
		"--dest", artifactPath,
		"--age-recipients", recipientsPath,
		"--cluster-name", "e2e-age",
	}, addConfigFlags(configPaths)...)
	if _, err := runBackup(t, createArgs...); err != nil {
		t.Fatalf("create: %v", err)
	}

	// The artifact bytes should not contain any plaintext config.
	// Spot-check by looking for the magic suffix of the unencrypted
	// tar trailer (manifest.json) and asserting it is NOT present.
	encryptedBytes, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(encryptedBytes, []byte("manifest.json")) {
		t.Errorf("encrypted artifact appears to contain plaintext manifest.json (age envelope failed)")
	}
	if !bytes.HasPrefix(encryptedBytes, []byte("age-encryption.org/v1\n")) {
		t.Errorf("encrypted artifact missing age header (first 24 bytes = %q)", encryptedBytes[:min(24, len(encryptedBytes))])
	}

	if _, err := runBackup(t, "verify",
		"--src", artifactPath,
		"--age-identity", identityPath,
	); err != nil {
		t.Fatalf("verify: %v", err)
	}

	freshDir := t.TempDir()
	if _, err := runBackup(t, "restore",
		"--src", artifactPath,
		"--age-identity", identityPath,
		"--config-out-dir", freshDir,
	); err != nil {
		t.Fatalf("restore: %v", err)
	}

	got := dirHashes(t, freshDir)
	assertHashMapEqual(t, got, want)
}

// ---- populated-cluster guard -----------------------------------------

// stubDetector is the one-method ClusterDetector for the populated-
// cluster scenario. The CLI does not wire a real detector yet
// (production wiring defers per gate-v1.0 ROADMAP entry); this test
// constructs a RestoreManager directly to exercise the guard contract.
type stubDetector struct {
	populated bool
}

func (s stubDetector) IsPopulated(_ context.Context) (bool, error) {
	return s.populated, nil
}

// recordingConfigRestore captures the ConfigFile list so the
// populated-cluster test can confirm dispatch happened only on the
// --force path.
type recordingConfigRestore struct {
	files []bkp.ConfigFile
}

func (r *recordingConfigRestore) Restore(_ context.Context, files []bkp.ConfigFile) error {
	r.files = files
	return nil
}

func TestBackup_Restore_Force_Required_For_Populated(t *testing.T) {
	clusterDir := t.TempDir()
	files := map[string]int{"server.yaml": 2 << 10}
	stageCluster(t, clusterDir, files)
	configPaths := configPathsFromStage(clusterDir, files)

	artifactDir := t.TempDir()
	artifactPath := filepath.Join(artifactDir, "backup.tar")
	createArgs := append([]string{"create", "--dest", artifactPath}, addConfigFlags(configPaths)...)
	if _, err := runBackup(t, createArgs...); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Open + read the artifact for direct RestoreManager use.
	rc, err := os.Open(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(rc)
	_ = rc.Close()
	if err != nil {
		t.Fatal(err)
	}

	// First pass: populated detector + no force → expect error.
	rec1 := &recordingConfigRestore{}
	mgr, err := bkp.NewRestoreManager(
		bkp.WithConfigRestore(rec1),
		bkp.WithClusterDetector(stubDetector{populated: true}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = mgr.Restore(context.Background(), bytes.NewReader(body), bkp.RestoreOptions{
		Selection: bkp.SelectAll(),
	})
	if !errors.Is(err, bkp.ErrClusterPopulated) {
		t.Fatalf("err = %v, want ErrClusterPopulated", err)
	}
	if rec1.files != nil {
		t.Errorf("ConfigRestore invoked despite populated rejection")
	}

	// Second pass: same detector but Force=true → expect success.
	rec2 := &recordingConfigRestore{}
	mgr2, err := bkp.NewRestoreManager(
		bkp.WithConfigRestore(rec2),
		bkp.WithClusterDetector(stubDetector{populated: true}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mgr2.Restore(context.Background(), bytes.NewReader(body), bkp.RestoreOptions{
		Force:     true,
		Selection: bkp.SelectAll(),
	}); err != nil {
		t.Fatalf("Restore with Force: %v", err)
	}
	if len(rec2.files) != 1 || rec2.files[0].Name != "server.yaml" {
		t.Errorf("ConfigRestore not invoked correctly: %+v", rec2.files)
	}
}

// ---- tamper detection ------------------------------------------------

func TestVerify_DetectsTampering(t *testing.T) {
	clusterDir := t.TempDir()
	stageCluster(t, clusterDir, map[string]int{"server.yaml": 4 << 10})
	configPaths := configPathsFromStage(clusterDir, map[string]int{"server.yaml": 4 << 10})

	artifactPath := filepath.Join(t.TempDir(), "backup.tar")
	createArgs := append([]string{"create", "--dest", artifactPath}, addConfigFlags(configPaths)...)
	if _, err := runBackup(t, createArgs...); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Flip a byte inside the first tar entry body (offset 1024 is
	// well past the tar header but inside the first config entry's
	// 4 KB payload).
	raw, err := os.ReadFile(artifactPath)
	if err != nil {
		t.Fatal(err)
	}
	raw[1024] ^= 0xFF
	if err := os.WriteFile(artifactPath, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	_, err = runBackup(t, "verify", "--src", artifactPath)
	if err == nil {
		t.Fatal("verify: want error from tampered artifact")
	}
}

// ---- list output ------------------------------------------------------

func TestList_Sorts_And_Filters(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"z.tar", "a.tar", "m.tar"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("y"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(dir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}

	stdout, err := runBackup(t, "list", "--dest", dir)
	if err != nil {
		t.Fatalf("list: %v", err)
	}

	// Sorted: a < m < z.
	aIdx := strings.Index(stdout, "a.tar")
	mIdx := strings.Index(stdout, "m.tar")
	zIdx := strings.Index(stdout, "z.tar")
	if aIdx < 0 || mIdx < 0 || zIdx < 0 {
		t.Fatalf("missing entries: %q", stdout)
	}
	if aIdx >= mIdx || mIdx >= zIdx {
		t.Errorf("not sorted: a=%d m=%d z=%d (%q)", aIdx, mIdx, zIdx, stdout)
	}
	if strings.Contains(stdout, "notes.txt") {
		t.Errorf("non-.tar leaked: %q", stdout)
	}
	if strings.Contains(stdout, "subdir") {
		t.Errorf("subdir leaked: %q", stdout)
	}
}

// ---- shared helpers ---------------------------------------------------

func addConfigFlags(paths []string) []string {
	out := make([]string, 0, 2*len(paths))
	for _, p := range paths {
		out = append(out, "--config", p)
	}
	return out
}

func assertHashMapEqual(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Errorf("file count: got %d, want %d (got=%v want=%v)", len(got), len(want), keys(got), keys(want))
	}
	for name, h := range want {
		if got[name] != h {
			t.Errorf("%s: hash mismatch (got %s, want %s)", name, got[name], h)
		}
	}
}

func keys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// Confirm age + time + filippo are imported (older toolchains can
// warn on unused imports even when transitively reachable).
var _ = time.Time{}
var _ = io.EOF
var _ = bkpage.LoadIdentityFile
