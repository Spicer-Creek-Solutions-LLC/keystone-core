package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/shawnbutts/keystone-core/internal/events"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// mockEventStore implements events.EventStore for testing.
type mockEventStore struct {
	events     map[string]*events.Event
	queryResult *events.EventQueryResult
	countResult int64
	queryErr   error
	getErr     error
	storeErr   error
}

func newMockEventStore() *mockEventStore {
	return &mockEventStore{events: make(map[string]*events.Event)}
}

func (m *mockEventStore) Store(_ context.Context, event *events.Event) error {
	if m.storeErr != nil {
		return m.storeErr
	}
	m.events[event.ID] = event
	return nil
}

func (m *mockEventStore) StoreBatch(_ context.Context, evts []*events.Event) error {
	for _, e := range evts {
		m.events[e.ID] = e
	}
	return nil
}

func (m *mockEventStore) Get(_ context.Context, id string) (*events.Event, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	evt, ok := m.events[id]
	if !ok {
		return nil, errors.New("event not found")
	}
	return evt, nil
}

func (m *mockEventStore) Query(_ context.Context, _ *events.EventQuery) (*events.EventQueryResult, error) {
	if m.queryErr != nil {
		return nil, m.queryErr
	}
	if m.queryResult != nil {
		return m.queryResult, nil
	}
	return &events.EventQueryResult{Events: []*events.Event{}, TotalCount: 0}, nil
}

func (m *mockEventStore) Delete(_ context.Context, _ string) error { return nil }
func (m *mockEventStore) DeleteBatch(_ context.Context, _ []string) error { return nil }

func (m *mockEventStore) Count(_ context.Context, _ *events.EventQuery) (int64, error) {
	if m.queryErr != nil {
		return 0, m.queryErr
	}
	return m.countResult, nil
}

func (m *mockEventStore) ApplyRetention(_ context.Context, _ *events.RetentionPolicy) (int64, error) {
	return 0, nil
}

func (m *mockEventStore) Close() error { return nil }

// mockEventPublisher implements events.EventPublisher for testing.
type mockEventPublisher struct {
	published []*events.Event
	err       error
}

func (m *mockEventPublisher) Publish(event *events.Event) error {
	if m.err != nil {
		return m.err
	}
	m.published = append(m.published, event)
	return nil
}

func (m *mockEventPublisher) PublishAsync(event *events.Event) error {
	return m.Publish(event)
}

func (m *mockEventPublisher) Close() error { return nil }

// mockEventSubscriber implements events.EventSubscriber for testing.
type mockEventSubscriber struct {
	handler   events.EventHandler
	err       error
	readyCh   chan struct{} // signals when handler has been set
}

func newMockEventSubscriber() *mockEventSubscriber {
	return &mockEventSubscriber{readyCh: make(chan struct{})}
}

func (m *mockEventSubscriber) Subscribe(_ string, handler events.EventHandler) (*events.Subscription, error) {
	if m.err != nil {
		return nil, m.err
	}
	m.handler = handler
	if m.readyCh != nil {
		close(m.readyCh)
	}
	return &events.Subscription{ID: "sub-1", Active: true}, nil
}

func (m *mockEventSubscriber) SubscribeQueue(_ string, _ string, handler events.EventHandler) (*events.Subscription, error) {
	return m.Subscribe("", handler)
}

func (m *mockEventSubscriber) Close() error { return nil }

// mockSubscribeStream implements grpc.ServerStreamingServer[pb.Event] for testing.
type mockSubscribeStream struct {
	grpc.ServerStreamingServer[pb.Event]
	sent   []*pb.Event
	ctx    context.Context
	cancel context.CancelFunc
}

func newMockSubscribeStream() *mockSubscribeStream {
	ctx, cancel := context.WithCancel(context.Background())
	return &mockSubscribeStream{ctx: ctx, cancel: cancel}
}

func (m *mockSubscribeStream) Send(evt *pb.Event) error {
	m.sent = append(m.sent, evt)
	return nil
}

func (m *mockSubscribeStream) Context() context.Context {
	return m.ctx
}

// --- ListEvents tests ---

