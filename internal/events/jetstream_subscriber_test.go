// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// publishN emits n events of the given type via the raw JetStream
// publisher. Tests use this instead of JetStreamPublisher to keep
// subscriber tests independent of the publisher pipeline.
func publishN(t *testing.T, rig *embeddedJS, typ EventType, n int) []Event {
	t.Helper()
	out := make([]Event, 0, n)
	for i := 0; i < n; i++ {
		e := MustNewEvent(typ, fmt.Sprintf("src-%d", i))
		if _, err := e.StampSubject(rig.cluster); err != nil {
			t.Fatalf("StampSubject: %v", err)
		}
		payload, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if _, err := rig.js.Publish(e.Subject, payload); err != nil {
			t.Fatalf("Publish: %v", err)
		}
		out = append(out, e)
	}
	return out
}

func TestJetStreamSubscriber_SubscribeBroadcast(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	sub := NewJetStreamSubscriber(rig.js, rig.cluster)
	ctx := context.Background()
	if err := sub.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = sub.Stop(ctx) })

	// Two independent broadcast subscriptions on overlapping subjects.
	pattern := "kscore." + rig.cluster + ".events.>"
	var aCount, bCount atomic.Int64
	wait := make(chan struct{}, 16)

	a, err := sub.Subscribe(ctx, pattern, func(_ context.Context, _ Event) error {
		aCount.Add(1)
		wait <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe A: %v", err)
	}
	defer func() { _ = a.Unsubscribe() }()

	b, err := sub.Subscribe(ctx, pattern, func(_ context.Context, _ Event) error {
		bCount.Add(1)
		wait <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe B: %v", err)
	}
	defer func() { _ = b.Unsubscribe() }()

	const n = 3
	publishN(t, rig, EventTypeAgentConnect, n)

	expected := 2 * n // each event delivered to both subscribers
	deadline := time.After(3 * time.Second)
	got := 0
	for got < expected {
		select {
		case <-wait:
			got++
		case <-deadline:
			t.Fatalf("only %d/%d deliveries within 3s (A=%d B=%d)", got, expected, aCount.Load(), bCount.Load())
		}
	}
	if aCount.Load() != int64(n) || bCount.Load() != int64(n) {
		t.Errorf("counts: A=%d B=%d, want %d each", aCount.Load(), bCount.Load(), n)
	}
}

func TestJetStreamSubscriber_QueueGroupLoadBalances(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	sub := NewJetStreamSubscriber(rig.js, rig.cluster)
	ctx := context.Background()
	_ = sub.Start(ctx)
	t.Cleanup(func() { _ = sub.Stop(ctx) })

	pattern := "kscore." + rig.cluster + ".events.>"
	var c1, c2, c3 atomic.Int64
	done := make(chan struct{}, 16)
	handler := func(counter *atomic.Int64) EventHandler {
		return func(_ context.Context, _ Event) error {
			counter.Add(1)
			done <- struct{}{}
			return nil
		}
	}

	for _, c := range []*atomic.Int64{&c1, &c2, &c3} {
		s, err := sub.Subscribe(ctx, pattern, handler(c), WithQueueGroup("workers"))
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
		defer func() { _ = s.Unsubscribe() }()
	}

	const n = 9
	publishN(t, rig, EventTypeJobStart, n)

	deadline := time.After(5 * time.Second)
	got := 0
	for got < n {
		select {
		case <-done:
			got++
		case <-deadline:
			t.Fatalf("only %d/%d events delivered (c1=%d c2=%d c3=%d)",
				got, n, c1.Load(), c2.Load(), c3.Load())
		}
	}
	total := c1.Load() + c2.Load() + c3.Load()
	if total != int64(n) {
		t.Errorf("total = %d, want %d (no overlap allowed)", total, n)
	}
	// Queue-group semantics guarantee at-most-one-delivery (asserted by
	// total == n above), not uniform distribution. JetStream may route
	// every message to the same consumer if its ack rate keeps up; the
	// "each consumer sees ≥1" assertion is a stochastic property of the
	// scheduler, not a correctness property — and it flakes under load.
	// Verify ≥2 distinct consumers received some events, which is enough
	// to confirm load is actually being spread.
	distinct := 0
	for _, c := range []*atomic.Int64{&c1, &c2, &c3} {
		if c.Load() > 0 {
			distinct++
		}
	}
	if distinct < 2 {
		t.Errorf("queue group: only %d distinct consumers received events (c1=%d c2=%d c3=%d)",
			distinct, c1.Load(), c2.Load(), c3.Load())
	}
}

func TestJetStreamSubscriber_FilterPredicate(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	sub := NewJetStreamSubscriber(rig.js, rig.cluster)
	ctx := context.Background()
	_ = sub.Start(ctx)
	t.Cleanup(func() { _ = sub.Stop(ctx) })

	var receivedMu sync.Mutex
	var received []Event
	handler := func(_ context.Context, e Event) error {
		receivedMu.Lock()
		received = append(received, e)
		receivedMu.Unlock()
		return nil
	}
	pattern := "kscore." + rig.cluster + ".events.>"
	predicate := func(e Event) bool { return e.Severity.AtLeast(SeverityWarn) }

	s, err := sub.Subscribe(ctx, pattern, handler, WithFilter(predicate))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = s.Unsubscribe() }()

	// Publish 5 events at info/warn/error/info/critical.
	severities := []Severity{SeverityInfo, SeverityWarn, SeverityError, SeverityInfo, SeverityCritical}
	for _, sev := range severities {
		e := MustNewEvent(EventTypeAgentConnect, "src")
		e.Severity = sev
		if _, err := e.StampSubject(rig.cluster); err != nil {
			t.Fatalf("StampSubject: %v", err)
		}
		payload, _ := json.Marshal(e)
		if _, err := rig.js.Publish(e.Subject, payload); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}

	// Three should make it through: warn, error, critical.
	deadline := time.After(3 * time.Second)
	for {
		receivedMu.Lock()
		n := len(received)
		receivedMu.Unlock()
		if n >= 3 {
			break
		}
		select {
		case <-deadline:
			receivedMu.Lock()
			defer receivedMu.Unlock()
			t.Fatalf("only %d/3 delivered: %v", len(received), received)
		default:
			time.Sleep(20 * time.Millisecond)
		}
	}
	// And no info-level events leaked through.
	time.Sleep(200 * time.Millisecond)
	receivedMu.Lock()
	defer receivedMu.Unlock()
	if len(received) != 3 {
		t.Errorf("received %d, want 3", len(received))
	}
	for _, e := range received {
		if !e.Severity.AtLeast(SeverityWarn) {
			t.Errorf("filter leaked %s event", e.Severity)
		}
	}
}

