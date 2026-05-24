// SPDX-License-Identifier: Apache-2.0

package outbound

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/pkg/dbutil"
)

// SQLiteStore is the durable [SubscriptionStore] backed by SQLite via
// [dbutil.OpenSQLite] (the project's standard WAL / busy-timeout /
// foreign-key conventions). Schema is literal to §4.14.
//
// # Lossy round-trip note
//
// [DeliveryRecord.Error] is a string already, so deliveries
// round-trip faithfully. [Subscription] has no error fields. Times
// are stored as RFC3339Nano UTC; events and headers as JSON TEXT.
type SQLiteStore struct {
	mu sync.Mutex
	db *sql.DB
}

const sqliteOutboundSchema = `
CREATE TABLE IF NOT EXISTS subscriptions (
	id            TEXT PRIMARY KEY,
	seq           INTEGER NOT NULL,
	name          TEXT NOT NULL,
	url           TEXT NOT NULL,
	secret        TEXT NOT NULL DEFAULT '',
	events_json   TEXT NOT NULL DEFAULT '[]',
	enabled       INTEGER NOT NULL DEFAULT 1,
	headers_json  TEXT NOT NULL DEFAULT '{}',
	max_retries   INTEGER NOT NULL DEFAULT 3,
	timeout_sec   INTEGER NOT NULL DEFAULT 10,
	created_at    TEXT NOT NULL,
	updated_at    TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS deliveries (
	id              TEXT PRIMARY KEY,
	seq             INTEGER NOT NULL,
	subscription_id TEXT NOT NULL,
	event_type      TEXT NOT NULL,
	event_id        TEXT NOT NULL,
	status          TEXT NOT NULL,
	status_code     INTEGER NOT NULL DEFAULT 0,
	attempt         INTEGER NOT NULL DEFAULT 0,
	error           TEXT NOT NULL DEFAULT '',
	delivered_at    TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_subscriptions_seq ON subscriptions(seq);
CREATE INDEX IF NOT EXISTS idx_deliveries_sub   ON deliveries(subscription_id, seq);
`

