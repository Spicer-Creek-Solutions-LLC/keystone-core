package identity

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"time"
)

// AgentIdentityClientConfig contains configuration for the agent identity client.
type AgentIdentityClientConfig struct {
	// ProviderAddress is the address of the identity provider.
	// For embedded provider, this should be empty or "embedded".
	ProviderAddress string

	// TrustDomain is the expected trust domain.
	TrustDomain string

	// AgentID is the agent's identifier.
	AgentID string

	// AttestationType is the type of attestation to use.
	AttestationType string

	// AttestationData is the attestation evidence data.
	AttestationData []byte

	// AttestationMetadata is additional attestation metadata.
	AttestationMetadata map[string]string

	// RotationInterval is how often to check for SVID rotation.
	RotationInterval time.Duration

	// GracePeriod is how long to keep using an expired SVID.
	GracePeriod time.Duration

	// RetryInterval is how long to wait before retrying failed operations.
	RetryInterval time.Duration

	// MaxRetries is the maximum number of retries for failed operations.
	MaxRetries int
}

// AgentIdentityClient manages identity for an agent.
type AgentIdentityClient struct {
	config *AgentIdentityClientConfig

	// Current SVID and trust bundle
	currentSVID *X509SVID
	trustBundle *TrustBundle
	tlsConfig   *tls.Config
	mu          sync.RWMutex

	// For embedded provider
	embeddedProvider *EmbeddedProvider

	// Callbacks
	rotationCallbacks []SVIDRotationCallback
	bundleCallbacks   []TrustBundleUpdateCallback
	callbackMu        sync.Mutex

	// State
	connected bool
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// NewAgentIdentityClient creates a new agent identity client.
func NewAgentIdentityClient(config *AgentIdentityClientConfig) (*AgentIdentityClient, error) {
	if config == nil {
		return nil, fmt.Errorf("config required")
	}

	// Set defaults
	if config.RotationInterval == 0 {
		config.RotationInterval = 30 * time.Second
	}
	if config.GracePeriod == 0 {
		config.GracePeriod = 10 * time.Minute
	}
	if config.RetryInterval == 0 {
		config.RetryInterval = 5 * time.Second
	}
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}

	return &AgentIdentityClient{
		config:            config,
		rotationCallbacks: make([]SVIDRotationCallback, 0),
		bundleCallbacks:   make([]TrustBundleUpdateCallback, 0),
		stopCh:            make(chan struct{}),
	}, nil
}

// Connect connects to the identity provider and obtains initial identity.
func (c *AgentIdentityClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.connected {
		return nil
	}

	// For embedded provider, we need a reference to the provider
	if c.config.ProviderAddress == "" || c.config.ProviderAddress == "embedded" {
		// Embedded mode - provider must be set via SetEmbeddedProvider
		if c.embeddedProvider == nil {
			return fmt.Errorf("embedded provider not set")
		}
	}

	// Perform attestation
	evidence := &AttestationEvidence{
		Type:     c.config.AttestationType,
		Data:     c.config.AttestationData,
		Metadata: c.config.AttestationMetadata,
	}

	if evidence.Metadata == nil {
		evidence.Metadata = make(map[string]string)
	}
	evidence.Metadata["agent_id"] = c.config.AgentID

	result, err := c.embeddedProvider.Attest(ctx, evidence)
	if err != nil {
		return fmt.Errorf("attestation failed: %w", err)
	}

	if !result.Success {
		return fmt.Errorf("attestation denied: %s", result.Error)
	}

	// Request SVID
	svid, err := c.embeddedProvider.IssueX509SVID(ctx, &X509SVIDRequest{
		SPIFFEID: result.SPIFFEID,
	})
	if err != nil {
		return fmt.Errorf("failed to obtain SVID: %w", err)
	}

	c.currentSVID = svid

	// Get trust bundle
	bundle, err := c.embeddedProvider.GetTrustBundle(ctx)
	if err != nil {
		return fmt.Errorf("failed to get trust bundle: %w", err)
	}

	c.trustBundle = bundle

	// Build TLS config
	c.tlsConfig = c.buildTLSConfig()

	// Start background rotation
	c.wg.Add(1)
	go c.runRotation() //nolint:contextcheck // background loop uses internal context

	c.connected = true
	return nil
}

