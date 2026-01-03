package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/url"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"github.com/shawnbutts/keystone-core/pkg/config"
)

// Helper to generate a test certificate
func generateTestCert(t *testing.T, cn string, dnsNames []string, emails []string, uris []*url.URL) *x509.Certificate {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName:   cn,
			Organization: []string{"Test Org"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
		EmailAddresses:        emails,
		URIs:                  uris,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("Failed to create certificate: %v", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		t.Fatalf("Failed to parse certificate: %v", err)
	}

	return cert
}

// Helper to create a context with TLS peer info
func contextWithCert(t *testing.T, cert *x509.Certificate) context.Context {
	t.Helper()

	tlsInfo := credentials.TLSInfo{
		State: tls.ConnectionState{
			VerifiedChains: [][]*x509.Certificate{{cert}},
		},
	}

	p := &peer.Peer{
		Addr:     &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
		AuthInfo: tlsInfo,
	}

	return peer.NewContext(context.Background(), p)
}

func TestMTLSAuthenticator_NewWithEmptyConfig(t *testing.T) {
	cfg := config.MTLSAuthConfig{}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	if auth.Name() != "mtls" {
		t.Errorf("Expected name 'mtls', got '%s'", auth.Name())
	}
}

func TestMTLSAuthenticator_NewWithCertRoles(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		RequireClientCert: true,
		CertRoles: map[string]string{
			"*.admin.example.com": "admin",
			"*.ops.example.com":   "operator",
			"*":                   "readonly",
		},
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	if len(auth.certRoles) != 3 {
		t.Errorf("Expected 3 cert roles, got %d", len(auth.certRoles))
	}
}

func TestMTLSAuthenticator_NewWithInvalidRole(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		CertRoles: map[string]string{
			"*.example.com": "invalid-role",
		},
	}

	_, err := NewMTLSAuthenticator(cfg)
	if err == nil {
		t.Error("Expected error for invalid role")
	}
}

func TestMTLSAuthenticator_AuthenticateWithCN(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		RequireClientCert: true,
		CertRoles: map[string]string{
			"admin.example.com": "admin",
		},
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	cert := generateTestCert(t, "admin.example.com", nil, nil, nil)
	ctx := contextWithCert(t, cert)

	principal, err := auth.AuthenticateFromContext(ctx)
	if err != nil {
		t.Fatalf("AuthenticateFromContext failed: %v", err)
	}

	if principal.ID != "mtls:admin.example.com" {
		t.Errorf("Expected ID 'mtls:admin.example.com', got '%s'", principal.ID)
	}
	if principal.Name != "admin.example.com" {
		t.Errorf("Expected Name 'admin.example.com', got '%s'", principal.Name)
	}
	if principal.Role != RoleAdmin {
		t.Errorf("Expected Role 'admin', got '%s'", principal.Role)
	}
	if principal.AuthMethod != "mtls" {
		t.Errorf("Expected AuthMethod 'mtls', got '%s'", principal.AuthMethod)
	}
	if principal.Metadata["cn"] != "admin.example.com" {
		t.Errorf("Expected CN in metadata")
	}
}

func TestMTLSAuthenticator_AuthenticateWithDNS(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		CertRoles: map[string]string{
			"service.ops.example.com": "operator",
		},
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	cert := generateTestCert(t, "other-cn", []string{"service.ops.example.com"}, nil, nil)
	ctx := contextWithCert(t, cert)

	principal, err := auth.AuthenticateFromContext(ctx)
	if err != nil {
		t.Fatalf("AuthenticateFromContext failed: %v", err)
	}

	if principal.Role != RoleOperator {
		t.Errorf("Expected Role 'operator', got '%s'", principal.Role)
	}
	if principal.Metadata["dns_names"] != "service.ops.example.com" {
		t.Errorf("Expected DNS names in metadata, got '%s'", principal.Metadata["dns_names"])
	}
}

