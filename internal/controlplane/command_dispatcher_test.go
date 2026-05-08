package controlplane_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// fakeSubjects is a hand-rolled subject builder used by dispatcher
// tests. Mirrors the v1.0 hierarchy that internal/nats.SubjectBuilder
// produces so existing assertions on captured subject strings
// (e.g., "kscore.default.agent.agent-1.command") keep passing.
type fakeSubjects struct {
	cluster string
}

func (f fakeSubjects) AgentCommand(agentID string) string {
	return "kscore." + f.cluster + ".agent." + agentID + ".command"
}

func (f fakeSubjects) BootstrapRegisterPattern() string {
	return "kscore." + f.cluster + ".bootstrap.*.register"
}

func (f fakeSubjects) BootstrapResponse(agentID string) string {
	return "kscore." + f.cluster + ".bootstrap." + agentID + ".response"
}

func (f fakeSubjects) Cluster() string { return f.cluster }

func (f fakeSubjects) Prefix() string { return "kscore." + f.cluster }

// fakePublisher captures every Publish call for inspection. It is
// goroutine-safe so concurrent dispatcher tests can still assert on
// the captured stream.
type fakePublisher struct {
	mu       sync.Mutex
	calls    []publishCall
	failNext bool
	failErr  error
}

type publishCall struct {
	subject  string
	envelope envelope.Envelope
}

func (p *fakePublisher) PublishEnvelope(_ context.Context, subject string, env envelope.Envelope) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.failNext {
		p.failNext = false
		return p.failErr
	}
	// Deep-copy the payload so later test mutations cannot retroactively
	// edit recorded history.
	cp := make([]byte, len(env.Payload))
	copy(cp, env.Payload)
	env.Payload = cp
	p.calls = append(p.calls, publishCall{subject: subject, envelope: env})
	return nil
}

func (p *fakePublisher) Calls() []publishCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]publishCall, len(p.calls))
	copy(out, p.calls)
	return out
}

// dispatcherFixture wires a ConnectionManager + CommandDispatcher
// against a real SQLite store with a registered "agent-1". Tests
// extend or replace pieces as needed.
type dispatcherFixture struct {
	store     state.Store
	mgr       *controlplane.ConnectionManager
	publisher *fakePublisher
	clk       *fakeClock
	disp      *controlplane.CommandDispatcher
}

func newDispatcherFixture(t *testing.T, opts ...func(*controlplane.DispatcherConfig)) *dispatcherFixture {
	t.Helper()
	store := newTestStore(t)
	clk := newFakeClock(time.Date(2026, 5, 7, 0, 0, 0, 0, time.UTC))

	mgr := mustNew(t, controlplane.Config{
		Store:             store,
		HeartbeatInterval: time.Hour,
		Clock:             clk.Now,
	})
	mustStart(t, mgr)
	t.Cleanup(func() { stopOK(t, mgr) })

	if err := mgr.Register(context.Background(), newAgent("agent-1")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	pub := &fakePublisher{}
	cfg := controlplane.DispatcherConfig{
		Store:                store,
		Agents:               mgr,
		Publisher:            pub,
		Subjects:             fakeSubjects{cluster: "default"},
		DefaultTimeoutSeconds: 60,
		RetentionWindow:      time.Hour,
		RetentionInterval:    time.Hour,
		TimeoutCheckInterval: 5 * time.Millisecond,
		Clock:                clk.Now,
		NewID:                seqIDGenerator(),
	}
	for _, o := range opts {
		o(&cfg)
	}
	disp, err := controlplane.NewDispatcher(cfg)
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if err := disp.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := disp.Stop(ctx); err != nil {
			t.Errorf("dispatcher Stop: %v", err)
		}
	})

	return &dispatcherFixture{
		store: store, mgr: mgr, publisher: pub, clk: clk, disp: disp,
	}
}

func seqIDGenerator() func() string {
	var n atomic.Int64
	return func() string {
		return fmt.Sprintf("cmd-%d", n.Add(1))
	}
}

