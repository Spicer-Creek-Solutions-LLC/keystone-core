// Package rest provides a REST/HTTP protocol adapter for proxy agents.
package rest

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

var (
	insecureTLSWarningOnce sync.Once
)

// ClientConfig contains HTTP client configuration.
type ClientConfig struct {
	// BaseURL is the base URL for all requests.
	BaseURL string

	// Timeout is the request timeout.
	Timeout time.Duration

	// FollowRedirects enables following HTTP redirects.
	FollowRedirects bool

	// MaxRedirects is the maximum number of redirects to follow.
	MaxRedirects int

	// ValidateSSL enables SSL certificate validation.
	ValidateSSL bool

	// MaxRetries is the maximum number of retries.
	MaxRetries int

	// RetryDelay is the delay between retries.
	RetryDelay time.Duration

	// RetryOnStatus specifies HTTP status codes that trigger retry.
	RetryOnStatus []int

	// DefaultHeaders are headers added to all requests.
	DefaultHeaders map[string]string

	// ProxyURL is the HTTP proxy URL.
	ProxyURL string

	// MaxIdleConns is the maximum number of idle connections.
	MaxIdleConns int

	// MaxIdleConnsPerHost is the maximum number of idle connections per host.
	MaxIdleConnsPerHost int

	// IdleConnTimeout is the timeout for idle connections.
	IdleConnTimeout time.Duration
}

// DefaultClientConfig returns default HTTP client configuration.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Timeout:             30 * time.Second,
		FollowRedirects:     true,
		MaxRedirects:        10,
		ValidateSSL:         true,
		MaxRetries:          3,
		RetryDelay:          time.Second,
		RetryOnStatus:       []int{429, 502, 503, 504},
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 10,
		IdleConnTimeout:     90 * time.Second,
	}
}

// Client is an HTTP client with retry support.
type Client struct {
	config     *ClientConfig
	httpClient *http.Client
	baseURL    *url.URL
}

// NewClient creates a new HTTP client.
func NewClient(config *ClientConfig) *Client {
	if config == nil {
		config = DefaultClientConfig()
	}

	// Parse base URL
	var baseURL *url.URL
	if config.BaseURL != "" {
		var err error
		baseURL, err = url.Parse(config.BaseURL)
		if err != nil {
			baseURL = nil
		}
	}

	// Create transport
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   30 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		MaxIdleConns:          config.MaxIdleConns,
		MaxIdleConnsPerHost:   config.MaxIdleConnsPerHost,
		IdleConnTimeout:       config.IdleConnTimeout,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	}

	// Configure TLS
	if !config.ValidateSSL {
		// Require explicit environment variable to disable SSL validation
		// This prevents accidental disabling of certificate verification
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			insecureTLSWarningOnce.Do(func() {
				log.Printf("WARNING: ValidateSSL=false ignored. Set KSCORE_ALLOW_INSECURE_TLS=1 to allow insecure TLS connections.")
			})
			// Keep SSL validation enabled (don't set InsecureSkipVerify)
		} else {
			insecureTLSWarningOnce.Do(func() {
				log.Printf("WARNING: TLS certificate verification is disabled. This is insecure and should only be used for testing.")
			})
			transport.TLSClientConfig = &tls.Config{ // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.bypass-tls-verification.bypass-tls-verification -- InsecureSkipVerify only allowed with KSCORE_ALLOW_INSECURE_TLS=1 for dev/test
				MinVersion:         tls.VersionTLS12,
				InsecureSkipVerify: true, // #nosec G402 -- gated by KSCORE_ALLOW_INSECURE_TLS env var
			}
		}
	}

	// Configure proxy
	if config.ProxyURL != "" {
		proxyURL, err := url.Parse(config.ProxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(proxyURL)
		}
	}

	// Create HTTP client
	httpClient := &http.Client{
		Transport: transport,
		Timeout:   config.Timeout,
	}

	// Configure redirect policy
	if !config.FollowRedirects {
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		}
	} else if config.MaxRedirects > 0 {
		httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= config.MaxRedirects {
				return fmt.Errorf("stopped after %d redirects", config.MaxRedirects)
			}
			return nil
		}
	}

	return &Client{
		config:     config,
		httpClient: httpClient,
		baseURL:    baseURL,
	}
}

// NewRequest creates a new HTTP request.
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	// Build full URL
	var reqURL string
	if c.baseURL != nil && !isAbsoluteURL(path) {
		u := *c.baseURL
		u.Path = joinPaths(u.Path, path)
		reqURL = u.String()
	} else {
		reqURL = path
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, method, reqURL, body)
	if err != nil {
		return nil, err
	}

	// Set default headers
	for k, v := range c.config.DefaultHeaders {
		req.Header.Set(k, v)
	}

	return req, nil
}

