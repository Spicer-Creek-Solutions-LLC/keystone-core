package identity

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"errors"
	"io"
	"log/slog"
	"math/big"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---- helpers ----------------------------------------------------

func newTestProviderForTLS(t *testing.T) (*EmbeddedProvider, *InMemoryJoinTokenStore) {
	t.Helper()
	store := NewInMemoryJoinTokenStore()
	caStorage, err := NewFileCAStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	cfg := EmbeddedProviderConfig{
		CAConfig: shortLifetimeCAConfigTLS(),
		Storage:  caStorage,
		// Long rotator interval — tests that need rotation drive
		// it manually via the provider's RotateSigningCA helper.
		RotatorInterval: time.Hour,
		JoinTokenStore:  store,
		Logger:          slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	provider, err := NewEmbeddedProvider(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if err := provider.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = provider.Stop(stopCtx)
	})
	return provider, store
}

// shortLifetimeCAConfigTLS — a tighter version of the constructor
// in ca_test.go, copied here so tls_test.go doesn't reach into
// the file-local helper.
func shortLifetimeCAConfigTLS() CAConfig {
	c := DefaultCAConfig(DefaultTrustDomain)
	c.RootCATTL = 10 * time.Hour
	c.SigningCATTL = 2 * time.Hour
	c.RotateBefore = 30 * time.Minute
	c.DefaultSVIDTTL = 15 * time.Minute
	c.MaxSVIDTTL = time.Hour
	return c
}

// mintAgentClientCert issues a real X509SVID for `spiffe://td/agent/<id>`
// through the provider and projects it into a *tls.Certificate so
// the test client can present it during the handshake.
func mintAgentClientCert(t *testing.T, p *EmbeddedProvider, agentID string) tls.Certificate {
	t.Helper()
	id, err := AgentID(p.TrustDomain(), agentID)
	if err != nil {
		t.Fatalf("AgentID: %v", err)
	}
	svid, err := p.IssueX509SVID(context.Background(), IssueX509SVIDRequest{
		ID:  id,
		TTL: 30 * time.Minute,
	})
	if err != nil {
		t.Fatalf("IssueX509SVID: %v", err)
	}
	chainRaw := make([][]byte, 0, len(svid.Chain()))
	for _, c := range svid.Chain() {
		chainRaw = append(chainRaw, c.Raw)
	}
	return tls.Certificate{
		Certificate: chainRaw,
		PrivateKey:  svid.PrivateKey(),
		Leaf:        svid.Leaf(),
	}
}

// runHandshake stands up a listener with `serverConfig`, dials it
// from a client using `clientConfig`, and returns the resulting
// handshake error (or nil on success). Always tears down both
// sides cleanly via t.Cleanup.
func runHandshake(t *testing.T, serverConfig, clientConfig *tls.Config) error {
	t.Helper()
	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverConfig)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	var (
		serverErr error
		wg        sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		conn, err := listener.Accept()
		if err != nil {
			serverErr = err
			return
		}
		defer func() { _ = conn.Close() }()
		// Force the handshake to complete server-side so any
		// client-cert verification error surfaces here.
		if tc, ok := conn.(*tls.Conn); ok {
			serverErr = tc.HandshakeContext(context.Background())
			_ = tc.CloseWrite()
		}
	}()

	conn, err := tls.Dial("tcp", listener.Addr().String(), clientConfig)
	if err != nil {
		wg.Wait()
		// Client-side dial error usually wraps the server's TLS
		// alert; return it as the failure.
		return err
	}
	_ = conn.Close()
	wg.Wait()
	return serverErr
}

// ---- BuildServerTLSConfig --------------------------------------

func TestBuildServerTLSConfig_NilProvider(t *testing.T) {
	t.Parallel()
	_, cancel, err := BuildServerTLSConfig(context.Background(), nil, ServerRoleControlPlane, nil)
	t.Cleanup(cancel)
	if err == nil || !errors.Is(err, ErrTLSConfig) {
		t.Errorf("err = %v, want ErrTLSConfig", err)
	}
}

