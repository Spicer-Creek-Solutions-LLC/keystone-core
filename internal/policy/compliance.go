package policy

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// ComplianceFramework represents a compliance framework (e.g., CIS, SOC2, NIST)
type ComplianceFramework string

// FrameworkCIS and related constants.
const (
	FrameworkCIS      ComplianceFramework = "CIS"
	FrameworkSOC2     ComplianceFramework = "SOC2"
	FrameworkNIST     ComplianceFramework = "NIST-800-53"
	FrameworkHIPAA    ComplianceFramework = "HIPAA"
	FrameworkPCIDSS   ComplianceFramework = "PCI-DSS"
	FrameworkGDPR     ComplianceFramework = "GDPR"
	FrameworkISO27001 ComplianceFramework = "ISO-27001"
	FrameworkCustom   ComplianceFramework = "CUSTOM"
)

// ComplianceControl represents a control within a framework
type ComplianceControl struct {
	// ID is the control identifier (e.g., "CIS-1.1.1")
	ID string `json:"id"`

	// Framework is the compliance framework this control belongs to
	Framework ComplianceFramework `json:"framework"`

	// Title is the control title
	Title string `json:"title"`

	// Description describes the control requirement
	Description string `json:"description"`

	// Category groups related controls
	Category string `json:"category,omitempty"`

	// Severity indicates the impact of non-compliance
	Severity Severity `json:"severity"`

	// PolicyIDs lists policies that implement this control
	PolicyIDs []string `json:"policy_ids"`

	// Requirements describes what the control requires
	Requirements []string `json:"requirements,omitempty"`
}

// ControlMapping maps policies to compliance controls
type ControlMapping struct {
	mu       sync.RWMutex
	controls map[string]*ComplianceControl // control ID -> control
	policies map[string][]string           // policy ID -> control IDs
}

// NewControlMapping creates a new control mapping
func NewControlMapping() *ControlMapping {
	return &ControlMapping{
		controls: make(map[string]*ComplianceControl),
		policies: make(map[string][]string),
	}
}

// AddControl adds a compliance control
func (cm *ControlMapping) AddControl(control *ComplianceControl) error {
	if control == nil {
		return fmt.Errorf("control cannot be nil")
	}
	if control.ID == "" {
		return fmt.Errorf("control ID is required")
	}

	cm.mu.Lock()
	defer cm.mu.Unlock()

	cm.controls[control.ID] = control

	// Update reverse mapping
	for _, policyID := range control.PolicyIDs {
		cm.policies[policyID] = append(cm.policies[policyID], control.ID)
	}

	return nil
}

// GetControl retrieves a control by ID
func (cm *ControlMapping) GetControl(id string) (*ComplianceControl, bool) {
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	control, ok := cm.controls[id]
	return control, ok
}

// GetControlsForPolicy returns all controls mapped to a policy
func (cm *ControlMapping) GetControlsForPolicy(policyID string) []*ComplianceControl {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	controlIDs := cm.policies[policyID]
	controls := make([]*ComplianceControl, 0, len(controlIDs))
	for _, id := range controlIDs {
		if control, ok := cm.controls[id]; ok {
			controls = append(controls, control)
		}
	}
	return controls
}

// GetControlsByFramework returns all controls for a framework
func (cm *ControlMapping) GetControlsByFramework(framework ComplianceFramework) []*ComplianceControl {
	cm.mu.RLock()
	defer cm.mu.RUnlock()

	controls := make([]*ComplianceControl, 0)
	for _, control := range cm.controls {
		if control.Framework == framework {
			controls = append(controls, control)
		}
	}
	return controls
}