func TestMTLSAuthenticator_AuthenticateWithEmail(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		CertRoles: map[string]string{
			"admin@example.com": "admin",
		},
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	cert := generateTestCert(t, "", nil, []string{"admin@example.com"}, nil)
	ctx := contextWithCert(t, cert)

	principal, err := auth.AuthenticateFromContext(ctx)
	if err != nil {
		t.Fatalf("AuthenticateFromContext failed: %v", err)
	}

	if principal.Role != RoleAdmin {
		t.Errorf("Expected Role 'admin', got '%s'", principal.Role)
	}
	// When CN is empty, should fall back to email
	if principal.Name != "admin@example.com" {
		t.Errorf("Expected Name 'admin@example.com', got '%s'", principal.Name)
	}
}

func TestMTLSAuthenticator_AuthenticateWithURI(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		CertRoles: map[string]string{
			"spiffe://cluster.local/ns/admin/sa/admin-service": "admin",
		},
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	uri, _ := url.Parse("spiffe://cluster.local/ns/admin/sa/admin-service")
	cert := generateTestCert(t, "admin-service", nil, nil, []*url.URL{uri})
	ctx := contextWithCert(t, cert)

	principal, err := auth.AuthenticateFromContext(ctx)
	if err != nil {
		t.Fatalf("AuthenticateFromContext failed: %v", err)
	}

	if principal.Role != RoleAdmin {
		t.Errorf("Expected Role 'admin', got '%s'", principal.Role)
	}
}

func TestMTLSAuthenticator_GlobPatterns(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		CertRoles: map[string]string{
			"*.admin.example.com":  "admin",
			"svc-*.ops.example.com": "operator",
			"**":                   "readonly",
		},
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	tests := []struct {
		name     string
		cn       string
		expected Role
	}{
		{"wildcard admin", "service.admin.example.com", RoleAdmin},
		{"prefix operator", "svc-api.ops.example.com", RoleOperator},
		{"double wildcard fallback", "any.thing.else.com", RoleReadonly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert := generateTestCert(t, tt.cn, nil, nil, nil)
			ctx := contextWithCert(t, cert)

			principal, err := auth.AuthenticateFromContext(ctx)
			if err != nil {
				t.Fatalf("AuthenticateFromContext failed: %v", err)
			}

			if principal.Role != tt.expected {
				t.Errorf("Expected role %s, got %s", tt.expected, principal.Role)
			}
		})
	}
}

func TestMTLSAuthenticator_NoMatchingRole(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		RequireClientCert: true,
		CertRoles: map[string]string{
			"specific.example.com": "admin",
		},
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	cert := generateTestCert(t, "other.example.com", nil, nil, nil)
	ctx := contextWithCert(t, cert)

	_, err = auth.AuthenticateFromContext(ctx)
	if err != ErrNoMatchingCertRole {
		t.Errorf("Expected ErrNoMatchingCertRole, got %v", err)
	}
}

func TestMTLSAuthenticator_DefaultRole(t *testing.T) {
	// When no cert roles are configured, use default role
	cfg := config.MTLSAuthConfig{
		RequireClientCert: true,
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	cert := generateTestCert(t, "any.example.com", nil, nil, nil)
	ctx := contextWithCert(t, cert)

	principal, err := auth.AuthenticateFromContext(ctx)
	if err != nil {
		t.Fatalf("AuthenticateFromContext failed: %v", err)
	}

	// Default role is readonly
	if principal.Role != RoleReadonly {
		t.Errorf("Expected default role 'readonly', got '%s'", principal.Role)
	}
}

func TestMTLSAuthenticator_NoPeerInfo(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		RequireClientCert: true,
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	// Context without peer info
	ctx := context.Background()

	_, err = auth.AuthenticateFromContext(ctx)
	if err != ErrNoPeerInfo {
		t.Errorf("Expected ErrNoPeerInfo, got %v", err)
	}
}

func TestMTLSAuthenticator_NoTLSInfo(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		RequireClientCert: true,
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	// Context with peer but no TLS info
	p := &peer.Peer{
		Addr: &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
		// No AuthInfo
	}
	ctx := peer.NewContext(context.Background(), p)

	_, err = auth.AuthenticateFromContext(ctx)
	if err != ErrNoTLSInfo {
		t.Errorf("Expected ErrNoTLSInfo, got %v", err)
	}
}

func TestMTLSAuthenticator_NoClientCert(t *testing.T) {
	cfg := config.MTLSAuthConfig{
		RequireClientCert: true,
	}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	// Context with TLS but no verified chains
	tlsInfo := credentials.TLSInfo{
		State: tls.ConnectionState{
			VerifiedChains: nil,
		},
	}
	p := &peer.Peer{
		Addr:     &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 12345},
		AuthInfo: tlsInfo,
	}
	ctx := peer.NewContext(context.Background(), p)

	_, err = auth.AuthenticateFromContext(ctx)
	if err != ErrNoClientCert {
		t.Errorf("Expected ErrNoClientCert, got %v", err)
	}
}

