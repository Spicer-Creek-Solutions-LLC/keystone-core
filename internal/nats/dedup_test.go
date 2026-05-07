package nats

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
)

func defaultDedupConfig() config.DedupConfig {
	return config.DedupConfig{
		Enabled:         true,
		WindowDuration:  5 * time.Minute,
		MaxEntries:      1000,
		CleanupInterval: time.Hour, // long; tests drive cleanup manually via purgeExpired
	}
}

func newTestDedup(t *testing.T, cfg config.DedupConfig) *Dedup {
	t.Helper()
	d := NewDedup(cfg, testLogger())
	if d == nil {
		t.Fatal("NewDedup returned nil for enabled config")
	}
	return d
}

func TestNewDedup_DisabledReturnsNil(t *testing.T) {
	cfg := defaultDedupConfig()
	cfg.Enabled = false
	if d := NewDedup(cfg, testLogger()); d != nil {
		t.Errorf("NewDedup(disabled) = %v, want nil", d)
	}
}

func TestDedup_NilSafety(t *testing.T) {
	var d *Dedup
	// All methods are nil-safe so Manager doesn't have to nil-check
	// at every call site.
	if d.IsDuplicate("kscore.test.x", "id-1") {
		t.Error("nil Dedup.IsDuplicate = true, want false")
	}
	d.Record("kscore.test.x", "id-1") // must not panic
	d.Start()
	d.Stop()
	if got := d.Size(); got != 0 {
		t.Errorf("nil Dedup.Size = %d, want 0", got)
	}
}

func TestDedup_RecordThenIsDuplicate(t *testing.T) {
	d := newTestDedup(t, defaultDedupConfig())
	if d.IsDuplicate("kscore.test.cmd", "id-1") {
		t.Error("first IsDuplicate = true, want false")
	}
	d.Record("kscore.test.cmd", "id-1")
	if !d.IsDuplicate("kscore.test.cmd", "id-1") {
		t.Error("after Record, IsDuplicate = false, want true")
	}
	if got := d.Size(); got != 1 {
		t.Errorf("Size = %d, want 1", got)
	}
}

func TestDedup_RerecordRefreshesExpiry(t *testing.T) {
	d := newTestDedup(t, defaultDedupConfig())
	clock := newDedupClock(t, d)

	d.Record("kscore.test.cmd", "id-1")
	clock.Advance(4 * time.Minute)
	d.Record("kscore.test.cmd", "id-1") // bumps expiry forward
	clock.Advance(4 * time.Minute)      // T=8m: original expiry was T=5m
	if !d.IsDuplicate("kscore.test.cmd", "id-1") {
		t.Error("re-recorded entry expired prematurely")
	}
}

func TestDedup_DifferentSubjectsDoNotCollide(t *testing.T) {
	d := newTestDedup(t, defaultDedupConfig())
	d.Record("kscore.test.cmd", "id-1")
	if d.IsDuplicate("kscore.test.heartbeat", "id-1") {
		t.Error("same MessageID on different subject reported as duplicate")
	}
}

func TestDedup_WindowExpiry(t *testing.T) {
	d := newTestDedup(t, defaultDedupConfig())
	clock := newDedupClock(t, d)

	d.Record("kscore.test.cmd", "id-1")
	clock.Advance(5 * time.Minute) // expiry is exclusive: T=expiry → expired
	if d.IsDuplicate("kscore.test.cmd", "id-1") {
		t.Error("entry past window still reported as duplicate")
	}
}

func TestDedup_PerSubjectOverrideShorterWindow(t *testing.T) {
	cfg := defaultDedupConfig()
	cfg.PerSubjectOverrides = []config.SubjectOverride{
		{Prefix: "kscore.test.heartbeat", WindowDuration: time.Minute},
	}
	d := newTestDedup(t, cfg)
	clock := newDedupClock(t, d)

	d.Record("kscore.test.heartbeat", "hb-1") // 1m window via override
	d.Record("kscore.test.cmd", "cmd-1")      // 5m default
	clock.Advance(2 * time.Minute)
	if d.IsDuplicate("kscore.test.heartbeat", "hb-1") {
		t.Error("heartbeat (1m override) still flagged duplicate at T=2m")
	}
	if !d.IsDuplicate("kscore.test.cmd", "cmd-1") {
		t.Error("command (5m default) expired prematurely at T=2m")
	}
}

func TestDedup_PerSubjectOverrideLongestPrefixWins(t *testing.T) {
	cfg := defaultDedupConfig()
	cfg.PerSubjectOverrides = []config.SubjectOverride{
		{Prefix: "kscore.test.", WindowDuration: 10 * time.Minute},
		{Prefix: "kscore.test.heartbeat", WindowDuration: time.Minute},
	}
	d := newTestDedup(t, cfg)
	clock := newDedupClock(t, d)

	d.Record("kscore.test.heartbeat", "hb-1")
	clock.Advance(2 * time.Minute)
	// "kscore.test.heartbeat" is the longer prefix → 1m window applies.
	if d.IsDuplicate("kscore.test.heartbeat", "hb-1") {
		t.Error("longest-prefix override (1m) was overridden by shorter (10m)")
	}
}

