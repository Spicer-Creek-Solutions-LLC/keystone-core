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
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"sync"
	"time"
)

// ErrInvalidCAConfig wraps every rejection in this file —
// configuration validation, issuance rejection, rotation
// preconditions. Callers branch with [errors.Is].
var ErrInvalidCAConfig = errors.New("identity: invalid CA config")

// CAKeyType selects the asymmetric algorithm + curve / size for the
// root + signing CAs per PROJECT-DETAILS §4.10. ECDSA-P256 is the
// default; the other three exist for operators with policy
// constraints (FIPS, regulatory minimums, …).
type CAKeyType string

const (
	KeyTypeECDSAP256 CAKeyType = "ecdsa-p256"
	KeyTypeECDSAP384 CAKeyType = "ecdsa-p384"
	KeyTypeRSA2048   CAKeyType = "rsa-2048"
	KeyTypeRSA4096   CAKeyType = "rsa-4096"
)

// CAConfig drives [CAManager]. Use [DefaultCAConfig] for the §4.10
// defaults; override only what you need.
type CAConfig struct {
	TrustDomain    string
	KeyType        CAKeyType
	RootCATTL      time.Duration
	SigningCATTL   time.Duration
	RotateBefore   time.Duration
	DefaultSVIDTTL time.Duration
	MaxSVIDTTL     time.Duration
	Clock          func() time.Time // tests inject; production callers leave zero
}

// Default durations per PROJECT-DETAILS §4.10.
const (
	defaultRootCATTL    = 10 * 365 * 24 * time.Hour // ~10 years
	defaultSigningCATTL = 365 * 24 * time.Hour      // ~1 year
	defaultRotateBefore = 30 * 24 * time.Hour       // 30 days
	defaultSVIDTTL      = 1 * time.Hour
	maxSVIDTTLDefault   = 24 * time.Hour
)

// DefaultCAConfig returns the §4.10 default config for trustDomain.
// Callers who want a different KeyType / TTL override after the call.
func DefaultCAConfig(trustDomain string) CAConfig {
	return CAConfig{
		TrustDomain:    trustDomain,
		KeyType:        KeyTypeECDSAP256,
		RootCATTL:      defaultRootCATTL,
		SigningCATTL:   defaultSigningCATTL,
		RotateBefore:   defaultRotateBefore,
		DefaultSVIDTTL: defaultSVIDTTL,
		MaxSVIDTTL:     maxSVIDTTLDefault,
	}
}

func (c CAConfig) withDefaults() CAConfig {
	if c.KeyType == "" {
		c.KeyType = KeyTypeECDSAP256
	}
	if c.RootCATTL == 0 {
		c.RootCATTL = defaultRootCATTL
	}
	if c.SigningCATTL == 0 {
		c.SigningCATTL = defaultSigningCATTL
	}
	if c.RotateBefore == 0 {
		c.RotateBefore = defaultRotateBefore
	}
	if c.DefaultSVIDTTL == 0 {
		c.DefaultSVIDTTL = defaultSVIDTTL
	}
	if c.MaxSVIDTTL == 0 {
		c.MaxSVIDTTL = maxSVIDTTLDefault
	}
	if c.Clock == nil {
		c.Clock = time.Now
	}
	return c
}

func (c CAConfig) validate() error {
	if c.TrustDomain == "" {
		return fmt.Errorf("%w: TrustDomain is required", ErrInvalidCAConfig)
	}
	switch c.KeyType {
	case "", KeyTypeECDSAP256, KeyTypeECDSAP384, KeyTypeRSA2048, KeyTypeRSA4096:
	default:
		return fmt.Errorf("%w: unknown KeyType %q", ErrInvalidCAConfig, c.KeyType)
	}
	if c.RotateBefore >= c.SigningCATTL && c.SigningCATTL != 0 {
		return fmt.Errorf("%w: RotateBefore (%s) must be < SigningCATTL (%s)", ErrInvalidCAConfig, c.RotateBefore, c.SigningCATTL)
	}
	if c.MaxSVIDTTL > c.SigningCATTL && c.SigningCATTL != 0 {
		return fmt.Errorf("%w: MaxSVIDTTL (%s) must be ≤ SigningCATTL (%s)", ErrInvalidCAConfig, c.MaxSVIDTTL, c.SigningCATTL)
	}
	return nil
}

