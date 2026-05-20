//go:build integration

// Package webhooke2e is the Epic 16 task-18 end-to-end integration
// suite. It wires the REAL outbound pipeline exactly as kscore-server
// boot will compose it — task-11 SQLiteStore (:memory:) + task-12
// Manager + task-14 retry + task-15 CircuitBreaker + task-13
// HTTPDispatcher + task-17 signing — and validates the §4.14
// contract against a real httptest.Server receiver. The receiver
// uses outbound.Verify on the inbound X-Keystone-Signature so the
// test IS the receiver-side validator the epic asks for.
//
// Run with: make test-integration  (i.e. -tags=integration).
package webhooke2e

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"

	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/webhook/outbound"
)

// waitFor polls predicate until true or deadline. Integration tests
// wait on the store because Manager.Handle's fan-out is async.
func waitFor(t *testing.T, predicate func() bool, label string) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", label)
}

// fastRetry is the deterministic retry policy used by every test:
// sub-millisecond backoff and zero jitter keep the suite fast and
// race-free without changing the retry semantics under test.
var fastRetry = outbound.RetryPolicy{
	BaseBackoff: time.Millisecond,
	MaxBackoff:  2 * time.Millisecond,
}

func newSub(t *testing.T, id, url, secret string, maxRetries int, events []string) *outbound.Subscription {
	t.Helper()
	now := time.Now().UTC()
	return &outbound.Subscription{
		ID: id, Name: id, URL: url, Secret: secret,
		Events: events, Enabled: true,
		MaxRetries: maxRetries, TimeoutSec: 5,
		CreatedAt: now, UpdatedAt: now,
	}
}

// newManager builds a Manager wired with the given Dispatcher over a
// fresh in-memory SQLite store and Starts it. Stop is registered via
// t.Cleanup. Sub MaxRetries defaults to 0 (one shot) unless the test
// creates a sub with a higher number.
func newManager(t *testing.T, dispatcher outbound.Dispatcher, maxConcurrent int) (*outbound.Manager, outbound.SubscriptionStore) {
	t.Helper()
	store, err := outbound.NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	m := &outbound.Manager{
		Store:                   store,
		Dispatcher:              dispatcher,
		MaxConcurrentDeliveries: maxConcurrent,
		Retry:                   fastRetry,
		Jitterer:                func() float64 { return 0 },
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Manager.Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = m.Stop(ctx)
	})
	return m, store
}

// TestE2E_HappyPath_EventDeliveredWithValidSignature wires the full
// pipeline: event emitted → Manager glob-matches → HTTPDispatcher
// POSTs → receiver verifies the X-Keystone-Signature via the public
// outbound.Verify (task 17) → DeliveryRecord persisted in the
// SQLite store as Success/200/Attempt=1.
func TestE2E_HappyPath_EventDeliveredWithValidSignature(t *testing.T) {
	t.Parallel()
	const secret = "topsecret"
	var sigValidated int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		sig := r.Header.Get("X-Keystone-Signature")
		if !outbound.Verify([]byte(secret), sig, body) {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		atomic.AddInt32(&sigValidated, 1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)

	d := &outbound.HTTPDispatcher{HTTPClient: srv.Client()}
	m, store := newManager(t, d, 4)
	ctx := context.Background()

	sub := newSub(t, uuid.NewString(), srv.URL, secret, 0, []string{"state.drift"})
	if err := store.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("create sub: %v", err)
	}
	if err := m.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	ev, err := events.NewEvent("state.drift", "e2e")
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	m.Handle(ctx, ev)

	waitFor(t, func() bool {
		list, _ := store.ListDeliveries(ctx, sub.ID, 0)
		return len(list) == 1 && list[0].Status == outbound.DeliverySuccess
	}, "delivery success persisted")

	if atomic.LoadInt32(&sigValidated) != 1 {
		t.Errorf("sig validations = %d, want 1 — receiver did not see a valid X-Keystone-Signature", sigValidated)
	}
	list, _ := store.ListDeliveries(ctx, sub.ID, 0)
	if list[0].StatusCode != 200 || list[0].Attempt != 1 || list[0].EventType != "state.drift" {
		t.Errorf("delivery record = %+v, want 200/Attempt=1/state.drift", list[0])
	}
}

