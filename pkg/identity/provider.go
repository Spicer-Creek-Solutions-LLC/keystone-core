package identity

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// EmbeddedProvider is the built-in identity provider for Keystone Core.
// It provides zero-configuration SPIFFE identity out of the box.
type EmbeddedProvider struct {
	config *Config
	ca     *CAManager
	issuer *SVIDIssuerService
	engine *AttestationEngine
	tokens JoinTokenStore

	trustBundle     *TrustBundle
	trustBundleMu   sync.RWMutex
	bundleWatchers  []chan *TrustBundle
	bundleWatcherMu sync.Mutex

	status   ProviderStatus
	statusMu sync.RWMutex

	started bool
	stopCh  chan struct{}
	wg      sync.WaitGroup
}

// NewEmbeddedProvider creates a new embedded identity provider.
func NewEmbeddedProvider(config *Config) (*EmbeddedProvider, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	p := &EmbeddedProvider{
		config:         config,
		status:         ProviderStatusUnknown,
		bundleWatchers: make([]chan *TrustBundle, 0),
		stopCh:         make(chan struct{}),
	}

	return p, nil
}

// Type returns the provider type.
func (p *EmbeddedProvider) Type() ProviderType {
	return ProviderTypeEmbedded
}

// Start initializes and starts the provider.
func (p *EmbeddedProvider) Start(ctx context.Context) error {
	if p.started {
		return fmt.Errorf("provider already started")
	}

	// Initialize CA manager
	caConfig := &CAManagerConfig{
		KeyType:               p.config.CA.KeyType,
		RootCATTL:             p.config.CA.RootCATTL,
		SigningCATTL:          p.config.CA.SigningCATTL,
		RotateSigningCABefore: p.config.CA.RotateSigningCABefore,
		StoragePath:           p.config.CA.StoragePath,
		EncryptionKey:         p.config.CA.EncryptionKey,
		TrustDomain:           p.config.TrustDomain,
		Subject:               p.config.CA.Subject,
	}

	ca, err := NewCAManager(caConfig)
	if err != nil {
		return fmt.Errorf("failed to create CA manager: %w", err)
	}

	if err := ca.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize CA: %w", err)
	}

	p.ca = ca

	// Initialize token store
	p.tokens = NewInMemoryTokenStore()

	// Initialize attestation engine
	engineConfig := &AttestationEngineConfig{
		TrustDomain:      p.config.TrustDomain,
		AllowedAttestors: p.config.Attestation.AllowedAttestors,
		AllowNone:        p.config.Attestation.AllowNone,
		JoinTokenStore:   p.tokens,
	}

	engine, err := NewAttestationEngine(engineConfig)
	if err != nil {
		return fmt.Errorf("failed to create attestation engine: %w", err)
	}

	p.engine = engine

	// Initialize SVID issuer
	issuerConfig := &SVIDIssuerConfig{
		CA:          p.ca,
		TrustDomain: p.config.TrustDomain,
		DefaultTTL:  p.config.SVID.DefaultTTL,
		MaxTTL:      p.config.SVID.MaxTTL,
	}

	issuer, err := NewSVIDIssuerService(issuerConfig)
	if err != nil {
		return fmt.Errorf("failed to create SVID issuer: %w", err)
	}

	p.issuer = issuer

	// Build initial trust bundle
	bundle, err := p.buildTrustBundle()
	if err != nil {
		return fmt.Errorf("failed to build trust bundle: %w", err)
	}
	p.trustBundle = bundle

	// Start background tasks
	p.wg.Add(1)
	go p.runBackgroundTasks()

	p.started = true
	p.setStatus(ProviderStatusHealthy)

	return nil
}

// Stop gracefully shuts down the provider.
func (p *EmbeddedProvider) Stop(ctx context.Context) error {
	if !p.started {
		return nil
	}

	close(p.stopCh)

	// Wait for background tasks with timeout
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		return ctx.Err()
	}

	// Close all bundle watchers
	p.bundleWatcherMu.Lock()
	for _, ch := range p.bundleWatchers {
		close(ch)
	}
	p.bundleWatchers = nil
	p.bundleWatcherMu.Unlock()

	p.started = false
	p.setStatus(ProviderStatusUnknown)

	return nil
}

// Health returns the current health status.
func (p *EmbeddedProvider) Health(ctx context.Context) ProviderStatus {
	p.statusMu.RLock()
	defer p.statusMu.RUnlock()
	return p.status
}

