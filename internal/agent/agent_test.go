// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/goleak"

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
func (f fakeSubjects) AgentConverge(id string) string {
	return "kscore." + f.cluster + ".agent." + id + ".converge"
}
func (f fakeSubjects) AgentConvergeResult(id string) string {
	return "kscore." + f.cluster + ".agent." + id + ".converge.result"
}
func (f fakeSubjects) BootstrapRegister(id string) string {
	return "kscore." + f.cluster + ".bootstrap." + id + ".register"
}
func (f fakeSubjects) BootstrapResponse(id string) string {
	return "kscore." + f.cluster + ".bootstrap." + id + ".response"
}
func (f fakeSubjects) SecretRequest() string {
	return "kscore." + f.cluster + ".secret.request"
}
func (f fakeSubjects) SecretResponse(id string) string {
	return "kscore." + f.cluster + ".secret." + id + ".response"
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
	mu sync.Mutex
	// subSubj is the COMMAND subscription's subject. The agent also
	// subscribes to its converge subject, so the last-write-wins field
	// alone stopped being meaningful — handlers records every one.
	subSubj  string
	handler  MessageHandler
	handlers map[string]MessageHandler
	subjects []string
	sub      *fakeSubscription
	subs     []*fakeSubscription
	subErr   error
	// subErrAfter, when > 0, lets the Nth subscribe succeed and fails
	// the ones after it — used to exercise the half-subscribed unwind.
	subErrAfter int
	publishes   map[string][]envelope.Envelope
	pubErr      error
}

func newFakeNATS() *fakeNATSClient {
	return &fakeNATSClient{
		publishes: map[string][]envelope.Envelope{},
		handlers:  map[string]MessageHandler{},
	}
}

// handlerFor returns the callback registered for subject.
func (f *fakeNATSClient) handlerFor(subject string) MessageHandler {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.handlers[subject]
}

// captured returns the envelopes published to subject.
func (f *fakeNATSClient) captured(subject string) []envelope.Envelope {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]envelope.Envelope(nil), f.publishes[subject]...)
}

// subscribed reports whether subject was subscribed.
func (f *fakeNATSClient) subscribed(subject string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.subjects {
		if s == subject {
			return true
		}
	}
	return false
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
	if f.subErr != nil && (f.subErrAfter == 0 || len(f.subjects) >= f.subErrAfter) {
		return nil, f.subErr
	}
	if f.handlers == nil {
		f.handlers = map[string]MessageHandler{}
	}
	f.handlers[subject] = h
	f.subjects = append(f.subjects, subject)
	sub := &fakeSubscription{}
	f.subs = append(f.subs, sub)
	// Keep the single-value fields pointing at the COMMAND subscription
	// so existing assertions keep meaning what they meant.
	if strings.HasSuffix(subject, ".command") {
		f.subSubj = subject
		f.handler = h
		f.sub = sub
	}
	return sub, nil
}

func (f *fakeNATSClient) Health(_ context.Context) error { return nil }

func (f *fakeNATSClient) publishCount(subject string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.publishes[subject])
}

// hasHandler reports whether a subscription exists for subject. Used
// to assert subscribe-before-publish ordering.
func (f *fakeNATSClient) hasHandler(subject string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.handlers[subject]
	return ok
}

func (f *fakeNATSClient) deliver(t *testing.T, subject string, env envelope.Envelope) error {
	t.Helper()
	f.mu.Lock()
	// Route by subject — the agent has more than one subscription now,
	// so delivering to whichever handler registered last would silently
	// send commands to the converge handler and vice versa.
	h, ok := f.handlers[subject]
	if !ok {
		h = f.handler
	}
	f.mu.Unlock()
	if h == nil {
		t.Fatal("deliver: no handler attached")
	}
	return h(context.Background(), subject, env)
}

