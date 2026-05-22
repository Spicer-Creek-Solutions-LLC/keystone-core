package state

import (
	"context"
	"fmt"
	"time"
)

// MigrationOptions configures a single Migrate call.
type MigrationOptions struct {
	// DryRun reads the source and emits progress but skips all writes.
	// Stats and txlog still record what would have happened.
	DryRun bool

	// BatchSize is the number of source rows read per page. Default 100.
	BatchSize int

	// ContinueOnError keeps Migrate going past per-row TARGET write
	// errors. Each error is recorded in MigrationStats.Errors and the
	// txlog.
	//
	// Scope: this flag tolerates per-row write errors only (FK
	// violations, duplicate keys without SkipExisting, NOT NULL
	// rejections, etc.). It does NOT recover from source-side read
	// failures — a corrupted source row makes its entire batch's
	// ListX scan fail and aborts the run. v1.0 assumes source data is
	// valid; surface source corruption with a separate validation
	// pass before migrating.
	ContinueOnError bool

	// SkipExisting appends "ON CONFLICT (id) DO NOTHING" (or composite-PK
	// equivalent for batch_agent_results) so re-runs are idempotent.
	SkipExisting bool

	// ProgressCallback fires once per batch per table with the current
	// rate + ETA. nil disables progress emission.
	ProgressCallback func(ProgressUpdate)

	// TxLogPath enables an append-only JSONL audit trail at the given
	// path. Empty disables the txlog.
	TxLogPath string
}

func (o *MigrationOptions) applyDefaults() {
	if o.BatchSize <= 0 {
		o.BatchSize = 100
	}
}

// MigrationStats summarizes one Migrate run.
type MigrationStats struct {
	StartedAt   time.Time
	CompletedAt time.Time
	Duration    time.Duration

	// Tables maps table name -> per-table counters.
	Tables map[string]TableStats

	// Errors collects per-row failures when ContinueOnError is set.
	Errors []MigrationError
}

// TableStats holds per-table counters during/after a migration run.
type TableStats struct {
	Read    int // rows read from source
	Written int // rows written to target (or would-be written in DryRun)
	Skipped int // rows skipped via ON CONFLICT DO NOTHING (SkipExisting)
	Errored int // rows that failed to write (ContinueOnError)
}

// MigrationError records a per-row failure.
type MigrationError struct {
	Table string
	ID    string
	Err   error
}

// ValidationResult summarizes a ValidateMigration call.
type ValidationResult struct {
	Tables map[string]ValidationTableResult
	Match  bool // true iff every table's source/target counts match
}

// ValidationTableResult is per-table count comparison.
type ValidationTableResult struct {
	SourceCount int
	TargetCount int
	Match       bool
}

// Migrator copies rows from a SQLite source to a PostgreSQL target,
// preserving FK order: agents -> commands -> batch_jobs ->
// batch_agent_results.
type Migrator struct {
	src *SQLiteStore
	dst *PostgreSQLStore
}

// NewMigrator returns a Migrator. Both stores must be open and have
// the v1.0 schema applied (NewStore handles schema on open).
func NewMigrator(src *SQLiteStore, dst *PostgreSQLStore) *Migrator {
	return &Migrator{src: src, dst: dst}
}

// Migrate copies every row from source to target in FK order.
//
// Returns the (possibly partial) MigrationStats even on error so the
// caller can report what completed before the failure.
func (m *Migrator) Migrate(ctx context.Context, opts MigrationOptions) (*MigrationStats, error) {
	opts.applyDefaults()

	stats := &MigrationStats{
		StartedAt: time.Now().UTC(),
		Tables:    map[string]TableStats{},
	}

	var txlog *TransactionLog
	if opts.TxLogPath != "" {
		l, err := OpenTxLog(opts.TxLogPath)
		if err != nil {
			return nil, fmt.Errorf("state: open txlog: %w", err)
		}
		txlog = l
		defer func() { _ = txlog.Close() }()
	}

	reporter := newProgressReporter(opts.ProgressCallback)

	stages := []func(context.Context, MigrationOptions, *MigrationStats, *TransactionLog, *progressReporter) error{
		m.migrateAgents,
		m.migrateCommands,
		m.migrateBatchJobs,
		m.migrateBatchAgentResults,
		m.migrateAPIKeys,
	}

	for _, stage := range stages {
		if err := stage(ctx, opts, stats, txlog, reporter); err != nil {
			return m.finalize(stats), err
		}
	}
	return m.finalize(stats), nil
}

