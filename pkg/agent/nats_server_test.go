package agent

import (
	"context"
	"crypto/tls"
	"testing"
	"time"
)

func TestEmbeddedNATSMode_String(t *testing.T) {
	tests := []struct {
		mode     EmbeddedNATSMode
		expected string
	}{
		{EmbeddedNATSModeDisabled, "disabled"},
		{EmbeddedNATSModeStandalone, "standalone"},
		{EmbeddedNATSModeLeaf, "leaf"},
		{EmbeddedNATSMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("EmbeddedNATSMode.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseEmbeddedNATSMode(t *testing.T) {
	tests := []struct {
		input    string
		expected EmbeddedNATSMode
		wantErr  bool
	}{
		{"disabled", EmbeddedNATSModeDisabled, false},
		{"", EmbeddedNATSModeDisabled, false},
		{"standalone", EmbeddedNATSModeStandalone, false},
		{"leaf", EmbeddedNATSModeLeaf, false},
		{"invalid", EmbeddedNATSModeDisabled, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseEmbeddedNATSMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseEmbeddedNATSMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseEmbeddedNATSMode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestEmbeddedNATSState_String(t *testing.T) {
	tests := []struct {
		state    EmbeddedNATSState
		expected string
	}{
		{EmbeddedNATSStateStopped, "stopped"},
		{EmbeddedNATSStateStarting, "starting"},
		{EmbeddedNATSStateRunning, "running"},
		{EmbeddedNATSStateStopping, "stopping"},
		{EmbeddedNATSState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("EmbeddedNATSState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultEmbeddedNATSConfig(t *testing.T) {
	cfg := DefaultEmbeddedNATSConfig()

	if cfg.Mode != EmbeddedNATSModeDisabled {
		t.Errorf("expected mode disabled, got %v", cfg.Mode)
	}
	if cfg.Host != "0.0.0.0" {
		t.Errorf("expected host 0.0.0.0, got %v", cfg.Host)
	}
	if cfg.Port != 4222 {
		t.Errorf("expected port 4222, got %v", cfg.Port)
	}
	if cfg.MaxConnections != 100 {
		t.Errorf("expected max connections 100, got %v", cfg.MaxConnections)
	}
	if cfg.MaxPayload != 1024*1024 {
		t.Errorf("expected max payload 1MB, got %v", cfg.MaxPayload)
	}
	if cfg.MaxPending != 64*1024*1024 {
		t.Errorf("expected max pending 64MB, got %v", cfg.MaxPending)
	}
	if cfg.WriteDeadline != 10*time.Second {
		t.Errorf("expected write deadline 10s, got %v", cfg.WriteDeadline)
	}
}

func TestEmbeddedNATSConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *EmbeddedNATSConfig
		wantErr bool
	}{
		{
			name:    "disabled mode always valid",
			config:  &EmbeddedNATSConfig{Mode: EmbeddedNATSModeDisabled},
			wantErr: false,
		},
		{
			name: "valid standalone config",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeStandalone,
				Host: "0.0.0.0",
				Port: 4222,
			},
			wantErr: false,
		},
		{
			name: "invalid port zero",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeStandalone,
				Port: 0,
			},
			wantErr: true,
		},
		{
			name: "invalid port negative",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeStandalone,
				Port: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid port too large",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeStandalone,
				Port: 65536,
			},
			wantErr: true,
		},
		{
			name: "invalid max connections",
			config: &EmbeddedNATSConfig{
				Mode:           EmbeddedNATSModeStandalone,
				Port:           4222,
				MaxConnections: -1,
			},
			wantErr: true,
		},
		{
			name: "invalid max payload",
			config: &EmbeddedNATSConfig{
				Mode:       EmbeddedNATSModeStandalone,
				Port:       4222,
				MaxPayload: -1,
			},
			wantErr: true,
		},
		{
			name: "leaf mode without remotes",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeLeaf,
				Port: 4222,
			},
			wantErr: true,
		},
		{
			name: "leaf mode with remotes",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeLeaf,
				Port: 4222,
				LeafRemotes: []LeafRemoteConfig{
					{URLs: []string{"nats://upstream:4222"}},
				},
			},
			wantErr: false,
		},
		{
			name: "TLS without cert file",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeStandalone,
				Port: 4222,
				TLSConfig: &EmbeddedNATSTLSConfig{
					KeyFile: "/path/to/key",
				},
			},
			wantErr: true,
		},
		{
			name: "TLS without key file",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeStandalone,
				Port: 4222,
				TLSConfig: &EmbeddedNATSTLSConfig{
					CertFile: "/path/to/cert",
				},
			},
			wantErr: true,
		},
		{
			name: "TLS with pre-configured tls.Config",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeStandalone,
				Port: 4222,
				TLSConfig: &EmbeddedNATSTLSConfig{
					TLSConfig: &tls.Config{},
				},
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

func TestEmbeddedNATSConfig_GetAdvertiseAddress(t *testing.T) {
	tests := []struct {
		name     string
		config   *EmbeddedNATSConfig
		expected string
	}{
		{
			name: "with advertise host and port",
			config: &EmbeddedNATSConfig{
				Host:          "0.0.0.0",
				Port:          4222,
				AdvertiseHost: "public.example.com",
				AdvertisePort: 5222,
			},
			expected: "public.example.com:5222",
		},
		{
			name: "with advertise host only",
			config: &EmbeddedNATSConfig{
				Host:          "0.0.0.0",
				Port:          4222,
				AdvertiseHost: "public.example.com",
			},
			expected: "public.example.com:4222",
		},
		{
			name: "with specific host",
			config: &EmbeddedNATSConfig{
				Host: "192.168.1.10",
				Port: 4222,
			},
			expected: "192.168.1.10:4222",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.GetAdvertiseAddress()
			if got != tt.expected {
				t.Errorf("GetAdvertiseAddress() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNewEmbeddedNATSServer(t *testing.T) {
	tests := []struct {
		name    string
		config  *EmbeddedNATSConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name:    "valid config",
			config:  DefaultEmbeddedNATSConfig(),
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &EmbeddedNATSConfig{
				Mode: EmbeddedNATSModeStandalone,
				Port: -1,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server, err := NewEmbeddedNATSServer(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewEmbeddedNATSServer() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && server == nil {
				t.Error("expected server to be non-nil")
			}
		})
	}
}

func TestEmbeddedNATSServer_DisabledMode(t *testing.T) {
	server, err := NewEmbeddedNATSServer(&EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeDisabled,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()

	// Start should succeed without doing anything
	if err := server.Start(ctx); err != nil {
		t.Errorf("Start() error = %v, expected nil for disabled mode", err)
	}

	// State should be stopped (server never started)
	if server.State() != EmbeddedNATSStateStopped {
		t.Errorf("State() = %v, want stopped", server.State())
	}

	// IsRunning should be false
	if server.IsRunning() {
		t.Error("IsRunning() = true, want false for disabled mode")
	}

	// GetClientURL should be empty
	if url := server.GetClientURL(); url != "" {
		t.Errorf("GetClientURL() = %v, want empty for disabled mode", url)
	}

	// Stop should succeed
	if err := server.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}
}

func TestEmbeddedNATSServer_StandaloneMode(t *testing.T) {
	// Use a random high port to avoid conflicts
	server, err := NewEmbeddedNATSServer(&EmbeddedNATSConfig{
		Mode:           EmbeddedNATSModeStandalone,
		Host:           "127.0.0.1",
		Port:           14222,
		MaxConnections: 10,
		ServerName:     "test-agent-nats",
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Test state change callback
	var stateChanges []EmbeddedNATSState
	server.SetStateChangeCallback(func(state EmbeddedNATSState) {
		stateChanges = append(stateChanges, state)
	})

	ctx := context.Background()

	// Start the server
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Verify state is running
	if server.State() != EmbeddedNATSStateRunning {
		t.Errorf("State() = %v, want running", server.State())
	}

	// Verify IsRunning
	if !server.IsRunning() {
		t.Error("IsRunning() = false, want true")
	}

	// Verify GetClientURL
	url := server.GetClientURL()
	if url == "" {
		t.Error("GetClientURL() = empty, expected URL")
	}
	if url != "nats://127.0.0.1:14222" {
		t.Errorf("GetClientURL() = %v, want nats://127.0.0.1:14222", url)
	}

	// Get stats
	stats := server.GetStats()
	if stats == nil {
		t.Error("GetStats() = nil")
	}
	if stats.State != EmbeddedNATSStateRunning {
		t.Errorf("stats.State = %v, want running", stats.State)
	}

	// Verify config
	cfg := server.Config()
	if cfg.ServerName != "test-agent-nats" {
		t.Errorf("Config().ServerName = %v, want test-agent-nats", cfg.ServerName)
	}

	// Verify underlying server is accessible
	ns := server.Server()
	if ns == nil {
		t.Error("Server() = nil, expected NATS server")
	}

	// Stop the server
	if err := server.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Verify state is stopped
	if server.State() != EmbeddedNATSStateStopped {
		t.Errorf("State() = %v, want stopped", server.State())
	}

	// Verify state changes were recorded
	if len(stateChanges) < 3 {
		t.Errorf("expected at least 3 state changes, got %d", len(stateChanges))
	}
	if stateChanges[0] != EmbeddedNATSStateStarting {
		t.Errorf("first state change = %v, want starting", stateChanges[0])
	}
	if stateChanges[1] != EmbeddedNATSStateRunning {
		t.Errorf("second state change = %v, want running", stateChanges[1])
	}
}

func TestEmbeddedNATSServer_StopWithoutStart(t *testing.T) {
	server, err := NewEmbeddedNATSServer(&EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Port: 14223,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Stop without starting should not error
	if err := server.Stop(); err != nil {
		t.Errorf("Stop() error = %v, expected nil", err)
	}
}

func TestEmbeddedNATSServer_WithTokenAuth(t *testing.T) {
	server, err := NewEmbeddedNATSServer(&EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Host: "127.0.0.1",
		Port: 14224,
		AuthConfig: &EmbeddedNATSAuthConfig{
			Token: "test-secret-token",
		},
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Stop()

	if !server.IsRunning() {
		t.Error("server should be running")
	}
}

func TestEmbeddedNATSServer_WithUserAuth(t *testing.T) {
	server, err := NewEmbeddedNATSServer(&EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Host: "127.0.0.1",
		Port: 14225,
		AuthConfig: &EmbeddedNATSAuthConfig{
			Users: []EmbeddedNATSUser{
				{
					Username: "user1",
					Password: "pass1",
					Permissions: &EmbeddedNATSPermissions{
						Publish: PermissionScope{
							Allow: []string{"foo.>"},
						},
						Subscribe: PermissionScope{
							Allow: []string{"bar.>"},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Stop()

	if !server.IsRunning() {
		t.Error("server should be running")
	}
}

func TestEmbeddedNATSServer_WithNKeyAuth(t *testing.T) {
	// Use a valid NKey public key format
	server, err := NewEmbeddedNATSServer(&EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Host: "127.0.0.1",
		Port: 14226,
		AuthConfig: &EmbeddedNATSAuthConfig{
			NKeyUsers: []EmbeddedNATSNKeyUser{
				{
					NKey: "UAXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXXX",
					Permissions: &EmbeddedNATSPermissions{
						Publish: PermissionScope{
							Allow: []string{">"},
						},
						Subscribe: PermissionScope{
							Allow: []string{">"},
						},
					},
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Stop()

	if !server.IsRunning() {
		t.Error("server should be running")
	}
}

func TestEmbeddedNATSServer_WithDebugTrace(t *testing.T) {
	server, err := NewEmbeddedNATSServer(&EmbeddedNATSConfig{
		Mode:  EmbeddedNATSModeStandalone,
		Host:  "127.0.0.1",
		Port:  14227,
		Debug: true,
		Trace: true,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Stop()

	if !server.IsRunning() {
		t.Error("server should be running")
	}
}

func TestEmbeddedNATSServer_ClientConnCallback(t *testing.T) {
	server, err := NewEmbeddedNATSServer(&EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Host: "127.0.0.1",
		Port: 14228,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	var clientEvents []struct {
		clientID  uint64
		connected bool
	}

	server.SetClientConnCallback(func(clientID uint64, connected bool) {
		clientEvents = append(clientEvents, struct {
			clientID  uint64
			connected bool
		}{clientID, connected})
	})

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Stop()

	// Simulate recording client connections (these are internal tracking events)
	server.recordClientConnect(1)
	server.recordClientConnect(2)
	server.recordClientDisconnect(1)

	if len(clientEvents) != 3 {
		t.Errorf("expected 3 client events, got %d", len(clientEvents))
	}

	// Verify callback events have correct data
	if clientEvents[0].clientID != 1 || !clientEvents[0].connected {
		t.Errorf("first event should be client 1 connected")
	}
	if clientEvents[1].clientID != 2 || !clientEvents[1].connected {
		t.Errorf("second event should be client 2 connected")
	}
	if clientEvents[2].clientID != 1 || clientEvents[2].connected {
		t.Errorf("third event should be client 1 disconnected")
	}

	// Check server stats are available
	stats := server.GetStats()
	if stats.State != EmbeddedNATSStateRunning {
		t.Errorf("expected running state, got %v", stats.State)
	}
}

func TestEmbeddedNATSStats(t *testing.T) {
	server, err := NewEmbeddedNATSServer(&EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Host: "127.0.0.1",
		Port: 14229,
	})
	if err != nil {
		t.Fatalf("failed to create server: %v", err)
	}

	// Stats before start
	stats := server.GetStats()
	if stats.State != EmbeddedNATSStateStopped {
		t.Errorf("stats.State = %v, want stopped", stats.State)
	}

	ctx := context.Background()
	if err := server.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer server.Stop()

	// Let server run briefly
	time.Sleep(100 * time.Millisecond)

	// Stats after start
	stats = server.GetStats()
	if stats.State != EmbeddedNATSStateRunning {
		t.Errorf("stats.State = %v, want running", stats.State)
	}
	if stats.Uptime < 100*time.Millisecond {
		t.Errorf("stats.Uptime = %v, want >= 100ms", stats.Uptime)
	}
}

func TestGetOutboundIP(t *testing.T) {
	// This function tries to get the outbound IP
	ip := getOutboundIP()
	// It may return empty if there's no network, which is fine
	// Just ensure it doesn't panic
	t.Logf("getOutboundIP() = %v", ip)
}

func TestLeafRemoteConfig(t *testing.T) {
	cfg := LeafRemoteConfig{
		URLs:              []string{"nats://upstream1:4222", "nats://upstream2:4222"},
		Credentials:       "/path/to/creds",
		ReconnectInterval: 30 * time.Second,
		Hub:               true,
	}

	if len(cfg.URLs) != 2 {
		t.Errorf("expected 2 URLs, got %d", len(cfg.URLs))
	}
	if !cfg.Hub {
		t.Error("expected Hub to be true")
	}
}

func TestEmbeddedNATSPermissions(t *testing.T) {
	perms := &EmbeddedNATSPermissions{
		Publish: PermissionScope{
			Allow: []string{"foo.>", "bar.*"},
			Deny:  []string{"foo.secret"},
		},
		Subscribe: PermissionScope{
			Allow: []string{"baz.>"},
		},
	}

	if len(perms.Publish.Allow) != 2 {
		t.Errorf("expected 2 publish allow, got %d", len(perms.Publish.Allow))
	}
	if len(perms.Publish.Deny) != 1 {
		t.Errorf("expected 1 publish deny, got %d", len(perms.Publish.Deny))
	}
	if len(perms.Subscribe.Allow) != 1 {
		t.Errorf("expected 1 subscribe allow, got %d", len(perms.Subscribe.Allow))
	}
}
