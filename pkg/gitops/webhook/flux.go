package webhook

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// FluxHandler handles Flux webhook events
type FluxHandler struct{}

// Type returns the webhook type
func (h *FluxHandler) Type() WebhookType {
	return WebhookTypeFlux
}

// FluxWebhookPayload represents a Flux webhook payload
type FluxWebhookPayload struct {
	InvolvedObject struct {
		Kind       string `json:"kind"`
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		APIVersion string `json:"apiVersion"`
	} `json:"involvedObject"`
	Severity  string    `json:"severity"`
	Timestamp time.Time `json:"timestamp"`
	Message   string    `json:"message"`
	Reason    string    `json:"reason"`
	Metadata  map[string]string `json:"metadata"`
	ReportingController string `json:"reportingController"`
	ReportingInstance   string `json:"reportingInstance"`
}

// Parse parses a Flux webhook payload
func (h *FluxHandler) Parse(r *http.Request, body []byte) (*WebhookEvent, error) {
	var payload FluxWebhookPayload
	if err := ParseJSON(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse Flux webhook: %w", err)
	}

	// Get event type from header or reason
	eventType := r.Header.Get("X-Flux-Event")
	if eventType == "" {
		eventType = payload.Reason
	}

	// Extract revision from metadata
	revision := ""
	if payload.Metadata != nil {
		revision = payload.Metadata["revision"]
	}

	event := &WebhookEvent{
		ID:          uuid.New().String(),
		Type:        WebhookTypeFlux,
		EventType:   eventType,
		Source:      "flux",
		Timestamp:   payload.Timestamp,
		Application: payload.InvolvedObject.Name,
		Namespace:   payload.InvolvedObject.Namespace,
		Revision:    revision,
		Status:      payload.Severity,
		Data: map[string]interface{}{
			"kind":                 payload.InvolvedObject.Kind,
			"api_version":          payload.InvolvedObject.APIVersion,
			"severity":             payload.Severity,
			"message":              payload.Message,
			"reason":               payload.Reason,
			"metadata":             payload.Metadata,
			"reporting_controller": payload.ReportingController,
			"reporting_instance":   payload.ReportingInstance,
		},
	}

	return event, nil
}
