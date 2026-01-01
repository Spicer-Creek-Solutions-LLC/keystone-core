package nats

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/nats-io/nats.go"
)

// ConnectionStrategy defines how to establish a NATS connection
type ConnectionStrategy interface {
	// Name returns the strategy name for logging/metrics
	Name() string

	// SupportsEndpoint returns true if this strategy can connect to the endpoint
	SupportsEndpoint(endpoint *Endpoint) bool

	// ConfigureOptions returns NATS options for connecting with this strategy
	ConfigureOptions(endpoint *Endpoint, config *EndpointConfig) ([]nats.Option, error)

	// Priority returns the strategy priority (lower = preferred)
	Priority() int
}

// StrategyConfig holds configuration for connection strategies
type StrategyConfig struct {
	// TLS configuration
	TLS *TLSStrategyConfig

	// WebSocket configuration
	WebSocket *WebSocketStrategyConfig

	// LeafNode configuration
	LeafNode *LeafNodeStrategyConfig

	// Connection timeouts
	ConnectTimeout time.Duration
	ReconnectWait  time.Duration
	MaxReconnects  int
}

// TLSStrategyConfig holds TLS-specific configuration
type TLSStrategyConfig struct {
	// CACertFile is the CA certificate file path
	CACertFile string

	// CertFile is the client certificate file path
	CertFile string

	// KeyFile is the client key file path
	KeyFile string

	// InsecureSkipVerify skips server certificate verification
	InsecureSkipVerify bool

	// ServerName overrides the server name for verification
	ServerName string

	// MinVersion is the minimum TLS version (1.2 or 1.3)
	MinVersion uint16
}

// WebSocketStrategyConfig holds WebSocket-specific configuration
type WebSocketStrategyConfig struct {
	// Compression enables WebSocket compression
	Compression bool

	// CustomHeaders are custom HTTP headers for the WebSocket upgrade
	CustomHeaders map[string]string

	// ProxyURL is the HTTP proxy URL for WebSocket connections
	ProxyURL string
}

// LeafNodeStrategyConfig holds leaf node-specific configuration
type LeafNodeStrategyConfig struct {
	// RemoteURL is the remote NATS cluster URL
	RemoteURL string

	// Credentials is the path to the credentials file for the remote
	Credentials string

	// Hub indicates if this is a hub connection (vs spoke)
	Hub bool

	// DenyImports prevents importing messages from remote
	DenyImports bool

	// DenyExports prevents exporting messages to remote
	DenyExports bool
}

// DirectStrategy implements direct TCP connection to NATS
type DirectStrategy struct {
	config *StrategyConfig
}

// NewDirectStrategy creates a new direct connection strategy
func NewDirectStrategy(config *StrategyConfig) *DirectStrategy {
	if config == nil {
		config = &StrategyConfig{}
	}
	return &DirectStrategy{config: config}
}

func (s *DirectStrategy) Name() string {
	return "direct"
}

func (s *DirectStrategy) SupportsEndpoint(endpoint *Endpoint) bool {
	return endpoint.Scheme == SchemeNATS
}

func (s *DirectStrategy) ConfigureOptions(endpoint *Endpoint, config *EndpointConfig) ([]nats.Option, error) {
	opts := []nats.Option{
		nats.Name(fmt.Sprintf("kscore-client-%s", endpoint.Host)),
	}

	// Add authentication options
	authOpts := s.configureAuth(endpoint, config)
	opts = append(opts, authOpts...)

	// Add connection options
	connOpts := s.configureConnection(config)
	opts = append(opts, connOpts...)

	return opts, nil
}

func (s *DirectStrategy) configureAuth(endpoint *Endpoint, config *EndpointConfig) []nats.Option {
	var opts []nats.Option

	// Credentials file takes precedence
	if config.Credentials != "" {
		opts = append(opts, nats.UserCredentials(config.Credentials))
		return opts
	}

	// Token auth
	if endpoint.Token != "" {
		opts = append(opts, nats.Token(endpoint.Token))
	} else if config.Token != "" {
		opts = append(opts, nats.Token(config.Token))
	}

	// Username/password auth
	if endpoint.Username != "" {
		opts = append(opts, nats.UserInfo(endpoint.Username, endpoint.Password))
	}

	return opts
}

