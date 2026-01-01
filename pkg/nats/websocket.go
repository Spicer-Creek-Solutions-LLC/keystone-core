package nats

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
)

// ============================================================================
// WebSocket Configuration Types - T6.1
// ============================================================================

// WebSocketConfig holds comprehensive WebSocket configuration
type WebSocketConfig struct {
	// Host is the WebSocket listen host (for server mode)
	Host string

	// Port is the WebSocket listen port (default: 443 for WSS, 80 for WS)
	Port int

	// Path is the WebSocket path (default: /nats)
	Path string

	// TLS configuration for WSS
	TLS *WebSocketTLSConfig

	// Proxy configuration for client mode
	Proxy *WebSocketProxyConfig

	// Compression enables WebSocket compression
	Compression bool

	// HandshakeTimeout is the WebSocket handshake timeout
	HandshakeTimeout time.Duration

	// ReadBufferSize is the WebSocket read buffer size
	ReadBufferSize int

	// WriteBufferSize is the WebSocket write buffer size
	WriteBufferSize int

	// MaxMessageSize is the maximum WebSocket message size (0 = default)
	MaxMessageSize int64

	// AllowedOrigins for CORS (server mode)
	AllowedOrigins []string

	// CustomHeaders are custom HTTP headers for the WebSocket upgrade
	CustomHeaders map[string]string

	// NoTLS forces WS even when TLS is available (for testing)
	NoTLS bool

	// SameOrigin enforces same origin policy (server mode)
	SameOrigin bool

	// JWTCookie is the name of the cookie containing a JWT token (server mode)
	JWTCookie string

	// AuthTimeout is the auth timeout for WebSocket connections
	AuthTimeout time.Duration
}

// DefaultWebSocketConfig returns a WebSocketConfig with sensible defaults
func DefaultWebSocketConfig() *WebSocketConfig {
	return &WebSocketConfig{
		Host:             "",
		Port:             443, // Default to WSS port
		Path:             "/nats",
		Compression:      true,
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   32 * 1024,  // 32KB
		WriteBufferSize:  32 * 1024,  // 32KB
		MaxMessageSize:   64 * 1024,  // 64KB
		AllowedOrigins:   []string{}, // Empty = all origins
		SameOrigin:       false,
		AuthTimeout:      5 * time.Second,
	}
}

// Validate validates the WebSocket configuration
func (c *WebSocketConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.HandshakeTimeout < 0 {
		return fmt.Errorf("handshake timeout must be non-negative")
	}
	if c.ReadBufferSize < 0 {
		return fmt.Errorf("read buffer size must be non-negative")
	}
	if c.WriteBufferSize < 0 {
		return fmt.Errorf("write buffer size must be non-negative")
	}
	if c.MaxMessageSize < 0 {
		return fmt.Errorf("max message size must be non-negative")
	}
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return fmt.Errorf("TLS config: %w", err)
		}
	}
	if c.Proxy != nil {
		if err := c.Proxy.Validate(); err != nil {
			return fmt.Errorf("proxy config: %w", err)
		}
	}
	return nil
}

// IsSecure returns true if WSS is configured
func (c *WebSocketConfig) IsSecure() bool {
	return c.TLS != nil && !c.NoTLS
}

// GetScheme returns the appropriate WebSocket scheme
func (c *WebSocketConfig) GetScheme() Scheme {
	if c.IsSecure() {
		return SchemeWSS
	}
	return SchemeWS
}

// GetListenAddress returns the WebSocket listen address
func (c *WebSocketConfig) GetListenAddress() string {
	host := c.Host
	if host == "" {
		host = "0.0.0.0"
	}
	return fmt.Sprintf("%s:%d", host, c.Port)
}

// WebSocketTLSConfig holds TLS configuration for WebSocket
type WebSocketTLSConfig struct {
	// CertFile is the server certificate file
	CertFile string

	// KeyFile is the server private key file
	KeyFile string

	// CAFile is the CA certificate file for client verification
	CAFile string

	// InsecureSkipVerify skips certificate verification (client mode)
	InsecureSkipVerify bool

	// MinVersion is the minimum TLS version (default: 1.2)
	MinVersion uint16

	// ClientAuth specifies client authentication mode
	ClientAuth tls.ClientAuthType

	// CipherSuites is the list of cipher suites to use
	CipherSuites []uint16
}