func TestBuildServerTLSConfig_ProviderNotRunning(t *testing.T) {
	t.Parallel()
	store := NewInMemoryJoinTokenStore()
	caStorage, _ := NewFileCAStorage(t.TempDir())
	p, _ := NewEmbeddedProvider(EmbeddedProviderConfig{
		CAConfig:       shortLifetimeCAConfigTLS(),
		Storage:        caStorage,
		JoinTokenStore: store,
	})
	// Don't Start — Health should fail.
	_, cancel, err := BuildServerTLSConfig(context.Background(), p, ServerRoleControlPlane, nil)
	t.Cleanup(cancel)
	if err == nil || !errors.Is(err, ErrTLSConfig) {
		t.Errorf("err = %v, want ErrTLSConfig", err)
	}
}

func TestBuildServerTLSConfig_HappyHandshake(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)
	serverCfg, cancel, err := BuildServerTLSConfig(context.Background(), p, ServerRoleControlPlane, nil)
	if err != nil {
		t.Fatalf("BuildServerTLSConfig: %v", err)
	}
	t.Cleanup(cancel)

	// Build a client config that presents an agent SVID + trusts
	// the provider's root.
	rootPool := x509.NewCertPool()
	bundle, _ := p.GetTrustBundle(context.Background())
	for _, c := range bundle.X509Authorities() {
		rootPool.AddCert(c)
	}
	clientCert := mintAgentClientCert(t, p, "agent-handshake")
	clientCfg := &tls.Config{
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS13,
		RootCAs:      rootPool,
		Certificates: []tls.Certificate{clientCert},
	}

	if err := runHandshake(t, serverCfg, clientCfg); err != nil {
		t.Fatalf("handshake: %v", err)
	}
}

func TestBuildServerTLSConfig_RejectsTLS12(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)
	serverCfg, cancel, err := BuildServerTLSConfig(context.Background(), p, ServerRoleControlPlane, nil)
	if err != nil {
		t.Fatalf("BuildServerTLSConfig: %v", err)
	}
	t.Cleanup(cancel)

	rootPool := x509.NewCertPool()
	bundle, _ := p.GetTrustBundle(context.Background())
	for _, c := range bundle.X509Authorities() {
		rootPool.AddCert(c)
	}
	// Force TLS 1.2 max — server requires 1.3.
	clientCfg := &tls.Config{
		ServerName: "localhost",
		MinVersion: tls.VersionTLS12,
		MaxVersion: tls.VersionTLS12,
		RootCAs:    rootPool,
	}
	err = runHandshake(t, serverCfg, clientCfg)
	if err == nil {
		t.Fatal("TLS 1.2 client must be rejected by 1.3-min server")
	}
	// Error wording varies across Go versions; just check the
	// handshake failed.
	if !strings.Contains(err.Error(), "tls") && !strings.Contains(err.Error(), "handshake") {
		t.Errorf("err = %v; want TLS/handshake error", err)
	}
}

func TestBuildServerTLSConfig_RejectsForeignClientCert(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)
	serverCfg, cancel, err := BuildServerTLSConfig(context.Background(), p, ServerRoleControlPlane,
		&ServerTLSOptions{ClientAuth: tls.RequireAndVerifyClientCert}, // strict mTLS
	)
	if err != nil {
		t.Fatalf("BuildServerTLSConfig: %v", err)
	}
	t.Cleanup(cancel)

	// Build a self-signed client cert that the server's trust
	// bundle won't recognize.
	foreignKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	foreignCert := selfSignedForeignCert(t, foreignKey)

	rootPool := x509.NewCertPool()
	bundle, _ := p.GetTrustBundle(context.Background())
	for _, c := range bundle.X509Authorities() {
		rootPool.AddCert(c)
	}
	clientCfg := &tls.Config{
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS13,
		RootCAs:      rootPool,
		Certificates: []tls.Certificate{foreignCert},
	}
	err = runHandshake(t, serverCfg, clientCfg)
	if err == nil {
		t.Fatal("foreign client cert must be rejected under RequireAndVerifyClientCert")
	}
}

