package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// BuiltinPolicyName defines the name of a built-in policy
type BuiltinPolicyName string

const (
	// BuiltinRequireLabels requires specific labels on resources
	BuiltinRequireLabels BuiltinPolicyName = "require-labels"

	// BuiltinRequireOwner requires owner annotation on resources
	BuiltinRequireOwner BuiltinPolicyName = "require-owner"

	// BuiltinAllowedEnvironments restricts to specific environments
	BuiltinAllowedEnvironments BuiltinPolicyName = "allowed-environments"

	// BuiltinAllowedActions restricts to specific actions
	BuiltinAllowedActions BuiltinPolicyName = "allowed-actions"

	// BuiltinDenyPrivileged blocks privileged execution
	BuiltinDenyPrivileged BuiltinPolicyName = "deny-privileged"

	// BuiltinAllowedUsers restricts to specific users
	BuiltinAllowedUsers BuiltinPolicyName = "allowed-users"

	// BuiltinDeniedUsers blocks specific users
	BuiltinDeniedUsers BuiltinPolicyName = "denied-users"

	// BuiltinTimeWindow restricts operations to time windows
	BuiltinTimeWindow BuiltinPolicyName = "time-window"

	// BuiltinNoRootExecution blocks running as root
	BuiltinNoRootExecution BuiltinPolicyName = "no-root-execution"

	// BuiltinRequireApproval requires approval for actions
	BuiltinRequireApproval BuiltinPolicyName = "require-approval"

	// BuiltinMaxConcurrent limits concurrent operations
	BuiltinMaxConcurrent BuiltinPolicyName = "max-concurrent"

	// BuiltinResourceQuota enforces resource quotas
	BuiltinResourceQuota BuiltinPolicyName = "resource-quota"

	// BuiltinPatternDeny blocks resources matching patterns
	BuiltinPatternDeny BuiltinPolicyName = "pattern-deny"

	// BuiltinPatternAllow only allows resources matching patterns
	BuiltinPatternAllow BuiltinPolicyName = "pattern-allow"
)

// BuiltinPolicyConfig is the base configuration for built-in policies
type BuiltinPolicyConfig struct {
	// Name is the built-in policy name
	Name BuiltinPolicyName `json:"name"`

	// Config contains the policy-specific configuration
	Config json.RawMessage `json:"config"`
}

// RequireLabelsConfig configures the require-labels policy
type RequireLabelsConfig struct {
	// Labels that must be present
	Labels []string `json:"labels"`

	// LabelPatterns regex patterns for label keys
	LabelPatterns []string `json:"label_patterns,omitempty"`

	// RequireValues if true, labels must have non-empty values
	RequireValues bool `json:"require_values,omitempty"`
}

// RequireOwnerConfig configures the require-owner policy
type RequireOwnerConfig struct {
	// OwnerLabel is the label key for owner
	OwnerLabel string `json:"owner_label"`

	// TeamLabel is the label key for team
	TeamLabel string `json:"team_label,omitempty"`

	// ValidOwners is optional list of valid owner values
	ValidOwners []string `json:"valid_owners,omitempty"`

	// ValidTeams is optional list of valid team values
	ValidTeams []string `json:"valid_teams,omitempty"`
}

// AllowedEnvironmentsConfig configures the allowed-environments policy
type AllowedEnvironmentsConfig struct {
	// Environments list of allowed environments
	Environments []string `json:"environments"`

	// EnvironmentKey is the context key for environment (default: "environment")
	EnvironmentKey string `json:"environment_key,omitempty"`
}

// AllowedActionsConfig configures the allowed-actions policy
type AllowedActionsConfig struct {
	// Actions list of allowed actions
	Actions []string `json:"actions"`

	// ActionPatterns regex patterns for allowed actions
	ActionPatterns []string `json:"action_patterns,omitempty"`
}

