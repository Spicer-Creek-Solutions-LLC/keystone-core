package auth_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"math/big"
	"net/url"
	"testing"
	"time"

	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"

	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// makeSPIFFECert returns a self-signed cert with a single
// `spiffe://<td>/<path>` URI SAN. The cert is its own issuer; tests
// place it in VerifiedChains directly so the chain-build doesn't run.
func makeSPIFFECert(t *testing.T, trustDomain, path string) *x509.Certificate {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("genkey: %v", err)
	}
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    now.Add(-time.Hour),
		NotAfter:     now.Add(time.Hour),
		URIs: []*url.URL{
			{Scheme: "spiffe", Host: trustDomain, Path: "/" + path},
		},
		KeyUsage:    x509.KeyUsageDigitalSignature,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert
}

// ctxWithPeerCert constructs a context that mimics gRPC's peer-info
// shape after a verified TLS handshake.
func ctxWithPeerCert(cert *x509.Certificate) context.Context {
	authInfo := credentials.TLSInfo{}
	if cert != nil {
		authInfo.State.VerifiedChains = [][]*x509.Certificate{{cert}}
	}
	p := &peer.Peer{AuthInfo: authInfo}
	return peer.NewContext(context.Background(), p)
}

// rolePathResolver maps SPIFFE paths to roles for tests.
func rolePathResolver(path string) (auth.Role, error) {
	switch {
	case path == "server/control-plane":
		return auth.RoleAdmin, nil
	case len(path) > 6 && path[:6] == "agent/":
		return auth.RoleOperator, nil
	default:
		return auth.RoleNone, errors.New("unknown SPIFFE path")
	}
}

func TestMTLSAuthenticator_Success_Server(t *testing.T) {
	cfg := auth.MTLSConfig{TrustDomain: "kscore.local", RoleResolver: rolePathResolver}
	a, err := auth.NewMTLSAuthenticator(cfg)
	if err != nil {
		t.Fatal(err)
	}

	cert := makeSPIFFECert(t, "kscore.local", "server/control-plane")
	got, err := a.Authenticate(ctxWithPeerCert(cert))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.Role != auth.RoleAdmin {
		t.Errorf("Role = %v, want admin", got.Role)
	}
	if got.AuthMethod != auth.AuthMethodMTLS {
		t.Errorf("AuthMethod = %v, want mtls", got.AuthMethod)
	}
	if got.ID != "spiffe://kscore.local/server/control-plane" {
		t.Errorf("ID = %q", got.ID)
	}
}

func TestMTLSAuthenticator_Success_Agent(t *testing.T) {
	cfg := auth.MTLSConfig{TrustDomain: "kscore.local", RoleResolver: rolePathResolver}
	a, _ := auth.NewMTLSAuthenticator(cfg)

	cert := makeSPIFFECert(t, "kscore.local", "agent/abc-123")
	got, err := a.Authenticate(ctxWithPeerCert(cert))
	if err != nil {
		t.Fatalf("Authenticate: %v", err)
	}
	if got.Role != auth.RoleOperator {
		t.Errorf("Role = %v, want operator", got.Role)
	}
}

func TestMTLSAuthenticator_NoPeerInfo(t *testing.T) {
	a, _ := auth.NewMTLSAuthenticator(auth.MTLSConfig{TrustDomain: "kscore.local", RoleResolver: rolePathResolver})
	_, err := a.Authenticate(context.Background())
	if !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestMTLSAuthenticator_NoVerifiedChain(t *testing.T) {
	a, _ := auth.NewMTLSAuthenticator(auth.MTLSConfig{TrustDomain: "kscore.local", RoleResolver: rolePathResolver})
	// peer with TLSInfo but empty VerifiedChains — the TLS handshake
	// didn't complete cert verification. Treat as "no credential".
	authInfo := credentials.TLSInfo{}
	p := &peer.Peer{AuthInfo: authInfo}
	ctx := peer.NewContext(context.Background(), p)

	_, err := a.Authenticate(ctx)
	if !errors.Is(err, auth.ErrCredentialsNotFound) {
		t.Errorf("err = %v, want ErrCredentialsNotFound", err)
	}
}

func TestMTLSAuthenticator_NonSPIFFECert(t *testing.T) {
	a, _ := auth.NewMTLSAuthenticator(auth.MTLSConfig{TrustDomain: "kscore.local", RoleResolver: rolePathResolver})

	// Cert without a spiffe:// URI SAN.
	priv, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, _ := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	cert, _ := x509.ParseCertificate(der)

	_, err := a.Authenticate(ctxWithPeerCert(cert))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestMTLSAuthenticator_WrongTrustDomain(t *testing.T) {
	a, _ := auth.NewMTLSAuthenticator(auth.MTLSConfig{TrustDomain: "kscore.local", RoleResolver: rolePathResolver})

	cert := makeSPIFFECert(t, "evil.example.com", "server/whatever")
	_, err := a.Authenticate(ctxWithPeerCert(cert))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestMTLSAuthenticator_RoleResolverRejects(t *testing.T) {
	a, _ := auth.NewMTLSAuthenticator(auth.MTLSConfig{TrustDomain: "kscore.local", RoleResolver: rolePathResolver})

	cert := makeSPIFFECert(t, "kscore.local", "unknown/path")
	_, err := a.Authenticate(ctxWithPeerCert(cert))
	if !errors.Is(err, auth.ErrInvalidCredentials) {
		t.Errorf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestNewMTLSAuthenticator_Validates(t *testing.T) {
	if _, err := auth.NewMTLSAuthenticator(auth.MTLSConfig{}); err == nil {
		t.Error("expected error for missing TrustDomain")
	}
	if _, err := auth.NewMTLSAuthenticator(auth.MTLSConfig{TrustDomain: "x"}); err == nil {
		t.Error("expected error for missing RoleResolver")
	}
}
