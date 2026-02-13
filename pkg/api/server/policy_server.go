package server

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/shawnbutts/keystone-core/internal/policy"
	pb "github.com/shawnbutts/keystone-core/pkg/api/v1"
)

// PolicyRegistry provides CRUD operations for policies and policy sets.
type PolicyRegistry interface {
	RegisterPolicy(p *policy.Policy) error
	GetPolicy(id string) (*policy.Policy, bool)
	ListPolicies() []*policy.Policy
	ListPoliciesByCategory(category policy.Category) []*policy.Policy
	ListPoliciesByType(policyType policy.Type) []*policy.Policy
	UpdatePolicy(p *policy.Policy) error
	DeletePolicy(id string) error
	RegisterPolicySet(set *policy.Set) error
	GetPolicySet(id string) (*policy.Set, bool)
	ListPolicySets() []*policy.Set
}

// PolicyEvaluator evaluates policies against input data.
type PolicyEvaluator interface {
	Evaluate(ctx context.Context, policyID string, input *policy.EvaluationInput) (*policy.EvaluationResult, error)
	EvaluatePolicySet(ctx context.Context, setID string, input *policy.EvaluationInput) (*policy.Result, error)
}

// PolicyAuditor provides access to policy evaluation audit data.
type PolicyAuditor interface {
	GetEntries(filter *policy.AuditFilter) []policy.AuditEntry
}

// PolicyComplianceReporter generates compliance reports.
type PolicyComplianceReporter interface {
	GenerateReport(period policy.ReportPeriod) *policy.ComplianceReport
}

// PolicyServer implements the PolicyService gRPC server.
type PolicyServer struct {
	pb.UnimplementedPolicyServiceServer
	registry   PolicyRegistry
	evaluator  PolicyEvaluator
	auditor    PolicyAuditor
	reporter   PolicyComplianceReporter
}

// NewPolicyServer creates a new PolicyServer.
// Any dependency may be nil — RPCs return codes.Unavailable if the required dep is nil.
func NewPolicyServer(registry PolicyRegistry, evaluator PolicyEvaluator, auditor PolicyAuditor, reporter PolicyComplianceReporter) *PolicyServer {
	return &PolicyServer{
		registry:  registry,
		evaluator: evaluator,
		auditor:   auditor,
		reporter:  reporter,
	}
}

// EvaluatePolicy evaluates a single policy against input data.
func (s *PolicyServer) EvaluatePolicy(ctx context.Context, req *pb.EvaluatePolicyRequest) (*pb.EvaluatePolicyResponse, error) {
	if s.evaluator == nil {
		return nil, status.Error(codes.Unavailable, "policy evaluator not available")
	}
	if req.PolicyId == "" {
		return nil, status.Error(codes.InvalidArgument, "policy_id is required")
	}
	if req.Input == nil {
		return nil, status.Error(codes.InvalidArgument, "input is required")
	}

	input := protoToEvaluationInput(req.Input)
	result, err := s.evaluator.Evaluate(ctx, req.PolicyId, input)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "policy evaluation failed: %v", err)
	}

	return &pb.EvaluatePolicyResponse{
		Result: evaluationResultToProto(result),
	}, nil
}

// EvaluatePolicySet evaluates all policies in a policy set.
func (s *PolicyServer) EvaluatePolicySet(ctx context.Context, req *pb.EvaluatePolicySetRequest) (*pb.EvaluatePolicySetResponse, error) {
	if s.evaluator == nil {
		return nil, status.Error(codes.Unavailable, "policy evaluator not available")
	}
	if req.PolicySetId == "" {
		return nil, status.Error(codes.InvalidArgument, "policy_set_id is required")
	}
	if req.Input == nil {
		return nil, status.Error(codes.InvalidArgument, "input is required")
	}

	input := protoToEvaluationInput(req.Input)
	result, err := s.evaluator.EvaluatePolicySet(ctx, req.PolicySetId, input)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "policy set evaluation failed: %v", err)
	}

	resp := &pb.EvaluatePolicySetResponse{
		Allowed:        result.Allowed,
		TotalDurationMs: result.TotalDuration.Milliseconds(),
		EvaluatedAt:    timestamppb.New(result.EvaluatedAt),
	}

	for _, r := range result.Results {
		resp.Results = append(resp.Results, evaluationResultToProto(r))
	}

	if result.Summary != nil {
		resp.Summary = policySummaryToProto(result.Summary)
	}

	return resp, nil
}

