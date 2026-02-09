// Package metrics provides NATS metrics transport for centralized collection.
// Epic 15: Observability Enhancements - NATS telemetry transport.
package metrics

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

// NATSMetricsConfig configures the NATS metrics transport.
type NATSMetricsConfig struct {
	// URL is the NATS server URL (e.g., "nats://localhost:4222")
	URL string

	// Subject is the base subject for metrics
	// Default: "kscore.metrics"
	Subject string

	// SubjectPerMetric uses separate subjects per metric name
	// e.g., kscore.metrics.kscore_agents_total
	SubjectPerMetric bool

	// SubjectPerService uses separate subjects per service
	// e.g., kscore.metrics.kscore-server
	SubjectPerService bool

	// ServiceName identifies the metrics source
	ServiceName string

	// PublishInterval is how often to publish aggregated metrics
	// Default: 10s
	PublishInterval time.Duration

	// BufferSize is the number of metric updates to buffer
	// Default: 10000
	BufferSize int

	// ConnectTimeout is the timeout for connecting to NATS
	// Default: 5s
	ConnectTimeout time.Duration

	// ReconnectWait is the wait time between reconnection attempts
	// Default: 1s
	ReconnectWait time.Duration

	// MaxReconnects is the maximum number of reconnection attempts
	// Default: -1 (unlimited)
	MaxReconnects int

	// IncludeLabels includes label values in the published metrics
	// Default: true
	IncludeLabels bool

	// IncludeTimestamp includes timestamp in metric messages
	// Default: true
	IncludeTimestamp bool

	// Credentials for authentication
	Token    string
	User     string
	Password string
	NKeyFile string
	CredFile string
}

// DefaultNATSMetricsConfig returns a default NATS metrics configuration.
func DefaultNATSMetricsConfig() *NATSMetricsConfig {
	return &NATSMetricsConfig{
		URL:               "nats://localhost:4222",
		Subject:           "kscore.metrics",
		SubjectPerMetric:  false,
		SubjectPerService: false,
		PublishInterval:   10 * time.Second,
		BufferSize:        10000,
		ConnectTimeout:    5 * time.Second,
		ReconnectWait:     1 * time.Second,
		MaxReconnects:     -1,
		IncludeLabels:     true,
		IncludeTimestamp:  true,
	}
}

// NATSMetricMessage is the structure sent over NATS.
type NATSMetricMessage struct {
	// Metric identification
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Labels map[string]string `json:"labels,omitempty"`

	// Value based on metric type
	Value     float64    `json:"value,omitempty"`     // For counters and gauges
	Count     uint64     `json:"count,omitempty"`     // For histograms/summaries
	Sum       float64    `json:"sum,omitempty"`       // For histograms/summaries
	Buckets   []Bucket   `json:"buckets,omitempty"`   // For histograms
	Quantiles []Quantile `json:"quantiles,omitempty"` // For summaries

	// Metadata
	Timestamp string `json:"timestamp,omitempty"`
	Service   string `json:"service,omitempty"`
	Host      string `json:"host,omitempty"`
}

// Bucket represents a histogram bucket.
type Bucket struct {
	UpperBound float64 `json:"upper_bound"`
	Count      uint64  `json:"count"`
}

// Quantile represents a summary quantile.
type Quantile struct {
	Quantile float64 `json:"quantile"`
	Value    float64 `json:"value"`
}

// NATSMetricsBatch is a batch of metrics sent together.
type NATSMetricsBatch struct {
	Metrics   []NATSMetricMessage `json:"metrics"`
	Timestamp string              `json:"timestamp"`
	Service   string              `json:"service"`
	Host      string              `json:"host"`
}

// metricUpdate represents a single metric update to be buffered.
type metricUpdate struct {
	name       string
	metricType MetricType
	value      float64
	labels     map[string]string
	timestamp  time.Time
}

// NATSCollector implements Collector with NATS transport.
type NATSCollector struct {
	config   *NATSMetricsConfig
	conn     *nats.Conn
	updates  chan *metricUpdate
	mu       sync.RWMutex
	closed   bool
	hostname string

	// Aggregated metrics state
	counters   map[string]float64
	gauges     map[string]float64
	histograms map[string]*histogramState
	summaries  map[string]*summaryState

	// Stats
	messagesPublished int64
	messagesDropped   int64
	lastError         error
	lastErrorTime     time.Time
}

type histogramState struct {
	sum     float64
	count   uint64
	buckets map[float64]uint64
}

type summaryState struct {
	values []float64
}

