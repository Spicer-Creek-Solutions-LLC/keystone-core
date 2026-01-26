package metrics

import (
	"fmt"
	"hash/fnv"
	"sort"
	"sync"
	"time"
)

// CardinalityConfig configures the cardinality limiter
type CardinalityConfig struct {
	// MaxCardinality is the maximum number of unique label combinations per metric
	MaxCardinality int `json:"max_cardinality"`

	// MaxLabelValueLength is the maximum length of a label value
	MaxLabelValueLength int `json:"max_label_value_length"`

	// HighCardinalityLabels are labels that commonly have high cardinality
	// and should be monitored more closely
	HighCardinalityLabels []string `json:"high_cardinality_labels,omitempty"`

	// ReplacementValue is used when cardinality is exceeded
	ReplacementValue string `json:"replacement_value"`

	// ExcludedMetrics are metrics exempt from cardinality limiting
	ExcludedMetrics []string `json:"excluded_metrics,omitempty"`

	// CleanupInterval is how often to clean up stale entries
	CleanupInterval time.Duration `json:"cleanup_interval,omitempty"`

	// EntryTTL is how long to keep an unused label combination
	EntryTTL time.Duration `json:"entry_ttl,omitempty"`
}

// DefaultCardinalityConfig returns sensible defaults
func DefaultCardinalityConfig() *CardinalityConfig {
	return &CardinalityConfig{
		MaxCardinality:        10000,
		MaxLabelValueLength:   128,
		HighCardinalityLabels: []string{"agent_id", "user_id", "request_id", "trace_id"},
		ReplacementValue:      "__cardinality_exceeded__",
		CleanupInterval:       5 * time.Minute,
		EntryTTL:              1 * time.Hour,
	}
}

// labelEntry tracks a unique label combination
type labelEntry struct {
	hash     uint64
	lastSeen time.Time
}

// metricCardinality tracks cardinality for a single metric
type metricCardinality struct {
	entries     map[uint64]*labelEntry
	exceededAt  time.Time
	exceedCount int64
	mu          sync.RWMutex
}

// CardinalityLimiter limits metric cardinality
type CardinalityLimiter struct {
	config      *CardinalityConfig
	metrics     map[string]*metricCardinality
	mu          sync.RWMutex
	stopCh      chan struct{}
	stats       *CardinalityStats
	statsCollector Collector
}

// CardinalityStats holds statistics about cardinality limiting
type CardinalityStats struct {
	TotalMetrics           int                `json:"total_metrics"`
	TotalLabelCombinations int64              `json:"total_label_combinations"`
	ExceededMetrics        int                `json:"exceeded_metrics"`
	DroppedLabels          int64              `json:"dropped_labels"`
	MetricCardinalities    map[string]int     `json:"metric_cardinalities"`
	HighCardinalityMetrics []string           `json:"high_cardinality_metrics"`
	mu                     sync.RWMutex
}

// NewCardinalityLimiter creates a new cardinality limiter
func NewCardinalityLimiter(config *CardinalityConfig) *CardinalityLimiter {
	if config == nil {
		config = DefaultCardinalityConfig()
	}

	limiter := &CardinalityLimiter{
		config:  config,
		metrics: make(map[string]*metricCardinality),
		stopCh:  make(chan struct{}),
		stats: &CardinalityStats{
			MetricCardinalities: make(map[string]int),
		},
	}

	// Start cleanup goroutine
	if config.CleanupInterval > 0 {
		go limiter.cleanupLoop()
	}

	return limiter
}

// SetStatsCollector sets the collector for cardinality stats
func (l *CardinalityLimiter) SetStatsCollector(collector Collector) {
	l.statsCollector = collector
}

// ProcessLabels processes labels and returns potentially modified labels
func (l *CardinalityLimiter) ProcessLabels(metricName string, labels map[string]string) map[string]string {
	// Check if metric is excluded
	if l.isExcluded(metricName) {
		return labels
	}

	// Truncate long label values
	processedLabels := l.truncateLabels(labels)

	// Get or create metric cardinality tracker
	mc := l.getOrCreateMetricCardinality(metricName)

	// Calculate hash of label combination
	hash := l.hashLabels(processedLabels)

	mc.mu.Lock()
	defer mc.mu.Unlock()

	// Check if this combination exists
	if entry, exists := mc.entries[hash]; exists {
		entry.lastSeen = time.Now()
		return processedLabels
	}

	// Check cardinality limit
	if len(mc.entries) >= l.config.MaxCardinality {
		// Cardinality exceeded - replace high-cardinality labels
		mc.exceedCount++
		if mc.exceededAt.IsZero() {
			mc.exceededAt = time.Now()
		}

		// Update stats
		l.stats.mu.Lock()
		l.stats.DroppedLabels++
		l.stats.mu.Unlock()

		// Replace high-cardinality label values
		return l.replaceHighCardinalityLabels(processedLabels)
	}

	// Add new entry
	mc.entries[hash] = &labelEntry{
		hash:     hash,
		lastSeen: time.Now(),
	}

	return processedLabels
}

