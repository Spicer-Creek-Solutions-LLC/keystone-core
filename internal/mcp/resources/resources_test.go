package resources

import (
	"context"
	"strings"
	"testing"

	"google.golang.org/grpc"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

type mockControlPlane struct {
	pb.ControlPlaneServiceClient

	listAgents func(ctx context.Context, in *pb.ListAgentsRequest, opts ...grpc.CallOption) (*pb.ListAgentsResponse, error)
	getStatus  func(ctx context.Context, in *pb.GetServerStatusRequest, opts ...grpc.CallOption) (*pb.GetServerStatusResponse, error)
}

func (m *mockControlPlane) ListAgents(ctx context.Context, in *pb.ListAgentsRequest, opts ...grpc.CallOption) (*pb.ListAgentsResponse, error) {
	return m.listAgents(ctx, in, opts...)
}

func (m *mockControlPlane) GetServerStatus(ctx context.Context, in *pb.GetServerStatusRequest, opts ...grpc.CallOption) (*pb.GetServerStatusResponse, error) {
	return m.getStatus(ctx, in, opts...)
}

type mockEvent struct {
	pb.EventServiceClient

	listEvents func(ctx context.Context, in *pb.ListEventsRequest, opts ...grpc.CallOption) (*pb.ListEventsResponse, error)
}

func (m *mockEvent) ListEvents(ctx context.Context, in *pb.ListEventsRequest, opts ...grpc.CallOption) (*pb.ListEventsResponse, error) {
	return m.listEvents(ctx, in, opts...)
}

func TestRegisterAll(t *testing.T) {
	client := mcpserver.NewGRPCClientForTest(nil, nil, nil, "")
	resources := RegisterAll(client)
	if len(resources) != 3 {
		t.Fatalf("expected 3 resources, got %d", len(resources))
	}

	uris := map[string]bool{}
	for _, r := range resources {
		uris[r.Resource.URI] = true
	}
	for _, want := range []string{"keystone://agents", "keystone://cluster/status", "keystone://events/recent"} {
		if !uris[want] {
			t.Errorf("missing resource URI: %s", want)
		}
	}
}

func TestAgentsResource(t *testing.T) {
	cp := &mockControlPlane{
		listAgents: func(ctx context.Context, in *pb.ListAgentsRequest, opts ...grpc.CallOption) (*pb.ListAgentsResponse, error) {
			return &pb.ListAgentsResponse{
				Agents: []*pb.AgentInfo{
					{AgentId: "a-1", Status: pb.AgentStatus_AGENT_STATUS_ONLINE},
				},
			}, nil
		},
	}
	client := mcpserver.NewGRPCClientForTest(cp, nil, nil, "")
	reg := agentsResource(client)

	result, err := reg.Handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "a-1") {
		t.Errorf("expected agent ID in output, got: %s", result.Contents[0].Text)
	}
}

func TestClusterStatusResource(t *testing.T) {
	cp := &mockControlPlane{
		getStatus: func(ctx context.Context, in *pb.GetServerStatusRequest, opts ...grpc.CallOption) (*pb.GetServerStatusResponse, error) {
			return &pb.GetServerStatusResponse{
				Status: &pb.ServerStatus{Version: "1.0.0", UptimeSeconds: 3600},
			}, nil
		},
	}
	client := mcpserver.NewGRPCClientForTest(cp, nil, nil, "")
	reg := clusterStatusResource(client)

	result, err := reg.Handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Contents) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Contents))
	}
	if !strings.Contains(result.Contents[0].Text, "1.0.0") {
		t.Errorf("expected version in output, got: %s", result.Contents[0].Text)
	}
}

func TestRecentEventsResource(t *testing.T) {
	ev := &mockEvent{
		listEvents: func(ctx context.Context, in *pb.ListEventsRequest, opts ...grpc.CallOption) (*pb.ListEventsResponse, error) {
			if in.PageSize != 50 {
				t.Errorf("expected page size 50, got %d", in.PageSize)
			}
			return &pb.ListEventsResponse{
				Events: []*pb.Event{{Id: "e-1", Type: "agent.connect"}},
			}, nil
		},
	}
	client := mcpserver.NewGRPCClientForTest(nil, nil, ev, "")
	reg := recentEventsResource(client)

	result, err := reg.Handler(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Contents[0].Text, "e-1") {
		t.Errorf("expected event ID in output, got: %s", result.Contents[0].Text)
	}
}
