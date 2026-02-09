package e2e

import (
	"context"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/proto"

	"github.com/shawnbutts/keystone-core/internal/config"
	"github.com/shawnbutts/keystone-core/internal/controlplane"
	natsmgr "github.com/shawnbutts/keystone-core/internal/nats"
	"github.com/shawnbutts/keystone-core/internal/state"
	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	"github.com/shawnbutts/keystone-core/pkg/api/server"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// testEnvironment holds all components for e2e testing
type testEnvironment struct {
	natsManager     *natsmgr.Manager
	stateStore      state.Store
	connMgr         *controlplane.ConnectionManager
	cmdDispatcher   *controlplane.CommandDispatcher
	batchDispatcher *controlplane.BatchDispatcher
	grpcServer      *grpc.Server
	grpcAddr        string
	cleanup         func()
}

// setupTestEnvironment creates a complete test environment with server and infrastructure
func setupTestEnvironment(t *testing.T) *testEnvironment {
	// Create temporary directory
	tmpDir, err := os.MkdirTemp("", "kscore-e2e-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	cleanup := func() {
		os.RemoveAll(tmpDir)
	}

	// Initialize NATS with dynamic port
	natsPort := helpers.FreePort(t)
	natsConfig := &config.NATSConfig{
		Mode: config.NATSModeEmbedded,
		Embedded: config.NATSEmbeddedConfig{
			Port:            natsPort,
			EnableJetStream: true,
			StoreDir:        filepath.Join(tmpDir, "nats"),
		},
	}

	natsManager, err := natsmgr.NewManager(natsConfig)
	if err != nil {
		cleanup()
		t.Fatalf("Failed to create NATS manager: %v", err)
	}

	if err := natsManager.Start(); err != nil {
		cleanup()
		t.Fatalf("Failed to start NATS manager: %v", err)
	}

	time.Sleep(100 * time.Millisecond) // Wait for NATS to be ready

	// Initialize state store
	storeConfig := &state.Config{
		Backend:    "sqlite",
		SQLitePath: filepath.Join(tmpDir, "test.db"),
		SQLiteWAL:  false,
	}

	stateStore, err := state.NewStore(storeConfig)
	if err != nil {
		natsManager.Shutdown()
		cleanup()
		t.Fatalf("Failed to create state store: %v", err)
	}

	ctx := context.Background()
	if err := stateStore.Ping(ctx); err != nil {
		stateStore.Close()
		natsManager.Shutdown()
		cleanup()
		t.Fatalf("Failed to ping state store: %v", err)
	}

	// Initialize control plane components
	connMgr := controlplane.NewConnectionManager(natsManager)
	if err := connMgr.Start(); err != nil {
		stateStore.Close()
		natsManager.Shutdown()
		cleanup()
		t.Fatalf("Failed to start connection manager: %v", err)
	}

	time.Sleep(100 * time.Millisecond) // Wait for connection manager

	cmdDispatcher := controlplane.NewCommandDispatcher(connMgr, stateStore)
	if err := cmdDispatcher.Start(); err != nil {
		connMgr.Stop()
		stateStore.Close()
		natsManager.Shutdown()
		cleanup()
		t.Fatalf("Failed to start command dispatcher: %v", err)
	}

	batchDispatcher := controlplane.NewBatchDispatcher(connMgr, cmdDispatcher, stateStore)

	// Start gRPC server
	grpcServer := grpc.NewServer()
	apiServer := server.NewControlPlaneServer(connMgr, cmdDispatcher, batchDispatcher, stateStore)
	pb.RegisterControlPlaneServiceServer(grpcServer, apiServer)

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "localhost:0") // Random available port
	if err != nil {
		connMgr.Stop()
		stateStore.Close()
		natsManager.Shutdown()
		cleanup()
		t.Fatalf("Failed to create listener: %v", err)
	}

	grpcAddr := listener.Addr().String()

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			t.Logf("gRPC server error: %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond) // Wait for server to start

	fullCleanup := func() {
		grpcServer.GracefulStop()
		cmdDispatcher.Stop()
		connMgr.Stop()
		stateStore.Close()
		natsManager.Shutdown()
		cleanup()
	}

	return &testEnvironment{
		natsManager:     natsManager,
		stateStore:      stateStore,
		connMgr:         connMgr,
		cmdDispatcher:   cmdDispatcher,
		batchDispatcher: batchDispatcher,
		grpcServer:      grpcServer,
		grpcAddr:        grpcAddr,
		cleanup:         fullCleanup,
	}
}

// mockAgent simulates an agent that responds to commands
type mockAgent struct {
	id          string
	metadata    *pb.AgentMetadata
	natsManager *natsmgr.Manager
	stopChan    chan struct{}
}

func newMockAgent(id string, metadata *pb.AgentMetadata, natsManager *natsmgr.Manager) *mockAgent {
	return &mockAgent{
		id:          id,
		metadata:    metadata,
		natsManager: natsManager,
		stopChan:    make(chan struct{}),
	}
}

func (a *mockAgent) Start(t *testing.T) {
	// Register agent
	regReq := &pb.RegisterRequest{
		AgentId:  a.id,
		Metadata: a.metadata,
	}

	data, err := proto.Marshal(regReq)
	if err != nil {
		t.Fatalf("Failed to marshal registration: %v", err)
	}

	_, err = a.natsManager.PublishRequest("kscore.default.agent.register", data, 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to register agent %s: %v", a.id, err)
	}

	// Send heartbeat
	go a.heartbeatLoop(t)

	// Listen for commands
	go a.commandLoop(t)
}

func (a *mockAgent) Stop() {
	close(a.stopChan)
}

func (a *mockAgent) heartbeatLoop(t *testing.T) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			hbReq := &pb.HeartbeatRequest{
				AgentId: a.id,
				Metrics: &pb.SystemMetrics{
					CpuPercent:    10.0,
					MemoryPercent: 20.0,
					DiskPercent:   30.0,
				},
			}

			data, err := proto.Marshal(hbReq)
			if err != nil {
				t.Logf("Agent %s: failed to marshal heartbeat: %v", a.id, err)
				continue
			}

			if err := a.natsManager.Publish("kscore.default.agent.heartbeat", data); err != nil {
				t.Logf("Agent %s: failed to send heartbeat: %v", a.id, err)
			}

		case <-a.stopChan:
			return
		}
	}
}

