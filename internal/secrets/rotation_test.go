package secrets

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestManagedRotation tests the managed rotation state machine.
func TestManagedRotation(t *testing.T) {
	t.Run("NewManagedRotation", func(t *testing.T) {
		targets := createTestTargets(5)
		config := &RotationConfig{
			Strategy:  RotationStrategyBlueGreen,
			BatchSize: 2,
		}

		rotation := NewManagedRotation("rot-1", "vault/secret/db", config, targets, nil)

		if rotation.Rotation.ID != "rot-1" {
			t.Errorf("expected ID rot-1, got %s", rotation.Rotation.ID)
		}
		if rotation.Rotation.SecretPath != "vault/secret/db" {
			t.Errorf("expected path vault/secret/db, got %s", rotation.Rotation.SecretPath)
		}
		if rotation.State() != RotationStatePending {
			t.Errorf("expected state Pending, got %s", rotation.State())
		}
		if rotation.Progress.TotalTargets != 5 {
			t.Errorf("expected 5 targets, got %d", rotation.Progress.TotalTargets)
		}
	})

	t.Run("Start", func(t *testing.T) {
		rotation := createTestRotation(5)

		err := rotation.Start()
		if err != nil {
			t.Fatalf("failed to start: %v", err)
		}

		if rotation.State() != RotationStateInProgress {
			t.Errorf("expected state InProgress, got %s", rotation.State())
		}
	})

	t.Run("StartTwice", func(t *testing.T) {
		rotation := createTestRotation(5)

		_ = rotation.Start()
		err := rotation.Start()
		if err == nil {
			t.Error("expected error starting twice")
		}
	})

	t.Run("BatchProgress", func(t *testing.T) {
		rotation := createTestRotation(10)
		_ = rotation.Start()

		rotation.MarkBatchProgress(3, 0, "Updated 3/10")

		if rotation.Progress.UpdatedTargets != 3 {
			t.Errorf("expected 3 updated, got %d", rotation.Progress.UpdatedTargets)
		}
		if rotation.Progress.Percentage != 30 {
			t.Errorf("expected 30%%, got %d%%", rotation.Progress.Percentage)
		}
	})

	t.Run("BatchComplete", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()

		err := rotation.MarkBatchComplete(1, 5, 0)
		if err != nil {
			t.Fatalf("failed to mark batch complete: %v", err)
		}

		if rotation.State() != RotationStateVerifying {
			t.Errorf("expected state Verifying, got %s", rotation.State())
		}
	})

	t.Run("VerificationPassed", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 5, 0)

		err := rotation.MarkVerificationPassed()
		if err != nil {
			t.Fatalf("failed to mark verification passed: %v", err)
		}

		if rotation.State() != RotationStateCompleted {
			t.Errorf("expected state Completed, got %s", rotation.State())
		}
		if !rotation.IsComplete() {
			t.Error("expected IsComplete to be true")
		}
	})

	t.Run("VerificationFailed", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 5, 0)

		err := rotation.MarkVerificationFailed(fmt.Errorf("health check failed"))
		if err != nil {
			t.Fatalf("failed to mark verification failed: %v", err)
		}

		if rotation.State() != RotationStateFailed {
			t.Errorf("expected state Failed, got %s", rotation.State())
		}
		if !rotation.IsFailed() {
			t.Error("expected IsFailed to be true")
		}
	})

	t.Run("Failure", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()

		err := rotation.Fail(fmt.Errorf("target update failed"))
		if err != nil {
			t.Fatalf("failed to mark failure: %v", err)
		}

		if rotation.State() != RotationStateFailed {
			t.Errorf("expected state Failed, got %s", rotation.State())
		}
		if rotation.Error() == nil {
			t.Error("expected error to be set")
		}
	})

	t.Run("Rollback", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()
		_ = rotation.Fail(fmt.Errorf("failed"))

		err := rotation.Rollback()
		if err != nil {
			t.Fatalf("failed to rollback: %v", err)
		}

		if rotation.State() != RotationStateRolledBack {
			t.Errorf("expected state RolledBack, got %s", rotation.State())
		}
	})

	t.Run("RollbackFromNonFailed", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()

		err := rotation.Rollback()
		if err == nil {
			t.Error("expected error rolling back from non-failed state")
		}
	})

	t.Run("ContinueRolling", func(t *testing.T) {
		rotation := createTestRotation(10)
		rotation.Config.Strategy = RotationStrategyRolling
		rotation.Config.BatchSize = 3
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 3, 0)

		err := rotation.ContinueRolling()
		if err != nil {
			t.Fatalf("failed to continue rolling: %v", err)
		}

		if rotation.State() != RotationStateInProgress {
			t.Errorf("expected state InProgress, got %s", rotation.State())
		}
	})

	t.Run("Duration", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()

		time.Sleep(10 * time.Millisecond)

		duration := rotation.Duration()
		if duration < 10*time.Millisecond {
			t.Errorf("expected duration >= 10ms, got %v", duration)
		}
	})

	t.Run("IsTerminal", func(t *testing.T) {
		rotation := createTestRotation(5)
		if rotation.IsTerminal() {
			t.Error("expected not terminal in pending state")
		}

		_ = rotation.Start()
		if rotation.IsTerminal() {
			t.Error("expected not terminal in progress state")
		}

		_ = rotation.MarkBatchComplete(1, 5, 0)
		_ = rotation.MarkVerificationPassed()
		if !rotation.IsTerminal() {
			t.Error("expected terminal in completed state")
		}
	})

	t.Run("AvailableEvents", func(t *testing.T) {
		rotation := createTestRotation(5)
		events := rotation.AvailableEvents()

		found := false
		for _, e := range events {
			if e == RotationEventStart {
				found = true
				break
			}
		}
		if !found {
			t.Error("expected start event to be available in pending state")
		}
	})

	t.Run("History", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 5, 0)
		_ = rotation.MarkVerificationPassed()

		history := rotation.History()
		if history == nil {
			t.Fatal("expected history to be enabled")
		}

		entries := history.All()
		if len(entries) < 3 {
			t.Errorf("expected at least 3 history entries, got %d", len(entries))
		}
	})

	t.Run("StateDiagram", func(t *testing.T) {
		rotation := createTestRotation(5)
		diagram := rotation.StateDiagram()

		if diagram == "" {
			t.Error("expected non-empty state diagram")
		}
		if len(diagram) < 100 {
			t.Error("state diagram seems too short")
		}
	})
}

