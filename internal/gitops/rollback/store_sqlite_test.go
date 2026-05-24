// SPDX-License-Identifier: Apache-2.0

package rollback

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/pkg/dbutil"
)

func newSQLiteStore(t *testing.T) *SQLiteStore {
	t.Helper()
	s, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("NewSQLiteStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func sampleRollback(id string) *Rollback {
	ts := time.Date(2026, 5, 19, 12, 0, 0, 0, time.UTC)
	return &Rollback{
		ID:              id,
		Application:     "web",
		ExecutorType:    "git",
		Strategy:        StrategySpecific,
		Revision:        "c1",
		Reason:          "hotfix",
		RequireApproval: true,
		State:           StateCompleted,
		FromRevision:    "c2",
		ToRevision:      "c1",
		Approver:        "alice",
		Result: &Result{
			Success:      true,
			Message:      "git: reverted",
			FromRevision: "c2",
			ToRevision:   "c1",
			Data:         map[string]any{"new_commit": "c3"},
			Duration:     1500 * time.Millisecond,
			Error:        errors.New("ignored on success"),
		},
		Transitions: []TransitionRecord{
			{From: StatePending, To: StateApproved, Event: EventApprove, At: ts},
			{From: StateApproved, To: StateInProgress, Event: EventStart, At: ts},
		},
		Config:    Config{"repo_url": "https://example.com/repo.git", "branch": "main"},
		CreatedAt: ts,
		UpdatedAt: ts,
	}
}

func TestSQLiteStore_SaveGet(t *testing.T) {
	t.Parallel()
	s := newSQLiteStore(t)
	in := sampleRollback("rb-1")
	if err := s.Save(context.Background(), in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, ok, err := s.Get(context.Background(), "rb-1")
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.Application != "web" || got.Strategy != StrategySpecific || got.State != StateCompleted ||
		!got.RequireApproval || got.Approver != "alice" || got.ToRevision != "c1" {
		t.Errorf("scalar round-trip wrong: %+v", got)
	}
	if got.Result == nil || !got.Result.Success || got.Result.Duration != 1500*time.Millisecond ||
		got.Result.Data["new_commit"] != "c3" {
		t.Errorf("result round-trip wrong: %+v", got.Result)
	}
	// Lossy-error contract: rehydrated as a flat errors.New value.
	if got.Result.Error == nil || got.Result.Error.Error() != "ignored on success" {
		t.Errorf("result error round-trip = %v", got.Result.Error)
	}
	if len(got.Transitions) != 2 || got.Transitions[1].Event != EventStart {
		t.Errorf("transitions round-trip wrong: %+v", got.Transitions)
	}
	if got.Config["repo_url"] != "https://example.com/repo.git" || got.Config["branch"] != "main" {
		t.Errorf("config round-trip wrong: %+v", got.Config)
	}
	if !got.CreatedAt.Equal(in.CreatedAt) {
		t.Errorf("CreatedAt = %v, want %v", got.CreatedAt, in.CreatedAt)
	}
}

func TestSQLiteStore_Upsert_SeqStable(t *testing.T) {
	t.Parallel()
	s := newSQLiteStore(t)
	ctx := context.Background()
	_ = s.Save(ctx, sampleRollback("a"))
	_ = s.Save(ctx, sampleRollback("b"))

	upd := sampleRollback("a")
	upd.State = StateFailed
	upd.Error = "boom"
	if err := s.Save(ctx, upd); err != nil {
		t.Fatalf("re-Save: %v", err)
	}
	got, _, _ := s.Get(ctx, "a")
	if got.State != StateFailed || got.Error != "boom" {
		t.Errorf("upsert did not update: %+v", got)
	}
	list, err := s.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 || list[0].ID != "a" || list[1].ID != "b" {
		t.Errorf("seq order not stable across upsert: %v", []string{list[0].ID, list[1].ID})
	}
}

func TestSQLiteStore_NotFoundAndBadInput(t *testing.T) {
	t.Parallel()
	s := newSQLiteStore(t)
	rb, ok, err := s.Get(context.Background(), "missing")
	if rb != nil || ok || err != nil {
		t.Errorf("Get(missing) = %v,%v,%v want nil,false,nil", rb, ok, err)
	}
	if err := s.Save(context.Background(), nil); err == nil {
		t.Error("Save(nil) = nil, want error")
	}
	if err := s.Save(context.Background(), &Rollback{}); err == nil {
		t.Error("Save(empty id) = nil, want error")
	}
}

func TestSQLiteStore_PersistsAcrossReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "rb.db")
	s1, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open1: %v", err)
	}
	if err := s1.Save(context.Background(), sampleRollback("persist")); err != nil {
		t.Fatalf("Save: %v", err)
	}
	_ = s1.Close()

	s2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("open2: %v", err)
	}
	t.Cleanup(func() { _ = s2.Close() })
	got, ok, err := s2.Get(context.Background(), "persist")
	if err != nil || !ok || got.Application != "web" {
		t.Errorf("record did not survive reopen: ok=%v err=%v rb=%+v", ok, err, got)
	}
}

