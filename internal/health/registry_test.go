package health

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(io.Discard, nil))
}

type stubChecker struct {
	name     string
	interval time.Duration
	fn       func(ctx context.Context) error
	calls    atomic.Int64
}

func (s *stubChecker) Name() string                       { return s.name }
func (s *stubChecker) Interval() time.Duration            { return s.interval }
func (s *stubChecker) Check(ctx context.Context) error {
	s.calls.Add(1)
	if s.fn == nil {
		return nil
	}
	return s.fn(ctx)
}

func TestStatus_String(t *testing.T) {
	cases := []struct{ s Status }{
		{StatusHealthy}, {StatusDegraded}, {StatusUnhealthy}, {StatusUnknown},
	}
	for _, c := range cases {
		if got := c.s.String(); got != string(c.s) {
			t.Errorf("%s.String() = %q, want %q", c.s, got, string(c.s))
		}
	}
}

func TestRegistry_AllHealthyAfterGrace(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := NewRegistry(Options{
		CheckTimeout:       time.Second,
		StartupGracePeriod: 30 * time.Second,
		StartedAt:          now.Add(-time.Minute),
		Now:                func() time.Time { return now },
		Logger:             discardLogger(),
	})
	r.Register(&stubChecker{name: "a"}, &stubChecker{name: "b"})

	snap := r.Snapshot(context.Background())
	if !snap.Ready {
		t.Errorf("Ready=false, want true")
	}
	if snap.InGracePeriod {
		t.Errorf("InGracePeriod=true after 60s uptime")
	}
	if len(snap.Results) != 2 {
		t.Fatalf("Results len = %d, want 2", len(snap.Results))
	}
	for _, res := range snap.Results {
		if res.Status != StatusHealthy {
			t.Errorf("%s: status = %s, want healthy", res.Name, res.Status)
		}
	}
}

func TestRegistry_InGracePeriod(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := NewRegistry(Options{
		StartupGracePeriod: 30 * time.Second,
		StartedAt:          now.Add(-5 * time.Second),
		Now:                func() time.Time { return now },
		Logger:             discardLogger(),
	})
	r.Register(&stubChecker{name: "a"})
	snap := r.Snapshot(context.Background())
	if snap.Ready {
		t.Errorf("Ready=true during grace")
	}
	if !snap.InGracePeriod {
		t.Errorf("InGracePeriod=false during grace")
	}
}

func TestRegistry_OneFailure_NotReady(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := NewRegistry(Options{
		StartupGracePeriod: time.Second,
		StartedAt:          now.Add(-time.Minute),
		Now:                func() time.Time { return now },
		Logger:             discardLogger(),
	})
	wantErr := errors.New("boom")
	r.Register(
		&stubChecker{name: "a"},
		&stubChecker{name: "b", fn: func(context.Context) error { return wantErr }},
	)
	snap := r.Snapshot(context.Background())
	if snap.Ready {
		t.Errorf("Ready=true with one failure")
	}
	byName := map[string]Result{}
	for _, r := range snap.Results {
		byName[r.Name] = r
	}
	if byName["a"].Status != StatusHealthy {
		t.Errorf("a status = %s, want healthy", byName["a"].Status)
	}
	if byName["b"].Status != StatusUnhealthy {
		t.Errorf("b status = %s, want unhealthy", byName["b"].Status)
	}
	if !errors.Is(byName["b"].Err, wantErr) {
		t.Errorf("b err = %v, want %v", byName["b"].Err, wantErr)
	}
}

func TestRegistry_PerCheckTimeout(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	r := NewRegistry(Options{
		CheckTimeout: 20 * time.Millisecond,
		StartedAt:    now,
		Now:          func() time.Time { return now },
		Logger:       discardLogger(),
	})
	r.Register(&stubChecker{
		name: "slow",
		fn: func(ctx context.Context) error {
			select {
			case <-time.After(200 * time.Millisecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		},
	})
	snap := r.Snapshot(context.Background())
	if got := snap.Results[0].Status; got != StatusUnhealthy {
		t.Errorf("slow check status = %s, want unhealthy (timed out)", got)
	}
}

func TestRegistry_ChecksRunInParallel(t *testing.T) {
	r := NewRegistry(Options{
		CheckTimeout: 500 * time.Millisecond,
		StartedAt:    time.Now().Add(-time.Minute),
		Logger:       discardLogger(),
	})
	delay := 80 * time.Millisecond
	r.Register(
		&stubChecker{name: "a", fn: func(ctx context.Context) error { time.Sleep(delay); return nil }},
		&stubChecker{name: "b", fn: func(ctx context.Context) error { time.Sleep(delay); return nil }},
	)
	t0 := time.Now()
	r.Snapshot(context.Background())
	if elapsed := time.Since(t0); elapsed > delay*2-10*time.Millisecond {
		// Sequential would take ~2*delay; parallel should be ~delay.
		// Tolerance for scheduler jitter; the failure case is "elapsed
		// is roughly 2*delay" which would fail clearly.
		t.Errorf("checks ran serially: %s elapsed for two %s checks", elapsed, delay)
	}
}

func TestRegistry_RegisterAccumulates(t *testing.T) {
	r := NewRegistry(Options{Logger: discardLogger()})
	r.Register(&stubChecker{name: "a"})
	r.Register(&stubChecker{name: "b"}, &stubChecker{name: "c"})
	names := r.Names()
	if want := []string{"a", "b", "c"}; !equalSlices(names, want) {
		t.Errorf("Names = %v, want %v", names, want)
	}
}

func TestRegistry_StartedAt_DefaultsToNow(t *testing.T) {
	tNow := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r := NewRegistry(Options{
		Now:    func() time.Time { return tNow },
		Logger: discardLogger(),
	})
	if got := r.StartedAt(); !got.Equal(tNow) {
		t.Errorf("StartedAt = %s, want %s", got, tNow)
	}
}

func TestRegistry_LogsOnFailure(t *testing.T) {
	var sb strings.Builder
	logger := slog.New(slog.NewTextHandler(&sb, &slog.HandlerOptions{Level: slog.LevelWarn}))
	r := NewRegistry(Options{StartedAt: time.Now(), Logger: logger})
	r.Register(&stubChecker{name: "a", fn: func(context.Context) error { return errors.New("nope") }})
	r.Snapshot(context.Background())
	if !strings.Contains(sb.String(), "health: check failed") {
		t.Errorf("expected warn log, got: %s", sb.String())
	}
	if !strings.Contains(sb.String(), `component=a`) {
		t.Errorf("warn log missing component=a: %s", sb.String())
	}
}

func TestRegistry_ConcurrentSnapshots_RaceFree(t *testing.T) {
	r := NewRegistry(Options{
		CheckTimeout: 100 * time.Millisecond,
		StartedAt:    time.Now().Add(-time.Minute),
		Logger:       discardLogger(),
	})
	r.Register(&stubChecker{name: "a"}, &stubChecker{name: "b"})
	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			snap := r.Snapshot(context.Background())
			if !snap.Ready {
				t.Errorf("Ready=false in concurrent snapshot")
			}
		}()
	}
	wg.Wait()
}

func equalSlices(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
