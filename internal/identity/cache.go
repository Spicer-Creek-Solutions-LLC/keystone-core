package identity

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// CacheConfig configures the SVID cache.
type CacheConfig struct {
	// MaxSize is the maximum number of cached SVIDs.
	MaxSize int

	// TTL is the default cache TTL.
	TTL time.Duration

	// CleanupInterval is how often to clean expired entries.
	CleanupInterval time.Duration

	// PreRotationBuffer is how long before expiry to evict entries.
	PreRotationBuffer time.Duration

	// EnableMetrics enables cache metrics collection.
	EnableMetrics bool
}

// DefaultCacheConfig returns a default cache configuration.
func DefaultCacheConfig() *CacheConfig {
	return &CacheConfig{
		MaxSize:           10000,
		TTL:               30 * time.Minute,
		CleanupInterval:   1 * time.Minute,
		PreRotationBuffer: 5 * time.Minute,
		EnableMetrics:     true,
	}
}

// CacheMetrics contains cache statistics.
type CacheMetrics struct {
	// Hits is the number of cache hits.
	Hits int64
	// Misses is the number of cache misses.
	Misses int64
	// Evictions is the number of cache evictions.
	Evictions int64
	// Size is the current cache size.
	Size int
	// MaxSize is the maximum cache size.
	MaxSize int
	// HitRate is the cache hit rate (0.0-1.0).
	HitRate float64
}

// LRUSVIDCache provides LRU caching for SVIDs with eviction and metrics.
// This is an enhanced version of SVIDCache with configurable limits and TTL.
type LRUSVIDCache struct {
	config  *CacheConfig
	mu      sync.RWMutex
	metrics CacheMetrics

	// LRU cache implementation
	cache  map[string]*list.Element
	lru    *list.List
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// cacheEntry is an entry in the cache.
type cacheEntry struct {
	key       string
	svid      *X509SVID
	expiresAt time.Time
	createdAt time.Time
}

// NewLRUSVIDCache creates a new LRU SVID cache.
func NewLRUSVIDCache(config *CacheConfig) *LRUSVIDCache {
	if config == nil {
		config = DefaultCacheConfig()
	}

	c := &LRUSVIDCache{
		config: config,
		cache:  make(map[string]*list.Element),
		lru:    list.New(),
		metrics: CacheMetrics{
			MaxSize: config.MaxSize,
		},
		stopCh: make(chan struct{}),
	}

	// Start cleanup goroutine
	c.wg.Add(1)
	go c.cleanupLoop()

	return c
}

// Get retrieves an SVID from the cache.
func (c *LRUSVIDCache) Get(spiffeID string) (*X509SVID, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.cache[spiffeID]
	if !ok {
		c.metrics.Misses++
		c.updateHitRate()
		return nil, false
	}

	entry := elem.Value.(*cacheEntry)

	// Check if expired or about to expire
	if time.Now().After(entry.expiresAt.Add(-c.config.PreRotationBuffer)) {
		c.removeElement(elem)
		c.metrics.Misses++
		c.updateHitRate()
		return nil, false
	}

	// Move to front (most recently used)
	c.lru.MoveToFront(elem)
	c.metrics.Hits++
	c.updateHitRate()

	return entry.svid, true
}

// Put adds an SVID to the cache.
func (c *LRUSVIDCache) Put(svid *X509SVID) {
	if svid == nil {
		return
	}

	key := svid.SPIFFEID.String()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if already exists
	if elem, ok := c.cache[key]; ok {
		c.lru.MoveToFront(elem)
		elem.Value.(*cacheEntry).svid = svid
		elem.Value.(*cacheEntry).expiresAt = svid.ExpiresAt
		return
	}

	// Evict if at capacity
	for c.lru.Len() >= c.config.MaxSize {
		c.evictOldest()
	}

	// Add new entry
	entry := &cacheEntry{
		key:       key,
		svid:      svid,
		expiresAt: svid.ExpiresAt,
		createdAt: time.Now(),
	}

	elem := c.lru.PushFront(entry)
	c.cache[key] = elem
	c.metrics.Size = c.lru.Len()
}

// Remove removes an SVID from the cache.
func (c *LRUSVIDCache) Remove(spiffeID string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.cache[spiffeID]; ok {
		c.removeElement(elem)
	}
}

// Clear clears the entire cache.
func (c *LRUSVIDCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cache = make(map[string]*list.Element)
	c.lru.Init()
	c.metrics.Size = 0
}

// GetMetrics returns cache metrics.
func (c *LRUSVIDCache) GetMetrics() CacheMetrics {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.metrics
}

// Close closes the cache and stops the cleanup goroutine.
func (c *LRUSVIDCache) Close() {
	close(c.stopCh)
	c.wg.Wait()
}

// evictOldest evicts the oldest (least recently used) entry.
func (c *LRUSVIDCache) evictOldest() {
	elem := c.lru.Back()
	if elem != nil {
		c.removeElement(elem)
		c.metrics.Evictions++
	}
}

// removeElement removes an element from the cache.
func (c *LRUSVIDCache) removeElement(elem *list.Element) {
	c.lru.Remove(elem)
	entry := elem.Value.(*cacheEntry)
	delete(c.cache, entry.key)
	c.metrics.Size = c.lru.Len()
}

// updateHitRate updates the hit rate metric.
func (c *LRUSVIDCache) updateHitRate() {
	total := c.metrics.Hits + c.metrics.Misses
	if total > 0 {
		c.metrics.HitRate = float64(c.metrics.Hits) / float64(total)
	}
}

// cleanupLoop periodically cleans up expired entries.
func (c *LRUSVIDCache) cleanupLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.stopCh:
			return
		case <-ticker.C:
			c.cleanup()
		}
	}
}

