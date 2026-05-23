package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"go.keystone-core.io/keystone-core/internal/cli"
	"go.keystone-core.io/keystone-core/internal/config"
)

func noopRun(_ context.Context, _ *config.Config, _ *slog.Logger) error { return nil }

func TestRootCommand_PanicsWithoutName(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when Name is empty")
		}
	}()
	cli.RootCommand(cli.Options{Run: noopRun})
}

func TestRootCommand_PanicsWithoutRun(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("expected panic when Run is nil")
		}
	}()
	cli.RootCommand(cli.Options{Name: "kscore-test"})
}

func TestRootCommand_VersionFlag(t *testing.T) {
	cmd := cli.RootCommand(cli.Options{
		Name:  "kscore-test",
		Short: "test binary",
		Run:   noopRun,
	})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got := out.String()
	for _, want := range []string{"kscore-test", "commit:", "built:"} {
		if !strings.Contains(got, want) {
			t.Errorf("--version output missing %q\nfull output:\n%s", want, got)
		}
	}
}

func TestAddVersion(t *testing.T) {
	cmd := &cobra.Command{Use: "kscore-example"}
	cli.AddVersion(cmd)

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--version"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	got := out.String()
	for _, want := range []string{"kscore-example", "commit:", "built:"} {
		if !strings.Contains(got, want) {
			t.Errorf("--version output missing %q\nfull output:\n%s", want, got)
		}
	}
}

func TestRootCommand_HelpFlag(t *testing.T) {
	cmd := cli.RootCommand(cli.Options{
		Name:  "kscore-test",
		Short: "test binary",
		Run:   noopRun,
	})

	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--help"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	if !strings.Contains(out.String(), "--config") {
		t.Errorf("--help did not advertise --config flag\noutput:\n%s", out.String())
	}
}

func TestRootCommand_ConfigMissing(t *testing.T) {
	cmd := cli.RootCommand(cli.Options{
		Name: "kscore-test",
		Run:  noopRun,
	})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", "testdata/does-not-exist.yaml"})

	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
	if !strings.Contains(err.Error(), "config:") {
		t.Errorf("expected error to wrap with 'config:' prefix; got %v", err)
	}
}

func TestRootCommand_ConfigLoaded(t *testing.T) {
	var got *config.Config
	cmd := cli.RootCommand(cli.Options{
		Name: "kscore-test",
		Run: func(_ context.Context, cfg *config.Config, _ *slog.Logger) error {
			got = cfg
			return nil
		},
	})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", "testdata/dev.yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if got == nil {
		t.Fatal("Run callback did not receive Config")
	}
	if got.Server.Host != "127.0.0.1" || got.Server.GRPCPort != 9090 {
		t.Errorf("unexpected config: %+v", got.Server)
	}
}

func TestRootCommand_StartupLogIsJSONWithCorrelationID(t *testing.T) {
	var buf bytes.Buffer
	cmd := cli.RootCommand(cli.Options{
		Name: "kscore-test",
		Run:  noopRun,
	})

	cmd.SetOut(&buf)
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", "testdata/dev.yaml"})

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected at least 2 log lines (starting + stopped); got %d:\n%s", len(lines), buf.String())
	}

	var rec map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &rec); err != nil {
		t.Fatalf("first log line is not JSON: %v\nline: %s", err, lines[0])
	}
	if rec["msg"] != "starting" {
		t.Errorf("first log line msg = %q, want %q", rec["msg"], "starting")
	}
	if rec["binary"] != "kscore-test" {
		t.Errorf("missing/wrong binary attr: %v", rec["binary"])
	}
	cid, ok := rec["correlation_id"].(string)
	if !ok || cid == "" {
		t.Errorf("missing/empty correlation_id; got %v", rec["correlation_id"])
	}
}

func TestRootCommand_ContextCancellationPropagates(t *testing.T) {
	var sawCancel atomic.Bool
	cmd := cli.RootCommand(cli.Options{
		Name: "kscore-test",
		Run: func(ctx context.Context, _ *config.Config, _ *slog.Logger) error {
			<-ctx.Done()
			sawCancel.Store(true)
			return nil
		},
	})

	parent, cancel := context.WithCancel(context.Background())
	cmd.SetContext(parent)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", "testdata/dev.yaml"})

	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !sawCancel.Load() {
		t.Fatal("Run callback did not observe context cancellation")
	}
}

func TestRootCommand_RunErrorPropagates(t *testing.T) {
	want := errors.New("boom")
	cmd := cli.RootCommand(cli.Options{
		Name: "kscore-test",
		Run: func(_ context.Context, _ *config.Config, _ *slog.Logger) error {
			return want
		},
	})

	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"--config", "testdata/dev.yaml"})

	if err := cmd.Execute(); !errors.Is(err, want) {
		t.Errorf("expected error %v; got %v", want, err)
	}
}