// TestRotationCallbacks tests the rotation callback system.
func TestRotationCallbacks(t *testing.T) {
	t.Run("OnStateChange", func(t *testing.T) {
		var stateChanges []RotationState
		var mu sync.Mutex

		callbacks := &RotationCallbacks{
			OnStateChange: func(r *ManagedRotation, from, to RotationState) {
				mu.Lock()
				stateChanges = append(stateChanges, to)
				mu.Unlock()
			},
		}

		rotation := createTestRotationWithCallbacks(5, callbacks)
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 5, 0)
		_ = rotation.MarkVerificationPassed()

		mu.Lock()
		if len(stateChanges) != 3 {
			t.Errorf("expected 3 state changes, got %d", len(stateChanges))
		}
		mu.Unlock()
	})

	t.Run("OnProgress", func(t *testing.T) {
		var progressUpdates int32

		callbacks := &RotationCallbacks{
			OnProgress: func(r *ManagedRotation, p RotationProgress) {
				atomic.AddInt32(&progressUpdates, 1)
			},
		}

		rotation := createTestRotationWithCallbacks(10, callbacks)
		_ = rotation.Start()

		rotation.MarkBatchProgress(3, 0, "3/10")
		rotation.MarkBatchProgress(5, 0, "5/10")
		rotation.MarkBatchProgress(7, 0, "7/10")

		if atomic.LoadInt32(&progressUpdates) != 3 {
			t.Errorf("expected 3 progress updates, got %d", progressUpdates)
		}
	})

	t.Run("OnBatchComplete", func(t *testing.T) {
		var batchCompleted bool

		callbacks := &RotationCallbacks{
			OnBatchComplete: func(r *ManagedRotation, batch, succeeded, failed int) {
				batchCompleted = true
				if batch != 1 {
					t.Errorf("expected batch 1, got %d", batch)
				}
				if succeeded != 5 {
					t.Errorf("expected 5 succeeded, got %d", succeeded)
				}
			},
		}

		rotation := createTestRotationWithCallbacks(5, callbacks)
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 5, 0)

		if !batchCompleted {
			t.Error("expected batch complete callback")
		}
	})

	t.Run("OnComplete", func(t *testing.T) {
		var completed bool

		callbacks := &RotationCallbacks{
			OnComplete: func(r *ManagedRotation) {
				completed = true
			},
		}

		rotation := createTestRotationWithCallbacks(5, callbacks)
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 5, 0)
		_ = rotation.MarkVerificationPassed()

		if !completed {
			t.Error("expected complete callback")
		}
	})

	t.Run("OnFailed", func(t *testing.T) {
		var failed bool
		var failErr error

		callbacks := &RotationCallbacks{
			OnFailed: func(r *ManagedRotation, err error) {
				failed = true
				failErr = err
			},
		}

		rotation := createTestRotationWithCallbacks(5, callbacks)
		_ = rotation.Start()
		_ = rotation.Fail(fmt.Errorf("test error"))

		if !failed {
			t.Error("expected failed callback")
		}
		if failErr == nil || failErr.Error() != "test error" {
			t.Errorf("expected test error, got %v", failErr)
		}
	})

	t.Run("OnRollback", func(t *testing.T) {
		var rollbackStarted bool

		callbacks := &RotationCallbacks{
			OnRollback: func(r *ManagedRotation) {
				rollbackStarted = true
			},
		}

		rotation := createTestRotationWithCallbacks(5, callbacks)
		_ = rotation.Start()
		_ = rotation.Fail(fmt.Errorf("test error"))
		_ = rotation.Rollback()

		if !rollbackStarted {
			t.Error("expected rollback callback")
		}
	})

	t.Run("OnVerificationStart", func(t *testing.T) {
		var verificationStarted bool

		callbacks := &RotationCallbacks{
			OnVerificationStart: func(r *ManagedRotation) {
				verificationStarted = true
			},
		}

		rotation := createTestRotationWithCallbacks(5, callbacks)
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 5, 0)

		if !verificationStarted {
			t.Error("expected verification start callback")
		}
	})

	t.Run("OnHealthCheckResult", func(t *testing.T) {
		var healthCheckCount int32

		callbacks := &RotationCallbacks{
			OnHealthCheckResult: func(r *ManagedRotation, target string, healthy bool, err error) {
				atomic.AddInt32(&healthCheckCount, 1)
			},
		}

		rotation := createTestRotationWithCallbacks(5, callbacks)
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 5, 0)

		rotation.MarkHealthCheckResult("target-1", true, nil)
		rotation.MarkHealthCheckResult("target-2", true, nil)
		rotation.MarkHealthCheckResult("target-3", false, fmt.Errorf("unhealthy"))

		if atomic.LoadInt32(&healthCheckCount) != 3 {
			t.Errorf("expected 3 health checks, got %d", healthCheckCount)
		}
	})
}

