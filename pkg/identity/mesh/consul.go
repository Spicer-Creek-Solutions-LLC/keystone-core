// Package mesh provides service mesh identity providers for Istio, Consul, and Linkerd.
package mesh

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/identity"
)

// ConsulConfig configures the Consul Connect identity provider.
type ConsulConfig struct {
	// TrustDomain is the SPIFFE trust domain.
	// If empty, derived from Consul datacenter.
	TrustDomain string

	// HTTPAddr is the Consul HTTP API address.
	// Default: http://127.0.0.1:8500
	HTTPAddr string

	// Token is the Consul ACL token.
	Token string

	// CertPath is the path to Connect certificates.
	// Default: /etc/consul.d/connect
	CertPath string

	// CACertPath is the path to the CA certificate.
	CACertPath string

	// CertFile is the path to the leaf certificate.
	CertFile string

	// KeyFile is the path to the private key.
	KeyFile string

	// ServiceName is the registered service name.
	ServiceName string

	// Datacenter is the Consul datacenter.
	Datacenter string

	// RefreshInterval is how often to refresh certificates.
	// Default: 1 minute
	RefreshInterval time.Duration

	// HealthCheckInterval is how often to check provider health.
	// Default: 30 seconds
	HealthCheckInterval time.Duration

	// HTTPTimeout is the timeout for HTTP requests.
	// Default: 10 seconds
	HTTPTimeout time.Duration
}

// DefaultConsulConfig returns a ConsulConfig with default values.
func DefaultConsulConfig() *ConsulConfig {
	return &ConsulConfig{
		HTTPAddr:            "http://127.0.0.1:8500",
		CertPath:            "/etc/consul.d/connect",
		RefreshInterval:     time.Minute,
		HealthCheckInterval: 30 * time.Second,
		HTTPTimeout:         10 * time.Second,
	}
}

// ConsulProvider implements the IdentityProvider interface for Consul Connect.
type ConsulProvider struct {
	config *ConsulConfig
	client *http.Client

	mu            sync.RWMutex
	started       bool
	status        identity.ProviderStatus
	statusMessage string
	trustDomain   string
	trustBundle   *identity.TrustBundle
	currentSVID   *identity.X509SVID
	spiffeID      identity.SPIFFEID
	datacenter    string
	serviceID     string

	healthCheckCancel context.CancelFunc
	refreshCancel     context.CancelFunc
	lastHealthCheck   time.Time
	certModTime       time.Time
}

// ConsulAgentResponse is the response from the Consul agent API.
type ConsulAgentResponse struct {
	Config ConsulAgentConfig `json:"Config"`
}

// ConsulAgentConfig is the agent configuration.
type ConsulAgentConfig struct {
	Datacenter string `json:"Datacenter"`
	NodeName   string `json:"NodeName"`
	NodeID     string `json:"NodeID"`
}

// ConsulLeafCert is a Consul Connect leaf certificate.
type ConsulLeafCert struct {
	CertPEM       string `json:"CertPEM"`
	PrivateKeyPEM string `json:"PrivateKeyPEM"`
	Service       string `json:"Service"`
	ServiceURI    string `json:"ServiceURI"`
	ValidBefore   string `json:"ValidBefore"`
	ValidAfter    string `json:"ValidAfter"`
}

// ConsulCARoots contains the CA roots from Consul.
type ConsulCARoots struct {
	ActiveRootID string          `json:"ActiveRootID"`
	TrustDomain  string          `json:"TrustDomain"`
	Roots        []ConsulCARoot  `json:"Roots"`
}

// ConsulCARoot is a single CA root.
type ConsulCARoot struct {
	ID          string `json:"ID"`
	Name        string `json:"Name"`
	RootCert    string `json:"RootCert"`
	Active      bool   `json:"Active"`
	CreateIndex uint64 `json:"CreateIndex"`
	ModifyIndex uint64 `json:"ModifyIndex"`
}

