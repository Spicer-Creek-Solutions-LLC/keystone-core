package proxy

import (
	"context"
	"net"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	dto "github.com/prometheus/client_model/go"

	"go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/files/backend"
	"go.keystone-core.io/keystone-core/internal/files/transport"
	"go.keystone-core.io/keystone-core/internal/metrics"
	natspkg "go.keystone-core.io/keystone-core/internal/nats"
)

// TestCache_RoundTrip_OverNATS_IncrementsHitMetric is the acceptance-
// criteria proof for Epic 18 line "Proxy cache hit on second download
// (visible in metrics)". The rig wires:
//
//	MemoryStore  ←  transport.Service  ←  NATS  →  transport.Client  →  Cache
//
// and verifies that the first Get triggers a miss + populates the
// cache, the second Get hits without invoking the source, and the
// counter values surface through the metrics registry.
func TestCache_RoundTrip_OverNATS_IncrementsHitMetric(t *testing.T) {
	r := newIntegRig(t)
	ctx := context.Background()

	body := []byte("the brown fox")
	if _, err := r.client.Put(ctx, files.FileMetadata{Path: "p"}, body); err != nil {
		t.Fatalf("Put: %v", err)
	}

	// First Get is a miss — populates the cache.
	meta1, body1, err := r.cache.Get(ctx, "p", transport.GetOptions{})
	if err != nil {
		t.Fatalf("Get 1: %v", err)
	}
	if string(body1) != string(body) {
		t.Errorf("body 1 mismatch: %q vs %q", body1, body)
	}

	// Second Get must hit — assert via cache.Len + metric counter.
	meta2, body2, err := r.cache.Get(ctx, "p", transport.GetOptions{})
	if err != nil {
		t.Fatalf("Get 2: %v", err)
	}
	if string(body2) != string(body) {
		t.Errorf("body 2 mismatch")
	}
	if meta1.Path != meta2.Path || meta1.Hash != meta2.Hash {
		t.Errorf("metadata differed between hit and miss: %+v vs %+v", meta1, meta2)
	}

	hits := gatherCounter(t, r.registry, "kscore_files_cache_hits_total", nil)
	misses := gatherCounter(t, r.registry, "kscore_files_cache_misses_total", map[string]string{"reason": "miss"})
	if hits != 1 {
		t.Errorf("hits = %v, want 1", hits)
	}
	if misses != 1 {
		t.Errorf("misses{reason=miss} = %v, want 1", misses)
	}
}

func TestCache_BypassMetric_OverNATS(t *testing.T) {
	r := newIntegRig(t)
	ctx := context.Background()

	// Put a body large enough to have multiple chunks so a
	// from_chunk=1 resume is a valid call.
	body := make([]byte, files.ChunkSize+128)
	for i := range body {
		body[i] = byte(i % 200)
	}
	if _, err := r.client.Put(ctx, files.FileMetadata{Path: "big"}, body); err != nil {
		t.Fatalf("Put: %v", err)
	}

	if _, _, err := r.cache.Get(ctx, "big", transport.GetOptions{FromChunk: 1}); err != nil {
		t.Fatalf("bypass Get: %v", err)
	}

	bypass := gatherCounter(t, r.registry, "kscore_files_cache_misses_total", map[string]string{"reason": "bypass"})
	if bypass != 1 {
		t.Errorf("misses{reason=bypass} = %v, want 1", bypass)
	}
	if r.cache.Len() != 0 {
		t.Errorf("bypass populated cache: len = %d", r.cache.Len())
	}
}

func TestCache_ExpiredMetric_OverNATS(t *testing.T) {
	r := newIntegRig(t)
	ctx := context.Background()

	// Replace the default cache with one wired to a manual clock so
	// we can step past the TTL without sleeping.
	var now time.Time
	clock := func() time.Time { return now }
	cache, err := New(r.client, Config{Capacity: 10, TTL: 100 * time.Millisecond}, r.cacheMetrics, clock)
	if err != nil {
		t.Fatalf("New cache with clock: %v", err)
	}

	body := []byte("v")
	if _, err := r.client.Put(ctx, files.FileMetadata{Path: "p"}, body); err != nil {
		t.Fatal(err)
	}

	now = time.Date(2026, 5, 21, 12, 0, 0, 0, time.UTC)
	if _, _, err := cache.Get(ctx, "p", transport.GetOptions{}); err != nil {
		t.Fatal(err)
	}

	// Step past TTL.
	now = now.Add(200 * time.Millisecond)
	if _, _, err := cache.Get(ctx, "p", transport.GetOptions{}); err != nil {
		t.Fatal(err)
	}

	expired := gatherCounter(t, r.registry, "kscore_files_cache_misses_total", map[string]string{"reason": "expired"})
	if expired != 1 {
		t.Errorf("misses{reason=expired} = %v, want 1", expired)
	}
}