// CAManager owns the two-tier root + signing CA pair. A long-lived
// root signs the signing CA; the signing CA signs end-entity
// X.509-SVIDs. Concurrent calls to [CAManager.IssueCertificate] are
// safe.
//
// State machine:
//
//	NewCAManager(cfg, storage)
//	 │
//	 ▼
//	Initialize(ctx) ─► loads or generates root + signing CAs
//	 │
//	 ▼
//	IssueCertificate(req)
//	 │
//	 ▼
//	ShouldRotateSigningCA(now) ─► RotateSigningCA(ctx)
type CAManager struct {
	cfg     CAConfig
	storage CAStorage

	mu          sync.RWMutex
	initialized bool
	rootCert    *x509.Certificate
	rootKey     crypto.Signer
	signingCert *x509.Certificate
	signingKey  crypto.Signer
}

// NewCAManager validates cfg + storage and returns an uninitialized
// manager. Call [CAManager.Initialize] before [CAManager.IssueCertificate].
func NewCAManager(cfg CAConfig, storage CAStorage) (*CAManager, error) {
	if storage == nil {
		return nil, fmt.Errorf("%w: storage is required", ErrInvalidCAConfig)
	}
	cfg = cfg.withDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &CAManager{cfg: cfg, storage: storage}, nil
}

// Initialize loads existing CAs from storage; when absent it
// generates a fresh root + signing CA pair and persists them.
// Idempotent — calling twice on the same manager is a no-op.
func (m *CAManager) Initialize(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.initialized {
		return nil
	}

	if err := m.loadOrGenerateRoot(ctx); err != nil {
		return err
	}
	if err := m.loadOrGenerateSigning(ctx); err != nil {
		return err
	}
	m.initialized = true
	return nil
}

func (m *CAManager) loadOrGenerateRoot(_ context.Context) error {
	has, err := m.storage.HasRootCA()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCAConfig, err)
	}
	if has {
		cert, key, err := m.storage.LoadRootCA()
		if err != nil {
			return fmt.Errorf("%w: load root: %v", ErrInvalidCAConfig, err)
		}
		m.rootCert, m.rootKey = cert, key
		return nil
	}
	cert, key, err := m.generateRoot()
	if err != nil {
		return err
	}
	if err := m.storage.SaveRootCA(cert, key); err != nil {
		return fmt.Errorf("%w: save root: %v", ErrInvalidCAConfig, err)
	}
	m.rootCert, m.rootKey = cert, key
	return nil
}

func (m *CAManager) loadOrGenerateSigning(_ context.Context) error {
	has, err := m.storage.HasSigningCA()
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCAConfig, err)
	}
	if has {
		cert, key, err := m.storage.LoadSigningCA()
		if err != nil {
			return fmt.Errorf("%w: load signing: %v", ErrInvalidCAConfig, err)
		}
		m.signingCert, m.signingKey = cert, key
		return nil
	}
	cert, key, err := m.generateSigning()
	if err != nil {
		return err
	}
	if err := m.storage.SaveSigningCA(cert, key); err != nil {
		return fmt.Errorf("%w: save signing: %v", ErrInvalidCAConfig, err)
	}
	m.signingCert, m.signingKey = cert, key
	return nil
}

