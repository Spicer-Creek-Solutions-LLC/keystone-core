// Package webhook is the GitOps inbound webhook receiver (Epic 16).
//
// It accepts deployment-tool webhooks (ArgoCD, Flux) and Git-provider
// webhooks (GitHub, GitLab), parses each provider's payload into a
// unified [Event], and (from Epic 16 task 4 onward) re-emits it on the
// Keystone event bus as `gitops.{argocd|flux|github|gitlab}.*`.
//
// Tasks 1-2: the HTTP [Receiver], the [Handler] interface + its
// [Registry], the four concrete provider handlers, and header-based
// source auto-detection ([Registry.Detect]). Request authentication
// (task 3) and event-bus emission (task 4) land in their own tasks;
// until task 4 a successfully parsed webhook is acknowledged with 202.
package webhook

import "encoding/json"

// Provider identifies the GitOps source that sent a webhook. The set
// is closed: each value has a dedicated [Handler] and (from task 4) a
// canonical `gitops.<provider>.*` event category.
type Provider string

const (
	// ProviderArgoCD is the ArgoCD application controller.
	ProviderArgoCD Provider = "argocd"
	// ProviderFlux is the Flux GitOps toolkit.
	ProviderFlux Provider = "flux"
	// ProviderGitHub is github.com / GitHub Enterprise.
	ProviderGitHub Provider = "github"
	// ProviderGitLab is gitlab.com / self-managed GitLab.
	ProviderGitLab Provider = "gitlab"
)

// String returns the underlying lowercase provider name.
func (p Provider) String() string { return string(p) }

// Valid reports whether p is one of the four known providers.
func (p Provider) Valid() bool {
	switch p {
	case ProviderArgoCD, ProviderFlux, ProviderGitHub, ProviderGitLab:
		return true
	default:
		return false
	}
}

// Event is the provider-neutral normalization of an inbound webhook.
// Every [Handler.Parse] maps its provider's payload onto these fields;
// task 4's ToKscoreEvent converts an Event into a Keystone bus event.
//
// Application/Namespace/Revision/Status are best-effort: a provider
// payload that does not carry a field leaves it empty rather than
// failing the parse — the receiver still records that a verified
// webhook arrived.
type Event struct {
	// WebhookID is a stable per-delivery identifier. Providers that
	// send one (GitHub `X-GitHub-Delivery`, GitLab `X-Gitlab-Event-UUID`)
	// have it populated by the handler; others leave it empty and the
	// receiver assigns one (task 4).
	WebhookID string `json:"webhook_id,omitempty"`
	// Provider is the source that sent the webhook.
	Provider Provider `json:"provider"`
	// Application is the deployed unit: ArgoCD app, Flux Kustomization/
	// HelmRelease, or the Git repository name for GitHub/GitLab.
	Application string `json:"application,omitempty"`
	// Namespace is the Kubernetes namespace when the provider reports
	// one; empty for pure Git-provider events.
	Namespace string `json:"namespace,omitempty"`
	// Revision is the Git SHA / target revision the event concerns.
	Revision string `json:"revision,omitempty"`
	// Status is the provider-reported outcome, lowercased and
	// normalized per handler (e.g. "synced", "succeeded", "failed").
	Status string `json:"status,omitempty"`
	// Raw is the verbatim request body, retained for audit and for
	// consumers that need provider-specific fields not normalized here.
	Raw json.RawMessage `json:"raw,omitempty"`
}
