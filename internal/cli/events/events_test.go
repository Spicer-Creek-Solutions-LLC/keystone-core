package events_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	cli "go.keystone-core.io/keystone-core/internal/cli/events"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	internalevents "go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// cliRig boots the real EventsGRPCServer over bufconn and exposes
// a Deps the CLI's NewCommand can use. The store is real
// (SQL-backed in-memory SQLite); publisher + subscriber are stubs.
type cliRig struct {
	listen *bufconn.Listener
	grpc   *grpc.Server
	store  internalevents.EventStore
	pub    *stubPub
	sub    *stubSub
}

type stubPub struct {
	mu        sync.Mutex
	published []internalevents.Event
}

func (s *stubPub) Start(context.Context) error { return nil }
func (s *stubPub) Publish(_ context.Context, e internalevents.Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.published = append(s.published, e)
	return nil
}
func (s *stubPub) PublishAsync(ctx context.Context, e internalevents.Event) error {
	return s.Publish(ctx, e)
}
func (s *stubPub) Stop(context.Context) error { return nil }

func (s *stubPub) snapshot() []internalevents.Event {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]internalevents.Event, len(s.published))
	copy(out, s.published)
	return out
}

type stubSub struct {
	mu      sync.Mutex
	handler internalevents.EventHandler
}

func (s *stubSub) Start(context.Context) error { return nil }
func (s *stubSub) Subscribe(_ context.Context, _ string, h internalevents.EventHandler, _ ...internalevents.SubscribeOption) (internalevents.Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = h
	return stubSubscription{}, nil
}
func (s *stubSub) Stop(context.Context) error { return nil }

func (s *stubSub) currentHandler() internalevents.EventHandler {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.handler
}

type stubSubscription struct{}

func (stubSubscription) Unsubscribe() error       { return nil }
func (stubSubscription) Pending() (uint64, error) { return 0, nil }

func newCLIRig(t *testing.T) *cliRig {
	t.Helper()

	stateStore, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "events.db")},
	})
	if err != nil {
		t.Fatalf("state.NewStore: %v", err)
	}
	t.Cleanup(func() { _ = stateStore.Close() })

	store := internalevents.NewSQLEventStore(stateStore)
	pub := &stubPub{}
	sub := &stubSub{}

	server := controlplane.NewEventsGRPCServer(store, pub, sub)

	listener := bufconn.Listen(1 << 20)
	grpcSrv := grpc.NewServer()
	v1.RegisterEventServiceServer(grpcSrv, server)
	go func() {
		if err := grpcSrv.Serve(listener); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("grpc serve: %v", err)
		}
	}()
	t.Cleanup(func() {
		grpcSrv.Stop()
		_ = listener.Close()
	})

	return &cliRig{listen: listener, grpc: grpcSrv, store: store, pub: pub, sub: sub}
}

// deps returns a Deps wired to talk to the rig's bufconn.
func (r *cliRig) deps() cli.Deps {
	return cli.Deps{
		Dial: func(_ context.Context, _, _ string) (v1.EventServiceClient, io.Closer, error) {
			conn, err := grpc.NewClient(
				"passthrough://bufnet",
				grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
					return r.listen.DialContext(ctx)
				}),
				grpc.WithTransportCredentials(insecure.NewCredentials()),
			)
			if err != nil {
				return nil, nil, err
			}
			return v1.NewEventServiceClient(conn), conn, nil
		},
	}
}

func (r *cliRig) runCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewCommand(r.deps())
	cmd.SetArgs(args)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(context.Background())
	err := cmd.Execute()
	return buf.String(), err
}

func (r *cliRig) runCmdCtx(t *testing.T, ctx context.Context, args ...string) (string, error) {
	t.Helper()
	cmd := cli.NewCommand(r.deps())
	cmd.SetArgs(args)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	cmd.SetContext(ctx)
	err := cmd.Execute()
	return buf.String(), err
}

// ---- emit -----------------------------------------------------------------

func TestCLI_Emit_RoundTrip(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	out, err := r.runCmd(t, "emit",
		"--type", "agent.connect",
		"--source", "agent-1",
		"--severity", "warn",
		"--tag", "role=web",
		"--data", `{"latency_ms": 12.5}`,
	)
	if err != nil {
		t.Fatalf("emit: %v\n%s", err, out)
	}
	if !strings.Contains(out, "emitted ") {
		t.Errorf("missing 'emitted' line: %s", out)
	}
	pubs := r.pub.snapshot()
	if len(pubs) != 1 {
		t.Fatalf("publisher saw %d events", len(pubs))
	}
	if pubs[0].Severity != internalevents.SeverityWarn {
		t.Errorf("severity = %s", pubs[0].Severity)
	}
	if pubs[0].Tags["role"] != "web" {
		t.Errorf("tags lost: %+v", pubs[0].Tags)
	}
	if pubs[0].Data["latency_ms"] != 12.5 {
		t.Errorf("data lost: %+v", pubs[0].Data)
	}
}

