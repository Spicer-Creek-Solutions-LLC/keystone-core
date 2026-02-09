// Package cardinality provides cardinality limiting for metrics.
package cardinality

import (
	"context"
	"errors"
	"hash/fnv"
	"math"
	"sort"
	"sync"
	"time"
)

// Errors returned by the cardinality limiter.
var (
	ErrCardinalityExceeded = errors.New("cardinality limit exceeded")
	ErrMetricDropped       = errors.New("metric dropped due to cardinality limit")
)

// Strategy defines how to handle cardinality limit violations.
type Strategy string

const (
	// StrategyDrop drops new series when limit is reached.
	StrategyDrop Strategy = "drop"
	// StrategyAggregate aggregates into an "other" bucket.
	StrategyAggregate Strategy = "aggregate"
	// StrategyEvictOldest removes oldest series to make room.
	StrategyEvictOldest Strategy = "evict_oldest"
	// StrategyEvictLRU removes least recently used series.
	StrategyEvictLRU Strategy = "evict_lru"
)

// Config holds cardinality limiter configuration.
type Config struct {
	// MaxSeries is the maximum number of unique series per metric.
	MaxSeries int
	// MaxLabels is the maximum number of labels per series.
	MaxLabels int
	// MaxLabelValueLength is the maximum length of label values.
	MaxLabelValueLength int
	// Strategy defines how to handle limit violations.
	Strategy Strategy
	// TTL is how long to keep inactive series.
	TTL time.Duration
	// CleanupInterval is how often to run cleanup.
	CleanupInterval time.Duration
	// WarnThreshold is the percentage at which to warn (0-1).
	WarnThreshold float64
}

// DefaultConfig returns a default cardinality limiter configuration.
func DefaultConfig() *Config {
	return &Config{
		MaxSeries:           10000,
		MaxLabels:           20,
		MaxLabelValueLength: 256,
		Strategy:            StrategyDrop,
		TTL:                 time.Hour,
		CleanupInterval:     time.Minute * 5,
		WarnThreshold:       0.8,
	}
}

// Labels represents a set of metric labels.
type Labels map[string]string

// Hash returns a hash of the labels for efficient comparison.
func (l Labels) Hash() uint64 {
	h := fnv.New64a()
	// Sort keys for consistent hashing
	keys := make([]string, 0, len(l))
	for k := range l {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{0})
		h.Write([]byte(l[k]))
		h.Write([]byte{0})
	}
	return h.Sum64()
}

// Series represents a unique time series.
type Series struct {
	MetricName string
	Labels     Labels
	Hash       uint64
	CreatedAt  time.Time
	LastUsedAt time.Time
	Count      int64
}

// Metric represents a metric with its series.
type Metric struct {
	Name       string
	MaxSeries  int
	Series     map[uint64]*Series
	mu         sync.RWMutex
	createdAt  time.Time
	totalCount int64
}

// Event represents a cardinality-related event.
type Event struct {
	Type       string
	MetricName string
	Labels     Labels
	Timestamp  time.Time
	Message    string
	Current    int
	Limit      int
}

// Listener is a function that receives cardinality events.
type Listener func(*Event)

// Limiter manages cardinality limits for metrics.
type Limiter struct {
	config    *Config
	metrics   map[string]*Metric
	mu        sync.RWMutex
	listeners []Listener
	stats     *Stats
	stopCh    chan struct{}
	wg        sync.WaitGroup
}

// Stats holds cardinality statistics.
type Stats struct {
	TotalMetrics  int64
	TotalSeries   int64
	DroppedSeries int64
	Evictions     int64
	Aggregations  int64
	mu            sync.RWMutex
}