func TestMTLSAuthenticator_AddRemoveCertRole(t *testing.T) {
	cfg := config.MTLSAuthConfig{}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	// Add a role
	err = auth.AddCertRole("*.admin.example.com", "admin")
	if err != nil {
		t.Fatalf("AddCertRole failed: %v", err)
	}

	cert := generateTestCert(t, "service.admin.example.com", nil, nil, nil)
	ctx := contextWithCert(t, cert)

	principal, err := auth.AuthenticateFromContext(ctx)
	if err != nil {
		t.Fatalf("AuthenticateFromContext failed: %v", err)
	}
	if principal.Role != RoleAdmin {
		t.Errorf("Expected role 'admin', got '%s'", principal.Role)
	}

	// Remove the role
	auth.RemoveCertRole("*.admin.example.com")

	// Now should get ErrNoMatchingCertRole since pattern was removed and no patterns left
	// But wait - after removing, there are no patterns, so it should use default role
	// Let me check the logic again...
	// Actually when certRoles is empty, it uses defaultRole
	principal, err = auth.AuthenticateFromContext(ctx)
	if err != nil {
		t.Fatalf("After removal, got error: %v", err)
	}
	if principal.Role != RoleReadonly {
		t.Errorf("Expected default role 'readonly' after removal, got '%s'", principal.Role)
	}
}

func TestMTLSAuthenticator_SetDefaultRole(t *testing.T) {
	cfg := config.MTLSAuthConfig{}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	auth.SetDefaultRole(RoleOperator)

	cert := generateTestCert(t, "any.example.com", nil, nil, nil)
	ctx := contextWithCert(t, cert)

	principal, err := auth.AuthenticateFromContext(ctx)
	if err != nil {
		t.Fatalf("AuthenticateFromContext failed: %v", err)
	}

	if principal.Role != RoleOperator {
		t.Errorf("Expected role 'operator', got '%s'", principal.Role)
	}
}

func TestMTLSAuthenticator_CertificateMetadata(t *testing.T) {
	cfg := config.MTLSAuthConfig{}

	auth, err := NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatalf("NewMTLSAuthenticator failed: %v", err)
	}

	cert := generateTestCert(t, "service.example.com",
		[]string{"dns1.example.com", "dns2.example.com"},
		[]string{"user@example.com"},
		nil,
	)
	ctx := contextWithCert(t, cert)

	principal, err := auth.AuthenticateFromContext(ctx)
	if err != nil {
		t.Fatalf("AuthenticateFromContext failed: %v", err)
	}

	// Check all metadata
	if principal.Metadata["cn"] != "service.example.com" {
		t.Errorf("Expected CN 'service.example.com', got '%s'", principal.Metadata["cn"])
	}
	if principal.Metadata["org"] != "Test Org" {
		t.Errorf("Expected org 'Test Org', got '%s'", principal.Metadata["org"])
	}
	if principal.Metadata["dns_names"] != "dns1.example.com,dns2.example.com" {
		t.Errorf("Expected DNS names, got '%s'", principal.Metadata["dns_names"])
	}
	if principal.Metadata["emails"] != "user@example.com" {
		t.Errorf("Expected emails, got '%s'", principal.Metadata["emails"])
	}
	if principal.Metadata["serial"] == "" {
		t.Error("Expected serial number in metadata")
	}
	if principal.Metadata["not_before"] == "" {
		t.Error("Expected not_before in metadata")
	}
	if principal.Metadata["not_after"] == "" {
		t.Error("Expected not_after in metadata")
	}
}

func TestCompileGlobPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		input    string
		expected bool
	}{
		// Simple wildcards
		{"*.example.com", "service.example.com", true},
		{"*.example.com", "service.sub.example.com", false}, // * doesn't match dots
		{"**.example.com", "service.sub.example.com", true}, // ** matches anything
		{"svc-*.example.com", "svc-api.example.com", true},
		{"svc-*.example.com", "api.example.com", false},

		// Question mark
		{"svc-?.example.com", "svc-1.example.com", true},
		{"svc-?.example.com", "svc-12.example.com", false},

		// Exact match
		{"exact.example.com", "exact.example.com", true},
		{"exact.example.com", "other.example.com", false},

		// Catch-all
		{"**", "anything.at.all", true},
		{"*", "nodots", true},
		{"*", "with.dots", false},

		// Email patterns
		{"*@example.com", "user@example.com", true},
		{"admin@**", "admin@anywhere.com", true}, // ** matches anything including dots

		// SPIFFE patterns
		{"spiffe://cluster.local/**", "spiffe://cluster.local/ns/default/sa/app", true},
	}

	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.input, func(t *testing.T) {
			regex, err := compileGlobPattern(tt.pattern)
			if err != nil {
				t.Fatalf("compileGlobPattern failed: %v", err)
			}

			result := regex.MatchString(tt.input)
			if result != tt.expected {
				t.Errorf("Pattern %q on input %q: expected %v, got %v", tt.pattern, tt.input, tt.expected, result)
			}
		})
	}
}

func TestCompileGlobPattern_Regex(t *testing.T) {
	// Test that regex patterns (starting with ^) are passed through
	pattern := "^.*\\.admin\\.example\\.com$"
	regex, err := compileGlobPattern(pattern)
	if err != nil {
		t.Fatalf("compileGlobPattern failed: %v", err)
	}

	if !regex.MatchString("service.admin.example.com") {
		t.Error("Expected regex to match")
	}
}

func TestCompileGlobPattern_Invalid(t *testing.T) {
	_, err := compileGlobPattern("")
	if err != ErrInvalidCertPattern {
		t.Errorf("Expected ErrInvalidCertPattern for empty pattern, got %v", err)
	}
}

func TestExtractClientCertFromContext(t *testing.T) {
	cert := generateTestCert(t, "test.example.com", nil, nil, nil)
	ctx := contextWithCert(t, cert)

	extracted, err := ExtractClientCertFromContext(ctx)
	if err != nil {
		t.Fatalf("ExtractClientCertFromContext failed: %v", err)
	}

	if extracted.Subject.CommonName != "test.example.com" {
		t.Errorf("Expected CN 'test.example.com', got '%s'", extracted.Subject.CommonName)
	}
}

func TestExtractClientCertFromContext_Errors(t *testing.T) {
	// No peer info
	_, err := ExtractClientCertFromContext(context.Background())
	if err != ErrNoPeerInfo {
		t.Errorf("Expected ErrNoPeerInfo, got %v", err)
	}

	// No TLS info
	p := &peer.Peer{Addr: &net.TCPAddr{}}
	ctx := peer.NewContext(context.Background(), p)
	_, err = ExtractClientCertFromContext(ctx)
	if err != ErrNoTLSInfo {
		t.Errorf("Expected ErrNoTLSInfo, got %v", err)
	}

	// No verified chains
	tlsInfo := credentials.TLSInfo{State: tls.ConnectionState{}}
	p = &peer.Peer{Addr: &net.TCPAddr{}, AuthInfo: tlsInfo}
	ctx = peer.NewContext(context.Background(), p)
	_, err = ExtractClientCertFromContext(ctx)
	if err != ErrNoClientCert {
		t.Errorf("Expected ErrNoClientCert, got %v", err)
	}
}
