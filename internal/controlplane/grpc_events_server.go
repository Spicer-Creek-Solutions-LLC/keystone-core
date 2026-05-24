// SPDX-License-Identifier: Apache-2.0

// Package controlplane Epic 11 task 6 — EventService gRPC adapter.
//
// EventsGRPCServer implements [v1.EventServiceServer] by delegating
// to:
//
//   - [events.EventStore]      — list / get / count / delete.
//   - [events.EventPublisher]  — emit (sync publish through the
//     v1.0 store-first pipeline; NATS ack is the caller's signal).
//   - [events.EventSubscriber] — SubscribeEvents server-streaming
//     RPC; per-stream subscription with optional CEL filter +
//     replay + queue-group.
//
// Any of the three may be nil when the corresponding feature is not
// configured — affected RPCs return codes.Unavailable. The boot
// wiring in cmd/kscore-server/events.go always supplies at least
// the store; publisher / subscriber are gated on
// config.EventsConfig.Publisher.Enabled / .Subscriber.Enabled.

package controlplane

import (
	"context"
	"errors"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// EventsGRPCServer is the gRPC adapter for the §4.9 EventService.
type EventsGRPCServer struct {
	v1.UnimplementedEventServiceServer

	Store      events.EventStore
	Publisher  events.EventPublisher
	Subscriber events.EventSubscriber
}

// Compile-time interface check.
var _ v1.EventServiceServer = (*EventsGRPCServer)(nil)

// NewEventsGRPCServer wires the gRPC adapter. Pass nil for components
// that are not configured.
func NewEventsGRPCServer(store events.EventStore, pub events.EventPublisher, sub events.EventSubscriber) *EventsGRPCServer {
	return &EventsGRPCServer{Store: store, Publisher: pub, Subscriber: sub}
}

// ---- ListEvents -----------------------------------------------------------

func (s *EventsGRPCServer) ListEvents(ctx context.Context, req *v1.ListEventsRequest) (*v1.ListEventsResponse, error) {
	if s.Store == nil {
		return nil, status.Error(codes.Unavailable, "events store not configured")
	}
	q, err := eventQueryFromProto(req.GetFilter(), req.GetPageToken(), int(req.GetPageSize()), req.GetDescending())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	page, err := s.Store.Query(ctx, q)
	if err != nil {
		return nil, eventsErrToStatus(err)
	}
	resp := &v1.ListEventsResponse{
		Events:        make([]*v1.Event, 0, len(page.Events)),
		NextPageToken: page.NextCursor,
	}
	for i := range page.Events {
		pb, err := eventToProto(page.Events[i])
		if err != nil {
			return nil, status.Errorf(codes.Internal, "marshal event %s: %v", page.Events[i].ID, err)
		}
		resp.Events = append(resp.Events, pb)
	}
	return resp, nil
}

// ---- GetEvent -------------------------------------------------------------

func (s *EventsGRPCServer) GetEvent(ctx context.Context, req *v1.GetEventRequest) (*v1.GetEventResponse, error) {
	if s.Store == nil {
		return nil, status.Error(codes.Unavailable, "events store not configured")
	}
	if req.GetEventId() == "" {
		return nil, status.Error(codes.InvalidArgument, "event_id is required")
	}
	e, err := s.Store.Get(ctx, req.GetEventId())
	if err != nil {
		return nil, eventsErrToStatus(err)
	}
	pb, err := eventToProto(e)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal event %s: %v", e.ID, err)
	}
	return &v1.GetEventResponse{Event: pb}, nil
}

// ---- EmitEvent ------------------------------------------------------------

func (s *EventsGRPCServer) EmitEvent(ctx context.Context, req *v1.EmitEventRequest) (*v1.EmitEventResponse, error) {
	if s.Publisher == nil {
		return nil, status.Error(codes.Unavailable, "events publisher not configured")
	}
	if req.GetEvent() == nil {
		return nil, status.Error(codes.InvalidArgument, "event is required")
	}
	e, err := eventFromProto(req.GetEvent())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	// Server-side defaults for fields the caller may have left empty.
	if e.ID == "" {
		stamped, err := events.NewEvent(e.Type, e.Source)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		// Keep caller's severity / tags / data / correlation_id /
		// subject if set; only fill ID + Time when missing.
		stamped.Severity = e.Severity
		stamped.CorrelationID = e.CorrelationID
		stamped.Tags = e.Tags
		stamped.Data = e.Data
		stamped.Subject = e.Subject
		if !e.Time.IsZero() {
			stamped.Time = e.Time
		}
		e = stamped
	}
	if err := s.Publisher.Publish(ctx, e); err != nil {
		return nil, eventsErrToStatus(err)
	}
	return &v1.EmitEventResponse{EventId: e.ID}, nil
}

