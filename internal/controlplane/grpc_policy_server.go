package controlplane

import (
	"context"
	"errors"
	"strconv"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
	v1 "go.keystone-core.io/keystone-core/pkg/api/v1"
)

// PolicyGRPCServer implements v1.PolicyServiceServer (Epic 12 task
// 12). v1.0 is audit-mode: EvaluatePolicy* run the engine + emit an
// audit entry but the Enforcer never blocks. The CRUD methods
// (CreatePolicy / UpdatePolicy / DeletePolicy) are intentionally
// left on the embedded UnimplementedPolicyServiceServer base, so
// they return codes.Unimplemented until v1.8 (the in-memory
// Registry has no Update/Deregister in v1.0 by design).
//
// Each backing component is independently nilable; the RPC group
// that needs a missing component returns codes.Unavailable (the
// established SecretsGRPCServer / grpc_events_server precedent).
type PolicyGRPCServer struct {
	v1.UnimplementedPolicyServiceServer

	Engine   *policy.Engine
	Reports  *policy.ReportGenerator
	AuditLog audit.AuditStore
	// Auditor receives one entry per policy evaluation — the §4.12
	// "every sensitive op emits" hook for policy eval (task 4
	// deferred this to tasks 5-9; the engine now exists).
	Auditor audit.Auditor
}

// NewPolicyGRPCServer wires the server. Any field may be nil; the
// dependent RPCs then return codes.Unavailable. Auditor nil → policy
// evaluations are not audited (NoopAuditor semantics).
func NewPolicyGRPCServer(engine *policy.Engine, reports *policy.ReportGenerator, auditLog audit.AuditStore, auditor audit.Auditor) *PolicyGRPCServer {
	if auditor == nil {
		auditor = audit.NoopAuditor{}
	}
	return &PolicyGRPCServer{
		Engine:   engine,
		Reports:  reports,
		AuditLog: auditLog,
		Auditor:  auditor,
	}
}

// translatePolicyError maps the policy/audit sentinels to gRPC
// codes. Mirrors SecretsGRPCServer.translateError.
func translatePolicyError(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, policy.ErrNotFound):
		return status.Error(codes.NotFound, err.Error())
	case errors.Is(err, policy.ErrPolicyDisabled), errors.Is(err, policy.ErrNoEvaluator):
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, policy.ErrInvalidPolicy), errors.Is(err, audit.ErrInvalidAuditEntry):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, policy.ErrEngineMisconfigured):
		return status.Error(codes.Internal, err.Error())
	default:
		return status.Error(codes.Internal, err.Error())
	}
}

// ---- Evaluate -------------------------------------------------------------

func (s *PolicyGRPCServer) EvaluatePolicy(ctx context.Context, req *v1.EvaluatePolicyRequest) (*v1.EvaluatePolicyResponse, error) {
	if s.Engine == nil {
		return nil, status.Error(codes.Unavailable, "policy engine not configured")
	}
	if req.GetPolicyId() == "" {
		return nil, status.Error(codes.InvalidArgument, "policy_id is required")
	}
	in := evalInputFromProto(req.GetInput())
	res, err := s.Engine.Evaluate(ctx, req.GetPolicyId(), in)
	if err != nil {
		return nil, translatePolicyError(err)
	}
	s.emitPolicyAudit(ctx, req.GetPolicyId(), in, res)
	return &v1.EvaluatePolicyResponse{Result: evalResultToProto(res)}, nil
}

func (s *PolicyGRPCServer) EvaluatePolicySet(ctx context.Context, req *v1.EvaluatePolicySetRequest) (*v1.EvaluatePolicySetResponse, error) {
	if s.Engine == nil {
		return nil, status.Error(codes.Unavailable, "policy engine not configured")
	}
	if req.GetPolicySetId() == "" {
		return nil, status.Error(codes.InvalidArgument, "policy_set_id is required")
	}
	in := evalInputFromProto(req.GetInput())
	results, err := s.Engine.EvaluatePolicySet(ctx, req.GetPolicySetId(), in)
	if err != nil {
		return nil, translatePolicyError(err)
	}
	out := &v1.EvaluatePolicySetResponse{AllowedAll: policy.AllowedAll(results)}
	for _, r := range results {
		out.Results = append(out.Results, evalResultToProto(r))
		s.emitPolicyAudit(ctx, r.PolicyID, in, r)
	}
	return out, nil
}

