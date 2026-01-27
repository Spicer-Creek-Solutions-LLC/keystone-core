package secrets

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite" // SQLite driver
)

// LeaseStore provides persistent storage for leases.
type LeaseStore interface {
	// Initialize creates the database schema.
	Initialize(ctx context.Context) error

	// Create creates a new lease.
	Create(ctx context.Context, lease *Lease) error

	// Get retrieves a lease by ID.
	Get(ctx context.Context, leaseID string) (*Lease, error)

	// Update updates an existing lease.
	Update(ctx context.Context, lease *Lease) error

	// Delete deletes a lease.
	Delete(ctx context.Context, leaseID string) error

	// List lists leases with optional filtering.
	List(ctx context.Context, filter *LeaseFilter) ([]*Lease, error)

	// Count counts leases matching the filter.
	Count(ctx context.Context, filter *LeaseFilter) (int64, error)

	// DeleteByFilter deletes leases matching the filter.
	DeleteByFilter(ctx context.Context, filter *LeaseFilter) (int64, error)

	// UpdateState updates the state of multiple leases.
	UpdateState(ctx context.Context, filter *LeaseFilter, state LeaseState) (int64, error)

	// Close closes the store.
	Close() error
}

// LeaseFilter specifies criteria for filtering leases.
type LeaseFilter struct {
	// IDs filters by lease IDs.
	IDs []string

	// SecretPath filters by exact secret path.
	SecretPath string

	// PathPrefix filters by path prefix.
	PathPrefix string

	// Backend filters by backend type.
	Backend BackendType

	// State filters by lease state.
	State LeaseState

	// States filters by multiple states.
	States []LeaseState

	// AgentID filters by agent ID.
	AgentID string

	// ExpiringBefore filters for leases expiring before this time.
	ExpiringBefore time.Time

	// ExpiringAfter filters for leases expiring after this time.
	ExpiringAfter time.Time

	// Renewable filters for renewable leases.
	Renewable *bool

	// Limit limits the number of results.
	Limit int

	// Offset skips the first N results.
	Offset int

	// OrderBy specifies the sort order.
	OrderBy string

	// OrderDesc reverses the sort order.
	OrderDesc bool
}

// SQLiteLeaseStore provides SQLite-based lease storage.
type SQLiteLeaseStore struct {
	db *sql.DB
	mu sync.RWMutex
}

// NewSQLiteLeaseStore creates a new SQLite lease store.
func NewSQLiteLeaseStore(dsn string) (*SQLiteLeaseStore, error) {
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(1) // SQLite only supports one writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to enable foreign keys: %w", err)
	}

	return &SQLiteLeaseStore{db: db}, nil
}

// Initialize creates the database schema.
func (s *SQLiteLeaseStore) Initialize(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	schema := `
	-- Leases table
	CREATE TABLE IF NOT EXISTS leases (
		id TEXT PRIMARY KEY,
		secret_path TEXT NOT NULL,
		backend TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'active',
		ttl_seconds INTEGER NOT NULL,
		max_ttl_seconds INTEGER,
		issued_at TEXT NOT NULL,
		expires_at TEXT NOT NULL,
		last_renewal TEXT,
		renewal_count INTEGER NOT NULL DEFAULT 0,
		renewable INTEGER NOT NULL DEFAULT 0,
		revocable INTEGER NOT NULL DEFAULT 1,
		agent_id TEXT,
		spiffe_id TEXT,
		metadata TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now')),
		updated_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	-- Indexes for efficient queries
	CREATE INDEX IF NOT EXISTS idx_leases_secret_path ON leases(secret_path);
	CREATE INDEX IF NOT EXISTS idx_leases_backend ON leases(backend);
	CREATE INDEX IF NOT EXISTS idx_leases_state ON leases(state);
	CREATE INDEX IF NOT EXISTS idx_leases_expires_at ON leases(expires_at);
	CREATE INDEX IF NOT EXISTS idx_leases_agent_id ON leases(agent_id);
	CREATE INDEX IF NOT EXISTS idx_leases_state_expires ON leases(state, expires_at);

	-- Trigger to update updated_at on modification
	CREATE TRIGGER IF NOT EXISTS leases_updated_at
	AFTER UPDATE ON leases
	FOR EACH ROW
	BEGIN
		UPDATE leases SET updated_at = datetime('now') WHERE id = NEW.id;
	END;

	-- Lease events table for audit history
	CREATE TABLE IF NOT EXISTS lease_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		lease_id TEXT NOT NULL,
		event_type TEXT NOT NULL,
		old_state TEXT,
		new_state TEXT,
		error TEXT,
		metadata TEXT,
		created_at TEXT NOT NULL DEFAULT (datetime('now'))
	);

	CREATE INDEX IF NOT EXISTS idx_lease_events_lease_id ON lease_events(lease_id);
	CREATE INDEX IF NOT EXISTS idx_lease_events_created_at ON lease_events(created_at);
	`

	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("failed to create schema: %w", err)
	}

	return nil
}

