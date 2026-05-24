// SPDX-License-Identifier: Apache-2.0

package backup

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"testing"
	"time"
)

// recordingRestoreHandler captures what the orchestrator passed to
// each restore method so the round-trip tests can assert byte-equal
// pass-through. The same struct satisfies every restore interface
// just like recordingComponent on the backup side.
type recordingRestoreHandler struct {
	gotStorage   []byte
	gotJetStream []byte
	gotEtcd      []byte
	gotSecrets   []byte
	gotCluster   []byte
	gotConfig    []ConfigFile

	failAt  string
	failErr error
}

func (h *recordingRestoreHandler) maybeFail(name string) error {
	if h.failAt == name && h.failErr != nil {
		return h.failErr
	}
	return nil
}

func (h *recordingRestoreHandler) Restore(ctx context.Context, r io.Reader) error {
	// Not used: each component has a dedicated typed method below.
	return errors.New("unreachable")
}

// Method specialization — Go interface matching is structural, but
// keeping six named methods + the dispatch routing keeps the test
// double clear.
type storageRestoreFn struct{ h *recordingRestoreHandler }

func (s *storageRestoreFn) Restore(_ context.Context, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.h.gotStorage = b
	return s.h.maybeFail("storage")
}

type jetStreamRestoreFn struct{ h *recordingRestoreHandler }

func (j *jetStreamRestoreFn) Restore(_ context.Context, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	j.h.gotJetStream = b
	return j.h.maybeFail("jetstream")
}

type etcdRestoreFn struct{ h *recordingRestoreHandler }

func (e *etcdRestoreFn) Restore(_ context.Context, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	e.h.gotEtcd = b
	return e.h.maybeFail("etcd")
}

type configRestoreFn struct{ h *recordingRestoreHandler }

func (c *configRestoreFn) Restore(_ context.Context, files []ConfigFile) error {
	c.h.gotConfig = files
	return c.h.maybeFail("config")
}

type secretsRestoreFn struct{ h *recordingRestoreHandler }

func (s *secretsRestoreFn) Restore(_ context.Context, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	s.h.gotSecrets = b
	return s.h.maybeFail("secrets")
}

type clusterRestoreFn struct{ h *recordingRestoreHandler }

func (c *clusterRestoreFn) Restore(_ context.Context, r io.Reader) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	c.h.gotCluster = b
	return c.h.maybeFail("cluster")
}

type stubDetector struct {
	populated bool
	err       error
}

func (s stubDetector) IsPopulated(_ context.Context) (bool, error) {
	return s.populated, s.err
}

// allHandlers wires every recording restore handler to a fresh
// RestoreManager.
func allHandlers(t *testing.T, h *recordingRestoreHandler, extra ...RestoreOption) *RestoreManager {
	t.Helper()
	opts := []RestoreOption{
		WithStorageRestore(&storageRestoreFn{h: h}),
		WithJetStreamRestore(&jetStreamRestoreFn{h: h}),
		WithEtcdRestore(&etcdRestoreFn{h: h}),
		WithConfigRestore(&configRestoreFn{h: h}),
		WithSecretsRestore(&secretsRestoreFn{h: h}),
		WithClusterRestore(&clusterRestoreFn{h: h}),
	}
	opts = append(opts, extra...)
	rm, err := NewRestoreManager(opts...)
	if err != nil {
		t.Fatalf("NewRestoreManager: %v", err)
	}
	return rm
}

