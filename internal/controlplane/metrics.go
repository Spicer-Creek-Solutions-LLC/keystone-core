package controlplane

import (
	"context"
	"fmt"
	"time"

	"go.keystone-core.io/keystone-core/internal/metrics"
	"go.keystone-core.io/keystone-core/internal/state"
)

// Metrics is the controlplane-package emitter for v1.0 agent + command
// observability. Nil-safe.
type Metrics struct {
	agents          metrics.Gauge
	commandsTotal   metrics.Counter
	commandDuration metrics.Histogram
}

// NewMetrics registers the three controlplane metrics against r.
func NewMetrics(r *metrics.Registry) (*Metrics, error) {
	if r == nil {
		return nil, nil
	}
	agents, err := r.NewGauge(metrics.DefAgentsTotal)
	if err != nil {
		return nil, fmt.Errorf("controlplane: register agents_total: %w", err)
	}
	commandsTotal, err := r.NewCounter(metrics.DefCommandsExecutedTotal)
	if err != nil {
		return nil, fmt.Errorf("controlplane: register commands_executed_total: %w", err)
	}
	commandDuration, err := r.NewHistogram(metrics.DefCommandDurationSeconds)
	if err != nil {
		return nil, fmt.Errorf("controlplane: register command_duration_seconds: %w", err)
	}
	return &Metrics{
		agents:          agents,
		commandsTotal:   commandsTotal,
		commandDuration: commandDuration,
	}, nil
}

// SetAgentCounts publishes the per-status gauge from a ConnectionManager
// Counts snapshot. cluster is the operator-configured cluster name (used
// as the cluster label); typically constant for a given process.
func (m *Metrics) SetAgentCounts(cluster string, c Counts) {
	if m == nil {
		return
	}
	if cluster == "" {
		cluster = "_unspecified"
	}
	set := func(status string, value int) {
		m.agents.With(metrics.Labels{"cluster": cluster, "status": status}).Set(float64(value))
	}
	set("pending", c.Pending)
	set("connected", c.Connected)
	set("stale", c.Stale)
	set("disabled", c.Disabled)
}

// RecordCommand observes a single terminated command: the counter
// increments labelled by status + agent, and the histogram records the
// wall-clock duration. rec is the persisted CommandRecord; result is
// the terminal result delivered to OnCommandTerminal.
func (m *Metrics) RecordCommand(rec *state.CommandRecord, result state.CommandResult) {
	if m == nil || rec == nil {
		return
	}
	agent := rec.AgentID
	if agent == "" {
		agent = "_unassigned"
	}
	cmdType := rec.Command
	if cmdType == "" {
		cmdType = "_unspecified"
	}
	m.commandsTotal.With(metrics.Labels{
		"status": string(result.Status),
		"agent":  agent,
	}).Inc()
	if !rec.StartedAt.IsZero() && !result.CompletedAt.IsZero() && result.CompletedAt.After(rec.StartedAt) {
		dur := result.CompletedAt.Sub(rec.StartedAt)
		m.commandDuration.With(metrics.Labels{"type": cmdType}).Observe(dur.Seconds())
	}
}

// MetricsTerminalCommandFunc returns a TerminalCommandFunc that records
// command completion into m. Wire as DispatcherConfig.OnCommandTerminal
// alongside (or wrapping) any audit emitter.
func MetricsTerminalCommandFunc(m *Metrics) TerminalCommandFunc {
	return func(_ context.Context, _ string, rec *state.CommandRecord, result state.CommandResult) {
		m.RecordCommand(rec, result)
	}
}

// ChainTerminalCommandFuncs returns a TerminalCommandFunc that invokes
// each non-nil fn in order. Used to compose metrics emission with the
// existing audit hook without forcing callers to choose one or the
// other.
func ChainTerminalCommandFuncs(fns ...TerminalCommandFunc) TerminalCommandFunc {
	live := make([]TerminalCommandFunc, 0, len(fns))
	for _, fn := range fns {
		if fn != nil {
			live = append(live, fn)
		}
	}
	if len(live) == 0 {
		return nil
	}
	return func(ctx context.Context, principal string, rec *state.CommandRecord, result state.CommandResult) {
		for _, fn := range live {
			fn(ctx, principal, rec, result)
		}
	}
}

// AgentCountsRefresher exposes the seam from ConnectionManager that the
// metrics ticker needs.
type AgentCountsRefresher interface {
	Counts() Counts
}

// RefreshAgentCounts samples cm at the given interval (defaults to 10s
// if non-positive) and pushes each snapshot into m.SetAgentCounts. Runs
// until ctx is cancelled. Returns a stop function the caller can call
// directly; calling stop is equivalent to cancelling ctx.
func RefreshAgentCounts(ctx context.Context, m *Metrics, cm AgentCountsRefresher, cluster string, interval time.Duration) func() {
	if m == nil || cm == nil {
		return func() {}
	}
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ctx, cancel := context.WithCancel(ctx)
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		m.SetAgentCounts(cluster, cm.Counts())
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				m.SetAgentCounts(cluster, cm.Counts())
			}
		}
	}()
	return cancel
}
