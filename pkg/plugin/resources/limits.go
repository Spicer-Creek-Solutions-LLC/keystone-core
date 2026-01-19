// Package resources provides configurable resource limits for Keystone plugin modules.
package resources

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Common errors.
var (
	ErrCPUTimeExceeded    = errors.New("CPU time limit exceeded")
	ErrMemoryExceeded     = errors.New("memory limit exceeded")
	ErrTimeoutExceeded    = errors.New("execution timeout exceeded")
	ErrNetworkDisabled    = errors.New("network access is disabled")
	ErrFilesystemDisabled = errors.New("filesystem access is disabled")
	ErrIOPSExceeded       = errors.New("IOPS limit exceeded")
)

// Limits defines resource limits for a module.
type Limits struct {
	// CPUTime is the maximum CPU time allowed.
	CPUTime time.Duration `json:"cpuTime,omitempty"`
	// WallTime is the maximum wall clock time allowed.
	WallTime time.Duration `json:"wallTime,omitempty"`
	// Memory is the maximum memory in bytes.
	Memory int64 `json:"memory,omitempty"`
	// MaxGoroutines is the maximum number of goroutines.
	MaxGoroutines int `json:"maxGoroutines,omitempty"`
	// MaxFileSize is the maximum file size in bytes.
	MaxFileSize int64 `json:"maxFileSize,omitempty"`
	// MaxOpenFiles is the maximum number of open files.
	MaxOpenFiles int `json:"maxOpenFiles,omitempty"`
	// MaxIOPS is the maximum I/O operations per second.
	MaxIOPS int64 `json:"maxIOPS,omitempty"`
	// MaxNetworkConns is the maximum network connections.
	MaxNetworkConns int `json:"maxNetworkConns,omitempty"`
	// AllowNetwork enables network access.
	AllowNetwork bool `json:"allowNetwork"`
	// AllowFilesystem enables filesystem access.
	AllowFilesystem bool `json:"allowFilesystem"`
	// AllowExec enables subprocess execution.
	AllowExec bool `json:"allowExec"`
}

// DefaultLimits returns default resource limits.
func DefaultLimits() *Limits {
	return &Limits{
		CPUTime:         30 * time.Second,
		WallTime:        60 * time.Second,
		Memory:          256 * 1024 * 1024, // 256 MB
		MaxGoroutines:   100,
		MaxFileSize:     100 * 1024 * 1024, // 100 MB
		MaxOpenFiles:    100,
		MaxIOPS:         1000,
		MaxNetworkConns: 10,
		AllowNetwork:    true,
		AllowFilesystem: true,
		AllowExec:       false,
	}
}

// RestrictedLimits returns restrictive limits for untrusted code.
func RestrictedLimits() *Limits {
	return &Limits{
		CPUTime:         5 * time.Second,
		WallTime:        10 * time.Second,
		Memory:          64 * 1024 * 1024, // 64 MB
		MaxGoroutines:   10,
		MaxFileSize:     10 * 1024 * 1024, // 10 MB
		MaxOpenFiles:    10,
		MaxIOPS:         100,
		MaxNetworkConns: 0,
		AllowNetwork:    false,
		AllowFilesystem: false,
		AllowExec:       false,
	}
}

// Usage tracks resource usage.
type Usage struct {
	CPUTime       time.Duration `json:"cpuTime"`
	WallTime      time.Duration `json:"wallTime"`
	Memory        int64         `json:"memory"`
	PeakMemory    int64         `json:"peakMemory"`
	Goroutines    int           `json:"goroutines"`
	OpenFiles     int64         `json:"openFiles"`
	IOPS          int64         `json:"iops"`
	NetworkConns  int64         `json:"networkConns"`
	BytesRead     int64         `json:"bytesRead"`
	BytesWritten  int64         `json:"bytesWritten"`
}

// Enforcer enforces resource limits.
type Enforcer struct {
	limits    *Limits
	startTime time.Time
	usage     *usage
	stopCh    chan struct{}
	doneCh    chan struct{}
	mu        sync.RWMutex
	listeners []LimitEventListener
	violated  error
}

type usage struct {
	cpuTime      int64 // nanoseconds, atomic
	memory       int64 // atomic
	peakMemory   int64 // atomic
	goroutines   int64 // atomic
	openFiles    int64 // atomic
	iops         int64 // atomic
	networkConns int64 // atomic
	bytesRead    int64 // atomic
	bytesWritten int64 // atomic
}

