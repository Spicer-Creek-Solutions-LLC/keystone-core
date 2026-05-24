// SPDX-License-Identifier: Apache-2.0

package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeBackend is the shared test stub the broker dispatches against.
// Records every call so tests can assert routing + capability paths.
// Per-method "return this" hooks let a test inject errors / responses.
type fakeBackend struct {
	mu sync.Mutex

	name string
	caps []BackendCapability

	startErr  error
	stopErr   error
	healthErr error

	// Recorded inbound calls per method.
	getCalls    []GetSecretRequest
	writeCalls  []WriteSecretRequest
	listCalls   []ListSecretsRequest
	deleteCalls []DeleteSecretRequest
	issueCalls  []IssueDynamicSecretRequest
	renewCalls  []RenewLeaseRequest
	revokeCalls []RevokeLeaseRequest

	// Hooks. nil → default-shaped response.
	getFn    func(GetSecretRequest) (*Secret, error)
	writeFn  func(WriteSecretRequest) (*Secret, error)
	listFn   func(ListSecretsRequest) (*ListSecretsResponse, error)
	deleteFn func(DeleteSecretRequest) error
	issueFn  func(IssueDynamicSecretRequest) (*Secret, error)
	renewFn  func(RenewLeaseRequest) (*LeaseInfo, error)
	revokeFn func(RevokeLeaseRequest) error

	startCount int
	stopCount  int
}

func (b *fakeBackend) Name() string                      { return b.name }
func (b *fakeBackend) Capabilities() []BackendCapability { return b.caps }

func (b *fakeBackend) Start(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.startCount++
	return b.startErr
}

func (b *fakeBackend) Stop(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.stopCount++
	return b.stopErr
}

func (b *fakeBackend) Health(_ context.Context) error { return b.healthErr }

func (b *fakeBackend) GetSecret(_ context.Context, req GetSecretRequest) (*Secret, error) {
	b.mu.Lock()
	b.getCalls = append(b.getCalls, req)
	fn := b.getFn
	b.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return &Secret{Path: req.Path, Data: map[string]any{"k": "v"}}, nil
}

func (b *fakeBackend) WriteSecret(_ context.Context, req WriteSecretRequest) (*Secret, error) {
	b.mu.Lock()
	b.writeCalls = append(b.writeCalls, req)
	fn := b.writeFn
	b.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return &Secret{Path: req.Path, Data: req.Data, Version: 1}, nil
}

func (b *fakeBackend) ListSecrets(_ context.Context, req ListSecretsRequest) (*ListSecretsResponse, error) {
	b.mu.Lock()
	b.listCalls = append(b.listCalls, req)
	fn := b.listFn
	b.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return &ListSecretsResponse{}, nil
}

func (b *fakeBackend) DeleteSecret(_ context.Context, req DeleteSecretRequest) error {
	b.mu.Lock()
	b.deleteCalls = append(b.deleteCalls, req)
	fn := b.deleteFn
	b.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return nil
}

func (b *fakeBackend) IssueDynamicSecret(_ context.Context, req IssueDynamicSecretRequest) (*Secret, error) {
	b.mu.Lock()
	b.issueCalls = append(b.issueCalls, req)
	fn := b.issueFn
	b.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return &Secret{
		Path:          req.Path,
		Data:          map[string]any{"password": "dyn"},
		LeaseID:       "lease-" + b.name + "-1",
		LeaseDuration: 30 * time.Minute,
		Renewable:     true,
	}, nil
}

func (b *fakeBackend) RenewLease(_ context.Context, req RenewLeaseRequest) (*LeaseInfo, error) {
	b.mu.Lock()
	b.renewCalls = append(b.renewCalls, req)
	fn := b.renewFn
	b.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return &LeaseInfo{ID: req.LeaseID, State: LeaseStateActive}, nil
}

func (b *fakeBackend) RevokeLease(_ context.Context, req RevokeLeaseRequest) error {
	b.mu.Lock()
	b.revokeCalls = append(b.revokeCalls, req)
	fn := b.revokeFn
	b.mu.Unlock()
	if fn != nil {
		return fn(req)
	}
	return nil
}