// ListPolicies lists all policies with optional filtering.
func (s *PolicyServer) ListPolicies(_ context.Context, req *pb.ListPoliciesRequest) (*pb.ListPoliciesResponse, error) {
	if s.registry == nil {
		return nil, status.Error(codes.Unavailable, "policy registry not available")
	}

	var policies []*policy.Policy
	switch {
	case req.Category != pb.PolicyCategory_POLICY_CATEGORY_UNSPECIFIED:
		policies = s.registry.ListPoliciesByCategory(protoCategoryToInternal(req.Category))
	case req.Type != pb.PolicyType_POLICY_TYPE_UNSPECIFIED:
		policies = s.registry.ListPoliciesByType(protoTypeToInternal(req.Type))
	default:
		policies = s.registry.ListPolicies()
	}

	// Apply additional filters
	filtered := policies[:0:0]
	for _, p := range policies {
		if req.Enabled != nil && p.Enabled != req.Enabled.Value {
			continue
		}
		if len(req.Tags) > 0 && !hasAllTags(p.Tags, req.Tags) {
			continue
		}
		// If both category and type were specified, apply type filter too
		if req.Category != pb.PolicyCategory_POLICY_CATEGORY_UNSPECIFIED &&
			req.Type != pb.PolicyType_POLICY_TYPE_UNSPECIFIED &&
			p.Type != protoTypeToInternal(req.Type) {
			continue
		}
		filtered = append(filtered, p)
	}

	// Pagination
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := 0
	if req.PageToken != "" {
		offset = parsePageToken(req.PageToken)
	}

	total := len(filtered)
	end := offset + pageSize
	if end > total {
		end = total
	}
	var page []*policy.Policy
	if offset < total {
		page = filtered[offset:end]
	}

	resp := &pb.ListPoliciesResponse{
		TotalCount: int32(total), //nolint:gosec // G115: bounded by policy count
	}
	for _, p := range page {
		resp.Policies = append(resp.Policies, policyToProto(p))
	}
	if end < total {
		resp.NextPageToken = encodePageToken(end)
	}

	return resp, nil
}

// GetPolicy retrieves a specific policy by ID.
func (s *PolicyServer) GetPolicy(_ context.Context, req *pb.GetPolicyRequest) (*pb.GetPolicyResponse, error) {
	if s.registry == nil {
		return nil, status.Error(codes.Unavailable, "policy registry not available")
	}
	if req.PolicyId == "" {
		return nil, status.Error(codes.InvalidArgument, "policy_id is required")
	}

	p, ok := s.registry.GetPolicy(req.PolicyId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "policy %q not found", req.PolicyId)
	}

	return &pb.GetPolicyResponse{
		Policy: policyToProto(p),
	}, nil
}

// CreatePolicy creates a new policy.
func (s *PolicyServer) CreatePolicy(_ context.Context, req *pb.CreatePolicyRequest) (*pb.CreatePolicyResponse, error) {
	if s.registry == nil {
		return nil, status.Error(codes.Unavailable, "policy registry not available")
	}
	if req.Policy == nil {
		return nil, status.Error(codes.InvalidArgument, "policy is required")
	}

	p := protoToPolicy(req.Policy)
	now := time.Now()
	if p.ID == "" {
		p.ID = fmt.Sprintf("policy-%d", now.UnixNano())
	}
	p.CreatedAt = now
	p.UpdatedAt = now

	if err := s.registry.RegisterPolicy(p); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to create policy: %v", err)
	}

	return &pb.CreatePolicyResponse{
		Policy: policyToProto(p),
	}, nil
}