// AllowedUsersConfig configures the allowed-users and denied-users policies
type AllowedUsersConfig struct {
	// Users list of usernames
	Users []string `json:"users"`

	// UserPatterns regex patterns for usernames
	UserPatterns []string `json:"user_patterns,omitempty"`

	// Groups list of groups
	Groups []string `json:"groups,omitempty"`
}

// TimeWindowConfig configures the time-window policy
type TimeWindowConfig struct {
	// AllowedDays days when operation is allowed (0=Sunday, 6=Saturday)
	AllowedDays []int `json:"allowed_days,omitempty"`

	// AllowedHoursStart start hour (0-23)
	AllowedHoursStart int `json:"allowed_hours_start,omitempty"`

	// AllowedHoursEnd end hour (0-23)
	AllowedHoursEnd int `json:"allowed_hours_end,omitempty"`

	// Timezone for time calculations (default: UTC)
	Timezone string `json:"timezone,omitempty"`

	// BlockedDates specific dates to block (YYYY-MM-DD format)
	BlockedDates []string `json:"blocked_dates,omitempty"`
}

// NoRootExecutionConfig configures the no-root-execution policy
type NoRootExecutionConfig struct {
	// AllowedUsers users allowed to run as root
	AllowedUsers []string `json:"allowed_users,omitempty"`

	// AllowedActions actions allowed as root
	AllowedActions []string `json:"allowed_actions,omitempty"`
}

// RequireApprovalConfig configures the require-approval policy
type RequireApprovalConfig struct {
	// Actions that require approval
	Actions []string `json:"actions"`

	// Approvers list of users who can approve
	Approvers []string `json:"approvers,omitempty"`

	// MinApprovals minimum number of approvals required
	MinApprovals int `json:"min_approvals,omitempty"`
}

// MaxConcurrentConfig configures the max-concurrent policy
type MaxConcurrentConfig struct {
	// MaxConcurrent maximum concurrent operations
	MaxConcurrent int `json:"max_concurrent"`

	// Scope for counting (global, user, resource)
	Scope string `json:"scope,omitempty"`
}

// ResourceQuotaConfig configures the resource-quota policy
type ResourceQuotaConfig struct {
	// MaxResources maximum number of resources
	MaxResources int `json:"max_resources,omitempty"`

	// MaxPerUser maximum resources per user
	MaxPerUser int `json:"max_per_user,omitempty"`

	// MaxPerTeam maximum resources per team
	MaxPerTeam int `json:"max_per_team,omitempty"`
}

// PatternConfig configures pattern-deny and pattern-allow policies
type PatternConfig struct {
	// Patterns regex patterns to match
	Patterns []string `json:"patterns"`

	// Field the field to match against (default: "name")
	Field string `json:"field,omitempty"`
}

// BuiltinEvaluator evaluates built-in policies
type BuiltinEvaluator struct {
	// handlers maps policy names to handler functions
	handlers map[BuiltinPolicyName]builtinHandler
}

// builtinHandler is a function that evaluates a built-in policy
type builtinHandler func(ctx context.Context, config json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error)

// NewBuiltinEvaluator creates a new built-in policy evaluator
func NewBuiltinEvaluator() *BuiltinEvaluator {
	e := &BuiltinEvaluator{
		handlers: make(map[BuiltinPolicyName]builtinHandler),
	}

	// Register handlers
	e.handlers[BuiltinRequireLabels] = e.evaluateRequireLabels
	e.handlers[BuiltinRequireOwner] = e.evaluateRequireOwner
	e.handlers[BuiltinAllowedEnvironments] = e.evaluateAllowedEnvironments
	e.handlers[BuiltinAllowedActions] = e.evaluateAllowedActions
	e.handlers[BuiltinDenyPrivileged] = e.evaluateDenyPrivileged
	e.handlers[BuiltinAllowedUsers] = e.evaluateAllowedUsers
	e.handlers[BuiltinDeniedUsers] = e.evaluateDeniedUsers
	e.handlers[BuiltinTimeWindow] = e.evaluateTimeWindow
	e.handlers[BuiltinNoRootExecution] = e.evaluateNoRootExecution
	e.handlers[BuiltinRequireApproval] = e.evaluateRequireApproval
	e.handlers[BuiltinMaxConcurrent] = e.evaluateMaxConcurrent
	e.handlers[BuiltinResourceQuota] = e.evaluateResourceQuota
	e.handlers[BuiltinPatternDeny] = e.evaluatePatternDeny
	e.handlers[BuiltinPatternAllow] = e.evaluatePatternAllow

	return e
}

