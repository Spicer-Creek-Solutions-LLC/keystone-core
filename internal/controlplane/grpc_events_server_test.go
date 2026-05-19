package controlplane

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// eventsTestRig wires a real EventStore backed by an in-memory SQLite
// state.Store plus stub publisher/subscriber. The stubs let us assert
// what the gRPC adapter passes through without spinning up NATS.
type eventsTestRig struct {
	server *EventsGRPCServer
	store  events.EventStore
	pub    *stubPublisher
	sub    *stubSubscriber
}

func newEventsRig(t *testing.T) *eventsTestRig {
	t.Helper()
	stateStore, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "events.db")},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = stateStore.Close() })

	store := events.NewSQLEventStore(stateStore)
	pub := &stubPublisher{}
	sub := &stubSubscriber{}
	return &eventsTestRig{
		server: NewEventsGRPCServer(store, pub, sub),
		store:  store,
		pub:    pub,
		sub:    sub,
	}
}

// stubPublisher records every Publish + PublishAsync call.
type stubPublisher struct {
	started   bool
	published []events.Event
	publishFn func(events.Event) error // optional override
}

func (s *stubPublisher) Start(context.Context) error { s.started = true; return nil }
func (s *stubPublisher) Publish(_ context.Context, e events.Event) error {
	if s.publishFn != nil {
		return s.publishFn(e)
	}
	s.published = append(s.published, e)
	return nil
}
func (s *stubPublisher) PublishAsync(ctx context.Context, e events.Event) error {
	return s.Publish(ctx, e)
}
func (s *stubPublisher) Stop(context.Context) error { return nil }

// stubSubscriber records Subscribe calls + lets tests drive
// handler invocations via the captured handler. All fields are
// mutex-guarded so the test goroutine + the SubscribeEvents
// goroutine race cleanly.
type stubSubscriber struct {
	mu              sync.Mutex
	subscribeCalled int
	lastPattern     string
	lastOpts        []events.SubscribeOption
	handler         events.EventHandler
	closed          bool
}

func (s *stubSubscriber) Start(context.Context) error { return nil }
func (s *stubSubscriber) Subscribe(_ context.Context, pattern string, h events.EventHandler, opts ...events.SubscribeOption) (events.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.subscribeCalled++
	s.lastPattern = pattern
	s.lastOpts = opts
	s.handler = h
	return &stubSubscription{parent: s}, nil
}
func (s *stubSubscriber) Stop(context.Context) error { return nil }

func (s *stubSubscriber) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.subscribeCalled
}
func (s *stubSubscriber) currentHandler() events.EventHandler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handler
}
func (s *stubSubscriber) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

type stubSubscription struct {
	parent       *stubSubscriber
	unsubscribed bool
}

func (s *stubSubscription) Unsubscribe() error {
	s.parent.mu.Lock()
	defer s.parent.mu.Unlock()
	s.unsubscribed = true
	s.parent.closed = true
	return nil
}
func (s *stubSubscription) Pending() (uint64, error) { return 0, nil }

// mockServerStream is a minimal grpc.ServerStream impl for testing
// SubscribeEvents without a real gRPC connection.
type mockServerStream struct {
	grpc.ServerStream
	ctx      context.Context
	received []*v1.SubscribeEventsResponse
	sendErr  error
}

func newMockServerStream(ctx context.Context) *mockServerStream {
	return &mockServerStream{ctx: ctx}
}

func (m *mockServerStream) Context() context.Context { return m.ctx }
func (m *mockServerStream) Send(resp *v1.SubscribeEventsResponse) error {
	if m.sendErr != nil {
		return m.sendErr
	}
	m.received = append(m.received, resp)
	return nil
}
func (m *mockServerStream) SetHeader(metadata.MD) error  { return nil }
func (m *mockServerStream) SendHeader(metadata.MD) error { return nil }
func (m *mockServerStream) SetTrailer(metadata.MD)       {}

// ---- ListEvents -----------------------------------------------------------

func TestEventsGRPC_ListEvents_Empty(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	resp, err := r.server.ListEvents(context.Background(), &v1.ListEventsRequest{})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(resp.GetEvents()) != 0 {
		t.Errorf("len = %d, want 0", len(resp.GetEvents()))
	}
}

