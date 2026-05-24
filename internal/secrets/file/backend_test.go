// SPDX-License-Identifier: Apache-2.0

package file

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/secrets"
)

func makeInlineKey(t *testing.T) string {
	t.Helper()
	bytes := make([]byte, KeyLen)
	for i := range bytes {
		bytes[i] = byte(i*7 + 3)
	}
	return "inline:" + hex.EncodeToString(bytes)
}

func newTestBackend(t *testing.T) (*Backend, string, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "secrets.bin")
	src := makeInlineKey(t)
	b, err := NewBackend(Config{
		Path:            path,
		MasterKeySource: src,
		EnsureParentDir: true,
	})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = b.Stop(context.Background()) })
	return b, path, src
}

func TestNewBackend_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     Config
		wantSub string
	}{
		{
			name:    "missing path",
			cfg:     Config{MasterKeySource: "inline:" + hex.EncodeToString(make([]byte, KeyLen))},
			wantSub: "Path is required",
		},
		{
			name:    "missing master key source",
			cfg:     Config{Path: "/tmp/x"},
			wantSub: "MasterKeySource is required",
		},
		{
			name:    "bad master key source",
			cfg:     Config{Path: "/tmp/x", MasterKeySource: "bogus"},
			wantSub: "missing scheme",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewBackend(tc.cfg)
			if err == nil {
				t.Fatalf("NewBackend = nil err, want %q", tc.wantSub)
			}
			if !errors.Is(err, secrets.ErrInvalidBackend) {
				t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestBackend_StartFreshFileCreatesIt(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.bin")
	src := makeInlineKey(t)

	b, err := NewBackend(Config{Path: path, MasterKeySource: src})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer b.Stop(context.Background())

	if _, statErr := os.Stat(path); statErr != nil {
		t.Errorf("Start did not create the state file: %v", statErr)
	}
}

func TestBackend_StartExistingFile_DecryptsAndPopulates(t *testing.T) {
	t.Parallel()

	b1, path, src := newTestBackend(t)

	_, err := b1.WriteSecret(context.Background(), secrets.WriteSecretRequest{
		Path: "kv/app/db",
		Data: map[string]any{"password": "hunter2"},
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	if err := b1.Stop(context.Background()); err != nil {
		t.Fatalf("Stop b1: %v", err)
	}

	// Fresh backend pointed at the same file + same key → should
	// decrypt the persisted state.
	b2, err := NewBackend(Config{Path: path, MasterKeySource: src})
	if err != nil {
		t.Fatalf("NewBackend b2: %v", err)
	}
	if err := b2.Start(context.Background()); err != nil {
		t.Fatalf("Start b2: %v", err)
	}
	defer b2.Stop(context.Background())

	got, err := b2.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "kv/app/db"})
	if err != nil {
		t.Fatalf("GetSecret after restart: %v", err)
	}
	if got.Data["password"] != "hunter2" {
		t.Errorf("persisted secret lost: %#v", got)
	}
}

func TestBackend_StartWithWrongKey(t *testing.T) {
	t.Parallel()

	b1, path, _ := newTestBackend(t)
	defer b1.Stop(context.Background())

	// Open with a different key.
	wrongKeyBytes := make([]byte, KeyLen)
	for i := range wrongKeyBytes {
		wrongKeyBytes[i] = 0xff
	}
	wrongSrc := "inline:" + hex.EncodeToString(wrongKeyBytes)

	b2, err := NewBackend(Config{Path: path, MasterKeySource: wrongSrc})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	err = b2.Start(context.Background())
	if err == nil {
		t.Fatalf("Start with wrong key = nil err")
	}
	if !errors.Is(err, errEnvelopeKeyMismatch) {
		t.Errorf("err does not wrap errEnvelopeKeyMismatch: %v", err)
	}
}

func TestBackend_StartCleansUpStaleTemp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "with-tmp.bin")
	src := makeInlineKey(t)

	// Drop a leftover tmp file as if a previous write crashed.
	if err := os.WriteFile(path+tempSuffix, []byte("crashed"), 0600); err != nil {
		t.Fatalf("WriteFile tmp: %v", err)
	}

	b, err := NewBackend(Config{Path: path, MasterKeySource: src})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer b.Stop(context.Background())

	if _, statErr := os.Stat(path + tempSuffix); !errors.Is(statErr, os.ErrNotExist) {
		t.Errorf("stale tmp not cleaned up at Start: %v", statErr)
	}
}

func TestBackend_DoubleStart_Rejected(t *testing.T) {
	t.Parallel()

	b, _, _ := newTestBackend(t)
	if err := b.Start(context.Background()); err == nil {
		t.Errorf("double Start = nil err")
	}
}

func TestBackend_StopIsIdempotent(t *testing.T) {
	t.Parallel()

	b, _, _ := newTestBackend(t)
	if err := b.Stop(context.Background()); err != nil {
		t.Fatalf("first Stop: %v", err)
	}
	if err := b.Stop(context.Background()); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestBackend_StartAfterStop_Rejected(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	if err := b.Stop(context.Background()); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := b.Start(context.Background()); err == nil {
		t.Errorf("Start after Stop = nil err")
	}
}

func TestBackend_Health(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "h.bin")
	b, err := NewBackend(Config{Path: path, MasterKeySource: makeInlineKey(t)})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Health(context.Background()); !errors.Is(err, secrets.ErrBackendNotStarted) {
		t.Errorf("Health pre-Start = %v, want ErrBackendNotStarted", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := b.Health(context.Background()); err != nil {
		t.Errorf("Health post-Start = %v, want nil", err)
	}
	_ = b.Stop(context.Background())
	if err := b.Health(context.Background()); !errors.Is(err, secrets.ErrBackendNotStarted) {
		t.Errorf("Health post-Stop = %v, want ErrBackendNotStarted", err)
	}
}

func TestBackend_NameAndCapabilities(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "c.bin")
	b, err := NewBackend(Config{Path: path, MasterKeySource: makeInlineKey(t)})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if b.Name() != DefaultBackendName {
		t.Errorf("Name() = %q, want %q", b.Name(), DefaultBackendName)
	}
	caps := b.Capabilities()
	if !secrets.HasCapability(caps, secrets.CapKV) {
		t.Errorf("missing CapKV")
	}
	if !secrets.HasCapability(caps, secrets.CapList) {
		t.Errorf("missing CapList")
	}
	for _, denied := range []secrets.BackendCapability{secrets.CapDynamic, secrets.CapTransit, secrets.CapLeaseRenew, secrets.CapLeaseRevoke} {
		if secrets.HasCapability(caps, denied) {
			t.Errorf("unexpected capability advertised: %s", denied)
		}
	}

	// Custom name overrides.
	b2, err := NewBackend(Config{Path: path, MasterKeySource: makeInlineKey(t), Name: "kv-prod"})
	if err != nil {
		t.Fatalf("NewBackend custom name: %v", err)
	}
	if b2.Name() != "kv-prod" {
		t.Errorf("custom Name() = %q, want %q", b2.Name(), "kv-prod")
	}
}

func TestBackend_CRUD_RoundTrip(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	ctx := context.Background()

	written, err := b.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path:     "kv/app/db",
		Data:     map[string]any{"password": "hunter2", "user": "alice"},
		Metadata: map[string]string{"owner": "platform"},
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	if written.Version != 1 {
		t.Errorf("initial Version = %d, want 1", written.Version)
	}
	if written.CreatedAt.IsZero() || written.UpdatedAt.IsZero() {
		t.Errorf("timestamps missing: %#v", written)
	}

	got, err := b.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/app/db"})
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got.Data["password"] != "hunter2" {
		t.Errorf("Get returned wrong password: %v", got.Data["password"])
	}
	if got.Metadata["owner"] != "platform" {
		t.Errorf("Get lost metadata: %#v", got.Metadata)
	}

	// Mutate-clone independence.
	got.Data["password"] = "MUTATED"
	again, _ := b.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/app/db"})
	if again.Data["password"] != "hunter2" {
		t.Errorf("clone defense breached: caller mutated backend state")
	}

	// Update bumps version.
	updated, err := b.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/app/db",
		Data: map[string]any{"password": "newpw"},
	})
	if err != nil {
		t.Fatalf("WriteSecret update: %v", err)
	}
	if updated.Version != 2 {
		t.Errorf("updated Version = %d, want 2", updated.Version)
	}
	if !updated.UpdatedAt.After(written.UpdatedAt) && !updated.UpdatedAt.Equal(written.UpdatedAt) {
		t.Errorf("UpdatedAt regressed: %v -> %v", written.UpdatedAt, updated.UpdatedAt)
	}

	// Delete.
	if err := b.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: "kv/app/db"}); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	_, err = b.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/app/db"})
	if !errors.Is(err, secrets.ErrSecretNotFound) {
		t.Errorf("Get after Delete err = %v, want ErrSecretNotFound", err)
	}
}

