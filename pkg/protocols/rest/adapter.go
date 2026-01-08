// Package rest provides a REST/HTTP protocol adapter for proxy agents.
package rest

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/credentials"
	"github.com/shawnbutts/keystone-core/pkg/protocols"
	"github.com/shawnbutts/keystone-core/pkg/proxy"
)

// Config contains REST adapter configuration.
type Config struct {
	// ConnectionConfig contains common connection settings.
	*protocols.ConnectionConfig

	// BaseURL is the base URL for API requests.
	BaseURL string `json:"base_url,omitempty"`

	// DefaultHeaders are headers added to all requests.
	DefaultHeaders map[string]string `json:"default_headers,omitempty"`

	// FollowRedirects enables following HTTP redirects.
	FollowRedirects bool `json:"follow_redirects,omitempty"`

	// MaxRedirects is the maximum number of redirects to follow.
	MaxRedirects int `json:"max_redirects,omitempty"`

	// ValidateSSL enables SSL certificate validation.
	ValidateSSL bool `json:"validate_ssl,omitempty"`

	// RateLimitPerSecond limits requests per second (0 = unlimited).
	RateLimitPerSecond int `json:"rate_limit_per_second,omitempty"`

	// RetryOnStatus specifies HTTP status codes that trigger retry.
	RetryOnStatus []int `json:"retry_on_status,omitempty"`

	// ContentType is the default content type for requests.
	ContentType string `json:"content_type,omitempty"`

	// AcceptType is the default Accept header value.
	AcceptType string `json:"accept_type,omitempty"`
}

// DefaultConfig returns a default REST configuration.
func DefaultConfig() *Config {
	return &Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		FollowRedirects:  true,
		MaxRedirects:     10,
		ValidateSSL:      true,
		ContentType:      "application/json",
		AcceptType:       "application/json",
		RetryOnStatus:    []int{429, 502, 503, 504},
	}
}

// Adapter implements the REST/HTTP protocol adapter.
type Adapter struct {
	config     *Config
	client     *Client
	device     *proxy.ProxiedDevice
	credential credentials.Credential
	auth       Authenticator
	mu         sync.RWMutex

	// Connection state
	connected   bool
	lastError   error
	lastConnect time.Time

	// Metrics
	metrics *protocols.AdapterMetrics

	// Rate limiting
	rateLimiter *RateLimiter
}

// NewAdapter creates a new REST adapter.
func NewAdapter(config *Config) *Adapter {
	if config == nil {
		config = DefaultConfig()
	}
	if config.ConnectionConfig == nil {
		config.ConnectionConfig = protocols.DefaultConnectionConfig()
	}

	adapter := &Adapter{
		config:  config,
		metrics: &protocols.AdapterMetrics{},
	}

	if config.RateLimitPerSecond > 0 {
		adapter.rateLimiter = NewRateLimiter(config.RateLimitPerSecond)
	}

	return adapter
}

// Type returns the protocol type.
func (a *Adapter) Type() protocols.ProtocolType {
	return protocols.ProtocolREST
}

// Connect establishes a REST API connection.
func (a *Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.device = device
	a.credential = cred

	// Build base URL
	baseURL := a.config.BaseURL
	if baseURL == "" {
		scheme := "https"
		if !a.config.ValidateSSL {
			scheme = "http"
		}
		port := device.Port
		if port == 0 {
			if scheme == "https" {
				port = 443
			} else {
				port = 80
			}
		}
		baseURL = fmt.Sprintf("%s://%s:%d", scheme, device.Address, port)
	}

	// Create HTTP client
	clientConfig := &ClientConfig{
		BaseURL:         baseURL,
		Timeout:         a.config.Timeout,
		FollowRedirects: a.config.FollowRedirects,
		MaxRedirects:    a.config.MaxRedirects,
		ValidateSSL:     a.config.ValidateSSL,
		MaxRetries:      a.config.MaxRetries,
		RetryDelay:      a.config.RetryDelay,
		RetryOnStatus:   a.config.RetryOnStatus,
		DefaultHeaders:  a.config.DefaultHeaders,
	}

	a.client = NewClient(clientConfig)

	// Setup authentication
	auth, err := a.setupAuth(cred)
	if err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("failed to setup authentication: %w", err)
	}
	a.auth = auth

	// Verify connectivity with a simple request
	if err := a.verifyConnectivity(ctx); err != nil {
		a.lastError = err
		a.metrics.ConnectionErrors++
		return fmt.Errorf("connectivity verification failed: %w", err)
	}

	a.connected = true
	a.lastConnect = time.Now()
	a.lastError = nil
	a.metrics.ConnectionCount++
	a.metrics.LastActivity = time.Now()

	return nil
}

