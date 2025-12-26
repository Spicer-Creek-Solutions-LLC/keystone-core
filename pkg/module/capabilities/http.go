package capabilities

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// HTTPGetCapability allows making HTTP GET requests
type HTTPGetCapability struct {
	AllowedDomains []string      // List of allowed domains (e.g., "api.example.com")
	DeniedDomains  []string      // List of denied domains
	TimeoutMax     time.Duration // Maximum timeout for requests
	RateLimit      *RateLimit    // Rate limiting configuration
	MaxRespSize    int64         // Maximum response size in bytes (0 = unlimited)

	rateLimiter *rateLimiter
	once        sync.Once
}

// Name returns the capability name
func (c *HTTPGetCapability) Name() string {
	return "http.get"
}

// Validate checks if the capability configuration is valid
func (c *HTTPGetCapability) Validate() error {
	if len(c.AllowedDomains) == 0 {
		return fmt.Errorf("%w: at least one allowed domain required", ErrInvalidConfiguration)
	}

	if c.TimeoutMax <= 0 {
		return fmt.Errorf("%w: timeout must be positive", ErrInvalidConfiguration)
	}

	if c.MaxRespSize < 0 {
		return fmt.Errorf("%w: max response size cannot be negative", ErrInvalidConfiguration)
	}

	if c.RateLimit != nil {
		if err := c.RateLimit.Validate(); err != nil {
			return fmt.Errorf("%w: invalid rate limit: %v", ErrInvalidConfiguration, err)
		}
	}

	return nil
}

