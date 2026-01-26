// Package policy provides HTTP handlers for policy REST API endpoints.
package policy

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/shawnbutts/keystone-core/internal/policy"
)

// Handler provides HTTP handlers for policy API endpoints.
type Handler struct {
	engine   *policy.PolicyEngine
	auditor  *policy.PolicyAuditor
	reporter *policy.ComplianceReporter
}

// NewHandler creates a new policy API handler.
func NewHandler(engine *policy.PolicyEngine, auditor *policy.PolicyAuditor, reporter *policy.ComplianceReporter) *Handler {
	return &Handler{
		engine:   engine,
		auditor:  auditor,
		reporter: reporter,
	}
}

// RegisterRoutes registers the policy API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/policies/evaluate", h.handleEvaluate)
	mux.HandleFunc("/api/v1/policies/violations", h.handleViolations)
	mux.HandleFunc("/api/v1/policies/compliance", h.handleCompliance)
}

// EvaluateRequest represents the request body for POST /api/v1/policies/evaluate
type EvaluateRequest struct {
	// PolicyID to evaluate (single policy)
	PolicyID string `json:"policy_id,omitempty"`

	// PolicySetID to evaluate (policy set)
	PolicySetID string `json:"policy_set_id,omitempty"`

	// ResourceType to evaluate for (evaluates all bound policies)
	ResourceType string `json:"resource_type,omitempty"`

	// Resource being evaluated
	Resource interface{} `json:"resource"`

	// Action being performed
	Action string `json:"action"`

	// User performing the action
	User string `json:"user,omitempty"`

	// Context for evaluation
	Context map[string]interface{} `json:"context,omitempty"`
}

// EvaluateResponse represents the response for POST /api/v1/policies/evaluate
type EvaluateResponse struct {
	Allowed         bool                        `json:"allowed"`
	Results         []EvaluationResultResponse  `json:"results,omitempty"`
	Summary         *policy.PolicySummary       `json:"summary,omitempty"`
	TotalDuration   string                      `json:"total_duration"`
	EvaluatedAt     time.Time                   `json:"evaluated_at"`
}

// EvaluationResultResponse represents a single policy evaluation result
type EvaluationResultResponse struct {
	PolicyID    string             `json:"policy_id"`
	PolicyName  string             `json:"policy_name"`
	Allowed     bool               `json:"allowed"`
	Violations  []policy.Violation `json:"violations,omitempty"`
	Warnings    []string           `json:"warnings,omitempty"`
	Message     string             `json:"message,omitempty"`
	Duration    string             `json:"duration"`
	EvaluatedAt time.Time          `json:"evaluated_at"`
}

// ViolationsResponse represents the response for GET /api/v1/policies/violations
type ViolationsResponse struct {
	Violations  []ViolationEntry      `json:"violations"`
	Summary     *policy.AuditSummary  `json:"summary"`
	Total       int                   `json:"total"`
	Limit       int                   `json:"limit"`
	RetrievedAt time.Time             `json:"retrieved_at"`
}

// ViolationEntry represents a violation from audit
type ViolationEntry struct {
	ID              string             `json:"id"`
	Timestamp       time.Time          `json:"timestamp"`
	PolicyID        string             `json:"policy_id"`
	PolicyName      string             `json:"policy_name"`
	ResourceType    string             `json:"resource_type,omitempty"`
	User            string             `json:"user,omitempty"`
	Action          string             `json:"action,omitempty"`
	Violations      []policy.Violation `json:"violations"`
	EnforcementMode string             `json:"enforcement_mode"`
}

// ComplianceResponse represents the response for GET /api/v1/policies/compliance
type ComplianceResponse struct {
	Report      *ComplianceReportResponse `json:"report"`
	Period      PeriodResponse            `json:"period"`
	GeneratedAt time.Time                 `json:"generated_at"`
}

// ComplianceReportResponse is a JSON-friendly compliance report
type ComplianceReportResponse struct {
	TotalPolicies     int                         `json:"total_policies"`
	CompliantPolicies int                         `json:"compliant_policies"`
	ComplianceRate    float64                     `json:"compliance_rate"`
	TopViolations     []ViolationSummaryResponse  `json:"top_violations,omitempty"`
	SeverityBreakdown map[string]int              `json:"severity_breakdown"`
}

// ViolationSummaryResponse is a JSON-friendly violation summary
type ViolationSummaryResponse struct {
	PolicyID   string `json:"policy_id"`
	PolicyName string `json:"policy_name,omitempty"`
	Count      int    `json:"count"`
	Severity   string `json:"severity,omitempty"`
}

