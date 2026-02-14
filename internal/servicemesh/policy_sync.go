// Package servicemesh provides service mesh integration for Keystone Core.
package servicemesh

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// SyncConfig configures the policy synchronizer
type SyncConfig struct {
	// Kubeconfig path (empty for in-cluster)
	Kubeconfig string

	// SyncInterval for periodic sync
	SyncInterval time.Duration

	// Namespaces to watch (empty = all namespaces)
	Namespaces []string

	// MeshType for the synchronizer
	MeshType MeshType

	// VerifyOnSync whether to verify policies during sync
	VerifyOnSync bool

	// ContinueOnError whether to continue syncing on individual errors
	ContinueOnError bool
}

// DefaultSyncConfig returns the default synchronizer configuration
func DefaultSyncConfig() *SyncConfig {
	return &SyncConfig{
		SyncInterval:    1 * time.Minute,
		MeshType:        MeshTypeIstio,
		VerifyOnSync:    true,
		ContinueOnError: true,
	}
}

// SyncResult contains the result of a synchronization
type SyncResult struct {
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

	// AuthPoliciesFound is the number of authorization policies found
	AuthPoliciesFound int `json:"auth_policies_found"`

	// DestRulesFound is the number of destination rules found
	DestRulesFound int `json:"dest_rules_found"`

	// Errors encountered during sync
	Errors []SyncError `json:"errors,omitempty"`

	// Duration of the sync operation
	Duration time.Duration `json:"duration"`
}

// SyncError represents a synchronization error
type SyncError struct {
	// PolicyName is the name of the policy
	PolicyName string `json:"policy_name,omitempty"`

	// Namespace is the namespace of the policy
	Namespace string `json:"namespace,omitempty"`

	// ResourceType is the type of resource (PeerAuthentication, AuthorizationPolicy, etc.)
	ResourceType string `json:"resource_type,omitempty"`

	// Error is the error message
	Error string `json:"error"`

	// Timestamp when the error occurred
	Timestamp time.Time `json:"timestamp"`
}

// PolicyChangeEvent represents a policy change
type PolicyChangeEvent struct {
	// Type of change (added, modified, deleted)
	Type string `json:"type"`

	// ResourceType (mTLS, Authorization, DestinationRule)
	ResourceType string `json:"resource_type"`

	// Policy is the current policy (nil for deleted)
	Policy *MTLSPolicy `json:"policy,omitempty"`

	// AuthPolicy is the current authorization policy (nil for deleted)
	AuthPolicy *AuthorizationPolicy `json:"auth_policy,omitempty"`

	// OldPolicy is the previous policy (for modifications)
	OldPolicy *MTLSPolicy `json:"old_policy,omitempty"`

	// OldAuthPolicy is the previous authorization policy (for modifications)
	OldAuthPolicy *AuthorizationPolicy `json:"old_auth_policy,omitempty"`

	// Timestamp of the change
	Timestamp time.Time `json:"timestamp"`
}

// PolicySynchronizer watches and verifies mesh policies
type PolicySynchronizer struct {
	config   *SyncConfig
	client   *IstioCRDClient
	verifier *PolicyVerifier
	store    *PolicyStore

	// State
	mu             sync.RWMutex
	running        bool
	stopCh         chan struct{}
	lastSyncResult *SyncResult
	lastSyncError  error

	// Known policies for change detection
	knownMTLSPolicies map[string]*MTLSPolicy // key: namespace/name
	knownAuthPolicies map[string]*AuthorizationPolicy

	// Callbacks
	onPolicyChange func(PolicyChangeEvent)
	onSyncComplete func(*SyncResult)
	onSyncError    func(SyncError)
}

// NewPolicySynchronizer creates a new policy synchronizer
func NewPolicySynchronizer(config *SyncConfig) (*PolicySynchronizer, error) {
	if config == nil {
		config = DefaultSyncConfig()
	}

	client, err := NewIstioCRDClient(config.Kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Istio CRD client: %w", err)
	}

	return &PolicySynchronizer{
		config:            config,
		client:            client,
		verifier:          NewPolicyVerifier(config.MeshType, nil),
		store:             NewPolicyStore(),
		stopCh:            make(chan struct{}),
		knownMTLSPolicies: make(map[string]*MTLSPolicy),
		knownAuthPolicies: make(map[string]*AuthorizationPolicy),
	}, nil
}

// NewPolicySynchronizerWithClient creates a synchronizer with a provided client
func NewPolicySynchronizerWithClient(config *SyncConfig, client *IstioCRDClient) *PolicySynchronizer {
	if config == nil {
		config = DefaultSyncConfig()
	}

	return &PolicySynchronizer{
		config:            config,
		client:            client,
		verifier:          NewPolicyVerifier(config.MeshType, nil),
		store:             NewPolicyStore(),
		stopCh:            make(chan struct{}),
		knownMTLSPolicies: make(map[string]*MTLSPolicy),
		knownAuthPolicies: make(map[string]*AuthorizationPolicy),
	}
}

