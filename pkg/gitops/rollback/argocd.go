package rollback

import (
	"context"
	"fmt"

	"github.com/shawnbutts/keystone-core/pkg/gitops/argocd"
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
func (e *ArgoCDExecutor) Type() RollbackType {
	return RollbackTypeArgoCD
}

// Execute executes an ArgoCD rollback
func (e *ArgoCDExecutor) Execute(ctx context.Context, config *RollbackConfig, request *RollbackRequest) (*RollbackResult, error) {
	result := &RollbackResult{
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
func (e *ArgoCDExecutor) GetPreviousRevision(ctx context.Context, config *RollbackConfig) (string, error) {
	status, err := e.client.GetApplicationStatus(ctx, config.Application, config.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get application: %w", err)
	}

	// For now, return the current revision since we don't have access to history
	// In a production implementation, this would query the application history
	// through a direct gRPC call to get historical revisions
	if status.Revision == "" {
		return "", fmt.Errorf("no revision available")
	}

	// This is a placeholder - real implementation would get parent commit from Git
	return status.Revision, nil
}

// GetLastKnownGood gets the last known good revision for an ArgoCD application
func (e *ArgoCDExecutor) GetLastKnownGood(ctx context.Context, config *RollbackConfig) (string, error) {
	status, err := e.client.GetApplicationStatus(ctx, config.Application, config.Namespace)
	if err != nil {
		return "", fmt.Errorf("failed to get application: %w", err)
	}

	// In a real implementation, this would check deployment history and health status
	// to find the last known good revision. For now, we return the current revision
	// if the application is healthy
	if status.HealthStatus == "Healthy" && status.Revision != "" {
		return status.Revision, nil
	}

	return "", fmt.Errorf("no known good revision found")
}
