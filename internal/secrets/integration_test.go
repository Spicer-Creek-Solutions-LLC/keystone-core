//go:build integration

// End-to-end integration test that walks every PROJECT-DETAILS
// §4.11 / epics/10-secrets.md acceptance criterion against an
// integrated stack: real file backend + real broker + real
// SecretCache + in-memory LeaseDirectory (no scheduler running in
// this test rig — the scheduler-side acceptance is pinned in
// internal/secrets/lease_manager_test.go) + recording auditor.
//
// Uses the external `_test` package so we can import
// internal/secrets/file (which itself imports internal/secrets) —
// avoids the cycle that an internal-package test would create.
//
// Run via `go test -tags integration ./internal/secrets/...`; the
// default `make test` skips it.
package secrets_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/secrets/file"
)

// integrationRig wires the live components used by every sub-test.
type integrationRig struct {
	broker       *secrets.Broker
	fileBackend  *file.Backend
	fakeBackend  *recordingBackend
	cache        *secrets.SecretCache
	auditor      *recordingIntegrationAuditor
	leases       *secrets.InMemoryLeaseDirectory
	statePath    string
}

func newIntegrationRig(t *testing.T) *integrationRig {
	t.Helper()

	statePath := filepath.Join(t.TempDir(), "secrets.bin")
	// Inline 32-byte key (operator config would resolve env: or file:).
	masterKey := "inline:" + strings.Repeat("ab", 32)

	fb, err := file.NewBackend(file.Config{
		Path:            statePath,
		MasterKeySource: masterKey,
		Name:            "file",
		EnsureParentDir: true,
	})
	if err != nil {
		t.Fatalf("file.NewBackend: %v", err)
	}

	fakeBE := &recordingBackend{
		name: "fake",
		caps: []secrets.BackendCapability{secrets.CapKV, secrets.CapList},
		store: make(map[string]*secrets.Secret),
	}

	router, err := secrets.NewRouter([]secrets.Route{
		{Prefix: "kv/", Backend: "file"},
		{Prefix: "secret/", Backend: "fake"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	cache, err := secrets.NewSecretCache(secrets.SecretCacheConfig{})
	if err != nil {
		t.Fatalf("NewSecretCache: %v", err)
	}

	auditor := &recordingIntegrationAuditor{}
	leases := secrets.NewInMemoryLeaseDirectory()

	broker, err := secrets.NewBroker(secrets.BrokerConfig{
		Router:         router,
		Backends:       []secrets.SecretBackend{fb, fakeBE},
		DefaultBackend: "file",
		Cache:          cache,
		Auditor:        auditor,
		LeaseDirectory: leases,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	if err := broker.Start(context.Background()); err != nil {
		t.Fatalf("broker.Start: %v", err)
	}
	t.Cleanup(func() { _ = broker.Stop(context.Background()) })

	return &integrationRig{
		broker:      broker,
		fileBackend: fb,
		fakeBackend: fakeBE,
		cache:       cache,
		auditor:     auditor,
		leases:      leases,
		statePath:   statePath,
	}
}

// TestEpic10_Integration is the close-out test for Epic 10. Each
// sub-test maps 1:1 to an acceptance criterion in
// epics/10-secrets.md — operators auditing v1.0 readiness can
// grep "TestEpic10_Integration/" to see the full mapping.
func TestEpic10_Integration(t *testing.T) {
	t.Run("RoundTrip_FileBackend", testE2EFileBackendRoundTrip)
	t.Run("PathPrefixRouting", testE2EPathPrefixRouting)
	t.Run("CacheHitRate_Above80Pct", testE2ECacheHitRate)
	t.Run("CacheInvalidatesOnDelete", testE2ECacheInvalidatesOnDelete)
	t.Run("AuditFiresOnEveryOp_WithMasking", testE2EAuditFires)
	t.Run("AuditFailuresAlwaysEmit_WithSampling", testE2EAuditFailuresUnsampled)
	t.Run("FileBackend_AESGCM_AtRest", testE2EFileBackendAESGCM)
}

func testE2EFileBackendRoundTrip(t *testing.T) {
	rig := newIntegrationRig(t)
	ctx := context.Background()
	const path = "kv/app/db"

	// Write.
	written, err := rig.broker.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: path,
		Data: map[string]any{"password": "hunter2", "user": "alice"},
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	if written.Version != 1 {
		t.Errorf("Version after write = %d, want 1", written.Version)
	}

	// Get — through the cache by default; force a backend read with Refresh.
	got, err := rig.broker.GetSecret(ctx, secrets.GetSecretRequest{Path: path, Refresh: true})
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Data["password"] != "hunter2" {
		t.Errorf("round-trip lost password: %#v", got.Data)
	}
	if got.Data["user"] != "alice" {
		t.Errorf("round-trip lost user: %#v", got.Data)
	}

	// Delete.
	if err := rig.broker.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: path}); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	// Subsequent Get returns secrets.ErrSecretNotFound.
	_, err = rig.broker.GetSecret(ctx, secrets.GetSecretRequest{Path: path, Refresh: true})
	if err == nil {
		t.Errorf("Get after Delete returned nil err, want secrets.ErrSecretNotFound")
	}
}

func testE2EPathPrefixRouting(t *testing.T) {
	rig := newIntegrationRig(t)
	ctx := context.Background()

	// kv/ routes to file backend.
	if _, err := rig.broker.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/x", Data: map[string]any{"k": "v"},
	}); err != nil {
		t.Fatalf("WriteSecret kv/x: %v", err)
	}

	// secret/ routes to fake backend.
	if _, err := rig.broker.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "secret/y", Data: map[string]any{"k": "v"},
	}); err != nil {
		t.Fatalf("WriteSecret secret/y: %v", err)
	}

	// Verify routing went to the right place.
	if _, err := rig.fileBackend.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/x"}); err != nil {
		t.Errorf("kv/x not in file backend: %v", err)
	}
	rig.fakeBackend.mu.Lock()
	_, secretYInFake := rig.fakeBackend.store["secret/y"]
	rig.fakeBackend.mu.Unlock()
	if !secretYInFake {
		t.Errorf("secret/y not in fake backend")
	}
}

