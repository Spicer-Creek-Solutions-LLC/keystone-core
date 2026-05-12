package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// Detector observes drift across a topo-ordered declaration list. It
// is a thin overlay on Runner.Check (Task 6): Check fires per-decl
// state.drift events; the Detector adds per-decl severity
// classification and aggregate reporting so operators can answer
// "how bad is this and what should I look at first?"
//
// Detect runs the Check phase only — no Apply, no mutation. Auto-
// remediation (`kscorectl state drift --fix`) lives in the CLI
// (Task 10) and re-invokes Runner.Run on the drifted subset.
//
// PROJECT-DETAILS §4.8 suggests an `internal/statemgmt/drift/`
// subpackage; we kept it in this package because the detector
// reuses Runner internals + test fakes. The seam is severity
// policy (the SeverityResolver), not a self-contained subsystem.
type Detector struct {
	Registry         *Registry
	Observer         RunObserver
	SeverityResolver SeverityResolver
	DeclTimeout      time.Duration
}

// NewDetector returns a Detector. Pass nil for reg to fall back to
// DefaultRegistry and nil for obs to get a no-op observer. The
// SeverityResolver defaults to DefaultSeverityResolver.
func NewDetector(reg *Registry, obs RunObserver) *Detector {
	return &Detector{Registry: reg, Observer: obs}
}

// DriftSeverity classifies how bad one drifted declaration is.
// Used for aggregation and CLI prioritisation. The default for any
// unannotated drift is DriftSeverityMedium.
type DriftSeverity int

const (
	DriftSeverityNone DriftSeverity = iota
	DriftSeverityLow
	DriftSeverityMedium
	DriftSeverityHigh
	DriftSeverityCritical
)

func (s DriftSeverity) String() string {
	switch s {
	case DriftSeverityNone:
		return "none"
	case DriftSeverityLow:
		return "low"
	case DriftSeverityMedium:
		return "medium"
	case DriftSeverityHigh:
		return "high"
	case DriftSeverityCritical:
		return "critical"
	default:
		return fmt.Sprintf("DriftSeverity(%d)", int(s))
	}
}

// parseDriftSeverity accepts the five string forms recognised by the
// DSL Params["severity"] override. Returns (severity, ok); ok==false
// signals an unrecognised value, which the layered resolver treats
// as "no override" and falls through to module / default.
func parseDriftSeverity(s string) (DriftSeverity, bool) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "none":
		return DriftSeverityNone, true
	case "low":
		return DriftSeverityLow, true
	case "medium":
		return DriftSeverityMedium, true
	case "high":
		return DriftSeverityHigh, true
	case "critical":
		return DriftSeverityCritical, true
	default:
		return DriftSeverityNone, false
	}
}

// DriftState classifies what the detector observed for one
// declaration. Severity is only meaningful when State==Drifted.
type DriftState int

const (
	DriftStateInSync  DriftState = iota // Check matched
	DriftStateDrifted                    // Check showed drift
	DriftStateError                      // Check failed
	DriftStateSkipped                    // cascaded skip after earlier error
)

func (s DriftState) String() string {
	switch s {
	case DriftStateInSync:
		return "in-sync"
	case DriftStateDrifted:
		return "drifted"
	case DriftStateError:
		return "error"
	case DriftStateSkipped:
		return "skipped"
	default:
		return fmt.Sprintf("DriftState(%d)", int(s))
	}
}

// DriftStatus is the per-declaration drift record. Severity is
// DriftSeverityNone unless State == DriftStateDrifted. Diff carries
// Check's human-readable diff when present.
type DriftStatus struct {
	DeclID    string
	Module    string
	State     DriftState
	Severity  DriftSeverity
	Diff      string
	Error     error
	StartedAt time.Time
	Duration  time.Duration
}

// DriftReport is the aggregated output of one Detect call. Statuses
// are in execution order; counters give the CLI its summary without
// re-walking Statuses; AggregateSeverity is the highest severity
// across drifted declarations (or DriftSeverityNone if nothing
// drifted).
type DriftReport struct {
	StartedAt time.Time
	EndedAt   time.Time
	Statuses  []DriftStatus

	AggregateSeverity DriftSeverity

	TotalChecked int
	InSync       int
	Drifted      int
	Errors       int
	Skipped      int
}

// SeverityResolver picks a DriftSeverity for one drifted declaration.
// Implementations can inspect decl.Params, the module instance, or
// any external policy source (allow-lists, business hours, etc.).
type SeverityResolver interface {
	Resolve(decl *Declaration, mod Module, check *ModuleCheckResult) DriftSeverity
}

