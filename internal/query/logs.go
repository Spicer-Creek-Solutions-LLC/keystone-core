// Package query provides unified telemetry querying for Keystone Core.
// It implements:
//   - Logs querying with filtering, time ranges, and search
//   - Metrics querying via Prometheus-compatible API
//   - Traces querying via Jaeger-compatible API
//   - In-memory implementations for testing
//   - Backend integrations for Loki, Prometheus, and Jaeger
//
// The query package enables the control plane and CLI tools to retrieve
// observability data from various backends through a consistent interface.
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

// InMemoryLogsQuerier is a simple in-memory logs querier for testing
// In production, this would be replaced with a Loki client or similar
type InMemoryLogsQuerier struct {
	entries []LogEntry
	mu      sync.RWMutex
}

// NewInMemoryLogsQuerier creates a new in-memory logs querier
func NewInMemoryLogsQuerier() *InMemoryLogsQuerier {
	return &InMemoryLogsQuerier{
		entries: make([]LogEntry, 0),
	}
}

// AddEntry adds a log entry (for testing)
func (l *InMemoryLogsQuerier) AddEntry(entry LogEntry) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.entries = append(l.entries, entry)
}

// Query executes a logs query
func (l *InMemoryLogsQuerier) Query(ctx context.Context, query *LogsQuery) (*LogsResult, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	startTime := time.Now()

	// Determine effective start time, using pagination cursor if provided
	effectiveStart := query.Range.Start
	if query.Start != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, query.Start); err == nil {
			effectiveStart = parsed
		}
	}

	// Filter entries by time range
	filtered := make([]LogEntry, 0)
	for _, entry := range l.entries {
		if entry.Timestamp.After(effectiveStart) && entry.Timestamp.Before(query.Range.End) {
			// Simple query matching - check if query string is in the line
			if query.Query == "" || containsQuery(entry, query.Query) {
				filtered = append(filtered, entry)
			}
		}
	}

	// Sort by direction
	direction := query.Direction
	if direction == "" {
		direction = "backward" // Default to most recent first
	}

	if direction == "backward" {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Timestamp.After(filtered[j].Timestamp)
		})
	} else {
		sort.Slice(filtered, func(i, j int) bool {
			return filtered[i].Timestamp.Before(filtered[j].Timestamp)
		})
	}

	// Apply limit
	limit := query.Limit
	if limit == 0 {
		limit = 100 // Default limit
	}

	if len(filtered) > limit {
		filtered = filtered[:limit]
	}

	execTime := time.Since(startTime).Seconds()

	return &LogsResult{
		Entries: filtered,
		Stats: &LogsStats{
			Summary: LogsSummary{
				BytesProcessed:      calculateBytes(filtered),
				LinesProcessed:      int64(len(filtered)),
				TotalBytesProcessed: calculateBytes(l.entries),
				ExecTime:            execTime,
			},
		},
	}, nil
}

