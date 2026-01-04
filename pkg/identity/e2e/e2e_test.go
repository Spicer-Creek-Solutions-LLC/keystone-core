// Package e2e provides end-to-end integration tests for the SPIFFE identity system.
package e2e

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/identity"
	"github.com/shawnbutts/keystone-core/pkg/identity/federation"
)

// Test helpers for generating certificates

func generateTestCA(t *testing.T, trustDomain string) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   trustDomain + " CA",
			Organization: []string{"Keystone Core Test"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
		MaxPathLen:            1,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA certificate: %v", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("failed to parse CA certificate: %v", err)
	}

	return caCert, caKey
}

func generateTestSVID(t *testing.T, trustDomain, path string, caCert *x509.Certificate, caKey *ecdsa.PrivateKey) (*x509.Certificate, *ecdsa.PrivateKey) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	spiffeID, _ := url.Parse("spiffe://" + trustDomain + path)

	template := &x509.Certificate{
		SerialNumber: big.NewInt(time.Now().UnixNano()),
		Subject: pkix.Name{
			CommonName: "SPIFFE SVID",
		},
		NotBefore:   time.Now().Add(-1 * time.Minute),
		NotAfter:    time.Now().Add(1 * time.Hour),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		URIs:        []*url.URL{spiffeID},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create SVID: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("failed to parse SVID: %v", err)
	}

	return cert, key
}

func writeCertToFile(t *testing.T, cert *x509.Certificate, path string) {
	t.Helper()

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create cert file: %v", err)
	}
	defer f.Close()

	if err := pem.Encode(f, &pem.Block{Type: "CERTIFICATE", Bytes: cert.Raw}); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
}

func writeKeyToFile(t *testing.T, key *ecdsa.PrivateKey, path string) {
	t.Helper()

	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}

	f, err := os.Create(path)
	if err != nil {
		t.Fatalf("failed to create key file: %v", err)
	}
	defer f.Close()

	if err := pem.Encode(f, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}
}

// E2E Test: Complete identity lifecycle
func TestE2E_IdentityLifecycle(t *testing.T) {
	// This test verifies the complete lifecycle of identity management:
	// 1. Create trust bundles
	// 2. Create SVIDs
	// 3. Validate SVIDs
	// 4. Verify chain trust

	trustDomain := "example.org"

	// Generate CA and trust bundle
	caCert, caKey := generateTestCA(t, trustDomain)

	trustBundle := &identity.TrustBundle{
		TrustDomain:     trustDomain,
		X509Authorities: []*x509.Certificate{caCert},
		UpdatedAt:       time.Now(),
	}

	// Generate SVID for agent
	svidCert, svidKey := generateTestSVID(t, trustDomain, "/agent/test-agent-1", caCert, caKey)

	// Create X509SVID
	svid := &identity.X509SVID{
		SPIFFEID: identity.SPIFFEID{
			TrustDomain: trustDomain,
			Path:        "/agent/test-agent-1",
		},
		Certificates: []*x509.Certificate{svidCert, caCert},
		PrivateKey:   svidKey,
	}

	// Verify SVID fields
	if svid.SPIFFEID.String() != "spiffe://example.org/agent/test-agent-1" {
		t.Errorf("unexpected SPIFFE ID: %s", svid.SPIFFEID.String())
	}

	// Verify certificate chain
	roots := x509.NewCertPool()
	roots.AddCert(caCert)

	opts := x509.VerifyOptions{
		Roots: roots,
	}

	if _, err := svidCert.Verify(opts); err != nil {
		t.Errorf("failed to verify SVID certificate: %v", err)
	}

	// Verify trust bundle contains CA
	if len(trustBundle.X509Authorities) != 1 {
		t.Errorf("expected 1 CA in trust bundle, got %d", len(trustBundle.X509Authorities))
	}
}