// LimitEvent represents a limit event.
type LimitEvent struct {
	Type      string    `json:"type"`
	Limit     string    `json:"limit"`
	Current   int64     `json:"current"`
	Maximum   int64     `json:"maximum"`
	Timestamp time.Time `json:"timestamp"`
}

// LimitEventListener is called when limit events occur.
type LimitEventListener func(*LimitEvent)

// NewEnforcer creates a new resource limit enforcer.
func NewEnforcer(limits *Limits) *Enforcer {
	if limits == nil {
		limits = DefaultLimits()
	}

	return &Enforcer{
		limits:    limits,
		startTime: time.Now(),
		usage:     &usage{},
		stopCh:    make(chan struct{}),
		doneCh:    make(chan struct{}),
	}
}

// Start starts enforcing limits.
func (e *Enforcer) Start(ctx context.Context) error {
	go e.monitor(ctx)
	return nil
}

// Stop stops enforcing limits.
func (e *Enforcer) Stop() {
	close(e.stopCh)
	<-e.doneCh
}

// AddListener adds an event listener.
func (e *Enforcer) AddListener(listener LimitEventListener) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.listeners = append(e.listeners, listener)
}

// CheckNetwork checks if network access is allowed.
func (e *Enforcer) CheckNetwork() error {
	if !e.limits.AllowNetwork {
		return ErrNetworkDisabled
	}
	return nil
}

// CheckFilesystem checks if filesystem access is allowed.
func (e *Enforcer) CheckFilesystem() error {
	if !e.limits.AllowFilesystem {
		return ErrFilesystemDisabled
	}
	return nil
}

// CheckExec checks if subprocess execution is allowed.
func (e *Enforcer) CheckExec() error {
	if !e.limits.AllowExec {
		return errors.New("subprocess execution is disabled")
	}
	return nil
}

// RecordMemory records memory usage.
func (e *Enforcer) RecordMemory(bytes int64) error {
	current := atomic.AddInt64(&e.usage.memory, bytes)

	// Update peak
	for {
		peak := atomic.LoadInt64(&e.usage.peakMemory)
		if current <= peak {
			break
		}
		if atomic.CompareAndSwapInt64(&e.usage.peakMemory, peak, current) {
			break
		}
	}

	if e.limits.Memory > 0 && current > e.limits.Memory {
		e.emit(&LimitEvent{
			Type:      "exceeded",
			Limit:     "memory",
			Current:   current,
			Maximum:   e.limits.Memory,
			Timestamp: time.Now(),
		})
		return ErrMemoryExceeded
	}

	return nil
}

// ReleaseMemory releases memory.
func (e *Enforcer) ReleaseMemory(bytes int64) {
	atomic.AddInt64(&e.usage.memory, -bytes)
}

// RecordGoroutine records a goroutine being started.
func (e *Enforcer) RecordGoroutine() error {
	current := atomic.AddInt64(&e.usage.goroutines, 1)

	if e.limits.MaxGoroutines > 0 && int(current) > e.limits.MaxGoroutines {
		atomic.AddInt64(&e.usage.goroutines, -1)
		e.emit(&LimitEvent{
			Type:      "exceeded",
			Limit:     "goroutines",
			Current:   current,
			Maximum:   int64(e.limits.MaxGoroutines),
			Timestamp: time.Now(),
		})
		return errors.New("goroutine limit exceeded")
	}

	return nil
}

// ReleaseGoroutine releases a goroutine.
func (e *Enforcer) ReleaseGoroutine() {
	atomic.AddInt64(&e.usage.goroutines, -1)
}

// RecordOpenFile records a file being opened.
func (e *Enforcer) RecordOpenFile() error {
	current := atomic.AddInt64(&e.usage.openFiles, 1)

	if e.limits.MaxOpenFiles > 0 && int(current) > e.limits.MaxOpenFiles {
		atomic.AddInt64(&e.usage.openFiles, -1)
		e.emit(&LimitEvent{
			Type:      "exceeded",
			Limit:     "openFiles",
			Current:   current,
			Maximum:   int64(e.limits.MaxOpenFiles),
			Timestamp: time.Now(),
		})
		return errors.New("open file limit exceeded")
	}

	return nil
}

// ReleaseOpenFile releases an open file.
func (e *Enforcer) ReleaseOpenFile() {
	atomic.AddInt64(&e.usage.openFiles, -1)
}

// RecordIOPS records an I/O operation.
func (e *Enforcer) RecordIOPS() error {
	current := atomic.AddInt64(&e.usage.iops, 1)

	// IOPS is checked per second in monitor loop
	_ = current

	return nil
}

