package failover

import (
	"context"
	"net"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Strategy != StrategyHappyEyeballs {
		t.Errorf("Default strategy = %s, want happy_eyeballs", cfg.Strategy)
	}
	if cfg.ConnectTimeout != 10*time.Second {
		t.Errorf("Default ConnectTimeout = %v", cfg.ConnectTimeout)
	}
	if cfg.HappyEyeballsDelay != 250*time.Millisecond {
		t.Errorf("Default HappyEyeballsDelay = %v", cfg.HappyEyeballsDelay)
	}
}

func TestEndpoint_String(t *testing.T) {
	tests := []struct {
		ep       *Endpoint
		expected string
	}{
		{
			ep:       &Endpoint{Address: "192.168.1.1", Port: 80, Protocol: ProtocolIPv4},
			expected: "192.168.1.1:80",
		},
		{
			ep:       &Endpoint{Address: "::1", Port: 443, Protocol: ProtocolIPv6},
			expected: "[::1]:443",
		},
		{
			ep:       &Endpoint{Address: "2001:db8::1", Port: 8080, Protocol: ProtocolIPv6},
			expected: "[2001:db8::1]:8080",
		},
	}

	for _, tt := range tests {
		if got := tt.ep.String(); got != tt.expected {
			t.Errorf("Endpoint.String() = %s, want %s", got, tt.expected)
		}
	}
}

func TestEndpoint_IsIPv6(t *testing.T) {
	ipv4 := &Endpoint{Protocol: ProtocolIPv4}
	ipv6 := &Endpoint{Protocol: ProtocolIPv6}

	if ipv4.IsIPv6() {
		t.Error("IPv4 endpoint should not report as IPv6")
	}
	if !ipv6.IsIPv6() {
		t.Error("IPv6 endpoint should report as IPv6")
	}
}

func TestNewResolver(t *testing.T) {
	// With nil config
	r := NewResolver(nil)
	if r == nil {
		t.Fatal("NewResolver returned nil")
	}
	if r.config.Strategy != StrategyHappyEyeballs {
		t.Error("Should use default config")
	}

	// With custom config
	cfg := &Config{Strategy: StrategyPreferIPv4}
	r = NewResolver(cfg)
	if r.config.Strategy != StrategyPreferIPv4 {
		t.Error("Should use provided config")
	}
}

func TestResolver_assignPriorities(t *testing.T) {
	endpoints := []*Endpoint{
		{Address: "192.168.1.1", Protocol: ProtocolIPv4},
		{Address: "::1", Protocol: ProtocolIPv6},
		{Address: "10.0.0.1", Protocol: ProtocolIPv4},
		{Address: "2001:db8::1", Protocol: ProtocolIPv6},
	}

	// Test prefer IPv6
	cfg := &Config{Strategy: StrategyPreferIPv6}
	r := NewResolver(cfg)
	r.assignPriorities(endpoints)

	for _, ep := range endpoints {
		if ep.IsIPv6() && ep.Priority != 0 {
			t.Errorf("IPv6 should have priority 0 with StrategyPreferIPv6")
		}
		if !ep.IsIPv6() && ep.Priority != 1 {
			t.Errorf("IPv4 should have priority 1 with StrategyPreferIPv6")
		}
	}

	// Test prefer IPv4
	cfg = &Config{Strategy: StrategyPreferIPv4}
	r = NewResolver(cfg)
	r.assignPriorities(endpoints)

	for _, ep := range endpoints {
		if ep.IsIPv6() && ep.Priority != 1 {
			t.Errorf("IPv6 should have priority 1 with StrategyPreferIPv4")
		}
		if !ep.IsIPv6() && ep.Priority != 0 {
			t.Errorf("IPv4 should have priority 0 with StrategyPreferIPv4")
		}
	}
}

func TestResolver_sortEndpoints(t *testing.T) {
	endpoints := []*Endpoint{
		{Address: "a", Priority: 2, Latency: 10 * time.Millisecond},
		{Address: "b", Priority: 0, Latency: 50 * time.Millisecond},
		{Address: "c", Priority: 0, Latency: 20 * time.Millisecond},
		{Address: "d", Priority: 1, Latency: 5 * time.Millisecond},
	}

	r := NewResolver(nil)
	sorted := r.sortEndpoints(endpoints)

	// Should be: c (pri 0, low latency), b (pri 0, high latency), d (pri 1), a (pri 2)
	if sorted[0].Address != "c" {
		t.Errorf("First should be 'c', got %s", sorted[0].Address)
	}
	if sorted[1].Address != "b" {
		t.Errorf("Second should be 'b', got %s", sorted[1].Address)
	}
	if sorted[2].Address != "d" {
		t.Errorf("Third should be 'd', got %s", sorted[2].Address)
	}
	if sorted[3].Address != "a" {
		t.Errorf("Fourth should be 'a', got %s", sorted[3].Address)
	}
}

