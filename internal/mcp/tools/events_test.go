package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

func TestEventQuery(t *testing.T) {
	ev := &mockEvent{
		listEvents: func(ctx context.Context, in *pb.ListEventsRequest, opts ...grpc.CallOption) (*pb.ListEventsResponse, error) {
			if in.Type != "agent.heartbeat" {
				t.Errorf("unexpected type filter: %s", in.Type)
			}
			if in.PageSize != 10 {
				t.Errorf("expected page size 10, got %d", in.PageSize)
			}
			return &pb.ListEventsResponse{
				Events: []*pb.Event{
					{Id: "evt-1", Type: "agent.heartbeat"},
					{Id: "evt-2", Type: "agent.heartbeat"},
				},
			}, nil
		},
	}
	client := testClient(nil, nil, ev, "")
	registrar := eventRegistrar(client)
	result := callTool(t, registrar, "event_query", map[string]any{
		"type":  "agent.heartbeat",
		"limit": 10,
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "Found 2 events") {
		t.Errorf("expected event count, got: %s", text)
	}
}

func TestEventQuery_Defaults(t *testing.T) {
	var receivedPageSize int32
	ev := &mockEvent{
		listEvents: func(ctx context.Context, in *pb.ListEventsRequest, opts ...grpc.CallOption) (*pb.ListEventsResponse, error) {
			receivedPageSize = in.PageSize
			return &pb.ListEventsResponse{}, nil
		},
	}
	client := testClient(nil, nil, ev, "")
	registrar := eventRegistrar(client)
	callTool(t, registrar, "event_query", nil)
	if receivedPageSize != 50 {
		t.Errorf("expected default page size 50, got %d", receivedPageSize)
	}
}

func TestEventQuery_Error(t *testing.T) {
	ev := &mockEvent{
		listEvents: func(ctx context.Context, in *pb.ListEventsRequest, opts ...grpc.CallOption) (*pb.ListEventsResponse, error) {
			return nil, fmt.Errorf("unavailable")
		},
	}
	client := testClient(nil, nil, ev, "")
	registrar := eventRegistrar(client)
	result := callTool(t, registrar, "event_query", nil)
	if !result.IsError {
		t.Error("expected error result")
	}
}

func TestEventStats(t *testing.T) {
	ev := &mockEvent{
		getStats: func(ctx context.Context, in *pb.GetEventStatsRequest, opts ...grpc.CallOption) (*pb.GetEventStatsResponse, error) {
			return &pb.GetEventStatsResponse{
				TotalEvents: 1000,
			}, nil
		},
	}
	client := testClient(nil, nil, ev, "")
	registrar := eventRegistrar(client)
	result := callTool(t, registrar, "event_stats", nil)

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "1000") {
		t.Errorf("expected total events, got: %s", text)
	}
}

func TestEventStats_Error(t *testing.T) {
	ev := &mockEvent{
		getStats: func(ctx context.Context, in *pb.GetEventStatsRequest, opts ...grpc.CallOption) (*pb.GetEventStatsResponse, error) {
			return nil, fmt.Errorf("unavailable")
		},
	}
	client := testClient(nil, nil, ev, "")
	registrar := eventRegistrar(client)
	result := callTool(t, registrar, "event_stats", nil)
	if !result.IsError {
		t.Error("expected error result")
	}
}
