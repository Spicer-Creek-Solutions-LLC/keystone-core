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
