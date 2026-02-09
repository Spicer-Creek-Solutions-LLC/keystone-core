package rollback

import (
	"context"
	"fmt"

	"github.com/shawnbutts/keystone-core/internal/gitops/argocd"
)

// ArgoCDExecutor executes rollbacks via ArgoCD
type ArgoCDExecutor struct {
	client *argocd.Client
}

// NewArgoCDExecutor creates a new ArgoCD rollback executor
func NewArgoCDExecutor(client *argocd.Client) *ArgoCDExecutor {
	return &ArgoCDExecutor{
		client: client,
	}
}

// Type returns the rollback type
func (e *ArgoCDExecutor) Type() Type {
	return TypeArgoCD
}

// Execute executes an ArgoCD rollback
func (e *ArgoCDExecutor) Execute(ctx context.Context, config *Config, request *Request) (*Result, error) {
	result := &Result{
		Config:  config,
		Request: request,
	}

	// Get current application status
	status, err := e.client.GetApplicationStatus(ctx, config.Application, config.Namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}

	result.PreviousRevision = status.Revision

	// Determine target revision
	targetRevision := request.OverrideRevision
	if targetRevision == "" {
		targetRevision = config.Revision
	}

	// Execute rollback
	err = e.client.Rollback(ctx, &argocd.RollbackRequest{
		Application: config.Application,
		Namespace:   config.Namespace,
		Revision:    targetRevision,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to rollback: %w", err)
	}

	result.CurrentRevision = targetRevision
	return result, nil
}

// GetPreviousRevision gets the previous revision for an ArgoCD application
func (e *ArgoCDExecutor) GetPreviousRevision(ctx context.Context, config *Config) (string, error) {
	// Get deployment history from ArgoCD
	history, err := e.client.GetApplicationHistory(ctx, config.Application, config.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get application history: %w", err)
	}

	if len(history) == 0 {
		return "", fmt.Errorf("no deployment history available")
	}

	// Get the previous deployment (second most recent)
	prev := history.GetPrevious()
	if prev == nil {
		return "", fmt.Errorf("no previous revision available (only one deployment in history)")
	}

	return prev.Revision, nil
}

// GetLastKnownGood gets the last known good revision for an ArgoCD application
func (e *ArgoCDExecutor) GetLastKnownGood(ctx context.Context, config *Config) (string, error) {
	// First check if the current revision is healthy
	status, err := e.client.GetApplicationStatus(ctx, config.Application, config.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get application status: %w", err)
	}

	// If current state is healthy and synced, that's our last known good
	if status.HealthStatus == "Healthy" && status.SyncStatus == "Synced" && status.Revision != "" {
		return status.Revision, nil
	}

	// Current state is not healthy - check deployment history for a previous deployment
	// ArgoCD doesn't track health status per-revision in history, so the best we can do
	// is return the previous revision and assume it was working before this deployment
	history, err := e.client.GetApplicationHistory(ctx, config.Application, config.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get application history: %w", err)
	}

	if len(history) == 0 {
		return "", fmt.Errorf("no deployment history available")
	}

	// Return the previous revision (if current is unhealthy, previous is our best guess)
	// In a more sophisticated implementation, you would track health status per deployment
	// in an external database or use annotations to mark known-good revisions
	prev := history.GetPrevious()
	if prev != nil {
		return prev.Revision, nil
	}

	// If there's only one deployment and it's not healthy, we have no known good
	return "", fmt.Errorf("no known good revision found (current deployment is %s and no previous deployments)", status.HealthStatus)
}
