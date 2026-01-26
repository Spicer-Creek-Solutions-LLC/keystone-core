package remediation

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
)

// RemediationType defines the type of remediation action
type RemediationType string

const (
	// RemediationTypeRollback rolls back to previous version
	RemediationTypeRollback RemediationType = "rollback"
	// RemediationTypeRestart restarts the affected services
	RemediationTypeRestart RemediationType = "restart"
	// RemediationTypeScale scales up/down resources
	RemediationTypeScale RemediationType = "scale"
	// RemediationTypeRunbook executes a predefined runbook
	RemediationTypeRunbook RemediationType = "runbook"
	// RemediationTypeScript executes a custom script
	RemediationTypeScript RemediationType = "script"
	// RemediationTypeWebhook calls an external webhook
	RemediationTypeWebhook RemediationType = "webhook"
	// RemediationTypeAlert sends alerts without automated action
	RemediationTypeAlert RemediationType = "alert"
	// RemediationTypeCustom executes a custom remediation handler
	RemediationTypeCustom RemediationType = "custom"
)

// RemediationStatus tracks the status of a remediation action
type RemediationStatus string

const (
	// StatusTriggered remediation has been triggered
	StatusTriggered RemediationStatus = "triggered"
	// StatusPending waiting to execute
	StatusPending RemediationStatus = "pending"
	// StatusRunning remediation is executing
	StatusRunning RemediationStatus = "running"
	// StatusSucceeded remediation completed successfully
	StatusSucceeded RemediationStatus = "succeeded"
	// StatusFailed remediation failed
	StatusFailed RemediationStatus = "failed"
	// StatusSkipped remediation was skipped
	StatusSkipped RemediationStatus = "skipped"
	// StatusCooldown in cooldown period after previous remediation
	StatusCooldown RemediationStatus = "cooldown"
	// StatusMaxAttemptsReached max remediation attempts exceeded
	StatusMaxAttemptsReached RemediationStatus = "max_attempts_reached"
)

// RemediationAction defines a single remediation action
type RemediationAction struct {
	// Name of the action
	Name string `json:"name"`

	// Type of remediation
	Type RemediationType `json:"type"`

	// Priority for action ordering (higher executes first)
	Priority int `json:"priority,omitempty"`

	// Timeout for the action
	Timeout time.Duration `json:"timeout,omitempty"`

	// Config contains action-specific configuration
	Config map[string]interface{} `json:"config,omitempty"`

	// ContinueOnFailure continues with next action even if this fails
	ContinueOnFailure bool `json:"continue_on_failure,omitempty"`

	// RequiresConfirmation requires manual confirmation before execution
	RequiresConfirmation bool `json:"requires_confirmation,omitempty"`
}

// RemediationPolicy defines when and how to remediate
type RemediationPolicy struct {
	// Name of the policy
	Name string `json:"name"`

	// Description of the policy
	Description string `json:"description,omitempty"`

	// Enabled indicates if the policy is active
	Enabled bool `json:"enabled"`

	// Environment this policy applies to (empty for all)
	Environment string `json:"environment,omitempty"`

	// Application this policy applies to (empty for all)
	Application string `json:"application,omitempty"`

	// Triggers that activate this policy
	Triggers []RemediationTrigger `json:"triggers"`

	// Actions to execute in order
	Actions []RemediationAction `json:"actions"`

	// MaxAttempts maximum remediation attempts before giving up
	MaxAttempts int `json:"max_attempts"`

	// CooldownPeriod minimum time between remediation attempts
	CooldownPeriod time.Duration `json:"cooldown_period"`

	// EscalationPolicy what to do when max attempts reached
	EscalationPolicy *EscalationPolicy `json:"escalation_policy,omitempty"`

	// RequireApproval requires manual approval before remediation
	RequireApproval bool `json:"require_approval,omitempty"`

	// Approvers who can approve remediation
	Approvers []string `json:"approvers,omitempty"`

	// DryRun if true, don't execute but log what would happen
	DryRun bool `json:"dry_run,omitempty"`
}

// RemediationTrigger defines what triggers remediation
type RemediationTrigger struct {
	// Type of trigger
	Type TriggerType `json:"type"`

	// Condition for the trigger
	Condition TriggerCondition `json:"condition"`

	// Threshold for numeric conditions
	Threshold float64 `json:"threshold,omitempty"`

	// Duration the condition must persist
	Duration time.Duration `json:"duration,omitempty"`
}

