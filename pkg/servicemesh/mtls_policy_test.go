package servicemesh

import (
	"context"
	"crypto/x509"
	"net/url"
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

// ConnectionTester tests

func TestNewConnectionTester(t *testing.T) {
	// Test with zero timeout (should use default)
	tester := NewConnectionTester(0, nil)
	if tester == nil {
		t.Fatal("Expected tester to be non-nil")
	}
	if tester.timeout != 5*time.Second {
		t.Errorf("Expected default timeout of 5s, got %v", tester.timeout)
	}

	// Test with custom timeout
	tester = NewConnectionTester(10*time.Second, nil)
	if tester.timeout != 10*time.Second {
		t.Errorf("Expected timeout of 10s, got %v", tester.timeout)
	}

	// Test with metadata
	metadata := &Metadata{
		MeshType: MeshTypeIstio,
		TLSConfig: &TLSConfig{
			Enabled: true,
		},
	}
	tester = NewConnectionTester(5*time.Second, metadata)
	if tester.metadata != metadata {
		t.Error("Metadata not set correctly")
	}
}

func TestConnectionTester_TestPlaintext(t *testing.T) {
	tester := NewConnectionTester(1*time.Second, nil)

	// Test connection to invalid address (should fail)
	connected, err := tester.TestPlaintext("127.0.0.1:99999")
	if connected {
		t.Error("Expected connection to fail for invalid port")
	}
	if err == nil {
		t.Error("Expected error for invalid address")
	}

	// Test connection to non-existent host (should fail)
	connected, err = tester.TestPlaintext("nonexistent.invalid:80")
	if connected {
		t.Error("Expected connection to fail for non-existent host")
	}
}

func TestConnectionTester_TestMTLS_NoServer(t *testing.T) {
	tester := NewConnectionTester(1*time.Second, nil)

	// Test mTLS connection to invalid address
	result, err := tester.TestMTLS("127.0.0.1:99999")
	if err != nil {
		t.Fatalf("TestMTLS should not return error, got %v", err)
	}
	if result.Connected {
		t.Error("Expected connection to fail")
	}
	if result.Error == nil {
		t.Error("Expected result.Error to be set")
	}
}

func TestConnectionTester_VerifyPeerIdentity(t *testing.T) {
	tester := NewConnectionTester(5*time.Second, nil)

	tests := []struct {
		name           string
		certs          []*x509.Certificate
		expectedSPIFFE string
		want           bool
	}{
		{
			name:           "empty certs",
			certs:          nil,
			expectedSPIFFE: "spiffe://cluster.local/ns/default/sa/test",
			want:           false,
		},
		{
			name:           "empty expected SPIFFE",
			certs:          []*x509.Certificate{{}},
			expectedSPIFFE: "",
			want:           false,
		},
		{
			name: "no SPIFFE in cert",
			certs: []*x509.Certificate{{
				// No URIs
			}},
			expectedSPIFFE: "spiffe://cluster.local/ns/default/sa/test",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tester.VerifyPeerIdentity(tt.certs, tt.expectedSPIFFE)
			if result != tt.want {
				t.Errorf("VerifyPeerIdentity() = %v, want %v", result, tt.want)
			}
		})
	}
}

func TestExtractSPIFFEID(t *testing.T) {
	tests := []struct {
		name string
		cert *x509.Certificate
		want string
	}{
		{
			name: "no URIs",
			cert: &x509.Certificate{},
			want: "",
		},
		{
			name: "non-SPIFFE URI",
			cert: &x509.Certificate{
				URIs: []*url.URL{
					{Scheme: "https", Host: "example.com"},
				},
			},
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := extractSPIFFEID(tt.cert)
			if result != tt.want {
				t.Errorf("extractSPIFFEID() = %q, want %q", result, tt.want)
			}
		})
	}
}

func TestConnectionResult_Types(t *testing.T) {
	result := &ConnectionResult{
		Connected:    true,
		TLSEnabled:   true,
		TLSVersion:   "TLS 1.3",
		CipherSuite:  "TLS_AES_256_GCM_SHA384",
		PeerSPIFFEID: "spiffe://cluster.local/ns/default/sa/test",
		Duration:     100 * time.Millisecond,
	}

	if !result.Connected {
		t.Error("Connected not set correctly")
	}
	if !result.TLSEnabled {
		t.Error("TLSEnabled not set correctly")
	}
	if result.TLSVersion != "TLS 1.3" {
		t.Error("TLSVersion not set correctly")
	}
}

// AuthorizationPolicy tests

