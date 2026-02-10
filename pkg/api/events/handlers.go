// Package events provides HTTP handlers for events REST API endpoints.
package events

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/events"
	"github.com/shawnbutts/keystone-core/pkg/api/apierror"
)

// Handler provides HTTP handlers for events API endpoints.
type Handler struct {
	store     events.EventStore
	publisher events.EventPublisher
}

// NewHandler creates a new events API handler.
func NewHandler(store events.EventStore, publisher events.EventPublisher) *Handler {
	return &Handler{
		store:     store,
		publisher: publisher,
	}
}

// RegisterRoutes registers the events API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/events", h.handleEvents)
	mux.HandleFunc("/api/v1/events/", h.handleEvent)
}

// EventResponse represents an event in API responses.
type EventResponse struct {
	ID            string                 `json:"id"`
	Type          string                 `json:"type"`
	Source        string                 `json:"source"`
	Timestamp     time.Time              `json:"timestamp"`
	Severity      string                 `json:"severity"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Tags          map[string]string      `json:"tags,omitempty"`
	Data          map[string]interface{} `json:"data,omitempty"`
}

// EventListResponse represents the list events API response.
type EventListResponse struct {
	Events      []EventResponse `json:"events"`
	Total       int64           `json:"total"`
	Limit       int             `json:"limit"`
	Offset      int             `json:"offset"`
	RetrievedAt time.Time       `json:"retrieved_at"`
}

// CreateEventRequest represents the request body for creating an event.
type CreateEventRequest struct {
	Type          string                 `json:"type"`
	Source        string                 `json:"source"`
	Severity      string                 `json:"severity,omitempty"`
	CorrelationID string                 `json:"correlation_id,omitempty"`
	Tags          map[string]string      `json:"tags,omitempty"`
	Data          map[string]interface{} `json:"data,omitempty"`
}

// handleEvents handles GET/POST /api/v1/events
func (h *Handler) handleEvents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listEvents(w, r)
	case http.MethodPost:
		h.createEvent(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleEvent handles GET /api/v1/events/{id}
func (h *Handler) handleEvent(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract event ID from path
	eventID := strings.TrimPrefix(r.URL.Path, "/api/v1/events/")
	if eventID == "" {
		writeError(w, http.StatusBadRequest, "Event ID required")
		return
	}

	// Get event from store
	ctx := r.Context()
	event, err := h.store.Get(ctx, eventID)
	if err != nil {
		writeError(w, http.StatusNotFound, "Event not found")
		return
	}

	resp := convertEvent(event)
	writeJSON(w, http.StatusOK, resp)
}

// listEvents handles GET /api/v1/events
func (h *Handler) listEvents(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	query := r.URL.Query()

	// Build event query
	eventQuery := &events.EventQuery{}

	// Filter by type
	if types := query.Get("type"); types != "" {
		for _, t := range strings.Split(types, ",") {
			eventQuery.Types = append(eventQuery.Types, events.EventType(strings.TrimSpace(t)))
		}
	}

	// Filter by source
	if sources := query.Get("source"); sources != "" {
		eventQuery.Sources = strings.Split(sources, ",")
	}

	// Filter by severity
	if severities := query.Get("severity"); severities != "" {
		for _, s := range strings.Split(severities, ",") {
			eventQuery.Severities = append(eventQuery.Severities, events.Severity(strings.TrimSpace(s)))
		}
	}

	// Filter by correlation ID
	if correlationID := query.Get("correlation_id"); correlationID != "" {
		eventQuery.CorrelationID = correlationID
	}

	// Parse tags (format: key=value,key2=value2)
	if tags := query.Get("tags"); tags != "" {
		eventQuery.Tags = parseTags(tags)
	}

	// Parse time range
	if start := query.Get("start"); start != "" {
		if t, err := time.Parse(time.RFC3339, start); err == nil {
			eventQuery.StartTime = &t
		}
	}
	if end := query.Get("end"); end != "" {
		if t, err := time.Parse(time.RFC3339, end); err == nil {
			eventQuery.EndTime = &t
		}
	}

	// Parse sorting
	eventQuery.SortBy = query.Get("sort")
	if eventQuery.SortBy == "" {
		eventQuery.SortBy = "time"
	}
	eventQuery.SortOrder = query.Get("order")
	if eventQuery.SortOrder == "" {
		eventQuery.SortOrder = "desc"
	}

	// Parse pagination
	eventQuery.Limit = 50 // default
	if limitStr := query.Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			eventQuery.Limit = l
		}
	}
	if offsetStr := query.Get("offset"); offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			eventQuery.Offset = o
		}
	}

	// Query events
	ctx := r.Context()
	result, err := h.store.Query(ctx, eventQuery)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to query events: "+err.Error())
		return
	}

	// Build response
	eventResponses := make([]EventResponse, 0, len(result.Events))
	for _, e := range result.Events {
		eventResponses = append(eventResponses, convertEvent(e))
	}

	resp := EventListResponse{
		Events:      eventResponses,
		Total:       result.TotalCount,
		Limit:       result.Limit,
		Offset:      result.Offset,
		RetrievedAt: time.Now().UTC(),
	}

	writeJSON(w, http.StatusOK, resp)
}

// createEvent handles POST /api/v1/events
func (h *Handler) createEvent(w http.ResponseWriter, r *http.Request) {
	var req CreateEventRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body: "+err.Error())
		return
	}

	// Validate request
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}

	// Set default severity
	severity := events.SeverityInfo
	if req.Severity != "" {
		severity = events.Severity(req.Severity)
	}

	// Build event using builder pattern
	builder := events.NewEvent(events.EventType(req.Type)).
		Source(req.Source).
		Severity(severity)

	// Add correlation ID if provided
	if req.CorrelationID != "" {
		builder = builder.CorrelationID(req.CorrelationID)
	}

	// Add tags
	for k, v := range req.Tags {
		builder = builder.Tag(k, v)
	}

	// Add data
	if req.Data != nil {
		builder = builder.DataMap(req.Data)
	}

	// Build final event
	event := builder.Build()

	// Publish event
	if err := h.publisher.Publish(event); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to publish event: "+err.Error())
		return
	}

	// Store event - best-effort, event was already published and will be stored via event bus
	ctx := r.Context()
	_ = h.store.Store(ctx, event)

	resp := convertEvent(event)
	writeJSON(w, http.StatusCreated, resp)
}

// Helper functions

func convertEvent(e *events.Event) EventResponse {
	return EventResponse{
		ID:            e.ID,
		Type:          string(e.Type),
		Source:        e.Source,
		Timestamp:     e.Time,
		Severity:      string(e.Severity),
		CorrelationID: e.CorrelationID,
		Tags:          e.Tags,
		Data:          e.Data,
	}
}

func parseTags(tagStr string) map[string]string {
	tags := make(map[string]string)
	pairs := strings.Split(tagStr, ",")
	for _, pair := range pairs {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			tags[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
		}
	}
	return tags
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	apierror.Write(w, status, message)
}
