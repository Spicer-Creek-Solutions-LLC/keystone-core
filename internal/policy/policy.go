// Package policy is the v1.0 policy engine per PROJECT-DETAILS
// §4.12. It owns the policy data model (Policy, PolicySet, Binding),
// the in-memory Registry, the Evaluator seam (tasks 6-8 implement
// OPA / CEL / Builtin), and the Engine coordinator.
//
// The package depends on internal/audit for the shared value enums
// (Severity, EnforcementMode, PolicyType) and the Violation type.
// The dependency is one-way: audit is the lower-level recording
// primitive and never imports policy. A policy evaluation produces
// audit.Violation values that flow into the audit log unchanged
// (no translation layer — see Epic 12 task 4's emission hooks).
//
// v1.0 ships the engine in audit-mode-only: policies evaluate and
// record but the Enforcer (task 10) always allows. Full enforcement
// is post-v1.0.
package policy

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/audit"
)

// ErrInvalidPolicy is the family root for Policy / PolicySet /
// Binding structural rejections. Constructors + Validate wrap
// context with `fmt.Errorf("%w: ...", ErrInvalidPolicy)` so call
// sites match with errors.Is.
var ErrInvalidPolicy = errors.New("policy: invalid policy")

// Category is the §4.12 policy classification. Unlike audit's
// enums this one is policy-local — the audit log doesn't record a
// category (it records the policy ID; category is metadata for
// reporting + ComplianceReport framework mapping in task 11).
type Category string

const (
	// CategorySecurity — access control, privilege, secrets-handling
	// policies.
	CategorySecurity Category = "security"

	// CategoryCompliance — regulatory framework controls (SOC2,
	// NIST, etc.; task 11 maps these).
	CategoryCompliance Category = "compliance"

	// CategoryOperational — day-2 operational guardrails (change
	// windows, approval gates).
	CategoryOperational Category = "operational"

	// CategoryCost — spend / quota / resource-ceiling policies.
	CategoryCost Category = "cost"

	// CategoryCustom — operator-defined; no built-in semantics.
	CategoryCustom Category = "custom"
)

// String returns the underlying string.
func (c Category) String() string { return string(c) }

// IsKnown reports whether the receiver is one of the five v1.0
// categories. Empty string reports false.
func (c Category) IsKnown() bool {
	switch c {
	case CategorySecurity, CategoryCompliance, CategoryOperational,
		CategoryCost, CategoryCustom:
		return true
	}
	return false
}

// ParseCategory accepts the canonical lowercase names. Whitespace
// is trimmed and case folded. Empty / unknown is an error — every
// policy must declare a category (unlike audit.PolicyType where
// empty is the valid "non-policy entry" sentinel).
func ParseCategory(s string) (Category, error) {
	trimmed := strings.ToLower(strings.TrimSpace(s))
	if trimmed == "" {
		return "", fmt.Errorf("%w: category is empty", ErrInvalidPolicy)
	}
	c := Category(trimmed)
	if !c.IsKnown() {
		return "", fmt.Errorf("%w: unknown category %q (known: security, compliance, operational, cost, custom)", ErrInvalidPolicy, s)
	}
	return c, nil
}

// MarshalText implements encoding.TextMarshaler.
func (c Category) MarshalText() ([]byte, error) {
	if !c.IsKnown() {
		return nil, fmt.Errorf("%w: unknown category %q", ErrInvalidPolicy, string(c))
	}
	return []byte(c), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (c *Category) UnmarshalText(b []byte) error {
	parsed, err := ParseCategory(string(b))
	if err != nil {
		return err
	}
	*c = parsed
	return nil
}

// AllCategories returns the five v1.0 categories in declaration
// order. Fresh slice per call (callers may sort/filter).
func AllCategories() []Category {
	return []Category{
		CategorySecurity, CategoryCompliance, CategoryOperational,
		CategoryCost, CategoryCustom,
	}
}

// Policy is the v1.0 policy definition per §4.12. Type selects the
// evaluator; Code is the evaluator-specific source (Rego for OPA,
// a CEL expression for CEL, JSON config for Builtin). Severity is
// the policy's declared severity — the per-violation severity may
// differ (a single policy can yield violations of mixed severity).
//
// EnforcementMode is recorded on every evaluation's audit entry
// but the v1.0 Enforcer (task 10) ignores it (always allows); a later
// (v1.x) release honors it.
type Policy struct {
	ID              string
	Name            string
	Type            audit.PolicyType
	Category        Category
	Severity        audit.Severity
	EnforcementMode audit.EnforcementMode
	Code            string
	Enabled         bool
	Tags            []string
	Metadata        map[string]string
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// Validate enforces the structural invariants. A disabled policy
// is still validated — Enabled gates evaluation, not registration
// (operators register-then-toggle).
func (p *Policy) Validate() error {
	if p == nil {
		return fmt.Errorf("%w: nil policy", ErrInvalidPolicy)
	}
	if strings.TrimSpace(p.ID) == "" {
		return fmt.Errorf("%w: ID is required", ErrInvalidPolicy)
	}
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("%w: Name is required", ErrInvalidPolicy)
	}
	if !p.Type.IsKnown() {
		return fmt.Errorf("%w: Type %q is not a known evaluator (opa, cel, builtin)", ErrInvalidPolicy, p.Type)
	}
	if !p.Category.IsKnown() {
		return fmt.Errorf("%w: Category %q is not known", ErrInvalidPolicy, p.Category)
	}
	if !p.Severity.IsValid() {
		return fmt.Errorf("%w: Severity is not a valid level", ErrInvalidPolicy)
	}
	if !p.EnforcementMode.IsValid() {
		return fmt.Errorf("%w: EnforcementMode is not valid", ErrInvalidPolicy)
	}
	if strings.TrimSpace(p.Code) == "" {
		return fmt.Errorf("%w: Code is required (evaluator source)", ErrInvalidPolicy)
	}
	return nil
}

// Clone returns a deep copy so registry callers can't mutate
// stored policy state through a shared slice/map header.
func (p *Policy) Clone() *Policy {
	if p == nil {
		return nil
	}
	cp := *p
	if p.Tags != nil {
		cp.Tags = append([]string(nil), p.Tags...)
	}
	if p.Metadata != nil {
		cp.Metadata = make(map[string]string, len(p.Metadata))
		for k, v := range p.Metadata {
			cp.Metadata[k] = v
		}
	}
	return &cp
}
