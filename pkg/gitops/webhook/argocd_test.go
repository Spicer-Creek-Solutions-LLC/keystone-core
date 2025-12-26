package webhook

import (
	"net/http"
	"testing"
)

func TestArgoCDHandler(t *testing.T) {
	handler := &ArgoCDHandler{}

	if handler.Type() != WebhookTypeArgoCD {
		t.Errorf("Type() = %v, want %v", handler.Type(), WebhookTypeArgoCD)
	}

	payload := `{
		"application": {
			"metadata": {
				"name": "test-app",
				"namespace": "argocd"
			},
			"spec": {
				"source": {
					"repoURL": "https://github.com/example/repo",
					"targetRevision": "main"
				}
			},
			"status": {
				"sync": {
					"status": "Synced",
					"revision": "abc123"
				},
				"health": {
					"status": "Healthy"
				},
				"operationState": {
					"phase": "Succeeded",
					"startedAt": "2025-01-01T00:00:00Z",
					"finishedAt": "2025-01-01T00:01:00Z"
				}
			}
		},
		"type": "sync"
	}`

	req := &http.Request{
		Header: http.Header{},
	}

	event, err := handler.Parse(req, []byte(payload))
	if err != nil {
		t.Fatalf("Parse() error = %v", err)
	}

	// Validate event
	if event.Type != WebhookTypeArgoCD {
		t.Errorf("event.Type = %v, want %v", event.Type, WebhookTypeArgoCD)
	}

	if event.EventType != "sync" {
		t.Errorf("event.EventType = %v, want %v", event.EventType, "sync")
	}

	if event.Application != "test-app" {
		t.Errorf("event.Application = %v, want %v", event.Application, "test-app")
	}

	if event.Namespace != "argocd" {
		t.Errorf("event.Namespace = %v, want %v", event.Namespace, "argocd")
	}

	if event.Revision != "abc123" {
		t.Errorf("event.Revision = %v, want %v", event.Revision, "abc123")
	}

	if event.Status != "Healthy" {
		t.Errorf("event.Status = %v, want %v", event.Status, "Healthy")
	}

	// Validate data
	if event.Data["repo_url"] != "https://github.com/example/repo" {
		t.Errorf("event.Data[repo_url] = %v, want %v", event.Data["repo_url"], "https://github.com/example/repo")
	}

	if event.Data["sync_status"] != "Synced" {
		t.Errorf("event.Data[sync_status] = %v, want %v", event.Data["sync_status"], "Synced")
	}

	// Test conversion to TitanEvent
	titanEvent := event.ToTitanEvent()
	if titanEvent == nil {
		t.Fatal("ToTitanEvent() returned nil")
	}

	if titanEvent.Type != "gitops.argocd.sync" {
		t.Errorf("titanEvent.Type = %v, want %v", titanEvent.Type, "gitops.argocd.sync")
	}

	if titanEvent.Source != "webhook/argocd" {
		t.Errorf("titanEvent.Source = %v, want %v", titanEvent.Source, "webhook/argocd")
	}
}

func TestArgoCDHandlerInvalidPayload(t *testing.T) {
	handler := &ArgoCDHandler{}
	req := &http.Request{
		Header: http.Header{},
	}

	_, err := handler.Parse(req, []byte("invalid json"))
	if err == nil {
		t.Error("Parse() expected error for invalid JSON, got nil")
	}
}