// TestE2E_RetryExhaustion_Acceptance115 covers the §4.14 retry
// contract end-to-end: receiver always returns 502, Manager retries
// 1 + MaxRetries times (acceptance line 115 says "3 attempts"), each
// attempt signed via the same Sign helper, final DeliveryRecord
// `failed` and **retained** in the store.
func TestE2E_RetryExhaustion_Acceptance115(t *testing.T) {
	t.Parallel()
	const secret = "shh"
	var (
		calls       int32
		validCount  int32
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if outbound.Verify([]byte(secret), r.Header.Get("X-Keystone-Signature"), body) {
			atomic.AddInt32(&validCount, 1)
		}
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	d := &outbound.HTTPDispatcher{HTTPClient: srv.Client()}
	m, store := newManager(t, d, 4)
	ctx := context.Background()

	// MaxRetries=2 → 1 initial + 2 retries = 3 attempts (§4.14 line 115).
	sub := newSub(t, uuid.NewString(), srv.URL, secret, 2, []string{"policy.violation"})
	if err := store.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("create sub: %v", err)
	}
	if err := m.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	ev, err := events.NewEvent("policy.violation", "e2e")
	if err != nil {
		t.Fatalf("NewEvent: %v", err)
	}
	m.Handle(ctx, ev)

	waitFor(t, func() bool {
		list, _ := store.ListDeliveries(ctx, sub.ID, 0)
		return len(list) == 1 && list[0].Status == outbound.DeliveryFailed
	}, "final failed delivery persisted")

	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Errorf("receiver POSTs = %d, want 3 (1+MaxRetries)", got)
	}
	if got := atomic.LoadInt32(&validCount); got != 3 {
		t.Errorf("valid-signature attempts = %d, want 3 (every retry signed)", got)
	}

	// Record retained per §4.14 — Get after the failure returns it.
	got, ok, err := store.GetDelivery(ctx, mustOneDelivery(t, store, sub.ID).ID)
	if err != nil || !ok {
		t.Fatalf("GetDelivery: ok=%v err=%v", ok, err)
	}
	if got.Status != outbound.DeliveryFailed || got.Attempt != 3 || got.StatusCode != 502 || got.Error == "" {
		t.Errorf("retained record = %+v, want Failed/Attempt=3/502/+error", got)
	}
}

// TestE2E_CircuitBreaker_FastFailsAfterFiveFailures wires the CB
// decorator (task 15) around the HTTPDispatcher and proves that
// after 5 consecutive failed deliveries the breaker opens and a 6th
// delivery never reaches the receiver — the §4.14 5/30s/2 contract
// observed against a real receiver.
func TestE2E_CircuitBreaker_FastFailsAfterFiveFailures(t *testing.T) {
	t.Parallel()
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	t.Cleanup(srv.Close)

	// Real HTTPDispatcher wrapped by the real CB. No retries on the
	// subscription itself — each Handle is one attempt — so the
	// breaker's failure count tracks Handle calls 1:1.
	inner := &outbound.HTTPDispatcher{HTTPClient: srv.Client()}
	cb := &outbound.CircuitBreaker{Inner: inner}
	// MaxConcurrent=1 serializes the fan-out goroutines so the
	// breaker's per-key counter advances deterministically (Handle
	// blocks on the semaphore acquire when full).
	m, store := newManager(t, cb, 1)
	ctx := context.Background()

	sub := newSub(t, uuid.NewString(), srv.URL, "k", 0, []string{"state.drift"})
	if err := store.CreateSubscription(ctx, sub); err != nil {
		t.Fatalf("create sub: %v", err)
	}
	if err := m.Refresh(ctx); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	// Fire 5 events → 5 attempts, all fail → CB opens.
	for i := 0; i < 5; i++ {
		ev, _ := events.NewEvent("state.drift", "e2e")
		m.Handle(ctx, ev)
	}
	waitFor(t, func() bool {
		list, _ := store.ListDeliveries(ctx, sub.ID, 0)
		if len(list) != 5 {
			return false
		}
		for _, r := range list {
			if r.Status != outbound.DeliveryFailed {
				return false
			}
		}
		return true
	}, "5 failed deliveries persisted")

	if got := atomic.LoadInt32(&calls); got != 5 {
		t.Fatalf("after 5 events, receiver POSTs = %d, want 5", got)
	}

	// 6th event: CB is open → fast-fail without touching the receiver.
	ev, _ := events.NewEvent("state.drift", "e2e")
	m.Handle(ctx, ev)
	// Wait for the 6th delivery to reach its TERMINAL state — Manager
	// saves Pending first then upserts Failed after the goroutine
	// runs, so len==6 alone doesn't prove the fast-fail landed.
	waitFor(t, func() bool {
		list, _ := store.ListDeliveries(ctx, sub.ID, 0)
		return len(list) == 6 && list[5].Status == outbound.DeliveryFailed
	}, "6th (fast-failed) delivery persisted")

	if got := atomic.LoadInt32(&calls); got != 5 {
		t.Errorf("after CB opens, receiver POSTs = %d, want still 5 (no extra POST)", got)
	}
	list, _ := store.ListDeliveries(ctx, sub.ID, 0)
	last := list[len(list)-1]
	if last.Status != outbound.DeliveryFailed {
		t.Errorf("6th delivery status = %s, want failed", last.Status)
	}
	if !strings.Contains(last.Error, "circuit breaker open") {
		t.Errorf("6th delivery error = %q, want it to mention the CB", last.Error)
	}
}

func mustOneDelivery(t *testing.T, store outbound.SubscriptionStore, subID string) *outbound.DeliveryRecord {
	t.Helper()
	list, _ := store.ListDeliveries(context.Background(), subID, 0)
	if len(list) != 1 {
		t.Fatalf("ListDeliveries(%s) = %d, want 1", subID, len(list))
	}
	return list[0]
}
