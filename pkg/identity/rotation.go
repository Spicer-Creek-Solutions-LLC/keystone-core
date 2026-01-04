package identity

import (
	"context"
	"crypto/tls"
	"fmt"
	"math"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"
)

// RetryStrategy defines how SVID rotation retries are performed.
type RetryStrategy string

const (
	// RetryStrategyExponential uses exponential backoff with jitter.
	RetryStrategyExponential RetryStrategy = "exponential"
	// RetryStrategyLinear uses linear backoff.
	RetryStrategyLinear RetryStrategy = "linear"
	// RetryStrategyConstant uses constant delays between retries.
	RetryStrategyConstant RetryStrategy = "constant"
)

// RotationConfig configures SVID rotation behavior.
type RotationConfig struct {
	// CheckInterval is how often to check if rotation is needed.
	CheckInterval time.Duration

	// RotationThreshold is the fraction of SVID lifetime at which to rotate (0.0-1.0).
	RotationThreshold float64

	// RetryStrategy is the retry strategy to use on rotation failure.
	RetryStrategy RetryStrategy

	// InitialRetryDelay is the initial delay before first retry.
	InitialRetryDelay time.Duration

	// MaxRetryDelay is the maximum delay between retries.
	MaxRetryDelay time.Duration

	// MaxRetries is the maximum number of retry attempts (0 = unlimited).
	MaxRetries int

	// RetryMultiplier is the multiplier for exponential backoff.
	RetryMultiplier float64

	// JitterFraction is the fraction of jitter to add (0.0-1.0).
	JitterFraction float64

	// GracePeriod is how long to keep expired SVIDs usable.
	GracePeriod time.Duration

	// ConnectionDrainTimeout is how long to wait for connections to drain during rotation.
	ConnectionDrainTimeout time.Duration

	// EnableTLSRenegotiation enables TLS renegotiation on rotation.
	EnableTLSRenegotiation bool

	// PreRotationHook is called before rotation starts.
	PreRotationHook func(ctx context.Context, currentSVID *X509SVID) error

	// PostRotationHook is called after rotation completes.
	PostRotationHook func(ctx context.Context, oldSVID, newSVID *X509SVID) error
}

// DefaultRotationConfig returns a default rotation configuration.
func DefaultRotationConfig() *RotationConfig {
	return &RotationConfig{
		CheckInterval:          30 * time.Second,
		RotationThreshold:      0.5, // Rotate at 50% of lifetime
		RetryStrategy:          RetryStrategyExponential,
		InitialRetryDelay:      1 * time.Second,
		MaxRetryDelay:          5 * time.Minute,
		MaxRetries:             10,
		RetryMultiplier:        2.0,
		JitterFraction:         0.1,
		GracePeriod:            10 * time.Minute,
		ConnectionDrainTimeout: 30 * time.Second,
		EnableTLSRenegotiation: true,
	}
}

// RotationState represents the current state of SVID rotation.
type RotationState string

const (
	RotationStateIdle       RotationState = "idle"
	RotationStateRotating   RotationState = "rotating"
	RotationStateDraining   RotationState = "draining"
	RotationStateRetrying   RotationState = "retrying"
	RotationStateFailed     RotationState = "failed"
	RotationStateGracePeriod RotationState = "grace_period"
)

// RotationMetrics tracks SVID rotation statistics.
type RotationMetrics struct {
	// TotalRotations is the total number of successful rotations.
	TotalRotations int64
	// FailedRotations is the total number of failed rotation attempts.
	FailedRotations int64
	// RetryCount is the total number of retries across all rotations.
	RetryCount int64
	// LastRotationTime is when the last successful rotation occurred.
	LastRotationTime time.Time
	// LastRotationDuration is how long the last rotation took.
	LastRotationDuration time.Duration
	// LastFailureTime is when the last failure occurred.
	LastFailureTime time.Time
	// LastFailureReason is the reason for the last failure.
	LastFailureReason string
	// AverageRotationDuration is the average rotation duration.
	AverageRotationDuration time.Duration
	// CurrentState is the current rotation state.
	CurrentState RotationState
	// CurrentRetryAttempt is the current retry attempt number.
	CurrentRetryAttempt int
	// NextRotationTime is when the next rotation is expected.
	NextRotationTime time.Time
	// SVIDExpiry is when the current SVID expires.
	SVIDExpiry time.Time
	// UsingCachedSVID indicates if we're using a cached/grace period SVID.
	UsingCachedSVID bool
}

