package trigger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestServiceNowTrigger_Validate(t *testing.T) {
	tests := []struct {
		name    string
		trigger *ServiceNowTrigger
		wantErr bool
	}{
		{
			name: "valid trigger",
			trigger: &ServiceNowTrigger{
				ID:         "sn-trigger",
				Name:       "ServiceNow Trigger",
				RunbookRef: RunbookRef{Name: "remediate"},
				Enabled:    true,
			},
			wantErr: false,
		},
		{
			name: "valid trigger with change request",
			trigger: &ServiceNowTrigger{
				ID:                  "sn-change",
				Name:                "ServiceNow Change",
				RunbookRef:          RunbookRef{Name: "deploy"},
				CreateChangeRequest: true,
				WaitForApproval:     true,
				Enabled:             true,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			trigger: &ServiceNowTrigger{
				Name:       "Test",
				RunbookRef: RunbookRef{Name: "test"},
			},
			wantErr: true,
		},
		{
			name: "missing name",
			trigger: &ServiceNowTrigger{
				ID:         "test",
				RunbookRef: RunbookRef{Name: "test"},
			},
			wantErr: true,
		},
		{
			name: "missing runbook",
			trigger: &ServiceNowTrigger{
				ID:   "test",
				Name: "Test",
			},
			wantErr: true,
		},
		{
			name: "wait for approval without change request",
			trigger: &ServiceNowTrigger{
				ID:              "test",
				Name:            "Test",
				RunbookRef:      RunbookRef{Name: "test"},
				WaitForApproval: true,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.trigger.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceNowClient_ParseWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})
	if err != nil {
		t.Fatalf("NewServiceNowClient() error = %v", err)
	}

	payload := map[string]interface{}{
		"sys_id":            "abc123",
		"number":            "INC0012345",
		"short_description": "Server down",
		"description":       "Production server is not responding",
		"state":             "1",
		"priority":          "1",
		"assignment_group":  "ops-team",
		"category":          "hardware",
		"action":            "insert",
		"table_name":        "incident",
		"opened_at":         "2024-01-15 10:30:00",
	}

	body, _ := json.Marshal(payload)
	incident, err := client.ParseWebhook(body)
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}

	if incident.ID != "abc123" {
		t.Errorf("ID = %v, want abc123", incident.ID)
	}
	if incident.ExternalID != "INC0012345" {
		t.Errorf("Number = %v, want INC0012345", incident.ExternalID)
	}
	if incident.Title != "Server down" {
		t.Errorf("Title = %v, want Server down", incident.Title)
	}
	if incident.Severity != IncidentSeverityCritical {
		t.Errorf("Severity = %v, want %v", incident.Severity, IncidentSeverityCritical)
	}
	if incident.Status != IncidentStatusTriggered {
		t.Errorf("Status = %v, want %v", incident.Status, IncidentStatusTriggered)
	}
	if incident.Service != "ops-team" {
		t.Errorf("Service = %v, want ops-team", incident.Service)
	}
}

func TestServiceNowClient_ParseWebhook_InvalidJSON(t *testing.T) {
	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: "https://test.service-now.com",
		Username:    "admin",
		Password:    "secret",
	})

	_, err := client.ParseWebhook([]byte("invalid json"))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestServiceNowClient_CreateChangeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/now/table/change_request" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Errorf("Unexpected method: %s", r.Method)
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"sys_id":            "cr123",
				"number":            "CHG0001234",
				"short_description": "Test change",
			},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	cr, err := client.CreateChangeRequest(context.Background(), &ChangeRequest{
		ShortDescription: "Test change",
		Type:             "normal",
	})
	if err != nil {
		t.Fatalf("CreateChangeRequest() error = %v", err)
	}

	if cr.SysID != "cr123" {
		t.Errorf("SysID = %v, want cr123", cr.SysID)
	}
	if cr.Number != "CHG0001234" {
		t.Errorf("Number = %v, want CHG0001234", cr.Number)
	}
}

