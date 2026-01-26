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
			"keystonecore.io/verified":  "true",
			"keystonecore.io/timestamp": "2025-01-01T00:00:00Z",
		},
	}

	if len(req.Annotations) != 2 {
		t.Errorf("Annotations length = %d, want 2", len(req.Annotations))
	}

	if req.Annotations["keystonecore.io/verified"] != "true" {
		t.Errorf("verified annotation = %v, want true", req.Annotations["keystonecore.io/verified"])
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

func TestRevisionHistoryEntry(t *testing.T) {
	entry := &RevisionHistoryEntry{
		Revision:        "abc123",
		DeployedAt:      time.Now(),
		ID:              1,
		DeployStartedAt: time.Now().Add(-5 * time.Minute),
		Source: &ApplicationSource{
			RepoURL:        "https://github.com/example/repo",
			Path:           "manifests",
			TargetRevision: "main",
			Chart:          "",
		},
	}

	if entry.Revision != "abc123" {
		t.Errorf("Revision = %v, want abc123", entry.Revision)
	}

	if entry.ID != 1 {
		t.Errorf("ID = %v, want 1", entry.ID)
	}

	if entry.Source.RepoURL != "https://github.com/example/repo" {
		t.Errorf("Source.RepoURL = %v, want https://github.com/example/repo", entry.Source.RepoURL)
	}
}

func TestRevisionHistory_GetPrevious(t *testing.T) {
	tests := []struct {
		name    string
		history RevisionHistory
		wantNil bool
		wantRev string
	}{
		{
			name:    "empty history",
			history: RevisionHistory{},
			wantNil: true,
		},
		{
			name: "single entry",
			history: RevisionHistory{
				{Revision: "abc123", ID: 1},
			},
			wantNil: true,
		},
		{
			name: "two entries",
			history: RevisionHistory{
				{Revision: "def456", ID: 2},
				{Revision: "abc123", ID: 1},
			},
			wantNil: false,
			wantRev: "abc123",
		},
		{
			name: "three entries",
			history: RevisionHistory{
				{Revision: "ghi789", ID: 3},
				{Revision: "def456", ID: 2},
				{Revision: "abc123", ID: 1},
			},
			wantNil: false,
			wantRev: "def456",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prev := tt.history.GetPrevious()
			if tt.wantNil {
				if prev != nil {
					t.Errorf("GetPrevious() = %v, want nil", prev)
				}
			} else {
				if prev == nil {
					t.Error("GetPrevious() = nil, want non-nil")
				} else if prev.Revision != tt.wantRev {
					t.Errorf("GetPrevious().Revision = %v, want %v", prev.Revision, tt.wantRev)
				}
			}
		})
	}
}

func TestRevisionHistory_GetByID(t *testing.T) {
	history := RevisionHistory{
		{Revision: "ghi789", ID: 3},
		{Revision: "def456", ID: 2},
		{Revision: "abc123", ID: 1},
	}

	tests := []struct {
		id      int64
		wantNil bool
		wantRev string
	}{
		{id: 1, wantNil: false, wantRev: "abc123"},
		{id: 2, wantNil: false, wantRev: "def456"},
		{id: 3, wantNil: false, wantRev: "ghi789"},
		{id: 4, wantNil: true},
		{id: 0, wantNil: true},
	}

	for _, tt := range tests {
		entry := history.GetByID(tt.id)
		if tt.wantNil {
			if entry != nil {
				t.Errorf("GetByID(%d) = %v, want nil", tt.id, entry)
			}
		} else {
			if entry == nil {
				t.Errorf("GetByID(%d) = nil, want non-nil", tt.id)
			} else if entry.Revision != tt.wantRev {
				t.Errorf("GetByID(%d).Revision = %v, want %v", tt.id, entry.Revision, tt.wantRev)
			}
		}
	}
}

func TestRevisionHistory_GetLastHealthy(t *testing.T) {
	history := RevisionHistory{
		{Revision: "abc123", ID: 1},
	}

	// GetLastHealthy returns nil because ArgoCD doesn't track health in history
	if history.GetLastHealthy() != nil {
		t.Error("GetLastHealthy() should return nil (health not tracked in history)")
	}
}

func TestApplicationSource(t *testing.T) {
	source := &ApplicationSource{
		RepoURL:        "https://github.com/example/repo",
		Path:           "charts/myapp",
		TargetRevision: "v1.2.3",
		Chart:          "myapp",
	}

	if source.RepoURL != "https://github.com/example/repo" {
		t.Errorf("RepoURL = %v, want https://github.com/example/repo", source.RepoURL)
	}

	if source.Path != "charts/myapp" {
		t.Errorf("Path = %v, want charts/myapp", source.Path)
	}

	if source.Chart != "myapp" {
		t.Errorf("Chart = %v, want myapp", source.Chart)
	}
}