// Evaluate evaluates a built-in policy
func (e *BuiltinEvaluator) Evaluate(ctx context.Context, policy *Policy, input *EvaluationInput) (*EvaluationResult, error) {
	start := time.Now()

	// Parse the policy configuration
	var cfg BuiltinPolicyConfig
	if err := json.Unmarshal([]byte(policy.Policy), &cfg); err != nil {
		return nil, fmt.Errorf("invalid built-in policy configuration: %w", err)
	}

	// Find the handler
	handler, ok := e.handlers[cfg.Name]
	if !ok {
		return nil, fmt.Errorf("unknown built-in policy: %s", cfg.Name)
	}

	// Execute the handler
	result, err := handler(ctx, cfg.Config, input, policy.Severity)
	if err != nil {
		return nil, err
	}

	// Fill in common fields
	result.PolicyID = policy.ID
	result.PolicyName = policy.Name
	result.Duration = time.Since(start)
	result.EvaluatedAt = time.Now()

	return result, nil
}

// ValidatePolicy validates a built-in policy configuration
func (e *BuiltinEvaluator) ValidatePolicy(ctx context.Context, policyCode string) error {
	var cfg BuiltinPolicyConfig
	if err := json.Unmarshal([]byte(policyCode), &cfg); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	if cfg.Name == "" {
		return fmt.Errorf("policy name is required")
	}

	if _, ok := e.handlers[cfg.Name]; !ok {
		return fmt.Errorf("unknown built-in policy: %s", cfg.Name)
	}

	return nil
}

// ListBuiltinPolicies returns a list of available built-in policies
func (e *BuiltinEvaluator) ListBuiltinPolicies() []BuiltinPolicyName {
	names := make([]BuiltinPolicyName, 0, len(e.handlers))
	for name := range e.handlers {
		names = append(names, name)
	}
	return names
}

// evaluateRequireLabels checks for required labels
func (e *BuiltinEvaluator) evaluateRequireLabels(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config RequireLabelsConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid require-labels config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	// Get labels from resource
	labels := getResourceLabels(input.Resource)

	// Check required labels
	for _, reqLabel := range config.Labels {
		value, exists := labels[reqLabel]
		if !exists {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "require-labels",
				Message:     fmt.Sprintf("missing required label: %s", reqLabel),
				Severity:    severity,
				Path:        fmt.Sprintf("labels.%s", reqLabel),
				Remediation: fmt.Sprintf("Add label '%s' to the resource", reqLabel),
			})
		} else if config.RequireValues && value == "" {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "require-labels",
				Message:     fmt.Sprintf("label '%s' must have a value", reqLabel),
				Severity:    severity,
				Path:        fmt.Sprintf("labels.%s", reqLabel),
				Remediation: fmt.Sprintf("Set a non-empty value for label '%s'", reqLabel),
			})
		}
	}

	// Check label patterns
	for _, pattern := range config.LabelPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("invalid pattern '%s': %v", pattern, err))
			continue
		}

		found := false
		for key := range labels {
			if re.MatchString(key) {
				found = true
				break
			}
		}

		if !found {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "require-labels",
				Message:     fmt.Sprintf("no label matching pattern: %s", pattern),
				Severity:    severity,
				Remediation: fmt.Sprintf("Add a label matching pattern '%s'", pattern),
			})
		}
	}

	if result.Allowed {
		result.Message = "All required labels present"
	} else {
		result.Message = fmt.Sprintf("%d label violations found", len(result.Violations))
	}

	return result, nil
}

