// SPDX-License-Identifier: Apache-2.0

//go:build integration

// Package main test — Epic 06 task 12 full integration test.
//
// Boots embedded NATS + agent stack in the same process, exercises
// the wire end-to-end, and tears down cleanly. Closes Epic 06.
//
// Build-tagged `integration` because it spins up an in-process
// nats-server/v2 and runs real /bin/echo through the agent's
// Executor. ~2s runtime; not part of `make test` (the unit tier).
// Run with `make test-integration`.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"runtime"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"
	"go.uber.org/goleak"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	natsmgr "go.keystone-core.io/keystone-core/internal/nats"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// pickFreePort grabs an ephemeral TCP port and immediately closes
// the listener. Brief race window before the embedded NATS server
// claims it; acceptable for tests.
func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

// stubCommandStore is the minimal CommandStore the dispatcher
// needs for a Dispatch happy path. We don't care about
// persistence — the test asserts on the wire, not on storage.
type stubCommandStore struct{}

func (stubCommandStore) CreateCommand(_ context.Context, _ *state.CommandRecord) error {
	return nil
}
func (stubCommandStore) GetCommand(_ context.Context, _ string) (*state.CommandRecord, error) {
	return nil, errors.New("stub: not implemented")
}
func (stubCommandStore) ListCommands(_ context.Context, _ state.CommandFilter) ([]*state.CommandRecord, error) {
	return nil, nil
}
func (stubCommandStore) UpdateCommandResult(_ context.Context, _ string, _ state.CommandResult) error {
	return nil
}
func (stubCommandStore) DeleteCommandsBefore(_ context.Context, _ time.Time, _ []state.CommandStatus) (int, error) {
	return 0, nil
}

// stubAgentLookup pretends every agent ID is registered + connected.
// Real CP would consult ConnectionManager / AgentStore here.
type stubAgentLookup struct{ id string }

func (s stubAgentLookup) Get(_ context.Context, id string) (*state.AgentRecord, error) {
	if id != s.id {
		return nil, errors.New("stub: unknown agent")
	}
	return &state.AgentRecord{
		ID:           s.id,
		Status:       state.AgentStatusConnected,
		RegisteredAt: time.Now(),
	}, nil
}

// integrationFixture holds every component the integration test
// boots so cleanup is centralized in the t.Cleanup function.
type integrationFixture struct {
	natsServer  *natsmgr.Manager // embedded nats-server (the "server" in the spec)
	natsAgent   *natsmgr.Manager // agent's external client
	agent       *agent.Agent     // the agent runtime
	dispatcher  *controlplane.CommandDispatcher
	testConn    *natsclient.Conn        // sibling connection for test subscribers
	subBuilder  *natsmgr.SubjectBuilder // for subject construction
	enforcer    *agent.SecurityEnforcer // shared between sign + verify
	clusterName string
	agentID     string
}

// silentLogger discards everything below ERROR. Keeps test
// output clean unless something goes wrong server-side.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

