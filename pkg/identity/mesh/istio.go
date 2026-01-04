// Package mesh provides service mesh identity providers for Istio, Consul, and Linkerd.
package mesh

import (
	"context"
	"crypto"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/identity"
)

// IstioConfig configures the Istio identity provider.
type IstioConfig struct {
	// TrustDomain is the SPIFFE trust domain.
	// If empty, auto-detected from Istio.
	TrustDomain string

	// SDSAddress is the SDS (Secret Discovery Service) address.
	// Default: unix:///var/run/secrets/workload-spiffe-uds/socket
	SDSAddress string

	// CertPath is the path to the Istio certificates directory.
	// Default: /var/run/secrets/istio
	CertPath string

	// RootCertPath is the path to the root CA certificate.
	// Default: /var/run/secrets/istio/root-cert.pem
	RootCertPath string

	// CertChainPath is the path to the certificate chain.
	// Default: /var/run/secrets/istio/cert-chain.pem
	CertChainPath string

	// KeyPath is the path to the private key.
	// Default: /var/run/secrets/istio/key.pem
	KeyPath string

	// ServiceAccountTokenPath is the path to the service account token.
	// Default: /var/run/secrets/kubernetes.io/serviceaccount/token
	ServiceAccountTokenPath string

	// RefreshInterval is how often to check for certificate updates.
	// Default: 1 minute
	RefreshInterval time.Duration

	// HealthCheckInterval is how often to check provider health.
	// Default: 30 seconds
	HealthCheckInterval time.Duration
}

// DefaultIstioConfig returns an IstioConfig with default values.
func DefaultIstioConfig() *IstioConfig {
	return &IstioConfig{
		SDSAddress:              "unix:///var/run/secrets/workload-spiffe-uds/socket",
		CertPath:                "/var/run/secrets/istio",
		RootCertPath:            "/var/run/secrets/istio/root-cert.pem",
		CertChainPath:           "/var/run/secrets/istio/cert-chain.pem",
		KeyPath:                 "/var/run/secrets/istio/key.pem",
		ServiceAccountTokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token",
		RefreshInterval:         time.Minute,
		HealthCheckInterval:     30 * time.Second,
	}
}

// IstioProvider implements the IdentityProvider interface for Istio.
type IstioProvider struct {
	config *IstioConfig

	mu            sync.RWMutex
	started       bool
	status        identity.ProviderStatus
	statusMessage string
	trustDomain   string
	trustBundle   *identity.TrustBundle
	currentSVID   *identity.X509SVID
	spiffeID      identity.SPIFFEID

	healthCheckCancel context.CancelFunc
	refreshCancel     context.CancelFunc
	lastHealthCheck   time.Time
	certModTime       time.Time
}

// NewIstioProvider creates a new Istio identity provider.
func NewIstioProvider(config *IstioConfig) (*IstioProvider, error) {
	if config == nil {
		config = DefaultIstioConfig()
	}

	return &IstioProvider{
		config: config,
		status: identity.ProviderStatusUnknown,
	}, nil
}

// Type returns the provider type.
func (p *IstioProvider) Type() identity.ProviderType {
	return identity.ProviderTypeIstio
}

// Start starts the provider.
func (p *IstioProvider) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = true
	p.mu.Unlock()

	// Detect Istio environment
	if err := p.detectEnvironment(ctx); err != nil {
		p.mu.Lock()
		p.status = identity.ProviderStatusUnhealthy
		p.statusMessage = err.Error()
		p.mu.Unlock()
		return err
	}

	// Load initial certificates
	if err := p.loadCertificates(); err != nil {
		p.mu.Lock()
		p.status = identity.ProviderStatusUnhealthy
		p.statusMessage = err.Error()
		p.mu.Unlock()
		return err
	}

	p.mu.Lock()
	p.status = identity.ProviderStatusHealthy
	p.statusMessage = ""
	p.mu.Unlock()

	// Start health check loop
	healthCtx, healthCancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.healthCheckCancel = healthCancel
	p.mu.Unlock()
	go p.healthCheckLoop(healthCtx)

	// Start certificate refresh loop
	refreshCtx, refreshCancel := context.WithCancel(context.Background())
	p.mu.Lock()
	p.refreshCancel = refreshCancel
	p.mu.Unlock()
	go p.refreshLoop(refreshCtx)

	return nil
}

// Stop stops the provider.
func (p *IstioProvider) Stop(ctx context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.started {
		return nil
	}
	p.started = false

	if p.healthCheckCancel != nil {
		p.healthCheckCancel()
	}
	if p.refreshCancel != nil {
		p.refreshCancel()
	}

	return nil
}

