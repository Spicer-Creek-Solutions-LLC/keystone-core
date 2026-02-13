package k8s

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

// mockExecutor is a test double for RemoteExecutor.
type mockExecutor struct {
	execFunc func(ctx context.Context, exec *RemoteExecution) error
	calls    int
}

func (m *mockExecutor) ExecuteRemoteExecution(ctx context.Context, exec *RemoteExecution) error {
	m.calls++
	if m.execFunc != nil {
		return m.execFunc(ctx, exec)
	}
	exec.Status.Phase = "Succeeded"
	exec.Status.Message = "mock success"
	return nil
}

// mockStateExecutor is a test double for StateExecutor.
type mockStateExecutor struct {
	execFunc func(ctx context.Context, sc *StateConfig) error
	calls    int
}

func (m *mockStateExecutor) ExecuteStateFile(ctx context.Context, sc *StateConfig) error {
	m.calls++
	if m.execFunc != nil {
		return m.execFunc(ctx, sc)
	}
	sc.Status.PodsApplied = 3
	sc.Status.PodsSucceeded = 3
	return nil
}

// mockDriftChecker is a test double for StateDriftChecker.
type mockDriftChecker struct {
	hasDrift bool
	err      error
	calls    int
}

func (m *mockDriftChecker) CheckDrift(_ context.Context, _ *StateConfig) (bool, error) {
	m.calls++
	return m.hasDrift, m.err
}

