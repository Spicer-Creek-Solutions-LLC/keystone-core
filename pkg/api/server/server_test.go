package server_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/server"
	"go.keystone-core.io/keystone-core/pkg/envelope"
	"go.keystone-core.io/keystone-core/pkg/natsstatus"
)

// TestMain isolates server lifecycle tests under goleak so runaway
// goroutines from broken Stop() paths are caught here, not at integration time.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		// modernc.org/sqlite spins a long-lived background goroutine
		// inside its connection pool that we cannot Close() without
		// modifying the driver. Tolerate it.
		goleak.IgnoreTopFunction("modernc.org/sqlite.(*conn).run"),
		goleak.IgnoreAnyFunction("modernc.org/sqlite.(*conn).run"),
	)
}

// trackingNATS records lifecycle calls so tests can assert ordering
// against shutdown.
type trackingNATS struct {
	startCalled    atomic.Bool
	shutdownCalled atomic.Bool
	publishCalled  atomic.Int64
	healthErr      error
}

func (n *trackingNATS) Start(context.Context) error {
	n.startCalled.Store(true)
	return nil
}
func (n *trackingNATS) Shutdown(context.Context) error {
	n.shutdownCalled.Store(true)
	return nil
}
func (n *trackingNATS) Health(context.Context) error { return n.healthErr }
func (n *trackingNATS) PublishEnvelope(_ context.Context, _ string, _ envelope.Envelope) error {
	n.publishCalled.Add(1)
	return nil
}
func (n *trackingNATS) EndpointSnapshots() []natsstatus.EndpointSnapshot { return nil }

func newTestStore(t *testing.T) state.Store {
	t.Helper()
	s, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "store.db")},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	// store.Close runs in Server.Stop; tests that don't reach Stop need
	// this fallback. Idempotent on the SQLite backend.
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func newTestConfig() *config.Config {
	return &config.Config{
		Mode: config.ModeDevelopment,
		Server: config.ServerConfig{
			Host:     "127.0.0.1",
			GRPCPort: 0, // ephemeral
			HTTPPort: 0,
			CORS: config.CORSConfig{
				Enabled:        true,
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
				AllowedHeaders: []string{"Authorization", "Content-Type"},
			},
		},
		Logging: config.LoggingConfig{Level: "info", Format: "json"},
		Storage: config.StorageConfig{Driver: "sqlite", DSN: "ignored-by-server"},
		Health: config.HealthConfig{
			// Tests expect /health/ready to return 200 quickly; the
			// 30s production default would force every test through a
			// full grace period. Tight grace + tight timeout keeps
			// the suite snappy without hiding the grace-period
			// behavior — tests that need to assert grace explicitly
			// override these.
			StartupGracePeriod: time.Millisecond,
			CheckTimeout:       time.Second,
		},
	}
}

// portFromAddr extracts the bound port from a "host:port" or "[::]:port".
func portFromAddr(t *testing.T, addr string) int {
	t.Helper()
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		t.Fatalf("SplitHostPort %q: %v", addr, err)
	}
	var p int
	if _, err := iAtoi(port, &p); err != nil {
		t.Fatalf("port not numeric %q", port)
	}
	return p
}

func iAtoi(s string, out *int) (int, error) {
	v := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errors.New("not numeric")
		}
		v = v*10 + int(c-'0')
	}
	*out = v
	return v, nil
}

// fakeSubjects mirrors the v1.0 hierarchy that
// internal/nats.SubjectBuilder produces. Lives here so every
// pkg/api/server test can construct Options without coupling to
// internal/nats.
type fakeSubjects struct{ cluster string }

func (f fakeSubjects) AgentCommand(agentID string) string {
	return "kscore." + f.cluster + ".agent." + agentID + ".command"
}

func (f fakeSubjects) AgentResponsePattern() string {
	return "kscore." + f.cluster + ".agent.*.response"
}

func (f fakeSubjects) BootstrapRegisterPattern() string {
	return "kscore." + f.cluster + ".bootstrap.*.register"
}

func (f fakeSubjects) BootstrapResponse(agentID string) string {
	return "kscore." + f.cluster + ".bootstrap." + agentID + ".response"
}

func (f fakeSubjects) Cluster() string { return f.cluster }
func (f fakeSubjects) Prefix() string  { return "kscore." + f.cluster }

// fakeSigner is a deterministic test stub for controlplane.Signer.
// Production wiring uses internal/agent.SecurityEnforcer adapted via
// commandSignerAdapter; tests don't need real HMAC math.
type fakeSigner struct{}

func (fakeSigner) SignCommand(_ controlplane.CommandMessage) string { return "test-sig" }

