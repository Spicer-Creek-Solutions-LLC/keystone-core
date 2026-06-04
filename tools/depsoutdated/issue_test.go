// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestIssueBody(t *testing.T) {
	got := issueBody("deps-outdated: 1 minor/patch update(s) available:\n  foo v1 => v1.1\n")
	if !strings.Contains(got, "```\ndeps-outdated: 1 minor/patch update(s) available:\n  foo v1 => v1.1\n```") {
		t.Errorf("body missing fenced report:\n%s", got)
	}
	if !strings.Contains(got, "ci-full.yml") {
		t.Errorf("body missing provenance note:\n%s", got)
	}
}

// recorder captures the requests syncIssue makes so each branch can be
// asserted without a real forge.
type recorder struct {
	calls []call
}

type call struct {
	method string
	path   string
	body   map[string]any
}

// newServer returns an httptest server whose GET /issues lists the given
// open issues, and records create/patch calls into rec.
func newServer(t *testing.T, rec *recorder, open []map[string]any) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if r.Body != nil {
			if data, _ := io.ReadAll(r.Body); len(data) > 0 {
				_ = json.Unmarshal(data, &body)
			}
		}
		rec.calls = append(rec.calls, call{method: r.Method, path: r.URL.Path, body: body})
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/issues"):
			_ = json.NewEncoder(w).Encode(open)
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"number": 7})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func setEnv(t *testing.T, apiURL string) {
	t.Helper()
	t.Setenv("GITHUB_API_URL", apiURL)
	t.Setenv("GITHUB_REPOSITORY", "owner/repo")
	t.Setenv("GITHUB_TOKEN", "tok")
}

func TestSyncIssue_CreatesWhenOutdatedAndNoneOpen(t *testing.T) {
	rec := &recorder{}
	srv := newServer(t, rec, nil)
	setEnv(t, srv.URL)

	syncIssue(true, "report")

	if len(rec.calls) != 2 || rec.calls[1].method != http.MethodPost {
		t.Fatalf("expected GET then POST create, got %+v", rec.calls)
	}
	if rec.calls[1].body["title"] != issueTitle {
		t.Errorf("create title = %v, want %q", rec.calls[1].body["title"], issueTitle)
	}
}

func TestSyncIssue_UpdatesWhenOutdatedAndOpen(t *testing.T) {
	rec := &recorder{}
	srv := newServer(t, rec, []map[string]any{{"number": 42, "title": issueTitle}})
	setEnv(t, srv.URL)

	syncIssue(true, "report")

	if len(rec.calls) != 2 || rec.calls[1].method != http.MethodPatch {
		t.Fatalf("expected GET then PATCH update, got %+v", rec.calls)
	}
	if !strings.HasSuffix(rec.calls[1].path, "/issues/42") {
		t.Errorf("patched wrong issue: %s", rec.calls[1].path)
	}
	if _, closed := rec.calls[1].body["state"]; closed {
		t.Error("update must not set state when still outdated")
	}
}

func TestSyncIssue_ClosesWhenCurrentAndOpen(t *testing.T) {
	rec := &recorder{}
	srv := newServer(t, rec, []map[string]any{{"number": 42, "title": issueTitle}})
	setEnv(t, srv.URL)

	syncIssue(false, "all current")

	if len(rec.calls) != 2 || rec.calls[1].method != http.MethodPatch {
		t.Fatalf("expected GET then PATCH close, got %+v", rec.calls)
	}
	if rec.calls[1].body["state"] != "closed" {
		t.Errorf("expected state=closed, got %v", rec.calls[1].body["state"])
	}
}

func TestSyncIssue_NoopWhenCurrentAndNoneOpen(t *testing.T) {
	rec := &recorder{}
	srv := newServer(t, rec, nil)
	setEnv(t, srv.URL)

	syncIssue(false, "all current")

	if len(rec.calls) != 1 || rec.calls[0].method != http.MethodGet {
		t.Fatalf("expected only the GET lookup, got %+v", rec.calls)
	}
}

func TestSyncIssue_SkipsWhenEnvMissing(t *testing.T) {
	t.Setenv("GITHUB_API_URL", "")
	t.Setenv("GITHUB_REPOSITORY", "")
	t.Setenv("GITHUB_TOKEN", "")
	// Must not panic or make any call; nothing to assert beyond returning.
	syncIssue(true, "report")
}
