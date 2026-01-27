package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

// SQLiteStorage implements Storage using SQLite.
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage creates a new SQLite storage instance.
func NewSQLiteStorage(dbPath string) (*SQLiteStorage, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	s := &SQLiteStorage{db: db}

	// Initialize schema
	if err := s.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return s, nil
}

// initSchema creates the database tables if they don't exist.
func (s *SQLiteStorage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS runbook_executions (
		id TEXT PRIMARY KEY,
		runbook_name TEXT NOT NULL,
		runbook_version TEXT,
		state TEXT NOT NULL,
		inputs TEXT,
		outputs TEXT,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		error TEXT,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_executions_runbook ON runbook_executions(runbook_name);
	CREATE INDEX IF NOT EXISTS idx_executions_state ON runbook_executions(state);
	CREATE INDEX IF NOT EXISTS idx_executions_created ON runbook_executions(created_at);

	CREATE TABLE IF NOT EXISTS runbook_step_executions (
		id TEXT PRIMARY KEY,
		execution_id TEXT NOT NULL,
		step_name TEXT NOT NULL,
		step_type TEXT NOT NULL,
		state TEXT NOT NULL,
		inputs TEXT,
		outputs TEXT,
		started_at TIMESTAMP,
		completed_at TIMESTAMP,
		error TEXT,
		retry_count INTEGER DEFAULT 0,
		duration_ns INTEGER DEFAULT 0,
		FOREIGN KEY (execution_id) REFERENCES runbook_executions(id) ON DELETE CASCADE
	);

	CREATE INDEX IF NOT EXISTS idx_step_executions_exec ON runbook_step_executions(execution_id);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_step_executions_name ON runbook_step_executions(execution_id, step_name);
	`

	_, err := s.db.Exec(schema)
	return err
}

// SaveExecution saves or updates an execution record.
func (s *SQLiteStorage) SaveExecution(ctx context.Context, exec *runbook.Execution) error {
	inputsJSON, err := json.Marshal(exec.Inputs)
	if err != nil {
		return fmt.Errorf("marshal inputs: %w", err)
	}

	outputsJSON, err := json.Marshal(exec.Outputs)
	if err != nil {
		return fmt.Errorf("marshal outputs: %w", err)
	}

	query := `
	INSERT INTO runbook_executions (id, runbook_name, runbook_version, state, inputs, outputs, started_at, completed_at, error, created_at)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		state = excluded.state,
		outputs = excluded.outputs,
		started_at = excluded.started_at,
		completed_at = excluded.completed_at,
		error = excluded.error
	`

	_, err = s.db.ExecContext(ctx, query,
		exec.ID,
		exec.RunbookName,
		exec.RunbookVersion,
		string(exec.State),
		string(inputsJSON),
		string(outputsJSON),
		exec.StartedAt,
		exec.CompletedAt,
		exec.Error,
		exec.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("save execution: %w", err)
	}

	// Save step executions
	for _, step := range exec.Steps {
		if err := s.SaveStepExecution(ctx, exec.ID, step); err != nil {
			return fmt.Errorf("save step execution %s: %w", step.Name, err)
		}
	}

	return nil
}

// GetExecution retrieves an execution by ID.
func (s *SQLiteStorage) GetExecution(ctx context.Context, id string) (*runbook.Execution, error) {
	query := `
	SELECT id, runbook_name, runbook_version, state, inputs, outputs, started_at, completed_at, error, created_at
	FROM runbook_executions
	WHERE id = ?
	`

	var exec runbook.Execution
	var inputsJSON, outputsJSON string
	var startedAt, completedAt sql.NullTime
	var state string

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&exec.ID,
		&exec.RunbookName,
		&exec.RunbookVersion,
		&state,
		&inputsJSON,
		&outputsJSON,
		&startedAt,
		&completedAt,
		&exec.Error,
		&exec.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}

	exec.State = runbook.ExecutionState(state)

	if startedAt.Valid {
		exec.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		exec.CompletedAt = &completedAt.Time
	}

	if inputsJSON != "" {
		if err := json.Unmarshal([]byte(inputsJSON), &exec.Inputs); err != nil {
			return nil, fmt.Errorf("unmarshal inputs: %w", err)
		}
	}

	if outputsJSON != "" {
		if err := json.Unmarshal([]byte(outputsJSON), &exec.Outputs); err != nil {
			return nil, fmt.Errorf("unmarshal outputs: %w", err)
		}
	}

	// Load step executions
	steps, err := s.ListStepExecutions(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("load step executions: %w", err)
	}

	exec.Steps = make(map[string]*runbook.StepExecution)
	for _, step := range steps {
		exec.Steps[step.Name] = step
	}

	return &exec, nil
}

// ListExecutions lists executions with optional filtering.
func (s *SQLiteStorage) ListExecutions(ctx context.Context, opts ListOptions) ([]*runbook.Execution, error) {
	query := `
	SELECT id, runbook_name, runbook_version, state, inputs, outputs, started_at, completed_at, error, created_at
	FROM runbook_executions
	WHERE 1=1
	`
	var args []interface{}

	if opts.RunbookName != "" {
		query += " AND runbook_name = ?"
		args = append(args, opts.RunbookName)
	}

	if opts.State != "" {
		query += " AND state = ?"
		args = append(args, string(opts.State))
	}

	if opts.Since != nil {
		query += " AND created_at >= ?"
		args = append(args, opts.Since)
	}

	if opts.Until != nil {
		query += " AND created_at <= ?"
		args = append(args, opts.Until)
	}

	query += " ORDER BY created_at DESC"

	if opts.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, opts.Limit)
	}

	if opts.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, opts.Offset)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()

	var executions []*runbook.Execution
	for rows.Next() {
		var exec runbook.Execution
		var inputsJSON, outputsJSON string
		var startedAt, completedAt sql.NullTime
		var state string

		if err := rows.Scan(
			&exec.ID,
			&exec.RunbookName,
			&exec.RunbookVersion,
			&state,
			&inputsJSON,
			&outputsJSON,
			&startedAt,
			&completedAt,
			&exec.Error,
			&exec.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}

		exec.State = runbook.ExecutionState(state)

		if startedAt.Valid {
			exec.StartedAt = &startedAt.Time
		}
		if completedAt.Valid {
			exec.CompletedAt = &completedAt.Time
		}

		if inputsJSON != "" {
			if err := json.Unmarshal([]byte(inputsJSON), &exec.Inputs); err != nil {
				return nil, fmt.Errorf("unmarshal inputs: %w", err)
			}
		}

		if outputsJSON != "" {
			if err := json.Unmarshal([]byte(outputsJSON), &exec.Outputs); err != nil {
				return nil, fmt.Errorf("unmarshal outputs: %w", err)
			}
		}

		executions = append(executions, &exec)
	}

	return executions, rows.Err()
}

// DeleteExecution deletes an execution and its step records.
func (s *SQLiteStorage) DeleteExecution(ctx context.Context, id string) error {
	// Steps are deleted via CASCADE
	_, err := s.db.ExecContext(ctx, "DELETE FROM runbook_executions WHERE id = ?", id)
	return err
}

// SaveStepExecution saves or updates a step execution record.
func (s *SQLiteStorage) SaveStepExecution(ctx context.Context, executionID string, step *runbook.StepExecution) error {
	inputsJSON, err := json.Marshal(step.Inputs)
	if err != nil {
		return fmt.Errorf("marshal inputs: %w", err)
	}

	outputsJSON, err := json.Marshal(step.Outputs)
	if err != nil {
		return fmt.Errorf("marshal outputs: %w", err)
	}

	// Generate ID if not set
	stepID := fmt.Sprintf("%s-%s", executionID, step.Name)

	query := `
	INSERT INTO runbook_step_executions (id, execution_id, step_name, step_type, state, inputs, outputs, started_at, completed_at, error, retry_count, duration_ns)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(execution_id, step_name) DO UPDATE SET
		state = excluded.state,
		outputs = excluded.outputs,
		started_at = excluded.started_at,
		completed_at = excluded.completed_at,
		error = excluded.error,
		retry_count = excluded.retry_count,
		duration_ns = excluded.duration_ns
	`

	_, err = s.db.ExecContext(ctx, query,
		stepID,
		executionID,
		step.Name,
		string(step.Type),
		string(step.State),
		string(inputsJSON),
		string(outputsJSON),
		step.StartedAt,
		step.CompletedAt,
		step.Error,
		step.RetryCount,
		step.Duration.Nanoseconds(),
	)
	return err
}

// GetStepExecution retrieves a step execution by execution ID and step name.
func (s *SQLiteStorage) GetStepExecution(ctx context.Context, executionID, stepName string) (*runbook.StepExecution, error) {
	query := `
	SELECT step_name, step_type, state, inputs, outputs, started_at, completed_at, error, retry_count, duration_ns
	FROM runbook_step_executions
	WHERE execution_id = ? AND step_name = ?
	`

	return s.scanStepExecution(ctx, query, executionID, stepName)
}

// ListStepExecutions lists all step executions for an execution.
func (s *SQLiteStorage) ListStepExecutions(ctx context.Context, executionID string) ([]*runbook.StepExecution, error) {
	query := `
	SELECT step_name, step_type, state, inputs, outputs, started_at, completed_at, error, retry_count, duration_ns
	FROM runbook_step_executions
	WHERE execution_id = ?
	ORDER BY started_at
	`

	rows, err := s.db.QueryContext(ctx, query, executionID)
	if err != nil {
		return nil, fmt.Errorf("list step executions: %w", err)
	}
	defer rows.Close()

	var steps []*runbook.StepExecution
	for rows.Next() {
		step, err := s.scanStepExecutionRow(rows)
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
	}

	return steps, rows.Err()
}

func (s *SQLiteStorage) scanStepExecution(ctx context.Context, query string, args ...interface{}) (*runbook.StepExecution, error) {
	var step runbook.StepExecution
	var inputsJSON, outputsJSON string
	var startedAt, completedAt sql.NullTime
	var state, stepType string
	var durationNs int64

	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&step.Name,
		&stepType,
		&state,
		&inputsJSON,
		&outputsJSON,
		&startedAt,
		&completedAt,
		&step.Error,
		&step.RetryCount,
		&durationNs,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan step execution: %w", err)
	}

	step.Type = runbook.StepType(stepType)
	step.State = runbook.StepState(state)
	step.Duration = time.Duration(durationNs)

	if startedAt.Valid {
		step.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		step.CompletedAt = &completedAt.Time
	}

	if inputsJSON != "" {
		if err := json.Unmarshal([]byte(inputsJSON), &step.Inputs); err != nil {
			return nil, fmt.Errorf("unmarshal inputs: %w", err)
		}
	}

	if outputsJSON != "" {
		if err := json.Unmarshal([]byte(outputsJSON), &step.Outputs); err != nil {
			return nil, fmt.Errorf("unmarshal outputs: %w", err)
		}
	}

	return &step, nil
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func (s *SQLiteStorage) scanStepExecutionRow(row scanner) (*runbook.StepExecution, error) {
	var step runbook.StepExecution
	var inputsJSON, outputsJSON string
	var startedAt, completedAt sql.NullTime
	var state, stepType string
	var durationNs int64

	err := row.Scan(
		&step.Name,
		&stepType,
		&state,
		&inputsJSON,
		&outputsJSON,
		&startedAt,
		&completedAt,
		&step.Error,
		&step.RetryCount,
		&durationNs,
	)
	if err != nil {
		return nil, fmt.Errorf("scan step execution: %w", err)
	}

	step.Type = runbook.StepType(stepType)
	step.State = runbook.StepState(state)
	step.Duration = time.Duration(durationNs)

	if startedAt.Valid {
		step.StartedAt = &startedAt.Time
	}
	if completedAt.Valid {
		step.CompletedAt = &completedAt.Time
	}

	if inputsJSON != "" {
		if err := json.Unmarshal([]byte(inputsJSON), &step.Inputs); err != nil {
			return nil, fmt.Errorf("unmarshal inputs: %w", err)
		}
	}

	if outputsJSON != "" {
		if err := json.Unmarshal([]byte(outputsJSON), &step.Outputs); err != nil {
			return nil, fmt.Errorf("unmarshal outputs: %w", err)
		}
	}

	return &step, nil
}

// Close closes the database connection.
func (s *SQLiteStorage) Close() error {
	return s.db.Close()
}