// Close closes the client connection.
func (c *AgentIdentityClient) Close() error {
	c.mu.Lock()
	if !c.connected {
		c.mu.Unlock()
		return nil
	}
	c.connected = false
	c.mu.Unlock()

	close(c.stopCh)
	c.wg.Wait()

	return nil
}

// SetEmbeddedProvider sets the embedded provider for direct access.
func (c *AgentIdentityClient) SetEmbeddedProvider(provider *EmbeddedProvider) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.embeddedProvider = provider
}

// FetchX509SVID returns the current X.509 SVID.
func (c *AgentIdentityClient) FetchX509SVID(ctx context.Context) (*X509SVID, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.currentSVID == nil {
		return nil, fmt.Errorf("not connected")
	}

	return c.currentSVID, nil
}

// FetchJWTSVID fetches a JWT SVID for the given audience.
func (c *AgentIdentityClient) FetchJWTSVID(ctx context.Context, audience []string) (*JWTSVID, error) {
	c.mu.RLock()
	svid := c.currentSVID
	provider := c.embeddedProvider
	c.mu.RUnlock()

	if svid == nil || provider == nil {
		return nil, fmt.Errorf("not connected")
	}

	return provider.IssueJWTSVID(ctx, &JWTSVIDRequest{
		SPIFFEID: svid.SPIFFEID,
		Audience: audience,
	})
}

// FetchTrustBundle returns the current trust bundle.
func (c *AgentIdentityClient) FetchTrustBundle(ctx context.Context) (*TrustBundle, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.trustBundle == nil {
		return nil, fmt.Errorf("not connected")
	}

	return c.trustBundle, nil
}

// GetTLSConfig returns a TLS config using the current SVID.
func (c *AgentIdentityClient) GetTLSConfig() *tls.Config {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.tlsConfig
}

// WatchX509SVID registers a callback for SVID rotation.
func (c *AgentIdentityClient) WatchX509SVID(ctx context.Context, callback SVIDRotationCallback) error {
	c.callbackMu.Lock()
	c.rotationCallbacks = append(c.rotationCallbacks, callback)
	c.callbackMu.Unlock()

	// Provide current SVID immediately
	c.mu.RLock()
	svid := c.currentSVID
	c.mu.RUnlock()

	if svid != nil {
		callback(nil, svid)
	}

	return nil
}

// WatchTrustBundle registers a callback for trust bundle updates.
func (c *AgentIdentityClient) WatchTrustBundle(ctx context.Context, callback TrustBundleUpdateCallback) error {
	c.callbackMu.Lock()
	c.bundleCallbacks = append(c.bundleCallbacks, callback)
	c.callbackMu.Unlock()

	// Provide current bundle immediately
	c.mu.RLock()
	bundle := c.trustBundle
	c.mu.RUnlock()

	if bundle != nil {
		callback(bundle)
	}

	return nil
}

// GetSPIFFEID returns the current SPIFFE ID.
func (c *AgentIdentityClient) GetSPIFFEID() (SPIFFEID, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if c.currentSVID == nil {
		return SPIFFEID{}, fmt.Errorf("not connected")
	}

	return c.currentSVID.SPIFFEID, nil
}

// runRotation runs the background SVID rotation loop.
func (c *AgentIdentityClient) runRotation() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.RotationInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return

		case <-ticker.C:
			c.checkRotation()
		}
	}
}

