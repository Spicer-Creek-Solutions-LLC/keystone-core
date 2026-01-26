package dr

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestDefaultSchedulerConfig(t *testing.T) {
	config := DefaultSchedulerConfig()

	if config.CheckInterval <= 0 {
		t.Error("CheckInterval should be positive")
	}
	if config.MaxConcurrent <= 0 {
		t.Error("MaxConcurrent should be positive")
	}
	if config.DefaultTimeout <= 0 {
		t.Error("DefaultTimeout should be positive")
	}
}

func TestScheduler_AddSchedule(t *testing.T) {
	scheduler := NewScheduler(nil)

	schedule := &Schedule{
		Type:      DrillTypeFailover,
		Frequency: time.Hour,
		Enabled:   true,
	}

	err := scheduler.AddSchedule("test", schedule)
	if err != nil {
		t.Fatalf("AddSchedule failed: %v", err)
	}

	schedules := scheduler.ListSchedules()
	if len(schedules) != 1 {
		t.Errorf("Expected 1 schedule, got %d", len(schedules))
	}
}

func TestScheduler_AddSchedule_Invalid(t *testing.T) {
	scheduler := NewScheduler(nil)

	schedule := &Schedule{
		Type:    DrillTypeFailover,
		Enabled: true,
		// No Frequency or Cron
	}

	err := scheduler.AddSchedule("test", schedule)
	if err != ErrInvalidSchedule {
		t.Errorf("Expected ErrInvalidSchedule, got %v", err)
	}
}

func TestScheduler_RemoveSchedule(t *testing.T) {
	scheduler := NewScheduler(nil)

	schedule := &Schedule{
		Type:      DrillTypeFailover,
		Frequency: time.Hour,
		Enabled:   true,
	}

	scheduler.AddSchedule("test", schedule)
	scheduler.RemoveSchedule("test")

	schedules := scheduler.ListSchedules()
	if len(schedules) != 0 {
		t.Errorf("Expected 0 schedules, got %d", len(schedules))
	}
}

func TestScheduler_EnableSchedule(t *testing.T) {
	scheduler := NewScheduler(nil)

	schedule := &Schedule{
		Type:      DrillTypeFailover,
		Frequency: time.Hour,
		Enabled:   false,
	}

	scheduler.AddSchedule("test", schedule)
	scheduler.EnableSchedule("test", true)

	schedules := scheduler.ListSchedules()
	if !schedules["test"].Enabled {
		t.Error("Schedule should be enabled")
	}
}

func TestScheduler_RegisterExecutor(t *testing.T) {
	scheduler := NewScheduler(nil)

	executor := &MockExecutor{}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	// Executor should be registered (tested via ExecuteDrill)
	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second,
	}

	drill, err := scheduler.ExecuteDrill(context.Background(), config)
	if err != nil {
		t.Fatalf("ExecuteDrill failed: %v", err)
	}

	if drill.Status != StatusCompleted {
		t.Errorf("Drill status = %v, want %v", drill.Status, StatusCompleted)
	}
}

func TestScheduler_ExecuteDrill(t *testing.T) {
	scheduler := NewScheduler(nil)

	var executeCalled bool
	executor := &MockExecutor{
		ExecuteFunc: func(ctx context.Context, config *DrillConfig) (*DrillResults, error) {
			executeCalled = true
			return &DrillResults{
				Success:      true,
				RecoveryTime: time.Second * 5,
				Steps: []*DrillStep{
					{Name: "step1", Status: StatusCompleted},
				},
			}, nil
		},
	}

	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second * 10,
		Targets: []string{"service-a", "service-b"},
	}

	drill, err := scheduler.ExecuteDrill(context.Background(), config)
	if err != nil {
		t.Fatalf("ExecuteDrill failed: %v", err)
	}

	if !executeCalled {
		t.Error("Executor was not called")
	}

	if drill.Status != StatusCompleted {
		t.Errorf("Drill status = %v, want %v", drill.Status, StatusCompleted)
	}

	if drill.Results == nil || !drill.Results.Success {
		t.Error("Drill results should show success")
	}
}