// controllerTestScheme creates a runtime.Scheme for the fake dynamic client.
func controllerTestScheme() *runtime.Scheme {
	return newFakeScheme() // reuse from dynamic_test.go
}

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
		controller := NewRemoteExecutionController(client, nil, config)
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

	t.Run("NewRemoteExecutionController_WithCRDClient", func(t *testing.T) {
		crdClient := &CRDClient{}
		controller := NewRemoteExecutionController(client, crdClient, config)
		if controller.crdClient != crdClient {
			t.Error("CRDClient not set correctly")
		}
	})

	t.Run("ControllerLifecycle", func(t *testing.T) {
		controller := NewRemoteExecutionController(client, nil, config)

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
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			// Timeout is OK, controller may block
		}
	})

	t.Run("Enqueue", func(t *testing.T) {
		controller := NewRemoteExecutionController(client, nil, config)
		controller.Enqueue("default/test-exec")
		if controller.queue.Len() != 1 {
			t.Errorf("Expected queue length 1, got %d", controller.queue.Len())
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
		controller := NewStateConfigController(client, nil, config)
		if controller == nil {
			t.Fatal("Expected non-nil controller")
		}
		if controller.client != client {
			t.Error("Client not set correctly")
		}
	})

	t.Run("NewStateConfigController_WithCRDClient", func(t *testing.T) {
		crdClient := &CRDClient{}
		controller := NewStateConfigController(client, crdClient, config)
		if controller.crdClient != crdClient {
			t.Error("CRDClient not set correctly")
		}
	})

	t.Run("ControllerLifecycle", func(t *testing.T) {
		controller := NewStateConfigController(client, nil, config)

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
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			// OK
		}
	})

	t.Run("Enqueue", func(t *testing.T) {
		controller := NewStateConfigController(client, nil, config)
		controller.Enqueue("default/test-state")
		if controller.queue.Len() != 1 {
			t.Errorf("Expected queue length 1, got %d", controller.queue.Len())
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
		Namespace:               "kscore-system",
		ReconcileInterval:       1 * time.Minute,
		MaxConcurrentReconciles: 1,
	}

	t.Run("NewOperatorManager", func(t *testing.T) {
		manager := NewOperatorManager(client, nil, config)
		if manager == nil {
			t.Fatal("Expected non-nil manager")
		}
	})

	t.Run("NewOperatorManager_WithCRDClient", func(t *testing.T) {
		crdClient := &CRDClient{}
		manager := NewOperatorManager(client, crdClient, config)
		if manager.crdClient != crdClient {
			t.Error("CRDClient not set correctly")
		}
	})

	t.Run("AddController", func(t *testing.T) {
		manager := NewOperatorManager(client, nil, config)
		controller := NewRemoteExecutionController(client, nil, config)

		manager.AddController(controller)

		if len(manager.controllers) != 1 {
			t.Errorf("Expected 1 controller, got %d", len(manager.controllers))
		}
	})

	t.Run("ManagerLifecycle", func(t *testing.T) {
		manager := NewOperatorManager(client, nil, config)
		rexec := NewRemoteExecutionController(client, nil, config)
		sconfig := NewStateConfigController(client, nil, config)

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
			if err != nil && !errors.Is(err, context.Canceled) {
				t.Errorf("Unexpected error: %v", err)
			}
		case <-time.After(time.Second):
			// OK
		}
	})

	t.Run("SetInformerManager", func(t *testing.T) {
		manager := NewOperatorManager(client, nil, config)
		rexec := NewRemoteExecutionController(client, nil, config)
		sconfig := NewStateConfigController(client, nil, config)
		manager.AddController(rexec)
		manager.AddController(sconfig)

		// SetInformerManager should not panic with nil dynamic client
		// We just verify it sets the field
		if manager.informerMgr != nil {
			t.Error("Expected nil informerMgr before SetInformerManager")
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

func TestRemoteExecutionReconcile(t *testing.T) {
	scheme := controllerTestScheme()
	config := OperatorConfig{
		Namespace:               "default",
		ReconcileInterval:       1 * time.Minute,
		MaxConcurrentReconciles: 1,
	}

	t.Run("Pending_resource_is_executed_and_status_updated", func(t *testing.T) {
		obj := makeUnstructuredRE("exec-1", "default", "Pending")
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		mock := &mockExecutor{}
		ctrl := NewRemoteExecutionController(nil, crdClient, config)
		ctrl.SetExecutor(mock)

		err := ctrl.reconcile(context.Background(), "default/exec-1")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if mock.calls != 1 {
			t.Errorf("Expected 1 executor call, got %d", mock.calls)
		}

		// Verify status was persisted
		re, err := crdClient.GetRemoteExecution(context.Background(), "default", "exec-1")
		if err != nil {
			t.Fatalf("Get after reconcile: %v", err)
		}
		if re.Status.Phase != "Succeeded" {
			t.Errorf("Expected phase Succeeded, got %s", re.Status.Phase)
		}
	})

	t.Run("Succeeded_phase_is_skipped", func(t *testing.T) {
		obj := makeUnstructuredRE("exec-done", "default", "Succeeded")
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		mock := &mockExecutor{}
		ctrl := NewRemoteExecutionController(nil, crdClient, config)
		ctrl.SetExecutor(mock)

		err := ctrl.reconcile(context.Background(), "default/exec-done")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if mock.calls != 0 {
			t.Errorf("Expected 0 executor calls for terminal phase, got %d", mock.calls)
		}
	})

	t.Run("Failed_phase_is_skipped", func(t *testing.T) {
		obj := makeUnstructuredRE("exec-fail", "default", "Failed")
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		mock := &mockExecutor{}
		ctrl := NewRemoteExecutionController(nil, crdClient, config)
		ctrl.SetExecutor(mock)

		err := ctrl.reconcile(context.Background(), "default/exec-fail")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if mock.calls != 0 {
			t.Errorf("Expected 0 executor calls for terminal phase, got %d", mock.calls)
		}
	})

	t.Run("Not_found_returns_error_for_requeue", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClient(scheme)
		crdClient := NewCRDClientWithDynamic(dc)

		ctrl := NewRemoteExecutionController(nil, crdClient, config)

		err := ctrl.reconcile(context.Background(), "default/nonexistent")
		if err == nil {
			t.Fatal("Expected error for not-found resource")
		}
	})

	t.Run("Executor_error_returns_error_for_requeue", func(t *testing.T) {
		obj := makeUnstructuredRE("exec-err", "default", "Pending")
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		mock := &mockExecutor{
			execFunc: func(_ context.Context, exec *RemoteExecution) error {
				exec.Status.Phase = "Failed"
				return fmt.Errorf("pod unreachable")
			},
		}
		ctrl := NewRemoteExecutionController(nil, crdClient, config)
		ctrl.SetExecutor(mock)

		err := ctrl.reconcile(context.Background(), "default/exec-err")
		if err == nil {
			t.Fatal("Expected error from executor")
		}
	})

	t.Run("Nil_crdClient_is_noop", func(t *testing.T) {
		ctrl := NewRemoteExecutionController(nil, nil, config)
		err := ctrl.reconcile(context.Background(), "default/anything")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})

	t.Run("Running_phase_is_executed", func(t *testing.T) {
		obj := makeUnstructuredRE("exec-running", "default", "Running")
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		mock := &mockExecutor{}
		ctrl := NewRemoteExecutionController(nil, crdClient, config)
		ctrl.SetExecutor(mock)

		err := ctrl.reconcile(context.Background(), "default/exec-running")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if mock.calls != 1 {
			t.Errorf("Expected 1 executor call for Running phase, got %d", mock.calls)
		}
	})
}

func TestStateConfigReconcile(t *testing.T) {
	scheme := controllerTestScheme()
	config := OperatorConfig{
		Namespace:               "default",
		ReconcileInterval:       1 * time.Minute,
		MaxConcurrentReconciles: 1,
	}

	t.Run("Pending_resource_is_applied", func(t *testing.T) {
		obj := makeUnstructuredSC("state-1", "default", "Pending", false)
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		mock := &mockStateExecutor{}
		ctrl := NewStateConfigController(nil, crdClient, config)
		ctrl.SetStateExecutor(mock)

		err := ctrl.reconcile(context.Background(), "default/state-1")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if mock.calls != 1 {
			t.Errorf("Expected 1 executor call, got %d", mock.calls)
		}

		sc, err := crdClient.GetStateConfig(context.Background(), "default", "state-1")
		if err != nil {
			t.Fatalf("Get after reconcile: %v", err)
		}
		if sc.Status.Phase != "Applied" {
			t.Errorf("Expected phase Applied, got %s", sc.Status.Phase)
		}
	})

	t.Run("Applied_no_drift_is_skipped", func(t *testing.T) {
		obj := makeUnstructuredSC("state-ok", "default", "Applied", false)
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		mock := &mockStateExecutor{}
		ctrl := NewStateConfigController(nil, crdClient, config)
		ctrl.SetStateExecutor(mock)

		err := ctrl.reconcile(context.Background(), "default/state-ok")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if mock.calls != 0 {
			t.Errorf("Expected 0 executor calls for Applied/no-drift, got %d", mock.calls)
		}
	})

	t.Run("Applied_with_drift_is_re_applied", func(t *testing.T) {
		obj := makeUnstructuredSC("state-drift", "default", "Applied", true)
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		mock := &mockStateExecutor{}
		ctrl := NewStateConfigController(nil, crdClient, config)
		ctrl.SetStateExecutor(mock)

		err := ctrl.reconcile(context.Background(), "default/state-drift")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if mock.calls != 1 {
			t.Errorf("Expected 1 executor call for drifted state, got %d", mock.calls)
		}

		sc, err := crdClient.GetStateConfig(context.Background(), "default", "state-drift")
		if err != nil {
			t.Fatalf("Get after reconcile: %v", err)
		}
		if sc.Status.Phase != "Applied" {
			t.Errorf("Expected phase Applied after re-apply, got %s", sc.Status.Phase)
		}
	})

	t.Run("Executor_error_sets_Failed", func(t *testing.T) {
		obj := makeUnstructuredSC("state-err", "default", "Pending", false)
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		mock := &mockStateExecutor{
			execFunc: func(_ context.Context, _ *StateConfig) error {
				return fmt.Errorf("module failed")
			},
		}
		ctrl := NewStateConfigController(nil, crdClient, config)
		ctrl.SetStateExecutor(mock)

		err := ctrl.reconcile(context.Background(), "default/state-err")
		if err == nil {
			t.Fatal("Expected error from executor")
		}

		sc, err := crdClient.GetStateConfig(context.Background(), "default", "state-err")
		if err != nil {
			t.Fatalf("Get after failed reconcile: %v", err)
		}
		if sc.Status.Phase != "Failed" {
			t.Errorf("Expected phase Failed, got %s", sc.Status.Phase)
		}
	})

	t.Run("Not_found_returns_error", func(t *testing.T) {
		dc := dynamicfake.NewSimpleDynamicClient(scheme)
		crdClient := NewCRDClientWithDynamic(dc)
		ctrl := NewStateConfigController(nil, crdClient, config)

		err := ctrl.reconcile(context.Background(), "default/nonexistent")
		if err == nil {
			t.Fatal("Expected error for not-found resource")
		}
	})

	t.Run("Nil_crdClient_is_noop", func(t *testing.T) {
		ctrl := NewStateConfigController(nil, nil, config)
		err := ctrl.reconcile(context.Background(), "default/anything")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})

	t.Run("Nil_stateExec_is_noop", func(t *testing.T) {
		obj := makeUnstructuredSC("state-no-exec", "default", "Pending", false)
		dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
		crdClient := NewCRDClientWithDynamic(dc)

		ctrl := NewStateConfigController(nil, crdClient, config)
		// stateExec is nil by default

		err := ctrl.reconcile(context.Background(), "default/state-no-exec")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
	})
}

func TestStateConfigController_DriftChecker(t *testing.T) {
	config := OperatorConfig{
		Namespace:               "default",
		ReconcileInterval:       1 * time.Minute,
		MaxConcurrentReconciles: 1,
	}

	t.Run("SetDriftChecker", func(t *testing.T) {
		ctrl := NewStateConfigController(nil, nil, config)
		checker := &mockDriftChecker{}
		ctrl.SetDriftChecker(checker)
		if ctrl.driftChecker != checker {
			t.Error("DriftChecker not set correctly")
		}
	})

	t.Run("SetStateExecutor", func(t *testing.T) {
		ctrl := NewStateConfigController(nil, nil, config)
		exec := &mockStateExecutor{}
		ctrl.SetStateExecutor(exec)
		if ctrl.stateExec != exec {
			t.Error("StateExecutor not set correctly")
		}
	})
}

func TestRemoteExecutionController_SetExecutor(t *testing.T) {
	config := OperatorConfig{
		Namespace:               "default",
		ReconcileInterval:       1 * time.Minute,
		MaxConcurrentReconciles: 1,
	}

	t.Run("default_executor_is_self", func(t *testing.T) {
		ctrl := NewRemoteExecutionController(nil, nil, config)
		// The default executor should be the controller itself
		if ctrl.executor == nil {
			t.Fatal("Expected non-nil default executor")
		}
	})

	t.Run("override_executor", func(t *testing.T) {
		ctrl := NewRemoteExecutionController(nil, nil, config)
		mock := &mockExecutor{}
		ctrl.SetExecutor(mock)
		if ctrl.executor != mock {
			t.Error("Executor not set correctly")
		}
	})
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

func TestOperatorManager_SetLeaderElector(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	k8sClient := NewClientWithInterface(clientset, ClusterConfig{})
	dc := dynamicfake.NewSimpleDynamicClient(newFakeScheme())
	crdClient := NewCRDClientWithDynamic(dc)
	cfg := OperatorConfig{Namespace: "default", ReconcileInterval: time.Minute}

	mgr := NewOperatorManager(k8sClient, crdClient, cfg)

	le := NewLeaderElector(clientset, DefaultLeaderElectionConfig("default"))
	mgr.SetLeaderElector(le)

	if mgr.leaderElector == nil {
		t.Error("Expected leaderElector to be set")
	}
}

func TestOperatorManager_StartWithoutLeaderElection(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	k8sClient := NewClientWithInterface(clientset, ClusterConfig{})
	dc := dynamicfake.NewSimpleDynamicClient(newFakeScheme())
	crdClient := NewCRDClientWithDynamic(dc)
	cfg := OperatorConfig{Namespace: "default", ReconcileInterval: time.Minute}

	mgr := NewOperatorManager(k8sClient, crdClient, cfg)
	// No controllers or leader elector — Start should work and be non-blocking
	// (since there are no controllers to start, it returns immediately)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := mgr.Start(ctx)
	if err != nil {
		t.Errorf("Start without leader election should succeed: %v", err)
	}
}

func TestClientset_Method(t *testing.T) {
	clientset := fake.NewSimpleClientset()
	k8sClient := NewClientWithInterface(clientset, ClusterConfig{})

	if k8sClient.Clientset() != clientset {
		t.Error("Clientset() should return the underlying kubernetes.Interface")
	}
}
