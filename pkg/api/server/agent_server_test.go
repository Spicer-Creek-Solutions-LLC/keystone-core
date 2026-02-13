package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/shawnbutts/keystone-core/internal/controlplane"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// mockAgentProvider implements AgentProvider for testing.
type mockAgentProvider struct {
	agents map[string]*controlplane.AgentInfo
}

func newMockAgentProvider() *mockAgentProvider {
	return &mockAgentProvider{agents: make(map[string]*controlplane.AgentInfo)}
}

func (m *mockAgentProvider) GetAgent(agentID string) (*controlplane.AgentInfo, error) {
	info, ok := m.agents[agentID]
	if !ok {
		return nil, errors.New("agent not found")
	}
	return info, nil
}

func TestAgentServer_GetAgentInfo(t *testing.T) {
	now := time.Now()
	provider := newMockAgentProvider()
	provider.agents["agent-1"] = &controlplane.AgentInfo{
		ID: "agent-1",
		Metadata: &pb.AgentMetadata{
			Hostname: "web-01",
			Os:       "linux",
			Arch:     "amd64",
		},
		Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
		LastHeartbeat: now,
		RegisteredAt:  now.Add(-24 * time.Hour),
	}

	srv := NewAgentServer(provider)
	ctx := context.Background()

	resp, err := srv.GetAgentInfo(ctx, &pb.GetAgentInfoRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.AgentId != "agent-1" {
		t.Errorf("got agent_id %q, want %q", resp.AgentId, "agent-1")
	}
	if resp.Metadata.Hostname != "web-01" {
		t.Errorf("got hostname %q, want %q", resp.Metadata.Hostname, "web-01")
	}
	if resp.Status != pb.AgentStatus_AGENT_STATUS_ONLINE {
		t.Errorf("got status %v, want ONLINE", resp.Status)
	}
	if resp.LastHeartbeat == nil {
		t.Error("expected last_heartbeat to be set")
	}
	if resp.RegisteredAt == nil {
		t.Error("expected registered_at to be set")
	}
}

func TestAgentServer_GetAgentInfo_NotFound(t *testing.T) {
	provider := newMockAgentProvider()
	srv := NewAgentServer(provider)

	_, err := srv.GetAgentInfo(context.Background(), &pb.GetAgentInfoRequest{AgentId: "nonexistent"})
	if err == nil {
		t.Fatal("expected error for missing agent")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
}

func TestAgentServer_GetAgentInfo_NilProvider(t *testing.T) {
	srv := NewAgentServer(nil)

	_, err := srv.GetAgentInfo(context.Background(), &pb.GetAgentInfoRequest{AgentId: "agent-1"})
	if err == nil {
		t.Fatal("expected error for nil provider")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestAgentServer_GetAgentInfo_EmptyID(t *testing.T) {
	srv := NewAgentServer(newMockAgentProvider())

	_, err := srv.GetAgentInfo(context.Background(), &pb.GetAgentInfoRequest{AgentId: ""})
	if err == nil {
		t.Fatal("expected error for empty agent_id")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestAgentServer_Register_Unimplemented(t *testing.T) {
	srv := NewAgentServer(nil)

	_, err := srv.Register(context.Background(), &pb.RegisterRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Errorf("got code %v, want Unimplemented", st.Code())
	}
}

func TestAgentServer_Heartbeat_Unimplemented(t *testing.T) {
	srv := NewAgentServer(nil)

	_, err := srv.Heartbeat(context.Background(), &pb.HeartbeatRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Errorf("got code %v, want Unimplemented", st.Code())
	}
}

func TestAgentServer_ExecuteCommand_Unimplemented(t *testing.T) {
	srv := NewAgentServer(nil)

	err := srv.ExecuteCommand(&pb.ExecuteCommandRequest{}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
	st, ok := status.FromError(err)
	if !ok {
		t.Fatalf("expected gRPC status error, got %v", err)
	}
	if st.Code() != codes.Unimplemented {
		t.Errorf("got code %v, want Unimplemented", st.Code())
	}
}