// NewConsulProvider creates a new Consul Connect identity provider.
func NewConsulProvider(config *ConsulConfig) (*ConsulProvider, error) {
	if config == nil {
		config = DefaultConsulConfig()
	}

	return &ConsulProvider{
		config: config,
		client: &http.Client{
			Timeout: config.HTTPTimeout,
		},
		status: identity.ProviderStatusUnknown,
	}, nil
}

// Type returns the provider type.
func (p *ConsulProvider) Type() identity.ProviderType {
	return identity.ProviderTypeConsul
}

// Start starts the provider.
func (p *ConsulProvider) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = true
	p.mu.Unlock()

	// Detect Consul environment
	if err := p.detectEnvironment(ctx); err != nil {
		p.mu.Lock()
		p.status = identity.ProviderStatusUnhealthy
		p.statusMessage = err.Error()
		p.mu.Unlock()
		return err
	}

	// Load initial certificates
	if err := p.loadCertificates(ctx); err != nil {
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
func (p *ConsulProvider) Stop(ctx context.Context) error {
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
func (p *ConsulProvider) Health(ctx context.Context) identity.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// Info returns detailed provider information.
func (p *ConsulProvider) Info(ctx context.Context) identity.ProviderInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metadata := map[string]string{
		"http_addr": p.config.HTTPAddr,
	}
	if p.datacenter != "" {
		metadata["datacenter"] = p.datacenter
	}
	if p.config.ServiceName != "" {
		metadata["service_name"] = p.config.ServiceName
	}

	if p.currentSVID != nil {
		metadata["svid_expires"] = p.currentSVID.ExpiresAt.Format(time.RFC3339)
		metadata["spiffe_id"] = p.spiffeID.String()
	}

	return identity.ProviderInfo{
		Type:            identity.ProviderTypeConsul,
		Status:          p.status,
		TrustDomain:     p.trustDomain,
		Capabilities:    []string{"x509_svid", "trust_bundle", "connect"},
		LastHealthCheck: p.lastHealthCheck,
		ErrorMessage:    p.statusMessage,
		Metadata:        metadata,
	}
}

// TrustDomain returns the trust domain.
func (p *ConsulProvider) TrustDomain() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.config.TrustDomain != "" {
		return p.config.TrustDomain
	}
	return p.trustDomain
}

// GetTrustBundle returns the current trust bundle.
func (p *ConsulProvider) GetTrustBundle(ctx context.Context) (*identity.TrustBundle, error) {
	p.mu.RLock()
	bundle := p.trustBundle
	p.mu.RUnlock()

	if bundle != nil {
		return bundle, nil
	}

	// Fetch from Consul API
	return p.fetchCARoots(ctx)
}

