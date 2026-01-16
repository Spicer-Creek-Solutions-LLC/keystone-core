package upgrade

import (
	"testing"

	"github.com/shawnbutts/keystone-core/pkg/query"
)

func TestExtractMetricValue(t *testing.T) {
	t.Run("scalar", func(t *testing.T) {
		value, err := extractMetricValue(&query.MetricsResult{
			ResultType: "scalar",
			Result: map[string]interface{}{
				"value": 42.0,
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != 42.0 {
			t.Fatalf("value = %v, want 42", value)
		}
	})

	t.Run("vector average", func(t *testing.T) {
		value, err := extractMetricValue(&query.MetricsResult{
			ResultType: "vector",
			Result: []map[string]interface{}{
				{"value": 10.0},
				{"value": 20.0},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != 15.0 {
			t.Fatalf("value = %v, want 15", value)
		}
	})

	t.Run("vector empty", func(t *testing.T) {
		_, err := extractMetricValue(&query.MetricsResult{
			ResultType: "vector",
			Result:     []map[string]interface{}{},
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("unsupported result type", func(t *testing.T) {
		_, err := extractMetricValue(&query.MetricsResult{
			ResultType: "matrix",
			Result:     []map[string]interface{}{},
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