// NewNATSCollector creates a new NATS metrics collector.
func NewNATSCollector(config *NATSMetricsConfig) (*NATSCollector, error) {
	if config == nil {
		config = DefaultNATSMetricsConfig()
	}

	// Build NATS options
	opts := []nats.Option{
		nats.Name("kscore-metrics-" + config.ServiceName),
		nats.Timeout(config.ConnectTimeout),
		nats.ReconnectWait(config.ReconnectWait),
		nats.MaxReconnects(config.MaxReconnects),
	}

	// Add authentication
	if config.Token != "" {
		opts = append(opts, nats.Token(config.Token))
	}
	if config.User != "" && config.Password != "" {
		opts = append(opts, nats.UserInfo(config.User, config.Password))
	}
	if config.NKeyFile != "" {
		opt, err := nats.NkeyOptionFromSeed(config.NKeyFile)
		if err != nil {
			return nil, fmt.Errorf("failed to load NKey: %w", err)
		}
		opts = append(opts, opt)
	}
	if config.CredFile != "" {
		opts = append(opts, nats.UserCredentials(config.CredFile))
	}

	// Connect to NATS
	conn, err := nats.Connect(config.URL, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to NATS: %w", err)
	}

	bufferSize := config.BufferSize
	if bufferSize <= 0 {
		bufferSize = 10000
	}

	collector := &NATSCollector{
		config:     config,
		conn:       conn,
		updates:    make(chan *metricUpdate, bufferSize),
		counters:   make(map[string]float64),
		gauges:     make(map[string]float64),
		histograms: make(map[string]*histogramState),
		summaries:  make(map[string]*summaryState),
	}

	// Start the publish loop
	go collector.publishLoop()

	return collector, nil
}

// IncCounter increments a counter metric.
func (c *NATSCollector) IncCounter(name string, labels map[string]string) {
	c.AddCounter(name, 1, labels)
}

// AddCounter adds a value to a counter metric.
func (c *NATSCollector) AddCounter(name string, value float64, labels map[string]string) {
	c.queueUpdate(&metricUpdate{
		name:       name,
		metricType: MetricTypeCounter,
		value:      value,
		labels:     labels,
		timestamp:  time.Now(),
	})
}

// SetGauge sets a gauge metric.
func (c *NATSCollector) SetGauge(name string, value float64, labels map[string]string) {
	c.queueUpdate(&metricUpdate{
		name:       name,
		metricType: MetricTypeGauge,
		value:      value,
		labels:     labels,
		timestamp:  time.Now(),
	})
}

// IncGauge increments a gauge metric.
func (c *NATSCollector) IncGauge(name string, labels map[string]string) {
	c.queueUpdate(&metricUpdate{
		name:       name,
		metricType: MetricTypeGauge,
		value:      1,
		labels:     labels,
		timestamp:  time.Now(),
	})
}

// DecGauge decrements a gauge metric.
func (c *NATSCollector) DecGauge(name string, labels map[string]string) {
	c.queueUpdate(&metricUpdate{
		name:       name,
		metricType: MetricTypeGauge,
		value:      -1,
		labels:     labels,
		timestamp:  time.Now(),
	})
}

// ObserveHistogram records a histogram observation.
func (c *NATSCollector) ObserveHistogram(name string, value float64, labels map[string]string) {
	c.queueUpdate(&metricUpdate{
		name:       name,
		metricType: MetricTypeHistogram,
		value:      value,
		labels:     labels,
		timestamp:  time.Now(),
	})
}

// ObserveSummary records a summary observation.
func (c *NATSCollector) ObserveSummary(name string, value float64, labels map[string]string) {
	c.queueUpdate(&metricUpdate{
		name:       name,
		metricType: MetricTypeSummary,
		value:      value,
		labels:     labels,
		timestamp:  time.Now(),
	})
}

// RecordDuration records a duration in seconds.
func (c *NATSCollector) RecordDuration(name string, duration time.Duration, labels map[string]string) {
	c.ObserveHistogram(name, duration.Seconds(), labels)
}

// queueUpdate adds a metric update to the buffer.
func (c *NATSCollector) queueUpdate(update *metricUpdate) {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return
	}
	c.mu.RUnlock()

	select {
	case c.updates <- update:
	default:
		// Buffer full
		c.mu.Lock()
		c.messagesDropped++
		c.mu.Unlock()
	}
}

// buildMetricKey creates a unique key for a metric with labels.
func buildMetricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	// Simple key construction
	key := name
	for k, v := range labels {
		key += fmt.Sprintf(";%s=%s", k, v)
	}
	return key
}

