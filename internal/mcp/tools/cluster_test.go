package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

func TestClusterStatus(t *testing.T) {
	cp := &mockControlPlane{
		getStatus: func(ctx context.Context, in *pb.GetServerStatusRequest, opts ...grpc.CallOption) (*pb.GetServerStatusResponse, error) {
			return &pb.GetServerStatusResponse{
				Status: &pb.ServerStatus{
					Version:         "2.0.0",
					UptimeSeconds:   7200,
					ConnectedAgents: 3,
					TotalAgents:     5,
					GoroutineCount:  42,
				},
			}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := clusterRegistrar(client)
	result := callTool(t, registrar, "cluster_status", nil)

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "2.0.0") {
		t.Errorf("expected version, got: %s", text)
	}
	if !strings.Contains(text, "7200") {
		t.Errorf("expected uptime, got: %s", text)
	}
}

func TestClusterStatus_NilStatus(t *testing.T) {
	cp := &mockControlPlane{
		getStatus: func(ctx context.Context, in *pb.GetServerStatusRequest, opts ...grpc.CallOption) (*pb.GetServerStatusResponse, error) {
			return &pb.GetServerStatusResponse{}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := clusterRegistrar(client)
	result := callTool(t, registrar, "cluster_status", nil)

	text := textContent(t, result)
	if !strings.Contains(text, "unknown") {
		t.Errorf("expected unknown status for nil, got: %s", text)
	}
}

func TestClusterStatus_Error(t *testing.T) {
	cp := &mockControlPlane{
		getStatus: func(ctx context.Context, in *pb.GetServerStatusRequest, opts ...grpc.CallOption) (*pb.GetServerStatusResponse, error) {
			return nil, fmt.Errorf("unavailable")
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := clusterRegistrar(client)
	result := callTool(t, registrar, "cluster_status", nil)

	if !result.IsError {
		t.Error("expected error result")
	}
}
