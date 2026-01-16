package events

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// HTTPReceiverConfig holds HTTP receiver configuration
type HTTPReceiverConfig struct {
	// Server configuration
	Address      string
	Port         int
	Path         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration

	// Security
	Secret            string
	SignatureHeader   string
	SignatureRequired bool

	// Format
	AcceptCloudEvents bool
	AcceptNative      bool

	// Rate limiting
	MaxRequestsPerSecond int
}

// DefaultHTTPReceiverConfig returns default HTTP receiver configuration
func DefaultHTTPReceiverConfig() *HTTPReceiverConfig {
	return &HTTPReceiverConfig{
		Address:              "0.0.0.0",
		Port:                 8080,
		Path:                 "/events",
		ReadTimeout:          30 * time.Second,
		WriteTimeout:         30 * time.Second,
		SignatureHeader:      "X-Keystone-Core-Signature",
		SignatureRequired:    false,
		AcceptCloudEvents:    true,
		AcceptNative:         true,
		MaxRequestsPerSecond: 1000,
	}
}

// HTTPReceiver receives events via HTTP
type HTTPReceiver struct {
	config    *HTTPReceiverConfig
	publisher EventPublisher
	server    *http.Server
	mu        sync.RWMutex
	running   bool

	// Metrics
	eventsReceived int64
	eventsFailed   int64
	lastEvent      time.Time
}

// NewHTTPReceiver creates a new HTTP event receiver
func NewHTTPReceiver(config *HTTPReceiverConfig, publisher EventPublisher) (*HTTPReceiver, error) {
	if config == nil {
		config = DefaultHTTPReceiverConfig()
	}

	if publisher == nil {
		return nil, fmt.Errorf("publisher is required")
	}

	receiver := &HTTPReceiver{
		config:    config,
		publisher: publisher,
	}

	// Create HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc(config.Path, receiver.handleEvent)
	mux.HandleFunc(config.Path+"/batch", receiver.handleBatch)
	mux.HandleFunc("/health", receiver.handleHealth)
	mux.HandleFunc("/metrics", receiver.handleMetrics)

	receiver.server = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", config.Address, config.Port),
		Handler:      mux,
		ReadTimeout:  config.ReadTimeout,
		WriteTimeout: config.WriteTimeout,
	}

	return receiver, nil
}

// Start starts the HTTP receiver
func (r *HTTPReceiver) Start() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.running {
		return fmt.Errorf("receiver is already running")
	}

	r.running = true

	// Start server in background
	go func() {
		if err := r.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("events http receiver error: %v", err)
		}
	}()

	return nil
}

// Stop stops the HTTP receiver
func (r *HTTPReceiver) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	r.running = false
	return r.server.Close()
}

// IsRunning returns whether the receiver is running
func (r *HTTPReceiver) IsRunning() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.running
}

