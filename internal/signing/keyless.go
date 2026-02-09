package signing

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

// KeylessSignerConfig configures the keyless signer.
type KeylessSignerConfig struct {
	// OIDCToken is a pre-obtained OIDC identity token.
	// For CI/CD environments (GitHub Actions, GitLab CI, etc.), this token
	// is typically available automatically.
	OIDCToken string

	// OIDCIssuer is the OIDC issuer URL (for token validation).
	OIDCIssuer string

	// FulcioURL is the Fulcio CA URL.
	// Default: https://fulcio.sigstore.dev
	FulcioURL string

	// RekorURL is the Rekor transparency log URL.
	// Default: https://rekor.sigstore.dev
	RekorURL string

	// Timeout for HTTP requests.
	Timeout time.Duration

	// Annotations are additional metadata to include in the signature.
	Annotations map[string]string

	// HTTPClient is an optional custom HTTP client.
	HTTPClient *http.Client
}

// KeylessSigner implements keyless signing using Sigstore/Fulcio.
// This signer generates ephemeral keys, obtains a certificate from Fulcio
// using an OIDC token, signs the data, and optionally records to Rekor.
type KeylessSigner struct {
	config     *KeylessSignerConfig
	httpClient *http.Client
}

// NewKeylessSigner creates a new keyless signer.
// Requires a pre-obtained OIDC token (suitable for CI/CD environments).
func NewKeylessSigner(config *KeylessSignerConfig) (*KeylessSigner, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}
	if config.OIDCToken == "" {
		return nil, fmt.Errorf("OIDC token is required for keyless signing; " +
			"in CI/CD, use the environment's OIDC token (e.g., ACTIONS_ID_TOKEN_REQUEST_TOKEN)")
	}

	// Set defaults
	if config.FulcioURL == "" {
		config.FulcioURL = "https://fulcio.sigstore.dev"
	}
	if config.RekorURL == "" {
		config.RekorURL = "https://rekor.sigstore.dev"
	}
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: config.Timeout,
		}
	}

	return &KeylessSigner{
		config:     config,
		httpClient: httpClient,
	}, nil
}

// Sign creates a keyless signature for the given data.
func (s *KeylessSigner) Sign(ctx context.Context, data []byte) (*SignatureResult, error) {
	// Generate ephemeral key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Get certificate from Fulcio
	certPEM, certChain, identity, err := s.getCertificateFromFulcio(ctx, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get certificate from Fulcio: %w", err)
	}

	// Calculate hash and sign
	hash := sha256.Sum256(data)
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Create result
	result := &SignatureResult{
		Signature:       signature,
		SignatureBase64: base64.StdEncoding.EncodeToString(signature),
		Digest:          fmt.Sprintf("sha256:%s", base64.StdEncoding.EncodeToString(hash[:])),
		DigestAlgorithm: HashSHA256,
		Timestamp:       time.Now().UTC(),
		SignerIdentity:  identity,
		Certificate:     certPEM,
		Annotations:     s.config.Annotations,
	}

	// Add certificate chain if present
	if len(certChain) > 0 {
		result.CertificateChain = certChain
	}

	// Create bundle
	result.Bundle = s.createBundle(signature, certPEM, hash[:])

	return result, nil
}

// SignFile creates a keyless signature for a file.
func (s *KeylessSigner) SignFile(ctx context.Context, path string) (*SignatureResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return s.Sign(ctx, data)
}

// KeyType returns the key type (always ECDSA for keyless signing).
func (s *KeylessSigner) KeyType() KeyType {
	return KeyTypeECDSA
}

// PublicKey returns an error as keyless signing uses ephemeral keys.
func (s *KeylessSigner) PublicKey() ([]byte, error) {
	return nil, fmt.Errorf("keyless signing uses ephemeral keys; public key is in the certificate")
}

// getCertificateFromFulcio obtains a signing certificate from Fulcio.
func (s *KeylessSigner) getCertificateFromFulcio(ctx context.Context, privateKey *ecdsa.PrivateKey) (cert []byte, chain [][]byte, identity string, err error) {
	// Create the public key bytes
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	// Create the CSR-like request for Fulcio
	// Fulcio expects a proof of possession, which is a signature over the public key
	hash := sha256.Sum256(pubKeyBytes)
	proof, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to create proof: %w", err)
	}

	// Prepare request body
	reqBody := map[string]interface{}{
		"publicKeyRequest": map[string]interface{}{
			"publicKey": map[string]interface{}{
				"algorithm": "ECDSA",
				"content":   base64.StdEncoding.EncodeToString(pubKeyBytes),
			},
			"proofOfPossession": base64.StdEncoding.EncodeToString(proof),
		},
	}

	reqBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	url := s.config.FulcioURL + "/api/v2/signingCert"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(reqBytes))
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+s.config.OIDCToken)
	req.Header.Set("Accept", "application/pem-certificate-chain")

	// Send request
	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, nil, "", fmt.Errorf("fulcio request failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Parse the certificate chain
	return s.parseCertificateChain(body)
}