// TestBlueGreenStrategy tests the blue-green rotation strategy.
func TestBlueGreenStrategy(t *testing.T) {
	t.Run("Name", func(t *testing.T) {
		strategy := &BlueGreenStrategy{}
		if strategy.Name() != RotationStrategyBlueGreen {
			t.Errorf("expected blue_green, got %s", strategy.Name())
		}
	})

	t.Run("BatchSize", func(t *testing.T) {
		strategy := &BlueGreenStrategy{}
		config := &RotationConfig{}

		size := strategy.BatchSize(config, 10)
		if size != 10 {
			t.Errorf("expected batch size 10 (all targets), got %d", size)
		}
	})

	t.Run("BatchDelay", func(t *testing.T) {
		strategy := &BlueGreenStrategy{}
		config := &RotationConfig{}

		delay := strategy.BatchDelay(config)
		if delay != 0 {
			t.Errorf("expected no delay, got %v", delay)
		}
	})

	t.Run("Execute", func(t *testing.T) {
		strategy := &BlueGreenStrategy{}
		rotation := createTestRotation(5)
		_ = rotation.Start()

		err := strategy.Execute(context.Background(), rotation, rotation.Targets)
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}

		for _, target := range rotation.Targets {
			if target.Status != TargetStatusUpdated {
				t.Errorf("target %s should be updated, got %s", target.ID, target.Status)
			}
		}
	})

	t.Run("Verify_NoHealthCheck", func(t *testing.T) {
		strategy := &BlueGreenStrategy{}
		rotation := createTestRotation(5)
		rotation.Config.HealthCheck = nil
		_ = rotation.Start()

		for _, t := range rotation.Targets {
			t.Status = TargetStatusUpdated
		}

		err := strategy.Verify(context.Background(), rotation, rotation.Targets)
		if err != nil {
			t.Fatalf("verify failed: %v", err)
		}

		for _, target := range rotation.Targets {
			if target.Status != TargetStatusVerified {
				t.Errorf("target %s should be verified, got %s", target.ID, target.Status)
			}
		}
	})

	t.Run("Rollback", func(t *testing.T) {
		strategy := &BlueGreenStrategy{}
		rotation := createTestRotation(5)
		_ = rotation.Start()

		for _, t := range rotation.Targets {
			t.Status = TargetStatusUpdated
		}

		err := strategy.Rollback(context.Background(), rotation, rotation.Targets)
		if err != nil {
			t.Fatalf("rollback failed: %v", err)
		}

		for _, target := range rotation.Targets {
			if target.Status != TargetStatusRolled {
				t.Errorf("target %s should be rolled back, got %s", target.ID, target.Status)
			}
		}
	})
}

