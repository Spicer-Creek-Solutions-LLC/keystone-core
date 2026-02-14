package history

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"
)

func TestNewSQLiteStore(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	defer store.Close()
}

func TestSaveAndListRuns(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	run := &StateRunRecord{
		RunID:         "run-1",
		AgentID:       "agent-1",
		StateFiles:    []string{"/etc/state/web.yaml"},
		Target:        "os:linux",
		DryRun:        false,
		Success:       true,
		Total:         5,
		Succeeded:     4,
		Failed:        1,
		Changed:       3,
		Unchanged:     2,
		StartTime:     now,
		EndTime:       now.Add(10 * time.Second),
		DurationMs:    10000,
		CorrelationID: "corr-1",
		User:          "admin",
	}

	if err := store.SaveRun(ctx, run); err != nil {
		t.Fatalf("SaveRun: %v", err)
	}

	runs, nextToken, err := store.ListRuns(ctx, nil)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected 1 run, got %d", len(runs))
	}
	if nextToken != "" {
		t.Errorf("expected empty next token, got %q", nextToken)
	}

	got := runs[0]
	if got.RunID != "run-1" {
		t.Errorf("RunID = %q, want %q", got.RunID, "run-1")
	}
	if got.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", got.AgentID, "agent-1")
	}
	if !got.Success {
		t.Error("expected Success = true")
	}
	if got.Total != 5 {
		t.Errorf("Total = %d, want 5", got.Total)
	}
	if got.Failed != 1 {
		t.Errorf("Failed = %d, want 1", got.Failed)
	}
	if len(got.StateFiles) != 1 || got.StateFiles[0] != "/etc/state/web.yaml" {
		t.Errorf("StateFiles = %v, want [/etc/state/web.yaml]", got.StateFiles)
	}
}

