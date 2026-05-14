package controlplane

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/identity"
)

// ---- shared fixtures --------------------------------------------

// newJTBootstrapProvider builds a started [identity.EmbeddedProvider]
// + an in-memory JoinTokenStore + a join-token attestor. Tests use
// the provider for both the validator (token verification) AND the
// SVID issuer (cert minting), matching the production wiring.
func newJTBootstrapProvider(t *testing.T) (*identity.EmbeddedProvider, *identity.InMemoryJoinTokenStore, *identity.JoinTokenAttestor) {
	t.Helper()
	store := identity.NewInMemoryJoinTokenStore()
	caStorage, err := identity.NewFileCAStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileCAStorage: %v", err)
	}
	att, err := identity.NewJoinTokenAttestor(identity.JoinTokenAttestorConfig{
		Store:       store,
		TrustDomain: identity.DefaultTrustDomain,
	})
	if err != nil {
		t.Fatalf("NewJoinTokenAttestor: %v", err)
	}
	cfg := identity.EmbeddedProviderConfig{
		CAConfig:        jtBootstrapCAConfig(),
		Storage:         caStorage,
		RotatorInterval: time.Hour,
		JoinTokenStore:  store,
		Attestors:       []identity.Attestor{att},
	}
	provider, err := identity.NewEmbeddedProvider(cfg)
	if err != nil {
		t.Fatalf("NewEmbeddedProvider: %v", err)
	}
	if err := provider.Start(context.Background()); err != nil {
		t.Fatalf("provider.Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = provider.Stop(stopCtx)
	})
	return provider, store, att
}

func jtBootstrapCAConfig() identity.CAConfig {
	c := identity.DefaultCAConfig(identity.DefaultTrustDomain)
	c.RootCATTL = 10 * time.Hour
	c.SigningCATTL = 2 * time.Hour
	c.RotateBefore = 30 * time.Minute
	c.DefaultSVIDTTL = 15 * time.Minute
	c.MaxSVIDTTL = time.Hour
	return c
}

// mintAgentToken creates a join token via the provider's
// CreateJoinToken path and returns the cleartext + ID. Mirrors
// the kscore-identity CLI flow.
func mintAgentToken(t *testing.T, p *identity.EmbeddedProvider, agentID string) (cleartext, recordID string) {
	t.Helper()
	tok, err := p.CreateJoinToken(context.Background(), identity.CreateJoinTokenRequest{
		AgentID: agentID,
	})
	if err != nil {
		t.Fatalf("CreateJoinToken: %v", err)
	}
	return tok.Token, tok.ID
}

// stubBaseIssuer is the smallest CredentialIssuer that satisfies
// SVIDBootstrapIssuer's contract: returns a fixed API key + the
// passed-in agent ID. Lets the tests assert the wrapper merges
// the SVID fields without mutating the base credentials.
type stubBaseIssuer struct {
	apiKey  string
	err     error
	issueAt time.Time
}

func (s *stubBaseIssuer) Issue(_ context.Context, agentID string) (AgentCredentials, error) {
	if s.err != nil {
		return AgentCredentials{}, s.err
	}
	t := s.issueAt
	if t.IsZero() {
		t = time.Now().UTC()
	}
	return AgentCredentials{
		APIKey:   s.apiKey,
		AgentID:  agentID,
		IssuedAt: t,
	}, nil
}

// ---- JoinTokenBootstrapValidator tests --------------------------

func TestNewJoinTokenBootstrapValidator_RequiresAttestor(t *testing.T) {
	t.Parallel()
	_, err := NewJoinTokenBootstrapValidator(nil, identity.DefaultTrustDomain)
	if err == nil {
		t.Error("nil attestor accepted")
	}
}

func TestNewJoinTokenBootstrapValidator_RequiresTrustDomain(t *testing.T) {
	t.Parallel()
	_, _, att := newJTBootstrapProvider(t)
	if _, err := NewJoinTokenBootstrapValidator(att, ""); err == nil {
		t.Error("empty trust domain accepted")
	}
}