func (a *mockAgent) commandLoop(t *testing.T) {
	subject := fmt.Sprintf("kscore.default.agent.%s.command", a.id)

	sub, err := a.natsManager.Subscribe(subject, func(msg *nats.Msg) {
		var cmdReq pb.ExecuteCommandRequest
		if err := proto.Unmarshal(msg.Data, &cmdReq); err != nil {
			t.Logf("Agent %s: failed to unmarshal command: %v", a.id, err)
			return
		}

		t.Logf("Agent %s: executing command: %s %v", a.id, cmdReq.Command, cmdReq.Args)

		// Send success response immediately (mock execution)
		resp := &pb.ExecuteCommandResponse{
			CommandId: cmdReq.CommandId,
			Type:      pb.CommandResponseType_COMMAND_RESPONSE_TYPE_COMPLETED,
			ExitCode:  0,
		}

		data, err := proto.Marshal(resp)
		if err != nil {
			t.Logf("Agent %s: failed to marshal response: %v", a.id, err)
			return
		}

		// Reply to the command request
		if msg.Reply != "" {
			if err := a.natsManager.Publish(msg.Reply, data); err != nil {
				t.Logf("Agent %s: failed to send response: %v", a.id, err)
			}
		}
	})

	if err != nil {
		t.Errorf("Agent %s: failed to subscribe to commands: %v", a.id, err)
		return
	}
	defer sub.Unsubscribe()

	<-a.stopChan
}