func testE2ECacheHitRate(t *testing.T) {
	rig := newIntegrationRig(t)
	ctx := context.Background()

	// Seed via the file backend so the broker dispatch path mirrors
	// the v1.0 read flow.
	if _, err := rig.broker.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/hot", Data: map[string]any{"k": "v"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}

	// Reset cache + auditor to isolate this measurement.
	rig.cache.Clear()
	rig.auditor.reset()

	// 10 sequential Gets. First → cache miss + backend dispatch;
	// subsequent → cache hits.
	const n = 10
	for i := 0; i < n; i++ {
		if _, err := rig.broker.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/hot"}); err != nil {
			t.Fatalf("GetSecret[%d]: %v", i, err)
		}
	}

	stats := rig.cache.Stats()
	total := stats.Hits + stats.Misses
	hitRate := float64(stats.Hits) / float64(total)
	if hitRate < 0.8 {
		t.Errorf("cache hit rate = %.2f over %d reads, want > 0.8 (§4.11 acceptance)", hitRate, n)
	}
}

func testE2ECacheInvalidatesOnDelete(t *testing.T) {
	rig := newIntegrationRig(t)
	ctx := context.Background()

	if _, err := rig.broker.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/x", Data: map[string]any{"k": "v"},
	}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// Warm the cache.
	if _, err := rig.broker.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/x"}); err != nil {
		t.Fatalf("warm: %v", err)
	}
	if _, hit := rig.cache.Get("kv/x"); !hit {
		t.Fatalf("cache did not warm")
	}

	if err := rig.broker.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: "kv/x"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, hit := rig.cache.Get("kv/x"); hit {
		t.Errorf("cache still has entry after Delete (§4.11 invalidation invariant)")
	}
}

