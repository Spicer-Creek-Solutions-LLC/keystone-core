package spire

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"math/rand" // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- jitter does not require crypto randomness
	"net"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/identity"
	"github.com/shawnbutts/keystone-core/pkg/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientState represents the state of the SPIRE client.
type ClientState string

// ClientStateDisconnected constants define the possible states.
const (
	ClientStateDisconnected ClientState = "disconnected"
	ClientStateConnecting   ClientState = "connecting"
	ClientStateConnected    ClientState = "connected"
	ClientStateReconnecting ClientState = "reconnecting"
	ClientStateFallback     ClientState = "fallback"
	ClientStateClosed       ClientState = "closed"
)

// Client is a SPIRE Workload API client.
// It connects to a SPIRE Agent to obtain X.509 SVIDs, JWT SVIDs, and trust bundles.
type Client struct {
	config *Config

	mu          sync.RWMutex
	state       ClientState
	conn        *grpc.ClientConn
	trustDomain string

	// Current SVID and trust bundle (cached)
	currentSVID   *identity.X509SVID
	trustBundle   *identity.TrustBundle
	lastFetchTime time.Time

	// Streaming
	x509SVIDChan       chan *identity.X509SVID
	trustBundleChan    chan *identity.TrustBundle
	x509StreamCancel   context.CancelFunc
	bundleStreamCancel context.CancelFunc

	// Callbacks
	onStateChange  func(oldState, newState ClientState)
	onSVIDRotation identity.SVIDRotationCallback
	onBundleUpdate identity.TrustBundleUpdateCallback

	// Health
	lastHealthCheck time.Time
	healthStatus    bool
	healthError     error

	// Metrics
	stats ClientStats
}

// ClientStats contains statistics about the client.
type ClientStats struct {
	ConnectAttempts   int64
	ConnectSuccesses  int64
	ConnectFailures   int64
	FetchSVIDCount    int64
	FetchSVIDErrors   int64
	FetchBundleCount  int64
	FetchBundleErrors int64
	StreamRestarts    int64
	FallbackCount     int64
	TotalLatencyMs    int64
	LastConnectTime   time.Time
	LastFetchTime     time.Time
}

// NewClient creates a new SPIRE Workload API client.
func NewClient(config *Config) (*Client, error) {
	if config == nil {
		config = DefaultConfig()
	} else {
		config.ApplyDefaults()
	}

	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	return &Client{
		config:          config,
		state:           ClientStateDisconnected,
		x509SVIDChan:    make(chan *identity.X509SVID, config.StreamBufferSize),
		trustBundleChan: make(chan *identity.TrustBundle, config.StreamBufferSize),
	}, nil
}

// Connect connects to the SPIRE Agent.
func (c *Client) Connect(ctx context.Context) error {
	c.mu.Lock()
	if c.state == ClientStateConnected {
		c.mu.Unlock()
		return nil
	}
	if c.state == ClientStateClosed {
		c.mu.Unlock()
		return fmt.Errorf("client is closed")
	}

	oldState := c.state
	c.state = ClientStateConnecting
	c.stats.ConnectAttempts++
	c.mu.Unlock()

	c.notifyStateChange(oldState, ClientStateConnecting)

	// Create gRPC connection using Unix socket
	dialCtx, cancel := context.WithTimeout(ctx, c.config.DialTimeout)
	defer cancel()

	// Use Unix socket dialer
	dialer := func(ctx context.Context, addr string) (net.Conn, error) {
		d := &net.Dialer{Timeout: c.config.DialTimeout}
		return d.DialContext(ctx, "unix", c.config.SocketPath)
	}

	conn, err := grpc.DialContext(dialCtx, "unix://"+c.config.SocketPath, //nolint:staticcheck // SA1019: grpc.DialContext is deprecated but supported throughout gRPC 1.x; migration to NewClient requires significant refactoring
		grpc.WithContextDialer(dialer),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithBlock(), //nolint:staticcheck // SA1019: grpc.WithBlock is deprecated but supported throughout gRPC 1.x
	)
	if err != nil {
		c.mu.Lock()
		c.state = ClientStateDisconnected
		c.stats.ConnectFailures++
		c.mu.Unlock()
		c.notifyStateChange(ClientStateConnecting, ClientStateDisconnected)

		// Try fallback if configured
		if c.config.FallbackConfig != nil && c.config.FallbackConfig.Enabled {
			return c.enterFallbackMode(ctx, err)
		}
		return fmt.Errorf("failed to connect to SPIRE Agent at %s: %w", c.config.SocketPath, err)
	}

	c.mu.Lock()
	c.conn = conn
	c.state = ClientStateConnected
	c.stats.ConnectSuccesses++
	c.stats.LastConnectTime = time.Now()
	c.mu.Unlock()

	c.notifyStateChange(ClientStateConnecting, ClientStateConnected)

	return nil
}

