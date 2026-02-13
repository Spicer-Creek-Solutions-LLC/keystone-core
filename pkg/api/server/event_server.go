package server

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/shawnbutts/keystone-core/internal/events"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// EventServer implements the EventService gRPC server.
type EventServer struct {
	pb.UnimplementedEventServiceServer
	store      events.EventStore
	publisher  events.EventPublisher
	subscriber events.EventSubscriber
}

// NewEventServer creates a new EventServer.
// Any dependency may be nil — RPCs return codes.Unavailable if the required dep is nil.
func NewEventServer(store events.EventStore, publisher events.EventPublisher, subscriber events.EventSubscriber) *EventServer {
	return &EventServer{
		store:      store,
		publisher:  publisher,
		subscriber: subscriber,
	}
}

// ListEvents lists events with filtering and pagination.
func (s *EventServer) ListEvents(ctx context.Context, req *pb.ListEventsRequest) (*pb.ListEventsResponse, error) {
	if s.store == nil {
		return nil, status.Error(codes.Unavailable, "event store not available")
	}

	query := events.NewEventQuery()

	if req.Type != "" {
		query.WithTypes(events.EventType(req.Type))
	}
	if req.Source != "" {
		query.WithSources(req.Source)
	}
	if req.Severity != pb.EventSeverity_EVENT_SEVERITY_UNSPECIFIED {
		query.WithSeverities(protoToEventSeverity(req.Severity))
	}
	if req.MinSeverity != pb.EventSeverity_EVENT_SEVERITY_UNSPECIFIED {
		sev := protoToEventSeverity(req.MinSeverity)
		query.Severities = append(query.Severities, severitiesAtLeast(sev)...)
	}
	if req.CorrelationId != "" {
		query.WithCorrelationID(req.CorrelationId)
	}
	if len(req.Tags) > 0 {
		for k, v := range req.Tags {
			query.WithTag(k, v)
		}
	}
	if req.StartTime != nil {
		t := req.StartTime.AsTime()
		query.StartTime = &t
	}
	if req.EndTime != nil {
		t := req.EndTime.AsTime()
		query.EndTime = &t
	}

	switch req.SortOrder {
	case pb.SortOrder_SORT_ORDER_ASCENDING:
		query.SortOrder = "asc"
	case pb.SortOrder_SORT_ORDER_DESCENDING:
		query.SortOrder = "desc"
	default:
		query.SortOrder = "desc"
	}

	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 100
	}
	offset := parsePageToken(req.PageToken)
	query.WithPagination(pageSize, offset)

	result, err := s.store.Query(ctx, query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to query events: %v", err)
	}

	pbEvents := make([]*pb.Event, 0, len(result.Events))
	for _, evt := range result.Events {
		pbEvents = append(pbEvents, eventToProto(evt))
	}

	var nextPageToken string
	if offset+pageSize < int(result.TotalCount) {
		nextPageToken = encodePageToken(offset + pageSize)
	}

	return &pb.ListEventsResponse{
		Events:        pbEvents,
		NextPageToken: nextPageToken,
		TotalCount:    int32(result.TotalCount), //nolint:gosec // G115: bounded by event count
	}, nil
}

// GetEvent retrieves a specific event by ID.
func (s *EventServer) GetEvent(ctx context.Context, req *pb.GetEventRequest) (*pb.GetEventResponse, error) {
	if s.store == nil {
		return nil, status.Error(codes.Unavailable, "event store not available")
	}
	if req.EventId == "" {
		return nil, status.Error(codes.InvalidArgument, "event_id is required")
	}

	evt, err := s.store.Get(ctx, req.EventId)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "event not found: %s", req.EventId)
	}

	return &pb.GetEventResponse{
		Event: eventToProto(evt),
	}, nil
}

