package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sDeploymentModule implements Kubernetes deployment management
type K8sDeploymentModule struct {
	*K8sBaseModule
}

// NewK8sDeploymentModule creates a new Kubernetes deployment module
func NewK8sDeploymentModule(client k8s.ClientInterface) *K8sDeploymentModule {
	return &K8sDeploymentModule{
		K8sBaseModule: NewK8sBaseModule("k8s_deployment", []string{"present", "absent"}, client),
	}
}

// Check checks the current state of a Kubernetes deployment
func (m *K8sDeploymentModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("deployment name is required")
	}

	// Get deployment from cluster
	deployment, err := m.client.GetDeployment(namespace, name)
	if err != nil {
		// Check if deployment doesn't exist (not found error)
		if strings.Contains(err.Error(), "not found") {
			result.Present = false
			result.CurrentState = "absent"
			result.Matches = (decl.State == "absent")
			if !result.Matches {
				result.Diff["state"] = map[string]string{
					"current": "absent",
					"desired": decl.State,
				}
			}
			return result, nil
		}
		return nil, fmt.Errorf("failed to check deployment: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["namespace"] = deployment.Namespace
	result.Metadata["replicas"] = deployment.Replicas
	result.Metadata["availableReplicas"] = deployment.AvailableReplicas
	result.Metadata["readyReplicas"] = deployment.ReadyReplicas
	result.Metadata["updatedReplicas"] = deployment.UpdatedReplicas
	result.Metadata["status"] = string(deployment.Status)

	// Check if state matches desired
	if decl.State == "absent" {
		result.Matches = false
		result.Diff["state"] = map[string]string{
			"current": "present",
			"desired": "absent",
		}
		return result, nil
	}

	// For "present" state, check additional properties
	result.Matches = true

	// Check replicas
	desiredReplicas := getInt32Parameter(decl, "replicas", 1)
	if deployment.Replicas != desiredReplicas {
		result.Matches = false
		result.Diff["replicas"] = map[string]interface{}{
			"current": deployment.Replicas,
			"desired": desiredReplicas,
		}
	}

	// Check labels
	desiredLabels := getLabels(decl)
	if len(desiredLabels) > 0 {
		if !compareLabels(deployment.Labels, desiredLabels) {
			result.Matches = false
			result.Diff["labels"] = map[string]interface{}{
				"current": deployment.Labels,
				"desired": desiredLabels,
			}
		}
	}

	// Check annotations
	desiredAnnotations := getAnnotations(decl)
	if len(desiredAnnotations) > 0 {
		if !compareAnnotations(deployment.Annotations, desiredAnnotations) {
			result.Matches = false
			result.Diff["annotations"] = map[string]interface{}{
				"current": deployment.Annotations,
				"desired": desiredAnnotations,
			}
		}
	}

	return result, nil
}

// Apply applies the deployment state
func (m *K8sDeploymentModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		StartTime: startTime,
		Changes:   make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		result.Success = false
		result.Comment = "Deployment name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("deployment name is required")
	}

	// Check current state
	checkResult, err := m.Check(ctx, decl)
	if err != nil {
		result.Success = false
		result.Comment = fmt.Sprintf("Failed to check current state: %v", err)
		result.Duration = time.Since(startTime)
		return result, err
	}

	// If already in desired state, nothing to do
	if checkResult.Matches {
		result.Success = true
		result.Comment = fmt.Sprintf("Deployment %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.DeploymentSpec{
			Name:          name,
			Replicas:      getInt32Parameter(decl, "replicas", 1),
			Labels:        getLabels(decl),
			Annotations:   getAnnotations(decl),
			Image:         getStringParameter(decl, "image", ""),
			ContainerPort: getInt32Parameter(decl, "container_port", 0),
			Selector:      getSelectorLabels(decl),
		}

		if !checkResult.Present {
			// Validate required fields for creation
			if spec.Image == "" {
				result.Success = false
				result.Comment = "Image is required to create a deployment"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("image is required to create a deployment")
			}

			// Create deployment
			if err := m.client.CreateDeployment(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create deployment: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["replicas"] = spec.Replicas
			result.Changes["image"] = spec.Image
			result.Comment = fmt.Sprintf("Created deployment %s/%s", namespace, name)
		} else {
			// Update deployment
			if err := m.client.UpdateDeployment(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update deployment: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if replicasDiff, ok := checkResult.Diff["replicas"]; ok {
				result.Changes["replicas_updated"] = replicasDiff
			}
			if labelsDiff, ok := checkResult.Diff["labels"]; ok {
				result.Changes["labels_updated"] = labelsDiff
			}
			if annotationsDiff, ok := checkResult.Diff["annotations"]; ok {
				result.Changes["annotations_updated"] = annotationsDiff
			}
			result.Comment = fmt.Sprintf("Updated deployment %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete deployment
			if err := m.client.DeleteDeployment(namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete deployment: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted deployment %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("Deployment %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for deployment", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the deployment is in the desired state
func (m *K8sDeploymentModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// Helper function to get int32 parameter
func getInt32Parameter(decl *StateDeclaration, key string, defaultValue int32) int32 {
	if val, ok := decl.Parameters[key]; ok {
		switch v := val.(type) {
		case int:
			return int32(v) //nolint:gosec // G115: k8s params are small ints
		case int32:
			return v
		case int64:
			return int32(v) //nolint:gosec // G115: k8s params are small ints
		case float64:
			return int32(v) //nolint:gosec // G115: k8s params are small ints
		}
	}
	return defaultValue
}

// getSelectorLabels extracts selector labels from state declaration
func getSelectorLabels(decl *StateDeclaration) map[string]string {
	if selector, ok := decl.Parameters["selector"].(map[string]interface{}); ok {
		result := make(map[string]string)
		for k, v := range selector {
			if str, ok := v.(string); ok {
				result[k] = str
			}
		}
		return result
	}
	return nil
}
