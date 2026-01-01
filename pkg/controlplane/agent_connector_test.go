package controlplane

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/shawnbutts/keystone-core/pkg/agent"
)

func TestAgentConnectionState_String(t *testing.T) {
	tests := []struct {
		state    AgentConnectionState
		expected string
	}{
		{AgentConnectionStateDisconnected, "disconnected"},
		{AgentConnectionStateConnecting, "connecting"},
		{AgentConnectionStateConnected, "connected"},
		{AgentConnectionStateReconnecting, "reconnecting"},
		{AgentConnectionStateFailed, "failed"},
		{AgentConnectionState(99), "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.state.String(); got != tt.expected {
				t.Errorf("AgentConnectionState.String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDefaultAgentConnectorConfig(t *testing.T) {
	cfg := DefaultAgentConnectorConfig()

	if cfg.ConnectTimeout != 10*time.Second {
		t.Errorf("ConnectTimeout = %v, want 10s", cfg.ConnectTimeout)
	}
	if cfg.ReconnectWait != 2*time.Second {
		t.Errorf("ReconnectWait = %v, want 2s", cfg.ReconnectWait)
	}
	if cfg.MaxReconnects != -1 {
		t.Errorf("MaxReconnects = %v, want -1", cfg.MaxReconnects)
	}
	if cfg.PingInterval != 30*time.Second {
		t.Errorf("PingInterval = %v, want 30s", cfg.PingInterval)
	}
	if cfg.MaxPingsOut != 3 {
		t.Errorf("MaxPingsOut = %v, want 3", cfg.MaxPingsOut)
	}
	if cfg.MaxConnectionsPerAgent != 1 {
		t.Errorf("MaxConnectionsPerAgent = %v, want 1", cfg.MaxConnectionsPerAgent)
	}
	if cfg.DiscoveryInterval != 30*time.Second {
		t.Errorf("DiscoveryInterval = %v, want 30s", cfg.DiscoveryInterval)
	}
	if cfg.CleanupInterval != 60*time.Second {
		t.Errorf("CleanupInterval = %v, want 60s", cfg.CleanupInterval)
	}
}

func TestNewAgentConnector(t *testing.T) {
	registry := agent.NewEndpointRegistry()

	tests := []struct {
		name     string
		config   *AgentConnectorConfig
		registry *agent.EndpointRegistry
		wantErr  bool
	}{
		{
			name:     "nil config uses defaults",
			config:   nil,
			registry: registry,
			wantErr:  false,
		},
		{
			name:     "nil registry",
			config:   DefaultAgentConnectorConfig(),
			registry: nil,
			wantErr:  true,
		},
		{
			name:     "valid config and registry",
			config:   DefaultAgentConnectorConfig(),
			registry: registry,
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			connector, err := NewAgentConnector(tt.config, tt.registry)
			if (err != nil) != tt.wantErr {
				t.Errorf("NewAgentConnector() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && connector == nil {
				t.Error("expected connector to be non-nil")
			}
		})
	}
}

func TestAgentConnector_Lifecycle(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	ctx := context.Background()

	// Start
	if err := connector.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	if !connector.IsRunning() {
		t.Error("IsRunning() = false, want true")
	}

	// Start again should error
	if err := connector.Start(ctx); err == nil {
		t.Error("Start() should error when already running")
	}

	// Stop
	if err := connector.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	if connector.IsRunning() {
		t.Error("IsRunning() = true after stop, want false")
	}
}

func TestAgentConnector_GetStats(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	stats := connector.GetStats()
	if stats == nil {
		t.Fatal("GetStats() = nil")
	}

	if stats.TotalConnections != 0 {
		t.Errorf("TotalConnections = %d, want 0", stats.TotalConnections)
	}
	if stats.ConnectedCount != 0 {
		t.Errorf("ConnectedCount = %d, want 0", stats.ConnectedCount)
	}
}

func TestAgentConnector_ConnectToUnknownAgent(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	// Try to connect to non-existent agent
	err = connector.ConnectToAgent("unknown-agent")
	if err == nil {
		t.Error("ConnectToAgent() should error for unknown agent")
	}
}

func TestAgentConnector_GetConnection(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	// Get non-existent connection
	conn := connector.GetConnection("unknown-agent")
	if conn != nil {
		t.Error("GetConnection() should return nil for unknown agent")
	}
}

func TestAgentConnector_GetConnectedAgents(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	connected := connector.GetConnectedAgents()
	if len(connected) != 0 {
		t.Errorf("GetConnectedAgents() = %v, want empty", connected)
	}
}

func TestAgentConnector_ConnectionCount(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	count := connector.ConnectionCount()
	if count != 0 {
		t.Errorf("ConnectionCount() = %d, want 0", count)
	}
}

func TestAgentConnector_DisconnectFromUnknownAgent(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	// Disconnect from non-existent agent should not error
	err = connector.DisconnectFromAgent("unknown-agent")
	if err != nil {
		t.Errorf("DisconnectFromAgent() error = %v, want nil", err)
	}
}

func TestAgentConnector_Callbacks(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	var connectCalls []string
	var disconnectCalls []struct {
		agentID string
		err     error
	}
	var mu sync.Mutex

	connector.SetConnectCallback(func(agentID string, conn *nats.Conn) {
		mu.Lock()
		connectCalls = append(connectCalls, agentID)
		mu.Unlock()
	})

	connector.SetDisconnectCallback(func(agentID string, err error) {
		mu.Lock()
		disconnectCalls = append(disconnectCalls, struct {
			agentID string
			err     error
		}{agentID, err})
		mu.Unlock()
	})

	// Callbacks are set, verify they're stored
	if connector.onConnect == nil {
		t.Error("onConnect callback should be set")
	}
	if connector.onDisconnect == nil {
		t.Error("onDisconnect callback should be set")
	}
}

func TestAgentConnector_PublishToUnknownAgent(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	err = connector.PublishToAgent("unknown-agent", "test.subject", []byte("test"))
	if err == nil {
		t.Error("PublishToAgent() should error for unknown agent")
	}
}

func TestAgentConnector_RequestToUnknownAgent(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	_, err = connector.RequestToAgent("unknown-agent", "test.subject", []byte("test"), time.Second)
	if err == nil {
		t.Error("RequestToAgent() should error for unknown agent")
	}
}

func TestAgentConnector_SubscribeOnUnknownAgent(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	_, err = connector.SubscribeOnAgent("unknown-agent", "test.subject", func(msg *nats.Msg) {})
	if err == nil {
		t.Error("SubscribeOnAgent() should error for unknown agent")
	}
}

func TestAgentConnection_State(t *testing.T) {
	conn := &AgentConnection{
		AgentID: "test-agent",
	}

	// Initial state should be disconnected
	if conn.State() != AgentConnectionStateDisconnected {
		t.Errorf("State() = %v, want disconnected", conn.State())
	}

	if conn.IsConnected() {
		t.Error("IsConnected() = true, want false")
	}

	// Simulate connection
	conn.state.Store(int32(AgentConnectionStateConnected))
	if !conn.IsConnected() {
		t.Error("IsConnected() = false, want true")
	}

	// Check Conn() returns nil initially
	if conn.Conn() != nil {
		t.Error("Conn() should be nil")
	}

	// Check LastError returns nil initially
	if conn.LastError() != nil {
		t.Error("LastError() should be nil initially")
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

func TestAgentConnector_HandleEndpointChange(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	cfg := DefaultAgentConnectorConfig()
	cfg.DiscoveryInterval = 1 * time.Hour  // Prevent auto-discovery
	cfg.CleanupInterval = 1 * time.Hour    // Prevent auto-cleanup

	connector, err := NewAgentConnector(cfg, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	ctx := context.Background()
	if err := connector.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer connector.Stop()

	// Register an endpoint (will trigger handleEndpointChange via callback)
	adv := &agent.EndpointAdvertisement{
		AgentID:        "agent-1",
		Host:           "127.0.0.1",
		Port:           14300,
		TTL:            30,
		Timestamp:      time.Now(),
		SequenceNumber: 1,
		HealthStatus:   agent.EndpointHealthHealthy,
	}

	if err := registry.Register(adv); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	// Give time for async connection attempt
	time.Sleep(100 * time.Millisecond)

	// Connection should be tracked (even if failed due to no server)
	conn := connector.GetConnection("agent-1")
	if conn == nil {
		t.Error("GetConnection() = nil, expected connection record")
	}

	// Unregister should trigger disconnect
	registry.Unregister("agent-1")

	// Give time for cleanup
	time.Sleep(100 * time.Millisecond)

	// Connection should be removed
	conn = connector.GetConnection("agent-1")
	if conn != nil {
		t.Error("GetConnection() should return nil after unregister")
	}
}

func TestAgentConnector_TLSRequired(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	cfg := DefaultAgentConnectorConfig()
	cfg.TLSRequired = true

	connector, err := NewAgentConnector(cfg, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	// Register non-TLS endpoint
	adv := &agent.EndpointAdvertisement{
		AgentID:        "agent-1",
		Host:           "127.0.0.1",
		Port:           14301,
		TLSEnabled:     false,
		TTL:            30,
		Timestamp:      time.Now(),
		SequenceNumber: 1,
		HealthStatus:   agent.EndpointHealthHealthy,
	}

	registry.Register(adv)

	// Start connector
	ctx := context.Background()
	if err := connector.Start(ctx); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer connector.Stop()

	// Give time for connection attempt
	time.Sleep(100 * time.Millisecond)

	// Connection should be tracked
	conn := connector.GetConnection("agent-1")
	if conn == nil {
		t.Fatal("GetConnection() = nil")
	}

	// Should be in failed state due to TLS requirement
	if conn.State() != AgentConnectionStateFailed {
		t.Errorf("State() = %v, want failed", conn.State())
	}

	lastErr := conn.LastError()
	if lastErr == nil {
		t.Error("LastError() should not be nil")
	}
}

func TestAgentConnector_StopWithoutStart(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	// Stop without starting should not error
	if err := connector.Stop(); err != nil {
		t.Errorf("Stop() error = %v, want nil", err)
	}
}

func TestAgentConnectorStats(t *testing.T) {
	registry := agent.NewEndpointRegistry()
	connector, err := NewAgentConnector(nil, registry)
	if err != nil {
		t.Fatalf("failed to create connector: %v", err)
	}

	// Add some connections with different states
	connector.connMu.Lock()
	connector.connections["agent-1"] = &AgentConnection{AgentID: "agent-1"}
	connector.connections["agent-1"].state.Store(int32(AgentConnectionStateConnected))

	connector.connections["agent-2"] = &AgentConnection{AgentID: "agent-2"}
	connector.connections["agent-2"].state.Store(int32(AgentConnectionStateConnecting))

	connector.connections["agent-3"] = &AgentConnection{AgentID: "agent-3"}
	connector.connections["agent-3"].state.Store(int32(AgentConnectionStateFailed))
	connector.connections["agent-3"].connectAttempts.Store(5)
	connector.connMu.Unlock()

	stats := connector.GetStats()

	if stats.TotalConnections != 3 {
		t.Errorf("TotalConnections = %d, want 3", stats.TotalConnections)
	}
	if stats.ConnectedCount != 1 {
		t.Errorf("ConnectedCount = %d, want 1", stats.ConnectedCount)
	}
	if stats.ConnectingCount != 1 {
		t.Errorf("ConnectingCount = %d, want 1", stats.ConnectingCount)
	}
	if stats.FailedCount != 1 {
		t.Errorf("FailedCount = %d, want 1", stats.FailedCount)
	}
	if stats.TotalConnectAttempts != 5 {
		t.Errorf("TotalConnectAttempts = %d, want 5", stats.TotalConnectAttempts)
	}
}
