package secrets

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// HealthServer provides health check endpoints for the secret broker.
type HealthServer struct {
	broker     *SecretBroker
	metrics    *BrokerMetrics
	rateLimiter *RateLimiter

	mu      sync.RWMutex
	server  *http.Server
	running bool
}

// HealthServerConfig configures the health server.
type HealthServerConfig struct {
	// Port is the HTTP port to listen on.
	Port int `json:"port" yaml:"port"`

	// ReadTimeout is the HTTP read timeout.
	ReadTimeout time.Duration `json:"read_timeout" yaml:"read_timeout"`

	// WriteTimeout is the HTTP write timeout.
	WriteTimeout time.Duration `json:"write_timeout" yaml:"write_timeout"`

	// EnableMetrics enables the /metrics endpoint.
	EnableMetrics bool `json:"enable_metrics" yaml:"enable_metrics"`

	// EnablePprof enables pprof endpoints.
	EnablePprof bool `json:"enable_pprof" yaml:"enable_pprof"`
}

// DefaultHealthServerConfig returns a health server configuration with sensible defaults.
func DefaultHealthServerConfig() *HealthServerConfig {
	return &HealthServerConfig{
		Port:          8080,
		ReadTimeout:   10 * time.Second,
		WriteTimeout:  10 * time.Second,
		EnableMetrics: true,
		EnablePprof:   false,
	}
}

// HealthStatus represents the overall health status.
type HealthStatus struct {
	// Status is the overall health status (healthy, degraded, unhealthy).
	Status string `json:"status"`

	// Timestamp is when the health check was performed.
	Timestamp time.Time `json:"timestamp"`

	// Uptime is how long the broker has been running.
	Uptime time.Duration `json:"uptime"`

	// Version is the broker version.
	Version string `json:"version,omitempty"`

	// Components contains health status for each component.
	Components map[string]*ComponentHealth `json:"components"`

	// Checks contains results of individual health checks.
	Checks []HealthCheck `json:"checks,omitempty"`
}

// ComponentHealth represents the health of a component.
type ComponentHealth struct {
	// Status is the component health status.
	Status string `json:"status"`

	// Message provides additional context.
	Message string `json:"message,omitempty"`

	// LastChecked is when the component was last checked.
	LastChecked time.Time `json:"last_checked"`

	// Metadata contains additional component-specific data.
	Metadata map[string]interface{} `json:"metadata,omitempty"`
}

// HealthCheck represents an individual health check result.
type HealthCheck struct {
	// Name is the check name.
	Name string `json:"name"`

	// Status is the check status (pass, warn, fail).
	Status string `json:"status"`

	// Message provides context for the check result.
	Message string `json:"message,omitempty"`

	// Duration is how long the check took.
	Duration time.Duration `json:"duration"`
}

// NewHealthServer creates a new health server.
func NewHealthServer(broker *SecretBroker, metrics *BrokerMetrics, rateLimiter *RateLimiter) *HealthServer {
	return &HealthServer{
		broker:      broker,
		metrics:     metrics,
		rateLimiter: rateLimiter,
	}
}

// Start starts the health server.
func (h *HealthServer) Start(ctx context.Context, config *HealthServerConfig) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.running {
		return fmt.Errorf("health server already running")
	}

	if config == nil {
		config = DefaultHealthServerConfig()
	}

	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc("/health", h.handleHealth)
	mux.HandleFunc("/health/live", h.handleLiveness)
	mux.HandleFunc("/health/ready", h.handleReadiness)
	mux.HandleFunc("/health/detailed", h.handleDetailedHealth)

	// Stats endpoints
	mux.HandleFunc("/stats", h.handleStats)
	mux.HandleFunc("/stats/backends", h.handleBackendStats)
	mux.HandleFunc("/stats/cache", h.handleCacheStats)
	mux.HandleFunc("/stats/leases", h.handleLeaseStats)

	// Metrics endpoint
	if config.EnableMetrics && h.metrics != nil {
		mux.HandleFunc("/metrics", h.handleMetrics)
	}

	h.server = &http.Server{
		Addr:         fmt.Sprintf(":%d", config.Port),
		Handler:      mux,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	h.running = true

	go func() {
		if err := h.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("health server error: %v\n", err)
		}
	}()

	return nil
}

