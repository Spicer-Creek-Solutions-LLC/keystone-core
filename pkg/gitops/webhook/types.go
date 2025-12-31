package webhook

import (
	"time"

	"github.com/shawnbutts/keystone-core/pkg/events"
)

// WebhookType represents the type of webhook source
type WebhookType string

const (
	WebhookTypeArgoCD WebhookType = "argocd"
	WebhookTypeFlux   WebhookType = "flux"
	WebhookTypeGitHub WebhookType = "github"
	WebhookTypeGitLab WebhookType = "gitlab"
)

// WebhookEvent represents a parsed webhook event
type WebhookEvent struct {
	ID          string                 `json:"id"`
	Type        WebhookType            `json:"type"`
	EventType   string                 `json:"event_type"`
	Source      string                 `json:"source"`
	Timestamp   time.Time              `json:"timestamp"`
	Application string                 `json:"application,omitempty"`
	Namespace   string                 `json:"namespace,omitempty"`
	Revision    string                 `json:"revision,omitempty"`
	Status      string                 `json:"status,omitempty"`
	Data        map[string]interface{} `json:"data"`
}

// ToKscoreEvent converts a webhook event to a Keystone Core event
func (w *WebhookEvent) ToKscoreEvent() *events.Event {
	eventType := events.EventType("gitops.webhook")
	switch w.Type {
	case WebhookTypeArgoCD:
		eventType = events.EventType("gitops.argocd." + w.EventType)
	case WebhookTypeFlux:
		eventType = events.EventType("gitops.flux." + w.EventType)
	case WebhookTypeGitHub:
		eventType = events.EventType("gitops.github." + w.EventType)
	case WebhookTypeGitLab:
		eventType = events.EventType("gitops.gitlab." + w.EventType)
	}

	data := make(map[string]interface{})
	data["webhook_id"] = w.ID
	data["webhook_type"] = string(w.Type)
	data["application"] = w.Application
	data["namespace"] = w.Namespace
	data["revision"] = w.Revision
	data["status"] = w.Status
	for k, v := range w.Data {
		data[k] = v
	}

	return events.NewEvent(eventType).
		Source("webhook/" + string(w.Type)).
		Severity(events.SeverityInfo).
		CorrelationID("webhook-" + w.ID).
		DataMap(data).
		Build()
}

// AuthConfig represents webhook authentication configuration
type AuthConfig struct {
	Type   AuthType `json:"type"`
	Secret string   `json:"secret,omitempty"`
	Token  string   `json:"token,omitempty"`
}

// AuthType represents the authentication method
type AuthType string

const (
	AuthTypeNone   AuthType = "none"
	AuthTypeHMAC   AuthType = "hmac"
	AuthTypeBearer AuthType = "bearer"
)

// WebhookConfig represents webhook receiver configuration
type WebhookConfig struct {
	Enabled  bool       `json:"enabled"`
	Addr     string     `json:"addr"`
	Path     string     `json:"path"`
	Auth     AuthConfig `json:"auth"`
	Handlers []string   `json:"handlers"` // Which webhook types to accept
}

// DefaultWebhookConfig returns default configuration
func DefaultWebhookConfig() *WebhookConfig {
	return &WebhookConfig{
		Enabled: true,
		Addr:    ":8080",
		Path:    "/webhooks",
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
		Handlers: []string{"argocd", "flux", "github", "gitlab"},
	}
}
