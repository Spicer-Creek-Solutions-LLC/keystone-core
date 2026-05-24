// SPDX-License-Identifier: Apache-2.0

package nats

import (
	"container/list"
	"crypto/sha256"
	"encoding/binary"
	"log/slog"
	"sync"
	"time"

	"go.keystone-core.io/keystone-core/internal/config"
)

// Dedup is a producer-side sliding-window cache of recently-published
// (subject, MessageID) pairs. PublishEnvelope consults it to suppress
// accidental duplicates within the configured window — see §4.2's
// "max network RTT" gotcha for the operational rationale.
//
// Hash construction is length-prefixed:
//
//	sha256( uint32(len(subject)) || subject || uint32(len(messageID)) || messageID )
//
// Length prefixes prevent the (subject, messageID) concatenation from
// being ambiguous: without them, ("kscore.x", "y\x00z") and
// ("kscore.x\x00y", "z") would hash to the same key, letting a
// crafted producer suppress a different producer's legitimate
// publish. SubjectBuilder.Validate and envelope.Validate also reject
// non-printable bytes as defense-in-depth, but the length-prefix is
// what makes the keying provably unambiguous.
//
// Storage is a map[hash]*list.Element + a doubly-linked list of
// entries. The map gives O(1) lookup; the list gives FIFO eviction
// when MaxEntries is exceeded. Cleanup walks the full list (per-
// subject overrides break expiry monotonicity) and removes expired
// entries.
type Dedup struct {
	cfg config.DedupConfig
	log *slog.Logger
	now func() time.Time

	mu    sync.Mutex
	list  *list.List // entries; oldest at front
	index map[[32]byte]*list.Element

	stopCh chan struct{}
	doneCh chan struct{}
}

type dedupEntry struct {
	hash   [32]byte
	expiry time.Time
}

// NewDedup constructs a Dedup from the validated config. Returns nil
// when cfg.Enabled is false — Manager checks for nil before calling
// IsDuplicate / Record.
func NewDedup(cfg config.DedupConfig, log *slog.Logger) *Dedup {
	if !cfg.Enabled {
		return nil
	}
	if log == nil {
		log = slog.Default()
	}
	return &Dedup{
		cfg:   cfg,
		log:   log,
		now:   time.Now,
		list:  list.New(),
		index: make(map[[32]byte]*list.Element, cfg.MaxEntries),
	}
}

// Start launches the cleanup goroutine. Idempotent — second call is
// a no-op.
func (d *Dedup) Start() {
	if d == nil {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopCh != nil {
		return
	}
	d.stopCh = make(chan struct{})
	d.doneCh = make(chan struct{})
	go d.runCleanup(d.stopCh, d.doneCh)
}

// Stop halts the cleanup goroutine and blocks until it exits.
// Idempotent — safe to call before Start or twice.
func (d *Dedup) Stop() {
	if d == nil {
		return
	}
	d.mu.Lock()
	stopCh := d.stopCh
	doneCh := d.doneCh
	d.stopCh = nil
	d.mu.Unlock()
	if stopCh == nil {
		return
	}
	close(stopCh)
	<-doneCh
}

// IsDuplicate reports whether (subject, messageID) is currently in
// the dedup window. Expired entries are not counted as duplicates.
func (d *Dedup) IsDuplicate(subject, messageID string) bool {
	if d == nil {
		return false
	}
	key := dedupKey(subject, messageID)
	now := d.now()

	d.mu.Lock()
	defer d.mu.Unlock()
	elem, ok := d.index[key]
	if !ok {
		return false
	}
	entry := elem.Value.(*dedupEntry)
	if !now.Before(entry.expiry) {
		// Stale; remove eagerly so a successive Record can re-insert
		// without a duplicate-key path.
		d.list.Remove(elem)
		delete(d.index, key)
		return false
	}
	return true
}

// Record stamps (subject, messageID) into the cache with an expiry
// derived from PerSubjectOverrides (longest-prefix match) or the
// default WindowDuration. Evicts the oldest entry if MaxEntries is
// exceeded. Idempotent: re-recording an already-known key just
// refreshes its expiry.
func (d *Dedup) Record(subject, messageID string) {
	if d == nil {
		return
	}
	key := dedupKey(subject, messageID)
	now := d.now()
	expiry := now.Add(d.windowFor(subject))

	d.mu.Lock()
	defer d.mu.Unlock()

	if elem, ok := d.index[key]; ok {
		entry := elem.Value.(*dedupEntry)
		entry.expiry = expiry
		d.list.MoveToBack(elem)
		return
	}

	entry := &dedupEntry{hash: key, expiry: expiry}
	elem := d.list.PushBack(entry)
	d.index[key] = elem

	for d.list.Len() > d.cfg.MaxEntries {
		oldest := d.list.Front()
		if oldest == nil {
			break
		}
		d.list.Remove(oldest)
		delete(d.index, oldest.Value.(*dedupEntry).hash)
	}
}

// Size returns the current entry count. Used by tests and for
// future observability surfacing.
func (d *Dedup) Size() int {
	if d == nil {
		return 0
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.list.Len()
}

// windowFor returns the configured window for subject, honoring
// PerSubjectOverrides. Longest-prefix match wins; equality is a
// prefix match. Falls back to WindowDuration on no match.
func (d *Dedup) windowFor(subject string) time.Duration {
	best := -1
	win := d.cfg.WindowDuration
	for _, o := range d.cfg.PerSubjectOverrides {
		if !hasPrefix(subject, o.Prefix) {
			continue
		}
		if len(o.Prefix) > best {
			best = len(o.Prefix)
			win = o.WindowDuration
		}
	}
	return win
}

// runCleanup is the periodic-purge loop. Walks the entire list each
// tick (per-subject overrides break expiry monotonicity, so an
// early-stop pass would miss entries with shorter overrides).
//
// Captures stopCh at entry so a Stop that nils d.stopCh doesn't
// turn the select branch into <-nil (blocks forever).
func (d *Dedup) runCleanup(stopCh, doneCh chan struct{}) {
	defer close(doneCh)
	t := time.NewTicker(d.cfg.CleanupInterval)
	defer t.Stop()
	for {
		select {
		case <-stopCh:
			return
		case <-t.C:
			d.purgeExpired()
		}
	}
}

func (d *Dedup) purgeExpired() {
	now := d.now()
	d.mu.Lock()
	defer d.mu.Unlock()

	var next *list.Element
	for e := d.list.Front(); e != nil; e = next {
		next = e.Next()
		entry := e.Value.(*dedupEntry)
		if now.Before(entry.expiry) {
			continue
		}
		d.list.Remove(e)
		delete(d.index, entry.hash)
	}
}

// dedupKey builds the SHA-256 hash of length-prefixed (subject,
// messageID). The two uint32 length prefixes make the encoding
// injective regardless of the field contents.
func dedupKey(subject, messageID string) [32]byte {
	h := sha256.New()
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(subject)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write([]byte(subject))
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(messageID)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write([]byte(messageID))
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// hasPrefix is strings.HasPrefix; tiny helper avoids the import for
// one call.
func hasPrefix(s, p string) bool {
	return len(s) >= len(p) && s[:len(p)] == p
}
