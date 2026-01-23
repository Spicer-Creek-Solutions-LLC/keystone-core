package signing

import (
	"bytes"
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
	"strings"
)

// GenerateKeyPair generates a new key pair of the specified type.
// For RSA, bits specifies the key size (minimum 2048).
// For ECDSA, bits selects the curve (256=P-256, 384=P-384, 521=P-521).
// For Ed25519, bits is ignored.
func GenerateKeyPair(keyType KeyType, bits int) (*KeyPair, error) {
	var priv crypto.PrivateKey
	var pub crypto.PublicKey

	switch keyType {
	case KeyTypeECDSA:
		curve := elliptic.P256()
		if bits >= 521 {
			curve = elliptic.P521()
		} else if bits >= 384 {
			curve = elliptic.P384()
		}
		key, err := ecdsa.GenerateKey(curve, rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate ECDSA key: %w", err)
		}
		priv = key
		pub = &key.PublicKey

	case KeyTypeRSA:
		if bits < 2048 {
			bits = 2048
		}
		key, err := rsa.GenerateKey(rand.Reader, bits)
		if err != nil {
			return nil, fmt.Errorf("failed to generate RSA key: %w", err)
		}
		priv = key
		pub = &key.PublicKey

	case KeyTypeEd25519:
		pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
		if err != nil {
			return nil, fmt.Errorf("failed to generate Ed25519 key: %w", err)
		}
		priv = privKey
		pub = pubKey

	default:
		return nil, fmt.Errorf("%w: %s", ErrUnsupportedKeyType, keyType)
	}

	// Marshal private key to PKCS8
	privBytes, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	privPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: privBytes,
	})

	// Marshal public key to PKIX
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	pubPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	})

	return &KeyPair{
		PrivateKey: privPEM,
		PublicKey:  pubPEM,
		Type:       keyType,
	}, nil
}

// LoadPrivateKey loads a private key from PEM data.
// If the key is encrypted, password must be provided.
func LoadPrivateKey(pemData []byte, password string) (crypto.PrivateKey, KeyType, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, "", fmt.Errorf("%w: failed to decode PEM block", ErrInvalidKey)
	}

	var keyBytes []byte
	var err error

	if x509.IsEncryptedPEMBlock(block) { //nolint:staticcheck
		if password == "" {
			return nil, "", ErrKeyEncrypted
		}
		keyBytes, err = x509.DecryptPEMBlock(block, []byte(password)) //nolint:staticcheck
		if err != nil {
			return nil, "", fmt.Errorf("%w: failed to decrypt key: %v", ErrInvalidKey, err)
		}
	} else {
		keyBytes = block.Bytes
	}

	// Try to parse based on PEM type
	switch block.Type {
	case "EC PRIVATE KEY":
		key, err := x509.ParseECPrivateKey(keyBytes)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidKey, err)
		}
		return key, KeyTypeECDSA, nil

	case "RSA PRIVATE KEY":
		key, err := x509.ParsePKCS1PrivateKey(keyBytes)
		if err != nil {
			return nil, "", fmt.Errorf("%w: %v", ErrInvalidKey, err)
		}
		return key, KeyTypeRSA, nil

	case "PRIVATE KEY":
		return parsePKCS8PrivateKey(keyBytes)

	case "ED25519 PRIVATE KEY":
		if len(keyBytes) == ed25519.PrivateKeySize {
			return ed25519.PrivateKey(keyBytes), KeyTypeEd25519, nil
		}
		// Try PKCS8 format
		return parsePKCS8PrivateKey(keyBytes)

	default:
		// Try PKCS8 as fallback
		return parsePKCS8PrivateKey(keyBytes)
	}
}

// parsePKCS8PrivateKey parses a PKCS8 private key and returns the key type.
func parsePKCS8PrivateKey(keyBytes []byte) (crypto.PrivateKey, KeyType, error) {
	key, err := x509.ParsePKCS8PrivateKey(keyBytes)
	if err != nil {
		return nil, "", fmt.Errorf("%w: %v", ErrInvalidKey, err)
	}

	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		return k, KeyTypeECDSA, nil
	case *rsa.PrivateKey:
		return k, KeyTypeRSA, nil
	case ed25519.PrivateKey:
		return k, KeyTypeEd25519, nil
	default:
		return nil, "", fmt.Errorf("%w: %T", ErrUnsupportedKeyType, key)
	}
}