// setupAuth configures authentication based on credential type.
func (a *Adapter) setupAuth(cred credentials.Credential) (Authenticator, error) {
	switch c := cred.(type) {
	case *credentials.RESTBasicCredential:
		return NewBasicAuth(c.Username, c.Password), nil

	case *credentials.RESTBearerCredential:
		return NewBearerAuth(c.Token), nil

	case *credentials.RESTAPIKeyCredential:
		// Determine location from credential fields
		location := APIKeyHeader
		key := c.HeaderName
		if key == "" {
			key = "X-API-Key"
		}
		if c.QueryParam != "" {
			location = APIKeyQuery
			key = c.QueryParam
		}
		return NewAPIKeyAuth(key, c.APIKey, location), nil

	case *credentials.RESTOAuth2Credential:
		oauth := NewOAuth2ClientCredentials(c.ClientID, c.ClientSecret, c.TokenURL)
		oauth.Scopes = c.Scopes
		return oauth, nil

	default:
		// Allow nil credential for unauthenticated APIs
		if cred == nil {
			return &NoAuth{}, nil
		}
		return nil, fmt.Errorf("unsupported credential type for REST: %T", cred)
	}
}

// verifyConnectivity checks if the API is reachable.
func (a *Adapter) verifyConnectivity(ctx context.Context) error {
	req, err := a.client.NewRequest(ctx, http.MethodHead, "/", nil)
	if err != nil {
		// Try GET if HEAD fails
		req, err = a.client.NewRequest(ctx, http.MethodGet, "/", nil)
		if err != nil {
			return err
		}
	}

	if a.auth != nil {
		if err := a.auth.Authenticate(req); err != nil {
			return err
		}
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Accept any 2xx or 3xx status as "connected"
	if resp.StatusCode >= 200 && resp.StatusCode < 400 {
		return nil
	}

	// Also accept 401/403 - the API is reachable, just auth may be wrong
	if resp.StatusCode == 401 || resp.StatusCode == 403 {
		return nil
	}

	return fmt.Errorf("unexpected status code: %d", resp.StatusCode)
}

// Execute implements the ProtocolAdapter interface.
func (a *Adapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	result := &protocols.ExecuteResult{
		StartTime: time.Now(),
	}

	// Parse the command as a REST request
	// Format: METHOD /path [body]
	method, path, body, err := parseRESTCommand(req.Command)
	if err != nil {
		result.Error = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, err
	}

	// Apply rate limiting
	if a.rateLimiter != nil {
		if err := a.rateLimiter.Wait(ctx); err != nil {
			result.Error = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			return result, err
		}
	}

	// Create request
	httpReq, err := client.NewRequest(ctx, method, path, body)
	if err != nil {
		result.Error = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, err
	}

	// Set content type
	if body != nil {
		httpReq.Header.Set("Content-Type", a.config.ContentType)
	}
	httpReq.Header.Set("Accept", a.config.AcceptType)

	// Apply authentication
	if a.auth != nil {
		if err := a.auth.Authenticate(httpReq); err != nil {
			result.Error = err.Error()
			result.EndTime = time.Now()
			result.Duration = result.EndTime.Sub(result.StartTime)
			return result, err
		}
	}

	// Execute request
	resp, err := client.Do(httpReq)
	if err != nil {
		result.Error = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		a.metrics.ExecutionErrors++
		return result, err
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		result.Error = err.Error()
		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		return result, err
	}

	result.Stdout = respBody
	result.ExitCode = resp.StatusCode
	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	// Update metrics
	a.mu.Lock()
	a.metrics.ExecutionCount++
	if resp.StatusCode >= 400 {
		a.metrics.ExecutionErrors++
	}
	a.metrics.LastActivity = time.Now()
	a.mu.Unlock()

	return result, nil
}

// Request performs a REST API request.
func (a *Adapter) Request(ctx context.Context, method, path string, body interface{}) (*Response, error) {
	a.mu.RLock()
	client := a.client
	connected := a.connected
	a.mu.RUnlock()

	if !connected || client == nil {
		return nil, fmt.Errorf("not connected")
	}

	// Apply rate limiting
	if a.rateLimiter != nil {
		if err := a.rateLimiter.Wait(ctx); err != nil {
			return nil, err
		}
	}

	// Serialize body if needed
	var bodyReader io.Reader
	if body != nil {
		switch v := body.(type) {
		case string:
			bodyReader = strings.NewReader(v)
		case []byte:
			bodyReader = bytes.NewReader(v)
		case io.Reader:
			bodyReader = v
		default:
			data, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("failed to marshal body: %w", err)
			}
			bodyReader = bytes.NewReader(data)
		}
	}

	// Create request
	req, err := client.NewRequest(ctx, method, path, bodyReader)
	if err != nil {
		return nil, err
	}

	// Set content type
	if body != nil {
		req.Header.Set("Content-Type", a.config.ContentType)
	}
	req.Header.Set("Accept", a.config.AcceptType)

	// Apply authentication
	if a.auth != nil {
		if err := a.auth.Authenticate(req); err != nil {
			return nil, err
		}
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}

	return NewResponse(resp)
}

