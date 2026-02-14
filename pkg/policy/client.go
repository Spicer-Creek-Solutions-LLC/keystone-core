package policy

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// Client provides access to the Keystone Core policy API over gRPC.
type Client struct {
	conn   *grpc.ClientConn
	client pb.PolicyServiceClient
}

// NewClient creates a new policy client that connects to the given address.
// The caller must call Close when done.
func NewClient(addr string, opts ...grpc.DialOption) (*Client, error) {
	if len(opts) == 0 {
		opts = append(opts, grpc.WithTransportCredentials(insecure.NewCredentials()))
	}
	conn, err := grpc.NewClient(addr, opts...)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:   conn,
		client: pb.NewPolicyServiceClient(conn),
	}, nil
}

// NewClientFromConn creates a new policy client from an existing gRPC connection.
// The caller retains ownership of the connection; Close is a no-op.
func NewClientFromConn(conn grpc.ClientConnInterface) *Client {
	return &Client{
		client: pb.NewPolicyServiceClient(conn),
	}
}

// Close closes the underlying gRPC connection.
// No-op if the client was created with NewClientFromConn.
func (c *Client) Close() error {
	if c.conn != nil {
		return c.conn.Close()
	}
	return nil
}

// GetAuditLog retrieves policy evaluation audit log entries with optional filters.
func (c *Client) GetAuditLog(ctx context.Context, opts AuditLogOptions) (*AuditLogResult, error) {
	req := &pb.GetAuditLogRequest{
		PolicyId:     opts.PolicyID,
		User:         opts.User,
		Action:       opts.Action,
		ResourceType: opts.ResourceType,
		PageSize:     opts.PageSize,
		PageToken:    opts.PageToken,
	}
	if opts.Allowed != nil {
		req.Allowed = &pb.OptionalBool{Value: *opts.Allowed}
	}
	if !opts.StartTime.IsZero() {
		req.StartTime = timestamppb.New(opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		req.EndTime = timestamppb.New(opts.EndTime)
	}

	resp, err := c.client.GetAuditLog(ctx, req)
	if err != nil {
		return nil, grpcStatusToError(err)
	}

	entries := make([]AuditEntry, len(resp.GetEntries()))
	for i, e := range resp.GetEntries() {
		entries[i] = auditEntryFromProto(e)
	}

	return &AuditLogResult{
		Entries:       entries,
		NextPageToken: resp.GetNextPageToken(),
		TotalCount:    int(resp.GetTotalCount()),
	}, nil
}

// GetComplianceReport generates a compliance report for the given options.
func (c *Client) GetComplianceReport(ctx context.Context, opts ComplianceReportOptions) (*ComplianceReport, error) {
	req := &pb.GetComplianceReportRequest{
		PolicySetId:       opts.PolicySetID,
		IncludeViolations: opts.IncludeViolations,
	}
	if !opts.StartTime.IsZero() {
		req.StartTime = timestamppb.New(opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		req.EndTime = timestamppb.New(opts.EndTime)
	}

	resp, err := c.client.GetComplianceReport(ctx, req)
	if err != nil {
		return nil, grpcStatusToError(err)
	}

	return complianceReportFromProto(resp), nil
}

// ListViolations lists policy violations with optional filters.
func (c *Client) ListViolations(ctx context.Context, opts ViolationListOptions) (*ViolationListResult, error) {
	req := &pb.ListViolationsRequest{
		PolicyId:     opts.PolicyID,
		ResourceType: opts.ResourceType,
		User:         opts.User,
		PageSize:     opts.PageSize,
		PageToken:    opts.PageToken,
	}
	if opts.Severity != "" {
		req.Severity = severityToProto(opts.Severity)
	}
	if !opts.StartTime.IsZero() {
		req.StartTime = timestamppb.New(opts.StartTime)
	}
	if !opts.EndTime.IsZero() {
		req.EndTime = timestamppb.New(opts.EndTime)
	}

	resp, err := c.client.ListViolations(ctx, req)
	if err != nil {
		return nil, grpcStatusToError(err)
	}

	records := make([]ViolationRecord, len(resp.GetRecords()))
	for i, r := range resp.GetRecords() {
		records[i] = violationRecordFromProto(r)
	}

	return &ViolationListResult{
		Records:       records,
		NextPageToken: resp.GetNextPageToken(),
		TotalCount:    int(resp.GetTotalCount()),
	}, nil
}

// EvaluatePolicy evaluates a single policy against the given input.
func (c *Client) EvaluatePolicy(ctx context.Context, policyID string, opts EvalOptions) (*EvalResult, error) {
	input := &pb.EvaluationInput{
		Action:  opts.Action,
		User:    opts.User,
		Context: opts.Context,
	}
	if opts.Resource != nil {
		pbResource, err := structpb.NewStruct(opts.Resource)
		if err != nil {
			return nil, err
		}
		input.Resource = pbResource
	}

	resp, err := c.client.EvaluatePolicy(ctx, &pb.EvaluatePolicyRequest{
		PolicyId: policyID,
		Input:    input,
	})
	if err != nil {
		return nil, grpcStatusToError(err)
	}

	return evalResultFromProto(resp.GetResult()), nil
}

// auditEntryFromProto converts a proto AuditEntry to a public AuditEntry.
func auditEntryFromProto(e *pb.AuditEntry) AuditEntry {
	if e == nil {
		return AuditEntry{}
	}
	entry := AuditEntry{
		ID:              e.GetId(),
		PolicyID:        e.GetPolicyId(),
		PolicyName:      e.GetPolicyName(),
		ResourceType:    e.GetResourceType(),
		Allowed:         e.GetAllowed(),
		DurationMS:      e.GetDurationMs(),
		EnforcementMode: e.GetEnforcementMode().String(),
		User:            e.GetUser(),
		Action:          e.GetAction(),
		Metadata:        e.GetMetadata(),
	}
	if e.GetTimestamp() != nil {
		entry.Timestamp = e.GetTimestamp().AsTime()
	}
	if e.GetPolicyType() != pb.PolicyType_POLICY_TYPE_UNSPECIFIED {
		entry.PolicyType = e.GetPolicyType().String()
	}
	entry.Violations = make([]Violation, len(e.GetViolations()))
	for i, v := range e.GetViolations() {
		entry.Violations[i] = violationFromProto(v)
	}
	return entry
}

// violationFromProto converts a proto Violation to a public Violation.
func violationFromProto(v *pb.Violation) Violation {
	if v == nil {
		return Violation{}
	}
	return Violation{
		Rule:        v.GetRule(),
		Message:     v.GetMessage(),
		Severity:    severityFromProto(v.GetSeverity()),
		Path:        v.GetPath(),
		Expected:    v.GetExpected(),
		Actual:      v.GetActual(),
		Remediation: v.GetRemediation(),
	}
}

// violationRecordFromProto converts a proto ViolationRecord to a public ViolationRecord.
func violationRecordFromProto(r *pb.ViolationRecord) ViolationRecord {
	if r == nil {
		return ViolationRecord{}
	}
	rec := ViolationRecord{
		ID:           r.GetId(),
		PolicyID:     r.GetPolicyId(),
		PolicyName:   r.GetPolicyName(),
		ResourceType: r.GetResourceType(),
		User:         r.GetUser(),
	}
	if r.GetTimestamp() != nil {
		rec.Timestamp = r.GetTimestamp().AsTime()
	}
	if r.GetViolation() != nil {
		rec.Violation = violationFromProto(r.GetViolation())
	}
	return rec
}

// complianceReportFromProto converts a proto GetComplianceReportResponse to a public ComplianceReport.
func complianceReportFromProto(resp *pb.GetComplianceReportResponse) *ComplianceReport {
	if resp == nil {
		return &ComplianceReport{}
	}
	report := &ComplianceReport{
		ComplianceRate:          float64(resp.GetComplianceRate()),
		TotalEvaluations:        resp.GetTotalEvaluations(),
		CompliantEvaluations:    resp.GetCompliantEvaluations(),
		NonCompliantEvaluations: resp.GetNonCompliantEvaluations(),
		ViolationsBySeverity:    resp.GetViolationsBySeverity(),
	}
	if resp.GetPeriod() != nil {
		if resp.GetPeriod().GetStartTime() != nil {
			report.StartTime = resp.GetPeriod().GetStartTime().AsTime()
		}
		if resp.GetPeriod().GetEndTime() != nil {
			report.EndTime = resp.GetPeriod().GetEndTime().AsTime()
		}
	}
	report.PolicyStats = make([]ComplianceStats, len(resp.GetPolicyStats()))
	for i, s := range resp.GetPolicyStats() {
		report.PolicyStats[i] = ComplianceStats{
			PolicyID:             s.GetPolicyId(),
			PolicyName:           s.GetPolicyName(),
			TotalEvaluations:     s.GetTotalEvaluations(),
			CompliantEvaluations: s.GetCompliantEvaluations(),
			ComplianceRate:       float64(s.GetComplianceRate()),
			ViolationCount:       s.GetViolationCount(),
		}
	}
	report.TopViolations = make([]ViolationSummary, len(resp.GetTopViolations()))
	for i, v := range resp.GetTopViolations() {
		report.TopViolations[i] = ViolationSummary{
			PolicyID:   v.GetPolicyId(),
			PolicyName: v.GetPolicyName(),
			Rule:       v.GetRule(),
			Count:      v.GetCount(),
			Severity:   severityFromProto(v.GetSeverity()),
		}
	}
	return report
}

// evalResultFromProto converts a proto EvaluationResult to a public EvalResult.
func evalResultFromProto(r *pb.EvaluationResult) *EvalResult {
	if r == nil {
		return &EvalResult{}
	}
	result := &EvalResult{
		PolicyID:   r.GetPolicyId(),
		PolicyName: r.GetPolicyName(),
		Allowed:    r.GetAllowed(),
		Warnings:   r.GetWarnings(),
		Message:    r.GetMessage(),
		DurationMS: r.GetDurationMs(),
	}
	if r.GetEvaluatedAt() != nil {
		result.EvaluatedAt = r.GetEvaluatedAt().AsTime()
	}
	result.Violations = make([]Violation, len(r.GetViolations()))
	for i, v := range r.GetViolations() {
		result.Violations[i] = violationFromProto(v)
	}
	return result
}

// severityFromProto converts a proto PolicySeverity to a string.
func severityFromProto(s pb.PolicySeverity) string {
	switch s {
	case pb.PolicySeverity_POLICY_SEVERITY_LOW:
		return "low"
	case pb.PolicySeverity_POLICY_SEVERITY_MEDIUM:
		return "medium"
	case pb.PolicySeverity_POLICY_SEVERITY_HIGH:
		return "high"
	case pb.PolicySeverity_POLICY_SEVERITY_CRITICAL:
		return "critical"
	default:
		return "unspecified"
	}
}

// severityToProto converts a severity string to a proto PolicySeverity.
func severityToProto(s string) pb.PolicySeverity {
	switch s {
	case "low":
		return pb.PolicySeverity_POLICY_SEVERITY_LOW
	case "medium":
		return pb.PolicySeverity_POLICY_SEVERITY_MEDIUM
	case "high":
		return pb.PolicySeverity_POLICY_SEVERITY_HIGH
	case "critical":
		return pb.PolicySeverity_POLICY_SEVERITY_CRITICAL
	default:
		return pb.PolicySeverity_POLICY_SEVERITY_UNSPECIFIED
	}
}