// Validate validates the TLS configuration
func (c *WebSocketTLSConfig) Validate() error {
	if c.CertFile != "" && c.KeyFile == "" {
		return errors.New("key file required when cert file is specified")
	}
	if c.KeyFile != "" && c.CertFile == "" {
		return errors.New("cert file required when key file is specified")
	}
	return nil
}

// ToTLSConfig converts to a standard tls.Config
func (c *WebSocketTLSConfig) ToTLSConfig() (*tls.Config, error) {
	tlsConfig := &tls.Config{
		MinVersion: tls.VersionTLS12,
	}

	if c.MinVersion != 0 {
		tlsConfig.MinVersion = c.MinVersion
	}

	// Load server certificate
	if c.CertFile != "" && c.KeyFile != "" {
		cert, err := tls.LoadX509KeyPair(c.CertFile, c.KeyFile)
		if err != nil {
			return nil, fmt.Errorf("load certificate: %w", err)
		}
		tlsConfig.Certificates = []tls.Certificate{cert}
	}

	// Load CA certificate
	if c.CAFile != "" {
		caCert, err := os.ReadFile(c.CAFile)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		caCertPool := x509.NewCertPool()
		if !caCertPool.AppendCertsFromPEM(caCert) {
			return nil, errors.New("failed to parse CA certificate")
		}
		tlsConfig.RootCAs = caCertPool
		tlsConfig.ClientCAs = caCertPool
	}

	tlsConfig.InsecureSkipVerify = c.InsecureSkipVerify
	tlsConfig.ClientAuth = c.ClientAuth

	if len(c.CipherSuites) > 0 {
		tlsConfig.CipherSuites = c.CipherSuites
	}

	return tlsConfig, nil
}

// ============================================================================
// WebSocket Proxy Configuration - T6.3
// ============================================================================

// ProxyAuthType defines proxy authentication types
type ProxyAuthType int

const (
	// ProxyAuthNone indicates no authentication
	ProxyAuthNone ProxyAuthType = iota
	// ProxyAuthBasic is HTTP Basic authentication
	ProxyAuthBasic
	// ProxyAuthDigest is HTTP Digest authentication
	ProxyAuthDigest
	// ProxyAuthNTLM is NTLM authentication
	ProxyAuthNTLM
)

func (t ProxyAuthType) String() string {
	switch t {
	case ProxyAuthNone:
		return "none"
	case ProxyAuthBasic:
		return "basic"
	case ProxyAuthDigest:
		return "digest"
	case ProxyAuthNTLM:
		return "ntlm"
	default:
		return "unknown"
	}
}

// WebSocketProxyConfig holds proxy configuration
type WebSocketProxyConfig struct {
	// URL is the proxy URL (http://proxy:port)
	URL string

	// AuthType is the authentication type
	AuthType ProxyAuthType

	// Username for proxy authentication
	Username string

	// Password for proxy authentication
	Password string

	// NoProxy is a list of hosts to bypass the proxy
	NoProxy []string

	// UseEnvironment uses HTTP_PROXY/HTTPS_PROXY/NO_PROXY environment variables
	UseEnvironment bool

	// ConnectTimeout is the timeout for establishing proxy connection
	ConnectTimeout time.Duration

	// TunnelHeaders are custom headers for the CONNECT request
	TunnelHeaders map[string]string

	// KeepAlive enables keep-alive for the proxy connection
	KeepAlive bool

	// KeepAliveInterval is the keep-alive interval
	KeepAliveInterval time.Duration
}

// DefaultWebSocketProxyConfig returns a proxy config with sensible defaults
func DefaultWebSocketProxyConfig() *WebSocketProxyConfig {
	return &WebSocketProxyConfig{
		AuthType:          ProxyAuthNone,
		UseEnvironment:    true,
		ConnectTimeout:    30 * time.Second,
		KeepAlive:         true,
		KeepAliveInterval: 30 * time.Second,
	}
}

// Validate validates the proxy configuration
func (c *WebSocketProxyConfig) Validate() error {
	if c.URL != "" {
		u, err := url.Parse(c.URL)
		if err != nil {
			return fmt.Errorf("invalid proxy URL: %w", err)
		}
		if u.Scheme != "http" && u.Scheme != "https" {
			return fmt.Errorf("proxy scheme must be http or https, got %s", u.Scheme)
		}
	}
	if c.AuthType != ProxyAuthNone && c.Username == "" {
		return errors.New("username required for proxy authentication")
	}
	if c.ConnectTimeout < 0 {
		return errors.New("connect timeout must be non-negative")
	}
	return nil
}

