package nats

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestHealthStatus_String(t *testing.T) {
	tests := []struct {
		status HealthStatus
		want   string
	}{
		{HealthStatusUnknown, "unknown"},
		{HealthStatusHealthy, "healthy"},
		{HealthStatusDegraded, "degraded"},
		{HealthStatusUnhealthy, "unhealthy"},
		{HealthStatus(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.status.String(); got != tt.want {
			t.Errorf("HealthStatus(%d).String() = %s, want %s", tt.status, got, tt.want)
		}
	}
}

func TestHealthStatus_IsAvailable(t *testing.T) {
	tests := []struct {
		status HealthStatus
		want   bool
	}{
		{HealthStatusUnknown, false},
		{HealthStatusHealthy, true},
		{HealthStatusDegraded, true},
		{HealthStatusUnhealthy, false},
	}

	for _, tt := range tests {
		if got := tt.status.IsAvailable(); got != tt.want {
			t.Errorf("HealthStatus(%d).IsAvailable() = %v, want %v", tt.status, got, tt.want)
		}
	}
}

func TestDefaultHealthConfig(t *testing.T) {
	config := DefaultHealthConfig()

	if config.CheckInterval != 30*time.Second {
		t.Errorf("CheckInterval = %v, want 30s", config.CheckInterval)
	}
	if config.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", config.Timeout)
	}
	if config.HealthyThreshold != 2 {
		t.Errorf("HealthyThreshold = %d, want 2", config.HealthyThreshold)
	}
	if config.UnhealthyThreshold != 3 {
		t.Errorf("UnhealthyThreshold = %d, want 3", config.UnhealthyThreshold)
	}
	if config.DegradedLatencyThreshold != 100*time.Millisecond {
		t.Errorf("DegradedLatencyThreshold = %v, want 100ms", config.DegradedLatencyThreshold)
	}
}

func TestEndpointHealth_SuccessRate(t *testing.T) {
	tests := []struct {
		name         string
		successCount int64
		failureCount int64
		want         float64
	}{
		{"no checks", 0, 0, 1.0},
		{"all success", 10, 0, 1.0},
		{"all failure", 0, 10, 0.0},
		{"50/50", 5, 5, 0.5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &EndpointHealth{
				SuccessCount: tt.successCount,
				FailureCount: tt.failureCount,
			}
			if got := h.SuccessRate(); got != tt.want {
				t.Errorf("SuccessRate() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEndpointHealth_Score(t *testing.T) {
	tests := []struct {
		name         string
		status       HealthStatus
		successCount int64
		failureCount int64
		wantMin      float64
		wantMax      float64
	}{
		{"healthy 100%", HealthStatusHealthy, 10, 0, 1.0, 1.0},
		{"healthy 50%", HealthStatusHealthy, 5, 5, 0.5, 0.5},
		{"degraded 100%", HealthStatusDegraded, 10, 0, 0.74, 0.76},
		{"unknown 100%", HealthStatusUnknown, 10, 0, 0.49, 0.51},
		{"unhealthy", HealthStatusUnhealthy, 10, 0, 0.0, 0.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := &EndpointHealth{
				Status:       tt.status,
				SuccessCount: tt.successCount,
				FailureCount: tt.failureCount,
			}
			got := h.Score()
			if got < tt.wantMin || got > tt.wantMax {
				t.Errorf("Score() = %v, want between %v and %v", got, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestEndpointHealth_recordLatency(t *testing.T) {
	h := &EndpointHealth{}

	// Record some latencies
	for i := 0; i < 10; i++ {
		h.recordLatency(time.Duration(i+1) * time.Millisecond)
	}

	if h.Latency != 10*time.Millisecond {
		t.Errorf("Latency = %v, want 10ms", h.Latency)
	}
	if len(h.latencies) != 10 {
		t.Errorf("latencies length = %d, want 10", len(h.latencies))
	}

	// P50 should be around 5-6ms
	if h.LatencyP50 < 4*time.Millisecond || h.LatencyP50 > 7*time.Millisecond {
		t.Errorf("LatencyP50 = %v, want around 5-6ms", h.LatencyP50)
	}
}

func TestEndpointHealth_recordLatency_MaxSize(t *testing.T) {
	h := &EndpointHealth{}

	// Record more than 100 latencies
	for i := 0; i < 150; i++ {
		h.recordLatency(time.Millisecond)
	}

	if len(h.latencies) != 100 {
		t.Errorf("latencies length = %d, want 100 (max)", len(h.latencies))
	}
}

func TestNewHealthTracker(t *testing.T) {
	checker := &NoOpHealthChecker{}

	tracker := NewHealthTracker(nil, checker)
	if tracker == nil {
		t.Fatal("NewHealthTracker returned nil")
	}
	defer tracker.Stop()

	if tracker.config == nil {
		t.Error("config is nil")
	}
	if tracker.checker != checker {
		t.Error("checker not set correctly")
	}
}

func TestHealthTracker_AddRemoveEndpoint(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	endpoint := &Endpoint{
		Scheme: SchemeNATS,
		Host:   "localhost",
		Port:   4222,
	}

	// Add endpoint
	tracker.AddEndpoint(endpoint)

	health := tracker.GetHealth(endpoint)
	if health == nil {
		t.Fatal("GetHealth returned nil after AddEndpoint")
	}
	if health.Status != HealthStatusUnknown {
		t.Errorf("initial status = %v, want unknown", health.Status)
	}

	// Remove endpoint
	tracker.RemoveEndpoint(endpoint)

	health = tracker.GetHealth(endpoint)
	if health != nil {
		t.Error("GetHealth should return nil after RemoveEndpoint")
	}
}

func TestHealthTracker_GetAllHealth(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	// Add multiple endpoints
	for i := 0; i < 3; i++ {
		tracker.AddEndpoint(&Endpoint{
			Scheme: SchemeNATS,
			Host:   "localhost",
			Port:   4222 + i,
		})
	}

	health := tracker.GetAllHealth()
	if len(health) != 3 {
		t.Errorf("GetAllHealth returned %d, want 3", len(health))
	}
}

func TestHealthTracker_GetHealthyEndpoints(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	// Add endpoints with different statuses
	ep1 := &Endpoint{Scheme: SchemeNATS, Host: "host1", Port: 4222}
	ep2 := &Endpoint{Scheme: SchemeNATS, Host: "host2", Port: 4222}
	ep3 := &Endpoint{Scheme: SchemeNATS, Host: "host3", Port: 4222}

	tracker.AddEndpoint(ep1)
	tracker.AddEndpoint(ep2)
	tracker.AddEndpoint(ep3)

	// Set different statuses
	tracker.mu.Lock()
	tracker.endpoints[ep1.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep2.Address()].Status = HealthStatusDegraded
	tracker.endpoints[ep3.Address()].Status = HealthStatusUnhealthy
	tracker.mu.Unlock()

	healthy := tracker.GetHealthyEndpoints()
	if len(healthy) != 2 {
		t.Errorf("GetHealthyEndpoints returned %d, want 2", len(healthy))
	}
}

func TestHealthTracker_StartStop(t *testing.T) {
	checker := &NoOpHealthChecker{}
	config := &HealthConfig{
		CheckInterval: 10 * time.Millisecond,
		Timeout:       5 * time.Millisecond,
	}
	tracker := NewHealthTracker(config, checker)

	endpoint := &Endpoint{Scheme: SchemeNATS, Host: "localhost", Port: 4222}
	tracker.AddEndpoint(endpoint)

	// Start tracking
	tracker.Start()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		health := tracker.GetHealth(endpoint)
		return health != nil && health.Status != HealthStatusUnknown, nil
	}); err != nil {
		t.Fatalf("expected at least one check: %v", err)
	}

	// Stop should work
	tracker.Stop()

	// Double stop should be safe
	tracker.Stop()
}

func TestHealthTracker_CheckNow(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	endpoint := &Endpoint{Scheme: SchemeNATS, Host: "localhost", Port: 4222}
	tracker.AddEndpoint(endpoint)

	result := tracker.CheckNow(endpoint)
	if result == nil {
		t.Fatal("CheckNow returned nil")
	}
	if result.Status != HealthStatusHealthy {
		t.Errorf("CheckNow status = %v, want healthy", result.Status)
	}
	if result.Endpoint != endpoint {
		t.Error("CheckNow endpoint mismatch")
	}
}

func TestRoutingStrategy_String(t *testing.T) {
	tests := []struct {
		strategy RoutingStrategy
		want     string
	}{
		{RoutingStrategyPriority, "priority"},
		{RoutingStrategyRoundRobin, "round-robin"},
		{RoutingStrategyLeastLatency, "least-latency"},
		{RoutingStrategyWeighted, "weighted"},
		{RoutingStrategyRandom, "random"},
		{RoutingStrategy(99), "unknown"},
	}

	for _, tt := range tests {
		if got := tt.strategy.String(); got != tt.want {
			t.Errorf("RoutingStrategy(%d).String() = %s, want %s", tt.strategy, got, tt.want)
		}
	}
}

func TestNewHealthBasedRouter(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	router := NewHealthBasedRouter(tracker, RoutingStrategyPriority)
	if router == nil {
		t.Fatal("NewHealthBasedRouter returned nil")
	}
	if router.tracker != tracker {
		t.Error("tracker not set correctly")
	}
	if router.strategy != RoutingStrategyPriority {
		t.Errorf("strategy = %v, want priority", router.strategy)
	}
}

func TestHealthBasedRouter_SelectEndpoint_NoEndpoints(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	router := NewHealthBasedRouter(tracker, RoutingStrategyPriority)

	endpoint := router.SelectEndpoint()
	if endpoint != nil {
		t.Error("SelectEndpoint should return nil when no endpoints")
	}
}

func TestHealthBasedRouter_SelectEndpoint_Priority(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	// Add endpoints with different priorities
	ep1 := &Endpoint{Scheme: SchemeNATS, Host: "host1", Port: 4222, Priority: 2}
	ep2 := &Endpoint{Scheme: SchemeNATS, Host: "host2", Port: 4222, Priority: 0}
	ep3 := &Endpoint{Scheme: SchemeNATS, Host: "host3", Port: 4222, Priority: 1}

	tracker.AddEndpoint(ep1)
	tracker.AddEndpoint(ep2)
	tracker.AddEndpoint(ep3)

	// Mark all as healthy
	tracker.mu.Lock()
	tracker.endpoints[ep1.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep2.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep3.Address()].Status = HealthStatusHealthy
	tracker.mu.Unlock()

	router := NewHealthBasedRouter(tracker, RoutingStrategyPriority)

	selected := router.SelectEndpoint()
	if selected == nil {
		t.Fatal("SelectEndpoint returned nil")
	}
	if selected.Priority != 0 {
		t.Errorf("selected endpoint priority = %d, want 0", selected.Priority)
	}
}

func TestHealthBasedRouter_SelectEndpoint_LeastLatency(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	ep1 := &Endpoint{Scheme: SchemeNATS, Host: "host1", Port: 4222}
	ep2 := &Endpoint{Scheme: SchemeNATS, Host: "host2", Port: 4222}
	ep3 := &Endpoint{Scheme: SchemeNATS, Host: "host3", Port: 4222}

	tracker.AddEndpoint(ep1)
	tracker.AddEndpoint(ep2)
	tracker.AddEndpoint(ep3)

	// Mark all as healthy with different latencies
	tracker.mu.Lock()
	tracker.endpoints[ep1.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep1.Address()].Latency = 100 * time.Millisecond
	tracker.endpoints[ep2.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep2.Address()].Latency = 10 * time.Millisecond // Lowest
	tracker.endpoints[ep3.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep3.Address()].Latency = 50 * time.Millisecond
	tracker.mu.Unlock()

	router := NewHealthBasedRouter(tracker, RoutingStrategyLeastLatency)

	selected := router.SelectEndpoint()
	if selected == nil {
		t.Fatal("SelectEndpoint returned nil")
	}
	if selected.Host != "host2" {
		t.Errorf("selected endpoint host = %s, want host2 (lowest latency)", selected.Host)
	}
}

func TestHealthBasedRouter_SelectEndpoint_Weighted(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	ep1 := &Endpoint{Scheme: SchemeNATS, Host: "host1", Port: 4222, Weight: 1}
	ep2 := &Endpoint{Scheme: SchemeNATS, Host: "host2", Port: 4222, Weight: 10} // Highest weight
	ep3 := &Endpoint{Scheme: SchemeNATS, Host: "host3", Port: 4222, Weight: 5}

	tracker.AddEndpoint(ep1)
	tracker.AddEndpoint(ep2)
	tracker.AddEndpoint(ep3)

	// Mark all as healthy
	tracker.mu.Lock()
	tracker.endpoints[ep1.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep1.Address()].SuccessCount = 10
	tracker.endpoints[ep2.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep2.Address()].SuccessCount = 10
	tracker.endpoints[ep3.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep3.Address()].SuccessCount = 10
	tracker.mu.Unlock()

	router := NewHealthBasedRouter(tracker, RoutingStrategyWeighted)

	selected := router.SelectEndpoint()
	if selected == nil {
		t.Fatal("SelectEndpoint returned nil")
	}
	if selected.Host != "host2" {
		t.Errorf("selected endpoint host = %s, want host2 (highest weight)", selected.Host)
	}
}

func TestHealthBasedRouter_SelectEndpoints(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	ep1 := &Endpoint{Scheme: SchemeNATS, Host: "host1", Port: 4222, Priority: 2}
	ep2 := &Endpoint{Scheme: SchemeNATS, Host: "host2", Port: 4222, Priority: 0}
	ep3 := &Endpoint{Scheme: SchemeNATS, Host: "host3", Port: 4222, Priority: 1}

	tracker.AddEndpoint(ep1)
	tracker.AddEndpoint(ep2)
	tracker.AddEndpoint(ep3)

	tracker.mu.Lock()
	tracker.endpoints[ep1.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep2.Address()].Status = HealthStatusHealthy
	tracker.endpoints[ep3.Address()].Status = HealthStatusHealthy
	tracker.mu.Unlock()

	router := NewHealthBasedRouter(tracker, RoutingStrategyPriority)

	endpoints := router.SelectEndpoints()
	if len(endpoints) != 3 {
		t.Fatalf("SelectEndpoints returned %d, want 3", len(endpoints))
	}

	// Should be sorted by priority
	if endpoints[0].Priority != 0 {
		t.Errorf("first endpoint priority = %d, want 0", endpoints[0].Priority)
	}
	if endpoints[1].Priority != 1 {
		t.Errorf("second endpoint priority = %d, want 1", endpoints[1].Priority)
	}
	if endpoints[2].Priority != 2 {
		t.Errorf("third endpoint priority = %d, want 2", endpoints[2].Priority)
	}
}

func TestHealthBasedRouter_SetGetStrategy(t *testing.T) {
	checker := &NoOpHealthChecker{}
	tracker := NewHealthTracker(nil, checker)
	defer tracker.Stop()

	router := NewHealthBasedRouter(tracker, RoutingStrategyPriority)

	if router.GetStrategy() != RoutingStrategyPriority {
		t.Errorf("initial strategy = %v, want priority", router.GetStrategy())
	}

	router.SetStrategy(RoutingStrategyLeastLatency)

	if router.GetStrategy() != RoutingStrategyLeastLatency {
		t.Errorf("updated strategy = %v, want least-latency", router.GetStrategy())
	}
}

func TestNoOpHealthChecker(t *testing.T) {
	checker := &NoOpHealthChecker{}

	if checker.Name() != "noop" {
		t.Errorf("Name() = %s, want noop", checker.Name())
	}

	endpoint := &Endpoint{Scheme: SchemeNATS, Host: "localhost", Port: 4222}
	result := checker.Check(context.Background(), endpoint)

	if result.Status != HealthStatusHealthy {
		t.Errorf("Status = %v, want healthy", result.Status)
	}
	if result.Endpoint != endpoint {
		t.Error("Endpoint mismatch")
	}
}

func TestHealthTracker_checkEndpoint(t *testing.T) {
	checker := &NoOpHealthChecker{}
	config := &HealthConfig{
		HealthyThreshold:         2,
		UnhealthyThreshold:       2,
		Timeout:                  time.Second,
		DegradedLatencyThreshold: 100 * time.Millisecond, // NoOpChecker returns 1ms
	}
	tracker := NewHealthTracker(config, checker)
	defer tracker.Stop()

	endpoint := &Endpoint{Scheme: SchemeNATS, Host: "localhost", Port: 4222}
	tracker.AddEndpoint(endpoint)

	// First check - should not become healthy (need 2)
	tracker.checkEndpoint(endpoint)
	health := tracker.GetHealth(endpoint)
	if health.Status == HealthStatusHealthy {
		t.Error("Should not be healthy after 1 check")
	}
	if health.ConsecutiveSuccess != 1 {
		t.Errorf("ConsecutiveSuccess = %d, want 1", health.ConsecutiveSuccess)
	}

	// Second check - should become healthy
	tracker.checkEndpoint(endpoint)
	health = tracker.GetHealth(endpoint)
	if health.Status != HealthStatusHealthy {
		t.Error("Should be healthy after 2 checks")
	}
	if health.ConsecutiveSuccess != 2 {
		t.Errorf("ConsecutiveSuccess = %d, want 2", health.ConsecutiveSuccess)
	}
}
