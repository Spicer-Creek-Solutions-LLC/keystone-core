// Package spire provides integration with SPIRE for workload identity.
// It implements the SPIFFE Workload API client for fetching SVIDs and
// trust bundles from a SPIRE Agent.
package spire

import (
	"fmt"
	"runtime"
	"time"
)

// Config configures the SPIRE Workload API client.
type Config struct {
	// SocketPath is the path to the SPIRE Agent socket.
	// Default: /run/spire/sockets/agent.sock (Linux)
	//          /var/run/spire/sockets/agent.sock (macOS)
	SocketPath string

	// Timeout is the timeout for individual API calls.
	// Default: 30 seconds
	Timeout time.Duration

	// DialTimeout is the timeout for connecting to the agent socket.
	// Default: 10 seconds
	DialTimeout time.Duration

	// RetryConfig configures retry behavior for failed operations.
	RetryConfig *RetryConfig

	// HealthCheckInterval is how often to check agent health.
	// Default: 30 seconds
	HealthCheckInterval time.Duration

	// FallbackConfig configures fallback behavior when SPIRE is unavailable.
	FallbackConfig *FallbackConfig

	// StreamBufferSize is the buffer size for streaming channels.
	// Default: 10
	StreamBufferSize int

	// TrustDomain overrides the trust domain from SPIRE.
	// If empty, uses the trust domain from SPIRE.
	TrustDomain string
}

// RetryConfig configures retry behavior.
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts.
	// Default: 3
	MaxRetries int

	// InitialDelay is the initial delay between retries.
	// Default: 100ms
	InitialDelay time.Duration

	// MaxDelay is the maximum delay between retries.
	// Default: 30 seconds
	MaxDelay time.Duration

	// Multiplier is the backoff multiplier.
	// Default: 2.0
	Multiplier float64

	// Jitter adds randomness to delay (0-1 as fraction of delay).
	// Default: 0.1
	Jitter float64
}

// FallbackConfig configures fallback behavior when SPIRE is unavailable.
type FallbackConfig struct {
	// Enabled enables fallback mode when SPIRE is unavailable.
	Enabled bool

	// FallbackProvider is the provider type to fall back to.
	// Options: "embedded", "cached", "none"
	FallbackProvider string

	// GracePeriod is how long to use cached credentials after SPIRE becomes unavailable.
	// Default: 1 hour
	GracePeriod time.Duration

	// CachePath is the path to cache credentials for fallback.
	// Default: /var/lib/keystone/identity-cache
	CachePath string

	// ReconnectInterval is how often to attempt reconnection to SPIRE.
	// Default: 1 minute
	ReconnectInterval time.Duration
}

// DefaultConfig returns a Config with default values.
func DefaultConfig() *Config {
	return &Config{
		SocketPath:          defaultSocketPath(),
		Timeout:             30 * time.Second,
		DialTimeout:         10 * time.Second,
		RetryConfig:         DefaultRetryConfig(),
		HealthCheckInterval: 30 * time.Second,
		StreamBufferSize:    10,
	}
}

// DefaultRetryConfig returns a RetryConfig with default values.
func DefaultRetryConfig() *RetryConfig {
	return &RetryConfig{
		MaxRetries:   3,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     30 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
	}
}

// DefaultFallbackConfig returns a FallbackConfig with default values.
func DefaultFallbackConfig() *FallbackConfig {
	return &FallbackConfig{
		Enabled:           false,
		FallbackProvider:  "cached",
		GracePeriod:       time.Hour,
		CachePath:         defaultCachePath(),
		ReconnectInterval: time.Minute,
	}
}

// defaultSocketPath returns the default socket path for the current OS.
func defaultSocketPath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/var/run/spire/sockets/agent.sock"
	default:
		return "/run/spire/sockets/agent.sock"
	}
}

// defaultCachePath returns the default cache path.
func defaultCachePath() string {
	switch runtime.GOOS {
	case "darwin":
		return "/var/lib/keystone/identity-cache"
	case "windows":
		return "C:\\ProgramData\\keystone\\identity-cache"
	default:
		return "/var/lib/keystone/identity-cache"
	}
}

// Validate validates the configuration.
func (c *Config) Validate() error {
	if c.SocketPath == "" {
		return fmt.Errorf("socket path cannot be empty")
	}
	if c.Timeout <= 0 {
		return fmt.Errorf("timeout must be positive")
	}
	if c.DialTimeout <= 0 {
		return fmt.Errorf("dial timeout must be positive")
	}
	if c.HealthCheckInterval <= 0 {
		return fmt.Errorf("health check interval must be positive")
	}
	if c.StreamBufferSize <= 0 {
		return fmt.Errorf("stream buffer size must be positive")
	}

	if c.RetryConfig != nil {
		if err := c.RetryConfig.Validate(); err != nil {
			return fmt.Errorf("retry config: %w", err)
		}
	}

	if c.FallbackConfig != nil {
		if err := c.FallbackConfig.Validate(); err != nil {
			return fmt.Errorf("fallback config: %w", err)
		}
	}

	return nil
}

// Validate validates the retry configuration.
func (c *RetryConfig) Validate() error {
	if c.MaxRetries < 0 {
		return fmt.Errorf("max retries cannot be negative")
	}
	if c.InitialDelay <= 0 {
		return fmt.Errorf("initial delay must be positive")
	}
	if c.MaxDelay <= 0 {
		return fmt.Errorf("max delay must be positive")
	}
	if c.Multiplier < 1 {
		return fmt.Errorf("multiplier must be at least 1")
	}
	if c.Jitter < 0 || c.Jitter > 1 {
		return fmt.Errorf("jitter must be between 0 and 1")
	}
	return nil
}

// Validate validates the fallback configuration.
func (c *FallbackConfig) Validate() error {
	if !c.Enabled {
		return nil
	}

	validProviders := map[string]bool{
		"embedded": true,
		"cached":   true,
		"none":     true,
	}
	if !validProviders[c.FallbackProvider] {
		return fmt.Errorf("invalid fallback provider: %s", c.FallbackProvider)
	}

	if c.GracePeriod <= 0 {
		return fmt.Errorf("grace period must be positive")
	}
	if c.ReconnectInterval <= 0 {
		return fmt.Errorf("reconnect interval must be positive")
	}

	return nil
}

// ApplyDefaults applies default values to any unset fields.
func (c *Config) ApplyDefaults() {
	defaults := DefaultConfig()

	if c.SocketPath == "" {
		c.SocketPath = defaults.SocketPath
	}
	if c.Timeout == 0 {
		c.Timeout = defaults.Timeout
	}
	if c.DialTimeout == 0 {
		c.DialTimeout = defaults.DialTimeout
	}
	if c.HealthCheckInterval == 0 {
		c.HealthCheckInterval = defaults.HealthCheckInterval
	}
	if c.StreamBufferSize == 0 {
		c.StreamBufferSize = defaults.StreamBufferSize
	}
	if c.RetryConfig == nil {
		c.RetryConfig = DefaultRetryConfig()
	}
}
