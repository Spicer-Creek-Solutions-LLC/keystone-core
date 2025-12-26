package webhook

import (
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// GitHubHandler handles GitHub webhook events
type GitHubHandler struct{}

// Type returns the webhook type
func (h *GitHubHandler) Type() WebhookType {
	return WebhookTypeGitHub
}

// GitHubWebhookPayload represents a generic GitHub webhook payload
type GitHubWebhookPayload struct {
	Action     string `json:"action"`
	Repository struct {
		Name     string `json:"name"`
		FullName string `json:"full_name"`
		HTMLURL  string `json:"html_url"`
	} `json:"repository"`
	Sender struct {
		Login string `json:"login"`
	} `json:"sender"`
	// Deployment specific
	Deployment struct {
		ID          int64  `json:"id"`
		SHA         string `json:"sha"`
		Ref         string `json:"ref"`
		Task        string `json:"task"`
		Environment string `json:"environment"`
		Description string `json:"description"`
	} `json:"deployment"`
	DeploymentStatus struct {
		State       string    `json:"state"`
		Description string    `json:"description"`
		CreatedAt   time.Time `json:"created_at"`
	} `json:"deployment_status"`
	// Workflow specific
	WorkflowRun struct {
		ID         int64     `json:"id"`
		Name       string    `json:"name"`
		Status     string    `json:"status"`
		Conclusion string    `json:"conclusion"`
		HeadSHA    string    `json:"head_sha"`
		CreatedAt  time.Time `json:"created_at"`
		UpdatedAt  time.Time `json:"updated_at"`
	} `json:"workflow_run"`
	// Push specific
	Ref    string `json:"ref"`
	After  string `json:"after"`
	Before string `json:"before"`
	Commits []struct {
		ID      string `json:"id"`
		Message string `json:"message"`
		Author  struct {
			Name  string `json:"name"`
			Email string `json:"email"`
		} `json:"author"`
	} `json:"commits"`
}

// Parse parses a GitHub webhook payload
func (h *GitHubHandler) Parse(r *http.Request, body []byte) (*WebhookEvent, error) {
	var payload GitHubWebhookPayload
	if err := ParseJSON(body, &payload); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub webhook: %w", err)
	}

	// Get event type from header
	eventType := r.Header.Get("X-GitHub-Event")
	if eventType == "" {
		return nil, fmt.Errorf("missing X-GitHub-Event header")
	}

	// Determine status and revision based on event type
	status := ""
	revision := ""
	namespace := ""

	switch eventType {
	case "deployment":
		status = "deployed"
		revision = payload.Deployment.SHA
		namespace = payload.Deployment.Environment
	case "deployment_status":
		status = payload.DeploymentStatus.State
		revision = payload.Deployment.SHA
		namespace = payload.Deployment.Environment
	case "workflow_run":
		status = payload.WorkflowRun.Status
		if payload.WorkflowRun.Conclusion != "" {
			status = payload.WorkflowRun.Conclusion
		}
		revision = payload.WorkflowRun.HeadSHA
	case "push":
		status = "pushed"
		revision = payload.After
	}

	event := &WebhookEvent{
		ID:          uuid.New().String(),
		Type:        WebhookTypeGitHub,
		EventType:   eventType,
		Source:      "github",
		Timestamp:   time.Now(),
		Application: payload.Repository.Name,
		Namespace:   namespace,
		Revision:    revision,
		Status:      status,
		Data: map[string]interface{}{
			"action":          payload.Action,
			"repository":      payload.Repository.FullName,
			"repository_url":  payload.Repository.HTMLURL,
			"sender":          payload.Sender.Login,
			"deployment":      payload.Deployment,
			"deployment_status": payload.DeploymentStatus,
			"workflow_run":    payload.WorkflowRun,
			"ref":             payload.Ref,
			"commits":         payload.Commits,
		},
	}

	return event, nil
}