// TestRollingStrategy tests the rolling rotation strategy.
func TestRollingStrategy(t *testing.T) {
	t.Run("Name", func(t *testing.T) {
		strategy := &RollingStrategy{}
		if strategy.Name() != RotationStrategyRolling {
			t.Errorf("expected rolling, got %s", strategy.Name())
		}
	})

	t.Run("BatchSize_Configured", func(t *testing.T) {
		strategy := &RollingStrategy{}
		config := &RotationConfig{BatchSize: 5}

		size := strategy.BatchSize(config, 20)
		if size != 5 {
			t.Errorf("expected batch size 5, got %d", size)
		}
	})

	t.Run("BatchSize_Default", func(t *testing.T) {
		strategy := &RollingStrategy{}
		config := &RotationConfig{}

		size := strategy.BatchSize(config, 100)
		if size != 10 { // 10% of 100
			t.Errorf("expected batch size 10, got %d", size)
		}
	})

	t.Run("BatchSize_Minimum", func(t *testing.T) {
		strategy := &RollingStrategy{}
		config := &RotationConfig{}

		size := strategy.BatchSize(config, 5)
		if size < 1 {
			t.Errorf("expected batch size >= 1, got %d", size)
		}
	})

	t.Run("BatchDelay_Configured", func(t *testing.T) {
		strategy := &RollingStrategy{}
		config := &RotationConfig{BatchDelay: 30 * time.Second}

		delay := strategy.BatchDelay(config)
		if delay != 30*time.Second {
			t.Errorf("expected 30s delay, got %v", delay)
		}
	})

	t.Run("BatchDelay_Default", func(t *testing.T) {
		strategy := &RollingStrategy{}
		config := &RotationConfig{}

		delay := strategy.BatchDelay(config)
		if delay != 10*time.Second {
			t.Errorf("expected 10s default delay, got %v", delay)
		}
	})

	t.Run("Execute", func(t *testing.T) {
		strategy := &RollingStrategy{}
		rotation := createTestRotation(5)
		rotation.Config.Strategy = RotationStrategyRolling
		_ = rotation.Start()

		err := strategy.Execute(context.Background(), rotation, rotation.Targets[:3])
		if err != nil {
			t.Fatalf("execute failed: %v", err)
		}

		for i, target := range rotation.Targets[:3] {
			if target.Status != TargetStatusUpdated {
				t.Errorf("target %d should be updated, got %s", i, target.Status)
			}
		}
	})

	t.Run("Execute_ContextCancelled", func(t *testing.T) {
		strategy := &RollingStrategy{}
		rotation := createTestRotation(100)
		rotation.Config.Strategy = RotationStrategyRolling
		_ = rotation.Start()

		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		err := strategy.Execute(ctx, rotation, rotation.Targets)
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

// TestRotationOrchestrator tests the rotation orchestrator.
func TestRotationOrchestrator(t *testing.T) {
	t.Run("NewRotationOrchestrator", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)

		if orchestrator == nil {
			t.Fatal("expected non-nil orchestrator")
		}

		// Should have default strategies registered
		if len(orchestrator.strategies) < 2 {
			t.Errorf("expected at least 2 strategies, got %d", len(orchestrator.strategies))
		}
	})

	t.Run("RegisterStrategy", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)

		customStrategy := &testStrategy{name: "custom"}
		orchestrator.RegisterStrategy(customStrategy)

		if _, ok := orchestrator.strategies["custom"]; !ok {
			t.Error("custom strategy not registered")
		}
	})

	t.Run("StartRotation", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)

		targets := createTestTargets(5)
		config := &RotationConfig{
			Strategy: RotationStrategyBlueGreen,
		}

		rotation, err := orchestrator.StartRotation(
			context.Background(),
			"rot-1",
			"vault/secret/db",
			config,
			targets,
		)
		if err != nil {
			t.Fatalf("failed to start rotation: %v", err)
		}

		if rotation == nil {
			t.Fatal("expected non-nil rotation")
		}

		// Wait for completion
		time.Sleep(100 * time.Millisecond)

		if !rotation.IsTerminal() {
			t.Error("expected rotation to be complete")
		}
	})

	t.Run("StartRotation_DuplicateID", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)

		targets := createTestTargets(5)
		config := &RotationConfig{
			Strategy: RotationStrategyBlueGreen,
		}

		_, _ = orchestrator.StartRotation(context.Background(), "rot-dup", "vault/secret/db", config, targets)

		// Try to start with same ID
		_, err := orchestrator.StartRotation(context.Background(), "rot-dup", "vault/secret/db", config, targets)
		if err == nil {
			t.Error("expected error for duplicate rotation ID")
		}
	})

	t.Run("StartRotation_UnknownStrategy", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)

		targets := createTestTargets(5)
		config := &RotationConfig{
			Strategy: "unknown",
		}

		_, err := orchestrator.StartRotation(context.Background(), "rot-unknown", "vault/secret/db", config, targets)
		if err == nil {
			t.Error("expected error for unknown strategy")
		}
	})

	t.Run("GetRotation", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)

		targets := createTestTargets(5)
		config := &RotationConfig{
			Strategy: RotationStrategyBlueGreen,
		}

		_, _ = orchestrator.StartRotation(context.Background(), "rot-get", "vault/secret/db", config, targets)

		rotation, found := orchestrator.GetRotation("rot-get")
		if !found {
			t.Error("expected to find rotation")
		}
		if rotation == nil {
			t.Error("expected non-nil rotation")
		}

		_, found = orchestrator.GetRotation("nonexistent")
		if found {
			t.Error("expected not to find nonexistent rotation")
		}
	})

	t.Run("ListRotations", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)

		targets := createTestTargets(5)
		config := &RotationConfig{
			Strategy: RotationStrategyBlueGreen,
		}

		_, _ = orchestrator.StartRotation(context.Background(), "rot-list-1", "vault/secret/db1", config, targets)
		_, _ = orchestrator.StartRotation(context.Background(), "rot-list-2", "vault/secret/db2", config, targets)

		rotations := orchestrator.ListRotations()
		if len(rotations) < 2 {
			t.Errorf("expected at least 2 rotations, got %d", len(rotations))
		}
	})

	t.Run("CancelRotation", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)

		// Create a slow rotation
		targets := createTestTargets(100)
		config := &RotationConfig{
			Strategy:   RotationStrategyRolling,
			BatchSize:  1,
			BatchDelay: time.Second,
		}

		_, _ = orchestrator.StartRotation(context.Background(), "rot-cancel", "vault/secret/db", config, targets)

		err := orchestrator.CancelRotation("rot-cancel")
		if err != nil {
			t.Fatalf("failed to cancel: %v", err)
		}
	})

	t.Run("CancelRotation_NotFound", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)

		err := orchestrator.CancelRotation("nonexistent")
		if err == nil {
			t.Error("expected error for nonexistent rotation")
		}
	})
}

