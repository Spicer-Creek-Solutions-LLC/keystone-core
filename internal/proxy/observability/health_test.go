package observability

import (
	"context"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestNewHealthMonitor(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	m := NewHealthMonitor(config)

	if m == nil {
		t.Fatal("NewHealthMonitor returned nil")
	}

	if m.checkers == nil {
		t.Error("checkers slice is nil")
	}

	if m.results == nil {
		t.Error("results map is nil")
	}
}

func TestDefaultHealthMonitorConfig(t *testing.T) {
	config := DefaultHealthMonitorConfig()

	if config.CheckInterval != 30*time.Second {
		t.Errorf("expected CheckInterval 30s, got %v", config.CheckInterval)
	}

	if config.Timeout != 10*time.Second {
		t.Errorf("expected Timeout 10s, got %v", config.Timeout)
	}
}

func TestHealthMonitor_RegisterChecker(t *testing.T) {
	config := DefaultHealthMonitorConfig()
	m := NewHealthMonitor(config)

	checker := NewFunctionHealthChecker("test", func(ctx context.Context) (HealthStatus, string, map[string]string) {
		return HealthStatusHealthy, "OK", nil
	})

	m.RegisterChecker(checker)

	if len(m.checkers) != 1 {
		t.Errorf("expected 1 checker, got %d", len(m.checkers))
	}
}

func TestHealthMonitor_RunChecks(t *testing.T) {
	config := HealthMonitorConfig{
		CheckInterval: 1 * time.Second,
		Timeout:       5 * time.Second,
	}
	m := NewHealthMonitor(config)

	checker := NewFunctionHealthChecker("test", func(ctx context.Context) (HealthStatus, string, map[string]string) {
		return HealthStatusHealthy, "All good", map[string]string{"key": "value"}
	})

	m.RegisterChecker(checker)
	m.RunChecks()

	results := m.GetResults()
	if len(results) != 1 {
		t.Errorf("expected 1 result, got %d", len(results))
	}

	result := results["test"]
	if result == nil {
		t.Fatal("result for 'test' is nil")
	}

	if result.Status != HealthStatusHealthy {
		t.Errorf("expected status healthy, got %s", result.Status)
	}

	if result.Message != "All good" {
		t.Errorf("expected message 'All good', got '%s'", result.Message)
	}
}

func TestHealthMonitor_GetStatus(t *testing.T) {
	config := HealthMonitorConfig{
		CheckInterval: 1 * time.Second,
		Timeout:       5 * time.Second,
	}
	m := NewHealthMonitor(config)

	// No checkers - should be unknown
	status := m.GetStatus()
	if status != HealthStatusUnknown {
		t.Errorf("expected status unknown with no checkers, got %s", status)
	}

	// Add healthy checker
	m.RegisterChecker(NewFunctionHealthChecker("healthy", func(ctx context.Context) (HealthStatus, string, map[string]string) {
		return HealthStatusHealthy, "OK", nil
	}))

	m.RunChecks()
	status = m.GetStatus()
	if status != HealthStatusHealthy {
		t.Errorf("expected status healthy, got %s", status)
	}

	// Add degraded checker
	m.RegisterChecker(NewFunctionHealthChecker("degraded", func(ctx context.Context) (HealthStatus, string, map[string]string) {
		return HealthStatusDegraded, "Warning", nil
	}))

	m.RunChecks()
	status = m.GetStatus()
	if status != HealthStatusDegraded {
		t.Errorf("expected status degraded, got %s", status)
	}

	// Add unhealthy checker
	m.RegisterChecker(NewFunctionHealthChecker("unhealthy", func(ctx context.Context) (HealthStatus, string, map[string]string) {
		return HealthStatusUnhealthy, "Error", nil
	}))

	m.RunChecks()
	status = m.GetStatus()
	if status != HealthStatusUnhealthy {
		t.Errorf("expected status unhealthy, got %s", status)
	}
}

func TestHealthMonitor_StartStop(t *testing.T) {
	config := HealthMonitorConfig{
		CheckInterval: 100 * time.Millisecond,
		Timeout:       1 * time.Second,
	}
	m := NewHealthMonitor(config)

	m.RegisterChecker(NewFunctionHealthChecker("test", func(ctx context.Context) (HealthStatus, string, map[string]string) {
		return HealthStatusHealthy, "OK", nil
	}))

	// Start should succeed
	err := m.Start()
	if err != nil {
		t.Errorf("Start failed: %v", err)
	}

	// Second start should fail
	err = m.Start()
	if err == nil {
		t.Error("expected error on second Start")
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return len(m.GetResults()) > 0, nil
	}); err != nil {
		t.Fatalf("expected results after running checks: %v", err)
	}

	// Stop
	m.Stop()

	// Results should be populated
	results := m.GetResults()
	if len(results) == 0 {
		t.Error("expected results after running checks")
	}
}

