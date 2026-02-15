package saga

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	_ "modernc.org/sqlite" // Register pure-Go SQLite driver
)

// SQLiteLog persists saga executions in a SQLite database.
type SQLiteLog struct {
	db *sql.DB
}

// NewSQLiteLog creates a new SQLite-backed saga log.
func NewSQLiteLog(dbPath string) (*SQLiteLog, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)

	ctx := context.Background()
	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("set WAL mode: %w", err)
	}

	s := &SQLiteLog{db: db}
	if err := s.initSchema(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return s, nil
}

func (s *SQLiteLog) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS saga_executions (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		status TEXT NOT NULL,
		data TEXT,
		steps TEXT,
		started_at TIMESTAMP NOT NULL,
		ended_at TIMESTAMP,
		error TEXT
	);
	CREATE INDEX IF NOT EXISTS idx_saga_name ON saga_executions(name);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Save persists an execution, using ON CONFLICT to upsert.
func (s *SQLiteLog) Save(ctx context.Context, exec *Execution) error {
	dataJSON, err := json.Marshal(exec.Data)
	if err != nil {
		return fmt.Errorf("marshal data: %w", err)
	}

	stepsJSON, err := json.Marshal(exec.Steps)
	if err != nil {
		return fmt.Errorf("marshal steps: %w", err)
	}

	query := `
	INSERT INTO saga_executions (id, name, status, data, steps, started_at, ended_at, error)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	ON CONFLICT(id) DO UPDATE SET
		status = excluded.status,
		data = excluded.data,
		steps = excluded.steps,
		ended_at = excluded.ended_at,
		error = excluded.error
	`

	_, err = s.db.ExecContext(ctx, query,
		exec.ID,
		exec.Name,
		string(exec.Status),
		string(dataJSON),
		string(stepsJSON),
		exec.StartedAt,
		exec.EndedAt,
		exec.Error,
	)
	if err != nil {
		return fmt.Errorf("save execution: %w", err)
	}
	return nil
}

// Get retrieves an execution by ID. Returns nil if not found.
func (s *SQLiteLog) Get(ctx context.Context, id string) (*Execution, error) {
	query := `
	SELECT id, name, status, data, steps, started_at, ended_at, error
	FROM saga_executions
	WHERE id = ?
	`

	var exec Execution
	var dataJSON, stepsJSON string
	var status string
	var endedAt sql.NullTime
	var errStr sql.NullString

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&exec.ID,
		&exec.Name,
		&status,
		&dataJSON,
		&stepsJSON,
		&exec.StartedAt,
		&endedAt,
		&errStr,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get execution: %w", err)
	}

	exec.Status = Status(status)
	if endedAt.Valid {
		exec.EndedAt = &endedAt.Time
	}
	if errStr.Valid {
		exec.Error = errStr.String
	}

	if dataJSON != "" {
		if err := json.Unmarshal([]byte(dataJSON), &exec.Data); err != nil {
			return nil, fmt.Errorf("unmarshal data: %w", err)
		}
	}

	if stepsJSON != "" {
		if err := json.Unmarshal([]byte(stepsJSON), &exec.Steps); err != nil {
			return nil, fmt.Errorf("unmarshal steps: %w", err)
		}
	}

	return &exec, nil
}

// List returns executions filtered by name with an optional limit.
func (s *SQLiteLog) List(ctx context.Context, name string, limit int) ([]*Execution, error) {
	query := `SELECT id, name, status, data, steps, started_at, ended_at, error FROM saga_executions`
	var args []any

	if name != "" {
		query += " WHERE name = ?"
		args = append(args, name)
	}

	query += " ORDER BY started_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list executions: %w", err)
	}
	defer rows.Close()

	var result []*Execution
	for rows.Next() {
		var exec Execution
		var dataJSON, stepsJSON string
		var status string
		var endedAt sql.NullTime
		var errStr sql.NullString

		if err := rows.Scan(
			&exec.ID,
			&exec.Name,
			&status,
			&dataJSON,
			&stepsJSON,
			&exec.StartedAt,
			&endedAt,
			&errStr,
		); err != nil {
			return nil, fmt.Errorf("scan execution: %w", err)
		}

		exec.Status = Status(status)
		if endedAt.Valid {
			exec.EndedAt = &endedAt.Time
		}
		if errStr.Valid {
			exec.Error = errStr.String
		}

		if dataJSON != "" {
			if err := json.Unmarshal([]byte(dataJSON), &exec.Data); err != nil {
				return nil, fmt.Errorf("unmarshal data: %w", err)
			}
		}
		if stepsJSON != "" {
			if err := json.Unmarshal([]byte(stepsJSON), &exec.Steps); err != nil {
				return nil, fmt.Errorf("unmarshal steps: %w", err)
			}
		}

		result = append(result, &exec)
	}

	return result, rows.Err()
}

// Close closes the database connection.
func (s *SQLiteLog) Close() error {
	return s.db.Close()
}
