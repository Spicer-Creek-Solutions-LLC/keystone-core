package traces

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// OTLPConfig holds configuration for the OTLP exporter.
type OTLPConfig struct {
	// Endpoint is the OTLP endpoint (e.g., http://tempo:4318/v1/traces)
	Endpoint string

	// Protocol is the protocol (grpc or http)
	Protocol string

	// Compression is the compression type (gzip or none)
	Compression string

	// BatchSize is the number of spans per batch
	BatchSize int

	// FlushInterval is how often to flush batches
	FlushInterval time.Duration

	// Timeout for export requests
	Timeout time.Duration

	// Headers to include in requests
	Headers map[string]string

	// MaxRetries is the maximum number of retries
	MaxRetries int

	// RetryBackoff is the initial backoff duration
	RetryBackoff time.Duration
}

// DefaultOTLPConfig returns a configuration with sensible defaults.
func DefaultOTLPConfig() OTLPConfig {
	return OTLPConfig{
		Protocol:      "http",
		Compression:   "gzip",
		BatchSize:     100,
		FlushInterval: 5 * time.Second,
		Timeout:       30 * time.Second,
		MaxRetries:    3,
		RetryBackoff:  1 * time.Second,
		Headers:       make(map[string]string),
	}
}

// OTLPExporter exports traces to an OTLP endpoint.
type OTLPExporter struct {
	config OTLPConfig
	client *http.Client
	store  *TracesStore

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	// Track exported traces
	mu             sync.Mutex
	exportedTraces map[string]bool

	// Stats
	statsMu       sync.RWMutex
	spansSent     int64
	spansFailed   int64
	batchesSent   int64
	batchesFailed int64
	lastError     error
	lastErrorTime time.Time
}

// NewOTLPExporter creates a new OTLP exporter.
func NewOTLPExporter(store *TracesStore, config OTLPConfig) *OTLPExporter {
	ctx, cancel := context.WithCancel(context.Background())
	return &OTLPExporter{
		config: config,
		client: &http.Client{
			Timeout: config.Timeout,
		},
		store:          store,
		ctx:            ctx,
		cancel:         cancel,
		exportedTraces: make(map[string]bool),
	}
}

// Start starts the OTLP exporter.
func (e *OTLPExporter) Start() error {
	if e.config.Endpoint == "" {
		return fmt.Errorf("OTLP endpoint is required")
	}

	e.wg.Add(1)
	go e.exportLoop()

	log.Printf("OTLP exporter started for %s", e.config.Endpoint)
	return nil
}

// Stop stops the OTLP exporter.
func (e *OTLPExporter) Stop() error {
	e.cancel()

	// Flush remaining traces
	e.flush()

	e.wg.Wait()
	return nil
}

// exportLoop periodically exports traces.
func (e *OTLPExporter) exportLoop() {
	defer e.wg.Done()

	ticker := time.NewTicker(e.config.FlushInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.flush()
		}
	}
}

// flush exports pending traces.
func (e *OTLPExporter) flush() {
	traces := e.store.GetPending(e.config.BatchSize)
	if len(traces) == 0 {
		return
	}

	// Filter already exported traces
	e.mu.Lock()
	var toExport []*Trace
	for _, trace := range traces {
		if !e.exportedTraces[trace.TraceID] {
			toExport = append(toExport, trace)
		}
	}
	e.mu.Unlock()

	if len(toExport) == 0 {
		return
	}

	// Convert to OTLP format
	request := e.tracesToOTLP(toExport)

	// Export
	if err := e.export(request); err != nil {
		e.recordError(err)
		e.recordFailure(countSpans(toExport))
		return
	}

	// Mark as exported
	e.mu.Lock()
	for _, trace := range toExport {
		e.exportedTraces[trace.TraceID] = true
	}
	e.mu.Unlock()

	e.recordSuccess(countSpans(toExport))
}

// countSpans counts total spans across traces.
func countSpans(traces []*Trace) int {
	count := 0
	for _, trace := range traces {
		count += len(trace.Spans)
	}
	return count
}

// OTLP types for JSON encoding

type otlpTraceRequest struct {
	ResourceSpans []otlpResourceSpans `json:"resourceSpans"`
}

