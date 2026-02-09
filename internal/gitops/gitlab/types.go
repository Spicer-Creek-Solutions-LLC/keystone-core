// Package gitlab provides GitLab API integration for merge request management
// and GitOps automation workflows.
package gitlab

// Config represents GitLab client configuration
type Config struct {
	// Token is the GitLab personal access token or project token
	Token string

	// BaseURL is the GitLab API base URL (for self-hosted GitLab)
	BaseURL string

	// ProjectID is the default project ID or path (e.g., "group/project")
	ProjectID string
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		BaseURL: "https://gitlab.com",
	}
}

// MergeRequestRequest represents an MR creation request
type MergeRequestRequest struct {
	// ProjectID (optional, uses config default)
	ProjectID string

	// Title of the MR
	Title string

	// Description
	Description string

	// SourceBranch
	SourceBranch string

	// TargetBranch
	TargetBranch string

	// RemoveSourceBranch after merge
	RemoveSourceBranch bool

	// AllowCollaboration
	AllowCollaboration bool
}

// CommitStatusRequest represents a commit status update request
type CommitStatusRequest struct {
	// ProjectID (optional)
	ProjectID string

	// Ref (commit SHA)
	Ref string

	// State (pending, running, success, failed, canceled)
	State string

	// TargetURL for more details
	TargetURL string

	// Description of the status
	Description string

	// Name of the status check
	Name string
}

// CommentRequest represents an MR comment request
type CommentRequest struct {
	// ProjectID (optional)
	ProjectID string

	// MR IID (internal ID)
	MRIID int64

	// Comment body
	Body string
}

// MergeRequestInfo represents MR information
type MergeRequestInfo struct {
	// IID is the internal ID
	IID int64

	// Title of the MR
	Title string

	// State (opened, closed, locked, merged)
	State string

	// SourceBranch
	SourceBranch string

	// TargetBranch
	TargetBranch string

	// MergedAt timestamp (nil if not merged)
	MergedAt string

	// Draft indicates if MR is a draft
	Draft bool

	// WebURL to the MR
	WebURL string
}
