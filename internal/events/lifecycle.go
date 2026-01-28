package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

// LifecycleState represents the state of an event in its lifecycle
type LifecycleState string

const (
	// LifecycleStateCreated indicates the event has been created
	LifecycleStateCreated LifecycleState = "created"

	// LifecycleStatePublished indicates the event has been published to the bus
	LifecycleStatePublished LifecycleState = "published"

	// LifecycleStateRouted indicates the event has been routed to handlers
	LifecycleStateRouted LifecycleState = "routed"

	// LifecycleStateProcessing indicates the event is being processed
	LifecycleStateProcessing LifecycleState = "processing"

	// LifecycleStateProcessed indicates the event has been successfully processed
	LifecycleStateProcessed LifecycleState = "processed"

	// LifecycleStateFailed indicates the event processing failed
	LifecycleStateFailed LifecycleState = "failed"

	// LifecycleStateRetrying indicates the event is being retried
	LifecycleStateRetrying LifecycleState = "retrying"

	// LifecycleStateArchived indicates the event has been archived
	LifecycleStateArchived LifecycleState = "archived"

	// LifecycleStateExpired indicates the event has expired
	LifecycleStateExpired LifecycleState = "expired"
)

// LifecycleTransition represents a state transition in an event's lifecycle
type LifecycleTransition struct {
	// EventID is the ID of the event
	EventID string `json:"event_id"`

	// FromState is the previous state (empty for initial state)
	FromState LifecycleState `json:"from_state,omitempty"`

	// ToState is the new state
	ToState LifecycleState `json:"to_state"`

	// Timestamp is when the transition occurred
	Timestamp time.Time `json:"timestamp"`

	// Component is the component that triggered the transition
	Component string `json:"component"`

	// Details contains additional context about the transition
	Details map[string]interface{} `json:"details,omitempty"`

	// Duration is the time spent in the previous state (if applicable)
	Duration time.Duration `json:"duration,omitempty"`
}

