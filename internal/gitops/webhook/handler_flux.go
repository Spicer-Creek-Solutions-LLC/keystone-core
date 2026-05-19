package webhook

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// FluxHandler parses Flux notification-controller webhook events. The
// controller posts its Event type: an `involvedObject` (Kustomization
// or HelmRelease), a `severity`/`reason`, and `metadata.revision`
// (typically `<branch>@sha1:<sha>` or an OCI digest).
type FluxHandler struct{}

// Type implements [Handler].
func (FluxHandler) Type() Provider { return ProviderFlux }

type fluxPayload struct {
	InvolvedObject struct {
		Kind      string `json:"kind"`
		Namespace string `json:"namespace"`
		Name      string `json:"name"`
	} `json:"involvedObject"`
	Severity string `json:"severity"`
	Reason   string `json:"reason"`
	Metadata struct {
		Revision string `json:"revision"`
	} `json:"metadata"`
}

// Parse implements [Handler]. Status is the lowercased reason (e.g.
// "reconciliationsucceeded", "progressing"); when absent it falls back
// to the severity so an error event is still classifiable.
func (FluxHandler) Parse(_ *http.Request, body []byte) (Event, error) {
	var p fluxPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return Event{}, fmt.Errorf("flux: decode payload: %w", err)
	}
	if p.InvolvedObject.Name == "" {
		return Event{}, fmt.Errorf("flux: payload missing involvedObject.name")
	}
	status := p.Reason
	if status == "" {
		status = p.Severity
	}
	return Event{
		Provider:    ProviderFlux,
		Application: p.InvolvedObject.Name,
		Namespace:   p.InvolvedObject.Namespace,
		Revision:    p.Metadata.Revision,
		Status:      strings.ToLower(status),
		Raw:         append(json.RawMessage(nil), body...),
	}, nil
}
