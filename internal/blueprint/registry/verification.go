package registry

import (
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

// VerificationResult contains the result of a signature verification.
type VerificationResult struct {
	// Valid indicates whether the signature is valid.
	Valid bool

	// Digest is the SHA-256 digest of the verified content.
	Digest string

	// SignerIdentity contains information about the signer.
	SignerIdentity string

	// Timestamp is when the signature was verified.
	Timestamp time.Time

	// Errors contains any verification errors.
	Errors []string

	// Warnings contains any verification warnings.
	Warnings []string

	// Certificate contains the signing certificate (if present).
	Certificate *x509.Certificate

	// Annotations are metadata from the signature.
	Annotations map[string]string
}

// VerificationConfig holds configuration for verification operations.
type VerificationConfig struct {
	// PublicKeyPath is the path to the public key file.
	PublicKeyPath string

	// PublicKeyData is the public key data (alternative to path).
	PublicKeyData []byte

	// TrustedRoots is a list of trusted root certificates.
	TrustedRoots []*x509.Certificate

	// CertPool is a certificate pool for chain verification.
	CertPool *x509.CertPool

	// AllowExpiredCerts allows verification of expired certificates.
	AllowExpiredCerts bool

	// RequireCertificate requires a certificate in the signature.
	RequireCertificate bool

	// ExpectedIdentity is the expected signer identity (optional).
	ExpectedIdentity string

	// ExpectedAnnotations are required annotations (optional).
	ExpectedAnnotations map[string]string
}

// Verifier handles blueprint signature verification using the shared signing package.
type Verifier struct {
	config      *VerificationConfig
	keyVerifier *signing.KeyVerifier
	keyType     KeyType
}

// NewVerifier creates a new Verifier with the given configuration.
func NewVerifier(config *VerificationConfig) (*Verifier, error) {
	if config == nil {
		return nil, fmt.Errorf("verification config is required")
	}

	// Create the underlying verifier using the shared signing package
	keyVerifier, err := signing.NewKeyVerifier(&signing.KeyVerifierConfig{
		PublicKeyPEM:  config.PublicKeyData,
		PublicKeyPath: config.PublicKeyPath,
		HashAlgorithm: signing.HashSHA256,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create verifier: %w", err)
	}

	// Map signing.KeyType to registry.KeyType
	var kt KeyType
	switch keyVerifier.KeyType() {
	case signing.KeyTypeECDSA:
		kt = KeyTypeECDSA
	case signing.KeyTypeRSA:
		kt = KeyTypeRSA
	case signing.KeyTypeEd25519:
		kt = KeyTypeEd25519
	}

	return &Verifier{
		config:      config,
		keyVerifier: keyVerifier,
		keyType:     kt,
	}, nil
}

// Verify verifies a signature against data.
func (v *Verifier) Verify(ctx context.Context, data []byte, signature string) (*VerificationResult, error) {
	result := &VerificationResult{
		Valid:     false,
		Timestamp: time.Now().UTC(),
		Errors:    []string{},
		Warnings:  []string{},
	}

	// Decode signature
	sigBytes, err := base64.StdEncoding.DecodeString(signature)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to decode signature: %v", err))
		return result, nil
	}

	// Calculate digest
	hash := sha256.Sum256(data)
	result.Digest = fmt.Sprintf("sha256:%x", hash)

	// Verify signature using the shared signing package
	valid, err := v.keyVerifier.Verify(ctx, data, sigBytes)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("signature verification failed: %v", err))
		return result, nil
	}

	result.Valid = valid
	if !valid {
		result.Errors = append(result.Errors, "signature does not match")
	}

	return result, nil
}

