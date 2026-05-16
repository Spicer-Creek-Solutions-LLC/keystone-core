package controlplane

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
	"go.keystone-core.io/keystone-core/internal/state"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// recordingAuditor captures emitted entries for the policy-eval
// "every sensitive op emits" assertion.
type recordingAuditor struct {
	mu      sync.Mutex
	entries []audit.AuditEntry
}

func (r *recordingAuditor) Emit(_ context.Context, e audit.AuditEntry) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.entries = append(r.entries, e)
}
func (r *recordingAuditor) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.entries)
}

type policyRig struct {
	server   *PolicyGRPCServer
	engine   *policy.Engine
	auditLog audit.AuditStore
	auditor  *recordingAuditor
}

func newPolicyRig(t *testing.T) *policyRig {
	t.Helper()
	st, err := state.NewStore(&state.Config{
		Backend: state.BackendSQLite,
		SQLite:  state.SQLiteConfig{Path: filepath.Join(t.TempDir(), "policy.db")},
	})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })

	reg := policy.NewRegistry()
	eng, err := policy.NewEngine(reg,
		policy.WithEvaluator(audit.PolicyTypeOPA, policy.NewOPAEvaluator()),
		policy.WithEvaluator(audit.PolicyTypeCEL, policy.NewCELEvaluator()),
		policy.WithEvaluator(audit.PolicyTypeBuiltin, policy.NewBuiltinEvaluator()),
	)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	auditLog := audit.NewSQLAuditStore(st)
	gen, err := policy.NewReportGenerator(auditLog, policy.NewControlMapping())
	if err != nil {
		t.Fatalf("NewReportGenerator: %v", err)
	}
	rec := &recordingAuditor{}
	return &policyRig{
		server:   NewPolicyGRPCServer(eng, gen, auditLog, rec),
		engine:   eng,
		auditLog: auditLog,
		auditor:  rec,
	}
}

func regBuiltin(t *testing.T, eng *policy.Engine, id, code string, enabled bool) {
	t.Helper()
	if err := eng.Registry().RegisterPolicy(&policy.Policy{
		ID: id, Name: id, Type: audit.PolicyTypeBuiltin,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Code: code, Enabled: enabled,
	}); err != nil {
		t.Fatalf("RegisterPolicy %s: %v", id, err)
	}
}

func TestPolicyGRPC_EvaluatePolicy_BuiltinAllow(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	regBuiltin(t, r.engine, "allow-read", `{"rule":"allowed-actions","allowed":["read"]}`, true)

	resp, err := r.server.EvaluatePolicy(context.Background(), &v1.EvaluatePolicyRequest{
		PolicyId: "allow-read",
		Input:    &v1.EvaluationInput{Action: "read", User: "alice"},
	})
	if err != nil {
		t.Fatalf("EvaluatePolicy: %v", err)
	}
	if !resp.GetResult().GetAllowed() {
		t.Errorf("allowed = false, want true")
	}
	if r.auditor.count() != 1 {
		t.Errorf("policy-eval audit emissions = %d, want 1", r.auditor.count())
	}
}

