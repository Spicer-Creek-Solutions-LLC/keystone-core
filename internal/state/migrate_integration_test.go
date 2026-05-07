//go:build integration

package state

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// migrateTestPair returns a SQLite source store and a Postgres target
// store both with the v1.0 schema applied. Skips if Postgres isn't
// available (KSCORE_TEST_POSTGRES_DSN unset).
func migrateTestPair(t *testing.T) (*SQLiteStore, *PostgreSQLStore) {
	t.Helper()
	src := newSQLiteStoreForTest(t)
	dst := newPgStoreForTest(t)
	return src, dst
}

// seedSource inserts N agents, N*3 commands, and N batch_jobs each with
// 2 batch_agent_results into the source SQLite store. Returns total
// rows-per-table for assertions.
func seedSource(t *testing.T, src *SQLiteStore, n int) (agents, commands, jobs, results int) {
	t.Helper()
	ctx := t.Context()

	for i := 0; i < n; i++ {
		a := sampleAgent(fmt.Sprintf("agent-%03d", i))
		if err := src.CreateAgent(ctx, a); err != nil {
			t.Fatalf("seed agent: %v", err)
		}
		agents++

		for j := 0; j < 3; j++ {
			c := sampleCommand(fmt.Sprintf("cmd-%03d-%d", i, j), a.ID)
			if err := src.CreateCommand(ctx, c); err != nil {
				t.Fatalf("seed command: %v", err)
			}
			commands++
		}

		b := sampleBatchJob(fmt.Sprintf("job-%03d", i))
		if err := src.CreateBatchJob(ctx, b); err != nil {
			t.Fatalf("seed batch job: %v", err)
		}
		jobs++

		for j := 0; j < 2; j++ {
			r := &BatchAgentResultRecord{
				BatchJobID:  b.ID,
				AgentID:     a.ID,
				Success:     j == 0,
				ExitCode:    j,
				Error:       "",
				StartedAt:   time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC),
				CompletedAt: time.Date(2026, 5, 6, 14, 0, 1, 0, time.UTC),
			}
			// Distinct (batch_job_id, agent_id) pairs required by PK,
			// so vary agent_id for j>0.
			if j > 0 {
				// seed an additional agent for this BAR row
				other := sampleAgent(fmt.Sprintf("agent-other-%03d-%d", i, j))
				if err := src.CreateAgent(ctx, other); err != nil {
					t.Fatal(err)
				}
				agents++
				r.AgentID = other.ID
			}
			if err := src.CreateBatchAgentResult(ctx, r); err != nil {
				t.Fatalf("seed batch_agent_result: %v", err)
			}
			results++
		}
	}
	return agents, commands, jobs, results
}

func TestMigrator_FullFlow(t *testing.T) {
	src, dst := migrateTestPair(t)
	wantAgents, wantCmds, wantJobs, wantResults := seedSource(t, src, 10)

	m := NewMigrator(src, dst)
	stats, err := m.Migrate(t.Context(), MigrationOptions{BatchSize: 5})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if stats.Tables["agents"].Read != wantAgents || stats.Tables["agents"].Written != wantAgents {
		t.Errorf("agents stats: %+v, want Read=%d Written=%d",
			stats.Tables["agents"], wantAgents, wantAgents)
	}
	if stats.Tables["commands"].Written != wantCmds {
		t.Errorf("commands written: %d, want %d", stats.Tables["commands"].Written, wantCmds)
	}
	if stats.Tables["batch_jobs"].Written != wantJobs {
		t.Errorf("batch_jobs written: %d, want %d", stats.Tables["batch_jobs"].Written, wantJobs)
	}
	if stats.Tables["batch_agent_results"].Written != wantResults {
		t.Errorf("batch_agent_results written: %d, want %d",
			stats.Tables["batch_agent_results"].Written, wantResults)
	}
	if stats.Duration <= 0 {
		t.Errorf("Duration should be > 0; got %s", stats.Duration)
	}

	vr, err := m.ValidateMigration(t.Context())
	if err != nil {
		t.Fatalf("ValidateMigration: %v", err)
	}
	if !vr.Match {
		t.Errorf("ValidateMigration.Match = false; per-table: %+v", vr.Tables)
	}
}

func TestMigrator_DryRun(t *testing.T) {
	src, dst := migrateTestPair(t)
	seedSource(t, src, 3)

	m := NewMigrator(src, dst)
	stats, err := m.Migrate(t.Context(), MigrationOptions{DryRun: true})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	if stats.Tables["agents"].Read == 0 {
		t.Error("DryRun should still Read source rows")
	}
	if stats.Tables["agents"].Written == 0 {
		t.Error("DryRun should record Written counts (would-be writes)")
	}

	// Target must be empty
	var n int
	if err := dst.db.QueryRowContext(t.Context(), `SELECT COUNT(*) FROM agents`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("DryRun wrote to target: agents count = %d", n)
	}
}

func TestMigrator_SkipExisting(t *testing.T) {
	src, dst := migrateTestPair(t)
	seedSource(t, src, 3)

	// Pre-seed one agent on the target so it collides on migrate.
	preExist := sampleAgent("agent-000")
	preExist.Hostname = "pre-existing"
	if err := dst.CreateAgent(t.Context(), preExist); err != nil {
		t.Fatal(err)
	}

	m := NewMigrator(src, dst)
	stats, err := m.Migrate(t.Context(), MigrationOptions{SkipExisting: true})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	agentStats := stats.Tables["agents"]
	if agentStats.Skipped < 1 {
		t.Errorf("expected at least 1 skipped agent; got %+v", agentStats)
	}

	// Pre-existing row should retain its ORIGINAL hostname.
	got, err := dst.GetAgent(t.Context(), "agent-000")
	if err != nil {
		t.Fatal(err)
	}
	if got.Hostname != "pre-existing" {
		t.Errorf("SkipExisting overwrote: hostname = %q", got.Hostname)
	}
}

