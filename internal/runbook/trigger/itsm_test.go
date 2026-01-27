package trigger

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/shawnbutts/keystone-core/internal/runbook"
)

func TestITSMTrigger_Validate(t *testing.T) {
	tests := []struct {
		name    string
		trigger *ITSMTrigger
		wantErr bool
	}{
		{
			name: "valid pagerduty trigger",
			trigger: &ITSMTrigger{
				ID:         "pd-trigger",
				Name:       "PagerDuty Trigger",
				Type:       ITSMTypePagerDuty,
				RunbookRef: RunbookRef{Name: "remediate"},
				Enabled:    true,
			},
			wantErr: false,
		},
		{
			name: "valid opsgenie trigger",
			trigger: &ITSMTrigger{
				ID:         "og-trigger",
				Name:       "Opsgenie Trigger",
				Type:       ITSMTypeOpsgenie,
				RunbookRef: RunbookRef{Name: "remediate"},
				Enabled:    true,
			},
			wantErr: false,
		},
		{
			name: "missing id",
			trigger: &ITSMTrigger{
				Name:       "Test",
				Type:       ITSMTypePagerDuty,
				RunbookRef: RunbookRef{Name: "test"},
			},
			wantErr: true,
		},
		{
			name: "missing type",
			trigger: &ITSMTrigger{
				ID:         "test",
				Name:       "Test",
				RunbookRef: RunbookRef{Name: "test"},
			},
			wantErr: true,
		},
		{
			name: "invalid type",
			trigger: &ITSMTrigger{
				ID:         "test",
				Name:       "Test",
				Type:       ITSMType("invalid"),
				RunbookRef: RunbookRef{Name: "test"},
			},
			wantErr: true,
		},
		{
			name: "missing runbook",
			trigger: &ITSMTrigger{
				ID:   "test",
				Name: "Test",
				Type: ITSMTypePagerDuty,
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

func TestPagerDutyClient_ParseWebhook(t *testing.T) {
	client := NewPagerDutyClient(&PagerDutyConfig{APIKey: "test"})

	payload := `{
		"messages": [{
			"event": "incident.trigger",
			"incident": {
				"id": "PINCIDENT1",
				"title": "Server Down",
				"description": "Web server is not responding",
				"status": "triggered",
				"urgency": "high",
				"html_url": "https://example.pagerduty.com/incidents/PINCIDENT1",
				"created_at": "2024-01-15T10:00:00Z",
				"service": {
					"id": "PSERVICE1",
					"name": "Web Service"
				}
			}
		}]
	}`

	incident, err := client.ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}

	if incident.ExternalID != "PINCIDENT1" {
		t.Errorf("ExternalID = %q, want %q", incident.ExternalID, "PINCIDENT1")
	}
	if incident.Title != "Server Down" {
		t.Errorf("Title = %q, want %q", incident.Title, "Server Down")
	}
	if incident.Status != IncidentStatusTriggered {
		t.Errorf("Status = %v, want %v", incident.Status, IncidentStatusTriggered)
	}
	if incident.Severity != IncidentSeverityHigh {
		t.Errorf("Severity = %v, want %v", incident.Severity, IncidentSeverityHigh)
	}
	if incident.Service != "Web Service" {
		t.Errorf("Service = %q, want %q", incident.Service, "Web Service")
	}
}

func TestPagerDutyClient_ParseWebhook_InvalidJSON(t *testing.T) {
	client := NewPagerDutyClient(&PagerDutyConfig{APIKey: "test"})

	_, err := client.ParseWebhook([]byte(`{invalid}`))
	if err == nil {
		t.Error("Expected error for invalid JSON")
	}
}

func TestPagerDutyClient_ParseWebhook_EmptyMessages(t *testing.T) {
	client := NewPagerDutyClient(&PagerDutyConfig{APIKey: "test"})

	_, err := client.ParseWebhook([]byte(`{"messages":[]}`))
	if err == nil {
		t.Error("Expected error for empty messages")
	}
}

func TestPagerDutyClient_AcknowledgeIncident(t *testing.T) {
	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "PUT" {
			t.Errorf("Method = %s, want PUT", r.Method)
		}
		if r.URL.Path != "/incidents/PINCIDENT1" {
			t.Errorf("Path = %s, want /incidents/PINCIDENT1", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Token token=test-api-key" {
			t.Errorf("Missing or wrong Authorization header")
		}

		// Check request body
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("Failed to decode body: %v", err)
		}
		incident := body["incident"].(map[string]interface{})
		if incident["status"] != "acknowledged" {
			t.Errorf("Status = %v, want acknowledged", incident["status"])
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewPagerDutyClient(&PagerDutyConfig{
		APIKey:  "test-api-key",
		Email:   "test@example.com",
		BaseURL: server.URL,
	})

	err := client.AcknowledgeIncident(context.Background(), "PINCIDENT1")
	if err != nil {
		t.Errorf("AcknowledgeIncident() error = %v", err)
	}
}

func TestPagerDutyClient_AddNote(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/incidents/PINCIDENT1/notes" {
			t.Errorf("Path = %s, want /incidents/PINCIDENT1/notes", r.URL.Path)
		}

		var body map[string]interface{}
		json.NewDecoder(r.Body).Decode(&body)
		note := body["note"].(map[string]interface{})
		if note["content"] != "Test note" {
			t.Errorf("Content = %v, want 'Test note'", note["content"])
		}

		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewPagerDutyClient(&PagerDutyConfig{
		APIKey:  "test-api-key",
		Email:   "test@example.com",
		BaseURL: server.URL,
	})

	err := client.AddNote(context.Background(), "PINCIDENT1", "Test note")
	if err != nil {
		t.Errorf("AddNote() error = %v", err)
	}
}

func TestOpsgenieClient_ParseWebhook(t *testing.T) {
	client := NewOpsgenieClient(&OpsgenieConfig{APIKey: "test"})

	payload := `{
		"action": "Create",
		"alert": {
			"alertId": "OALERT1",
			"message": "Database connection failed",
			"description": "Unable to connect to primary database",
			"priority": "P1",
			"source": "monitoring",
			"createdAt": "2024-01-15T10:00:00Z"
		}
	}`

	incident, err := client.ParseWebhook([]byte(payload))
	if err != nil {
		t.Fatalf("ParseWebhook() error = %v", err)
	}

	if incident.ExternalID != "OALERT1" {
		t.Errorf("ExternalID = %q, want %q", incident.ExternalID, "OALERT1")
	}
	if incident.Title != "Database connection failed" {
		t.Errorf("Title = %q, want %q", incident.Title, "Database connection failed")
	}
	if incident.Status != IncidentStatusTriggered {
		t.Errorf("Status = %v, want %v", incident.Status, IncidentStatusTriggered)
	}
	if incident.Severity != IncidentSeverityCritical {
		t.Errorf("Severity = %v, want %v", incident.Severity, IncidentSeverityCritical)
	}
}

func TestOpsgenieClient_AcknowledgeIncident(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/alerts/OALERT1/acknowledge" {
			t.Errorf("Path = %s, want /alerts/OALERT1/acknowledge", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "GenieKey test-api-key" {
			t.Errorf("Missing or wrong Authorization header")
		}

		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewOpsgenieClient(&OpsgenieConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	})

	err := client.AcknowledgeIncident(context.Background(), "OALERT1")
	if err != nil {
		t.Errorf("AcknowledgeIncident() error = %v", err)
	}
}

func TestOpsgenieClient_ResolveIncident(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/alerts/OALERT1/close" {
			t.Errorf("Path = %s, want /alerts/OALERT1/close", r.URL.Path)
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()

	client := NewOpsgenieClient(&OpsgenieConfig{
		APIKey:  "test-api-key",
		BaseURL: server.URL,
	})

	err := client.ResolveIncident(context.Background(), "OALERT1")
	if err != nil {
		t.Errorf("ResolveIncident() error = %v", err)
	}
}

func TestITSMTriggerManager_RegisterAndGet(t *testing.T) {
	manager := NewITSMTriggerManager()

	trigger := &ITSMTrigger{
		ID:         "pd-trigger",
		Name:       "PagerDuty Trigger",
		Type:       ITSMTypePagerDuty,
		RunbookRef: RunbookRef{Name: "remediate"},
		Enabled:    true,
	}

	// Register
	if err := manager.Register(trigger); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Get
	got, ok := manager.Get("pd-trigger")
	if !ok {
		t.Fatal("Get() returned false")
	}
	if got.ID != trigger.ID {
		t.Errorf("Get() ID = %v, want %v", got.ID, trigger.ID)
	}

	// List
	triggers := manager.List()
	if len(triggers) != 1 {
		t.Errorf("List() returned %d triggers, want 1", len(triggers))
	}

	// Duplicate registration
	if err := manager.Register(trigger); err == nil {
		t.Error("Expected error for duplicate registration")
	}

	// Unregister
	if err := manager.Unregister("pd-trigger"); err != nil {
		t.Fatalf("Unregister() error = %v", err)
	}

	_, ok = manager.Get("pd-trigger")
	if ok {
		t.Error("Get() should return false after unregister")
	}
}

func TestITSMTriggerManager_EnableDisable(t *testing.T) {
	manager := NewITSMTriggerManager()

	trigger := &ITSMTrigger{
		ID:         "pd-trigger",
		Name:       "PagerDuty Trigger",
		Type:       ITSMTypePagerDuty,
		RunbookRef: RunbookRef{Name: "remediate"},
		Enabled:    true,
	}
	manager.Register(trigger)

	// Disable
	if err := manager.Disable("pd-trigger"); err != nil {
		t.Fatalf("Disable() error = %v", err)
	}

	got, _ := manager.Get("pd-trigger")
	if got.Enabled {
		t.Error("Trigger should be disabled")
	}

	// Enable
	if err := manager.Enable("pd-trigger"); err != nil {
		t.Fatalf("Enable() error = %v", err)
	}

	got, _ = manager.Get("pd-trigger")
	if !got.Enabled {
		t.Error("Trigger should be enabled")
	}
}

func TestITSMTriggerManager_HandleWebhook(t *testing.T) {
	// Create mock PagerDuty server
	pdServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer pdServer.Close()

	repo := newMockRepository()
	executor := newMockExecutor()
	publisher := newMockPublisher()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "remediate-incident"},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{{Name: "fix", Type: runbook.StepTypeNoop}},
		},
	})

	pdClient := NewPagerDutyClient(&PagerDutyConfig{
		APIKey:  "test-key",
		Email:   "test@example.com",
		BaseURL: pdServer.URL,
	})

	manager := NewITSMTriggerManager(
		WithITSMRepository(repo),
		WithITSMExecutor(executor),
		WithITSMPublisher(publisher),
		WithITSMClient(pdClient),
	)

	trigger := &ITSMTrigger{
		ID:                 "pd-auto-remediate",
		Name:               "Auto Remediate",
		Type:               ITSMTypePagerDuty,
		RunbookRef:         RunbookRef{Name: "remediate-incident"},
		ServiceFilter:      []string{"Web Service"},
		AcknowledgeOnStart: true,
		UpdateIncident:     true,
		Enabled:            true,
	}
	manager.Register(trigger)

	// Send webhook
	payload := `{
		"messages": [{
			"event": "incident.trigger",
			"incident": {
				"id": "PINCIDENT1",
				"title": "Server Down",
				"status": "triggered",
				"urgency": "high",
				"created_at": "2024-01-15T10:00:00Z",
				"service": {
					"id": "PSERVICE1",
					"name": "Web Service"
				}
			}
		}]
	}`

	err := manager.HandleWebhook(context.Background(), ITSMTypePagerDuty, []byte(payload))
	if err != nil {
		t.Fatalf("HandleWebhook() error = %v", err)
	}

	// Verify runbook was executed
	if executor.ExecutionCount() != 1 {
		t.Errorf("ExecutionCount = %d, want 1", executor.ExecutionCount())
	}

	// Verify inputs
	lastExec := executor.LastExecution()
	if lastExec.inputs["incident_title"] != "Server Down" {
		t.Errorf("incident_title = %v, want 'Server Down'", lastExec.inputs["incident_title"])
	}
	if lastExec.inputs["incident_service"] != "Web Service" {
		t.Errorf("incident_service = %v, want 'Web Service'", lastExec.inputs["incident_service"])
	}

	// Verify incident linking
	execID, ok := manager.GetLinkedExecution("PINCIDENT1")
	if !ok {
		t.Error("Incident should be linked to execution")
	}
	if execID == "" {
		t.Error("Execution ID should not be empty")
	}
}