// evaluateRequireOwner checks for owner/team annotations
func (e *BuiltinEvaluator) evaluateRequireOwner(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config RequireOwnerConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid require-owner config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	labels := getResourceLabels(input.Resource)

	// Check owner label
	if config.OwnerLabel != "" {
		owner, exists := labels[config.OwnerLabel]
		if !exists {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "require-owner",
				Message:     fmt.Sprintf("missing owner label: %s", config.OwnerLabel),
				Severity:    severity,
				Path:        fmt.Sprintf("labels.%s", config.OwnerLabel),
				Remediation: fmt.Sprintf("Add owner label '%s'", config.OwnerLabel),
			})
		} else if len(config.ValidOwners) > 0 && !contains(config.ValidOwners, owner) {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "require-owner",
				Message:     fmt.Sprintf("invalid owner '%s', must be one of: %v", owner, config.ValidOwners),
				Severity:    severity,
				Path:        fmt.Sprintf("labels.%s", config.OwnerLabel),
				Actual:      owner,
				Expected:    config.ValidOwners,
				Remediation: fmt.Sprintf("Set owner to one of: %v", config.ValidOwners),
			})
		}
	}

	// Check team label
	if config.TeamLabel != "" {
		team, exists := labels[config.TeamLabel]
		if !exists {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "require-owner",
				Message:     fmt.Sprintf("missing team label: %s", config.TeamLabel),
				Severity:    severity,
				Path:        fmt.Sprintf("labels.%s", config.TeamLabel),
				Remediation: fmt.Sprintf("Add team label '%s'", config.TeamLabel),
			})
		} else if len(config.ValidTeams) > 0 && !contains(config.ValidTeams, team) {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "require-owner",
				Message:     fmt.Sprintf("invalid team '%s', must be one of: %v", team, config.ValidTeams),
				Severity:    severity,
				Path:        fmt.Sprintf("labels.%s", config.TeamLabel),
				Actual:      team,
				Expected:    config.ValidTeams,
				Remediation: fmt.Sprintf("Set team to one of: %v", config.ValidTeams),
			})
		}
	}

	if result.Allowed {
		result.Message = "Owner/team requirements satisfied"
	}

	return result, nil
}

// evaluateAllowedEnvironments checks environment restrictions
func (e *BuiltinEvaluator) evaluateAllowedEnvironments(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config AllowedEnvironmentsConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid allowed-environments config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	envKey := config.EnvironmentKey
	if envKey == "" {
		envKey = "environment"
	}

	// Get environment from context
	env := ""
	if input.Context != nil {
		if v, ok := input.Context[envKey]; ok {
			env = fmt.Sprintf("%v", v)
		}
	}

	if env == "" {
		result.Warnings = append(result.Warnings, "No environment specified in context")
		result.Message = "No environment specified"
		return result, nil
	}

	if !contains(config.Environments, env) {
		result.Allowed = false
		result.Violations = append(result.Violations, Violation{
			Rule:        "allowed-environments",
			Message:     fmt.Sprintf("environment '%s' is not allowed", env),
			Severity:    severity,
			Actual:      env,
			Expected:    config.Environments,
			Remediation: fmt.Sprintf("Use one of allowed environments: %v", config.Environments),
		})
	} else {
		result.Message = fmt.Sprintf("Environment '%s' is allowed", env)
	}

	return result, nil
}