// publishLoop periodically aggregates and publishes metrics.
func (c *NATSCollector) publishLoop() {
	ticker := time.NewTicker(c.config.PublishInterval)
	defer ticker.Stop()

	for {
		select {
		case update, ok := <-c.updates:
			if !ok {
				return
			}
			c.processUpdate(update)

		case <-ticker.C:
			c.publishMetrics()
		}
	}
}

// processUpdate processes a single metric update.
func (c *NATSCollector) processUpdate(update *metricUpdate) {
	c.mu.Lock()
	defer c.mu.Unlock()

	key := buildMetricKey(update.name, update.labels)

	switch update.metricType {
	case MetricTypeCounter:
		c.counters[key] += update.value

	case MetricTypeGauge:
		// For gauges, check if this is an increment/decrement or absolute
		// Simple heuristic: if value is 1 or -1, treat as relative
		if update.value == 1 || update.value == -1 {
			c.gauges[key] += update.value
		} else {
			c.gauges[key] = update.value
		}

	case MetricTypeHistogram:
		if _, exists := c.histograms[key]; !exists {
			c.histograms[key] = &histogramState{
				buckets: make(map[float64]uint64),
			}
		}
		state := c.histograms[key]
		state.sum += update.value
		state.count++
		// Update bucket counts
		for _, bound := range DefaultBuckets {
			if update.value <= bound {
				state.buckets[bound]++
			}
		}

	case MetricTypeSummary:
		if _, exists := c.summaries[key]; !exists {
			c.summaries[key] = &summaryState{
				values: make([]float64, 0, 1000),
			}
		}
		state := c.summaries[key]
		// Keep last 1000 values for quantile calculation
		if len(state.values) >= 1000 {
			state.values = state.values[1:]
		}
		state.values = append(state.values, update.value)
	}
}

// publishMetrics publishes aggregated metrics to NATS.
func (c *NATSCollector) publishMetrics() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.conn == nil || !c.conn.IsConnected() {
		return
	}

	batch := NATSMetricsBatch{
		Metrics:   make([]NATSMetricMessage, 0),
		Timestamp: time.Now().Format(time.RFC3339Nano),
		Service:   c.config.ServiceName,
		Host:      c.hostname,
	}

	// Add counters
	for key, value := range c.counters {
		name, labels := parseMetricKey(key)
		msg := NATSMetricMessage{
			Name:  name,
			Type:  string(MetricTypeCounter),
			Value: value,
		}
		if c.config.IncludeLabels && len(labels) > 0 {
			msg.Labels = labels
		}
		batch.Metrics = append(batch.Metrics, msg)
	}

	// Add gauges
	for key, value := range c.gauges {
		name, labels := parseMetricKey(key)
		msg := NATSMetricMessage{
			Name:  name,
			Type:  string(MetricTypeGauge),
			Value: value,
		}
		if c.config.IncludeLabels && len(labels) > 0 {
			msg.Labels = labels
		}
		batch.Metrics = append(batch.Metrics, msg)
	}

	// Add histograms
	for key, state := range c.histograms {
		name, labels := parseMetricKey(key)
		buckets := make([]Bucket, 0, len(state.buckets))
		for bound, count := range state.buckets {
			buckets = append(buckets, Bucket{
				UpperBound: bound,
				Count:      count,
			})
		}
		msg := NATSMetricMessage{
			Name:    name,
			Type:    string(MetricTypeHistogram),
			Sum:     state.sum,
			Count:   state.count,
			Buckets: buckets,
		}
		if c.config.IncludeLabels && len(labels) > 0 {
			msg.Labels = labels
		}
		batch.Metrics = append(batch.Metrics, msg)
	}

	// Add summaries
	for key, state := range c.summaries {
		if len(state.values) == 0 {
			continue
		}
		name, labels := parseMetricKey(key)
		quantiles := calculateQuantiles(state.values)
		msg := NATSMetricMessage{
			Name:      name,
			Type:      string(MetricTypeSummary),
			Count:     uint64(len(state.values)),
			Quantiles: quantiles,
		}
		if c.config.IncludeLabels && len(labels) > 0 {
			msg.Labels = labels
		}
		batch.Metrics = append(batch.Metrics, msg)
	}

	if len(batch.Metrics) == 0 {
		return
	}

	// Publish batch
	data, err := json.Marshal(batch)
	if err != nil {
		c.lastError = err
		c.lastErrorTime = time.Now()
		return
	}

	subject := c.buildSubject()
	if err := c.conn.Publish(subject, data); err != nil {
		c.lastError = err
		c.lastErrorTime = time.Now()
		return
	}

	c.messagesPublished++
}

