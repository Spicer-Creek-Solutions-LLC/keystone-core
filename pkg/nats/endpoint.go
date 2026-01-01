package nats

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Scheme represents a NATS connection scheme
type Scheme string

const (
	// SchemeNATS is standard NATS TCP connection
	SchemeNATS Scheme = "nats"
	// SchemeTLS is NATS with TLS encryption
	SchemeTLS Scheme = "tls"
	// SchemeWS is NATS over WebSocket
	SchemeWS Scheme = "ws"
	// SchemeWSS is NATS over WebSocket with TLS
	SchemeWSS Scheme = "wss"
)

// Endpoint represents a single NATS server endpoint
type Endpoint struct {
	// URL is the original URL string
	URL string
	// Scheme is the connection scheme (nats, tls, ws, wss)
	Scheme Scheme
	// Host is the hostname or IP address
	Host string
	// Port is the port number
	Port int
	// Username for authentication (optional)
	Username string
	// Password for authentication (optional)
	Password string
	// Token for token-based auth (optional)
	Token string
	// Priority for failover ordering (lower = higher priority)
	Priority int
	// Weight for load balancing (higher = more traffic)
	Weight int
	// Tags for filtering/selection
	Tags []string
}

// EndpointConfig holds configuration for multiple NATS endpoints
type EndpointConfig struct {
	// URLs is a list of NATS server URLs
	URLs []string `yaml:"urls" json:"urls"`
	// Primary is the primary endpoint URL (optional, first URL used if not set)
	Primary string `yaml:"primary,omitempty" json:"primary,omitempty"`
	// Credentials file path (NATS .creds file)
	Credentials string `yaml:"credentials,omitempty" json:"credentials,omitempty"`
	// Token for authentication
	Token string `yaml:"token,omitempty" json:"token,omitempty"`
	// TLS configuration
	TLS EndpointTLSConfig `yaml:"tls,omitempty" json:"tls,omitempty"`
	// Connection settings
	ConnectTimeout   time.Duration `yaml:"connect_timeout,omitempty" json:"connect_timeout,omitempty"`
	ReconnectWait    time.Duration `yaml:"reconnect_wait,omitempty" json:"reconnect_wait,omitempty"`
	MaxReconnects    int           `yaml:"max_reconnects,omitempty" json:"max_reconnects,omitempty"`
	ReconnectJitter  time.Duration `yaml:"reconnect_jitter,omitempty" json:"reconnect_jitter,omitempty"`
	PingInterval     time.Duration `yaml:"ping_interval,omitempty" json:"ping_interval,omitempty"`
	MaxPingsOut      int           `yaml:"max_pings_out,omitempty" json:"max_pings_out,omitempty"`
	// Failover settings
	FailoverTimeout  time.Duration `yaml:"failover_timeout,omitempty" json:"failover_timeout,omitempty"`
	HealthCheckInterval time.Duration `yaml:"health_check_interval,omitempty" json:"health_check_interval,omitempty"`
	// Environment variable prefix for interpolation
	EnvPrefix string `yaml:"env_prefix,omitempty" json:"env_prefix,omitempty"`
}

// EndpointTLSConfig holds TLS-specific configuration
type EndpointTLSConfig struct {
	// Enabled enables TLS
	Enabled bool `yaml:"enabled" json:"enabled"`
	// CAFile is the CA certificate file
	CAFile string `yaml:"ca_file,omitempty" json:"ca_file,omitempty"`
	// CertFile is the client certificate file
	CertFile string `yaml:"cert_file,omitempty" json:"cert_file,omitempty"`
	// KeyFile is the client key file
	KeyFile string `yaml:"key_file,omitempty" json:"key_file,omitempty"`
	// InsecureSkipVerify skips server certificate verification
	InsecureSkipVerify bool `yaml:"insecure_skip_verify,omitempty" json:"insecure_skip_verify,omitempty"`
}

// DefaultEndpointConfig returns a configuration with sensible defaults
func DefaultEndpointConfig() *EndpointConfig {
	return &EndpointConfig{
		URLs:                []string{"nats://localhost:4222"},
		ConnectTimeout:      5 * time.Second,
		ReconnectWait:       2 * time.Second,
		MaxReconnects:       60,
		ReconnectJitter:     100 * time.Millisecond,
		PingInterval:        2 * time.Minute,
		MaxPingsOut:         2,
		FailoverTimeout:     10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}
}

