// Package github provides GitHub API integration for pull request management
// and GitOps automation workflows.
package github

// Config represents GitHub client configuration
type Config struct {
	// Token is the GitHub personal access token or app token
	Token string

	// BaseURL is the GitHub API base URL (for GitHub Enterprise)
	BaseURL string

	// Owner is the default repository owner
	Owner string

	// Repo is the default repository name
	Repo string
}

// DefaultConfig returns default configuration
func DefaultConfig() *Config {
	return &Config{
		BaseURL: "https://api.github.com",
	}
}

// PullRequestRequest represents a PR creation request
type PullRequestRequest struct {
	// Owner of the repository (optional, uses config default)
	Owner string

	// Repo name (optional, uses config default)
	Repo string

	// Title of the PR
	Title string

	// Body description
	Body string

	// Head branch (source)
	Head string

	// Base branch (target)
	Base string

	// Draft PR
	Draft bool

	// Maintainer can modify
	MaintainerCanModify bool
}

// CommitStatusRequest represents a commit status update request
type CommitStatusRequest struct {
	// Owner of the repository (optional)
	Owner string

	// Repo name (optional)
	Repo string

	// Ref (commit SHA, branch name, or tag name)
	Ref string

	// State (error, failure, pending, success)
	State string

	// TargetURL for more details
	TargetURL string

	// Description of the status
	Description string

	// Context identifies the status check
	Context string
}

// CommentRequest represents a PR comment request
type CommentRequest struct {
	// Owner of the repository (optional)
	Owner string

	// Repo name (optional)
	Repo string

	// PR number
	PRNumber int

	// Comment body
	Body string
}

// PullRequestInfo represents PR information
type PullRequestInfo struct {
	// Number is the PR number
	Number int

	// Title of the PR
	Title string

	// State (open, closed, merged)
	State string

	// Head branch
	Head string

	// Base branch
	Base string

	// Merged indicates if PR was merged
	Merged bool

	// Draft indicates if PR is a draft
	Draft bool

	// URL to the PR
	URL string
}
