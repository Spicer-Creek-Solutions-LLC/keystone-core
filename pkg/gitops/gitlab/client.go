package gitlab

import (
	"context"
	"fmt"

	gitlab "gitlab.com/gitlab-org/api/client-go"
)

// Client is a GitLab API client
type Client struct {
	config *Config
	client *gitlab.Client
}

// NewClient creates a new GitLab client
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.Token == "" {
		return nil, fmt.Errorf("GitLab token is required")
	}

	// Create GitLab client
	opts := []gitlab.ClientOptionFunc{}
	if config.BaseURL != "" && config.BaseURL != "https://gitlab.com" {
		opts = append(opts, gitlab.WithBaseURL(config.BaseURL))
	}

	glClient, err := gitlab.NewClient(config.Token, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to create GitLab client: %w", err)
	}

	return &Client{
		config: config,
		client: glClient,
	}, nil
}

// CreateMergeRequest creates a new merge request
func (c *Client) CreateMergeRequest(ctx context.Context, req *MergeRequestRequest) (*MergeRequestInfo, error) {
	projectID := req.ProjectID
	if projectID == "" {
		projectID = c.config.ProjectID
	}

	if projectID == "" {
		return nil, fmt.Errorf("project ID must be specified")
	}

	// Create MR
	createOpts := &gitlab.CreateMergeRequestOptions{
		Title:              &req.Title,
		Description:        &req.Description,
		SourceBranch:       &req.SourceBranch,
		TargetBranch:       &req.TargetBranch,
		RemoveSourceBranch: &req.RemoveSourceBranch,
		AllowCollaboration: &req.AllowCollaboration,
	}

	mr, _, err := c.client.MergeRequests.CreateMergeRequest(projectID, createOpts)
	if err != nil {
		return nil, fmt.Errorf("failed to create merge request: %w", err)
	}

	mergedAt := ""
	if mr.MergedAt != nil {
		mergedAt = mr.MergedAt.String()
	}

	return &MergeRequestInfo{
		IID:          mr.IID,
		Title:        mr.Title,
		State:        mr.State,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		MergedAt:     mergedAt,
		Draft:        mr.Draft,
		WebURL:       mr.WebURL,
	}, nil
}

// UpdateCommitStatus updates the status of a commit
func (c *Client) UpdateCommitStatus(ctx context.Context, req *CommitStatusRequest) error {
	projectID := req.ProjectID
	if projectID == "" {
		projectID = c.config.ProjectID
	}

	if projectID == "" {
		return fmt.Errorf("project ID must be specified")
	}

	// Create status
	opts := &gitlab.SetCommitStatusOptions{
		State:       gitlab.BuildStateValue(req.State),
		TargetURL:   &req.TargetURL,
		Description: &req.Description,
		Name:        &req.Name,
	}

	_, _, err := c.client.Commits.SetCommitStatus(projectID, req.Ref, opts)
	if err != nil {
		return fmt.Errorf("failed to set commit status: %w", err)
	}

	return nil
}

// CommentOnMR adds a comment to a merge request
func (c *Client) CommentOnMR(ctx context.Context, req *CommentRequest) error {
	projectID := req.ProjectID
	if projectID == "" {
		projectID = c.config.ProjectID
	}

	if projectID == "" {
		return fmt.Errorf("project ID must be specified")
	}

	// Create note (comment)
	opts := &gitlab.CreateMergeRequestNoteOptions{
		Body: &req.Body,
	}

	_, _, err := c.client.Notes.CreateMergeRequestNote(projectID, req.MRIID, opts)
	if err != nil {
		return fmt.Errorf("failed to create merge request note: %w", err)
	}

	return nil
}

// GetMergeRequest retrieves information about a merge request
func (c *Client) GetMergeRequest(ctx context.Context, projectID string, iid int64) (*MergeRequestInfo, error) {
	if projectID == "" {
		projectID = c.config.ProjectID
	}

	if projectID == "" {
		return nil, fmt.Errorf("project ID must be specified")
	}

	mr, _, err := c.client.MergeRequests.GetMergeRequest(projectID, iid, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get merge request: %w", err)
	}

	mergedAt := ""
	if mr.MergedAt != nil {
		mergedAt = mr.MergedAt.String()
	}

	return &MergeRequestInfo{
		IID:          mr.IID,
		Title:        mr.Title,
		State:        mr.State,
		SourceBranch: mr.SourceBranch,
		TargetBranch: mr.TargetBranch,
		MergedAt:     mergedAt,
		Draft:        mr.Draft,
		WebURL:       mr.WebURL,
	}, nil
}

// ListMergeRequests lists merge requests
func (c *Client) ListMergeRequests(ctx context.Context, projectID, state string) ([]*MergeRequestInfo, error) {
	if projectID == "" {
		projectID = c.config.ProjectID
	}

	if projectID == "" {
		return nil, fmt.Errorf("project ID must be specified")
	}

	opts := &gitlab.ListProjectMergeRequestsOptions{
		State: &state,
	}

	mrs, _, err := c.client.MergeRequests.ListProjectMergeRequests(projectID, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list merge requests: %w", err)
	}

	result := make([]*MergeRequestInfo, len(mrs))
	for i, mr := range mrs {
		mergedAt := ""
		if mr.MergedAt != nil {
			mergedAt = mr.MergedAt.String()
		}

		result[i] = &MergeRequestInfo{
			IID:          mr.IID,
			Title:        mr.Title,
			State:        mr.State,
			SourceBranch: mr.SourceBranch,
			TargetBranch: mr.TargetBranch,
			MergedAt:     mergedAt,
			Draft:        mr.Draft,
			WebURL:       mr.WebURL,
		}
	}

	return result, nil
}

// MergeMergeRequest merges a merge request
func (c *Client) MergeMergeRequest(ctx context.Context, projectID string, iid int64, message string) error {
	if projectID == "" {
		projectID = c.config.ProjectID
	}

	if projectID == "" {
		return fmt.Errorf("project ID must be specified")
	}

	opts := &gitlab.AcceptMergeRequestOptions{
		MergeCommitMessage: &message,
	}

	_, _, err := c.client.MergeRequests.AcceptMergeRequest(projectID, iid, opts)
	if err != nil {
		return fmt.Errorf("failed to merge merge request: %w", err)
	}

	return nil
}
