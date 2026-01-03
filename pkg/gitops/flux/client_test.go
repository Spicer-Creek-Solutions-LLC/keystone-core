package flux

import (
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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

func TestCondition(t *testing.T) {
	conditions := []Condition{
		{
			Type:    "Ready",
			Status:  "True",
			Reason:  "ReconciliationSucceeded",
			Message: "Applied revision: main@sha1:abc123",
		},
		{
			Type:    "Stalled",
			Status:  "False",
			Reason:  "",
			Message: "",
		},
		{
			Type:    "Healthy",
			Status:  "Unknown",
			Reason:  "Progressing",
			Message: "Reconciliation in progress",
		},
	}

	// Verify Ready condition
	if conditions[0].Type != "Ready" {
		t.Errorf("conditions[0].Type = %v, want Ready", conditions[0].Type)
	}
	if conditions[0].Status != "True" {
		t.Errorf("conditions[0].Status = %v, want True", conditions[0].Status)
	}

	// Verify Stalled condition
	if conditions[1].Type != "Stalled" {
		t.Errorf("conditions[1].Type = %v, want Stalled", conditions[1].Type)
	}
	if conditions[1].Status != "False" {
		t.Errorf("conditions[1].Status = %v, want False", conditions[1].Status)
	}

	// Verify Healthy condition
	if conditions[2].Reason != "Progressing" {
		t.Errorf("conditions[2].Reason = %v, want Progressing", conditions[2].Reason)
	}
}

func TestResourceStatus_WithLastReconcileTime(t *testing.T) {
	reconcileTime := time.Now().Add(-5 * time.Minute)
	status := &ResourceStatus{
		Kind:              KindGitRepository,
		Name:              "my-repo",
		Namespace:         "flux-system",
		Ready:             true,
		Revision:          "main@sha1:def456",
		LastReconcileTime: reconcileTime,
	}

	if status.LastReconcileTime.IsZero() {
		t.Error("LastReconcileTime should not be zero")
	}

	if status.Kind != KindGitRepository {
		t.Errorf("Kind = %v, want %v", status.Kind, KindGitRepository)
	}
}

func TestResourceStatus_Suspended(t *testing.T) {
	status := &ResourceStatus{
		Kind:      KindHelmRelease,
		Name:      "suspended-app",
		Namespace: "production",
		Suspended: true,
		Ready:     false,
		Message:   "Reconciliation is suspended",
	}

	if !status.Suspended {
		t.Error("Suspended = false, want true")
	}

	if status.Ready {
		t.Error("Ready = true, want false (suspended)")
	}
}

func TestGvrForKind(t *testing.T) {
	// Create a dummy client to test gvrForKind
	client := &Client{
		config: DefaultConfig(),
	}

	tests := []struct {
		kind          ResourceKind
		expectedGroup string
		expectedRes   string
	}{
		{KindKustomization, "kustomize.toolkit.fluxcd.io", "kustomizations"},
		{KindHelmRelease, "helm.toolkit.fluxcd.io", "helmreleases"},
		{KindGitRepository, "source.toolkit.fluxcd.io", "gitrepositories"},
		{KindHelmRepository, "source.toolkit.fluxcd.io", "helmrepositories"},
		{ResourceKind("Unknown"), "", ""}, // Unknown kind
	}

	for _, tt := range tests {
		t.Run(string(tt.kind), func(t *testing.T) {
			gvr := client.gvrForKind(tt.kind)
			if gvr.Group != tt.expectedGroup {
				t.Errorf("gvrForKind(%v).Group = %v, want %v", tt.kind, gvr.Group, tt.expectedGroup)
			}
			if gvr.Resource != tt.expectedRes {
				t.Errorf("gvrForKind(%v).Resource = %v, want %v", tt.kind, gvr.Resource, tt.expectedRes)
			}
		})
	}
}

func TestParseResourceStatus(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	// Create a mock unstructured object
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
			"kind":       "Kustomization",
			"metadata": map[string]interface{}{
				"name":      "test-kustomization",
				"namespace": "flux-system",
			},
			"spec": map[string]interface{}{
				"suspend": false,
			},
			"status": map[string]interface{}{
				"lastAppliedRevision": "main@sha1:abc123",
				"lastReconcileTime":   "2024-01-15T10:30:00Z",
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "True",
						"reason":  "ReconciliationSucceeded",
						"message": "Applied revision: main@sha1:abc123",
					},
				},
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	if status.Name != "test-kustomization" {
		t.Errorf("Name = %v, want test-kustomization", status.Name)
	}

	if status.Namespace != "flux-system" {
		t.Errorf("Namespace = %v, want flux-system", status.Namespace)
	}

	if !status.Ready {
		t.Error("Ready = false, want true")
	}

	if status.Revision != "main@sha1:abc123" {
		t.Errorf("Revision = %v, want main@sha1:abc123", status.Revision)
	}

	if len(status.Conditions) != 1 {
		t.Errorf("Conditions length = %d, want 1", len(status.Conditions))
	}

	if status.Conditions[0].Type != "Ready" {
		t.Errorf("Condition type = %v, want Ready", status.Conditions[0].Type)
	}
}

