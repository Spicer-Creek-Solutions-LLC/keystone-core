// Package events exposes REST routes for the events domain
// (Epic 11 task 6).
//
// Routes (all sync; SubscribeEvents stays gRPC-only):
//
//	GET    /api/v1/events                list (?type=&category=&source=&min_severity=&since=&until=&cursor=&limit=&desc=)
//	GET    /api/v1/events/{id}           get one
//	POST   /api/v1/events                emit (JSON body)
//	GET    /api/v1/events/types          taxonomy
//	GET    /api/v1/events/stats          counts by type + severity (?since=&until=)
//
// JSON DTOs serialise Severity as the canonical lowercase name for
// browser / CLI ergonomics; gRPC uses the EventSeverity enum.
//
// Authentication + RBAC are enforced by the auth interceptor /
// middleware (Epic 03 task 4); this handler trusts incoming requests
// to have already passed those checks.
//
// When the store / publisher are not configured (`events.enabled:
// false` or sub-component disabled), the handler returns HTTP 503
// Service Unavailable on the affected routes.
package events

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/internal/events"
	"go.keystone-core.io/keystone-core/internal/state"
)

// Handler exposes REST routes for the events domain. Either of store
// or publisher may be nil — affected routes return 503.
type Handler struct {
	store     events.EventStore
	publisher events.EventPublisher
}

// NewHandler returns a Handler. Pass nil for components that aren't
// configured.
func NewHandler(store events.EventStore, publisher events.EventPublisher) *Handler {
	return &Handler{store: store, publisher: publisher}
}

// Register installs the routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// More-specific patterns first so /events/types and /events/stats
	// don't collide with the {id} wildcard.
	mux.HandleFunc("GET /api/v1/events/types", h.handleTypes)
	mux.HandleFunc("GET /api/v1/events/stats", h.handleStats)
	mux.HandleFunc("GET /api/v1/events/{id}", h.handleGet)
	mux.HandleFunc("GET /api/v1/events", h.handleList)
	mux.HandleFunc("POST /api/v1/events", h.handleEmit)
}

// ---- DTOs ----------------------------------------------------------------