func TestJoinTokenBootstrapValidator_HappyPath(t *testing.T) {
	t.Parallel()
	p, store, att := newJTBootstrapProvider(t)
	v, err := NewJoinTokenBootstrapValidator(att, p.TrustDomain())
	if err != nil {
		t.Fatalf("NewJoinTokenBootstrapValidator: %v", err)
	}
	cleartext, recordID := mintAgentToken(t, p, "agent-happy")

	if err := v.Validate(context.Background(), "agent-happy", []byte(cleartext)); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Store now reports the token as used.
	rec, err := store.Get(context.Background(), recordID)
	if err != nil {
		t.Fatalf("store.Get: %v", err)
	}
	if rec.UsedCount != 1 {
		t.Errorf("UsedCount = %d, want 1 after Validate", rec.UsedCount)
	}
}

func TestJoinTokenBootstrapValidator_EmptyClaimedID(t *testing.T) {
	t.Parallel()
	p, _, att := newJTBootstrapProvider(t)
	v, _ := NewJoinTokenBootstrapValidator(att, p.TrustDomain())
	err := v.Validate(context.Background(), "", []byte("kscore-join-anything"))
	if err == nil {
		t.Fatal("empty claimedID should reject")
	}
}

func TestJoinTokenBootstrapValidator_EmptyProof(t *testing.T) {
	t.Parallel()
	p, _, att := newJTBootstrapProvider(t)
	v, _ := NewJoinTokenBootstrapValidator(att, p.TrustDomain())
	err := v.Validate(context.Background(), "agent-x", nil)
	if err == nil {
		t.Fatal("empty proof should reject")
	}
}

func TestJoinTokenBootstrapValidator_UnknownToken(t *testing.T) {
	t.Parallel()
	p, _, att := newJTBootstrapProvider(t)
	v, _ := NewJoinTokenBootstrapValidator(att, p.TrustDomain())
	// A well-formed prefix but never minted → store miss.
	err := v.Validate(context.Background(), "agent-x",
		[]byte("kscore-join-UNKNOWNX"+strings.Repeat("A", 32)))
	if !errors.Is(err, identity.ErrAttestation) {
		t.Errorf("err = %v, want wrapped ErrAttestation", err)
	}
}

func TestJoinTokenBootstrapValidator_Exhausted(t *testing.T) {
	t.Parallel()
	p, _, att := newJTBootstrapProvider(t)
	v, _ := NewJoinTokenBootstrapValidator(att, p.TrustDomain())
	cleartext, _ := mintAgentToken(t, p, "agent-exhausted")

	// First use consumes (MaxUses default = 1).
	if err := v.Validate(context.Background(), "agent-exhausted", []byte(cleartext)); err != nil {
		t.Fatalf("first Validate: %v", err)
	}
	// Second use → exhausted.
	err := v.Validate(context.Background(), "agent-exhausted", []byte(cleartext))
	if !errors.Is(err, identity.ErrJoinTokenExhausted) {
		t.Errorf("err = %v, want wrapped ErrJoinTokenExhausted", err)
	}
}

func TestJoinTokenBootstrapValidator_AgentMismatch(t *testing.T) {
	t.Parallel()
	p, _, att := newJTBootstrapProvider(t)
	v, _ := NewJoinTokenBootstrapValidator(att, p.TrustDomain())
	cleartext, _ := mintAgentToken(t, p, "agent-alpha")

	err := v.Validate(context.Background(), "agent-beta", []byte(cleartext))
	if !errors.Is(err, ErrJoinTokenAgentMismatch) {
		t.Errorf("err = %v, want ErrJoinTokenAgentMismatch", err)
	}
	if !strings.Contains(err.Error(), "agent-alpha") || !strings.Contains(err.Error(), "agent-beta") {
		t.Errorf("err message %q missing both ids", err)
	}
}

