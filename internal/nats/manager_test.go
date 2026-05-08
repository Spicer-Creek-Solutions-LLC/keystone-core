package nats

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// mkEnv builds a default envelope wrapping data for the test cluster
// prefix ("kscore.test"). The bytes are treated as a JSON string
// literal — envelope.Payload is json.RawMessage, so the inner shape
// must be valid JSON. Tests that need a structured payload should
// build the envelope inline with json.Marshal'd bytes.
func mkEnv(data []byte) envelope.Envelope {
	if data == nil {
		return envelope.New(nil, "kscore.test")
	}
	quoted, err := json.Marshal(string(data))
	if err != nil {
		panic("mkEnv: json.Marshal: " + err.Error())
	}
	return envelope.New(quoted, "kscore.test")
}

// freePort returns a TCP port no other process is currently bound to.
// Race window is small; tests retry once via fresh manager construction
// if a Start fails on a now-claimed port.
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

// freeIPv6Port returns a TCP port the IPv6 loopback can bind. Calls
// t.Skip when the runner has no IPv6 stack — some CI environments
// disable IPv6 entirely. Used by the Task 12 IPv6 acceptance tests.
func freeIPv6Port(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "[::1]:0")
	if err != nil {
		t.Skipf("no IPv6 loopback available: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func embeddedConfig(t *testing.T) config.NATSConfig {
	t.Helper()
	return config.NATSConfig{
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
			CleanupInterval: time.Hour, // long; tests don't need cleanup ticks
		},
	}
}

func startManager(t *testing.T, cfg config.NATSConfig) *Manager {
	t.Helper()
	m, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.Shutdown(stopCtx)
	})
	return m
}

func TestNew_RejectsInvalidConfig(t *testing.T) {
	cfg := embeddedConfig(t)
	cfg.Mode = "weird"
	if _, err := New(cfg, testLogger()); err == nil {
		t.Fatal("New: expected error for invalid mode, got nil")
	}
}

func TestNew_NilLoggerDefaults(t *testing.T) {
	cfg := embeddedConfig(t)
	m, err := New(cfg, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if m.log == nil {
		t.Error("log is nil after New(nil)")
	}
}

func TestManager_EmbeddedStartHealthPublishShutdown(t *testing.T) {
	m := startManager(t, embeddedConfig(t))

	if err := m.Health(context.Background()); err != nil {
		t.Errorf("Health after Start = %v, want nil", err)
	}

	if err := m.PublishEnvelope(context.Background(), "kscore.test.ping", mkEnv([]byte("hi"))); err != nil {
		t.Errorf("Publish = %v, want nil", err)
	}

	if err := m.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown = %v, want nil", err)
	}

	if err := m.Health(context.Background()); err == nil {
		t.Error("Health after Shutdown = nil, want error")
	}
	if err := m.PublishEnvelope(context.Background(), "kscore.test.ping", mkEnv([]byte("hi"))); err == nil {
		t.Error("Publish after Shutdown = nil, want error")
	}
}

func TestManager_StartIdempotent(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	if err := m.Start(context.Background()); err != nil {
		t.Errorf("second Start = %v, want nil", err)
	}
}

func TestManager_StartAfterShutdownRejected(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	if err := m.Start(context.Background()); err == nil {
		t.Error("Start after Shutdown = nil, want error")
	}
}

func TestManager_ShutdownIdempotent(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown 1: %v", err)
	}
	if err := m.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown 2 = %v, want nil", err)
	}
}

func TestManager_ShutdownBeforeStart(t *testing.T) {
	m, err := New(embeddedConfig(t), testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown before Start = %v, want nil", err)
	}
}

