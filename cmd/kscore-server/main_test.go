package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
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
func TestRun_BootsServesAndShutsDownCleanly(t *testing.T) {
	cfg := &config.Config{
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
		Storage: config.StorageConfig{
			Driver: "sqlite",
			DSN:    filepath.Join(t.TempDir(), "store.db"),
		},
		Health: config.HealthConfig{
			StartupGracePeriod: time.Millisecond,
			CheckTimeout:       time.Second,
		},
	}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- run(ctx, cfg, log) }()

	// Let the server actually serve for a moment before signalling
	// shutdown — otherwise we'd just exercise the construct +
	// immediate-stop path that the unit tests already cover.
	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("run: %v", err)
		}
	case <-time.After(35 * time.Second): // 30s shutdown ceiling + slack
		t.Fatal("run did not return after cancel")
	}
}
