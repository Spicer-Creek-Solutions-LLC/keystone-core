package metrics

import "time"

// Timer measures elapsed wall-clock time and records it as a histogram
// observation in seconds. Usage:
//
//	t := metrics.StartTimer()
//	defer t.ObserveDuration(commandLatency.With(metrics.Labels{"type": "exec"}))
//	... work ...
//
// Timer is stateless and unsynchronised — construct one per call site.
type Timer struct {
	start time.Time
	now   func() time.Time
}

// StartTimer returns a Timer marked at the current time.Now().
func StartTimer() Timer { return Timer{start: time.Now()} }

// startTimerWithClock is the test seam for deterministic timing.
func startTimerWithClock(now func() time.Time) Timer {
	return Timer{start: now(), now: now}
}

// ObserveDuration records elapsed seconds since StartTimer into h.
// Returns the observed duration so callers can both record and use it.
func (t Timer) ObserveDuration(h Histogram) time.Duration {
	elapsed := t.elapsed()
	h.Observe(elapsed.Seconds())
	return elapsed
}

// ObserveSummary records elapsed seconds into a Summary (e.g. for
// quantile reporting where bucketed histograms are too coarse).
func (t Timer) ObserveSummary(s Summary) time.Duration {
	elapsed := t.elapsed()
	s.Observe(elapsed.Seconds())
	return elapsed
}

// Elapsed returns the duration since StartTimer without recording it.
func (t Timer) Elapsed() time.Duration { return t.elapsed() }

func (t Timer) elapsed() time.Duration {
	if t.now != nil {
		return t.now().Sub(t.start)
	}
	return time.Since(t.start)
}
