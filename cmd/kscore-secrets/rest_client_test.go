package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	apisecrets "github.com/shawnbutts/keystone-core/pkg/api/secrets"
)

func TestRESTClient_ListBackends(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/backends" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodGet {
			t.Errorf("unexpected method: %s", r.Method)
		}
		json.NewEncoder(w).Encode(apisecrets.BackendListResponse{
			Backends: []*apisecrets.BackendInfoResponse{
				{Name: "vault", Type: "vault", Healthy: true},
			},
			Total: 1,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.ListBackends()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Errorf("expected 1 backend, got %d", resp.Total)
	}
	if resp.Backends[0].Name != "vault" {
		t.Errorf("expected name 'vault', got %q", resp.Backends[0].Name)
	}
}

func TestRESTClient_GetBackend(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/backends/vault" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.BackendInfoResponse{
			Name: "vault", Type: "vault", Healthy: true,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.GetBackend("vault")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Name != "vault" {
		t.Errorf("expected 'vault', got %q", resp.Name)
	}
}

func TestRESTClient_ListAuditEntries(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/audit/logs" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.URL.Query().Get("path") != "secret/test" {
			t.Errorf("expected path param 'secret/test', got %q", r.URL.Query().Get("path"))
		}
		if r.URL.Query().Get("limit") != "10" {
			t.Errorf("expected limit=10, got %q", r.URL.Query().Get("limit"))
		}
		json.NewEncoder(w).Encode(apisecrets.AuditLogResponse{
			Events: []*apisecrets.AuditEventResponse{
				{SecretPath: "secret/test", Action: "read", Success: true},
			},
			Total: 1,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.ListAuditEntries(AuditListOpts{
		Path:  "secret/test",
		Limit: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Errorf("expected 1 event, got %d", resp.Total)
	}
}

func TestRESTClient_GetCacheStats(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/cache/stats" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.CacheStatsResponse{
			Entries:    10,
			MaxEntries: 1000,
			Hits:       50,
			Misses:     5,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.GetCacheStats()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Entries != 10 {
		t.Errorf("expected 10 entries, got %d", resp.Entries)
	}
	if resp.Hits != 50 {
		t.Errorf("expected 50 hits, got %d", resp.Hits)
	}
}

func TestRESTClient_ClearCache(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/cache" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodDelete {
			t.Errorf("unexpected method: %s", r.Method)
		}
		json.NewEncoder(w).Encode(apisecrets.CacheClearResponse{
			Message: "cache cleared",
			Cleared: 10,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.ClearCache()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Cleared != 10 {
		t.Errorf("expected 10 cleared, got %d", resp.Cleared)
	}
}

func TestRESTClient_ServerError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"error": "something went wrong"})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	_, err := client.ListBackends()
	if err == nil {
		t.Fatal("expected error for 500 response")
	}
}

func TestRESTClient_ListRotations(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rotations" || r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationListResponse{
			Rotations: []apisecrets.RotationResponse{
				{ID: "rot-1", SecretPath: "vault/db", State: "in_progress", Strategy: "rolling"},
			},
			Total: 1,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.ListRotations()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Errorf("expected 1 rotation, got %d", resp.Total)
	}
}

func TestRESTClient_GetRotation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rotations/rot-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationResponse{
			ID: "rot-1", SecretPath: "vault/db", State: "in_progress",
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.GetRotation("rot-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "rot-1" {
		t.Errorf("expected 'rot-1', got %q", resp.ID)
	}
}