func TestAuthorizationAction_Values(t *testing.T) {
	tests := []struct {
		action AuthorizationAction
		want   string
	}{
		{AuthorizationActionAllow, "ALLOW"},
		{AuthorizationActionDeny, "DENY"},
		{AuthorizationActionAudit, "AUDIT"},
		{AuthorizationActionCustom, "CUSTOM"},
	}

	for _, tt := range tests {
		if string(tt.action) != tt.want {
			t.Errorf("AuthorizationAction %v = %q, want %q", tt.action, tt.action, tt.want)
		}
	}
}

func TestAuthorizationPolicy_Types(t *testing.T) {
	policy := &AuthorizationPolicy{
		Name:      "test-policy",
		Namespace: "default",
		Action:    AuthorizationActionAllow,
		Selector: map[string]string{
			"app": "my-app",
		},
		Rules: []AuthorizationRule{
			{
				From: []RuleSource{
					{
						Principals:    []string{"cluster.local/ns/default/sa/frontend"},
						Namespaces:    []string{"default"},
						IPBlocks:      []string{"10.0.0.0/8"},
						NotIPBlocks:   []string{"10.0.1.0/24"},
					},
				},
				To: []RuleDestination{
					{
						Methods: []string{"GET", "POST"},
						Paths:   []string{"/api/*"},
						Ports:   []string{"8080"},
					},
				},
				When: []RuleCondition{
					{
						Key:    "request.headers[x-custom-header]",
						Values: []string{"allowed"},
					},
				},
			},
		},
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	if policy.Name != "test-policy" {
		t.Error("Name not set correctly")
	}
	if policy.Action != AuthorizationActionAllow {
		t.Error("Action not set correctly")
	}
	if len(policy.Rules) != 1 {
		t.Error("Rules not set correctly")
	}
	if len(policy.Rules[0].From) != 1 {
		t.Error("Rule From not set correctly")
	}
	if len(policy.Rules[0].From[0].Principals) != 1 {
		t.Error("Rule principals not set correctly")
	}
	if len(policy.Rules[0].To) != 1 {
		t.Error("Rule To not set correctly")
	}
	if len(policy.Rules[0].To[0].Methods) != 2 {
		t.Error("Rule methods not set correctly")
	}
	if len(policy.Rules[0].When) != 1 {
		t.Error("Rule When not set correctly")
	}
}

func TestRuleSource_Types(t *testing.T) {
	source := RuleSource{
		Principals:           []string{"principal1", "principal2"},
		NotPrincipals:        []string{"excluded-principal"},
		RequestPrincipals:    []string{"request-principal"},
		NotRequestPrincipals: []string{"excluded-request"},
		Namespaces:           []string{"ns1", "ns2"},
		NotNamespaces:        []string{"excluded-ns"},
		IPBlocks:             []string{"10.0.0.0/8", "192.168.0.0/16"},
		NotIPBlocks:          []string{"10.0.1.0/24"},
	}

	if len(source.Principals) != 2 {
		t.Error("Principals not set correctly")
	}
	if len(source.NotPrincipals) != 1 {
		t.Error("NotPrincipals not set correctly")
	}
	if len(source.Namespaces) != 2 {
		t.Error("Namespaces not set correctly")
	}
	if len(source.IPBlocks) != 2 {
		t.Error("IPBlocks not set correctly")
	}
}

func TestRuleDestination_Types(t *testing.T) {
	dest := RuleDestination{
		Hosts:      []string{"host1.example.com", "host2.example.com"},
		NotHosts:   []string{"excluded.example.com"},
		Ports:      []string{"8080", "443"},
		NotPorts:   []string{"22"},
		Methods:    []string{"GET", "POST", "PUT"},
		NotMethods: []string{"DELETE"},
		Paths:      []string{"/api/*", "/health"},
		NotPaths:   []string{"/internal/*"},
	}

	if len(dest.Hosts) != 2 {
		t.Error("Hosts not set correctly")
	}
	if len(dest.Ports) != 2 {
		t.Error("Ports not set correctly")
	}
	if len(dest.Methods) != 3 {
		t.Error("Methods not set correctly")
	}
	if len(dest.Paths) != 2 {
		t.Error("Paths not set correctly")
	}
}

func TestRuleCondition_Types(t *testing.T) {
	cond := RuleCondition{
		Key:       "request.headers[authorization]",
		Values:    []string{"Bearer *"},
		NotValues: []string{""},
	}

	if cond.Key != "request.headers[authorization]" {
		t.Error("Key not set correctly")
	}
	if len(cond.Values) != 1 {
		t.Error("Values not set correctly")
	}
	if len(cond.NotValues) != 1 {
		t.Error("NotValues not set correctly")
	}
}