// Create creates a new lease.
func (s *SQLiteLeaseStore) Create(ctx context.Context, lease *Lease) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	metadata, err := json.Marshal(lease.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
	INSERT INTO leases (
		id, secret_path, backend, state, ttl_seconds, max_ttl_seconds,
		issued_at, expires_at, last_renewal, renewal_count,
		renewable, revocable, agent_id, metadata
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	var lastRenewal *string
	if !lease.LastRenewal.IsZero() {
		t := lease.LastRenewal.Format(time.RFC3339)
		lastRenewal = &t
	}

	var maxTTL *int64
	if lease.MaxTTL > 0 {
		v := int64(lease.MaxTTL.Seconds())
		maxTTL = &v
	}

	_, err = s.db.ExecContext(ctx, query,
		lease.ID,
		lease.SecretPath,
		string(lease.Backend),
		string(lease.State),
		int64(lease.TTL.Seconds()),
		maxTTL,
		lease.IssuedAt.Format(time.RFC3339),
		lease.ExpiresAt.Format(time.RFC3339),
		lastRenewal,
		lease.RenewalCount,
		boolToInt(lease.Renewable),
		boolToInt(lease.Revocable),
		getAgentID(lease),
		string(metadata),
	)

	if err != nil {
		return fmt.Errorf("failed to create lease: %w", err)
	}

	return nil
}

// Get retrieves a lease by ID.
func (s *SQLiteLeaseStore) Get(ctx context.Context, leaseID string) (*Lease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, secret_path, backend, state, ttl_seconds, max_ttl_seconds,
		issued_at, expires_at, last_renewal, renewal_count,
		renewable, revocable, agent_id, metadata
	FROM leases WHERE id = ?
	`

	row := s.db.QueryRowContext(ctx, query, leaseID)
	return s.scanLease(row)
}

// Update updates an existing lease.
func (s *SQLiteLeaseStore) Update(ctx context.Context, lease *Lease) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	metadata, err := json.Marshal(lease.Metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	query := `
	UPDATE leases SET
		secret_path = ?,
		backend = ?,
		state = ?,
		ttl_seconds = ?,
		max_ttl_seconds = ?,
		expires_at = ?,
		last_renewal = ?,
		renewal_count = ?,
		renewable = ?,
		revocable = ?,
		agent_id = ?,
		metadata = ?
	WHERE id = ?
	`

	var lastRenewal *string
	if !lease.LastRenewal.IsZero() {
		t := lease.LastRenewal.Format(time.RFC3339)
		lastRenewal = &t
	}

	var maxTTL *int64
	if lease.MaxTTL > 0 {
		v := int64(lease.MaxTTL.Seconds())
		maxTTL = &v
	}

	result, err := s.db.ExecContext(ctx, query,
		lease.SecretPath,
		string(lease.Backend),
		string(lease.State),
		int64(lease.TTL.Seconds()),
		maxTTL,
		lease.ExpiresAt.Format(time.RFC3339),
		lastRenewal,
		lease.RenewalCount,
		boolToInt(lease.Renewable),
		boolToInt(lease.Revocable),
		getAgentID(lease),
		string(metadata),
		lease.ID,
	)

	if err != nil {
		return fmt.Errorf("failed to update lease: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return ErrLeaseNotFound
	}

	return nil
}

// Delete deletes a lease.
func (s *SQLiteLeaseStore) Delete(ctx context.Context, leaseID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx, "DELETE FROM leases WHERE id = ?", leaseID)
	if err != nil {
		return fmt.Errorf("failed to delete lease: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get affected rows: %w", err)
	}

	if rows == 0 {
		return ErrLeaseNotFound
	}

	return nil
}

// List lists leases with optional filtering.
func (s *SQLiteLeaseStore) List(ctx context.Context, filter *LeaseFilter) ([]*Lease, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query, args := s.buildListQuery(filter)
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list leases: %w", err)
	}
	defer rows.Close()

	var leases []*Lease
	for rows.Next() {
		lease, err := s.scanLeaseRow(rows)
		if err != nil {
			return nil, err
		}
		leases = append(leases, lease)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating rows: %w", err)
	}

	return leases, nil
}

// Count counts leases matching the filter.
func (s *SQLiteLeaseStore) Count(ctx context.Context, filter *LeaseFilter) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	where, args := s.buildWhereClause(filter)
	query := "SELECT COUNT(*) FROM leases" + where

	var count int64
	err := s.db.QueryRowContext(ctx, query, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count leases: %w", err)
	}

	return count, nil
}

// DeleteByFilter deletes leases matching the filter.
func (s *SQLiteLeaseStore) DeleteByFilter(ctx context.Context, filter *LeaseFilter) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	where, args := s.buildWhereClause(filter)
	query := "DELETE FROM leases" + where

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to delete leases: %w", err)
	}

	return result.RowsAffected()
}

// UpdateState updates the state of multiple leases.
func (s *SQLiteLeaseStore) UpdateState(ctx context.Context, filter *LeaseFilter, state LeaseState) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	where, args := s.buildWhereClause(filter)
	query := "UPDATE leases SET state = ?" + where
	args = append([]interface{}{string(state)}, args...)

	result, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return 0, fmt.Errorf("failed to update lease states: %w", err)
	}

	return result.RowsAffected()
}

// Close closes the store.
func (s *SQLiteLeaseStore) Close() error {
	return s.db.Close()
}

// buildListQuery builds a SELECT query from a filter.
func (s *SQLiteLeaseStore) buildListQuery(filter *LeaseFilter) (string, []interface{}) {
	query := `
	SELECT id, secret_path, backend, state, ttl_seconds, max_ttl_seconds,
		issued_at, expires_at, last_renewal, renewal_count,
		renewable, revocable, agent_id, metadata
	FROM leases
	`

	where, args := s.buildWhereClause(filter)
	query += where

	// Order by
	if filter != nil && filter.OrderBy != "" {
		query += " ORDER BY " + filter.OrderBy
		if filter.OrderDesc {
			query += " DESC"
		}
	} else {
		query += " ORDER BY expires_at ASC"
	}

	// Limit and offset
	if filter != nil && filter.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", filter.Limit)
		if filter.Offset > 0 {
			query += fmt.Sprintf(" OFFSET %d", filter.Offset)
		}
	}

	return query, args
}

// buildWhereClause builds a WHERE clause from a filter.
func (s *SQLiteLeaseStore) buildWhereClause(filter *LeaseFilter) (string, []interface{}) {
	if filter == nil {
		return "", nil
	}

	var conditions []string
	var args []interface{}

	if len(filter.IDs) > 0 {
		placeholders := make([]string, len(filter.IDs))
		for i, id := range filter.IDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		conditions = append(conditions, "id IN ("+strings.Join(placeholders, ", ")+")")
	}

	if filter.SecretPath != "" {
		conditions = append(conditions, "secret_path = ?")
		args = append(args, filter.SecretPath)
	}

	if filter.PathPrefix != "" {
		conditions = append(conditions, "secret_path LIKE ?")
		args = append(args, filter.PathPrefix+"%")
	}

	if filter.Backend != "" {
		conditions = append(conditions, "backend = ?")
		args = append(args, string(filter.Backend))
	}

	if filter.State != "" {
		conditions = append(conditions, "state = ?")
		args = append(args, string(filter.State))
	}

	if len(filter.States) > 0 {
		placeholders := make([]string, len(filter.States))
		for i, state := range filter.States {
			placeholders[i] = "?"
			args = append(args, string(state))
		}
		conditions = append(conditions, "state IN ("+strings.Join(placeholders, ", ")+")")
	}

	if filter.AgentID != "" {
		conditions = append(conditions, "agent_id = ?")
		args = append(args, filter.AgentID)
	}

	if !filter.ExpiringBefore.IsZero() {
		conditions = append(conditions, "expires_at < ?")
		args = append(args, filter.ExpiringBefore.Format(time.RFC3339))
	}

	if !filter.ExpiringAfter.IsZero() {
		conditions = append(conditions, "expires_at > ?")
		args = append(args, filter.ExpiringAfter.Format(time.RFC3339))
	}

	if filter.Renewable != nil {
		conditions = append(conditions, "renewable = ?")
		args = append(args, boolToInt(*filter.Renewable))
	}

	if len(conditions) == 0 {
		return "", nil
	}

	return " WHERE " + strings.Join(conditions, " AND "), args
}

// scanLease scans a single lease from a row.
func (s *SQLiteLeaseStore) scanLease(row *sql.Row) (*Lease, error) {
	var lease Lease
	var ttlSeconds int64
	var maxTTLSeconds sql.NullInt64
	var issuedAt, expiresAt string
	var lastRenewal sql.NullString
	var renewable, revocable int
	var agentID sql.NullString
	var metadataJSON string

	err := row.Scan(
		&lease.ID,
		&lease.SecretPath,
		&lease.Backend,
		&lease.State,
		&ttlSeconds,
		&maxTTLSeconds,
		&issuedAt,
		&expiresAt,
		&lastRenewal,
		&lease.RenewalCount,
		&renewable,
		&revocable,
		&agentID,
		&metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, ErrLeaseNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan lease: %w", err)
	}

	return s.populateLease(&lease, ttlSeconds, maxTTLSeconds, issuedAt, expiresAt, lastRenewal, renewable, revocable, agentID, metadataJSON)
}

// scanLeaseRow scans a lease from a rows result.
func (s *SQLiteLeaseStore) scanLeaseRow(rows *sql.Rows) (*Lease, error) {
	var lease Lease
	var ttlSeconds int64
	var maxTTLSeconds sql.NullInt64
	var issuedAt, expiresAt string
	var lastRenewal sql.NullString
	var renewable, revocable int
	var agentID sql.NullString
	var metadataJSON string

	err := rows.Scan(
		&lease.ID,
		&lease.SecretPath,
		&lease.Backend,
		&lease.State,
		&ttlSeconds,
		&maxTTLSeconds,
		&issuedAt,
		&expiresAt,
		&lastRenewal,
		&lease.RenewalCount,
		&renewable,
		&revocable,
		&agentID,
		&metadataJSON,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to scan lease: %w", err)
	}

	return s.populateLease(&lease, ttlSeconds, maxTTLSeconds, issuedAt, expiresAt, lastRenewal, renewable, revocable, agentID, metadataJSON)
}

// populateLease populates lease fields from scanned values.
func (s *SQLiteLeaseStore) populateLease(lease *Lease, ttlSeconds int64, maxTTLSeconds sql.NullInt64, issuedAt, expiresAt string, lastRenewal sql.NullString, renewable, revocable int, agentID sql.NullString, metadataJSON string) (*Lease, error) {
	lease.TTL = time.Duration(ttlSeconds) * time.Second

	if maxTTLSeconds.Valid {
		lease.MaxTTL = time.Duration(maxTTLSeconds.Int64) * time.Second
	}

	var err error
	lease.IssuedAt, err = time.Parse(time.RFC3339, issuedAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse issued_at: %w", err)
	}

	lease.ExpiresAt, err = time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return nil, fmt.Errorf("failed to parse expires_at: %w", err)
	}

	if lastRenewal.Valid {
		lease.LastRenewal, err = time.Parse(time.RFC3339, lastRenewal.String)
		if err != nil {
			return nil, fmt.Errorf("failed to parse last_renewal: %w", err)
		}
	}

	lease.Renewable = renewable == 1
	lease.Revocable = revocable == 1

	if agentID.Valid {
		if lease.Metadata == nil {
			lease.Metadata = make(map[string]string)
		}
		lease.Metadata["agent_id"] = agentID.String
	}

	if metadataJSON != "" && metadataJSON != "{}" {
		if err := json.Unmarshal([]byte(metadataJSON), &lease.Metadata); err != nil {
			return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
		}
	}

	return lease, nil
}

// Helper functions

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func getAgentID(lease *Lease) *string {
	if lease.Metadata == nil {
		return nil
	}
	if agentID, ok := lease.Metadata["agent_id"]; ok {
		return &agentID
	}
	return nil
}

// RecordLeaseEvent records a lease event in the audit table.
func (s *SQLiteLeaseStore) RecordLeaseEvent(ctx context.Context, leaseID, eventType string, oldState, newState *LeaseState, err error, metadata map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	var oldStateStr, newStateStr, errorStr *string
	if oldState != nil {
		v := string(*oldState)
		oldStateStr = &v
	}
	if newState != nil {
		v := string(*newState)
		newStateStr = &v
	}
	if err != nil {
		v := err.Error()
		errorStr = &v
	}

	var metadataJSON *string
	if metadata != nil {
		data, _ := json.Marshal(metadata)
		v := string(data)
		metadataJSON = &v
	}

	query := `
	INSERT INTO lease_events (lease_id, event_type, old_state, new_state, error, metadata)
	VALUES (?, ?, ?, ?, ?, ?)
	`

	_, dbErr := s.db.ExecContext(ctx, query, leaseID, eventType, oldStateStr, newStateStr, errorStr, metadataJSON)
	if dbErr != nil {
		return fmt.Errorf("failed to record lease event: %w", dbErr)
	}

	return nil
}

// GetLeaseEvents retrieves events for a lease.
func (s *SQLiteLeaseStore) GetLeaseEvents(ctx context.Context, leaseID string, limit int) ([]*LeaseEvent, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
	SELECT id, lease_id, event_type, old_state, new_state, error, metadata, created_at
	FROM lease_events
	WHERE lease_id = ?
	ORDER BY created_at DESC
	`

	if limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := s.db.QueryContext(ctx, query, leaseID)
	if err != nil {
		return nil, fmt.Errorf("failed to get lease events: %w", err)
	}
	defer rows.Close()

	var events []*LeaseEvent
	for rows.Next() {
		var event LeaseEvent
		var oldState, newState, errorStr, metadataJSON sql.NullString
		var createdAt string

		err := rows.Scan(
			&event.ID,
			&event.LeaseID,
			&event.EventType,
			&oldState,
			&newState,
			&errorStr,
			&metadataJSON,
			&createdAt,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan lease event: %w", err)
		}

		if oldState.Valid {
			s := LeaseState(oldState.String)
			event.OldState = &s
		}
		if newState.Valid {
			s := LeaseState(newState.String)
			event.NewState = &s
		}
		if errorStr.Valid {
			event.Error = errorStr.String
		}
		if metadataJSON.Valid && metadataJSON.String != "" {
			_ = json.Unmarshal([]byte(metadataJSON.String), &event.Metadata)
		}

		event.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		events = append(events, &event)
	}

	return events, nil
}

// LeaseEvent represents a lease lifecycle event.
type LeaseEvent struct {
	ID        int64             `json:"id"`
	LeaseID   string            `json:"lease_id"`
	EventType string            `json:"event_type"`
	OldState  *LeaseState       `json:"old_state,omitempty"`
	NewState  *LeaseState       `json:"new_state,omitempty"`
	Error     string            `json:"error,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	CreatedAt time.Time         `json:"created_at"`
}

// Ensure SQLiteLeaseStore implements LeaseStore.
var _ LeaseStore = (*SQLiteLeaseStore)(nil)
