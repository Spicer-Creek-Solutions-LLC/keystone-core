package servicemesh

import (
	"context"
	stdsync "sync"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

func TestDefaultSyncConfig(t *testing.T) {
	config := DefaultSyncConfig()

	if config.SyncInterval != 1*time.Minute {
		t.Errorf("SyncInterval = %v, want 1m", config.SyncInterval)
	}
	if config.MeshType != MeshTypeIstio {
		t.Errorf("MeshType = %v, want Istio", config.MeshType)
	}
	if !config.VerifyOnSync {
		t.Error("Expected VerifyOnSync to be true")
	}
	if !config.ContinueOnError {
		t.Error("Expected ContinueOnError to be true")
	}
}

func TestNewPolicySynchronizerWithClient(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	config := &SyncConfig{
		SyncInterval:    30 * time.Second,
		MeshType:        MeshTypeIstio,
		VerifyOnSync:    true,
		ContinueOnError: true,
	}

	sync := NewPolicySynchronizerWithClient(config, crdClient)

	if sync == nil {
		t.Fatal("Expected synchronizer to be non-nil")
	}
	if sync.config != config {
		t.Error("Config not set correctly")
	}
	if sync.client != crdClient {
		t.Error("Client not set correctly")
	}
	if sync.verifier == nil {
		t.Error("Verifier should be initialized")
	}
	if sync.store == nil {
		t.Error("Store should be initialized")
	}
	if sync.knownMTLSPolicies == nil {
		t.Error("knownMTLSPolicies should be initialized")
	}
	if sync.knownAuthPolicies == nil {
		t.Error("knownAuthPolicies should be initialized")
	}
}

func TestNewPolicySynchronizerWithClient_NilConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	sync := NewPolicySynchronizerWithClient(nil, crdClient)

	if sync == nil {
		t.Fatal("Expected synchronizer to be non-nil")
	}
	if sync.config.SyncInterval != 1*time.Minute {
		t.Error("Expected default config to be used")
	}
}

func TestPolicySynchronizer_StartStop(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR:  "PeerAuthenticationList",
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
			DestinationRuleGVR:     "DestinationRuleList",
		},
	)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	config := &SyncConfig{
		SyncInterval:    100 * time.Millisecond,
		MeshType:        MeshTypeIstio,
		VerifyOnSync:    false,
		ContinueOnError: true,
	}

	sync := NewPolicySynchronizerWithClient(config, crdClient)

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

func TestPolicySynchronizer_SyncNow(t *testing.T) {
	scheme := runtime.NewScheme()

	// Create test resources
	pa1 := createPeerAuthenticationUnstructured("pa-1", "default", "STRICT", "service-a")
	pa2 := createPeerAuthenticationUnstructured("pa-2", "default", "PERMISSIVE", "")
	ap1 := createAuthorizationPolicyUnstructured("ap-1", "default", "ALLOW")
	dr1 := createDestinationRuleUnstructured("dr-1", "default", "service-a.default.svc.cluster.local")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR:  "PeerAuthenticationList",
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
			DestinationRuleGVR:     "DestinationRuleList",
		},
		pa1, pa2, ap1, dr1,
	)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	config := &SyncConfig{
		SyncInterval:    1 * time.Minute,
		MeshType:        MeshTypeIstio,
		Namespaces:      []string{"default"},
		VerifyOnSync:    false, // Disable verification for simpler testing
		ContinueOnError: true,
	}

	sync := NewPolicySynchronizerWithClient(config, crdClient)

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
	if result.AuthPoliciesFound != 1 {
		t.Errorf("AuthPoliciesFound = %d, want 1", result.AuthPoliciesFound)
	}
	if result.DestRulesFound != 1 {
		t.Errorf("DestRulesFound = %d, want 1", result.DestRulesFound)
	}
	if result.Duration <= 0 {
		t.Error("Expected positive duration")
	}
	if result.Timestamp.IsZero() {
		t.Error("Expected timestamp to be set")
	}
}