// --- rig ---------------------------------------------------------------------

type integRig struct {
	srv          *natsserver.Server
	conn         *nats.Conn
	svc          *transport.Service
	client       *transport.Client
	registry     *metrics.Registry
	cacheMetrics *Metrics
	cache        *Cache
}

func newIntegRig(t *testing.T) *integRig {
	t.Helper()

	opts := &natsserver.Options{
		Host:       "127.0.0.1",
		Port:       freePort(t),
		NoSigs:     true,
		NoLog:      true,
		MaxPayload: 4 * 1024 * 1024,
	}
	srv, err := natsserver.NewServer(opts)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	go srv.Start()
	if !srv.ReadyForConnections(5 * time.Second) {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal("embedded NATS not ready")
	}

	conn, err := nats.Connect(srv.ClientURL())
	if err != nil {
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatalf("connect: %v", err)
	}

	subjects, err := natspkg.NewSubjectBuilder("test")
	if err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal(err)
	}

	store := backend.NewMemoryStore(nil)
	svc, err := transport.NewService(conn, subjects, store, nil)
	if err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal(err)
	}
	if err := svc.Start(context.Background()); err != nil {
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal(err)
	}

	client, err := transport.NewClient(conn, subjects)
	if err != nil {
		_ = svc.Stop()
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal(err)
	}

	registry := metrics.NewRegistry(metrics.Options{})
	m, err := NewMetrics(registry)
	if err != nil {
		_ = svc.Stop()
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal(err)
	}
	cache, err := New(client, Config{Capacity: 16}, m, nil)
	if err != nil {
		_ = svc.Stop()
		conn.Close()
		srv.Shutdown()
		srv.WaitForShutdown()
		t.Fatal(err)
	}

	rig := &integRig{
		srv:          srv,
		conn:         conn,
		svc:          svc,
		client:       client,
		registry:     registry,
		cacheMetrics: m,
		cache:        cache,
	}
	t.Cleanup(rig.close)
	return rig
}

func (r *integRig) close() {
	_ = r.svc.Stop()
	if r.conn != nil {
		r.conn.Close()
	}
	if r.srv != nil {
		r.srv.Shutdown()
		r.srv.WaitForShutdown()
	}
}

func freePort(t *testing.T) int {
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

// gatherCounter pulls metric_name from the registry and returns the
// counter value for the matching label set. Returns 0 if no matching
// row exists (the metric was registered but never observed).
func gatherCounter(t *testing.T, r *metrics.Registry, name string, labels map[string]string) float64 {
	t.Helper()
	families, err := r.Gatherer().Gather()
	if err != nil {
		t.Fatalf("gather: %v", err)
	}
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			if labelsMatch(m, labels) {
				if c := m.GetCounter(); c != nil {
					return c.GetValue()
				}
			}
		}
	}
	if labels == nil {
		// Counter registered but never observed.
		return 0
	}
	// Print available label sets so a failing test surfaces what was actually emitted.
	for _, mf := range families {
		if mf.GetName() != name {
			continue
		}
		for _, m := range mf.GetMetric() {
			t.Logf("found %s with labels %s", name, labelString(m))
		}
	}
	return 0
}

func labelsMatch(m *dto.Metric, want map[string]string) bool {
	if len(want) == 0 {
		return true
	}
	have := make(map[string]string, len(m.GetLabel()))
	for _, lp := range m.GetLabel() {
		have[lp.GetName()] = lp.GetValue()
	}
	for k, v := range want {
		if have[k] != v {
			return false
		}
	}
	return true
}

func labelString(m *dto.Metric) string {
	var b strings.Builder
	for i, lp := range m.GetLabel() {
		if i > 0 {
			b.WriteString(",")
		}
		b.WriteString(lp.GetName())
		b.WriteString("=")
		b.WriteString(lp.GetValue())
	}
	return b.String()
}