// checkRotation checks if the SVID needs rotation and rotates if necessary.
func (c *AgentIdentityClient) checkRotation() {
	c.mu.RLock()
	svid := c.currentSVID
	provider := c.embeddedProvider
	c.mu.RUnlock()

	if svid == nil || provider == nil {
		return
	}

	// Check if rotation is needed
	if !svid.ShouldRotate() {
		return
	}

	// Attempt rotation with retries
	var newSVID *X509SVID
	var err error

	for i := 0; i < c.config.MaxRetries; i++ {
		attemptCtx, attemptCancel := context.WithTimeout(context.Background(), 30*time.Second)
		newSVID, err = provider.RenewX509SVID(attemptCtx, svid)
		attemptCancel()

		if err == nil {
			break
		}

		if !waitForRetryOrStop(c.stopCh, c.config.RetryInterval) {
			return
		}
	}

	if err != nil {
		// Log error but continue with current SVID
		return
	}

	// Update SVID
	c.mu.Lock()
	oldSVID := c.currentSVID
	c.currentSVID = newSVID
	c.tlsConfig = c.buildTLSConfig()
	c.mu.Unlock()

	// Notify callbacks
	c.notifyRotationCallbacks(oldSVID, newSVID)
}

func waitForRetryOrStop(stopCh <-chan struct{}, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-stopCh:
		return false
	case <-timer.C:
		return true
	}
}

// buildTLSConfig builds a TLS config using the current SVID and trust bundle.
func (c *AgentIdentityClient) buildTLSConfig() *tls.Config {
	if c.currentSVID == nil || c.trustBundle == nil {
		return nil
	}

	// Build certificate chain
	cert := tls.Certificate{
		PrivateKey: c.currentSVID.PrivateKey,
	}
	for _, c := range c.currentSVID.Certificates {
		cert.Certificate = append(cert.Certificate, c.Raw)
	}

	// Build CA pool
	caPool := x509.NewCertPool()
	for _, ca := range c.trustBundle.X509Authorities {
		caPool.AddCert(ca)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caPool,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS12,
		// Verify peer certificates
		ClientAuth: tls.RequireAndVerifyClientCert,
	}
}

// notifyRotationCallbacks notifies all rotation callbacks.
func (c *AgentIdentityClient) notifyRotationCallbacks(oldSVID, newSVID *X509SVID) {
	c.callbackMu.Lock()
	callbacks := make([]SVIDRotationCallback, len(c.rotationCallbacks))
	copy(callbacks, c.rotationCallbacks)
	c.callbackMu.Unlock()

	for _, cb := range callbacks {
		cb(oldSVID, newSVID)
	}
}

// AwareTLSConfig creates a TLS config that automatically refreshes
// with the latest SVID and trust bundle.
type AwareTLSConfig struct {
	client *AgentIdentityClient
}

// NewAwareTLSConfig creates a new identity-aware TLS config.
func NewAwareTLSConfig(client *AgentIdentityClient) *AwareTLSConfig {
	return &AwareTLSConfig{client: client}
}

// GetClientConfig returns a TLS config for client connections.
func (c *AwareTLSConfig) GetClientConfig() *tls.Config {
	baseConfig := c.client.GetTLSConfig()
	if baseConfig == nil {
		return nil
	}

	// Create a copy with dynamic certificate retrieval
	return &tls.Config{
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			config := c.client.GetTLSConfig()
			if config == nil || len(config.Certificates) == 0 {
				return nil, fmt.Errorf("no certificate available")
			}
			return &config.Certificates[0], nil
		},
		RootCAs:    baseConfig.RootCAs,
		MinVersion: tls.VersionTLS12,
	}
}

// GetServerConfig returns a TLS config for server connections.
func (c *AwareTLSConfig) GetServerConfig() *tls.Config {
	baseConfig := c.client.GetTLSConfig()
	if baseConfig == nil {
		return nil
	}

	return &tls.Config{
		GetCertificate: func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
			config := c.client.GetTLSConfig()
			if config == nil || len(config.Certificates) == 0 {
				return nil, fmt.Errorf("no certificate available")
			}
			return &config.Certificates[0], nil
		},
		ClientCAs:  baseConfig.ClientCAs,
		ClientAuth: tls.RequireAndVerifyClientCert,
		MinVersion: tls.VersionTLS12,
	}
}