// cleanup removes expired entries.
func (c *LRUSVIDCache) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	threshold := now.Add(c.config.PreRotationBuffer)

	// Iterate from back (oldest) to front
	for elem := c.lru.Back(); elem != nil; {
		entry := elem.Value.(*cacheEntry)
		prev := elem.Prev()

		if entry.expiresAt.Before(threshold) {
			c.removeElement(elem)
			c.metrics.Evictions++
		}

		elem = prev
	}
}

// BatchConfig configures batch operations.
type BatchConfig struct {
	// MaxBatchSize is the maximum number of operations per batch.
	MaxBatchSize int

	// BatchTimeout is the maximum time to wait for a batch to fill.
	BatchTimeout time.Duration

	// MaxConcurrentBatches is the maximum number of concurrent batches.
	MaxConcurrentBatches int

	// RetryOnPartialFailure retries failed items in the batch.
	RetryOnPartialFailure bool
}

// DefaultBatchConfig returns a default batch configuration.
func DefaultBatchConfig() *BatchConfig {
	return &BatchConfig{
		MaxBatchSize:          100,
		BatchTimeout:          50 * time.Millisecond,
		MaxConcurrentBatches:  10,
		RetryOnPartialFailure: true,
	}
}

// BatchResult contains the result of a batch operation.
type BatchResult struct {
	// Successful contains successful items.
	Successful []interface{}
	// Failed contains failed items with their errors.
	Failed []BatchError
	// Duration is how long the batch took.
	Duration time.Duration
}

// BatchError contains an error for a batch item.
type BatchError struct {
	Item  interface{}
	Error error
}

// SVIDIssuerProvider is an interface for issuing SVIDs.
type SVIDIssuerProvider interface {
	IssueX509SVID(ctx context.Context, req *X509SVIDRequest) (*X509SVID, error)
}