// TestBatchExecution_E2E_MultipleAgents tests batch execution across multiple agents
func TestBatchExecution_E2E_MultipleAgents(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.cleanup()

	// Start mock agents
	agents := []*mockAgent{
		newMockAgent("web-1", &pb.AgentMetadata{
			Hostname: "web-server-01",
			Os:       "linux",
			Arch:     "amd64",
			Labels: map[string]string{
				"role": "web",
				"env":  "prod",
			},
		}, env.natsManager),
		newMockAgent("web-2", &pb.AgentMetadata{
			Hostname: "web-server-02",
			Os:       "linux",
			Arch:     "amd64",
			Labels: map[string]string{
				"role": "web",
				"env":  "prod",
			},
		}, env.natsManager),
		newMockAgent("db-1", &pb.AgentMetadata{
			Hostname: "db-server-01",
			Os:       "linux",
			Arch:     "amd64",
			Labels: map[string]string{
				"role": "db",
				"env":  "prod",
			},
		}, env.natsManager),
	}

	for _, agent := range agents {
		agent.Start(t)
		defer agent.Stop()
	}

	// Wait for agents to be fully registered
	time.Sleep(500 * time.Millisecond)

	// Verify agents are registered
	agentCount := env.connMgr.GetAgentCount()
	if agentCount != len(agents) {
		t.Fatalf("Expected %d agents, got %d", len(agents), agentCount)
	}

	// Create gRPC client
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, env.grpcAddr, //nolint:staticcheck // SA1019: grpc.DialContext is deprecated but supported throughout gRPC 1.x; migration to NewClient requires significant refactoring
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck // SA1019: grpc.WithBlock is deprecated but supported throughout gRPC 1.x
	)
	if err != nil {
		t.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := pb.NewControlPlaneServiceClient(conn)

	// Execute batch command targeting web servers
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "test-e2e-batch-1",
		Target:      "role:web",
		Command:     "echo",
		Args:        []string{"hello", "world"},
		Concurrency: 2,
	}

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	var responses []*pb.BatchExecuteCommandResponse
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("Stream error: %v", err)
		}

		responses = append(responses, resp)
		t.Logf("Received response: type=%s", resp.Type)
	}

	// Verify responses
	if len(responses) == 0 {
		t.Fatal("Expected at least one response")
	}

	// First response should be BATCH_START
	if responses[0].Type != pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_START {
		t.Errorf("First response should be BATCH_START, got %s", responses[0].Type)
	}

	// Last response should be BATCH_COMPLETE
	lastResp := responses[len(responses)-1]
	if lastResp.Type != pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
		t.Errorf("Last response should be BATCH_COMPLETE, got %s", lastResp.Type)
	}

	// Verify summary
	if lastResp.Summary == nil {
		t.Fatal("Expected summary in final response")
	}

	summary := lastResp.Summary
	if summary.Total != 2 {
		t.Errorf("Expected 2 total agents (web servers), got %d", summary.Total)
	}

	if summary.Successful != 2 {
		t.Errorf("Expected 2 successful executions, got %d", summary.Successful)
	}

	if summary.Failed != 0 {
		t.Errorf("Expected 0 failed executions, got %d", summary.Failed)
	}

	if summary.SuccessRate != 100.0 {
		t.Errorf("Expected 100%% success rate, got %.1f%%", summary.SuccessRate)
	}

	// Verify batch job was persisted
	statusReq := &pb.GetBatchJobStatusRequest{
		BatchJobId: req.BatchJobId,
	}

	statusResp, err := client.GetBatchJobStatus(ctx, statusReq)
	if err != nil {
		t.Fatalf("Failed to get batch job status: %v", err)
	}

	job := statusResp.Job
	if job.BatchJobId != req.BatchJobId {
		t.Errorf("Expected job ID %s, got %s", req.BatchJobId, job.BatchJobId)
	}

	if job.Status != pb.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("Expected job status COMPLETED, got %s", job.Status)
	}
}

