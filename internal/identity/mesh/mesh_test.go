package mesh

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
)

// Test helper functions

func createTestCertificate(t *testing.T, spiffeID string) (certPEM, keyPEM []byte, cert *x509.Certificate) {
	t.Helper()

	// Generate private key
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	// Parse SPIFFE ID for URI
	spiffeURL, err := url.Parse(spiffeID)
	if err != nil {
		t.Fatalf("failed to parse SPIFFE ID: %v", err)
	}

	// Create certificate template
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth, x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		URIs:                  []*url.URL{spiffeURL},
		DNSNames:              []string{"test.default.serviceaccount.identity.linkerd.cluster.local"},
	}

	// Self-sign the certificate
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	// Parse the certificate back
	cert, err = x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("failed to parse certificate: %v", err)
	}

	// Encode certificate to PEM
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certDER,
	})

	// Encode private key to PEM
	keyDER, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		t.Fatalf("failed to marshal key: %v", err)
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: keyDER,
	})

	return certPEM, keyPEM, cert
}

func createTestDir(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "mesh-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func writeTestCertFiles(t *testing.T, dir string, certPEM, keyPEM []byte) (certPath, keyPath, rootPath string) {
	t.Helper()

	certPath = filepath.Join(dir, "cert.pem")
	keyPath = filepath.Join(dir, "key.pem")
	rootPath = filepath.Join(dir, "root.pem")

	if err := os.WriteFile(certPath, certPEM, 0644); err != nil {
		t.Fatalf("failed to write cert: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		t.Fatalf("failed to write key: %v", err)
	}
	// Write root cert (same as cert for self-signed)
	if err := os.WriteFile(rootPath, certPEM, 0644); err != nil {
		t.Fatalf("failed to write root: %v", err)
	}

	return certPath, keyPath, rootPath
}

// Istio Provider Tests

func TestIstioConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		config := DefaultIstioConfig()
		if config.SDSAddress == "" {
			t.Error("expected default SDS address")
		}
		if config.CertPath == "" {
			t.Error("expected default cert path")
		}
		if config.RefreshInterval == 0 {
			t.Error("expected default refresh interval")
		}
	})
}

func TestIstioProvider_Create(t *testing.T) {
	t.Run("with_nil_config", func(t *testing.T) {
		provider, err := NewIstioProvider(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected provider")
		}
		if provider.Type() != identity.ProviderTypeIstio {
			t.Errorf("expected type Istio, got %v", provider.Type())
		}
	})

	t.Run("with_custom_config", func(t *testing.T) {
		config := &IstioConfig{
			TrustDomain: "test.local",
			CertPath:    "/custom/path",
		}
		provider, err := NewIstioProvider(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider.TrustDomain() != "test.local" {
			t.Errorf("expected trust domain test.local, got %s", provider.TrustDomain())
		}
	})
}

func TestIstioProvider_Lifecycle(t *testing.T) {
	dir := createTestDir(t)
	certPEM, keyPEM, _ := createTestCertificate(t, "spiffe://test.local/ns/default/sa/test")
	certPath, keyPath, rootPath := writeTestCertFiles(t, dir, certPEM, keyPEM)

	config := &IstioConfig{
		TrustDomain:         "test.local",
		CertChainPath:       certPath,
		KeyPath:             keyPath,
		RootCertPath:        rootPath,
		RefreshInterval:     time.Hour,
		HealthCheckInterval: time.Hour,
	}

	provider, err := NewIstioProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	ctx := context.Background()

	// Start provider
	if err := provider.Start(ctx); err != nil {
		t.Fatalf("failed to start provider: %v", err)
	}

	// Check status
	status := provider.Health(ctx)
	if status != identity.ProviderStatusHealthy {
		t.Errorf("expected healthy status, got %v", status)
	}

	// Get trust bundle
	bundle, err := provider.GetTrustBundle(ctx)
	if err != nil {
		t.Fatalf("failed to get trust bundle: %v", err)
	}
	if bundle.TrustDomain != "test.local" {
		t.Errorf("expected trust domain test.local, got %s", bundle.TrustDomain)
	}

	// Get SVID
	svid, err := provider.GetX509SVID(ctx)
	if err != nil {
		t.Fatalf("failed to get SVID: %v", err)
	}
	if svid.SPIFFEID.TrustDomain != "test.local" {
		t.Errorf("expected SPIFFE ID trust domain test.local, got %s", svid.SPIFFEID.TrustDomain)
	}

	// Stop provider
	if err := provider.Stop(ctx); err != nil {
		t.Fatalf("failed to stop provider: %v", err)
	}
}

func TestIstioProvider_Info(t *testing.T) {
	dir := createTestDir(t)
	certPEM, keyPEM, _ := createTestCertificate(t, "spiffe://test.local/ns/default/sa/test")
	certPath, keyPath, rootPath := writeTestCertFiles(t, dir, certPEM, keyPEM)

	config := &IstioConfig{
		TrustDomain:         "test.local",
		CertChainPath:       certPath,
		KeyPath:             keyPath,
		RootCertPath:        rootPath,
		RefreshInterval:     time.Hour,
		HealthCheckInterval: time.Hour,
	}

	provider, _ := NewIstioProvider(config)
	ctx := context.Background()
	provider.Start(ctx)
	defer provider.Stop(ctx)

	info := provider.Info(ctx)
	if info.Type != identity.ProviderTypeIstio {
		t.Errorf("expected type Istio, got %v", info.Type)
	}
	if info.TrustDomain != "test.local" {
		t.Errorf("expected trust domain test.local, got %s", info.TrustDomain)
	}
	if len(info.Capabilities) == 0 {
		t.Error("expected capabilities")
	}
}

func TestIstioProvider_NotAvailable(t *testing.T) {
	config := &IstioConfig{
		CertChainPath: "/nonexistent/path",
		SDSAddress:    "unix:///nonexistent/socket",
	}

	provider, _ := NewIstioProvider(config)
	if provider.IsAvailable() {
		t.Error("expected provider to not be available")
	}
}

// Consul Provider Tests

func TestConsulConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		config := DefaultConsulConfig()
		if config.HTTPAddr == "" {
			t.Error("expected default HTTP address")
		}
		if config.HTTPTimeout == 0 {
			t.Error("expected default HTTP timeout")
		}
	})
}

