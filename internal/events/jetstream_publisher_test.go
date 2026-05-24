// SPDX-License-Identifier: Apache-2.0

package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// --- embedded NATS fixture ---------------------------------------------------

// embeddedJS spins an in-process nats-server with JetStream enabled,
// creates the `KSCORE_EVENTS_<cluster>` stream with the §4.9 subject
// filter, and returns a connected client + JetStreamContext. Tests
// reuse this to exercise the publisher against a real JetStream.
type embeddedJS struct {
	srv     *natsserver.Server
	conn    *nats.Conn
	js      nats.JetStreamContext
	cluster string
	stream  string
}

func newEmbeddedJS(t *testing.T) *embeddedJS {
	t.Helper()

	storeDir := filepath.Join(t.TempDir(), "jetstream")
	opts := &natsserver.Options{
		Host:      "127.0.0.1",
		Port:      freePortForJS(t),
		NoSigs:    true,
		NoLog:     true,
		JetStream: true,
		StoreDir:  storeDir,
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal("embedded NATS not ready")
	}

	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("connect: %v", err)
	}
	js, err := conn.JetStream()
	if err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("jetstream: %v", err)
	}

	const cluster = "test"
	const stream = "KSCORE_EVENTS_test"
	if _, err := js.AddStream(&nats.StreamConfig{
		Name:      stream,
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

	rig := &embeddedJS{srv: srv, conn: conn, js: js, cluster: cluster, stream: stream}
	t.Cleanup(rig.close)
	return rig
}

func (r *embeddedJS) close() {
	if r.conn != nil {
		r.conn.Close()
	}
	if r.srv != nil {
		r.srv.Shutdown()
		r.srv.WaitForShutdown()
	}
}

func freePortForJS(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

// --- sync publish ------------------------------------------------------------

func TestJetStreamPublisher_SyncRoundTrip(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	p := NewJetStreamPublisher(rig.js, rig.cluster)

	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop(ctx) })

	// Subscribe before publishing so we don't race the server.
	in := MustNewEvent(EventTypeAgentConnect, "agent-1")
	in.Tags = map[string]string{"role": "web"}

	expectedSubject := "kscore." + rig.cluster + ".events.agent.connect"
	msgs := make(chan *nats.Msg, 1)
	sub, err := rig.js.Subscribe(expectedSubject, func(m *nats.Msg) {
		msgs <- m
		_ = m.AckSync()
	}, nats.AckExplicit())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	if err := p.Publish(ctx, in); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	select {
	case m := <-msgs:
		if m.Subject != expectedSubject {
			t.Errorf("subject = %q, want %q", m.Subject, expectedSubject)
		}
		var got Event
		if err := json.Unmarshal(m.Data, &got); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if got.ID != in.ID || got.Type != in.Type || got.Source != in.Source {
			t.Errorf("round-trip mismatch:\n got=%+v\nwant=%+v", got, in)
		}
		if got.Tags["role"] != "web" {
			t.Errorf("tags lost: %+v", got.Tags)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive within 2s")
	}
}

func TestJetStreamPublisher_StampsSubjectIfEmpty(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	p := NewJetStreamPublisher(rig.js, rig.cluster)
	ctx := context.Background()
	_ = p.Start(ctx)
	t.Cleanup(func() { _ = p.Stop(ctx) })

	in := MustNewEvent(EventTypeJobStart, "scheduler")
	if in.Subject != "" {
		t.Fatalf("precondition: NewEvent stamped Subject = %q", in.Subject)
	}
	want := "kscore." + rig.cluster + ".events.job.start"

	msgs := make(chan *nats.Msg, 1)
	sub, err := rig.js.Subscribe(want, func(m *nats.Msg) { msgs <- m; _ = m.AckSync() }, nats.AckExplicit())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	if err := p.Publish(ctx, in); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	select {
	case m := <-msgs:
		var got Event
		if err := json.Unmarshal(m.Data, &got); err != nil {
			t.Fatalf("Unmarshal: %v", err)
		}
		if got.Subject != want {
			t.Errorf("published Subject = %q, want %q", got.Subject, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("did not receive within 2s")
	}
}

// --- store integration -------------------------------------------------------

// stubStore is a recording in-memory EventStore for the publisher
// tests. Honors a configurable storeErr to simulate persistence
// failures.
type stubStore struct {
	mu       sync.Mutex
	stored   []Event
	storeErr error
}

func (s *stubStore) Store(_ context.Context, e Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storeErr != nil {
		return s.storeErr
	}
	s.stored = append(s.stored, e)
	return nil
}
func (s *stubStore) StoreBatch(_ context.Context, _ []Event) error { return nil }
func (s *stubStore) Get(_ context.Context, _ string) (Event, error) {
	return Event{}, ErrEventNotFound
}
func (s *stubStore) Query(_ context.Context, _ EventQuery) (EventPage, error) {
	return EventPage{}, nil
}
func (s *stubStore) Count(_ context.Context, _ EventQuery) (int, error) { return 0, nil }
func (s *stubStore) Delete(_ context.Context, _ string) error           { return nil }
func (s *stubStore) ApplyRetention(_ context.Context, _ []RetentionPolicy) (int, error) {
	return 0, nil
}
func (s *stubStore) Close() error { return nil }

func (s *stubStore) snapshot() []Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]Event, len(s.stored))
	copy(out, s.stored)
	return out
}

