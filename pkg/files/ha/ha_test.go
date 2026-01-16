package ha

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

// mockHealthChecker is a mock health checker for testing.
type mockHealthChecker struct {
	healthy bool
	err     error
}

func (m *mockHealthChecker) Check(ctx context.Context) error {
	if !m.healthy {
		return m.err
	}
	return nil
}

func TestInstanceManager_NewInstanceManager(t *testing.T) {
	config := &InstanceManagerConfig{
		ID:       "test-1",
		Hostname: "localhost",
		Address:  "localhost:8080",
	}

	manager := NewInstanceManager(config)

	if manager.info.ID != "test-1" {
		t.Errorf("expected ID 'test-1', got '%s'", manager.info.ID)
	}

	if manager.info.Hostname != "localhost" {
		t.Errorf("expected hostname 'localhost', got '%s'", manager.info.Hostname)
	}

	if manager.info.State != InstanceStateStarting {
		t.Errorf("expected state Starting, got '%s'", manager.info.State)
	}
}

func TestInstanceManager_StartStop(t *testing.T) {
	config := &InstanceManagerConfig{
		ID:             "test-1",
		Hostname:       "localhost",
		Address:        "localhost:8080",
		HealthInterval: 100 * time.Millisecond,
	}

	manager := NewInstanceManager(config)

	ctx := context.Background()

	// Start the manager.
	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	if manager.info.State != InstanceStateReady {
		t.Errorf("expected state Ready after start, got '%s'", manager.info.State)
	}

	// Stop the manager.
	if err := manager.Stop(ctx); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	if manager.info.State != InstanceStateStopped {
		t.Errorf("expected state Stopped after stop, got '%s'", manager.info.State)
	}
}

func TestInstanceManager_WithHealthChecker(t *testing.T) {
	healthChecker := &mockHealthChecker{healthy: true}

	config := &InstanceManagerConfig{
		ID:             "test-1",
		Hostname:       "localhost",
		Address:        "localhost:8080",
		HealthInterval: 50 * time.Millisecond,
		HealthChecker:  healthChecker,
	}

	manager := NewInstanceManager(config)

	ctx := context.Background()

	if err := manager.Start(ctx); err != nil {
		t.Fatalf("Start() error: %v", err)
	}
	defer manager.Stop(ctx)

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return manager.info.State == InstanceStateReady, nil
	}); err != nil {
		t.Fatalf("expected state Ready with healthy checker: %v", err)
	}

	if manager.info.State != InstanceStateReady {
		t.Errorf("expected state Ready with healthy checker, got '%s'", manager.info.State)
	}

	// Make the health check fail.
	healthChecker.healthy = false
	healthChecker.err = errors.New("unhealthy")

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return manager.info.State == InstanceStateUnhealthy, nil
	}); err != nil {
		t.Fatalf("expected state Unhealthy with failing checker: %v", err)
	}

	if manager.info.State != InstanceStateUnhealthy {
		t.Errorf("expected state Unhealthy with failing checker, got '%s'", manager.info.State)
	}

	// Make the health check pass again.
	healthChecker.healthy = true

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return manager.info.State == InstanceStateReady, nil
	}); err != nil {
		t.Fatalf("expected state Ready after recovery: %v", err)
	}

	if manager.info.State != InstanceStateReady {
		t.Errorf("expected state Ready after recovery, got '%s'", manager.info.State)
	}
}

func TestInstanceManager_MetricsRecording(t *testing.T) {
	config := &InstanceManagerConfig{
		ID:       "test-1",
		Hostname: "localhost",
		Address:  "localhost:8080",
	}

	manager := NewInstanceManager(config)

	// Record some transfers.
	manager.RecordTransfer(1024)
	manager.RecordTransfer(2048)

	if manager.info.Metrics.TransfersTotal != 2 {
		t.Errorf("expected 2 total transfers, got %d", manager.info.Metrics.TransfersTotal)
	}

	if manager.info.Metrics.BytesTransferred != 3072 {
		t.Errorf("expected 3072 bytes transferred, got %d", manager.info.Metrics.BytesTransferred)
	}

	// Record an error.
	manager.RecordError()

	if manager.info.Metrics.ErrorsTotal != 1 {
		t.Errorf("expected 1 error, got %d", manager.info.Metrics.ErrorsTotal)
	}

	// Test active transfers.
	manager.IncrementActiveTransfers()
	manager.IncrementActiveTransfers()

	if manager.info.Metrics.TransfersActive != 2 {
		t.Errorf("expected 2 active transfers, got %d", manager.info.Metrics.TransfersActive)
	}

	manager.DecrementActiveTransfers()

	if manager.info.Metrics.TransfersActive != 1 {
		t.Errorf("expected 1 active transfer, got %d", manager.info.Metrics.TransfersActive)
	}
}

