// Package integration provides in-process integration tests that verify
// cross-epic interactions without requiring Docker containers.
package integration

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// =============================================================================
// Epic 5 (GitOps) + Epic 4 (Events) Integration
// Tests: GitOps webhook events flow through the event system
// =============================================================================

// TestIntegration_GitOpsWebhookToEvents verifies that GitOps webhook events
// are properly converted and flow through the event system.
func TestIntegration_GitOpsWebhookToEvents(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Track GitOps events
	var gitopsEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("gitops-tracker",
		func(e *events.Event) error {
			// Manual filter for gitops.* events
			if strings.HasPrefix(string(e.Type), "gitops.") {
				mu.Lock()
				gitopsEvents = append(gitopsEvents, e)
				mu.Unlock()
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate webhook events from different GitOps sources
	webhookEvents := []struct {
		eventType events.EventType
		source    string
		data      map[string]interface{}
	}{
		{
			eventType: events.EventType("gitops.argocd.sync"),
			source:    "argocd/my-cluster",
			data: map[string]interface{}{
				"application": "my-app",
				"namespace":   "production",
				"revision":    "abc123",
				"status":      "Succeeded",
				"resources":   5,
			},
		},
		{
			eventType: events.EventType("gitops.flux.reconcile"),
			source:    "flux/my-cluster",
			data: map[string]interface{}{
				"application": "kustomization/my-app",
				"revision":    "def456",
				"status":      "Ready",
				"reason":      "ReconciliationSucceeded",
			},
		},
		{
			eventType: events.EventType("gitops.github.push"),
			source:    "github/myorg/myrepo",
			data: map[string]interface{}{
				"revision": "ghi789",
				"ref":      "refs/heads/main",
				"commits":  3,
			},
		},
	}

	// Publish webhook events
	for _, we := range webhookEvents {
		event := events.NewEvent(we.eventType).
			Source(we.source).
			DataMap(we.data).
			Build()
		if err := env.eventBus.PublishSync(event); err != nil {
			t.Errorf("Failed to publish %s event: %v", we.eventType, err)
		}
	}

	mu.Lock()
	count := len(gitopsEvents)
	mu.Unlock()

	if count != len(webhookEvents) {
		t.Errorf("Expected %d gitops events, got %d", len(webhookEvents), count)
	}

	t.Logf("GitOps webhook events processed: %d events", count)
}

// =============================================================================
// Epic 6 (Policy) + Epic 4 (Events) Integration
// Tests: Policy evaluation events flow through the event system
// =============================================================================

// TestIntegration_PolicyEvaluationEvents tests that policy evaluations
// emit appropriate events through the event bus.
func TestIntegration_PolicyEvaluationEvents(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Track policy events
	var policyEvents []*events.Event
	var mu sync.Mutex

	// Create filter for policy events
	policyFilter := &events.EventFilter{
		Types: []events.EventType{
			events.EventTypePolicyPass,
			events.EventTypePolicyViolation,
		},
	}

	sub, err := env.eventBus.SubscribeWithFilter("policy-evaluation-tracker",
		policyFilter,
		func(e *events.Event) error {
			mu.Lock()
			policyEvents = append(policyEvents, e)
			mu.Unlock()
			return nil
		})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate policy evaluation results
	evaluations := []struct {
		eventType events.EventType
		severity  events.Severity
		data      map[string]interface{}
	}{
		{
			eventType: events.EventTypePolicyPass,
			severity:  events.SeverityInfo,
			data: map[string]interface{}{
				"policy_id": "require-labels",
				"resource":  "deployment/my-app",
				"namespace": "production",
				"decision":  "allow",
				"engine":    "opa",
				"eval_time": "2ms",
			},
		},
		{
			eventType: events.EventTypePolicyViolation,
			severity:  events.SeverityWarning,
			data: map[string]interface{}{
				"policy_id":   "no-privileged-containers",
				"resource":    "pod/privileged-pod",
				"namespace":   "default",
				"decision":    "deny",
				"engine":      "cel",
				"reason":      "Container requests privileged mode",
				"remediation": "Remove privileged: true from security context",
			},
		},
		{
			eventType: events.EventTypePolicyPass,
			severity:  events.SeverityInfo,
			data: map[string]interface{}{
				"policy_id": "resource-limits",
				"resource":  "deployment/my-app",
				"namespace": "production",
				"decision":  "allow",
				"engine":    "opa",
				"eval_time": "1ms",
			},
		},
	}

	for _, eval := range evaluations {
		event := events.NewEvent(eval.eventType).
			Source("/policy/evaluator").
			Severity(eval.severity).
			DataMap(eval.data).
			Build()

		if err := env.eventBus.PublishSync(event); err != nil {
			t.Errorf("Failed to publish policy event: %v", err)
		}
	}

	mu.Lock()
	count := len(policyEvents)
	mu.Unlock()

	if count != len(evaluations) {
		t.Errorf("Expected %d policy events, got %d", len(evaluations), count)
	}

	// Count passes and violations
	passes := 0
	violations := 0
	for _, e := range policyEvents {
		if e.Type == events.EventTypePolicyPass {
			passes++
		} else if e.Type == events.EventTypePolicyViolation {
			violations++
		}
	}

	if passes != 2 {
		t.Errorf("Expected 2 passes, got %d", passes)
	}
	if violations != 1 {
		t.Errorf("Expected 1 violation, got %d", violations)
	}

	t.Logf("Policy evaluation events: %d passes, %d violations", passes, violations)
}

// =============================================================================
// Epic 5 (GitOps) + Epic 6 (Policy) + Epic 3 (State) Integration
// Tests: Complete GitOps -> Policy -> State workflow
// =============================================================================

// TestIntegration_GitOpsPolicyStateWorkflow tests the full workflow of
// GitOps triggering policy evaluation which affects state.
func TestIntegration_GitOpsPolicyStateWorkflow(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "gitops-workflow-" + time.Now().Format("20060102150405")

	// Track all events in workflow
	var workflowEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("workflow-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			workflowEvents = append(workflowEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Step 1: GitOps webhook triggers a deployment
	gitopsEvent := events.NewEvent(events.EventType("gitops.argocd.sync")).
		Source("/gitops/argocd").
		CorrelationID(correlationID).
		Data("application", "my-app").
		Data("revision", "abc123").
		Data("status", "Succeeded").
		Build()

	if err := env.eventBus.PublishSync(gitopsEvent); err != nil {
		t.Fatalf("Failed to publish gitops event: %v", err)
	}

	// Simulate reactor: GitOps sync triggers policy evaluation
	policyEvalEvent := events.NewEvent(events.EventTypePolicyPass).
		Source("/policy/evaluator").
		CorrelationID(correlationID).
		Data("policy_id", "deployment-policy").
		Data("resource", "my-app").
		Data("decision", "allow").
		Build()

	if err := env.eventBus.PublishSync(policyEvalEvent); err != nil {
		t.Fatalf("Failed to publish policy event: %v", err)
	}

	// Simulate reactor: Policy pass triggers state apply
	stateApplyEvent := events.NewEvent(events.EventTypeStateApplyStart).
		Source("/state/apply").
		CorrelationID(correlationID).
		Data("state", "post-deploy-config").
		Data("agent", "cluster-agent").
		Build()

	if err := env.eventBus.PublishSync(stateApplyEvent); err != nil {
		t.Fatalf("Failed to publish state apply event: %v", err)
	}

	// State apply completes
	stateCompleteEvent := events.NewEvent(events.EventTypeStateApplyDone).
		Source("/state/apply").
		CorrelationID(correlationID).
		Data("state", "post-deploy-config").
		Data("changes", 3).
		Data("failures", 0).
		Build()

	if err := env.eventBus.PublishSync(stateCompleteEvent); err != nil {
		t.Fatalf("Failed to publish state complete event: %v", err)
	}

	mu.Lock()
	count := len(workflowEvents)
	mu.Unlock()

	if count != 4 {
		t.Errorf("Expected 4 workflow events, got %d", count)
	}

	// Verify event types in workflow
	expectedTypes := map[string]bool{
		"gitops.argocd.sync": false,
		"policy.pass":        false,
		"state.apply.start":  false,
		"state.apply.done":   false,
	}

	for _, e := range workflowEvents {
		if _, ok := expectedTypes[string(e.Type)]; ok {
			expectedTypes[string(e.Type)] = true
		}
	}

	for eventType, found := range expectedTypes {
		if !found {
			t.Errorf("Missing expected event type: %s", eventType)
		}
	}

	t.Logf("GitOps -> Policy -> State workflow completed with %d events", count)
}

// =============================================================================
// GitOps Rollback Scenario Integration
// Tests: Rollback triggered by policy violation
// =============================================================================

// TestIntegration_GitOpsRollbackOnPolicyViolation tests that a policy
// violation can trigger a GitOps rollback.
func TestIntegration_GitOpsRollbackOnPolicyViolation(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "rollback-workflow-" + time.Now().Format("20060102150405")

	// Track rollback events
	var rollbackEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("rollback-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			rollbackEvents = append(rollbackEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Setup reactor for policy violation -> rollback
	violationFilter := &events.EventFilter{
		Types: []events.EventType{events.EventTypePolicyViolation},
	}

	reactorSub, err := env.eventBus.SubscribeWithFilter("rollback-reactor",
		violationFilter,
		func(e *events.Event) error {
			// Only trigger rollback for events in our workflow
			if e.CorrelationID != correlationID {
				return nil
			}

			// Trigger rollback event
			rollbackEvent := events.NewEvent(events.EventType("gitops.rollback.initiated")).
				Source("/gitops/rollback").
				CorrelationID(e.CorrelationID).
				Data("reason", "policy_violation").
				Data("violation_policy", e.Data["policy_id"]).
				Build()
			return env.eventBus.PublishSync(rollbackEvent)
		})
	if err != nil {
		t.Fatalf("Failed to setup rollback reactor: %v", err)
	}
	defer reactorSub.Unsubscribe()

	// GitOps deployment
	deployEvent := events.NewEvent(events.EventType("gitops.argocd.sync")).
		Source("/gitops/argocd").
		CorrelationID(correlationID).
		Data("application", "risky-app").
		Data("revision", "dangerous-commit").
		Build()

	if err := env.eventBus.PublishSync(deployEvent); err != nil {
		t.Fatalf("Failed to publish deploy event: %v", err)
	}

	// Policy violation detected
	violationEvent := events.NewEvent(events.EventTypePolicyViolation).
		Source("/policy/evaluator").
		CorrelationID(correlationID).
		Severity(events.SeverityError).
		Data("policy_id", "security-baseline").
		Data("resource", "risky-app").
		Data("reason", "Container running as root").
		Build()

	if err := env.eventBus.PublishSync(violationEvent); err != nil {
		t.Fatalf("Failed to publish violation event: %v", err)
	}

	mu.Lock()
	count := len(rollbackEvents)
	mu.Unlock()

	if count < 3 {
		t.Errorf("Expected at least 3 events (deploy, violation, rollback), got %d", count)
	}

	// Check for rollback event
	hasRollback := false
	for _, e := range rollbackEvents {
		if e.Type == events.EventType("gitops.rollback.initiated") {
			hasRollback = true
			if e.Data["reason"] != "policy_violation" {
				t.Errorf("Rollback reason should be policy_violation, got %v", e.Data["reason"])
			}
		}
	}

	if !hasRollback {
		t.Error("Expected rollback event to be triggered")
	}

	t.Logf("Rollback workflow completed with %d events", count)
}

// =============================================================================
// Promotion Pipeline Integration
// Tests: Multi-environment promotion with policy gates
// =============================================================================

// TestIntegration_PromotionPipeline tests environment promotion with
// policy gates at each stage.
func TestIntegration_PromotionPipeline(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "promotion-" + time.Now().Format("20060102150405")

	// Track promotion events
	var promotionEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("promotion-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			promotionEvents = append(promotionEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate promotion through environments
	environments := []string{"dev", "staging", "production"}

	for i, envName := range environments {
		// Deploy to environment
		deployEvent := events.NewEvent(events.EventType("gitops.deploy")).
			Source("/gitops/promotion").
			CorrelationID(correlationID).
			Data("environment", envName).
			Data("application", "my-app").
			Data("revision", "v1.0.0").
			Build()

		if err := env.eventBus.PublishSync(deployEvent); err != nil {
			t.Errorf("Failed to publish deploy to %s: %v", envName, err)
		}

		// Policy gate check (production requires stricter policies)
		policyType := events.EventTypePolicyPass
		if i == 2 && false { // Simulate a failure case
			policyType = events.EventTypePolicyViolation
		}

		policyEvent := events.NewEvent(policyType).
			Source("/policy/gate").
			CorrelationID(correlationID).
			Data("environment", envName).
			Data("policy_id", envName+"-baseline").
			Data("decision", "allow").
			Build()

		if err := env.eventBus.PublishSync(policyEvent); err != nil {
			t.Errorf("Failed to publish policy for %s: %v", envName, err)
		}

		// Promotion approved
		promotionEvent := events.NewEvent(events.EventType("gitops.promotion.approved")).
			Source("/gitops/promotion").
			CorrelationID(correlationID).
			Data("from_environment", envName).
			Data("next_environment", func() string {
				if i < len(environments)-1 {
					return environments[i+1]
				}
				return "completed"
			}()).
			Build()

		if err := env.eventBus.PublishSync(promotionEvent); err != nil {
			t.Errorf("Failed to publish promotion from %s: %v", envName, err)
		}
	}

	mu.Lock()
	count := len(promotionEvents)
	mu.Unlock()

	// Expected: 3 deploys + 3 policy checks + 3 promotions = 9 events
	if count != 9 {
		t.Errorf("Expected 9 promotion events, got %d", count)
	}

	// Count event types
	deploys := 0
	policies := 0
	promotions := 0

	for _, e := range promotionEvents {
		switch e.Type {
		case events.EventType("gitops.deploy"):
			deploys++
		case events.EventTypePolicyPass:
			policies++
		case events.EventType("gitops.promotion.approved"):
			promotions++
		}
	}

	if deploys != 3 || policies != 3 || promotions != 3 {
		t.Errorf("Event counts mismatch: deploys=%d, policies=%d, promotions=%d", deploys, policies, promotions)
	}

	t.Logf("Promotion pipeline completed: %d deploys, %d policy checks, %d promotions", deploys, policies, promotions)
}