// TriggerType defines what can trigger remediation
type TriggerType string

const (
	// TriggerVerificationFailure triggered by verification failure
	TriggerVerificationFailure TriggerType = "verification_failure"
	// TriggerMetricThreshold triggered by metric threshold breach
	TriggerMetricThreshold TriggerType = "metric_threshold"
	// TriggerHealthCheckFailure triggered by health check failure
	TriggerHealthCheckFailure TriggerType = "health_check_failure"
	// TriggerDeploymentFailure triggered by deployment failure
	TriggerDeploymentFailure TriggerType = "deployment_failure"
	// TriggerAlertFiring triggered by alert firing
	TriggerAlertFiring TriggerType = "alert_firing"
	// TriggerManual triggered manually
	TriggerManual TriggerType = "manual"
)

// TriggerCondition defines the condition for triggering
type TriggerCondition string

const (
	// ConditionAny any failure triggers
	ConditionAny TriggerCondition = "any"
	// ConditionConsecutive requires consecutive failures
	ConditionConsecutive TriggerCondition = "consecutive"
	// ConditionPercentage requires failure percentage threshold
	ConditionPercentage TriggerCondition = "percentage"
	// ConditionCount requires specific failure count
	ConditionCount TriggerCondition = "count"
)

// EscalationPolicy defines what to do when remediation fails
type EscalationPolicy struct {
	// Type of escalation
	Type EscalationType `json:"type"`

	// Channels to notify
	Channels []string `json:"channels,omitempty"`

	// Runbook to execute
	Runbook string `json:"runbook,omitempty"`

	// Webhook to call
	WebhookURL string `json:"webhook_url,omitempty"`
}

// EscalationType defines escalation types
type EscalationType string

const (
	// EscalationNotify send notifications
	EscalationNotify EscalationType = "notify"
	// EscalationPage page on-call
	EscalationPage EscalationType = "page"
	// EscalationRunbook execute escalation runbook
	EscalationRunbook EscalationType = "runbook"
	// EscalationWebhook call webhook
	EscalationWebhook EscalationType = "webhook"
)

// RemediationEvent records a remediation event
type RemediationEvent struct {
	// ID of the event
	ID string `json:"id"`

	// PolicyName that triggered this
	PolicyName string `json:"policy_name"`

	// TriggerType that initiated remediation
	TriggerType TriggerType `json:"trigger_type"`

	// TriggerDetails additional trigger information
	TriggerDetails map[string]interface{} `json:"trigger_details,omitempty"`

	// Environment being remediated
	Environment string `json:"environment"`

	// Application being remediated
	Application string `json:"application,omitempty"`

	// Actions executed
	Actions []ActionResult `json:"actions"`

	// Status overall status
	Status RemediationStatus `json:"status"`

	// Attempt number
	Attempt int `json:"attempt"`

	// StartTime when remediation started
	StartTime time.Time `json:"start_time"`

	// EndTime when remediation completed
	EndTime time.Time `json:"end_time"`

	// Duration of remediation
	Duration time.Duration `json:"duration"`

	// TriggeredBy user or system that triggered
	TriggeredBy string `json:"triggered_by"`

	// ApprovedBy user who approved (if required)
	ApprovedBy string `json:"approved_by,omitempty"`

	// Message summary message
	Message string `json:"message"`

	// Error if remediation failed
	Error string `json:"error,omitempty"`
}

// ActionResult records the result of a single action
type ActionResult struct {
	// Action name
	Name string `json:"name"`

	// Type of action
	Type RemediationType `json:"type"`

	// Status of the action
	Status RemediationStatus `json:"status"`

	// StartTime of action
	StartTime time.Time `json:"start_time"`

	// EndTime of action
	EndTime time.Time `json:"end_time"`

	// Duration of action
	Duration time.Duration `json:"duration"`

	// Output from action
	Output string `json:"output,omitempty"`

	// Error if action failed
	Error string `json:"error,omitempty"`
}

// RemediationHandler executes a specific type of remediation
type RemediationHandler interface {
	// Type returns the remediation type this handler handles
	Type() RemediationType

	// Execute performs the remediation action
	Execute(ctx context.Context, action *RemediationAction, event *RemediationEvent) (*ActionResult, error)

	// Validate validates the action configuration
	Validate(action *RemediationAction) error
}

