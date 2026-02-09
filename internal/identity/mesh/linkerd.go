// Package mesh provides service mesh identity providers for Istio, Consul, and Linkerd.
package mesh

import (
	"context"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
)

// LinkerdConfig configures the Linkerd identity provider.
type LinkerdConfig struct {
	// TrustDomain is the SPIFFE trust domain.
	// If empty, derived from Linkerd identity.
	TrustDomain string

	// IdentityDir is the path to the Linkerd identity directory.
	// Default: /var/run/linkerd/identity/end-entity
	IdentityDir string

	// CertPath is the path to the certificate.
	// Default: /var/run/linkerd/identity/end-entity/crt.pem
	CertPath string

	// KeyPath is the path to the private key.
	// Default: /var/run/linkerd/identity/end-entity/key.pem
	KeyPath string

	// TrustAnchorsPath is the path to the trust anchors.
	// Default: /var/run/linkerd/identity/trust-anchors/ca-bundle.crt
	TrustAnchorsPath string

	// TokenPath is the path to the service account token.
	// Default: /var/run/secrets/kubernetes.io/serviceaccount/token
	TokenPath string

	// DestinationAPIAddr is the Linkerd destination API address.
	// Default: linkerd-destination.linkerd.svc.cluster.local:8086
	DestinationAPIAddr string

	// RefreshInterval is how often to check for certificate updates.
	// Default: 1 minute
	RefreshInterval time.Duration

	// HealthCheckInterval is how often to check provider health.
	// Default: 30 seconds
	HealthCheckInterval time.Duration
}

// DefaultLinkerdConfig returns a LinkerdConfig with default values.
func DefaultLinkerdConfig() *LinkerdConfig {
	return &LinkerdConfig{
		IdentityDir:         "/var/run/linkerd/identity/end-entity",
		CertPath:            "/var/run/linkerd/identity/end-entity/crt.pem",
		KeyPath:             "/var/run/linkerd/identity/end-entity/key.pem",
		TrustAnchorsPath:    "/var/run/linkerd/identity/trust-anchors/ca-bundle.crt",
		TokenPath:           "/var/run/secrets/kubernetes.io/serviceaccount/token",
		DestinationAPIAddr:  "linkerd-destination.linkerd.svc.cluster.local:8086",
		RefreshInterval:     time.Minute,
		HealthCheckInterval: 30 * time.Second,
	}
}

// LinkerdProvider implements the IdentityProvider interface for Linkerd.
type LinkerdProvider struct {
	config *LinkerdConfig

	mu             sync.RWMutex
	started        bool
	status         identity.ProviderStatus
	statusMessage  string
	trustDomain    string
	trustBundle    *identity.TrustBundle
	currentSVID    *identity.X509SVID
	spiffeID       identity.SPIFFEID
	namespace      string
	podName        string
	serviceAccount string

	healthCheckCancel context.CancelFunc
	refreshCancel     context.CancelFunc
	lastHealthCheck   time.Time
	certModTime       time.Time
}

// LinkerdIdentityInfo contains Linkerd identity information.
type LinkerdIdentityInfo struct {
	TrustDomain    string `json:"trustDomain"`
	Namespace      string `json:"namespace"`
	ServiceAccount string `json:"serviceAccount"`
	PodName        string `json:"podName"`
}

// NewLinkerdProvider creates a new Linkerd identity provider.
func NewLinkerdProvider(config *LinkerdConfig) (*LinkerdProvider, error) {
	if config == nil {
		config = DefaultLinkerdConfig()
	}

	return &LinkerdProvider{
		config: config,
		status: identity.ProviderStatusUnknown,
	}, nil
}

// Type returns the provider type.
func (p *LinkerdProvider) Type() identity.ProviderType {
	return identity.ProviderTypeLinkerd
}

// Start starts the provider.
func (p *LinkerdProvider) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = true
	p.mu.Unlock()

	// Detect Linkerd environment
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

	// Start health check loop - use WithoutCancel so it's not tied to Start()'s ctx lifecycle
	healthCtx, healthCancel := context.WithCancel(context.WithoutCancel(ctx))
	p.mu.Lock()
	p.healthCheckCancel = healthCancel
	p.mu.Unlock()
	go p.healthCheckLoop(healthCtx)

	// Start certificate refresh loop - use WithoutCancel so it's not tied to Start()'s ctx lifecycle
	refreshCtx, refreshCancel := context.WithCancel(context.WithoutCancel(ctx))
	p.mu.Lock()
	p.refreshCancel = refreshCancel
	p.mu.Unlock()
	go p.refreshLoop(refreshCtx)

	return nil
}

