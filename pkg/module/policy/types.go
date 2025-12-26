package policy

import (
	"time"

	"github.com/titananvil/titan-anvil/pkg/module/capabilities"
	"github.com/titananvil/titan-anvil/pkg/module/manifest"
	policypkg "github.com/titananvil/titan-anvil/pkg/policy"
)

// ModulePolicyContext provides context for policy evaluation
type ModulePolicyContext struct {
	// Module is the module manifest
	Module *manifest.Manifest

	// Capabilities are the requested capabilities
	Capabilities []string

	// TrustLevel is the trust level of the module source
	TrustLevel TrustLevel

	// Environment is the deployment environment (dev, staging, prod)
	Environment string

	// User is the user loading the module
	User string

	// Timestamp is when the policy is being evaluated
	Timestamp time.Time
}

// TrustLevel represents the trust level of a module
type TrustLevel string

const (
	// TrustLevelUnknown means the module source is unknown
	TrustLevelUnknown TrustLevel = "unknown"

	// TrustLevelUntrusted means the module is from an untrusted source
	TrustLevelUntrusted TrustLevel = "untrusted"

	// TrustLevelCommunity means the module is from the community registry
	TrustLevelCommunity TrustLevel = "community"

	// TrustLevelVerified means the module signature is verified
	TrustLevelVerified TrustLevel = "verified"

	// TrustLevelInternal means the module is from internal sources
	TrustLevelInternal TrustLevel = "internal"

	// TrustLevelSystem means the module is a system module
	TrustLevelSystem TrustLevel = "system"
)

// ModulePolicyResult represents the result of policy evaluation
type ModulePolicyResult struct {
	// Allowed indicates if the module is allowed to load
	Allowed bool

	// AllowedCapabilities are the capabilities that are allowed
	AllowedCapabilities []string

	// DeniedCapabilities are the capabilities that are denied
	DeniedCapabilities []string

	// Warnings are any policy warnings
	Warnings []string

	// Violations are any policy violations
	Violations []policypkg.Violation

	// Reason explains why the module was allowed or denied
	Reason string

	// EvaluationTime is how long policy evaluation took
	EvaluationTime time.Duration
}

// ModulePolicyValidator validates modules against policies
type ModulePolicyValidator interface {
	// ValidateModule validates a module against policies
	ValidateModule(ctx *ModulePolicyContext) (*ModulePolicyResult, error)

	// ValidateCapability validates a single capability
	ValidateCapability(ctx *ModulePolicyContext, capability string) (bool, error)

	// ValidateCapabilities validates multiple capabilities
	ValidateCapabilities(ctx *ModulePolicyContext, caps []string) (*ModulePolicyResult, error)
}

// CapabilityPolicyConfig configures capability-based policies
type CapabilityPolicyConfig struct {
	// AllowByDefault determines if capabilities are allowed by default
	AllowByDefault bool

	// BlockedCapabilities are always blocked
	BlockedCapabilities []string

	// RequireApprovalCapabilities require manual approval
	RequireApprovalCapabilities []string

	// TrustLevelRequirements maps capabilities to minimum trust levels
	TrustLevelRequirements map[string]TrustLevel

	// EnvironmentRestrictions restricts capabilities by environment
	// e.g., "prod" -> ["exec", "http.post"] means exec and http.post are blocked in prod
	EnvironmentRestrictions map[string][]string
}

// ModulePolicyRule represents a policy rule for modules
type ModulePolicyRule struct {
	// ID is the unique rule identifier
	ID string

	// Name is a human-readable name
	Name string

	// Description explains what the rule does
	Description string

	// Enabled indicates if the rule is active
	Enabled bool

	// Conditions are the conditions under which the rule applies
	Conditions RuleConditions

	// Action is what happens when the rule matches
	Action RuleAction

	// Priority determines rule evaluation order (higher = earlier)
	Priority int
}

// RuleConditions define when a rule applies
type RuleConditions struct {
	// ModuleNamePattern is a glob pattern for module names
	ModuleNamePattern string

	// MinTrustLevel is the minimum required trust level
	MinTrustLevel TrustLevel

	// MaxTrustLevel is the maximum allowed trust level (optional)
	MaxTrustLevel TrustLevel

	// Environments are the environments where this rule applies
	Environments []string

	// RequiredCapabilities are capabilities that must be present
	RequiredCapabilities []string

	// ForbiddenCapabilities are capabilities that must not be present
	ForbiddenCapabilities []string
}

// RuleAction defines what happens when a rule matches
type RuleAction struct {
	// Type is the action type
	Type ActionType

	// AllowCapabilities are capabilities to allow
	AllowCapabilities []string

	// DenyCapabilities are capabilities to deny
	DenyCapabilities []string

	// Warn generates a warning
	Warn string

	// Block prevents module loading
	Block bool

	// BlockReason explains why the module is blocked
	BlockReason string
}

// ActionType represents the type of action to take
type ActionType string

const (
	// ActionAllow allows the module
	ActionAllow ActionType = "allow"

	// ActionDeny denies the module
	ActionDeny ActionType = "deny"

	// ActionWarn generates a warning
	ActionWarn ActionType = "warn"

	// ActionModify modifies allowed capabilities
	ActionModify ActionType = "modify"
)

// ModulePolicyEngine coordinates policy evaluation for modules
type ModulePolicyEngine struct {
	// PolicyEngine is the underlying policy engine
	PolicyEngine *policypkg.PolicyEngine

	// CapabilityConfig is the capability policy configuration
	CapabilityConfig *CapabilityPolicyConfig

	// Rules are custom module policy rules
	Rules []*ModulePolicyRule

	// EnforcementMode determines how policies are enforced
	EnforcementMode policypkg.EnforcementMode
}

// CapabilityValidator validates capabilities against policies
type CapabilityValidator struct {
	// Config is the capability policy configuration
	Config *CapabilityPolicyConfig

	// CapabilityRegistry is the capability registry
	CapabilityRegistry *capabilities.CapabilityRegistry
}

// LoadTimePolicy represents a policy that runs when a module is loaded
type LoadTimePolicy struct {
	// Check is called before a module is loaded
	Check func(ctx *ModulePolicyContext) (*ModulePolicyResult, error)
}

// RuntimePolicy represents a policy that runs during module execution
type RuntimePolicy struct {
	// Check is called during module execution
	Check func(ctx *ModulePolicyContext, operation string) (*ModulePolicyResult, error)
}
