package transfer

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/internal/statemgmt/history"
)

// writeTestFile is a helper for tests that need to create files.
func writeTestFile(path string, data []byte) error {
	//nolint:gosec // G306: test helper
	return os.WriteFile(path, data, 0o644)
}

// mockEventStore satisfies events.EventStore for testing.
type mockEventStore struct {
	events []*events.Event
}

func (m *mockEventStore) Store(context.Context, *events.Event) error   { return nil }
func (m *mockEventStore) StoreBatch(context.Context, []*events.Event) error { return nil }
func (m *mockEventStore) Get(context.Context, string) (*events.Event, error) {
	return nil, nil
}
func (m *mockEventStore) Delete(context.Context, string) error         { return nil }
func (m *mockEventStore) DeleteBatch(context.Context, []string) error  { return nil }
func (m *mockEventStore) Count(context.Context, *events.EventQuery) (int64, error) {
	return int64(len(m.events)), nil
}
func (m *mockEventStore) ApplyRetention(context.Context, *events.RetentionPolicy) (int64, error) {
	return 0, nil
}
func (m *mockEventStore) Close() error { return nil }

func (m *mockEventStore) Query(_ context.Context, q *events.EventQuery) (*events.EventQueryResult, error) {
	start := q.Offset
	if start >= len(m.events) {
		return &events.EventQueryResult{}, nil
	}
	end := start + q.Limit
	if end > len(m.events) {
		end = len(m.events)
	}

	// Apply time range filter
	var filtered []*events.Event
	for _, evt := range m.events[start:end] {
		if q.StartTime != nil && evt.Time.Before(*q.StartTime) {
			continue
		}
		if q.EndTime != nil && evt.Time.After(*q.EndTime) {
			continue
		}
		filtered = append(filtered, evt)
	}

	return &events.EventQueryResult{
		Events:     filtered,
		TotalCount: int64(len(m.events)),
		Offset:     q.Offset,
		Limit:      q.Limit,
	}, nil
}

func makeTestEvents(n int) []*events.Event {
	base := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	result := make([]*events.Event, n)
	for i := 0; i < n; i++ {
		result[i] = &events.Event{
			ID:       "evt-" + string(rune('a'+i)),
			Type:     events.EventTypeAgentConnect,
			Source:   "test",
			Time:     base.Add(time.Duration(i) * time.Minute),
			Severity: events.SeverityInfo,
		}
	}
	return result
}

func TestEventCollector_Collect(t *testing.T) {
	store := &mockEventStore{events: makeTestEvents(5)}
	c := NewEventCollector(store)

	if c.Name() != "events" {
		t.Errorf("Name() = %q, want %q", c.Name(), "events")
	}

	var buf bytes.Buffer
	result, err := c.Collect(context.Background(), &buf, nil)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}

	if result.Count != 5 {
		t.Errorf("Count = %d, want 5", result.Count)
	}

	// Verify JSONL output
	lines := countLines(t, buf.Bytes())
	if lines != 5 {
		t.Errorf("JSONL lines = %d, want 5", lines)
	}
}

func TestEventCollector_Collect_Empty(t *testing.T) {
	store := &mockEventStore{}
	c := NewEventCollector(store)

	var buf bytes.Buffer
	result, err := c.Collect(context.Background(), &buf, nil)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if result.Count != 0 {
		t.Errorf("Count = %d, want 0", result.Count)
	}
}

func TestEventCollector_Collect_WithTimeRange(t *testing.T) {
	evts := makeTestEvents(10)
	store := &mockEventStore{events: evts}
	c := NewEventCollector(store)

	start := evts[2].Time
	end := evts[5].Time
	opts := &CollectOptions{
		TimeRange: &TimeRange{Start: start, End: end},
	}

	var buf bytes.Buffer
	result, err := c.Collect(context.Background(), &buf, opts)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if result.Count != 4 {
		t.Errorf("Count = %d, want 4 (events at indices 2-5)", result.Count)
	}
}

func TestEventCollector_Collect_ContextCancelled(t *testing.T) {
	store := &mockEventStore{events: makeTestEvents(5)}
	c := NewEventCollector(store)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	var buf bytes.Buffer
	_, err := c.Collect(ctx, &buf, nil)
	if err == nil {
		t.Error("expected error for cancelled context")
	}
}