// E2E Test: Federation between trust domains
func TestE2E_Federation(t *testing.T) {
	ctx := context.Background()

	// Create two trust domains
	domain1 := "cluster-a.example.org"
	domain2 := "cluster-b.example.org"

	// Generate CAs for each domain
	ca1Cert, ca1Key := generateTestCA(t, domain1)
	ca2Cert, ca2Key := generateTestCA(t, domain2)

	// Create trust bundles
	bundle1 := &identity.TrustBundle{
		TrustDomain:     domain1,
		X509Authorities: []*x509.Certificate{ca1Cert},
		UpdatedAt:       time.Now(),
	}

	bundle2 := &identity.TrustBundle{
		TrustDomain:     domain2,
		X509Authorities: []*x509.Certificate{ca2Cert},
		UpdatedAt:       time.Now(),
	}

	// Create federation manager for domain1
	store := federation.NewInMemoryStore()
	config := &federation.FederationConfig{
		LocalTrustDomain:       domain1,
		LocalTrustBundle:       bundle1,
		DefaultRefreshInterval: 5 * time.Minute,
		Store:                  store,
	}

	manager, err := federation.NewManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Add domain2 as federated domain
	fedDomain := &federation.FederatedDomain{
		TrustDomain: domain2,
		Type:        federation.FederationTypeBidirectional,
		State:       federation.FederationStateActive,
		TrustBundle: bundle2,
		Policy: &federation.TrustPolicy{
			Name:         "allow-agents",
			AllowedPaths: []string{"/agent/**"},
		},
	}

	if err := manager.AddFederatedDomain(ctx, fedDomain); err != nil {
		t.Fatalf("failed to add federated domain: %v", err)
	}

	// Generate SVID from domain2
	svidCert, _ := generateTestSVID(t, domain2, "/agent/remote-agent", ca2Cert, ca2Key)

	svid := &identity.X509SVID{
		SPIFFEID: identity.SPIFFEID{
			TrustDomain: domain2,
			Path:        "/agent/remote-agent",
		},
		Certificates: []*x509.Certificate{svidCert, ca2Cert},
	}

	// Validate SVID from federated domain
	result, err := manager.ValidateSVID(ctx, svid)
	if err != nil {
		t.Fatalf("failed to validate SVID: %v", err)
	}

	if !result.Valid {
		t.Errorf("expected SVID to be valid: %s", result.Error)
	}

	if !result.IsFederated {
		t.Error("expected SVID to be marked as federated")
	}

	if result.TrustDomain != domain2 {
		t.Errorf("expected trust domain %s, got %s", domain2, result.TrustDomain)
	}

	// Generate SVID from domain1 (local)
	localSvidCert, _ := generateTestSVID(t, domain1, "/agent/local-agent", ca1Cert, ca1Key)

	localSvid := &identity.X509SVID{
		SPIFFEID: identity.SPIFFEID{
			TrustDomain: domain1,
			Path:        "/agent/local-agent",
		},
		Certificates: []*x509.Certificate{localSvidCert, ca1Cert},
	}

	// Validate local SVID
	localResult, err := manager.ValidateSVID(ctx, localSvid)
	if err != nil {
		t.Fatalf("failed to validate local SVID: %v", err)
	}

	if !localResult.Valid {
		t.Errorf("expected local SVID to be valid: %s", localResult.Error)
	}

	if localResult.IsFederated {
		t.Error("expected local SVID to NOT be marked as federated")
	}

	// Test aggregated trust bundle
	aggregated, err := manager.GetAggregatedTrustBundle(ctx)
	if err != nil {
		t.Fatalf("failed to get aggregated bundle: %v", err)
	}

	// Should have CAs from both domains
	if len(aggregated.X509Authorities) != 2 {
		t.Errorf("expected 2 CAs in aggregated bundle, got %d", len(aggregated.X509Authorities))
	}
}