// parseCertificateChain parses a PEM certificate chain.
func (s *KeylessSigner) parseCertificateChain(pemData []byte) (cert []byte, chain [][]byte, identity string, err error) {
	var certs [][]byte
	rest := pemData

	for {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}

		if block.Type == "CERTIFICATE" {
			certPEM := pem.EncodeToMemory(block)
			certs = append(certs, certPEM)

			// Extract identity from the first certificate (signing cert)
			if identity == "" {
				cert, err := x509.ParseCertificate(block.Bytes)
				if err == nil {
					identity = extractIdentityFromCert(cert)
				}
			}
		}
	}

	if len(certs) == 0 {
		return nil, nil, "", fmt.Errorf("no certificates found in response")
	}

	// First cert is the signing certificate, rest are the chain
	signingCert := certs[0]
	if len(certs) > 1 {
		chain = certs[1:]
	}

	return signingCert, chain, identity, nil
}

// createBundle creates a signature bundle for keyless signatures.
func (s *KeylessSigner) createBundle(signature, certPEM, hash []byte) *SignatureBundle {
	return &SignatureBundle{
		MediaType:   "application/vnd.kscore.signature.v1+json",
		PayloadType: "application/vnd.kscore.payload.v1+json",
		Signatures: []BundleSignature{
			{
				Sig:         base64.StdEncoding.EncodeToString(signature),
				Certificate: base64.StdEncoding.EncodeToString(certPEM),
			},
		},
	}
}

// KeylessVerifier verifies keyless signatures using the embedded certificate.
type KeylessVerifier struct {
	// TrustedRoots are the trusted root certificates for Fulcio.
	// If nil, certificate chain verification is skipped.
	TrustedRoots *x509.CertPool

	// ExpectedIssuer is the expected OIDC issuer (optional).
	ExpectedIssuer string

	// ExpectedIdentity is the expected signer identity (optional).
	// Can be an email or URI pattern.
	ExpectedIdentity string
}

// NewKeylessVerifier creates a new keyless verifier.
func NewKeylessVerifier() *KeylessVerifier {
	return &KeylessVerifier{}
}

// WithTrustedRoots sets the trusted root certificates.
func (v *KeylessVerifier) WithTrustedRoots(roots *x509.CertPool) *KeylessVerifier {
	v.TrustedRoots = roots
	return v
}

// WithExpectedIssuer sets the expected OIDC issuer.
func (v *KeylessVerifier) WithExpectedIssuer(issuer string) *KeylessVerifier {
	v.ExpectedIssuer = issuer
	return v
}

// WithExpectedIdentity sets the expected signer identity.
func (v *KeylessVerifier) WithExpectedIdentity(identity string) *KeylessVerifier {
	v.ExpectedIdentity = identity
	return v
}

// Verify verifies a keyless signature.
func (v *KeylessVerifier) Verify(ctx context.Context, data, signature, certificate []byte) (*VerificationResult, error) {
	result := &VerificationResult{
		Valid: false,
	}

	// Parse the certificate
	cert, err := parseCertificate(certificate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificate: %w", err)
	}

	result.Certificate = certificate
	result.SignerIdentity = extractIdentityFromCert(cert)

	// Check expected identity if specified
	if v.ExpectedIdentity != "" && result.SignerIdentity != v.ExpectedIdentity {
		result.Warnings = append(result.Warnings,
			fmt.Sprintf("signer identity mismatch: got %s, expected %s", result.SignerIdentity, v.ExpectedIdentity))
	}

	// Verify certificate chain if we have trusted roots
	if v.TrustedRoots != nil {
		opts := x509.VerifyOptions{
			Roots: v.TrustedRoots,
		}
		if _, err := cert.Verify(opts); err != nil {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("certificate chain verification failed: %v", err))
		}
	}

	// Verify the signature using the certificate's public key
	pubKeyPEM, err := EncodePublicKey(cert.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to encode public key: %w", err)
	}

	verifier, err := NewKeyVerifier(&KeyVerifierConfig{
		PublicKeyPEM:  pubKeyPEM,
		HashAlgorithm: HashSHA256,
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

// VerifyBundle verifies a keyless signature bundle.
func (v *KeylessVerifier) VerifyBundle(ctx context.Context, data []byte, bundle *SignatureBundle) (*VerificationResult, error) {
	if len(bundle.Signatures) == 0 {
		return nil, fmt.Errorf("no signatures in bundle")
	}

	sig := bundle.Signatures[0]

	if sig.Certificate == "" {
		return nil, fmt.Errorf("bundle has no certificate; this doesn't appear to be a keyless signature")
	}

	signature, err := base64.StdEncoding.DecodeString(sig.Sig)
	if err != nil {
		return nil, fmt.Errorf("failed to decode signature: %w", err)
	}

	certificate, err := base64.StdEncoding.DecodeString(sig.Certificate)
	if err != nil {
		return nil, fmt.Errorf("failed to decode certificate: %w", err)
	}

	return v.Verify(ctx, data, signature, certificate)
}

// GetSigner returns the appropriate signer based on available credentials.
// If an OIDC token is available, returns a keyless signer.
// Otherwise, requires a private key.
func GetSigner(config interface{}) (Signer, error) {
	switch c := config.(type) {
	case *KeySignerConfig:
		return NewKeySigner(c)
	case *KeylessSignerConfig:
		return NewKeylessSigner(c)
	default:
		return nil, fmt.Errorf("unsupported signer config type: %T", config)
	}
}
