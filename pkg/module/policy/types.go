package policy

import "time"

// TrustLevel represents the trust level of a module
type TrustLevel string

const (
	TrustLevelUnknown   TrustLevel = "unknown"
	TrustLevelUntrusted TrustLevel = "untrusted"
	TrustLevelLow       TrustLevel = "low"
	TrustLevelCommunity TrustLevel = "community"
	TrustLevelMedium    TrustLevel = "medium"
	TrustLevelVerified  TrustLevel = "verified"
	TrustLevelHigh      TrustLevel = "high"
	TrustLevelInternal  TrustLevel = "internal"
	TrustLevelTrusted   TrustLevel = "trusted"
	TrustLevelSystem    TrustLevel = "system"
)

// ActionType represents the action to take when a policy rule matches
type ActionType string

const (
	ActionAllow  ActionType = "allow"
	ActionDeny   ActionType = "deny"
	ActionWarn   ActionType = "warn"
	ActionModify ActionType = "modify"
)

// Violation represents a policy violation
type Violation struct {
	PolicyID    string
	RuleID      string
	Message     string
	Severity    string
	Remediation string
}

// ModuleInfo contains basic module information
type ModuleInfo struct {
	Name    string
	Version string
}

// ModulePolicyContext provides context for policy evaluation
type ModulePolicyContext struct {
	Module       *ModuleInfo
	Version      string
	Capabilities []string
	TrustLevel   TrustLevel
	Environment  string
	Timestamp    time.Time
}

// ModulePolicyResult contains the result of policy evaluation
type ModulePolicyResult struct {
	Allowed             bool
	AllowedCapabilities []string
	DeniedCapabilities  []string
	Violations          []Violation
	Warnings            []string
	Reason              string
	EvaluationTime      time.Duration
}

// CapabilityPolicyConfig configures capability policies
type CapabilityPolicyConfig struct {
	AllowByDefault              bool
	DenyList                    []string
	AllowList                   []string
	BlockedCapabilities         []string
	RequireApprovalCapabilities []string
	TrustLevelRequirements      map[string]TrustLevel
	EnvironmentRestrictions     map[string][]string
}

// ModulePolicyEngine evaluates module policies
type ModulePolicyEngine struct {
	PolicyEngine     interface{}
	CapabilityConfig *CapabilityPolicyConfig
	Rules            []*ModulePolicyRule
	EnforcementMode  interface{} // policypkg.EnforcementMode
}

// PolicyCondition represents a condition that must be met for a rule to apply
type PolicyCondition struct {
	ModuleNamePattern    string
	MinTrustLevel        TrustLevel
	MaxTrustLevel        TrustLevel
	Environments         []string
	RequiredCapabilities []string
	ForbiddenCapabilities []string
}

// PolicyAction represents the action to take when a rule matches
type PolicyAction struct {
	Type              ActionType
	Block             bool
	BlockReason       string
	Warn              string
	AllowCapabilities []string
	DenyCapabilities  []string
}

// ModulePolicyRule represents a policy rule
type ModulePolicyRule struct {
	ID          string
	Name        string
	Description string
	Enabled     bool
	Priority    int
	Conditions  PolicyCondition
	Action      PolicyAction
}

// Registry holds policy rules
type Registry struct {
	// Implementation details
}

// NewRegistry creates a new policy registry
func NewRegistry() *Registry {
	return &Registry{}
}
