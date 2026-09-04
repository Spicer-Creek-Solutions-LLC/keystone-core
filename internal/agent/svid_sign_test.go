// SPDX-License-Identifier: Apache-2.0

package agent

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net/url"
	"testing"
	"time"
)

// testCA is a throwaway certificate authority for signing test leaves.
type testCA struct {
	cert *x509.Certificate
	key  crypto.Signer
	pem  string
}

func newTestCA(t *testing.T) *testCA {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("ca key: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
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
	return &testCA{
		cert: cert,
		key:  key,
		pem:  string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
	}
}

// issue mints a leaf for spiffeURI and returns credentials carrying it.
// notAfter zero means one hour out.
func (ca *testCA) issue(t *testing.T, spiffeURI string, key crypto.Signer, notAfter time.Time) *Credentials {
	t.Helper()
	if key == nil {
		var err error
		key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		if err != nil {
			t.Fatalf("leaf key: %v", err)
		}
	}
	if notAfter.IsZero() {
		notAfter = time.Now().Add(time.Hour)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "leaf"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature,
	}
	if spiffeURI != "" {
		u, err := url.Parse(spiffeURI)
		if err != nil {
			t.Fatalf("parse uri: %v", err)
		}
		tmpl.URIs = []*url.URL{u}
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, ca.cert, key.Public(), ca.key)
	if err != nil {
		t.Fatalf("leaf cert: %v", err)
	}
	chain := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})) + ca.pem

	pkcs8, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	keyPEM := string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: pkcs8}))

	return &Credentials{
		APIKey:         "api-key",
		AgentID:        "agent-1",
		CertChainPEM:   chain,
		PrivateKeyPEM:  keyPEM,
		TrustBundlePEM: ca.pem,
	}
}

func TestSVIDSigner_SignAndVerify(t *testing.T) {
	ca := newTestCA(t)
	creds := ca.issue(t, "spiffe://example.org/agent/agent-1", nil, time.Time{})

	signer, err := NewSVIDSigner(creds)
	if err != nil {
		t.Fatalf("NewSVIDSigner: %v", err)
	}
	if signer.AgentID() != "agent-1" {
		t.Errorf("AgentID() = %q, want %q", signer.AgentID(), "agent-1")
	}

	payload := []byte(`{"path":"app/db"}`)
	req, err := signer.Sign(payload)
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if req.Signature == "" {
		t.Fatal("Sign produced no signature")
	}
	if string(req.Payload) != string(payload) {
		t.Error("Sign altered the payload")
	}

	block, _ := pem.Decode([]byte(req.CertChainPEM))
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	if err := VerifySignature(leaf.PublicKey, req); err != nil {
		t.Errorf("VerifySignature: %v", err)
	}
}