// ---- SubscribeEvents ------------------------------------------------------

func (s *EventsGRPCServer) SubscribeEvents(req *v1.SubscribeEventsRequest, stream v1.EventService_SubscribeEventsServer) error {
	if s.Subscriber == nil {
		return status.Error(codes.Unavailable, "events subscriber not configured")
	}

	pattern, err := subjectPatternFromFilter(req.GetFilter())
	if err != nil {
		return status.Error(codes.InvalidArgument, err.Error())
	}

	var opts []events.SubscribeOption
	if exp := req.GetFilterExpression(); exp != "" {
		f, err := events.CompileFilter(exp)
		if err != nil {
			return status.Errorf(codes.InvalidArgument, "filter_expression: %v", err)
		}
		opts = append(opts, events.WithFilter(f.Match))
	}
	// Predicate covering the structural-filter fields CEL doesn't:
	// type / categories / source / severity / correlation_id / tags /
	// since / until. Subject pattern handles type/category fan-in;
	// the predicate enforces the rest.
	if pred := predicateFromFilter(req.GetFilter()); pred != nil {
		opts = append(opts, events.WithFilter(pred))
	}
	if r := req.GetReplaySeconds(); r > 0 {
		opts = append(opts, events.WithReplay(time.Duration(r)*time.Second))
	}
	if g := req.GetQueueGroup(); g != "" {
		opts = append(opts, events.WithQueueGroup(g))
	}

	streamCtx := stream.Context()
	handler := func(_ context.Context, e events.Event) error {
		pb, err := eventToProto(e)
		if err != nil {
			return fmt.Errorf("marshal: %w", err)
		}
		if err := stream.Send(&v1.SubscribeEventsResponse{Event: pb}); err != nil {
			// Client gone — the gRPC machinery sets streamCtx.Done().
			return err
		}
		return nil
	}

	sub, err := s.Subscriber.Subscribe(streamCtx, pattern, handler, opts...)
	if err != nil {
		return eventsErrToStatus(err)
	}
	defer func() { _ = sub.Unsubscribe() }()

	// Block until the client disconnects or the server stops.
	<-streamCtx.Done()
	return nil
}

// ---- GetEventTypes --------------------------------------------------------

func (s *EventsGRPCServer) GetEventTypes(_ context.Context, _ *v1.GetEventTypesRequest) (*v1.GetEventTypesResponse, error) {
	canon := events.CanonicalEventTypes()
	out := make([]string, len(canon))
	for i, t := range canon {
		out[i] = string(t)
	}
	return &v1.GetEventTypesResponse{Types: out}, nil
}

// ---- GetEventStats --------------------------------------------------------

