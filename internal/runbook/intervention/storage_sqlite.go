package intervention

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteStorage implements Storage using SQLite.
type SQLiteStorage struct {
	db *sql.DB
}

// NewSQLiteStorage creates a new SQLite storage instance.
func NewSQLiteStorage(db *sql.DB) (*SQLiteStorage, error) {
	s := &SQLiteStorage{db: db}

	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return s, nil
}

// initSchema creates the database tables if they don't exist.
func (s *SQLiteStorage) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS runbook_intervention_requests (
		id TEXT PRIMARY KEY,
		execution_id TEXT NOT NULL,
		step_name TEXT NOT NULL,
		type TEXT NOT NULL,
		state TEXT NOT NULL,
		title TEXT NOT NULL,
		description TEXT,
		prompts TEXT,
		response TEXT,
		timeout_ns INTEGER,
		expires_at TIMESTAMP,
		metadata TEXT,
		created_at TIMESTAMP NOT NULL,
		updated_at TIMESTAMP NOT NULL,
		completed_at TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_intervention_requests_execution ON runbook_intervention_requests(execution_id);
	CREATE INDEX IF NOT EXISTS idx_intervention_requests_state ON runbook_intervention_requests(state);
	CREATE INDEX IF NOT EXISTS idx_intervention_requests_expires ON runbook_intervention_requests(expires_at);
	CREATE UNIQUE INDEX IF NOT EXISTS idx_intervention_requests_exec_step ON runbook_intervention_requests(execution_id, step_name);
	`

	_, err := s.db.Exec(schema)
	return err
}

// SaveRequest saves or updates an intervention request.
func (s *SQLiteStorage) SaveRequest(ctx context.Context, req *Request) error {
	promptsJSON, err := json.Marshal(req.Prompts)
	if err != nil {
		return fmt.Errorf("marshal prompts: %w", err)
	}

	responseJSON, err := json.Marshal(req.Response)
	if err != nil {
		return fmt.Errorf("marshal response: %w", err)
	}

	metadataJSON, err := json.Marshal(req.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := `
	INSERT INTO runbook_intervention_requests (
		id, execution_id, step_name, type, state, title, description,
		prompts, response, timeout_ns, expires_at, metadata,
		created_at, updated_at, completed_at
	)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		state = excluded.state,
		response = excluded.response,
		updated_at = excluded.updated_at,
		completed_at = excluded.completed_at,
		metadata = excluded.metadata
	`

	_, err = s.db.ExecContext(ctx, query,
		req.ID,
		req.ExecutionID,
		req.StepName,
		string(req.Type),
		string(req.State),
		req.Title,
		req.Description,
		string(promptsJSON),
		string(responseJSON),
		req.Timeout.Nanoseconds(),
		req.ExpiresAt,
		string(metadataJSON),
		req.CreatedAt,
		req.UpdatedAt,
		req.CompletedAt,
	)
	if err != nil {
		return fmt.Errorf("save request: %w", err)
	}

	return nil
}

// GetRequest retrieves an intervention request by ID.
func (s *SQLiteStorage) GetRequest(ctx context.Context, id string) (*Request, error) {
	query := `
	SELECT id, execution_id, step_name, type, state, title, description,
		prompts, response, timeout_ns, expires_at, metadata,
		created_at, updated_at, completed_at
	FROM runbook_intervention_requests
	WHERE id = ?
	`

	return s.scanRequest(ctx, query, id)
}

// GetRequestByExecution retrieves an intervention request by execution ID and step name.
func (s *SQLiteStorage) GetRequestByExecution(ctx context.Context, executionID, stepName string) (*Request, error) {
	query := `
	SELECT id, execution_id, step_name, type, state, title, description,
		prompts, response, timeout_ns, expires_at, metadata,
		created_at, updated_at, completed_at
	FROM runbook_intervention_requests
	WHERE execution_id = ? AND step_name = ?
	`

	return s.scanRequest(ctx, query, executionID, stepName)
}

// ListRequests lists intervention requests with optional filtering.
func (s *SQLiteStorage) ListRequests(ctx context.Context, opts ListOptions) ([]*Request, error) {
	query := `
	SELECT id, execution_id, step_name, type, state, title, description,
		prompts, response, timeout_ns, expires_at, metadata,
		created_at, updated_at, completed_at
	FROM runbook_intervention_requests
	WHERE 1=1
	`
	var args []interface{}

	if opts.ExecutionID != "" {
		query += " AND execution_id = ?"
		args = append(args, opts.ExecutionID)
	}

	if opts.State != "" {
		query += " AND state = ?"
		args = append(args, string(opts.State))
	}

	if opts.Type != "" {
		query += " AND type = ?"
		args = append(args, string(opts.Type))
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
		return nil, fmt.Errorf("list requests: %w", err)
	}
	defer rows.Close()

	var requests []*Request
	for rows.Next() {
		req, err := s.scanRequestRow(rows)
		if err != nil {
			return nil, err
		}
		requests = append(requests, req)
	}

	return requests, rows.Err()
}

// DeleteRequest deletes an intervention request.
func (s *SQLiteStorage) DeleteRequest(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM runbook_intervention_requests WHERE id = ?", id)
	return err
}

func (s *SQLiteStorage) scanRequest(ctx context.Context, query string, args ...interface{}) (*Request, error) {
	var req Request
	var promptsJSON, responseJSON, metadataJSON string
	var state, reqType string
	var expiresAt, completedAt sql.NullTime
	var timeoutNs int64

	err := s.db.QueryRowContext(ctx, query, args...).Scan(
		&req.ID,
		&req.ExecutionID,
		&req.StepName,
		&reqType,
		&state,
		&req.Title,
		&req.Description,
		&promptsJSON,
		&responseJSON,
		&timeoutNs,
		&expiresAt,
		&metadataJSON,
		&req.CreatedAt,
		&req.UpdatedAt,
		&completedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("scan request: %w", err)
	}

	req.Type = InterventionType(reqType)
	req.State = InterventionState(state)
	req.Timeout = time.Duration(timeoutNs)

	if expiresAt.Valid {
		req.ExpiresAt = &expiresAt.Time
	}
	if completedAt.Valid {
		req.CompletedAt = &completedAt.Time
	}

	if promptsJSON != "" && promptsJSON != "null" {
		if err := json.Unmarshal([]byte(promptsJSON), &req.Prompts); err != nil {
			return nil, fmt.Errorf("unmarshal prompts: %w", err)
		}
	}

	if responseJSON != "" && responseJSON != "null" {
		if err := json.Unmarshal([]byte(responseJSON), &req.Response); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
	}

	if metadataJSON != "" && metadataJSON != "null" {
		if err := json.Unmarshal([]byte(metadataJSON), &req.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return &req, nil
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func (s *SQLiteStorage) scanRequestRow(row scanner) (*Request, error) {
	var req Request
	var promptsJSON, responseJSON, metadataJSON string
	var state, reqType string
	var expiresAt, completedAt sql.NullTime
	var timeoutNs int64

	err := row.Scan(
		&req.ID,
		&req.ExecutionID,
		&req.StepName,
		&reqType,
		&state,
		&req.Title,
		&req.Description,
		&promptsJSON,
		&responseJSON,
		&timeoutNs,
		&expiresAt,
		&metadataJSON,
		&req.CreatedAt,
		&req.UpdatedAt,
		&completedAt,
	)
	if err != nil {
		return nil, fmt.Errorf("scan request: %w", err)
	}

	req.Type = InterventionType(reqType)
	req.State = InterventionState(state)
	req.Timeout = time.Duration(timeoutNs)

	if expiresAt.Valid {
		req.ExpiresAt = &expiresAt.Time
	}
	if completedAt.Valid {
		req.CompletedAt = &completedAt.Time
	}

	if promptsJSON != "" && promptsJSON != "null" {
		if err := json.Unmarshal([]byte(promptsJSON), &req.Prompts); err != nil {
			return nil, fmt.Errorf("unmarshal prompts: %w", err)
		}
	}

	if responseJSON != "" && responseJSON != "null" {
		if err := json.Unmarshal([]byte(responseJSON), &req.Response); err != nil {
			return nil, fmt.Errorf("unmarshal response: %w", err)
		}
	}

	if metadataJSON != "" && metadataJSON != "null" {
		if err := json.Unmarshal([]byte(metadataJSON), &req.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
	}

	return &req, nil
}