// evaluateAllowedActions checks action restrictions
func (e *BuiltinEvaluator) evaluateAllowedActions(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config AllowedActionsConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid allowed-actions config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	// Check direct action match
	if contains(config.Actions, input.Action) {
		result.Message = fmt.Sprintf("Action '%s' is allowed", input.Action)
		return result, nil
	}

	// Check patterns
	for _, pattern := range config.ActionPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("invalid pattern '%s': %v", pattern, err))
			continue
		}
		if re.MatchString(input.Action) {
			result.Message = fmt.Sprintf("Action '%s' matches pattern '%s'", input.Action, pattern)
			return result, nil
		}
	}

	result.Allowed = false
	result.Violations = append(result.Violations, Violation{
		Rule:        "allowed-actions",
		Message:     fmt.Sprintf("action '%s' is not allowed", input.Action),
		Severity:    severity,
		Actual:      input.Action,
		Expected:    config.Actions,
		Remediation: fmt.Sprintf("Use one of allowed actions: %v", config.Actions),
	})

	return result, nil
}

// evaluateDenyPrivileged checks for privileged execution
func (e *BuiltinEvaluator) evaluateDenyPrivileged(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	// Check for privileged flag in context
	if input.Context != nil {
		if privileged, ok := input.Context["privileged"]; ok {
			if p, ok := privileged.(bool); ok && p {
				result.Allowed = false
				result.Violations = append(result.Violations, Violation{
					Rule:        "deny-privileged",
					Message:     "privileged execution is not allowed",
					Severity:    severity,
					Remediation: "Remove privileged flag from execution context",
				})
				return result, nil
			}
		}

		// Check for root/sudo in context
		if user, ok := input.Context["run_as_user"]; ok {
			userStr := fmt.Sprintf("%v", user)
			if userStr == "root" || userStr == "0" {
				result.Allowed = false
				result.Violations = append(result.Violations, Violation{
					Rule:        "deny-privileged",
					Message:     "running as root is not allowed",
					Severity:    severity,
					Actual:      userStr,
					Remediation: "Use a non-root user for execution",
				})
				return result, nil
			}
		}
	}

	result.Message = "No privileged execution detected"
	return result, nil
}

// evaluateAllowedUsers checks user restrictions
func (e *BuiltinEvaluator) evaluateAllowedUsers(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config AllowedUsersConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid allowed-users config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	if input.User == "" {
		result.Warnings = append(result.Warnings, "No user specified")
		result.Message = "No user specified"
		return result, nil
	}

	// Check direct user match
	if contains(config.Users, input.User) {
		result.Message = fmt.Sprintf("User '%s' is allowed", input.User)
		return result, nil
	}

	// Check patterns
	for _, pattern := range config.UserPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("invalid pattern '%s': %v", pattern, err))
			continue
		}
		if re.MatchString(input.User) {
			result.Message = fmt.Sprintf("User '%s' matches pattern '%s'", input.User, pattern)
			return result, nil
		}
	}

	// Check groups
	if len(config.Groups) > 0 && input.Context != nil {
		if groups, ok := input.Context["user_groups"]; ok {
			if groupList, ok := groups.([]string); ok {
				for _, g := range groupList {
					if contains(config.Groups, g) {
						result.Message = fmt.Sprintf("User in allowed group '%s'", g)
						return result, nil
					}
				}
			}
		}
	}

	result.Allowed = false
	result.Violations = append(result.Violations, Violation{
		Rule:        "allowed-users",
		Message:     fmt.Sprintf("user '%s' is not allowed", input.User),
		Severity:    severity,
		Actual:      input.User,
		Expected:    config.Users,
		Remediation: "Use an allowed user account",
	})

	return result, nil
}

// evaluateDeniedUsers checks for denied users
func (e *BuiltinEvaluator) evaluateDeniedUsers(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config AllowedUsersConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid denied-users config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	if input.User == "" {
		result.Message = "No user specified"
		return result, nil
	}

	// Check direct user match
	if contains(config.Users, input.User) {
		result.Allowed = false
		result.Violations = append(result.Violations, Violation{
			Rule:        "denied-users",
			Message:     fmt.Sprintf("user '%s' is denied", input.User),
			Severity:    severity,
			Actual:      input.User,
			Remediation: "Use a different user account",
		})
		return result, nil
	}

	// Check patterns
	for _, pattern := range config.UserPatterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("invalid pattern '%s': %v", pattern, err))
			continue
		}
		if re.MatchString(input.User) {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "denied-users",
				Message:     fmt.Sprintf("user '%s' matches denied pattern '%s'", input.User, pattern),
				Severity:    severity,
				Actual:      input.User,
				Remediation: "Use a different user account",
			})
			return result, nil
		}
	}

	result.Message = fmt.Sprintf("User '%s' is not denied", input.User)
	return result, nil
}

