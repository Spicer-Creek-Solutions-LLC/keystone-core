package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// ArgoCDHandler parses ArgoCD notification webhooks. ArgoCD's
// notifications engine posts an operator-defined template; the
// repo-recommended template serializes the Application object, so we
// parse the stable `app.metadata` / `app.status` subset and tolerate
// missing fields (a health-only notification has no sync revision).
type ArgoCDHandler struct{}

// Type implements [Handler].
func (ArgoCDHandler) Type() Provider { return ProviderArgoCD }

type argoCDPayload struct {
	App struct {
		Metadata struct {
			Name      string `json:"name"`
			Namespace string `json:"namespace"`
		} `json:"metadata"`
		Status struct {
			Sync struct {
				Status   string `json:"status"`
				Revision string `json:"revision"`
			} `json:"sync"`
			Health struct {
				Status string `json:"status"`
			} `json:"health"`
		} `json:"status"`
	} `json:"app"`
}

// Parse implements [Handler]. Status prefers the sync status, falling
// back to the health status so a health-degraded notification still
// carries a meaningful outcome.
func (ArgoCDHandler) Parse(_ *http.Request, body []byte) (Event, error) {
	var p argoCDPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, fmt.Errorf("argocd: decode payload: %w", err)
	}
	if p.App.Metadata.Name == "" {
		return Event{}, fmt.Errorf("argocd: payload missing app.metadata.name")
	}
	status := p.App.Status.Sync.Status
	if status == "" {
		status = p.App.Status.Health.Status
	}
	return Event{
		Provider:    ProviderArgoCD,
		Application: p.App.Metadata.Name,
		Namespace:   p.App.Metadata.Namespace,
		Revision:    p.App.Status.Sync.Revision,
		Status:      strings.ToLower(status),
		Raw:         append(json.RawMessage(nil), body...),
	}, nil
}
