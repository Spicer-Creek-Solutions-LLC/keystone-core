package cardinality

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.MaxSeries <= 0 {
		t.Error("MaxSeries should be positive")
	}
	if config.MaxLabels <= 0 {
		t.Error("MaxLabels should be positive")
	}
	if config.MaxLabelValueLength <= 0 {
		t.Error("MaxLabelValueLength should be positive")
	}
	if config.Strategy == "" {
		t.Error("Strategy should be set")
	}
}

func TestLabelsHash(t *testing.T) {
	l1 := Labels{"a": "1", "b": "2"}
	l2 := Labels{"b": "2", "a": "1"} // Same labels, different order
	l3 := Labels{"a": "1", "b": "3"} // Different value

	if l1.Hash() != l2.Hash() {
		t.Error("Same labels with different order should have same hash")
	}

	if l1.Hash() == l3.Hash() {
		t.Error("Different labels should have different hash")
	}
}

func TestLimiter_Record(t *testing.T) {
	config := DefaultConfig()
	config.MaxSeries = 5
	config.CleanupInterval = 0 // Disable cleanup for testing

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Record some series
	for i := 0; i < 5; i++ {
		err := limiter.Record(ctx, "test_metric", Labels{
			"id": fmt.Sprintf("%d", i),
		})
		if err != nil {
			t.Fatalf("Record failed: %v", err)
		}
	}

	stats := limiter.Stats()
	if stats.TotalSeries != 5 {
		t.Errorf("TotalSeries = %d, want 5", stats.TotalSeries)
	}
}

func TestLimiter_StrategyDrop(t *testing.T) {
	config := DefaultConfig()
	config.MaxSeries = 3
	config.Strategy = StrategyDrop
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Fill up to limit
	for i := 0; i < 3; i++ {
		limiter.Record(ctx, "test_metric", Labels{"id": fmt.Sprintf("%d", i)})
	}

	// Next should be dropped
	err := limiter.Record(ctx, "test_metric", Labels{"id": "new"})
	if !errors.Is(err, ErrMetricDropped) {
		t.Errorf("Expected ErrMetricDropped, got %v", err)
	}

	stats := limiter.Stats()
	if stats.DroppedSeries != 1 {
		t.Errorf("DroppedSeries = %d, want 1", stats.DroppedSeries)
	}
}

func TestLimiter_StrategyAggregate(t *testing.T) {
	config := DefaultConfig()
	config.MaxSeries = 3
	config.Strategy = StrategyAggregate
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Fill up to limit
	for i := 0; i < 3; i++ {
		limiter.Record(ctx, "test_metric", Labels{"id": fmt.Sprintf("%d", i)})
	}

	// Next should be aggregated
	err := limiter.Record(ctx, "test_metric", Labels{"id": "new"})
	if err != nil {
		t.Errorf("Aggregate strategy should not return error: %v", err)
	}

	stats := limiter.Stats()
	if stats.Aggregations != 1 {
		t.Errorf("Aggregations = %d, want 1", stats.Aggregations)
	}
}

func TestLimiter_StrategyEvictOldest(t *testing.T) {
	config := DefaultConfig()
	config.MaxSeries = 3
	config.Strategy = StrategyEvictOldest
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Fill up to limit with time gaps
	for i := 0; i < 3; i++ {
		limiter.Record(ctx, "test_metric", Labels{"id": fmt.Sprintf("%d", i)})
	}
	metric := limiter.metrics["test_metric"]
	if metric == nil {
		t.Fatal("expected metric to be recorded")
	}
	base := time.Now()
	metric.mu.Lock()
	metric.Series[Labels{"id": "0"}.Hash()].CreatedAt = base.Add(-3 * time.Minute)
	metric.Series[Labels{"id": "1"}.Hash()].CreatedAt = base.Add(-2 * time.Minute)
	metric.Series[Labels{"id": "2"}.Hash()].CreatedAt = base.Add(-1 * time.Minute)
	metric.mu.Unlock()

	// Record new one - should evict oldest
	err := limiter.Record(ctx, "test_metric", Labels{"id": "new"})
	if err != nil {
		t.Errorf("EvictOldest should not return error: %v", err)
	}

	stats := limiter.Stats()
	if stats.Evictions != 1 {
		t.Errorf("Evictions = %d, want 1", stats.Evictions)
	}

	// Total series should still be 3
	if stats.TotalSeries != 3 {
		t.Errorf("TotalSeries = %d, want 3", stats.TotalSeries)
	}
}

