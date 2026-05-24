// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"
)

// recordingComponent is a configurable test double that writes a
// fixed payload (or returns failErr) via every component-interface
// method. The same struct value implements all six interfaces, which
// is fine for unit tests — each Option wires the right method.
type recordingComponent struct {
	body    []byte
	files   []ConfigFile
	failErr error
}

func (r *recordingComponent) write(w io.Writer) error {
	if r.failErr != nil {
		return r.failErr
	}
	_, err := w.Write(r.body)
	return err
}

func (r *recordingComponent) Dump(_ context.Context, w io.Writer) error     { return r.write(w) }
func (r *recordingComponent) Snapshot(_ context.Context, w io.Writer) error { return r.write(w) }
func (r *recordingComponent) Copy(_ context.Context, w io.Writer) error     { return r.write(w) }
func (r *recordingComponent) Write(_ context.Context, w io.Writer) error    { return r.write(w) }
func (r *recordingComponent) Collect(_ context.Context) ([]ConfigFile, error) {
	if r.failErr != nil {
		return nil, r.failErr
	}
	return r.files, nil
}

func fixedClock(t time.Time) func() time.Time { return func() time.Time { return t } }

// tarContents reads the artifact tar and returns each entry as
// name -> bytes. Used to assert manifest + per-component layout.
func tarContents(t *testing.T, buf []byte) map[string][]byte {
	t.Helper()
	out := map[string][]byte{}
	r := tar.NewReader(bytes.NewReader(buf))
	for {
		hdr, err := r.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		body, err := io.ReadAll(r)
		if err != nil {
			t.Fatalf("tar read body %q: %v", hdr.Name, err)
		}
		out[hdr.Name] = body
	}
	return out
}

// sha256Hex helper so assertions read like the manifest.
func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func TestCreateBackup_Empty(t *testing.T) {
	m, err := NewBackupManager(WithClock(fixedClock(time.Unix(1700000000, 0).UTC())))
	if err != nil {
		t.Fatalf("NewBackupManager: %v", err)
	}
	var buf bytes.Buffer
	manifest, err := m.CreateBackup(context.Background(), &buf)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if got := len(manifest.Components); got != 0 {
		t.Errorf("components = %d, want 0", got)
	}
	if manifest.FormatVersion != ManifestFormatV1 {
		t.Errorf("FormatVersion = %d, want %d", manifest.FormatVersion, ManifestFormatV1)
	}

	entries := tarContents(t, buf.Bytes())
	if _, ok := entries[ManifestFilename]; !ok {
		t.Fatalf("manifest entry missing: have %v", keys(entries))
	}
	if len(entries) != 1 {
		t.Errorf("tar entries = %d, want 1: %v", len(entries), keys(entries))
	}
}

func TestCreateBackup_NilWriter(t *testing.T) {
	m, _ := NewBackupManager()
	if _, err := m.CreateBackup(context.Background(), nil); err == nil {
		t.Fatal("CreateBackup(nil): want error")
	}
}

