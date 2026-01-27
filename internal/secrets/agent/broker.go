package agent

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/internal/identity"
	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// BrokerClientConfig configures the NATS-based broker client.
type BrokerClientConfig struct {
	// NATSURLs is a list of NATS server URLs.
	NATSURLs []string

	// RequestTimeout is the timeout for each request.
	RequestTimeout time.Duration

	// IdentityClient provides SPIFFE identities for mTLS.
	IdentityClient *identity.AgentIdentityClient

	// SubjectPrefix is the NATS subject prefix for secrets.
	SubjectPrefix string

	// MaxReconnects is the maximum number of reconnection attempts.
	MaxReconnects int

	// ReconnectWait is the time to wait between reconnection attempts.
	ReconnectWait time.Duration

	// Name is used for client identification.
	Name string
}

// DefaultBrokerClientConfig returns a configuration with default values.
func DefaultBrokerClientConfig() *BrokerClientConfig {
	return &BrokerClientConfig{
		NATSURLs:       []string{nats.DefaultURL},
		RequestTimeout: 10 * time.Second,
		SubjectPrefix:  "keystone.secrets",
		MaxReconnects:  -1, // infinite
		ReconnectWait:  2 * time.Second,
		Name:           "keystone-agent",
	}
}

// NATSBrokerClient implements SecretBrokerClient using NATS.
type NATSBrokerClient struct {
	config *BrokerClientConfig

	mu     sync.RWMutex
	conn   *nats.Conn
	closed bool

	// TLS config from SPIFFE
	tlsConfig *tls.Config

	// Stats
	stats BrokerClientStats
}

// BrokerClientStats contains statistics about broker operations.
type BrokerClientStats struct {
	ConnectAttempts int64
	ConnectFailures int64
	RequestCount    int64
	RequestErrors   int64
	ReconnectCount  int64
}

// NewNATSBrokerClient creates a new NATS-based broker client.
func NewNATSBrokerClient(config *BrokerClientConfig) (*NATSBrokerClient, error) {
	if config == nil {
		config = DefaultBrokerClientConfig()
	}

	if len(config.NATSURLs) == 0 {
		return nil, fmt.Errorf("at least one NATS URL is required")
	}

	return &NATSBrokerClient{
		config: config,
	}, nil
}

// Connect establishes a connection to the NATS server.
func (c *NATSBrokerClient) Connect(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return fmt.Errorf("client is closed")
	}

	if c.conn != nil && c.conn.IsConnected() {
		return nil
	}

	c.stats.ConnectAttempts++

	// Build NATS options
	opts := []nats.Option{
		nats.Name(c.config.Name),
		nats.MaxReconnects(c.config.MaxReconnects),
		nats.ReconnectWait(c.config.ReconnectWait),
		nats.ReconnectHandler(func(_ *nats.Conn) {
			c.mu.Lock()
			c.stats.ReconnectCount++
			c.mu.Unlock()
		}),
	}

	// Configure TLS from SPIFFE identity
	if c.config.IdentityClient != nil {
		tlsConfig := c.config.IdentityClient.GetTLSConfig()
		if tlsConfig != nil {
			c.tlsConfig = tlsConfig
			opts = append(opts, nats.Secure(tlsConfig))
		}
	}

	// Connect to NATS
	conn, err := nats.Connect(
		natsURLsToString(c.config.NATSURLs),
		opts...,
	)
	if err != nil {
		c.stats.ConnectFailures++
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	c.conn = conn
	return nil
}

// Close closes the connection.
func (c *NATSBrokerClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.closed = true
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	return nil
}

// GetSecret retrieves a single secret from the broker.
func (c *NATSBrokerClient) GetSecret(ctx context.Context, req *SecretRequest) (*secrets.Secret, error) {
	c.mu.RLock()
	conn := c.conn
	c.stats.RequestCount++
	c.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, ErrNotConnected
	}

	// Build request message
	reqMsg := &secretGetRequest{
		Path:    req.Path,
		Version: req.Version,
	}

	payload, err := json.Marshal(reqMsg)
	if err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	subject := c.config.SubjectPrefix + ".get"
	timeout := c.config.RequestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}

	msg, err := conn.Request(subject, payload, timeout)
	if err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Parse response
	var resp secretGetResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.Error != "" {
		if resp.Error == "not found" {
			return nil, ErrSecretNotFound
		}
		return nil, fmt.Errorf("broker error: %s", resp.Error)
	}

	return resp.Secret, nil
}