// E2E Test: Policy enforcement in federation
func TestE2E_FederationPolicy(t *testing.T) {
	ctx := context.Background()

	domain1 := "local.example.org"
	domain2 := "remote.example.org"

	// Generate CAs
	ca1Cert, _ := generateTestCA(t, domain1)
	ca2Cert, ca2Key := generateTestCA(t, domain2)

	bundle1 := &identity.TrustBundle{
		TrustDomain:     domain1,
		X509Authorities: []*x509.Certificate{ca1Cert},
		UpdatedAt:       time.Now(),
	}

	bundle2 := &identity.TrustBundle{
		TrustDomain:     domain2,
		X509Authorities: []*x509.Certificate{ca2Cert},
		UpdatedAt:       time.Now(),
	}

	// Create manager with restrictive policy
	store := federation.NewInMemoryStore()
	config := &federation.FederationConfig{
		LocalTrustDomain:       domain1,
		LocalTrustBundle:       bundle1,
		DefaultRefreshInterval: 5 * time.Minute,
		Store:                  store,
	}

	manager, err := federation.NewManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Add federated domain with restrictive policy
	// Only allow /service/** paths, deny /admin/**
	fedDomain := &federation.FederatedDomain{
		TrustDomain: domain2,
		Type:        federation.FederationTypeBidirectional,
		State:       federation.FederationStateActive,
		TrustBundle: bundle2,
		Policy: &federation.TrustPolicy{
			Name:         "restrict-admin",
			AllowedPaths: []string{"/service/**", "/agent/**"},
			DeniedPaths:  []string{"/admin/**"},
		},
	}

	if err := manager.AddFederatedDomain(ctx, fedDomain); err != nil {
		t.Fatalf("failed to add federated domain: %v", err)
	}

	tests := []struct {
		name      string
		path      string
		wantValid bool
	}{
		{
			name:      "allowed service path",
			path:      "/service/api",
			wantValid: true,
		},
		{
			name:      "allowed nested service path",
			path:      "/service/api/v1",
			wantValid: true,
		},
		{
			name:      "allowed agent path",
			path:      "/agent/test",
			wantValid: true,
		},
		{
			name:      "denied admin path",
			path:      "/admin/dashboard",
			wantValid: false,
		},
		{
			name:      "denied nested admin path",
			path:      "/admin/users/delete",
			wantValid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svidCert, _ := generateTestSVID(t, domain2, tt.path, ca2Cert, ca2Key)

			svid := &identity.X509SVID{
				SPIFFEID: identity.SPIFFEID{
					TrustDomain: domain2,
					Path:        tt.path,
				},
				Certificates: []*x509.Certificate{svidCert, ca2Cert},
			}

			result, err := manager.ValidateSVID(ctx, svid)
			if err != nil {
				t.Fatalf("validation error: %v", err)
			}

			if result.Valid != tt.wantValid {
				t.Errorf("expected valid=%v, got valid=%v (error: %s)", tt.wantValid, result.Valid, result.Error)
			}
		})
	}
}