// collectFor polls until at least n envelopes arrive on subject or
// the deadline lapses. Returns a copy of the captured slice. Used by
// command-flow tests to assert on the published response.
func (f *fakeNATSClient) collectFor(t *testing.T, subject string, n int, timeout time.Duration) []envelope.Envelope {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		f.mu.Lock()
		got := append([]envelope.Envelope(nil), f.publishes[subject]...)
		f.mu.Unlock()
		if len(got) >= n {
			return got
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("subject %q: got %d envelopes within %s, want >=%d", subject, f.publishCount(subject), timeout, n)
	return nil
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

// fakeMetricsCollector returns deterministic canned values so the
// agent_test publishes can be asserted on payload contents (Task 3).
// Production wiring uses gopsutilCollector instead.
type fakeMetricsCollector struct {
	cpu    float64
	mem    float64
	host   string
	labels map[string]string
}

func (f *fakeMetricsCollector) Heartbeat(_ context.Context, agentID string) HeartbeatMetrics {
	return HeartbeatMetrics{
		AgentID:    agentID,
		TS:         time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
		CPUPercent: f.cpu,
		MemPercent: f.mem,
	}
}

func (f *fakeMetricsCollector) Metadata(_ context.Context, agentID string, labels map[string]string) AgentMetadata {
	if labels == nil {
		labels = f.labels
	}
	return AgentMetadata{
		AgentID:  agentID,
		TS:       time.Date(2026, 5, 8, 12, 0, 0, 0, time.UTC),
		Hostname: f.host,
		OS:       "linux",
		Labels:   labels,
	}
}

// testEnforcer returns a SecurityEnforcer with HMAC disabled
// (empty secret) and DefaultPolicy=allow — friendly defaults for
// agent-lifecycle tests that aren't exercising the security gate.
// Tests that assert reject paths construct their own enforcer.
func testEnforcer(t *testing.T) *SecurityEnforcer {
	t.Helper()
	enf, err := NewSecurityEnforcer(SecurityPolicy{
		DefaultPolicy: PolicyAllow,
	}, testLogger())
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	return enf
}

// testExecutor returns an Executor with tight timeouts so tests
// don't pay for the production 5m default.
func testExecutor(t *testing.T) *Executor {
	t.Helper()
	return NewExecutor(ExecutorConfig{
		Logger:         testLogger(),
		KillGrace:      50 * time.Millisecond,
		DefaultTimeout: 2 * time.Second,
	})
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
	metrics := &fakeMetricsCollector{cpu: 12.5, mem: 33.0, host: "test-host"}
	a, err := New(cfg, nats, subjects, metrics, testExecutor(t), testEnforcer(t), testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, nats, subjects
}

func TestNew_RequiresFields(t *testing.T) {
	type deps struct {
		nats     NATSClient
		subjects Subjects
		metrics  MetricsCollector
		executor *Executor
		enforcer *SecurityEnforcer
	}
	good := func(t *testing.T) deps {
		return deps{
			nats:     newFakeNATS(),
			subjects: fakeSubjects{cluster: "test"},
			metrics:  &fakeMetricsCollector{},
			executor: testExecutor(t),
			enforcer: testEnforcer(t),
		}
	}
	cases := []struct {
		name string
		mut  func(*Config, *deps)
	}{
		{"nil agent id", func(c *Config, _ *deps) { c.AgentID = "" }},
		{"nil nats", func(_ *Config, d *deps) { d.nats = nil }},
		{"nil subjects", func(_ *Config, d *deps) { d.subjects = nil }},
		{"nil metrics", func(_ *Config, d *deps) { d.metrics = nil }},
		{"nil executor", func(_ *Config, d *deps) { d.executor = nil }},
		{"nil enforcer", func(_ *Config, d *deps) { d.enforcer = nil }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := Config{AgentID: "agent-1"}
			d := good(t)
			tc.mut(&cfg, &d)
			if _, err := New(cfg, d.nats, d.subjects, d.metrics, d.executor, d.enforcer, testLogger()); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestNew_NilLoggerDefaults(t *testing.T) {
	_, err := New(Config{AgentID: "agent-1"}, newFakeNATS(), fakeSubjects{cluster: "test"}, &fakeMetricsCollector{}, testExecutor(t), testEnforcer(t), nil)
	if err != nil {
		t.Errorf("New with nil logger: %v", err)
	}
}

func TestNew_DefaultsAppliedWhenZero(t *testing.T) {
	a, err := New(Config{AgentID: "agent-1"}, newFakeNATS(), fakeSubjects{cluster: "test"}, &fakeMetricsCollector{}, testExecutor(t), testEnforcer(t), testLogger())
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

func TestAgent_HeartbeatPayloadContainsMetrics(t *testing.T) {
	a, nats, subj := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if nats.publishCount(subj.AgentHeartbeat()) >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	nats.mu.Lock()
	envs := append([]envelope.Envelope(nil), nats.publishes[subj.AgentHeartbeat()]...)
	nats.mu.Unlock()
	if len(envs) == 0 {
		t.Fatal("no heartbeat envelope captured within 500ms")
	}

	var hb HeartbeatMetrics
	if err := json.Unmarshal(envs[0].Payload, &hb); err != nil {
		t.Fatalf("Unmarshal: %v (raw=%s)", err, envs[0].Payload)
	}
	if hb.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", hb.AgentID)
	}
	if hb.CPUPercent != 12.5 {
		t.Errorf("CPUPercent = %v, want 12.5 (canned)", hb.CPUPercent)
	}
	if hb.MemPercent != 33.0 {
		t.Errorf("MemPercent = %v, want 33.0 (canned)", hb.MemPercent)
	}
}

func TestAgent_MetadataPayloadContainsHostFields(t *testing.T) {
	a, nats, subj := newTestAgent(t)
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	stateSubject := subj.AgentState("agent-1")
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if nats.publishCount(stateSubject) >= 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	nats.mu.Lock()
	envs := append([]envelope.Envelope(nil), nats.publishes[stateSubject]...)
	nats.mu.Unlock()
	if len(envs) == 0 {
		t.Fatal("no metadata envelope captured within 500ms")
	}

	var md AgentMetadata
	if err := json.Unmarshal(envs[0].Payload, &md); err != nil {
		t.Fatalf("Unmarshal: %v (raw=%s)", err, envs[0].Payload)
	}
	if md.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want agent-1", md.AgentID)
	}
	if md.Hostname != "test-host" {
		t.Errorf("Hostname = %q, want test-host (canned)", md.Hostname)
	}
	if md.OS != "linux" {
		t.Errorf("OS = %q, want linux", md.OS)
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

	// Send a malformed payload; handler should not error and should
	// NOT publish a response (unmarshal failure logs and drops).
	env := envelope.New([]byte(`not-json`), subj.Prefix(),
		envelope.WithMessageID("cmd-bad"))
	if err := nats.deliver(t, subj.AgentCommand("agent-1"), env); err != nil {
		t.Errorf("handler returned error: %v", err)
	}
	if got := nats.publishCount(subj.AgentResponse("agent-1")); got != 0 {
		t.Errorf("response published for malformed payload: count=%d", got)
	}
}

// newAgentWithEnforcer constructs an agent whose SecurityEnforcer
// uses the supplied policy. Used by command-flow tests that exercise
// HMAC, allowlists, etc.
func newAgentWithEnforcer(t *testing.T, policy SecurityPolicy) (*Agent, *fakeNATSClient, fakeSubjects, *SecurityEnforcer) {
	t.Helper()
	enf, err := NewSecurityEnforcer(policy, testLogger())
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	cfg := Config{
		AgentID:           "agent-1",
		HeartbeatInterval: 10 * time.Second, // long; we want command tests to drive the handler explicitly
		MetadataInterval:  10 * time.Second,
		CommandTimeout:    2 * time.Second,
	}
	nats := newFakeNATS()
	subjects := fakeSubjects{cluster: "test"}
	metrics := &fakeMetricsCollector{}
	a, err := New(cfg, nats, subjects, metrics, testExecutor(t), enf, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a, nats, subjects, enf
}

func TestAgent_CommandFlow_SuccessRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sh; runs unix only in v1.0")
	}
	a, nats, subj, enf := newAgentWithEnforcer(t, SecurityPolicy{
		HMACSecret:    []byte("secret-1"),
		DefaultPolicy: PolicyAllow,
	})
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	req := CommandRequest{
		MessageID:      "cmd-success",
		Principal:      "admin",
		Command:        "/bin/sh",
		Args:           []string{"-c", "echo hello"},
		TimeoutSeconds: 2,
	}
	req.Signature = enf.ComputeHMAC(req)

	body, _ := json.Marshal(req)
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("cmd-success"))
	if err := nats.deliver(t, subj.AgentCommand("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	envs := nats.collectFor(t, subj.AgentResponse("agent-1"), 1, time.Second)
	var resp CommandResponse
	if err := json.Unmarshal(envs[0].Payload, &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if resp.Rejected {
		t.Errorf("Rejected = true, want false (RejectReason=%q)", resp.RejectReason)
	}
	if resp.ExitCode != 0 {
		t.Errorf("ExitCode = %d, want 0", resp.ExitCode)
	}
	if string(resp.Stdout) != "hello\n" {
		t.Errorf("Stdout = %q, want hello\\n", resp.Stdout)
	}
	if resp.MessageID != "cmd-success" {
		t.Errorf("MessageID = %q, want cmd-success", resp.MessageID)
	}
	if envs[0].CorrelationID != "cmd-success" {
		t.Errorf("Envelope CorrelationID = %q, want cmd-success", envs[0].CorrelationID)
	}
}

func TestAgent_CommandFlow_HMACInvalidPublishesRejection(t *testing.T) {
	a, nats, subj, _ := newAgentWithEnforcer(t, SecurityPolicy{
		HMACSecret:    []byte("secret-1"),
		DefaultPolicy: PolicyAllow,
	})
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	req := CommandRequest{
		MessageID: "cmd-bad-sig",
		Principal: "admin",
		Command:   "/bin/echo",
		Args:      []string{"hi"},
		Signature: "not-a-real-signature",
	}
	body, _ := json.Marshal(req)
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("cmd-bad-sig"))
	if err := nats.deliver(t, subj.AgentCommand("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	envs := nats.collectFor(t, subj.AgentResponse("agent-1"), 1, time.Second)
	var resp CommandResponse
	if err := json.Unmarshal(envs[0].Payload, &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !resp.Rejected {
		t.Error("Rejected = false, want true")
	}
	if resp.RejectReason == "" {
		t.Error("RejectReason empty")
	}
	if resp.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 on rejection", resp.ExitCode)
	}
}

func TestAgent_CommandFlow_AllowlistBlocksPublishesRejection(t *testing.T) {
	a, nats, subj, enf := newAgentWithEnforcer(t, SecurityPolicy{
		DefaultPolicy: PolicyDeny, // nothing allowed unless explicitly listed
	})
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	req := CommandRequest{
		MessageID: "cmd-blocked",
		Principal: "admin",
		Command:   "/bin/echo",
	}
	req.Signature = enf.ComputeHMAC(req)
	body, _ := json.Marshal(req)
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("cmd-blocked"))
	if err := nats.deliver(t, subj.AgentCommand("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	envs := nats.collectFor(t, subj.AgentResponse("agent-1"), 1, time.Second)
	var resp CommandResponse
	if err := json.Unmarshal(envs[0].Payload, &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !resp.Rejected {
		t.Errorf("Rejected = false, want true (DefaultPolicy=deny)")
	}
}

func TestAgent_CommandFlow_TimeoutSurfacesInResponse(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sleep; runs unix only in v1.0")
	}
	a, nats, subj, enf := newAgentWithEnforcer(t, SecurityPolicy{
		DefaultPolicy: PolicyAllow,
	})
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = a.Shutdown(context.Background()) })

	req := CommandRequest{
		MessageID:      "cmd-timeout",
		Command:        "/bin/sleep",
		Args:           []string{"5"},
		TimeoutSeconds: 0, // falls back to Agent.cfg.CommandTimeout (2s in test) — actually we want to override
	}
	// Force a tight per-command timeout via the request field.
	req.TimeoutSeconds = 1
	req.Signature = enf.ComputeHMAC(req)

	body, _ := json.Marshal(req)
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("cmd-timeout"))
	if err := nats.deliver(t, subj.AgentCommand("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	envs := nats.collectFor(t, subj.AgentResponse("agent-1"), 1, 3*time.Second)
	var resp CommandResponse
	if err := json.Unmarshal(envs[0].Payload, &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !resp.TimedOut {
		t.Errorf("TimedOut = false, want true")
	}
	if resp.ExitCode != -1 {
		t.Errorf("ExitCode = %d, want -1 on signal-kill", resp.ExitCode)
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

// TestAgent_ShutdownDrainsInFlightCommand: a command lands, its
// executor goroutine starts a /bin/sleep, Shutdown is called.
// The kill protocol (SIGTERM grace then SIGKILL) fires via
// commandCtx cancellation; Shutdown waits until the in-flight
// goroutine exits. Asserts: Shutdown returned within budget, no
// ctx-err returned, and a CommandResponse with TimedOut=true was
// published before Shutdown completed.
func TestAgent_ShutdownDrainsInFlightCommand(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sleep; unix only")
	}
	a, nats, subj, enf := newAgentWithEnforcer(t, SecurityPolicy{
		DefaultPolicy: PolicyAllow,
	})
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	req := CommandRequest{
		MessageID:      "cmd-shutdown-drain",
		Command:        "/bin/sleep",
		Args:           []string{"30"}, // would block well past test budget
		TimeoutSeconds: 60,             // big enough that the per-command timeout doesn't kick in
	}
	req.Signature = enf.ComputeHMAC(req)
	body, _ := json.Marshal(req)
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("cmd-shutdown-drain"))

	// Deliver in a goroutine — handleCommand runs synchronously
	// (blocks on executor.Execute waiting for /bin/sleep).
	delivered := make(chan struct{})
	go func() {
		_ = nats.deliver(t, subj.AgentCommand("agent-1"), env)
		close(delivered)
	}()

	// Give the executor a moment to start the child process so
	// the "in-flight at shutdown" condition is real, not a race.
	time.Sleep(75 * time.Millisecond)

	shutdownStart := time.Now()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := a.Shutdown(shutdownCtx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	shutdownDur := time.Since(shutdownStart)
	if shutdownDur > 2*time.Second {
		t.Errorf("Shutdown took %s; expected to drain quickly via SIGTERM grace", shutdownDur)
	}

	// deliver goroutine must have completed — handleCommand
	// should have published its response before returning.
	select {
	case <-delivered:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("deliver goroutine still running after Shutdown returned")
	}

	envs := nats.collectFor(t, subj.AgentResponse("agent-1"), 1, 200*time.Millisecond)
	var resp CommandResponse
	if err := json.Unmarshal(envs[0].Payload, &resp); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if !resp.TimedOut {
		t.Errorf("response.TimedOut = false; expected true (kill protocol fired)")
	}
}

// TestAgent_ShutdownRefusesNewCommandsAfterStop: post-shutdown
// commands delivered via the fake NATS short-circuit cleanly.
// We assert no executor invocation happened — the published-
// responses count stays at zero.
func TestAgent_ShutdownRefusesNewCommandsAfterStop(t *testing.T) {
	a, nats, subj, enf := newAgentWithEnforcer(t, SecurityPolicy{
		DefaultPolicy: PolicyAllow,
	})
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}

	req := CommandRequest{
		MessageID: "cmd-after-shutdown",
		Command:   "/bin/true",
	}
	req.Signature = enf.ComputeHMAC(req)
	body, _ := json.Marshal(req)
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("cmd-after-shutdown"))

	if err := nats.deliver(t, subj.AgentCommand("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	// Brief settling window; if the handler didn't short-circuit,
	// a response would be published.
	time.Sleep(50 * time.Millisecond)
	if c := nats.publishCount(subj.AgentResponse("agent-1")); c != 0 {
		t.Errorf("published responses after shutdown = %d; want 0", c)
	}
}

// TestAgent_ShutdownBudgetExceeded: the shutdown ctx expires
// before the in-flight command finishes (we use a hung sleep
// AND a short shutdown budget). Asserts: Shutdown returns the
// ctx error, the WARN log fires, and the goroutine continues
// until the per-command timeout cleans up.
func TestAgent_ShutdownBudgetExceeded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/sleep; unix only")
	}
	// Capture log output to verify the WARN fires.
	var logBuf safeLogBuffer
	logger := slog.New(slog.NewTextHandler(&logBuf, nil))

	enf, err := NewSecurityEnforcer(SecurityPolicy{DefaultPolicy: PolicyAllow}, logger)
	if err != nil {
		t.Fatalf("NewSecurityEnforcer: %v", err)
	}
	// Use a kill grace longer than the shutdown budget so the
	// budget expires first (proves the WARN path), but short
	// enough that the post-test cleanup is fast: SIGKILL fires
	// 300ms after cancel.
	exec := NewExecutor(ExecutorConfig{
		Logger:         logger,
		KillGrace:      300 * time.Millisecond,
		DefaultTimeout: 10 * time.Second,
	})
	a, err := New(Config{
		AgentID:           "agent-1",
		HeartbeatInterval: 10 * time.Second,
		MetadataInterval:  10 * time.Second,
		CommandTimeout:    10 * time.Second,
	}, newFakeNATS(), fakeSubjects{cluster: "test"}, &fakeMetricsCollector{}, exec, enf, logger)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// SIGTERM-trapping shell loop — ignores TERM so the kill
	// grace fully elapses. Without this, /bin/sleep exits on
	// SIGTERM and Shutdown drains within the 100ms budget.
	req := CommandRequest{
		MessageID:      "cmd-budget",
		Command:        "/bin/sh",
		Args:           []string{"-c", `trap "" TERM; while :; do sleep 0.1; done`},
		TimeoutSeconds: 30,
	}
	req.Signature = enf.ComputeHMAC(req)
	body, _ := json.Marshal(req)
	env := envelope.New(body, "kscore.test", envelope.WithMessageID("cmd-budget"))

	go func() {
		_ = a.handleCommand(context.Background(), "kscore.test.agent.agent-1.command", env)
	}()
	time.Sleep(75 * time.Millisecond)

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	err = a.Shutdown(shutdownCtx)
	if err == nil {
		t.Fatal("Shutdown returned nil; expected ctx-deadline error")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Shutdown err = %v; want DeadlineExceeded", err)
	}
	if !logBuf.contains("agent: command drain timed out") {
		t.Errorf("expected drain-timeout WARN in log; got:\n%s", logBuf.String())
	}

	// Wait for the rogue goroutine to clean up after SIGKILL
	// (kill grace = 300ms above) so the test runner doesn't
	// inherit the orphan child.
	time.Sleep(500 * time.Millisecond)
}

// TestAgent_NoGoroutineLeak_AfterShutdown: spin Start, send a
// short command through the full happy path, Shutdown, then
// verify no goroutines linger. Catches the Epic 06 fd-leak risk.
func TestAgent_NoGoroutineLeak_AfterShutdown(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses /bin/true; unix only")
	}
	defer goleak.VerifyNone(t,
		// slog's default handler may park goroutines briefly on
		// log emission; ignore. None of these are agent-spawned.
		goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"),
	)
	a, nats, subj, enf := newAgentWithEnforcer(t, SecurityPolicy{
		DefaultPolicy: PolicyAllow,
	})
	if err := a.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	req := CommandRequest{
		MessageID:      "cmd-leak-check",
		Command:        "/bin/true",
		TimeoutSeconds: 5,
	}
	req.Signature = enf.ComputeHMAC(req)
	body, _ := json.Marshal(req)
	env := envelope.New(body, subj.Prefix(), envelope.WithMessageID("cmd-leak-check"))

	if err := nats.deliver(t, subj.AgentCommand("agent-1"), env); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	// Wait for the response to confirm the command finished.
	_ = nats.collectFor(t, subj.AgentResponse("agent-1"), 1, 2*time.Second)

	if err := a.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	// Brief grace for goroutine teardown observability — Shutdown
	// already wg.Wait'd, but goleak inspects a snapshot.
	time.Sleep(20 * time.Millisecond)
}

// safeLogBuffer is a goroutine-safe wrapper around bytes.Buffer
// for tests that capture slog output. slog handlers can write
// from background goroutines; a plain bytes.Buffer races.
type safeLogBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (b *safeLogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	return len(p), nil
}

func (b *safeLogBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

func (b *safeLogBuffer) contains(s string) bool {
	return strings.Contains(b.String(), s)
}