// evaluateTimeWindow checks time-based restrictions
func (e *BuiltinEvaluator) evaluateTimeWindow(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config TimeWindowConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid time-window config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	// Get timezone
	loc := time.UTC
	if config.Timezone != "" {
		var err error
		loc, err = time.LoadLocation(config.Timezone)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("invalid timezone '%s', using UTC", config.Timezone))
			loc = time.UTC
		}
	}

	now := time.Now().In(loc)

	// Check blocked dates
	dateStr := now.Format("2006-01-02")
	if contains(config.BlockedDates, dateStr) {
		result.Allowed = false
		result.Violations = append(result.Violations, Violation{
			Rule:        "time-window",
			Message:     fmt.Sprintf("operations blocked on %s", dateStr),
			Severity:    severity,
			Actual:      dateStr,
			Remediation: "Wait until an allowed date",
		})
		return result, nil
	}

	// Check allowed days
	if len(config.AllowedDays) > 0 {
		dayNum := int(now.Weekday())
		if !containsInt(config.AllowedDays, dayNum) {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "time-window",
				Message:     fmt.Sprintf("operations not allowed on %s", now.Weekday().String()),
				Severity:    severity,
				Actual:      now.Weekday().String(),
				Remediation: "Wait until an allowed day",
			})
			return result, nil
		}
	}

	// Check allowed hours
	if config.AllowedHoursStart != 0 || config.AllowedHoursEnd != 0 {
		hour := now.Hour()
		if config.AllowedHoursStart <= config.AllowedHoursEnd {
			// Normal range (e.g., 9-17)
			if hour < config.AllowedHoursStart || hour >= config.AllowedHoursEnd {
				result.Allowed = false
				result.Violations = append(result.Violations, Violation{
					Rule:        "time-window",
					Message:     fmt.Sprintf("operations only allowed between %02d:00-%02d:00", config.AllowedHoursStart, config.AllowedHoursEnd),
					Severity:    severity,
					Actual:      fmt.Sprintf("%02d:00", hour),
					Remediation: fmt.Sprintf("Wait until %02d:00", config.AllowedHoursStart),
				})
				return result, nil
			}
		} else {
			// Overnight range (e.g., 22-6)
			if hour < config.AllowedHoursStart && hour >= config.AllowedHoursEnd {
				result.Allowed = false
				result.Violations = append(result.Violations, Violation{
					Rule:        "time-window",
					Message:     fmt.Sprintf("operations only allowed between %02d:00-%02d:00", config.AllowedHoursStart, config.AllowedHoursEnd),
					Severity:    severity,
					Actual:      fmt.Sprintf("%02d:00", hour),
					Remediation: fmt.Sprintf("Wait until %02d:00", config.AllowedHoursStart),
				})
				return result, nil
			}
		}
	}

	result.Message = "Within allowed time window"
	return result, nil
}

