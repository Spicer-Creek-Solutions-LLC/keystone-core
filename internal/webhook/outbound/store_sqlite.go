package outbound

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"time"
)

// SQLiteStore implements SubscriptionStore using SQLite.
type SQLiteStore struct {
	db *sql.DB
	mu sync.Mutex
}

// NewSQLiteStore creates a new SQLite-backed subscription store.
// The caller is responsible for opening the database and setting pragmas (e.g., WAL mode).
func NewSQLiteStore(db *sql.DB) (*SQLiteStore, error) {
	if db == nil {
		return nil, fmt.Errorf("database connection required")
	}
	s := &SQLiteStore{db: db}
	if err := s.initSchema(); err != nil {
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}
	return s, nil
}

func (s *SQLiteStore) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS outbound_subscriptions (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			url TEXT NOT NULL,
			secret TEXT NOT NULL DEFAULT '',
			events TEXT NOT NULL DEFAULT '[]',
			enabled INTEGER NOT NULL DEFAULT 1,
			headers TEXT NOT NULL DEFAULT '{}',
			max_retries INTEGER NOT NULL DEFAULT 3,
			timeout_sec INTEGER NOT NULL DEFAULT 10,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_obs_enabled ON outbound_subscriptions(enabled);

		CREATE TABLE IF NOT EXISTS outbound_deliveries (
			id TEXT PRIMARY KEY,
			subscription_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			event_id TEXT NOT NULL,
			status TEXT NOT NULL,
			status_code INTEGER NOT NULL DEFAULT 0,
			attempt INTEGER NOT NULL DEFAULT 1,
			error TEXT NOT NULL DEFAULT '',
			delivered_at DATETIME NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_obd_sub ON outbound_deliveries(subscription_id);
		CREATE INDEX IF NOT EXISTS idx_obd_delivered ON outbound_deliveries(delivered_at);
	`
	_, err := s.db.ExecContext(context.Background(), schema)
	return err
}

// Create inserts a new subscription into the store.
func (s *SQLiteStore) Create(ctx context.Context, sub *Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	eventsJSON, err := json.Marshal(sub.Events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	headersJSON, err := json.Marshal(sub.Headers)
	if err != nil {
		return fmt.Errorf("marshal headers: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO outbound_subscriptions (id, name, url, secret, events, enabled, headers, max_retries, timeout_sec, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sub.ID, sub.Name, sub.URL, sub.Secret,
		string(eventsJSON), boolToInt(sub.Enabled), string(headersJSON),
		sub.MaxRetries, sub.TimeoutSec,
		sub.CreatedAt.UTC(), sub.UpdatedAt.UTC(),
	)
	return err
}

// Get retrieves a subscription by ID.
func (s *SQLiteStore) Get(ctx context.Context, id string) (*Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	row := s.db.QueryRowContext(ctx,
		`SELECT id, name, url, secret, events, enabled, headers, max_retries, timeout_sec, created_at, updated_at
		 FROM outbound_subscriptions WHERE id = ?`, id)
	return scanSubscription(row)
}

// List returns all subscriptions ordered by creation time.
func (s *SQLiteStore) List(ctx context.Context) ([]*Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, secret, events, enabled, headers, max_retries, timeout_sec, created_at, updated_at
		 FROM outbound_subscriptions ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

// Update modifies an existing subscription.
func (s *SQLiteStore) Update(ctx context.Context, sub *Subscription) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	eventsJSON, err := json.Marshal(sub.Events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	headersJSON, err := json.Marshal(sub.Headers)
	if err != nil {
		return fmt.Errorf("marshal headers: %w", err)
	}

	result, err := s.db.ExecContext(ctx,
		`UPDATE outbound_subscriptions
		 SET name = ?, url = ?, secret = ?, events = ?, enabled = ?, headers = ?,
		     max_retries = ?, timeout_sec = ?, updated_at = ?
		 WHERE id = ?`,
		sub.Name, sub.URL, sub.Secret,
		string(eventsJSON), boolToInt(sub.Enabled), string(headersJSON),
		sub.MaxRetries, sub.TimeoutSec,
		sub.UpdatedAt.UTC(), sub.ID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("subscription not found: %s", sub.ID)
	}
	return nil
}

// Delete removes a subscription and its delivery records.
func (s *SQLiteStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	result, err := s.db.ExecContext(ctx,
		`DELETE FROM outbound_subscriptions WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("subscription not found: %s", id)
	}
	// Clean up delivery records for deleted subscription.
	_, _ = s.db.ExecContext(ctx,
		`DELETE FROM outbound_deliveries WHERE subscription_id = ?`, id)
	return nil
}

