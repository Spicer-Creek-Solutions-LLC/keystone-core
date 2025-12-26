package policy

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// AuditEntry represents a single policy evaluation audit record
type AuditEntry struct {
	ID              string                 `json:"id"`
	Timestamp       time.Time              `json:"timestamp"`
	PolicyID        string                 `json:"policy_id"`
	PolicyName      string                 `json:"policy_name"`
	PolicyType      PolicyType             `json:"policy_type"`
	ResourceType    string                 `json:"resource_type,omitempty"`
	Allowed         bool                   `json:"allowed"`
	Duration        time.Duration          `json:"duration"`
	Violations      []Violation            `json:"violations"`
	EnforcementMode EnforcementMode        `json:"enforcement_mode"`
	User            string                 `json:"user,omitempty"`
	Action          string                 `json:"action,omitempty"`
	Metadata        map[string]interface{} `json:"metadata,omitempty"`
}

// PolicyAuditor tracks and records policy evaluations
type PolicyAuditor struct {
	entries []AuditEntry
	mu      sync.RWMutex
	maxSize int
}

// NewPolicyAuditor creates a new policy auditor
func NewPolicyAuditor(maxSize int) *PolicyAuditor {
	if maxSize <= 0 {
		maxSize = 10000 // Default to 10k entries
	}
	return &PolicyAuditor{
		entries: make([]AuditEntry, 0, maxSize),
		maxSize: maxSize,
	}
}

// RecordEvaluation records a policy evaluation
func (a *PolicyAuditor) RecordEvaluation(ctx context.Context, result *EvaluationResult, resourceType, user, action string, enforcementMode EnforcementMode) {
	a.mu.Lock()
	defer a.mu.Unlock()

	entry := AuditEntry{
		ID:              generateAuditID(),
		Timestamp:       result.EvaluatedAt,
		PolicyID:        result.PolicyID,
		PolicyName:      result.PolicyName,
		ResourceType:    resourceType,
		Allowed:         result.Allowed,
		Duration:        result.Duration,
		Violations:      result.Violations,
		EnforcementMode: enforcementMode,
		User:            user,
		Action:          action,
		Metadata:        make(map[string]interface{}),
	}

	// Add entry, removing oldest if at capacity
	if len(a.entries) >= a.maxSize {
		a.entries = a.entries[1:]
	}
	a.entries = append(a.entries, entry)
}

// RecordPolicyResult records a multi-policy evaluation result
func (a *PolicyAuditor) RecordPolicyResult(ctx context.Context, result *PolicyResult, resourceType, user, action string, enforcementMode EnforcementMode) {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Record each individual policy evaluation
	for _, r := range result.Results {
		entry := AuditEntry{
			ID:              generateAuditID(),
			Timestamp:       r.EvaluatedAt,
			PolicyID:        r.PolicyID,
			PolicyName:      r.PolicyName,
			ResourceType:    resourceType,
			Allowed:         r.Allowed,
			Duration:        r.Duration,
			Violations:      r.Violations,
			EnforcementMode: enforcementMode,
			User:            user,
			Action:          action,
			Metadata:        make(map[string]interface{}),
		}

		// Add entry, removing oldest if at capacity
		if len(a.entries) >= a.maxSize {
			a.entries = a.entries[1:]
		}
		a.entries = append(a.entries, entry)
	}
}

// GetEntries retrieves audit entries with optional filtering
func (a *PolicyAuditor) GetEntries(filter *AuditFilter) []AuditEntry {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if filter == nil {
		// Return all entries
		result := make([]AuditEntry, len(a.entries))
		copy(result, a.entries)
		return result
	}

	// Filter entries
	filtered := make([]AuditEntry, 0)
	for _, entry := range a.entries {
		if filter.Matches(entry) {
			filtered = append(filtered, entry)
		}
	}

	// Apply limit
	if filter.Limit > 0 && len(filtered) > filter.Limit {
		filtered = filtered[:filter.Limit]
	}

	return filtered
}

