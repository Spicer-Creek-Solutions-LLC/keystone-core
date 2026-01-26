package metrics

import (
	"testing"
)

func TestNewCustomMetricRegistry(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	if registry == nil {
		t.Fatal("Expected registry to be created")
	}
	if registry.config == nil {
		t.Error("Expected config to be set")
	}
	if registry.Count() != 0 {
		t.Error("Expected empty registry")
	}
}

func TestCustomMetricRegistry_RegisterMetric(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name:   "test_counter",
			Type:   MetricTypeCounter,
			Help:   "A test counter",
			Labels: []string{"label1", "label2"},
		},
		Namespace: NamespaceUser,
		Owner:     "test-owner",
	}

	err := registry.RegisterMetric(metric)
	if err != nil {
		t.Fatalf("Failed to register metric: %v", err)
	}

	if registry.Count() != 1 {
		t.Errorf("Expected 1 metric, got %d", registry.Count())
	}
}

func TestCustomMetricRegistry_RegisterMetric_AllTypes(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	tests := []struct {
		name       string
		metricType MetricType
	}{
		{"counter", MetricTypeCounter},
		{"gauge", MetricTypeGauge},
		{"histogram", MetricTypeHistogram},
		{"summary", MetricTypeSummary},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := &CustomMetric{
				Definition: MetricDefinition{
					Name: "test_" + tt.name,
					Type: tt.metricType,
					Help: "A test " + tt.name,
				},
				Namespace: NamespaceUser,
				Owner:     "test-owner",
			}

			if err := registry.RegisterMetric(metric); err != nil {
				t.Errorf("Failed to register %s: %v", tt.name, err)
			}
		})
	}
}

func TestCustomMetricRegistry_RegisterMetric_Duplicate(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name: "dup_counter",
			Type: MetricTypeCounter,
			Help: "A test counter",
		},
		Namespace: NamespaceUser,
		Owner:     "test-owner",
	}

	if err := registry.RegisterMetric(metric); err != nil {
		t.Fatalf("Failed to register metric: %v", err)
	}

	// Try to register again
	err := registry.RegisterMetric(metric)
	if err == nil {
		t.Error("Expected error when registering duplicate metric")
	}
}

func TestCustomMetricRegistry_RegisterMetric_InvalidName(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	tests := []struct {
		name        string
		metricName  string
		shouldError bool
	}{
		{"valid_name", "valid_metric", false},
		{"starts_with_underscore", "_metric", false},
		{"starts_with_number", "1metric", true},
		{"has_dash", "metric-name", true},
		{"has_space", "metric name", true},
		{"empty", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metric := &CustomMetric{
				Definition: MetricDefinition{
					Name: tt.metricName,
					Type: MetricTypeCounter,
					Help: "Test",
				},
				Namespace: NamespaceUser,
				Owner:     "test-owner",
			}

			err := registry.RegisterMetric(metric)
			if tt.shouldError && err == nil {
				t.Error("Expected error but got none")
			}
			if !tt.shouldError && err != nil {
				t.Errorf("Unexpected error: %v", err)
			}
		})
	}
}

func TestCustomMetricRegistry_RegisterMetric_InvalidLabels(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name:   "test_invalid_label",
			Type:   MetricTypeCounter,
			Help:   "Test",
			Labels: []string{"valid_label", "1invalid"},
		},
		Namespace: NamespaceUser,
		Owner:     "test-owner",
	}

	err := registry.RegisterMetric(metric)
	if err == nil {
		t.Error("Expected error for invalid label name")
	}
}

func TestCustomMetricRegistry_RegisterMetric_TooManyLabels(t *testing.T) {
	collector := NewPrometheusCollector()
	config := DefaultCustomMetricConfig()
	config.MaxLabels = 2
	registry := NewCustomMetricRegistry(collector, config)

	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name:   "test_many_labels",
			Type:   MetricTypeCounter,
			Help:   "Test",
			Labels: []string{"label1", "label2", "label3"},
		},
		Namespace: NamespaceUser,
		Owner:     "test-owner",
	}

	err := registry.RegisterMetric(metric)
	if err == nil {
		t.Error("Expected error for too many labels")
	}
}

