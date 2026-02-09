package identity

import (
	"bytes"
	"crypto"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// KeyProtectionConfig configures how CA private keys are protected.
type KeyProtectionConfig struct {
	// Method is the key protection method.
	// Options: "plaintext", "encrypted", "hsm"
	Method string

	// EncryptionKey is the key encryption key (KEK) for "encrypted" method.
	// Can be set via environment variable or loaded from a file.
	EncryptionKey []byte

	// EncryptionKeyEnvVar is the environment variable containing the KEK.
	EncryptionKeyEnvVar string

	// EncryptionKeyFile is the path to a file containing the KEK.
	EncryptionKeyFile string

	// HSMConfig is the configuration for HSM key protection.
	HSMConfig *HSMConfig
}

// HSMConfig configures Hardware Security Module integration.
type HSMConfig struct {
	// ModulePath is the path to the PKCS#11 module (.so file).
	ModulePath string

	// SlotID is the HSM slot ID.
	SlotID uint

	// PIN is the HSM PIN for authentication.
	PIN string

	// TokenLabel is the label of the token to use.
	TokenLabel string

	// KeyLabel is the label for the CA key in the HSM.
	KeyLabel string
}

// KeyProtector handles secure storage of CA private keys.
type KeyProtector struct {
	config *KeyProtectionConfig
	kek    []byte // Key encryption key
	mu     sync.RWMutex
}

// NewKeyProtector creates a new key protector with the given configuration.
func NewKeyProtector(config *KeyProtectionConfig) (*KeyProtector, error) {
	if config == nil {
		config = &KeyProtectionConfig{Method: "plaintext"}
	}

	kp := &KeyProtector{
		config: config,
	}

	// Load KEK for encrypted method
	if config.Method == "encrypted" {
		kek, err := kp.loadKEK()
		if err != nil {
			return nil, fmt.Errorf("failed to load key encryption key: %w", err)
		}
		kp.kek = kek
	}

	return kp, nil
}

// loadKEK loads the key encryption key from configured sources.
func (kp *KeyProtector) loadKEK() ([]byte, error) {
	// Try direct configuration first
	if len(kp.config.EncryptionKey) > 0 {
		return kp.config.EncryptionKey, nil
	}

	// Try environment variable
	if kp.config.EncryptionKeyEnvVar != "" {
		keyStr := os.Getenv(kp.config.EncryptionKeyEnvVar)
		if keyStr != "" {
			// Decode base64-encoded key
			key, err := base64.StdEncoding.DecodeString(keyStr)
			if err != nil {
				return nil, fmt.Errorf("invalid base64 key from env var: %w", err)
			}
			return key, nil
		}
	}

	// Try key file
	if kp.config.EncryptionKeyFile != "" {
		keyData, err := os.ReadFile(kp.config.EncryptionKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}
		// Decode base64-encoded key
		key, err := base64.StdEncoding.DecodeString(string(bytes.TrimSpace(keyData)))
		if err != nil {
			return nil, fmt.Errorf("invalid base64 key from file: %w", err)
		}
		return key, nil
	}

	return nil, fmt.Errorf("no key encryption key configured")
}

// ProtectKey encrypts a private key for storage.
func (kp *KeyProtector) ProtectKey(key crypto.PrivateKey) ([]byte, error) {
	kp.mu.RLock()
	defer kp.mu.RUnlock()

	// Serialize the private key to PKCS#8 DER format
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	switch kp.config.Method {
	case "plaintext", "":
		// Return as PEM without encryption
		pemBlock := &pem.Block{
			Type:  "PRIVATE KEY",
			Bytes: keyDER,
		}
		return pem.EncodeToMemory(pemBlock), nil

	case "encrypted":
		// Encrypt with AES-256-GCM
		encrypted, err := kp.encryptAESGCM(keyDER)
		if err != nil {
			return nil, fmt.Errorf("failed to encrypt key: %w", err)
		}
		pemBlock := &pem.Block{
			Type:  "ENCRYPTED PRIVATE KEY",
			Bytes: encrypted,
			Headers: map[string]string{
				"Encryption": "AES-256-GCM",
			},
		}
		return pem.EncodeToMemory(pemBlock), nil

	case "hsm":
		// HSM stores key internally, return key handle
		return nil, fmt.Errorf("HSM key protection not yet implemented")

	default:
		return nil, fmt.Errorf("unknown key protection method: %s", kp.config.Method)
	}
}

// UnprotectKey decrypts a protected private key.
func (kp *KeyProtector) UnprotectKey(protectedKey []byte) (crypto.PrivateKey, error) {
	kp.mu.RLock()
	defer kp.mu.RUnlock()

	block, _ := pem.Decode(protectedKey)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	var keyDER []byte

	switch block.Type {
	case "PRIVATE KEY":
		// Plaintext key
		keyDER = block.Bytes

	case "ENCRYPTED PRIVATE KEY":
		// Check encryption type
		if block.Headers["Encryption"] != "AES-256-GCM" {
			return nil, fmt.Errorf("unsupported encryption: %s", block.Headers["Encryption"])
		}
		var err error
		keyDER, err = kp.decryptAESGCM(block.Bytes)
		if err != nil {
			return nil, fmt.Errorf("failed to decrypt key: %w", err)
		}

	default:
		return nil, fmt.Errorf("unexpected PEM block type: %s", block.Type)
	}

	// Parse the PKCS#8 private key
	key, err := x509.ParsePKCS8PrivateKey(keyDER)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return key, nil
}

// encryptAESGCM encrypts data using AES-256-GCM.
func (kp *KeyProtector) encryptAESGCM(plaintext []byte) ([]byte, error) {
	if len(kp.kek) != 32 {
		return nil, fmt.Errorf("KEK must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(kp.kek)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	// Prepend nonce to ciphertext
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// decryptAESGCM decrypts data using AES-256-GCM.
func (kp *KeyProtector) decryptAESGCM(ciphertext []byte) ([]byte, error) {
	if len(kp.kek) != 32 {
		return nil, fmt.Errorf("KEK must be 32 bytes for AES-256")
	}

	block, err := aes.NewCipher(kp.kek)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	if len(ciphertext) < gcm.NonceSize() {
		return nil, fmt.Errorf("ciphertext too short")
	}

	nonce := ciphertext[:gcm.NonceSize()]
	ciphertext = ciphertext[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return nil, fmt.Errorf("decryption failed: %w", err)
	}

	return plaintext, nil
}

// GenerateKEK generates a new key encryption key.
func GenerateKEK() ([]byte, error) {
	key := make([]byte, 32) // 256 bits for AES-256
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, fmt.Errorf("failed to generate KEK: %w", err)
	}
	return key, nil
}

// CARotationConfig configures CA rotation behavior.
type CARotationConfig struct {
	// RotationThreshold is when to start rotation (as fraction of CA lifetime).
	// Default: 0.7 (rotate when 70% of lifetime has passed)
	RotationThreshold float64

	// OverlapDuration is how long old and new CA overlap.
	// Default: 24h
	OverlapDuration time.Duration

	// DualSigningEnabled enables signing with both old and new CA during rotation.
	DualSigningEnabled bool

	// AutoRotate enables automatic CA rotation.
	AutoRotate bool
}

// DefaultCARotationConfig returns the default CA rotation configuration.
func DefaultCARotationConfig() *CARotationConfig {
	return &CARotationConfig{
		RotationThreshold:  0.7,
		OverlapDuration:    24 * time.Hour,
		DualSigningEnabled: true,
		AutoRotate:         true,
	}
}

// CARotationState tracks the state of CA rotation.
type CARotationState struct {
	// InProgress indicates if rotation is currently happening.
	InProgress bool `json:"in_progress"`

	// StartedAt is when the rotation started.
	StartedAt time.Time `json:"started_at,omitempty"`

	// CurrentCAID is the ID of the current active CA.
	CurrentCAID string `json:"current_ca_id"`

	// NextCAID is the ID of the next CA (during rotation).
	NextCAID string `json:"next_ca_id,omitempty"`

	// SwitchoverAt is when to complete the switchover.
	SwitchoverAt time.Time `json:"switchover_at,omitempty"`
}

// CARotationManager handles CA rotation.
type CARotationManager struct {
	config *CARotationConfig
	state  *CARotationState
	mu     sync.RWMutex

	// Callbacks
	onRotationStarted   func(oldCAID, newCAID string)
	onRotationCompleted func(newCAID string)
}

// NewCARotationManager creates a new CA rotation manager.
func NewCARotationManager(config *CARotationConfig) *CARotationManager {
	if config == nil {
		config = DefaultCARotationConfig()
	}

	return &CARotationManager{
		config: config,
		state: &CARotationState{
			InProgress: false,
		},
	}
}

// ShouldRotate checks if the CA should be rotated.
func (rm *CARotationManager) ShouldRotate(caNotBefore, caNotAfter time.Time) bool {
	if !rm.config.AutoRotate {
		return false
	}

	rm.mu.RLock()
	defer rm.mu.RUnlock()

	if rm.state.InProgress {
		return false
	}

	lifetime := caNotAfter.Sub(caNotBefore)
	elapsed := time.Since(caNotBefore)
	elapsedFraction := float64(elapsed) / float64(lifetime)

	return elapsedFraction >= rm.config.RotationThreshold
}

// StartRotation begins the CA rotation process.
func (rm *CARotationManager) StartRotation(currentCAID, newCAID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if rm.state.InProgress {
		return fmt.Errorf("rotation already in progress")
	}

	rm.state.InProgress = true
	rm.state.StartedAt = time.Now()
	rm.state.CurrentCAID = currentCAID
	rm.state.NextCAID = newCAID
	rm.state.SwitchoverAt = time.Now().Add(rm.config.OverlapDuration)

	if rm.onRotationStarted != nil {
		rm.onRotationStarted(currentCAID, newCAID)
	}

	return nil
}

// CompleteRotation finishes the CA rotation process.
func (rm *CARotationManager) CompleteRotation() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if !rm.state.InProgress {
		return fmt.Errorf("no rotation in progress")
	}

	newCAID := rm.state.NextCAID

	rm.state.InProgress = false
	rm.state.CurrentCAID = newCAID
	rm.state.NextCAID = ""
	rm.state.StartedAt = time.Time{}
	rm.state.SwitchoverAt = time.Time{}

	if rm.onRotationCompleted != nil {
		rm.onRotationCompleted(newCAID)
	}

	return nil
}

// GetState returns the current rotation state.
func (rm *CARotationManager) GetState() CARotationState {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return *rm.state
}

// OnRotationStarted sets the callback for when rotation starts.
func (rm *CARotationManager) OnRotationStarted(callback func(oldCAID, newCAID string)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.onRotationStarted = callback
}

// OnRotationCompleted sets the callback for when rotation completes.
func (rm *CARotationManager) OnRotationCompleted(callback func(newCAID string)) {
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.onRotationCompleted = callback
}

// IsDualSigningActive returns true if dual-signing should be used.
func (rm *CARotationManager) IsDualSigningActive() bool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.state.InProgress && rm.config.DualSigningEnabled
}

// CABackup represents a backup of CA data.
type CABackup struct {
	// Version is the backup format version.
	Version string `json:"version"`

	// CreatedAt is when the backup was created.
	CreatedAt time.Time `json:"created_at"`

	// TrustDomain is the trust domain of the CA.
	TrustDomain string `json:"trust_domain"`

	// RootCA is the root CA data.
	RootCA *CABackupData `json:"root_ca"`

	// SigningCA is the signing CA data.
	SigningCA *CABackupData `json:"signing_ca"`

	// RotationState is the current rotation state.
	RotationState *CARotationState `json:"rotation_state,omitempty"`

	// Checksum is the SHA-256 checksum of the backup data.
	Checksum string `json:"checksum"`
}

// CABackupData contains the backed up CA certificate and key.
type CABackupData struct {
	// Certificate is the PEM-encoded certificate.
	Certificate []byte `json:"certificate"`

	// PrivateKey is the PEM-encoded (possibly encrypted) private key.
	PrivateKey []byte `json:"private_key"`

	// NotBefore is when the certificate becomes valid.
	NotBefore time.Time `json:"not_before"`

	// NotAfter is when the certificate expires.
	NotAfter time.Time `json:"not_after"`

	// SerialNumber is the certificate serial number (hex).
	SerialNumber string `json:"serial_number"`
}

// CABackupManager handles CA backup and recovery operations.
type CABackupManager struct {
	keyProtector *KeyProtector
	backupDir    string
}

// NewCABackupManager creates a new CA backup manager.
func NewCABackupManager(backupDir string, keyProtector *KeyProtector) *CABackupManager {
	return &CABackupManager{
		keyProtector: keyProtector,
		backupDir:    backupDir,
	}
}

// CreateBackup creates a backup of the CA.
func (bm *CABackupManager) CreateBackup(
	trustDomain string,
	rootCert *x509.Certificate,
	rootKey crypto.PrivateKey,
	signingCert *x509.Certificate,
	signingKey crypto.PrivateKey,
	rotationState *CARotationState,
) (*CABackup, error) {
	// Protect keys
	protectedRootKey, err := bm.keyProtector.ProtectKey(rootKey)
	if err != nil {
		return nil, fmt.Errorf("failed to protect root key: %w", err)
	}

	protectedSigningKey, err := bm.keyProtector.ProtectKey(signingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to protect signing key: %w", err)
	}

	// Encode certificates
	rootCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: rootCert.Raw,
	})

	signingCertPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: signingCert.Raw,
	})

	backup := &CABackup{
		Version:     "1.0",
		CreatedAt:   time.Now().UTC(),
		TrustDomain: trustDomain,
		RootCA: &CABackupData{
			Certificate:  rootCertPEM,
			PrivateKey:   protectedRootKey,
			NotBefore:    rootCert.NotBefore,
			NotAfter:     rootCert.NotAfter,
			SerialNumber: rootCert.SerialNumber.Text(16),
		},
		SigningCA: &CABackupData{
			Certificate:  signingCertPEM,
			PrivateKey:   protectedSigningKey,
			NotBefore:    signingCert.NotBefore,
			NotAfter:     signingCert.NotAfter,
			SerialNumber: signingCert.SerialNumber.Text(16),
		},
		RotationState: rotationState,
	}

	// Calculate checksum
	backup.Checksum = bm.calculateChecksum(backup)

	return backup, nil
}

