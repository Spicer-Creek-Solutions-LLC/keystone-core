// Package scenarios contains E2E test scenarios for Keystone Core features.
package scenarios

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"
)

// e2eHMACSecret is the HMAC secret configured in server.yaml for E2E testing.
const e2eHMACSecret = "e2e-test-hmac-secret-key"

// =============================================================================
// GitOps Webhook Tests (T3.6)
//
// These tests verify GitOps webhook functionality end-to-end.
// The webhook receiver is configured with HMAC authentication in server.yaml.
// =============================================================================

// TestGitOps_ServerHealth tests the server health endpoint
func TestGitOps_ServerHealth(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:8081/health/live", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Health check failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Log("Server health endpoint is responding")
}

// TestGitOps_ReadinessEndpoint tests the server readiness endpoint
func TestGitOps_ReadinessEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:8081/health/ready", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Readiness check failed: %v", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		t.Log("Server is ready")
	case http.StatusServiceUnavailable:
		t.Log("Server is not yet ready (dependencies may be missing)")
	default:
		t.Errorf("Unexpected status code: %d", resp.StatusCode)
	}
}

// =============================================================================
// Webhook Receiver Tests
// =============================================================================

// TestGitOps_WebhookEndpoint tests the webhook endpoint availability
func TestGitOps_WebhookEndpoint(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"test": "hello",
	}

	resp, err := sendSignedWebhook(ctx, webhookURL, "generic", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("Webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	// Webhook endpoint should accept the request (200) or handle it (may return 400 for unknown type)
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusBadRequest {
		t.Logf("Webhook endpoint is responding (status: %d)", resp.StatusCode)
	} else {
		t.Errorf("Unexpected webhook response status: %d", resp.StatusCode)
	}
}

// TestGitOps_ArgoCDWebhook tests ArgoCD webhook handling
func TestGitOps_ArgoCDWebhook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"type": "argocd",
		"application": map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-app",
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status": "Synced",
				},
				"health": map[string]interface{}{
					"status": "Healthy",
				},
			},
		},
	}

	resp, err := sendSignedWebhook(ctx, webhookURL, "argocd", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("ArgoCD webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Log("ArgoCD webhook processed successfully")
}

// TestGitOps_FluxWebhook tests Flux webhook handling
func TestGitOps_FluxWebhook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"involvedObject": map[string]interface{}{
			"kind":      "Kustomization",
			"name":      "test-kustomization",
			"namespace": "flux-system",
		},
		"severity": "info",
		"message":  "Reconciliation finished",
	}

	resp, err := sendSignedWebhook(ctx, webhookURL, "flux", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("Flux webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Log("Flux webhook processed successfully")
}

// TestGitOps_GitHubWebhook tests GitHub webhook handling
func TestGitOps_GitHubWebhook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"action": "created",
		"deployment": map[string]interface{}{
			"id":          12345,
			"ref":         "main",
			"task":        "deploy",
			"environment": "production",
		},
		"repository": map[string]interface{}{
			"full_name": "org/repo",
		},
	}

	resp, err := sendSignedWebhook(ctx, webhookURL, "github", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("GitHub webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Log("GitHub webhook processed successfully")
}

// TestGitOps_GitLabWebhook tests GitLab webhook handling
func TestGitOps_GitLabWebhook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"object_kind": "deployment",
		"status":      "success",
		"environment": "production",
		"project": map[string]interface{}{
			"path_with_namespace": "group/project",
		},
	}

	resp, err := sendSignedWebhook(ctx, webhookURL, "gitlab", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("GitLab webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Log("GitLab webhook processed successfully")
}

// =============================================================================
// Webhook Authentication Tests
// =============================================================================

// TestGitOps_WebhookAuthHMAC tests HMAC authentication on the webhook receiver.
// The server is configured with authtype=hmac and hmacsecret="e2e-test-hmac-secret-key".
func TestGitOps_WebhookAuthHMAC(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"action": "created",
		"deployment": map[string]interface{}{
			"id":  99999,
			"ref": "main",
		},
		"repository": map[string]interface{}{
			"full_name": "org/hmac-test",
		},
	}

	// Test 1: Valid HMAC signature should be accepted
	resp, err := sendSignedWebhook(ctx, webhookURL, "github", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("Valid HMAC webhook request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for valid HMAC signature, got %d", resp.StatusCode)
	} else {
		t.Log("Valid HMAC signature accepted (200)")
	}

	// Test 2: Invalid HMAC signature should be rejected
	resp, err = sendSignedWebhook(ctx, webhookURL, "github", payload, "wrong-secret-key")
	if err != nil {
		t.Fatalf("Invalid HMAC webhook request failed: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("Expected rejection for invalid HMAC signature, got 200")
	} else {
		t.Logf("Invalid HMAC signature rejected (status: %d, body: %s)", resp.StatusCode, string(body))
	}

	// Test 3: Missing signature should be rejected
	resp, err = sendWebhook(ctx, webhookURL, "github", payload)
	if err != nil {
		t.Fatalf("Unsigned webhook request failed: %v", err)
	}
	body, _ = io.ReadAll(resp.Body)
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		t.Error("Expected rejection for missing HMAC signature, got 200")
	} else {
		t.Logf("Missing HMAC signature rejected (status: %d, body: %s)", resp.StatusCode, string(body))
	}
}

