// SPDX-License-Identifier: Apache-2.0

package argoexec

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestClient_GetApplication(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/applications/web" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"status":{"sync":{"revision":"r3"},"history":[{"id":1,"revision":"r1"},{"id":2,"revision":"r2"},{"id":3,"revision":"r3"}]}}`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{BaseURL: srv.URL, Token: "tok"}
	app, err := c.GetApplication(context.Background(), "web")
	if err != nil {
		t.Fatalf("GetApplication: %v", err)
	}
	if app.SyncRevision != "r3" || len(app.History) != 3 {
		t.Fatalf("app = %+v", app)
	}
	if app.History[0].Revision != "r1" || app.History[2].ID != 3 {
		t.Errorf("history mapping wrong: %+v", app.History)
	}
}

func TestClient_SyncToRevision(t *testing.T) {
	t.Parallel()
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/sync") {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &gotBody)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)

	if err := (&Client{BaseURL: srv.URL}).SyncToRevision(context.Background(), "web", "r2"); err != nil {
		t.Fatalf("SyncToRevision: %v", err)
	}
	if gotBody["revision"] != "r2" {
		t.Errorf("posted body = %v, want revision=r2", gotBody)
	}
}

func TestClient_ErrorStatus(t *testing.T) {
	t.Parallel()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`permission denied`))
	}))
	t.Cleanup(srv.Close)

	c := &Client{BaseURL: srv.URL}
	if _, err := c.GetApplication(context.Background(), "web"); err == nil ||
		!strings.Contains(err.Error(), "403") {
		t.Errorf("err = %v, want 403", err)
	}
	if err := c.SyncToRevision(context.Background(), "web", "r1"); err == nil {
		t.Error("SyncToRevision on 403 = nil, want error")
	}
}