// GetSecretBatch retrieves multiple secrets from the broker.
func (c *NATSBrokerClient) GetSecretBatch(ctx context.Context, reqs []*SecretRequest) ([]*secrets.Secret, error) {
	c.mu.RLock()
	conn := c.conn
	c.stats.RequestCount++
	c.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, ErrNotConnected
	}

	// Build batch request
	reqMsg := &secretBatchRequest{
		Requests: make([]secretGetRequest, len(reqs)),
	}
	for i, req := range reqs {
		reqMsg.Requests[i] = secretGetRequest{
			Path:    req.Path,
			Version: req.Version,
		}
	}

	payload, err := json.Marshal(reqMsg)
	if err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	subject := c.config.SubjectPrefix + ".batch"
	timeout := c.config.RequestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}

	msg, err := conn.Request(subject, payload, timeout)
	if err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Parse response
	var resp secretBatchResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("broker error: %s", resp.Error)
	}

	return resp.Secrets, nil
}

// RenewLease renews a secret's lease.
func (c *NATSBrokerClient) RenewLease(ctx context.Context, leaseID string, increment time.Duration) (*secrets.Lease, error) {
	c.mu.RLock()
	conn := c.conn
	c.stats.RequestCount++
	c.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, ErrNotConnected
	}

	// Build request message
	reqMsg := &leaseRenewRequest{
		LeaseID:   leaseID,
		Increment: increment,
	}

	payload, err := json.Marshal(reqMsg)
	if err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	subject := c.config.SubjectPrefix + ".lease.renew"
	timeout := c.config.RequestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}

	msg, err := conn.Request(subject, payload, timeout)
	if err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Parse response
	var resp leaseRenewResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.Error != "" {
		if resp.Error == "lease expired" {
			return nil, ErrLeaseExpired
		}
		return nil, fmt.Errorf("broker error: %s", resp.Error)
	}

	return resp.Lease, nil
}

// ListSecrets lists secrets under a prefix.
func (c *NATSBrokerClient) ListSecrets(ctx context.Context, prefix string) ([]string, error) {
	c.mu.RLock()
	conn := c.conn
	c.stats.RequestCount++
	c.mu.RUnlock()

	if conn == nil || !conn.IsConnected() {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, ErrNotConnected
	}

	// Build request message
	reqMsg := &secretListRequest{
		Prefix: prefix,
	}

	payload, err := json.Marshal(reqMsg)
	if err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request
	subject := c.config.SubjectPrefix + ".list"
	timeout := c.config.RequestTimeout
	if deadline, ok := ctx.Deadline(); ok {
		if remaining := time.Until(deadline); remaining < timeout {
			timeout = remaining
		}
	}

	msg, err := conn.Request(subject, payload, timeout)
	if err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("request failed: %w", err)
	}

	// Parse response
	var resp secretListResponse
	if err := json.Unmarshal(msg.Data, &resp); err != nil {
		c.mu.Lock()
		c.stats.RequestErrors++
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	if resp.Error != "" {
		return nil, fmt.Errorf("broker error: %s", resp.Error)
	}

	return resp.Paths, nil
}

// Healthy returns whether the client is connected.
func (c *NATSBrokerClient) Healthy() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn != nil && c.conn.IsConnected()
}

// Stats returns the broker client statistics.
func (c *NATSBrokerClient) Stats() BrokerClientStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.stats
}

// Message types for NATS communication

type secretGetRequest struct {
	Path    string `json:"path"`
	Version int    `json:"version,omitempty"`
}

type secretGetResponse struct {
	Secret *secrets.Secret `json:"secret,omitempty"`
	Error  string          `json:"error,omitempty"`
}

type secretBatchRequest struct {
	Requests []secretGetRequest `json:"requests"`
}

type secretBatchResponse struct {
	Secrets []*secrets.Secret `json:"secrets,omitempty"`
	Error   string            `json:"error,omitempty"`
}

