package nats

import (
	"context"
	"crypto/tls"
	"fmt"
	"testing"
	"time"
)

// ============================================================================
// WebSocketConfig Tests
// ============================================================================

func TestDefaultWebSocketConfig(t *testing.T) {
	config := DefaultWebSocketConfig()

	// Default host is empty (means 0.0.0.0 when listening)
	if config.Host != "" {
		t.Errorf("Host = %v, want empty", config.Host)
	}
	if config.Port != 443 {
		t.Errorf("Port = %v, want 443", config.Port)
	}
	if config.Path != "/nats" {
		t.Errorf("Path = %v, want /nats", config.Path)
	}
	if config.HandshakeTimeout != 10*time.Second {
		t.Errorf("HandshakeTimeout = %v, want 10s", config.HandshakeTimeout)
	}
	if config.ReadBufferSize != 32*1024 {
		t.Errorf("ReadBufferSize = %v, want 32KB", config.ReadBufferSize)
	}
	if config.WriteBufferSize != 32*1024 {
		t.Errorf("WriteBufferSize = %v, want 32KB", config.WriteBufferSize)
	}
	if config.MaxMessageSize != 64*1024 {
		t.Errorf("MaxMessageSize = %v, want 64KB", config.MaxMessageSize)
	}
}

func TestWebSocketConfig_IsSecure(t *testing.T) {
	tests := []struct {
		name   string
		config WebSocketConfig
		want   bool
	}{
		{
			name:   "insecure with NoTLS",
			config: WebSocketConfig{NoTLS: true},
			want:   false,
		},
		{
			name: "with TLS config is secure",
			config: WebSocketConfig{
				TLS: &WebSocketTLSConfig{
					CertFile: "/path/to/cert",
				},
			},
			want: true,
		},
		{
			name:   "without TLS config is insecure",
			config: WebSocketConfig{NoTLS: false},
			want:   false, // No TLS config means not secure
		},
		{
			name: "TLS with NoTLS flag is insecure",
			config: WebSocketConfig{
				TLS:   &WebSocketTLSConfig{CertFile: "/path"},
				NoTLS: true,
			},
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.IsSecure()
			if got != tt.want {
				t.Errorf("IsSecure() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebSocketConfig_GetScheme(t *testing.T) {
	tests := []struct {
		name   string
		config WebSocketConfig
		want   Scheme
	}{
		{
			name:   "NoTLS returns ws",
			config: WebSocketConfig{NoTLS: true},
			want:   SchemeWS,
		},
		{
			name:   "no TLS config returns ws",
			config: WebSocketConfig{NoTLS: false},
			want:   SchemeWS, // IsSecure() is false without TLS config
		},
		{
			name: "with TLS config returns wss",
			config: WebSocketConfig{
				TLS: &WebSocketTLSConfig{CertFile: "/path"},
			},
			want: SchemeWSS,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetScheme()
			if got != tt.want {
				t.Errorf("GetScheme() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebSocketConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  WebSocketConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: WebSocketConfig{
				Host:  "example.com",
				Port:  443,
				Path:  "/",
				NoTLS: false,
			},
			wantErr: false,
		},
		{
			name: "negative port",
			config: WebSocketConfig{
				Host: "example.com",
				Port: -1,
			},
			wantErr: true,
		},
		{
			name: "port too high",
			config: WebSocketConfig{
				Host: "example.com",
				Port: 70000,
			},
			wantErr: true,
		},
		{
			name: "valid port range",
			config: WebSocketConfig{
				Host: "example.com",
				Port: 8080,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWebSocketConfig_GetListenAddress(t *testing.T) {
	tests := []struct {
		name   string
		config WebSocketConfig
		want   string
	}{
		{
			name:   "default host",
			config: WebSocketConfig{Port: 8080},
			want:   "0.0.0.0:8080",
		},
		{
			name:   "specific host",
			config: WebSocketConfig{Host: "127.0.0.1", Port: 443},
			want:   "127.0.0.1:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetListenAddress()
			if got != tt.want {
				t.Errorf("GetListenAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// WebSocketTLSConfig Tests
// ============================================================================

func TestWebSocketTLSConfig_ToTLSConfig(t *testing.T) {
	tests := []struct {
		name      string
		config    WebSocketTLSConfig
		wantErr   bool
		setupEnv  bool // Set KSCORE_ALLOW_INSECURE_TLS=1 for this test
	}{
		{
			name:    "empty config is valid",
			config:  WebSocketTLSConfig{},
			wantErr: false,
		},
		{
			name: "insecure skip verify",
			config: WebSocketTLSConfig{
				InsecureSkipVerify: true,
			},
			wantErr:  false,
			setupEnv: true, // Requires env var to allow InsecureSkipVerify
		},
		{
			name: "insecure skip verify blocked without env var",
			config: WebSocketTLSConfig{
				InsecureSkipVerify: true,
			},
			wantErr: true, // Should fail without KSCORE_ALLOW_INSECURE_TLS
		},
		{
			name: "non-existent cert file",
			config: WebSocketTLSConfig{
				CertFile: "/nonexistent/cert.pem",
				KeyFile:  "/nonexistent/key.pem",
			},
			wantErr: true,
		},
		{
			name: "non-existent CA file",
			config: WebSocketTLSConfig{
				CAFile: "/nonexistent/ca.pem",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupEnv {
				t.Setenv("KSCORE_ALLOW_INSECURE_TLS", "1")
			}
			tlsConfig, err := tt.config.ToTLSConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("ToTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if tlsConfig == nil {
					t.Error("expected non-nil TLS config")
					return
				}
				if tlsConfig.MinVersion != tls.VersionTLS12 {
					t.Errorf("MinVersion = %v, want TLS 1.2", tlsConfig.MinVersion)
				}
				if tt.config.InsecureSkipVerify != tlsConfig.InsecureSkipVerify {
					t.Errorf("InsecureSkipVerify = %v, want %v",
						tlsConfig.InsecureSkipVerify, tt.config.InsecureSkipVerify)
				}
			}
		})
	}
}

// ============================================================================
// WebSocketProxyConfig Tests
// ============================================================================

func TestWebSocketProxyConfig_ShouldBypass(t *testing.T) {
	config := WebSocketProxyConfig{
		NoProxy: []string{"localhost", "127.0.0.1", "*.internal.example.com"},
	}

	tests := []struct {
		host   string
		bypass bool
	}{
		{"localhost", true},
		{"127.0.0.1", true},
		{"api.internal.example.com", true},  // Matches *.internal.example.com
		{"internal.example.com", false},     // Doesn't match *.internal.example.com (no subdomain)
		{"external.example.com", false},
		{"proxy.example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.host, func(t *testing.T) {
			got := config.ShouldBypass(tt.host)
			if got != tt.bypass {
				t.Errorf("ShouldBypass(%s) = %v, want %v", tt.host, got, tt.bypass)
			}
		})
	}
}

func TestWebSocketProxyConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  WebSocketProxyConfig
		wantErr bool
	}{
		{
			name:    "empty config valid",
			config:  WebSocketProxyConfig{},
			wantErr: false,
		},
		{
			name: "valid proxy URL",
			config: WebSocketProxyConfig{
				URL: "http://proxy.example.com:8080",
			},
			wantErr: false,
		},
		{
			name: "invalid proxy URL",
			config: WebSocketProxyConfig{
				URL: "://invalid-url",
			},
			wantErr: true,
		},
		{
			name: "auth without credentials",
			config: WebSocketProxyConfig{
				AuthType: ProxyAuthBasic,
			},
			wantErr: true,
		},
		{
			name: "auth with credentials",
			config: WebSocketProxyConfig{
				AuthType: ProxyAuthBasic,
				Username: "user",
				Password: "pass",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestProxyAuthType_String(t *testing.T) {
	tests := []struct {
		authType ProxyAuthType
		want     string
	}{
		{ProxyAuthNone, "none"},
		{ProxyAuthBasic, "basic"},
		{ProxyAuthDigest, "digest"},
		{ProxyAuthNTLM, "ntlm"},
		{ProxyAuthType(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.authType.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

// ============================================================================
// ProxyDialer Tests
// ============================================================================

func TestNewProxyDialer(t *testing.T) {
	config := &WebSocketProxyConfig{
		URL:            "http://proxy.example.com:8080",
		AuthType:       ProxyAuthBasic,
		Username:       "user",
		Password:       "pass",
		ConnectTimeout: 30 * time.Second,
	}

	dialer := NewProxyDialer(config, nil)

	if dialer == nil {
		t.Fatal("expected non-nil ProxyDialer")
	}
	if dialer.config != config {
		t.Error("config not set correctly")
	}
}

func TestProxyDialer_Dial_InvalidURL(t *testing.T) {
	config := &WebSocketProxyConfig{
		URL: "://invalid-url",
	}

	dialer := NewProxyDialer(config, nil)

	ctx := context.Background()
	_, err := dialer.Dial(ctx, "tcp", "example.com:443")
	if err == nil {
		t.Error("expected error for invalid URL")
	}
}

// ============================================================================
// WebSocketServerConfig Tests
// ============================================================================

func TestDefaultWebSocketServerConfig(t *testing.T) {
	config := DefaultWebSocketServerConfig()

	if config.Port != 443 {
		t.Errorf("Port = %v, want 443", config.Port)
	}
	if config.Path != "/nats" {
		t.Errorf("Path = %v, want /nats", config.Path)
	}
	if !config.Compression {
		t.Error("Compression should be enabled by default")
	}
	if config.HandshakeTimeout != 10*time.Second {
		t.Errorf("HandshakeTimeout = %v, want 10s", config.HandshakeTimeout)
	}
	if config.Enabled {
		t.Error("Enabled should be false by default")
	}
}

func TestWebSocketServerConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  WebSocketServerConfig
		wantErr bool
	}{
		{
			name: "valid config with TLS",
			config: WebSocketServerConfig{
				Enabled: true,
				Port:    443,
				TLS: &WebSocketTLSConfig{
					CertFile: "/path/to/cert",
					KeyFile:  "/path/to/key",
				},
			},
			wantErr: false,
		},
		{
			name: "valid config without TLS",
			config: WebSocketServerConfig{
				Enabled: true,
				Port:    8080,
				NoTLS:   true,
			},
			wantErr: false,
		},
		{
			name: "no TLS config and NoTLS not set",
			config: WebSocketServerConfig{
				Enabled: true,
				Port:    443,
				NoTLS:   false,
			},
			wantErr: false, // Validation allows this - ToNATSWebsocket() will default to NoTLS
		},
		{
			name: "invalid port",
			config: WebSocketServerConfig{
				Enabled: true,
				Port:    -1,
				NoTLS:   true,
			},
			wantErr: true,
		},
		{
			name: "disabled config skips validation",
			config: WebSocketServerConfig{
				Enabled: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWebSocketServerConfig_GetListenAddress(t *testing.T) {
	tests := []struct {
		name   string
		config WebSocketServerConfig
		want   string
	}{
		{
			name:   "default host",
			config: WebSocketServerConfig{Port: 8080},
			want:   "0.0.0.0:8080",
		},
		{
			name:   "specific host",
			config: WebSocketServerConfig{Host: "127.0.0.1", Port: 443},
			want:   "127.0.0.1:443",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetListenAddress()
			if got != tt.want {
				t.Errorf("GetListenAddress() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebSocketServerConfig_GetURL(t *testing.T) {
	tests := []struct {
		name     string
		config   WebSocketServerConfig
		hostname string
		want     string
	}{
		{
			name: "wss URL with TLS config",
			config: WebSocketServerConfig{
				Port: 443,
				Path: "/nats",
				TLS:  &WebSocketTLSConfig{CertFile: "/path"},
			},
			hostname: "example.com",
			want:     "wss://example.com:443/nats",
		},
		{
			name: "ws URL without TLS config",
			config: WebSocketServerConfig{
				Port: 443,
				Path: "/nats",
			},
			hostname: "example.com",
			want:     "ws://example.com:443/nats", // No TLS config = ws
		},
		{
			name: "ws URL with NoTLS",
			config: WebSocketServerConfig{
				Port:  8080,
				Path:  "/",
				NoTLS: true,
			},
			hostname: "localhost",
			want:     "ws://localhost:8080/",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetURL(tt.hostname)
			if got != tt.want {
				t.Errorf("GetURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebSocketServerConfig_ToNATSWebsocket(t *testing.T) {
	config := WebSocketServerConfig{
		Enabled:          true,
		Host:             "0.0.0.0",
		Port:             8080,
		Compression:      true,
		HandshakeTimeout: 15 * time.Second,
		NoTLS:            true,
		AllowedOrigins:   []string{"https://example.com"},
		SameOrigin:       false,
		JWTCookie:        "jwt_token",
		NoAuthUser:       "anonymous",
		AuthTimeout:      30 * time.Second,
	}

	wsOpts := config.ToNATSWebsocket()

	if wsOpts.Host != "0.0.0.0" {
		t.Errorf("Host = %v, want 0.0.0.0", wsOpts.Host)
	}
	if wsOpts.Port != 8080 {
		t.Errorf("Port = %v, want 8080", wsOpts.Port)
	}
	if !wsOpts.Compression {
		t.Error("Compression should be enabled")
	}
	if wsOpts.HandshakeTimeout != 15*time.Second {
		t.Errorf("HandshakeTimeout = %v, want 15s", wsOpts.HandshakeTimeout)
	}
	if !wsOpts.NoTLS {
		t.Error("NoTLS should be true")
	}
	if len(wsOpts.AllowedOrigins) != 1 || wsOpts.AllowedOrigins[0] != "https://example.com" {
		t.Errorf("AllowedOrigins = %v, want [https://example.com]", wsOpts.AllowedOrigins)
	}
	if wsOpts.JWTCookie != "jwt_token" {
		t.Errorf("JWTCookie = %v, want jwt_token", wsOpts.JWTCookie)
	}
	if wsOpts.NoAuthUser != "anonymous" {
		t.Errorf("NoAuthUser = %v, want anonymous", wsOpts.NoAuthUser)
	}
	// AuthTimeout is in seconds as float64
	if wsOpts.AuthTimeout != 30.0 {
		t.Errorf("AuthTimeout = %v, want 30.0", wsOpts.AuthTimeout)
	}
}

// ============================================================================
// WebSocketConnection Tests
// ============================================================================

func TestNewWebSocketConnection(t *testing.T) {
	config := &WebSocketConfig{
		Host:  "example.com",
		Port:  443,
		Path:  "/nats",
		NoTLS: false,
	}

	conn, err := NewWebSocketConnection(config, nil)
	if err != nil {
		t.Fatalf("NewWebSocketConnection() error = %v", err)
	}

	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	if conn.config != config {
		t.Error("config not set correctly")
	}
	if conn.State() != WSStateDisconnected {
		t.Errorf("initial state = %v, want disconnected", conn.State())
	}
}

func TestNewWebSocketConnection_InvalidConfig(t *testing.T) {
	config := &WebSocketConfig{
		Host: "example.com",
		Port: -1, // Invalid port
	}

	_, err := NewWebSocketConnection(config, nil)
	if err == nil {
		t.Error("expected error for invalid config")
	}
}

func TestNewWebSocketConnection_DefaultConfig(t *testing.T) {
	conn, err := NewWebSocketConnection(nil, nil)
	if err != nil {
		t.Fatalf("NewWebSocketConnection() error = %v", err)
	}

	if conn == nil {
		t.Fatal("expected non-nil connection")
	}
	// Default config has empty host
	if conn.config.Host != "" {
		t.Errorf("Host = %v, want empty", conn.config.Host)
	}
}

func TestWebSocketConnection_State(t *testing.T) {
	conn := &WebSocketConnection{}

	// Initial state
	if conn.State() != WSStateDisconnected {
		t.Errorf("initial state = %v, want disconnected", conn.State())
	}

	// Set to connecting
	conn.state.Store(int32(WSStateConnecting))
	if conn.State() != WSStateConnecting {
		t.Errorf("state = %v, want connecting", conn.State())
	}

	// Set to connected
	conn.state.Store(int32(WSStateConnected))
	if conn.State() != WSStateConnected {
		t.Errorf("state = %v, want connected", conn.State())
	}
}

func TestWebSocketConnectionState_String(t *testing.T) {
	tests := []struct {
		state WebSocketConnectionState
		want  string
	}{
		{WSStateDisconnected, "disconnected"},
		{WSStateConnecting, "connecting"},
		{WSStateConnected, "connected"},
		{WSStateReconnecting, "reconnecting"},
		{WSStateClosed, "closed"},
		{WebSocketConnectionState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.state.String()
			if got != tt.want {
				t.Errorf("String() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebSocketConnection_SetCallbacks(t *testing.T) {
	config := &WebSocketConfig{
		Host:  "example.com",
		Port:  443,
		NoTLS: true,
	}
	conn, err := NewWebSocketConnection(config, nil)
	if err != nil {
		t.Fatalf("NewWebSocketConnection() error = %v", err)
	}

	stateChangeCalled := false

	conn.SetStateChangeCallback(func(state WebSocketConnectionState) {
		stateChangeCalled = true
	})

	// Notify state change to trigger callback
	conn.notifyStateChange(WSStateConnected)

	if !stateChangeCalled {
		t.Error("state change callback should have been called")
	}
}

func TestWebSocketConnection_buildURL(t *testing.T) {
	tests := []struct {
		name     string
		config   *WebSocketConfig
		endpoint *Endpoint
		want     string
	}{
		{
			name: "from config ws with NoTLS",
			config: &WebSocketConfig{
				Host:  "example.com",
				Port:  8080,
				Path:  "/nats",
				NoTLS: true,
			},
			want: "ws://example.com:8080/nats",
		},
		{
			name: "from config ws without TLS config",
			config: &WebSocketConfig{
				Host:  "secure.example.com",
				Port:  443,
				Path:  "/",
				NoTLS: false,
			},
			want: "ws://secure.example.com:443/", // No TLS config = ws
		},
		{
			name: "from config wss with TLS",
			config: &WebSocketConfig{
				Host: "secure.example.com",
				Port: 443,
				Path: "/",
				TLS:  &WebSocketTLSConfig{CertFile: "/path"},
			},
			want: "wss://secure.example.com:443/",
		},
		{
			name:   "from endpoint",
			config: &WebSocketConfig{NoTLS: true}, // Need valid config
			endpoint: &Endpoint{
				URL: "wss://endpoint.example.com:443/ws",
			},
			want: "wss://endpoint.example.com:443/ws",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			conn := &WebSocketConnection{
				config:   tt.config,
				endpoint: tt.endpoint,
			}
			got := conn.buildURL()
			if got != tt.want {
				t.Errorf("buildURL() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWebSocketConnection_Stats(t *testing.T) {
	config := &WebSocketConfig{
		Host:  "example.com",
		Port:  443,
		NoTLS: true,
	}
	conn, err := NewWebSocketConnection(config, nil)
	if err != nil {
		t.Fatalf("NewWebSocketConnection() error = %v", err)
	}

	stats := conn.Stats()

	if stats == nil {
		t.Fatal("expected non-nil stats")
	}
	if stats.State != WSStateDisconnected {
		t.Errorf("State = %v, want disconnected", stats.State)
	}
}

// ============================================================================
// WebSocketManager Tests
// ============================================================================

func TestNewWebSocketManager(t *testing.T) {
	config := &WebSocketConfig{
		Host: "example.com",
		Port: 443,
	}

	manager := NewWebSocketManager(config)

	if manager == nil {
		t.Fatal("expected non-nil manager")
	}
	if manager.config != config {
		t.Error("config not set correctly")
	}
}

func TestNewWebSocketManager_DefaultConfig(t *testing.T) {
	manager := NewWebSocketManager(nil)

	if manager == nil {
		t.Fatal("expected non-nil manager")
	}
	if manager.config == nil {
		t.Error("expected default config")
	}
	// Default config has empty host
	if manager.config.Host != "" {
		t.Errorf("Host = %v, want empty", manager.config.Host)
	}
}

func TestWebSocketManager_GetStats(t *testing.T) {
	manager := NewWebSocketManager(nil)

	// Create some mock connections
	manager.connMu.Lock()
	conn1, _ := NewWebSocketConnection(&WebSocketConfig{Host: "a", Port: 443, NoTLS: true}, nil)
	conn1.state.Store(int32(WSStateConnected))
	manager.connections["conn1"] = conn1

	conn2, _ := NewWebSocketConnection(&WebSocketConfig{Host: "b", Port: 443, NoTLS: true}, nil)
	conn2.state.Store(int32(WSStateConnecting))
	manager.connections["conn2"] = conn2

	conn3, _ := NewWebSocketConnection(&WebSocketConfig{Host: "c", Port: 443, NoTLS: true}, nil)
	conn3.state.Store(int32(WSStateClosed))
	manager.connections["conn3"] = conn3
	manager.connMu.Unlock()

	stats := manager.GetStats()

	if len(stats) != 3 {
		t.Errorf("expected 3 stats entries, got %d", len(stats))
	}
	if stats["conn1"].State != WSStateConnected {
		t.Errorf("conn1 state = %v, want connected", stats["conn1"].State)
	}
	if stats["conn2"].State != WSStateConnecting {
		t.Errorf("conn2 state = %v, want connecting", stats["conn2"].State)
	}
	if stats["conn3"].State != WSStateClosed {
		t.Errorf("conn3 state = %v, want closed", stats["conn3"].State)
	}
}

func TestWebSocketManager_GetConnection(t *testing.T) {
	manager := NewWebSocketManager(nil)

	// No connection
	conn := manager.GetConnection("test")
	if conn != nil {
		t.Error("expected nil for non-existent connection")
	}

	// Add connection
	manager.connMu.Lock()
	testConn, _ := NewWebSocketConnection(&WebSocketConfig{Host: "a", Port: 443, NoTLS: true}, nil)
	manager.connections["test"] = testConn
	manager.connMu.Unlock()

	// Get existing connection
	conn = manager.GetConnection("test")
	if conn != testConn {
		t.Error("expected to get the added connection")
	}
}

func TestWebSocketManager_RemoveConnection(t *testing.T) {
	manager := NewWebSocketManager(nil)

	// Add connection
	manager.connMu.Lock()
	testConn, _ := NewWebSocketConnection(&WebSocketConfig{Host: "a", Port: 443, NoTLS: true}, nil)
	manager.connections["test"] = testConn
	manager.connMu.Unlock()

	// Remove connection
	err := manager.RemoveConnection("test")
	if err != nil {
		t.Errorf("RemoveConnection() error = %v", err)
	}

	// Verify removed
	conn := manager.GetConnection("test")
	if conn != nil {
		t.Error("connection should have been removed")
	}
}

func TestWebSocketManager_AddConnection(t *testing.T) {
	manager := NewWebSocketManager(nil)

	endpoint := &Endpoint{
		Scheme: SchemeWSS,
		Host:   "example.com",
		Port:   443,
	}

	err := manager.AddConnection("test", endpoint)
	if err != nil {
		t.Fatalf("AddConnection() error = %v", err)
	}

	conn := manager.GetConnection("test")
	if conn == nil {
		t.Error("expected non-nil connection")
	}

	// Duplicate should error
	err = manager.AddConnection("test", endpoint)
	if err == nil {
		t.Error("expected error for duplicate connection")
	}
}

func TestWebSocketManager_Close(t *testing.T) {
	manager := NewWebSocketManager(nil)

	// Add connection
	endpoint := &Endpoint{
		Scheme: SchemeWSS,
		Host:   "example.com",
		Port:   443,
	}
	manager.AddConnection("test", endpoint)

	err := manager.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}

	// Connections should be removed
	conn := manager.GetConnection("test")
	if conn != nil {
		t.Error("connection should have been removed after close")
	}
}

// ============================================================================
// EnhancedWebSocketStrategy Tests
// ============================================================================

func TestNewEnhancedWebSocketStrategy(t *testing.T) {
	config := &StrategyConfig{}
	proxyConfig := &WebSocketProxyConfig{
		URL: "http://proxy.example.com:8080",
	}

	strategy := NewEnhancedWebSocketStrategy(config, proxyConfig)

	if strategy == nil {
		t.Fatal("expected non-nil strategy")
	}
	if strategy.Name() != "websocket-proxy" {
		t.Errorf("Name() = %v, want websocket-proxy", strategy.Name())
	}
	if strategy.Priority() != 150 {
		t.Errorf("Priority() = %v, want 150", strategy.Priority())
	}
}

func TestEnhancedWebSocketStrategy_SupportsEndpoint(t *testing.T) {
	strategy := NewEnhancedWebSocketStrategy(nil, nil)

	tests := []struct {
		scheme  Scheme
		support bool
	}{
		{SchemeWS, true},
		{SchemeWSS, true},
		{SchemeNATS, false},
		{SchemeTLS, false},
	}

	for _, tt := range tests {
		t.Run(string(tt.scheme), func(t *testing.T) {
			endpoint := &Endpoint{Scheme: tt.scheme}
			got := strategy.SupportsEndpoint(endpoint)
			if got != tt.support {
				t.Errorf("SupportsEndpoint(%s) = %v, want %v", tt.scheme, got, tt.support)
			}
		})
	}
}

func TestEnhancedWebSocketStrategy_ConfigureOptions(t *testing.T) {
	proxyConfig := &WebSocketProxyConfig{
		URL: "http://proxy.example.com:8080",
	}

	strategy := NewEnhancedWebSocketStrategy(nil, proxyConfig)

	endpoint := &Endpoint{
		Scheme: SchemeWSS,
		Host:   "example.com",
		Port:   443,
	}

	config := &EndpointConfig{}

	opts, err := strategy.ConfigureOptions(endpoint, config)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("expected non-empty options")
	}
}

func TestEnhancedWebSocketStrategy_ConfigureOptions_WithProxyAuth(t *testing.T) {
	proxyConfig := &WebSocketProxyConfig{
		URL:      "http://proxy.example.com:8080",
		AuthType: ProxyAuthBasic,
		Username: "user",
		Password: "pass",
	}

	strategy := NewEnhancedWebSocketStrategy(nil, proxyConfig)

	endpoint := &Endpoint{
		Scheme: SchemeWSS,
		Host:   "example.com",
		Port:   443,
	}

	config := &EndpointConfig{}

	opts, err := strategy.ConfigureOptions(endpoint, config)
	if err != nil {
		t.Fatalf("ConfigureOptions() error = %v", err)
	}

	if len(opts) == 0 {
		t.Error("expected non-empty options")
	}

	// Check that proxy config is set
	if proxyConfig.URL == "" {
		t.Error("proxy URL should be set")
	}
}

// ============================================================================
// Benchmarks - T6.4: WebSocket Performance Testing
// ============================================================================

func BenchmarkWebSocketConfig_Validate(b *testing.B) {
	config := &WebSocketConfig{
		Host:             "example.com",
		Port:             443,
		Path:             "/nats",
		Compression:      true,
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   32 * 1024,
		WriteBufferSize:  32 * 1024,
		MaxMessageSize:   64 * 1024,
		TLS: &WebSocketTLSConfig{
			InsecureSkipVerify: true,
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.Validate()
	}
}

func BenchmarkWebSocketConfig_BuildURL(b *testing.B) {
	conn := &WebSocketConnection{
		config: &WebSocketConfig{
			Host: "example.com",
			Port: 443,
			Path: "/nats",
			TLS:  &WebSocketTLSConfig{CertFile: "/path"},
		},
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = conn.buildURL()
	}
}

func BenchmarkWebSocketProxyConfig_ShouldBypass(b *testing.B) {
	config := &WebSocketProxyConfig{
		NoProxy: []string{
			"localhost",
			"127.0.0.1",
			"*.internal.example.com",
			"*.dev.example.com",
			"*.staging.example.com",
		},
	}

	hosts := []string{
		"localhost",
		"api.internal.example.com",
		"external.example.com",
		"service.dev.example.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		host := hosts[i%len(hosts)]
		_ = config.ShouldBypass(host)
	}
}

func BenchmarkNewWebSocketConnection(b *testing.B) {
	config := &WebSocketConfig{
		Host:             "example.com",
		Port:             443,
		Path:             "/nats",
		Compression:      true,
		HandshakeTimeout: 10 * time.Second,
		NoTLS:            true,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		conn, _ := NewWebSocketConnection(config, nil)
		if conn != nil {
			conn.Close()
		}
	}
}

func BenchmarkWebSocketConnection_BuildOptions(b *testing.B) {
	config := &WebSocketConfig{
		Host:             "example.com",
		Port:             443,
		Path:             "/nats",
		Compression:      true,
		HandshakeTimeout: 10 * time.Second,
		NoTLS:            true,
	}
	conn, _ := NewWebSocketConnection(config, nil)
	defer conn.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = conn.buildOptions()
	}
}

func BenchmarkWebSocketServerConfig_ToNATSWebsocket(b *testing.B) {
	config := &WebSocketServerConfig{
		Enabled:          true,
		Host:             "0.0.0.0",
		Port:             8080,
		Path:             "/nats",
		Compression:      true,
		HandshakeTimeout: 10 * time.Second,
		NoTLS:            true,
		AllowedOrigins:   []string{"https://example.com", "https://api.example.com"},
		SameOrigin:       false,
		JWTCookie:        "jwt_token",
		NoAuthUser:       "anonymous",
		AuthTimeout:      30 * time.Second,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = config.ToNATSWebsocket()
	}
}

func BenchmarkEnhancedWebSocketStrategy_ConfigureOptions(b *testing.B) {
	proxyConfig := &WebSocketProxyConfig{
		URL: "http://proxy.example.com:8080",
	}

	strategy := NewEnhancedWebSocketStrategy(nil, proxyConfig)

	endpoint := &Endpoint{
		Scheme: SchemeWSS,
		Host:   "example.com",
		Port:   443,
	}

	config := &EndpointConfig{}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = strategy.ConfigureOptions(endpoint, config)
	}
}

func BenchmarkWebSocketManager_AddRemove(b *testing.B) {
	manager := NewWebSocketManager(nil)

	endpoint := &Endpoint{
		Scheme: SchemeWSS,
		Host:   "example.com",
		Port:   443,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		name := fmt.Sprintf("conn-%d", i)
		_ = manager.AddConnection(name, endpoint)
		_ = manager.RemoveConnection(name)
	}
}

func BenchmarkWebSocketConnectionStats(b *testing.B) {
	config := &WebSocketConfig{
		Host:  "example.com",
		Port:  443,
		NoTLS: true,
	}
	conn, _ := NewWebSocketConnection(config, nil)
	defer conn.Close()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = conn.Stats()
	}
}

func BenchmarkWebSocketManager_GetStats(b *testing.B) {
	manager := NewWebSocketManager(nil)

	// Add multiple connections
	for i := 0; i < 100; i++ {
		endpoint := &Endpoint{
			Scheme: SchemeWSS,
			Host:   fmt.Sprintf("host-%d.example.com", i),
			Port:   443,
		}
		manager.AddConnection(fmt.Sprintf("conn-%d", i), endpoint)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = manager.GetStats()
	}
}

func BenchmarkProxyDialer_ShouldBypass(b *testing.B) {
	config := &WebSocketProxyConfig{
		URL: "http://proxy.example.com:8080",
		NoProxy: []string{
			"localhost",
			"127.0.0.1",
			"10.0.0.0/8",
			"*.internal.example.com",
			"*.local",
		},
	}

	hosts := []string{
		"api.example.com",
		"localhost",
		"service.internal.example.com",
		"external.site.com",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		host := hosts[i%len(hosts)]
		_ = config.ShouldBypass(host)
	}
}
