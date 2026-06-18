// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"strings"
	"testing"
	"time"
)

func chainToPEM(t *testing.T, chain []*x509.Certificate) string {
	t.Helper()
	var b strings.Builder
	for _, c := range chain {
		if err := pem.Encode(&b, &pem.Block{Type: "CERTIFICATE", Bytes: c.Raw}); err != nil {
			t.Fatalf("pem.Encode: %v", err)
		}
	}
	return b.String()
}

func issueAgentChain(t *testing.T) (chainPEM string, bundle *TrustBundle, expiresAt time.Time) {
	t.Helper()
	p := newStartedProvider(t)
	id, err := AgentID(DefaultTrustDomain, "agent-verify-1")
	if err != nil {
		t.Fatalf("AgentID: %v", err)
	}
	svid, err := p.IssueX509SVID(context.Background(), IssueX509SVIDRequest{ID: id, TTL: time.Hour})
	if err != nil {
		t.Fatalf("IssueX509SVID: %v", err)
	}
	b, err := p.GetTrustBundle(context.Background())
	if err != nil {
		t.Fatalf("GetTrustBundle: %v", err)
	}
	return chainToPEM(t, svid.Chain()), b, svid.ExpiresAt()
}

func TestVerifyAgentCert_Valid(t *testing.T) {
	chainPEM, bundle, _ := issueAgentChain(t)
	res, err := VerifyAgentCert(chainPEM, bundle, DefaultTrustDomain, time.Now())
	if err != nil {
		t.Fatalf("VerifyAgentCert: %v", err)
	}
	if !res.OK() {
		t.Fatalf("OK()=false, want true: %+v", res)
	}
	if !res.ChainValid || !res.SPIFFEMatch || res.Expired || res.NotYetValid {
		t.Errorf("unexpected result: %+v", res)
	}
	if !strings.HasPrefix(res.SPIFFEID, "spiffe://") {
		t.Errorf("SPIFFEID = %q", res.SPIFFEID)
	}
}

func TestVerifyAgentCert_Expired(t *testing.T) {
	chainPEM, bundle, expiresAt := issueAgentChain(t)
	res, err := VerifyAgentCert(chainPEM, bundle, DefaultTrustDomain, expiresAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("VerifyAgentCert: %v", err)
	}
	if !res.Expired {
		t.Errorf("Expired=false, want true")
	}
	// Still chains to the CA — expiry is reported separately, not as an
	// untrusted chain.
	if !res.ChainValid {
		t.Errorf("ChainValid=false; expiry should not mask a trusted chain")
	}
	if res.OK() {
		t.Errorf("OK()=true for an expired cert")
	}
}

func TestVerifyAgentCert_WrongTrustDomain(t *testing.T) {
	chainPEM, bundle, _ := issueAgentChain(t)
	res, err := VerifyAgentCert(chainPEM, bundle, "other.example", time.Now())
	if err != nil {
		t.Fatalf("VerifyAgentCert: %v", err)
	}
	if res.SPIFFEMatch {
		t.Errorf("SPIFFEMatch=true for a mismatched trust domain")
	}
}

func TestVerifyAgentCert_UntrustedChain(t *testing.T) {
	chainPEM, _, _ := issueAgentChain(t)
	empty, err := NewTrustBundle(DefaultTrustDomain)
	if err != nil {
		t.Fatalf("NewTrustBundle: %v", err)
	}
	res, err := VerifyAgentCert(chainPEM, empty, DefaultTrustDomain, time.Now())
	if err != nil {
		t.Fatalf("VerifyAgentCert: %v", err)
	}
	if res.ChainValid {
		t.Errorf("ChainValid=true against an empty trust bundle")
	}
}

func TestVerifyAgentCert_Errors(t *testing.T) {
	_, bundle, _ := issueAgentChain(t)
	if _, err := VerifyAgentCert("not a pem", bundle, DefaultTrustDomain, time.Now()); err == nil {
		t.Error("expected error for malformed PEM")
	}
	if _, err := VerifyAgentCert("", nil, DefaultTrustDomain, time.Now()); err == nil {
		t.Error("expected error for nil bundle")
	}
}
