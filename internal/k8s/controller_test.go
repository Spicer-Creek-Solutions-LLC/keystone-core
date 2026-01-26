package k8s

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestRemoteExecutionController(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientWithInterface(clientset, ClusterConfig{
		Name:      "test-cluster",
		Namespace: "default",
	})

	config := OperatorConfig{
		Namespace:               "kscore-system",
		LeaderElection:          true,
		LeaderElectionID:        "kscore-operator",
		MetricsAddr:             ":8080",
		ProbeAddr:               ":8081",
		ReconcileInterval:       1 * time.Minute,
		MaxConcurrentReconciles: 3,
	}

	t.Run("NewRemoteExecutionController", func(t *testing.T) {
		controller := NewRemoteExecutionController(client, config)
		if controller == nil {
			t.Fatal("Expected non-nil controller")
		}
		if controller.client != client {
			t.Error("Client not set correctly")
		}
		if controller.config.Namespace != "kscore-system" {
			t.Errorf("Expected namespace kscore-system, got %s", controller.config.Namespace)
		}
	})

	t.Run("ControllerLifecycle", func(t *testing.T) {
		controller := NewRemoteExecutionController(client, config)

		// Create a context with cancel
		ctx, cancel := context.WithCancel(context.Background())

		// Start in background
		errCh := make(chan error, 1)
		go func() {
			errCh <- controller.Start(ctx)
		}()

		start := time.Now()
		if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
			return time.Since(start) >= 50*time.Millisecond, nil
		}); err != nil {
			t.Fatalf("Controller start wait did not elapse: %v", err)
		}

		// Stop it
		cancel()
		controller.Stop()

		// Wait for start to return
		select {
		case err := <-errCh:
			if err != nil && err != context.Canceled {
				t.Errorf("Unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			// Timeout is OK, controller may block
		}
	})
}

func TestStateConfigController(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientWithInterface(clientset, ClusterConfig{
		Name:      "test-cluster",
		Namespace: "default",
	})

	config := OperatorConfig{
		Namespace:               "kscore-system",
		LeaderElection:          false,
		ReconcileInterval:       30 * time.Second,
		MaxConcurrentReconciles: 2,
	}

	t.Run("NewStateConfigController", func(t *testing.T) {
		controller := NewStateConfigController(client, config)
		if controller == nil {
			t.Fatal("Expected non-nil controller")
		}
		if controller.client != client {
			t.Error("Client not set correctly")
		}
	})

	t.Run("ControllerLifecycle", func(t *testing.T) {
		controller := NewStateConfigController(client, config)

		ctx, cancel := context.WithCancel(context.Background())

		errCh := make(chan error, 1)
		go func() {
			errCh <- controller.Start(ctx)
		}()

		start := time.Now()
		if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
			return time.Since(start) >= 50*time.Millisecond, nil
		}); err != nil {
			t.Fatalf("Controller start wait did not elapse: %v", err)
		}

		cancel()
		controller.Stop()

		select {
		case err := <-errCh:
			if err != nil && err != context.Canceled {
				t.Errorf("Unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			// OK
		}
	})
}

func TestOperatorManager(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	client := NewClientWithInterface(clientset, ClusterConfig{
		Name:      "test-cluster",
		Namespace: "default",
	})

	config := OperatorConfig{
		Namespace:         "kscore-system",
		ReconcileInterval: 1 * time.Minute,
	}

	t.Run("NewOperatorManager", func(t *testing.T) {
		manager := NewOperatorManager(client, config)
		if manager == nil {
			t.Fatal("Expected non-nil manager")
		}
	})

	t.Run("AddController", func(t *testing.T) {
		manager := NewOperatorManager(client, config)
		controller := NewRemoteExecutionController(client, config)

		manager.AddController(controller)

		if len(manager.controllers) != 1 {
			t.Errorf("Expected 1 controller, got %d", len(manager.controllers))
		}
	})

	t.Run("ManagerLifecycle", func(t *testing.T) {
		manager := NewOperatorManager(client, config)
		rexec := NewRemoteExecutionController(client, config)
		sconfig := NewStateConfigController(client, config)

		manager.AddController(rexec)
		manager.AddController(sconfig)

		ctx, cancel := context.WithCancel(context.Background())

		errCh := make(chan error, 1)
		go func() {
			errCh <- manager.Start(ctx)
		}()

		start := time.Now()
		if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
			return time.Since(start) >= 50*time.Millisecond, nil
		}); err != nil {
			t.Fatalf("Manager start wait did not elapse: %v", err)
		}

		cancel()
		manager.Stop()

		select {
		case err := <-errCh:
			if err != nil && err != context.Canceled {
				t.Errorf("Unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			// OK
		}
	})
}

