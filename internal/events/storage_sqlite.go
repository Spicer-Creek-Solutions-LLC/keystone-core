package events

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// SQLiteEventStore implements EventStore using SQLite
type SQLiteEventStore struct {
	db     *sql.DB
	config *EventStoreConfig
	mu     sync.RWMutex

	// Auto-retention
	retentionCancel context.CancelFunc
	retentionWg     sync.WaitGroup
}

// NewSQLiteEventStore creates a new SQLite event store
func NewSQLiteEventStore(config *EventStoreConfig) (*SQLiteEventStore, error) {
	if config == nil {
		config = DefaultEventStoreConfig()
	}

	// Open database
	db, err := sql.Open("sqlite", config.Path)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(config.MaxOpenConns)
	db.SetMaxIdleConns(config.MaxIdleConns)

	store := &SQLiteEventStore{
		db:     db,
		config: config,
	}

	// Initialize schema
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Start auto-retention if enabled
	if config.AutoRetention && config.DefaultRetentionPolicy != nil {
		store.startAutoRetention()
	}

	return store, nil
}

// initSchema initializes the database schema
func (s *SQLiteEventStore) initSchema() error {
	schema := `
	CREATE TABLE IF NOT EXISTS events (
		id TEXT PRIMARY KEY,
		type TEXT NOT NULL,
		source TEXT NOT NULL,
		severity TEXT NOT NULL,
		correlation_id TEXT,
		subject TEXT,
		time INTEGER NOT NULL,
		tags TEXT,
		data TEXT,
		created_at INTEGER NOT NULL
	);

	CREATE INDEX IF NOT EXISTS idx_events_type ON events(type);
	CREATE INDEX IF NOT EXISTS idx_events_source ON events(source);
	CREATE INDEX IF NOT EXISTS idx_events_severity ON events(severity);
	CREATE INDEX IF NOT EXISTS idx_events_time ON events(time);
	CREATE INDEX IF NOT EXISTS idx_events_correlation_id ON events(correlation_id);
	CREATE INDEX IF NOT EXISTS idx_events_created_at ON events(created_at);
	`

	_, err := s.db.Exec(schema)
	return err
}

// Store stores a single event
func (s *SQLiteEventStore) Store(ctx context.Context, event *Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Serialize tags and data
	tagsJSON, err := json.Marshal(event.Tags)
	if err != nil {
		return fmt.Errorf("failed to marshal tags: %w", err)
	}

	dataJSON, err := json.Marshal(event.Data)
	if err != nil {
		return fmt.Errorf("failed to marshal data: %w", err)
	}

	query := `
		INSERT INTO events (id, type, source, severity, correlation_id, subject, time, tags, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`

	_, err = s.db.ExecContext(ctx, query,
		event.ID,
		event.Type,
		event.Source,
		event.Severity,
		event.CorrelationID,
		event.Subject,
		event.Time.Unix(),
		string(tagsJSON),
		string(dataJSON),
		time.Now().Unix(),
	)

	if err != nil {
		return fmt.Errorf("failed to store event: %w", err)
	}

	return nil
}