// Close closes the client connection.
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.state == ClientStateClosed {
		return nil
	}

	oldState := c.state
	c.state = ClientStateClosed

	// Cancel any active streams
	if c.x509StreamCancel != nil {
		c.x509StreamCancel()
	}
	if c.bundleStreamCancel != nil {
		c.bundleStreamCancel()
	}

	// Close channels
	close(c.x509SVIDChan)
	close(c.trustBundleChan)

	// Close gRPC connection
	var err error
	if c.conn != nil {
		err = c.conn.Close()
		c.conn = nil
	}

	go c.notifyStateChange(oldState, ClientStateClosed)

	return err
}

// State returns the current client state.
func (c *Client) State() ClientState {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state
}

// Stats returns client statistics.
func (c *Client) Stats() ClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// FetchX509SVID fetches an X.509 SVID from the SPIRE Agent.
func (c *Client) FetchX509SVID(ctx context.Context) (*identity.X509SVID, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.stats.FetchSVIDCount++
	c.mu.Unlock()

	start := time.Now()

	svid, err := c.fetchX509SVIDWithRetry(ctx)
	if err != nil {
		c.mu.Lock()
		c.stats.FetchSVIDErrors++
		c.mu.Unlock()
		return nil, err
	}

	c.mu.Lock()
	c.currentSVID = svid
	c.lastFetchTime = time.Now()
	c.stats.LastFetchTime = c.lastFetchTime
	c.stats.TotalLatencyMs += time.Since(start).Milliseconds()
	c.mu.Unlock()

	return svid, nil
}

// FetchJWTSVID fetches a JWT SVID from the SPIRE Agent.
func (c *Client) FetchJWTSVID(ctx context.Context, audience []string) (*identity.JWTSVID, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	return c.fetchJWTSVIDWithRetry(ctx, audience)
}

// FetchTrustBundle fetches the trust bundle from the SPIRE Agent.
func (c *Client) FetchTrustBundle(ctx context.Context) (*identity.TrustBundle, error) {
	if err := c.ensureConnected(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.stats.FetchBundleCount++
	c.mu.Unlock()

	bundle, err := c.fetchX509BundlesWithRetry(ctx)
	if err != nil {
		c.mu.Lock()
		c.stats.FetchBundleErrors++
		c.mu.Unlock()
		return nil, err
	}

	c.mu.Lock()
	c.trustBundle = bundle
	c.mu.Unlock()

	return bundle, nil
}

// WatchX509SVID watches for X.509 SVID updates via streaming.
func (c *Client) WatchX509SVID(ctx context.Context, callback identity.SVIDRotationCallback) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	c.onSVIDRotation = callback
	c.mu.Unlock()

	// Create cancellable context for the stream
	streamCtx, cancel := context.WithCancel(ctx)

	c.mu.Lock()
	if c.x509StreamCancel != nil {
		c.x509StreamCancel() // Cancel any existing stream
	}
	c.x509StreamCancel = cancel
	c.mu.Unlock()

	go c.watchX509SVIDLoop(streamCtx)

	return nil
}

