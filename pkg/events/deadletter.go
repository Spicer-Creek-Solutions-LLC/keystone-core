package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// DeadLetterEntry represents a failed reactor execution stored in the dead letter queue
type DeadLetterEntry struct {
	// ID is the unique identifier for this entry
	ID string `json:"id"`

	// ReactorID is the ID of the reactor that failed
	ReactorID string `json:"reactor_id"`

	// ReactorName is the name of the reactor
	ReactorName string `json:"reactor_name"`

	// Event is the original event that triggered the reactor
	Event *Event `json:"event"`

	// ActionIndex is the index of the action that failed (or -1 if reactor-level failure)
	ActionIndex int `json:"action_index"`

	// ActionName is the name of the action that failed
	ActionName string `json:"action_name"`

	// Error is the error message
	Error string `json:"error"`

	// RetryCount is the number of times this entry has been retried
	RetryCount int `json:"retry_count"`

	// MaxRetries is the maximum number of retries allowed
	MaxRetries int `json:"max_retries"`

	// CreatedAt is when this entry was created
	CreatedAt time.Time `json:"created_at"`

	// LastRetryAt is when this entry was last retried
	LastRetryAt *time.Time `json:"last_retry_at,omitempty"`

	// NextRetryAt is when this entry should be retried next
	NextRetryAt *time.Time `json:"next_retry_at,omitempty"`

	// Status is the current status of this entry
	Status DeadLetterStatus `json:"status"`

	// Metadata contains additional context
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// DeadLetterStatus represents the status of a dead letter entry
type DeadLetterStatus string

const (
	// DeadLetterStatusPending indicates the entry is waiting to be retried or processed
	DeadLetterStatusPending DeadLetterStatus = "pending"

	// DeadLetterStatusRetrying indicates the entry is currently being retried
	DeadLetterStatusRetrying DeadLetterStatus = "retrying"

	// DeadLetterStatusFailed indicates the entry has exceeded max retries
	DeadLetterStatusFailed DeadLetterStatus = "failed"

	// DeadLetterStatusResolved indicates the entry was manually resolved
	DeadLetterStatusResolved DeadLetterStatus = "resolved"

	// DeadLetterStatusDiscarded indicates the entry was manually discarded
	DeadLetterStatusDiscarded DeadLetterStatus = "discarded"
)

// DeadLetterQueue defines the interface for a dead letter queue
type DeadLetterQueue interface {
	// Enqueue adds a failed execution to the queue
	Enqueue(ctx context.Context, entry *DeadLetterEntry) error

	// Get retrieves an entry by ID
	Get(ctx context.Context, id string) (*DeadLetterEntry, error)

	// Query queries entries with filters
	Query(ctx context.Context, query *DeadLetterQuery) (*DeadLetterQueryResult, error)

	// UpdateStatus updates the status of an entry
	UpdateStatus(ctx context.Context, id string, status DeadLetterStatus) error

	// IncrementRetry increments the retry count for an entry
	IncrementRetry(ctx context.Context, id string) error

	// Delete removes an entry
	Delete(ctx context.Context, id string) error

	// DeleteByReactor removes all entries for a reactor
	DeleteByReactor(ctx context.Context, reactorID string) error

	// Purge removes entries older than the given age
	Purge(ctx context.Context, maxAge time.Duration) (int64, error)

	// Count returns the number of entries matching the query
	Count(ctx context.Context, query *DeadLetterQuery) (int64, error)

	// Close closes the queue
	Close() error
}

// DeadLetterQuery defines query parameters for the dead letter queue
type DeadLetterQuery struct {
	// Filter by reactor IDs
	ReactorIDs []string

	// Filter by status
	Statuses []DeadLetterStatus

	// Filter by time range
	StartTime *time.Time
	EndTime   *time.Time

	// Filter by retry count
	MinRetries *int
	MaxRetries *int

	// Filter by ready for retry (NextRetryAt <= now)
	ReadyForRetry bool

	// Pagination
	Limit  int
	Offset int

	// Sorting
	SortBy    string // "created_at", "next_retry_at", "retry_count"
	SortOrder string // "asc", "desc"
}

// DeadLetterQueryResult holds query results
type DeadLetterQueryResult struct {
	Entries    []*DeadLetterEntry
	TotalCount int64
	Offset     int
	Limit      int
}

// DeadLetterConfig holds configuration for the dead letter queue
type DeadLetterConfig struct {
	// Path is the database path
	Path string

	// MaxRetries is the default maximum number of retries
	MaxRetries int

	// RetryBackoff is the initial backoff duration between retries
	RetryBackoff time.Duration

	// RetryBackoffMultiplier multiplies backoff on each retry
	RetryBackoffMultiplier float64

	// MaxBackoff is the maximum backoff duration
	MaxBackoff time.Duration

	// AlertThreshold triggers an alert when this many entries are pending
	AlertThreshold int

	// AlertCallback is called when the threshold is reached
	AlertCallback DeadLetterAlertFunc

	// AutoRetry enables automatic retry processing
	AutoRetry bool

	// RetryInterval is how often to check for entries to retry
	RetryInterval time.Duration
}

// DeadLetterAlertFunc is called when alert conditions are met
type DeadLetterAlertFunc func(alert *DeadLetterAlert)

// DeadLetterAlert contains information about an alert condition
type DeadLetterAlert struct {
	// Reason describes why the alert was triggered
	Reason string `json:"reason"`

	// PendingCount is the current number of pending entries
	PendingCount int64 `json:"pending_count"`

	// FailedCount is the number of permanently failed entries
	FailedCount int64 `json:"failed_count"`

	// Threshold that was exceeded
	Threshold int `json:"threshold"`

	// TopReactors shows the reactors with most failures
	TopReactors []ReactorFailureCount `json:"top_reactors"`

	// Timestamp when the alert was generated
	Timestamp time.Time `json:"timestamp"`
}

// ReactorFailureCount tracks failures per reactor
type ReactorFailureCount struct {
	ReactorID   string `json:"reactor_id"`
	ReactorName string `json:"reactor_name"`
	Count       int64  `json:"count"`
}

// DefaultDeadLetterConfig returns default configuration
func DefaultDeadLetterConfig() *DeadLetterConfig {
	return &DeadLetterConfig{
		Path:                   "deadletter.db",
		MaxRetries:             5,
		RetryBackoff:           30 * time.Second,
		RetryBackoffMultiplier: 2.0,
		MaxBackoff:             1 * time.Hour,
		AlertThreshold:         100,
		AutoRetry:              true,
		RetryInterval:          1 * time.Minute,
	}
}

// SQLiteDeadLetterQueue implements DeadLetterQueue using SQLite
type SQLiteDeadLetterQueue struct {
	db     *sql.DB
	config *DeadLetterConfig

	// Metrics
	enqueuedCount   uint64
	retriedCount    uint64
	resolvedCount   uint64
	discardedCount  uint64
	alertsTriggered uint64

	// Auto-retry control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Alert state
	alertMu        sync.Mutex
	lastAlertTime  time.Time
	alertCooldown  time.Duration

	// Retry handler
	retryHandler DeadLetterRetryHandler
}

// DeadLetterRetryHandler handles retrying dead letter entries
type DeadLetterRetryHandler interface {
	// Retry attempts to retry the failed execution
	Retry(ctx context.Context, entry *DeadLetterEntry) error
}

// NewSQLiteDeadLetterQueue creates a new SQLite-backed dead letter queue
func NewSQLiteDeadLetterQueue(config *DeadLetterConfig) (*SQLiteDeadLetterQueue, error) {
	if config == nil {
		config = DefaultDeadLetterConfig()
	}

	db, err := sql.Open("sqlite", config.Path+"?_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Initialize schema
	if err := initDeadLetterSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	dlq := &SQLiteDeadLetterQueue{
		db:            db,
		config:        config,
		ctx:           ctx,
		cancel:        cancel,
		alertCooldown: 5 * time.Minute,
	}

	// Start auto-retry if enabled
	if config.AutoRetry {
		dlq.wg.Add(1)
		go dlq.autoRetryLoop()
	}

	return dlq, nil
}

// initDeadLetterSchema creates the dead letter queue table
func initDeadLetterSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS dead_letter_entries (
			id TEXT PRIMARY KEY,
			reactor_id TEXT NOT NULL,
			reactor_name TEXT,
			event_json TEXT NOT NULL,
			action_index INTEGER DEFAULT -1,
			action_name TEXT,
			error TEXT NOT NULL,
			retry_count INTEGER DEFAULT 0,
			max_retries INTEGER DEFAULT 5,
			created_at DATETIME NOT NULL,
			last_retry_at DATETIME,
			next_retry_at DATETIME,
			status TEXT NOT NULL DEFAULT 'pending',
			metadata_json TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_dlq_reactor_id ON dead_letter_entries(reactor_id);
		CREATE INDEX IF NOT EXISTS idx_dlq_status ON dead_letter_entries(status);
		CREATE INDEX IF NOT EXISTS idx_dlq_created_at ON dead_letter_entries(created_at);
		CREATE INDEX IF NOT EXISTS idx_dlq_next_retry_at ON dead_letter_entries(next_retry_at);
	`)
	return err
}

// SetRetryHandler sets the handler for retrying entries
func (dlq *SQLiteDeadLetterQueue) SetRetryHandler(handler DeadLetterRetryHandler) {
	dlq.retryHandler = handler
}

// Enqueue adds a failed execution to the queue
func (dlq *SQLiteDeadLetterQueue) Enqueue(ctx context.Context, entry *DeadLetterEntry) error {
	if entry.ID == "" {
		entry.ID = generateEventID()
	}
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now()
	}
	if entry.Status == "" {
		entry.Status = DeadLetterStatusPending
	}
	if entry.MaxRetries == 0 {
		entry.MaxRetries = dlq.config.MaxRetries
	}

	// Calculate next retry time
	nextRetry := dlq.calculateNextRetry(entry.RetryCount)
	entry.NextRetryAt = &nextRetry

	// Serialize event and metadata
	eventJSON, err := json.Marshal(entry.Event)
	if err != nil {
		return fmt.Errorf("failed to serialize event: %w", err)
	}

	var metadataJSON []byte
	if entry.Metadata != nil {
		metadataJSON, err = json.Marshal(entry.Metadata)
		if err != nil {
			return fmt.Errorf("failed to serialize metadata: %w", err)
		}
	}

	_, err = dlq.db.ExecContext(ctx, `
		INSERT INTO dead_letter_entries (
			id, reactor_id, reactor_name, event_json, action_index, action_name,
			error, retry_count, max_retries, created_at, last_retry_at, next_retry_at,
			status, metadata_json
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`,
		entry.ID, entry.ReactorID, entry.ReactorName, string(eventJSON),
		entry.ActionIndex, entry.ActionName, entry.Error, entry.RetryCount,
		entry.MaxRetries, entry.CreatedAt, entry.LastRetryAt, entry.NextRetryAt,
		entry.Status, string(metadataJSON),
	)
	if err != nil {
		return fmt.Errorf("failed to enqueue entry: %w", err)
	}

	atomic.AddUint64(&dlq.enqueuedCount, 1)

	// Check alert threshold
	dlq.checkAlertThreshold(ctx)

	return nil
}

// Get retrieves an entry by ID
func (dlq *SQLiteDeadLetterQueue) Get(ctx context.Context, id string) (*DeadLetterEntry, error) {
	row := dlq.db.QueryRowContext(ctx, `
		SELECT id, reactor_id, reactor_name, event_json, action_index, action_name,
			error, retry_count, max_retries, created_at, last_retry_at, next_retry_at,
			status, metadata_json
		FROM dead_letter_entries
		WHERE id = ?
	`, id)

	return dlq.scanEntry(row)
}

// Query queries entries with filters
func (dlq *SQLiteDeadLetterQueue) Query(ctx context.Context, query *DeadLetterQuery) (*DeadLetterQueryResult, error) {
	whereClause, args := dlq.buildWhereClause(query)

	// Count total
	countQuery := "SELECT COUNT(*) FROM dead_letter_entries" + whereClause
	var totalCount int64
	if err := dlq.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("failed to count entries: %w", err)
	}

	// Build order clause
	orderClause := " ORDER BY "
	switch query.SortBy {
	case "next_retry_at":
		orderClause += "next_retry_at"
	case "retry_count":
		orderClause += "retry_count"
	default:
		orderClause += "created_at"
	}
	if query.SortOrder == "asc" {
		orderClause += " ASC"
	} else {
		orderClause += " DESC"
	}

	// Apply pagination
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	offset := query.Offset
	orderClause += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)

	// Query entries
	selectQuery := `
		SELECT id, reactor_id, reactor_name, event_json, action_index, action_name,
			error, retry_count, max_retries, created_at, last_retry_at, next_retry_at,
			status, metadata_json
		FROM dead_letter_entries` + whereClause + orderClause

	rows, err := dlq.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query entries: %w", err)
	}
	defer rows.Close()

	entries := make([]*DeadLetterEntry, 0)
	for rows.Next() {
		entry, err := dlq.scanEntryRows(rows)
		if err != nil {
			return nil, err
		}
		entries = append(entries, entry)
	}

	return &DeadLetterQueryResult{
		Entries:    entries,
		TotalCount: totalCount,
		Offset:     offset,
		Limit:      limit,
	}, nil
}

// buildWhereClause builds the WHERE clause for queries
func (dlq *SQLiteDeadLetterQueue) buildWhereClause(query *DeadLetterQuery) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if len(query.ReactorIDs) > 0 {
		placeholders := make([]string, len(query.ReactorIDs))
		for i, id := range query.ReactorIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		conditions = append(conditions, fmt.Sprintf("reactor_id IN (%s)", joinStrings(placeholders, ", ")))
	}

	if len(query.Statuses) > 0 {
		placeholders := make([]string, len(query.Statuses))
		for i, status := range query.Statuses {
			placeholders[i] = "?"
			args = append(args, status)
		}
		conditions = append(conditions, fmt.Sprintf("status IN (%s)", joinStrings(placeholders, ", ")))
	}

	if query.StartTime != nil {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, *query.StartTime)
	}

	if query.EndTime != nil {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, *query.EndTime)
	}

	if query.MinRetries != nil {
		conditions = append(conditions, "retry_count >= ?")
		args = append(args, *query.MinRetries)
	}

	if query.MaxRetries != nil {
		conditions = append(conditions, "retry_count <= ?")
		args = append(args, *query.MaxRetries)
	}

	if query.ReadyForRetry {
		conditions = append(conditions, "next_retry_at <= ? AND status = 'pending'")
		args = append(args, time.Now())
	}

	if len(conditions) == 0 {
		return "", args
	}

	return " WHERE " + joinStrings(conditions, " AND "), args
}

// UpdateStatus updates the status of an entry
func (dlq *SQLiteDeadLetterQueue) UpdateStatus(ctx context.Context, id string, status DeadLetterStatus) error {
	result, err := dlq.db.ExecContext(ctx,
		"UPDATE dead_letter_entries SET status = ? WHERE id = ?",
		status, id,
	)
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("entry not found: %s", id)
	}

	switch status {
	case DeadLetterStatusResolved:
		atomic.AddUint64(&dlq.resolvedCount, 1)
	case DeadLetterStatusDiscarded:
		atomic.AddUint64(&dlq.discardedCount, 1)
	}

	return nil
}

// IncrementRetry increments the retry count for an entry
func (dlq *SQLiteDeadLetterQueue) IncrementRetry(ctx context.Context, id string) error {
	entry, err := dlq.Get(ctx, id)
	if err != nil {
		return err
	}

	newRetryCount := entry.RetryCount + 1
	now := time.Now()
	nextRetry := dlq.calculateNextRetry(newRetryCount)

	var status DeadLetterStatus
	if newRetryCount >= entry.MaxRetries {
		status = DeadLetterStatusFailed
	} else {
		status = DeadLetterStatusPending
	}

	_, err = dlq.db.ExecContext(ctx, `
		UPDATE dead_letter_entries
		SET retry_count = ?, last_retry_at = ?, next_retry_at = ?, status = ?
		WHERE id = ?
	`, newRetryCount, now, nextRetry, status, id)
	if err != nil {
		return fmt.Errorf("failed to increment retry: %w", err)
	}

	atomic.AddUint64(&dlq.retriedCount, 1)

	return nil
}

// Delete removes an entry
func (dlq *SQLiteDeadLetterQueue) Delete(ctx context.Context, id string) error {
	result, err := dlq.db.ExecContext(ctx, "DELETE FROM dead_letter_entries WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete entry: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected == 0 {
		return fmt.Errorf("entry not found: %s", id)
	}

	return nil
}

// DeleteByReactor removes all entries for a reactor
func (dlq *SQLiteDeadLetterQueue) DeleteByReactor(ctx context.Context, reactorID string) error {
	_, err := dlq.db.ExecContext(ctx, "DELETE FROM dead_letter_entries WHERE reactor_id = ?", reactorID)
	if err != nil {
		return fmt.Errorf("failed to delete entries: %w", err)
	}
	return nil
}

// Purge removes entries older than the given age
func (dlq *SQLiteDeadLetterQueue) Purge(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge)
	result, err := dlq.db.ExecContext(ctx,
		"DELETE FROM dead_letter_entries WHERE created_at < ?",
		cutoff,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to purge entries: %w", err)
	}

	affected, _ := result.RowsAffected()
	return affected, nil
}

// Count returns the number of entries matching the query
func (dlq *SQLiteDeadLetterQueue) Count(ctx context.Context, query *DeadLetterQuery) (int64, error) {
	whereClause, args := dlq.buildWhereClause(query)
	countQuery := "SELECT COUNT(*) FROM dead_letter_entries" + whereClause

	var count int64
	if err := dlq.db.QueryRowContext(ctx, countQuery, args...).Scan(&count); err != nil {
		return 0, fmt.Errorf("failed to count entries: %w", err)
	}

	return count, nil
}

// Close closes the queue
func (dlq *SQLiteDeadLetterQueue) Close() error {
	dlq.cancel()
	dlq.wg.Wait()
	return dlq.db.Close()
}

// GetMetrics returns queue metrics
func (dlq *SQLiteDeadLetterQueue) GetMetrics() *DeadLetterMetrics {
	return &DeadLetterMetrics{
		EnqueuedCount:   atomic.LoadUint64(&dlq.enqueuedCount),
		RetriedCount:    atomic.LoadUint64(&dlq.retriedCount),
		ResolvedCount:   atomic.LoadUint64(&dlq.resolvedCount),
		DiscardedCount:  atomic.LoadUint64(&dlq.discardedCount),
		AlertsTriggered: atomic.LoadUint64(&dlq.alertsTriggered),
	}
}

// DeadLetterMetrics contains queue metrics
type DeadLetterMetrics struct {
	EnqueuedCount   uint64 `json:"enqueued_count"`
	RetriedCount    uint64 `json:"retried_count"`
	ResolvedCount   uint64 `json:"resolved_count"`
	DiscardedCount  uint64 `json:"discarded_count"`
	AlertsTriggered uint64 `json:"alerts_triggered"`
}

// calculateNextRetry calculates the next retry time using exponential backoff
func (dlq *SQLiteDeadLetterQueue) calculateNextRetry(retryCount int) time.Time {
	backoff := dlq.config.RetryBackoff
	for i := 0; i < retryCount; i++ {
		backoff = time.Duration(float64(backoff) * dlq.config.RetryBackoffMultiplier)
		if backoff > dlq.config.MaxBackoff {
			backoff = dlq.config.MaxBackoff
			break
		}
	}
	return time.Now().Add(backoff)
}

// checkAlertThreshold checks if alert threshold is exceeded
func (dlq *SQLiteDeadLetterQueue) checkAlertThreshold(ctx context.Context) {
	if dlq.config.AlertCallback == nil || dlq.config.AlertThreshold <= 0 {
		return
	}

	dlq.alertMu.Lock()
	defer dlq.alertMu.Unlock()

	// Check cooldown
	if time.Since(dlq.lastAlertTime) < dlq.alertCooldown {
		return
	}

	// Count pending entries
	var pendingCount int64
	dlq.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM dead_letter_entries WHERE status = 'pending'").Scan(&pendingCount)

	if pendingCount < int64(dlq.config.AlertThreshold) {
		return
	}

	// Get failed count
	var failedCount int64
	dlq.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM dead_letter_entries WHERE status = 'failed'").Scan(&failedCount)

	// Get top failing reactors
	rows, _ := dlq.db.QueryContext(ctx, `
		SELECT reactor_id, reactor_name, COUNT(*) as count
		FROM dead_letter_entries
		WHERE status IN ('pending', 'failed')
		GROUP BY reactor_id, reactor_name
		ORDER BY count DESC
		LIMIT 5
	`)
	defer rows.Close()

	var topReactors []ReactorFailureCount
	for rows.Next() {
		var rf ReactorFailureCount
		rows.Scan(&rf.ReactorID, &rf.ReactorName, &rf.Count)
		topReactors = append(topReactors, rf)
	}

	// Trigger alert
	alert := &DeadLetterAlert{
		Reason:       "Dead letter queue threshold exceeded",
		PendingCount: pendingCount,
		FailedCount:  failedCount,
		Threshold:    dlq.config.AlertThreshold,
		TopReactors:  topReactors,
		Timestamp:    time.Now(),
	}

	dlq.lastAlertTime = time.Now()
	atomic.AddUint64(&dlq.alertsTriggered, 1)

	go dlq.config.AlertCallback(alert)
}

// autoRetryLoop runs the automatic retry processor
func (dlq *SQLiteDeadLetterQueue) autoRetryLoop() {
	defer dlq.wg.Done()

	ticker := time.NewTicker(dlq.config.RetryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-dlq.ctx.Done():
			return
		case <-ticker.C:
			dlq.processRetries()
		}
	}
}

// processRetries processes entries ready for retry
func (dlq *SQLiteDeadLetterQueue) processRetries() {
	if dlq.retryHandler == nil {
		return
	}

	ctx := dlq.ctx

	// Find entries ready for retry
	result, err := dlq.Query(ctx, &DeadLetterQuery{
		ReadyForRetry: true,
		Limit:         10,
	})
	if err != nil {
		return
	}

	for _, entry := range result.Entries {
		// Mark as retrying
		dlq.UpdateStatus(ctx, entry.ID, DeadLetterStatusRetrying)

		// Attempt retry
		err := dlq.retryHandler.Retry(ctx, entry)
		if err != nil {
			// Increment retry count (will set status back to pending or failed)
			dlq.IncrementRetry(ctx, entry.ID)
		} else {
			// Mark as resolved
			dlq.UpdateStatus(ctx, entry.ID, DeadLetterStatusResolved)
		}
	}
}

// scanEntry scans a single row into a DeadLetterEntry
func (dlq *SQLiteDeadLetterQueue) scanEntry(row *sql.Row) (*DeadLetterEntry, error) {
	var entry DeadLetterEntry
	var eventJSON, metadataJSON string
	var lastRetryAt, nextRetryAt sql.NullTime

	err := row.Scan(
		&entry.ID, &entry.ReactorID, &entry.ReactorName, &eventJSON,
		&entry.ActionIndex, &entry.ActionName, &entry.Error, &entry.RetryCount,
		&entry.MaxRetries, &entry.CreatedAt, &lastRetryAt, &nextRetryAt,
		&entry.Status, &metadataJSON,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("entry not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan entry: %w", err)
	}

	// Deserialize event
	if err := json.Unmarshal([]byte(eventJSON), &entry.Event); err != nil {
		return nil, fmt.Errorf("failed to deserialize event: %w", err)
	}

	// Handle nullable times
	if lastRetryAt.Valid {
		entry.LastRetryAt = &lastRetryAt.Time
	}
	if nextRetryAt.Valid {
		entry.NextRetryAt = &nextRetryAt.Time
	}

	// Deserialize metadata
	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
	}

	return &entry, nil
}

// scanEntryRows scans a row from rows into a DeadLetterEntry
func (dlq *SQLiteDeadLetterQueue) scanEntryRows(rows *sql.Rows) (*DeadLetterEntry, error) {
	var entry DeadLetterEntry
	var eventJSON, metadataJSON string
	var lastRetryAt, nextRetryAt sql.NullTime

	err := rows.Scan(
		&entry.ID, &entry.ReactorID, &entry.ReactorName, &eventJSON,
		&entry.ActionIndex, &entry.ActionName, &entry.Error, &entry.RetryCount,
		&entry.MaxRetries, &entry.CreatedAt, &lastRetryAt, &nextRetryAt,
		&entry.Status, &metadataJSON,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to scan entry: %w", err)
	}

	// Deserialize event
	if err := json.Unmarshal([]byte(eventJSON), &entry.Event); err != nil {
		return nil, fmt.Errorf("failed to deserialize event: %w", err)
	}

	// Handle nullable times
	if lastRetryAt.Valid {
		entry.LastRetryAt = &lastRetryAt.Time
	}
	if nextRetryAt.Valid {
		entry.NextRetryAt = &nextRetryAt.Time
	}

	// Deserialize metadata
	if metadataJSON != "" {
		json.Unmarshal([]byte(metadataJSON), &entry.Metadata)
	}

	return &entry, nil
}

// joinStrings joins strings with a separator
func joinStrings(strs []string, sep string) string {
	if len(strs) == 0 {
		return ""
	}
	result := strs[0]
	for i := 1; i < len(strs); i++ {
		result += sep + strs[i]
	}
	return result
}
