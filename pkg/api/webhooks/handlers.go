// Package webhooks provides HTTP handlers for webhooks REST API endpoints.
package webhooks

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/shawnbutts/keystone-core/internal/gitops/webhook"
)

// Handler provides HTTP handlers for webhooks API endpoints.
type Handler struct {
	receiver *webhook.Receiver
	registry *webhook.HandlerRegistry
}

// NewHandler creates a new webhooks API handler.
func NewHandler(receiver *webhook.Receiver, registry *webhook.HandlerRegistry) *Handler {
	return &Handler{
		receiver: receiver,
		registry: registry,
	}
}

// RegisterRoutes registers the webhooks API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/webhooks", h.handleWebhook)
	mux.HandleFunc("/api/v1/webhooks/stats", h.handleStats)
	mux.HandleFunc("/api/v1/webhooks/config", h.handleConfig)
}

// WebhookRequest represents the request body for POST /api/v1/webhooks
type WebhookRequest struct {
	// Type is the webhook source type (argocd, flux, github, gitlab)
	Type string `json:"type"`

	// Payload is the raw webhook payload
	Payload map[string]interface{} `json:"payload"`
}

// WebhookResponse represents the response for POST /api/v1/webhooks
type WebhookResponse struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	Type      string    `json:"type"`
	EventType string    `json:"event_type,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// StatsResponse represents the response for GET /api/v1/webhooks/stats
type StatsResponse struct {
	TotalReceived     int64            `json:"total_received"`
	TotalProcessed    int64            `json:"total_processed"`
	TotalFailed       int64            `json:"total_failed"`
	ByType            map[string]int64 `json:"by_type"`
	LastReceivedTime  *time.Time       `json:"last_received_time,omitempty"`
	LastProcessedTime *time.Time       `json:"last_processed_time,omitempty"`
	RetrievedAt       time.Time        `json:"retrieved_at"`
}

// ConfigResponse represents the response for GET /api/v1/webhooks/config
type ConfigResponse struct {
	Enabled    bool     `json:"enabled"`
	Addr       string   `json:"addr"`
	Path       string   `json:"path"`
	AuthType   string   `json:"auth_type"`
	Handlers   []string `json:"handlers"`
	WebhookURL string   `json:"webhook_url,omitempty"`
}

// handleWebhook handles POST /api/v1/webhooks
func (h *Handler) handleWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Read request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to read request body: "+err.Error())
		return
	}
	defer r.Body.Close()

	// Try to detect webhook type from headers first (for native webhook forwarding)
	webhookType, err := h.registry.DetectType(r)
	if err != nil {
		// If detection fails, try to parse as WebhookRequest
		var req WebhookRequest
		if jsonErr := json.Unmarshal(body, &req); jsonErr == nil && req.Type != "" {
			webhookType = webhook.WebhookType(req.Type)
			// Re-encode payload as body for handler
			if req.Payload != nil {
				body, _ = json.Marshal(req.Payload)
			}
		} else {
			writeError(w, http.StatusBadRequest, "Unable to detect webhook type. Provide 'type' field or appropriate headers")
			return
		}
	}

	// Get handler for webhook type
	handler, ok := h.registry.Get(webhookType)
	if !ok {
		writeError(w, http.StatusBadRequest, "No handler for webhook type: "+string(webhookType))
		return
	}

	// Parse webhook payload
	event, err := handler.Parse(r, body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "Failed to parse webhook: "+err.Error())
		return
	}

	// Return success response with event details
	resp := WebhookResponse{
		ID:        event.ID,
		Status:    "accepted",
		Type:      string(event.Type),
		EventType: event.EventType,
		Timestamp: event.Timestamp,
	}

	writeJSON(w, http.StatusAccepted, resp)
}

// handleStats handles GET /api/v1/webhooks/stats
func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := h.receiver.GetStats()

	// Convert map keys to strings
	byType := make(map[string]int64)
	for k, v := range stats.ByType {
		byType[string(k)] = v
	}

	resp := StatsResponse{
		TotalReceived:  stats.TotalReceived,
		TotalProcessed: stats.TotalProcessed,
		TotalFailed:    stats.TotalFailed,
		ByType:         byType,
		RetrievedAt:    time.Now().UTC(),
	}

	// Only include times if they're non-zero
	if !stats.LastReceivedTime.IsZero() {
		resp.LastReceivedTime = &stats.LastReceivedTime
	}
	if !stats.LastProcessedTime.IsZero() {
		resp.LastProcessedTime = &stats.LastProcessedTime
	}

	writeJSON(w, http.StatusOK, resp)
}

// handleConfig handles GET /api/v1/webhooks/config
func (h *Handler) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// We don't have direct access to config from receiver, so return a basic response
	// In a real implementation, you'd expose config from the receiver
	resp := ConfigResponse{
		Enabled:  true,
		Handlers: []string{"argocd", "flux", "github", "gitlab"},
	}

	writeJSON(w, http.StatusOK, resp)
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}