// isExcluded checks if a metric is excluded from cardinality limiting
func (l *CardinalityLimiter) isExcluded(metricName string) bool {
	for _, excluded := range l.config.ExcludedMetrics {
		if excluded == metricName {
			return true
		}
	}
	return false
}

// truncateLabels truncates label values that are too long
func (l *CardinalityLimiter) truncateLabels(labels map[string]string) map[string]string {
	if l.config.MaxLabelValueLength <= 0 {
		return labels
	}

	result := make(map[string]string, len(labels))
	for k, v := range labels {
		if len(v) > l.config.MaxLabelValueLength {
			result[k] = v[:l.config.MaxLabelValueLength]
		} else {
			result[k] = v
		}
	}
	return result
}

// hashLabels creates a hash of the label key-value pairs
func (l *CardinalityLimiter) hashLabels(labels map[string]string) uint64 {
	// Sort keys for consistent hashing
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	h := fnv.New64a()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte("="))
		h.Write([]byte(labels[k]))
		h.Write([]byte(","))
	}
	return h.Sum64()
}

// getOrCreateMetricCardinality gets or creates a metric cardinality tracker
func (l *CardinalityLimiter) getOrCreateMetricCardinality(metricName string) *metricCardinality {
	l.mu.RLock()
	mc, exists := l.metrics[metricName]
	l.mu.RUnlock()

	if exists {
		return mc
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	// Double-check after acquiring write lock
	if mc, exists = l.metrics[metricName]; exists {
		return mc
	}

	mc = &metricCardinality{
		entries: make(map[uint64]*labelEntry),
	}
	l.metrics[metricName] = mc
	return mc
}

// replaceHighCardinalityLabels replaces values of high-cardinality labels
func (l *CardinalityLimiter) replaceHighCardinalityLabels(labels map[string]string) map[string]string {
	result := make(map[string]string, len(labels))
	for k, v := range labels {
		if l.isHighCardinalityLabel(k) {
			result[k] = l.config.ReplacementValue
		} else {
			result[k] = v
		}
	}
	return result
}

// isHighCardinalityLabel checks if a label is known to be high-cardinality
func (l *CardinalityLimiter) isHighCardinalityLabel(label string) bool {
	for _, hcLabel := range l.config.HighCardinalityLabels {
		if hcLabel == label {
			return true
		}
	}
	return false
}

// cleanupLoop periodically cleans up stale entries
func (l *CardinalityLimiter) cleanupLoop() {
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

// cleanup removes stale label combination entries
func (l *CardinalityLimiter) cleanup() {
	now := time.Now()
	ttl := l.config.EntryTTL

	l.mu.RLock()
	metricNames := make([]string, 0, len(l.metrics))
	for name := range l.metrics {
		metricNames = append(metricNames, name)
	}
	l.mu.RUnlock()

	for _, name := range metricNames {
		l.mu.RLock()
		mc := l.metrics[name]
		l.mu.RUnlock()

		if mc == nil {
			continue
		}

		mc.mu.Lock()
		for hash, entry := range mc.entries {
			if now.Sub(entry.lastSeen) > ttl {
				delete(mc.entries, hash)
			}
		}
		mc.mu.Unlock()
	}

	l.updateStats()
}

// updateStats updates the cardinality statistics
func (l *CardinalityLimiter) updateStats() {
	l.stats.mu.Lock()
	defer l.stats.mu.Unlock()

	l.mu.RLock()
	defer l.mu.RUnlock()

	l.stats.TotalMetrics = len(l.metrics)
	l.stats.TotalLabelCombinations = 0
	l.stats.ExceededMetrics = 0
	l.stats.MetricCardinalities = make(map[string]int)
	l.stats.HighCardinalityMetrics = nil

	threshold := int(float64(l.config.MaxCardinality) * 0.8) // 80% of max

	for name, mc := range l.metrics {
		mc.mu.RLock()
		count := len(mc.entries)
		exceeded := mc.exceedCount > 0
		mc.mu.RUnlock()

		l.stats.TotalLabelCombinations += int64(count)
		l.stats.MetricCardinalities[name] = count

		if exceeded {
			l.stats.ExceededMetrics++
		}
		if count >= threshold {
			l.stats.HighCardinalityMetrics = append(l.stats.HighCardinalityMetrics, name)
		}
	}
}

// GetStats returns current cardinality statistics
func (l *CardinalityLimiter) GetStats() *CardinalityStats {
	l.updateStats()
	l.stats.mu.RLock()
	defer l.stats.mu.RUnlock()

	// Return a copy
	stats := &CardinalityStats{
		TotalMetrics:           l.stats.TotalMetrics,
		TotalLabelCombinations: l.stats.TotalLabelCombinations,
		ExceededMetrics:        l.stats.ExceededMetrics,
		DroppedLabels:          l.stats.DroppedLabels,
		MetricCardinalities:    make(map[string]int),
		HighCardinalityMetrics: make([]string, len(l.stats.HighCardinalityMetrics)),
	}
	for k, v := range l.stats.MetricCardinalities {
		stats.MetricCardinalities[k] = v
	}
	copy(stats.HighCardinalityMetrics, l.stats.HighCardinalityMetrics)
	return stats
}

// GetMetricCardinality returns the current cardinality for a metric
func (l *CardinalityLimiter) GetMetricCardinality(metricName string) int {
	l.mu.RLock()
	mc, exists := l.metrics[metricName]
	l.mu.RUnlock()

	if !exists {
		return 0
	}

	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return len(mc.entries)
}

// IsExceeded checks if a metric has exceeded its cardinality limit
func (l *CardinalityLimiter) IsExceeded(metricName string) bool {
	l.mu.RLock()
	mc, exists := l.metrics[metricName]
	l.mu.RUnlock()

	if !exists {
		return false
	}

	mc.mu.RLock()
	defer mc.mu.RUnlock()
	return mc.exceedCount > 0
}

// Reset resets cardinality tracking for a metric
func (l *CardinalityLimiter) Reset(metricName string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.metrics, metricName)
}

// ResetAll resets all cardinality tracking
func (l *CardinalityLimiter) ResetAll() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.metrics = make(map[string]*metricCardinality)
	l.stats.mu.Lock()
	l.stats.DroppedLabels = 0
	l.stats.mu.Unlock()
}

