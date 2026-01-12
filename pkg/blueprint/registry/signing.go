package registry

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"
	"time"
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

// Signer handles blueprint signing operations.
type Signer struct {
	config     *SigningConfig
	privateKey crypto.PrivateKey
	publicKey  crypto.PublicKey
	keyType    KeyType
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

	signer := &Signer{
		config: config,
	}

	// Load the private key
	if err := signer.loadKey(); err != nil {
		return nil, fmt.Errorf("failed to load key: %w", err)
	}

	return signer, nil
}

// loadKey loads the private key from file.
func (s *Signer) loadKey() error {
	keyData, err := os.ReadFile(s.config.KeyPath)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	block, _ := pem.Decode(keyData)
	if block == nil {
		return fmt.Errorf("failed to decode PEM block")
	}

	var keyBytes []byte
	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck // Deprecated but still used
		if s.config.KeyPassword == "" {
			return fmt.Errorf("key is encrypted but no password provided")
		}
		keyBytes, err = x509.DecryptPEMBlock(block, []byte(s.config.KeyPassword)) //nolint:staticcheck
		if err != nil {
			return fmt.Errorf("failed to decrypt key: %w", err)
		}
	} else {
		keyBytes = block.Bytes
	}

	// Try to parse as different key types
	switch block.Type {
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(keyBytes)
		if err != nil {
			return fmt.Errorf("failed to parse EC private key: %w", err)
		}
		s.privateKey = key
		s.publicKey = &key.PublicKey
		s.keyType = KeyTypeECDSA
		return nil

	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(keyBytes)
		if err != nil {
			return fmt.Errorf("failed to parse RSA private key: %w", err)
		}
		s.privateKey = key
		s.publicKey = &key.PublicKey
		s.keyType = KeyTypeRSA
		return nil

	case "PRIVATE KEY":
		key, err := x509.ParsePKCS8PrivateKey(keyBytes)
		if err != nil {
			return fmt.Errorf("failed to parse PKCS8 private key: %w", err)
		}
		return s.setKeyFromParsed(key)

	case "ED25519 PRIVATE KEY":
		if len(keyBytes) != ed25519.PrivateKeySize {
			// Try PKCS8 format
			key, err := x509.ParsePKCS8PrivateKey(keyBytes)
			if err != nil {
				return fmt.Errorf("failed to parse Ed25519 private key: %w", err)
			}
			return s.setKeyFromParsed(key)
		}
		s.privateKey = ed25519.PrivateKey(keyBytes)
		s.publicKey = ed25519.PrivateKey(keyBytes).Public()
		s.keyType = KeyTypeEd25519
		return nil

	default:
		// Try PKCS8 as fallback
		key, err := x509.ParsePKCS8PrivateKey(keyBytes)
		if err != nil {
			return fmt.Errorf("unsupported key type: %s", block.Type)
		}
		return s.setKeyFromParsed(key)
	}
}

func (s *Signer) setKeyFromParsed(key interface{}) error {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		s.privateKey = k
		s.publicKey = &k.PublicKey
		s.keyType = KeyTypeECDSA
	case *rsa.PrivateKey:
		s.privateKey = k
		s.publicKey = &k.PublicKey
		s.keyType = KeyTypeRSA
	case ed25519.PrivateKey:
		s.privateKey = k
		s.publicKey = k.Public()
		s.keyType = KeyTypeEd25519
	default:
		return fmt.Errorf("unsupported key type: %T", key)
	}
	return nil
}