func testE2EAuditFires(t *testing.T) {
	rig := newIntegrationRig(t)
	ctx := context.Background()

	rig.auditor.reset()

	_, err := rig.broker.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/audited", Data: map[string]any{"password": "hunter2"},
	})
	if err != nil {
		t.Fatalf("Write: %v", err)
	}
	if _, err := rig.broker.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/audited", Refresh: true}); err != nil {
		t.Fatalf("Get: %v", err)
	}
	if err := rig.broker.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: "kv/audited"}); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	events := rig.auditor.snapshot()
	if got := len(events); got != 3 {
		t.Fatalf("audit events = %d, want 3 (Write + Get + Delete)", got)
	}
	for i, evt := range events {
		if !evt.Allowed {
			t.Errorf("event[%d].Allowed = false: %#v", i, evt)
		}
		// Cleartext must never appear in MaskedPayload.
		for k, v := range evt.MaskedPayload {
			if s, ok := v.(string); ok && s == "hunter2" {
				t.Errorf("event[%d].MaskedPayload leaked cleartext at key %q", i, k)
			}
		}
	}
	// The Write event MUST have MaskedPayload populated (per task 3's contract).
	writeEvt := events[0]
	if writeEvt.Action != secrets.ActionWriteSecret {
		t.Errorf("event[0].Action = %q, want %q", writeEvt.Action, secrets.ActionWriteSecret)
	}
	if writeEvt.MaskedPayload["password"] != secrets.MaskedValue {
		t.Errorf("Write event MaskedPayload[password] = %v, want %q", writeEvt.MaskedPayload["password"], secrets.MaskedValue)
	}
}

