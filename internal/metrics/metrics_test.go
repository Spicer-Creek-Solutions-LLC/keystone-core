// SPDX-License-Identifier: Apache-2.0

package metrics

import (
	"io"
	"log/slog"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"

	"go.keystone-core.io/keystone-core/internal/metrics/cardinality"
)

func newTestRegistry(t *testing.T) *Registry {
	t.Helper()
	return NewRegistry(Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultMaxCardinality:    1000,
		DisableRuntimeCollectors: true,
	})
}

// gather returns the named metric family or nil.
func gather(t *testing.T, r *Registry, name string) *dto.MetricFamily {
	t.Helper()
	mfs, err := r.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

// sampleByLabels returns the metric matching the given label map, or nil.
func sampleByLabels(mf *dto.MetricFamily, want Labels) *dto.Metric {
	if mf == nil {
		return nil
	}
outer:
	for _, m := range mf.GetMetric() {
		got := map[string]string{}
		for _, lp := range m.GetLabel() {
			got[lp.GetName()] = lp.GetValue()
		}
		if len(got) != len(want) {
			continue
		}
		for k, v := range want {
			if got[k] != v {
				continue outer
			}
		}
		return m
	}
	return nil
}

func TestRegistry_AutoRegistersRuntimeCollectors(t *testing.T) {
	r := NewRegistry(Options{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	mfs, err := r.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	want := map[string]bool{"go_goroutines": false, "process_start_time_seconds": false}
	for _, mf := range mfs {
		if _, ok := want[mf.GetName()]; ok {
			want[mf.GetName()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("runtime collector %q not auto-registered", name)
		}
	}
}

func TestRegistry_DisableRuntimeCollectors_SuppressesThem(t *testing.T) {
	r := NewRegistry(Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DisableRuntimeCollectors: true,
	})
	mfs, err := r.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range mfs {
		if mf.GetName() == "go_goroutines" || mf.GetName() == "process_start_time_seconds" {
			t.Errorf("runtime collector %q leaked despite DisableRuntimeCollectors=true", mf.GetName())
		}
	}
}

func TestRegistry_PreRegistersCardinalityCounter(t *testing.T) {
	r := newTestRegistry(t)
	// Prom Gather() only returns metric families with at least one
	// observed child. Verify pre-registration via Definitions() instead
	// (the *Vec is registered with the underlying Prom registry; an
	// observation lands in the gather output as soon as one fires).
	if _, ok := r.Definitions()[CardinalityMetricName]; !ok {
		t.Fatalf("%s not registered at construction", CardinalityMetricName)
	}
}

func TestRegistry_RejectsDuplicateName(t *testing.T) {
	r := newTestRegistry(t)
	def := MetricDef{Name: "kscore_test_total", Help: "h"}
	if _, err := r.NewCounter(def); err != nil {
		t.Fatalf("first register: %v", err)
	}
	_, err := r.NewCounter(def)
	if err == nil {
		t.Fatalf("second register: want error, got nil")
	}
}

func TestRegistry_RejectsEmptyName(t *testing.T) {
	r := newTestRegistry(t)
	_, err := r.NewCounter(MetricDef{Help: "h"})
	if err == nil {
		t.Fatalf("want error for empty Name")
	}
}

func TestRegistry_RejectsEmptyHelp(t *testing.T) {
	r := newTestRegistry(t)
	_, err := r.NewCounter(MetricDef{Name: "kscore_test_total"})
	if err == nil {
		t.Fatalf("want error for empty Help")
	}
}

func TestRegistry_Definitions(t *testing.T) {
	r := newTestRegistry(t)
	if _, err := r.NewCounter(MetricDef{Name: "kscore_a_total", Help: "a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := r.NewGauge(MetricDef{Name: "kscore_b", Help: "b"}); err != nil {
		t.Fatal(err)
	}

	defs := r.Definitions()
	if _, ok := defs["kscore_a_total"]; !ok {
		t.Errorf("kscore_a_total missing from Definitions")
	}
	if _, ok := defs["kscore_b"]; !ok {
		t.Errorf("kscore_b missing from Definitions")
	}
	// Pre-registered self-metric should appear too.
	if _, ok := defs[CardinalityMetricName]; !ok {
		t.Errorf("%s missing from Definitions", CardinalityMetricName)
	}
	// Copy semantics: mutating returned map must not affect Registry.
	delete(defs, "kscore_a_total")
	if _, ok := r.Definitions()["kscore_a_total"]; !ok {
		t.Errorf("Definitions did not return a copy")
	}
}

func TestCounter_NoLabels_IncAdd(t *testing.T) {
	r := newTestRegistry(t)
	c, err := r.NewCounter(MetricDef{Name: "kscore_jobs_total", Help: "h"})
	if err != nil {
		t.Fatal(err)
	}
	c.Inc()
	c.Add(4)
	c.Add(-2) // negative → silently dropped

	mf := gather(t, r, "kscore_jobs_total")
	m := sampleByLabels(mf, Labels{})
	if m == nil {
		t.Fatalf("no sample")
	}
	if got := m.GetCounter().GetValue(); got != 5 {
		t.Fatalf("counter = %v, want 5", got)
	}
}

func TestCounter_Labels_BoundIncOnly(t *testing.T) {
	r := newTestRegistry(t)
	c, err := r.NewCounter(MetricDef{
		Name: "kscore_cmds_total", Help: "h", Labels: []string{"status", "agent"},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.Inc() // unbound + labeled → no-op
	c.With(Labels{"status": "ok", "agent": "a1"}).Inc()
	c.With(Labels{"status": "ok", "agent": "a1"}).Add(2)
	c.With(Labels{"status": "fail", "agent": "a1"}).Inc()

	mf := gather(t, r, "kscore_cmds_total")
	if m := sampleByLabels(mf, Labels{"status": "ok", "agent": "a1"}); m == nil || m.GetCounter().GetValue() != 3 {
		t.Errorf("status=ok value = %v, want 3", m)
	}
	if m := sampleByLabels(mf, Labels{"status": "fail", "agent": "a1"}); m == nil || m.GetCounter().GetValue() != 1 {
		t.Errorf("status=fail value = %v, want 1", m)
	}
}

func TestGauge_NoLabels_AllOps(t *testing.T) {
	r := newTestRegistry(t)
	g, err := r.NewGauge(MetricDef{Name: "kscore_g", Help: "h"})
	if err != nil {
		t.Fatal(err)
	}
	g.Set(10)
	g.Inc()
	g.Inc()
	g.Dec()
	g.Add(3)
	g.Sub(1)

	mf := gather(t, r, "kscore_g")
	m := sampleByLabels(mf, Labels{})
	if got := m.GetGauge().GetValue(); got != 13 {
		t.Fatalf("gauge = %v, want 13", got)
	}
}

func TestGauge_Labels(t *testing.T) {
	r := newTestRegistry(t)
	g, err := r.NewGauge(MetricDef{
		Name: "kscore_gv", Help: "h", Labels: []string{"k"},
	})
	if err != nil {
		t.Fatal(err)
	}
	g.With(Labels{"k": "v1"}).Set(2.5)
	g.With(Labels{"k": "v2"}).Set(7.0)

	mf := gather(t, r, "kscore_gv")
	if m := sampleByLabels(mf, Labels{"k": "v1"}); m.GetGauge().GetValue() != 2.5 {
		t.Errorf("v1 = %v, want 2.5", m.GetGauge().GetValue())
	}
	if m := sampleByLabels(mf, Labels{"k": "v2"}); m.GetGauge().GetValue() != 7.0 {
		t.Errorf("v2 = %v, want 7.0", m.GetGauge().GetValue())
	}
}

func TestHistogram_DefBuckets(t *testing.T) {
	r := newTestRegistry(t)
	h, err := r.NewHistogram(MetricDef{Name: "kscore_h_seconds", Help: "h"})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range []float64{0.001, 0.05, 0.5, 5} {
		h.Observe(v)
	}
	mf := gather(t, r, "kscore_h_seconds")
	m := sampleByLabels(mf, Labels{})
	if m == nil {
		t.Fatalf("no histogram sample")
	}
	if got := m.GetHistogram().GetSampleCount(); got != 4 {
		t.Errorf("sample count = %d, want 4", got)
	}
	if got := len(m.GetHistogram().GetBucket()); got != len(prometheus.DefBuckets) {
		t.Errorf("bucket count = %d, want %d", got, len(prometheus.DefBuckets))
	}
}

func TestHistogram_CustomBuckets(t *testing.T) {
	r := newTestRegistry(t)
	h, err := r.NewHistogram(MetricDef{
		Name:    "kscore_h2_seconds",
		Help:    "h",
		Buckets: []float64{0.1, 1, 10},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.Observe(0.5)
	mf := gather(t, r, "kscore_h2_seconds")
	m := sampleByLabels(mf, Labels{})
	if got := len(m.GetHistogram().GetBucket()); got != 3 {
		t.Fatalf("custom bucket count = %d, want 3", got)
	}
}

func TestHistogram_Labels(t *testing.T) {
	r := newTestRegistry(t)
	h, err := r.NewHistogram(MetricDef{
		Name: "kscore_hv_seconds", Help: "h", Labels: []string{"type"},
	})
	if err != nil {
		t.Fatal(err)
	}
	h.With(Labels{"type": "exec"}).Observe(0.1)
	h.With(Labels{"type": "state"}).Observe(0.2)

	mf := gather(t, r, "kscore_hv_seconds")
	if m := sampleByLabels(mf, Labels{"type": "exec"}); m.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("type=exec count = %d", m.GetHistogram().GetSampleCount())
	}
	if m := sampleByLabels(mf, Labels{"type": "state"}); m.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("type=state count = %d", m.GetHistogram().GetSampleCount())
	}
}

func TestSummary_DefaultObjectives(t *testing.T) {
	r := newTestRegistry(t)
	s, err := r.NewSummary(MetricDef{Name: "kscore_s_seconds", Help: "h"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		s.Observe(float64(i))
	}
	mf := gather(t, r, "kscore_s_seconds")
	m := sampleByLabels(mf, Labels{})
	if got := m.GetSummary().GetSampleCount(); got != 100 {
		t.Errorf("summary sample count = %d, want 100", got)
	}
	if got := len(m.GetSummary().GetQuantile()); got != 3 {
		t.Errorf("quantile count = %d, want 3 (default objectives)", got)
	}
}

func TestSummary_Labels(t *testing.T) {
	r := newTestRegistry(t)
	s, err := r.NewSummary(MetricDef{
		Name: "kscore_sv_seconds", Help: "h", Labels: []string{"k"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s.With(Labels{"k": "a"}).Observe(1)
	s.With(Labels{"k": "b"}).Observe(2)

	mf := gather(t, r, "kscore_sv_seconds")
	if m := sampleByLabels(mf, Labels{"k": "a"}); m.GetSummary().GetSampleCount() != 1 {
		t.Errorf("k=a count = %d", m.GetSummary().GetSampleCount())
	}
}

func TestTimer_RecordsElapsed(t *testing.T) {
	r := newTestRegistry(t)
	h, err := r.NewHistogram(MetricDef{Name: "kscore_t_seconds", Help: "h"})
	if err != nil {
		t.Fatal(err)
	}

	clock := time.Unix(0, 0)
	timer := startTimerWithClock(func() time.Time { return clock })
	clock = clock.Add(250 * time.Millisecond)
	elapsed := timer.ObserveDuration(h)

	if elapsed != 250*time.Millisecond {
		t.Errorf("elapsed = %v, want 250ms", elapsed)
	}
	mf := gather(t, r, "kscore_t_seconds")
	m := sampleByLabels(mf, Labels{})
	if m.GetHistogram().GetSampleCount() != 1 {
		t.Errorf("histogram sample count = %d, want 1", m.GetHistogram().GetSampleCount())
	}
	// 0.25s — falls into the 0.25 bucket of DefBuckets.
	if sum := m.GetHistogram().GetSampleSum(); sum < 0.24 || sum > 0.26 {
		t.Errorf("histogram sum = %v, want ~0.25", sum)
	}
}

func TestTimer_ObserveSummary(t *testing.T) {
	r := newTestRegistry(t)
	s, err := r.NewSummary(MetricDef{Name: "kscore_ts_seconds", Help: "h"})
	if err != nil {
		t.Fatal(err)
	}

	clock := time.Unix(0, 0)
	timer := startTimerWithClock(func() time.Time { return clock })
	clock = clock.Add(time.Second)
	if got := timer.ObserveSummary(s); got != time.Second {
		t.Errorf("elapsed = %v, want 1s", got)
	}
}

func TestTimer_Elapsed_DoesNotRecord(t *testing.T) {
	r := newTestRegistry(t)
	h, err := r.NewHistogram(MetricDef{Name: "kscore_te_seconds", Help: "h"})
	if err != nil {
		t.Fatal(err)
	}
	clock := time.Unix(0, 0)
	timer := startTimerWithClock(func() time.Time { return clock })
	clock = clock.Add(time.Second)
	_ = timer.Elapsed()
	_ = h // not used; Elapsed must not record on its own.

	mf := gather(t, r, "kscore_te_seconds")
	if m := sampleByLabels(mf, Labels{}); m != nil && m.GetHistogram().GetSampleCount() > 0 {
		t.Errorf("Elapsed should not record; got %d samples", m.GetHistogram().GetSampleCount())
	}
}

func TestStartTimer_WallClock(t *testing.T) {
	timer := StartTimer()
	time.Sleep(5 * time.Millisecond)
	if e := timer.Elapsed(); e < time.Millisecond {
		t.Errorf("Elapsed = %v, want at least 1ms", e)
	}
}

func TestCardinalityLimiter_DropMode_BlocksObservation(t *testing.T) {
	r := NewRegistry(Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultMaxCardinality:    2,
		DisableRuntimeCollectors: true,
	})
	c, err := r.NewCounter(MetricDef{
		Name: "kscore_lim_total", Help: "h", Labels: []string{"k"},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.With(Labels{"k": "a"}).Inc()
	c.With(Labels{"k": "b"}).Inc()
	c.With(Labels{"k": "c"}).Inc() // should be dropped

	mf := gather(t, r, "kscore_lim_total")
	if sampleByLabels(mf, Labels{"k": "c"}) != nil {
		t.Errorf("over-cap label value k=c should not have been observed")
	}
	// Self-metric: drop count should be 1.
	cardMF := gather(t, r, CardinalityMetricName)
	m := sampleByLabels(cardMF, Labels{"metric": "kscore_lim_total", "outcome": "dropped"})
	if m == nil || m.GetCounter().GetValue() != 1 {
		t.Errorf("dropped self-metric = %v, want 1", m)
	}
	mAcc := sampleByLabels(cardMF, Labels{"metric": "kscore_lim_total", "outcome": "accepted"})
	if mAcc == nil || mAcc.GetCounter().GetValue() != 2 {
		t.Errorf("accepted self-metric = %v, want 2", mAcc)
	}
}

func TestCardinalityLimiter_AggregateMode_FoldsToOverflow(t *testing.T) {
	r := NewRegistry(Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DefaultMaxCardinality:    1,
		CardinalityMode:          cardinality.Aggregate,
		DisableRuntimeCollectors: true,
	})
	c, err := r.NewCounter(MetricDef{
		Name: "kscore_agg_total", Help: "h", Labels: []string{"k"},
	})
	if err != nil {
		t.Fatal(err)
	}
	c.With(Labels{"k": "first"}).Inc()  // fills cap
	c.With(Labels{"k": "second"}).Inc() // → _overflow
	c.With(Labels{"k": "third"}).Inc()  // → _overflow

	mf := gather(t, r, "kscore_agg_total")
	m := sampleByLabels(mf, Labels{"k": cardinality.OverflowSentinel})
	if m == nil {
		t.Fatalf("overflow bucket missing")
	}
	if got := m.GetCounter().GetValue(); got != 2 {
		t.Errorf("overflow bucket value = %v, want 2", got)
	}
}

func TestCardinalityLimiter_PerMetricCapOverride(t *testing.T) {
	r := newTestRegistry(t)
	tight, err := r.NewCounter(MetricDef{
		Name: "kscore_tight_total", Help: "h", Labels: []string{"k"}, MaxCardinality: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	tight.With(Labels{"k": "a"}).Inc()
	tight.With(Labels{"k": "b"}).Inc() // dropped

	cardMF := gather(t, r, CardinalityMetricName)
	m := sampleByLabels(cardMF, Labels{"metric": "kscore_tight_total", "outcome": "dropped"})
	if m == nil || m.GetCounter().GetValue() != 1 {
		t.Errorf("per-metric override did not take effect: %v", m)
	}
}

func TestCounter_Concurrent_RaceFree(t *testing.T) {
	r := newTestRegistry(t)
	c, err := r.NewCounter(MetricDef{
		Name: "kscore_race_total", Help: "h", Labels: []string{"agent"},
	})
	if err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for g := 0; g < 16; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			label := Labels{"agent": "a" + strconv.Itoa(gid%4)}
			for i := 0; i < 1000; i++ {
				c.With(label).Inc()
			}
		}(g)
	}
	wg.Wait()

	mf := gather(t, r, "kscore_race_total")
	total := 0.0
	for _, m := range mf.GetMetric() {
		total += m.GetCounter().GetValue()
	}
	if total != 16*1000 {
		t.Fatalf("total = %v, want 16000", total)
	}
}

func TestRegistry_Limiter_Accessor(t *testing.T) {
	r := newTestRegistry(t)
	if r.Limiter() == nil {
		t.Fatal("Limiter() returned nil")
	}
}

func TestGatherer_ReturnsRegistry(t *testing.T) {
	r := newTestRegistry(t)
	g := r.Gatherer()
	if _, ok := g.(*prometheus.Registry); !ok {
		t.Errorf("Gatherer = %T, want *prometheus.Registry", g)
	}
}
