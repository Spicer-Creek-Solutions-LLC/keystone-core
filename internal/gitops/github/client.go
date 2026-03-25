package github

import (
	"context"
	"fmt"

	"github.com/google/go-github/v57/github"
	"golang.org/x/oauth2"

	"github.com/shawnbutts/keystone-core/internal/gitops"
)

// Client is a GitHub API client
type Client struct {
	config *Config
	client *github.Client
}

// NewClient creates a new GitHub client
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.Token == "" {
		return nil, fmt.Errorf("GitHub token is required")
	}

	// Create OAuth2 token source with circuit breaker transport
	ts := oauth2.StaticTokenSource(
		&oauth2.Token{AccessToken: config.Token},
	)
	tc := oauth2.NewClient(context.Background(), ts)
	tc.Transport = gitops.NewCircuitBreakerTransport(tc.Transport, gitops.CircuitBreakerConfig{})

	// Create GitHub client
	client := github.NewClient(tc)
	if config.BaseURL != "" && config.BaseURL != "https://api.github.com" {
		var err error
		client, err = client.WithEnterpriseURLs(config.BaseURL, config.BaseURL)
		if err != nil {
			return nil, fmt.Errorf("failed to set enterprise URL: %w", err)
		}
	}

	return &Client{
		config: config,
		client: client,
	}, nil
}

// CreatePullRequest creates a new pull request
func (c *Client) CreatePullRequest(ctx context.Context, req *PullRequestRequest) (*PullRequestInfo, error) {
	owner := req.Owner
	if owner == "" {
		owner = c.config.Owner
	}

	repo := req.Repo
	if repo == "" {
		repo = c.config.Repo
	}

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner and repo must be specified")
	}

	// Create PR
	newPR := &github.NewPullRequest{
		Title:               github.String(req.Title),
		Head:                github.String(req.Head),
		Base:                github.String(req.Base),
		Body:                github.String(req.Body),
		MaintainerCanModify: github.Bool(req.MaintainerCanModify),
		Draft:               github.Bool(req.Draft),
	}

	pr, _, err := c.client.PullRequests.Create(ctx, owner, repo, newPR)
	if err != nil {
		return nil, fmt.Errorf("failed to create pull request: %w", err)
	}

	return &PullRequestInfo{
		Number: pr.GetNumber(),
		Title:  pr.GetTitle(),
		State:  pr.GetState(),
		Head:   pr.GetHead().GetRef(),
		Base:   pr.GetBase().GetRef(),
		Merged: pr.GetMerged(),
		Draft:  pr.GetDraft(),
		URL:    pr.GetHTMLURL(),
	}, nil
}

// UpdateCommitStatus updates the status of a commit
func (c *Client) UpdateCommitStatus(ctx context.Context, req *CommitStatusRequest) error {
	owner := req.Owner
	if owner == "" {
		owner = c.config.Owner
	}

	repo := req.Repo
	if repo == "" {
		repo = c.config.Repo
	}

	if owner == "" || repo == "" {
		return fmt.Errorf("owner and repo must be specified")
	}

	// Create status
	status := &github.RepoStatus{
		State:       github.String(req.State),
		TargetURL:   github.String(req.TargetURL),
		Description: github.String(req.Description),
		Context:     github.String(req.Context),
	}

	_, _, err := c.client.Repositories.CreateStatus(ctx, owner, repo, req.Ref, status)
	if err != nil {
		return fmt.Errorf("failed to create commit status: %w", err)
	}

	return nil
}

// CommentOnPR adds a comment to a pull request
func (c *Client) CommentOnPR(ctx context.Context, req *CommentRequest) error {
	owner := req.Owner
	if owner == "" {
		owner = c.config.Owner
	}

	repo := req.Repo
	if repo == "" {
		repo = c.config.Repo
	}

	if owner == "" || repo == "" {
		return fmt.Errorf("owner and repo must be specified")
	}

	// Create comment
	comment := &github.IssueComment{
		Body: github.String(req.Body),
	}

	_, _, err := c.client.Issues.CreateComment(ctx, owner, repo, req.PRNumber, comment)
	if err != nil {
		return fmt.Errorf("failed to create comment: %w", err)
	}

	return nil
}

// GetPullRequest retrieves information about a pull request
func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (*PullRequestInfo, error) {
	if owner == "" {
		owner = c.config.Owner
	}

	if repo == "" {
		repo = c.config.Repo
	}

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner and repo must be specified")
	}

	pr, _, err := c.client.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		return nil, fmt.Errorf("failed to get pull request: %w", err)
	}

	return &PullRequestInfo{
		Number: pr.GetNumber(),
		Title:  pr.GetTitle(),
		State:  pr.GetState(),
		Head:   pr.GetHead().GetRef(),
		Base:   pr.GetBase().GetRef(),
		Merged: pr.GetMerged(),
		Draft:  pr.GetDraft(),
		URL:    pr.GetHTMLURL(),
	}, nil
}

// ListPullRequests lists pull requests
func (c *Client) ListPullRequests(ctx context.Context, owner, repo, state string) ([]*PullRequestInfo, error) {
	if owner == "" {
		owner = c.config.Owner
	}

	if repo == "" {
		repo = c.config.Repo
	}

	if owner == "" || repo == "" {
		return nil, fmt.Errorf("owner and repo must be specified")
	}

	opts := &github.PullRequestListOptions{
		State: state,
	}

	prs, _, err := c.client.PullRequests.List(ctx, owner, repo, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to list pull requests: %w", err)
	}

	result := make([]*PullRequestInfo, len(prs))
	for i, pr := range prs {
		result[i] = &PullRequestInfo{
			Number: pr.GetNumber(),
			Title:  pr.GetTitle(),
			State:  pr.GetState(),
			Head:   pr.GetHead().GetRef(),
			Base:   pr.GetBase().GetRef(),
			Merged: pr.GetMerged(),
			Draft:  pr.GetDraft(),
			URL:    pr.GetHTMLURL(),
		}
	}

	return result, nil
}

// MergePullRequest merges a pull request
func (c *Client) MergePullRequest(ctx context.Context, owner, repo string, number int, commitMessage string) error {
	if owner == "" {
		owner = c.config.Owner
	}

	if repo == "" {
		repo = c.config.Repo
	}

	if owner == "" || repo == "" {
		return fmt.Errorf("owner and repo must be specified")
	}

	opts := &github.PullRequestOptions{}

	_, _, err := c.client.PullRequests.Merge(ctx, owner, repo, number, commitMessage, opts)
	if err != nil {
		return fmt.Errorf("failed to merge pull request: %w", err)
	}

	return nil
}