func TestScheduler_ExecuteDrill_Failure(t *testing.T) {
	scheduler := NewScheduler(nil)

	expectedErr := errors.New("drill failed")
	executor := &MockExecutor{
		ExecuteFunc: func(ctx context.Context, config *DrillConfig) (*DrillResults, error) {
			return nil, expectedErr
		},
	}

	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second,
	}

	drill, err := scheduler.ExecuteDrill(context.Background(), config)
	if err != expectedErr {
		t.Errorf("Expected error %v, got %v", expectedErr, err)
	}

	if drill.Status != StatusFailed {
		t.Errorf("Drill status = %v, want %v", drill.Status, StatusFailed)
	}
}

func TestScheduler_ExecuteDrill_NoExecutor(t *testing.T) {
	scheduler := NewScheduler(nil)

	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second,
	}

	_, err := scheduler.ExecuteDrill(context.Background(), config)
	if err == nil {
		t.Error("Expected error for missing executor")
	}
}

func TestScheduler_ExecuteDrill_ValidationFailure(t *testing.T) {
	scheduler := NewScheduler(nil)

	validationErr := errors.New("validation failed")
	executor := &MockExecutor{
		ValidateFunc: func(config *DrillConfig) error {
			return validationErr
		},
	}

	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second,
	}

	_, err := scheduler.ExecuteDrill(context.Background(), config)
	if err == nil || !errors.Is(err, validationErr) {
		t.Errorf("Expected validation error, got %v", err)
	}
}

func TestScheduler_ExecuteDrill_Callbacks(t *testing.T) {
	scheduler := NewScheduler(nil)

	executor := &MockExecutor{}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	var startCalled, completeCalled bool
	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second,
		OnStart: func(drill *Drill) {
			startCalled = true
		},
		OnComplete: func(drill *Drill) {
			completeCalled = true
		},
	}

	scheduler.ExecuteDrill(context.Background(), config)

	if !startCalled {
		t.Error("OnStart callback not called")
	}
	if !completeCalled {
		t.Error("OnComplete callback not called")
	}
}

func TestScheduler_ExecuteDrill_FailCallback(t *testing.T) {
	scheduler := NewScheduler(nil)

	executor := &MockExecutor{
		ExecuteFunc: func(ctx context.Context, config *DrillConfig) (*DrillResults, error) {
			return nil, errors.New("failed")
		},
	}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	var failCalled bool
	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second,
		OnFail: func(drill *Drill, err error) {
			failCalled = true
		},
	}

	scheduler.ExecuteDrill(context.Background(), config)

	if !failCalled {
		t.Error("OnFail callback not called")
	}
}

func TestScheduler_CancelDrill(t *testing.T) {
	scheduler := NewScheduler(nil)

	startedCh := make(chan struct{})
	blockCh := make(chan struct{})

	executor := &MockExecutor{}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Minute,
	}

	executor.ExecuteFunc = func(ctx context.Context, config *DrillConfig) (*DrillResults, error) {
		select {
		case <-startedCh:
		default:
			close(startedCh)
		}
		<-blockCh
		return &DrillResults{Success: true}, nil
	}

	// Start drill in background
	go scheduler.ExecuteDrill(context.Background(), config)

	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		select {
		case <-startedCh:
			return true, nil
		default:
			return false, nil
		}
	}); err != nil {
		t.Fatalf("expected drill to start: %v", err)
	}

	// Cancel
	err := scheduler.CancelDrill("test-drill")
	if err != nil {
		t.Fatalf("CancelDrill failed: %v", err)
	}
	close(blockCh)

	drill := scheduler.GetDrill("test-drill")
	drill.mu.Lock()
	status := drill.Status
	drill.mu.Unlock()
	if status != StatusCancelled {
		t.Errorf("Drill status = %v, want %v", status, StatusCancelled)
	}
}

func TestScheduler_CancelDrill_NotFound(t *testing.T) {
	scheduler := NewScheduler(nil)

	err := scheduler.CancelDrill("nonexistent")
	if err != ErrDrillNotFound {
		t.Errorf("Expected ErrDrillNotFound, got %v", err)
	}
}

