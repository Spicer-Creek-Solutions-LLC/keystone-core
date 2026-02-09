// Package webhook provides webhook handling for GitOps events from ArgoCD,
// Flux, GitHub, and GitLab sources.
package webhook

import (
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
)

// Type represents the type of webhook source
type Type string

// WebhookTypeArgoCD constants define the supported types.
const (
	WebhookTypeArgoCD Type = "argocd"
	WebhookTypeFlux   Type = "flux"
	WebhookTypeGitHub Type = "github"
	WebhookTypeGitLab Type = "gitlab"
)

// Event represents a parsed webhook event
type Event struct {
	ID          string                 `json:"id"`
	Type        Type            `json:"type"`
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
func (w *Event) ToKscoreEvent() *events.Event {
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

// AuthType constants define the supported types.
const (
	AuthTypeNone   AuthType = "none"
	AuthTypeHMAC   AuthType = "hmac"
	AuthTypeBearer AuthType = "bearer"
)

// Config represents webhook receiver configuration
type Config struct {
	Enabled  bool       `json:"enabled"`
	Addr     string     `json:"addr"`
	Path     string     `json:"path"`
	Auth     AuthConfig `json:"auth"`
	Handlers []string   `json:"handlers"` // Which webhook types to accept
}

// DefaultWebhookConfig returns default configuration
func DefaultWebhookConfig() *Config {
	return &Config{
		Enabled: true,
		Addr:    ":8080",
		Path:    "/webhooks",
		Auth: AuthConfig{
			Type: AuthTypeNone,
		},
		Handlers: []string{"argocd", "flux", "github", "gitlab"},
	}
}
