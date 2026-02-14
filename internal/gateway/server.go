// Package gateway provides telemetry gateway services that aggregate metrics, logs,
// and traces from agents over NATS and expose them to observability backends.
package gateway

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/nats-io/nats.go"

	"github.com/shawnbutts/keystone-core/internal/gateway/logs"
	"github.com/shawnbutts/keystone-core/internal/gateway/metrics"
	"github.com/shawnbutts/keystone-core/internal/gateway/traces"
)

// Server is the telemetry gateway server.
type Server struct {
	config Config
	nc     *nats.Conn

	// Metrics components
	metricsStore      *metrics.Store
	metricsSubscriber *metrics.Subscriber
	metricsHandler    *metrics.Handler
	remoteWriter      *metrics.RemoteWriter

	// Logs components
	logsStore      *logs.Store
	logsSubscriber *logs.Subscriber
	lokiPusher     *logs.LokiPusher

	// Traces components
	tracesStore      *traces.Store
	tracesSubscriber *traces.Subscriber
	otlpExporter     *traces.OTLPExporter

	// HTTP server
	httpServer *http.Server

	// Stale cleanup
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServer creates a new telemetry gateway server.
func NewServer(config *Config) *Server {
	if config == nil {
		config = DefaultConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Server{
		config: *config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the telemetry gateway server.
func (s *Server) Start() error {
	// Connect to NATS
	if err := s.connectNATS(); err != nil {
		return fmt.Errorf("failed to connect to NATS: %w", err)
	}

	// Initialize metrics gateway
	if s.config.Metrics.Enabled {
		if err := s.initMetrics(); err != nil {
			return fmt.Errorf("failed to initialize metrics gateway: %w", err)
		}
	}

	// Initialize logs gateway
	if s.config.Logs.Enabled {
		if err := s.initLogs(); err != nil {
			return fmt.Errorf("failed to initialize logs gateway: %w", err)
		}
	}

	// Initialize traces gateway
	if s.config.Traces.Enabled {
		if err := s.initTraces(); err != nil {
			return fmt.Errorf("failed to initialize traces gateway: %w", err)
		}
	}

	// Start HTTP server
	if err := s.startHTTP(); err != nil {
		return fmt.Errorf("failed to start HTTP server: %w", err)
	}

	// Start stale cleanup
	s.wg.Add(1)
	go s.cleanupLoop()

	log.Printf("Telemetry gateway started on %s", s.config.Server.Listen)
	return nil
}

// Stop stops the telemetry gateway server.
func (s *Server) Stop() error {
	s.cancel()

	// Stop HTTP server
	if s.httpServer != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := s.httpServer.Shutdown(ctx); err != nil {
			log.Printf("Warning: HTTP server shutdown error: %v", err)
		}
	}

	// Stop metrics components
	if s.metricsSubscriber != nil {
		s.metricsSubscriber.Stop()
	}
	if s.remoteWriter != nil {
		s.remoteWriter.Stop()
	}

	// Stop logs components
	if s.logsSubscriber != nil {
		s.logsSubscriber.Stop()
	}
	if s.lokiPusher != nil {
		s.lokiPusher.Stop()
	}

	// Stop traces components
	if s.tracesSubscriber != nil {
		s.tracesSubscriber.Stop()
	}
	if s.otlpExporter != nil {
		s.otlpExporter.Stop()
	}

	// Close NATS
	if s.nc != nil {
		s.nc.Close()
	}

	s.wg.Wait()

	log.Printf("Telemetry gateway stopped")
	return nil
}

// connectNATS connects to NATS.
func (s *Server) connectNATS() error {
	opts := []nats.Option{
		nats.MaxReconnects(s.config.NATS.MaxReconnects),
		nats.ReconnectWait(s.config.NATS.ReconnectWait),
		nats.ReconnectJitter(s.config.NATS.ReconnectJitter, s.config.NATS.ReconnectJitter),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Printf("NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("NATS reconnected to %s", nc.ConnectedUrl())
		}),
	}

	// Add TLS configuration if enabled
	if s.config.NATS.TLS.Enabled {
		tlsConfig := &tls.Config{MinVersion: tls.VersionTLS13}
		if s.config.NATS.TLS.MinVersion == "1.2" {
			tlsConfig.MinVersion = tls.VersionTLS12
		}
		if s.config.NATS.TLS.CertFile != "" && s.config.NATS.TLS.KeyFile != "" {
			cert, err := tls.LoadX509KeyPair(s.config.NATS.TLS.CertFile, s.config.NATS.TLS.KeyFile)
			if err != nil {
				return fmt.Errorf("loading NATS TLS certificate: %w", err)
			}
			tlsConfig.Certificates = []tls.Certificate{cert}
		}
		if s.config.NATS.TLS.CAFile != "" {
			caCert, err := os.ReadFile(s.config.NATS.TLS.CAFile)
			if err != nil {
				return fmt.Errorf("reading NATS CA certificate: %w", err)
			}
			caPool := x509.NewCertPool()
			if !caPool.AppendCertsFromPEM(caCert) {
				return fmt.Errorf("failed to parse NATS CA certificate")
			}
			tlsConfig.RootCAs = caPool
		}
		if s.config.NATS.TLS.Insecure {
			tlsConfig.InsecureSkipVerify = true //nolint:gosec // user-configured insecure mode
		}
		opts = append(opts, nats.Secure(tlsConfig))
	}

	// Add credentials if configured
	if s.config.NATS.CredentialsFile != "" {
		opts = append(opts, nats.UserCredentials(s.config.NATS.CredentialsFile))
	}

	// Build URL list
	urls := s.config.NATS.URLs
	if len(urls) == 0 {
		urls = []string{"nats://localhost:4222"}
	}

	var err error
	for _, url := range urls {
		s.nc, err = nats.Connect(url, opts...)
		if err == nil {
			log.Printf("Connected to NATS at %s", url)
			return nil
		}
	}

	return fmt.Errorf("failed to connect to any NATS server: %w", err)
}

// initMetrics initializes the metrics gateway.
func (s *Server) initMetrics() error {
	// Create metrics store
	storeConfig := metrics.StoreConfig{
		MaxAge:                   s.config.Metrics.StaleTimeout,
		MaxSeries:                s.config.Metrics.Cardinality.MaxSeries,
		MaxLabelsPerSeries:       s.config.Metrics.Cardinality.MaxLabelsPerSeries,
		DropHighCardinality:      s.config.Metrics.Cardinality.DropHighCardinality,
		HighCardinalityThreshold: 10000,
	}
	s.metricsStore = metrics.NewMetricsStore(storeConfig)

	// Create subscriber
	subConfig := metrics.SubscriberConfig{
		Subject:        s.config.Metrics.Subject,
		QueueGroup:     s.config.HA.QueueGroup,
		BufferSize:     1024,
		ProcessTimeout: 5 * time.Second,
	}
	s.metricsSubscriber = metrics.NewSubscriber(s.nc, s.metricsStore, subConfig)

	// Start subscriber
	if err := s.metricsSubscriber.Start(); err != nil {
		return fmt.Errorf("failed to start metrics subscriber: %w", err)
	}

	// Create handler
	s.metricsHandler = metrics.NewHandler(s.metricsStore)
	s.metricsHandler.SetAddLabels(s.config.Metrics.Labels.Add)
	s.metricsHandler.SetDropLabels(s.config.Metrics.Labels.Drop)

	// Build rewrite map
	rewriteMap := make(map[string]string)
	for _, r := range s.config.Metrics.Labels.Rewrite {
		rewriteMap[r.Source] = r.Target
	}
	s.metricsHandler.SetRewriteLabels(rewriteMap)

	// Create remote writer if configured
	if s.config.Metrics.RemoteWrite.Enabled && s.config.Metrics.RemoteWrite.URL != "" {
		rwConfig := metrics.RemoteWriteConfig{
			URL:             s.config.Metrics.RemoteWrite.URL,
			BatchSize:       s.config.Metrics.RemoteWrite.BatchSize,
			FlushInterval:   s.config.Metrics.RemoteWrite.FlushInterval,
			Timeout:         30 * time.Second,
			MaxRetries:      s.config.Metrics.RemoteWrite.Retry.MaxAttempts,
			RetryBackoff:    s.config.Metrics.RemoteWrite.Retry.Backoff,
			MaxRetryBackoff: s.config.Metrics.RemoteWrite.Retry.MaxBackoff,
		}

		// Configure auth
		switch s.config.Metrics.RemoteWrite.Auth.Type {
		case "basic":
			rwConfig.BasicAuth = &metrics.BasicAuth{
				Username: s.config.Metrics.RemoteWrite.Auth.Username,
				Password: s.config.Metrics.RemoteWrite.Auth.Password,
			}
		case "bearer":
			rwConfig.BearerToken = s.config.Metrics.RemoteWrite.Auth.Token
		}

		s.remoteWriter = metrics.NewRemoteWriter(s.metricsStore, rwConfig)
		if err := s.remoteWriter.Start(); err != nil {
			log.Printf("Warning: failed to start remote writer: %v", err)
		}
	}

	log.Printf("Metrics gateway initialized")
	return nil
}

// initLogs initializes the logs gateway.
func (s *Server) initLogs() error {
	// Create logs store
	storeConfig := logs.StoreConfig{
		MaxEntries:     100000,
		MaxAge:         1 * time.Hour,
		MinLevel:       s.config.Logs.MinLevel,
		IncludeSources: s.config.Logs.Sources.Include,
		ExcludeSources: s.config.Logs.Sources.Exclude,
	}
	s.logsStore = logs.NewLogsStore(storeConfig)

	// Create subscriber
	subConfig := logs.SubscriberConfig{
		Subject:        s.config.Logs.Subject,
		QueueGroup:     s.config.HA.QueueGroup,
		BufferSize:     1024,
		ProcessTimeout: 5 * time.Second,
	}
	s.logsSubscriber = logs.NewSubscriber(s.nc, s.logsStore, subConfig)

	// Start subscriber
	if err := s.logsSubscriber.Start(); err != nil {
		return fmt.Errorf("failed to start logs subscriber: %w", err)
	}

	// Create Loki pusher if configured
	if s.config.Logs.Loki.Enabled && s.config.Logs.Loki.URL != "" {
		lokiConfig := logs.LokiConfig{
			URL:       s.config.Logs.Loki.URL,
			TenantID:  s.config.Logs.Loki.TenantID,
			BatchSize: s.config.Logs.Loki.BatchSize,
			BatchWait: s.config.Logs.Loki.BatchWait,
			Labels:    s.config.Logs.Loki.Labels,
		}
		s.lokiPusher = logs.NewLokiPusher(s.logsStore, lokiConfig)
		if err := s.lokiPusher.Start(); err != nil {
			log.Printf("Warning: failed to start Loki pusher: %v", err)
		}
	}

	log.Printf("Logs gateway initialized")
	return nil
}

// initTraces initializes the traces gateway.
func (s *Server) initTraces() error {
	// Create traces store
	storeConfig := traces.StoreConfig{
		MaxTraces:     10000,
		MaxAge:        1 * time.Hour,
		SamplingRate:  s.config.Traces.Sampling.Rate,
		SampleErrors:  s.config.Traces.Sampling.PrioritySample.Errors,
		SlowThreshold: s.config.Traces.Sampling.PrioritySample.SlowThreshold,
	}
	s.tracesStore = traces.NewTracesStore(storeConfig)

	// Create subscriber
	subConfig := traces.SubscriberConfig{
		Subject:        s.config.Traces.Subject,
		QueueGroup:     s.config.HA.QueueGroup,
		BufferSize:     1024,
		ProcessTimeout: 5 * time.Second,
	}
	s.tracesSubscriber = traces.NewSubscriber(s.nc, s.tracesStore, subConfig)

	// Start subscriber
	if err := s.tracesSubscriber.Start(); err != nil {
		return fmt.Errorf("failed to start traces subscriber: %w", err)
	}

	// Create OTLP exporter if configured
	if s.config.Traces.OTLP.Enabled && s.config.Traces.OTLP.Endpoint != "" {
		otlpConfig := traces.OTLPConfig{
			Endpoint:      s.config.Traces.OTLP.Endpoint,
			Protocol:      s.config.Traces.OTLP.Protocol,
			Compression:   s.config.Traces.OTLP.Compression,
			BatchSize:     s.config.Traces.OTLP.BatchSize,
			FlushInterval: s.config.Traces.OTLP.FlushInterval,
			Headers:       s.config.Traces.OTLP.Headers,
		}
		s.otlpExporter = traces.NewOTLPExporter(s.tracesStore, otlpConfig)
		if err := s.otlpExporter.Start(); err != nil {
			log.Printf("Warning: failed to start OTLP exporter: %v", err)
		}
	}

	log.Printf("Traces gateway initialized")
	return nil
}

// startHTTP starts the HTTP server.
func (s *Server) startHTTP() error {
	mux := http.NewServeMux()

	// Health endpoints
	mux.HandleFunc(s.config.Server.HealthPath, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	mux.HandleFunc(s.config.Server.ReadyPath, func(w http.ResponseWriter, r *http.Request) {
		// Check if we have active subscriptions
		ready := (!s.config.Metrics.Enabled || s.metricsSubscriber != nil) &&
			(!s.config.Logs.Enabled || s.logsSubscriber != nil) &&
			(!s.config.Traces.Enabled || s.tracesSubscriber != nil)
		if ready {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("ready"))
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
		}
	})

	// Metrics endpoints
	if s.config.Metrics.Enabled {
		mux.Handle(s.config.Server.MetricsPath, s.metricsHandler)
		if s.config.Metrics.Federation.Enabled {
			federateHandler := metrics.NewFederateHandler(s.metricsStore)
			mux.Handle(s.config.Server.FederatePath, federateHandler)
		}

		// Health handler with stats
		healthHandler := metrics.NewHealthHandler(s.metricsStore, s.metricsSubscriber)
		mux.Handle("/metrics/health", healthHandler)
	}

	s.httpServer = &http.Server{
		Addr:         s.config.Server.Listen,
		Handler:      mux,
		ReadTimeout:  s.config.Server.ReadTimeout,
		WriteTimeout: s.config.Server.WriteTimeout,
	}

	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("HTTP server error: %v", err)
		}
	}()

	return nil
}

