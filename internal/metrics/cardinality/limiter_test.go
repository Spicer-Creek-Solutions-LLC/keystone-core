package cardinality

import (
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recordingReporter struct {
	mu       sync.Mutex
	outcomes map[string][]Outcome
}

func newRecorder() *recordingReporter {
	return &recordingReporter{outcomes: make(map[string][]Outcome)}
}

func (r *recordingReporter) Report(metric string, outcome Outcome) {
	r.mu.Lock()
	r.outcomes[metric] = append(r.outcomes[metric], outcome)
	r.mu.Unlock()
}

func (r *recordingReporter) count(metric string, want Outcome) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, o := range r.outcomes[metric] {
		if o == want {
			n++
		}
	}
	return n
}

func TestOutcomeString(t *testing.T) {
	tests := []struct {
		o    Outcome
		want string
	}{
		{Accepted, "accepted"},
		{Dropped, "dropped"},
		{Aggregated, "aggregated"},
		{Outcome(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.o.String(); got != tt.want {
			t.Errorf("Outcome(%d).String() = %q, want %q", tt.o, got, tt.want)
		}
	}
}

func TestDropMode_UnderCap_Accepts(t *testing.T) {
	rec := newRecorder()
	l := New(Options{Mode: Drop, DefaultMax: 3, Reporter: rec})
	l.Configure("m", 3)

	for i := 0; i < 3; i++ {
		outcome, vals := l.Track("m", []string{strconv.Itoa(i)})
		if outcome != Accepted {
			t.Fatalf("iter %d: outcome = %v, want Accepted", i, outcome)
		}
		if len(vals) != 1 || vals[0] != strconv.Itoa(i) {
			t.Fatalf("iter %d: vals = %v", i, vals)
		}
	}
	if got := l.Snapshot()["m"]; got != 3 {
		t.Fatalf("snapshot = %d, want 3", got)
	}
	if rec.count("m", Accepted) != 3 {
		t.Fatalf("accepted count = %d, want 3", rec.count("m", Accepted))
	}
}

func TestDropMode_OverCap_Drops(t *testing.T) {
	rec := newRecorder()
	l := New(Options{Mode: Drop, DefaultMax: 2, Reporter: rec})
	l.Configure("m", 2)

	for i := 0; i < 2; i++ {
		l.Track("m", []string{strconv.Itoa(i)})
	}
	// 3rd unique combination → dropped.
	outcome, vals := l.Track("m", []string{"3"})
	if outcome != Dropped {
		t.Fatalf("outcome = %v, want Dropped", outcome)
	}
	if vals != nil {
		t.Fatalf("vals = %v, want nil", vals)
	}
	if rec.count("m", Dropped) != 1 {
		t.Fatalf("dropped count = %d, want 1", rec.count("m", Dropped))
	}
}

func TestDropMode_RepeatedCombo_AlwaysAccepted(t *testing.T) {
	rec := newRecorder()
	l := New(Options{Mode: Drop, DefaultMax: 1, Reporter: rec})
	l.Configure("m", 1)

	for i := 0; i < 100; i++ {
		outcome, _ := l.Track("m", []string{"only"})
		if outcome != Accepted {
			t.Fatalf("iter %d: outcome = %v, want Accepted (combo previously seen)", i, outcome)
		}
	}
	if rec.count("m", Accepted) != 100 {
		t.Fatalf("accepted count = %d, want 100", rec.count("m", Accepted))
	}
}

func TestAggregateMode_OverCap_RewritesToOverflow(t *testing.T) {
	rec := newRecorder()
	l := New(Options{Mode: Aggregate, DefaultMax: 1, Reporter: rec})
	l.Configure("m", 1)

	l.Track("m", []string{"first", "x"}) // fills the cap
	outcome, vals := l.Track("m", []string{"second", "y"})
	if outcome != Aggregated {
		t.Fatalf("outcome = %v, want Aggregated", outcome)
	}
	if len(vals) != 2 || vals[0] != OverflowSentinel || vals[1] != OverflowSentinel {
		t.Fatalf("vals = %v, want [%s %s]", vals, OverflowSentinel, OverflowSentinel)
	}
}

func TestNilReporter_StillTracks(t *testing.T) {
	l := New(Options{Mode: Drop, DefaultMax: 1, Reporter: nil})
	l.Configure("m", 1)
	if outcome, _ := l.Track("m", []string{"a"}); outcome != Accepted {
		t.Fatalf("outcome = %v", outcome)
	}
	if outcome, _ := l.Track("m", []string{"b"}); outcome != Dropped {
		t.Fatalf("outcome = %v", outcome)
	}
}

func TestUnconfiguredMetric_UsesDefaultMax(t *testing.T) {
	rec := newRecorder()
	l := New(Options{Mode: Drop, DefaultMax: 2, Reporter: rec})
	// No Configure call for "m".
	l.Track("m", []string{"a"})
	l.Track("m", []string{"b"})
	if outcome, _ := l.Track("m", []string{"c"}); outcome != Dropped {
		t.Fatalf("3rd combo outcome = %v, want Dropped (default cap = 2)", outcome)
	}
}

func TestConfigureGrowingCap_NewCombosAccepted(t *testing.T) {
	rec := newRecorder()
	l := New(Options{Mode: Drop, DefaultMax: 1, Reporter: rec})
	l.Configure("m", 1)
	l.Track("m", []string{"a"})
	if outcome, _ := l.Track("m", []string{"b"}); outcome != Dropped {
		t.Fatalf("pre-grow: outcome = %v, want Dropped", outcome)
	}

	l.Configure("m", 5)
	if outcome, _ := l.Track("m", []string{"b"}); outcome != Accepted {
		t.Fatalf("post-grow: outcome = %v, want Accepted", outcome)
	}
}

func TestConcurrentTrack_RaceFree(t *testing.T) {
	rec := newRecorder()
	l := New(Options{Mode: Drop, DefaultMax: 50, Reporter: rec})
	l.Configure("m", 50)

	var wg sync.WaitGroup
	var dropped atomic.Int64
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				outcome, _ := l.Track("m", []string{strconv.Itoa(gid*200 + i)})
				if outcome == Dropped {
					dropped.Add(1)
				}
			}
		}(g)
	}
	wg.Wait()

	accepted := l.Snapshot()["m"]
	if accepted != 50 {
		t.Fatalf("accepted combos = %d, want 50 (cap)", accepted)
	}
	// Each goroutine produces 200 unique combos × 8 = 1600 total;
	// only 50 fit in the cap, so exactly 1550 must be Dropped.
	if dropped.Load() != 1550 {
		t.Fatalf("dropped = %d, want 1550", dropped.Load())
	}
}

