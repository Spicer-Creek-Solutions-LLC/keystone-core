package nats

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Duplicate Detection - T8.3
// ============================================================================

// DedupConfig holds deduplication configuration
type DedupConfig struct {
	// WindowDuration is the deduplication window
	WindowDuration time.Duration

	// MaxEntries is the maximum number of entries to track
	MaxEntries int

	// CleanupInterval is how often to clean expired entries
	CleanupInterval time.Duration

	// PerSubject enables per-subject deduplication
	PerSubject bool

	// SubjectConfigs holds per-subject configurations
	SubjectConfigs map[string]*SubjectDedupConfig

	// IDGenerator generates message IDs if not provided
	IDGenerator func(subject string, data []byte) string
}

// SubjectDedupConfig holds subject-specific deduplication config
type SubjectDedupConfig struct {
	// WindowDuration overrides global window
	WindowDuration time.Duration

	// MaxEntries overrides global max entries
	MaxEntries int

	// Enabled can disable dedup for specific subjects
	Enabled bool
}

// DefaultDedupConfig returns sensible defaults
func DefaultDedupConfig() *DedupConfig {
	return &DedupConfig{
		WindowDuration:  5 * time.Minute,
		MaxEntries:      100000,
		CleanupInterval: 30 * time.Second,
		PerSubject:      false,
		IDGenerator:     DefaultIDGenerator,
	}
}

// Validate validates the configuration
func (c *DedupConfig) Validate() error {
	if c.WindowDuration <= 0 {
		return errors.New("window duration must be positive")
	}
	if c.MaxEntries <= 0 {
		return errors.New("max entries must be positive")
	}
	if c.CleanupInterval <= 0 {
		return errors.New("cleanup interval must be positive")
	}
	return nil
}

// DefaultIDGenerator generates a message ID from content hash
func DefaultIDGenerator(subject string, data []byte) string {
	hash := sha256.New()
	hash.Write([]byte(subject))
	hash.Write(data)
	return hex.EncodeToString(hash.Sum(nil)[:16])
}

// DedupEntry represents a tracked message
type DedupEntry struct {
	// ID is the message ID
	ID string

	// Subject is the message subject
	Subject string

	// FirstSeen is when the message was first seen
	FirstSeen time.Time

	// LastSeen is when the message was last seen
	LastSeen time.Time

	// Count is how many times the message was seen
	Count int64

	// ExpiresAt is when this entry expires
	ExpiresAt time.Time
}

// IsExpired returns true if the entry has expired
func (e *DedupEntry) IsExpired() bool {
	return time.Now().After(e.ExpiresAt)
}

// DedupStats holds deduplication statistics
type DedupStats struct {
	// TotalChecked is total messages checked
	TotalChecked int64

	// TotalDuplicates is total duplicates detected
	TotalDuplicates int64

	// TotalUnique is total unique messages
	TotalUnique int64

	// EntryCount is current entry count
	EntryCount int64

	// EntriesExpired is total entries expired
	EntriesExpired int64

	// EntriesEvicted is total entries evicted (due to max size)
	EntriesEvicted int64

	// DuplicateRate is the duplicate percentage
	DuplicateRate float64
}