type otlpResourceSpans struct {
	Resource   otlpResource    `json:"resource"`
	ScopeSpans []otlpScopeSpan `json:"scopeSpans"`
}

type otlpResource struct {
	Attributes []otlpKeyValue `json:"attributes"`
}

type otlpScopeSpan struct {
	Scope otlpScope  `json:"scope"`
	Spans []otlpSpan `json:"spans"`
}

type otlpScope struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

type otlpSpan struct {
	TraceID           string         `json:"traceId"`
	SpanID            string         `json:"spanId"`
	ParentSpanID      string         `json:"parentSpanId,omitempty"`
	Name              string         `json:"name"`
	Kind              int            `json:"kind"`
	StartTimeUnixNano string         `json:"startTimeUnixNano"`
	EndTimeUnixNano   string         `json:"endTimeUnixNano"`
	Attributes        []otlpKeyValue `json:"attributes,omitempty"`
	Events            []otlpEvent    `json:"events,omitempty"`
	Status            otlpStatus     `json:"status"`
}

type otlpKeyValue struct {
	Key   string    `json:"key"`
	Value otlpValue `json:"value"`
}

type otlpValue struct {
	StringValue string `json:"stringValue,omitempty"`
	IntValue    string `json:"intValue,omitempty"`
	BoolValue   bool   `json:"boolValue,omitempty"`
}

type otlpEvent struct {
	TimeUnixNano string         `json:"timeUnixNano"`
	Name         string         `json:"name"`
	Attributes   []otlpKeyValue `json:"attributes,omitempty"`
}

type otlpStatus struct {
	Code    int    `json:"code"`
	Message string `json:"message,omitempty"`
}

// tracesToOTLP converts traces to OTLP format.
func (e *OTLPExporter) tracesToOTLP(traces []*Trace) *otlpTraceRequest {
	// Group spans by service
	serviceSpans := make(map[string][]otlpSpan)

	for _, trace := range traces {
		for _, span := range trace.Spans {
			otlpSpan := e.spanToOTLP(span)
			serviceSpans[span.ServiceName] = append(serviceSpans[span.ServiceName], otlpSpan)
		}
	}

	// Build resource spans
	var resourceSpans []otlpResourceSpans
	for serviceName, spans := range serviceSpans {
		rs := otlpResourceSpans{
			Resource: otlpResource{
				Attributes: []otlpKeyValue{
					{Key: "service.name", Value: otlpValue{StringValue: serviceName}},
				},
			},
			ScopeSpans: []otlpScopeSpan{
				{
					Scope: otlpScope{Name: "kscore-gateway"},
					Spans: spans,
				},
			},
		}
		resourceSpans = append(resourceSpans, rs)
	}

	return &otlpTraceRequest{ResourceSpans: resourceSpans}
}

// spanToOTLP converts a span to OTLP format.
func (e *OTLPExporter) spanToOTLP(span Span) otlpSpan {
	// Convert trace and span IDs to hex
	traceID := span.TraceID
	spanID := span.SpanID
	parentSpanID := span.ParentSpanID

	// Ensure IDs are hex strings
	if len(traceID) != 32 {
		traceID = fmt.Sprintf("%032s", traceID)
	}
	if len(spanID) != 16 {
		spanID = fmt.Sprintf("%016s", spanID)
	}

	otlp := otlpSpan{
		TraceID:           traceID,
		SpanID:            spanID,
		ParentSpanID:      parentSpanID,
		Name:              span.OperationName,
		Kind:              1, // Internal
		StartTimeUnixNano: fmt.Sprintf("%d", span.StartTime.UnixNano()),
		EndTimeUnixNano:   fmt.Sprintf("%d", span.EndTime.UnixNano()),
		Status: otlpStatus{
			Code: int(span.Status),
		},
	}

	// Convert attributes
	if len(span.Attributes) > 0 {
		for k, v := range span.Attributes {
			kv := attributeToOTLP(k, v)
			otlp.Attributes = append(otlp.Attributes, kv)
		}
	}

	// Add agent_id as attribute
	otlp.Attributes = append(otlp.Attributes, otlpKeyValue{
		Key:   "agent_id",
		Value: otlpValue{StringValue: span.AgentID},
	})

	// Convert events
	if len(span.Events) > 0 {
		for _, event := range span.Events {
			otlpEvt := otlpEvent{
				TimeUnixNano: fmt.Sprintf("%d", event.Timestamp.UnixNano()),
				Name:         event.Name,
			}
			for k, v := range event.Attributes {
				otlpEvt.Attributes = append(otlpEvt.Attributes, attributeToOTLP(k, v))
			}
			otlp.Events = append(otlp.Events, otlpEvt)
		}
	}

	return otlp
}

