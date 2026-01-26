package statemgmt

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sSecretModule implements Kubernetes secret management
type K8sSecretModule struct {
	*K8sBaseModule
}

// NewK8sSecretModule creates a new Kubernetes secret module
func NewK8sSecretModule(client k8s.ClientInterface) *K8sSecretModule {
	return &K8sSecretModule{
		K8sBaseModule: NewK8sBaseModule("k8s_secret", []string{"present", "absent"}, client),
	}
}

// Check checks the current state of a Kubernetes secret
func (m *K8sSecretModule) Check(ctx context.Context, decl *StateDeclaration) (*ModuleCheckResult, error) {
	result := &ModuleCheckResult{
		Diff:     make(map[string]interface{}),
		Metadata: make(map[string]interface{}),
	}

	namespace := getNamespace(decl)
	name := decl.ID
	if name == "" {
		return nil, fmt.Errorf("secret name is required")
	}

	// Get secret from cluster
	secret, err := m.client.GetSecret(namespace, name)
	if err != nil {
		// Check if secret doesn't exist (not found error)
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
		return nil, fmt.Errorf("failed to check secret: %w", err)
	}

	result.Present = true
	result.CurrentState = "present"
	result.Metadata["namespace"] = secret.Namespace
	result.Metadata["type"] = secret.Type
	result.Metadata["data_keys"] = getBinaryMapKeys(secret.Data)

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

	// Check type
	desiredType := getSecretType(decl)
	if desiredType != "" && secret.Type != desiredType {
		result.Matches = false
		result.Diff["type"] = map[string]string{
			"current": secret.Type,
			"desired": desiredType,
		}
	}

	// Check data - compare combined data from Data and StringData
	desiredData := getSecretData(decl)
	if desiredData != nil && len(desiredData) > 0 {
		if !compareSecretData(secret.Data, desiredData) {
			result.Matches = false
			result.Diff["data"] = map[string]interface{}{
				"current_keys": getBinaryMapKeys(secret.Data),
				"desired_keys": getBinaryMapKeys(desiredData),
			}
		}
	}

	// Check labels
	desiredLabels := getLabels(decl)
	if desiredLabels != nil && len(desiredLabels) > 0 {
		if !compareLabels(secret.Labels, desiredLabels) {
			result.Matches = false
			result.Diff["labels"] = map[string]interface{}{
				"current": secret.Labels,
				"desired": desiredLabels,
			}
		}
	}

	// Check annotations
	desiredAnnotations := getAnnotations(decl)
	if desiredAnnotations != nil && len(desiredAnnotations) > 0 {
		if !compareAnnotations(secret.Annotations, desiredAnnotations) {
			result.Matches = false
			result.Diff["annotations"] = map[string]interface{}{
				"current": secret.Annotations,
				"desired": desiredAnnotations,
			}
		}
	}

	return result, nil
}

// Apply applies the secret state
func (m *K8sSecretModule) Apply(ctx context.Context, decl *StateDeclaration) (*StateResult, error) {
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
		result.Comment = "Secret name is required"
		result.Duration = time.Since(startTime)
		return result, fmt.Errorf("secret name is required")
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
		result.Comment = fmt.Sprintf("Secret %s/%s is already in desired state '%s'", namespace, name, decl.State)
		result.Duration = time.Since(startTime)
		return result, nil
	}

	// Apply changes based on desired state
	switch decl.State {
	case "present":
		spec := k8s.SecretSpec{
			Name:        name,
			Type:        getSecretType(decl),
			Labels:      getLabels(decl),
			Annotations: getAnnotations(decl),
			Data:        getSecretData(decl),
			StringData:  getSecretStringData(decl),
		}

		if !checkResult.Present {
			// Create secret
			if err := m.client.CreateSecret(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to create secret: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["created"] = true
			result.Changes["type"] = spec.Type
			result.Changes["data_keys"] = getBinaryMapKeys(spec.Data)
			result.Comment = fmt.Sprintf("Created secret %s/%s", namespace, name)
		} else {
			// Update secret
			if err := m.client.UpdateSecret(namespace, spec); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to update secret: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["updated"] = true
			if typeDiff, ok := checkResult.Diff["type"]; ok {
				result.Changes["type_updated"] = typeDiff
			}
			if dataDiff, ok := checkResult.Diff["data"]; ok {
				result.Changes["data_updated"] = dataDiff
			}
			if labelsDiff, ok := checkResult.Diff["labels"]; ok {
				result.Changes["labels_updated"] = labelsDiff
			}
			if annotationsDiff, ok := checkResult.Diff["annotations"]; ok {
				result.Changes["annotations_updated"] = annotationsDiff
			}
			result.Comment = fmt.Sprintf("Updated secret %s/%s", namespace, name)
		}
		result.Success = true

	case "absent":
		if checkResult.Present {
			// Delete secret
			if err := m.client.DeleteSecret(namespace, name); err != nil {
				result.Success = false
				result.Comment = fmt.Sprintf("Failed to delete secret: %v", err)
				result.Duration = time.Since(startTime)
				return result, err
			}
			result.Changes["deleted"] = true
			result.Comment = fmt.Sprintf("Deleted secret %s/%s", namespace, name)
			result.Success = true
		} else {
			result.Success = true
			result.Comment = fmt.Sprintf("Secret %s/%s already absent", namespace, name)
		}

	default:
		result.Success = false
		result.Comment = fmt.Sprintf("Invalid state '%s' for secret", decl.State)
		return result, fmt.Errorf("invalid state: %s", decl.State)
	}

	result.Duration = time.Since(startTime)
	return result, nil
}

// Test tests if the secret is in the desired state
func (m *K8sSecretModule) Test(ctx context.Context, decl *StateDeclaration) (bool, error) {
	result, err := m.Check(ctx, decl)
	if err != nil {
		return false, err
	}
	return result.Matches, nil
}

// getSecretType extracts secret type from state declaration
func getSecretType(decl *StateDeclaration) string {
	typeRaw, ok := decl.Parameters["type"]
	if !ok {
		return "Opaque"
	}
	if str, ok := typeRaw.(string); ok {
		return str
	}
	return "Opaque"
}

// getSecretData extracts secret data (as bytes) from state declaration
// Combines both data and string_data into a single map
func getSecretData(decl *StateDeclaration) map[string][]byte {
	result := make(map[string][]byte)

	// Handle data (already bytes or string to convert)
	if dataRaw, ok := decl.Parameters["data"]; ok {
		if dataMap, ok := dataRaw.(map[string]interface{}); ok {
			for k, v := range dataMap {
				switch val := v.(type) {
				case []byte:
					result[k] = val
				case string:
					result[k] = []byte(val)
				}
			}
		}
	}

	// Handle string_data (convert string to bytes)
	if stringDataRaw, ok := decl.Parameters["string_data"]; ok {
		if stringDataMap, ok := stringDataRaw.(map[string]interface{}); ok {
			for k, v := range stringDataMap {
				if str, ok := v.(string); ok {
					result[k] = []byte(str)
				}
			}
		}
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

// getSecretStringData extracts string_data from state declaration
func getSecretStringData(decl *StateDeclaration) map[string]string {
	dataRaw, ok := decl.Parameters["string_data"]
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

	if len(data) == 0 {
		return nil
	}
	return data
}

// compareSecretData compares two secret data maps
func compareSecretData(current, desired map[string][]byte) bool {
	if len(current) != len(desired) {
		return false
	}
	return reflect.DeepEqual(current, desired)
}
