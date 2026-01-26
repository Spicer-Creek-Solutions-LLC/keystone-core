package signing

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"hash"
	"math/big"
	"os"
	"strings"
)

// KeyVerifierConfig configures the key-based verifier.
type KeyVerifierConfig struct {
	// PublicKeyPEM is the PEM-encoded public key.
	PublicKeyPEM []byte

	// PublicKeyPath is the path to the public key file (alternative to PublicKeyPEM).
	PublicKeyPath string

	// HashAlgorithm to use (default: SHA256).
	HashAlgorithm HashAlgorithm

	// AllowedFormats specifies which signature formats are accepted.
	AllowedFormats []SignatureFormat
}

// KeyVerifier implements Verifier using a public key.
type KeyVerifier struct {
	publicKey crypto.PublicKey
	keyType   KeyType
	config    *KeyVerifierConfig
}

// NewKeyVerifier creates a new key-based verifier.
func NewKeyVerifier(config *KeyVerifierConfig) (*KeyVerifier, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Load the public key
	var publicKey crypto.PublicKey
	var keyType KeyType
	var err error

	if len(config.PublicKeyPEM) > 0 {
		publicKey, keyType, err = LoadPublicKey(config.PublicKeyPEM)
	} else if config.PublicKeyPath != "" {
		publicKey, keyType, err = LoadPublicKeyFromFile(config.PublicKeyPath)
	} else {
		return nil, fmt.Errorf("public key is required (provide PublicKeyPEM or PublicKeyPath)")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load public key: %w", err)
	}

	// Set defaults
	if config.HashAlgorithm == "" {
		config.HashAlgorithm = HashSHA256
	}

	return &KeyVerifier{
		publicKey: publicKey,
		keyType:   keyType,
		config:    config,
	}, nil
}

// Verify verifies a signature against data.
func (v *KeyVerifier) Verify(ctx context.Context, data, signature []byte) (bool, error) {
	// Calculate hash
	hasher := v.newHasher()
	hasher.Write(data)
	hash := hasher.Sum(nil)

	return v.verifyHash(hash, signature)
}

// VerifyFile verifies a signature against a file.
func (v *KeyVerifier) VerifyFile(ctx context.Context, dataPath, signaturePath string) (bool, error) {
	data, err := os.ReadFile(dataPath)
	if err != nil {
		return false, fmt.Errorf("failed to read data file: %w", err)
	}

	sigData, err := os.ReadFile(signaturePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false, ErrSignatureNotFound
		}
		return false, fmt.Errorf("failed to read signature file: %w", err)
	}

	// Try to decode as base64
	signature, err := decodeSignature(sigData)
	if err != nil {
		return false, fmt.Errorf("failed to decode signature: %w", err)
	}

	return v.Verify(ctx, data, signature)
}

// GetSignerIdentity extracts the signer identity from a signature.
// For key-based signatures, this returns the key fingerprint.
func (v *KeyVerifier) GetSignerIdentity(signature []byte) (string, error) {
	pubKeyPEM, err := EncodePublicKey(v.publicKey)
	if err != nil {
		return "", err
	}
	return KeyFingerprint(pubKeyPEM), nil
}

// KeyType returns the type of the verification key.
func (v *KeyVerifier) KeyType() KeyType {
	return v.keyType
}

// newHasher creates a new hash function based on the configured algorithm.
func (v *KeyVerifier) newHasher() hash.Hash {
	switch v.config.HashAlgorithm {
	case HashSHA384:
		return sha512.New384()
	case HashSHA512:
		return sha512.New()
	default:
		return sha256.New()
	}
}

// verifyHash verifies a signature against a hash.
func (v *KeyVerifier) verifyHash(hash, signature []byte) (bool, error) {
	switch key := v.publicKey.(type) {
	case *ecdsa.PublicKey:
		return verifyECDSA(key, hash, signature)

	case *rsa.PublicKey:
		return verifyRSA(key, hash, signature, v.cryptoHash())

	case ed25519.PublicKey:
		return verifyEd25519(key, hash, signature)

	default:
		return false, fmt.Errorf("%w: %T", ErrUnsupportedKeyType, key)
	}
}

// cryptoHash returns the crypto.Hash for the configured algorithm.
func (v *KeyVerifier) cryptoHash() crypto.Hash {
	switch v.config.HashAlgorithm {
	case HashSHA384:
		return crypto.SHA384
	case HashSHA512:
		return crypto.SHA512
	default:
		return crypto.SHA256
	}
}

// verifyECDSA verifies an ECDSA signature.
func verifyECDSA(pubKey *ecdsa.PublicKey, hash, signature []byte) (bool, error) {
	// Try ASN.1 DER format first
	if ecdsa.VerifyASN1(pubKey, hash, signature) {
		return true, nil
	}

	// Try raw R||S format (64 bytes for P-256, 96 for P-384, 132 for P-521)
	keySize := (pubKey.Curve.Params().BitSize + 7) / 8
	if len(signature) == keySize*2 {
		r := new(big.Int).SetBytes(signature[:keySize])
		s := new(big.Int).SetBytes(signature[keySize:])
		if ecdsa.Verify(pubKey, hash, r, s) {
			return true, nil
		}
	}

	return false, nil
}