func TestEventsGRPC_ListEvents_FilterByType(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	ctx := context.Background()
	for _, typ := range []events.EventType{
		events.EventTypeAgentConnect,
		events.EventTypeJobStart,
		events.EventTypeAgentConnect,
	} {
		e := events.MustNewEvent(typ, "src")
		if err := r.store.Store(ctx, e); err != nil {
			t.Fatalf("seed: %v", err)
		}
	}

	resp, err := r.server.ListEvents(ctx, &v1.ListEventsRequest{
		Filter: &v1.EventFilter{Type: "agent.connect"},
	})
	if err != nil {
		t.Fatalf("ListEvents: %v", err)
	}
	if len(resp.GetEvents()) != 2 {
		t.Errorf("len = %d, want 2", len(resp.GetEvents()))
	}
	for _, e := range resp.GetEvents() {
		if e.GetType() != "agent.connect" {
			t.Errorf("type = %q", e.GetType())
		}
	}
}

func TestEventsGRPC_ListEvents_FilterByCategory(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	ctx := context.Background()
	for _, typ := range []events.EventType{
		events.EventTypeAgentConnect, events.EventTypeAgentHeartbeat, events.EventTypeJobStart,
	} {
		_ = r.store.Store(ctx, events.MustNewEvent(typ, "src"))
	}
	resp, _ := r.server.ListEvents(ctx, &v1.ListEventsRequest{
		Filter: &v1.EventFilter{Categories: []string{"agent"}},
	})
	if len(resp.GetEvents()) != 2 {
		t.Errorf("agent fan-in: %d events, want 2", len(resp.GetEvents()))
	}
}

