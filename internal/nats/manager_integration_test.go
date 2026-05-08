//go:build integration

package nats

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	natsclient "github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// TestManager_EmbeddedFullRoundTrip is the integration smoke for Task 1:
// embedded boot, JetStream-enabled, an external subscriber on the same
// server, and a publish that the subscriber receives. Task 13 grows
// this into the JetStream consume path.
func TestManager_EmbeddedFullRoundTrip(t *testing.T) {
	m := startManager(t, embeddedConfig(t))

	sub, err := natsclient.Connect(m.ClientURL())
	if err != nil {
		t.Fatalf("subscriber connect: %v", err)
	}
	defer sub.Close()

	var wg sync.WaitGroup
	wg.Add(1)
	var got []byte
	subscription, err := sub.Subscribe("kscore.test.integration", func(msg *natsclient.Msg) {
		got = append([]byte(nil), msg.Data...)
		wg.Done()
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = subscription.Unsubscribe() }()
	if err := sub.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	wantInner := []byte(`"ping"`)
	env := envelope.New(wantInner, "kscore.test")
	if err := m.PublishEnvelope(context.Background(), "kscore.test.integration", env); err != nil {
		t.Fatalf("PublishEnvelope: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not receive message within 2s")
	}
	gotEnv, err := envelope.Unmarshal(got)
	if err != nil {
		t.Fatalf("subscriber decode: %v (raw=%s)", err, got)
	}
	if string(gotEnv.Payload) != string(wantInner) {
		t.Errorf("inner payload = %s, want %s", gotEnv.Payload, wantInner)
	}
}

// startStandaloneNATS spins a bare nats-server (no JetStream) on the
// requested port. Returns a function to stop it. Used by the
// recovery test to simulate "NATS goes down" while a Manager is
// connected externally.
func startStandaloneNATS(t *testing.T, port int) (*natsserver.Server, func()) {
	t.Helper()
	opts := &natsserver.Options{
		Host:   "127.0.0.1",
		Port:   port,
		NoSigs: true,
		NoLog:  true,
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal("standalone NATS not ready")
	}
	return srv, func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	}
}

// TestManager_HealthDownThenUpRecovery exercises the §4.2 acceptance
// bullet at the Manager level: external mode, NATS goes down →
// Health() error, NATS comes back on the same port → Health() nil
// after nats.go's reconnect loop kicks in.
//
// External-mode-only because shutting down the embedded server is
// one-way: we can't restart it inside the same Manager. Instead we
// spin a standalone nats-server, point an external Manager at it,
// kill the standalone server, restart it on the same port, and
// observe the Manager's Health flip back to OK.
func TestManager_HealthDownThenUpRecovery(t *testing.T) {
	port := freePort(t)

	// Phase 1: NATS up; Manager connects.
	_, stop1 := startStandaloneNATS(t, port)
	defer func() {
		// Defensive: if the second start failed, cleanup runs from here.
		// stop1 is a no-op if already called above.
	}()

	cfg := config.NATSConfig{
		Mode:           config.NATSModeExternal,
		URLs:           []string{"nats://127.0.0.1:" + intStr(port)},
		ClusterName:    "test",
		MaxReconnects:  -1, // infinite — we want the reconnect to take place
		ReconnectWait:  100 * time.Millisecond,
		JetStream:      config.JetStreamConfig{Enabled: false},
		Dedup:          config.DedupConfig{Enabled: false},
		CircuitBreaker: config.CircuitBreakerConfig{Enabled: false},
	}
	m, err := New(cfg, testLogger())
	if err != nil {
		stop1()
		t.Fatalf("New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		stop1()
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = m.Shutdown(stopCtx)
	})

	if err := m.Health(context.Background()); err != nil {
		stop1()
		t.Fatalf("phase 1: Health = %v, want nil", err)
	}

	// Phase 2: kill NATS. Health flips to error.
	stop1()

	// Wait for the disconnect callback to update conn state. nats.go
	// detects loss via PING/PONG; default ping interval is 2m, but
	// the underlying TCP close fires immediately.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if err := m.Health(context.Background()); err != nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err := m.Health(context.Background()); err == nil {
		t.Fatal("phase 2: Health = nil after NATS killed; want error")
	}

	// Phase 3: bring NATS back on the same port. Health recovers.
	_, stop2 := startStandaloneNATS(t, port)
	defer stop2()

	deadline = time.Now().Add(5 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		if err := m.Health(context.Background()); err == nil {
			lastErr = nil
			break
		} else {
			lastErr = err
		}
		time.Sleep(50 * time.Millisecond)
	}
	if lastErr != nil {
		t.Fatalf("phase 3 (recovery): Health did not return to nil within 5s; last err = %v", lastErr)
	}
}

// intStr is a small int→string helper to keep imports minimal in
// the integration test.
func intStr(n int) string {
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

// suppress unused-import warning when the file compiles with only one
// of the helpers active under different test selections.
var _ = filepath.Join
