package query

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"time"
)

// InMemoryTracesQuerier is a simple in-memory traces querier for testing
// In production, this would be replaced with a Jaeger or Tempo client
type InMemoryTracesQuerier struct {
	traces map[string]*TraceResult
	mu     sync.RWMutex
}

// NewInMemoryTracesQuerier creates a new in-memory traces querier
func NewInMemoryTracesQuerier() *InMemoryTracesQuerier {
	return &InMemoryTracesQuerier{
		traces: make(map[string]*TraceResult),
	}
}

// AddTrace adds a trace (for testing)
func (t *InMemoryTracesQuerier) AddTrace(trace *TraceResult) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.traces[trace.TraceID] = trace
}

// Query executes a traces query
func (t *InMemoryTracesQuerier) Query(ctx context.Context, query *TracesQuery) (*TracesResult, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	// Filter traces
	filtered := make([]*TraceResult, 0)
	for _, trace := range t.traces {
		if matchesQuery(trace, query) {
			filtered = append(filtered, trace)
		}
	}

	// Sort by start time (most recent first)
	sort.Slice(filtered, func(i, j int) bool {
		return getTraceStartTime(filtered[i]).After(getTraceStartTime(filtered[j]))
	})

	// Apply limit
	limit := query.Limit
	if limit == 0 {
		limit = 20 // Default limit
	}

	total := len(filtered)
	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	// Convert to result
	results := make([]TraceResult, len(filtered))
	for i, trace := range filtered {
		results[i] = *trace
	}

	return &TracesResult{
		Traces: results,
		Total:  total,
	}, nil
}

// GetTrace retrieves a specific trace by ID
func (t *InMemoryTracesQuerier) GetTrace(ctx context.Context, traceID string) (*TraceResult, error) {
	t.mu.RLock()
	defer t.mu.RUnlock()

	trace, exists := t.traces[traceID]
	if !exists {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}

	return trace, nil
}

// matchesQuery checks if a trace matches the query criteria
func matchesQuery(trace *TraceResult, query *TracesQuery) bool {
	// Check service filter
	if query.Service != "" {
		serviceMatches := false
		for _, process := range trace.Processes {
			if process.ServiceName == query.Service {
				serviceMatches = true
				break
			}
		}
		if !serviceMatches {
			return false
		}
	}

	// Check operation filter
	if query.Operation != "" {
		operationMatches := false
		for i := range trace.Spans {
			if trace.Spans[i].OperationName == query.Operation {
				operationMatches = true
				break
			}
		}
		if !operationMatches {
			return false
		}
	}

	// Check tags filter
	if len(query.Tags) > 0 {
		tagsMatch := false
		for i := range trace.Spans {
			if matchesTags(trace.Spans[i].Tags, query.Tags) {
				tagsMatch = true
				break
			}
		}
		if !tagsMatch {
			return false
		}
	}

	// Check time range
	if query.Range != nil {
		traceStart := getTraceStartTime(trace)
		if traceStart.Before(query.Range.Start) || traceStart.After(query.Range.End) {
			return false
		}
	}

	// Check duration filters
	if query.MinDuration > 0 || query.MaxDuration > 0 {
		traceDuration := getTraceDuration(trace)
		if query.MinDuration > 0 && traceDuration < query.MinDuration {
			return false
		}
		if query.MaxDuration > 0 && traceDuration > query.MaxDuration {
			return false
		}
	}

	return true
}

// matchesTags checks if span tags match the query tags
func matchesTags(spanTags map[string]interface{}, queryTags map[string]string) bool {
	for key, value := range queryTags {
		spanValue, exists := spanTags[key]
		if !exists {
			return false
		}
		if fmt.Sprintf("%v", spanValue) != value {
			return false
		}
	}
	return true
}

// getTraceStartTime returns the earliest span start time in a trace
func getTraceStartTime(trace *TraceResult) time.Time {
	if len(trace.Spans) == 0 {
		return time.Time{}
	}

	earliest := trace.Spans[0].StartTime
	spans := trace.Spans[1:]
	for i := range spans {
		if spans[i].StartTime.Before(earliest) {
			earliest = spans[i].StartTime
		}
	}
	return earliest
}

// getTraceDuration returns the total duration of a trace
func getTraceDuration(trace *TraceResult) time.Duration {
	if len(trace.Spans) == 0 {
		return 0
	}

	start := getTraceStartTime(trace)
	var end time.Time

	for i := range trace.Spans {
		spanEnd := trace.Spans[i].StartTime.Add(trace.Spans[i].Duration)
		if spanEnd.After(end) {
			end = spanEnd
		}
	}

	return end.Sub(start)
}

// JaegerConfig holds configuration for the Jaeger querier
type JaegerConfig struct {
	// Address is the Jaeger query service address (e.g., "http://jaeger-query:16686")
	Address string

	// Username for basic auth (optional)
	Username string

	// Password for basic auth (optional)
	Password string

	// TLSConfig for secure connections (optional)
	TLSConfig *tls.Config

	// Timeout for HTTP requests
	Timeout time.Duration
}