// UpdatePolicy updates an existing policy.
func (s *PolicyServer) UpdatePolicy(_ context.Context, req *pb.UpdatePolicyRequest) (*pb.UpdatePolicyResponse, error) {
	if s.registry == nil {
		return nil, status.Error(codes.Unavailable, "policy registry not available")
	}
	if req.Policy == nil || req.Policy.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "policy with id is required")
	}

	existing, ok := s.registry.GetPolicy(req.Policy.Id)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "policy %q not found", req.Policy.Id)
	}

	updated := protoToPolicy(req.Policy)
	updated.CreatedAt = existing.CreatedAt
	updated.UpdatedAt = time.Now()

	if err := s.registry.UpdatePolicy(updated); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to update policy: %v", err)
	}

	return &pb.UpdatePolicyResponse{
		Policy: policyToProto(updated),
	}, nil
}

// DeletePolicy deletes a policy by ID.
func (s *PolicyServer) DeletePolicy(_ context.Context, req *pb.DeletePolicyRequest) (*pb.DeletePolicyResponse, error) {
	if s.registry == nil {
		return nil, status.Error(codes.Unavailable, "policy registry not available")
	}
	if req.PolicyId == "" {
		return nil, status.Error(codes.InvalidArgument, "policy_id is required")
	}

	if err := s.registry.DeletePolicy(req.PolicyId); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to delete policy: %v", err)
	}

	return &pb.DeletePolicyResponse{Deleted: true}, nil
}

// ListViolations lists policy violations from the audit log.
func (s *PolicyServer) ListViolations(_ context.Context, req *pb.ListViolationsRequest) (*pb.ListViolationsResponse, error) {
	if s.auditor == nil {
		return nil, status.Error(codes.Unavailable, "policy auditor not available")
	}

	filter := &policy.AuditFilter{
		PolicyID: req.PolicyId,
		User:     req.User,
	}
	if req.ResourceType != "" {
		filter.ResourceType = req.ResourceType
	}
	if req.StartTime != nil {
		filter.StartTime = req.StartTime.AsTime()
	}
	if req.EndTime != nil {
		filter.EndTime = req.EndTime.AsTime()
	}
	denied := false
	filter.Allowed = &denied

	entries := s.auditor.GetEntries(filter)

	// Filter by severity if specified
	var records []*pb.ViolationRecord
	for i := range entries {
		entry := &entries[i]
		for _, v := range entry.Violations {
			if req.Severity != pb.PolicySeverity_POLICY_SEVERITY_UNSPECIFIED &&
				policySeverityToProto(v.Severity) != req.Severity {
				continue
			}
			if req.MinSeverity != pb.PolicySeverity_POLICY_SEVERITY_UNSPECIFIED &&
				policySeverityToProto(v.Severity) < req.MinSeverity {
				continue
			}
			records = append(records, &pb.ViolationRecord{
				Id:              entry.ID,
				Timestamp:       timestamppb.New(entry.Timestamp),
				PolicyId:        entry.PolicyID,
				PolicyName:      entry.PolicyName,
				Violation:       violationToProto(&v),
				ResourceType:    entry.ResourceType,
				User:            entry.User,
				Action:          entry.Action,
				EnforcementMode: enforcementModeToProto(entry.EnforcementMode),
				Blocked:         entry.EnforcementMode == policy.ModeEnforce,
			})
		}
	}

	// Pagination
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := 0
	if req.PageToken != "" {
		offset = parsePageToken(req.PageToken)
	}

	total := len(records)
	end := offset + pageSize
	if end > total {
		end = total
	}
	var page []*pb.ViolationRecord
	if offset < total {
		page = records[offset:end]
	}

	resp := &pb.ListViolationsResponse{
		Records:    page,
		TotalCount: int32(total), //nolint:gosec // G115: bounded by violation count
	}
	if end < total {
		resp.NextPageToken = encodePageToken(end)
	}

	return resp, nil
}

