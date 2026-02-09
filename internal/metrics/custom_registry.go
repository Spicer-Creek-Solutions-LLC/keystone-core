package metrics

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sync"
	"time"
)

// CustomMetricNamespace represents a namespace for custom metrics
type CustomMetricNamespace string

const (
	// NamespaceUser is for user-defined metrics
	NamespaceUser CustomMetricNamespace = "user"

	// NamespacePlugin is for plugin-defined metrics
	NamespacePlugin CustomMetricNamespace = "plugin"

	// NamespaceModule is for module-defined metrics
	NamespaceModule CustomMetricNamespace = "module"
)

// CustomMetricConfig holds configuration for the custom metric registry
type CustomMetricConfig struct {
	// Prefix to add to all custom metric names
	Prefix string `json:"prefix"`

	// MaxMetrics is the maximum number of custom metrics allowed (0 = unlimited)
	MaxMetrics int `json:"max_metrics,omitempty"`

	// MaxLabels is the maximum number of labels per metric
	MaxLabels int `json:"max_labels,omitempty"`

	// MaxLabelValueLength is the maximum length of label values
	MaxLabelValueLength int `json:"max_label_value_length,omitempty"`

	// AllowedNamespaces restricts which namespaces can register metrics
	AllowedNamespaces []CustomMetricNamespace `json:"allowed_namespaces,omitempty"`

	// ReservedPrefixes are metric name prefixes that cannot be used
	ReservedPrefixes []string `json:"reserved_prefixes,omitempty"`
}

// DefaultCustomMetricConfig returns sensible defaults
func DefaultCustomMetricConfig() *CustomMetricConfig {
	return &CustomMetricConfig{
		Prefix:              "kscore_custom_",
		MaxMetrics:          1000,
		MaxLabels:           10,
		MaxLabelValueLength: 128,
		AllowedNamespaces: []CustomMetricNamespace{
			NamespaceUser,
			NamespacePlugin,
			NamespaceModule,
		},
		ReservedPrefixes: []string{
			"kscore_",   // Internal metrics
			"go_",       // Go runtime metrics
			"process_",  // Process metrics
			"promhttp_", // Prometheus HTTP metrics
		},
	}
}

// CustomMetric represents a user-registered metric
type CustomMetric struct {
	// Definition holds the metric definition
	Definition MetricDefinition `json:"definition"`

	// Namespace categorizes the metric
	Namespace CustomMetricNamespace `json:"namespace"`

	// Owner identifies who registered the metric
	Owner string `json:"owner"`

	// Description provides additional context
	Description string `json:"description,omitempty"`

	// Tags for categorization
	Tags []string `json:"tags,omitempty"`

	// CreatedAt is when the metric was registered
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is when the metric was last updated
	UpdatedAt time.Time `json:"updated_at"`

	// Enabled indicates if the metric is currently active
	Enabled bool `json:"enabled"`
}

// CustomMetricRegistry manages custom metric registration
type CustomMetricRegistry struct {
	config     *CustomMetricConfig
	collector  *PrometheusCollector
	metrics    map[string]*CustomMetric
	mu         sync.RWMutex
	nameRegex  *regexp.Regexp
	labelRegex *regexp.Regexp
}

// NewCustomMetricRegistry creates a new custom metric registry
func NewCustomMetricRegistry(collector *PrometheusCollector, config *CustomMetricConfig) *CustomMetricRegistry {
	if config == nil {
		config = DefaultCustomMetricConfig()
	}

	return &CustomMetricRegistry{
		config:     config,
		collector:  collector,
		metrics:    make(map[string]*CustomMetric),
		nameRegex:  regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`),
		labelRegex: regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`),
	}
}