// recordingAuditor captures every event so tests can assert content + count.
type recordingAuditor struct {
	mu     sync.Mutex
	events []SecretAccessEvent
}

func (a *recordingAuditor) Emit(_ context.Context, evt SecretAccessEvent) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.events = append(a.events, evt)
}

func (a *recordingAuditor) Last() SecretAccessEvent {
	a.mu.Lock()
	defer a.mu.Unlock()
	if len(a.events) == 0 {
		return SecretAccessEvent{}
	}
	return a.events[len(a.events)-1]
}

func (a *recordingAuditor) Count() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.events)
}

// recordingCache is a real cache that records ops.
type recordingCache struct {
	mu     sync.Mutex
	store  map[string]*Secret
	hits   int
	misses int
	puts   int
	invalP int
	invalX int
}

func newRecordingCache() *recordingCache { return &recordingCache{store: make(map[string]*Secret)} }

func (c *recordingCache) Get(path string) (*Secret, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	s, ok := c.store[path]
	if ok {
		c.hits++
	} else {
		c.misses++
	}
	return s, ok
}

func (c *recordingCache) Put(path string, s *Secret) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.puts++
	c.store[path] = s
}

func (c *recordingCache) InvalidatePath(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalP++
	delete(c.store, path)
}

func (c *recordingCache) InvalidatePrefix(prefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invalX++
	for k := range c.store {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			delete(c.store, k)
		}
	}
}

func (c *recordingCache) Stats() CacheStats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return CacheStats{Hits: uint64(c.hits), Misses: uint64(c.misses), Entries: len(c.store)}
}

// Helper: build a broker with two backends + a router that sends
// `kv/` → file, `secret/` → vault, default = file.
type brokerFixture struct {
	broker  *Broker
	file    *fakeBackend
	vault   *fakeBackend
	auditor *recordingAuditor
	cache   *recordingCache
}

func newFixture(t *testing.T) brokerFixture {
	t.Helper()

	file := &fakeBackend{name: "file", caps: []BackendCapability{CapKV, CapList}}
	vault := &fakeBackend{name: "vault", caps: []BackendCapability{CapKV, CapList, CapDynamic, CapLeaseRenew, CapLeaseRevoke, CapTransit}}

	router, err := NewRouter([]Route{
		{Prefix: "kv/", Backend: "file"},
		{Prefix: "secret/", Backend: "vault"},
		{Prefix: "database/", Backend: "vault"},
	})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}

	auditor := &recordingAuditor{}
	cache := newRecordingCache()

	b, err := NewBroker(BrokerConfig{
		Router:         router,
		Backends:       []SecretBackend{file, vault},
		DefaultBackend: "file",
		Cache:          cache,
		Auditor:        auditor,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}
	return brokerFixture{broker: b, file: file, vault: vault, auditor: auditor, cache: cache}
}

// ---- Construction validation ----

