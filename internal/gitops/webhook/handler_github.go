package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GitHubHandler parses GitHub webhooks. GitHub multiplexes payload
// shapes by the `X-GitHub-Event` header (deployment_status,
// workflow_run, push, …); the per-delivery `X-GitHub-Delivery` UUID
// becomes the Event.WebhookID. Reading the event-type header here is
// payload disambiguation, not provider auto-detection (task 2).
type GitHubHandler struct{}

// Type implements [Handler].
func (GitHubHandler) Type() Provider { return ProviderGitHub }

type gitHubPayload struct {
	Ref        string `json:"ref"`
	After      string `json:"after"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	DeploymentStatus *struct {
		State string `json:"state"`
	} `json:"deployment_status"`
	Deployment *struct {
		SHA         string `json:"sha"`
		Environment string `json:"environment"`
	} `json:"deployment"`
	WorkflowRun *struct {
		HeadSHA    string `json:"head_sha"`
		Status     string `json:"status"`
		Conclusion string `json:"conclusion"`
	} `json:"workflow_run"`
}

// Parse implements [Handler]. The repository full name is the
// Application; revision and status are taken from whichever event
// payload is present (deployment_status → workflow_run → push).
func (GitHubHandler) Parse(r *http.Request, body []byte) (Event, error) {
	event := r.Header.Get("X-GitHub-Event")
	if event == "" {
		return Event{}, fmt.Errorf("github: missing X-GitHub-Event header")
	}
	var p gitHubPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, fmt.Errorf("github: decode payload: %w", err)
	}
	if p.Repository.FullName == "" {
		return Event{}, fmt.Errorf("github: payload missing repository.full_name")
	}

	var revision, status string
	switch {
	case p.DeploymentStatus != nil:
		status = p.DeploymentStatus.State
		if p.Deployment != nil {
			revision = p.Deployment.SHA
		}
	case p.WorkflowRun != nil:
		revision = p.WorkflowRun.HeadSHA
		status = p.WorkflowRun.Conclusion
		if status == "" {
			status = p.WorkflowRun.Status
		}
	default: // push and others
		revision = p.After
		status = event
	}

	return Event{
		WebhookID:   r.Header.Get("X-GitHub-Delivery"),
		Provider:    ProviderGitHub,
		Application: p.Repository.FullName,
		Revision:    revision,
		Status:      strings.ToLower(status),
		Raw:         append(json.RawMessage(nil), body...),
	}, nil
}