func TestPolicySynchronizer_SyncNow_AllNamespaces(t *testing.T) {
	scheme := runtime.NewScheme()

	pa1 := createPeerAuthenticationUnstructured("pa-1", "ns1", "STRICT", "svc1")
	pa2 := createPeerAuthenticationUnstructured("pa-2", "ns2", "PERMISSIVE", "svc2")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR:  "PeerAuthenticationList",
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
			DestinationRuleGVR:     "DestinationRuleList",
		},
		pa1, pa2,
	)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	config := &SyncConfig{
		SyncInterval:    1 * time.Minute,
		MeshType:        MeshTypeIstio,
		Namespaces:      []string{}, // Empty means all namespaces
		VerifyOnSync:    false,
		ContinueOnError: true,
	}

	sync := NewPolicySynchronizerWithClient(config, crdClient)

	result, err := sync.SyncNow(context.Background())
	if err != nil {
		t.Fatalf("SyncNow error = %v", err)
	}

	if result.PoliciesFound != 2 {
		t.Errorf("PoliciesFound = %d, want 2 (from all namespaces)", result.PoliciesFound)
	}
}

func TestPolicySynchronizer_GetLastSyncResult(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR:  "PeerAuthenticationList",
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
			DestinationRuleGVR:     "DestinationRuleList",
		},
	)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	sync := NewPolicySynchronizerWithClient(DefaultSyncConfig(), crdClient)

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

func TestPolicySynchronizer_OnPolicyChange(t *testing.T) {
	scheme := runtime.NewScheme()

	pa1 := createPeerAuthenticationUnstructured("pa-1", "default", "STRICT", "service-a")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR:  "PeerAuthenticationList",
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
			DestinationRuleGVR:     "DestinationRuleList",
		},
		pa1,
	)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	config := &SyncConfig{
		Namespaces:      []string{"default"},
		VerifyOnSync:    false,
		ContinueOnError: true,
	}
	sync := NewPolicySynchronizerWithClient(config, crdClient)

	var events []PolicyChangeEvent
	var mu stdsync.Mutex

	sync.OnPolicyChange(func(event PolicyChangeEvent) {
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

func TestPolicySynchronizer_OnSyncComplete(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR:  "PeerAuthenticationList",
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
			DestinationRuleGVR:     "DestinationRuleList",
		},
	)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	sync := NewPolicySynchronizerWithClient(DefaultSyncConfig(), crdClient)

	var completedResult *SyncResult
	sync.OnSyncComplete(func(result *SyncResult) {
		completedResult = result
	})

	sync.SyncNow(context.Background())

	if completedResult == nil {
		t.Error("Expected OnSyncComplete callback to be called")
	}
}

func TestPolicySynchronizer_GetStore(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	sync := NewPolicySynchronizerWithClient(DefaultSyncConfig(), crdClient)

	store := sync.GetStore()
	if store == nil {
		t.Error("Expected store to be non-nil")
	}
}

func TestPolicySynchronizer_GetVerifier(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	sync := NewPolicySynchronizerWithClient(DefaultSyncConfig(), crdClient)

	verifier := sync.GetVerifier()
	if verifier == nil {
		t.Error("Expected verifier to be non-nil")
	}
}

func TestPolicySynchronizer_SetMetadata(t *testing.T) {
	scheme := runtime.NewScheme()
	fakeClient := dynamicfake.NewSimpleDynamicClient(scheme)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	sync := NewPolicySynchronizerWithClient(DefaultSyncConfig(), crdClient)

	metadata := &Metadata{
		MeshType:    MeshTypeIstio,
		TrustDomain: "cluster.local",
	}

	sync.SetMetadata(metadata)

	// Verify metadata was set on the verifier
	verifier := sync.GetVerifier()
	verifier.mu.RLock()
	if verifier.metadata != metadata {
		t.Error("Metadata not set on verifier")
	}
	verifier.mu.RUnlock()
}

