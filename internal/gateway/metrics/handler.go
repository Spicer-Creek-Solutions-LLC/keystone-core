package metrics

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/common/expfmt"
)

// Handler provides HTTP handlers for metrics endpoints.
type Handler struct {
	store *Store

	// Label manipulation
	addLabels     map[string]string
	dropLabels    []string
	rewriteLabels map[string]string
}

// NewHandler creates a new metrics handler.
func NewHandler(store *Store) *Handler {
	return &Handler{
		store:         store,
		addLabels:     make(map[string]string),
		rewriteLabels: make(map[string]string),
	}
}

// SetAddLabels sets labels to add to all metrics.
func (h *Handler) SetAddLabels(labels map[string]string) {
	h.addLabels = labels
}

// SetDropLabels sets labels to drop from all metrics.
func (h *Handler) SetDropLabels(labels []string) {
	h.dropLabels = labels
}

// SetRewriteLabels sets label rewrite rules.
func (h *Handler) SetRewriteLabels(rules map[string]string) {
	h.rewriteLabels = rules
}

// ServeHTTP implements http.Handler for /metrics endpoint.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	families := h.store.GetAllFamilies()

	// Apply label transformations
	for _, family := range families {
		for _, m := range family.Metric {
			h.transformLabels(m)
		}
	}

	// Determine content type
	contentType := expfmt.Negotiate(r.Header)
	w.Header().Set("Content-Type", string(contentType))

	// Encode metrics
	encoder := expfmt.NewEncoder(w, contentType)
	for _, family := range families {
		if err := encoder.Encode(family); err != nil {
			http.Error(w, fmt.Sprintf("encoding error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// transformLabels applies label transformations to a metric.
func (h *Handler) transformLabels(m *dto.Metric) {
	// Build label map for easier manipulation
	labels := make(map[string]string)
	for _, l := range m.Label {
		if l.Name != nil && l.Value != nil {
			labels[*l.Name] = *l.Value
		}
	}

	// Drop labels
	for _, drop := range h.dropLabels {
		delete(labels, drop)
	}

	// Rewrite labels
	for source, target := range h.rewriteLabels {
		if val, exists := labels[source]; exists {
			labels[target] = val
			delete(labels, source)
		}
	}

	// Add labels
	for k, v := range h.addLabels {
		labels[k] = v
	}

	// Rebuild label slice
	m.Label = make([]*dto.LabelPair, 0, len(labels))
	for k, v := range labels {
		key := k
		val := v
		m.Label = append(m.Label, &dto.LabelPair{
			Name:  &key,
			Value: &val,
		})
	}

	// Sort labels by name for consistent output
	sort.Slice(m.Label, func(i, j int) bool {
		return *m.Label[i].Name < *m.Label[j].Name
	})
}

// FederateHandler provides the /federate endpoint for Prometheus federation.
type FederateHandler struct {
	store *Store
}

// NewFederateHandler creates a new federate handler.
func NewFederateHandler(store *Store) *FederateHandler {
	return &FederateHandler{store: store}
}

// ServeHTTP implements http.Handler for /federate endpoint.
func (h *FederateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse match[] parameters
	matches := r.URL.Query()["match[]"]

	families := h.store.GetAllFamilies()

	// Filter families based on match[] parameters
	if len(matches) > 0 {
		families = h.filterFamilies(families, matches)
	}

	// Add federation metadata
	for _, family := range families {
		for _, m := range family.Metric {
			// Add timestamp if not present
			if m.TimestampMs == nil {
				ts := time.Now().UnixMilli()
				m.TimestampMs = &ts
			}
		}
	}

	// Determine content type
	contentType := expfmt.Negotiate(r.Header)
	w.Header().Set("Content-Type", string(contentType))

	// Encode metrics
	encoder := expfmt.NewEncoder(w, contentType)
	for _, family := range families {
		if err := encoder.Encode(family); err != nil {
			http.Error(w, fmt.Sprintf("encoding error: %v", err), http.StatusInternalServerError)
			return
		}
	}
}

// filterFamilies filters metric families based on match expressions.
func (h *FederateHandler) filterFamilies(families []*dto.MetricFamily, matches []string) []*dto.MetricFamily {
	var result []*dto.MetricFamily

	for _, family := range families {
		if family.Name == nil {
			continue
		}

		for _, match := range matches {
			if h.matchesSelector(family, match) {
				result = append(result, family)
				break
			}
		}
	}

	return result
}

// matchesSelector checks if a metric family matches a selector.
// Supports simple selectors like: {job="foo"} or metric_name{label="value"}
func (h *FederateHandler) matchesSelector(family *dto.MetricFamily, selector string) bool {
	selector = strings.TrimSpace(selector)

	// Parse metric name and label matchers
	var metricName string
	var labelMatchers map[string]string

	if idx := strings.Index(selector, "{"); idx >= 0 {
		metricName = selector[:idx]
		labelPart := strings.Trim(selector[idx:], "{}")
		labelMatchers = h.parseLabelMatchers(labelPart)
	} else {
		metricName = selector
	}

	// Check metric name
	if metricName != "" && *family.Name != metricName {
		// Check for regex match (simple prefix/suffix)
		if !strings.HasPrefix(metricName, ".*") && !strings.HasSuffix(metricName, ".*") {
			return false
		}
	}

	// Check label matchers against any metric in the family
	if len(labelMatchers) > 0 {
		for _, m := range family.Metric {
			if h.metricsMatchLabels(m, labelMatchers) {
				return true
			}
		}
		return false
	}

	return true
}

// parseLabelMatchers parses label matchers from a string like: label1="value1",label2="value2"
func (h *FederateHandler) parseLabelMatchers(s string) map[string]string {
	result := make(map[string]string)
	pairs := strings.Split(s, ",")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if idx := strings.Index(pair, "="); idx > 0 {
			key := strings.TrimSpace(pair[:idx])
			value := strings.Trim(strings.TrimSpace(pair[idx+1:]), "\"")
			result[key] = value
		}
	}
	return result
}

// metricsMatchLabels checks if a metric matches label matchers.
func (h *FederateHandler) metricsMatchLabels(m *dto.Metric, matchers map[string]string) bool {
	labels := make(map[string]string)
	for _, l := range m.Label {
		if l.Name != nil && l.Value != nil {
			labels[*l.Name] = *l.Value
		}
	}

	for k, v := range matchers {
		if labels[k] != v {
			return false
		}
	}
	return true
}

// HealthHandler provides health check endpoints.
type HealthHandler struct {
	store      *Store
	subscriber *Subscriber
}

// NewHealthHandler creates a new health handler.
func NewHealthHandler(store *Store, subscriber *Subscriber) *HealthHandler {
	return &HealthHandler{
		store:      store,
		subscriber: subscriber,
	}
}

// HealthResponse represents a health check response.
type HealthResponse struct {
	Status    string    `json:"status"`
	Agents    int       `json:"agents"`
	Series    int       `json:"series"`
	Timestamp time.Time `json:"timestamp"`
}

// ServeHTTP implements http.Handler for /health endpoint.
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	stats := h.store.Stats()

	status := "healthy"
	if stats.AgentCount == 0 {
		status = "degraded"
	}

	response := HealthResponse{
		Status:    status,
		Agents:    stats.AgentCount,
		Series:    stats.TotalSeries,
		Timestamp: time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	if status == "healthy" {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("failed to write health response: %v", err)
	}
}

// ReadyHandler provides readiness check endpoint.
type ReadyHandler struct {
	subscriber *Subscriber
}

// NewReadyHandler creates a new readiness handler.
func NewReadyHandler(subscriber *Subscriber) *ReadyHandler {
	return &ReadyHandler{subscriber: subscriber}
}

// ServeHTTP implements http.Handler for /ready endpoint.
func (h *ReadyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Check if subscriber is receiving messages
	stats := h.subscriber.Stats()

	if stats.MessagesReceived > 0 || stats.LastError == nil {
		w.WriteHeader(http.StatusOK)
		if _, err := io.WriteString(w, "ready"); err != nil { // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- plain text readiness response
			log.Printf("failed to write readiness response: %v", err)
		}
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
		if _, err := io.WriteString(w, "not ready"); err != nil { // nosemgrep: go.lang.security.audit.xss.no-direct-write-to-responsewriter.no-direct-write-to-responsewriter -- plain text readiness response
			log.Printf("failed to write readiness response: %v", err)
		}
	}
}
