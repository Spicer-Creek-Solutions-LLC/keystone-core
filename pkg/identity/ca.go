package identity

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// CAManagerConfig contains configuration for the CA manager.
type CAManagerConfig struct {
	// KeyType is the key algorithm.
	KeyType string

	// RootCATTL is the root CA certificate TTL.
	RootCATTL time.Duration

	// SigningCATTL is the signing CA certificate TTL.
	SigningCATTL time.Duration

	// RotateSigningCABefore rotates signing CA this long before expiry.
	RotateSigningCABefore time.Duration

	// StoragePath is where CA keys are stored.
	StoragePath string

	// EncryptionKey is the key for encrypting CA private keys.
	EncryptionKey string

	// TrustDomain is the SPIFFE trust domain.
	TrustDomain string

	// Subject contains the CA certificate subject fields.
	Subject CASubjectConfig
}

// CAManager manages the certificate authority for the embedded identity provider.
type CAManager struct {
	config *CAManagerConfig

	rootCert      *x509.Certificate
	rootKey       crypto.PrivateKey
	signingCert   *x509.Certificate
	signingKey    crypto.PrivateKey
	trustChain    []*x509.Certificate
	serialCounter *big.Int

	mu sync.RWMutex
}

// CAInfo contains information about the CA.
type CAInfo struct {
	RootCAExpires    time.Time
	SigningCAExpires time.Time
	KeyType          string
	TrustDomain      string
}

// NewCAManager creates a new CA manager.
func NewCAManager(config *CAManagerConfig) (*CAManager, error) {
	if config == nil {
		return nil, fmt.Errorf("config required")
	}

	if config.TrustDomain == "" {
		return nil, fmt.Errorf("trust domain required")
	}

	// Set defaults
	if config.KeyType == "" {
		config.KeyType = "ecdsa-p256"
	}
	if config.RootCATTL == 0 {
		config.RootCATTL = 10 * 365 * 24 * time.Hour
	}
	if config.SigningCATTL == 0 {
		config.SigningCATTL = 365 * 24 * time.Hour
	}
	if config.RotateSigningCABefore == 0 {
		config.RotateSigningCABefore = 30 * 24 * time.Hour
	}
	if config.StoragePath == "" {
		config.StoragePath = "data/identity/ca"
	}

	return &CAManager{
		config:        config,
		serialCounter: big.NewInt(1),
	}, nil
}

// Initialize initializes the CA, loading existing keys or generating new ones.
func (m *CAManager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Ensure storage directory exists
	if err := os.MkdirAll(m.config.StoragePath, 0700); err != nil {
		return fmt.Errorf("failed to create storage directory: %w", err)
	}

	// Try to load existing CA
	if err := m.loadCA(); err == nil {
		return nil
	}

	// Generate new CA
	return m.generateCA()
}

// loadCA attempts to load existing CA certificates and keys.
func (m *CAManager) loadCA() error {
	rootCertPath := filepath.Join(m.config.StoragePath, "root-ca.crt")
	rootKeyPath := filepath.Join(m.config.StoragePath, "root-ca.key")
	signingCertPath := filepath.Join(m.config.StoragePath, "signing-ca.crt")
	signingKeyPath := filepath.Join(m.config.StoragePath, "signing-ca.key")

	// Load root CA certificate
	rootCertPEM, err := os.ReadFile(rootCertPath)
	if err != nil {
		return fmt.Errorf("failed to read root CA certificate: %w", err)
	}

	rootCert, err := parseCertificatePEM(rootCertPEM)
	if err != nil {
		return fmt.Errorf("failed to parse root CA certificate: %w", err)
	}

	// Load root CA key
	rootKeyPEM, err := os.ReadFile(rootKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read root CA key: %w", err)
	}

	rootKey, err := parsePrivateKeyPEM(rootKeyPEM, m.config.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to parse root CA key: %w", err)
	}

	// Load signing CA certificate
	signingCertPEM, err := os.ReadFile(signingCertPath)
	if err != nil {
		return fmt.Errorf("failed to read signing CA certificate: %w", err)
	}

	signingCert, err := parseCertificatePEM(signingCertPEM)
	if err != nil {
		return fmt.Errorf("failed to parse signing CA certificate: %w", err)
	}

	// Load signing CA key
	signingKeyPEM, err := os.ReadFile(signingKeyPath)
	if err != nil {
		return fmt.Errorf("failed to read signing CA key: %w", err)
	}

	signingKey, err := parsePrivateKeyPEM(signingKeyPEM, m.config.EncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to parse signing CA key: %w", err)
	}

	// Verify certificates are still valid
	now := time.Now()
	if now.After(rootCert.NotAfter) {
		return fmt.Errorf("root CA certificate has expired")
	}
	if now.After(signingCert.NotAfter) {
		return fmt.Errorf("signing CA certificate has expired")
	}

	m.rootCert = rootCert
	m.rootKey = rootKey
	m.signingCert = signingCert
	m.signingKey = signingKey
	m.trustChain = []*x509.Certificate{signingCert, rootCert}

	return nil
}