// Stop stops the cleanup goroutine
func (l *CardinalityLimiter) Stop() {
	close(l.stopCh)
}

// Config returns the current configuration
func (l *CardinalityLimiter) Config() *CardinalityConfig {
	return l.config
}

// CardinalityLimitingCollector wraps a Collector with cardinality limiting
type CardinalityLimitingCollector struct {
	wrapped Collector
	limiter *CardinalityLimiter
}

// NewCardinalityLimitingCollector creates a new cardinality-limiting collector
func NewCardinalityLimitingCollector(wrapped Collector, config *CardinalityConfig) *CardinalityLimitingCollector {
	return &CardinalityLimitingCollector{
		wrapped: wrapped,
		limiter: NewCardinalityLimiter(config),
	}
}

// IncCounter increments a counter with cardinality limiting
func (c *CardinalityLimitingCollector) IncCounter(name string, labels map[string]string) {
	processedLabels := c.limiter.ProcessLabels(name, labels)
	c.wrapped.IncCounter(name, processedLabels)
}

// AddCounter adds to a counter with cardinality limiting
func (c *CardinalityLimitingCollector) AddCounter(name string, value float64, labels map[string]string) {
	processedLabels := c.limiter.ProcessLabels(name, labels)
	c.wrapped.AddCounter(name, value, processedLabels)
}

// SetGauge sets a gauge with cardinality limiting
func (c *CardinalityLimitingCollector) SetGauge(name string, value float64, labels map[string]string) {
	processedLabels := c.limiter.ProcessLabels(name, labels)
	c.wrapped.SetGauge(name, value, processedLabels)
}

// IncGauge increments a gauge with cardinality limiting
func (c *CardinalityLimitingCollector) IncGauge(name string, labels map[string]string) {
	processedLabels := c.limiter.ProcessLabels(name, labels)
	c.wrapped.IncGauge(name, processedLabels)
}

// DecGauge decrements a gauge with cardinality limiting
func (c *CardinalityLimitingCollector) DecGauge(name string, labels map[string]string) {
	processedLabels := c.limiter.ProcessLabels(name, labels)
	c.wrapped.DecGauge(name, processedLabels)
}

// ObserveHistogram records a histogram observation with cardinality limiting
func (c *CardinalityLimitingCollector) ObserveHistogram(name string, value float64, labels map[string]string) {
	processedLabels := c.limiter.ProcessLabels(name, labels)
	c.wrapped.ObserveHistogram(name, value, processedLabels)
}