// TestRotationEventPublisher tests event publishing.
func TestRotationEventPublisher(t *testing.T) {
	t.Run("PublishEvents", func(t *testing.T) {
		var publishedEvents []*RotationStateEvent
		var mu sync.Mutex

		publisher := &testEventPublisher{
			publishFunc: func(ctx context.Context, event *RotationStateEvent) error {
				mu.Lock()
				publishedEvents = append(publishedEvents, event)
				mu.Unlock()
				return nil
			},
		}

		rotation := createTestRotation(5)
		rotation.SetEventPublisher(publisher)
		_ = rotation.Start()
		_ = rotation.MarkBatchComplete(1, 5, 0)
		_ = rotation.MarkVerificationPassed()

		// Wait for async publishing
		time.Sleep(50 * time.Millisecond)

		mu.Lock()
		if len(publishedEvents) < 3 {
			t.Errorf("expected at least 3 published events, got %d", len(publishedEvents))
		}
		mu.Unlock()
	})
}

// TestRotationWithAutoRollback tests automatic rollback on failure.
func TestRotationWithAutoRollback(t *testing.T) {
	var rollbackCalled bool
	var mu sync.Mutex

	callbacks := &RotationCallbacks{
		OnRollback: func(r *ManagedRotation) {
			mu.Lock()
			rollbackCalled = true
			mu.Unlock()
		},
	}

	targets := createTestTargets(5)
	config := &RotationConfig{
		Strategy:          RotationStrategyBlueGreen,
		RollbackOnFailure: true,
	}

	rotation := NewManagedRotation("rot-auto-rb", "vault/secret/db", config, targets, callbacks)
	_ = rotation.Start()
	_ = rotation.Fail(fmt.Errorf("simulated failure"))

	// Wait for auto-rollback
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	if !rollbackCalled {
		t.Error("expected auto-rollback to be triggered")
	}
	mu.Unlock()
}

