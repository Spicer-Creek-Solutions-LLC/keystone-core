package main

import (
	"context"
	"io"
	"log/slog"
	"net"
	"path/filepath"
	"sync"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"

	"go.keystone-core.io/keystone-core/internal/config"
)

func TestNewCommand(t *testing.T) {
	cmd := newCommand()
	if cmd.Use != "kscore-agent" {
		t.Errorf("Use = %q, want %q", cmd.Use, "kscore-agent")
	}
	if cmd.Short == "" {
		t.Error("Short is empty")
	}
}

// freeTCPPort picks an ephemeral port for the synthetic NATS server.
// Same race-window caveat as the kscore-server tests.
func freeTCPPort(t *testing.T) int {
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

// startSyntheticNATS spins a bare nats-server on the requested port
// for cmd/kscore-agent to connect to in external mode. Returned
// stop func tears it down on cleanup.
func startSyntheticNATS(t *testing.T, port int) func() {
	t.Helper()
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host:   "127.0.0.1",
		Port:   port,
		NoSigs: true,
		NoLog:  true,
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal("synthetic NATS not ready")
	}
	return func() {
		srv.Shutdown()
		srv.WaitForShutdown()
	}
}

// runCfg returns a hermetic test config that points at a synthetic
// NATS server bound to port. JetStream is disabled because the
// agent doesn't need it — Manager.ensureStreams is a server-side
// concern.
func runCfg(t *testing.T, natsPort int) *config.Config {
	t.Helper()
	url := "nats://127.0.0.1:" + intToStr(natsPort)
	return &config.Config{
		Mode: config.ModeDevelopment,
		Server: config.ServerConfig{
			Host:     "127.0.0.1",
			GRPCPort: 0,
			HTTPPort: 0,
			CORS: config.CORSConfig{
				Enabled:        true,
				AllowedOrigins: []string{"*"},
				AllowedMethods: []string{"GET"},
				AllowedHeaders: []string{"Authorization"},
			},
		},
		Logging: config.LoggingConfig{Level: "info", Format: "json"},
		Storage: config.StorageConfig{Driver: "sqlite", DSN: filepath.Join(t.TempDir(), "store.db")},
		Health:  config.HealthConfig{StartupGracePeriod: time.Millisecond, CheckTimeout: time.Second},
		NATS: config.NATSConfig{
			Mode:          config.NATSModeExternal,
			URLs:          []string{url},
			ClusterName:   "test",
			MaxReconnects: 1,
			ReconnectWait: 100 * time.Millisecond,
			JetStream:     config.JetStreamConfig{Enabled: false},
			Embedded: config.EmbeddedNATSConfig{
				Host: "127.0.0.1",
				Port: 4222, // unused in external mode but must pass Validate
			},
			Dedup:          config.DedupConfig{Enabled: false},
			CircuitBreaker: config.CircuitBreakerConfig{Enabled: false},
		},
		Agent: config.AgentConfig{
			AgentID:           "agent-test",
			HeartbeatInterval: 50 * time.Millisecond,
			MetadataInterval:  60 * time.Millisecond,
			CommandTimeout:    time.Second,
		},
	}
}

// TestRun_BootsAndShutsDownCleanly is the v1.0 daemon-boot smoke
// test. Spins a synthetic NATS, runs the agent against it for
// ~750ms (long enough for at least one heartbeat + metadata
// publish), cancels, asserts clean return.
func TestRun_BootsAndShutsDownCleanly(t *testing.T) {
	port := freeTCPPort(t)
	stopNATS := startSyntheticNATS(t, port)
	defer stopNATS()

	cfg := runCfg(t, port)
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, log) }()

	time.Sleep(750 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run: %v", err)
		}
	case <-time.After(35 * time.Second):
		t.Fatal("run did not return after cancel")
	}
}

func TestRun_RejectsEmbeddedNATS(t *testing.T) {
	cfg := runCfg(t, 4222)
	cfg.NATS.Mode = config.NATSModeEmbedded
	cfg.NATS.URLs = nil

	if err := run(context.Background(), cfg, slog.New(slog.NewJSONHandler(io.Discard, nil))); err == nil {
		t.Error("run with embedded NATS = nil, want error")
	}
}

func TestRun_RejectsEmptyAgentID(t *testing.T) {
	cfg := runCfg(t, 4222)
	cfg.Agent.AgentID = ""

	if err := run(context.Background(), cfg, slog.New(slog.NewJSONHandler(io.Discard, nil))); err == nil {
		t.Error("run with empty AgentID = nil, want error")
	}
}

// intToStr is a tiny int→string helper to keep the test imports
// minimal (no strconv).
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

// suppress unused-import warning under partial test selection.
var _ = sync.Mutex{}