// RecordNetworkConn records a network connection.
func (e *Enforcer) RecordNetworkConn() error {
	if !e.limits.AllowNetwork {
		return ErrNetworkDisabled
	}

	current := atomic.AddInt64(&e.usage.networkConns, 1)

	if e.limits.MaxNetworkConns > 0 && int(current) > e.limits.MaxNetworkConns {
		atomic.AddInt64(&e.usage.networkConns, -1)
		e.emit(&LimitEvent{
			Type:      "exceeded",
			Limit:     "networkConns",
			Current:   current,
			Maximum:   int64(e.limits.MaxNetworkConns),
			Timestamp: time.Now(),
		})
		return errors.New("network connection limit exceeded")
	}

	return nil
}

// ReleaseNetworkConn releases a network connection.
func (e *Enforcer) ReleaseNetworkConn() {
	atomic.AddInt64(&e.usage.networkConns, -1)
}

// RecordBytes records bytes read or written.
func (e *Enforcer) RecordBytes(read, written int64) {
	if read > 0 {
		atomic.AddInt64(&e.usage.bytesRead, read)
	}
	if written > 0 {
		atomic.AddInt64(&e.usage.bytesWritten, written)
	}
}

// CheckFileSize checks if a file size is within limits.
func (e *Enforcer) CheckFileSize(size int64) error {
	if e.limits.MaxFileSize > 0 && size > e.limits.MaxFileSize {
		return errors.New("file size limit exceeded")
	}
	return nil
}

// Usage returns current usage.
func (e *Enforcer) Usage() Usage {
	return Usage{
		CPUTime:      time.Duration(atomic.LoadInt64(&e.usage.cpuTime)),
		WallTime:     time.Since(e.startTime),
		Memory:       atomic.LoadInt64(&e.usage.memory),
		PeakMemory:   atomic.LoadInt64(&e.usage.peakMemory),
		Goroutines:   int(atomic.LoadInt64(&e.usage.goroutines)),
		OpenFiles:    atomic.LoadInt64(&e.usage.openFiles),
		IOPS:         atomic.LoadInt64(&e.usage.iops),
		NetworkConns: atomic.LoadInt64(&e.usage.networkConns),
		BytesRead:    atomic.LoadInt64(&e.usage.bytesRead),
		BytesWritten: atomic.LoadInt64(&e.usage.bytesWritten),
	}
}

// Violated returns the first violated limit error, if any.
func (e *Enforcer) Violated() error {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.violated
}

func (e *Enforcer) monitor(ctx context.Context) {
	defer close(e.doneCh)

	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	iopsResetTicker := time.NewTicker(time.Second)
	defer iopsResetTicker.Stop()

	var lastCPUTime int64

	for {
		select {
		case <-ctx.Done():
			return
		case <-e.stopCh:
			return
		case <-ticker.C:
			// Check wall time
			wallTime := time.Since(e.startTime)
			if e.limits.WallTime > 0 && wallTime > e.limits.WallTime {
				e.setViolated(ErrTimeoutExceeded)
				e.emit(&LimitEvent{
					Type:      "exceeded",
					Limit:     "wallTime",
					Current:   int64(wallTime),
					Maximum:   int64(e.limits.WallTime),
					Timestamp: time.Now(),
				})
			}

			// Estimate CPU time from runtime stats
			var m runtime.MemStats
			runtime.ReadMemStats(&m)

			// Update memory usage from runtime (this is a simplification)
			atomic.StoreInt64(&e.usage.memory, int64(m.Alloc))

			for {
				peak := atomic.LoadInt64(&e.usage.peakMemory)
				if int64(m.Alloc) <= peak {
					break
				}
				if atomic.CompareAndSwapInt64(&e.usage.peakMemory, peak, int64(m.Alloc)) {
					break
				}
			}

			// Check memory limit
			if e.limits.Memory > 0 && int64(m.Alloc) > e.limits.Memory {
				e.setViolated(ErrMemoryExceeded)
				e.emit(&LimitEvent{
					Type:      "exceeded",
					Limit:     "memory",
					Current:   int64(m.Alloc),
					Maximum:   e.limits.Memory,
					Timestamp: time.Now(),
				})
			}

			// Estimate CPU time
			currentCPUTime := int64(m.TotalAlloc) // Rough proxy for CPU work
			if lastCPUTime > 0 {
				cpuDelta := currentCPUTime - lastCPUTime
				atomic.AddInt64(&e.usage.cpuTime, cpuDelta/1000) // Scale down
			}
			lastCPUTime = currentCPUTime

			// Check CPU time limit
			cpuTime := time.Duration(atomic.LoadInt64(&e.usage.cpuTime))
			if e.limits.CPUTime > 0 && cpuTime > e.limits.CPUTime {
				e.setViolated(ErrCPUTimeExceeded)
				e.emit(&LimitEvent{
					Type:      "exceeded",
					Limit:     "cpuTime",
					Current:   int64(cpuTime),
					Maximum:   int64(e.limits.CPUTime),
					Timestamp: time.Now(),
				})
			}

		case <-iopsResetTicker.C:
			// Check IOPS before reset
			iops := atomic.LoadInt64(&e.usage.iops)
			if e.limits.MaxIOPS > 0 && iops > e.limits.MaxIOPS {
				e.setViolated(ErrIOPSExceeded)
				e.emit(&LimitEvent{
					Type:      "exceeded",
					Limit:     "iops",
					Current:   iops,
					Maximum:   e.limits.MaxIOPS,
					Timestamp: time.Now(),
				})
			}
			// Reset IOPS counter
			atomic.StoreInt64(&e.usage.iops, 0)
		}
	}
}