// TestBatchExecution_E2E_AllAgents tests targeting all agents
func TestBatchExecution_E2E_AllAgents(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.cleanup()

	// Start mock agents with different roles
	agents := []*mockAgent{
		newMockAgent("agent-1", &pb.AgentMetadata{
			Os:   "linux",
			Arch: "amd64",
			Labels: map[string]string{
				"role": "web",
			},
		}, env.natsManager),
		newMockAgent("agent-2", &pb.AgentMetadata{
			Os:   "linux",
			Arch: "amd64",
			Labels: map[string]string{
				"role": "db",
			},
		}, env.natsManager),
		newMockAgent("agent-3", &pb.AgentMetadata{
			Os:   "darwin",
			Arch: "arm64",
			Labels: map[string]string{
				"role": "api",
			},
		}, env.natsManager),
	}

	for _, agent := range agents {
		agent.Start(t)
		defer agent.Stop()
	}

	time.Sleep(500 * time.Millisecond)

	// Create client
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, env.grpcAddr, //nolint:staticcheck // SA1019: grpc.DialContext is deprecated but supported throughout gRPC 1.x; migration to NewClient requires significant refactoring
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck // SA1019: grpc.WithBlock is deprecated but supported throughout gRPC 1.x
	)
	if err != nil {
		t.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := pb.NewControlPlaneServiceClient(conn)

	// Execute on all Linux agents
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "test-e2e-batch-linux",
		Target:      "os:linux",
		Command:     "uptime",
		Concurrency: 5,
	}

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	var summary *pb.BatchSummary
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("Stream error: %v", err)
		}

		if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
			summary = resp.Summary
		}
	}

	// Should have executed on 2 Linux agents (web and db)
	if summary == nil {
		t.Fatal("Expected summary")
	}

	if summary.Total != 2 {
		t.Errorf("Expected 2 Linux agents, got %d", summary.Total)
	}

	// List all batch jobs
	listReq := &pb.ListBatchJobsRequest{
		PageSize: 10,
	}

	listResp, err := client.ListBatchJobs(ctx, listReq)
	if err != nil {
		t.Fatalf("Failed to list batch jobs: %v", err)
	}

	// Should have at least this job
	found := false
	for _, job := range listResp.Jobs {
		if job.BatchJobId == req.BatchJobId {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find batch job in list")
	}
}

// TestBatchExecution_E2E_ConcurrencyControl tests concurrency limiting
func TestBatchExecution_E2E_ConcurrencyControl(t *testing.T) {
	env := setupTestEnvironment(t)
	defer env.cleanup()

	// Start 5 agents
	agents := make([]*mockAgent, 5)
	for i := 0; i < 5; i++ {
		agents[i] = newMockAgent(fmt.Sprintf("agent-%d", i+1), &pb.AgentMetadata{
			Os:   "linux",
			Arch: "amd64",
		}, env.natsManager)
		agents[i].Start(t)
		defer agents[i].Stop()
	}

	time.Sleep(500 * time.Millisecond)

	// Create client
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	conn, err := grpc.DialContext(ctx, env.grpcAddr, //nolint:staticcheck // SA1019: grpc.DialContext is deprecated but supported throughout gRPC 1.x; migration to NewClient requires significant refactoring
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck // SA1019: grpc.WithBlock is deprecated but supported throughout gRPC 1.x
	)
	if err != nil {
		t.Fatalf("Failed to connect to gRPC server: %v", err)
	}
	defer conn.Close()

	client := pb.NewControlPlaneServiceClient(conn)

	// Execute with concurrency limit of 2
	req := &pb.BatchExecuteCommandRequest{
		BatchJobId:  "test-e2e-concurrency",
		Target:      "os:linux",
		Command:     "sleep",
		Args:        []string{"0.1"},
		Concurrency: 2, // Limit to 2 concurrent executions
	}

	stream, err := client.BatchExecuteCommand(ctx, req)
	if err != nil {
		t.Fatalf("Failed to execute batch command: %v", err)
	}

	var summary *pb.BatchSummary
	for {
		resp, err := stream.Recv()
		if err != nil {
			if err.Error() == "EOF" {
				break
			}
			t.Fatalf("Stream error: %v", err)
		}

		if resp.Type == pb.BatchResponseType_BATCH_RESPONSE_TYPE_BATCH_COMPLETE {
			summary = resp.Summary
		}
	}

	// All agents should have been executed on
	if summary == nil {
		t.Fatal("Expected summary")
	}

	if summary.Total != 5 {
		t.Errorf("Expected 5 agents, got %d", summary.Total)
	}

	// Note: We can't easily test that concurrency was actually limited without
	// adding timing instrumentation, but we verify all agents were processed
}
