package statemgmt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/k8s"
)

// K8sStatefulSetModule implements Kubernetes statefulset management
type K8sStatefulSetModule struct {
	*K8sBaseModule
}

// NewK8sStatefulSetModule creates a new Kubernetes statefulset module
func NewK8sStatefulSetModule(client k8s.ClientInterface) *K8sStatefulSetModule {
	return &K8sStatefulSetModule{
		K8sBaseModule: NewK8sBaseModule("k8s_statefulset", []string{"present", "absent"}, client),
	}
}

// Check checks the current state of a Kubernetes statefulset
func (m *K8sStatefulSetModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("statefulset name is required")
	}

	// Get statefulset from cluster
	sts, err := m.client.GetStatefulSet(namespace, name)
	if err != nil {
		// Check if statefulset doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check statefulset: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["namespace"] = sts.Namespace
	result.Metadata["replicas"] = sts.Replicas
	result.Metadata["ready_replicas"] = sts.ReadyReplicas
	result.Metadata["current_replicas"] = sts.CurrentReplicas
	result.Metadata["updated_replicas"] = sts.UpdatedReplicas
	result.Metadata["current_revision"] = sts.CurrentRevision
	result.Metadata["update_revision"] = sts.UpdateRevision
	result.Metadata["service_name"] = sts.ServiceName
	result.Metadata["pod_management_policy"] = sts.PodManagementPolicy
	result.Metadata["update_strategy"] = sts.UpdateStrategy
	result.Metadata["status"] = string(sts.Status)

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
	if sts.Replicas != desiredReplicas {
		result.Matches = false
		result.Diff["replicas"] = map[string]interface{}{
			"current": sts.Replicas,
			"desired": desiredReplicas,
		}
	}

	// Check update strategy
	desiredUpdateStrategy := getStringParameter(decl, "update_strategy", "")
	if desiredUpdateStrategy != "" && sts.UpdateStrategy != desiredUpdateStrategy {
		result.Matches = false
		result.Diff["update_strategy"] = map[string]string{
			"current": sts.UpdateStrategy,
			"desired": desiredUpdateStrategy,
		}
	}

	// Check labels
	desiredLabels := getLabels(decl)
	if desiredLabels != nil && len(desiredLabels) > 0 {
		if !compareLabels(sts.Labels, desiredLabels) {
			result.Matches = false
			result.Diff["labels"] = map[string]interface{}{
				"current": sts.Labels,
				"desired": desiredLabels,
			}
		}
	}

	// Check annotations
	desiredAnnotations := getAnnotations(decl)
	if desiredAnnotations != nil && len(desiredAnnotations) > 0 {
		if !compareAnnotations(sts.Annotations, desiredAnnotations) {
			result.Matches = false
			result.Diff["annotations"] = map[string]interface{}{
				"current": sts.Annotations,
				"desired": desiredAnnotations,
			}
		}
	}

	return result, nil
}

// Apply applies the statefulset state
func (m *K8sStatefulSetModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Comment = "StatefulSet name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("statefulset name is required")
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
		result.Comment = fmt.Sprintf("StatefulSet %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.StatefulSetSpec{
			Name:                name,
			Replicas:            getInt32Parameter(decl, "replicas", 1),
			Labels:              getLabels(decl),
			Annotations:         getAnnotations(decl),
			Image:               getStringParameter(decl, "image", ""),
			ContainerPort:       getInt32Parameter(decl, "container_port", 0),
			Selector:            getSelectorLabels(decl),
			ServiceName:         getStringParameter(decl, "service_name", ""),
			PodManagementPolicy: getStringParameter(decl, "pod_management_policy", "OrderedReady"),
			UpdateStrategy:      getStringParameter(decl, "update_strategy", "RollingUpdate"),
			VolumeClaimTemplates: getVolumeClaimTemplates(decl),
		}

		if !checkResult.Present {
			// Validate required fields for creation
			if spec.Image == "" {
				result.Success = false
				result.Comment = "Image is required to create a statefulset"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("image is required to create a statefulset")
			}
			if spec.ServiceName == "" {
				result.Success = false
				result.Comment = "service_name is required to create a statefulset"
				result.Duration = time.Since(startTime)
				return result, fmt.Errorf("service_name is required to create a statefulset")
			}

			// Create statefulset
			if err := m.client.CreateStatefulSet(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create statefulset: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["replicas"] = spec.Replicas
			result.Changes["image"] = spec.Image
			result.Changes["service_name"] = spec.ServiceName
			result.Changes["pod_management_policy"] = spec.PodManagementPolicy
			result.Changes["update_strategy"] = spec.UpdateStrategy
			if len(spec.VolumeClaimTemplates) > 0 {
				result.Changes["volume_claim_templates_count"] = len(spec.VolumeClaimTemplates)
			}
			result.Comment = fmt.Sprintf("Created statefulset %s/%s", namespace, name)
		} else {
			// Update statefulset
			if err := m.client.UpdateStatefulSet(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update statefulset: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if replicasDiff, ok := checkResult.Diff["replicas"]; ok {
				result.Changes["replicas_updated"] = replicasDiff
			}
			if updateStrategyDiff, ok := checkResult.Diff["update_strategy"]; ok {
				result.Changes["update_strategy_updated"] = updateStrategyDiff
			}
			if labelsDiff, ok := checkResult.Diff["labels"]; ok {
				result.Changes["labels_updated"] = labelsDiff
			}
			if annotationsDiff, ok := checkResult.Diff["annotations"]; ok {
				result.Changes["annotations_updated"] = annotationsDiff
			}
			result.Comment = fmt.Sprintf("Updated statefulset %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete statefulset
			if err := m.client.DeleteStatefulSet(namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete statefulset: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted statefulset %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("StatefulSet %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for statefulset", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the statefulset is in the desired state
func (m *K8sStatefulSetModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// getVolumeClaimTemplates extracts volume claim templates from state declaration
func getVolumeClaimTemplates(decl *StateDeclaration) []k8s.VolumeClaimTemplate {
	vctRaw, ok := decl.Parameters["volume_claim_templates"]
	if !ok {
		return nil
	}

	vctSlice, ok := vctRaw.([]interface{})
	if !ok {
		return nil
	}

	templates := make([]k8s.VolumeClaimTemplate, 0, len(vctSlice))
	for _, vctRaw := range vctSlice {
		vctMap, ok := vctRaw.(map[string]interface{})
		if !ok {
			continue
		}

		vct := k8s.VolumeClaimTemplate{}

		if name, ok := vctMap["name"].(string); ok {
			vct.Name = name
		}
		if storageClass, ok := vctMap["storage_class"].(string); ok {
			vct.StorageClassName = storageClass
		}
		if storageSize, ok := vctMap["storage_size"].(string); ok {
			vct.StorageSize = storageSize
		}

		// Parse access modes
		if accessModesRaw, ok := vctMap["access_modes"].([]interface{}); ok {
			accessModes := make([]string, 0, len(accessModesRaw))
			for _, amRaw := range accessModesRaw {
				if am, ok := amRaw.(string); ok {
					accessModes = append(accessModes, am)
				}
			}
			vct.AccessModes = accessModes
		} else if accessMode, ok := vctMap["access_mode"].(string); ok {
			// Support single access_mode as well
			vct.AccessModes = []string{accessMode}
		}

		if vct.Name != "" {
			templates = append(templates, vct)
		}
	}

	if len(templates) == 0 {
		return nil
	}
	return templates
}