// emitPolicyAudit records one audit entry per policy evaluation —
// the §4.12 "every sensitive op MUST emit" rule for policy eval.
// Best-effort: a build/emit failure must not fail the RPC (the
// Auditor is fire-and-forget; matches the task-4 hooks).
func (s *PolicyGRPCServer) emitPolicyAudit(ctx context.Context, policyID string, in policy.EvaluationInput, res policy.EvaluationResult) {
	var ptype audit.PolicyType
	if p, err := s.Engine.Registry().GetPolicy(policyID); err == nil {
		ptype = p.Type
	}
	entry, err := policy.EvaluationAuditEntry(ptype, res, in.User)
	if err != nil {
		return
	}
	s.Auditor.Emit(ctx, entry)
}

// ---- Get / List -----------------------------------------------------------

func (s *PolicyGRPCServer) GetPolicy(_ context.Context, req *v1.GetPolicyRequest) (*v1.GetPolicyResponse, error) {
	if s.Engine == nil {
		return nil, status.Error(codes.Unavailable, "policy engine not configured")
	}
	p, err := s.Engine.Registry().GetPolicy(req.GetId())
	if err != nil {
		return nil, translatePolicyError(err)
	}
	return &v1.GetPolicyResponse{Policy: policyToProto(p)}, nil
}