// Validate validates the endpoint configuration
func (c *EndpointConfig) Validate() error {
	if len(c.URLs) == 0 {
		return errors.New("at least one NATS URL is required")
	}

	for i, u := range c.URLs {
		if _, err := ParseEndpoint(u); err != nil {
			return fmt.Errorf("invalid URL at index %d: %w", i, err)
		}
	}

	if c.Primary != "" {
		if _, err := ParseEndpoint(c.Primary); err != nil {
			return fmt.Errorf("invalid primary URL: %w", err)
		}
	}

	if c.ConnectTimeout < 0 {
		return errors.New("connect_timeout must be non-negative")
	}

	if c.ReconnectWait < 0 {
		return errors.New("reconnect_wait must be non-negative")
	}

	if c.MaxReconnects < -1 {
		return errors.New("max_reconnects must be -1 (unlimited) or non-negative")
	}

	if c.TLS.Enabled {
		if c.TLS.CertFile != "" && c.TLS.KeyFile == "" {
			return errors.New("cert_file requires key_file")
		}
		if c.TLS.KeyFile != "" && c.TLS.CertFile == "" {
			return errors.New("key_file requires cert_file")
		}
	}

	return nil
}

// InterpolateEnv replaces environment variable references in the configuration
func (c *EndpointConfig) InterpolateEnv() error {
	var err error

	// Interpolate URLs
	for i, u := range c.URLs {
		c.URLs[i], err = interpolateEnvString(u, c.EnvPrefix)
		if err != nil {
			return fmt.Errorf("URL[%d]: %w", i, err)
		}
	}

	// Interpolate other string fields
	c.Primary, _ = interpolateEnvString(c.Primary, c.EnvPrefix)
	c.Credentials, _ = interpolateEnvString(c.Credentials, c.EnvPrefix)
	c.Token, _ = interpolateEnvString(c.Token, c.EnvPrefix)
	c.TLS.CAFile, _ = interpolateEnvString(c.TLS.CAFile, c.EnvPrefix)
	c.TLS.CertFile, _ = interpolateEnvString(c.TLS.CertFile, c.EnvPrefix)
	c.TLS.KeyFile, _ = interpolateEnvString(c.TLS.KeyFile, c.EnvPrefix)

	return nil
}

// GetEndpoints parses all URLs and returns Endpoint objects
func (c *EndpointConfig) GetEndpoints() ([]*Endpoint, error) {
	endpoints := make([]*Endpoint, 0, len(c.URLs))

	for i, u := range c.URLs {
		ep, err := ParseEndpoint(u)
		if err != nil {
			return nil, fmt.Errorf("URL[%d]: %w", i, err)
		}
		ep.Priority = i

		// Apply global token if endpoint doesn't have one
		if ep.Token == "" && c.Token != "" {
			ep.Token = c.Token
		}

		endpoints = append(endpoints, ep)
	}

	return endpoints, nil
}

// GetPrimaryEndpoint returns the primary endpoint
func (c *EndpointConfig) GetPrimaryEndpoint() (*Endpoint, error) {
	if c.Primary != "" {
		return ParseEndpoint(c.Primary)
	}
	if len(c.URLs) > 0 {
		return ParseEndpoint(c.URLs[0])
	}
	return nil, errors.New("no endpoints configured")
}

// ParseEndpoint parses a NATS URL into an Endpoint struct
func ParseEndpoint(rawURL string) (*Endpoint, error) {
	if rawURL == "" {
		return nil, errors.New("empty URL")
	}

	// Handle URLs without scheme
	if !strings.Contains(rawURL, "://") {
		rawURL = "nats://" + rawURL
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL: %w", err)
	}

	// Validate scheme
	scheme := Scheme(strings.ToLower(parsed.Scheme))
	switch scheme {
	case SchemeNATS, SchemeTLS, SchemeWS, SchemeWSS:
		// Valid schemes
	default:
		return nil, fmt.Errorf("unsupported scheme: %s", parsed.Scheme)
	}

	// Parse host and port
	host := parsed.Hostname()
	if host == "" {
		return nil, errors.New("missing host")
	}

	port := getDefaultPort(scheme)
	if parsed.Port() != "" {
		p, err := strconv.Atoi(parsed.Port())
		if err != nil {
			return nil, fmt.Errorf("invalid port: %w", err)
		}
		if p < 1 || p > 65535 {
			return nil, fmt.Errorf("port out of range: %d", p)
		}
		port = p
	}

	endpoint := &Endpoint{
		URL:    rawURL,
		Scheme: scheme,
		Host:   host,
		Port:   port,
		Weight: 1,
	}

	// Parse username/password from URL
	if parsed.User != nil {
		endpoint.Username = parsed.User.Username()
		endpoint.Password, _ = parsed.User.Password()
	}

	// Parse token from query string
	if token := parsed.Query().Get("token"); token != "" {
		endpoint.Token = token
	}

	// Parse priority from query string
	if priority := parsed.Query().Get("priority"); priority != "" {
		if p, err := strconv.Atoi(priority); err == nil {
			endpoint.Priority = p
		}
	}

	// Parse weight from query string
	if weight := parsed.Query().Get("weight"); weight != "" {
		if w, err := strconv.Atoi(weight); err == nil && w > 0 {
			endpoint.Weight = w
		}
	}

	// Parse tags from query string
	if tags := parsed.Query().Get("tags"); tags != "" {
		endpoint.Tags = strings.Split(tags, ",")
	}

	return endpoint, nil
}

