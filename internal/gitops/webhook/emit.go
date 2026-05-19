package webhook

import (
	"context"
	"fmt"
	"strings"

	"go.keystone-core.io/keystone-core/internal/events"
)

// EventEmitter is the subset of [events.EventPublisher] this package
// needs to re-emit a parsed webhook on the Keystone bus.
// events.EventPublisher satisfies it. Kept narrow (the runbook
// observer's precedent) so the package does not depend on the full
// publisher surface.
type EventEmitter interface {
	Publish(ctx context.Context, e events.Event) error
}

// outcome is the normalized (subtype, severity) a provider status
// maps to. Subtype is the second segment of the
// `gitops.<provider>.<subtype>` event type.
type outcome struct {
	subtype  string
	severity events.Severity
}

// normalize maps a parsed [Event] onto its event subtype + severity.
// The per-provider tables cover the common deploy signals; an
// unrecognized status falls back to `<provider>.<sanitized-status>`
// at Info — valid because [events.CategoryGitops] is a known category
// and subtypes are free-form (provider statuses are open-ended).
func normalize(e Event) outcome {
	s := strings.ToLower(strings.TrimSpace(e.Status))
	p := e.Provider.String()
	switch e.Provider {
	case ProviderArgoCD:
		switch s {
		case "synced":
			return outcome{"argocd.sync_succeeded", events.SeverityInfo}
		case "failed", "error":
			return outcome{"argocd.sync_failed", events.SeverityError}
		case "degraded":
			return outcome{"argocd.health_degraded", events.SeverityError}
		case "outofsync":
			return outcome{"argocd.out_of_sync", events.SeverityWarn}
		case "progressing":
			return outcome{"argocd.progressing", events.SeverityInfo}
		}
	case ProviderFlux:
		switch {
		case strings.Contains(s, "succeeded"):
			return outcome{"flux.reconciliation_succeeded", events.SeverityInfo}
		case strings.Contains(s, "failed"), s == "error":
			return outcome{"flux.reconciliation_failed", events.SeverityError}
		}
	case ProviderGitHub:
		switch s {
		case "success":
			return outcome{"github.success", events.SeverityInfo}
		case "failure", "error":
			return outcome{"github.failure", events.SeverityError}
		case "push":
			return outcome{"github.push", events.SeverityInfo}
		case "pending", "in_progress", "queued":
			return outcome{"github.in_progress", events.SeverityInfo}
		}
	case ProviderGitLab:
		switch s {
		case "success":
			return outcome{"gitlab.success", events.SeverityInfo}
		case "failed":
			return outcome{"gitlab.failed", events.SeverityError}
		case "running", "pending":
			return outcome{"gitlab.running", events.SeverityInfo}
		case "push":
			return outcome{"gitlab.push", events.SeverityInfo}
		}
	}
	return outcome{p + "." + sanitizeSubtype(s), events.SeverityInfo}
}

// sanitizeSubtype lowercases and replaces every rune outside
// [a-z0-9_] with '_', guaranteeing a non-empty, whitespace-free
// segment that [events.ParseEventType] accepts. Empty input (no
// status on the payload) becomes "unknown".
func sanitizeSubtype(s string) string {
	if s == "" {
		return "unknown"
	}
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	return b.String()
}

// ToKscoreEvent normalizes a parsed webhook into a Keystone bus event
// of type `gitops.<provider>.<subtype>`. source is the Event.Source
// field (e.g. the server node name); empty defaults to
// "gitops-webhook" (events.NewEvent rejects an empty source). Raw is
// intentionally excluded
// from Data — raw bodies bloat the SQL EventStore and are not
// queryable; they stay at the receiver for audit.
func ToKscoreEvent(e Event, source string) (events.Event, error) {
	if source == "" {
		source = "gitops-webhook"
	}
	o := normalize(e)
	typ := events.EventType(fmt.Sprintf("gitops.%s", o.subtype))
	ev, err := events.NewEvent(typ, source)
	if err != nil {
		return events.Event{}, fmt.Errorf("gitops/webhook: build event: %w", err)
	}
	ev.Severity = o.severity
	if e.WebhookID != "" {
		ev.CorrelationID = e.WebhookID
	}
	ev.Tags = nonEmptyTags(map[string]string{
		"provider":    e.Provider.String(),
		"application": e.Application,
		"namespace":   e.Namespace,
		"revision":    e.Revision,
		"status":      e.Status,
		"webhook_id":  e.WebhookID,
	})
	ev.Data = map[string]any{
		"provider":    e.Provider.String(),
		"application": e.Application,
		"namespace":   e.Namespace,
		"revision":    e.Revision,
		"status":      e.Status,
	}
	return ev, nil
}

// nonEmptyTags drops empty-valued keys so indexed tags stay sparse.
func nonEmptyTags(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		if v != "" {
			out[k] = v
		}
	}
	return out
}