func TestITSMTriggerManager_ServiceFilter(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "remediate"},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{{Name: "fix", Type: runbook.StepTypeNoop}},
		},
	})

	pdClient := NewPagerDutyClient(&PagerDutyConfig{APIKey: "test"})

	manager := NewITSMTriggerManager(
		WithITSMRepository(repo),
		WithITSMExecutor(executor),
		WithITSMClient(pdClient),
	)

	trigger := &ITSMTrigger{
		ID:            "pd-db-only",
		Name:          "DB Only",
		Type:          ITSMTypePagerDuty,
		RunbookRef:    RunbookRef{Name: "remediate"},
		ServiceFilter: []string{"Database Service"},
		Enabled:       true,
	}
	manager.Register(trigger)

	// Send webhook for different service
	payload := `{
		"messages": [{
			"event": "incident.trigger",
			"incident": {
				"id": "PINCIDENT1",
				"title": "Web Down",
				"status": "triggered",
				"urgency": "high",
				"created_at": "2024-01-15T10:00:00Z",
				"service": {
					"id": "PSERVICE1",
					"name": "Web Service"
				}
			}
		}]
	}`

	manager.HandleWebhook(context.Background(), ITSMTypePagerDuty, []byte(payload))

	// Should not execute because service doesn't match
	if executor.ExecutionCount() != 0 {
		t.Errorf("ExecutionCount = %d, want 0 (filtered)", executor.ExecutionCount())
	}

	// Send webhook for matching service
	payload = `{
		"messages": [{
			"event": "incident.trigger",
			"incident": {
				"id": "PINCIDENT2",
				"title": "DB Down",
				"status": "triggered",
				"urgency": "high",
				"created_at": "2024-01-15T10:00:00Z",
				"service": {
					"id": "PSERVICE2",
					"name": "Database Service"
				}
			}
		}]
	}`

	manager.HandleWebhook(context.Background(), ITSMTypePagerDuty, []byte(payload))

	if executor.ExecutionCount() != 1 {
		t.Errorf("ExecutionCount = %d, want 1", executor.ExecutionCount())
	}
}