func (m *Migrator) finalize(s *MigrationStats) *MigrationStats {
	s.CompletedAt = time.Now().UTC()
	s.Duration = s.CompletedAt.Sub(s.StartedAt)
	return s
}

// ValidateMigration compares source and target row counts for the four
// v1.0 tables. ValidationResult.Match is true iff every table matches.
func (m *Migrator) ValidateMigration(ctx context.Context) (*ValidationResult, error) {
	out := &ValidationResult{
		Tables: map[string]ValidationTableResult{},
		Match:  true,
	}
	for _, table := range migrationTables {
		var srcCount, tgtCount int
		if err := m.src.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&srcCount); err != nil {
			return nil, fmt.Errorf("state: count source %s: %w", table, err)
		}
		if err := m.dst.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&tgtCount); err != nil {
			return nil, fmt.Errorf("state: count target %s: %w", table, err)
		}
		match := srcCount == tgtCount
		if !match {
			out.Match = false
		}
		out.Tables[table] = ValidationTableResult{
			SourceCount: srcCount,
			TargetCount: tgtCount,
			Match:       match,
		}
	}
	return out, nil
}

// migrationTables in FK order; used by Migrate (and ValidateMigration
// for consistent table ordering). apikeys has no FK dependency on the
// other tables and is migrated last for clarity.
var migrationTables = []string{
	"agents", "commands", "batch_jobs", "batch_agent_results", "apikeys",
}

// ---- per-table migrations -------------------------------------------------

func (m *Migrator) migrateAgents(ctx context.Context, opts MigrationOptions, stats *MigrationStats, txlog *TransactionLog, reporter *progressReporter) error {
	const table = "agents"

	total, err := m.countSource(ctx, table)
	if err != nil {
		return err
	}
	reporter.Start(table, total)

	ts := TableStats{}
	defer func() { stats.Tables[table] = ts }()

	offset := 0
	for {
		agents, err := m.src.ListAgents(ctx, AgentFilter{
			SortColumn: "id", SortDesc: false,
			Limit: opts.BatchSize, Offset: offset,
		})
		if err != nil {
			return fmt.Errorf("state: read source agents at offset %d: %w", offset, err)
		}
		if len(agents) == 0 {
			break
		}

		for _, a := range agents {
			ts.Read++
			if err := m.processAgent(ctx, a, opts, &ts, txlog); err != nil {
				stats.Errors = append(stats.Errors, MigrationError{Table: table, ID: a.ID, Err: err})
				if !opts.ContinueOnError {
					return err
				}
			}
		}
		reporter.Update(table, ts.Read)
		offset += len(agents)
	}

	// Emit a final progress event so every table reports at least once
	// (empty tables would otherwise never fire Update via the loop).
	reporter.Update(table, ts.Read)
	checkpoint(txlog, table, lastIDOf(stats, table))
	return nil
}

func (m *Migrator) migrateCommands(ctx context.Context, opts MigrationOptions, stats *MigrationStats, txlog *TransactionLog, reporter *progressReporter) error {
	const table = "commands"

	total, err := m.countSource(ctx, table)
	if err != nil {
		return err
	}
	reporter.Start(table, total)

	ts := TableStats{}
	defer func() { stats.Tables[table] = ts }()

	offset := 0
	for {
		cmds, err := m.src.ListCommands(ctx, CommandFilter{
			SortColumn: "id", SortDesc: false,
			Limit: opts.BatchSize, Offset: offset,
		})
		if err != nil {
			return fmt.Errorf("state: read source commands at offset %d: %w", offset, err)
		}
		if len(cmds) == 0 {
			break
		}

		for _, c := range cmds {
			ts.Read++
			if err := m.processCommand(ctx, c, opts, &ts, txlog); err != nil {
				stats.Errors = append(stats.Errors, MigrationError{Table: table, ID: c.ID, Err: err})
				if !opts.ContinueOnError {
					return err
				}
			}
		}
		reporter.Update(table, ts.Read)
		offset += len(cmds)
	}

	reporter.Update(table, ts.Read)
	checkpoint(txlog, table, "")
	return nil
}