func (s *EventsGRPCServer) GetEventStats(ctx context.Context, req *v1.GetEventStatsRequest) (*v1.GetEventStatsResponse, error) {
	if s.Store == nil {
		return nil, status.Error(codes.Unavailable, "events store not configured")
	}
	baseQuery := events.EventQuery{
		Since: tsToTime(req.GetSince()),
		Until: tsToTime(req.GetUntil()),
	}

	total, err := s.Store.Count(ctx, baseQuery)
	if err != nil {
		return nil, eventsErrToStatus(err)
	}

	byType := make(map[string]int64, len(events.CanonicalEventTypes()))
	for _, t := range events.CanonicalEventTypes() {
		q := baseQuery
		q.Type = t
		n, err := s.Store.Count(ctx, q)
		if err != nil {
			return nil, eventsErrToStatus(err)
		}
		if n > 0 {
			byType[string(t)] = int64(n)
		}
	}

	bySeverity := make(map[string]int64, len(events.AllSeverities()))
	for _, sev := range events.AllSeverities() {
		q := baseQuery
		q.MinSeverity = sev
		atLeast, err := s.Store.Count(ctx, q)
		if err != nil {
			return nil, eventsErrToStatus(err)
		}
		// Convert "at-least" counts into per-level counts by
		// subtracting the next-higher band. AllSeverities is in
		// ascending order; for the highest (Critical) the count is
		// simply atLeast.
		nextIdx := sevIndex(sev) + 1
		if nextIdx < len(events.AllSeverities()) {
			nextQ := baseQuery
			nextQ.MinSeverity = events.AllSeverities()[nextIdx]
			nextN, err := s.Store.Count(ctx, nextQ)
			if err != nil {
				return nil, eventsErrToStatus(err)
			}
			atLeast -= nextN
		}
		if atLeast > 0 {
			bySeverity[sev.String()] = int64(atLeast)
		}
	}

	return &v1.GetEventStatsResponse{
		ByType:     byType,
		BySeverity: bySeverity,
		Total:      int64(total),
	}, nil
}

func sevIndex(s events.Severity) int {
	for i, lvl := range events.AllSeverities() {
		if lvl == s {
			return i
		}
	}
	return -1
}

// ---- proto ↔ Go translation -----------------------------------------------

// eventToProto marshals an events.Event into the wire shape.
func eventToProto(e events.Event) (*v1.Event, error) {
	pb := &v1.Event{
		Id:            e.ID,
		Type:          string(e.Type),
		Source:        e.Source,
		Severity:      v1.EventSeverity(e.Severity),
		CorrelationId: e.CorrelationID,
		Tags:          e.Tags,
		Subject:       e.Subject,
	}
	if !e.Time.IsZero() {
		pb.Time = timestamppb.New(e.Time.UTC())
	}
	if len(e.Data) > 0 {
		st, err := structpb.NewStruct(e.Data)
		if err != nil {
			return nil, fmt.Errorf("data → struct: %w", err)
		}
		pb.Data = st
	}
	return pb, nil
}

// eventFromProto unmarshals the wire shape into an events.Event.
// Does NOT validate — caller validates after applying server-side
// defaults (id stamping, etc.).
func eventFromProto(pb *v1.Event) (events.Event, error) {
	if pb == nil {
		return events.Event{}, errors.New("event is nil")
	}
	e := events.Event{
		ID:            pb.GetId(),
		Type:          events.EventType(pb.GetType()),
		Source:        pb.GetSource(),
		Severity:      events.Severity(pb.GetSeverity()),
		CorrelationID: pb.GetCorrelationId(),
		Tags:          pb.GetTags(),
		Subject:       pb.GetSubject(),
	}
	if pb.GetTime() != nil {
		e.Time = pb.GetTime().AsTime()
	}
	if d := pb.GetData(); d != nil {
		e.Data = d.AsMap()
	}
	// Default severity to Info when the proto omits it — the caller
	// can override; matches NewEvent semantics for stamping fresh.
	if e.Severity == events.SeverityUnknown {
		e.Severity = events.SeverityInfo
	}
	return e, nil
}

// eventQueryFromProto translates a proto EventFilter + pagination
// fields into the typed EventQuery the EventStore expects.
//
// Mutual-exclusion rule: filter.Type wins over filter.Categories
// (operator picks exact match or fan-in, not both). When both are
// set we error rather than silently picking one.
func eventQueryFromProto(filter *v1.EventFilter, pageToken string, pageSize int, descending bool) (events.EventQuery, error) {
	q := events.EventQuery{
		Cursor:     pageToken,
		Limit:      pageSize,
		Descending: descending,
	}
	if filter != nil {
		if filter.GetType() != "" && len(filter.GetCategories()) > 0 {
			return events.EventQuery{}, errors.New("filter.type and filter.categories are mutually exclusive")
		}
		if filter.GetType() != "" {
			q.Type = events.EventType(filter.GetType())
		}
		if cats := filter.GetCategories(); len(cats) > 0 {
			// v1.0 supports a single category at the typed-query
			// layer (translates to LIKE 'cat.%' in SQL). Multiple
			// categories require subject-pattern fan-out at the
			// subscriber layer — out of scope for ListEvents.
			if len(cats) > 1 {
				return events.EventQuery{}, errors.New("filter.categories: ListEvents supports at most one category; use SubscribeEvents for multi-category fan-in")
			}
			q.Category = events.Category(cats[0])
		}
		q.Source = filter.GetSource()
		q.CorrelationID = filter.GetCorrelationId()
		if filter.GetMinSeverity() != v1.EventSeverity_EVENT_SEVERITY_UNSPECIFIED {
			q.MinSeverity = events.Severity(filter.GetMinSeverity())
		}
		q.Tags = filter.GetTags()
		q.Since = tsToTime(filter.GetSince())
		q.Until = tsToTime(filter.GetUntil())
	}
	return q, nil
}