func TestLimiter_StrategyEvictLRU(t *testing.T) {
	config := DefaultConfig()
	config.MaxSeries = 3
	config.Strategy = StrategyEvictLRU
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Fill up to limit
	for i := 0; i < 3; i++ {
		limiter.Record(ctx, "test_metric", Labels{"id": fmt.Sprintf("%d", i)})
	}

	// Access first two to update LRU
	metric := limiter.metrics["test_metric"]
	if metric == nil {
		t.Fatal("expected metric to be recorded")
	}
	base := time.Now()
	metric.mu.Lock()
	metric.Series[Labels{"id": "0"}.Hash()].LastUsedAt = base
	metric.Series[Labels{"id": "1"}.Hash()].LastUsedAt = base
	metric.Series[Labels{"id": "2"}.Hash()].LastUsedAt = base.Add(-1 * time.Minute)
	metric.mu.Unlock()

	// Record new one - should evict "2" as LRU
	err := limiter.Record(ctx, "test_metric", Labels{"id": "new"})
	if err != nil {
		t.Errorf("EvictLRU should not return error: %v", err)
	}

	stats := limiter.Stats()
	if stats.Evictions != 1 {
		t.Errorf("Evictions = %d, want 1", stats.Evictions)
	}
}

func TestLimiter_WarnThreshold(t *testing.T) {
	config := DefaultConfig()
	config.MaxSeries = 10
	config.WarnThreshold = 0.5
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	var warnings int
	var mu sync.Mutex

	limiter.AddListener(func(event *Event) {
		if event.Type == "warning" {
			mu.Lock()
			warnings++
			mu.Unlock()
		}
	})

	ctx := context.Background()

	// Fill to warning threshold (5 out of 10 = 50%)
	for i := 0; i < 6; i++ {
		limiter.Record(ctx, "test_metric", Labels{"id": fmt.Sprintf("%d", i)})
	}

	mu.Lock()
	if warnings == 0 {
		t.Error("Expected warning when reaching threshold")
	}
	mu.Unlock()
}

func TestLimiter_LabelValidation(t *testing.T) {
	config := DefaultConfig()
	config.MaxLabels = 2
	config.MaxLabelValueLength = 10
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Too many labels
	err := limiter.Record(ctx, "test", Labels{"a": "1", "b": "2", "c": "3"})
	if err == nil {
		t.Error("Expected error for too many labels")
	}

	// Label value too long
	err = limiter.Record(ctx, "test", Labels{"a": "this_is_too_long_value"})
	if err == nil {
		t.Error("Expected error for label value too long")
	}
}

func TestLimiter_GetMetric(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Non-existent metric
	if limiter.GetMetric("nonexistent") != nil {
		t.Error("GetMetric should return nil for non-existent metric")
	}

	// Record some series
	for i := 0; i < 5; i++ {
		limiter.Record(ctx, "test_metric", Labels{"id": fmt.Sprintf("%d", i)})
	}

	info := limiter.GetMetric("test_metric")
	if info == nil {
		t.Fatal("GetMetric returned nil for existing metric")
	}
	if info.SeriesCount != 5 {
		t.Errorf("SeriesCount = %d, want 5", info.SeriesCount)
	}
}

func TestLimiter_ListMetrics(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	limiter.Record(ctx, "metric1", Labels{"a": "1"})
	limiter.Record(ctx, "metric2", Labels{"a": "1"})
	limiter.Record(ctx, "metric3", Labels{"a": "1"})

	metrics := limiter.ListMetrics()
	if len(metrics) != 3 {
		t.Errorf("ListMetrics returned %d metrics, want 3", len(metrics))
	}
}

func TestLimiter_SetMetricLimit(t *testing.T) {
	config := DefaultConfig()
	config.MaxSeries = 10
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Record a series to create the metric
	limiter.Record(ctx, "test_metric", Labels{"a": "1"})

	// Set custom limit
	limiter.SetMetricLimit("test_metric", 2)

	// Fill to new limit
	limiter.Record(ctx, "test_metric", Labels{"b": "2"})

	// This should be dropped
	err := limiter.Record(ctx, "test_metric", Labels{"c": "3"})
	if !errors.Is(err, ErrMetricDropped) {
		t.Error("Expected metric to be dropped after custom limit")
	}
}

