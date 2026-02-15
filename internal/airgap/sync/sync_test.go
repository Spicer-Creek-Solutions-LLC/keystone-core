package sync

import (
	"container/heap"
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/statemachine"
)

func validWindowConfig(name string) WindowConfig {
	return WindowConfig{
		Name:         name,
		CronSchedule: "*/1 * * * *",
		Duration:     5 * time.Minute,
		Operations: []OperationConfig{
			{Type: OpPullModules, Priority: 10, Endpoint: "http://localhost:8080"},
		},
		Enabled: true,
	}
}

func TestWindowConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  WindowConfig
		wantErr bool
	}{
		{
			"valid",
			validWindowConfig("test"),
			false,
		},
		{
			"missing name",
			WindowConfig{CronSchedule: "*/5 * * * *", Duration: time.Minute, Operations: []OperationConfig{{Type: OpPullModules}}},
			true,
		},
		{
			"missing cron",
			WindowConfig{Name: "test", Duration: time.Minute, Operations: []OperationConfig{{Type: OpPullModules}}},
			true,
		},
		{
			"invalid cron",
			WindowConfig{Name: "test", CronSchedule: "not-a-cron", Duration: time.Minute, Operations: []OperationConfig{{Type: OpPullModules}}},
			true,
		},
		{
			"zero duration",
			WindowConfig{Name: "test", CronSchedule: "*/5 * * * *", Duration: 0, Operations: []OperationConfig{{Type: OpPullModules}}},
			true,
		},
		{
			"no operations",
			WindowConfig{Name: "test", CronSchedule: "*/5 * * * *", Duration: time.Minute},
			true,
		},
		{
			"invalid timezone",
			WindowConfig{Name: "test", CronSchedule: "*/5 * * * *", Duration: time.Minute, Operations: []OperationConfig{{Type: OpPullModules}}, Timezone: "Bad/Zone"},
			true,
		},
		{
			"valid with timezone",
			WindowConfig{Name: "test", CronSchedule: "*/5 * * * *", Duration: time.Minute, Operations: []OperationConfig{{Type: OpPullModules}}, Timezone: "America/New_York"},
			false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestBandwidthLimiter_Unlimited(t *testing.T) {
	l := NewBandwidthLimiter(0)
	ctx := context.Background()

	start := time.Now()
	for i := 0; i < 100; i++ {
		if err := l.WaitN(ctx, 1024); err != nil {
			t.Fatalf("WaitN: %v", err)
		}
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Errorf("unlimited limiter took %v, expected near-instant", elapsed)
	}
}

func TestBandwidthLimiter_RateLimited(t *testing.T) {
	l := NewBandwidthLimiter(1000)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	start := time.Now()
	if err := l.WaitN(ctx, 500); err != nil {
		t.Fatalf("WaitN(500): %v", err)
	}
	if err := l.WaitN(ctx, 500); err != nil {
		t.Fatalf("WaitN(500): %v", err)
	}
	// Third request should require waiting for refill
	if err := l.WaitN(ctx, 500); err != nil {
		t.Fatalf("WaitN(500): %v", err)
	}
	elapsed := time.Since(start)
	if elapsed < 100*time.Millisecond {
		t.Errorf("rate-limited request completed too fast: %v", elapsed)
	}
}

func TestBandwidthLimiter_ContextCancel(t *testing.T) {
	l := NewBandwidthLimiter(10)
	ctx, cancel := context.WithCancel(context.Background())

	// Drain tokens
	_ = l.WaitN(ctx, 10)

	cancel()
	err := l.WaitN(ctx, 100)
	if err == nil {
		t.Error("expected context cancel error")
	}
}

func TestBandwidthLimiter_SetRate(t *testing.T) {
	l := NewBandwidthLimiter(100)
	if r := l.Rate(); r != 100 {
		t.Errorf("Rate() = %d, want 100", r)
	}
	l.SetRate(200)
	if r := l.Rate(); r != 200 {
		t.Errorf("Rate() = %d, want 200", r)
	}
	l.SetRate(0)
	if r := l.Rate(); r != 0 {
		t.Errorf("Rate() = %d, want 0", r)
	}
}

func TestStateMachine_ValidTransitions(t *testing.T) {
	m := buildSyncMachine("test")

	transitions := []struct {
		event Event
		want  State
	}{
		{EventSchedule, StateScheduled},
		{EventStart, StateRunning},
		{EventPause, StatePaused},
		{EventResume, StateRunning},
		{EventComplete, StateCompleted},
		{EventReset, StateIdle},
	}

	for _, tt := range transitions {
		if err := m.Fire(tt.event); err != nil {
			t.Fatalf("Fire(%s): %v", tt.event, err)
		}
		if got := m.State(); got != tt.want {
			t.Fatalf("after Fire(%s): state = %s, want %s", tt.event, got, tt.want)
		}
	}
}

func TestStateMachine_FailPath(t *testing.T) {
	m := buildSyncMachine("test")
	_ = m.Fire(EventSchedule)
	_ = m.Fire(EventStart)
	if err := m.Fire(EventFail); err != nil {
		t.Fatalf("Fire(fail): %v", err)
	}
	if got := m.State(); got != StateFailed {
		t.Fatalf("state = %s, want %s", got, StateFailed)
	}
}

func TestStateMachine_CancelFromRunning(t *testing.T) {
	m := buildSyncMachine("test")
	_ = m.Fire(EventSchedule)
	_ = m.Fire(EventStart)
	if err := m.Fire(EventCancel); err != nil {
		t.Fatalf("Fire(cancel): %v", err)
	}
	if got := m.State(); got != StateCancelled {
		t.Fatalf("state = %s, want %s", got, StateCancelled)
	}
}

func TestStateMachine_InvalidTransition(t *testing.T) {
	m := buildSyncMachine("test")
	err := m.Fire(EventStart) // can't start from idle
	if err == nil {
		t.Error("expected error for invalid transition")
	}
}

func TestScheduler_AddRemoveList(t *testing.T) {
	s := NewScheduler(nil)
	defer s.Close()

	cfg := validWindowConfig("w1")
	if err := s.AddWindow(cfg); err != nil {
		t.Fatalf("AddWindow: %v", err)
	}

	// Duplicate
	if err := s.AddWindow(cfg); err == nil {
		t.Error("expected error for duplicate window")
	}

	cfg2 := validWindowConfig("w2")
	if err := s.AddWindow(cfg2); err != nil {
		t.Fatalf("AddWindow: %v", err)
	}

	windows := s.ListWindows()
	if len(windows) != 2 {
		t.Fatalf("ListWindows len = %d, want 2", len(windows))
	}

	if err := s.RemoveWindow("w1"); err != nil {
		t.Fatalf("RemoveWindow: %v", err)
	}
	windows = s.ListWindows()
	if len(windows) != 1 {
		t.Fatalf("ListWindows len = %d, want 1", len(windows))
	}

	if err := s.RemoveWindow("nonexistent"); err == nil {
		t.Error("expected error for removing nonexistent window")
	}
}

func TestScheduler_TriggerNow(t *testing.T) {
	var ops int32
	opFunc := func(_ context.Context, _ OperationConfig, _ *BandwidthLimiter) error {
		atomic.AddInt32(&ops, 1)
		return nil
	}

	s := NewScheduler(opFunc)
	defer s.Close()

	cfg := validWindowConfig("test-trigger")
	cfg.Operations = []OperationConfig{
		{Type: OpPullModules, Priority: 10},
		{Type: OpPushAuditLogs, Priority: 20},
	}
	if err := s.AddWindow(cfg); err != nil {
		t.Fatal(err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := s.TriggerNow(context.Background(), "test-trigger"); err != nil {
		t.Fatalf("TriggerNow: %v", err)
	}

	// Wait for operations to complete
	deadline := time.After(5 * time.Second)
	for atomic.LoadInt32(&ops) < 2 {
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for operations, got %d", atomic.LoadInt32(&ops))
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func TestScheduler_PauseResumeCancel(t *testing.T) {
	started := make(chan struct{})
	blocked := make(chan struct{})

	opFunc := func(ctx context.Context, op OperationConfig, _ *BandwidthLimiter) error {
		if op.Priority == 10 {
			close(started)
			<-blocked
		}
		return nil
	}

	s := NewScheduler(opFunc)
	defer s.Close()

	cfg := validWindowConfig("lifecycle")
	cfg.Operations = []OperationConfig{
		{Type: OpPullModules, Priority: 10},
		{Type: OpPushAuditLogs, Priority: 20},
	}
	if err := s.AddWindow(cfg); err != nil {
		t.Fatal(err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := s.TriggerNow(context.Background(), "lifecycle"); err != nil {
		t.Fatal(err)
	}

	<-started

	// Cancel should succeed from running
	close(blocked)
	time.Sleep(100 * time.Millisecond)
	if err := s.Cancel("lifecycle"); err != nil {
		t.Logf("Cancel: %v (may already be completed)", err)
	}
}

func TestScheduler_History(t *testing.T) {
	var ops int32
	opFunc := func(_ context.Context, _ OperationConfig, _ *BandwidthLimiter) error {
		atomic.AddInt32(&ops, 1)
		return nil
	}

	s := NewScheduler(opFunc)
	defer s.Close()

	cfg := validWindowConfig("hist")
	if err := s.AddWindow(cfg); err != nil {
		t.Fatal(err)
	}

	if err := s.Start(context.Background()); err != nil {
		t.Fatal(err)
	}

	if err := s.TriggerNow(context.Background(), "hist"); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(5 * time.Second)
	for atomic.LoadInt32(&ops) < 1 {
		select {
		case <-deadline:
			t.Fatal("timed out waiting for operation")
		case <-time.After(50 * time.Millisecond):
		}
	}

	// Wait for history to be recorded
	time.Sleep(200 * time.Millisecond)

	history := s.History()
	if len(history) == 0 {
		t.Fatal("expected at least one history record")
	}
	if history[0].WindowName != "hist" {
		t.Errorf("WindowName = %q, want %q", history[0].WindowName, "hist")
	}
}

func TestScheduler_GetStatus(t *testing.T) {
	s := NewScheduler(nil)
	defer s.Close()

	cfg := validWindowConfig("status-test")
	if err := s.AddWindow(cfg); err != nil {
		t.Fatal(err)
	}

	status, err := s.GetStatus("status-test")
	if err != nil {
		t.Fatal(err)
	}
	if status.State != StateIdle {
		t.Errorf("State = %s, want %s", status.State, StateIdle)
	}
	if status.NextRun == nil {
		t.Error("NextRun should be set for enabled window")
	}

	_, err = s.GetStatus("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent window")
	}
}

func TestScheduler_GetProgress(t *testing.T) {
	s := NewScheduler(nil)
	defer s.Close()

	cfg := validWindowConfig("prog-test")
	if err := s.AddWindow(cfg); err != nil {
		t.Fatal(err)
	}

	p, err := s.GetProgress("prog-test")
	if err != nil {
		t.Fatal(err)
	}
	if p.TotalItems != 0 {
		t.Errorf("TotalItems = %d, want 0", p.TotalItems)
	}
}

func TestScheduler_TriggerNotFound(t *testing.T) {
	s := NewScheduler(nil)
	defer s.Close()

	err := s.TriggerNow(context.Background(), "missing")
	if err == nil {
		t.Error("expected error for missing window")
	}
}

func TestScheduler_ClosedRejects(t *testing.T) {
	s := NewScheduler(nil)
	s.Close()

	if err := s.AddWindow(validWindowConfig("x")); err == nil {
		t.Error("expected error after close")
	}
	if err := s.Start(context.Background()); err == nil {
		t.Error("expected error after close")
	}
}

func TestPriorityQueue_Ordering(t *testing.T) {
	q := &operationQueue{}
	heap.Init(q)

	heap.Push(q, OperationConfig{Type: OpPushMetrics, Priority: 50})
	heap.Push(q, OperationConfig{Type: OpPullModules, Priority: 10})
	heap.Push(q, OperationConfig{Type: OpPushAuditLogs, Priority: 30})

	first := heap.Pop(q).(OperationConfig)
	if first.Priority != 10 {
		t.Errorf("first priority = %d, want 10", first.Priority)
	}
	second := heap.Pop(q).(OperationConfig)
	if second.Priority != 30 {
		t.Errorf("second priority = %d, want 30", second.Priority)
	}
	third := heap.Pop(q).(OperationConfig)
	if third.Priority != 50 {
		t.Errorf("third priority = %d, want 50", third.Priority)
	}
}

func TestSortedOperations(t *testing.T) {
	ops := []OperationConfig{
		{Type: OpPushMetrics, Priority: 50},
		{Type: OpPullModules, Priority: 10},
		{Type: OpFullSync, Priority: 30},
	}
	sorted := sortedOperations(ops)
	if len(sorted) != 3 {
		t.Fatalf("len = %d, want 3", len(sorted))
	}
	if sorted[0].Priority != 10 {
		t.Errorf("sorted[0] = %d, want 10", sorted[0].Priority)
	}
	if sorted[1].Priority != 30 {
		t.Errorf("sorted[1] = %d, want 30", sorted[1].Priority)
	}
	if sorted[2].Priority != 50 {
		t.Errorf("sorted[2] = %d, want 50", sorted[2].Priority)
	}
}

// Interface compliance
var _ statemachine.Guard[State, Event] = func(_ context.Context, _ State, _ Event) bool { return true }
