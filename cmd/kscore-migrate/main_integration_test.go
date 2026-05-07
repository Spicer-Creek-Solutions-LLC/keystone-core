//go:build integration

package main

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	// state imports lib/pq blank, registering the postgres driver, so
	// this test can sql.Open("postgres", dsn) directly for setup +
	// truncate without repeating the import.
	"go.keystone-core.io/keystone-core/internal/state"
)

// runCLI invokes the kscore-migrate cobra command tree in-process with
// the given args and returns captured stdout, stderr, and the exit
// error from cmd.Execute. Each call constructs a fresh command so flag
// state doesn't leak between invocations.
func runCLI(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	cmd := newCommand()
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// requirePostgresDSN returns KSCORE_TEST_POSTGRES_DSN or skips. The
// integration build tag is the static gate; the env var is the runtime
// gate (so `make test-integration` works locally even without a
// Postgres standing by).
func requirePostgresDSN(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("KSCORE_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("KSCORE_TEST_POSTGRES_DSN not set")
	}
	return dsn
}

// ensurePostgresSchema opens the target via state.NewStore once so the
// v1.0 baseline schema exists. Idempotent (CREATE TABLE IF NOT EXISTS).
func ensurePostgresSchema(t *testing.T, dsn string) {
	t.Helper()
	s, err := state.NewStore(&state.Config{
		Backend:    state.BackendPostgreSQL,
		PostgreSQL: state.PostgreSQLConfig{DSN: dsn},
	})
	if err != nil {
		t.Fatalf("ensure pg schema: %v", err)
	}
	_ = s.Close()
}

// truncatePostgres clears all four v1.0 tables on the target. Run before
// each test to avoid cross-test pollution.
func truncatePostgres(t *testing.T, dsn string) {
	t.Helper()
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open pg: %v", err)
	}
	defer func() { _ = db.Close() }()
	const stmt = `TRUNCATE TABLE
        batch_agent_results, batch_jobs, commands, agents, apikeys
        RESTART IDENTITY CASCADE`
	if _, err := db.ExecContext(t.Context(), stmt); err != nil {
		t.Fatalf("truncate: %v", err)
	}
}

// seedSQLite writes a SQLite file at path with `n` agents, 3n commands
// (3 per agent), 50 batch_jobs (with cycle through agents), and 100
// batch_agent_results. Closes the store before returning so the CLI
// can reopen it without lock contention.
func seedSQLite(t *testing.T, path string, n int) (rowCounts map[string]int) {
	t.Helper()
	store, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: path},
	})
	if err != nil {
		t.Fatalf("open seed sqlite: %v", err)
	}
	defer func() { _ = store.Close() }()

	ctx := t.Context()
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)

	counts := map[string]int{}

	for i := 0; i < n; i++ {
		a := &state.AgentRecord{
			ID:           fmt.Sprintf("agent-%04d", i),
			Hostname:     fmt.Sprintf("host-%04d", i),
			OS:           "linux",
			Architecture: "amd64",
			IPAddresses:  []string{"10.0.0.1"},
			Labels:       map[string]string{"role": "web"},
			Status:       state.AgentStatusConnected,
			RegisteredAt: now,
		}
		if err := store.CreateAgent(ctx, a); err != nil {
			t.Fatalf("seed agent %d: %v", i, err)
		}
		counts["agents"]++

		for j := 0; j < 3; j++ {
			c := &state.CommandRecord{
				ID:        fmt.Sprintf("cmd-%04d-%d", i, j),
				AgentID:   a.ID,
				Command:   "uptime",
				Args:      []string{"-p"},
				Env:       map[string]string{},
				Status:    state.CommandStatusPending,
				StartedAt: now,
			}
			if err := store.CreateCommand(ctx, c); err != nil {
				t.Fatalf("seed command %d/%d: %v", i, j, err)
			}
			counts["commands"]++
		}
	}

	// 50 batch jobs, each cycling through existing agents.
	for i := 0; i < 50; i++ {
		b := &state.BatchJobRecord{
			ID:        fmt.Sprintf("job-%04d", i),
			Target:    map[string]any{"role": "web"},
			Command:   "uptime",
			Args:      []string{"-p"},
			Status:    state.BatchJobStatusPending,
			CreatedAt: now,
		}
		if err := store.CreateBatchJob(ctx, b); err != nil {
			t.Fatalf("seed batch job %d: %v", i, err)
		}
		counts["batch_jobs"]++
	}

	// 100 batch_agent_results spread across the first 50 jobs (2/job).
	for i := 0; i < 50; i++ {
		jobID := fmt.Sprintf("job-%04d", i)
		for j := 0; j < 2; j++ {
			agentID := fmt.Sprintf("agent-%04d", (i*2+j)%n)
			r := &state.BatchAgentResultRecord{
				BatchJobID:  jobID,
				AgentID:     agentID,
				Success:     j == 0,
				ExitCode:    j,
				StartedAt:   now,
				CompletedAt: now.Add(time.Second),
			}
			if err := store.CreateBatchAgentResult(ctx, r); err != nil {
				t.Fatalf("seed bar %s/%s: %v", jobID, agentID, err)
			}
			counts["batch_agent_results"]++
		}
	}

	return counts
}

