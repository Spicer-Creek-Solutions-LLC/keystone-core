package health

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

// countingChecker counts how many times Check is called.
type countingChecker struct {
	name  string
	count atomic.Int32
}

func (c *countingChecker) Name() string { return c.name }
func (c *countingChecker) Check(ctx context.Context) CheckResult {
	c.count.Add(1)
	return CheckResult{
		Status:    StatusHealthy,
		Message:   "ok",
		Timestamp: time.Now(),
	}
}

// panickingChecker panics when Check is called.
type panickingChecker struct{ name string }

func (c *panickingChecker) Name() string                        { return c.name }
func (c *panickingChecker) Check(ctx context.Context) CheckResult { panic("checker exploded") }

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

func TestManagerRunAllChecks_NoDoubleInvocation(t *testing.T) {
	m := NewManager(&Config{
		CheckInterval: 1 * time.Hour,
		CheckTimeout:  5 * time.Second,
	})

	c1 := &countingChecker{name: "checker-a"}
	c2 := &countingChecker{name: "checker-b"}

	m.RegisterChecker(c1)
	m.RegisterChecker(c2)

	m.runAllChecks(context.Background())

	if got := c1.count.Load(); got != 1 {
		t.Errorf("checker-a invoked %d times, want 1", got)
	}
	if got := c2.count.Load(); got != 1 {
		t.Errorf("checker-b invoked %d times, want 1", got)
	}

	// Verify results are stored under the correct names
	r1, ok1 := m.GetCheckResult("checker-a")
	r2, ok2 := m.GetCheckResult("checker-b")
	if !ok1 || !ok2 {
		t.Fatalf("results not found: checker-a=%v, checker-b=%v", ok1, ok2)
	}
	if r1.Status != StatusHealthy || r2.Status != StatusHealthy {
		t.Error("expected both results to be healthy")
	}
}

func TestManagerRunAllChecks_PanicRecovery(t *testing.T) {
	m := NewManager(&Config{
		CheckInterval: 1 * time.Hour,
		CheckTimeout:  5 * time.Second,
	})

	m.RegisterChecker(&panickingChecker{name: "boom"})
	m.RegisterChecker(NewAlwaysHealthyChecker("stable"))

	// Should not panic
	m.runAllChecks(context.Background())

	// Panicking checker should produce an unhealthy result
	r, ok := m.GetCheckResult("boom")
	if !ok {
		t.Fatal("expected result for panicking checker")
	}
	if r.Status != StatusUnhealthy {
		t.Errorf("expected unhealthy status, got %v", r.Status)
	}

	// Healthy checker should still have run
	r2, ok2 := m.GetCheckResult("stable")
	if !ok2 {
		t.Fatal("expected result for stable checker")
	}
	if r2.Status != StatusHealthy {
		t.Errorf("expected healthy status, got %v", r2.Status)
	}
}
