package statemgmt

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sConfigMapModule implements Kubernetes configmap management
type K8sConfigMapModule struct {
	*K8sBaseModule
}

// NewK8sConfigMapModule creates a new Kubernetes configmap module
func NewK8sConfigMapModule(client k8s.ClientInterface) *K8sConfigMapModule {
	return &K8sConfigMapModule{
		K8sBaseModule: NewK8sBaseModule("k8s_configmap", []string{"present", "absent"}, client),
	}
}

// Check checks the current state of a Kubernetes configmap
func (m *K8sConfigMapModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("configmap name is required")
	}

	// Get configmap from cluster
	cm, err := m.client.GetConfigMap(ctx, namespace, name)
	if err != nil {
		// Check if configmap doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check configmap: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["namespace"] = cm.Namespace
	result.Metadata["data_keys"] = getMapKeys(cm.Data)
	result.Metadata["binary_data_keys"] = getBinaryMapKeys(cm.BinaryData)

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

	// Check data
	desiredData := getConfigMapData(decl)
	if len(desiredData) > 0 {
		if !reflect.DeepEqual(cm.Data, desiredData) {
			result.Matches = false
			result.Diff["data"] = map[string]interface{}{
				"current": cm.Data,
				"desired": desiredData,
			}
		}
	}

	// Check labels
	desiredLabels := getLabels(decl)
	if len(desiredLabels) > 0 {
		if !compareLabels(cm.Labels, desiredLabels) {
			result.Matches = false
			result.Diff["labels"] = map[string]interface{}{
				"current": cm.Labels,
				"desired": desiredLabels,
			}
		}
	}

	// Check annotations
	desiredAnnotations := getAnnotations(decl)
	if len(desiredAnnotations) > 0 {
		if !compareAnnotations(cm.Annotations, desiredAnnotations) {
			result.Matches = false
			result.Diff["annotations"] = map[string]interface{}{
				"current": cm.Annotations,
				"desired": desiredAnnotations,
			}
		}
	}

	return result, nil
}

// Apply applies the configmap state
func (m *K8sConfigMapModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Comment = "ConfigMap name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("configmap name is required")
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
		result.Comment = fmt.Sprintf("ConfigMap %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.ConfigMapSpec{
			Name:        name,
			Labels:      getLabels(decl),
			Annotations: getAnnotations(decl),
			Data:        getConfigMapData(decl),
			BinaryData:  getConfigMapBinaryData(decl),
		}

		if !checkResult.Present {
			// Create configmap
			if err := m.client.CreateConfigMap(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create configmap: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["data_keys"] = getMapKeys(spec.Data)
			result.Comment = fmt.Sprintf("Created configmap %s/%s", namespace, name)
		} else {
			// Update configmap
			if err := m.client.UpdateConfigMap(ctx, namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update configmap: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if dataDiff, ok := checkResult.Diff["data"]; ok {
				result.Changes["data_updated"] = dataDiff
			}
			if labelsDiff, ok := checkResult.Diff["labels"]; ok {
				result.Changes["labels_updated"] = labelsDiff
			}
			if annotationsDiff, ok := checkResult.Diff["annotations"]; ok {
				result.Changes["annotations_updated"] = annotationsDiff
			}
			result.Comment = fmt.Sprintf("Updated configmap %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete configmap
			if err := m.client.DeleteConfigMap(ctx, namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete configmap: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted configmap %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("ConfigMap %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for configmap", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the configmap is in the desired state
func (m *K8sConfigMapModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// getConfigMapData extracts configmap data from state declaration
func getConfigMapData(decl *StateDeclaration) map[string]string {
	dataRaw, ok := decl.Parameters["data"]
	if !ok {
		return nil
	}

	dataMap, ok := dataRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	data := make(map[string]string)
	for k, v := range dataMap {
		if str, ok := v.(string); ok {
			data[k] = str
		}
	}

	return data
}

// getConfigMapBinaryData extracts configmap binary data from state declaration
func getConfigMapBinaryData(decl *StateDeclaration) map[string][]byte {
	dataRaw, ok := decl.Parameters["binary_data"]
	if !ok {
		return nil
	}

	dataMap, ok := dataRaw.(map[string]interface{})
	if !ok {
		return nil
	}

	data := make(map[string][]byte)
	for k, v := range dataMap {
		switch val := v.(type) {
		case []byte:
			data[k] = val
		case string:
			data[k] = []byte(val)
		}
	}

	return data
}

// getMapKeys returns the keys from a string map
func getMapKeys(m map[string]string) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}

// getBinaryMapKeys returns the keys from a binary data map
func getBinaryMapKeys(m map[string][]byte) []string {
	if m == nil {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
