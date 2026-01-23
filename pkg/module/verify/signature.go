package verify

import (
	"context"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"strings"

	"github.com/shawnbutts/keystone-core/pkg/signing"
)

// DefaultSignatureVerifier implements SignatureVerifier using the shared signing package
type DefaultSignatureVerifier struct {
	format SignatureFormat
}

// NewSignatureVerifier creates a new signature verifier
func NewSignatureVerifier(format SignatureFormat) *DefaultSignatureVerifier {
	return &DefaultSignatureVerifier{
		format: format,
	}
}

// VerifySignature verifies a signature against a module
func (v *DefaultSignatureVerifier) VerifySignature(modulePath, signaturePath string, publicKey []byte) (bool, error) {
	verifier, err := signing.NewKeyVerifier(&signing.KeyVerifierConfig{
		PublicKeyPEM:  publicKey,
		HashAlgorithm: signing.HashSHA256,
	})
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrInvalidPublicKey, err)
	}

	valid, err := verifier.VerifyFile(context.Background(), modulePath, signaturePath)
	if err != nil {
		if err == signing.ErrSignatureNotFound {
			return false, ErrSignatureNotFound
		}
		return false, err
	}

	return valid, nil
}

// GetSignerIdentity extracts the signer's identity from a signature
func (v *DefaultSignatureVerifier) GetSignerIdentity(signaturePath string) (string, error) {
	return "unknown", nil
}

// CosignVerifier implements Cosign-style verification
// Cosign uses ECDSA P-256 (default) or Ed25519 signatures
// Signatures are base64-encoded, public keys are PEM-encoded
type CosignVerifier struct {
	// RekorURL is the optional Rekor transparency log URL for keyless verification
	RekorURL string
	// SkipRekor disables Rekor verification even if a Rekor entry exists
	SkipRekor bool
}

// CosignVerifierOption configures the CosignVerifier
type CosignVerifierOption func(*CosignVerifier)

// WithRekorURL sets the Rekor transparency log URL
func WithRekorURL(url string) CosignVerifierOption {
	return func(v *CosignVerifier) {
		v.RekorURL = url
	}
}

// WithSkipRekor disables Rekor verification
func WithSkipRekor() CosignVerifierOption {
	return func(v *CosignVerifier) {
		v.SkipRekor = true
	}
}

// NewCosignVerifier creates a new Cosign verifier
func NewCosignVerifier(opts ...CosignVerifierOption) *CosignVerifier {
	v := &CosignVerifier{
		RekorURL:  "https://rekor.sigstore.dev",
		SkipRekor: true, // Default to skip Rekor for now (requires network)
	}
	for _, opt := range opts {
		opt(v)
	}
	return v
}

// VerifySignature verifies a Cosign signature against a module
// Cosign signatures are base64-encoded ECDSA or Ed25519 signatures
// over the SHA256 hash of the content
func (v *CosignVerifier) VerifySignature(modulePath, signaturePath string, publicKey []byte) (bool, error) {
	// Read module data
	moduleData, err := os.ReadFile(modulePath)
	if err != nil {
		return false, fmt.Errorf("failed to read module: %w", err)
	}

	// Read signature (base64-encoded)
	signatureB64, err := os.ReadFile(signaturePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, ErrSignatureNotFound
		}
		return false, fmt.Errorf("failed to read signature: %w", err)
	}

	// Decode base64 signature
	signature, err := decodeCosignSignature(signatureB64)
	if err != nil {
		return false, fmt.Errorf("failed to decode signature: %w", err)
	}

	// Parse public key (PEM-encoded, cosign format)
	pubKey, err := parseCosignPublicKey(publicKey)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrInvalidPublicKey, err)
	}

	// Compute SHA256 hash of module content
	hash := sha256.Sum256(moduleData)

	// Verify based on key type
	switch key := pubKey.(type) {
	case *ecdsa.PublicKey:
		return verifyCosignECDSA(key, hash[:], signature)
	case ed25519.PublicKey:
		// Ed25519 signs the message directly, not the hash
		return verifyCosignEd25519(key, moduleData, signature)
	default:
		return false, fmt.Errorf("%w: unsupported key type %T (cosign uses ECDSA P-256 or Ed25519)", ErrInvalidPublicKey, pubKey)
	}
}

// GetSignerIdentity extracts the signer's identity from a Cosign signature
// For key-based signing, this returns the public key fingerprint
// For keyless signing, this would return the OIDC identity from the certificate
func (v *CosignVerifier) GetSignerIdentity(signaturePath string) (string, error) {
	// Read signature file
	sigData, err := os.ReadFile(signaturePath)
	if err != nil {
		return "", fmt.Errorf("failed to read signature: %w", err)
	}

	// Check if this is a bundle (JSON) or raw signature (base64)
	sigStr := strings.TrimSpace(string(sigData))

	// Try to parse as JSON bundle (keyless signature)
	if strings.HasPrefix(sigStr, "{") {
		identity, err := extractIdentityFromBundle(sigData)
		if err == nil {
			return identity, nil
		}
		// Fall through to raw signature handling
	}

	// For raw signatures, we can't extract identity without the public key
	// Return a placeholder indicating key-based signing
	return "key-based-signature", nil
}

