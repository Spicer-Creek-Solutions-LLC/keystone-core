// SPDX-License-Identifier: Apache-2.0

//go:build integration

// End-to-end integration tests for Epic 11 — the §4.9 acceptance
// lines that pull every prior task together: emit 1000 events,
// query by filter, subscribe with replay, retention deletes old,
// slow consumer redelivers up to 3 times.
//
// External package (events_test) so the tests exercise only the
// public API surface — no access to internals. Build-tagged so
// they don't run in the default `make test`; run via
// `make test-integration` or `go test -tags integration ./internal/events/...`.

package events_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/state"
)

// integrationRig wires every Epic 11 layer end-to-end: embedded
// nats-server + JetStream + SQLite-backed state.Store + events.
// EventStore + JetStreamPublisher + JetStreamSubscriber.
//
// Each sub-test calls newIntegrationRig for a fresh stack — cheap
// (~30ms per rig) and avoids cross-test contamination.
type integrationRig struct {
	srv        *natsserver.Server
	conn       *nats.Conn
	js         nats.JetStreamContext
	stateStore state.Store
	eventStore events.EventStore
	publisher  *events.JetStreamPublisher
	subscriber *events.JetStreamSubscriber
	cluster    string
	streamName string
}

func newIntegrationRig(t *testing.T) *integrationRig {
	t.Helper()

	// Embedded NATS with JetStream enabled. TempDir survives the
	// duration of the test only.
	storeDir := filepath.Join(t.TempDir(), "jetstream")
	port := freeTCPPort(t)
	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      port,
		NoSigs:    true,
		NoLog:     true,
		JetStream: true,
		StoreDir:  storeDir,
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("nats NewServer: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("nats not ready")
	}

	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("nats connect: %v", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("jetstream: %v", err)
	}

	const cluster = "test"
	streamName := "KSCORE_EVENTS_test"
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:      streamName,
		Subjects:  []string{"kscore." + cluster + ".events.>"},
		Retention: nats.LimitsPolicy,
		Discard:   nats.DiscardNew,
		Storage:   nats.FileStorage,
	}); err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("AddStream: %v", err)
	}

	// SQLite state.Store in TempDir.
	stateStore, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "events.db")},
	})
	if err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("state.NewStore: %v", err)
	}

	eventStore := events.NewSQLEventStore(stateStore)

	publisher := events.NewJetStreamPublisher(js, cluster,
		events.WithStore(eventStore),
		events.WithBufferSize(2000),
		events.WithFlushTimeout(200*time.Millisecond),
	)
	if err := publisher.Start(context.Background()); err != nil {
		_ = stateStore.Close()
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("publisher Start: %v", err)
	}

	subscriber := events.NewJetStreamSubscriber(js, cluster,
		events.WithSubscriberStore(eventStore),
		events.WithDedupSize(2000),
	)
	if err := subscriber.Start(context.Background()); err != nil {
		_ = publisher.Stop(context.Background())
		_ = stateStore.Close()
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("subscriber Start: %v", err)
	}

	rig := &integrationRig{
		srv:        srv,
		conn:       conn,
		js:         js,
		stateStore: stateStore,
		eventStore: eventStore,
		publisher:  publisher,
		subscriber: subscriber,
		cluster:    cluster,
		streamName: streamName,
	}
	t.Cleanup(rig.close)
	return rig
}

func (r *integrationRig) close() {
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = r.subscriber.Stop(stopCtx)
	_ = r.publisher.Stop(stopCtx)
	_ = r.stateStore.Close()
	if r.conn != nil {
		r.conn.Close()
	}
	if r.srv != nil {
		r.srv.Shutdown()
		r.srv.WaitForShutdown()
	}
}

func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// TestEpic11_Integration is the umbrella that walks every §4.9
// acceptance line as a sub-test. Run via
// `go test -tags integration ./internal/events/...` or
// `make test-integration`.
func TestEpic11_Integration(t *testing.T) {
	t.Run("Emit_StoresAndPublishes_1000Events", testEmit1000Events)
	t.Run("ListWithStructuralFilters", testListWithStructuralFilters)
	t.Run("SubscribeWithCELFilter_LiveStreaming", testSubscribeWithCELFilter)
	t.Run("SubscribeWithReplay_HistoricalThenLive", testSubscribeWithReplay)
	t.Run("RetentionDeletesOldEvents", testRetentionDeletesOldEvents)
	t.Run("SlowConsumerRedelivers_MaxThreeTimes", testSlowConsumerRedelivers)
}

