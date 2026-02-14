package policy

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// mockPolicyServiceClient implements pb.PolicyServiceClient for testing.
type mockPolicyServiceClient struct {
	pb.PolicyServiceClient

	getAuditLogResp       *pb.GetAuditLogResponse
	getAuditLogErr        error
	getComplianceResp     *pb.GetComplianceReportResponse
	getComplianceErr      error
	listViolationsResp    *pb.ListViolationsResponse
	listViolationsErr     error
	evaluatePolicyResp    *pb.EvaluatePolicyResponse
	evaluatePolicyErr     error
}

func (m *mockPolicyServiceClient) GetAuditLog(_ context.Context, _ *pb.GetAuditLogRequest, _ ...grpc.CallOption) (*pb.GetAuditLogResponse, error) {
	return m.getAuditLogResp, m.getAuditLogErr
}

func (m *mockPolicyServiceClient) GetComplianceReport(_ context.Context, _ *pb.GetComplianceReportRequest, _ ...grpc.CallOption) (*pb.GetComplianceReportResponse, error) {
	return m.getComplianceResp, m.getComplianceErr
}

func (m *mockPolicyServiceClient) ListViolations(_ context.Context, _ *pb.ListViolationsRequest, _ ...grpc.CallOption) (*pb.ListViolationsResponse, error) {
	return m.listViolationsResp, m.listViolationsErr
}

func (m *mockPolicyServiceClient) EvaluatePolicy(_ context.Context, _ *pb.EvaluatePolicyRequest, _ ...grpc.CallOption) (*pb.EvaluatePolicyResponse, error) {
	return m.evaluatePolicyResp, m.evaluatePolicyErr
}

func TestGetAuditLog(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	mock := &mockPolicyServiceClient{
		getAuditLogResp: &pb.GetAuditLogResponse{
			Entries: []*pb.AuditEntry{
				{
					Id:           "entry-1",
					Timestamp:    timestamppb.New(now),
					PolicyId:     "pol-1",
					PolicyName:   "security-no-root",
					ResourceType: "container",
					Allowed:      false,
					DurationMs:   5,
					User:         "admin",
					Violations: []*pb.Violation{
						{Rule: "no-root", Message: "runs as root", Severity: pb.PolicySeverity_POLICY_SEVERITY_HIGH},
					},
				},
			},
			NextPageToken: "token-2",
			TotalCount:    10,
		},
	}

	c := &Client{client: mock}
	denied := false
	result, err := c.GetAuditLog(context.Background(), AuditLogOptions{
		PolicyID:  "pol-1",
		Allowed:   &denied,
		StartTime: now.Add(-time.Hour),
		EndTime:   now,
		PageSize:  50,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(result.Entries))
	}
	if result.Entries[0].PolicyID != "pol-1" {
		t.Errorf("expected policy ID pol-1, got %s", result.Entries[0].PolicyID)
	}
	if result.Entries[0].Allowed {
		t.Error("expected Allowed=false")
	}
	if len(result.Entries[0].Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(result.Entries[0].Violations))
	}
	if result.Entries[0].Violations[0].Severity != "high" {
		t.Errorf("expected severity high, got %s", result.Entries[0].Violations[0].Severity)
	}
	if result.NextPageToken != "token-2" {
		t.Errorf("expected next page token-2, got %s", result.NextPageToken)
	}
	if result.TotalCount != 10 {
		t.Errorf("expected total count 10, got %d", result.TotalCount)
	}
}