// buildArtifact uses the real BackupManager so the test exercises
// the production tar layout and SHA-256 logic end-to-end.
func buildArtifact(t *testing.T, payloads map[string][]byte, configFiles []ConfigFile) []byte {
	t.Helper()
	opts := []Option{
		WithClock(func() time.Time { return time.Unix(1700000000, 0).UTC() }),
		WithClusterName("test-cluster"),
	}
	if b, ok := payloads["storage"]; ok {
		opts = append(opts, WithStorage(&recordingComponent{body: b}))
	}
	if b, ok := payloads["jetstream"]; ok {
		opts = append(opts, WithJetStream(&recordingComponent{body: b}))
	}
	if b, ok := payloads["etcd"]; ok {
		opts = append(opts, WithEtcd(&recordingComponent{body: b}))
	}
	if configFiles != nil {
		opts = append(opts, WithConfig(&recordingComponent{files: configFiles}))
	}
	if b, ok := payloads["secrets"]; ok {
		opts = append(opts, WithSecrets(&recordingComponent{body: b}))
	}
	if b, ok := payloads["cluster"]; ok {
		opts = append(opts, WithCluster(&recordingComponent{body: b}))
	}

	mgr, err := NewBackupManager(opts...)
	if err != nil {
		t.Fatalf("NewBackupManager: %v", err)
	}
	var buf bytes.Buffer
	if _, err := mgr.CreateBackup(context.Background(), &buf); err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	return buf.Bytes()
}

func TestRestore_NilReader(t *testing.T) {
	rm, _ := NewRestoreManager()
	if _, err := rm.Restore(context.Background(), nil, RestoreOptions{}); err == nil {
		t.Fatal("want error")
	}
}

func TestRestore_FullRoundTrip(t *testing.T) {
	payloads := map[string][]byte{
		"storage":   []byte("pg-dump-bytes"),
		"jetstream": []byte("jsm-bytes"),
		"etcd":      []byte("etcd-bytes"),
		"secrets":   []byte("secrets-bytes"),
		"cluster":   []byte("cluster-bytes"),
	}
	configFiles := []ConfigFile{
		{Name: "server.yaml", Body: []byte("server: {}")},
		{Name: "agent.yaml", Body: []byte("agent: {}")},
	}
	artifact := buildArtifact(t, payloads, configFiles)

	h := &recordingRestoreHandler{}
	rm := allHandlers(t, h)

	manifest, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{Selection: SelectAll()})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if got, want := manifest.ClusterName, "test-cluster"; got != want {
		t.Errorf("ClusterName = %q, want %q", got, want)
	}
	if !bytes.Equal(h.gotStorage, payloads["storage"]) {
		t.Errorf("storage mismatch")
	}
	if !bytes.Equal(h.gotJetStream, payloads["jetstream"]) {
		t.Errorf("jetstream mismatch")
	}
	if !bytes.Equal(h.gotEtcd, payloads["etcd"]) {
		t.Errorf("etcd mismatch")
	}
	if !bytes.Equal(h.gotSecrets, payloads["secrets"]) {
		t.Errorf("secrets mismatch")
	}
	if !bytes.Equal(h.gotCluster, payloads["cluster"]) {
		t.Errorf("cluster mismatch")
	}
	if len(h.gotConfig) != 2 {
		t.Fatalf("config files = %d, want 2", len(h.gotConfig))
	}
	if h.gotConfig[0].Name != "server.yaml" || string(h.gotConfig[0].Body) != "server: {}" {
		t.Errorf("config[0] = %+v", h.gotConfig[0])
	}
	if h.gotConfig[1].Name != "agent.yaml" || string(h.gotConfig[1].Body) != "agent: {}" {
		t.Errorf("config[1] = %+v", h.gotConfig[1])
	}
}

func TestRestore_PartialConfigOnly(t *testing.T) {
	payloads := map[string][]byte{
		"storage":   []byte("ignore-me"),
		"jetstream": []byte("ignore-me"),
	}
	configFiles := []ConfigFile{{Name: "server.yaml", Body: []byte("server: {}")}}
	artifact := buildArtifact(t, payloads, configFiles)

	h := &recordingRestoreHandler{}
	rm := allHandlers(t, h)

	if _, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{Selection: SelectConfigOnly()}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if h.gotStorage != nil {
		t.Errorf("storage handler invoked: %q", h.gotStorage)
	}
	if h.gotJetStream != nil {
		t.Errorf("jetstream handler invoked: %q", h.gotJetStream)
	}
	if len(h.gotConfig) != 1 {
		t.Errorf("config files = %d, want 1", len(h.gotConfig))
	}
}

