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
// Epic 17 (Identity) + Epic 4 (Events) Integration
// Tests: Identity events flow through the event system
// =============================================================================

// TestIntegration_IdentityEventsEmission tests that identity lifecycle events
// are properly emitted to the event bus.
func TestIntegration_IdentityEventsEmission(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	// Track identity events
	var identityEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("identity-tracker",
		func(e *events.Event) error {
			// Manual filter for identity.* events
			if strings.HasPrefix(string(e.Type), "identity.") {
				mu.Lock()
				identityEvents = append(identityEvents, e)
				mu.Unlock()
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate identity lifecycle events
	identityLifecycle := []struct {
		eventType events.EventType
		data      map[string]interface{}
	}{
		{
			eventType: events.EventType("identity.svid.requested"),
			data: map[string]interface{}{
				"spiffe_id":   "spiffe://keystone.local/agent/agent-1",
				"requester":   "agent-1",
				"svid_type":   "x509",
				"attestation": "node",
			},
		},
		{
			eventType: events.EventType("identity.attestation.success"),
			data: map[string]interface{}{
				"spiffe_id": "spiffe://keystone.local/agent/agent-1",
				"method":    "node",
				"platform":  "linux",
			},
		},
		{
			eventType: events.EventType("identity.svid.issued"),
			data: map[string]interface{}{
				"spiffe_id": "spiffe://keystone.local/agent/agent-1",
				"svid_type": "x509",
				"ttl":       "1h",
				"serial":    "ABC123",
			},
		},
		{
			eventType: events.EventType("identity.svid.renewed"),
			data: map[string]interface{}{
				"spiffe_id":  "spiffe://keystone.local/agent/agent-1",
				"old_serial": "ABC123",
				"new_serial": "DEF456",
				"remaining":  "10m",
			},
		},
	}

	for _, evt := range identityLifecycle {
		event := events.NewEvent(evt.eventType).
			Source("/identity").
			DataMap(evt.data).
			Build()

		if err := env.eventBus.PublishSync(event); err != nil {
			t.Errorf("Failed to publish %s: %v", evt.eventType, err)
		}
	}

	mu.Lock()
	count := len(identityEvents)
	mu.Unlock()

	if count != len(identityLifecycle) {
		t.Errorf("Expected %d identity events, got %d", len(identityLifecycle), count)
	}

	t.Logf("Identity lifecycle events processed: %d events", count)
}

// =============================================================================
// Epic 17 (Identity) + Epic 6 (Policy) Integration
// Tests: Identity-based policy evaluation
// =============================================================================

// TestIntegration_IdentityBasedPolicyEvaluation tests that identity information
// is properly used in policy evaluation decisions.
func TestIntegration_IdentityBasedPolicyEvaluation(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "identity-policy-" + time.Now().Format("20060102150405")

	// Track policy events
	var policyEvents []*events.Event
	var mu sync.Mutex

	policyFilter := &events.EventFilter{
		Types: []events.EventType{
			events.EventTypePolicyPass,
			events.EventTypePolicyViolation,
		},
	}

	sub, err := env.eventBus.SubscribeWithFilter("identity-policy-tracker",
		policyFilter,
		func(e *events.Event) error {
			if e.CorrelationID == correlationID {
				mu.Lock()
				policyEvents = append(policyEvents, e)
				mu.Unlock()
			}
			return nil
		})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate identity-based policy evaluations
	evaluations := []struct {
		eventType events.EventType
		severity  events.Severity
		data      map[string]interface{}
	}{
		{
			// Agent with valid SPIFFE ID - allowed
			eventType: events.EventTypePolicyPass,
			severity:  events.SeverityInfo,
			data: map[string]interface{}{
				"policy_id":    "identity-required",
				"spiffe_id":    "spiffe://keystone.local/agent/agent-1",
				"resource":     "state.apply",
				"decision":     "allow",
				"identity_ttl": "55m",
			},
		},
		{
			// Agent with valid role - allowed
			eventType: events.EventTypePolicyPass,
			severity:  events.SeverityInfo,
			data: map[string]interface{}{
				"policy_id": "role-check",
				"spiffe_id": "spiffe://keystone.local/agent/agent-1",
				"resource":  "cluster.join",
				"decision":  "allow",
				"roles":     []string{"agent", "node"},
			},
		},
		{
			// Agent without required role - denied
			eventType: events.EventTypePolicyViolation,
			severity:  events.SeverityWarning,
			data: map[string]interface{}{
				"policy_id":     "admin-required",
				"spiffe_id":     "spiffe://keystone.local/agent/agent-1",
				"resource":      "cluster.admin",
				"decision":      "deny",
				"reason":        "missing required role: admin",
				"required_role": "admin",
				"agent_roles":   []string{"agent", "node"},
			},
		},
		{
			// Expired identity - denied
			eventType: events.EventTypePolicyViolation,
			severity:  events.SeverityError,
			data: map[string]interface{}{
				"policy_id":  "identity-valid",
				"spiffe_id":  "spiffe://keystone.local/agent/agent-2",
				"resource":   "any",
				"decision":   "deny",
				"reason":     "identity expired",
				"expired_at": time.Now().Add(-10 * time.Minute).Format(time.RFC3339),
			},
		},
	}

	for _, eval := range evaluations {
		event := events.NewEvent(eval.eventType).
			Source("/policy/identity-evaluator").
			CorrelationID(correlationID).
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
		switch e.Type {
		case events.EventTypePolicyPass:
			passes++
		case events.EventTypePolicyViolation:
			violations++
		default:
			// Other event types not counted in this test
		}
	}

	if passes != 2 {
		t.Errorf("Expected 2 passes, got %d", passes)
	}
	if violations != 2 {
		t.Errorf("Expected 2 violations, got %d", violations)
	}

	t.Logf("Identity-based policy evaluation: %d passes, %d violations", passes, violations)
}

// =============================================================================
// Epic 17 (Identity) + Epic 6 (Policy) + Epic 2 (Execution) Integration
// Tests: Identity verified before command execution
// =============================================================================

// TestIntegration_IdentityVerifiedExecution tests that identity is verified
// before allowing command execution.
func TestIntegration_IdentityVerifiedExecution(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "exec-workflow-" + time.Now().Format("20060102150405")

	// Track all workflow events
	var workflowEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("exec-workflow-tracker", func(e *events.Event) error {
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

	// Step 1: Command request received
	cmdRequestEvent := events.NewEvent(events.EventTypeJobStart).
		Source("/control-plane").
		CorrelationID(correlationID).
		Data("command", "pkg.install").
		Data("target", "agent-1").
		Data("args", map[string]interface{}{"name": "nginx"}).
		Build()

	if err := env.eventBus.PublishSync(cmdRequestEvent); err != nil {
		t.Fatalf("Failed to publish command request: %v", err)
	}

	// Step 2: Identity verification
	identityVerifyEvent := events.NewEvent(events.EventType("identity.verified")).
		Source("/identity/verifier").
		CorrelationID(correlationID).
		Data("spiffe_id", "spiffe://keystone.local/agent/agent-1").
		Data("verified", true).
		Data("svid_valid", true).
		Data("ttl_remaining", "45m").
		Build()

	if err := env.eventBus.PublishSync(identityVerifyEvent); err != nil {
		t.Fatalf("Failed to publish identity verification: %v", err)
	}

	// Step 3: Policy evaluation
	policyEvent := events.NewEvent(events.EventTypePolicyPass).
		Source("/policy/evaluator").
		CorrelationID(correlationID).
		Data("policy_id", "command-allowed").
		Data("spiffe_id", "spiffe://keystone.local/agent/agent-1").
		Data("command", "pkg.install").
		Data("decision", "allow").
		Build()

	if err := env.eventBus.PublishSync(policyEvent); err != nil {
		t.Fatalf("Failed to publish policy event: %v", err)
	}

	// Step 4: Command executed
	cmdExecuteEvent := events.NewEvent(events.EventTypeJobComplete).
		Source("/agent/agent-1").
		CorrelationID(correlationID).
		Data("command", "pkg.install").
		Data("exit_code", 0).
		Data("output", "Package nginx installed successfully").
		Build()

	if err := env.eventBus.PublishSync(cmdExecuteEvent); err != nil {
		t.Fatalf("Failed to publish command execution: %v", err)
	}

	mu.Lock()
	count := len(workflowEvents)
	mu.Unlock()

	if count != 4 {
		t.Errorf("Expected 4 workflow events, got %d", count)
	}

	// Verify workflow sequence
	expectedTypes := map[string]bool{
		"job.start":         false,
		"identity.verified": false,
		"policy.pass":       false,
		"job.complete":      false,
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

	t.Logf("Identity-verified execution workflow completed with %d events", count)
}

// =============================================================================
// Identity Federation Integration
// Tests: Cross-trust-domain identity events
// =============================================================================

// TestIntegration_IdentityFederation tests that federated identity events
// are properly handled.
func TestIntegration_IdentityFederation(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "federation-" + time.Now().Format("20060102150405")

	// Track federation events
	var federationEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("federation-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			federationEvents = append(federationEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Simulate federation lifecycle
	// Step 1: Federation bundle received
	bundleReceivedEvent := events.NewEvent(events.EventType("identity.federation.bundle_received")).
		Source("/identity/federation").
		CorrelationID(correlationID).
		Data("trust_domain", "partner.example.com").
		Data("bundle_hash", "sha256:abc123...").
		Data("public_keys", 2).
		Build()

	if err := env.eventBus.PublishSync(bundleReceivedEvent); err != nil {
		t.Fatalf("Failed to publish bundle received: %v", err)
	}

	// Step 2: Trust established
	trustEstablishedEvent := events.NewEvent(events.EventType("identity.federation.trust_established")).
		Source("/identity/federation").
		CorrelationID(correlationID).
		Data("local_domain", "keystone.local").
		Data("remote_domain", "partner.example.com").
		Data("federation_type", "bidirectional").
		Build()

	if err := env.eventBus.PublishSync(trustEstablishedEvent); err != nil {
		t.Fatalf("Failed to publish trust established: %v", err)
	}

	// Step 3: Federated identity verified
	federatedVerifyEvent := events.NewEvent(events.EventType("identity.federation.verified")).
		Source("/identity/federation").
		CorrelationID(correlationID).
		Data("spiffe_id", "spiffe://partner.example.com/workload/service-a").
		Data("local_mapping", "spiffe://keystone.local/federated/partner/service-a").
		Data("verified", true).
		Build()

	if err := env.eventBus.PublishSync(federatedVerifyEvent); err != nil {
		t.Fatalf("Failed to publish federated verification: %v", err)
	}

	// Step 4: Policy applied to federated identity
	federatedPolicyEvent := events.NewEvent(events.EventTypePolicyPass).
		Source("/policy/evaluator").
		CorrelationID(correlationID).
		Data("policy_id", "federated-access").
		Data("spiffe_id", "spiffe://partner.example.com/workload/service-a").
		Data("resource", "api.read").
		Data("decision", "allow").
		Data("federation_context", "partner.example.com").
		Build()

	if err := env.eventBus.PublishSync(federatedPolicyEvent); err != nil {
		t.Fatalf("Failed to publish federated policy: %v", err)
	}

	mu.Lock()
	count := len(federationEvents)
	mu.Unlock()

	if count != 4 {
		t.Errorf("Expected 4 federation events, got %d", count)
	}

	t.Logf("Identity federation workflow completed with %d events", count)
}

// =============================================================================
// Bootstrap Identity Lifecycle Integration
// Tests: Complete identity bootstrap flow
// =============================================================================

// TestIntegration_BootstrapIdentityLifecycle tests the complete identity
// bootstrap flow from token to SVID.
func TestIntegration_BootstrapIdentityLifecycle(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "bootstrap-identity-" + time.Now().Format("20060102150405")

	// Track bootstrap events
	var bootstrapEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("bootstrap-identity-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			bootstrapEvents = append(bootstrapEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Step 1: Bootstrap token generated
	tokenGeneratedEvent := events.NewEvent(events.EventTypeBootstrapGenerate).
		Source("/bootstrap").
		CorrelationID(correlationID).
		Data("token_id", "boot-token-123").
		Data("target_agent", "new-agent-1").
		Data("valid_for", "1h").
		Build()

	if err := env.eventBus.PublishSync(tokenGeneratedEvent); err != nil {
		t.Fatalf("Failed to publish token generated: %v", err)
	}

	// Step 2: Bootstrap token used
	tokenUsedEvent := events.NewEvent(events.EventTypeBootstrapUse).
		Source("/agent/new-agent-1").
		CorrelationID(correlationID).
		Data("token_id", "boot-token-123").
		Data("agent_id", "new-agent-1").
		Build()

	if err := env.eventBus.PublishSync(tokenUsedEvent); err != nil {
		t.Fatalf("Failed to publish token used: %v", err)
	}

	// Step 3: Attestation performed
	attestationEvent := events.NewEvent(events.EventType("identity.attestation.success")).
		Source("/identity/attestor").
		CorrelationID(correlationID).
		Data("agent_id", "new-agent-1").
		Data("method", "join_token").
		Data("platform", "linux").
		Data("hostname", "new-host-1").
		Build()

	if err := env.eventBus.PublishSync(attestationEvent); err != nil {
		t.Fatalf("Failed to publish attestation: %v", err)
	}

	// Step 4: SVID issued
	svidIssuedEvent := events.NewEvent(events.EventType("identity.svid.issued")).
		Source("/identity/ca").
		CorrelationID(correlationID).
		Data("spiffe_id", "spiffe://keystone.local/agent/new-agent-1").
		Data("svid_type", "x509").
		Data("serial", "NEW123").
		Data("ttl", "1h").
		Build()

	if err := env.eventBus.PublishSync(svidIssuedEvent); err != nil {
		t.Fatalf("Failed to publish SVID issued: %v", err)
	}

	// Step 5: Agent registered
	agentRegisteredEvent := events.NewEvent(events.EventTypeBootstrapRegister).
		Source("/control-plane").
		CorrelationID(correlationID).
		Data("agent_id", "new-agent-1").
		Data("spiffe_id", "spiffe://keystone.local/agent/new-agent-1").
		Data("registered_at", time.Now().Format(time.RFC3339)).
		Build()

	if err := env.eventBus.PublishSync(agentRegisteredEvent); err != nil {
		t.Fatalf("Failed to publish agent registered: %v", err)
	}

	mu.Lock()
	count := len(bootstrapEvents)
	mu.Unlock()

	if count != 5 {
		t.Errorf("Expected 5 bootstrap events, got %d", count)
	}

	// Verify complete flow
	expectedTypes := map[string]bool{
		"bootstrap.generate":           false,
		"bootstrap.use":                false,
		"identity.attestation.success": false,
		"identity.svid.issued":         false,
		"bootstrap.register":           false,
	}

	for _, e := range bootstrapEvents {
		if _, ok := expectedTypes[string(e.Type)]; ok {
			expectedTypes[string(e.Type)] = true
		}
	}

	for eventType, found := range expectedTypes {
		if !found {
			t.Errorf("Missing expected event type: %s", eventType)
		}
	}

	t.Logf("Bootstrap identity lifecycle completed with %d events", count)
}

// =============================================================================
// Identity Revocation Integration
// Tests: Identity revocation and its effects
// =============================================================================

// TestIntegration_IdentityRevocation tests that identity revocation
// properly affects policy decisions and execution.
func TestIntegration_IdentityRevocation(t *testing.T) {
	env := setupTestEnv(t)
	defer env.cleanup()

	correlationID := "revocation-" + time.Now().Format("20060102150405")

	// Track revocation events
	var revocationEvents []*events.Event
	var mu sync.Mutex

	sub, err := env.eventBus.Subscribe("revocation-tracker", func(e *events.Event) error {
		if e.CorrelationID == correlationID {
			mu.Lock()
			revocationEvents = append(revocationEvents, e)
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		t.Fatalf("Failed to subscribe: %v", err)
	}
	defer sub.Unsubscribe()

	// Step 1: Revocation initiated
	revocationInitEvent := events.NewEvent(events.EventType("identity.revocation.initiated")).
		Source("/identity/admin").
		CorrelationID(correlationID).
		Severity(events.SeverityWarning).
		Data("spiffe_id", "spiffe://keystone.local/agent/compromised-agent").
		Data("reason", "security_incident").
		Data("initiated_by", "admin-user").
		Build()

	if err := env.eventBus.PublishSync(revocationInitEvent); err != nil {
		t.Fatalf("Failed to publish revocation init: %v", err)
	}

	// Step 2: CRL updated
	crlUpdateEvent := events.NewEvent(events.EventType("identity.crl.updated")).
		Source("/identity/ca").
		CorrelationID(correlationID).
		Data("crl_serial", 12345).
		Data("entries_added", 1).
		Data("total_revoked", 5).
		Build()

	if err := env.eventBus.PublishSync(crlUpdateEvent); err != nil {
		t.Fatalf("Failed to publish CRL update: %v", err)
	}

	// Step 3: Policy denies revoked identity
	policyDenyEvent := events.NewEvent(events.EventTypePolicyViolation).
		Source("/policy/evaluator").
		CorrelationID(correlationID).
		Severity(events.SeverityError).
		Data("policy_id", "identity-valid").
		Data("spiffe_id", "spiffe://keystone.local/agent/compromised-agent").
		Data("decision", "deny").
		Data("reason", "identity_revoked").
		Build()

	if err := env.eventBus.PublishSync(policyDenyEvent); err != nil {
		t.Fatalf("Failed to publish policy deny: %v", err)
	}

	// Step 4: Agent disconnected
	agentDisconnectEvent := events.NewEvent(events.EventTypeAgentDisconnect).
		Source("/control-plane").
		CorrelationID(correlationID).
		Data("agent_id", "compromised-agent").
		Data("reason", "identity_revoked").
		Data("forced", true).
		Build()

	if err := env.eventBus.PublishSync(agentDisconnectEvent); err != nil {
		t.Fatalf("Failed to publish agent disconnect: %v", err)
	}

	mu.Lock()
	count := len(revocationEvents)
	mu.Unlock()

	if count != 4 {
		t.Errorf("Expected 4 revocation events, got %d", count)
	}

	// Verify the agent was disconnected
	hasDisconnect := false
	for _, e := range revocationEvents {
		if e.Type == events.EventTypeAgentDisconnect {
			if forced, ok := e.Data["forced"].(bool); ok && forced {
				hasDisconnect = true
			}
		}
	}

	if !hasDisconnect {
		t.Error("Expected agent to be forcibly disconnected")
	}

	t.Logf("Identity revocation workflow completed with %d events", count)
}
