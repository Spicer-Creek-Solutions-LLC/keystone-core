package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// Handler processes webhook payloads
type Handler interface {
	// Type returns the webhook type this handler supports
	Type() WebhookType

	// Parse parses the webhook payload into a WebhookEvent
	Parse(r *http.Request, body []byte) (*WebhookEvent, error)
}

// HandlerRegistry manages webhook handlers
type HandlerRegistry struct {
	handlers map[WebhookType]Handler
}

// NewHandlerRegistry creates a new handler registry
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{
		handlers: make(map[WebhookType]Handler),
	}
}

// Register registers a webhook handler
func (r *HandlerRegistry) Register(handler Handler) {
	r.handlers[handler.Type()] = handler
}

// Get retrieves a handler by type
func (r *HandlerRegistry) Get(webhookType WebhookType) (Handler, bool) {
	handler, ok := r.handlers[webhookType]
	return handler, ok
}

// DetectType attempts to detect the webhook type from the request
func (r *HandlerRegistry) DetectType(req *http.Request) (WebhookType, error) {
	// Check User-Agent header
	userAgent := req.Header.Get("User-Agent")

	// ArgoCD specific headers
	if req.Header.Get("X-Argo-CD-Webhook") != "" {
		return WebhookTypeArgoCD, nil
	}

	// Flux specific headers
	if req.Header.Get("X-Flux-Event") != "" {
		return WebhookTypeFlux, nil
	}

	// GitHub specific headers
	if req.Header.Get("X-GitHub-Event") != "" {
		return WebhookTypeGitHub, nil
	}

	// GitLab specific headers
	if req.Header.Get("X-Gitlab-Event") != "" {
		return WebhookTypeGitLab, nil
	}

	// Try to detect from User-Agent
	if contains(userAgent, "ArgoCD") {
		return WebhookTypeArgoCD, nil
	}
	if contains(userAgent, "Flux") {
		return WebhookTypeFlux, nil
	}
	if contains(userAgent, "GitHub-Hookshot") {
		return WebhookTypeGitHub, nil
	}
	if contains(userAgent, "GitLab") {
		return WebhookTypeGitLab, nil
	}

	return "", fmt.Errorf("unable to detect webhook type")
}

// ParseJSON is a helper to parse JSON payloads
func ParseJSON(body []byte, v interface{}) error {
	return json.Unmarshal(body, v)
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 &&
		(s == substr || len(s) >= len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
		findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