func TestKscoreMigrateE2E_RunThenValidate(t *testing.T) {
	dsn := requirePostgresDSN(t)
	ensurePostgresSchema(t, dsn)
	truncatePostgres(t, dsn)

	sqlitePath := filepath.Join(t.TempDir(), "source.db")
	want := seedSQLite(t, sqlitePath, 100)

	// run migration via the CLI
	stdout, stderr, err := runCLI(t,
		"run",
		"--sqlite", sqlitePath,
		"--postgres", dsn,
		"--quiet",
	)
	if err != nil {
		t.Fatalf("run: err=%v\nstdout:\n%s\nstderr:\n%s", err, stdout, stderr)
	}
	for _, want := range []string{"Migration completed", "agents", "commands", "batch_jobs", "batch_agent_results"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("run stdout missing %q\nfull output:\n%s", want, stdout)
		}
	}

	// validate via the CLI
	vstdout, vstderr, verr := runCLI(t,
		"validate",
		"--sqlite", sqlitePath,
		"--postgres", dsn,
	)
	if verr != nil {
		t.Fatalf("validate: err=%v\nstdout:\n%s\nstderr:\n%s", verr, vstdout, vstderr)
	}
	if !strings.Contains(vstdout, "PASS") {
		t.Errorf("validate did not report PASS:\n%s", vstdout)
	}

	// sanity: per-table counts in stdout
	for table, n := range want {
		if !strings.Contains(vstdout, fmt.Sprintf("source=%d", n)) {
			t.Errorf("validate stdout missing source=%d for %s\nfull output:\n%s",
				n, table, vstdout)
		}
	}
}

func TestKscoreMigrateE2E_DryRunDoesntWrite(t *testing.T) {
	dsn := requirePostgresDSN(t)
	ensurePostgresSchema(t, dsn)
	truncatePostgres(t, dsn)

	sqlitePath := filepath.Join(t.TempDir(), "source.db")
	seedSQLite(t, sqlitePath, 50)

	stdout, _, err := runCLI(t,
		"run",
		"--sqlite", sqlitePath,
		"--postgres", dsn,
		"--dry-run",
		"--quiet",
	)
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	if !strings.Contains(stdout, "dry-run") {
		t.Errorf("dry-run header missing:\n%s", stdout)
	}

	// Validate must FAIL — target should still be empty.
	vstdout, _, verr := runCLI(t,
		"validate",
		"--sqlite", sqlitePath,
		"--postgres", dsn,
	)
	if verr == nil {
		t.Fatalf("expected validate to FAIL after dry-run; stdout:\n%s", vstdout)
	}
	if !strings.Contains(verr.Error(), "FAIL") {
		t.Errorf("validate error should report FAIL: %v", verr)
	}
}

func TestKscoreMigrateE2E_SkipExisting(t *testing.T) {
	dsn := requirePostgresDSN(t)
	ensurePostgresSchema(t, dsn)
	truncatePostgres(t, dsn)

	sqlitePath := filepath.Join(t.TempDir(), "source.db")
	seedSQLite(t, sqlitePath, 10)

	// Pre-populate target with one agent that has a colliding ID but a
	// distinct hostname; --skip-existing must preserve it.
	preExisting := preInsertAgent(t, dsn, "agent-0000", "pre-existing-host")
	defer func() { _ = preExisting }()

	_, _, err := runCLI(t,
		"run",
		"--sqlite", sqlitePath,
		"--postgres", dsn,
		"--skip-existing",
		"--quiet",
	)
	if err != nil {
		t.Fatalf("run --skip-existing: %v", err)
	}

	vstdout, _, verr := runCLI(t,
		"validate",
		"--sqlite", sqlitePath,
		"--postgres", dsn,
	)
	if verr != nil {
		t.Fatalf("validate after skip-existing: %v\n%s", verr, vstdout)
	}
	if !strings.Contains(vstdout, "PASS") {
		t.Errorf("validate did not PASS:\n%s", vstdout)
	}

	// Confirm the pre-existing row was preserved (not overwritten).
	store, err := state.NewStore(&state.Config{
		Backend:    state.BackendPostgreSQL,
		PostgreSQL: state.PostgreSQLConfig{DSN: dsn},
	})
	if err != nil {
		t.Fatalf("open target: %v", err)
	}
	defer store.Close()
	got, err := store.GetAgent(t.Context(), "agent-0000")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname != "pre-existing-host" {
		t.Errorf("--skip-existing overwrote existing row: hostname = %q", got.Hostname)
	}
}

// preInsertAgent inserts a single agent into the Postgres target with
// the given id+hostname so a subsequent --skip-existing migration can
// be observed.
func preInsertAgent(t *testing.T, dsn, id, hostname string) string {
	t.Helper()
	store, err := state.NewStore(&state.Config{
		Backend:    state.BackendPostgreSQL,
		PostgreSQL: state.PostgreSQLConfig{DSN: dsn},
	})
	if err != nil {
		t.Fatalf("open target for pre-insert: %v", err)
	}
	defer store.Close()

	a := &state.AgentRecord{
		ID:           id,
		Hostname:     hostname,
		OS:           "linux",
		Architecture: "amd64",
		IPAddresses:  []string{"10.0.0.1"},
		Labels:       map[string]string{"role": "web"},
		Status:       state.AgentStatusConnected,
		RegisteredAt: time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC),
	}
	if err := store.CreateAgent(context.Background(), a); err != nil {
		t.Fatalf("pre-insert: %v", err)
	}
	return id
}
