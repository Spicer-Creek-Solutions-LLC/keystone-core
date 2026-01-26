package nats

import (
	"testing"
	"time"
)

func TestConnectionState_String(t *testing.T) {
	tests := []struct {
		state ConnectionState
		want  string
	}{
		{ConnectionStateDisconnected, "disconnected"},
		{ConnectionStateConnecting, "connecting"},
		{ConnectionStateConnected, "connected"},
		{ConnectionStateReconnecting, "reconnecting"},
		{ConnectionStateClosed, "closed"},
		{ConnectionState(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.state.String(); got != tt.want {
			t.Errorf("ConnectionState(%d).String() = %s, want %s", tt.state, got, tt.want)
		}
	}
}

func TestEndpointState_AverageLatency(t *testing.T) {
	tests := []struct {
		name         string
		successCount int64
		totalLatency time.Duration
		want         time.Duration
	}{
		{"no attempts", 0, 0, 0},
		{"single attempt", 1, 100 * time.Millisecond, 100 * time.Millisecond},
		{"multiple attempts", 4, 400 * time.Millisecond, 100 * time.Millisecond},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &EndpointState{
				SuccessCount: tt.successCount,
				TotalLatency: tt.totalLatency,
			}
			if got := state.AverageLatency(); got != tt.want {
				t.Errorf("AverageLatency() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndpointState_IsHealthy(t *testing.T) {
	tests := []struct {
		name        string
		state       ConnectionState
		circuitOpen bool
		want        bool
	}{
		{"connected no circuit", ConnectionStateConnected, false, true},
		{"connected with circuit", ConnectionStateConnected, true, false},
		{"disconnected no circuit", ConnectionStateDisconnected, false, false},
		{"reconnecting", ConnectionStateReconnecting, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &EndpointState{
				State:       tt.state,
				CircuitOpen: tt.circuitOpen,
			}
			if got := state.IsHealthy(); got != tt.want {
				t.Errorf("IsHealthy() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndpointState_SuccessRate(t *testing.T) {
	tests := []struct {
		name         string
		successCount int64
		failureCount int64
		want         float64
	}{
		{"no attempts", 0, 0, 1.0},
		{"all success", 10, 0, 1.0},
		{"all failure", 0, 10, 0.0},
		{"50/50", 5, 5, 0.5},
		{"75% success", 3, 1, 0.75},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			state := &EndpointState{
				SuccessCount: tt.successCount,
				FailureCount: tt.failureCount,
			}
			if got := state.SuccessRate(); got != tt.want {
				t.Errorf("SuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestDefaultCircuitBreakerConfig(t *testing.T) {
	config := DefaultCircuitBreakerConfig()

	if config.FailureThreshold != 5 {
		t.Errorf("FailureThreshold = %d, want 5", config.FailureThreshold)
	}
	if config.SuccessThreshold != 3 {
		t.Errorf("SuccessThreshold = %d, want 3", config.SuccessThreshold)
	}
	if config.OpenDuration != 30*time.Second {
		t.Errorf("OpenDuration = %v, want 30s", config.OpenDuration)
	}
	if config.HalfOpenMaxAttempts != 1 {
		t.Errorf("HalfOpenMaxAttempts = %d, want 1", config.HalfOpenMaxAttempts)
	}
}

func TestDefaultConnectionManagerConfig(t *testing.T) {
	config := DefaultConnectionManagerConfig()

	if config.EndpointConfig == nil {
		t.Error("EndpointConfig is nil")
	}
	if config.StrategyConfig == nil {
		t.Error("StrategyConfig is nil")
	}
	if config.CircuitBreaker == nil {
		t.Error("CircuitBreaker is nil")
	}
	if config.HealthCheckInterval != 30*time.Second {
		t.Errorf("HealthCheckInterval = %v, want 30s", config.HealthCheckInterval)
	}
	if config.FailoverTimeout != 10*time.Second {
		t.Errorf("FailoverTimeout = %v, want 10s", config.FailoverTimeout)
	}
}

func TestNewPooledConnectionManager(t *testing.T) {
	tests := []struct {
		name    string
		config  *ConnectionManagerConfig
		wantErr bool
	}{
		{
			name:    "nil config uses defaults",
			config:  nil,
			wantErr: false,
		},
		{
			name: "valid config",
			config: &ConnectionManagerConfig{
				EndpointConfig: &EndpointConfig{
					URLs: []string{"nats://localhost:4222"},
				},
			},
			wantErr: false,
		},
		{
			name: "empty URLs",
			config: &ConnectionManagerConfig{
				EndpointConfig: &EndpointConfig{
					URLs: []string{},
				},
			},
			wantErr: true,
		},
		{
			name: "invalid URL",
			config: &ConnectionManagerConfig{
				EndpointConfig: &EndpointConfig{
					URLs: []string{"http://invalid"},
				},
			},
			wantErr: true,
		},
		{
			name: "nil endpoint config",
			config: &ConnectionManagerConfig{
				EndpointConfig: nil,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mgr, err := NewPooledConnectionManager(tt.config)
			if tt.wantErr {
				if err == nil {
					t.Error("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if mgr == nil {
				t.Error("manager is nil")
			}
			defer mgr.Close()
		})
	}
}

func TestPooledConnectionManager_MultipleEndpoints(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{
				"nats://nats1:4222",
				"nats://nats2:4222",
				"nats://nats3:4222",
			},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	states := mgr.EndpointStates()
	if len(states) != 3 {
		t.Errorf("got %d endpoints, want 3", len(states))
	}

	for i, state := range states {
		if state.State != ConnectionStateDisconnected {
			t.Errorf("endpoint[%d] state = %v, want disconnected", i, state.State)
		}
	}
}

func TestPooledConnectionManager_State(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Initial state should be disconnected
	if mgr.State() != ConnectionStateDisconnected {
		t.Errorf("initial state = %v, want disconnected", mgr.State())
	}

	// After close, state should be closed
	mgr.Close()
	if mgr.State() != ConnectionStateClosed {
		t.Errorf("closed state = %v, want closed", mgr.State())
	}
}

func TestPooledConnectionManager_IsConnected(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	if mgr.IsConnected() {
		t.Error("IsConnected() should be false initially")
	}
}

func TestPooledConnectionManager_Connection(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	if mgr.Connection() != nil {
		t.Error("Connection() should be nil initially")
	}
}

func TestPooledConnectionManager_ActiveEndpoint(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	if mgr.ActiveEndpoint() != nil {
		t.Error("ActiveEndpoint() should be nil initially")
	}
}

func TestPooledConnectionManager_Close(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First close should succeed
	if err := mgr.Close(); err != nil {
		t.Errorf("first Close() error = %v", err)
	}

	// Second close should be idempotent
	if err := mgr.Close(); err != nil {
		t.Errorf("second Close() error = %v", err)
	}
}

func TestPooledConnectionManager_ConnectAfterClose(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mgr.Close()

	if err := mgr.Connect(); err == nil {
		t.Error("Connect() after Close() should return error")
	}
}

func TestPooledConnectionManager_ReconnectAfterClose(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mgr.Close()

	if err := mgr.Reconnect(); err == nil {
		t.Error("Reconnect() after Close() should return error")
	}
}

func TestPooledConnectionManager_FailoverAfterClose(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	mgr.Close()

	if err := mgr.Failover(); err == nil {
		t.Error("Failover() after Close() should return error")
	}
}

func TestPooledConnectionManager_Flush_NotConnected(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	if err := mgr.Flush(); err == nil {
		t.Error("Flush() when not connected should return error")
	}
}

func TestPooledConnectionManager_Drain_NotConnected(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Drain when not connected should not error
	if err := mgr.Drain(); err != nil {
		t.Errorf("Drain() when not connected error = %v", err)
	}
}

func TestPooledConnectionManager_RTT_NotConnected(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	_, err = mgr.RTT()
	if err == nil {
		t.Error("RTT() when not connected should return error")
	}
}

func TestPooledConnectionManager_Stats(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Stats when not connected should return empty stats
	stats := mgr.Stats()
	if stats.InMsgs != 0 {
		t.Errorf("Stats().InMsgs = %d, want 0", stats.InMsgs)
	}
}

func TestPooledConnectionManager_EndpointOrdering(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{
				"nats://nats1:4222?priority=2",
				"nats://nats2:4222?priority=0",
				"nats://nats3:4222?priority=1",
			},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Get ordered endpoints
	ordered := mgr.getOrderedEndpoints()
	if len(ordered) != 3 {
		t.Fatalf("got %d endpoints, want 3", len(ordered))
	}

	// Should be ordered by priority
	if ordered[0].Endpoint.Priority != 0 {
		t.Errorf("first endpoint priority = %d, want 0", ordered[0].Endpoint.Priority)
	}
	if ordered[1].Endpoint.Priority != 1 {
		t.Errorf("second endpoint priority = %d, want 1", ordered[1].Endpoint.Priority)
	}
	if ordered[2].Endpoint.Priority != 2 {
		t.Errorf("third endpoint priority = %d, want 2", ordered[2].Endpoint.Priority)
	}
}

func TestPooledConnectionManager_CircuitBreakerOpens(t *testing.T) {
	var circuitOpened bool

	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
		CircuitBreaker: &CircuitBreakerConfig{
			FailureThreshold: 2,
			OpenDuration:     time.Minute,
		},
		ConnectionCallbacks: ConnectionCallbacks{
			OnCircuitOpen: func(endpoint *Endpoint) {
				circuitOpened = true
			},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Simulate failures
	state := mgr.endpoints[0]
	mgr.recordFailure(state, errSimulated)
	mgr.recordFailure(state, errSimulated)

	if !state.CircuitOpen {
		t.Error("circuit should be open after failures")
	}
	if !circuitOpened {
		t.Error("OnCircuitOpen callback should have been called")
	}
}

var errSimulated = errTestError{}

type errTestError struct{}

func (e errTestError) Error() string { return "simulated error" }

func TestPooledConnectionManager_CircuitBreakerCloses(t *testing.T) {
	var circuitClosed bool

	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
		CircuitBreaker: &CircuitBreakerConfig{
			FailureThreshold: 2,
			OpenDuration:     time.Minute,
		},
		ConnectionCallbacks: ConnectionCallbacks{
			OnCircuitClose: func(endpoint *Endpoint) {
				circuitClosed = true
			},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Open the circuit
	state := mgr.endpoints[0]
	state.CircuitOpen = true

	// Record success should close circuit
	mgr.recordSuccess(state, 100*time.Millisecond)

	if state.CircuitOpen {
		t.Error("circuit should be closed after success")
	}
	if !circuitClosed {
		t.Error("OnCircuitClose callback should have been called")
	}
}

func TestPooledConnectionManager_Callbacks(t *testing.T) {
	var connectCalled, disconnectCalled, errorCalled bool

	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{"nats://localhost:4222"},
		},
		ConnectionCallbacks: ConnectionCallbacks{
			OnConnect: func(endpoint *Endpoint) {
				connectCalled = true
			},
			OnDisconnect: func(endpoint *Endpoint, err error) {
				disconnectCalled = true
			},
			OnError: func(endpoint *Endpoint, err error) {
				errorCalled = true
			},
		},
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Verify callbacks are configured (we can't trigger them without a real connection)
	if mgr.config.ConnectionCallbacks.OnConnect == nil {
		t.Error("OnConnect callback should be set")
	}
	if mgr.config.ConnectionCallbacks.OnDisconnect == nil {
		t.Error("OnDisconnect callback should be set")
	}
	if mgr.config.ConnectionCallbacks.OnError == nil {
		t.Error("OnError callback should be set")
	}

	// Suppress unused variable warnings
	_ = connectCalled
	_ = disconnectCalled
	_ = errorCalled
}

func TestPooledConnectionManager_WithDifferentSchemes(t *testing.T) {
	tests := []struct {
		name string
		urls []string
	}{
		{"nats", []string{"nats://localhost:4222"}},
		{"tls", []string{"tls://localhost:4222"}},
		{"ws", []string{"ws://localhost:80"}},
		{"wss", []string{"wss://localhost:443"}},
		{"mixed", []string{"nats://nats1:4222", "tls://nats2:4222"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &ConnectionManagerConfig{
				EndpointConfig: &EndpointConfig{
					URLs: tt.urls,
				},
			}

			mgr, err := NewPooledConnectionManager(config)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			defer mgr.Close()

			if len(mgr.endpoints) != len(tt.urls) {
				t.Errorf("got %d endpoints, want %d", len(mgr.endpoints), len(tt.urls))
			}
		})
	}
}

func TestAddressFamilyPreference_String(t *testing.T) {
	tests := []struct {
		pref AddressFamilyPreference
		want string
	}{
		{PreferIPv4, "prefer_ipv4"},
		{PreferIPv6, "prefer_ipv6"},
		{IPv4Only, "ipv4_only"},
		{IPv6Only, "ipv6_only"},
		{AddressFamilyPreference(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.pref.String(); got != tt.want {
			t.Errorf("AddressFamilyPreference(%d).String() = %s, want %s", tt.pref, got, tt.want)
		}
	}
}

func TestPooledConnectionManager_AddressFamilyPreference_IPv4Only(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{
				"nats://192.168.1.1:4222",
				"nats://[::1]:4222",
				"nats://10.0.0.1:4222",
				"nats://[2001:db8::1]:4222",
			},
		},
		AddressFamilyPreference: IPv4Only,
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Get ordered endpoints
	ordered := mgr.getOrderedEndpoints()

	// IPv4Only should filter out all IPv6 endpoints
	if len(ordered) != 2 {
		t.Fatalf("got %d endpoints, want 2 (IPv4 only)", len(ordered))
	}

	for i, state := range ordered {
		if state.Endpoint.IsIPv6() {
			t.Errorf("endpoint[%d] is IPv6, but IPv4Only should filter them out", i)
		}
	}
}

func TestPooledConnectionManager_AddressFamilyPreference_IPv6Only(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{
				"nats://192.168.1.1:4222",
				"nats://[::1]:4222",
				"nats://10.0.0.1:4222",
				"nats://[2001:db8::1]:4222",
			},
		},
		AddressFamilyPreference: IPv6Only,
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Get ordered endpoints
	ordered := mgr.getOrderedEndpoints()

	// IPv6Only should filter out all IPv4 endpoints
	if len(ordered) != 2 {
		t.Fatalf("got %d endpoints, want 2 (IPv6 only)", len(ordered))
	}

	for i, state := range ordered {
		if !state.Endpoint.IsIPv6() {
			t.Errorf("endpoint[%d] is IPv4, but IPv6Only should filter them out", i)
		}
	}
}

func TestPooledConnectionManager_AddressFamilyPreference_PreferIPv4(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{
				"nats://[::1]:4222",
				"nats://192.168.1.1:4222",
				"nats://[2001:db8::1]:4222",
				"nats://10.0.0.1:4222",
			},
		},
		AddressFamilyPreference: PreferIPv4,
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Get ordered endpoints
	ordered := mgr.getOrderedEndpoints()

	// PreferIPv4 should keep all endpoints but order IPv4 first
	if len(ordered) != 4 {
		t.Fatalf("got %d endpoints, want 4 (all endpoints kept)", len(ordered))
	}

	// First two should be IPv4
	for i := 0; i < 2; i++ {
		if ordered[i].Endpoint.IsIPv6() {
			t.Errorf("endpoint[%d] should be IPv4 (preferred), got IPv6", i)
		}
	}

	// Last two should be IPv6
	for i := 2; i < 4; i++ {
		if !ordered[i].Endpoint.IsIPv6() {
			t.Errorf("endpoint[%d] should be IPv6 (fallback), got IPv4", i)
		}
	}
}

func TestPooledConnectionManager_AddressFamilyPreference_PreferIPv6(t *testing.T) {
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{
				"nats://192.168.1.1:4222",
				"nats://[::1]:4222",
				"nats://10.0.0.1:4222",
				"nats://[2001:db8::1]:4222",
			},
		},
		AddressFamilyPreference: PreferIPv6,
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	// Get ordered endpoints
	ordered := mgr.getOrderedEndpoints()

	// PreferIPv6 should keep all endpoints but order IPv6 first
	if len(ordered) != 4 {
		t.Fatalf("got %d endpoints, want 4 (all endpoints kept)", len(ordered))
	}

	// First two should be IPv6
	for i := 0; i < 2; i++ {
		if !ordered[i].Endpoint.IsIPv6() {
			t.Errorf("endpoint[%d] should be IPv6 (preferred), got IPv4", i)
		}
	}

	// Last two should be IPv4
	for i := 2; i < 4; i++ {
		if ordered[i].Endpoint.IsIPv6() {
			t.Errorf("endpoint[%d] should be IPv4 (fallback), got IPv6", i)
		}
	}
}

func TestPooledConnectionManager_AddressFamilyPreference_Default(t *testing.T) {
	// Test that default (PreferIPv4) is applied when not specified
	config := &ConnectionManagerConfig{
		EndpointConfig: &EndpointConfig{
			URLs: []string{
				"nats://[::1]:4222",
				"nats://192.168.1.1:4222",
			},
		},
		// AddressFamilyPreference not set, defaults to PreferIPv4 (0)
	}

	mgr, err := NewPooledConnectionManager(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer mgr.Close()

	if mgr.config.AddressFamilyPreference != PreferIPv4 {
		t.Errorf("default AddressFamilyPreference = %v, want PreferIPv4", mgr.config.AddressFamilyPreference)
	}

	// Get ordered endpoints - with default PreferIPv4, IPv4 should be first
	ordered := mgr.getOrderedEndpoints()
	if len(ordered) != 2 {
		t.Fatalf("got %d endpoints, want 2", len(ordered))
	}

	if ordered[0].Endpoint.IsIPv6() {
		t.Error("first endpoint should be IPv4 (default preference)")
	}
}
