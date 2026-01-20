// Package k8s provides Kubernetes integration for Keystone.
package k8s

import (
	"context"
	"fmt"
	"time"
)

// K8sNetworkPolicyStore implements PolicyStore using Kubernetes as the backend.
// This allows PolicyManager and other components to work directly with
// NetworkPolicy resources in a Kubernetes cluster.
type K8sNetworkPolicyStore struct {
	client    *Client
	namespace string // default namespace (empty = require namespace in each call)
}

// NewK8sNetworkPolicyStore creates a new Kubernetes-backed policy store.
func NewK8sNetworkPolicyStore(client *Client, defaultNamespace string) *K8sNetworkPolicyStore {
	return &K8sNetworkPolicyStore{
		client:    client,
		namespace: defaultNamespace,
	}
}

// resolveNamespace returns the namespace to use, preferring the provided one
// over the default.
func (s *K8sNetworkPolicyStore) resolveNamespace(namespace string) string {
	if namespace != "" {
		return namespace
	}
	return s.namespace
}

// Get retrieves a NetworkPolicy from Kubernetes.
func (s *K8sNetworkPolicyStore) Get(ctx context.Context, namespace, name string) (*NetworkPolicy, error) {
	ns := s.resolveNamespace(namespace)
	if ns == "" {
		return nil, fmt.Errorf("namespace is required")
	}

	policy, err := s.client.GetNetworkPolicy(ns, name)
	if err != nil {
		return nil, err
	}

	return policy, nil
}

// List lists all NetworkPolicies in a namespace.
// If namespace is empty, lists from all namespaces.
func (s *K8sNetworkPolicyStore) List(ctx context.Context, namespace string) ([]*NetworkPolicy, error) {
	ns := s.resolveNamespace(namespace)
	// Empty namespace is valid here - means list from all namespaces

	policies, err := s.client.ListNetworkPolicies(ns, "")
	if err != nil {
		return nil, err
	}

	return policies, nil
}

// Create creates a new NetworkPolicy in Kubernetes.
func (s *K8sNetworkPolicyStore) Create(ctx context.Context, policy *NetworkPolicy) error {
	if policy == nil {
		return fmt.Errorf("policy cannot be nil")
	}

	ns := s.resolveNamespace(policy.Namespace)
	if ns == "" {
		return fmt.Errorf("namespace is required")
	}

	policy.Namespace = ns
	policy.CreatedAt = time.Now()
	policy.UpdatedAt = policy.CreatedAt

	return s.client.CreateNetworkPolicy(ns, policy)
}

// Update updates an existing NetworkPolicy in Kubernetes.
func (s *K8sNetworkPolicyStore) Update(ctx context.Context, policy *NetworkPolicy) error {
	if policy == nil {
		return fmt.Errorf("policy cannot be nil")
	}

	ns := s.resolveNamespace(policy.Namespace)
	if ns == "" {
		return fmt.Errorf("namespace is required")
	}

	policy.Namespace = ns
	policy.UpdatedAt = time.Now()

	return s.client.UpdateNetworkPolicy(ns, policy)
}

// Delete deletes a NetworkPolicy from Kubernetes.
func (s *K8sNetworkPolicyStore) Delete(ctx context.Context, namespace, name string) error {
	ns := s.resolveNamespace(namespace)
	if ns == "" {
		return fmt.Errorf("namespace is required")
	}

	return s.client.DeleteNetworkPolicy(ns, name)
}

// ListWithSelector lists NetworkPolicies matching a label selector.
func (s *K8sNetworkPolicyStore) ListWithSelector(ctx context.Context, namespace, labelSelector string) ([]*NetworkPolicy, error) {
	ns := s.resolveNamespace(namespace)

	policies, err := s.client.ListNetworkPolicies(ns, labelSelector)
	if err != nil {
		return nil, err
	}

	return policies, nil
}

// Watch watches for NetworkPolicy changes in Kubernetes.
func (s *K8sNetworkPolicyStore) Watch(ctx context.Context, namespace string) (<-chan NetworkPolicyStoreEvent, error) {
	ns := s.resolveNamespace(namespace)

	watchChan, err := s.client.WatchNetworkPolicies(ns, "")
	if err != nil {
		return nil, err
	}

	eventChan := make(chan NetworkPolicyStoreEvent, 100)

	go func() {
		defer close(eventChan)
		for {
			select {
			case <-ctx.Done():
				return
			case event, ok := <-watchChan:
				if !ok {
					return
				}
				eventChan <- NetworkPolicyStoreEvent{
					Type:      event.Type,
					Policy:    event.Policy,
					Timestamp: event.Timestamp,
				}
			}
		}
	}()

	return eventChan, nil
}

// NetworkPolicyStoreEvent represents a policy change event from the store.
type NetworkPolicyStoreEvent struct {
	Type      string         `json:"type"` // ADDED, MODIFIED, DELETED
	Policy    *NetworkPolicy `json:"policy"`
	Timestamp time.Time      `json:"timestamp"`
}

// Ensure K8sNetworkPolicyStore implements PolicyStore
var _ PolicyStore = (*K8sNetworkPolicyStore)(nil)