// Health returns the current health status.
func (p *IstioProvider) Health(ctx context.Context) identity.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// Info returns detailed provider information.
func (p *IstioProvider) Info(ctx context.Context) identity.ProviderInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metadata := map[string]string{
		"sds_address": p.config.SDSAddress,
		"cert_path":   p.config.CertPath,
	}

	if p.currentSVID != nil {
		metadata["svid_expires"] = p.currentSVID.ExpiresAt.Format(time.RFC3339)
		metadata["spiffe_id"] = p.spiffeID.String()
	}

	return identity.ProviderInfo{
		Type:            identity.ProviderTypeIstio,
		Status:          p.status,
		TrustDomain:     p.trustDomain,
		Capabilities:    []string{"x509_svid", "trust_bundle", "sds"},
		LastHealthCheck: p.lastHealthCheck,
		ErrorMessage:    p.statusMessage,
		Metadata:        metadata,
	}
}

// TrustDomain returns the trust domain.
func (p *IstioProvider) TrustDomain() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.config.TrustDomain != "" {
		return p.config.TrustDomain
	}
	return p.trustDomain
}

// GetTrustBundle returns the current trust bundle.
func (p *IstioProvider) GetTrustBundle(ctx context.Context) (*identity.TrustBundle, error) {
	p.mu.RLock()
	bundle := p.trustBundle
	p.mu.RUnlock()

	if bundle != nil {
		return bundle, nil
	}

	// Load from file
	return p.loadTrustBundle()
}

// WatchTrustBundle watches for trust bundle updates.
func (p *IstioProvider) WatchTrustBundle(ctx context.Context) (<-chan *identity.TrustBundle, error) {
	ch := make(chan *identity.TrustBundle, 1)

	// Send current bundle
	if bundle, err := p.GetTrustBundle(ctx); err == nil {
		select {
		case ch <- bundle:
		default:
		}
	}

	// Watch for file changes (simple polling for now)
	go func() {
		ticker := time.NewTicker(p.config.RefreshInterval)
		defer ticker.Stop()
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if bundle, err := p.loadTrustBundle(); err == nil {
					select {
					case ch <- bundle:
					default:
					}
				}
			}
		}
	}()

	return ch, nil
}

// GetX509SVID returns the current X.509 SVID.
func (p *IstioProvider) GetX509SVID(ctx context.Context) (*identity.X509SVID, error) {
	p.mu.RLock()
	svid := p.currentSVID
	p.mu.RUnlock()

	if svid != nil && !svid.Expired() && !svid.ShouldRotate() {
		return svid, nil
	}

	// Reload certificates
	if err := p.loadCertificates(); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentSVID, nil
}

// GetSPIFFEID returns the SPIFFE ID.
func (p *IstioProvider) GetSPIFFEID() (identity.SPIFFEID, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.spiffeID.TrustDomain == "" {
		return identity.SPIFFEID{}, fmt.Errorf("SPIFFE ID not available")
	}
	return p.spiffeID, nil
}

// IsAvailable returns true if Istio identity is available.
func (p *IstioProvider) IsAvailable() bool {
	// Check for Istio cert files
	if _, err := os.Stat(p.config.CertChainPath); err == nil {
		return true
	}

	// Check for SDS socket
	socketPath := strings.TrimPrefix(p.config.SDSAddress, "unix://")
	if _, err := os.Stat(socketPath); err == nil {
		return true
	}

	return false
}

// CreateAttestationEvidence creates attestation evidence for Istio.
func (p *IstioProvider) CreateAttestationEvidence(ctx context.Context) (*identity.AttestationEvidence, error) {
	// Read service account token
	token, err := os.ReadFile(p.config.ServiceAccountTokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service account token: %w", err)
	}

	p.mu.RLock()
	metadata := map[string]string{
		"trust_domain": p.trustDomain,
	}
	if p.spiffeID.TrustDomain != "" {
		metadata["spiffe_id"] = p.spiffeID.String()
	}
	p.mu.RUnlock()

	return &identity.AttestationEvidence{
		Type:     identity.AttestationTypeK8sSAT,
		Data:     token,
		Metadata: metadata,
	}, nil
}

// Private methods

func (p *IstioProvider) detectEnvironment(ctx context.Context) error {
	if !p.IsAvailable() {
		return fmt.Errorf("Istio environment not detected")
	}

	// Try to load certificates to detect trust domain
	if err := p.loadCertificates(); err != nil {
		// Non-fatal, we may be able to use SDS instead
		return nil
	}

	return nil
}

func (p *IstioProvider) loadCertificates() error {
	// Read certificate chain
	certPEM, err := os.ReadFile(p.config.CertChainPath)
	if err != nil {
		return fmt.Errorf("failed to read cert chain: %w", err)
	}

	// Read private key
	keyPEM, err := os.ReadFile(p.config.KeyPath)
	if err != nil {
		return fmt.Errorf("failed to read key: %w", err)
	}

	// Parse certificates
	certs, err := parsePEMCertificates(certPEM)
	if err != nil {
		return fmt.Errorf("failed to parse certificates: %w", err)
	}

	if len(certs) == 0 {
		return fmt.Errorf("no certificates in chain")
	}

	// Parse private key
	key, err := parsePEMPrivateKey(keyPEM)
	if err != nil {
		return fmt.Errorf("failed to parse key: %w", err)
	}

	// Extract SPIFFE ID from certificate
	spiffeID, err := extractSPIFFEIDFromCert(certs[0])
	if err != nil {
		return fmt.Errorf("failed to extract SPIFFE ID: %w", err)
	}

	// Get file modification time
	info, err := os.Stat(p.config.CertChainPath)
	if err != nil {
		return fmt.Errorf("failed to stat cert file: %w", err)
	}

	p.mu.Lock()
	p.currentSVID = &identity.X509SVID{
		SPIFFEID:     spiffeID,
		Certificates: certs,
		PrivateKey:   key,
		ExpiresAt:    certs[0].NotAfter,
		IssuedAt:     certs[0].NotBefore,
	}
	p.spiffeID = spiffeID
	p.trustDomain = spiffeID.TrustDomain
	p.certModTime = info.ModTime()
	p.mu.Unlock()

	// Load trust bundle
	if bundle, err := p.loadTrustBundle(); err == nil {
		p.mu.Lock()
		p.trustBundle = bundle
		p.mu.Unlock()
	}

	return nil
}

