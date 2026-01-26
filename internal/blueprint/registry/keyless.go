package registry

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
	"net/url"
	"os"
	"strings"
	"time"
)

// Default Sigstore endpoints
const (
	DefaultFulcioURL = "https://fulcio.sigstore.dev"
	DefaultRekorURL  = "https://rekor.sigstore.dev"
	DefaultOIDCIssuer = "https://oauth2.sigstore.dev/auth"
)

// KeylessSigningConfig holds configuration for keyless (OIDC) signing.
type KeylessSigningConfig struct {
	// FulcioURL is the Fulcio server URL for certificate issuance.
	FulcioURL string

	// RekorURL is the Rekor transparency log URL.
	RekorURL string

	// OIDCIssuer is the OIDC issuer URL.
	OIDCIssuer string

	// OIDCClientID is the OIDC client ID.
	OIDCClientID string

	// OIDCToken is a pre-obtained OIDC token (optional, for CI/CD).
	OIDCToken string

	// Timeout for HTTP requests.
	Timeout time.Duration

	// Annotations are additional metadata to include in the signature.
	Annotations map[string]string
}

// DefaultKeylessSigningConfig returns a KeylessSigningConfig with default values.
func DefaultKeylessSigningConfig() *KeylessSigningConfig {
	return &KeylessSigningConfig{
		FulcioURL:    DefaultFulcioURL,
		RekorURL:     DefaultRekorURL,
		OIDCIssuer:   DefaultOIDCIssuer,
		OIDCClientID: "sigstore",
		Timeout:      60 * time.Second,
	}
}

// KeylessSigner handles keyless signing operations using OIDC and Fulcio.
type KeylessSigner struct {
	config     *KeylessSigningConfig
	httpClient *http.Client
}

// NewKeylessSigner creates a new KeylessSigner with the given configuration.
func NewKeylessSigner(config *KeylessSigningConfig) (*KeylessSigner, error) {
	if config == nil {
		config = DefaultKeylessSigningConfig()
	}

	if config.FulcioURL == "" {
		config.FulcioURL = DefaultFulcioURL
	}
	if config.RekorURL == "" {
		config.RekorURL = DefaultRekorURL
	}
	if config.OIDCIssuer == "" {
		config.OIDCIssuer = DefaultOIDCIssuer
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}

	return &KeylessSigner{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}, nil
}

// KeylessSigningResult contains the result of a keyless signing operation.
type KeylessSigningResult struct {
	// Signature is the base64-encoded signature.
	Signature string

	// Certificate is the PEM-encoded signing certificate from Fulcio.
	Certificate string

	// CertificateChain is the full certificate chain.
	CertificateChain string

	// Digest is the SHA-256 digest of the signed content.
	Digest string

	// Timestamp is when the signature was created.
	Timestamp time.Time

	// RekorLogEntry contains the transparency log entry (if recorded).
	RekorLogEntry *RekorEntry

	// Annotations are metadata included in the signature.
	Annotations map[string]string

	// SignerIdentity is the identity from the OIDC token.
	SignerIdentity string
}

// RekorEntry represents an entry in the Rekor transparency log.
type RekorEntry struct {
	// LogIndex is the index in the transparency log.
	LogIndex int64 `json:"logIndex"`

	// UUID is the unique identifier of the log entry.
	UUID string `json:"uuid"`

	// IntegratedTime is when the entry was added to the log.
	IntegratedTime int64 `json:"integratedTime"`

	// LogID identifies the transparency log.
	LogID string `json:"logID"`
}

// FulcioRequest represents a certificate signing request to Fulcio.
type FulcioRequest struct {
	PublicKeyRequest PublicKeyRequest `json:"publicKeyRequest"`
}

// PublicKeyRequest is the public key portion of a Fulcio request.
type PublicKeyRequest struct {
	PublicKey         PublicKey `json:"publicKey"`
	ProofOfPossession string    `json:"proofOfPossession"`
}

// PublicKey represents a public key in Fulcio format.
type PublicKey struct {
	Algorithm string `json:"algorithm"`
	Content   string `json:"content"`
}

// FulcioResponse represents the response from Fulcio.
type FulcioResponse struct {
	SignedCertificateEmbeddedSct *SignedCertificate `json:"signedCertificateEmbeddedSct"`
}

// SignedCertificate contains the signed certificate from Fulcio.
type SignedCertificate struct {
	Chain *CertificateChain `json:"chain"`
}

// CertificateChain represents a certificate chain.
type CertificateChain struct {
	Certificates []string `json:"certificates"`
}

