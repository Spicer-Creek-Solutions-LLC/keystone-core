//go:build linux

package cron

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type recordedCall struct {
	args  []string
	stdin string
}

func recorder(out string, err error) (commandRunner, *[]recordedCall) {
	var calls []recordedCall
	run := func(_ context.Context, _ string, args []string, stdin string) (string, error) {
		calls = append(calls, recordedCall{args: args, stdin: stdin})
		return out, err
	}
	return run, &calls
}

func TestLinuxProvider_Read(t *testing.T) {
	run, calls := recorder("0 0 * * * /bin/x\n", nil)
	p := &linuxProvider{crontab: "crontab", run: run}
	got, err := p.Read(context.Background(), "alice")
	if err != nil {
		t.Fatal(err)
	}
	if got != "0 0 * * * /bin/x\n" {
		t.Errorf("Read = %q", got)
	}
	if len(*calls) != 1 {
		t.Fatalf("expected 1 call, got %d", len(*calls))
	}
	if strings.Join((*calls)[0].args, " ") != "-l -u alice" {
		t.Errorf("Read args = %v", (*calls)[0].args)
	}
	if (*calls)[0].stdin != "" {
		t.Errorf("Read should not pass stdin, got %q", (*calls)[0].stdin)
	}
}

func TestLinuxProvider_Read_NoCrontabIsEmpty(t *testing.T) {
	run, _ := recorder("", errors.New("no crontab for alice"))
	p := &linuxProvider{crontab: "crontab", run: run}
	got, err := p.Read(context.Background(), "alice")
	if err != nil {
		t.Fatalf("'no crontab' should not be an error: %v", err)
	}
	if got != "" {
		t.Errorf("Read = %q, want empty", got)
	}

	// a different error propagates
	run, _ = recorder("", errors.New("crontab: must be privileged to use -u"))
	p = &linuxProvider{crontab: "crontab", run: run}
	if _, err := p.Read(context.Background(), "root"); err == nil {
		t.Error("a real error should propagate from Read")
	}
}

func TestLinuxProvider_Write(t *testing.T) {
	run, calls := recorder("", nil)
	p := &linuxProvider{crontab: "crontab", run: run}
	if err := p.Write(context.Background(), "alice", "@daily /bin/x\n"); err != nil {
		t.Fatal(err)
	}
	if strings.Join((*calls)[0].args, " ") != "-u alice -" {
		t.Errorf("Write args = %v", (*calls)[0].args)
	}
	if (*calls)[0].stdin != "@daily /bin/x\n" {
		t.Errorf("Write stdin = %q", (*calls)[0].stdin)
	}
}

func TestExecRun(t *testing.T) {
	t.Parallel()
	// `false` exits 1 — exercises the ExitError branch without
	// touching the real crontab.
	if _, err := execRun(context.Background(), "false", nil, ""); err == nil {
		t.Error("expected an error from `false`")
	}
	// a missing binary — non-ExitError branch
	if _, err := execRun(context.Background(), "/nonexistent/crontab", nil, ""); err == nil {
		t.Error("expected an error from a missing binary")
	}
	// `cat` echoes stdin back — exercises the stdin path + success
	out, err := execRun(context.Background(), "cat", nil, "hello\n")
	if err != nil {
		t.Fatal(err)
	}
	if out != "hello\n" {
		t.Errorf("cat round-trip = %q", out)
	}
}

func TestDefaultProvider_NonNil(t *testing.T) {
	t.Parallel()
	if defaultProvider() == nil {
		t.Fatal("defaultProvider returned nil")
	}
}
