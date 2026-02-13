package k8s

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newFakeScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "keystonecore.io", Version: "v1", Kind: "RemoteExecution"},
		&unstructured.Unstructured{},
	)
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "keystonecore.io", Version: "v1", Kind: "RemoteExecutionList"},
		&unstructured.UnstructuredList{},
	)
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "keystonecore.io", Version: "v1", Kind: "StateConfig"},
		&unstructured.Unstructured{},
	)
	s.AddKnownTypeWithName(
		schema.GroupVersionKind{Group: "keystonecore.io", Version: "v1", Kind: "StateConfigList"},
		&unstructured.UnstructuredList{},
	)
	return s
}

func makeUnstructuredRE(name, namespace, phase string) *unstructured.Unstructured {
	re := &RemoteExecution{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keystonecore.io/v1",
			Kind:       "RemoteExecution",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: RemoteExecutionSpec{
			Target:  PodSelector{Namespace: namespace, LabelSelector: "app=test"},
			Command: []string{"echo", "hello"},
			Mode:    ExecModePod,
			Timeout: metav1.Duration{Duration: 30 * time.Second},
		},
		Status: RemoteExecutionStatus{
			Phase: phase,
		},
	}
	data, _ := json.Marshal(re)
	var obj map[string]interface{}
	json.Unmarshal(data, &obj) //nolint:errcheck
	return &unstructured.Unstructured{Object: obj}
}

func makeUnstructuredSC(name, namespace, phase string, drift bool) *unstructured.Unstructured {
	sc := &StateConfig{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "keystonecore.io/v1",
			Kind:       "StateConfig",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: StateConfigSpec{
			Target: PodSelector{Namespace: namespace},
			States: []StateDeclaration{
				{Name: "test_state", Module: "file", Parameters: map[string]interface{}{"path": "/tmp/test"}},
			},
		},
		Status: StateConfigStatus{
			Phase:         phase,
			DriftDetected: drift,
		},
	}
	data, _ := json.Marshal(sc)
	var obj map[string]interface{}
	json.Unmarshal(data, &obj) //nolint:errcheck
	return &unstructured.Unstructured{Object: obj}
}

func TestGVRConstants(t *testing.T) {
	t.Run("RemoteExecutionGVR", func(t *testing.T) {
		if RemoteExecutionGVR.Group != "keystonecore.io" {
			t.Errorf("Expected group keystonecore.io, got %s", RemoteExecutionGVR.Group)
		}
		if RemoteExecutionGVR.Version != "v1" {
			t.Errorf("Expected version v1, got %s", RemoteExecutionGVR.Version)
		}
		if RemoteExecutionGVR.Resource != "remoteexecutions" {
			t.Errorf("Expected resource remoteexecutions, got %s", RemoteExecutionGVR.Resource)
		}
	})

	t.Run("StateConfigGVR", func(t *testing.T) {
		if StateConfigGVR.Group != "keystonecore.io" {
			t.Errorf("Expected group keystonecore.io, got %s", StateConfigGVR.Group)
		}
		if StateConfigGVR.Version != "v1" {
			t.Errorf("Expected version v1, got %s", StateConfigGVR.Version)
		}
		if StateConfigGVR.Resource != "stateconfigs" {
			t.Errorf("Expected resource stateconfigs, got %s", StateConfigGVR.Resource)
		}
	})
}

func TestNewCRDClientWithDynamic(t *testing.T) {
	scheme := newFakeScheme()
	dc := dynamicfake.NewSimpleDynamicClient(scheme)
	client := NewCRDClientWithDynamic(dc)
	if client == nil {
		t.Fatal("Expected non-nil CRDClient")
	}
}

func TestCRDClient_GetRemoteExecution(t *testing.T) {
	ctx := context.Background()
	scheme := newFakeScheme()
	obj := makeUnstructuredRE("test-exec", "default", "Pending")
	dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
	client := NewCRDClientWithDynamic(dc)

	t.Run("found", func(t *testing.T) {
		re, err := client.GetRemoteExecution(ctx, "default", "test-exec")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if re.Name != "test-exec" {
			t.Errorf("Expected name test-exec, got %s", re.Name)
		}
		if re.Namespace != "default" {
			t.Errorf("Expected namespace default, got %s", re.Namespace)
		}
		if re.Status.Phase != "Pending" {
			t.Errorf("Expected phase Pending, got %s", re.Status.Phase)
		}
		if re.Spec.Target.LabelSelector != "app=test" {
			t.Errorf("Expected label selector app=test, got %s", re.Spec.Target.LabelSelector)
		}
	})

	t.Run("not_found", func(t *testing.T) {
		_, err := client.GetRemoteExecution(ctx, "default", "nonexistent")
		if err == nil {
			t.Fatal("Expected error for nonexistent resource")
		}
	})
}