// GetProxyURL returns the effective proxy URL
func (c *WebSocketProxyConfig) GetProxyURL() string {
	if c.URL != "" {
		return c.URL
	}
	if c.UseEnvironment {
		// Check environment variables
		if proxy := os.Getenv("HTTPS_PROXY"); proxy != "" {
			return proxy
		}
		if proxy := os.Getenv("https_proxy"); proxy != "" {
			return proxy
		}
		if proxy := os.Getenv("HTTP_PROXY"); proxy != "" {
			return proxy
		}
		if proxy := os.Getenv("http_proxy"); proxy != "" {
			return proxy
		}
	}
	return ""
}

// ShouldBypass checks if the host should bypass the proxy
func (c *WebSocketProxyConfig) ShouldBypass(host string) bool {
	// Check explicit no-proxy list
	for _, noProxy := range c.NoProxy {
		if matchHost(host, noProxy) {
			return true
		}
	}

	// Check environment NO_PROXY
	if c.UseEnvironment {
		noProxy := os.Getenv("NO_PROXY")
		if noProxy == "" {
			noProxy = os.Getenv("no_proxy")
		}
		if noProxy != "" {
			for _, pattern := range strings.Split(noProxy, ",") {
				pattern = strings.TrimSpace(pattern)
				if matchHost(host, pattern) {
					return true
				}
			}
		}
	}

	return false
}

// matchHost checks if host matches a pattern (supports wildcards)
func matchHost(host, pattern string) bool {
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		// Wildcard domain match
		suffix := pattern[1:] // Remove the *
		return strings.HasSuffix(host, suffix)
	}
	return host == pattern
}

// ============================================================================
// WebSocket Proxy Dialer - T6.3
// ============================================================================

// ProxyDialer creates connections through an HTTP CONNECT proxy
type ProxyDialer struct {
	config    *WebSocketProxyConfig
	tlsConfig *tls.Config
}

// NewProxyDialer creates a new proxy dialer
func NewProxyDialer(config *WebSocketProxyConfig, tlsConfig *tls.Config) *ProxyDialer {
	if config == nil {
		config = DefaultWebSocketProxyConfig()
	}
	return &ProxyDialer{
		config:    config,
		tlsConfig: tlsConfig,
	}
}

// Dial connects to the target through the proxy
func (d *ProxyDialer) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	// Parse target address
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}

	// Check if we should bypass proxy
	if d.config.ShouldBypass(host) {
		return d.directDial(ctx, network, addr)
	}

	// Get proxy URL
	proxyURL := d.config.GetProxyURL()
	if proxyURL == "" {
		return d.directDial(ctx, network, addr)
	}

	// Parse proxy URL
	proxy, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("parse proxy URL: %w", err)
	}

	// Connect to proxy
	proxyAddr := proxy.Host
	if proxy.Port() == "" {
		if proxy.Scheme == "https" {
			proxyAddr = net.JoinHostPort(proxy.Hostname(), "443")
		} else {
			proxyAddr = net.JoinHostPort(proxy.Hostname(), "80")
		}
	}

	// Create connection to proxy
	var dialer net.Dialer
	if d.config.ConnectTimeout > 0 {
		dialer.Timeout = d.config.ConnectTimeout
	}

	proxyConn, err := dialer.DialContext(ctx, network, proxyAddr)
	if err != nil {
		return nil, fmt.Errorf("connect to proxy: %w", err)
	}

	// Wrap with TLS if proxy is HTTPS
	if proxy.Scheme == "https" {
		proxyConn = tls.Client(proxyConn, d.tlsConfig)
	}

	// Send CONNECT request
	if err := d.sendConnect(proxyConn, addr); err != nil {
		proxyConn.Close()
		return nil, fmt.Errorf("CONNECT request: %w", err)
	}

	return proxyConn, nil
}

func (d *ProxyDialer) directDial(ctx context.Context, network, addr string) (net.Conn, error) {
	var dialer net.Dialer
	if d.config.ConnectTimeout > 0 {
		dialer.Timeout = d.config.ConnectTimeout
	}
	if d.config.KeepAlive {
		dialer.KeepAlive = d.config.KeepAliveInterval
	}
	return dialer.DialContext(ctx, network, addr)
}