// verifyRSA verifies an RSA signature.
func verifyRSA(pubKey *rsa.PublicKey, hash, signature []byte, hashType crypto.Hash) (bool, error) {
	err := rsa.VerifyPKCS1v15(pubKey, hashType, hash, signature)
	return err == nil, nil
}

// verifyEd25519 verifies an Ed25519 signature.
func verifyEd25519(pubKey ed25519.PublicKey, hash, signature []byte) (bool, error) {
	// Ed25519 signs the message directly, but we sign the hash for consistency
	return ed25519.Verify(pubKey, hash, signature), nil
}

// decodeSignature decodes a signature from various formats.
func decodeSignature(data []byte) ([]byte, error) {
	str := strings.TrimSpace(string(data))

	// Check if it's a JSON bundle
	if strings.HasPrefix(str, "{") {
		return decodeSignatureBundle(data)
	}

	// Try base64 decoding
	if sig, err := base64.StdEncoding.DecodeString(str); err == nil {
		return sig, nil
	}
	if sig, err := base64.URLEncoding.DecodeString(str); err == nil {
		return sig, nil
	}
	if sig, err := base64.RawStdEncoding.DecodeString(str); err == nil {
		return sig, nil
	}

	// Assume raw bytes
	return data, nil
}

// decodeSignatureBundle extracts the signature from a bundle.
func decodeSignatureBundle(data []byte) ([]byte, error) {
	var bundle SignatureBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("failed to parse bundle: %w", err)
	}

	if len(bundle.Signatures) == 0 {
		return nil, fmt.Errorf("no signatures in bundle")
	}

	return base64.StdEncoding.DecodeString(bundle.Signatures[0].Sig)
}

// BundleVerifier verifies signature bundles with optional certificate verification.
type BundleVerifier struct {
	// TrustedRoots are the trusted root certificates.
	TrustedRoots *x509.CertPool

	// HashAlgorithm to use for verification.
	HashAlgorithm HashAlgorithm
}

// NewBundleVerifier creates a new bundle verifier.
func NewBundleVerifier() *BundleVerifier {
	return &BundleVerifier{
		HashAlgorithm: HashSHA256,
	}
}

// VerifyBundle verifies a signature bundle against data.
func (v *BundleVerifier) VerifyBundle(ctx context.Context, data []byte, bundle *SignatureBundle) (*VerificationResult, error) {
	if len(bundle.Signatures) == 0 {
		return nil, fmt.Errorf("no signatures in bundle")
	}

	result := &VerificationResult{
		Valid: false,
	}

	// Get the first signature
	sig := bundle.Signatures[0]

	// Decode the signature
	signature, err := base64.StdEncoding.DecodeString(sig.Sig)
	if err != nil {
		return nil, fmt.Errorf("failed to decode signature: %w", err)
	}

	// If there's a certificate, use it for verification
	if sig.Certificate != "" {
		return v.verifyWithCertificate(ctx, data, signature, sig.Certificate, result)
	}

	// No certificate - can't verify without a public key
	return nil, fmt.Errorf("bundle has no certificate; provide a public key for verification")
}

// verifyWithCertificate verifies using an embedded certificate.
func (v *BundleVerifier) verifyWithCertificate(ctx context.Context, data, signature []byte, certB64 string, result *VerificationResult) (*VerificationResult, error) {
	// Decode the certificate
	certPEM, err := base64.StdEncoding.DecodeString(certB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode certificate: %w", err)
	}

	// Parse the certificate
	cert, err := parseCertificate(certPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	result.Certificate = certPEM
	result.SignerIdentity = extractIdentityFromCert(cert)

	// Verify certificate chain if we have trusted roots
	if v.TrustedRoots != nil {
		opts := x509.VerifyOptions{
			Roots: v.TrustedRoots,
		}
		if _, err := cert.Verify(opts); err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("certificate chain verification failed: %v", err))
		}
	}

	// Create a verifier with the certificate's public key
	pubKeyPEM, err := EncodePublicKey(cert.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encode public key: %w", err)
	}

	verifier, err := NewKeyVerifier(&KeyVerifierConfig{
		PublicKeyPEM:  pubKeyPEM,
		HashAlgorithm: v.HashAlgorithm,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create verifier: %w", err)
	}

	valid, err := verifier.Verify(ctx, data, signature)
	if err != nil {
		return nil, err
	}

	result.Valid = valid
	return result, nil
}

// parseCertificate parses a certificate from PEM or DER format.
func parseCertificate(data []byte) (*x509.Certificate, error) {
	// Try PEM first
	block, _ := pem.Decode(data)
	if block != nil {
		return x509.ParseCertificate(block.Bytes)
	}

	// Try DER
	return x509.ParseCertificate(data)
}

// extractIdentityFromCert extracts the signer identity from a certificate.
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
