package query

import (
	"time"
)

// QueryType represents the type of query
type QueryType string

const (
	QueryTypeMetrics QueryType = "metrics"
	QueryTypeLogs    QueryType = "logs"
	QueryTypeTraces  QueryType = "traces"
)

// TimeRange represents a time range for queries
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// MetricsQuery represents a metrics query request
type MetricsQuery struct {
	// Query is the PromQL query expression
	Query string `json:"query"`

	// Time is the evaluation timestamp (for instant queries)
	Time *time.Time `json:"time,omitempty"`

	// Range is the time range (for range queries)
	Range *TimeRange `json:"range,omitempty"`

	// Step is the query resolution step width (for range queries)
	Step time.Duration `json:"step,omitempty"`

	// Timeout is the evaluation timeout
	Timeout time.Duration `json:"timeout,omitempty"`
}

// MetricsResult represents a metrics query result
type MetricsResult struct {
	// ResultType is the type of result (matrix, vector, scalar, string)
	ResultType string `json:"result_type"`

	// Result is the actual result data
	Result interface{} `json:"result"`

	// Warnings are any warnings from the query
	Warnings []string `json:"warnings,omitempty"`
}

// LogsQuery represents a logs query request
type LogsQuery struct {
	// Query is the LogQL query expression
	Query string `json:"query"`

	// Range is the time range
	Range TimeRange `json:"range"`

	// Limit is the maximum number of entries to return
	Limit int `json:"limit,omitempty"`

	// Direction is the sort direction (forward or backward)
	Direction string `json:"direction,omitempty"`

	// Start is the cursor for pagination
	Start string `json:"start,omitempty"`
}

// LogEntry represents a single log entry
type LogEntry struct {
	// Timestamp is when the log was generated
	Timestamp time.Time `json:"timestamp"`

	// Line is the log message
	Line string `json:"line"`

	// Labels are the log labels
	Labels map[string]string `json:"labels,omitempty"`
}

// LogsResult represents a logs query result
type LogsResult struct {
	// Entries are the log entries
	Entries []LogEntry `json:"entries"`

	// Stats contains query statistics
	Stats *LogsStats `json:"stats,omitempty"`
}

// LogsStats contains statistics about a logs query
type LogsStats struct {
	// Summary contains summary statistics
	Summary LogsSummary `json:"summary"`
}

// LogsSummary contains summary statistics
type LogsSummary struct {
	// BytesProcessed is the number of bytes processed
	BytesProcessed int64 `json:"bytes_processed"`

	// LinesProcessed is the number of lines processed
	LinesProcessed int64 `json:"lines_processed"`

	// TotalBytesProcessed is the total bytes in the time range
	TotalBytesProcessed int64 `json:"total_bytes_processed"`

	// ExecTime is the execution time
	ExecTime float64 `json:"exec_time"`
}

// TracesQuery represents a traces query request
type TracesQuery struct {
	// Service is the service name to filter by
	Service string `json:"service,omitempty"`

	// Operation is the operation name to filter by
	Operation string `json:"operation,omitempty"`

	// Tags are tags to filter by
	Tags map[string]string `json:"tags,omitempty"`

	// Range is the time range
	Range *TimeRange `json:"range,omitempty"`

	// MinDuration is the minimum trace duration
	MinDuration time.Duration `json:"min_duration,omitempty"`

	// MaxDuration is the maximum trace duration
	MaxDuration time.Duration `json:"max_duration,omitempty"`

	// Limit is the maximum number of traces to return
	Limit int `json:"limit,omitempty"`
}

// TraceResult represents a single trace
type TraceResult struct {
	// TraceID is the unique trace identifier
	TraceID string `json:"trace_id"`

	// Spans are the spans in this trace
	Spans []Span `json:"spans"`

	// Processes are the processes involved in this trace
	Processes map[string]Process `json:"processes,omitempty"`

	// Warnings are any warnings
	Warnings []string `json:"warnings,omitempty"`
}

// Span represents a single span in a trace
type Span struct {
	// TraceID is the trace this span belongs to
	TraceID string `json:"trace_id"`

	// SpanID is the unique span identifier
	SpanID string `json:"span_id"`

	// OperationName is the name of the operation
	OperationName string `json:"operation_name"`

	// References are references to other spans
	References []SpanRef `json:"references,omitempty"`

	// StartTime is when the span started
	StartTime time.Time `json:"start_time"`

	// Duration is how long the span took
	Duration time.Duration `json:"duration"`

	// Tags are span tags
	Tags map[string]interface{} `json:"tags,omitempty"`

	// Logs are span logs
	Logs []SpanLog `json:"logs,omitempty"`

	// ProcessID is the ID of the process that created this span
	ProcessID string `json:"process_id,omitempty"`
}

// SpanRef represents a reference to another span
type SpanRef struct {
	// RefType is the reference type (CHILD_OF, FOLLOWS_FROM)
	RefType string `json:"ref_type"`

	// TraceID is the referenced trace ID
	TraceID string `json:"trace_id"`

	// SpanID is the referenced span ID
	SpanID string `json:"span_id"`
}

// SpanLog represents a log entry in a span
type SpanLog struct {
	// Timestamp is when the log was created
	Timestamp time.Time `json:"timestamp"`

	// Fields are the log fields
	Fields map[string]interface{} `json:"fields"`
}

// Process represents a process in a trace
type Process struct {
	// ServiceName is the name of the service
	ServiceName string `json:"service_name"`

	// Tags are process-level tags
	Tags map[string]interface{} `json:"tags,omitempty"`
}

// TracesResult represents a traces query result
type TracesResult struct {
	// Traces are the matching traces
	Traces []TraceResult `json:"traces"`

	// Total is the total number of matching traces
	Total int `json:"total,omitempty"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	// Message is the error message
	Message string `json:"error"`

	// Code is the error code
	Code string `json:"code,omitempty"`
}
