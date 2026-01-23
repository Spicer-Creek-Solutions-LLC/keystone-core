package signing

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"hash"
	"os"
	"time"
)

// KeySignerConfig configures the key-based signer.
type KeySignerConfig struct {
	// PrivateKeyPEM is the PEM-encoded private key.
	PrivateKeyPEM []byte

	// PrivateKeyPath is the path to the private key file (alternative to PrivateKeyPEM).
	PrivateKeyPath string

	// Password for encrypted private keys.
	Password string

	// HashAlgorithm to use (default: SHA256).
	HashAlgorithm HashAlgorithm

	// Format specifies the output signature format.
	Format SignatureFormat

	// Annotations are additional metadata to include.
	Annotations map[string]string
}

// KeySigner implements Signer using a private key.
type KeySigner struct {
	privateKey crypto.PrivateKey
	publicKey  crypto.PublicKey
	keyType    KeyType
	config     *KeySignerConfig
}

// NewKeySigner creates a new key-based signer.
func NewKeySigner(config *KeySignerConfig) (*KeySigner, error) {
	if config == nil {
		return nil, fmt.Errorf("config is required")
	}

	// Load the private key
	var privateKey crypto.PrivateKey
	var keyType KeyType
	var err error

	if len(config.PrivateKeyPEM) > 0 {
		privateKey, keyType, err = LoadPrivateKey(config.PrivateKeyPEM, config.Password)
	} else if config.PrivateKeyPath != "" {
		privateKey, keyType, err = LoadPrivateKeyFromFile(config.PrivateKeyPath, config.Password)
	} else {
		return nil, fmt.Errorf("private key is required (provide PrivateKeyPEM or PrivateKeyPath)")
	}

	if err != nil {
		return nil, fmt.Errorf("failed to load private key: %w", err)
	}

	publicKey, err := GetPublicKeyFromPrivate(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	// Set defaults
	if config.HashAlgorithm == "" {
		config.HashAlgorithm = HashSHA256
	}
	if config.Format == "" {
		config.Format = FormatBase64
	}

	return &KeySigner{
		privateKey: privateKey,
		publicKey:  publicKey,
		keyType:    keyType,
		config:     config,
	}, nil
}

// Sign creates a signature for the given data.
func (s *KeySigner) Sign(ctx context.Context, data []byte) (*SignatureResult, error) {
	// Calculate hash
	hasher := s.newHasher()
	hasher.Write(data)
	hash := hasher.Sum(nil)

	// Create signature
	signature, err := s.signHash(hash)
	if err != nil {
		return nil, fmt.Errorf("failed to sign: %w", err)
	}

	// Get public key for identity
	pubKeyPEM, err := s.PublicKey()
	if err != nil {
		return nil, fmt.Errorf("failed to get public key: %w", err)
	}

	result := &SignatureResult{
		Signature:       signature,
		SignatureBase64: base64.StdEncoding.EncodeToString(signature),
		Digest:          fmt.Sprintf("%s:%s", s.config.HashAlgorithm, base64.StdEncoding.EncodeToString(hash)),
		DigestAlgorithm: s.config.HashAlgorithm,
		Timestamp:       time.Now().UTC(),
		SignerIdentity:  KeyFingerprint(pubKeyPEM),
		Annotations:     s.config.Annotations,
	}

	// Create bundle if requested
	if s.config.Format == FormatBundle {
		result.Bundle = s.createBundle(data, hash, signature)
	}

	return result, nil
}

// SignFile creates a signature for the contents of a file.
func (s *KeySigner) SignFile(ctx context.Context, path string) (*SignatureResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %w", err)
	}
	return s.Sign(ctx, data)
}

// KeyType returns the type of key used for signing.
func (s *KeySigner) KeyType() KeyType {
	return s.keyType
}

// PublicKey returns the public key in PEM format.
func (s *KeySigner) PublicKey() ([]byte, error) {
	return EncodePublicKey(s.publicKey)
}