func TestNewBroker_Validation(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		cfg     BrokerConfig
		wantSub string
	}{
		{
			name:    "no backends",
			cfg:     BrokerConfig{},
			wantSub: "at least one backend is required",
		},
		{
			name: "nil backend",
			cfg: BrokerConfig{
				Backends: []SecretBackend{nil},
			},
			wantSub: "backends[0] is nil",
		},
		{
			name: "empty backend name",
			cfg: BrokerConfig{
				Backends: []SecretBackend{&fakeBackend{name: ""}},
			},
			wantSub: "empty Name()",
		},
		{
			name: "duplicate backend names",
			cfg: BrokerConfig{
				Backends: []SecretBackend{
					&fakeBackend{name: "file"},
					&fakeBackend{name: "file"},
				},
			},
			wantSub: "duplicate backend name",
		},
		{
			name: "default_backend not registered",
			cfg: BrokerConfig{
				Backends:       []SecretBackend{&fakeBackend{name: "file"}},
				DefaultBackend: "nope",
			},
			wantSub: `default_backend "nope" does not name a registered backend`,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := NewBroker(tc.cfg)
			if err == nil {
				t.Fatalf("NewBroker = nil err, want %q", tc.wantSub)
			}
			if !errors.Is(err, ErrInvalidBackend) {
				t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("err = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestNewBroker_RouteToUnknownBackend(t *testing.T) {
	t.Parallel()

	router, err := NewRouter([]Route{{Prefix: "kv/", Backend: "ghost"}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	_, err = NewBroker(BrokerConfig{
		Router:   router,
		Backends: []SecretBackend{&fakeBackend{name: "file", caps: []BackendCapability{CapKV}}},
	})
	if err == nil {
		t.Fatalf("NewBroker = nil err, want unknown backend rejection")
	}
	if !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
	if !strings.Contains(err.Error(), `unknown backend "ghost"`) {
		t.Errorf("err = %q, want substring about unknown backend", err.Error())
	}
}

func TestNewBroker_NilRouterOK(t *testing.T) {
	t.Parallel()

	file := &fakeBackend{name: "file", caps: []BackendCapability{CapKV}}
	_, err := NewBroker(BrokerConfig{
		Backends:       []SecretBackend{file},
		DefaultBackend: "file",
		// no router
	})
	if err != nil {
		t.Fatalf("NewBroker with nil router: %v", err)
	}
}

// ---- Routing dispatch ----

func TestBroker_GetSecret_Routes(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)

	if _, err := fx.broker.GetSecret(context.Background(), GetSecretRequest{Path: "kv/foo"}); err != nil {
		t.Fatalf("GetSecret(kv/foo): %v", err)
	}
	if got := len(fx.file.getCalls); got != 1 {
		t.Errorf("file backend got %d calls, want 1", got)
	}
	if got := len(fx.vault.getCalls); got != 0 {
		t.Errorf("vault backend got %d calls, want 0", got)
	}

	if _, err := fx.broker.GetSecret(context.Background(), GetSecretRequest{Path: "secret/baz"}); err != nil {
		t.Fatalf("GetSecret(secret/baz): %v", err)
	}
	if got := len(fx.vault.getCalls); got != 1 {
		t.Errorf("vault backend got %d calls, want 1", got)
	}
}

func TestBroker_DefaultBackendFallback(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)

	// Path with no matching prefix → falls back to default (file).
	if _, err := fx.broker.GetSecret(context.Background(), GetSecretRequest{Path: "uncategorized/foo"}); err != nil {
		t.Fatalf("GetSecret(no-match): %v", err)
	}
	if got := len(fx.file.getCalls); got != 1 {
		t.Errorf("default file backend got %d calls, want 1", got)
	}
	last := fx.auditor.Last()
	if last.Backend != "file" {
		t.Errorf("audit backend = %q, want file (default)", last.Backend)
	}
	if !last.Allowed {
		t.Errorf("audit allowed = false: %#v", last)
	}
}

func TestBroker_NoRouteNoDefault_FiresAuditOnEveryMethod(t *testing.T) {
	t.Parallel()

	// Build a router that won't match anything and a broker with no
	// default — every dispatch should reject with ErrInvalidBackend
	// AND emit one audit event per call.
	router, _ := NewRouter([]Route{{Prefix: "kv/", Backend: "file"}})
	file := &fakeBackend{name: "file", caps: []BackendCapability{CapKV, CapList, CapDynamic, CapLeaseRenew, CapLeaseRevoke}}
	auditor := &recordingAuditor{}
	b, err := NewBroker(BrokerConfig{
		Router:   router,
		Backends: []SecretBackend{file},
		Auditor:  auditor,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}

	ctx := context.Background()
	calls := []func() error{
		func() error { _, err := b.GetSecret(ctx, GetSecretRequest{Path: "unrouted/a", Refresh: true}); return err },
		func() error {
			_, err := b.WriteSecret(ctx, WriteSecretRequest{Path: "unrouted/b", Data: map[string]any{"x": "y"}})
			return err
		},
		func() error { _, err := b.ListSecrets(ctx, ListSecretsRequest{Prefix: "unrouted/"}); return err },
		func() error { return b.DeleteSecret(ctx, DeleteSecretRequest{Path: "unrouted/c"}) },
		func() error {
			_, err := b.IssueDynamicSecret(ctx, IssueDynamicSecretRequest{Path: "unrouted/d"})
			return err
		},
	}

	for i, call := range calls {
		err := call()
		if err == nil {
			t.Errorf("call[%d] returned nil err on unrouted path", i)
			continue
		}
		if !errors.Is(err, ErrInvalidBackend) {
			t.Errorf("call[%d] err = %v, want wraps ErrInvalidBackend", i, err)
		}
	}
	if got, want := auditor.Count(), len(calls); got != want {
		t.Errorf("audit Count = %d, want %d (failure to log = bug per §4.11)", got, want)
	}
	for i, evt := range auditor.events {
		if evt.Allowed {
			t.Errorf("audit[%d].Allowed = true on unrouted path", i)
		}
	}
	// Backend should never have been dispatched to.
	if len(file.getCalls)+len(file.writeCalls)+len(file.listCalls)+len(file.deleteCalls)+len(file.issueCalls) != 0 {
		t.Errorf("backend dispatched despite unrouted paths")
	}
}

func TestBroker_NoRouteNoDefault_Rejects(t *testing.T) {
	t.Parallel()

	router, err := NewRouter([]Route{{Prefix: "kv/", Backend: "file"}})
	if err != nil {
		t.Fatalf("NewRouter: %v", err)
	}
	file := &fakeBackend{name: "file", caps: []BackendCapability{CapKV}}
	auditor := &recordingAuditor{}
	b, err := NewBroker(BrokerConfig{
		Router:   router,
		Backends: []SecretBackend{file},
		Auditor:  auditor,
		// no DefaultBackend
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}

	_, err = b.GetSecret(context.Background(), GetSecretRequest{Path: "unrouted/foo"})
	if err == nil {
		t.Fatalf("GetSecret(unrouted) = nil err, want ErrInvalidBackend wrap")
	}
	if !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
	if got := len(file.getCalls); got != 0 {
		t.Errorf("backend dispatched on no-route: %d calls", got)
	}
	// Audit MUST still fire (failure to log = bug per §4.11).
	if auditor.Count() != 1 {
		t.Errorf("audit Count = %d, want 1 (failure event)", auditor.Count())
	}
	if auditor.Last().Allowed {
		t.Errorf("audit Allowed = true on rejection")
	}
}

// ---- Capability refusal ----

func TestBroker_CapabilityRefusal(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	// file backend lacks CapDynamic.
	_, err := fx.broker.IssueDynamicSecret(context.Background(), IssueDynamicSecretRequest{Path: "kv/db"})
	if err == nil {
		t.Fatalf("IssueDynamicSecret on cap-less backend = nil err, want rejection")
	}
	if !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
	if !strings.Contains(err.Error(), "does not support capability dynamic") {
		t.Errorf("err = %q, want capability message", err.Error())
	}
	if got := len(fx.file.issueCalls); got != 0 {
		t.Errorf("backend dispatched on cap refusal: %d calls", got)
	}
	last := fx.auditor.Last()
	if last.Allowed {
		t.Errorf("audit Allowed = true on cap refusal")
	}
	if last.Backend != "file" {
		t.Errorf("audit backend = %q, want file", last.Backend)
	}
}

// ---- Cache layer ----

func TestBroker_GetSecret_CacheHit(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	// Pre-populate cache.
	fx.cache.Put("kv/cached", &Secret{Path: "kv/cached", Data: map[string]any{"hit": "yes"}})

	got, err := fx.broker.GetSecret(context.Background(), GetSecretRequest{Path: "kv/cached"})
	if err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if got == nil || got.Data["hit"] != "yes" {
		t.Errorf("cache hit returned %#v, want cached value", got)
	}
	if calls := len(fx.file.getCalls); calls != 0 {
		t.Errorf("backend dispatched on cache hit: %d calls", calls)
	}
	last := fx.auditor.Last()
	if last.Backend != CacheBackendLabel {
		t.Errorf("audit backend = %q, want %q", last.Backend, CacheBackendLabel)
	}
}

func TestBroker_GetSecret_CacheMissThenPopulate(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)

	// First call → miss → backend dispatch → cache populated.
	if _, err := fx.broker.GetSecret(context.Background(), GetSecretRequest{Path: "kv/cold"}); err != nil {
		t.Fatalf("first GetSecret: %v", err)
	}
	if calls := len(fx.file.getCalls); calls != 1 {
		t.Fatalf("first dispatch count = %d, want 1", calls)
	}

	// Second call → cache hit; backend count unchanged.
	if _, err := fx.broker.GetSecret(context.Background(), GetSecretRequest{Path: "kv/cold"}); err != nil {
		t.Fatalf("second GetSecret: %v", err)
	}
	if calls := len(fx.file.getCalls); calls != 1 {
		t.Errorf("second dispatch count = %d, want 1 (cache hit expected)", calls)
	}
}

func TestBroker_GetSecret_RefreshBypassesCache(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	fx.cache.Put("kv/foo", &Secret{Path: "kv/foo", Data: map[string]any{"stale": true}})

	_, err := fx.broker.GetSecret(context.Background(), GetSecretRequest{Path: "kv/foo", Refresh: true})
	if err != nil {
		t.Fatalf("GetSecret(Refresh): %v", err)
	}
	if calls := len(fx.file.getCalls); calls != 1 {
		t.Errorf("Refresh did not bypass cache: %d dispatch calls", calls)
	}
}

func TestBroker_WriteSecret_InvalidatesCache(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	fx.cache.Put("kv/foo", &Secret{Path: "kv/foo"})

	_, err := fx.broker.WriteSecret(context.Background(), WriteSecretRequest{
		Path: "kv/foo",
		Data: map[string]any{"new": "val"},
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	if _, hit := fx.cache.Get("kv/foo"); hit {
		t.Errorf("cache not invalidated after WriteSecret")
	}
}

func TestBroker_DeleteSecret_InvalidatesCache(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	fx.cache.Put("kv/foo", &Secret{Path: "kv/foo"})

	if err := fx.broker.DeleteSecret(context.Background(), DeleteSecretRequest{Path: "kv/foo"}); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}
	if _, hit := fx.cache.Get("kv/foo"); hit {
		t.Errorf("cache not invalidated after DeleteSecret")
	}
}

// ---- Dynamic secret + lease directory ----

func TestBroker_IssueDynamicSecret_RecordsLease(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	dir := NewInMemoryLeaseDirectory()
	// Rebuild fixture broker with our directory.
	router, _ := NewRouter([]Route{
		{Prefix: "database/", Backend: "vault"},
	})
	b, err := NewBroker(BrokerConfig{
		Router:         router,
		Backends:       []SecretBackend{fx.file, fx.vault},
		DefaultBackend: "vault",
		LeaseDirectory: dir,
		Auditor:        fx.auditor,
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}

	secret, err := b.IssueDynamicSecret(context.Background(), IssueDynamicSecretRequest{
		Path: "database/creds/app",
		Role: "app",
	})
	if err != nil {
		t.Fatalf("IssueDynamicSecret: %v", err)
	}
	if secret.LeaseID == "" {
		t.Fatalf("backend returned no LeaseID")
	}
	if dir.Len() != 1 {
		t.Errorf("LeaseDirectory not populated: Len=%d", dir.Len())
	}
	rec, ok := dir.Lookup(secret.LeaseID)
	if !ok {
		t.Fatalf("Lookup(%q) missed", secret.LeaseID)
	}
	if rec.Backend != "vault" {
		t.Errorf("recorded backend = %q, want vault", rec.Backend)
	}
	if rec.Path != "database/creds/app" {
		t.Errorf("recorded path = %q, want database/creds/app", rec.Path)
	}
}

func TestBroker_RenewLease_RoutesToBackend(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	dir := NewInMemoryLeaseDirectory()
	dir.Record("lease-xyz", LeaseRecord{Backend: "vault", Path: "database/creds/app"})

	router, _ := NewRouter([]Route{{Prefix: "database/", Backend: "vault"}})
	b, _ := NewBroker(BrokerConfig{
		Router:         router,
		Backends:       []SecretBackend{fx.file, fx.vault},
		LeaseDirectory: dir,
		Auditor:        fx.auditor,
	})

	info, err := b.RenewLease(context.Background(), RenewLeaseRequest{LeaseID: "lease-xyz"})
	if err != nil {
		t.Fatalf("RenewLease: %v", err)
	}
	if info == nil || info.ID != "lease-xyz" {
		t.Errorf("RenewLease returned %#v, want lease-xyz", info)
	}
	if len(fx.vault.renewCalls) != 1 {
		t.Errorf("vault.RenewLease dispatch count = %d, want 1", len(fx.vault.renewCalls))
	}
}

func TestBroker_RenewLease_UnknownID(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)

	_, err := fx.broker.RenewLease(context.Background(), RenewLeaseRequest{LeaseID: "never-existed"})
	if err == nil {
		t.Fatalf("RenewLease(unknown) = nil err, want ErrLeaseNotFound wrap")
	}
	if !errors.Is(err, ErrLeaseNotFound) {
		t.Errorf("err does not wrap ErrLeaseNotFound: %v", err)
	}
	last := fx.auditor.Last()
	if last.Allowed {
		t.Errorf("audit Allowed = true on unknown lease")
	}
}

func TestBroker_RevokeLease_ForgetsAndInvalidatesCache(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	dir := NewInMemoryLeaseDirectory()
	dir.Record("lease-revokeme", LeaseRecord{Backend: "vault", Path: "database/creds/app"})

	cache := newRecordingCache()
	cache.Put("database/creds/app", &Secret{Path: "database/creds/app", LeaseID: "lease-revokeme"})

	router, _ := NewRouter([]Route{{Prefix: "database/", Backend: "vault"}})
	b, _ := NewBroker(BrokerConfig{
		Router:         router,
		Backends:       []SecretBackend{fx.file, fx.vault},
		LeaseDirectory: dir,
		Cache:          cache,
		Auditor:        fx.auditor,
	})

	if err := b.RevokeLease(context.Background(), RevokeLeaseRequest{LeaseID: "lease-revokeme"}); err != nil {
		t.Fatalf("RevokeLease: %v", err)
	}
	if dir.Len() != 0 {
		t.Errorf("LeaseDirectory still has %d entries", dir.Len())
	}
	if _, hit := cache.Get("database/creds/app"); hit {
		t.Errorf("cache not invalidated on RevokeLease")
	}

	// Second RevokeLease should now miss in the directory.
	err := b.RevokeLease(context.Background(), RevokeLeaseRequest{LeaseID: "lease-revokeme"})
	if !errors.Is(err, ErrLeaseNotFound) {
		t.Errorf("second RevokeLease err = %v, want ErrLeaseNotFound", err)
	}
}

func TestBroker_LeaseRoutesToUnknownBackend(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	dir := NewInMemoryLeaseDirectory()
	// Lease was issued by a backend that no longer exists in this
	// broker's config (post-config-change or restore scenario).
	dir.Record("orphan-lease", LeaseRecord{Backend: "ghost", Path: "database/creds/app"})

	router, _ := NewRouter([]Route{{Prefix: "database/", Backend: "vault"}})
	b, _ := NewBroker(BrokerConfig{
		Router:         router,
		Backends:       []SecretBackend{fx.file, fx.vault},
		LeaseDirectory: dir,
		Auditor:        fx.auditor,
	})

	_, err := b.RenewLease(context.Background(), RenewLeaseRequest{LeaseID: "orphan-lease"})
	if err == nil {
		t.Fatalf("RenewLease on orphan lease = nil err, want ErrInvalidBackend wrap")
	}
	if !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
	if !strings.Contains(err.Error(), `unknown backend "ghost"`) {
		t.Errorf("err = %q, want unknown-backend message", err.Error())
	}
	// Same for RevokeLease.
	err = b.RevokeLease(context.Background(), RevokeLeaseRequest{LeaseID: "orphan-lease"})
	if !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("RevokeLease err does not wrap ErrInvalidBackend: %v", err)
	}
}

func TestBroker_LeaseCapabilityRefusal(t *testing.T) {
	t.Parallel()

	// Backend was registered, lease was recorded against it, but the
	// backend never declared CapLeaseRenew. Defensive.
	stripped := &fakeBackend{name: "stripped", caps: []BackendCapability{CapKV, CapDynamic}}
	auditor := &recordingAuditor{}
	dir := NewInMemoryLeaseDirectory()
	dir.Record("lease-x", LeaseRecord{Backend: "stripped", Path: "kv/foo"})

	b, _ := NewBroker(BrokerConfig{
		Backends:       []SecretBackend{stripped},
		DefaultBackend: "stripped",
		LeaseDirectory: dir,
		Auditor:        auditor,
	})

	_, err := b.RenewLease(context.Background(), RenewLeaseRequest{LeaseID: "lease-x"})
	if err == nil || !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("RenewLease err = %v, want wraps ErrInvalidBackend", err)
	}
	if !strings.Contains(err.Error(), "capability lease_renew") {
		t.Errorf("err = %q, want lease_renew capability message", err.Error())
	}
	err = b.RevokeLease(context.Background(), RevokeLeaseRequest{LeaseID: "lease-x"})
	if err == nil || !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("RevokeLease err = %v, want wraps ErrInvalidBackend", err)
	}
}

func TestBroker_RenewLease_ExpiredForgetsLease(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	dir := NewInMemoryLeaseDirectory()
	dir.Record("lease-expired", LeaseRecord{Backend: "vault", Path: "database/creds/app"})

	fx.vault.renewFn = func(RenewLeaseRequest) (*LeaseInfo, error) {
		return nil, fmt.Errorf("backend says: %w", ErrLeaseExpired)
	}

	router, _ := NewRouter([]Route{{Prefix: "database/", Backend: "vault"}})
	b, _ := NewBroker(BrokerConfig{
		Router:         router,
		Backends:       []SecretBackend{fx.file, fx.vault},
		LeaseDirectory: dir,
		Auditor:        fx.auditor,
	})

	_, err := b.RenewLease(context.Background(), RenewLeaseRequest{LeaseID: "lease-expired"})
	if !errors.Is(err, ErrLeaseExpired) {
		t.Fatalf("RenewLease err = %v, want wraps ErrLeaseExpired", err)
	}
	if dir.Len() != 0 {
		t.Errorf("expired lease not forgotten: dir.Len = %d", dir.Len())
	}
}

// ---- Audit propagation ----

func TestBroker_AuditFiresOnEveryOp(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	ctx := context.Background()

	if _, err := fx.broker.GetSecret(ctx, GetSecretRequest{Path: "kv/a"}); err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	if _, err := fx.broker.WriteSecret(ctx, WriteSecretRequest{Path: "kv/b", Data: map[string]any{"x": "y"}}); err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	if _, err := fx.broker.ListSecrets(ctx, ListSecretsRequest{Prefix: "kv/"}); err != nil {
		t.Fatalf("ListSecrets: %v", err)
	}
	if err := fx.broker.DeleteSecret(ctx, DeleteSecretRequest{Path: "kv/c"}); err != nil {
		t.Fatalf("DeleteSecret: %v", err)
	}

	if got, want := fx.auditor.Count(), 4; got != want {
		t.Errorf("audit Count = %d, want %d", got, want)
	}
}

func TestBroker_WriteSecret_AuditMasksPayload(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	_, err := fx.broker.WriteSecret(context.Background(), WriteSecretRequest{
		Path: "kv/db",
		Data: map[string]any{"password": "hunter2"},
	})
	if err != nil {
		t.Fatalf("WriteSecret: %v", err)
	}
	last := fx.auditor.Last()
	if last.MaskedPayload == nil {
		t.Fatalf("audit MaskedPayload nil; want masked map")
	}
	if last.MaskedPayload["password"] != MaskedValue {
		t.Errorf("audit MaskedPayload[password] = %v, want %q", last.MaskedPayload["password"], MaskedValue)
	}
	for _, v := range last.MaskedPayload {
		if v == "hunter2" {
			t.Errorf("cleartext leaked into audit MaskedPayload: %#v", last.MaskedPayload)
		}
	}
}

func TestBroker_PrincipalPropagated(t *testing.T) {
	t.Parallel()

	file := &fakeBackend{name: "file", caps: []BackendCapability{CapKV}}
	auditor := &recordingAuditor{}

	b, err := NewBroker(BrokerConfig{
		Backends:       []SecretBackend{file},
		DefaultBackend: "file",
		Auditor:        auditor,
		ExtractPrincipal: func(context.Context) Principal {
			return Principal{
				AgentID:  "agent-1",
				SPIFFEID: "spiffe://kscore.local/agent/agent-1",
				User:     "alice",
			}
		},
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}

	if _, err := b.GetSecret(context.Background(), GetSecretRequest{Path: "kv/foo"}); err != nil {
		t.Fatalf("GetSecret: %v", err)
	}
	last := auditor.Last()
	if last.Principal.AgentID != "agent-1" || last.Principal.SPIFFEID != "spiffe://kscore.local/agent/agent-1" || last.Principal.User != "alice" {
		t.Errorf("Principal not propagated: %#v", last.Principal)
	}
}

// ---- Lifecycle ----

func TestBroker_Lifecycle(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	ctx := context.Background()

	if err := fx.broker.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if fx.file.startCount != 1 || fx.vault.startCount != 1 {
		t.Errorf("Start fan-out incomplete: file=%d vault=%d", fx.file.startCount, fx.vault.startCount)
	}

	// Double-Start rejected.
	if err := fx.broker.Start(ctx); err == nil {
		t.Errorf("double-Start = nil err, want rejection")
	}

	// Health passes when every backend reports healthy.
	if err := fx.broker.Health(ctx); err != nil {
		t.Errorf("Health on healthy backends: %v", err)
	}

	// Unhealthy backend propagates.
	fx.vault.healthErr = errors.New("vault is sad")
	if err := fx.broker.Health(ctx); err == nil {
		t.Errorf("Health = nil err when vault unhealthy")
	} else if !strings.Contains(err.Error(), "vault") {
		t.Errorf("Health err = %q, want vault name in message", err.Error())
	}

	if err := fx.broker.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if fx.file.stopCount != 1 || fx.vault.stopCount != 1 {
		t.Errorf("Stop fan-out incomplete: file=%d vault=%d", fx.file.stopCount, fx.vault.stopCount)
	}

	// Double-Stop is idempotent.
	if err := fx.broker.Stop(ctx); err != nil {
		t.Errorf("double-Stop returned err: %v", err)
	}
}

func TestBroker_StartFailureRollsBack(t *testing.T) {
	t.Parallel()

	good := &fakeBackend{name: "good", caps: []BackendCapability{CapKV}}
	bad := &fakeBackend{name: "bad", caps: []BackendCapability{CapKV}, startErr: errors.New("nope")}

	b, err := NewBroker(BrokerConfig{
		Backends:       []SecretBackend{good, bad},
		DefaultBackend: "good",
	})
	if err != nil {
		t.Fatalf("NewBroker: %v", err)
	}

	err = b.Start(context.Background())
	if err == nil {
		t.Fatalf("Start = nil err on bad backend, want failure")
	}
	if !errors.Is(err, ErrInvalidBackend) {
		t.Errorf("err does not wrap ErrInvalidBackend: %v", err)
	}
	// The good backend may have been started before the bad one
	// failed; if so, Stop must have run on it during rollback.
	if good.startCount > 0 && good.stopCount == 0 {
		t.Errorf("rollback skipped: good.start=%d good.stop=%d", good.startCount, good.stopCount)
	}
}

func TestBroker_StartAfterStop_Rejected(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	ctx := context.Background()

	if err := fx.broker.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := fx.broker.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := fx.broker.Start(ctx); err == nil {
		t.Errorf("Start after Stop = nil err, want rejection")
	}
}

func TestBroker_HealthBeforeStart(t *testing.T) {
	t.Parallel()

	fx := newFixture(t)
	if err := fx.broker.Health(context.Background()); !errors.Is(err, ErrBackendNotStarted) {
		t.Errorf("Health pre-Start = %v, want wraps ErrBackendNotStarted", err)
	}
}