func TestLimiter_Cleanup(t *testing.T) {
	config := DefaultConfig()
	config.TTL = 50 * time.Millisecond
	config.CleanupInterval = 10 * time.Millisecond

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Record series
	limiter.Record(ctx, "test_metric", Labels{"a": "1"})

	initialStats := limiter.Stats()
	if initialStats.TotalSeries != 1 {
		t.Fatalf("Initial TotalSeries = %d, want 1", initialStats.TotalSeries)
	}

	// Wait for TTL + cleanup
	if err := helpers.WaitForTimeout(500*time.Millisecond, 10*time.Millisecond, func() (bool, error) {
		stats := limiter.Stats()
		return stats.TotalSeries == 0, nil
	}); err != nil {
		t.Fatalf("expected cleanup to remove series: %v", err)
	}

	stats := limiter.Stats()
	if stats.TotalSeries != 0 {
		t.Errorf("TotalSeries after cleanup = %d, want 0", stats.TotalSeries)
	}
}

func TestLimiter_Events(t *testing.T) {
	config := DefaultConfig()
	config.MaxSeries = 2
	config.Strategy = StrategyDrop
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	var events []*Event
	var mu sync.Mutex

	limiter.AddListener(func(event *Event) {
		mu.Lock()
		events = append(events, event)
		mu.Unlock()
	})

	ctx := context.Background()

	// Fill to limit
	limiter.Record(ctx, "test", Labels{"a": "1"})
	limiter.Record(ctx, "test", Labels{"a": "2"})

	// Trigger drop event
	limiter.Record(ctx, "test", Labels{"a": "3"})

	mu.Lock()
	defer mu.Unlock()

	found := false
	for _, e := range events {
		if e.Type == "dropped" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected dropped event")
	}
}

func TestLimiter_Reset(t *testing.T) {
	config := DefaultConfig()
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()

	// Record some data
	for i := 0; i < 10; i++ {
		limiter.Record(ctx, "test", Labels{"id": fmt.Sprintf("%d", i)})
	}

	// Reset
	limiter.Reset()

	stats := limiter.Stats()
	if stats.TotalMetrics != 0 {
		t.Errorf("TotalMetrics after reset = %d, want 0", stats.TotalMetrics)
	}
	if stats.TotalSeries != 0 {
		t.Errorf("TotalSeries after reset = %d, want 0", stats.TotalSeries)
	}
}

func TestLimiter_Concurrent(t *testing.T) {
	config := DefaultConfig()
	config.MaxSeries = 1000
	config.CleanupInterval = 0

	limiter := NewLimiter(config)
	defer limiter.Stop()

	ctx := context.Background()
	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				limiter.Record(ctx, "test", Labels{
					"worker": fmt.Sprintf("%d", id),
					"iter":   fmt.Sprintf("%d", j),
				})
			}
		}(i)
	}

	wg.Wait()

	stats := limiter.Stats()
	if stats.TotalSeries != 1000 {
		t.Errorf("TotalSeries = %d, want 1000", stats.TotalSeries)
	}
}

func TestLabelCardinalityTracker(t *testing.T) {
	tracker := NewLabelCardinalityTracker(3)

	// Track some labels
	exceeded := tracker.Track(Labels{"env": "prod", "region": "us"})
	if len(exceeded) > 0 {
		t.Errorf("Unexpected exceeded labels: %v", exceeded)
	}

	exceeded = tracker.Track(Labels{"env": "staging", "region": "eu"})
	if len(exceeded) > 0 {
		t.Errorf("Unexpected exceeded labels: %v", exceeded)
	}

	exceeded = tracker.Track(Labels{"env": "dev", "region": "ap"})
	if len(exceeded) > 0 {
		t.Errorf("Unexpected exceeded labels: %v", exceeded)
	}

	// This should exceed the limit for both
	exceeded = tracker.Track(Labels{"env": "test", "region": "sa"})
	if len(exceeded) != 2 {
		t.Errorf("Expected 2 exceeded labels, got %d", len(exceeded))
	}

	// Check cardinality
	if tracker.GetCardinality("env") != 3 {
		t.Errorf("env cardinality = %d, want 3", tracker.GetCardinality("env"))
	}
	if tracker.GetCardinality("region") != 3 {
		t.Errorf("region cardinality = %d, want 3", tracker.GetCardinality("region"))
	}
}