func TestHealthMonitor_StatusChangeCallback(t *testing.T) {
	config := HealthMonitorConfig{
		CheckInterval: 100 * time.Millisecond,
		Timeout:       1 * time.Second,
	}
	m := NewHealthMonitor(config)

	callbackCalled := false
	var oldStatus, newStatus HealthStatus

	m.SetStatusChangeCallback(func(old, new HealthStatus) {
		callbackCalled = true
		oldStatus = old
		newStatus = new
	})

	// Initially unknown status
	m.RegisterChecker(NewFunctionHealthChecker("test", func(ctx context.Context) (HealthStatus, string, map[string]string) {
		return HealthStatusHealthy, "OK", nil
	}))

	m.RunChecks()

	if !callbackCalled {
		t.Error("callback should have been called on status change from unknown to healthy")
	}

	if oldStatus != HealthStatusUnknown {
		t.Errorf("expected old status unknown, got %s", oldStatus)
	}

	if newStatus != HealthStatusHealthy {
		t.Errorf("expected new status healthy, got %s", newStatus)
	}
}

func TestHealthStatusConstants(t *testing.T) {
	if HealthStatusHealthy != "healthy" {
		t.Errorf("unexpected HealthStatusHealthy: %s", HealthStatusHealthy)
	}

	if HealthStatusDegraded != "degraded" {
		t.Errorf("unexpected HealthStatusDegraded: %s", HealthStatusDegraded)
	}

	if HealthStatusUnhealthy != "unhealthy" {
		t.Errorf("unexpected HealthStatusUnhealthy: %s", HealthStatusUnhealthy)
	}

	if HealthStatusUnknown != "unknown" {
		t.Errorf("unexpected HealthStatusUnknown: %s", HealthStatusUnknown)
	}
}

func TestNewDeviceHealthChecker(t *testing.T) {
	getDevices := func() []DeviceStatus {
		return []DeviceStatus{
			{ID: "dev1", Healthy: true},
			{ID: "dev2", Healthy: true},
			{ID: "dev3", Healthy: false},
		}
	}

	checker := NewDeviceHealthChecker("devices", getDevices, 1, 50.0)

	if checker == nil {
		t.Fatal("NewDeviceHealthChecker returned nil")
	}

	if checker.Name() != "devices" {
		t.Errorf("expected name 'devices', got '%s'", checker.Name())
	}

	// Run check
	result := checker.Check(context.Background())

	if result.Name != "devices" {
		t.Errorf("expected result name 'devices', got '%s'", result.Name)
	}

	// 2/3 healthy = 66.7%, min is 1 device and 50%
	if result.Status != HealthStatusHealthy {
		t.Errorf("expected status healthy (2/3 healthy), got %s", result.Status)
	}
}

func TestDeviceHealthChecker_NoDevices(t *testing.T) {
	getDevices := func() []DeviceStatus {
		return []DeviceStatus{}
	}

	checker := NewDeviceHealthChecker("devices", getDevices, 1, 50.0)
	result := checker.Check(context.Background())

	if result.Status != HealthStatusUnknown {
		t.Errorf("expected status unknown for no devices, got %s", result.Status)
	}

	if result.Message != "No devices registered" {
		t.Errorf("unexpected message: %s", result.Message)
	}
}

func TestDeviceHealthChecker_AllUnhealthy(t *testing.T) {
	getDevices := func() []DeviceStatus {
		return []DeviceStatus{
			{ID: "dev1", Healthy: false},
			{ID: "dev2", Healthy: false},
		}
	}

	checker := NewDeviceHealthChecker("devices", getDevices, 1, 50.0)
	result := checker.Check(context.Background())

	if result.Status != HealthStatusUnhealthy {
		t.Errorf("expected status unhealthy for all unhealthy devices, got %s", result.Status)
	}
}

func TestNewConnectionHealthChecker(t *testing.T) {
	getStats := func() ConnectionStats {
		return ConnectionStats{
			Total:      100,
			Active:     50,
			Failed:     5,
			AvgLatency: 100 * time.Millisecond,
		}
	}

	checker := NewConnectionHealthChecker("connections", getStats, 10.0, 500.0)

	if checker == nil {
		t.Fatal("NewConnectionHealthChecker returned nil")
	}

	if checker.Name() != "connections" {
		t.Errorf("expected name 'connections', got '%s'", checker.Name())
	}

	result := checker.Check(context.Background())

	// 5% failure rate (5/100), max is 10%, so healthy
	if result.Status != HealthStatusHealthy {
		t.Errorf("expected status healthy (5%% failure rate), got %s", result.Status)
	}
}