func TestBackend_GetSecret_NotFound(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	_, err := b.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "kv/missing"})
	if !errors.Is(err, secrets.ErrSecretNotFound) {
		t.Errorf("err = %v, want ErrSecretNotFound", err)
	}
}

func TestBackend_GetSecret_VersionMismatch(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	ctx := context.Background()
	_, _ = b.WriteSecret(ctx, secrets.WriteSecretRequest{Path: "kv/v", Data: map[string]any{"x": "y"}})

	_, err := b.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/v", Version: 99})
	if !errors.Is(err, secrets.ErrSecretNotFound) {
		t.Errorf("err = %v, want ErrSecretNotFound on version mismatch", err)
	}

	// Matching version returns the secret.
	got, err := b.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/v", Version: 1})
	if err != nil {
		t.Errorf("matching-version Get = %v, want nil", err)
	}
	if got.Version != 1 {
		t.Errorf("Version = %d, want 1", got.Version)
	}
}

func TestBackend_DeleteSecret_NotFound(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	err := b.DeleteSecret(context.Background(), secrets.DeleteSecretRequest{Path: "kv/missing"})
	if !errors.Is(err, secrets.ErrSecretNotFound) {
		t.Errorf("err = %v, want ErrSecretNotFound", err)
	}
}

func TestBackend_WriteSecret_CAS(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	ctx := context.Background()

	// CAS=0 against an absent path: succeeds.
	cas0 := uint64(0)
	_, err := b.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/cas",
		Data: map[string]any{"v": 1},
		CAS:  &cas0,
	})
	if err != nil {
		t.Fatalf("CAS=0 against absent: %v", err)
	}

	// CAS=0 against a present version 1: rejected.
	_, err = b.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/cas",
		Data: map[string]any{"v": 2},
		CAS:  &cas0,
	})
	if err == nil || !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("stale CAS = %v, want ErrInvalidBackend wrap", err)
	}
	if !strings.Contains(err.Error(), "CAS mismatch") {
		t.Errorf("err = %q, want CAS-mismatch context", err.Error())
	}

	// CAS=1 against version 1: succeeds.
	cas1 := uint64(1)
	updated, err := b.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/cas",
		Data: map[string]any{"v": 2},
		CAS:  &cas1,
	})
	if err != nil {
		t.Fatalf("matching CAS: %v", err)
	}
	if updated.Version != 2 {
		t.Errorf("Version after matching CAS = %d, want 2", updated.Version)
	}
}