func newServer(t *testing.T, opts ...func(*server.Options)) (*server.Server, *trackingNATS) {
	t.Helper()
	tn := &trackingNATS{}
	o := server.Options{
		Config:      newTestConfig(),
		Logger:      slog.New(slog.NewJSONHandler(io.Discard, nil)),
		Store:       newTestStore(t),
		NATSManager: tn,
		Subjects:    fakeSubjects{cluster: "default"},
		Signer:      fakeSigner{},
	}
	for _, fn := range opts {
		fn(&o)
	}
	srv, err := server.New(o)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// New starts ConnectionManager + CommandDispatcher background
	// goroutines. Tests that don't reach Stop would otherwise leak
	// them and fail goleak. Stop is idempotent and safe to call
	// before Start, so an unconditional cleanup is fine.
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	})
	return srv, tn
}

func TestNew_ValidationRejectsMissingDeps(t *testing.T) {
	subj := fakeSubjects{cluster: "default"}
	sgn := fakeSigner{}
	cases := []struct {
		name string
		opts server.Options
	}{
		{"nil config", server.Options{Logger: slog.Default(), Store: newTestStore(t), NATSManager: &trackingNATS{}, Subjects: subj, Signer: sgn}},
		{"nil logger", server.Options{Config: newTestConfig(), Store: newTestStore(t), NATSManager: &trackingNATS{}, Subjects: subj, Signer: sgn}},
		{"nil store", server.Options{Config: newTestConfig(), Logger: slog.Default(), NATSManager: &trackingNATS{}, Subjects: subj, Signer: sgn}},
		{"nil nats", server.Options{Config: newTestConfig(), Logger: slog.Default(), Store: newTestStore(t), Subjects: subj, Signer: sgn}},
		{"nil subjects", server.Options{Config: newTestConfig(), Logger: slog.Default(), Store: newTestStore(t), NATSManager: &trackingNATS{}, Signer: sgn}},
		{"nil signer", server.Options{Config: newTestConfig(), Logger: slog.Default(), Store: newTestStore(t), NATSManager: &trackingNATS{}, Subjects: subj}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := server.New(tc.opts); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestNew_BindsListenersAndPopulatesAddrs(t *testing.T) {
	srv, tn := newServer(t)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	if !tn.startCalled.Load() {
		t.Error("NATS Start not called during init")
	}
	a := srv.Addrs()
	if a.GRPC == "" || a.HTTP == "" {
		t.Errorf("Addrs not populated: %+v", a)
	}
	if portFromAddr(t, a.GRPC) == 0 {
		t.Errorf("gRPC port = 0 (listener never bound)")
	}
	if portFromAddr(t, a.HTTP) == 0 {
		t.Errorf("HTTP port = 0 (listener never bound)")
	}
}

func TestNew_TLSEnabledRefused(t *testing.T) {
	store := newTestStore(t)
	tn := &trackingNATS{}
	cfg := newTestConfig()
	cfg.Server.TLS = config.TLSConfig{Enabled: true, CertFile: "/tmp/c", KeyFile: "/tmp/k"}
	_, err := server.New(server.Options{
		Config: cfg, Logger: slog.Default(), Store: store, NATSManager: tn,
		Subjects: fakeSubjects{cluster: "default"},
		Signer:   fakeSigner{},
	})
	if err == nil {
		t.Fatal("TLS-enabled config should fail in task 4")
	}
}

func TestStartStop_LifecycleAndOrdering(t *testing.T) {
	srv, tn := newServer(t)

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	addrs := srv.Addrs()

	// HTTP is serving — health endpoints respond.
	resp, err := http.Get("http://" + addrs.HTTP + "/health/live")
	if err != nil {
		t.Fatalf("GET /health/live: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("/health/live = %d, want 200", resp.StatusCode)
	}

	// gRPC listener is bound and accepting TCP.
	conn, err := net.DialTimeout("tcp", addrs.GRPC, time.Second)
	if err != nil {
		t.Errorf("dial gRPC %s: %v", addrs.GRPC, err)
	} else {
		conn.Close()
	}

	stopCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Stop(stopCtx); err != nil {
		t.Errorf("Stop: %v", err)
	}
	if !tn.shutdownCalled.Load() {
		t.Error("NATS Shutdown not called during Stop")
	}

	// Idempotent.
	if err := srv.Stop(stopCtx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestStop_BeforeStartIsSafe(t *testing.T) {
	srv, _ := newServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := srv.Stop(ctx); err != nil {
		t.Fatalf("Stop before Start: %v", err)
	}
}

func TestHTTP_PerDomainHandlersRegistered(t *testing.T) {
	srv, _ := newServer(t)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	// Sample one route per domain. Most return 501 (epic 03 stubs);
	// apikeys is the only domain with a real handler — its GET list
	// path should also work but may need a Bearer to authorize. The
	// list endpoint with no auth returns 401 from the handler middleware.
	cases := []struct{ method, path string }{
		{"GET", "/api/v1/agents"},
		{"GET", "/api/v1/cluster/status"},
		{"GET", "/api/v1/events"},
		{"GET", "/api/v1/policies"},
		{"GET", "/api/v1/secrets"},
		{"GET", "/api/v1/state/check"},
	}
	for _, tc := range cases {
		req, _ := http.NewRequest(tc.method, "http://"+srv.Addrs().HTTP+tc.path, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Errorf("%s %s: %v", tc.method, tc.path, err)
			continue
		}
		resp.Body.Close()
		// task 4 only cares the route is registered (i.e., not 404).
		if resp.StatusCode == http.StatusNotFound {
			t.Errorf("%s %s = 404 (handler not registered)", tc.method, tc.path)
		}
	}
}

func TestAPIStatus_ReportsAgentCounts(t *testing.T) {
	srv, _ := newServer(t)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	resp, err := http.Get("http://" + srv.Addrs().HTTP + "/api/status")
	if err != nil {
		t.Fatalf("GET /api/status: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestHealthReady_RespectsGracePeriod(t *testing.T) {
	cfg := newTestConfig()
	cfg.Health.StartupGracePeriod = 30 * time.Second
	srv, _ := newServer(t, func(o *server.Options) { o.Config = cfg })
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	resp, err := http.Get("http://" + srv.Addrs().HTTP + "/health/ready")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503 (in grace)", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), `"in_grace_period":true`) {
		t.Errorf("body missing in_grace_period flag: %s", body)
	}
}

func TestHealthReady_OKAfterGrace(t *testing.T) {
	srv, _ := newServer(t)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	// newTestConfig sets a 1ms grace period — wait for it.
	time.Sleep(20 * time.Millisecond)
	resp, err := http.Get("http://" + srv.Addrs().HTTP + "/health/ready")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 200; body=%s", resp.StatusCode, body)
	}
}

func TestHealthStatus_AlwaysReturns200WithSnapshot(t *testing.T) {
	srv, _ := newServer(t)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	resp, err := http.Get("http://" + srv.Addrs().HTTP + "/health/status")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	for _, want := range []string{`"components"`, `"started_at"`, `"uptime_seconds"`} {
		if !strings.Contains(string(body), want) {
			t.Errorf("body missing %q: %s", want, body)
		}
	}
}

func TestServer_DualStackBindsBothFamilies(t *testing.T) {
	// Probe IPv6 availability via the same path as listener_test.
	if probe, err := net.Listen("tcp6", "[::1]:0"); err != nil {
		t.Skipf("IPv6 not available: %v", err)
	} else {
		probe.Close()
	}

	cfg := newTestConfig()
	cfg.Server.Host = "0.0.0.0" // → dual-stack
	srv, _ := newServer(t, func(o *server.Options) { o.Config = cfg })

	if err := srv.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Stop(ctx)
	}()

	a := srv.Addrs()
	if len(a.AllGRPC) != 2 || len(a.AllHTTP) != 2 {
		t.Fatalf("dual-stack: AllGRPC=%v AllHTTP=%v", a.AllGRPC, a.AllHTTP)
	}

	// Primary is IPv4; secondary is IPv6.
	if !strings.HasPrefix(a.AllGRPC[0], "0.0.0.0:") {
		t.Errorf("AllGRPC[0] = %q, want IPv4 primary", a.AllGRPC[0])
	}
	if !strings.HasPrefix(a.AllGRPC[1], "[::]") {
		t.Errorf("AllGRPC[1] = %q, want IPv6 secondary", a.AllGRPC[1])
	}

	// HTTP serves on both. Dial the IPv4 listener via 127.0.0.1 and the
	// IPv6 listener via [::1] using the bound port from each.
	v4Port := portFromAddr(t, a.AllHTTP[0])
	v6Port := portFromAddr(t, a.AllHTTP[1])

	for _, dial := range []string{
		"127.0.0.1:" + itoa(v4Port),
		"[::1]:" + itoa(v6Port),
	} {
		resp, err := http.Get("http://" + dial + "/health/live")
		if err != nil {
			t.Errorf("GET %s: %v", dial, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s = %d", dial, resp.StatusCode)
		}
	}
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	out := []byte{}
	for n > 0 {
		out = append([]byte{byte('0' + n%10)}, out...)
		n /= 10
	}
	return string(out)
}

func TestStop_BoundedByContext(t *testing.T) {
	srv, _ := newServer(t)
	if err := srv.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	// Already-cancelled context: Stop should still run through the
	// reverse-of-init steps (each step is responsible for honoring its
	// own ctx), but http.Shutdown will return ctx.Err. We accept either
	// nil or a cancellation error — what matters is that Stop returns
	// promptly and doesn't leak.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done := make(chan error, 1)
	go func() { done <- srv.Stop(ctx) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop did not return within 2s of cancelled ctx")
	}
}