type eventDTO struct {
	ID            string            `json:"id"`
	Type          string            `json:"type"`
	Source        string            `json:"source"`
	Severity      string            `json:"severity"`
	Time          time.Time         `json:"time"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	Data          map[string]any    `json:"data,omitempty"`
	Subject       string            `json:"subject,omitempty"`
}

type emitRequest struct {
	Type          string            `json:"type"`
	Source        string            `json:"source"`
	Severity      string            `json:"severity,omitempty"`
	Time          time.Time         `json:"time,omitempty"`
	CorrelationID string            `json:"correlation_id,omitempty"`
	Tags          map[string]string `json:"tags,omitempty"`
	Data          map[string]any    `json:"data,omitempty"`
}

type emitResponse struct {
	EventID string `json:"event_id"`
}

type listResponse struct {
	Events     []eventDTO `json:"events"`
	NextCursor string     `json:"next_cursor,omitempty"`
}

type typesResponse struct {
	Types []string `json:"types"`
}

type statsResponse struct {
	ByType     map[string]int64 `json:"by_type"`
	BySeverity map[string]int64 `json:"by_severity"`
	Total      int64            `json:"total"`
}

type errorDTO struct {
	Error string `json:"error"`
}

// ---- handlers ------------------------------------------------------------

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "events disabled")
		return
	}
	q, err := queryFromRequest(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	page, err := h.store.Query(r.Context(), q)
	if err != nil {
		writeEventsError(w, err)
		return
	}
	out := listResponse{
		Events:     make([]eventDTO, 0, len(page.Events)),
		NextCursor: page.NextCursor,
	}
	for _, e := range page.Events {
		out.Events = append(out.Events, eventToDTO(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "events disabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "event id is required")
		return
	}
	e, err := h.store.Get(r.Context(), id)
	if err != nil {
		writeEventsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, eventToDTO(e))
}

func (h *Handler) handleEmit(w http.ResponseWriter, r *http.Request) {
	if h.publisher == nil {
		writeError(w, http.StatusServiceUnavailable, "events publisher disabled")
		return
	}
	var req emitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type is required")
		return
	}
	if req.Source == "" {
		writeError(w, http.StatusBadRequest, "source is required")
		return
	}
	typ, err := events.ParseEventType(req.Type)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	e, err := events.NewEvent(typ, req.Source)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Severity != "" {
		sev, err := events.ParseSeverity(req.Severity)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		e.Severity = sev
	}
	if !req.Time.IsZero() {
		e.Time = req.Time
	}
	e.CorrelationID = req.CorrelationID
	e.Tags = req.Tags
	e.Data = req.Data
	if err := h.publisher.Publish(r.Context(), e); err != nil {
		writeEventsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, emitResponse{EventID: e.ID})
}

func (h *Handler) handleTypes(w http.ResponseWriter, _ *http.Request) {
	canon := events.CanonicalEventTypes()
	out := typesResponse{Types: make([]string, len(canon))}
	for i, t := range canon {
		out.Types[i] = string(t)
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleStats(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "events disabled")
		return
	}
	base := events.EventQuery{}
	if v := r.URL.Query().Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid since: %v", err))
			return
		}
		base.Since = t
	}
	if v := r.URL.Query().Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid until: %v", err))
			return
		}
		base.Until = t
	}

	total, err := h.store.Count(r.Context(), base)
	if err != nil {
		writeEventsError(w, err)
		return
	}
	byType := make(map[string]int64)
	for _, t := range events.CanonicalEventTypes() {
		q := base
		q.Type = t
		n, err := h.store.Count(r.Context(), q)
		if err != nil {
			writeEventsError(w, err)
			return
		}
		if n > 0 {
			byType[string(t)] = int64(n)
		}
	}
	bySeverity := make(map[string]int64)
	severities := events.AllSeverities()
	for i, sev := range severities {
		q := base
		q.MinSeverity = sev
		atLeast, err := h.store.Count(r.Context(), q)
		if err != nil {
			writeEventsError(w, err)
			return
		}
		if i+1 < len(severities) {
			nextQ := base
			nextQ.MinSeverity = severities[i+1]
			nextN, err := h.store.Count(r.Context(), nextQ)
			if err != nil {
				writeEventsError(w, err)
				return
			}
			atLeast -= nextN
		}
		if atLeast > 0 {
			bySeverity[sev.String()] = int64(atLeast)
		}
	}
	writeJSON(w, http.StatusOK, statsResponse{
		ByType:     byType,
		BySeverity: bySeverity,
		Total:      int64(total),
	})
}

// ---- helpers -------------------------------------------------------------

func queryFromRequest(r *http.Request) (events.EventQuery, error) {
	q := events.EventQuery{
		Source:        r.URL.Query().Get("source"),
		CorrelationID: r.URL.Query().Get("correlation_id"),
		Cursor:        r.URL.Query().Get("cursor"),
	}
	if typ := r.URL.Query().Get("type"); typ != "" {
		q.Type = events.EventType(typ)
	}
	if cat := r.URL.Query().Get("category"); cat != "" {
		if q.Type != "" {
			return events.EventQuery{}, errors.New("type and category are mutually exclusive")
		}
		q.Category = events.Category(cat)
	}
	if v := r.URL.Query().Get("min_severity"); v != "" {
		sev, err := events.ParseSeverity(v)
		if err != nil {
			return events.EventQuery{}, fmt.Errorf("min_severity: %w", err)
		}
		q.MinSeverity = sev
	}
	if v := r.URL.Query().Get("since"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return events.EventQuery{}, fmt.Errorf("invalid since: %w", err)
		}
		q.Since = t
	}
	if v := r.URL.Query().Get("until"); v != "" {
		t, err := time.Parse(time.RFC3339, v)
		if err != nil {
			return events.EventQuery{}, fmt.Errorf("invalid until: %w", err)
		}
		q.Until = t
	}
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return events.EventQuery{}, fmt.Errorf("invalid limit %q", v)
		}
		q.Limit = n
	}
	if r.URL.Query().Get("desc") == "true" {
		q.Descending = true
	}
	// Tag filters: every query param of the form `tag.<key>=<value>`
	// becomes a tag predicate. Multiple tag.* params are ANDed.
	for k, vs := range r.URL.Query() {
		const prefix = "tag."
		if !strings.HasPrefix(k, prefix) || len(vs) == 0 {
			continue
		}
		if q.Tags == nil {
			q.Tags = map[string]string{}
		}
		q.Tags[k[len(prefix):]] = vs[0]
	}
	return q, nil
}

func eventToDTO(e events.Event) eventDTO {
	return eventDTO{
		ID:            e.ID,
		Type:          string(e.Type),
		Source:        e.Source,
		Severity:      e.Severity.String(),
		Time:          e.Time,
		CorrelationID: e.CorrelationID,
		Tags:          e.Tags,
		Data:          e.Data,
		Subject:       e.Subject,
	}
}

func readJSON(r *http.Request, v any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return fmt.Errorf("decode body: %w", err)
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(errorDTO{Error: msg})
}

// writeEventsError maps the events / state sentinels to HTTP status
// codes. Mirrors the gRPC table in
// controlplane/grpc_events_server.go.
func writeEventsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, state.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, events.ErrEventNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, events.ErrInvalidEvent):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, events.ErrInvalidFilter):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, events.ErrPublisherNotStarted):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, events.ErrSubscriberNotStarted):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, events.ErrPublisherBufferFull):
		writeError(w, http.StatusTooManyRequests, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}