// Info returns detailed information about the provider.
func (p *EmbeddedProvider) Info(ctx context.Context) ProviderInfo {
	p.statusMu.RLock()
	status := p.status
	p.statusMu.RUnlock()

	info := ProviderInfo{
		Type:            ProviderTypeEmbedded,
		Status:          status,
		TrustDomain:     p.config.TrustDomain,
		LastHealthCheck: time.Now(),
		Capabilities: []string{
			"x509_svid",
			"jwt_svid",
			"trust_bundle",
			"join_token",
		},
		Metadata: map[string]string{
			"key_type":        p.config.CA.KeyType,
			"svid_default_ttl": p.config.SVID.DefaultTTL.String(),
		},
	}

	if p.ca != nil {
		caInfo := p.ca.Info()
		info.Metadata["root_ca_expires"] = caInfo.RootCAExpires.Format(time.RFC3339)
		info.Metadata["signing_ca_expires"] = caInfo.SigningCAExpires.Format(time.RFC3339)
	}

	return info
}

// TrustDomain returns the trust domain managed by this provider.
func (p *EmbeddedProvider) TrustDomain() string {
	return p.config.TrustDomain
}

// GetTrustBundle returns the current trust bundle.
func (p *EmbeddedProvider) GetTrustBundle(ctx context.Context) (*TrustBundle, error) {
	p.trustBundleMu.RLock()
	defer p.trustBundleMu.RUnlock()

	if p.trustBundle == nil {
		return nil, fmt.Errorf("trust bundle not available")
	}

	return p.trustBundle, nil
}

// WatchTrustBundle watches for trust bundle updates.
func (p *EmbeddedProvider) WatchTrustBundle(ctx context.Context) (<-chan *TrustBundle, error) {
	ch := make(chan *TrustBundle, 1)

	p.bundleWatcherMu.Lock()
	p.bundleWatchers = append(p.bundleWatchers, ch)
	p.bundleWatcherMu.Unlock()

	// Send current bundle immediately
	p.trustBundleMu.RLock()
	if p.trustBundle != nil {
		select {
		case ch <- p.trustBundle:
		default:
		}
	}
	p.trustBundleMu.RUnlock()

	return ch, nil
}

// Attest performs attestation and returns the result.
func (p *EmbeddedProvider) Attest(ctx context.Context, evidence *AttestationEvidence) (*AttestationResult, error) {
	if p.engine == nil {
		return nil, fmt.Errorf("provider not started")
	}
	return p.engine.Attest(ctx, evidence)
}

// IssueX509SVID issues an X.509 SVID for the given request.
func (p *EmbeddedProvider) IssueX509SVID(ctx context.Context, req *X509SVIDRequest) (*X509SVID, error) {
	if p.issuer == nil {
		return nil, fmt.Errorf("provider not started")
	}
	return p.issuer.IssueX509SVID(ctx, req)
}

// IssueJWTSVID issues a JWT SVID for the given request.
func (p *EmbeddedProvider) IssueJWTSVID(ctx context.Context, req *JWTSVIDRequest) (*JWTSVID, error) {
	if p.issuer == nil {
		return nil, fmt.Errorf("provider not started")
	}
	return p.issuer.IssueJWTSVID(ctx, req)
}

// RenewX509SVID renews an existing X.509 SVID.
func (p *EmbeddedProvider) RenewX509SVID(ctx context.Context, current *X509SVID) (*X509SVID, error) {
	if p.issuer == nil {
		return nil, fmt.Errorf("provider not started")
	}
	return p.issuer.RenewX509SVID(ctx, current)
}

// CreateJoinToken creates a new join token.
func (p *EmbeddedProvider) CreateJoinToken(ctx context.Context, opts *JoinTokenOptions) (*JoinToken, error) {
	if p.tokens == nil {
		return nil, fmt.Errorf("provider not started")
	}

	if opts == nil {
		opts = &JoinTokenOptions{}
	}

	ttl := opts.TTL
	if ttl == 0 {
		ttl = p.config.Attestation.JoinToken.DefaultTTL
	}
	if ttl > p.config.Attestation.JoinToken.MaxTTL {
		ttl = p.config.Attestation.JoinToken.MaxTTL
	}

	token := &JoinToken{
		Token:     generateToken(p.config.Attestation.JoinToken.TokenLength),
		AgentID:   opts.AgentID,
		ExpiresAt: time.Now().Add(ttl),
		CreatedAt: time.Now(),
		Metadata:  opts.Metadata,
	}

	if err := p.tokens.Create(ctx, token); err != nil {
		return nil, fmt.Errorf("failed to create token: %w", err)
	}

	return token, nil
}