// Stop stops the health server.
func (h *HealthServer) Stop(ctx context.Context) error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.running || h.server == nil {
		return nil
	}

	h.running = false
	return h.server.Shutdown(ctx)
}

// handleHealth returns overall health status.
func (h *HealthServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := h.checkHealth(ctx)

	w.Header().Set("Content-Type", "application/json")

	if status.Status == "unhealthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	} else if status.Status == "degraded" {
		w.WriteHeader(http.StatusOK) // Still return 200 for degraded
	} else {
		w.WriteHeader(http.StatusOK)
	}

	json.NewEncoder(w).Encode(status)
}

// handleLiveness returns liveness probe status.
func (h *HealthServer) handleLiveness(w http.ResponseWriter, r *http.Request) {
	// Liveness: is the process alive and not deadlocked?
	// This should almost always return OK unless the process is truly broken
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ok",
	})
}

// handleReadiness returns readiness probe status.
func (h *HealthServer) handleReadiness(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Readiness: can we serve requests?
	// Check if at least one backend is healthy
	if h.broker == nil || !h.broker.Healthy(ctx) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "not_ready",
			"message": "no healthy backends available",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status": "ready",
	})
}

// handleDetailedHealth returns detailed health status.
func (h *HealthServer) handleDetailedHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	status := h.checkDetailedHealth(ctx)

	w.Header().Set("Content-Type", "application/json")

	if status.Status == "unhealthy" {
		w.WriteHeader(http.StatusServiceUnavailable)
	}

	json.NewEncoder(w).Encode(status)
}

// handleStats returns broker statistics.
func (h *HealthServer) handleStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	stats := struct {
		Broker    *BrokerStats      `json:"broker,omitempty"`
		Metrics   *MetricsSnapshot  `json:"metrics,omitempty"`
		RateLimit *RateLimitStats   `json:"rate_limit,omitempty"`
	}{}

	if h.broker != nil {
		stats.Broker = h.broker.Stats(ctx)
	}

	if h.metrics != nil {
		stats.Metrics = h.metrics.Snapshot()
	}

	if h.rateLimiter != nil {
		rlStats := h.rateLimiter.Stats()
		stats.RateLimit = &rlStats
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleBackendStats returns per-backend statistics.
func (h *HealthServer) handleBackendStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.broker == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	backendHealth := h.broker.BackendHealth(ctx)
	backends := h.broker.ListBackends()

	stats := make(map[string]interface{})
	for _, name := range backends {
		stats[name] = map[string]interface{}{
			"healthy": backendHealth[name],
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// handleCacheStats returns cache statistics.
func (h *HealthServer) handleCacheStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.broker == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	stats := h.broker.Stats(ctx)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats.CacheStats)
}

// handleLeaseStats returns lease statistics.
func (h *HealthServer) handleLeaseStats(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if h.broker == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	stats := h.broker.Stats(ctx)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats.LeaseStats)
}

// handleMetrics returns Prometheus metrics.
func (h *HealthServer) handleMetrics(w http.ResponseWriter, r *http.Request) {
	if h.metrics == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	exporter := NewPrometheusExporter(h.metrics, "keystone_secrets")
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write([]byte(exporter.Export()))
}

func (h *HealthServer) checkHealth(ctx context.Context) *HealthStatus {
	status := &HealthStatus{
		Status:     "healthy",
		Timestamp:  time.Now(),
		Components: make(map[string]*ComponentHealth),
	}

	// Check broker
	if h.broker == nil {
		status.Status = "unhealthy"
		status.Components["broker"] = &ComponentHealth{
			Status:      "fail",
			Message:     "broker not initialized",
			LastChecked: time.Now(),
		}
	} else {
		brokerHealthy := h.broker.Healthy(ctx)
		if brokerHealthy {
			status.Components["broker"] = &ComponentHealth{
				Status:      "pass",
				LastChecked: time.Now(),
			}
		} else {
			status.Status = "degraded"
			status.Components["broker"] = &ComponentHealth{
				Status:      "warn",
				Message:     "no healthy backends",
				LastChecked: time.Now(),
			}
		}
	}

	return status
}

