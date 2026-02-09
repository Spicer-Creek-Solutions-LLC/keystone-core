// Package vault provides a HashiCorp Vault backend for the secrets broker.
package vault

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// ClientConfig configures the Vault client.
type ClientConfig struct {
	// Address is the Vault server address (e.g., "https://vault.example.com:8200").
	Address string `json:"address"`

	// Namespace is the Vault namespace (Enterprise only).
	Namespace string `json:"namespace,omitempty"`

	// TLS configures TLS settings.
	TLS *TLSConfig `json:"tls,omitempty"`

	// Timeout is the HTTP client timeout.
	Timeout time.Duration `json:"timeout,omitempty"`

	// MaxRetries is the maximum number of retries for failed requests.
	MaxRetries int `json:"max_retries,omitempty"`

	// RetryWaitMin is the minimum wait time between retries.
	RetryWaitMin time.Duration `json:"retry_wait_min,omitempty"`

	// RetryWaitMax is the maximum wait time between retries.
	RetryWaitMax time.Duration `json:"retry_wait_max,omitempty"`

	// Auth configures authentication.
	Auth *AuthConfig `json:"auth,omitempty"`
}

// TLSConfig configures TLS settings for the Vault client.
type TLSConfig struct {
	// CACert is the path to a CA certificate file.
	CACert string `json:"ca_cert,omitempty"`

	// CAPath is the path to a directory of CA certificates.
	CAPath string `json:"ca_path,omitempty"`

	// ClientCert is the path to a client certificate file.
	ClientCert string `json:"client_cert,omitempty"`

	// ClientKey is the path to a client key file.
	ClientKey string `json:"client_key,omitempty"`

	// ServerName is the server name to use for TLS verification.
	ServerName string `json:"server_name,omitempty"`

	// SkipVerify disables TLS verification (not recommended for production).
	SkipVerify bool `json:"skip_verify,omitempty"`
}

// DefaultClientConfig returns a client configuration with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Address:      "http://127.0.0.1:8200",
		Timeout:      30 * time.Second,
		MaxRetries:   3,
		RetryWaitMin: 500 * time.Millisecond,
		RetryWaitMax: 5 * time.Second,
	}
}

// Client is a Vault API client with connection pooling and retry logic.
type Client struct {
	mu sync.RWMutex

	config     *ClientConfig
	httpClient *http.Client
	token      string
	namespace  string

	// Health status
	healthy         atomic.Bool
	sealed          atomic.Bool
	lastHealthCheck time.Time

	// Token renewal
	tokenExpiry    time.Time
	tokenRenewable bool
	renewCancel    context.CancelFunc
	renewWg        sync.WaitGroup

	// Authenticator for token management
	auth Authenticator

	closed atomic.Bool
}

// NewClient creates a new Vault client.
func NewClient(config *ClientConfig) (*Client, error) {
	if config == nil {
		config = DefaultClientConfig()
	}

	if config.Address == "" {
		return nil, fmt.Errorf("vault address is required")
	}

	// Build HTTP client with TLS
	httpClient, err := buildHTTPClient(config)
	if err != nil {
		return nil, fmt.Errorf("failed to build HTTP client: %w", err)
	}

	client := &Client{
		config:     config,
		httpClient: httpClient,
		namespace:  config.Namespace,
	}

	client.healthy.Store(false)
	client.sealed.Store(false)

	return client, nil
}

// buildHTTPClient creates an HTTP client with the configured TLS settings.
func buildHTTPClient(config *ClientConfig) (*http.Client, error) {
	transport := &http.Transport{
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 100,
		IdleConnTimeout:     90 * time.Second,
	}

	if config.TLS != nil {
		tlsConfig, err := buildTLSConfig(config.TLS)
		if err != nil {
			return nil, err
		}
		transport.TLSClientConfig = tlsConfig
	}

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &http.Client{
		Transport: transport,
		Timeout:   timeout,
	}, nil
}