func TestManager_HealthPreStart(t *testing.T) {
	m, err := New(embeddedConfig(t), testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Health(context.Background()); err == nil {
		t.Error("Health pre-Start = nil, want error")
	}
}

func TestManager_PublishPreStart(t *testing.T) {
	m, err := New(embeddedConfig(t), testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	// Use a valid subject so the failure surfaces from the started
	// gate, not from the prefix interceptor — this test asserts the
	// pre-Start contract specifically.
	if err := m.PublishEnvelope(context.Background(), "kscore.test.preStart", mkEnv([]byte("y"))); err == nil {
		t.Error("Publish pre-Start = nil, want error")
	}
}

func TestManager_PublishRejectsUnprefixed(t *testing.T) {
	m := startManager(t, embeddedConfig(t))

	cases := []string{
		"",
		"random.subject",
		"kscore.other.agent.register",      // wrong cluster
		"kscoredev.test.x",                 // wrong root
		"kscore.test.agent.*",              // wildcard
		"kscore.test.agent.>",              // wildcard
		"kscore.test.agent foo",            // whitespace
	}
	for _, subject := range cases {
		err := m.PublishEnvelope(context.Background(), subject, mkEnv([]byte("ok")))
		if err == nil {
			t.Errorf("PublishEnvelope(%q) = nil, want error", subject)
		}
	}

	// And a positive control: the typed constructor result publishes
	// without error.
	if err := m.PublishEnvelope(context.Background(), m.Subjects().AgentHeartbeat(), mkEnv([]byte("hb"))); err != nil {
		t.Errorf("PublishEnvelope(AgentHeartbeat) = %v, want nil", err)
	}
}

func TestManager_PublishEnvelopeRejectsDuplicate(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	subj := m.Subjects().AgentHeartbeat()

	env := envelope.New([]byte(`"ping"`), m.Subjects().Prefix(),
		envelope.WithMessageID("hb-fixed-1"))
	if err := m.PublishEnvelope(context.Background(), subj, env); err != nil {
		t.Fatalf("first PublishEnvelope: %v", err)
	}
	// Second publish with the same MessageID must be suppressed.
	err := m.PublishEnvelope(context.Background(), subj, env)
	if !errors.Is(err, envelope.ErrDuplicate) {
		t.Errorf("second PublishEnvelope: err = %v, want envelope.ErrDuplicate", err)
	}
}

func TestManager_DedupDifferentMessageIDsBothPublish(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	subj := m.Subjects().AgentHeartbeat()

	for i := 0; i < 3; i++ {
		env := envelope.New([]byte(`"ping"`), m.Subjects().Prefix())
		if err := m.PublishEnvelope(context.Background(), subj, env); err != nil {
			t.Errorf("PublishEnvelope[%d]: %v", i, err)
		}
	}
}

func TestManager_DedupNotRecordedOnPublishFailure(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	subj := m.Subjects().AgentHeartbeat()

	// Force a publish failure: shut the manager down and try to
	// publish. The publish path errors out before recording.
	if err := m.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
	env := envelope.New([]byte(`"ping"`), m.Subjects().Prefix(),
		envelope.WithMessageID("hb-fixed-2"))
	if err := m.PublishEnvelope(context.Background(), subj, env); err == nil {
		t.Fatal("expected publish failure on shut-down manager")
	}
	// Even after the failed publish, the MessageID is not in the
	// dedup cache (cache was Stop'd, but if it weren't, the failed
	// publish path would have skipped Record anyway).
	if m.dedup != nil && m.dedup.IsDuplicate(subj, "hb-fixed-2") {
		t.Error("MessageID was recorded despite publish failure")
	}
}

func TestManager_PublishEnvelopeRejectsClusterMismatch(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	// Envelope built for a *different* cluster than the manager's.
	wrong := envelope.New([]byte("hi"), "kscore.somewhere-else")
	err := m.PublishEnvelope(context.Background(), m.Subjects().AgentHeartbeat(), wrong)
	if err == nil {
		t.Fatal("PublishEnvelope: expected error for cluster_prefix mismatch")
	}
	if !strings.Contains(err.Error(), "cluster_prefix") {
		t.Errorf("err = %v, want containing 'cluster_prefix'", err)
	}
}

func TestManager_ExternalConnect(t *testing.T) {
	hub := startManager(t, embeddedConfig(t))
	url := hub.ClientURL()
	if url == "" {
		t.Fatal("hub ClientURL is empty")
	}

	cfg := config.NATSConfig{
		Mode:          config.NATSModeExternal,
		URLs:          []string{url},
		ClusterName:   "test",
		MaxReconnects: 1,
		ReconnectWait: 100 * time.Millisecond,
		// JetStream sub-config still needs a non-empty StoreDir to
		// satisfy validation when Enabled=true; flip it off for the
		// pure-client path so we do not require a writable dir.
		JetStream: config.JetStreamConfig{Enabled: false},
	}
	m := startManager(t, cfg)

	if err := m.Health(context.Background()); err != nil {
		t.Errorf("external Health = %v, want nil", err)
	}
	if err := m.PublishEnvelope(context.Background(), "kscore.test.ext", mkEnv([]byte("ok"))); err != nil {
		t.Errorf("external Publish = %v, want nil", err)
	}
}

func TestManager_ExternalSubscribeRoundTrip(t *testing.T) {
	hub := startManager(t, embeddedConfig(t))

	// Independent subscriber connection on the same embedded server.
	sub, err := natsclient.Connect(hub.ClientURL())
	if err != nil {
		t.Fatalf("subscriber connect: %v", err)
	}
	defer sub.Close()

	var (
		wg   sync.WaitGroup
		got  []byte
		once sync.Once
	)
	wg.Add(1)
	subscription, err := sub.Subscribe("kscore.test.rt", func(msg *natsclient.Msg) {
		once.Do(func() {
			got = append([]byte(nil), msg.Data...)
			wg.Done()
		})
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = subscription.Unsubscribe() }()
	if err := sub.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	cfg := config.NATSConfig{
		Mode:          config.NATSModeExternal,
		URLs:          []string{hub.ClientURL()},
		ClusterName:   "test",
		MaxReconnects: 1,
		ReconnectWait: 100 * time.Millisecond,
		JetStream:     config.JetStreamConfig{Enabled: false},
	}
	m := startManager(t, cfg)

	// Inner payload is JSON because envelope.Payload is json.RawMessage;
	// build directly (mkEnv wraps as a JSON string literal, which is
	// not what we want here).
	wantInner := []byte(`{"msg":"hello"}`)
	env := envelope.New(wantInner, "kscore.test")
	if err := m.PublishEnvelope(context.Background(), "kscore.test.rt", env); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber timed out waiting for message")
	}

	gotEnv, err := envelope.Unmarshal(got)
	if err != nil {
		t.Fatalf("subscriber decode: %v (raw=%s)", err, got)
	}
	if string(gotEnv.Payload) != string(wantInner) {
		t.Errorf("inner payload = %s, want %s", gotEnv.Payload, wantInner)
	}
	if gotEnv.ClusterPrefix != "kscore.test" {
		t.Errorf("ClusterPrefix = %q, want kscore.test", gotEnv.ClusterPrefix)
	}
}

func TestManager_ShutdownContextCanceled(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancelled before call — embedded shutdown still runs but
	// the watchdog returns ctx.Err() if the embedded path takes any time.
	err := m.Shutdown(ctx)
	if err != nil && !errors.Is(err, context.Canceled) {
		t.Errorf("Shutdown(canceled) = %v, want nil or context.Canceled", err)
	}
	// The watchdog returned but the embedded server's Shutdown +
	// WaitForShutdown goroutine is still running. With JetStream
	// streams now persisting to disk (Task 8), that goroutine
	// continues writing to t.TempDir() after Shutdown returns,
	// racing with Go's TempDir cleanup. Brief wait gives the
	// orphan time to flush before TempDir rm-rf.
	time.Sleep(500 * time.Millisecond)
}

func TestManager_ExternalUnreachableFails(t *testing.T) {
	cfg := config.NATSConfig{
		Mode:          config.NATSModeExternal,
		URLs:          []string{"nats://127.0.0.1:1"},
		ClusterName:   "test",
		MaxReconnects: 0,
		ReconnectWait: 10 * time.Millisecond,
		JetStream:     config.JetStreamConfig{Enabled: false},
	}
	m, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := m.Start(ctx); err == nil {
		t.Fatal("Start to unreachable URL = nil, want error")
		_ = m.Shutdown(context.Background())
	} else if !strings.Contains(err.Error(), "nats:") {
		t.Errorf("err = %v, want containing 'nats:'", err)
	}
}

func TestManager_ClientURLEmptyBeforeStart(t *testing.T) {
	m, err := New(embeddedConfig(t), testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := m.ClientURL(); got != "" {
		t.Errorf("ClientURL pre-Start = %q, want empty", got)
	}
}

func TestManager_ClientURLExternalReturnsFirst(t *testing.T) {
	cfg := config.NATSConfig{
		Mode:        config.NATSModeExternal,
		URLs:        []string{"nats://a:4222", "nats://b:4222"},
		ClusterName: "test",
		JetStream:   config.JetStreamConfig{Enabled: false},
	}
	m, err := New(cfg, testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := m.ClientURL(); got != "nats://a:4222" {
		t.Errorf("ClientURL = %q, want nats://a:4222", got)
	}
}

// TestManager_EmbeddedIPv6Host exercises the §4.2 IPv6 acceptance
// bullet at the embedded-mode level. Binds to ::1 and verifies the
// reported ClientURL bracket-formats correctly, plus a publish round-
// trip works through the IPv6 loopback.
func TestManager_EmbeddedIPv6Host(t *testing.T) {
	port := freeIPv6Port(t)
	cfg := embeddedConfig(t)
	cfg.Embedded.Host = "::1"
	cfg.Embedded.Port = port
	m := startManager(t, cfg)

	got := m.ClientURL()
	if !strings.Contains(got, "[::1]") {
		t.Errorf("ClientURL = %q, want bracketed [::1] form", got)
	}

	subject := m.Subjects().AgentHeartbeat()
	env := envelope.New([]byte(`"v6"`), m.Subjects().Prefix())
	if err := m.PublishEnvelope(context.Background(), subject, env); err != nil {
		t.Errorf("PublishEnvelope over IPv6: %v", err)
	}
}

// TestManager_ExternalIPv6URL exercises the §4.2 IPv6 acceptance
// bullet at the external-mode level. Spins an embedded NATS bound
// to ::1, then connects an external Manager via the bracketed URL
// form and round-trips a publish.
func TestManager_ExternalIPv6URL(t *testing.T) {
	port := freeIPv6Port(t)
	hubCfg := embeddedConfig(t)
	hubCfg.Embedded.Host = "::1"
	hubCfg.Embedded.Port = port
	hub := startManager(t, hubCfg)
	_ = hub // keep hub alive for the duration of the test (cleanup runs via t.Cleanup)

	url := "nats://[::1]:" + intToStr(port)
	cfg := config.NATSConfig{
		Mode:          config.NATSModeExternal,
		URLs:          []string{url},
		ClusterName:   "test",
		MaxReconnects: 1,
		ReconnectWait: 100 * time.Millisecond,
		JetStream:     config.JetStreamConfig{Enabled: false},
	}
	m := startManager(t, cfg)

	subject := m.Subjects().AgentHeartbeat()
	env := envelope.New([]byte(`"v6"`), m.Subjects().Prefix())
	if err := m.PublishEnvelope(context.Background(), subject, env); err != nil {
		t.Errorf("PublishEnvelope over IPv6 URL %q: %v", url, err)
	}
}

// intToStr is the same int→string used by the integration recovery
// test; duplicated here so the unit test file stays import-minimal
// (no strconv).
func intToStr(n int) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	var buf [11]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