// Sign signs the given data using keyless signing (OIDC + Fulcio).
func (s *KeylessSigner) Sign(ctx context.Context, data []byte) (*KeylessSigningResult, error) {
	// Validate we have an OIDC token
	if s.config.OIDCToken == "" {
		return nil, fmt.Errorf("OIDC token required for keyless signing; set OIDCToken in config or use device flow")
	}

	// Generate ephemeral key pair
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ephemeral key: %w", err)
	}

	// Get certificate from Fulcio
	certPEM, certChain, signerIdentity, err := s.getCertificateFromFulcio(ctx, privateKey, s.config.OIDCToken)
	if err != nil {
		return nil, fmt.Errorf("failed to get certificate from Fulcio: %w", err)
	}

	// Calculate digest
	hash := sha256.Sum256(data)
	digest := fmt.Sprintf("sha256:%s", base64.StdEncoding.EncodeToString(hash[:]))

	// Sign the data
	signature, err := ecdsa.SignASN1(rand.Reader, privateKey, hash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign data: %w", err)
	}

	result := &KeylessSigningResult{
		Signature:        base64.StdEncoding.EncodeToString(signature),
		Certificate:      certPEM,
		CertificateChain: certChain,
		Digest:           digest,
		Timestamp:        time.Now().UTC(),
		Annotations:      s.config.Annotations,
		SignerIdentity:   signerIdentity,
	}

	// Record in Rekor transparency log
	if s.config.RekorURL != "" {
		entry, err := s.recordInRekor(ctx, data, signature, certPEM)
		if err != nil {
			// Log warning but don't fail - Rekor recording is optional
			result.Annotations["rekor_error"] = err.Error()
		} else {
			result.RekorLogEntry = entry
		}
	}

	return result, nil
}

// getCertificateFromFulcio requests a certificate from Fulcio using the OIDC token.
func (s *KeylessSigner) getCertificateFromFulcio(ctx context.Context, privateKey *ecdsa.PrivateKey, token string) (certPEM, certChain, identity string, err error) {
	// Marshal public key
	pubKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	// Create proof of possession (sign the OIDC token subject)
	// In a real implementation, this would sign a challenge from Fulcio
	tokenParts := strings.Split(token, ".")
	if len(tokenParts) >= 2 {
		// Extract identity from token claims
		claimsJSON, err := base64.RawURLEncoding.DecodeString(tokenParts[1])
		if err == nil {
			var claims map[string]interface{}
			if json.Unmarshal(claimsJSON, &claims) == nil {
				if email, ok := claims["email"].(string); ok {
					identity = email
				} else if sub, ok := claims["sub"].(string); ok {
					identity = sub
				}
			}
		}
	}

	// Create proof of possession by signing the email/identity
	proofData := []byte(identity)
	proofHash := sha256.Sum256(proofData)
	proofSig, err := ecdsa.SignASN1(rand.Reader, privateKey, proofHash[:])
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create proof of possession: %w", err)
	}

	// Create Fulcio request
	req := FulcioRequest{
		PublicKeyRequest: PublicKeyRequest{
			PublicKey: PublicKey{
				Algorithm: "ECDSA",
				Content:   base64.StdEncoding.EncodeToString(pubKeyBytes),
			},
			ProofOfPossession: base64.StdEncoding.EncodeToString(proofSig),
		},
	}

	reqBody, err := json.Marshal(req)
	if err != nil {
		return "", "", "", fmt.Errorf("failed to marshal Fulcio request: %w", err)
	}

	// Make request to Fulcio
	fulcioURL, err := url.JoinPath(s.config.FulcioURL, "/api/v2/signingCert")
	if err != nil {
		return "", "", "", fmt.Errorf("invalid Fulcio URL: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", fulcioURL, bytes.NewReader(reqBody))
	if err != nil {
		return "", "", "", fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+token)
	httpReq.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return "", "", "", fmt.Errorf("Fulcio request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return "", "", "", fmt.Errorf("Fulcio returned status %d: %s", resp.StatusCode, string(body))
	}

	var fulcioResp FulcioResponse
	if err := json.NewDecoder(resp.Body).Decode(&fulcioResp); err != nil {
		return "", "", "", fmt.Errorf("failed to decode Fulcio response: %w", err)
	}

	if fulcioResp.SignedCertificateEmbeddedSct == nil ||
		fulcioResp.SignedCertificateEmbeddedSct.Chain == nil ||
		len(fulcioResp.SignedCertificateEmbeddedSct.Chain.Certificates) == 0 {
		return "", "", "", fmt.Errorf("Fulcio response missing certificate")
	}

	certs := fulcioResp.SignedCertificateEmbeddedSct.Chain.Certificates
	certPEM = certs[0]

	// Build full chain
	var chainBuf bytes.Buffer
	for _, cert := range certs {
		chainBuf.WriteString(cert)
		chainBuf.WriteString("\n")
	}
	certChain = chainBuf.String()

	return certPEM, certChain, identity, nil
}