// Stop stops the provider.
func (p *LinkerdProvider) Stop(ctx context.Context) error {
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
func (p *LinkerdProvider) Health(ctx context.Context) identity.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// Info returns detailed provider information.
func (p *LinkerdProvider) Info(ctx context.Context) identity.ProviderInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metadata := map[string]string{
		"identity_dir": p.config.IdentityDir,
	}
	if p.namespace != "" {
		metadata["namespace"] = p.namespace
	}
	if p.serviceAccount != "" {
		metadata["service_account"] = p.serviceAccount
	}
	if p.podName != "" {
		metadata["pod_name"] = p.podName
	}

	if p.currentSVID != nil {
		metadata["svid_expires"] = p.currentSVID.ExpiresAt.Format(time.RFC3339)
		metadata["spiffe_id"] = p.spiffeID.String()
	}

	return identity.ProviderInfo{
		Type:            identity.ProviderTypeLinkerd,
		Status:          p.status,
		TrustDomain:     p.trustDomain,
		Capabilities:    []string{"x509_svid", "trust_bundle", "mtls"},
		LastHealthCheck: p.lastHealthCheck,
		ErrorMessage:    p.statusMessage,
		Metadata:        metadata,
	}
}

// TrustDomain returns the trust domain.
func (p *LinkerdProvider) TrustDomain() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.config.TrustDomain != "" {
		return p.config.TrustDomain
	}
	return p.trustDomain
}

// GetTrustBundle returns the current trust bundle.
func (p *LinkerdProvider) GetTrustBundle(ctx context.Context) (*identity.TrustBundle, error) {
	p.mu.RLock()
	bundle := p.trustBundle
	p.mu.RUnlock()

	if bundle != nil {
		return bundle, nil
	}

	// Load from file
	return p.loadTrustAnchors()
}

