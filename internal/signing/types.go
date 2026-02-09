package signing

import (
	"context"
	"errors"
	"time"
)

// Common errors for signing operations.
var (
	ErrInvalidKey         = errors.New("invalid key")
	ErrInvalidSignature   = errors.New("invalid signature")
	ErrSignatureNotFound  = errors.New("signature not found")
	ErrVerificationFailed = errors.New("signature verification failed")
	ErrUnsupportedKeyType = errors.New("unsupported key type")
	ErrKeyEncrypted       = errors.New("key is encrypted but no password provided")
)

// KeyType represents the type of cryptographic key.
type KeyType string

// KeyTypeECDSA constants define the supported types.
const (
	KeyTypeECDSA   KeyType = "ecdsa"
	KeyTypeRSA     KeyType = "rsa"
	KeyTypeEd25519 KeyType = "ed25519"
)

// SignatureFormat represents the format of a signature.
type SignatureFormat string

const (
	// FormatRaw is a raw binary signature.
	FormatRaw SignatureFormat = "raw"

	// FormatBase64 is a base64-encoded signature.
	FormatBase64 SignatureFormat = "base64"

	// FormatCosign uses the cosign signature format.
	FormatCosign SignatureFormat = "cosign"

	// FormatBundle uses a signature bundle format with metadata.
	FormatBundle SignatureFormat = "bundle"
)

// HashAlgorithm represents the hash algorithm used for signing.
type HashAlgorithm string

// HashSHA256 constants define the hash algorithms.
const (
	HashSHA256 HashAlgorithm = "sha256"
	HashSHA384 HashAlgorithm = "sha384"
	HashSHA512 HashAlgorithm = "sha512"
)

// Signer is the interface for creating signatures.
type Signer interface {
	// Sign creates a signature for the given data.
	Sign(ctx context.Context, data []byte) (*SignatureResult, error)

	// SignFile creates a signature for the contents of a file.
	SignFile(ctx context.Context, path string) (*SignatureResult, error)

	// KeyType returns the type of key used for signing.
	KeyType() KeyType

	// PublicKey returns the public key in PEM format.
	PublicKey() ([]byte, error)
}

// Verifier is the interface for verifying signatures.
type Verifier interface {
	// Verify verifies a signature against data.
	Verify(ctx context.Context, data, signature []byte) (bool, error)

	// VerifyFile verifies a signature against a file.
	VerifyFile(ctx context.Context, dataPath, signaturePath string) (bool, error)

	// GetSignerIdentity extracts the signer identity from a signature.
	GetSignerIdentity(signature []byte) (string, error)
}

// SignatureResult contains the result of a signing operation.
type SignatureResult struct {
	// Signature is the raw signature bytes.
	Signature []byte

	// SignatureBase64 is the base64-encoded signature.
	SignatureBase64 string

	// Digest is the digest of the signed content.
	Digest string

	// DigestAlgorithm is the algorithm used for the digest.
	DigestAlgorithm HashAlgorithm

	// Timestamp is when the signature was created.
	Timestamp time.Time

	// SignerIdentity identifies the signer (email, key fingerprint, etc.).
	SignerIdentity string

	// Certificate is the signing certificate in PEM format (optional).
	Certificate []byte

	// CertificateChain is the certificate chain (optional).
	CertificateChain [][]byte

	// Bundle is the complete signature bundle (for bundle format).
	Bundle *SignatureBundle

	// Annotations are additional metadata.
	Annotations map[string]string
}

// SignatureBundle represents a complete signature bundle.
type SignatureBundle struct {
	// MediaType describes the bundle format.
	MediaType string `json:"mediaType"`

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

	// Certificate is the signing certificate in base64 (optional).
	Certificate string `json:"cert,omitempty"`
}

// VerificationResult contains the result of a verification operation.
type VerificationResult struct {
	// Valid indicates if the signature is valid.
	Valid bool

	// SignerIdentity identifies who signed (if determinable).
	SignerIdentity string

	// Timestamp is when the signature was created (if determinable).
	Timestamp *time.Time

	// Certificate is the signing certificate (if present).
	Certificate []byte

	// Warnings are non-fatal issues found during verification.
	Warnings []string
}

// KeyPair represents a public/private key pair.
type KeyPair struct {
	// PrivateKey is the PEM-encoded private key.
	PrivateKey []byte

	// PublicKey is the PEM-encoded public key.
	PublicKey []byte

	// Type is the key type.
	Type KeyType
}
