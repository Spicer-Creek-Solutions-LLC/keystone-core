package agent

import (
	"container/heap"
	"context"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// RefreshScheduler manages automatic secret refresh before expiration.
type RefreshScheduler struct {
	client *Client
	config *RefreshConfig

	mu        sync.Mutex
	scheduled *refreshHeap
	pathIndex map[string]*refreshEntry

	// Semaphore for limiting concurrent refreshes
	sem chan struct{}
}

type refreshEntry struct {
	path      string
	refreshAt time.Time
	index     int // heap index
}

type refreshHeap []*refreshEntry

func (h refreshHeap) Len() int           { return len(h) }
func (h refreshHeap) Less(i, j int) bool { return h[i].refreshAt.Before(h[j].refreshAt) }
func (h refreshHeap) Swap(i, j int) {
	h[i], h[j] = h[j], h[i]
	h[i].index = i
	h[j].index = j
}

func (h *refreshHeap) Push(x interface{}) {
	n := len(*h)
	entry := x.(*refreshEntry)
	entry.index = n
	*h = append(*h, entry)
}

func (h *refreshHeap) Pop() interface{} {
	old := *h
	n := len(old)
	entry := old[n-1]
	old[n-1] = nil
	entry.index = -1
	*h = old[0 : n-1]
	return entry
}

// NewRefreshScheduler creates a new refresh scheduler.
func NewRefreshScheduler(client *Client, config *RefreshConfig) *RefreshScheduler {
	if config == nil {
		config = &RefreshConfig{
			Enabled:              true,
			RefreshThreshold:     0.75,
			CheckInterval:        30 * time.Second,
			MaxConcurrentRefresh: 5,
		}
	}

	if config.RefreshThreshold <= 0 || config.RefreshThreshold >= 1 {
		config.RefreshThreshold = 0.75
	}
	if config.CheckInterval <= 0 {
		config.CheckInterval = 30 * time.Second
	}
	if config.MaxConcurrentRefresh <= 0 {
		config.MaxConcurrentRefresh = 5
	}

	h := &refreshHeap{}
	heap.Init(h)

	return &RefreshScheduler{
		client:    client,
		config:    config,
		scheduled: h,
		pathIndex: make(map[string]*refreshEntry),
		sem:       make(chan struct{}, config.MaxConcurrentRefresh),
	}
}

// Schedule schedules a secret for refresh before its expiration.
func (rs *RefreshScheduler) Schedule(path string, expiresAt time.Time) {
	if expiresAt.IsZero() {
		return
	}

	// Calculate refresh time based on threshold
	now := time.Now()
	ttl := expiresAt.Sub(now)
	if ttl <= 0 {
		return
	}

	refreshAt := now.Add(time.Duration(float64(ttl) * rs.config.RefreshThreshold))

	rs.mu.Lock()
	defer rs.mu.Unlock()

	// Update existing entry or create new one
	if entry, exists := rs.pathIndex[path]; exists {
		entry.refreshAt = refreshAt
		heap.Fix(rs.scheduled, entry.index)
	} else {
		entry := &refreshEntry{
			path:      path,
			refreshAt: refreshAt,
		}
		heap.Push(rs.scheduled, entry)
		rs.pathIndex[path] = entry
	}
}

// Cancel cancels a scheduled refresh.
func (rs *RefreshScheduler) Cancel(path string) {
	rs.mu.Lock()
	defer rs.mu.Unlock()

	if entry, exists := rs.pathIndex[path]; exists {
		heap.Remove(rs.scheduled, entry.index)
		delete(rs.pathIndex, path)
	}
}

// Pending returns the number of pending refreshes.
func (rs *RefreshScheduler) Pending() int {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.scheduled.Len()
}

// Run starts the refresh scheduler loop.
func (rs *RefreshScheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(rs.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			rs.processRefreshes(ctx)
		}
	}
}