// GetComplianceReport generates a compliance report.
func (s *PolicyServer) GetComplianceReport(_ context.Context, req *pb.GetComplianceReportRequest) (*pb.GetComplianceReportResponse, error) {
	if s.reporter == nil {
		return nil, status.Error(codes.Unavailable, "compliance reporter not available")
	}

	period := policy.ReportPeriod{}
	if req.StartTime != nil {
		period.Start = req.StartTime.AsTime()
	}
	if req.EndTime != nil {
		period.End = req.EndTime.AsTime()
	}

	report := s.reporter.GenerateReport(period)

	resp := &pb.GetComplianceReportResponse{
		Period: &pb.ReportPeriod{
			StartTime: timestamppb.New(report.Period.Start),
			EndTime:   timestamppb.New(report.Period.End),
		},
		ComplianceRate:          float32(report.ComplianceRate),
		TotalEvaluations:        int64(report.TotalPolicies),
		CompliantEvaluations:    int64(report.CompliantPolicies),
		NonCompliantEvaluations: int64(report.ViolatingPolicies),
	}

	for _, ps := range report.PolicyResults {
		resp.PolicyStats = append(resp.PolicyStats, &pb.PolicyComplianceStats{
			TotalEvaluations:     int64(ps.TotalPolicies),
			CompliantEvaluations: int64(ps.AllowedPolicies),
			ComplianceRate:       safeComplianceRate(ps.AllowedPolicies, ps.TotalPolicies),
			ViolationCount:       int64(ps.TotalViolations),
		})
	}

	for _, vs := range report.TopViolations {
		resp.TopViolations = append(resp.TopViolations, &pb.ViolationSummary{
			PolicyId:   vs.PolicyID,
			PolicyName: vs.PolicyName,
			Count:      int64(vs.Count),
			Severity:   policySeverityToProto(vs.Severity),
		})
	}

	if len(report.ViolationsBySeverity) > 0 {
		resp.ViolationsBySeverity = make(map[string]int64, len(report.ViolationsBySeverity))
		for sev, count := range report.ViolationsBySeverity {
			resp.ViolationsBySeverity[string(sev)] = int64(count)
		}
	}

	return resp, nil
}

// GetAuditLog retrieves policy evaluation audit log entries.
func (s *PolicyServer) GetAuditLog(_ context.Context, req *pb.GetAuditLogRequest) (*pb.GetAuditLogResponse, error) {
	if s.auditor == nil {
		return nil, status.Error(codes.Unavailable, "policy auditor not available")
	}

	filter := &policy.AuditFilter{
		PolicyID:     req.PolicyId,
		User:         req.User,
		Action:       req.Action,
		ResourceType: req.ResourceType,
	}
	if req.Allowed != nil {
		allowed := req.Allowed.Value
		filter.Allowed = &allowed
	}
	if req.StartTime != nil {
		filter.StartTime = req.StartTime.AsTime()
	}
	if req.EndTime != nil {
		filter.EndTime = req.EndTime.AsTime()
	}

	entries := s.auditor.GetEntries(filter)

	// Pagination
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := 0
	if req.PageToken != "" {
		offset = parsePageToken(req.PageToken)
	}

	total := len(entries)
	end := offset + pageSize
	if end > total {
		end = total
	}
	var page []policy.AuditEntry
	if offset < total {
		page = entries[offset:end]
	}

	resp := &pb.GetAuditLogResponse{
		TotalCount: int32(total), //nolint:gosec // G115: bounded by audit entry count
	}
	for i := range page {
		resp.Entries = append(resp.Entries, auditEntryToProto(&page[i]))
	}
	if end < total {
		resp.NextPageToken = encodePageToken(end)
	}

	return resp, nil
}

// ListPolicySets lists all policy sets.
func (s *PolicyServer) ListPolicySets(_ context.Context, req *pb.ListPolicySetsRequest) (*pb.ListPolicySetsResponse, error) {
	if s.registry == nil {
		return nil, status.Error(codes.Unavailable, "policy registry not available")
	}

	sets := s.registry.ListPolicySets()

	// Filter by enabled
	if req.Enabled != nil {
		filtered := sets[:0:0]
		for _, set := range sets {
			if set.Enabled == req.Enabled.Value {
				filtered = append(filtered, set)
			}
		}
		sets = filtered
	}

	// Pagination
	pageSize := int(req.PageSize)
	if pageSize <= 0 {
		pageSize = 50
	}
	offset := 0
	if req.PageToken != "" {
		offset = parsePageToken(req.PageToken)
	}

	total := len(sets)
	end := offset + pageSize
	if end > total {
		end = total
	}
	var page []*policy.Set
	if offset < total {
		page = sets[offset:end]
	}

	resp := &pb.ListPolicySetsResponse{
		TotalCount: int32(total), //nolint:gosec // G115: bounded by policy set count
	}
	for _, set := range page {
		resp.PolicySets = append(resp.PolicySets, policySetToProto(set))
	}
	if end < total {
		resp.NextPageToken = encodePageToken(end)
	}

	return resp, nil
}