func TestJetStreamPublisher_StoreFirst_Success(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	store := &stubStore{}
	p := NewJetStreamPublisher(rig.js, rig.cluster, WithStore(store))
	ctx := context.Background()
	_ = p.Start(ctx)
	t.Cleanup(func() { _ = p.Stop(ctx) })

	e := MustNewEvent(EventTypeAgentConnect, "agent-1")
	if err := p.Publish(ctx, e); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	stored := store.snapshot()
	if len(stored) != 1 {
		t.Fatalf("stored len = %d, want 1", len(stored))
	}
	if stored[0].ID != e.ID {
		t.Errorf("stored ID = %q, want %q", stored[0].ID, e.ID)
	}
	// Subject was stamped before store, so it's persisted with the
	// stamped value.
	if stored[0].Subject == "" {
		t.Errorf("stored Subject empty; want stamped")
	}
}

func TestJetStreamPublisher_StoreFailureAbortsPublish(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	storeErr := errors.New("simulated store failure")
	store := &stubStore{storeErr: storeErr}
	p := NewJetStreamPublisher(rig.js, rig.cluster, WithStore(store))
	ctx := context.Background()
	_ = p.Start(ctx)
	t.Cleanup(func() { _ = p.Stop(ctx) })

	// Subscribe so we can assert NATS is NOT touched on store failure.
	msgs := make(chan *nats.Msg, 1)
	sub, err := rig.js.Subscribe("kscore."+rig.cluster+".events.>", func(m *nats.Msg) {
		msgs <- m
		_ = m.AckSync()
	}, nats.AckExplicit())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	e := MustNewEvent(EventTypeAgentConnect, "agent-1")
	err = p.Publish(ctx, e)
	if err == nil {
		t.Fatalf("Publish succeeded; want store error")
	}
	if !errors.Is(err, storeErr) {
		t.Errorf("err = %v; want errors.Is(storeErr)", err)
	}

	// NATS must have received nothing.
	select {
	case m := <-msgs:
		t.Errorf("unexpected NATS publish after store failure: subject=%q", m.Subject)
	case <-time.After(200 * time.Millisecond):
		// expected — no message
	}
}

// --- validation guard --------------------------------------------------------

func TestJetStreamPublisher_RejectsInvalidEvent(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	store := &stubStore{}
	p := NewJetStreamPublisher(rig.js, rig.cluster, WithStore(store))
	ctx := context.Background()
	_ = p.Start(ctx)
	t.Cleanup(func() { _ = p.Stop(ctx) })

	// Zero-value Event fails Validate before store or NATS touched.
	err := p.Publish(ctx, Event{})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("err = %v; want ErrInvalidEvent", err)
	}
	if len(store.snapshot()) != 0 {
		t.Errorf("store received invalid event: %+v", store.snapshot())
	}

	// PublishAsync also rejects synchronously.
	err = p.PublishAsync(ctx, Event{})
	if !errors.Is(err, ErrInvalidEvent) {
		t.Errorf("async err = %v; want ErrInvalidEvent", err)
	}
}

// --- lifecycle guards --------------------------------------------------------

