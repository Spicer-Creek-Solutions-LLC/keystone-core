package history

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	_ "modernc.org/sqlite" // Register pure-Go SQLite driver
)

// SQLiteStore implements Store using SQLite.
type SQLiteStore struct {
	db *sql.DB
}

// NewSQLiteStore creates a new SQLite-backed state history store.
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

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
	CREATE TABLE IF NOT EXISTS state_runs (
		run_id TEXT PRIMARY KEY,
		agent_id TEXT NOT NULL,
		state_files TEXT,
		target TEXT,
		dry_run INTEGER NOT NULL DEFAULT 0,
		success INTEGER NOT NULL DEFAULT 0,
		total INTEGER NOT NULL DEFAULT 0,
		succeeded INTEGER NOT NULL DEFAULT 0,
		failed INTEGER NOT NULL DEFAULT 0,
		changed INTEGER NOT NULL DEFAULT 0,
		unchanged INTEGER NOT NULL DEFAULT 0,
		start_time TIMESTAMP,
		end_time TIMESTAMP,
		duration_ms INTEGER NOT NULL DEFAULT 0,
		correlation_id TEXT,
		user TEXT
	);

	CREATE INDEX IF NOT EXISTS idx_state_runs_agent ON state_runs(agent_id);
	CREATE INDEX IF NOT EXISTS idx_state_runs_start ON state_runs(start_time);

	CREATE TABLE IF NOT EXISTS state_status (
		agent_id TEXT NOT NULL,
		state_id TEXT NOT NULL,
		module TEXT,
		current_state TEXT,
		desired_state TEXT,
		compliant INTEGER NOT NULL DEFAULT 0,
		last_applied TIMESTAMP,
		last_checked TIMESTAMP,
		PRIMARY KEY (agent_id, state_id)
	);
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// SaveRun persists a state run record.
func (s *SQLiteStore) SaveRun(ctx context.Context, run *StateRunRecord) error {
	filesJSON, err := json.Marshal(run.StateFiles)
	if err != nil {
		return fmt.Errorf("marshal state files: %w", err)
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO state_runs (run_id, agent_id, state_files, target, dry_run, success,
			total, succeeded, failed, changed, unchanged, start_time, end_time,
			duration_ms, correlation_id, user)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.RunID, run.AgentID, string(filesJSON), run.Target,
		boolToInt(run.DryRun), boolToInt(run.Success),
		run.Total, run.Succeeded, run.Failed, run.Changed, run.Unchanged,
		run.StartTime, run.EndTime, run.DurationMs,
		run.CorrelationID, run.User,
	)
	return err
}

// ListRuns returns state runs matching the filter with pagination.
func (s *SQLiteStore) ListRuns(ctx context.Context, filter *ListFilter) ([]*StateRunRecord, string, error) {
	var conditions []string
	var args []interface{}

	if filter != nil {
		if filter.AgentID != "" {
			conditions = append(conditions, "agent_id = ?")
			args = append(args, filter.AgentID)
		}
		if filter.StatePath != "" {
			conditions = append(conditions, "state_files LIKE ?")
			args = append(args, "%"+filter.StatePath+"%")
		}
		if filter.Success != nil {
			conditions = append(conditions, "success = ?")
			args = append(args, boolToInt(*filter.Success))
		}
		if filter.StartTime != nil {
			conditions = append(conditions, "start_time >= ?")
			args = append(args, *filter.StartTime)
		}
		if filter.EndTime != nil {
			conditions = append(conditions, "end_time <= ?")
			args = append(args, *filter.EndTime)
		}
	}

	query := "SELECT run_id, agent_id, state_files, target, dry_run, success, total, succeeded, failed, changed, unchanged, start_time, end_time, duration_ms, correlation_id, user FROM state_runs"
	if len(conditions) > 0 {
		query += " WHERE " + strings.Join(conditions, " AND ")
	}
	query += " ORDER BY start_time DESC"

	pageSize := 50
	offset := 0
	if filter != nil {
		if filter.PageSize > 0 {
			pageSize = filter.PageSize
		}
		if filter.PageToken != "" {
			offset = parsePageToken(filter.PageToken)
		}
	}

	query += fmt.Sprintf(" LIMIT %d OFFSET %d", pageSize+1, offset)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, "", fmt.Errorf("query state runs: %w", err)
	}
	defer rows.Close()

	var runs []*StateRunRecord
	for rows.Next() {
		r := &StateRunRecord{}
		var filesJSON string
		var dryRun, success int
		if err := rows.Scan(&r.RunID, &r.AgentID, &filesJSON, &r.Target, &dryRun, &success,
			&r.Total, &r.Succeeded, &r.Failed, &r.Changed, &r.Unchanged,
			&r.StartTime, &r.EndTime, &r.DurationMs,
			&r.CorrelationID, &r.User); err != nil {
			return nil, "", fmt.Errorf("scan row: %w", err)
		}
		r.DryRun = dryRun != 0
		r.Success = success != 0
		if filesJSON != "" {
			json.Unmarshal([]byte(filesJSON), &r.StateFiles)
		}
		runs = append(runs, r)
	}
	if err := rows.Err(); err != nil {
		return nil, "", fmt.Errorf("iterate rows: %w", err)
	}

	var nextToken string
	if len(runs) > pageSize {
		runs = runs[:pageSize]
		nextToken = encodePageToken(offset + pageSize)
	}

	return runs, nextToken, nil
}

// GetStatus returns current state status records for an agent.
func (s *SQLiteStore) GetStatus(ctx context.Context, agentID, statePath string) ([]*StateStatusRecord, error) {
	query := "SELECT agent_id, state_id, module, current_state, desired_state, compliant, last_applied, last_checked FROM state_status WHERE agent_id = ?"
	args := []interface{}{agentID}

	if statePath != "" {
		query += " AND state_id LIKE ?"
		args = append(args, "%"+statePath+"%")
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query state status: %w", err)
	}
	defer rows.Close()

	var records []*StateStatusRecord
	for rows.Next() {
		r := &StateStatusRecord{}
		var compliant int
		if err := rows.Scan(&r.AgentID, &r.StateID, &r.Module, &r.CurrentState, &r.DesiredState, &compliant, &r.LastApplied, &r.LastChecked); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}
		r.Compliant = compliant != 0
		records = append(records, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return records, nil
}

// SaveStatus upserts a state status record.
func (s *SQLiteStore) SaveStatus(ctx context.Context, status *StateStatusRecord) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO state_status (agent_id, state_id, module, current_state, desired_state, compliant, last_applied, last_checked)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(agent_id, state_id) DO UPDATE SET
			module = excluded.module,
			current_state = excluded.current_state,
			desired_state = excluded.desired_state,
			compliant = excluded.compliant,
			last_applied = excluded.last_applied,
			last_checked = excluded.last_checked`,
		status.AgentID, status.StateID, status.Module, status.CurrentState, status.DesiredState,
		boolToInt(status.Compliant), status.LastApplied, status.LastChecked,
	)
	return err
}

// Close closes the database connection.
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parsePageToken(token string) int {
	data, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return 0
	}
	v, err := strconv.Atoi(string(data))
	if err != nil || v < 0 {
		return 0
	}
	return v
}

func encodePageToken(offset int) string {
	if offset <= 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString([]byte(strconv.Itoa(offset)))
}