// JoinTokenOptions contains options for creating a join token.
type JoinTokenOptions struct {
	// TTL is the token time-to-live.
	TTL time.Duration

	// AgentID is the expected agent ID (optional).
	AgentID string

	// Metadata contains additional token metadata.
	Metadata map[string]string
}

// ListJoinTokens lists all join tokens.
func (p *EmbeddedProvider) ListJoinTokens(ctx context.Context) ([]*JoinToken, error) {
	if p.tokens == nil {
		return nil, fmt.Errorf("provider not started")
	}
	return p.tokens.List(ctx)
}

// DeleteJoinToken deletes a join token.
func (p *EmbeddedProvider) DeleteJoinToken(ctx context.Context, tokenValue string) error {
	if p.tokens == nil {
		return fmt.Errorf("provider not started")
	}
	return p.tokens.Delete(ctx, tokenValue)
}

// runBackgroundTasks runs periodic background tasks.
func (p *EmbeddedProvider) runBackgroundTasks() {
	defer p.wg.Done()

	// CA rotation check interval
	caRotationTicker := time.NewTicker(1 * time.Hour)
	defer caRotationTicker.Stop()

	// Token cleanup interval
	tokenCleanupTicker := time.NewTicker(p.config.Attestation.JoinToken.CleanupInterval)
	defer tokenCleanupTicker.Stop()

	// Trust bundle refresh interval
	bundleRefreshTicker := time.NewTicker(5 * time.Minute)
	defer bundleRefreshTicker.Stop()

	for {
		select {
		case <-p.stopCh:
			return

		case <-caRotationTicker.C:
			p.checkCARotation()

		case <-tokenCleanupTicker.C:
			p.cleanupTokens()

		case <-bundleRefreshTicker.C:
			p.refreshTrustBundle()
		}
	}
}

// checkCARotation checks if the signing CA needs rotation.
func (p *EmbeddedProvider) checkCARotation() {
	if p.ca == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if p.ca.ShouldRotateSigningCA() {
		if err := p.ca.RotateSigningCA(ctx); err != nil {
			p.setStatus(ProviderStatusDegraded)
			return
		}

		// Update trust bundle after CA rotation
		p.refreshTrustBundle()
	}
}

// cleanupTokens removes expired and used tokens.
func (p *EmbeddedProvider) cleanupTokens() {
	if p.tokens == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	_, _ = p.tokens.Cleanup(ctx)
}

// refreshTrustBundle refreshes the trust bundle.
func (p *EmbeddedProvider) refreshTrustBundle() {
	bundle, err := p.buildTrustBundle()
	if err != nil {
		return
	}

	p.trustBundleMu.Lock()
	oldSequence := uint64(0)
	if p.trustBundle != nil {
		oldSequence = p.trustBundle.SequenceNumber
	}
	bundle.SequenceNumber = oldSequence + 1
	p.trustBundle = bundle
	p.trustBundleMu.Unlock()

	// Notify watchers
	p.notifyBundleWatchers(bundle)
}

// buildTrustBundle builds the trust bundle from the CA.
func (p *EmbeddedProvider) buildTrustBundle() (*TrustBundle, error) {
	if p.ca == nil {
		return nil, fmt.Errorf("CA not initialized")
	}

	certs := p.ca.GetTrustChain()
	if len(certs) == 0 {
		return nil, fmt.Errorf("no CA certificates available")
	}

	return &TrustBundle{
		TrustDomain:     p.config.TrustDomain,
		X509Authorities: certs,
		RefreshHint:     5 * time.Minute,
		UpdatedAt:       time.Now(),
	}, nil
}

// notifyBundleWatchers notifies all bundle watchers of an update.
func (p *EmbeddedProvider) notifyBundleWatchers(bundle *TrustBundle) {
	p.bundleWatcherMu.Lock()
	defer p.bundleWatcherMu.Unlock()

	for _, ch := range p.bundleWatchers {
		select {
		case ch <- bundle:
		default:
			// Watcher not ready, skip
		}
	}
}

// setStatus sets the provider status.
func (p *EmbeddedProvider) setStatus(status ProviderStatus) {
	p.statusMu.Lock()
	defer p.statusMu.Unlock()
	p.status = status
}

// CAManager returns the CA manager (for testing/advanced use).
func (p *EmbeddedProvider) CAManager() *CAManager {
	return p.ca
}

// AttestationEngine returns the attestation engine (for testing/advanced use).
func (p *EmbeddedProvider) AttestationEngine() *AttestationEngine {
	return p.engine
}