// newHasher creates a new hash function based on the configured algorithm.
func (s *KeySigner) newHasher() hash.Hash {
	switch s.config.HashAlgorithm {
	case HashSHA384:
		return sha512.New384()
	case HashSHA512:
		return sha512.New()
	default:
		return sha256.New()
	}
}

// cryptoHash returns the crypto.Hash for the configured algorithm.
func (s *KeySigner) cryptoHash() crypto.Hash {
	switch s.config.HashAlgorithm {
	case HashSHA384:
		return crypto.SHA384
	case HashSHA512:
		return crypto.SHA512
	default:
		return crypto.SHA256
	}
}

// signHash signs a hash using the private key.
func (s *KeySigner) signHash(hash []byte) ([]byte, error) {
	switch key := s.privateKey.(type) {
	case *ecdsa.PrivateKey:
		return ecdsa.SignASN1(rand.Reader, key, hash)

	case *rsa.PrivateKey:
		return rsa.SignPKCS1v15(rand.Reader, key, s.cryptoHash(), hash)

	case ed25519.PrivateKey:
		// Ed25519 signs the message directly, but we sign the hash for consistency
		return ed25519.Sign(key, hash), nil

	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedKeyType, key)
	}
}

// createBundle creates a signature bundle.
func (s *KeySigner) createBundle(data, hash, signature []byte) *SignatureBundle {
	payload := map[string]interface{}{
		"critical": map[string]interface{}{
			"type": "generic signature",
			"digest": map[string]string{
				string(s.config.HashAlgorithm): fmt.Sprintf("%x", hash),
			},
		},
		"optional": s.config.Annotations,
	}

	payloadBytes, _ := json.Marshal(payload)

	return &SignatureBundle{
		MediaType:   "application/vnd.kscore.signature.v1+json",
		PayloadType: "application/vnd.kscore.payload.v1+json",
		Payload:     base64.StdEncoding.EncodeToString(payloadBytes),
		Signatures: []BundleSignature{
			{
				KeyID: KeyFingerprint(must(s.PublicKey())),
				Sig:   base64.StdEncoding.EncodeToString(signature),
			},
		},
	}
}

// must is a helper that panics on error (used only for infallible operations).
func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

// SignatureWriter provides a convenient way to sign and write signatures.
type SignatureWriter struct {
	signer Signer
}

// NewSignatureWriter creates a new signature writer.
func NewSignatureWriter(signer Signer) *SignatureWriter {
	return &SignatureWriter{signer: signer}
}

// SignToFile signs data and writes the signature to a file.
func (w *SignatureWriter) SignToFile(ctx context.Context, data []byte, signaturePath string) error {
	result, err := w.signer.Sign(ctx, data)
	if err != nil {
		return err
	}

	return os.WriteFile(signaturePath, []byte(result.SignatureBase64), 0644)
}

// SignFileToFile signs a file and writes the signature to another file.
func (w *SignatureWriter) SignFileToFile(ctx context.Context, dataPath, signaturePath string) error {
	result, err := w.signer.SignFile(ctx, dataPath)
	if err != nil {
		return err
	}

	return os.WriteFile(signaturePath, []byte(result.SignatureBase64), 0644)
}

// SignFileToBundleFile signs a file and writes a signature bundle.
func (w *SignatureWriter) SignFileToBundleFile(ctx context.Context, dataPath, bundlePath string) error {
	result, err := w.signer.SignFile(ctx, dataPath)
	if err != nil {
		return err
	}

	if result.Bundle == nil {
		// Create a simple bundle if not already present
		result.Bundle = &SignatureBundle{
			MediaType:   "application/vnd.kscore.signature.v1+json",
			PayloadType: "application/octet-stream",
			Signatures: []BundleSignature{
				{
					Sig: result.SignatureBase64,
				},
			},
		}
	}

	bundleBytes, err := json.MarshalIndent(result.Bundle, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal bundle: %w", err)
	}

	return os.WriteFile(bundlePath, bundleBytes, 0644)
}
