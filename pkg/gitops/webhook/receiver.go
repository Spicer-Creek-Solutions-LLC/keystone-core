package webhook

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/events"
)

// EventProcessor handles webhook events
type EventProcessor interface {
	ProcessEvent(ctx context.Context, event *WebhookEvent) error
}

// Receiver receives and processes webhooks
type Receiver struct {
	config     *WebhookConfig
	auth       Authenticator
	registry   *HandlerRegistry
	processor  EventProcessor
	server     *http.Server
	mu         sync.RWMutex
	stats      *ReceiverStats
}

// ReceiverStats tracks webhook receiver statistics
type ReceiverStats struct {
	mu                sync.RWMutex
	TotalReceived     int64
	TotalProcessed    int64
	TotalFailed       int64
	ByType            map[WebhookType]int64
	LastReceivedTime  time.Time
	LastProcessedTime time.Time
}

// NewReceiver creates a new webhook receiver
func NewReceiver(config *WebhookConfig, processor EventProcessor) *Receiver {
	if config == nil {
		config = DefaultWebhookConfig()
	}

	// Create handler registry
	registry := NewHandlerRegistry()

	// Register default handlers
	for _, handlerType := range config.Handlers {
		switch WebhookType(handlerType) {
		case WebhookTypeArgoCD:
			registry.Register(&ArgoCDHandler{})
		case WebhookTypeFlux:
			registry.Register(&FluxHandler{})
		case WebhookTypeGitHub:
			registry.Register(&GitHubHandler{})
		case WebhookTypeGitLab:
			registry.Register(&GitLabHandler{})
		}
	}

	return &Receiver{
		config:    config,
		auth:      NewAuthenticator(config.Auth),
		registry:  registry,
		processor: processor,
		stats: &ReceiverStats{
			ByType: make(map[WebhookType]int64),
		},
	}
}

// Start starts the webhook receiver HTTP server
func (r *Receiver) Start() error {
	if !r.config.Enabled {
		return fmt.Errorf("webhook receiver is disabled")
	}

	mux := http.NewServeMux()
	mux.HandleFunc(r.config.Path, r.handleWebhook)
	mux.HandleFunc("/health", r.handleHealth)
	mux.HandleFunc("/stats", r.handleStats)

	r.server = &http.Server{
		Addr:         r.config.Addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	log.Printf("Starting webhook receiver on %s%s", r.config.Addr, r.config.Path)
	return r.server.ListenAndServe()
}

// Stop stops the webhook receiver
func (r *Receiver) Stop(ctx context.Context) error {
	if r.server != nil {
		return r.server.Shutdown(ctx)
	}
	return nil
}

// handleWebhook handles incoming webhook requests
func (r *Receiver) handleWebhook(w http.ResponseWriter, req *http.Request) {
	// Only accept POST requests
	if req.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer req.Body.Close()

	// Authenticate request
	if err := r.auth.Authenticate(req, body); err != nil {
		log.Printf("Authentication failed: %v", err)
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		r.incrementFailed()
		return
	}

	// Detect webhook type
	webhookType, err := r.registry.DetectType(req)
	if err != nil {
		log.Printf("Failed to detect webhook type: %v", err)
		http.Error(w, "Unable to detect webhook type", http.StatusBadRequest)
		r.incrementFailed()
		return
	}

	// Get handler for webhook type
	handler, ok := r.registry.Get(webhookType)
	if !ok {
		log.Printf("No handler for webhook type: %s", webhookType)
		http.Error(w, fmt.Sprintf("No handler for webhook type: %s", webhookType), http.StatusBadRequest)
		r.incrementFailed()
		return
	}

	// Parse webhook payload
	event, err := handler.Parse(req, body)
	if err != nil {
		log.Printf("Failed to parse webhook: %v", err)
		http.Error(w, "Failed to parse webhook", http.StatusBadRequest)
		r.incrementFailed()
		return
	}

	// Update stats
	r.incrementReceived(webhookType)

	// Process event asynchronously
	go func() {
		ctx := context.Background()
		if err := r.processor.ProcessEvent(ctx, event); err != nil {
			log.Printf("Failed to process webhook event: %v", err)
			r.incrementFailed()
			return
		}
		r.incrementProcessed()
	}()

	// Send success response
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "accepted",
		"id":     event.ID,
	})
}

// handleHealth handles health check requests
func (r *Receiver) handleHealth(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status": "healthy",
		"config": map[string]interface{}{
			"enabled":  r.config.Enabled,
			"addr":     r.config.Addr,
			"path":     r.config.Path,
			"handlers": r.config.Handlers,
		},
	})
}

// handleStats handles statistics requests
func (r *Receiver) handleStats(w http.ResponseWriter, req *http.Request) {
	r.stats.mu.RLock()
	defer r.stats.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(r.stats)
}

// incrementReceived increments received counter
func (r *Receiver) incrementReceived(webhookType WebhookType) {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.TotalReceived++
	r.stats.ByType[webhookType]++
	r.stats.LastReceivedTime = time.Now()
}

// incrementProcessed increments processed counter
func (r *Receiver) incrementProcessed() {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.TotalProcessed++
	r.stats.LastProcessedTime = time.Now()
}

// incrementFailed increments failed counter
func (r *Receiver) incrementFailed() {
	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()
	r.stats.TotalFailed++
}

// GetStats returns current statistics
func (r *Receiver) GetStats() *ReceiverStats {
	r.stats.mu.RLock()
	defer r.stats.mu.RUnlock()

	// Create a copy
	stats := &ReceiverStats{
		TotalReceived:     r.stats.TotalReceived,
		TotalProcessed:    r.stats.TotalProcessed,
		TotalFailed:       r.stats.TotalFailed,
		ByType:            make(map[WebhookType]int64),
		LastReceivedTime:  r.stats.LastReceivedTime,
		LastProcessedTime: r.stats.LastProcessedTime,
	}

	for k, v := range r.stats.ByType {
		stats.ByType[k] = v
	}

	return stats
}

// EventBusProcessor implements EventProcessor using the event bus
type EventBusProcessor struct {
	publisher events.EventPublisher
}

// NewEventBusProcessor creates a new event bus processor
func NewEventBusProcessor(publisher events.EventPublisher) *EventBusProcessor {
	return &EventBusProcessor{
		publisher: publisher,
	}
}

// ProcessEvent publishes webhook events to the event bus
func (p *EventBusProcessor) ProcessEvent(ctx context.Context, webhook *WebhookEvent) error {
	// Convert webhook to Keystone Core event
	event := webhook.ToKscoreEvent()

	// Publish to event bus
	return p.publisher.PublishAsync(event)
}
