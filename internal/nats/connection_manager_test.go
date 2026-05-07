package nats

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	natsclient "github.com/nats-io/nats.go"

	"go.keystone-core.io/keystone-core/internal/config"
)

// startSyntheticEndpoint spins an embedded nats-server for use as a
// reachable endpoint and returns its client URL. Cleaned up via
// t.Cleanup.
func startSyntheticEndpoint(t *testing.T) string {
	t.Helper()
	m := startManager(t, embeddedConfig(t))
	url := m.ClientURL()
	if url == "" {
		t.Fatal("synthetic endpoint ClientURL is empty")
	}
	return url
}

func externalCfg(urls []string) config.NATSConfig {
	return config.NATSConfig{
		Mode:          config.NATSModeExternal,
		URLs:          urls,
		ClusterName:   "test",
		MaxReconnects: 10,
		ReconnectWait: 50 * time.Millisecond,
		JetStream:     config.JetStreamConfig{Enabled: false},
	}
}

func startConnMgr(t *testing.T, cfg config.NATSConfig) *ConnectionManager {
	t.Helper()
	cm, err := NewConnectionManager(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewConnectionManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cm.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		stopCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = cm.Shutdown(stopCtx)
	})
	return cm
}

func TestNewConnectionManager_RejectsEmbeddedMode(t *testing.T) {
	cfg := embeddedConfig(t)
	if _, err := NewConnectionManager(cfg, testLogger()); err == nil {
		t.Fatal("NewConnectionManager(embedded): expected error, got nil")
	}
}

func TestNewConnectionManager_RejectsNoEndpoints(t *testing.T) {
	cfg := config.NATSConfig{
		Mode:        config.NATSModeExternal,
		ClusterName: "test",
		JetStream:   config.JetStreamConfig{Enabled: false},
	}
	if _, err := NewConnectionManager(cfg, testLogger()); err == nil {
		t.Fatal("NewConnectionManager(no endpoints): expected error, got nil")
	}
}

func TestConnectionManager_StartHealthPublish(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))

	if err := cm.Health(context.Background()); err != nil {
		t.Errorf("Health = %v, want nil", err)
	}
	if err := cm.publishBytes(context.Background(), "kscore.test.cm", []byte("ok")); err != nil {
		t.Errorf("Publish = %v, want nil", err)
	}

	active, ok := cm.ActiveEndpoint()
	if !ok || active != url {
		t.Errorf("ActiveEndpoint = (%q, %v), want (%q, true)", active, ok, url)
	}
}

func TestConnectionManager_FailoverWhenPrimaryDownAtStart(t *testing.T) {
	// Primary is unreachable; secondary is a live synthetic endpoint.
	primary := "nats://127.0.0.1:1"
	secondary := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{primary, secondary}))

	if err := cm.Health(context.Background()); err != nil {
		t.Fatalf("Health after failover = %v", err)
	}
	active, ok := cm.ActiveEndpoint()
	if !ok || active != secondary {
		t.Errorf("ActiveEndpoint = (%q, %v), want (%q, true)", active, ok, secondary)
	}
}

func TestConnectionManager_StartFailsAllEndpointsDown(t *testing.T) {
	cfg := externalCfg([]string{"nats://127.0.0.1:1", "nats://127.0.0.1:2"})
	cfg.MaxReconnects = 0
	cm, err := NewConnectionManager(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewConnectionManager: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := cm.Start(ctx); err == nil {
		t.Fatal("Start: expected error, got nil")
		_ = cm.Shutdown(context.Background())
	} else if !strings.Contains(err.Error(), "nats:") {
		t.Errorf("err = %v, want containing 'nats:'", err)
	}

	// All endpoint states should be Failed after a failed Start.
	for _, snap := range cm.Snapshot() {
		if snap.Status != EndpointStatusFailed {
			t.Errorf("Snapshot[%s].Status = %q, want failed", snap.URL, snap.Status)
		}
		if snap.FailureCount == 0 {
			t.Errorf("Snapshot[%s].FailureCount = 0, want >=1", snap.URL)
		}
	}
}

func TestConnectionManager_SnapshotReflectsConnectedEndpoint(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))

	snaps := cm.Snapshot()
	if len(snaps) != 1 {
		t.Fatalf("Snapshot len = %d, want 1", len(snaps))
	}
	if snaps[0].URL != url {
		t.Errorf("URL = %q, want %q", snaps[0].URL, url)
	}
	if snaps[0].Status != EndpointStatusConnected {
		t.Errorf("Status = %q, want connected", snaps[0].Status)
	}
	if snaps[0].Circuit != CircuitClosed {
		t.Errorf("Circuit = %q, want closed", snaps[0].Circuit)
	}
	if snaps[0].SuccessCount == 0 {
		t.Error("SuccessCount = 0, want >=1 after connect")
	}
}