// GetSummary returns a summary of audit entries
func (a *PolicyAuditor) GetSummary(filter *AuditFilter) *AuditSummary {
	entries := a.GetEntries(filter)

	summary := &AuditSummary{
		TotalEvaluations:      len(entries),
		AllowedEvaluations:    0,
		DeniedEvaluations:     0,
		TotalViolations:       0,
		ViolationsBySeverity:  make(map[Severity]int),
		ViolationsByPolicy:    make(map[string]int),
		EvaluationsByResource: make(map[string]int),
		AverageDuration:       0,
	}

	totalDuration := time.Duration(0)

	for _, entry := range entries {
		if entry.Allowed {
			summary.AllowedEvaluations++
		} else {
			summary.DeniedEvaluations++
		}

		summary.TotalViolations += len(entry.Violations)
		totalDuration += entry.Duration

		for _, v := range entry.Violations {
			summary.ViolationsBySeverity[v.Severity]++
			summary.ViolationsByPolicy[entry.PolicyID]++
		}

		if entry.ResourceType != "" {
			summary.EvaluationsByResource[entry.ResourceType]++
		}
	}

	if len(entries) > 0 {
		summary.AverageDuration = totalDuration / time.Duration(len(entries))
	}

	return summary
}

// Clear clears all audit entries
func (a *PolicyAuditor) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.entries = make([]AuditEntry, 0, a.maxSize)
}

// AuditFilter defines criteria for filtering audit entries
type AuditFilter struct {
	PolicyID     string
	ResourceType string
	Allowed      *bool // nil means both
	StartTime    time.Time
	EndTime      time.Time
	User         string
	Action       string
	Limit        int
}

// Matches checks if an entry matches the filter
func (f *AuditFilter) Matches(entry AuditEntry) bool {
	if f.PolicyID != "" && entry.PolicyID != f.PolicyID {
		return false
	}
	if f.ResourceType != "" && entry.ResourceType != f.ResourceType {
		return false
	}
	if f.Allowed != nil && entry.Allowed != *f.Allowed {
		return false
	}
	if !f.StartTime.IsZero() && entry.Timestamp.Before(f.StartTime) {
		return false
	}
	if !f.EndTime.IsZero() && entry.Timestamp.After(f.EndTime) {
		return false
	}
	if f.User != "" && entry.User != f.User {
		return false
	}
	if f.Action != "" && entry.Action != f.Action {
		return false
	}
	return true
}

// AuditSummary provides statistical summary of policy evaluations
type AuditSummary struct {
	TotalEvaluations      int                `json:"total_evaluations"`
	AllowedEvaluations    int                `json:"allowed_evaluations"`
	DeniedEvaluations     int                `json:"denied_evaluations"`
	TotalViolations       int                `json:"total_violations"`
	ViolationsBySeverity  map[Severity]int   `json:"violations_by_severity"`
	ViolationsByPolicy    map[string]int     `json:"violations_by_policy"`
	EvaluationsByResource map[string]int     `json:"evaluations_by_resource"`
	AverageDuration       time.Duration      `json:"average_duration"`
}

// ComplianceReport represents a compliance status report
type ComplianceReport struct {
	GeneratedAt         time.Time           `json:"generated_at"`
	Period              ReportPeriod        `json:"period"`
	TotalPolicies       int                 `json:"total_policies"`
	CompliantPolicies   int                 `json:"compliant_policies"`
	ViolatingPolicies   int                 `json:"violating_policies"`
	ComplianceRate      float64             `json:"compliance_rate"`
	PolicyResults       []*PolicySummary    `json:"policy_results"`
	ViolationsBySeverity map[Severity]int   `json:"violations_by_severity"`
	TopViolations       []ViolationSummary  `json:"top_violations"`
}

// ViolationSummary summarizes violations by policy
type ViolationSummary struct {
	PolicyID   string   `json:"policy_id"`
	PolicyName string   `json:"policy_name"`
	Count      int      `json:"count"`
	Severity   Severity `json:"severity"`
}

// ReportPeriod defines the time period for a report
type ReportPeriod struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// ComplianceReporter generates compliance reports
type ComplianceReporter struct {
	auditor  *PolicyAuditor
	registry *Registry
}