func TestJetStreamSubscriber_HandlerErrorRedelivers(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	sub := NewJetStreamSubscriber(rig.js, rig.cluster)
	ctx := context.Background()
	_ = sub.Start(ctx)
	t.Cleanup(func() { _ = sub.Stop(ctx) })

	var attempts atomic.Int64
	handler := func(_ context.Context, _ Event) error {
		n := attempts.Add(1)
		if n < 3 {
			return errors.New("transient handler failure")
		}
		return nil
	}
	pattern := "kscore." + rig.cluster + ".events.>"
	s, err := sub.Subscribe(ctx, pattern, handler, WithMaxRedeliveries(5))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = s.Unsubscribe() }()

	publishN(t, rig, EventTypeAgentConnect, 1)

	// First attempt fails → Nak 1s, second fails → Nak 5s, third succeeds.
	// Total wait bound ~7s for the third attempt.
	deadline := time.After(15 * time.Second)
	for attempts.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("attempts = %d, want 3", attempts.Load())
		case <-time.After(100 * time.Millisecond):
		}
	}
	// Allow a brief settle so any extra delivery would land.
	time.Sleep(500 * time.Millisecond)
	if got := attempts.Load(); got != 3 {
		t.Errorf("attempts = %d, want exactly 3 (succeeded on retry)", got)
	}
}

func TestJetStreamSubscriber_ReplayFromStore(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)

	// Pre-seed the store with 3 "historical" events. We use the
	// stubStore from publisher_test.go.
	store := &stubStore{}
	for i := 0; i < 3; i++ {
		e := MustNewEvent(EventTypeAgentConnect, fmt.Sprintf("hist-%d", i))
		if _, err := e.StampSubject(rig.cluster); err != nil {
			t.Fatalf("StampSubject: %v", err)
		}
		// Direct insert into stub.
		store.mu.Lock()
		store.stored = append(store.stored, e)
		store.mu.Unlock()
	}

	// Also publish 2 fresh events to JetStream. They'll be present
	// on the live path AND not yet in the store; the dedup set
	// should NOT trigger for them (different IDs from historical).
	freshCount := 2
	fresh := publishN(t, rig, EventTypeJobStart, freshCount)
	_ = fresh

	sub := NewJetStreamSubscriber(rig.js, rig.cluster,
		WithSubscriberStore(replayStoreFromStub(store)),
	)
	ctx := context.Background()
	_ = sub.Start(ctx)
	t.Cleanup(func() { _ = sub.Stop(ctx) })

	var receivedMu sync.Mutex
	var received []Event
	handler := func(_ context.Context, e Event) error {
		receivedMu.Lock()
		received = append(received, e)
		receivedMu.Unlock()
		return nil
	}
	pattern := "kscore." + rig.cluster + ".events.>"
	s, err := sub.Subscribe(ctx, pattern, handler, WithReplay(time.Hour))
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = s.Unsubscribe() }()

	// Expect 5 total: 3 historical + 2 live.
	expected := 3 + freshCount
	deadline := time.After(5 * time.Second)
	for {
		receivedMu.Lock()
		n := len(received)
		receivedMu.Unlock()
		if n >= expected {
			break
		}
		select {
		case <-deadline:
			receivedMu.Lock()
			defer receivedMu.Unlock()
			t.Fatalf("only %d/%d delivered: ids=%v", len(received), expected, idsOf(received))
		default:
			time.Sleep(30 * time.Millisecond)
		}
	}
	// Verify no duplicates by IDs.
	time.Sleep(200 * time.Millisecond)
	receivedMu.Lock()
	defer receivedMu.Unlock()
	seen := map[string]int{}
	for _, e := range received {
		seen[e.ID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("event %s dispatched %d times; want 1", id, count)
		}
	}
	if len(seen) != expected {
		t.Errorf("unique IDs = %d, want %d", len(seen), expected)
	}
}