// WatchTrustBundle watches for trust bundle updates via streaming.
func (c *Client) WatchTrustBundle(ctx context.Context, callback identity.TrustBundleUpdateCallback) error {
	if err := c.ensureConnected(ctx); err != nil {
		return err
	}

	c.mu.Lock()
	c.onBundleUpdate = callback
	c.mu.Unlock()

	// Create cancellable context for the stream
	streamCtx, cancel := context.WithCancel(ctx)

	c.mu.Lock()
	if c.bundleStreamCancel != nil {
		c.bundleStreamCancel() // Cancel any existing stream
	}
	c.bundleStreamCancel = cancel
	c.mu.Unlock()

	go c.watchTrustBundleLoop(streamCtx)

	return nil
}

// Health returns whether the client is healthy.
func (c *Client) Health(ctx context.Context) (bool, error) {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	if state != ClientStateConnected && state != ClientStateFallback {
		return false, fmt.Errorf("client not connected: %s", state)
	}

	// Try to fetch SVID as health check
	_, err := c.FetchX509SVID(ctx)
	if err != nil {
		c.mu.Lock()
		c.healthStatus = false
		c.healthError = err
		c.lastHealthCheck = time.Now()
		c.mu.Unlock()
		return false, err
	}

	c.mu.Lock()
	c.healthStatus = true
	c.healthError = nil
	c.lastHealthCheck = time.Now()
	c.mu.Unlock()

	return true, nil
}

// OnStateChange registers a callback for state changes.
func (c *Client) OnStateChange(callback func(oldState, newState ClientState)) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.onStateChange = callback
}

// CurrentSVID returns the currently cached SVID.
func (c *Client) CurrentSVID() *identity.X509SVID {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.currentSVID
}

// CurrentTrustBundle returns the currently cached trust bundle.
func (c *Client) CurrentTrustBundle() *identity.TrustBundle {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.trustBundle
}

// TrustDomain returns the trust domain.
func (c *Client) TrustDomain() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.config.TrustDomain != "" {
		return c.config.TrustDomain
	}
	return c.trustDomain
}

// Private methods

func (c *Client) ensureConnected(ctx context.Context) error {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()

	switch state {
	case ClientStateConnected, ClientStateFallback:
		return nil
	case ClientStateClosed:
		return fmt.Errorf("client is closed")
	default:
		return c.Connect(ctx)
	}
}

func (c *Client) notifyStateChange(oldState, newState ClientState) {
	c.mu.RLock()
	callback := c.onStateChange
	c.mu.RUnlock()

	if callback != nil {
		callback(oldState, newState)
	}
}

func (c *Client) enterFallbackMode(ctx context.Context, originalErr error) error {
	c.mu.Lock()
	c.state = ClientStateFallback
	c.stats.FallbackCount++
	c.mu.Unlock()

	c.notifyStateChange(ClientStateConnecting, ClientStateFallback)

	// Start reconnection attempts in background
	go c.reconnectLoop(ctx)

	// For fallback mode, we can use cached credentials if available
	if c.config.FallbackConfig.FallbackProvider == "cached" {
		c.mu.RLock()
		hasCached := c.currentSVID != nil && !c.currentSVID.Expired()
		c.mu.RUnlock()

		if hasCached {
			return nil // Continue with cached credentials
		}
	}

	return fmt.Errorf("SPIRE unavailable, fallback mode active: %w", originalErr)
}

func (c *Client) reconnectLoop(ctx context.Context) {
	interval := c.config.FallbackConfig.ReconnectInterval

	for {
		if err := wait.ForContext(ctx, interval); err != nil {
			return
		}

		c.mu.RLock()
		state := c.state
		c.mu.RUnlock()

		if state == ClientStateClosed {
			return
		}
		if state == ClientStateConnected {
			return
		}

		// Try to reconnect
		err := c.Connect(ctx)
		if err == nil {
			return
		}
	}
}