func (rs *RefreshScheduler) processRefreshes(ctx context.Context) {
	now := time.Now()

	for {
		rs.mu.Lock()
		if rs.scheduled.Len() == 0 {
			rs.mu.Unlock()
			return
		}

		// Peek at the next entry
		entry := (*rs.scheduled)[0]
		if entry.refreshAt.After(now) {
			rs.mu.Unlock()
			return
		}

		// Pop the entry
		heap.Pop(rs.scheduled)
		delete(rs.pathIndex, entry.path)
		rs.mu.Unlock()

		// Acquire semaphore for rate limiting
		select {
		case rs.sem <- struct{}{}:
		case <-ctx.Done():
			return
		}

		// Refresh in background
		go func(path string) {
			defer func() { <-rs.sem }()
			_ = rs.client.refreshSecret(ctx, path)
		}(entry.path)
	}
}

// =============================================================================
// Request Batcher
// =============================================================================

// RequestBatcher batches multiple secret requests for efficiency.
type RequestBatcher struct {
	client *Client
	config *BatchConfig

	mu       sync.Mutex
	pending  []*batchRequest
	timer    *time.Timer
	notifyCh chan struct{}
}

type batchRequest struct {
	req    *SecretRequest
	result chan *batchResult
}

type batchResult struct {
	secret *secrets.Secret
	err    error
}

// NewRequestBatcher creates a new request batcher.
func NewRequestBatcher(client *Client, config *BatchConfig) *RequestBatcher {
	if config == nil {
		config = &BatchConfig{
			Enabled:      true,
			MaxBatchSize: 10,
			BatchTimeout: 50 * time.Millisecond,
		}
	}

	if config.MaxBatchSize <= 0 {
		config.MaxBatchSize = 10
	}
	if config.BatchTimeout <= 0 {
		config.BatchTimeout = 50 * time.Millisecond
	}

	return &RequestBatcher{
		client:   client,
		config:   config,
		notifyCh: make(chan struct{}, 1),
	}
}

// Get submits a request to the batcher and waits for the result.
func (rb *RequestBatcher) Get(ctx context.Context, req *SecretRequest) (*secrets.Secret, error) {
	resultCh := make(chan *batchResult, 1)

	rb.mu.Lock()
	rb.pending = append(rb.pending, &batchRequest{
		req:    req,
		result: resultCh,
	})

	// Start timer if this is the first request
	if len(rb.pending) == 1 {
		rb.timer = time.AfterFunc(rb.config.BatchTimeout, func() {
			select {
			case rb.notifyCh <- struct{}{}:
			default:
			}
		})
	}

	// Notify if batch is full
	if len(rb.pending) >= rb.config.MaxBatchSize {
		if rb.timer != nil {
			rb.timer.Stop()
		}
		select {
		case rb.notifyCh <- struct{}{}:
		default:
		}
	}
	rb.mu.Unlock()

	// Wait for result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-resultCh:
		return result.secret, result.err
	}
}

// Run starts the batcher loop.
func (rb *RequestBatcher) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			rb.drainPending(ctx.Err())
			return
		case <-rb.notifyCh:
			rb.processBatch(ctx)
		}
	}
}

func (rb *RequestBatcher) processBatch(ctx context.Context) {
	rb.mu.Lock()
	if len(rb.pending) == 0 {
		rb.mu.Unlock()
		return
	}

	batch := rb.pending
	rb.pending = nil
	rb.mu.Unlock()

	// Build batch request
	reqs := make([]*SecretRequest, len(batch))
	for i, br := range batch {
		reqs[i] = br.req
	}

	// Execute batch request
	results, err := rb.client.fetchBatchFromBroker(ctx, reqs)

	// Send results to waiters
	for _, br := range batch {
		result := &batchResult{}
		if err != nil {
			result.err = err
		} else if secret, ok := results[br.req.Path]; ok {
			result.secret = secret
		} else {
			result.err = ErrSecretNotFound
		}
		br.result <- result
	}
}

func (rb *RequestBatcher) drainPending(err error) {
	rb.mu.Lock()
	defer rb.mu.Unlock()

	for _, br := range rb.pending {
		br.result <- &batchResult{err: err}
	}
	rb.pending = nil
}

// Pending returns the number of pending requests.
func (rb *RequestBatcher) Pending() int {
	rb.mu.Lock()
	defer rb.mu.Unlock()
	return len(rb.pending)
}
