package k8s

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
)

// GVR constants for Keystone Core CRDs.
var (
	RemoteExecutionGVR = schema.GroupVersionResource{
		Group:    "keystonecore.io",
		Version:  "v1",
		Resource: "remoteexecutions",
	}
	StateConfigGVR = schema.GroupVersionResource{
		Group:    "keystonecore.io",
		Version:  "v1",
		Resource: "stateconfigs",
	}
)

// CRDClient provides typed access to Keystone Core CRD resources via the dynamic client.
type CRDClient struct {
	client dynamic.Interface
}

// NewCRDClient creates a CRDClient from a rest.Config.
func NewCRDClient(config *rest.Config) (*CRDClient, error) {
	dc, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("create dynamic client: %w", err)
	}
	return &CRDClient{client: dc}, nil
}

// NewCRDClientWithDynamic creates a CRDClient with an existing dynamic.Interface (for testing).
func NewCRDClientWithDynamic(client dynamic.Interface) *CRDClient {
	return &CRDClient{client: client}
}

// Dynamic returns the underlying dynamic.Interface for use with informers.
func (c *CRDClient) Dynamic() dynamic.Interface {
	return c.client
}

// GetRemoteExecution fetches a RemoteExecution resource by namespace and name.
func (c *CRDClient) GetRemoteExecution(ctx context.Context, namespace, name string) (*RemoteExecution, error) {
	obj, err := c.client.Resource(RemoteExecutionGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return unstructuredToRemoteExecution(obj)
}

// ListRemoteExecutions lists all RemoteExecution resources in a namespace.
func (c *CRDClient) ListRemoteExecutions(ctx context.Context, namespace string) ([]RemoteExecution, error) {
	list, err := c.client.Resource(RemoteExecutionGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	result := make([]RemoteExecution, 0, len(list.Items))
	for i := range list.Items {
		re, err := unstructuredToRemoteExecution(&list.Items[i])
		if err != nil {
			return nil, fmt.Errorf("convert item %d: %w", i, err)
		}
		result = append(result, *re)
	}
	return result, nil
}

// UpdateRemoteExecutionStatus updates the status subresource of a RemoteExecution.
func (c *CRDClient) UpdateRemoteExecutionStatus(ctx context.Context, namespace, name string, status *RemoteExecutionStatus) error {
	obj, err := c.client.Resource(RemoteExecutionGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get for status update: %w", err)
	}

	statusMap, err := statusToUnstructured(status)
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}
	if err := unstructured.SetNestedField(obj.Object, statusMap, "status"); err != nil {
		return fmt.Errorf("set status field: %w", err)
	}

	_, err = c.client.Resource(RemoteExecutionGVR).Namespace(namespace).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	return err
}

// GetStateConfig fetches a StateConfig resource by namespace and name.
func (c *CRDClient) GetStateConfig(ctx context.Context, namespace, name string) (*StateConfig, error) {
	obj, err := c.client.Resource(StateConfigGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	return unstructuredToStateConfig(obj)
}

// ListStateConfigs lists all StateConfig resources in a namespace.
func (c *CRDClient) ListStateConfigs(ctx context.Context, namespace string) ([]StateConfig, error) {
	list, err := c.client.Resource(StateConfigGVR).Namespace(namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}
	result := make([]StateConfig, 0, len(list.Items))
	for i := range list.Items {
		sc, err := unstructuredToStateConfig(&list.Items[i])
		if err != nil {
			return nil, fmt.Errorf("convert item %d: %w", i, err)
		}
		result = append(result, *sc)
	}
	return result, nil
}

// UpdateStateConfigStatus updates the status subresource of a StateConfig.
func (c *CRDClient) UpdateStateConfigStatus(ctx context.Context, namespace, name string, status *StateConfigStatus) error {
	obj, err := c.client.Resource(StateConfigGVR).Namespace(namespace).Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get for status update: %w", err)
	}

	statusMap, err := statusToUnstructured(status)
	if err != nil {
		return fmt.Errorf("marshal status: %w", err)
	}
	if err := unstructured.SetNestedField(obj.Object, statusMap, "status"); err != nil {
		return fmt.Errorf("set status field: %w", err)
	}

	_, err = c.client.Resource(StateConfigGVR).Namespace(namespace).UpdateStatus(ctx, obj, metav1.UpdateOptions{})
	return err
}

// unstructuredToRemoteExecution converts an unstructured object to a RemoteExecution via JSON round-trip.
func unstructuredToRemoteExecution(obj *unstructured.Unstructured) (*RemoteExecution, error) {
	data, err := json.Marshal(obj.Object)
	if err != nil {
		return nil, fmt.Errorf("marshal unstructured: %w", err)
	}
	var re RemoteExecution
	if err := json.Unmarshal(data, &re); err != nil {
		return nil, fmt.Errorf("unmarshal RemoteExecution: %w", err)
	}
	return &re, nil
}

// unstructuredToStateConfig converts an unstructured object to a StateConfig via JSON round-trip.
func unstructuredToStateConfig(obj *unstructured.Unstructured) (*StateConfig, error) {
	data, err := json.Marshal(obj.Object)
	if err != nil {
		return nil, fmt.Errorf("marshal unstructured: %w", err)
	}
	var sc StateConfig
	if err := json.Unmarshal(data, &sc); err != nil {
		return nil, fmt.Errorf("unmarshal StateConfig: %w", err)
	}
	return &sc, nil
}

// statusToUnstructured converts a status struct to a map[string]interface{} via JSON round-trip.
func statusToUnstructured(status interface{}) (map[string]interface{}, error) {
	data, err := json.Marshal(status)
	if err != nil {
		return nil, err
	}
	var result map[string]interface{}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return result, nil
}