// SVIDRotationManager manages SVID rotation with robustness features.
type SVIDRotationManager struct {
	config  *RotationConfig
	metrics *RotationMetrics
	mu      sync.RWMutex

	// Current and cached SVIDs
	currentSVID *X509SVID
	cachedSVID  *X509SVID
	tlsConfig   *tls.Config

	// Rotation state
	state           RotationState
	retryAttempt    int
	lastRotation    time.Time

	// Callbacks
	onSVIDRotated    func(oldSVID, newSVID *X509SVID)
	onRotationFailed func(err error, attempt int)
	onStateChanged   func(oldState, newState RotationState)

	// Provider for obtaining new SVIDs
	svidProvider SVIDProvider

	// Connection tracking for draining
	activeConnections int64
	connectionWaiters []chan struct{}

	// Control
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// SVIDProvider is an interface for obtaining new SVIDs.
type SVIDProvider interface {
	// RenewSVID renews the given SVID.
	RenewSVID(ctx context.Context, current *X509SVID) (*X509SVID, error)
}

// NewSVIDRotationManager creates a new SVID rotation manager.
func NewSVIDRotationManager(config *RotationConfig, provider SVIDProvider) (*SVIDRotationManager, error) {
	if config == nil {
		config = DefaultRotationConfig()
	}

	if provider == nil {
		return nil, fmt.Errorf("SVID provider required")
	}

	if config.RotationThreshold <= 0 || config.RotationThreshold >= 1 {
		return nil, fmt.Errorf("rotation threshold must be between 0 and 1")
	}

	return &SVIDRotationManager{
		config:       config,
		svidProvider: provider,
		metrics: &RotationMetrics{
			CurrentState: RotationStateIdle,
		},
		state:  RotationStateIdle,
		stopCh: make(chan struct{}),
	}, nil
}

// Start starts the rotation manager background loop.
func (m *SVIDRotationManager) Start(ctx context.Context) error {
	m.mu.Lock()
	if m.currentSVID == nil {
		m.mu.Unlock()
		return fmt.Errorf("no current SVID set")
	}
	m.mu.Unlock()

	m.wg.Add(1)
	go m.rotationLoop(ctx)

	return nil
}

// Stop stops the rotation manager.
func (m *SVIDRotationManager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// SetCurrentSVID sets the current SVID.
func (m *SVIDRotationManager) SetCurrentSVID(svid *X509SVID) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.currentSVID = svid
	m.updateTLSConfig()
	m.updateMetrics()
}

// GetCurrentSVID returns the current SVID.
func (m *SVIDRotationManager) GetCurrentSVID() *X509SVID {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.currentSVID
}

// GetTLSConfig returns a TLS config that auto-updates on rotation.
func (m *SVIDRotationManager) GetTLSConfig() *tls.Config {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.tlsConfig
}

// GetMetrics returns current rotation metrics.
func (m *SVIDRotationManager) GetMetrics() RotationMetrics {
	m.mu.RLock()
	defer m.mu.RUnlock()

	metrics := *m.metrics
	metrics.CurrentState = m.state
	metrics.CurrentRetryAttempt = m.retryAttempt

	if m.currentSVID != nil {
		metrics.SVIDExpiry = m.currentSVID.ExpiresAt
		metrics.NextRotationTime = m.calculateNextRotationTime()
	}

	return metrics
}

// OnSVIDRotated registers a callback for SVID rotation events.
func (m *SVIDRotationManager) OnSVIDRotated(callback func(oldSVID, newSVID *X509SVID)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onSVIDRotated = callback
}

// OnRotationFailed registers a callback for rotation failure events.
func (m *SVIDRotationManager) OnRotationFailed(callback func(err error, attempt int)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onRotationFailed = callback
}

// OnStateChanged registers a callback for state change events.
func (m *SVIDRotationManager) OnStateChanged(callback func(oldState, newState RotationState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStateChanged = callback
}

// TrackConnection increments the active connection count.
func (m *SVIDRotationManager) TrackConnection() {
	atomic.AddInt64(&m.activeConnections, 1)
}

// UntrackConnection decrements the active connection count.
func (m *SVIDRotationManager) UntrackConnection() {
	count := atomic.AddInt64(&m.activeConnections, -1)

	// Notify any waiters if we're draining
	if count == 0 {
		m.mu.Lock()
		for _, ch := range m.connectionWaiters {
			close(ch)
		}
		m.connectionWaiters = nil
		m.mu.Unlock()
	}
}

// ForceRotation triggers an immediate rotation.
func (m *SVIDRotationManager) ForceRotation(ctx context.Context) error {
	m.mu.Lock()
	if m.state == RotationStateRotating || m.state == RotationStateDraining {
		m.mu.Unlock()
		return fmt.Errorf("rotation already in progress")
	}
	m.mu.Unlock()

	return m.performRotation(ctx)
}

// rotationLoop is the main rotation check loop.
func (m *SVIDRotationManager) rotationLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.stopCh:
			return
		case <-ticker.C:
			if m.shouldRotate() {
				if err := m.performRotation(ctx); err != nil {
					// Error handling is done in performRotation
					continue
				}
			}
		}
	}
}

