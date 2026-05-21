//go:build slo

// Package perf is the Epic 19 task 3 performance-SLO suite. It
// complements test/e2e/ha/slo_test.go (Epic 13 task 18, the HA/
// cluster-formation SLOs) by asserting the three throughput +
// latency SLOs the operator-facing v1.0 release commits to:
//
//   - Single-agent command latency < 100 ms (local NATS)
//   - 1000-event emission throughput > 10k events/s
//   - 100-batch command exec across 10 agents < 2 s
//
// Build-tagged `slo` so the suite runs alongside the HA SLOs under
// `make slo` (no -race; race instrumentation inflates wall-clock
// 2-10× and would make the asserted numbers meaningless — the
// in-`-race` functional smoke lives in the per-domain integration
// tests).
//
// Harness shape: in-process embedded NATS + in-process control-plane
// dispatcher/response-router + in-process agent runtime(s). Docker-
// compose is deliberately avoided — bridge NAT adds 1-5 ms per RPC,
// which is 5% of the 100 ms command-latency SLO. The cost we want to
// measure is the mechanism cost, not the container-orchestration
// cost.
package perf

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"runtime"
	"sort"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/agent"
	"go.keystone-core.io/keystone-core/internal/config"
	"go.keystone-core.io/keystone-core/internal/controlplane"
	natsmgr "go.keystone-core.io/keystone-core/internal/nats"
	"go.keystone-core.io/keystone-core/internal/state"
	"go.keystone-core.io/keystone-core/pkg/envelope"
)

// silentLogger discards everything below ERROR — keeps SLO test
// output clean so the only thing in `go test -v` is the measured
// numbers.
func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelError}))
}

// pickFreePort grabs an ephemeral TCP port and immediately closes
// the listener. The embedded NATS server is then asked to bind it.
// There's a brief race window between Close and Listen by the NATS
// server, but it's acceptable for tests.
func pickFreePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	if err := l.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	return port
}

// stubCommandStore is the minimal CommandStore the dispatcher needs.
// SLO tests don't care about the commands table — they measure the
// wire-roundtrip cost.
type stubCommandStore struct{}

func (stubCommandStore) CreateCommand(_ context.Context, _ *state.CommandRecord) error { return nil }
func (stubCommandStore) GetCommand(_ context.Context, _ string) (*state.CommandRecord, error) {
	return nil, errors.New("stub: not implemented")
}
func (stubCommandStore) ListCommands(_ context.Context, _ state.CommandFilter) ([]*state.CommandRecord, error) {
	return nil, nil
}
func (stubCommandStore) UpdateCommandResult(_ context.Context, _ string, _ state.CommandResult) error {
	return nil
}
func (stubCommandStore) DeleteCommandsBefore(_ context.Context, _ time.Time, _ []state.CommandStatus) (int, error) {
	return 0, nil
}

// stubAgentLookup answers Get for any known agent ID. SLO tests
// preregister their agents in the cache so the dispatcher's pre-
// dispatch lookup passes without touching ConnectionManager.
type stubAgentLookup struct {
	ids map[string]struct{}
}

func newStubAgentLookup(ids ...string) stubAgentLookup {
	s := stubAgentLookup{ids: map[string]struct{}{}}
	for _, id := range ids {
		s.ids[id] = struct{}{}
	}
	return s
}

func (s stubAgentLookup) Get(_ context.Context, id string) (*state.AgentRecord, error) {
	if _, ok := s.ids[id]; !ok {
		return nil, errors.New("stub: unknown agent " + id)
	}
	return &state.AgentRecord{
		ID:           id,
		Status:       state.AgentStatusConnected,
		RegisteredAt: time.Now(),
	}, nil
}

// natsPublisherAdapter satisfies controlplane.NATSPublisher.
type natsPublisherAdapter struct{ m *natsmgr.Manager }

func (a natsPublisherAdapter) PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error {
	return a.m.PublishEnvelope(ctx, subject, env)
}

// natsSubscriberAdapter satisfies controlplane.Subscriber.
type natsSubscriberAdapter struct{ m *natsmgr.Manager }

func (a natsSubscriberAdapter) Subscribe(subject string, h controlplane.MessageHandler) (controlplane.Subscription, error) {
	sub, err := a.m.Subscribe(subject, natsmgr.MessageHandler(h))
	if err != nil {
		return nil, err
	}
	return sub, nil
}

// agentNATSAdapter satisfies agent.NATSClient.
type agentNATSAdapter struct{ m *natsmgr.Manager }

func (a agentNATSAdapter) PublishEnvelope(ctx context.Context, subject string, env envelope.Envelope) error {
	return a.m.PublishEnvelope(ctx, subject, env)
}