// ---- Emit_StoresAndPublishes_1000Events ---------------------------------

// Pins the §4.9 acceptance: emit records event in store AND
// publishes on NATS. The "1000 events" comes from the task-11 spec
// literal ("emit 1000 events").
func testEmit1000Events(t *testing.T) {
	t.Parallel()
	rig := newIntegrationRig(t)
	ctx := context.Background()

	// Subscribe BEFORE emitting so the subscription captures every
	// message. Counts atomically so the assertion is race-free.
	const n = 1000
	var received atomic.Int64
	done := make(chan struct{})
	handler := func(_ context.Context, _ events.Event) error {
		if received.Add(1) == int64(n) {
			close(done)
		}
		return nil
	}
	sub, err := rig.subscriber.Subscribe(ctx,
		"kscore."+rig.cluster+".events.>",
		handler,
	)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	// Emit 1000 events. The publisher is store-first so each event
	// lands in the store before NATS.
	for i := 0; i < n; i++ {
		e := events.MustNewEvent(events.EventTypeAgentHeartbeat, fmt.Sprintf("agent-%d", i%50))
		if err := rig.publisher.Publish(ctx, e); err != nil {
			t.Fatalf("Publish[%d]: %v", i, err)
		}
	}

	// Wait up to 30s for the subscriber to drain.
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("subscriber only received %d/%d events within 30s", received.Load(), n)
	}

	// Verify the store has every event too.
	count, err := rig.eventStore.Count(ctx, events.EventQuery{})
	if err != nil {
		t.Fatalf("Count: %v", err)
	}
	if count != n {
		t.Errorf("store Count = %d, want %d", count, n)
	}
}

// ---- ListWithStructuralFilters -----------------------------------------