// ---- SVIDBootstrapIssuer tests ----------------------------------

func TestNewSVIDBootstrapIssuer_Validation(t *testing.T) {
	t.Parallel()
	p, _, _ := newJTBootstrapProvider(t)
	base := &stubBaseIssuer{apiKey: "test"}
	cases := []struct {
		name string
		fn   func() error
	}{
		{"nil provider", func() error {
			_, err := NewSVIDBootstrapIssuer(nil, base, 0)
			return err
		}},
		{"nil base", func() error {
			_, err := NewSVIDBootstrapIssuer(p, nil, 0)
			return err
		}},
		{"negative ttl", func() error {
			_, err := NewSVIDBootstrapIssuer(p, base, -time.Second)
			return err
		}},
	}
	for _, c := range cases {
		c := c
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			if err := c.fn(); err == nil {
				t.Error("expected error")
			}
		})
	}
}

func TestSVIDBootstrapIssuer_HappyPath(t *testing.T) {
	t.Parallel()
	p, _, _ := newJTBootstrapProvider(t)
	base := &stubBaseIssuer{apiKey: "api-key-from-base"}
	issuer, err := NewSVIDBootstrapIssuer(p, base, 0)
	if err != nil {
		t.Fatalf("NewSVIDBootstrapIssuer: %v", err)
	}
	creds, err := issuer.Issue(context.Background(), "agent-happy")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if creds.APIKey != "api-key-from-base" {
		t.Errorf("APIKey = %q, want propagated", creds.APIKey)
	}
	if creds.AgentID != "agent-happy" {
		t.Errorf("AgentID = %q", creds.AgentID)
	}
	if creds.CertChainPEM == "" {
		t.Fatal("CertChainPEM empty")
	}
	if creds.PrivateKeyPEM == "" {
		t.Fatal("PrivateKeyPEM empty")
	}
	if creds.TrustBundlePEM == "" {
		t.Fatal("TrustBundlePEM empty")
	}
}

func TestSVIDBootstrapIssuer_LeafSPIFFEIDMatches(t *testing.T) {
	t.Parallel()
	p, _, _ := newJTBootstrapProvider(t)
	base := &stubBaseIssuer{apiKey: "k"}
	issuer, _ := NewSVIDBootstrapIssuer(p, base, 0)

	creds, err := issuer.Issue(context.Background(), "agent-spiffe-check")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	leaf := firstCertFromPEM(t, creds.CertChainPEM)
	if len(leaf.URIs) != 1 {
		t.Fatalf("leaf URIs = %v, want exactly 1", leaf.URIs)
	}
	got := leaf.URIs[0].String()
	want := "spiffe://" + p.TrustDomain() + "/agent/agent-spiffe-check"
	if got != want {
		t.Errorf("URI SAN = %q, want %q", got, want)
	}
}

func TestSVIDBootstrapIssuer_ChainHasTwoBlocks(t *testing.T) {
	t.Parallel()
	p, _, _ := newJTBootstrapProvider(t)
	issuer, _ := NewSVIDBootstrapIssuer(p, &stubBaseIssuer{apiKey: "k"}, 0)
	creds, _ := issuer.Issue(context.Background(), "agent-chain")

	blocks := decodeAllPEM(t, []byte(creds.CertChainPEM))
	if len(blocks) != 2 {
		t.Errorf("chain blocks = %d, want 2 (leaf + signing CA)", len(blocks))
	}
}

func TestSVIDBootstrapIssuer_BundleParses(t *testing.T) {
	t.Parallel()
	p, _, _ := newJTBootstrapProvider(t)
	issuer, _ := NewSVIDBootstrapIssuer(p, &stubBaseIssuer{apiKey: "k"}, 0)
	creds, _ := issuer.Issue(context.Background(), "agent-bundle")

	blocks := decodeAllPEM(t, []byte(creds.TrustBundlePEM))
	if len(blocks) == 0 {
		t.Fatal("bundle PEM has zero blocks")
	}
	for _, b := range blocks {
		if _, err := x509.ParseCertificate(b.Bytes); err != nil {
			t.Errorf("bundle block won't parse: %v", err)
		}
	}
}