// ObserveSummary records a summary observation with cardinality limiting
func (c *CardinalityLimitingCollector) ObserveSummary(name string, value float64, labels map[string]string) {
	processedLabels := c.limiter.ProcessLabels(name, labels)
	c.wrapped.ObserveSummary(name, value, processedLabels)
}

// RecordDuration records a duration with cardinality limiting
func (c *CardinalityLimitingCollector) RecordDuration(name string, duration time.Duration, labels map[string]string) {
	processedLabels := c.limiter.ProcessLabels(name, labels)
	c.wrapped.RecordDuration(name, duration, processedLabels)
}

// Limiter returns the underlying cardinality limiter
func (c *CardinalityLimitingCollector) Limiter() *CardinalityLimiter {
	return c.limiter
}

// Wrapped returns the wrapped collector
func (c *CardinalityLimitingCollector) Wrapped() Collector {
	return c.wrapped
}

// Stop stops the cardinality limiter
func (c *CardinalityLimitingCollector) Stop() {
	c.limiter.Stop()
}

// CardinalityReport generates a human-readable cardinality report
type CardinalityReport struct {
	GeneratedAt            time.Time            `json:"generated_at"`
	Config                 *CardinalityConfig   `json:"config"`
	Stats                  *CardinalityStats    `json:"stats"`
	TopMetrics             []MetricCardinality  `json:"top_metrics"`
	ExceededMetrics        []MetricExceedInfo   `json:"exceeded_metrics"`
	Recommendations        []string             `json:"recommendations"`
}

// MetricCardinality holds cardinality info for a single metric
type MetricCardinality struct {
	Name        string  `json:"name"`
	Cardinality int     `json:"cardinality"`
	Percentage  float64 `json:"percentage"`
}

// MetricExceedInfo holds info about a metric that exceeded limits
type MetricExceedInfo struct {
	Name        string    `json:"name"`
	ExceededAt  time.Time `json:"exceeded_at"`
	ExceedCount int64     `json:"exceed_count"`
}

// GenerateReport generates a cardinality report
func (l *CardinalityLimiter) GenerateReport() *CardinalityReport {
	stats := l.GetStats()

	report := &CardinalityReport{
		GeneratedAt:     time.Now(),
		Config:          l.config,
		Stats:           stats,
		TopMetrics:      make([]MetricCardinality, 0),
		ExceededMetrics: make([]MetricExceedInfo, 0),
		Recommendations: make([]string, 0),
	}

	// Collect top metrics by cardinality
	type mcPair struct {
		name        string
		cardinality int
	}
	pairs := make([]mcPair, 0, len(stats.MetricCardinalities))
	for name, card := range stats.MetricCardinalities {
		pairs = append(pairs, mcPair{name, card})
	}
	sort.Slice(pairs, func(i, j int) bool {
		return pairs[i].cardinality > pairs[j].cardinality
	})

	// Top 10 metrics
	for i := 0; i < len(pairs) && i < 10; i++ {
		report.TopMetrics = append(report.TopMetrics, MetricCardinality{
			Name:        pairs[i].name,
			Cardinality: pairs[i].cardinality,
			Percentage:  float64(pairs[i].cardinality) / float64(l.config.MaxCardinality) * 100,
		})
	}

	// Exceeded metrics
	l.mu.RLock()
	for name, mc := range l.metrics {
		mc.mu.RLock()
		if mc.exceedCount > 0 {
			report.ExceededMetrics = append(report.ExceededMetrics, MetricExceedInfo{
				Name:        name,
				ExceededAt:  mc.exceededAt,
				ExceedCount: mc.exceedCount,
			})
		}
		mc.mu.RUnlock()
	}
	l.mu.RUnlock()

	// Generate recommendations
	if stats.ExceededMetrics > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("%d metrics have exceeded cardinality limits. Consider reducing label cardinality or increasing limits.", stats.ExceededMetrics))
	}
	if len(stats.HighCardinalityMetrics) > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("%d metrics are approaching cardinality limits (>80%%). Review label usage.", len(stats.HighCardinalityMetrics)))
	}
	if stats.DroppedLabels > 0 {
		report.Recommendations = append(report.Recommendations,
			fmt.Sprintf("%d label combinations have been replaced with placeholders. This may affect metric accuracy.", stats.DroppedLabels))
	}
	if len(report.Recommendations) == 0 {
		report.Recommendations = append(report.Recommendations, "Cardinality is within healthy limits.")
	}

	return report
}
