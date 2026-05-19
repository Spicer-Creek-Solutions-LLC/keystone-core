package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// GitLabHandler parses GitLab webhooks. GitLab tags every payload with
// an `object_kind` discriminator (push, pipeline, deployment, …) and
// sends a per-delivery `X-Gitlab-Event-UUID` used as Event.WebhookID.
type GitLabHandler struct{}

// Type implements [Handler].
func (GitLabHandler) Type() Provider { return ProviderGitLab }

// DetectHeader implements [Handler].
func (GitLabHandler) DetectHeader() string { return HeaderGitLab }

type gitLabPayload struct {
	ObjectKind  string `json:"object_kind"`
	Ref         string `json:"ref"`
	CheckoutSHA string `json:"checkout_sha"`
	// deployment hook (top-level status/sha/ref)
	Status string `json:"status"`
	SHA    string `json:"sha"`
	Project struct {
		PathWithNamespace string `json:"path_with_namespace"`
	} `json:"project"`
	// pipeline hook
	ObjectAttributes *struct {
		Status string `json:"status"`
		SHA    string `json:"sha"`
	} `json:"object_attributes"`
}

// Parse implements [Handler]. The project path is the Application;
// revision and status are read from the payload shape selected by
// object_kind (pipeline → object_attributes; deployment → top-level;
// push → checkout_sha + object_kind).
func (GitLabHandler) Parse(r *http.Request, body []byte) (Event, error) {
	var p gitLabPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, fmt.Errorf("gitlab: decode payload: %w", err)
	}
	if p.ObjectKind == "" {
		return Event{}, fmt.Errorf("gitlab: payload missing object_kind")
	}
	if p.Project.PathWithNamespace == "" {
		return Event{}, fmt.Errorf("gitlab: payload missing project.path_with_namespace")
	}

	var revision, status string
	switch p.ObjectKind {
	case "pipeline":
		if p.ObjectAttributes != nil {
			revision = p.ObjectAttributes.SHA
			status = p.ObjectAttributes.Status
		}
	case "deployment":
		revision = p.SHA
		status = p.Status
	default: // push and others
		revision = p.CheckoutSHA
		status = p.ObjectKind
	}

	return Event{
		WebhookID:   r.Header.Get("X-Gitlab-Event-UUID"),
		Provider:    ProviderGitLab,
		Application: p.Project.PathWithNamespace,
		Revision:    revision,
		Status:      strings.ToLower(status),
		Raw:         append(json.RawMessage(nil), body...),
	}, nil
}
