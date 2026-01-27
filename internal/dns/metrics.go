package dns

import (
	"sync"
	"sync/atomic"
	"time"
)

// Metrics collects DNS operation metrics.
type Metrics struct {
	mu sync.RWMutex

	// Operation counts
	getRecordsCalls    atomic.Int64
	createRecordCalls  atomic.Int64
	updateRecordCalls  atomic.Int64
	deleteRecordCalls  atomic.Int64

	// Success/failure counts
	getRecordsSuccess    atomic.Int64
	getRecordsFailure    atomic.Int64
	createRecordSuccess  atomic.Int64
	createRecordFailure  atomic.Int64
	updateRecordSuccess  atomic.Int64
	updateRecordFailure  atomic.Int64
	deleteRecordSuccess  atomic.Int64
	deleteRecordFailure  atomic.Int64

	// Sync operation counts
	syncCalls   atomic.Int64
	syncSuccess atomic.Int64
	syncFailure atomic.Int64

	// Record change counts
	recordsCreated atomic.Int64
	recordsUpdated atomic.Int64
	recordsDeleted atomic.Int64

	// Latency tracking
	latencies     map[string][]time.Duration
	latenciesMu   sync.RWMutex
	maxLatencies  int

	// Provider-specific metrics
	providerMetrics map[string]*ProviderMetrics
}

// ProviderMetrics tracks metrics for a specific provider.
type ProviderMetrics struct {
	Name           string
	Calls          atomic.Int64
	Errors         atomic.Int64
	TotalLatencyNs atomic.Int64
}

// NewMetrics creates a new metrics collector.
func NewMetrics() *Metrics {
	return &Metrics{
		latencies:       make(map[string][]time.Duration),
		maxLatencies:    1000, // Keep last 1000 latency samples per operation
		providerMetrics: make(map[string]*ProviderMetrics),
	}
}

// RecordGetRecords records a GetRecords operation.
func (m *Metrics) RecordGetRecords(provider string, duration time.Duration, err error) {
	m.getRecordsCalls.Add(1)
	if err != nil {
		m.getRecordsFailure.Add(1)
	} else {
		m.getRecordsSuccess.Add(1)
	}
	m.recordLatency("get_records", duration)
	m.recordProviderCall(provider, duration, err)
}

// RecordCreateRecord records a CreateRecord operation.
func (m *Metrics) RecordCreateRecord(provider string, duration time.Duration, err error) {
	m.createRecordCalls.Add(1)
	if err != nil {
		m.createRecordFailure.Add(1)
	} else {
		m.createRecordSuccess.Add(1)
		m.recordsCreated.Add(1)
	}
	m.recordLatency("create_record", duration)
	m.recordProviderCall(provider, duration, err)
}

// RecordUpdateRecord records an UpdateRecord operation.
func (m *Metrics) RecordUpdateRecord(provider string, duration time.Duration, err error) {
	m.updateRecordCalls.Add(1)
	if err != nil {
		m.updateRecordFailure.Add(1)
	} else {
		m.updateRecordSuccess.Add(1)
		m.recordsUpdated.Add(1)
	}
	m.recordLatency("update_record", duration)
	m.recordProviderCall(provider, duration, err)
}

// RecordDeleteRecord records a DeleteRecord operation.
func (m *Metrics) RecordDeleteRecord(provider string, duration time.Duration, err error) {
	m.deleteRecordCalls.Add(1)
	if err != nil {
		m.deleteRecordFailure.Add(1)
	} else {
		m.deleteRecordSuccess.Add(1)
		m.recordsDeleted.Add(1)
	}
	m.recordLatency("delete_record", duration)
	m.recordProviderCall(provider, duration, err)
}

// RecordSync records a sync operation.
func (m *Metrics) RecordSync(provider string, duration time.Duration, result *SyncResult) {
	m.syncCalls.Add(1)
	if result != nil && result.HasErrors() {
		m.syncFailure.Add(1)
	} else {
		m.syncSuccess.Add(1)
	}
	m.recordLatency("sync", duration)
	m.recordProviderCall(provider, duration, nil)

	if result != nil {
		m.recordsCreated.Add(int64(result.Created))
		m.recordsUpdated.Add(int64(result.Updated))
		m.recordsDeleted.Add(int64(result.Deleted))
	}
}

// recordLatency records a latency sample.
func (m *Metrics) recordLatency(operation string, duration time.Duration) {
	m.latenciesMu.Lock()
	defer m.latenciesMu.Unlock()

	samples := m.latencies[operation]
	if len(samples) >= m.maxLatencies {
		// Remove oldest sample
		samples = samples[1:]
	}
	m.latencies[operation] = append(samples, duration)
}

// recordProviderCall records a call to a specific provider.
func (m *Metrics) recordProviderCall(provider string, duration time.Duration, err error) {
	m.mu.Lock()
	pm, exists := m.providerMetrics[provider]
	if !exists {
		pm = &ProviderMetrics{Name: provider}
		m.providerMetrics[provider] = pm
	}
	m.mu.Unlock()

	pm.Calls.Add(1)
	pm.TotalLatencyNs.Add(duration.Nanoseconds())
	if err != nil {
		pm.Errors.Add(1)
	}
}