// Engine orchestrates automatic remediation
type Engine struct {
	handlers       map[RemediationType]RemediationHandler
	policies       map[string]*RemediationPolicy
	events         []*RemediationEvent
	cooldowns      map[string]time.Time // key: environment:application
	attemptCounts  map[string]int       // key: environment:application
	mu             sync.RWMutex
	eventsMu       sync.RWMutex
	cooldownMu     sync.RWMutex
	notifier       Notifier
	approvalStore  ApprovalStore
	eventListeners []EventListener
}

// Notifier sends notifications
type Notifier interface {
	// Notify sends a notification
	Notify(ctx context.Context, channels []string, message string, event *RemediationEvent) error
}

// ApprovalStore manages remediation approvals
type ApprovalStore interface {
	// RequestApproval creates an approval request
	RequestApproval(ctx context.Context, event *RemediationEvent, approvers []string) (string, error)

	// GetApprovalStatus checks if approval was granted
	GetApprovalStatus(ctx context.Context, requestID string) (bool, string, error)

	// WaitForApproval waits for approval with timeout
	WaitForApproval(ctx context.Context, requestID string, timeout time.Duration) (bool, string, error)
}

// EventListener receives remediation events
type EventListener interface {
	// OnRemediationStarted called when remediation starts
	OnRemediationStarted(event *RemediationEvent)

	// OnRemediationCompleted called when remediation completes
	OnRemediationCompleted(event *RemediationEvent)

	// OnActionCompleted called when an action completes
	OnActionCompleted(event *RemediationEvent, result *ActionResult)
}

// NewEngine creates a new remediation engine
func NewEngine() *Engine {
	return &Engine{
		handlers:       make(map[RemediationType]RemediationHandler),
		policies:       make(map[string]*RemediationPolicy),
		events:         make([]*RemediationEvent, 0),
		cooldowns:      make(map[string]time.Time),
		attemptCounts:  make(map[string]int),
		eventListeners: make([]EventListener, 0),
	}
}

// SetNotifier sets the notifier for alerts and escalations
func (e *Engine) SetNotifier(notifier Notifier) {
	e.notifier = notifier
}

// SetApprovalStore sets the approval store for manual approvals
func (e *Engine) SetApprovalStore(store ApprovalStore) {
	e.approvalStore = store
}

// AddEventListener adds an event listener
func (e *Engine) AddEventListener(listener EventListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.eventListeners = append(e.eventListeners, listener)
}

// RegisterHandler registers a remediation handler
func (e *Engine) RegisterHandler(handler RemediationHandler) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.handlers[handler.Type()] = handler
}

// RegisterPolicy registers a remediation policy
func (e *Engine) RegisterPolicy(policy *RemediationPolicy) error {
	if policy.Name == "" {
		return fmt.Errorf("policy name is required")
	}
	if len(policy.Actions) == 0 {
		return fmt.Errorf("policy must have at least one action")
	}

	// Validate actions
	e.mu.RLock()
	for _, action := range policy.Actions {
		handler, ok := e.handlers[action.Type]
		if !ok {
			e.mu.RUnlock()
			return fmt.Errorf("no handler registered for action type: %s", action.Type)
		}
		if err := handler.Validate(&action); err != nil {
			e.mu.RUnlock()
			return fmt.Errorf("invalid action %s: %w", action.Name, err)
		}
	}
	e.mu.RUnlock()

	e.mu.Lock()
	defer e.mu.Unlock()
	e.policies[policy.Name] = policy
	return nil
}

// GetPolicy returns a policy by name
func (e *Engine) GetPolicy(name string) (*RemediationPolicy, bool) {
	e.mu.RLock()
	defer e.mu.RUnlock()
	policy, ok := e.policies[name]
	return policy, ok
}

// ListPolicies returns all registered policies
func (e *Engine) ListPolicies() []*RemediationPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()

	policies := make([]*RemediationPolicy, 0, len(e.policies))
	for _, p := range e.policies {
		policies = append(policies, p)
	}
	return policies
}

// RemovePolicy removes a policy
func (e *Engine) RemovePolicy(name string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	if _, ok := e.policies[name]; ok {
		delete(e.policies, name)
		return true
	}
	return false
}

// TriggerRequest contains information to trigger remediation
type TriggerRequest struct {
	// TriggerType what triggered the remediation
	TriggerType TriggerType

	// Environment to remediate
	Environment string

	// Application to remediate (optional)
	Application string

	// Details additional context
	Details map[string]interface{}

	// TriggeredBy who/what triggered
	TriggeredBy string

	// Force skip cooldown and attempt limits
	Force bool
}

