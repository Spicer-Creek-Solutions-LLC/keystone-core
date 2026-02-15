package tools

import (
	"context"
	"io"
	"testing"

	sdkmcp "github.com/modelcontextprotocol/go-sdk/mcp"
	"google.golang.org/grpc"

	mcpserver "github.com/shawnbutts/keystone-core/internal/mcp"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// mockStream implements grpc.ServerStreamingClient for testing streaming RPCs.
type mockStream[T any] struct {
	grpc.ClientStream
	msgs []T
	idx  int
}

func (s *mockStream[T]) Recv() (*T, error) {
	if s.idx >= len(s.msgs) {
		return nil, io.EOF
	}
	msg := &s.msgs[s.idx]
	s.idx++
	return msg, nil
}

func (s *mockStream[T]) Context() context.Context { return context.Background() }

// mockControlPlane implements pb.ControlPlaneServiceClient for testing.
type mockControlPlane struct {
	pb.ControlPlaneServiceClient

	listAgents    func(ctx context.Context, in *pb.ListAgentsRequest, opts ...grpc.CallOption) (*pb.ListAgentsResponse, error)
	getAgent      func(ctx context.Context, in *pb.GetAgentRequest, opts ...grpc.CallOption) (*pb.GetAgentResponse, error)
	getStatus     func(ctx context.Context, in *pb.GetServerStatusRequest, opts ...grpc.CallOption) (*pb.GetServerStatusResponse, error)
	batchExec     func(ctx context.Context, in *pb.BatchExecuteCommandRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.BatchExecuteCommandResponse], error)
	getBatchJob   func(ctx context.Context, in *pb.GetBatchJobStatusRequest, opts ...grpc.CallOption) (*pb.GetBatchJobStatusResponse, error)
	listCommands  func(ctx context.Context, in *pb.ListCommandsRequest, opts ...grpc.CallOption) (*pb.ListCommandsResponse, error)
}

func (m *mockControlPlane) ListAgents(ctx context.Context, in *pb.ListAgentsRequest, opts ...grpc.CallOption) (*pb.ListAgentsResponse, error) {
	return m.listAgents(ctx, in, opts...)
}

func (m *mockControlPlane) GetAgent(ctx context.Context, in *pb.GetAgentRequest, opts ...grpc.CallOption) (*pb.GetAgentResponse, error) {
	return m.getAgent(ctx, in, opts...)
}

func (m *mockControlPlane) GetServerStatus(ctx context.Context, in *pb.GetServerStatusRequest, opts ...grpc.CallOption) (*pb.GetServerStatusResponse, error) {
	return m.getStatus(ctx, in, opts...)
}

func (m *mockControlPlane) BatchExecuteCommand(ctx context.Context, in *pb.BatchExecuteCommandRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.BatchExecuteCommandResponse], error) {
	return m.batchExec(ctx, in, opts...)
}

func (m *mockControlPlane) GetBatchJobStatus(ctx context.Context, in *pb.GetBatchJobStatusRequest, opts ...grpc.CallOption) (*pb.GetBatchJobStatusResponse, error) {
	return m.getBatchJob(ctx, in, opts...)
}

func (m *mockControlPlane) ListCommands(ctx context.Context, in *pb.ListCommandsRequest, opts ...grpc.CallOption) (*pb.ListCommandsResponse, error) {
	return m.listCommands(ctx, in, opts...)
}

// mockState implements pb.StateServiceClient for testing.
type mockState struct {
	pb.StateServiceClient

	checkState   func(ctx context.Context, in *pb.CheckStateRequest, opts ...grpc.CallOption) (*pb.CheckStateResponse, error)
	detectDrift  func(ctx context.Context, in *pb.DetectDriftRequest, opts ...grpc.CallOption) (*pb.DetectDriftResponse, error)
	getHistory   func(ctx context.Context, in *pb.GetStateHistoryRequest, opts ...grpc.CallOption) (*pb.GetStateHistoryResponse, error)
	applyState   func(ctx context.Context, in *pb.ApplyStateRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.ApplyStateResponse], error)
}

func (m *mockState) CheckState(ctx context.Context, in *pb.CheckStateRequest, opts ...grpc.CallOption) (*pb.CheckStateResponse, error) {
	return m.checkState(ctx, in, opts...)
}

