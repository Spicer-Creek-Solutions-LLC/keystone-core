// SPDX-License-Identifier: Apache-2.0

package audit

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type retentionStubStore struct {
	mu            sync.Mutex
	applyCalls    int
	lastPolicy    RetentionPolicy
	deleteCount   int
	err           error
	applySignaled chan struct{}
}

func (s *retentionStubStore) Store(context.Context, AuditEntry) error        { return nil }
func (s *retentionStubStore) StoreBatch(context.Context, []AuditEntry) error { return nil }
func (s *retentionStubStore) Get(context.Context, string) (AuditEntry, error) {
	return AuditEntry{}, nil
}
func (s *retentionStubStore) Query(context.Context, AuditQuery) (AuditPage, error) {
	return AuditPage{}, nil
}
func (s *retentionStubStore) Count(context.Context, AuditQuery) (int, error) { return 0, nil }
func (s *retentionStubStore) Delete(context.Context, string) error           { return nil }
func (s *retentionStubStore) ApplyRetention(_ context.Context, p RetentionPolicy) (int, error) {
	s.mu.Lock()
	s.applyCalls++
	s.lastPolicy = p
	deleted := s.deleteCount
	err := s.err
	signal := s.applySignaled
	s.mu.Unlock()
	if signal != nil {
		select {
		case signal <- struct{}{}:
		default:
		}
	}
	return deleted, err
}
func (s *retentionStubStore) Summarize(context.Context, AuditQuery) (AuditSummary, error) {
	return AuditSummary{}, nil
}
func (s *retentionStubStore) Close() error { return nil }

func (s *retentionStubStore) calls() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.applyCalls
}

func silentLog() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewRetentionEnforcer_RequiresStore(t *testing.T) {
	t.Parallel()
	_, err := NewRetentionEnforcer()
	if err == nil {
		t.Errorf("missing-store validated")
	}
}

func TestNewRetentionEnforcer_DefaultsApplied(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{}
	e, err := NewRetentionEnforcer(WithRetentionStore(s))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if e.interval != DefaultRetentionInterval {
		t.Errorf("interval = %v", e.interval)
	}
	if e.jitter != DefaultRetentionJitter {
		t.Errorf("jitter = %f", e.jitter)
	}
	if e.policy.MaxAge != DefaultRetentionMaxAge {
		t.Errorf("default policy.MaxAge = %v, want %v", e.policy.MaxAge, DefaultRetentionMaxAge)
	}
	if e.policy.MaxCount != DefaultRetentionMaxCount {
		t.Errorf("default policy.MaxCount = %d", e.policy.MaxCount)
	}
}

func TestNewRetentionEnforcer_InvalidValuesFallback(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(s),
		WithRetentionInterval(0),
		WithRetentionJitter(-0.1),
		WithRetentionJitter(0.99),
		WithRetentionLeaderCheck(nil),
		WithRetentionLogger(nil),
	)
	if e.interval != DefaultRetentionInterval {
		t.Errorf("interval fallback failed")
	}
	if e.jitter != DefaultRetentionJitter {
		t.Errorf("jitter fallback failed")
	}
	if e.leaderCheck == nil {
		t.Errorf("leaderCheck nil after WithRetentionLeaderCheck(nil)")
	}
	if e.logger == nil {
		t.Errorf("logger nil")
	}
}

func TestRunOnce_HappyPath(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{deleteCount: 7}
	e, _ := NewRetentionEnforcer(WithRetentionStore(s), WithRetentionLogger(silentLog()))
	deleted, err := e.RunOnce(context.Background())
	if err != nil {
		t.Fatalf("%v", err)
	}
	if deleted != 7 {
		t.Errorf("deleted = %d, want 7", deleted)
	}
	if e.LastRunDeleted() != 7 {
		t.Errorf("LastRunDeleted = %d", e.LastRunDeleted())
	}
	if e.TotalDeleted() != 7 {
		t.Errorf("TotalDeleted = %d", e.TotalDeleted())
	}
	if e.LastRunAt().IsZero() {
		t.Errorf("LastRunAt zero after success")
	}
}

func TestRunOnce_StoreErrorRecorded(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{err: errors.New("sim")}
	e, _ := NewRetentionEnforcer(WithRetentionStore(s), WithRetentionLogger(silentLog()))
	if _, err := e.RunOnce(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
	if e.RunsFailed() != 1 {
		t.Errorf("RunsFailed = %d", e.RunsFailed())
	}
	if e.LastRunDeleted() != 0 {
		t.Errorf("LastRunDeleted updated on failure")
	}
}

func TestRunOnce_LeaderCheckFalseSkips(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{deleteCount: 5}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(s),
		WithRetentionLeaderCheck(func() bool { return false }),
		WithRetentionLogger(silentLog()),
	)
	if d, err := e.RunOnce(context.Background()); err != nil || d != 0 {
		t.Errorf("non-leader: d=%d err=%v", d, err)
	}
	if s.calls() != 0 {
		t.Errorf("ApplyRetention called %d times, want 0", s.calls())
	}
}

func TestRunOnce_PassesPolicyToStore(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{}
	want := RetentionPolicy{
		MaxAge:      72 * time.Hour,
		MaxCount:    500,
		MinSeverity: SeverityHigh,
	}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(s),
		WithRetentionPolicy(want),
		WithRetentionLogger(silentLog()),
	)
	_, _ = e.RunOnce(context.Background())
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lastPolicy.MaxAge != want.MaxAge || s.lastPolicy.MaxCount != want.MaxCount {
		t.Errorf("policy not passed: %+v", s.lastPolicy)
	}
	if s.lastPolicy.MinSeverity != SeverityHigh {
		t.Errorf("MinSeverity not passed: %s", s.lastPolicy.MinSeverity)
	}
}