func TestParseResourceStatus_Suspended(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "suspended-ks",
				"namespace": "flux-system",
			},
			"spec": map[string]interface{}{
				"suspend": true,
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	if !status.Suspended {
		t.Error("Suspended = false, want true")
	}
}

func TestParseResourceStatus_NoStatus(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "new-ks",
				"namespace": "flux-system",
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	if status.Name != "new-ks" {
		t.Errorf("Name = %v, want new-ks", status.Name)
	}

	if status.Ready {
		t.Error("Ready should be false for new resource without status")
	}
}

func TestParseResourceStatus_NotReady(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "failing-ks",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "False",
						"reason":  "ReconciliationFailed",
						"message": "Validation failed",
					},
				},
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	if status.Ready {
		t.Error("Ready should be false")
	}

	if status.Conditions[0].Reason != "ReconciliationFailed" {
		t.Errorf("Reason = %v, want ReconciliationFailed", status.Conditions[0].Reason)
	}
}

func TestConfig(t *testing.T) {
	config := &Config{
		Namespace:  "custom-namespace",
		Kubeconfig: "/path/to/kubeconfig",
		Context:    "custom-context",
	}

	if config.Namespace != "custom-namespace" {
		t.Errorf("Namespace = %v, want custom-namespace", config.Namespace)
	}

	if config.Kubeconfig != "/path/to/kubeconfig" {
		t.Errorf("Kubeconfig = %v, want /path/to/kubeconfig", config.Kubeconfig)
	}

	if config.Context != "custom-context" {
		t.Errorf("Context = %v, want custom-context", config.Context)
	}
}

func TestSuspendRequest_AllFields(t *testing.T) {
	tests := []struct {
		name      string
		kind      ResourceKind
		resName   string
		namespace string
		suspend   bool
	}{
		{"suspend kustomization", KindKustomization, "infra", "flux-system", true},
		{"resume kustomization", KindKustomization, "infra", "flux-system", false},
		{"suspend helm release", KindHelmRelease, "myapp", "production", true},
		{"suspend git repo", KindGitRepository, "repo", "flux-system", true},
		{"suspend helm repo", KindHelmRepository, "charts", "flux-system", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &SuspendRequest{
				Kind:      tt.kind,
				Name:      tt.resName,
				Namespace: tt.namespace,
				Suspend:   tt.suspend,
			}

			if req.Kind != tt.kind {
				t.Errorf("Kind = %v, want %v", req.Kind, tt.kind)
			}
			if req.Name != tt.resName {
				t.Errorf("Name = %v, want %v", req.Name, tt.resName)
			}
			if req.Namespace != tt.namespace {
				t.Errorf("Namespace = %v, want %v", req.Namespace, tt.namespace)
			}
			if req.Suspend != tt.suspend {
				t.Errorf("Suspend = %v, want %v", req.Suspend, tt.suspend)
			}
		})
	}
}

func TestReconcileRequest_AllKinds(t *testing.T) {
	kinds := []ResourceKind{
		KindKustomization,
		KindHelmRelease,
		KindGitRepository,
		KindHelmRepository,
	}

	for _, kind := range kinds {
		t.Run(string(kind), func(t *testing.T) {
			req := &ReconcileRequest{
				Kind:      kind,
				Name:      "test-resource",
				Namespace: "flux-system",
			}

			if req.Kind != kind {
				t.Errorf("Kind = %v, want %v", req.Kind, kind)
			}
		})
	}
}

func TestParseResourceStatus_InvalidConditionType(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	// Create object with invalid condition type (string instead of map)
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-ks",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					"invalid-string-condition", // Should be a map
				},
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	// Should have no conditions since the invalid one is skipped
	if len(status.Conditions) != 0 {
		t.Errorf("Conditions length = %d, want 0 (invalid condition should be skipped)", len(status.Conditions))
	}
}

func TestParseResourceStatus_InvalidTimestamp(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-ks",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"lastReconcileTime": "not-a-valid-timestamp",
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	// LastReconcileTime should be zero when parsing fails
	if !status.LastReconcileTime.IsZero() {
		t.Error("LastReconcileTime should be zero for invalid timestamp")
	}
}

