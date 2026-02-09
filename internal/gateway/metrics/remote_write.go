package metrics

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"sync"
	"time"

	"github.com/golang/snappy"
	dto "github.com/prometheus/client_model/go"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// RemoteWriteConfig holds configuration for Prometheus remote write.
type RemoteWriteConfig struct {
	// URL is the remote write endpoint
	URL string

	// BatchSize is the number of samples per batch
	BatchSize int

	// FlushInterval is how often to flush batches
	FlushInterval time.Duration

	// Timeout for remote write requests
	Timeout time.Duration

	// MaxRetries is the maximum number of retries
	MaxRetries int

	// RetryBackoff is the initial backoff duration
	RetryBackoff time.Duration

	// MaxRetryBackoff is the maximum backoff duration
	MaxRetryBackoff time.Duration

	// Headers to include in requests
	Headers map[string]string

	// BasicAuth credentials
	BasicAuth *BasicAuth

	// BearerToken for authentication
	BearerToken string
}

// BasicAuth holds basic authentication credentials.
type BasicAuth struct {
	Username string
	Password string
}

// DefaultRemoteWriteConfig returns a configuration with sensible defaults.
func DefaultRemoteWriteConfig() RemoteWriteConfig {
	return RemoteWriteConfig{
		BatchSize:       1000,
		FlushInterval:   15 * time.Second,
		Timeout:         30 * time.Second,
		MaxRetries:      3,
		RetryBackoff:    1 * time.Second,
		MaxRetryBackoff: 30 * time.Second,
		Headers:         make(map[string]string),
	}
}

// TimeSeries represents a Prometheus time series.
type TimeSeries struct {
	Labels  []Label
	Samples []Sample
}

// Label represents a Prometheus label.
type Label struct {
	Name  string
	Value string
}

// Sample represents a Prometheus sample.
type Sample struct {
	Value     float64
	Timestamp int64
}

// WriteRequest represents a Prometheus remote write request.
type WriteRequest struct {
	Timeseries []TimeSeries
}

// RemoteWriter writes metrics to a Prometheus remote write endpoint.
type RemoteWriter struct {
	config RemoteWriteConfig
	client *http.Client
	store  *Store

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Buffer for pending samples
	mu      sync.Mutex
	pending []TimeSeries

	// Stats
	statsMu       sync.RWMutex
	samplesSent   int64
	samplesFailed int64
	batchesSent   int64
	batchesFailed int64
	lastError     error
	lastErrorTime time.Time
}