func (p *IstioProvider) loadTrustBundle() (*identity.TrustBundle, error) {
	rootPEM, err := os.ReadFile(p.config.RootCertPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read root cert: %w", err)
	}

	certs, err := parsePEMCertificates(rootPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse root certs: %w", err)
	}

	p.mu.RLock()
	trustDomain := p.trustDomain
	p.mu.RUnlock()

	if trustDomain == "" {
		trustDomain = p.config.TrustDomain
	}

	return &identity.TrustBundle{
		TrustDomain:     trustDomain,
		X509Authorities: certs,
		UpdatedAt:       time.Now(),
	}, nil
}

func (p *IstioProvider) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.performHealthCheck()
		}
	}
}

func (p *IstioProvider) performHealthCheck() {
	p.mu.Lock()
	p.lastHealthCheck = time.Now()

	if p.currentSVID == nil {
		p.status = identity.ProviderStatusUnhealthy
		p.statusMessage = "no SVID loaded"
		p.mu.Unlock()
		return
	}

	if p.currentSVID.Expired() {
		p.status = identity.ProviderStatusUnhealthy
		p.statusMessage = "SVID expired"
		p.mu.Unlock()
		return
	}

	if p.currentSVID.ShouldRotate() {
		p.status = identity.ProviderStatusDegraded
		p.statusMessage = "SVID needs rotation"
		p.mu.Unlock()
		return
	}

	p.status = identity.ProviderStatusHealthy
	p.statusMessage = ""
	p.mu.Unlock()
}

func (p *IstioProvider) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(p.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkForCertUpdates()
		}
	}
}

func (p *IstioProvider) checkForCertUpdates() {
	info, err := os.Stat(p.config.CertChainPath)
	if err != nil {
		return
	}

	p.mu.RLock()
	modTime := p.certModTime
	p.mu.RUnlock()

	if info.ModTime().After(modTime) {
		p.loadCertificates()
	}
}

// Verify IstioProvider implements IdentityProvider
var _ identity.IdentityProvider = (*IstioProvider)(nil)

// Helper functions

func parsePEMCertificates(pemData []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for {
		block, rest := pem.Decode(pemData)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			certs = append(certs, cert)
		}
		pemData = rest
	}
	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found")
	}
	return certs, nil
}

func parsePEMPrivateKey(pemData []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}

	// Try PKCS8
	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err == nil {
		return key, nil
	}

	// Try EC
	ecKey, err := x509.ParseECPrivateKey(block.Bytes)
	if err == nil {
		return ecKey, nil
	}

	// Try RSA
	rsaKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err == nil {
		return rsaKey, nil
	}

	return nil, fmt.Errorf("failed to parse private key")
}

func extractSPIFFEIDFromCert(cert *x509.Certificate) (identity.SPIFFEID, error) {
	for _, uri := range cert.URIs {
		if uri.Scheme == "spiffe" {
			return identity.SPIFFEID{
				TrustDomain: uri.Host,
				Path:        uri.Path,
			}, nil
		}
	}
	return identity.SPIFFEID{}, fmt.Errorf("no SPIFFE ID in certificate")
}

// IstioSDSClient is a client for Istio's Secret Discovery Service.
type IstioSDSClient struct {
	address string
	// Note: Full SDS implementation would use Envoy's xDS protocol
	// This is a placeholder for future implementation
}

// NewIstioSDSClient creates a new SDS client.
func NewIstioSDSClient(address string) *IstioSDSClient {
	return &IstioSDSClient{address: address}
}

// FetchSecret fetches a secret from SDS.
func (c *IstioSDSClient) FetchSecret(ctx context.Context, name string) ([]byte, error) {
	// Note: Full implementation would use gRPC with Envoy's SDS protocol
	return nil, fmt.Errorf("SDS not implemented - use file-based certificates")
}

// GetCertPaths returns the standard Istio certificate paths.
func GetCertPaths(basePath string) (certChain, key, rootCert string) {
	if basePath == "" {
		basePath = "/var/run/secrets/istio"
	}
	return filepath.Join(basePath, "cert-chain.pem"),
		filepath.Join(basePath, "key.pem"),
		filepath.Join(basePath, "root-cert.pem")
}
