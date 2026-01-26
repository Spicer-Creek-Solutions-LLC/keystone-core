package verify

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/shawnbutts/keystone-core/internal/signing"
)

// Signer signs module files
type Signer interface {
	// Sign creates a signature for the given module file
	Sign(modulePath string, privateKey []byte) ([]byte, error)
	// SignToFile creates a signature and writes it to a file
	SignToFile(modulePath, outputPath string, privateKey []byte) error
}

// DefaultSigner implements module signing using the shared signing package
type DefaultSigner struct {
	algorithm HashAlgorithm
}

// NewSigner creates a new module signer
func NewSigner() *DefaultSigner {
	return &DefaultSigner{
		algorithm: SHA256,
	}
}

// NewSignerWithAlgorithm creates a signer with a specific hash algorithm
func NewSignerWithAlgorithm(alg HashAlgorithm) *DefaultSigner {
	return &DefaultSigner{
		algorithm: alg,
	}
}

// Sign creates a signature for the given module file
func (s *DefaultSigner) Sign(modulePath string, privateKeyPEM []byte) ([]byte, error) {
	hashAlg := signing.HashSHA256
	if s.algorithm == SHA512 {
		hashAlg = signing.HashSHA512
	}

	signer, err := signing.NewKeySigner(&signing.KeySignerConfig{
		PrivateKeyPEM: privateKeyPEM,
		HashAlgorithm: hashAlg,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create signer: %w", err)
	}

	result, err := signer.SignFile(context.Background(), modulePath)
	if err != nil {
		return nil, err
	}

	return result.Signature, nil
}

// SignToFile creates a signature and writes it to a file
func (s *DefaultSigner) SignToFile(modulePath, outputPath string, privateKeyPEM []byte) error {
	signature, err := s.Sign(modulePath, privateKeyPEM)
	if err != nil {
		return err
	}

	// Create output directory if needed
	if dir := filepath.Dir(outputPath); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	// Write signature to file
	if err := os.WriteFile(outputPath, signature, 0644); err != nil {
		return fmt.Errorf("failed to write signature: %w", err)
	}

	return nil
}

// GenerateKeyPair generates a new key pair for signing
// This is a convenience function for testing and development
func GenerateKeyPair(keyType string, bits int) (privateKeyPEM, publicKeyPEM []byte, err error) {
	var kt signing.KeyType
	switch keyType {
	case "rsa":
		kt = signing.KeyTypeRSA
	case "ecdsa":
		kt = signing.KeyTypeECDSA
	case "ed25519":
		kt = signing.KeyTypeEd25519
	default:
		return nil, nil, fmt.Errorf("unsupported key type: %s (use rsa, ecdsa, or ed25519)", keyType)
	}

	keyPair, err := signing.GenerateKeyPair(kt, bits)
	if err != nil {
		return nil, nil, err
	}

	return keyPair.PrivateKey, keyPair.PublicKey, nil
}