func TestNewDispatcher_Validation(t *testing.T) {
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)
	defer stopOK(t, mgr)
	pub := &fakePublisher{}

	subj := fakeSubjects{cluster: "default"}
	cases := []struct {
		name string
		cfg  controlplane.DispatcherConfig
	}{
		{"nil store", controlplane.DispatcherConfig{Agents: mgr, Publisher: pub, Subjects: subj}},
		{"nil agents", controlplane.DispatcherConfig{Store: store, Publisher: pub, Subjects: subj}},
		{"nil publisher", controlplane.DispatcherConfig{Store: store, Agents: mgr, Subjects: subj}},
		{"nil subjects", controlplane.DispatcherConfig{Store: store, Agents: mgr, Publisher: pub}},
		{"negative retention window", controlplane.DispatcherConfig{
			Store: store, Agents: mgr, Publisher: pub, Subjects: subj, RetentionWindow: -time.Second,
		}},
		{"negative retention interval", controlplane.DispatcherConfig{
			Store: store, Agents: mgr, Publisher: pub, Subjects: subj, RetentionInterval: -time.Second,
		}},
		{"negative timeout check interval", controlplane.DispatcherConfig{
			Store: store, Agents: mgr, Publisher: pub, Subjects: subj, TimeoutCheckInterval: -time.Second,
		}},
		{"negative default timeout", controlplane.DispatcherConfig{
			Store: store, Agents: mgr, Publisher: pub, Subjects: subj, DefaultTimeoutSeconds: -1,
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := controlplane.NewDispatcher(tc.cfg); err == nil {
				t.Error("expected error")
			}
		})
	}

	// All defaults applied — should succeed.
	if _, err := controlplane.NewDispatcher(controlplane.DispatcherConfig{
		Store: store, Agents: mgr, Publisher: pub, Subjects: subj,
	}); err != nil {
		t.Errorf("default cfg: %v", err)
	}
}

func TestDispatch_HappyPath(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	id, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID: "agent-1",
		Command: "uptime",
		Args:    []string{"-p"},
		TimeoutSeconds: 30,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if id != "cmd-1" {
		t.Errorf("ID = %q, want cmd-1", id)
	}

	calls := f.publisher.Calls()
	if len(calls) != 1 {
		t.Fatalf("publish calls = %d, want 1", len(calls))
	}
	if want := "kscore.default.agent.agent-1.command"; calls[0].subject != want {
		t.Errorf("subject = %q, want %q", calls[0].subject, want)
	}

	// Envelope-level assertions: CorrelationID == command ID, cluster
	// prefix matches the configured cluster, MessageID stamped.
	got := calls[0].envelope
	if got.CorrelationID != "cmd-1" {
		t.Errorf("CorrelationID = %q, want cmd-1", got.CorrelationID)
	}
	if got.ClusterPrefix != "kscore.default" {
		t.Errorf("ClusterPrefix = %q, want kscore.default", got.ClusterPrefix)
	}
	if got.MessageID == "" {
		t.Error("MessageID empty")
	}

	// Inner payload assertions: the commandMessage shape is preserved.
	var msg map[string]any
	if err := json.Unmarshal(got.Payload, &msg); err != nil {
		t.Fatalf("payload not JSON: %v", err)
	}
	if msg["id"] != "cmd-1" || msg["command"] != "uptime" {
		t.Errorf("payload = %v", msg)
	}
	if msg["timeout_seconds"].(float64) != 30 {
		t.Errorf("timeout_seconds = %v", msg["timeout_seconds"])
	}

	stored, err := f.store.GetCommand(ctx, id)
	if err != nil {
		t.Fatalf("GetCommand: %v", err)
	}
	if stored.Status != state.CommandStatusRunning {
		t.Errorf("Status = %q, want running", stored.Status)
	}
	if f.disp.InFlight() != 1 {
		t.Errorf("InFlight = %d, want 1", f.disp.InFlight())
	}
}

func TestDispatch_DefaultTimeoutApplied(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	id, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID: "agent-1",
		Command: "uptime",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	stored, err := f.store.GetCommand(ctx, id)
	if err != nil {
		t.Fatalf("GetCommand: %v", err)
	}
	if stored.TimeoutSeconds != 60 {
		t.Errorf("TimeoutSeconds = %d, want 60", stored.TimeoutSeconds)
	}
}

func TestDispatch_RejectsInvalidRequests(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	if _, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{Command: "x"}); !errors.Is(err, controlplane.ErrInvalidDispatch) {
		t.Errorf("missing AgentID: err = %v", err)
	}
	if _, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{AgentID: "agent-1"}); !errors.Is(err, controlplane.ErrInvalidDispatch) {
		t.Errorf("missing Command: err = %v", err)
	}
}