func TestEventsGRPC_ListEvents_MutuallyExclusiveTypeAndCategory(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	_, err := r.server.ListEvents(context.Background(), &v1.ListEventsRequest{
		Filter: &v1.EventFilter{Type: "agent.connect", Categories: []string{"agent"}},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestEventsGRPC_ListEvents_NilStore(t *testing.T) {
	t.Parallel()
	server := NewEventsGRPCServer(nil, nil, nil)
	_, err := server.ListEvents(context.Background(), &v1.ListEventsRequest{})
	if status.Code(err) != codes.Unavailable {
		t.Errorf("code = %v, want Unavailable", status.Code(err))
	}
}

// ---- GetEvent -------------------------------------------------------------

func TestEventsGRPC_GetEvent_RoundTrip(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	ctx := context.Background()
	in := events.MustNewEvent(events.EventTypeAgentConnect, "agent-1")
	in.Tags = map[string]string{"role": "web"}
	in.Data = map[string]any{"k": "v"}
	if err := r.store.Store(ctx, in); err != nil {
		t.Fatalf("seed: %v", err)
	}

	resp, err := r.server.GetEvent(ctx, &v1.GetEventRequest{EventId: in.ID})
	if err != nil {
		t.Fatalf("GetEvent: %v", err)
	}
	if resp.GetEvent().GetId() != in.ID {
		t.Errorf("id = %q, want %q", resp.GetEvent().GetId(), in.ID)
	}
	if resp.GetEvent().GetTags()["role"] != "web" {
		t.Errorf("tags lost: %v", resp.GetEvent().GetTags())
	}
	if resp.GetEvent().GetData().AsMap()["k"] != "v" {
		t.Errorf("data lost: %v", resp.GetEvent().GetData().AsMap())
	}
}

func TestEventsGRPC_GetEvent_NotFound(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	_, err := r.server.GetEvent(context.Background(), &v1.GetEventRequest{EventId: "ghost"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("code = %v, want NotFound", status.Code(err))
	}
}

func TestEventsGRPC_GetEvent_EmptyID(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	_, err := r.server.GetEvent(context.Background(), &v1.GetEventRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

// ---- EmitEvent ------------------------------------------------------------

func TestEventsGRPC_EmitEvent_StampsID(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	resp, err := r.server.EmitEvent(context.Background(), &v1.EmitEventRequest{
		Event: &v1.Event{
			Type:     "agent.connect",
			Source:   "agent-1",
			Severity: v1.EventSeverity_EVENT_SEVERITY_INFO,
		},
	})
	if err != nil {
		t.Fatalf("EmitEvent: %v", err)
	}
	if resp.GetEventId() == "" {
		t.Errorf("EventId empty")
	}
	if len(r.pub.published) != 1 {
		t.Fatalf("publisher got %d events, want 1", len(r.pub.published))
	}
	got := r.pub.published[0]
	if got.ID != resp.GetEventId() {
		t.Errorf("published ID %q != response ID %q", got.ID, resp.GetEventId())
	}
	if got.Type != events.EventTypeAgentConnect {
		t.Errorf("type = %s", got.Type)
	}
}

func TestEventsGRPC_EmitEvent_PreservesProvidedFields(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	customTime := time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC)
	data, _ := structpb.NewStruct(map[string]any{"latency_ms": 12.5})
	_, err := r.server.EmitEvent(context.Background(), &v1.EmitEventRequest{
		Event: &v1.Event{
			Id:            "explicit-id",
			Type:          "job.start",
			Source:        "scheduler",
			Severity:      v1.EventSeverity_EVENT_SEVERITY_WARN,
			Time:          timestamppb.New(customTime),
			CorrelationId: "req-42",
			Tags:          map[string]string{"role": "worker"},
			Data:          data,
		},
	})
	if err != nil {
		t.Fatalf("EmitEvent: %v", err)
	}
	got := r.pub.published[0]
	if got.ID != "explicit-id" {
		t.Errorf("explicit ID overridden: %q", got.ID)
	}
	if !got.Time.Equal(customTime) {
		t.Errorf("time changed: %v vs %v", got.Time, customTime)
	}
	if got.Severity != events.SeverityWarn {
		t.Errorf("severity = %s", got.Severity)
	}
	if got.CorrelationID != "req-42" {
		t.Errorf("correlation = %q", got.CorrelationID)
	}
	if got.Tags["role"] != "worker" {
		t.Errorf("tags lost")
	}
	if got.Data["latency_ms"] != 12.5 {
		t.Errorf("data lost: %v", got.Data)
	}
}

func TestEventsGRPC_EmitEvent_BadType(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	_, err := r.server.EmitEvent(context.Background(), &v1.EmitEventRequest{
		Event: &v1.Event{Type: "bogus", Source: "x", Severity: v1.EventSeverity_EVENT_SEVERITY_INFO},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestEventsGRPC_EmitEvent_NilPublisher(t *testing.T) {
	t.Parallel()
	server := NewEventsGRPCServer(nil, nil, nil)
	_, err := server.EmitEvent(context.Background(), &v1.EmitEventRequest{
		Event: &v1.Event{Type: "agent.connect", Source: "x"},
	})
	if status.Code(err) != codes.Unavailable {
		t.Errorf("code = %v, want Unavailable", status.Code(err))
	}
}

func TestEventsGRPC_EmitEvent_PublisherError(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	r.pub.publishFn = func(_ events.Event) error {
		return events.ErrPublisherNotStarted
	}
	_, err := r.server.EmitEvent(context.Background(), &v1.EmitEventRequest{
		Event: &v1.Event{Type: "agent.connect", Source: "x", Severity: v1.EventSeverity_EVENT_SEVERITY_INFO},
	})
	if status.Code(err) != codes.Unavailable {
		t.Errorf("code = %v, want Unavailable", status.Code(err))
	}
}

// ---- SubscribeEvents ------------------------------------------------------

func TestEventsGRPC_SubscribeEvents_Subscribes(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockServerStream(ctx)

	// Cancel after Subscribe returns so the handler blocks then exits.
	done := make(chan error, 1)
	go func() {
		done <- r.server.SubscribeEvents(&v1.SubscribeEventsRequest{
			Filter:      &v1.EventFilter{Categories: []string{"agent"}},
			QueueGroup:  "workers",
			ReplaySeconds: 30,
		}, stream)
	}()

	// Give Subscribe time to register.
	deadline := time.After(2 * time.Second)
	for r.sub.calls() == 0 {
		select {
		case <-deadline:
			cancel()
			t.Fatalf("Subscribe never called on stub")
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("SubscribeEvents err = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after cancel")
	}
	if !r.sub.isClosed() {
		t.Errorf("subscription not unsubscribed after handler returned")
	}
}

func TestEventsGRPC_SubscribeEvents_BadFilterExpression(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	ctx := context.Background()
	stream := newMockServerStream(ctx)
	err := r.server.SubscribeEvents(&v1.SubscribeEventsRequest{
		FilterExpression: "((malformed",
	}, stream)
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", status.Code(err))
	}
	if r.sub.calls() != 0 {
		t.Errorf("Subscribe called despite filter compile failure")
	}
}

func TestEventsGRPC_SubscribeEvents_HandlerSends(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	stream := newMockServerStream(ctx)

	done := make(chan error, 1)
	go func() {
		done <- r.server.SubscribeEvents(&v1.SubscribeEventsRequest{}, stream)
	}()

	// Wait for Subscribe to register the handler.
	deadline := time.After(2 * time.Second)
	var h events.EventHandler
	for {
		h = r.sub.currentHandler()
		if h != nil {
			break
		}
		select {
		case <-deadline:
			cancel()
			t.Fatalf("handler never installed")
		case <-time.After(10 * time.Millisecond):
		}
	}
	// Drive the handler with an event.
	in := events.MustNewEvent(events.EventTypeAgentConnect, "agent-1")
	if err := h(ctx, in); err != nil {
		t.Errorf("handler returned err: %v", err)
	}
	// Give the goroutine a moment to flush.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if len(stream.received) != 1 {
		t.Fatalf("received = %d, want 1", len(stream.received))
	}
	if stream.received[0].GetEvent().GetId() != in.ID {
		t.Errorf("received wrong event")
	}
}

func TestEventsGRPC_SubscribeEvents_NilSubscriber(t *testing.T) {
	t.Parallel()
	server := NewEventsGRPCServer(nil, nil, nil)
	stream := newMockServerStream(context.Background())
	err := server.SubscribeEvents(&v1.SubscribeEventsRequest{}, stream)
	if status.Code(err) != codes.Unavailable {
		t.Errorf("code = %v, want Unavailable", status.Code(err))
	}
}

// ---- GetEventTypes --------------------------------------------------------

func TestEventsGRPC_GetEventTypes(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	resp, err := r.server.GetEventTypes(context.Background(), &v1.GetEventTypesRequest{})
	if err != nil {
		t.Fatalf("GetEventTypes: %v", err)
	}
	if len(resp.GetTypes()) != 29 {
		t.Errorf("len = %d, want 29 (§4.9 canonical taxonomy)", len(resp.GetTypes()))
	}
}

// ---- GetEventStats --------------------------------------------------------

func TestEventsGRPC_GetEventStats(t *testing.T) {
	t.Parallel()
	r := newEventsRig(t)
	ctx := context.Background()
	// 3 agent.connect at INFO, 2 job.start at WARN, 1 system.error at CRITICAL.
	mix := []struct {
		typ events.EventType
		sev events.Severity
		n   int
	}{
		{events.EventTypeAgentConnect, events.SeverityInfo, 3},
		{events.EventTypeJobStart, events.SeverityWarn, 2},
		{events.EventTypeSystemError, events.SeverityCritical, 1},
	}
	for _, m := range mix {
		for i := 0; i < m.n; i++ {
			e := events.MustNewEvent(m.typ, "src")
			e.Severity = m.sev
			if err := r.store.Store(ctx, e); err != nil {
				t.Fatalf("seed: %v", err)
			}
		}
	}

	resp, err := r.server.GetEventStats(ctx, &v1.GetEventStatsRequest{})
	if err != nil {
		t.Fatalf("GetEventStats: %v", err)
	}
	if resp.GetTotal() != 6 {
		t.Errorf("total = %d, want 6", resp.GetTotal())
	}
	if resp.GetByType()["agent.connect"] != 3 {
		t.Errorf("agent.connect = %d, want 3", resp.GetByType()["agent.connect"])
	}
	if resp.GetByType()["job.start"] != 2 {
		t.Errorf("job.start = %d, want 2", resp.GetByType()["job.start"])
	}
	if resp.GetBySeverity()["info"] != 3 {
		t.Errorf("info count = %d, want 3", resp.GetBySeverity()["info"])
	}
	if resp.GetBySeverity()["warn"] != 2 {
		t.Errorf("warn count = %d, want 2", resp.GetBySeverity()["warn"])
	}
	if resp.GetBySeverity()["critical"] != 1 {
		t.Errorf("critical count = %d, want 1", resp.GetBySeverity()["critical"])
	}
}

// ---- error mapping --------------------------------------------------------

func TestEventsErrToStatus_Mapping(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want codes.Code
	}{
		{state.ErrNotFound, codes.NotFound},
		{events.ErrEventNotFound, codes.NotFound},
		{events.ErrInvalidEvent, codes.InvalidArgument},
		{events.ErrInvalidFilter, codes.InvalidArgument},
		{events.ErrPublisherNotStarted, codes.Unavailable},
		{events.ErrSubscriberNotStarted, codes.Unavailable},
		{events.ErrPublisherBufferFull, codes.ResourceExhausted},
		{errors.New("generic"), codes.Internal},
	}
	for _, c := range cases {
		got := status.Code(eventsErrToStatus(c.err))
		if got != c.want {
			t.Errorf("err=%v got code %v, want %v", c.err, got, c.want)
		}
	}
}