func (a agentNATSAdapter) Subscribe(subject string, h agent.MessageHandler) (agent.Subscription, error) {
	sub, err := a.m.Subscribe(subject, natsmgr.MessageHandler(h))
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func (a agentNATSAdapter) Health(ctx context.Context) error { return a.m.Health(ctx) }

// commandSignerAdapter adapts agent.SecurityEnforcer to the
// controlplane.Signer interface. Mirrors cmd/kscore-server's
// production adapter; reproduced here so the test stays free of
// cross-binary imports.
type commandSignerAdapter struct{ enf *agent.SecurityEnforcer }

func (a commandSignerAdapter) SignCommand(msg controlplane.CommandMessage) string {
	return a.enf.ComputeHMAC(agent.CommandRequest{
		MessageID:      msg.MessageID,
		Principal:      msg.Principal,
		Command:        msg.Command,
		Args:           msg.Args,
		Env:            msg.Env,
		WorkingDir:     msg.WorkingDir,
		User:           msg.User,
		TimeoutSeconds: msg.TimeoutSeconds,
	})
}

// embeddedNATSConfig is the shared embedded-server config the perf
// harness boots. JetStream is enabled because SLO B exercises the
// JetStream publisher path explicitly; SLO A/C use core NATS only
// but the server handles both.
func embeddedNATSConfig(t *testing.T, clusterName string, jsStoreDir string) config.NATSConfig {
	t.Helper()
	cfg := config.NATSConfig{
		Mode:              config.NATSModeEmbedded,
		ClusterName:       clusterName,
		MaxReconnects:     1,
		ReconnectWait:     50 * time.Millisecond,
		MaxReconnectDelay: 200 * time.Millisecond,
		ReconnectJitter:   0.1,
		Embedded: config.EmbeddedNATSConfig{
			Host: "127.0.0.1",
			Port: pickFreePort(t),
		},
		Dedup:          config.DedupConfig{Enabled: false},
		CircuitBreaker: config.CircuitBreakerConfig{Enabled: false},
	}
	if jsStoreDir != "" {
		cfg.JetStream = config.JetStreamConfig{
			Enabled:        true,
			StoreDir:       jsStoreDir,
			StreamMaxAge:   time.Hour,
			StreamMaxBytes: 64 << 20,
			StreamMaxMsgs:  100_000,
			StreamReplicas: 1,
		}
	} else {
		cfg.JetStream = config.JetStreamConfig{Enabled: false}
	}
	return cfg
}

// externalNATSConfig is the agent-side config pointing at the
// already-started embedded server's clientURL. Same fixture used by
// cmd/kscore-server/integration_test.go.
func externalNATSConfig(clusterName, url string) config.NATSConfig {
	return config.NATSConfig{
		Mode:              config.NATSModeExternal,
		URLs:              []string{url},
		ClusterName:       clusterName,
		MaxReconnects:     -1,
		ReconnectWait:     50 * time.Millisecond,
		MaxReconnectDelay: 200 * time.Millisecond,
		ReconnectJitter:   0.1,
		JetStream:         config.JetStreamConfig{Enabled: false},
		Embedded:          config.EmbeddedNATSConfig{Host: "127.0.0.1", Port: 4222},
		Dedup:             config.DedupConfig{Enabled: false},
		CircuitBreaker:    config.CircuitBreakerConfig{Enabled: false},
	}
}

// assertWithin logs the measured value and fails if it's outside the
// SLO bound. Mirrors test/e2e/ha/slo_test.go's helper; reproduced
// here for a self-contained package.
func assertWithin(t *testing.T, what string, got, bound time.Duration) {
	t.Helper()
	t.Logf("SLO %-32s measured=%-12s bound=%s", what, got.Round(time.Microsecond), bound)
	if got > bound {
		t.Fatalf("SLO VIOLATION: %s took %s, bound %s", what, got, bound)
	}
}

// assertThroughputAbove logs and fails if events/sec is below the
// floor. Used by SLO B.
func assertThroughputAbove(t *testing.T, what string, eventsPerSec, floor float64) {
	t.Helper()
	t.Logf("SLO %-32s measured=%-12.0f floor=%.0f events/s", what, eventsPerSec, floor)
	if eventsPerSec < floor {
		t.Fatalf("SLO VIOLATION: %s = %.0f events/s, want > %.0f", what, eventsPerSec, floor)
	}
}

// percentile returns the p (0..1) percentile of durations. Caller
// must pass a *sorted* slice — perf paths sort once and reuse.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}

// summarize sorts the durations in place and returns p50/p95/max.
func summarize(d []time.Duration) (p50, p95, mx time.Duration) {
	sort.Slice(d, func(i, j int) bool { return d[i] < d[j] })
	if len(d) == 0 {
		return
	}
	p50 = percentile(d, 0.50)
	p95 = percentile(d, 0.95)
	mx = d[len(d)-1]
	return
}

// requireLinux skips on non-linux because the SLO A test exec's
// /bin/echo via the real agent Executor.
func requireLinux(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("perf SLOs: linux only (uses /bin/echo via the real agent Executor)")
	}
}
