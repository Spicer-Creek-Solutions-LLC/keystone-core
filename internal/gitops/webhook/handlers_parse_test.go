// SPDX-License-Identifier: Apache-2.0

package webhook

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func req(t *testing.T, headers map[string]string) *http.Request {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/webhooks", nil)
	for k, v := range headers {
		r.Header.Set(k, v)
	}
	return r
}

func TestArgoCDHandler_Parse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		body       string
		wantErr    bool
		wantApp    string
		wantNS     string
		wantRev    string
		wantStatus string
	}{
		{
			name:       "sync succeeded",
			body:       `{"app":{"metadata":{"name":"web","namespace":"prod"},"status":{"sync":{"status":"Synced","revision":"abc123"},"health":{"status":"Healthy"}}}}`,
			wantApp:    "web",
			wantNS:     "prod",
			wantRev:    "abc123",
			wantStatus: "synced",
		},
		{
			name:       "health-only falls back to health status",
			body:       `{"app":{"metadata":{"name":"api"},"status":{"health":{"status":"Degraded"}}}}`,
			wantApp:    "api",
			wantStatus: "degraded",
		},
		{name: "missing app name", body: `{"app":{"status":{}}}`, wantErr: true},
		{name: "malformed json", body: `{`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev, err := ArgoCDHandler{}.Parse(req(t, nil), []byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", ev)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ev.Provider != ProviderArgoCD {
				t.Errorf("Provider = %q, want argocd", ev.Provider)
			}
			if ev.Application != tc.wantApp || ev.Namespace != tc.wantNS ||
				ev.Revision != tc.wantRev || ev.Status != tc.wantStatus {
				t.Errorf("got app=%q ns=%q rev=%q status=%q; want app=%q ns=%q rev=%q status=%q",
					ev.Application, ev.Namespace, ev.Revision, ev.Status,
					tc.wantApp, tc.wantNS, tc.wantRev, tc.wantStatus)
			}
			if string(ev.Raw) != tc.body {
				t.Errorf("Raw = %q, want verbatim body", ev.Raw)
			}
		})
	}
}

func TestFluxHandler_Parse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		body       string
		wantErr    bool
		wantApp    string
		wantStatus string
		wantRev    string
	}{
		{
			name:       "kustomization reconciled",
			body:       `{"involvedObject":{"kind":"Kustomization","namespace":"flux-system","name":"apps"},"severity":"info","reason":"ReconciliationSucceeded","metadata":{"revision":"main@sha1:deadbeef"}}`,
			wantApp:    "apps",
			wantStatus: "reconciliationsucceeded",
			wantRev:    "main@sha1:deadbeef",
		},
		{
			name:       "no reason falls back to severity",
			body:       `{"involvedObject":{"kind":"HelmRelease","name":"redis"},"severity":"error"}`,
			wantApp:    "redis",
			wantStatus: "error",
		},
		{name: "missing name", body: `{"involvedObject":{"kind":"Kustomization"}}`, wantErr: true},
		{name: "malformed", body: `nope`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev, err := FluxHandler{}.Parse(req(t, nil), []byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", ev)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ev.Application != tc.wantApp || ev.Status != tc.wantStatus || ev.Revision != tc.wantRev {
				t.Errorf("got app=%q status=%q rev=%q; want app=%q status=%q rev=%q",
					ev.Application, ev.Status, ev.Revision, tc.wantApp, tc.wantStatus, tc.wantRev)
			}
		})
	}
}

func TestGitHubHandler_Parse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		event      string
		body       string
		wantErr    bool
		wantApp    string
		wantRev    string
		wantStatus string
		wantID     string
	}{
		{
			name:       "deployment_status",
			event:      "deployment_status",
			body:       `{"repository":{"full_name":"acme/web"},"deployment_status":{"state":"success"},"deployment":{"sha":"f00","environment":"prod"}}`,
			wantApp:    "acme/web",
			wantRev:    "f00",
			wantStatus: "success",
			wantID:     "delivery-1",
		},
		{
			name:       "workflow_run conclusion",
			event:      "workflow_run",
			body:       `{"repository":{"full_name":"acme/api"},"workflow_run":{"head_sha":"beef","status":"completed","conclusion":"failure"}}`,
			wantApp:    "acme/api",
			wantRev:    "beef",
			wantStatus: "failure",
		},
		{
			name:       "push fallback",
			event:      "push",
			body:       `{"repository":{"full_name":"acme/infra"},"ref":"refs/heads/main","after":"cafe"}`,
			wantApp:    "acme/infra",
			wantRev:    "cafe",
			wantStatus: "push",
		},
		{name: "missing event header", event: "", body: `{"repository":{"full_name":"x/y"}}`, wantErr: true},
		{name: "missing repo", event: "push", body: `{}`, wantErr: true},
		{name: "malformed", event: "push", body: `{`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := map[string]string{}
			if tc.event != "" {
				h["X-GitHub-Event"] = tc.event
			}
			if tc.wantID != "" {
				h["X-GitHub-Delivery"] = tc.wantID
			}
			ev, err := GitHubHandler{}.Parse(req(t, h), []byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", ev)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ev.Application != tc.wantApp || ev.Revision != tc.wantRev ||
				ev.Status != tc.wantStatus || ev.WebhookID != tc.wantID {
				t.Errorf("got app=%q rev=%q status=%q id=%q; want app=%q rev=%q status=%q id=%q",
					ev.Application, ev.Revision, ev.Status, ev.WebhookID,
					tc.wantApp, tc.wantRev, tc.wantStatus, tc.wantID)
			}
		})
	}
}

func TestGitLabHandler_Parse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		body       string
		wantErr    bool
		wantApp    string
		wantRev    string
		wantStatus string
	}{
		{
			name:       "pipeline",
			body:       `{"object_kind":"pipeline","project":{"path_with_namespace":"grp/app"},"object_attributes":{"status":"success","sha":"aaa"}}`,
			wantApp:    "grp/app",
			wantRev:    "aaa",
			wantStatus: "success",
		},
		{
			name:       "deployment",
			body:       `{"object_kind":"deployment","status":"running","sha":"bbb","project":{"path_with_namespace":"grp/svc"}}`,
			wantApp:    "grp/svc",
			wantRev:    "bbb",
			wantStatus: "running",
		},
		{
			name:       "push fallback",
			body:       `{"object_kind":"push","checkout_sha":"ccc","project":{"path_with_namespace":"grp/infra"}}`,
			wantApp:    "grp/infra",
			wantRev:    "ccc",
			wantStatus: "push",
		},
		{name: "missing object_kind", body: `{"project":{"path_with_namespace":"a/b"}}`, wantErr: true},
		{name: "missing project", body: `{"object_kind":"push"}`, wantErr: true},
		{name: "malformed", body: `{`, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			ev, err := GitLabHandler{}.Parse(req(t, map[string]string{"X-Gitlab-Event-UUID": "uuid-9"}), []byte(tc.body))
			if tc.wantErr {
				if err == nil {
					t.Fatalf("want error, got %+v", ev)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ev.Application != tc.wantApp || ev.Revision != tc.wantRev ||
				ev.Status != tc.wantStatus || ev.WebhookID != "uuid-9" {
				t.Errorf("got app=%q rev=%q status=%q id=%q; want app=%q rev=%q status=%q id=uuid-9",
					ev.Application, ev.Revision, ev.Status, ev.WebhookID,
					tc.wantApp, tc.wantRev, tc.wantStatus)
			}
		})
	}
}
