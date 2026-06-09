// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Epic 07 task 12 — full integration test for the operator-facing
// remote-execution pipeline. Boots embedded NATS, 5 in-process
// agents, the full server-side stack (state store, command
// dispatcher, response router, batch dispatcher, NATS batch
// executor, gRPC ControlPlaneService), then dials the gRPC server
// over bufconn and exercises BatchExecuteCommand end-to-end.
//
// Build-tagged `integration`. Run via `make test-integration`.
package main

import (
	"context"
	"io"
	"net"
	"runtime"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	natsmgr "go.keystone-core.io/keystone-core/internal/nats"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/envelope"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// execFixture wires the full pipeline. 5 agents, real SQLite store,
// real dispatchers, real gRPC server over bufconn.
type execFixture struct {
	natsServer  *natsmgr.Manager
	agentNATS   []*natsmgr.Manager
	agents      []*agent.Agent
	store       state.Store
	cmdDisp     *controlplane.CommandDispatcher
	router      *controlplane.ResponseRouter
	batchDisp   *controlplane.BatchDispatcher
	cpGRPC      *controlplane.GRPCServer
	natsExec    *controlplane.NATSBatchExecutor
	grpcServer  *grpc.Server
	clientConn  *grpc.ClientConn
	client      v1.ControlPlaneServiceClient
	clusterName string
	cleanup     []func()
}

const (
	execClusterName = "epic07-test"
	execHMAC        = "epic07-test-shared-hmac-secret"
)

// agentSpec describes one agent seeded in the fixture.
type agentSpec struct {
	ID     string
	Labels map[string]string
}

// bootExecFixture wires the stack with the given agent specs. Each
// agent has a real Executor + SecurityEnforcer; commands published
// over the embedded NATS bus run against the host's /bin/echo.
func bootExecFixture(t *testing.T, specs []agentSpec) *execFixture {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("uses /bin/echo via real Executor; unix only in v1.0")
	}

	log := silentLogger()
	fix := &execFixture{clusterName: execClusterName}

	// Embedded NATS server.
	serverCfg := config.NATSConfig{
		Mode:              config.NATSModeEmbedded,
		ClusterName:       execClusterName,
		MaxReconnects:     1,
		ReconnectWait:     50 * time.Millisecond,
		MaxReconnectDelay: 200 * time.Millisecond,
		ReconnectJitter:   0.1,
		JetStream:         config.JetStreamConfig{Enabled: false},
		Embedded:          config.EmbeddedNATSConfig{Host: "127.0.0.1", Port: pickFreePort(t)},
		Dedup:             config.DedupConfig{Enabled: false},
		CircuitBreaker:    config.CircuitBreakerConfig{Enabled: false},
	}
	serverNATS, err := natsmgr.New(serverCfg, log)
	if err != nil {
		t.Fatalf("server natsmgr.New: %v", err)
	}
	if err := serverNATS.Start(context.Background()); err != nil {
		t.Fatalf("server natsmgr.Start: %v", err)
	}
	fix.natsServer = serverNATS

	// Single shared enforcer (same secret on sign + verify path).
	enforcer, err := agent.NewSecurityEnforcer(agent.SecurityPolicy{
		HMACSecret:    []byte(execHMAC),
		DefaultPolicy: agent.PolicyAllow,
	}, log)
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}

	// SQLite state store. BatchDispatcher needs a real one to persist
	// batch_jobs and batch_agent_results.
	store, storeCleanup := newExecTestStore(t)
	fix.store = store
	fix.cleanup = append(fix.cleanup, storeCleanup)

	// Seed agents into the registry so AgentResolver can find them.
	now := time.Now()
	for _, spec := range specs {
		if err := store.CreateAgent(context.Background(), &state.AgentRecord{
			ID: spec.ID, Hostname: spec.ID + ".example",
			OS: "linux", Architecture: "amd64",
			Labels:          spec.Labels,
			Status:          state.AgentStatusConnected,
			RegisteredAt:    now,
			LastHeartbeatAt: now,
		}); err != nil {
			t.Fatalf("seed agent %s: %v", spec.ID, err)
		}
	}

	// Boot one in-process agent per spec. Each subscribes to its own
	// command subject and publishes responses.
	for _, spec := range specs {
		ac := config.NATSConfig{
			Mode: config.NATSModeExternal, URLs: []string{serverNATS.ClientURL()},
			ClusterName: execClusterName, MaxReconnects: -1,
			ReconnectWait: 50 * time.Millisecond, MaxReconnectDelay: 200 * time.Millisecond,
			ReconnectJitter: 0.1, JetStream: config.JetStreamConfig{Enabled: false},
			Embedded: config.EmbeddedNATSConfig{Host: "127.0.0.1", Port: 4222},
			Dedup:    config.DedupConfig{Enabled: false}, CircuitBreaker: config.CircuitBreakerConfig{Enabled: false},
		}
		am, err := natsmgr.New(ac, log)
		if err != nil {
			t.Fatalf("agent natsmgr.New(%s): %v", spec.ID, err)
		}
		if err := am.Start(context.Background()); err != nil {
			t.Fatalf("agent natsmgr.Start(%s): %v", spec.ID, err)
		}
		fix.agentNATS = append(fix.agentNATS, am)

		executor := agent.NewExecutor(agent.ExecutorConfig{
			Logger: log, KillGrace: 100 * time.Millisecond, DefaultTimeout: 5 * time.Second,
		})
		a, err := agent.New(agent.Config{
			AgentID:           spec.ID,
			HeartbeatInterval: 1 * time.Hour, // suppress heartbeat noise during the test
			MetadataInterval:  1 * time.Hour,
			CommandTimeout:    2 * time.Second,
		}, agentNATSAdapter{m: am}, am.Subjects(),
			agent.NewGopsutilCollector(log), executor, enforcer, log)
		if err != nil {
			t.Fatalf("agent.New(%s): %v", spec.ID, err)
		}
		if err := a.Start(context.Background()); err != nil {
			t.Fatalf("agent.Start(%s): %v", spec.ID, err)
		}
		fix.agents = append(fix.agents, a)
	}

	// AgentLookup that reads directly from the seeded registry —
	// avoids spinning up a full ConnectionManager + its heartbeat
	// monitor for the test scope.
	connMgr := storeAgentLookup{store: store}

	// Server-side CommandDispatcher publishes signed CommandMessages
	// onto the embedded bus.
	disp, err := controlplane.NewDispatcher(controlplane.DispatcherConfig{
		Store:                 store,
		Agents:                connMgr,
		Publisher:             serverNATSPublisher{m: serverNATS},
		Subjects:              serverNATS.Subjects(),
		Signer:                commandSignerAdapter{enf: enforcer},
		Logger:                log,
		// A local echo round-trips in well under a second, but under the
		// race detector + the full sequential integration suite a NATS
		// round-trip can dilate past a few seconds; a generous ceiling
		// keeps a slow-but-completing command from being falsely timed
		// out (the <2s SLO below is asserted only in non-race runs).
		DefaultTimeoutSeconds: 20,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if err := disp.Start(context.Background()); err != nil {
		t.Fatalf("Dispatcher.Start: %v", err)
	}
	fix.cmdDisp = disp

	// Response router subscribes to agent.*.response.
	router, err := controlplane.NewResponseRouter(controlplane.ResponseRouterConfig{
		Subscriber: testSubscriberAdapter{m: serverNATS},
		Subjects:   serverNATS.Subjects(),
		Dispatcher: disp,
		Logger:     log,
	})
	if err != nil {
		t.Fatalf("NewResponseRouter: %v", err)
	}
	if err := router.Start(context.Background()); err != nil {
		t.Fatalf("router.Start: %v", err)
	}
	fix.router = router

	// Batch dispatcher + NATS executor.
	bd, err := controlplane.NewBatchDispatcher(controlplane.BatchDispatcherConfig{Store: store, Logger: log})
	if err != nil {
		t.Fatalf("NewBatchDispatcher: %v", err)
	}
	fix.batchDisp = bd

	natsExec, err := controlplane.NewNATSBatchExecutor(controlplane.NATSBatchExecutorConfig{
		Dispatcher:     disp,
		Router:         router,
		DefaultTimeout: 10 * time.Second,
	})
	if err != nil {
		t.Fatalf("NewNATSBatchExecutor: %v", err)
	}
	fix.natsExec = natsExec

	cpGRPC, err := controlplane.NewGRPCServer(controlplane.GRPCServerConfig{
		Dispatcher: bd, Store: store, Executor: natsExec, Logger: log,
	})
	if err != nil {
		t.Fatalf("NewGRPCServer: %v", err)
	}
	fix.cpGRPC = cpGRPC

	// bufconn-backed gRPC.
	lis := bufconn.Listen(1 << 20)
	gs := grpc.NewServer()
	v1.RegisterControlPlaneServiceServer(gs, cpGRPC)
	go func() {
		_ = gs.Serve(lis)
	}()
	fix.grpcServer = gs

	conn, err := grpc.NewClient("passthrough:bufnet",
		grpc.WithContextDialer(func(_ context.Context, _ string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("dial bufconn: %v", err)
	}
	fix.clientConn = conn
	fix.client = v1.NewControlPlaneServiceClient(conn)

	return fix
}

func (f *execFixture) shutdown() {
	if f.clientConn != nil {
		_ = f.clientConn.Close()
	}
	if f.grpcServer != nil {
		f.grpcServer.GracefulStop()
	}
	if f.router != nil {
		_ = f.router.Stop()
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if f.cmdDisp != nil {
		_ = f.cmdDisp.Stop(stopCtx)
	}
	for _, a := range f.agents {
		_ = a.Shutdown(stopCtx)
	}
	for _, m := range f.agentNATS {
		_ = m.Shutdown(stopCtx)
	}
	if f.natsServer != nil {
		_ = f.natsServer.Shutdown(stopCtx)
	}
	for i := len(f.cleanup) - 1; i >= 0; i-- {
		f.cleanup[i]()
	}
}

// newExecTestStore opens a SQLite in-memory store for the integration
// fixture. Test-only — production wiring goes through the real
// config path.
func newExecTestStore(t *testing.T) (state.Store, func()) {
	t.Helper()
	s, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: ":memory:"},
	})
	if err != nil {
		t.Fatalf("state.NewStore: %v", err)
	}
	return s, func() { _ = s.Close() }
}

// storeAgentLookup reads the agent registry directly from the store,
// bypassing ConnectionManager's in-memory cache (which would require
// Register/Heartbeat plumbing during boot).
type storeAgentLookup struct{ store state.AgentStore }

func (a storeAgentLookup) Get(ctx context.Context, id string) (*state.AgentRecord, error) {
	return a.store.GetAgent(ctx, id)
}

// testSubscriberAdapter wraps natsmgr.Manager into the controlplane
// Subscriber interface used by ResponseRouter (mirrors the production
// natsSubscriberAdapter in main.go but lives in the test package).
type testSubscriberAdapter struct{ m *natsmgr.Manager }

func (a testSubscriberAdapter) Subscribe(subject string, h controlplane.MessageHandler) (controlplane.Subscription, error) {
	return a.m.Subscribe(subject, func(ctx context.Context, subj string, env envelope.Envelope) error {
		return h(ctx, subj, env)
	})
}

// ---- Tests ---------------------------------------------------------------

func TestEpic07_BatchExecute_3of5Hits(t *testing.T) {
	specs := []agentSpec{
		{ID: "web-1", Labels: map[string]string{"role": "web"}},
		{ID: "web-2", Labels: map[string]string{"role": "web"}},
		{ID: "web-3", Labels: map[string]string{"role": "web"}},
		{ID: "db-1", Labels: map[string]string{"role": "db"}},
		{ID: "db-2", Labels: map[string]string{"role": "db"}},
	}
	fix := bootExecFixture(t, specs)
	defer fix.shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	started := time.Now()
	stream, err := fix.client.BatchExecuteCommand(ctx, &v1.BatchExecuteCommandRequest{
		Target:  &v1.Target{Labels: map[string]string{"role": "web"}},
		Command: "/bin/echo",
		Args:    []string{"hello-from-batch"},
	})
	if err != nil {
		t.Fatalf("BatchExecuteCommand: %v", err)
	}

	var batchID string
	starts, completes, fails := 0, 0, 0
	var terminal *v1.BatchTerminal
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("stream Recv: %v", err)
		}
		if id := ev.GetBatchJobId(); id != "" {
			batchID = id
		}
		if l := ev.GetLifecycle(); l != nil {
			switch l.Kind {
			case v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_START:
				starts++
			case v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_COMPLETE:
				completes++
			case v1.BatchAgentLifecycleKind_BATCH_AGENT_LIFECYCLE_KIND_AGENT_FAILED:
				fails++
			}
		}
		if term := ev.GetTerminal(); term != nil {
			terminal = term
			break
		}
	}
	elapsed := time.Since(started)

	if batchID == "" {
		t.Fatal("no batch_job_id event")
	}
	if starts != 3 || completes != 3 || fails != 0 {
		t.Errorf("lifecycle counts: start=%d complete=%d fail=%d; want 3/3/0", starts, completes, fails)
	}
	if terminal == nil || terminal.Status != v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("terminal = %+v; want COMPLETED", terminal)
	}
	// Acceptance bullet: 5-agent batch (3 hits) < 2s on local NATS.
	// Skipped under the race detector, which dilates NATS/scheduling
	// latency past any representative SLO — the real SLO gate is `make
	// slo` (non-race). See raceEnabled.
	if !raceEnabled && elapsed > 2*time.Second {
		t.Errorf("batch elapsed %v, want <2s", elapsed)
	}

	// GetBatchAgentResult round-trip for one agent — verifies stdout
	// persisted with the expected content.
	resp, err := fix.client.GetBatchAgentResult(ctx, &v1.GetBatchAgentResultRequest{
		BatchJobId: batchID, AgentId: "web-1",
	})
	if err != nil {
		t.Fatalf("GetBatchAgentResult: %v", err)
	}
	if !resp.Result.Success {
		t.Errorf("web-1 result Success=false: %+v", resp.Result)
	}
	if string(resp.Result.Stdout) != "hello-from-batch\n" {
		t.Errorf("web-1 stdout = %q, want %q", resp.Result.Stdout, "hello-from-batch\n")
	}
}