// evaluateNoRootExecution checks for root execution
func (e *BuiltinEvaluator) evaluateNoRootExecution(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config NoRootExecutionConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid no-root-execution config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	// Check if running as root
	runAsUser := ""
	if input.Context != nil {
		if user, ok := input.Context["run_as_user"]; ok {
			runAsUser = fmt.Sprintf("%v", user)
		}
	}

	if runAsUser != "root" && runAsUser != "0" {
		result.Message = "Not running as root"
		return result, nil
	}

	// Check if user is allowed to run as root
	if contains(config.AllowedUsers, input.User) {
		result.Message = fmt.Sprintf("User '%s' is allowed to run as root", input.User)
		return result, nil
	}

	// Check if action is allowed as root
	if contains(config.AllowedActions, input.Action) {
		result.Message = fmt.Sprintf("Action '%s' is allowed as root", input.Action)
		return result, nil
	}

	result.Allowed = false
	result.Violations = append(result.Violations, Violation{
		Rule:        "no-root-execution",
		Message:     fmt.Sprintf("user '%s' cannot run action '%s' as root", input.User, input.Action),
		Severity:    severity,
		Actual:      fmt.Sprintf("user=%s, action=%s, run_as=root", input.User, input.Action),
		Remediation: "Use a non-root user or request an exception",
	})

	return result, nil
}

// evaluateRequireApproval checks if approval is required
func (e *BuiltinEvaluator) evaluateRequireApproval(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config RequireApprovalConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid require-approval config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	// Check if action requires approval
	if !contains(config.Actions, input.Action) {
		result.Message = fmt.Sprintf("Action '%s' does not require approval", input.Action)
		return result, nil
	}

	// Check if approval was provided
	if input.Context != nil {
		if approved, ok := input.Context["approved"]; ok {
			if a, ok := approved.(bool); ok && a {
				result.Message = "Approval granted"
				return result, nil
			}
		}
	}

	result.Allowed = false
	result.Violations = append(result.Violations, Violation{
		Rule:        "require-approval",
		Message:     fmt.Sprintf("action '%s' requires approval", input.Action),
		Severity:    severity,
		Remediation: "Request approval from an authorized approver",
	})

	return result, nil
}

// evaluateMaxConcurrent checks concurrent operation limits
func (e *BuiltinEvaluator) evaluateMaxConcurrent(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config MaxConcurrentConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid max-concurrent config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	// Get current concurrent count from context
	currentCount := 0
	if input.Context != nil {
		if count, ok := input.Context["concurrent_count"]; ok {
			if c, ok := count.(int); ok {
				currentCount = c
			} else if c, ok := count.(float64); ok {
				currentCount = int(c)
			}
		}
	}

	if currentCount >= config.MaxConcurrent {
		result.Allowed = false
		result.Violations = append(result.Violations, Violation{
			Rule:        "max-concurrent",
			Message:     fmt.Sprintf("maximum concurrent operations (%d) reached", config.MaxConcurrent),
			Severity:    severity,
			Actual:      currentCount,
			Expected:    config.MaxConcurrent,
			Remediation: "Wait for existing operations to complete",
		})
	} else {
		result.Message = fmt.Sprintf("Concurrent count %d/%d", currentCount, config.MaxConcurrent)
	}

	return result, nil
}

// evaluateResourceQuota checks resource quotas
func (e *BuiltinEvaluator) evaluateResourceQuota(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config ResourceQuotaConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid resource-quota config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	if input.Context == nil {
		result.Message = "No quota context available"
		return result, nil
	}

	// Check total resources
	if config.MaxResources > 0 {
		if count, ok := input.Context["total_resources"]; ok {
			c := toInt(count)
			if c >= config.MaxResources {
				result.Allowed = false
				result.Violations = append(result.Violations, Violation{
					Rule:        "resource-quota",
					Message:     fmt.Sprintf("total resource quota (%d) exceeded", config.MaxResources),
					Severity:    severity,
					Actual:      c,
					Expected:    config.MaxResources,
					Remediation: "Delete unused resources or request a quota increase",
				})
			}
		}
	}

	// Check per-user resources
	if config.MaxPerUser > 0 {
		if count, ok := input.Context["user_resources"]; ok {
			c := toInt(count)
			if c >= config.MaxPerUser {
				result.Allowed = false
				result.Violations = append(result.Violations, Violation{
					Rule:        "resource-quota",
					Message:     fmt.Sprintf("user resource quota (%d) exceeded", config.MaxPerUser),
					Severity:    severity,
					Actual:      c,
					Expected:    config.MaxPerUser,
					Remediation: "Delete your unused resources or request a quota increase",
				})
			}
		}
	}

	if result.Allowed {
		result.Message = "Within resource quotas"
	}

	return result, nil
}

