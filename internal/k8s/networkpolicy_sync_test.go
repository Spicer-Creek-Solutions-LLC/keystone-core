package k8s

import (
	"context"
	stdsync "sync"
	"testing"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestDefaultNetworkPolicySyncConfig(t *testing.T) {
	config := DefaultNetworkPolicySyncConfig()

	if config.SyncInterval != 1*time.Minute {
		t.Errorf("SyncInterval = %v, want 1m", config.SyncInterval)
	}
	if !config.VerifyOnSync {
		t.Error("Expected VerifyOnSync to be true")
	}
	if !config.ContinueOnError {
		t.Error("Expected ContinueOnError to be true")
	}
}

func TestNewNetworkPolicySynchronizerWithClient(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	config := &NetworkPolicySyncConfig{
		SyncInterval:    30 * time.Second,
		VerifyOnSync:    true,
		ContinueOnError: true,
	}

	sync := NewNetworkPolicySynchronizerWithClient(config, client)

	if sync == nil {
		t.Fatal("Expected synchronizer to be non-nil")
	}
	if sync.config != config {
		t.Error("Config not set correctly")
	}
	if sync.client != client {
		t.Error("Client not set correctly")
	}
	if sync.verifier == nil {
		t.Error("Verifier should be initialized")
	}
	if sync.store == nil {
		t.Error("Store should be initialized")
	}
	if sync.knownPolicies == nil {
		t.Error("knownPolicies should be initialized")
	}
}

func TestNewNetworkPolicySynchronizerWithClient_NilConfig(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	sync := NewNetworkPolicySynchronizerWithClient(nil, client)

	if sync == nil {
		t.Fatal("Expected synchronizer to be non-nil")
	}
	if sync.config.SyncInterval != 1*time.Minute {
		t.Error("Expected default config to be used")
	}
}

func TestNetworkPolicySynchronizer_StartStop(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	config := &NetworkPolicySyncConfig{
		SyncInterval:    100 * time.Millisecond,
		VerifyOnSync:    false,
		ContinueOnError: true,
	}

	sync := NewNetworkPolicySynchronizerWithClient(config, client)

	// Test that it's not running initially
	if sync.IsRunning() {
		t.Error("Synchronizer should not be running initially")
	}

	// Start the synchronizer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := sync.Start(ctx)
	if err != nil {
		t.Fatalf("Start error = %v", err)
	}

	if !sync.IsRunning() {
		t.Error("Synchronizer should be running after Start")
	}

	// Try to start again - should error
	err = sync.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting already running synchronizer")
	}

	// Stop the synchronizer
	err = sync.Stop()
	if err != nil {
		t.Fatalf("Stop error = %v", err)
	}

	if sync.IsRunning() {
		t.Error("Synchronizer should not be running after Stop")
	}

	// Stop again should be no-op
	err = sync.Stop()
	if err != nil {
		t.Fatalf("Second Stop error = %v", err)
	}
}

func TestNetworkPolicySynchronizer_SyncNow(t *testing.T) {
	// Create test policies
	np1 := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}
	np2 := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-2",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{},
		},
	}

	fakeClient := fake.NewSimpleClientset(np1, np2)
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	config := &NetworkPolicySyncConfig{
		SyncInterval:    1 * time.Minute,
		Namespaces:      []string{"default"},
		VerifyOnSync:    false, // Disable verification for simpler testing
		ContinueOnError: true,
	}

	sync := NewNetworkPolicySynchronizerWithClient(config, client)

	result, err := sync.SyncNow(context.Background())
	if err != nil {
		t.Fatalf("SyncNow error = %v", err)
	}

	if result == nil {
		t.Fatal("Expected result to be non-nil")
	}

	if result.PoliciesFound != 2 {
		t.Errorf("PoliciesFound = %d, want 2", result.PoliciesFound)
	}
	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}
	if result.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestNetworkPolicySynchronizer_SyncNow_AllNamespaces(t *testing.T) {
	np1 := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "ns1",
		},
		Spec: networkingv1.NetworkPolicySpec{},
	}
	np2 := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-2",
			Namespace: "ns2",
		},
		Spec: networkingv1.NetworkPolicySpec{},
	}

	fakeClient := fake.NewSimpleClientset(np1, np2)
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	config := &NetworkPolicySyncConfig{
		SyncInterval:    1 * time.Minute,
		Namespaces:      []string{}, // Empty means all namespaces
		VerifyOnSync:    false,
		ContinueOnError: true,
	}

	sync := NewNetworkPolicySynchronizerWithClient(config, client)

	result, err := sync.SyncNow(context.Background())
	if err != nil {
		t.Fatalf("SyncNow error = %v", err)
	}

	if result.PoliciesFound != 2 {
		t.Errorf("PoliciesFound = %d, want 2 (from all namespaces)", result.PoliciesFound)
	}
}