// VerifyBundle verifies a signature bundle.
func (v *Verifier) VerifyBundle(ctx context.Context, data []byte, bundle *SignatureBundle) (*VerificationResult, error) {
	result := &VerificationResult{
		Valid:       false,
		Timestamp:   time.Now().UTC(),
		Errors:      []string{},
		Warnings:    []string{},
		Annotations: make(map[string]string),
	}

	if bundle == nil {
		result.Errors = append(result.Errors, "bundle is nil")
		return result, nil
	}

	if len(bundle.Signatures) == 0 {
		result.Errors = append(result.Errors, "bundle has no signatures")
		return result, nil
	}

	// Decode payload
	payloadBytes, err := base64.StdEncoding.DecodeString(bundle.Payload)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to decode payload: %v", err))
		return result, nil
	}

	// Parse payload
	var payload SignaturePayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse payload: %v", err))
		return result, nil
	}

	// Calculate expected digest
	hash := sha256.Sum256(data)
	expectedDigest := fmt.Sprintf("sha256:%x", hash)
	result.Digest = expectedDigest

	// Verify digest in payload matches
	if payload.Critical.Image.DockerManifestDigest != expectedDigest {
		result.Errors = append(result.Errors, fmt.Sprintf("digest mismatch: expected %s, got %s",
			expectedDigest, payload.Critical.Image.DockerManifestDigest))
		return result, nil
	}

	// Extract annotations
	for k, v := range payload.Optional {
		if s, ok := v.(string); ok {
			result.Annotations[k] = s
		}
	}

	// Verify expected annotations
	for k, expectedVal := range v.config.ExpectedAnnotations {
		if actualVal, ok := result.Annotations[k]; !ok {
			result.Errors = append(result.Errors, fmt.Sprintf("missing required annotation: %s", k))
		} else if actualVal != expectedVal {
			result.Errors = append(result.Errors, fmt.Sprintf("annotation mismatch for %s: expected %s, got %s", k, expectedVal, actualVal))
		}
	}

	if len(result.Errors) > 0 {
		return result, nil
	}

	// Verify first signature
	sig := bundle.Signatures[0]
	sigBytes, err := base64.StdEncoding.DecodeString(sig.Sig)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to decode signature: %v", err))
		return result, nil
	}

	// Verify against payload using the shared signing package
	valid, err := v.keyVerifier.Verify(ctx, payloadBytes, sigBytes)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("signature verification failed: %v", err))
		return result, nil
	}

	if !valid {
		result.Errors = append(result.Errors, "signature does not match")
		return result, nil
	}

	// Verify certificate if present
	if sig.Certificate != "" {
		cert, err := v.verifyCertificate(sig.Certificate)
		if err != nil {
			result.Warnings = append(result.Warnings, fmt.Sprintf("certificate verification: %v", err))
		} else {
			result.Certificate = cert
			result.SignerIdentity = extractIdentity(cert)

			// Verify expected identity
			if v.config.ExpectedIdentity != "" && result.SignerIdentity != v.config.ExpectedIdentity {
				result.Errors = append(result.Errors, fmt.Sprintf("identity mismatch: expected %s, got %s",
					v.config.ExpectedIdentity, result.SignerIdentity))
				return result, nil
			}
		}
	} else if v.config.RequireCertificate {
		result.Errors = append(result.Errors, "certificate required but not present")
		return result, nil
	}

	result.Valid = len(result.Errors) == 0
	return result, nil
}

// verifyCertificate verifies a base64-encoded certificate.
func (v *Verifier) verifyCertificate(certData string) (*x509.Certificate, error) {
	certBytes, err := base64.StdEncoding.DecodeString(certData)
	if err != nil {
		return nil, fmt.Errorf("failed to decode certificate: %w", err)
	}

	// Try to parse as PEM first
	block, _ := pem.Decode(certBytes)
	var certDER []byte
	if block != nil {
		certDER = block.Bytes
	} else {
		certDER = certBytes
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	// Check expiration
	now := time.Now()
	if now.Before(cert.NotBefore) {
		return cert, fmt.Errorf("certificate not yet valid")
	}
	if now.After(cert.NotAfter) && !v.config.AllowExpiredCerts {
		return cert, fmt.Errorf("certificate has expired")
	}

	// Verify against trusted roots if provided
	if v.config.CertPool != nil {
		opts := x509.VerifyOptions{
			Roots:       v.config.CertPool,
			CurrentTime: now,
		}
		if _, err := cert.Verify(opts); err != nil {
			return cert, fmt.Errorf("certificate chain verification failed: %w", err)
		}
	}

	return cert, nil
}

// extractIdentity extracts identity from a certificate.
func extractIdentity(cert *x509.Certificate) string {
	if cert == nil {
		return ""
	}

	// Try email first
	if len(cert.EmailAddresses) > 0 {
		return cert.EmailAddresses[0]
	}

	// Try SAN URIs (for SPIFFE IDs)
	for _, uri := range cert.URIs {
		if strings.HasPrefix(uri.String(), "spiffe://") {
			return uri.String()
		}
	}

	// Try DNS names
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0]
	}

	// Fall back to subject CN
	if cert.Subject.CommonName != "" {
		return cert.Subject.CommonName
	}

	return cert.Subject.String()
}