func (d *ProxyDialer) sendConnect(conn net.Conn, targetAddr string) error {
	// Build CONNECT request
	req := fmt.Sprintf("CONNECT %s HTTP/1.1\r\nHost: %s\r\n", targetAddr, targetAddr)

	// Add proxy authentication
	if d.config.AuthType != ProxyAuthNone {
		authHeader := d.buildAuthHeader()
		if authHeader != "" {
			req += fmt.Sprintf("Proxy-Authorization: %s\r\n", authHeader)
		}
	}

	// Add custom tunnel headers
	for key, value := range d.config.TunnelHeaders {
		req += fmt.Sprintf("%s: %s\r\n", key, value)
	}

	req += "\r\n"

	// Send request
	if _, err := conn.Write([]byte(req)); err != nil {
		return fmt.Errorf("write CONNECT request: %w", err)
	}

	// Read response
	reader := bufio.NewReader(conn)
	resp, err := http.ReadResponse(reader, nil)
	if err != nil {
		return fmt.Errorf("read CONNECT response: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("proxy returned status %d: %s", resp.StatusCode, resp.Status)
	}

	return nil
}

func (d *ProxyDialer) buildAuthHeader() string {
	switch d.config.AuthType {
	case ProxyAuthBasic:
		credentials := base64.StdEncoding.EncodeToString(
			[]byte(d.config.Username + ":" + d.config.Password),
		)
		return "Basic " + credentials
	case ProxyAuthDigest:
		// Digest auth requires challenge-response, simplified here
		// Full implementation would parse WWW-Authenticate header
		return ""
	case ProxyAuthNTLM:
		// NTLM requires multi-step negotiation
		// This is a placeholder - full NTLM is complex
		return ""
	default:
		return ""
	}
}

// ============================================================================
// WebSocket Server Configuration - T6.2
// ============================================================================

// WebSocketServerConfig holds server-side WebSocket configuration
type WebSocketServerConfig struct {
	// Enabled enables WebSocket listener
	Enabled bool

	// Host is the listen address (empty = all interfaces)
	Host string

	// Port is the listen port
	Port int

	// Path is the WebSocket path
	Path string

	// TLS configuration
	TLS *WebSocketTLSConfig

	// AdvertiseURL is the URL to advertise to clients
	AdvertiseURL string

	// Compression enables WebSocket compression
	Compression bool

	// HandshakeTimeout is the WebSocket handshake timeout
	HandshakeTimeout time.Duration

	// NoTLS disables TLS (for testing)
	NoTLS bool

	// AllowedOrigins for CORS
	AllowedOrigins []string

	// SameOrigin enforces same origin policy
	SameOrigin bool

	// JWTCookie is the name of the cookie containing a JWT token
	JWTCookie string

	// AuthTimeout is the auth timeout for WebSocket connections
	AuthTimeout time.Duration

	// NoAuthUser is the user for unauthenticated connections
	NoAuthUser string
}

// DefaultWebSocketServerConfig returns a server config with sensible defaults
func DefaultWebSocketServerConfig() *WebSocketServerConfig {
	return &WebSocketServerConfig{
		Enabled:          false,
		Port:             443,
		Path:             "/nats",
		Compression:      true,
		HandshakeTimeout: 10 * time.Second,
		AuthTimeout:      5 * time.Second,
	}
}

// Validate validates the server configuration
func (c *WebSocketServerConfig) Validate() error {
	if c.Port < 0 || c.Port > 65535 {
		return fmt.Errorf("invalid port: %d", c.Port)
	}
	if c.TLS != nil {
		if err := c.TLS.Validate(); err != nil {
			return fmt.Errorf("TLS config: %w", err)
		}
	}
	return nil
}

// ToNATSWebsocket converts to NATS server WebSocket configuration
func (c *WebSocketServerConfig) ToNATSWebsocket() *server.WebsocketOpts {
	ws := &server.WebsocketOpts{
		Host:             c.Host,
		Port:             c.Port,
		Compression:      c.Compression,
		HandshakeTimeout: c.HandshakeTimeout,
		SameOrigin:       c.SameOrigin,
		JWTCookie:        c.JWTCookie,
		AuthTimeout:      c.AuthTimeout.Seconds(),
		NoAuthUser:       c.NoAuthUser,
	}

	// Handle path - NATS expects no leading slash for matching
	if c.Path != "" {
		ws.Host = c.Host // Host is reused for path matching in some versions
	}

	// TLS configuration
	if c.TLS != nil && !c.NoTLS {
		tlsConfig := &tls.Config{
			MinVersion: tls.VersionTLS12,
		}

		// Load client certificate if specified
		if c.TLS.CertFile != "" && c.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(c.TLS.CertFile, c.TLS.KeyFile)
			if err == nil {
				tlsConfig.Certificates = []tls.Certificate{cert}
			}
		}

		// Load CA certificate if specified
		if c.TLS.CAFile != "" {
			caCert, err := os.ReadFile(c.TLS.CAFile)
			if err == nil {
				caCertPool := x509.NewCertPool()
				caCertPool.AppendCertsFromPEM(caCert)
				tlsConfig.ClientCAs = caCertPool
				tlsConfig.ClientAuth = tls.RequireAndVerifyClientCert
			}
		}

		tlsConfig.InsecureSkipVerify = c.TLS.InsecureSkipVerify
		ws.TLSConfig = tlsConfig
	} else {
		ws.NoTLS = true
	}

	// Allowed origins
	if len(c.AllowedOrigins) > 0 {
		ws.AllowedOrigins = c.AllowedOrigins
	}

	return ws
}

