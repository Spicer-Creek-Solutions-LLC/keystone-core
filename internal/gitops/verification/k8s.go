package verification

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

// K8sVerifier checks Kubernetes resources
type K8sVerifier struct {
	client dynamic.Interface
}

// NewK8sVerifier creates a new Kubernetes verifier
func NewK8sVerifier(kubeconfig string) (*K8sVerifier, error) {
	// Build kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loadingRules.ExplicitPath = kubeconfig
	}

	configOverrides := &clientcmd.ConfigOverrides{}
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

	return &K8sVerifier{
		client: dynamicClient,
	}, nil
}

// Type returns the verification type
func (v *K8sVerifier) Type() Type {
	return TypeK8s
}

// Verify checks a Kubernetes resource
func (v *K8sVerifier) Verify(step *Step) (*Result, error) {
	result := &Result{
		StepName:  step.Name,
		Timestamp: time.Now(),
		Data:      make(map[string]interface{}),
	}

	// Extract configuration
	resource, ok := step.Config["resource"].(string)
	if !ok || resource == "" {
		result.Success = false
		result.Message = "Missing 'resource' in config (e.g., 'deployment/myapp')"
		return result, fmt.Errorf("missing resource")
	}

	namespace := "default"
	if ns, ok := step.Config["namespace"].(string); ok {
		namespace = ns
	}

	condition, ok := step.Config["condition"].(string)
	if !ok {
		condition = "available"
	}

	// Parse resource (kind/name)
	gvr, name, err := parseResource(resource)
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Invalid resource format: %v", err)
		return result, err
	}

	// Get resource
	ctx := context.Background()
	obj, err := v.client.Resource(gvr).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		result.Success = false
		result.Message = fmt.Sprintf("Failed to get resource: %v", err)
		return result, err
	}

	// Check condition
	success, message := checkK8sCondition(obj, condition, step.Config)
	result.Success = success
	result.Message = message

	if success {
		result.Data["resource"] = fmt.Sprintf("%s/%s", namespace, name)
		result.Data["kind"] = gvr.Resource
	}

	return result, nil
}

// parseResource parses a resource string like "deployment/myapp" into GVR and name
func parseResource(resource string) (schema.GroupVersionResource, string, error) {
	parts := splitResource(resource)
	if len(parts) != 2 {
		return schema.GroupVersionResource{}, "", fmt.Errorf("invalid resource format, expected 'kind/name'")
	}

	kind := parts[0]
	name := parts[1]

	// Map common kinds to GVR
	gvr := kindToGVR(kind)
	if gvr.Resource == "" {
		return schema.GroupVersionResource{}, "", fmt.Errorf("unknown resource kind: %s", kind)
	}

	return gvr, name, nil
}

func splitResource(s string) []string {
	result := make([]string, 0, 2)
	current := ""
	for _, ch := range s {
		if ch == '/' {
			if current != "" {
				result = append(result, current)
				current = ""
			}
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		result = append(result, current)
	}
	return result
}

// kindToGVR maps Kubernetes resource kinds to GroupVersionResource
func kindToGVR(kind string) schema.GroupVersionResource {
	mapping := map[string]schema.GroupVersionResource{
		"deployment": {
			Group:    "apps",
			Version:  "v1",
			Resource: "deployments",
		},
		"statefulset": {
			Group:    "apps",
			Version:  "v1",
			Resource: "statefulsets",
		},
		"daemonset": {
			Group:    "apps",
			Version:  "v1",
			Resource: "daemonsets",
		},
		"service": {
			Group:    "",
			Version:  "v1",
			Resource: "services",
		},
		"pod": {
			Group:    "",
			Version:  "v1",
			Resource: "pods",
		},
		"replicaset": {
			Group:    "apps",
			Version:  "v1",
			Resource: "replicasets",
		},
	}

	if gvr, ok := mapping[kind]; ok {
		return gvr
	}

	return schema.GroupVersionResource{}
}

// checkK8sCondition checks if a resource meets the specified condition
func checkK8sCondition(obj *unstructured.Unstructured, condition string, config map[string]interface{}) (met bool, message string) {
	switch condition {
	case "available":
		return checkAvailableCondition(obj, config)
	case "ready":
		return checkReadyCondition(obj, config)
	case "exists":
		return true, "Resource exists"
	default:
		return false, fmt.Sprintf("Unknown condition: %s", condition)
	}
}

// checkAvailableCondition checks if a deployment/statefulset is available
func checkAvailableCondition(obj *unstructured.Unstructured, config map[string]interface{}) (available bool, message string) {
	// Check replicas for deployments/statefulsets
	status, found, err := unstructured.NestedMap(obj.Object, "status")
	if err != nil || !found {
		return false, "No status found"
	}

	availableReplicas, found, err := unstructured.NestedInt64(status, "availableReplicas")
	if err != nil || !found {
		availableReplicas = 0
	}

	replicas, found, err := unstructured.NestedInt64(status, "replicas")
	if err != nil || !found {
		replicas = 0
	}

	// Check if expected replicas are specified in config
	expectedReplicas := replicas
	switch r := config["replicas"].(type) {
	case int:
		expectedReplicas = int64(r)
	case float64:
		expectedReplicas = int64(r)
	}

	if availableReplicas >= expectedReplicas && expectedReplicas > 0 {
		return true, fmt.Sprintf("%d/%d replicas available", availableReplicas, expectedReplicas)
	}

	return false, fmt.Sprintf("Only %d/%d replicas available", availableReplicas, expectedReplicas)
}

// checkReadyCondition checks if a resource is ready
func checkReadyCondition(obj *unstructured.Unstructured, config map[string]interface{}) (ready bool, message string) {
	conditions, found, err := unstructured.NestedSlice(obj.Object, "status", "conditions")
	if err != nil || !found {
		return false, "No conditions found"
	}

	for _, cond := range conditions {
		condMap, ok := cond.(map[string]interface{})
		if !ok {
			continue
		}

		condType, _, _ := unstructured.NestedString(condMap, "type")
		status, _, _ := unstructured.NestedString(condMap, "status")

		if condType == "Ready" && status == "True" {
			return true, "Resource is ready"
		}
	}

	return false, "Resource is not ready"
}
