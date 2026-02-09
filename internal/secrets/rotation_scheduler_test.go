package secrets

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestParseCron(t *testing.T) {
	t.Run("ValidExpressions", func(t *testing.T) {
		tests := []struct {
			name string
			expr string
		}{
			{"every minute", "* * * * *"},
			{"every hour", "0 * * * *"},
			{"daily at midnight", "0 0 * * *"},
			{"daily at 2am", "0 2 * * *"},
			{"weekly on sunday", "0 0 * * 0"},
			{"monthly on first", "0 0 1 * *"},
			{"specific time", "30 14 * * *"},
			{"range", "0-30 * * * *"},
			{"list", "0,15,30,45 * * * *"},
			{"step", "*/5 * * * *"},
			{"complex", "0 2 1,15 * 1-5"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				schedule, err := ParseCron(tt.expr)
				if err != nil {
					t.Errorf("failed to parse %s: %v", tt.expr, err)
				}
				if schedule == nil {
					t.Error("schedule should not be nil")
				}
			})
		}
	})

	t.Run("InvalidExpressions", func(t *testing.T) {
		tests := []struct {
			name string
			expr string
		}{
			{"too few fields", "* * * *"},
			{"too many fields", "* * * * * *"},
			{"invalid minute", "60 * * * *"},
			{"invalid hour", "* 25 * * *"},
			{"invalid day", "* * 32 * *"},
			{"invalid month", "* * * 13 *"},
			{"invalid weekday", "* * * * 7"},
			{"invalid range", "30-20 * * * *"},
			{"invalid step", "*/0 * * * *"},
			{"non-numeric", "abc * * * *"},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, err := ParseCron(tt.expr)
				if err == nil {
					t.Errorf("expected error for %s", tt.expr)
				}
			})
		}
	})
}

func TestCronScheduleMatches(t *testing.T) {
	t.Run("EveryMinute", func(t *testing.T) {
		schedule, _ := ParseCron("* * * * *")

		// Should match any time
		times := []time.Time{
			time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			time.Date(2024, 6, 15, 12, 30, 0, 0, time.UTC),
			time.Date(2024, 12, 31, 23, 59, 0, 0, time.UTC),
		}

		for _, tm := range times {
			if !schedule.Matches(tm) {
				t.Errorf("should match %v", tm)
			}
		}
	})

	t.Run("SpecificTime", func(t *testing.T) {
		schedule, _ := ParseCron("30 14 * * *")

		matching := time.Date(2024, 1, 1, 14, 30, 0, 0, time.UTC)
		if !schedule.Matches(matching) {
			t.Errorf("should match 14:30")
		}

		notMatching := time.Date(2024, 1, 1, 14, 31, 0, 0, time.UTC)
		if schedule.Matches(notMatching) {
			t.Errorf("should not match 14:31")
		}
	})

	t.Run("SpecificDayOfWeek", func(t *testing.T) {
		schedule, _ := ParseCron("0 0 * * 1") // Monday

		// January 1, 2024 is a Monday
		monday := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
		if !schedule.Matches(monday) {
			t.Errorf("should match Monday")
		}

		// January 2, 2024 is a Tuesday
		tuesday := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
		if schedule.Matches(tuesday) {
			t.Errorf("should not match Tuesday")
		}
	})
}

func TestNextCronRun(t *testing.T) {
	t.Run("EveryMinute", func(t *testing.T) {
		now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
		next, err := NextCronRun("* * * * *", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := time.Date(2024, 1, 1, 12, 1, 0, 0, time.UTC)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, next)
		}
	})

	t.Run("EveryHour", func(t *testing.T) {
		now := time.Date(2024, 1, 1, 12, 30, 0, 0, time.UTC)
		next, err := NextCronRun("0 * * * *", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := time.Date(2024, 1, 1, 13, 0, 0, 0, time.UTC)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, next)
		}
	})

	t.Run("Daily", func(t *testing.T) {
		now := time.Date(2024, 1, 1, 3, 0, 0, 0, time.UTC)
		next, err := NextCronRun("0 2 * * *", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := time.Date(2024, 1, 2, 2, 0, 0, 0, time.UTC)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, next)
		}
	})

	t.Run("Every5Minutes", func(t *testing.T) {
		now := time.Date(2024, 1, 1, 12, 7, 0, 0, time.UTC)
		next, err := NextCronRun("*/5 * * * *", now)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		expected := time.Date(2024, 1, 1, 12, 10, 0, 0, time.UTC)
		if !next.Equal(expected) {
			t.Errorf("expected %v, got %v", expected, next)
		}
	})
}

