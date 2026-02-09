package nats

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestLeafNodeRole_String(t *testing.T) {
	tests := []struct {
		role     LeafNodeRole
		expected string
	}{
		{LeafNodeRoleLeaf, "leaf"},
		{LeafNodeRoleHub, "hub"},
		{LeafNodeRoleBridge, "bridge"},
		{LeafNodeRole(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.role.String(); got != tt.expected {
				t.Errorf("LeafNodeRole.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseLeafNodeRole(t *testing.T) {
	tests := []struct {
		input    string
		expected LeafNodeRole
		wantErr  bool
	}{
		{"leaf", LeafNodeRoleLeaf, false},
		{"", LeafNodeRoleLeaf, false}, // Empty defaults to leaf
		{"hub", LeafNodeRoleHub, false},
		{"bridge", LeafNodeRoleBridge, false},
		{"invalid", LeafNodeRoleLeaf, true},
		{"HUB", LeafNodeRoleLeaf, true}, // Case sensitive
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseLeafNodeRole(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseLeafNodeRole() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.expected {
				t.Errorf("ParseLeafNodeRole() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLeafConnectionState_String(t *testing.T) {
	tests := []struct {
		state    LeafConnectionState
		expected string
	}{
		{LeafConnectionStateDisconnected, "disconnected"},
		{LeafConnectionStateConnecting, "connecting"},
		{LeafConnectionStateConnected, "connected"},
		{LeafConnectionStateReconnecting, "reconnecting"},
		{LeafConnectionStateFailed, "failed"},
		{LeafConnectionState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("LeafConnectionState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultLeafNodeConfig(t *testing.T) {
	cfg := DefaultLeafNodeConfig()

	if cfg.Role != LeafNodeRoleLeaf {
		t.Errorf("Role = %v, want leaf", cfg.Role)
	}
	if cfg.Port != 7422 {
		t.Errorf("Port = %d, want 7422", cfg.Port)
	}
	if cfg.ReconnectInterval != 2*time.Second {
		t.Errorf("ReconnectInterval = %v, want 2s", cfg.ReconnectInterval)
	}
	if cfg.MaxReconnects != -1 {
		t.Errorf("MaxReconnects = %d, want -1", cfg.MaxReconnects)
	}
}

func TestLeafNodeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *LeafNodeConfig
		wantErr bool
	}{
		{
			name: "valid leaf config",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleLeaf,
				Port: 7422,
				Remotes: []LeafRemoteConfig{
					{URLs: []string{"nats://localhost:7422"}},
				},
			},
			wantErr: false,
		},
		{
			name: "valid hub config",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleHub,
				Port: 7422,
			},
			wantErr: false,
		},
		{
			name: "valid bridge config",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleBridge,
				Port: 7422,
				Remotes: []LeafRemoteConfig{
					{URLs: []string{"nats://localhost:7423"}},
				},
			},
			wantErr: false,
		},
		{
			name: "leaf without remotes",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleLeaf,
				Port: 7422,
			},
			wantErr: true,
		},
		{
			name: "bridge without remotes",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleBridge,
				Port: 7422,
			},
			wantErr: true,
		},
		{
			name: "remote without URLs",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleLeaf,
				Remotes: []LeafRemoteConfig{
					{URLs: []string{}},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid port for hub",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleHub,
				Port: 0,
			},
			wantErr: true,
		},
		{
			name: "port too high",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleHub,
				Port: 70000,
			},
			wantErr: true,
		},
		{
			name: "negative reconnect interval",
			config: &LeafNodeConfig{
				Role:              LeafNodeRoleHub,
				Port:              7422,
				ReconnectInterval: -1 * time.Second,
			},
			wantErr: true,
		},
		{
			name: "invalid URL",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleLeaf,
				Remotes: []LeafRemoteConfig{
					{URLs: []string{"not-a-valid-url://\x00"}},
				},
			},
			wantErr: true,
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

func TestLeafConnection_State(t *testing.T) {
	conn := &LeafConnection{
		Remote: &LeafRemoteConfig{URLs: []string{"nats://localhost:7422"}},
	}

	// Initial state should be disconnected
	if conn.State() != LeafConnectionStateDisconnected {
		t.Errorf("initial State() = %v, want disconnected", conn.State())
	}

	if conn.IsConnected() {
		t.Error("IsConnected() = true, want false")
	}

	// Simulate connection
	conn.state.Store(int32(LeafConnectionStateConnected))
	if !conn.IsConnected() {
		t.Error("IsConnected() = false after setting connected, want true")
	}

	// Test state changes
	conn.state.Store(int32(LeafConnectionStateReconnecting))
	if conn.State() != LeafConnectionStateReconnecting {
		t.Errorf("State() = %v, want reconnecting", conn.State())
	}
	if conn.IsConnected() {
		t.Error("IsConnected() = true while reconnecting, want false")
	}
}

func TestLeafConnection_LastError(t *testing.T) {
	conn := &LeafConnection{
		Remote: &LeafRemoteConfig{URLs: []string{"nats://localhost:7422"}},
	}

	// Initial error should be nil
	if conn.LastError() != nil {
		t.Error("initial LastError() should be nil")
	}

	// Store an error
	testErr := errors.New("connection failed")
	conn.lastError.Store(testErr)

	if !errors.Is(conn.LastError(), testErr) {
		t.Errorf("LastError() = %v, want %v", conn.LastError(), testErr)
	}
}

func TestLeafConnection_Latency(t *testing.T) {
	conn := &LeafConnection{
		Remote: &LeafRemoteConfig{URLs: []string{"nats://localhost:7422"}},
	}

	// Initial latency should be 0
	if conn.Latency() != 0 {
		t.Errorf("initial Latency() = %v, want 0", conn.Latency())
	}

	// Store latency
	conn.latency.Store(int64(5 * time.Millisecond))
	if conn.Latency() != 5*time.Millisecond {
		t.Errorf("Latency() = %v, want 5ms", conn.Latency())
	}
}

func TestLeafConnection_Reconnects(t *testing.T) {
	conn := &LeafConnection{
		Remote: &LeafRemoteConfig{URLs: []string{"nats://localhost:7422"}},
	}

	// Initial reconnects should be 0
	if conn.Reconnects() != 0 {
		t.Errorf("initial Reconnects() = %d, want 0", conn.Reconnects())
	}

	// Increment reconnects
	conn.reconnects.Add(1)
	if conn.Reconnects() != 1 {
		t.Errorf("Reconnects() = %d, want 1", conn.Reconnects())
	}

	conn.reconnects.Add(4)
	if conn.Reconnects() != 5 {
		t.Errorf("Reconnects() = %d, want 5", conn.Reconnects())
	}
}

func TestNewLeafNodeManager(t *testing.T) {
	tests := []struct {
		name    string
		config  *LeafNodeConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: true, // Default config is leaf without remotes
		},
		{
			name: "valid hub config",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleHub,
				Port: 7422,
			},
			wantErr: false,
		},
		{
			name: "valid leaf config",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleLeaf,
				Remotes: []LeafRemoteConfig{
					{URLs: []string{"nats://localhost:7422"}},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid config",
			config: &LeafNodeConfig{
				Role: LeafNodeRoleLeaf,
				// No remotes
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewLeafNodeManager(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewLeafNodeManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && manager == nil {
				t.Error("NewLeafNodeManager() returned nil manager without error")
			}
		})
	}
}

func TestLeafNodeManager_Lifecycle(t *testing.T) {
	config := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafNodeManager(config)
	if err != nil {
		t.Fatalf("NewLeafNodeManager() error = %v", err)
	}

	// Should not be running initially
	if manager.IsRunning() {
		t.Error("IsRunning() = true before Start, want false")
	}

	// Start the manager
	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	// Should be running after start
	if !manager.IsRunning() {
		t.Error("IsRunning() = false after Start, want true")
	}

	// State should be connected
	if manager.State() != LeafConnectionStateConnected {
		t.Errorf("State() = %v, want connected", manager.State())
	}

	// Client should be available
	if manager.GetClient() == nil {
		t.Error("GetClient() = nil, want non-nil")
	}

	// Config should be accessible
	if manager.Config() != config {
		t.Error("Config() did not return expected config")
	}

	// Start again should error
	if err := manager.Start(ctx); err == nil {
		t.Error("Start() should error when already running")
	}

	// Stop the manager
	if err := manager.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Should not be running after stop
	if manager.IsRunning() {
		t.Error("IsRunning() = true after Stop, want false")
	}

	// State should be disconnected
	if manager.State() != LeafConnectionStateDisconnected {
		t.Errorf("State() = %v after Stop, want disconnected", manager.State())
	}
}

func TestLeafNodeManager_StopWithoutStart(t *testing.T) {
	config := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafNodeManager(config)
	if err != nil {
		t.Fatalf("NewLeafNodeManager() error = %v", err)
	}

	// Stop without starting should not error
	if err := manager.Stop(); err != nil {
		t.Errorf("Stop() without Start should not error, got %v", err)
	}
}

func TestLeafNodeManager_GetStats(t *testing.T) {
	config := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafNodeManager(config)
	if err != nil {
		t.Fatalf("NewLeafNodeManager() error = %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	stats := manager.GetStats()
	if stats == nil {
		t.Fatal("GetStats() = nil")
	}

	if stats.Role != LeafNodeRoleHub {
		t.Errorf("stats.Role = %v, want hub", stats.Role)
	}

	if stats.State != LeafConnectionStateConnected {
		t.Errorf("stats.State = %v, want connected", stats.State)
	}
}

func TestLeafNodeManager_GetConnections(t *testing.T) {
	config := &LeafNodeConfig{
		Role: LeafNodeRoleLeaf,
		Remotes: []LeafRemoteConfig{
			{URLs: []string{"nats://localhost:7422"}},
			{URLs: []string{"nats://localhost:7423"}},
		},
	}

	manager, err := NewLeafNodeManager(config)
	if err != nil {
		t.Fatalf("NewLeafNodeManager() error = %v", err)
	}

	conns := manager.GetConnections()
	if len(conns) != 2 {
		t.Errorf("GetConnections() returned %d connections, want 2", len(conns))
	}

	// Verify connection remotes are properly set
	for i, conn := range conns {
		if conn.Remote == nil {
			t.Errorf("connection %d has nil Remote", i)
		}
	}
}

func TestLeafNodeManager_GetConnectedRemotes(t *testing.T) {
	config := &LeafNodeConfig{
		Role: LeafNodeRoleLeaf,
		Remotes: []LeafRemoteConfig{
			{URLs: []string{"nats://localhost:7422"}},
			{URLs: []string{"nats://localhost:7423"}},
		},
	}

	manager, err := NewLeafNodeManager(config)
	if err != nil {
		t.Fatalf("NewLeafNodeManager() error = %v", err)
	}

	// Initially no connected remotes
	connected := manager.GetConnectedRemotes()
	if len(connected) != 0 {
		t.Errorf("GetConnectedRemotes() returned %d, want 0", len(connected))
	}

	// Simulate first connection connected
	conns := manager.GetConnections()
	conns[0].state.Store(int32(LeafConnectionStateConnected))

	connected = manager.GetConnectedRemotes()
	if len(connected) != 1 {
		t.Errorf("GetConnectedRemotes() returned %d after connect, want 1", len(connected))
	}
}

func TestLeafNodeManager_Callbacks(t *testing.T) {
	config := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafNodeManager(config)
	if err != nil {
		t.Fatalf("NewLeafNodeManager() error = %v", err)
	}

	// Track callback invocations
	var stateChanges []LeafConnectionState
	var connectCalls int
	var disconnectCalls int

	manager.SetStateChangeCallback(func(state LeafConnectionState) {
		stateChanges = append(stateChanges, state)
	})

	manager.SetRemoteConnectCallback(func(remote *LeafRemoteConfig) {
		connectCalls++
	})

	manager.SetRemoteDisconnectCallback(func(remote *LeafRemoteConfig, err error) {
		disconnectCalls++
	})

	manager.SetMessageCallback(func(subject string, data []byte, isFromRemote bool) {
		// No-op for test
	})

	// Start and stop to trigger state changes
	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	manager.Stop()

	// Should have received state changes
	if len(stateChanges) < 2 {
		t.Errorf("expected at least 2 state changes, got %d", len(stateChanges))
	}

	// Verify state transitions
	if len(stateChanges) >= 1 && stateChanges[0] != LeafConnectionStateConnecting {
		t.Errorf("first state change = %v, want connecting", stateChanges[0])
	}
}

func TestLeafChainConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *LeafChainConfig
		wantErr bool
	}{
		{
			name: "valid simple chain",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{
					{Name: "edge", Role: LeafNodeRoleLeaf, UpstreamURLs: []string{"nats://regional:7422"}},
					{Name: "regional", Role: LeafNodeRoleHub, ListenPort: 7422},
				},
			},
			wantErr: false,
		},
		{
			name: "valid chain with bridge",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{
					{Name: "edge", Role: LeafNodeRoleLeaf, UpstreamURLs: []string{"nats://regional:7422"}},
					{Name: "regional", Role: LeafNodeRoleBridge, UpstreamURLs: []string{"nats://central:7422"}, ListenPort: 7422},
					{Name: "central", Role: LeafNodeRoleHub, ListenPort: 7422},
				},
			},
			wantErr: false,
		},
		{
			name: "empty chain",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{},
			},
			wantErr: true,
		},
		{
			name: "hop without name",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{
					{Role: LeafNodeRoleLeaf, UpstreamURLs: []string{"nats://hub:7422"}},
					{Name: "hub", Role: LeafNodeRoleHub, ListenPort: 7422},
				},
			},
			wantErr: true,
		},
		{
			name: "first hop must be leaf or bridge",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{
					{Name: "hub1", Role: LeafNodeRoleHub, ListenPort: 7422},
					{Name: "hub2", Role: LeafNodeRoleHub, ListenPort: 7423},
				},
			},
			wantErr: true,
		},
		{
			name: "last hop must be hub or bridge",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{
					{Name: "leaf1", Role: LeafNodeRoleLeaf, UpstreamURLs: []string{"nats://leaf2:7422"}},
					{Name: "leaf2", Role: LeafNodeRoleLeaf, UpstreamURLs: []string{"nats://somewhere:7422"}},
				},
			},
			wantErr: true,
		},
		{
			name: "middle hop must be bridge",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{
					{Name: "edge", Role: LeafNodeRoleLeaf, UpstreamURLs: []string{"nats://hub1:7422"}},
					{Name: "hub1", Role: LeafNodeRoleHub, ListenPort: 7422},
					{Name: "hub2", Role: LeafNodeRoleHub, ListenPort: 7423},
				},
			},
			wantErr: true,
		},
		{
			name: "leaf without upstream URLs",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{
					{Name: "edge", Role: LeafNodeRoleLeaf, UpstreamURLs: []string{}},
					{Name: "hub", Role: LeafNodeRoleHub, ListenPort: 7422},
				},
			},
			wantErr: true,
		},
		{
			name: "hub without listen port",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{
					{Name: "edge", Role: LeafNodeRoleLeaf, UpstreamURLs: []string{"nats://hub:7422"}},
					{Name: "hub", Role: LeafNodeRoleHub, ListenPort: 0},
				},
			},
			wantErr: true,
		},
		{
			name: "hub with invalid port",
			config: &LeafChainConfig{
				Hops: []LeafChainHop{
					{Name: "edge", Role: LeafNodeRoleLeaf, UpstreamURLs: []string{"nats://hub:7422"}},
					{Name: "hub", Role: LeafNodeRoleHub, ListenPort: 70000},
				},
			},
			wantErr: true,
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

func TestDefaultLeafChainConfig(t *testing.T) {
	cfg := DefaultLeafChainConfig()

	if cfg.TotalTimeout != 30*time.Second {
		t.Errorf("TotalTimeout = %v, want 30s", cfg.TotalTimeout)
	}
	if !cfg.EnableJetStream {
		t.Error("EnableJetStream = false, want true")
	}
	if cfg.BufferSize != 64*1024*1024 {
		t.Errorf("BufferSize = %d, want 67108864 (64MB)", cfg.BufferSize)
	}
	if cfg.DeduplicationWindow != 2*time.Minute {
		t.Errorf("DeduplicationWindow = %v, want 2m", cfg.DeduplicationWindow)
	}
}

func TestLeafChainConfig_CalculateHopTimeout(t *testing.T) {
	tests := []struct {
		name     string
		config   *LeafChainConfig
		expected time.Duration
	}{
		{
			name: "explicit hop timeout",
			config: &LeafChainConfig{
				HopTimeout: 5 * time.Second,
				Hops:       make([]LeafChainHop, 3),
			},
			expected: 5 * time.Second,
		},
		{
			name: "calculated from total",
			config: &LeafChainConfig{
				TotalTimeout: 30 * time.Second,
				Hops:         make([]LeafChainHop, 3),
			},
			expected: 10 * time.Second,
		},
		{
			name: "no hops defaults to 10s",
			config: &LeafChainConfig{
				TotalTimeout: 30 * time.Second,
				Hops:         []LeafChainHop{},
			},
			expected: 10 * time.Second,
		},
		{
			name: "no timeout set defaults to 10s",
			config: &LeafChainConfig{
				Hops: make([]LeafChainHop, 3),
			},
			expected: 10 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.config.CalculateHopTimeout()
			if got != tt.expected {
				t.Errorf("CalculateHopTimeout() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestLeafChainConfig_BuildHopConfigs(t *testing.T) {
	config := &LeafChainConfig{
		TotalTimeout: 30 * time.Second,
		Hops: []LeafChainHop{
			{
				Name:         "edge",
				Role:         LeafNodeRoleLeaf,
				UpstreamURLs: []string{"nats://regional:7422"},
			},
			{
				Name:         "regional",
				Role:         LeafNodeRoleBridge,
				UpstreamURLs: []string{"nats://central:7422"},
				ListenPort:   7422,
			},
			{
				Name:       "central",
				Role:       LeafNodeRoleHub,
				ListenPort: 7422,
			},
		},
	}

	configs, err := config.BuildHopConfigs()
	if err != nil {
		t.Fatalf("BuildHopConfigs() error = %v", err)
	}

	if len(configs) != 3 {
		t.Fatalf("BuildHopConfigs() returned %d configs, want 3", len(configs))
	}

	// Verify first hop (leaf)
	if configs[0].Name != "edge" {
		t.Errorf("config[0].Name = %s, want edge", configs[0].Name)
	}
	if configs[0].Role != LeafNodeRoleLeaf {
		t.Errorf("config[0].Role = %v, want leaf", configs[0].Role)
	}
	if len(configs[0].Remotes) != 1 {
		t.Errorf("config[0].Remotes length = %d, want 1", len(configs[0].Remotes))
	}
	if configs[0].Remotes[0].URLs[0] != "nats://regional:7422" {
		t.Errorf("config[0].Remotes[0].URLs[0] = %s, want nats://regional:7422", configs[0].Remotes[0].URLs[0])
	}

	// Verify second hop (bridge)
	if configs[1].Name != "regional" {
		t.Errorf("config[1].Name = %s, want regional", configs[1].Name)
	}
	if configs[1].Role != LeafNodeRoleBridge {
		t.Errorf("config[1].Role = %v, want bridge", configs[1].Role)
	}
	if configs[1].Port != 7422 {
		t.Errorf("config[1].Port = %d, want 7422", configs[1].Port)
	}
	if len(configs[1].Remotes) != 1 {
		t.Errorf("config[1].Remotes length = %d, want 1", len(configs[1].Remotes))
	}

	// Verify third hop (hub)
	if configs[2].Name != "central" {
		t.Errorf("config[2].Name = %s, want central", configs[2].Name)
	}
	if configs[2].Role != LeafNodeRoleHub {
		t.Errorf("config[2].Role = %v, want hub", configs[2].Role)
	}
	if configs[2].Port != 7422 {
		t.Errorf("config[2].Port = %d, want 7422", configs[2].Port)
	}
	if len(configs[2].Remotes) != 0 {
		t.Errorf("config[2].Remotes length = %d, want 0", len(configs[2].Remotes))
	}
}

func TestLeafChainConfig_BuildHopConfigs_InvalidConfig(t *testing.T) {
	config := &LeafChainConfig{
		Hops: []LeafChainHop{}, // Empty chain is invalid
	}

	_, err := config.BuildHopConfigs()
	if err == nil {
		t.Error("BuildHopConfigs() should error for invalid config")
	}
}

func TestLeafNodeManager_HubMode(t *testing.T) {
	config := &LeafNodeConfig{
		Role:   LeafNodeRoleHub,
		Listen: "127.0.0.1",
		Port:   7422,
	}

	manager, err := NewLeafNodeManager(config)
	if err != nil {
		t.Fatalf("NewLeafNodeManager() error = %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	// Verify hub is accepting connections
	stats := manager.GetStats()
	if stats.Role != LeafNodeRoleHub {
		t.Errorf("stats.Role = %v, want hub", stats.Role)
	}

	// Client should be connected
	client := manager.GetClient()
	if client == nil {
		t.Fatal("GetClient() = nil")
	}
	if !client.IsConnected() {
		t.Error("client.IsConnected() = false, want true")
	}
}

func TestLeafRemoteStats(t *testing.T) {
	stats := LeafRemoteStats{
		URL:        "nats://localhost:7422",
		State:      LeafConnectionStateConnected,
		Latency:    5 * time.Millisecond,
		Reconnects: 3,
		LastError:  "test error",
	}

	if stats.URL != "nats://localhost:7422" {
		t.Errorf("URL = %s, want nats://localhost:7422", stats.URL)
	}
	if stats.State != LeafConnectionStateConnected {
		t.Errorf("State = %v, want connected", stats.State)
	}
	if stats.Latency != 5*time.Millisecond {
		t.Errorf("Latency = %v, want 5ms", stats.Latency)
	}
	if stats.Reconnects != 3 {
		t.Errorf("Reconnects = %d, want 3", stats.Reconnects)
	}
	if stats.LastError != "test error" {
		t.Errorf("LastError = %s, want 'test error'", stats.LastError)
	}
}

func TestLeafNodeStats(t *testing.T) {
	stats := LeafNodeStats{
		State:            LeafConnectionStateConnected,
		Role:             LeafNodeRoleHub,
		ConnectedRemotes: 2,
		TotalRemotes:     3,
		MessagesIn:       100,
		MessagesOut:      50,
		BytesIn:          1000,
		BytesOut:         500,
		Reconnects:       5,
	}

	if stats.State != LeafConnectionStateConnected {
		t.Errorf("State = %v, want connected", stats.State)
	}
	if stats.Role != LeafNodeRoleHub {
		t.Errorf("Role = %v, want hub", stats.Role)
	}
	if stats.ConnectedRemotes != 2 {
		t.Errorf("ConnectedRemotes = %d, want 2", stats.ConnectedRemotes)
	}
	if stats.TotalRemotes != 3 {
		t.Errorf("TotalRemotes = %d, want 3", stats.TotalRemotes)
	}
}

func TestSubjectMapping(t *testing.T) {
	mapping := SubjectMapping{
		Source:      "local.>",
		Destination: "remote.>",
		Weight:      50,
		Cluster:     "east",
	}

	if mapping.Source != "local.>" {
		t.Errorf("Source = %s, want local.>", mapping.Source)
	}
	if mapping.Destination != "remote.>" {
		t.Errorf("Destination = %s, want remote.>", mapping.Destination)
	}
	if mapping.Weight != 50 {
		t.Errorf("Weight = %d, want 50", mapping.Weight)
	}
	if mapping.Cluster != "east" {
		t.Errorf("Cluster = %s, want east", mapping.Cluster)
	}
}

func TestSubjectPermission(t *testing.T) {
	perm := SubjectPermission{
		Allow: []string{"kscore.>", "events.>"},
		Deny:  []string{"admin.>"},
	}

	if len(perm.Allow) != 2 {
		t.Errorf("Allow length = %d, want 2", len(perm.Allow))
	}
	if len(perm.Deny) != 1 {
		t.Errorf("Deny length = %d, want 1", len(perm.Deny))
	}
}

func TestLeafUser(t *testing.T) {
	user := LeafUser{
		User:     "testuser",
		Password: "testpass",
		Account:  "testaccount",
		Permissions: &LeafPermissions{
			Publish: SubjectPermission{
				Allow: []string{"kscore.>"},
			},
			Subscribe: SubjectPermission{
				Allow: []string{">"},
			},
		},
	}

	if user.User != "testuser" {
		t.Errorf("User = %s, want testuser", user.User)
	}
	if user.Password != "testpass" {
		t.Errorf("Password = %s, want testpass", user.Password)
	}
	if user.Account != "testaccount" {
		t.Errorf("Account = %s, want testaccount", user.Account)
	}
	if user.Permissions == nil {
		t.Fatal("Permissions = nil")
	}
	if len(user.Permissions.Publish.Allow) != 1 {
		t.Errorf("Publish.Allow length = %d, want 1", len(user.Permissions.Publish.Allow))
	}
}

func TestLeafAuthConfig(t *testing.T) {
	auth := LeafAuthConfig{
		Token:   "secret-token",
		Account: "default",
		Users: []LeafUser{
			{User: "user1", Password: "pass1"},
		},
	}

	if auth.Token != "secret-token" {
		t.Errorf("Token = %s, want secret-token", auth.Token)
	}
	if auth.Account != "default" {
		t.Errorf("Account = %s, want default", auth.Account)
	}
	if len(auth.Users) != 1 {
		t.Errorf("Users length = %d, want 1", len(auth.Users))
	}
}

func TestLeafTLSConfig(t *testing.T) {
	tlsConfig := LeafTLSConfig{
		CertFile:           "/path/to/cert.pem",
		KeyFile:            "/path/to/key.pem",
		CAFile:             "/path/to/ca.pem",
		InsecureSkipVerify: false,
	}

	if tlsConfig.CertFile != "/path/to/cert.pem" {
		t.Errorf("CertFile = %s, want /path/to/cert.pem", tlsConfig.CertFile)
	}
	if tlsConfig.KeyFile != "/path/to/key.pem" {
		t.Errorf("KeyFile = %s, want /path/to/key.pem", tlsConfig.KeyFile)
	}
	if tlsConfig.CAFile != "/path/to/ca.pem" {
		t.Errorf("CAFile = %s, want /path/to/ca.pem", tlsConfig.CAFile)
	}
	if tlsConfig.InsecureSkipVerify {
		t.Error("InsecureSkipVerify = true, want false")
	}
}

func TestLeafRemoteConfig(t *testing.T) {
	remote := LeafRemoteConfig{
		URLs:              []string{"nats://hub1:7422", "nats://hub2:7422"},
		Credentials:       "/path/to/creds.creds",
		Account:           "myaccount",
		Hub:               true,
		DenyImports:       false,
		DenyExports:       false,
		LocalAccountName:  "local",
		Compression:       true,
		ReconnectInterval: 5 * time.Second,
		SigningKey:        "my-signing-key",
	}

	if len(remote.URLs) != 2 {
		t.Errorf("URLs length = %d, want 2", len(remote.URLs))
	}
	if remote.Credentials != "/path/to/creds.creds" {
		t.Errorf("Credentials = %s, want /path/to/creds.creds", remote.Credentials)
	}
	if !remote.Hub {
		t.Error("Hub = false, want true")
	}
	if !remote.Compression {
		t.Error("Compression = false, want true")
	}
	if remote.ReconnectInterval != 5*time.Second {
		t.Errorf("ReconnectInterval = %v, want 5s", remote.ReconnectInterval)
	}
}

func TestLeafNodeConfig_NoRandomize(t *testing.T) {
	config := &LeafNodeConfig{
		Role: LeafNodeRoleLeaf,
		Remotes: []LeafRemoteConfig{
			{URLs: []string{"nats://hub1:7422", "nats://hub2:7422"}},
		},
		NoRandomize: true,
	}

	if !config.NoRandomize {
		t.Error("NoRandomize = false, want true")
	}
}

func TestLeafChainHop(t *testing.T) {
	hop := LeafChainHop{
		Name:            "regional",
		Role:            LeafNodeRoleBridge,
		UpstreamURLs:    []string{"nats://central:7422"},
		ListenPort:      7422,
		Credentials:     "/path/to/creds",
		ExpectedLatency: 10 * time.Millisecond,
	}

	if hop.Name != "regional" {
		t.Errorf("Name = %s, want regional", hop.Name)
	}
	if hop.Role != LeafNodeRoleBridge {
		t.Errorf("Role = %v, want bridge", hop.Role)
	}
	if len(hop.UpstreamURLs) != 1 {
		t.Errorf("UpstreamURLs length = %d, want 1", len(hop.UpstreamURLs))
	}
	if hop.ListenPort != 7422 {
		t.Errorf("ListenPort = %d, want 7422", hop.ListenPort)
	}
	if hop.ExpectedLatency != 10*time.Millisecond {
		t.Errorf("ExpectedLatency = %v, want 10ms", hop.ExpectedLatency)
	}
}

func TestLeafNodeManager_StateChangeCallback(t *testing.T) {
	config := &LeafNodeConfig{
		Role: LeafNodeRoleHub,
		Port: 7422,
	}

	manager, err := NewLeafNodeManager(config)
	if err != nil {
		t.Fatalf("NewLeafNodeManager() error = %v", err)
	}

	var stateChanges []LeafConnectionState
	var stateChangeCount atomic.Int32

	manager.SetStateChangeCallback(func(state LeafConnectionState) {
		stateChanges = append(stateChanges, state)
		stateChangeCount.Add(1)
	})

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	manager.Stop()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return stateChangeCount.Load() >= 2, nil
	}); err != nil {
		t.Fatalf("expected state change callbacks: %v", err)
	}

	if stateChangeCount.Load() < 2 {
		t.Errorf("expected at least 2 state changes, got %d", stateChangeCount.Load())
	}

	// First should be connecting, then connected, then disconnected
	foundConnecting := false
	foundConnected := false
	foundDisconnected := false

	for _, state := range stateChanges {
		switch state {
		case LeafConnectionStateConnecting:
			foundConnecting = true
		case LeafConnectionStateConnected:
			foundConnected = true
		case LeafConnectionStateDisconnected:
			foundDisconnected = true
		default:
		}
	}

	if !foundConnecting {
		t.Error("expected 'connecting' state change")
	}
	if !foundConnected {
		t.Error("expected 'connected' state change")
	}
	if !foundDisconnected {
		t.Error("expected 'disconnected' state change")
	}
}