// GetPolicySet retrieves a policy set and its policies.
func (s *PolicyServer) GetPolicySet(_ context.Context, req *pb.GetPolicySetRequest) (*pb.GetPolicySetResponse, error) {
	if s.registry == nil {
		return nil, status.Error(codes.Unavailable, "policy registry not available")
	}
	if req.PolicySetId == "" {
		return nil, status.Error(codes.InvalidArgument, "policy_set_id is required")
	}

	set, ok := s.registry.GetPolicySet(req.PolicySetId)
	if !ok {
		return nil, status.Errorf(codes.NotFound, "policy set %q not found", req.PolicySetId)
	}

	resp := &pb.GetPolicySetResponse{
		PolicySet: policySetToProto(set),
	}

	// Resolve policies in the set
	for _, pid := range set.Policies {
		if p, found := s.registry.GetPolicy(pid); found {
			resp.Policies = append(resp.Policies, policyToProto(p))
		}
	}

	return resp, nil
}

// --- Conversion helpers ---

func policyToProto(p *policy.Policy) *pb.Policy {
	proto := &pb.Policy{
		Id:              p.ID,
		Name:            p.Name,
		Description:     p.Description,
		Type:            policyTypeToProto(p.Type),
		Category:        policyCategoryToProto(p.Category),
		Severity:        policySeverityToProto(p.Severity),
		EnforcementMode: enforcementModeToProto(p.EnforcementMode),
		Policy:          p.Policy,
		Enabled:         p.Enabled,
		Tags:            p.Tags,
		Metadata:        p.Metadata,
	}
	if !p.CreatedAt.IsZero() {
		proto.CreatedAt = timestamppb.New(p.CreatedAt)
	}
	if !p.UpdatedAt.IsZero() {
		proto.UpdatedAt = timestamppb.New(p.UpdatedAt)
	}
	return proto
}

func protoToPolicy(p *pb.Policy) *policy.Policy {
	result := &policy.Policy{
		ID:              p.Id,
		Name:            p.Name,
		Description:     p.Description,
		Type:            protoTypeToInternal(p.Type),
		Category:        protoCategoryToInternal(p.Category),
		Severity:        protoSeverityToInternal(p.Severity),
		EnforcementMode: protoEnforcementToInternal(p.EnforcementMode),
		Policy:          p.Policy,
		Enabled:         p.Enabled,
		Tags:            p.Tags,
		Metadata:        p.Metadata,
	}
	if p.CreatedAt != nil {
		result.CreatedAt = p.CreatedAt.AsTime()
	}
	if p.UpdatedAt != nil {
		result.UpdatedAt = p.UpdatedAt.AsTime()
	}
	return result
}

func policySetToProto(s *policy.Set) *pb.PolicySet {
	proto := &pb.PolicySet{
		Id:              s.ID,
		Name:            s.Name,
		Description:     s.Description,
		PolicyIds:       s.Policies,
		EnforcementMode: enforcementModeToProto(s.EnforcementMode),
		Enabled:         s.Enabled,
	}
	if !s.CreatedAt.IsZero() {
		proto.CreatedAt = timestamppb.New(s.CreatedAt)
	}
	if !s.UpdatedAt.IsZero() {
		proto.UpdatedAt = timestamppb.New(s.UpdatedAt)
	}
	return proto
}

func evaluationResultToProto(r *policy.EvaluationResult) *pb.EvaluationResult {
	proto := &pb.EvaluationResult{
		PolicyId:    r.PolicyID,
		PolicyName:  r.PolicyName,
		Allowed:     r.Allowed,
		Warnings:    r.Warnings,
		Message:     r.Message,
		DurationMs:  r.Duration.Milliseconds(),
		EvaluatedAt: timestamppb.New(r.EvaluatedAt),
	}
	for i := range r.Violations {
		proto.Violations = append(proto.Violations, violationToProto(&r.Violations[i]))
	}
	return proto
}

