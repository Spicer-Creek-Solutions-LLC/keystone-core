package policy

import (
	"context"
	"fmt"
	"time"

	"github.com/titananvil/titan-anvil/pkg/events"
)

// EnforcementPoint defines where policy enforcement occurs
type EnforcementPoint string

const (
	EnforcementPointPreExecution  EnforcementPoint = "pre_execution"  // Before state execution
	EnforcementPointPostExecution EnforcementPoint = "post_execution" // After state execution
	EnforcementPointOnChange      EnforcementPoint = "on_change"      // On resource change
	EnforcementPointOnDrift       EnforcementPoint = "on_drift"       // On drift detection
	EnforcementPointOnEvent       EnforcementPoint = "on_event"       // On specific events
)

// EnforcementAction defines what happens when a policy fails
type EnforcementAction string

const (
	EnforcementActionBlock    EnforcementAction = "block"     // Block the operation
	EnforcementActionWarn     EnforcementAction = "warn"      // Warn but allow
	EnforcementActionAudit    EnforcementAction = "audit"     // Log for audit
	EnforcementActionRemediate EnforcementAction = "remediate" // Auto-remediate
)

// EnforcementConfig configures policy enforcement behavior
type EnforcementConfig struct {
	// Point where enforcement occurs
	Point EnforcementPoint

	// Action to take on policy violation
	Action EnforcementAction

	// ResourceTypes to enforce policies on
	ResourceTypes []string

	// EnableEventEmission enables policy violation events
	EnableEventEmission bool

	// EventPublisher for publishing policy events
	EventPublisher events.EventPublisher
}

// PolicyEnforcer enforces policies at various points in the system
type PolicyEnforcer struct {
	engine            *PolicyEngine
	config            *EnforcementConfig
	violationHandlers []ViolationHandler
}

// ViolationHandler handles policy violations
type ViolationHandler func(ctx context.Context, result *PolicyResult) error

// NewPolicyEnforcer creates a new policy enforcer
func NewPolicyEnforcer(engine *PolicyEngine, config *EnforcementConfig) *PolicyEnforcer {
	return &PolicyEnforcer{
		engine:            engine,
		config:            config,
		violationHandlers: make([]ViolationHandler, 0),
	}
}

// AddViolationHandler adds a custom violation handler
func (e *PolicyEnforcer) AddViolationHandler(handler ViolationHandler) {
	e.violationHandlers = append(e.violationHandlers, handler)
}

// EnforceForResource evaluates and enforces policies for a resource
func (e *PolicyEnforcer) EnforceForResource(ctx context.Context, resourceType string, input *EvaluationInput) (*PolicyResult, error) {
	// Check if resource type is in scope
	if len(e.config.ResourceTypes) > 0 {
		inScope := false
		for _, rt := range e.config.ResourceTypes {
			if rt == resourceType {
				inScope = true
				break
			}
		}
		if !inScope {
			// Resource type not in enforcement scope, allow
			return &PolicyResult{
				Allowed:       true,
				Results:       make([]*EvaluationResult, 0),
				EvaluatedAt:   time.Now(),
				TotalDuration: 0,
				Summary: &PolicySummary{
					TotalPolicies:        0,
					AllowedPolicies:      0,
					DeniedPolicies:       0,
					TotalViolations:      0,
					ViolationsBySeverity: make(map[Severity]int),
				},
			}, nil
		}
	}

	// Evaluate policies
	result, err := e.engine.EvaluateForResource(ctx, resourceType, input)
	if err != nil {
		return nil, fmt.Errorf("policy evaluation failed: %w", err)
	}

	// Handle violations
	if !result.Allowed || result.Summary.TotalViolations > 0 {
		if err := e.handleViolations(ctx, result); err != nil {
			return result, fmt.Errorf("violation handling failed: %w", err)
		}
	}

	// Emit event if configured
	if e.config.EnableEventEmission && e.config.EventPublisher != nil {
		if err := e.emitPolicyEvent(ctx, result); err != nil {
			// Log error but don't fail enforcement
			// In production, this would use proper logging
		}
	}

	// Apply enforcement action
	return e.applyEnforcementAction(result)
}

// EnforcePolicy evaluates and enforces a specific policy
func (e *PolicyEnforcer) EnforcePolicy(ctx context.Context, policyID string, input *EvaluationInput) (*EvaluationResult, error) {
	// Evaluate policy
	result, err := e.engine.Evaluate(ctx, policyID, input)
	if err != nil {
		return nil, fmt.Errorf("policy evaluation failed: %w", err)
	}

	// Handle violations
	if !result.Allowed || len(result.Violations) > 0 {
		policyResult := &PolicyResult{
			Allowed:       result.Allowed,
			Results:       []*EvaluationResult{result},
			EvaluatedAt:   result.EvaluatedAt,
			TotalDuration: result.Duration,
			Summary: &PolicySummary{
				TotalPolicies:   1,
				AllowedPolicies: 0,
				DeniedPolicies:  1,
				TotalViolations: len(result.Violations),
				ViolationsBySeverity: make(map[Severity]int),
			},
		}

		// Count violations by severity
		for _, v := range result.Violations {
			policyResult.Summary.ViolationsBySeverity[v.Severity]++
		}

		if err := e.handleViolations(ctx, policyResult); err != nil {
			return result, fmt.Errorf("violation handling failed: %w", err)
		}
	}

	return result, nil
}

// handleViolations processes policy violations
func (e *PolicyEnforcer) handleViolations(ctx context.Context, result *PolicyResult) error {
	// Call custom violation handlers
	for _, handler := range e.violationHandlers {
		if err := handler(ctx, result); err != nil {
			return fmt.Errorf("violation handler failed: %w", err)
		}
	}

	return nil
}

