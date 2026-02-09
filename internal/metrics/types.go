package metrics

import (
	"time"
)

// Collector defines the interface for metrics collection
type Collector interface {
	// IncCounter increments a counter metric
	IncCounter(name string, labels map[string]string)

	// AddCounter adds a value to a counter metric
	AddCounter(name string, value float64, labels map[string]string)

	// SetGauge sets a gauge metric
	SetGauge(name string, value float64, labels map[string]string)

	// IncGauge increments a gauge metric
	IncGauge(name string, labels map[string]string)

	// DecGauge decrements a gauge metric
	DecGauge(name string, labels map[string]string)

	// ObserveHistogram records a histogram observation
	ObserveHistogram(name string, value float64, labels map[string]string)

	// ObserveSummary records a summary observation
	ObserveSummary(name string, value float64, labels map[string]string)

	// RecordDuration records a duration in seconds
	RecordDuration(name string, duration time.Duration, labels map[string]string)
}

// MetricType represents the type of metric
type MetricType string

// MetricTypeCounter constants define the supported types.
const (
	MetricTypeCounter   MetricType = "counter"
	MetricTypeGauge     MetricType = "gauge"
	MetricTypeHistogram MetricType = "histogram"
	MetricTypeSummary   MetricType = "summary"
)

// MetricDefinition defines a metric's metadata
type MetricDefinition struct {
	Name       string
	Type       MetricType
	Help       string
	Labels     []string
	Buckets    []float64           // For histograms
	Objectives map[float64]float64 // For summaries (quantile -> error)
}

// MetricRegistry manages metric definitions
type MetricRegistry interface {
	// RegisterMetric registers a metric definition
	RegisterMetric(def MetricDefinition) error

	// GetMetric retrieves a metric definition
	GetMetric(name string) (*MetricDefinition, bool)

	// ListMetrics lists all registered metrics
	ListMetrics() []MetricDefinition
}

// Timer helps time operations
type Timer struct {
	collector Collector
	name      string
	labels    map[string]string
	start     time.Time
}

// NewTimer creates a new timer
func NewTimer(collector Collector, name string, labels map[string]string) *Timer {
	return &Timer{
		collector: collector,
		name:      name,
		labels:    labels,
		start:     time.Now(),
	}
}

// ObserveDuration records the duration since timer creation
func (t *Timer) ObserveDuration() {
	duration := time.Since(t.start)
	t.collector.RecordDuration(t.name, duration, t.labels)
}

// ObserveDurationWithLabels records the duration with additional labels
func (t *Timer) ObserveDurationWithLabels(additionalLabels map[string]string) {
	// Merge labels
	labels := make(map[string]string)
	for k, v := range t.labels {
		labels[k] = v
	}
	for k, v := range additionalLabels {
		labels[k] = v
	}

	duration := time.Since(t.start)
	t.collector.RecordDuration(t.name, duration, labels)
}

// DefaultBuckets provides default histogram buckets for duration metrics (in seconds)
var DefaultBuckets = []float64{
	0.001, // 1ms
	0.005, // 5ms
	0.01,  // 10ms
	0.025, // 25ms
	0.05,  // 50ms
	0.1,   // 100ms
	0.25,  // 250ms
	0.5,   // 500ms
	1.0,   // 1s
	2.5,   // 2.5s
	5.0,   // 5s
	10.0,  // 10s
}

// DefaultObjectives provides default summary objectives (quantile -> error)
var DefaultObjectives = map[float64]float64{
	0.5:  0.05,  // 50th percentile (median) with 5% error
	0.9:  0.01,  // 90th percentile with 1% error
	0.95: 0.005, // 95th percentile with 0.5% error
	0.99: 0.001, // 99th percentile with 0.1% error
}
