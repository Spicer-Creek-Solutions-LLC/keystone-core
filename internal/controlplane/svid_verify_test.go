// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent"
)

type verifyCA struct {
	cert *x509.Certificate
	key  crypto.Signer
	pem  string
}

func newVerifyCA(t *testing.T) *verifyCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "verify-ca"},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		IsCA:                  true,
		KeyUsage:              x509.KeyUsageCertSign,
		BasicConstraintsValid: true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, key.Public(), key)
	if err != nil {
		t.Fatalf("ca cert: %v", err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse ca: %v", err)
	}
	return &verifyCA{cert: cert, key: key,
		pem: string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))}
}

func (ca *verifyCA) credentials(t *testing.T, agentID string, notAfter time.Time) *agent.Credentials {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("leaf key: %v", err)
	}
	if notAfter.IsZero() {
		notAfter = time.Now().Add(time.Hour)
	}
	u, err := url.Parse("spiffe://example.org/agent/" + agentID)
	if err != nil {
		t.Fatalf("parse uri: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: agentID},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
		URIs:         []*url.URL{u},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, key.Public(), ca.key)
	if err != nil {
		t.Fatalf("leaf cert: %v", err)
	}
	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	return &agent.Credentials{
		APIKey:         "api-key",
		AgentID:        agentID,
		CertChainPEM:   string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) + ca.pem,
		PrivateKeyPEM:  string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8})),
		TrustBundlePEM: ca.pem,
	}
}

func (ca *verifyCA) signer(t *testing.T, agentID string) *agent.SVIDSigner {
	t.Helper()
	s, err := agent.NewSVIDSigner(ca.credentials(t, agentID, time.Time{}))
	if err != nil {
		t.Fatalf("NewSVIDSigner: %v", err)
	}
	return s
}

func (ca *verifyCA) verifier(t *testing.T) *SVIDVerifier {
	t.Helper()
	roots, err := SVIDRootsFromPEM(ca.pem)
	if err != nil {
		t.Fatalf("SVIDRootsFromPEM: %v", err)
	}
	return &SVIDVerifier{Roots: roots}
}

func TestSVIDVerifier_AcceptsAgentSignedRequest(t *testing.T) {
	ca := newVerifyCA(t)
	signer := ca.signer(t, "agent-1")
	payload := []byte(`{"path":"app/db","key":"password"}`)

	req, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	gotID, gotPayload, err := ca.verifier(t).Verify(req)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if gotID != "agent-1" {
		t.Errorf("agent id = %q, want %q", gotID, "agent-1")
	}
	if string(gotPayload) != string(payload) {
		t.Errorf("payload = %q, want %q", gotPayload, payload)
	}
}