// buildTLSConfig creates a TLS configuration from the config.
func buildTLSConfig(config *TLSConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if config.SkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	if config.ServerName != "" {
		tlsConfig.ServerName = config.ServerName
	}

	// Load CA certificate
	if config.CACert != "" {
		caCert, err := os.ReadFile(config.CACert)
		if err != nil {
			return nil, fmt.Errorf("failed to read CA certificate: %w", err)
		}

		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Load client certificate
	if config.ClientCert != "" && config.ClientKey != "" {
		cert, err := tls.LoadX509KeyPair(config.ClientCert, config.ClientKey)
		if err != nil {
			return nil, fmt.Errorf("failed to load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	return tlsConfig, nil
}

// SetToken sets the authentication token.
func (c *Client) SetToken(token string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.token = token
}

// Token returns the current authentication token.
func (c *Client) Token() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.token
}

// SetNamespace sets the Vault namespace for requests.
func (c *Client) SetNamespace(namespace string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.namespace = namespace
}

// Namespace returns the current namespace.
func (c *Client) Namespace() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.namespace
}

// SetAuthenticator sets the authenticator for token management.
func (c *Client) SetAuthenticator(auth Authenticator) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.auth = auth
}

// Authenticate performs authentication using the configured authenticator.
func (c *Client) Authenticate(ctx context.Context) error {
	c.mu.RLock()
	auth := c.auth
	c.mu.RUnlock()

	if auth == nil {
		return fmt.Errorf("no authenticator configured")
	}

	authResp, err := auth.Authenticate(ctx, c)
	if err != nil {
		return fmt.Errorf("authentication failed: %w", err)
	}

	c.mu.Lock()
	c.token = authResp.Token
	c.tokenExpiry = authResp.ExpiresAt
	c.tokenRenewable = authResp.Renewable
	c.mu.Unlock()

	return nil
}

// StartTokenRenewal starts automatic token renewal.
func (c *Client) StartTokenRenewal(ctx context.Context) {
	c.mu.Lock()
	if c.renewCancel != nil {
		c.mu.Unlock()
		return
	}

	renewCtx, cancel := context.WithCancel(ctx)
	c.renewCancel = cancel
	c.mu.Unlock()

	c.renewWg.Add(1)
	go c.tokenRenewalLoop(renewCtx)
}

// StopTokenRenewal stops automatic token renewal.
func (c *Client) StopTokenRenewal() {
	c.mu.Lock()
	if c.renewCancel != nil {
		c.renewCancel()
		c.renewCancel = nil
	}
	c.mu.Unlock()

	c.renewWg.Wait()
}

// tokenRenewalLoop periodically renews the token.
func (c *Client) tokenRenewalLoop(ctx context.Context) {
	defer c.renewWg.Done()

	for {
		c.mu.RLock()
		expiry := c.tokenExpiry
		renewable := c.tokenRenewable
		c.mu.RUnlock()

		if !renewable || expiry.IsZero() {
			// Token is not renewable, re-authenticate when it expires
			if !expiry.IsZero() {
				sleepDuration := time.Until(expiry) - 30*time.Second
				if sleepDuration > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(sleepDuration):
						_ = c.Authenticate(ctx)
					}
				}
			}
			continue
		}

		// Renew at 50% of TTL
		ttl := time.Until(expiry)
		renewAt := ttl / 2
		if renewAt < time.Minute {
			renewAt = time.Minute
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(renewAt):
			if err := c.renewToken(ctx); err != nil {
				// Try to re-authenticate
				_ = c.Authenticate(ctx)
			}
		}
	}
}