func TestEpic07_BatchExecute_DryRun(t *testing.T) {
	specs := []agentSpec{
		{ID: "w-1", Labels: map[string]string{"role": "web"}},
		{ID: "w-2", Labels: map[string]string{"role": "web"}},
		{ID: "d-1", Labels: map[string]string{"role": "db"}},
	}
	fix := bootExecFixture(t, specs)
	defer fix.shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	stream, err := fix.client.BatchExecuteCommand(ctx, &v1.BatchExecuteCommandRequest{
		Target:  &v1.Target{Labels: map[string]string{"role": "web"}},
		Command: "/bin/echo",
		Args:    []string{"never-runs"},
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("BatchExecuteCommand dry-run: %v", err)
	}

	var preview *v1.BatchPreview
	var terminal *v1.BatchTerminal
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if p := ev.GetPreview(); p != nil {
			preview = p
		}
		if term := ev.GetTerminal(); term != nil {
			terminal = term
		}
	}
	if preview == nil || len(preview.AgentIds) != 2 {
		t.Errorf("preview = %+v; want 2 agents (w-1, w-2)", preview)
	}
	if terminal == nil || terminal.Status != v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
		t.Errorf("terminal = %+v", terminal)
	}

	// No batch row created.
	jobs, _ := fix.batchDisp.ListBatches(ctx, state.BatchJobFilter{Limit: 10})
	if len(jobs) != 0 {
		t.Errorf("dry-run created %d batch jobs; want 0", len(jobs))
	}
}