// generateRoot mints a self-signed root CA.
func (m *CAManager) generateRoot() (*x509.Certificate, crypto.Signer, error) {
	key, err := generateKey(m.cfg.KeyType)
	if err != nil {
		return nil, nil, err
	}
	now := m.cfg.Clock()
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "kscore root CA (" + m.cfg.TrustDomain + ")"},
		NotBefore:             now,
		NotAfter:              now.Add(m.cfg.RootCATTL),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            1, // root signs signing CA only
		MaxPathLenZero:        false,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, key.Public(), key)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: create root cert: %v", ErrInvalidCAConfig, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse root cert: %v", ErrInvalidCAConfig, err)
	}
	return cert, key, nil
}

// generateSigning mints a signing CA signed by the root.
// Precondition: m.rootCert and m.rootKey must already be set.
func (m *CAManager) generateSigning() (*x509.Certificate, crypto.Signer, error) {
	key, err := generateKey(m.cfg.KeyType)
	if err != nil {
		return nil, nil, err
	}
	now := m.cfg.Clock()
	serial, err := randomSerial()
	if err != nil {
		return nil, nil, err
	}
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "kscore signing CA (" + m.cfg.TrustDomain + ")"},
		NotBefore:             now,
		NotAfter:              now.Add(m.cfg.SigningCATTL),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0, // signs leaves only
		MaxPathLenZero:        true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, m.rootCert, key.Public(), m.rootKey)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: create signing cert: %v", ErrInvalidCAConfig, err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: parse signing cert: %v", ErrInvalidCAConfig, err)
	}
	return cert, key, nil
}

// GetTrustChain returns the verification anchors for this CA — the
// root. The signing CA chains to the root and is included in every
// issued leaf's chain, so verifiers only need the root in their
// trust bundle.
//
// nil before Initialize.
func (m *CAManager) GetTrustChain() []*x509.Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.rootCert == nil {
		return nil
	}
	return []*x509.Certificate{m.rootCert}
}

// BuildTrustBundle returns a [*TrustBundle] for the configured
// trust domain seeded with the root cert as the X509 authority.
// JWT authorities are NOT added here — [Provider] wiring (task 7)
// is responsible for those alongside the JWT signing key.
func (m *CAManager) BuildTrustBundle() (*TrustBundle, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return nil, fmt.Errorf("%w: BuildTrustBundle before Initialize", ErrInvalidCAConfig)
	}
	return TrustBundleFromAuthorities(
		m.cfg.TrustDomain,
		[]*x509.Certificate{m.rootCert},
		nil,
	)
}

// IssueRequest is the operator-facing issuance input. ID populates
// the SPIFFE URI SAN; PublicKey is the subject's public half (the
// caller retains the private half). TTL=0 falls back to
// CAConfig.DefaultSVIDTTL; values above CAConfig.MaxSVIDTTL are
// silently capped.
type IssueRequest struct {
	ID          SPIFFEID
	PublicKey   crypto.PublicKey
	TTL         time.Duration
	DNSNames    []string
	IPAddresses []net.IP
	KeyUsage    x509.KeyUsage
	ExtKeyUsage []x509.ExtKeyUsage
}

// IssuedCertificate is what [CAManager.IssueCertificate] returns.
// Chain is [leaf, signingCA] — exactly the shape [NewX509SVID]
// expects.
type IssuedCertificate struct {
	Chain []*x509.Certificate
	Leaf  *x509.Certificate
}