// NewLimiter creates a new cardinality limiter.
func NewLimiter(config *Config) *Limiter {
	if config == nil {
		config = DefaultConfig()
	}

	l := &Limiter{
		config:  config,
		metrics: make(map[string]*Metric),
		stats:   &Stats{},
		stopCh:  make(chan struct{}),
	}

	// Start cleanup goroutine
	if config.CleanupInterval > 0 {
		l.wg.Add(1)
		go l.cleanupLoop()
	}

	return l
}

// Record records a metric series and returns whether it was accepted.
func (l *Limiter) Record(ctx context.Context, metricName string, labels Labels) error {
	// Validate labels
	if err := l.validateLabels(labels); err != nil {
		return err
	}

	l.mu.Lock()
	metric, exists := l.metrics[metricName]
	if !exists {
		metric = &Metric{
			Name:      metricName,
			MaxSeries: l.config.MaxSeries,
			Series:    make(map[uint64]*Series),
			createdAt: time.Now(),
		}
		l.metrics[metricName] = metric
		l.stats.mu.Lock()
		l.stats.TotalMetrics++
		l.stats.mu.Unlock()
	}
	l.mu.Unlock()

	return l.recordSeries(metric, labels)
}

func (l *Limiter) recordSeries(metric *Metric, labels Labels) error {
	hash := labels.Hash()

	metric.mu.Lock()
	defer metric.mu.Unlock()

	// Check if series already exists
	if series, exists := metric.Series[hash]; exists {
		series.LastUsedAt = time.Now()
		series.Count++
		metric.totalCount++
		return nil
	}

	// Check if we're at the limit
	if len(metric.Series) >= metric.MaxSeries {
		return l.handleLimitExceeded(metric, labels, hash)
	}

	// Check warning threshold
	if l.config.WarnThreshold > 0 {
		ratio := float64(len(metric.Series)) / float64(metric.MaxSeries)
		if ratio >= l.config.WarnThreshold {
			l.emitEvent(&Event{
				Type:       "warning",
				MetricName: metric.Name,
				Timestamp:  time.Now(),
				Message:    "approaching cardinality limit",
				Current:    len(metric.Series),
				Limit:      metric.MaxSeries,
			})
		}
	}

	// Add new series
	now := time.Now()
	metric.Series[hash] = &Series{
		MetricName: metric.Name,
		Labels:     labels,
		Hash:       hash,
		CreatedAt:  now,
		LastUsedAt: now,
		Count:      1,
	}
	metric.totalCount++

	l.stats.mu.Lock()
	l.stats.TotalSeries++
	l.stats.mu.Unlock()

	return nil
}

func (l *Limiter) handleLimitExceeded(metric *Metric, labels Labels, hash uint64) error {
	switch l.config.Strategy {
	case StrategyDrop:
		l.stats.mu.Lock()
		l.stats.DroppedSeries++
		l.stats.mu.Unlock()
		l.emitEvent(&Event{
			Type:       "dropped",
			MetricName: metric.Name,
			Labels:     labels,
			Timestamp:  time.Now(),
			Message:    "series dropped due to cardinality limit",
			Current:    len(metric.Series),
			Limit:      metric.MaxSeries,
		})
		return ErrMetricDropped

	case StrategyAggregate:
		// Aggregate into an "other" bucket
		otherLabels := Labels{"__aggregated__": "true"}
		otherHash := otherLabels.Hash()
		if series, exists := metric.Series[otherHash]; exists {
			series.LastUsedAt = time.Now()
			series.Count++
		} else {
			now := time.Now()
			metric.Series[otherHash] = &Series{
				MetricName: metric.Name,
				Labels:     otherLabels,
				Hash:       otherHash,
				CreatedAt:  now,
				LastUsedAt: now,
				Count:      1,
			}
		}
		metric.totalCount++
		l.stats.mu.Lock()
		l.stats.Aggregations++
		l.stats.mu.Unlock()
		return nil

	case StrategyEvictOldest:
		return l.evictOldest(metric, labels, hash)

	case StrategyEvictLRU:
		return l.evictLRU(metric, labels, hash)

	default:
		return ErrCardinalityExceeded
	}
}

