package metrics

import (
	"encoding/json"
	"sort"
	"testing"
	"time"
)

func TestDefaultNATSMetricsConfig(t *testing.T) {
	config := DefaultNATSMetricsConfig()

	if config.URL != "nats://localhost:4222" {
		t.Errorf("Expected URL nats://localhost:4222, got %s", config.URL)
	}

	if config.Subject != "kscore.metrics" {
		t.Errorf("Expected Subject kscore.metrics, got %s", config.Subject)
	}

	if config.PublishInterval != 10*time.Second {
		t.Errorf("Expected PublishInterval 10s, got %v", config.PublishInterval)
	}

	if config.BufferSize != 10000 {
		t.Errorf("Expected BufferSize 10000, got %d", config.BufferSize)
	}

	if config.ConnectTimeout != 5*time.Second {
		t.Errorf("Expected ConnectTimeout 5s, got %v", config.ConnectTimeout)
	}

	if config.ReconnectWait != 1*time.Second {
		t.Errorf("Expected ReconnectWait 1s, got %v", config.ReconnectWait)
	}

	if config.MaxReconnects != -1 {
		t.Errorf("Expected MaxReconnects -1 (unlimited), got %d", config.MaxReconnects)
	}

	if !config.IncludeLabels {
		t.Error("Expected IncludeLabels to be true by default")
	}

	if !config.IncludeTimestamp {
		t.Error("Expected IncludeTimestamp to be true by default")
	}

	if config.SubjectPerMetric {
		t.Error("Expected SubjectPerMetric to be false by default")
	}

	if config.SubjectPerService {
		t.Error("Expected SubjectPerService to be false by default")
	}
}

func TestNATSMetricMessage(t *testing.T) {
	msg := NATSMetricMessage{
		Name:      "test_counter",
		Type:      "counter",
		Value:     42.0,
		Labels:    map[string]string{"env": "prod"},
		Timestamp: "2024-01-15T10:30:00Z",
		Service:   "kscore-server",
		Host:      "localhost",
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal NATSMetricMessage: %v", err)
	}

	var decoded NATSMetricMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NATSMetricMessage: %v", err)
	}

	if decoded.Name != msg.Name {
		t.Errorf("Expected Name %s, got %s", msg.Name, decoded.Name)
	}

	if decoded.Type != msg.Type {
		t.Errorf("Expected Type %s, got %s", msg.Type, decoded.Type)
	}

	if decoded.Value != msg.Value {
		t.Errorf("Expected Value %f, got %f", msg.Value, decoded.Value)
	}

	if decoded.Service != msg.Service {
		t.Errorf("Expected Service %s, got %s", msg.Service, decoded.Service)
	}

	if decoded.Labels["env"] != "prod" {
		t.Errorf("Expected Labels['env'] = 'prod', got %s", decoded.Labels["env"])
	}
}

func TestNATSMetricsBatch(t *testing.T) {
	batch := NATSMetricsBatch{
		Metrics: []NATSMetricMessage{
			{Name: "metric1", Type: "counter", Value: 10},
			{Name: "metric2", Type: "gauge", Value: 20},
		},
		Timestamp: "2024-01-15T10:30:00Z",
		Service:   "test-service",
		Host:      "localhost",
	}

	data, err := json.Marshal(batch)
	if err != nil {
		t.Fatalf("Failed to marshal NATSMetricsBatch: %v", err)
	}

	var decoded NATSMetricsBatch
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal NATSMetricsBatch: %v", err)
	}

	if len(decoded.Metrics) != 2 {
		t.Errorf("Expected 2 metrics, got %d", len(decoded.Metrics))
	}

	if decoded.Service != "test-service" {
		t.Errorf("Expected Service 'test-service', got %s", decoded.Service)
	}
}

