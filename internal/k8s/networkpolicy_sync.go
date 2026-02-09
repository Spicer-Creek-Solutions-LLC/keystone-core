// Package k8s provides Kubernetes integration for Keystone.
package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// NetworkPolicySyncConfig configures the network policy synchronizer.
type NetworkPolicySyncConfig struct {
	// Kubeconfig path (empty for in-cluster)
	Kubeconfig string

	// SyncInterval for periodic synchronization
	SyncInterval time.Duration

	// Namespaces to watch (empty = all namespaces)
	Namespaces []string

	// LabelSelector to filter policies
	LabelSelector string

	// VerifyOnSync whether to verify policies during sync
	VerifyOnSync bool

	// ContinueOnError whether to continue syncing on individual errors
	ContinueOnError bool
}

// DefaultNetworkPolicySyncConfig returns the default synchronizer configuration.
func DefaultNetworkPolicySyncConfig() *NetworkPolicySyncConfig {
	return &NetworkPolicySyncConfig{
		SyncInterval:    1 * time.Minute,
		VerifyOnSync:    true,
		ContinueOnError: true,
	}
}

// NetworkPolicySyncResult contains the result of a synchronization.
type NetworkPolicySyncResult struct {
	// Timestamp when the sync started
	Timestamp time.Time `json:"timestamp"`

	// PoliciesFound is the number of policies found
	PoliciesFound int `json:"policies_found"`

	// PoliciesVerified is the number of policies verified
	PoliciesVerified int `json:"policies_verified"`

	// PoliciesPassed is the number of policies that passed verification
	PoliciesPassed int `json:"policies_passed"`

	// PoliciesFailed is the number of policies that failed verification
	PoliciesFailed int `json:"policies_failed"`

	// Errors encountered during sync
	Errors []NetworkPolicySyncError `json:"errors,omitempty"`

	// Duration of the sync operation
	Duration time.Duration `json:"duration"`
}

// NetworkPolicySyncError represents a synchronization error.
type NetworkPolicySyncError struct {
	// PolicyName is the name of the policy
	PolicyName string `json:"policy_name,omitempty"`

	// Namespace is the namespace of the policy
	Namespace string `json:"namespace,omitempty"`

	// Error is the error message
	Error string `json:"error"`

	// Timestamp when the error occurred
	Timestamp time.Time `json:"timestamp"`
}

// NetworkPolicyChangeEvent represents a policy change.
type NetworkPolicyChangeEvent struct {
	// Type of change (added, modified, deleted)
	Type string `json:"type"`

	// Policy is the current policy (nil for deleted)
	Policy *NetworkPolicy `json:"policy,omitempty"`

	// OldPolicy is the previous policy (for modifications)
	OldPolicy *NetworkPolicy `json:"old_policy,omitempty"`

	// Timestamp of the change
	Timestamp time.Time `json:"timestamp"`
}

// NetworkPolicySynchronizer watches and synchronizes network policies.
type NetworkPolicySynchronizer struct {
	config   *NetworkPolicySyncConfig
	client   *Client
	store    PolicyStore
	verifier *NetworkPolicyVerifier

	// State
	mu             sync.RWMutex
	running        bool
	stopCh         chan struct{}
	lastSyncResult *NetworkPolicySyncResult
	lastSyncError  error

	// Known policies for change detection
	knownPolicies map[string]*NetworkPolicy // key: namespace/name

	// Callbacks
	onPolicyChange func(NetworkPolicyChangeEvent)
	onSyncComplete func(*NetworkPolicySyncResult)
	onSyncError    func(NetworkPolicySyncError)
}

// NewNetworkPolicySynchronizer creates a new network policy synchronizer.
func NewNetworkPolicySynchronizer(config *NetworkPolicySyncConfig) (*NetworkPolicySynchronizer, error) {
	if config == nil {
		config = DefaultNetworkPolicySyncConfig()
	}

	client, err := NewClient(ClusterConfig{
		Kubeconfig: config.Kubeconfig,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return &NetworkPolicySynchronizer{
		config:        config,
		client:        client,
		store:         NewInMemoryPolicyStore(),
		verifier:      NewNetworkPolicyVerifier(client),
		stopCh:        make(chan struct{}),
		knownPolicies: make(map[string]*NetworkPolicy),
	}, nil
}

// NewNetworkPolicySynchronizerWithClient creates a synchronizer with a provided client.
func NewNetworkPolicySynchronizerWithClient(config *NetworkPolicySyncConfig, client *Client) *NetworkPolicySynchronizer {
	if config == nil {
		config = DefaultNetworkPolicySyncConfig()
	}

	return &NetworkPolicySynchronizer{
		config:        config,
		client:        client,
		store:         NewInMemoryPolicyStore(),
		verifier:      NewNetworkPolicyVerifier(client),
		stopCh:        make(chan struct{}),
		knownPolicies: make(map[string]*NetworkPolicy),
	}
}

// Start starts the network policy synchronizer.
func (s *NetworkPolicySynchronizer) Start(ctx context.Context) error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return fmt.Errorf("synchronizer already running")
	}
	s.running = true
	s.stopCh = make(chan struct{})
	s.mu.Unlock()

	// Run initial sync
	go func() {
		_, _ = s.SyncNow(ctx)
	}()

	// Start periodic sync
	go func() {
		ticker := time.NewTicker(s.config.SyncInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-s.stopCh:
				return
			case <-ticker.C:
				_, _ = s.SyncNow(ctx)
			}
		}
	}()

	return nil
}

// Stop stops the network policy synchronizer.
func (s *NetworkPolicySynchronizer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	close(s.stopCh)
	s.running = false
	return nil
}

