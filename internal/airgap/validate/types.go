// Package validate provides air-gap compliance validation tooling.
// It scans binaries, configuration files, module registries, and active
// network connections to identify external dependencies that would
// break air-gapped operation.
package validate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"
)

// Severity indicates the importance of a finding.
type Severity string

// Severity levels.
const (
	SeverityPass Severity = "pass"
	SeverityWarn Severity = "warn"
	SeverityFail Severity = "fail"
)

// CheckCategory groups related checks.
type CheckCategory string

// Check categories.
const (
	CategoryBinary        CheckCategory = "binary"
	CategoryConfiguration CheckCategory = "configuration"
	CategoryModule        CheckCategory = "module"
	CategoryNetwork       CheckCategory = "network"
)

// Finding represents a single check result.
type Finding struct {
	Category    CheckCategory `json:"category"`
	Check       string        `json:"check"`
	Severity    Severity      `json:"severity"`
	Message     string        `json:"message"`
	Detail      string        `json:"detail,omitempty"`
	Remediation string        `json:"remediation,omitempty"`
}

// Report aggregates findings from all checkers.
type Report struct {
	Timestamp time.Time `json:"timestamp"`
	Hostname  string    `json:"hostname"`
	Compliant bool      `json:"compliant"`
	PassCount int       `json:"pass_count"`
	WarnCount int       `json:"warn_count"`
	FailCount int       `json:"fail_count"`
	Findings  []Finding `json:"findings"`
}

// Checker is the interface implemented by each validation check.
type Checker interface {
	Name() string
	Category() CheckCategory
	Check(ctx context.Context) ([]Finding, error)
}

// WriteReportToFile writes a report as JSON.
func WriteReportToFile(report *Report, path string) error {
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal report: %w", err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write report: %w", err)
	}
	return nil
}