func TestEpic07_SingleAgentLatency(t *testing.T) {
	specs := []agentSpec{{ID: "solo", Labels: map[string]string{"role": "web"}}}
	fix := bootExecFixture(t, specs)
	defer fix.shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	started := time.Now()
	stream, err := fix.client.BatchExecuteCommand(ctx, &v1.BatchExecuteCommandRequest{
		Target:  &v1.Target{AgentIds: []string{"solo"}},
		Command: "/bin/echo",
		Args:    []string{"latency-probe"},
	})
	if err != nil {
		t.Fatalf("BatchExecuteCommand: %v", err)
	}
	for {
		ev, err := stream.Recv()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		if ev.GetTerminal() != nil {
			break
		}
	}
	elapsed := time.Since(started)
	// Acceptance bullet: single-agent <1s on local NATS.
	if elapsed > 1*time.Second {
		t.Errorf("single-agent elapsed %v, want <1s", elapsed)
	}
}

func TestEpic07_AsyncStatusFlow(t *testing.T) {
	specs := []agentSpec{
		{ID: "a-1", Labels: map[string]string{"role": "web"}},
		{ID: "a-2", Labels: map[string]string{"role": "web"}},
	}
	fix := bootExecFixture(t, specs)
	defer fix.shutdown()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	stream, err := fix.client.BatchExecuteCommand(ctx, &v1.BatchExecuteCommandRequest{
		Target:  &v1.Target{Labels: map[string]string{"role": "web"}},
		Command: "/bin/echo",
		Args:    []string{"async"},
	})
	if err != nil {
		t.Fatalf("BatchExecuteCommand: %v", err)
	}

	// Consume only the batch_job_id event (async behavior).
	var batchID string
	for batchID == "" {
		ev, err := stream.Recv()
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		batchID = ev.GetBatchJobId()
	}

	// Poll GetBatchJob until terminal.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := fix.client.GetBatchJob(ctx, &v1.GetBatchJobRequest{BatchJobId: batchID})
		if err != nil {
			t.Fatalf("GetBatchJob: %v", err)
		}
		if resp.Batch.Status == v1.BatchJobStatus_BATCH_JOB_STATUS_COMPLETED {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("batch never reached COMPLETED")
}