// NewSQLiteStore opens (or creates) a SQLite-backed
// [SubscriptionStore] at path and applies its schema. Pass
// ":memory:" for an ephemeral DB. The returned store satisfies
// [io.Closer]; the caller owns its lifetime.
func NewSQLiteStore(path string, opts ...dbutil.Option) (*SQLiteStore, error) {
	db, err := dbutil.OpenSQLite(path, opts...)
	if err != nil {
		return nil, fmt.Errorf("outbound: open sqlite: %w", err)
	}
	if _, err := db.ExecContext(context.Background(), sqliteOutboundSchema); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("outbound: apply schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// Close releases the underlying database. Safe on a nil receiver.
func (s *SQLiteStore) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func parseStoreTime(s string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Time{}, err
	}
	return t.UTC(), nil
}

// --- subscriptions -----------------------------------------------------------

// CreateSubscription implements [SubscriptionStore].
func (s *SQLiteStore) CreateSubscription(ctx context.Context, sub *Subscription) error {
	if sub == nil || sub.ID == "" {
		return errors.New("outbound: create: nil subscription or empty id")
	}
	events, headers, err := marshalSubFields(sub)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	const q = `
INSERT INTO subscriptions
	(id, seq, name, url, secret, events_json, enabled, headers_json,
	 max_retries, timeout_sec, created_at, updated_at)
VALUES
	(?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM subscriptions),
	 ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	if _, err := s.db.ExecContext(ctx, q,
		sub.ID, sub.Name, sub.URL, sub.Secret, events, boolToInt(sub.Enabled),
		headers, sub.MaxRetries, sub.TimeoutSec,
		sub.CreatedAt.UTC().Format(time.RFC3339Nano),
		sub.UpdatedAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("outbound: create %q: %w", sub.ID, err)
	}
	return nil
}

// GetSubscription implements [SubscriptionStore].
func (s *SQLiteStore) GetSubscription(ctx context.Context, id string) (*Subscription, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	const q = `
SELECT name, url, secret, events_json, enabled, headers_json,
	max_retries, timeout_sec, created_at, updated_at
FROM subscriptions WHERE id = ?`
	sub, err := scanSubscription(id, s.db.QueryRowContext(ctx, q, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return sub, true, nil
}

// ListSubscriptions implements [SubscriptionStore], oldest first.
func (s *SQLiteStore) ListSubscriptions(ctx context.Context) ([]*Subscription, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	const q = `
SELECT id, name, url, secret, events_json, enabled, headers_json,
	max_retries, timeout_sec, created_at, updated_at
FROM subscriptions ORDER BY seq ASC`
	rows, err := s.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("outbound: list subscriptions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*Subscription
	for rows.Next() {
		var id string
		sub, err := scanSubscription("", func(dest ...any) error {
			return rows.Scan(append([]any{&id}, dest...)...)
		})
		if err != nil {
			return nil, err
		}
		sub.ID = id
		out = append(out, sub)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbound: list subscriptions: %w", err)
	}
	return out, nil
}

// UpdateSubscription implements [SubscriptionStore].
func (s *SQLiteStore) UpdateSubscription(ctx context.Context, sub *Subscription) error {
	if sub == nil || sub.ID == "" {
		return errors.New("outbound: update: nil subscription or empty id")
	}
	events, headers, err := marshalSubFields(sub)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	const q = `
UPDATE subscriptions SET
	name         = ?,
	url          = ?,
	secret       = ?,
	events_json  = ?,
	enabled      = ?,
	headers_json = ?,
	max_retries  = ?,
	timeout_sec  = ?,
	updated_at   = ?
WHERE id = ?`
	res, err := s.db.ExecContext(ctx, q,
		sub.Name, sub.URL, sub.Secret, events, boolToInt(sub.Enabled),
		headers, sub.MaxRetries, sub.TimeoutSec,
		sub.UpdatedAt.UTC().Format(time.RFC3339Nano), sub.ID,
	)
	if err != nil {
		return fmt.Errorf("outbound: update %q: %w", sub.ID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

// DeleteSubscription implements [SubscriptionStore].
func (s *SQLiteStore) DeleteSubscription(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx, `DELETE FROM subscriptions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("outbound: delete %q: %w", id, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrSubscriptionNotFound
	}
	return nil
}

func marshalSubFields(sub *Subscription) (events, headers string, err error) {
	ev := sub.Events
	if ev == nil {
		ev = []string{}
	}
	eb, merr := json.Marshal(ev)
	if merr != nil {
		return "", "", fmt.Errorf("outbound: marshal events: %w", merr)
	}
	hd := sub.Headers
	if hd == nil {
		hd = map[string]string{}
	}
	hb, merr := json.Marshal(hd)
	if merr != nil {
		return "", "", fmt.Errorf("outbound: marshal headers: %w", merr)
	}
	return string(eb), string(hb), nil
}

func scanSubscription(id string, scan func(dest ...any) error) (*Subscription, error) {
	var (
		name, url, secret, eventsJSON, headersJSON string
		enabledInt, maxRetries, timeoutSec         int
		createdAt, updatedAt                       string
	)
	if err := scan(&name, &url, &secret, &eventsJSON, &enabledInt, &headersJSON,
		&maxRetries, &timeoutSec, &createdAt, &updatedAt); err != nil {
		return nil, err
	}
	var events []string
	if err := json.Unmarshal([]byte(eventsJSON), &events); err != nil {
		return nil, fmt.Errorf("outbound: unmarshal events: %w", err)
	}
	if len(events) == 0 {
		events = nil
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(headersJSON), &headers); err != nil {
		return nil, fmt.Errorf("outbound: unmarshal headers: %w", err)
	}
	if len(headers) == 0 {
		headers = nil
	}
	created, err := parseStoreTime(createdAt)
	if err != nil {
		return nil, fmt.Errorf("outbound: parse created_at: %w", err)
	}
	updated, err := parseStoreTime(updatedAt)
	if err != nil {
		return nil, fmt.Errorf("outbound: parse updated_at: %w", err)
	}
	return &Subscription{
		ID:         id,
		Name:       name,
		URL:        url,
		Secret:     secret,
		Events:     events,
		Enabled:    enabledInt != 0,
		Headers:    headers,
		MaxRetries: maxRetries,
		TimeoutSec: timeoutSec,
		CreatedAt:  created,
		UpdatedAt:  updated,
	}, nil
}

// --- deliveries --------------------------------------------------------------

// SaveDelivery implements [SubscriptionStore] (upsert).
func (s *SQLiteStore) SaveDelivery(ctx context.Context, d *DeliveryRecord) error {
	if d == nil || d.ID == "" {
		return errors.New("outbound: save delivery: nil record or empty id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	const q = `
INSERT INTO deliveries
	(id, seq, subscription_id, event_type, event_id, status,
	 status_code, attempt, error, delivered_at)
VALUES
	(?, (SELECT COALESCE(MAX(seq), 0) + 1 FROM deliveries),
	 ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
	subscription_id = excluded.subscription_id,
	event_type      = excluded.event_type,
	event_id        = excluded.event_id,
	status          = excluded.status,
	status_code     = excluded.status_code,
	attempt         = excluded.attempt,
	error           = excluded.error,
	delivered_at    = excluded.delivered_at`
	if _, err := s.db.ExecContext(ctx, q,
		d.ID, d.SubscriptionID, d.EventType, d.EventID, string(d.Status),
		d.StatusCode, d.Attempt, d.Error,
		d.DeliveredAt.UTC().Format(time.RFC3339Nano),
	); err != nil {
		return fmt.Errorf("outbound: save delivery %q: %w", d.ID, err)
	}
	return nil
}

// GetDelivery implements [SubscriptionStore].
func (s *SQLiteStore) GetDelivery(ctx context.Context, id string) (*DeliveryRecord, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	const q = `
SELECT subscription_id, event_type, event_id, status, status_code,
	attempt, error, delivered_at
FROM deliveries WHERE id = ?`
	d, err := scanDelivery(id, s.db.QueryRowContext(ctx, q, id).Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	return d, true, nil
}

// ListDeliveries implements [SubscriptionStore].
func (s *SQLiteStore) ListDeliveries(ctx context.Context, subscriptionID string, limit int) ([]*DeliveryRecord, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var (
		q    string
		args []any
	)
	if subscriptionID != "" {
		q = `
SELECT id, subscription_id, event_type, event_id, status, status_code,
	attempt, error, delivered_at
FROM deliveries WHERE subscription_id = ? ORDER BY seq ASC`
		args = []any{subscriptionID}
	} else {
		q = `
SELECT id, subscription_id, event_type, event_id, status, status_code,
	attempt, error, delivered_at
FROM deliveries ORDER BY seq ASC`
	}
	if limit > 0 {
		q += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("outbound: list deliveries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*DeliveryRecord
	for rows.Next() {
		var id string
		d, err := scanDelivery("", func(dest ...any) error {
			return rows.Scan(append([]any{&id}, dest...)...)
		})
		if err != nil {
			return nil, err
		}
		d.ID = id
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("outbound: list deliveries: %w", err)
	}
	return out, nil
}

// DeleteOldDeliveries implements [SubscriptionStore].
func (s *SQLiteStore) DeleteOldDeliveries(ctx context.Context, before time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM deliveries WHERE delivered_at < ?`,
		before.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return 0, fmt.Errorf("outbound: delete old deliveries: %w", err)
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func scanDelivery(id string, scan func(dest ...any) error) (*DeliveryRecord, error) {
	var (
		subID, eventType, eventID, status, errStr string
		statusCode, attempt                       int
		deliveredAt                               string
	)
	if err := scan(&subID, &eventType, &eventID, &status, &statusCode,
		&attempt, &errStr, &deliveredAt); err != nil {
		return nil, err
	}
	t, err := parseStoreTime(deliveredAt)
	if err != nil {
		return nil, fmt.Errorf("outbound: parse delivered_at: %w", err)
	}
	return &DeliveryRecord{
		ID:             id,
		SubscriptionID: subID,
		EventType:      eventType,
		EventID:        eventID,
		Status:         DeliveryStatus(status),
		StatusCode:     statusCode,
		Attempt:        attempt,
		Error:          errStr,
		DeliveredAt:    t,
	}, nil
}
