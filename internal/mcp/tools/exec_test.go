package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"google.golang.org/grpc"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

func TestExecRun(t *testing.T) {
	cp := &mockControlPlane{
		batchExec: func(ctx context.Context, in *pb.BatchExecuteCommandRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.BatchExecuteCommandResponse], error) {
			if in.Command != "uname -a" {
				t.Errorf("unexpected command: %s", in.Command)
			}
			if in.Target != "os:linux" {
				t.Errorf("unexpected target: %s", in.Target)
			}
			return &mockStream[pb.BatchExecuteCommandResponse]{
				msgs: []pb.BatchExecuteCommandResponse{
					{
						BatchJobId: "job-1",
						AgentId:    "agent-1",
						Type:       pb.BatchResponseType_BATCH_RESPONSE_TYPE_AGENT_COMPLETE,
						Data:       []byte("Linux host1 5.15.0"),
						ExitCode:   0,
					},
				},
			}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := execRegistrar(client, 100)
	result := callTool(t, registrar, "exec_run", map[string]any{
		"command": "uname -a",
		"target":  "os:linux",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "job-1") {
		t.Errorf("expected job ID in output, got: %s", text)
	}
	if !strings.Contains(text, "Linux host1") {
		t.Errorf("expected command output, got: %s", text)
	}
}

func TestExecRun_MissingCommand(t *testing.T) {
	cp := &mockControlPlane{}
	client := testClient(cp, nil, nil, "")
	registrar := execRegistrar(client, 100)
	err := callToolExpectError(t, registrar, "exec_run", map[string]any{
		"target": "os:linux",
	})
	if !strings.Contains(err.Error(), "command") {
		t.Errorf("expected error about command, got: %v", err)
	}
}

func TestExecRun_MissingTarget(t *testing.T) {
	cp := &mockControlPlane{}
	client := testClient(cp, nil, nil, "")
	registrar := execRegistrar(client, 100)
	err := callToolExpectError(t, registrar, "exec_run", map[string]any{
		"command": "ls",
	})
	if !strings.Contains(err.Error(), "target") {
		t.Errorf("expected error about target, got: %v", err)
	}
}

func TestExecRun_Timeout(t *testing.T) {
	var receivedTimeout int32
	cp := &mockControlPlane{
		batchExec: func(ctx context.Context, in *pb.BatchExecuteCommandRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.BatchExecuteCommandResponse], error) {
			receivedTimeout = in.Timeout
			return &mockStream[pb.BatchExecuteCommandResponse]{}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := execRegistrar(client, 100)
	callTool(t, registrar, "exec_run", map[string]any{
		"command": "ls",
		"target":  "all",
		"timeout": 30,
	})
	if receivedTimeout != 30 {
		t.Errorf("expected timeout=30, got %d", receivedTimeout)
	}
}

func TestExecStatus(t *testing.T) {
	cp := &mockControlPlane{
		getBatchJob: func(ctx context.Context, in *pb.GetBatchJobStatusRequest, opts ...grpc.CallOption) (*pb.GetBatchJobStatusResponse, error) {
			if in.BatchJobId != "job-1" {
				return nil, fmt.Errorf("not found")
			}
			return &pb.GetBatchJobStatusResponse{
				Job: &pb.BatchJobInfo{BatchJobId: "job-1"},
			}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := execRegistrar(client, 100)
	result := callTool(t, registrar, "exec_status", map[string]any{
		"batch_job_id": "job-1",
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "job-1") {
		t.Errorf("expected job ID, got: %s", text)
	}
}

func TestExecStatus_MissingID(t *testing.T) {
	cp := &mockControlPlane{}
	client := testClient(cp, nil, nil, "")
	registrar := execRegistrar(client, 100)
	err := callToolExpectError(t, registrar, "exec_status", map[string]any{})
	if !strings.Contains(err.Error(), "batch_job_id") {
		t.Errorf("expected error about batch_job_id, got: %v", err)
	}
}

func TestExecHistory(t *testing.T) {
	cp := &mockControlPlane{
		listCommands: func(ctx context.Context, in *pb.ListCommandsRequest, opts ...grpc.CallOption) (*pb.ListCommandsResponse, error) {
			if in.PageSize != 10 {
				t.Errorf("expected page size 10, got %d", in.PageSize)
			}
			return &pb.ListCommandsResponse{
				Commands: []*pb.CommandInfo{
					{CommandId: "cmd-1", Command: "ls -la"},
				},
			}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := execRegistrar(client, 100)
	result := callTool(t, registrar, "exec_history", map[string]any{
		"limit": 10,
	})

	if result.IsError {
		t.Errorf("unexpected error: %s", textContent(t, result))
	}
	text := textContent(t, result)
	if !strings.Contains(text, "cmd-1") {
		t.Errorf("expected command ID, got: %s", text)
	}
}

func TestExecHistory_DefaultLimit(t *testing.T) {
	var receivedPageSize int32
	cp := &mockControlPlane{
		listCommands: func(ctx context.Context, in *pb.ListCommandsRequest, opts ...grpc.CallOption) (*pb.ListCommandsResponse, error) {
			receivedPageSize = in.PageSize
			return &pb.ListCommandsResponse{}, nil
		},
	}
	client := testClient(cp, nil, nil, "")
	registrar := execRegistrar(client, 100)
	callTool(t, registrar, "exec_history", nil)
	if receivedPageSize != 20 {
		t.Errorf("expected default page size 20, got %d", receivedPageSize)
	}
}
