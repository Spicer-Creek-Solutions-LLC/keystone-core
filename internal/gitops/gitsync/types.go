// Package gitsync provides Git repository synchronization for tracking
// and applying configuration changes from remote repositories.
package gitsync

import "time"

// RepositoryConfig represents configuration for a Git repository
type RepositoryConfig struct {
	// Name is a unique identifier for this repository
	Name string `json:"name"`

	// URL is the Git repository URL (https:// or git@)
	URL string `json:"url"`

	// Branch to track
	Branch string `json:"branch"`

	// LocalPath where the repository should be cloned
	LocalPath string `json:"local_path"`

	// Auth contains authentication details
	Auth AuthConfig `json:"auth"`

	// SyncInterval is how often to check for updates
	SyncInterval time.Duration `json:"sync_interval"`

	// Paths within the repository to sync
	Paths PathsConfig `json:"paths"`
}

// AuthConfig represents Git authentication configuration
type AuthConfig struct {
	// Type of authentication (none, token, ssh)
	Type AuthType `json:"type"`

	// Username for HTTP basic auth
	Username string `json:"username,omitempty"`

	// Token for HTTP authentication
	Token string `json:"token,omitempty"`

	// SSHKeyPath for SSH authentication
	SSHKeyPath string `json:"ssh_key_path,omitempty"`

	// SSHKeyPassphrase for encrypted SSH keys
	SSHKeyPassphrase string `json:"ssh_key_passphrase,omitempty"`
}

// AuthType represents the authentication method
type AuthType string

// AuthType constants define the supported types.
const (
	AuthTypeNone  AuthType = "none"
	AuthTypeToken AuthType = "token"
	AuthTypeSSH   AuthType = "ssh"
)

// PathsConfig defines which paths to sync from the repository
type PathsConfig struct {
	// States directory path (e.g., "states/")
	States string `json:"states,omitempty"`

	// Reactors directory path (e.g., "reactors/")
	Reactors string `json:"reactors,omitempty"`

	// Vars directory path (e.g., "vars/")
	Vars string `json:"vars,omitempty"`

	// Workflows directory path (e.g., "workflows/")
	Workflows string `json:"workflows,omitempty"`
}

// SyncResult represents the result of a sync operation
type SyncResult struct {
	// Repository name
	Repository string `json:"repository"`

	// Success indicates if sync succeeded
	Success bool `json:"success"`

	// Updated indicates if there were changes
	Updated bool `json:"updated"`

	// PreviousCommit is the commit before sync
	PreviousCommit string `json:"previous_commit"`

	// CurrentCommit is the commit after sync
	CurrentCommit string `json:"current_commit"`

	// FilesChanged lists files that changed
	FilesChanged []string `json:"files_changed,omitempty"`

	// Message contains sync details
	Message string `json:"message"`

	// Error if sync failed
	Error error `json:"error,omitempty"`

	// Timestamp of the sync
	Timestamp time.Time `json:"timestamp"`
}

// CommitRequest represents a request to create a commit
type CommitRequest struct {
	// Repository name
	Repository string

	// Message for the commit
	Message string

	// Files to add (paths relative to repo root)
	Files []string

	// Author name
	AuthorName string

	// Author email
	AuthorEmail string
}

// BranchRequest represents a request to create a branch
type BranchRequest struct {
	// Repository name
	Repository string

	// BranchName to create
	BranchName string

	// FromBranch to branch from (defaults to current branch)
	FromBranch string
}

// PullRequestRequest represents a request to create a pull request
type PullRequestRequest struct {
	// Repository name
	Repository string

	// SourceBranch to merge from
	SourceBranch string

	// TargetBranch to merge into
	TargetBranch string

	// Title of the pull request
	Title string

	// Description of the pull request
	Description string
}
