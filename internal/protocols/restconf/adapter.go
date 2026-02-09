// Package restconf implements the RESTCONF protocol adapter (RFC 8040)
// for managing network devices over HTTPS with YANG-modeled data.
package restconf

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/credentials"
	"github.com/shawnbutts/keystone-core/internal/protocols"
	"github.com/shawnbutts/keystone-core/internal/protocols/rest"
	"github.com/shawnbutts/keystone-core/internal/proxy"
)

func init() {
	protocols.Register(protocols.ProtocolRESTCONF, NewAdapterFactory(nil))
	protocols.RegisterRestconf(protocols.ProtocolRESTCONF, NewRestconfAdapterFactory(nil))
}

// Config contains RESTCONF adapter configuration.
type Config struct {
	*protocols.ConnectionConfig

	// Port is the RESTCONF HTTPS port (default 443).
	Port int `json:"port,omitempty"`

	// RootPath overrides the RESTCONF API root (skip discovery).
	RootPath string `json:"root_path,omitempty"`

	// Encoding is the default YANG data format (default: application/yang-data+json).
	Encoding ContentType `json:"encoding,omitempty"`

	// ValidateSSL enables TLS certificate validation (default: true).
	ValidateSSL bool `json:"validate_ssl,omitempty"`

	// DefaultHeaders are HTTP headers added to all requests.
	DefaultHeaders map[string]string `json:"default_headers,omitempty"`

	// RateLimitPerSec limits requests per second (0 = unlimited).
	RateLimitPerSec int `json:"rate_limit_per_second,omitempty"`

	// DiscoverRoot enables well-known root path discovery (default: true).
	DiscoverRoot bool `json:"discover_root,omitempty"`
}

// DefaultConfig returns a default RESTCONF configuration.
func DefaultConfig() *Config {
	return &Config{
		ConnectionConfig: protocols.DefaultConnectionConfig(),
		Port:             DefaultPort,
		Encoding:         ContentTypeYANGJSON,
		ValidateSSL:      true,
		DiscoverRoot:     true,
	}
}

// Adapter implements the RESTCONF protocol adapter.
type Adapter struct {
	config     *Config
	client     *rest.Client
	auth       rest.Authenticator
	device     *proxy.ProxiedDevice
	credential credentials.Credential
	mu         sync.RWMutex

	connected   bool
	rootPath    string
	lastError   error
	lastConnect time.Time

	metrics *protocols.AdapterMetrics
}

// NewAdapter creates a new RESTCONF adapter with the given configuration.
func NewAdapter(config *Config) *Adapter {
	if config == nil {
		config = DefaultConfig()
	}
	if config.ConnectionConfig == nil {
		config.ConnectionConfig = protocols.DefaultConnectionConfig()
	}
	if config.Port == 0 {
		config.Port = DefaultPort
	}
	if config.Encoding == "" {
		config.Encoding = ContentTypeYANGJSON
	}

	return &Adapter{
		config:  config,
		metrics: &protocols.AdapterMetrics{},
	}
}

// Type returns the protocol type.
func (a *Adapter) Type() protocols.ProtocolType {
	return protocols.ProtocolRESTCONF
}

// Connect establishes a RESTCONF session.
func (a *Adapter) Connect(ctx context.Context, device *proxy.ProxiedDevice, cred credentials.Credential) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.connected {
		return nil
	}

	port := a.config.Port
	if device.Port > 0 {
		port = device.Port
	}

	baseURL := fmt.Sprintf("https://%s:%d", device.Address, port)

	clientConfig := &rest.ClientConfig{
		BaseURL:     baseURL,
		Timeout:     a.config.Timeout,
		ValidateSSL: a.config.ValidateSSL,
		MaxRetries:  a.config.MaxRetries,
		RetryDelay:  a.config.RetryDelay,
	}
	if a.config.DefaultHeaders != nil {
		clientConfig.DefaultHeaders = a.config.DefaultHeaders
	}

	a.client = rest.NewClient(clientConfig)
	a.device = device
	a.credential = cred

	auth, err := setupAuth(cred)
	if err != nil {
		a.metrics.ConnectionErrors++
		a.lastError = err
		return fmt.Errorf("setup auth: %w", err)
	}
	a.auth = auth

	// Discover root path
	switch {
	case a.config.RootPath != "":
		a.rootPath = a.config.RootPath
	case a.config.DiscoverRoot:
		a.rootPath = discoverRootPath(ctx, a.client, a.auth)
	default:
		a.rootPath = DefaultRootPath
	}

	a.connected = true
	a.lastConnect = time.Now()
	a.metrics.ConnectionCount++

	return nil
}