func TestPolicyGRPC_EvaluatePolicy_DenyEmitsViolation(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	regBuiltin(t, r.engine, "only-read", `{"rule":"allowed-actions","allowed":["read"]}`, true)

	resp, err := r.server.EvaluatePolicy(context.Background(), &v1.EvaluatePolicyRequest{
		PolicyId: "only-read",
		Input:    &v1.EvaluationInput{Action: "delete"},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if resp.GetResult().GetAllowed() {
		t.Errorf("allowed = true, want deny")
	}
	if len(resp.GetResult().GetViolations()) == 0 {
		t.Errorf("expected violations on deny")
	}
	if r.auditor.count() != 1 {
		t.Errorf("audit emissions = %d, want 1", r.auditor.count())
	}
}

func TestPolicyGRPC_EvaluatePolicy_OPAandCEL(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	_ = r.engine.Registry().RegisterPolicy(&policy.Policy{
		ID: "opa-allow", Name: "opa-allow", Type: audit.PolicyTypeOPA,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Enabled: true,
		Code: "package keystone.policy\n\nallow := true\n",
	})
	_ = r.engine.Registry().RegisterPolicy(&policy.Policy{
		ID: "cel-read", Name: "cel-read", Type: audit.PolicyTypeCEL,
		Category: policy.CategorySecurity, Severity: audit.SeverityHigh,
		EnforcementMode: audit.EnforcementModeAudit, Enabled: true,
		Code: `action == "read"`,
	})

	ro, err := r.server.EvaluatePolicy(context.Background(), &v1.EvaluatePolicyRequest{PolicyId: "opa-allow"})
	if err != nil || !ro.GetResult().GetAllowed() {
		t.Errorf("OPA: resp=%v err=%v", ro, err)
	}
	rc, err := r.server.EvaluatePolicy(context.Background(), &v1.EvaluatePolicyRequest{
		PolicyId: "cel-read", Input: &v1.EvaluationInput{Action: "read"},
	})
	if err != nil || !rc.GetResult().GetAllowed() {
		t.Errorf("CEL: resp=%v err=%v", rc, err)
	}
}

func TestPolicyGRPC_EvaluatePolicy_Errors(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	regBuiltin(t, r.engine, "disabled-p", `{"rule":"allowed-actions","allowed":["x"]}`, false)

	if _, err := r.server.EvaluatePolicy(context.Background(), &v1.EvaluatePolicyRequest{}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("empty policy_id code = %v, want InvalidArgument", status.Code(err))
	}
	if _, err := r.server.EvaluatePolicy(context.Background(), &v1.EvaluatePolicyRequest{PolicyId: "ghost"}); status.Code(err) != codes.NotFound {
		t.Errorf("missing policy code = %v, want NotFound", status.Code(err))
	}
	if _, err := r.server.EvaluatePolicy(context.Background(), &v1.EvaluatePolicyRequest{PolicyId: "disabled-p"}); status.Code(err) != codes.FailedPrecondition {
		t.Errorf("disabled policy code = %v, want FailedPrecondition", status.Code(err))
	}
}

func TestPolicyGRPC_EvaluatePolicySet_AllowedAll(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	regBuiltin(t, r.engine, "a", `{"rule":"allowed-actions","allowed":["read"]}`, true)
	regBuiltin(t, r.engine, "b", `{"rule":"allowed-actions","allowed":["read","write"]}`, true)
	_ = r.engine.Registry().RegisterPolicySet(&policy.PolicySet{
		ID: "set", Name: "set", PolicyIDs: []string{"a", "b"}, Enabled: true,
	})

	resp, err := r.server.EvaluatePolicySet(context.Background(), &v1.EvaluatePolicySetRequest{
		PolicySetId: "set", Input: &v1.EvaluationInput{Action: "write"},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	// a denies write, b allows → AllowedAll false, 2 results, 2 audits.
	if resp.GetAllowedAll() {
		t.Errorf("AllowedAll = true, want false (member a denies write)")
	}
	if len(resp.GetResults()) != 2 {
		t.Errorf("results = %d, want 2", len(resp.GetResults()))
	}
	if r.auditor.count() != 2 {
		t.Errorf("audit emissions = %d, want 2 (one per member)", r.auditor.count())
	}
}

func TestPolicyGRPC_GetAndListPolicies(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	regBuiltin(t, r.engine, "p-b", `{"rule":"allowed-actions","allowed":["x"]}`, true)
	regBuiltin(t, r.engine, "p-a", `{"rule":"allowed-actions","allowed":["x"]}`, true)

	g, err := r.server.GetPolicy(context.Background(), &v1.GetPolicyRequest{Id: "p-a"})
	if err != nil || g.GetPolicy().GetId() != "p-a" {
		t.Errorf("GetPolicy: %v / %v", g, err)
	}
	if _, err := r.server.GetPolicy(context.Background(), &v1.GetPolicyRequest{Id: "nope"}); status.Code(err) != codes.NotFound {
		t.Errorf("GetPolicy(missing) code = %v", status.Code(err))
	}
	l, err := r.server.ListPolicies(context.Background(), &v1.ListPoliciesRequest{})
	if err != nil {
		t.Fatalf("ListPolicies: %v", err)
	}
	if len(l.GetPolicies()) != 2 || l.GetPolicies()[0].GetId() != "p-a" {
		t.Errorf("ListPolicies sorted = %+v", l.GetPolicies())
	}
	if l.GetTotalCount() != 2 {
		t.Errorf("total_count = %d, want 2", l.GetTotalCount())
	}
}

func TestPolicyGRPC_ListPolicies_Pagination(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	for _, id := range []string{"p1", "p2", "p3"} {
		regBuiltin(t, r.engine, id, `{"rule":"allowed-actions","allowed":["x"]}`, true)
	}
	p1, _ := r.server.ListPolicies(context.Background(), &v1.ListPoliciesRequest{PageSize: 2})
	if len(p1.GetPolicies()) != 2 || p1.GetNextPageToken() == "" {
		t.Fatalf("page1: %d entries, token=%q", len(p1.GetPolicies()), p1.GetNextPageToken())
	}
	p2, _ := r.server.ListPolicies(context.Background(), &v1.ListPoliciesRequest{
		PageSize: 2, PageToken: p1.GetNextPageToken(),
	})
	if len(p2.GetPolicies()) != 1 || p2.GetNextPageToken() != "" {
		t.Errorf("page2: %d entries, token=%q", len(p2.GetPolicies()), p2.GetNextPageToken())
	}
	if _, err := r.server.ListPolicies(context.Background(), &v1.ListPoliciesRequest{PageToken: "abc"}); status.Code(err) != codes.InvalidArgument {
		t.Errorf("bad token code = %v", status.Code(err))
	}
}

func TestPolicyGRPC_GetAuditLogAndViolations(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	regBuiltin(t, r.engine, "only-read", `{"rule":"allowed-actions","allowed":["read"]}`, true)
	// Generate one deny → one audit entry persisted via the SQL store
	// path (emitPolicyAudit uses the recording auditor, so persist
	// directly through the audit store for the query-side test).
	ctx := context.Background()
	e := audit.MustNewAuditEntry(audit.AuditEntryInput{
		PolicyID: "only-read", Action: "policy.evaluate", Allowed: false,
		Severity: audit.SeverityHigh,
		Violations: []audit.Violation{{Rule: "allowed-actions", Message: "no", Severity: audit.SeverityHigh}},
	})
	if err := r.auditLog.Store(ctx, e); err != nil {
		t.Fatalf("seed audit: %v", err)
	}

	al, err := r.server.GetAuditLog(ctx, &v1.GetAuditLogRequest{})
	if err != nil {
		t.Fatalf("GetAuditLog: %v", err)
	}
	if len(al.GetEntries()) != 1 || al.GetEntries()[0].GetPolicyId() != "only-read" {
		t.Errorf("audit log = %+v", al.GetEntries())
	}
	vl, err := r.server.ListViolations(ctx, &v1.ListViolationsRequest{})
	if err != nil {
		t.Fatalf("ListViolations: %v", err)
	}
	if len(vl.GetEntries()) != 1 || vl.GetEntries()[0].GetAllowed() {
		t.Errorf("violations = %+v (want 1 denied)", vl.GetEntries())
	}
}

func TestPolicyGRPC_GetComplianceReport(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	ctx := context.Background()
	now := time.Now().UTC()
	for i := 0; i < 3; i++ {
		allowed := i != 0 // 1 deny, 2 allow
		_ = r.auditLog.Store(ctx, audit.MustNewAuditEntry(audit.AuditEntryInput{
			PolicyID: "p", Action: "policy.evaluate", Allowed: allowed,
			Severity: audit.SeverityHigh,
		}))
	}
	resp, err := r.server.GetComplianceReport(ctx, &v1.GetComplianceReportRequest{
		Since: timestamppb.New(now.Add(-time.Hour)),
		Until: timestamppb.New(now.Add(time.Hour)),
	})
	if err != nil {
		t.Fatalf("GetComplianceReport: %v", err)
	}
	if resp.GetTotalEvaluations() != 3 || resp.GetCompliantEvaluations() != 2 {
		t.Errorf("report counts: total=%d compliant=%d", resp.GetTotalEvaluations(), resp.GetCompliantEvaluations())
	}
}

func TestPolicyGRPC_CRUD_Unimplemented(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	if _, err := r.server.CreatePolicy(context.Background(), &v1.CreatePolicyRequest{}); status.Code(err) != codes.Unimplemented {
		t.Errorf("CreatePolicy code = %v, want Unimplemented", status.Code(err))
	}
	if _, err := r.server.UpdatePolicy(context.Background(), &v1.UpdatePolicyRequest{}); status.Code(err) != codes.Unimplemented {
		t.Errorf("UpdatePolicy code = %v, want Unimplemented", status.Code(err))
	}
	if _, err := r.server.DeletePolicy(context.Background(), &v1.DeletePolicyRequest{}); status.Code(err) != codes.Unimplemented {
		t.Errorf("DeletePolicy code = %v, want Unimplemented", status.Code(err))
	}
}

func TestPolicyGRPC_NilComponentsUnavailable(t *testing.T) {
	t.Parallel()
	s := NewPolicyGRPCServer(nil, nil, nil, nil)
	if _, err := s.EvaluatePolicy(context.Background(), &v1.EvaluatePolicyRequest{PolicyId: "x"}); status.Code(err) != codes.Unavailable {
		t.Errorf("nil engine EvaluatePolicy code = %v", status.Code(err))
	}
	if _, err := s.GetComplianceReport(context.Background(), &v1.GetComplianceReportRequest{}); status.Code(err) != codes.Unavailable {
		t.Errorf("nil reports code = %v", status.Code(err))
	}
	if _, err := s.GetAuditLog(context.Background(), &v1.GetAuditLogRequest{}); status.Code(err) != codes.Unavailable {
		t.Errorf("nil auditLog code = %v", status.Code(err))
	}
}

func TestPolicyGRPC_EvaluationInputStructResource(t *testing.T) {
	t.Parallel()
	r := newPolicyRig(t)
	regBuiltin(t, r.engine, "labels", `{"rule":"require-labels","keys":["owner"]}`, true)

	rs, _ := structpb.NewStruct(map[string]any{
		"labels": map[string]any{"owner": "team-a"},
	})
	resp, err := r.server.EvaluatePolicy(context.Background(), &v1.EvaluatePolicyRequest{
		PolicyId: "labels",
		Input:    &v1.EvaluationInput{Resource: rs},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !resp.GetResult().GetAllowed() {
		t.Errorf("require-labels with owner present should allow; violations=%+v", resp.GetResult().GetViolations())
	}
}