func (c *Client) fetchX509SVIDWithRetry(ctx context.Context) (*identity.X509SVID, error) {
	var lastErr error
	retryConfig := c.config.RetryConfig

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.calculateRetryDelay(attempt, retryConfig)
			if err := wait.ForContext(ctx, delay); err != nil {
				return nil, err
			}
		}

		svid, err := c.doFetchX509SVID(ctx)
		if err == nil {
			return svid, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", retryConfig.MaxRetries+1, lastErr)
}

func (c *Client) fetchJWTSVIDWithRetry(ctx context.Context, audience []string) (*identity.JWTSVID, error) {
	var lastErr error
	retryConfig := c.config.RetryConfig

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.calculateRetryDelay(attempt, retryConfig)
			if err := wait.ForContext(ctx, delay); err != nil {
				return nil, err
			}
		}

		svid, err := c.doFetchJWTSVID(ctx, audience)
		if err == nil {
			return svid, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", retryConfig.MaxRetries+1, lastErr)
}

func (c *Client) fetchX509BundlesWithRetry(ctx context.Context) (*identity.TrustBundle, error) {
	var lastErr error
	retryConfig := c.config.RetryConfig

	for attempt := 0; attempt <= retryConfig.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := c.calculateRetryDelay(attempt, retryConfig)
			if err := wait.ForContext(ctx, delay); err != nil {
				return nil, err
			}
		}

		bundle, err := c.doFetchX509Bundles(ctx)
		if err == nil {
			return bundle, nil
		}
		lastErr = err
	}

	return nil, fmt.Errorf("failed after %d attempts: %w", retryConfig.MaxRetries+1, lastErr)
}

func (c *Client) calculateRetryDelay(attempt int, config *RetryConfig) time.Duration {
	delay := config.InitialDelay
	for i := 1; i < attempt; i++ {
		delay = time.Duration(float64(delay) * config.Multiplier)
		if delay > config.MaxDelay {
			delay = config.MaxDelay
			break
		}
	}

	// Add jitter
	if config.Jitter > 0 {
		jitterAmount := float64(delay) * config.Jitter
		//nolint:gosec // G404: math/rand used for retry jitter timing, not security
		delay = time.Duration(float64(delay) + (rand.Float64()*2-1)*jitterAmount) // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- jitter does not require crypto randomness
	}

	return delay
}

// doFetchX509SVID performs the actual SVID fetch via the Workload API.
// This implements the SPIFFE Workload API FetchX509SVID RPC.
func (c *Client) doFetchX509SVID(ctx context.Context) (*identity.X509SVID, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Create the Workload API client
	client := NewSpiffeWorkloadClient(conn)

	// Call FetchX509SVID
	resp, err := client.FetchX509SVID(ctx)
	if err != nil {
		return nil, fmt.Errorf("FetchX509SVID failed: %w", err)
	}

	if len(resp.SVIDs) == 0 {
		return nil, fmt.Errorf("no SVIDs returned")
	}

	// Convert the first SVID to our format
	svidData := resp.SVIDs[0]
	return c.parseSVIDResponse(svidData)
}

// doFetchJWTSVID performs the actual JWT SVID fetch via the Workload API.
func (c *Client) doFetchJWTSVID(ctx context.Context, audience []string) (*identity.JWTSVID, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	client := NewSpiffeWorkloadClient(conn)

	resp, err := client.FetchJWTSVID(ctx, audience)
	if err != nil {
		return nil, fmt.Errorf("FetchJWTSVID failed: %w", err)
	}

	if len(resp.SVIDs) == 0 {
		return nil, fmt.Errorf("no JWT SVIDs returned")
	}

	// Parse the JWT SVID
	jwtData := resp.SVIDs[0]
	return c.parseJWTSVIDResponse(jwtData)
}