// RegisterMetric registers a new custom metric
func (r *CustomMetricRegistry) RegisterMetric(metric *CustomMetric) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Validate the metric
	if err := r.validateMetric(metric); err != nil {
		return fmt.Errorf("validation failed: %w", err)
	}

	// Check if metric already exists
	fullName := r.fullMetricName(metric.Definition.Name, metric.Namespace)
	if _, exists := r.metrics[fullName]; exists {
		return fmt.Errorf("metric already registered: %s", fullName)
	}

	// Check max metrics limit
	if r.config.MaxMetrics > 0 && len(r.metrics) >= r.config.MaxMetrics {
		return fmt.Errorf("maximum number of custom metrics reached (%d)", r.config.MaxMetrics)
	}

	// Create the full metric definition with prefixed name
	def := metric.Definition
	def.Name = fullName

	// Register with Prometheus
	if err := r.collector.RegisterMetric(def); err != nil {
		return fmt.Errorf("failed to register with prometheus: %w", err)
	}

	// Store the custom metric
	now := time.Now()
	metric.CreatedAt = now
	metric.UpdatedAt = now
	metric.Enabled = true

	// Store with the full name
	storedMetric := *metric
	storedMetric.Definition.Name = fullName
	r.metrics[fullName] = &storedMetric

	return nil
}

// validateMetric validates a custom metric definition
func (r *CustomMetricRegistry) validateMetric(metric *CustomMetric) error {
	// Validate namespace
	if !r.isNamespaceAllowed(metric.Namespace) {
		return fmt.Errorf("namespace not allowed: %s", metric.Namespace)
	}

	// Validate metric name
	if !r.nameRegex.MatchString(metric.Definition.Name) {
		return fmt.Errorf("invalid metric name: %s (must match [a-zA-Z_][a-zA-Z0-9_]*)", metric.Definition.Name)
	}

	// Check reserved prefixes (but allow our own configured prefix)
	fullName := r.fullMetricName(metric.Definition.Name, metric.Namespace)
	for _, reserved := range r.config.ReservedPrefixes {
		// Skip check if the reserved prefix is our configured prefix
		if len(r.config.Prefix) >= len(reserved) && r.config.Prefix[:len(reserved)] == reserved {
			continue
		}
		if len(fullName) >= len(reserved) && fullName[:len(reserved)] == reserved {
			return fmt.Errorf("metric name uses reserved prefix: %s", reserved)
		}
	}

	// Validate owner
	if metric.Owner == "" {
		return fmt.Errorf("metric owner is required")
	}

	// Validate metric type
	switch metric.Definition.Type {
	case MetricTypeCounter, MetricTypeGauge, MetricTypeHistogram, MetricTypeSummary:
		// Valid types
	default:
		return fmt.Errorf("invalid metric type: %s", metric.Definition.Type)
	}

	// Validate labels
	if r.config.MaxLabels > 0 && len(metric.Definition.Labels) > r.config.MaxLabels {
		return fmt.Errorf("too many labels: %d (max: %d)", len(metric.Definition.Labels), r.config.MaxLabels)
	}

	for _, label := range metric.Definition.Labels {
		if !r.labelRegex.MatchString(label) {
			return fmt.Errorf("invalid label name: %s (must match [a-zA-Z_][a-zA-Z0-9_]*)", label)
		}
	}

	// Validate histogram buckets
	if metric.Definition.Type == MetricTypeHistogram {
		for i, bucket := range metric.Definition.Buckets {
			if i > 0 && bucket <= metric.Definition.Buckets[i-1] {
				return fmt.Errorf("histogram buckets must be in increasing order")
			}
		}
	}

	// Validate summary objectives
	if metric.Definition.Type == MetricTypeSummary {
		for quantile := range metric.Definition.Objectives {
			if quantile < 0 || quantile > 1 {
				return fmt.Errorf("summary quantile must be between 0 and 1: %f", quantile)
			}
		}
	}

	return nil
}