// Two signatures over the same payload must differ, or a captured
// signature identifies the request that produced it.
func TestSVIDSigner_NonceVariesPerSignature(t *testing.T) {
	ca := newTestCA(t)
	signer, err := NewSVIDSigner(ca.issue(t, "spiffe://example.org/agent/agent-1", nil, time.Time{}))
	if err != nil {
		t.Fatalf("NewSVIDSigner: %v", err)
	}
	first, err := signer.Sign([]byte("same"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	second, err := signer.Sign([]byte("same"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if first.Nonce == second.Nonce {
		t.Error("two signatures reused the same nonce")
	}
	if first.Signature == second.Signature {
		t.Error("two signatures over the same payload are identical")
	}
}

func TestSVIDSigner_RSAKey(t *testing.T) {
	ca := newTestCA(t)
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	signer, err := NewSVIDSigner(ca.issue(t, "spiffe://example.org/agent/agent-1", key, time.Time{}))
	if err != nil {
		t.Fatalf("NewSVIDSigner: %v", err)
	}
	req, err := signer.Sign([]byte("payload"))
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if err := VerifySignature(&key.PublicKey, req); err != nil {
		t.Errorf("VerifySignature with an RSA key: %v", err)
	}
}

func TestNewSVIDSigner_Errors(t *testing.T) {
	ca := newTestCA(t)

	t.Run("api-key-only credential", func(t *testing.T) {
		_, err := NewSVIDSigner(&Credentials{APIKey: "k"})
		if !errors.Is(err, ErrNoSVID) {
			t.Errorf("error = %v, want ErrNoSVID", err)
		}
	})

	t.Run("nil credential", func(t *testing.T) {
		if _, err := NewSVIDSigner(nil); !errors.Is(err, ErrNoSVID) {
			t.Errorf("error = %v, want ErrNoSVID", err)
		}
	})

	t.Run("chain is not PEM", func(t *testing.T) {
		c := ca.issue(t, "spiffe://example.org/agent/agent-1", nil, time.Time{})
		c.CertChainPEM = "not pem"
		if _, err := NewSVIDSigner(c); err == nil {
			t.Error("error = nil, want a PEM error")
		}
	})

	t.Run("key is not PEM", func(t *testing.T) {
		c := ca.issue(t, "spiffe://example.org/agent/agent-1", nil, time.Time{})
		c.PrivateKeyPEM = "not pem"
		if _, err := NewSVIDSigner(c); err == nil {
			t.Error("error = nil, want a PEM error")
		}
	})

	// A key that does not match the certificate would otherwise fail
	// on every request with an error pointing at the signature.
	t.Run("key does not match the certificate", func(t *testing.T) {
		mine := ca.issue(t, "spiffe://example.org/agent/agent-1", nil, time.Time{})
		other := ca.issue(t, "spiffe://example.org/agent/agent-2", nil, time.Time{})
		mine.PrivateKeyPEM = other.PrivateKeyPEM
		_, err := NewSVIDSigner(mine)
		if err == nil {
			t.Fatal("error = nil, want a key/certificate mismatch")
		}
	})

	t.Run("leaf carries no URI SAN", func(t *testing.T) {
		if _, err := NewSVIDSigner(ca.issue(t, "", nil, time.Time{})); err == nil {
			t.Error("error = nil, want a missing-SAN error")
		}
	})

	t.Run("leaf is not an agent identity", func(t *testing.T) {
		c := ca.issue(t, "spiffe://example.org/server/control-plane", nil, time.Time{})
		if _, err := NewSVIDSigner(c); err == nil {
			t.Error("error = nil, want a non-agent-identity error")
		}
	})
}

func TestAgentIDFromCert(t *testing.T) {
	ca := newTestCA(t)
	leafOf := func(t *testing.T, uri string) *x509.Certificate {
		t.Helper()
		block, _ := pem.Decode([]byte(ca.issue(t, uri, nil, time.Time{}).CertChainPEM))
		leaf, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		return leaf
	}

	t.Run("agent identity", func(t *testing.T) {
		got, err := AgentIDFromCert(leafOf(t, "spiffe://example.org/agent/web-7"))
		if err != nil {
			t.Fatalf("AgentIDFromCert: %v", err)
		}
		if got != "web-7" {
			t.Errorf("AgentIDFromCert() = %q, want %q", got, "web-7")
		}
	})

	t.Run("nil leaf", func(t *testing.T) {
		if _, err := AgentIDFromCert(nil); err == nil {
			t.Error("error = nil, want an error")
		}
	})

	t.Run("non-spiffe uri", func(t *testing.T) {
		if _, err := AgentIDFromCert(leafOf(t, "https://example.org/agent/web-7")); err == nil {
			t.Error("error = nil, want a non-SPIFFE error")
		}
	})

	t.Run("empty agent id", func(t *testing.T) {
		if _, err := AgentIDFromCert(leafOf(t, "spiffe://example.org/agent/")); err == nil {
			t.Error("error = nil, want an empty-id error")
		}
	})
}