// recordInRekor records the signature in the Rekor transparency log.
func (s *KeylessSigner) recordInRekor(ctx context.Context, data, signature []byte, certPEM string) (*RekorEntry, error) {
	// Calculate artifact hash
	hash := sha256.Sum256(data)

	// Create Rekor entry request
	entry := map[string]interface{}{
		"kind":       "hashedrekord",
		"apiVersion": "0.0.1",
		"spec": map[string]interface{}{
			"signature": map[string]interface{}{
				"content":   base64.StdEncoding.EncodeToString(signature),
				"publicKey": map[string]string{"content": certPEM},
			},
			"data": map[string]string{
				"hash": map[string]string{
					"algorithm": "sha256",
					"value":     fmt.Sprintf("%x", hash),
				}["value"],
			},
		},
	}

	reqBody, err := json.Marshal(entry)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal Rekor request: %w", err)
	}

	rekorURL, err := url.JoinPath(s.config.RekorURL, "/api/v1/log/entries")
	if err != nil {
		return nil, fmt.Errorf("invalid Rekor URL: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", rekorURL, bytes.NewReader(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("Rekor request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Rekor returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response - Rekor returns a map with UUID as key
	var rekorResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&rekorResp); err != nil {
		return nil, fmt.Errorf("failed to decode Rekor response: %w", err)
	}

	// Extract entry from response
	for uuid, entryData := range rekorResp {
		if entry, ok := entryData.(map[string]interface{}); ok {
			result := &RekorEntry{UUID: uuid}

			if logIndex, ok := entry["logIndex"].(float64); ok {
				result.LogIndex = int64(logIndex)
			}
			if integratedTime, ok := entry["integratedTime"].(float64); ok {
				result.IntegratedTime = int64(integratedTime)
			}
			if logID, ok := entry["logID"].(string); ok {
				result.LogID = logID
			}

			return result, nil
		}
	}

	return nil, fmt.Errorf("unexpected Rekor response format")
}

// SignBlueprint signs a blueprint archive using keyless signing.
func (s *KeylessSigner) SignBlueprint(ctx context.Context, archivePath string) (*KeylessSigningResult, error) {
	f, err := os.Open(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open archive: %w", err)
	}
	defer f.Close()

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive: %w", err)
	}

	return s.Sign(ctx, data)
}

// VerifyKeylessSignature verifies a keyless signature.
func VerifyKeylessSignature(ctx context.Context, data []byte, signature string, certPEM string) (*VerificationResult, error) {
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

	// Parse certificate
	block, _ := pem.Decode([]byte(certPEM))
	if block == nil {
		result.Errors = append(result.Errors, "failed to decode certificate PEM")
		return result, nil
	}

	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		result.Errors = append(result.Errors, fmt.Sprintf("failed to parse certificate: %v", err))
		return result, nil
	}

	// Extract public key
	publicKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
	if !ok {
		result.Errors = append(result.Errors, "certificate does not contain ECDSA public key")
		return result, nil
	}

	// Verify signature
	hash := sha256.Sum256(data)
	if !ecdsa.VerifyASN1(publicKey, hash[:], sigBytes) {
		result.Errors = append(result.Errors, "signature verification failed")
		return result, nil
	}

	result.Valid = true
	result.Digest = fmt.Sprintf("sha256:%x", hash)
	result.Certificate = cert
	result.SignerIdentity = extractIdentity(cert)

	// Check certificate validity
	now := time.Now()
	if now.Before(cert.NotBefore) {
		result.Warnings = append(result.Warnings, "certificate is not yet valid")
	}
	if now.After(cert.NotAfter) {
		result.Warnings = append(result.Warnings, "certificate has expired")
	}

	return result, nil
}

// GetOIDCToken retrieves an OIDC token using device authorization flow.
// This is a placeholder - real implementation would use interactive OAuth flow.
func GetOIDCToken(ctx context.Context, issuer, clientID string) (string, error) {
	return "", fmt.Errorf("interactive OIDC token retrieval not implemented; please provide OIDCToken directly for CI/CD use")
}
