package controlplane

import (
	"context"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"

	"go.keystone-core.io/keystone-core/internal/metrics"
	"go.keystone-core.io/keystone-core/internal/state"
)

func cpTestRegistry(t *testing.T) *metrics.Registry {
	t.Helper()
	return metrics.NewRegistry(metrics.Options{
		Logger:                   slog.New(slog.NewTextHandler(io.Discard, nil)),
		DisableRuntimeCollectors: true,
	})
}

func cpSample(t *testing.T, r *metrics.Registry, name string, want map[string]string) *dto.Metric {
	t.Helper()
	mfs, _ := r.Gatherer().Gather()
	for _, mf := range mfs {
		if mf.GetName() != name {
			continue
		}
	outer:
		for _, m := range mf.GetMetric() {
			labels := map[string]string{}
			for _, lp := range m.GetLabel() {
				labels[lp.GetName()] = lp.GetValue()
			}
			if len(labels) != len(want) {
				continue
			}
			for k, v := range want {
				if labels[k] != v {
					continue outer
				}
			}
			return m
		}
	}
	return nil
}

func TestCPMetrics_NilSafe(t *testing.T) {
	m, err := NewMetrics(nil)
	if err != nil || m != nil {
		t.Fatalf("NewMetrics(nil) = %v, %v", m, err)
	}
	m.SetAgentCounts("c", Counts{Total: 1})
	m.RecordCommand(&state.CommandRecord{}, state.CommandResult{})
}

func TestCPMetrics_SetAgentCounts(t *testing.T) {
	r := cpTestRegistry(t)
	m, _ := NewMetrics(r)
	m.SetAgentCounts("default", Counts{Total: 5, Pending: 1, Connected: 3, Stale: 1, Disabled: 0})

	if s := cpSample(t, r, metrics.DefAgentsTotal.Name, map[string]string{"cluster": "default", "status": "connected"}); s.GetGauge().GetValue() != 3 {
		t.Errorf("connected = %v, want 3", s.GetGauge().GetValue())
	}
	if s := cpSample(t, r, metrics.DefAgentsTotal.Name, map[string]string{"cluster": "default", "status": "stale"}); s.GetGauge().GetValue() != 1 {
		t.Errorf("stale = %v, want 1", s.GetGauge().GetValue())
	}
	// Empty cluster falls back to _unspecified.
	m.SetAgentCounts("", Counts{Connected: 2})
	if s := cpSample(t, r, metrics.DefAgentsTotal.Name, map[string]string{"cluster": "_unspecified", "status": "connected"}); s.GetGauge().GetValue() != 2 {
		t.Errorf("unspecified connected = %v, want 2", s.GetGauge().GetValue())
	}
}

func TestCPMetrics_RecordCommand(t *testing.T) {
	r := cpTestRegistry(t)
	m, _ := NewMetrics(r)
	start := time.Unix(1000, 0)
	end := start.Add(2 * time.Second)
	rec := &state.CommandRecord{ID: "c1", AgentID: "a1", Command: "exec", StartedAt: start}
	m.RecordCommand(rec, state.CommandResult{Status: state.CommandStatusCompleted, CompletedAt: end})
	m.RecordCommand(rec, state.CommandResult{Status: state.CommandStatusFailed, CompletedAt: end})

	if s := cpSample(t, r, metrics.DefCommandsExecutedTotal.Name, map[string]string{"status": string(state.CommandStatusCompleted), "agent": "a1"}); s.GetCounter().GetValue() != 1 {
		t.Errorf("succeeded.a1 = %v, want 1", s.GetCounter().GetValue())
	}
	if s := cpSample(t, r, metrics.DefCommandsExecutedTotal.Name, map[string]string{"status": string(state.CommandStatusFailed), "agent": "a1"}); s.GetCounter().GetValue() != 1 {
		t.Errorf("failed.a1 = %v, want 1", s.GetCounter().GetValue())
	}
	// Histogram: two 2-second observations.
	if s := cpSample(t, r, metrics.DefCommandDurationSeconds.Name, map[string]string{"type": "exec"}); s.GetHistogram().GetSampleCount() != 2 {
		t.Errorf("histogram count = %d, want 2", s.GetHistogram().GetSampleCount())
	}
}

func TestCPMetrics_RecordCommand_NoDuration_When_TimestampsMissing(t *testing.T) {
	r := cpTestRegistry(t)
	m, _ := NewMetrics(r)
	rec := &state.CommandRecord{ID: "c1", AgentID: "a1", Command: "exec"} // no StartedAt
	m.RecordCommand(rec, state.CommandResult{Status: state.CommandStatusCompleted})
	if s := cpSample(t, r, metrics.DefCommandDurationSeconds.Name, map[string]string{"type": "exec"}); s != nil && s.GetHistogram().GetSampleCount() > 0 {
		t.Errorf("histogram should be empty when timestamps missing, got %d samples", s.GetHistogram().GetSampleCount())
	}
}

func TestChainTerminalCommandFuncs(t *testing.T) {
	var a, b atomic.Int64
	f := ChainTerminalCommandFuncs(
		nil,
		func(context.Context, string, *state.CommandRecord, state.CommandResult) { a.Add(1) },
		func(context.Context, string, *state.CommandRecord, state.CommandResult) { b.Add(1) },
		nil,
	)
	f(context.Background(), "p", &state.CommandRecord{}, state.CommandResult{})
	if a.Load() != 1 || b.Load() != 1 {
		t.Errorf("a=%d, b=%d; want both 1", a.Load(), b.Load())
	}

	// All-nil chain returns nil.
	if got := ChainTerminalCommandFuncs(nil, nil); got != nil {
		t.Errorf("all-nil chain = %v, want nil", got)
	}
}

type stubCounts struct{ c Counts }

func (s stubCounts) Counts() Counts { return s.c }

func TestRefreshAgentCounts_Ticks(t *testing.T) {
	r := cpTestRegistry(t)
	m, _ := NewMetrics(r)
	cm := stubCounts{c: Counts{Connected: 7}}
	ctx, cancel := context.WithCancel(context.Background())
	stop := RefreshAgentCounts(ctx, m, cm, "default", 5*time.Millisecond)
	defer stop()
	defer cancel()
	// Wait briefly for the first tick.
	time.Sleep(20 * time.Millisecond)
	if s := cpSample(t, r, metrics.DefAgentsTotal.Name, map[string]string{"cluster": "default", "status": "connected"}); s.GetGauge().GetValue() != 7 {
		t.Errorf("connected after tick = %v, want 7", s.GetGauge().GetValue())
	}
}

func TestRefreshAgentCounts_NilSafe(t *testing.T) {
	stop := RefreshAgentCounts(context.Background(), nil, stubCounts{}, "c", time.Millisecond)
	stop() // must not panic
}