func (l *Limiter) evictOldest(metric *Metric, newLabels Labels, newHash uint64) error {
	var oldest *Series
	for _, s := range metric.Series {
		if oldest == nil || s.CreatedAt.Before(oldest.CreatedAt) {
			oldest = s
		}
	}

	if oldest != nil {
		delete(metric.Series, oldest.Hash)
		l.stats.mu.Lock()
		l.stats.Evictions++
		l.stats.TotalSeries--
		l.stats.mu.Unlock()

		l.emitEvent(&Event{
			Type:       "evicted",
			MetricName: metric.Name,
			Labels:     oldest.Labels,
			Timestamp:  time.Now(),
			Message:    "series evicted (oldest)",
		})
	}

	// Add new series
	now := time.Now()
	metric.Series[newHash] = &Series{
		MetricName: metric.Name,
		Labels:     newLabels,
		Hash:       newHash,
		CreatedAt:  now,
		LastUsedAt: now,
		Count:      1,
	}
	metric.totalCount++

	l.stats.mu.Lock()
	l.stats.TotalSeries++
	l.stats.mu.Unlock()

	return nil
}

func (l *Limiter) evictLRU(metric *Metric, newLabels Labels, newHash uint64) error {
	var lru *Series
	for _, s := range metric.Series {
		if lru == nil || s.LastUsedAt.Before(lru.LastUsedAt) {
			lru = s
		}
	}

	if lru != nil {
		delete(metric.Series, lru.Hash)
		l.stats.mu.Lock()
		l.stats.Evictions++
		l.stats.TotalSeries--
		l.stats.mu.Unlock()

		l.emitEvent(&Event{
			Type:       "evicted",
			MetricName: metric.Name,
			Labels:     lru.Labels,
			Timestamp:  time.Now(),
			Message:    "series evicted (LRU)",
		})
	}

	// Add new series
	now := time.Now()
	metric.Series[newHash] = &Series{
		MetricName: metric.Name,
		Labels:     newLabels,
		Hash:       newHash,
		CreatedAt:  now,
		LastUsedAt: now,
		Count:      1,
	}
	metric.totalCount++

	l.stats.mu.Lock()
	l.stats.TotalSeries++
	l.stats.mu.Unlock()

	return nil
}

func (l *Limiter) validateLabels(labels Labels) error {
	if len(labels) > l.config.MaxLabels {
		return errors.New("too many labels")
	}

	for _, v := range labels {
		if len(v) > l.config.MaxLabelValueLength {
			return errors.New("label value too long")
		}
	}

	return nil
}

// GetMetric returns information about a metric.
func (l *Limiter) GetMetric(name string) *MetricInfo {
	l.mu.RLock()
	metric, exists := l.metrics[name]
	l.mu.RUnlock()

	if !exists {
		return nil
	}

	metric.mu.RLock()
	defer metric.mu.RUnlock()

	return &MetricInfo{
		Name:        metric.Name,
		SeriesCount: len(metric.Series),
		MaxSeries:   metric.MaxSeries,
		TotalCount:  metric.totalCount,
		CreatedAt:   metric.createdAt,
	}
}

// MetricInfo provides information about a metric.
type MetricInfo struct {
	Name        string
	SeriesCount int
	MaxSeries   int
	TotalCount  int64
	CreatedAt   time.Time
}

// ListMetrics returns all tracked metrics.
func (l *Limiter) ListMetrics() []*MetricInfo {
	l.mu.RLock()
	defer l.mu.RUnlock()

	result := make([]*MetricInfo, 0, len(l.metrics))
	for _, m := range l.metrics {
		m.mu.RLock()
		result = append(result, &MetricInfo{
			Name:        m.Name,
			SeriesCount: len(m.Series),
			MaxSeries:   m.MaxSeries,
			TotalCount:  m.totalCount,
			CreatedAt:   m.createdAt,
		})
		m.mu.RUnlock()
	}

	return result
}

