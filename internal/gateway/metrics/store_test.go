package metrics

import (
	"testing"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestMetricsStore_Store(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewMetricsStore(config)

	// Create test metric family
	name := "test_metric"
	help := "A test metric"
	metricType := dto.MetricType_GAUGE
	value := 42.0

	family := &dto.MetricFamily{
		Name: &name,
		Help: &help,
		Type: &metricType,
		Metric: []*dto.Metric{
			{
				Gauge: &dto.Gauge{Value: &value},
			},
		},
	}

	// Store metrics
	err := store.Store("agent-1", map[string]string{"role": "web"}, []*dto.MetricFamily{family})
	if err != nil {
		t.Fatalf("Store() error = %v", err)
	}

	// Verify agent was added
	if store.AgentCount() != 1 {
		t.Errorf("AgentCount() = %d, want 1", store.AgentCount())
	}

	// Verify series count
	if store.SeriesCount() != 1 {
		t.Errorf("SeriesCount() = %d, want 1", store.SeriesCount())
	}

	// Get agent metrics
	agent, exists := store.Get("agent-1")
	if !exists {
		t.Fatal("Get() returned false, want true")
	}

	if agent.Labels["role"] != "web" {
		t.Errorf("agent.Labels[role] = %s, want web", agent.Labels["role"])
	}

	if len(agent.Families) != 1 {
		t.Errorf("len(agent.Families) = %d, want 1", len(agent.Families))
	}
}

func TestMetricsStore_GetAllFamilies(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewMetricsStore(config)

	// Add metrics for two agents
	name1 := "metric_a"
	name2 := "metric_b"
	metricType := dto.MetricType_COUNTER
	value1 := 10.0
	value2 := 20.0

	store.Store("agent-1", nil, []*dto.MetricFamily{
		{
			Name: &name1,
			Type: &metricType,
			Metric: []*dto.Metric{
				{Counter: &dto.Counter{Value: &value1}},
			},
		},
	})

	store.Store("agent-2", nil, []*dto.MetricFamily{
		{
			Name: &name2,
			Type: &metricType,
			Metric: []*dto.Metric{
				{Counter: &dto.Counter{Value: &value2}},
			},
		},
	})

	// Get all families
	families := store.GetAllFamilies()
	if len(families) != 2 {
		t.Errorf("len(families) = %d, want 2", len(families))
	}
}

func TestMetricsStore_Remove(t *testing.T) {
	config := DefaultStoreConfig()
	store := NewMetricsStore(config)

	name := "test_metric"
	metricType := dto.MetricType_GAUGE
	value := 1.0

	store.Store("agent-1", nil, []*dto.MetricFamily{
		{
			Name: &name,
			Type: &metricType,
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: &value}},
			},
		},
	})

	if store.AgentCount() != 1 {
		t.Fatalf("AgentCount() = %d, want 1", store.AgentCount())
	}

	store.Remove("agent-1")

	if store.AgentCount() != 0 {
		t.Errorf("AgentCount() after Remove() = %d, want 0", store.AgentCount())
	}
}

func TestMetricsStore_RemoveStale(t *testing.T) {
	config := StoreConfig{
		MaxAge:    100 * time.Millisecond,
		MaxSeries: 1000,
	}
	store := NewMetricsStore(config)

	name := "test_metric"
	metricType := dto.MetricType_GAUGE
	value := 1.0

	store.Store("agent-1", nil, []*dto.MetricFamily{
		{
			Name: &name,
			Type: &metricType,
			Metric: []*dto.Metric{
				{Gauge: &dto.Gauge{Value: &value}},
			},
		},
	})

	// Wait for staleness
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 150*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("staleness wait did not elapse: %v", err)
	}

	removed := store.RemoveStale()
	if len(removed) != 1 {
		t.Errorf("RemoveStale() removed %d agents, want 1", len(removed))
	}

	if store.AgentCount() != 0 {
		t.Errorf("AgentCount() after RemoveStale() = %d, want 0", store.AgentCount())
	}
}

func TestMetricsStore_Cardinality(t *testing.T) {
	config := StoreConfig{
		MaxAge:                   1 * time.Hour,
		MaxSeries:                5,
		MaxLabelsPerSeries:       2,
		DropHighCardinality:      true,
		HighCardinalityThreshold: 3,
	}
	store := NewMetricsStore(config)

	name := "high_cardinality_metric"
	metricType := dto.MetricType_GAUGE
	value := 1.0

	// Create high cardinality metric (4 series > threshold of 3)
	metrics := make([]*dto.Metric, 4)
	for i := 0; i < 4; i++ {
		v := value + float64(i)
		metrics[i] = &dto.Metric{
			Gauge: &dto.Gauge{Value: &v},
		}
	}

	store.Store("agent-1", nil, []*dto.MetricFamily{
		{
			Name:   &name,
			Type:   &metricType,
			Metric: metrics,
		},
	})

	// High cardinality should be dropped
	stats := store.Stats()
	if stats.DroppedSeries != 4 {
		t.Errorf("DroppedSeries = %d, want 4", stats.DroppedSeries)
	}
}

func TestDefaultStoreConfig(t *testing.T) {
	config := DefaultStoreConfig()

	if config.MaxAge != 60*time.Second {
		t.Errorf("MaxAge = %v, want 60s", config.MaxAge)
	}
	if config.MaxSeries != 100000 {
		t.Errorf("MaxSeries = %d, want 100000", config.MaxSeries)
	}
	if config.MaxLabelsPerSeries != 20 {
		t.Errorf("MaxLabelsPerSeries = %d, want 20", config.MaxLabelsPerSeries)
	}
}