func TestServiceNowClient_GetChangeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/now/table/change_request/cr123" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"sys_id":            "cr123",
					"number":            "CHG0001234",
					"short_description": "Test change",
					"state":             -2,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	cr, err := client.GetChangeRequest(context.Background(), "cr123")
	if err != nil {
		t.Fatalf("GetChangeRequest() error = %v", err)
	}

	if cr.SysID != "cr123" {
		t.Errorf("SysID = %v, want cr123", cr.SysID)
	}
}

func TestServiceNowClient_UpdateChangeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Errorf("Unexpected method: %s", r.Method)
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"sys_id":     "cr123",
				"work_notes": "Updated",
			},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	_, err := client.UpdateChangeRequest(context.Background(), "cr123", &ChangeRequest{
		WorkNotes: "Updated",
	})
	if err != nil {
		t.Fatalf("UpdateChangeRequest() error = %v", err)
	}
}

func TestServiceNowClient_CloseChangeRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"sys_id":      "cr123",
				"state":       3,
				"close_code":  "successful",
				"close_notes": "Done",
			},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	err := client.CloseChangeRequest(context.Background(), "cr123", "successful", "Done")
	if err != nil {
		t.Fatalf("CloseChangeRequest() error = %v", err)
	}
}

func TestServiceNowClient_GetCMDBCI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/now/table/cmdb_ci/ci123" {
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{
					"sys_id":        "ci123",
					"name":          "web-server-01",
					"sys_class_name": "cmdb_ci_server",
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	ci, err := client.GetCMDBCI(context.Background(), "ci123")
	if err != nil {
		t.Fatalf("GetCMDBCI() error = %v", err)
	}

	if ci.SysID != "ci123" {
		t.Errorf("SysID = %v, want ci123", ci.SysID)
	}
	if ci.Name != "web-server-01" {
		t.Errorf("Name = %v, want web-server-01", ci.Name)
	}
}

func TestServiceNowClient_UpdateCMDBCI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"sys_id":            "ci123",
				"operational_status": 1,
			},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	_, err := client.UpdateCMDBCI(context.Background(), "ci123", &CMDBConfigurationItem{
		OperationalStatus: 1,
	})
	if err != nil {
		t.Fatalf("UpdateCMDBCI() error = %v", err)
	}
}

func TestServiceNowClient_GetApprovals(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"sys_id":      "app1",
					"state":       "approved",
					"approver":    "admin",
					"document_id": "cr123",
				},
				{
					"sys_id":      "app2",
					"state":       "approved",
					"approver":    "manager",
					"document_id": "cr123",
				},
			},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	approvals, err := client.GetApprovals(context.Background(), "cr123")
	if err != nil {
		t.Fatalf("GetApprovals() error = %v", err)
	}

	if len(approvals) != 2 {
		t.Errorf("Got %d approvals, want 2", len(approvals))
	}
}

func TestServiceNowClient_IsApproved(t *testing.T) {
	tests := []struct {
		name      string
		approvals []map[string]interface{}
		want      bool
	}{
		{
			name: "all approved",
			approvals: []map[string]interface{}{
				{"sys_id": "app1", "state": "approved"},
				{"sys_id": "app2", "state": "approved"},
			},
			want: true,
		},
		{
			name: "one pending",
			approvals: []map[string]interface{}{
				{"sys_id": "app1", "state": "approved"},
				{"sys_id": "app2", "state": "requested"},
			},
			want: false,
		},
		{
			name:      "no approvals required",
			approvals: []map[string]interface{}{},
			want:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"result": tt.approvals,
				})
			}))
			defer server.Close()

			client, _ := NewServiceNowClient(ServiceNowConfig{
				InstanceURL: server.URL,
				Username:    "admin",
				Password:    "secret",
			})

			approved, err := client.IsApproved(context.Background(), "cr123")
			if err != nil {
				t.Fatalf("IsApproved() error = %v", err)
			}

			if approved != tt.want {
				t.Errorf("IsApproved() = %v, want %v", approved, tt.want)
			}
		})
	}
}