func TestConnectionManager_SnapshotOrderedByPriority(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cfg := config.NATSConfig{
		Mode:        config.NATSModeExternal,
		ClusterName: "test",
		JetStream:   config.JetStreamConfig{Enabled: false},
		Endpoints: []config.EndpointConfig{
			{URL: "nats://low:4222", Priority: 1},
			{URL: url, Priority: 100},
			{URL: "nats://mid:4222", Priority: 50},
		},
		MaxReconnects: 10,
		ReconnectWait: 50 * time.Millisecond,
	}
	cm, err := NewConnectionManager(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewConnectionManager: %v", err)
	}
	t.Cleanup(func() { _ = cm.Shutdown(context.Background()) })
	if err := cm.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}

	snaps := cm.Snapshot()
	wantOrder := []string{url, "nats://mid:4222", "nats://low:4222"}
	if len(snaps) != len(wantOrder) {
		t.Fatalf("Snapshot len = %d, want %d", len(snaps), len(wantOrder))
	}
	for i, want := range wantOrder {
		if snaps[i].URL != want {
			t.Errorf("Snapshot[%d].URL = %q, want %q", i, snaps[i].URL, want)
		}
	}
}

func TestConnectionManager_ConcurrentPublish(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))

	const goroutines = 8
	const messages = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < messages; i++ {
				if err := cm.publishBytes(context.Background(), "kscore.test.concurrent", []byte("x")); err != nil {
					t.Errorf("Publish: %v", err)
					return
				}
			}
		}()
	}
	wg.Wait()

	if err := cm.Health(context.Background()); err != nil {
		t.Errorf("Health after concurrent publish = %v", err)
	}
}

func TestConnectionManager_HealthPreStart(t *testing.T) {
	cm, err := NewConnectionManager(externalCfg([]string{"nats://x:4222"}), testLogger())
	if err != nil {
		t.Fatalf("NewConnectionManager: %v", err)
	}
	if err := cm.Health(context.Background()); err == nil {
		t.Error("Health pre-Start = nil, want error")
	}
	if err := cm.publishBytes(context.Background(), "x", []byte("y")); err == nil {
		t.Error("Publish pre-Start = nil, want error")
	}
	if _, ok := cm.ActiveEndpoint(); ok {
		t.Error("ActiveEndpoint pre-Start ok=true, want false")
	}
}

func TestConnectionManager_ShutdownIdempotent(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))
	if err := cm.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown 1: %v", err)
	}
	if err := cm.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown 2 = %v, want nil", err)
	}
	if err := cm.Health(context.Background()); err == nil {
		t.Error("Health post-Shutdown = nil, want error")
	}
	if err := cm.publishBytes(context.Background(), "x", []byte("y")); err == nil {
		t.Error("Publish post-Shutdown = nil, want error")
	}
}

func TestConnectionManager_ShutdownBeforeStart(t *testing.T) {
	cm, err := NewConnectionManager(externalCfg([]string{"nats://x:4222"}), testLogger())
	if err != nil {
		t.Fatalf("NewConnectionManager: %v", err)
	}
	if err := cm.Shutdown(context.Background()); err != nil {
		t.Errorf("Shutdown before Start = %v, want nil", err)
	}
}

func TestConnectionManager_StartIdempotent(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))
	if err := cm.Start(context.Background()); err != nil {
		t.Errorf("second Start = %v, want nil", err)
	}
}

func TestConnectionManager_PublishReflectsInSubscriber(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))

	sub, err := natsclient.Connect(url)
	if err != nil {
		t.Fatalf("subscriber connect: %v", err)
	}
	defer sub.Close()

	var (
		wg   sync.WaitGroup
		got  []byte
		once sync.Once
	)
	wg.Add(1)
	subscription, err := sub.Subscribe("kscore.test.cm.rt", func(msg *natsclient.Msg) {
		once.Do(func() {
			got = append([]byte(nil), msg.Data...)
			wg.Done()
		})
	})
	if err != nil {
		t.Fatalf("subscribe: %v", err)
	}
	defer func() { _ = subscription.Unsubscribe() }()
	if err := sub.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	if err := cm.publishBytes(context.Background(), "kscore.test.cm.rt", []byte("hello")); err != nil {
		t.Fatalf("Publish: %v", err)
	}

	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("subscriber did not receive message within 2s")
	}
	if string(got) != "hello" {
		t.Errorf("payload = %q, want hello", got)
	}
}