func TestRestore_PartialSecretsOnly(t *testing.T) {
	payloads := map[string][]byte{
		"storage": []byte("ignore"),
		"secrets": []byte("the-secret"),
	}
	artifact := buildArtifact(t, payloads, nil)

	h := &recordingRestoreHandler{}
	rm := allHandlers(t, h)

	if _, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{Selection: SelectSecretsOnly()}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if h.gotStorage != nil {
		t.Errorf("storage handler invoked")
	}
	if !bytes.Equal(h.gotSecrets, []byte("the-secret")) {
		t.Errorf("secrets = %q", h.gotSecrets)
	}
}

func TestRestore_EmptyArtifact(t *testing.T) {
	// BackupManager with no components writes a manifest-only tar.
	artifact := buildArtifact(t, nil, nil)
	h := &recordingRestoreHandler{}
	rm := allHandlers(t, h)
	manifest, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{Selection: SelectAll()})
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if len(manifest.Components) != 0 {
		t.Errorf("components = %d, want 0", len(manifest.Components))
	}
	if h.gotStorage != nil || h.gotConfig != nil {
		t.Errorf("handlers should not have been invoked")
	}
}

func TestRestore_IntegrityCorruptEntry(t *testing.T) {
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("original")}, nil)
	// Mutate one byte in the storage entry. The tar header carries
	// the size, so flipping a byte changes the SHA-256 without
	// breaking the tar structure.
	mutated := mutateTarEntry(t, artifact, "components/storage.bin", func(b []byte) []byte {
		out := make([]byte, len(b))
		copy(out, b)
		out[0] ^= 0xFF
		return out
	})

	rm := allHandlers(t, &recordingRestoreHandler{})
	_, err := rm.Restore(context.Background(), bytes.NewReader(mutated), RestoreOptions{Selection: SelectAll()})
	if !errors.Is(err, ErrIntegrityCheckFailed) {
		t.Fatalf("err = %v, want ErrIntegrityCheckFailed", err)
	}
}

func TestRestore_IntegrityMissingEntry(t *testing.T) {
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("present")}, nil)
	mutated := dropTarEntry(t, artifact, "components/storage.bin")

	rm := allHandlers(t, &recordingRestoreHandler{})
	_, err := rm.Restore(context.Background(), bytes.NewReader(mutated), RestoreOptions{Selection: SelectAll()})
	if !errors.Is(err, ErrIntegrityCheckFailed) {
		t.Fatalf("err = %v, want ErrIntegrityCheckFailed", err)
	}
}

func TestRestore_IntegrityExtraEntry(t *testing.T) {
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("present")}, nil)
	mutated := appendTarEntry(t, artifact, "components/bonus.bin", []byte("not-in-manifest"))

	rm := allHandlers(t, &recordingRestoreHandler{})
	_, err := rm.Restore(context.Background(), bytes.NewReader(mutated), RestoreOptions{Selection: SelectAll()})
	if !errors.Is(err, ErrIntegrityCheckFailed) {
		t.Fatalf("err = %v, want ErrIntegrityCheckFailed", err)
	}
}

func TestRestore_SchemaIncompatible(t *testing.T) {
	artifact := buildArtifact(t, nil, nil)
	mutated := rewriteManifest(t, artifact, func(m *Manifest) {
		m.FormatVersion = 999
	})

	rm := allHandlers(t, &recordingRestoreHandler{})
	_, err := rm.Restore(context.Background(), bytes.NewReader(mutated), RestoreOptions{Selection: SelectAll()})
	if !errors.Is(err, ErrSchemaIncompatible) {
		t.Fatalf("err = %v, want ErrSchemaIncompatible", err)
	}
}

func TestRestore_ManifestMissing(t *testing.T) {
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("x")}, nil)
	mutated := dropTarEntry(t, artifact, ManifestFilename)

	rm := allHandlers(t, &recordingRestoreHandler{})
	_, err := rm.Restore(context.Background(), bytes.NewReader(mutated), RestoreOptions{Selection: SelectAll()})
	if !errors.Is(err, ErrManifestMissing) {
		t.Fatalf("err = %v, want ErrManifestMissing", err)
	}
}

