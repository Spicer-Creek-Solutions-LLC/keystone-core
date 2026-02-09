package policy

import (
	"fmt"
	"path/filepath"
	"sort"
	"time"

	policypkg "github.com/shawnbutts/keystone-core/internal/policy"
)

// NewModulePolicyEngine creates a new module policy engine
func NewModulePolicyEngine(policyEngine *policypkg.PolicyEngine, config *CapabilityPolicyConfig) *ModulePolicyEngine {
	if config == nil {
		config = DefaultCapabilityPolicyConfig()
	}

	return &ModulePolicyEngine{
		PolicyEngine:     policyEngine,
		CapabilityConfig: config,
		Rules:            make([]*ModulePolicyRule, 0),
		EnforcementMode:  policypkg.ModeEnforce,
	}
}

// DefaultCapabilityPolicyConfig returns the default capability configuration
func DefaultCapabilityPolicyConfig() *CapabilityPolicyConfig {
	return &CapabilityPolicyConfig{
		AllowByDefault:      false,
		BlockedCapabilities: []string{
			// Never allow these by default
		},
		RequireApprovalCapabilities: []string{
			"exec",          // Command execution
			"http.post",     // Outbound HTTP POST
			"secrets.write", // Secret modification
		},
		TrustLevelRequirements: map[string]TrustLevel{
			"exec":          TrustLevelVerified,
			"http.post":     TrustLevelCommunity,
			"secrets.write": TrustLevelVerified,
			"secrets.read":  TrustLevelCommunity,
		},
		EnvironmentRestrictions: map[string][]string{
			"prod": {"exec", "secrets.write"},
		},
	}
}

// ValidateModule validates a module against all policies
func (e *ModulePolicyEngine) ValidateModule(ctx *ModulePolicyContext) (*ModulePolicyResult, error) {
	startTime := time.Now()

	result := &ModulePolicyResult{
		Allowed:             true,
		AllowedCapabilities: make([]string, 0),
		DeniedCapabilities:  make([]string, 0),
		Warnings:            make([]string, 0),
		Violations:          make([]Violation, 0),
	}

	// Check custom rules first
	if err := e.applyRules(ctx, result); err != nil {
		return nil, err
	}

	// If blocked by rules, return early
	if !result.Allowed {
		result.EvaluationTime = time.Since(startTime)
		return result, nil
	}

	// Validate capabilities
	capResult, err := e.ValidateCapabilities(ctx, ctx.Capabilities)
	if err != nil {
		return nil, err
	}

	// Merge capability results
	result.AllowedCapabilities = capResult.AllowedCapabilities
	result.DeniedCapabilities = capResult.DeniedCapabilities
	result.Warnings = append(result.Warnings, capResult.Warnings...)
	result.Violations = append(result.Violations, capResult.Violations...)

	// If any capability is denied, module may still load but with restricted capabilities
	if len(result.DeniedCapabilities) > 0 {
		if e.EnforcementMode == policypkg.ModeEnforce {
			result.Allowed = false
			result.Reason = fmt.Sprintf("denied capabilities: %v", result.DeniedCapabilities)
		} else {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("denied capabilities (audit mode): %v", result.DeniedCapabilities))
		}
	}

	// Set success reason if allowed
	if result.Allowed && result.Reason == "" {
		result.Reason = "all policy checks passed"
	}

	result.EvaluationTime = time.Since(startTime)
	return result, nil
}

// ValidateCapability validates a single capability
func (e *ModulePolicyEngine) ValidateCapability(ctx *ModulePolicyContext, capability string) (bool, error) {
	// Check if blocked
	for _, blocked := range e.CapabilityConfig.BlockedCapabilities {
		if capability == blocked {
			return false, nil
		}
	}

	// Check trust level requirements
	if requiredTrust, exists := e.CapabilityConfig.TrustLevelRequirements[capability]; exists {
		if !meetsMinimumTrust(ctx.TrustLevel, requiredTrust) {
			return false, nil
		}
	}

	// Check environment restrictions
	if restricted, exists := e.CapabilityConfig.EnvironmentRestrictions[ctx.Environment]; exists {
		for _, cap := range restricted {
			if capability == cap {
				return false, nil
			}
		}
	}

	// Default to config setting
	return e.CapabilityConfig.AllowByDefault, nil
}

// ValidateCapabilities validates multiple capabilities
func (e *ModulePolicyEngine) ValidateCapabilities(ctx *ModulePolicyContext, caps []string) (*ModulePolicyResult, error) {
	result := &ModulePolicyResult{
		Allowed:             true,
		AllowedCapabilities: make([]string, 0),
		DeniedCapabilities:  make([]string, 0),
		Warnings:            make([]string, 0),
		Violations:          make([]Violation, 0),
	}

	for _, cap := range caps {
		allowed, err := e.ValidateCapability(ctx, cap)
		if err != nil {
			return nil, err
		}

		if allowed {
			result.AllowedCapabilities = append(result.AllowedCapabilities, cap)
		} else {
			result.DeniedCapabilities = append(result.DeniedCapabilities, cap)

			// Add violation
			result.Violations = append(result.Violations, Violation{
				PolicyID:    "capability-policy",
				RuleID:      "capability-denied",
				Message:     fmt.Sprintf("capability %s denied by policy", cap),
				Severity:    "medium",
				Remediation: fmt.Sprintf("increase module trust level or remove %s capability", cap),
			})
		}

		// Check if requires approval
		for _, reqApproval := range e.CapabilityConfig.RequireApprovalCapabilities {
			if cap == reqApproval {
				result.Warnings = append(result.Warnings,
					fmt.Sprintf("capability %s requires manual approval", cap))
			}
		}
	}

	return result, nil
}