// Get performs a GET request.
func (a *Adapter) Get(ctx context.Context, path string) (*Response, error) {
	return a.Request(ctx, http.MethodGet, path, nil)
}

// Post performs a POST request.
func (a *Adapter) Post(ctx context.Context, path string, body interface{}) (*Response, error) {
	return a.Request(ctx, http.MethodPost, path, body)
}

// Put performs a PUT request.
func (a *Adapter) Put(ctx context.Context, path string, body interface{}) (*Response, error) {
	return a.Request(ctx, http.MethodPut, path, body)
}

// Patch performs a PATCH request.
func (a *Adapter) Patch(ctx context.Context, path string, body interface{}) (*Response, error) {
	return a.Request(ctx, http.MethodPatch, path, body)
}

// Delete performs a DELETE request.
func (a *Adapter) Delete(ctx context.Context, path string) (*Response, error) {
	return a.Request(ctx, http.MethodDelete, path, nil)
}

// Disconnect closes the REST adapter.
func (a *Adapter) Disconnect(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.connected = false
	a.client = nil
	return nil
}

// HealthCheck performs a health check on the REST connection.
func (a *Adapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	a.mu.RLock()
	connected := a.connected
	a.mu.RUnlock()

	result := &protocols.HealthCheckResult{
		LastCheck: time.Now(),
		Details:   make(map[string]interface{}),
	}

	if !connected {
		result.Healthy = false
		result.Status = "not connected"
		return result, nil
	}

	start := time.Now()
	err := a.verifyConnectivity(ctx)
	result.Latency = time.Since(start)

	if err != nil {
		result.Healthy = false
		result.Status = fmt.Sprintf("connectivity check failed: %v", err)
		return result, nil
	}

	result.Healthy = true
	result.Status = "connected"
	result.Details["last_connect"] = a.lastConnect
	result.Details["base_url"] = a.config.BaseURL

	return result, nil
}

// IsConnected returns true if the REST adapter is connected.
func (a *Adapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected
}

// Metrics returns the adapter metrics.
func (a *Adapter) Metrics() *protocols.AdapterMetrics {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.metrics
}

