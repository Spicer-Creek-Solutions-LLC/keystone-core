package webhook

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// GitLabHandler handles GitLab webhook events
type GitLabHandler struct{}

// Type returns the webhook type
func (h *GitLabHandler) Type() WebhookType {
	return WebhookTypeGitLab
}

// GitLabWebhookPayload represents a generic GitLab webhook payload
type GitLabWebhookPayload struct {
	ObjectKind string `json:"object_kind"`
	EventName  string `json:"event_name"`
	Project    struct {
		ID                int64  `json:"id"`
		Name              string `json:"name"`
		PathWithNamespace string `json:"path_with_namespace"`
		WebURL            string `json:"web_url"`
	} `json:"project"`
	User struct {
		Name     string `json:"name"`
		Username string `json:"username"`
		Email    string `json:"email"`
	} `json:"user"`
	// Deployment specific
	Status            string    `json:"status"`
	Environment       string    `json:"environment"`
	EnvironmentTier   string    `json:"environment_tier"`
	Ref               string    `json:"ref"`
	SHA               string    `json:"sha"`
	Deployment        int64     `json:"deployment_id"`
	CreatedAt         time.Time `json:"created_at"`
	// Pipeline specific
	ObjectAttributes struct {
		ID         int64     `json:"id"`
		Ref        string    `json:"ref"`
		SHA        string    `json:"sha"`
		Status     string    `json:"status"`
		CreatedAt  time.Time `json:"created_at"`
		FinishedAt time.Time `json:"finished_at"`
	} `json:"object_attributes"`
	// Push specific
	Before  string `json:"before"`
	After   string `json:"after"`
	Commits []struct {
		ID      string    `json:"id"`
		Message string    `json:"message"`
		Timestamp time.Time `json:"timestamp"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"commits"`
}

// Parse parses a GitLab webhook payload
func (h *GitLabHandler) Parse(r *http.Request, body []byte) (*WebhookEvent, error) {
	var payload GitLabWebhookPayload
	if err := ParseJSON(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse GitLab webhook: %w", err)
	}

	// Get event type from header or payload
	eventType := r.Header.Get("X-Gitlab-Event")
	if eventType == "" {
		eventType = payload.ObjectKind
	}
	if eventType == "" {
		eventType = payload.EventName
	}

	// Determine status and revision based on event type
	status := payload.Status
	revision := payload.SHA
	namespace := payload.Environment

	if payload.ObjectAttributes.Status != "" {
		status = payload.ObjectAttributes.Status
		revision = payload.ObjectAttributes.SHA
	}

	if eventType == "push" {
		revision = payload.After
		status = "pushed"
	}

	event := &WebhookEvent{
		ID:          uuid.New().String(),
		Type:        WebhookTypeGitLab,
		EventType:   eventType,
		Source:      "gitlab",
		Timestamp:   time.Now(),
		Application: payload.Project.Name,
		Namespace:   namespace,
		Revision:    revision,
		Status:      status,
		Data: map[string]interface{}{
			"object_kind":       payload.ObjectKind,
			"event_name":        payload.EventName,
			"project":           payload.Project.PathWithNamespace,
			"project_url":       payload.Project.WebURL,
			"user":              payload.User.Username,
			"environment_tier":  payload.EnvironmentTier,
			"ref":               payload.Ref,
			"deployment_id":     payload.Deployment,
			"created_at":        payload.CreatedAt,
			"object_attributes": payload.ObjectAttributes,
			"commits":           payload.Commits,
			"before":            payload.Before,
			"after":             payload.After,
		},
	}

	return event, nil
}