// BatchSVIDIssuer issues SVIDs in batches for efficiency.
type BatchSVIDIssuer struct {
	config   *BatchConfig
	provider SVIDIssuerProvider
	mu       sync.Mutex

	// Pending requests
	pending   []*batchRequest
	pendingCh chan struct{}
	processCh chan struct{}

	// Metrics
	batchesProcessed int64
	itemsProcessed   int64
	itemsFailed      int64

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// batchRequest represents a pending batch request.
type batchRequest struct {
	req      *X509SVIDRequest
	resultCh chan *batchResponse
}

// batchResponse is the response for a batch request.
type batchResponse struct {
	svid *X509SVID
	err  error
}

// NewBatchSVIDIssuer creates a new batch SVID issuer.
func NewBatchSVIDIssuer(config *BatchConfig, provider SVIDIssuerProvider) *BatchSVIDIssuer {
	if config == nil {
		config = DefaultBatchConfig()
	}

	b := &BatchSVIDIssuer{
		config:    config,
		provider:  provider,
		pending:   make([]*batchRequest, 0, config.MaxBatchSize),
		pendingCh: make(chan struct{}, 1),
		processCh: make(chan struct{}, config.MaxConcurrentBatches),
		stopCh:    make(chan struct{}),
	}

	// Start batch processor
	b.wg.Add(1)
	go b.processLoop()

	return b
}

// IssueX509SVID issues an SVID, batching with other requests.
func (b *BatchSVIDIssuer) IssueX509SVID(ctx context.Context, req *X509SVIDRequest) (*X509SVID, error) {
	resultCh := make(chan *batchResponse, 1)

	br := &batchRequest{
		req:      req,
		resultCh: resultCh,
	}

	// Add to pending queue
	b.mu.Lock()
	b.pending = append(b.pending, br)
	shouldProcess := len(b.pending) >= b.config.MaxBatchSize
	b.mu.Unlock()

	// Signal that there's work to do
	select {
	case b.pendingCh <- struct{}{}:
	default:
	}

	// Trigger immediate processing if batch is full
	if shouldProcess {
		select {
		case b.processCh <- struct{}{}:
		default:
		}
	}

	// Wait for result
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case resp := <-resultCh:
		return resp.svid, resp.err
	}
}

// Close closes the batch issuer.
func (b *BatchSVIDIssuer) Close() {
	close(b.stopCh)
	b.wg.Wait()
}

// GetMetrics returns batch issuer metrics.
func (b *BatchSVIDIssuer) GetMetrics() map[string]int64 {
	return map[string]int64{
		"batches_processed": b.batchesProcessed,
		"items_processed":   b.itemsProcessed,
		"items_failed":      b.itemsFailed,
	}
}

// processLoop processes batches.
func (b *BatchSVIDIssuer) processLoop() {
	defer b.wg.Done()

	timer := time.NewTimer(b.config.BatchTimeout)
	defer timer.Stop()

	for {
		select {
		case <-b.stopCh:
			// Process remaining items
			b.processBatch()
			return

		case <-b.pendingCh:
			// Reset timer when work arrives
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			timer.Reset(b.config.BatchTimeout)

		case <-timer.C:
			// Timeout - process whatever we have
			b.processBatch()
			timer.Reset(b.config.BatchTimeout)

		case <-b.processCh:
			// Batch is full - process immediately
			b.processBatch()
		}
	}
}

// processBatch processes the current batch of requests.
func (b *BatchSVIDIssuer) processBatch() {
	b.mu.Lock()
	if len(b.pending) == 0 {
		b.mu.Unlock()
		return
	}

	// Get batch
	batch := b.pending
	b.pending = make([]*batchRequest, 0, b.config.MaxBatchSize)
	b.mu.Unlock()

	// Process batch
	ctx := context.Background()

	for _, req := range batch {
		svid, err := b.provider.IssueX509SVID(ctx, req.req)

		if err != nil {
			b.itemsFailed++
		} else {
			b.itemsProcessed++
		}

		req.resultCh <- &batchResponse{
			svid: svid,
			err:  err,
		}
	}

	b.batchesProcessed++
}

