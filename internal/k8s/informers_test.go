package k8s

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func newInformerFakeScheme() *runtime.Scheme {
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

func TestNewInformerManager(t *testing.T) {
	scheme := newInformerFakeScheme()
	dc := dynamicfake.NewSimpleDynamicClient(scheme)

	t.Run("all_namespaces", func(t *testing.T) {
		im := NewInformerManager(dc, "", 30*time.Second)
		if im == nil {
			t.Fatal("Expected non-nil InformerManager")
		}
		if im.synced {
			t.Error("Expected synced=false before Start")
		}
	})

	t.Run("specific_namespace", func(t *testing.T) {
		im := NewInformerManager(dc, "kscore-system", 30*time.Second)
		if im == nil {
			t.Fatal("Expected non-nil InformerManager")
		}
	})
}

func TestInformerManager_HasSynced(t *testing.T) {
	scheme := newInformerFakeScheme()
	dc := dynamicfake.NewSimpleDynamicClient(scheme)
	im := NewInformerManager(dc, "", 30*time.Second)

	if im.HasSynced() {
		t.Error("Expected HasSynced()=false before Start")
	}
}

func TestInformerManager_StartStop(t *testing.T) {
	scheme := newInformerFakeScheme()
	dc := dynamicfake.NewSimpleDynamicClient(scheme)
	im := NewInformerManager(dc, "", 30*time.Second)

	// Register at least one handler so informers get created
	called := make(chan string, 10)
	im.SetRemoteExecutionHandler(&testEventHandler{called: called})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := im.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	if !im.HasSynced() {
		t.Error("Expected HasSynced()=true after Start")
	}

	im.Stop()
}

func TestInformerManager_SetRemoteExecutionHandler(t *testing.T) {
	scheme := newInformerFakeScheme()
	dc := dynamicfake.NewSimpleDynamicClient(scheme)
	im := NewInformerManager(dc, "", 30*time.Second)

	called := make(chan string, 10)
	handler := &testEventHandler{called: called}
	im.SetRemoteExecutionHandler(handler)

	// Should not panic
}

func TestInformerManager_SetStateConfigHandler(t *testing.T) {
	scheme := newInformerFakeScheme()
	dc := dynamicfake.NewSimpleDynamicClient(scheme)
	im := NewInformerManager(dc, "", 30*time.Second)

	called := make(chan string, 10)
	handler := &testEventHandler{called: called}
	im.SetStateConfigHandler(handler)

	// Should not panic
}

func TestKeyFromObject(t *testing.T) {
	t.Run("namespaced_object", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetName("test-exec")
		obj.SetNamespace("default")

		key, err := KeyFromObject(obj)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if key != "default/test-exec" {
			t.Errorf("Expected key default/test-exec, got %s", key)
		}
	})

	t.Run("cluster_scoped_object", func(t *testing.T) {
		obj := &unstructured.Unstructured{}
		obj.SetName("global-resource")

		key, err := KeyFromObject(obj)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if key != "global-resource" {
			t.Errorf("Expected key global-resource, got %s", key)
		}
	})

	t.Run("typed_object", func(t *testing.T) {
		obj := &RemoteExecution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-exec",
				Namespace: "prod",
			},
		}

		key, err := KeyFromObject(obj)
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if key != "prod/my-exec" {
			t.Errorf("Expected key prod/my-exec, got %s", key)
		}
	})
}

// testEventHandler is a simple event handler for testing.
type testEventHandler struct {
	called chan string
}

func (h *testEventHandler) OnAdd(obj interface{}, _ bool) {
	h.called <- "add"
}

func (h *testEventHandler) OnUpdate(_, _ interface{}) {
	h.called <- "update"
}

func (h *testEventHandler) OnDelete(obj interface{}) {
	h.called <- "delete"
}