func bootIntegrationFixture(t *testing.T) *integrationFixture {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("uses /bin/echo via real Executor; unix only in v1.0")
	}
	const (
		clusterName = "epic06-test"
		agentID     = "agent-int-1"
		hmacSecret  = "epic06-test-shared-hmac-secret"
	)
	log := silentLogger()

	// Embedded NATS server. JetStream off — we don't need
	// at-least-once for the wire roundtrip.
	serverCfg := config.NATSConfig{
		Mode:              config.NATSModeEmbedded,
		ClusterName:       clusterName,
		MaxReconnects:     1,
		ReconnectWait:     50 * time.Millisecond,
		MaxReconnectDelay: 200 * time.Millisecond,
		ReconnectJitter:   0.1,
		JetStream:         config.JetStreamConfig{Enabled: false},
		Embedded: config.EmbeddedNATSConfig{
			Host: "127.0.0.1",
			Port: pickFreePort(t),
		},
		Dedup:          config.DedupConfig{Enabled: false},
		CircuitBreaker: config.CircuitBreakerConfig{Enabled: false},
	}
	serverNATS, err := natsmgr.New(serverCfg, log)
	if err != nil {
		t.Fatalf("server natsmgr.New: %v", err)
	}
	if err := serverNATS.Start(context.Background()); err != nil {
		t.Fatalf("server natsmgr.Start: %v", err)
	}

	// Agent's external NATS client points at the embedded server.
	agentCfg := config.NATSConfig{
		Mode:              config.NATSModeExternal,
		URLs:              []string{serverNATS.ClientURL()},
		ClusterName:       clusterName,
		MaxReconnects:     -1,
		ReconnectWait:     50 * time.Millisecond,
		MaxReconnectDelay: 200 * time.Millisecond,
		ReconnectJitter:   0.1,
		JetStream:         config.JetStreamConfig{Enabled: false},
		Embedded:          config.EmbeddedNATSConfig{Host: "127.0.0.1", Port: 4222},
		Dedup:             config.DedupConfig{Enabled: false},
		CircuitBreaker:    config.CircuitBreakerConfig{Enabled: false},
	}
	agentNATS, err := natsmgr.New(agentCfg, log)
	if err != nil {
		t.Fatalf("agent natsmgr.New: %v", err)
	}
	if err := agentNATS.Start(context.Background()); err != nil {
		t.Fatalf("agent natsmgr.Start: %v", err)
	}

	// Single SecurityEnforcer drives both sign (server side via
	// commandSignerAdapter) and verify (agent side). Same secret →
	// HMACs match.
	enforcer, err := agent.NewSecurityEnforcer(agent.SecurityPolicy{
		HMACSecret:    []byte(hmacSecret),
		DefaultPolicy: agent.PolicyAllow,
	}, log)
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}

	executor := agent.NewExecutor(agent.ExecutorConfig{
		Logger:         log,
		KillGrace:      100 * time.Millisecond,
		DefaultTimeout: 5 * time.Second,
	})

	a, err := agent.New(agent.Config{
		AgentID:           agentID,
		HeartbeatInterval: 50 * time.Millisecond,
		MetadataInterval:  60 * time.Millisecond,
		CommandTimeout:    2 * time.Second,
	}, agentNATSAdapter{m: agentNATS}, agentNATS.Subjects(),
		agent.NewGopsutilCollector(log), executor, enforcer, log)
	if err != nil {
		t.Fatalf("agent.New: %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("agent.Start: %v", err)
	}

	// Dispatcher uses the SERVER's NATS (publishes on the embedded
	// bus the agent is subscribed to). Stubs for store + lookup.
	disp, err := controlplane.NewDispatcher(controlplane.DispatcherConfig{
		Store:                 stubCommandStore{},
		Agents:                stubAgentLookup{id: agentID},
		Publisher:             serverNATSPublisher{m: serverNATS},
		Subjects:              serverNATS.Subjects(),
		Signer:                commandSignerAdapter{enf: enforcer},
		Logger:                log,
		DefaultTimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if err := disp.Start(context.Background()); err != nil {
		t.Fatalf("Dispatcher.Start: %v", err)
	}

	// Sibling test subscriber connection — runs alongside the
	// agent + server NATS clients on the same embedded bus so we
	// can observe heartbeat / metadata / response subjects.
	testConn, err := natsclient.Connect(serverNATS.ClientURL(),
		natsclient.Name("epic06-int-test-subscriber"))
	if err != nil {
		t.Fatalf("testConn Connect: %v", err)
	}

	// Caller must explicitly call fix.shutdown() before goleak —
	// t.Cleanup runs AFTER deferred goleak.VerifyNone, which would
	// catch the still-alive goroutines as false-positive leaks.
	return &integrationFixture{
		natsServer:  serverNATS,
		natsAgent:   agentNATS,
		agent:       a,
		dispatcher:  disp,
		testConn:    testConn,
		subBuilder:  serverNATS.Subjects(),
		enforcer:    enforcer,
		clusterName: clusterName,
		agentID:     agentID,
	}
}

func (f *integrationFixture) shutdown() {
	stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// Order matters: stop dispatcher (no more publishes), shutdown
	// agent (drain in-flight commands per Task 11), close test
	// subscriber, shutdown agent's NATS client, shutdown embedded
	// server.
	_ = f.dispatcher.Stop(stopCtx)
	_ = f.agent.Shutdown(stopCtx)
	f.testConn.Close()
	_ = f.natsAgent.Shutdown(stopCtx)
	_ = f.natsServer.Shutdown(stopCtx)
}

// subscribeBuffer registers a NATS subscription that pushes raw
// envelope bytes onto a buffered channel. Returns the channel +
// the subscription handle for cleanup.
func subscribeBuffer(t *testing.T, conn *natsclient.Conn, subject string, buf int) (<-chan []byte, *natsclient.Subscription) {
	t.Helper()
	ch := make(chan []byte, buf)
	sub, err := conn.Subscribe(subject, func(msg *natsclient.Msg) {
		select {
		case ch <- msg.Data:
		default:
			// Drop if buffer full; tests assert on the first N.
		}
	})
	if err != nil {
		t.Fatalf("Subscribe %q: %v", subject, err)
	}
	if err := conn.Flush(); err != nil {
		t.Fatalf("Flush %q: %v", subject, err)
	}
	t.Cleanup(func() { _ = sub.Unsubscribe() })
	return ch, sub
}

func TestEpic06_FullIntegration_HeartbeatMetadataAndCommand(t *testing.T) {
	// goleak guard: catches any goroutine the agent / NATS / dispatcher
	// stack leaks past Shutdown. Epic 06 risk line: "every command
	// goroutine must defer cleanup; verified by goleak in integration test."
	//
	// fix.shutdown() runs explicitly below to guarantee teardown
	// completes BEFORE this defer fires (t.Cleanup runs after
	// defers, which would race with goleak).
	defer goleak.VerifyNone(t,
		// nats.go's per-subscription readLoop / flusher may briefly
		// outlive the connection during embedded-server teardown;
		// not an agent goroutine, ignored.
		goleak.IgnoreTopFunction("github.com/nats-io/nats.go.(*Conn).flusher"),
		goleak.IgnoreTopFunction("github.com/nats-io/nats.go.(*Conn).readLoop"),
		goleak.IgnoreTopFunction("github.com/nats-io/nats.go.(*Conn).waitForMsgs"),
	)

	fix := bootIntegrationFixture(t)
	defer fix.shutdown() // runs BEFORE the goleak.VerifyNone defer (LIFO)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 1. Agent registered (NATS-level): Manager.Health is nil after
	//    a successful connect + JetStream init.
	if err := fix.natsAgent.Health(ctx); err != nil {
		t.Fatalf("agent NATS Health = %v after Start; want nil (registration)", err)
	}

	// 2. Heartbeat: agent publishes on the heartbeat subject.
	hbCh, _ := subscribeBuffer(t, fix.testConn, fix.subBuilder.AgentHeartbeat(), 16)
	select {
	case raw := <-hbCh:
		env, err := envelope.Unmarshal(raw)
		if err != nil {
			t.Fatalf("heartbeat envelope decode: %v", err)
		}
		var hb agent.HeartbeatMetrics
		if err := json.Unmarshal(env.Payload, &hb); err != nil {
			t.Fatalf("heartbeat payload decode: %v", err)
		}
		if hb.AgentID != fix.agentID {
			t.Errorf("heartbeat agent_id = %q, want %q", hb.AgentID, fix.agentID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no heartbeat received within 2s")
	}

	// 3. Metadata: agent publishes on the state subject on its
	//    interval (60ms in this test).
	mdCh, _ := subscribeBuffer(t, fix.testConn, fix.subBuilder.AgentState(fix.agentID), 16)
	select {
	case raw := <-mdCh:
		env, err := envelope.Unmarshal(raw)
		if err != nil {
			t.Fatalf("metadata envelope decode: %v", err)
		}
		var md agent.AgentMetadata
		if err := json.Unmarshal(env.Payload, &md); err != nil {
			t.Fatalf("metadata payload decode: %v", err)
		}
		if md.AgentID != fix.agentID {
			t.Errorf("metadata agent_id = %q, want %q", md.AgentID, fix.agentID)
		}
		if md.Hostname == "" {
			t.Error("metadata hostname empty; gopsutil should populate")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no metadata received within 2s")
	}

	// 4. Command exec: subscribe to the response subject FIRST so
	//    we don't miss the response between dispatch and subscribe.
	respCh, _ := subscribeBuffer(t, fix.testConn,
		fix.subBuilder.AgentResponse(fix.agentID), 4)

	cmdID, err := fix.dispatcher.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID:        fix.agentID,
		Command:        "/bin/echo",
		Args:           []string{"hello-from-epic-06"},
		Principal:      "test",
		TimeoutSeconds: 5,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Wait for the response with the matching CorrelationID.
	deadline := time.After(3 * time.Second)
	var resp agent.CommandResponse
	var respEnv envelope.Envelope
	for {
		select {
		case raw := <-respCh:
			respEnv, err = envelope.Unmarshal(raw)
			if err != nil {
				t.Fatalf("response envelope decode: %v", err)
			}
			if respEnv.CorrelationID != cmdID {
				// Different command's response (none expected, but
				// resilient): keep waiting.
				continue
			}
			if err := json.Unmarshal(respEnv.Payload, &resp); err != nil {
				t.Fatalf("response payload decode: %v", err)
			}
			goto gotResponse
		case <-deadline:
			t.Fatal("no response received within 3s of dispatch")
		}
	}
gotResponse:
	if resp.Rejected {
		t.Fatalf("response.Rejected = true (reason=%q); HMAC sign/verify should pass",
			resp.RejectReason)
	}
	if resp.ExitCode != 0 {
		t.Errorf("response.ExitCode = %d, want 0\nstderr=%q", resp.ExitCode, resp.Stderr)
	}
	if got, want := string(resp.Stdout), "hello-from-epic-06\n"; got != want {
		t.Errorf("response.Stdout = %q, want %q", got, want)
	}
	if resp.AgentID != fix.agentID {
		t.Errorf("response.AgentID = %q, want %q", resp.AgentID, fix.agentID)
	}
	if respEnv.CorrelationID != cmdID {
		t.Errorf("response CorrelationID = %q, want dispatched %q",
			respEnv.CorrelationID, cmdID)
	}

	// 5. Graceful shutdown is verified by t.Cleanup → fix.shutdown().
	//    goleak.VerifyNone (deferred above) catches any leaked goroutine.
}

// serverNATSPublisher adapts internal/nats.Manager to
// controlplane.NATSPublisher for the integration test.
// pkg/api/server's production natsPublisherAdapter is unexported
// — three lines here keeps the test self-contained.
type serverNATSPublisher struct{ m *natsmgr.Manager }

func (a serverNATSPublisher) PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error {
	return a.m.PublishEnvelope(ctx, subject, env)
}

// agentNATSAdapter bridges internal/nats.Manager into the
// agent.NATSClient interface. The cmd/kscore-agent binary has
// its own copy of this adapter (different package); test
// declares a local one to avoid cross-binary imports.
type agentNATSAdapter struct{ m *natsmgr.Manager }

func (a agentNATSAdapter) PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error {
	return a.m.PublishEnvelope(ctx, subject, env)
}

func (a agentNATSAdapter) Subscribe(subject string, h agent.MessageHandler) (agent.Subscription, error) {
	sub, err := a.m.Subscribe(subject, natsmgr.MessageHandler(h))
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (a agentNATSAdapter) Health(ctx context.Context) error {
	return a.m.Health(ctx)
}