func TestServiceNowClient_AcknowledgeIncident(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/now/table/incident/inc123" {
			t.Errorf("Unexpected path: %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"sys_id": "inc123",
				"state":  "2",
			},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	err := client.AcknowledgeIncident(context.Background(), "inc123")
	if err != nil {
		t.Fatalf("AcknowledgeIncident() error = %v", err)
	}
}

func TestServiceNowClient_ResolveIncident(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"sys_id": "inc123",
				"state":  "6",
			},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	err := client.ResolveIncident(context.Background(), "inc123")
	if err != nil {
		t.Fatalf("ResolveIncident() error = %v", err)
	}
}

func TestServiceNowClient_AddNote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"sys_id": "inc123",
			},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	err := client.AddNote(context.Background(), "inc123", "Test note")
	if err != nil {
		t.Fatalf("AddNote() error = %v", err)
	}
}

func TestServiceNowClient_GetIncident(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{
				"sys_id":            "inc123",
				"number":            "INC0012345",
				"short_description": "Test incident",
				"state":             "1",
				"priority":          "2",
			},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	incident, err := client.GetIncident(context.Background(), "inc123")
	if err != nil {
		t.Fatalf("GetIncident() error = %v", err)
	}

	if incident.ID != "inc123" {
		t.Errorf("ID = %v, want inc123", incident.ID)
	}
	if incident.ExternalID != "INC0012345" {
		t.Errorf("Number = %v, want INC0012345", incident.ExternalID)
	}
	if incident.Severity != IncidentSeverityHigh {
		t.Errorf("Severity = %v, want %v", incident.Severity, IncidentSeverityHigh)
	}
}

func TestNewServiceNowClient_Validation(t *testing.T) {
	tests := []struct {
		name    string
		config  ServiceNowConfig
		wantErr bool
	}{
		{
			name: "valid basic auth",
			config: ServiceNowConfig{
				InstanceURL: "https://test.service-now.com",
				Username:    "admin",
				Password:    "secret",
			},
			wantErr: false,
		},
		{
			name: "valid oauth",
			config: ServiceNowConfig{
				InstanceURL:  "https://test.service-now.com",
				ClientID:     "client123",
				ClientSecret: "secret456",
			},
			wantErr: false,
		},
		{
			name: "missing instance URL",
			config: ServiceNowConfig{
				Username: "admin",
				Password: "secret",
			},
			wantErr: true,
		},
		{
			name: "missing auth",
			config: ServiceNowConfig{
				InstanceURL: "https://test.service-now.com",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewServiceNowClient(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewServiceNowClient() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestServiceNowTriggerManager_RegisterAndGet(t *testing.T) {
	manager := NewServiceNowTriggerManager(nil, nil, nil, nil)

	trigger := &ServiceNowTrigger{
		ID:         "sn-trigger",
		Name:       "ServiceNow Trigger",
		RunbookRef: RunbookRef{Name: "remediate"},
		Enabled:    true,
	}

	err := manager.Register(trigger)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Get should work
	got, ok := manager.Get("sn-trigger")
	if !ok {
		t.Fatal("Get() returned false")
	}
	if got.Name != "ServiceNow Trigger" {
		t.Errorf("Name = %v, want ServiceNow Trigger", got.Name)
	}

	// List should include trigger
	triggers := manager.List()
	if len(triggers) != 1 {
		t.Errorf("List() len = %d, want 1", len(triggers))
	}

	// Duplicate registration should fail
	err = manager.Register(trigger)
	if err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Unregister
	err = manager.Unregister("sn-trigger")
	if err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	// Get should fail
	_, ok = manager.Get("sn-trigger")
	if ok {
		t.Error("Get() should return false after unregister")
	}
}

func TestServiceNowTriggerManager_EnableDisable(t *testing.T) {
	manager := NewServiceNowTriggerManager(nil, nil, nil, nil)

	trigger := &ServiceNowTrigger{
		ID:         "sn-trigger",
		Name:       "ServiceNow Trigger",
		RunbookRef: RunbookRef{Name: "remediate"},
		Enabled:    true,
	}

	_ = manager.Register(trigger)

	// Disable
	err := manager.Disable("sn-trigger")
	if err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	got, _ := manager.Get("sn-trigger")
	if got.Enabled {
		t.Error("Trigger should be disabled")
	}

	// Enable
	err = manager.Enable("sn-trigger")
	if err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	got, _ = manager.Get("sn-trigger")
	if !got.Enabled {
		t.Error("Trigger should be enabled")
	}
}

func TestServiceNowTriggerManager_HandleWebhook(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/now/table/incident/inc123" {
			if r.Method == http.MethodGet {
				w.WriteHeader(http.StatusOK)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"result": map[string]interface{}{
						"sys_id":            "inc123",
						"number":            "INC0012345",
						"short_description": "Test incident",
						"state":             "1",
					},
				})
				return
			}
			// PATCH for acknowledge/notes
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"result": map[string]interface{}{"sys_id": "inc123"},
			})
			return
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	repo := &mockRunbookRepository{
		runbooks: map[string]*runbook.Runbook{
			"remediate": {
				Metadata: runbook.Metadata{Name: "remediate"},
			},
		},
	}

	executor := newMockExecutor()

	manager := NewServiceNowTriggerManager(client, repo, executor, nil)

	trigger := &ServiceNowTrigger{
		ID:                 "sn-trigger",
		Name:               "ServiceNow Trigger",
		RunbookRef:         RunbookRef{Name: "remediate"},
		TableFilter:        []string{"incident"},
		AcknowledgeOnStart: true,
		Enabled:            true,
	}

	_ = manager.Register(trigger)

	payload := map[string]interface{}{
		"sys_id":            "inc123",
		"number":            "INC0012345",
		"short_description": "Server down",
		"state":             "1",
		"priority":          "1",
		"assignment_group":  "ops-team",
		"action":            "insert",
		"table_name":        "incident",
	}

	body, _ := json.Marshal(payload)
	exec, err := manager.HandleWebhook(context.Background(), body)
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}

	if exec == nil {
		t.Fatal("HandleWebhook() returned nil execution")
	}

	if exec.Status != "completed" {
		t.Errorf("Status = %v, want completed", exec.Status)
	}

	if exec.IncidentID != "inc123" {
		t.Errorf("IncidentID = %v, want inc123", exec.IncidentID)
	}
}

