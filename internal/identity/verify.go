// SPDX-License-Identifier: Apache-2.0

package identity

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"time"
)

// VerifyResult reports the outcome of [VerifyAgentCert].
type VerifyResult struct {
	// SPIFFEID is the leaf's spiffe:// URI SAN, or "" if it has none.
	SPIFFEID string
	// ChainValid reports whether the leaf chains to a trust-bundle
	// authority, independent of expiry (an expired-but-trusted cert is
	// ChainValid=true, Expired=true).
	ChainValid bool
	// Expired / NotYetValid place `now` relative to the leaf's validity
	// window.
	Expired     bool
	NotYetValid bool
	// SPIFFEMatch reports whether the leaf carries an `agent/*` SPIFFE
	// ID in the expected trust domain.
	SPIFFEMatch bool
	// ExpiresAt is the leaf's NotAfter.
	ExpiresAt time.Time
}

// OK reports whether the cert is currently valid for an agent: it chains
// to the bundle, is within its validity window, and carries a matching
// agent SPIFFE identity.
func (r VerifyResult) OK() bool {
	return r.ChainValid && !r.Expired && !r.NotYetValid && r.SPIFFEMatch
}

// VerifyAgentCert verifies a stored agent certificate chain (PEM-encoded,
// leaf first then any intermediates) against the trust bundle and
// expected trust domain. It reports chain validity, expiry, and whether
// the leaf carries an `agent/*` SPIFFE ID in trustDomain. `now` is the
// reference time (injectable for tests). A malformed PEM / empty chain is
// the only error; verification outcomes are reported in the result.
func VerifyAgentCert(chainPEM string, bundle *TrustBundle, trustDomain string, now time.Time) (VerifyResult, error) {
	if bundle == nil {
		return VerifyResult{}, fmt.Errorf("identity: VerifyAgentCert: nil trust bundle")
	}
	leaf, intermediates, err := parseChainPEM(chainPEM)
	if err != nil {
		return VerifyResult{}, err
	}

	roots := x509.NewCertPool()
	for _, a := range bundle.X509Authorities() {
		roots.AddCert(a)
	}

	res := VerifyResult{
		ExpiresAt:   leaf.NotAfter,
		Expired:     now.After(leaf.NotAfter),
		NotYetValid: now.Before(leaf.NotBefore),
	}

	verify := func(at time.Time) error {
		_, err := leaf.Verify(x509.VerifyOptions{
			Roots:         roots,
			Intermediates: intermediates,
			CurrentTime:   at,
			KeyUsages:     []x509.ExtKeyUsage{x509.ExtKeyUsageAny},
		})
		return err
	}
	if verify(now) == nil {
		res.ChainValid = true
	} else if res.Expired || res.NotYetValid {
		// Isolate chain trust from the time problem: re-check at a point
		// inside the leaf's own validity window so an expired-but-trusted
		// cert still reports ChainValid=true.
		mid := leaf.NotBefore.Add(leaf.NotAfter.Sub(leaf.NotBefore) / 2)
		res.ChainValid = verify(mid) == nil
	}

	for _, u := range leaf.URIs {
		if u == nil || u.Scheme != "spiffe" {
			continue
		}
		res.SPIFFEID = u.String()
		if id, perr := ParseSPIFFEID(u.String()); perr == nil {
			segs := id.Segments()
			res.SPIFFEMatch = id.TrustDomain() == trustDomain &&
				len(segs) > 0 && segs[0] == pathPrefixAgent
		}
		break
	}
	return res, nil
}

// parseChainPEM decodes a PEM chain into the leaf (first CERTIFICATE
// block) and a pool of any remaining intermediates.
func parseChainPEM(chainPEM string) (leaf *x509.Certificate, intermediates *x509.CertPool, err error) {
	rest := []byte(chainPEM)
	intermediates = x509.NewCertPool()
	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, perr := x509.ParseCertificate(block.Bytes)
		if perr != nil {
			return nil, nil, fmt.Errorf("identity: VerifyAgentCert: parse cert: %w", perr)
		}
		if leaf == nil {
			leaf = cert
			continue
		}
		intermediates.AddCert(cert)
	}
	if leaf == nil {
		return nil, nil, fmt.Errorf("identity: VerifyAgentCert: no certificate in chain")
	}
	return leaf, intermediates, nil
}
