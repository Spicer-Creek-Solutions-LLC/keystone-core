package metrics

import (
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

// PrometheusCollector implements the Collector interface using Prometheus
type PrometheusCollector struct {
	registry *prometheus.Registry
	counters map[string]*prometheus.CounterVec
	gauges   map[string]*prometheus.GaugeVec
	histograms map[string]*prometheus.HistogramVec
	summaries map[string]*prometheus.SummaryVec
	mu       sync.RWMutex
}

// NewPrometheusCollector creates a new Prometheus collector
func NewPrometheusCollector() *PrometheusCollector {
	return &PrometheusCollector{
		registry:   prometheus.NewRegistry(),
		counters:   make(map[string]*prometheus.CounterVec),
		gauges:     make(map[string]*prometheus.GaugeVec),
		histograms: make(map[string]*prometheus.HistogramVec),
		summaries:  make(map[string]*prometheus.SummaryVec),
	}
}

// RegisterMetric registers a metric with Prometheus
func (c *PrometheusCollector) RegisterMetric(def MetricDefinition) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	switch def.Type {
	case MetricTypeCounter:
		counter := prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: def.Name,
				Help: def.Help,
			},
			def.Labels,
		)
		if err := c.registry.Register(counter); err != nil {
			return fmt.Errorf("failed to register counter: %w", err)
		}
		c.counters[def.Name] = counter

	case MetricTypeGauge:
		gauge := prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Name: def.Name,
				Help: def.Help,
			},
			def.Labels,
		)
		if err := c.registry.Register(gauge); err != nil {
			return fmt.Errorf("failed to register gauge: %w", err)
		}
		c.gauges[def.Name] = gauge

	case MetricTypeHistogram:
		buckets := def.Buckets
		if len(buckets) == 0 {
			buckets = DefaultBuckets
		}
		histogram := prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Name:    def.Name,
				Help:    def.Help,
				Buckets: buckets,
			},
			def.Labels,
		)
		if err := c.registry.Register(histogram); err != nil {
			return fmt.Errorf("failed to register histogram: %w", err)
		}
		c.histograms[def.Name] = histogram

	case MetricTypeSummary:
		objectives := def.Objectives
		if len(objectives) == 0 {
			objectives = DefaultObjectives
		}
		summary := prometheus.NewSummaryVec(
			prometheus.SummaryOpts{
				Name:       def.Name,
				Help:       def.Help,
				Objectives: objectives,
			},
			def.Labels,
		)
		if err := c.registry.Register(summary); err != nil {
			return fmt.Errorf("failed to register summary: %w", err)
		}
		c.summaries[def.Name] = summary

	default:
		return fmt.Errorf("unknown metric type: %s", def.Type)
	}

	return nil
}

// IncCounter increments a counter metric
func (c *PrometheusCollector) IncCounter(name string, labels map[string]string) {
	c.mu.RLock()
	counter, ok := c.counters[name]
	c.mu.RUnlock()

	if ok {
		counter.With(labels).Inc()
	}
}

// AddCounter adds a value to a counter metric
func (c *PrometheusCollector) AddCounter(name string, value float64, labels map[string]string) {
	c.mu.RLock()
	counter, ok := c.counters[name]
	c.mu.RUnlock()

	if ok {
		counter.With(labels).Add(value)
	}
}

// SetGauge sets a gauge metric
func (c *PrometheusCollector) SetGauge(name string, value float64, labels map[string]string) {
	c.mu.RLock()
	gauge, ok := c.gauges[name]
	c.mu.RUnlock()

	if ok {
		gauge.With(labels).Set(value)
	}
}

// IncGauge increments a gauge metric
func (c *PrometheusCollector) IncGauge(name string, labels map[string]string) {
	c.mu.RLock()
	gauge, ok := c.gauges[name]
	c.mu.RUnlock()

	if ok {
		gauge.With(labels).Inc()
	}
}

// DecGauge decrements a gauge metric
func (c *PrometheusCollector) DecGauge(name string, labels map[string]string) {
	c.mu.RLock()
	gauge, ok := c.gauges[name]
	c.mu.RUnlock()

	if ok {
		gauge.With(labels).Dec()
	}
}

// ObserveHistogram records a histogram observation
func (c *PrometheusCollector) ObserveHistogram(name string, value float64, labels map[string]string) {
	c.mu.RLock()
	histogram, ok := c.histograms[name]
	c.mu.RUnlock()

	if ok {
		histogram.With(labels).Observe(value)
	}
}

// ObserveSummary records a summary observation
func (c *PrometheusCollector) ObserveSummary(name string, value float64, labels map[string]string) {
	c.mu.RLock()
	summary, ok := c.summaries[name]
	c.mu.RUnlock()

	if ok {
		summary.With(labels).Observe(value)
	}
}

// RecordDuration records a duration in seconds
func (c *PrometheusCollector) RecordDuration(name string, duration time.Duration, labels map[string]string) {
	// Try histogram first, then summary
	c.mu.RLock()
	histogram, histOk := c.histograms[name]
	summary, summOk := c.summaries[name]
	c.mu.RUnlock()

	seconds := duration.Seconds()

	if histOk {
		histogram.With(labels).Observe(seconds)
	} else if summOk {
		summary.With(labels).Observe(seconds)
	}
}

// Handler returns an HTTP handler for the /metrics endpoint
func (c *PrometheusCollector) Handler() http.Handler {
	return promhttp.HandlerFor(c.registry, promhttp.HandlerOpts{})
}

// Registry returns the underlying Prometheus registry
func (c *PrometheusCollector) Registry() *prometheus.Registry {
	return c.registry
}