func TestCustomMetricRegistry_RegisterMetric_MaxMetrics(t *testing.T) {
	collector := NewPrometheusCollector()
	config := DefaultCustomMetricConfig()
	config.MaxMetrics = 2
	registry := NewCustomMetricRegistry(collector, config)

	for i := 0; i < 2; i++ {
		metric := &CustomMetric{
			Definition: MetricDefinition{
				Name: "test_metric_" + string(rune('a'+i)),
				Type: MetricTypeCounter,
				Help: "Test",
			},
			Namespace: NamespaceUser,
			Owner:     "test-owner",
		}
		if err := registry.RegisterMetric(metric); err != nil {
			t.Fatalf("Failed to register metric %d: %v", i, err)
		}
	}

	// Try to register one more
	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name: "test_metric_overflow",
			Type: MetricTypeCounter,
			Help: "Test",
		},
		Namespace: NamespaceUser,
		Owner:     "test-owner",
	}

	err := registry.RegisterMetric(metric)
	if err == nil {
		t.Error("Expected error when exceeding max metrics")
	}
}

func TestCustomMetricRegistry_RegisterMetric_InvalidNamespace(t *testing.T) {
	collector := NewPrometheusCollector()
	config := DefaultCustomMetricConfig()
	config.AllowedNamespaces = []CustomMetricNamespace{NamespaceUser}
	registry := NewCustomMetricRegistry(collector, config)

	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name: "test_plugin_metric",
			Type: MetricTypeCounter,
			Help: "Test",
		},
		Namespace: NamespacePlugin, // Not allowed
		Owner:     "test-owner",
	}

	err := registry.RegisterMetric(metric)
	if err == nil {
		t.Error("Expected error for disallowed namespace")
	}
}

func TestCustomMetricRegistry_RegisterMetric_MissingOwner(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name: "test_no_owner",
			Type: MetricTypeCounter,
			Help: "Test",
		},
		Namespace: NamespaceUser,
		Owner:     "", // Empty owner
	}

	err := registry.RegisterMetric(metric)
	if err == nil {
		t.Error("Expected error for missing owner")
	}
}

func TestCustomMetricRegistry_GetMetric(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name: "get_test_counter",
			Type: MetricTypeCounter,
			Help: "A test counter",
		},
		Namespace:   NamespaceUser,
		Owner:       "test-owner",
		Description: "Test description",
		Tags:        []string{"test", "example"},
	}

	if err := registry.RegisterMetric(metric); err != nil {
		t.Fatalf("Failed to register metric: %v", err)
	}

	retrieved, ok := registry.GetMetric("get_test_counter", NamespaceUser)
	if !ok {
		t.Fatal("Metric not found")
	}

	if retrieved.Owner != "test-owner" {
		t.Errorf("Expected owner 'test-owner', got '%s'", retrieved.Owner)
	}
	if retrieved.Description != "Test description" {
		t.Errorf("Expected description 'Test description', got '%s'", retrieved.Description)
	}
	if !retrieved.Enabled {
		t.Error("Expected metric to be enabled")
	}
}

func TestCustomMetricRegistry_GetMetric_NotFound(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	_, ok := registry.GetMetric("nonexistent", NamespaceUser)
	if ok {
		t.Error("Expected metric not to be found")
	}
}

func TestCustomMetricRegistry_ListMetrics(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	for i := 0; i < 3; i++ {
		metric := &CustomMetric{
			Definition: MetricDefinition{
				Name: "list_test_" + string(rune('a'+i)),
				Type: MetricTypeCounter,
				Help: "Test",
			},
			Namespace: NamespaceUser,
			Owner:     "test-owner",
		}
		if err := registry.RegisterMetric(metric); err != nil {
			t.Fatalf("Failed to register metric: %v", err)
		}
	}

	metrics := registry.ListMetrics()
	if len(metrics) != 3 {
		t.Errorf("Expected 3 metrics, got %d", len(metrics))
	}
}

func TestCustomMetricRegistry_ListMetricsByNamespace(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	// Register user metrics
	for i := 0; i < 2; i++ {
		metric := &CustomMetric{
			Definition: MetricDefinition{
				Name: "ns_user_" + string(rune('a'+i)),
				Type: MetricTypeCounter,
				Help: "Test",
			},
			Namespace: NamespaceUser,
			Owner:     "test-owner",
		}
		if err := registry.RegisterMetric(metric); err != nil {
			t.Fatalf("Failed to register metric: %v", err)
		}
	}

	// Register plugin metrics
	for i := 0; i < 3; i++ {
		metric := &CustomMetric{
			Definition: MetricDefinition{
				Name: "ns_plugin_" + string(rune('a'+i)),
				Type: MetricTypeCounter,
				Help: "Test",
			},
			Namespace: NamespacePlugin,
			Owner:     "test-owner",
		}
		if err := registry.RegisterMetric(metric); err != nil {
			t.Fatalf("Failed to register metric: %v", err)
		}
	}

	userMetrics := registry.ListMetricsByNamespace(NamespaceUser)
	if len(userMetrics) != 2 {
		t.Errorf("Expected 2 user metrics, got %d", len(userMetrics))
	}

	pluginMetrics := registry.ListMetricsByNamespace(NamespacePlugin)
	if len(pluginMetrics) != 3 {
		t.Errorf("Expected 3 plugin metrics, got %d", len(pluginMetrics))
	}
}

