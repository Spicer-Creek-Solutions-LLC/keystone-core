// Package mirror implements mirror groups and geographic routing for file distribution.
package mirror

import (
	"math/rand" // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- non-crypto randomness for load distribution
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

// ReadRouter selects mirrors for read operations.
type ReadRouter interface {
	// SelectForRead returns mirrors in order of preference for reading.
	SelectForRead(group *Group, agentLocation *Location) []*Mirror
}

// WriteRouter selects mirrors for write operations.
type WriteRouter interface {
	// SelectForWrite returns mirrors for writing based on policy.
	SelectForWrite(group *Group) ([]*Mirror, error)
}

// NearestRouter routes to the mirror with lowest latency.
type NearestRouter struct {
	latencies map[string]time.Duration // mirrorID -> latency
	mu        sync.RWMutex
}

// NewNearestRouter creates a new nearest router.
func NewNearestRouter() *NearestRouter {
	return &NearestRouter{
		latencies: make(map[string]time.Duration),
	}
}

// UpdateLatency updates the latency measurement for a mirror.
func (r *NearestRouter) UpdateLatency(mirrorID string, latency time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.latencies[mirrorID] = latency
}

// GetLatency returns the current latency for a mirror.
func (r *NearestRouter) GetLatency(mirrorID string) (time.Duration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	l, ok := r.latencies[mirrorID]
	return l, ok
}

// SelectForRead returns mirrors sorted by latency (lowest first).
func (r *NearestRouter) SelectForRead(group *Group, agentLocation *Location) []*Mirror {
	mirrors := group.GetHealthyMirrors()
	if len(mirrors) == 0 {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Sort by latency
	sort.Slice(mirrors, func(i, j int) bool {
		li, oki := r.latencies[mirrors[i].ID]
		lj, okj := r.latencies[mirrors[j].ID]

		// Unknown latency goes to the end
		if !oki && !okj {
			return mirrors[i].Priority < mirrors[j].Priority
		}
		if !oki {
			return false
		}
		if !okj {
			return true
		}
		return li < lj
	})

	return mirrors
}

// RoundRobinRouter distributes reads evenly across mirrors.
type RoundRobinRouter struct {
	counter uint64
}

// NewRoundRobinRouter creates a new round-robin router.
func NewRoundRobinRouter() *RoundRobinRouter {
	return &RoundRobinRouter{}
}

// SelectForRead returns mirrors in round-robin order, respecting weights.
func (r *RoundRobinRouter) SelectForRead(group *Group, agentLocation *Location) []*Mirror {
	mirrors := group.GetHealthyMirrors()
	if len(mirrors) == 0 {
		return nil
	}

	// Build weighted list
	weighted := make([]*Mirror, 0)
	for _, m := range mirrors {
		for i := 0; i < m.Weight; i++ {
			weighted = append(weighted, m)
		}
	}

	if len(weighted) == 0 {
		return mirrors
	}

	// Get next index
	idx := atomic.AddUint64(&r.counter, 1) - 1
	//nolint:gosec // G115: modulo result is bounded by len(weighted), fits in int
	startIdx := int(idx % uint64(len(weighted)))

	// Build result starting from this index
	seen := make(map[string]bool)
	result := make([]*Mirror, 0, len(mirrors))

	for i := 0; i < len(weighted); i++ {
		m := weighted[(startIdx+i)%len(weighted)]
		if !seen[m.ID] {
			seen[m.ID] = true
			result = append(result, m)
		}
	}

	return result
}

// FailoverRouter routes to mirrors in priority order.
type FailoverRouter struct{}

// NewFailoverRouter creates a new failover router.
func NewFailoverRouter() *FailoverRouter {
	return &FailoverRouter{}
}

// SelectForRead returns mirrors sorted by priority (lowest first).
func (r *FailoverRouter) SelectForRead(group *Group, agentLocation *Location) []*Mirror {
	mirrors := group.GetHealthyMirrors()
	if len(mirrors) == 0 {
		return nil
	}

	// Sort by priority
	sort.Slice(mirrors, func(i, j int) bool {
		return mirrors[i].Priority < mirrors[j].Priority
	})

	return mirrors
}

// FastestRouter routes based on recent response times.
type FastestRouter struct {
	responseTimes map[string]*responseTimeTracker
	mu            sync.RWMutex
}

type responseTimeTracker struct {
	times      []time.Duration
	maxSamples int
	idx        int
}

// NewFastestRouter creates a new fastest router.
func NewFastestRouter() *FastestRouter {
	return &FastestRouter{
		responseTimes: make(map[string]*responseTimeTracker),
	}
}

// RecordResponseTime records a response time for a mirror.
func (r *FastestRouter) RecordResponseTime(mirrorID string, duration time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tracker, ok := r.responseTimes[mirrorID]
	if !ok {
		tracker = &responseTimeTracker{
			times:      make([]time.Duration, 100),
			maxSamples: 100,
		}
		r.responseTimes[mirrorID] = tracker
	}

	tracker.times[tracker.idx%tracker.maxSamples] = duration
	tracker.idx++
}

// GetAvgResponseTime returns the average response time for a mirror.
func (r *FastestRouter) GetAvgResponseTime(mirrorID string) (time.Duration, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	tracker, ok := r.responseTimes[mirrorID]
	if !ok || tracker.idx == 0 {
		return 0, false
	}

	count := tracker.idx
	if count > tracker.maxSamples {
		count = tracker.maxSamples
	}

	var sum time.Duration
	for i := 0; i < count; i++ {
		sum += tracker.times[i]
	}
	return sum / time.Duration(count), true
}

// SelectForRead returns mirrors sorted by average response time.
func (r *FastestRouter) SelectForRead(group *Group, agentLocation *Location) []*Mirror {
	mirrors := group.GetHealthyMirrors()
	if len(mirrors) == 0 {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Sort by average response time
	sort.Slice(mirrors, func(i, j int) bool {
		ti, oki := r.getAvgUnlocked(mirrors[i].ID)
		tj, okj := r.getAvgUnlocked(mirrors[j].ID)

		if !oki && !okj {
			return mirrors[i].Priority < mirrors[j].Priority
		}
		if !oki {
			return false
		}
		if !okj {
			return true
		}
		return ti < tj
	})

	return mirrors
}

func (r *FastestRouter) getAvgUnlocked(mirrorID string) (time.Duration, bool) {
	tracker, ok := r.responseTimes[mirrorID]
	if !ok || tracker.idx == 0 {
		return 0, false
	}

	count := tracker.idx
	if count > tracker.maxSamples {
		count = tracker.maxSamples
	}

	var sum time.Duration
	for i := 0; i < count; i++ {
		sum += tracker.times[i]
	}
	return sum / time.Duration(count), true
}

// RandomRouter selects a random mirror (for load distribution testing).
type RandomRouter struct {
	rng *rand.Rand
	mu  sync.Mutex
}

// NewRandomRouter creates a new random router.
func NewRandomRouter() *RandomRouter {
	return &RandomRouter{
		//nolint:gosec // G404: math/rand used for load distribution shuffling, not security
		rng: rand.New(rand.NewSource(time.Now().UnixNano())), // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- non-crypto randomness for load distribution
	}
}

// SelectForRead returns mirrors in random order.
func (r *RandomRouter) SelectForRead(group *Group, agentLocation *Location) []*Mirror {
	mirrors := group.GetHealthyMirrors()
	if len(mirrors) == 0 {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	// Shuffle
	result := make([]*Mirror, len(mirrors))
	copy(result, mirrors)
	r.rng.Shuffle(len(result), func(i, j int) {
		result[i], result[j] = result[j], result[i]
	})

	return result
}

// CompositeRouter combines multiple routers with fallback.
type CompositeRouter struct {
	primary   ReadRouter
	fallbacks []ReadRouter
}

// NewCompositeRouter creates a router that tries primary then fallbacks.
func NewCompositeRouter(primary ReadRouter, fallbacks ...ReadRouter) *CompositeRouter {
	return &CompositeRouter{
		primary:   primary,
		fallbacks: fallbacks,
	}
}

// SelectForRead tries primary router, then fallbacks if empty.
func (r *CompositeRouter) SelectForRead(group *Group, agentLocation *Location) []*Mirror {
	mirrors := r.primary.SelectForRead(group, agentLocation)
	if len(mirrors) > 0 {
		return mirrors
	}

	for _, fb := range r.fallbacks {
		mirrors = fb.SelectForRead(group, agentLocation)
		if len(mirrors) > 0 {
			return mirrors
		}
	}

	return nil
}

// NewReadRouter creates a read router based on strategy.
func NewReadRouter(strategy ReadStrategy) ReadRouter {
	switch strategy {
	case ReadStrategyNearest:
		return NewNearestRouter()
	case ReadStrategyRoundRobin:
		return NewRoundRobinRouter()
	case ReadStrategyFailover:
		return NewFailoverRouter()
	case ReadStrategyFastest:
		return NewFastestRouter()
	default:
		return NewFailoverRouter()
	}
}

// AllWriteRouter writes to all mirrors synchronously.
type AllWriteRouter struct{}

// NewAllWriteRouter creates an all-write router.
func NewAllWriteRouter() *AllWriteRouter {
	return &AllWriteRouter{}
}

// SelectForWrite returns all writable mirrors.
func (r *AllWriteRouter) SelectForWrite(group *Group) ([]*Mirror, error) {
	mirrors := group.GetHealthyMirrors()
	result := make([]*Mirror, 0)
	for _, m := range mirrors {
		if !m.ReadOnly {
			result = append(result, m)
		}
	}
	if len(result) == 0 {
		return nil, ErrNoWritableMirrors
	}
	return result, nil
}

// QuorumWriteRouter writes to a quorum of mirrors.
type QuorumWriteRouter struct {
	quorumSize int
}

// NewQuorumWriteRouter creates a quorum write router.
func NewQuorumWriteRouter(quorumSize int) *QuorumWriteRouter {
	return &QuorumWriteRouter{quorumSize: quorumSize}
}

// SelectForWrite returns enough mirrors to reach quorum.
func (r *QuorumWriteRouter) SelectForWrite(group *Group) ([]*Mirror, error) {
	mirrors := group.GetHealthyMirrors()
	result := make([]*Mirror, 0)

	// Sort by priority for deterministic selection
	sort.Slice(mirrors, func(i, j int) bool {
		return mirrors[i].Priority < mirrors[j].Priority
	})

	for _, m := range mirrors {
		if !m.ReadOnly {
			result = append(result, m)
		}
	}

	if len(result) < r.quorumSize {
		return nil, ErrInsufficientQuorum
	}

	return result, nil
}

// PrimaryOnlyWriteRouter writes only to primary mirror.
type PrimaryOnlyWriteRouter struct{}

// NewPrimaryOnlyWriteRouter creates a primary-only write router.
func NewPrimaryOnlyWriteRouter() *PrimaryOnlyWriteRouter {
	return &PrimaryOnlyWriteRouter{}
}

// SelectForWrite returns only the primary (lowest priority) mirror.
func (r *PrimaryOnlyWriteRouter) SelectForWrite(group *Group) ([]*Mirror, error) {
	mirrors := group.GetHealthyMirrors()
	var primary *Mirror

	for _, m := range mirrors {
		if m.ReadOnly {
			continue
		}
		if primary == nil || m.Priority < primary.Priority {
			primary = m
		}
	}

	if primary == nil {
		return nil, ErrNoWritableMirrors
	}
	return []*Mirror{primary}, nil
}

// PrimarySecondaryWriteRouter writes sync to primary + one secondary.
type PrimarySecondaryWriteRouter struct{}

// NewPrimarySecondaryWriteRouter creates a primary-secondary write router.
func NewPrimarySecondaryWriteRouter() *PrimarySecondaryWriteRouter {
	return &PrimarySecondaryWriteRouter{}
}

// SelectForWrite returns primary and one secondary mirror.
func (r *PrimarySecondaryWriteRouter) SelectForWrite(group *Group) ([]*Mirror, error) {
	mirrors := group.GetHealthyMirrors()

	// Sort by priority
	writable := make([]*Mirror, 0)
	for _, m := range mirrors {
		if !m.ReadOnly {
			writable = append(writable, m)
		}
	}

	if len(writable) == 0 {
		return nil, ErrNoWritableMirrors
	}

	sort.Slice(writable, func(i, j int) bool {
		return writable[i].Priority < writable[j].Priority
	})

	if len(writable) == 1 {
		return writable[:1], nil
	}
	return writable[:2], nil
}

// NewWriteRouter creates a write router based on policy.
func NewWriteRouter(policy WritePolicy, quorumSize int) WriteRouter {
	switch policy {
	case WritePolicyAll:
		return NewAllWriteRouter()
	case WritePolicyQuorum:
		return NewQuorumWriteRouter(quorumSize)
	case WritePolicyPrimaryOnly:
		return NewPrimaryOnlyWriteRouter()
	case WritePolicyPrimarySecondary:
		return NewPrimarySecondaryWriteRouter()
	default:
		return NewAllWriteRouter()
	}
}