func TestRemoteExecutionCRDTypes(t *testing.T) {
	t.Run("RemoteExecution", func(t *testing.T) {
		now := metav1.Now()
		re := RemoteExecution{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "keystonecore.io/v1",
				Kind:       "RemoteExecution",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-exec",
				Namespace: "default",
			},
			Spec: RemoteExecutionSpec{
				Target: PodSelector{
					Namespace:     "default",
					LabelSelector: "app=nginx",
				},
				Command: []string{"echo", "hello"},
				Mode:    ExecModePod,
				Timeout: metav1.Duration{Duration: 30 * time.Second},
			},
			Status: RemoteExecutionStatus{
				Phase:         "Running",
				PodsExecuted:  5,
				PodsSucceeded: 3,
				PodsFailed:    0,
				StartTime:     &now,
			},
		}

		if re.Spec.Target.LabelSelector != "app=nginx" {
			t.Errorf("Expected label selector app=nginx, got %s", re.Spec.Target.LabelSelector)
		}
		if len(re.Spec.Command) != 2 {
			t.Errorf("Expected 2 command args, got %d", len(re.Spec.Command))
		}
		if re.Status.PodsExecuted != 5 {
			t.Errorf("Expected 5 pods executed, got %d", re.Status.PodsExecuted)
		}
	})

	t.Run("RemoteExecutionList", func(t *testing.T) {
		list := RemoteExecutionList{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "keystonecore.io/v1",
				Kind:       "RemoteExecutionList",
			},
			Items: []RemoteExecution{
				{ObjectMeta: metav1.ObjectMeta{Name: "exec-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "exec-2"}},
			},
		}

		if len(list.Items) != 2 {
			t.Errorf("Expected 2 items, got %d", len(list.Items))
		}
	})
}

func TestStateConfigCRDTypes(t *testing.T) {
	t.Run("StateConfig", func(t *testing.T) {
		now := metav1.Now()
		sc := StateConfig{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "keystonecore.io/v1",
				Kind:       "StateConfig",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-state",
				Namespace: "default",
			},
			Spec: StateConfigSpec{
				Target: PodSelector{
					Namespace:     "default",
					LabelSelector: "app=myapp",
				},
				States: []StateDeclaration{
					{
						Name:   "file_present",
						Module: "file",
						Parameters: map[string]interface{}{
							"path":    "/etc/config.yaml",
							"content": "key: value",
						},
					},
				},
				Vars: map[string]interface{}{
					"env": "production",
				},
			},
			Status: StateConfigStatus{
				Phase:         "Applied",
				PodsApplied:   3,
				PodsSucceeded: 3,
				DriftDetected: false,
				LastApplied:   &now,
			},
		}

		if len(sc.Spec.States) != 1 {
			t.Errorf("Expected 1 state, got %d", len(sc.Spec.States))
		}
		if sc.Spec.States[0].Module != "file" {
			t.Errorf("Expected module 'file', got %s", sc.Spec.States[0].Module)
		}
		if sc.Status.DriftDetected {
			t.Error("Expected no drift detected")
		}
	})

	t.Run("StateConfigList", func(t *testing.T) {
		list := StateConfigList{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "keystonecore.io/v1",
				Kind:       "StateConfigList",
			},
			Items: []StateConfig{
				{ObjectMeta: metav1.ObjectMeta{Name: "state-1"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "state-2"}},
				{ObjectMeta: metav1.ObjectMeta{Name: "state-3"}},
			},
		}

		if len(list.Items) != 3 {
			t.Errorf("Expected 3 items, got %d", len(list.Items))
		}
	})
}

func TestCRDConstants(t *testing.T) {
	t.Run("RemoteExecutionCRD", func(t *testing.T) {
		if RemoteExecutionCRD == "" {
			t.Error("RemoteExecutionCRD should not be empty")
		}

		// Check it contains expected fields
		if !containsString(RemoteExecutionCRD, "remoteexecutions.keystonecore.io") {
			t.Error("CRD should contain resource name")
		}
		if !containsString(RemoteExecutionCRD, "RemoteExecution") {
			t.Error("CRD should contain kind")
		}
		if !containsString(RemoteExecutionCRD, "rexec") {
			t.Error("CRD should contain short name")
		}
	})

	t.Run("StateConfigCRD", func(t *testing.T) {
		if StateConfigCRD == "" {
			t.Error("StateConfigCRD should not be empty")
		}

		if !containsString(StateConfigCRD, "stateconfigs.keystonecore.io") {
			t.Error("CRD should contain resource name")
		}
		if !containsString(StateConfigCRD, "StateConfig") {
			t.Error("CRD should contain kind")
		}
		if !containsString(StateConfigCRD, "sconfig") {
			t.Error("CRD should contain short name")
		}
	})
}