func TestCRDClient_ListRemoteExecutions(t *testing.T) {
	ctx := context.Background()
	scheme := newFakeScheme()
	obj1 := makeUnstructuredRE("exec-1", "default", "Pending")
	obj2 := makeUnstructuredRE("exec-2", "default", "Succeeded")
	dc := dynamicfake.NewSimpleDynamicClient(scheme, obj1, obj2)
	client := NewCRDClientWithDynamic(dc)

	items, err := client.ListRemoteExecutions(ctx, "default")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("Expected 2 items, got %d", len(items))
	}
}

func TestCRDClient_UpdateRemoteExecutionStatus(t *testing.T) {
	ctx := context.Background()
	scheme := newFakeScheme()
	obj := makeUnstructuredRE("test-exec", "default", "Pending")
	dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
	client := NewCRDClientWithDynamic(dc)

	status := &RemoteExecutionStatus{
		Phase:         "Succeeded",
		PodsExecuted:  3,
		PodsSucceeded: 3,
		Message:       "All done",
	}

	err := client.UpdateRemoteExecutionStatus(ctx, "default", "test-exec", status)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	// Verify the update was persisted
	re, err := client.GetRemoteExecution(ctx, "default", "test-exec")
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}
	if re.Status.Phase != "Succeeded" {
		t.Errorf("Expected phase Succeeded, got %s", re.Status.Phase)
	}
}

func TestCRDClient_GetStateConfig(t *testing.T) {
	ctx := context.Background()
	scheme := newFakeScheme()
	obj := makeUnstructuredSC("test-state", "default", "Applied", false)
	dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
	client := NewCRDClientWithDynamic(dc)

	t.Run("found", func(t *testing.T) {
		sc, err := client.GetStateConfig(ctx, "default", "test-state")
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if sc.Name != "test-state" {
			t.Errorf("Expected name test-state, got %s", sc.Name)
		}
		if sc.Status.Phase != "Applied" {
			t.Errorf("Expected phase Applied, got %s", sc.Status.Phase)
		}
		if len(sc.Spec.States) != 1 {
			t.Errorf("Expected 1 state declaration, got %d", len(sc.Spec.States))
		}
	})

	t.Run("not_found", func(t *testing.T) {
		_, err := client.GetStateConfig(ctx, "default", "nonexistent")
		if err == nil {
			t.Fatal("Expected error for nonexistent resource")
		}
	})
}

func TestCRDClient_ListStateConfigs(t *testing.T) {
	ctx := context.Background()
	scheme := newFakeScheme()
	obj1 := makeUnstructuredSC("state-1", "default", "Applied", false)
	obj2 := makeUnstructuredSC("state-2", "default", "Pending", false)
	obj3 := makeUnstructuredSC("state-3", "default", "Applied", true)
	dc := dynamicfake.NewSimpleDynamicClient(scheme, obj1, obj2, obj3)
	client := NewCRDClientWithDynamic(dc)

	items, err := client.ListStateConfigs(ctx, "default")
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("Expected 3 items, got %d", len(items))
	}
}

func TestCRDClient_UpdateStateConfigStatus(t *testing.T) {
	ctx := context.Background()
	scheme := newFakeScheme()
	obj := makeUnstructuredSC("test-state", "default", "Pending", false)
	dc := dynamicfake.NewSimpleDynamicClient(scheme, obj)
	client := NewCRDClientWithDynamic(dc)

	now := metav1.Now()
	status := &StateConfigStatus{
		Phase:         "Applied",
		PodsApplied:   5,
		PodsSucceeded: 5,
		LastApplied:   &now,
		DriftDetected: false,
		Message:       "Applied successfully",
	}

	err := client.UpdateStateConfigStatus(ctx, "default", "test-state", status)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}

	sc, err := client.GetStateConfig(ctx, "default", "test-state")
	if err != nil {
		t.Fatalf("Get after update failed: %v", err)
	}
	if sc.Status.Phase != "Applied" {
		t.Errorf("Expected phase Applied, got %s", sc.Status.Phase)
	}
}