// EventLifecycle represents the complete lifecycle of an event
type EventLifecycle struct {
	// EventID is the ID of the event
	EventID string `json:"event_id"`

	// EventType is the type of the event
	EventType EventType `json:"event_type"`

	// CurrentState is the current lifecycle state
	CurrentState LifecycleState `json:"current_state"`

	// CreatedAt is when the event was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the lifecycle was last updated
	UpdatedAt time.Time `json:"updated_at"`

	// Transitions is the ordered list of state transitions
	Transitions []*LifecycleTransition `json:"transitions"`

	// ProcessingAttempts is the number of processing attempts
	ProcessingAttempts int `json:"processing_attempts"`

	// LastError is the last error message (if any)
	LastError string `json:"last_error,omitempty"`

	// Metadata contains additional lifecycle metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// TotalDuration returns the total duration from creation to current state
func (l *EventLifecycle) TotalDuration() time.Duration {
	return l.UpdatedAt.Sub(l.CreatedAt)
}

// StateDuration returns the time spent in a specific state
func (l *EventLifecycle) StateDuration(state LifecycleState) time.Duration {
	var total time.Duration
	for _, t := range l.Transitions {
		if t.FromState == state && t.Duration > 0 {
			total += t.Duration
		}
	}
	return total
}

// IsTerminal returns true if the lifecycle is in a terminal state
func (l *EventLifecycle) IsTerminal() bool {
	return l.CurrentState == LifecycleStateProcessed ||
		l.CurrentState == LifecycleStateFailed ||
		l.CurrentState == LifecycleStateArchived ||
		l.CurrentState == LifecycleStateExpired
}

// LifecycleTracker tracks the lifecycle of events
type LifecycleTracker interface {
	// Track records a lifecycle transition
	Track(ctx context.Context, transition *LifecycleTransition) error

	// Get retrieves the lifecycle for an event
	Get(ctx context.Context, eventID string) (*EventLifecycle, error)

	// Query queries lifecycles with filters
	Query(ctx context.Context, query *LifecycleQuery) (*LifecycleQueryResult, error)

	// GetMetrics returns lifecycle metrics
	GetMetrics(ctx context.Context) (*LifecycleMetrics, error)

	// Cleanup removes old lifecycle records
	Cleanup(ctx context.Context, maxAge time.Duration) (int64, error)

	// Close closes the tracker
	Close() error
}

// LifecycleQuery defines query parameters for lifecycle queries
type LifecycleQuery struct {
	// Filter by event IDs
	EventIDs []string

	// Filter by event types
	EventTypes []EventType

	// Filter by current state
	States []LifecycleState

	// Filter by time range (based on created_at)
	StartTime *time.Time
	EndTime   *time.Time

	// Filter by component
	Component string

	// Include only terminal states
	TerminalOnly bool

	// Include only non-terminal states
	ActiveOnly bool

	// Pagination
	Limit  int
	Offset int

	// Sorting
	SortBy    string // "created_at", "updated_at", "duration"
	SortOrder string // "asc", "desc"
}

// LifecycleQueryResult holds query results
type LifecycleQueryResult struct {
	Lifecycles []*EventLifecycle
	TotalCount int64
	Offset     int
	Limit      int
}

// LifecycleMetrics contains lifecycle statistics
type LifecycleMetrics struct {
	// Total events tracked
	TotalTracked int64 `json:"total_tracked"`

	// Events by current state
	ByState map[LifecycleState]int64 `json:"by_state"`

	// Events by type
	ByType map[EventType]int64 `json:"by_type"`

	// Average processing time (for processed events)
	AvgProcessingTime time.Duration `json:"avg_processing_time"`

	// P50 processing time
	P50ProcessingTime time.Duration `json:"p50_processing_time"`

	// P95 processing time
	P95ProcessingTime time.Duration `json:"p95_processing_time"`

	// P99 processing time
	P99ProcessingTime time.Duration `json:"p99_processing_time"`

	// Success rate (processed / (processed + failed))
	SuccessRate float64 `json:"success_rate"`

	// Retry rate (events with retries / total)
	RetryRate float64 `json:"retry_rate"`

	// Events processed in last hour
	ProcessedLastHour int64 `json:"processed_last_hour"`

	// Events failed in last hour
	FailedLastHour int64 `json:"failed_last_hour"`
}

// LifecycleTrackerConfig configures the lifecycle tracker
type LifecycleTrackerConfig struct {
	// Path is the database path
	Path string

	// RetentionPeriod is how long to keep lifecycle records
	RetentionPeriod time.Duration

	// CleanupInterval is how often to run cleanup
	CleanupInterval time.Duration

	// EnableAutoCleanup enables automatic cleanup
	EnableAutoCleanup bool

	// MaxBatchSize for bulk operations
	MaxBatchSize int
}

// DefaultLifecycleTrackerConfig returns default configuration
func DefaultLifecycleTrackerConfig() *LifecycleTrackerConfig {
	return &LifecycleTrackerConfig{
		Path:              "lifecycle.db",
		RetentionPeriod:   7 * 24 * time.Hour, // 7 days
		CleanupInterval:   1 * time.Hour,
		EnableAutoCleanup: true,
		MaxBatchSize:      1000,
	}
}

// SQLiteLifecycleTracker implements LifecycleTracker using SQLite
type SQLiteLifecycleTracker struct {
	db     *sql.DB
	config *LifecycleTrackerConfig

	// Metrics
	trackedCount    uint64
	transitionCount uint64

	// Cleanup control
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu sync.RWMutex
}

// NewSQLiteLifecycleTracker creates a new SQLite-backed lifecycle tracker
func NewSQLiteLifecycleTracker(config *LifecycleTrackerConfig) (*SQLiteLifecycleTracker, error) {
	if config == nil {
		config = DefaultLifecycleTrackerConfig()
	}

	db, err := sql.Open("sqlite", config.Path+"?_journal=WAL&_busy_timeout=5000")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := initLifecycleSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	tracker := &SQLiteLifecycleTracker{
		db:     db,
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}

	if config.EnableAutoCleanup {
		tracker.wg.Add(1)
		go tracker.cleanupLoop()
	}

	return tracker, nil
}

// initLifecycleSchema creates the lifecycle tables
func initLifecycleSchema(db *sql.DB) error {
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS event_lifecycles (
			event_id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			current_state TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL,
			processing_attempts INTEGER DEFAULT 0,
			last_error TEXT,
			metadata_json TEXT
		);

		CREATE TABLE IF NOT EXISTS lifecycle_transitions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_id TEXT NOT NULL,
			from_state TEXT,
			to_state TEXT NOT NULL,
			timestamp DATETIME NOT NULL,
			component TEXT,
			details_json TEXT,
			duration_ns INTEGER,
			FOREIGN KEY (event_id) REFERENCES event_lifecycles(event_id)
		);

		CREATE INDEX IF NOT EXISTS idx_lifecycle_state ON event_lifecycles(current_state);
		CREATE INDEX IF NOT EXISTS idx_lifecycle_type ON event_lifecycles(event_type);
		CREATE INDEX IF NOT EXISTS idx_lifecycle_created ON event_lifecycles(created_at);
		CREATE INDEX IF NOT EXISTS idx_lifecycle_updated ON event_lifecycles(updated_at);
		CREATE INDEX IF NOT EXISTS idx_transitions_event ON lifecycle_transitions(event_id);
		CREATE INDEX IF NOT EXISTS idx_transitions_timestamp ON lifecycle_transitions(timestamp);
	`)
	return err
}

// Track records a lifecycle transition
func (t *SQLiteLifecycleTracker) Track(ctx context.Context, transition *LifecycleTransition) error {
	if transition == nil {
		return fmt.Errorf("transition cannot be nil")
	}
	if transition.EventID == "" {
		return fmt.Errorf("event ID is required")
	}
	if transition.ToState == "" {
		return fmt.Errorf("to state is required")
	}
	if transition.Timestamp.IsZero() {
		transition.Timestamp = time.Now()
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Start transaction
	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if lifecycle exists
	var exists bool
	var currentState string
	var createdAt time.Time
	err = tx.QueryRowContext(ctx,
		"SELECT current_state, created_at FROM event_lifecycles WHERE event_id = ?",
		transition.EventID,
	).Scan(&currentState, &createdAt)

	if err == sql.ErrNoRows {
		exists = false
	} else if err != nil {
		return fmt.Errorf("failed to check existing lifecycle: %w", err)
	} else {
		exists = true
		transition.FromState = LifecycleState(currentState)
		transition.Duration = transition.Timestamp.Sub(createdAt)
	}

	// Serialize details
	var detailsJSON []byte
	if transition.Details != nil {
		detailsJSON, _ = json.Marshal(transition.Details)
	}

	// Insert transition
	_, err = tx.ExecContext(ctx, `
		INSERT INTO lifecycle_transitions (event_id, from_state, to_state, timestamp, component, details_json, duration_ns)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, transition.EventID, transition.FromState, transition.ToState, transition.Timestamp,
		transition.Component, string(detailsJSON), transition.Duration.Nanoseconds())
	if err != nil {
		return fmt.Errorf("failed to insert transition: %w", err)
	}

	// Update or insert lifecycle record
	if exists {
		setClauses := []string{"current_state = ?", "updated_at = ?"}
		args := []interface{}{transition.ToState, transition.Timestamp}

		// Update processing attempts if retrying
		if transition.ToState == LifecycleStateRetrying || transition.ToState == LifecycleStateProcessing {
			setClauses = append(setClauses, "processing_attempts = processing_attempts + 1")
		}

		// Update error if failed
		if transition.ToState == LifecycleStateFailed {
			if errMsg, ok := transition.Details["error"].(string); ok {
				setClauses = append(setClauses, "last_error = ?")
				args = append(args, errMsg)
			}
		}

		query := "UPDATE event_lifecycles SET " + strings.Join(setClauses, ", ") + " WHERE event_id = ?"
		args = append(args, transition.EventID)
		_, err = tx.ExecContext(ctx, query, args...)
	} else {
		// Get event type from details if available
		eventType := ""
		if et, ok := transition.Details["event_type"].(string); ok {
			eventType = et
		}

		_, err = tx.ExecContext(ctx, `
			INSERT INTO event_lifecycles (event_id, event_type, current_state, created_at, updated_at, processing_attempts)
			VALUES (?, ?, ?, ?, ?, 0)
		`, transition.EventID, eventType, transition.ToState, transition.Timestamp, transition.Timestamp)

		atomic.AddUint64(&t.trackedCount, 1)
	}

	if err != nil {
		return fmt.Errorf("failed to update lifecycle: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	atomic.AddUint64(&t.transitionCount, 1)
	return nil
}

// Get retrieves the lifecycle for an event
func (t *SQLiteLifecycleTracker) Get(ctx context.Context, eventID string) (*EventLifecycle, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Get lifecycle record
	var lifecycle EventLifecycle
	var metadataJSON sql.NullString
	var lastError sql.NullString

	err := t.db.QueryRowContext(ctx, `
		SELECT event_id, event_type, current_state, created_at, updated_at, processing_attempts, last_error, metadata_json
		FROM event_lifecycles
		WHERE event_id = ?
	`, eventID).Scan(
		&lifecycle.EventID, &lifecycle.EventType, &lifecycle.CurrentState,
		&lifecycle.CreatedAt, &lifecycle.UpdatedAt, &lifecycle.ProcessingAttempts,
		&lastError, &metadataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("lifecycle not found for event: %s", eventID)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get lifecycle: %w", err)
	}

	if lastError.Valid {
		lifecycle.LastError = lastError.String
	}
	if metadataJSON.Valid {
		json.Unmarshal([]byte(metadataJSON.String), &lifecycle.Metadata)
	}

	// Get transitions
	rows, err := t.db.QueryContext(ctx, `
		SELECT from_state, to_state, timestamp, component, details_json, duration_ns
		FROM lifecycle_transitions
		WHERE event_id = ?
		ORDER BY timestamp ASC
	`, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed to get transitions: %w", err)
	}
	defer rows.Close()

	lifecycle.Transitions = make([]*LifecycleTransition, 0)
	for rows.Next() {
		var trans LifecycleTransition
		var fromState sql.NullString
		var component sql.NullString
		var detailsJSON sql.NullString
		var durationNS sql.NullInt64

		if err := rows.Scan(&fromState, &trans.ToState, &trans.Timestamp, &component, &detailsJSON, &durationNS); err != nil {
			return nil, fmt.Errorf("failed to scan transition: %w", err)
		}

		trans.EventID = eventID
		if fromState.Valid {
			trans.FromState = LifecycleState(fromState.String)
		}
		if component.Valid {
			trans.Component = component.String
		}
		if detailsJSON.Valid {
			json.Unmarshal([]byte(detailsJSON.String), &trans.Details)
		}
		if durationNS.Valid {
			trans.Duration = time.Duration(durationNS.Int64)
		}

		lifecycle.Transitions = append(lifecycle.Transitions, &trans)
	}

	return &lifecycle, nil
}

// Query queries lifecycles with filters
func (t *SQLiteLifecycleTracker) Query(ctx context.Context, query *LifecycleQuery) (*LifecycleQueryResult, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	whereClause, args := t.buildWhereClause(query)

	// Count total
	countQuery := "SELECT COUNT(*) FROM event_lifecycles" + whereClause // nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- whereClause built from whitelisted fields with placeholders; args are bound separately
	var totalCount int64
	if err := t.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("failed to count lifecycles: %w", err)
	}

	// Build order clause
	orderClause := " ORDER BY "
	switch query.SortBy {
	case "updated_at":
		orderClause += "updated_at"
	case "duration":
		orderClause += "(julianday(updated_at) - julianday(created_at))"
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
	orderClause += " LIMIT ? OFFSET ?"

	// Query lifecycles
	selectQuery := `
		SELECT event_id, event_type, current_state, created_at, updated_at, processing_attempts, last_error
		FROM event_lifecycles` + whereClause + orderClause

	args = append(args, limit, offset)
	rows, err := t.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query lifecycles: %w", err)
	}
	defer rows.Close()

	lifecycles := make([]*EventLifecycle, 0)
	for rows.Next() {
		var lc EventLifecycle
		var lastError sql.NullString

		if err := rows.Scan(&lc.EventID, &lc.EventType, &lc.CurrentState, &lc.CreatedAt,
			&lc.UpdatedAt, &lc.ProcessingAttempts, &lastError); err != nil {
			return nil, fmt.Errorf("failed to scan lifecycle: %w", err)
		}

		if lastError.Valid {
			lc.LastError = lastError.String
		}

		lifecycles = append(lifecycles, &lc)
	}

	return &LifecycleQueryResult{
		Lifecycles: lifecycles,
		TotalCount: totalCount,
		Offset:     offset,
		Limit:      limit,
	}, nil
}

