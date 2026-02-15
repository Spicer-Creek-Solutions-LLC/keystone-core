package validate

import (
	"context"
	"os"
	"time"
)

// Validator runs all registered checkers and aggregates results into a Report.
type Validator struct {
	checkers []Checker
}

// NewValidator creates an empty validator.
func NewValidator() *Validator {
	return &Validator{}
}

// AddChecker registers a checker.
func (v *Validator) AddChecker(c Checker) {
	v.checkers = append(v.checkers, c)
}

// Validate runs all checkers and returns an aggregated report.
func (v *Validator) Validate(ctx context.Context) (*Report, error) {
	hostname, _ := os.Hostname()

	report := &Report{
		Timestamp: time.Now().UTC(),
		Hostname:  hostname,
		Compliant: true,
	}

	for _, checker := range v.checkers {
		if ctx.Err() != nil {
			return report, ctx.Err()
		}

		findings, err := checker.Check(ctx)
		if err != nil {
			report.Findings = append(report.Findings, Finding{
				Category: checker.Category(),
				Check:    checker.Name(),
				Severity: SeverityWarn,
				Message:  err.Error(),
			})
			report.WarnCount++
			continue
		}

		for _, f := range findings {
			report.Findings = append(report.Findings, f)
			switch f.Severity {
			case SeverityPass:
				report.PassCount++
			case SeverityWarn:
				report.WarnCount++
			case SeverityFail:
				report.FailCount++
				report.Compliant = false
			}
		}
	}

	return report, nil
}