func TestCreateBackup_AllComponents(t *testing.T) {
	storageBody := []byte("--- pg_dump output ---")
	jsBody := []byte("jetstream-snapshot-bytes")
	etcdBody := []byte("etcd-snapshot-bytes")
	secretsBody := []byte("encrypted-secrets-bytes")
	clusterBody := []byte("cluster-snapshot-bytes")
	configFiles := []ConfigFile{
		{Name: "kscore-server.yaml", Body: []byte("server: {}")},
		{Name: "kscore-agent.yaml", Body: []byte("agent: {}")},
	}
	clk := fixedClock(time.Unix(1700000000, 0).UTC())

	m, err := NewBackupManager(
		WithStorage(&recordingComponent{body: storageBody}),
		WithJetStream(&recordingComponent{body: jsBody}),
		WithEtcd(&recordingComponent{body: etcdBody}),
		WithConfig(&recordingComponent{files: configFiles}),
		WithSecrets(&recordingComponent{body: secretsBody}),
		WithCluster(&recordingComponent{body: clusterBody}),
		WithClock(clk),
		WithClusterName("test-cluster"),
	)
	if err != nil {
		t.Fatalf("NewBackupManager: %v", err)
	}

	var buf bytes.Buffer
	manifest, err := m.CreateBackup(context.Background(), &buf)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}

	// Manifest stamping.
	if manifest.ClusterName != "test-cluster" {
		t.Errorf("ClusterName = %q, want test-cluster", manifest.ClusterName)
	}
	if !manifest.TakenAt.Equal(clk()) {
		t.Errorf("TakenAt = %v, want %v", manifest.TakenAt, clk())
	}

	// One entry per component (+ one per config file, both named "config").
	wantNames := []string{"storage", "jetstream", "etcd", "config", "config", "secrets", "cluster"}
	gotNames := make([]string, 0, len(manifest.Components))
	for _, c := range manifest.Components {
		gotNames = append(gotNames, c.Name)
	}
	if !equalStrings(gotNames, wantNames) {
		t.Errorf("manifest order = %v, want %v", gotNames, wantNames)
	}

	// SHA-256 + size match the tar entries.
	entries := tarContents(t, buf.Bytes())
	for _, c := range manifest.Components {
		body, ok := entries[c.Path]
		if !ok {
			t.Errorf("tar missing %q", c.Path)
			continue
		}
		if int64(len(body)) != c.Size {
			t.Errorf("%s: tar size = %d, manifest = %d", c.Path, len(body), c.Size)
		}
		if got := sha256Hex(body); got != c.SHA256Hex {
			t.Errorf("%s: sha = %s, manifest = %s", c.Path, got, c.SHA256Hex)
		}
	}

	// Manifest entry actually carries the same JSON we returned.
	manifestEntry, ok := entries[ManifestFilename]
	if !ok {
		t.Fatal("manifest entry missing from tar")
	}
	var roundTrip Manifest
	if err := json.Unmarshal(manifestEntry, &roundTrip); err != nil {
		t.Fatalf("json.Unmarshal manifest: %v", err)
	}
	if len(roundTrip.Components) != len(manifest.Components) {
		t.Errorf("round-trip components = %d, want %d", len(roundTrip.Components), len(manifest.Components))
	}
}

func TestCreateBackup_PerComponentError(t *testing.T) {
	cases := []struct {
		name    string
		opt     Option
		wantSub string
	}{
		{name: "storage", opt: WithStorage(&recordingComponent{failErr: errors.New("disk")}), wantSub: "storage:"},
		{name: "jetstream", opt: WithJetStream(&recordingComponent{failErr: errors.New("nats")}), wantSub: "jetstream:"},
		{name: "etcd", opt: WithEtcd(&recordingComponent{failErr: errors.New("kv")}), wantSub: "etcd:"},
		{name: "config-collect", opt: WithConfig(&recordingComponent{failErr: errors.New("read")}), wantSub: "config collect:"},
		{name: "secrets", opt: WithSecrets(&recordingComponent{failErr: errors.New("seal")}), wantSub: "secrets:"},
		{name: "cluster", opt: WithCluster(&recordingComponent{failErr: errors.New("etcd-down")}), wantSub: "cluster:"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m, err := NewBackupManager(tc.opt)
			if err != nil {
				t.Fatalf("NewBackupManager: %v", err)
			}
			var buf bytes.Buffer
			_, err = m.CreateBackup(context.Background(), &buf)
			if err == nil {
				t.Fatal("CreateBackup: want error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %v, want substring %q", err, tc.wantSub)
			}
		})
	}
}

