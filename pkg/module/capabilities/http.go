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

// Default security limits for HTTP capabilities
const (
	// DefaultMaxRespSize is 10MB - a reasonable default for most API responses
	DefaultMaxRespSize = 10 * 1024 * 1024
	// MaxAllowedRespSize is 100MB - the absolute maximum allowed
	MaxAllowedRespSize = 100 * 1024 * 1024
	// DefaultMaxReqSize is 10MB - a reasonable default for request bodies
	DefaultMaxReqSize = 10 * 1024 * 1024
	// MaxAllowedReqSize is 100MB - the absolute maximum allowed for requests
	MaxAllowedReqSize = 100 * 1024 * 1024
)

// DefaultHTTPRateLimit provides a secure default rate limit
var DefaultHTTPRateLimit = &RateLimit{
	Requests: 100,
	Period:   time.Minute,
}

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

	// Validate domain patterns for security
	for _, domain := range c.AllowedDomains {
		if err := validateDomainPatternSecurity(domain); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidConfiguration, err)
		}
	}

	if c.TimeoutMax <= 0 {
		return fmt.Errorf("%w: timeout must be positive", ErrInvalidConfiguration)
	}

	// MaxRespSize must be set and within bounds for security
	if c.MaxRespSize <= 0 {
		return fmt.Errorf("%w: max response size must be set (got %d, use DefaultMaxRespSize=%d or specify explicitly)",
			ErrInvalidConfiguration, c.MaxRespSize, DefaultMaxRespSize)
	}
	if c.MaxRespSize > MaxAllowedRespSize {
		return fmt.Errorf("%w: max response size %d exceeds maximum allowed %d",
			ErrInvalidConfiguration, c.MaxRespSize, MaxAllowedRespSize)
	}

	// Rate limiting is required for HTTP capabilities to prevent abuse
	if c.RateLimit == nil {
		return fmt.Errorf("%w: rate limit is required (use DefaultHTTPRateLimit or specify explicitly)",
			ErrInvalidConfiguration)
	}
	if err := c.RateLimit.Validate(); err != nil {
		return fmt.Errorf("%w: invalid rate limit: %w", ErrInvalidConfiguration, err)
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
	req, err := http.NewRequestWithContext(ctx.Context, "GET", urlStr, http.NoBody)
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

	// Read response body with size limit (MaxRespSize is always required)
	limitedReader := io.LimitReader(resp.Body, c.MaxRespSize+1)
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(body)) > c.MaxRespSize {
		return nil, fmt.Errorf("%w: response size exceeds limit %d", ErrMaxSizeExceeded, c.MaxRespSize)
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

	// Validate domain patterns for security
	for _, domain := range c.AllowedDomains {
		if err := validateDomainPatternSecurity(domain); err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidConfiguration, err)
		}
	}

	if c.TimeoutMax <= 0 {
		return fmt.Errorf("%w: timeout must be positive", ErrInvalidConfiguration)
	}

	// MaxReqSize must be set and within bounds for security
	if c.MaxReqSize <= 0 {
		return fmt.Errorf("%w: max request size must be set (got %d, use DefaultMaxReqSize=%d or specify explicitly)",
			ErrInvalidConfiguration, c.MaxReqSize, DefaultMaxReqSize)
	}
	if c.MaxReqSize > MaxAllowedReqSize {
		return fmt.Errorf("%w: max request size %d exceeds maximum allowed %d",
			ErrInvalidConfiguration, c.MaxReqSize, MaxAllowedReqSize)
	}

	// MaxRespSize must be set and within bounds for security
	if c.MaxRespSize <= 0 {
		return fmt.Errorf("%w: max response size must be set (got %d, use DefaultMaxRespSize=%d or specify explicitly)",
			ErrInvalidConfiguration, c.MaxRespSize, DefaultMaxRespSize)
	}
	if c.MaxRespSize > MaxAllowedRespSize {
		return fmt.Errorf("%w: max response size %d exceeds maximum allowed %d",
			ErrInvalidConfiguration, c.MaxRespSize, MaxAllowedRespSize)
	}

	// Rate limiting is required for HTTP capabilities to prevent abuse
	if c.RateLimit == nil {
		return fmt.Errorf("%w: rate limit is required (use DefaultHTTPRateLimit or specify explicitly)",
			ErrInvalidConfiguration)
	}
	if err := c.RateLimit.Validate(); err != nil {
		return fmt.Errorf("%w: invalid rate limit: %w", ErrInvalidConfiguration, err)
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
	// Check request size (MaxReqSize is always required)
	if int64(len(body)) > c.MaxReqSize {
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

	// Read response body with size limit (MaxRespSize is always required)
	limitedReader := io.LimitReader(resp.Body, c.MaxRespSize+1)
	respBody, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	if int64(len(respBody)) > c.MaxRespSize {
		return nil, fmt.Errorf("%w: response size exceeds limit %d", ErrMaxSizeExceeded, c.MaxRespSize)
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

// Dangerous domain patterns that should be blocked
var dangerousDomainPatterns = []string{
	"*",           // All domains
	"*.*",         // All domains with extension
	"*.*.*",       // All subdomains
	"*.com",       // All .com domains
	"*.net",       // All .net domains
	"*.org",       // All .org domains
	"*.io",        // All .io domains
	"*.dev",       // All .dev domains
	"*.edu",       // All .edu domains
	"*.gov",       // All .gov domains
	"*.mil",       // All .mil domains
	"localhost",   // Localhost (internal services)
	"*.localhost", // Localhost subdomains
	"127.0.0.1",   // Loopback IP
	"0.0.0.0",     // All interfaces
	"*.internal",  // Internal domains
	"*.local",     // Local network domains
	"*.intranet",  // Intranet domains
	"10.*.*.*",    // Private IP range
	"172.16.*.*",  // Private IP range
	"192.168.*.*", // Private IP range
}

// validateDomainPatternSecurity checks if a domain pattern is overly broad or dangerous
func validateDomainPatternSecurity(pattern string) error {
	// Normalize the pattern for comparison
	normalizedPattern := strings.ToLower(strings.TrimSpace(pattern))

	// Check against dangerous patterns
	for _, dangerous := range dangerousDomainPatterns {
		if normalizedPattern == dangerous {
			return fmt.Errorf("domain pattern %q is too broad and could allow access to unintended services", pattern)
		}
	}

	// Check for patterns that are just a TLD wildcard
	if strings.HasPrefix(normalizedPattern, "*.") {
		suffix := normalizedPattern[2:]
		// If the suffix has no dots, it's a TLD wildcard (e.g., *.com)
		if !strings.Contains(suffix, ".") {
			return fmt.Errorf("domain pattern %q matches all domains under a TLD, which is too broad", pattern)
		}
	}

	// Require at least a second-level domain for wildcards
	// e.g., *.example.com is ok, but *.com is not
	if normalizedPattern == "*" {
		return fmt.Errorf("domain pattern %q matches all domains, which is not allowed", pattern)
	}

	return nil
}