func TestCLI_Emit_BadType(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "emit", "--type", "bogus", "--source", "x")
	if err == nil {
		t.Errorf("emit bad-type returned nil err")
	}
}

func TestCLI_Emit_MissingRequiredFlags(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "emit", "--source", "x")
	if err == nil {
		t.Errorf("emit without --type returned nil err")
	}
}

// ---- list ----------------------------------------------------------------

func TestCLI_List_Empty(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	out, err := r.runCmd(t, "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	if !strings.Contains(out, "no events") {
		t.Errorf("expected 'no events': %s", out)
	}
}

func TestCLI_List_PopulatesTable(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	for _, typ := range []internalevents.EventType{
		internalevents.EventTypeAgentConnect,
		internalevents.EventTypeJobStart,
	} {
		_ = r.store.Store(context.Background(), internalevents.MustNewEvent(typ, "src"))
	}
	out, err := r.runCmd(t, "list")
	if err != nil {
		t.Fatalf("list: %v\n%s", err, out)
	}
	for _, want := range []string{"TIME", "SEVERITY", "agent.connect", "job.start"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
}

func TestCLI_List_TypeCategoryMutuallyExclusive(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "list", "--type", "agent.connect", "--category", "agent")
	if err == nil {
		t.Errorf("expected --type+--category to fail")
	}
}

func TestCLI_List_FilterByCategory(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	for _, typ := range []internalevents.EventType{
		internalevents.EventTypeAgentConnect,
		internalevents.EventTypeAgentHeartbeat,
		internalevents.EventTypeJobStart,
	} {
		_ = r.store.Store(context.Background(), internalevents.MustNewEvent(typ, "src"))
	}
	out, err := r.runCmd(t, "list", "--category", "agent")
	if err != nil {
		t.Fatalf("%v\n%s", err, out)
	}
	if strings.Contains(out, "job.start") {
		t.Errorf("category filter leaked job.start: %s", out)
	}
	if !strings.Contains(out, "agent.connect") {
		t.Errorf("expected agent.connect: %s", out)
	}
}

func TestCLI_List_JSONOutput(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_ = r.store.Store(context.Background(), internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "src"))

	out, err := r.runCmd(t, "-o", "json", "list")
	if err != nil {
		t.Fatalf("list json: %v\n%s", err, out)
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(out), &parsed); err != nil {
		t.Fatalf("JSON not valid: %v\n%s", err, out)
	}
	if _, ok := parsed["events"]; !ok {
		t.Errorf("missing events key: %#v", parsed)
	}
}

func TestCLI_List_InvalidOutput(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "-o", "yaml", "list")
	if err == nil {
		t.Errorf("expected --output yaml to be rejected")
	}
}

// ---- get -----------------------------------------------------------------

func TestCLI_Get_RoundTrip(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	e := internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "agent-1")
	_ = r.store.Store(context.Background(), e)

	out, err := r.runCmd(t, "get", e.ID)
	if err != nil {
		t.Fatalf("get: %v\n%s", err, out)
	}
	for _, want := range []string{"id", e.ID, "agent.connect", "agent-1"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in: %s", want, out)
		}
	}
}

func TestCLI_Get_NotFound(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "get", "ghost-id")
	if err == nil {
		t.Errorf("get ghost-id returned nil err")
	}
}

// ---- types ---------------------------------------------------------------

func TestCLI_Types_ListsCanonical(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	out, err := r.runCmd(t, "types")
	if err != nil {
		t.Fatalf("types: %v\n%s", err, out)
	}
	for _, want := range []string{"agent.connect", "job.start", "state.drift", "policy.violation"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in types output: %s", want, out)
		}
	}
}

// ---- stats ---------------------------------------------------------------

func TestCLI_Stats(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = r.store.Store(ctx, internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "src"))
	}
	for i := 0; i < 2; i++ {
		e := internalevents.MustNewEvent(internalevents.EventTypeJobStart, "src")
		e.Severity = internalevents.SeverityWarn
		_ = r.store.Store(ctx, e)
	}
	out, err := r.runCmd(t, "stats")
	if err != nil {
		t.Fatalf("stats: %v\n%s", err, out)
	}
	for _, want := range []string{"total: 5", "BY TYPE", "BY SEVERITY", "agent.connect", "warn"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in stats: %s", want, out)
		}
	}
}

// ---- watch (streaming) ---------------------------------------------------