func TestResolver_recordSuccess(t *testing.T) {
	r := NewResolver(nil)

	ep := &Endpoint{
		Address:             "192.168.1.1",
		Protocol:            ProtocolIPv4,
		Status:              StatusUnhealthy,
		ConsecutiveFailures: 5,
	}

	r.recordSuccess(ep)

	if ep.Status != StatusHealthy {
		t.Errorf("Status = %s, want healthy", ep.Status)
	}
	if ep.ConsecutiveFailures != 0 {
		t.Errorf("ConsecutiveFailures = %d, want 0", ep.ConsecutiveFailures)
	}
	if r.stats.IPv4Connections != 1 {
		t.Errorf("IPv4Connections = %d, want 1", r.stats.IPv4Connections)
	}
}

func TestResolver_recordFailure(t *testing.T) {
	cfg := DefaultConfig()
	cfg.MaxConsecutiveFailures = 3
	r := NewResolver(cfg)

	ep := &Endpoint{
		Address:             "::1",
		Protocol:            ProtocolIPv6,
		Status:              StatusHealthy,
		ConsecutiveFailures: 0,
	}

	// First failures should mark as degraded
	r.recordFailure(ep)
	r.recordFailure(ep)

	if ep.Status != StatusDegraded {
		t.Errorf("Status = %s, want degraded after 2 failures", ep.Status)
	}
	if ep.ConsecutiveFailures != 2 {
		t.Errorf("ConsecutiveFailures = %d, want 2", ep.ConsecutiveFailures)
	}

	// Third failure should mark as unhealthy
	r.recordFailure(ep)

	if ep.Status != StatusUnhealthy {
		t.Errorf("Status = %s, want unhealthy after 3 failures", ep.Status)
	}
	if r.stats.IPv6Failures != 3 {
		t.Errorf("IPv6Failures = %d, want 3", r.stats.IPv6Failures)
	}
}

func TestResolver_ClearCache(t *testing.T) {
	r := NewResolver(nil)

	// Add some endpoints
	r.mu.Lock()
	r.endpoints["example.com"] = []*Endpoint{
		{Address: "192.168.1.1"},
	}
	r.mu.Unlock()

	r.ClearCache()

	r.mu.RLock()
	count := len(r.endpoints)
	r.mu.RUnlock()

	if count != 0 {
		t.Errorf("Cache should be empty after ClearCache, got %d entries", count)
	}
}

func TestStats_RecordSuccess(t *testing.T) {
	s := NewStats()

	s.RecordSuccess(ProtocolIPv4)
	s.RecordSuccess(ProtocolIPv4)
	s.RecordSuccess(ProtocolIPv6)

	if s.IPv4Connections != 2 {
		t.Errorf("IPv4Connections = %d, want 2", s.IPv4Connections)
	}
	if s.IPv6Connections != 1 {
		t.Errorf("IPv6Connections = %d, want 1", s.IPv6Connections)
	}
}

func TestStats_RecordFailure(t *testing.T) {
	s := NewStats()

	s.RecordFailure(ProtocolIPv4)
	s.RecordFailure(ProtocolIPv6)
	s.RecordFailure(ProtocolIPv6)

	if s.IPv4Failures != 1 {
		t.Errorf("IPv4Failures = %d, want 1", s.IPv4Failures)
	}
	if s.IPv6Failures != 2 {
		t.Errorf("IPv6Failures = %d, want 2", s.IPv6Failures)
	}
}

func TestStats_Snapshot(t *testing.T) {
	s := NewStats()
	s.RecordSuccess(ProtocolIPv4)
	s.RecordFailure(ProtocolIPv6)

	snapshot := s.Snapshot()

	// Modify original
	s.RecordSuccess(ProtocolIPv4)

	// Snapshot should be unchanged
	if snapshot.IPv4Connections != 1 {
		t.Error("Snapshot should be independent copy")
	}
}