func TestConnectionHealthChecker_HighFailureRate(t *testing.T) {
	getStats := func() ConnectionStats {
		return ConnectionStats{
			Total:      100,
			Active:     30,
			Failed:     20,
			AvgLatency: 100 * time.Millisecond,
		}
	}

	checker := NewConnectionHealthChecker("connections", getStats, 10.0, 500.0)
	result := checker.Check(context.Background())

	// 20% failure rate, max is 10%
	if result.Status != HealthStatusUnhealthy {
		t.Errorf("expected status unhealthy (20%% failure rate), got %s", result.Status)
	}
}

func TestConnectionHealthChecker_HighLatency(t *testing.T) {
	getStats := func() ConnectionStats {
		return ConnectionStats{
			Total:      100,
			Active:     50,
			Failed:     2,
			AvgLatency: 600 * time.Millisecond,
		}
	}

	checker := NewConnectionHealthChecker("connections", getStats, 10.0, 500.0)
	result := checker.Check(context.Background())

	// 600ms latency, max is 500ms
	if result.Status != HealthStatusDegraded {
		t.Errorf("expected status degraded (high latency), got %s", result.Status)
	}
}

func TestNewProtocolHealthChecker(t *testing.T) {
	checkFunc := func(ctx context.Context) error {
		return nil
	}

	checker := NewProtocolHealthChecker("ssh-protocol", "ssh", checkFunc)

	if checker == nil {
		t.Fatal("NewProtocolHealthChecker returned nil")
	}

	if checker.Name() != "ssh-protocol" {
		t.Errorf("expected name 'ssh-protocol', got '%s'", checker.Name())
	}

	result := checker.Check(context.Background())

	if result.Status != HealthStatusHealthy {
		t.Errorf("expected status healthy, got %s", result.Status)
	}
}

func TestProtocolHealthChecker_Error(t *testing.T) {
	checkFunc := func(ctx context.Context) error {
		return context.DeadlineExceeded
	}

	checker := NewProtocolHealthChecker("ssh-protocol", "ssh", checkFunc)
	result := checker.Check(context.Background())

	if result.Status != HealthStatusUnhealthy {
		t.Errorf("expected status unhealthy on error, got %s", result.Status)
	}
}

func TestNewFunctionHealthChecker(t *testing.T) {
	checker := NewFunctionHealthChecker("custom", func(ctx context.Context) (HealthStatus, string, map[string]string) {
		return HealthStatusHealthy, "All systems operational", map[string]string{
			"version": "1.0.0",
		}
	})

	if checker == nil {
		t.Fatal("NewFunctionHealthChecker returned nil")
	}

	if checker.Name() != "custom" {
		t.Errorf("expected name 'custom', got '%s'", checker.Name())
	}

	result := checker.Check(context.Background())

	if result.Status != HealthStatusHealthy {
		t.Errorf("expected status healthy, got %s", result.Status)
	}

	if result.Message != "All systems operational" {
		t.Errorf("unexpected message: %s", result.Message)
	}

	if result.Metadata["version"] != "1.0.0" {
		t.Errorf("expected metadata version '1.0.0', got '%s'", result.Metadata["version"])
	}
}

func TestHealthCheck_Duration(t *testing.T) {
	checker := NewFunctionHealthChecker("slow", func(ctx context.Context) (HealthStatus, string, map[string]string) {
		start := time.Now()
		_ = helpers.WaitForTimeout(2*time.Second, 1*time.Millisecond, func() (bool, error) {
			return time.Since(start) >= 50*time.Millisecond, nil
		})
		return HealthStatusHealthy, "Done", nil
	})

	result := checker.Check(context.Background())

	if result.Duration < 50*time.Millisecond {
		t.Errorf("expected duration >= 50ms, got %v", result.Duration)
	}
}

func TestHealthCheckerInterface(t *testing.T) {
	// Verify all checkers implement HealthChecker interface
	var _ HealthChecker = (*DeviceHealthChecker)(nil)
	var _ HealthChecker = (*ConnectionHealthChecker)(nil)
	var _ HealthChecker = (*ProtocolHealthChecker)(nil)
	var _ HealthChecker = (*FunctionHealthChecker)(nil)
}