// parseMetricKey parses a metric key into name and labels.
func parseMetricKey(key string) (name string, labels map[string]string) {
	labels = make(map[string]string)

	// Find first semicolon
	idx := 0
	for i, ch := range key {
		if ch == ';' {
			idx = i
			break
		}
	}

	if idx == 0 {
		return key, labels
	}

	name = key[:idx]
	labelPart := key[idx+1:]

	// Parse label pairs
	for labelPart != "" {
		// Find next semicolon or end
		nextSemi := len(labelPart)
		for i, ch := range labelPart {
			if ch == ';' {
				nextSemi = i
				break
			}
		}

		pair := labelPart[:nextSemi]
		// Find equals sign
		for i, ch := range pair {
			if ch == '=' {
				labels[pair[:i]] = pair[i+1:]
				break
			}
		}

		if nextSemi < len(labelPart) {
			labelPart = labelPart[nextSemi+1:]
		} else {
			break
		}
	}

	return name, labels
}

// calculateQuantiles calculates quantiles from a slice of values.
func calculateQuantiles(values []float64) []Quantile {
	if len(values) == 0 {
		return nil
	}

	// Sort values (simple insertion sort for small arrays)
	sorted := make([]float64, len(values))
	copy(sorted, values)
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j-1] > sorted[j]; j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}

	quantiles := []Quantile{
		{Quantile: 0.5, Value: getQuantileValue(sorted, 0.5)},
		{Quantile: 0.9, Value: getQuantileValue(sorted, 0.9)},
		{Quantile: 0.95, Value: getQuantileValue(sorted, 0.95)},
		{Quantile: 0.99, Value: getQuantileValue(sorted, 0.99)},
	}

	return quantiles
}

// getQuantileValue gets the value at a specific quantile.
func getQuantileValue(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * q)
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// buildSubject builds the NATS subject for metrics.
func (c *NATSCollector) buildSubject() string {
	subject := c.config.Subject
	if c.config.SubjectPerService && c.config.ServiceName != "" {
		subject = fmt.Sprintf("%s.%s", subject, c.config.ServiceName)
	}
	return subject
}

// Stats returns collector statistics.
func (c *NATSCollector) Stats() (published, dropped int64, lastErrTime time.Time, lastErr error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.messagesPublished, c.messagesDropped, c.lastErrorTime, c.lastError
}

// IsConnected returns whether NATS is connected.
func (c *NATSCollector) IsConnected() bool {
	return c.conn != nil && c.conn.IsConnected()
}

// Close closes the NATS collector.
func (c *NATSCollector) Close() error {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	c.mu.Unlock()

	// Close updates channel
	close(c.updates)

	// Final publish
	c.publishMetrics()

	// Close NATS connection
	if c.conn != nil {
		c.conn.Drain()
		c.conn.Close()
	}

	return nil
}

// NATSMetricsSubscriber subscribes to NATS metrics messages.
type NATSMetricsSubscriber struct {
	conn         *nats.Conn
	sub          *nats.Subscription
	batchHandler func(*NATSMetricsBatch)
	mu           sync.Mutex
}

// NewNATSMetricsSubscriber creates a subscriber for NATS metrics.
func NewNATSMetricsSubscriber(conn *nats.Conn, subject string, handler func(*NATSMetricsBatch)) (*NATSMetricsSubscriber, error) {
	s := &NATSMetricsSubscriber{
		conn:         conn,
		batchHandler: handler,
	}

	sub, err := conn.Subscribe(subject, func(msg *nats.Msg) {
		var batch NATSMetricsBatch
		if err := json.Unmarshal(msg.Data, &batch); err != nil {
			return
		}
		if handler != nil {
			handler(&batch)
		}
	})
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe: %w", err)
	}

	s.sub = sub
	return s, nil
}

// Unsubscribe unsubscribes from NATS.
func (s *NATSMetricsSubscriber) Unsubscribe() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.sub != nil {
		return s.sub.Unsubscribe()
	}
	return nil
}

// Close closes the subscriber.
func (s *NATSMetricsSubscriber) Close() error {
	return s.Unsubscribe()
}

// Subscription returns the underlying NATS subscription.
func (s *NATSMetricsSubscriber) Subscription() *nats.Subscription {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.sub
}

// NewNATSMetricsMessageSubscriber creates a subscriber that receives individual metrics.
// This is a convenience wrapper that unwraps batches into individual messages.
func NewNATSMetricsMessageSubscriber(conn *nats.Conn, subject string, handler func(*NATSMetricMessage)) (*NATSMetricsSubscriber, error) {
	batchHandler := func(batch *NATSMetricsBatch) {
		for i := range batch.Metrics {
			handler(&batch.Metrics[i])
		}
	}
	return NewNATSMetricsSubscriber(conn, subject, batchHandler)
}
