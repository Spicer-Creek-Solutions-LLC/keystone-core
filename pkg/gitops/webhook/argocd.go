package webhook

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// ArgoCDHandler handles ArgoCD webhook events
type ArgoCDHandler struct{}

// Type returns the webhook type
func (h *ArgoCDHandler) Type() WebhookType {
	return WebhookTypeArgoCD
}

// ArgoCDWebhookPayload represents an ArgoCD webhook payload
type ArgoCDWebhookPayload struct {
	Application struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Spec struct {
			Source struct {
				RepoURL        string `json:"repoURL"`
				TargetRevision string `json:"targetRevision"`
			} `json:"source"`
		} `json:"spec"`
		Status struct {
			Sync struct {
				Status   string `json:"status"`
				Revision string `json:"revision"`
			} `json:"sync"`
			Health struct {
				Status string `json:"status"`
			} `json:"health"`
			OperationState struct {
				Phase      string    `json:"phase"`
				StartedAt  time.Time `json:"startedAt"`
				FinishedAt time.Time `json:"finishedAt"`
			} `json:"operationState"`
		} `json:"status"`
	} `json:"application"`
	Type string `json:"type"`
}

// Parse parses an ArgoCD webhook payload
func (h *ArgoCDHandler) Parse(r *http.Request, body []byte) (*WebhookEvent, error) {
	var payload ArgoCDWebhookPayload
	if err := ParseJSON(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse ArgoCD webhook: %w", err)
	}

	// Determine event type
	eventType := "sync"
	if payload.Type != "" {
		eventType = payload.Type
	} else if payload.Application.Status.OperationState.Phase != "" {
		eventType = payload.Application.Status.OperationState.Phase
	}

	// Extract status
	status := payload.Application.Status.Sync.Status
	if payload.Application.Status.Health.Status != "" {
		status = payload.Application.Status.Health.Status
	}

	event := &WebhookEvent{
		ID:          uuid.New().String(),
		Type:        WebhookTypeArgoCD,
		EventType:   eventType,
		Source:      "argocd",
		Timestamp:   time.Now(),
		Application: payload.Application.Metadata.Name,
		Namespace:   payload.Application.Metadata.Namespace,
		Revision:    payload.Application.Status.Sync.Revision,
		Status:      status,
		Data: map[string]interface{}{
			"repo_url":        payload.Application.Spec.Source.RepoURL,
			"target_revision": payload.Application.Spec.Source.TargetRevision,
			"sync_status":     payload.Application.Status.Sync.Status,
			"health_status":   payload.Application.Status.Health.Status,
			"phase":           payload.Application.Status.OperationState.Phase,
			"started_at":      payload.Application.Status.OperationState.StartedAt,
			"finished_at":     payload.Application.Status.OperationState.FinishedAt,
		},
	}

	return event, nil
}