// NewComplianceReporter creates a new compliance reporter
func NewComplianceReporter(auditor *PolicyAuditor, registry *Registry) *ComplianceReporter {
	return &ComplianceReporter{
		auditor:  auditor,
		registry: registry,
	}
}

// GenerateReport generates a compliance report for the given period
func (r *ComplianceReporter) GenerateReport(period ReportPeriod) *ComplianceReport {
	// Get audit entries for period
	filter := &AuditFilter{
		StartTime: period.Start,
		EndTime:   period.End,
	}

	entries := r.auditor.GetEntries(filter)

	// Aggregate by policy
	statsMap := make(map[string]*policyStats)
	violationsByPolicy := make(map[string]int)
	violationsBySeverity := make(map[Severity]int)

	for _, entry := range entries {
		if _, exists := statsMap[entry.PolicyID]; !exists {
			statsMap[entry.PolicyID] = &policyStats{
				policyID:   entry.PolicyID,
				policyName: entry.PolicyName,
				total:      0,
				allowed:    0,
				denied:     0,
				violations: 0,
			}
		}

		stats := statsMap[entry.PolicyID]
		stats.total++
		if entry.Allowed {
			stats.allowed++
		} else {
			stats.denied++
		}
		stats.violations += len(entry.Violations)

		for _, v := range entry.Violations {
			violationsByPolicy[entry.PolicyID]++
			violationsBySeverity[v.Severity]++
		}
	}

	// Build policy results
	policyResults := make([]*PolicySummary, 0, len(statsMap))
	compliantCount := 0
	violatingCount := 0

	for _, stats := range statsMap {
		summary := &PolicySummary{
			TotalPolicies:        1,
			AllowedPolicies:      stats.allowed,
			DeniedPolicies:       stats.denied,
			TotalViolations:      stats.violations,
			ViolationsBySeverity: make(map[Severity]int),
		}
		policyResults = append(policyResults, summary)

		if stats.violations == 0 {
			compliantCount++
		} else {
			violatingCount++
		}
	}

	// Calculate top violations
	topViolations := r.getTopViolations(violationsByPolicy, statsMap, 10)

	// Calculate compliance rate
	totalPolicies := len(statsMap)
	complianceRate := 0.0
	if totalPolicies > 0 {
		complianceRate = float64(compliantCount) / float64(totalPolicies) * 100.0
	}

	return &ComplianceReport{
		GeneratedAt:          time.Now(),
		Period:               period,
		TotalPolicies:        totalPolicies,
		CompliantPolicies:    compliantCount,
		ViolatingPolicies:    violatingCount,
		ComplianceRate:       complianceRate,
		PolicyResults:        policyResults,
		ViolationsBySeverity: violationsBySeverity,
		TopViolations:        topViolations,
	}
}

// policyStats tracks statistics for a single policy
type policyStats struct {
	policyID   string
	policyName string
	total      int
	allowed    int
	denied     int
	violations int
}

// getTopViolations returns the top N policies by violation count
func (r *ComplianceReporter) getTopViolations(violationsByPolicy map[string]int, policyStats map[string]*policyStats, limit int) []ViolationSummary {
	summaries := make([]ViolationSummary, 0, len(violationsByPolicy))

	for policyID, count := range violationsByPolicy {
		stats := policyStats[policyID]
		if stats == nil {
			continue
		}

		// Get policy to determine severity
		policy, ok := r.registry.GetPolicy(policyID)
		severity := SeverityMedium // Default
		if ok {
			severity = policy.Severity
		}

		summaries = append(summaries, ViolationSummary{
			PolicyID:   policyID,
			PolicyName: stats.policyName,
			Count:      count,
			Severity:   severity,
		})
	}

	// Sort by count (descending)
	for i := 0; i < len(summaries); i++ {
		for j := i + 1; j < len(summaries); j++ {
			if summaries[j].Count > summaries[i].Count {
				summaries[i], summaries[j] = summaries[j], summaries[i]
			}
		}
	}

	// Return top N
	if len(summaries) > limit {
		summaries = summaries[:limit]
	}

	return summaries
}

// generateAuditID generates a unique audit entry ID
func generateAuditID() string {
	return fmt.Sprintf("audit-%d", time.Now().UnixNano())
}