// containsQuery checks if a log entry matches the query
// This is a simple implementation - a real implementation would use LogQL
func containsQuery(entry LogEntry, query string) bool {
	// Check if query is in the line
	if entry.Line != "" && contains(entry.Line, query) {
		return true
	}

	// Check labels
	for k, v := range entry.Labels {
		if contains(k, query) || contains(v, query) {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	// Simple case-insensitive substring match
	return findSubstring(toLowerCase(s), toLowerCase(substr))
}

func findSubstring(s, substr string) bool {
	if substr == "" {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func toLowerCase(s string) string {
	result := make([]rune, len(s))
	for i, r := range s {
		if r >= 'A' && r <= 'Z' {
			result[i] = r + 32
		} else {
			result[i] = r
		}
	}
	return string(result)
}

func calculateBytes(entries []LogEntry) int64 {
	var total int64
	for _, entry := range entries {
		total += int64(len(entry.Line))
	}
	return total
}

// LokiConfig holds configuration for the Loki querier
type LokiConfig struct {
	// Address is the Loki server address (e.g., "http://loki:3100")
	Address string

	// Username for basic auth (optional)
	Username string

	// Password for basic auth (optional)
	Password string

	// TenantID for multi-tenant Loki (optional)
	TenantID string

	// TLSConfig for secure connections (optional)
	TLSConfig *tls.Config

	// Timeout for HTTP requests
	Timeout time.Duration
}

// LokiQuerier queries logs from Grafana Loki
type LokiQuerier struct {
	config *LokiConfig
	client *http.Client
}

// NewLokiQuerier creates a new Loki logs querier
func NewLokiQuerier(address string) *LokiQuerier {
	return NewLokiQuerierWithConfig(&LokiConfig{
		Address: address,
		Timeout: 30 * time.Second,
	})
}

// NewLokiQuerierWithConfig creates a new Loki logs querier with full configuration
func NewLokiQuerierWithConfig(config *LokiConfig) *LokiQuerier {
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}

	transport := &http.Transport{}
	if config.TLSConfig != nil {
		transport.TLSClientConfig = config.TLSConfig
	}

	return &LokiQuerier{
		config: config,
		client: &http.Client{
			Timeout:   config.Timeout,
			Transport: transport,
		},
	}
}

// lokiQueryRangeResponse represents the Loki query_range API response
type lokiQueryRangeResponse struct {
	Status string `json:"status"`
	Data   struct {
		ResultType string `json:"resultType"`
		Result     []struct {
			Stream map[string]string `json:"stream"`
			Values [][]string        `json:"values"` // [timestamp_ns, line]
		} `json:"result"`
		Stats struct {
			Summary struct {
				BytesProcessedPerSecond int64   `json:"bytesProcessedPerSecond"`
				LinesProcessedPerSecond int64   `json:"linesProcessedPerSecond"`
				TotalBytesProcessed     int64   `json:"totalBytesProcessed"`
				TotalLinesProcessed     int64   `json:"totalLinesProcessed"`
				ExecTime                float64 `json:"execTime"`
			} `json:"summary"`
		} `json:"stats"`
	} `json:"data"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"errorType,omitempty"`
}

// Query executes a logs query against Loki using the query_range API
func (l *LokiQuerier) Query(ctx context.Context, query *LogsQuery) (*LogsResult, error) {
	// Build the query URL
	baseURL := l.config.Address
	if baseURL == "" {
		return nil, fmt.Errorf("loki address not configured")
	}

	// Use query_range endpoint for time-based queries
	endpoint := fmt.Sprintf("%s/loki/api/v1/query_range", baseURL)

	// Build query parameters
	params := url.Values{}
	params.Set("query", query.Query)

	// Use pagination cursor as start if provided, otherwise use range start
	if query.Start != "" {
		params.Set("start", query.Start)
	} else {
		params.Set("start", strconv.FormatInt(query.Range.Start.UnixNano(), 10))
	}
	params.Set("end", strconv.FormatInt(query.Range.End.UnixNano(), 10))

	if query.Limit > 0 {
		params.Set("limit", strconv.Itoa(query.Limit))
	}

	if query.Direction != "" {
		params.Set("direction", query.Direction)
	}

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add headers
	req.Header.Set("Accept", "application/json")

	// Add authentication if configured
	if l.config.Username != "" && l.config.Password != "" {
		req.SetBasicAuth(l.config.Username, l.config.Password)
	}

	// Add tenant ID if configured (for multi-tenant Loki)
	if l.config.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", l.config.TenantID)
	}

	// Execute request
	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loki request failed: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for HTTP errors
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var lokiResp lokiQueryRangeResponse
	if err := json.Unmarshal(body, &lokiResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Check for Loki errors
	if lokiResp.Status != "success" {
		return nil, fmt.Errorf("loki query failed: %s (%s)", lokiResp.Error, lokiResp.ErrorType)
	}

	// Convert to our format
	entries := make([]LogEntry, 0)
	for _, stream := range lokiResp.Data.Result {
		for _, value := range stream.Values {
			if len(value) < 2 {
				continue
			}

			// Parse timestamp (nanoseconds since epoch)
			tsNano, err := strconv.ParseInt(value[0], 10, 64)
			if err != nil {
				continue
			}
			ts := time.Unix(0, tsNano)

			entries = append(entries, LogEntry{
				Timestamp: ts,
				Line:      value[1],
				Labels:    stream.Stream,
			})
		}
	}

	// Sort entries based on direction
	direction := query.Direction
	if direction == "" {
		direction = "backward"
	}

	if direction == "backward" {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Timestamp.After(entries[j].Timestamp)
		})
	} else {
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Timestamp.Before(entries[j].Timestamp)
		})
	}

	return &LogsResult{
		Entries: entries,
		Stats: &LogsStats{
			Summary: LogsSummary{
				BytesProcessed:      lokiResp.Data.Stats.Summary.TotalBytesProcessed,
				LinesProcessed:      lokiResp.Data.Stats.Summary.TotalLinesProcessed,
				TotalBytesProcessed: lokiResp.Data.Stats.Summary.TotalBytesProcessed,
				ExecTime:            lokiResp.Data.Stats.Summary.ExecTime,
			},
		},
	}, nil
}

// Labels retrieves available label names from Loki
func (l *LokiQuerier) Labels(ctx context.Context, start, end time.Time) ([]string, error) {
	endpoint := fmt.Sprintf("%s/loki/api/v1/labels", l.config.Address)

	params := url.Values{}
	params.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	params.Set("end", strconv.FormatInt(end.UnixNano(), 10))

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if l.config.Username != "" && l.config.Password != "" {
		req.SetBasicAuth(l.config.Username, l.config.Password)
	}
	if l.config.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", l.config.TenantID)
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loki request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki returned status %d: %s", resp.StatusCode, string(body))
	}

	var labelsResp struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if err := json.Unmarshal(body, &labelsResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return labelsResp.Data, nil
}

// LabelValues retrieves values for a specific label from Loki
func (l *LokiQuerier) LabelValues(ctx context.Context, label string, start, end time.Time) ([]string, error) {
	endpoint := fmt.Sprintf("%s/loki/api/v1/label/%s/values", l.config.Address, url.PathEscape(label))

	params := url.Values{}
	params.Set("start", strconv.FormatInt(start.UnixNano(), 10))
	params.Set("end", strconv.FormatInt(end.UnixNano(), 10))

	fullURL := fmt.Sprintf("%s?%s", endpoint, params.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", "application/json")
	if l.config.Username != "" && l.config.Password != "" {
		req.SetBasicAuth(l.config.Username, l.config.Password)
	}
	if l.config.TenantID != "" {
		req.Header.Set("X-Scope-OrgID", l.config.TenantID)
	}

	resp, err := l.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("loki request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("loki returned status %d: %s", resp.StatusCode, string(body))
	}

	var valuesResp struct {
		Status string   `json:"status"`
		Data   []string `json:"data"`
	}
	if err := json.Unmarshal(body, &valuesResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return valuesResp.Data, nil
}