// ConnectionPoolConfig configures the connection pool.
type ConnectionPoolConfig struct {
	// MaxConnections is the maximum number of connections.
	MaxConnections int

	// MinConnections is the minimum number of connections to keep.
	MinConnections int

	// MaxIdleTime is how long a connection can be idle before being closed.
	MaxIdleTime time.Duration

	// ConnectionTimeout is the timeout for creating new connections.
	ConnectionTimeout time.Duration

	// HealthCheckInterval is how often to check connection health.
	HealthCheckInterval time.Duration
}

// DefaultConnectionPoolConfig returns a default connection pool configuration.
func DefaultConnectionPoolConfig() *ConnectionPoolConfig {
	return &ConnectionPoolConfig{
		MaxConnections:      100,
		MinConnections:      10,
		MaxIdleTime:         5 * time.Minute,
		ConnectionTimeout:   10 * time.Second,
		HealthCheckInterval: 30 * time.Second,
	}
}

// PooledConnection represents a pooled connection.
type PooledConnection struct {
	id        string
	conn      interface{}
	createdAt time.Time
	lastUsed  time.Time
	inUse     bool
}

// ConnectionPool manages a pool of connections.
type ConnectionPool struct {
	config *ConnectionPoolConfig
	mu     sync.Mutex
	cond   *sync.Cond

	// Pool of connections
	connections []*PooledConnection
	available   int

	// Factory for creating new connections
	factory func(context.Context) (interface{}, error)

	// Health check function
	healthCheck func(interface{}) bool

	// Metrics
	created   int64
	destroyed int64
	reused    int64
	waits     int64
	waitTime  time.Duration

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// NewConnectionPool creates a new connection pool.
func NewConnectionPool(config *ConnectionPoolConfig, factory func(context.Context) (interface{}, error)) (*ConnectionPool, error) {
	if config == nil {
		config = DefaultConnectionPoolConfig()
	}

	if factory == nil {
		return nil, fmt.Errorf("connection factory required")
	}

	p := &ConnectionPool{
		config:      config,
		connections: make([]*PooledConnection, 0, config.MaxConnections),
		factory:     factory,
		stopCh:      make(chan struct{}),
	}
	p.cond = sync.NewCond(&p.mu)

	// Create minimum connections
	ctx := context.Background()
	for i := 0; i < config.MinConnections; i++ {
		conn, err := factory(ctx)
		if err != nil {
			continue
		}

		pc := &PooledConnection{
			id:        generateConnectionID(),
			conn:      conn,
			createdAt: time.Now(),
			lastUsed:  time.Now(),
			inUse:     false,
		}
		p.connections = append(p.connections, pc)
		p.available++
		p.created++
	}

	// Start maintenance goroutine
	p.wg.Add(1)
	go p.maintenanceLoop()

	return p, nil
}

// Get gets a connection from the pool.
func (p *ConnectionPool) Get(ctx context.Context) (interface{}, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	start := time.Now()

	for {
		// Try to find an available connection
		for _, pc := range p.connections {
			if pc.inUse {
				continue
			}
			pc.inUse = true
			pc.lastUsed = time.Now()
			p.available--
			p.reused++
			return pc.conn, nil
		}

		// No available connections - check if we can create one
		if len(p.connections) < p.config.MaxConnections {
			p.mu.Unlock()
			conn, err := p.factory(ctx)
			p.mu.Lock()

			if err != nil {
				return nil, err
			}

			pc := &PooledConnection{
				id:        generateConnectionID(),
				conn:      conn,
				createdAt: time.Now(),
				lastUsed:  time.Now(),
				inUse:     true,
			}
			p.connections = append(p.connections, pc)
			p.created++
			return pc.conn, nil
		}

		// Wait for a connection to become available
		p.waits++

		// Set up timeout
		done := make(chan struct{})
		go func() {
			select {
			case <-ctx.Done():
				p.cond.Broadcast()
			case <-done:
			}
		}()

		p.cond.Wait()
		close(done)

		if ctx.Err() != nil {
			p.waitTime += time.Since(start)
			return nil, ctx.Err()
		}
	}
}

// Put returns a connection to the pool.
func (p *ConnectionPool) Put(conn interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, pc := range p.connections {
		if pc.conn != conn {
			continue
		}
		pc.inUse = false
		pc.lastUsed = time.Now()
		p.available++
		p.cond.Signal()
		return
	}
}

// Remove removes a connection from the pool (e.g., if it's unhealthy).
func (p *ConnectionPool) Remove(conn interface{}) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for i, pc := range p.connections {
		if pc.conn == conn {
			p.connections = append(p.connections[:i], p.connections[i+1:]...)
			if !pc.inUse {
				p.available--
			}
			p.destroyed++
			return
		}
	}
}