func violationToProto(v *policy.Violation) *pb.Violation {
	return &pb.Violation{
		Rule:        v.Rule,
		Message:     v.Message,
		Severity:    policySeverityToProto(v.Severity),
		Path:        v.Path,
		Expected:    fmt.Sprintf("%v", v.Expected),
		Actual:      fmt.Sprintf("%v", v.Actual),
		Remediation: v.Remediation,
	}
}

func policySummaryToProto(s *policy.Summary) *pb.PolicyEvaluationSummary {
	proto := &pb.PolicyEvaluationSummary{
		TotalPolicies:   int32(s.TotalPolicies),   //nolint:gosec // G115: bounded
		AllowedPolicies: int32(s.AllowedPolicies),  //nolint:gosec // G115: bounded
		DeniedPolicies:  int32(s.DeniedPolicies),   //nolint:gosec // G115: bounded
		TotalViolations: int32(s.TotalViolations),  //nolint:gosec // G115: bounded
	}
	if len(s.ViolationsBySeverity) > 0 {
		proto.ViolationsBySeverity = make(map[string]int32, len(s.ViolationsBySeverity))
		for sev, count := range s.ViolationsBySeverity {
			proto.ViolationsBySeverity[string(sev)] = int32(count) //nolint:gosec // G115: bounded
		}
	}
	return proto
}

func auditEntryToProto(e *policy.AuditEntry) *pb.AuditEntry {
	proto := &pb.AuditEntry{
		Id:              e.ID,
		Timestamp:       timestamppb.New(e.Timestamp),
		PolicyId:        e.PolicyID,
		PolicyName:      e.PolicyName,
		PolicyType:      policyTypeToProto(e.PolicyType),
		ResourceType:    e.ResourceType,
		Allowed:         e.Allowed,
		DurationMs:      e.Duration.Milliseconds(),
		EnforcementMode: enforcementModeToProto(e.EnforcementMode),
		User:            e.User,
		Action:          e.Action,
	}
	for i := range e.Violations {
		proto.Violations = append(proto.Violations, violationToProto(&e.Violations[i]))
	}
	if len(e.Metadata) > 0 {
		proto.Metadata = make(map[string]string, len(e.Metadata))
		for k, v := range e.Metadata {
			proto.Metadata[k] = fmt.Sprintf("%v", v)
		}
	}
	return proto
}

func protoToEvaluationInput(input *pb.EvaluationInput) *policy.EvaluationInput {
	result := &policy.EvaluationInput{
		Action:    input.Action,
		User:      input.User,
		Timestamp: time.Now(),
	}
	if input.Resource != nil {
		result.Resource = input.Resource.AsMap()
	}
	if len(input.Context) > 0 {
		result.Context = make(map[string]interface{}, len(input.Context))
		for k, v := range input.Context {
			result.Context[k] = v
		}
	}
	return result
}

// --- Enum conversion helpers ---

func policyTypeToProto(t policy.Type) pb.PolicyType {
	switch t {
	case policy.TypeOPA:
		return pb.PolicyType_POLICY_TYPE_OPA
	case policy.TypeCEL:
		return pb.PolicyType_POLICY_TYPE_CEL
	case policy.TypeBuiltin:
		return pb.PolicyType_POLICY_TYPE_BUILTIN
	default:
		return pb.PolicyType_POLICY_TYPE_UNSPECIFIED
	}
}

func protoTypeToInternal(t pb.PolicyType) policy.Type {
	switch t {
	case pb.PolicyType_POLICY_TYPE_OPA:
		return policy.TypeOPA
	case pb.PolicyType_POLICY_TYPE_CEL:
		return policy.TypeCEL
	case pb.PolicyType_POLICY_TYPE_BUILTIN:
		return policy.TypeBuiltin
	default:
		return ""
	}
}

func policyCategoryToProto(c policy.Category) pb.PolicyCategory {
	switch c {
	case policy.CategorySecurity:
		return pb.PolicyCategory_POLICY_CATEGORY_SECURITY
	case policy.CategoryCompliance:
		return pb.PolicyCategory_POLICY_CATEGORY_COMPLIANCE
	case policy.CategoryOperational:
		return pb.PolicyCategory_POLICY_CATEGORY_OPERATIONAL
	case policy.CategoryCost:
		return pb.PolicyCategory_POLICY_CATEGORY_COST
	case policy.CategoryCustom:
		return pb.PolicyCategory_POLICY_CATEGORY_CUSTOM
	default:
		return pb.PolicyCategory_POLICY_CATEGORY_UNSPECIFIED
	}
}

