package webhook

import (
	"net/http"
	"testing"
)

func TestFluxHandler(t *testing.T) {
	handler := &FluxHandler{}

	if handler.Type() != WebhookTypeFlux {
		t.Errorf("Type() = %v, want %v", handler.Type(), WebhookTypeFlux)
	}

	payload := `{
		"involvedObject": {
			"kind": "HelmRelease",
			"name": "test-app",
			"namespace": "production",
			"apiVersion": "helm.toolkit.fluxcd.io/v2beta1"
		},
		"severity": "info",
		"timestamp": "2025-01-01T00:00:00Z",
		"message": "Helm release reconciled successfully",
		"reason": "ReconciliationSucceeded",
		"metadata": {
			"revision": "v1.2.3"
		},
		"reportingController": "helm-controller",
		"reportingInstance": "flux-system"
	}`

	req := &http.Request{
		Header: http.Header{
			"X-Flux-Event": []string{"reconciliation"},
		},
	}

	event, err := handler.Parse(req, []byte(payload))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Validate event
	if event.Type != WebhookTypeFlux {
		t.Errorf("event.Type = %v, want %v", event.Type, WebhookTypeFlux)
	}

	if event.EventType != "reconciliation" {
		t.Errorf("event.EventType = %v, want %v", event.EventType, "reconciliation")
	}

	if event.Application != "test-app" {
		t.Errorf("event.Application = %v, want %v", event.Application, "test-app")
	}

	if event.Namespace != "production" {
		t.Errorf("event.Namespace = %v, want %v", event.Namespace, "production")
	}

	if event.Revision != "v1.2.3" {
		t.Errorf("event.Revision = %v, want %v", event.Revision, "v1.2.3")
	}

	if event.Status != "info" {
		t.Errorf("event.Status = %v, want %v", event.Status, "info")
	}

	// Validate data
	if event.Data["kind"] != "HelmRelease" {
		t.Errorf("event.Data[kind] = %v, want %v", event.Data["kind"], "HelmRelease")
	}

	if event.Data["message"] != "Helm release reconciled successfully" {
		t.Errorf("event.Data[message] = %v", event.Data["message"])
	}

	// Test conversion to KscoreEvent
	kscoreEvent := event.ToKscoreEvent()
	if kscoreEvent == nil {
		t.Fatal("ToKscoreEvent() returned nil")
	}

	if kscoreEvent.Type != "gitops.flux.reconciliation" {
		t.Errorf("kscoreEvent.Type = %v, want %v", kscoreEvent.Type, "gitops.flux.reconciliation")
	}

	if kscoreEvent.Source != "webhook/flux" {
		t.Errorf("kscoreEvent.Source = %v, want %v", kscoreEvent.Source, "webhook/flux")
	}
}

func TestFluxHandlerFallbackEventType(t *testing.T) {
	handler := &FluxHandler{}

	payload := `{
		"involvedObject": {
			"kind": "Kustomization",
			"name": "infrastructure",
			"namespace": "flux-system"
		},
		"severity": "error",
		"timestamp": "2025-01-01T00:00:00Z",
		"message": "Reconciliation failed",
		"reason": "ReconciliationFailed"
	}`

	req := &http.Request{
		Header: http.Header{},
	}

	event, err := handler.Parse(req, []byte(payload))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// When X-Flux-Event header is missing, should use reason
	if event.EventType != "ReconciliationFailed" {
		t.Errorf("event.EventType = %v, want %v", event.EventType, "ReconciliationFailed")
	}
}

func TestFluxHandlerInvalidPayload(t *testing.T) {
	handler := &FluxHandler{}
	req := &http.Request{
		Header: http.Header{},
	}

	_, err := handler.Parse(req, []byte("invalid json"))
	if err == nil {
		t.Error("Parse() expected error for invalid JSON, got nil")
	}
}
