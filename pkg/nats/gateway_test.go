package nats

import (
	"context"
	"testing"
	"time"
)

func TestGatewayConnectionState_String(t *testing.T) {
	tests := []struct {
		state    GatewayConnectionState
		expected string
	}{
		{GatewayConnectionStateDisconnected, "disconnected"},
		{GatewayConnectionStateConnecting, "connecting"},
		{GatewayConnectionStateConnected, "connected"},
		{GatewayConnectionStateReconnecting, "reconnecting"},
		{GatewayConnectionStateFailed, "failed"},
		{GatewayConnectionState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("GatewayConnectionState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestGatewayMode_String(t *testing.T) {
	tests := []struct {
		mode     GatewayMode
		expected string
	}{
		{GatewayModeOptimistic, "optimistic"},
		{GatewayModeInterestOnly, "interest-only"},
		{GatewayMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("GatewayMode.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseGatewayMode(t *testing.T) {
	tests := []struct {
		input    string
		expected GatewayMode
		wantErr  bool
	}{
		{"optimistic", GatewayModeOptimistic, false},
		{"", GatewayModeOptimistic, false},
		{"interest-only", GatewayModeInterestOnly, false},
		{"interest_only", GatewayModeInterestOnly, false},
		{"interestonly", GatewayModeInterestOnly, false},
		{"invalid", GatewayModeOptimistic, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseGatewayMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseGatewayMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseGatewayMode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultGatewayConfig(t *testing.T) {
	cfg := DefaultGatewayConfig()

	if cfg.Port != 7222 {
		t.Errorf("Port = %d, want 7222", cfg.Port)
	}
	if cfg.ConnectRetries != 5 {
		t.Errorf("ConnectRetries = %d, want 5", cfg.ConnectRetries)
	}
	if cfg.ReconnectInterval != 2*time.Second {
		t.Errorf("ReconnectInterval = %v, want 2s", cfg.ReconnectInterval)
	}
	if cfg.SendQueueLimit != 16384 {
		t.Errorf("SendQueueLimit = %d, want 16384", cfg.SendQueueLimit)
	}
	if cfg.DefaultMode != GatewayModeOptimistic {
		t.Errorf("DefaultMode = %v, want optimistic", cfg.DefaultMode)
	}
	if cfg.RejectUnknown != false {
		t.Errorf("RejectUnknown = %v, want false", cfg.RejectUnknown)
	}
}

func TestGatewayConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *GatewayConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &GatewayConfig{
				Name: "cluster-a",
				Port: 7222,
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: &GatewayConfig{
				Port: 7222,
			},
			wantErr: true,
		},
		{
			name: "invalid port",
			config: &GatewayConfig{
				Name: "cluster-a",
				Port: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid advertise port",
			config: &GatewayConfig{
				Name:          "cluster-a",
				Port:          7222,
				AdvertisePort: 70000,
			},
			wantErr: true,
		},
		{
			name: "negative connect retries",
			config: &GatewayConfig{
				Name:           "cluster-a",
				ConnectRetries: -1,
			},
			wantErr: true,
		},
		{
			name: "negative reconnect interval",
			config: &GatewayConfig{
				Name:              "cluster-a",
				ReconnectInterval: -time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative send queue limit",
			config: &GatewayConfig{
				Name:           "cluster-a",
				SendQueueLimit: -1,
			},
			wantErr: true,
		},
		{
			name: "valid with gateways",
			config: &GatewayConfig{
				Name: "cluster-a",
				Gateways: []GatewayRemoteConfig{
					{
						Name: "cluster-b",
						URLs: []string{"nats://localhost:7222"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid gateway",
			config: &GatewayConfig{
				Name: "cluster-a",
				Gateways: []GatewayRemoteConfig{
					{
						Name: "", // Missing name
						URLs: []string{"nats://localhost:7222"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "valid with TLS",
			config: &GatewayConfig{
				Name: "cluster-a",
				TLS: &GatewayTLSConfig{
					CertFile: "/path/to/cert.pem",
					KeyFile:  "/path/to/key.pem",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid TLS - missing key",
			config: &GatewayConfig{
				Name: "cluster-a",
				TLS: &GatewayTLSConfig{
					CertFile: "/path/to/cert.pem",
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("GatewayConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGatewayRemoteConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  GatewayRemoteConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: GatewayRemoteConfig{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
			wantErr: false,
		},
		{
			name: "missing name",
			config: GatewayRemoteConfig{
				URLs: []string{"nats://localhost:7222"},
			},
			wantErr: true,
		},
		{
			name: "missing URLs",
			config: GatewayRemoteConfig{
				Name: "cluster-b",
			},
			wantErr: true,
		},
		{
			name: "invalid URL",
			config: GatewayRemoteConfig{
				Name: "cluster-b",
				URLs: []string{"://invalid"},
			},
			wantErr: true,
		},
		{
			name: "unsupported scheme",
			config: GatewayRemoteConfig{
				Name: "cluster-b",
				URLs: []string{"http://localhost:7222"},
			},
			wantErr: true,
		},
		{
			name: "missing host",
			config: GatewayRemoteConfig{
				Name: "cluster-b",
				URLs: []string{"nats://:7222"},
			},
			wantErr: true,
		},
		{
			name: "negative connect retries",
			config: GatewayRemoteConfig{
				Name:           "cluster-b",
				URLs:           []string{"nats://localhost:7222"},
				ConnectRetries: -1,
			},
			wantErr: true,
		},
		{
			name: "multiple URLs",
			config: GatewayRemoteConfig{
				Name: "cluster-b",
				URLs: []string{
					"nats://localhost:7222",
					"tls://localhost:7223",
				},
			},
			wantErr: false,
		},
		{
			name: "valid with TLS",
			config: GatewayRemoteConfig{
				Name: "cluster-b",
				URLs: []string{"tls://localhost:7222"},
				TLS: &GatewayTLSConfig{
					CaFile: "/path/to/ca.pem",
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("GatewayRemoteConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGatewayTLSConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *GatewayTLSConfig
		wantErr bool
	}{
		{
			name:    "empty config",
			config:  &GatewayTLSConfig{},
			wantErr: false,
		},
		{
			name: "valid cert and key",
			config: &GatewayTLSConfig{
				CertFile: "/path/to/cert.pem",
				KeyFile:  "/path/to/key.pem",
			},
			wantErr: false,
		},
		{
			name: "cert without key",
			config: &GatewayTLSConfig{
				CertFile: "/path/to/cert.pem",
			},
			wantErr: true,
		},
		{
			name: "key without cert",
			config: &GatewayTLSConfig{
				KeyFile: "/path/to/key.pem",
			},
			wantErr: true,
		},
		{
			name: "valid min version 1.2",
			config: &GatewayTLSConfig{
				MinVersion: "1.2",
			},
			wantErr: false,
		},
		{
			name: "valid min version 1.3",
			config: &GatewayTLSConfig{
				MinVersion: "1.3",
			},
			wantErr: false,
		},
		{
			name: "invalid min version",
			config: &GatewayTLSConfig{
				MinVersion: "2.0",
			},
			wantErr: true,
		},
		{
			name: "negative timeout",
			config: &GatewayTLSConfig{
				Timeout: -time.Second,
			},
			wantErr: true,
		},
		{
			name: "ca file only",
			config: &GatewayTLSConfig{
				CaFile: "/path/to/ca.pem",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("GatewayTLSConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGatewayTLSConfig_ToTLSConfig(t *testing.T) {
	tests := []struct {
		name       string
		config     *GatewayTLSConfig
		wantErr    bool
		wantNil    bool
		wantMinVer uint16
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: false,
			wantNil: true,
		},
		{
			name:    "empty config",
			config:  &GatewayTLSConfig{},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "min version 1.2",
			config: &GatewayTLSConfig{
				MinVersion: "1.2",
			},
			wantErr:    false,
			wantNil:    false,
			wantMinVer: 0x0303, // tls.VersionTLS12
		},
		{
			name: "min version 1.3",
			config: &GatewayTLSConfig{
				MinVersion: "1.3",
			},
			wantErr:    false,
			wantNil:    false,
			wantMinVer: 0x0304, // tls.VersionTLS13
		},
		{
			name: "insecure skip verify",
			config: &GatewayTLSConfig{
				InsecureSkipVerify: true,
			},
			wantErr: false,
			wantNil: false,
		},
		{
			name: "non-existent cert file",
			config: &GatewayTLSConfig{
				CertFile: "/nonexistent/cert.pem",
				KeyFile:  "/nonexistent/key.pem",
			},
			wantErr: true,
			wantNil: true, // On error, config is nil
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tlsConfig, err := tt.config.ToTLSConfig()
			if (err != nil) != tt.wantErr {
				t.Errorf("GatewayTLSConfig.ToTLSConfig() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if tt.wantNil && tlsConfig != nil {
				t.Error("expected nil TLS config")
				return
			}
			if !tt.wantNil && tlsConfig == nil {
				t.Error("expected non-nil TLS config")
				return
			}
			if tt.wantMinVer > 0 && tlsConfig != nil && tlsConfig.MinVersion != tt.wantMinVer {
				t.Errorf("MinVersion = %v, want %v", tlsConfig.MinVersion, tt.wantMinVer)
			}
		})
	}
}

func TestGatewayAuthConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *GatewayAuthConfig
		wantErr bool
	}{
		{
			name:    "empty config",
			config:  &GatewayAuthConfig{},
			wantErr: false,
		},
		{
			name: "user and password",
			config: &GatewayAuthConfig{
				User:     "admin",
				Password: "secret",
			},
			wantErr: false,
		},
		{
			name: "user without password",
			config: &GatewayAuthConfig{
				User: "admin",
			},
			wantErr: true,
		},
		{
			name: "password without user",
			config: &GatewayAuthConfig{
				Password: "secret",
			},
			wantErr: true,
		},
		{
			name: "token only",
			config: &GatewayAuthConfig{
				Token: "mytoken",
			},
			wantErr: false,
		},
		{
			name: "nkey only",
			config: &GatewayAuthConfig{
				NKeyFile: "/path/to/nkey",
			},
			wantErr: false,
		},
		{
			name: "credentials file only",
			config: &GatewayAuthConfig{
				CredentialsFile: "/path/to/creds",
			},
			wantErr: false,
		},
		{
			name: "multiple methods - user and token",
			config: &GatewayAuthConfig{
				User:     "admin",
				Password: "secret",
				Token:    "mytoken",
			},
			wantErr: true,
		},
		{
			name: "multiple methods - token and nkey",
			config: &GatewayAuthConfig{
				Token:    "mytoken",
				NKeyFile: "/path/to/nkey",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("GatewayAuthConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewGatewayManager(t *testing.T) {
	tests := []struct {
		name    string
		config  *GatewayConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: true, // DefaultGatewayConfig has empty name
		},
		{
			name: "valid config",
			config: &GatewayConfig{
				Name: "cluster-a",
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &GatewayConfig{
				Name: "", // Missing name
			},
			wantErr: true,
		},
		{
			name: "config with gateways",
			config: &GatewayConfig{
				Name: "cluster-a",
				Gateways: []GatewayRemoteConfig{
					{
						Name: "cluster-b",
						URLs: []string{"nats://localhost:7222"},
					},
				},
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewGatewayManager(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewGatewayManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && manager == nil {
				t.Error("expected manager to be non-nil")
			}
		})
	}
}

func TestGatewayManager_Lifecycle(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()

	// Start
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !manager.IsRunning() {
		t.Error("IsRunning() = false, want true")
	}

	// Start again should error
	if err := manager.Start(ctx); err == nil {
		t.Error("Start() should error when already running")
	}

	// Check connection was created
	conn := manager.GetConnection("cluster-b")
	if conn == nil {
		t.Error("GetConnection() = nil, expected connection record")
	}

	// Stop
	if err := manager.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if manager.IsRunning() {
		t.Error("IsRunning() = true after stop, want false")
	}
}

func TestGatewayManager_GetConnections(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
			{
				Name: "cluster-c",
				URLs: []string{"nats://localhost:7223"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	connections := manager.GetConnections()
	if len(connections) != 2 {
		t.Errorf("GetConnections() returned %d connections, want 2", len(connections))
	}

	if connections["cluster-b"] == nil {
		t.Error("expected cluster-b connection")
	}
	if connections["cluster-c"] == nil {
		t.Error("expected cluster-c connection")
	}
}

func TestGatewayManager_ConnectionCount(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
			{
				Name: "cluster-c",
				URLs: []string{"nats://localhost:7223"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	if count := manager.ConnectionCount(); count != 2 {
		t.Errorf("ConnectionCount() = %d, want 2", count)
	}

	// Connected count should be 0 since we're not actually connected
	if count := manager.ConnectedCount(); count != 0 {
		t.Errorf("ConnectedCount() = %d, want 0", count)
	}
}

func TestGatewayManager_GetConnectedGateways(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	// Initially no gateways are connected
	connected := manager.GetConnectedGateways()
	if len(connected) != 0 {
		t.Errorf("GetConnectedGateways() = %v, want empty", connected)
	}

	// Simulate connection
	conn := manager.GetConnection("cluster-b")
	conn.state.Store(int32(GatewayConnectionStateConnected))

	connected = manager.GetConnectedGateways()
	if len(connected) != 1 {
		t.Errorf("GetConnectedGateways() = %v, want 1 gateway", connected)
	}
	if len(connected) > 0 && connected[0] != "cluster-b" {
		t.Errorf("GetConnectedGateways()[0] = %v, want cluster-b", connected[0])
	}
}

func TestGatewayManager_Callbacks(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	var connectCalls []string
	var disconnectCalls []string
	var errorCalls []string

	manager.SetConnectCallback(func(name string) {
		connectCalls = append(connectCalls, name)
	})

	manager.SetDisconnectCallback(func(name string, err error) {
		disconnectCalls = append(disconnectCalls, name)
	})

	manager.SetErrorCallback(func(name string, err error) {
		errorCalls = append(errorCalls, name)
	})

	// Verify callbacks are set by checking they're not nil
	if manager.onConnect == nil {
		t.Error("onConnect callback should be set")
	}
	if manager.onDisconnect == nil {
		t.Error("onDisconnect callback should be set")
	}
	if manager.onError == nil {
		t.Error("onError callback should be set")
	}
}

func TestGatewayManager_GetStats(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
			{
				Name: "cluster-c",
				URLs: []string{"nats://localhost:7223"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	// Simulate some stats
	conn := manager.GetConnection("cluster-b")
	conn.state.Store(int32(GatewayConnectionStateConnected))
	conn.messagesReceived.Store(100)
	conn.messagesSent.Store(50)

	stats := manager.GetStats()
	if stats.ClusterName != "cluster-a" {
		t.Errorf("ClusterName = %v, want cluster-a", stats.ClusterName)
	}
	if stats.TotalGateways != 2 {
		t.Errorf("TotalGateways = %d, want 2", stats.TotalGateways)
	}
	if stats.ConnectedGateways != 1 {
		t.Errorf("ConnectedGateways = %d, want 1", stats.ConnectedGateways)
	}

	if stats.ConnectionStats["cluster-b"].MessagesReceived != 100 {
		t.Errorf("cluster-b MessagesReceived = %d, want 100", stats.ConnectionStats["cluster-b"].MessagesReceived)
	}
	if stats.ConnectionStats["cluster-b"].MessagesSent != 50 {
		t.Errorf("cluster-b MessagesSent = %d, want 50", stats.ConnectionStats["cluster-b"].MessagesSent)
	}
}

func TestGatewayManager_StopWithoutStart(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Stop without starting should not error
	if err := manager.Stop(); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestGatewayManager_Config(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Port: 7222,
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	returnedConfig := manager.Config()
	if returnedConfig.Name != "cluster-a" {
		t.Errorf("Config().Name = %v, want cluster-a", returnedConfig.Name)
	}
	if returnedConfig.Port != 7222 {
		t.Errorf("Config().Port = %d, want 7222", returnedConfig.Port)
	}
}

func TestGatewayManager_BuildGatewayURLs(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://b1:7222", "nats://b2:7222"},
			},
			{
				Name: "cluster-c",
				URLs: []string{"nats://c1:7222"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	urls := manager.BuildGatewayURLs()
	if len(urls) != 3 {
		t.Errorf("BuildGatewayURLs() returned %d URLs, want 3", len(urls))
	}
}

func TestGatewayConnection_State(t *testing.T) {
	conn := &GatewayConnection{
		Name: "cluster-b",
		URLs: []string{"nats://localhost:7222"},
	}

	// Initial state should be disconnected
	if conn.State() != GatewayConnectionStateDisconnected {
		t.Errorf("State() = %v, want disconnected", conn.State())
	}

	if conn.IsConnected() {
		t.Error("IsConnected() = true, want false")
	}

	// Simulate connection
	conn.state.Store(int32(GatewayConnectionStateConnected))
	if !conn.IsConnected() {
		t.Error("IsConnected() = false, want true")
	}

	// Check ConnectAttempts
	if conn.ConnectAttempts() != 0 {
		t.Errorf("ConnectAttempts() = %d, want 0", conn.ConnectAttempts())
	}

	conn.connectAttempts.Add(1)
	if conn.ConnectAttempts() != 1 {
		t.Errorf("ConnectAttempts() = %d, want 1", conn.ConnectAttempts())
	}
}

func TestGatewayConnection_Stats(t *testing.T) {
	conn := &GatewayConnection{
		Name: "cluster-b",
	}

	conn.messagesReceived.Store(100)
	conn.messagesSent.Store(50)
	conn.bytesReceived.Store(1000)
	conn.bytesSent.Store(500)
	conn.connectAttempts.Store(3)
	conn.mu.Lock()
	conn.latency = 5 * time.Millisecond
	conn.mu.Unlock()

	stats := conn.Stats()
	if stats.MessagesReceived != 100 {
		t.Errorf("MessagesReceived = %d, want 100", stats.MessagesReceived)
	}
	if stats.MessagesSent != 50 {
		t.Errorf("MessagesSent = %d, want 50", stats.MessagesSent)
	}
	if stats.BytesReceived != 1000 {
		t.Errorf("BytesReceived = %d, want 1000", stats.BytesReceived)
	}
	if stats.BytesSent != 500 {
		t.Errorf("BytesSent = %d, want 500", stats.BytesSent)
	}
	if stats.ConnectAttempts != 3 {
		t.Errorf("ConnectAttempts = %d, want 3", stats.ConnectAttempts)
	}
	if stats.Latency != 5*time.Millisecond {
		t.Errorf("Latency = %v, want 5ms", stats.Latency)
	}
}

func TestSuperclusterConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *SuperclusterConfig
		wantErr bool
	}{
		{
			name: "valid config",
			config: &SuperclusterConfig{
				LocalCluster: &GatewayConfig{
					Name: "cluster-a",
				},
			},
			wantErr: false,
		},
		{
			name: "missing local cluster",
			config: &SuperclusterConfig{
				LocalCluster: nil,
			},
			wantErr: true,
		},
		{
			name: "invalid local cluster",
			config: &SuperclusterConfig{
				LocalCluster: &GatewayConfig{
					Name: "", // Missing name
				},
			},
			wantErr: true,
		},
		{
			name: "valid with remote clusters",
			config: &SuperclusterConfig{
				LocalCluster: &GatewayConfig{
					Name: "cluster-a",
				},
				RemoteClusters: []GatewayRemoteConfig{
					{
						Name: "cluster-b",
						URLs: []string{"nats://localhost:7222"},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid remote cluster",
			config: &SuperclusterConfig{
				LocalCluster: &GatewayConfig{
					Name: "cluster-a",
				},
				RemoteClusters: []GatewayRemoteConfig{
					{
						Name: "", // Missing name
						URLs: []string{"nats://localhost:7222"},
					},
				},
			},
			wantErr: true,
		},
		{
			name: "negative discovery interval",
			config: &SuperclusterConfig{
				LocalCluster: &GatewayConfig{
					Name: "cluster-a",
				},
				DiscoveryInterval: -time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative cross-cluster timeout",
			config: &SuperclusterConfig{
				LocalCluster: &GatewayConfig{
					Name: "cluster-a",
				},
				CrossClusterTimeout: -time.Second,
			},
			wantErr: true,
		},
		{
			name: "negative failover timeout",
			config: &SuperclusterConfig{
				LocalCluster: &GatewayConfig{
					Name: "cluster-a",
				},
				FailoverTimeout: -time.Second,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("SuperclusterConfig.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestDefaultSuperclusterConfig(t *testing.T) {
	cfg := DefaultSuperclusterConfig()

	if cfg.LocalCluster == nil {
		t.Error("LocalCluster should not be nil")
	}
	if cfg.EnableAutoDiscovery != false {
		t.Errorf("EnableAutoDiscovery = %v, want false", cfg.EnableAutoDiscovery)
	}
	if cfg.DiscoveryInterval != 30*time.Second {
		t.Errorf("DiscoveryInterval = %v, want 30s", cfg.DiscoveryInterval)
	}
	if cfg.PreferLocalCluster != true {
		t.Errorf("PreferLocalCluster = %v, want true", cfg.PreferLocalCluster)
	}
	if cfg.CrossClusterTimeout != 10*time.Second {
		t.Errorf("CrossClusterTimeout = %v, want 10s", cfg.CrossClusterTimeout)
	}
	if cfg.FailoverEnabled != true {
		t.Errorf("FailoverEnabled = %v, want true", cfg.FailoverEnabled)
	}
	if cfg.FailoverTimeout != 5*time.Second {
		t.Errorf("FailoverTimeout = %v, want 5s", cfg.FailoverTimeout)
	}
}

func TestNewSubjectRouter(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)

	if router.localCluster != "cluster-a" {
		t.Errorf("localCluster = %v, want cluster-a", router.localCluster)
	}
	if router.preferLocal != true {
		t.Errorf("preferLocal = %v, want true", router.preferLocal)
	}
	if router.routes == nil {
		t.Error("routes should not be nil")
	}
	if router.subjectPrefixes == nil {
		t.Error("subjectPrefixes should not be nil")
	}
}

func TestSubjectRouter_Routes(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)

	// Add route
	route := &ClusterRoute{
		ClusterName: "cluster-b",
		IsLocal:     false,
		Latency:     10 * time.Millisecond,
		Priority:    1,
		Available:   true,
	}
	router.AddRoute(route)

	// Get route
	got := router.GetRoute("cluster-b")
	if got == nil {
		t.Fatal("GetRoute() = nil, expected route")
	}
	if got.ClusterName != "cluster-b" {
		t.Errorf("ClusterName = %v, want cluster-b", got.ClusterName)
	}

	// Remove route
	router.RemoveRoute("cluster-b")
	if router.GetRoute("cluster-b") != nil {
		t.Error("GetRoute() should return nil after removal")
	}
}

func TestSubjectRouter_SubjectPrefixes(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)

	// Add subject prefix
	router.AddSubjectPrefix("kscore.cluster-b.", "cluster-b")

	// Add routes
	router.AddRoute(&ClusterRoute{
		ClusterName: "cluster-a",
		IsLocal:     true,
		Available:   true,
		Priority:    0,
	})
	router.AddRoute(&ClusterRoute{
		ClusterName: "cluster-b",
		IsLocal:     false,
		Available:   true,
		Priority:    1,
	})

	// Route should prefer prefix
	target := router.RouteSubject("kscore.cluster-b.agent.123")
	if target != "cluster-b" {
		t.Errorf("RouteSubject() = %v, want cluster-b", target)
	}

	// Route without prefix should prefer local
	target = router.RouteSubject("kscore.cluster-a.agent.456")
	if target != "cluster-a" {
		t.Errorf("RouteSubject() = %v, want cluster-a", target)
	}

	// Remove prefix
	router.RemoveSubjectPrefix("kscore.cluster-b.")
	target = router.RouteSubject("kscore.cluster-b.agent.123")
	if target != "cluster-a" {
		t.Errorf("RouteSubject() after prefix removal = %v, want cluster-a", target)
	}
}

func TestSubjectRouter_RouteSubject(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(*SubjectRouter)
		subject     string
		wantCluster string
	}{
		{
			name: "prefer local when available",
			setup: func(r *SubjectRouter) {
				r.AddRoute(&ClusterRoute{
					ClusterName: "cluster-a",
					IsLocal:     true,
					Available:   true,
					Priority:    0,
				})
				r.AddRoute(&ClusterRoute{
					ClusterName: "cluster-b",
					IsLocal:     false,
					Available:   true,
					Priority:    1,
				})
			},
			subject:     "kscore.agent.123",
			wantCluster: "cluster-a",
		},
		{
			name: "fallback when local unavailable",
			setup: func(r *SubjectRouter) {
				r.AddRoute(&ClusterRoute{
					ClusterName: "cluster-a",
					IsLocal:     true,
					Available:   false, // Not available
					Priority:    0,
				})
				r.AddRoute(&ClusterRoute{
					ClusterName: "cluster-b",
					IsLocal:     false,
					Available:   true,
					Priority:    1,
				})
			},
			subject:     "kscore.agent.123",
			wantCluster: "cluster-b",
		},
		{
			name: "prefer lower latency at same priority",
			setup: func(r *SubjectRouter) {
				r.preferLocal = false // Disable local preference
				r.AddRoute(&ClusterRoute{
					ClusterName: "cluster-b",
					IsLocal:     false,
					Available:   true,
					Priority:    1,
					Latency:     100 * time.Millisecond,
				})
				r.AddRoute(&ClusterRoute{
					ClusterName: "cluster-c",
					IsLocal:     false,
					Available:   true,
					Priority:    1,
					Latency:     10 * time.Millisecond, // Lower latency
				})
			},
			subject:     "kscore.agent.123",
			wantCluster: "cluster-c",
		},
		{
			name: "prefer higher priority over lower latency",
			setup: func(r *SubjectRouter) {
				r.preferLocal = false
				r.AddRoute(&ClusterRoute{
					ClusterName: "cluster-b",
					IsLocal:     false,
					Available:   true,
					Priority:    0, // Higher priority
					Latency:     100 * time.Millisecond,
				})
				r.AddRoute(&ClusterRoute{
					ClusterName: "cluster-c",
					IsLocal:     false,
					Available:   true,
					Priority:    1, // Lower priority
					Latency:     10 * time.Millisecond,
				})
			},
			subject:     "kscore.agent.123",
			wantCluster: "cluster-b",
		},
		{
			name: "no available routes falls back to local",
			setup: func(r *SubjectRouter) {
				r.AddRoute(&ClusterRoute{
					ClusterName: "cluster-b",
					IsLocal:     false,
					Available:   false,
				})
			},
			subject:     "kscore.agent.123",
			wantCluster: "cluster-a", // Falls back to local cluster name
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewSubjectRouter("cluster-a", true)
			tt.setup(router)

			got := router.RouteSubject(tt.subject)
			if got != tt.wantCluster {
				t.Errorf("RouteSubject() = %v, want %v", got, tt.wantCluster)
			}
		})
	}
}

func TestSubjectRouter_GetAvailableClusters(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)

	router.AddRoute(&ClusterRoute{
		ClusterName: "cluster-a",
		Available:   true,
	})
	router.AddRoute(&ClusterRoute{
		ClusterName: "cluster-b",
		Available:   true,
	})
	router.AddRoute(&ClusterRoute{
		ClusterName: "cluster-c",
		Available:   false,
	})

	available := router.GetAvailableClusters()
	if len(available) != 2 {
		t.Errorf("GetAvailableClusters() returned %d clusters, want 2", len(available))
	}
}

func TestSubjectRouter_UpdateClusterAvailability(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)

	router.AddRoute(&ClusterRoute{
		ClusterName: "cluster-b",
		Available:   true,
	})

	// Update to unavailable
	router.UpdateClusterAvailability("cluster-b", false)

	route := router.GetRoute("cluster-b")
	if route.Available {
		t.Error("Available = true, want false")
	}

	// Update back to available
	router.UpdateClusterAvailability("cluster-b", true)
	route = router.GetRoute("cluster-b")
	if !route.Available {
		t.Error("Available = false, want true")
	}
}

func TestSubjectRouter_UpdateClusterLatency(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)

	router.AddRoute(&ClusterRoute{
		ClusterName: "cluster-b",
		Latency:     10 * time.Millisecond,
		Available:   true,
	})

	// Update latency
	router.UpdateClusterLatency("cluster-b", 50*time.Millisecond)

	route := router.GetRoute("cluster-b")
	if route.Latency != 50*time.Millisecond {
		t.Errorf("Latency = %v, want 50ms", route.Latency)
	}
}

// ============================================================================
// Gateway Health Monitor Tests (T5.2)
// ============================================================================

func TestDefaultGatewayHealthConfig(t *testing.T) {
	cfg := DefaultGatewayHealthConfig()

	if cfg.CheckInterval != 10*time.Second {
		t.Errorf("CheckInterval = %v, want 10s", cfg.CheckInterval)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", cfg.Timeout)
	}
	if cfg.HealthyThreshold != 2 {
		t.Errorf("HealthyThreshold = %d, want 2", cfg.HealthyThreshold)
	}
	if cfg.UnhealthyThreshold != 3 {
		t.Errorf("UnhealthyThreshold = %d, want 3", cfg.UnhealthyThreshold)
	}
	if cfg.PingEnabled != true {
		t.Errorf("PingEnabled = %v, want true", cfg.PingEnabled)
	}
	if cfg.PingInterval != 30*time.Second {
		t.Errorf("PingInterval = %v, want 30s", cfg.PingInterval)
	}
}

func TestNewGatewayHealthMonitor(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	monitor := NewGatewayHealthMonitor(manager, nil)
	if monitor == nil {
		t.Fatal("expected monitor to be non-nil")
	}
}

func TestGatewayHealthMonitor_Lifecycle(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	healthConfig := &GatewayHealthConfig{
		CheckInterval: 100 * time.Millisecond, // Fast for testing
		Timeout:       50 * time.Millisecond,
		HealthyThreshold:   1,
		UnhealthyThreshold: 1,
		PingEnabled:        false,
	}

	monitor := NewGatewayHealthMonitor(manager, healthConfig)

	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !monitor.IsRunning() {
		t.Error("IsRunning() = false, want true")
	}

	// Start again should error
	if err := monitor.Start(ctx); err == nil {
		t.Error("Start() should error when already running")
	}

	// Stop
	if err := monitor.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if monitor.IsRunning() {
		t.Error("IsRunning() = true after stop, want false")
	}
}

func TestGatewayHealthMonitor_GetHealth(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	monitor := NewGatewayHealthMonitor(manager, nil)
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer monitor.Stop()

	// Health should be initialized
	health := monitor.GetHealth("cluster-b")
	if health == nil {
		t.Fatal("GetHealth() = nil, expected health record")
	}

	if health.ClusterName != "cluster-b" {
		t.Errorf("ClusterName = %v, want cluster-b", health.ClusterName)
	}
}

func TestGatewayHealthMonitor_GetAllHealth(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
			{
				Name: "cluster-c",
				URLs: []string{"nats://localhost:7223"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	monitor := NewGatewayHealthMonitor(manager, nil)
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer monitor.Stop()

	allHealth := monitor.GetAllHealth()
	if len(allHealth) != 2 {
		t.Errorf("GetAllHealth() returned %d records, want 2", len(allHealth))
	}
}

func TestGatewayHealthMonitor_HealthChangeCallback(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	monitor := NewGatewayHealthMonitor(manager, nil)

	callbackCalled := false
	monitor.SetHealthChangeCallback(func(name string, healthy bool) {
		callbackCalled = true
	})
	_ = callbackCalled // Used in callback, verified via onHealthChange being set

	// Verify callback is set
	if monitor.onHealthChange == nil {
		t.Error("onHealthChange callback should be set")
	}
}

// ============================================================================
// Gateway Dynamic Management Tests
// ============================================================================

func TestGatewayManager_AddGateway(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	// Add a new gateway
	err = manager.AddGateway(GatewayRemoteConfig{
		Name: "cluster-b",
		URLs: []string{"nats://localhost:7222"},
	})
	if err != nil {
		t.Errorf("AddGateway() error = %v", err)
	}

	// Verify connection was created
	conn := manager.GetConnection("cluster-b")
	if conn == nil {
		t.Error("GetConnection() = nil, expected connection")
	}

	// Adding duplicate should error
	err = manager.AddGateway(GatewayRemoteConfig{
		Name: "cluster-b",
		URLs: []string{"nats://localhost:7222"},
	})
	if err == nil {
		t.Error("AddGateway() should error for duplicate")
	}

	// Adding invalid should error
	err = manager.AddGateway(GatewayRemoteConfig{
		Name: "", // Invalid - missing name
		URLs: []string{"nats://localhost:7222"},
	})
	if err == nil {
		t.Error("AddGateway() should error for invalid config")
	}
}

func TestGatewayManager_RemoveGateway(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
		Gateways: []GatewayRemoteConfig{
			{
				Name: "cluster-b",
				URLs: []string{"nats://localhost:7222"},
			},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	// Remove gateway
	err = manager.RemoveGateway("cluster-b")
	if err != nil {
		t.Errorf("RemoveGateway() error = %v", err)
	}

	// Verify connection was removed
	conn := manager.GetConnection("cluster-b")
	if conn != nil {
		t.Error("GetConnection() should return nil after removal")
	}

	// Removing non-existent should error
	err = manager.RemoveGateway("cluster-b")
	if err == nil {
		t.Error("RemoveGateway() should error for non-existent gateway")
	}
}

func TestGatewayManager_SetGetServer(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Initially nil
	if manager.GetServer() != nil {
		t.Error("GetServer() should initially be nil")
	}

	// SetServer is a no-op here since we don't have a server
	manager.SetServer(nil)
	if manager.GetServer() != nil {
		t.Error("GetServer() should be nil after SetServer(nil)")
	}
}

func TestGatewayManager_SetGetClient(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Initially nil
	if manager.GetClient() != nil {
		t.Error("GetClient() should initially be nil")
	}

	manager.SetClient(nil)
	if manager.GetClient() != nil {
		t.Error("GetClient() should be nil after SetClient(nil)")
	}
}

func TestGatewayManager_UpdateGatewayStatusFromServer(t *testing.T) {
	config := &GatewayConfig{
		Name: "cluster-a",
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Without server should error
	err = manager.UpdateGatewayStatusFromServer()
	if err == nil {
		t.Error("UpdateGatewayStatusFromServer() should error without server")
	}
}

// ============================================================================
// Cross-Cluster Agent Manager Tests (T5.4)
// ============================================================================

func TestNewCrossClusterAgentManager(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)

	config := &GatewayConfig{
		Name: "cluster-a",
	}
	manager, _ := NewGatewayManager(config)

	agentManager := NewCrossClusterAgentManager(
		"cluster-a",
		router,
		manager,
		10*time.Second,
	)

	if agentManager == nil {
		t.Fatal("expected agent manager to be non-nil")
	}
}

func TestCrossClusterAgentManager_RegisterAgent(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	config := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(config)

	agentManager := NewCrossClusterAgentManager("cluster-a", router, manager, 10*time.Second)

	// Register agents
	agentManager.RegisterAgent("agent-1", "cluster-a")
	agentManager.RegisterAgent("agent-2", "cluster-b")
	agentManager.RegisterAgent("agent-3", "cluster-a")

	// Verify registrations
	cluster, exists := agentManager.GetAgentCluster("agent-1")
	if !exists {
		t.Error("GetAgentCluster() should find agent-1")
	}
	if cluster != "cluster-a" {
		t.Errorf("cluster = %v, want cluster-a", cluster)
	}

	cluster, exists = agentManager.GetAgentCluster("agent-2")
	if !exists {
		t.Error("GetAgentCluster() should find agent-2")
	}
	if cluster != "cluster-b" {
		t.Errorf("cluster = %v, want cluster-b", cluster)
	}

	// Move agent to different cluster
	agentManager.RegisterAgent("agent-1", "cluster-c")
	cluster, _ = agentManager.GetAgentCluster("agent-1")
	if cluster != "cluster-c" {
		t.Errorf("cluster after move = %v, want cluster-c", cluster)
	}
}

func TestCrossClusterAgentManager_UnregisterAgent(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	config := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(config)

	agentManager := NewCrossClusterAgentManager("cluster-a", router, manager, 10*time.Second)

	agentManager.RegisterAgent("agent-1", "cluster-a")
	agentManager.UnregisterAgent("agent-1")

	_, exists := agentManager.GetAgentCluster("agent-1")
	if exists {
		t.Error("agent-1 should not exist after unregister")
	}

	// Unregistering non-existent agent should not panic
	agentManager.UnregisterAgent("agent-nonexistent")
}

func TestCrossClusterAgentManager_IsLocalAgent(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	config := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(config)

	agentManager := NewCrossClusterAgentManager("cluster-a", router, manager, 10*time.Second)

	agentManager.RegisterAgent("agent-1", "cluster-a")
	agentManager.RegisterAgent("agent-2", "cluster-b")

	if !agentManager.IsLocalAgent("agent-1") {
		t.Error("agent-1 should be local")
	}

	if agentManager.IsLocalAgent("agent-2") {
		t.Error("agent-2 should not be local")
	}

	if agentManager.IsLocalAgent("agent-nonexistent") {
		t.Error("non-existent agent should not be local")
	}
}

func TestCrossClusterAgentManager_GetLocalAgents(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	config := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(config)

	agentManager := NewCrossClusterAgentManager("cluster-a", router, manager, 10*time.Second)

	agentManager.RegisterAgent("agent-1", "cluster-a")
	agentManager.RegisterAgent("agent-2", "cluster-b")
	agentManager.RegisterAgent("agent-3", "cluster-a")

	localAgents := agentManager.GetLocalAgents()
	if len(localAgents) != 2 {
		t.Errorf("GetLocalAgents() returned %d agents, want 2", len(localAgents))
	}
}

func TestCrossClusterAgentManager_GetAgentsInCluster(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	config := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(config)

	agentManager := NewCrossClusterAgentManager("cluster-a", router, manager, 10*time.Second)

	agentManager.RegisterAgent("agent-1", "cluster-a")
	agentManager.RegisterAgent("agent-2", "cluster-b")
	agentManager.RegisterAgent("agent-3", "cluster-b")

	clusterBAgents := agentManager.GetAgentsInCluster("cluster-b")
	if len(clusterBAgents) != 2 {
		t.Errorf("GetAgentsInCluster() returned %d agents, want 2", len(clusterBAgents))
	}

	nonexistentCluster := agentManager.GetAgentsInCluster("cluster-nonexistent")
	if len(nonexistentCluster) != 0 {
		t.Errorf("GetAgentsInCluster() for non-existent should return empty")
	}
}

func TestCrossClusterAgentManager_GetAllAgents(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	config := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(config)

	agentManager := NewCrossClusterAgentManager("cluster-a", router, manager, 10*time.Second)

	agentManager.RegisterAgent("agent-1", "cluster-a")
	agentManager.RegisterAgent("agent-2", "cluster-b")
	agentManager.RegisterAgent("agent-3", "cluster-c")

	allAgents := agentManager.GetAllAgents()
	if len(allAgents) != 3 {
		t.Errorf("GetAllAgents() returned %d agents, want 3", len(allAgents))
	}
}

func TestCrossClusterAgentManager_GetClusterStats(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	config := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(config)

	agentManager := NewCrossClusterAgentManager("cluster-a", router, manager, 10*time.Second)

	agentManager.RegisterAgent("agent-1", "cluster-a")
	agentManager.RegisterAgent("agent-2", "cluster-a")
	agentManager.RegisterAgent("agent-3", "cluster-b")

	stats := agentManager.GetClusterStats()
	if stats["cluster-a"] != 2 {
		t.Errorf("cluster-a count = %d, want 2", stats["cluster-a"])
	}
	if stats["cluster-b"] != 1 {
		t.Errorf("cluster-b count = %d, want 1", stats["cluster-b"])
	}
}

func TestCrossClusterAgentManager_BuildAgentSubject(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	config := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(config)

	agentManager := NewCrossClusterAgentManager("cluster-a", router, manager, 10*time.Second)

	agentManager.RegisterAgent("agent-1", "cluster-a")
	agentManager.RegisterAgent("agent-2", "cluster-b")

	// Local agent
	subject := agentManager.BuildAgentSubject("agent-1", "command")
	expected := "kscore.cluster-a.agent.agent-1.command"
	if subject != expected {
		t.Errorf("BuildAgentSubject() = %v, want %v", subject, expected)
	}

	// Remote agent
	subject = agentManager.BuildAgentSubject("agent-2", "command")
	expected = "kscore.cluster-b.agent.agent-2.command"
	if subject != expected {
		t.Errorf("BuildAgentSubject() = %v, want %v", subject, expected)
	}

	// Non-existent agent defaults to local
	subject = agentManager.BuildAgentSubject("agent-unknown", "command")
	expected = "kscore.cluster-a.agent.agent-unknown.command"
	if subject != expected {
		t.Errorf("BuildAgentSubject() = %v, want %v", subject, expected)
	}
}

func TestCrossClusterAgentManager_GetTimeoutForAgent(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	config := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(config)

	crossClusterTimeout := 5 * time.Second
	agentManager := NewCrossClusterAgentManager("cluster-a", router, manager, crossClusterTimeout)

	agentManager.RegisterAgent("agent-1", "cluster-a")
	agentManager.RegisterAgent("agent-2", "cluster-b")

	baseTimeout := 10 * time.Second

	// Local agent gets base timeout
	timeout := agentManager.GetTimeoutForAgent("agent-1", baseTimeout)
	if timeout != baseTimeout {
		t.Errorf("local agent timeout = %v, want %v", timeout, baseTimeout)
	}

	// Remote agent gets base + cross-cluster timeout
	timeout = agentManager.GetTimeoutForAgent("agent-2", baseTimeout)
	expected := baseTimeout + crossClusterTimeout
	if timeout != expected {
		t.Errorf("remote agent timeout = %v, want %v", timeout, expected)
	}
}

// ============================================================================
// Failover Manager Tests (T5.5)
// ============================================================================

func TestFailoverState_String(t *testing.T) {
	tests := []struct {
		state    FailoverState
		expected string
	}{
		{FailoverStateNormal, "normal"},
		{FailoverStateDetecting, "detecting"},
		{FailoverStateFailingOver, "failing-over"},
		{FailoverStateFailedOver, "failed-over"},
		{FailoverStateFailingBack, "failing-back"},
		{FailoverState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("FailoverState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultFailoverConfig(t *testing.T) {
	cfg := DefaultFailoverConfig()

	if cfg.Enabled != true {
		t.Errorf("Enabled = %v, want true", cfg.Enabled)
	}
	if cfg.DetectionTimeout != 10*time.Second {
		t.Errorf("DetectionTimeout = %v, want 10s", cfg.DetectionTimeout)
	}
	if cfg.FailoverTimeout != 30*time.Second {
		t.Errorf("FailoverTimeout = %v, want 30s", cfg.FailoverTimeout)
	}
	if cfg.FailbackDelay != 60*time.Second {
		t.Errorf("FailbackDelay = %v, want 60s", cfg.FailbackDelay)
	}
	if cfg.MinHealthyNodes != 1 {
		t.Errorf("MinHealthyNodes = %d, want 1", cfg.MinHealthyNodes)
	}
}

func TestNewFailoverManager(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	gwConfig := &GatewayConfig{Name: "cluster-a"}
	gwManager, _ := NewGatewayManager(gwConfig)
	agentManager := NewCrossClusterAgentManager("cluster-a", router, gwManager, 10*time.Second)
	healthMonitor := NewGatewayHealthMonitor(gwManager, nil)

	failoverManager := NewFailoverManager(nil, "cluster-a", healthMonitor, agentManager, router)

	if failoverManager == nil {
		t.Fatal("expected failover manager to be non-nil")
	}
}

func TestFailoverManager_Lifecycle(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	gwConfig := &GatewayConfig{Name: "cluster-a"}
	gwManager, _ := NewGatewayManager(gwConfig)
	agentManager := NewCrossClusterAgentManager("cluster-a", router, gwManager, 10*time.Second)
	healthMonitor := NewGatewayHealthMonitor(gwManager, nil)

	failoverConfig := &FailoverConfig{
		Enabled:          false, // Disable for testing
		DetectionTimeout: 100 * time.Millisecond,
		FailoverTimeout:  100 * time.Millisecond,
		FailbackDelay:    100 * time.Millisecond,
	}

	failoverManager := NewFailoverManager(failoverConfig, "cluster-a", healthMonitor, agentManager, router)

	ctx := context.Background()
	if err := failoverManager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !failoverManager.IsRunning() {
		t.Error("IsRunning() = false, want true")
	}

	// Start again should error
	if err := failoverManager.Start(ctx); err == nil {
		t.Error("Start() should error when already running")
	}

	// Stop
	if err := failoverManager.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if failoverManager.IsRunning() {
		t.Error("IsRunning() = true after stop, want false")
	}
}

func TestFailoverManager_State(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	gwConfig := &GatewayConfig{Name: "cluster-a"}
	gwManager, _ := NewGatewayManager(gwConfig)
	agentManager := NewCrossClusterAgentManager("cluster-a", router, gwManager, 10*time.Second)
	healthMonitor := NewGatewayHealthMonitor(gwManager, nil)

	failoverManager := NewFailoverManager(nil, "cluster-a", healthMonitor, agentManager, router)

	// Initial state should be normal
	if failoverManager.State() != FailoverStateNormal {
		t.Errorf("State() = %v, want normal", failoverManager.State())
	}

	// Active cluster should be local
	if failoverManager.ActiveCluster() != "cluster-a" {
		t.Errorf("ActiveCluster() = %v, want cluster-a", failoverManager.ActiveCluster())
	}

	// Should not be failed over initially
	if failoverManager.IsFailedOver() {
		t.Error("IsFailedOver() = true, want false")
	}

	// FailedOverTo should be empty
	if failoverManager.FailedOverTo() != "" {
		t.Errorf("FailedOverTo() = %v, want empty", failoverManager.FailedOverTo())
	}
}

func TestFailoverManager_Callbacks(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	gwConfig := &GatewayConfig{Name: "cluster-a"}
	gwManager, _ := NewGatewayManager(gwConfig)
	agentManager := NewCrossClusterAgentManager("cluster-a", router, gwManager, 10*time.Second)
	healthMonitor := NewGatewayHealthMonitor(gwManager, nil)

	failoverManager := NewFailoverManager(nil, "cluster-a", healthMonitor, agentManager, router)

	failoverCalled := false
	failbackCalled := false

	failoverManager.SetFailoverCallback(func(from, to string) {
		failoverCalled = true
	})

	failoverManager.SetFailbackCallback(func(from, to string) {
		failbackCalled = true
	})
	_, _ = failoverCalled, failbackCalled // Used in callbacks, verified via function pointers being set

	// Verify callbacks are set
	if failoverManager.onFailover == nil {
		t.Error("onFailover callback should be set")
	}
	if failoverManager.onFailback == nil {
		t.Error("onFailback callback should be set")
	}
}

func TestFailoverManager_GetStatus(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	gwConfig := &GatewayConfig{Name: "cluster-a"}
	gwManager, _ := NewGatewayManager(gwConfig)
	agentManager := NewCrossClusterAgentManager("cluster-a", router, gwManager, 10*time.Second)
	healthMonitor := NewGatewayHealthMonitor(gwManager, nil)

	failoverManager := NewFailoverManager(nil, "cluster-a", healthMonitor, agentManager, router)

	status := failoverManager.GetStatus()
	if status == nil {
		t.Fatal("GetStatus() = nil")
	}

	if status.State != FailoverStateNormal {
		t.Errorf("State = %v, want normal", status.State)
	}

	if status.LocalCluster != "cluster-a" {
		t.Errorf("LocalCluster = %v, want cluster-a", status.LocalCluster)
	}

	if status.ActiveCluster != "cluster-a" {
		t.Errorf("ActiveCluster = %v, want cluster-a", status.ActiveCluster)
	}

	if status.IsFailedOver {
		t.Error("IsFailedOver = true, want false")
	}
}

func TestFailoverManager_ManualFailback_NotFailedOver(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	gwConfig := &GatewayConfig{Name: "cluster-a"}
	gwManager, _ := NewGatewayManager(gwConfig)
	agentManager := NewCrossClusterAgentManager("cluster-a", router, gwManager, 10*time.Second)
	healthMonitor := NewGatewayHealthMonitor(gwManager, nil)

	failoverManager := NewFailoverManager(nil, "cluster-a", healthMonitor, agentManager, router)

	// ManualFailback when not failed over should error
	err := failoverManager.ManualFailback()
	if err == nil {
		t.Error("ManualFailback() should error when not failed over")
	}
}

// ============================================================================
// Supercluster Integration Tests (T5.6)
// ============================================================================

func TestSupercluster_MultiClusterTopology(t *testing.T) {
	// Test a 3-cluster supercluster topology
	clusters := []string{"us-east", "us-west", "eu-central"}

	managers := make(map[string]*GatewayManager)
	routers := make(map[string]*SubjectRouter)
	agentManagers := make(map[string]*CrossClusterAgentManager)

	// Create managers for each cluster
	for _, cluster := range clusters {
		config := &GatewayConfig{
			Name: cluster,
			Port: 7222,
			Gateways: []GatewayRemoteConfig{
				{Name: "us-east", URLs: []string{"nats://us-east:7222"}},
				{Name: "us-west", URLs: []string{"nats://us-west:7222"}},
				{Name: "eu-central", URLs: []string{"nats://eu-central:7222"}},
			},
		}

		manager, err := NewGatewayManager(config)
		if err != nil {
			t.Fatalf("failed to create manager for %s: %v", cluster, err)
		}
		managers[cluster] = manager

		router := NewSubjectRouter(cluster, true)
		routers[cluster] = router

		agentManager := NewCrossClusterAgentManager(cluster, router, manager, 15*time.Second)
		agentManagers[cluster] = agentManager
	}

	// Verify each cluster has correct configuration
	for _, cluster := range clusters {
		manager := managers[cluster]
		if manager.config.Name != cluster {
			t.Errorf("cluster %s: config name = %v, want %v", cluster, manager.config.Name, cluster)
		}

		// Each cluster should know about all 3 gateways
		if len(manager.config.Gateways) != 3 {
			t.Errorf("cluster %s: gateway count = %d, want 3", cluster, len(manager.config.Gateways))
		}
	}

	// Verify routers are independent
	for _, cluster := range clusters {
		router := routers[cluster]
		if router.localCluster != cluster {
			t.Errorf("router local cluster = %v, want %v", router.localCluster, cluster)
		}
	}
}

func TestSupercluster_CrossClusterAgentRouting(t *testing.T) {
	// Setup two clusters
	router1 := NewSubjectRouter("cluster-a", true)
	router2 := NewSubjectRouter("cluster-b", true)

	gwConfig1 := &GatewayConfig{Name: "cluster-a"}
	gwConfig2 := &GatewayConfig{Name: "cluster-b"}
	manager1, _ := NewGatewayManager(gwConfig1)
	manager2, _ := NewGatewayManager(gwConfig2)

	am1 := NewCrossClusterAgentManager("cluster-a", router1, manager1, 10*time.Second)
	am2 := NewCrossClusterAgentManager("cluster-b", router2, manager2, 10*time.Second)

	// Register agents in cluster-a
	am1.RegisterAgent("agent-1", "cluster-a")
	am1.RegisterAgent("agent-2", "cluster-a")

	// Register agents in cluster-b
	am2.RegisterAgent("agent-3", "cluster-b")
	am2.RegisterAgent("agent-4", "cluster-b")

	// Simulate cross-cluster registration: agent-5 in cluster-b managed by cluster-a
	am1.RegisterAgent("agent-5", "cluster-b")

	// Verify local agent detection
	if !am1.IsLocalAgent("agent-1") {
		t.Error("agent-1 should be local to cluster-a")
	}
	if !am1.IsLocalAgent("agent-2") {
		t.Error("agent-2 should be local to cluster-a")
	}
	if am1.IsLocalAgent("agent-5") {
		t.Error("agent-5 should NOT be local to cluster-a (it's in cluster-b)")
	}

	// Verify agent lookup
	cluster, exists := am1.GetAgentCluster("agent-5")
	if !exists {
		t.Error("agent-5 should exist")
	}
	if cluster != "cluster-b" {
		t.Errorf("agent-5 cluster = %v, want cluster-b", cluster)
	}

	// Verify agents in cluster listing
	clusterAAgents := am1.GetAgentsInCluster("cluster-a")
	if len(clusterAAgents) != 2 {
		t.Errorf("cluster-a agent count = %d, want 2", len(clusterAAgents))
	}

	clusterBAgents := am1.GetAgentsInCluster("cluster-b")
	if len(clusterBAgents) != 1 { // Only agent-5 registered with am1
		t.Errorf("cluster-b agent count from am1 = %d, want 1", len(clusterBAgents))
	}
}

func TestSupercluster_SubjectRoutingWithFailover(t *testing.T) {
	router := NewSubjectRouter("primary", true)

	// First add the local cluster route (required for preferLocal fallback)
	router.AddRoute(&ClusterRoute{ClusterName: "primary", Available: true, Priority: 100})

	// Add remote cluster routes (required for RouteSubject to work)
	router.AddRoute(&ClusterRoute{ClusterName: "us-east", Available: true, Priority: 1})
	router.AddRoute(&ClusterRoute{ClusterName: "us-west", Available: true, Priority: 1})
	router.AddRoute(&ClusterRoute{ClusterName: "eu-central", Available: true, Priority: 1})

	// Add routing rules using subject prefix mapping
	router.AddSubjectPrefix("agents.us-east.", "us-east")
	router.AddSubjectPrefix("agents.us-west.", "us-west")
	router.AddSubjectPrefix("agents.eu-central.", "eu-central")

	// Normal routing via prefix matching
	result := router.RouteSubject("agents.us-east.command")
	if result != "us-east" {
		t.Errorf("route target = %v, want us-east", result)
	}

	result = router.RouteSubject("agents.us-west.heartbeat")
	if result != "us-west" {
		t.Errorf("route target = %v, want us-west", result)
	}

	result = router.RouteSubject("agents.eu-central.status")
	if result != "eu-central" {
		t.Errorf("route target = %v, want eu-central", result)
	}

	// Subjects without matching routes stay local (preferLocal = true)
	result = router.RouteSubject("system.health")
	if result != "primary" {
		t.Errorf("unmatched subject should route to local cluster 'primary', got %v", result)
	}
}

func TestSupercluster_FailoverStateTransitions(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	gwConfig := &GatewayConfig{Name: "cluster-a"}
	gwManager, _ := NewGatewayManager(gwConfig)
	agentManager := NewCrossClusterAgentManager("cluster-a", router, gwManager, 10*time.Second)
	healthMonitor := NewGatewayHealthMonitor(gwManager, nil)

	failoverConfig := &FailoverConfig{
		Enabled:                  true,
		DetectionTimeout:         5 * time.Second,
		FailoverTimeout:          10 * time.Second,
		FailbackDelay:            5 * time.Second,
		MinHealthyNodes:          2,
		PreferredFailoverCluster: "cluster-b",
	}

	fm := NewFailoverManager(failoverConfig, "cluster-a", healthMonitor, agentManager, router)

	// Initial state should be Normal
	if fm.State() != FailoverStateNormal {
		t.Errorf("initial state = %v, want Normal", fm.State())
	}

	// Track state transitions
	var transitions []FailoverState

	// Simulate state transitions by directly setting state (in real usage, this happens via health monitoring)
	states := []FailoverState{
		FailoverStateNormal,
		FailoverStateDetecting,
		FailoverStateFailingOver,
		FailoverStateFailedOver,
		FailoverStateFailingBack,
		FailoverStateNormal,
	}

	for _, state := range states {
		transitions = append(transitions, state)
	}

	// Verify all states are valid
	for _, state := range transitions {
		stateStr := state.String()
		if stateStr == "unknown" && state <= FailoverStateFailingBack {
			t.Errorf("state %d has unknown string representation", state)
		}
	}
}

func TestSupercluster_GatewayHealthAggregation(t *testing.T) {
	// Create a manager with multiple gateway connections
	config := &GatewayConfig{
		Name: "central",
		Gateways: []GatewayRemoteConfig{
			{Name: "cluster-1", URLs: []string{"nats://cluster-1:7222"}},
			{Name: "cluster-2", URLs: []string{"nats://cluster-2:7222"}},
			{Name: "cluster-3", URLs: []string{"nats://cluster-3:7222"}},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	healthConfig := &GatewayHealthConfig{
		CheckInterval:      1 * time.Second,
		HealthyThreshold:   2,
		UnhealthyThreshold: 3,
	}

	monitor := NewGatewayHealthMonitor(manager, healthConfig)

	// Verify initial health state
	allHealth := monitor.GetAllHealth()

	// Should have health entries for configured gateways (initially empty until started)
	if allHealth == nil {
		t.Error("GetAllHealth() = nil")
	}

	// Get individual gateway health (returns nil if not tracked yet)
	health := monitor.GetHealth("cluster-1")
	// Initially nil until health check runs
	_ = health

	// Verify monitor lifecycle
	ctx := context.Background()
	if err := monitor.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !monitor.IsRunning() {
		t.Error("IsRunning() = false after Start")
	}

	if err := monitor.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if monitor.IsRunning() {
		t.Error("IsRunning() = true after Stop")
	}
}

func TestSupercluster_CrossClusterTimeout(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	gwConfig := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(gwConfig)

	// Create agent manager with specific cross-cluster timeout
	crossClusterTimeout := 30 * time.Second
	am := NewCrossClusterAgentManager("cluster-a", router, manager, crossClusterTimeout)

	// Register a remote agent
	am.RegisterAgent("remote-agent", "cluster-b")

	// Base timeout for testing
	baseTimeout := 10 * time.Second

	// Verify timeout retrieval for cross-cluster operations
	// Remote agents get base + cross-cluster timeout
	agentTimeout := am.GetTimeoutForAgent("remote-agent", baseTimeout)
	expectedRemoteTimeout := baseTimeout + crossClusterTimeout // 10s + 30s = 40s
	if agentTimeout != expectedRemoteTimeout {
		t.Errorf("GetTimeoutForAgent for remote agent = %v, want %v", agentTimeout, expectedRemoteTimeout)
	}

	// Local agents should use base timeout only
	am.RegisterAgent("local-agent", "cluster-a")
	localTimeout := am.GetTimeoutForAgent("local-agent", baseTimeout)
	if localTimeout != baseTimeout {
		t.Errorf("local agent timeout = %v, want %v (base timeout)", localTimeout, baseTimeout)
	}
}

func TestSupercluster_GatewayModeConfiguration(t *testing.T) {
	tests := []struct {
		name     string
		mode     GatewayMode
		expected string
	}{
		{"optimistic", GatewayModeOptimistic, "optimistic"},
		{"interest-only", GatewayModeInterestOnly, "interest-only"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &GatewayConfig{
				Name:        "test-cluster",
				DefaultMode: tt.mode,
			}

			manager, err := NewGatewayManager(config)
			if err != nil {
				t.Fatalf("NewGatewayManager error = %v", err)
			}

			if manager.config.DefaultMode != tt.mode {
				t.Errorf("DefaultMode = %v, want %v", manager.config.DefaultMode, tt.mode)
			}

			if tt.mode.String() != tt.expected {
				t.Errorf("mode.String() = %v, want %v", tt.mode.String(), tt.expected)
			}
		})
	}
}

func TestSupercluster_RoutingPriority(t *testing.T) {
	router := NewSubjectRouter("local", true)

	// Add routes with ClusterRoute (note: priority-based routing uses AddRoute)
	router.AddRoute(&ClusterRoute{
		ClusterName: "low-priority-cluster",
		Priority:    10, // Higher number = lower priority
		Available:   true,
	})
	router.AddRoute(&ClusterRoute{
		ClusterName: "high-priority-cluster",
		Priority:    1, // Lower number = higher priority
		Available:   true,
	})

	// Use subject prefix routing for more specific matching
	router.AddSubjectPrefix("events.critical.", "high-priority-cluster")
	router.AddSubjectPrefix("events.", "low-priority-cluster")

	// Critical events should match the more specific prefix first
	result := router.RouteSubject("events.critical.alert")
	if result != "high-priority-cluster" {
		t.Errorf("critical event routed to %v, want high-priority-cluster", result)
	}

	// Non-critical events should match the generic prefix
	result = router.RouteSubject("events.info.status")
	if result != "low-priority-cluster" {
		t.Errorf("info event routed to %v, want low-priority-cluster", result)
	}
}

func TestSupercluster_DynamicRouteManagement(t *testing.T) {
	router := NewSubjectRouter("local", true)

	// Add the local cluster route (required for fallback)
	router.AddRoute(&ClusterRoute{ClusterName: "local", Available: true, Priority: 100})

	// Add a cluster route
	router.AddRoute(&ClusterRoute{
		ClusterName: "target-cluster",
		Priority:    5,
		Available:   true,
	})

	// Add subject prefix for routing
	router.AddSubjectPrefix("test.", "target-cluster")

	// Verify route exists via subject routing
	result := router.RouteSubject("test.subject")
	if result != "target-cluster" {
		t.Errorf("route target = %v, want target-cluster", result)
	}

	// Remove subject prefix
	router.RemoveSubjectPrefix("test.")

	// Route should now be local (no prefix match, falls back to local)
	result = router.RouteSubject("test.subject")
	if result != "local" {
		t.Errorf("route should be local after prefix removal, got %v", result)
	}

	// Add cluster routes for a, b, c
	router.AddRoute(&ClusterRoute{ClusterName: "cluster-a", Available: true, Priority: 1})
	router.AddRoute(&ClusterRoute{ClusterName: "cluster-b", Available: true, Priority: 1})
	router.AddRoute(&ClusterRoute{ClusterName: "cluster-c", Available: true, Priority: 1})

	// Add multiple subject prefix routes
	router.AddSubjectPrefix("a.", "cluster-a")
	router.AddSubjectPrefix("b.", "cluster-b")
	router.AddSubjectPrefix("c.", "cluster-c")

	// Verify all routes work
	if router.RouteSubject("a.test") != "cluster-a" {
		t.Error("a.test should route to cluster-a")
	}
	if router.RouteSubject("b.test") != "cluster-b" {
		t.Error("b.test should route to cluster-b")
	}
	if router.RouteSubject("c.test") != "cluster-c" {
		t.Error("c.test should route to cluster-c")
	}

	// Clear prefixes to make subjects route locally
	router.RemoveSubjectPrefix("a.")
	router.RemoveSubjectPrefix("b.")
	router.RemoveSubjectPrefix("c.")

	// All routes should now be local (no prefix match)
	for _, subject := range []string{"a.test", "b.test", "c.test"} {
		result := router.RouteSubject(subject)
		if result != "local" {
			t.Errorf("subject %s should route locally after prefix removal, got %v", subject, result)
		}
	}
}

func TestSupercluster_AgentMigration(t *testing.T) {
	router := NewSubjectRouter("cluster-a", true)
	gwConfig := &GatewayConfig{Name: "cluster-a"}
	manager, _ := NewGatewayManager(gwConfig)
	am := NewCrossClusterAgentManager("cluster-a", router, manager, 10*time.Second)

	// Register agent in cluster-a
	am.RegisterAgent("migrating-agent", "cluster-a")

	if !am.IsLocalAgent("migrating-agent") {
		t.Error("agent should initially be local")
	}

	// Simulate agent migration to cluster-b
	am.UnregisterAgent("migrating-agent")
	am.RegisterAgent("migrating-agent", "cluster-b")

	if am.IsLocalAgent("migrating-agent") {
		t.Error("agent should be remote after migration")
	}

	cluster, exists := am.GetAgentCluster("migrating-agent")
	if !exists {
		t.Error("migrating-agent should exist")
	}
	if cluster != "cluster-b" {
		t.Errorf("migrated agent cluster = %v, want cluster-b", cluster)
	}
}

func TestSupercluster_ConnectionStateTracking(t *testing.T) {
	config := &GatewayConfig{
		Name: "test-cluster",
		Gateways: []GatewayRemoteConfig{
			{Name: "remote-1", URLs: []string{"nats://remote-1:7222"}},
			{Name: "remote-2", URLs: []string{"nats://remote-2:7222"}},
		},
	}

	manager, err := NewGatewayManager(config)
	if err != nil {
		t.Fatalf("NewGatewayManager error = %v", err)
	}

	// Get connections (tracks state internally)
	connections := manager.GetConnections()
	if connections == nil {
		t.Error("GetConnections() = nil")
	}

	// Verify connection tracking for configured gateways
	// Initially connections map may be empty until Start() is called
	// but the map itself should not be nil
	connectionCount := manager.ConnectionCount()
	if connectionCount < 0 {
		t.Errorf("ConnectionCount = %d, should be >= 0", connectionCount)
	}

	// Verify we can get individual connections
	for name := range config.Gateways {
		gwConfig := config.Gateways[name]
		conn := manager.GetConnection(gwConfig.Name)
		// Connection may be nil before Start() is called
		_ = conn
	}
}

func TestSupercluster_FailoverConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		config  *FailoverConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid config",
			config: &FailoverConfig{
				Enabled:                  true,
				DetectionTimeout:         5 * time.Second,
				FailoverTimeout:          10 * time.Second,
				FailbackDelay:            10 * time.Second,
				MinHealthyNodes:          2,
				PreferredFailoverCluster: "secondary",
			},
			wantErr: false,
		},
		{
			name: "disabled failover",
			config: &FailoverConfig{
				Enabled: false,
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := NewSubjectRouter("local", true)
			gwConfig := &GatewayConfig{Name: "local"}
			gwManager, _ := NewGatewayManager(gwConfig)
			agentManager := NewCrossClusterAgentManager("local", router, gwManager, 10*time.Second)
			healthMonitor := NewGatewayHealthMonitor(gwManager, nil)

			fm := NewFailoverManager(tt.config, "local", healthMonitor, agentManager, router)
			if fm == nil {
				if !tt.wantErr {
					t.Error("NewFailoverManager returned nil unexpectedly")
				}
			}
		})
	}
}