func TestParseAddress(t *testing.T) {
	tests := []struct {
		addr     string
		expected Protocol
	}{
		{"192.168.1.1:80", ProtocolIPv4},
		{"192.168.1.1", ProtocolIPv4},
		{"[::1]:443", ProtocolIPv6},
		{"::1", ProtocolIPv6},
		{"2001:db8::1", ProtocolIPv6},
		{"example.com:80", ProtocolAny},
		{"example.com", ProtocolAny},
	}

	for _, tt := range tests {
		got, err := ParseAddress(tt.addr)
		if err != nil {
			t.Errorf("ParseAddress(%q) error: %v", tt.addr, err)
			continue
		}
		if got != tt.expected {
			t.Errorf("ParseAddress(%q) = %s, want %s", tt.addr, got, tt.expected)
		}
	}
}

func TestResolver_ResolveHost_Localhost(t *testing.T) {
	r := NewResolver(nil)
	ctx := context.Background()

	// Resolve localhost - should work on all systems
	endpoints, err := r.ResolveHost(ctx, "localhost", 80)
	if err != nil {
		t.Skipf("Could not resolve localhost: %v", err)
	}

	if len(endpoints) == 0 {
		t.Error("Should resolve to at least one endpoint")
	}

	// Check that endpoints have correct port
	for _, ep := range endpoints {
		if ep.Port != 80 {
			t.Errorf("Endpoint port = %d, want 80", ep.Port)
		}
		if ep.Protocol != ProtocolIPv4 && ep.Protocol != ProtocolIPv6 {
			t.Errorf("Invalid protocol: %s", ep.Protocol)
		}
	}
}

func TestResolver_ResolveHost_Caching(t *testing.T) {
	r := NewResolver(nil)
	ctx := context.Background()

	// First resolve
	endpoints1, err := r.ResolveHost(ctx, "localhost", 80)
	if err != nil {
		t.Skipf("Could not resolve localhost: %v", err)
	}

	// Second resolve should return cached
	endpoints2, err := r.ResolveHost(ctx, "localhost", 80)
	if err != nil {
		t.Fatalf("Second resolve failed: %v", err)
	}

	// Should be same endpoints (from cache)
	if len(endpoints1) != len(endpoints2) {
		t.Error("Cached resolve should return same endpoints")
	}
}

// Integration test - requires network
func TestResolver_Connect_Localhost(t *testing.T) {
	// Start a test server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port

	// Accept connections in background
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			conn.Close()
		}
	}()

	r := NewResolver(nil)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	conn, ep, err := r.Connect(ctx, "127.0.0.1", port)
	if err != nil {
		t.Fatalf("Connect failed: %v", err)
	}
	defer conn.Close()

	if ep == nil {
		t.Error("Endpoint should not be nil")
	}
	if ep.Protocol != ProtocolIPv4 {
		t.Errorf("Protocol = %s, want ipv4", ep.Protocol)
	}
}

func TestResolver_Connect_Failure(t *testing.T) {
	r := NewResolver(&Config{
		Strategy:       StrategyPreferIPv4,
		ConnectTimeout: 100 * time.Millisecond,
		RetryAttempts:  1,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Try to connect to a port that's not listening
	_, _, err := r.Connect(ctx, "127.0.0.1", 59999)
	if err == nil {
		t.Error("Should fail to connect to non-listening port")
	}
}

func TestResolver_RoundRobin(t *testing.T) {
	r := NewResolver(&Config{
		Strategy:       StrategyRoundRobin,
		ConnectTimeout: 100 * time.Millisecond,
		RetryAttempts:  1,
	})

	// Pre-populate with test endpoints
	r.mu.Lock()
	r.endpoints["test.local"] = []*Endpoint{
		{Address: "192.168.1.1", Port: 80, Protocol: ProtocolIPv4, Status: StatusHealthy},
		{Address: "192.168.1.2", Port: 80, Protocol: ProtocolIPv4, Status: StatusHealthy},
		{Address: "192.168.1.3", Port: 80, Protocol: ProtocolIPv4, Status: StatusHealthy},
	}
	r.mu.Unlock()

	// Test that round-robin counter increments
	initial := r.rrCounter

	ctx := context.Background()
	// These will fail but should increment counter
	r.Connect(ctx, "test.local", 80)
	r.Connect(ctx, "test.local", 80)
	r.Connect(ctx, "test.local", 80)

	if r.rrCounter <= initial {
		t.Error("Round-robin counter should increment")
	}
}