// cleanupLoop periodically cleans up stale data.
func (s *Server) cleanupLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-ticker.C:
			if s.metricsStore != nil {
				removed := s.metricsStore.RemoveStale()
				if len(removed) > 0 {
					log.Printf("Removed %d stale agents from metrics store", len(removed))
				}
			}
			if s.logsStore != nil {
				s.logsStore.Cleanup()
			}
			if s.tracesStore != nil {
				s.tracesStore.Cleanup()
			}
		}
	}
}

// ServerStats contains gateway server statistics.
type ServerStats struct {
	Metrics *MetricsStats
	Logs    *LogsStats
	Traces  *TracesStats
}

// MetricsStats holds metrics gateway statistics.
type MetricsStats struct {
	AgentCount   int
	SeriesCount  int
	MessagesRecv int64
	BytesRecv    int64
}

// LogsStats holds logs gateway statistics.
type LogsStats struct {
	EntryCount   int
	MessagesRecv int64
	BytesRecv    int64
}

// TracesStats holds traces gateway statistics.
type TracesStats struct {
	TraceCount   int
	MessagesRecv int64
	BytesRecv    int64
}

// Stats returns current gateway statistics.
func (s *Server) Stats() ServerStats {
	stats := ServerStats{}

	if s.metricsStore != nil && s.metricsSubscriber != nil {
		storeStats := s.metricsStore.Stats()
		subStats := s.metricsSubscriber.Stats()
		stats.Metrics = &MetricsStats{
			AgentCount:   storeStats.AgentCount,
			SeriesCount:  storeStats.TotalSeries,
			MessagesRecv: subStats.MessagesReceived,
			BytesRecv:    subStats.BytesReceived,
		}
	}

	if s.logsStore != nil && s.logsSubscriber != nil {
		storeStats := s.logsStore.Stats()
		subStats := s.logsSubscriber.Stats()
		stats.Logs = &LogsStats{
			EntryCount:   storeStats.EntryCount,
			MessagesRecv: subStats.MessagesReceived,
			BytesRecv:    subStats.BytesReceived,
		}
	}

	if s.tracesStore != nil && s.tracesSubscriber != nil {
		storeStats := s.tracesStore.Stats()
		subStats := s.tracesSubscriber.Stats()
		stats.Traces = &TracesStats{
			TraceCount:   storeStats.TraceCount,
			MessagesRecv: subStats.MessagesReceived,
			BytesRecv:    subStats.BytesReceived,
		}
	}

	return stats
}