func TestBuildServerTLSConfig_VerifyIfGiven_NoClientCert(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)
	// Default opts → ClientAuth = VerifyClientCertIfGiven.
	serverCfg, cancel, err := BuildServerTLSConfig(context.Background(), p, ServerRoleControlPlane, nil)
	if err != nil {
		t.Fatalf("BuildServerTLSConfig: %v", err)
	}
	t.Cleanup(cancel)

	rootPool := x509.NewCertPool()
	bundle, _ := p.GetTrustBundle(context.Background())
	for _, c := range bundle.X509Authorities() {
		rootPool.AddCert(c)
	}
	clientCfg := &tls.Config{
		ServerName: "localhost",
		MinVersion: tls.VersionTLS13,
		RootCAs:    rootPool,
		// No Certificates — client connects anonymously over TLS.
	}
	if err := runHandshake(t, serverCfg, clientCfg); err != nil {
		t.Errorf("API-key-style client (no peer cert) should succeed under VerifyClientCertIfGiven, got: %v", err)
	}
}

func TestBuildServerTLSConfig_RefreshAfterRotation(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)
	serverCfg, cancel, err := BuildServerTLSConfig(context.Background(), p, ServerRoleControlPlane, nil)
	if err != nil {
		t.Fatalf("BuildServerTLSConfig: %v", err)
	}
	t.Cleanup(cancel)

	// Capture the current server cert's signing-CA serial.
	cert1, err := serverCfg.GetCertificate(&tls.ClientHelloInfo{})
	if err != nil {
		t.Fatalf("GetCertificate before rotation: %v", err)
	}
	signing1 := cert1.Certificate[1] // [leaf, signingCA]

	// Force a rotation through the provider — same call path the
	// gRPC RotateSigningCA RPC uses.
	if err := p.RotateSigningCA(context.Background()); err != nil {
		t.Fatalf("RotateSigningCA: %v", err)
	}

	// Wait up to 2s for the watcher to consume the bundle update +
	// reissue the server cert.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		cert2, err := serverCfg.GetCertificate(&tls.ClientHelloInfo{})
		if err != nil {
			t.Fatalf("GetCertificate post-rotation: %v", err)
		}
		if string(cert2.Certificate[1]) != string(signing1) {
			// Watcher caught up — cert now anchored under the new
			// signing CA.
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server cert never refreshed after rotation within 2s")
}

func TestBuildServerTLSConfig_CancelStopsWatcher(t *testing.T) {
	t.Parallel()
	p, _ := newTestProviderForTLS(t)
	_, cancel, err := BuildServerTLSConfig(context.Background(), p, ServerRoleControlPlane, nil)
	if err != nil {
		t.Fatalf("BuildServerTLSConfig: %v", err)
	}
	// Should return without panic / hang.
	cancel()
	// Second cancel safe.
	cancel()
}

// ---- selfSignedForeignCert helper -------------------------------

// selfSignedForeignCert mints a self-signed cert (CA = self) that
// carries a SPIFFE URI SAN from a DIFFERENT trust domain. The
// provider's root won't anchor it, so strict-mTLS handshakes
// reject it.
func selfSignedForeignCert(t *testing.T, key *ecdsa.PrivateKey) tls.Certificate {
	t.Helper()
	foreignID, _ := AgentID("foreign.org", "rogue")
	uri := foreignID.URI()
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		Subject:      pkix.Name{CommonName: "rogue-client"},
		URIs:         []*url.URL{uri},
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	parsed, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return tls.Certificate{
		Certificate: [][]byte{der},
		PrivateKey:  key,
		Leaf:        parsed,
	}
}