// Start starts the policy synchronizer
func (s *PolicySynchronizer) Start(ctx context.Context) error {
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

// Stop stops the policy synchronizer
func (s *PolicySynchronizer) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	close(s.stopCh)
	s.running = false
	return nil
}

// IsRunning returns whether the synchronizer is running
func (s *PolicySynchronizer) IsRunning() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.running
}

// SyncNow performs an immediate synchronization
func (s *PolicySynchronizer) SyncNow(ctx context.Context) (*SyncResult, error) {
	start := time.Now()
	result := &SyncResult{
		Timestamp: start,
		Errors:    make([]SyncError, 0),
	}

	namespaces := s.config.Namespaces
	if len(namespaces) == 0 {
		namespaces = []string{""} // Empty string means all namespaces
	}

	// Track current policies for change detection
	currentMTLSPolicies := make(map[string]*MTLSPolicy)
	currentAuthPolicies := make(map[string]*AuthorizationPolicy)

	for _, ns := range namespaces {
		// Sync PeerAuthentications
		if err := s.syncPeerAuthentications(ctx, ns, result, currentMTLSPolicies); err != nil {
			if !s.config.ContinueOnError {
				result.Duration = time.Since(start)
				s.setLastSyncResult(result, err)
				return result, err
			}
		}

		// Sync AuthorizationPolicies
		if err := s.syncAuthorizationPolicies(ctx, ns, result, currentAuthPolicies); err != nil {
			if !s.config.ContinueOnError {
				result.Duration = time.Since(start)
				s.setLastSyncResult(result, err)
				return result, err
			}
		}

		// Sync DestinationRules
		if err := s.syncDestinationRules(ctx, ns, result); err != nil {
			if !s.config.ContinueOnError {
				result.Duration = time.Since(start)
				s.setLastSyncResult(result, err)
				return result, err
			}
		}
	}

	// Detect deleted policies
	s.detectDeletedPolicies(currentMTLSPolicies, currentAuthPolicies)

	// Update known policies
	s.mu.Lock()
	s.knownMTLSPolicies = currentMTLSPolicies
	s.knownAuthPolicies = currentAuthPolicies
	s.mu.Unlock()

	result.Duration = time.Since(start)
	s.setLastSyncResult(result, nil)

	// Notify completion
	if s.onSyncComplete != nil {
		s.onSyncComplete(result)
	}

	return result, nil
}