// testE2EAuditFailuresUnsampled wraps the rig's auditor in a
// SamplingAuditor(0.0) and asserts that failures still emit — the
// §4.11 "failure to log = bug" carve-out from task 11.
func testE2EAuditFailuresUnsampled(t *testing.T) {
	rig := newIntegrationRig(t)
	ctx := context.Background()

	// Swap the rig's auditor for a SamplingAuditor(0.0) wrapping
	// the recording one. Direct broker mutation isn't supported, so
	// we build a fresh broker pointed at the same backends with the
	// sampling auditor wired in.
	recording := &recordingIntegrationAuditor{}
	sampling, err := secrets.NewSamplingAuditor(recording, 0.0)
	if err != nil {
		t.Fatalf("NewSamplingAuditor: %v", err)
	}
	router, _ := secrets.NewRouter([]secrets.Route{{Prefix: "kv/", Backend: "file"}})
	cache, _ := secrets.NewSecretCache(secrets.SecretCacheConfig{})
	broker, err := secrets.NewBroker(secrets.BrokerConfig{
		Router:         router,
		Backends:       []secrets.SecretBackend{rig.fileBackend},
		DefaultBackend: "file",
		Cache:          cache,
		Auditor:        sampling,
		LeaseDirectory: secrets.NewInMemoryLeaseDirectory(),
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	defer func() { _ = broker.Stop(ctx) }()

	// 5 successful Writes — sampling drops them all.
	for i := 0; i < 5; i++ {
		if _, err := broker.WriteSecret(ctx, secrets.WriteSecretRequest{
			Path: fmt.Sprintf("kv/ok-%d", i), Data: map[string]any{"k": "v"},
		}); err != nil {
			t.Fatalf("Write[%d]: %v", i, err)
		}
	}
	// 5 failing Gets — sampling MUST emit every one.
	for i := 0; i < 5; i++ {
		_, _ = broker.GetSecret(ctx, secrets.GetSecretRequest{Path: fmt.Sprintf("kv/never-%d", i)})
	}

	events := recording.snapshot()
	if got := len(events); got != 5 {
		t.Fatalf("with sampling=0.0: got %d events, want 5 (5 failures pass through; 5 successes dropped)", got)
	}
	for i, evt := range events {
		if evt.Allowed {
			t.Errorf("event[%d].Allowed = true; sampling=0.0 should have dropped successes", i)
		}
	}
}

// testE2EFileBackendAESGCM inspects the on-disk state file and
// asserts it carries the AES-GCM envelope magic — proving the
// at-rest encryption invariant.
func testE2EFileBackendAESGCM(t *testing.T) {
	rig := newIntegrationRig(t)
	ctx := context.Background()

	if _, err := rig.broker.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/x", Data: map[string]any{"password": "hunter2"},
	}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	raw := readFile(t, rig.statePath)
	// The file backend's on-disk envelope starts with the
	// `KSCORE-SECRETS\0\0` magic (16 bytes; see internal/secrets/file/storage.go).
	const wantPrefix = "KSCORE-SECRETS"
	if !bytes.HasPrefix(raw, []byte(wantPrefix)) {
		t.Errorf("state file does not start with expected magic; got first 16 bytes = %x", raw[:min(16, len(raw))])
	}

	// And the cleartext MUST NOT appear in the file — AES-GCM is
	// confidential by construction; this catches an accidental
	// regression that bypassed the encrypt step.
	if bytes.Contains(raw, []byte("hunter2")) {
		t.Errorf("state file contains cleartext 'hunter2' — at-rest encryption broken")
	}
}

// ---- helpers + fakes -------------------------------------------

type recordingBackend struct {
	mu    sync.Mutex
	name  string
	caps  []secrets.BackendCapability
	store map[string]*secrets.Secret
}

func (b *recordingBackend) Name() string                      { return b.name }
func (b *recordingBackend) Capabilities() []secrets.BackendCapability { return b.caps }
func (b *recordingBackend) Start(context.Context) error       { return nil }
func (b *recordingBackend) Stop(context.Context) error        { return nil }
func (b *recordingBackend) Health(context.Context) error      { return nil }

func (b *recordingBackend) GetSecret(_ context.Context, req secrets.GetSecretRequest) (*secrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s, ok := b.store[req.Path]
	if !ok {
		return nil, fmt.Errorf("%w: %q", secrets.ErrSecretNotFound, req.Path)
	}
	return s, nil
}

func (b *recordingBackend) WriteSecret(_ context.Context, req secrets.WriteSecretRequest) (*secrets.Secret, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	s := &secrets.Secret{Path: req.Path, Data: req.Data, Metadata: req.Metadata, Version: 1}
	b.store[req.Path] = s
	return s, nil
}

func (b *recordingBackend) ListSecrets(_ context.Context, _ secrets.ListSecretsRequest) (*secrets.ListSecretsResponse, error) {
	return &secrets.ListSecretsResponse{}, nil
}

func (b *recordingBackend) DeleteSecret(_ context.Context, req secrets.DeleteSecretRequest) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.store, req.Path)
	return nil
}

func (b *recordingBackend) IssueDynamicSecret(context.Context, secrets.IssueDynamicSecretRequest) (*secrets.Secret, error) {
	return nil, secrets.ErrNotImplementedYet
}
func (b *recordingBackend) RenewLease(context.Context, secrets.RenewLeaseRequest) (*secrets.LeaseInfo, error) {
	return nil, secrets.ErrNotImplementedYet
}
func (b *recordingBackend) RevokeLease(context.Context, secrets.RevokeLeaseRequest) error {
	return secrets.ErrNotImplementedYet
}

type recordingIntegrationAuditor struct {
	mu     sync.Mutex
	events []secrets.SecretAccessEvent
}

func (a *recordingIntegrationAuditor) Emit(_ context.Context, e secrets.SecretAccessEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, e)
}

func (a *recordingIntegrationAuditor) snapshot() []secrets.SecretAccessEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]secrets.SecretAccessEvent, len(a.events))
	copy(out, a.events)
	return out
}

func (a *recordingIntegrationAuditor) reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = nil
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	raw, err := os.ReadFile(path) // #nosec G304 -- test-controlled temp path
	if err != nil {
		t.Fatalf("read %q: %v", path, err)
	}
	return raw
}
