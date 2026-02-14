// Package policy provides a public client for the Keystone Core policy API over gRPC.
//
// This package is UNSTABLE — the API may change between minor versions.
package policy

import "time"

// AuditEntry represents a policy evaluation audit record.
type AuditEntry struct {
	ID              string
	Timestamp       time.Time
	PolicyID        string
	PolicyName      string
	PolicyType      string
	ResourceType    string
	Allowed         bool
	DurationMS      int64
	Violations      []Violation
	EnforcementMode string
	User            string
	Action          string
	Metadata        map[string]string
}

// Violation represents a policy violation.
type Violation struct {
	Rule        string
	Message     string
	Severity    string
	Path        string
	Expected    string
	Actual      string
	Remediation string
}

// AuditLogResult contains the result of a GetAuditLog call.
type AuditLogResult struct {
	Entries       []AuditEntry
	NextPageToken string
	TotalCount    int
}

// ViolationRecord represents a recorded violation with context.
type ViolationRecord struct {
	ID           string
	Timestamp    time.Time
	PolicyID     string
	PolicyName   string
	Violation    Violation
	ResourceType string
	User         string
}

// ViolationListResult contains the result of a ListViolations call.
type ViolationListResult struct {
	Records       []ViolationRecord
	NextPageToken string
	TotalCount    int
}

// ComplianceReport contains compliance statistics.
type ComplianceReport struct {
	StartTime               time.Time
	EndTime                 time.Time
	ComplianceRate          float64
	TotalEvaluations        int64
	CompliantEvaluations    int64
	NonCompliantEvaluations int64
	PolicyStats             []ComplianceStats
	TopViolations           []ViolationSummary
	ViolationsBySeverity    map[string]int64
}

// ComplianceStats contains per-policy compliance statistics.
type ComplianceStats struct {
	PolicyID             string
	PolicyName           string
	TotalEvaluations     int64
	CompliantEvaluations int64
	ComplianceRate       float64
	ViolationCount       int64
}

// ViolationSummary summarizes a frequently occurring violation.
type ViolationSummary struct {
	PolicyID   string
	PolicyName string
	Rule       string
	Count      int64
	Severity   string
}

// EvalResult contains the result of evaluating a single policy.
type EvalResult struct {
	PolicyID    string
	PolicyName  string
	Allowed     bool
	Violations  []Violation
	Warnings    []string
	Message     string
	DurationMS  int64
	EvaluatedAt time.Time
}

// AuditLogOptions contains filter options for GetAuditLog.
type AuditLogOptions struct {
	PolicyID     string
	User         string
	Action       string
	ResourceType string
	Allowed      *bool
	StartTime    time.Time
	EndTime      time.Time
	PageSize     int32
	PageToken    string
}

// ViolationListOptions contains filter options for ListViolations.
type ViolationListOptions struct {
	PolicyID     string
	Severity     string
	ResourceType string
	User         string
	StartTime    time.Time
	EndTime      time.Time
	PageSize     int32
	PageToken    string
}

// ComplianceReportOptions contains options for GetComplianceReport.
type ComplianceReportOptions struct {
	StartTime         time.Time
	EndTime           time.Time
	PolicySetID       string
	IncludeViolations bool
}

// EvalOptions contains options for EvaluatePolicy.
type EvalOptions struct {
	Resource map[string]interface{}
	Action   string
	User     string
	Context  map[string]string
}