// GetListenAddress returns the listen address
func (c *WebSocketServerConfig) GetListenAddress() string {
	host := c.Host
	if host == "" {
		host = "0.0.0.0"
	}
	return fmt.Sprintf("%s:%d", host, c.Port)
}

// GetURL returns the WebSocket URL for clients
func (c *WebSocketServerConfig) GetURL(hostname string) string {
	if c.AdvertiseURL != "" {
		return c.AdvertiseURL
	}

	scheme := "wss"
	if c.NoTLS || c.TLS == nil {
		scheme = "ws"
	}

	host := hostname
	if host == "" {
		host = "localhost"
	}

	path := c.Path
	if path == "" {
		path = "/nats"
	}

	return fmt.Sprintf("%s://%s:%d%s", scheme, host, c.Port, path)
}

// ============================================================================
// WebSocket Connection Manager - T6.1, T6.2
// ============================================================================

// WebSocketConnectionState represents the connection state
type WebSocketConnectionState int

const (
	// WSStateDisconnected indicates no connection
	WSStateDisconnected WebSocketConnectionState = iota
	// WSStateConnecting indicates connection in progress
	WSStateConnecting
	// WSStateConnected indicates active connection
	WSStateConnected
	// WSStateReconnecting indicates reconnection in progress
	WSStateReconnecting
	// WSStateClosed indicates connection closed
	WSStateClosed
)

func (s WebSocketConnectionState) String() string {
	switch s {
	case WSStateDisconnected:
		return "disconnected"
	case WSStateConnecting:
		return "connecting"
	case WSStateConnected:
		return "connected"
	case WSStateReconnecting:
		return "reconnecting"
	case WSStateClosed:
		return "closed"
	default:
		return "unknown"
	}
}

// WebSocketConnection wraps a NATS connection over WebSocket
type WebSocketConnection struct {
	// Configuration
	config    *WebSocketConfig
	endpoint  *Endpoint
	tlsConfig *tls.Config

	// Connection
	conn *nats.Conn
	mu   sync.RWMutex

	// State
	state          atomic.Int32
	connectTime    time.Time
	disconnectTime time.Time
	reconnects     atomic.Int64
	lastError      atomic.Value

	// Callbacks
	onStateChange func(state WebSocketConnectionState)
	onError       func(err error)

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
}

// NewWebSocketConnection creates a new WebSocket connection
func NewWebSocketConnection(config *WebSocketConfig, endpoint *Endpoint) (*WebSocketConnection, error) {
	if config == nil {
		config = DefaultWebSocketConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	wsc := &WebSocketConnection{
		config:   config,
		endpoint: endpoint,
		ctx:      ctx,
		cancel:   cancel,
	}
	wsc.state.Store(int32(WSStateDisconnected))

	// Build TLS config if needed
	if config.TLS != nil {
		tlsConfig, err := config.TLS.ToTLSConfig()
		if err != nil {
			cancel()
			return nil, fmt.Errorf("TLS config: %w", err)
		}
		wsc.tlsConfig = tlsConfig
	}

	return wsc, nil
}

// Connect establishes the WebSocket connection
func (c *WebSocketConnection) Connect() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	state := WebSocketConnectionState(c.state.Load())
	if state == WSStateConnected || state == WSStateConnecting {
		return nil
	}

	c.state.Store(int32(WSStateConnecting))
	c.notifyStateChange(WSStateConnecting)

	// Build connection options
	opts, err := c.buildOptions()
	if err != nil {
		c.state.Store(int32(WSStateDisconnected))
		c.setLastError(err)
		return fmt.Errorf("build options: %w", err)
	}

	// Build URL
	wsURL := c.buildURL()

	// Connect
	conn, err := nats.Connect(wsURL, opts...)
	if err != nil {
		c.state.Store(int32(WSStateDisconnected))
		c.setLastError(err)
		c.notifyStateChange(WSStateDisconnected)
		return fmt.Errorf("connect: %w", err)
	}

	c.conn = conn
	c.connectTime = time.Now()
	c.state.Store(int32(WSStateConnected))
	c.notifyStateChange(WSStateConnected)

	return nil
}