// shouldRotate checks if rotation is needed.
func (m *SVIDRotationManager) shouldRotate() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if m.currentSVID == nil {
		return false
	}

	// Already rotating
	if m.state == RotationStateRotating || m.state == RotationStateDraining {
		return false
	}

	return m.currentSVID.ShouldRotate()
}

// performRotation performs the actual rotation with retry logic.
func (m *SVIDRotationManager) performRotation(ctx context.Context) error {
	m.setState(RotationStateRotating)
	startTime := time.Now()

	// Call pre-rotation hook
	if m.config.PreRotationHook != nil {
		m.mu.RLock()
		currentSVID := m.currentSVID
		m.mu.RUnlock()

		if err := m.config.PreRotationHook(ctx, currentSVID); err != nil {
			m.setState(RotationStateIdle)
			return fmt.Errorf("pre-rotation hook failed: %w", err)
		}
	}

	var lastErr error
	attempt := 0

	for {
		attempt++
		m.mu.Lock()
		m.retryAttempt = attempt
		m.mu.Unlock()

		// Check max retries
		if m.config.MaxRetries > 0 && attempt > m.config.MaxRetries {
			m.handleRotationFailure(lastErr, attempt-1)
			return fmt.Errorf("rotation failed after %d attempts: %w", attempt-1, lastErr)
		}

		// Attempt rotation
		newSVID, err := m.attemptRotation(ctx)
		if err == nil {
			// Success - drain connections and switch
			if err := m.drainAndSwitch(ctx, newSVID); err != nil {
				lastErr = err
				m.setState(RotationStateRetrying)
				m.waitForRetry(ctx, attempt)
				continue
			}

			// Update metrics
			m.mu.Lock()
			m.metrics.TotalRotations++
			m.metrics.LastRotationTime = time.Now()
			m.metrics.LastRotationDuration = time.Since(startTime)
			m.updateAverageRotationDuration()
			m.mu.Unlock()

			m.setState(RotationStateIdle)
			return nil
		}

		lastErr = err
		m.mu.Lock()
		m.metrics.RetryCount++
		m.mu.Unlock()

		// Notify failure callback
		if m.onRotationFailed != nil {
			m.onRotationFailed(err, attempt)
		}

		m.setState(RotationStateRetrying)

		// Wait before retry
		if !m.waitForRetry(ctx, attempt) {
			// Context cancelled or stopped
			m.setState(RotationStateIdle)
			return ctx.Err()
		}
	}
}

// attemptRotation attempts a single rotation.
func (m *SVIDRotationManager) attemptRotation(ctx context.Context) (*X509SVID, error) {
	m.mu.RLock()
	currentSVID := m.currentSVID
	m.mu.RUnlock()

	return m.svidProvider.RenewSVID(ctx, currentSVID)
}

// drainAndSwitch drains connections and switches to the new SVID.
func (m *SVIDRotationManager) drainAndSwitch(ctx context.Context, newSVID *X509SVID) error {
	m.setState(RotationStateDraining)

	// Wait for active connections to drain
	if m.config.ConnectionDrainTimeout > 0 {
		drainCtx, cancel := context.WithTimeout(ctx, m.config.ConnectionDrainTimeout)
		defer cancel()

		if err := m.waitForConnectionsDrained(drainCtx); err != nil {
			// Log warning but continue - don't fail rotation due to drain timeout
		}
	}

	// Switch to new SVID
	m.mu.Lock()
	oldSVID := m.currentSVID
	m.cachedSVID = oldSVID // Keep old SVID as cache/fallback
	m.currentSVID = newSVID
	m.lastRotation = time.Now()
	m.retryAttempt = 0
	m.updateTLSConfig()
	m.updateMetrics()
	m.mu.Unlock()

	// Call post-rotation hook
	if m.config.PostRotationHook != nil {
		if err := m.config.PostRotationHook(ctx, oldSVID, newSVID); err != nil {
			// Log warning but don't fail - rotation is complete
		}
	}

	// Notify callback
	if m.onSVIDRotated != nil {
		m.onSVIDRotated(oldSVID, newSVID)
	}

	return nil
}