// DriftSeverityModule is the optional interface a Module implements
// to declare its own default drift severity. Modules without per-decl
// nuance can pin one severity here; finer-grained decisions live in
// a custom SeverityResolver.
type DriftSeverityModule interface {
	DriftSeverity(decl *Declaration, check *ModuleCheckResult) DriftSeverity
}

// ReservedSeverityParamKey is the Declaration.Params key the
// DefaultSeverityResolver reads for per-declaration overrides.
// "severity" is treated as engine-reserved — modules should not also
// claim it. A future Validator pass can enforce that (V1X candidate).
const ReservedSeverityParamKey = "severity"

// DefaultSeverityResolver applies the layered v1.0 policy:
//
//  1. decl.Params["severity"] = "low" | "medium" | "high" | "critical"
//     wins. Unrecognised values fall through (silently — keeps the
//     resolver decoupled from validation; Validator can layer on top).
//  2. mod.(DriftSeverityModule) is consulted next.
//  3. Falls back to DriftSeverityMedium — the "no information given"
//     pose.
var DefaultSeverityResolver SeverityResolver = defaultSeverityResolver{}

type defaultSeverityResolver struct{}

func (defaultSeverityResolver) Resolve(decl *Declaration, mod Module, check *ModuleCheckResult) DriftSeverity {
	if decl != nil {
		if raw, ok := decl.Params[ReservedSeverityParamKey]; ok {
			if str, isString := raw.(string); isString {
				if sev, parsed := parseDriftSeverity(str); parsed {
					return sev
				}
			}
		}
	}
	if dsm, ok := mod.(DriftSeverityModule); ok {
		return dsm.DriftSeverity(decl.moduleView(), check)
	}
	return DriftSeverityMedium
}

// Detect runs the Check phase on each declaration and returns a
// DriftReport. Does NOT apply. Severity is computed only for
// declarations whose Outcome was OutcomeDriftDetected.
func (d *Detector) Detect(ctx context.Context, decls []*Declaration) (*DriftReport, error) {
	runner := &Runner{
		Registry:    d.Registry,
		Observer:    d.Observer,
		DeclTimeout: d.DeclTimeout,
	}
	runReport, runErr := runner.Check(ctx, decls)

	resolver := d.SeverityResolver
	if resolver == nil {
		resolver = DefaultSeverityResolver
	}
	reg := runner.registry() // same fallback rules

	report := &DriftReport{
		StartedAt: runReport.StartedAt,
		EndedAt:   runReport.EndedAt,
	}

	for i, res := range runReport.Results {
		status := DriftStatus{
			DeclID:    res.DeclID,
			Module:    res.Module,
			Error:     res.Error,
			StartedAt: res.StartedAt,
			Duration:  res.Duration,
		}
		if res.Check != nil {
			status.Diff = res.Check.Diff
		}
		switch res.Outcome {
		case OutcomeUnchanged:
			status.State = DriftStateInSync
			report.InSync++
		case OutcomeDriftDetected:
			status.State = DriftStateDrifted
			status.Severity = resolveSeverity(reg, resolver, decls, i, res.Check)
			report.Drifted++
			if status.Severity > report.AggregateSeverity {
				report.AggregateSeverity = status.Severity
			}
		case OutcomeFailed:
			status.State = DriftStateError
			report.Errors++
		case OutcomeSkipped:
			status.State = DriftStateSkipped
			report.Skipped++
		default:
			// Check mode never produces Changed / NoOp; this branch
			// is a defensive guard. Treat as Error so an unexpected
			// outcome is surfaced rather than silently in-sync.
			status.State = DriftStateError
			if status.Error == nil {
				status.Error = fmt.Errorf("unexpected Outcome %v from Runner.Check", res.Outcome)
			}
			report.Errors++
		}
		report.Statuses = append(report.Statuses, status)
	}

	report.TotalChecked = len(report.Statuses)
	return report, runErr
}

// resolveSeverity looks up the module via the registry (so the
// resolver can inspect the module instance) and asks the resolver.
// A registry lookup failure falls back to the resolver with mod=nil
// so per-decl overrides still take effect.
func resolveSeverity(reg *Registry, resolver SeverityResolver, decls []*Declaration, i int, check *ModuleCheckResult) DriftSeverity {
	if i < 0 || i >= len(decls) {
		return DriftSeverityMedium
	}
	decl := decls[i]
	if decl == nil {
		return DriftSeverityMedium
	}
	// Validator should have caught an unknown module, but we
	// tolerate one slipping through to keep the report shape
	// intact. mod=nil still lets Params["severity"] take effect via
	// the resolver's first layer.
	mod, _ := reg.Get(decl.Module)
	return resolver.Resolve(decl, mod, check)
}