// isNamespaceAllowed checks if a namespace is allowed
func (r *CustomMetricRegistry) isNamespaceAllowed(namespace CustomMetricNamespace) bool {
	if len(r.config.AllowedNamespaces) == 0 {
		return true
	}
	for _, allowed := range r.config.AllowedNamespaces {
		if allowed == namespace {
			return true
		}
	}
	return false
}

// fullMetricName generates the full metric name with prefix and namespace
func (r *CustomMetricRegistry) fullMetricName(name string, namespace CustomMetricNamespace) string {
	return fmt.Sprintf("%s%s_%s", r.config.Prefix, namespace, name)
}

// GetMetric retrieves a registered custom metric
func (r *CustomMetricRegistry) GetMetric(name string, namespace CustomMetricNamespace) (*CustomMetric, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	fullName := r.fullMetricName(name, namespace)
	metric, ok := r.metrics[fullName]
	return metric, ok
}

// ListMetrics returns all registered custom metrics
func (r *CustomMetricRegistry) ListMetrics() []*CustomMetric {
	r.mu.RLock()
	defer r.mu.RUnlock()

	metrics := make([]*CustomMetric, 0, len(r.metrics))
	for _, metric := range r.metrics {
		metrics = append(metrics, metric)
	}
	return metrics
}

// ListMetricsByNamespace returns metrics in a specific namespace
func (r *CustomMetricRegistry) ListMetricsByNamespace(namespace CustomMetricNamespace) []*CustomMetric {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var metrics []*CustomMetric
	for _, metric := range r.metrics {
		if metric.Namespace == namespace {
			metrics = append(metrics, metric)
		}
	}
	return metrics
}

// ListMetricsByOwner returns metrics registered by a specific owner
func (r *CustomMetricRegistry) ListMetricsByOwner(owner string) []*CustomMetric {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var metrics []*CustomMetric
	for _, metric := range r.metrics {
		if metric.Owner == owner {
			metrics = append(metrics, metric)
		}
	}
	return metrics
}

// ListMetricsByTag returns metrics with a specific tag
func (r *CustomMetricRegistry) ListMetricsByTag(tag string) []*CustomMetric {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var metrics []*CustomMetric
	for _, metric := range r.metrics {
		for _, t := range metric.Tags {
			if t == tag {
				metrics = append(metrics, metric)
				break
			}
		}
	}
	return metrics
}

// UnregisterMetric removes a custom metric
func (r *CustomMetricRegistry) UnregisterMetric(name string, namespace CustomMetricNamespace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fullName := r.fullMetricName(name, namespace)
	metric, ok := r.metrics[fullName]
	if !ok {
		return fmt.Errorf("metric not found: %s", fullName)
	}

	// Mark as disabled (Prometheus doesn't support unregistration of individual metrics)
	metric.Enabled = false
	metric.UpdatedAt = time.Now()

	delete(r.metrics, fullName)
	return nil
}

// EnableMetric enables a disabled metric
func (r *CustomMetricRegistry) EnableMetric(name string, namespace CustomMetricNamespace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fullName := r.fullMetricName(name, namespace)
	metric, ok := r.metrics[fullName]
	if !ok {
		return fmt.Errorf("metric not found: %s", fullName)
	}

	metric.Enabled = true
	metric.UpdatedAt = time.Now()
	return nil
}

// DisableMetric disables a metric without removing it
func (r *CustomMetricRegistry) DisableMetric(name string, namespace CustomMetricNamespace) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fullName := r.fullMetricName(name, namespace)
	metric, ok := r.metrics[fullName]
	if !ok {
		return fmt.Errorf("metric not found: %s", fullName)
	}

	metric.Enabled = false
	metric.UpdatedAt = time.Now()
	return nil
}

// Count returns the number of registered custom metrics
func (r *CustomMetricRegistry) Count() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.metrics)
}

// CountByNamespace returns the count of metrics in a namespace
func (r *CustomMetricRegistry) CountByNamespace(namespace CustomMetricNamespace) int {
	r.mu.RLock()
	defer r.mu.RUnlock()

	count := 0
	for _, metric := range r.metrics {
		if metric.Namespace == namespace {
			count++
		}
	}
	return count
}