func TestScheduler_FirstTickAfterInterval(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{
		applySignaled: make(chan struct{}, 4),
		deleteCount:   1,
	}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(s),
		WithRetentionInterval(50*time.Millisecond),
		WithRetentionJitter(0),
		WithRetentionLogger(silentLog()),
	)
	if err := e.Start(context.Background()); err != nil {
		t.Fatalf("%v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = e.Stop(stopCtx)
	})

	// No immediate tick.
	select {
	case <-s.applySignaled:
		t.Fatalf("ApplyRetention fired before first interval (boot storm)")
	case <-time.After(10 * time.Millisecond):
	}
	// First tick by ~60ms.
	select {
	case <-s.applySignaled:
	case <-time.After(500 * time.Millisecond):
		t.Fatalf("first tick never fired")
	}
}

func TestScheduler_TicksRepeatedly(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{applySignaled: make(chan struct{}, 8)}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(s),
		WithRetentionInterval(20*time.Millisecond),
		WithRetentionJitter(0),
		WithRetentionLogger(silentLog()),
	)
	_ = e.Start(context.Background())
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = e.Stop(stopCtx)
	})
	for i := 0; i < 3; i++ {
		select {
		case <-s.applySignaled:
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("tick %d never fired", i)
		}
	}
}

func TestScheduler_ErrorsDoNotStopScheduler(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{
		err:           errors.New("transient"),
		applySignaled: make(chan struct{}, 8),
	}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(s),
		WithRetentionInterval(20*time.Millisecond),
		WithRetentionJitter(0),
		WithRetentionLogger(silentLog()),
	)
	_ = e.Start(context.Background())
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		_ = e.Stop(stopCtx)
	})
	for i := 0; i < 3; i++ {
		select {
		case <-s.applySignaled:
		case <-time.After(200 * time.Millisecond):
			t.Fatalf("tick %d never fired (errors stopped scheduler?)", i)
		}
	}
	// runsFailed increments AFTER the stub's Apply returns
	// (retention.go:196), but applySignaled fires INSIDE the stub
	// BEFORE the production-side increment — so the third counter
	// bump may not be visible the instant the third channel signal
	// is consumed. Poll briefly so the increment lands; read once
	// into got so the error message can't disagree with the branch
	// condition (the pre-fix code called RunsFailed() twice and
	// produced "RunsFailed = 3, want >= 3" because the counter
	// incremented between the if and the Errorf format args).
	var got int64
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		got = e.RunsFailed()
		if got >= 3 {
			break
		}
		time.Sleep(2 * time.Millisecond)
	}
	if got < 3 {
		t.Errorf("RunsFailed = %d, want >= 3", got)
	}
}

func TestScheduler_StopIsIdempotent(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(WithRetentionStore(s), WithRetentionLogger(silentLog()))
	if err := e.Stop(context.Background()); err != nil {
		t.Errorf("Stop-before-Start: %v", err)
	}
	_ = e.Start(context.Background())
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := e.Stop(stopCtx); err != nil {
		t.Errorf("first Stop: %v", err)
	}
	if err := e.Stop(stopCtx); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestScheduler_DoubleStartRejected(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(WithRetentionStore(s), WithRetentionLogger(silentLog()))
	_ = e.Start(context.Background())
	t.Cleanup(func() { _ = e.Stop(context.Background()) })
	if err := e.Start(context.Background()); err == nil {
		t.Errorf("double Start succeeded")
	}
}

func TestRetentionEnforcer_ConcurrentRunOnce(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{deleteCount: 1}
	e, _ := NewRetentionEnforcer(WithRetentionStore(s), WithRetentionLogger(silentLog()))
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = e.RunOnce(context.Background())
		}()
	}
	wg.Wait()
	if e.TotalDeleted() != int64(n) {
		t.Errorf("TotalDeleted = %d, want %d", e.TotalDeleted(), n)
	}
}

func TestNextWait_RespectsJitterBound(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(s),
		WithRetentionInterval(time.Second),
		WithRetentionJitter(0.2),
	)
	for i := 0; i < 200; i++ {
		d := e.nextWait()
		if d < 800*time.Millisecond || d > 1200*time.Millisecond {
			t.Fatalf("sample %d: %v outside ±20%%", i, d)
		}
	}
}

func TestNextWait_ZeroJitterIsExact(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(s),
		WithRetentionInterval(time.Second),
		WithRetentionJitter(0),
	)
	if d := e.nextWait(); d != time.Second {
		t.Errorf("zero jitter = %v", d)
	}
}

func TestRetentionEnforcer_String(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{}
	e, _ := NewRetentionEnforcer(
		WithRetentionStore(s),
		WithRetentionPolicy(RetentionPolicy{
			MaxAge:      48 * time.Hour,
			MaxCount:    500,
			MinSeverity: SeverityHigh,
		}),
	)
	got := e.String()
	for _, want := range []string{"max_age=48h", "max_count=500", "min_severity=high"} {
		if !containsStr(got, want) {
			t.Errorf("String missing %q: %s", want, got)
		}
	}
}

func containsStr(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

// concurrent counter sanity — race detector catches misuse.
func TestRetentionEnforcer_TotalDeletedAtomic(t *testing.T) {
	t.Parallel()
	s := &retentionStubStore{deleteCount: 3}
	e, _ := NewRetentionEnforcer(WithRetentionStore(s), WithRetentionLogger(silentLog()))
	const n = 100
	var wg sync.WaitGroup
	var sum atomic.Int64
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d, _ := e.RunOnce(context.Background())
			sum.Add(int64(d))
		}()
	}
	wg.Wait()
	if e.TotalDeleted() != sum.Load() {
		t.Errorf("TotalDeleted = %d, want %d", e.TotalDeleted(), sum.Load())
	}
}