func TestGetAuditLog_Error(t *testing.T) {
	mock := &mockPolicyServiceClient{
		getAuditLogErr: status.Error(codes.Unavailable, "service down"),
	}
	c := &Client{client: mock}
	_, err := c.GetAuditLog(context.Background(), AuditLogOptions{})
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

func TestGetComplianceReport(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	start := now.Add(-7 * 24 * time.Hour)
	mock := &mockPolicyServiceClient{
		getComplianceResp: &pb.GetComplianceReportResponse{
			Period: &pb.ReportPeriod{
				StartTime: timestamppb.New(start),
				EndTime:   timestamppb.New(now),
			},
			ComplianceRate:          85.5,
			TotalEvaluations:        1000,
			CompliantEvaluations:    855,
			NonCompliantEvaluations: 145,
			PolicyStats: []*pb.PolicyComplianceStats{
				{PolicyId: "pol-1", PolicyName: "sec-policy", ComplianceRate: 90.0, TotalEvaluations: 500, CompliantEvaluations: 450, ViolationCount: 50},
			},
			TopViolations: []*pb.ViolationSummary{
				{PolicyId: "pol-1", PolicyName: "sec-policy", Rule: "no-root", Count: 42, Severity: pb.PolicySeverity_POLICY_SEVERITY_HIGH},
			},
			ViolationsBySeverity: map[string]int64{"high": 42, "low": 103},
		},
	}

	c := &Client{client: mock}
	report, err := c.GetComplianceReport(context.Background(), ComplianceReportOptions{
		StartTime: start,
		EndTime:   now,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if report.ComplianceRate != 85.5 {
		t.Errorf("expected 85.5, got %f", report.ComplianceRate)
	}
	if report.TotalEvaluations != 1000 {
		t.Errorf("expected 1000, got %d", report.TotalEvaluations)
	}
	if len(report.PolicyStats) != 1 {
		t.Fatalf("expected 1 policy stat, got %d", len(report.PolicyStats))
	}
	if report.PolicyStats[0].PolicyID != "pol-1" {
		t.Errorf("expected pol-1, got %s", report.PolicyStats[0].PolicyID)
	}
	if len(report.TopViolations) != 1 {
		t.Fatalf("expected 1 top violation, got %d", len(report.TopViolations))
	}
	if report.TopViolations[0].Count != 42 {
		t.Errorf("expected count 42, got %d", report.TopViolations[0].Count)
	}
	if report.ViolationsBySeverity["high"] != 42 {
		t.Errorf("expected 42 high violations, got %d", report.ViolationsBySeverity["high"])
	}
}

func TestGetComplianceReport_Error(t *testing.T) {
	mock := &mockPolicyServiceClient{
		getComplianceErr: status.Error(codes.Unimplemented, "not implemented"),
	}
	c := &Client{client: mock}
	_, err := c.GetComplianceReport(context.Background(), ComplianceReportOptions{})
	if !errors.Is(err, ErrUnimplemented) {
		t.Errorf("expected ErrUnimplemented, got %v", err)
	}
}

func TestListViolations(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	mock := &mockPolicyServiceClient{
		listViolationsResp: &pb.ListViolationsResponse{
			Records: []*pb.ViolationRecord{
				{
					Id:           "vr-1",
					Timestamp:    timestamppb.New(now),
					PolicyId:     "pol-1",
					PolicyName:   "sec-policy",
					ResourceType: "container",
					User:         "admin",
					Violation: &pb.Violation{
						Rule:     "no-root",
						Message:  "runs as root",
						Severity: pb.PolicySeverity_POLICY_SEVERITY_HIGH,
					},
				},
			},
			NextPageToken: "vr-next",
			TotalCount:    5,
		},
	}

	c := &Client{client: mock}
	result, err := c.ListViolations(context.Background(), ViolationListOptions{
		PolicyID: "pol-1",
		Severity: "high",
		PageSize: 20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Records) != 1 {
		t.Fatalf("expected 1 record, got %d", len(result.Records))
	}
	if result.Records[0].PolicyID != "pol-1" {
		t.Errorf("expected pol-1, got %s", result.Records[0].PolicyID)
	}
	if result.Records[0].Violation.Severity != "high" {
		t.Errorf("expected high, got %s", result.Records[0].Violation.Severity)
	}
	if result.TotalCount != 5 {
		t.Errorf("expected total 5, got %d", result.TotalCount)
	}
}

func TestListViolations_Error(t *testing.T) {
	mock := &mockPolicyServiceClient{
		listViolationsErr: status.Error(codes.NotFound, "not found"),
	}
	c := &Client{client: mock}
	_, err := c.ListViolations(context.Background(), ViolationListOptions{})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestEvaluatePolicy(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	mock := &mockPolicyServiceClient{
		evaluatePolicyResp: &pb.EvaluatePolicyResponse{
			Result: &pb.EvaluationResult{
				PolicyId:   "pol-1",
				PolicyName: "sec-policy",
				Allowed:    false,
				Message:    "policy violated",
				DurationMs: 3,
				EvaluatedAt: timestamppb.New(now),
				Violations: []*pb.Violation{
					{Rule: "no-root", Message: "runs as root", Severity: pb.PolicySeverity_POLICY_SEVERITY_CRITICAL},
				},
				Warnings: []string{"deprecated API"},
			},
		},
	}

	c := &Client{client: mock}
	result, err := c.EvaluatePolicy(context.Background(), "pol-1", EvalOptions{
		Resource: map[string]interface{}{"name": "test-pod"},
		Action:   "deploy",
		User:     "admin",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Allowed {
		t.Error("expected not allowed")
	}
	if result.PolicyID != "pol-1" {
		t.Errorf("expected pol-1, got %s", result.PolicyID)
	}
	if len(result.Violations) != 1 {
		t.Fatalf("expected 1 violation, got %d", len(result.Violations))
	}
	if result.Violations[0].Severity != "critical" {
		t.Errorf("expected critical, got %s", result.Violations[0].Severity)
	}
	if len(result.Warnings) != 1 {
		t.Fatalf("expected 1 warning, got %d", len(result.Warnings))
	}
	if result.DurationMS != 3 {
		t.Errorf("expected 3ms, got %d", result.DurationMS)
	}
}

func TestEvaluatePolicy_Error(t *testing.T) {
	mock := &mockPolicyServiceClient{
		evaluatePolicyErr: status.Error(codes.InvalidArgument, "bad input"),
	}
	c := &Client{client: mock}
	_, err := c.EvaluatePolicy(context.Background(), "pol-1", EvalOptions{})
	if !errors.Is(err, ErrInvalidArgument) {
		t.Errorf("expected ErrInvalidArgument, got %v", err)
	}
}

func TestSeverityConversion(t *testing.T) {
	cases := []struct {
		str   string
		proto pb.PolicySeverity
	}{
		{"low", pb.PolicySeverity_POLICY_SEVERITY_LOW},
		{"medium", pb.PolicySeverity_POLICY_SEVERITY_MEDIUM},
		{"high", pb.PolicySeverity_POLICY_SEVERITY_HIGH},
		{"critical", pb.PolicySeverity_POLICY_SEVERITY_CRITICAL},
		{"unknown", pb.PolicySeverity_POLICY_SEVERITY_UNSPECIFIED},
	}
	for _, tc := range cases {
		got := severityToProto(tc.str)
		if got != tc.proto {
			t.Errorf("severityToProto(%q) = %v, want %v", tc.str, got, tc.proto)
		}
		back := severityFromProto(tc.proto)
		if tc.str != "unknown" && back != tc.str {
			t.Errorf("severityFromProto(%v) = %q, want %q", tc.proto, back, tc.str)
		}
	}
}

func TestGrpcStatusToError(t *testing.T) {
	cases := []struct {
		code codes.Code
		want error
	}{
		{codes.OK, nil},
		{codes.NotFound, ErrNotFound},
		{codes.PermissionDenied, ErrAccessDenied},
		{codes.InvalidArgument, ErrInvalidArgument},
		{codes.Unavailable, ErrUnavailable},
		{codes.Unauthenticated, ErrUnauthenticated},
		{codes.Unimplemented, ErrUnimplemented},
		{codes.AlreadyExists, ErrAlreadyExists},
		{codes.ResourceExhausted, ErrResourceExhausted},
	}
	for _, tc := range cases {
		err := grpcStatusToError(status.Error(tc.code, "test"))
		if !errors.Is(err, tc.want) {
			t.Errorf("code %v: got %v, want %v", tc.code, err, tc.want)
		}
	}

	if grpcStatusToError(nil) != nil {
		t.Error("expected nil for nil input")
	}
}

func TestCloseNoop(t *testing.T) {
	c := &Client{}
	if err := c.Close(); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}

func TestNilProtoConversions(t *testing.T) {
	entry := auditEntryFromProto(nil)
	if entry.ID != "" {
		t.Error("expected empty for nil entry")
	}

	v := violationFromProto(nil)
	if v.Rule != "" {
		t.Error("expected empty for nil violation")
	}

	rec := violationRecordFromProto(nil)
	if rec.ID != "" {
		t.Error("expected empty for nil record")
	}

	report := complianceReportFromProto(nil)
	if report.TotalEvaluations != 0 {
		t.Error("expected 0 for nil report")
	}

	result := evalResultFromProto(nil)
	if result.PolicyID != "" {
		t.Error("expected empty for nil result")
	}
}