func (m *Migrator) migrateBatchJobs(ctx context.Context, opts MigrationOptions, stats *MigrationStats, txlog *TransactionLog, reporter *progressReporter) error {
	const table = "batch_jobs"

	total, err := m.countSource(ctx, table)
	if err != nil {
		return err
	}
	reporter.Start(table, total)

	ts := TableStats{}
	defer func() { stats.Tables[table] = ts }()

	offset := 0
	for {
		jobs, err := m.src.ListBatchJobs(ctx, BatchJobFilter{
			SortColumn: "id", SortDesc: false,
			Limit: opts.BatchSize, Offset: offset,
		})
		if err != nil {
			return fmt.Errorf("state: read source batch_jobs at offset %d: %w", offset, err)
		}
		if len(jobs) == 0 {
			break
		}

		for _, b := range jobs {
			ts.Read++
			if err := m.processBatchJob(ctx, b, opts, &ts, txlog); err != nil {
				stats.Errors = append(stats.Errors, MigrationError{Table: table, ID: b.ID, Err: err})
				if !opts.ContinueOnError {
					return err
				}
			}
		}
		reporter.Update(table, ts.Read)
		offset += len(jobs)
	}

	reporter.Update(table, ts.Read)
	checkpoint(txlog, table, "")
	return nil
}

func (m *Migrator) migrateAPIKeys(ctx context.Context, opts MigrationOptions, stats *MigrationStats, txlog *TransactionLog, reporter *progressReporter) error {
	const table = "apikeys"

	total, err := m.countSource(ctx, table)
	if err != nil {
		return err
	}
	reporter.Start(table, total)

	ts := TableStats{}
	defer func() { stats.Tables[table] = ts }()

	offset := 0
	for {
		keys, err := m.src.ListAPIKeys(ctx, APIKeyFilter{
			SortColumn: "id", SortDesc: false,
			Limit: opts.BatchSize, Offset: offset,
		})
		if err != nil {
			return fmt.Errorf("state: read source apikeys at offset %d: %w", offset, err)
		}
		if len(keys) == 0 {
			break
		}

		for _, k := range keys {
			ts.Read++
			if err := m.processAPIKey(ctx, k, opts, &ts, txlog); err != nil {
				stats.Errors = append(stats.Errors, MigrationError{Table: table, ID: k.ID, Err: err})
				if !opts.ContinueOnError {
					return err
				}
			}
		}
		reporter.Update(table, ts.Read)
		offset += len(keys)
	}

	reporter.Update(table, ts.Read)
	checkpoint(txlog, table, "")
	return nil
}

func (m *Migrator) processAPIKey(ctx context.Context, k *APIKeyRecord, opts MigrationOptions, ts *TableStats, txlog *TransactionLog) error {
	if opts.DryRun {
		ts.Written++
		appendTxLog(txlog, "apikeys", "insert", k.ID, "dryrun", nil)
		return nil
	}
	written, err := m.insertAPIKeyTarget(ctx, k, opts.SkipExisting)
	return handleInsertResult(txlog, "apikeys", k.ID, written, err, ts)
}