func TestServiceNowTriggerManager_TableFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	manager := NewServiceNowTriggerManager(client, nil, nil, nil)

	trigger := &ServiceNowTrigger{
		ID:          "sn-trigger",
		Name:        "Incident Only",
		RunbookRef:  RunbookRef{Name: "remediate"},
		TableFilter: []string{"incident"},
		Enabled:     true,
	}

	_ = manager.Register(trigger)

	// Change request should not match
	payload := map[string]interface{}{
		"sys_id":     "cr123",
		"number":     "CHG0001234",
		"table_name": "change_request",
	}

	body, _ := json.Marshal(payload)
	exec, err := manager.HandleWebhook(context.Background(), body)
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}

	if exec != nil {
		t.Error("Expected no execution for non-matching table")
	}
}

func TestServiceNowTriggerManager_CategoryFilter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"result": map[string]interface{}{},
		})
	}))
	defer server.Close()

	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: server.URL,
		Username:    "admin",
		Password:    "secret",
	})

	manager := NewServiceNowTriggerManager(client, nil, nil, nil)

	trigger := &ServiceNowTrigger{
		ID:             "sn-trigger",
		Name:           "Hardware Only",
		RunbookRef:     RunbookRef{Name: "remediate"},
		CategoryFilter: []string{"hardware"},
		Enabled:        true,
	}

	_ = manager.Register(trigger)

	// Software category should not match
	payload := map[string]interface{}{
		"sys_id":     "inc123",
		"category":   "software",
		"table_name": "incident",
	}

	body, _ := json.Marshal(payload)
	exec, err := manager.HandleWebhook(context.Background(), body)
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}

	if exec != nil {
		t.Error("Expected no execution for non-matching category")
	}
}