func (h *HealthServer) checkDetailedHealth(ctx context.Context) *HealthStatus {
	status := h.checkHealth(ctx)
	status.Checks = make([]HealthCheck, 0)

	// Check each backend
	if h.broker != nil {
		backendHealth := h.broker.BackendHealth(ctx)
		for name, healthy := range backendHealth {
			check := HealthCheck{
				Name: fmt.Sprintf("backend_%s", name),
			}
			if healthy {
				check.Status = "pass"
				check.Message = "backend is healthy"
			} else {
				check.Status = "fail"
				check.Message = "backend is unhealthy"
				if status.Status == "healthy" {
					status.Status = "degraded"
				}
			}
			status.Checks = append(status.Checks, check)

			status.Components[name] = &ComponentHealth{
				Status:      check.Status,
				Message:     check.Message,
				LastChecked: time.Now(),
			}
		}

		// Check cache
		brokerStats := h.broker.Stats(ctx)
		if brokerStats.CacheStats != nil {
			cacheCheck := HealthCheck{
				Name:   "cache",
				Status: "pass",
			}
			status.Components["cache"] = &ComponentHealth{
				Status:      "pass",
				LastChecked: time.Now(),
				Metadata: map[string]interface{}{
					"entries":   brokerStats.CacheStats.Entries,
					"hit_rate":  brokerStats.CacheStats.HitRate(),
				},
			}
			status.Checks = append(status.Checks, cacheCheck)
		}

		// Check lease manager
		if brokerStats.LeaseStats != nil {
			leaseCheck := HealthCheck{
				Name:   "lease_manager",
				Status: "pass",
			}
			status.Components["lease_manager"] = &ComponentHealth{
				Status:      "pass",
				LastChecked: time.Now(),
				Metadata: map[string]interface{}{
					"active_leases":   brokerStats.LeaseStats.ActiveLeases,
					"expiring_leases": brokerStats.LeaseStats.ExpiringLeases,
				},
			}
			status.Checks = append(status.Checks, leaseCheck)
		}
	}

	// Check rate limiter
	if h.rateLimiter != nil {
		rlStats := h.rateLimiter.Stats()
		rlCheck := HealthCheck{
			Name:   "rate_limiter",
			Status: "pass",
		}
		if rlStats.RejectedRequests > 0 {
			rejectionRate := float64(rlStats.RejectedRequests) / float64(rlStats.TotalRequests) * 100
			if rejectionRate > 10 {
				rlCheck.Status = "warn"
				rlCheck.Message = fmt.Sprintf("high rejection rate: %.1f%%", rejectionRate)
			}
		}
		status.Components["rate_limiter"] = &ComponentHealth{
			Status:      rlCheck.Status,
			Message:     rlCheck.Message,
			LastChecked: time.Now(),
			Metadata: map[string]interface{}{
				"total_requests":    rlStats.TotalRequests,
				"rejected_requests": rlStats.RejectedRequests,
				"active_buckets":    rlStats.ActiveBuckets,
			},
		}
		status.Checks = append(status.Checks, rlCheck)
	}

	return status
}

// =============================================================================
// Health Check Client
// =============================================================================

// HealthClient provides a client for checking broker health.
type HealthClient struct {
	baseURL string
	client  *http.Client
}

// NewHealthClient creates a new health client.
func NewHealthClient(baseURL string) *HealthClient {
	return &HealthClient{
		baseURL: baseURL,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// CheckHealth checks the broker health.
func (c *HealthClient) CheckHealth(ctx context.Context) (*HealthStatus, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status HealthStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	return &status, nil
}

// CheckLiveness checks the broker liveness.
func (c *HealthClient) CheckLiveness(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health/live", nil)
	if err != nil {
		return false, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// CheckReadiness checks the broker readiness.
func (c *HealthClient) CheckReadiness(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/health/ready", nil)
	if err != nil {
		return false, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// GetStats retrieves broker statistics.
func (c *HealthClient) GetStats(ctx context.Context) (map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/stats", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var stats map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, err
	}

	return stats, nil
}