// SetMetricLimit sets a specific limit for a metric.
func (l *Limiter) SetMetricLimit(metricName string, limit int) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if metric, exists := l.metrics[metricName]; exists {
		metric.mu.Lock()
		metric.MaxSeries = limit
		metric.mu.Unlock()
	}
}

// Stats returns current statistics.
func (l *Limiter) Stats() *Stats {
	l.stats.mu.RLock()
	defer l.stats.mu.RUnlock()
	return &Stats{
		TotalMetrics:  l.stats.TotalMetrics,
		TotalSeries:   l.stats.TotalSeries,
		DroppedSeries: l.stats.DroppedSeries,
		Evictions:     l.stats.Evictions,
		Aggregations:  l.stats.Aggregations,
	}
}

// AddListener adds an event listener.
func (l *Limiter) AddListener(listener Listener) {
	l.mu.Lock()
	l.listeners = append(l.listeners, listener)
	l.mu.Unlock()
}

func (l *Limiter) emitEvent(event *Event) {
	l.mu.RLock()
	listeners := l.listeners
	l.mu.RUnlock()

	for _, listener := range listeners {
		listener(event)
	}
}

func (l *Limiter) cleanupLoop() {
	defer l.wg.Done()

	ticker := time.NewTicker(l.config.CleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			l.cleanup()
		case <-l.stopCh:
			return
		}
	}
}

func (l *Limiter) cleanup() {
	now := time.Now()
	cutoff := now.Add(-l.config.TTL)

	l.mu.RLock()
	metrics := make([]*Metric, 0, len(l.metrics))
	for _, m := range l.metrics {
		metrics = append(metrics, m)
	}
	l.mu.RUnlock()

	for _, metric := range metrics {
		l.cleanupMetric(metric, cutoff)
	}
}

func (l *Limiter) cleanupMetric(metric *Metric, cutoff time.Time) {
	metric.mu.Lock()
	defer metric.mu.Unlock()

	for hash, series := range metric.Series {
		if !series.LastUsedAt.Before(cutoff) {
			continue
		}
		delete(metric.Series, hash)
		l.stats.mu.Lock()
		l.stats.TotalSeries--
		l.stats.mu.Unlock()

		l.emitEvent(&Event{
			Type:       "expired",
			MetricName: metric.Name,
			Labels:     series.Labels,
			Timestamp:  time.Now(),
			Message:    "series expired due to TTL",
		})
	}
}

// Stop stops the limiter and its cleanup goroutine.
func (l *Limiter) Stop() {
	close(l.stopCh)
	l.wg.Wait()
}

// Reset clears all metrics and series.
func (l *Limiter) Reset() {
	l.mu.Lock()
	defer l.mu.Unlock()

	l.metrics = make(map[string]*Metric)
	l.stats.mu.Lock()
	l.stats.TotalMetrics = 0
	l.stats.TotalSeries = 0
	l.stats.DroppedSeries = 0
	l.stats.Evictions = 0
	l.stats.Aggregations = 0
	l.stats.mu.Unlock()
}

// LabelCardinalityTracker tracks cardinality at the label level.
type LabelCardinalityTracker struct {
	labelValues map[string]map[string]struct{} // label name -> set of values
	maxValues   int
	mu          sync.RWMutex
}

// NewLabelCardinalityTracker creates a new label cardinality tracker.
func NewLabelCardinalityTracker(maxValuesPerLabel int) *LabelCardinalityTracker {
	return &LabelCardinalityTracker{
		labelValues: make(map[string]map[string]struct{}),
		maxValues:   maxValuesPerLabel,
	}
}