// Trigger triggers remediation based on matching policies
func (e *Engine) Trigger(ctx context.Context, req *TriggerRequest) ([]*RemediationEvent, error) {
	// Find matching policies
	policies := e.findMatchingPolicies(req)
	if len(policies) == 0 {
		return nil, nil // No matching policies
	}

	events := make([]*RemediationEvent, 0, len(policies))
	for _, policy := range policies {
		event, err := e.executePolicy(ctx, policy, req)
		if err != nil {
			// Log error but continue with other policies
			event = &RemediationEvent{
				ID:             uuid.New().String(),
				PolicyName:     policy.Name,
				TriggerType:    req.TriggerType,
				TriggerDetails: req.Details,
				Environment:    req.Environment,
				Application:    req.Application,
				Status:         StatusFailed,
				StartTime:      time.Now(),
				EndTime:        time.Now(),
				TriggeredBy:    req.TriggeredBy,
				Error:          err.Error(),
				Message:        fmt.Sprintf("Failed to execute policy: %v", err),
			}
		}
		events = append(events, event)
	}

	return events, nil
}

// findMatchingPolicies finds policies that match the trigger
func (e *Engine) findMatchingPolicies(req *TriggerRequest) []*RemediationPolicy {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var matches []*RemediationPolicy
	for _, policy := range e.policies {
		if !policy.Enabled {
			continue
		}

		// Check environment match
		if policy.Environment != "" && policy.Environment != req.Environment {
			continue
		}

		// Check application match
		if policy.Application != "" && policy.Application != req.Application {
			continue
		}

		// Check trigger match
		hasMatchingTrigger := false
		for _, trigger := range policy.Triggers {
			if trigger.Type == req.TriggerType {
				hasMatchingTrigger = true
				break
			}
		}
		if !hasMatchingTrigger {
			continue
		}

		matches = append(matches, policy)
	}

	return matches
}