// EmitEvent emits a custom event.
func (s *EventServer) EmitEvent(ctx context.Context, req *pb.EmitEventRequest) (*pb.EmitEventResponse, error) {
	if s.publisher == nil {
		return nil, status.Error(codes.Unavailable, "event publisher not available")
	}
	if req.Type == "" {
		return nil, status.Error(codes.InvalidArgument, "type is required")
	}
	if req.Source == "" {
		return nil, status.Error(codes.InvalidArgument, "source is required")
	}

	builder := events.NewEvent(events.EventType(req.Type)).
		Source(req.Source)

	if req.Severity != pb.EventSeverity_EVENT_SEVERITY_UNSPECIFIED {
		builder.Severity(protoToEventSeverity(req.Severity))
	}
	if req.CorrelationId != "" {
		builder.CorrelationID(req.CorrelationId)
	}
	for k, v := range req.Tags {
		builder.Tag(k, v)
	}
	if req.Data != nil {
		builder.DataMap(req.Data.AsMap())
	}

	evt := builder.Build()

	if err := s.publisher.Publish(evt); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to publish event: %v", err)
	}

	// Also store the event if a store is available
	if s.store != nil {
		_ = s.store.Store(ctx, evt) // best-effort storage
	}

	return &pb.EmitEventResponse{
		EventId:   evt.ID,
		Timestamp: timestamppb.New(evt.Time),
	}, nil
}

