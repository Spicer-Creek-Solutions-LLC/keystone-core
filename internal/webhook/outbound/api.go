package outbound

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// APIHandler provides HTTP handlers for outbound webhook subscription endpoints.
type APIHandler struct {
	store SubscriptionStore
}

// NewAPIHandler creates a new outbound webhook API handler.
func NewAPIHandler(store SubscriptionStore) *APIHandler {
	return &APIHandler{store: store}
}

// RegisterRoutes registers the outbound webhook API routes.
func (h *APIHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/webhooks/subscriptions", h.handleSubscriptions)
	mux.HandleFunc("/api/v1/webhooks/subscriptions/", h.handleSubscription)
}

func (h *APIHandler) handleSubscriptions(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.handleList(w, r)
	case http.MethodPost:
		h.handleCreate(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) handleSubscription(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/webhooks/subscriptions/")

	if strings.HasSuffix(path, "/test") {
		id := strings.TrimSuffix(path, "/test")
		if r.Method == http.MethodPost {
			h.handleTest(w, r, id)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if strings.HasSuffix(path, "/deliveries") {
		id := strings.TrimSuffix(path, "/deliveries")
		if r.Method == http.MethodGet {
			h.handleDeliveries(w, r, id)
			return
		}
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	id := path
	switch r.Method {
	case http.MethodGet:
		h.handleGet(w, r, id)
	case http.MethodPatch:
		h.handleUpdate(w, r, id)
	case http.MethodDelete:
		h.handleDelete(w, r, id)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *APIHandler) handleList(w http.ResponseWriter, r *http.Request) {
	subs, err := h.store.List(r.Context())
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	masked := make([]map[string]interface{}, 0, len(subs))
	for _, s := range subs {
		masked = append(masked, maskSubscription(s))
	}
	writeJSON(w, http.StatusOK, masked)
}

type createRequest struct {
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Secret     string            `json:"secret"`
	Events     []string          `json:"events"`
	Headers    map[string]string `json:"headers"`
	MaxRetries *int              `json:"max_retries"`
	TimeoutSec *int              `json:"timeout_secs"`
}

func (h *APIHandler) handleCreate(w http.ResponseWriter, r *http.Request) {
	var req createRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name is required"})
		return
	}
	if req.URL == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "url is required"})
		return
	}
	if _, err := url.ParseRequestURI(req.URL); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid url"})
		return
	}
	if len(req.Events) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "events is required"})
		return
	}

	now := time.Now()
	sub := &Subscription{
		ID:         generateID(),
		Name:       req.Name,
		URL:        req.URL,
		Secret:     req.Secret,
		Events:     req.Events,
		Enabled:    true,
		Headers:    req.Headers,
		MaxRetries: 3,
		TimeoutSec: 10,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if sub.Headers == nil {
		sub.Headers = map[string]string{}
	}
	if req.MaxRetries != nil {
		sub.MaxRetries = *req.MaxRetries
	}
	if req.TimeoutSec != nil {
		sub.TimeoutSec = *req.TimeoutSec
	}

	if err := h.store.Create(r.Context(), sub); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusCreated, maskSubscription(sub))
}

func (h *APIHandler) handleGet(w http.ResponseWriter, r *http.Request, id string) {
	sub, err := h.store.Get(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, maskSubscription(sub))
}

type updateRequest struct {
	Name       *string            `json:"name"`
	URL        *string            `json:"url"`
	Secret     *string            `json:"secret"`
	Events     []string           `json:"events"`
	Enabled    *bool              `json:"enabled"`
	Headers    *map[string]string `json:"headers"`
	MaxRetries *int               `json:"max_retries"`
	TimeoutSec *int               `json:"timeout_secs"`
}

func (h *APIHandler) handleUpdate(w http.ResponseWriter, r *http.Request, id string) {
	sub, err := h.store.Get(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	var req updateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON"})
		return
	}

	if req.Name != nil {
		sub.Name = *req.Name
	}
	if req.URL != nil {
		if _, err := url.ParseRequestURI(*req.URL); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid url"})
			return
		}
		sub.URL = *req.URL
	}
	if req.Secret != nil {
		sub.Secret = *req.Secret
	}
	if req.Events != nil {
		sub.Events = req.Events
	}
	if req.Enabled != nil {
		sub.Enabled = *req.Enabled
	}
	if req.Headers != nil {
		sub.Headers = *req.Headers
	}
	if req.MaxRetries != nil {
		sub.MaxRetries = *req.MaxRetries
	}
	if req.TimeoutSec != nil {
		sub.TimeoutSec = *req.TimeoutSec
	}
	sub.UpdatedAt = time.Now()

	if err := h.store.Update(r.Context(), sub); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, maskSubscription(sub))
}

func (h *APIHandler) handleDelete(w http.ResponseWriter, r *http.Request, id string) {
	if err := h.store.Delete(r.Context(), id); err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *APIHandler) handleTest(w http.ResponseWriter, r *http.Request, id string) {
	sub, err := h.store.Get(r.Context(), id)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "subscription not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	testPayload := map[string]interface{}{
		"type":      "webhook.test",
		"source":    "keystone-core",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"data":      map[string]string{"message": "test event from Keystone Core"},
	}
	payload, _ := json.Marshal(testPayload)

	deliveryID := generateID()
	dispatcher := NewDispatcher(time.Duration(sub.TimeoutSec) * time.Second)
	code, deliverErr := dispatcher.Deliver(r.Context(), sub, payload, deliveryID)

	result := map[string]interface{}{
		"delivery_id": deliveryID,
		"status_code": code,
		"success":     deliverErr == nil,
	}
	if deliverErr != nil {
		result["error"] = deliverErr.Error()
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *APIHandler) handleDeliveries(w http.ResponseWriter, r *http.Request, id string) {
	limit := 50
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	records, err := h.store.ListDeliveries(r.Context(), id, limit)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if records == nil {
		records = []*DeliveryRecord{}
	}
	writeJSON(w, http.StatusOK, records)
}

func maskSubscription(s *Subscription) map[string]interface{} {
	secret := ""
	if s.Secret != "" {
		secret = "***"
	}
	return map[string]interface{}{
		"id":           s.ID,
		"name":         s.Name,
		"url":          s.URL,
		"secret":       secret,
		"events":       s.Events,
		"enabled":      s.Enabled,
		"headers":      s.Headers,
		"max_retries":  s.MaxRetries,
		"timeout_secs": s.TimeoutSec,
		"created_at":   s.CreatedAt,
		"updated_at":   s.UpdatedAt,
	}
}

func writeJSON(w http.ResponseWriter, code int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(data)
}
