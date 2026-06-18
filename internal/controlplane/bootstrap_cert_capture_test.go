// SPDX-License-Identifier: Apache-2.0

package controlplane_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"net/url"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/controlplane"
	"go.keystone-core.io/keystone-core/internal/state"
)

func genLeafPEM(t *testing.T, spiffeID string, notAfter time.Time) (pemStr string, der []byte) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	uri, err := url.Parse(spiffeID)
	if err != nil {
		t.Fatalf("parse spiffe id: %v", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "agent"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     notAfter,
		URIs:         []*url.URL{uri},
	}
	der, err = x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("CreateCertificate: %v", err)
	}
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})), der
}

// TestBootstrapHandler_CapturesCertMetadata verifies that the cert chain
// the issuer returns is persisted on the AgentRecord, with the derived
// fingerprint/expiry/SPIFFE-ID, at registration time.
func TestBootstrapHandler_CapturesCertMetadata(t *testing.T) {
	notAfter := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	certPEM, der := genLeafPEM(t, "spiffe://example.org/agent/agent-1", notAfter)

	var store state.Store
	_, sub, _, iss, val := newBootstrapFixture(t, func(c *controlplane.BootstrapHandlerConfig) {
		store = newTestStore(t)
		c.Store = store
	})
	iss.certChainPEM = certPEM
	val.seed("agent-1", []byte("secret-bytes"))

	env := makeRegisterEnvelope(t, "agent-1", []byte("secret-bytes"))
	if err := sub.deliver(t, "kscore.default.bootstrap.agent-1.register", env); err != nil {
		t.Fatalf("deliver: %v", err)
	}

	rec, err := store.GetAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if rec.CertChainPEM != certPEM {
		t.Errorf("CertChainPEM not stored")
	}
	sum := sha256.Sum256(der)
	if want := hex.EncodeToString(sum[:]); rec.CertFingerprint != want {
		t.Errorf("CertFingerprint = %q, want %q", rec.CertFingerprint, want)
	}
	if !rec.CertNotAfter.Equal(notAfter) {
		t.Errorf("CertNotAfter = %v, want %v", rec.CertNotAfter, notAfter)
	}
	if rec.SPIFFEID != "spiffe://example.org/agent/agent-1" {
		t.Errorf("SPIFFEID = %q", rec.SPIFFEID)
	}
}

// TestBootstrapHandler_NoCertChainLeavesMetadataEmpty confirms an
// API-key-only issuance (no cert chain) leaves the cert fields empty.
func TestBootstrapHandler_NoCertChainLeavesMetadataEmpty(t *testing.T) {
	var store state.Store
	_, sub, _, _, val := newBootstrapFixture(t, func(c *controlplane.BootstrapHandlerConfig) {
		store = newTestStore(t)
		c.Store = store
	})
	val.seed("agent-2", []byte("secret-bytes"))

	env := makeRegisterEnvelope(t, "agent-2", []byte("secret-bytes"))
	if err := sub.deliver(t, "kscore.default.bootstrap.agent-2.register", env); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	rec, err := store.GetAgent(context.Background(), "agent-2")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if rec.CertChainPEM != "" || rec.CertFingerprint != "" || rec.SPIFFEID != "" || !rec.CertNotAfter.IsZero() {
		t.Errorf("expected empty cert metadata, got %+v", rec)
	}
}