// renewToken renews the current token.
func (c *Client) renewToken(ctx context.Context) error {
	resp, err := c.Write(ctx, "auth/token/renew-self", nil)
	if err != nil {
		return err
	}

	auth, ok := resp["auth"].(map[string]interface{})
	if !ok {
		return fmt.Errorf("invalid renewal response")
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if token, ok := auth["client_token"].(string); ok {
		c.token = token
	}

	if leaseDuration, ok := auth["lease_duration"].(float64); ok {
		c.tokenExpiry = time.Now().Add(time.Duration(leaseDuration) * time.Second)
	}

	if renewable, ok := auth["renewable"].(bool); ok {
		c.tokenRenewable = renewable
	}

	return nil
}

// Health checks the health of the Vault server.
func (c *Client) Health(ctx context.Context) (*HealthResponse, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.config.Address+"/v1/sys/health", http.NoBody)
	if err != nil {
		return nil, err
	}

	// Health endpoint doesn't require auth but we'll add namespace if set
	c.mu.RLock()
	namespace := c.namespace
	c.mu.RUnlock()

	if namespace != "" {
		req.Header.Set("X-Vault-Namespace", namespace)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		c.healthy.Store(false)
		return nil, fmt.Errorf("health check failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read health response: %w", err)
	}

	var health HealthResponse
	if err := json.Unmarshal(body, &health); err != nil {
		return nil, fmt.Errorf("failed to parse health response: %w", err)
	}

	// Update cached status
	c.mu.Lock()
	c.lastHealthCheck = time.Now()
	c.mu.Unlock()

	c.sealed.Store(health.Sealed)
	c.healthy.Store(health.Initialized && !health.Sealed)

	return &health, nil
}

// HealthResponse represents the Vault health endpoint response.
type HealthResponse struct {
	Initialized                bool   `json:"initialized"`
	Sealed                     bool   `json:"sealed"`
	Standby                    bool   `json:"standby"`
	PerformanceStandby         bool   `json:"performance_standby"`
	ReplicationPerformanceMode string `json:"replication_performance_mode"`
	ReplicationDRMode          string `json:"replication_dr_mode"`
	ServerTimeUTC              int64  `json:"server_time_utc"`
	Version                    string `json:"version"`
	ClusterName                string `json:"cluster_name,omitempty"`
	ClusterID                  string `json:"cluster_id,omitempty"`
}

// Healthy returns true if the Vault server is healthy.
func (c *Client) Healthy(ctx context.Context) bool {
	// Check cached status if recent
	c.mu.RLock()
	lastCheck := c.lastHealthCheck
	c.mu.RUnlock()

	if time.Since(lastCheck) < 30*time.Second {
		return c.healthy.Load()
	}

	// Perform fresh health check
	health, err := c.Health(ctx)
	if err != nil {
		return false
	}

	return health.Initialized && !health.Sealed
}

// IsSealed returns true if Vault is sealed.
func (c *Client) IsSealed() bool {
	return c.sealed.Load()
}

// Read reads a secret from Vault.
func (c *Client) Read(ctx context.Context, path string) (map[string]interface{}, error) {
	return c.request(ctx, http.MethodGet, path, nil)
}

// Write writes data to Vault.
func (c *Client) Write(ctx context.Context, path string, data map[string]interface{}) (map[string]interface{}, error) {
	return c.request(ctx, http.MethodPost, path, data)
}

// Delete deletes a secret from Vault.
func (c *Client) Delete(ctx context.Context, path string) (map[string]interface{}, error) {
	return c.request(ctx, http.MethodDelete, path, nil)
}

// List lists secrets at a path.
func (c *Client) List(ctx context.Context, path string) ([]string, error) {
	// Vault list uses a query parameter
	resp, err := c.request(ctx, "LIST", path, nil)
	if err != nil {
		return nil, err
	}

	data, ok := resp["data"].(map[string]interface{})
	if !ok {
		return nil, nil
	}

	keys, ok := data["keys"].([]interface{})
	if !ok {
		return nil, nil
	}

	result := make([]string, 0, len(keys))
	for _, k := range keys {
		if s, ok := k.(string); ok {
			result = append(result, s)
		}
	}

	return result, nil
}