func TestSQLiteStore_NilResultRoundTrips(t *testing.T) {
	t.Parallel()
	s := newSQLiteStore(t)
	rb := sampleRollback("nores")
	rb.Result = nil
	rb.Transitions = nil
	if err := s.Save(context.Background(), rb); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, _, _ := s.Get(context.Background(), "nores")
	if got.Result != nil {
		t.Errorf("nil Result should stay nil, got %+v", got.Result)
	}
	if len(got.Transitions) != 0 {
		t.Errorf("nil Transitions should load empty, got %+v", got.Transitions)
	}
}

var _ RollbackStore = (*SQLiteStore)(nil)

func TestSQLiteStore_AddConfigColumnGuard(t *testing.T) {
	t.Parallel()
	// Open a DB with the pre-task-10 schema (no `config` column),
	// then re-open with NewSQLiteStore — the idempotent ALTER guard
	// must add the column so a record with Config saves + loads.
	path := filepath.Join(t.TempDir(), "preT10.db")

	{
		legacy, err := dbutil.OpenSQLite(path)
		if err != nil {
			t.Fatalf("open legacy: %v", err)
		}
		const preSchema = `
CREATE TABLE IF NOT EXISTS gitops_rollbacks (
	id               TEXT PRIMARY KEY,
	seq              INTEGER NOT NULL,
	application      TEXT NOT NULL DEFAULT '',
	executor_type    TEXT NOT NULL DEFAULT '',
	strategy         TEXT NOT NULL DEFAULT '',
	revision         TEXT NOT NULL DEFAULT '',
	reason           TEXT NOT NULL DEFAULT '',
	require_approval INTEGER NOT NULL DEFAULT 0,
	state            TEXT NOT NULL,
	from_revision    TEXT NOT NULL DEFAULT '',
	to_revision      TEXT NOT NULL DEFAULT '',
	approver         TEXT NOT NULL DEFAULT '',
	error            TEXT NOT NULL DEFAULT '',
	result           TEXT NOT NULL DEFAULT '',
	transitions      TEXT NOT NULL DEFAULT '[]',
	created_at       TEXT NOT NULL,
	updated_at       TEXT NOT NULL
);`
		if _, err := legacy.ExecContext(context.Background(), preSchema); err != nil {
			t.Fatalf("seed legacy schema: %v", err)
		}
		_ = legacy.Close()
	}

	s, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("re-open with migration: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Save(context.Background(), sampleRollback("after-alter")); err != nil {
		t.Fatalf("Save after ALTER: %v", err)
	}
	got, ok, _ := s.Get(context.Background(), "after-alter")
	if !ok || got.Config["repo_url"] != "https://example.com/repo.git" {
		t.Errorf("ALTER guard didn't take: ok=%v config=%+v", ok, got.Config)
	}

	// Idempotent: a second open must not re-ALTER or break.
	s2, err := NewSQLiteStore(path)
	if err != nil {
		t.Fatalf("third open: %v", err)
	}
	_ = s2.Close()
}