func protoCategoryToInternal(c pb.PolicyCategory) policy.Category {
	switch c {
	case pb.PolicyCategory_POLICY_CATEGORY_SECURITY:
		return policy.CategorySecurity
	case pb.PolicyCategory_POLICY_CATEGORY_COMPLIANCE:
		return policy.CategoryCompliance
	case pb.PolicyCategory_POLICY_CATEGORY_OPERATIONAL:
		return policy.CategoryOperational
	case pb.PolicyCategory_POLICY_CATEGORY_COST:
		return policy.CategoryCost
	case pb.PolicyCategory_POLICY_CATEGORY_CUSTOM:
		return policy.CategoryCustom
	default:
		return ""
	}
}

func policySeverityToProto(s policy.Severity) pb.PolicySeverity {
	switch s {
	case policy.SeverityLow:
		return pb.PolicySeverity_POLICY_SEVERITY_LOW
	case policy.SeverityMedium:
		return pb.PolicySeverity_POLICY_SEVERITY_MEDIUM
	case policy.SeverityHigh:
		return pb.PolicySeverity_POLICY_SEVERITY_HIGH
	case policy.SeverityCritical:
		return pb.PolicySeverity_POLICY_SEVERITY_CRITICAL
	default:
		return pb.PolicySeverity_POLICY_SEVERITY_UNSPECIFIED
	}
}

func protoSeverityToInternal(s pb.PolicySeverity) policy.Severity {
	switch s {
	case pb.PolicySeverity_POLICY_SEVERITY_LOW:
		return policy.SeverityLow
	case pb.PolicySeverity_POLICY_SEVERITY_MEDIUM:
		return policy.SeverityMedium
	case pb.PolicySeverity_POLICY_SEVERITY_HIGH:
		return policy.SeverityHigh
	case pb.PolicySeverity_POLICY_SEVERITY_CRITICAL:
		return policy.SeverityCritical
	default:
		return ""
	}
}

func enforcementModeToProto(m policy.EnforcementMode) pb.EnforcementMode {
	switch m {
	case policy.ModeAudit:
		return pb.EnforcementMode_ENFORCEMENT_MODE_AUDIT
	case policy.ModeWarn:
		return pb.EnforcementMode_ENFORCEMENT_MODE_WARN
	case policy.ModeEnforce:
		return pb.EnforcementMode_ENFORCEMENT_MODE_ENFORCE
	default:
		return pb.EnforcementMode_ENFORCEMENT_MODE_UNSPECIFIED
	}
}

func protoEnforcementToInternal(m pb.EnforcementMode) policy.EnforcementMode {
	switch m {
	case pb.EnforcementMode_ENFORCEMENT_MODE_AUDIT:
		return policy.ModeAudit
	case pb.EnforcementMode_ENFORCEMENT_MODE_WARN:
		return policy.ModeWarn
	case pb.EnforcementMode_ENFORCEMENT_MODE_ENFORCE:
		return policy.ModeEnforce
	default:
		return ""
	}
}

func hasAllTags(policyTags, filterTags []string) bool {
	tagSet := make(map[string]struct{}, len(policyTags))
	for _, t := range policyTags {
		tagSet[t] = struct{}{}
	}
	for _, t := range filterTags {
		if _, ok := tagSet[t]; !ok {
			return false
		}
	}
	return true
}

func safeComplianceRate(compliant, total int) float32 {
	if total == 0 {
		return 0
	}
	return float32(compliant) / float32(total) * 100
}

// Ensure PolicyServer satisfies the interface at compile time.
var _ pb.PolicyServiceServer = (*PolicyServer)(nil)

// Ensure Registry satisfies PolicyRegistry at compile time.
var _ PolicyRegistry = (*policy.Registry)(nil)

// Ensure Engine satisfies PolicyEvaluator at compile time.
var _ PolicyEvaluator = (*policy.Engine)(nil)