// E2E Test: Federation state transitions
func TestE2E_FederationStateTransitions(t *testing.T) {
	ctx := context.Background()

	domain1 := "primary.example.org"
	domain2 := "secondary.example.org"

	ca1Cert, _ := generateTestCA(t, domain1)
	ca2Cert, ca2Key := generateTestCA(t, domain2)

	bundle1 := &identity.TrustBundle{
		TrustDomain:     domain1,
		X509Authorities: []*x509.Certificate{ca1Cert},
		UpdatedAt:       time.Now(),
	}

	bundle2 := &identity.TrustBundle{
		TrustDomain:     domain2,
		X509Authorities: []*x509.Certificate{ca2Cert},
		UpdatedAt:       time.Now(),
	}

	store := federation.NewInMemoryStore()
	config := &federation.FederationConfig{
		LocalTrustDomain:       domain1,
		LocalTrustBundle:       bundle1,
		DefaultRefreshInterval: 5 * time.Minute,
		Store:                  store,
	}

	manager, err := federation.NewManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Create federated domain
	fedDomain := &federation.FederatedDomain{
		TrustDomain: domain2,
		Type:        federation.FederationTypeBidirectional,
		State:       federation.FederationStateActive,
		TrustBundle: bundle2,
		Policy: &federation.TrustPolicy{
			Name:         "default",
			AllowedPaths: []string{"/**"},
		},
	}

	if err := manager.AddFederatedDomain(ctx, fedDomain); err != nil {
		t.Fatalf("failed to add federated domain: %v", err)
	}

	// Generate test SVID
	svidCert, _ := generateTestSVID(t, domain2, "/service/test", ca2Cert, ca2Key)
	svid := &identity.X509SVID{
		SPIFFEID: identity.SPIFFEID{
			TrustDomain: domain2,
			Path:        "/service/test",
		},
		Certificates: []*x509.Certificate{svidCert, ca2Cert},
	}

	// Verify active state allows validation
	result, _ := manager.ValidateSVID(ctx, svid)
	if !result.Valid {
		t.Error("expected SVID to be valid with active federation")
	}

	// Suspend the federation
	fedDomain.State = federation.FederationStateSuspended
	if err := manager.UpdateFederatedDomain(ctx, fedDomain); err != nil {
		t.Fatalf("failed to suspend federation: %v", err)
	}

	// Verify suspended state rejects validation
	result, _ = manager.ValidateSVID(ctx, svid)
	if result.Valid {
		t.Error("expected SVID to be invalid with suspended federation")
	}

	// Reactivate the federation
	fedDomain.State = federation.FederationStateActive
	if err := manager.UpdateFederatedDomain(ctx, fedDomain); err != nil {
		t.Fatalf("failed to reactivate federation: %v", err)
	}

	// Verify active state allows validation again
	result, _ = manager.ValidateSVID(ctx, svid)
	if !result.Valid {
		t.Error("expected SVID to be valid after reactivation")
	}

	// Revoke the federation
	fedDomain.State = federation.FederationStateRevoked
	if err := manager.UpdateFederatedDomain(ctx, fedDomain); err != nil {
		t.Fatalf("failed to revoke federation: %v", err)
	}

	// Verify revoked state rejects validation
	result, _ = manager.ValidateSVID(ctx, svid)
	if result.Valid {
		t.Error("expected SVID to be invalid with revoked federation")
	}
}

// E2E Test: Trust bundle with multiple CAs
func TestE2E_MultipleCAs(t *testing.T) {
	trustDomain := "multi-ca.example.org"

	// Generate multiple CAs (simulating CA rotation)
	ca1Cert, ca1Key := generateTestCA(t, trustDomain)
	ca2Cert, ca2Key := generateTestCA(t, trustDomain)

	// Trust bundle with both CAs
	trustBundle := &identity.TrustBundle{
		TrustDomain:     trustDomain,
		X509Authorities: []*x509.Certificate{ca1Cert, ca2Cert},
		UpdatedAt:       time.Now(),
	}

	// Generate SVIDs signed by each CA
	svid1Cert, _ := generateTestSVID(t, trustDomain, "/agent/1", ca1Cert, ca1Key)
	svid2Cert, _ := generateTestSVID(t, trustDomain, "/agent/2", ca2Cert, ca2Key)

	// Create root pool with both CAs
	roots := x509.NewCertPool()
	for _, ca := range trustBundle.X509Authorities {
		roots.AddCert(ca)
	}

	opts := x509.VerifyOptions{
		Roots: roots,
	}

	// Both SVIDs should verify
	if _, err := svid1Cert.Verify(opts); err != nil {
		t.Errorf("SVID from CA1 failed to verify: %v", err)
	}

	if _, err := svid2Cert.Verify(opts); err != nil {
		t.Errorf("SVID from CA2 failed to verify: %v", err)
	}
}