// SaveBackup saves a backup to disk.
func (bm *CABackupManager) SaveBackup(backup *CABackup) (string, error) {
	// Ensure backup directory exists
	if err := os.MkdirAll(bm.backupDir, 0o700); err != nil {
		return "", fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Generate backup filename
	filename := fmt.Sprintf("ca-backup-%s.json", backup.CreatedAt.Format("20060102-150405"))
	backupPath := filepath.Join(bm.backupDir, filename)

	// Serialize backup
	data, err := json.MarshalIndent(backup, "", "  ")
	if err != nil {
		return "", fmt.Errorf("failed to serialize backup: %w", err)
	}

	// Write backup file
	if err := os.WriteFile(backupPath, data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write backup file: %w", err)
	}

	return backupPath, nil
}

// LoadBackup loads a backup from disk.
func (bm *CABackupManager) LoadBackup(backupPath string) (*CABackup, error) {
	data, err := os.ReadFile(backupPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read backup file: %w", err)
	}

	var backup CABackup
	if err := json.Unmarshal(data, &backup); err != nil {
		return nil, fmt.Errorf("failed to parse backup: %w", err)
	}

	// Verify checksum
	expectedChecksum := backup.Checksum
	backup.Checksum = "" // Clear for recalculation
	actualChecksum := bm.calculateChecksum(&backup)
	backup.Checksum = expectedChecksum

	if expectedChecksum != actualChecksum {
		return nil, fmt.Errorf("backup checksum mismatch")
	}

	return &backup, nil
}

// RestoreCA restores CA from a backup.
func (bm *CABackupManager) RestoreCA(backup *CABackup) (
	rootCert *x509.Certificate,
	rootKey crypto.PrivateKey,
	signingCert *x509.Certificate,
	signingKey crypto.PrivateKey,
	err error,
) {
	// Parse root certificate
	rootCertBlock, _ := pem.Decode(backup.RootCA.Certificate)
	if rootCertBlock == nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to decode root certificate PEM")
	}
	rootCert, err = x509.ParseCertificate(rootCertBlock.Bytes)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to parse root certificate: %w", err)
	}

	// Unprotect root key
	rootKey, err = bm.keyProtector.UnprotectKey(backup.RootCA.PrivateKey)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to unprotect root key: %w", err)
	}

	// Parse signing certificate
	signingCertBlock, _ := pem.Decode(backup.SigningCA.Certificate)
	if signingCertBlock == nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to decode signing certificate PEM")
	}
	signingCert, err = x509.ParseCertificate(signingCertBlock.Bytes)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to parse signing certificate: %w", err)
	}

	// Unprotect signing key
	signingKey, err = bm.keyProtector.UnprotectKey(backup.SigningCA.PrivateKey)
	if err != nil {
		return nil, nil, nil, nil, fmt.Errorf("failed to unprotect signing key: %w", err)
	}

	// Verify key-certificate pairs
	if err := verifyKeyCertPair(rootCert, rootKey); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("root key-cert mismatch: %w", err)
	}
	if err := verifyKeyCertPair(signingCert, signingKey); err != nil {
		return nil, nil, nil, nil, fmt.Errorf("signing key-cert mismatch: %w", err)
	}

	return rootCert, rootKey, signingCert, signingKey, nil
}

