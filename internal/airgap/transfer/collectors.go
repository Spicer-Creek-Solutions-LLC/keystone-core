package transfer

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/statemgmt/history"
)

// CollectResult holds statistics from a data collection operation.
type CollectResult struct {
	Count        int64
	BytesWritten int64
	TimeRange    TimeRange
}

// CollectOptions controls what data to collect.
type CollectOptions struct {
	TimeRange *TimeRange
	Limit     int // 0 = no limit
}

// DataCollector collects data from a source and writes JSONL to a writer.
type DataCollector interface {
	Name() string
	Collect(ctx context.Context, w io.Writer, opts *CollectOptions) (*CollectResult, error)
}

// EventCollector collects events from an EventStore.
type EventCollector struct {
	store events.EventStore
}

// NewEventCollector creates a new event collector.
func NewEventCollector(store events.EventStore) *EventCollector {
	return &EventCollector{store: store}
}

// Name returns the dataset name.
func (c *EventCollector) Name() string { return "events" }

// Collect queries events and writes them as JSONL.
func (c *EventCollector) Collect(ctx context.Context, w io.Writer, opts *CollectOptions) (*CollectResult, error) {
	result := &CollectResult{}
	enc := json.NewEncoder(w)

	const batchSize = 500
	offset := 0
	var earliest, latest time.Time
	firstRecord := true

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		q := events.NewEventQuery().
			WithSort("time", "asc").
			WithPagination(batchSize, offset)

		if opts != nil && opts.TimeRange != nil {
			q = q.WithTimeRange(opts.TimeRange.Start, opts.TimeRange.End)
		}

		qr, err := c.store.Query(ctx, q)
		if err != nil {
			return nil, fmt.Errorf("querying events: %w", err)
		}
		if len(qr.Events) == 0 {
			break
		}

		for _, evt := range qr.Events {
			if err := enc.Encode(evt); err != nil {
				return nil, fmt.Errorf("encoding event: %w", err)
			}
			result.Count++
			if firstRecord {
				earliest = evt.Time
				firstRecord = false
			}
			latest = evt.Time
		}

		offset += len(qr.Events)
		if opts != nil && opts.Limit > 0 && int(result.Count) >= opts.Limit {
			break
		}
		if len(qr.Events) < batchSize {
			break
		}
	}

	if result.Count > 0 {
		result.TimeRange = TimeRange{Start: earliest, End: latest}
	}
	return result, nil
}

// AuditEntry mirrors the audit entry structure for JSON serialization.
type AuditEntry struct {
	Timestamp time.Time `json:"timestamp"`
	AuditType string    `json:"audit_type"`
	User      string    `json:"user"`
	Tool      string    `json:"tool"`
	Command   string    `json:"command"`
	Result    string    `json:"result"`
	Extra     any       `json:"extra,omitempty"`
}

// AuditQueryFunc retrieves audit entries matching the given options.
type AuditQueryFunc func(ctx context.Context, opts *CollectOptions) ([]*AuditEntry, error)

// AuditCollector collects audit log entries.
type AuditCollector struct {
	queryFn AuditQueryFunc
}

// NewAuditCollector creates a new audit collector using the provided query function.
func NewAuditCollector(fn AuditQueryFunc) *AuditCollector {
	return &AuditCollector{queryFn: fn}
}

// Name returns the dataset name.
func (c *AuditCollector) Name() string { return "audit" }

// Collect queries audit entries and writes them as JSONL.
func (c *AuditCollector) Collect(ctx context.Context, w io.Writer, opts *CollectOptions) (*CollectResult, error) {
	entries, err := c.queryFn(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("querying audit entries: %w", err)
	}

	result := &CollectResult{}
	enc := json.NewEncoder(w)
	var earliest, latest time.Time
	firstRecord := true

	for _, entry := range entries {
		if err := enc.Encode(entry); err != nil {
			return nil, fmt.Errorf("encoding audit entry: %w", err)
		}
		result.Count++
		if firstRecord {
			earliest = entry.Timestamp
			firstRecord = false
		}
		latest = entry.Timestamp
	}

	if result.Count > 0 {
		result.TimeRange = TimeRange{Start: earliest, End: latest}
	}
	return result, nil
}

// StateCollector collects state run history.
type StateCollector struct {
	store history.Store
}

// NewStateCollector creates a new state history collector.
func NewStateCollector(store history.Store) *StateCollector {
	return &StateCollector{store: store}
}

// Name returns the dataset name.
func (c *StateCollector) Name() string { return "state" }

// Collect queries state runs and writes them as JSONL.
func (c *StateCollector) Collect(ctx context.Context, w io.Writer, opts *CollectOptions) (*CollectResult, error) {
	result := &CollectResult{}
	enc := json.NewEncoder(w)
	var earliest, latest time.Time
	firstRecord := true

	const pageSize = 500
	pageToken := ""

	for {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		filter := &history.ListFilter{
			PageSize:  pageSize,
			PageToken: pageToken,
		}
		if opts != nil && opts.TimeRange != nil {
			filter.StartTime = &opts.TimeRange.Start
			filter.EndTime = &opts.TimeRange.End
		}

		runs, nextToken, err := c.store.ListRuns(ctx, filter)
		if err != nil {
			return nil, fmt.Errorf("listing state runs: %w", err)
		}
		if len(runs) == 0 {
			break
		}

		for _, run := range runs {
			if err := enc.Encode(run); err != nil {
				return nil, fmt.Errorf("encoding state run: %w", err)
			}
			result.Count++
			if firstRecord {
				earliest = run.StartTime
				firstRecord = false
			}
			latest = run.StartTime
		}

		if nextToken == "" {
			break
		}
		pageToken = nextToken
		if opts != nil && opts.Limit > 0 && int(result.Count) >= opts.Limit {
			break
		}
	}

	if result.Count > 0 {
		result.TimeRange = TimeRange{Start: earliest, End: latest}
	}
	return result, nil
}