// parseRESTCommand parses a REST command string.
// Format: METHOD /path [body]
func parseRESTCommand(cmd string) (method, path string, body io.Reader, err error) {
	parts := strings.SplitN(cmd, " ", 3)
	if len(parts) < 2 {
		return "", "", nil, fmt.Errorf("invalid REST command format, expected: METHOD /path [body]")
	}

	method = strings.ToUpper(parts[0])
	path = parts[1]

	// Validate method
	validMethods := map[string]bool{
		http.MethodGet:     true,
		http.MethodPost:    true,
		http.MethodPut:     true,
		http.MethodPatch:   true,
		http.MethodDelete:  true,
		http.MethodHead:    true,
		http.MethodOptions: true,
	}
	if !validMethods[method] {
		return "", "", nil, fmt.Errorf("invalid HTTP method: %s", method)
	}

	// Validate path
	if !strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "http") {
		path = "/" + path
	}

	// Parse body if present
	if len(parts) == 3 {
		body = strings.NewReader(parts[2])
	}

	return method, path, body, nil
}

// NewAdapterFactory creates an adapter factory for REST.
func NewAdapterFactory(config *Config) protocols.AdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.ProtocolAdapter, error) {
		cfg := config
		if cfg == nil {
			cfg = DefaultConfig()
		}
		cfg.ConnectionConfig = connConfig
		return NewAdapter(cfg), nil
	}
}

// RateLimiter provides simple rate limiting.
type RateLimiter struct {
	rate     int
	tokens   int
	lastTime time.Time
	mu       sync.Mutex
}

// NewRateLimiter creates a new rate limiter.
func NewRateLimiter(ratePerSecond int) *RateLimiter {
	return &RateLimiter{
		rate:     ratePerSecond,
		tokens:   ratePerSecond,
		lastTime: time.Now(),
	}
}

// Wait waits for rate limit token.
func (r *RateLimiter) Wait(ctx context.Context) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Replenish tokens
	now := time.Now()
	elapsed := now.Sub(r.lastTime)
	r.tokens += int(elapsed.Seconds() * float64(r.rate))
	if r.tokens > r.rate {
		r.tokens = r.rate
	}
	r.lastTime = now

	if r.tokens > 0 {
		r.tokens--
		return nil
	}

	// Wait for next token
	waitTime := time.Second / time.Duration(r.rate)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(waitTime):
		return nil
	}
}

// GetJSON performs a GET request and unmarshals the response.
func (a *Adapter) GetJSON(ctx context.Context, path string, result interface{}) error {
	resp, err := a.Get(ctx, path)
	if err != nil {
		return err
	}
	return resp.JSON(result)
}

// PostJSON performs a POST request with JSON body and unmarshals the response.
func (a *Adapter) PostJSON(ctx context.Context, path string, body, result interface{}) error {
	resp, err := a.Post(ctx, path, body)
	if err != nil {
		return err
	}
	return resp.JSON(result)
}

// PutJSON performs a PUT request with JSON body and unmarshals the response.
func (a *Adapter) PutJSON(ctx context.Context, path string, body, result interface{}) error {
	resp, err := a.Put(ctx, path, body)
	if err != nil {
		return err
	}
	return resp.JSON(result)
}

// PatchJSON performs a PATCH request with JSON body and unmarshals the response.
func (a *Adapter) PatchJSON(ctx context.Context, path string, body, result interface{}) error {
	resp, err := a.Patch(ctx, path, body)
	if err != nil {
		return err
	}
	return resp.JSON(result)
}

// BuildURL builds a URL with query parameters.
func (a *Adapter) BuildURL(path string, params map[string]string) string {
	if len(params) == 0 {
		return path
	}

	values := url.Values{}
	for k, v := range params {
		values.Set(k, v)
	}

	if strings.Contains(path, "?") {
		return path + "&" + values.Encode()
	}
	return path + "?" + values.Encode()
}

// init registers the REST adapter with the default registry.
func init() {
	protocols.Register(protocols.ProtocolREST, NewAdapterFactory(nil))
}
