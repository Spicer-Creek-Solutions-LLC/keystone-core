package statemgmt

import (
	"context"
	"testing"
	"time"

	"github.com/titananvil/titan-anvil/pkg/k8s"
)

// MockK8sClient is a mock Kubernetes client for testing
type MockK8sClient struct{}

func (m *MockK8sClient) ExecInPod(opts k8s.PodExecOptions) (*k8s.PodExecResult, error) {
	return &k8s.PodExecResult{ExitCode: 0}, nil
}

func (m *MockK8sClient) ExecInPods(selector k8s.PodSelector, command []string) ([]k8s.PodExecResult, error) {
	return []k8s.PodExecResult{{ExitCode: 0}}, nil
}

func (m *MockK8sClient) GetPod(namespace, name string) (*k8s.ResourceInfo, error) {
	return &k8s.ResourceInfo{
		Kind:      "Pod",
		Namespace: namespace,
		Name:      name,
		Status:    k8s.StatusRunning,
	}, nil
}

func (m *MockK8sClient) ListPods(selector k8s.PodSelector) ([]k8s.ResourceInfo, error) {
	return []k8s.ResourceInfo{
		{
			Kind:      "Pod",
			Namespace: "default",
			Name:      "test-pod",
			Status:    k8s.StatusRunning,
		},
	}, nil
}

func (m *MockK8sClient) GetDeployment(namespace, name string) (*k8s.DeploymentInfo, error) {
	return &k8s.DeploymentInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:              "Deployment",
			Namespace:         namespace,
			Name:              name,
			Labels:            map[string]string{"app": "test"},
			Annotations:       map[string]string{},
			Status:            k8s.StatusRunning,
			CreationTimestamp: time.Now(),
		},
		Replicas:          3,
		AvailableReplicas: 3,
		ReadyReplicas:     3,
		UpdatedReplicas:   3,
	}, nil
}

func (m *MockK8sClient) GetService(namespace, name string) (*k8s.ServiceInfo, error) {
	return &k8s.ServiceInfo{
		ResourceInfo: k8s.ResourceInfo{
			Kind:      "Service",
			Namespace: namespace,
			Name:      name,
			Status:    k8s.StatusRunning,
		},
		Type:      "ClusterIP",
		ClusterIP: "10.0.0.1",
	}, nil
}

func (m *MockK8sClient) WatchPods(selector k8s.PodSelector) (<-chan k8s.WatchEvent, error) {
	ch := make(chan k8s.WatchEvent)
	close(ch)
	return ch, nil
}

func (m *MockK8sClient) CreateResource(namespace string, manifest []byte) error {
	return nil
}

func (m *MockK8sClient) UpdateResource(namespace string, manifest []byte) error {
	return nil
}

func (m *MockK8sClient) DeleteResource(namespace, kind, name string) error {
	return nil
}

func (m *MockK8sClient) GetClusterInfo() (*k8s.ClusterInfo, error) {
	return &k8s.ClusterInfo{
		Version:    "v1.26.0",
		Nodes:      3,
		Namespaces: 5,
		APIServer:  "https://127.0.0.1:6443",
	}, nil
}

func TestK8sNamespaceModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	if module.Name() != "k8s_namespace" {
		t.Errorf("expected module name 'k8s_namespace', got '%s'", module.Name())
	}

	validStates := module.ValidStates()
	if len(validStates) != 2 {
		t.Errorf("expected 2 valid states, got %d", len(validStates))
	}
}

func TestK8sNamespaceCheck(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	// Test checking default namespace (exists)
	decl := &StateDeclaration{
		ID:         "default",
		State:      "present",
		Module:     "k8s_namespace",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("expected default namespace to be present")
	}

	if result.CurrentState != "present" {
		t.Errorf("expected current state 'present', got '%s'", result.CurrentState)
	}

	if !result.Matches {
		t.Error("expected state to match")
	}
}

func TestK8sNamespaceApply(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sNamespaceModule(client)

	decl := &StateDeclaration{
		ID:         "default",
		State:      "present",
		Module:     "k8s_namespace",
		Parameters: map[string]interface{}{},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
}

func TestK8sDeploymentModule(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	if module.Name() != "k8s_deployment" {
		t.Errorf("expected module name 'k8s_deployment', got '%s'", module.Name())
	}

	validStates := module.ValidStates()
	if len(validStates) != 2 {
		t.Errorf("expected 2 valid states, got %d", len(validStates))
	}
}

func TestK8sDeploymentCheck(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	decl := &StateDeclaration{
		ID:     "test-deployment",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(3),
			"labels": map[string]interface{}{
				"app": "test",
			},
		},
	}

	result, err := module.Check(context.Background(), decl)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !result.Present {
		t.Error("expected deployment to be present")
	}

	if result.CurrentState != "present" {
		t.Errorf("expected current state 'present', got '%s'", result.CurrentState)
	}
}

func TestK8sDeploymentApply(t *testing.T) {
	client := &MockK8sClient{}
	module := NewK8sDeploymentModule(client)

	decl := &StateDeclaration{
		ID:     "test-deployment",
		State:  "present",
		Module: "k8s_deployment",
		Parameters: map[string]interface{}{
			"namespace": "default",
			"replicas":  int32(3),
		},
	}

	result, err := module.Apply(context.Background(), decl)
	if err != nil {
		t.Fatalf("Apply failed: %v", err)
	}

	if !result.Success {
		t.Errorf("expected success, got failure: %s", result.Comment)
	}
}

func TestGetInt32Parameter(t *testing.T) {
	tests := []struct {
		name     string
		value    interface{}
		expected int32
	}{
		{"int", int(42), 42},
		{"int32", int32(42), 42},
		{"int64", int64(42), 42},
		{"float64", float64(42), 42},
		{"missing", nil, 10}, // Uses default value
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			decl := &StateDeclaration{
				Parameters: map[string]interface{}{},
			}
			if tt.value != nil {
				decl.Parameters["test"] = tt.value
			}

			result := getInt32Parameter(decl, "test", 10)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestGetNamespace(t *testing.T) {
	tests := []struct {
		name     string
		decl     *StateDeclaration
		expected string
	}{
		{
			name: "explicit namespace",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{
					"namespace": "custom",
				},
			},
			expected: "custom",
		},
		{
			name: "default namespace",
			decl: &StateDeclaration{
				Parameters: map[string]interface{}{},
			},
			expected: "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getNamespace(tt.decl)
			if result != tt.expected {
				t.Errorf("expected '%s', got '%s'", tt.expected, result)
			}
		})
	}
}

func TestCompareLabels(t *testing.T) {
	tests := []struct {
		name     string
		current  map[string]string
		desired  map[string]string
		expected bool
	}{
		{
			name:     "identical labels",
			current:  map[string]string{"app": "test", "version": "1.0"},
			desired:  map[string]string{"app": "test", "version": "1.0"},
			expected: true,
		},
		{
			name:     "different labels",
			current:  map[string]string{"app": "test"},
			desired:  map[string]string{"app": "test", "version": "1.0"},
			expected: false,
		},
		{
			name:     "empty labels",
			current:  map[string]string{},
			desired:  map[string]string{},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareLabels(tt.current, tt.desired)
			if result != tt.expected {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}
