// Package runbook exposes the v1.0 runbook REST routes (Epic 15
// task 11): list/get runbooks, execute a runbook, fetch an
// execution. Backends are injected via Providers; a nil provider
// degrades its routes to 503 (the pkg/api/cluster precedent — routes
// exist, light up when the server boot-wires real providers; see
// ROADMAP "Durable runbook execution store").
package runbook

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"

	rb "go.keystone-core.io/keystone-core/internal/runbook"
)

// RunbookCatalog lists and fetches runbook definitions (by
// metadata.name).
type RunbookCatalog interface {
	List(ctx context.Context) ([]*rb.Runbook, error)
	Get(ctx context.Context, id string) (*rb.Runbook, error)
}

// RunbookRunner executes a runbook by id with the given inputs and
// records the execution.
type RunbookRunner interface {
	Execute(ctx context.Context, id string, inputs map[string]any) (*rb.Execution, error)
}

// ExecutionStore fetches a recorded execution by id.
type ExecutionStore interface {
	Get(ctx context.Context, id string) (*rb.Execution, error)
}

// Providers bundles the (individually nilable) backends.
type Providers struct {
	Catalog RunbookCatalog
	Runner  RunbookRunner
	Store   ExecutionStore
}

// Handler exposes REST routes for the runbook domain.
type Handler struct{ p Providers }

// NewHandler returns a Handler. Pass a zero Providers for the
// not-yet-wired case (routes then return 503).
func NewHandler(p Providers) *Handler { return &Handler{p: p} }

// Register installs the runbook-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/runbooks", h.list)
	mux.HandleFunc("GET /api/v1/runbooks/{id}", h.get)
	mux.HandleFunc("POST /api/v1/runbooks", h.execute)
	mux.HandleFunc("GET /api/v1/executions/{id}", h.execution)
}

type stepDTO struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Status   string `json:"status"`
	Attempts int    `json:"attempts,omitempty"`
	Error    string `json:"error,omitempty"`
}

type runbookDTO struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace,omitempty"`
	Version   string   `json:"version,omitempty"`
	Steps     []string `json:"steps"`
}

type executionDTO struct {
	ID      string    `json:"id"`
	Runbook string    `json:"runbook"`
	Status  string    `json:"status"`
	Steps   []stepDTO `json:"steps"`
	Error   string    `json:"error,omitempty"`
}

func toRunbookDTO(r *rb.Runbook) runbookDTO {
	steps := make([]string, 0, len(r.Spec.Steps))
	for _, s := range r.Spec.Steps {
		steps = append(steps, s.Name)
	}
	return runbookDTO{
		Name:      r.Metadata.Name,
		Namespace: r.Metadata.Namespace,
		Version:   r.Metadata.Version,
		Steps:     steps,
	}
}

func toExecutionDTO(e *rb.Execution) executionDTO {
	steps := make([]stepDTO, 0, len(e.Steps))
	for _, s := range e.Steps {
		d := stepDTO{Name: s.Name, Type: s.Type, Status: string(s.Status), Attempts: s.Attempts}
		if s.Error != nil {
			d.Error = s.Error.Error()
		}
		steps = append(steps, d)
	}
	out := executionDTO{ID: e.ID, Runbook: e.Runbook, Status: string(e.Status), Steps: steps}
	if e.Error != nil {
		out.Error = e.Error.Error()
	}
	return out
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if h.p.Catalog == nil {
		unavailable(w)
		return
	}
	rbs, err := h.p.Catalog.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]runbookDTO, 0, len(rbs))
	for _, x := range rbs {
		out = append(out, toRunbookDTO(x))
	}
	writeJSON(w, http.StatusOK, map[string]any{"runbooks": out, "total_count": len(out)})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.p.Catalog == nil {
		unavailable(w)
		return
	}
	x, err := h.p.Catalog.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toRunbookDTO(x))
}

type executeRequest struct {
	Runbook string         `json:"runbook"`
	Inputs  map[string]any `json:"inputs"`
}

func (h *Handler) execute(w http.ResponseWriter, r *http.Request) {
	if h.p.Runner == nil {
		unavailable(w)
		return
	}
	var req executeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return
	}
	if req.Runbook == "" {
		writeErr(w, http.StatusBadRequest, "field \"runbook\" is required")
		return
	}
	exec, err := h.p.Runner.Execute(r.Context(), req.Runbook, req.Inputs)
	if exec == nil && err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// A failed run still returns the execution; the caller inspects
	// Status. Only a non-execution error (bad runbook, setup) is 5xx.
	if err != nil && !errors.Is(err, rb.ErrExecutionFailed) {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toExecutionDTO(exec))
}

func (h *Handler) execution(w http.ResponseWriter, r *http.Request) {
	if h.p.Store == nil {
		unavailable(w)
		return
	}
	e, err := h.p.Store.Get(r.Context(), r.PathValue("id"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toExecutionDTO(e))
}

func unavailable(w http.ResponseWriter) {
	writeErr(w, http.StatusServiceUnavailable, "runbook subsystem not wired")
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
