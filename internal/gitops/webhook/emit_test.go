package webhook

import (
	"context"
	"errors"
	"sync"
	"testing"

	"go.keystone-core.io/keystone-core/internal/events"
)

func TestToKscoreEvent_Normalization(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		ev       Event
		wantType events.EventType
		wantSev  events.Severity
	}{
		{
			name:     "argocd synced (acceptance line 102)",
			ev:       Event{Provider: ProviderArgoCD, Application: "web", Status: "synced", Revision: "abc"},
			wantType: "gitops.argocd.sync_succeeded",
			wantSev:  events.SeverityInfo,
		},
		{
			name:     "argocd degraded → error",
			ev:       Event{Provider: ProviderArgoCD, Status: "degraded"},
			wantType: "gitops.argocd.health_degraded",
			wantSev:  events.SeverityError,
		},
		{
			name:     "argocd outofsync → warn",
			ev:       Event{Provider: ProviderArgoCD, Status: "OutOfSync"},
			wantType: "gitops.argocd.out_of_sync",
			wantSev:  events.SeverityWarn,
		},
		{
			name:     "flux reconciliation succeeded (substring match)",
			ev:       Event{Provider: ProviderFlux, Status: "ReconciliationSucceeded"},
			wantType: "gitops.flux.reconciliation_succeeded",
			wantSev:  events.SeverityInfo,
		},
		{
			name:     "flux failed → error",
			ev:       Event{Provider: ProviderFlux, Status: "BuildFailed"},
			wantType: "gitops.flux.reconciliation_failed",
			wantSev:  events.SeverityError,
		},
		{
			name:     "github failure → error",
			ev:       Event{Provider: ProviderGitHub, Status: "failure"},
			wantType: "gitops.github.failure",
			wantSev:  events.SeverityError,
		},
		{
			name:     "gitlab push",
			ev:       Event{Provider: ProviderGitLab, Status: "push"},
			wantType: "gitops.gitlab.push",
			wantSev:  events.SeverityInfo,
		},
		{
			name:     "unknown status → sanitized free-form subtype",
			ev:       Event{Provider: ProviderArgoCD, Status: "Some Weird/Status"},
			wantType: "gitops.argocd.some_weird_status",
			wantSev:  events.SeverityInfo,
		},
		{
			name:     "empty status → unknown",
			ev:       Event{Provider: ProviderGitHub},
			wantType: "gitops.github.unknown",
			wantSev:  events.SeverityInfo,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got, err := ToKscoreEvent(tc.ev, "node-1")
			if err != nil {
				t.Fatalf("ToKscoreEvent: %v", err)
			}
			if got.Type != tc.wantType {
				t.Errorf("Type = %q, want %q", got.Type, tc.wantType)
			}
			if got.Severity != tc.wantSev {
				t.Errorf("Severity = %v, want %v", got.Severity, tc.wantSev)
			}
			if got.Type.Category() != events.CategoryGitops {
				t.Errorf("Category = %q, want gitops", got.Type.Category())
			}
			if _, perr := events.ParseEventType(got.Type.String()); perr != nil {
				t.Errorf("emitted type %q does not parse: %v", got.Type, perr)
			}
			if got.Source != "node-1" {
				t.Errorf("Source = %q, want node-1", got.Source)
			}
		})
	}
}

func TestToKscoreEvent_TagsDataCorrelation(t *testing.T) {
	t.Parallel()
	ev := Event{
		Provider:    ProviderGitHub,
		Application: "acme/web",
		Namespace:   "",
		Revision:    "deadbeef",
		Status:      "success",
		WebhookID:   "delivery-7",
		Raw:         []byte(`{"big":"payload"}`),
	}
	got, err := ToKscoreEvent(ev, "")
	if err != nil {
		t.Fatalf("ToKscoreEvent: %v", err)
	}
	if got.CorrelationID != "delivery-7" {
		t.Errorf("CorrelationID = %q, want delivery-7", got.CorrelationID)
	}
	if got.Tags["provider"] != "github" || got.Tags["application"] != "acme/web" ||
		got.Tags["revision"] != "deadbeef" || got.Tags["webhook_id"] != "delivery-7" {
		t.Errorf("tags missing expected entries: %v", got.Tags)
	}
	if _, ok := got.Tags["namespace"]; ok {
		t.Errorf("empty namespace must not be tagged: %v", got.Tags)
	}
	if _, ok := got.Data["status"]; !ok {
		t.Errorf("Data missing status: %v", got.Data)
	}
	for k, v := range got.Data {
		if k == "raw" {
			t.Fatalf("raw must not be in Data (EventStore bloat): %v", v)
		}
	}
}

// errTestPublish is the canned publish failure for best-effort tests.
var errTestPublish = errors.New("publish boom")

// fakeEmitter records published events and can be made to fail.
type fakeEmitter struct {
	mu  sync.Mutex
	got []events.Event
	err error
}

func (f *fakeEmitter) Publish(_ context.Context, e events.Event) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return f.err
	}
	f.got = append(f.got, e)
	return nil
}

func (f *fakeEmitter) events() []events.Event {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]events.Event(nil), f.got...)
}

var _ EventEmitter = (*fakeEmitter)(nil)

func TestToKscoreEvent_BuildErrorIsImpossibleForKnownProviders(t *testing.T) {
	t.Parallel()
	// Guard: every provider+empty-status still yields a parseable type.
	for _, p := range []Provider{ProviderArgoCD, ProviderFlux, ProviderGitHub, ProviderGitLab} {
		if _, err := ToKscoreEvent(Event{Provider: p}, "s"); err != nil {
			t.Errorf("provider %q: unexpected build error %v", p, err)
		}
	}
}
