package main

// AuditEmissionContract — Epic 12 task 4 contract tests.
//
// PROJECT-DETAILS §4.12 rule:
//
//	"Every sensitive op (auth decision, secret access, command exec,
//	 state apply, policy eval) emits an audit entry. Failure to log
//	 = bug."
//
// These sub-tests exercise each sensitive op through its boot-wired
// emitter function and verify the entry lands in a recording
// audit.Auditor. They are NOT plain unit tests of the emitter glue
// — those live in audit_store_bridge_test.go / auth_emitter_test.go
// / etc. — they're a CONTRACT regression suite: if anyone disables
// or drops an emission site, the matching sub-test fails and the
// "Failure to log = bug" invariant catches it.
//
// Policy eval is NOT covered here. The Engine that emits for policy
// eval lands in tasks 5+9 (Epic 12); task 4 deliberately doesn't
// stub an emission site for it. When task 9 lands, this file should
// gain a fifth sub-test PolicyEval_EmitsAudit.

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/internal/statemgmt"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// recorder is a recording auditor + helper that asserts on entries
// in order; cmd/kscore-server has no other shared test helper.
type emissionRecorder struct {
	audit.NoopAuditor
	got []audit.AuditEntry
}

func (r *emissionRecorder) Emit(_ context.Context, e audit.AuditEntry) {
	r.got = append(r.got, e)
}

func TestAuditEmissionContract_AuthDecision_AllowedEmits(t *testing.T) {
	// AuditEmissionContract: auth (allow path)
	t.Parallel()
	rec := &emissionRecorder{}
	emit := newAuthDecisionEmitter(rec)

	emit(context.Background(), "/svc/Foo",
		&auth.Principal{ID: "u-1", Role: auth.RoleAdmin},
		true, nil)

	if len(rec.got) != 1 {
		t.Fatalf("expected 1 audit entry, got %d", len(rec.got))
	}
	e := rec.got[0]
	if e.Action != "/svc/Foo" || e.ResourceType != "rpc" {
		t.Errorf("entry shape: %+v", e)
	}
	if !e.Allowed {
		t.Errorf("Allowed false on allow path")
	}
	if e.User != "u-1" {
		t.Errorf("User = %q", e.User)
	}
	if e.Severity != audit.SeverityLow {
		t.Errorf("Severity = %v on allow", e.Severity)
	}
}

func TestAuditEmissionContract_AuthDecision_DeniedEmitsWithViolation(t *testing.T) {
	// AuditEmissionContract: auth (deny path)
	t.Parallel()
	rec := &emissionRecorder{}
	emit := newAuthDecisionEmitter(rec)

	emit(context.Background(), "/svc/AdminOp",
		&auth.Principal{ID: "u-1", Role: auth.RoleReadonly},
		false, errors.New("insufficient role"))

	if len(rec.got) != 1 {
		t.Fatalf("got %d", len(rec.got))
	}
	e := rec.got[0]
	if e.Allowed {
		t.Errorf("Allowed true on deny path")
	}
	if e.Severity != audit.SeverityMedium {
		t.Errorf("Severity = %v on deny, want Medium", e.Severity)
	}
	if len(e.Violations) != 1 || e.Violations[0].Message != "insufficient role" {
		t.Errorf("violations: %+v", e.Violations)
	}
}

func TestAuditEmissionContract_AuthDecision_NilAuditorNoOp(t *testing.T) {
	// AuditEmissionContract: auth (nil-auditor safety)
	t.Parallel()
	emit := newAuthDecisionEmitter(nil)
	emit(context.Background(), "/svc/X", &auth.Principal{ID: "u"}, true, nil)
	// Must not panic.
}

func TestAuditEmissionContract_SecretAccess_EmitsAudit(t *testing.T) {
	// AuditEmissionContract: secrets
	t.Parallel()
	rec := &emissionRecorder{}
	bridge := newSecretsAuditStoreBridge(rec)

	bridge.Emit(context.Background(), secrets.SecretAccessEvent{
		Timestamp: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		Action:    "secret.get",
		Path:      "kv/data/app/db",
		Backend:   "vault",
		Principal: secrets.Principal{SPIFFEID: "spiffe://k.local/agent/a-1"},
		Allowed:   true,
		Duration:  10 * time.Millisecond,
	})
	if len(rec.got) != 1 {
		t.Fatalf("got %d", len(rec.got))
	}
	e := rec.got[0]
	if e.Action != "secret.get" {
		t.Errorf("Action = %q", e.Action)
	}
	if e.ResourceType != "secret" {
		t.Errorf("ResourceType = %q", e.ResourceType)
	}
	if e.User != "spiffe://k.local/agent/a-1" {
		t.Errorf("actor not from SPIFFE: %q", e.User)
	}
}

