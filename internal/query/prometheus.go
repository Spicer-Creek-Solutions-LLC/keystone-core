package query

import (
	"context"
	"fmt"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/common/model"
)

// PrometheusQuerier queries metrics from Prometheus
type PrometheusQuerier struct {
	client v1.API
}

// NewPrometheusQuerier creates a new Prometheus metrics querier
func NewPrometheusQuerier(address string) (*PrometheusQuerier, error) {
	client, err := api.NewClient(api.Config{
		Address: address,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Prometheus client: %w", err)
	}

	return &PrometheusQuerier{
		client: v1.NewAPI(client),
	}, nil
}

// Query executes an instant query
func (p *PrometheusQuerier) Query(ctx context.Context, query *MetricsQuery) (*MetricsResult, error) {
	if query.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	queryTime := time.Now()
	if query.Time != nil {
		queryTime = *query.Time
	}

	if query.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, query.Timeout)
		defer cancel()
	}

	result, warnings, err := p.client.Query(ctx, query.Query, queryTime)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}

	return &MetricsResult{
		ResultType: result.Type().String(),
		Result:     convertPrometheusValue(result),
		Warnings:   warnings,
	}, nil
}

// QueryRange executes a range query
func (p *PrometheusQuerier) QueryRange(ctx context.Context, query *MetricsQuery) (*MetricsResult, error) {
	if query.Query == "" {
		return nil, fmt.Errorf("query is required")
	}

	if query.Range == nil {
		return nil, fmt.Errorf("range is required for range queries")
	}

	step := query.Step
	if step == 0 {
		step = 15 * time.Second // Default step
	}

	if query.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, query.Timeout)
		defer cancel()
	}

	r := v1.Range{
		Start: query.Range.Start,
		End:   query.Range.End,
		Step:  step,
	}

	result, warnings, err := p.client.QueryRange(ctx, query.Query, r)
	if err != nil {
		return nil, fmt.Errorf("range query failed: %w", err)
	}

	return &MetricsResult{
		ResultType: result.Type().String(),
		Result:     convertPrometheusValue(result),
		Warnings:   warnings,
	}, nil
}

// convertPrometheusValue converts Prometheus value to our format
func convertPrometheusValue(value model.Value) interface{} {
	switch v := value.(type) {
	case model.Matrix:
		return convertMatrix(v)
	case model.Vector:
		return convertVector(v)
	case *model.Scalar:
		return convertScalar(v)
	case *model.String:
		return map[string]interface{}{
			"value":     v.Value,
			"timestamp": v.Timestamp.Time(),
		}
	default:
		return value
	}
}

func convertMatrix(matrix model.Matrix) []map[string]interface{} {
	result := make([]map[string]interface{}, len(matrix))
	for i, stream := range matrix {
		values := make([]map[string]interface{}, len(stream.Values))
		for j, pair := range stream.Values {
			values[j] = map[string]interface{}{
				"timestamp": pair.Timestamp.Time(),
				"value":     float64(pair.Value),
			}
		}

		result[i] = map[string]interface{}{
			"metric": convertLabels(stream.Metric),
			"values": values,
		}
	}
	return result
}

func convertVector(vector model.Vector) []map[string]interface{} {
	result := make([]map[string]interface{}, len(vector))
	for i, sample := range vector {
		result[i] = map[string]interface{}{
			"metric":    convertLabels(sample.Metric),
			"value":     float64(sample.Value),
			"timestamp": sample.Timestamp.Time(),
		}
	}
	return result
}

func convertScalar(scalar *model.Scalar) map[string]interface{} {
	return map[string]interface{}{
		"value":     float64(scalar.Value),
		"timestamp": scalar.Timestamp.Time(),
	}
}

func convertLabels(metric model.Metric) map[string]string {
	labels := make(map[string]string, len(metric))
	for k, v := range metric {
		labels[string(k)] = string(v)
	}
	return labels
}