func TestBuildMetricKey(t *testing.T) {
	tests := []struct {
		name     string
		metric   string
		labels   map[string]string
		expected string
	}{
		{
			name:     "no labels",
			metric:   "test_metric",
			labels:   nil,
			expected: "test_metric",
		},
		{
			name:     "empty labels",
			metric:   "test_metric",
			labels:   map[string]string{},
			expected: "test_metric",
		},
		{
			name:     "single label",
			metric:   "test_metric",
			labels:   map[string]string{"env": "prod"},
			expected: "test_metric;env=prod",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildMetricKey(tt.metric, tt.labels)
			// For single label case, just check it contains the expected parts
			if tt.name == "single label" {
				if result != "test_metric;env=prod" {
					t.Errorf("buildMetricKey() = %s, want %s", result, tt.expected)
				}
			} else if result != tt.expected {
				t.Errorf("buildMetricKey() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestParseMetricKey(t *testing.T) {
	tests := []struct {
		name           string
		key            string
		expectedName   string
		expectedLabels map[string]string
	}{
		{
			name:           "no labels",
			key:            "test_metric",
			expectedName:   "test_metric",
			expectedLabels: map[string]string{},
		},
		{
			name:           "single label",
			key:            "test_metric;env=prod",
			expectedName:   "test_metric",
			expectedLabels: map[string]string{"env": "prod"},
		},
		{
			name:           "multiple labels",
			key:            "test_metric;env=prod;region=us",
			expectedName:   "test_metric",
			expectedLabels: map[string]string{"env": "prod", "region": "us"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name, labels := parseMetricKey(tt.key)
			if name != tt.expectedName {
				t.Errorf("parseMetricKey() name = %s, want %s", name, tt.expectedName)
			}
			if len(labels) != len(tt.expectedLabels) {
				t.Errorf("parseMetricKey() labels count = %d, want %d", len(labels), len(tt.expectedLabels))
			}
			for k, v := range tt.expectedLabels {
				if labels[k] != v {
					t.Errorf("parseMetricKey() labels[%s] = %s, want %s", k, labels[k], v)
				}
			}
		})
	}
}

func TestCalculateQuantiles(t *testing.T) {
	values := []float64{1, 2, 3, 4, 5, 6, 7, 8, 9, 10}
	quantiles := calculateQuantiles(values)

	if len(quantiles) != 4 {
		t.Fatalf("Expected 4 quantiles, got %d", len(quantiles))
	}

	// Check quantile values
	expectedQuantiles := map[float64]float64{
		0.5:  5,  // median
		0.9:  9,
		0.95: 9,
		0.99: 9,
	}

	for _, q := range quantiles {
		expected, ok := expectedQuantiles[q.Quantile]
		if !ok {
			t.Errorf("Unexpected quantile %f", q.Quantile)
			continue
		}
		// Allow some tolerance due to index calculation
		if q.Value < expected-1 || q.Value > expected+1 {
			t.Errorf("Quantile %f: expected ~%f, got %f", q.Quantile, expected, q.Value)
		}
	}
}

func TestCalculateQuantilesEmpty(t *testing.T) {
	quantiles := calculateQuantiles([]float64{})
	if quantiles != nil {
		t.Error("Expected nil quantiles for empty values")
	}
}

func TestGetQuantileValue(t *testing.T) {
	sorted := []float64{1, 2, 3, 4, 5}

	tests := []struct {
		quantile float64
		expected float64
	}{
		{0.0, 1},
		{0.5, 3},
		{1.0, 5},
	}

	for _, tt := range tests {
		result := getQuantileValue(sorted, tt.quantile)
		if result != tt.expected {
			t.Errorf("getQuantileValue(%f) = %f, want %f", tt.quantile, result, tt.expected)
		}
	}
}

func TestGetQuantileValueEmpty(t *testing.T) {
	result := getQuantileValue([]float64{}, 0.5)
	if result != 0 {
		t.Errorf("getQuantileValue for empty slice = %f, want 0", result)
	}
}

func TestNATSCollectorBuildSubject(t *testing.T) {
	tests := []struct {
		name     string
		config   *NATSMetricsConfig
		expected string
	}{
		{
			name: "base subject only",
			config: &NATSMetricsConfig{
				Subject:           "kscore.metrics",
				SubjectPerService: false,
			},
			expected: "kscore.metrics",
		},
		{
			name: "with service",
			config: &NATSMetricsConfig{
				Subject:           "kscore.metrics",
				SubjectPerService: true,
				ServiceName:       "kscore-server",
			},
			expected: "kscore.metrics.kscore-server",
		},
		{
			name: "empty service name",
			config: &NATSMetricsConfig{
				Subject:           "kscore.metrics",
				SubjectPerService: true,
				ServiceName:       "",
			},
			expected: "kscore.metrics",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			collector := &NATSCollector{
				config: tt.config,
			}
			result := collector.buildSubject()
			if result != tt.expected {
				t.Errorf("buildSubject() = %s, want %s", result, tt.expected)
			}
		})
	}
}

func TestNATSCollectorStats(t *testing.T) {
	collector := &NATSCollector{
		messagesPublished: 100,
		messagesDropped:   5,
		lastError:         nil,
		lastErrorTime:     time.Time{},
	}

	published, dropped, lastErr, lastErrTime := collector.Stats()

	if published != 100 {
		t.Errorf("Expected published 100, got %d", published)
	}

	if dropped != 5 {
		t.Errorf("Expected dropped 5, got %d", dropped)
	}

	if lastErr != nil {
		t.Errorf("Expected lastErr nil, got %v", lastErr)
	}

	if !lastErrTime.IsZero() {
		t.Errorf("Expected lastErrTime to be zero, got %v", lastErrTime)
	}
}

func TestNATSCollectorIsConnectedNil(t *testing.T) {
	collector := &NATSCollector{
		conn: nil,
	}

	if collector.IsConnected() {
		t.Error("Expected IsConnected() to return false when conn is nil")
	}
}

func TestNATSCollectorQueueUpdateClosed(t *testing.T) {
	collector := &NATSCollector{
		closed:  true,
		updates: make(chan *metricUpdate, 10),
	}

	// Should not panic
	collector.queueUpdate(&metricUpdate{
		name: "test",
	})

	// Channel should be empty
	select {
	case <-collector.updates:
		t.Error("Expected no update in channel when collector is closed")
	default:
		// Expected
	}
}

func TestNATSCollectorQueueUpdateBufferFull(t *testing.T) {
	collector := &NATSCollector{
		closed:  false,
		updates: make(chan *metricUpdate, 1), // Small buffer
	}

	// Fill the buffer
	collector.updates <- &metricUpdate{name: "first"}

	// Try to queue when buffer is full
	collector.queueUpdate(&metricUpdate{name: "second"})

	if collector.messagesDropped != 1 {
		t.Errorf("Expected messagesDropped 1, got %d", collector.messagesDropped)
	}
}

func TestNATSCollectorProcessUpdateCounter(t *testing.T) {
	collector := &NATSCollector{
		counters: make(map[string]float64),
		gauges:   make(map[string]float64),
	}

	collector.processUpdate(&metricUpdate{
		name:       "test_counter",
		metricType: MetricTypeCounter,
		value:      5,
		labels:     nil,
	})

	if collector.counters["test_counter"] != 5 {
		t.Errorf("Expected counter value 5, got %f", collector.counters["test_counter"])
	}

	// Add more
	collector.processUpdate(&metricUpdate{
		name:       "test_counter",
		metricType: MetricTypeCounter,
		value:      3,
		labels:     nil,
	})

	if collector.counters["test_counter"] != 8 {
		t.Errorf("Expected counter value 8, got %f", collector.counters["test_counter"])
	}
}

func TestNATSCollectorProcessUpdateGauge(t *testing.T) {
	collector := &NATSCollector{
		counters: make(map[string]float64),
		gauges:   make(map[string]float64),
	}

	// Set absolute value
	collector.processUpdate(&metricUpdate{
		name:       "test_gauge",
		metricType: MetricTypeGauge,
		value:      42.5,
		labels:     nil,
	})

	if collector.gauges["test_gauge"] != 42.5 {
		t.Errorf("Expected gauge value 42.5, got %f", collector.gauges["test_gauge"])
	}

	// Increment
	collector.processUpdate(&metricUpdate{
		name:       "test_gauge",
		metricType: MetricTypeGauge,
		value:      1,
		labels:     nil,
	})

	if collector.gauges["test_gauge"] != 43.5 {
		t.Errorf("Expected gauge value 43.5, got %f", collector.gauges["test_gauge"])
	}

	// Decrement
	collector.processUpdate(&metricUpdate{
		name:       "test_gauge",
		metricType: MetricTypeGauge,
		value:      -1,
		labels:     nil,
	})

	if collector.gauges["test_gauge"] != 42.5 {
		t.Errorf("Expected gauge value 42.5, got %f", collector.gauges["test_gauge"])
	}
}

func TestNATSCollectorProcessUpdateHistogram(t *testing.T) {
	collector := &NATSCollector{
		histograms: make(map[string]*histogramState),
	}

	collector.processUpdate(&metricUpdate{
		name:       "test_histogram",
		metricType: MetricTypeHistogram,
		value:      0.05, // 50ms
		labels:     nil,
	})

	state, ok := collector.histograms["test_histogram"]
	if !ok {
		t.Fatal("Expected histogram to be created")
	}

	if state.count != 1 {
		t.Errorf("Expected count 1, got %d", state.count)
	}

	if state.sum != 0.05 {
		t.Errorf("Expected sum 0.05, got %f", state.sum)
	}
}

func TestNATSCollectorProcessUpdateSummary(t *testing.T) {
	collector := &NATSCollector{
		summaries: make(map[string]*summaryState),
	}

	for i := 0; i < 10; i++ {
		collector.processUpdate(&metricUpdate{
			name:       "test_summary",
			metricType: MetricTypeSummary,
			value:      float64(i),
			labels:     nil,
		})
	}

	state, ok := collector.summaries["test_summary"]
	if !ok {
		t.Fatal("Expected summary to be created")
	}

	if len(state.values) != 10 {
		t.Errorf("Expected 10 values, got %d", len(state.values))
	}
}

func TestNATSCollectorCloseAlreadyClosed(t *testing.T) {
	collector := &NATSCollector{
		closed: true,
	}

	err := collector.Close()
	if err != nil {
		t.Errorf("Close() on already closed collector should not error, got %v", err)
	}
}

func TestNATSMetricsSubscriberUnsubscribeNil(t *testing.T) {
	subscriber := &NATSMetricsSubscriber{
		sub: nil,
	}

	err := subscriber.Unsubscribe()
	if err != nil {
		t.Errorf("Unsubscribe() with nil subscription should not error, got %v", err)
	}
}

func TestNATSMetricsSubscriberClose(t *testing.T) {
	subscriber := &NATSMetricsSubscriber{
		sub: nil,
	}

	err := subscriber.Close()
	if err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

func TestBucket(t *testing.T) {
	bucket := Bucket{
		UpperBound: 0.5,
		Count:      100,
	}

	data, err := json.Marshal(bucket)
	if err != nil {
		t.Fatalf("Failed to marshal Bucket: %v", err)
	}

	var decoded Bucket
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Bucket: %v", err)
	}

	if decoded.UpperBound != 0.5 {
		t.Errorf("Expected UpperBound 0.5, got %f", decoded.UpperBound)
	}

	if decoded.Count != 100 {
		t.Errorf("Expected Count 100, got %d", decoded.Count)
	}
}

func TestQuantile(t *testing.T) {
	quantile := Quantile{
		Quantile: 0.99,
		Value:    123.45,
	}

	data, err := json.Marshal(quantile)
	if err != nil {
		t.Fatalf("Failed to marshal Quantile: %v", err)
	}

	var decoded Quantile
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal Quantile: %v", err)
	}

	if decoded.Quantile != 0.99 {
		t.Errorf("Expected Quantile 0.99, got %f", decoded.Quantile)
	}

	if decoded.Value != 123.45 {
		t.Errorf("Expected Value 123.45, got %f", decoded.Value)
	}
}

func TestHistogramMessage(t *testing.T) {
	msg := NATSMetricMessage{
		Name:  "request_duration",
		Type:  "histogram",
		Sum:   15.5,
		Count: 100,
		Buckets: []Bucket{
			{UpperBound: 0.1, Count: 50},
			{UpperBound: 0.5, Count: 80},
			{UpperBound: 1.0, Count: 95},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal histogram message: %v", err)
	}

	var decoded NATSMetricMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal histogram message: %v", err)
	}

	if len(decoded.Buckets) != 3 {
		t.Errorf("Expected 3 buckets, got %d", len(decoded.Buckets))
	}

	if decoded.Sum != 15.5 {
		t.Errorf("Expected Sum 15.5, got %f", decoded.Sum)
	}

	if decoded.Count != 100 {
		t.Errorf("Expected Count 100, got %d", decoded.Count)
	}
}

func TestSummaryMessage(t *testing.T) {
	msg := NATSMetricMessage{
		Name:  "request_size",
		Type:  "summary",
		Count: 1000,
		Quantiles: []Quantile{
			{Quantile: 0.5, Value: 100},
			{Quantile: 0.9, Value: 200},
			{Quantile: 0.99, Value: 500},
		},
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal summary message: %v", err)
	}

	var decoded NATSMetricMessage
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal summary message: %v", err)
	}

	if len(decoded.Quantiles) != 3 {
		t.Errorf("Expected 3 quantiles, got %d", len(decoded.Quantiles))
	}

	if decoded.Count != 1000 {
		t.Errorf("Expected Count 1000, got %d", decoded.Count)
	}
}

func TestMetricOmitEmpty(t *testing.T) {
	msg := NATSMetricMessage{
		Name:  "test",
		Type:  "counter",
		Value: 1,
	}

	data, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("Failed to marshal NATSMetricMessage: %v", err)
	}

	dataStr := string(data)

	// Check that empty optional fields are omitted
	if containsField(dataStr, "labels") {
		t.Error("Expected labels to be omitted when empty")
	}
	if containsField(dataStr, "timestamp") {
		t.Error("Expected timestamp to be omitted when empty")
	}
	if containsField(dataStr, "buckets") {
		t.Error("Expected buckets to be omitted when nil")
	}
	if containsField(dataStr, "quantiles") {
		t.Error("Expected quantiles to be omitted when nil")
	}
}

