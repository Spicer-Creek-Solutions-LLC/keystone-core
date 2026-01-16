package e2e

import (
	"context"
	"net"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/shawnbutts/keystone-core/pkg/api/server"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

func TestControlPlaneGRPC_AgentLifecycle(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.cleanup()

	agent := newMockAgent("agent-1", &pb.AgentMetadata{
		Hostname: "agent-1",
		Os:       "linux",
		Arch:     "amd64",
	}, env.natsManager)
	agent.Start(t)
	defer agent.Stop()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return env.connMgr.GetAgentCount() == 1, nil
	}); err != nil {
		t.Fatalf("agent registration did not complete: %v", err)
	}

	grpcServer := grpc.NewServer()
	apiServer := server.NewControlPlaneServer(env.connMgr, env.cmdDispatcher, env.batchDispatcher, nil)
	pb.RegisterControlPlaneServiceServer(grpcServer, apiServer)

	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}
	grpcAddr := listener.Addr().String()

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()
	defer grpcServer.GracefulStop()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, grpcAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(),
	)
	if err != nil {
		t.Fatalf("failed to dial gRPC: %v", err)
	}
	defer conn.Close()

	client := pb.NewControlPlaneServiceClient(conn)

	listResp, err := client.ListAgents(ctx, &pb.ListAgentsRequest{})
	if err != nil {
		t.Fatalf("ListAgents() error = %v", err)
	}
	if len(listResp.Agents) != 1 {
		t.Fatalf("ListAgents() count = %d, want 1", len(listResp.Agents))
	}
	if listResp.Agents[0].AgentId != "agent-1" {
		t.Fatalf("ListAgents() id = %q, want %q", listResp.Agents[0].AgentId, "agent-1")
	}

	getResp, err := client.GetAgent(ctx, &pb.GetAgentRequest{AgentId: "agent-1"})
	if err != nil {
		t.Fatalf("GetAgent() error = %v", err)
	}
	if getResp.Agent == nil || getResp.Agent.AgentId != "agent-1" {
		t.Fatalf("GetAgent() id = %v, want agent-1", getResp.Agent)
	}
}