func (m *Migrator) migrateBatchAgentResults(ctx context.Context, opts MigrationOptions, stats *MigrationStats, txlog *TransactionLog, reporter *progressReporter) error {
	const table = "batch_agent_results"

	total, err := m.countSource(ctx, table)
	if err != nil {
		return err
	}
	reporter.Start(table, total)

	ts := TableStats{}
	defer func() { stats.Tables[table] = ts }()

	// batch_agent_results has no top-level filter API; iterate batch_jobs
	// then list per-job results.
	jobOffset := 0
	for {
		jobs, err := m.src.ListBatchJobs(ctx, BatchJobFilter{
			SortColumn: "id", SortDesc: false,
			Limit: opts.BatchSize, Offset: jobOffset,
		})
		if err != nil {
			return fmt.Errorf("state: enumerate batch_jobs: %w", err)
		}
		if len(jobs) == 0 {
			break
		}
		for _, j := range jobs {
			results, err := m.src.ListBatchAgentResults(ctx, j.ID)
			if err != nil {
				return fmt.Errorf("state: list batch_agent_results for %s: %w", j.ID, err)
			}
			for _, r := range results {
				ts.Read++
				id := r.BatchJobID + "/" + r.AgentID
				if err := m.processBatchAgentResult(ctx, r, opts, &ts, txlog); err != nil {
					stats.Errors = append(stats.Errors, MigrationError{Table: table, ID: id, Err: err})
					if !opts.ContinueOnError {
						return err
					}
				}
			}
			reporter.Update(table, ts.Read)
		}
		jobOffset += len(jobs)
	}

	reporter.Update(table, ts.Read)
	checkpoint(txlog, table, "")
	return nil
}

// ---- per-row write helpers ------------------------------------------------

func (m *Migrator) processAgent(ctx context.Context, a *AgentRecord, opts MigrationOptions, ts *TableStats, txlog *TransactionLog) error {
	if opts.DryRun {
		ts.Written++
		appendTxLog(txlog, "agents", "insert", a.ID, "dryrun", nil)
		return nil
	}
	written, err := m.insertAgentTarget(ctx, a, opts.SkipExisting)
	return handleInsertResult(txlog, "agents", a.ID, written, err, ts)
}

func (m *Migrator) processCommand(ctx context.Context, c *CommandRecord, opts MigrationOptions, ts *TableStats, txlog *TransactionLog) error {
	if opts.DryRun {
		ts.Written++
		appendTxLog(txlog, "commands", "insert", c.ID, "dryrun", nil)
		return nil
	}
	written, err := m.insertCommandTarget(ctx, c, opts.SkipExisting)
	return handleInsertResult(txlog, "commands", c.ID, written, err, ts)
}

func (m *Migrator) processBatchJob(ctx context.Context, b *BatchJobRecord, opts MigrationOptions, ts *TableStats, txlog *TransactionLog) error {
	if opts.DryRun {
		ts.Written++
		appendTxLog(txlog, "batch_jobs", "insert", b.ID, "dryrun", nil)
		return nil
	}
	written, err := m.insertBatchJobTarget(ctx, b, opts.SkipExisting)
	return handleInsertResult(txlog, "batch_jobs", b.ID, written, err, ts)
}

func (m *Migrator) processBatchAgentResult(ctx context.Context, r *BatchAgentResultRecord, opts MigrationOptions, ts *TableStats, txlog *TransactionLog) error {
	id := r.BatchJobID + "/" + r.AgentID
	if opts.DryRun {
		ts.Written++
		appendTxLog(txlog, "batch_agent_results", "insert", id, "dryrun", nil)
		return nil
	}
	written, err := m.insertBatchAgentResultTarget(ctx, r, opts.SkipExisting)
	return handleInsertResult(txlog, "batch_agent_results", id, written, err, ts)
}

// handleInsertResult routes the outcome of an insert into the right
// counter and txlog entry.
func handleInsertResult(txlog *TransactionLog, table, id string, written bool, err error, ts *TableStats) error {
	if err != nil {
		ts.Errored++
		appendTxLog(txlog, table, "insert", id, "error", err)
		return err
	}
	if written {
		ts.Written++
		appendTxLog(txlog, table, "insert", id, "ok", nil)
	} else {
		ts.Skipped++
		appendTxLog(txlog, table, "insert", id, "skipped", nil)
	}
	return nil
}