// E2E Test: File-based identity provider simulation
func TestE2E_FileBasedIdentity(t *testing.T) {
	tmpDir := t.TempDir()
	trustDomain := "file-based.example.org"

	// Generate CA and SVID
	caCert, caKey := generateTestCA(t, trustDomain)
	svidCert, svidKey := generateTestSVID(t, trustDomain, "/agent/file-test", caCert, caKey)

	// Write to files
	certPath := filepath.Join(tmpDir, "svid.pem")
	keyPath := filepath.Join(tmpDir, "key.pem")
	bundlePath := filepath.Join(tmpDir, "bundle.pem")

	writeCertToFile(t, svidCert, certPath)
	writeKeyToFile(t, svidKey, keyPath)
	writeCertToFile(t, caCert, bundlePath)

	// Read and verify files
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatalf("failed to read cert: %v", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		t.Fatal("failed to decode cert PEM")
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("failed to parse cert: %v", err)
	}

	// Verify SPIFFE ID
	if len(cert.URIs) == 0 {
		t.Fatal("no URIs in certificate")
	}

	spiffeID := cert.URIs[0].String()
	if spiffeID != "spiffe://file-based.example.org/agent/file-test" {
		t.Errorf("unexpected SPIFFE ID: %s", spiffeID)
	}

	// Verify certificate against bundle
	bundlePEM, err := os.ReadFile(bundlePath)
	if err != nil {
		t.Fatalf("failed to read bundle: %v", err)
	}

	bundleBlock, _ := pem.Decode(bundlePEM)
	bundleCert, err := x509.ParseCertificate(bundleBlock.Bytes)
	if err != nil {
		t.Fatalf("failed to parse bundle cert: %v", err)
	}

	roots := x509.NewCertPool()
	roots.AddCert(bundleCert)

	if _, err := cert.Verify(x509.VerifyOptions{Roots: roots}); err != nil {
		t.Errorf("failed to verify cert against bundle: %v", err)
	}
}

// E2E Test: SPIFFE ID parsing and validation
func TestE2E_SPIFFEIDParsing(t *testing.T) {
	tests := []struct {
		name        string
		input       string
		wantDomain  string
		wantPath    string
		wantValid   bool
	}{
		{
			name:       "standard agent ID",
			input:      "spiffe://example.org/agent/node1",
			wantDomain: "example.org",
			wantPath:   "/agent/node1",
			wantValid:  true,
		},
		{
			name:       "service account ID",
			input:      "spiffe://cluster.local/ns/default/sa/myservice",
			wantDomain: "cluster.local",
			wantPath:   "/ns/default/sa/myservice",
			wantValid:  true,
		},
		{
			name:       "control plane ID",
			input:      "spiffe://keystone.example.com/server/control-plane-1",
			wantDomain: "keystone.example.com",
			wantPath:   "/server/control-plane-1",
			wantValid:  true,
		},
		{
			name:       "workload ID",
			input:      "spiffe://prod.example.org/workload/api/v2",
			wantDomain: "prod.example.org",
			wantPath:   "/workload/api/v2",
			wantValid:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := identity.ParseSPIFFEID(tt.input)

			if tt.wantValid {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}

				if id.TrustDomain != tt.wantDomain {
					t.Errorf("expected domain %s, got %s", tt.wantDomain, id.TrustDomain)
				}

				if id.Path != tt.wantPath {
					t.Errorf("expected path %s, got %s", tt.wantPath, id.Path)
				}

				// Round-trip test
				if id.String() != tt.input {
					t.Errorf("round-trip failed: %s != %s", id.String(), tt.input)
				}
			} else {
				if err == nil {
					t.Error("expected error but got none")
				}
			}
		})
	}
}

// E2E Test: Trust bundle expiration handling
func TestE2E_TrustBundleExpiration(t *testing.T) {
	trustDomain := "expiry-test.example.org"

	// Generate CA with short validity (already expired)
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate CA key: %v", err)
	}

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: trustDomain + " CA",
		},
		NotBefore:             time.Now().Add(-48 * time.Hour),
		NotAfter:              time.Now().Add(-24 * time.Hour), // Already expired
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		IsCA:                  true,
		BasicConstraintsValid: true,
	}

	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("failed to create CA certificate: %v", err)
	}

	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		t.Fatalf("failed to parse CA certificate: %v", err)
	}

	// Check that CA is expired
	now := time.Now()
	if now.Before(caCert.NotAfter) {
		t.Error("expected CA to be expired")
	}

	// Trust bundle should still hold the certificate
	// but validation should fail
	trustBundle := &identity.TrustBundle{
		TrustDomain:     trustDomain,
		X509Authorities: []*x509.Certificate{caCert},
		UpdatedAt:       time.Now(),
	}

	if len(trustBundle.X509Authorities) != 1 {
		t.Error("expected trust bundle to contain the expired CA")
	}
}