func TestCLI_Watch_StreamsEvent(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	ctx, cancel := context.WithCancel(context.Background())

	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := r.runCmdCtx(t, ctx, "watch")
		done <- result{out: out, err: err}
	}()

	// Wait for the stub Subscribe to install the handler then push
	// one event through it.
	deadline := time.After(2 * time.Second)
	var h internalevents.EventHandler
	for h == nil {
		select {
		case <-deadline:
			cancel()
			t.Fatalf("handler not installed within deadline")
		case <-time.After(10 * time.Millisecond):
			h = r.sub.currentHandler()
		}
	}
	in := internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "agent-1")
	if err := h(ctx, in); err != nil {
		t.Errorf("handler err: %v", err)
	}
	// Brief settle then cancel.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case res := <-done:
		if res.err != nil {
			t.Errorf("watch err: %v\n%s", res.err, res.out)
		}
		if !strings.Contains(res.out, "agent.connect") {
			t.Errorf("watch output missing event type: %s", res.out)
		}
		if !strings.Contains(res.out, "agent-1") {
			t.Errorf("watch output missing source: %s", res.out)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("watch did not return after cancel")
	}
}

// ---- subscribe (streaming) -----------------------------------------------

func TestCLI_Subscribe_BadFilter(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "subscribe", "--filter", "((malformed")
	if err == nil {
		t.Errorf("subscribe with malformed filter returned nil err")
	}
}

func TestCLI_Subscribe_StreamsJSONLine(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	ctx, cancel := context.WithCancel(context.Background())

	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := r.runCmdCtx(t, ctx, "subscribe")
		done <- result{out: out, err: err}
	}()

	deadline := time.After(2 * time.Second)
	var h internalevents.EventHandler
	for h == nil {
		select {
		case <-deadline:
			cancel()
			t.Fatalf("handler not installed")
		case <-time.After(10 * time.Millisecond):
			h = r.sub.currentHandler()
		}
	}
	in := internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "agent-1")
	_ = h(ctx, in)
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case res := <-done:
		if res.err != nil {
			t.Errorf("subscribe err: %v", res.err)
		}
		// JSON lines: should start with `{` and parse as JSON.
		line := strings.TrimSpace(strings.SplitN(res.out, "\n", 2)[0])
		if !strings.HasPrefix(line, "{") {
			t.Errorf("expected JSON line, got %q", line)
		}
		var event map[string]any
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Errorf("first line not valid JSON: %v (line=%q)", err, line)
		}
		if event["type"] != "agent.connect" {
			t.Errorf("type = %v", event["type"])
		}
	case <-time.After(3 * time.Second):
		t.Fatal("subscribe did not return after cancel")
	}
}

// ---- replay --------------------------------------------------------------

func TestCLI_Replay_RequiresSince(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	_, err := r.runCmd(t, "replay")
	if err == nil {
		t.Errorf("replay without --since returned nil err")
	}
}

func TestCLI_Replay_StreamsHistorical(t *testing.T) {
	t.Parallel()
	r := newCLIRig(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		_ = r.store.Store(ctx, internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "agent-1"))
	}
	out, err := r.runCmd(t, "replay", "--since", "1h")
	if err != nil {
		t.Fatalf("replay: %v\n%s", err, out)
	}
	// JSON lines default — three events → three JSON-prefixed lines.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 {
		t.Errorf("got %d lines, want 3:\n%s", len(lines), out)
	}
	for i, line := range lines {
		if !strings.HasPrefix(strings.TrimSpace(line), "{") {
			t.Errorf("line %d not JSON: %q", i, line)
		}
	}
}

// ---- format helpers ------------------------------------------------------

func TestSeverityNameFromEnum_Mapping(t *testing.T) {
	t.Parallel()
	// Quick sanity that the proto → string conversion in the
	// formatter doesn't drift. Tests via the watch path so we
	// exercise the actual emitter.
	r := newCLIRig(t)
	ctx, cancel := context.WithCancel(context.Background())
	type result struct {
		out string
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := r.runCmdCtx(t, ctx, "watch")
		done <- result{out: out, err: err}
	}()

	deadline := time.After(2 * time.Second)
	var h internalevents.EventHandler
	for h == nil {
		select {
		case <-deadline:
			cancel()
			t.Fatalf("handler not installed")
		case <-time.After(10 * time.Millisecond):
			h = r.sub.currentHandler()
		}
	}
	in := internalevents.MustNewEvent(internalevents.EventTypeAgentConnect, "x")
	in.Severity = internalevents.SeverityCritical
	_ = h(ctx, in)
	time.Sleep(100 * time.Millisecond)
	cancel()

	res := <-done
	if !strings.Contains(res.out, "critical") {
		t.Errorf("severity 'critical' missing in watch output: %s", res.out)
	}
}
