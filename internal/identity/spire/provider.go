package spire

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
)

// Provider implements the IdentityProvider interface using SPIRE.
type Provider struct {
	config *Config
	client *Client

	mu            sync.RWMutex
	started       bool
	status        identity.ProviderStatus
	statusMessage string
	trustBundle   *identity.TrustBundle

	// Callbacks
	onTrustBundleUpdate identity.TrustBundleUpdateCallback

	// Drop counters
	trustBundleDropped atomic.Int64

	// Health check
	healthCheckCancel context.CancelFunc
	lastHealthCheck   time.Time
}

// ProviderOption is a functional option for configuring the provider.
type ProviderOption func(*Provider)

// WithFallback enables fallback mode with the given configuration.
func WithFallback(config *FallbackConfig) ProviderOption {
	return func(p *Provider) {
		p.config.FallbackConfig = config
	}
}

// WithTrustDomain overrides the trust domain.
func WithTrustDomain(domain string) ProviderOption {
	return func(p *Provider) {
		p.config.TrustDomain = domain
	}
}

// NewProvider creates a new SPIRE identity provider.
func NewProvider(config *Config, opts ...ProviderOption) (*Provider, error) {
	if config == nil {
		config = DefaultConfig()
	} else {
		config.ApplyDefaults()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	p := &Provider{
		config: config,
		status: identity.ProviderStatusUnknown,
	}

	for _, opt := range opts {
		opt(p)
	}

	// Create client
	client, err := NewClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	p.client = client

	// Set up client state change callback
	p.client.OnStateChange(p.onClientStateChange)

	return p, nil
}

// Type returns the provider type.
func (p *Provider) Type() identity.ProviderType {
	return identity.ProviderTypeSPIRE
}

// Start starts the provider.
func (p *Provider) Start(ctx context.Context) error {
	p.mu.Lock()
	if p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = true
	p.mu.Unlock()

	// Connect to SPIRE Agent
	if err := p.client.Connect(ctx); err != nil {
		p.mu.Lock()
		p.status = identity.ProviderStatusUnhealthy
		p.statusMessage = err.Error()
		p.mu.Unlock()
		return err
	}

	// Fetch initial trust bundle
	bundle, err := p.client.FetchTrustBundle(ctx)
	if err != nil {
		p.mu.Lock()
		p.status = identity.ProviderStatusDegraded
		p.statusMessage = "connected but failed to fetch trust bundle: " + err.Error()
		p.mu.Unlock()
	} else {
		p.mu.Lock()
		p.trustBundle = bundle
		p.status = identity.ProviderStatusHealthy
		p.statusMessage = ""
		p.mu.Unlock()
	}

	// Start trust bundle watcher
	if err := p.client.WatchTrustBundle(ctx, p.handleTrustBundleUpdate); err != nil {
		// Non-fatal, we can still work without streaming
		p.mu.Lock()
		if p.status == identity.ProviderStatusHealthy {
			p.status = identity.ProviderStatusDegraded
			p.statusMessage = "streaming not available: " + err.Error()
		}
		p.mu.Unlock()
	}

	// Start health check loop - use WithoutCancel so it's not tied to Start()'s ctx lifecycle
	healthCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	p.mu.Lock()
	p.healthCheckCancel = cancel
	p.mu.Unlock()
	go p.healthCheckLoop(healthCtx)

	return nil
}

// Stop stops the provider.
func (p *Provider) Stop(ctx context.Context) error {
	p.mu.Lock()
	if !p.started {
		p.mu.Unlock()
		return nil
	}
	p.started = false

	if p.healthCheckCancel != nil {
		p.healthCheckCancel()
	}
	p.mu.Unlock()

	return p.client.Close()
}

// Health returns the current health status.
func (p *Provider) Health(ctx context.Context) identity.ProviderStatus {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// Info returns detailed provider information.
func (p *Provider) Info(ctx context.Context) identity.ProviderInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()

	metadata := map[string]string{
		"socket_path": p.config.SocketPath,
	}

	if p.trustBundle != nil {
		metadata["trust_domain"] = p.trustBundle.TrustDomain
		metadata["ca_count"] = fmt.Sprintf("%d", len(p.trustBundle.X509Authorities))
	}

	clientState := p.client.State()
	metadata["client_state"] = string(clientState)

	stats := p.client.Stats()
	metadata["connect_attempts"] = fmt.Sprintf("%d", stats.ConnectAttempts)
	metadata["fetch_svid_count"] = fmt.Sprintf("%d", stats.FetchSVIDCount)

	return identity.ProviderInfo{
		Type:            identity.ProviderTypeSPIRE,
		Status:          p.status,
		TrustDomain:     p.TrustDomain(),
		Capabilities:    []string{"x509_svid", "jwt_svid", "trust_bundle", "streaming"},
		LastHealthCheck: p.lastHealthCheck,
		ErrorMessage:    p.statusMessage,
		Metadata:        metadata,
	}
}

// TrustDomain returns the trust domain.
func (p *Provider) TrustDomain() string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	if p.config.TrustDomain != "" {
		return p.config.TrustDomain
	}
	if p.trustBundle != nil {
		return p.trustBundle.TrustDomain
	}
	return p.client.TrustDomain()
}

