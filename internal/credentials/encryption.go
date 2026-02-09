// Package credentials provides secure credential management for proxy agents.
package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// Encryptor provides encryption and decryption operations.
type Encryptor interface {
	// Encrypt encrypts data.
	Encrypt(plaintext []byte) ([]byte, error)
	// Decrypt decrypts data.
	Decrypt(ciphertext []byte) ([]byte, error)
}

// AESEncryptor encrypts data using AES-GCM.
type AESEncryptor struct {
	key []byte
}

// NewAESEncryptor creates a new AES encryptor with the given key.
// The key must be 16, 24, or 32 bytes for AES-128, AES-192, or AES-256.
func NewAESEncryptor(key []byte) (*AESEncryptor, error) {
	switch len(key) {
	case 16, 24, 32:
		// Valid key sizes
	default:
		return nil, fmt.Errorf("invalid key size: %d bytes (must be 16, 24, or 32)", len(key))
	}
	return &AESEncryptor{key: key}, nil
}

// Encrypt encrypts data using AES-GCM.
func (e *AESEncryptor) Encrypt(plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and prepend nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt decrypts data using AES-GCM.
func (e *AESEncryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(e.key)
	if err != nil {
		return nil, fmt.Errorf("failed to create cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	// Extract nonce and decrypt
	nonce := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// Ensure AESEncryptor implements Encryptor.
var _ Encryptor = (*AESEncryptor)(nil)

// X25519KeyPair represents a Curve25519 key pair for key exchange.
type X25519KeyPair struct {
	PublicKey  [32]byte
	PrivateKey [32]byte
}

// GenerateX25519KeyPair generates a new X25519 key pair.
func GenerateX25519KeyPair() (*X25519KeyPair, error) {
	var privateKey [32]byte
	if _, err := io.ReadFull(rand.Reader, privateKey[:]); err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	var publicKey [32]byte
	curve25519.ScalarBaseMult(&publicKey, &privateKey)

	return &X25519KeyPair{
		PublicKey:  publicKey,
		PrivateKey: privateKey,
	}, nil
}

// ComputeSharedSecret computes a shared secret using X25519 ECDH.
func (kp *X25519KeyPair) ComputeSharedSecret(peerPublicKey [32]byte) ([32]byte, error) {
	var sharedSecret [32]byte
	out, err := curve25519.X25519(kp.PrivateKey[:], peerPublicKey[:])
	if err != nil {
		return sharedSecret, fmt.Errorf("failed to compute shared secret: %w", err)
	}
	copy(sharedSecret[:], out)
	return sharedSecret, nil
}

// X25519Encryptor uses X25519 key exchange for secure credential transport.
type X25519Encryptor struct {
	localKeyPair  *X25519KeyPair
	peerPublicKey [32]byte
	sharedKey     []byte
}

// NewX25519Encryptor creates a new X25519-based encryptor.
// It uses the local key pair and peer public key to derive a shared encryption key.
func NewX25519Encryptor(localKeyPair *X25519KeyPair, peerPublicKey [32]byte) (*X25519Encryptor, error) {
	// Compute shared secret
	sharedSecret, err := localKeyPair.ComputeSharedSecret(peerPublicKey)
	if err != nil {
		return nil, err
	}

	// Derive encryption key using HKDF
	hkdfReader := hkdf.New(sha256.New, sharedSecret[:], nil, []byte("keystone-core-credential-encryption"))
	derivedKey := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(hkdfReader, derivedKey); err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	return &X25519Encryptor{
		localKeyPair:  localKeyPair,
		peerPublicKey: peerPublicKey,
		sharedKey:     derivedKey,
	}, nil
}

// Encrypt encrypts data using the derived shared key.
func (e *X25519Encryptor) Encrypt(plaintext []byte) ([]byte, error) {
	aesEnc, err := NewAESEncryptor(e.sharedKey)
	if err != nil {
		return nil, err
	}
	return aesEnc.Encrypt(plaintext)
}

// Decrypt decrypts data using the derived shared key.
func (e *X25519Encryptor) Decrypt(ciphertext []byte) ([]byte, error) {
	aesEnc, err := NewAESEncryptor(e.sharedKey)
	if err != nil {
		return nil, err
	}
	return aesEnc.Decrypt(ciphertext)
}

// GetPublicKey returns the local public key for sharing.
func (e *X25519Encryptor) GetPublicKey() [32]byte {
	return e.localKeyPair.PublicKey
}

// Ensure X25519Encryptor implements Encryptor.
var _ Encryptor = (*X25519Encryptor)(nil)

// EncryptedCredentialEnvelope wraps encrypted credential data for transport.
type EncryptedCredentialEnvelope struct {
	// SenderPublicKey is the sender's X25519 public key.
	SenderPublicKey [32]byte `json:"sender_public_key"`
	// EncryptedData is the AES-GCM encrypted credential.
	EncryptedData []byte `json:"encrypted_data"`
	// CredentialType is the type of the encrypted credential.
	CredentialType CredentialType `json:"credential_type"`
}

// CredentialEncryptor handles end-to-end encryption of credentials.
type CredentialEncryptor struct {
	localKeyPair *X25519KeyPair
}

// NewCredentialEncryptor creates a new credential encryptor.
func NewCredentialEncryptor() (*CredentialEncryptor, error) {
	keyPair, err := GenerateX25519KeyPair()
	if err != nil {
		return nil, err
	}
	return &CredentialEncryptor{localKeyPair: keyPair}, nil
}

// NewCredentialEncryptorWithKeyPair creates a credential encryptor with an existing key pair.
func NewCredentialEncryptorWithKeyPair(keyPair *X25519KeyPair) *CredentialEncryptor {
	return &CredentialEncryptor{localKeyPair: keyPair}
}

// GetPublicKey returns the public key for sharing with peers.
func (ce *CredentialEncryptor) GetPublicKey() [32]byte {
	return ce.localKeyPair.PublicKey
}

// EncryptForPeer encrypts credential data for a specific peer.
func (ce *CredentialEncryptor) EncryptForPeer(credType CredentialType, data []byte, peerPublicKey [32]byte) (*EncryptedCredentialEnvelope, error) {
	encryptor, err := NewX25519Encryptor(ce.localKeyPair, peerPublicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKeyExchangeFailed, err)
	}

	encrypted, err := encryptor.Encrypt(data)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrEncryptionFailed, err)
	}

	return &EncryptedCredentialEnvelope{
		SenderPublicKey: ce.localKeyPair.PublicKey,
		EncryptedData:   encrypted,
		CredentialType:  credType,
	}, nil
}

// DecryptFromPeer decrypts credential data from a peer.
func (ce *CredentialEncryptor) DecryptFromPeer(envelope *EncryptedCredentialEnvelope) ([]byte, error) {
	encryptor, err := NewX25519Encryptor(ce.localKeyPair, envelope.SenderPublicKey)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrKeyExchangeFailed, err)
	}

	decrypted, err := encryptor.Decrypt(envelope.EncryptedData)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrDecryptionFailed, err)
	}

	return decrypted, nil
}

// DeriveKeyFromPassword derives an encryption key from a password using HKDF.
func DeriveKeyFromPassword(password string, salt []byte) ([]byte, error) {
	if len(salt) == 0 {
		salt = make([]byte, 32)
		if _, err := io.ReadFull(rand.Reader, salt); err != nil {
			return nil, fmt.Errorf("failed to generate salt: %w", err)
		}
	}

	// Use HKDF to derive a key
	hkdfReader := hkdf.New(sha256.New, []byte(password), salt, []byte("keystone-core-credential-key"))
	key := make([]byte, 32) // AES-256
	if _, err := io.ReadFull(hkdfReader, key); err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	return key, nil
}

// GenerateRandomKey generates a random encryption key.
func GenerateRandomKey(size int) ([]byte, error) {
	key := make([]byte, size)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate random key: %w", err)
	}
	return key, nil
}