func TestJetStreamPublisher_PublishBeforeStartRejected(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	p := NewJetStreamPublisher(rig.js, rig.cluster)
	e := MustNewEvent(EventTypeAgentConnect, "agent-1")
	if err := p.Publish(context.Background(), e); !errors.Is(err, ErrPublisherNotStarted) {
		t.Errorf("Publish before Start: err = %v, want ErrPublisherNotStarted", err)
	}
	if err := p.PublishAsync(context.Background(), e); !errors.Is(err, ErrPublisherNotStarted) {
		t.Errorf("PublishAsync before Start: err = %v, want ErrPublisherNotStarted", err)
	}
}

func TestJetStreamPublisher_DoubleStartRejected(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	p := NewJetStreamPublisher(rig.js, rig.cluster)
	ctx := context.Background()
	if err := p.Start(ctx); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	t.Cleanup(func() { _ = p.Stop(ctx) })
	if err := p.Start(ctx); err == nil {
		t.Errorf("second Start succeeded; want error")
	}
}

func TestJetStreamPublisher_StopIsIdempotent(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	p := NewJetStreamPublisher(rig.js, rig.cluster)
	ctx := context.Background()

	// Stop before Start: no-op.
	if err := p.Stop(ctx); err != nil {
		t.Errorf("Stop before Start: %v", err)
	}
	if err := p.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Errorf("first Stop: %v", err)
	}
	if err := p.Stop(ctx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

// --- async pipeline ----------------------------------------------------------

func TestJetStreamPublisher_AsyncDrains(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	p := NewJetStreamPublisher(rig.js, rig.cluster)
	ctx := context.Background()
	_ = p.Start(ctx)
	t.Cleanup(func() { _ = p.Stop(ctx) })

	subject := "kscore." + rig.cluster + ".events.>"
	const n = 20
	var received atomic.Int64
	gotAll := make(chan struct{})
	sub, err := rig.js.Subscribe(subject, func(m *nats.Msg) {
		_ = m.AckSync()
		if received.Add(1) == int64(n) {
			close(gotAll)
		}
	}, nats.AckExplicit())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	for i := 0; i < n; i++ {
		e := MustNewEvent(EventTypeAgentHeartbeat, fmt.Sprintf("agent-%d", i))
		if err := p.PublishAsync(ctx, e); err != nil {
			t.Fatalf("PublishAsync[%d]: %v", i, err)
		}
	}

	select {
	case <-gotAll:
	case <-time.After(5 * time.Second):
		t.Fatalf("only received %d/%d within 5s", received.Load(), n)
	}
	if p.FailedPublishes() != 0 {
		t.Errorf("FailedPublishes = %d, want 0", p.FailedPublishes())
	}
}

// --- stub JetStreamContext for deterministic backpressure / failure tests ----

// blockingJS embeds nats.JetStreamContext so we can override PublishMsg
// without implementing all 20+ unused methods. The worker's publishOne
// path only calls PublishMsg; methods we don't override would panic if
// called, surfacing a test bug as a clear panic rather than a silent
// nil-deref.
type blockingJS struct {
	nats.JetStreamContext
	block chan struct{} // closed → publish returns; never closed → publish hangs
	fail  bool          // when true, publish returns an error after block fires
}

func (s *blockingJS) PublishMsg(_ *nats.Msg, _ ...nats.PubOpt) (*nats.PubAck, error) {
	<-s.block
	if s.fail {
		return nil, errors.New("stub publish failure")
	}
	return &nats.PubAck{}, nil
}

func TestJetStreamPublisher_AsyncBufferFullTimesOut(t *testing.T) {
	t.Parallel()
	stub := &blockingJS{block: make(chan struct{})}
	p := NewJetStreamPublisher(stub, "test",
		WithBufferSize(1),
		WithFlushTimeout(50*time.Millisecond),
	)
	ctx := context.Background()
	_ = p.Start(ctx)
	t.Cleanup(func() {
		close(stub.block) // release worker so Stop completes
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = p.Stop(stopCtx)
	})

	// Event 1: worker takes it, blocks in stub.PublishMsg.
	if err := p.PublishAsync(ctx, MustNewEvent(EventTypeAgentConnect, "a")); err != nil {
		t.Fatalf("PublishAsync[1]: %v", err)
	}
	// Wait until the worker has picked up event 1 (otherwise it
	// might still be in the queue, masking the bufferSize=1 fill).
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) && len(p.queue) > 0 {
		time.Sleep(5 * time.Millisecond)
	}

	// Event 2: fills the 1-slot buffer.
	if err := p.PublishAsync(ctx, MustNewEvent(EventTypeAgentConnect, "b")); err != nil {
		t.Fatalf("PublishAsync[2]: %v", err)
	}

	// Event 3: buffer is full, worker is blocked → timeout fires.
	err := p.PublishAsync(ctx, MustNewEvent(EventTypeAgentConnect, "c"))
	if !errors.Is(err, ErrPublisherBufferFull) {
		t.Errorf("PublishAsync[3] err = %v, want ErrPublisherBufferFull", err)
	}
}