func TestITSMTriggerManager_SeverityFilter(t *testing.T) {
	repo := newMockRepository()
	executor := newMockExecutor()

	repo.Add(&runbook.Runbook{
		APIVersion: "runbook.keystone.io/v1",
		Kind:       "Runbook",
		Metadata:   runbook.Metadata{Name: "critical-handler"},
		Spec: runbook.RunbookSpec{
			Steps: []runbook.Step{{Name: "fix", Type: runbook.StepTypeNoop}},
		},
	})

	pdClient := NewPagerDutyClient(&PagerDutyConfig{APIKey: "test"})

	manager := NewITSMTriggerManager(
		WithITSMRepository(repo),
		WithITSMExecutor(executor),
		WithITSMClient(pdClient),
	)

	trigger := &ITSMTrigger{
		ID:             "pd-high-only",
		Name:           "High Severity Only",
		Type:           ITSMTypePagerDuty,
		RunbookRef:     RunbookRef{Name: "critical-handler"},
		SeverityFilter: []string{"high", "critical"},
		Enabled:        true,
	}
	manager.Register(trigger)

	// Send low urgency incident
	payload := `{
		"messages": [{
			"event": "incident.trigger",
			"incident": {
				"id": "PINCIDENT1",
				"title": "Low Priority",
				"status": "triggered",
				"urgency": "low",
				"created_at": "2024-01-15T10:00:00Z",
				"service": {"id": "SVC1", "name": "Test"}
			}
		}]
	}`

	manager.HandleWebhook(context.Background(), ITSMTypePagerDuty, []byte(payload))

	if executor.ExecutionCount() != 0 {
		t.Errorf("ExecutionCount = %d, want 0 (filtered by severity)", executor.ExecutionCount())
	}

	// Send high urgency incident
	payload = `{
		"messages": [{
			"event": "incident.trigger",
			"incident": {
				"id": "PINCIDENT2",
				"title": "High Priority",
				"status": "triggered",
				"urgency": "high",
				"created_at": "2024-01-15T10:00:00Z",
				"service": {"id": "SVC1", "name": "Test"}
			}
		}]
	}`

	manager.HandleWebhook(context.Background(), ITSMTypePagerDuty, []byte(payload))

	if executor.ExecutionCount() != 1 {
		t.Errorf("ExecutionCount = %d, want 1", executor.ExecutionCount())
	}
}