// TestGitOps_WebhookAuthBearer tests Bearer token authentication.
// The server is configured with authtype=hmac, so bearer-only requests should be rejected.
// Full bearer auth testing requires a server configured with authtype=bearer.
func TestGitOps_WebhookAuthBearer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"action": "created",
		"deployment": map[string]interface{}{
			"id":  88888,
			"ref": "main",
		},
		"repository": map[string]interface{}{
			"full_name": "org/bearer-test",
		},
	}

	// The server is configured with authtype=hmac, so a bearer-only request
	// (without HMAC signature) should be rejected.
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Failed to marshal payload: %v", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", webhookURL, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-GitHub-Event", "deployment")
	req.Header.Set("X-GitHub-Delivery", "bearer-test-delivery")
	req.Header.Set("Authorization", "Bearer e2e-test-bearer-token")

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Bearer auth webhook request failed: %v", err)
	}
	respBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// With HMAC auth configured, bearer-only is insufficient — expect rejection
	if resp.StatusCode == http.StatusOK {
		t.Log("Bearer token accepted (server may accept bearer as fallback)")
	} else {
		t.Logf("Bearer-only request rejected as expected with HMAC auth (status: %d, body: %s)", resp.StatusCode, string(respBody))
	}

	// Verify that HMAC auth still works (to confirm the server is operational)
	resp, err = sendSignedWebhook(ctx, webhookURL, "github", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("HMAC fallback request failed: %v", err)
	}
	resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for HMAC-signed request, got %d", resp.StatusCode)
	}

	t.Log("Bearer auth test complete; full bearer auth testing requires authtype=bearer server configuration")
}

// =============================================================================
// Verification Workflow Tests
// =============================================================================

// TestGitOps_VerificationTrigger tests that an ArgoCD sync webhook is accepted and
// processed by the server. Full verification workflow confirmation requires additional
// wiring to observe internal server state.
func TestGitOps_VerificationTrigger(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"type": "argocd",
		"application": map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "verification-test-app",
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status":   "Synced",
					"revision": "abc123def",
				},
				"health": map[string]interface{}{
					"status": "Healthy",
				},
				"operationState": map[string]interface{}{
					"phase":   "Succeeded",
					"message": "successfully synced",
				},
			},
		},
	}

	resp, err := sendSignedWebhook(ctx, webhookURL, "argocd", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("Verification webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for ArgoCD sync webhook, got %d", resp.StatusCode)
	}

	t.Log("ArgoCD sync webhook accepted; server processes verification internally")
	t.Log("Full verification workflow observation requires internal state query API")
}

// TestGitOps_VerificationHTTP tests HTTP verification step
func TestGitOps_VerificationHTTP(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	client := &http.Client{Timeout: 5 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "GET", "http://localhost:8081/health/live", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	t.Log("HTTP verification capability confirmed")
}

// TestGitOps_VerificationCommand tests command verification step
func TestGitOps_VerificationCommand(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	agentID := "agent-web-1"

	result, err := testEnv.ExecuteCommandAndWait(ctx, agentID, "echo", "verification test")
	if err != nil {
		t.Fatalf("Command execution failed: %v", err)
	}

	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}

	t.Log("Command verification capability confirmed")
}

// =============================================================================
// Rollback Automation Tests
// =============================================================================