func TestWarnThrottling(t *testing.T) {
	rec := newRecorder()
	// We can't easily intercept the slog warn — assert via clock that
	// the maybeWarn path doesn't deadlock or panic under load.
	l := New(Options{Mode: Drop, DefaultMax: 1, Reporter: rec, WarnEveryAfter: time.Hour})
	l.Configure("m", 1)
	now := time.Unix(0, 0)
	l.SetClock(func() time.Time { return now })

	l.Track("m", []string{"a"})    // accepted, fills cap
	l.Track("m", []string{"b"})    // dropped → warn fires
	l.Track("m", []string{"c"})    // dropped → throttled (same now)

	now = now.Add(2 * time.Hour)
	l.Track("m", []string{"d"})    // dropped → warn fires again

	// We assert outcomes only; warn logging is observable to operators
	// but we won't pin slog wiring in a unit test.
	if rec.count("m", Dropped) != 3 {
		t.Fatalf("dropped count = %d, want 3", rec.count("m", Dropped))
	}
}

func TestJoinValues_HandlesSeparator(t *testing.T) {
	// Pathological label values containing the record separator must
	// still produce stable keys; we only need the key to be a function
	// of the slice, not strictly unambiguous.
	a := joinValues([]string{"x\x1ey", "z"})
	b := joinValues([]string{"x", "y\x1ez"})
	// These hash to the same string today; documented limitation. Test
	// the documented behaviour so a future change is intentional.
	if !strings.Contains(a, "\x1e") || !strings.Contains(b, "\x1e") {
		t.Fatalf("joinValues should retain separator: a=%q b=%q", a, b)
	}
}
