package flux

import (
	"context"
	"fmt"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// Client is a Flux API client
type Client struct {
	config  *Config
	dynamic dynamic.Interface
}

// NewClient creates a new Flux client
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	}

	// Build kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if config.Kubeconfig != "" {
		loadingRules.ExplicitPath = config.Kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
	if config.Context != "" {
		configOverrides.CurrentContext = config.Context
	}

	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)
	restConfig, err := kubeConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to create rest config: %w", err)
	}

	// Create dynamic client
	dynamicClient, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &Client{
		config:  config,
		dynamic: dynamicClient,
	}, nil
}

// GetResourceStatus retrieves the status of a Flux resource
func (c *Client) GetResourceStatus(ctx context.Context, kind ResourceKind, name, namespace string) (*ResourceStatus, error) {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	// Get GVR for resource kind
	gvr := c.gvrForKind(kind)

	// Get resource
	obj, err := c.dynamic.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to get resource: %w", err)
	}

	return c.parseResourceStatus(obj, kind)
}

// Suspend suspends or resumes reconciliation for a resource
func (c *Client) Suspend(ctx context.Context, req *SuspendRequest) error {
	if req.Namespace == "" {
		req.Namespace = c.config.Namespace
	}

	// Get GVR for resource kind
	gvr := c.gvrForKind(req.Kind)

	// Get current resource
	obj, err := c.dynamic.Resource(gvr).Namespace(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get resource: %w", err)
	}

	// Update suspend field
	if err := unstructured.SetNestedField(obj.Object, req.Suspend, "spec", "suspend"); err != nil {
		return fmt.Errorf("failed to set suspend field: %w", err)
	}

	// Update resource
	_, err = c.dynamic.Resource(gvr).Namespace(req.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update resource: %w", err)
	}

	return nil
}

// Reconcile triggers immediate reconciliation by updating the reconcile annotation
func (c *Client) Reconcile(ctx context.Context, req *ReconcileRequest) error {
	if req.Namespace == "" {
		req.Namespace = c.config.Namespace
	}

	// Get GVR for resource kind
	gvr := c.gvrForKind(req.Kind)

	// Get current resource
	obj, err := c.dynamic.Resource(gvr).Namespace(req.Namespace).Get(ctx, req.Name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("failed to get resource: %w", err)
	}

	// Update reconcile annotation
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations["reconcile.fluxcd.io/requestedAt"] = time.Now().Format(time.RFC3339)
	obj.SetAnnotations(annotations)

	// Update resource
	_, err = c.dynamic.Resource(gvr).Namespace(req.Namespace).Update(ctx, obj, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update resource: %w", err)
	}

	return nil
}

// List lists all resources of a specific kind
func (c *Client) List(ctx context.Context, kind ResourceKind, namespace string) ([]*ResourceStatus, error) {
	if namespace == "" {
		namespace = c.config.Namespace
	}

	// Get GVR for resource kind
	gvr := c.gvrForKind(kind)

	// List resources
	list, err := c.dynamic.Resource(gvr).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("failed to list resources: %w", err)
	}

	// Convert to status list
	statuses := make([]*ResourceStatus, len(list.Items))
	for i, obj := range list.Items {
		status, err := c.parseResourceStatus(&obj, kind)
		if err != nil {
			return nil, fmt.Errorf("failed to parse resource status: %w", err)
		}
		statuses[i] = status
	}

	return statuses, nil
}

// gvrForKind returns the GroupVersionResource for a Flux resource kind
func (c *Client) gvrForKind(kind ResourceKind) schema.GroupVersionResource {
	switch kind {
	case KindKustomization:
		return schema.GroupVersionResource{
			Group:    "kustomize.toolkit.fluxcd.io",
			Version:  "v1",
			Resource: "kustomizations",
		}
	case KindHelmRelease:
		return schema.GroupVersionResource{
			Group:    "helm.toolkit.fluxcd.io",
			Version:  "v2beta1",
			Resource: "helmreleases",
		}
	case KindGitRepository:
		return schema.GroupVersionResource{
			Group:    "source.toolkit.fluxcd.io",
			Version:  "v1",
			Resource: "gitrepositories",
		}
	case KindHelmRepository:
		return schema.GroupVersionResource{
			Group:    "source.toolkit.fluxcd.io",
			Version:  "v1beta2",
			Resource: "helmrepositories",
		}
	default:
		return schema.GroupVersionResource{}
	}
}

// parseResourceStatus parses an unstructured object into ResourceStatus
func (c *Client) parseResourceStatus(obj *unstructured.Unstructured, kind ResourceKind) (*ResourceStatus, error) {
	status := &ResourceStatus{
		Kind:       kind,
		Name:       obj.GetName(),
		Namespace:  obj.GetNamespace(),
		ObservedAt: time.Now(),
	}

	// Get suspend status
	suspended, found, err := unstructured.NestedBool(obj.Object, "spec", "suspend")
	if err == nil && found {
		status.Suspended = suspended
	}

	// Get status fields
	statusObj, found, err := unstructured.NestedMap(obj.Object, "status")
	if err != nil || !found {
		return status, nil
	}

	// Parse conditions
	conditions, found, err := unstructured.NestedSlice(statusObj, "conditions")
	if err == nil && found {
		for _, cond := range conditions {
			condMap, ok := cond.(map[string]interface{})
			if !ok {
				continue
			}
			condition := Condition{
				Type:    getStringField(condMap, "type"),
				Status:  getStringField(condMap, "status"),
				Reason:  getStringField(condMap, "reason"),
				Message: getStringField(condMap, "message"),
			}
			status.Conditions = append(status.Conditions, condition)

			// Check if resource is ready
			if condition.Type == "Ready" && condition.Status == "True" {
				status.Ready = true
			}

			if condition.Message != "" {
				status.Message = condition.Message
			}
		}
	}

	// Get revision
	if rev, found, err := unstructured.NestedString(statusObj, "lastAppliedRevision"); err == nil && found {
		status.Revision = rev
	}

	// Get last reconcile time
	if reconcileTime, found, err := unstructured.NestedString(statusObj, "lastReconcileTime"); err == nil && found {
		if t, err := time.Parse(time.RFC3339, reconcileTime); err == nil {
			status.LastReconcileTime = t
		}
	}

	return status, nil
}

// getStringField safely gets a string field from a map
func getStringField(m map[string]interface{}, key string) string {
	if val, ok := m[key]; ok {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}