func TestCustomMetricRegistry_ListMetricsByOwner(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	owners := []string{"owner1", "owner2"}
	for i, owner := range owners {
		for j := 0; j < i+1; j++ {
			metric := &CustomMetric{
				Definition: MetricDefinition{
					Name: "owner_test_" + owner + "_" + string(rune('a'+j)),
					Type: MetricTypeCounter,
					Help: "Test",
				},
				Namespace: NamespaceUser,
				Owner:     owner,
			}
			if err := registry.RegisterMetric(metric); err != nil {
				t.Fatalf("Failed to register metric: %v", err)
			}
		}
	}

	owner1Metrics := registry.ListMetricsByOwner("owner1")
	if len(owner1Metrics) != 1 {
		t.Errorf("Expected 1 metric for owner1, got %d", len(owner1Metrics))
	}

	owner2Metrics := registry.ListMetricsByOwner("owner2")
	if len(owner2Metrics) != 2 {
		t.Errorf("Expected 2 metrics for owner2, got %d", len(owner2Metrics))
	}
}

func TestCustomMetricRegistry_ListMetricsByTag(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	// Register metrics with different tags
	metrics := []*CustomMetric{
		{
			Definition: MetricDefinition{Name: "tag_test_a", Type: MetricTypeCounter, Help: "Test"},
			Namespace:  NamespaceUser,
			Owner:      "test",
			Tags:       []string{"prod", "important"},
		},
		{
			Definition: MetricDefinition{Name: "tag_test_b", Type: MetricTypeCounter, Help: "Test"},
			Namespace:  NamespaceUser,
			Owner:      "test",
			Tags:       []string{"staging", "important"},
		},
		{
			Definition: MetricDefinition{Name: "tag_test_c", Type: MetricTypeCounter, Help: "Test"},
			Namespace:  NamespaceUser,
			Owner:      "test",
			Tags:       []string{"prod"},
		},
	}

	for _, m := range metrics {
		if err := registry.RegisterMetric(m); err != nil {
			t.Fatalf("Failed to register metric: %v", err)
		}
	}

	prodMetrics := registry.ListMetricsByTag("prod")
	if len(prodMetrics) != 2 {
		t.Errorf("Expected 2 metrics with 'prod' tag, got %d", len(prodMetrics))
	}

	importantMetrics := registry.ListMetricsByTag("important")
	if len(importantMetrics) != 2 {
		t.Errorf("Expected 2 metrics with 'important' tag, got %d", len(importantMetrics))
	}
}

func TestCustomMetricRegistry_UnregisterMetric(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name: "unregister_test",
			Type: MetricTypeCounter,
			Help: "Test",
		},
		Namespace: NamespaceUser,
		Owner:     "test-owner",
	}

	if err := registry.RegisterMetric(metric); err != nil {
		t.Fatalf("Failed to register metric: %v", err)
	}

	if registry.Count() != 1 {
		t.Error("Expected 1 metric after registration")
	}

	if err := registry.UnregisterMetric("unregister_test", NamespaceUser); err != nil {
		t.Fatalf("Failed to unregister metric: %v", err)
	}

	if registry.Count() != 0 {
		t.Error("Expected 0 metrics after unregistration")
	}
}

func TestCustomMetricRegistry_UnregisterMetric_NotFound(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	err := registry.UnregisterMetric("nonexistent", NamespaceUser)
	if err == nil {
		t.Error("Expected error when unregistering nonexistent metric")
	}
}

