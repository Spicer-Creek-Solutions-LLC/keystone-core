package rollback

import (
	"context"
	"fmt"

	"github.com/shawnbutts/keystone-core/internal/gitops/gitsync"
)

// GitExecutor executes rollbacks via Git operations
type GitExecutor struct {
	manager *gitsync.Manager
}

// NewGitExecutor creates a new Git rollback executor
func NewGitExecutor(manager *gitsync.Manager) *GitExecutor {
	return &GitExecutor{
		manager: manager,
	}
}

// Type returns the rollback type
func (e *GitExecutor) Type() Type {
	return TypeGit
}

// Execute executes a Git rollback
func (e *GitExecutor) Execute(ctx context.Context, config *Config, request *Request) (*Result, error) {
	result := &Result{
		Config:  config,
		Request: request,
	}

	// Get repository
	repo, ok := e.manager.GetRepository(config.Application)
	if !ok {
		return nil, fmt.Errorf("repository not found: %s", config.Application)
	}

	// Get current commit
	currentCommit, err := repo.GetCurrentCommit()
	if err != nil {
		return nil, fmt.Errorf("failed to get current commit: %w", err)
	}
	result.PreviousRevision = currentCommit

	// Determine target commit
	targetCommit := request.OverrideRevision
	if targetCommit == "" {
		targetCommit = config.Revision
	}

	result.CurrentRevision = targetCommit

	// Create rollback branch
	branchName := fmt.Sprintf("rollback-%s-%s", config.Application, result.ID)
	err = repo.CreateBranch(&gitsync.BranchRequest{
		Repository: config.Application,
		BranchName: branchName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create rollback branch: %w", err)
	}

	// Create revert commit
	err = repo.Commit(ctx, &gitsync.CommitRequest{
		Repository: config.Application,
		Message: fmt.Sprintf("Rollback to %s\n\nReason: %s\nRequested by: %s",
			targetCommit[:7], request.Reason, request.RequestedBy),
		Files:       []string{}, // Would populate with actual files to revert
		AuthorName:  "Keystone Core Rollback",
		AuthorEmail: "rollback@keystonecore.io",
	})
	if err != nil {
		return nil, fmt.Errorf("failed to commit rollback: %w", err)
	}

	// Push rollback branch
	err = repo.Push(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to push rollback: %w", err)
	}

	return result, nil
}

// GetPreviousRevision gets the previous commit (parent of HEAD)
func (e *GitExecutor) GetPreviousRevision(ctx context.Context, config *Config) (string, error) {
	repo, ok := e.manager.GetRepository(config.Application)
	if !ok {
		return "", fmt.Errorf("repository not found: %s", config.Application)
	}

	previousCommit, err := repo.GetPreviousCommit()
	if err != nil {
		return "", fmt.Errorf("failed to get previous commit: %w", err)
	}

	return previousCommit, nil
}

// GetLastKnownGood gets the last known good commit
// Since Git doesn't track deployment success, this returns the previous commit
// as the best approximation. For production use, you should track deployment
// status externally (e.g., via annotations, tags, or a deployment database).
func (e *GitExecutor) GetLastKnownGood(ctx context.Context, config *Config) (string, error) {
	repo, ok := e.manager.GetRepository(config.Application)
	if !ok {
		return "", fmt.Errorf("repository not found: %s", config.Application)
	}

	// Get commit history to find a previous deployment
	// In a more sophisticated implementation, you would:
	// 1. Look for Git tags marking successful deployments
	// 2. Check an external deployment database
	// 3. Use annotations on commits
	history, err := repo.GetCommitHistory(10)
	if err != nil {
		return "", fmt.Errorf("failed to get commit history: %w", err)
	}

	if len(history) < 2 {
		return "", fmt.Errorf("no previous commits available (history has %d commits)", len(history))
	}

	// Return the second commit (first previous) as the best approximation
	// The first commit is HEAD (current), second is previous
	return history[1].Hash, nil
}