// Sign signs the given data and returns a SigningResult.
func (s *Signer) Sign(ctx context.Context, data []byte) (*SigningResult, error) {
	// Calculate digest
	hash := sha256.Sum256(data)
	digest := fmt.Sprintf("sha256:%s", base64.StdEncoding.EncodeToString(hash[:]))

	// Create signature based on format
	var signature []byte
	var err error

	switch s.config.Format {
	case SignatureFormatCosign, SignatureFormatDetached:
		signature, err = s.signData(hash[:])
		if err != nil {
			return nil, fmt.Errorf("failed to sign data: %w", err)
		}

	case SignatureFormatBundle:
		return s.signBundle(ctx, data, hash[:])

	default:
		return nil, fmt.Errorf("unsupported signature format: %s", s.config.Format)
	}

	result := &SigningResult{
		Signature:   base64.StdEncoding.EncodeToString(signature),
		Digest:      digest,
		Timestamp:   time.Now().UTC(),
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

// signData signs the hash using the private key.
func (s *Signer) signData(hash []byte) ([]byte, error) {
	switch key := s.privateKey.(type) {
	case *ecdsa.PrivateKey:
		return ecdsa.SignASN1(rand.Reader, key, hash)

	case *rsa.PrivateKey:
		return rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash)

	case ed25519.PrivateKey:
		return ed25519.Sign(key, hash), nil

	default:
		return nil, fmt.Errorf("unsupported key type: %T", key)
	}
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

	// Sign the payload
	payloadHash := sha256.Sum256(payloadBytes)
	signature, err := s.signData(payloadHash[:])
	if err != nil {
		return nil, fmt.Errorf("failed to sign payload: %w", err)
	}

	bundle := &SignatureBundle{
		PayloadType: "application/vnd.kscore.blueprint.v1+json",
		Payload:     base64.StdEncoding.EncodeToString(payloadBytes),
		Signatures: []BundleSignature{
			{
				Sig: base64.StdEncoding.EncodeToString(signature),
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
		Signature:   base64.StdEncoding.EncodeToString(signature),
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
	var keyBytes []byte
	var err error

	switch key := s.publicKey.(type) {
	case *ecdsa.PublicKey:
		keyBytes, err = x509.MarshalPKIXPublicKey(key)
	case *rsa.PublicKey:
		keyBytes, err = x509.MarshalPKIXPublicKey(key)
	case ed25519.PublicKey:
		keyBytes, err = x509.MarshalPKIXPublicKey(key)
	default:
		return nil, fmt.Errorf("unsupported key type: %T", key)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	block := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: keyBytes,
	}

	var buf bytes.Buffer
	if err := pem.Encode(&buf, block); err != nil {
		return nil, fmt.Errorf("failed to encode PEM: %w", err)
	}

	return buf.Bytes(), nil
}

// KeyType returns the type of the signing key.
func (s *Signer) KeyType() KeyType {
	return s.keyType
}

// GenerateKeyPair generates a new key pair.
func GenerateKeyPair(keyType KeyType, bits int) (privateKey, publicKey []byte, err error) {
	var priv crypto.PrivateKey
	var pub crypto.PublicKey

	switch keyType {
	case KeyTypeECDSA:
		curve := elliptic.P256()
		if bits >= 384 {
			curve = elliptic.P384()
		} else if bits >= 521 {
			curve = elliptic.P521()
		}
		key, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
		}
		priv = key
		pub = &key.PublicKey

	case KeyTypeRSA:
		if bits < 2048 {
			bits = 2048
		}
		key, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate RSA key: %w", err)
		}
		priv = key
		pub = &key.PublicKey

	case KeyTypeEd25519:
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to generate Ed25519 key: %w", err)
		}
		priv = privKey
		pub = pubKey

	default:
		return nil, nil, fmt.Errorf("unsupported key type: %s", keyType)
	}

	// Marshal private key
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	privBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	}

	var privBuf bytes.Buffer
	if err := pem.Encode(&privBuf, privBlock); err != nil {
		return nil, nil, fmt.Errorf("failed to encode private key PEM: %w", err)
	}

	// Marshal public key
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	pubBlock := &pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}

	var pubBuf bytes.Buffer
	if err := pem.Encode(&pubBuf, pubBlock); err != nil {
		return nil, nil, fmt.Errorf("failed to encode public key PEM: %w", err)
	}

	return privBuf.Bytes(), pubBuf.Bytes(), nil
}

// EncryptPrivateKey encrypts a private key with a password.
func EncryptPrivateKey(privateKey []byte, password string) ([]byte, error) {
	block, _ := pem.Decode(privateKey)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	//nolint:staticcheck // Using deprecated function for compatibility
	encBlock, err := x509.EncryptPEMBlock(rand.Reader, block.Type, block.Bytes, []byte(password), x509.PEMCipherAES256)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt private key: %w", err)
	}

	var buf bytes.Buffer
	if err := pem.Encode(&buf, encBlock); err != nil {
		return nil, fmt.Errorf("failed to encode encrypted PEM: %w", err)
	}

	return buf.Bytes(), nil
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

// FormatFingerprint formats a public key fingerprint.
func FormatFingerprint(publicKey []byte) string {
	block, _ := pem.Decode(publicKey)
	if block == nil {
		return ""
	}

	hash := sha256.Sum256(block.Bytes)
	return fmt.Sprintf("sha256:%s", formatHexFingerprint(hash[:]))
}

// formatHexFingerprint formats bytes as a colon-separated hex string.
func formatHexFingerprint(data []byte) string {
	var parts []string
	for _, b := range data[:8] { // Use first 8 bytes for short fingerprint
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	return strings.Join(parts, ":")
}