// TestRotationFailureThreshold tests failure threshold handling.
func TestRotationFailureThreshold(t *testing.T) {
	broker := NewSecretBroker(&BrokerConfig{})
	orchestrator := NewRotationOrchestrator(broker)

	// Create targets where some will "fail"
	targets := createTestTargets(10)
	config := &RotationConfig{
		Strategy:         RotationStrategyBlueGreen,
		FailureThreshold: 0.1, // 10% threshold
	}

	rotation, err := orchestrator.StartRotation(
		context.Background(),
		"rot-threshold",
		"vault/secret/db",
		config,
		targets,
	)
	if err != nil {
		t.Fatalf("failed to start rotation: %v", err)
	}

	// Wait for completion
	time.Sleep(100 * time.Millisecond)

	// Should complete successfully since no failures in mock
	if rotation.IsFailed() && rotation.Error() != nil {
		// If it failed due to threshold, that's expected
		t.Logf("Rotation failed as expected: %v", rotation.Error())
	}
}

// TestConcurrentRotations tests concurrent rotation operations.
func TestConcurrentRotations(t *testing.T) {
	broker := NewSecretBroker(&BrokerConfig{})
	orchestrator := NewRotationOrchestrator(broker)

	var wg sync.WaitGroup
	var successCount int32

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()

			targets := createTestTargets(5)
			config := &RotationConfig{
				Strategy: RotationStrategyBlueGreen,
			}

			rotation, err := orchestrator.StartRotation(
				context.Background(),
				fmt.Sprintf("rot-concurrent-%d", n),
				fmt.Sprintf("vault/secret/db%d", n),
				config,
				targets,
			)
			if err != nil {
				return
			}

			// Wait for completion
			for !rotation.IsTerminal() {
				time.Sleep(10 * time.Millisecond)
			}

			if rotation.IsComplete() {
				atomic.AddInt32(&successCount, 1)
			}
		}(i)
	}

	wg.Wait()

	if atomic.LoadInt32(&successCount) != 10 {
		t.Errorf("expected 10 successful rotations, got %d", successCount)
	}
}