// ResourceAuditTrail represents the audit history for a specific resource
type ResourceAuditTrail struct {
	// ResourceID identifies the resource
	ResourceID string `json:"resource_id"`

	// ResourceType is the type of resource (e.g., "file", "package", "service")
	ResourceType string `json:"resource_type"`

	// ResourceName is a human-readable name
	ResourceName string `json:"resource_name,omitempty"`

	// Evaluations lists all policy evaluations for this resource
	Evaluations []*ResourceEvaluation `json:"evaluations"`

	// FirstSeen is when the resource was first evaluated
	FirstSeen time.Time `json:"first_seen"`

	// LastSeen is when the resource was last evaluated
	LastSeen time.Time `json:"last_seen"`

	// ComplianceStatus summarizes overall compliance
	ComplianceStatus ResourceComplianceStatus `json:"compliance_status"`

	// Metadata contains resource-specific metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ResourceComplianceStatus summarizes compliance for a resource
type ResourceComplianceStatus struct {
	IsCompliant       bool     `json:"is_compliant"`
	TotalEvaluations  int      `json:"total_evaluations"`
	PassedEvaluations int      `json:"passed_evaluations"`
	FailedEvaluations int      `json:"failed_evaluations"`
	ComplianceRate    float64  `json:"compliance_rate"`
	HighestSeverity   Severity `json:"highest_severity"`
}

// ResourceEvaluation represents a single policy evaluation for a resource
type ResourceEvaluation struct {
	// EvaluationID is the unique evaluation identifier
	EvaluationID string `json:"evaluation_id"`

	// PolicyID is the policy that was evaluated
	PolicyID string `json:"policy_id"`

	// PolicyName is the policy name
	PolicyName string `json:"policy_name"`

	// Timestamp is when the evaluation occurred
	Timestamp time.Time `json:"timestamp"`

	// Allowed indicates if the policy allowed the action
	Allowed bool `json:"allowed"`

	// Violations lists any policy violations
	Violations []Violation `json:"violations,omitempty"`

	// Duration is how long the evaluation took
	Duration time.Duration `json:"duration"`

	// User is who initiated the action
	User string `json:"user,omitempty"`

	// Action is what action was evaluated
	Action string `json:"action,omitempty"`

	// Controls lists compliance controls covered by this evaluation
	Controls []string `json:"controls,omitempty"`
}

// DetailedComplianceReport extends ComplianceReport with resource-level details
type DetailedComplianceReport struct {
	// Basic report info
	ID          string              `json:"id"`
	GeneratedAt time.Time           `json:"generated_at"`
	GeneratedBy string              `json:"generated_by,omitempty"`
	Period      ReportPeriod        `json:"period"`
	Framework   ComplianceFramework `json:"framework,omitempty"`

	// Summary statistics
	Summary *ComplianceSummary `json:"summary"`

	// Control-level results
	ControlResults []*ControlResult `json:"control_results,omitempty"`

	// Policy-level results
	PolicyResults []*DetailedPolicySummary `json:"policy_results"`

	// Resource-level audit trails
	ResourceTrails []*ResourceAuditTrail `json:"resource_trails,omitempty"`

	// Top violations
	TopViolations []*DetailedViolation `json:"top_violations"`

	// Recommendations for improving compliance
	Recommendations []string `json:"recommendations,omitempty"`

	// Metadata
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// ComplianceSummary provides high-level compliance metrics
type ComplianceSummary struct {
	TotalResources         int     `json:"total_resources"`
	CompliantResources     int     `json:"compliant_resources"`
	NonCompliantResources  int     `json:"non_compliant_resources"`
	ResourceComplianceRate float64 `json:"resource_compliance_rate"`

	TotalPolicies        int     `json:"total_policies"`
	CompliantPolicies    int     `json:"compliant_policies"`
	ViolatingPolicies    int     `json:"violating_policies"`
	PolicyComplianceRate float64 `json:"policy_compliance_rate"`

	TotalEvaluations   int     `json:"total_evaluations"`
	PassedEvaluations  int     `json:"passed_evaluations"`
	FailedEvaluations  int     `json:"failed_evaluations"`
	EvaluationPassRate float64 `json:"evaluation_pass_rate"`

	TotalViolations      int              `json:"total_violations"`
	ViolationsBySeverity map[Severity]int `json:"violations_by_severity"`
	ViolationsByCategory map[string]int   `json:"violations_by_category,omitempty"`

	TotalControls   int     `json:"total_controls,omitempty"`
	PassingControls int     `json:"passing_controls,omitempty"`
	FailingControls int     `json:"failing_controls,omitempty"`
	ControlPassRate float64 `json:"control_pass_rate,omitempty"`

	AverageEvaluationDuration time.Duration `json:"average_evaluation_duration"`
}

// ControlResult represents compliance status for a specific control
type ControlResult struct {
	Control     *ComplianceControl `json:"control"`
	Status      ControlStatus      `json:"status"`
	PassedTests int                `json:"passed_tests"`
	FailedTests int                `json:"failed_tests"`
	TotalTests  int                `json:"total_tests"`
	Evidence    []string           `json:"evidence,omitempty"`
	LastChecked time.Time          `json:"last_checked"`
}

// ControlStatus represents the status of a compliance control
type ControlStatus string

// ControlStatusPass constants define the possible statuses.
const (
	ControlStatusPass        ControlStatus = "PASS"
	ControlStatusFail        ControlStatus = "FAIL"
	ControlStatusPartial     ControlStatus = "PARTIAL"
	ControlStatusNotAssessed ControlStatus = "NOT_ASSESSED"
)

// DetailedPolicySummary extends policy summary with more details
type DetailedPolicySummary struct {
	PolicyID             string           `json:"policy_id"`
	PolicyName           string           `json:"policy_name"`
	PolicyType           PolicyType       `json:"policy_type"`
	Severity             Severity         `json:"severity"`
	TotalEvaluations     int              `json:"total_evaluations"`
	PassedEvaluations    int              `json:"passed_evaluations"`
	FailedEvaluations    int              `json:"failed_evaluations"`
	PassRate             float64          `json:"pass_rate"`
	TotalViolations      int              `json:"total_violations"`
	ViolationsBySeverity map[Severity]int `json:"violations_by_severity"`
	AffectedResources    []string         `json:"affected_resources"`
	MappedControls       []string         `json:"mapped_controls,omitempty"`
	LastEvaluated        time.Time        `json:"last_evaluated"`
}

// DetailedViolation extends violation info for reports
type DetailedViolation struct {
	PolicyID     string    `json:"policy_id"`
	PolicyName   string    `json:"policy_name"`
	ResourceID   string    `json:"resource_id,omitempty"`
	ResourceType string    `json:"resource_type,omitempty"`
	Rule         string    `json:"rule"`
	Message      string    `json:"message"`
	Severity     Severity  `json:"severity"`
	Count        int       `json:"count"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	Remediation  string    `json:"remediation,omitempty"`
	Controls     []string  `json:"controls,omitempty"`
}

// ReportFormat specifies the output format for compliance reports
type ReportFormat string

// ReportFormatJSON constants define the output formats.
const (
	ReportFormatJSON     ReportFormat = "json"
	ReportFormatCSV      ReportFormat = "csv"
	ReportFormatText     ReportFormat = "text"
	ReportFormatMarkdown ReportFormat = "markdown"
	ReportFormatHTML     ReportFormat = "html"
)

// ComplianceReportGenerator generates detailed compliance reports
type ComplianceReportGenerator struct {
	auditStore     AuditStore
	registry       *Registry
	controlMapping *ControlMapping
	mu             sync.RWMutex
}

// NewComplianceReportGenerator creates a new report generator
func NewComplianceReportGenerator(auditStore AuditStore, registry *Registry) *ComplianceReportGenerator {
	return &ComplianceReportGenerator{
		auditStore:     auditStore,
		registry:       registry,
		controlMapping: NewControlMapping(),
	}
}

// SetControlMapping sets the control mapping for framework compliance
func (g *ComplianceReportGenerator) SetControlMapping(mapping *ControlMapping) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.controlMapping = mapping
}

// ReportOptions configures report generation
type ReportOptions struct {
	// Period defines the time range for the report
	Period ReportPeriod

	// Framework filters to a specific compliance framework
	Framework ComplianceFramework

	// IncludeResourceTrails enables resource-level audit trails
	IncludeResourceTrails bool

	// MaxResourceTrails limits the number of resource trails
	MaxResourceTrails int

	// IncludeControls enables control-level reporting
	IncludeControls bool

	// TopViolationsLimit limits top violations
	TopViolationsLimit int

	// GeneratedBy identifies who generated the report
	GeneratedBy string

	// Filters
	PolicyIDs     []string
	ResourceTypes []string
	Users         []string
	MinSeverity   *Severity
}

// DefaultReportOptions returns sensible defaults
func DefaultReportOptions(period ReportPeriod) *ReportOptions {
	return &ReportOptions{
		Period:                period,
		IncludeResourceTrails: true,
		MaxResourceTrails:     1000,
		IncludeControls:       true,
		TopViolationsLimit:    20,
	}
}

// GenerateReport generates a detailed compliance report
func (g *ComplianceReportGenerator) GenerateReport(ctx context.Context, opts *ReportOptions) (*DetailedComplianceReport, error) {
	if opts == nil {
		return nil, fmt.Errorf("report options required")
	}

	// Build audit filter
	filter := &AuditFilter{
		StartTime: opts.Period.Start,
		EndTime:   opts.Period.End,
	}

	// Query audit entries
	entries, err := g.auditStore.Query(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("failed to query audit entries: %w", err)
	}

	// Apply additional filters
	entries = g.filterEntries(entries, opts)

	// Build report
	report := &DetailedComplianceReport{
		ID:          fmt.Sprintf("compliance-report-%d", time.Now().UnixNano()),
		GeneratedAt: time.Now(),
		GeneratedBy: opts.GeneratedBy,
		Period:      opts.Period,
		Framework:   opts.Framework,
		Metadata:    make(map[string]interface{}),
	}

	// Generate summary
	report.Summary = g.generateSummary(entries, opts)

	// Generate policy results
	report.PolicyResults = g.generatePolicyResults(entries, opts)

	// Generate resource trails if enabled
	if opts.IncludeResourceTrails {
		report.ResourceTrails = g.generateResourceTrails(entries, opts)
	}

	// Generate control results if framework specified
	if opts.IncludeControls && opts.Framework != "" {
		report.ControlResults = g.generateControlResults(entries, opts)
	}

	// Generate top violations
	report.TopViolations = g.generateTopViolations(entries, opts)

	// Generate recommendations
	report.Recommendations = g.generateRecommendations(report)

	return report, nil
}

// filterEntries applies additional filters to entries
func (g *ComplianceReportGenerator) filterEntries(entries []*AuditEntry, opts *ReportOptions) []*AuditEntry {
	if opts == nil {
		return entries
	}

	filtered := make([]*AuditEntry, 0, len(entries))

	policySet := make(map[string]bool)
	for _, id := range opts.PolicyIDs {
		policySet[id] = true
	}

	resourceSet := make(map[string]bool)
	for _, rt := range opts.ResourceTypes {
		resourceSet[rt] = true
	}

	userSet := make(map[string]bool)
	for _, u := range opts.Users {
		userSet[u] = true
	}

	for _, entry := range entries {
		if len(policySet) > 0 && !policySet[entry.PolicyID] {
			continue
		}
		if len(resourceSet) > 0 && !resourceSet[entry.ResourceType] {
			continue
		}
		if len(userSet) > 0 && !userSet[entry.User] {
			continue
		}
		if opts.MinSeverity != nil {
			hasHighEnoughSeverity := false
			for _, v := range entry.Violations {
				if severityRank(v.Severity) >= severityRank(*opts.MinSeverity) {
					hasHighEnoughSeverity = true
					break
				}
			}
			if !entry.Allowed && !hasHighEnoughSeverity {
				continue
			}
		}
		filtered = append(filtered, entry)
	}

	return filtered
}

// severityRank returns numeric rank for severity comparison
func severityRank(s Severity) int {
	switch s {
	case SeverityCritical:
		return 4
	case SeverityHigh:
		return 3
	case SeverityMedium:
		return 2
	case SeverityLow:
		return 1
	default:
		return 0
	}
}

// generateSummary generates the compliance summary
func (g *ComplianceReportGenerator) generateSummary(entries []*AuditEntry, opts *ReportOptions) *ComplianceSummary {
	summary := &ComplianceSummary{
		ViolationsBySeverity: make(map[Severity]int),
		ViolationsByCategory: make(map[string]int),
	}

	resources := make(map[string]bool)
	compliantResources := make(map[string]bool)
	policies := make(map[string]bool)
	compliantPolicies := make(map[string]bool)
	violatingPolicies := make(map[string]bool)

	var totalDuration time.Duration

	for _, entry := range entries {
		summary.TotalEvaluations++
		totalDuration += entry.Duration

		if entry.Allowed {
			summary.PassedEvaluations++
		} else {
			summary.FailedEvaluations++
		}

		// Track resources
		if entry.ResourceType != "" {
			resourceKey := entry.ResourceType + ":" + entry.Metadata["resource_id"].(string)
			resources[resourceKey] = true
			if entry.Allowed {
				if !compliantResources[resourceKey] {
					compliantResources[resourceKey] = true
				}
			} else {
				delete(compliantResources, resourceKey)
			}
		}

		// Track policies
		policies[entry.PolicyID] = true
		if len(entry.Violations) > 0 {
			violatingPolicies[entry.PolicyID] = true
			delete(compliantPolicies, entry.PolicyID)
		} else if !violatingPolicies[entry.PolicyID] {
			compliantPolicies[entry.PolicyID] = true
		}

		// Count violations
		for _, v := range entry.Violations {
			summary.TotalViolations++
			summary.ViolationsBySeverity[v.Severity]++
		}
	}

	summary.TotalResources = len(resources)
	summary.CompliantResources = len(compliantResources)
	summary.NonCompliantResources = summary.TotalResources - summary.CompliantResources
	if summary.TotalResources > 0 {
		summary.ResourceComplianceRate = float64(summary.CompliantResources) / float64(summary.TotalResources) * 100.0
	}

	summary.TotalPolicies = len(policies)
	summary.CompliantPolicies = len(compliantPolicies)
	summary.ViolatingPolicies = len(violatingPolicies)
	if summary.TotalPolicies > 0 {
		summary.PolicyComplianceRate = float64(summary.CompliantPolicies) / float64(summary.TotalPolicies) * 100.0
	}

	if summary.TotalEvaluations > 0 {
		summary.EvaluationPassRate = float64(summary.PassedEvaluations) / float64(summary.TotalEvaluations) * 100.0
		summary.AverageEvaluationDuration = totalDuration / time.Duration(summary.TotalEvaluations)
	}

	// Control statistics
	if opts.Framework != "" && g.controlMapping != nil {
		controls := g.controlMapping.GetControlsByFramework(opts.Framework)
		summary.TotalControls = len(controls)
		// Control pass/fail would be determined in generateControlResults
	}

	return summary
}

// generatePolicyResults generates detailed policy results
func (g *ComplianceReportGenerator) generatePolicyResults(entries []*AuditEntry, opts *ReportOptions) []*DetailedPolicySummary {
	policyStats := make(map[string]*DetailedPolicySummary)

	for _, entry := range entries {
		if _, exists := policyStats[entry.PolicyID]; !exists {
			policyStats[entry.PolicyID] = &DetailedPolicySummary{
				PolicyID:             entry.PolicyID,
				PolicyName:           entry.PolicyName,
				PolicyType:           entry.PolicyType,
				ViolationsBySeverity: make(map[Severity]int),
				AffectedResources:    make([]string, 0),
			}

			// Get policy severity if available
			if policy, ok := g.registry.GetPolicy(entry.PolicyID); ok {
				policyStats[entry.PolicyID].Severity = policy.Severity
			}

			// Get mapped controls
			if g.controlMapping != nil {
				controls := g.controlMapping.GetControlsForPolicy(entry.PolicyID)
				for _, c := range controls {
					policyStats[entry.PolicyID].MappedControls = append(
						policyStats[entry.PolicyID].MappedControls, c.ID)
				}
			}
		}

		stats := policyStats[entry.PolicyID]
		stats.TotalEvaluations++
		if entry.Allowed {
			stats.PassedEvaluations++
		} else {
			stats.FailedEvaluations++
			if entry.ResourceType != "" {
				stats.AffectedResources = appendUnique(stats.AffectedResources, entry.ResourceType)
			}
		}

		for _, v := range entry.Violations {
			stats.TotalViolations++
			stats.ViolationsBySeverity[v.Severity]++
		}

		if entry.Timestamp.After(stats.LastEvaluated) {
			stats.LastEvaluated = entry.Timestamp
		}
	}

	// Calculate pass rates and convert to slice
	results := make([]*DetailedPolicySummary, 0, len(policyStats))
	for _, stats := range policyStats {
		if stats.TotalEvaluations > 0 {
			stats.PassRate = float64(stats.PassedEvaluations) / float64(stats.TotalEvaluations) * 100.0
		}
		results = append(results, stats)
	}

	// Sort by violation count descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalViolations > results[j].TotalViolations
	})

	return results
}

// generateResourceTrails generates resource-level audit trails
func (g *ComplianceReportGenerator) generateResourceTrails(entries []*AuditEntry, opts *ReportOptions) []*ResourceAuditTrail {
	resourceMap := make(map[string]*ResourceAuditTrail)

	for _, entry := range entries {
		if entry.ResourceType == "" {
			continue
		}

		// Create resource key from type and ID
		var resourceID string
		if id, ok := entry.Metadata["resource_id"].(string); ok {
			resourceID = id
		} else {
			resourceID = entry.ResourceType // Fallback to type if no ID
		}
		resourceKey := entry.ResourceType + ":" + resourceID

		if _, exists := resourceMap[resourceKey]; !exists {
			resourceMap[resourceKey] = &ResourceAuditTrail{
				ResourceID:   resourceID,
				ResourceType: entry.ResourceType,
				Evaluations:  make([]*ResourceEvaluation, 0),
				FirstSeen:    entry.Timestamp,
				LastSeen:     entry.Timestamp,
				Metadata:     make(map[string]interface{}),
			}
		}

		trail := resourceMap[resourceKey]

		// Update timestamps
		if entry.Timestamp.Before(trail.FirstSeen) {
			trail.FirstSeen = entry.Timestamp
		}
		if entry.Timestamp.After(trail.LastSeen) {
			trail.LastSeen = entry.Timestamp
		}

		// Add evaluation
		eval := &ResourceEvaluation{
			EvaluationID: entry.ID,
			PolicyID:     entry.PolicyID,
			PolicyName:   entry.PolicyName,
			Timestamp:    entry.Timestamp,
			Allowed:      entry.Allowed,
			Violations:   entry.Violations,
			Duration:     entry.Duration,
			User:         entry.User,
			Action:       entry.Action,
		}

		// Add control mappings
		if g.controlMapping != nil {
			controls := g.controlMapping.GetControlsForPolicy(entry.PolicyID)
			for _, c := range controls {
				eval.Controls = append(eval.Controls, c.ID)
			}
		}

		trail.Evaluations = append(trail.Evaluations, eval)
	}

	// Calculate compliance status for each resource
	trails := make([]*ResourceAuditTrail, 0, len(resourceMap))
	for _, trail := range resourceMap {
		passed := 0
		failed := 0
		highestSeverity := SeverityLow

		for _, eval := range trail.Evaluations {
			if eval.Allowed {
				passed++
			} else {
				failed++
				for _, v := range eval.Violations {
					if severityRank(v.Severity) > severityRank(highestSeverity) {
						highestSeverity = v.Severity
					}
				}
			}
		}

		trail.ComplianceStatus = ResourceComplianceStatus{
			IsCompliant:       failed == 0,
			TotalEvaluations:  passed + failed,
			PassedEvaluations: passed,
			FailedEvaluations: failed,
			HighestSeverity:   highestSeverity,
		}
		if trail.ComplianceStatus.TotalEvaluations > 0 {
			trail.ComplianceStatus.ComplianceRate = float64(passed) /
				float64(trail.ComplianceStatus.TotalEvaluations) * 100.0
		}

		trails = append(trails, trail)
	}

	// Sort by compliance (non-compliant first) then by failure count
	sort.Slice(trails, func(i, j int) bool {
		if trails[i].ComplianceStatus.IsCompliant != trails[j].ComplianceStatus.IsCompliant {
			return !trails[i].ComplianceStatus.IsCompliant
		}
		return trails[i].ComplianceStatus.FailedEvaluations > trails[j].ComplianceStatus.FailedEvaluations
	})

	// Apply limit
	if opts.MaxResourceTrails > 0 && len(trails) > opts.MaxResourceTrails {
		trails = trails[:opts.MaxResourceTrails]
	}

	return trails
}

// generateControlResults generates control-level results
func (g *ComplianceReportGenerator) generateControlResults(entries []*AuditEntry, opts *ReportOptions) []*ControlResult {
	if g.controlMapping == nil {
		return nil
	}

	controls := g.controlMapping.GetControlsByFramework(opts.Framework)
	if len(controls) == 0 {
		return nil
	}

	// Map entries to policies
	policyResults := make(map[string]struct{ passed, failed int })
	for _, entry := range entries {
		stats := policyResults[entry.PolicyID]
		if entry.Allowed {
			stats.passed++
		} else {
			stats.failed++
		}
		policyResults[entry.PolicyID] = stats
	}

	// Build control results
	results := make([]*ControlResult, 0, len(controls))
	for _, control := range controls {
		result := &ControlResult{
			Control:     control,
			Status:      ControlStatusNotAssessed,
			LastChecked: time.Now(),
			Evidence:    make([]string, 0),
		}

		// Aggregate policy results for this control
		for _, policyID := range control.PolicyIDs {
			if stats, ok := policyResults[policyID]; ok {
				result.PassedTests += stats.passed
				result.FailedTests += stats.failed
				result.TotalTests += stats.passed + stats.failed
			}
		}

		// Determine status
		switch {
		case result.TotalTests == 0:
			result.Status = ControlStatusNotAssessed
		case result.FailedTests == 0:
			result.Status = ControlStatusPass
		case result.PassedTests == 0:
			result.Status = ControlStatusFail
		default:
			result.Status = ControlStatusPartial
		}

		results = append(results, result)
	}

	// Sort by status (fail first) then by control ID
	sort.Slice(results, func(i, j int) bool {
		statusOrder := map[ControlStatus]int{
			ControlStatusFail:        0,
			ControlStatusPartial:     1,
			ControlStatusNotAssessed: 2,
			ControlStatusPass:        3,
		}
		if statusOrder[results[i].Status] != statusOrder[results[j].Status] {
			return statusOrder[results[i].Status] < statusOrder[results[j].Status]
		}
		return results[i].Control.ID < results[j].Control.ID
	})

	return results
}

// generateTopViolations generates top violation summary
func (g *ComplianceReportGenerator) generateTopViolations(entries []*AuditEntry, opts *ReportOptions) []*DetailedViolation {
	violationMap := make(map[string]*DetailedViolation)

	for _, entry := range entries {
		for _, v := range entry.Violations {
			key := entry.PolicyID + ":" + v.Rule
			if _, exists := violationMap[key]; !exists {
				violationMap[key] = &DetailedViolation{
					PolicyID:     entry.PolicyID,
					PolicyName:   entry.PolicyName,
					ResourceType: entry.ResourceType,
					Rule:         v.Rule,
					Message:      v.Message,
					Severity:     v.Severity,
					Count:        0,
					FirstSeen:    entry.Timestamp,
					LastSeen:     entry.Timestamp,
					Remediation:  v.Remediation,
				}

				// Add control mappings
				if g.controlMapping != nil {
					controls := g.controlMapping.GetControlsForPolicy(entry.PolicyID)
					for _, c := range controls {
						violationMap[key].Controls = append(violationMap[key].Controls, c.ID)
					}
				}
			}

			viol := violationMap[key]
			viol.Count++
			if entry.Timestamp.Before(viol.FirstSeen) {
				viol.FirstSeen = entry.Timestamp
			}
			if entry.Timestamp.After(viol.LastSeen) {
				viol.LastSeen = entry.Timestamp
			}
		}
	}

	// Convert to slice and sort
	violations := make([]*DetailedViolation, 0, len(violationMap))
	for _, v := range violationMap {
		violations = append(violations, v)
	}

	sort.Slice(violations, func(i, j int) bool {
		// Sort by severity first, then by count
		if severityRank(violations[i].Severity) != severityRank(violations[j].Severity) {
			return severityRank(violations[i].Severity) > severityRank(violations[j].Severity)
		}
		return violations[i].Count > violations[j].Count
	})

	// Apply limit
	limit := opts.TopViolationsLimit
	if limit <= 0 {
		limit = 20
	}
	if len(violations) > limit {
		violations = violations[:limit]
	}

	return violations
}

// generateRecommendations generates recommendations based on report data
func (g *ComplianceReportGenerator) generateRecommendations(report *DetailedComplianceReport) []string {
	recommendations := make([]string, 0)

	if report.Summary == nil {
		return recommendations
	}

	// Check for critical/high severity violations
	if count := report.Summary.ViolationsBySeverity[SeverityCritical]; count > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Address %d critical severity violations immediately to reduce security risk", count))
	}
	if count := report.Summary.ViolationsBySeverity[SeverityHigh]; count > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Prioritize remediation of %d high severity violations", count))
	}

	// Check compliance rate
	if report.Summary.EvaluationPassRate < 90.0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Evaluation pass rate (%.1f%%) is below 90%%. Review failing policies and resource configurations.",
				report.Summary.EvaluationPassRate))
	}

	// Check for non-compliant resources
	if report.Summary.NonCompliantResources > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Investigate %d non-compliant resources and apply remediation",
				report.Summary.NonCompliantResources))
	}

	// Check control status
	if report.Summary.FailingControls > 0 {
		recommendations = append(recommendations,
			fmt.Sprintf("Review %d failing compliance controls and implement required policies",
				report.Summary.FailingControls))
	}

	return recommendations
}

// appendUnique appends item to slice if not already present
func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// FormatReport formats a report in the specified format
func (g *ComplianceReportGenerator) FormatReport(report *DetailedComplianceReport, format ReportFormat) ([]byte, error) {
	switch format {
	case ReportFormatJSON:
		return g.formatJSON(report)
	case ReportFormatCSV:
		return g.formatCSV(report)
	case ReportFormatText:
		return g.formatText(report)
	case ReportFormatMarkdown:
		return g.formatMarkdown(report)
	case ReportFormatHTML:
		return g.formatHTML(report)
	default:
		return nil, fmt.Errorf("unsupported report format: %s", format)
	}
}

// formatJSON formats report as JSON
func (g *ComplianceReportGenerator) formatJSON(report *DetailedComplianceReport) ([]byte, error) {
	return json.MarshalIndent(report, "", "  ")
}

// formatCSV formats report as CSV (summary + violations)
func (g *ComplianceReportGenerator) formatCSV(report *DetailedComplianceReport) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)

	// Write header
	w.Write([]string{
		"Policy ID", "Policy Name", "Rule", "Severity",
		"Count", "First Seen", "Last Seen", "Remediation",
	})

	// Write violation rows
	for _, v := range report.TopViolations {
		w.Write([]string{
			v.PolicyID,
			v.PolicyName,
			v.Rule,
			string(v.Severity),
			fmt.Sprintf("%d", v.Count),
			v.FirstSeen.Format(time.RFC3339),
			v.LastSeen.Format(time.RFC3339),
			v.Remediation,
		})
	}

	w.Flush()
	return buf.Bytes(), w.Error()
}

// formatText formats report as plain text
func (g *ComplianceReportGenerator) formatText(report *DetailedComplianceReport) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString("COMPLIANCE REPORT\n")
	buf.WriteString(strings.Repeat("=", 60) + "\n\n")

	buf.WriteString(fmt.Sprintf("Report ID:     %s\n", report.ID))
	buf.WriteString(fmt.Sprintf("Generated:     %s\n", report.GeneratedAt.Format(time.RFC3339)))
	buf.WriteString(fmt.Sprintf("Period:        %s to %s\n",
		report.Period.Start.Format(time.RFC3339),
		report.Period.End.Format(time.RFC3339)))
	if report.Framework != "" {
		buf.WriteString(fmt.Sprintf("Framework:     %s\n", report.Framework))
	}
	buf.WriteString("\n")

	// Summary
	if report.Summary != nil {
		buf.WriteString("SUMMARY\n")
		buf.WriteString(strings.Repeat("-", 40) + "\n")
		buf.WriteString(fmt.Sprintf("Total Evaluations:      %d\n", report.Summary.TotalEvaluations))
		buf.WriteString(fmt.Sprintf("Passed Evaluations:     %d (%.1f%%)\n",
			report.Summary.PassedEvaluations, report.Summary.EvaluationPassRate))
		buf.WriteString(fmt.Sprintf("Failed Evaluations:     %d\n", report.Summary.FailedEvaluations))
		buf.WriteString(fmt.Sprintf("Total Violations:       %d\n", report.Summary.TotalViolations))
		buf.WriteString(fmt.Sprintf("  Critical:             %d\n", report.Summary.ViolationsBySeverity[SeverityCritical]))
		buf.WriteString(fmt.Sprintf("  High:                 %d\n", report.Summary.ViolationsBySeverity[SeverityHigh]))
		buf.WriteString(fmt.Sprintf("  Medium:               %d\n", report.Summary.ViolationsBySeverity[SeverityMedium]))
		buf.WriteString(fmt.Sprintf("  Low:                  %d\n", report.Summary.ViolationsBySeverity[SeverityLow]))
		buf.WriteString("\n")
	}

	// Top violations
	if len(report.TopViolations) > 0 {
		buf.WriteString("TOP VIOLATIONS\n")
		buf.WriteString(strings.Repeat("-", 40) + "\n")
		for i, v := range report.TopViolations {
			buf.WriteString(fmt.Sprintf("%d. [%s] %s - %s (%d occurrences)\n",
				i+1, v.Severity, v.PolicyName, v.Rule, v.Count))
		}
		buf.WriteString("\n")
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		buf.WriteString("RECOMMENDATIONS\n")
		buf.WriteString(strings.Repeat("-", 40) + "\n")
		for i, rec := range report.Recommendations {
			buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
		}
	}

	return buf.Bytes(), nil
}

// formatMarkdown formats report as Markdown
func (g *ComplianceReportGenerator) formatMarkdown(report *DetailedComplianceReport) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString("# Compliance Report\n\n")
	buf.WriteString(fmt.Sprintf("**Report ID:** %s  \n", report.ID))
	buf.WriteString(fmt.Sprintf("**Generated:** %s  \n", report.GeneratedAt.Format(time.RFC3339)))
	buf.WriteString(fmt.Sprintf("**Period:** %s to %s  \n",
		report.Period.Start.Format("2006-01-02"),
		report.Period.End.Format("2006-01-02")))
	if report.Framework != "" {
		buf.WriteString(fmt.Sprintf("**Framework:** %s  \n", report.Framework))
	}
	buf.WriteString("\n")

	// Summary
	if report.Summary != nil {
		buf.WriteString("## Summary\n\n")
		buf.WriteString("| Metric | Value |\n")
		buf.WriteString("|--------|-------|\n")
		buf.WriteString(fmt.Sprintf("| Total Evaluations | %d |\n", report.Summary.TotalEvaluations))
		buf.WriteString(fmt.Sprintf("| Pass Rate | %.1f%% |\n", report.Summary.EvaluationPassRate))
		buf.WriteString(fmt.Sprintf("| Total Violations | %d |\n", report.Summary.TotalViolations))
		buf.WriteString(fmt.Sprintf("| Critical | %d |\n", report.Summary.ViolationsBySeverity[SeverityCritical]))
		buf.WriteString(fmt.Sprintf("| High | %d |\n", report.Summary.ViolationsBySeverity[SeverityHigh]))
		buf.WriteString(fmt.Sprintf("| Medium | %d |\n", report.Summary.ViolationsBySeverity[SeverityMedium]))
		buf.WriteString(fmt.Sprintf("| Low | %d |\n", report.Summary.ViolationsBySeverity[SeverityLow]))
		buf.WriteString("\n")
	}

	// Top violations
	if len(report.TopViolations) > 0 {
		buf.WriteString("## Top Violations\n\n")
		buf.WriteString("| Severity | Policy | Rule | Count |\n")
		buf.WriteString("|----------|--------|------|-------|\n")
		for _, v := range report.TopViolations {
			buf.WriteString(fmt.Sprintf("| %s | %s | %s | %d |\n",
				v.Severity, v.PolicyName, v.Rule, v.Count))
		}
		buf.WriteString("\n")
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		buf.WriteString("## Recommendations\n\n")
		for i, rec := range report.Recommendations {
			buf.WriteString(fmt.Sprintf("%d. %s\n", i+1, rec))
		}
	}

	return buf.Bytes(), nil
}

// formatHTML formats report as HTML
func (g *ComplianceReportGenerator) formatHTML(report *DetailedComplianceReport) ([]byte, error) {
	var buf bytes.Buffer

	buf.WriteString(`<!DOCTYPE html>
<html>
<head>
    <title>Compliance Report</title>
    <style>
        body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; margin: 40px; }
        h1 { color: #333; }
        h2 { color: #555; border-bottom: 2px solid #ddd; padding-bottom: 10px; }
        table { border-collapse: collapse; width: 100%; margin: 20px 0; }
        th, td { border: 1px solid #ddd; padding: 12px; text-align: left; }
        th { background-color: #f5f5f5; }
        .severity-critical { color: #d32f2f; font-weight: bold; }
        .severity-high { color: #f57c00; font-weight: bold; }
        .severity-medium { color: #fbc02d; }
        .severity-low { color: #388e3c; }
        .summary-card { background: #f9f9f9; padding: 20px; border-radius: 8px; margin: 20px 0; }
        .metric { display: inline-block; margin-right: 40px; }
        .metric-value { font-size: 24px; font-weight: bold; color: #1976d2; }
        .metric-label { font-size: 14px; color: #666; }
    </style>
</head>
<body>
    <h1>Compliance Report</h1>
`)

	buf.WriteString(fmt.Sprintf(`    <p><strong>Report ID:</strong> %s<br>
    <strong>Generated:</strong> %s<br>
    <strong>Period:</strong> %s to %s`,
		report.ID,
		report.GeneratedAt.Format(time.RFC3339),
		report.Period.Start.Format("2006-01-02"),
		report.Period.End.Format("2006-01-02")))

	if report.Framework != "" {
		buf.WriteString(fmt.Sprintf(`<br><strong>Framework:</strong> %s`, report.Framework))
	}
	buf.WriteString(`</p>`)

	// Summary
	if report.Summary != nil {
		buf.WriteString(`
    <h2>Summary</h2>
    <div class="summary-card">
`)
		buf.WriteString(fmt.Sprintf(`        <div class="metric">
            <div class="metric-value">%d</div>
            <div class="metric-label">Total Evaluations</div>
        </div>`, report.Summary.TotalEvaluations))
		buf.WriteString(fmt.Sprintf(`        <div class="metric">
            <div class="metric-value">%.1f%%</div>
            <div class="metric-label">Pass Rate</div>
        </div>`, report.Summary.EvaluationPassRate))
		buf.WriteString(fmt.Sprintf(`        <div class="metric">
            <div class="metric-value">%d</div>
            <div class="metric-label">Violations</div>
        </div>`, report.Summary.TotalViolations))
		buf.WriteString(`
    </div>
`)
	}

	// Top violations table
	if len(report.TopViolations) > 0 {
		buf.WriteString(`
    <h2>Top Violations</h2>
    <table>
        <tr><th>Severity</th><th>Policy</th><th>Rule</th><th>Count</th></tr>
`)
		for _, v := range report.TopViolations {
			severityClass := "severity-" + strings.ToLower(string(v.Severity))
			buf.WriteString(fmt.Sprintf(`        <tr><td class="%s">%s</td><td>%s</td><td>%s</td><td>%d</td></tr>
`,
				severityClass, v.Severity, v.PolicyName, v.Rule, v.Count))
		}
		buf.WriteString(`    </table>
`)
	}

	// Recommendations
	if len(report.Recommendations) > 0 {
		buf.WriteString(`
    <h2>Recommendations</h2>
    <ol>
`)
		for _, rec := range report.Recommendations {
			buf.WriteString(fmt.Sprintf(`        <li>%s</li>
`, rec))
		}
		buf.WriteString(`    </ol>
`)
	}

	buf.WriteString(`</body>
</html>`)

	return buf.Bytes(), nil
}

// LoadBuiltinControlMappings loads predefined control mappings for common frameworks
func LoadBuiltinControlMappings(mapping *ControlMapping) {
	// CIS Benchmark Controls (example subset)
	cisControls := []*ComplianceControl{
		{
			ID:          "CIS-1.1.1",
			Framework:   FrameworkCIS,
			Title:       "Ensure system is configured to prevent IP forwarding",
			Description: "IP forwarding should be disabled unless the system is a router",
			Category:    "Network Configuration",
			Severity:    SeverityMedium,
		},
		{
			ID:          "CIS-1.1.2",
			Framework:   FrameworkCIS,
			Title:       "Ensure packet redirect sending is disabled",
			Description: "ICMP redirects should not be accepted",
			Category:    "Network Configuration",
			Severity:    SeverityMedium,
		},
		{
			ID:          "CIS-4.1.1",
			Framework:   FrameworkCIS,
			Title:       "Ensure auditing is enabled",
			Description: "System auditing should be configured and running",
			Category:    "Logging and Auditing",
			Severity:    SeverityHigh,
		},
		{
			ID:          "CIS-5.2.1",
			Framework:   FrameworkCIS,
			Title:       "Ensure SSH root login is disabled",
			Description: "Root login via SSH should be prohibited",
			Category:    "Access Control",
			Severity:    SeverityHigh,
		},
	}

	for _, c := range cisControls {
		_ = mapping.AddControl(c) //nolint:errcheck // control registration in init
	}

	// SOC2 Controls (example subset)
	soc2Controls := []*ComplianceControl{
		{
			ID:          "SOC2-CC6.1",
			Framework:   FrameworkSOC2,
			Title:       "Logical and Physical Access Controls",
			Description: "The entity implements logical access controls to protect against threats",
			Category:    "Access Control",
			Severity:    SeverityHigh,
		},
		{
			ID:          "SOC2-CC6.2",
			Framework:   FrameworkSOC2,
			Title:       "User Authentication",
			Description: "Prior to accessing protected information, user authentication is required",
			Category:    "Access Control",
			Severity:    SeverityHigh,
		},
		{
			ID:          "SOC2-CC7.1",
			Framework:   FrameworkSOC2,
			Title:       "Security Monitoring",
			Description: "The entity monitors system components for anomalies",
			Category:    "Monitoring",
			Severity:    SeverityMedium,
		},
	}

	for _, c := range soc2Controls {
		_ = mapping.AddControl(c) //nolint:errcheck // control registration in init
	}

	// NIST 800-53 Controls (example subset)
	nistControls := []*ComplianceControl{
		{
			ID:          "NIST-AC-2",
			Framework:   FrameworkNIST,
			Title:       "Account Management",
			Description: "The organization manages information system accounts",
			Category:    "Access Control",
			Severity:    SeverityHigh,
		},
		{
			ID:          "NIST-AU-2",
			Framework:   FrameworkNIST,
			Title:       "Audit Events",
			Description: "The organization determines auditable events",
			Category:    "Audit and Accountability",
			Severity:    SeverityMedium,
		},
		{
			ID:          "NIST-CM-6",
			Framework:   FrameworkNIST,
			Title:       "Configuration Settings",
			Description: "The organization establishes and documents configuration settings",
			Category:    "Configuration Management",
			Severity:    SeverityMedium,
		},
	}

	for _, c := range nistControls {
		_ = mapping.AddControl(c) //nolint:errcheck // control registration in init
	}
}