// ---- target insert SQL ----------------------------------------------------

// Migrator-internal INSERTs duplicate (intentionally) the
// PostgreSQLStore.Create* SQL so the optional ON CONFLICT clause can be
// appended uniformly. If a column changes, both sites must be updated.

const migrateInsertAgentSQL = `INSERT INTO agents (
    id, hostname, os, architecture, ip_addresses,
    platform_version, agent_version, labels, status,
    registered_at, last_heartbeat_at, metrics
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`

const migrateInsertCommandSQL = `INSERT INTO commands (
    id, agent_id, command, args, env,
    working_dir, "user", timeout_seconds, status,
    exit_code, stdout, stderr, started_at, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14)`

const migrateInsertBatchJobSQL = `INSERT INTO batch_jobs (
    id, target, command, args, status, concurrency,
    total_agents, completed_agents, successful_agents, failed_agents,
    created_at, started_at, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`

const migrateInsertBatchAgentResultSQL = `INSERT INTO batch_agent_results (
    batch_job_id, agent_id, success, exit_code, error,
    stdout, stderr, stdout_truncated, stderr_truncated,
    started_at, completed_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`

// gosec G101 false-positive: SQL fragment, not a credential.
//
// #nosec G101 -- SQL INSERT, not a credential.
//
//nolint:gosec
const migrateInsertAPIKeySQL = `INSERT INTO apikeys (
    id, name, key_hash, role, created_at, expires_at, last_used
) VALUES ($1, $2, $3, $4, $5, $6, $7)`

// insertAgentTarget writes a single AgentRecord to dst. Returns
// written=true if a row was inserted, false if SkipExisting suppressed
// it.
func (m *Migrator) insertAgentTarget(ctx context.Context, a *AgentRecord, skipExisting bool) (written bool, err error) {
	ipAddrs, err := marshalJSONBytes(a.IPAddresses)
	if err != nil {
		return false, err
	}
	labels, err := marshalJSONBytes(a.Labels)
	if err != nil {
		return false, err
	}
	metrics, err := agentMetricsArgPg(a.Metrics)
	if err != nil {
		return false, err
	}

	sql := migrateInsertAgentSQL
	if skipExisting {
		sql += ` ON CONFLICT (id) DO NOTHING`
	}

	res, err := m.dst.db.ExecContext(ctx, sql,
		a.ID, a.Hostname, a.OS, a.Architecture, ipAddrs,
		nullableString(a.PlatformVersion), nullableString(a.AgentVersion),
		labels, string(a.Status),
		a.RegisteredAt.UTC(), nullableTime(a.LastHeartbeatAt), metrics,
	)
	if err != nil {
		return false, err
	}
	return wasWritten(res), nil
}

func (m *Migrator) insertCommandTarget(ctx context.Context, c *CommandRecord, skipExisting bool) (bool, error) {
	args, err := marshalJSONBytes(c.Args)
	if err != nil {
		return false, err
	}
	env, err := marshalJSONBytes(c.Env)
	if err != nil {
		return false, err
	}

	sql := migrateInsertCommandSQL
	if skipExisting {
		sql += ` ON CONFLICT (id) DO NOTHING`
	}

	res, err := m.dst.db.ExecContext(ctx, sql,
		c.ID, c.AgentID, c.Command, args, env,
		nullableString(c.WorkingDir), nullableString(c.User),
		c.TimeoutSeconds, string(c.Status),
		nullableExitCode(c.ExitCode, c.Status),
		nullableString(c.Stdout), nullableString(c.Stderr),
		nullableTime(c.StartedAt), nullableTime(c.CompletedAt),
	)
	if err != nil {
		return false, err
	}
	return wasWritten(res), nil
}

