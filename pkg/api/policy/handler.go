// Package policy exposes REST routes for the policy domain
// (Epic 12 task 13).
//
// Routes (all sync; streaming/CRUD stays gRPC-only / v1.8):
//
//	GET  /api/v1/policies                 list (?page_size=&page_token=)
//	GET  /api/v1/policies/{id}            get one
//	POST /api/v1/policies/evaluate        evaluate (body: policy_id XOR policy_set_id + input)
//	GET  /api/v1/policies/violations      denied audit entries (cursor)
//	GET  /api/v1/policies/compliance      ComplianceReport (?since=&until=&framework=&bucket_interval=&top_n=)
//	GET  /api/v1/policies/audit           audit log (cursor + filters)
//	GET  /api/v1/policy-sets              list (?page_size=&page_token=)
//	GET  /api/v1/policy-sets/{id}         get one
//
// JSON DTOs serialise enums as the canonical lowercase name (the
// events/secrets REST convention); the gRPC PolicyService carries
// the same string values. v1.0 is audit-mode (the Enforcer never
// blocks); CRUD is v1.8 (gRPC returns Unimplemented; not routed
// here). When a backing component is nil the affected routes return
// HTTP 503.
//
// Authentication + RBAC are enforced upstream by the auth
// interceptor / middleware (Epic 03); this handler trusts requests
// to have passed those checks.
package policy

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
	"go.keystone-core.io/keystone-core/internal/policy"
)

// Handler exposes the policy-domain REST routes. Any component may
// be nil — the routes that need it return 503.
type Handler struct {
	engine   *policy.Engine
	reports  *policy.ReportGenerator
	auditLog audit.AuditStore
	auditor  audit.Auditor
}

// NewHandler returns a Handler. Pass nil for components that aren't
// configured (their routes then return 503). A nil auditor means
// policy evaluations are not audited (NoopAuditor semantics).
func NewHandler(engine *policy.Engine, reports *policy.ReportGenerator, auditLog audit.AuditStore, auditor audit.Auditor) *Handler {
	if auditor == nil {
		auditor = audit.NoopAuditor{}
	}
	return &Handler{engine: engine, reports: reports, auditLog: auditLog, auditor: auditor}
}

// Register installs the routes onto mux. More-specific literal
// patterns are registered before the {id} wildcards (Go 1.22
// ServeMux resolves by specificity, but the ordering documents
// intent — same convention as the events handler).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/v1/policies/evaluate", h.handleEvaluate)
	mux.HandleFunc("GET /api/v1/policies/violations", h.handleViolations)
	mux.HandleFunc("GET /api/v1/policies/compliance", h.handleCompliance)
	mux.HandleFunc("GET /api/v1/policies/audit", h.handleAuditLog)
	mux.HandleFunc("GET /api/v1/policies/{id}", h.handleGetPolicy)
	mux.HandleFunc("GET /api/v1/policies", h.handleListPolicies)
	mux.HandleFunc("GET /api/v1/policy-sets/{id}", h.handleGetPolicySet)
	mux.HandleFunc("GET /api/v1/policy-sets", h.handleListPolicySets)
}

// ---- DTOs -----------------------------------------------------------------

type policyDTO struct {
	ID              string            `json:"id"`
	Name            string            `json:"name"`
	Type            string            `json:"type"`
	Category        string            `json:"category"`
	Severity        string            `json:"severity"`
	EnforcementMode string            `json:"enforcement_mode"`
	Code            string            `json:"code"`
	Enabled         bool              `json:"enabled"`
	Tags            []string          `json:"tags,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
	CreatedAt       time.Time         `json:"created_at,omitempty"`
	UpdatedAt       time.Time         `json:"updated_at,omitempty"`
}

type policySetDTO struct {
	ID                  string            `json:"id"`
	Name                string            `json:"name"`
	PolicyIDs           []string          `json:"policy_ids"`
	EnforcementOverride string            `json:"enforcement_override,omitempty"`
	Enabled             bool              `json:"enabled"`
	Tags                []string          `json:"tags,omitempty"`
	Metadata            map[string]string `json:"metadata,omitempty"`
	CreatedAt           time.Time         `json:"created_at,omitempty"`
	UpdatedAt           time.Time         `json:"updated_at,omitempty"`
}

type violationDTO struct {
	Rule        string `json:"rule"`
	Message     string `json:"message"`
	Severity    string `json:"severity"`
	Path        string `json:"path,omitempty"`
	Expected    string `json:"expected,omitempty"`
	Actual      string `json:"actual,omitempty"`
	Remediation string `json:"remediation,omitempty"`
}

type evaluationResultDTO struct {
	PolicyID    string         `json:"policy_id"`
	PolicyName  string         `json:"policy_name"`
	Allowed     bool           `json:"allowed"`
	Violations  []violationDTO `json:"violations,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
	Message     string         `json:"message,omitempty"`
	DurationNS  int64          `json:"duration_ns"`
	EvaluatedAt time.Time      `json:"evaluated_at"`
}