func TestMigrator_TxLog(t *testing.T) {
	src, dst := migrateTestPair(t)
	seedSource(t, src, 2)

	logPath := filepath.Join(t.TempDir(), "migrate.jsonl")
	m := NewMigrator(src, dst)
	if _, err := m.Migrate(t.Context(), MigrationOptions{TxLogPath: logPath}); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	f, err := os.Open(logPath)
	if err != nil {
		t.Fatalf("open txlog: %v", err)
	}
	defer f.Close()

	var (
		insertCount int
		checkpoints int
	)
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var e TxLogEntry
		if err := json.Unmarshal(scanner.Bytes(), &e); err != nil {
			t.Fatalf("decode: %v", err)
		}
		switch e.Op {
		case "insert":
			insertCount++
		case "checkpoint":
			checkpoints++
		}
	}
	if scanner.Err() != nil {
		t.Fatal(scanner.Err())
	}
	if insertCount == 0 {
		t.Error("expected insert entries in txlog")
	}
	// One checkpoint per migrationTables entry (5 in v1.0:
	// agents, commands, batch_jobs, batch_agent_results, apikeys).
	if checkpoints != len(migrationTables) {
		t.Errorf("expected %d checkpoints (one per table); got %d",
			len(migrationTables), checkpoints)
	}
}

func TestMigrator_ProgressCallback(t *testing.T) {
	src, dst := migrateTestPair(t)
	seedSource(t, src, 5)

	var updates []ProgressUpdate
	m := NewMigrator(src, dst)
	_, err := m.Migrate(t.Context(), MigrationOptions{
		BatchSize: 2,
		ProgressCallback: func(p ProgressUpdate) {
			updates = append(updates, p)
		},
	})
	if err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	if len(updates) == 0 {
		t.Fatal("expected ProgressCallback to fire at least once")
	}
	tablesSeen := map[string]bool{}
	for _, u := range updates {
		tablesSeen[u.Table] = true
	}
	for _, want := range migrationTables {
		if !tablesSeen[want] {
			t.Errorf("missing progress for table %q", want)
		}
	}
}

// TestMigrator_ContinueOnError exercises the documented contract for
// ContinueOnError: per-row TARGET write failures are recorded in
// stats.Errors and the run continues. Source-side read errors are not
// covered by ContinueOnError — a malformed source row fails the whole
// batch's ListX scan and aborts the run regardless of the flag. That
// boundary is documented in migrate.go.
func TestMigrator_ContinueOnError(t *testing.T) {
	src, dst := migrateTestPair(t)
	seedSource(t, src, 3)

	// Pre-populate target with an agent whose ID collides with one on
	// source (different hostname so we can verify it's untouched).
	// Without SkipExisting, the target INSERT for that ID will fail
	// with a duplicate-key error — exactly the kind of per-row error
	// ContinueOnError is designed to tolerate.
	pre := sampleAgent("agent-001")
	pre.Hostname = "pre-existing"
	if err := dst.CreateAgent(t.Context(), pre); err != nil {
		t.Fatal(err)
	}

	m := NewMigrator(src, dst)

	// Without ContinueOnError, the duplicate-key error aborts.
	_, err := m.Migrate(t.Context(), MigrationOptions{})
	if err == nil {
		t.Error("expected migration to fail on duplicate key without ContinueOnError")
	}

	// Reset target and re-insert the colliding row so the second run
	// hits the same conflict.
	truncateAll(t, dst.db)
	if err := dst.CreateAgent(t.Context(), pre); err != nil {
		t.Fatal(err)
	}

	// With ContinueOnError, the duplicate is recorded but other rows
	// migrate successfully.
	stats, err := m.Migrate(t.Context(), MigrationOptions{
		ContinueOnError: true,
	})
	if err != nil {
		t.Fatalf("Migrate with ContinueOnError: %v", err)
	}
	if len(stats.Errors) == 0 {
		t.Fatal("expected at least one error in stats.Errors")
	}
	if stats.Tables["agents"].Errored == 0 {
		t.Errorf("expected agents.Errored > 0; got %+v", stats.Tables["agents"])
	}
	if stats.Tables["agents"].Written < 2 {
		t.Errorf("expected at least 2 agents written (the non-conflicting ones); got %+v",
			stats.Tables["agents"])
	}
	// At least one error should reference agents/agent-001.
	var foundDup bool
	for _, e := range stats.Errors {
		if e.Table == "agents" && e.ID == "agent-001" {
			foundDup = true
			break
		}
	}
	if !foundDup {
		t.Errorf("expected duplicate-key error for agents/agent-001; got %+v", stats.Errors)
	}
}

// Sanity check that Migrate returns the (possibly partial) stats even
// when an early-stage error aborts the run.
func TestMigrator_ReturnsPartialStatsOnError(t *testing.T) {
	src, dst := migrateTestPair(t)
	seedSource(t, src, 1)
	// Force migrate to fail on commands by pre-inserting an agent on
	// the target with a colliding ID. Without SkipExisting, the
	// duplicate-key error aborts.
	dup := sampleAgent("agent-000")
	if err := dst.CreateAgent(t.Context(), dup); err != nil {
		t.Fatal(err)
	}

	m := NewMigrator(src, dst)
	stats, err := m.Migrate(context.Background(), MigrationOptions{})
	if err == nil {
		t.Fatal("expected duplicate-key error")
	}
	if stats == nil {
		t.Fatal("stats should be non-nil even on error")
	}
	if stats.Duration <= 0 {
		t.Error("Duration should be set on error path")
	}
}
