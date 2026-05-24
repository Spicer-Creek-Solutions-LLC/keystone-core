// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/state"
)

func TestNewCommand_HasSubcommands(t *testing.T) {
	cmd := newCommand()
	want := map[string]bool{"run": false, "validate": false, "version": false}
	for _, c := range cmd.Commands() {
		want[c.Name()] = true
	}
	for sub, ok := range want {
		if !ok {
			t.Errorf("missing subcommand: %q", sub)
		}
	}
}

func TestRootCommand_VersionFlag(t *testing.T) {
	cmd := newCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"--version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	for _, want := range []string{"kscore-migrate", "commit:", "built:"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("--version output missing %q\nfull output:\n%s", want, out.String())
		}
	}
}

func TestVersionSubcommand(t *testing.T) {
	cmd := newCommand()
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	cmd.SetArgs([]string{"version"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("execute: %v", err)
	}
	if !strings.Contains(out.String(), "kscore-migrate") {
		t.Errorf("version output missing binary name:\n%s", out.String())
	}
}

func TestRunCommand_RequiresSqliteFlag(t *testing.T) {
	cmd := newCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"run", "--postgres", "postgres://x"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --sqlite")
	}
	if !strings.Contains(err.Error(), "sqlite") {
		t.Errorf("error should mention sqlite flag: %v", err)
	}
}

func TestRunCommand_RequiresPostgresFlag(t *testing.T) {
	cmd := newCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"run", "--sqlite", "/tmp/x.db"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing --postgres")
	}
	if !strings.Contains(err.Error(), "postgres") {
		t.Errorf("error should mention postgres flag: %v", err)
	}
}

func TestRunCommand_FlagDefaults(t *testing.T) {
	// Inspect the run subcommand's flag set without executing.
	root := newCommand()
	out := &bytes.Buffer{}
	root.SetOut(out)
	root.SetErr(out)
	for _, c := range root.Commands() {
		if c.Name() != "run" {
			continue
		}
		bs, _ := c.Flags().GetInt("batch-size")
		if bs != 100 {
			t.Errorf("--batch-size default = %d, want 100", bs)
		}
		dr, _ := c.Flags().GetBool("dry-run")
		if dr {
			t.Errorf("--dry-run default = true, want false")
		}
		coe, _ := c.Flags().GetBool("continue-on-error")
		if coe {
			t.Errorf("--continue-on-error default = true, want false")
		}
		return
	}
	t.Fatal("run subcommand not found")
}

func TestValidateCommand_RequiresFlags(t *testing.T) {
	cmd := newCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"validate"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing required flags on validate")
	}
}

func TestFormatProgress(t *testing.T) {
	got := formatProgress(state.ProgressUpdate{
		Table:         "agents",
		RowsCompleted: 250,
		RowsTotal:     1000,
		RowsPerSecond: 80.5,
		ETA:           9 * time.Second,
	})
	for _, want := range []string{"agents", "250/1000", "25.0%", "80.5", "9s"} {
		if !strings.Contains(got, want) {
			t.Errorf("formatProgress missing %q\noutput: %s", want, got)
		}
	}
}

func TestFormatProgress_UnknownTotal(t *testing.T) {
	got := formatProgress(state.ProgressUpdate{
		Table:         "agents",
		RowsCompleted: 50,
		RowsTotal:     0,
	})
	if !strings.Contains(got, "0.0%") {
		t.Errorf("expected 0.0%% for unknown total; got: %s", got)
	}
	if !strings.Contains(got, "ETA ?") {
		t.Errorf("expected ETA ? for zero ETA; got: %s", got)
	}
}

func TestPrintStats_DryRunHeader(t *testing.T) {
	var buf bytes.Buffer
	stats := &state.MigrationStats{
		Duration: 4200 * time.Millisecond,
		Tables: map[string]state.TableStats{
			"agents":              {Read: 10, Written: 10},
			"commands":            {Read: 30, Written: 30},
			"batch_jobs":          {Read: 5, Written: 5},
			"batch_agent_results": {Read: 10, Written: 10},
		},
	}
	printStats(&buf, stats, true)
	out := buf.String()
	if !strings.Contains(out, "dry-run") {
		t.Errorf("dry-run header missing:\n%s", out)
	}
	if !strings.Contains(out, "agents") || !strings.Contains(out, "10") {
		t.Errorf("table row missing:\n%s", out)
	}
}

func TestPrintStats_RegularHeader(t *testing.T) {
	var buf bytes.Buffer
	printStats(&buf, &state.MigrationStats{
		Duration: time.Second,
		Tables:   map[string]state.TableStats{},
	}, false)
	out := buf.String()
	if strings.Contains(out, "dry-run") {
		t.Errorf("non-dry-run output should not contain 'dry-run':\n%s", out)
	}
	if !strings.Contains(out, "Migration completed") {
		t.Errorf("missing 'Migration completed' header:\n%s", out)
	}
}

func TestPrintStats_NilSafe(t *testing.T) {
	var buf bytes.Buffer
	printStats(&buf, nil, false)
	if buf.Len() != 0 {
		t.Errorf("nil stats should produce no output; got: %s", buf.String())
	}
}

func TestPrintStats_RendersErrors(t *testing.T) {
	var buf bytes.Buffer
	printStats(&buf, &state.MigrationStats{
		Duration: time.Second,
		Tables:   map[string]state.TableStats{},
		Errors: []state.MigrationError{
			{Table: "agents", ID: "a-1", Err: errSentinel},
		},
	}, false)
	out := buf.String()
	if !strings.Contains(out, "1 error(s)") {
		t.Errorf("error count summary missing:\n%s", out)
	}
	if !strings.Contains(out, "a-1") {
		t.Errorf("error row id missing:\n%s", out)
	}
}

// errSentinel is a fixed error used by TestPrintStats_RendersErrors.
var errSentinel = sentinelErr{}

type sentinelErr struct{}

func (sentinelErr) Error() string { return "boom" }