// CheckDomain validates if a domain is allowed
func (c *HTTPGetCapability) CheckDomain(domain string) error {
	// Check denied domains first
	for _, denied := range c.DeniedDomains {
		if matchesDomain(denied, domain) {
			return fmt.Errorf("%w: %s matches denied domain %s", ErrDomainDenied, domain, denied)
		}
	}

	// Check allowed domains
	allowed := false
	for _, allowedDomain := range c.AllowedDomains {
		if matchesDomain(allowedDomain, domain) {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("%w: %s", ErrDomainNotAllowed, domain)
	}

	return nil
}

// Get performs an HTTP GET request
func (c *HTTPGetCapability) Get(ctx *CapabilityContext, urlStr string, headers map[string]string) (*HTTPResponse, error) {
	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Check domain
	if err := c.CheckDomain(parsedURL.Host); err != nil {
		return nil, err
	}

	// Check rate limit
	c.once.Do(func() {
		if c.RateLimit != nil {
			c.rateLimiter = newRateLimiter(c.RateLimit)
		}
	})

	if c.rateLimiter != nil {
		if !c.rateLimiter.Allow() {
			return nil, ErrRateLimitExceeded
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: c.TimeoutMax,
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx.Context, "GET", urlStr, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body with size limit
	var body []byte
	if c.MaxRespSize > 0 {
		limitedReader := io.LimitReader(resp.Body, c.MaxRespSize+1)
		body, err = io.ReadAll(limitedReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		if int64(len(body)) > c.MaxRespSize {
			return nil, fmt.Errorf("%w: response size exceeds limit %d", ErrMaxSizeExceeded, c.MaxRespSize)
		}
	} else {
		body, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       body,
	}, nil
}

// HTTPPostCapability allows making HTTP POST requests
type HTTPPostCapability struct {
	AllowedDomains []string      // List of allowed domains
	DeniedDomains  []string      // List of denied domains
	TimeoutMax     time.Duration // Maximum timeout for requests
	RateLimit      *RateLimit    // Rate limiting configuration
	MaxReqSize     int64         // Maximum request body size in bytes (0 = unlimited)
	MaxRespSize    int64         // Maximum response size in bytes (0 = unlimited)

	rateLimiter *rateLimiter
	once        sync.Once
}

// Name returns the capability name
func (c *HTTPPostCapability) Name() string {
	return "http.post"
}

// Validate checks if the capability configuration is valid
func (c *HTTPPostCapability) Validate() error {
	if len(c.AllowedDomains) == 0 {
		return fmt.Errorf("%w: at least one allowed domain required", ErrInvalidConfiguration)
	}

	if c.TimeoutMax <= 0 {
		return fmt.Errorf("%w: timeout must be positive", ErrInvalidConfiguration)
	}

	if c.MaxReqSize < 0 {
		return fmt.Errorf("%w: max request size cannot be negative", ErrInvalidConfiguration)
	}

	if c.MaxRespSize < 0 {
		return fmt.Errorf("%w: max response size cannot be negative", ErrInvalidConfiguration)
	}

	if c.RateLimit != nil {
		if err := c.RateLimit.Validate(); err != nil {
			return fmt.Errorf("%w: invalid rate limit: %v", ErrInvalidConfiguration, err)
		}
	}

	return nil
}

// CheckDomain validates if a domain is allowed
func (c *HTTPPostCapability) CheckDomain(domain string) error {
	// Check denied domains first
	for _, denied := range c.DeniedDomains {
		if matchesDomain(denied, domain) {
			return fmt.Errorf("%w: %s matches denied domain %s", ErrDomainDenied, domain, denied)
		}
	}

	// Check allowed domains
	allowed := false
	for _, allowedDomain := range c.AllowedDomains {
		if matchesDomain(allowedDomain, domain) {
			allowed = true
			break
		}
	}

	if !allowed {
		return fmt.Errorf("%w: %s", ErrDomainNotAllowed, domain)
	}

	return nil
}

// Post performs an HTTP POST request
func (c *HTTPPostCapability) Post(ctx *CapabilityContext, urlStr string, body []byte, headers map[string]string) (*HTTPResponse, error) {
	// Check request size
	if c.MaxReqSize > 0 && int64(len(body)) > c.MaxReqSize {
		return nil, fmt.Errorf("%w: request body size %d exceeds limit %d", ErrMaxSizeExceeded, len(body), c.MaxReqSize)
	}

	// Parse URL
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Check domain
	if err := c.CheckDomain(parsedURL.Host); err != nil {
		return nil, err
	}

	// Check rate limit
	c.once.Do(func() {
		if c.RateLimit != nil {
			c.rateLimiter = newRateLimiter(c.RateLimit)
		}
	})

	if c.rateLimiter != nil {
		if !c.rateLimiter.Allow() {
			return nil, ErrRateLimitExceeded
		}
	}

	// Create HTTP client with timeout
	client := &http.Client{
		Timeout: c.TimeoutMax,
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx.Context, "POST", urlStr, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	for key, value := range headers {
		req.Header.Set(key, value)
	}

	// Execute request
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body with size limit
	var respBody []byte
	if c.MaxRespSize > 0 {
		limitedReader := io.LimitReader(resp.Body, c.MaxRespSize+1)
		respBody, err = io.ReadAll(limitedReader)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
		if int64(len(respBody)) > c.MaxRespSize {
			return nil, fmt.Errorf("%w: response size exceeds limit %d", ErrMaxSizeExceeded, c.MaxRespSize)
		}
	} else {
		respBody, err = io.ReadAll(resp.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read response: %w", err)
		}
	}

	return &HTTPResponse{
		StatusCode: resp.StatusCode,
		Headers:    resp.Header,
		Body:       respBody,
	}, nil
}

// HTTPResponse represents an HTTP response
type HTTPResponse struct {
	StatusCode int
	Headers    http.Header
	Body       []byte
}

// RateLimit defines rate limiting configuration
type RateLimit struct {
	Requests int           // Number of requests allowed
	Period   time.Duration // Time period for rate limit
}

// Validate checks if the rate limit configuration is valid
func (r *RateLimit) Validate() error {
	if r.Requests <= 0 {
		return fmt.Errorf("requests must be positive")
	}
	if r.Period <= 0 {
		return fmt.Errorf("period must be positive")
	}
	return nil
}

// rateLimiter implements token bucket rate limiting
type rateLimiter struct {
	rate     *RateLimit
	tokens   int
	lastTime time.Time
	mu       sync.Mutex
}

func newRateLimiter(rate *RateLimit) *rateLimiter {
	return &rateLimiter{
		rate:     rate,
		tokens:   rate.Requests,
		lastTime: time.Now(),
	}
}

func (r *rateLimiter) Allow() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(r.lastTime)

	// Refill tokens based on elapsed time
	if elapsed >= r.rate.Period {
		r.tokens = r.rate.Requests
		r.lastTime = now
	}

	// Check if we have tokens available
	if r.tokens > 0 {
		r.tokens--
		return true
	}

	return false
}

// matchesDomain checks if a domain matches a pattern (supports * wildcard)
func matchesDomain(pattern, domain string) bool {
	// Exact match
	if pattern == domain {
		return true
	}

	// Wildcard match (*.example.com matches api.example.com)
	if strings.HasPrefix(pattern, "*.") {
		suffix := pattern[2:]
		return strings.HasSuffix(domain, "."+suffix) || domain == suffix
	}

	return false
}