// Deduplicator provides message deduplication
type Deduplicator struct {
	config *DedupConfig

	// Global entries (when PerSubject is false)
	entries   map[string]*DedupEntry
	entriesMu sync.RWMutex

	// Per-subject entries (when PerSubject is true)
	subjectEntries   map[string]map[string]*DedupEntry
	subjectEntriesMu sync.RWMutex

	// Entry ordering for eviction (oldest first)
	entryOrder []string
	orderMu    sync.Mutex

	// Statistics
	stats DeduplicatorStats

	// Callbacks
	onDuplicate func(entry *DedupEntry)

	// Lifecycle
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// DeduplicatorStats holds runtime stats
type DeduplicatorStats struct {
	totalChecked    int64
	totalDuplicates int64
	totalUnique     int64
	entriesExpired  int64
	entriesEvicted  int64
}

// NewDeduplicator creates a new deduplicator
func NewDeduplicator(config *DedupConfig) (*Deduplicator, error) {
	if config == nil {
		config = DefaultDedupConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	d := &Deduplicator{
		config:         config,
		entries:        make(map[string]*DedupEntry),
		subjectEntries: make(map[string]map[string]*DedupEntry),
		entryOrder:     make([]string, 0, config.MaxEntries),
		ctx:            ctx,
		cancel:         cancel,
	}

	return d, nil
}

// Start starts the deduplicator
func (d *Deduplicator) Start() error {
	if d.running.Load() {
		return errors.New("already running")
	}
	d.running.Store(true)

	// Start cleanup loop
	d.wg.Add(1)
	go d.cleanupLoop()

	return nil
}

// Stop stops the deduplicator
func (d *Deduplicator) Stop() error {
	if !d.running.Load() {
		return nil
	}

	d.cancel()
	d.wg.Wait()
	d.running.Store(false)
	return nil
}

// IsDuplicate checks if a message is a duplicate
func (d *Deduplicator) IsDuplicate(subject string, data []byte) bool {
	return d.IsDuplicateWithID(subject, data, "")
}

// IsDuplicateWithID checks if a message with given ID is a duplicate
func (d *Deduplicator) IsDuplicateWithID(subject string, data []byte, msgID string) bool {
	atomic.AddInt64(&d.stats.totalChecked, 1)

	// Generate ID if not provided
	if msgID == "" {
		if d.config.IDGenerator != nil {
			msgID = d.config.IDGenerator(subject, data)
		} else {
			msgID = DefaultIDGenerator(subject, data)
		}
	}

	// Get config for subject
	windowDuration := d.config.WindowDuration
	maxEntries := d.config.MaxEntries

	if d.config.PerSubject {
		if subjectConfig, ok := d.config.SubjectConfigs[subject]; ok {
			if !subjectConfig.Enabled {
				// Dedup disabled for this subject
				return false
			}
			if subjectConfig.WindowDuration > 0 {
				windowDuration = subjectConfig.WindowDuration
			}
			if subjectConfig.MaxEntries > 0 {
				maxEntries = subjectConfig.MaxEntries
			}
		}
	}

	now := time.Now()
	expiresAt := now.Add(windowDuration)

	if d.config.PerSubject {
		return d.checkPerSubject(subject, msgID, now, expiresAt, maxEntries)
	}
	return d.checkGlobal(subject, msgID, now, expiresAt, maxEntries)
}

func (d *Deduplicator) checkGlobal(subject, msgID string, now, expiresAt time.Time, maxEntries int) bool {
	d.entriesMu.Lock()
	defer d.entriesMu.Unlock()

	if entry, exists := d.entries[msgID]; exists {
		if !entry.IsExpired() {
			entry.LastSeen = now
			entry.Count++
			atomic.AddInt64(&d.stats.totalDuplicates, 1)

			if d.onDuplicate != nil {
				go d.onDuplicate(entry)
			}
			return true
		}
		// Entry expired, treat as new
		delete(d.entries, msgID)
	}

	// Evict if at capacity
	if len(d.entries) >= maxEntries {
		d.evictOldest()
	}

	// Add new entry
	entry := &DedupEntry{
		ID:        msgID,
		Subject:   subject,
		FirstSeen: now,
		LastSeen:  now,
		Count:     1,
		ExpiresAt: expiresAt,
	}
	d.entries[msgID] = entry

	d.orderMu.Lock()
	d.entryOrder = append(d.entryOrder, msgID)
	d.orderMu.Unlock()

	atomic.AddInt64(&d.stats.totalUnique, 1)
	return false
}

func (d *Deduplicator) checkPerSubject(subject, msgID string, now, expiresAt time.Time, maxEntries int) bool {
	d.subjectEntriesMu.Lock()
	defer d.subjectEntriesMu.Unlock()

	subjectMap, exists := d.subjectEntries[subject]
	if !exists {
		subjectMap = make(map[string]*DedupEntry)
		d.subjectEntries[subject] = subjectMap
	}

	if entry, exists := subjectMap[msgID]; exists {
		if !entry.IsExpired() {
			entry.LastSeen = now
			entry.Count++
			atomic.AddInt64(&d.stats.totalDuplicates, 1)

			if d.onDuplicate != nil {
				go d.onDuplicate(entry)
			}
			return true
		}
		// Entry expired, treat as new
		delete(subjectMap, msgID)
	}

	// Evict if at capacity for this subject
	if len(subjectMap) >= maxEntries {
		d.evictOldestFromMap(subjectMap)
	}

	// Add new entry
	entry := &DedupEntry{
		ID:        msgID,
		Subject:   subject,
		FirstSeen: now,
		LastSeen:  now,
		Count:     1,
		ExpiresAt: expiresAt,
	}
	subjectMap[msgID] = entry

	atomic.AddInt64(&d.stats.totalUnique, 1)
	return false
}

func (d *Deduplicator) evictOldest() {
	d.orderMu.Lock()
	defer d.orderMu.Unlock()

	if len(d.entryOrder) == 0 {
		return
	}

	// Remove oldest
	oldestID := d.entryOrder[0]
	d.entryOrder = d.entryOrder[1:]
	delete(d.entries, oldestID)
	atomic.AddInt64(&d.stats.entriesEvicted, 1)
}

func (d *Deduplicator) evictOldestFromMap(m map[string]*DedupEntry) {
	var oldestID string
	var oldestTime time.Time

	for id, entry := range m {
		if oldestID == "" || entry.FirstSeen.Before(oldestTime) {
			oldestID = id
			oldestTime = entry.FirstSeen
		}
	}

	if oldestID != "" {
		delete(m, oldestID)
		atomic.AddInt64(&d.stats.entriesEvicted, 1)
	}
}

func (d *Deduplicator) cleanupLoop() {
	defer d.wg.Done()

	ticker := time.NewTicker(d.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-d.ctx.Done():
			return
		case <-ticker.C:
			d.cleanup()
		}
	}
}

func (d *Deduplicator) cleanup() {
	now := time.Now()
	var expired int64

	if d.config.PerSubject {
		d.subjectEntriesMu.Lock()
		for subject, subjectMap := range d.subjectEntries {
			for id, entry := range subjectMap {
				if now.After(entry.ExpiresAt) {
					delete(subjectMap, id)
					expired++
				}
			}
			// Remove empty subject maps
			if len(subjectMap) == 0 {
				delete(d.subjectEntries, subject)
			}
		}
		d.subjectEntriesMu.Unlock()
	} else {
		d.entriesMu.Lock()
		for id, entry := range d.entries {
			if now.After(entry.ExpiresAt) {
				delete(d.entries, id)
				expired++
			}
		}
		d.entriesMu.Unlock()

		// Clean up order list
		if expired > 0 {
			d.orderMu.Lock()
			newOrder := make([]string, 0, len(d.entryOrder))
			d.entriesMu.RLock()
			for _, id := range d.entryOrder {
				if _, exists := d.entries[id]; exists {
					newOrder = append(newOrder, id)
				}
			}
			d.entriesMu.RUnlock()
			d.entryOrder = newOrder
			d.orderMu.Unlock()
		}
	}

	if expired > 0 {
		atomic.AddInt64(&d.stats.entriesExpired, expired)
	}
}

// GetStats returns deduplication statistics
func (d *Deduplicator) GetStats() DedupStats {
	totalChecked := atomic.LoadInt64(&d.stats.totalChecked)
	totalDuplicates := atomic.LoadInt64(&d.stats.totalDuplicates)
	totalUnique := atomic.LoadInt64(&d.stats.totalUnique)

	var entryCount int64
	if d.config.PerSubject {
		d.subjectEntriesMu.RLock()
		for _, m := range d.subjectEntries {
			entryCount += int64(len(m))
		}
		d.subjectEntriesMu.RUnlock()
	} else {
		d.entriesMu.RLock()
		entryCount = int64(len(d.entries))
		d.entriesMu.RUnlock()
	}

	var duplicateRate float64
	if totalChecked > 0 {
		duplicateRate = float64(totalDuplicates) / float64(totalChecked) * 100
	}

	return DedupStats{
		TotalChecked:    totalChecked,
		TotalDuplicates: totalDuplicates,
		TotalUnique:     totalUnique,
		EntryCount:      entryCount,
		EntriesExpired:  atomic.LoadInt64(&d.stats.entriesExpired),
		EntriesEvicted:  atomic.LoadInt64(&d.stats.entriesEvicted),
		DuplicateRate:   duplicateRate,
	}
}

// GetEntry returns a specific entry
func (d *Deduplicator) GetEntry(msgID string) *DedupEntry {
	d.entriesMu.RLock()
	defer d.entriesMu.RUnlock()

	if entry, exists := d.entries[msgID]; exists {
		entryCopy := *entry
		return &entryCopy
	}
	return nil
}

// GetEntryForSubject returns a specific entry for a subject
func (d *Deduplicator) GetEntryForSubject(subject, msgID string) *DedupEntry {
	d.subjectEntriesMu.RLock()
	defer d.subjectEntriesMu.RUnlock()

	if subjectMap, exists := d.subjectEntries[subject]; exists {
		if entry, exists := subjectMap[msgID]; exists {
			entryCopy := *entry
			return &entryCopy
		}
	}
	return nil
}

// Clear removes all entries
func (d *Deduplicator) Clear() {
	if d.config.PerSubject {
		d.subjectEntriesMu.Lock()
		d.subjectEntries = make(map[string]map[string]*DedupEntry)
		d.subjectEntriesMu.Unlock()
	} else {
		d.entriesMu.Lock()
		d.entries = make(map[string]*DedupEntry)
		d.entriesMu.Unlock()

		d.orderMu.Lock()
		d.entryOrder = make([]string, 0, d.config.MaxEntries)
		d.orderMu.Unlock()
	}
}

// SetDuplicateCallback sets the callback for duplicate detection
func (d *Deduplicator) SetDuplicateCallback(fn func(*DedupEntry)) {
	d.onDuplicate = fn
}

// EntryCount returns the current entry count
func (d *Deduplicator) EntryCount() int {
	if d.config.PerSubject {
		d.subjectEntriesMu.RLock()
		defer d.subjectEntriesMu.RUnlock()
		var count int
		for _, m := range d.subjectEntries {
			count += len(m)
		}
		return count
	}

	d.entriesMu.RLock()
	defer d.entriesMu.RUnlock()
	return len(d.entries)
}

// SubjectCount returns the number of tracked subjects (per-subject mode)
func (d *Deduplicator) SubjectCount() int {
	if !d.config.PerSubject {
		return 0
	}

	d.subjectEntriesMu.RLock()
	defer d.subjectEntriesMu.RUnlock()
	return len(d.subjectEntries)
}

// MarkSeen manually marks a message ID as seen
func (d *Deduplicator) MarkSeen(subject string, msgID string) {
	now := time.Now()
	expiresAt := now.Add(d.config.WindowDuration)

	entry := &DedupEntry{
		ID:        msgID,
		Subject:   subject,
		FirstSeen: now,
		LastSeen:  now,
		Count:     1,
		ExpiresAt: expiresAt,
	}

	if d.config.PerSubject {
		d.subjectEntriesMu.Lock()
		subjectMap, exists := d.subjectEntries[subject]
		if !exists {
			subjectMap = make(map[string]*DedupEntry)
			d.subjectEntries[subject] = subjectMap
		}
		subjectMap[msgID] = entry
		d.subjectEntriesMu.Unlock()
	} else {
		d.entriesMu.Lock()
		d.entries[msgID] = entry
		d.entriesMu.Unlock()

		d.orderMu.Lock()
		d.entryOrder = append(d.entryOrder, msgID)
		d.orderMu.Unlock()
	}
}

// Remove removes a specific entry
func (d *Deduplicator) Remove(msgID string) {
	if d.config.PerSubject {
		d.subjectEntriesMu.Lock()
		for _, subjectMap := range d.subjectEntries {
			delete(subjectMap, msgID)
		}
		d.subjectEntriesMu.Unlock()
	} else {
		d.entriesMu.Lock()
		delete(d.entries, msgID)
		d.entriesMu.Unlock()
	}
}