func (s *PolicyGRPCServer) ListPolicies(_ context.Context, req *v1.ListPoliciesRequest) (*v1.ListPoliciesResponse, error) {
	if s.Engine == nil {
		return nil, status.Error(codes.Unavailable, "policy engine not configured")
	}
	all := s.Engine.Registry().ListPolicies()
	page, next, err := paginate(len(all), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := &v1.ListPoliciesResponse{NextPageToken: next, TotalCount: int32(len(all))}
	for _, p := range all[page.lo:page.hi] {
		out.Policies = append(out.Policies, policyToProto(p))
	}
	return out, nil
}

func (s *PolicyGRPCServer) GetPolicySet(_ context.Context, req *v1.GetPolicySetRequest) (*v1.GetPolicySetResponse, error) {
	if s.Engine == nil {
		return nil, status.Error(codes.Unavailable, "policy engine not configured")
	}
	ps, err := s.Engine.Registry().GetPolicySet(req.GetId())
	if err != nil {
		return nil, translatePolicyError(err)
	}
	return &v1.GetPolicySetResponse{PolicySet: policySetToProto(ps)}, nil
}

func (s *PolicyGRPCServer) ListPolicySets(_ context.Context, req *v1.ListPolicySetsRequest) (*v1.ListPolicySetsResponse, error) {
	if s.Engine == nil {
		return nil, status.Error(codes.Unavailable, "policy engine not configured")
	}
	all := s.Engine.Registry().ListPolicySets()
	page, next, err := paginate(len(all), req.GetPageSize(), req.GetPageToken())
	if err != nil {
		return nil, err
	}
	out := &v1.ListPolicySetsResponse{NextPageToken: next, TotalCount: int32(len(all))}
	for _, ps := range all[page.lo:page.hi] {
		out.PolicySets = append(out.PolicySets, policySetToProto(ps))
	}
	return out, nil
}

func (s *PolicyGRPCServer) ListViolations(ctx context.Context, req *v1.ListViolationsRequest) (*v1.ListViolationsResponse, error) {
	if s.AuditLog == nil {
		return nil, status.Error(codes.Unavailable, "audit store not configured")
	}
	denied := false
	q := audit.AuditQuery{
		PolicyID:     req.GetPolicyId(),
		ResourceType: req.GetResourceType(),
		User:         req.GetUser(),
		Allowed:      &denied,
		Since:        tsToTimeP(req.GetSince()),
		Until:        tsToTimeP(req.GetUntil()),
		Cursor:       req.GetCursor(),
		Limit:        int(req.GetLimit()),
	}
	pg, err := s.AuditLog.Query(ctx, q)
	if err != nil {
		return nil, translatePolicyError(err)
	}
	out := &v1.ListViolationsResponse{NextCursor: pg.NextCursor}
	for _, e := range pg.Entries {
		out.Entries = append(out.Entries, auditEntryToProto(e))
	}
	return out, nil
}

// ---- Compliance + audit ---------------------------------------------------

func (s *PolicyGRPCServer) GetComplianceReport(ctx context.Context, req *v1.GetComplianceReportRequest) (*v1.GetComplianceReportResponse, error) {
	if s.Reports == nil {
		return nil, status.Error(codes.Unavailable, "report generator not configured")
	}
	q := policy.ReportQuery{
		Since:          tsToTimeP(req.GetSince()),
		Until:          tsToTimeP(req.GetUntil()),
		BucketInterval: time.Duration(req.GetBucketIntervalNs()),
		TopN:           int(req.GetTopN()),
	}
	if fw := req.GetFramework(); fw != "" {
		parsed, err := policy.ParseFramework(fw)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		q.Framework = parsed
	}
	r, err := s.Reports.Generate(ctx, q)
	if err != nil {
		return nil, translatePolicyError(err)
	}
	return complianceReportToProto(r), nil
}

func (s *PolicyGRPCServer) GetAuditLog(ctx context.Context, req *v1.GetAuditLogRequest) (*v1.GetAuditLogResponse, error) {
	if s.AuditLog == nil {
		return nil, status.Error(codes.Unavailable, "audit store not configured")
	}
	q := audit.AuditQuery{
		PolicyID:     req.GetPolicyId(),
		User:         req.GetUser(),
		ResourceType: req.GetResourceType(),
		Action:       req.GetAction(),
		Since:        tsToTimeP(req.GetSince()),
		Until:        tsToTimeP(req.GetUntil()),
		Cursor:       req.GetCursor(),
		Limit:        int(req.GetLimit()),
	}
	if ms := req.GetMinSeverity(); ms != "" {
		sev, err := audit.ParseSeverity(ms)
		if err != nil {
			return nil, status.Error(codes.InvalidArgument, err.Error())
		}
		q.MinSeverity = sev
	}
	pg, err := s.AuditLog.Query(ctx, q)
	if err != nil {
		return nil, translatePolicyError(err)
	}
	out := &v1.GetAuditLogResponse{NextCursor: pg.NextCursor}
	for _, e := range pg.Entries {
		out.Entries = append(out.Entries, auditEntryToProto(e))
	}
	return out, nil
}

// ---- DTO mappers ----------------------------------------------------------

func evalInputFromProto(in *v1.EvaluationInput) policy.EvaluationInput {
	if in == nil {
		return policy.EvaluationInput{}
	}
	return policy.EvaluationInput{
		Resource:  in.GetResource().AsMap(),
		Action:    in.GetAction(),
		User:      in.GetUser(),
		Context:   in.GetContext().AsMap(),
		Timestamp: tsToTimeP(in.GetTimestamp()),
	}
}

func evalResultToProto(r policy.EvaluationResult) *v1.EvaluationResult {
	out := &v1.EvaluationResult{
		PolicyId:    r.PolicyID,
		PolicyName:  r.PolicyName,
		Allowed:     r.Allowed,
		Warnings:    r.Warnings,
		Message:     r.Message,
		DurationNs:  r.Duration.Nanoseconds(),
		EvaluatedAt: timeToProto(r.EvaluatedAt),
	}
	for _, v := range r.Violations {
		out.Violations = append(out.Violations, violationToProto(v))
	}
	return out
}

func violationToProto(v audit.Violation) *v1.Violation {
	return &v1.Violation{
		Rule:        v.Rule,
		Message:     v.Message,
		Severity:    v.Severity.String(),
		Path:        v.Path,
		Expected:    v.Expected,
		Actual:      v.Actual,
		Remediation: v.Remediation,
	}
}

func policyToProto(p *policy.Policy) *v1.Policy {
	if p == nil {
		return nil
	}
	return &v1.Policy{
		Id:              p.ID,
		Name:            p.Name,
		Type:            p.Type.String(),
		Category:        p.Category.String(),
		Severity:        p.Severity.String(),
		EnforcementMode: p.EnforcementMode.String(),
		Code:            p.Code,
		Enabled:         p.Enabled,
		Tags:            p.Tags,
		Metadata:        p.Metadata,
		CreatedAt:       timeToProto(p.CreatedAt),
		UpdatedAt:       timeToProto(p.UpdatedAt),
	}
}

func policySetToProto(ps *policy.PolicySet) *v1.PolicySet {
	if ps == nil {
		return nil
	}
	out := &v1.PolicySet{
		Id:        ps.ID,
		Name:      ps.Name,
		PolicyIds: ps.PolicyIDs,
		Enabled:   ps.Enabled,
		Tags:      ps.Tags,
		Metadata:  ps.Metadata,
		CreatedAt: timeToProto(ps.CreatedAt),
		UpdatedAt: timeToProto(ps.UpdatedAt),
	}
	if ps.EnforcementOverride != nil {
		out.EnforcementOverride = ps.EnforcementOverride.String()
	}
	return out
}

func auditEntryToProto(e audit.AuditEntry) *v1.AuditEntry {
	out := &v1.AuditEntry{
		Id:              e.ID,
		Timestamp:       timeToProto(e.Timestamp),
		PolicyId:        e.PolicyID,
		PolicyName:      e.PolicyName,
		PolicyType:      e.PolicyType.String(),
		ResourceType:    e.ResourceType,
		Allowed:         e.Allowed,
		DurationNs:      e.Duration.Nanoseconds(),
		EnforcementMode: e.EnforcementMode.String(),
		Severity:        e.Severity.String(),
		User:            e.User,
		Action:          e.Action,
		Metadata:        e.Metadata,
	}
	for _, v := range e.Violations {
		out.Violations = append(out.Violations, violationToProto(v))
	}
	return out
}

func complianceReportToProto(r policy.ComplianceReport) *v1.GetComplianceReportResponse {
	out := &v1.GetComplianceReportResponse{
		PeriodStart:             timeToProto(r.Period.Start),
		PeriodEnd:               timeToProto(r.Period.End),
		ComplianceRate:          r.ComplianceRate,
		TotalEvaluations:        int32(r.TotalEvaluations),
		CompliantEvaluations:    int32(r.CompliantEvaluations),
		NonCompliantEvaluations: int32(r.NonCompliantEvaluations),
		ViolationsBySeverity:    map[string]int32{},
	}
	for _, ps := range r.PolicyStats {
		out.PolicyStats = append(out.PolicyStats, &v1.PolicyStat{
			PolicyId:    ps.PolicyID,
			Evaluations: int32(ps.Evaluations),
			Passed:      int32(ps.Passed),
			Failed:      int32(ps.Failed),
			Rate:        ps.Rate,
		})
	}
	for _, tv := range r.TopViolations {
		out.TopViolations = append(out.TopViolations, &v1.ViolationCount{
			PolicyId: tv.PolicyID,
			Count:    int32(tv.Count),
		})
	}
	for sev, n := range r.ViolationsBySeverity {
		out.ViolationsBySeverity[sev.String()] = int32(n)
	}
	for _, tp := range r.Trend {
		out.Trend = append(out.Trend, &v1.TrendPoint{
			Start:                   timeToProto(tp.Start),
			End:                     timeToProto(tp.End),
			TotalEvaluations:        int32(tp.TotalEvaluations),
			CompliantEvaluations:    int32(tp.CompliantEvaluations),
			NonCompliantEvaluations: int32(tp.NonCompliantEvaluations),
			ComplianceRate:          tp.ComplianceRate,
		})
	}
	return out
}

// ---- helpers --------------------------------------------------------------

func timeToProto(t time.Time) *timestamppb.Timestamp {
	if t.IsZero() {
		return nil
	}
	return timestamppb.New(t.UTC())
}

func tsToTimeP(ts *timestamppb.Timestamp) time.Time {
	if ts == nil {
		return time.Time{}
	}
	return ts.AsTime()
}

type pageBounds struct{ lo, hi int }

// paginate is offset pagination over an in-memory sorted slice.
// pageToken is the integer start offset; pageSize defaults to 50,
// caps at 500. Returns the [lo,hi) bounds + the next token ("" when
// exhausted).
func paginate(total int, pageSize int32, pageToken string) (pageBounds, string, error) {
	const defSize, maxSize = 50, 500
	size := int(pageSize)
	if size <= 0 {
		size = defSize
	}
	if size > maxSize {
		size = maxSize
	}
	lo := 0
	if pageToken != "" {
		n, err := strconv.Atoi(pageToken)
		if err != nil || n < 0 {
			return pageBounds{}, "", status.Errorf(codes.InvalidArgument, "page_token %q is not a valid offset", pageToken)
		}
		lo = n
	}
	if lo > total {
		lo = total
	}
	hi := lo + size
	if hi > total {
		hi = total
	}
	next := ""
	if hi < total {
		next = strconv.Itoa(hi)
	}
	return pageBounds{lo: lo, hi: hi}, next, nil
}