// StoreBatch stores multiple events
func (s *SQLiteEventStore) StoreBatch(ctx context.Context, events []*Event) error {
	if len(events) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		INSERT INTO events (id, type, source, severity, correlation_id, subject, time, tags, data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("failed to prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, event := range events {
		tagsJSON, _ := json.Marshal(event.Tags)
		dataJSON, _ := json.Marshal(event.Data)

		_, err = stmt.ExecContext(ctx,
			event.ID,
			event.Type,
			event.Source,
			event.Severity,
			event.CorrelationID,
			event.Subject,
			event.Time.Unix(),
			string(tagsJSON),
			string(dataJSON),
			time.Now().Unix(),
		)

		if err != nil {
			return fmt.Errorf("failed to store event %s: %w", event.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	return nil
}

// Get retrieves an event by ID
func (s *SQLiteEventStore) Get(ctx context.Context, id string) (*Event, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := `
		SELECT id, type, source, severity, correlation_id, subject, time, tags, data
		FROM events
		WHERE id = ?
	`

	var event Event
	var timeUnix int64
	var tagsJSON, dataJSON string

	err := s.db.QueryRowContext(ctx, query, id).Scan(
		&event.ID,
		&event.Type,
		&event.Source,
		&event.Severity,
		&event.CorrelationID,
		&event.Subject,
		&timeUnix,
		&tagsJSON,
		&dataJSON,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get event: %w", err)
	}

	event.Time = time.Unix(timeUnix, 0)

	if err := json.Unmarshal([]byte(tagsJSON), &event.Tags); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tags: %w", err)
	}

	if err := json.Unmarshal([]byte(dataJSON), &event.Data); err != nil {
		return nil, fmt.Errorf("failed to unmarshal data: %w", err)
	}

	return &event, nil
}

// Query queries events with filters
func (s *SQLiteEventStore) Query(ctx context.Context, query *EventQuery) (*EventQueryResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build WHERE clause
	var conditions []string
	var args []interface{}

	if len(query.Types) > 0 {
		placeholders := make([]string, len(query.Types))
		for i, t := range query.Types {
			placeholders[i] = "?"
			args = append(args, t)
		}
		conditions = append(conditions, fmt.Sprintf("type IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(query.Sources) > 0 {
		placeholders := make([]string, len(query.Sources))
		for i, s := range query.Sources {
			placeholders[i] = "?"
			args = append(args, s)
		}
		conditions = append(conditions, fmt.Sprintf("source IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(query.Severities) > 0 {
		placeholders := make([]string, len(query.Severities))
		for i, sev := range query.Severities {
			placeholders[i] = "?"
			args = append(args, sev)
		}
		conditions = append(conditions, fmt.Sprintf("severity IN (%s)", strings.Join(placeholders, ",")))
	}

	if query.CorrelationID != "" {
		conditions = append(conditions, "correlation_id = ?")
		args = append(args, query.CorrelationID)
	}

	if query.StartTime != nil {
		conditions = append(conditions, "time >= ?")
		args = append(args, query.StartTime.Unix())
	}

	if query.EndTime != nil {
		conditions = append(conditions, "time <= ?")
		args = append(args, query.EndTime.Unix())
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count
	countQuery := "SELECT COUNT(*) FROM events " + whereClause // nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- whereClause built from whitelisted fields with placeholders; args are bound separately
	var totalCount int64
	err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount)
	if err != nil {
		return nil, fmt.Errorf("failed to count events: %w", err)
	}

	// Build ORDER BY clause
	sortBy := query.SortBy
	if sortBy == "" {
		sortBy = "time"
	}
	sortOrder := strings.ToLower(query.SortOrder)
	if sortOrder != "asc" {
		sortOrder = "desc"
	}

	validSortColumns := map[string]string{
		"time":           "time",
		"type":           "type",
		"source":         "source",
		"severity":       "severity",
		"correlation_id": "correlation_id",
		"subject":        "subject",
		"id":             "id",
	}
	if column, ok := validSortColumns[sortBy]; ok {
		sortBy = column
	} else {
		sortBy = "time"
	}

	// Get events
	dataQuery := `
		SELECT id, type, source, severity, correlation_id, subject, time, tags, data
		FROM events
	` + whereClause + " ORDER BY " + sortBy + " " + strings.ToUpper(sortOrder) + " LIMIT ? OFFSET ?"

	args = append(args, query.Limit, query.Offset)

	rows, err := s.db.QueryContext(ctx, dataQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []*Event
	for rows.Next() {
		var event Event
		var timeUnix int64
		var tagsJSON, dataJSON string

		err := rows.Scan(
			&event.ID,
			&event.Type,
			&event.Source,
			&event.Severity,
			&event.CorrelationID,
			&event.Subject,
			&timeUnix,
			&tagsJSON,
			&dataJSON,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		event.Time = time.Unix(timeUnix, 0)
		json.Unmarshal([]byte(tagsJSON), &event.Tags)
		json.Unmarshal([]byte(dataJSON), &event.Data)

		events = append(events, &event)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate events: %w", err)
	}

	return &EventQueryResult{
		Events:     events,
		TotalCount: totalCount,
		Offset:     query.Offset,
		Limit:      query.Limit,
	}, nil
}

// Count returns the total number of events matching the query
func (s *SQLiteEventStore) Count(ctx context.Context, query *EventQuery) (int64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Build WHERE clause (same as Query)
	var conditions []string
	var args []interface{}

	if len(query.Types) > 0 {
		placeholders := make([]string, len(query.Types))
		for i, t := range query.Types {
			placeholders[i] = "?"
			args = append(args, t)
		}
		conditions = append(conditions, fmt.Sprintf("type IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(query.Sources) > 0 {
		placeholders := make([]string, len(query.Sources))
		for i, s := range query.Sources {
			placeholders[i] = "?"
			args = append(args, s)
		}
		conditions = append(conditions, fmt.Sprintf("source IN (%s)", strings.Join(placeholders, ",")))
	}

	if query.StartTime != nil {
		conditions = append(conditions, "time >= ?")
		args = append(args, query.StartTime.Unix())
	}

	if query.EndTime != nil {
		conditions = append(conditions, "time <= ?")
		args = append(args, query.EndTime.Unix())
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	countQuery := "SELECT COUNT(*) FROM events " + whereClause // nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- whereClause built from whitelisted fields with placeholders; args are bound separately
	var count int64
	err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("failed to count events: %w", err)
	}

	return count, nil
}

// Delete deletes an event by ID
func (s *SQLiteEventStore) Delete(ctx context.Context, id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete event: %w", err)
	}

	return nil
}

// DeleteBatch deletes multiple events
func (s *SQLiteEventStore) DeleteBatch(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	placeholders := make([]string, len(ids))
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		placeholders[i] = "?"
		args[i] = id
	}

	query := "DELETE FROM events WHERE id IN (" + strings.Join(placeholders, ",") + ")" // nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- placeholders are generated for bound args only
	_, err := s.db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("failed to delete events: %w", err)
	}

	return nil
}

// ApplyRetention applies retention policy and returns number of deleted events
func (s *SQLiteEventStore) ApplyRetention(ctx context.Context, policy *RetentionPolicy) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	var totalDeleted int64

	// Delete events older than MaxAge
	if policy.MaxAge > 0 {
		cutoffTime := time.Now().Add(-policy.MaxAge).Unix()
		result, err := s.db.ExecContext(ctx, "DELETE FROM events WHERE time < ?", cutoffTime)
		if err != nil {
			return 0, fmt.Errorf("failed to delete old events: %w", err)
		}
		deleted, _ := result.RowsAffected()
		totalDeleted += deleted
	}

	// Delete events exceeding MaxCount (keep most recent)
	if policy.MaxCount > 0 {
		var count int64
		err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&count)
		if err != nil {
			return totalDeleted, fmt.Errorf("failed to count events: %w", err)
		}

		if count > policy.MaxCount {
			toDelete := count - policy.MaxCount
			query := `
				DELETE FROM events WHERE id IN (
					SELECT id FROM events ORDER BY created_at ASC, id ASC LIMIT ?
				)
			`
			result, err := s.db.ExecContext(ctx, query, toDelete)
			if err != nil {
				return totalDeleted, fmt.Errorf("failed to delete excess events: %w", err)
			}
			deleted, _ := result.RowsAffected()
			totalDeleted += deleted
		}
	}

	// Delete events below MinSeverity
	if policy.MinSeverity != "" {
		severityOrder := map[Severity]int{
			SeverityDebug:    0,
			SeverityInfo:     1,
			SeverityWarning:  2,
			SeverityError:    3,
			SeverityCritical: 4,
		}

		minLevel := severityOrder[policy.MinSeverity]
		var toDelete []Severity
		for sev, level := range severityOrder {
			if level < minLevel {
				toDelete = append(toDelete, sev)
			}
		}

		if len(toDelete) > 0 {
			placeholders := make([]string, len(toDelete))
			args := make([]interface{}, len(toDelete))
			for i, sev := range toDelete {
				placeholders[i] = "?"
				args[i] = sev
			}

			query := "DELETE FROM events WHERE severity IN (" + strings.Join(placeholders, ",") + ")" // nosemgrep: go.lang.security.audit.database.string-formatted-query.string-formatted-query -- placeholders are generated for bound args only
			result, err := s.db.ExecContext(ctx, query, args...)
			if err != nil {
				return totalDeleted, fmt.Errorf("failed to delete low severity events: %w", err)
			}
			deleted, _ := result.RowsAffected()
			totalDeleted += deleted
		}
	}

	return totalDeleted, nil
}

// GetMetrics returns storage metrics
func (s *SQLiteEventStore) GetMetrics(ctx context.Context) (*StorageMetrics, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	metrics := &StorageMetrics{
		EventsByType:     make(map[EventType]int64),
		EventsBySeverity: make(map[Severity]int64),
	}

	// Total events
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM events").Scan(&metrics.TotalEvents)
	if err != nil {
		return nil, fmt.Errorf("failed to count total events: %w", err)
	}

	// Events by type
	rows, err := s.db.QueryContext(ctx, "SELECT type, COUNT(*) FROM events GROUP BY type")
	if err != nil {
		return nil, fmt.Errorf("failed to count by type: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var eventType EventType
		var count int64
		if err := rows.Scan(&eventType, &count); err != nil {
			return nil, err
		}
		metrics.EventsByType[eventType] = count
	}

	// Events by severity
	rows, err = s.db.QueryContext(ctx, "SELECT severity, COUNT(*) FROM events GROUP BY severity")
	if err != nil {
		return nil, fmt.Errorf("failed to count by severity: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var severity Severity
		var count int64
		if err := rows.Scan(&severity, &count); err != nil {
			return nil, err
		}
		metrics.EventsBySeverity[severity] = count
	}

	// Oldest and newest events
	var oldestUnix, newestUnix int64
	err = s.db.QueryRowContext(ctx, "SELECT MIN(time), MAX(time) FROM events").Scan(&oldestUnix, &newestUnix)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to get time range: %w", err)
	}

	if oldestUnix > 0 {
		metrics.OldestEvent = time.Unix(oldestUnix, 0)
		metrics.NewestEvent = time.Unix(newestUnix, 0)
	}

	// Database size
	if info, err := os.Stat(s.config.Path); err == nil {
		metrics.DatabaseSize = info.Size()
	}

	return metrics, nil
}

// startAutoRetention starts automatic retention policy application
func (s *SQLiteEventStore) startAutoRetention() {
	ctx, cancel := context.WithCancel(context.Background())
	s.retentionCancel = cancel

	s.retentionWg.Add(1)
	go func() {
		defer s.retentionWg.Done()

		ticker := time.NewTicker(s.config.RetentionCheckInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				if s.config.DefaultRetentionPolicy != nil {
					s.ApplyRetention(ctx, s.config.DefaultRetentionPolicy)
				}
			case <-ctx.Done():
				return
			}
		}
	}()
}

// Close closes the event store
func (s *SQLiteEventStore) Close() error {
	// Stop auto-retention
	if s.retentionCancel != nil {
		s.retentionCancel()
		s.retentionWg.Wait()
	}

	return s.db.Close()
}

// Replay replays events matching the query
func (s *SQLiteEventStore) Replay(ctx context.Context, query *EventQuery, handler EventHandler) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Query events
	result, err := s.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to query events for replay: %w", err)
	}

	// Replay events
	for _, event := range result.Events {
		if err := handler(event); err != nil {
			return fmt.Errorf("replay handler failed for event %s: %w", event.ID, err)
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// ReplayFrom replays events from a specific time to now
// Events are replayed in chronological order (oldest first)
func (s *SQLiteEventStore) ReplayFrom(ctx context.Context, startTime time.Time, handler EventHandler) error {
	query := NewEventQuery().
		WithTimeRange(startTime, time.Now()).
		WithSort("time", "asc")

	return s.Replay(ctx, query, handler)
}

// ReplayRange replays events within a specific time range
// Events are replayed in chronological order (oldest first)
func (s *SQLiteEventStore) ReplayRange(ctx context.Context, startTime, endTime time.Time, handler EventHandler) error {
	if startTime.After(endTime) {
		return fmt.Errorf("start time %v is after end time %v", startTime, endTime)
	}

	query := NewEventQuery().
		WithTimeRange(startTime, endTime).
		WithSort("time", "asc")

	return s.Replay(ctx, query, handler)
}

// ReplayWithProgress replays events with progress tracking
func (s *SQLiteEventStore) ReplayWithProgress(ctx context.Context, query *EventQuery, handler EventHandler, progressFn ReplayProgressFunc) error {
	s.mu.RLock()

	// First get total count for progress tracking
	count, err := s.Count(ctx, query)
	if err != nil {
		s.mu.RUnlock()
		return fmt.Errorf("failed to count events for replay: %w", err)
	}

	// Query events
	result, err := s.Query(ctx, query)
	if err != nil {
		s.mu.RUnlock()
		return fmt.Errorf("failed to query events for replay: %w", err)
	}
	s.mu.RUnlock()

	// Replay events with progress updates
	for i, event := range result.Events {
		if err := handler(event); err != nil {
			return fmt.Errorf("replay handler failed for event %s: %w", event.ID, err)
		}

		// Report progress if callback provided
		if progressFn != nil {
			progress := &ReplayProgress{
				Current:    int64(i + 1),
				Total:      count,
				LastEvent:  event,
				Percentage: float64(i+1) / float64(count) * 100.0,
			}
			progressFn(progress)
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}

	return nil
}

// ReplayBatched replays events in batches for memory-efficient processing
func (s *SQLiteEventStore) ReplayBatched(ctx context.Context, query *EventQuery, batchSize int, handler BatchEventHandler) error {
	if batchSize <= 0 {
		batchSize = 100
	}

	offset := 0
	for {
		batchQuery := *query
		batchQuery.Limit = batchSize
		batchQuery.Offset = offset

		s.mu.RLock()
		result, err := s.Query(ctx, &batchQuery)
		s.mu.RUnlock()

		if err != nil {
			return fmt.Errorf("failed to query batch at offset %d: %w", offset, err)
		}

		if len(result.Events) == 0 {
			break // No more events
		}

		// Process batch
		if err := handler(result.Events); err != nil {
			return fmt.Errorf("batch handler failed at offset %d: %w", offset, err)
		}

		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// If we got fewer events than batch size, we're done
		if len(result.Events) < batchSize {
			break
		}

		offset += batchSize
	}

	return nil
}

// ReplayRangeWithTypes replays events of specific types within a time range
func (s *SQLiteEventStore) ReplayRangeWithTypes(ctx context.Context, startTime, endTime time.Time, types []EventType, handler EventHandler) error {
	if startTime.After(endTime) {
		return fmt.Errorf("start time %v is after end time %v", startTime, endTime)
	}

	query := NewEventQuery().
		WithTimeRange(startTime, endTime).
		WithTypes(types...).
		WithSort("time", "asc")

	return s.Replay(ctx, query, handler)
}