// IsRunning returns whether the synchronizer is running.
func (s *NetworkPolicySynchronizer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// SyncNow performs an immediate synchronization.
func (s *NetworkPolicySynchronizer) SyncNow(ctx context.Context) (*NetworkPolicySyncResult, error) {
	start := time.Now()
	result := &NetworkPolicySyncResult{
		Timestamp: start,
		Errors:    make([]NetworkPolicySyncError, 0),
	}

	namespaces := s.config.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""} // Empty string means all namespaces
	}

	// Track current policies for change detection
	currentPolicies := make(map[string]*NetworkPolicy)

	for _, ns := range namespaces {
		if err := s.syncNamespace(ctx, ns, result, currentPolicies); err != nil {
			if !s.config.ContinueOnError {
				result.Duration = time.Since(start)
				s.setLastSyncResult(result, err)
				return result, err
			}
		}
	}

	// Detect deleted policies
	s.detectDeletedPolicies(currentPolicies)

	// Update known policies
	s.mu.Lock()
	s.knownPolicies = currentPolicies
	s.mu.Unlock()

	result.Duration = time.Since(start)
	s.setLastSyncResult(result, nil)

	// Notify completion
	if s.onSyncComplete != nil {
		s.onSyncComplete(result)
	}

	return result, nil
}

// syncNamespace syncs policies from a specific namespace.
func (s *NetworkPolicySynchronizer) syncNamespace(ctx context.Context, namespace string, result *NetworkPolicySyncResult, current map[string]*NetworkPolicy) error {
	policies, err := s.client.ListNetworkPolicies(ctx, namespace, s.config.LabelSelector)
	if err != nil {
		syncErr := NetworkPolicySyncError{
			Namespace: namespace,
			Error:     err.Error(),
			Timestamp: time.Now(),
		}
		result.Errors = append(result.Errors, syncErr)
		if s.onSyncError != nil {
			s.onSyncError(syncErr)
		}
		return err
	}

	for _, policy := range policies {
		result.PoliciesFound++
		key := policyKey(policy.Namespace, policy.Name)
		current[key] = policy

		// Add to store
		_ = s.store.Create(ctx, policy)

		// Detect changes
		s.mu.RLock()
		oldPolicy, existed := s.knownPolicies[key]
		s.mu.RUnlock()

		if !existed {
			s.emitPolicyChange(NetworkPolicyChangeEvent{
				Type:      "added",
				Policy:    policy,
				Timestamp: time.Now(),
			})
		} else if !networkPoliciesEqual(oldPolicy, policy) {
			s.emitPolicyChange(NetworkPolicyChangeEvent{
				Type:      "modified",
				Policy:    policy,
				OldPolicy: oldPolicy,
				Timestamp: time.Now(),
			})
		}

		// Verify if configured
		if s.config.VerifyOnSync && s.verifier != nil {
			verifyResult, err := s.verifier.Verify(ctx, policy)
			if err != nil {
				syncErr := NetworkPolicySyncError{
					PolicyName: policy.Name,
					Namespace:  policy.Namespace,
					Error:      err.Error(),
					Timestamp:  time.Now(),
				}
				result.Errors = append(result.Errors, syncErr)
				if s.onSyncError != nil {
					s.onSyncError(syncErr)
				}
			} else {
				result.PoliciesVerified++
				if verifyResult.Passed {
					result.PoliciesPassed++
				} else {
					result.PoliciesFailed++
				}
			}
		}
	}

	return nil
}

// detectDeletedPolicies detects and emits events for deleted policies.
func (s *NetworkPolicySynchronizer) detectDeletedPolicies(currentPolicies map[string]*NetworkPolicy) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	for key, oldPolicy := range s.knownPolicies {
		if _, exists := currentPolicies[key]; !exists {
			s.emitPolicyChange(NetworkPolicyChangeEvent{
				Type:      "deleted",
				OldPolicy: oldPolicy,
				Timestamp: time.Now(),
			})
		}
	}
}

// emitPolicyChange emits a policy change event.
func (s *NetworkPolicySynchronizer) emitPolicyChange(event NetworkPolicyChangeEvent) {
	if s.onPolicyChange != nil {
		s.onPolicyChange(event)
	}
}

// setLastSyncResult sets the last sync result.
func (s *NetworkPolicySynchronizer) setLastSyncResult(result *NetworkPolicySyncResult, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSyncResult = result
	s.lastSyncError = err
}

// GetLastSyncResult returns the last sync result.
func (s *NetworkPolicySynchronizer) GetLastSyncResult() (*NetworkPolicySyncResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncResult, s.lastSyncError
}

// GetStore returns the policy store.
func (s *NetworkPolicySynchronizer) GetStore() PolicyStore {
	return s.store
}

// GetVerifier returns the policy verifier.
func (s *NetworkPolicySynchronizer) GetVerifier() *NetworkPolicyVerifier {
	return s.verifier
}

// OnPolicyChange sets the callback for policy changes.
func (s *NetworkPolicySynchronizer) OnPolicyChange(callback func(NetworkPolicyChangeEvent)) {
	s.onPolicyChange = callback
}

// OnSyncComplete sets the callback for sync completion.
func (s *NetworkPolicySynchronizer) OnSyncComplete(callback func(*NetworkPolicySyncResult)) {
	s.onSyncComplete = callback
}

// OnSyncError sets the callback for sync errors.
func (s *NetworkPolicySynchronizer) OnSyncError(callback func(NetworkPolicySyncError)) {
	s.onSyncError = callback
}

// networkPoliciesEqual compares two network policies for equality.
func networkPoliciesEqual(a, b *NetworkPolicy) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Hash() == b.Hash()
}

// policyKey generates a unique key for a policy.
func policyKey(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "/" + name
}