type leaseRenewRequest struct {
	LeaseID   string        `json:"lease_id"`
	Increment time.Duration `json:"increment"`
}

type leaseRenewResponse struct {
	Lease *secrets.Lease `json:"lease,omitempty"`
	Error string         `json:"error,omitempty"`
}

type secretListRequest struct {
	Prefix string `json:"prefix"`
}

type secretListResponse struct {
	Paths []string `json:"paths,omitempty"`
	Error string   `json:"error,omitempty"`
}

// natsURLsToString converts a slice of URLs to a comma-separated string.
func natsURLsToString(urls []string) string {
	if len(urls) == 0 {
		return nats.DefaultURL
	}
	result := urls[0]
	for i := 1; i < len(urls); i++ {
		result += "," + urls[i]
	}
	return result
}

// =============================================================================
// SPIFFE Authorization
// =============================================================================

// SPIFFEAuthorizer provides SPIFFE-based authorization for secret access.
type SPIFFEAuthorizer struct {
	// AllowedTrustDomains is a list of trust domains allowed to access secrets.
	AllowedTrustDomains []string

	// PathPolicies maps secret path prefixes to allowed SPIFFE ID patterns.
	PathPolicies map[string][]string
}

// NewSPIFFEAuthorizer creates a new SPIFFE authorizer.
func NewSPIFFEAuthorizer() *SPIFFEAuthorizer {
	return &SPIFFEAuthorizer{
		AllowedTrustDomains: make([]string, 0),
		PathPolicies:        make(map[string][]string),
	}
}

// AddTrustDomain adds an allowed trust domain.
func (a *SPIFFEAuthorizer) AddTrustDomain(domain string) {
	a.AllowedTrustDomains = append(a.AllowedTrustDomains, domain)
}

// AddPathPolicy adds a policy for a secret path prefix.
func (a *SPIFFEAuthorizer) AddPathPolicy(pathPrefix string, allowedPatterns []string) {
	a.PathPolicies[pathPrefix] = allowedPatterns
}

// Authorize checks if a SPIFFE ID is authorized to access a secret path.
func (a *SPIFFEAuthorizer) Authorize(spiffeID identity.SPIFFEID, secretPath string) error {
	// Check trust domain
	trustDomainAllowed := len(a.AllowedTrustDomains) == 0
	for _, domain := range a.AllowedTrustDomains {
		if spiffeID.TrustDomain == domain {
			trustDomainAllowed = true
			break
		}
	}
	if !trustDomainAllowed {
		return fmt.Errorf("trust domain %q not allowed", spiffeID.TrustDomain)
	}

	// Check path policies (most specific match wins)
	var bestMatchPolicy []string
	bestMatchLen := 0
	for prefix, patterns := range a.PathPolicies {
		if len(prefix) > bestMatchLen && hasPrefix(secretPath, prefix) {
			bestMatchPolicy = patterns
			bestMatchLen = len(prefix)
		}
	}

	// No policy means allow (if trust domain was allowed)
	if bestMatchPolicy == nil {
		return nil
	}

	// Check if SPIFFE ID matches any allowed pattern
	for _, pattern := range bestMatchPolicy {
		if matchSPIFFEPattern(spiffeID, pattern) {
			return nil
		}
	}

	return fmt.Errorf("access denied: %s cannot access %s", spiffeID.String(), secretPath)
}

// hasPrefix checks if a path has a prefix (supporting glob patterns).
func hasPrefix(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	return path[:len(prefix)] == prefix
}

// matchSPIFFEPattern checks if a SPIFFE ID matches a pattern.
// Patterns can use * for wildcard matching.
func matchSPIFFEPattern(id identity.SPIFFEID, pattern string) bool {
	idStr := id.String()

	// Simple glob matching
	if pattern == "*" {
		return true
	}

	// Exact match
	if idStr == pattern {
		return true
	}

	// Prefix match with wildcard
	if len(pattern) > 1 && pattern[len(pattern)-1] == '*' {
		prefix := pattern[:len(pattern)-1]
		return len(idStr) >= len(prefix) && idStr[:len(prefix)] == prefix
	}

	return false
}
