package servicemesh

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPolicyMode_String(t *testing.T) {
	tests := []struct {
		mode PolicyMode
		want string
	}{
		{PolicyModeStrict, "STRICT"},
		{PolicyModePermissive, "PERMISSIVE"},
		{PolicyModeDisable, "DISABLE"},
	}

	for _, tt := range tests {
		if string(tt.mode) != tt.want {
			t.Errorf("PolicyMode string = %q, want %q", tt.mode, tt.want)
		}
	}
}

func TestNewPolicyVerifier(t *testing.T) {
	verifier := NewPolicyVerifier(MeshTypeIstio, nil)
	if verifier == nil {
		t.Fatal("Expected verifier to be non-nil")
	}
	if verifier.meshType != MeshTypeIstio {
		t.Errorf("meshType = %v, want %v", verifier.meshType, MeshTypeIstio)
	}
}

func TestPolicyVerifier_SetMetadata(t *testing.T) {
	verifier := NewPolicyVerifier(MeshTypeIstio, nil)

	metadata := &Metadata{
		MeshType: MeshTypeIstio,
		TLSConfig: &TLSConfig{
			Enabled: true,
			Mode:    "STRICT",
		},
	}

	verifier.SetMetadata(metadata)

	verifier.mu.RLock()
	defer verifier.mu.RUnlock()
	if verifier.metadata != metadata {
		t.Error("Metadata not set correctly")
	}
}

func TestPolicyVerifier_VerifyPolicy_NoMetadata(t *testing.T) {
	verifier := NewPolicyVerifier(MeshTypeIstio, nil)

	policy := &MTLSPolicy{
		Name: "test-policy",
		Mode: PolicyModeStrict,
	}

	result, err := verifier.VerifyPolicy(context.Background(), policy)
	if err != nil {
		t.Fatalf("VerifyPolicy error = %v", err)
	}

	// Should have run checks but many will fail without metadata
	if len(result.Checks) == 0 {
		t.Error("Expected checks to be run")
	}

	// Should not pass without metadata
	if result.Passed {
		t.Error("Expected verification to fail without metadata")
	}
}

