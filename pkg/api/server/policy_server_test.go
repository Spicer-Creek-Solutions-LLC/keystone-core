package server

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/shawnbutts/keystone-core/internal/policy"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// --- Mock types ---

type mockPolicyRegistry struct {
	policies   map[string]*policy.Policy
	sets       map[string]*policy.Set
	registerErr error
	updateErr   error
	deleteErr   error
}

func newMockPolicyRegistry() *mockPolicyRegistry {
	return &mockPolicyRegistry{
		policies: make(map[string]*policy.Policy),
		sets:     make(map[string]*policy.Set),
	}
}

func (m *mockPolicyRegistry) RegisterPolicy(p *policy.Policy) error {
	if m.registerErr != nil {
		return m.registerErr
	}
	m.policies[p.ID] = p
	return nil
}

func (m *mockPolicyRegistry) GetPolicy(id string) (*policy.Policy, bool) {
	p, ok := m.policies[id]
	return p, ok
}

func (m *mockPolicyRegistry) ListPolicies() []*policy.Policy {
	result := make([]*policy.Policy, 0, len(m.policies))
	for _, p := range m.policies {
		result = append(result, p)
	}
	return result
}

func (m *mockPolicyRegistry) ListPoliciesByCategory(category policy.Category) []*policy.Policy {
	var result []*policy.Policy
	for _, p := range m.policies {
		if p.Category == category {
			result = append(result, p)
		}
	}
	return result
}

func (m *mockPolicyRegistry) ListPoliciesByType(policyType policy.Type) []*policy.Policy {
	var result []*policy.Policy
	for _, p := range m.policies {
		if p.Type == policyType {
			result = append(result, p)
		}
	}
	return result
}

func (m *mockPolicyRegistry) UpdatePolicy(p *policy.Policy) error {
	if m.updateErr != nil {
		return m.updateErr
	}
	m.policies[p.ID] = p
	return nil
}

func (m *mockPolicyRegistry) DeletePolicy(id string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.policies, id)
	return nil
}

func (m *mockPolicyRegistry) RegisterPolicySet(set *policy.Set) error {
	if m.registerErr != nil {
		return m.registerErr
	}
	m.sets[set.ID] = set
	return nil
}

func (m *mockPolicyRegistry) GetPolicySet(id string) (*policy.Set, bool) {
	s, ok := m.sets[id]
	return s, ok
}

func (m *mockPolicyRegistry) ListPolicySets() []*policy.Set {
	result := make([]*policy.Set, 0, len(m.sets))
	for _, s := range m.sets {
		result = append(result, s)
	}
	return result
}

type mockPolicyEvaluator struct {
	result    *policy.EvaluationResult
	setResult *policy.Result
	err       error
}