func TestAuditEmissionContract_StateApply_EmitsPerDeclaration(t *testing.T) {
	// AuditEmissionContract: state apply
	t.Parallel()
	rec := &emissionRecorder{}
	obs := audit.NewStateApplyObserver(nil, rec)

	obs.Done(context.Background(), &statemgmt.DeclarationResult{
		DeclID:    "files:/etc/nginx.conf",
		Module:    "file",
		Outcome:   statemgmt.OutcomeChanged,
		StartedAt: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		Duration:  150 * time.Millisecond,
		Apply:     &statemgmt.StateResult{Changed: true},
	})
	if len(rec.got) != 1 {
		t.Fatalf("got %d", len(rec.got))
	}
	e := rec.got[0]
	if e.Action != "state.apply" || e.ResourceType != "file" {
		t.Errorf("entry shape: action=%q resource_type=%q", e.Action, e.ResourceType)
	}
	if !e.Allowed {
		t.Errorf("Allowed false on Changed outcome")
	}
	if e.Metadata["decl_id"] != "files:/etc/nginx.conf" {
		t.Errorf("decl_id metadata missing: %+v", e.Metadata)
	}
}

func TestAuditEmissionContract_StateApply_SkipDoesNotEmit(t *testing.T) {
	// AuditEmissionContract: state apply (skip-suppression invariant)
	t.Parallel()
	rec := &emissionRecorder{}
	obs := audit.NewStateApplyObserver(nil, rec)
	obs.Skip(context.Background(),
		&statemgmt.Declaration{ID: "x", Module: "file"},
		errors.New("upstream failed"))
	if len(rec.got) != 0 {
		t.Errorf("Skip emitted: %+v", rec.got)
	}
}

func TestAuditEmissionContract_CommandExec_TerminalEmits(t *testing.T) {
	// AuditEmissionContract: command exec
	t.Parallel()
	rec := &emissionRecorder{}
	emit := newCommandTerminalEmitter(rec)

	emit(context.Background(),
		"user:alice",
		&state.CommandRecord{
			ID:        "cmd-1",
			AgentID:   "a-1",
			Command:   "ls",
			StartedAt: time.Date(2026, 5, 15, 12, 0, 0, 0, time.UTC),
		},
		state.CommandResult{
			Status:      state.CommandStatusCompleted,
			ExitCode:    0,
			CompletedAt: time.Date(2026, 5, 15, 12, 0, 0, 150_000_000, time.UTC),
		})
	if len(rec.got) != 1 {
		t.Fatalf("got %d", len(rec.got))
	}
	e := rec.got[0]
	if e.Action != "command.exec" || e.ResourceType != "command" {
		t.Errorf("entry shape: action=%q resource_type=%q", e.Action, e.ResourceType)
	}
	if e.User != "user:alice" {
		t.Errorf("User = %q", e.User)
	}
	if !e.Allowed {
		t.Errorf("Allowed false on Completed+exit=0")
	}
	if e.Metadata["command_id"] != "cmd-1" || e.Metadata["agent_id"] != "a-1" {
		t.Errorf("metadata: %+v", e.Metadata)
	}
}

func TestAuditEmissionContract_CommandExec_FailedRaisesSeverity(t *testing.T) {
	// AuditEmissionContract: command exec (failure path)
	t.Parallel()
	rec := &emissionRecorder{}
	emit := newCommandTerminalEmitter(rec)
	emit(context.Background(), "",
		&state.CommandRecord{ID: "cmd-2", AgentID: "a-1", Command: "false", User: "root"},
		state.CommandResult{Status: state.CommandStatusFailed, ExitCode: 1, Stderr: "exec failed"})
	if len(rec.got) != 1 {
		t.Fatalf("got %d", len(rec.got))
	}
	e := rec.got[0]
	if e.Allowed {
		t.Errorf("Allowed true on Failed status")
	}
	if e.Severity != audit.SeverityHigh {
		t.Errorf("Severity = %v on failure, want High", e.Severity)
	}
	if len(e.Violations) != 1 || e.Violations[0].Message != "exec failed" {
		t.Errorf("violations: %+v", e.Violations)
	}
	// User falls back to record.User when principal is empty.
	if e.User != "root" {
		t.Errorf("User = %q, want root", e.User)
	}
}

func TestAuditEmissionContract_PolicyEval_DeferredToTask9(t *testing.T) {
	// AuditEmissionContract: policy eval (deferred to tasks 5+9)
	//
	// This test asserts the seam exists in the audit package
	// (Severity, Violations, PolicyType enums) but does NOT
	// exercise an emission site — there isn't one yet. When task 9
	// ships Engine.Evaluate, replace this with a real emission test.
	t.Parallel()
	if !audit.PolicyTypeOPA.IsKnown() {
		t.Errorf("policy-eval seam regressed: PolicyTypeOPA unknown")
	}
	if audit.SeverityHigh.String() == "" {
		t.Errorf("severity enum regressed")
	}
}