// JaegerQuerier queries traces from Jaeger
type JaegerQuerier struct {
	config *JaegerConfig
	client *http.Client
}

// NewJaegerQuerier creates a new Jaeger traces querier
func NewJaegerQuerier(address string) *JaegerQuerier {
	return NewJaegerQuerierWithConfig(&JaegerConfig{
		Address: address,
		Timeout: 30 * time.Second,
	})
}

// NewJaegerQuerierWithConfig creates a new Jaeger traces querier with full configuration
func NewJaegerQuerierWithConfig(config *JaegerConfig) *JaegerQuerier {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	transport := &http.Transport{}
	if config.TLSConfig != nil {
		transport.TLSClientConfig = config.TLSConfig
	}

	return &JaegerQuerier{
		config: config,
		client: &http.Client{
			Timeout:   config.Timeout,
			Transport: transport,
		},
	}
}

// jaegerTracesResponse represents the Jaeger traces API response
type jaegerTracesResponse struct {
	Data   []jaegerTrace `json:"data"`
	Total  int           `json:"total"`
	Limit  int           `json:"limit"`
	Offset int           `json:"offset"`
	Errors []struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	} `json:"errors,omitempty"`
}

// jaegerTrace represents a trace in Jaeger's format
type jaegerTrace struct {
	TraceID   string                   `json:"traceID"`
	Spans     []jaegerSpan             `json:"spans"`
	Processes map[string]jaegerProcess `json:"processes"`
	Warnings  []string                 `json:"warnings,omitempty"`
}

// jaegerSpan represents a span in Jaeger's format
type jaegerSpan struct {
	TraceID       string           `json:"traceID"`
	SpanID        string           `json:"spanID"`
	OperationName string           `json:"operationName"`
	References    []jaegerSpanRef  `json:"references,omitempty"`
	StartTime     int64            `json:"startTime"` // microseconds since epoch
	Duration      int64            `json:"duration"`  // microseconds
	Tags          []jaegerKeyValue `json:"tags,omitempty"`
	Logs          []jaegerLog      `json:"logs,omitempty"`
	ProcessID     string           `json:"processID"`
	Warnings      []string         `json:"warnings,omitempty"`
}

// jaegerSpanRef represents a reference to another span
type jaegerSpanRef struct {
	RefType string `json:"refType"`
	TraceID string `json:"traceID"`
	SpanID  string `json:"spanID"`
}

// jaegerKeyValue represents a key-value pair in Jaeger's format
type jaegerKeyValue struct {
	Key   string      `json:"key"`
	Type  string      `json:"type"`
	Value interface{} `json:"value"`
}

// jaegerLog represents a log entry in a span
type jaegerLog struct {
	Timestamp int64            `json:"timestamp"` // microseconds since epoch
	Fields    []jaegerKeyValue `json:"fields"`
}

// jaegerProcess represents a process in Jaeger's format
type jaegerProcess struct {
	ServiceName string           `json:"serviceName"`
	Tags        []jaegerKeyValue `json:"tags,omitempty"`
}