// waitForConnectionsDrained waits for all active connections to close.
func (m *SVIDRotationManager) waitForConnectionsDrained(ctx context.Context) error {
	if atomic.LoadInt64(&m.activeConnections) == 0 {
		return nil
	}

	// Create a waiter channel
	waiter := make(chan struct{})
	m.mu.Lock()
	m.connectionWaiters = append(m.connectionWaiters, waiter)
	m.mu.Unlock()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-waiter:
		return nil
	}
}

// waitForRetry waits for the appropriate retry delay.
func (m *SVIDRotationManager) waitForRetry(ctx context.Context, attempt int) bool {
	delay := m.calculateRetryDelay(attempt)

	select {
	case <-ctx.Done():
		return false
	case <-m.stopCh:
		return false
	case <-time.After(delay):
		return true
	}
}

// calculateRetryDelay calculates the delay before the next retry.
func (m *SVIDRotationManager) calculateRetryDelay(attempt int) time.Duration {
	var delay time.Duration

	switch m.config.RetryStrategy {
	case RetryStrategyExponential:
		// Exponential backoff: initial * multiplier^(attempt-1)
		delay = time.Duration(float64(m.config.InitialRetryDelay) *
			math.Pow(m.config.RetryMultiplier, float64(attempt-1)))

	case RetryStrategyLinear:
		// Linear backoff: initial * attempt
		delay = m.config.InitialRetryDelay * time.Duration(attempt)

	case RetryStrategyConstant:
		// Constant delay
		delay = m.config.InitialRetryDelay

	default:
		delay = m.config.InitialRetryDelay
	}

	// Cap at max delay
	if delay > m.config.MaxRetryDelay {
		delay = m.config.MaxRetryDelay
	}

	// Add jitter
	if m.config.JitterFraction > 0 {
		jitter := time.Duration(float64(delay) * m.config.JitterFraction * (rand.Float64()*2 - 1))
		delay += jitter
		if delay < 0 {
			delay = m.config.InitialRetryDelay
		}
	}

	return delay
}

// handleRotationFailure handles a rotation failure after all retries exhausted.
func (m *SVIDRotationManager) handleRotationFailure(err error, attempts int) {
	m.mu.Lock()
	m.metrics.FailedRotations++
	m.metrics.LastFailureTime = time.Now()
	m.metrics.LastFailureReason = err.Error()

	// Check if we can use cached SVID during grace period
	if m.cachedSVID != nil && !m.cachedSVID.Expired() {
		m.metrics.UsingCachedSVID = true
		m.setState(RotationStateGracePeriod)
	} else if m.currentSVID != nil && time.Since(m.currentSVID.ExpiresAt) < m.config.GracePeriod {
		// Use expired SVID during grace period
		m.metrics.UsingCachedSVID = true
		m.setState(RotationStateGracePeriod)
	} else {
		m.setState(RotationStateFailed)
	}
	m.mu.Unlock()
}

// setState changes the rotation state and notifies listeners.
func (m *SVIDRotationManager) setState(newState RotationState) {
	m.mu.Lock()
	oldState := m.state
	m.state = newState
	m.metrics.CurrentState = newState
	callback := m.onStateChanged
	m.mu.Unlock()

	if callback != nil && oldState != newState {
		callback(oldState, newState)
	}
}