// WatchTrustBundle watches for trust bundle updates.
func (p *ConsulProvider) WatchTrustBundle(ctx context.Context) (<-chan *identity.TrustBundle, error) {
	ch := make(chan *identity.TrustBundle, 1)

	// Send current bundle
	if bundle, err := p.GetTrustBundle(ctx); err == nil {
		select {
		case ch <- bundle:
		default:
		}
	}

	// Watch for updates
	go func() {
		ticker := time.NewTicker(p.config.RefreshInterval)
		defer ticker.Stop()
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if bundle, err := p.fetchCARoots(ctx); err == nil {
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
func (p *ConsulProvider) GetX509SVID(ctx context.Context) (*identity.X509SVID, error) {
	p.mu.RLock()
	svid := p.currentSVID
	p.mu.RUnlock()

	if svid != nil && !svid.Expired() && !svid.ShouldRotate() {
		return svid, nil
	}

	// Reload certificates
	if err := p.loadCertificates(ctx); err != nil {
		return nil, err
	}

	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.currentSVID, nil
}

// GetSPIFFEID returns the SPIFFE ID.
func (p *ConsulProvider) GetSPIFFEID() (identity.SPIFFEID, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.spiffeID.TrustDomain == "" {
		return identity.SPIFFEID{}, fmt.Errorf("SPIFFE ID not available")
	}
	return p.spiffeID, nil
}

// IsAvailable returns true if Consul Connect identity is available.
func (p *ConsulProvider) IsAvailable() bool {
	// Check for Consul agent
	req, err := http.NewRequest("GET", p.config.HTTPAddr+"/v1/agent/self", nil)
	if err != nil {
		return false
	}
	if p.config.Token != "" {
		req.Header.Set("X-Consul-Token", p.config.Token)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req = req.WithContext(ctx)

	resp, err := p.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// CreateAttestationEvidence creates attestation evidence for Consul.
func (p *ConsulProvider) CreateAttestationEvidence(ctx context.Context) (*identity.AttestationEvidence, error) {
	// Get leaf certificate as evidence
	svid, err := p.GetX509SVID(ctx)
	if err != nil {
		return nil, err
	}

	var certPEM []byte
	for _, cert := range svid.Certificates {
		certPEM = append(certPEM, cert.Raw...)
	}

	p.mu.RLock()
	metadata := map[string]string{
		"trust_domain": p.trustDomain,
	}
	if p.datacenter != "" {
		metadata["datacenter"] = p.datacenter
	}
	if p.config.ServiceName != "" {
		metadata["service_name"] = p.config.ServiceName
	}
	if p.spiffeID.TrustDomain != "" {
		metadata["spiffe_id"] = p.spiffeID.String()
	}
	p.mu.RUnlock()

	return &identity.AttestationEvidence{
		Type:     identity.AttestationTypeConsulConnect,
		Data:     certPEM,
		Metadata: metadata,
	}, nil
}

// Private methods

func (p *ConsulProvider) detectEnvironment(ctx context.Context) error {
	if !p.IsAvailable() {
		return fmt.Errorf("Consul agent not available")
	}

	// Get agent info
	req, err := http.NewRequestWithContext(ctx, "GET", p.config.HTTPAddr+"/v1/agent/self", nil)
	if err != nil {
		return err
	}
	if p.config.Token != "" {
		req.Header.Set("X-Consul-Token", p.config.Token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var agentResp ConsulAgentResponse
	if err := json.NewDecoder(resp.Body).Decode(&agentResp); err != nil {
		return err
	}

	p.mu.Lock()
	p.datacenter = agentResp.Config.Datacenter
	if p.config.Datacenter == "" {
		p.config.Datacenter = p.datacenter
	}
	p.mu.Unlock()

	return nil
}

func (p *ConsulProvider) loadCertificates(ctx context.Context) error {
	// First try file-based certificates
	if p.config.CertFile != "" && p.config.KeyFile != "" {
		return p.loadCertificatesFromFiles()
	}

	// Otherwise fetch from Consul API
	return p.fetchLeafCertificate(ctx)
}

func (p *ConsulProvider) loadCertificatesFromFiles() error {
	certPEM, err := os.ReadFile(p.config.CertFile)
	if err != nil {
		return fmt.Errorf("failed to read cert file: %w", err)
	}

	keyPEM, err := os.ReadFile(p.config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to read key file: %w", err)
	}

	certs, err := parsePEMCertificates(certPEM)
	if err != nil {
		return fmt.Errorf("failed to parse certificates: %w", err)
	}

	if len(certs) == 0 {
		return fmt.Errorf("no certificates in file")
	}

	key, err := parsePEMPrivateKey(keyPEM)
	if err != nil {
		return fmt.Errorf("failed to parse key: %w", err)
	}

	spiffeID, err := extractSPIFFEIDFromCert(certs[0])
	if err != nil {
		return fmt.Errorf("failed to extract SPIFFE ID: %w", err)
	}

	info, err := os.Stat(p.config.CertFile)
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

	return nil
}

func (p *ConsulProvider) fetchLeafCertificate(ctx context.Context) error {
	serviceName := p.config.ServiceName
	if serviceName == "" {
		return fmt.Errorf("service name required for API-based certificate fetch")
	}

	url := fmt.Sprintf("%s/v1/agent/connect/ca/leaf/%s", p.config.HTTPAddr, serviceName)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return err
	}
	if p.config.Token != "" {
		req.Header.Set("X-Consul-Token", p.config.Token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to fetch leaf certificate: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Consul returned status %d: %s", resp.StatusCode, string(body))
	}

	var leaf ConsulLeafCert
	if err := json.NewDecoder(resp.Body).Decode(&leaf); err != nil {
		return fmt.Errorf("failed to decode leaf certificate: %w", err)
	}

	certs, err := parsePEMCertificates([]byte(leaf.CertPEM))
	if err != nil {
		return fmt.Errorf("failed to parse leaf certificate: %w", err)
	}

	if len(certs) == 0 {
		return fmt.Errorf("no certificates in response")
	}

	key, err := parsePEMPrivateKey([]byte(leaf.PrivateKeyPEM))
	if err != nil {
		return fmt.Errorf("failed to parse private key: %w", err)
	}

	spiffeID, err := extractSPIFFEIDFromCert(certs[0])
	if err != nil {
		return fmt.Errorf("failed to extract SPIFFE ID: %w", err)
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
	p.serviceID = leaf.Service
	p.mu.Unlock()

	// Also load trust bundle
	if bundle, err := p.fetchCARoots(ctx); err == nil {
		p.mu.Lock()
		p.trustBundle = bundle
		p.mu.Unlock()
	}

	return nil
}

func (p *ConsulProvider) fetchCARoots(ctx context.Context) (*identity.TrustBundle, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", p.config.HTTPAddr+"/v1/agent/connect/ca/roots", nil)
	if err != nil {
		return nil, err
	}
	if p.config.Token != "" {
		req.Header.Set("X-Consul-Token", p.config.Token)
	}

	resp, err := p.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch CA roots: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Consul returned status %d: %s", resp.StatusCode, string(body))
	}

	var roots ConsulCARoots
	if err := json.NewDecoder(resp.Body).Decode(&roots); err != nil {
		return nil, fmt.Errorf("failed to decode CA roots: %w", err)
	}

	var certs []*x509.Certificate
	for _, root := range roots.Roots {
		if root.Active {
			rootCerts, err := parsePEMCertificates([]byte(root.RootCert))
			if err == nil {
				certs = append(certs, rootCerts...)
			}
		}
	}

	trustDomain := roots.TrustDomain
	if trustDomain == "" {
		p.mu.RLock()
		trustDomain = p.trustDomain
		p.mu.RUnlock()
	}

	return &identity.TrustBundle{
		TrustDomain:     trustDomain,
		X509Authorities: certs,
		UpdatedAt:       time.Now(),
	}, nil
}

func (p *ConsulProvider) healthCheckLoop(ctx context.Context) {
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

func (p *ConsulProvider) performHealthCheck() {
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

func (p *ConsulProvider) refreshLoop(ctx context.Context) {
	ticker := time.NewTicker(p.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.checkForCertUpdates(ctx)
		}
	}
}

func (p *ConsulProvider) checkForCertUpdates(ctx context.Context) {
	// If using file-based certs, check file modification time
	if p.config.CertFile != "" {
		info, err := os.Stat(p.config.CertFile)
		if err != nil {
			return
		}

		p.mu.RLock()
		modTime := p.certModTime
		p.mu.RUnlock()

		if info.ModTime().After(modTime) {
			p.loadCertificatesFromFiles()
		}
		return
	}

	// Otherwise check if SVID needs rotation
	p.mu.RLock()
	svid := p.currentSVID
	p.mu.RUnlock()

	if svid != nil && svid.ShouldRotate() {
		p.fetchLeafCertificate(ctx)
	}
}

// Verify ConsulProvider implements IdentityProvider
var _ identity.IdentityProvider = (*ConsulProvider)(nil)