// handleEvent handles single event POST
func (r *HTTPReceiver) handleEvent(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Verify signature if required
	if r.config.SignatureRequired {
		if !r.verifySignature(req, body) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Determine content type
	contentType := req.Header.Get("Content-Type")
	isCloudEvent := strings.Contains(contentType, "cloudevents") ||
		req.Header.Get("Ce-Specversion") != "" // CloudEvents binary mode

	var event *Event

	if isCloudEvent {
		if !r.config.AcceptCloudEvents {
			http.Error(w, "CloudEvents format not accepted", http.StatusBadRequest)
			return
		}

		// Parse as CloudEvent
		var ce CloudEvent
		if err := json.Unmarshal(body, &ce); err != nil {
			http.Error(w, "Failed to parse CloudEvent", http.StatusBadRequest)
			return
		}

		event, err = FromCloudEvent(&ce)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to convert CloudEvent: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		if !r.config.AcceptNative {
			http.Error(w, "Native format not accepted", http.StatusBadRequest)
			return
		}

		// Parse as native event
		if err := json.Unmarshal(body, &event); err != nil {
			http.Error(w, "Failed to parse event", http.StatusBadRequest)
			return
		}
	}

	// Publish event
	if err := r.publisher.PublishAsync(event); err != nil {
		r.eventsFailed++
		http.Error(w, fmt.Sprintf("Failed to publish event: %v", err), http.StatusInternalServerError)
		return
	}

	r.eventsReceived++
	r.lastEvent = time.Now()

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"status":"accepted"}`))
}

// handleBatch handles batch event POST
func (r *HTTPReceiver) handleBatch(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Verify signature if required
	if r.config.SignatureRequired {
		if !r.verifySignature(req, body) {
			http.Error(w, "Invalid signature", http.StatusUnauthorized)
			return
		}
	}

	// Determine content type
	contentType := req.Header.Get("Content-Type")
	isCloudEvent := strings.Contains(contentType, "cloudevents")

	var events []*Event

	if isCloudEvent {
		if !r.config.AcceptCloudEvents {
			http.Error(w, "CloudEvents format not accepted", http.StatusBadRequest)
			return
		}

		// Parse as CloudEvent batch
		var batch CloudEventBatch
		if err := json.Unmarshal(body, &batch); err != nil {
			http.Error(w, "Failed to parse CloudEvent batch", http.StatusBadRequest)
			return
		}

		events, err = FromCloudEventBatch(&batch)
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to convert CloudEvent batch: %v", err), http.StatusBadRequest)
			return
		}
	} else {
		if !r.config.AcceptNative {
			http.Error(w, "Native format not accepted", http.StatusBadRequest)
			return
		}

		// Parse as native event array
		if err := json.Unmarshal(body, &events); err != nil {
			http.Error(w, "Failed to parse event batch", http.StatusBadRequest)
			return
		}
	}

	// Publish all events
	var failed int
	for _, event := range events {
		if err := r.publisher.PublishAsync(event); err != nil {
			failed++
		}
	}

	r.eventsReceived += int64(len(events) - failed)
	r.eventsFailed += int64(failed)
	r.lastEvent = time.Now()

	if failed > 0 {
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte(fmt.Sprintf(`{"status":"partial","accepted":%d,"failed":%d}`,
			len(events)-failed, failed)))
	} else {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(fmt.Sprintf(`{"status":"accepted","count":%d}`, len(events))))
	}
}

// handleHealth handles health check
func (r *HTTPReceiver) handleHealth(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	status := "unhealthy"
	if r.IsRunning() {
		status = "healthy"
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(fmt.Sprintf(`{"status":"%s"}`, status)))
}

// handleMetrics handles metrics endpoint
func (r *HTTPReceiver) handleMetrics(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	metrics := map[string]interface{}{
		"events_received": r.eventsReceived,
		"events_failed":   r.eventsFailed,
		"last_event":      r.lastEvent.Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}

// verifySignature verifies HMAC signature
func (r *HTTPReceiver) verifySignature(req *http.Request, body []byte) bool {
	if r.config.Secret == "" {
		return true
	}

	signature := req.Header.Get(r.config.SignatureHeader)
	if signature == "" {
		return false
	}

	// Calculate expected signature
	mac := hmac.New(sha256.New, []byte(r.config.Secret))
	mac.Write(body)
	expectedSig := "sha256=" + hex.EncodeToString(mac.Sum(nil))

	return hmac.Equal([]byte(signature), []byte(expectedSig))
}

// HTTPSender sends events via HTTP POST
type HTTPSender struct {
	url        string
	secret     string
	httpClient *http.Client
	useCloudEvents bool
}

// NewHTTPSender creates a new HTTP event sender
func NewHTTPSender(url string, secret string, useCloudEvents bool) *HTTPSender {
	return &HTTPSender{
		url:            url,
		secret:         secret,
		useCloudEvents: useCloudEvents,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Send sends a single event via HTTP
func (s *HTTPSender) Send(event *Event) error {
	var body []byte
	var contentType string
	var err error

	if s.useCloudEvents {
		ce := ToCloudEvent(event)
		body, err = json.Marshal(ce)
		contentType = "application/cloudevents+json"
	} else {
		body, err = json.Marshal(event)
		contentType = "application/json"
	}

	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	req, err := http.NewRequest("POST", s.url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)

	// Add signature if secret is configured
	if s.secret != "" {
		mac := hmac.New(sha256.New, []byte(s.secret))
		mac.Write(body)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Keystone-Core-Signature", signature)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// SendBatch sends multiple events via HTTP
func (s *HTTPSender) SendBatch(events []*Event) error {
	var body []byte
	var contentType string
	var err error

	if s.useCloudEvents {
		batch := ToCloudEventBatch(events)
		body, err = json.Marshal(batch)
		contentType = "application/cloudevents+json"
	} else {
		body, err = json.Marshal(events)
		contentType = "application/json"
	}

	if err != nil {
		return fmt.Errorf("failed to marshal events: %w", err)
	}

	url := s.url
	if !strings.HasSuffix(url, "/batch") {
		url += "/batch"
	}

	req, err := http.NewRequest("POST", url, strings.NewReader(string(body)))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", contentType)

	// Add signature if secret is configured
	if s.secret != "" {
		mac := hmac.New(sha256.New, []byte(s.secret))
		mac.Write(body)
		signature := "sha256=" + hex.EncodeToString(mac.Sum(nil))
		req.Header.Set("X-Keystone-Core-Signature", signature)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("HTTP request failed with status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}