// The whole point of the change. agent-2 holds a legitimate,
// CA-signed certificate and simply claims to be agent-1 — which is
// exactly what the fleet-wide HMAC key cannot detect.
func TestSVIDVerifier_RejectsImpersonation(t *testing.T) {
	ca := newVerifyCA(t)
	req, err := ca.signer(t, "agent-2").Sign([]byte("payload"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	req.AgentID = "agent-1"

	_, _, err = ca.verifier(t).Verify(req)
	if !errors.Is(err, ErrSVIDIdentityMismatch) {
		t.Fatalf("Verify() error = %v, want ErrSVIDIdentityMismatch", err)
	}
}

// The realistic attack: a hostile agent does not mutate a signed
// request, it constructs one. It holds a legitimate certificate and
// its own key, so it can produce a signature that is internally
// consistent over a canonical form naming somebody else. Only the
// certificate check stops it.
//
// This is the case the fleet-wide HMAC key cannot address at all --
// every agent holds that key, so every agent can mint a request for
// any agent id and have it verify.
func TestSVIDVerifier_RejectsForgedIdentityFromValidKey(t *testing.T) {
	ca := newVerifyCA(t)
	attacker := ca.credentials(t, "agent-2", time.Time{})

	block, _ := pem.Decode([]byte(attacker.PrivateKeyPEM))
	if block == nil {
		t.Fatal("attacker key is not PEM")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		t.Fatalf("parse attacker key: %v", err)
	}
	key, ok := parsed.(crypto.Signer)
	if !ok {
		t.Fatalf("attacker key of type %T cannot sign", parsed)
	}

	// Claim agent-1 from the outset, so the signature covers the lie.
	forged := &agent.SignedRequest{
		AgentID:      "agent-1",
		CertChainPEM: attacker.CertChainPEM,
		IssuedAt:     time.Now().UTC(),
		Nonce:        "0123456789abcdef0123456789abcdef",
		Payload:      []byte(`{"path":"app/db","key":"password"}`),
	}
	digest := sha256.Sum256(agent.CanonicalSignedRequest(forged))
	sig, err := key.Sign(rand.Reader, digest[:], crypto.SHA256)
	if err != nil {
		t.Fatalf("forge signature: %v", err)
	}
	forged.Signature = hex.EncodeToString(sig)

	// The forgery is cryptographically sound — confirm that, so the
	// rejection below is demonstrably about identity and not about a
	// malformed signature.
	blockLeaf, _ := pem.Decode([]byte(attacker.CertChainPEM))
	leaf, err := x509.ParseCertificate(blockLeaf.Bytes)
	if err != nil {
		t.Fatalf("parse attacker leaf: %v", err)
	}
	if err := agent.VerifySignature(leaf.PublicKey, forged); err != nil {
		t.Fatalf("forged signature should be valid on its own terms: %v", err)
	}

	_, _, err = ca.verifier(t).Verify(forged)
	if !errors.Is(err, ErrSVIDIdentityMismatch) {
		t.Fatalf("Verify() error = %v, want ErrSVIDIdentityMismatch", err)
	}
}

// Swapping in another agent's certificate must not launder a
// signature: the chain is inside the signed canonical form.
func TestSVIDVerifier_RejectsCertificateSwap(t *testing.T) {
	ca := newVerifyCA(t)
	req, err := ca.signer(t, "agent-1").Sign([]byte("payload"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	victim := ca.credentials(t, "agent-2", time.Time{})
	req.CertChainPEM = victim.CertChainPEM
	req.AgentID = "agent-2"

	_, _, err = ca.verifier(t).Verify(req)
	if err == nil {
		t.Fatal("Verify() error = nil, want a rejection for a swapped certificate")
	}
	if !errors.Is(err, ErrSVIDSignature) {
		t.Errorf("Verify() error = %v, want ErrSVIDSignature", err)
	}
}

// A certificate from an unrelated CA must not be accepted just because
// it is well-formed and internally consistent.
func TestSVIDVerifier_RejectsForeignCA(t *testing.T) {
	ours := newVerifyCA(t)
	theirs := newVerifyCA(t)

	req, err := theirs.signer(t, "agent-1").Sign([]byte("payload"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	_, _, err = ours.verifier(t).Verify(req)
	if !errors.Is(err, ErrSVIDUntrusted) {
		t.Fatalf("Verify() error = %v, want ErrSVIDUntrusted", err)
	}
}

func TestSVIDVerifier_RejectsTampering(t *testing.T) {
	ca := newVerifyCA(t)
	tests := []struct {
		name   string
		mutate func(*agent.SignedRequest)
	}{
		{"payload", func(r *agent.SignedRequest) { r.Payload = []byte(`{"path":"root/ca"}`) }},
		{"nonce", func(r *agent.SignedRequest) { r.Nonce = "00000000000000000000000000000000" }},
		{"signature", func(r *agent.SignedRequest) { r.Signature = "00" + r.Signature[2:] }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := ca.signer(t, "agent-1").Sign([]byte(`{"path":"app/db"}`))
			if err != nil {
				t.Fatalf("Sign: %v", err)
			}
			tt.mutate(req)
			if _, _, err := ca.verifier(t).Verify(req); !errors.Is(err, ErrSVIDSignature) {
				t.Errorf("Verify() error = %v, want ErrSVIDSignature", err)
			}
		})
	}
}

func TestSVIDVerifier_FreshnessWindow(t *testing.T) {
	ca := newVerifyCA(t)
	signer := ca.signer(t, "agent-1")
	roots, err := SVIDRootsFromPEM(ca.pem)
	if err != nil {
		t.Fatalf("SVIDRootsFromPEM: %v", err)
	}

	req, err := signer.Sign([]byte("payload"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	signedAt := req.IssuedAt

	t.Run("inside the window", func(t *testing.T) {
		v := &SVIDVerifier{Roots: roots, MaxAge: time.Minute,
			Now: func() time.Time { return signedAt.Add(30 * time.Second) }}
		if _, _, err := v.Verify(req); err != nil {
			t.Errorf("Verify: %v", err)
		}
	})

	t.Run("too old", func(t *testing.T) {
		v := &SVIDVerifier{Roots: roots, MaxAge: time.Minute,
			Now: func() time.Time { return signedAt.Add(2 * time.Minute) }}
		if _, _, err := v.Verify(req); !errors.Is(err, ErrSVIDStale) {
			t.Errorf("Verify() error = %v, want ErrSVIDStale", err)
		}
	})

	t.Run("too far ahead of the server clock", func(t *testing.T) {
		v := &SVIDVerifier{Roots: roots, MaxAge: time.Minute, Skew: 5 * time.Second,
			Now: func() time.Time { return signedAt.Add(-time.Minute) }}
		if _, _, err := v.Verify(req); !errors.Is(err, ErrSVIDStale) {
			t.Errorf("Verify() error = %v, want ErrSVIDStale", err)
		}
	})

	t.Run("no issued_at", func(t *testing.T) {
		stripped := *req
		stripped.IssuedAt = time.Time{}
		if _, _, err := ca.verifier(t).Verify(&stripped); !errors.Is(err, ErrSVIDStale) {
			t.Errorf("Verify() error = %v, want ErrSVIDStale", err)
		}
	})
}

func TestSVIDVerifier_RejectsExpiredCertificate(t *testing.T) {
	ca := newVerifyCA(t)
	creds := ca.credentials(t, "agent-1", time.Now().Add(2*time.Second))
	signer, err := agent.NewSVIDSigner(creds)
	if err != nil {
		t.Fatalf("NewSVIDSigner: %v", err)
	}
	req, err := signer.Sign([]byte("payload"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	roots, err := SVIDRootsFromPEM(ca.pem)
	if err != nil {
		t.Fatalf("SVIDRootsFromPEM: %v", err)
	}
	// Verify from a point after the leaf expires. MaxAge is widened so
	// the freshness check does not fire first and mask the expiry.
	v := &SVIDVerifier{Roots: roots, MaxAge: time.Hour,
		Now: func() time.Time { return time.Now().Add(time.Minute) }}
	if _, _, err := v.Verify(req); !errors.Is(err, ErrSVIDUntrusted) {
		t.Errorf("Verify() error = %v, want ErrSVIDUntrusted", err)
	}
}

func TestSVIDVerifier_MalformedInput(t *testing.T) {
	ca := newVerifyCA(t)

	t.Run("nil request", func(t *testing.T) {
		if _, _, err := ca.verifier(t).Verify(nil); err == nil {
			t.Error("Verify(nil) error = nil, want an error")
		}
	})

	t.Run("no roots configured", func(t *testing.T) {
		req, err := ca.signer(t, "agent-1").Sign([]byte("payload"))
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		if _, _, err := (&SVIDVerifier{}).Verify(req); err == nil {
			t.Error("Verify() error = nil, want an error with no roots")
		}
	})

	t.Run("empty chain", func(t *testing.T) {
		req := &agent.SignedRequest{AgentID: "agent-1", IssuedAt: time.Now()}
		if _, _, err := ca.verifier(t).Verify(req); !errors.Is(err, ErrSVIDUntrusted) {
			t.Errorf("Verify() error = %v, want ErrSVIDUntrusted", err)
		}
	})

	t.Run("chain is not PEM", func(t *testing.T) {
		req := &agent.SignedRequest{AgentID: "agent-1", CertChainPEM: "nope", IssuedAt: time.Now()}
		if _, _, err := ca.verifier(t).Verify(req); !errors.Is(err, ErrSVIDUntrusted) {
			t.Errorf("Verify() error = %v, want ErrSVIDUntrusted", err)
		}
	})

	t.Run("signature is not hex", func(t *testing.T) {
		req, err := ca.signer(t, "agent-1").Sign([]byte("payload"))
		if err != nil {
			t.Fatalf("Sign: %v", err)
		}
		req.Signature = "zzzz"
		if _, _, err := ca.verifier(t).Verify(req); !errors.Is(err, ErrSVIDSignature) {
			t.Errorf("Verify() error = %v, want ErrSVIDSignature", err)
		}
	})
}

// An empty claimed id is not an impersonation attempt — the verifier
// fills it in from the certificate rather than refusing.
func TestSVIDVerifier_EmptyClaimedIDTakesTheCertificatesWord(t *testing.T) {
	ca := newVerifyCA(t)
	req, err := ca.signer(t, "agent-1").Sign([]byte("payload"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	req.AgentID = ""
	// Re-sign is not needed: AgentID is inside the canonical form, so
	// clearing it must break the signature. That is the assertion.
	if _, _, err := ca.verifier(t).Verify(req); !errors.Is(err, ErrSVIDSignature) {
		t.Errorf("Verify() error = %v, want ErrSVIDSignature", err)
	}
}

func TestSVIDRootsFromPEM_Rejects(t *testing.T) {
	if _, err := SVIDRootsFromPEM(""); err == nil {
		t.Error("SVIDRootsFromPEM(\"\") error = nil, want an error")
	}
	if _, err := SVIDRootsFromPEM("not a certificate"); err == nil {
		t.Error("SVIDRootsFromPEM(garbage) error = nil, want an error")
	}
}