func TestDispatch_UnknownAgent(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	_, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID: "ghost", Command: "uptime",
	})
	if !errors.Is(err, controlplane.ErrAgentUnreachable) {
		t.Fatalf("err = %v, want ErrAgentUnreachable", err)
	}
	if len(f.publisher.Calls()) != 0 {
		t.Errorf("publisher called for unknown agent")
	}
}

func TestDispatch_DisabledAgent(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	if err := f.mgr.Disable(ctx, "agent-1"); err != nil {
		t.Fatalf("Disable: %v", err)
	}
	_, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID: "agent-1", Command: "uptime",
	})
	if !errors.Is(err, controlplane.ErrAgentUnreachable) {
		t.Fatalf("err = %v, want ErrAgentUnreachable", err)
	}
}

func TestDispatch_PublishFailureMarksFailed(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)
	f.publisher.failNext = true
	f.publisher.failErr = errors.New("nats: timeout")

	id, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID: "agent-1", Command: "uptime",
	})
	if !errors.Is(err, controlplane.ErrAgentUnreachable) {
		t.Fatalf("err = %v, want ErrAgentUnreachable", err)
	}
	// On publish failure the dispatcher does not surface the generated
	// ID — but the stored row exists. ListCommands by agent will turn
	// it up; assert its terminal state.
	commands, lerr := f.store.ListCommands(ctx, state.CommandFilter{AgentID: "agent-1"})
	if lerr != nil {
		t.Fatalf("ListCommands: %v", lerr)
	}
	if len(commands) != 1 {
		t.Fatalf("commands = %d, want 1", len(commands))
	}
	got := commands[0]
	if got.Status != state.CommandStatusFailed {
		t.Errorf("Status = %q, want failed", got.Status)
	}
	if got.Stderr == "" {
		t.Errorf("Stderr empty, want publish error")
	}
	if id != "" {
		t.Errorf("Dispatch returned ID %q on failure", id)
	}
}

func TestRecordResult_HappyPath(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	id, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID: "agent-1", Command: "uptime",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	f.clk.Advance(time.Second)
	if err := f.disp.RecordResult(ctx, id, state.CommandResult{
		Status:   state.CommandStatusCompleted,
		ExitCode: 0,
		Stdout:   "12:34 up 1 day",
	}); err != nil {
		t.Fatalf("RecordResult: %v", err)
	}

	stored, err := f.store.GetCommand(ctx, id)
	if err != nil {
		t.Fatalf("GetCommand: %v", err)
	}
	if stored.Status != state.CommandStatusCompleted {
		t.Errorf("Status = %q", stored.Status)
	}
	if stored.Stdout != "12:34 up 1 day" {
		t.Errorf("Stdout = %q", stored.Stdout)
	}
	if !stored.CompletedAt.Equal(f.clk.Now()) {
		t.Errorf("CompletedAt = %v, want %v", stored.CompletedAt, f.clk.Now())
	}
	if f.disp.InFlight() != 0 {
		t.Errorf("InFlight = %d, want 0", f.disp.InFlight())
	}
}

func TestRecordResult_UnknownID(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	err := f.disp.RecordResult(ctx, "ghost", state.CommandResult{Status: state.CommandStatusCompleted})
	if !errors.Is(err, controlplane.ErrCommandNotFound) {
		t.Fatalf("err = %v, want ErrCommandNotFound", err)
	}
}

func TestRecordResult_AlreadyFinalized(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	id, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID: "agent-1", Command: "uptime",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	if err := f.disp.RecordResult(ctx, id, state.CommandResult{
		Status: state.CommandStatusCompleted, ExitCode: 0,
	}); err != nil {
		t.Fatalf("first RecordResult: %v", err)
	}
	err = f.disp.RecordResult(ctx, id, state.CommandResult{
		Status: state.CommandStatusCompleted, ExitCode: 0,
	})
	if !errors.Is(err, controlplane.ErrCommandFinalized) {
		t.Fatalf("err = %v, want ErrCommandFinalized", err)
	}
}

func TestTimeoutWatcher_TransitionsExpiredCommands(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	id, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID:        "agent-1",
		Command:        "uptime",
		TimeoutSeconds: 1,
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}

	// Advance well past the deadline, then wait for the watcher to fire.
	f.clk.Advance(time.Hour)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		stored, err := f.store.GetCommand(ctx, id)
		if err != nil {
			t.Fatalf("GetCommand: %v", err)
		}
		if stored.Status == state.CommandStatusTimeout {
			if f.disp.InFlight() != 0 {
				t.Errorf("InFlight = %d after timeout", f.disp.InFlight())
			}
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("command never transitioned to timeout")
}

