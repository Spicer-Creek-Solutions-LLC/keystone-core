package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sHPAModule implements Kubernetes horizontal pod autoscaler management
type K8sHPAModule struct {
	*K8sBaseModule
}

// NewK8sHPAModule creates a new Kubernetes HPA module
func NewK8sHPAModule(client k8s.ClientInterface) *K8sHPAModule {
	return &K8sHPAModule{
		K8sBaseModule: NewK8sBaseModule("k8s_hpa", []string{"present", "absent"}, client),
	}
}

// Check checks the current state of a Kubernetes HPA
func (m *K8sHPAModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("hpa name is required")
	}

	// Get HPA from cluster
	hpa, err := m.client.GetHPA(namespace, name)
	if err != nil {
		// Check if HPA doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check hpa: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["namespace"] = hpa.Namespace
	result.Metadata["min_replicas"] = hpa.MinReplicas
	result.Metadata["max_replicas"] = hpa.MaxReplicas
	result.Metadata["current_replicas"] = hpa.CurrentReplicas
	result.Metadata["desired_replicas"] = hpa.DesiredReplicas
	result.Metadata["target_kind"] = hpa.TargetKind
	result.Metadata["target_name"] = hpa.TargetName
	if hpa.CurrentCPUUtilization != nil {
		result.Metadata["current_cpu_utilization"] = *hpa.CurrentCPUUtilization
	}
	if hpa.TargetCPUUtilization != nil {
		result.Metadata["target_cpu_utilization"] = *hpa.TargetCPUUtilization
	}
	result.Metadata["status"] = string(hpa.Status)

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

	// Check min replicas
	desiredMinReplicas := getInt32Parameter(decl, "min_replicas", 0)
	if desiredMinReplicas > 0 && hpa.MinReplicas != desiredMinReplicas {
		result.Matches = false
		result.Diff["min_replicas"] = map[string]interface{}{
			"current": hpa.MinReplicas,
			"desired": desiredMinReplicas,
		}
	}

	// Check max replicas
	desiredMaxReplicas := getInt32Parameter(decl, "max_replicas", 0)
	if desiredMaxReplicas > 0 && hpa.MaxReplicas != desiredMaxReplicas {
		result.Matches = false
		result.Diff["max_replicas"] = map[string]interface{}{
			"current": hpa.MaxReplicas,
			"desired": desiredMaxReplicas,
		}
	}

	// Check target CPU utilization
	desiredTargetCPU := getInt32Parameter(decl, "target_cpu_utilization", 0)
	if desiredTargetCPU > 0 {
		if hpa.TargetCPUUtilization == nil || *hpa.TargetCPUUtilization != desiredTargetCPU {
			currentCPU := int32(0)
			if hpa.TargetCPUUtilization != nil {
				currentCPU = *hpa.TargetCPUUtilization
			}
			result.Matches = false
			result.Diff["target_cpu_utilization"] = map[string]interface{}{
				"current": currentCPU,
				"desired": desiredTargetCPU,
			}
		}
	}

	// Check target reference
	desiredTargetKind := getStringParameter(decl, "target_kind", "")
	desiredTargetName := getStringParameter(decl, "target_name", "")
	if desiredTargetKind != "" && hpa.TargetKind != desiredTargetKind {
		result.Matches = false
		result.Diff["target_kind"] = map[string]string{
			"current": hpa.TargetKind,
			"desired": desiredTargetKind,
		}
	}
	if desiredTargetName != "" && hpa.TargetName != desiredTargetName {
		result.Matches = false
		result.Diff["target_name"] = map[string]string{
			"current": hpa.TargetName,
			"desired": desiredTargetName,
		}
	}

	// Check labels
	desiredLabels := getLabels(decl)
	if len(desiredLabels) > 0 {
		if !compareLabels(hpa.Labels, desiredLabels) {
			result.Matches = false
			result.Diff["labels"] = map[string]interface{}{
				"current": hpa.Labels,
				"desired": desiredLabels,
			}
		}
	}

	// Check annotations
	desiredAnnotations := getAnnotations(decl)
	if len(desiredAnnotations) > 0 {
		if !compareAnnotations(hpa.Annotations, desiredAnnotations) {
			result.Matches = false
			result.Diff["annotations"] = map[string]interface{}{
				"current": hpa.Annotations,
				"desired": desiredAnnotations,
			}
		}
	}

	return result, nil
}

// Apply applies the HPA state
func (m *K8sHPAModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Comment = "HPA name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("hpa name is required")
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
		result.Comment = fmt.Sprintf("HPA %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.HPASpec{
			Name:                    name,
			Labels:                  getLabels(decl),
			Annotations:             getAnnotations(decl),
			MinReplicas:             getInt32Parameter(decl, "min_replicas", 1),
			MaxReplicas:             getInt32Parameter(decl, "max_replicas", 10),
			TargetCPUUtilization:    getInt32PointerParameter(decl, "target_cpu_utilization"),
			TargetMemoryUtilization: getInt32PointerParameter(decl, "target_memory_utilization"),
			TargetKind:              getStringParameter(decl, "target_kind", "Deployment"),
			TargetName:              getStringParameter(decl, "target_name", ""),
		}

		if !checkResult.Present {
			// Validate required fields for creation
			if spec.TargetName == "" {
				result.Success = false
				result.Comment = "target_name is required to create an HPA"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("target_name is required to create an HPA")
			}

			// Create HPA
			if err := m.client.CreateHPA(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create HPA: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["min_replicas"] = spec.MinReplicas
			result.Changes["max_replicas"] = spec.MaxReplicas
			result.Changes["target_kind"] = spec.TargetKind
			result.Changes["target_name"] = spec.TargetName
			if spec.TargetCPUUtilization != nil {
				result.Changes["target_cpu_utilization"] = *spec.TargetCPUUtilization
			}
			result.Comment = fmt.Sprintf("Created HPA %s/%s", namespace, name)
		} else {
			// Update HPA
			if err := m.client.UpdateHPA(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update HPA: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if minDiff, ok := checkResult.Diff["min_replicas"]; ok {
				result.Changes["min_replicas_updated"] = minDiff
			}
			if maxDiff, ok := checkResult.Diff["max_replicas"]; ok {
				result.Changes["max_replicas_updated"] = maxDiff
			}
			if cpuDiff, ok := checkResult.Diff["target_cpu_utilization"]; ok {
				result.Changes["target_cpu_utilization_updated"] = cpuDiff
			}
			if labelsDiff, ok := checkResult.Diff["labels"]; ok {
				result.Changes["labels_updated"] = labelsDiff
			}
			if annotationsDiff, ok := checkResult.Diff["annotations"]; ok {
				result.Changes["annotations_updated"] = annotationsDiff
			}
			result.Comment = fmt.Sprintf("Updated HPA %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete HPA
			if err := m.client.DeleteHPA(namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete HPA: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted HPA %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("HPA %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for HPA", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the HPA is in the desired state
func (m *K8sHPAModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// getInt32PointerParameter extracts an optional int32 parameter and returns a pointer
func getInt32PointerParameter(decl *StateDeclaration, name string) *int32 {
	val, ok := decl.Parameters[name]
	if !ok {
		return nil
	}

	switch v := val.(type) {
	case int:
		i := int32(v) //nolint:gosec // G115: k8s HPA params are small ints
		return &i
	case int32:
		return &v
	case int64:
		i := int32(v) //nolint:gosec // G115: k8s HPA params are small ints
		return &i
	case float64:
		i := int32(v) //nolint:gosec // G115: k8s HPA params are small ints
		return &i
	default:
		return nil
	}
}
