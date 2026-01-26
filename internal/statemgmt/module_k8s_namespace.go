package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sNamespaceModule implements Kubernetes namespace management
type K8sNamespaceModule struct {
	*K8sBaseModule
}

// NewK8sNamespaceModule creates a new Kubernetes namespace module
func NewK8sNamespaceModule(client k8s.ClientInterface) *K8sNamespaceModule {
	return &K8sNamespaceModule{
		K8sBaseModule: NewK8sBaseModule("k8s_namespace", []string{"present", "absent"}, client),
	}
}

// Check checks the current state of a Kubernetes namespace
func (m *K8sNamespaceModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespaceName := decl.ID
	if namespaceName == "" {
		return nil, fmt.Errorf("namespace name is required")
	}

	// Get the namespace from Kubernetes
	nsInfo, err := m.client.GetNamespace(namespaceName)
	if err != nil {
		// Check if namespace doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check namespace: %w", err)
	}

	// Namespace exists
	result.Present = true
	result.CurrentState = "present"
	result.Metadata["name"] = nsInfo.Name
	result.Metadata["creationTimestamp"] = nsInfo.CreationTimestamp
	result.Metadata["phase"] = nsInfo.Phase
	result.Metadata["labels"] = nsInfo.Labels
	result.Metadata["annotations"] = nsInfo.Annotations

	// Check if state matches desired
	result.Matches = (decl.State == "present")

	if !result.Matches {
		result.Diff["state"] = map[string]string{
			"current": "present",
			"desired": decl.State,
		}
		return result, nil
	}

	// Check labels if namespace exists and desired state is present
	desiredLabels := getLabels(decl)
	if desiredLabels != nil && len(desiredLabels) > 0 {
		currentLabels := nsInfo.Labels
		if currentLabels == nil {
			currentLabels = make(map[string]string)
		}

		// Check if all desired labels are present with correct values
		for k, v := range desiredLabels {
			if currentLabels[k] != v {
				result.Matches = false
				result.Diff["labels"] = map[string]interface{}{
					"current": currentLabels,
					"desired": desiredLabels,
				}
				break
			}
		}
	}

	// Check annotations if specified
	desiredAnnotations := getAnnotations(decl)
	if desiredAnnotations != nil && len(desiredAnnotations) > 0 {
		currentAnnotations := nsInfo.Annotations
		if currentAnnotations == nil {
			currentAnnotations = make(map[string]string)
		}

		// Check if all desired annotations are present with correct values
		for k, v := range desiredAnnotations {
			if currentAnnotations[k] != v {
				result.Matches = false
				result.Diff["annotations"] = map[string]interface{}{
					"current": currentAnnotations,
					"desired": desiredAnnotations,
				}
				break
			}
		}
	}

	return result, nil
}

// Apply applies the namespace state
func (m *K8sNamespaceModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
	startTime := time.Now()
	result := &StateResult{
		StateID:   decl.ID,
		Module:    m.Name(),
		StartTime: startTime,
		Changes:   make(map[string]interface{}),
	}

	namespaceName := decl.ID
	if namespaceName == "" {
		result.Success = false
		result.Comment = "Namespace name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("namespace name is required")
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
		result.Comment = fmt.Sprintf("Namespace %s is already in desired state '%s'", namespaceName, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.NamespaceSpec{
			Name:        namespaceName,
			Labels:      getLabels(decl),
			Annotations: getAnnotations(decl),
		}

		if !checkResult.Present {
			// Create namespace
			if err := m.client.CreateNamespace(spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create namespace: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Comment = fmt.Sprintf("Created namespace %s", namespaceName)
		} else {
			// Update namespace (labels, annotations, etc.)
			if err := m.client.UpdateNamespace(spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update namespace: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if checkResult.Diff["labels"] != nil {
				result.Changes["labels_updated"] = checkResult.Diff["labels"]
			}
			if checkResult.Diff["annotations"] != nil {
				result.Changes["annotations_updated"] = checkResult.Diff["annotations"]
			}
			result.Comment = fmt.Sprintf("Updated namespace %s", namespaceName)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete namespace
			if err := m.client.DeleteNamespace(namespaceName); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete namespace: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted namespace %s", namespaceName)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("Namespace %s already absent", namespaceName)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for namespace", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the namespace is in the desired state
func (m *K8sNamespaceModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}