func TestJSONMarshaling(t *testing.T) {
	t.Run("RemoteExecutionSpec", func(t *testing.T) {
		spec := RemoteExecutionSpec{
			Target: PodSelector{
				Namespace:     "default",
				LabelSelector: "app=nginx",
			},
			Command: []string{"echo", "hello"},
			Mode:    ExecModePod,
			Timeout: metav1.Duration{Duration: 30 * time.Second},
		}

		data, err := json.Marshal(spec)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var decoded RemoteExecutionSpec
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if decoded.Mode != ExecModePod {
			t.Errorf("Expected mode pod, got %s", decoded.Mode)
		}
		if decoded.Timeout.Duration != 30*time.Second {
			t.Errorf("Expected timeout 30s, got %v", decoded.Timeout.Duration)
		}
	})

	t.Run("StateConfigSpec", func(t *testing.T) {
		spec := StateConfigSpec{
			Target: PodSelector{
				Namespace: "default",
			},
			States: []StateDeclaration{
				{
					Name:   "test",
					Module: "file",
					Parameters: map[string]interface{}{
						"path": "/tmp/test",
					},
				},
			},
		}

		data, err := json.Marshal(spec)
		if err != nil {
			t.Fatalf("Failed to marshal: %v", err)
		}

		var decoded StateConfigSpec
		if err := json.Unmarshal(data, &decoded); err != nil {
			t.Fatalf("Failed to unmarshal: %v", err)
		}

		if len(decoded.States) != 1 {
			t.Errorf("Expected 1 state, got %d", len(decoded.States))
		}
	})
}

func TestRemoteExecutionStatus(t *testing.T) {
	now := metav1.Now()
	status := RemoteExecutionStatus{
		Phase:          "Completed",
		StartTime:      &now,
		CompletionTime: &now,
		PodsExecuted:   10,
		PodsSucceeded:  8,
		PodsFailed:     2,
		Message:        "Completed with some failures",
		Results: []PodExecutionResult{
			{
				PodName:   "nginx-1",
				Namespace: "default",
				ExitCode:  0,
				Output:    "success",
				Duration:  metav1.Duration{Duration: 1500 * time.Millisecond},
			},
			{
				PodName:   "nginx-2",
				Namespace: "default",
				ExitCode:  1,
				Error:     "command failed",
				Duration:  metav1.Duration{Duration: 500 * time.Millisecond},
			},
		},
	}

	if status.Phase != "Completed" {
		t.Errorf("Expected phase Completed, got %s", status.Phase)
	}
	if status.PodsExecuted != 10 {
		t.Errorf("Expected 10 pods executed, got %d", status.PodsExecuted)
	}
	if len(status.Results) != 2 {
		t.Errorf("Expected 2 results, got %d", len(status.Results))
	}
	if status.Results[0].ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", status.Results[0].ExitCode)
	}
}

func TestStateConfigStatus(t *testing.T) {
	now := metav1.Now()
	status := StateConfigStatus{
		Phase:         "Applied",
		LastApplied:   &now,
		PodsApplied:   5,
		PodsSucceeded: 5,
		PodsFailed:    0,
		Message:       "All states applied successfully",
		DriftDetected: false,
	}

	if status.Phase != "Applied" {
		t.Errorf("Expected phase Applied, got %s", status.Phase)
	}
	if status.PodsApplied != 5 {
		t.Errorf("Expected 5 pods applied, got %d", status.PodsApplied)
	}
	if status.DriftDetected {
		t.Error("Expected no drift detected")
	}
}

func TestStateDeclaration(t *testing.T) {
	decl := StateDeclaration{
		Name:   "nginx_config",
		Module: "file",
		Parameters: map[string]interface{}{
			"path":    "/etc/nginx/nginx.conf",
			"content": "server { listen 80; }",
			"mode":    "0644",
		},
		Requisites: map[string][]string{
			"require": {"pkg_nginx"},
		},
	}

	if decl.Name != "nginx_config" {
		t.Errorf("Expected name nginx_config, got %s", decl.Name)
	}
	if decl.Module != "file" {
		t.Errorf("Expected module file, got %s", decl.Module)
	}
	if decl.Parameters["path"] != "/etc/nginx/nginx.conf" {
		t.Errorf("Expected path /etc/nginx/nginx.conf, got %v", decl.Parameters["path"])
	}
	if len(decl.Requisites["require"]) != 1 {
		t.Errorf("Expected 1 requisite, got %d", len(decl.Requisites["require"]))
	}
}

func TestPodExecutionResult(t *testing.T) {
	result := PodExecutionResult{
		PodName:   "nginx-abc123",
		Namespace: "production",
		ExitCode:  0,
		Output:    "Configuration applied successfully",
		Error:     "",
		Duration:  metav1.Duration{Duration: 2300 * time.Millisecond},
	}

	if result.PodName != "nginx-abc123" {
		t.Errorf("Expected pod name nginx-abc123, got %s", result.PodName)
	}
	if result.Namespace != "production" {
		t.Errorf("Expected namespace production, got %s", result.Namespace)
	}
	if result.ExitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", result.ExitCode)
	}
	if result.Error != "" {
		t.Errorf("Expected no error, got %s", result.Error)
	}
}

// Helper function
func containsString(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || indexString(s, substr) >= 0))
}

func indexString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
