package registry

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

// SignatureFormat represents the format of a signature.
type SignatureFormat string

const (
	// SignatureFormatCosign uses the cosign signature format.
	SignatureFormatCosign SignatureFormat = "cosign"

	// SignatureFormatDetached uses a detached signature format.
	SignatureFormatDetached SignatureFormat = "detached"

	// SignatureFormatBundle uses a signature bundle format.
	SignatureFormatBundle SignatureFormat = "bundle"
)

// KeyType represents the type of cryptographic key.
type KeyType string

// KeyTypeECDSA constants define the supported types.
const (
	KeyTypeECDSA   KeyType = "ecdsa"
	KeyTypeRSA     KeyType = "rsa"
	KeyTypeEd25519 KeyType = "ed25519"
)

// SigningConfig holds configuration for signing operations.
type SigningConfig struct {
	// KeyPath is the path to the private key file.
	KeyPath string

	// KeyPassword is the password for encrypted keys.
	KeyPassword string

	// CertPath is the path to the certificate file (optional).
	CertPath string

	// Format specifies the signature format.
	Format SignatureFormat

	// Annotations are additional metadata to include in the signature.
	Annotations map[string]string
}

// SigningResult contains the result of a signing operation.
type SigningResult struct {
	// Signature is the base64-encoded signature.
	Signature string

	// Certificate is the base64-encoded certificate (if provided).
	Certificate string

	// Digest is the SHA-256 digest of the signed content.
	Digest string

	// Timestamp is when the signature was created.
	Timestamp time.Time

	// Annotations are metadata included in the signature.
	Annotations map[string]string

	// Bundle is the complete signature bundle (for bundle format).
	Bundle *SignatureBundle
}

// SignatureBundle represents a complete signature bundle.
type SignatureBundle struct {
	// PayloadType describes the type of payload.
	PayloadType string `json:"payloadType"`

	// Payload is the base64-encoded payload.
	Payload string `json:"payload"`

	// Signatures contains the list of signatures.
	Signatures []BundleSignature `json:"signatures"`
}

// BundleSignature represents a single signature in a bundle.
type BundleSignature struct {
	// KeyID identifies the signing key.
	KeyID string `json:"keyid,omitempty"`

	// Sig is the base64-encoded signature.
	Sig string `json:"sig"`

	// Certificate is the signing certificate (optional).
	Certificate string `json:"cert,omitempty"`
}

// SignaturePayload is the payload that gets signed.
type SignaturePayload struct {
	// Critical contains critical signature data.
	Critical CriticalPayload `json:"critical"`

	// Optional contains optional metadata.
	Optional map[string]interface{} `json:"optional,omitempty"`
}

// CriticalPayload contains critical signature data.
type CriticalPayload struct {
	// Type identifies the payload type.
	Type string `json:"type"`

	// Image contains the image/artifact reference.
	Image ImageRef `json:"image"`

	// Identity contains signer identity (optional).
	Identity *SignerIdentity `json:"identity,omitempty"`
}

// ImageRef contains the artifact reference.
type ImageRef struct {
	// DockerManifestDigest is the digest of the artifact.
	DockerManifestDigest string `json:"docker-manifest-digest"`
}

// SignerIdentity contains information about the signer.
type SignerIdentity struct {
	// DockerReference is the expected image reference (optional).
	DockerReference string `json:"docker-reference,omitempty"`
}

// Signer handles blueprint signing operations using the shared signing package.
type Signer struct {
	config    *SigningConfig
	keySigner *signing.KeySigner
	keyType   KeyType
}