// executePolicy executes a single policy
func (e *Engine) executePolicy(ctx context.Context, policy *RemediationPolicy, req *TriggerRequest) (*RemediationEvent, error) {
	key := fmt.Sprintf("%s:%s", req.Environment, req.Application)

	// Check cooldown
	if !req.Force {
		e.cooldownMu.RLock()
		lastRun, hasCooldown := e.cooldowns[key]
		e.cooldownMu.RUnlock()

		if hasCooldown && time.Since(lastRun) < policy.CooldownPeriod {
			return &RemediationEvent{
				ID:             uuid.New().String(),
				PolicyName:     policy.Name,
				TriggerType:    req.TriggerType,
				Environment:    req.Environment,
				Application:    req.Application,
				Status:         StatusCooldown,
				StartTime:      time.Now(),
				EndTime:        time.Now(),
				TriggeredBy:    req.TriggeredBy,
				Message:        fmt.Sprintf("In cooldown period until %v", lastRun.Add(policy.CooldownPeriod)),
			}, nil
		}
	}

	// Check max attempts
	if !req.Force {
		e.cooldownMu.RLock()
		attempts := e.attemptCounts[key]
		e.cooldownMu.RUnlock()

		if policy.MaxAttempts > 0 && attempts >= policy.MaxAttempts {
			// Handle escalation
			event := &RemediationEvent{
				ID:             uuid.New().String(),
				PolicyName:     policy.Name,
				TriggerType:    req.TriggerType,
				Environment:    req.Environment,
				Application:    req.Application,
				Status:         StatusMaxAttemptsReached,
				Attempt:        attempts + 1,
				StartTime:      time.Now(),
				EndTime:        time.Now(),
				TriggeredBy:    req.TriggeredBy,
				Message:        fmt.Sprintf("Max attempts (%d) reached", policy.MaxAttempts),
			}

			// Execute escalation
			if policy.EscalationPolicy != nil {
				e.executeEscalation(ctx, policy.EscalationPolicy, event)
			}

			return event, nil
		}
	}

	// Create event
	event := &RemediationEvent{
		ID:             uuid.New().String(),
		PolicyName:     policy.Name,
		TriggerType:    req.TriggerType,
		TriggerDetails: req.Details,
		Environment:    req.Environment,
		Application:    req.Application,
		Status:         StatusTriggered,
		StartTime:      time.Now(),
		TriggeredBy:    req.TriggeredBy,
		Actions:        make([]ActionResult, 0, len(policy.Actions)),
	}

	// Get attempt number
	e.cooldownMu.RLock()
	event.Attempt = e.attemptCounts[key] + 1
	e.cooldownMu.RUnlock()

	// Notify listeners
	e.notifyListeners(func(l EventListener) { l.OnRemediationStarted(event) })

	// Handle approval if required
	if policy.RequireApproval && e.approvalStore != nil {
		event.Status = StatusPending
		requestID, err := e.approvalStore.RequestApproval(ctx, event, policy.Approvers)
		if err != nil {
			event.Status = StatusFailed
			event.Error = fmt.Sprintf("Failed to request approval: %v", err)
			event.EndTime = time.Now()
			event.Duration = event.EndTime.Sub(event.StartTime)
			return event, err
		}

		// Wait for approval (with timeout)
		timeout := 30 * time.Minute // Default approval timeout
		approved, approver, err := e.approvalStore.WaitForApproval(ctx, requestID, timeout)
		if err != nil {
			event.Status = StatusFailed
			event.Error = fmt.Sprintf("Approval failed: %v", err)
			event.EndTime = time.Now()
			event.Duration = event.EndTime.Sub(event.StartTime)
			return event, err
		}

		if !approved {
			event.Status = StatusSkipped
			event.Message = "Remediation not approved"
			event.EndTime = time.Now()
			event.Duration = event.EndTime.Sub(event.StartTime)
			return event, nil
		}

		event.ApprovedBy = approver
	}

	// Execute actions
	event.Status = StatusRunning
	if policy.DryRun {
		event.Message = "[DRY RUN] "
	}

	allSucceeded := true
	for _, action := range policy.Actions {
		result := e.executeAction(ctx, &action, event, policy.DryRun)
		event.Actions = append(event.Actions, *result)

		// Notify listeners
		e.notifyListeners(func(l EventListener) { l.OnActionCompleted(event, result) })

		if result.Status != StatusSucceeded {
			allSucceeded = false
			if !action.ContinueOnFailure {
				event.Status = StatusFailed
				event.Error = fmt.Sprintf("Action %s failed: %s", action.Name, result.Error)
				break
			}
		}
	}

	// Update status
	if allSucceeded {
		event.Status = StatusSucceeded
		event.Message += "All remediation actions completed successfully"

		// Reset attempt count on success
		e.cooldownMu.Lock()
		e.attemptCounts[key] = 0
		e.cooldownMu.Unlock()
	} else if event.Status != StatusFailed {
		event.Status = StatusFailed
		event.Message += "Some remediation actions failed"
	}

	event.EndTime = time.Now()
	event.Duration = event.EndTime.Sub(event.StartTime)

	// Update cooldown
	e.cooldownMu.Lock()
	e.cooldowns[key] = event.EndTime
	if event.Status != StatusSucceeded {
		e.attemptCounts[key]++
	}
	e.cooldownMu.Unlock()

	// Store event
	e.eventsMu.Lock()
	e.events = append(e.events, event)
	e.eventsMu.Unlock()

	// Notify listeners
	e.notifyListeners(func(l EventListener) { l.OnRemediationCompleted(event) })

	return event, nil
}

// executeAction executes a single remediation action
func (e *Engine) executeAction(ctx context.Context, action *RemediationAction, event *RemediationEvent, dryRun bool) *ActionResult {
	result := &ActionResult{
		Name:      action.Name,
		Type:      action.Type,
		StartTime: time.Now(),
	}

	// Get handler
	e.mu.RLock()
	handler, ok := e.handlers[action.Type]
	e.mu.RUnlock()

	if !ok {
		result.Status = StatusFailed
		result.Error = fmt.Sprintf("No handler for action type: %s", action.Type)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result
	}

	// Dry run
	if dryRun {
		result.Status = StatusSucceeded
		result.Output = fmt.Sprintf("[DRY RUN] Would execute %s action: %s", action.Type, action.Name)
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result
	}

	// Set timeout
	actionCtx := ctx
	if action.Timeout > 0 {
		var cancel context.CancelFunc
		actionCtx, cancel = context.WithTimeout(ctx, action.Timeout)
		defer cancel()
	}

	// Execute
	execResult, err := handler.Execute(actionCtx, action, event)
	if err != nil {
		result.Status = StatusFailed
		result.Error = err.Error()
	} else if execResult != nil {
		result = execResult
	} else {
		result.Status = StatusSucceeded
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)
	return result
}