func TestListRuns_Filters(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	runs := []*StateRunRecord{
		{RunID: "r1", AgentID: "a1", Success: true, StartTime: now, EndTime: now.Add(time.Second)},
		{RunID: "r2", AgentID: "a1", Success: false, StartTime: now.Add(time.Minute), EndTime: now.Add(2 * time.Minute)},
		{RunID: "r3", AgentID: "a2", Success: true, StartTime: now.Add(2 * time.Minute), EndTime: now.Add(3 * time.Minute)},
	}
	for _, r := range runs {
		if err := store.SaveRun(ctx, r); err != nil {
			t.Fatalf("SaveRun(%s): %v", r.RunID, err)
		}
	}

	// Filter by agent
	got, _, err := store.ListRuns(ctx, &ListFilter{AgentID: "a1"})
	if err != nil {
		t.Fatalf("ListRuns agent filter: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("agent filter: expected 2 runs, got %d", len(got))
	}

	// Filter by success
	success := true
	got, _, err = store.ListRuns(ctx, &ListFilter{Success: &success})
	if err != nil {
		t.Fatalf("ListRuns success filter: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("success filter: expected 2 runs, got %d", len(got))
	}

	failure := false
	got, _, err = store.ListRuns(ctx, &ListFilter{Success: &failure})
	if err != nil {
		t.Fatalf("ListRuns failure filter: %v", err)
	}
	if len(got) != 1 {
		t.Errorf("failure filter: expected 1 run, got %d", len(got))
	}
}

func TestListRuns_Pagination(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	for i := 0; i < 5; i++ {
		store.SaveRun(ctx, &StateRunRecord{
			RunID:     fmt.Sprintf("r%d", i),
			AgentID:   "a1",
			StartTime: now.Add(time.Duration(i) * time.Minute),
			EndTime:   now.Add(time.Duration(i+1) * time.Minute),
		})
	}

	// Page 1: size 2
	page1, token1, err := store.ListRuns(ctx, &ListFilter{PageSize: 2})
	if err != nil {
		t.Fatalf("page 1: %v", err)
	}
	if len(page1) != 2 {
		t.Fatalf("page 1: expected 2 runs, got %d", len(page1))
	}
	if token1 == "" {
		t.Fatal("expected non-empty next page token")
	}

	// Page 2: continue with token
	page2, token2, err := store.ListRuns(ctx, &ListFilter{PageSize: 2, PageToken: token1})
	if err != nil {
		t.Fatalf("page 2: %v", err)
	}
	if len(page2) != 2 {
		t.Fatalf("page 2: expected 2 runs, got %d", len(page2))
	}
	if token2 == "" {
		t.Fatal("expected non-empty token for page 2")
	}

	// Page 3: last page
	page3, token3, err := store.ListRuns(ctx, &ListFilter{PageSize: 2, PageToken: token2})
	if err != nil {
		t.Fatalf("page 3: %v", err)
	}
	if len(page3) != 1 {
		t.Fatalf("page 3: expected 1 run, got %d", len(page3))
	}
	if token3 != "" {
		t.Errorf("expected empty token for last page, got %q", token3)
	}
}

func TestSaveAndGetStatus(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	status := &StateStatusRecord{
		AgentID:      "agent-1",
		StateID:      "pkg_nginx",
		Module:       "pkg",
		CurrentState: "installed",
		DesiredState: "installed",
		Compliant:    true,
		LastApplied:  now,
		LastChecked:  now,
	}

	if err := store.SaveStatus(ctx, status); err != nil {
		t.Fatalf("SaveStatus: %v", err)
	}

	records, err := store.GetStatus(ctx, "agent-1", "")
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 status, got %d", len(records))
	}

	got := records[0]
	if got.StateID != "pkg_nginx" {
		t.Errorf("StateID = %q, want %q", got.StateID, "pkg_nginx")
	}
	if !got.Compliant {
		t.Error("expected Compliant = true")
	}
}

func TestSaveStatus_Upsert(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	status := &StateStatusRecord{
		AgentID:      "agent-1",
		StateID:      "pkg_nginx",
		Module:       "pkg",
		CurrentState: "installed",
		DesiredState: "installed",
		Compliant:    true,
		LastApplied:  now,
		LastChecked:  now,
	}
	store.SaveStatus(ctx, status)

	// Update to non-compliant
	status.CurrentState = "absent"
	status.Compliant = false
	status.LastChecked = now.Add(time.Minute)
	store.SaveStatus(ctx, status)

	records, _ := store.GetStatus(ctx, "agent-1", "")
	if len(records) != 1 {
		t.Fatalf("expected 1 status after upsert, got %d", len(records))
	}
	if records[0].Compliant {
		t.Error("expected Compliant = false after upsert")
	}
	if records[0].CurrentState != "absent" {
		t.Errorf("CurrentState = %q, want %q", records[0].CurrentState, "absent")
	}
}

func TestGetStatus_FilterByStatePath(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()
	ctx := context.Background()

	now := time.Now().Truncate(time.Second)
	store.SaveStatus(ctx, &StateStatusRecord{AgentID: "a1", StateID: "pkg_nginx", Module: "pkg", LastApplied: now, LastChecked: now})
	store.SaveStatus(ctx, &StateStatusRecord{AgentID: "a1", StateID: "file_config", Module: "file", LastApplied: now, LastChecked: now})

	records, _ := store.GetStatus(ctx, "a1", "pkg")
	if len(records) != 1 {
		t.Fatalf("expected 1 status with path filter, got %d", len(records))
	}
	if records[0].StateID != "pkg_nginx" {
		t.Errorf("StateID = %q, want %q", records[0].StateID, "pkg_nginx")
	}
}

func TestGetStatus_NoResults(t *testing.T) {
	store := newTestStore(t)
	defer store.Close()

	records, err := store.GetStatus(context.Background(), "nonexistent", "")
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if len(records) != 0 {
		t.Errorf("expected 0 records, got %d", len(records))
	}
}

func TestPageTokenRoundTrip(t *testing.T) {
	for _, offset := range []int{1, 10, 50, 100} {
		token := encodePageToken(offset)
		got := parsePageToken(token)
		if got != offset {
			t.Errorf("round trip failed: offset=%d, token=%q, got=%d", offset, token, got)
		}
	}
}

func TestPageToken_EdgeCases(t *testing.T) {
	if v := parsePageToken(""); v != 0 {
		t.Errorf("empty token: got %d, want 0", v)
	}
	if v := parsePageToken("invalid!!!"); v != 0 {
		t.Errorf("invalid base64: got %d, want 0", v)
	}
	if v := encodePageToken(0); v != "" {
		t.Errorf("zero offset: got %q, want empty", v)
	}
	if v := encodePageToken(-1); v != "" {
		t.Errorf("negative offset: got %q, want empty", v)
	}
}

func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	return store
}