func TestServiceNowTriggerManager_NoClient(t *testing.T) {
	manager := NewServiceNowTriggerManager(nil, nil, nil, nil)

	_, err := manager.HandleWebhook(context.Background(), []byte("{}"))
	if err == nil {
		t.Error("Expected error when client is not configured")
	}
}

func TestServiceNowStateMapping(t *testing.T) {
	tests := []struct {
		state string
		want  IncidentStatus
	}{
		{"1", IncidentStatusTriggered},
		{"2", IncidentStatusAcknowledged},
		{"6", IncidentStatusResolved},
		{"7", IncidentStatusResolved},
		{"8", IncidentStatusResolved},
		{"99", IncidentStatusTriggered}, // unknown defaults to triggered
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got := mapServiceNowState(tt.state)
			if got != tt.want {
				t.Errorf("mapServiceNowState(%s) = %v, want %v", tt.state, got, tt.want)
			}
		})
	}
}

func TestServiceNowPriorityMapping(t *testing.T) {
	tests := []struct {
		priority string
		want     IncidentSeverity
	}{
		{"1", IncidentSeverityCritical},
		{"2", IncidentSeverityHigh},
		{"3", IncidentSeverityMedium},
		{"4", IncidentSeverityLow},
		{"5", IncidentSeverityInfo},
		{"99", IncidentSeverityMedium}, // unknown defaults to medium
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			got := mapServiceNowPriority(tt.priority)
			if got != tt.want {
				t.Errorf("mapServiceNowPriority(%s) = %v, want %v", tt.priority, got, tt.want)
			}
		})
	}
}

func TestResolveIncidentMapping(t *testing.T) {
	incident := &Incident{
		ID:          "inc123",
		ExternalID:  "INC0012345",
		Title:       "Test Incident",
		Description: "Test description",
		Severity:    IncidentSeverityHigh,
		Status:      IncidentStatusTriggered,
		Service:     "ops-team",
		RawData: map[string]interface{}{
			"category":    "hardware",
			"subcategory": "server",
		},
	}

	tests := []struct {
		mapping string
		want    interface{}
	}{
		{"incident.id", "inc123"},
		{"incident.number", "INC0012345"},
		{"incident.external_id", "INC0012345"},
		{"incident.title", "Test Incident"},
		{"incident.description", "Test description"},
		{"incident.severity", "high"},
		{"incident.status", "triggered"},
		{"incident.service", "ops-team"},
		{"incident.raw_data.category", "hardware"},
		{"incident.raw_data.subcategory", "server"},
		{"incident.raw_data.nonexistent", "incident.raw_data.nonexistent"}, // returns original
		{"literal_value", "literal_value"},                                  // unknown returns as-is
	}

	for _, tt := range tests {
		t.Run(tt.mapping, func(t *testing.T) {
			got := resolveIncidentMapping(tt.mapping, incident)
			if got != tt.want {
				t.Errorf("resolveIncidentMapping(%s) = %v, want %v", tt.mapping, got, tt.want)
			}
		})
	}
}

func TestServiceNowClient_Type(t *testing.T) {
	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: "https://test.service-now.com",
		Username:    "admin",
		Password:    "secret",
	})

	if client.Type() != ITSMTypeServiceNow {
		t.Errorf("Type() = %v, want %v", client.Type(), ITSMTypeServiceNow)
	}
}

func TestServiceNowTriggerManager_GetExecution(t *testing.T) {
	manager := NewServiceNowTriggerManager(nil, nil, nil, nil)

	// Should not find non-existent execution
	_, ok := manager.GetExecution("nonexistent")
	if ok {
		t.Error("Expected GetExecution to return false for non-existent ID")
	}
}

func TestServiceNowTriggerManager_GetClient(t *testing.T) {
	client, _ := NewServiceNowClient(ServiceNowConfig{
		InstanceURL: "https://test.service-now.com",
		Username:    "admin",
		Password:    "secret",
	})

	manager := NewServiceNowTriggerManager(client, nil, nil, nil)

	if manager.GetClient() != client {
		t.Error("GetClient() should return the configured client")
	}
}