// applyEnforcementAction applies the configured enforcement action
func (e *PolicyEnforcer) applyEnforcementAction(result *PolicyResult) (*PolicyResult, error) {
	switch e.config.Action {
	case EnforcementActionBlock:
		// Block on violations - result.Allowed already reflects this
		return result, nil

	case EnforcementActionWarn:
		// Warn but allow
		result.Allowed = true
		return result, nil

	case EnforcementActionAudit:
		// Audit - allow but record violations
		result.Allowed = true
		return result, nil

	case EnforcementActionRemediate:
		// Remediation would be implemented here
		// For now, just return the result
		return result, nil

	default:
		return result, nil
	}
}

// emitPolicyEvent emits a policy evaluation event
func (e *PolicyEnforcer) emitPolicyEvent(ctx context.Context, result *PolicyResult) error {
	if e.config.EventPublisher == nil {
		return nil
	}

	// Determine event type based on result
	eventType := events.EventTypePolicyPass
	severity := events.SeverityInfo

	if !result.Allowed {
		eventType = events.EventTypePolicyViolation
		severity = e.severityToEventSeverity(result.Summary.ViolationsBySeverity)
	}

	// Build event data
	eventData := map[string]interface{}{
		"point":            string(e.config.Point),
		"action":           string(e.config.Action),
		"allowed":          result.Allowed,
		"total_policies":   result.Summary.TotalPolicies,
		"total_violations": result.Summary.TotalViolations,
		"evaluation_time":  result.TotalDuration.Milliseconds(),
	}

	// Add violation details
	if result.Summary.TotalViolations > 0 {
		violations := make([]map[string]interface{}, 0, len(result.Results))
		for _, r := range result.Results {
			for _, v := range r.Violations {
				violations = append(violations, map[string]interface{}{
					"policy":      r.PolicyID,
					"rule":        v.Rule,
					"message":     v.Message,
					"severity":    string(v.Severity),
					"path":        v.Path,
					"remediation": v.Remediation,
				})
			}
		}
		eventData["violations"] = violations
	}

	// Create and publish event
	event := events.NewEvent(eventType).
		Source("policy-enforcer").
		Severity(severity).
		DataMap(eventData).
		Build()

	return e.config.EventPublisher.Publish(event)
}

// severityToEventSeverity converts policy severity to event severity
func (e *PolicyEnforcer) severityToEventSeverity(severityMap map[Severity]int) events.Severity {
	if severityMap[SeverityCritical] > 0 {
		return events.SeverityCritical
	}
	if severityMap[SeverityHigh] > 0 {
		return events.SeverityError
	}
	if severityMap[SeverityMedium] > 0 {
		return events.SeverityWarning
	}
	return events.SeverityInfo
}

// StateEnforcementHook creates a hook for state management integration
func (e *PolicyEnforcer) StateEnforcementHook(resourceType string) func(context.Context, interface{}) error {
	return func(ctx context.Context, resource interface{}) error {
		// Convert resource to evaluation input
		input := &EvaluationInput{
			Resource: resource,
			Context:  make(map[string]interface{}),
		}

		// Add enforcement point to context
		input.Context["enforcement_point"] = string(e.config.Point)

		// Enforce policies
		result, err := e.EnforceForResource(ctx, resourceType, input)
		if err != nil {
			return err
		}

		// Block if not allowed
		if !result.Allowed {
			return fmt.Errorf("policy enforcement failed: %d violations", result.Summary.TotalViolations)
		}

		return nil
	}
}

// PolicyEnforcementAction implements an action for policy enforcement
type PolicyEnforcementAction struct {
	enforcer     *PolicyEnforcer
	resourceType string
}

// Name returns the action name
func (a *PolicyEnforcementAction) Name() string {
	return fmt.Sprintf("policy-enforce-%s", a.resourceType)
}

// Type returns the action type
func (a *PolicyEnforcementAction) Type() string {
	return "policy-enforcement"
}

// Execute enforces policies on the event
func (a *PolicyEnforcementAction) Execute(ctx context.Context, event *events.Event) error {
	// Extract resource from event
	input := &EvaluationInput{
		Resource: event.Data,
		Context:  make(map[string]interface{}),
	}

	// Add event context
	input.Context["event_type"] = string(event.Type)
	input.Context["event_source"] = event.Source
	input.Context["enforcement_point"] = string(a.enforcer.config.Point)

	// Enforce policies
	result, err := a.enforcer.EnforceForResource(ctx, a.resourceType, input)
	if err != nil {
		return err
	}

	// Handle based on enforcement action
	if !result.Allowed && a.enforcer.config.Action == EnforcementActionBlock {
		return fmt.Errorf("policy enforcement blocked operation: %d violations", result.Summary.TotalViolations)
	}

	return nil
}

// EventEnforcementReactor creates a reactor for event-based policy enforcement
func (e *PolicyEnforcer) EventEnforcementReactor(resourceType string) *events.Reactor {
	action := &PolicyEnforcementAction{
		enforcer:     e,
		resourceType: resourceType,
	}

	return &events.Reactor{
		ID:          fmt.Sprintf("policy-enforcer-%s", resourceType),
		Name:        fmt.Sprintf("Policy Enforcer for %s", resourceType),
		Description: fmt.Sprintf("Enforces policies on %s resources", resourceType),
		Filter:      nil, // Would be configured based on requirements
		Actions:     []events.Action{action},
		Enabled:      true,
		MaxConcurrent: 5,
		Timeout:      30 * time.Second,
	}
}