// Query executes a traces query against Jaeger using the search API
func (j *JaegerQuerier) Query(ctx context.Context, query *TracesQuery) (*TracesResult, error) {
	// Build the query URL
	baseURL := j.config.Address
	if baseURL == "" {
		return nil, fmt.Errorf("jaeger address not configured")
	}

	// Use the traces search endpoint
	endpoint := fmt.Sprintf("%s/api/traces", baseURL)

	// Build query parameters
	params := url.Values{}

	if query.Service != "" {
		params.Set("service", query.Service)
	}

	if query.Operation != "" {
		params.Set("operation", query.Operation)
	}

	// Add tags as JSON
	if len(query.Tags) > 0 {
		tagsJSON, err := json.Marshal(query.Tags)
		if err == nil {
			params.Set("tags", string(tagsJSON))
		}
	}

	// Time range (Jaeger expects microseconds)
	if query.Range != nil {
		params.Set("start", strconv.FormatInt(query.Range.Start.UnixMicro(), 10))
		params.Set("end", strconv.FormatInt(query.Range.End.UnixMicro(), 10))
	}

	// Duration filters (Jaeger expects duration strings like "100ms" or microseconds)
	if query.MinDuration > 0 {
		params.Set("minDuration", strconv.FormatInt(query.MinDuration.Microseconds(), 10)+"us")
	}
	if query.MaxDuration > 0 {
		params.Set("maxDuration", strconv.FormatInt(query.MaxDuration.Microseconds(), 10)+"us")
	}

	// Limit
	limit := query.Limit
	if limit == 0 {
		limit = 20
	}
	params.Set("limit", strconv.Itoa(limit))

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Accept", "application/json")

	// Add authentication if configured
	if j.config.Username != "" && j.config.Password != "" {
		req.SetBasicAuth(j.config.Username, j.config.Password)
	}

	// Execute request
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jaeger request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jaeger returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var jaegerResp jaegerTracesResponse
	if err := json.Unmarshal(body, &jaegerResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for errors
	if len(jaegerResp.Errors) > 0 {
		return nil, fmt.Errorf("jaeger query failed: %s", jaegerResp.Errors[0].Msg)
	}

	// Convert to our format
	traces := make([]TraceResult, len(jaegerResp.Data))
	for i, jt := range jaegerResp.Data {
		traces[i] = convertJaegerTrace(&jt)
	}

	return &TracesResult{
		Traces: traces,
		Total:  jaegerResp.Total,
	}, nil
}

// GetTrace retrieves a specific trace from Jaeger
func (j *JaegerQuerier) GetTrace(ctx context.Context, traceID string) (*TraceResult, error) {
	// Build the query URL
	baseURL := j.config.Address
	if baseURL == "" {
		return nil, fmt.Errorf("jaeger address not configured")
	}

	endpoint := fmt.Sprintf("%s/api/traces/%s", baseURL, url.PathEscape(traceID))

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if j.config.Username != "" && j.config.Password != "" {
		req.SetBasicAuth(j.config.Username, j.config.Password)
	}

	// Execute request
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jaeger request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jaeger returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response (single trace response)
	var jaegerResp jaegerTracesResponse
	if err := json.Unmarshal(body, &jaegerResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	if len(jaegerResp.Data) == 0 {
		return nil, fmt.Errorf("trace not found: %s", traceID)
	}

	trace := convertJaegerTrace(&jaegerResp.Data[0])
	return &trace, nil
}

// GetServices retrieves available service names from Jaeger
func (j *JaegerQuerier) GetServices(ctx context.Context) ([]string, error) {
	endpoint := fmt.Sprintf("%s/api/services", j.config.Address)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if j.config.Username != "" && j.config.Password != "" {
		req.SetBasicAuth(j.config.Username, j.config.Password)
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jaeger request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jaeger returned status %d: %s", resp.StatusCode, string(body))
	}

	var servicesResp struct {
		Data   []string `json:"data"`
		Total  int      `json:"total"`
		Errors []struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(body, &servicesResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return servicesResp.Data, nil
}

// GetOperations retrieves available operations for a service from Jaeger
func (j *JaegerQuerier) GetOperations(ctx context.Context, service string) ([]string, error) {
	endpoint := fmt.Sprintf("%s/api/services/%s/operations", j.config.Address, url.PathEscape(service))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if j.config.Username != "" && j.config.Password != "" {
		req.SetBasicAuth(j.config.Username, j.config.Password)
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jaeger request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jaeger returned status %d: %s", resp.StatusCode, string(body))
	}

	var opsResp struct {
		Data   []string `json:"data"`
		Total  int      `json:"total"`
		Errors []struct {
			Code int    `json:"code"`
			Msg  string `json:"msg"`
		} `json:"errors,omitempty"`
	}
	if err := json.Unmarshal(body, &opsResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return opsResp.Data, nil
}

// convertJaegerTrace converts a Jaeger trace to our format
func convertJaegerTrace(jt *jaegerTrace) TraceResult {
	// Convert spans
	spans := make([]Span, len(jt.Spans))
	for i := range jt.Spans {
		js := &jt.Spans[i]
		spans[i] = Span{
			TraceID:       js.TraceID,
			SpanID:        js.SpanID,
			OperationName: js.OperationName,
			References:    convertJaegerRefs(js.References),
			StartTime:     time.UnixMicro(js.StartTime),
			Duration:      time.Duration(js.Duration) * time.Microsecond,
			Tags:          convertJaegerTags(js.Tags),
			Logs:          convertJaegerLogs(js.Logs),
			ProcessID:     js.ProcessID,
		}
	}

	// Convert processes
	processes := make(map[string]Process, len(jt.Processes))
	for id, jp := range jt.Processes {
		processes[id] = Process{
			ServiceName: jp.ServiceName,
			Tags:        convertJaegerTags(jp.Tags),
		}
	}

	return TraceResult{
		TraceID:   jt.TraceID,
		Spans:     spans,
		Processes: processes,
		Warnings:  jt.Warnings,
	}
}

// convertJaegerRefs converts Jaeger span references to our format
func convertJaegerRefs(refs []jaegerSpanRef) []SpanRef {
	result := make([]SpanRef, len(refs))
	for i, ref := range refs {
		result[i] = SpanRef(ref)
	}
	return result
}

// convertJaegerTags converts Jaeger key-value pairs to our format
func convertJaegerTags(tags []jaegerKeyValue) map[string]interface{} {
	result := make(map[string]interface{}, len(tags))
	for _, tag := range tags {
		result[tag.Key] = tag.Value
	}
	return result
}

// convertJaegerLogs converts Jaeger logs to our format
func convertJaegerLogs(logs []jaegerLog) []SpanLog {
	result := make([]SpanLog, len(logs))
	for i, log := range logs {
		result[i] = SpanLog{
			Timestamp: time.UnixMicro(log.Timestamp),
			Fields:    convertJaegerTags(log.Fields),
		}
	}
	return result
}