// Do executes an HTTP request with retry support.
func (c *Client) Do(req *http.Request) (*http.Response, error) {
	var resp *http.Response
	var err error

	for attempt := 0; attempt <= c.config.MaxRetries; attempt++ {
		// Clone request for retry (body may have been consumed)
		var reqBody io.Reader
		if req.Body != nil && req.GetBody != nil {
			reqBody, err = req.GetBody()
			if err != nil {
				return nil, err
			}
		}

		// Create new request for retry
		if attempt > 0 {
			newReq, err := http.NewRequestWithContext(req.Context(), req.Method, req.URL.String(), reqBody)
			if err != nil {
				return nil, err
			}
			newReq.Header = req.Header.Clone()
			req = newReq
		}

		resp, err = c.httpClient.Do(req)

		// Don't retry on context errors
		if err != nil {
			if req.Context().Err() != nil {
				return nil, err
			}
			// Retry on network errors
			if attempt < c.config.MaxRetries {
				if err := waitForRetry(req.Context(), retryBackoff(c.config.RetryDelay, attempt)); err != nil {
					return nil, err
				}
				continue
			}
			return nil, err
		}

		// Check if we should retry based on status code
		if c.shouldRetry(resp.StatusCode) && attempt < c.config.MaxRetries {
			resp.Body.Close()
			if err := waitForRetry(req.Context(), retryBackoff(c.config.RetryDelay, attempt)); err != nil {
				return nil, err
			}
			continue
		}

		return resp, nil
	}

	return resp, err
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	return wait.ForContext(ctx, delay)
}

// retryBackoff computes an exponential backoff delay with ~10% jitter,
// capped at 30 seconds.
func retryBackoff(base time.Duration, attempt int) time.Duration {
	delay := base * time.Duration(1<<uint(attempt))
	if delay > 30*time.Second {
		delay = 30 * time.Second
	}
	// Add up to 10% jitter
	tenth := delay / 10
	if tenth > 0 {
		var buf [8]byte
		_, _ = rand.Read(buf[:])
		n := int64(binary.LittleEndian.Uint64(buf[:]) >> 1) //nolint:gosec // right-shift ensures positive int64
		delay += time.Duration(n % int64(tenth))
	}
	return delay
}

// shouldRetry checks if the response status code should trigger a retry.
func (c *Client) shouldRetry(statusCode int) bool {
	for _, code := range c.config.RetryOnStatus {
		if statusCode == code {
			return true
		}
	}
	return false
}

// isAbsoluteURL checks if a URL is absolute.
func isAbsoluteURL(path string) bool {
	u, err := url.Parse(path)
	return err == nil && u.IsAbs()
}

// joinPaths joins two URL paths.
func joinPaths(base, path string) string {
	if path == "" {
		return base
	}
	if base == "" {
		return path
	}

	// Ensure base doesn't end with / and path starts with /
	base = trimSuffix(base, "/")
	if !hasPrefix(path, "/") {
		path = "/" + path
	}
	return base + path
}

// trimSuffix removes a suffix from a string.
func trimSuffix(s, suffix string) string {
	if len(s) >= len(suffix) && s[len(s)-len(suffix):] == suffix {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// hasPrefix checks if a string has a prefix.
func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}

// Get performs a GET request.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Post performs a POST request.
func (c *Client) Post(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodPost, path, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Put performs a PUT request.
func (c *Client) Put(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodPut, path, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Patch performs a PATCH request.
func (c *Client) Patch(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodPatch, path, body)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Delete performs a DELETE request.
func (c *Client) Delete(ctx context.Context, path string) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodDelete, path, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Head performs a HEAD request.
func (c *Client) Head(ctx context.Context, path string) (*http.Response, error) {
	req, err := c.NewRequest(ctx, http.MethodHead, path, nil)
	if err != nil {
		return nil, err
	}
	return c.Do(req)
}

// Close closes the HTTP client.
func (c *Client) Close() {
	c.httpClient.CloseIdleConnections()
}

// SetHeader sets a default header.
func (c *Client) SetHeader(key, value string) {
	if c.config.DefaultHeaders == nil {
		c.config.DefaultHeaders = make(map[string]string)
	}
	c.config.DefaultHeaders[key] = value
}

// RemoveHeader removes a default header.
func (c *Client) RemoveHeader(key string) {
	delete(c.config.DefaultHeaders, key)
}