// String returns the URL string for the endpoint
func (e *Endpoint) String() string {
	if e.URL != "" {
		return e.URL
	}
	return fmt.Sprintf("%s://%s:%d", e.Scheme, e.Host, e.Port)
}

// Address returns the host:port string
func (e *Endpoint) Address() string {
	return fmt.Sprintf("%s:%d", e.Host, e.Port)
}

// IsTLS returns true if the endpoint uses TLS
func (e *Endpoint) IsTLS() bool {
	return e.Scheme == SchemeTLS || e.Scheme == SchemeWSS
}

// IsWebSocket returns true if the endpoint uses WebSocket
func (e *Endpoint) IsWebSocket() bool {
	return e.Scheme == SchemeWS || e.Scheme == SchemeWSS
}

// HasCredentials returns true if the endpoint has authentication configured
func (e *Endpoint) HasCredentials() bool {
	return e.Username != "" || e.Token != ""
}

// ToNATSURL returns the URL in NATS client format
func (e *Endpoint) ToNATSURL() string {
	var sb strings.Builder
	sb.WriteString(string(e.Scheme))
	sb.WriteString("://")

	if e.Username != "" {
		sb.WriteString(url.QueryEscape(e.Username))
		if e.Password != "" {
			sb.WriteString(":")
			sb.WriteString(url.QueryEscape(e.Password))
		}
		sb.WriteString("@")
	}

	sb.WriteString(e.Host)
	sb.WriteString(":")
	sb.WriteString(strconv.Itoa(e.Port))

	return sb.String()
}

// getDefaultPort returns the default port for a scheme
func getDefaultPort(scheme Scheme) int {
	switch scheme {
	case SchemeWS:
		return 80
	case SchemeWSS:
		return 443
	default:
		return 4222
	}
}

// envVarPattern matches ${VAR} or $VAR patterns
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}|\$([A-Za-z_][A-Za-z0-9_]*)`)

// interpolateEnvString replaces environment variable references in a string
func interpolateEnvString(s string, prefix string) (string, error) {
	if s == "" {
		return s, nil
	}

	var missingVars []string

	result := envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		var varName string
		if strings.HasPrefix(match, "${") {
			varName = match[2 : len(match)-1]
		} else {
			varName = match[1:]
		}

		// Try with prefix first
		if prefix != "" {
			if val := os.Getenv(prefix + varName); val != "" {
				return val
			}
		}

		// Try without prefix
		if val := os.Getenv(varName); val != "" {
			return val
		}

		// Handle default value syntax: ${VAR:-default}
		if idx := strings.Index(varName, ":-"); idx > 0 {
			envName := varName[:idx]
			defaultVal := varName[idx+2:]

			if prefix != "" {
				if val := os.Getenv(prefix + envName); val != "" {
					return val
				}
			}
			if val := os.Getenv(envName); val != "" {
				return val
			}
			return defaultVal
		}

		missingVars = append(missingVars, varName)
		return match
	})

	if len(missingVars) > 0 {
		return result, fmt.Errorf("undefined environment variables: %v", missingVars)
	}

	return result, nil
}

// EndpointList is a sortable list of endpoints
type EndpointList []*Endpoint

func (l EndpointList) Len() int           { return len(l) }
func (l EndpointList) Less(i, j int) bool { return l[i].Priority < l[j].Priority }
func (l EndpointList) Swap(i, j int)      { l[i], l[j] = l[j], l[i] }

// URLs returns all endpoint URLs as a string slice
func (l EndpointList) URLs() []string {
	urls := make([]string, len(l))
	for i, ep := range l {
		urls[i] = ep.ToNATSURL()
	}
	return urls
}

// FilterByScheme returns endpoints matching the given scheme
func (l EndpointList) FilterByScheme(scheme Scheme) EndpointList {
	result := make(EndpointList, 0)
	for _, ep := range l {
		if ep.Scheme == scheme {
			result = append(result, ep)
		}
	}
	return result
}

// FilterByTag returns endpoints containing the given tag
func (l EndpointList) FilterByTag(tag string) EndpointList {
	result := make(EndpointList, 0)
	for _, ep := range l {
		for _, t := range ep.Tags {
			if t == tag {
				result = append(result, ep)
				break
			}
		}
	}
	return result
}