func TestDedup_MaxEntriesEvictsOldest(t *testing.T) {
	cfg := defaultDedupConfig()
	cfg.MaxEntries = 3
	d := newTestDedup(t, cfg)

	d.Record("kscore.test.s", "id-1")
	d.Record("kscore.test.s", "id-2")
	d.Record("kscore.test.s", "id-3")
	d.Record("kscore.test.s", "id-4") // evicts id-1

	if d.IsDuplicate("kscore.test.s", "id-1") {
		t.Error("evicted id-1 still reported as duplicate")
	}
	if !d.IsDuplicate("kscore.test.s", "id-2") {
		t.Error("id-2 should still be present")
	}
	if !d.IsDuplicate("kscore.test.s", "id-4") {
		t.Error("most-recent id-4 should be present")
	}
	if got := d.Size(); got != cfg.MaxEntries {
		t.Errorf("Size = %d, want %d", got, cfg.MaxEntries)
	}
}

func TestDedup_PurgeExpiredClearsStale(t *testing.T) {
	d := newTestDedup(t, defaultDedupConfig())
	clock := newDedupClock(t, d)

	d.Record("kscore.test.s", "id-1")
	d.Record("kscore.test.s", "id-2")
	clock.Advance(6 * time.Minute) // both past window
	d.purgeExpired()
	if got := d.Size(); got != 0 {
		t.Errorf("Size after purge = %d, want 0", got)
	}
}

func TestDedup_PurgeKeepsLiveEntries(t *testing.T) {
	d := newTestDedup(t, defaultDedupConfig())
	clock := newDedupClock(t, d)

	d.Record("kscore.test.s", "id-1") // T=0, expiry T=5m
	clock.Advance(3 * time.Minute)
	d.Record("kscore.test.s", "id-2") // T=3m, expiry T=8m
	clock.Advance(3 * time.Minute)    // T=6m → id-1 expired, id-2 live
	d.purgeExpired()
	if d.IsDuplicate("kscore.test.s", "id-1") {
		t.Error("id-1 should have been purged")
	}
	if !d.IsDuplicate("kscore.test.s", "id-2") {
		t.Error("id-2 should have been kept")
	}
}

func TestDedup_StartStopIdempotent(t *testing.T) {
	d := newTestDedup(t, defaultDedupConfig())
	d.Start()
	d.Start() // idempotent — second call must not start a second goroutine
	d.Stop()
	d.Stop() // idempotent
}

func TestDedup_StopBeforeStart(t *testing.T) {
	d := newTestDedup(t, defaultDedupConfig())
	d.Stop() // must not panic / block
}

func TestDedup_ConcurrentAccess(t *testing.T) {
	cfg := defaultDedupConfig()
	cfg.MaxEntries = 10_000 // headroom for goroutines*iterations below
	d := newTestDedup(t, cfg)

	const goroutines = 8
	const iterations = 200
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		gid := g
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				subject := "kscore.test.s"
				id := uniqueID(gid, i)
				if !d.IsDuplicate(subject, id) {
					d.Record(subject, id)
				}
			}
		}()
	}
	wg.Wait()
	// All IDs are unique across goroutines so every call records a
	// fresh entry. Size should equal goroutines*iterations.
	if got, want := d.Size(), goroutines*iterations; got != want {
		t.Errorf("Size = %d, want %d", got, want)
	}
}

func TestDedupKey_LengthPrefixedDisambiguation(t *testing.T) {
	// Constructing two distinct (subject, messageID) pairs that would
	// collide under naive concatenation with a "\x00" separator.
	// With length prefixing, their hashes must differ.
	a := dedupKey("kscore.test", "x\x00y")
	b := dedupKey("kscore.test\x00x", "y")
	if a == b {
		t.Error("length-prefixed hash collided across (subject, messageID) boundary")
	}
}

func TestDedupKey_DeterministicAcrossCalls(t *testing.T) {
	a := dedupKey("kscore.test.cmd", "id-1")
	b := dedupKey("kscore.test.cmd", "id-1")
	if a != b {
		t.Error("dedupKey not deterministic")
	}
}

// dedupClock is a manually-advanced fake clock for window-expiry
// tests. Drives Dedup.now via a function pointer so tests don't need
// to sleep.
type dedupClock struct {
	t   *testing.T
	mu  sync.Mutex
	cur time.Time
}

func newDedupClock(t *testing.T, d *Dedup) *dedupClock {
	t.Helper()
	c := &dedupClock{t: t, cur: time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)}
	d.mu.Lock()
	d.now = c.Now
	d.mu.Unlock()
	return c
}

func (c *dedupClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.cur
}

func (c *dedupClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cur = c.cur.Add(d)
}

// uniqueID builds a stable, distinct ID per (goroutine, iteration).
// Used by the concurrent-access test to avoid relying on uuid.
var uniqueIDCounter atomic.Int64

func uniqueID(g, i int) string {
	_ = g
	_ = i
	n := uniqueIDCounter.Add(1)
	return time.Now().Format("ID-1504050000-") + atomicToString(n)
}

func atomicToString(n int64) string {
	if n == 0 {
		return "0"
	}
	const digits = "0123456789"
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = digits[n%10]
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