// Snapshot returns a snapshot of current metrics.
type MetricsSnapshot struct {
	// Operation counts
	GetRecordsCalls   int64
	CreateRecordCalls int64
	UpdateRecordCalls int64
	DeleteRecordCalls int64

	// Success counts
	GetRecordsSuccess   int64
	CreateRecordSuccess int64
	UpdateRecordSuccess int64
	DeleteRecordSuccess int64

	// Failure counts
	GetRecordsFailure   int64
	CreateRecordFailure int64
	UpdateRecordFailure int64
	DeleteRecordFailure int64

	// Sync counts
	SyncCalls   int64
	SyncSuccess int64
	SyncFailure int64

	// Record change counts
	RecordsCreated int64
	RecordsUpdated int64
	RecordsDeleted int64

	// Latency percentiles (milliseconds)
	LatencyP50 map[string]float64
	LatencyP95 map[string]float64
	LatencyP99 map[string]float64

	// Provider metrics
	Providers map[string]ProviderMetricsSnapshot
}

// ProviderMetricsSnapshot is a snapshot of provider metrics.
type ProviderMetricsSnapshot struct {
	Name           string
	Calls          int64
	Errors         int64
	AvgLatencyMs   float64
	ErrorRate      float64
}

// Snapshot returns a snapshot of current metrics.
func (m *Metrics) Snapshot() *MetricsSnapshot {
	snap := &MetricsSnapshot{
		GetRecordsCalls:     m.getRecordsCalls.Load(),
		CreateRecordCalls:   m.createRecordCalls.Load(),
		UpdateRecordCalls:   m.updateRecordCalls.Load(),
		DeleteRecordCalls:   m.deleteRecordCalls.Load(),
		GetRecordsSuccess:   m.getRecordsSuccess.Load(),
		CreateRecordSuccess: m.createRecordSuccess.Load(),
		UpdateRecordSuccess: m.updateRecordSuccess.Load(),
		DeleteRecordSuccess: m.deleteRecordSuccess.Load(),
		GetRecordsFailure:   m.getRecordsFailure.Load(),
		CreateRecordFailure: m.createRecordFailure.Load(),
		UpdateRecordFailure: m.updateRecordFailure.Load(),
		DeleteRecordFailure: m.deleteRecordFailure.Load(),
		SyncCalls:           m.syncCalls.Load(),
		SyncSuccess:         m.syncSuccess.Load(),
		SyncFailure:         m.syncFailure.Load(),
		RecordsCreated:      m.recordsCreated.Load(),
		RecordsUpdated:      m.recordsUpdated.Load(),
		RecordsDeleted:      m.recordsDeleted.Load(),
		LatencyP50:          make(map[string]float64),
		LatencyP95:          make(map[string]float64),
		LatencyP99:          make(map[string]float64),
		Providers:           make(map[string]ProviderMetricsSnapshot),
	}

	// Calculate latency percentiles
	m.latenciesMu.RLock()
	for op, samples := range m.latencies {
		if len(samples) > 0 {
			snap.LatencyP50[op] = percentile(samples, 50)
			snap.LatencyP95[op] = percentile(samples, 95)
			snap.LatencyP99[op] = percentile(samples, 99)
		}
	}
	m.latenciesMu.RUnlock()

	// Collect provider metrics
	m.mu.RLock()
	for name, pm := range m.providerMetrics {
		calls := pm.Calls.Load()
		errors := pm.Errors.Load()
		totalNs := pm.TotalLatencyNs.Load()

		var avgLatencyMs float64
		if calls > 0 {
			avgLatencyMs = float64(totalNs) / float64(calls) / 1e6
		}

		var errorRate float64
		if calls > 0 {
			errorRate = float64(errors) / float64(calls)
		}

		snap.Providers[name] = ProviderMetricsSnapshot{
			Name:         name,
			Calls:        calls,
			Errors:       errors,
			AvgLatencyMs: avgLatencyMs,
			ErrorRate:    errorRate,
		}
	}
	m.mu.RUnlock()

	return snap
}

// Reset resets all metrics.
func (m *Metrics) Reset() {
	m.getRecordsCalls.Store(0)
	m.createRecordCalls.Store(0)
	m.updateRecordCalls.Store(0)
	m.deleteRecordCalls.Store(0)
	m.getRecordsSuccess.Store(0)
	m.getRecordsFailure.Store(0)
	m.createRecordSuccess.Store(0)
	m.createRecordFailure.Store(0)
	m.updateRecordSuccess.Store(0)
	m.updateRecordFailure.Store(0)
	m.deleteRecordSuccess.Store(0)
	m.deleteRecordFailure.Store(0)
	m.syncCalls.Store(0)
	m.syncSuccess.Store(0)
	m.syncFailure.Store(0)
	m.recordsCreated.Store(0)
	m.recordsUpdated.Store(0)
	m.recordsDeleted.Store(0)

	m.latenciesMu.Lock()
	m.latencies = make(map[string][]time.Duration)
	m.latenciesMu.Unlock()

	m.mu.Lock()
	m.providerMetrics = make(map[string]*ProviderMetrics)
	m.mu.Unlock()
}

// percentile calculates the pth percentile of durations (returns milliseconds).
func percentile(samples []time.Duration, p int) float64 {
	if len(samples) == 0 {
		return 0
	}

	// Sort samples
	sorted := make([]time.Duration, len(samples))
	copy(sorted, samples)
	sortDurations(sorted)

	// Calculate index
	idx := int(float64(len(sorted)-1) * float64(p) / 100.0)
	return float64(sorted[idx].Nanoseconds()) / 1e6 // Convert to milliseconds
}

// sortDurations sorts durations in ascending order.
func sortDurations(d []time.Duration) {
	for i := 1; i < len(d); i++ {
		for j := i; j > 0 && d[j] < d[j-1]; j-- {
			d[j], d[j-1] = d[j-1], d[j]
		}
	}
}

// DefaultMetrics is the global metrics collector.
var DefaultMetrics = NewMetrics()