func TestCustomMetricRegistry_EnableDisable(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name: "enable_disable_test",
			Type: MetricTypeCounter,
			Help: "Test",
		},
		Namespace: NamespaceUser,
		Owner:     "test-owner",
	}

	if err := registry.RegisterMetric(metric); err != nil {
		t.Fatalf("Failed to register metric: %v", err)
	}

	// Disable
	if err := registry.DisableMetric("enable_disable_test", NamespaceUser); err != nil {
		t.Fatalf("Failed to disable metric: %v", err)
	}

	retrieved, _ := registry.GetMetric("enable_disable_test", NamespaceUser)
	if retrieved.Enabled {
		t.Error("Expected metric to be disabled")
	}

	// Enable
	if err := registry.EnableMetric("enable_disable_test", NamespaceUser); err != nil {
		t.Fatalf("Failed to enable metric: %v", err)
	}

	retrieved, _ = registry.GetMetric("enable_disable_test", NamespaceUser)
	if !retrieved.Enabled {
		t.Error("Expected metric to be enabled")
	}
}

func TestCustomMetricRegistry_Summary(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	// Register various metrics
	metrics := []*CustomMetric{
		{
			Definition: MetricDefinition{Name: "summary_counter", Type: MetricTypeCounter, Help: "Test"},
			Namespace:  NamespaceUser,
			Owner:      "owner1",
		},
		{
			Definition: MetricDefinition{Name: "summary_gauge", Type: MetricTypeGauge, Help: "Test"},
			Namespace:  NamespacePlugin,
			Owner:      "owner1",
		},
		{
			Definition: MetricDefinition{Name: "summary_histogram", Type: MetricTypeHistogram, Help: "Test"},
			Namespace:  NamespaceUser,
			Owner:      "owner2",
		},
	}

	for _, m := range metrics {
		if err := registry.RegisterMetric(m); err != nil {
			t.Fatalf("Failed to register metric: %v", err)
		}
	}

	summary := registry.Summary()

	if summary.TotalMetrics != 3 {
		t.Errorf("Expected 3 total metrics, got %d", summary.TotalMetrics)
	}
	if summary.MetricsByNamespace[NamespaceUser] != 2 {
		t.Errorf("Expected 2 user metrics, got %d", summary.MetricsByNamespace[NamespaceUser])
	}
	if summary.MetricsByNamespace[NamespacePlugin] != 1 {
		t.Errorf("Expected 1 plugin metric, got %d", summary.MetricsByNamespace[NamespacePlugin])
	}
	if summary.MetricsByOwner["owner1"] != 2 {
		t.Errorf("Expected 2 metrics for owner1, got %d", summary.MetricsByOwner["owner1"])
	}
	if summary.EnabledCount != 3 {
		t.Errorf("Expected 3 enabled metrics, got %d", summary.EnabledCount)
	}
}

func TestCustomMetricBuilder(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	// Build and register a counter
	err := NewCounter("builder_counter").
		Help("A counter built with the builder").
		Labels("label1", "label2").
		Namespace(NamespaceUser).
		Owner("test-builder").
		Description("Built using the fluent API").
		Tags("test", "builder").
		Register(registry)

	if err != nil {
		t.Fatalf("Failed to register counter: %v", err)
	}

	metric, ok := registry.GetMetric("builder_counter", NamespaceUser)
	if !ok {
		t.Fatal("Counter not found")
	}
	if metric.Definition.Help != "A counter built with the builder" {
		t.Error("Help text not set correctly")
	}
	if len(metric.Definition.Labels) != 2 {
		t.Error("Labels not set correctly")
	}
}

func TestCustomMetricBuilder_Gauge(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	err := NewGauge("builder_gauge").
		Help("A test gauge").
		Owner("test").
		Register(registry)

	if err != nil {
		t.Fatalf("Failed to register gauge: %v", err)
	}

	metric, ok := registry.GetMetric("builder_gauge", NamespaceUser)
	if !ok {
		t.Fatal("Gauge not found")
	}
	if metric.Definition.Type != MetricTypeGauge {
		t.Error("Metric type should be gauge")
	}
}

func TestCustomMetricBuilder_Histogram(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	customBuckets := []float64{0.1, 0.5, 1.0, 5.0}
	err := NewHistogram("builder_histogram").
		Help("A test histogram").
		Buckets(customBuckets...).
		Owner("test").
		Register(registry)

	if err != nil {
		t.Fatalf("Failed to register histogram: %v", err)
	}

	metric, ok := registry.GetMetric("builder_histogram", NamespaceUser)
	if !ok {
		t.Fatal("Histogram not found")
	}
	if metric.Definition.Type != MetricTypeHistogram {
		t.Error("Metric type should be histogram")
	}
}