func TestConsulProvider_Create(t *testing.T) {
	t.Run("with_nil_config", func(t *testing.T) {
		provider, err := NewConsulProvider(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected provider")
		}
		if provider.Type() != identity.ProviderTypeConsul {
			t.Errorf("expected type Consul, got %v", provider.Type())
		}
	})

	t.Run("with_custom_config", func(t *testing.T) {
		config := &ConsulConfig{
			TrustDomain: "consul.local",
			HTTPAddr:    "http://localhost:8500",
		}
		provider, err := NewConsulProvider(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider.TrustDomain() != "consul.local" {
			t.Errorf("expected trust domain consul.local, got %s", provider.TrustDomain())
		}
	})
}

func TestConsulProvider_MockAPI(t *testing.T) {
	// Create mock Consul server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agent/self":
			json.NewEncoder(w).Encode(ConsulAgentResponse{
				Config: ConsulAgentConfig{
					Datacenter: "dc1",
					NodeName:   "node1",
					NodeID:     "test-id",
				},
			})
		case "/v1/agent/connect/ca/roots":
			json.NewEncoder(w).Encode(ConsulCARoots{
				TrustDomain:  "consul.local",
				ActiveRootID: "root-1",
				Roots:        []ConsulCARoot{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := &ConsulConfig{
		TrustDomain:         "consul.local",
		HTTPAddr:            server.URL,
		HealthCheckInterval: time.Hour,
		RefreshInterval:     time.Hour,
		HTTPTimeout:         10 * time.Second,
	}

	provider, err := NewConsulProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	// Check availability
	if !provider.IsAvailable() {
		t.Error("expected provider to be available")
	}
}

func TestConsulProvider_FileBased(t *testing.T) {
	dir := createTestDir(t)
	certPEM, keyPEM, _ := createTestCertificate(t, "spiffe://consul.local/ns/default/dc/dc1/svc/test")
	certPath, keyPath, _ := writeTestCertFiles(t, dir, certPEM, keyPEM)

	// Create mock server for availability check
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agent/self":
			json.NewEncoder(w).Encode(ConsulAgentResponse{
				Config: ConsulAgentConfig{
					Datacenter: "dc1",
				},
			})
		case "/v1/agent/connect/ca/roots":
			json.NewEncoder(w).Encode(ConsulCARoots{
				TrustDomain: "consul.local",
				Roots:       []ConsulCARoot{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := &ConsulConfig{
		TrustDomain:         "consul.local",
		HTTPAddr:            server.URL,
		CertFile:            certPath,
		KeyFile:             keyPath,
		HealthCheckInterval: time.Hour,
		RefreshInterval:     time.Hour,
		HTTPTimeout:         10 * time.Second,
	}

	provider, err := NewConsulProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	ctx := context.Background()

	// Start provider
	if err := provider.Start(ctx); err != nil {
		t.Fatalf("failed to start provider: %v", err)
	}
	defer provider.Stop(ctx)

	// Check health
	status := provider.Health(ctx)
	if status != identity.ProviderStatusHealthy {
		t.Errorf("expected healthy status, got %v", status)
	}

	// Get SVID
	svid, err := provider.GetX509SVID(ctx)
	if err != nil {
		t.Fatalf("failed to get SVID: %v", err)
	}
	if svid.SPIFFEID.TrustDomain != "consul.local" {
		t.Errorf("expected SPIFFE ID trust domain consul.local, got %s", svid.SPIFFEID.TrustDomain)
	}
}

func TestConsulProvider_Info(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(ConsulAgentResponse{
			Config: ConsulAgentConfig{
				Datacenter: "dc1",
			},
		})
	}))
	defer server.Close()

	config := &ConsulConfig{
		TrustDomain:  "consul.local",
		HTTPAddr:     server.URL,
		ServiceName:  "test-service",
		Datacenter:   "dc1",
	}

	provider, _ := NewConsulProvider(config)
	ctx := context.Background()

	info := provider.Info(ctx)
	if info.Type != identity.ProviderTypeConsul {
		t.Errorf("expected type Consul, got %v", info.Type)
	}
}

// Linkerd Provider Tests

func TestLinkerdConfig(t *testing.T) {
	t.Run("default_config", func(t *testing.T) {
		config := DefaultLinkerdConfig()
		if config.IdentityDir == "" {
			t.Error("expected default identity dir")
		}
		if config.CertPath == "" {
			t.Error("expected default cert path")
		}
		if config.KeyPath == "" {
			t.Error("expected default key path")
		}
	})
}

func TestLinkerdProvider_Create(t *testing.T) {
	t.Run("with_nil_config", func(t *testing.T) {
		provider, err := NewLinkerdProvider(nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider == nil {
			t.Fatal("expected provider")
		}
		if provider.Type() != identity.ProviderTypeLinkerd {
			t.Errorf("expected type Linkerd, got %v", provider.Type())
		}
	})

	t.Run("with_custom_config", func(t *testing.T) {
		config := &LinkerdConfig{
			TrustDomain: "linkerd.local",
			IdentityDir: "/custom/path",
		}
		provider, err := NewLinkerdProvider(config)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if provider.TrustDomain() != "linkerd.local" {
			t.Errorf("expected trust domain linkerd.local, got %s", provider.TrustDomain())
		}
	})
}

func TestLinkerdProvider_Lifecycle(t *testing.T) {
	dir := createTestDir(t)
	certPEM, keyPEM, _ := createTestCertificate(t, "spiffe://cluster.local/default/test")
	certPath, keyPath, rootPath := writeTestCertFiles(t, dir, certPEM, keyPEM)

	config := &LinkerdConfig{
		TrustDomain:         "cluster.local",
		IdentityDir:         dir,
		CertPath:            certPath,
		KeyPath:             keyPath,
		TrustAnchorsPath:    rootPath,
		RefreshInterval:     time.Hour,
		HealthCheckInterval: time.Hour,
	}

	provider, err := NewLinkerdProvider(config)
	if err != nil {
		t.Fatalf("failed to create provider: %v", err)
	}

	ctx := context.Background()

	// Start provider
	if err := provider.Start(ctx); err != nil {
		t.Fatalf("failed to start provider: %v", err)
	}

	// Check status
	status := provider.Health(ctx)
	if status != identity.ProviderStatusHealthy {
		t.Errorf("expected healthy status, got %v", status)
	}

	// Get trust bundle
	bundle, err := provider.GetTrustBundle(ctx)
	if err != nil {
		t.Fatalf("failed to get trust bundle: %v", err)
	}
	if bundle.TrustDomain != "cluster.local" {
		t.Errorf("expected trust domain cluster.local, got %s", bundle.TrustDomain)
	}

	// Get SVID
	svid, err := provider.GetX509SVID(ctx)
	if err != nil {
		t.Fatalf("failed to get SVID: %v", err)
	}
	if len(svid.Certificates) == 0 {
		t.Error("expected certificates in SVID")
	}

	// Get identity info
	info := provider.GetLinkerdIdentityInfo()
	if info.TrustDomain != "cluster.local" {
		t.Errorf("expected trust domain cluster.local, got %s", info.TrustDomain)
	}

	// Stop provider
	if err := provider.Stop(ctx); err != nil {
		t.Fatalf("failed to stop provider: %v", err)
	}
}

func TestLinkerdProvider_Info(t *testing.T) {
	dir := createTestDir(t)
	certPEM, keyPEM, _ := createTestCertificate(t, "spiffe://cluster.local/default/test")
	certPath, keyPath, rootPath := writeTestCertFiles(t, dir, certPEM, keyPEM)

	config := &LinkerdConfig{
		TrustDomain:         "cluster.local",
		IdentityDir:         dir,
		CertPath:            certPath,
		KeyPath:             keyPath,
		TrustAnchorsPath:    rootPath,
		RefreshInterval:     time.Hour,
		HealthCheckInterval: time.Hour,
	}

	provider, _ := NewLinkerdProvider(config)
	ctx := context.Background()
	provider.Start(ctx)
	defer provider.Stop(ctx)

	info := provider.Info(ctx)
	if info.Type != identity.ProviderTypeLinkerd {
		t.Errorf("expected type Linkerd, got %v", info.Type)
	}
	if info.TrustDomain != "cluster.local" {
		t.Errorf("expected trust domain cluster.local, got %s", info.TrustDomain)
	}
	if len(info.Capabilities) == 0 {
		t.Error("expected capabilities")
	}
}

func TestLinkerdProvider_NotAvailable(t *testing.T) {
	config := &LinkerdConfig{
		IdentityDir: "/nonexistent/path",
		CertPath:    "/nonexistent/cert.pem",
	}

	provider, _ := NewLinkerdProvider(config)
	if provider.IsAvailable() {
		t.Error("expected provider to not be available")
	}
}

func TestLinkerdProvider_ConstructSPIFFEID(t *testing.T) {
	provider, _ := NewLinkerdProvider(&LinkerdConfig{
		TrustDomain: "cluster.local",
	})

	tests := []struct {
		name     string
		dnsNames []string
		cn       string
		wantPath string
	}{
		{
			name:     "linkerd_identity_dns",
			dnsNames: []string{"mysa.mynamespace.serviceaccount.identity.linkerd.cluster.local"},
			wantPath: "/mynamespace/mysa",
		},
		{
			name:     "common_name_fallback",
			dnsNames: []string{},
			cn:       "test-service",
			wantPath: "/default/test-service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := &x509.Certificate{
				DNSNames: tt.dnsNames,
				Subject:  pkix.Name{CommonName: tt.cn},
			}
			spiffeID := provider.constructSPIFFEIDFromCert(cert)
			if spiffeID.Path != tt.wantPath {
				t.Errorf("expected path %s, got %s", tt.wantPath, spiffeID.Path)
			}
		})
	}
}

// Helper function tests

func TestParsePEMCertificates(t *testing.T) {
	certPEM, _, _ := createTestCertificate(t, "spiffe://test.local/test")

	certs, err := parsePEMCertificates(certPEM)
	if err != nil {
		t.Fatalf("failed to parse certificates: %v", err)
	}
	if len(certs) != 1 {
		t.Errorf("expected 1 certificate, got %d", len(certs))
	}
}

func TestParsePEMCertificates_Invalid(t *testing.T) {
	_, err := parsePEMCertificates([]byte("not a certificate"))
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestParsePEMPrivateKey(t *testing.T) {
	_, keyPEM, _ := createTestCertificate(t, "spiffe://test.local/test")

	key, err := parsePEMPrivateKey(keyPEM)
	if err != nil {
		t.Fatalf("failed to parse key: %v", err)
	}
	if key == nil {
		t.Error("expected key")
	}
}

func TestParsePEMPrivateKey_Invalid(t *testing.T) {
	_, err := parsePEMPrivateKey([]byte("not a key"))
	if err == nil {
		t.Error("expected error for invalid PEM")
	}
}

func TestExtractSPIFFEIDFromCert(t *testing.T) {
	certPEM, _, cert := createTestCertificate(t, "spiffe://test.local/ns/default/sa/test")
	_ = certPEM

	spiffeID, err := extractSPIFFEIDFromCert(cert)
	if err != nil {
		t.Fatalf("failed to extract SPIFFE ID: %v", err)
	}
	if spiffeID.TrustDomain != "test.local" {
		t.Errorf("expected trust domain test.local, got %s", spiffeID.TrustDomain)
	}
	if spiffeID.Path != "/ns/default/sa/test" {
		t.Errorf("expected path /ns/default/sa/test, got %s", spiffeID.Path)
	}
}

func TestExtractSPIFFEIDFromCert_NoSPIFFE(t *testing.T) {
	// Create a certificate without SPIFFE URI
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(time.Hour),
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	certDER, _ := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	cert, _ := x509.ParseCertificate(certDER)

	_, err := extractSPIFFEIDFromCert(cert)
	if err == nil {
		t.Error("expected error for cert without SPIFFE ID")
	}
}

func TestGetCertPaths(t *testing.T) {
	certChain, key, rootCert := GetCertPaths("/custom/base")
	if certChain != "/custom/base/cert-chain.pem" {
		t.Errorf("expected /custom/base/cert-chain.pem, got %s", certChain)
	}
	if key != "/custom/base/key.pem" {
		t.Errorf("expected /custom/base/key.pem, got %s", key)
	}
	if rootCert != "/custom/base/root-cert.pem" {
		t.Errorf("expected /custom/base/root-cert.pem, got %s", rootCert)
	}
}

func TestGetCertPaths_DefaultBase(t *testing.T) {
	certChain, _, _ := GetCertPaths("")
	if certChain != "/var/run/secrets/istio/cert-chain.pem" {
		t.Errorf("expected default path, got %s", certChain)
	}
}

// Watch tests

func TestIstioProvider_WatchTrustBundle(t *testing.T) {
	dir := createTestDir(t)
	certPEM, keyPEM, _ := createTestCertificate(t, "spiffe://test.local/ns/default/sa/test")
	certPath, keyPath, rootPath := writeTestCertFiles(t, dir, certPEM, keyPEM)

	config := &IstioConfig{
		TrustDomain:         "test.local",
		CertChainPath:       certPath,
		KeyPath:             keyPath,
		RootCertPath:        rootPath,
		RefreshInterval:     100 * time.Millisecond,
		HealthCheckInterval: time.Hour,
	}

	provider, _ := NewIstioProvider(config)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	provider.Start(ctx)
	defer provider.Stop(context.Background())

	ch, err := provider.WatchTrustBundle(ctx)
	if err != nil {
		t.Fatalf("failed to watch trust bundle: %v", err)
	}

	// Should receive initial bundle
	select {
	case bundle := <-ch:
		if bundle == nil {
			t.Error("expected bundle")
		}
	case <-ctx.Done():
		t.Error("timeout waiting for bundle")
	}
}

func TestConsulProvider_WatchTrustBundle(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/agent/self":
			json.NewEncoder(w).Encode(ConsulAgentResponse{
				Config: ConsulAgentConfig{Datacenter: "dc1"},
			})
		case "/v1/agent/connect/ca/roots":
			json.NewEncoder(w).Encode(ConsulCARoots{
				TrustDomain: "consul.local",
				Roots:       []ConsulCARoot{},
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := &ConsulConfig{
		TrustDomain:     "consul.local",
		HTTPAddr:        server.URL,
		RefreshInterval: 100 * time.Millisecond,
		HTTPTimeout:     10 * time.Second,
	}

	provider, _ := NewConsulProvider(config)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	ch, err := provider.WatchTrustBundle(ctx)
	if err != nil {
		t.Fatalf("failed to watch trust bundle: %v", err)
	}

	// Should receive initial bundle
	select {
	case bundle := <-ch:
		if bundle == nil {
			t.Error("expected bundle")
		}
	case <-ctx.Done():
		t.Error("timeout waiting for bundle")
	}
}

func TestLinkerdProvider_WatchTrustBundle(t *testing.T) {
	dir := createTestDir(t)
	certPEM, keyPEM, _ := createTestCertificate(t, "spiffe://cluster.local/default/test")
	certPath, keyPath, rootPath := writeTestCertFiles(t, dir, certPEM, keyPEM)

	config := &LinkerdConfig{
		TrustDomain:         "cluster.local",
		IdentityDir:         dir,
		CertPath:            certPath,
		KeyPath:             keyPath,
		TrustAnchorsPath:    rootPath,
		RefreshInterval:     100 * time.Millisecond,
		HealthCheckInterval: time.Hour,
	}

	provider, _ := NewLinkerdProvider(config)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	provider.Start(ctx)
	defer provider.Stop(context.Background())

	ch, err := provider.WatchTrustBundle(ctx)
	if err != nil {
		t.Fatalf("failed to watch trust bundle: %v", err)
	}

	// Should receive initial bundle
	select {
	case bundle := <-ch:
		if bundle == nil {
			t.Error("expected bundle")
		}
	case <-ctx.Done():
		t.Error("timeout waiting for bundle")
	}
}

// Attestation evidence tests

func TestIstioProvider_CreateAttestationEvidence(t *testing.T) {
	dir := createTestDir(t)
	certPEM, keyPEM, _ := createTestCertificate(t, "spiffe://test.local/ns/default/sa/test")
	certPath, keyPath, rootPath := writeTestCertFiles(t, dir, certPEM, keyPEM)

	// Create token file
	tokenPath := filepath.Join(dir, "token")
	os.WriteFile(tokenPath, []byte("test-token"), 0644)

	config := &IstioConfig{
		TrustDomain:             "test.local",
		CertChainPath:           certPath,
		KeyPath:                 keyPath,
		RootCertPath:            rootPath,
		ServiceAccountTokenPath: tokenPath,
		RefreshInterval:         time.Hour,
		HealthCheckInterval:     time.Hour,
	}

	provider, _ := NewIstioProvider(config)
	ctx := context.Background()
	provider.Start(ctx)
	defer provider.Stop(ctx)

	evidence, err := provider.CreateAttestationEvidence(ctx)
	if err != nil {
		t.Fatalf("failed to create attestation evidence: %v", err)
	}
	if evidence.Type != identity.AttestationTypeK8sSAT {
		t.Errorf("expected K8s SAT attestation type, got %v", evidence.Type)
	}
	if string(evidence.Data) != "test-token" {
		t.Errorf("expected test-token, got %s", string(evidence.Data))
	}
}

func TestLinkerdProvider_CreateAttestationEvidence(t *testing.T) {
	dir := createTestDir(t)
	certPEM, keyPEM, _ := createTestCertificate(t, "spiffe://cluster.local/default/test")
	certPath, keyPath, rootPath := writeTestCertFiles(t, dir, certPEM, keyPEM)

	// Create token file
	tokenPath := filepath.Join(dir, "token")
	os.WriteFile(tokenPath, []byte("test-token"), 0644)

	config := &LinkerdConfig{
		TrustDomain:         "cluster.local",
		IdentityDir:         dir,
		CertPath:            certPath,
		KeyPath:             keyPath,
		TrustAnchorsPath:    rootPath,
		TokenPath:           tokenPath,
		RefreshInterval:     time.Hour,
		HealthCheckInterval: time.Hour,
	}

	provider, _ := NewLinkerdProvider(config)
	ctx := context.Background()
	provider.Start(ctx)
	defer provider.Stop(ctx)

	evidence, err := provider.CreateAttestationEvidence(ctx)
	if err != nil {
		t.Fatalf("failed to create attestation evidence: %v", err)
	}
	if evidence.Type != identity.AttestationTypeK8sSAT {
		t.Errorf("expected K8s SAT attestation type, got %v", evidence.Type)
	}
}

// Client tests

func TestIstioSDSClient(t *testing.T) {
	client := NewIstioSDSClient("unix:///test/socket")
	if client.address != "unix:///test/socket" {
		t.Errorf("expected unix:///test/socket, got %s", client.address)
	}

	// FetchSecret should return not implemented error
	_, err := client.FetchSecret(context.Background(), "test")
	if err == nil {
		t.Error("expected error for not implemented")
	}
}

func TestLinkerdDestinationClient(t *testing.T) {
	client := NewLinkerdDestinationClient("localhost:8086")
	if client.addr != "localhost:8086" {
		t.Errorf("expected localhost:8086, got %s", client.addr)
	}

	// GetProfile should return not implemented error
	_, err := client.GetProfile(context.Background(), "test")
	if err == nil {
		t.Error("expected error for not implemented")
	}

	// GetIdentity should return not implemented error
	_, err = client.GetIdentity(context.Background(), "test")
	if err == nil {
		t.Error("expected error for not implemented")
	}
}