// ListBackups lists available backups in the backup directory.
func (bm *CABackupManager) ListBackups() ([]string, error) {
	entries, err := os.ReadDir(bm.backupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("failed to list backup directory: %w", err)
	}

	var backups []string
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			backups = append(backups, filepath.Join(bm.backupDir, entry.Name()))
		}
	}

	return backups, nil
}

// calculateChecksum calculates a SHA-256 checksum of the backup data.
func (bm *CABackupManager) calculateChecksum(backup *CABackup) string {
	// Create deterministic representation for checksum
	data := fmt.Sprintf("%s|%s|%s|%s|%s|%s",
		backup.Version,
		backup.TrustDomain,
		base64.StdEncoding.EncodeToString(backup.RootCA.Certificate),
		base64.StdEncoding.EncodeToString(backup.RootCA.PrivateKey),
		base64.StdEncoding.EncodeToString(backup.SigningCA.Certificate),
		base64.StdEncoding.EncodeToString(backup.SigningCA.PrivateKey),
	)

	hash := sha256.Sum256([]byte(data))
	return base64.StdEncoding.EncodeToString(hash[:])
}

// verifyKeyCertPair verifies that a private key matches a certificate.
func verifyKeyCertPair(cert *x509.Certificate, key crypto.PrivateKey) error {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		pubKey, ok := cert.PublicKey.(*ecdsa.PublicKey)
		if !ok {
			return fmt.Errorf("certificate has non-ECDSA key but private key is ECDSA")
		}
		if k.X.Cmp(pubKey.X) != 0 || k.Y.Cmp(pubKey.Y) != 0 {
			return fmt.Errorf("ECDSA key mismatch")
		}
	case *rsa.PrivateKey:
		pubKey, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return fmt.Errorf("certificate has non-RSA key but private key is RSA")
		}
		if k.N.Cmp(pubKey.N) != 0 || k.E != pubKey.E {
			return fmt.Errorf("RSA key mismatch")
		}
	default:
		return fmt.Errorf("unsupported key type")
	}

	return nil
}