func (s *DirectStrategy) configureConnection(config *EndpointConfig) []nats.Option {
	var opts []nats.Option

	if config.ConnectTimeout > 0 {
		opts = append(opts, nats.Timeout(config.ConnectTimeout))
	} else if s.config.ConnectTimeout > 0 {
		opts = append(opts, nats.Timeout(s.config.ConnectTimeout))
	}

	if config.ReconnectWait > 0 {
		opts = append(opts, nats.ReconnectWait(config.ReconnectWait))
	} else if s.config.ReconnectWait > 0 {
		opts = append(opts, nats.ReconnectWait(s.config.ReconnectWait))
	}

	maxReconnects := config.MaxReconnects
	if maxReconnects == 0 && s.config.MaxReconnects > 0 {
		maxReconnects = s.config.MaxReconnects
	}
	if maxReconnects != 0 {
		opts = append(opts, nats.MaxReconnects(maxReconnects))
	}

	return opts
}

func (s *DirectStrategy) Priority() int {
	return 100 // Lower priority than TLS
}

// TLSStrategy implements TLS-encrypted connection to NATS
type TLSStrategy struct {
	config *StrategyConfig
}

// NewTLSStrategy creates a new TLS connection strategy
func NewTLSStrategy(config *StrategyConfig) *TLSStrategy {
	if config == nil {
		config = &StrategyConfig{}
	}
	return &TLSStrategy{config: config}
}

func (s *TLSStrategy) Name() string {
	return "tls"
}

func (s *TLSStrategy) SupportsEndpoint(endpoint *Endpoint) bool {
	return endpoint.Scheme == SchemeTLS
}

func (s *TLSStrategy) ConfigureOptions(endpoint *Endpoint, config *EndpointConfig) ([]nats.Option, error) {
	// Start with direct strategy options
	direct := NewDirectStrategy(s.config)
	opts, err := direct.ConfigureOptions(endpoint, config)
	if err != nil {
		return nil, err
	}

	// Add TLS configuration
	tlsConfig, err := s.buildTLSConfig(config)
	if err != nil {
		return nil, fmt.Errorf("TLS configuration error: %w", err)
	}

	opts = append(opts, nats.Secure(tlsConfig))

	return opts, nil
}