// subjectPatternFromFilter derives the NATS subject pattern the
// subscriber attaches to. v1.0 always returns the broad `>`
// wildcard — the stream's subject filter (set at stream-creation
// time to `kscore.<cluster>.events.>`) gates what JetStream
// delivers, and the predicate built by predicateFromFilter
// narrows structurally. CEL via filter_expression handles
// anything more ad-hoc.
//
// Subject-level narrowing (e.g., `>.events.agent.connect`) would
// require building the cluster-prefixed full subject here, which
// couples the gRPC handler to the cluster name. Keeping the
// pattern cluster-agnostic is simpler for v1.0; if measurements
// show the predicate is a hot path, narrow the subject in a
// follow-up.
func subjectPatternFromFilter(_ *v1.EventFilter) (string, error) {
	return ">", nil
}

// predicateFromFilter builds an events.func(Event) bool encoding
// the structural fields not covered by the subject pattern or by
// the CEL filter expression. Returns nil when the filter is empty —
// the subscriber then skips the WithFilter option entirely.
func predicateFromFilter(filter *v1.EventFilter) func(events.Event) bool {
	if filter == nil {
		return nil
	}
	typeMatch := filter.GetType()
	var categoryMatch events.Category
	if cats := filter.GetCategories(); len(cats) == 1 {
		categoryMatch = events.Category(cats[0])
	}
	source := filter.GetSource()
	correlationID := filter.GetCorrelationId()
	var minSev events.Severity
	if s := filter.GetMinSeverity(); s != v1.EventSeverity_EVENT_SEVERITY_UNSPECIFIED {
		minSev = events.Severity(s)
	}
	since := tsToTime(filter.GetSince())
	until := tsToTime(filter.GetUntil())
	tags := filter.GetTags()

	// All-zero filter → no predicate needed.
	if typeMatch == "" && categoryMatch == "" && source == "" && correlationID == "" &&
		minSev == 0 && since.IsZero() && until.IsZero() && len(tags) == 0 {
		return nil
	}

	return func(e events.Event) bool {
		if typeMatch != "" && string(e.Type) != typeMatch {
			return false
		}
		if categoryMatch != "" && e.Type.Category() != categoryMatch {
			return false
		}
		if source != "" && e.Source != source {
			return false
		}
		if correlationID != "" && e.CorrelationID != correlationID {
			return false
		}
		if minSev != 0 && !e.Severity.AtLeast(minSev) {
			return false
		}
		if !since.IsZero() && e.Time.Before(since) {
			return false
		}
		if !until.IsZero() && !e.Time.Before(until) {
			return false
		}
		for k, want := range tags {
			if got, ok := e.Tags[k]; !ok || got != want {
				return false
			}
		}
		return true
	}
}

func tsToTime(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

// eventsErrToStatus funnels [internal/events] + [internal/state]
// sentinels into the right gRPC status code. Mirrors the table in
// the secrets adapter.
func eventsErrToStatus(err error) error {
	switch {
	case errors.Is(err, state.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, events.ErrEventNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, events.ErrInvalidEvent):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, events.ErrInvalidFilter):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, events.ErrPublisherNotStarted):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, events.ErrSubscriberNotStarted):
		return status.Error(codes.Unavailable, err.Error())
	case errors.Is(err, events.ErrPublisherBufferFull):
		return status.Error(codes.ResourceExhausted, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}
