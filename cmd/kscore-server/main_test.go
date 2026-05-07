package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/api/apikeys"
)

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
// config mapper, construct the Server with a NoopNATSManager, Start
// serving, observe ctx.Done, run Stop within the shutdown ceiling.
//
// Distinct from pkg/api/server's TestIntegration_FullLifecycle: that
// test drives the Server directly with real gRPC + HTTP clients.
// This one proves the cmd-binary wiring around it (config mapper +
// NoopNATSManager wire-up + signal-driven shutdown) works
// end-to-end. We don't dial the bound ports here — they're 0-port
// ephemeral and the cmd binary doesn't expose them.
// runCfg returns a hermetic test config pointing at storePath.
func runCfg(storePath string) *config.Config {
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
	}
}

// runWithLogs invokes run() with a buffer-backed slog handler, lets
// it serve for ~100ms, then cancels and waits for clean return.
// Returns the captured logs for assertions.
func runWithLogs(t *testing.T, cfg *config.Config) *bytes.Buffer {
	t.Helper()
	buf := &bytes.Buffer{}
	log := slog.New(slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, log) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run: %v", err)
		}
	case <-time.After(35 * time.Second):
		t.Fatal("run did not return after cancel")
	}
	return buf
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
	cfg := runCfg(filepath.Join(t.TempDir(), "store.db"))
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, log) }()

	time.Sleep(100 * time.Millisecond)
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

func TestRun_DevMode_GeneratesAPIKey(t *testing.T) {
	cfg := runCfg(filepath.Join(t.TempDir(), "store.db"))

	buf := runWithLogs(t, cfg)
	if !containsWarnLine(buf, "DEV API KEY GENERATED") {
		t.Errorf("missing dev-key warn line:\n%s", buf.String())
	}
}

func TestRun_DevMode_DoesNotRegenerateExistingKey(t *testing.T) {
	storePath := filepath.Join(t.TempDir(), "store.db")
	cfg := runCfg(storePath)

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
	cfg := runCfg(filepath.Join(t.TempDir(), "store.db"))
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