func TestCustomMetricBuilder_Summary(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	customObjectives := map[float64]float64{0.5: 0.05, 0.99: 0.001}
	err := NewSummary("builder_summary").
		Help("A test summary").
		Objectives(customObjectives).
		Owner("test").
		Register(registry)

	if err != nil {
		t.Fatalf("Failed to register summary: %v", err)
	}

	metric, ok := registry.GetMetric("builder_summary", NamespaceUser)
	if !ok {
		t.Fatal("Summary not found")
	}
	if metric.Definition.Type != MetricTypeSummary {
		t.Error("Metric type should be summary")
	}
}

func TestCustomMetricBuilder_BucketsOnNonHistogram(t *testing.T) {
	_, err := NewCounter("invalid_buckets").
		Help("Test").
		Buckets(0.1, 0.5, 1.0).
		Owner("test").
		Build()

	if err == nil {
		t.Error("Expected error when setting buckets on non-histogram")
	}
}

func TestCustomMetricBuilder_ObjectivesOnNonSummary(t *testing.T) {
	_, err := NewCounter("invalid_objectives").
		Help("Test").
		Objectives(map[float64]float64{0.5: 0.05}).
		Owner("test").
		Build()

	if err == nil {
		t.Error("Expected error when setting objectives on non-summary")
	}
}

func TestLoadCustomMetricsFromJSON(t *testing.T) {
	jsonData := `[
		{
			"definition": {
				"name": "json_counter",
				"type": "counter",
				"help": "A counter from JSON"
			},
			"namespace": "user",
			"owner": "json-test"
		}
	]`

	metrics, err := LoadCustomMetricsFromJSON([]byte(jsonData))
	if err != nil {
		t.Fatalf("Failed to load metrics from JSON: %v", err)
	}

	if len(metrics) != 1 {
		t.Errorf("Expected 1 metric, got %d", len(metrics))
	}

	if metrics[0].Definition.Name != "json_counter" {
		t.Errorf("Expected name 'json_counter', got '%s'", metrics[0].Definition.Name)
	}
}

func TestExportCustomMetricsToJSON(t *testing.T) {
	metrics := []*CustomMetric{
		{
			Definition: MetricDefinition{
				Name: "export_test",
				Type: MetricTypeCounter,
				Help: "Test metric",
			},
			Namespace: NamespaceUser,
			Owner:     "test",
		},
	}

	jsonData, err := ExportCustomMetricsToJSON(metrics)
	if err != nil {
		t.Fatalf("Failed to export metrics to JSON: %v", err)
	}

	if len(jsonData) == 0 {
		t.Error("Expected non-empty JSON output")
	}
}

func TestCustomMetricRegistry_HistogramBucketsOrder(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	// Invalid bucket order
	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name:    "invalid_buckets",
			Type:    MetricTypeHistogram,
			Help:    "Test",
			Buckets: []float64{1.0, 0.5, 2.0}, // Not increasing
		},
		Namespace: NamespaceUser,
		Owner:     "test",
	}

	err := registry.RegisterMetric(metric)
	if err == nil {
		t.Error("Expected error for non-increasing histogram buckets")
	}
}

func TestCustomMetricRegistry_SummaryQuantileRange(t *testing.T) {
	collector := NewPrometheusCollector()
	registry := NewCustomMetricRegistry(collector, nil)

	// Invalid quantile
	metric := &CustomMetric{
		Definition: MetricDefinition{
			Name: "invalid_quantile",
			Type: MetricTypeSummary,
			Help: "Test",
			Objectives: map[float64]float64{
				1.5: 0.05, // Invalid quantile > 1
			},
		},
		Namespace: NamespaceUser,
		Owner:     "test",
	}

	err := registry.RegisterMetric(metric)
	if err == nil {
		t.Error("Expected error for invalid summary quantile")
	}
}

func TestDefaultCustomMetricConfig(t *testing.T) {
	config := DefaultCustomMetricConfig()

	if config.Prefix != "kscore_custom_" {
		t.Errorf("Expected prefix 'kscore_custom_', got '%s'", config.Prefix)
	}
	if config.MaxMetrics != 1000 {
		t.Errorf("Expected max metrics 1000, got %d", config.MaxMetrics)
	}
	if config.MaxLabels != 10 {
		t.Errorf("Expected max labels 10, got %d", config.MaxLabels)
	}
	if len(config.AllowedNamespaces) != 3 {
		t.Errorf("Expected 3 allowed namespaces, got %d", len(config.AllowedNamespaces))
	}
}
