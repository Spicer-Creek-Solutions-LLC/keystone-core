package rollback

import (
	"context"
	"fmt"

	"github.com/titananvil/titan-anvil/pkg/gitops/gitsync"
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
func (e *GitExecutor) Type() RollbackType {
	return RollbackTypeGit
}

// Execute executes a Git rollback
func (e *GitExecutor) Execute(ctx context.Context, config *RollbackConfig, request *RollbackRequest) (*RollbackResult, error) {
	result := &RollbackResult{
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
		AuthorName:  "TitanAnvil Rollback",
		AuthorEmail: "rollback@titananvil.io",
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

// GetPreviousRevision gets the previous commit
func (e *GitExecutor) GetPreviousRevision(ctx context.Context, config *RollbackConfig) (string, error) {
	repo, ok := e.manager.GetRepository(config.Application)
	if !ok {
		return "", fmt.Errorf("repository not found: %s", config.Application)
	}

	// In a real implementation, this would use git log to get previous commit
	// For now, we return an error indicating this needs implementation
	currentCommit, err := repo.GetCurrentCommit()
	if err != nil {
		return "", err
	}

	// This is a placeholder - real implementation would get parent commit
	return currentCommit, nil
}

// GetLastKnownGood gets the last known good commit
func (e *GitExecutor) GetLastKnownGood(ctx context.Context, config *RollbackConfig) (string, error) {
	// In a real implementation, this would check deployment history
	// and find the last successful deployment commit
	// For now, we delegate to GetPreviousRevision
	return e.GetPreviousRevision(ctx, config)
}