// Pins the §4.9 acceptance: list --type 'agent.*' --severity '>=warn'
// --since 1h --limit 50 returns paginated.
func testListWithStructuralFilters(t *testing.T) {
	t.Parallel()
	rig := newIntegrationRig(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// Seed a mix: 30 agent.* warn-or-above, 15 agent.* info, 20
	// job.* warn-or-above, 5 ancient agent.* warn (>1h old). Total
	// 70 events.
	seed := func(typ events.EventType, sev events.Severity, age time.Duration, n int) {
		for i := 0; i < n; i++ {
			e := events.MustNewEvent(typ, fmt.Sprintf("src-%d", i))
			e.Severity = sev
			e.Time = now.Add(-age)
			if err := rig.eventStore.Store(ctx, e); err != nil {
				t.Fatalf("seed Store: %v", err)
			}
		}
	}
	seed(events.EventTypeAgentConnect, events.SeverityWarn, 30*time.Minute, 15)
	seed(events.EventTypeAgentError, events.SeverityError, 45*time.Minute, 10)
	seed(events.EventTypeAgentHeartbeat, events.SeverityCritical, 10*time.Minute, 5)
	seed(events.EventTypeAgentConnect, events.SeverityInfo, 30*time.Minute, 15)
	seed(events.EventTypeJobFail, events.SeverityError, 30*time.Minute, 20)
	seed(events.EventTypeAgentError, events.SeverityWarn, 2*time.Hour, 5) // outside --since 1h

	// Query: --type 'agent.*' --severity '>=warn' --since 1h --limit 50.
	q := events.EventQuery{
		Category:    events.CategoryAgent,
		MinSeverity: events.SeverityWarn,
		Since:       now.Add(-time.Hour),
		Limit:       50,
	}
	page, err := rig.eventStore.Query(ctx, q)
	if err != nil {
		t.Fatalf("Query: %v", err)
	}

	// Expected: 30 events (15 connect-warn + 10 error-error + 5
	// heartbeat-critical), all within 1h, all agent category.
	const want = 30
	if len(page.Events) != want {
		t.Errorf("len = %d, want %d", len(page.Events), want)
	}
	// Cursor must be empty — fewer than Limit events, no next page.
	if page.NextCursor != "" {
		t.Errorf("NextCursor = %q, want empty (short page)", page.NextCursor)
	}
	// Each returned event must be agent.*, at-least warn, within
	// the time window.
	for _, e := range page.Events {
		if e.Type.Category() != events.CategoryAgent {
			t.Errorf("non-agent leaked: %s", e.Type)
		}
		if !e.Severity.AtLeast(events.SeverityWarn) {
			t.Errorf("below-threshold severity leaked: %s", e.Severity)
		}
		if e.Time.Before(now.Add(-time.Hour)) {
			t.Errorf("outside-window event leaked: %v", e.Time)
		}
	}

	// Pagination smoke: limit 20 → page1 has cursor, page2 fills
	// the rest.
	page1, _ := rig.eventStore.Query(ctx, events.EventQuery{
		Category:    events.CategoryAgent,
		MinSeverity: events.SeverityWarn,
		Since:       now.Add(-time.Hour),
		Limit:       20,
	})
	if len(page1.Events) != 20 || page1.NextCursor == "" {
		t.Errorf("page1: len=%d cursor=%q (want 20 + non-empty)", len(page1.Events), page1.NextCursor)
	}
	page2, _ := rig.eventStore.Query(ctx, events.EventQuery{
		Category:    events.CategoryAgent,
		MinSeverity: events.SeverityWarn,
		Since:       now.Add(-time.Hour),
		Limit:       20,
		Cursor:      page1.NextCursor,
	})
	if len(page2.Events) != 10 {
		t.Errorf("page2: len=%d, want 10 (remaining of 30)", len(page2.Events))
	}
}

// ---- SubscribeWithCELFilter_LiveStreaming -------------------------------

// Pins the §4.9 acceptance: subscribe --filter ... streams matching
// events realtime. Uses the §4.9-style filter expression in the
// `severity.at_least('warn')` form (the lex-correct version of the
// spec's `severity >= 'warn'` which doesn't work — task 5's note).
func testSubscribeWithCELFilter(t *testing.T) {
	t.Parallel()
	rig := newIntegrationRig(t)
	ctx := context.Background()

	filter, err := events.CompileFilter("tags.role == 'web' && severity.at_least('warn')")
	if err != nil {
		t.Fatalf("CompileFilter: %v", err)
	}

	var receivedMu sync.Mutex
	var received []events.Event
	handler := func(_ context.Context, e events.Event) error {
		receivedMu.Lock()
		received = append(received, e)
		receivedMu.Unlock()
		return nil
	}
	sub, err := rig.subscriber.Subscribe(ctx,
		"kscore."+rig.cluster+".events.>",
		handler,
		events.WithFilter(filter.Match),
	)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	// Emit a mix:
	//   - role=web + sev=warn   → match (1)
	//   - role=web + sev=error  → match (2)
	//   - role=web + sev=info   → drop (severity below)
	//   - role=db  + sev=error  → drop (tag mismatch)
	//   - no tags  + sev=warn   → drop (tag missing → CEL eval err → false)
	publish := func(role string, sev events.Severity) {
		e := events.MustNewEvent(events.EventTypeAgentConnect, "agent-x")
		e.Severity = sev
		if role != "" {
			e.Tags = map[string]string{"role": role}
		}
		if err := rig.publisher.Publish(ctx, e); err != nil {
			t.Fatalf("Publish: %v", err)
		}
	}
	publish("web", events.SeverityWarn)
	publish("web", events.SeverityError)
	publish("web", events.SeverityInfo)
	publish("db", events.SeverityError)
	publish("", events.SeverityWarn)

	// Wait for the 2 matches to land.
	deadline := time.After(10 * time.Second)
	for {
		receivedMu.Lock()
		n := len(received)
		receivedMu.Unlock()
		if n >= 2 {
			break
		}
		select {
		case <-deadline:
			receivedMu.Lock()
			defer receivedMu.Unlock()
			t.Fatalf("only %d/2 matches within 10s: %+v", len(received), received)
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Settle then assert no extras leaked.
	time.Sleep(500 * time.Millisecond)
	receivedMu.Lock()
	defer receivedMu.Unlock()
	if len(received) != 2 {
		t.Errorf("received %d, want 2 (filter let extras through)", len(received))
	}
	for _, e := range received {
		if e.Tags["role"] != "web" {
			t.Errorf("non-web event leaked: %+v", e.Tags)
		}
		if !e.Severity.AtLeast(events.SeverityWarn) {
			t.Errorf("below-warn event leaked: %s", e.Severity)
		}
	}
}

// ---- SubscribeWithReplay_HistoricalThenLive -----------------------------

// Pins the §4.9 acceptance: --replay 60s streams last 60s historical
// events then continues realtime.
func testSubscribeWithReplay(t *testing.T) {
	t.Parallel()
	rig := newIntegrationRig(t)
	ctx := context.Background()

	// Write 5 events directly to the store at past timestamps —
	// these are the "historical" events the replay should surface.
	// Use IDs the dedup set will recognise so JetStream-side
	// duplicates Ack-without-dispatch.
	now := time.Now().UTC()
	historical := make([]events.Event, 5)
	for i := range historical {
		e := events.MustNewEvent(events.EventTypeAgentConnect, fmt.Sprintf("hist-%d", i))
		e.Time = now.Add(-time.Duration(i+1) * time.Minute)
		if _, err := e.StampSubject(rig.cluster); err != nil {
			t.Fatalf("StampSubject: %v", err)
		}
		historical[i] = e
		// Persist to store ONLY — not publishing to NATS so the
		// replay path is the only way the subscriber sees them.
		if err := rig.eventStore.Store(ctx, e); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	var receivedMu sync.Mutex
	receivedIDs := make(map[string]int)
	handler := func(_ context.Context, e events.Event) error {
		receivedMu.Lock()
		receivedIDs[e.ID]++
		receivedMu.Unlock()
		return nil
	}
	sub, err := rig.subscriber.Subscribe(ctx,
		"kscore."+rig.cluster+".events.>",
		handler,
		events.WithReplay(1*time.Hour),
	)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	// Wait for the 5 historical events to flow through replay.
	waitForCount := func(min int, deadline time.Duration) int {
		end := time.After(deadline)
		for {
			receivedMu.Lock()
			n := len(receivedIDs)
			receivedMu.Unlock()
			if n >= min {
				return n
			}
			select {
			case <-end:
				return n
			case <-time.After(50 * time.Millisecond):
			}
		}
	}
	if got := waitForCount(5, 10*time.Second); got < 5 {
		t.Fatalf("replay phase: got %d historical, want >= 5", got)
	}

	// Now emit 3 live events. Same subscription should receive
	// them too.
	live := make([]events.Event, 3)
	for i := range live {
		e := events.MustNewEvent(events.EventTypeJobStart, fmt.Sprintf("live-%d", i))
		live[i] = e
		if err := rig.publisher.Publish(ctx, e); err != nil {
			t.Fatalf("live Publish[%d]: %v", i, err)
		}
	}

	if got := waitForCount(8, 10*time.Second); got < 8 {
		t.Fatalf("live phase: got %d total, want >= 8 (5 hist + 3 live)", got)
	}

	// Settle and assert no duplicates — every event dispatched
	// exactly once (dedup set caught overlap between store-replay
	// and live JetStream).
	time.Sleep(500 * time.Millisecond)
	receivedMu.Lock()
	defer receivedMu.Unlock()
	for id, count := range receivedIDs {
		if count > 1 {
			t.Errorf("event %s dispatched %d times; dedup failed", id, count)
		}
	}
	for _, e := range historical {
		if _, ok := receivedIDs[e.ID]; !ok {
			t.Errorf("historical event %s missing from replay", e.ID)
		}
	}
	for _, e := range live {
		if _, ok := receivedIDs[e.ID]; !ok {
			t.Errorf("live event %s missing", e.ID)
		}
	}
}

// ---- RetentionDeletesOldEvents ------------------------------------------

// Pins the §4.9 acceptance: retention deletes events older than
// configured age + count limits. Drives the enforcer's RunOnce
// directly (the scheduler-loop path is unit-tested in
// retention_test.go; here we want the e2e store interaction).
func testRetentionDeletesOldEvents(t *testing.T) {
	t.Parallel()
	rig := newIntegrationRig(t)
	ctx := context.Background()
	now := time.Now().UTC()

	// 30 old events (25h ago) + 30 fresh.
	const (
		oldCount   = 30
		freshCount = 30
	)
	for i := 0; i < oldCount; i++ {
		e := events.MustNewEvent(events.EventTypeAgentHeartbeat, fmt.Sprintf("old-%d", i))
		e.Time = now.Add(-25 * time.Hour)
		if err := rig.eventStore.Store(ctx, e); err != nil {
			t.Fatalf("seed old: %v", err)
		}
	}
	for i := 0; i < freshCount; i++ {
		e := events.MustNewEvent(events.EventTypeAgentHeartbeat, fmt.Sprintf("fresh-%d", i))
		if err := rig.eventStore.Store(ctx, e); err != nil {
			t.Fatalf("seed fresh: %v", err)
		}
	}

	preCount, _ := rig.eventStore.Count(ctx, events.EventQuery{})
	if preCount != oldCount+freshCount {
		t.Fatalf("pre-retention count = %d, want %d", preCount, oldCount+freshCount)
	}

	enforcer, err := events.NewRetentionEnforcer(
		events.WithRetentionStore(rig.eventStore),
		events.WithRetentionPolicies([]events.RetentionPolicy{
			{Type: "", MaxAge: 24 * time.Hour},
		}),
		events.WithRetentionLogger(silentLoggerForIntegration()),
	)
	if err != nil {
		t.Fatalf("NewRetentionEnforcer: %v", err)
	}

	deleted, err := enforcer.RunOnce(ctx)
	if err != nil {
		t.Fatalf("RunOnce: %v", err)
	}
	if deleted != oldCount {
		t.Errorf("deleted = %d, want %d", deleted, oldCount)
	}

	postCount, _ := rig.eventStore.Count(ctx, events.EventQuery{})
	if postCount != freshCount {
		t.Errorf("post-retention count = %d, want %d (fresh only)", postCount, freshCount)
	}
	if enforcer.TotalDeleted() != int64(oldCount) {
		t.Errorf("TotalDeleted = %d, want %d", enforcer.TotalDeleted(), oldCount)
	}
}

// ---- SlowConsumerRedelivers_MaxThreeTimes -------------------------------

// Pins the §4.9 acceptance: slow consumer (handler >ack-timeout)
// triggers redelivery up to 3 times. Handler errors on every
// delivery → Nak with backoff (task 4 schedule: 1s/5s/15s); JS's
// MaxDeliver(=MaxRedeliveries+1) caps the redelivery loop at 4
// total attempts. Test budget ~25s due to the 1+5+15 backoff
// schedule.
func testSlowConsumerRedelivers(t *testing.T) {
	t.Parallel()
	rig := newIntegrationRig(t)
	ctx := context.Background()

	var attempts atomic.Int64
	done := make(chan struct{})
	handler := func(_ context.Context, _ events.Event) error {
		n := attempts.Add(1)
		// Signal once we've seen MaxDeliver (=MaxRedeliveries+1) attempts.
		if n == 4 {
			close(done)
		}
		// Always error so JS Naks and redelivers (until MaxDeliver hits).
		return errors.New("simulated slow / failing consumer")
	}
	sub, err := rig.subscriber.Subscribe(ctx,
		"kscore."+rig.cluster+".events.>",
		handler,
		events.WithMaxRedeliveries(3),
	)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	// Single event — JS will redeliver up to 4 total times per
	// MaxRedeliveries(3).
	e := events.MustNewEvent(events.EventTypeAgentError, "slow-consumer-test")
	if err := rig.publisher.Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	// Total wait budget: backoff schedule is 1s + 5s + 15s before
	// each redelivery (task 4); 4th attempt fires at ~21s. Cap at
	// 30s to allow some slack.
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatalf("attempts = %d, want exactly 4 within 30s", attempts.Load())
	}

	// Allow 2s settle, verify no 5th attempt.
	time.Sleep(2 * time.Second)
	if got := attempts.Load(); got != 4 {
		t.Errorf("attempts = %d, want exactly 4 (1 initial + 3 redeliveries)", got)
	}
}

// ---- helpers -----------------------------------------------------------

// silentLoggerForIntegration returns a slog.Logger that discards
// every record. Used by the retention sub-test so retention-pass
// logs don't drown the integration-test output.
func silentLoggerForIntegration() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