func TestAuditCollector_Collect(t *testing.T) {
	entries := []*AuditEntry{
		{Timestamp: time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC), AuditType: "command_executed", User: "admin", Tool: "kscorectl", Command: "state apply", Result: "success"},
		{Timestamp: time.Date(2024, 1, 15, 11, 0, 0, 0, time.UTC), AuditType: "config_changed", User: "admin", Tool: "kscorectl", Command: "config set", Result: "success"},
	}

	queryFn := func(_ context.Context, _ *CollectOptions) ([]*AuditEntry, error) {
		return entries, nil
	}

	c := NewAuditCollector(queryFn)
	if c.Name() != "audit" {
		t.Errorf("Name() = %q, want %q", c.Name(), "audit")
	}

	var buf bytes.Buffer
	result, err := c.Collect(context.Background(), &buf, nil)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if result.Count != 2 {
		t.Errorf("Count = %d, want 2", result.Count)
	}

	lines := countLines(t, buf.Bytes())
	if lines != 2 {
		t.Errorf("JSONL lines = %d, want 2", lines)
	}

	// Verify first line is valid JSON
	scanner := bufio.NewScanner(bytes.NewReader(buf.Bytes()))
	scanner.Scan()
	var entry AuditEntry
	if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
		t.Fatalf("invalid JSONL: %v", err)
	}
	if entry.User != "admin" {
		t.Errorf("User = %q, want %q", entry.User, "admin")
	}
}

func TestAuditCollector_Collect_Empty(t *testing.T) {
	queryFn := func(_ context.Context, _ *CollectOptions) ([]*AuditEntry, error) {
		return nil, nil
	}

	c := NewAuditCollector(queryFn)
	var buf bytes.Buffer
	result, err := c.Collect(context.Background(), &buf, nil)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if result.Count != 0 {
		t.Errorf("Count = %d, want 0", result.Count)
	}
}

// mockStateStore satisfies history.Store for testing.
type mockStateStore struct {
	runs []*history.StateRunRecord
}

func (m *mockStateStore) SaveRun(context.Context, *history.StateRunRecord) error {
	return nil
}
func (m *mockStateStore) GetStatus(context.Context, string, string) ([]*history.StateStatusRecord, error) {
	return nil, nil
}
func (m *mockStateStore) SaveStatus(context.Context, *history.StateStatusRecord) error {
	return nil
}
func (m *mockStateStore) Close() error { return nil }

func (m *mockStateStore) ListRuns(_ context.Context, filter *history.ListFilter) ([]*history.StateRunRecord, string, error) {
	start := 0
	// Simple page token: empty = start, otherwise skip that many
	if filter.PageToken != "" {
		for i, r := range m.runs {
			if r.RunID == filter.PageToken {
				start = i
				break
			}
		}
	}

	end := start + filter.PageSize
	if end > len(m.runs) {
		end = len(m.runs)
	}

	var filtered []*history.StateRunRecord
	for _, r := range m.runs[start:end] {
		if filter.StartTime != nil && r.StartTime.Before(*filter.StartTime) {
			continue
		}
		if filter.EndTime != nil && r.StartTime.After(*filter.EndTime) {
			continue
		}
		filtered = append(filtered, r)
	}

	nextToken := ""
	if end < len(m.runs) {
		nextToken = m.runs[end].RunID
	}

	return filtered, nextToken, nil
}

func makeTestRuns(n int) []*history.StateRunRecord {
	base := time.Date(2024, 1, 15, 10, 0, 0, 0, time.UTC)
	result := make([]*history.StateRunRecord, n)
	for i := 0; i < n; i++ {
		result[i] = &history.StateRunRecord{
			RunID:     "run-" + string(rune('a'+i)),
			AgentID:   "agent-01",
			Success:   true,
			Total:     5,
			Succeeded: 5,
			StartTime: base.Add(time.Duration(i) * time.Hour),
			EndTime:   base.Add(time.Duration(i)*time.Hour + 30*time.Minute),
		}
	}
	return result
}

func TestStateCollector_Collect(t *testing.T) {
	store := &mockStateStore{runs: makeTestRuns(3)}
	c := NewStateCollector(store)

	if c.Name() != "state" {
		t.Errorf("Name() = %q, want %q", c.Name(), "state")
	}

	var buf bytes.Buffer
	result, err := c.Collect(context.Background(), &buf, nil)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if result.Count != 3 {
		t.Errorf("Count = %d, want 3", result.Count)
	}

	lines := countLines(t, buf.Bytes())
	if lines != 3 {
		t.Errorf("JSONL lines = %d, want 3", lines)
	}
}

func TestStateCollector_Collect_Empty(t *testing.T) {
	store := &mockStateStore{}
	c := NewStateCollector(store)

	var buf bytes.Buffer
	result, err := c.Collect(context.Background(), &buf, nil)
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if result.Count != 0 {
		t.Errorf("Count = %d, want 0", result.Count)
	}
}

func countLines(t *testing.T, data []byte) int {
	t.Helper()
	scanner := bufio.NewScanner(bytes.NewReader(data))
	count := 0
	for scanner.Scan() {
		if len(scanner.Bytes()) > 0 {
			count++
		}
	}
	return count
}