func TestIncidentStatusMapping(t *testing.T) {
	tests := []struct {
		pdStatus string
		want     IncidentStatus
	}{
		{"triggered", IncidentStatusTriggered},
		{"acknowledged", IncidentStatusAcknowledged},
		{"resolved", IncidentStatusResolved},
		{"unknown", IncidentStatusTriggered},
	}

	for _, tt := range tests {
		t.Run(tt.pdStatus, func(t *testing.T) {
			got := mapPagerDutyStatus(tt.pdStatus)
			if got != tt.want {
				t.Errorf("mapPagerDutyStatus(%q) = %v, want %v", tt.pdStatus, got, tt.want)
			}
		})
	}
}

func TestOpsgeniePriorityMapping(t *testing.T) {
	tests := []struct {
		priority string
		want     IncidentSeverity
	}{
		{"P1", IncidentSeverityCritical},
		{"P2", IncidentSeverityHigh},
		{"P3", IncidentSeverityMedium},
		{"P4", IncidentSeverityLow},
		{"P5", IncidentSeverityInfo},
		{"unknown", IncidentSeverityMedium},
	}

	for _, tt := range tests {
		t.Run(tt.priority, func(t *testing.T) {
			got := mapOpsgeniePriority(tt.priority)
			if got != tt.want {
				t.Errorf("mapOpsgeniePriority(%q) = %v, want %v", tt.priority, got, tt.want)
			}
		})
	}
}

func TestOpsgenieActionMapping(t *testing.T) {
	tests := []struct {
		action string
		want   IncidentStatus
	}{
		{"Create", IncidentStatusTriggered},
		{"Escalate", IncidentStatusTriggered},
		{"Acknowledge", IncidentStatusAcknowledged},
		{"Close", IncidentStatusResolved},
		{"Resolve", IncidentStatusResolved},
		{"unknown", IncidentStatusTriggered},
	}

	for _, tt := range tests {
		t.Run(tt.action, func(t *testing.T) {
			got := mapOpsgenieStatus(tt.action)
			if got != tt.want {
				t.Errorf("mapOpsgenieStatus(%q) = %v, want %v", tt.action, got, tt.want)
			}
		})
	}
}

func TestITSMTriggerManager_NoClient(t *testing.T) {
	manager := NewITSMTriggerManager()

	err := manager.HandleWebhook(context.Background(), ITSMTypePagerDuty, []byte(`{}`))
	if err == nil {
		t.Error("Expected error when no client configured")
	}
}