// attributeToOTLP converts an attribute to OTLP format.
func attributeToOTLP(key string, value interface{}) otlpKeyValue {
	kv := otlpKeyValue{Key: key}

	switch v := value.(type) {
	case string:
		kv.Value.StringValue = v
	case int, int32, int64:
		kv.Value.IntValue = fmt.Sprintf("%d", v)
	case bool:
		kv.Value.BoolValue = v
	case []byte:
		kv.Value.StringValue = hex.EncodeToString(v)
	default:
		kv.Value.StringValue = fmt.Sprintf("%v", v)
	}

	return kv
}

// export sends the request to the OTLP endpoint.
func (e *OTLPExporter) export(request *otlpTraceRequest) error {
	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Compress if configured
	var body io.Reader
	contentEncoding := ""
	if e.config.Compression == "gzip" {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		if _, err := gz.Write(data); err != nil {
			return fmt.Errorf("failed to compress: %w", err)
		}
		if err := gz.Close(); err != nil {
			return fmt.Errorf("failed to close gzip: %w", err)
		}
		body = &buf
		contentEncoding = "gzip"
	} else {
		body = bytes.NewReader(data)
	}

	// Retry loop
	backoff := e.config.RetryBackoff
	for attempt := 0; attempt <= e.config.MaxRetries; attempt++ {
		if attempt > 0 {
			if err := wait.ForContext(e.ctx, backoff); err != nil {
				return err
			}
			backoff *= 2
		}

		err = e.doExport(body, contentEncoding)
		if err == nil {
			return nil
		}

		// Re-create body for retry
		if e.config.Compression == "gzip" {
			var buf bytes.Buffer
			gz := gzip.NewWriter(&buf)
			gz.Write(data)
			gz.Close()
			body = &buf
		} else {
			body = bytes.NewReader(data)
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", e.config.MaxRetries+1, err)
}

// doExport performs the HTTP request.
func (e *OTLPExporter) doExport(body io.Reader, contentEncoding string) error {
	req, err := http.NewRequestWithContext(e.ctx, http.MethodPost, e.config.Endpoint, body)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if contentEncoding != "" {
		req.Header.Set("Content-Encoding", contentEncoding)
	}

	// Add custom headers
	for k, v := range e.config.Headers {
		req.Header.Set(k, v)
	}

	resp, err := e.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode/100 != 2 {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("OTLP export failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// recordSuccess records a successful batch export.
func (e *OTLPExporter) recordSuccess(count int) {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	e.spansSent += int64(count)
	e.batchesSent++
}

// recordFailure records a failed batch export.
func (e *OTLPExporter) recordFailure(count int) {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	e.spansFailed += int64(count)
	e.batchesFailed++
}

// recordError records an error.
func (e *OTLPExporter) recordError(err error) {
	e.statsMu.Lock()
	defer e.statsMu.Unlock()
	e.lastError = err
	e.lastErrorTime = time.Now()
	log.Printf("OTLP export error: %v", err)
}

// OTLPExporterStats holds exporter statistics.
type OTLPExporterStats struct {
	SpansSent     int64
	SpansFailed   int64
	BatchesSent   int64
	BatchesFailed int64
	LastError     error
	LastErrorTime time.Time
}

// Stats returns exporter statistics.
func (e *OTLPExporter) Stats() OTLPExporterStats {
	e.statsMu.RLock()
	defer e.statsMu.RUnlock()
	return OTLPExporterStats{
		SpansSent:     e.spansSent,
		SpansFailed:   e.spansFailed,
		BatchesSent:   e.batchesSent,
		BatchesFailed: e.batchesFailed,
		LastError:     e.lastError,
		LastErrorTime: e.lastErrorTime,
	}
}
