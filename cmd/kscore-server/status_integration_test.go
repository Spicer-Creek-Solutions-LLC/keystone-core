package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/controlplane"
)

func TestStatusEndpointIntegration(t *testing.T) {
	connMgr := controlplane.NewConnectionManager(nil)
	startTime := time.Now().Add(-2 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		writeJSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		status := buildServerStatusResponse(connMgr, startTime)
		writeJSONResponse(w, http.StatusOK, status)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/health/live", nil)
	if err != nil {
		t.Fatalf("create /health/live request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /health/live error = %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/health/live status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	req, err = http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/status", nil)
	if err != nil {
		t.Fatalf("create /api/status request: %v", err)
	}
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/status error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/status status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	var payload map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode /api/status response: %v", err)
	}
	resp.Body.Close()

	if payload["version"] == "" {
		t.Fatal("expected version in status payload")
	}
	agents, ok := payload["agents"].(map[string]interface{})
	if !ok {
		t.Fatal("expected agents in status payload")
	}
	if agents["total"] == nil || agents["online"] == nil {
		t.Fatal("expected agent counts in status payload")
	}
}