// doFetchX509Bundles fetches trust bundles via the Workload API.
func (c *Client) doFetchX509Bundles(ctx context.Context) (*identity.TrustBundle, error) {
	c.mu.RLock()
	conn := c.conn
	c.mu.RUnlock()

	if conn == nil {
		return nil, fmt.Errorf("not connected")
	}

	client := NewSpiffeWorkloadClient(conn)

	resp, err := client.FetchX509Bundles(ctx)
	if err != nil {
		return nil, fmt.Errorf("FetchX509Bundles failed: %w", err)
	}

	return c.parseBundleResponse(resp)
}

func (c *Client) parseSVIDResponse(data *X509SVIDData) (*identity.X509SVID, error) {
	// Parse SPIFFE ID
	spiffeID, err := identity.ParseSPIFFEID(data.SPIFFEID)
	if err != nil {
		return nil, fmt.Errorf("invalid SPIFFE ID: %w", err)
	}

	// Update trust domain
	c.mu.Lock()
	c.trustDomain = spiffeID.TrustDomain
	c.mu.Unlock()

	// Parse X.509 certificate chain
	certs, err := parseCertificates(data.X509SVID)
	if err != nil {
		return nil, fmt.Errorf("failed to parse certificates: %w", err)
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates in SVID")
	}

	// Parse private key
	key, err := parsePrivateKey(data.X509SVIDKey)
	if err != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", err)
	}

	return &identity.X509SVID{
		SPIFFEID:     spiffeID,
		Certificates: certs,
		PrivateKey:   key,
		ExpiresAt:    certs[0].NotAfter,
		IssuedAt:     certs[0].NotBefore,
		Hint:         data.Hint,
	}, nil
}

func (c *Client) parseJWTSVIDResponse(data *JWTSVIDData) (*identity.JWTSVID, error) {
	spiffeID, err := identity.ParseSPIFFEID(data.SPIFFEID)
	if err != nil {
		return nil, fmt.Errorf("invalid SPIFFE ID: %w", err)
	}

	return &identity.JWTSVID{
		SPIFFEID:  spiffeID,
		Token:     data.Token,
		ExpiresAt: time.Unix(data.ExpiresAt, 0),
		IssuedAt:  time.Unix(data.IssuedAt, 0),
	}, nil
}

func (c *Client) parseBundleResponse(resp *X509BundleResponse) (*identity.TrustBundle, error) {
	if len(resp.Bundles) == 0 {
		return nil, fmt.Errorf("no bundles returned")
	}

	// Get the first bundle (usually for our trust domain)
	var trustDomain string
	var bundleData []byte

	for td, data := range resp.Bundles {
		trustDomain = td
		bundleData = data
		break
	}

	certs, err := parseCertificates(bundleData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse bundle certificates: %w", err)
	}

	return &identity.TrustBundle{
		TrustDomain:     trustDomain,
		X509Authorities: certs,
		UpdatedAt:       time.Now(),
	}, nil
}

func (c *Client) watchX509SVIDLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		currentSVID := c.currentSVID
		c.mu.RUnlock()

		if conn == nil {
			if !waitForRetry(ctx, time.Second) {
				return
			}
			continue
		}

		client := NewSpiffeWorkloadClient(conn)

		// Start streaming
		stream, err := client.StreamX509SVID(ctx)
		if err != nil {
			c.mu.Lock()
			c.stats.StreamRestarts++
			c.mu.Unlock()
			if !waitForRetry(ctx, 5*time.Second) {
				return
			}
			continue
		}

		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}

			if len(resp.SVIDs) == 0 {
				continue
			}

			svid, err := c.parseSVIDResponse(resp.SVIDs[0])
			if err != nil {
				continue
			}

			c.mu.Lock()
			oldSVID := c.currentSVID
			c.currentSVID = svid
			callback := c.onSVIDRotation
			c.mu.Unlock()

			// Non-blocking send to channel
			select {
			case c.x509SVIDChan <- svid:
			default:
			}

			// Call rotation callback if SVID changed
			if callback != nil && (oldSVID == nil || oldSVID.ExpiresAt != svid.ExpiresAt) {
				callback(oldSVID, svid)
			}
		}

		// Stream ended, will restart
		c.mu.Lock()
		c.stats.StreamRestarts++
		c.mu.Unlock()

		// Restore current SVID from before stream issues
		if currentSVID != nil && !currentSVID.Expired() {
			c.mu.Lock()
			if c.currentSVID == nil {
				c.currentSVID = currentSVID
			}
			c.mu.Unlock()
		}

		if !waitForRetry(ctx, 5*time.Second) {
			return
		}
	}
}