func (c *WebSocketConnection) buildURL() string {
	if c.endpoint != nil {
		return c.endpoint.URL
	}

	scheme := "ws"
	if c.config.IsSecure() {
		scheme = "wss"
	}

	host := c.config.Host
	if host == "" {
		host = "localhost"
	}

	path := c.config.Path
	if path == "" {
		path = "/nats"
	}

	return fmt.Sprintf("%s://%s:%d%s", scheme, host, c.config.Port, path)
}

func (c *WebSocketConnection) buildOptions() ([]nats.Option, error) {
	opts := []nats.Option{
		nats.Name("kscore-websocket-client"),
	}

	// Connection callbacks
	opts = append(opts, nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
		c.handleDisconnect(err)
	}))

	opts = append(opts, nats.ReconnectHandler(func(_ *nats.Conn) {
		c.handleReconnect()
	}))

	opts = append(opts, nats.ClosedHandler(func(_ *nats.Conn) {
		c.handleClosed()
	}))

	opts = append(opts, nats.ErrorHandler(func(_ *nats.Conn, _ *nats.Subscription, err error) {
		c.handleError(err)
	}))

	// Timeouts
	if c.config.HandshakeTimeout > 0 {
		opts = append(opts, nats.Timeout(c.config.HandshakeTimeout))
	}

	// Compression
	if c.config.Compression {
		opts = append(opts, nats.Compression(true))
	}

	// TLS for WSS
	if c.config.IsSecure() && c.tlsConfig != nil {
		opts = append(opts, nats.Secure(c.tlsConfig))
	}

	// Proxy support
	if c.config.Proxy != nil && c.config.Proxy.GetProxyURL() != "" {
		proxyDialer := NewProxyDialer(c.config.Proxy, c.tlsConfig)
		opts = append(opts, nats.SetCustomDialer(&proxyDialerWrapper{dialer: proxyDialer}))
	}

	// Custom headers are passed via URL query parameters for NATS WebSocket
	// The NATS client handles WebSocket upgrade automatically

	return opts, nil
}

// proxyDialerWrapper wraps ProxyDialer to implement nats.CustomDialer
type proxyDialerWrapper struct {
	dialer *ProxyDialer
}

func (w *proxyDialerWrapper) Dial(network, address string) (net.Conn, error) {
	return w.dialer.Dial(context.Background(), network, address)
}

func (c *WebSocketConnection) handleDisconnect(err error) {
	c.disconnectTime = time.Now()
	c.setLastError(err)

	if WebSocketConnectionState(c.state.Load()) != WSStateClosed {
		c.state.Store(int32(WSStateReconnecting))
		c.notifyStateChange(WSStateReconnecting)
	}
}

func (c *WebSocketConnection) handleReconnect() {
	c.reconnects.Add(1)
	c.connectTime = time.Now()
	c.state.Store(int32(WSStateConnected))
	c.notifyStateChange(WSStateConnected)
}

func (c *WebSocketConnection) handleClosed() {
	c.state.Store(int32(WSStateClosed))
	c.notifyStateChange(WSStateClosed)
}

func (c *WebSocketConnection) handleError(err error) {
	c.setLastError(err)
	if c.onError != nil {
		c.onError(err)
	}
}

func (c *WebSocketConnection) setLastError(err error) {
	if err != nil {
		c.lastError.Store(err)
	}
}

func (c *WebSocketConnection) notifyStateChange(state WebSocketConnectionState) {
	if c.onStateChange != nil {
		c.onStateChange(state)
	}
}

// Close closes the connection
func (c *WebSocketConnection) Close() error {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}

	c.state.Store(int32(WSStateClosed))
	return nil
}

// Conn returns the underlying NATS connection
func (c *WebSocketConnection) Conn() *nats.Conn {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.conn
}

// State returns the current connection state
func (c *WebSocketConnection) State() WebSocketConnectionState {
	return WebSocketConnectionState(c.state.Load())
}

// IsConnected returns true if connected
func (c *WebSocketConnection) IsConnected() bool {
	return c.State() == WSStateConnected && c.conn != nil && c.conn.IsConnected()
}

