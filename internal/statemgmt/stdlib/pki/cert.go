// SPDX-License-Identifier: Apache-2.0

package pki

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"math/big"
	"net"
	"os"
	"slices"
	"sort"
	"time"
)

const (
	pemTypePrivateKey  = "PRIVATE KEY"
	pemTypeCertificate = "CERTIFICATE"
	keyFileMode        = 0o600
	certFileMode       = 0o644
)

// --- key generation + (de)serialisation -------------------------------

// generateKey produces a fresh private key per the declaration's key
// spec. The returned value is a crypto.Signer (*rsa.PrivateKey,
// *ecdsa.PrivateKey, or ed25519.PrivateKey).
func generateKey(p *params) (crypto.Signer, error) {
	switch p.KeyType {
	case KeyRSA:
		return rsa.GenerateKey(rand.Reader, p.RSABits)
	case KeyECDSA:
		var c elliptic.Curve
		switch p.ECDSACurve {
		case CurveP256:
			c = elliptic.P256()
		case CurveP384:
			c = elliptic.P384()
		case CurveP521:
			c = elliptic.P521()
		default:
			return nil, fmt.Errorf("unknown ecdsa curve %q", p.ECDSACurve)
		}
		return ecdsa.GenerateKey(c, rand.Reader)
	case KeyEd25519:
		_, priv, err := ed25519.GenerateKey(rand.Reader)
		return priv, err
	default:
		return nil, fmt.Errorf("unknown key type %q", p.KeyType)
	}
}

// marshalKeyPEM PKCS#8-encodes key and wraps it in a PEM block.
// PKCS#8 carries the algorithm, so the same encoding works for RSA,
// ECDSA and Ed25519.
func marshalKeyPEM(key crypto.Signer) ([]byte, error) {
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("marshal private key: %w", err)
	}
	return pem.EncodeToMemory(&pem.Block{Type: pemTypePrivateKey, Bytes: der}), nil
}

func marshalCertPEM(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: pemTypeCertificate, Bytes: der})
}

// loadPrivateKey reads and parses a PEM private key file. PKCS#8 is
// tried first, then the legacy PKCS#1 ("RSA PRIVATE KEY") and SEC1
// ("EC PRIVATE KEY") forms so pre-existing keys are accepted.
func loadPrivateKey(path string) (crypto.Signer, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied key path from a validated state declaration
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("%s: no PEM block found", path)
	}
	if k, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		s, ok := k.(crypto.Signer)
		if !ok {
			return nil, fmt.Errorf("%s: unsupported private key type %T", path, k)
		}
		return s, nil
	}
	switch block.Type {
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	}
	return nil, fmt.Errorf("%s: unrecognised private key format", path)
}

// loadCertificate reads and parses the first CERTIFICATE block from a
// PEM file (the leaf, when a chain is present).
func loadCertificate(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path) //nolint:gosec // operator-supplied certificate path from a validated state declaration
	if err != nil {
		return nil, err
	}
	for {
		var block *pem.Block
		block, data = pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("%s: no CERTIFICATE PEM block found", path)
		}
		if block.Type == pemTypeCertificate {
			return x509.ParseCertificate(block.Bytes)
		}
	}
}

// --- comparisons ------------------------------------------------------

func publicKeysEqual(a, b crypto.PublicKey) bool {
	type equaler interface{ Equal(crypto.PublicKey) bool }
	ea, ok := a.(equaler)
	if !ok {
		return false
	}
	return ea.Equal(b)
}

func splitSANs(sans []string) (dns, ips []string) {
	for _, s := range sans {
		if ip := net.ParseIP(s); ip != nil {
			ips = append(ips, ip.String())
		} else {
			dns = append(dns, s)
		}
	}
	return dns, ips
}

func sanSetEqual(cert *x509.Certificate, declared []string) bool {
	wantDNS, wantIP := splitSANs(declared)
	gotDNS := slices.Clone(cert.DNSNames)
	gotIP := make([]string, 0, len(cert.IPAddresses))
	for _, ip := range cert.IPAddresses {
		gotIP = append(gotIP, ip.String())
	}
	sort.Strings(wantDNS)
	sort.Strings(gotDNS)
	sort.Strings(wantIP)
	sort.Strings(gotIP)
	return slices.Equal(wantDNS, gotDNS) && slices.Equal(wantIP, gotIP)
}

// --- convergence check ------------------------------------------------