// NewRemoteWriter creates a new remote writer.
func NewRemoteWriter(store *Store, config RemoteWriteConfig) *RemoteWriter {
	ctx, cancel := context.WithCancel(context.Background())
	return &RemoteWriter{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		store:  store,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the remote writer.
func (w *RemoteWriter) Start() error {
	if w.config.URL == "" {
		return fmt.Errorf("remote write URL is required")
	}

	w.wg.Add(1)
	go w.flushLoop()

	log.Printf("Remote writer started for %s", w.config.URL)
	return nil
}

// Stop stops the remote writer.
func (w *RemoteWriter) Stop() error {
	w.cancel()

	// Flush remaining samples
	w.flush()

	w.wg.Wait()
	return nil
}

// flushLoop periodically flushes metrics.
func (w *RemoteWriter) flushLoop() {
	defer w.wg.Done()

	ticker := time.NewTicker(w.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-ticker.C:
			w.collectAndFlush()
		}
	}
}

// collectAndFlush collects metrics from the store and flushes them.
func (w *RemoteWriter) collectAndFlush() {
	families := w.store.GetAllFamilies()

	// Convert to time series
	timeSeries := w.convertToTimeSeries(families)
	if len(timeSeries) == 0 {
		return
	}

	// Send in batches
	for i := 0; i < len(timeSeries); i += w.config.BatchSize {
		end := i + w.config.BatchSize
		if end > len(timeSeries) {
			end = len(timeSeries)
		}
		batch := timeSeries[i:end]
		w.sendBatch(batch)
	}
}

// convertToTimeSeries converts metric families to Prometheus time series.
func (w *RemoteWriter) convertToTimeSeries(families []*dto.MetricFamily) []TimeSeries {
	var result []TimeSeries
	now := time.Now().UnixMilli()

	for _, family := range families {
		if family == nil || family.Name == nil {
			continue
		}

		for _, m := range family.Metric {
			labels := make([]Label, 0, len(m.Label)+1)

			// Add metric name label
			labels = append(labels, Label{
				Name:  "__name__",
				Value: *family.Name,
			})

			// Add other labels
			for _, l := range m.Label {
				if l.Name != nil && l.Value != nil {
					labels = append(labels, Label{
						Name:  *l.Name,
						Value: *l.Value,
					})
				}
			}

			// Get value and timestamp
			value, ts := w.getValueAndTimestamp(m, family.Type)
			if ts == 0 {
				ts = now
			}

			result = append(result, TimeSeries{
				Labels: labels,
				Samples: []Sample{
					{
						Value:     value,
						Timestamp: ts,
					},
				},
			})
		}
	}

	return result
}

// getValueAndTimestamp extracts value and timestamp from a metric.
func (w *RemoteWriter) getValueAndTimestamp(m *dto.Metric, metricType *dto.MetricType) (value float64, timestamp int64) {
	var ts int64

	if m.TimestampMs != nil {
		ts = *m.TimestampMs
	}

	switch {
	case m.Gauge != nil:
		value = m.Gauge.GetValue()
	case m.Counter != nil:
		value = m.Counter.GetValue()
	case m.Untyped != nil:
		value = m.Untyped.GetValue()
	case m.Summary != nil:
		value = m.Summary.GetSampleSum()
	case m.Histogram != nil:
		value = m.Histogram.GetSampleSum()
	}

	return value, ts
}

// marshalWriteRequest marshals a WriteRequest to protobuf wire format.
// This implements a simplified version of the Prometheus remote write protocol.
func (w *RemoteWriter) marshalWriteRequest(req *WriteRequest) ([]byte, error) {
	// Use a simple protobuf-compatible encoding
	// Field 1: repeated TimeSeries timeseries
	var buf bytes.Buffer

	for _, ts := range req.Timeseries {
		// Encode TimeSeries as a nested message
		tsData := w.marshalTimeSeries(&ts)

		// Write field 1, wire type 2 (length-delimited)
		buf.WriteByte(0x0a) // field 1, wire type 2
		writeVarint(&buf, uint64(len(tsData)))
		buf.Write(tsData)
	}

	return buf.Bytes(), nil
}

// marshalTimeSeries marshals a single TimeSeries to protobuf.
func (w *RemoteWriter) marshalTimeSeries(ts *TimeSeries) []byte {
	var buf bytes.Buffer

	// Field 1: repeated Label labels
	for _, l := range ts.Labels {
		labelData := w.marshalLabel(&l)
		buf.WriteByte(0x0a) // field 1, wire type 2
		writeVarint(&buf, uint64(len(labelData)))
		buf.Write(labelData)
	}

	// Field 2: repeated Sample samples
	for _, s := range ts.Samples {
		sampleData := w.marshalSample(&s)
		buf.WriteByte(0x12) // field 2, wire type 2
		writeVarint(&buf, uint64(len(sampleData)))
		buf.Write(sampleData)
	}

	return buf.Bytes()
}

// marshalLabel marshals a Label to protobuf.
func (w *RemoteWriter) marshalLabel(l *Label) []byte {
	var buf bytes.Buffer

	// Field 1: string name
	buf.WriteByte(0x0a) // field 1, wire type 2
	writeVarint(&buf, uint64(len(l.Name)))
	buf.WriteString(l.Name)

	// Field 2: string value
	buf.WriteByte(0x12) // field 2, wire type 2
	writeVarint(&buf, uint64(len(l.Value)))
	buf.WriteString(l.Value)

	return buf.Bytes()
}

// marshalSample marshals a Sample to protobuf.
func (w *RemoteWriter) marshalSample(s *Sample) []byte {
	var buf bytes.Buffer

	// Field 1: double value (wire type 1 = 64-bit)
	buf.WriteByte(0x09) // field 1, wire type 1
	writeFloat64(&buf, s.Value)

	// Field 2: int64 timestamp (wire type 0 = varint)
	buf.WriteByte(0x10) // field 2, wire type 0
	//nolint:gosec // G115: Unix timestamp in milliseconds is positive and fits in uint64
	writeVarint(&buf, uint64(s.Timestamp))

	return buf.Bytes()
}

// writeVarint writes a varint to the buffer.
func writeVarint(buf *bytes.Buffer, v uint64) {
	for v >= 0x80 {
		buf.WriteByte(byte(v) | 0x80)
		v >>= 7
	}
	buf.WriteByte(byte(v))
}

// writeFloat64 writes a float64 in little-endian format.
func writeFloat64(buf *bytes.Buffer, v float64) {
	bits := math.Float64bits(v)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, bits)
	buf.Write(b)
}