// decodeCosignSignature decodes a base64-encoded Cosign signature
func decodeCosignSignature(data []byte) ([]byte, error) {
	// Trim whitespace
	b64 := strings.TrimSpace(string(data))

	// Try standard base64 first
	sig, err := base64.StdEncoding.DecodeString(b64)
	if err == nil {
		return sig, nil
	}

	// Try URL-safe base64
	sig, err = base64.URLEncoding.DecodeString(b64)
	if err == nil {
		return sig, nil
	}

	// Try raw base64 (no padding)
	sig, err = base64.RawStdEncoding.DecodeString(b64)
	if err == nil {
		return sig, nil
	}

	return nil, fmt.Errorf("invalid base64 encoding")
}

// parseCosignPublicKey parses a Cosign public key (PEM-encoded)
// Cosign typically uses ECDSA P-256 keys
func parseCosignPublicKey(pemData []byte) (interface{}, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Cosign uses "PUBLIC KEY" type (PKIX format)
	if block.Type != "PUBLIC KEY" && block.Type != "RSA PUBLIC KEY" && block.Type != "EC PUBLIC KEY" {
		// Still try to parse it
	}

	// Try parsing as PKIX public key (standard format)
	if pubKey, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		return pubKey, nil
	}

	// Try parsing as certificate (for keyless signatures with embedded cert)
	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		return cert.PublicKey, nil
	}

	return nil, fmt.Errorf("unsupported key format")
}

// verifyCosignECDSA verifies an ECDSA signature in Cosign format
func verifyCosignECDSA(pubKey *ecdsa.PublicKey, hash, signature []byte) (bool, error) {
	// Cosign ECDSA signatures are ASN.1 DER encoded
	if ecdsa.VerifyASN1(pubKey, hash, signature) {
		return true, nil
	}

	// Try raw R||S format (64 bytes for P-256)
	if len(signature) == 64 {
		r := new(big.Int).SetBytes(signature[:32])
		s := new(big.Int).SetBytes(signature[32:])
		if ecdsa.Verify(pubKey, hash, r, s) {
			return true, nil
		}
	}

	return false, nil
}

// verifyCosignEd25519 verifies an Ed25519 signature
func verifyCosignEd25519(pubKey ed25519.PublicKey, message, signature []byte) (bool, error) {
	// Ed25519 signs the full message, not the hash
	if ed25519.Verify(pubKey, message, signature) {
		return true, nil
	}
	return false, nil
}

// CosignBundle represents a Cosign signature bundle (for keyless signing)
type CosignBundle struct {
	Base64Signature string `json:"base64Signature"`
	Cert            string `json:"cert"`
	RekorBundle     struct {
		SignedEntryTimestamp string `json:"signedEntryTimestamp"`
		Payload              struct {
			Body           string `json:"body"`
			IntegratedTime int64  `json:"integratedTime"`
			LogIndex       int64  `json:"logIndex"`
			LogID          string `json:"logID"`
		} `json:"Payload"`
	} `json:"rekorBundle,omitempty"`
}

// extractIdentityFromBundle extracts the signer identity from a Cosign bundle
func extractIdentityFromBundle(bundleData []byte) (string, error) {
	var bundle CosignBundle
	if err := json.Unmarshal(bundleData, &bundle); err != nil {
		return "", fmt.Errorf("failed to parse bundle: %w", err)
	}

	if bundle.Cert == "" {
		return "", fmt.Errorf("no certificate in bundle")
	}

	// Decode the certificate
	certPEM, err := base64.StdEncoding.DecodeString(bundle.Cert)
	if err != nil {
		return "", fmt.Errorf("failed to decode certificate: %w", err)
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		// Try as raw DER
		cert, err := x509.ParseCertificate(certPEM)
		if err != nil {
			return "", fmt.Errorf("failed to parse certificate: %w", err)
		}
		return extractIdentityFromCert(cert), nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("failed to parse certificate: %w", err)
	}

	return extractIdentityFromCert(cert), nil
}

// extractIdentityFromCert extracts identity from a Fulcio certificate
func extractIdentityFromCert(cert *x509.Certificate) string {
	// Check for email in SAN
	if len(cert.EmailAddresses) > 0 {
		return cert.EmailAddresses[0]
	}

	// Check for URI in SAN (GitHub Actions, etc.)
	if len(cert.URIs) > 0 {
		return cert.URIs[0].String()
	}

	// Fall back to subject CN
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}

	return "unknown"
}
