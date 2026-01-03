package argocd

import "time"

// Config represents ArgoCD client configuration
type Config struct {
	// ServerAddr is the ArgoCD server address
	ServerAddr string

	// Token is the authentication token
	Token string

	// Insecure skips TLS verification (for testing)
	Insecure bool

	// Timeout for API requests
	Timeout time.Duration
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		ServerAddr: "localhost:8080",
		Insecure:   false,
		Timeout:    30 * time.Second,
	}
}

// ApplicationStatus represents the status of an ArgoCD application
type ApplicationStatus struct {
	// Name is the application name
	Name string

	// Namespace is the application namespace
	Namespace string

	// SyncStatus is the sync status (Synced, OutOfSync, Unknown)
	SyncStatus string

	// HealthStatus is the health status (Healthy, Progressing, Degraded, Suspended, Missing, Unknown)
	HealthStatus string

	// Revision is the current git revision
	Revision string

	// RepoURL is the git repository URL
	RepoURL string

	// TargetRevision is the target git revision
	TargetRevision string

	// OperationPhase is the current operation phase (Running, Succeeded, Failed, Error, Terminating)
	OperationPhase string

	// Message contains status message
	Message string

	// ObservedAt is when the status was observed
	ObservedAt time.Time
}

// SyncRequest represents a sync request
type SyncRequest struct {
	// Application name
	Application string

	// Namespace (optional)
	Namespace string

	// Revision to sync to (optional, defaults to target revision)
	Revision string

	// Prune removes resources that are no longer in git
	Prune bool

	// DryRun performs a dry-run sync
	DryRun bool

	// Resources to sync (optional, empty means all)
	Resources []string
}

// RollbackRequest represents a rollback request
type RollbackRequest struct {
	// Application name
	Application string

	// Namespace (optional)
	Namespace string

	// Revision to rollback to (required)
	Revision string

	// Prune removes resources
	Prune bool
}

// AnnotationUpdate represents an annotation update
type AnnotationUpdate struct {
	// Application name
	Application string

	// Namespace (optional)
	Namespace string

	// Annotations to set
	Annotations map[string]string
}

// RevisionHistoryEntry represents a single deployment history entry
type RevisionHistoryEntry struct {
	// Revision is the git commit SHA or tag
	Revision string

	// DeployedAt is when this revision was deployed
	DeployedAt time.Time

	// ID is the deployment ID (incrementing number)
	ID int64

	// Source is the application source at the time of deployment
	Source *ApplicationSource

	// DeployStartedAt is when the deployment started
	DeployStartedAt time.Time
}

// ApplicationSource represents the source of an ArgoCD application
type ApplicationSource struct {
	// RepoURL is the git repository URL
	RepoURL string

	// Path is the path within the repository
	Path string

	// TargetRevision is the target revision (branch, tag, or commit)
	TargetRevision string

	// Chart is the Helm chart name (for Helm applications)
	Chart string
}

// RevisionHistory is a list of revision history entries
type RevisionHistory []*RevisionHistoryEntry

// GetPrevious returns the previous revision (n-1) if it exists
func (h RevisionHistory) GetPrevious() *RevisionHistoryEntry {
	if len(h) < 2 {
		return nil
	}
	// History is typically ordered newest first
	return h[1]
}

// GetByID returns the history entry with the given ID
func (h RevisionHistory) GetByID(id int64) *RevisionHistoryEntry {
	for _, entry := range h {
		if entry.ID == id {
			return entry
		}
	}
	return nil
}

// GetLastHealthy would return the last healthy deployment
// Note: Health status is not stored in history, so this returns nil
// Real implementation would need to track health status separately
func (h RevisionHistory) GetLastHealthy() *RevisionHistoryEntry {
	// ArgoCD doesn't store health status in history
	// The caller should track this separately or use a different approach
	return nil
}
