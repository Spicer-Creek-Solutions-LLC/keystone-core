package agent

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// fakeSubjects is a hand-rolled implementation of the Subjects
// interface. internal/nats.SubjectBuilder satisfies the same
// interface structurally; we duplicate the minimal logic here so
// the test isn't coupled to the production builder.
type fakeSubjects struct{ cluster string }

func (f fakeSubjects) AgentHeartbeat() string { return "kscore." + f.cluster + ".agent.heartbeat" }
func (f fakeSubjects) AgentCommand(id string) string {
	return "kscore." + f.cluster + ".agent." + id + ".command"
}
func (f fakeSubjects) AgentResponse(id string) string {
	return "kscore." + f.cluster + ".agent." + id + ".response"
}
func (f fakeSubjects) AgentState(id string) string {
	return "kscore." + f.cluster + ".agent." + id + ".state"
}
func (f fakeSubjects) Cluster() string { return f.cluster }
func (f fakeSubjects) Prefix() string  { return "kscore." + f.cluster }

// fakeSubscription records whether Unsubscribe was called.
type fakeSubscription struct {
	mu       sync.Mutex
	unsubbed bool
	err      error
}

func (s *fakeSubscription) Unsubscribe() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.unsubbed = true
	return s.err
}

// fakeNATSClient captures publishes per-subject and exposes the
// subscriber callback so tests can deliver synthetic commands.
type fakeNATSClient struct {
	mu       sync.Mutex
	subSubj  string
	handler  MessageHandler
	sub      *fakeSubscription
	subErr   error
	publishes map[string][]envelope.Envelope
	pubErr    error
}

func newFakeNATS() *fakeNATSClient {
	return &fakeNATSClient{publishes: map[string][]envelope.Envelope{}}
}

func (f *fakeNATSClient) PublishEnvelope(_ context.Context, subject string, env envelope.Envelope) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.pubErr != nil {
		return f.pubErr
	}
	f.publishes[subject] = append(f.publishes[subject], env)
	return nil
}

func (f *fakeNATSClient) Subscribe(subject string, h MessageHandler) (Subscription, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.subErr != nil {
		return nil, f.subErr
	}
	f.subSubj = subject
	f.handler = h
	f.sub = &fakeSubscription{}
	return f.sub, nil
}

func (f *fakeNATSClient) Health(_ context.Context) error { return nil }

func (f *fakeNATSClient) publishCount(subject string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.publishes[subject])
}

func (f *fakeNATSClient) deliver(t *testing.T, subject string, env envelope.Envelope) error {
	t.Helper()
	f.mu.Lock()
	h := f.handler
	f.mu.Unlock()
	if h == nil {
		t.Fatal("deliver: no handler attached")
	}
	return h(context.Background(), subject, env)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestAgent(t *testing.T, opts ...func(*Config)) (*Agent, *fakeNATSClient, fakeSubjects) {
	t.Helper()
	cfg := Config{
		AgentID:           "agent-1",
		HeartbeatInterval: 10 * time.Millisecond,
		MetadataInterval:  20 * time.Millisecond,
		CommandTimeout:    time.Second,
	}
	for _, o := range opts {
		o(&cfg)
	}
	nats := newFakeNATS()
	subjects := fakeSubjects{cluster: "test"}
	a, err := New(cfg, nats, subjects, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, nats, subjects
}

func TestNew_RequiresFields(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*Config) (NATSClient, Subjects)
	}{
		{"nil agent id", func(c *Config) (NATSClient, Subjects) {
			c.AgentID = ""
			return newFakeNATS(), fakeSubjects{cluster: "test"}
		}},
		{"nil nats", func(c *Config) (NATSClient, Subjects) {
			return nil, fakeSubjects{cluster: "test"}
		}},
		{"nil subjects", func(c *Config) (NATSClient, Subjects) {
			return newFakeNATS(), nil
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{AgentID: "agent-1"}
			nats, subj := tc.mut(&cfg)
			if _, err := New(cfg, nats, subj, testLogger()); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestNew_NilLoggerDefaults(t *testing.T) {
	_, err := New(Config{AgentID: "agent-1"}, newFakeNATS(), fakeSubjects{cluster: "test"}, nil)
	if err != nil {
		t.Errorf("New with nil logger: %v", err)
	}
}

func TestNew_DefaultsAppliedWhenZero(t *testing.T) {
	a, err := New(Config{AgentID: "agent-1"}, newFakeNATS(), fakeSubjects{cluster: "test"}, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if a.cfg.HeartbeatInterval != defaultHeartbeatInterval {
		t.Errorf("HeartbeatInterval = %s, want default %s", a.cfg.HeartbeatInterval, defaultHeartbeatInterval)
	}
	if a.cfg.MetadataInterval != defaultMetadataInterval {
		t.Errorf("MetadataInterval = %s, want default %s", a.cfg.MetadataInterval, defaultMetadataInterval)
	}
	if a.cfg.CommandTimeout != defaultCommandTimeout {
		t.Errorf("CommandTimeout = %s, want default %s", a.cfg.CommandTimeout, defaultCommandTimeout)
	}
}

func TestAgent_StartSubscribesAndStartsLoops(t *testing.T) {
	a, nats, subj := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = a.Shutdown(stopCtx)
	})

	nats.mu.Lock()
	wantSub := subj.AgentCommand("agent-1")
	gotSub := nats.subSubj
	nats.mu.Unlock()
	if gotSub != wantSub {
		t.Errorf("Subscribe subject = %q, want %q", gotSub, wantSub)
	}
}

func TestAgent_StartIdempotent(t *testing.T) {
	a, _, _ := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })
	if err := a.Start(context.Background()); err != nil {
		t.Errorf("second Start: %v, want nil (idempotent)", err)
	}
}

func TestAgent_ShutdownIdempotent(t *testing.T) {
	a, _, _ := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("first Shutdown: %v", err)
	}
	if err := a.Shutdown(context.Background()); err != nil {
		t.Errorf("second Shutdown: %v, want nil", err)
	}
}