func TestRestore_PopulatedClusterRefused(t *testing.T) {
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("x")}, nil)
	rm := allHandlers(t, &recordingRestoreHandler{}, WithClusterDetector(stubDetector{populated: true}))

	_, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{Selection: SelectAll()})
	if !errors.Is(err, ErrClusterPopulated) {
		t.Fatalf("err = %v, want ErrClusterPopulated", err)
	}
}

func TestRestore_PopulatedClusterForce(t *testing.T) {
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("x")}, nil)
	h := &recordingRestoreHandler{}
	rm := allHandlers(t, h, WithClusterDetector(stubDetector{populated: true}))

	if _, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{
		Force:     true,
		Selection: SelectAll(),
	}); err != nil {
		t.Fatalf("Restore with Force: %v", err)
	}
	if !bytes.Equal(h.gotStorage, []byte("x")) {
		t.Errorf("storage = %q, want x", h.gotStorage)
	}
}

func TestRestore_DetectorError(t *testing.T) {
	artifact := buildArtifact(t, nil, nil)
	want := errors.New("etcd down")
	rm := allHandlers(t, &recordingRestoreHandler{}, WithClusterDetector(stubDetector{err: want}))

	_, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{Selection: SelectAll()})
	if !errors.Is(err, want) {
		t.Fatalf("err = %v, want errors.Is(%v)", err, want)
	}
}

func TestRestore_NoDetector(t *testing.T) {
	// With no detector wired, populated check is skipped — restore
	// proceeds even on what would otherwise be a populated target.
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("x")}, nil)
	h := &recordingRestoreHandler{}
	rm := allHandlers(t, h)

	if _, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{Selection: SelectAll()}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !bytes.Equal(h.gotStorage, []byte("x")) {
		t.Errorf("storage = %q", h.gotStorage)
	}
}

func TestRestore_HandlerError(t *testing.T) {
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("x")}, nil)
	wantErr := errors.New("storage restore broken")
	h := &recordingRestoreHandler{failAt: "storage", failErr: wantErr}
	rm := allHandlers(t, h)

	_, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{Selection: SelectAll()})
	if !errors.Is(err, wantErr) {
		t.Fatalf("err = %v, want errors.Is(%v)", err, wantErr)
	}
}

func TestRestore_SelectionAbsentIsSilentSkip(t *testing.T) {
	// Backup has no etcd; Selection asks for both Storage and Etcd.
	// Etcd has nothing to restore so it is silently skipped; Storage
	// runs normally. Selection is an "interest list", not a strict
	// expectation — the manifest is authoritative on what's present.
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("x")}, nil)
	h := &recordingRestoreHandler{}
	rm := allHandlers(t, h)

	if _, err := rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{
		Selection: Selection{Storage: true, Etcd: true},
	}); err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if !bytes.Equal(h.gotStorage, []byte("x")) {
		t.Errorf("storage = %q, want x", h.gotStorage)
	}
	if h.gotEtcd != nil {
		t.Errorf("etcd handler invoked despite absent component")
	}
}

func TestRestore_SelectionUnwiredHandler(t *testing.T) {
	// Backup has storage; request restore of storage; but no
	// StorageRestore wired.
	artifact := buildArtifact(t, map[string][]byte{"storage": []byte("x")}, nil)
	rm, err := NewRestoreManager()
	if err != nil {
		t.Fatalf("NewRestoreManager: %v", err)
	}
	_, err = rm.Restore(context.Background(), bytes.NewReader(artifact), RestoreOptions{
		Selection: Selection{Storage: true},
	})
	if err == nil {
		t.Fatal("want error for unwired handler")
	}
}

// ---- tar mutation helpers ----

func mutateTarEntry(t *testing.T, artifact []byte, name string, mutate func([]byte) []byte) []byte {
	t.Helper()
	return rewriteTar(t, artifact, func(hdr *tar.Header, body []byte) (*tar.Header, []byte, bool) {
		if hdr.Name == name {
			body = mutate(body)
			hdr.Size = int64(len(body))
		}
		return hdr, body, true
	}, nil)
}

