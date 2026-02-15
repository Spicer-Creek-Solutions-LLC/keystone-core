package checkpoint

import (
	"context"
	"database/sql"
	"fmt"

	_ "modernc.org/sqlite" // Register pure-Go SQLite driver
)

// SQLiteStore persists machine checkpoints in a SQLite database.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed checkpoint store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
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

	s := &SQLiteStore{db: db}
	if err := s.initSchema(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return s, nil
}

func (s *SQLiteStore) initSchema(ctx context.Context) error {
	schema := `
	CREATE TABLE IF NOT EXISTS machine_checkpoints (
		machine_id TEXT PRIMARY KEY,
		machine_name TEXT NOT NULL,
		state TEXT NOT NULL,
		metadata TEXT,
		updated_at TIMESTAMP NOT NULL
	);
	CREATE INDEX IF NOT EXISTS idx_checkpoint_name ON machine_checkpoints(machine_name);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// Save persists a checkpoint, using ON CONFLICT to upsert.
func (s *SQLiteStore) Save(ctx context.Context, r *Record) error {
	meta, err := MarshalMetadata(r.Metadata)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	query := `
	INSERT INTO machine_checkpoints (machine_id, machine_name, state, metadata, updated_at)
	VALUES (?, ?, ?, ?, ?)
	ON CONFLICT(machine_id) DO UPDATE SET
		state = excluded.state,
		metadata = excluded.metadata,
		updated_at = excluded.updated_at
	`

	_, err = s.db.ExecContext(ctx, query, r.MachineID, r.MachineName, r.State, meta, r.UpdatedAt)
	if err != nil {
		return fmt.Errorf("save checkpoint: %w", err)
	}
	return nil
}

// Load retrieves a checkpoint by machine ID. Returns nil if not found.
func (s *SQLiteStore) Load(ctx context.Context, machineID string) (*Record, error) {
	query := `
	SELECT machine_id, machine_name, state, metadata, updated_at
	FROM machine_checkpoints
	WHERE machine_id = ?
	`

	var r Record
	var metaStr sql.NullString

	err := s.db.QueryRowContext(ctx, query, machineID).Scan(
		&r.MachineID, &r.MachineName, &r.State, &metaStr, &r.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load checkpoint: %w", err)
	}

	if metaStr.Valid && metaStr.String != "" {
		m, err := UnmarshalMetadata(metaStr.String)
		if err != nil {
			return nil, fmt.Errorf("unmarshal metadata: %w", err)
		}
		r.Metadata = m
	}

	return &r, nil
}

// Delete removes a checkpoint by machine ID.
func (s *SQLiteStore) Delete(ctx context.Context, machineID string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM machine_checkpoints WHERE machine_id = ?", machineID)
	return err
}

// List returns checkpoints filtered by machine name.
func (s *SQLiteStore) List(ctx context.Context, machineName string) ([]*Record, error) {
	query := `SELECT machine_id, machine_name, state, metadata, updated_at FROM machine_checkpoints`
	var args []any

	if machineName != "" {
		query += " WHERE machine_name = ?"
		args = append(args, machineName)
	}
	query += " ORDER BY updated_at DESC"

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list checkpoints: %w", err)
	}
	defer rows.Close()

	var result []*Record
	for rows.Next() {
		var r Record
		var metaStr sql.NullString

		if err := rows.Scan(&r.MachineID, &r.MachineName, &r.State, &metaStr, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan checkpoint: %w", err)
		}

		if metaStr.Valid && metaStr.String != "" {
			m, err := UnmarshalMetadata(metaStr.String)
			if err != nil {
				return nil, fmt.Errorf("unmarshal metadata: %w", err)
			}
			r.Metadata = m
		}

		result = append(result, &r)
	}

	return result, rows.Err()
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}