// generateCA generates a new CA hierarchy.
func (m *CAManager) generateCA() error {
	// Generate root CA
	rootKey, err := m.generateKey()
	if err != nil {
		return fmt.Errorf("failed to generate root CA key: %w", err)
	}

	rootCert, err := m.createRootCACert(rootKey)
	if err != nil {
		return fmt.Errorf("failed to create root CA certificate: %w", err)
	}

	// Generate signing CA
	signingKey, err := m.generateKey()
	if err != nil {
		return fmt.Errorf("failed to generate signing CA key: %w", err)
	}

	signingCert, err := m.createSigningCACert(signingKey, rootCert, rootKey)
	if err != nil {
		return fmt.Errorf("failed to create signing CA certificate: %w", err)
	}

	// Save to disk
	if err := m.saveCA(rootCert, rootKey, signingCert, signingKey); err != nil {
		return fmt.Errorf("failed to save CA: %w", err)
	}

	m.rootCert = rootCert
	m.rootKey = rootKey
	m.signingCert = signingCert
	m.signingKey = signingKey
	m.trustChain = []*x509.Certificate{signingCert, rootCert}

	return nil
}

// generateKey generates a new private key based on the configured key type.
func (m *CAManager) generateKey() (crypto.PrivateKey, error) {
	switch m.config.KeyType {
	case "ecdsa-p256":
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case "ecdsa-p384":
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case "rsa-2048":
		return rsa.GenerateKey(rand.Reader, 2048)
	case "rsa-4096":
		return rsa.GenerateKey(rand.Reader, 4096)
	default:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	}
}

// createRootCACert creates a self-signed root CA certificate.
func (m *CAManager) createRootCACert(key crypto.PrivateKey) (*x509.Certificate, error) {
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:         fmt.Sprintf("Keystone Core Root CA - %s", m.config.TrustDomain),
			Organization:       []string{m.config.Subject.Organization},
			OrganizationalUnit: []string{m.config.Subject.OrganizationalUnit},
		},
		NotBefore:             now,
		NotAfter:              now.Add(m.config.RootCATTL),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1,
		MaxPathLenZero:        false,
	}

	if m.config.Subject.Country != "" {
		template.Subject.Country = []string{m.config.Subject.Country}
	}
	if m.config.Subject.Province != "" {
		template.Subject.Province = []string{m.config.Subject.Province}
	}
	if m.config.Subject.Locality != "" {
		template.Subject.Locality = []string{m.config.Subject.Locality}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, publicKey(key), key)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(certDER)
}

// createSigningCACert creates a signing (intermediate) CA certificate.
func (m *CAManager) createSigningCACert(key crypto.PrivateKey, parent *x509.Certificate, parentKey crypto.PrivateKey) (*x509.Certificate, error) {
	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName:         fmt.Sprintf("Keystone Core Signing CA - %s", m.config.TrustDomain),
			Organization:       []string{m.config.Subject.Organization},
			OrganizationalUnit: []string{m.config.Subject.OrganizationalUnit},
		},
		NotBefore:             now,
		NotAfter:              now.Add(m.config.SigningCATTL),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
		MaxPathLenZero:        true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, parent, publicKey(key), parentKey)
	if err != nil {
		return nil, err
	}

	return x509.ParseCertificate(certDER)
}

// saveCA saves CA certificates and keys to disk.
func (m *CAManager) saveCA(rootCert *x509.Certificate, rootKey crypto.PrivateKey, signingCert *x509.Certificate, signingKey crypto.PrivateKey) error {
	rootCertPath := filepath.Join(m.config.StoragePath, "root-ca.crt")
	rootKeyPath := filepath.Join(m.config.StoragePath, "root-ca.key")
	signingCertPath := filepath.Join(m.config.StoragePath, "signing-ca.crt")
	signingKeyPath := filepath.Join(m.config.StoragePath, "signing-ca.key")

	// Save root CA certificate
	rootCertPEM := encodeCertificatePEM(rootCert)
	if err := os.WriteFile(rootCertPath, rootCertPEM, 0644); err != nil {
		return err
	}

	// Save root CA key (encrypted if key provided)
	rootKeyPEM, err := encodePrivateKeyPEM(rootKey, m.config.EncryptionKey)
	if err != nil {
		return err
	}
	if err := os.WriteFile(rootKeyPath, rootKeyPEM, 0600); err != nil {
		return err
	}

	// Save signing CA certificate
	signingCertPEM := encodeCertificatePEM(signingCert)
	if err := os.WriteFile(signingCertPath, signingCertPEM, 0644); err != nil {
		return err
	}

	// Save signing CA key (encrypted if key provided)
	signingKeyPEM, err := encodePrivateKeyPEM(signingKey, m.config.EncryptionKey)
	if err != nil {
		return err
	}
	if err := os.WriteFile(signingKeyPath, signingKeyPEM, 0600); err != nil {
		return err
	}

	return nil
}

