package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sDaemonSetModule implements Kubernetes daemonset management
type K8sDaemonSetModule struct {
	*K8sBaseModule
}

// NewK8sDaemonSetModule creates a new Kubernetes daemonset module
func NewK8sDaemonSetModule(client k8s.ClientInterface) *K8sDaemonSetModule {
	return &K8sDaemonSetModule{
		K8sBaseModule: NewK8sBaseModule("k8s_daemonset", []string{"present", "absent"}, client),
	}
}

// Check checks the current state of a Kubernetes daemonset
func (m *K8sDaemonSetModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("daemonset name is required")
	}

	// Get daemonset from cluster
	ds, err := m.client.GetDaemonSet(ctx, namespace, name)
	if err != nil {
		// Check if daemonset doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check daemonset: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["namespace"] = ds.Namespace
	result.Metadata["desired_number_scheduled"] = ds.DesiredNumberScheduled
	result.Metadata["current_number_scheduled"] = ds.CurrentNumberScheduled
	result.Metadata["number_ready"] = ds.NumberReady
	result.Metadata["number_available"] = ds.NumberAvailable
	result.Metadata["number_misscheduled"] = ds.NumberMisscheduled
	result.Metadata["updated_number_scheduled"] = ds.UpdatedNumberScheduled
	result.Metadata["update_strategy"] = ds.UpdateStrategy
	result.Metadata["status"] = string(ds.Status)

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

	// Check update strategy
	desiredUpdateStrategy := getStringParameter(decl, "update_strategy", "")
	if desiredUpdateStrategy != "" && ds.UpdateStrategy != desiredUpdateStrategy {
		result.Matches = false
		result.Diff["update_strategy"] = map[string]string{
			"current": ds.UpdateStrategy,
			"desired": desiredUpdateStrategy,
		}
	}

	// Check labels
	desiredLabels := getLabels(decl)
	if len(desiredLabels) > 0 {
		if !compareLabels(ds.Labels, desiredLabels) {
			result.Matches = false
			result.Diff["labels"] = map[string]interface{}{
				"current": ds.Labels,
				"desired": desiredLabels,
			}
		}
	}

	// Check annotations
	desiredAnnotations := getAnnotations(decl)
	if len(desiredAnnotations) > 0 {
		if !compareAnnotations(ds.Annotations, desiredAnnotations) {
			result.Matches = false
			result.Diff["annotations"] = map[string]interface{}{
				"current": ds.Annotations,
				"desired": desiredAnnotations,
			}
		}
	}

	return result, nil
}

// Apply applies the daemonset state
func (m *K8sDaemonSetModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Comment = "DaemonSet name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("daemonset name is required")
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
		result.Comment = fmt.Sprintf("DaemonSet %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.DaemonSetSpec{
			Name:           name,
			Labels:         getLabels(decl),
			Annotations:    getAnnotations(decl),
			Image:          getStringParameter(decl, "image", ""),
			ContainerPort:  getInt32Parameter(decl, "container_port", 0),
			Selector:       getSelectorLabels(decl),
			UpdateStrategy: getStringParameter(decl, "update_strategy", "RollingUpdate"),
			NodeSelector:   getNodeSelector(decl),
		}

		if !checkResult.Present {
			// Validate required fields for creation
			if spec.Image == "" {
				result.Success = false
				result.Comment = "Image is required to create a daemonset"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("image is required to create a daemonset")
			}

			// Create daemonset
			if err := m.client.CreateDaemonSet(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create daemonset: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["image"] = spec.Image
			result.Changes["update_strategy"] = spec.UpdateStrategy
			result.Comment = fmt.Sprintf("Created daemonset %s/%s", namespace, name)
		} else {
			// Update daemonset
			if err := m.client.UpdateDaemonSet(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update daemonset: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if updateStrategyDiff, ok := checkResult.Diff["update_strategy"]; ok {
				result.Changes["update_strategy_updated"] = updateStrategyDiff
			}
			if labelsDiff, ok := checkResult.Diff["labels"]; ok {
				result.Changes["labels_updated"] = labelsDiff
			}
			if annotationsDiff, ok := checkResult.Diff["annotations"]; ok {
				result.Changes["annotations_updated"] = annotationsDiff
			}
			result.Comment = fmt.Sprintf("Updated daemonset %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete daemonset
			if err := m.client.DeleteDaemonSet(ctx, namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete daemonset: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted daemonset %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("DaemonSet %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for daemonset", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the daemonset is in the desired state
func (m *K8sDaemonSetModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// getNodeSelector extracts node selector from state declaration
func getNodeSelector(decl *StateDeclaration) map[string]string {
	nsRaw, ok := decl.Parameters["node_selector"]
	if !ok {
		return nil
	}

	nsMap, ok := nsRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	selector := make(map[string]string)
	for k, v := range nsMap {
		if s, ok := v.(string); ok {
			selector[k] = s
		}
	}

	if len(selector) == 0 {
		return nil
	}
	return selector
}