func TestRotationScheduler(t *testing.T) {
	t.Run("AddSchedule", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)
		scheduler := NewRotationScheduler(orchestrator)

		schedule := &ScheduledRotation{
			ID:         "sched-1",
			SecretPath: "vault/secret/db",
			Schedule:   "0 2 * * *",
			Enabled:    true,
			Config:     &RotationConfig{Strategy: RotationStrategyBlueGreen},
			Targets:    createTestTargets(5),
		}

		err := scheduler.AddSchedule(schedule)
		if err != nil {
			t.Fatalf("failed to add schedule: %v", err)
		}

		// Verify it was added
		retrieved, ok := scheduler.GetSchedule("sched-1")
		if !ok {
			t.Error("schedule not found")
		}
		if retrieved.SecretPath != "vault/secret/db" {
			t.Errorf("wrong secret path: %s", retrieved.SecretPath)
		}
		if retrieved.NextRun.IsZero() {
			t.Error("next run should be calculated")
		}
	})

	t.Run("AddSchedule_DuplicateID", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)
		scheduler := NewRotationScheduler(orchestrator)

		schedule := &ScheduledRotation{
			ID:         "sched-1",
			SecretPath: "vault/secret/db",
			Schedule:   "0 2 * * *",
		}

		_ = scheduler.AddSchedule(schedule)
		err := scheduler.AddSchedule(schedule)
		if err == nil {
			t.Error("expected error for duplicate ID")
		}
	})

	t.Run("AddSchedule_InvalidCron", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)
		scheduler := NewRotationScheduler(orchestrator)

		schedule := &ScheduledRotation{
			ID:         "sched-1",
			SecretPath: "vault/secret/db",
			Schedule:   "invalid cron",
		}

		err := scheduler.AddSchedule(schedule)
		if err == nil {
			t.Error("expected error for invalid cron")
		}
	})

	t.Run("RemoveSchedule", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)
		scheduler := NewRotationScheduler(orchestrator)

		schedule := &ScheduledRotation{
			ID:         "sched-1",
			SecretPath: "vault/secret/db",
			Schedule:   "0 2 * * *",
		}

		_ = scheduler.AddSchedule(schedule)
		err := scheduler.RemoveSchedule("sched-1")
		if err != nil {
			t.Fatalf("failed to remove schedule: %v", err)
		}

		_, ok := scheduler.GetSchedule("sched-1")
		if ok {
			t.Error("schedule should be removed")
		}
	})

	t.Run("EnableDisable", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)
		scheduler := NewRotationScheduler(orchestrator)

		schedule := &ScheduledRotation{
			ID:         "sched-1",
			SecretPath: "vault/secret/db",
			Schedule:   "0 2 * * *",
			Enabled:    false,
		}

		_ = scheduler.AddSchedule(schedule)

		err := scheduler.EnableSchedule("sched-1")
		if err != nil {
			t.Fatalf("failed to enable: %v", err)
		}

		s, _ := scheduler.GetSchedule("sched-1")
		if !s.Enabled {
			t.Error("schedule should be enabled")
		}

		err = scheduler.DisableSchedule("sched-1")
		if err != nil {
			t.Fatalf("failed to disable: %v", err)
		}

		s, _ = scheduler.GetSchedule("sched-1")
		if s.Enabled {
			t.Error("schedule should be disabled")
		}
	})

	t.Run("ListSchedules", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)
		scheduler := NewRotationScheduler(orchestrator)

		for i := 1; i <= 3; i++ {
			schedule := &ScheduledRotation{
				ID:         string(rune('a' + i - 1)),
				SecretPath: "vault/secret/db",
				Schedule:   "0 2 * * *",
			}
			_ = scheduler.AddSchedule(schedule)
		}

		schedules := scheduler.ListSchedules()
		if len(schedules) != 3 {
			t.Errorf("expected 3 schedules, got %d", len(schedules))
		}
	})

	t.Run("StartStop", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)
		scheduler := NewRotationScheduler(orchestrator)

		if scheduler.IsRunning() {
			t.Error("should not be running initially")
		}

		err := scheduler.Start()
		if err != nil {
			t.Fatalf("failed to start: %v", err)
		}

		if !scheduler.IsRunning() {
			t.Error("should be running after start")
		}

		// Try to start again
		err = scheduler.Start()
		if err == nil {
			t.Error("expected error when starting already running scheduler")
		}

		scheduler.Stop()
		time.Sleep(10 * time.Millisecond) // Give goroutine time to stop

		if scheduler.IsRunning() {
			t.Error("should not be running after stop")
		}
	})

	t.Run("TriggerNow", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)
		scheduler := NewRotationScheduler(orchestrator)

		schedule := &ScheduledRotation{
			ID:         "sched-1",
			SecretPath: "vault/secret/db",
			Schedule:   "0 2 * * *",
			Enabled:    true,
			Config:     &RotationConfig{Strategy: RotationStrategyBlueGreen},
			Targets:    createTestTargets(3),
		}

		_ = scheduler.AddSchedule(schedule)
		_ = scheduler.Start()
		defer scheduler.Stop()

		err := scheduler.TriggerNow("sched-1")
		if err != nil {
			t.Fatalf("failed to trigger: %v", err)
		}

		// Wait for rotation to complete
		time.Sleep(200 * time.Millisecond)

		s, _ := scheduler.GetSchedule("sched-1")
		if s.LastRun.IsZero() {
			t.Error("last run should be set")
		}
	})

	t.Run("Callbacks", func(t *testing.T) {
		broker := NewSecretBroker(&BrokerConfig{})
		orchestrator := NewRotationOrchestrator(broker)
		scheduler := NewRotationScheduler(orchestrator)

		var startCalled, completeCalled atomic.Bool
		scheduler.SetCallbacks(
			func(s *ScheduledRotation) {
				startCalled.Store(true)
			},
			func(s *ScheduledRotation, r *RotationResult) {
				completeCalled.Store(true)
			},
		)

		schedule := &ScheduledRotation{
			ID:         "sched-1",
			SecretPath: "vault/secret/db",
			Schedule:   "0 2 * * *",
			Enabled:    true,
			Config:     &RotationConfig{Strategy: RotationStrategyBlueGreen},
			Targets:    createTestTargets(3),
		}

		_ = scheduler.AddSchedule(schedule)
		_ = scheduler.Start()
		defer scheduler.Stop()

		_ = scheduler.TriggerNow("sched-1")
		time.Sleep(200 * time.Millisecond)

		if !startCalled.Load() {
			t.Error("start callback should be called")
		}
		if !completeCalled.Load() {
			t.Error("complete callback should be called")
		}
	})
}

