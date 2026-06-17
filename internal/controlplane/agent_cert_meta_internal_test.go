// SPDX-License-Identifier: Apache-2.0

package controlplane

import (
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
)

// testLeafPEM generates a self-signed leaf cert with the given SPIFFE URI
// SAN and NotAfter, returning the PEM and the DER for assertions.
func testLeafPEM(t *testing.T, spiffeID string, notAfter time.Time) (pemStr string, der []byte) {
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

func TestAgentCertMeta(t *testing.T) {
	notAfter := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	pemStr, der := testLeafPEM(t, "spiffe://example.org/agent/foo", notAfter)

	fp, na, spiffe, err := agentCertMeta(pemStr)
	if err != nil {
		t.Fatalf("agentCertMeta: %v", err)
	}
	sum := sha256.Sum256(der)
	if want := hex.EncodeToString(sum[:]); fp != want {
		t.Errorf("fingerprint = %q, want %q", fp, want)
	}
	if !na.Equal(notAfter) {
		t.Errorf("notAfter = %v, want %v", na, notAfter)
	}
	if spiffe != "spiffe://example.org/agent/foo" {
		t.Errorf("spiffeID = %q", spiffe)
	}
}

func TestAgentCertMeta_NoSPIFFEURI(t *testing.T) {
	// A cert with no spiffe:// URI yields an empty SPIFFE ID, no error.
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "no-uri"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, _ := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	pemStr := string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
	_, _, spiffe, err := agentCertMeta(pemStr)
	if err != nil {
		t.Fatalf("agentCertMeta: %v", err)
	}
	if spiffe != "" {
		t.Errorf("spiffeID = %q, want empty", spiffe)
	}
}

func TestAgentCertMeta_InvalidPEM(t *testing.T) {
	if _, _, _, err := agentCertMeta("not a pem block"); err == nil {
		t.Fatal("expected error for non-PEM input")
	}
}