// buildWhereClause builds the WHERE clause for queries
func (t *SQLiteLifecycleTracker) buildWhereClause(query *LifecycleQuery) (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if len(query.EventIDs) > 0 {
		placeholders := make([]string, len(query.EventIDs))
		for i, id := range query.EventIDs {
			placeholders[i] = "?"
			args = append(args, id)
		}
		conditions = append(conditions, fmt.Sprintf("event_id IN (%s)", joinStrings(placeholders, ", ")))
	}

	if len(query.EventTypes) > 0 {
		placeholders := make([]string, len(query.EventTypes))
		for i, et := range query.EventTypes {
			placeholders[i] = "?"
			args = append(args, et)
		}
		conditions = append(conditions, fmt.Sprintf("event_type IN (%s)", joinStrings(placeholders, ", ")))
	}

	if len(query.States) > 0 {
		placeholders := make([]string, len(query.States))
		for i, state := range query.States {
			placeholders[i] = "?"
			args = append(args, state)
		}
		conditions = append(conditions, fmt.Sprintf("current_state IN (%s)", joinStrings(placeholders, ", ")))
	}

	if query.StartTime != nil {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, *query.StartTime)
	}

	if query.EndTime != nil {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, *query.EndTime)
	}

	if query.TerminalOnly {
		conditions = append(conditions, "current_state IN ('processed', 'failed', 'archived', 'expired')")
	}

	if query.ActiveOnly {
		conditions = append(conditions, "current_state NOT IN ('processed', 'failed', 'archived', 'expired')")
	}

	if len(conditions) == 0 {
		return "", args
	}

	return " WHERE " + joinStrings(conditions, " AND "), args
}