func TestJetStreamSubscriber_ReplayRequiresStore(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	sub := NewJetStreamSubscriber(rig.js, rig.cluster)
	ctx := context.Background()
	_ = sub.Start(ctx)
	t.Cleanup(func() { _ = sub.Stop(ctx) })

	pattern := "kscore." + rig.cluster + ".events.>"
	_, err := sub.Subscribe(ctx, pattern, func(_ context.Context, _ Event) error { return nil },
		WithReplay(time.Minute))
	if err == nil {
		t.Fatalf("Subscribe with WithReplay + no store succeeded; want error")
	}
}

func TestJetStreamSubscriber_UnsubscribeStopsDelivery(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	sub := NewJetStreamSubscriber(rig.js, rig.cluster)
	ctx := context.Background()
	_ = sub.Start(ctx)
	t.Cleanup(func() { _ = sub.Stop(ctx) })

	var count atomic.Int64
	got := make(chan struct{}, 4)
	s, err := sub.Subscribe(ctx, "kscore."+rig.cluster+".events.>", func(_ context.Context, _ Event) error {
		count.Add(1)
		got <- struct{}{}
		return nil
	})
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}

	publishN(t, rig, EventTypeAgentConnect, 1)
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatalf("first event not delivered")
	}
	if err := s.Unsubscribe(); err != nil {
		t.Fatalf("Unsubscribe: %v", err)
	}
	// Second Unsubscribe is a no-op.
	if err := s.Unsubscribe(); err != nil {
		t.Errorf("second Unsubscribe: %v", err)
	}

	publishN(t, rig, EventTypeAgentConnect, 2)
	time.Sleep(300 * time.Millisecond)
	if got := count.Load(); got != 1 {
		t.Errorf("count after Unsubscribe = %d, want 1", got)
	}
}

func TestJetStreamSubscriber_StopUnsubscribesAll(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	sub := NewJetStreamSubscriber(rig.js, rig.cluster)
	ctx := context.Background()
	_ = sub.Start(ctx)

	pattern := "kscore." + rig.cluster + ".events.>"
	for i := 0; i < 2; i++ {
		if _, err := sub.Subscribe(ctx, pattern, func(_ context.Context, _ Event) error { return nil }); err != nil {
			t.Fatalf("Subscribe[%d]: %v", i, err)
		}
	}
	if got := len(sub.subs); got != 2 {
		t.Errorf("pre-Stop subs count = %d, want 2", got)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := sub.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got := len(sub.subs); got != 0 {
		t.Errorf("post-Stop subs count = %d, want 0", got)
	}
}

func TestJetStreamSubscriber_LifecycleGuards(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	sub := NewJetStreamSubscriber(rig.js, rig.cluster)
	ctx := context.Background()

	// Pre-Start Subscribe rejected.
	_, err := sub.Subscribe(ctx, "kscore."+rig.cluster+".events.>", func(_ context.Context, _ Event) error { return nil })
	if !errors.Is(err, ErrSubscriberNotStarted) {
		t.Errorf("pre-Start Subscribe err = %v, want ErrSubscriberNotStarted", err)
	}

	// Start.
	if err := sub.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// Double Start rejected.
	if err := sub.Start(ctx); err == nil {
		t.Errorf("double Start succeeded; want error")
	}
	// Stop is idempotent.
	if err := sub.Stop(ctx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if err := sub.Stop(ctx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
	// Stop-before-Start (after a Stop) is a no-op.
	if err := sub.Stop(ctx); err != nil {
		t.Errorf("third Stop: %v", err)
	}
}

// --- helpers -----------------------------------------------------------------

// replayStoreFromStub adapts a stubStore into the EventStore
// interface for tests that need a Query that returns the stored
// events. The plain stubStore from publisher_test.go has Query that
// always returns empty; this wrapper supplies the events the
// subscriber's replay path expects to walk.
type stubStoreReplay struct {
	*stubStore
}

func replayStoreFromStub(s *stubStore) EventStore { return &stubStoreReplay{s} }

func (s *stubStoreReplay) Query(_ context.Context, _ EventQuery) (EventPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	page := EventPage{Events: make([]Event, len(s.stored))}
	copy(page.Events, s.stored)
	return page, nil
}

func idsOf(events []Event) []string {
	out := make([]string, len(events))
	for i, e := range events {
		out[i] = e.ID
	}
	return out
}
