// Copyright 2024 Keystone Core Contributors
// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTriggerRollback_Success(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	expected := rollbackResponse{
		ID:               "rb-001",
		Application:      "nginx",
		Namespace:        "default",
		Type:             "argocd",
		Strategy:         "previous",
		Status:           "completed",
		PreviousRevision: "abc123",
		CurrentRevision:  "def456",
		Message:          "Rollback successful",
		Duration:         "2.3s",
		StartTime:        now,
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/api/v1/gitops/rollback" {
			t.Errorf("expected /api/v1/gitops/rollback, got %s", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("expected Content-Type application/json, got %s", r.Header.Get("Content-Type"))
		}

		var req rollbackRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}
		if req.Application != "nginx" {
			t.Errorf("expected application nginx, got %s", req.Application)
		}
		if req.Reason != "test rollback" {
			t.Errorf("expected reason 'test rollback', got %s", req.Reason)
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(expected)
	}))
	defer srv.Close()

	client := newRESTClient(srv.URL)
	resp, err := client.TriggerRollback(rollbackRequest{
		Application: "nginx",
		Namespace:   "default",
		Type:        "argocd",
		Strategy:    "previous",
		Reason:      "test rollback",
		RequestedBy: "admin",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.ID != "rb-001" {
		t.Errorf("expected ID rb-001, got %s", resp.ID)
	}
	if resp.Status != "completed" {
		t.Errorf("expected status completed, got %s", resp.Status)
	}
	if resp.Application != "nginx" {
		t.Errorf("expected application nginx, got %s", resp.Application)
	}
}

func TestTriggerRollback_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(apiError{
			Error:   "bad_request",
			Message: "application is required",
		})
	}))
	defer srv.Close()

	client := newRESTClient(srv.URL)
	_, err := client.TriggerRollback(rollbackRequest{})
	if err == nil {
		t.Fatal("expected error for 400 response")
	}
	if got := err.Error(); got != "rollback failed (400): application is required" {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestTriggerRollback_ServerErrorRawBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer srv.Close()

	client := newRESTClient(srv.URL)
	_, err := client.TriggerRollback(rollbackRequest{Application: "test", Type: "argocd", Reason: "test"})
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
	if got := err.Error(); got != "rollback failed (500): internal error" {
		t.Errorf("unexpected error message: %s", got)
	}
}

func TestTriggerRollback_ConnectionError(t *testing.T) {
	client := newRESTClient("http://127.0.0.1:1")
	_, err := client.TriggerRollback(rollbackRequest{Application: "test", Type: "argocd", Reason: "test"})
	if err == nil {
		t.Fatal("expected connection error")
	}
	if got := err.Error(); !contains(got, "failed to connect") {
		t.Errorf("expected connection error, got: %s", got)
	}
}

func TestNewRESTClient(t *testing.T) {
	client := newRESTClient("http://localhost:8080")
	if client.baseURL != "http://localhost:8080" {
		t.Errorf("expected baseURL http://localhost:8080, got %s", client.baseURL)
	}
	if client.httpClient == nil {
		t.Error("expected httpClient to not be nil")
	}
	if client.httpClient.Timeout != 30*time.Second {
		t.Errorf("expected 30s timeout, got %v", client.httpClient.Timeout)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