func TestAgent_ShutdownBeforeStart(t *testing.T) {
	a, _, _ := newTestAgent(t)
	if err := a.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown before Start: %v, want nil", err)
	}
}

func TestAgent_StartAfterShutdownRejected(t *testing.T) {
	a, _, _ := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := a.Start(context.Background()); err == nil {
		t.Error("Start after Shutdown: nil, want error")
	}
}

func TestAgent_StartFailsOnSubscribeError(t *testing.T) {
	a, nats, _ := newTestAgent(t)
	nats.subErr = errors.New("synthetic subscribe failure")
	if err := a.Start(context.Background()); err == nil {
		t.Fatal("Start: nil, want error")
	}
}

func TestAgent_HeartbeatPublishesOnInterval(t *testing.T) {
	a, nats, subj := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	// Wait for at least 2 heartbeats. Interval is 10ms; 100ms gives
	// generous headroom on slow CI.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if nats.publishCount(subj.AgentHeartbeat()) >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("heartbeat publishes = %d after 500ms, want >=2", nats.publishCount(subj.AgentHeartbeat()))
}

func TestAgent_MetadataPublishesOnInterval(t *testing.T) {
	a, nats, subj := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	deadline := time.Now().Add(500 * time.Millisecond)
	stateSubject := subj.AgentState("agent-1")
	for time.Now().Before(deadline) {
		if nats.publishCount(stateSubject) >= 2 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Errorf("metadata publishes = %d after 500ms, want >=2", nats.publishCount(stateSubject))
}

func TestAgent_ShutdownStopsLoops(t *testing.T) {
	a, nats, subj := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Let a few heartbeats land.
	time.Sleep(50 * time.Millisecond)
	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	beforeStop := nats.publishCount(subj.AgentHeartbeat())
	time.Sleep(80 * time.Millisecond)
	afterStop := nats.publishCount(subj.AgentHeartbeat())
	if afterStop != beforeStop {
		t.Errorf("heartbeat publishes after Shutdown: %d → %d (loop didn't stop)", beforeStop, afterStop)
	}

	if !nats.sub.unsubbed {
		t.Error("Unsubscribe was not called on Shutdown")
	}
}

func TestAgent_CommandHandlerInvokedOnDelivery(t *testing.T) {
	a, nats, subj := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	env := envelope.New([]byte(`{"command":"uptime"}`), subj.Prefix(),
		envelope.WithMessageID("cmd-1"))
	if err := nats.deliver(t, subj.AgentCommand("agent-1"), env); err != nil {
		t.Errorf("handler returned error: %v", err)
	}
}

func TestAgent_HeartbeatPublishErrorLogged(t *testing.T) {
	// Set publish error AFTER Start so Subscribe still succeeds.
	a, nats, _ := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	nats.mu.Lock()
	nats.pubErr = errors.New("synthetic publish failure")
	nats.mu.Unlock()

	// Loop must not panic / exit despite repeated publish failures.
	time.Sleep(60 * time.Millisecond)
	// No assertion beyond "doesn't panic" — this is a smoke test for
	// the publish-error handling path.
}