// AddRule adds a custom policy rule
func (e *ModulePolicyEngine) AddRule(rule *ModulePolicyRule) {
	e.Rules = append(e.Rules, rule)

	// Sort by priority (higher first)
	sort.Slice(e.Rules, func(i, j int) bool {
		return e.Rules[i].Priority > e.Rules[j].Priority
	})
}

// RemoveRule removes a policy rule by ID
func (e *ModulePolicyEngine) RemoveRule(ruleID string) {
	filtered := make([]*ModulePolicyRule, 0)
	for _, rule := range e.Rules {
		if rule.ID != ruleID {
			filtered = append(filtered, rule)
		}
	}
	e.Rules = filtered
}

// applyRules applies custom rules to the module
func (e *ModulePolicyEngine) applyRules(ctx *ModulePolicyContext, result *ModulePolicyResult) error {
	for _, rule := range e.Rules {
		if !rule.Enabled {
			continue
		}

		if matches := e.ruleMatches(rule, ctx); matches {
			e.applyRuleAction(rule, result)

			// If rule blocks, stop processing
			if !result.Allowed {
				return nil
			}
		}
	}

	return nil
}

// ruleMatches checks if a rule's conditions match the context
func (e *ModulePolicyEngine) ruleMatches(rule *ModulePolicyRule, ctx *ModulePolicyContext) bool {
	cond := rule.Conditions

	// Check module name pattern
	if cond.ModuleNamePattern != "" {
		matched, _ := filepath.Match(cond.ModuleNamePattern, ctx.Module.Name)
		if !matched {
			return false
		}
	}

	// Check trust level
	if cond.MinTrustLevel != "" {
		if !meetsMinimumTrust(ctx.TrustLevel, cond.MinTrustLevel) {
			return false
		}
	}

	if cond.MaxTrustLevel != "" {
		if !meetsMaximumTrust(ctx.TrustLevel, cond.MaxTrustLevel) {
			return false
		}
	}

	// Check environment
	if len(cond.Environments) > 0 {
		found := false
		for _, env := range cond.Environments {
			if env == ctx.Environment {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}

	// Check required capabilities
	if len(cond.RequiredCapabilities) > 0 {
		for _, reqCap := range cond.RequiredCapabilities {
			found := false
			for _, cap := range ctx.Capabilities {
				if cap == reqCap {
					found = true
					break
				}
			}
			if !found {
				return false
			}
		}
	}

	// Check forbidden capabilities
	if len(cond.ForbiddenCapabilities) > 0 {
		for _, forbiddenCap := range cond.ForbiddenCapabilities {
			for _, cap := range ctx.Capabilities {
				if cap == forbiddenCap {
					return false
				}
			}
		}
	}

	return true
}

// applyRuleAction applies a rule's action to the result
func (e *ModulePolicyEngine) applyRuleAction(rule *ModulePolicyRule, result *ModulePolicyResult) {
	action := rule.Action

	switch action.Type {
	case ActionDeny:
		result.Allowed = false
		result.Reason = action.BlockReason
		if result.Reason == "" {
			result.Reason = fmt.Sprintf("denied by rule: %s", rule.Name)
		}

	case ActionWarn:
		if action.Warn != "" {
			result.Warnings = append(result.Warnings, action.Warn)
		}

	case ActionModify:
		// Add allowed capabilities
		for _, cap := range action.AllowCapabilities {
			if !contains(result.AllowedCapabilities, cap) {
				result.AllowedCapabilities = append(result.AllowedCapabilities, cap)
			}
		}

		// Add denied capabilities
		for _, cap := range action.DenyCapabilities {
			if !contains(result.DeniedCapabilities, cap) {
				result.DeniedCapabilities = append(result.DeniedCapabilities, cap)
			}
		}
	default:
	}

	if action.Block {
		result.Allowed = false
		if action.BlockReason != "" {
			result.Reason = action.BlockReason
		} else {
			result.Reason = fmt.Sprintf("blocked by rule: %s", rule.Name)
		}
	}
}

// meetsMinimumTrust checks if actual trust level meets minimum requirement
func meetsMinimumTrust(actual, minimum TrustLevel) bool {
	trustOrder := map[TrustLevel]int{
		TrustLevelUnknown:   0,
		TrustLevelUntrusted: 1,
		TrustLevelCommunity: 2,
		TrustLevelVerified:  3,
		TrustLevelInternal:  4,
		TrustLevelSystem:    5,
	}

	actualLevel := trustOrder[actual]
	minimumLevel := trustOrder[minimum]

	return actualLevel >= minimumLevel
}

// meetsMaximumTrust checks if actual trust level is at or below maximum
func meetsMaximumTrust(actual, maximum TrustLevel) bool {
	trustOrder := map[TrustLevel]int{
		TrustLevelUnknown:   0,
		TrustLevelUntrusted: 1,
		TrustLevelCommunity: 2,
		TrustLevelVerified:  3,
		TrustLevelInternal:  4,
		TrustLevelSystem:    5,
	}

	actualLevel := trustOrder[actual]
	maximumLevel := trustOrder[maximum]

	return actualLevel <= maximumLevel
}

// contains checks if a slice contains a string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}