// WatchTrustBundle watches for trust bundle updates.
func (p *LinkerdProvider) WatchTrustBundle(ctx context.Context) (<-chan *identity.TrustBundle, error) {
	ch := make(chan *identity.TrustBundle, 1)

	// Send current bundle
	if bundle, err := p.GetTrustBundle(ctx); err == nil {
		select {
		case ch <- bundle:
		default:
		}
	}

	// Watch for file changes
	go func() {
		ticker := time.NewTicker(p.config.RefreshInterval)
		defer ticker.Stop()
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if bundle, err := p.loadTrustAnchors(); err == nil {
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
func (p *LinkerdProvider) GetX509SVID(ctx context.Context) (*identity.X509SVID, error) {
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
func (p *LinkerdProvider) GetSPIFFEID() (identity.SPIFFEID, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.spiffeID.TrustDomain == "" {
		return identity.SPIFFEID{}, fmt.Errorf("SPIFFE ID not available")
	}
	return p.spiffeID, nil
}

// IsAvailable returns true if Linkerd identity is available.
func (p *LinkerdProvider) IsAvailable() bool {
	// Check for Linkerd identity directory
	if _, err := os.Stat(p.config.IdentityDir); err == nil {
		return true
	}

	// Check for certificate file
	if _, err := os.Stat(p.config.CertPath); err == nil {
		return true
	}

	return false
}

// CreateAttestationEvidence creates attestation evidence for Linkerd.
func (p *LinkerdProvider) CreateAttestationEvidence(ctx context.Context) (*identity.AttestationEvidence, error) {
	// Read service account token
	token, err := os.ReadFile(p.config.TokenPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read service account token: %w", err)
	}

	p.mu.RLock()
	metadata := map[string]string{
		"trust_domain": p.trustDomain,
	}
	if p.namespace != "" {
		metadata["namespace"] = p.namespace
	}
	if p.serviceAccount != "" {
		metadata["service_account"] = p.serviceAccount
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

func (p *LinkerdProvider) detectEnvironment(ctx context.Context) error {
	if !p.IsAvailable() {
		return fmt.Errorf("linkerd environment not detected")
	}

	// Try to detect namespace from environment
	p.mu.Lock()
	p.namespace = os.Getenv("LINKERD_NS")
	if p.namespace == "" {
		p.namespace = os.Getenv("POD_NAMESPACE")
	}
	if p.namespace == "" {
		// Try to read from downward API
		if ns, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace"); err == nil {
			p.namespace = strings.TrimSpace(string(ns))
		}
	}
	p.podName = os.Getenv("HOSTNAME")
	p.serviceAccount = os.Getenv("SERVICE_ACCOUNT")
	p.mu.Unlock()

	// Try to load certificates to detect trust domain
	if err := p.loadCertificates(); err != nil {
		return nil //nolint:nilerr // non-fatal: certs may not exist yet
	}

	return nil
}

func (p *LinkerdProvider) loadCertificates() error {
	// Read certificate
	certPEM, err := os.ReadFile(p.config.CertPath)
	if err != nil {
		return fmt.Errorf("failed to read cert: %w", err)
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
		// Linkerd may use DNS names instead of URI SANs
		// Try to construct SPIFFE ID from certificate subject
		spiffeID = p.constructSPIFFEIDFromCert(certs[0])
	}

	// Get file modification time
	info, err := os.Stat(p.config.CertPath)
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

	// Extract namespace and service account from SPIFFE ID path
	if spiffeID.Path != "" {
		parts := strings.Split(strings.TrimPrefix(spiffeID.Path, "/"), "/")
		if len(parts) >= 2 {
			if p.namespace == "" {
				p.namespace = parts[0]
			}
			if p.serviceAccount == "" {
				p.serviceAccount = parts[1]
			}
		}
	}
	p.mu.Unlock()

	// Load trust anchors
	if bundle, err := p.loadTrustAnchors(); err == nil {
		p.mu.Lock()
		p.trustBundle = bundle
		p.mu.Unlock()
	}

	return nil
}

func (p *LinkerdProvider) constructSPIFFEIDFromCert(cert *x509.Certificate) identity.SPIFFEID {
	// Try to construct from DNS names (Linkerd uses identity.linkerd.cluster.local)
	for _, dns := range cert.DNSNames {
		if strings.HasSuffix(dns, ".linkerd.cluster.local") {
			// Format: <sa>.<ns>.serviceaccount.identity.linkerd.cluster.local
			parts := strings.Split(dns, ".")
			if len(parts) >= 6 {
				sa := parts[0]
				ns := parts[1]
				return identity.SPIFFEID{
					TrustDomain: "cluster.local",
					Path:        fmt.Sprintf("/%s/%s", ns, sa),
				}
			}
		}
	}

	// Fallback: use subject CN
	if cert.Subject.CommonName != "" {
		p.mu.RLock()
		ns := p.namespace
		p.mu.RUnlock()
		if ns == "" {
			ns = "default"
		}
		return identity.SPIFFEID{
			TrustDomain: "cluster.local",
			Path:        fmt.Sprintf("/%s/%s", ns, cert.Subject.CommonName),
		}
	}

	// Last resort
	return identity.SPIFFEID{
		TrustDomain: "cluster.local",
		Path:        "/unknown",
	}
}

func (p *LinkerdProvider) loadTrustAnchors() (*identity.TrustBundle, error) {
	rootPEM, err := os.ReadFile(p.config.TrustAnchorsPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read trust anchors: %w", err)
	}

	certs, err := parsePEMCertificates(rootPEM)
	if err != nil {
		return nil, fmt.Errorf("failed to parse trust anchors: %w", err)
	}

	p.mu.RLock()
	trustDomain := p.trustDomain
	p.mu.RUnlock()

	if trustDomain == "" {
		trustDomain = p.config.TrustDomain
		if trustDomain == "" {
			trustDomain = "cluster.local"
		}
	}

	return &identity.TrustBundle{
		TrustDomain:     trustDomain,
		X509Authorities: certs,
		UpdatedAt:       time.Now(),
	}, nil
}

func (p *LinkerdProvider) healthCheckLoop(ctx context.Context) {
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

func (p *LinkerdProvider) performHealthCheck() {
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

func (p *LinkerdProvider) refreshLoop(ctx context.Context) {
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

func (p *LinkerdProvider) checkForCertUpdates() {
	info, err := os.Stat(p.config.CertPath)
	if err != nil {
		return
	}

	p.mu.RLock()
	modTime := p.certModTime
	p.mu.RUnlock()

	if info.ModTime().After(modTime) {
		_ = p.loadCertificates() //nolint:errcheck // best-effort cert reload
	}
}

// GetLinkerdIdentityInfo returns Linkerd-specific identity information.
func (p *LinkerdProvider) GetLinkerdIdentityInfo() LinkerdIdentityInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return LinkerdIdentityInfo{
		TrustDomain:    p.trustDomain,
		Namespace:      p.namespace,
		ServiceAccount: p.serviceAccount,
		PodName:        p.podName,
	}
}

// Verify LinkerdProvider implements Provider
var _ identity.Provider = (*LinkerdProvider)(nil)

// LinkerdDestinationClient is a client for Linkerd's destination API.
type LinkerdDestinationClient struct {
	addr   string
	client *http.Client
}

// NewLinkerdDestinationClient creates a new destination API client.
func NewLinkerdDestinationClient(addr string) *LinkerdDestinationClient {
	return &LinkerdDestinationClient{
		addr: addr,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// GetProfile fetches a service profile from the destination API.
func (c *LinkerdDestinationClient) GetProfile(ctx context.Context, service string) (map[string]interface{}, error) {
	// Note: Full implementation would use gRPC with Linkerd's protobuf definitions
	// This is a placeholder for future implementation
	return nil, fmt.Errorf("destination API not implemented - use file-based certificates")
}

// GetIdentity fetches identity information for a service.
func (c *LinkerdDestinationClient) GetIdentity(ctx context.Context, service string) (*LinkerdIdentityInfo, error) {
	// Note: Full implementation would query the destination API
	return nil, fmt.Errorf("identity API not implemented")
}