func TestBackend_DeleteSecret_CAS(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	ctx := context.Background()

	_, _ = b.WriteSecret(ctx, secrets.WriteSecretRequest{Path: "kv/cas-del", Data: map[string]any{"x": 1}})

	// Wrong version → rejected.
	err := b.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: "kv/cas-del", Version: 99})
	if err == nil || !errors.Is(err, secrets.ErrInvalidBackend) {
		t.Errorf("CAS mismatch delete err = %v, want ErrInvalidBackend", err)
	}

	// Matching version → succeeds.
	if err := b.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: "kv/cas-del", Version: 1}); err != nil {
		t.Errorf("matching-version delete: %v", err)
	}
}

func TestBackend_ListSecrets(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	ctx := context.Background()

	for _, p := range []string{"kv/app/db", "kv/app/cache", "kv/web/api", "other/x"} {
		if _, err := b.WriteSecret(ctx, secrets.WriteSecretRequest{
			Path:     p,
			Data:     map[string]any{"k": "v"},
			Metadata: map[string]string{"src": p},
		}); err != nil {
			t.Fatalf("WriteSecret %q: %v", p, err)
		}
	}

	resp, err := b.ListSecrets(ctx, secrets.ListSecretsRequest{Prefix: "kv/"})
	if err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if got, want := len(resp.Entries), 3; got != want {
		t.Fatalf("kv/ entries = %d, want %d (%v)", got, want, resp.Entries)
	}
	for _, e := range resp.Entries {
		if !strings.HasPrefix(e.Path, "kv/") {
			t.Errorf("entry not under prefix: %s", e.Path)
		}
		if e.Metadata["src"] != e.Path {
			t.Errorf("metadata round-trip lost: %v", e.Metadata)
		}
		if e.Version == 0 {
			t.Errorf("Version not propagated: %v", e)
		}
	}

	// Pagination: limit 2 → cursor → limit 1 picks up remainder.
	page1, err := b.ListSecrets(ctx, secrets.ListSecretsRequest{Prefix: "kv/", Limit: 2})
	if err != nil {
		t.Fatalf("page1: %v", err)
	}
	if len(page1.Entries) != 2 {
		t.Errorf("page1 entries = %d, want 2", len(page1.Entries))
	}
	if page1.NextCursor == "" {
		t.Errorf("page1.NextCursor empty when limit hit")
	}

	page2, err := b.ListSecrets(ctx, secrets.ListSecretsRequest{Prefix: "kv/", Limit: 2, Cursor: page1.NextCursor})
	if err != nil {
		t.Fatalf("page2: %v", err)
	}
	if len(page2.Entries) != 1 {
		t.Errorf("page2 entries = %d, want 1", len(page2.Entries))
	}
}