// checkState inspects the on-disk key + cert against p. It never
// mutates anything. needKey implies needCert (a new key invalidates
// the old cert).
func checkState(p *params, now time.Time) (converged, needKey, needCert bool, diff string, err error) {
	priv, kerr := loadPrivateKey(p.KeyPath)
	if kerr != nil {
		if errors.Is(kerr, fs.ErrNotExist) {
			return false, true, true, "private key missing → generate key + certificate", nil
		}
		return false, true, true, fmt.Sprintf("private key invalid (%v) → regenerate key + certificate", kerr), nil
	}

	cert, cerr := loadCertificate(p.CertPath)
	if cerr != nil {
		if errors.Is(cerr, fs.ErrNotExist) {
			return false, false, true, "certificate missing → generate certificate", nil
		}
		return false, false, true, fmt.Sprintf("certificate invalid (%v) → regenerate certificate", cerr), nil
	}

	if !publicKeysEqual(priv.Public(), cert.PublicKey) {
		return false, false, true, "certificate does not match the private key → regenerate certificate", nil
	}
	if p.CommonName != "" && cert.Subject.CommonName != p.CommonName {
		return false, false, true, fmt.Sprintf("certificate CN %q → %q → regenerate certificate", cert.Subject.CommonName, p.CommonName), nil
	}
	if !sanSetEqual(cert, p.SANs) {
		return false, false, true, "certificate subject-alt-names differ from the declaration → regenerate certificate", nil
	}
	if cert.IsCA != p.IsCA {
		return false, false, true, fmt.Sprintf("certificate IsCA=%v, want %v → regenerate certificate", cert.IsCA, p.IsCA), nil
	}
	if now.Before(cert.NotBefore) || now.After(cert.NotAfter) {
		return false, false, true, "certificate is not currently valid (NotBefore/NotAfter) → regenerate certificate", nil
	}
	if p.RenewDays > 0 && time.Until(cert.NotAfter) < time.Duration(p.RenewDays)*24*time.Hour {
		left := int(time.Until(cert.NotAfter).Hours() / 24)
		return false, false, true, fmt.Sprintf("certificate expires in %dd (< renew_days=%d) → regenerate certificate", left, p.RenewDays), nil
	}

	if p.CACertPath != "" {
		caCert, caErr := loadCertificate(p.CACertPath)
		if caErr != nil {
			return false, false, false, "", fmt.Errorf("read ca_cert %s: %w", p.CACertPath, caErr)
		}
		if caCert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) != nil ||
			!bytes.Equal(cert.RawIssuer, caCert.RawSubject) {
			return false, false, true, "certificate is not signed by the declared ca_cert → regenerate certificate", nil
		}
	} else {
		if cert.CheckSignature(cert.SignatureAlgorithm, cert.RawTBSCertificate, cert.Signature) != nil ||
			!bytes.Equal(cert.RawIssuer, cert.RawSubject) {
			return false, false, true, "certificate is not self-signed (a CA was used) → regenerate certificate", nil
		}
	}
	return true, false, false, "", nil
}

// --- certificate creation ---------------------------------------------

func buildTemplate(p *params, now time.Time) (*x509.Certificate, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("serial number: %w", err)
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: p.CommonName},
		NotBefore:             now.Add(-1 * time.Minute), // small backdate for clock skew
		NotAfter:              now.AddDate(0, 0, p.Days),
		BasicConstraintsValid: true,
	}
	if p.Organization != "" {
		tmpl.Subject.Organization = []string{p.Organization}
	}
	for _, san := range p.SANs {
		if ip := net.ParseIP(san); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, san)
		}
	}
	if p.IsCA {
		tmpl.IsCA = true
		tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature
	} else {
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	}
	return tmpl, nil
}

// signCert produces the DER certificate for tmpl. With caCert==nil it
// is self-signed (signed by leafKey); otherwise it is signed by
// caKey with caCert as the issuer.
func signCert(tmpl *x509.Certificate, leafKey crypto.Signer, caCert *x509.Certificate, caKey crypto.Signer) ([]byte, error) {
	parent := tmpl
	signKey := leafKey
	if caCert != nil {
		parent = caCert
		signKey = caKey
	}
	return x509.CreateCertificate(rand.Reader, tmpl, parent, leafKey.Public(), signKey)
}

// --- atomic write -----------------------------------------------------

// writeFileAtomic writes data to path via write-temp-then-rename in
// the same directory. An existing file's permission bits are
// preserved; a new file gets defaultMode.
func writeFileAtomic(path string, data []byte, defaultMode os.FileMode) error {
	mode := defaultMode
	if fi, err := os.Stat(path); err == nil {
		mode = fi.Mode().Perm()
	}
	tmp := path + ".keystone.tmp"
	if err := os.WriteFile(tmp, data, mode); err != nil { //nolint:gosec // mode mirrors the existing file or is the documented default (0600 key / 0644 cert)
		return fmt.Errorf("write tmp: %w", err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("chmod tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s → %s: %w", tmp, path, err)
	}
	return nil
}