func (m *Migrator) insertBatchJobTarget(ctx context.Context, b *BatchJobRecord, skipExisting bool) (bool, error) {
	target, err := marshalJSONBytes(b.Target)
	if err != nil {
		return false, err
	}
	args, err := marshalJSONBytes(b.Args)
	if err != nil {
		return false, err
	}

	sql := migrateInsertBatchJobSQL
	if skipExisting {
		sql += ` ON CONFLICT (id) DO NOTHING`
	}

	res, err := m.dst.db.ExecContext(ctx, sql,
		b.ID, target, b.Command, args, string(b.Status), b.Concurrency,
		b.TotalAgents, b.CompletedAgents, b.SuccessfulAgents, b.FailedAgents,
		b.CreatedAt.UTC(), nullableTime(b.StartedAt), nullableTime(b.CompletedAt),
	)
	if err != nil {
		return false, err
	}
	return wasWritten(res), nil
}

func (m *Migrator) insertBatchAgentResultTarget(ctx context.Context, r *BatchAgentResultRecord, skipExisting bool) (bool, error) {
	sql := migrateInsertBatchAgentResultSQL
	if skipExisting {
		sql += ` ON CONFLICT (batch_job_id, agent_id) DO NOTHING`
	}

	res, err := m.dst.db.ExecContext(ctx, sql,
		r.BatchJobID, r.AgentID, r.Success,
		nullableExitCodeForBatch(r.ExitCode, r.Success),
		nullableString(r.Error),
		r.Stdout, r.Stderr, r.StdoutTruncated, r.StderrTruncated,
		nullableTime(r.StartedAt), nullableTime(r.CompletedAt),
	)
	if err != nil {
		return false, err
	}
	return wasWritten(res), nil
}

func (m *Migrator) insertAPIKeyTarget(ctx context.Context, k *APIKeyRecord, skipExisting bool) (bool, error) {
	sql := migrateInsertAPIKeySQL
	if skipExisting {
		sql += ` ON CONFLICT (id) DO NOTHING`
	}

	res, err := m.dst.db.ExecContext(ctx, sql,
		k.ID, k.Name, k.KeyHash, k.Role,
		k.CreatedAt.UTC(),
		nullableTime(k.ExpiresAt),
		nullableTime(k.LastUsed),
	)
	if err != nil {
		return false, err
	}
	return wasWritten(res), nil
}

// ---- helpers --------------------------------------------------------------

// wasWritten translates RowsAffected into "did this insert produce a
// new row?" Used for SkipExisting / ON CONFLICT distinction.
func wasWritten(res any) bool {
	type rowAffecter interface {
		RowsAffected() (int64, error)
	}
	r, ok := res.(rowAffecter)
	if !ok {
		return true
	}
	n, err := r.RowsAffected()
	if err != nil {
		return true // assume written; we can't tell otherwise
	}
	return n > 0
}

// countSource returns the source row count for the given table.
func (m *Migrator) countSource(ctx context.Context, table string) (int, error) {
	var n int
	err := m.src.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("state: count source %s: %w", table, err)
	}
	return n, nil
}

func appendTxLog(t *TransactionLog, table, op, id, status string, err error) {
	if t == nil {
		return
	}
	entry := TxLogEntry{
		Time:   time.Now().UTC(),
		Table:  table,
		Op:     op,
		ID:     id,
		Status: status,
	}
	if err != nil {
		entry.Error = err.Error()
	}
	_ = t.Append(entry)
}

func checkpoint(t *TransactionLog, table, lastID string) {
	if t == nil {
		return
	}
	_ = t.Checkpoint(table, lastID)
}

func lastIDOf(_ *MigrationStats, _ string) string {
	// Future enhancement: track last-written id per table for
	// resume-from-checkpoint. For v1.0 the checkpoint marker exists in
	// the txlog format but the lastID field is unused.
	return ""
}

// nullableExitCodeForBatch handles batch_agent_results.exit_code which
// is nullable. Distinguishes "no exit code recorded" from "exited 0".
func nullableExitCodeForBatch(code int, success bool) any {
	if code == 0 && success {
		// Treat as unknown -> NULL so re-migration is consistent.
		return nil
	}
	return code
}

