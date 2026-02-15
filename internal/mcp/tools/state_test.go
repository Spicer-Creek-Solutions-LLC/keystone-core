package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

func TestStateCheck(t *testing.T) {
	st := &mockState{
		checkState: func(ctx context.Context, in *pb.CheckStateRequest, opts ...grpc.CallOption) (*pb.CheckStateResponse, error) {
			if in.Target != "os:linux" {
				t.Errorf("unexpected target: %s", in.Target)
			}
			return &pb.CheckStateResponse{}, nil
		},
	}
	client := testClient(nil, st, nil, "")
	registrar := stateRegistrar(client)
	result := callTool(t, registrar, "state_check", map[string]any{
		"target": "os:linux",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
}

func TestStateCheck_MissingTarget(t *testing.T) {
	client := testClient(nil, &mockState{}, nil, "")
	registrar := stateRegistrar(client)
	err := callToolExpectError(t, registrar, "state_check", map[string]any{})
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("expected error about target, got: %v", err)
	}
}

func TestStateCheck_Error(t *testing.T) {
	st := &mockState{
		checkState: func(ctx context.Context, in *pb.CheckStateRequest, opts ...grpc.CallOption) (*pb.CheckStateResponse, error) {
			return nil, fmt.Errorf("rpc error")
		},
	}
	client := testClient(nil, st, nil, "")
	registrar := stateRegistrar(client)
	result := callTool(t, registrar, "state_check", map[string]any{"target": "all"})
	if !result.IsError {
		t.Error("expected error result")
	}
}

func TestStateDrift(t *testing.T) {
	st := &mockState{
		detectDrift: func(ctx context.Context, in *pb.DetectDriftRequest, opts ...grpc.CallOption) (*pb.DetectDriftResponse, error) {
			if in.Target != "host:web-1" {
				t.Errorf("unexpected target: %s", in.Target)
			}
			return &pb.DetectDriftResponse{}, nil
		},
	}
	client := testClient(nil, st, nil, "")
	registrar := stateRegistrar(client)
	result := callTool(t, registrar, "state_drift", map[string]any{
		"target": "host:web-1",
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
}

func TestStateDrift_MissingTarget(t *testing.T) {
	client := testClient(nil, &mockState{}, nil, "")
	registrar := stateRegistrar(client)
	err := callToolExpectError(t, registrar, "state_drift", map[string]any{})
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("expected error about target, got: %v", err)
	}
}

func TestStateHistory(t *testing.T) {
	st := &mockState{
		getHistory: func(ctx context.Context, in *pb.GetStateHistoryRequest, opts ...grpc.CallOption) (*pb.GetStateHistoryResponse, error) {
			if in.PageSize != 10 {
				t.Errorf("expected page size 10, got %d", in.PageSize)
			}
			if in.AgentId != "agent-1" {
				t.Errorf("expected agent_id agent-1, got %s", in.AgentId)
			}
			return &pb.GetStateHistoryResponse{}, nil
		},
	}
	client := testClient(nil, st, nil, "")
	registrar := stateRegistrar(client)
	result := callTool(t, registrar, "state_history", map[string]any{
		"agent_id": "agent-1",
		"limit":    10,
	})
	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
}

func TestStateHistory_Defaults(t *testing.T) {
	var receivedPageSize int32
	st := &mockState{
		getHistory: func(ctx context.Context, in *pb.GetStateHistoryRequest, opts ...grpc.CallOption) (*pb.GetStateHistoryResponse, error) {
			receivedPageSize = in.PageSize
			return &pb.GetStateHistoryResponse{}, nil
		},
	}
	client := testClient(nil, st, nil, "")
	registrar := stateRegistrar(client)
	callTool(t, registrar, "state_history", nil)
	if receivedPageSize != 20 {
		t.Errorf("expected default page size 20, got %d", receivedPageSize)
	}
}

func TestStateApply(t *testing.T) {
	st := &mockState{
		applyState: func(ctx context.Context, in *pb.ApplyStateRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.ApplyStateResponse], error) {
			if in.Target != "os:linux" {
				t.Errorf("unexpected target: %s", in.Target)
			}
			return &mockStream[pb.ApplyStateResponse]{
				msgs: []pb.ApplyStateResponse{
					{
						RunId:   "run-1",
						AgentId: "agent-1",
						Type:    pb.StateResponseType_STATE_RESPONSE_TYPE_STATE_RESULT,
					},
				},
			}, nil
		},
	}
	client := testClient(nil, st, nil, "")
	registrar := stateRegistrar(client)
	result := callTool(t, registrar, "state_apply", map[string]any{
		"target": "os:linux",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "run-1") {
		t.Errorf("expected run ID, got: %s", text)
	}
}

func TestStateApply_MissingTarget(t *testing.T) {
	client := testClient(nil, &mockState{}, nil, "")
	registrar := stateRegistrar(client)
	err := callToolExpectError(t, registrar, "state_apply", map[string]any{})
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("expected error about target, got: %v", err)
	}
}