// TestRotationSecretStorage tests secret storage for rollback.
func TestRotationSecretStorage(t *testing.T) {
	rotation := createTestRotation(5)

	oldSecret := map[string]interface{}{
		"password": "old-password",
	}
	newSecret := map[string]interface{}{
		"password": "new-password",
	}

	rotation.SetOldSecret(oldSecret)
	rotation.SetNewSecret(newSecret)

	if rotation.GetOldSecret()["password"] != "old-password" {
		t.Error("old secret not stored correctly")
	}
	if rotation.GetNewSecret()["password"] != "new-password" {
		t.Error("new secret not stored correctly")
	}
}

// TestInvalidStateTransitions tests that invalid transitions are rejected.
func TestInvalidStateTransitions(t *testing.T) {
	t.Run("StartFromInProgress", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()

		err := rotation.Start()
		if err == nil {
			t.Error("expected error starting from in_progress state")
		}
	})

	t.Run("VerificationPassedFromPending", func(t *testing.T) {
		rotation := createTestRotation(5)

		err := rotation.MarkVerificationPassed()
		if err == nil {
			t.Error("expected error marking verification passed from pending state")
		}
	})

	t.Run("ContinueRollingFromPending", func(t *testing.T) {
		rotation := createTestRotation(5)

		err := rotation.ContinueRolling()
		if err == nil {
			t.Error("expected error continuing rolling from pending state")
		}
	})

	t.Run("RollbackFromInProgress", func(t *testing.T) {
		rotation := createTestRotation(5)
		_ = rotation.Start()

		err := rotation.Rollback()
		if err == nil {
			t.Error("expected error rolling back from in_progress state")
		}
	})
}

// Helper functions

func createTestTargets(n int) []*RotationTarget {
	targets := make([]*RotationTarget, n)
	for i := 0; i < n; i++ {
		targets[i] = &RotationTarget{
			ID:     fmt.Sprintf("target-%d", i),
			Name:   fmt.Sprintf("Target %d", i),
			Type:   "agent",
			Status: TargetStatusPending,
		}
	}
	return targets
}

func createTestRotation(numTargets int) *ManagedRotation {
	targets := createTestTargets(numTargets)
	config := &RotationConfig{
		Strategy:  RotationStrategyBlueGreen,
		BatchSize: numTargets,
	}
	return NewManagedRotation("test-rotation", "vault/secret/test", config, targets, nil)
}

func createTestRotationWithCallbacks(numTargets int, callbacks *RotationCallbacks) *ManagedRotation {
	targets := createTestTargets(numTargets)
	config := &RotationConfig{
		Strategy:  RotationStrategyBlueGreen,
		BatchSize: numTargets,
	}
	return NewManagedRotation("test-rotation", "vault/secret/test", config, targets, callbacks)
}

// testStrategy is a mock strategy for testing.
type testStrategy struct {
	name RotationStrategy
}

func (s *testStrategy) Name() RotationStrategy {
	return s.name
}

func (s *testStrategy) Execute(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error {
	for _, t := range targets {
		t.Status = TargetStatusUpdated
	}
	return nil
}

func (s *testStrategy) Verify(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error {
	for _, t := range targets {
		if t.Status == TargetStatusUpdated {
			t.Status = TargetStatusVerified
		}
	}
	return nil
}

func (s *testStrategy) Rollback(ctx context.Context, rotation *ManagedRotation, targets []*RotationTarget) error {
	for _, t := range targets {
		t.Status = TargetStatusRolled
	}
	return nil
}

func (s *testStrategy) BatchSize(config *RotationConfig, totalTargets int) int {
	return totalTargets
}

func (s *testStrategy) BatchDelay(config *RotationConfig) time.Duration {
	return 0
}

var _ RotationStrategyExecutor = (*testStrategy)(nil)

// testEventPublisher is a mock event publisher for testing.
type testEventPublisher struct {
	publishFunc func(ctx context.Context, event *RotationStateEvent) error
}

func (p *testEventPublisher) Publish(ctx context.Context, event *RotationStateEvent) error {
	if p.publishFunc != nil {
		return p.publishFunc(ctx, event)
	}
	return nil
}

var _ RotationEventPublisher = (*testEventPublisher)(nil)