func dropTarEntry(t *testing.T, artifact []byte, name string) []byte {
	t.Helper()
	return rewriteTar(t, artifact, func(hdr *tar.Header, body []byte) (*tar.Header, []byte, bool) {
		if hdr.Name == name {
			return hdr, body, false
		}
		return hdr, body, true
	}, nil)
}

func appendTarEntry(t *testing.T, artifact []byte, name string, body []byte) []byte {
	t.Helper()
	extra := []extraEntry{{name: name, body: body}}
	return rewriteTar(t, artifact, func(hdr *tar.Header, b []byte) (*tar.Header, []byte, bool) {
		return hdr, b, true
	}, extra)
}

func rewriteManifest(t *testing.T, artifact []byte, mutate func(*Manifest)) []byte {
	t.Helper()
	return rewriteTar(t, artifact, func(hdr *tar.Header, body []byte) (*tar.Header, []byte, bool) {
		if hdr.Name == ManifestFilename {
			var m Manifest
			if err := json.Unmarshal(body, &m); err != nil {
				t.Fatalf("unmarshal manifest: %v", err)
			}
			mutate(&m)
			rewritten, err := json.MarshalIndent(&m, "", "  ")
			if err != nil {
				t.Fatalf("marshal manifest: %v", err)
			}
			hdr.Size = int64(len(rewritten))
			return hdr, rewritten, true
		}
		return hdr, body, true
	}, nil)
}

type extraEntry struct {
	name string
	body []byte
}

func rewriteTar(t *testing.T, in []byte, transform func(*tar.Header, []byte) (*tar.Header, []byte, bool), extras []extraEntry) []byte {
	t.Helper()
	tr := tar.NewReader(bytes.NewReader(in))
	var out bytes.Buffer
	tw := tar.NewWriter(&out)
	// We need to preserve order: components first, manifest LAST so
	// extras get inserted before manifest (mimics what a tamperer
	// could do).
	type buffered struct {
		hdr  tar.Header
		body []byte
	}
	var list []buffered
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("tar next: %v", err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		newHdr, newBody, keep := transform(hdr, body)
		if !keep {
			continue
		}
		list = append(list, buffered{hdr: *newHdr, body: newBody})
	}
	// Insert extras just before the manifest entry (which is last).
	finalList := make([]buffered, 0, len(list)+len(extras))
	for i, b := range list {
		if i == len(list)-1 && b.hdr.Name == ManifestFilename {
			for _, e := range extras {
				finalList = append(finalList, buffered{
					hdr:  tar.Header{Name: e.name, Mode: 0o600, Size: int64(len(e.body))},
					body: e.body,
				})
			}
		}
		finalList = append(finalList, b)
	}
	for _, b := range finalList {
		hdr := b.hdr
		if err := tw.WriteHeader(&hdr); err != nil {
			t.Fatalf("write header: %v", err)
		}
		if _, err := tw.Write(b.body); err != nil {
			t.Fatalf("write body: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}
	return out.Bytes()
}

// TestSelectionHelpers exercises the canned selection constructors.
func TestSelectionHelpers(t *testing.T) {
	if !SelectAll().any() || !SelectConfigOnly().any() || !SelectSecretsOnly().any() {
		t.Error("canned selections all non-empty")
	}
	if (Selection{}).any() {
		t.Error("zero Selection should not be any()")
	}
	if !SelectAll().has("storage") || !SelectAll().has("cluster") {
		t.Error("SelectAll missing components")
	}
	if SelectConfigOnly().has("storage") {
		t.Error("SelectConfigOnly should not include storage")
	}
	if (Selection{}).has("xyz") {
		t.Error("Selection.has unknown name should be false")
	}
}

func TestWithRestoreLogger_NilIgnored(t *testing.T) {
	rm, err := NewRestoreManager(WithRestoreLogger(nil))
	if err != nil {
		t.Fatalf("NewRestoreManager: %v", err)
	}
	if rm.opts.logger == nil {
		t.Error("logger left nil")
	}
}