func TestPolicyVerifier_VerifyPolicy_WithMetadata(t *testing.T) {
	// Create temp directory for test certificates
	tmpDir := t.TempDir()

	// Create dummy certificate files
	certFile := filepath.Join(tmpDir, "cert-chain.pem")
	keyFile := filepath.Join(tmpDir, "key.pem")
	caFile := filepath.Join(tmpDir, "root-cert.pem")

	// Create a self-signed test certificate
	certPEM := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpegPjMCMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
c3RjYTAeFw0yNDAxMDEwMDAwMDBaFw0yNTAxMDEwMDAwMDBaMBExDzANBgNVBAMM
BnRlc3RjYTBcMA0GCSqGSIb3DQEBAQUAA0sAMEgCQQC7o96FCFofjHLdVGqvGMpA
JlAhMFFbPZHFaM0F8gYRPCUZC5R+3XQbg2N7xJnkHcJDLLcvYGnH5M/hxU4qnMk9
AgMBAAGjUzBRMB0GA1UdDgQWBBTjzZjjWgmKNTF5J3fYB0fQQzXj0TAfBgNVHSME
GDAWgBTjzZjjWgmKNTF5J3fYB0fQQzXj0TAPBgNVHRMBAf8EBTADAQH/MA0GCSqG
SIb3DQEBCwUAA0EA3nZQXwDB8M0SWOhiMJkP8GvJ9r6nF/rqE6D2X/cMQ0gQ0hL8
7sSHRpFMVFULAjZXYUaM5fCVbOQ7S7HQRJXM9w==
-----END CERTIFICATE-----`

	if err := os.WriteFile(certFile, []byte(certPEM), 0644); err != nil {
		t.Fatalf("Failed to write cert file: %v", err)
	}
	if err := os.WriteFile(keyFile, []byte("dummy key"), 0644); err != nil {
		t.Fatalf("Failed to write key file: %v", err)
	}
	if err := os.WriteFile(caFile, []byte(certPEM), 0644); err != nil {
		t.Fatalf("Failed to write ca file: %v", err)
	}

	verifier := NewPolicyVerifier(MeshTypeIstio, nil)

	metadata := &Metadata{
		MeshType:    MeshTypeIstio,
		TrustDomain: "cluster.local",
		TLSConfig: &TLSConfig{
			Enabled:        true,
			Mode:           "STRICT",
			CertChainFile:  certFile,
			PrivateKeyFile: keyFile,
			CAFile:         caFile,
			SPIFFEID:       "spiffe://cluster.local/ns/default/sa/test",
		},
	}
	verifier.SetMetadata(metadata)

	policy := &MTLSPolicy{
		Name: "test-policy",
		Mode: PolicyModeStrict,
	}

	result, err := verifier.VerifyPolicy(context.Background(), policy)
	if err != nil {
		t.Fatalf("VerifyPolicy error = %v", err)
	}

	// Check that all checks ran
	expectedChecks := 7
	if len(result.Checks) != expectedChecks {
		t.Errorf("Expected %d checks, got %d", expectedChecks, len(result.Checks))
	}

	// Log check results for debugging
	for _, check := range result.Checks {
		t.Logf("Check %s: passed=%v, message=%s", check.Name, check.Passed, check.Message)
	}
}

func TestPolicyVerifier_CheckCertificateExists(t *testing.T) {
	verifier := NewPolicyVerifier(MeshTypeIstio, nil)

	// Test without metadata
	policy := &MTLSPolicy{Mode: PolicyModeStrict}
	check := verifier.checkCertificateExists(policy)
	if check.Passed {
		t.Error("Expected check to fail without metadata")
	}

	// Test with non-existent files
	verifier.SetMetadata(&Metadata{
		TLSConfig: &TLSConfig{
			CertChainFile: "/nonexistent/cert.pem",
		},
	})
	check = verifier.checkCertificateExists(policy)
	if check.Passed {
		t.Error("Expected check to fail with non-existent file")
	}
}

func TestPolicyVerifier_CheckPolicyModeConsistency(t *testing.T) {
	verifier := NewPolicyVerifier(MeshTypeIstio, nil)

	// Set metadata with STRICT mode
	verifier.SetMetadata(&Metadata{
		TLSConfig: &TLSConfig{
			Enabled: true,
			Mode:    "STRICT",
		},
	})

	// Test matching mode
	policy := &MTLSPolicy{Mode: PolicyModeStrict}
	check := verifier.checkPolicyModeConsistency(policy)
	if !check.Passed {
		t.Errorf("Expected check to pass with matching mode, got: %s", check.Message)
	}

	// Test mismatching mode
	policy = &MTLSPolicy{Mode: PolicyModePermissive}
	check = verifier.checkPolicyModeConsistency(policy)
	if check.Passed {
		t.Error("Expected check to fail with mismatching mode")
	}
}

func TestPolicyVerifier_CheckSPIFFEIdentity(t *testing.T) {
	verifier := NewPolicyVerifier(MeshTypeIstio, nil)

	// Test without SPIFFE ID
	verifier.SetMetadata(&Metadata{
		TLSConfig: &TLSConfig{
			SPIFFEID: "",
		},
	})
	policy := &MTLSPolicy{Mode: PolicyModeStrict}
	check := verifier.checkSPIFFEIdentity(policy)
	if check.Passed {
		t.Error("Expected check to fail without SPIFFE ID")
	}

	// Test with invalid SPIFFE ID format
	verifier.SetMetadata(&Metadata{
		TLSConfig: &TLSConfig{
			SPIFFEID: "invalid-format",
		},
	})
	check = verifier.checkSPIFFEIdentity(policy)
	if check.Passed {
		t.Error("Expected check to fail with invalid SPIFFE ID format")
	}

	// Test with valid SPIFFE ID
	verifier.SetMetadata(&Metadata{
		TrustDomain: "cluster.local",
		TLSConfig: &TLSConfig{
			SPIFFEID: "spiffe://cluster.local/ns/default/sa/test",
		},
	})
	check = verifier.checkSPIFFEIdentity(policy)
	if !check.Passed {
		t.Errorf("Expected check to pass with valid SPIFFE ID, got: %s", check.Message)
	}

	// Test with mismatched trust domain
	verifier.SetMetadata(&Metadata{
		TrustDomain: "different.domain",
		TLSConfig: &TLSConfig{
			SPIFFEID: "spiffe://cluster.local/ns/default/sa/test",
		},
	})
	check = verifier.checkSPIFFEIdentity(policy)
	if check.Passed {
		t.Error("Expected check to fail with mismatched trust domain")
	}
}

func TestPolicyVerifier_CheckTLSConfiguration(t *testing.T) {
	verifier := NewPolicyVerifier(MeshTypeIstio, nil)

	// Test STRICT mode without required files
	verifier.SetMetadata(&Metadata{
		TLSConfig: &TLSConfig{
			Enabled:       true,
			CertChainFile: "",
		},
	})
	policy := &MTLSPolicy{Mode: PolicyModeStrict}
	check := verifier.checkTLSConfiguration(policy)
	if check.Passed {
		t.Error("Expected check to fail for STRICT mode without cert chain")
	}

	// Test with all required files for STRICT mode
	verifier.SetMetadata(&Metadata{
		TLSConfig: &TLSConfig{
			Enabled:        true,
			CertChainFile:  "/path/to/cert.pem",
			PrivateKeyFile: "/path/to/key.pem",
			CAFile:         "/path/to/ca.pem",
		},
	})
	check = verifier.checkTLSConfiguration(policy)
	if !check.Passed {
		t.Errorf("Expected check to pass with all required files, got: %s", check.Message)
	}

	// Test PERMISSIVE mode (less strict requirements)
	policy = &MTLSPolicy{Mode: PolicyModePermissive}
	verifier.SetMetadata(&Metadata{
		TLSConfig: &TLSConfig{
			Enabled: true,
		},
	})
	check = verifier.checkTLSConfiguration(policy)
	if !check.Passed {
		t.Errorf("Expected check to pass for PERMISSIVE mode, got: %s", check.Message)
	}
}

func TestNewPolicyStore(t *testing.T) {
	store := NewPolicyStore()
	if store == nil {
		t.Fatal("Expected store to be non-nil")
	}
	if store.policies == nil {
		t.Error("Expected policies map to be initialized")
	}
}

func TestPolicyStore_AddAndGet(t *testing.T) {
	store := NewPolicyStore()

	policy := &MTLSPolicy{
		Name:      "test-policy",
		Namespace: "default",
		Service:   "my-service",
		Mode:      PolicyModeStrict,
	}

	store.Add(policy)

	// Get the policy back
	got, ok := store.Get("default", "my-service")
	if !ok {
		t.Fatal("Expected to find policy")
	}
	if got.Name != policy.Name {
		t.Errorf("Policy name = %q, want %q", got.Name, policy.Name)
	}
	if got.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

func TestPolicyStore_GetNamespaceWide(t *testing.T) {
	store := NewPolicyStore()

	// Add namespace-wide policy
	policy := &MTLSPolicy{
		Name:      "namespace-policy",
		Namespace: "default",
		Service:   "", // Empty means namespace-wide
		Mode:      PolicyModeStrict,
	}
	store.Add(policy)

	// Should find it for any service in the namespace
	got, ok := store.Get("default", "any-service")
	if !ok {
		t.Fatal("Expected to find namespace-wide policy")
	}
	if got.Name != policy.Name {
		t.Errorf("Policy name = %q, want %q", got.Name, policy.Name)
	}
}

func TestPolicyStore_ServiceSpecificOverridesNamespace(t *testing.T) {
	store := NewPolicyStore()

	// Add namespace-wide policy
	nsPolicy := &MTLSPolicy{
		Name:      "namespace-policy",
		Namespace: "default",
		Service:   "",
		Mode:      PolicyModePermissive,
	}
	store.Add(nsPolicy)

	// Add service-specific policy
	svcPolicy := &MTLSPolicy{
		Name:      "service-policy",
		Namespace: "default",
		Service:   "my-service",
		Mode:      PolicyModeStrict,
	}
	store.Add(svcPolicy)

	// Service-specific should be returned
	got, ok := store.Get("default", "my-service")
	if !ok {
		t.Fatal("Expected to find policy")
	}
	if got.Name != "service-policy" {
		t.Errorf("Expected service-specific policy, got %q", got.Name)
	}

	// Other services should get namespace policy
	got, ok = store.Get("default", "other-service")
	if !ok {
		t.Fatal("Expected to find namespace policy")
	}
	if got.Name != "namespace-policy" {
		t.Errorf("Expected namespace policy, got %q", got.Name)
	}
}

func TestPolicyStore_Remove(t *testing.T) {
	store := NewPolicyStore()

	policy := &MTLSPolicy{
		Name:      "test-policy",
		Namespace: "default",
		Service:   "my-service",
		Mode:      PolicyModeStrict,
	}
	store.Add(policy)

	// Remove it
	removed := store.Remove("default", "my-service")
	if !removed {
		t.Error("Expected Remove to return true")
	}

	// Should not find it anymore
	_, ok := store.Get("default", "my-service")
	if ok {
		t.Error("Expected policy to be removed")
	}

	// Remove non-existent
	removed = store.Remove("default", "non-existent")
	if removed {
		t.Error("Expected Remove to return false for non-existent policy")
	}
}

func TestPolicyStore_List(t *testing.T) {
	store := NewPolicyStore()

	policies := []*MTLSPolicy{
		{Name: "policy1", Namespace: "ns1", Service: "svc1", Mode: PolicyModeStrict},
		{Name: "policy2", Namespace: "ns1", Service: "svc2", Mode: PolicyModePermissive},
		{Name: "policy3", Namespace: "ns2", Service: "svc1", Mode: PolicyModeStrict},
	}

	for _, p := range policies {
		store.Add(p)
	}

	list := store.List()
	if len(list) != 3 {
		t.Errorf("Expected 3 policies, got %d", len(list))
	}
}

func TestPolicyStore_GetEffectivePolicy(t *testing.T) {
	store := NewPolicyStore()

	// Test default policy when nothing is configured
	policy := store.GetEffectivePolicy("unknown", "service")
	if policy.Mode != PolicyModeStrict {
		t.Errorf("Expected default STRICT mode, got %s", policy.Mode)
	}

	// Add a default policy
	store.Add(&MTLSPolicy{
		Name:      "default-policy",
		Namespace: "default",
		Mode:      PolicyModePermissive,
	})

	// Test namespace policy
	policy = store.GetEffectivePolicy("default", "any-service")
	if policy.Mode != PolicyModePermissive {
		t.Errorf("Expected PERMISSIVE from namespace policy, got %s", policy.Mode)
	}
}

func TestTLSVersionString(t *testing.T) {
	tests := []struct {
		version uint16
		want    string
	}{
		{0x0301, "TLS 1.0"},
		{0x0302, "TLS 1.1"},
		{0x0303, "TLS 1.2"},
		{0x0304, "TLS 1.3"},
		{0x9999, "0x9999"},
	}

	for _, tt := range tests {
		got := tlsVersionString(tt.version)
		if got != tt.want {
			t.Errorf("tlsVersionString(0x%04x) = %q, want %q", tt.version, got, tt.want)
		}
	}
}

func TestMTLSPolicy_Types(t *testing.T) {
	// Test that all related types can be instantiated
	policy := &MTLSPolicy{
		Name:      "test",
		Namespace: "default",
		Service:   "my-service",
		Port:      443,
		Mode:      PolicyModeStrict,
		PeerAuthentication: &PeerAuthentication{
			Mode: PolicyModeStrict,
			PortLevelMtls: map[int]PolicyMode{
				443: PolicyModeStrict,
				80:  PolicyModePermissive,
			},
		},
		DestinationRule: &DestinationRule{
			Host: "my-service.default.svc.cluster.local",
			TrafficPolicy: &TrafficPolicy{
				TLS: &TLSSettings{
					Mode:              "ISTIO_MUTUAL",
					ClientCertificate: "/certs/cert.pem",
					PrivateKey:        "/certs/key.pem",
					CaCertificates:    "/certs/ca.pem",
					SubjectAltNames:   []string{"my-service"},
				},
				ConnectionPool: &ConnectionPoolSettings{
					TCP: &TCPSettings{
						MaxConnections: 100,
						ConnectTimeout: 5 * time.Second,
					},
					HTTP: &HTTPSettings{
						HTTP1MaxPendingRequests:  100,
						HTTP2MaxRequests:         1000,
						MaxRequestsPerConnection: 100,
						MaxRetries:               3,
					},
				},
				OutlierDetection: &OutlierDetection{
					Consecutive5xxErrors:     5,
					ConsecutiveGatewayErrors: 5,
					Interval:                 10 * time.Second,
					BaseEjectionTime:         30 * time.Second,
					MaxEjectionPercent:       10,
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if policy.Name != "test" {
		t.Error("Policy not initialized correctly")
	}

	if policy.PeerAuthentication.PortLevelMtls[443] != PolicyModeStrict {
		t.Error("Port level mTLS not set correctly")
	}

	if policy.DestinationRule.TrafficPolicy.TLS.Mode != "ISTIO_MUTUAL" {
		t.Error("TLS mode not set correctly")
	}

	if policy.DestinationRule.TrafficPolicy.ConnectionPool.TCP.MaxConnections != 100 {
		t.Error("TCP max connections not set correctly")
	}

	if policy.DestinationRule.TrafficPolicy.OutlierDetection.Consecutive5xxErrors != 5 {
		t.Error("Outlier detection not set correctly")
	}
}

func TestPolicyKey(t *testing.T) {
	tests := []struct {
		namespace string
		service   string
		want      string
	}{
		{"default", "my-service", "default/my-service"},
		{"default", "", "default"},
		{"", "my-service", "/my-service"},
		{"", "", ""},
	}

	for _, tt := range tests {
		got := policyKey(tt.namespace, tt.service)
		if got != tt.want {
			t.Errorf("policyKey(%q, %q) = %q, want %q", tt.namespace, tt.service, got, tt.want)
		}
	}
}
