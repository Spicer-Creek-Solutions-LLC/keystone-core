package argocd

import (
	"context"
	"fmt"
	"time"

	"github.com/argoproj/argo-cd/v2/pkg/apiclient"
	"github.com/argoproj/argo-cd/v2/pkg/apiclient/application"
	argocdapp "github.com/argoproj/argo-cd/v2/pkg/apis/application/v1alpha1"
	"google.golang.org/grpc"
)

// Client is an ArgoCD API client
type Client struct {
	config    *Config
	apiClient apiclient.Client
	appClient application.ApplicationServiceClient
}

// NewClient creates a new ArgoCD client
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Create API client options
	opts := apiclient.ClientOptions{
		ServerAddr: config.ServerAddr,
		AuthToken:  config.Token,
		Insecure:   config.Insecure,
		GRPCWeb:    true,
	}

	// Create API client
	apiClient, err := apiclient.NewClient(&opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create ArgoCD API client: %w", err)
	}

	// Get application service client
	conn, appClient, err := apiClient.NewApplicationClient()
	if err != nil {
		return nil, fmt.Errorf("failed to create application client: %w", err)
	}
	_ = conn // Keep connection alive

	return &Client{
		config:    config,
		apiClient: apiClient,
		appClient: appClient,
	}, nil
}

// GetApplicationStatus retrieves the status of an application
func (c *Client) GetApplicationStatus(ctx context.Context, name, namespace string) (*ApplicationStatus, error) {
	if namespace == "" {
		namespace = "argocd"
	}

	// Create request
	query := &application.ApplicationQuery{
		Name:         &name,
		AppNamespace: &namespace,
	}

	// Get application
	app, err := c.appClient.Get(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get application: %w", err)
	}

	// Extract status
	status := &ApplicationStatus{
		Name:           app.Name,
		Namespace:      app.Namespace,
		SyncStatus:     string(app.Status.Sync.Status),
		HealthStatus:   string(app.Status.Health.Status),
		Revision:       app.Status.Sync.Revision,
		RepoURL:        app.Spec.Source.RepoURL,
		TargetRevision: app.Spec.Source.TargetRevision,
		ObservedAt:     time.Now(),
	}

	if app.Status.OperationState != nil {
		status.OperationPhase = string(app.Status.OperationState.Phase)
		if app.Status.OperationState.Message != "" {
			status.Message = app.Status.OperationState.Message
		}
	}

	if app.Status.Conditions != nil && len(app.Status.Conditions) > 0 {
		status.Message = app.Status.Conditions[0].Message
	}

	return status, nil
}

// Sync triggers a sync operation for an application
func (c *Client) Sync(ctx context.Context, req *SyncRequest) error {
	if req.Namespace == "" {
		req.Namespace = "argocd"
	}

	// Create sync request
	syncReq := &application.ApplicationSyncRequest{
		Name:         &req.Application,
		AppNamespace: &req.Namespace,
		Prune:        &req.Prune,
		DryRun:       &req.DryRun,
	}

	if req.Revision != "" {
		syncReq.Revision = &req.Revision
	}

	if len(req.Resources) > 0 {
		syncReq.Resources = make([]*argocdapp.SyncOperationResource, len(req.Resources))
		for i, res := range req.Resources {
			syncReq.Resources[i] = &argocdapp.SyncOperationResource{
				Name: res,
			}
		}
	}

	// Trigger sync
	_, err := c.appClient.Sync(ctx, syncReq)
	if err != nil {
		return fmt.Errorf("failed to sync application: %w", err)
	}

	return nil
}

// Rollback rolls back an application to a previous revision
func (c *Client) Rollback(ctx context.Context, req *RollbackRequest) error {
	if req.Namespace == "" {
		req.Namespace = "argocd"
	}

	// Create rollback request (implemented as sync to specific revision)
	syncReq := &SyncRequest{
		Application: req.Application,
		Namespace:   req.Namespace,
		Revision:    req.Revision,
		Prune:       req.Prune,
	}

	return c.Sync(ctx, syncReq)
}

// UpdateAnnotations updates annotations on an application
func (c *Client) UpdateAnnotations(ctx context.Context, req *AnnotationUpdate) error {
	if req.Namespace == "" {
		req.Namespace = "argocd"
	}

	// Get current application
	query := &application.ApplicationQuery{
		Name:         &req.Application,
		AppNamespace: &req.Namespace,
	}

	app, err := c.appClient.Get(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to get application: %w", err)
	}

	// Update annotations
	if app.Annotations == nil {
		app.Annotations = make(map[string]string)
	}
	for k, v := range req.Annotations {
		app.Annotations[k] = v
	}

	// Update application
	updateReq := &application.ApplicationUpdateRequest{
		Application: app,
	}

	_, err = c.appClient.Update(ctx, updateReq)
	if err != nil {
		return fmt.Errorf("failed to update application: %w", err)
	}

	return nil
}

// List lists all applications
func (c *Client) List(ctx context.Context, namespace string) ([]*ApplicationStatus, error) {
	if namespace == "" {
		namespace = "argocd"
	}

	// Create request
	query := &application.ApplicationQuery{
		AppNamespace: &namespace,
	}

	// List applications
	appList, err := c.appClient.List(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("failed to list applications: %w", err)
	}

	// Convert to status list
	statuses := make([]*ApplicationStatus, len(appList.Items))
	for i, app := range appList.Items {
		statuses[i] = &ApplicationStatus{
			Name:           app.Name,
			Namespace:      app.Namespace,
			SyncStatus:     string(app.Status.Sync.Status),
			HealthStatus:   string(app.Status.Health.Status),
			Revision:       app.Status.Sync.Revision,
			RepoURL:        app.Spec.Source.RepoURL,
			TargetRevision: app.Spec.Source.TargetRevision,
			ObservedAt:     time.Now(),
		}

		if app.Status.OperationState != nil {
			statuses[i].OperationPhase = string(app.Status.OperationState.Phase)
		}
	}

	return statuses, nil
}

// Close closes the client connection
func (c *Client) Close() error {
	// ArgoCD client doesn't have a close method
	// Connections are managed by the underlying gRPC client
	return nil
}

// NewClientWithOptions creates a client with custom gRPC options
func NewClientWithOptions(config *Config, opts ...grpc.DialOption) (*Client, error) {
	// For now, just use the standard client
	// Custom gRPC options can be added in the future if needed
	return NewClient(config)
}
