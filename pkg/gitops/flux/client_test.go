package flux

import (
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.Namespace != "flux-system" {
		t.Errorf("Namespace = %v, want flux-system", config.Namespace)
	}
}

func TestResourceKinds(t *testing.T) {
	kinds := []ResourceKind{
		KindKustomization,
		KindHelmRelease,
		KindGitRepository,
		KindHelmRepository,
	}

	expected := []string{
		"Kustomization",
		"HelmRelease",
		"GitRepository",
		"HelmRepository",
	}

	for i, kind := range kinds {
		if string(kind) != expected[i] {
			t.Errorf("Kind[%d] = %v, want %v", i, kind, expected[i])
		}
	}
}

func TestSuspendRequest(t *testing.T) {
	req := &SuspendRequest{
		Kind:      KindKustomization,
		Name:      "infrastructure",
		Namespace: "flux-system",
		Suspend:   true,
	}

	if req.Kind != KindKustomization {
		t.Errorf("Kind = %v, want %v", req.Kind, KindKustomization)
	}

	if !req.Suspend {
		t.Error("Suspend = false, want true")
	}
}

func TestReconcileRequest(t *testing.T) {
	req := &ReconcileRequest{
		Kind:      KindHelmRelease,
		Name:      "myapp",
		Namespace: "production",
	}

	if req.Kind != KindHelmRelease {
		t.Errorf("Kind = %v, want %v", req.Kind, KindHelmRelease)
	}

	if req.Name != "myapp" {
		t.Errorf("Name = %v, want myapp", req.Name)
	}
}

func TestResourceStatus(t *testing.T) {
	status := &ResourceStatus{
		Kind:      KindKustomization,
		Name:      "infrastructure",
		Namespace: "flux-system",
		Ready:     true,
		Suspended: false,
		Revision:  "main@sha1:abc123",
		Message:   "Applied revision: main@sha1:abc123",
		Conditions: []Condition{
			{
				Type:    "Ready",
				Status:  "True",
				Reason:  "ReconciliationSucceeded",
				Message: "Applied revision: main@sha1:abc123",
			},
		},
	}

	if !status.Ready {
		t.Error("Ready = false, want true")
	}

	if status.Suspended {
		t.Error("Suspended = true, want false")
	}

	if len(status.Conditions) != 1 {
		t.Errorf("Conditions length = %d, want 1", len(status.Conditions))
	}

	if status.Conditions[0].Type != "Ready" {
		t.Errorf("Condition type = %v, want Ready", status.Conditions[0].Type)
	}
}

func TestGetStringField(t *testing.T) {
	m := map[string]interface{}{
		"type":    "Ready",
		"status":  "True",
		"number":  123,
		"missing": nil,
	}

	tests := []struct {
		name     string
		key      string
		expected string
	}{
		{"existing string", "type", "Ready"},
		{"another string", "status", "True"},
		{"non-string", "number", ""},
		{"missing key", "nothere", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getStringField(m, tt.key)
			if result != tt.expected {
				t.Errorf("getStringField(%s) = %v, want %v", tt.key, result, tt.expected)
			}
		})
	}
}