func TestBackend_UnsupportedOps(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	ctx := context.Background()

	cases := []struct {
		name string
		fn   func() error
		cap  secrets.BackendCapability
	}{
		{
			name: "IssueDynamicSecret",
			fn: func() error {
				_, err := b.IssueDynamicSecret(ctx, secrets.IssueDynamicSecretRequest{Path: "kv/x"})
				return err
			},
			cap: secrets.CapDynamic,
		},
		{
			name: "RenewLease",
			fn: func() error {
				_, err := b.RenewLease(ctx, secrets.RenewLeaseRequest{LeaseID: "x"})
				return err
			},
			cap: secrets.CapLeaseRenew,
		},
		{
			name: "RevokeLease",
			fn:   func() error { return b.RevokeLease(ctx, secrets.RevokeLeaseRequest{LeaseID: "x"}) },
			cap:  secrets.CapLeaseRevoke,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := tc.fn()
			if err == nil {
				t.Fatalf("%s = nil err, want capability rejection", tc.name)
			}
			if !errors.Is(err, secrets.ErrInvalidBackend) {
				t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
			}
			if !strings.Contains(err.Error(), tc.cap.String()) {
				t.Errorf("err = %q, want capability name %s", err.Error(), tc.cap)
			}
		})
	}
}

func TestBackend_ConcurrentReadsAndWrites(t *testing.T) {
	t.Parallel()
	b, _, _ := newTestBackend(t)
	ctx := context.Background()

	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		i := i
		wg.Add(2)
		go func() {
			defer wg.Done()
			path := fmt.Sprintf("kv/parallel/%d", i)
			_, err := b.WriteSecret(ctx, secrets.WriteSecretRequest{
				Path: path,
				Data: map[string]any{"k": i},
			})
			if err != nil {
				t.Errorf("concurrent WriteSecret: %v", err)
			}
		}()
		go func() {
			defer wg.Done()
			_, _ = b.GetSecret(ctx, secrets.GetSecretRequest{Path: fmt.Sprintf("kv/parallel/%d", (i+25)%n)})
		}()
	}
	wg.Wait()
}