func TestScheduler_GetDrill(t *testing.T) {
	scheduler := NewScheduler(nil)

	executor := &MockExecutor{}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second,
	}

	scheduler.ExecuteDrill(context.Background(), config)

	drill := scheduler.GetDrill("test-drill")
	if drill == nil {
		t.Fatal("GetDrill returned nil")
	}

	if drill.ID != "test-drill" {
		t.Errorf("Drill ID = %s, want test-drill", drill.ID)
	}
}

func TestScheduler_ListDrills(t *testing.T) {
	scheduler := NewScheduler(nil)

	executor := &MockExecutor{}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	for i := 0; i < 3; i++ {
		config := &DrillConfig{
			ID:      "drill-" + string(rune('a'+i)),
			Type:    DrillTypeFailover,
			Timeout: time.Second,
		}
		scheduler.ExecuteDrill(context.Background(), config)
	}

	drills := scheduler.ListDrills()
	if len(drills) != 3 {
		t.Errorf("Expected 3 drills, got %d", len(drills))
	}
}

func TestScheduler_Events(t *testing.T) {
	scheduler := NewScheduler(nil)

	executor := &MockExecutor{}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	var events []*DrillEvent
	var mu sync.Mutex

	scheduler.AddListener(func(event *DrillEvent) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second,
	}

	scheduler.ExecuteDrill(context.Background(), config)

	mu.Lock()
	defer mu.Unlock()

	if len(events) < 2 {
		t.Errorf("Expected at least 2 events, got %d", len(events))
	}

	hasStarted := false
	hasCompleted := false
	for _, e := range events {
		if e.Type == "drill_started" {
			hasStarted = true
		}
		if e.Type == "drill_completed" {
			hasCompleted = true
		}
	}

	if !hasStarted {
		t.Error("Expected drill_started event")
	}
	if !hasCompleted {
		t.Error("Expected drill_completed event")
	}
}

func TestScheduler_Stats(t *testing.T) {
	scheduler := NewScheduler(nil)

	schedule := &Schedule{
		Type:      DrillTypeFailover,
		Frequency: time.Hour,
		Enabled:   true,
	}
	scheduler.AddSchedule("test", schedule)

	executor := &MockExecutor{}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	config := &DrillConfig{
		ID:      "test-drill",
		Type:    DrillTypeFailover,
		Timeout: time.Second,
	}
	scheduler.ExecuteDrill(context.Background(), config)

	stats := scheduler.Stats()

	if stats.TotalSchedules != 1 {
		t.Errorf("TotalSchedules = %d, want 1", stats.TotalSchedules)
	}
	if stats.TotalDrills != 1 {
		t.Errorf("TotalDrills = %d, want 1", stats.TotalDrills)
	}
	if stats.ByStatus[StatusCompleted] != 1 {
		t.Errorf("Completed = %d, want 1", stats.ByStatus[StatusCompleted])
	}
	if stats.ByType[DrillTypeFailover] != 1 {
		t.Errorf("Failover type = %d, want 1", stats.ByType[DrillTypeFailover])
	}
}

func TestScheduler_TimeWindow(t *testing.T) {
	scheduler := NewScheduler(nil)

	t.Run("in window", func(t *testing.T) {
		window := &TimeWindow{
			DaysOfWeek: []int{0, 1, 2, 3, 4, 5, 6}, // All days
			StartHour:  0,
			EndHour:    24,
		}

		if !scheduler.isInWindow(window, time.Now()) {
			t.Error("Should be in window")
		}
	})

	t.Run("out of window by day", func(t *testing.T) {
		now := time.Now()
		today := int(now.Weekday())

		// Create window that excludes today
		excludedDays := make([]int, 0)
		for d := 0; d < 7; d++ {
			if d != today {
				excludedDays = append(excludedDays, d)
			}
		}

		window := &TimeWindow{
			DaysOfWeek: excludedDays,
			StartHour:  0,
			EndHour:    24,
		}

		if scheduler.isInWindow(window, now) {
			t.Error("Should be out of window")
		}
	})

	t.Run("nil window", func(t *testing.T) {
		if !scheduler.isInWindow(nil, time.Now()) {
			t.Error("Nil window should allow all times")
		}
	})
}