func (s *TLSStrategy) buildTLSConfig(config *EndpointConfig) (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	// Use strategy config TLS settings if endpoint config doesn't have them
	tlsStrategyConfig := s.config.TLS
	if tlsStrategyConfig == nil {
		tlsStrategyConfig = &TLSStrategyConfig{}
	}

	// CA certificate
	caFile := config.TLS.CAFile
	if caFile == "" && tlsStrategyConfig.CACertFile != "" {
		caFile = tlsStrategyConfig.CACertFile
	}
	if caFile != "" {
		caCert, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.New("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
	}

	// Client certificate and key
	certFile := config.TLS.CertFile
	keyFile := config.TLS.KeyFile
	if certFile == "" && tlsStrategyConfig.CertFile != "" {
		certFile = tlsStrategyConfig.CertFile
		keyFile = tlsStrategyConfig.KeyFile
	}
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// InsecureSkipVerify
	if config.TLS.InsecureSkipVerify || tlsStrategyConfig.InsecureSkipVerify {
		tlsConfig.InsecureSkipVerify = true
	}

	// Server name override
	if tlsStrategyConfig.ServerName != "" {
		tlsConfig.ServerName = tlsStrategyConfig.ServerName
	}

	// Minimum TLS version
	if tlsStrategyConfig.MinVersion != 0 {
		tlsConfig.MinVersion = tlsStrategyConfig.MinVersion
	}

	return tlsConfig, nil
}

func (s *TLSStrategy) Priority() int {
	return 50 // Preferred over direct
}

// WebSocketStrategy implements WebSocket connection to NATS
type WebSocketStrategy struct {
	config *StrategyConfig
}

// NewWebSocketStrategy creates a new WebSocket connection strategy
func NewWebSocketStrategy(config *StrategyConfig) *WebSocketStrategy {
	if config == nil {
		config = &StrategyConfig{}
	}
	return &WebSocketStrategy{config: config}
}

func (s *WebSocketStrategy) Name() string {
	return "websocket"
}

func (s *WebSocketStrategy) SupportsEndpoint(endpoint *Endpoint) bool {
	return endpoint.Scheme == SchemeWS || endpoint.Scheme == SchemeWSS
}

func (s *WebSocketStrategy) ConfigureOptions(endpoint *Endpoint, config *EndpointConfig) ([]nats.Option, error) {
	// Start with direct strategy options for auth and connection settings
	direct := NewDirectStrategy(s.config)
	opts, err := direct.ConfigureOptions(endpoint, config)
	if err != nil {
		return nil, err
	}

	// Add TLS for WSS
	if endpoint.Scheme == SchemeWSS {
		tlsStrategy := NewTLSStrategy(s.config)
		tlsConfig, err := tlsStrategy.buildTLSConfig(config)
		if err != nil {
			return nil, fmt.Errorf("TLS configuration for WSS: %w", err)
		}
		opts = append(opts, nats.Secure(tlsConfig))
	}

	// WebSocket-specific options
	wsConfig := s.config.WebSocket
	if wsConfig == nil {
		wsConfig = &WebSocketStrategyConfig{}
	}

	if wsConfig.Compression {
		opts = append(opts, nats.Compression(true))
	}

	// Custom headers would be added via ProxyPath or custom dialer
	// NATS Go client handles WebSocket upgrade automatically based on URL scheme

	return opts, nil
}

func (s *WebSocketStrategy) Priority() int {
	return 200 // Lower priority - use when TCP not available
}

// LeafNodeStrategy implements leaf node connection to remote NATS cluster
type LeafNodeStrategy struct {
	config *StrategyConfig
}

// NewLeafNodeStrategy creates a new leaf node connection strategy
func NewLeafNodeStrategy(config *StrategyConfig) *LeafNodeStrategy {
	if config == nil {
		config = &StrategyConfig{}
	}
	return &LeafNodeStrategy{config: config}
}

func (s *LeafNodeStrategy) Name() string {
	return "leafnode"
}

func (s *LeafNodeStrategy) SupportsEndpoint(endpoint *Endpoint) bool {
	// Leaf node strategy is selected based on configuration, not URL scheme
	// It uses standard nats:// or tls:// schemes
	return s.config.LeafNode != nil && s.config.LeafNode.RemoteURL != ""
}

func (s *LeafNodeStrategy) ConfigureOptions(endpoint *Endpoint, config *EndpointConfig) ([]nats.Option, error) {
	leafConfig := s.config.LeafNode
	if leafConfig == nil {
		return nil, errors.New("leaf node configuration required")
	}

	// Use TLS or direct strategy based on scheme
	var opts []nats.Option
	var err error

	if endpoint.IsTLS() {
		tlsStrategy := NewTLSStrategy(s.config)
		opts, err = tlsStrategy.ConfigureOptions(endpoint, config)
	} else {
		directStrategy := NewDirectStrategy(s.config)
		opts, err = directStrategy.ConfigureOptions(endpoint, config)
	}
	if err != nil {
		return nil, err
	}

	// Leaf node uses credentials file for remote authentication
	if leafConfig.Credentials != "" {
		opts = append(opts, nats.UserCredentials(leafConfig.Credentials))
	}

	return opts, nil
}

func (s *LeafNodeStrategy) Priority() int {
	return 300 // Lowest priority - special case
}

// StrategySelector selects the appropriate connection strategy for an endpoint
type StrategySelector struct {
	strategies []ConnectionStrategy
}

// NewStrategySelector creates a new strategy selector with the given strategies
func NewStrategySelector(strategies ...ConnectionStrategy) *StrategySelector {
	return &StrategySelector{strategies: strategies}
}

// DefaultStrategySelector creates a strategy selector with all default strategies
func DefaultStrategySelector(config *StrategyConfig) *StrategySelector {
	return NewStrategySelector(
		NewTLSStrategy(config),
		NewDirectStrategy(config),
		NewWebSocketStrategy(config),
		NewLeafNodeStrategy(config),
	)
}

// SelectStrategy returns the best strategy for the given endpoint
func (s *StrategySelector) SelectStrategy(endpoint *Endpoint) ConnectionStrategy {
	var bestStrategy ConnectionStrategy
	bestPriority := int(^uint(0) >> 1) // Max int

	for _, strategy := range s.strategies {
		if strategy.SupportsEndpoint(endpoint) && strategy.Priority() < bestPriority {
			bestStrategy = strategy
			bestPriority = strategy.Priority()
		}
	}

	return bestStrategy
}

// SelectStrategies returns all strategies that support the endpoint, sorted by priority
func (s *StrategySelector) SelectStrategies(endpoint *Endpoint) []ConnectionStrategy {
	var supported []ConnectionStrategy

	for _, strategy := range s.strategies {
		if strategy.SupportsEndpoint(endpoint) {
			supported = append(supported, strategy)
		}
	}

	// Sort by priority (simple insertion sort for small slice)
	for i := 1; i < len(supported); i++ {
		j := i
		for j > 0 && supported[j].Priority() < supported[j-1].Priority() {
			supported[j], supported[j-1] = supported[j-1], supported[j]
			j--
		}
	}

	return supported
}

// AddStrategy adds a custom strategy to the selector
func (s *StrategySelector) AddStrategy(strategy ConnectionStrategy) {
	s.strategies = append(s.strategies, strategy)
}

// StrategiesForScheme returns all strategies that support the given scheme
func (s *StrategySelector) StrategiesForScheme(scheme Scheme) []ConnectionStrategy {
	endpoint := &Endpoint{Scheme: scheme}
	return s.SelectStrategies(endpoint)
}
