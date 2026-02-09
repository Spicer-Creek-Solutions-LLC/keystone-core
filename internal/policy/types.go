package policy

import "time"

// Type represents the type of policy language
type Type string

const (
	// TypeOPA uses Open Policy Agent (Rego)
	TypeOPA Type = "opa"
	// TypeCEL uses Common Expression Language
	TypeCEL Type = "cel"
	// TypeBuiltin uses built-in policy rules
	TypeBuiltin Type = "builtin"
)

// PolicyType is deprecated: Use Type instead.
type PolicyType = Type //nolint:revive // Deprecated alias for backward compatibility

// PolicyTypeOPA is deprecated: Use TypeOPA instead.
const PolicyTypeOPA = TypeOPA

// PolicyTypeCEL is deprecated: Use TypeCEL instead.
const PolicyTypeCEL = TypeCEL

// PolicyTypeBuiltin is deprecated: Use TypeBuiltin instead.
const PolicyTypeBuiltin = TypeBuiltin

// Category represents the category of policy
type Category string

// PolicyCategory is deprecated: Use Category instead.
type PolicyCategory = Category //nolint:revive // Deprecated alias for backward compatibility

const (
	// CategorySecurity security-related policies
	CategorySecurity Category = "security"
	// CategoryCompliance compliance-related policies
	CategoryCompliance Category = "compliance"
	// CategoryOperational operational policies
	CategoryOperational Category = "operational"
	// CategoryCost cost-related policies
	CategoryCost Category = "cost"
	// CategoryCustom custom policies
	CategoryCustom Category = "custom"
)

// Severity represents policy violation severity
type Severity string

const (
	// SeverityLow low severity
	SeverityLow Severity = "low"
	// SeverityMedium medium severity
	SeverityMedium Severity = "medium"
	// SeverityHigh high severity
	SeverityHigh Severity = "high"
	// SeverityCritical critical severity
	SeverityCritical Severity = "critical"
)

// EnforcementMode defines how policy violations are handled
type EnforcementMode string

const (
	// ModeAudit only log violations, don't block
	ModeAudit EnforcementMode = "audit"
	// ModeEnforce block operations that violate policy
	ModeEnforce EnforcementMode = "enforce"
	// ModeWarn warn but allow operations
	ModeWarn EnforcementMode = "warn"
)

// Policy represents a policy definition
type Policy struct {
	// ID is the unique policy identifier
	ID string `json:"id"`

	// Name is the human-readable name
	Name string `json:"name"`

	// Description describes what the policy enforces
	Description string `json:"description"`

	// Type is the policy language type
	Type PolicyType `json:"type"`

	// Category is the policy category
	Category PolicyCategory `json:"category"`

	// Severity of violations
	Severity Severity `json:"severity"`

	// EnforcementMode defines enforcement behavior
	EnforcementMode EnforcementMode `json:"enforcement_mode"`

	// Policy is the policy code (Rego, CEL, etc.)
	Policy string `json:"policy"`

	// Enabled indicates if policy is active
	Enabled bool `json:"enabled"`

	// Tags for organizing policies
	Tags []string `json:"tags,omitempty"`

	// Metadata for additional information
	Metadata map[string]string `json:"metadata,omitempty"`

	// CreatedAt timestamp
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// EvaluationInput represents input data for policy evaluation
type EvaluationInput struct {
	// Resource being evaluated
	Resource interface{} `json:"resource"`

	// Action being performed
	Action string `json:"action"`

	// User performing the action
	User string `json:"user,omitempty"`

	// Context for evaluation
	Context map[string]interface{} `json:"context,omitempty"`

	// Timestamp of evaluation
	Timestamp time.Time `json:"timestamp"`
}

// EvaluationResult represents the result of policy evaluation
type EvaluationResult struct {
	// PolicyID that was evaluated
	PolicyID string `json:"policy_id"`

	// PolicyName for reference
	PolicyName string `json:"policy_name"`

	// Allowed indicates if the action is allowed
	Allowed bool `json:"allowed"`

	// Violations contains policy violations
	Violations []Violation `json:"violations,omitempty"`

	// Warnings contains policy warnings
	Warnings []string `json:"warnings,omitempty"`

	// Message contains evaluation details
	Message string `json:"message,omitempty"`

	// Metadata from evaluation
	Metadata map[string]interface{} `json:"metadata,omitempty"`

	// Duration of evaluation
	Duration time.Duration `json:"duration"`

	// EvaluatedAt timestamp
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// Violation represents a policy violation
type Violation struct {
	// Rule that was violated
	Rule string `json:"rule"`

	// Message describing the violation
	Message string `json:"message"`

	// Severity of the violation
	Severity Severity `json:"severity"`

	// Path to the violating element
	Path string `json:"path,omitempty"`

	// Expected value
	Expected interface{} `json:"expected,omitempty"`

	// Actual value
	Actual interface{} `json:"actual,omitempty"`

	// Remediation suggestion
	Remediation string `json:"remediation,omitempty"`
}

// Set represents a collection of policies
type Set struct {
	// ID of the policy set
	ID string `json:"id"`

	// Name of the policy set
	Name string `json:"name"`

	// Description of the policy set
	Description string `json:"description"`

	// Policies in this set
	Policies []string `json:"policies"`

	// EnforcementMode for the entire set
	EnforcementMode EnforcementMode `json:"enforcement_mode"`

	// Enabled indicates if the set is active
	Enabled bool `json:"enabled"`

	// CreatedAt timestamp
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// Result combines multiple evaluation results
type Result struct {
	// Allowed indicates overall result
	Allowed bool `json:"allowed"`

	// Results from individual policies
	Results []*EvaluationResult `json:"results"`

	// Summary statistics
	Summary *Summary `json:"summary"`

	// TotalDuration for all evaluations
	TotalDuration time.Duration `json:"total_duration"`

	// EvaluatedAt timestamp
	EvaluatedAt time.Time `json:"evaluated_at"`
}

// Summary provides summary statistics
type Summary struct {
	// TotalPolicies evaluated
	TotalPolicies int `json:"total_policies"`

	// AllowedPolicies count
	AllowedPolicies int `json:"allowed_policies"`

	// DeniedPolicies count
	DeniedPolicies int `json:"denied_policies"`

	// TotalViolations across all policies
	TotalViolations int `json:"total_violations"`

	// ViolationsBySeverity breakdown
	ViolationsBySeverity map[Severity]int `json:"violations_by_severity"`
}

// Binding binds policies to resources
type Binding struct {
	// ID of the binding
	ID string `json:"id"`

	// PolicySetID or individual PolicyID
	PolicySetID string `json:"policy_set_id,omitempty"`
	PolicyID    string `json:"policy_id,omitempty"`

	// ResourceType this binding applies to
	ResourceType string `json:"resource_type"`

	// ResourceSelector for filtering resources
	ResourceSelector map[string]string `json:"resource_selector,omitempty"`

	// Actions this binding applies to
	Actions []string `json:"actions,omitempty"`

	// Enabled indicates if binding is active
	Enabled bool `json:"enabled"`

	// CreatedAt timestamp
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt timestamp
	UpdatedAt time.Time `json:"updated_at"`
}

// Deprecated aliases for backward compatibility
//
//nolint:revive,staticcheck // stuttering names kept for backward compatibility
type (
	// PolicySet is deprecated: Use Set instead.
	PolicySet = Set
	// PolicyResult is deprecated: Use Result instead.
	PolicyResult = Result
	// PolicySummary is deprecated: Use Summary instead.
	PolicySummary = Summary
	// PolicyBinding is deprecated: Use Binding instead.
	PolicyBinding = Binding
)