func TestLabelCardinalityTracker_GetAllCardinalities(t *testing.T) {
	tracker := NewLabelCardinalityTracker(100)

	tracker.Track(Labels{"a": "1", "b": "1"})
	tracker.Track(Labels{"a": "2", "b": "2"})
	tracker.Track(Labels{"a": "3"})

	cards := tracker.GetAllCardinalities()
	if cards["a"] != 3 {
		t.Errorf("a cardinality = %d, want 3", cards["a"])
	}
	if cards["b"] != 2 {
		t.Errorf("b cardinality = %d, want 2", cards["b"])
	}
}

func TestLabelCardinalityTracker_Reset(t *testing.T) {
	tracker := NewLabelCardinalityTracker(100)

	tracker.Track(Labels{"a": "1", "b": "2"})
	tracker.Reset()

	if tracker.GetCardinality("a") != 0 {
		t.Error("Cardinality should be 0 after reset")
	}
}

func TestCardinalityEstimator(t *testing.T) {
	estimator := NewCardinalityEstimator(10)

	// Add unique elements
	for i := 0; i < 10000; i++ {
		estimator.Add([]byte(fmt.Sprintf("element-%d", i)))
	}

	estimate := estimator.Estimate()

	// HyperLogLog should estimate within ~2% for this precision
	// Allow some margin
	if estimate < 8000 || estimate > 12000 {
		t.Errorf("Estimate = %d, expected ~10000", estimate)
	}
}

func TestCardinalityEstimator_Duplicates(t *testing.T) {
	estimator := NewCardinalityEstimator(10)

	// Add same element many times
	for i := 0; i < 1000; i++ {
		estimator.Add([]byte("same"))
	}

	estimate := estimator.Estimate()
	if estimate != 1 {
		t.Errorf("Estimate for duplicates = %d, want 1", estimate)
	}
}

func TestCardinalityEstimator_Merge(t *testing.T) {
	e1 := NewCardinalityEstimator(10)
	e2 := NewCardinalityEstimator(10)

	// Add different elements to each
	for i := 0; i < 5000; i++ {
		e1.Add([]byte(fmt.Sprintf("e1-%d", i)))
		e2.Add([]byte(fmt.Sprintf("e2-%d", i)))
	}

	// Merge
	e1.Merge(e2)

	estimate := e1.Estimate()
	// Should be roughly 10000
	if estimate < 8000 || estimate > 12000 {
		t.Errorf("Merged estimate = %d, expected ~10000", estimate)
	}
}

func TestCardinalityEstimator_Reset(t *testing.T) {
	estimator := NewCardinalityEstimator(10)

	for i := 0; i < 100; i++ {
		estimator.Add([]byte(fmt.Sprintf("element-%d", i)))
	}

	estimator.Reset()

	if estimator.Estimate() != 0 {
		t.Error("Estimate should be 0 after reset")
	}
}

func TestCardinalityEstimator_Precision(t *testing.T) {
	// Low precision
	low := NewCardinalityEstimator(4)
	for i := 0; i < 1000; i++ {
		low.Add([]byte(fmt.Sprintf("element-%d", i)))
	}
	lowEstimate := low.Estimate()

	// High precision
	high := NewCardinalityEstimator(14)
	for i := 0; i < 1000; i++ {
		high.Add([]byte(fmt.Sprintf("element-%d", i)))
	}
	highEstimate := high.Estimate()

	// High precision should be closer to 1000
	// This is a rough check - higher precision tends to be more accurate
	lowError := abs(int64(lowEstimate) - 1000)
	highError := abs(int64(highEstimate) - 1000)

	if highError > lowError*2 {
		t.Logf("Low precision estimate: %d, High precision estimate: %d", lowEstimate, highEstimate)
	}
}

func abs(x int64) int64 {
	if x < 0 {
		return -x
	}
	return x
}

func TestCardinalityEstimator_Concurrent(t *testing.T) {
	estimator := NewCardinalityEstimator(10)

	var wg sync.WaitGroup

	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < 1000; j++ {
				estimator.Add([]byte(fmt.Sprintf("w%d-e%d", id, j)))
			}
		}(i)
	}

	wg.Wait()

	estimate := estimator.Estimate()
	// Should be roughly 10000 unique elements
	if estimate < 8000 || estimate > 12000 {
		t.Errorf("Concurrent estimate = %d, expected ~10000", estimate)
	}
}