func TestRESTClient_StartRotation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rotations" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(apisecrets.RotationResponse{
			ID: "rot-new", SecretPath: "vault/db", State: "pending",
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.StartRotation(&apisecrets.StartRotationRequest{
		ID: "rot-new", SecretPath: "vault/db",
		Config: apisecrets.RotationConfigRequest{Strategy: "rolling"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "rot-new" {
		t.Errorf("expected 'rot-new', got %q", resp.ID)
	}
}

func TestRESTClient_CancelRotation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rotations/rot-1/cancel" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationActionResponse{
			RotationID: "rot-1", Action: "cancel", Success: true,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.CancelRotation("rot-1")
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestRESTClient_RollbackRotation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rotations/rot-1/rollback" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationActionResponse{
			RotationID: "rot-1", Action: "rollback", Success: true,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.RollbackRotation("rot-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Action != "rollback" {
		t.Errorf("expected action 'rollback', got %q", resp.Action)
	}
}

func TestRESTClient_PauseRotation(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/rotations/rot-1/pause" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationActionResponse{
			RotationID: "rot-1", Action: "pause", Success: true,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.PauseRotation("rot-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Action != "pause" {
		t.Errorf("expected action 'pause', got %q", resp.Action)
	}
}

func TestRESTClient_ListRotationPolicies(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/rotation/policies" || r.Method != http.MethodGet {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationPolicyListResponse{
			Policies: []apisecrets.RotationPolicyResponse{
				{ID: "pol-1", Name: "test", MaxAge: "90d", Enabled: true},
			},
			Total: 1,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.ListRotationPolicies()
	if err != nil {
		t.Fatal(err)
	}
	if resp.Total != 1 {
		t.Errorf("expected 1 policy, got %d", resp.Total)
	}
	if resp.Policies[0].ID != "pol-1" {
		t.Errorf("expected ID 'pol-1', got %q", resp.Policies[0].ID)
	}
}

func TestRESTClient_GetRotationPolicy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/rotation/policies/pol-1" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationPolicyResponse{
			ID: "pol-1", Name: "test", MaxAge: "90d",
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.GetRotationPolicy("pol-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "pol-1" {
		t.Errorf("expected 'pol-1', got %q", resp.ID)
	}
}

func TestRESTClient_CreateRotationPolicy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/rotation/policies" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(apisecrets.RotationPolicyResponse{
			ID: "pol-new", Name: "new-policy", MaxAge: "90d",
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.CreateRotationPolicy(&apisecrets.CreateRotationPolicyRequest{
		ID: "pol-new", Name: "new-policy", MaxAge: "90d",
	})
	if err != nil {
		t.Fatal(err)
	}
	if resp.ID != "pol-new" {
		t.Errorf("expected 'pol-new', got %q", resp.ID)
	}
}

func TestRESTClient_DeleteRotationPolicy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/rotation/policies/pol-1" || r.Method != http.MethodDelete {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationPolicyActionResponse{
			PolicyID: "pol-1", Action: "delete", Success: true,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.DeleteRotationPolicy("pol-1")
	if err != nil {
		t.Fatal(err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
}

func TestRESTClient_EnableRotationPolicy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/rotation/policies/pol-1/enable" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationPolicyActionResponse{
			PolicyID: "pol-1", Action: "enable", Success: true,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.EnableRotationPolicy("pol-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Action != "enable" {
		t.Errorf("expected action 'enable', got %q", resp.Action)
	}
}

func TestRESTClient_DisableRotationPolicy(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/secrets/rotation/policies/pol-1/disable" || r.Method != http.MethodPost {
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
		json.NewEncoder(w).Encode(apisecrets.RotationPolicyActionResponse{
			PolicyID: "pol-1", Action: "disable", Success: true,
		})
	}))
	defer ts.Close()

	client := NewRESTClient(ts.URL)
	resp, err := client.DisableRotationPolicy("pol-1")
	if err != nil {
		t.Fatal(err)
	}
	if resp.Action != "disable" {
		t.Errorf("expected action 'disable', got %q", resp.Action)
	}
}

func TestRESTClient_ConnectionError(t *testing.T) {
	client := NewRESTClient("http://localhost:1")
	_, err := client.ListBackends()
	if err == nil {
		t.Fatal("expected error for connection failure")
	}
}
