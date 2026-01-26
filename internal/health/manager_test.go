package health

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestNewManager(t *testing.T) {
	m := NewManager(nil)
	if m == nil {
		t.Fatal("NewManager returned nil")
	}

	if m.config == nil {
		t.Error("Manager config should not be nil")
	}

	if len(m.checkers) != 0 {
		t.Error("Manager should start with no checkers")
	}
}

func TestManagerRegisterChecker(t *testing.T) {
	m := NewManager(nil)
	checker := NewAlwaysHealthyChecker("test")

	m.RegisterChecker(checker)

	m.mu.RLock()
	if len(m.checkers) != 1 {
		t.Errorf("Expected 1 checker, got %d", len(m.checkers))
	}

	if _, exists := m.checkers["test"]; !exists {
		t.Error("Checker not registered with correct name")
	}
	m.mu.RUnlock()
}

func TestManagerUnregisterChecker(t *testing.T) {
	m := NewManager(nil)
	checker := NewAlwaysHealthyChecker("test")

	m.RegisterChecker(checker)
	m.UnregisterChecker("test")

	m.mu.RLock()
	if len(m.checkers) != 0 {
		t.Errorf("Expected 0 checkers after unregister, got %d", len(m.checkers))
	}
	m.mu.RUnlock()
}

func TestManagerLiveness(t *testing.T) {
	m := NewManager(nil)
	response := m.Liveness()

	if response.Status != StatusHealthy {
		t.Errorf("Expected liveness status to be %s, got %s", StatusHealthy, response.Status)
	}
}

func TestManagerReadiness(t *testing.T) {
	config := &Config{
		CheckInterval:      1 * time.Second,
		CheckTimeout:       1 * time.Second,
		ReadinessChecks:    []string{"test"},
		StartupGracePeriod: 0, // No grace period for testing
	}

	m := NewManager(config)
	checker := NewAlwaysHealthyChecker("test")
	m.RegisterChecker(checker)

	// Run checks
	ctx := context.Background()
	m.runAllChecks(ctx)

	// Set ready manually since we have no grace period
	m.SetReady(true)

	response := m.Readiness()

	if response.Status != StatusHealthy {
		t.Errorf("Expected readiness status to be %s, got %s", StatusHealthy, response.Status)
	}

	if len(response.Checks) != 1 {
		t.Errorf("Expected 1 check result, got %d", len(response.Checks))
	}
}

func TestManagerReadinessUnhealthy(t *testing.T) {
	config := &Config{
		CheckInterval:      1 * time.Second,
		CheckTimeout:       1 * time.Second,
		ReadinessChecks:    []string{"test"},
		StartupGracePeriod: 0,
	}

	m := NewManager(config)

	// Register unhealthy checker
	unhealthyChecker := NewFunctionChecker("test", func(ctx context.Context) CheckResult {
		return CheckResult{
			Status:    StatusUnhealthy,
			Message:   "Test unhealthy",
			Timestamp: time.Now(),
		}
	})
	m.RegisterChecker(unhealthyChecker)

	// Run checks
	ctx := context.Background()
	m.runAllChecks(ctx)

	response := m.Readiness()

	if response.Status != StatusUnhealthy {
		t.Errorf("Expected readiness status to be %s, got %s", StatusUnhealthy, response.Status)
	}
}

func TestManagerStatus(t *testing.T) {
	m := NewManager(nil)
	checker := NewAlwaysHealthyChecker("test")
	m.RegisterChecker(checker)

	ctx := context.Background()
	m.runAllChecks(ctx)

	response := m.Status()

	if response.Status != StatusHealthy {
		t.Errorf("Expected status to be %s, got %s", StatusHealthy, response.Status)
	}

	if response.Version == "" {
		t.Error("Expected version to be set")
	}

	if response.Uptime == "" {
		t.Error("Expected uptime to be set")
	}

	if len(response.Components) != 1 {
		t.Errorf("Expected 1 component, got %d", len(response.Components))
	}
}

func TestManagerGetCheckResult(t *testing.T) {
	m := NewManager(nil)
	checker := NewAlwaysHealthyChecker("test")
	m.RegisterChecker(checker)

	ctx := context.Background()
	m.runAllChecks(ctx)

	result, exists := m.GetCheckResult("test")
	if !exists {
		t.Error("Expected check result to exist")
	}

	if result.Status != StatusHealthy {
		t.Errorf("Expected result status to be %s, got %s", StatusHealthy, result.Status)
	}
}

func TestManagerStartStop(t *testing.T) {
	config := &Config{
		CheckInterval:      100 * time.Millisecond,
		CheckTimeout:       1 * time.Second,
		ReadinessChecks:    []string{},
		StartupGracePeriod: 0,
	}

	m := NewManager(config)
	checker := NewAlwaysHealthyChecker("test")
	m.RegisterChecker(checker)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	m.Start(ctx)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		_, exists := m.GetCheckResult("test")
		return exists, nil
	}); err != nil {
		t.Fatalf("Expected at least one check to have run: %v", err)
	}

	m.Stop()

	// Verify check ran
	_, exists := m.GetCheckResult("test")
	if !exists {
		t.Error("Expected at least one check to have run")
	}
}