func TestBackend_PersistFailureRollsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("filesystem permission semantics differ on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("read-only directory bypassed by root (CAP_DAC_OVERRIDE); test requires non-root uid")
	}
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "rollback.bin")
	b, err := NewBackend(Config{Path: path, MasterKeySource: makeInlineKey(t), EnsureParentDir: true})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer b.Stop(context.Background())

	// First write succeeds.
	_, err = b.WriteSecret(context.Background(), secrets.WriteSecretRequest{
		Path: "kv/keep",
		Data: map[string]any{"original": "yes"},
	})
	if err != nil {
		t.Fatalf("first WriteSecret: %v", err)
	}

	// Make the directory read-only so the next write fails.
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("Chmod ro: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	_, err = b.WriteSecret(context.Background(), secrets.WriteSecretRequest{
		Path: "kv/keep",
		Data: map[string]any{"clobbered": "yes"},
	})
	if err == nil {
		t.Fatalf("expected write failure on read-only dir")
	}

	// In-memory state must NOT show the clobber.
	got, err := b.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "kv/keep"})
	if err != nil {
		t.Fatalf("GetSecret post-rollback: %v", err)
	}
	if got.Data["original"] != "yes" || got.Data["clobbered"] != nil {
		t.Errorf("rollback failed: %#v", got.Data)
	}
}

func TestBackend_PersistFailure_DeleteRollsBack(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("filesystem permission semantics differ on windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("read-only directory bypassed by root (CAP_DAC_OVERRIDE); test requires non-root uid")
	}
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "del-rollback.bin")
	b, err := NewBackend(Config{Path: path, MasterKeySource: makeInlineKey(t), EnsureParentDir: true})
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if err := b.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer b.Stop(context.Background())

	if _, err := b.WriteSecret(context.Background(), secrets.WriteSecretRequest{Path: "kv/keep", Data: map[string]any{"x": "y"}}); err != nil {
		t.Fatalf("seed write: %v", err)
	}
	if err := os.Chmod(dir, 0500); err != nil {
		t.Fatalf("Chmod ro: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0700) })

	if err := b.DeleteSecret(context.Background(), secrets.DeleteSecretRequest{Path: "kv/keep"}); err == nil {
		t.Fatalf("expected delete failure on read-only dir")
	}
	// State must still hold the entry.
	if _, err := b.GetSecret(context.Background(), secrets.GetSecretRequest{Path: "kv/keep"}); err != nil {
		t.Errorf("rolled-back delete should leave entry: err = %v", err)
	}
}

// End-to-end: build a real broker against the file backend and
// exercise the full chain.
func TestBackend_BrokerIntegration(t *testing.T) {
	t.Parallel()

	b, _, _ := newTestBackend(t)

	auditor := &fakeAuditor{}
	router, err := secrets.NewRouter([]secrets.Route{{Prefix: "kv/", Backend: DefaultBackendName}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	broker, err := secrets.NewBroker(secrets.BrokerConfig{
		Router:         router,
		Backends:       []secrets.SecretBackend{b},
		DefaultBackend: DefaultBackendName,
		Auditor:        auditor,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}

	ctx := context.Background()
	_, err = broker.WriteSecret(ctx, secrets.WriteSecretRequest{
		Path: "kv/app/db",
		Data: map[string]any{"password": "hunter2"},
	})
	if err != nil {
		t.Fatalf("broker.WriteSecret: %v", err)
	}

	got, err := broker.GetSecret(ctx, secrets.GetSecretRequest{Path: "kv/app/db"})
	if err != nil {
		t.Fatalf("broker.GetSecret: %v", err)
	}
	if got.Data["password"] != "hunter2" {
		t.Errorf("round-trip lost password: %#v", got)
	}

	if err := broker.DeleteSecret(ctx, secrets.DeleteSecretRequest{Path: "kv/app/db"}); err != nil {
		t.Fatalf("broker.DeleteSecret: %v", err)
	}

	// Audit fired on every op.
	if got, want := auditor.count(), 3; got != want {
		t.Errorf("auditor count = %d, want %d", got, want)
	}
}

// fakeAuditor records events for the broker integration test.
type fakeAuditor struct {
	mu  sync.Mutex
	evs []secrets.SecretAccessEvent
}

func (a *fakeAuditor) Emit(_ context.Context, e secrets.SecretAccessEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.evs = append(a.evs, e)
}

func (a *fakeAuditor) count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.evs)
}

// Ensure unused symbols don't drift.
var _ = time.Second
