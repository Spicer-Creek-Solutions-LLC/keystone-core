package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sPVCModule implements Kubernetes persistent volume claim management
type K8sPVCModule struct {
	*K8sBaseModule
}

// NewK8sPVCModule creates a new Kubernetes PVC module
func NewK8sPVCModule(client k8s.ClientInterface) *K8sPVCModule {
	return &K8sPVCModule{
		K8sBaseModule: NewK8sBaseModule("k8s_pvc", []string{"present", "absent", "bound"}, client),
	}
}

// Check checks the current state of a Kubernetes PVC
func (m *K8sPVCModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("pvc name is required")
	}

	// Get PVC from cluster
	pvc, err := m.client.GetPVC(ctx, namespace, name)
	if err != nil {
		// Check if PVC doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check pvc: %w", err)
	}

	result.Present = true
	result.Metadata["namespace"] = pvc.Namespace
	result.Metadata["phase"] = pvc.Phase
	result.Metadata["storage_class_name"] = pvc.StorageClassName
	result.Metadata["volume_name"] = pvc.VolumeName
	result.Metadata["access_modes"] = pvc.AccessModes
	result.Metadata["requested_storage"] = pvc.RequestedStorage
	result.Metadata["allocated_storage"] = pvc.AllocatedStorage
	result.Metadata["status"] = string(pvc.Status)

	// Determine current state based on phase
	if pvc.Phase == "Bound" {
		result.CurrentState = "bound"
	} else {
		result.CurrentState = "present"
	}

	// Check if state matches desired
	switch decl.State {
	case "absent":
		result.Matches = false
		result.Diff["state"] = map[string]string{
			"current": result.CurrentState,
			"desired": "absent",
		}
	case "bound":
		result.Matches = (pvc.Phase == "Bound")
		if !result.Matches {
			result.Diff["phase"] = map[string]string{
				"current": pvc.Phase,
				"desired": "Bound",
			}
		}
	case "present":
		result.Matches = true

		// Check storage class
		desiredStorageClass := getStringParameter(decl, "storage_class_name", "")
		if desiredStorageClass != "" && pvc.StorageClassName != desiredStorageClass {
			result.Matches = false
			result.Diff["storage_class_name"] = map[string]string{
				"current": pvc.StorageClassName,
				"desired": desiredStorageClass,
			}
		}

		// Check storage size (can only expand, not shrink)
		desiredStorageSize := getStringParameter(decl, "storage_size", "")
		if desiredStorageSize != "" && pvc.RequestedStorage != desiredStorageSize {
			// Note: We can only detect size difference, not whether it's an expansion
			result.Matches = false
			result.Diff["storage_size"] = map[string]string{
				"current": pvc.RequestedStorage,
				"desired": desiredStorageSize,
			}
		}

		// Check access modes
		desiredAccessModes := getAccessModes(decl)
		if len(desiredAccessModes) > 0 {
			if !compareAccessModes(pvc.AccessModes, desiredAccessModes) {
				result.Matches = false
				result.Diff["access_modes"] = map[string]interface{}{
					"current": pvc.AccessModes,
					"desired": desiredAccessModes,
				}
			}
		}

		// Check labels
		desiredLabels := getLabels(decl)
		if len(desiredLabels) > 0 {
			if !compareLabels(pvc.Labels, desiredLabels) {
				result.Matches = false
				result.Diff["labels"] = map[string]interface{}{
					"current": pvc.Labels,
					"desired": desiredLabels,
				}
			}
		}

		// Check annotations
		desiredAnnotations := getAnnotations(decl)
		if len(desiredAnnotations) > 0 {
			if !compareAnnotations(pvc.Annotations, desiredAnnotations) {
				result.Matches = false
				result.Diff["annotations"] = map[string]interface{}{
					"current": pvc.Annotations,
					"desired": desiredAnnotations,
				}
			}
		}
	default:
		result.Matches = false
	}

	return result, nil
}

// Apply applies the PVC state
func (m *K8sPVCModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Comment = "PVC name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("pvc name is required")
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
		result.Comment = fmt.Sprintf("PVC %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present", "bound":
		spec := k8s.PVCSpec{
			Name:             name,
			Labels:           getLabels(decl),
			Annotations:      getAnnotations(decl),
			StorageClassName: getStringParameter(decl, "storage_class_name", ""),
			StorageSize:      getStringParameter(decl, "storage_size", ""),
			AccessModes:      getAccessModes(decl),
			VolumeMode:       getStringParameter(decl, "volume_mode", "Filesystem"),
			VolumeName:       getStringParameter(decl, "volume_name", ""),
		}

		if !checkResult.Present {
			// Validate required fields for creation
			if spec.StorageSize == "" {
				result.Success = false
				result.Comment = "storage_size is required to create a PVC"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("storage_size is required to create a PVC")
			}
			if len(spec.AccessModes) == 0 {
				spec.AccessModes = []string{"ReadWriteOnce"} // Default access mode
			}

			// Create PVC
			if err := m.client.CreatePVC(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create PVC: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["storage_size"] = spec.StorageSize
			result.Changes["storage_class_name"] = spec.StorageClassName
			result.Changes["access_modes"] = spec.AccessModes
			result.Comment = fmt.Sprintf("Created PVC %s/%s", namespace, name)
		} else {
			// Update PVC (limited - mainly storage expansion and metadata)
			if err := m.client.UpdatePVC(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update PVC: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if storageDiff, ok := checkResult.Diff["storage_size"]; ok {
				result.Changes["storage_size_updated"] = storageDiff
			}
			if labelsDiff, ok := checkResult.Diff["labels"]; ok {
				result.Changes["labels_updated"] = labelsDiff
			}
			if annotationsDiff, ok := checkResult.Diff["annotations"]; ok {
				result.Changes["annotations_updated"] = annotationsDiff
			}
			result.Comment = fmt.Sprintf("Updated PVC %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete PVC
			if err := m.client.DeletePVC(ctx, namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete PVC: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted PVC %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("PVC %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for PVC", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the PVC is in the desired state
func (m *K8sPVCModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// getAccessModes extracts access modes from state declaration
func getAccessModes(decl *StateDeclaration) []string {
	amRaw, ok := decl.Parameters["access_modes"]
	if !ok {
		// Try singular access_mode
		if am, ok := decl.Parameters["access_mode"].(string); ok && am != "" {
			return []string{am}
		}
		return nil
	}

	amSlice, ok := amRaw.([]interface{})
	if !ok {
		// Try single string
		if amStr, ok := amRaw.(string); ok {
			return []string{amStr}
		}
		return nil
	}

	modes := make([]string, 0, len(amSlice))
	for _, a := range amSlice {
		if s, ok := a.(string); ok {
			modes = append(modes, s)
		}
	}
	return modes
}

// compareAccessModes compares two access mode slices
func compareAccessModes(current, desired []string) bool {
	if len(current) != len(desired) {
		return false
	}

	// Create maps for comparison (order doesn't matter)
	currentMap := make(map[string]bool)
	for _, m := range current {
		currentMap[m] = true
	}

	for _, m := range desired {
		if !currentMap[m] {
			return false
		}
	}
	return true
}