// ListEnabled returns all enabled subscriptions.
func (s *SQLiteStore) ListEnabled(ctx context.Context) ([]*Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, name, url, secret, events, enabled, headers, max_retries, timeout_sec, created_at, updated_at
		 FROM outbound_subscriptions WHERE enabled = 1 ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanSubscriptions(rows)
}

// RecordDelivery inserts a delivery record.
func (s *SQLiteStore) RecordDelivery(ctx context.Context, record *DeliveryRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO outbound_deliveries (id, subscription_id, event_type, event_id, status, status_code, attempt, error, delivered_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		record.ID, record.SubscriptionID, record.EventType, record.EventID,
		string(record.Status), record.StatusCode, record.Attempt,
		record.Error, record.DeliveredAt.UTC(),
	)
	return err
}

// ListDeliveries returns delivery records for a subscription, ordered by most recent first.
func (s *SQLiteStore) ListDeliveries(ctx context.Context, subscriptionID string, limit int) ([]*DeliveryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, subscription_id, event_type, event_id, status, status_code, attempt, error, delivered_at
		 FROM outbound_deliveries WHERE subscription_id = ?
		 ORDER BY delivered_at DESC LIMIT ?`,
		subscriptionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*DeliveryRecord
	for rows.Next() {
		r := &DeliveryRecord{}
		var status string
		err := rows.Scan(&r.ID, &r.SubscriptionID, &r.EventType, &r.EventID,
			&status, &r.StatusCode, &r.Attempt, &r.Error, &r.DeliveredAt)
		if err != nil {
			return nil, err
		}
		r.Status = DeliveryStatus(status)
		records = append(records, r)
	}
	return records, rows.Err()
}

// DeleteOldDeliveries removes delivery records older than the specified duration.
func (s *SQLiteStore) DeleteOldDeliveries(ctx context.Context, olderThan time.Duration) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cutoff := time.Now().Add(-olderThan).UTC()
	result, err := s.db.ExecContext(ctx,
		`DELETE FROM outbound_deliveries WHERE delivered_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// scanSubscription scans a single row into a Subscription.
func scanSubscription(row *sql.Row) (*Subscription, error) {
	sub := &Subscription{}
	var eventsJSON, headersJSON string
	var enabled int
	err := row.Scan(&sub.ID, &sub.Name, &sub.URL, &sub.Secret,
		&eventsJSON, &enabled, &headersJSON,
		&sub.MaxRetries, &sub.TimeoutSec,
		&sub.CreatedAt, &sub.UpdatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("subscription not found")
		}
		return nil, err
	}
	sub.Enabled = enabled != 0
	if err := json.Unmarshal([]byte(eventsJSON), &sub.Events); err != nil {
		return nil, fmt.Errorf("unmarshal events: %w", err)
	}
	if err := json.Unmarshal([]byte(headersJSON), &sub.Headers); err != nil {
		return nil, fmt.Errorf("unmarshal headers: %w", err)
	}
	return sub, nil
}

// scanSubscriptions scans multiple rows into Subscription slices.
func scanSubscriptions(rows *sql.Rows) ([]*Subscription, error) {
	var subs []*Subscription
	for rows.Next() {
		sub := &Subscription{}
		var eventsJSON, headersJSON string
		var enabled int
		err := rows.Scan(&sub.ID, &sub.Name, &sub.URL, &sub.Secret,
			&eventsJSON, &enabled, &headersJSON,
			&sub.MaxRetries, &sub.TimeoutSec,
			&sub.CreatedAt, &sub.UpdatedAt)
		if err != nil {
			return nil, err
		}
		sub.Enabled = enabled != 0
		if err := json.Unmarshal([]byte(eventsJSON), &sub.Events); err != nil {
			return nil, fmt.Errorf("unmarshal events: %w", err)
		}
		if err := json.Unmarshal([]byte(headersJSON), &sub.Headers); err != nil {
			return nil, fmt.Errorf("unmarshal headers: %w", err)
		}
		subs = append(subs, sub)
	}
	return subs, rows.Err()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