// GetMetrics returns lifecycle metrics
func (t *SQLiteLifecycleTracker) GetMetrics(ctx context.Context) (*LifecycleMetrics, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	metrics := &LifecycleMetrics{
		ByState: make(map[LifecycleState]int64),
		ByType:  make(map[EventType]int64),
	}

	// Total tracked
	t.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_lifecycles").Scan(&metrics.TotalTracked)

	// By state
	rows, _ := t.db.QueryContext(ctx, "SELECT current_state, COUNT(*) FROM event_lifecycles GROUP BY current_state")
	for rows.Next() {
		var state LifecycleState
		var count int64
		rows.Scan(&state, &count)
		metrics.ByState[state] = count
	}
	rows.Close()

	// By type
	rows, _ = t.db.QueryContext(ctx, "SELECT event_type, COUNT(*) FROM event_lifecycles WHERE event_type != '' GROUP BY event_type")
	for rows.Next() {
		var eventType EventType
		var count int64
		rows.Scan(&eventType, &count)
		metrics.ByType[eventType] = count
	}
	rows.Close()

	// Processing times for processed events
	var avgDuration float64
	t.db.QueryRowContext(ctx, `
		SELECT AVG((julianday(updated_at) - julianday(created_at)) * 86400000)
		FROM event_lifecycles
		WHERE current_state = 'processed'
	`).Scan(&avgDuration)
	metrics.AvgProcessingTime = time.Duration(avgDuration) * time.Millisecond

	// Success rate
	var processed, failed int64
	t.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_lifecycles WHERE current_state = 'processed'").Scan(&processed)
	t.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_lifecycles WHERE current_state = 'failed'").Scan(&failed)
	if processed+failed > 0 {
		metrics.SuccessRate = float64(processed) / float64(processed+failed)
	}

	// Retry rate
	var withRetries int64
	t.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM event_lifecycles WHERE processing_attempts > 1").Scan(&withRetries)
	if metrics.TotalTracked > 0 {
		metrics.RetryRate = float64(withRetries) / float64(metrics.TotalTracked)
	}

	// Last hour stats
	hourAgo := time.Now().Add(-1 * time.Hour)
	t.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM event_lifecycles WHERE current_state = 'processed' AND updated_at >= ?",
		hourAgo,
	).Scan(&metrics.ProcessedLastHour)
	t.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM event_lifecycles WHERE current_state = 'failed' AND updated_at >= ?",
		hourAgo,
	).Scan(&metrics.FailedLastHour)

	return metrics, nil
}