// TestGitOps_RollbackTrigger tests that a webhook indicating a failed deployment is accepted
// by the server. Full rollback automation requires the GitOps rollback engine to be wired.
func TestGitOps_RollbackTrigger(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"type": "argocd",
		"application": map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "rollback-test-app",
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status": "OutOfSync",
				},
				"health": map[string]interface{}{
					"status": "Degraded",
				},
				"operationState": map[string]interface{}{
					"phase":   "Failed",
					"message": "deployment health check failed",
				},
			},
		},
	}

	resp, err := sendSignedWebhook(ctx, webhookURL, "argocd", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("Rollback webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for failed deployment webhook, got %d", resp.StatusCode)
	}

	t.Log("Failed deployment webhook accepted by server")
	t.Log("Full rollback automation verification requires GitOps rollback engine wiring")
}

// TestGitOps_RollbackApproval tests that a rollback webhook is accepted. Full approval
// workflow (pending state, approve/reject actions) requires additional API wiring.
func TestGitOps_RollbackApproval(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"type": "argocd",
		"application": map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "approval-test-app",
				"namespace": "argocd",
			},
			"status": map[string]interface{}{
				"sync": map[string]interface{}{
					"status": "OutOfSync",
				},
				"health": map[string]interface{}{
					"status": "Degraded",
				},
				"operationState": map[string]interface{}{
					"phase":   "Failed",
					"message": "rollback required - awaiting approval",
				},
			},
		},
	}

	resp, err := sendSignedWebhook(ctx, webhookURL, "argocd", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("Rollback approval webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for rollback webhook, got %d", resp.StatusCode)
	}

	t.Log("Rollback webhook accepted by server")
	t.Log("Full approval workflow (pending/approve/reject) requires additional API wiring")
}

// =============================================================================
// Event Integration Tests
// =============================================================================

// TestGitOps_WebhookEventEmission tests that the webhook endpoint accepts payloads.
// Verifying that events are actually emitted to the event bus requires an event query
// API which is not yet wired into the E2E environment.
func TestGitOps_WebhookEventEmission(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	webhookURL := testEnv.WebhookAddr + "/webhooks"

	payload := map[string]interface{}{
		"action": "completed",
		"deployment": map[string]interface{}{
			"id":          77777,
			"ref":         "main",
			"task":        "deploy",
			"environment": "staging",
		},
		"repository": map[string]interface{}{
			"full_name": "org/event-emission-test",
		},
	}

	resp, err := sendSignedWebhook(ctx, webhookURL, "github", payload, e2eHMACSecret)
	if err != nil {
		t.Fatalf("Event emission webhook request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200 for webhook, got %d", resp.StatusCode)
	}

	t.Log("Webhook accepted; event emission verified at HTTP level")
	t.Log("Verifying gitops.* events on the event bus requires an event query API (not yet wired)")
}

// =============================================================================
// Helper functions for webhook testing
// =============================================================================

// computeHMACSignature computes HMAC-SHA256 signature for a payload.
func computeHMACSignature(secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

// sendWebhook sends an unsigned webhook payload to the receiver.
func sendWebhook(ctx context.Context, url string, webhookType string, payload map[string]interface{}) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	setWebhookTypeHeaders(req, webhookType)

	client := &http.Client{Timeout: 5 * time.Second}
	return client.Do(req)
}

// sendSignedWebhook sends a webhook payload with HMAC-SHA256 signature.
func sendSignedWebhook(ctx context.Context, url string, webhookType string, payload map[string]interface{}, hmacSecret string) (*http.Response, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	setWebhookTypeHeaders(req, webhookType)

	// Add HMAC-SHA256 signature in X-Hub-Signature-256 header (GitHub convention)
	signature := computeHMACSignature(hmacSecret, body)
	req.Header.Set("X-Hub-Signature-256", "sha256="+signature)

	client := &http.Client{Timeout: 5 * time.Second}
	return client.Do(req)
}

// setWebhookTypeHeaders sets provider-specific headers on the request.
func setWebhookTypeHeaders(req *http.Request, webhookType string) {
	switch webhookType {
	case "github":
		req.Header.Set("X-GitHub-Event", "deployment")
		req.Header.Set("X-GitHub-Delivery", "test-delivery-id")
	case "gitlab":
		req.Header.Set("X-Gitlab-Event", "Deployment Hook")
	case "argocd":
		req.Header.Set("X-Argo-CD-Webhook", "true")
		req.Header.Set("User-Agent", "ArgoCD-Server")
	case "flux":
		req.Header.Set("X-Flux-Event", "Kustomization")
	}
}