// updateTLSConfig updates the TLS config with the current SVID.
func (m *SVIDRotationManager) updateTLSConfig() {
	if m.currentSVID == nil {
		return
	}

	// Build certificate chain
	cert := tls.Certificate{
		PrivateKey: m.currentSVID.PrivateKey,
	}
	for _, c := range m.currentSVID.Certificates {
		cert.Certificate = append(cert.Certificate, c.Raw)
	}

	m.tlsConfig = &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   tls.VersionTLS12,
		// Use GetClientCertificate for dynamic certificate retrieval
		GetClientCertificate: func(*tls.CertificateRequestInfo) (*tls.Certificate, error) {
			m.mu.RLock()
			defer m.mu.RUnlock()
			if m.currentSVID == nil {
				return nil, fmt.Errorf("no SVID available")
			}
			c := tls.Certificate{
				PrivateKey: m.currentSVID.PrivateKey,
			}
			for _, cert := range m.currentSVID.Certificates {
				c.Certificate = append(c.Certificate, cert.Raw)
			}
			return &c, nil
		},
	}
}

// updateMetrics updates the metrics with current SVID info.
func (m *SVIDRotationManager) updateMetrics() {
	if m.currentSVID != nil {
		m.metrics.SVIDExpiry = m.currentSVID.ExpiresAt
		m.metrics.NextRotationTime = m.calculateNextRotationTime()
	}
}

// calculateNextRotationTime calculates when the next rotation should occur.
func (m *SVIDRotationManager) calculateNextRotationTime() time.Time {
	if m.currentSVID == nil {
		return time.Time{}
	}

	lifetime := m.currentSVID.ExpiresAt.Sub(m.currentSVID.IssuedAt)
	rotationPoint := time.Duration(float64(lifetime) * m.config.RotationThreshold)
	return m.currentSVID.IssuedAt.Add(rotationPoint)
}

// updateAverageRotationDuration updates the average rotation duration.
func (m *SVIDRotationManager) updateAverageRotationDuration() {
	// Simple exponential moving average
	alpha := 0.2
	if m.metrics.AverageRotationDuration == 0 {
		m.metrics.AverageRotationDuration = m.metrics.LastRotationDuration
	} else {
		m.metrics.AverageRotationDuration = time.Duration(
			float64(m.metrics.AverageRotationDuration)*(1-alpha) +
				float64(m.metrics.LastRotationDuration)*alpha,
		)
	}
}

// ConnectionContinuityManager manages connection continuity during SVID rotation.
type ConnectionContinuityManager struct {
	rotationManager *SVIDRotationManager
	mu              sync.RWMutex

	// Active TLS connections that need renegotiation
	tlsConnections map[string]*tls.Conn

	// Callback for notifying connections of rotation
	onRotation func(newCert *tls.Certificate)
}

// NewConnectionContinuityManager creates a new connection continuity manager.
func NewConnectionContinuityManager(rm *SVIDRotationManager) *ConnectionContinuityManager {
	ccm := &ConnectionContinuityManager{
		rotationManager: rm,
		tlsConnections:  make(map[string]*tls.Conn),
	}

	// Register for rotation events
	rm.OnSVIDRotated(func(oldSVID, newSVID *X509SVID) {
		ccm.handleRotation(newSVID)
	})

	return ccm
}

// RegisterConnection registers a TLS connection for continuity management.
func (ccm *ConnectionContinuityManager) RegisterConnection(id string, conn *tls.Conn) {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	ccm.tlsConnections[id] = conn
	ccm.rotationManager.TrackConnection()
}

// UnregisterConnection unregisters a TLS connection.
func (ccm *ConnectionContinuityManager) UnregisterConnection(id string) {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	delete(ccm.tlsConnections, id)
	ccm.rotationManager.UntrackConnection()
}

// OnRotation registers a callback for rotation events.
func (ccm *ConnectionContinuityManager) OnRotation(callback func(newCert *tls.Certificate)) {
	ccm.mu.Lock()
	defer ccm.mu.Unlock()
	ccm.onRotation = callback
}

// handleRotation handles SVID rotation for active connections.
func (ccm *ConnectionContinuityManager) handleRotation(newSVID *X509SVID) {
	ccm.mu.RLock()
	defer ccm.mu.RUnlock()

	// Build new certificate
	cert := tls.Certificate{
		PrivateKey: newSVID.PrivateKey,
	}
	for _, c := range newSVID.Certificates {
		cert.Certificate = append(cert.Certificate, c.Raw)
	}

	// Notify callback
	if ccm.onRotation != nil {
		ccm.onRotation(&cert)
	}

	// Note: Actual TLS renegotiation depends on the connection type
	// and protocol. For NATS, the connection will use the new cert
	// on the next handshake. For long-lived connections, the
	// GetClientCertificate callback in the TLS config handles this.
}
