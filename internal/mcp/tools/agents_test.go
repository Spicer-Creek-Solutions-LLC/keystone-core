package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

func TestAgentList(t *testing.T) {
	cp := &mockControlPlane{
		listAgents: func(ctx context.Context, in *pb.ListAgentsRequest, opts ...grpc.CallOption) (*pb.ListAgentsResponse, error) {
			return &pb.ListAgentsResponse{
				Agents: []*pb.AgentInfo{
					{
						AgentId:       "agent-1",
						Status:        pb.AgentStatus_AGENT_STATUS_ONLINE,
						Metadata:      &pb.AgentMetadata{Hostname: "host-1", Os: "linux"},
						LastHeartbeat: timestamppb.Now(),
					},
					{
						AgentId:  "agent-2",
						Status:   pb.AgentStatus_AGENT_STATUS_OFFLINE,
						Metadata: &pb.AgentMetadata{Hostname: "host-2", Os: "windows"},
					},
				},
			}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := agentRegistrar(client)
	result := callTool(t, registrar, "agent_list", nil)

	text := textContent(t, result)
	if !strings.Contains(text, "agent-1") || !strings.Contains(text, "agent-2") {
		t.Errorf("expected both agents in output, got: %s", text)
	}
	if !strings.Contains(text, "Found 2 agents") {
		t.Errorf("expected agent count, got: %s", text)
	}
}

func TestAgentList_StatusFilter(t *testing.T) {
	var receivedStatus pb.AgentStatus
	cp := &mockControlPlane{
		listAgents: func(ctx context.Context, in *pb.ListAgentsRequest, opts ...grpc.CallOption) (*pb.ListAgentsResponse, error) {
			receivedStatus = in.Status
			return &pb.ListAgentsResponse{}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := agentRegistrar(client)
	callTool(t, registrar, "agent_list", map[string]any{
		"status": "AGENT_STATUS_ONLINE",
	})

	if receivedStatus != pb.AgentStatus_AGENT_STATUS_ONLINE {
		t.Errorf("expected ONLINE filter, got %v", receivedStatus)
	}
}

func TestAgentList_Error(t *testing.T) {
	cp := &mockControlPlane{
		listAgents: func(ctx context.Context, in *pb.ListAgentsRequest, opts ...grpc.CallOption) (*pb.ListAgentsResponse, error) {
			return nil, fmt.Errorf("connection refused")
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := agentRegistrar(client)
	result := callTool(t, registrar, "agent_list", nil)

	if !result.IsError {
		t.Error("expected error result")
	}
	text := textContent(t, result)
	if !strings.Contains(text, "connection refused") {
		t.Errorf("expected error message, got: %s", text)
	}
}

func TestAgentShow(t *testing.T) {
	cp := &mockControlPlane{
		getAgent: func(ctx context.Context, in *pb.GetAgentRequest, opts ...grpc.CallOption) (*pb.GetAgentResponse, error) {
			if in.AgentId != "agent-1" {
				return nil, fmt.Errorf("not found")
			}
			return &pb.GetAgentResponse{
				Agent: &pb.AgentInfo{
					AgentId:  "agent-1",
					Status:   pb.AgentStatus_AGENT_STATUS_ONLINE,
					Metadata: &pb.AgentMetadata{Hostname: "host-1", Os: "linux"},
				},
			}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := agentRegistrar(client)
	result := callTool(t, registrar, "agent_show", map[string]any{
		"agent_id": "agent-1",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "agent-1") {
		t.Errorf("expected agent-1 in output, got: %s", text)
	}
}

func TestAgentShow_MissingID(t *testing.T) {
	cp := &mockControlPlane{}
	client := testClient(cp, nil, nil, "")
	registrar := agentRegistrar(client)
	// SDK validates required fields before the handler runs
	err := callToolExpectError(t, registrar, "agent_show", map[string]any{})
	if !strings.Contains(err.Error(), "agent_id") {
		t.Errorf("expected error about agent_id, got: %v", err)
	}
}

func TestAgentHealth(t *testing.T) {
	cp := &mockControlPlane{
		getStatus: func(ctx context.Context, in *pb.GetServerStatusRequest, opts ...grpc.CallOption) (*pb.GetServerStatusResponse, error) {
			return &pb.GetServerStatusResponse{
				Status: &pb.ServerStatus{
					Version:         "1.0.0",
					UptimeSeconds:   3600,
					ConnectedAgents: 5,
					TotalAgents:     10,
				},
			}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := agentRegistrar(client)
	result := callTool(t, registrar, "agent_health", nil)

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "1.0.0") {
		t.Errorf("expected version in output, got: %s", text)
	}
}
