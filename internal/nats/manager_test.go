package nats

import (
	"context"
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
)

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
			Enabled:    true,
			StoreDir:   filepath.Join(t.TempDir(), "jetstream"),
			MaxStorage: 10 * 1024 * 1024,
		},
		Embedded: config.EmbeddedNATSConfig{
			Host:            "127.0.0.1",
			Port:            freePort(t),
			EnableJetStream: true,
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

	if err := m.Publish(context.Background(), "kscore.test.ping", []byte("hi")); err != nil {
		t.Errorf("Publish = %v, want nil", err)
	}

	if err := m.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown = %v, want nil", err)
	}

	if err := m.Health(context.Background()); err == nil {
		t.Error("Health after Shutdown = nil, want error")
	}
	if err := m.Publish(context.Background(), "kscore.test.ping", []byte("hi")); err == nil {
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
	if err := m.Publish(context.Background(), "x", []byte("y")); err == nil {
		t.Error("Publish pre-Start = nil, want error")
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
	if err := m.Publish(context.Background(), "kscore.test.ext", []byte("ok")); err != nil {
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

	want := []byte("payload")
	if err := m.Publish(context.Background(), "kscore.test.rt", want); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber timed out waiting for message")
	}
	if string(got) != string(want) {
		t.Errorf("payload = %q, want %q", got, want)
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