// Config returns the registry configuration
func (r *CustomMetricRegistry) Config() *CustomMetricConfig {
	return r.config
}

// RegistrationSummary provides statistics about registered metrics
type RegistrationSummary struct {
	TotalMetrics       int                           `json:"total_metrics"`
	MetricsByNamespace map[CustomMetricNamespace]int `json:"metrics_by_namespace"`
	MetricsByType      map[MetricType]int            `json:"metrics_by_type"`
	MetricsByOwner     map[string]int                `json:"metrics_by_owner"`
	EnabledCount       int                           `json:"enabled_count"`
	DisabledCount      int                           `json:"disabled_count"`
}

// Summary returns a summary of registered metrics
func (r *CustomMetricRegistry) Summary() *RegistrationSummary {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summary := &RegistrationSummary{
		TotalMetrics:       len(r.metrics),
		MetricsByNamespace: make(map[CustomMetricNamespace]int),
		MetricsByType:      make(map[MetricType]int),
		MetricsByOwner:     make(map[string]int),
	}

	for _, metric := range r.metrics {
		summary.MetricsByNamespace[metric.Namespace]++
		summary.MetricsByType[metric.Definition.Type]++
		summary.MetricsByOwner[metric.Owner]++
		if metric.Enabled {
			summary.EnabledCount++
		} else {
			summary.DisabledCount++
		}
	}

	return summary
}

// CustomMetricBuilder provides a fluent interface for building custom metrics
type CustomMetricBuilder struct {
	metric *CustomMetric
	err    error
}

// NewCounter creates a builder for a counter metric
func NewCounter(name string) *CustomMetricBuilder {
	return &CustomMetricBuilder{
		metric: &CustomMetric{
			Definition: MetricDefinition{
				Name: name,
				Type: MetricTypeCounter,
			},
			Namespace: NamespaceUser,
			Enabled:   true,
		},
	}
}

// NewGauge creates a builder for a gauge metric
func NewGauge(name string) *CustomMetricBuilder {
	return &CustomMetricBuilder{
		metric: &CustomMetric{
			Definition: MetricDefinition{
				Name: name,
				Type: MetricTypeGauge,
			},
			Namespace: NamespaceUser,
			Enabled:   true,
		},
	}
}

// NewHistogram creates a builder for a histogram metric
func NewHistogram(name string) *CustomMetricBuilder {
	return &CustomMetricBuilder{
		metric: &CustomMetric{
			Definition: MetricDefinition{
				Name:    name,
				Type:    MetricTypeHistogram,
				Buckets: DefaultBuckets,
			},
			Namespace: NamespaceUser,
			Enabled:   true,
		},
	}
}

// NewSummary creates a builder for a summary metric
func NewSummary(name string) *CustomMetricBuilder {
	return &CustomMetricBuilder{
		metric: &CustomMetric{
			Definition: MetricDefinition{
				Name:       name,
				Type:       MetricTypeSummary,
				Objectives: DefaultObjectives,
			},
			Namespace: NamespaceUser,
			Enabled:   true,
		},
	}
}

// Help sets the help text for the metric
func (b *CustomMetricBuilder) Help(help string) *CustomMetricBuilder {
	b.metric.Definition.Help = help
	return b
}

// Labels sets the label names for the metric
func (b *CustomMetricBuilder) Labels(labels ...string) *CustomMetricBuilder {
	b.metric.Definition.Labels = labels
	return b
}

// Buckets sets the histogram buckets (only for histograms)
func (b *CustomMetricBuilder) Buckets(buckets ...float64) *CustomMetricBuilder {
	if b.metric.Definition.Type != MetricTypeHistogram {
		b.err = fmt.Errorf("buckets can only be set for histogram metrics")
		return b
	}
	b.metric.Definition.Buckets = buckets
	return b
}