func (e *Enforcer) setViolated(err error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.violated == nil {
		e.violated = err
	}
}

func (e *Enforcer) emit(event *LimitEvent) {
	e.mu.RLock()
	listeners := e.listeners
	e.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

// LimitedContext wraps a context with resource limit checking.
type LimitedContext struct {
	context.Context
	enforcer *Enforcer
}

// NewLimitedContext creates a new limited context.
func NewLimitedContext(ctx context.Context, enforcer *Enforcer) *LimitedContext {
	return &LimitedContext{
		Context:  ctx,
		enforcer: enforcer,
	}
}

// Enforcer returns the enforcer.
func (lc *LimitedContext) Enforcer() *Enforcer {
	return lc.enforcer
}

// Err returns context error or limit violation.
func (lc *LimitedContext) Err() error {
	if err := lc.Context.Err(); err != nil {
		return err
	}
	return lc.enforcer.Violated()
}

// Pool manages resource pools.
type Pool struct {
	name      string
	total     int64
	available int64 // atomic
	mu        sync.Mutex
}

// NewPool creates a new resource pool.
func NewPool(name string, total int64) *Pool {
	return &Pool{
		name:      name,
		total:     total,
		available: total,
	}
}

// Acquire acquires resources from the pool.
func (p *Pool) Acquire(ctx context.Context, amount int64) error {
	for {
		available := atomic.LoadInt64(&p.available)
		if available >= amount {
			if atomic.CompareAndSwapInt64(&p.available, available, available-amount) {
				return nil
			}
			continue
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Millisecond):
			// Retry
		}
	}
}

// TryAcquire tries to acquire without blocking.
func (p *Pool) TryAcquire(amount int64) bool {
	for {
		available := atomic.LoadInt64(&p.available)
		if available < amount {
			return false
		}
		if atomic.CompareAndSwapInt64(&p.available, available, available-amount) {
			return true
		}
	}
}

// Release releases resources back to the pool.
func (p *Pool) Release(amount int64) {
	atomic.AddInt64(&p.available, amount)
}

// Available returns available resources.
func (p *Pool) Available() int64 {
	return atomic.LoadInt64(&p.available)
}

// Total returns total resources.
func (p *Pool) Total() int64 {
	return p.total
}

// ResourceManager manages multiple resource pools.
type ResourceManager struct {
	pools map[string]*Pool
	mu    sync.RWMutex
}

// NewResourceManager creates a new resource manager.
func NewResourceManager() *ResourceManager {
	return &ResourceManager{
		pools: make(map[string]*Pool),
	}
}

// CreatePool creates a new pool.
func (rm *ResourceManager) CreatePool(name string, total int64) *Pool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	pool := NewPool(name, total)
	rm.pools[name] = pool
	return pool
}

// GetPool returns a pool by name.
func (rm *ResourceManager) GetPool(name string) *Pool {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.pools[name]
}

// Stats returns stats for all pools.
func (rm *ResourceManager) Stats() map[string]PoolStats {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	stats := make(map[string]PoolStats)
	for name, pool := range rm.pools {
		stats[name] = PoolStats{
			Name:      name,
			Total:     pool.Total(),
			Available: pool.Available(),
			Used:      pool.Total() - pool.Available(),
		}
	}
	return stats
}

// PoolStats contains pool statistics.
type PoolStats struct {
	Name      string `json:"name"`
	Total     int64  `json:"total"`
	Available int64  `json:"available"`
	Used      int64  `json:"used"`
}