// SetHealthCheck sets the health check function.
func (p *ConnectionPool) SetHealthCheck(check func(interface{}) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.healthCheck = check
}

// GetMetrics returns pool metrics.
func (p *ConnectionPool) GetMetrics() map[string]interface{} {
	p.mu.Lock()
	defer p.mu.Unlock()

	return map[string]interface{}{
		"total":       len(p.connections),
		"available":   p.available,
		"in_use":      len(p.connections) - p.available,
		"created":     p.created,
		"destroyed":   p.destroyed,
		"reused":      p.reused,
		"waits":       p.waits,
		"avg_wait_ms": float64(p.waitTime.Milliseconds()) / float64(max(p.waits, 1)),
	}
}

// Close closes the pool and all connections.
func (p *ConnectionPool) Close() {
	close(p.stopCh)
	p.wg.Wait()

	p.mu.Lock()
	defer p.mu.Unlock()

	// Close all connections (if they implement io.Closer)
	for _, pc := range p.connections {
		if closer, ok := pc.conn.(interface{ Close() error }); ok {
			closer.Close()
		}
	}

	p.connections = nil
	p.available = 0
}

// maintenanceLoop performs periodic maintenance.
func (p *ConnectionPool) maintenanceLoop() {
	defer p.wg.Done()

	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.performMaintenance()
		}
	}
}

// performMaintenance performs pool maintenance.
func (p *ConnectionPool) performMaintenance() {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := time.Now()
	toRemove := make([]int, 0)

	for i, pc := range p.connections {
		// Skip in-use connections
		if pc.inUse {
			continue
		}

		// Check idle timeout
		if now.Sub(pc.lastUsed) > p.config.MaxIdleTime {
			// Keep minimum connections
			if len(p.connections)-len(toRemove) > p.config.MinConnections {
				toRemove = append(toRemove, i)
				continue
			}
		}

		// Health check
		if p.healthCheck != nil && !p.healthCheck(pc.conn) {
			toRemove = append(toRemove, i)
		}
	}

	// Remove connections in reverse order
	for i := len(toRemove) - 1; i >= 0; i-- {
		idx := toRemove[i]
		pc := p.connections[idx]

		if closer, ok := pc.conn.(interface{ Close() error }); ok {
			closer.Close()
		}

		p.connections = append(p.connections[:idx], p.connections[idx+1:]...)
		p.available--
		p.destroyed++
	}

	// Ensure minimum connections
	for len(p.connections) < p.config.MinConnections {
		ctx, cancel := context.WithTimeout(context.Background(), p.config.ConnectionTimeout)
		conn, err := p.factory(ctx)
		cancel()

		if err != nil {
			break
		}

		pc := &PooledConnection{
			id:        generateConnectionID(),
			conn:      conn,
			createdAt: now,
			lastUsed:  now,
			inUse:     false,
		}
		p.connections = append(p.connections, pc)
		p.available++
		p.created++
	}
}

// generateConnectionID generates a unique connection ID.
func generateConnectionID() string {
	data := fmt.Sprintf("%d-%d", time.Now().UnixNano(), time.Now().Nanosecond())
	hash := sha256.Sum256([]byte(data))
	return hex.EncodeToString(hash[:8])
}