// NewSigner creates a new Signer with the given configuration.
func NewSigner(config *SigningConfig) (*Signer, error) {
	if config == nil {
		return nil, fmt.Errorf("signing config is required")
	}
	if config.KeyPath == "" {
		return nil, fmt.Errorf("key path is required")
	}
	if config.Format == "" {
		config.Format = SignatureFormatCosign
	}

	// Create the underlying signer using the shared signing package
	keySigner, err := signing.NewKeySigner(&signing.KeySignerConfig{
		PrivateKeyPath: config.KeyPath,
		Password:       config.KeyPassword,
		HashAlgorithm:  signing.HashSHA256,
		Annotations:    config.Annotations,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	// Map signing.KeyType to registry.KeyType
	var kt KeyType
	switch keySigner.KeyType() {
	case signing.KeyTypeECDSA:
		kt = KeyTypeECDSA
	case signing.KeyTypeRSA:
		kt = KeyTypeRSA
	case signing.KeyTypeEd25519:
		kt = KeyTypeEd25519
	}

	return &Signer{
		config:    config,
		keySigner: keySigner,
		keyType:   kt,
	}, nil
}

// Sign signs the given data and returns a SigningResult.
func (s *Signer) Sign(ctx context.Context, data []byte) (*SigningResult, error) {
	switch s.config.Format {
	case SignatureFormatBundle:
		hash := sha256.Sum256(data)
		return s.signBundle(ctx, data, hash[:])
	default:
	}

	// Use the shared signing package for cosign and detached formats
	sigResult, err := s.keySigner.Sign(ctx, data)
	if err != nil {
		return nil, fmt.Errorf("failed to sign data: %w", err)
	}

	result := &SigningResult{
		Signature:   sigResult.SignatureBase64,
		Digest:      sigResult.Digest,
		Timestamp:   sigResult.Timestamp,
		Annotations: s.config.Annotations,
	}

	// Load certificate if provided
	if s.config.CertPath != "" {
		certData, err := os.ReadFile(s.config.CertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate: %w", err)
		}
		result.Certificate = base64.StdEncoding.EncodeToString(certData)
	}

	return result, nil
}

// signBundle creates a signature bundle.
func (s *Signer) signBundle(ctx context.Context, data, hash []byte) (*SigningResult, error) {
	// Create payload
	payload := SignaturePayload{
		Critical: CriticalPayload{
			Type: "blueprint signature",
			Image: ImageRef{
				DockerManifestDigest: fmt.Sprintf("sha256:%x", hash),
			},
		},
		Optional: make(map[string]interface{}),
	}

	// Add annotations to optional
	for k, v := range s.config.Annotations {
		payload.Optional[k] = v
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Sign the payload using the shared signer
	sigResult, err := s.keySigner.Sign(ctx, payloadBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to sign payload: %w", err)
	}

	bundle := &SignatureBundle{
		PayloadType: "application/vnd.kscore.blueprint.v1+json",
		Payload:     base64.StdEncoding.EncodeToString(payloadBytes),
		Signatures: []BundleSignature{
			{
				Sig: sigResult.SignatureBase64,
			},
		},
	}

	// Add certificate if provided
	if s.config.CertPath != "" {
		certData, err := os.ReadFile(s.config.CertPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read certificate: %w", err)
		}
		bundle.Signatures[0].Certificate = base64.StdEncoding.EncodeToString(certData)
	}

	return &SigningResult{
		Signature:   sigResult.SignatureBase64,
		Digest:      fmt.Sprintf("sha256:%x", hash),
		Timestamp:   time.Now().UTC(),
		Annotations: s.config.Annotations,
		Bundle:      bundle,
	}, nil
}

// SignBlueprint signs a blueprint archive.
func (s *Signer) SignBlueprint(ctx context.Context, archivePath string) (*SigningResult, error) {
	data, err := os.ReadFile(archivePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read archive: %w", err)
	}

	return s.Sign(ctx, data)
}

// GetPublicKey returns the public key in PEM format.
func (s *Signer) GetPublicKey() ([]byte, error) {
	return s.keySigner.PublicKey()
}

// KeyType returns the type of the signing key.
func (s *Signer) KeyType() KeyType {
	return s.keyType
}

// GenerateKeyPair generates a new key pair using the shared signing package.
func GenerateKeyPair(keyType KeyType, bits int) (privateKey, publicKey []byte, err error) {
	var kt signing.KeyType
	switch keyType {
	case KeyTypeECDSA:
		kt = signing.KeyTypeECDSA
	case KeyTypeRSA:
		kt = signing.KeyTypeRSA
	case KeyTypeEd25519:
		kt = signing.KeyTypeEd25519
	default:
		return nil, nil, fmt.Errorf("unsupported key type: %s", keyType)
	}

	keyPair, err := signing.GenerateKeyPair(kt, bits)
	if err != nil {
		return nil, nil, err
	}

	return keyPair.PrivateKey, keyPair.PublicKey, nil
}

// EncryptPrivateKey encrypts a private key with a password using the shared signing package.
func EncryptPrivateKey(privateKey []byte, password string) ([]byte, error) {
	return signing.EncryptPrivateKey(privateKey, password)
}

// ParseSignature parses a base64-encoded signature.
func ParseSignature(sig string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(sig)
}

// ParseSignatureBundle parses a signature bundle from JSON.
func ParseSignatureBundle(data []byte) (*SignatureBundle, error) {
	var bundle SignatureBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("failed to parse signature bundle: %w", err)
	}
	return &bundle, nil
}

// FormatFingerprint formats a public key fingerprint using the shared signing package.
func FormatFingerprint(publicKey []byte) string {
	return signing.KeyFingerprint(publicKey)
}