func TestEndpointsFromConfig_PrefersEndpointsOverURLs(t *testing.T) {
	cfg := config.NATSConfig{
		Mode: config.NATSModeExternal,
		Endpoints: []config.EndpointConfig{
			{URL: "nats://e1:4222", Priority: 5, Weight: 2, Tags: []string{"east"}},
		},
	}
	got, err := endpointsFromConfig(cfg)
	if err != nil {
		t.Fatalf("endpointsFromConfig: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].URL != "nats://e1:4222" || got[0].Priority != 5 || got[0].Weight != 2 {
		t.Errorf("endpoint = %+v", got[0])
	}
	if got[0].Scheme != "nats" {
		t.Errorf("Scheme = %q, want nats", got[0].Scheme)
	}
	if len(got[0].Tags) != 1 || got[0].Tags[0] != "east" {
		t.Errorf("Tags = %v", got[0].Tags)
	}
}

func TestConnectionManager_EndpointsCopy(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))
	eps := cm.Endpoints()
	if len(eps) != 1 || eps[0].URL != url {
		t.Fatalf("Endpoints = %+v", eps)
	}
	// Mutating the returned slice must not affect internal state.
	eps[0].URL = "mutated"
	if cm.Endpoints()[0].URL != url {
		t.Error("Endpoints() returned a shared slice; expected a copy")
	}
}

func TestConnectionManager_ProbeRecordsRTT(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))

	// Direct call into the unexported probe; the periodic ticker would
	// take rttProbeInterval (5s) to fire otherwise.
	cm.probeOnce()
	cm.probeOnce()

	for _, snap := range cm.Snapshot() {
		if snap.URL == url {
			if snap.LatencyP50 == 0 {
				t.Errorf("after probe, P50 = 0 for %q (snap=%+v)", url, snap)
			}
			return
		}
	}
	t.Fatalf("active endpoint %q not in snapshot", url)
}

func TestConnectionManager_RecordFailureUpdatesState(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))

	cm.recordFailure(url, errors.New("synthetic"))
	for _, snap := range cm.Snapshot() {
		if snap.URL == url {
			if snap.FailureCount != 1 {
				t.Errorf("FailureCount = %d, want 1", snap.FailureCount)
			}
			if snap.LastError != "synthetic" {
				t.Errorf("LastError = %q, want synthetic", snap.LastError)
			}
			return
		}
	}
	t.Fatal("endpoint missing from snapshot after recordFailure")
}

func TestConnectionManager_RecordFailureUnknownURLNoop(t *testing.T) {
	url := startSyntheticEndpoint(t)
	cm := startConnMgr(t, externalCfg([]string{url}))
	// Should not panic for an endpoint that ConnectionManager does
	// not know — Publish-path callers may pass a stale URL during
	// reconnect.
	cm.recordFailure("nats://unknown:4222", errors.New("x"))
	cm.recordSuccess("nats://unknown:4222")
}

func TestManager_ExternalEndpointSnapshots(t *testing.T) {
	url := startSyntheticEndpoint(t)
	m, err := New(externalCfg([]string{url}), testLogger())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := m.Start(context.Background()); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = m.Shutdown(context.Background()) })

	snaps := m.EndpointSnapshots()
	if len(snaps) != 1 {
		t.Fatalf("len = %d, want 1", len(snaps))
	}
	if snaps[0].URL != url {
		t.Errorf("URL = %q, want %q", snaps[0].URL, url)
	}
}

func TestManager_EmbeddedEndpointSnapshotsNil(t *testing.T) {
	m := startManager(t, embeddedConfig(t))
	if got := m.EndpointSnapshots(); got != nil {
		t.Errorf("embedded EndpointSnapshots = %+v, want nil", got)
	}
}

func TestEndpointsFromConfig_TranslatesURLs(t *testing.T) {
	cfg := config.NATSConfig{
		Mode: config.NATSModeExternal,
		URLs: []string{"nats://x:4222", "tls://y:4222"},
	}
	got, err := endpointsFromConfig(cfg)
	if err != nil {
		t.Fatalf("endpointsFromConfig: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Priority != 0 || got[0].Weight != 1 {
		t.Errorf("URL[0] defaults wrong: %+v", got[0])
	}
	if got[1].Scheme != "tls" {
		t.Errorf("URL[1] Scheme = %q, want tls", got[1].Scheme)
	}
}
