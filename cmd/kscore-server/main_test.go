package main

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
)

// freeTCPPort returns a TCP port no other process currently holds.
// Used so the embedded NATS server in test runs binds an ephemeral
// port instead of the 4222 default (which would collide between
// parallel tests and on shared CI hosts).
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

func TestNewCommand(t *testing.T) {
	cmd := newCommand()
	if cmd.Use != "kscore-server" {
		t.Errorf("Use = %q, want %q", cmd.Use, "kscore-server")
	}
	if cmd.Short == "" {
		t.Error("Short is empty")
	}
}

// TestRun_BootsServesAndShutsDownCleanly exercises the full
// cmd/kscore-server boot path: build state.Store via the storage
// config mapper, construct the Server with a real internal/nats
// Manager (embedded mode on an ephemeral port), Start serving,
// observe ctx.Done, run Stop within the shutdown ceiling.
//
// Distinct from pkg/api/server's TestIntegration_FullLifecycle: that
// test drives the Server directly with real gRPC + HTTP clients.
// This one proves the cmd-binary wiring around it (config mapper +
// NATSManager wire-up + signal-driven shutdown) works end-to-end.
// We don't dial the bound ports here — they're 0-port ephemeral and
// the cmd binary doesn't expose them.
// runCfg returns a hermetic test config pointing at storePath.
func runCfg(t *testing.T, storePath string) *config.Config {
	t.Helper()
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
		Storage: config.StorageConfig{Driver: "sqlite", DSN: storePath},
		Health: config.HealthConfig{
			StartupGracePeriod: time.Millisecond,
			CheckTimeout:       time.Second,
		},
		NATS: config.NATSConfig{
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
				Port: freeTCPPort(t),
			},
		},
	}
}

// runWithLogs invokes run() with a buffer-backed slog handler,
// polls the buffer for the §4.4 step-20 startup banner (canonical
// "boot complete" signal), then cancels and waits for clean
// return. Polling instead of a wall-clock sleep makes the test
// robust under heavy parallel load (embedded NATS boot +
// ensureStreams + dev-key bootstrap can take a couple seconds
// when the rest of the suite is competing for CPU).
//
// Returns the captured logs for assertions.
func runWithLogs(t *testing.T, cfg *config.Config) *bytes.Buffer {
	t.Helper()
	buf := newSafeBuffer()
	log := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, log) }()

	waitForBanner(t, buf, 10*time.Second)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run: %v", err)
		}
	case <-time.After(35 * time.Second):
		t.Fatal("run did not return after cancel")
	}
	return buf.unwrap()
}

// safeBuffer wraps bytes.Buffer with a mutex so the slog goroutine
// (writing log records) can't race with the test goroutine
// (polling for the banner). Standard bytes.Buffer is not safe for
// concurrent use.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func newSafeBuffer() *safeBuffer { return &safeBuffer{} }

func (b *safeBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *safeBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// unwrap returns the internal buffer for the existing
// containsWarnLine helper, which expects *bytes.Buffer. The buffer
// is no longer being written to by the time unwrap is called
// (cancel + done has settled).
func (b *safeBuffer) unwrap() *bytes.Buffer {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := &bytes.Buffer{}
	out.Write(b.buf.Bytes())
	return out
}

// waitForBanner polls buf for the §4.4 step-20 banner log line
// ("kscore-server <version>...") which fires only after the full
// init sequence completes — including the dev-key bootstrap, NATS
// Start, and JetStream stream creation. t.Fatal if the banner
// doesn't appear within timeout.
func waitForBanner(t *testing.T, buf *safeBuffer, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), "kscore-server") {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("startup banner did not appear within %s; buf=%s", timeout, buf.String())
}

// containsWarnLine reports whether buf has a JSON log record matching
// level=WARN with msg containing needle.
func containsWarnLine(buf *bytes.Buffer, needle string) bool {
	for _, line := range strings.Split(buf.String(), "\n") {
		if line == "" {
			continue
		}
		var rec map[string]any
		if err := json.Unmarshal([]byte(line), &rec); err != nil {
			continue
		}
		if rec["level"] != "WARN" {
			continue
		}
		if msg, _ := rec["msg"].(string); strings.Contains(msg, needle) {
			return true
		}
	}
	return false
}

func TestRun_BootsServesAndShutsDownCleanly(t *testing.T) {
	cfg := runCfg(t, filepath.Join(t.TempDir(), "store.db"))
	// runWithLogs polls for the startup banner before cancelling, so
	// this test inherits the same robustness against slow-CI parallel
	// load.
	_ = runWithLogs(t, cfg)
}

func TestRun_DevMode_GeneratesAPIKey(t *testing.T) {
	cfg := runCfg(t, filepath.Join(t.TempDir(), "store.db"))

	buf := runWithLogs(t, cfg)
	if !containsWarnLine(buf, "DEV API KEY GENERATED") {
		t.Errorf("missing dev-key warn line:\n%s", buf.String())
	}
}

func TestRun_DevMode_DoesNotRegenerateExistingKey(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.db")
	cfg := runCfg(t, storePath)

	// Pre-create the dev key so EnsureDevKey takes the noop branch.
	preStore, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: storePath},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	if _, _, err := apikeys.EnsureDevKey(context.Background(), preStore); err != nil {
		t.Fatalf("seed EnsureDevKey: %v", err)
	}
	if err := preStore.Close(); err != nil {
		t.Fatalf("Close pre-seed store: %v", err)
	}

	buf := runWithLogs(t, cfg)
	if containsWarnLine(buf, "DEV API KEY GENERATED") {
		t.Errorf("dev-key warn line emitted on second run:\n%s", buf.String())
	}
}

func TestRun_ProductionMode_DoesNotGenerateKey(t *testing.T) {
	cfg := runCfg(t, filepath.Join(t.TempDir(), "store.db"))
	cfg.Mode = config.ModeProduction
	// Production mode requires TLS configured to pass Validate, but
	// we're not invoking config.Load here — the cfg goes straight to
	// run(), which doesn't re-validate. The dev-key check is what
	// we're testing; everything else is incidental.

	buf := runWithLogs(t, cfg)
	if containsWarnLine(buf, "DEV API KEY GENERATED") {
		t.Errorf("dev-key warn line emitted in production mode:\n%s", buf.String())
	}
}
