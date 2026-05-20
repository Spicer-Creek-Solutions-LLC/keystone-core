// Package webhooks exposes the v1.0 outbound-webhooks REST routes per
// PROJECT-DETAILS §4.14 (Epic 16 task 16): subscription CRUD,
// test-delivery, and per-subscription delivery history. Backends are
// injected via [Providers]; a nil provider degrades its routes to
// 503 (the pkg/api/blueprint / pkg/api/gitops precedent).
//
// Secret masking: every response except `POST /subscriptions`
// (creation) replaces the subscription's secret with "***" per the
// §4.14 gotcha. Creation returns the cleartext secret exactly once
// so the operator can store/share it.
package webhooks

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/google/uuid"

	"go.keystone-core.io/keystone-core/internal/webhook/outbound"
)

// maskedSecret is the placeholder shown in every non-create response.
const maskedSecret = "***"

// WebhookManager is the subset of [outbound.Manager] this handler
// needs. *outbound.Manager satisfies it. Kept narrow so the package
// does not depend on the full manager surface — task 12 + 16 only
// need refresh-after-CRUD and the synthetic test ping.
type WebhookManager interface {
	Refresh(ctx context.Context) error
	TestSubscription(ctx context.Context, subID string) (*outbound.DeliveryRecord, error)
}

// Providers bundles the (individually nilable) backends.
type Providers struct {
	Store   outbound.SubscriptionStore
	Manager WebhookManager
}

// Handler exposes REST routes for the outbound-webhooks domain.
type Handler struct {
	p     Providers
	now   func() time.Time
	idGen func() string
}

// NewHandler returns a Handler. Pass a zero Providers for the
// not-yet-wired case (routes return 503).
func NewHandler(p Providers) *Handler {
	return &Handler{p: p, now: time.Now, idGen: uuid.NewString}
}

// Register installs the §4.14 routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/webhooks/subscriptions", h.list)
	mux.HandleFunc("POST /api/v1/webhooks/subscriptions", h.create)
	mux.HandleFunc("GET /api/v1/webhooks/subscriptions/{id}", h.get)
	mux.HandleFunc("PATCH /api/v1/webhooks/subscriptions/{id}", h.patch)
	mux.HandleFunc("DELETE /api/v1/webhooks/subscriptions/{id}", h.delete)
	mux.HandleFunc("POST /api/v1/webhooks/subscriptions/{id}/test", h.test)
	mux.HandleFunc("GET /api/v1/webhooks/subscriptions/{id}/deliveries", h.deliveries)
}

// --- DTOs -------------------------------------------------------------------

