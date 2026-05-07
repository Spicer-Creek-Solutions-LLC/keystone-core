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

// run boots the full Server, blocks on ctx, and shuts down cleanly.
// We exercise that path with a pre-cancelled ctx and a temp-dir SQLite
// store so the test stays hermetic.
func TestRun_BootsAndShutsDownCleanly(t *testing.T) {
	cfg := &config.Config{
		Mode: config.ModeDevelopment,
		Server: config.ServerConfig{
			Host:     "127.0.0.1",
			GRPCPort: 0,
			HTTPPort: 0,
		},
		Logging: config.LoggingConfig{Level: "info", Format: "json"},
		Storage: config.StorageConfig{
			Driver: "sqlite",
			DSN:    filepath.Join(t.TempDir(), "store.db"),
		},
	}
	log := slog.New(slog.NewJSONHandler(io.Discard, nil))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cancel() // pre-cancel: run should boot, observe ctx.Done, then shut down.

	if err := run(ctx, cfg, log); err != nil {
		t.Fatalf("run: %v", err)
	}
}