func TestParseResourceStatus_MixedConditions(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-ks",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":    "Ready",
						"status":  "True",
						"reason":  "ReconciliationSucceeded",
						"message": "First message",
					},
					"invalid-condition", // Should be skipped
					map[string]interface{}{
						"type":    "Healthy",
						"status":  "True",
						"reason":  "AllChecksPass",
						"message": "Second message",
					},
				},
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	// Should have 2 valid conditions (invalid one skipped)
	if len(status.Conditions) != 2 {
		t.Errorf("Conditions length = %d, want 2", len(status.Conditions))
	}

	// Should be ready (first condition has Ready=True)
	if !status.Ready {
		t.Error("Ready = false, want true")
	}

	// Message should be from the last condition with a message
	if status.Message != "Second message" {
		t.Errorf("Message = %v, want 'Second message'", status.Message)
	}
}

func TestParseResourceStatus_EmptyConditionFields(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-ks",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "False",
						// reason and message are missing
					},
				},
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	if len(status.Conditions) != 1 {
		t.Fatalf("Conditions length = %d, want 1", len(status.Conditions))
	}

	// Empty fields should be empty strings
	if status.Conditions[0].Reason != "" {
		t.Errorf("Reason = %v, want empty", status.Conditions[0].Reason)
	}
	if status.Conditions[0].Message != "" {
		t.Errorf("Message = %v, want empty", status.Conditions[0].Message)
	}
}

func TestClient_StructFields(t *testing.T) {
	config := &Config{
		Namespace:  "test-namespace",
		Kubeconfig: "/path/to/kubeconfig",
		Context:    "test-context",
	}

	// Create client directly without connecting
	client := &Client{
		config: config,
	}

	// Verify config is stored correctly
	if client.config.Namespace != "test-namespace" {
		t.Errorf("config.Namespace = %v, want test-namespace", client.config.Namespace)
	}
	if client.config.Kubeconfig != "/path/to/kubeconfig" {
		t.Errorf("config.Kubeconfig = %v, want /path/to/kubeconfig", client.config.Kubeconfig)
	}
	if client.config.Context != "test-context" {
		t.Errorf("config.Context = %v, want test-context", client.config.Context)
	}
}

func TestResourceStatus_ZeroValues(t *testing.T) {
	status := &ResourceStatus{}

	if status.Kind != "" {
		t.Errorf("Kind = %v, want empty", status.Kind)
	}
	if status.Name != "" {
		t.Errorf("Name = %v, want empty", status.Name)
	}
	if status.Ready {
		t.Error("Ready = true, want false")
	}
	if status.Suspended {
		t.Error("Suspended = true, want false")
	}
	if len(status.Conditions) != 0 {
		t.Errorf("Conditions length = %d, want 0", len(status.Conditions))
	}
}

func TestCondition_ZeroValues(t *testing.T) {
	cond := Condition{}

	if cond.Type != "" {
		t.Errorf("Type = %v, want empty", cond.Type)
	}
	if cond.Status != "" {
		t.Errorf("Status = %v, want empty", cond.Status)
	}
	if cond.Reason != "" {
		t.Errorf("Reason = %v, want empty", cond.Reason)
	}
	if cond.Message != "" {
		t.Errorf("Message = %v, want empty", cond.Message)
	}
}

func TestParseResourceStatus_MultipleReadyConditions(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	// First Ready is False, second Ready is True - should be ready at the end
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-ks",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"conditions": []interface{}{
					map[string]interface{}{
						"type":   "Ready",
						"status": "False",
						"reason": "InProgress",
					},
					map[string]interface{}{
						"type":   "Ready",
						"status": "True",
						"reason": "Succeeded",
					},
				},
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	// Should be ready since second Ready condition is True
	if !status.Ready {
		t.Error("Ready = false, want true")
	}
}

func TestGetStringField_NilValue(t *testing.T) {
	m := map[string]interface{}{
		"nil_value": nil,
	}

	result := getStringField(m, "nil_value")
	if result != "" {
		t.Errorf("getStringField(nil) = %v, want empty", result)
	}
}

func TestParseResourceStatus_WithAllRevisionFields(t *testing.T) {
	client := &Client{
		config: DefaultConfig(),
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"metadata": map[string]interface{}{
				"name":      "test-ks",
				"namespace": "flux-system",
			},
			"status": map[string]interface{}{
				"lastAppliedRevision": "main@sha1:abc123def456",
				"lastReconcileTime":   "2024-01-15T10:30:00Z",
			},
		},
	}

	status, err := client.parseResourceStatus(obj, KindKustomization)
	if err != nil {
		t.Fatalf("parseResourceStatus() error: %v", err)
	}

	if status.Revision != "main@sha1:abc123def456" {
		t.Errorf("Revision = %v, want main@sha1:abc123def456", status.Revision)
	}

	expectedTime, _ := time.Parse(time.RFC3339, "2024-01-15T10:30:00Z")
	if !status.LastReconcileTime.Equal(expectedTime) {
		t.Errorf("LastReconcileTime = %v, want %v", status.LastReconcileTime, expectedTime)
	}
}
