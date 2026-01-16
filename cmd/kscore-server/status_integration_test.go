package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/controlplane"
)

func TestStatusEndpointIntegration(t *testing.T) {
	connMgr := controlplane.NewConnectionManager(nil)
	startTime := time.Now().Add(-2 * time.Minute)

	mux := http.NewServeMux()
	mux.HandleFunc("/health/live", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		statusJSON := buildServerStatusJSON(connMgr, startTime)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(statusJSON)
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	resp, err := http.Get(server.URL + "/health/live")
	if err != nil {
		t.Fatalf("GET /health/live error = %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/health/live status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	resp, err = http.Get(server.URL + "/api/status")
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