// E2E Test: Federation store persistence
func TestE2E_FederationStorePersistence(t *testing.T) {
	ctx := context.Background()
	store := federation.NewInMemoryStore()

	// Create multiple federated domains
	domains := []*federation.FederatedDomain{
		{
			TrustDomain: "domain-a.example.org",
			Type:        federation.FederationTypeBidirectional,
			State:       federation.FederationStateActive,
		},
		{
			TrustDomain: "domain-b.example.org",
			Type:        federation.FederationTypeUnidirectional,
			State:       federation.FederationStatePending,
		},
		{
			TrustDomain: "domain-c.example.org",
			Type:        federation.FederationTypeBidirectional,
			State:       federation.FederationStateSuspended,
		},
	}

	// Save all domains
	for _, domain := range domains {
		if err := store.Save(ctx, domain); err != nil {
			t.Fatalf("failed to save domain %s: %v", domain.TrustDomain, err)
		}
	}

	// Verify count
	if store.Count() != 3 {
		t.Errorf("expected 3 domains, got %d", store.Count())
	}

	// Load and verify each domain
	for _, original := range domains {
		loaded, err := store.Load(ctx, original.TrustDomain)
		if err != nil {
			t.Fatalf("failed to load domain %s: %v", original.TrustDomain, err)
		}

		if loaded.TrustDomain != original.TrustDomain {
			t.Errorf("trust domain mismatch: %s != %s", loaded.TrustDomain, original.TrustDomain)
		}

		if loaded.Type != original.Type {
			t.Errorf("type mismatch for %s: %s != %s", original.TrustDomain, loaded.Type, original.Type)
		}

		if loaded.State != original.State {
			t.Errorf("state mismatch for %s: %s != %s", original.TrustDomain, loaded.State, original.State)
		}
	}

	// List all domains
	listed, err := store.List(ctx)
	if err != nil {
		t.Fatalf("failed to list domains: %v", err)
	}

	if len(listed) != 3 {
		t.Errorf("expected 3 listed domains, got %d", len(listed))
	}

	// Delete one domain
	if err := store.Delete(ctx, "domain-b.example.org"); err != nil {
		t.Fatalf("failed to delete domain: %v", err)
	}

	if store.Count() != 2 {
		t.Errorf("expected 2 domains after delete, got %d", store.Count())
	}

	// Clear all
	store.Clear()
	if store.Count() != 0 {
		t.Errorf("expected 0 domains after clear, got %d", store.Count())
	}
}