func TestUnstructuredConversion_RoundTrip(t *testing.T) {
	t.Run("RemoteExecution", func(t *testing.T) {
		now := metav1.Now()
		original := &RemoteExecution{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "keystonecore.io/v1",
				Kind:       "RemoteExecution",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test",
				Namespace: "ns1",
			},
			Spec: RemoteExecutionSpec{
				Target:  PodSelector{Namespace: "ns1", LabelSelector: "app=web", MaxPods: 10},
				Command: []string{"ls", "-la"},
				Mode:    ExecModeJob,
				Timeout: metav1.Duration{Duration: 2 * time.Minute},
			},
			Status: RemoteExecutionStatus{
				Phase:         "Running",
				PodsExecuted:  5,
				PodsSucceeded: 3,
				PodsFailed:    2,
				StartTime:     &now,
				Message:       "In progress",
			},
		}

		// Marshal to unstructured
		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			t.Fatalf("Unmarshal to map: %v", err)
		}

		u := &unstructured.Unstructured{Object: obj}
		result, err := unstructuredToRemoteExecution(u)
		if err != nil {
			t.Fatalf("Conversion: %v", err)
		}

		if result.Name != "test" {
			t.Errorf("Name: got %s, want test", result.Name)
		}
		if result.Spec.Mode != ExecModeJob {
			t.Errorf("Mode: got %s, want job", result.Spec.Mode)
		}
		if result.Spec.Target.MaxPods != 10 {
			t.Errorf("MaxPods: got %d, want 10", result.Spec.Target.MaxPods)
		}
		if result.Status.PodsExecuted != 5 {
			t.Errorf("PodsExecuted: got %d, want 5", result.Status.PodsExecuted)
		}
	})

	t.Run("StateConfig", func(t *testing.T) {
		original := &StateConfig{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "keystonecore.io/v1",
				Kind:       "StateConfig",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "state-test",
				Namespace: "prod",
			},
			Spec: StateConfigSpec{
				Target: PodSelector{Namespace: "prod", LabelSelector: "tier=backend"},
				States: []StateDeclaration{
					{
						Name:       "install_pkg",
						Module:     "package",
						Parameters: map[string]interface{}{"name": "nginx", "version": "1.25"},
					},
					{
						Name:       "config_file",
						Module:     "file",
						Parameters: map[string]interface{}{"path": "/etc/nginx/nginx.conf"},
						Requisites: map[string][]string{"require": {"install_pkg"}},
					},
				},
				Vars: map[string]interface{}{"env": "production", "port": float64(8080)},
			},
			Status: StateConfigStatus{
				Phase:         "Applied",
				PodsApplied:   3,
				PodsSucceeded: 3,
				DriftDetected: true,
			},
		}

		data, err := json.Marshal(original)
		if err != nil {
			t.Fatalf("Marshal: %v", err)
		}
		var obj map[string]interface{}
		if err := json.Unmarshal(data, &obj); err != nil {
			t.Fatalf("Unmarshal to map: %v", err)
		}

		u := &unstructured.Unstructured{Object: obj}
		result, err := unstructuredToStateConfig(u)
		if err != nil {
			t.Fatalf("Conversion: %v", err)
		}

		if result.Name != "state-test" {
			t.Errorf("Name: got %s, want state-test", result.Name)
		}
		if len(result.Spec.States) != 2 {
			t.Fatalf("States len: got %d, want 2", len(result.Spec.States))
		}
		if result.Spec.States[0].Module != "package" {
			t.Errorf("Module: got %s, want package", result.Spec.States[0].Module)
		}
		if !result.Status.DriftDetected {
			t.Error("Expected drift detected")
		}
		if result.Spec.Vars["env"] != "production" {
			t.Errorf("Vars[env]: got %v, want production", result.Spec.Vars["env"])
		}
	})
}

func TestStatusToUnstructured(t *testing.T) {
	status := &RemoteExecutionStatus{
		Phase:         "Succeeded",
		PodsExecuted:  10,
		PodsSucceeded: 10,
		Message:       "All done",
	}

	result, err := statusToUnstructured(status)
	if err != nil {
		t.Fatalf("statusToUnstructured: %v", err)
	}

	if result["Phase"] != "Succeeded" {
		t.Errorf("Phase: got %v, want Succeeded", result["Phase"])
	}
	if result["Message"] != "All done" {
		t.Errorf("Message: got %v, want All done", result["Message"])
	}
}

func TestCRDClient_Dynamic(t *testing.T) {
	dc := dynamicfake.NewSimpleDynamicClient(newFakeScheme())
	client := NewCRDClientWithDynamic(dc)

	if client.Dynamic() != dc {
		t.Error("Dynamic() should return the underlying dynamic.Interface")
	}
}