type auditEntryDTO struct {
	ID              string            `json:"id"`
	Timestamp       time.Time         `json:"timestamp"`
	PolicyID        string            `json:"policy_id,omitempty"`
	PolicyName      string            `json:"policy_name,omitempty"`
	PolicyType      string            `json:"policy_type,omitempty"`
	ResourceType    string            `json:"resource_type,omitempty"`
	Allowed         bool              `json:"allowed"`
	DurationNS      int64             `json:"duration_ns"`
	Violations      []violationDTO    `json:"violations,omitempty"`
	EnforcementMode string            `json:"enforcement_mode"`
	Severity        string            `json:"severity"`
	User            string            `json:"user,omitempty"`
	Action          string            `json:"action"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

type evaluateRequest struct {
	PolicyID    string         `json:"policy_id,omitempty"`
	PolicySetID string         `json:"policy_set_id,omitempty"`
	Input       evaluateInput  `json:"input"`
}

type evaluateInput struct {
	Resource  map[string]any `json:"resource,omitempty"`
	Action    string         `json:"action,omitempty"`
	User      string         `json:"user,omitempty"`
	Context   map[string]any `json:"context,omitempty"`
	Timestamp time.Time      `json:"timestamp,omitempty"`
}

type evaluateResponse struct {
	Result     *evaluationResultDTO  `json:"result,omitempty"`
	Results    []evaluationResultDTO `json:"results,omitempty"`
	AllowedAll *bool                 `json:"allowed_all,omitempty"`
}

type policyStatDTO struct {
	PolicyID    string  `json:"policy_id"`
	Evaluations int     `json:"evaluations"`
	Passed      int     `json:"passed"`
	Failed      int     `json:"failed"`
	Rate        float64 `json:"rate"`
}

type violationCountDTO struct {
	PolicyID string `json:"policy_id"`
	Count    int    `json:"count"`
}

type trendPointDTO struct {
	Start                   time.Time `json:"start"`
	End                     time.Time `json:"end"`
	TotalEvaluations        int       `json:"total_evaluations"`
	CompliantEvaluations    int       `json:"compliant_evaluations"`
	NonCompliantEvaluations int       `json:"non_compliant_evaluations"`
	ComplianceRate          float64   `json:"compliance_rate"`
}

type complianceReportDTO struct {
	PeriodStart             time.Time           `json:"period_start"`
	PeriodEnd               time.Time           `json:"period_end"`
	ComplianceRate          float64             `json:"compliance_rate"`
	TotalEvaluations        int                 `json:"total_evaluations"`
	CompliantEvaluations    int                 `json:"compliant_evaluations"`
	NonCompliantEvaluations int                 `json:"non_compliant_evaluations"`
	PolicyStats             []policyStatDTO     `json:"policy_stats,omitempty"`
	TopViolations           []violationCountDTO `json:"top_violations,omitempty"`
	ViolationsBySeverity    map[string]int      `json:"violations_by_severity,omitempty"`
	Trend                   []trendPointDTO     `json:"trend,omitempty"`
}

type listPoliciesResponse struct {
	Policies      []policyDTO `json:"policies"`
	NextPageToken string      `json:"next_page_token,omitempty"`
	TotalCount    int         `json:"total_count"`
}

type listPolicySetsResponse struct {
	PolicySets    []policySetDTO `json:"policy_sets"`
	NextPageToken string         `json:"next_page_token,omitempty"`
	TotalCount    int            `json:"total_count"`
}

type auditListResponse struct {
	Entries    []auditEntryDTO `json:"entries"`
	NextCursor string          `json:"next_cursor,omitempty"`
}

type errorDTO struct {
	Error string `json:"error"`
}

// ---- handlers -------------------------------------------------------------

func (h *Handler) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "policy engine not configured")
		return
	}
	var req evaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	hasPolicy := req.PolicyID != ""
	hasSet := req.PolicySetID != ""
	if hasPolicy == hasSet { // both or neither
		writeError(w, http.StatusBadRequest, "exactly one of policy_id or policy_set_id is required")
		return
	}
	in := policy.EvaluationInput{
		Resource:  req.Input.Resource,
		Action:    req.Input.Action,
		User:      req.Input.User,
		Context:   req.Input.Context,
		Timestamp: req.Input.Timestamp,
	}
	if hasPolicy {
		res, err := h.engine.Evaluate(r.Context(), req.PolicyID, in)
		if err != nil {
			writePolicyError(w, err)
			return
		}
		h.emitAudit(r, req.PolicyID, in.User, res)
		dto := evalResultToDTO(res)
		writeJSON(w, http.StatusOK, evaluateResponse{Result: &dto})
		return
	}
	results, err := h.engine.EvaluatePolicySet(r.Context(), req.PolicySetID, in)
	if err != nil {
		writePolicyError(w, err)
		return
	}
	out := evaluateResponse{Results: make([]evaluationResultDTO, 0, len(results))}
	for _, res := range results {
		h.emitAudit(r, res.PolicyID, in.User, res)
		out.Results = append(out.Results, evalResultToDTO(res))
	}
	allowed := policy.AllowedAll(results)
	out.AllowedAll = &allowed
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleGetPolicy(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "policy engine not configured")
		return
	}
	p, err := h.engine.Registry().GetPolicy(r.PathValue("id"))
	if err != nil {
		writePolicyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policyToDTO(p))
}

func (h *Handler) handleListPolicies(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "policy engine not configured")
		return
	}
	all := h.engine.Registry().ListPolicies()
	lo, hi, next, err := pageBounds(len(all), r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := listPoliciesResponse{
		Policies:      make([]policyDTO, 0, hi-lo),
		NextPageToken: next,
		TotalCount:    len(all),
	}
	for _, p := range all[lo:hi] {
		out.Policies = append(out.Policies, policyToDTO(p))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleGetPolicySet(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "policy engine not configured")
		return
	}
	ps, err := h.engine.Registry().GetPolicySet(r.PathValue("id"))
	if err != nil {
		writePolicyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, policySetToDTO(ps))
}

func (h *Handler) handleListPolicySets(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		writeError(w, http.StatusServiceUnavailable, "policy engine not configured")
		return
	}
	all := h.engine.Registry().ListPolicySets()
	lo, hi, next, err := pageBounds(len(all), r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	out := listPolicySetsResponse{
		PolicySets:    make([]policySetDTO, 0, hi-lo),
		NextPageToken: next,
		TotalCount:    len(all),
	}
	for _, ps := range all[lo:hi] {
		out.PolicySets = append(out.PolicySets, policySetToDTO(ps))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleViolations(w http.ResponseWriter, r *http.Request) {
	if h.auditLog == nil {
		writeError(w, http.StatusServiceUnavailable, "audit store not configured")
		return
	}
	q, err := auditQueryFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	denied := false
	q.Allowed = &denied
	h.writeAuditPage(w, r, q)
}

func (h *Handler) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if h.auditLog == nil {
		writeError(w, http.StatusServiceUnavailable, "audit store not configured")
		return
	}
	q, err := auditQueryFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	h.writeAuditPage(w, r, q)
}

func (h *Handler) writeAuditPage(w http.ResponseWriter, r *http.Request, q audit.AuditQuery) {
	page, err := h.auditLog.Query(r.Context(), q)
	if err != nil {
		writePolicyError(w, err)
		return
	}
	out := auditListResponse{
		Entries:    make([]auditEntryDTO, 0, len(page.Entries)),
		NextCursor: page.NextCursor,
	}
	for _, e := range page.Entries {
		out.Entries = append(out.Entries, auditEntryToDTO(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleCompliance(w http.ResponseWriter, r *http.Request) {
	if h.reports == nil {
		writeError(w, http.StatusServiceUnavailable, "report generator not configured")
		return
	}
	q, err := reportQueryFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rep, err := h.reports.Generate(r.Context(), q)
	if err != nil {
		writePolicyError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, reportToDTO(rep))
}

// emitAudit records the policy-evaluation audit entry via the
// shared builder so the REST + gRPC paths emit an identical shape.
// Fire-and-forget — an emit failure never affects the response.
func (h *Handler) emitAudit(r *http.Request, policyID, user string, res policy.EvaluationResult) {
	var ptype audit.PolicyType
	if p, err := h.engine.Registry().GetPolicy(policyID); err == nil {
		ptype = p.Type
	}
	entry, err := policy.EvaluationAuditEntry(ptype, res, user)
	if err != nil {
		return
	}
	h.auditor.Emit(r.Context(), entry)
}

// ---- request parsing ------------------------------------------------------

func auditQueryFromRequest(r *http.Request) (audit.AuditQuery, error) {
	qv := r.URL.Query()
	q := audit.AuditQuery{
		PolicyID:     qv.Get("policy_id"),
		User:         qv.Get("user"),
		ResourceType: qv.Get("resource_type"),
		Action:       qv.Get("action"),
		Cursor:       qv.Get("cursor"),
	}
	if v := qv.Get("min_severity"); v != "" {
		sev, err := audit.ParseSeverity(v)
		if err != nil {
			return audit.AuditQuery{}, err
		}
		q.MinSeverity = sev
	}
	if v := qv.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return audit.AuditQuery{}, errors.New("since must be RFC3339")
		}
		q.Since = t
	}
	if v := qv.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return audit.AuditQuery{}, errors.New("until must be RFC3339")
		}
		q.Until = t
	}
	if v := qv.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return audit.AuditQuery{}, errors.New("limit must be a non-negative integer")
		}
		q.Limit = n
	}
	if qv.Get("desc") == "true" {
		q.Descending = true
	}
	return q, nil
}

func reportQueryFromRequest(r *http.Request) (policy.ReportQuery, error) {
	qv := r.URL.Query()
	q := policy.ReportQuery{}
	if v := qv.Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return policy.ReportQuery{}, errors.New("since must be RFC3339")
		}
		q.Since = t
	}
	if v := qv.Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return policy.ReportQuery{}, errors.New("until must be RFC3339")
		}
		q.Until = t
	}
	if v := qv.Get("framework"); v != "" {
		fw, err := policy.ParseFramework(v)
		if err != nil {
			return policy.ReportQuery{}, err
		}
		q.Framework = fw
	}
	if v := qv.Get("bucket_interval"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return policy.ReportQuery{}, errors.New("bucket_interval must be a Go duration (e.g. 24h)")
		}
		q.BucketInterval = d
	}
	if v := qv.Get("top_n"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return policy.ReportQuery{}, errors.New("top_n must be an integer")
		}
		q.TopN = n
	}
	return q, nil
}

// pageBounds resolves offset pagination over an in-memory sorted
// slice. page_token = integer start offset; page_size default 50,
// cap 500.
func pageBounds(total int, r *http.Request) (lo, hi int, next string, err error) {
	const defSize, maxSize = 50, 500
	qv := r.URL.Query()
	size := defSize
	if v := qv.Get("page_size"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n <= 0 {
			return 0, 0, "", errors.New("page_size must be a positive integer")
		}
		size = n
		if size > maxSize {
			size = maxSize
		}
	}
	if v := qv.Get("page_token"); v != "" {
		n, e := strconv.Atoi(v)
		if e != nil || n < 0 {
			return 0, 0, "", errors.New("page_token must be a non-negative offset")
		}
		lo = n
	}
	if lo > total {
		lo = total
	}
	hi = lo + size
	if hi > total {
		hi = total
	}
	if hi < total {
		next = strconv.Itoa(hi)
	}
	return lo, hi, next, nil
}

// ---- DTO mappers ----------------------------------------------------------

func policyToDTO(p *policy.Policy) policyDTO {
	return policyDTO{
		ID:              p.ID,
		Name:            p.Name,
		Type:            p.Type.String(),
		Category:        p.Category.String(),
		Severity:        p.Severity.String(),
		EnforcementMode: p.EnforcementMode.String(),
		Code:            p.Code,
		Enabled:         p.Enabled,
		Tags:            p.Tags,
		Metadata:        p.Metadata,
		CreatedAt:       p.CreatedAt,
		UpdatedAt:       p.UpdatedAt,
	}
}

func policySetToDTO(ps *policy.PolicySet) policySetDTO {
	out := policySetDTO{
		ID:        ps.ID,
		Name:      ps.Name,
		PolicyIDs: ps.PolicyIDs,
		Enabled:   ps.Enabled,
		Tags:      ps.Tags,
		Metadata:  ps.Metadata,
		CreatedAt: ps.CreatedAt,
		UpdatedAt: ps.UpdatedAt,
	}
	if ps.EnforcementOverride != nil {
		out.EnforcementOverride = ps.EnforcementOverride.String()
	}
	return out
}

func violationToDTO(v audit.Violation) violationDTO {
	return violationDTO{
		Rule:        v.Rule,
		Message:     v.Message,
		Severity:    v.Severity.String(),
		Path:        v.Path,
		Expected:    v.Expected,
		Actual:      v.Actual,
		Remediation: v.Remediation,
	}
}

func evalResultToDTO(res policy.EvaluationResult) evaluationResultDTO {
	out := evaluationResultDTO{
		PolicyID:    res.PolicyID,
		PolicyName:  res.PolicyName,
		Allowed:     res.Allowed,
		Warnings:    res.Warnings,
		Message:     res.Message,
		DurationNS:  res.Duration.Nanoseconds(),
		EvaluatedAt: res.EvaluatedAt,
	}
	for _, v := range res.Violations {
		out.Violations = append(out.Violations, violationToDTO(v))
	}
	return out
}

func auditEntryToDTO(e audit.AuditEntry) auditEntryDTO {
	out := auditEntryDTO{
		ID:              e.ID,
		Timestamp:       e.Timestamp,
		PolicyID:        e.PolicyID,
		PolicyName:      e.PolicyName,
		PolicyType:      e.PolicyType.String(),
		ResourceType:    e.ResourceType,
		Allowed:         e.Allowed,
		DurationNS:      e.Duration.Nanoseconds(),
		EnforcementMode: e.EnforcementMode.String(),
		Severity:        e.Severity.String(),
		User:            e.User,
		Action:          e.Action,
		Metadata:        e.Metadata,
	}
	for _, v := range e.Violations {
		out.Violations = append(out.Violations, violationToDTO(v))
	}
	return out
}

func reportToDTO(rep policy.ComplianceReport) complianceReportDTO {
	out := complianceReportDTO{
		PeriodStart:             rep.Period.Start,
		PeriodEnd:               rep.Period.End,
		ComplianceRate:          rep.ComplianceRate,
		TotalEvaluations:        rep.TotalEvaluations,
		CompliantEvaluations:    rep.CompliantEvaluations,
		NonCompliantEvaluations: rep.NonCompliantEvaluations,
		ViolationsBySeverity:    map[string]int{},
	}
	for _, ps := range rep.PolicyStats {
		out.PolicyStats = append(out.PolicyStats, policyStatDTO{
			PolicyID:    ps.PolicyID,
			Evaluations: ps.Evaluations,
			Passed:      ps.Passed,
			Failed:      ps.Failed,
			Rate:        ps.Rate,
		})
	}
	for _, tv := range rep.TopViolations {
		out.TopViolations = append(out.TopViolations, violationCountDTO{
			PolicyID: tv.PolicyID,
			Count:    tv.Count,
		})
	}
	for sev, n := range rep.ViolationsBySeverity {
		out.ViolationsBySeverity[sev.String()] = n
	}
	for _, tp := range rep.Trend {
		out.Trend = append(out.Trend, trendPointDTO{
			Start:                   tp.Start,
			End:                     tp.End,
			TotalEvaluations:        tp.TotalEvaluations,
			CompliantEvaluations:    tp.CompliantEvaluations,
			NonCompliantEvaluations: tp.NonCompliantEvaluations,
			ComplianceRate:          tp.ComplianceRate,
		})
	}
	return out
}

// ---- response helpers -----------------------------------------------------

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorDTO{Error: msg})
}

// writePolicyError maps the policy/audit sentinels to HTTP status,
// mirroring internal/controlplane.translatePolicyError.
func writePolicyError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, policy.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, policy.ErrPolicyDisabled), errors.Is(err, policy.ErrNoEvaluator):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, policy.ErrInvalidPolicy), errors.Is(err, audit.ErrInvalidAuditEntry):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