func TestSVIDBootstrapIssuer_KeyMatchesCert(t *testing.T) {
	t.Parallel()
	p, _, _ := newJTBootstrapProvider(t)
	issuer, _ := NewSVIDBootstrapIssuer(p, &stubBaseIssuer{apiKey: "k"}, 0)
	creds, _ := issuer.Issue(context.Background(), "agent-key-match")

	// tls.X509KeyPair both parses + checks the key matches the
	// cert — catches encoding mismatches in one call.
	if _, err := tls.X509KeyPair([]byte(creds.CertChainPEM), []byte(creds.PrivateKeyPEM)); err != nil {
		t.Errorf("X509KeyPair: %v", err)
	}
}

func TestSVIDBootstrapIssuer_TLSHandshakeRoundTrip(t *testing.T) {
	t.Parallel()
	p, _, _ := newJTBootstrapProvider(t)
	issuer, _ := NewSVIDBootstrapIssuer(p, &stubBaseIssuer{apiKey: "k"}, 0)
	agentCreds, err := issuer.Issue(context.Background(), "agent-tls-rt")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}

	// Build server-side TLS config from the provider (task 13's
	// BuildServerTLSConfig). The client side uses the freshly-
	// issued SVID + trust bundle, exactly as a real agent would.
	serverCfg, cancel, err := identity.BuildServerTLSConfig(
		context.Background(), p, identity.ServerRoleControlPlane,
		&identity.ServerTLSOptions{ClientAuth: tls.RequireAndVerifyClientCert},
	)
	if err != nil {
		t.Fatalf("BuildServerTLSConfig: %v", err)
	}
	t.Cleanup(cancel)

	clientCert, err := tls.X509KeyPair([]byte(agentCreds.CertChainPEM), []byte(agentCreds.PrivateKeyPEM))
	if err != nil {
		t.Fatalf("X509KeyPair: %v", err)
	}
	clientPool := x509.NewCertPool()
	if !clientPool.AppendCertsFromPEM([]byte(agentCreds.TrustBundlePEM)) {
		t.Fatal("AppendCertsFromPEM: no certs added")
	}
	clientCfg := &tls.Config{
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS13,
		RootCAs:      clientPool,
		Certificates: []tls.Certificate{clientCert},
	}

	// Stand up a listener + dial. Verifies BOTH directions:
	// the server's cert is anchored under the bundle's root
	// AND the agent's cert chains to that same root (required
	// because we set RequireAndVerifyClientCert).
	listener, err := tls.Listen("tcp", "127.0.0.1:0", serverCfg)
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
		if tc, ok := conn.(*tls.Conn); ok {
			serverErr = tc.HandshakeContext(context.Background())
			_ = tc.CloseWrite()
		}
	}()
	conn, err := tls.Dial("tcp", listener.Addr().String(), clientCfg)
	if err != nil {
		wg.Wait()
		t.Fatalf("tls.Dial: %v", err)
	}
	_ = conn.Close()
	wg.Wait()
	if serverErr != nil {
		t.Fatalf("server handshake: %v", serverErr)
	}
}

func TestSVIDBootstrapIssuer_BaseError(t *testing.T) {
	t.Parallel()
	p, _, _ := newJTBootstrapProvider(t)
	base := &stubBaseIssuer{err: errors.New("synthetic base failure")}
	issuer, _ := NewSVIDBootstrapIssuer(p, base, 0)
	_, err := issuer.Issue(context.Background(), "agent-fail")
	if err == nil {
		t.Fatal("base issuer error should propagate")
	}
	if !strings.Contains(err.Error(), "synthetic base failure") {
		t.Errorf("err = %v; want base failure cited", err)
	}
}