// request performs an HTTP request to Vault.
func (c *Client) request(ctx context.Context, method, path string, data map[string]interface{}) (map[string]interface{}, error) {
	if c.closed.Load() {
		return nil, fmt.Errorf("client is closed")
	}

	// Build URL
	url := c.config.Address + "/v1/" + strings.TrimPrefix(path, "/")

	// Build request body
	var body io.Reader
	if data != nil {
		jsonData, err := json.Marshal(data)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		body = strings.NewReader(string(jsonData))
	}

	// Handle LIST method (Vault uses GET with list=true query param)
	httpMethod := method
	if method == "LIST" {
		httpMethod = http.MethodGet
		if strings.Contains(url, "?") {
			url += "&list=true"
		} else {
			url += "?list=true"
		}
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, httpMethod, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	c.mu.RLock()
	token := c.token
	namespace := c.namespace
	c.mu.RUnlock()

	if token != "" {
		req.Header.Set("X-Vault-Token", token)
	}

	if namespace != "" {
		req.Header.Set("X-Vault-Namespace", namespace)
	}

	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	// Perform request with retries
	var lastErr error
	maxRetries := c.config.MaxRetries
	if maxRetries == 0 {
		maxRetries = 1
	}

	for attempt := 0; attempt < maxRetries; attempt++ {
		if attempt > 0 {
			// Wait before retry
			wait := c.config.RetryWaitMin
			if wait == 0 {
				wait = 500 * time.Millisecond
			}
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(wait):
			}

			// Recreate request for retry
			if data != nil {
				jsonData, _ := json.Marshal(data)
				body = strings.NewReader(string(jsonData))
			}
			req, _ = http.NewRequestWithContext(ctx, httpMethod, url, body)
			if token != "" {
				req.Header.Set("X-Vault-Token", token)
			}
			if namespace != "" {
				req.Header.Set("X-Vault-Namespace", namespace)
			}
			if data != nil {
				req.Header.Set("Content-Type", "application/json")
			}
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue
		}

		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			lastErr = fmt.Errorf("failed to read response: %w", err)
			continue
		}

		// Check for errors
		if resp.StatusCode == http.StatusNotFound {
			return nil, secrets.ErrSecretNotFound
		}

		if resp.StatusCode == http.StatusForbidden {
			return nil, secrets.ErrAccessDenied
		}

		if resp.StatusCode == http.StatusServiceUnavailable {
			// Vault is sealed or unavailable
			c.sealed.Store(true)
			c.healthy.Store(false)
			return nil, secrets.ErrBackendUnavailable
		}

		// Retry on 5xx errors (except 503 which is handled above)
		if resp.StatusCode >= 500 {
			var errResp struct {
				Errors []string `json:"errors"`
			}
			if err := json.Unmarshal(respBody, &errResp); err == nil && len(errResp.Errors) > 0 {
				lastErr = fmt.Errorf("vault error: %s", strings.Join(errResp.Errors, ", "))
			} else {
				lastErr = fmt.Errorf("vault request failed with status %d", resp.StatusCode)
			}
			continue
		}

		if resp.StatusCode >= 400 {
			var errResp struct {
				Errors []string `json:"errors"`
			}
			if err := json.Unmarshal(respBody, &errResp); err == nil && len(errResp.Errors) > 0 {
				return nil, fmt.Errorf("vault error: %s", strings.Join(errResp.Errors, ", "))
			}
			return nil, fmt.Errorf("vault request failed with status %d", resp.StatusCode)
		}

		// Parse successful response
		if len(respBody) == 0 {
			return nil, nil
		}

		var result map[string]interface{}
		if err := json.Unmarshal(respBody, &result); err != nil {
			return nil, fmt.Errorf("failed to parse response: %w", err)
		}

		return result, nil
	}

	return nil, lastErr
}

// Close closes the client and releases resources.
func (c *Client) Close() error {
	if c.closed.Swap(true) {
		return nil
	}

	c.StopTokenRenewal()

	c.httpClient.CloseIdleConnections()

	return nil
}

// Address returns the Vault server address.
func (c *Client) Address() string {
	return c.config.Address
}