func TestNetworkPolicySynchronizer_GetLastSyncResult(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	sync := NewNetworkPolicySynchronizerWithClient(DefaultNetworkPolicySyncConfig(), client)

	// Before any sync
	result, err := sync.GetLastSyncResult()
	if result != nil {
		t.Error("Expected nil result before first sync")
	}
	if err != nil {
		t.Error("Expected nil error before first sync")
	}

	// After sync
	sync.SyncNow(context.Background())

	result, err = sync.GetLastSyncResult()
	if result == nil {
		t.Error("Expected result after sync")
	}
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
}

func TestNetworkPolicySynchronizer_OnPolicyChange(t *testing.T) {
	np1 := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(np1)
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	config := &NetworkPolicySyncConfig{
		Namespaces:      []string{"default"},
		VerifyOnSync:    false,
		ContinueOnError: true,
	}
	sync := NewNetworkPolicySynchronizerWithClient(config, client)

	var events []NetworkPolicyChangeEvent
	var mu stdsync.Mutex

	sync.OnPolicyChange(func(event NetworkPolicyChangeEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// First sync - should emit "added" events
	sync.SyncNow(context.Background())

	mu.Lock()
	if len(events) == 0 {
		t.Error("Expected at least one policy change event")
	}

	foundAdded := false
	for _, e := range events {
		if e.Type == "added" {
			foundAdded = true
			break
		}
	}
	mu.Unlock()

	if !foundAdded {
		t.Error("Expected 'added' event for new policy")
	}
}

func TestNetworkPolicySynchronizer_OnSyncComplete(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	sync := NewNetworkPolicySynchronizerWithClient(DefaultNetworkPolicySyncConfig(), client)

	var completedResult *NetworkPolicySyncResult
	sync.OnSyncComplete(func(result *NetworkPolicySyncResult) {
		completedResult = result
	})

	sync.SyncNow(context.Background())

	if completedResult == nil {
		t.Error("Expected OnSyncComplete callback to be called")
	}
}

func TestNetworkPolicySynchronizer_GetStore(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	sync := NewNetworkPolicySynchronizerWithClient(DefaultNetworkPolicySyncConfig(), client)

	store := sync.GetStore()
	if store == nil {
		t.Error("Expected store to be non-nil")
	}
}

func TestNetworkPolicySynchronizer_GetVerifier(t *testing.T) {
	fakeClient := fake.NewSimpleClientset()
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	sync := NewNetworkPolicySynchronizerWithClient(DefaultNetworkPolicySyncConfig(), client)

	verifier := sync.GetVerifier()
	if verifier == nil {
		t.Error("Expected verifier to be non-nil")
	}
}

func TestNetworkPolicySynchronizer_ChangeDetection(t *testing.T) {
	np1 := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(np1)
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	config := &NetworkPolicySyncConfig{
		Namespaces:      []string{"default"},
		VerifyOnSync:    false,
		ContinueOnError: true,
	}
	sync := NewNetworkPolicySynchronizerWithClient(config, client)

	var events []NetworkPolicyChangeEvent
	var mu stdsync.Mutex

	sync.OnPolicyChange(func(event NetworkPolicyChangeEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	// First sync - should see "added"
	sync.SyncNow(context.Background())

	mu.Lock()
	addedCount := 0
	for _, e := range events {
		if e.Type == "added" {
			addedCount++
		}
	}
	mu.Unlock()

	if addedCount == 0 {
		t.Error("Expected 'added' event on first sync")
	}

	// Second sync with same data - should see no new events
	eventsBefore := len(events)
	sync.SyncNow(context.Background())

	mu.Lock()
	eventsAfter := len(events)
	mu.Unlock()

	if eventsAfter != eventsBefore {
		t.Errorf("Expected no new events on second sync with same data, got %d new events", eventsAfter-eventsBefore)
	}
}

func TestNetworkPolicySyncResult_Types(t *testing.T) {
	result := &NetworkPolicySyncResult{
		Timestamp:        time.Now(),
		PoliciesFound:    10,
		PoliciesVerified: 8,
		PoliciesPassed:   7,
		PoliciesFailed:   1,
		Errors: []NetworkPolicySyncError{
			{
				PolicyName: "test-policy",
				Namespace:  "default",
				Error:      "test error",
				Timestamp:  time.Now(),
			},
		},
		Duration: 100 * time.Millisecond,
	}

	if result.PoliciesFound != 10 {
		t.Error("PoliciesFound not set correctly")
	}
	if len(result.Errors) != 1 {
		t.Error("Errors not set correctly")
	}
	if result.Errors[0].PolicyName != "test-policy" {
		t.Error("Error PolicyName not set correctly")
	}
}

func TestNetworkPolicyChangeEvent_Types(t *testing.T) {
	event := NetworkPolicyChangeEvent{
		Type: "added",
		Policy: &NetworkPolicy{
			Name:      "test-policy",
			Namespace: "default",
		},
		Timestamp: time.Now(),
	}

	if event.Type != "added" {
		t.Error("Type not set correctly")
	}
	if event.Policy.Name != "test-policy" {
		t.Error("Policy not set correctly")
	}
}

func TestNetworkPoliciesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *NetworkPolicy
		b        *NetworkPolicy
		expected bool
	}{
		{
			name:     "both nil",
			a:        nil,
			b:        nil,
			expected: true,
		},
		{
			name:     "one nil",
			a:        &NetworkPolicy{Name: "test"},
			b:        nil,
			expected: false,
		},
		{
			name: "equal policies",
			a: &NetworkPolicy{
				Name:      "test",
				Namespace: "default",
				Spec: NetworkPolicySpec{
					PodSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			},
			b: &NetworkPolicy{
				Name:      "test",
				Namespace: "default",
				Spec: NetworkPolicySpec{
					PodSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "test"},
					},
				},
			},
			expected: true,
		},
		{
			name: "different spec",
			a: &NetworkPolicy{
				Name:      "test",
				Namespace: "default",
				Spec: NetworkPolicySpec{
					PodSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "test1"},
					},
				},
			},
			b: &NetworkPolicy{
				Name:      "test",
				Namespace: "default",
				Spec: NetworkPolicySpec{
					PodSelector: LabelSelector{
						MatchLabels: map[string]string{"app": "test2"},
					},
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := networkPoliciesEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("networkPoliciesEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestPolicyKey(t *testing.T) {
	tests := []struct {
		namespace string
		name      string
		expected  string
	}{
		{"default", "policy1", "default/policy1"},
		{"", "policy1", "policy1"},
		{"kube-system", "allow-dns", "kube-system/allow-dns"},
	}

	for _, tt := range tests {
		result := policyKey(tt.namespace, tt.name)
		if result != tt.expected {
			t.Errorf("policyKey(%q, %q) = %q, want %q", tt.namespace, tt.name, result, tt.expected)
		}
	}
}

func TestNetworkPolicySynchronizer_SyncWithVerification(t *testing.T) {
	np1 := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "policy-1",
			Namespace: "default",
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
		},
	}

	fakeClient := fake.NewSimpleClientset(np1)
	client := NewClientWithInterface(fakeClient, ClusterConfig{})

	config := &NetworkPolicySyncConfig{
		Namespaces:      []string{"default"},
		VerifyOnSync:    true, // Enable verification
		ContinueOnError: true,
	}
	sync := NewNetworkPolicySynchronizerWithClient(config, client)

	result, err := sync.SyncNow(context.Background())
	if err != nil {
		t.Fatalf("SyncNow error = %v", err)
	}

	// Should have found and verified policies
	if result.PoliciesFound != 1 {
		t.Errorf("PoliciesFound = %d, want 1", result.PoliciesFound)
	}

	// Verification should have been attempted
	if result.PoliciesVerified == 0 {
		t.Error("Expected verification to be attempted")
	}
}
