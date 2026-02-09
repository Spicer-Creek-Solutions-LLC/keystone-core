package query

import (
	"context"
)

// MetricsQuerier defines the interface for querying metrics
type MetricsQuerier interface {
	// Query executes an instant query
	Query(ctx context.Context, query *MetricsQuery) (*MetricsResult, error)

	// QueryRange executes a range query
	QueryRange(ctx context.Context, query *MetricsQuery) (*MetricsResult, error)
}

// LogsQuerier defines the interface for querying logs
type LogsQuerier interface {
	// Query executes a logs query
	Query(ctx context.Context, query *LogsQuery) (*LogsResult, error)
}

// TracesQuerier defines the interface for querying traces
type TracesQuerier interface {
	// Query executes a traces query
	Query(ctx context.Context, query *TracesQuery) (*TracesResult, error)

	// GetTrace retrieves a specific trace by ID
	GetTrace(ctx context.Context, traceID string) (*TraceResult, error)
}

// API provides a unified query API for all observability data
type API struct {
	metrics MetricsQuerier
	logs    LogsQuerier
	traces  TracesQuerier
}

// NewAPI creates a new query API
func NewAPI(metrics MetricsQuerier, logs LogsQuerier, traces TracesQuerier) *API {
	return &API{
		metrics: metrics,
		logs:    logs,
		traces:  traces,
	}
}

// QueryMetrics executes a metrics query
func (a *API) QueryMetrics(ctx context.Context, query *MetricsQuery) (*MetricsResult, error) {
	if a.metrics == nil {
		return nil, &ResponseError{Message: "metrics querier not configured", Code: "METRICS_UNAVAILABLE"}
	}

	if query.Range != nil {
		return a.metrics.QueryRange(ctx, query)
	}
	return a.metrics.Query(ctx, query)
}

// QueryLogs executes a logs query
func (a *API) QueryLogs(ctx context.Context, query *LogsQuery) (*LogsResult, error) {
	if a.logs == nil {
		return nil, &ResponseError{Message: "logs querier not configured", Code: "LOGS_UNAVAILABLE"}
	}

	return a.logs.Query(ctx, query)
}

// QueryTraces executes a traces query
func (a *API) QueryTraces(ctx context.Context, query *TracesQuery) (*TracesResult, error) {
	if a.traces == nil {
		return nil, &ResponseError{Message: "traces querier not configured", Code: "TRACES_UNAVAILABLE"}
	}

	return a.traces.Query(ctx, query)
}

// GetTrace retrieves a specific trace by ID
func (a *API) GetTrace(ctx context.Context, traceID string) (*TraceResult, error) {
	if a.traces == nil {
		return nil, &ResponseError{Message: "traces querier not configured", Code: "TRACES_UNAVAILABLE"}
	}

	return a.traces.GetTrace(ctx, traceID)
}

// Error implements the error interface for ResponseError
func (e *ResponseError) Error() string {
	return e.Message
}