// syncPeerAuthentications syncs PeerAuthentication resources
func (s *PolicySynchronizer) syncPeerAuthentications(ctx context.Context, namespace string, result *SyncResult, current map[string]*MTLSPolicy) error {
	policies, err := s.client.ListPeerAuthentications(ctx, namespace)
	if err != nil {
		syncErr := SyncError{
			Namespace:    namespace,
			ResourceType: "PeerAuthentication",
			Error:        err.Error(),
			Timestamp:    time.Now(),
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
		s.store.Add(policy)

		// Detect changes
		s.mu.RLock()
		oldPolicy, existed := s.knownMTLSPolicies[key]
		s.mu.RUnlock()

		if !existed {
			s.emitPolicyChange(PolicyChangeEvent{
				Type:         "added",
				ResourceType: "mTLS",
				Policy:       policy,
				Timestamp:    time.Now(),
			})
		} else if !mtlsPoliciesEqual(oldPolicy, policy) {
			s.emitPolicyChange(PolicyChangeEvent{
				Type:         "modified",
				ResourceType: "mTLS",
				Policy:       policy,
				OldPolicy:    oldPolicy,
				Timestamp:    time.Now(),
			})
		}

		// Verify if configured
		if s.config.VerifyOnSync {
			verifyResult, err := s.verifier.VerifyPolicy(ctx, policy)
			if err != nil {
				syncErr := SyncError{
					PolicyName:   policy.Name,
					Namespace:    policy.Namespace,
					ResourceType: "PeerAuthentication",
					Error:        err.Error(),
					Timestamp:    time.Now(),
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

// syncAuthorizationPolicies syncs AuthorizationPolicy resources
func (s *PolicySynchronizer) syncAuthorizationPolicies(ctx context.Context, namespace string, result *SyncResult, current map[string]*AuthorizationPolicy) error {
	policies, err := s.client.ListAuthorizationPolicies(ctx, namespace)
	if err != nil {
		syncErr := SyncError{
			Namespace:    namespace,
			ResourceType: "AuthorizationPolicy",
			Error:        err.Error(),
			Timestamp:    time.Now(),
		}
		result.Errors = append(result.Errors, syncErr)
		if s.onSyncError != nil {
			s.onSyncError(syncErr)
		}
		return err
	}

	for _, policy := range policies {
		result.AuthPoliciesFound++
		key := policyKey(policy.Namespace, policy.Name)
		current[key] = policy

		// Detect changes
		s.mu.RLock()
		existing, existed := s.knownAuthPolicies[key]
		s.mu.RUnlock()

		if !existed {
			s.emitPolicyChange(PolicyChangeEvent{
				Type:         "added",
				ResourceType: "Authorization",
				AuthPolicy:   policy,
				Timestamp:    time.Now(),
			})
		} else if !authPoliciesEqual(existing, policy) {
			s.emitPolicyChange(PolicyChangeEvent{
				Type:          "modified",
				ResourceType:  "Authorization",
				AuthPolicy:    policy,
				OldAuthPolicy: existing,
				Timestamp:     time.Now(),
			})
		}
	}

	return nil
}

// syncDestinationRules syncs DestinationRule resources
func (s *PolicySynchronizer) syncDestinationRules(ctx context.Context, namespace string, result *SyncResult) error {
	rules, err := s.client.ListDestinationRules(ctx, namespace)
	if err != nil {
		syncErr := SyncError{
			Namespace:    namespace,
			ResourceType: "DestinationRule",
			Error:        err.Error(),
			Timestamp:    time.Now(),
		}
		result.Errors = append(result.Errors, syncErr)
		if s.onSyncError != nil {
			s.onSyncError(syncErr)
		}
		return err
	}

	result.DestRulesFound += len(rules)
	return nil
}

// detectDeletedPolicies detects and emits events for deleted policies
func (s *PolicySynchronizer) detectDeletedPolicies(currentMTLS map[string]*MTLSPolicy, currentAuth map[string]*AuthorizationPolicy) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Check for deleted mTLS policies
	for key, oldPolicy := range s.knownMTLSPolicies {
		if _, exists := currentMTLS[key]; !exists {
			s.emitPolicyChange(PolicyChangeEvent{
				Type:         "deleted",
				ResourceType: "mTLS",
				OldPolicy:    oldPolicy,
				Timestamp:    time.Now(),
			})
			s.store.Remove(oldPolicy.Namespace, oldPolicy.Service)
		}
	}

	// Check for deleted authorization policies
	for key := range s.knownAuthPolicies {
		if _, exists := currentAuth[key]; !exists {
			s.emitPolicyChange(PolicyChangeEvent{
				Type:         "deleted",
				ResourceType: "Authorization",
				Timestamp:    time.Now(),
			})
		}
	}
}

// emitPolicyChange emits a policy change event
func (s *PolicySynchronizer) emitPolicyChange(event PolicyChangeEvent) {
	if s.onPolicyChange != nil {
		s.onPolicyChange(event)
	}
}

// setLastSyncResult sets the last sync result
func (s *PolicySynchronizer) setLastSyncResult(result *SyncResult, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastSyncResult = result
	s.lastSyncError = err
}

// GetLastSyncResult returns the last sync result
func (s *PolicySynchronizer) GetLastSyncResult() (*SyncResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastSyncResult, s.lastSyncError
}

// GetStore returns the policy store
func (s *PolicySynchronizer) GetStore() *PolicyStore {
	return s.store
}

// GetVerifier returns the policy verifier
func (s *PolicySynchronizer) GetVerifier() *PolicyVerifier {
	return s.verifier
}

// SetMetadata sets the mesh metadata for verification
func (s *PolicySynchronizer) SetMetadata(metadata *Metadata) {
	s.verifier.SetMetadata(metadata)
}

// OnPolicyChange sets the callback for policy changes
func (s *PolicySynchronizer) OnPolicyChange(callback func(PolicyChangeEvent)) {
	s.onPolicyChange = callback
}

// OnSyncComplete sets the callback for sync completion
func (s *PolicySynchronizer) OnSyncComplete(callback func(*SyncResult)) {
	s.onSyncComplete = callback
}

// OnSyncError sets the callback for sync errors
func (s *PolicySynchronizer) OnSyncError(callback func(SyncError)) {
	s.onSyncError = callback
}

// mtlsPoliciesEqual compares two mTLS policies for equality
func mtlsPoliciesEqual(a, b *MTLSPolicy) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Name == b.Name &&
		a.Namespace == b.Namespace &&
		a.Service == b.Service &&
		a.Mode == b.Mode
}

// authPoliciesEqual compares two authorization policies for equality
func authPoliciesEqual(a, b *AuthorizationPolicy) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.Name != b.Name || a.Namespace != b.Namespace || a.Action != b.Action {
		return false
	}
	if len(a.Rules) != len(b.Rules) {
		return false
	}
	if len(a.Selector) != len(b.Selector) {
		return false
	}
	for k, v := range a.Selector {
		if b.Selector[k] != v {
			return false
		}
	}
	return true
}