func TestInstanceManager_GetInstances(t *testing.T) {
	config := &InstanceManagerConfig{
		ID:       "test-1",
		Hostname: "localhost",
		Address:  "localhost:8080",
	}

	manager := NewInstanceManager(config)

	// Initially no instances known.
	instances := manager.GetInstances()
	if len(instances) != 0 {
		t.Errorf("expected 0 instances, got %d", len(instances))
	}

	// Simulate receiving health reports.
	manager.mu.Lock()
	manager.instances["inst-1"] = &InstanceInfo{
		ID:            "inst-1",
		State:         InstanceStateReady,
		LastHeartbeat: time.Now(),
		Metrics:       &InstanceMetrics{},
	}
	manager.instances["inst-2"] = &InstanceInfo{
		ID:            "inst-2",
		State:         InstanceStateUnhealthy,
		LastHeartbeat: time.Now(),
		Metrics:       &InstanceMetrics{},
	}
	manager.mu.Unlock()

	instances = manager.GetInstances()
	if len(instances) != 2 {
		t.Errorf("expected 2 instances, got %d", len(instances))
	}

	// Get healthy instances only.
	healthy := manager.GetHealthyInstances()
	if len(healthy) != 1 {
		t.Errorf("expected 1 healthy instance, got %d", len(healthy))
	}
	if healthy[0].ID != "inst-1" {
		t.Errorf("expected healthy instance 'inst-1', got '%s'", healthy[0].ID)
	}
}

func TestHealthHandler_AddCheck(t *testing.T) {
	config := &InstanceManagerConfig{
		ID:       "test-1",
		Hostname: "localhost",
		Address:  "localhost:8080",
	}

	manager := NewInstanceManager(config)
	handler := NewHealthHandler(manager)

	// Add a healthy check.
	handler.AddCheck("test-check", &mockHealthChecker{healthy: true})

	ctx := context.Background()
	result := handler.Check(ctx)

	if !result.Healthy {
		t.Error("expected overall healthy result")
	}

	if len(result.Checks) != 1 {
		t.Errorf("expected 1 check, got %d", len(result.Checks))
	}

	check, ok := result.Checks["test-check"]
	if !ok {
		t.Error("expected 'test-check' in checks")
	}

	if !check.Healthy {
		t.Error("expected test-check to be healthy")
	}
}

func TestHealthHandler_FailingCheck(t *testing.T) {
	config := &InstanceManagerConfig{
		ID:       "test-1",
		Hostname: "localhost",
		Address:  "localhost:8080",
	}

	manager := NewInstanceManager(config)
	handler := NewHealthHandler(manager)

	// Add a failing check.
	handler.AddCheck("failing-check", &mockHealthChecker{
		healthy: false,
		err:     errors.New("check failed"),
	})

	ctx := context.Background()
	result := handler.Check(ctx)

	if result.Healthy {
		t.Error("expected overall unhealthy result")
	}

	check := result.Checks["failing-check"]
	if check.Healthy {
		t.Error("expected failing-check to be unhealthy")
	}

	if check.Message != "check failed" {
		t.Errorf("expected message 'check failed', got '%s'", check.Message)
	}
}

func TestHealthHandler_LivenessReadiness(t *testing.T) {
	config := &InstanceManagerConfig{
		ID:       "test-1",
		Hostname: "localhost",
		Address:  "localhost:8080",
	}

	manager := NewInstanceManager(config)
	handler := NewHealthHandler(manager)

	// Initially starting - not ready but alive.
	if !handler.LivenessCheck() {
		t.Error("expected liveness check to pass when starting")
	}

	if handler.ReadinessCheck() {
		t.Error("expected readiness check to fail when starting")
	}

	// Start the manager.
	ctx := context.Background()
	manager.Start(ctx)
	defer manager.Stop(ctx)

	// Now ready.
	if !handler.LivenessCheck() {
		t.Error("expected liveness check to pass when ready")
	}

	if !handler.ReadinessCheck() {
		t.Error("expected readiness check to pass when ready")
	}

	// Stop the manager.
	manager.Stop(ctx)

	// Stopped - not alive.
	if handler.LivenessCheck() {
		t.Error("expected liveness check to fail when stopped")
	}

	if handler.ReadinessCheck() {
		t.Error("expected readiness check to fail when stopped")
	}
}
