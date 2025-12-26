package k8s

import (
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
)

func TestPodStatusToResourceStatus(t *testing.T) {
	tests := []struct {
		name     string
		phase    corev1.PodPhase
		expected ResourceStatus
	}{
		{"Running", corev1.PodRunning, StatusRunning},
		{"Pending", corev1.PodPending, StatusPending},
		{"Succeeded", corev1.PodSucceeded, StatusSucceeded},
		{"Failed", corev1.PodFailed, StatusFailed},
		{"Unknown", corev1.PodUnknown, StatusUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := podStatusToResourceStatus(tt.phase)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestGetTotalRestartCount(t *testing.T) {
	pod := &corev1.Pod{
		Status: corev1.PodStatus{
			ContainerStatuses: []corev1.ContainerStatus{
				{RestartCount: 5},
				{RestartCount: 3},
				{RestartCount: 2},
			},
		},
	}

	expected := int32(10)
	result := getTotalRestartCount(pod)

	if result != expected {
		t.Errorf("expected %d, got %d", expected, result)
	}
}

func TestClusterConfig(t *testing.T) {
	config := ClusterConfig{
		Name:       "test-cluster",
		Kubeconfig: "/path/to/kubeconfig",
		Context:    "test-context",
		Namespace:  "default",
		Timeout:    30 * time.Second,
	}

	if config.Name != "test-cluster" {
		t.Errorf("expected name 'test-cluster', got '%s'", config.Name)
	}

	if config.Timeout != 30*time.Second {
		t.Errorf("expected timeout 30s, got %v", config.Timeout)
	}
}

func TestPodSelector(t *testing.T) {
	selector := PodSelector{
		Namespace:     "default",
		LabelSelector: "app=nginx",
		FieldSelector: "status.phase=Running",
		Container:     "nginx",
		MaxPods:       10,
	}

	if selector.LabelSelector != "app=nginx" {
		t.Errorf("expected label selector 'app=nginx', got '%s'", selector.LabelSelector)
	}

	if selector.MaxPods != 10 {
		t.Errorf("expected max pods 10, got %d", selector.MaxPods)
	}
}

func TestResourceStatus(t *testing.T) {
	statuses := []ResourceStatus{
		StatusRunning,
		StatusPending,
		StatusSucceeded,
		StatusFailed,
		StatusUnknown,
	}

	expected := []string{
		"Running",
		"Pending",
		"Succeeded",
		"Failed",
		"Unknown",
	}

	for i, status := range statuses {
		if string(status) != expected[i] {
			t.Errorf("expected status '%s', got '%s'", expected[i], status)
		}
	}
}

func TestExecutionMode(t *testing.T) {
	modes := []ExecutionMode{
		ExecModePod,
		ExecModeJob,
		ExecModeNode,
	}

	expected := []string{
		"pod",
		"job",
		"node",
	}

	for i, mode := range modes {
		if string(mode) != expected[i] {
			t.Errorf("expected mode '%s', got '%s'", expected[i], mode)
		}
	}
}

func TestOperatorConfig(t *testing.T) {
	config := OperatorConfig{
		Namespace:               "titan-system",
		LeaderElection:          true,
		LeaderElectionID:        "titan-operator",
		MetricsAddr:             ":8080",
		ProbeAddr:               ":8081",
		ReconcileInterval:       1 * time.Minute,
		MaxConcurrentReconciles: 3,
	}

	if config.Namespace != "titan-system" {
		t.Errorf("expected namespace 'titan-system', got '%s'", config.Namespace)
	}

	if !config.LeaderElection {
		t.Error("expected leader election enabled")
	}

	if config.MaxConcurrentReconciles != 3 {
		t.Errorf("expected 3 concurrent reconciles, got %d", config.MaxConcurrentReconciles)
	}
}