func TestScheduler_StartStop(t *testing.T) {
	config := DefaultSchedulerConfig()
	config.CheckInterval = 50 * time.Millisecond

	scheduler := NewScheduler(config)

	executor := &MockExecutor{}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	schedule := &Schedule{
		Type:      DrillTypeFailover,
		Frequency: 10 * time.Millisecond,
		Enabled:   true,
	}
	scheduler.AddSchedule("test", schedule)

	scheduler.Start()
	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		return len(scheduler.ListDrills()) > 0, nil
	}); err != nil {
		t.Fatalf("expected drill to be triggered: %v", err)
	}
	scheduler.Stop()

	// Should have triggered at least one drill
	drills := scheduler.ListDrills()
	if len(drills) == 0 {
		t.Error("Expected at least one drill to be triggered")
	}
}

func TestScheduler_MaxConcurrent(t *testing.T) {
	config := DefaultSchedulerConfig()
	config.MaxConcurrent = 1
	config.CheckInterval = 20 * time.Millisecond

	scheduler := NewScheduler(config)

	var runningCount int64
	var maxRunning int64
	blockCh := make(chan struct{})

	executor := &MockExecutor{
		ExecuteFunc: func(ctx context.Context, config *DrillConfig) (*DrillResults, error) {
			current := atomic.AddInt64(&runningCount, 1)
			defer atomic.AddInt64(&runningCount, -1)

			for {
				max := atomic.LoadInt64(&maxRunning)
				if current > max {
					if atomic.CompareAndSwapInt64(&maxRunning, max, current) {
						break
					}
				} else {
					break
				}
			}

			<-blockCh
			return &DrillResults{Success: true}, nil
		},
	}
	scheduler.RegisterExecutor(DrillTypeFailover, executor)

	// Add multiple schedules
	for i := 0; i < 3; i++ {
		scheduler.AddSchedule("test"+string(rune('0'+i)), &Schedule{
			Type:      DrillTypeFailover,
			Frequency: 10 * time.Millisecond,
			Enabled:   true,
		})
	}

	scheduler.Start()
	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt64(&maxRunning) > 0, nil
	}); err != nil {
		t.Fatalf("expected drill execution to start: %v", err)
	}
	close(blockCh)
	scheduler.Stop()

	if atomic.LoadInt64(&maxRunning) > 1 {
		t.Errorf("Max concurrent = %d, want 1", atomic.LoadInt64(&maxRunning))
	}
}

func TestDrillResults(t *testing.T) {
	results := &DrillResults{
		Success:      true,
		RecoveryTime: time.Second * 30,
		Steps: []*DrillStep{
			{
				Name:      "failover",
				Status:    StatusCompleted,
				StartTime: time.Now(),
				EndTime:   time.Now().Add(time.Second * 10),
				Duration:  time.Second * 10,
			},
			{
				Name:      "validation",
				Status:    StatusCompleted,
				StartTime: time.Now(),
				EndTime:   time.Now().Add(time.Second * 20),
				Duration:  time.Second * 20,
			},
		},
		Metrics: map[string]float64{
			"failover_time_seconds":   10.0,
			"validation_time_seconds": 20.0,
		},
		Issues: []*Issue{
			{
				Severity:    "warning",
				Component:   "database",
				Description: "Slow query during failover",
				Timestamp:   time.Now(),
			},
		},
		DataLoss:       false,
		DataLossAmount: 0,
	}

	if !results.Success {
		t.Error("Results should show success")
	}
	if len(results.Steps) != 2 {
		t.Errorf("Expected 2 steps, got %d", len(results.Steps))
	}
	if len(results.Issues) != 1 {
		t.Errorf("Expected 1 issue, got %d", len(results.Issues))
	}
}

func TestMockExecutor(t *testing.T) {
	executor := &MockExecutor{}

	// Test default behavior
	err := executor.Validate(&DrillConfig{})
	if err != nil {
		t.Errorf("Default Validate should return nil: %v", err)
	}

	results, err := executor.Execute(context.Background(), &DrillConfig{})
	if err != nil {
		t.Errorf("Default Execute should return nil error: %v", err)
	}
	if !results.Success {
		t.Error("Default Execute should return successful results")
	}

	err = executor.Cleanup(context.Background(), &Drill{})
	if err != nil {
		t.Errorf("Default Cleanup should return nil: %v", err)
	}
}