type createDTO struct {
	Name       string            `json:"name"`
	URL        string            `json:"url"`
	Secret     string            `json:"secret,omitempty"`
	Events     []string          `json:"events,omitempty"`
	Enabled    *bool             `json:"enabled,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	MaxRetries int               `json:"max_retries,omitempty"`
	TimeoutSec int               `json:"timeout_sec,omitempty"`
}

type patchDTO struct {
	Name       *string            `json:"name,omitempty"`
	URL        *string            `json:"url,omitempty"`
	Secret     *string            `json:"secret,omitempty"`
	Events     *[]string          `json:"events,omitempty"`
	Enabled    *bool              `json:"enabled,omitempty"`
	Headers    *map[string]string `json:"headers,omitempty"`
	MaxRetries *int               `json:"max_retries,omitempty"`
	TimeoutSec *int               `json:"timeout_sec,omitempty"`
}

// --- subscription routes -----------------------------------------------------

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if h.p.Store == nil {
		writeUnavailable(w, "subscription store not configured")
		return
	}
	subs, err := h.p.Store.ListSubscriptions(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]*outbound.Subscription, 0, len(subs))
	for _, s := range subs {
		out = append(out, mask(s))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	if h.p.Store == nil {
		writeUnavailable(w, "subscription store not configured")
		return
	}
	var body createDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if body.Name == "" || body.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}
	now := h.now().UTC()
	sub := &outbound.Subscription{
		ID:         h.idGen(),
		Name:       body.Name,
		URL:        body.URL,
		Secret:     body.Secret,
		Events:     body.Events,
		Enabled:    true,
		Headers:    body.Headers,
		MaxRetries: body.MaxRetries,
		TimeoutSec: body.TimeoutSec,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	if body.Enabled != nil {
		sub.Enabled = *body.Enabled
	}
	if sub.MaxRetries < 0 {
		sub.MaxRetries = 0
	}
	if sub.TimeoutSec < 0 {
		sub.TimeoutSec = 0
	}
	if err := h.p.Store.CreateSubscription(r.Context(), sub); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.refresh(r.Context())
	// Creation is the only response that returns the cleartext secret
	// — §4.14 gotcha. Subsequent reads/updates mask it.
	writeJSON(w, http.StatusCreated, sub)
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.p.Store == nil {
		writeUnavailable(w, "subscription store not configured")
		return
	}
	sub, ok, err := h.p.Store.GetSubscription(r.Context(), r.PathValue("id"))
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	writeJSON(w, http.StatusOK, mask(sub))
}

func (h *Handler) patch(w http.ResponseWriter, r *http.Request) {
	if h.p.Store == nil {
		writeUnavailable(w, "subscription store not configured")
		return
	}
	id := r.PathValue("id")
	sub, ok, err := h.p.Store.GetSubscription(r.Context(), id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "subscription not found")
		return
	}
	var body patchDTO
	if err := decodeJSON(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	applyPatch(sub, body)
	sub.UpdatedAt = h.now().UTC()
	if err := h.p.Store.UpdateSubscription(r.Context(), sub); err != nil {
		if errors.Is(err, outbound.ErrSubscriptionNotFound) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	h.refresh(r.Context())
	writeJSON(w, http.StatusOK, mask(sub))
}

func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	if h.p.Store == nil {
		writeUnavailable(w, "subscription store not configured")
		return
	}
	err := h.p.Store.DeleteSubscription(r.Context(), r.PathValue("id"))
	switch {
	case errors.Is(err, outbound.ErrSubscriptionNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case err != nil:
		writeError(w, http.StatusInternalServerError, err.Error())
	default:
		h.refresh(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}
}

func (h *Handler) test(w http.ResponseWriter, r *http.Request) {
	if h.p.Manager == nil {
		writeUnavailable(w, "webhook manager not configured")
		return
	}
	rec, err := h.p.Manager.TestSubscription(r.Context(), r.PathValue("id"))
	if err != nil {
		// TestSubscription returns a not-found error for an unknown
		// id; everything else is treated as 500. The error sentinel
		// is internal to outbound; match on the message.
		if rec == nil && err.Error() != "" &&
			(errors.Is(err, outbound.ErrSubscriptionNotFound) ||
				containsNotFound(err.Error())) {
			writeError(w, http.StatusNotFound, err.Error())
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, rec)
}

func (h *Handler) deliveries(w http.ResponseWriter, r *http.Request) {
	if h.p.Store == nil {
		writeUnavailable(w, "subscription store not configured")
		return
	}
	id := r.PathValue("id")
	limit := 0
	if s := r.URL.Query().Get("limit"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			limit = n
		}
	}
	out, err := h.p.Store.ListDeliveries(r.Context(), id, limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if out == nil {
		out = []*outbound.DeliveryRecord{}
	}
	writeJSON(w, http.StatusOK, out)
}

// refresh is best-effort — a failure to refresh the Manager cache is
// logged by the Manager itself; the REST call's outcome stands.
func (h *Handler) refresh(ctx context.Context) {
	if h.p.Manager == nil {
		return
	}
	_ = h.p.Manager.Refresh(ctx)
}

// --- helpers -----------------------------------------------------------------

// mask returns a shallow copy of sub with Secret replaced by "***".
// Called on every response except creation, per §4.14.
func mask(sub *outbound.Subscription) *outbound.Subscription {
	if sub == nil {
		return nil
	}
	cp := *sub
	if cp.Secret != "" {
		cp.Secret = maskedSecret
	}
	return &cp
}

func applyPatch(sub *outbound.Subscription, body patchDTO) {
	if body.Name != nil {
		sub.Name = *body.Name
	}
	if body.URL != nil {
		sub.URL = *body.URL
	}
	if body.Secret != nil {
		sub.Secret = *body.Secret
	}
	if body.Events != nil {
		sub.Events = *body.Events
	}
	if body.Enabled != nil {
		sub.Enabled = *body.Enabled
	}
	if body.Headers != nil {
		sub.Headers = *body.Headers
	}
	if body.MaxRetries != nil {
		sub.MaxRetries = *body.MaxRetries
	}
	if body.TimeoutSec != nil {
		sub.TimeoutSec = *body.TimeoutSec
	}
}

func containsNotFound(s string) bool {
	for i := 0; i+len("not found") <= len(s); i++ {
		if s[i:i+len("not found")] == "not found" {
			return true
		}
	}
	return false
}

func decodeJSON(r *http.Request, dst any) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func writeUnavailable(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusServiceUnavailable, msg)
}