// GetTrustBundle returns the current trust bundle.
func (p *Provider) GetTrustBundle(ctx context.Context) (*identity.TrustBundle, error) {
	p.mu.RLock()
	bundle := p.trustBundle
	p.mu.RUnlock()

	if bundle != nil {
		return bundle, nil
	}

	// Fetch from SPIRE
	newBundle, err := p.client.FetchTrustBundle(ctx)
	if err != nil {
		return nil, err
	}

	p.mu.Lock()
	p.trustBundle = newBundle
	p.mu.Unlock()

	return newBundle, nil
}

// WatchTrustBundle watches for trust bundle updates.
func (p *Provider) WatchTrustBundle(ctx context.Context) (<-chan *identity.TrustBundle, error) {
	ch := make(chan *identity.TrustBundle, p.config.StreamBufferSize)

	// Register callback
	p.mu.Lock()
	oldCallback := p.onTrustBundleUpdate
	p.onTrustBundleUpdate = func(bundle *identity.TrustBundle) {
		select {
		case ch <- bundle:
		default:
			dropped := p.trustBundleDropped.Add(1)
			log.Printf("WARN: trust bundle update dropped (channel full, total: %d)", dropped)
		}
		if oldCallback != nil {
			oldCallback(bundle)
		}
	}
	p.mu.Unlock()

	// Send current bundle
	if bundle := p.trustBundle; bundle != nil {
		select {
		case ch <- bundle:
		default:
			p.trustBundleDropped.Add(1)
		}
	}

	return ch, nil
}

// IssueX509SVID issues an X.509 SVID.
// For SPIRE, this fetches the SVID from the agent (workload attestation required).
func (p *Provider) IssueX509SVID(ctx context.Context, req *identity.X509SVIDRequest) (*identity.X509SVID, error) {
	// SPIRE doesn't allow direct SVID issuance - it uses workload attestation
	// We fetch the SVID from the agent, which has already attested this workload
	return p.client.FetchX509SVID(ctx)
}

// IssueJWTSVID issues a JWT SVID.
func (p *Provider) IssueJWTSVID(ctx context.Context, req *identity.JWTSVIDRequest) (*identity.JWTSVID, error) {
	return p.client.FetchJWTSVID(ctx, req.Audience)
}

// RenewX509SVID renews an X.509 SVID.
func (p *Provider) RenewX509SVID(ctx context.Context, current *identity.X509SVID) (*identity.X509SVID, error) {
	// For SPIRE, renewal is just fetching a new SVID
	return p.client.FetchX509SVID(ctx)
}

// Client returns the underlying SPIRE client.
func (p *Provider) Client() *Client {
	return p.client
}

// Private methods

func (p *Provider) onClientStateChange(oldState, newState ClientState) {
	p.mu.Lock()
	defer p.mu.Unlock()

	switch newState {
	case ClientStateConnected:
		p.status = identity.ProviderStatusHealthy
		p.statusMessage = ""
	case ClientStateFallback:
		p.status = identity.ProviderStatusDegraded
		p.statusMessage = "using fallback mode"
	case ClientStateDisconnected, ClientStateReconnecting:
		p.status = identity.ProviderStatusUnhealthy
		p.statusMessage = "disconnected from SPIRE Agent"
	case ClientStateClosed:
		p.status = identity.ProviderStatusUnknown
		p.statusMessage = "client closed"
	default:
		// ClientStateConnecting - transitional state, no status update
	}
}

func (p *Provider) handleTrustBundleUpdate(bundle *identity.TrustBundle) {
	p.mu.Lock()
	p.trustBundle = bundle
	callback := p.onTrustBundleUpdate
	p.mu.Unlock()

	if callback != nil {
		callback(bundle)
	}
}

func (p *Provider) healthCheckLoop(ctx context.Context) {
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.performHealthCheck(ctx)
		}
	}
}

func (p *Provider) performHealthCheck(ctx context.Context) {
	checkCtx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	healthy, err := p.client.Health(checkCtx)

	p.mu.Lock()
	p.lastHealthCheck = time.Now()

	if healthy {
		p.status = identity.ProviderStatusHealthy
		p.statusMessage = ""
	} else {
		// Check if we're in fallback mode
		if p.client.State() == ClientStateFallback {
			p.status = identity.ProviderStatusDegraded
			p.statusMessage = "using fallback mode"
		} else {
			p.status = identity.ProviderStatusUnhealthy
			if err != nil {
				p.statusMessage = err.Error()
			}
		}
	}
	p.mu.Unlock()
}

// Verify Provider implements identity.Provider
var _ identity.Provider = (*Provider)(nil)
