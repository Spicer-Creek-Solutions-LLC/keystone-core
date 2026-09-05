// SPDX-License-Identifier: Apache-2.0

package audit_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
)

type recordingAuditor struct {
	mu      sync.Mutex
	entries []audit.AuditEntry
}

func (r *recordingAuditor) Emit(_ context.Context, e audit.AuditEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
}

func (r *recordingAuditor) get() []audit.AuditEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]audit.AuditEntry, len(r.entries))
	copy(out, r.entries)
	return out
}

type spyObserver struct {
	starts, drifts, changes, dones, skips int
}

func (s *spyObserver) Start(context.Context, *statemgmt.Declaration) { s.starts++ }
func (s *spyObserver) Drift(context.Context, *statemgmt.Declaration, *statemgmt.ModuleCheckResult) {
	s.drifts++
}
func (s *spyObserver) Change(context.Context, *statemgmt.Declaration, *statemgmt.StateResult) {
	s.changes++
}
func (s *spyObserver) Done(context.Context, *statemgmt.DeclarationResult)  { s.dones++ }
func (s *spyObserver) Skip(context.Context, *statemgmt.Declaration, error) { s.skips++ }

func TestStateApplyObserver_DoneEmitsAudit(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	obs := audit.NewStateApplyObserver(nil, rec)

	res := &statemgmt.DeclarationResult{
		DeclID:    "files:/etc/nginx",
		Module:    "file",
		Outcome:   statemgmt.OutcomeChanged,
		StartedAt: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		Duration:  150 * time.Millisecond,
		Apply:     &statemgmt.StateResult{Changed: true},
	}
	obs.Done(context.Background(), res)

	entries := rec.get()
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.Action != "state.apply" {
		t.Errorf("Action = %q", e.Action)
	}
	if e.ResourceType != "file" {
		t.Errorf("ResourceType = %q", e.ResourceType)
	}
	if !e.Allowed {
		t.Errorf("Allowed = false for non-failed outcome")
	}
	if e.Duration != 150*time.Millisecond {
		t.Errorf("Duration = %v", e.Duration)
	}
	if !e.Timestamp.Equal(res.StartedAt) {
		t.Errorf("Timestamp = %v, want %v", e.Timestamp, res.StartedAt)
	}
	if e.Metadata["decl_id"] != "files:/etc/nginx" {
		t.Errorf("decl_id = %q", e.Metadata["decl_id"])
	}
	if e.Metadata["changed"] != "true" {
		t.Errorf("changed = %q, want true", e.Metadata["changed"])
	}
	if e.Severity != audit.SeverityLow {
		t.Errorf("Severity = %v, want Low", e.Severity)
	}
}

func TestStateApplyObserver_FailedOutcomeRaisesSeverityWithViolation(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	obs := audit.NewStateApplyObserver(nil, rec)
	obs.Done(context.Background(), &statemgmt.DeclarationResult{
		DeclID:    "files:/etc/bad",
		Module:    "file",
		Outcome:   statemgmt.OutcomeFailed,
		StartedAt: time.Now().UTC(),
		Error:     errors.New("permission denied"),
	})
	entries := rec.get()
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	e := entries[0]
	if e.Allowed {
		t.Errorf("Allowed = true for failed outcome")
	}
	if e.Severity != audit.SeverityHigh {
		t.Errorf("Severity = %v, want High", e.Severity)
	}
	if len(e.Violations) != 1 {
		t.Fatalf("Violations = %d", len(e.Violations))
	}
	if e.Violations[0].Message != "permission denied" {
		t.Errorf("Violation message = %q", e.Violations[0].Message)
	}
}

func TestStateApplyObserver_SkipDoesNotEmit(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	obs := audit.NewStateApplyObserver(nil, rec)
	obs.Skip(context.Background(),
		&statemgmt.Declaration{ID: "files:/etc/cascade", Module: "file"},
		errors.New("upstream failed"))
	if entries := rec.get(); len(entries) != 0 {
		t.Errorf("Skip emitted: %+v", entries)
	}
}

func TestStateApplyObserver_NilResultNoOp(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	obs := audit.NewStateApplyObserver(nil, rec)
	obs.Done(context.Background(), nil) // must not panic
	if entries := rec.get(); len(entries) != 0 {
		t.Errorf("nil result emitted: %+v", entries)
	}
}

func TestStateApplyObserver_ForwardsToInner(t *testing.T) {
	t.Parallel()
	inner := &spyObserver{}
	obs := audit.NewStateApplyObserver(inner, audit.NoopAuditor{})

	decl := &statemgmt.Declaration{ID: "x", Module: "file"}
	ctx := context.Background()
	obs.Start(ctx, decl)
	obs.Drift(ctx, decl, &statemgmt.ModuleCheckResult{})
	obs.Change(ctx, decl, &statemgmt.StateResult{})
	obs.Done(ctx, &statemgmt.DeclarationResult{DeclID: "x", Module: "file", Outcome: statemgmt.OutcomeChanged})
	obs.Skip(ctx, decl, errors.New("r"))

	if inner.starts != 1 || inner.drifts != 1 || inner.changes != 1 || inner.dones != 1 || inner.skips != 1 {
		t.Errorf("inner spies = %+v", *inner)
	}
}

func TestStateApplyObserver_NilInnerOK(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	obs := audit.NewStateApplyObserver(nil, rec)
	// All methods on nil inner must not panic.
	obs.Start(context.Background(), &statemgmt.Declaration{ID: "x", Module: "file"})
	obs.Drift(context.Background(), &statemgmt.Declaration{ID: "x", Module: "file"}, &statemgmt.ModuleCheckResult{})
	obs.Change(context.Background(), &statemgmt.Declaration{ID: "x", Module: "file"}, &statemgmt.StateResult{})
	obs.Done(context.Background(), &statemgmt.DeclarationResult{DeclID: "x", Module: "file", Outcome: statemgmt.OutcomeUnchanged})
	obs.Skip(context.Background(), &statemgmt.Declaration{ID: "x", Module: "file"}, nil)
}

func TestStateApplyObserver_NilAuditorDefaultsToNoop(t *testing.T) {
	t.Parallel()
	obs := audit.NewStateApplyObserver(nil, nil)
	// Must not panic on Done.
	obs.Done(context.Background(), &statemgmt.DeclarationResult{
		DeclID: "x", Module: "file", Outcome: statemgmt.OutcomeUnchanged,
	})
}

func TestStateApplyObserver_CompensatedFlag(t *testing.T) {
	t.Parallel()
	rec := &recordingAuditor{}
	obs := audit.NewStateApplyObserver(nil, rec)
	obs.Done(context.Background(), &statemgmt.DeclarationResult{
		DeclID:      "x",
		Module:      "file",
		Outcome:     statemgmt.OutcomeChanged,
		Compensated: true,
	})
	entries := rec.get()
	if len(entries) != 1 {
		t.Fatalf("entries = %d", len(entries))
	}
	if entries[0].Metadata["compensated"] != "true" {
		t.Errorf("compensated metadata missing: %+v", entries[0].Metadata)
	}
}