// Cleanup removes old lifecycle records
func (t *SQLiteLifecycleTracker) Cleanup(ctx context.Context, maxAge time.Duration) (int64, error) {
	t.mu.Lock()
	defer t.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)

	// Delete transitions first (foreign key)
	_, err := t.db.ExecContext(ctx, `
		DELETE FROM lifecycle_transitions
		WHERE event_id IN (
			SELECT event_id FROM event_lifecycles
			WHERE updated_at < ? AND current_state IN ('processed', 'failed', 'archived', 'expired')
		)
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to delete transitions: %w", err)
	}

	// Delete lifecycles
	result, err := t.db.ExecContext(ctx, `
		DELETE FROM event_lifecycles
		WHERE updated_at < ? AND current_state IN ('processed', 'failed', 'archived', 'expired')
	`, cutoff)
	if err != nil {
		return 0, fmt.Errorf("failed to delete lifecycles: %w", err)
	}

	affected, _ := result.RowsAffected()
	return affected, nil
}

// Close closes the tracker
func (t *SQLiteLifecycleTracker) Close() error {
	t.cancel()
	t.wg.Wait()
	return t.db.Close()
}

// cleanupLoop runs periodic cleanup
func (t *SQLiteLifecycleTracker) cleanupLoop() {
	defer t.wg.Done()

	ticker := time.NewTicker(t.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.ctx.Done():
			return
		case <-ticker.C:
			t.Cleanup(t.ctx, t.config.RetentionPeriod)
		}
	}
}

// LifecycleMiddleware creates middleware that tracks event lifecycle
type LifecycleMiddleware struct {
	tracker   LifecycleTracker
	component string
}

// NewLifecycleMiddleware creates a new lifecycle middleware
func NewLifecycleMiddleware(tracker LifecycleTracker, component string) *LifecycleMiddleware {
	return &LifecycleMiddleware{
		tracker:   tracker,
		component: component,
	}
}

// TrackCreated tracks an event creation
func (m *LifecycleMiddleware) TrackCreated(ctx context.Context, event *Event) error {
	return m.tracker.Track(ctx, &LifecycleTransition{
		EventID:   event.ID,
		ToState:   LifecycleStateCreated,
		Timestamp: event.Time,
		Component: m.component,
		Details: map[string]interface{}{
			"event_type": string(event.Type),
			"source":     event.Source,
			"severity":   string(event.Severity),
		},
	})
}

// TrackPublished tracks an event publication
func (m *LifecycleMiddleware) TrackPublished(ctx context.Context, eventID string) error {
	return m.tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStatePublished,
		Timestamp: time.Now(),
		Component: m.component,
	})
}

// TrackRouted tracks an event being routed
func (m *LifecycleMiddleware) TrackRouted(ctx context.Context, eventID string, handlers int) error {
	return m.tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStateRouted,
		Timestamp: time.Now(),
		Component: m.component,
		Details: map[string]interface{}{
			"handlers": handlers,
		},
	})
}

// TrackProcessing tracks an event starting processing
func (m *LifecycleMiddleware) TrackProcessing(ctx context.Context, eventID string, handler string) error {
	return m.tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStateProcessing,
		Timestamp: time.Now(),
		Component: m.component,
		Details: map[string]interface{}{
			"handler": handler,
		},
	})
}

// TrackProcessed tracks an event being successfully processed
func (m *LifecycleMiddleware) TrackProcessed(ctx context.Context, eventID string) error {
	return m.tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStateProcessed,
		Timestamp: time.Now(),
		Component: m.component,
	})
}

// TrackFailed tracks an event processing failure
func (m *LifecycleMiddleware) TrackFailed(ctx context.Context, eventID string, err error) error {
	details := map[string]interface{}{}
	if err != nil {
		details["error"] = err.Error()
	}
	return m.tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStateFailed,
		Timestamp: time.Now(),
		Component: m.component,
		Details:   details,
	})
}

// TrackArchived tracks an event being archived
func (m *LifecycleMiddleware) TrackArchived(ctx context.Context, eventID string, destination string) error {
	return m.tracker.Track(ctx, &LifecycleTransition{
		EventID:   eventID,
		ToState:   LifecycleStateArchived,
		Timestamp: time.Now(),
		Component: m.component,
		Details: map[string]interface{}{
			"destination": destination,
		},
	})
}