func TestCreateBackup_EmptyComponentBytes(t *testing.T) {
	m, err := NewBackupManager(
		WithStorage(&recordingComponent{body: nil}),
		WithClock(fixedClock(time.Unix(1700000000, 0).UTC())),
	)
	if err != nil {
		t.Fatalf("NewBackupManager: %v", err)
	}
	var buf bytes.Buffer
	manifest, err := m.CreateBackup(context.Background(), &buf)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if len(manifest.Components) != 1 {
		t.Fatalf("components = %d, want 1", len(manifest.Components))
	}
	c := manifest.Components[0]
	if c.Size != 0 {
		t.Errorf("Size = %d, want 0", c.Size)
	}
	// SHA-256 of the empty string is the well-known constant.
	const emptySHA = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if c.SHA256Hex != emptySHA {
		t.Errorf("SHA256Hex = %s, want %s", c.SHA256Hex, emptySHA)
	}
}

func TestCreateBackup_MultiFileConfig(t *testing.T) {
	files := []ConfigFile{
		{Name: "a.yaml", Body: []byte("A")},
		{Name: "b.yaml", Body: []byte("BB")},
		{Name: "c.yaml", Body: []byte("CCC")},
	}
	m, err := NewBackupManager(
		WithConfig(&recordingComponent{files: files}),
		WithClock(fixedClock(time.Unix(1700000000, 0).UTC())),
	)
	if err != nil {
		t.Fatalf("NewBackupManager: %v", err)
	}
	var buf bytes.Buffer
	manifest, err := m.CreateBackup(context.Background(), &buf)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if len(manifest.Components) != 3 {
		t.Fatalf("components = %d, want 3", len(manifest.Components))
	}
	wantPaths := []string{
		"components/config/a.yaml",
		"components/config/b.yaml",
		"components/config/c.yaml",
	}
	for i, c := range manifest.Components {
		if c.Name != "config" {
			t.Errorf("[%d].Name = %q, want config", i, c.Name)
		}
		if c.Path != wantPaths[i] {
			t.Errorf("[%d].Path = %q, want %q", i, c.Path, wantPaths[i])
		}
		if c.Size != int64(len(files[i].Body)) {
			t.Errorf("[%d].Size = %d, want %d", i, c.Size, len(files[i].Body))
		}
	}
}

func TestCreateBackup_ClockAndClusterName(t *testing.T) {
	when := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	m, err := NewBackupManager(
		WithClock(func() time.Time { return when }),
		WithClusterName("prod-east"),
	)
	if err != nil {
		t.Fatalf("NewBackupManager: %v", err)
	}
	var buf bytes.Buffer
	manifest, err := m.CreateBackup(context.Background(), &buf)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if !manifest.TakenAt.Equal(when) {
		t.Errorf("TakenAt = %v, want %v", manifest.TakenAt, when)
	}
	if manifest.ClusterName != "prod-east" {
		t.Errorf("ClusterName = %q, want prod-east", manifest.ClusterName)
	}
}

// TestCreateBackup_OptionGuards ensures nil-valued setters don't
// clobber the constructor defaults (clock, logger).
func TestCreateBackup_OptionGuards(t *testing.T) {
	m, err := NewBackupManager(WithClock(nil), WithLogger(nil))
	if err != nil {
		t.Fatalf("NewBackupManager: %v", err)
	}
	if m.opts.clock == nil {
		t.Error("clock left nil")
	}
	if m.opts.logger == nil {
		t.Error("logger left nil")
	}
}

func TestCreateBackup_LoggerOverride(t *testing.T) {
	var sink bytes.Buffer
	l := slog.New(slog.NewTextHandler(&sink, nil))
	m, err := NewBackupManager(WithLogger(l))
	if err != nil {
		t.Fatalf("NewBackupManager: %v", err)
	}
	var buf bytes.Buffer
	if _, err := m.CreateBackup(context.Background(), &buf); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if !strings.Contains(sink.String(), "artifact written") {
		t.Errorf("logger output missing expected message: %q", sink.String())
	}
}

func keys(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