// SubscribeEvents subscribes to events in real-time via server-side streaming.
func (s *EventServer) SubscribeEvents(req *pb.SubscribeEventsRequest, stream grpc.ServerStreamingServer[pb.Event]) error {
	if s.subscriber == nil {
		return status.Error(codes.Unavailable, "event subscriber not available")
	}

	// Build filter for server-side filtering
	filter := &events.EventFilter{}
	for _, t := range req.Types {
		filter.Types = append(filter.Types, events.EventType(t))
	}
	filter.Sources = append(filter.Sources, req.Sources...)
	if req.MinSeverity != pb.EventSeverity_EVENT_SEVERITY_UNSPECIFIED {
		filter.Severity = protoToEventSeverity(req.MinSeverity)
	}
	if len(req.Tags) > 0 {
		filter.Tags = req.Tags
	}

	// Determine NATS subject pattern
	subject := "*"
	if len(req.Types) == 1 {
		subject = req.Types[0]
	}

	// Channel for streaming events to the client
	eventCh := make(chan *events.Event, 64)

	handler := func(evt *events.Event) error {
		if filter != nil && !filter.Matches(evt) {
			return nil
		}
		select {
		case eventCh <- evt:
			return nil
		case <-stream.Context().Done():
			return stream.Context().Err()
		}
	}

	var sub *events.Subscription
	var err error
	if req.QueueGroup != "" {
		sub, err = s.subscriber.SubscribeQueue(subject, req.QueueGroup, handler)
	} else {
		sub, err = s.subscriber.Subscribe(subject, handler)
	}
	if err != nil {
		return status.Errorf(codes.Internal, "failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe() //nolint:errcheck // best-effort cleanup

	// Stream events until client disconnects
	for {
		select {
		case evt := <-eventCh:
			if err := stream.Send(eventToProto(evt)); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

// GetEventTypes returns available event types.
func (s *EventServer) GetEventTypes(ctx context.Context, req *pb.GetEventTypesRequest) (*pb.GetEventTypesResponse, error) {
	types := allEventTypes()

	// Filter by category if specified
	if req.Category != pb.EventCategory_EVENT_CATEGORY_UNSPECIFIED {
		var filtered []*pb.EventTypeInfo
		for _, t := range types {
			if t.Category == req.Category {
				filtered = append(filtered, t)
			}
		}
		types = filtered
	}

	return &pb.GetEventTypesResponse{
		Types: types,
	}, nil
}

// GetEventStats returns event statistics.
func (s *EventServer) GetEventStats(ctx context.Context, req *pb.GetEventStatsRequest) (*pb.GetEventStatsResponse, error) {
	if s.store == nil {
		return nil, status.Error(codes.Unavailable, "event store not available")
	}

	query := events.NewEventQuery()
	if req.StartTime != nil {
		t := req.StartTime.AsTime()
		query.StartTime = &t
	}
	if req.EndTime != nil {
		t := req.EndTime.AsTime()
		query.EndTime = &t
	}
	query.Limit = 0 // we only need count

	totalCount, err := s.store.Count(ctx, query)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "failed to count events: %v", err)
	}

	resp := &pb.GetEventStatsResponse{
		TotalEvents: totalCount,
		ByType:      make(map[string]int64),
		BySeverity:  make(map[string]int64),
		BySource:    make(map[string]int64),
	}

	// Query events grouped by type
	for _, et := range knownEventTypes() {
		typeQuery := events.NewEventQuery().WithTypes(et)
		if query.StartTime != nil {
			typeQuery.StartTime = query.StartTime
		}
		if query.EndTime != nil {
			typeQuery.EndTime = query.EndTime
		}
		typeQuery.Limit = 0
		count, err := s.store.Count(ctx, typeQuery)
		if err == nil && count > 0 {
			resp.ByType[string(et)] = count
		}
	}

	// Query events grouped by severity
	for _, sev := range []events.Severity{events.SeverityDebug, events.SeverityInfo, events.SeverityWarning, events.SeverityError, events.SeverityCritical} {
		sevQuery := events.NewEventQuery().WithSeverities(sev)
		if query.StartTime != nil {
			sevQuery.StartTime = query.StartTime
		}
		if query.EndTime != nil {
			sevQuery.EndTime = query.EndTime
		}
		sevQuery.Limit = 0
		count, err := s.store.Count(ctx, sevQuery)
		if err == nil && count > 0 {
			resp.BySeverity[string(sev)] = count
		}
	}

	// Calculate event rate
	if totalCount > 0 && req.StartTime != nil && req.EndTime != nil {
		duration := req.EndTime.AsTime().Sub(req.StartTime.AsTime()).Seconds()
		if duration > 0 {
			resp.EventRate = float32(totalCount) / float32(duration) //nolint:gosec // G115: bounded
		}
	}

	return resp, nil
}

// eventToProto converts an internal Event to a protobuf Event.
func eventToProto(evt *events.Event) *pb.Event {
	pbEvt := &pb.Event{
		Id:            evt.ID,
		Type:          string(evt.Type),
		Source:        evt.Source,
		Time:          timestamppb.New(evt.Time),
		Severity:      eventSeverityToProto(evt.Severity),
		CorrelationId: evt.CorrelationID,
		Tags:          evt.Tags,
		Subject:       evt.Subject,
	}

	if evt.Data != nil {
		data, err := structpb.NewStruct(evt.Data)
		if err == nil {
			pbEvt.Data = data
		}
	}

	return pbEvt
}

// eventSeverityToProto maps internal Severity to proto EventSeverity.
func eventSeverityToProto(sev events.Severity) pb.EventSeverity {
	switch sev {
	case events.SeverityDebug:
		return pb.EventSeverity_EVENT_SEVERITY_DEBUG
	case events.SeverityInfo:
		return pb.EventSeverity_EVENT_SEVERITY_INFO
	case events.SeverityWarning:
		return pb.EventSeverity_EVENT_SEVERITY_WARNING
	case events.SeverityError:
		return pb.EventSeverity_EVENT_SEVERITY_ERROR
	case events.SeverityCritical:
		return pb.EventSeverity_EVENT_SEVERITY_CRITICAL
	default:
		return pb.EventSeverity_EVENT_SEVERITY_UNSPECIFIED
	}
}

// protoToEventSeverity maps proto EventSeverity to internal Severity.
func protoToEventSeverity(sev pb.EventSeverity) events.Severity {
	switch sev {
	case pb.EventSeverity_EVENT_SEVERITY_DEBUG:
		return events.SeverityDebug
	case pb.EventSeverity_EVENT_SEVERITY_INFO:
		return events.SeverityInfo
	case pb.EventSeverity_EVENT_SEVERITY_WARNING:
		return events.SeverityWarning
	case pb.EventSeverity_EVENT_SEVERITY_ERROR:
		return events.SeverityError
	case pb.EventSeverity_EVENT_SEVERITY_CRITICAL:
		return events.SeverityCritical
	default:
		return events.SeverityInfo
	}
}

// severitiesAtLeast returns all severity levels at or above the given minimum.
func severitiesAtLeast(minSev events.Severity) []events.Severity {
	all := []events.Severity{events.SeverityDebug, events.SeverityInfo, events.SeverityWarning, events.SeverityError, events.SeverityCritical}
	levels := map[events.Severity]int{
		events.SeverityDebug:    0,
		events.SeverityInfo:     1,
		events.SeverityWarning:  2,
		events.SeverityError:    3,
		events.SeverityCritical: 4,
	}

	minLevel := levels[minSev]
	var result []events.Severity
	for _, sev := range all {
		if levels[sev] >= minLevel {
			result = append(result, sev)
		}
	}
	return result
}

// knownEventTypes returns all known event type constants.
func knownEventTypes() []events.EventType {
	return []events.EventType{
		events.EventTypeAgentConnect, events.EventTypeAgentDisconnect,
		events.EventTypeAgentHeartbeat, events.EventTypeAgentHeartbeatFailed, events.EventTypeAgentError,
		events.EventTypeJobStart, events.EventTypeJobComplete, events.EventTypeJobFail, events.EventTypeJobOutput,
		events.EventTypeStateApplyStart, events.EventTypeStateApplyDone, events.EventTypeStateApplyFail,
		events.EventTypeStateChange, events.EventTypeStateDrift,
		events.EventTypeUserLogin, events.EventTypeUserCommand, events.EventTypeUserError,
		events.EventTypeSystemStartup, events.EventTypeSystemShutdown, events.EventTypeSystemError,
		events.EventTypePolicyPass, events.EventTypePolicyViolation,
		events.EventTypeBootstrapGenerate, events.EventTypeBootstrapValidate,
		events.EventTypeBootstrapUse, events.EventTypeBootstrapRegister,
		events.EventTypeBootstrapRevoke, events.EventTypeBootstrapExpire, events.EventTypeBootstrapCleanup,
	}
}

// allEventTypes returns EventTypeInfo for all known event types with categories and descriptions.
func allEventTypes() []*pb.EventTypeInfo {
	return []*pb.EventTypeInfo{
		{Type: "agent.connect", Category: pb.EventCategory_EVENT_CATEGORY_AGENT, Description: "Agent connected to control plane"},
		{Type: "agent.disconnect", Category: pb.EventCategory_EVENT_CATEGORY_AGENT, Description: "Agent disconnected from control plane"},
		{Type: "agent.heartbeat", Category: pb.EventCategory_EVENT_CATEGORY_AGENT, Description: "Agent heartbeat received"},
		{Type: "agent.heartbeat_failed", Category: pb.EventCategory_EVENT_CATEGORY_AGENT, Description: "Agent heartbeat missed or failed"},
		{Type: "agent.error", Category: pb.EventCategory_EVENT_CATEGORY_AGENT, Description: "Agent reported an error"},
		{Type: "job.start", Category: pb.EventCategory_EVENT_CATEGORY_JOB, Description: "Command execution started"},
		{Type: "job.complete", Category: pb.EventCategory_EVENT_CATEGORY_JOB, Description: "Command execution completed successfully"},
		{Type: "job.fail", Category: pb.EventCategory_EVENT_CATEGORY_JOB, Description: "Command execution failed"},
		{Type: "job.output", Category: pb.EventCategory_EVENT_CATEGORY_JOB, Description: "Command execution output received"},
		{Type: "state.apply.start", Category: pb.EventCategory_EVENT_CATEGORY_STATE, Description: "State application started"},
		{Type: "state.apply.done", Category: pb.EventCategory_EVENT_CATEGORY_STATE, Description: "State application completed"},
		{Type: "state.apply.fail", Category: pb.EventCategory_EVENT_CATEGORY_STATE, Description: "State application failed"},
		{Type: "state.change", Category: pb.EventCategory_EVENT_CATEGORY_STATE, Description: "State change detected"},
		{Type: "state.drift", Category: pb.EventCategory_EVENT_CATEGORY_STATE, Description: "Configuration drift detected"},
		{Type: "user.login", Category: pb.EventCategory_EVENT_CATEGORY_USER, Description: "User logged in"},
		{Type: "user.command", Category: pb.EventCategory_EVENT_CATEGORY_USER, Description: "User executed a command"},
		{Type: "user.error", Category: pb.EventCategory_EVENT_CATEGORY_USER, Description: "User action resulted in error"},
		{Type: "system.startup", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "System started up"},
		{Type: "system.shutdown", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "System shut down"},
		{Type: "system.error", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "System error occurred"},
		{Type: "policy.pass", Category: pb.EventCategory_EVENT_CATEGORY_POLICY, Description: "Policy evaluation passed"},
		{Type: "policy.violation", Category: pb.EventCategory_EVENT_CATEGORY_POLICY, Description: "Policy violation detected"},
		{Type: "bootstrap.generate", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "Bootstrap token generated"},
		{Type: "bootstrap.validate", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "Bootstrap token validated"},
		{Type: "bootstrap.use", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "Bootstrap token used"},
		{Type: "bootstrap.register", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "Bootstrap registration completed"},
		{Type: "bootstrap.revoke", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "Bootstrap token revoked"},
		{Type: "bootstrap.expire", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "Bootstrap token expired"},
		{Type: "bootstrap.cleanup", Category: pb.EventCategory_EVENT_CATEGORY_SYSTEM, Description: "Bootstrap tokens cleaned up"},
	}
}

// Ensure EventServer satisfies the interface at compile time.
var _ pb.EventServiceServer = (*EventServer)(nil)
