package argocd

import (
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.ServerAddr != "localhost:8080" {
		t.Errorf("ServerAddr = %v, want localhost:8080", config.ServerAddr)
	}

	if config.Insecure != false {
		t.Errorf("Insecure = %v, want false", config.Insecure)
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("Timeout = %v, want 30s", config.Timeout)
	}
}

func TestSyncRequest(t *testing.T) {
	req := &SyncRequest{
		Application: "test-app",
		Namespace:   "argocd",
		Revision:    "main",
		Prune:       true,
		DryRun:      false,
		Resources:   []string{"deployment/web"},
	}

	if req.Application != "test-app" {
		t.Errorf("Application = %v, want test-app", req.Application)
	}

	if len(req.Resources) != 1 {
		t.Errorf("Resources length = %d, want 1", len(req.Resources))
	}
}

func TestRollbackRequest(t *testing.T) {
	req := &RollbackRequest{
		Application: "test-app",
		Namespace:   "argocd",
		Revision:    "abc123",
		Prune:       false,
	}

	if req.Revision != "abc123" {
		t.Errorf("Revision = %v, want abc123", req.Revision)
	}
}

func TestAnnotationUpdate(t *testing.T) {
	req := &AnnotationUpdate{
		Application: "test-app",
		Annotations: map[string]string{
			"titan.io/verified": "true",
			"titan.io/timestamp": "2025-01-01T00:00:00Z",
		},
	}

	if len(req.Annotations) != 2 {
		t.Errorf("Annotations length = %d, want 2", len(req.Annotations))
	}

	if req.Annotations["titan.io/verified"] != "true" {
		t.Errorf("verified annotation = %v, want true", req.Annotations["titan.io/verified"])
	}
}

func TestApplicationStatus(t *testing.T) {
	status := &ApplicationStatus{
		Name:           "test-app",
		Namespace:      "argocd",
		SyncStatus:     "Synced",
		HealthStatus:   "Healthy",
		Revision:       "abc123",
		RepoURL:        "https://github.com/example/repo",
		TargetRevision: "main",
		OperationPhase: "Succeeded",
		Message:        "Sync successful",
		ObservedAt:     time.Now(),
	}

	if status.Name != "test-app" {
		t.Errorf("Name = %v, want test-app", status.Name)
	}

	if status.SyncStatus != "Synced" {
		t.Errorf("SyncStatus = %v, want Synced", status.SyncStatus)
	}

	if status.HealthStatus != "Healthy" {
		t.Errorf("HealthStatus = %v, want Healthy", status.HealthStatus)
	}
}