// LoadPrivateKeyFromFile loads a private key from a file.
func LoadPrivateKeyFromFile(path, password string) (crypto.PrivateKey, KeyType, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read key file: %w", err)
	}
	return LoadPrivateKey(data, password)
}

// LoadPublicKey loads a public key from PEM data.
func LoadPublicKey(pemData []byte) (crypto.PublicKey, KeyType, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, "", fmt.Errorf("%w: failed to decode PEM block", ErrInvalidKey)
	}

	// Try PKIX format first
	if pubKey, err := x509.ParsePKIXPublicKey(block.Bytes); err == nil {
		return publicKeyWithType(pubKey)
	}

	// Try certificate
	if cert, err := x509.ParseCertificate(block.Bytes); err == nil {
		return publicKeyWithType(cert.PublicKey)
	}

	// Try PKCS1 RSA
	if rsaKey, err := x509.ParsePKCS1PublicKey(block.Bytes); err == nil {
		return rsaKey, KeyTypeRSA, nil
	}

	return nil, "", fmt.Errorf("%w: unsupported public key format", ErrInvalidKey)
}

// publicKeyWithType returns the public key and its type.
func publicKeyWithType(pub crypto.PublicKey) (crypto.PublicKey, KeyType, error) {
	switch pub.(type) {
	case *ecdsa.PublicKey:
		return pub, KeyTypeECDSA, nil
	case *rsa.PublicKey:
		return pub, KeyTypeRSA, nil
	case ed25519.PublicKey:
		return pub, KeyTypeEd25519, nil
	default:
		return nil, "", fmt.Errorf("%w: %T", ErrUnsupportedKeyType, pub)
	}
}

// LoadPublicKeyFromFile loads a public key from a file.
func LoadPublicKeyFromFile(path string) (crypto.PublicKey, KeyType, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read key file: %w", err)
	}
	return LoadPublicKey(data)
}

// EncodePublicKey encodes a public key to PEM format.
func EncodePublicKey(pub crypto.PublicKey) ([]byte, error) {
	pubBytes, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	return pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: pubBytes,
	}), nil
}

// EncryptPrivateKey encrypts a PEM-encoded private key with a password.
func EncryptPrivateKey(privateKeyPEM []byte, password string) ([]byte, error) {
	block, _ := pem.Decode(privateKeyPEM)
	if block == nil {
		return nil, fmt.Errorf("%w: failed to decode PEM block", ErrInvalidKey)
	}

	//nolint:staticcheck
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

// KeyFingerprint returns the SHA-256 fingerprint of a public key.
func KeyFingerprint(publicKeyPEM []byte) string {
	block, _ := pem.Decode(publicKeyPEM)
	if block == nil {
		return ""
	}

	hash := sha256.Sum256(block.Bytes)
	return fmt.Sprintf("sha256:%s", formatFingerprint(hash[:]))
}

// formatFingerprint formats bytes as a colon-separated hex string.
func formatFingerprint(data []byte) string {
	var parts []string
	for _, b := range data[:8] { // Use first 8 bytes for short fingerprint
		parts = append(parts, fmt.Sprintf("%02x", b))
	}
	return strings.Join(parts, ":")
}

// GetPublicKeyFromPrivate extracts the public key from a private key.
func GetPublicKeyFromPrivate(priv crypto.PrivateKey) (crypto.PublicKey, error) {
	switch k := priv.(type) {
	case *ecdsa.PrivateKey:
		return &k.PublicKey, nil
	case *rsa.PrivateKey:
		return &k.PublicKey, nil
	case ed25519.PrivateKey:
		return k.Public(), nil
	default:
		return nil, fmt.Errorf("%w: %T", ErrUnsupportedKeyType, priv)
	}
}