// executeEscalation handles escalation when max attempts reached
func (e *Engine) executeEscalation(ctx context.Context, policy *EscalationPolicy, event *RemediationEvent) {
	if e.notifier == nil {
		return
	}

	message := fmt.Sprintf("Remediation max attempts reached for %s in %s: %s",
		event.Application, event.Environment, event.Message)

	switch policy.Type {
	case EscalationNotify, EscalationPage:
		_ = e.notifier.Notify(ctx, policy.Channels, message, event)
	case EscalationWebhook:
		_ = e.notifier.Notify(ctx, []string{policy.WebhookURL}, message, event)
	}
}

// notifyListeners notifies all event listeners
func (e *Engine) notifyListeners(fn func(EventListener)) {
	e.mu.RLock()
	listeners := make([]EventListener, len(e.eventListeners))
	copy(listeners, e.eventListeners)
	e.mu.RUnlock()

	for _, l := range listeners {
		fn(l)
	}
}

// GetEvents returns remediation events with optional filters
func (e *Engine) GetEvents(environment, application string, limit int) []*RemediationEvent {
	e.eventsMu.RLock()
	defer e.eventsMu.RUnlock()

	var filtered []*RemediationEvent
	for i := len(e.events) - 1; i >= 0; i-- {
		event := e.events[i]
		if environment != "" && event.Environment != environment {
			continue
		}
		if application != "" && event.Application != application {
			continue
		}
		filtered = append(filtered, event)
		if limit > 0 && len(filtered) >= limit {
			break
		}
	}
	return filtered
}

// GetEvent returns a specific event by ID
func (e *Engine) GetEvent(id string) (*RemediationEvent, bool) {
	e.eventsMu.RLock()
	defer e.eventsMu.RUnlock()

	for _, event := range e.events {
		if event.ID == id {
			return event, true
		}
	}
	return nil, false
}

// ResetAttempts resets the attempt counter for an environment/application
func (e *Engine) ResetAttempts(environment, application string) {
	key := fmt.Sprintf("%s:%s", environment, application)
	e.cooldownMu.Lock()
	defer e.cooldownMu.Unlock()
	delete(e.attemptCounts, key)
	delete(e.cooldowns, key)
}

// GetAttemptCount returns the current attempt count
func (e *Engine) GetAttemptCount(environment, application string) int {
	key := fmt.Sprintf("%s:%s", environment, application)
	e.cooldownMu.RLock()
	defer e.cooldownMu.RUnlock()
	return e.attemptCounts[key]
}

// DefaultPolicies returns common default remediation policies
var DefaultPolicies = map[string]*RemediationPolicy{
	"verification-failure-rollback": {
		Name:        "verification-failure-rollback",
		Description: "Automatically rollback on verification failure",
		Enabled:     true,
		Triggers: []RemediationTrigger{
			{Type: TriggerVerificationFailure, Condition: ConditionAny},
		},
		Actions: []RemediationAction{
			{Name: "rollback", Type: RemediationTypeRollback, Priority: 100},
		},
		MaxAttempts:    3,
		CooldownPeriod: 5 * time.Minute,
		EscalationPolicy: &EscalationPolicy{
			Type:     EscalationPage,
			Channels: []string{"oncall"},
		},
	},
	"health-check-restart": {
		Name:        "health-check-restart",
		Description: "Restart service on health check failure",
		Enabled:     true,
		Triggers: []RemediationTrigger{
			{Type: TriggerHealthCheckFailure, Condition: ConditionConsecutive, Threshold: 3},
		},
		Actions: []RemediationAction{
			{Name: "restart", Type: RemediationTypeRestart, Priority: 100},
		},
		MaxAttempts:    5,
		CooldownPeriod: 2 * time.Minute,
	},
	"deployment-failure-alert": {
		Name:        "deployment-failure-alert",
		Description: "Alert on deployment failure without automated action",
		Enabled:     true,
		Triggers: []RemediationTrigger{
			{Type: TriggerDeploymentFailure, Condition: ConditionAny},
		},
		Actions: []RemediationAction{
			{Name: "alert", Type: RemediationTypeAlert, Priority: 100,
				Config: map[string]interface{}{
					"channels": []string{"deployments", "oncall"},
					"severity": "high",
				}},
		},
		MaxAttempts:    1,
		CooldownPeriod: 1 * time.Minute,
	},
}

// GetDefaultPolicy returns a default policy by name
func GetDefaultPolicy(name string) (*RemediationPolicy, bool) {
	policy, ok := DefaultPolicies[name]
	return policy, ok
}