// PeriodResponse represents a time period
type PeriodResponse struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// handleEvaluate handles POST /api/v1/policies/evaluate
func (h *Handler) handleEvaluate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req EvaluateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate request - need at least one of policy_id, policy_set_id, or resource_type
	if req.PolicyID == "" && req.PolicySetID == "" && req.ResourceType == "" {
		writeError(w, http.StatusBadRequest, "One of policy_id, policy_set_id, or resource_type is required")
		return
	}
	if req.Action == "" {
		writeError(w, http.StatusBadRequest, "action is required")
		return
	}

	// Build evaluation input
	input := &policy.EvaluationInput{
		Resource:  req.Resource,
		Action:    req.Action,
		User:      req.User,
		Context:   req.Context,
		Timestamp: time.Now(),
	}

	ctx := r.Context()
	var resp EvaluateResponse

	// Evaluate based on request type
	if req.PolicyID != "" {
		// Single policy evaluation
		result, err := h.engine.Evaluate(ctx, req.PolicyID, input)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to evaluate policy: "+err.Error())
			return
		}

		resp = EvaluateResponse{
			Allowed:       result.Allowed,
			Results:       []EvaluationResultResponse{convertEvaluationResult(result)},
			TotalDuration: result.Duration.String(),
			EvaluatedAt:   result.EvaluatedAt,
		}
	} else if req.PolicySetID != "" {
		// Policy set evaluation
		result, err := h.engine.EvaluatePolicySet(ctx, req.PolicySetID, input)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to evaluate policy set: "+err.Error())
			return
		}

		resp = buildEvaluateResponse(result)
	} else {
		// Resource-based evaluation
		result, err := h.engine.EvaluateForResource(ctx, req.ResourceType, input)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "Failed to evaluate policies for resource: "+err.Error())
			return
		}

		resp = buildEvaluateResponse(result)
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleViolations handles GET /api/v1/policies/violations
func (h *Handler) handleViolations(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	query := r.URL.Query()

	// Build filter
	filter := &policy.AuditFilter{}

	if policyID := query.Get("policy_id"); policyID != "" {
		filter.PolicyID = policyID
	}
	if resourceType := query.Get("resource_type"); resourceType != "" {
		filter.ResourceType = resourceType
	}
	if user := query.Get("user"); user != "" {
		filter.User = user
	}
	if action := query.Get("action"); action != "" {
		filter.Action = action
	}

	// Parse time range
	if start := query.Get("start"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			filter.StartTime = t
		}
	}
	if end := query.Get("end"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			filter.EndTime = t
		}
	}

	// Parse limit
	limit := 100 // default
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}
	filter.Limit = limit

	// Only get denied evaluations (violations)
	denied := false
	filter.Allowed = &denied

	// Get audit entries
	entries := h.auditor.GetEntries(filter)
	summary := h.auditor.GetSummary(filter)

	// Convert to response format
	violations := make([]ViolationEntry, 0, len(entries))
	for _, entry := range entries {
		if len(entry.Violations) > 0 { // Only include entries with actual violations
			violations = append(violations, ViolationEntry{
				ID:              entry.ID,
				Timestamp:       entry.Timestamp,
				PolicyID:        entry.PolicyID,
				PolicyName:      entry.PolicyName,
				ResourceType:    entry.ResourceType,
				User:            entry.User,
				Action:          entry.Action,
				Violations:      entry.Violations,
				EnforcementMode: string(entry.EnforcementMode),
			})
		}
	}

	resp := ViolationsResponse{
		Violations:  violations,
		Summary:     summary,
		Total:       len(violations),
		Limit:       limit,
		RetrievedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleCompliance handles GET /api/v1/policies/compliance
func (h *Handler) handleCompliance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters for time period
	query := r.URL.Query()

	// Default to last 24 hours
	end := time.Now()
	start := end.Add(-24 * time.Hour)

	if startStr := query.Get("start"); startStr != "" {
		if t, err := time.Parse(time.RFC3339, startStr); err == nil {
			start = t
		}
	}
	if endStr := query.Get("end"); endStr != "" {
		if t, err := time.Parse(time.RFC3339, endStr); err == nil {
			end = t
		}
	}

	period := policy.ReportPeriod{
		Start: start,
		End:   end,
	}

	// Generate compliance report
	report := h.reporter.GenerateReport(period)

	// Convert to JSON-friendly format
	severityBreakdown := make(map[string]int)
	for sev, count := range report.ViolationsBySeverity {
		severityBreakdown[string(sev)] = count
	}

	topViolations := make([]ViolationSummaryResponse, 0, len(report.TopViolations))
	for _, v := range report.TopViolations {
		topViolations = append(topViolations, ViolationSummaryResponse{
			PolicyID:   v.PolicyID,
			PolicyName: v.PolicyName,
			Count:      v.Count,
			Severity:   string(v.Severity),
		})
	}

	resp := ComplianceResponse{
		Report: &ComplianceReportResponse{
			TotalPolicies:     report.TotalPolicies,
			CompliantPolicies: report.CompliantPolicies,
			ComplianceRate:    report.ComplianceRate,
			TopViolations:     topViolations,
			SeverityBreakdown: severityBreakdown,
		},
		Period: PeriodResponse{
			Start: period.Start,
			End:   period.End,
		},
		GeneratedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// Helper functions

func buildEvaluateResponse(result *policy.PolicyResult) EvaluateResponse {
	results := make([]EvaluationResultResponse, 0, len(result.Results))
	for _, r := range result.Results {
		results = append(results, convertEvaluationResult(r))
	}

	return EvaluateResponse{
		Allowed:       result.Allowed,
		Results:       results,
		Summary:       result.Summary,
		TotalDuration: result.TotalDuration.String(),
		EvaluatedAt:   result.EvaluatedAt,
	}
}

func convertEvaluationResult(r *policy.EvaluationResult) EvaluationResultResponse {
	return EvaluationResultResponse{
		PolicyID:    r.PolicyID,
		PolicyName:  r.PolicyName,
		Allowed:     r.Allowed,
		Violations:  r.Violations,
		Warnings:    r.Warnings,
		Message:     r.Message,
		Duration:    r.Duration.String(),
		EvaluatedAt: r.EvaluatedAt,
	}
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