// sendBatch sends a batch of time series.
func (w *RemoteWriter) sendBatch(batch []TimeSeries) {
	req := &WriteRequest{
		Timeseries: batch,
	}

	data, err := w.marshalWriteRequest(req)
	if err != nil {
		w.recordError(fmt.Errorf("failed to marshal write request: %w", err))
		return
	}

	compressed := snappy.Encode(nil, data)

	// Retry loop
	backoff := w.config.RetryBackoff
	for attempt := 0; attempt <= w.config.MaxRetries; attempt++ {
		if attempt > 0 {
			if err := wait.ForContext(w.ctx, backoff); err != nil {
				return
			}
			backoff *= 2
			if backoff > w.config.MaxRetryBackoff {
				backoff = w.config.MaxRetryBackoff
			}
		}

		err = w.doSend(compressed)
		if err == nil {
			w.recordSuccess(len(batch))
			return
		}
	}

	w.recordError(fmt.Errorf("failed to send batch after %d attempts: %w", w.config.MaxRetries+1, err))
	w.recordFailure(len(batch))
}

// doSend performs the HTTP request.
func (w *RemoteWriter) doSend(data []byte) error {
	req, err := http.NewRequestWithContext(w.ctx, http.MethodPost, w.config.URL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-protobuf")
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	// Add custom headers
	for k, v := range w.config.Headers {
		req.Header.Set(k, v)
	}

	// Add authentication
	if w.config.BasicAuth != nil {
		req.SetBasicAuth(w.config.BasicAuth.Username, w.config.BasicAuth.Password)
	} else if w.config.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+w.config.BearerToken)
	}

	resp, err := w.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("remote write failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// flush flushes any pending samples.
func (w *RemoteWriter) flush() {
	w.mu.Lock()
	pending := w.pending
	w.pending = nil
	w.mu.Unlock()

	if len(pending) > 0 {
		w.sendBatch(pending)
	}
}

// recordSuccess records a successful batch send.
func (w *RemoteWriter) recordSuccess(count int) {
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	w.samplesSent += int64(count)
	w.batchesSent++
}

// recordFailure records a failed batch send.
func (w *RemoteWriter) recordFailure(count int) {
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	w.samplesFailed += int64(count)
	w.batchesFailed++
}

// recordError records an error.
func (w *RemoteWriter) recordError(err error) {
	w.statsMu.Lock()
	defer w.statsMu.Unlock()
	w.lastError = err
	w.lastErrorTime = time.Now()
	log.Printf("Remote write error: %v", err)
}

// RemoteWriterStats holds remote writer statistics.
type RemoteWriterStats struct {
	SamplesSent   int64
	SamplesFailed int64
	BatchesSent   int64
	BatchesFailed int64
	LastError     error
	LastErrorTime time.Time
}

// Stats returns remote writer statistics.
func (w *RemoteWriter) Stats() RemoteWriterStats {
	w.statsMu.RLock()
	defer w.statsMu.RUnlock()
	return RemoteWriterStats{
		SamplesSent:   w.samplesSent,
		SamplesFailed: w.samplesFailed,
		BatchesSent:   w.batchesSent,
		BatchesFailed: w.batchesFailed,
		LastError:     w.lastError,
		LastErrorTime: w.lastErrorTime,
	}
}