// Track tracks a set of labels and returns if any label exceeded cardinality.
func (t *LabelCardinalityTracker) Track(labels Labels) []string {
	t.mu.Lock()
	defer t.mu.Unlock()

	var exceeded []string

	for name, value := range labels {
		values, exists := t.labelValues[name]
		if !exists {
			values = make(map[string]struct{})
			t.labelValues[name] = values
		}

		if _, hasValue := values[value]; !hasValue {
			if len(values) >= t.maxValues {
				exceeded = append(exceeded, name)
				continue
			}
			values[value] = struct{}{}
		}
	}

	return exceeded
}

// GetCardinality returns the cardinality of a label.
func (t *LabelCardinalityTracker) GetCardinality(labelName string) int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if values, exists := t.labelValues[labelName]; exists {
		return len(values)
	}
	return 0
}

// GetAllCardinalities returns cardinality for all labels.
func (t *LabelCardinalityTracker) GetAllCardinalities() map[string]int {
	t.mu.RLock()
	defer t.mu.RUnlock()

	result := make(map[string]int, len(t.labelValues))
	for name, values := range t.labelValues {
		result[name] = len(values)
	}
	return result
}

// Reset clears the tracker.
func (t *LabelCardinalityTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.labelValues = make(map[string]map[string]struct{})
}

// Estimator uses HyperLogLog for memory-efficient cardinality estimation.
type Estimator struct {
	registers []uint8
	precision uint8
	m         uint32 // number of registers
	mu        sync.RWMutex
}

// NewCardinalityEstimator creates a new cardinality estimator.
// Precision should be between 4 and 16. Higher = more accurate but more memory.
func NewCardinalityEstimator(precision uint8) *Estimator {
	if precision < 4 {
		precision = 4
	}
	if precision > 16 {
		precision = 16
	}

	m := uint32(1) << precision
	return &Estimator{
		registers: make([]uint8, m),
		precision: precision,
		m:         m,
	}
}

// Add adds an element to the estimator.
func (e *Estimator) Add(data []byte) {
	// Use FNV-1a hash
	h := fnv.New64a()
	h.Write(data)
	hash := h.Sum64()

	e.mu.Lock()
	defer e.mu.Unlock()

	// Use lower bits for register index (better distribution)
	idx := hash & uint64(e.m-1)

	// Use upper bits for counting leading zeros
	w := hash >> e.precision
	if w == 0 {
		w = 1
	}

	// Count trailing zeros + 1 (equivalent to leading zeros in reversed order)
	rho := uint8(1)
	for w&1 == 0 && rho < 64-e.precision {
		rho++
		w >>= 1
	}

	if rho > e.registers[idx] {
		e.registers[idx] = rho
	}
}

// Estimate returns the estimated cardinality.
func (e *Estimator) Estimate() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()

	m := float64(e.m)

	// Calculate the indicator (harmonic mean of 2^-register)
	sum := 0.0
	zeros := 0
	for _, val := range e.registers {
		if val == 0 {
			zeros++
			sum += 1.0 // 2^0 = 1
		} else {
			sum += math.Pow(2, -float64(val))
		}
	}

	// Calculate alpha_m (bias correction factor)
	var alpha float64
	switch e.m {
	case 16:
		alpha = 0.673
	case 32:
		alpha = 0.697
	case 64:
		alpha = 0.709
	default:
		alpha = 0.7213 / (1 + 1.079/m)
	}

	estimate := alpha * m * m / sum

	// Small range correction using linear counting
	if estimate <= 2.5*m && zeros > 0 {
		estimate = m * math.Log(m/float64(zeros))
	}

	return uint64(estimate + 0.5)
}

// Merge merges another estimator into this one.
func (e *Estimator) Merge(other *Estimator) {
	if e.precision != other.precision {
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	other.mu.RLock()
	defer other.mu.RUnlock()

	for i := range e.registers {
		if other.registers[i] > e.registers[i] {
			e.registers[i] = other.registers[i]
		}
	}
}

// Reset clears the estimator.
func (e *Estimator) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	for i := range e.registers {
		e.registers[i] = 0
	}
}