// LastError returns the last error
func (c *WebSocketConnection) LastError() error {
	val := c.lastError.Load()
	if val == nil {
		return nil
	}
	return val.(error)
}

// Stats returns connection statistics
func (c *WebSocketConnection) Stats() *WebSocketConnectionStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := &WebSocketConnectionStats{
		State:          c.State(),
		Reconnects:     c.reconnects.Load(),
		ConnectTime:    c.connectTime,
		DisconnectTime: c.disconnectTime,
	}

	if c.conn != nil {
		natsStats := c.conn.Stats()
		stats.InMsgs = natsStats.InMsgs
		stats.OutMsgs = natsStats.OutMsgs
		stats.InBytes = natsStats.InBytes
		stats.OutBytes = natsStats.OutBytes
	}

	return stats
}

// SetStateChangeCallback sets the state change callback
func (c *WebSocketConnection) SetStateChangeCallback(cb func(state WebSocketConnectionState)) {
	c.onStateChange = cb
}

// SetErrorCallback sets the error callback
func (c *WebSocketConnection) SetErrorCallback(cb func(err error)) {
	c.onError = cb
}

// WebSocketConnectionStats holds connection statistics
type WebSocketConnectionStats struct {
	State          WebSocketConnectionState
	Reconnects     int64
	ConnectTime    time.Time
	DisconnectTime time.Time
	InMsgs         uint64
	OutMsgs        uint64
	InBytes        uint64
	OutBytes       uint64
}

// ============================================================================
// WebSocket Manager - Manages Multiple Connections
// ============================================================================

// WebSocketManager manages WebSocket connections
type WebSocketManager struct {
	// Configuration
	config *WebSocketConfig

	// Connections
	connections map[string]*WebSocketConnection
	connMu      sync.RWMutex

	// Server configuration
	serverConfig *WebSocketServerConfig
	server       *server.Server
	serverMu     sync.RWMutex

	// State
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc

	// Callbacks
	onConnect    func(name string)
	onDisconnect func(name string, err error)
}