func (c *Client) watchTrustBundleLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		c.mu.RLock()
		conn := c.conn
		c.mu.RUnlock()

		if conn == nil {
			if !waitForRetry(ctx, time.Second) {
				return
			}
			continue
		}

		client := NewSpiffeWorkloadClient(conn)

		// Start streaming
		stream, err := client.StreamX509Bundles(ctx)
		if err != nil {
			if !waitForRetry(ctx, 5*time.Second) {
				return
			}
			continue
		}

		for {
			resp, err := stream.Recv()
			if err != nil {
				break
			}

			bundle, err := c.parseBundleResponse(resp)
			if err != nil {
				continue
			}

			c.mu.Lock()
			c.trustBundle = bundle
			callback := c.onBundleUpdate
			c.mu.Unlock()

			// Non-blocking send to channel
			select {
			case c.trustBundleChan <- bundle:
			default:
			}

			if callback != nil {
				callback(bundle)
			}
		}

		if !waitForRetry(ctx, 5*time.Second) {
			return
		}
	}
}

func waitForRetry(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return true
	}
	return wait.ForContext(ctx, delay) == nil
}

// Helper functions

func parseCertificates(data []byte) ([]*x509.Certificate, error) {
	var certs []*x509.Certificate
	for len(data) > 0 {
		block, rest := pem.Decode(data)
		if block == nil {
			// Try to parse as DER
			cert, err := x509.ParseCertificate(data)
			if err == nil {
				certs = append(certs, cert)
				return certs, nil
			}
			// Try to parse multiple DER certs
			parsedCerts, err := x509.ParseCertificates(data)
			if err == nil {
				certs = append(certs, parsedCerts...)
				return certs, nil
			}
			break
		}

		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			certs = append(certs, cert)
		}
		data = rest
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found")
	}
	return certs, nil
}

func parsePrivateKey(data []byte) (crypto.PrivateKey, error) {
	block, _ := pem.Decode(data)
	var keyData []byte
	if block != nil {
		keyData = block.Bytes
	} else {
		keyData = data
	}

	// Try PKCS8
	key, err := x509.ParsePKCS8PrivateKey(keyData)
	if err == nil {
		return key, nil
	}

	// Try EC private key
	ecKey, err := x509.ParseECPrivateKey(keyData)
	if err == nil {
		return ecKey, nil
	}

	// Try RSA
	rsaKey, err := x509.ParsePKCS1PrivateKey(keyData)
	if err == nil {
		return rsaKey, nil
	}

	return nil, fmt.Errorf("failed to parse private key")
}

// Validate private key matches certificate
func validateKeyPair(cert *x509.Certificate, key crypto.PrivateKey) error {
	switch pub := cert.PublicKey.(type) {
	case *rsa.PublicKey:
		priv, ok := key.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("key type mismatch: expected RSA")
		}
		if pub.N.Cmp(priv.N) != 0 {
			return fmt.Errorf("key does not match certificate")
		}
	case *ecdsa.PublicKey:
		priv, ok := key.(*ecdsa.PrivateKey)
		if !ok {
			return fmt.Errorf("key type mismatch: expected ECDSA")
		}
		if pub.X.Cmp(priv.X) != 0 || pub.Y.Cmp(priv.Y) != 0 {
			return fmt.Errorf("key does not match certificate")
		}
	default:
		return fmt.Errorf("unsupported key type")
	}
	return nil
}