// IssueCertificate signs a leaf for req under the active signing
// CA. The leaf carries req.ID as its sole URI SAN, with the
// declared key + extended key usage (defaults: digital signature +
// key encipherment for KeyUsage; server + client auth for
// ExtKeyUsage — matches the §4.10 mTLS profile).
func (m *CAManager) IssueCertificate(req IssueRequest) (*IssuedCertificate, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.initialized {
		return nil, fmt.Errorf("%w: IssueCertificate before Initialize", ErrInvalidCAConfig)
	}
	if req.ID.IsZero() {
		return nil, fmt.Errorf("%w: ID is required", ErrInvalidCAConfig)
	}
	if req.PublicKey == nil {
		return nil, fmt.Errorf("%w: PublicKey is required", ErrInvalidCAConfig)
	}
	ttl := req.TTL
	if ttl <= 0 {
		ttl = m.cfg.DefaultSVIDTTL
	}
	if ttl > m.cfg.MaxSVIDTTL {
		ttl = m.cfg.MaxSVIDTTL
	}

	serial, err := randomSerial()
	if err != nil {
		return nil, err
	}
	now := m.cfg.Clock()
	keyUsage := req.KeyUsage
	if keyUsage == 0 {
		keyUsage = x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment
	}
	extKeyUsage := req.ExtKeyUsage
	if len(extKeyUsage) == 0 {
		extKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth}
	}

	tpl := &x509.Certificate{
		SerialNumber:   serial,
		Subject:        pkix.Name{CommonName: req.ID.String()},
		NotBefore:      now,
		NotAfter:       now.Add(ttl),
		KeyUsage:       keyUsage,
		ExtKeyUsage:    extKeyUsage,
		URIs:           []*url.URL{req.ID.URI()},
		DNSNames:       req.DNSNames,
		IPAddresses:    req.IPAddresses,
		BasicConstraintsValid: true,
		IsCA:           false,
	}

	der, err := x509.CreateCertificate(rand.Reader, tpl, m.signingCert, req.PublicKey, m.signingKey)
	if err != nil {
		return nil, fmt.Errorf("%w: create leaf: %v", ErrInvalidCAConfig, err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		return nil, fmt.Errorf("%w: parse leaf: %v", ErrInvalidCAConfig, err)
	}
	return &IssuedCertificate{
		Chain: []*x509.Certificate{leaf, m.signingCert},
		Leaf:  leaf,
	}, nil
}

// ShouldRotateSigningCA reports whether the signing CA is within
// CAConfig.RotateBefore of expiry at `now`. Task 6's rotation loop
// polls this hourly.
func (m *CAManager) ShouldRotateSigningCA(now time.Time) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.signingCert == nil {
		return false
	}
	return !now.Before(m.signingCert.NotAfter.Add(-m.cfg.RotateBefore))
}

// RotateSigningCA generates a fresh signing CA signed by the root,
// persists it, and atomically replaces the active one. Old leaves
// continue to verify: their Chain still includes the old signing
// CA cert; the unchanged root in the trust bundle still anchors
// it.
func (m *CAManager) RotateSigningCA(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.initialized {
		return fmt.Errorf("%w: RotateSigningCA before Initialize", ErrInvalidCAConfig)
	}
	_ = ctx // reserved for future ctx-aware storage backends
	cert, key, err := m.generateSigning()
	if err != nil {
		return err
	}
	if err := m.storage.SaveSigningCA(cert, key); err != nil {
		return fmt.Errorf("%w: save signing: %v", ErrInvalidCAConfig, err)
	}
	m.signingCert, m.signingKey = cert, key
	return nil
}

// ---- key + serial helpers ----------------------------------------

// generateKey mints a fresh key of the requested type.
func generateKey(t CAKeyType) (crypto.Signer, error) {
	switch t {
	case KeyTypeECDSAP256:
		return ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	case KeyTypeECDSAP384:
		return ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	case KeyTypeRSA2048:
		return rsa.GenerateKey(rand.Reader, 2048)
	case KeyTypeRSA4096:
		return rsa.GenerateKey(rand.Reader, 4096)
	}
	return nil, fmt.Errorf("%w: unknown key type %q", ErrInvalidCAConfig, t)
}

// randomSerial draws a 128-bit random serial number. RFC 5280
// recommends ≥ 64 bits; 128 keeps a comfortable margin against
// collisions even at high issuance volumes.
func randomSerial() (*big.Int, error) {
	max := new(big.Int).Lsh(big.NewInt(1), 128)
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return nil, fmt.Errorf("%w: serial: %v", ErrInvalidCAConfig, err)
	}
	return n, nil
}
