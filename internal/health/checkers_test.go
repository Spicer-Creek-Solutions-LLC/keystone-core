package health

import (
	"context"
	"testing"
)

func TestAlwaysHealthyChecker(t *testing.T) {
	checker := NewAlwaysHealthyChecker("test")

	if checker.Name() != "test" {
		t.Errorf("Expected name 'test', got '%s'", checker.Name())
	}

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, result.Status)
	}

	if result.Message != "Always healthy" {
		t.Errorf("Unexpected message: %s", result.Message)
	}
}

func TestFunctionChecker(t *testing.T) {
	called := false
	checker := NewFunctionChecker("test", func(ctx context.Context) CheckResult {
		called = true
		return CheckResult{
			Status:  StatusHealthy,
			Message: "Function called",
		}
	})

	if checker.Name() != "test" {
		t.Errorf("Expected name 'test', got '%s'", checker.Name())
	}

	ctx := context.Background()
	result := checker.Check(ctx)

	if !called {
		t.Error("Function was not called")
	}

	if result.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, result.Status)
	}
}

func TestFunctionCheckerNil(t *testing.T) {
	checker := NewFunctionChecker("test", nil)

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusUnknown {
		t.Errorf("Expected status %s for nil function, got %s", StatusUnknown, result.Status)
	}
}

func TestAgentPoolChecker(t *testing.T) {
	connected := 80
	total := 100

	getConnected := func() int { return connected }
	getTotal := func() int { return total }

	checker := NewAgentPoolChecker(getConnected, getTotal, 0.8)

	if checker.Name() != "agents" {
		t.Errorf("Expected name 'agents', got '%s'", checker.Name())
	}

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, result.Status)
	}

	if result.Details["connected"] != 80 {
		t.Error("Expected connected count in details")
	}

	if result.Details["total"] != 100 {
		t.Error("Expected total count in details")
	}
}

func TestAgentPoolCheckerLowAvailability(t *testing.T) {
	connected := 40
	total := 100

	getConnected := func() int { return connected }
	getTotal := func() int { return total }

	checker := NewAgentPoolChecker(getConnected, getTotal, 0.8)

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusUnhealthy {
		t.Errorf("Expected status %s for low availability, got %s", StatusUnhealthy, result.Status)
	}
}

func TestAgentPoolCheckerDegraded(t *testing.T) {
	connected := 70
	total := 100

	getConnected := func() int { return connected }
	getTotal := func() int { return total }

	checker := NewAgentPoolChecker(getConnected, getTotal, 0.8)

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusDegraded {
		t.Errorf("Expected status %s for degraded availability, got %s", StatusDegraded, result.Status)
	}
}

func TestAgentPoolCheckerNoAgents(t *testing.T) {
	getConnected := func() int { return 0 }
	getTotal := func() int { return 0 }

	checker := NewAgentPoolChecker(getConnected, getTotal, 0.8)

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusHealthy {
		t.Errorf("Expected status %s when no agents registered, got %s", StatusHealthy, result.Status)
	}
}

func TestAgentPoolCheckerNilFunctions(t *testing.T) {
	checker := &AgentPoolChecker{
		name:                "agents",
		getConnectedAgents:  nil,
		getTotalAgents:      nil,
		minHealthyThreshold: 0.8,
	}

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusUnknown {
		t.Errorf("Expected status %s for nil functions, got %s", StatusUnknown, result.Status)
	}
}

func TestNetworkCheckerNoListeners(t *testing.T) {
	checker := NewNetworkChecker()

	if checker.Name() != "network" {
		t.Errorf("Expected name 'network', got '%s'", checker.Name())
	}

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusUnknown {
		t.Errorf("Expected status %s for no listeners, got %s", StatusUnknown, result.Status)
	}
}

func TestNetworkCheckerHealthyIPv4Only(t *testing.T) {
	checker := NewNetworkChecker()
	checker.AddListener(ListenerInfo{
		Address:   "0.0.0.0:8080",
		Protocol:  "grpc",
		IPVersion: "ipv4",
		Active:    true,
	})
	checker.AddListener(ListenerInfo{
		Address:   "0.0.0.0:8081",
		Protocol:  "http",
		IPVersion: "ipv4",
		Active:    true,
	})

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, result.Status)
	}

	if result.Details["dual_stack"] != false {
		t.Error("Expected dual_stack to be false")
	}

	if result.Details["ipv4_listeners"] != 2 {
		t.Errorf("Expected 2 IPv4 listeners, got %v", result.Details["ipv4_listeners"])
	}
}