func TestSVIDBootstrapIssuer_BadAgentID(t *testing.T) {
	t.Parallel()
	p, _, _ := newJTBootstrapProvider(t)
	issuer, _ := NewSVIDBootstrapIssuer(p, &stubBaseIssuer{apiKey: "k"}, 0)
	_, err := issuer.Issue(context.Background(), "")
	if err == nil {
		t.Error("empty agentID accepted")
	}
}

// ---- end-to-end through BootstrapHandler ------------------------

// TestBootstrapJoinTokenE2E_ValidatorPlusIssuer drives the
// validator + issuer through the same code path the production
// BootstrapHandler uses, verifying that:
//
//   - validator consumes the token (UsedCount → 1)
//   - issuer returns a complete AgentCredentials payload
//     (APIKey from base + CertChainPEM / PrivateKeyPEM /
//      TrustBundlePEM from the SVID side)
//   - replay of the same token fails (token now exhausted)
//
// The BootstrapHandler is exercised in
// internal/controlplane/bootstrap_test.go +
// bootstrap_integration_test.go; this test focuses on the new
// task-14 components from the handler's perspective without
// re-running the full NATS subscribe pipeline.
func TestBootstrapJoinTokenE2E_ValidatorPlusIssuer(t *testing.T) {
	t.Parallel()
	p, _, att := newJTBootstrapProvider(t)
	base := &stubBaseIssuer{apiKey: "e2e-api-key"}

	validator, err := NewJoinTokenBootstrapValidator(att, p.TrustDomain())
	if err != nil {
		t.Fatalf("NewJoinTokenBootstrapValidator: %v", err)
	}
	issuer, err := NewSVIDBootstrapIssuer(p, base, 0)
	if err != nil {
		t.Fatalf("NewSVIDBootstrapIssuer: %v", err)
	}

	cleartext, _ := mintAgentToken(t, p, "agent-e2e")

	// Step 1: validate the token (BootstrapHandler.Validate path).
	if err := validator.Validate(context.Background(), "agent-e2e", []byte(cleartext)); err != nil {
		t.Fatalf("Validate: %v", err)
	}

	// Step 2: issue credentials (BootstrapHandler.Issue path).
	creds, err := issuer.Issue(context.Background(), "agent-e2e")
	if err != nil {
		t.Fatalf("Issue: %v", err)
	}
	if creds.APIKey != "e2e-api-key" {
		t.Errorf("APIKey = %q, want base", creds.APIKey)
	}
	if creds.CertChainPEM == "" || creds.PrivateKeyPEM == "" || creds.TrustBundlePEM == "" {
		t.Error("SVID fields not populated")
	}

	// Step 3: replay the same token → exhausted.
	err = validator.Validate(context.Background(), "agent-e2e", []byte(cleartext))
	if !errors.Is(err, identity.ErrJoinTokenExhausted) {
		t.Errorf("replay err = %v, want ErrJoinTokenExhausted", err)
	}
}

// ---- pem helpers ------------------------------------------------

func firstCertFromPEM(t *testing.T, s string) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode([]byte(s))
	if block == nil {
		t.Fatalf("pem.Decode returned nil for %q…", truncate(s, 60))
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate: %v", err)
	}
	return cert
}

func decodeAllPEM(t *testing.T, b []byte) []*pem.Block {
	t.Helper()
	var out []*pem.Block
	for {
		block, rest := pem.Decode(b)
		if block == nil {
			break
		}
		out = append(out, block)
		b = rest
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// guardrail: keep net.Listen referenced — used implicitly by
// tls.Listen but the linter sometimes doesn't see through.
var _ = net.Listen

// guardrail: keep sha256 referenced if a future test wants a
// known-bad hash. Currently unused but the validator's path
// computes it internally.
var _ = sha256.Sum256