func (m *mockState) DetectDrift(ctx context.Context, in *pb.DetectDriftRequest, opts ...grpc.CallOption) (*pb.DetectDriftResponse, error) {
	return m.detectDrift(ctx, in, opts...)
}

func (m *mockState) GetStateHistory(ctx context.Context, in *pb.GetStateHistoryRequest, opts ...grpc.CallOption) (*pb.GetStateHistoryResponse, error) {
	return m.getHistory(ctx, in, opts...)
}

func (m *mockState) ApplyState(ctx context.Context, in *pb.ApplyStateRequest, opts ...grpc.CallOption) (grpc.ServerStreamingClient[pb.ApplyStateResponse], error) {
	return m.applyState(ctx, in, opts...)
}

// mockEvent implements pb.EventServiceClient for testing.
type mockEvent struct {
	pb.EventServiceClient

	listEvents func(ctx context.Context, in *pb.ListEventsRequest, opts ...grpc.CallOption) (*pb.ListEventsResponse, error)
	getStats   func(ctx context.Context, in *pb.GetEventStatsRequest, opts ...grpc.CallOption) (*pb.GetEventStatsResponse, error)
}

func (m *mockEvent) ListEvents(ctx context.Context, in *pb.ListEventsRequest, opts ...grpc.CallOption) (*pb.ListEventsResponse, error) {
	return m.listEvents(ctx, in, opts...)
}

func (m *mockEvent) GetEventStats(ctx context.Context, in *pb.GetEventStatsRequest, opts ...grpc.CallOption) (*pb.GetEventStatsResponse, error) {
	return m.getStats(ctx, in, opts...)
}

// testClient creates a GRPCClient with mock service clients for testing.
func testClient(cp pb.ControlPlaneServiceClient, st pb.StateServiceClient, ev pb.EventServiceClient, restURL string) *mcpserver.GRPCClient {
	return mcpserver.NewGRPCClientForTest(cp, st, ev, restURL)
}

// callTool creates an MCP server with the given registrar, connects a client,
// and calls the named tool with the given arguments. Returns the result.
func callTool(t *testing.T, registrar mcpserver.ToolRegistrar, name string, args map[string]any) *sdkmcp.CallToolResult {
	t.Helper()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	registrar(srv, mcpserver.ProfileOpsAdmin)

	ct, st := sdkmcp.NewInMemoryTransports()

	ctx := context.Background()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	result, err := cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("CallTool(%s): %v", name, err)
	}
	return result
}

// callToolExpectError calls a tool and expects the SDK to reject it (e.g.
// schema validation failure for missing required fields).
func callToolExpectError(t *testing.T, registrar mcpserver.ToolRegistrar, name string, args map[string]any) error {
	t.Helper()

	srv := sdkmcp.NewServer(&sdkmcp.Implementation{Name: "test", Version: "0.0.0"}, nil)
	registrar(srv, mcpserver.ProfileOpsAdmin)

	ct, st := sdkmcp.NewInMemoryTransports()

	ctx := context.Background()
	ss, err := srv.Connect(ctx, st, nil)
	if err != nil {
		t.Fatalf("server connect: %v", err)
	}
	t.Cleanup(func() { _ = ss.Close() })

	client := sdkmcp.NewClient(&sdkmcp.Implementation{Name: "test-client", Version: "0.0.0"}, nil)
	cs, err := client.Connect(ctx, ct, nil)
	if err != nil {
		t.Fatalf("client connect: %v", err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	_, err = cs.CallTool(ctx, &sdkmcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err == nil {
		t.Fatalf("expected CallTool(%s) to fail, but it succeeded", name)
	}
	return err
}

// textContent extracts the text from the first TextContent in a CallToolResult.
func textContent(t *testing.T, r *sdkmcp.CallToolResult) string {
	t.Helper()
	if len(r.Content) == 0 {
		t.Fatal("no content in result")
	}
	tc, ok := r.Content[0].(*sdkmcp.TextContent)
	if !ok {
		t.Fatalf("content[0] is %T, want *TextContent", r.Content[0])
	}
	return tc.Text
}