// Execute executes a command on the RESTCONF device.
func (a *Adapter) Execute(ctx context.Context, req *protocols.ExecuteRequest) (*protocols.ExecuteResult, error) {
	a.mu.RLock()
	if !a.connected {
		a.mu.RUnlock()
		return nil, fmt.Errorf("not connected")
	}
	a.mu.RUnlock()

	startTime := time.Now()
	a.metrics.ExecutionCount++

	data, err := a.executeCommand(ctx, req.Command)
	endTime := time.Now()

	if err != nil {
		a.metrics.ExecutionErrors++
		return &protocols.ExecuteResult{
			ExitCode:  1,
			Error:     err.Error(),
			StartTime: startTime,
			EndTime:   endTime,
			Duration:  endTime.Sub(startTime),
		}, err
	}

	return &protocols.ExecuteResult{
		ExitCode:  0,
		Stdout:    data,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  endTime.Sub(startTime),
	}, nil
}

// Disconnect closes the RESTCONF connection.
func (a *Adapter) Disconnect(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.connected = false
	a.client = nil
	a.auth = nil
	a.device = nil
	a.credential = nil
	a.rootPath = ""

	return nil
}

// HealthCheck checks RESTCONF connectivity.
func (a *Adapter) HealthCheck(ctx context.Context) (*protocols.HealthCheckResult, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	start := time.Now()
	result := &protocols.HealthCheckResult{
		LastCheck: start,
	}

	if !a.connected {
		result.Status = "not connected"
		return result, fmt.Errorf("not connected")
	}

	url := a.rootPath + "/yang-library-version"
	_, _, _, err := a.doRequest(ctx, "GET", url, nil, string(ContentTypeYANGJSON))
	result.Latency = time.Since(start)

	if err != nil {
		result.Status = "unhealthy"
		return result, err
	}

	result.Healthy = true
	result.Status = "healthy"
	return result, nil
}

// IsConnected returns true if currently connected.
func (a *Adapter) IsConnected() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.connected
}

// Metrics returns the adapter metrics.
func (a *Adapter) Metrics() *protocols.AdapterMetrics {
	return a.metrics
}

// setupAuth creates an authenticator from a credential.
func setupAuth(cred credentials.Credential) (rest.Authenticator, error) {
	switch c := cred.(type) {
	case *credentials.RESTBasicCredential:
		return rest.NewBasicAuth(c.Username, c.Password), nil

	case *credentials.RESTBearerCredential:
		return rest.NewBearerAuth(c.Token), nil

	case *credentials.RESTAPIKeyCredential:
		location := rest.APIKeyHeader
		key := c.HeaderName
		if key == "" {
			key = "X-API-Key"
		}
		if c.QueryParam != "" {
			location = rest.APIKeyQuery
			key = c.QueryParam
		}
		return rest.NewAPIKeyAuth(key, c.APIKey, location), nil

	case *credentials.RESTOAuth2Credential:
		oauth := rest.NewOAuth2ClientCredentials(c.ClientID, c.ClientSecret, c.TokenURL)
		oauth.Scopes = c.Scopes
		return oauth, nil

	default:
		if cred == nil {
			return &rest.NoAuth{}, nil
		}
		return nil, fmt.Errorf("unsupported credential type for RESTCONF: %T", cred)
	}
}

// NewAdapterFactory returns a factory function that creates RESTCONF adapters
// compatible with the ProtocolAdapter interface.
func NewAdapterFactory(config *Config) protocols.AdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.ProtocolAdapter, error) {
		cfg := DefaultConfig()
		if config != nil {
			cfg = config
		}
		if connConfig != nil {
			cfg.ConnectionConfig = connConfig
		}
		return NewAdapter(cfg), nil
	}
}

// NewRestconfAdapterFactory returns a factory function that creates RESTCONF adapters
// compatible with the RestconfAdapter interface.
func NewRestconfAdapterFactory(config *Config) protocols.RestconfAdapterFactory {
	return func(connConfig *protocols.ConnectionConfig) (protocols.RestconfAdapter, error) {
		cfg := DefaultConfig()
		if config != nil {
			cfg = config
		}
		if connConfig != nil {
			cfg.ConnectionConfig = connConfig
		}
		return NewAdapter(cfg), nil
	}
}

// Interface compliance.
var (
	_ protocols.ProtocolAdapter = (*Adapter)(nil)
	_ protocols.RestconfAdapter = (*Adapter)(nil)
)