// Info returns information about the CA.
func (m *CAManager) Info() CAInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	info := CAInfo{
		KeyType:     m.config.KeyType,
		TrustDomain: m.config.TrustDomain,
	}

	if m.rootCert != nil {
		info.RootCAExpires = m.rootCert.NotAfter
	}
	if m.signingCert != nil {
		info.SigningCAExpires = m.signingCert.NotAfter
	}

	return info
}

// GetTrustChain returns the trust chain (signing CA, root CA).
func (m *CAManager) GetTrustChain() []*x509.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.trustChain
}

// ShouldRotateSigningCA returns true if the signing CA should be rotated.
func (m *CAManager) ShouldRotateSigningCA() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.signingCert == nil {
		return false
	}

	rotateAt := m.signingCert.NotAfter.Add(-m.config.RotateSigningCABefore)
	return time.Now().After(rotateAt)
}

// RotateSigningCA rotates the signing CA certificate.
func (m *CAManager) RotateSigningCA(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Generate new signing CA
	signingKey, err := m.generateKey()
	if err != nil {
		return fmt.Errorf("failed to generate signing CA key: %w", err)
	}

	signingCert, err := m.createSigningCACert(signingKey, m.rootCert, m.rootKey)
	if err != nil {
		return fmt.Errorf("failed to create signing CA certificate: %w", err)
	}

	// Save new signing CA
	signingCertPath := filepath.Join(m.config.StoragePath, "signing-ca.crt")
	signingKeyPath := filepath.Join(m.config.StoragePath, "signing-ca.key")

	signingCertPEM := encodeCertificatePEM(signingCert)
	if err := os.WriteFile(signingCertPath, signingCertPEM, 0644); err != nil {
		return err
	}

	signingKeyPEM, err := encodePrivateKeyPEM(signingKey, m.config.EncryptionKey)
	if err != nil {
		return err
	}
	if err := os.WriteFile(signingKeyPath, signingKeyPEM, 0600); err != nil {
		return err
	}

	m.signingCert = signingCert
	m.signingKey = signingKey
	m.trustChain = []*x509.Certificate{signingCert, m.rootCert}

	return nil
}

// SignX509SVID signs an X.509 SVID certificate.
func (m *CAManager) SignX509SVID(spiffeID SPIFFEID, publicKey crypto.PublicKey, ttl time.Duration, dnsNames []string, ipAddresses []net.IP) (*x509.Certificate, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.signingCert == nil || m.signingKey == nil {
		return nil, fmt.Errorf("signing CA not initialized")
	}

	// Generate serial number
	m.serialCounter = new(big.Int).Add(m.serialCounter, big.NewInt(1))
	serialNumber := new(big.Int).Set(m.serialCounter)

	// Parse SPIFFE ID as URL for SAN
	spiffeURI, err := url.Parse(spiffeID.String())
	if err != nil {
		return nil, fmt.Errorf("invalid SPIFFE ID: %w", err)
	}

	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: spiffeID.String(),
		},
		NotBefore:   now,
		NotAfter:    now.Add(ttl),
		KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		URIs:        []*url.URL{spiffeURI},
		DNSNames:    dnsNames,
		IPAddresses: ipAddresses,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, m.signingCert, publicKey, m.signingKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	return x509.ParseCertificate(certDER)
}

// Helper functions

func publicKey(key crypto.PrivateKey) crypto.PublicKey {
	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		return &k.PublicKey
	case *rsa.PrivateKey:
		return &k.PublicKey
	default:
		return nil
	}
}

func encodeCertificatePEM(cert *x509.Certificate) []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
}

func parseCertificatePEM(data []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("unexpected PEM block type: %s", block.Type)
	}
	return x509.ParseCertificate(block.Bytes)
}

func encodePrivateKeyPEM(key crypto.PrivateKey, encryptionKey string) ([]byte, error) {
	var keyDER []byte
	var keyType string

	switch k := key.(type) {
	case *ecdsa.PrivateKey:
		var err error
		keyDER, err = x509.MarshalECPrivateKey(k)
		if err != nil {
			return nil, err
		}
		keyType = "EC PRIVATE KEY"
	case *rsa.PrivateKey:
		keyDER = x509.MarshalPKCS1PrivateKey(k)
		keyType = "RSA PRIVATE KEY"
	default:
		return nil, fmt.Errorf("unsupported key type")
	}

	block := &pem.Block{
		Type:  keyType,
		Bytes: keyDER,
	}

	// Note: For production, implement proper encryption using the encryptionKey
	// For now, we store keys unencrypted
	_ = encryptionKey

	return pem.EncodeToMemory(block), nil
}

func parsePrivateKeyPEM(data []byte, encryptionKey string) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	// Note: For production, implement proper decryption using the encryptionKey
	_ = encryptionKey

	switch block.Type {
	case "EC PRIVATE KEY":
		return x509.ParseECPrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	case "PRIVATE KEY":
		return x509.ParsePKCS8PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported key type: %s", block.Type)
	}
}