// NewWebSocketManager creates a new WebSocket manager
func NewWebSocketManager(config *WebSocketConfig) *WebSocketManager {
	if config == nil {
		config = DefaultWebSocketConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &WebSocketManager{
		config:      config,
		connections: make(map[string]*WebSocketConnection),
		ctx:         ctx,
		cancel:      cancel,
	}
}

// SetServerConfig sets the WebSocket server configuration
func (m *WebSocketManager) SetServerConfig(config *WebSocketServerConfig) {
	m.serverMu.Lock()
	defer m.serverMu.Unlock()
	m.serverConfig = config
}

// ConfigureNATSServer applies WebSocket configuration to a NATS server
func (m *WebSocketManager) ConfigureNATSServer(opts *server.Options) error {
	m.serverMu.RLock()
	defer m.serverMu.RUnlock()

	if m.serverConfig == nil || !m.serverConfig.Enabled {
		return nil
	}

	if err := m.serverConfig.Validate(); err != nil {
		return fmt.Errorf("invalid server config: %w", err)
	}

	opts.Websocket = *m.serverConfig.ToNATSWebsocket()
	return nil
}

// AddConnection adds a WebSocket connection
func (m *WebSocketManager) AddConnection(name string, endpoint *Endpoint) error {
	m.connMu.Lock()
	defer m.connMu.Unlock()

	if _, exists := m.connections[name]; exists {
		return fmt.Errorf("connection %s already exists", name)
	}

	conn, err := NewWebSocketConnection(m.config, endpoint)
	if err != nil {
		return fmt.Errorf("create connection: %w", err)
	}

	// Set up callbacks
	conn.SetStateChangeCallback(func(state WebSocketConnectionState) {
		switch state {
		case WSStateConnected:
			if m.onConnect != nil {
				m.onConnect(name)
			}
		case WSStateDisconnected, WSStateClosed:
			if m.onDisconnect != nil {
				m.onDisconnect(name, conn.LastError())
			}
		}
	})

	m.connections[name] = conn
	return nil
}

// RemoveConnection removes a WebSocket connection
func (m *WebSocketManager) RemoveConnection(name string) error {
	m.connMu.Lock()
	defer m.connMu.Unlock()

	conn, exists := m.connections[name]
	if !exists {
		return nil
	}

	if err := conn.Close(); err != nil {
		return fmt.Errorf("close connection: %w", err)
	}

	delete(m.connections, name)
	return nil
}

// GetConnection returns a WebSocket connection by name
func (m *WebSocketManager) GetConnection(name string) *WebSocketConnection {
	m.connMu.RLock()
	defer m.connMu.RUnlock()
	return m.connections[name]
}

// Connect connects a specific WebSocket connection
func (m *WebSocketManager) Connect(name string) error {
	m.connMu.RLock()
	conn := m.connections[name]
	m.connMu.RUnlock()

	if conn == nil {
		return fmt.Errorf("connection %s not found", name)
	}

	return conn.Connect()
}

// ConnectAll connects all WebSocket connections
func (m *WebSocketManager) ConnectAll() error {
	m.connMu.RLock()
	connections := make([]*WebSocketConnection, 0, len(m.connections))
	for _, conn := range m.connections {
		connections = append(connections, conn)
	}
	m.connMu.RUnlock()

	var errs []error
	for _, conn := range connections {
		if err := conn.Connect(); err != nil {
			errs = append(errs, err)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to connect %d connections", len(errs))
	}
	return nil
}

// Close closes all connections
func (m *WebSocketManager) Close() error {
	m.cancel()

	m.connMu.Lock()
	defer m.connMu.Unlock()

	for name, conn := range m.connections {
		conn.Close()
		delete(m.connections, name)
	}

	m.running.Store(false)
	return nil
}

// SetConnectCallback sets the connect callback
func (m *WebSocketManager) SetConnectCallback(cb func(name string)) {
	m.onConnect = cb
}

// SetDisconnectCallback sets the disconnect callback
func (m *WebSocketManager) SetDisconnectCallback(cb func(name string, err error)) {
	m.onDisconnect = cb
}

// GetStats returns statistics for all connections
func (m *WebSocketManager) GetStats() map[string]*WebSocketConnectionStats {
	m.connMu.RLock()
	defer m.connMu.RUnlock()

	stats := make(map[string]*WebSocketConnectionStats)
	for name, conn := range m.connections {
		stats[name] = conn.Stats()
	}
	return stats
}

// ============================================================================
// Enhanced WebSocket Strategy with Proxy Support
// ============================================================================

// EnhancedWebSocketStrategy extends WebSocketStrategy with proxy support
type EnhancedWebSocketStrategy struct {
	config      *StrategyConfig
	proxyConfig *WebSocketProxyConfig
}

// NewEnhancedWebSocketStrategy creates a new enhanced WebSocket strategy
func NewEnhancedWebSocketStrategy(config *StrategyConfig, proxyConfig *WebSocketProxyConfig) *EnhancedWebSocketStrategy {
	if config == nil {
		config = &StrategyConfig{}
	}
	if proxyConfig == nil {
		proxyConfig = DefaultWebSocketProxyConfig()
	}
	return &EnhancedWebSocketStrategy{
		config:      config,
		proxyConfig: proxyConfig,
	}
}

func (s *EnhancedWebSocketStrategy) Name() string {
	return "websocket-proxy"
}

func (s *EnhancedWebSocketStrategy) SupportsEndpoint(endpoint *Endpoint) bool {
	return endpoint.Scheme == SchemeWS || endpoint.Scheme == SchemeWSS
}

func (s *EnhancedWebSocketStrategy) ConfigureOptions(endpoint *Endpoint, config *EndpointConfig) ([]nats.Option, error) {
	// Start with base WebSocket strategy
	baseStrategy := NewWebSocketStrategy(s.config)
	opts, err := baseStrategy.ConfigureOptions(endpoint, config)
	if err != nil {
		return nil, err
	}

	// Add proxy support if configured
	if s.proxyConfig.GetProxyURL() != "" && !s.proxyConfig.ShouldBypass(endpoint.Host) {
		// Build TLS config for proxy
		var tlsConfig *tls.Config
		if endpoint.Scheme == SchemeWSS {
			tlsStrategy := NewTLSStrategy(s.config)
			tlsConfig, err = tlsStrategy.buildTLSConfig(config)
			if err != nil {
				return nil, fmt.Errorf("TLS config for proxy: %w", err)
			}
		}

		proxyDialer := NewProxyDialer(s.proxyConfig, tlsConfig)
		opts = append(opts, nats.SetCustomDialer(&proxyDialerWrapper{dialer: proxyDialer}))
	}

	return opts, nil
}

func (s *EnhancedWebSocketStrategy) Priority() int {
	return 150 // Between TLS (50) and basic WebSocket (200)
}