func (m *mockPolicyEvaluator) Evaluate(_ context.Context, _ string, _ *policy.EvaluationInput) (*policy.EvaluationResult, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func (m *mockPolicyEvaluator) EvaluatePolicySet(_ context.Context, _ string, _ *policy.EvaluationInput) (*policy.Result, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.setResult, nil
}

type mockPolicyAuditor struct {
	entries []policy.AuditEntry
}

func (m *mockPolicyAuditor) GetEntries(_ *policy.AuditFilter) []policy.AuditEntry {
	return m.entries
}

type mockComplianceReporter struct {
	report *policy.ComplianceReport
}

func (m *mockComplianceReporter) GenerateReport(_ policy.ReportPeriod) *policy.ComplianceReport {
	return m.report
}

// --- EvaluatePolicy tests ---

func TestPolicyServer_EvaluatePolicy_NilEvaluator(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.EvaluatePolicy(context.Background(), &pb.EvaluatePolicyRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_EvaluatePolicy_MissingPolicyID(t *testing.T) {
	eval := &mockPolicyEvaluator{}
	srv := NewPolicyServer(nil, eval, nil, nil)
	_, err := srv.EvaluatePolicy(context.Background(), &pb.EvaluatePolicyRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestPolicyServer_EvaluatePolicy_MissingInput(t *testing.T) {
	eval := &mockPolicyEvaluator{}
	srv := NewPolicyServer(nil, eval, nil, nil)
	_, err := srv.EvaluatePolicy(context.Background(), &pb.EvaluatePolicyRequest{
		PolicyId: "p1",
	})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestPolicyServer_EvaluatePolicy_Success(t *testing.T) {
	eval := &mockPolicyEvaluator{
		result: &policy.EvaluationResult{
			PolicyID:   "p1",
			PolicyName: "Test Policy",
			Allowed:    true,
			Duration:   50 * time.Millisecond,
			EvaluatedAt: time.Now(),
		},
	}
	srv := NewPolicyServer(nil, eval, nil, nil)

	resource, _ := structpb.NewStruct(map[string]interface{}{"type": "test"})
	resp, err := srv.EvaluatePolicy(context.Background(), &pb.EvaluatePolicyRequest{
		PolicyId: "p1",
		Input: &pb.EvaluationInput{
			Resource: resource,
			Action:   "create",
			User:     "admin",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Result.Allowed {
		t.Error("expected allowed=true")
	}
	if resp.Result.PolicyId != "p1" {
		t.Errorf("got policy_id %q, want %q", resp.Result.PolicyId, "p1")
	}
}

func TestPolicyServer_EvaluatePolicy_EvalError(t *testing.T) {
	eval := &mockPolicyEvaluator{err: errors.New("eval failed")}
	srv := NewPolicyServer(nil, eval, nil, nil)

	_, err := srv.EvaluatePolicy(context.Background(), &pb.EvaluatePolicyRequest{
		PolicyId: "p1",
		Input:    &pb.EvaluationInput{Action: "test"},
	})
	st, _ := status.FromError(err)
	if st.Code() != codes.Internal {
		t.Errorf("got code %v, want Internal", st.Code())
	}
}

// --- EvaluatePolicySet tests ---

func TestPolicyServer_EvaluatePolicySet_NilEvaluator(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.EvaluatePolicySet(context.Background(), &pb.EvaluatePolicySetRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_EvaluatePolicySet_Success(t *testing.T) {
	eval := &mockPolicyEvaluator{
		setResult: &policy.Result{
			Allowed: false,
			Results: []*policy.EvaluationResult{
				{PolicyID: "p1", Allowed: true},
				{PolicyID: "p2", Allowed: false, Violations: []policy.Violation{
					{Rule: "no-root", Message: "root access denied", Severity: policy.SeverityHigh},
				}},
			},
			Summary: &policy.Summary{
				TotalPolicies:   2,
				AllowedPolicies: 1,
				DeniedPolicies:  1,
				TotalViolations: 1,
			},
			TotalDuration: 100 * time.Millisecond,
			EvaluatedAt:   time.Now(),
		},
	}
	srv := NewPolicyServer(nil, eval, nil, nil)

	resp, err := srv.EvaluatePolicySet(context.Background(), &pb.EvaluatePolicySetRequest{
		PolicySetId: "set1",
		Input:       &pb.EvaluationInput{Action: "deploy"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Allowed {
		t.Error("expected allowed=false")
	}
	if len(resp.Results) != 2 {
		t.Fatalf("got %d results, want 2", len(resp.Results))
	}
	if resp.Summary.TotalPolicies != 2 {
		t.Errorf("got total_policies %d, want 2", resp.Summary.TotalPolicies)
	}
}

// --- ListPolicies tests ---

func TestPolicyServer_ListPolicies_NilRegistry(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.ListPolicies(context.Background(), &pb.ListPoliciesRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_ListPolicies_All(t *testing.T) {
	reg := newMockPolicyRegistry()
	reg.policies["p1"] = &policy.Policy{ID: "p1", Name: "Policy 1", Type: policy.TypeOPA, Category: policy.CategorySecurity, Enabled: true}
	reg.policies["p2"] = &policy.Policy{ID: "p2", Name: "Policy 2", Type: policy.TypeCEL, Category: policy.CategoryCompliance, Enabled: false}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.ListPolicies(context.Background(), &pb.ListPoliciesRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Policies) != 2 {
		t.Errorf("got %d policies, want 2", len(resp.Policies))
	}
	if resp.TotalCount != 2 {
		t.Errorf("got total_count %d, want 2", resp.TotalCount)
	}
}

func TestPolicyServer_ListPolicies_FilterByCategory(t *testing.T) {
	reg := newMockPolicyRegistry()
	reg.policies["p1"] = &policy.Policy{ID: "p1", Category: policy.CategorySecurity, Enabled: true}
	reg.policies["p2"] = &policy.Policy{ID: "p2", Category: policy.CategoryCompliance, Enabled: true}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.ListPolicies(context.Background(), &pb.ListPoliciesRequest{
		Category: pb.PolicyCategory_POLICY_CATEGORY_SECURITY,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Policies) != 1 {
		t.Fatalf("got %d policies, want 1", len(resp.Policies))
	}
	if resp.Policies[0].Id != "p1" {
		t.Errorf("got policy %q, want %q", resp.Policies[0].Id, "p1")
	}
}

func TestPolicyServer_ListPolicies_FilterByEnabled(t *testing.T) {
	reg := newMockPolicyRegistry()
	reg.policies["p1"] = &policy.Policy{ID: "p1", Enabled: true}
	reg.policies["p2"] = &policy.Policy{ID: "p2", Enabled: false}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.ListPolicies(context.Background(), &pb.ListPoliciesRequest{
		Enabled: &pb.OptionalBool{Value: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Policies) != 1 {
		t.Fatalf("got %d policies, want 1", len(resp.Policies))
	}
}

func TestPolicyServer_ListPolicies_FilterByTags(t *testing.T) {
	reg := newMockPolicyRegistry()
	reg.policies["p1"] = &policy.Policy{ID: "p1", Tags: []string{"production", "critical"}}
	reg.policies["p2"] = &policy.Policy{ID: "p2", Tags: []string{"staging"}}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.ListPolicies(context.Background(), &pb.ListPoliciesRequest{
		Tags: []string{"production"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Policies) != 1 {
		t.Fatalf("got %d policies, want 1", len(resp.Policies))
	}
}

// --- GetPolicy tests ---

func TestPolicyServer_GetPolicy_NilRegistry(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.GetPolicy(context.Background(), &pb.GetPolicyRequest{PolicyId: "p1"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_GetPolicy_EmptyID(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)
	_, err := srv.GetPolicy(context.Background(), &pb.GetPolicyRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestPolicyServer_GetPolicy_NotFound(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)
	_, err := srv.GetPolicy(context.Background(), &pb.GetPolicyRequest{PolicyId: "missing"})
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
}

func TestPolicyServer_GetPolicy_Success(t *testing.T) {
	reg := newMockPolicyRegistry()
	reg.policies["p1"] = &policy.Policy{
		ID: "p1", Name: "Test", Type: policy.TypeOPA, Category: policy.CategorySecurity,
		Severity: policy.SeverityHigh, EnforcementMode: policy.ModeEnforce, Enabled: true,
	}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.GetPolicy(context.Background(), &pb.GetPolicyRequest{PolicyId: "p1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Policy.Name != "Test" {
		t.Errorf("got name %q, want %q", resp.Policy.Name, "Test")
	}
	if resp.Policy.Type != pb.PolicyType_POLICY_TYPE_OPA {
		t.Errorf("got type %v, want OPA", resp.Policy.Type)
	}
	if resp.Policy.Severity != pb.PolicySeverity_POLICY_SEVERITY_HIGH {
		t.Errorf("got severity %v, want HIGH", resp.Policy.Severity)
	}
}

// --- CreatePolicy tests ---

func TestPolicyServer_CreatePolicy_NilRegistry(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.CreatePolicy(context.Background(), &pb.CreatePolicyRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_CreatePolicy_NilPolicy(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)
	_, err := srv.CreatePolicy(context.Background(), &pb.CreatePolicyRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestPolicyServer_CreatePolicy_Success(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.CreatePolicy(context.Background(), &pb.CreatePolicyRequest{
		Policy: &pb.Policy{
			Name:            "New Policy",
			Type:            pb.PolicyType_POLICY_TYPE_CEL,
			Category:        pb.PolicyCategory_POLICY_CATEGORY_COMPLIANCE,
			Severity:        pb.PolicySeverity_POLICY_SEVERITY_MEDIUM,
			EnforcementMode: pb.EnforcementMode_ENFORCEMENT_MODE_WARN,
			Policy:          "resource.labels.exists('owner')",
			Enabled:         true,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Policy.Id == "" {
		t.Error("expected generated ID")
	}
	if resp.Policy.Name != "New Policy" {
		t.Errorf("got name %q, want %q", resp.Policy.Name, "New Policy")
	}
	if len(reg.policies) != 1 {
		t.Errorf("registry has %d policies, want 1", len(reg.policies))
	}
}

func TestPolicyServer_CreatePolicy_WithID(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.CreatePolicy(context.Background(), &pb.CreatePolicyRequest{
		Policy: &pb.Policy{Id: "custom-id", Name: "Custom"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Policy.Id != "custom-id" {
		t.Errorf("got id %q, want %q", resp.Policy.Id, "custom-id")
	}
}

// --- UpdatePolicy tests ---

func TestPolicyServer_UpdatePolicy_NilRegistry(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.UpdatePolicy(context.Background(), &pb.UpdatePolicyRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_UpdatePolicy_NilPolicy(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)
	_, err := srv.UpdatePolicy(context.Background(), &pb.UpdatePolicyRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestPolicyServer_UpdatePolicy_NotFound(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)
	_, err := srv.UpdatePolicy(context.Background(), &pb.UpdatePolicyRequest{
		Policy: &pb.Policy{Id: "missing", Name: "Updated"},
	})
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
}

func TestPolicyServer_UpdatePolicy_Success(t *testing.T) {
	reg := newMockPolicyRegistry()
	created := time.Now().Add(-time.Hour)
	reg.policies["p1"] = &policy.Policy{ID: "p1", Name: "Old Name", CreatedAt: created}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.UpdatePolicy(context.Background(), &pb.UpdatePolicyRequest{
		Policy: &pb.Policy{Id: "p1", Name: "New Name", Enabled: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Policy.Name != "New Name" {
		t.Errorf("got name %q, want %q", resp.Policy.Name, "New Name")
	}
	// CreatedAt should be preserved
	if reg.policies["p1"].CreatedAt != created {
		t.Error("created_at was not preserved")
	}
}

// --- DeletePolicy tests ---

func TestPolicyServer_DeletePolicy_NilRegistry(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.DeletePolicy(context.Background(), &pb.DeletePolicyRequest{PolicyId: "p1"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_DeletePolicy_EmptyID(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)
	_, err := srv.DeletePolicy(context.Background(), &pb.DeletePolicyRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestPolicyServer_DeletePolicy_Success(t *testing.T) {
	reg := newMockPolicyRegistry()
	reg.policies["p1"] = &policy.Policy{ID: "p1"}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.DeletePolicy(context.Background(), &pb.DeletePolicyRequest{PolicyId: "p1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !resp.Deleted {
		t.Error("expected deleted=true")
	}
	if _, ok := reg.policies["p1"]; ok {
		t.Error("policy should have been deleted from registry")
	}
}

// --- ListViolations tests ---

func TestPolicyServer_ListViolations_NilAuditor(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.ListViolations(context.Background(), &pb.ListViolationsRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_ListViolations_Success(t *testing.T) {
	auditor := &mockPolicyAuditor{
		entries: []policy.AuditEntry{
			{
				ID:              "a1",
				Timestamp:       time.Now(),
				PolicyID:        "p1",
				PolicyName:      "Test Policy",
				Allowed:         false,
				EnforcementMode: policy.ModeEnforce,
				Violations: []policy.Violation{
					{Rule: "no-root", Message: "denied", Severity: policy.SeverityHigh},
				},
			},
		},
	}
	srv := NewPolicyServer(nil, nil, auditor, nil)

	resp, err := srv.ListViolations(context.Background(), &pb.ListViolationsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Records) != 1 {
		t.Fatalf("got %d records, want 1", len(resp.Records))
	}
	if resp.Records[0].PolicyId != "p1" {
		t.Errorf("got policy_id %q, want %q", resp.Records[0].PolicyId, "p1")
	}
	if !resp.Records[0].Blocked {
		t.Error("expected blocked=true for enforce mode")
	}
}

func TestPolicyServer_ListViolations_FilterBySeverity(t *testing.T) {
	auditor := &mockPolicyAuditor{
		entries: []policy.AuditEntry{
			{
				ID: "a1", Allowed: false,
				Violations: []policy.Violation{
					{Rule: "r1", Severity: policy.SeverityLow},
					{Rule: "r2", Severity: policy.SeverityHigh},
				},
			},
		},
	}
	srv := NewPolicyServer(nil, nil, auditor, nil)

	resp, err := srv.ListViolations(context.Background(), &pb.ListViolationsRequest{
		Severity: pb.PolicySeverity_POLICY_SEVERITY_HIGH,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Records) != 1 {
		t.Fatalf("got %d records, want 1 (filtered to HIGH only)", len(resp.Records))
	}
}

// --- GetComplianceReport tests ---

func TestPolicyServer_GetComplianceReport_NilReporter(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.GetComplianceReport(context.Background(), &pb.GetComplianceReportRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_GetComplianceReport_Success(t *testing.T) {
	reporter := &mockComplianceReporter{
		report: &policy.ComplianceReport{
			GeneratedAt:       time.Now(),
			Period:            policy.ReportPeriod{Start: time.Now().Add(-24 * time.Hour), End: time.Now()},
			TotalPolicies:     10,
			CompliantPolicies: 8,
			ViolatingPolicies: 2,
			ComplianceRate:    80.0,
			ViolationsBySeverity: map[policy.Severity]int{
				policy.SeverityHigh: 3,
				policy.SeverityLow:  1,
			},
			TopViolations: []policy.ViolationSummary{
				{PolicyID: "p1", PolicyName: "Require Labels", Count: 5, Severity: policy.SeverityHigh},
			},
		},
	}
	srv := NewPolicyServer(nil, nil, nil, reporter)

	resp, err := srv.GetComplianceReport(context.Background(), &pb.GetComplianceReportRequest{
		StartTime: timestamppb.Now(),
		EndTime:   timestamppb.Now(),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ComplianceRate != 80.0 {
		t.Errorf("got compliance_rate %v, want 80.0", resp.ComplianceRate)
	}
	if len(resp.TopViolations) != 1 {
		t.Fatalf("got %d top violations, want 1", len(resp.TopViolations))
	}
	if resp.TopViolations[0].Count != 5 {
		t.Errorf("got count %d, want 5", resp.TopViolations[0].Count)
	}
	if resp.ViolationsBySeverity["high"] != 3 {
		t.Errorf("got high violations %d, want 3", resp.ViolationsBySeverity["high"])
	}
}

// --- GetAuditLog tests ---

func TestPolicyServer_GetAuditLog_NilAuditor(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.GetAuditLog(context.Background(), &pb.GetAuditLogRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_GetAuditLog_Success(t *testing.T) {
	now := time.Now()
	auditor := &mockPolicyAuditor{
		entries: []policy.AuditEntry{
			{
				ID:              "audit-1",
				Timestamp:       now,
				PolicyID:        "p1",
				PolicyName:      "Test",
				PolicyType:      policy.TypeOPA,
				Allowed:         true,
				Duration:        10 * time.Millisecond,
				EnforcementMode: policy.ModeAudit,
				User:            "admin",
				Action:          "deploy",
			},
			{
				ID:              "audit-2",
				Timestamp:       now,
				PolicyID:        "p2",
				PolicyName:      "Compliance",
				PolicyType:      policy.TypeCEL,
				Allowed:         false,
				Duration:        5 * time.Millisecond,
				EnforcementMode: policy.ModeEnforce,
				Violations: []policy.Violation{
					{Rule: "no-root", Message: "denied"},
				},
			},
		},
	}
	srv := NewPolicyServer(nil, nil, auditor, nil)

	resp, err := srv.GetAuditLog(context.Background(), &pb.GetAuditLogRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(resp.Entries))
	}
	if resp.Entries[0].PolicyType != pb.PolicyType_POLICY_TYPE_OPA {
		t.Errorf("got type %v, want OPA", resp.Entries[0].PolicyType)
	}
	if resp.Entries[1].EnforcementMode != pb.EnforcementMode_ENFORCEMENT_MODE_ENFORCE {
		t.Errorf("got enforcement %v, want ENFORCE", resp.Entries[1].EnforcementMode)
	}
	if len(resp.Entries[1].Violations) != 1 {
		t.Errorf("got %d violations, want 1", len(resp.Entries[1].Violations))
	}
}

func TestPolicyServer_GetAuditLog_Pagination(t *testing.T) {
	entries := make([]policy.AuditEntry, 5)
	for i := range entries {
		entries[i] = policy.AuditEntry{ID: fmt.Sprintf("audit-%d", i)}
	}
	auditor := &mockPolicyAuditor{entries: entries}
	srv := NewPolicyServer(nil, nil, auditor, nil)

	resp, err := srv.GetAuditLog(context.Background(), &pb.GetAuditLogRequest{PageSize: 2})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Entries) != 2 {
		t.Fatalf("got %d entries, want 2", len(resp.Entries))
	}
	if resp.NextPageToken == "" {
		t.Error("expected next_page_token")
	}
	if resp.TotalCount != 5 {
		t.Errorf("got total_count %d, want 5", resp.TotalCount)
	}

	// Fetch page 2
	resp2, err := srv.GetAuditLog(context.Background(), &pb.GetAuditLogRequest{
		PageSize:  2,
		PageToken: resp.NextPageToken,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp2.Entries) != 2 {
		t.Fatalf("page 2 got %d entries, want 2", len(resp2.Entries))
	}
}

// --- ListPolicySets tests ---

func TestPolicyServer_ListPolicySets_NilRegistry(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.ListPolicySets(context.Background(), &pb.ListPolicySetsRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_ListPolicySets_Success(t *testing.T) {
	reg := newMockPolicyRegistry()
	reg.sets["set1"] = &policy.Set{ID: "set1", Name: "Production", Enabled: true, Policies: []string{"p1", "p2"}}
	reg.sets["set2"] = &policy.Set{ID: "set2", Name: "Staging", Enabled: false}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.ListPolicySets(context.Background(), &pb.ListPolicySetsRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.PolicySets) != 2 {
		t.Errorf("got %d sets, want 2", len(resp.PolicySets))
	}
}

func TestPolicyServer_ListPolicySets_FilterEnabled(t *testing.T) {
	reg := newMockPolicyRegistry()
	reg.sets["set1"] = &policy.Set{ID: "set1", Enabled: true}
	reg.sets["set2"] = &policy.Set{ID: "set2", Enabled: false}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.ListPolicySets(context.Background(), &pb.ListPolicySetsRequest{
		Enabled: &pb.OptionalBool{Value: true},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.PolicySets) != 1 {
		t.Fatalf("got %d sets, want 1", len(resp.PolicySets))
	}
}

// --- GetPolicySet tests ---

func TestPolicyServer_GetPolicySet_NilRegistry(t *testing.T) {
	srv := NewPolicyServer(nil, nil, nil, nil)
	_, err := srv.GetPolicySet(context.Background(), &pb.GetPolicySetRequest{PolicySetId: "set1"})
	st, _ := status.FromError(err)
	if st.Code() != codes.Unavailable {
		t.Errorf("got code %v, want Unavailable", st.Code())
	}
}

func TestPolicyServer_GetPolicySet_EmptyID(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)
	_, err := srv.GetPolicySet(context.Background(), &pb.GetPolicySetRequest{})
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("got code %v, want InvalidArgument", st.Code())
	}
}

func TestPolicyServer_GetPolicySet_NotFound(t *testing.T) {
	reg := newMockPolicyRegistry()
	srv := NewPolicyServer(reg, nil, nil, nil)
	_, err := srv.GetPolicySet(context.Background(), &pb.GetPolicySetRequest{PolicySetId: "missing"})
	st, _ := status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("got code %v, want NotFound", st.Code())
	}
}

func TestPolicyServer_GetPolicySet_Success(t *testing.T) {
	reg := newMockPolicyRegistry()
	reg.sets["set1"] = &policy.Set{ID: "set1", Name: "Prod", Policies: []string{"p1", "p2"}}
	reg.policies["p1"] = &policy.Policy{ID: "p1", Name: "Policy 1"}
	reg.policies["p2"] = &policy.Policy{ID: "p2", Name: "Policy 2"}
	srv := NewPolicyServer(reg, nil, nil, nil)

	resp, err := srv.GetPolicySet(context.Background(), &pb.GetPolicySetRequest{PolicySetId: "set1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.PolicySet.Name != "Prod" {
		t.Errorf("got name %q, want %q", resp.PolicySet.Name, "Prod")
	}
	if len(resp.Policies) != 2 {
		t.Errorf("got %d policies, want 2", len(resp.Policies))
	}
}

// --- Enum conversion tests ---

func TestPolicyTypeToProto(t *testing.T) {
	tests := []struct {
		input policy.Type
		want  pb.PolicyType
	}{
		{policy.TypeOPA, pb.PolicyType_POLICY_TYPE_OPA},
		{policy.TypeCEL, pb.PolicyType_POLICY_TYPE_CEL},
		{policy.TypeBuiltin, pb.PolicyType_POLICY_TYPE_BUILTIN},
		{"unknown", pb.PolicyType_POLICY_TYPE_UNSPECIFIED},
	}
	for _, tt := range tests {
		if got := policyTypeToProto(tt.input); got != tt.want {
			t.Errorf("policyTypeToProto(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPolicyCategoryToProto(t *testing.T) {
	tests := []struct {
		input policy.Category
		want  pb.PolicyCategory
	}{
		{policy.CategorySecurity, pb.PolicyCategory_POLICY_CATEGORY_SECURITY},
		{policy.CategoryCompliance, pb.PolicyCategory_POLICY_CATEGORY_COMPLIANCE},
		{policy.CategoryOperational, pb.PolicyCategory_POLICY_CATEGORY_OPERATIONAL},
		{policy.CategoryCost, pb.PolicyCategory_POLICY_CATEGORY_COST},
		{policy.CategoryCustom, pb.PolicyCategory_POLICY_CATEGORY_CUSTOM},
		{"unknown", pb.PolicyCategory_POLICY_CATEGORY_UNSPECIFIED},
	}
	for _, tt := range tests {
		if got := policyCategoryToProto(tt.input); got != tt.want {
			t.Errorf("policyCategoryToProto(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestPolicySeverityToProto(t *testing.T) {
	tests := []struct {
		input policy.Severity
		want  pb.PolicySeverity
	}{
		{policy.SeverityLow, pb.PolicySeverity_POLICY_SEVERITY_LOW},
		{policy.SeverityMedium, pb.PolicySeverity_POLICY_SEVERITY_MEDIUM},
		{policy.SeverityHigh, pb.PolicySeverity_POLICY_SEVERITY_HIGH},
		{policy.SeverityCritical, pb.PolicySeverity_POLICY_SEVERITY_CRITICAL},
		{"unknown", pb.PolicySeverity_POLICY_SEVERITY_UNSPECIFIED},
	}
	for _, tt := range tests {
		if got := policySeverityToProto(tt.input); got != tt.want {
			t.Errorf("policySeverityToProto(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestEnforcementModeToProto(t *testing.T) {
	tests := []struct {
		input policy.EnforcementMode
		want  pb.EnforcementMode
	}{
		{policy.ModeAudit, pb.EnforcementMode_ENFORCEMENT_MODE_AUDIT},
		{policy.ModeWarn, pb.EnforcementMode_ENFORCEMENT_MODE_WARN},
		{policy.ModeEnforce, pb.EnforcementMode_ENFORCEMENT_MODE_ENFORCE},
		{"unknown", pb.EnforcementMode_ENFORCEMENT_MODE_UNSPECIFIED},
	}
	for _, tt := range tests {
		if got := enforcementModeToProto(tt.input); got != tt.want {
			t.Errorf("enforcementModeToProto(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestHasAllTags(t *testing.T) {
	tests := []struct {
		policyTags []string
		filterTags []string
		want       bool
	}{
		{[]string{"a", "b", "c"}, []string{"a"}, true},
		{[]string{"a", "b"}, []string{"a", "b"}, true},
		{[]string{"a"}, []string{"a", "b"}, false},
		{nil, []string{"a"}, false},
		{[]string{"a"}, nil, true},
	}
	for _, tt := range tests {
		if got := hasAllTags(tt.policyTags, tt.filterTags); got != tt.want {
			t.Errorf("hasAllTags(%v, %v) = %v, want %v", tt.policyTags, tt.filterTags, got, tt.want)
		}
	}
}