func TestRetentionLoop_DeletesOldTerminalCommands(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t, func(c *controlplane.DispatcherConfig) {
		c.RetentionWindow = 10 * time.Second
		c.RetentionInterval = 5 * time.Millisecond
	})

	// Dispatch + finalize: terminal completed.
	id, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID: "agent-1", Command: "uptime",
	})
	if err != nil {
		t.Fatalf("Dispatch: %v", err)
	}
	f.clk.Advance(time.Second)
	if err := f.disp.RecordResult(ctx, id, state.CommandResult{
		Status: state.CommandStatusCompleted, ExitCode: 0,
	}); err != nil {
		t.Fatalf("RecordResult: %v", err)
	}

	// Dispatch a second one and leave it running — must NOT be pruned.
	pendingID, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
		AgentID: "agent-1", Command: "uptime",
	})
	if err != nil {
		t.Fatalf("Dispatch 2: %v", err)
	}

	// Advance well past the retention window, wait for the loop.
	f.clk.Advance(time.Minute)
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if _, err := f.store.GetCommand(ctx, id); errors.Is(err, state.ErrNotFound) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if _, err := f.store.GetCommand(ctx, id); !errors.Is(err, state.ErrNotFound) {
		t.Errorf("terminal command not pruned: %v", err)
	}
	if _, err := f.store.GetCommand(ctx, pendingID); err != nil {
		t.Errorf("running command was pruned: %v", err)
	}
}

func TestStop_IdempotentAndBoundedByCtx(t *testing.T) {
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	mustStart(t, mgr)
	defer stopOK(t, mgr)
	pub := &fakePublisher{}

	disp, err := controlplane.NewDispatcher(controlplane.DispatcherConfig{
		Store: store, Agents: mgr, Publisher: pub,
		Subjects:             fakeSubjects{cluster: "default"},
		RetentionInterval:    time.Hour,
		TimeoutCheckInterval: time.Hour,
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if err := disp.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := disp.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := disp.Stop(stopCtx); err != nil {
		t.Fatalf("second Stop: %v", err)
	}

	// Mutating ops after Stop must reject.
	if _, err := disp.Dispatch(context.Background(), controlplane.DispatchRequest{
		AgentID: "agent-1", Command: "uptime",
	}); !errors.Is(err, controlplane.ErrClosed) {
		t.Fatalf("Dispatch after Stop = %v, want ErrClosed", err)
	}
}

func TestDispatch_BeforeStartFails(t *testing.T) {
	store := newTestStore(t)
	mgr := mustNew(t, controlplane.Config{Store: store, HeartbeatInterval: time.Hour})
	disp, err := controlplane.NewDispatcher(controlplane.DispatcherConfig{
		Store: store, Agents: mgr, Publisher: &fakePublisher{},
		Subjects: fakeSubjects{cluster: "default"},
	})
	if err != nil {
		t.Fatalf("NewDispatcher: %v", err)
	}
	if _, err := disp.Dispatch(context.Background(), controlplane.DispatchRequest{
		AgentID: "agent-1", Command: "uptime",
	}); !errors.Is(err, controlplane.ErrNotStarted) {
		t.Fatalf("err = %v, want ErrNotStarted", err)
	}
}

func TestConcurrentDispatchAndRecordResult(t *testing.T) {
	ctx := context.Background()
	f := newDispatcherFixture(t)

	const n = 32
	ids := make(chan string, n)

	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			id, err := f.disp.Dispatch(ctx, controlplane.DispatchRequest{
				AgentID: "agent-1", Command: "uptime",
			})
			if err != nil {
				t.Errorf("Dispatch: %v", err)
				return
			}
			ids <- id
		}()
	}
	wg.Wait()
	close(ids)

	for id := range ids {
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			if err := f.disp.RecordResult(ctx, id, state.CommandResult{
				Status: state.CommandStatusCompleted,
			}); err != nil {
				t.Errorf("RecordResult %s: %v", id, err)
			}
		}(id)
	}
	wg.Wait()

	if f.disp.InFlight() != 0 {
		t.Errorf("InFlight = %d, want 0", f.disp.InFlight())
	}
}