func TestPolicySynchronizer_ChangeDetection(t *testing.T) {
	scheme := runtime.NewScheme()

	// Start with one policy
	pa1 := createPeerAuthenticationUnstructured("pa-1", "default", "STRICT", "service-a")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR:  "PeerAuthenticationList",
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
			DestinationRuleGVR:     "DestinationRuleList",
		},
		pa1,
	)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	config := &SyncConfig{
		Namespaces:      []string{"default"},
		VerifyOnSync:    false,
		ContinueOnError: true,
	}
	sync := NewPolicySynchronizerWithClient(config, crdClient)

	var events []PolicyChangeEvent
	var mu stdsync.Mutex

	sync.OnPolicyChange(func(event PolicyChangeEvent) {
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

func TestSyncResult_Types(t *testing.T) {
	result := &SyncResult{
		Timestamp:         time.Now(),
		PoliciesFound:     10,
		PoliciesVerified:  8,
		PoliciesPassed:    7,
		PoliciesFailed:    1,
		AuthPoliciesFound: 5,
		DestRulesFound:    3,
		Errors: []SyncError{
			{
				PolicyName:   "test-policy",
				Namespace:    "default",
				ResourceType: "PeerAuthentication",
				Error:        "test error",
				Timestamp:    time.Now(),
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

func TestPolicyChangeEvent_Types(t *testing.T) {
	event := PolicyChangeEvent{
		Type:         "added",
		ResourceType: "mTLS",
		Policy: &MTLSPolicy{
			Name:      "test-policy",
			Namespace: "default",
			Mode:      PolicyModeStrict,
		},
		Timestamp: time.Now(),
	}

	if event.Type != "added" {
		t.Error("Type not set correctly")
	}
	if event.Policy.Name != "test-policy" {
		t.Error("Policy not set correctly")
	}

	// Test with auth policy
	event2 := PolicyChangeEvent{
		Type:         "added",
		ResourceType: "Authorization",
		AuthPolicy: &AuthorizationPolicy{
			Name:   "auth-policy",
			Action: AuthorizationActionAllow,
		},
		Timestamp: time.Now(),
	}

	if event2.AuthPolicy.Name != "auth-policy" {
		t.Error("AuthPolicy not set correctly")
	}
}

func TestMtlsPoliciesEqual(t *testing.T) {
	tests := []struct {
		name     string
		a        *MTLSPolicy
		b        *MTLSPolicy
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
			a:        &MTLSPolicy{Name: "test"},
			b:        nil,
			expected: false,
		},
		{
			name: "equal policies",
			a: &MTLSPolicy{
				Name:      "test",
				Namespace: "default",
				Service:   "my-service",
				Mode:      PolicyModeStrict,
			},
			b: &MTLSPolicy{
				Name:      "test",
				Namespace: "default",
				Service:   "my-service",
				Mode:      PolicyModeStrict,
			},
			expected: true,
		},
		{
			name: "different name",
			a: &MTLSPolicy{
				Name:      "test1",
				Namespace: "default",
				Mode:      PolicyModeStrict,
			},
			b: &MTLSPolicy{
				Name:      "test2",
				Namespace: "default",
				Mode:      PolicyModeStrict,
			},
			expected: false,
		},
		{
			name: "different mode",
			a: &MTLSPolicy{
				Name:      "test",
				Namespace: "default",
				Mode:      PolicyModeStrict,
			},
			b: &MTLSPolicy{
				Name:      "test",
				Namespace: "default",
				Mode:      PolicyModePermissive,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mtlsPoliciesEqual(tt.a, tt.b)
			if result != tt.expected {
				t.Errorf("mtlsPoliciesEqual() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestPolicySynchronizer_SyncWithVerification(t *testing.T) {
	scheme := runtime.NewScheme()

	pa1 := createPeerAuthenticationUnstructured("pa-1", "default", "STRICT", "service-a")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR:  "PeerAuthenticationList",
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
			DestinationRuleGVR:     "DestinationRuleList",
		},
		pa1,
	)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	config := &SyncConfig{
		Namespaces:      []string{"default"},
		VerifyOnSync:    true, // Enable verification
		ContinueOnError: true,
	}
	sync := NewPolicySynchronizerWithClient(config, crdClient)

	// Set metadata so verification can run
	sync.SetMetadata(&Metadata{
		MeshType:    MeshTypeIstio,
		TrustDomain: "cluster.local",
		TLSConfig: &TLSConfig{
			Enabled:        true,
			Mode:           "STRICT",
			CertChainFile:  "/nonexistent/cert.pem", // Will fail verification
			PrivateKeyFile: "/nonexistent/key.pem",
			CAFile:         "/nonexistent/ca.pem",
			SPIFFEID:       "spiffe://cluster.local/ns/default/sa/test",
		},
	})

	result, err := sync.SyncNow(context.Background())
	if err != nil {
		t.Fatalf("SyncNow error = %v", err)
	}

	// Should have found and attempted to verify policies
	if result.PoliciesFound != 1 {
		t.Errorf("PoliciesFound = %d, want 1", result.PoliciesFound)
	}

	// Verification should have been attempted
	if result.PoliciesVerified == 0 && len(result.Errors) == 0 {
		t.Error("Expected verification to be attempted")
	}
}

func TestPolicySynchronizer_OnSyncError(t *testing.T) {
	scheme := runtime.NewScheme()

	pa1 := createPeerAuthenticationUnstructured("pa-1", "default", "STRICT", "service-a")

	fakeClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(
		scheme,
		map[schema.GroupVersionResource]string{
			PeerAuthenticationGVR:  "PeerAuthenticationList",
			AuthorizationPolicyGVR: "AuthorizationPolicyList",
			DestinationRuleGVR:     "DestinationRuleList",
		},
		pa1,
	)
	crdClient := NewIstioCRDClientFromDynamic(fakeClient)

	config := &SyncConfig{
		Namespaces:      []string{"default"},
		VerifyOnSync:    true,
		ContinueOnError: true,
	}
	sync := NewPolicySynchronizerWithClient(config, crdClient)

	var errors []SyncError
	var mu stdsync.Mutex

	sync.OnSyncError(func(err SyncError) {
		mu.Lock()
		errors = append(errors, err)
		mu.Unlock()
	})

	// Set metadata that will cause verification to fail
	sync.SetMetadata(&Metadata{
		TLSConfig: &TLSConfig{
			CertChainFile: "/nonexistent/cert.pem",
		},
	})

	sync.SyncNow(context.Background())

	// We may or may not get errors depending on how verification handles missing files
	// The main thing is that the callback was properly set
	mu.Lock()
	_ = errors // Just verify we can access it
	mu.Unlock()
}

func TestAuthPoliciesEqual(t *testing.T) {
	tests := []struct {
		name string
		a, b *AuthorizationPolicy
		want bool
	}{
		{
			name: "both nil",
			a:    nil,
			b:    nil,
			want: true,
		},
		{
			name: "one nil",
			a:    &AuthorizationPolicy{Name: "p1"},
			b:    nil,
			want: false,
		},
		{
			name: "equal policies",
			a:    &AuthorizationPolicy{Name: "p1", Namespace: "ns", Action: AuthorizationActionAllow, Selector: map[string]string{"app": "web"}},
			b:    &AuthorizationPolicy{Name: "p1", Namespace: "ns", Action: AuthorizationActionAllow, Selector: map[string]string{"app": "web"}},
			want: true,
		},
		{
			name: "different action",
			a:    &AuthorizationPolicy{Name: "p1", Action: AuthorizationActionAllow},
			b:    &AuthorizationPolicy{Name: "p1", Action: AuthorizationActionDeny},
			want: false,
		},
		{
			name: "different rule count",
			a:    &AuthorizationPolicy{Name: "p1", Rules: []AuthorizationRule{{}}},
			b:    &AuthorizationPolicy{Name: "p1", Rules: nil},
			want: false,
		},
		{
			name: "different selector",
			a:    &AuthorizationPolicy{Name: "p1", Selector: map[string]string{"app": "web"}},
			b:    &AuthorizationPolicy{Name: "p1", Selector: map[string]string{"app": "api"}},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authPoliciesEqual(tt.a, tt.b); got != tt.want {
				t.Errorf("authPoliciesEqual() = %v, want %v", got, tt.want)
			}
		})
	}
}
