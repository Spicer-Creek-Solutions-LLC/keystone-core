package statemgmt

import (
	"context"
	"fmt"

	"github.com/shawnbutts/keystone-core/internal/k8s"
)

// K8sBaseModule provides common functionality for Kubernetes state modules
type K8sBaseModule struct {
	*BaseModule
	client k8s.ClientInterface
}

// NewK8sBaseModule creates a new Kubernetes base module
func NewK8sBaseModule(name string, validStates []string, client k8s.ClientInterface) *K8sBaseModule {
	return &K8sBaseModule{
		BaseModule: NewBaseModule(name, validStates),
		client:     client,
	}
}

// GetClient returns the Kubernetes client
func (m *K8sBaseModule) GetClient() k8s.ClientInterface {
	return m.client
}

// getNamespace extracts the namespace from state declaration, defaulting to "default"
func getNamespace(decl *StateDeclaration) string {
	if ns := getStringParameter(decl, "namespace", ""); ns != "" {
		return ns
	}
	return "default"
}

// getLabels extracts labels from state declaration
func getLabels(decl *StateDeclaration) map[string]string {
	if labels, ok := decl.Parameters["labels"].(map[string]interface{}); ok {
		result := make(map[string]string)
		for k, v := range labels {
			if str, ok := v.(string); ok {
				result[k] = str
			}
		}
		return result
	}
	return nil
}

// getAnnotations extracts annotations from state declaration
func getAnnotations(decl *StateDeclaration) map[string]string {
	if annotations, ok := decl.Parameters["annotations"].(map[string]interface{}); ok {
		result := make(map[string]string)
		for k, v := range annotations {
			if str, ok := v.(string); ok {
				result[k] = str
			}
		}
		return result
	}
	return nil
}

// compareLabels compares two label maps
func compareLabels(current, desired map[string]string) bool {
	if len(current) != len(desired) {
		return false
	}
	for k, v := range desired {
		if current[k] != v {
			return false
		}
	}
	return true
}

// compareAnnotations compares two annotation maps
func compareAnnotations(current, desired map[string]string) bool {
	// Only check desired annotations are present
	for k, v := range desired {
		if current[k] != v {
			return false
		}
	}
	return true
}

// K8sModuleConfig holds Kubernetes module configuration
type K8sModuleConfig struct {
	// ClusterConfig is the Kubernetes cluster configuration
	ClusterConfig k8s.ClusterConfig
	// Client is an optional pre-configured client
	Client k8s.ClientInterface
}

// GetK8sClient returns a Kubernetes client from config or creates one
func GetK8sClient(ctx context.Context, config *K8sModuleConfig) (k8s.ClientInterface, error) {
	if config != nil && config.Client != nil {
		return config.Client, nil
	}

	// Create client from config
	clusterConfig := k8s.ClusterConfig{
		Name:       "default",
		Kubeconfig: "",
		Namespace:  "default",
	}
	if config != nil {
		clusterConfig = config.ClusterConfig
	}

	client, err := k8s.NewClient(clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return client, nil
}