// evaluatePatternDeny blocks resources matching patterns
func (e *BuiltinEvaluator) evaluatePatternDeny(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config PatternConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid pattern-deny config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	// Get field value
	field := config.Field
	if field == "" {
		field = "name"
	}
	value := getResourceField(input.Resource, field)

	for _, pattern := range config.Patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("invalid pattern '%s': %v", pattern, err))
			continue
		}
		if re.MatchString(value) {
			result.Allowed = false
			result.Violations = append(result.Violations, Violation{
				Rule:        "pattern-deny",
				Message:     fmt.Sprintf("'%s' matches denied pattern '%s'", value, pattern),
				Severity:    severity,
				Actual:      value,
				Path:        field,
				Remediation: fmt.Sprintf("Use a name that doesn't match pattern '%s'", pattern),
			})
			return result, nil
		}
	}

	result.Message = "No denied patterns matched"
	return result, nil
}

// evaluatePatternAllow only allows resources matching patterns
func (e *BuiltinEvaluator) evaluatePatternAllow(ctx context.Context, configData json.RawMessage, input *EvaluationInput, severity Severity) (*EvaluationResult, error) {
	var config PatternConfig
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("invalid pattern-allow config: %w", err)
	}

	result := &EvaluationResult{
		Allowed:    true,
		Violations: make([]Violation, 0),
		Warnings:   make([]string, 0),
	}

	// Get field value
	field := config.Field
	if field == "" {
		field = "name"
	}
	value := getResourceField(input.Resource, field)

	for _, pattern := range config.Patterns {
		re, err := regexp.Compile(pattern)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("invalid pattern '%s': %v", pattern, err))
			continue
		}
		if re.MatchString(value) {
			result.Message = fmt.Sprintf("'%s' matches allowed pattern '%s'", value, pattern)
			return result, nil
		}
	}

	result.Allowed = false
	result.Violations = append(result.Violations, Violation{
		Rule:        "pattern-allow",
		Message:     fmt.Sprintf("'%s' does not match any allowed patterns", value),
		Severity:    severity,
		Actual:      value,
		Path:        field,
		Expected:    config.Patterns,
		Remediation: fmt.Sprintf("Use a name that matches one of: %v", config.Patterns),
	})

	return result, nil
}

// Helper functions

func getResourceLabels(resource interface{}) map[string]string {
	if resource == nil {
		return make(map[string]string)
	}

	// Try direct map access
	if m, ok := resource.(map[string]interface{}); ok {
		if labels, ok := m["labels"]; ok {
			if labelMap, ok := labels.(map[string]interface{}); ok {
				result := make(map[string]string)
				for k, v := range labelMap {
					result[k] = fmt.Sprintf("%v", v)
				}
				return result
			}
			if labelMap, ok := labels.(map[string]string); ok {
				return labelMap
			}
		}
		// Maybe labels are at top level
		result := make(map[string]string)
		for k, v := range m {
			if s, ok := v.(string); ok {
				result[k] = s
			}
		}
		return result
	}

	return make(map[string]string)
}

func getResourceField(resource interface{}, field string) string {
	if resource == nil {
		return ""
	}

	if m, ok := resource.(map[string]interface{}); ok {
		// Handle nested fields with dot notation
		parts := strings.Split(field, ".")
		current := m
		for i, part := range parts {
			if i == len(parts)-1 {
				if v, ok := current[part]; ok {
					return fmt.Sprintf("%v", v)
				}
			} else {
				if nested, ok := current[part].(map[string]interface{}); ok {
					current = nested
				} else {
					return ""
				}
			}
		}
	}

	return ""
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func containsInt(slice []int, item int) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	default:
		return 0
	}
}
