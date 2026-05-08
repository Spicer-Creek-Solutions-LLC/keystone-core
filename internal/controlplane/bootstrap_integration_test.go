package controlplane_test

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	natsmgr "go.keystone-core.io/keystone-core/internal/nats"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// natsSubscriberAdapter mirrors cmd/kscore-server's adapter — bridges
// internal/nats.Manager.Subscribe (which uses nats.MessageHandler /
// Subscription) into the controlplane-shaped versions. Defined here
// so the integration test exercises the production-shaped wiring
// without importing cmd/kscore-server.
type natsSubscriberAdapter struct{ m *natsmgr.Manager }

func (a natsSubscriberAdapter) Subscribe(subject string, h controlplane.MessageHandler) (controlplane.Subscription, error) {
	sub, err := a.m.Subscribe(subject, natsmgr.MessageHandler(h))
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func freePort(t *testing.T) int {
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

func startEmbeddedManager(t *testing.T) *natsmgr.Manager {
	t.Helper()
	cfg := config.NATSConfig{
		Mode:          config.NATSModeEmbedded,
		ClusterName:   "test",
		MaxReconnects: 1,
		ReconnectWait: 100 * time.Millisecond,
		JetStream: config.JetStreamConfig{
			Enabled:        true,
			StoreDir:       filepath.Join(t.TempDir(), "jetstream"),
			MaxStorage:     10 * 1024 * 1024,
			StreamMaxAge:   time.Hour,
			StreamMaxBytes: 1024 * 1024,
			StreamMaxMsgs:  10_000,
			StreamReplicas: 1,
		},
		Embedded: config.EmbeddedNATSConfig{
			Host: "127.0.0.1",
			Port: freePort(t),
		},
		Dedup: config.DedupConfig{
			Enabled:         true,
			WindowDuration:  time.Minute,
			MaxEntries:      1024,
			CleanupInterval: time.Hour,
		},
	}
	m, err := natsmgr.New(cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("nats.New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.Shutdown(stopCtx)
	})
	return m
}

// TestBootstrapHandler_FullRoundTrip is the end-to-end test for the
// Task 9 acceptance bullet. It spins a real NATS Manager, attaches
// the bootstrap handler, then publishes a register message via a
// separate NATS client (simulating an agent). The test reads the
// response on the response subject and asserts the cleartext API
// key is non-empty and CorrelationID matches the inbound MessageID.
func TestBootstrapHandler_FullRoundTrip(t *testing.T) {
	m := startEmbeddedManager(t)
	store := newTestStore(t)

	val := newFakeValidator()
	val.seed("agent-7", []byte{0xde, 0xad, 0xbe, 0xef})

	issuer, err := controlplane.NewAPIKeyIssuer(controlplane.APIKeyIssuerConfig{Keys: store})
	if err != nil {
		t.Fatalf("NewAPIKeyIssuer: %v", err)
	}

	// Publisher adapter mirrors pkg/api/server's natsPublisherAdapter.
	pub := natsPublisherFromManager{m: m}

	subjects := managerSubjects{m: m}
	h, err := controlplane.NewBootstrapHandler(controlplane.BootstrapHandlerConfig{
		Subjects:   subjects,
		Subscriber: natsSubscriberAdapter{m: m},
		Publisher:  pub,
		Store:      store,
		Validator:  val,
		Issuer:     issuer,
	})
	if err != nil {
		t.Fatalf("NewBootstrapHandler: %v", err)
	}
	if err := h.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = h.Stop(context.Background()) })

	// Independent NATS client — simulating the agent.
	conn, err := natsclient.Connect(m.ClientURL())
	if err != nil {
		t.Fatalf("agent connect: %v", err)
	}
	defer conn.Close()

	respCh := make(chan envelope.Envelope, 1)
	respSubj := "kscore.test.bootstrap.agent-7.response"
	subscription, err := conn.Subscribe(respSubj, func(msg *natsclient.Msg) {
		env, err := envelope.Unmarshal(msg.Data)
		if err != nil {
			t.Errorf("unmarshal response: %v", err)
			return
		}
		respCh <- env
	})
	if err != nil {
		t.Fatalf("subscribe response: %v", err)
	}
	defer func() { _ = subscription.Unsubscribe() }()
	if err := conn.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	body, _ := json.Marshal(map[string]any{
		"agent_id": "agent-7",
		"proof":    hex.EncodeToString([]byte{0xde, 0xad, 0xbe, 0xef}),
	})
	registerEnv := envelope.New(body, "kscore.test", envelope.WithMessageID("msg-rt-1"))

	if err := m.PublishEnvelope(context.Background(), "kscore.test.bootstrap.agent-7.register", registerEnv); err != nil {
		t.Fatalf("PublishEnvelope: %v", err)
	}

	select {
	case got := <-respCh:
		if got.CorrelationID != "msg-rt-1" {
			t.Errorf("CorrelationID = %q, want msg-rt-1", got.CorrelationID)
		}
		var creds controlplane.AgentCredentials
		if err := json.Unmarshal(got.Payload, &creds); err != nil {
			t.Fatalf("creds unmarshal: %v (raw=%s)", err, got.Payload)
		}
		if creds.APIKey == "" {
			t.Error("APIKey empty in round-trip response")
		}
		if creds.AgentID != "agent-7" {
			t.Errorf("AgentID = %q, want agent-7", creds.AgentID)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("did not receive response within 3s")
	}

	// Verify the agent record was created.
	rec, err := store.GetAgent(context.Background(), "agent-7")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if rec.ID != "agent-7" {
		t.Errorf("AgentRecord.ID = %q, want agent-7", rec.ID)
	}
}

// natsPublisherFromManager satisfies controlplane.NATSPublisher by
// delegating to a real Manager. Test-local mirror of pkg/api/server's
// natsPublisherAdapter.
type natsPublisherFromManager struct{ m *natsmgr.Manager }

func (n natsPublisherFromManager) PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error {
	return n.m.PublishEnvelope(ctx, subject, env)
}

// managerSubjects satisfies controlplane.Subjects by delegating to a
// real Manager's SubjectBuilder.
type managerSubjects struct{ m *natsmgr.Manager }

func (s managerSubjects) AgentCommand(agentID string) string { return s.m.Subjects().AgentCommand(agentID) }
func (s managerSubjects) BootstrapRegisterPattern() string   { return s.m.Subjects().BootstrapRegisterPattern() }
func (s managerSubjects) BootstrapResponse(agentID string) string {
	return s.m.Subjects().BootstrapResponse(agentID)
}
func (s managerSubjects) Cluster() string { return s.m.Subjects().Cluster() }
func (s managerSubjects) Prefix() string  { return s.m.Subjects().Prefix() }

// Suppress unused-import warning if test build pulls only one of the
// helpers above.
var _ = sync.Mutex{}