// Objectives sets the summary objectives (only for summaries)
func (b *CustomMetricBuilder) Objectives(objectives map[float64]float64) *CustomMetricBuilder {
	if b.metric.Definition.Type != MetricTypeSummary {
		b.err = fmt.Errorf("objectives can only be set for summary metrics")
		return b
	}
	b.metric.Definition.Objectives = objectives
	return b
}

// Namespace sets the metric namespace
func (b *CustomMetricBuilder) Namespace(namespace CustomMetricNamespace) *CustomMetricBuilder {
	b.metric.Namespace = namespace
	return b
}

// Owner sets the metric owner
func (b *CustomMetricBuilder) Owner(owner string) *CustomMetricBuilder {
	b.metric.Owner = owner
	return b
}

// Description sets the metric description
func (b *CustomMetricBuilder) Description(description string) *CustomMetricBuilder {
	b.metric.Description = description
	return b
}

// Tags sets the metric tags
func (b *CustomMetricBuilder) Tags(tags ...string) *CustomMetricBuilder {
	b.metric.Tags = tags
	return b
}

// Build returns the built CustomMetric
func (b *CustomMetricBuilder) Build() (*CustomMetric, error) {
	if b.err != nil {
		return nil, b.err
	}
	return b.metric, nil
}

// Register registers the metric directly with a registry
func (b *CustomMetricBuilder) Register(registry *CustomMetricRegistry) error {
	if b.err != nil {
		return b.err
	}
	return registry.RegisterMetric(b.metric)
}

// LoadCustomMetricsFromJSON loads custom metrics from JSON configuration
func LoadCustomMetricsFromJSON(data []byte) ([]*CustomMetric, error) {
	var metrics []*CustomMetric
	if err := json.Unmarshal(data, &metrics); err != nil {
		return nil, fmt.Errorf("failed to parse custom metrics: %w", err)
	}
	return metrics, nil
}

// ExportCustomMetricsToJSON exports custom metrics to JSON
func ExportCustomMetricsToJSON(metrics []*CustomMetric) ([]byte, error) {
	// Convert to exportable format (map[float64]float64 isn't JSON-serializable)
	type exportableObjectives map[string]float64
	type exportableDefinition struct {
		Name       string               `json:"name"`
		Type       MetricType           `json:"type"`
		Help       string               `json:"help"`
		Labels     []string             `json:"labels,omitempty"`
		Buckets    []float64            `json:"buckets,omitempty"`
		Objectives exportableObjectives `json:"objectives,omitempty"`
	}
	type exportableMetric struct {
		Definition  exportableDefinition  `json:"definition"`
		Namespace   CustomMetricNamespace `json:"namespace"`
		Owner       string                `json:"owner"`
		Description string                `json:"description,omitempty"`
		Tags        []string              `json:"tags,omitempty"`
		CreatedAt   time.Time             `json:"created_at"`
		UpdatedAt   time.Time             `json:"updated_at"`
		Enabled     bool                  `json:"enabled"`
	}

	exportable := make([]exportableMetric, len(metrics))
	for i, m := range metrics {
		objectives := make(exportableObjectives)
		for k, v := range m.Definition.Objectives {
			objectives[fmt.Sprintf("%g", k)] = v
		}
		exportable[i] = exportableMetric{
			Definition: exportableDefinition{
				Name:       m.Definition.Name,
				Type:       m.Definition.Type,
				Help:       m.Definition.Help,
				Labels:     m.Definition.Labels,
				Buckets:    m.Definition.Buckets,
				Objectives: objectives,
			},
			Namespace:   m.Namespace,
			Owner:       m.Owner,
			Description: m.Description,
			Tags:        m.Tags,
			CreatedAt:   m.CreatedAt,
			UpdatedAt:   m.UpdatedAt,
			Enabled:     m.Enabled,
		}
	}
	return json.MarshalIndent(exportable, "", "  ")
}