func TestNetworkCheckerHealthyIPv6Only(t *testing.T) {
	checker := NewNetworkChecker()
	checker.AddListener(ListenerInfo{
		Address:   "[::]:8080",
		Protocol:  "grpc",
		IPVersion: "ipv6",
		Active:    true,
	})

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, result.Status)
	}

	if result.Details["ipv6_listeners"] != 1 {
		t.Errorf("Expected 1 IPv6 listener, got %v", result.Details["ipv6_listeners"])
	}
}

func TestNetworkCheckerHealthyDualStack(t *testing.T) {
	checker := NewNetworkChecker()
	checker.AddListener(ListenerInfo{
		Address:   "0.0.0.0:8080",
		Protocol:  "grpc",
		IPVersion: "ipv4",
		Active:    true,
	})
	checker.AddListener(ListenerInfo{
		Address:   "[::]:8080",
		Protocol:  "grpc",
		IPVersion: "ipv6",
		Active:    true,
	})

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusHealthy {
		t.Errorf("Expected status %s, got %s", StatusHealthy, result.Status)
	}

	if result.Details["dual_stack"] != true {
		t.Error("Expected dual_stack to be true")
	}

	if result.Details["ipv4_listeners"] != 1 {
		t.Errorf("Expected 1 IPv4 listener, got %v", result.Details["ipv4_listeners"])
	}

	if result.Details["ipv6_listeners"] != 1 {
		t.Errorf("Expected 1 IPv6 listener, got %v", result.Details["ipv6_listeners"])
	}
}

func TestNetworkCheckerDegraded(t *testing.T) {
	checker := NewNetworkChecker()
	checker.AddListener(ListenerInfo{
		Address:   "0.0.0.0:8080",
		Protocol:  "grpc",
		IPVersion: "ipv4",
		Active:    true,
	})
	checker.AddListener(ListenerInfo{
		Address:   "[::]:8080",
		Protocol:  "grpc",
		IPVersion: "ipv6",
		Active:    false, // Inactive listener
	})

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusDegraded {
		t.Errorf("Expected status %s, got %s", StatusDegraded, result.Status)
	}

	if result.Details["active"] != 1 {
		t.Errorf("Expected 1 active listener, got %v", result.Details["active"])
	}

	if result.Details["inactive"] != 1 {
		t.Errorf("Expected 1 inactive listener, got %v", result.Details["inactive"])
	}
}

func TestNetworkCheckerUnhealthy(t *testing.T) {
	checker := NewNetworkChecker()
	checker.AddListener(ListenerInfo{
		Address:   "0.0.0.0:8080",
		Protocol:  "grpc",
		IPVersion: "ipv4",
		Active:    false,
	})
	checker.AddListener(ListenerInfo{
		Address:   "[::]:8080",
		Protocol:  "grpc",
		IPVersion: "ipv6",
		Active:    false,
	})

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Status != StatusUnhealthy {
		t.Errorf("Expected status %s, got %s", StatusUnhealthy, result.Status)
	}
}

func TestNetworkCheckerClearListeners(t *testing.T) {
	checker := NewNetworkChecker()
	checker.AddListener(ListenerInfo{
		Address:   "0.0.0.0:8080",
		Protocol:  "grpc",
		IPVersion: "ipv4",
		Active:    true,
	})

	// Verify listener was added
	ctx := context.Background()
	result := checker.Check(ctx)
	if result.Details["total"] != 1 {
		t.Error("Expected 1 listener")
	}

	// Clear and verify
	checker.ClearListeners()
	result = checker.Check(ctx)
	if result.Status != StatusUnknown {
		t.Errorf("Expected status %s after clearing, got %s", StatusUnknown, result.Status)
	}
}

func TestNetworkCheckerProtocolCounts(t *testing.T) {
	checker := NewNetworkChecker()
	checker.AddListener(ListenerInfo{
		Address:   "0.0.0.0:8080",
		Protocol:  "grpc",
		IPVersion: "ipv4",
		Active:    true,
	})
	checker.AddListener(ListenerInfo{
		Address:   "0.0.0.0:8081",
		Protocol:  "http",
		IPVersion: "ipv4",
		Active:    true,
	})
	checker.AddListener(ListenerInfo{
		Address:   "[::]:8081",
		Protocol:  "http",
		IPVersion: "ipv6",
		Active:    true,
	})

	ctx := context.Background()
	result := checker.Check(ctx)

	if result.Details["grpc_listeners"] != 1 {
		t.Errorf("Expected 1 gRPC listener, got %v", result.Details["grpc_listeners"])
	}

	if result.Details["http_listeners"] != 2 {
		t.Errorf("Expected 2 HTTP listeners, got %v", result.Details["http_listeners"])
	}
}
