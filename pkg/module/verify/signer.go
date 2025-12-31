package verify

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// Signer signs module files
type Signer interface {
	// Sign creates a signature for the given module file
	Sign(modulePath string, privateKey []byte) ([]byte, error)
	// SignToFile creates a signature and writes it to a file
	SignToFile(modulePath, outputPath string, privateKey []byte) error
}

// DefaultSigner implements module signing
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
	// Read module data
	moduleData, err := os.ReadFile(modulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read module: %w", err)
	}

	// Parse private key
	privKey, err := parsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	// Compute hash
	hash := sha256.Sum256(moduleData)

	// Sign based on key type
	switch key := privKey.(type) {
	case *rsa.PrivateKey:
		return signRSA(key, hash[:])
	case *ecdsa.PrivateKey:
		return signECDSA(key, hash[:])
	case ed25519.PrivateKey:
		return signEd25519(key, hash[:])
	default:
		return nil, fmt.Errorf("unsupported key type: %T", privKey)
	}
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

// signRSA signs data using RSA PKCS1v15
func signRSA(key *rsa.PrivateKey, hash []byte) ([]byte, error) {
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash)
	if err != nil {
		return nil, fmt.Errorf("RSA signing failed: %w", err)
	}
	return signature, nil
}

// signECDSA signs data using ECDSA
func signECDSA(key *ecdsa.PrivateKey, hash []byte) ([]byte, error) {
	signature, err := ecdsa.SignASN1(rand.Reader, key, hash)
	if err != nil {
		return nil, fmt.Errorf("ECDSA signing failed: %w", err)
	}
	return signature, nil
}

// signEd25519 signs data using Ed25519
func signEd25519(key ed25519.PrivateKey, hash []byte) ([]byte, error) {
	// Ed25519 signs the message directly, not the hash
	// But for consistency with verification, we sign the hash
	signature := ed25519.Sign(key, hash)
	return signature, nil
}

// parsePrivateKey parses a PEM-encoded private key
func parsePrivateKey(pemData []byte) (interface{}, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	// Try parsing as PKCS8 (generic)
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try parsing as PKCS1 RSA private key
	if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try parsing as EC private key
	if key, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
		return key, nil
	}

	// Try parsing as Ed25519 (raw 64-byte seed in PEM)
	if len(block.Bytes) == ed25519.SeedSize {
		return ed25519.NewKeyFromSeed(block.Bytes), nil
	}

	// Try as 64-byte Ed25519 private key
	if len(block.Bytes) == ed25519.PrivateKeySize {
		return ed25519.PrivateKey(block.Bytes), nil
	}

	return nil, fmt.Errorf("unsupported private key format (type: %s)", block.Type)
}

// GenerateKeyPair generates a new key pair for signing
// This is a convenience function for testing and development
func GenerateKeyPair(keyType string, bits int) (privateKeyPEM, publicKeyPEM []byte, err error) {
	switch keyType {
	case "rsa":
		return generateRSAKeyPair(bits)
	case "ecdsa":
		return generateECDSAKeyPair()
	case "ed25519":
		return generateEd25519KeyPair()
	default:
		return nil, nil, fmt.Errorf("unsupported key type: %s (use rsa, ecdsa, or ed25519)", keyType)
	}
}

func generateRSAKeyPair(bits int) (privateKeyPEM, publicKeyPEM []byte, err error) {
	if bits < 2048 {
		bits = 2048
	}

	privateKey, err := rsa.GenerateKey(rand.Reader, bits)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate RSA key: %w", err)
	}

	// Encode private key
	privateKeyBytes := x509.MarshalPKCS1PrivateKey(privateKey)
	privateKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Encode public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	publicKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return privateKeyPEM, publicKeyPEM, nil
}

func generateECDSAKeyPair() (privateKeyPEM, publicKeyPEM []byte, err error) {
	// Use P-256 curve by default
	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
	}

	// Encode private key
	privateKeyBytes, err := x509.MarshalECPrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	privateKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "EC PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Encode public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	publicKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return privateKeyPEM, publicKeyPEM, nil
}

func generateEd25519KeyPair() (privateKeyPEM, publicKeyPEM []byte, err error) {
	publicKey, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate Ed25519 key: %w", err)
	}

	// Encode private key using PKCS8
	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}
	privateKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	// Encode public key
	publicKeyBytes, err := x509.MarshalPKIXPublicKey(publicKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal public key: %w", err)
	}
	publicKeyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return privateKeyPEM, publicKeyPEM, nil
}
