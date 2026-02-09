package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestConnectionRole_String(t *testing.T) {
	tests := []struct {
		role     ConnectionRole
		expected string
	}{
		{ConnectionRoleUndetermined, "undetermined"},
		{ConnectionRoleClient, "client"},
		{ConnectionRoleHost, "host"},
		{ConnectionRoleLeaf, "leaf"},
		{ConnectionRole(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.role.String(); got != tt.expected {
				t.Errorf("ConnectionRole.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseConnectionRole(t *testing.T) {
	tests := []struct {
		input    string
		expected ConnectionRole
		wantErr  bool
	}{
		{"", ConnectionRoleUndetermined, false},
		{"undetermined", ConnectionRoleUndetermined, false},
		{"client", ConnectionRoleClient, false},
		{"host", ConnectionRoleHost, false},
		{"leaf", ConnectionRoleLeaf, false},
		{"invalid", ConnectionRoleUndetermined, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseConnectionRole(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseConnectionRole() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseConnectionRole() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestRoleSelectionMode_String(t *testing.T) {
	tests := []struct {
		mode     RoleSelectionMode
		expected string
	}{
		{RoleSelectionAuto, "auto"},
		{RoleSelectionManual, "manual"},
		{RoleSelectionPreferHost, "prefer-host"},
		{RoleSelectionPreferClient, "prefer-client"},
		{RoleSelectionMode(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.expected {
				t.Errorf("RoleSelectionMode.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestParseRoleSelectionMode(t *testing.T) {
	tests := []struct {
		input    string
		expected RoleSelectionMode
		wantErr  bool
	}{
		{"", RoleSelectionAuto, false},
		{"auto", RoleSelectionAuto, false},
		{"manual", RoleSelectionManual, false},
		{"prefer-host", RoleSelectionPreferHost, false},
		{"prefer-client", RoleSelectionPreferClient, false},
		{"invalid", RoleSelectionAuto, true},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got, err := ParseRoleSelectionMode(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseRoleSelectionMode() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if got != tt.expected {
				t.Errorf("ParseRoleSelectionMode() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestNetworkReachability_String(t *testing.T) {
	tests := []struct {
		reach    NetworkReachability
		expected string
	}{
		{NetworkReachabilityUnknown, "unknown"},
		{NetworkReachabilityDirect, "direct"},
		{NetworkReachabilityNAT, "nat"},
		{NetworkReachabilityRestricted, "restricted"},
		{NetworkReachability(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.reach.String(); got != tt.expected {
				t.Errorf("NetworkReachability.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestHybridModeState_String(t *testing.T) {
	tests := []struct {
		state    HybridModeState
		expected string
	}{
		{HybridModeStateIdle, "idle"},
		{HybridModeStateDetermining, "determining"},
		{HybridModeStateConnecting, "connecting"},
		{HybridModeStateHosting, "hosting"},
		{HybridModeStateActive, "active"},
		{HybridModeStateFailed, "failed"},
		{HybridModeState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("HybridModeState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultHybridModeConfig(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")

	if config.AgentID != "test-agent" {
		t.Errorf("AgentID = %v, want test-agent", config.AgentID)
	}
	if config.SelectionMode != RoleSelectionAuto {
		t.Errorf("SelectionMode = %v, want auto", config.SelectionMode)
	}
	if config.ManualRole != ConnectionRoleUndetermined {
		t.Errorf("ManualRole = %v, want undetermined", config.ManualRole)
	}
	if config.ReachabilityCheckTimeout != 5*time.Second {
		t.Errorf("ReachabilityCheckTimeout = %v, want 5s", config.ReachabilityCheckTimeout)
	}
	if !config.FallbackToClient {
		t.Error("FallbackToClient should be true by default")
	}
	if config.FallbackToHost {
		t.Error("FallbackToHost should be false by default")
	}
}

func TestHybridModeConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*HybridModeConfig)
		wantErr bool
	}{
		{
			name:    "valid default config",
			modify:  func(c *HybridModeConfig) {},
			wantErr: false,
		},
		{
			name: "missing agent ID",
			modify: func(c *HybridModeConfig) {
				c.AgentID = ""
			},
			wantErr: true,
		},
		{
			name: "manual mode without role",
			modify: func(c *HybridModeConfig) {
				c.SelectionMode = RoleSelectionManual
				c.ManualRole = ConnectionRoleUndetermined
			},
			wantErr: true,
		},
		{
			name: "manual mode with role",
			modify: func(c *HybridModeConfig) {
				c.SelectionMode = RoleSelectionManual
				c.ManualRole = ConnectionRoleClient
				c.ExternalNATSURLs = []string{"nats://localhost:4222"}
			},
			wantErr: false,
		},
		{
			name: "zero timeout gets default",
			modify: func(c *HybridModeConfig) {
				c.ReachabilityCheckTimeout = 0
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := DefaultHybridModeConfig("test-agent")
			tt.modify(config)
			err := config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewHybridModeManager(t *testing.T) {
	tests := []struct {
		name    string
		config  *HybridModeConfig
		wantErr bool
	}{
		{
			name:    "nil config",
			config:  nil,
			wantErr: true,
		},
		{
			name: "invalid config",
			config: &HybridModeConfig{
				AgentID:       "",
				SelectionMode: RoleSelectionManual,
			},
			wantErr: true,
		},
		{
			name:    "valid config",
			config:  DefaultHybridModeConfig("test-agent"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			manager, err := NewHybridModeManager(tt.config)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewHybridModeManager() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && manager == nil {
				t.Error("expected manager to be non-nil")
			}
		})
	}
}

func TestHybridModeManager_InitialState(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if manager.State() != HybridModeStateIdle {
		t.Errorf("State() = %v, want idle", manager.State())
	}
	if manager.Role() != ConnectionRoleUndetermined {
		t.Errorf("Role() = %v, want undetermined", manager.Role())
	}
	if manager.Reachability() != NetworkReachabilityUnknown {
		t.Errorf("Reachability() = %v, want unknown", manager.Reachability())
	}
	if manager.IsActive() {
		t.Error("IsActive() should be false initially")
	}
	if manager.GetConnection() != nil {
		t.Error("GetConnection() should be nil initially")
	}
	if manager.GetEmbeddedServer() != nil {
		t.Error("GetEmbeddedServer() should be nil initially")
	}
	if manager.GetAdvertiser() != nil {
		t.Error("GetAdvertiser() should be nil initially")
	}
}

func TestHybridModeManager_Callbacks(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	var stateChanges []HybridModeState
	var roleChanges []ConnectionRole
	var mu sync.Mutex

	manager.SetStateChangeCallback(func(state HybridModeState) {
		mu.Lock()
		stateChanges = append(stateChanges, state)
		mu.Unlock()
	})

	manager.SetRoleChangeCallback(func(role ConnectionRole) {
		mu.Lock()
		roleChanges = append(roleChanges, role)
		mu.Unlock()
	})

	var connReady bool
	manager.SetConnectionReadyCallback(func(role ConnectionRole, conn *nats.Conn) {
		mu.Lock()
		connReady = true
		mu.Unlock()
	})

	var connLost bool
	manager.SetConnectionLostCallback(func(role ConnectionRole, err error) {
		mu.Lock()
		connLost = true
		mu.Unlock()
	})

	// Verify callbacks are set (we can't easily trigger them without a real NATS connection)
	if manager.onStateChange == nil {
		t.Error("onStateChange callback should be set")
	}
	if manager.onRoleChange == nil {
		t.Error("onRoleChange callback should be set")
	}
	if manager.onConnectionReady == nil {
		t.Error("onConnectionReady callback should be set")
	}
	if manager.onConnectionLost == nil {
		t.Error("onConnectionLost callback should be set")
	}

	// Test that the callbacks work through internal methods
	manager.setState(HybridModeStateDetermining)
	manager.setRole(ConnectionRoleClient)

	mu.Lock()
	if len(stateChanges) != 1 || stateChanges[0] != HybridModeStateDetermining {
		t.Error("state change callback not called correctly")
	}
	if len(roleChanges) != 1 || roleChanges[0] != ConnectionRoleClient {
		t.Error("role change callback not called correctly")
	}
	mu.Unlock()

	// These need actual connections to test properly
	_ = connReady
	_ = connLost
}

func TestHybridModeManager_GetStats(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	stats := manager.GetStats()
	if stats == nil {
		t.Fatal("GetStats() returned nil")
	}

	if stats.State != HybridModeStateIdle {
		t.Errorf("State = %v, want idle", stats.State)
	}
	if stats.Role != ConnectionRoleUndetermined {
		t.Errorf("Role = %v, want undetermined", stats.Role)
	}
	if stats.ClientConnected {
		t.Error("ClientConnected should be false")
	}
	if stats.ServerRunning {
		t.Error("ServerRunning should be false")
	}
	if stats.AdvertiserActive {
		t.Error("AdvertiserActive should be false")
	}
}

func TestHybridModeManager_Config(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if manager.Config() != config {
		t.Error("Config() should return the same config")
	}
	if manager.Config().AgentID != "test-agent" {
		t.Error("Config should preserve agent ID")
	}
}

func TestHybridModeManager_ManualClientRole(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionManual
	config.ManualRole = ConnectionRoleClient
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Test determineRole without actually connecting
	role, err := manager.determineRole(context.Background())
	if err != nil {
		t.Errorf("determineRole() error = %v", err)
	}
	if role != ConnectionRoleClient {
		t.Errorf("determineRole() = %v, want client", role)
	}
}

func TestHybridModeManager_ManualHostRole(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionManual
	config.ManualRole = ConnectionRoleHost
	config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Port: helpers.FreePort(t),
	}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	role, err := manager.determineRole(context.Background())
	if err != nil {
		t.Errorf("determineRole() error = %v", err)
	}
	if role != ConnectionRoleHost {
		t.Errorf("determineRole() = %v, want host", role)
	}
}

func TestHybridModeManager_PreferHost(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionPreferHost
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}
	config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Port: helpers.FreePort(t),
	}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	role, err := manager.determineRole(context.Background())
	if err != nil {
		t.Errorf("determineRole() error = %v", err)
	}
	// Should prefer host when hosting is possible
	if role != ConnectionRoleHost {
		t.Errorf("determineRole() = %v, want host", role)
	}
}

func TestHybridModeManager_PreferClient(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionPreferClient
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}
	config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Port: helpers.FreePort(t),
	}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	role, err := manager.determineRole(context.Background())
	if err != nil {
		t.Errorf("determineRole() error = %v", err)
	}
	// Should prefer client when connecting is possible
	if role != ConnectionRoleClient {
		t.Errorf("determineRole() = %v, want client", role)
	}
}

func TestHybridModeManager_PreferClientFallbackToHost(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionPreferClient
	// No external URLs
	config.ExternalNATSURLs = nil
	config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Port: helpers.FreePort(t),
	}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	role, err := manager.determineRole(context.Background())
	if err != nil {
		t.Errorf("determineRole() error = %v", err)
	}
	// Should fall back to host since no external URLs
	if role != ConnectionRoleHost {
		t.Errorf("determineRole() = %v, want host", role)
	}
}

func TestHybridModeManager_CanHost(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// No embedded config
	if manager.canHost() {
		t.Error("canHost() should be false without embedded config")
	}

	// With embedded config but disabled
	manager.config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeDisabled,
	}
	if manager.canHost() {
		t.Error("canHost() should be false with disabled mode")
	}

	// With enabled embedded config
	manager.config.EmbeddedConfig.Mode = EmbeddedNATSModeStandalone
	if !manager.canHost() {
		t.Error("canHost() should be true with standalone mode")
	}
}

func TestHybridModeManager_CanConnect(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// No external URLs
	if manager.canConnect() {
		t.Error("canConnect() should be false without external URLs")
	}

	// With external URLs
	manager.config.ExternalNATSURLs = []string{"nats://localhost:4222"}
	if !manager.canConnect() {
		t.Error("canConnect() should be true with external URLs")
	}
}

func TestHybridModeManager_RoleFromEmbeddedMode(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// No embedded config
	if manager.roleFromEmbeddedMode() != ConnectionRoleHost {
		t.Error("roleFromEmbeddedMode() should default to host")
	}

	// Standalone mode
	manager.config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
	}
	if manager.roleFromEmbeddedMode() != ConnectionRoleHost {
		t.Error("roleFromEmbeddedMode() should be host for standalone")
	}

	// Leaf mode
	manager.config.EmbeddedConfig.Mode = EmbeddedNATSModeLeaf
	if manager.roleFromEmbeddedMode() != ConnectionRoleLeaf {
		t.Error("roleFromEmbeddedMode() should be leaf for leaf mode")
	}
}

func TestHybridModeManager_StopWithoutStart(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Stop without starting should not error
	if err := manager.Stop(); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestHybridModeManager_StartWithHostMode(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionManual
	config.ManualRole = ConnectionRoleHost
	config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode:           EmbeddedNATSModeStandalone,
		Host:           "127.0.0.1",
		Port:           helpers.FreePort(t),
		MaxConnections: 10,
	}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	// Wait for startup
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return manager.State() == HybridModeStateActive, nil
	}); err != nil {
		t.Fatalf("manager did not become active: %v", err)
	}

	if manager.State() != HybridModeStateActive {
		t.Errorf("State() = %v, want active", manager.State())
	}
	if manager.Role() != ConnectionRoleHost {
		t.Errorf("Role() = %v, want host", manager.Role())
	}
	if !manager.IsActive() {
		t.Error("IsActive() should be true")
	}
	if manager.GetEmbeddedServer() == nil {
		t.Error("GetEmbeddedServer() should not be nil")
	}
	if manager.GetConnection() == nil {
		t.Error("GetConnection() should not be nil (local client)")
	}
}

func TestHybridModeManager_StartTwice(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionManual
	config.ManualRole = ConnectionRoleHost
	config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Host: "127.0.0.1",
		Port: helpers.FreePort(t),
	}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	// Wait for startup
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return manager.State() == HybridModeStateActive, nil
	}); err != nil {
		t.Fatalf("manager did not become active: %v", err)
	}

	// Starting again should error
	if err := manager.Start(ctx); err == nil {
		t.Error("Start() should error when already started")
	}
}

func TestHybridModeManager_StartWithLeafMode(t *testing.T) {
	// This test requires a running NATS server for leaf connection
	// We'll test just the role determination
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionManual
	config.ManualRole = ConnectionRoleLeaf
	config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeLeaf,
		Host: "127.0.0.1",
		Port: helpers.FreePort(t),
		LeafRemotes: []LeafRemoteConfig{
			{URLs: []string{"nats://localhost:4222"}},
		},
	}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	role, err := manager.determineRole(context.Background())
	if err != nil {
		t.Errorf("determineRole() error = %v", err)
	}
	if role != ConnectionRoleLeaf {
		t.Errorf("determineRole() = %v, want leaf", role)
	}
}

func TestHybridModeManager_FallbackToClient(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionAuto
	config.FallbackToClient = true
	config.ExternalNATSURLs = []string{"nats://localhost:4222"}
	// No embedded config, so can't host

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Should select client since can't host
	role, err := manager.determineRole(context.Background())
	if err != nil {
		t.Errorf("determineRole() error = %v", err)
	}
	if role != ConnectionRoleClient {
		t.Errorf("determineRole() = %v, want client", role)
	}
}

func TestHybridModeManager_NoOptionsAvailable(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionPreferClient
	// No external URLs and no embedded config

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	// Should fail since no options available
	_, err = manager.determineRole(context.Background())
	if err == nil {
		t.Error("determineRole() should error when no options available")
	}
}

func TestHybridModeManager_StatsAfterStart(t *testing.T) {
	config := DefaultHybridModeConfig("test-agent")
	config.SelectionMode = RoleSelectionManual
	config.ManualRole = ConnectionRoleHost
	config.EmbeddedConfig = &EmbeddedNATSConfig{
		Mode: EmbeddedNATSModeStandalone,
		Host: "127.0.0.1",
		Port: helpers.FreePort(t),
	}

	manager, err := NewHybridModeManager(config)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	ctx := context.Background()
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer manager.Stop()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		stats := manager.GetStats()
		return stats.State == HybridModeStateActive && stats.ClientConnected, nil
	}); err != nil {
		t.Fatalf("stats did not update: %v", err)
	}

	stats := manager.GetStats()
	if stats.State != HybridModeStateActive {
		t.Errorf("stats.State = %v, want active", stats.State)
	}
	if stats.Role != ConnectionRoleHost {
		t.Errorf("stats.Role = %v, want host", stats.Role)
	}
	if !stats.ServerRunning {
		t.Error("stats.ServerRunning should be true")
	}
	if !stats.ClientConnected {
		t.Error("stats.ClientConnected should be true (local client)")
	}
}