func containsField(s, field string) bool {
	// Simple check for field in JSON
	for i := 0; i <= len(s)-len(field)-3; i++ {
		if s[i:i+len(field)+3] == "\""+field+"\":" {
			return true
		}
	}
	return false
}

func TestAllMetricTypes(t *testing.T) {
	types := []MetricType{MetricTypeCounter, MetricTypeGauge, MetricTypeHistogram, MetricTypeSummary}
	typeStrings := []string{"counter", "gauge", "histogram", "summary"}

	for i, mt := range types {
		if string(mt) != typeStrings[i] {
			t.Errorf("MetricType %v should be %s", mt, typeStrings[i])
		}
	}
}

func TestLabelsRoundTrip(t *testing.T) {
	labels := map[string]string{
		"env":    "production",
		"region": "us-east-1",
		"app":    "kscore-server",
	}

	key := buildMetricKey("test_metric", labels)
	name, parsedLabels := parseMetricKey(key)

	if name != "test_metric" {
		t.Errorf("Expected name 'test_metric', got %s", name)
	}

	if len(parsedLabels) != len(labels) {
		t.Errorf("Expected %d labels, got %d", len(labels), len(parsedLabels))
	}

	for k, v := range labels {
		if parsedLabels[k] != v {
			t.Errorf("Label %s: expected %s, got %s", k, v, parsedLabels[k])
		}
	}
}

func TestCalculateQuantilesLargeDataset(t *testing.T) {
	// Create 1000 values
	values := make([]float64, 1000)
	for i := 0; i < 1000; i++ {
		values[i] = float64(i)
	}

	quantiles := calculateQuantiles(values)

	if len(quantiles) != 4 {
		t.Fatalf("Expected 4 quantiles, got %d", len(quantiles))
	}

	// Sort by quantile for consistent checking
	sort.Slice(quantiles, func(i, j int) bool {
		return quantiles[i].Quantile < quantiles[j].Quantile
	})

	// Check approximate values
	if quantiles[0].Quantile != 0.5 {
		t.Errorf("Expected first quantile 0.5, got %f", quantiles[0].Quantile)
	}

	// P50 should be around 499-500 for values 0-999
	if quantiles[0].Value < 490 || quantiles[0].Value > 510 {
		t.Errorf("P50 should be around 500, got %f", quantiles[0].Value)
	}
}