func TestCronField(t *testing.T) {
	t.Run("Wildcard", func(t *testing.T) {
		field, err := ParseCronField("*", 0, 59)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(field.values) != 60 {
			t.Errorf("expected 60 values, got %d", len(field.values))
		}
	})

	t.Run("SingleValue", func(t *testing.T) {
		field, err := ParseCronField("30", 0, 59)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(field.values) != 1 || field.values[0] != 30 {
			t.Errorf("expected [30], got %v", field.values)
		}
	})

	t.Run("Range", func(t *testing.T) {
		field, err := ParseCronField("10-15", 0, 59)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(field.values) != 6 {
			t.Errorf("expected 6 values, got %d", len(field.values))
		}
	})

	t.Run("List", func(t *testing.T) {
		field, err := ParseCronField("0,15,30,45", 0, 59)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(field.values) != 4 {
			t.Errorf("expected 4 values, got %d", len(field.values))
		}
	})

	t.Run("Step", func(t *testing.T) {
		field, err := ParseCronField("*/10", 0, 59)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 0, 10, 20, 30, 40, 50 = 6 values
		if len(field.values) != 6 {
			t.Errorf("expected 6 values, got %d", len(field.values))
		}
	})

	t.Run("RangeWithStep", func(t *testing.T) {
		field, err := ParseCronField("0-30/5", 0, 59)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// 0, 5, 10, 15, 20, 25, 30 = 7 values
		if len(field.values) != 7 {
			t.Errorf("expected 7 values, got %d", len(field.values))
		}
	})
}