func TestEventServer_ListEvents_NilStore(t *testing.T) {
	srv := NewEventServer(nil, nil, nil)

	_, err := srv.ListEvents(context.Background(), &pb.ListEventsRequest{})
	if err == nil {
		t.Fatal("expected error for nil store")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestEventServer_ListEvents_Success(t *testing.T) {
	now := time.Now()
	store := newMockEventStore()
	store.queryResult = &events.EventQueryResult{
		Events: []*events.Event{
			{
				ID:       "evt-1",
				Type:     events.EventTypeAgentConnect,
				Source:   "/agents/web-01",
				Time:     now,
				Severity: events.SeverityInfo,
				Tags:     map[string]string{"env": "prod"},
				Data:     map[string]interface{}{"hostname": "web-01"},
			},
		},
		TotalCount: 1,
	}

	srv := NewEventServer(store, nil, nil)
	resp, err := srv.ListEvents(context.Background(), &pb.ListEventsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Events) != 1 {
		t.Fatalf("got %d events, want 1", len(resp.Events))
	}
	if resp.Events[0].Id != "evt-1" {
		t.Errorf("event id = %q, want %q", resp.Events[0].Id, "evt-1")
	}
	if resp.Events[0].Type != "agent.connect" {
		t.Errorf("event type = %q, want %q", resp.Events[0].Type, "agent.connect")
	}
	if resp.Events[0].Severity != pb.EventSeverity_EVENT_SEVERITY_INFO {
		t.Errorf("severity = %v, want INFO", resp.Events[0].Severity)
	}
	if resp.TotalCount != 1 {
		t.Errorf("total_count = %d, want 1", resp.TotalCount)
	}
}

func TestEventServer_ListEvents_QueryError(t *testing.T) {
	store := newMockEventStore()
	store.queryErr = errors.New("db error")

	srv := NewEventServer(store, nil, nil)
	_, err := srv.ListEvents(context.Background(), &pb.ListEventsRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}
}

func TestEventServer_ListEvents_Pagination(t *testing.T) {
	store := newMockEventStore()
	store.queryResult = &events.EventQueryResult{
		Events:     []*events.Event{{ID: "evt-1", Type: "test", Severity: events.SeverityInfo, Tags: map[string]string{}}},
		TotalCount: 50,
	}

	srv := NewEventServer(store, nil, nil)
	resp, err := srv.ListEvents(context.Background(), &pb.ListEventsRequest{PageSize: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.NextPageToken == "" {
		t.Error("expected next_page_token for paginated results")
	}
}

// --- GetEvent tests ---

func TestEventServer_GetEvent_NilStore(t *testing.T) {
	srv := NewEventServer(nil, nil, nil)

	_, err := srv.GetEvent(context.Background(), &pb.GetEventRequest{EventId: "evt-1"})
	if err == nil {
		t.Fatal("expected error for nil store")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestEventServer_GetEvent_EmptyID(t *testing.T) {
	store := newMockEventStore()
	srv := NewEventServer(store, nil, nil)

	_, err := srv.GetEvent(context.Background(), &pb.GetEventRequest{EventId: ""})
	if err == nil {
		t.Fatal("expected error for empty event_id")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestEventServer_GetEvent_Success(t *testing.T) {
	store := newMockEventStore()
	store.events["evt-1"] = &events.Event{
		ID:       "evt-1",
		Type:     events.EventTypeStateChange,
		Source:   "/state-manager",
		Time:     time.Now(),
		Severity: events.SeverityWarning,
		Tags:     map[string]string{},
	}

	srv := NewEventServer(store, nil, nil)
	resp, err := srv.GetEvent(context.Background(), &pb.GetEventRequest{EventId: "evt-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Event.Id != "evt-1" {
		t.Errorf("event id = %q, want %q", resp.Event.Id, "evt-1")
	}
	if resp.Event.Severity != pb.EventSeverity_EVENT_SEVERITY_WARNING {
		t.Errorf("severity = %v, want WARNING", resp.Event.Severity)
	}
}

func TestEventServer_GetEvent_NotFound(t *testing.T) {
	store := newMockEventStore()
	srv := NewEventServer(store, nil, nil)

	_, err := srv.GetEvent(context.Background(), &pb.GetEventRequest{EventId: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing event")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
}

// --- EmitEvent tests ---

func TestEventServer_EmitEvent_NilPublisher(t *testing.T) {
	srv := NewEventServer(nil, nil, nil)

	_, err := srv.EmitEvent(context.Background(), &pb.EmitEventRequest{
		Type:   "test.event",
		Source: "/test",
	})
	if err == nil {
		t.Fatal("expected error for nil publisher")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestEventServer_EmitEvent_MissingType(t *testing.T) {
	pub := &mockEventPublisher{}
	srv := NewEventServer(nil, pub, nil)

	_, err := srv.EmitEvent(context.Background(), &pb.EmitEventRequest{Source: "/test"})
	if err == nil {
		t.Fatal("expected error for missing type")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestEventServer_EmitEvent_MissingSource(t *testing.T) {
	pub := &mockEventPublisher{}
	srv := NewEventServer(nil, pub, nil)

	_, err := srv.EmitEvent(context.Background(), &pb.EmitEventRequest{Type: "test.event"})
	if err == nil {
		t.Fatal("expected error for missing source")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestEventServer_EmitEvent_Success(t *testing.T) {
	pub := &mockEventPublisher{}
	store := newMockEventStore()
	srv := NewEventServer(store, pub, nil)

	data, _ := structpb.NewStruct(map[string]interface{}{"key": "value"})
	resp, err := srv.EmitEvent(context.Background(), &pb.EmitEventRequest{
		Type:          "custom.event",
		Source:        "/test",
		Severity:      pb.EventSeverity_EVENT_SEVERITY_WARNING,
		CorrelationId: "corr-123",
		Tags:          map[string]string{"env": "test"},
		Data:          data,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.EventId == "" {
		t.Error("expected non-empty event_id")
	}
	if resp.Timestamp == nil {
		t.Error("expected timestamp")
	}
	if len(pub.published) != 1 {
		t.Fatalf("got %d published events, want 1", len(pub.published))
	}
	evt := pub.published[0]
	if evt.Type != "custom.event" {
		t.Errorf("type = %q, want %q", evt.Type, "custom.event")
	}
	if evt.Source != "/test" {
		t.Errorf("source = %q, want %q", evt.Source, "/test")
	}
	if evt.Severity != events.SeverityWarning {
		t.Errorf("severity = %q, want %q", evt.Severity, events.SeverityWarning)
	}
	if evt.CorrelationID != "corr-123" {
		t.Errorf("correlation_id = %q, want %q", evt.CorrelationID, "corr-123")
	}
	if evt.Tags["env"] != "test" {
		t.Errorf("tags[env] = %q, want %q", evt.Tags["env"], "test")
	}
	// Should also be stored
	if len(store.events) != 1 {
		t.Errorf("got %d stored events, want 1", len(store.events))
	}
}

func TestEventServer_EmitEvent_PublishError(t *testing.T) {
	pub := &mockEventPublisher{err: errors.New("publish failed")}
	srv := NewEventServer(nil, pub, nil)

	_, err := srv.EmitEvent(context.Background(), &pb.EmitEventRequest{
		Type:   "test.event",
		Source: "/test",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}
}

// --- SubscribeEvents tests ---

func TestEventServer_SubscribeEvents_NilSubscriber(t *testing.T) {
	srv := NewEventServer(nil, nil, nil)
	stream := newMockSubscribeStream()
	defer stream.cancel()

	err := srv.SubscribeEvents(&pb.SubscribeEventsRequest{}, stream)
	if err == nil {
		t.Fatal("expected error for nil subscriber")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestEventServer_SubscribeEvents_SubscribeError(t *testing.T) {
	sub := &mockEventSubscriber{err: errors.New("subscribe failed")}
	srv := NewEventServer(nil, nil, sub)
	stream := newMockSubscribeStream()
	defer stream.cancel()

	err := srv.SubscribeEvents(&pb.SubscribeEventsRequest{}, stream)
	if err == nil {
		t.Fatal("expected error")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}
}

func TestEventServer_SubscribeEvents_StreamsEvents(t *testing.T) {
	sub := newMockEventSubscriber()
	srv := NewEventServer(nil, nil, sub)
	stream := newMockSubscribeStream()

	// Run subscribe in goroutine, cancel after sending an event
	done := make(chan error, 1)
	go func() {
		done <- srv.SubscribeEvents(&pb.SubscribeEventsRequest{}, stream)
	}()

	// Wait for subscribe to set up handler (channel-based, no race)
	<-sub.readyCh

	// Send an event through the handler
	evt := &events.Event{
		ID:       "evt-stream-1",
		Type:     events.EventTypeAgentConnect,
		Source:   "/test",
		Time:     time.Now(),
		Severity: events.SeverityInfo,
		Tags:     map[string]string{},
	}
	if err := sub.handler(evt); err != nil {
		t.Fatalf("handler error: %v", err)
	}

	// Give time for the event to be sent on the stream
	time.Sleep(10 * time.Millisecond)

	// Cancel the stream
	stream.cancel()

	// Wait for subscribe to exit
	<-done

	if len(stream.sent) != 1 {
		t.Fatalf("got %d sent events, want 1", len(stream.sent))
	}
	if stream.sent[0].Id != "evt-stream-1" {
		t.Errorf("sent event id = %q, want %q", stream.sent[0].Id, "evt-stream-1")
	}
}

// --- GetEventTypes tests ---

func TestEventServer_GetEventTypes_All(t *testing.T) {
	srv := NewEventServer(nil, nil, nil)

	resp, err := srv.GetEventTypes(context.Background(), &pb.GetEventTypesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Types) != 29 {
		t.Errorf("got %d event types, want 29", len(resp.Types))
	}
}

func TestEventServer_GetEventTypes_FilterByCategory(t *testing.T) {
	srv := NewEventServer(nil, nil, nil)

	resp, err := srv.GetEventTypes(context.Background(), &pb.GetEventTypesRequest{
		Category: pb.EventCategory_EVENT_CATEGORY_AGENT,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Types) != 5 {
		t.Errorf("got %d agent event types, want 5", len(resp.Types))
	}
	for _, et := range resp.Types {
		if et.Category != pb.EventCategory_EVENT_CATEGORY_AGENT {
			t.Errorf("event type %q has category %v, want AGENT", et.Type, et.Category)
		}
	}
}

func TestEventServer_GetEventTypes_PolicyCategory(t *testing.T) {
	srv := NewEventServer(nil, nil, nil)

	resp, err := srv.GetEventTypes(context.Background(), &pb.GetEventTypesRequest{
		Category: pb.EventCategory_EVENT_CATEGORY_POLICY,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Types) != 2 {
		t.Errorf("got %d policy event types, want 2", len(resp.Types))
	}
}

// --- GetEventStats tests ---

func TestEventServer_GetEventStats_NilStore(t *testing.T) {
	srv := NewEventServer(nil, nil, nil)

	_, err := srv.GetEventStats(context.Background(), &pb.GetEventStatsRequest{})
	if err == nil {
		t.Fatal("expected error for nil store")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestEventServer_GetEventStats_Success(t *testing.T) {
	store := newMockEventStore()
	store.countResult = 42

	srv := NewEventServer(store, nil, nil)
	resp, err := srv.GetEventStats(context.Background(), &pb.GetEventStatsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.TotalEvents != 42 {
		t.Errorf("total_events = %d, want 42", resp.TotalEvents)
	}
}

// --- Severity conversion tests ---

func TestEventSeverityToProto(t *testing.T) {
	tests := []struct {
		input events.Severity
		want  pb.EventSeverity
	}{
		{events.SeverityDebug, pb.EventSeverity_EVENT_SEVERITY_DEBUG},
		{events.SeverityInfo, pb.EventSeverity_EVENT_SEVERITY_INFO},
		{events.SeverityWarning, pb.EventSeverity_EVENT_SEVERITY_WARNING},
		{events.SeverityError, pb.EventSeverity_EVENT_SEVERITY_ERROR},
		{events.SeverityCritical, pb.EventSeverity_EVENT_SEVERITY_CRITICAL},
		{"unknown", pb.EventSeverity_EVENT_SEVERITY_UNSPECIFIED},
	}

	for _, tt := range tests {
		got := eventSeverityToProto(tt.input)
		if got != tt.want {
			t.Errorf("eventSeverityToProto(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestProtoToEventSeverity(t *testing.T) {
	tests := []struct {
		input pb.EventSeverity
		want  events.Severity
	}{
		{pb.EventSeverity_EVENT_SEVERITY_DEBUG, events.SeverityDebug},
		{pb.EventSeverity_EVENT_SEVERITY_INFO, events.SeverityInfo},
		{pb.EventSeverity_EVENT_SEVERITY_WARNING, events.SeverityWarning},
		{pb.EventSeverity_EVENT_SEVERITY_ERROR, events.SeverityError},
		{pb.EventSeverity_EVENT_SEVERITY_CRITICAL, events.SeverityCritical},
		{pb.EventSeverity_EVENT_SEVERITY_UNSPECIFIED, events.SeverityInfo},
	}

	for _, tt := range tests {
		got := protoToEventSeverity(tt.input)
		if got != tt.want {
			t.Errorf("protoToEventSeverity(%v) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSeveritiesAtLeast(t *testing.T) {
	tests := []struct {
		min  events.Severity
		want int
	}{
		{events.SeverityDebug, 5},
		{events.SeverityInfo, 4},
		{events.SeverityWarning, 3},
		{events.SeverityError, 2},
		{events.SeverityCritical, 1},
	}

	for _, tt := range tests {
		got := severitiesAtLeast(tt.min)
		if len(got) != tt.want {
			t.Errorf("severitiesAtLeast(%q) returned %d severities, want %d", tt.min, len(got), tt.want)
		}
	}
}