func TestJetStreamPublisher_FailedPublishesCounterAndCallback(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	close(block) // immediately released
	stub := &blockingJS{block: block, fail: true}

	var (
		cbMu     sync.Mutex
		cbEvents []Event
		cbErrs   []error
	)
	cb := func(e Event, err error) {
		cbMu.Lock()
		defer cbMu.Unlock()
		cbEvents = append(cbEvents, e)
		cbErrs = append(cbErrs, err)
	}

	p := NewJetStreamPublisher(stub, "test",
		WithBufferSize(16),
		WithFlushTimeout(50*time.Millisecond),
		WithAsyncErrorCallback(cb),
	)
	ctx := context.Background()
	_ = p.Start(ctx)
	t.Cleanup(func() { _ = p.Stop(ctx) })

	const n = 5
	for i := 0; i < n; i++ {
		if err := p.PublishAsync(ctx, MustNewEvent(EventTypeAgentError, fmt.Sprintf("a-%d", i))); err != nil {
			t.Fatalf("PublishAsync[%d]: %v", i, err)
		}
	}

	// Wait for the counter to reach n.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && p.FailedPublishes() < int64(n) {
		time.Sleep(20 * time.Millisecond)
	}
	if got := p.FailedPublishes(); got != int64(n) {
		t.Fatalf("FailedPublishes = %d, want %d", got, n)
	}

	cbMu.Lock()
	defer cbMu.Unlock()
	if len(cbEvents) != n {
		t.Errorf("callback fired %d times, want %d", len(cbEvents), n)
	}
	for i, err := range cbErrs {
		if err == nil {
			t.Errorf("callback err[%d] = nil; want non-nil", i)
		}
	}
}

func TestJetStreamPublisher_AsyncFailureFallsBackToLogWhenCallbackNil(t *testing.T) {
	t.Parallel()
	block := make(chan struct{})
	close(block)
	stub := &blockingJS{block: block, fail: true}

	p := NewJetStreamPublisher(stub, "test",
		WithBufferSize(8),
		WithFlushTimeout(50*time.Millisecond),
		// no callback — falls back to slog
	)
	ctx := context.Background()
	_ = p.Start(ctx)
	t.Cleanup(func() { _ = p.Stop(ctx) })

	for i := 0; i < 3; i++ {
		if err := p.PublishAsync(ctx, MustNewEvent(EventTypeAgentError, "x")); err != nil {
			t.Fatalf("PublishAsync[%d]: %v", i, err)
		}
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && p.FailedPublishes() < 3 {
		time.Sleep(20 * time.Millisecond)
	}
	if p.FailedPublishes() != 3 {
		t.Errorf("FailedPublishes = %d, want 3", p.FailedPublishes())
	}
}

func TestJetStreamPublisher_StopDrainsPendingEvents(t *testing.T) {
	t.Parallel()
	rig := newEmbeddedJS(t)
	p := NewJetStreamPublisher(rig.js, rig.cluster, WithBufferSize(64))
	ctx := context.Background()
	_ = p.Start(ctx)

	subject := "kscore." + rig.cluster + ".events.>"
	var received atomic.Int64
	sub, err := rig.js.Subscribe(subject, func(m *nats.Msg) {
		_ = m.AckSync()
		received.Add(1)
	}, nats.AckExplicit())
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	const n = 10
	for i := 0; i < n; i++ {
		if err := p.PublishAsync(ctx, MustNewEvent(EventTypeAgentHeartbeat, "src")); err != nil {
			t.Fatalf("PublishAsync[%d]: %v", i, err)
		}
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := p.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && received.Load() < int64(n) {
		time.Sleep(50 * time.Millisecond)
	}
	if got := received.Load(); got != int64(n) {
		t.Errorf("received = %d, want %d", got, n)
	}
}