// E2E Test: Complete workflow
func TestE2E_CompleteWorkflow(t *testing.T) {
	ctx := context.Background()

	// Setup: Create local trust domain
	localDomain := "local.cluster.example.org"
	localCA, localCAKey := generateTestCA(t, localDomain)
	localBundle := &identity.TrustBundle{
		TrustDomain:     localDomain,
		X509Authorities: []*x509.Certificate{localCA},
		UpdatedAt:       time.Now(),
	}

	// Setup: Create remote trust domain
	remoteDomain := "remote.cluster.example.org"
	remoteCA, remoteCAKey := generateTestCA(t, remoteDomain)
	remoteBundle := &identity.TrustBundle{
		TrustDomain:     remoteDomain,
		X509Authorities: []*x509.Certificate{remoteCA},
		UpdatedAt:       time.Now(),
	}

	// Step 1: Create federation manager
	store := federation.NewInMemoryStore()
	manager, err := federation.NewManager(&federation.FederationConfig{
		LocalTrustDomain:       localDomain,
		LocalTrustBundle:       localBundle,
		DefaultRefreshInterval: 5 * time.Minute,
		Store:                  store,
	})
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Step 2: Establish federation with remote domain
	err = manager.AddFederatedDomain(ctx, &federation.FederatedDomain{
		TrustDomain: remoteDomain,
		Type:        federation.FederationTypeBidirectional,
		State:       federation.FederationStateActive,
		TrustBundle: remoteBundle,
		Policy: &federation.TrustPolicy{
			Name:            "production-services",
			AllowedPaths:    []string{"/service/**"},
			AllowedServices: []string{"api", "web", "worker"},
		},
	})
	if err != nil {
		t.Fatalf("failed to establish federation: %v", err)
	}

	// Step 3: Local agent gets SVID
	localAgentCert, _ := generateTestSVID(t, localDomain, "/agent/local-1", localCA, localCAKey)
	localAgentSVID := &identity.X509SVID{
		SPIFFEID: identity.SPIFFEID{
			TrustDomain: localDomain,
			Path:        "/agent/local-1",
		},
		Certificates: []*x509.Certificate{localAgentCert, localCA},
	}

	// Step 4: Remote service gets SVID
	remoteServiceCert, _ := generateTestSVID(t, remoteDomain, "/service/api", remoteCA, remoteCAKey)
	remoteServiceSVID := &identity.X509SVID{
		SPIFFEID: identity.SPIFFEID{
			TrustDomain: remoteDomain,
			Path:        "/service/api",
		},
		Certificates: []*x509.Certificate{remoteServiceCert, remoteCA},
	}

	// Step 5: Validate both SVIDs
	localResult, err := manager.ValidateSVID(ctx, localAgentSVID)
	if err != nil {
		t.Fatalf("failed to validate local SVID: %v", err)
	}
	if !localResult.Valid {
		t.Errorf("local SVID should be valid: %s", localResult.Error)
	}

	remoteResult, err := manager.ValidateSVID(ctx, remoteServiceSVID)
	if err != nil {
		t.Fatalf("failed to validate remote SVID: %v", err)
	}
	if !remoteResult.Valid {
		t.Errorf("remote SVID should be valid: %s", remoteResult.Error)
	}
	if !remoteResult.IsFederated {
		t.Error("remote SVID should be marked as federated")
	}

	// Step 6: Get aggregated trust bundle for TLS config
	aggregated, err := manager.GetAggregatedTrustBundle(ctx)
	if err != nil {
		t.Fatalf("failed to get aggregated bundle: %v", err)
	}

	// Should have 2 CAs (local + remote)
	if len(aggregated.X509Authorities) != 2 {
		t.Errorf("expected 2 CAs in aggregated bundle, got %d", len(aggregated.X509Authorities))
	}

	// Step 7: Verify mTLS would work with aggregated bundle
	roots := x509.NewCertPool()
	for _, ca := range aggregated.X509Authorities {
		roots.AddCert(ca)
	}

	opts := x509.VerifyOptions{Roots: roots}

	// Both certificates should verify against aggregated bundle
	if _, err := localAgentCert.Verify(opts); err != nil {
		t.Errorf("local agent cert should verify against aggregated bundle: %v", err)
	}

	if _, err := remoteServiceCert.Verify(opts); err != nil {
		t.Errorf("remote service cert should verify against aggregated bundle: %v", err)
	}

	// Step 8: List federated domains
	domains, err := manager.ListFederatedDomains(ctx)
	if err != nil {
		t.Fatalf("failed to list domains: %v", err)
	}

	if len(domains) != 1 {
		t.Errorf("expected 1 federated domain, got %d", len(domains))
	}

	// Step 9: Remove federation
	if err := manager.RemoveFederatedDomain(ctx, remoteDomain); err != nil {
		t.Fatalf("failed to remove federation: %v", err)
	}

	// Step 10: Verify remote SVID no longer validates
	remoteResult, _ = manager.ValidateSVID(ctx, remoteServiceSVID)
	if remoteResult.Valid {
		t.Error("remote SVID should no longer be valid after federation removal")
	}
}