// VerifyBlueprint verifies a blueprint archive with a signature.
func (v *Verifier) VerifyBlueprint(ctx context.Context, archivePath, signature string) (*VerificationResult, error) {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive: %w", err)
	}

	return v.Verify(ctx, data, signature)
}

// VerifyBlueprintBundle verifies a blueprint archive with a signature bundle.
func (v *Verifier) VerifyBlueprintBundle(ctx context.Context, archivePath string, bundle *SignatureBundle) (*VerificationResult, error) {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive: %w", err)
	}

	return v.VerifyBundle(ctx, data, bundle)
}

// KeyType returns the type of the verification key.
func (v *Verifier) KeyType() KeyType {
	return v.keyType
}

// GetPublicKeyFingerprint returns the fingerprint of the public key.
func (v *Verifier) GetPublicKeyFingerprint() string {
	identity, err := v.keyVerifier.GetSignerIdentity(nil)
	if err != nil {
		return ""
	}
	return identity
}

// VerifyDigest verifies that the data matches the expected digest.
func VerifyDigest(data []byte, expectedDigest string) bool {
	hash := sha256.Sum256(data)
	actualDigest := fmt.Sprintf("sha256:%x", hash)

	// Handle both with and without prefix
	if !strings.HasPrefix(expectedDigest, "sha256:") {
		expectedDigest = "sha256:" + expectedDigest
	}

	return strings.EqualFold(actualDigest, expectedDigest)
}

// VerifyBlueprintDigest verifies a blueprint archive's digest.
func VerifyBlueprintDigest(archivePath, expectedDigest string) (bool, error) {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return false, fmt.Errorf("failed to read archive: %w", err)
	}

	return VerifyDigest(data, expectedDigest), nil
}

// TrustPolicy defines a policy for trusting signatures.
type TrustPolicy struct {
	// Name is the policy name.
	Name string

	// TrustedKeys are public keys trusted by this policy.
	TrustedKeys [][]byte

	// TrustedIdentities are identities trusted by this policy.
	TrustedIdentities []string

	// RequireSignature requires all blueprints to be signed.
	RequireSignature bool

	// RequireCertificate requires certificates in signatures.
	RequireCertificate bool

	// AllowExpired allows expired certificates.
	AllowExpired bool

	// RequiredAnnotations are annotations that must be present.
	RequiredAnnotations map[string]string
}

// TrustPolicyEvaluator evaluates blueprints against a trust policy.
type TrustPolicyEvaluator struct {
	policy *TrustPolicy
}

// NewTrustPolicyEvaluator creates a new trust policy evaluator.
func NewTrustPolicyEvaluator(policy *TrustPolicy) *TrustPolicyEvaluator {
	return &TrustPolicyEvaluator{policy: policy}
}

// Evaluate evaluates a verification result against the policy.
func (e *TrustPolicyEvaluator) Evaluate(result *VerificationResult) (allowed bool, violations []string) {

	if e.policy.RequireSignature && !result.Valid {
		violations = append(violations, "signature is required but verification failed")
	}

	if e.policy.RequireCertificate && result.Certificate == nil {
		violations = append(violations, "certificate is required but not present")
	}

	// Check trusted identities
	if len(e.policy.TrustedIdentities) > 0 && result.SignerIdentity != "" {
		trusted := false
		for _, id := range e.policy.TrustedIdentities {
			if matchIdentity(result.SignerIdentity, id) {
				trusted = true
				break
			}
		}
		if !trusted {
			violations = append(violations, fmt.Sprintf("signer identity %s is not trusted", result.SignerIdentity))
		}
	}

	// Check required annotations
	for k, v := range e.policy.RequiredAnnotations {
		if actual, ok := result.Annotations[k]; !ok {
			violations = append(violations, fmt.Sprintf("missing required annotation: %s", k))
		} else if actual != v {
			violations = append(violations, fmt.Sprintf("annotation %s mismatch: expected %s, got %s", k, v, actual))
		}
	}

	return len(violations) == 0, violations
}

// matchIdentity matches an identity against a pattern.
func matchIdentity(identity, pattern string) bool {
	if pattern == "*" {
		return true
	}

	// Support prefix matching with *
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(identity, strings.TrimSuffix(pattern, "*"))
	}

	// Support suffix matching with *
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(identity, strings.TrimPrefix(pattern, "*"))
	}

	return identity == pattern
}
