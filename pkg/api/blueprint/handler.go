// Package blueprint exposes the v1.0 blueprint REST routes (Epic 15
// task 11): list/get blueprints and apply one. Backends are injected
// via Providers; a nil provider degrades its routes to 503 (the
// pkg/api/cluster precedent — routes exist, light up when the server
// boot-wires real providers; see ROADMAP "Remote / distributed
// blueprint apply wiring"). Sensitive parameter values are never
// echoed in any response.
package blueprint

import (
	"context"
	"encoding/json"
	"net/http"
	"sort"

	bp "go.keystone-core.io/keystone-core/internal/blueprint"
)

// BlueprintCatalog lists and fetches blueprint manifests (by
// metadata.name).
type BlueprintCatalog interface {
	List(ctx context.Context) ([]*bp.Manifest, error)
	Get(ctx context.Context, name string) (*bp.Manifest, error)
}

// BlueprintApplier applies a blueprint by name.
type BlueprintApplier interface {
	Apply(ctx context.Context, name string, opts bp.ApplyOptions) (*bp.ApplyResult, error)
}

// Providers bundles the (individually nilable) backends.
type Providers struct {
	Catalog BlueprintCatalog
	Applier BlueprintApplier
}

// Handler exposes REST routes for the blueprint domain.
type Handler struct{ p Providers }

// NewHandler returns a Handler. Pass a zero Providers for the
// not-yet-wired case (routes then return 503).
func NewHandler(p Providers) *Handler { return &Handler{p: p} }

// Register installs the blueprint-domain routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/blueprints", h.list)
	mux.HandleFunc("GET /api/v1/blueprints/{name}", h.get)
	mux.HandleFunc("POST /api/v1/blueprints/{name}/apply", h.apply)
}

type manifestDTO struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	Description string   `json:"description,omitempty"`
	Parameters  []string `json:"parameters"`
	Features    []string `json:"features"`
	Entrypoints []string `json:"entrypoints"`
}

type applyResultDTO struct {
	RunID   string         `json:"run_id"`
	Status  string         `json:"status"`
	Outputs map[string]any `json:"outputs,omitempty"`
	Report  *reportDTO     `json:"report,omitempty"`
}

type reportDTO struct {
	Total   int `json:"total"`
	Changed int `json:"changed"`
	Failed  int `json:"failed"`
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func toManifestDTO(m *bp.Manifest) manifestDTO {
	eps := []string{}
	if m.Entrypoints.Default != "" {
		eps = append(eps, "default")
	}
	if m.Entrypoints.Rollback != "" {
		eps = append(eps, "rollback")
	}
	eps = append(eps, sortedKeys(m.Entrypoints.Named)...)
	return manifestDTO{
		Name:        m.Metadata.Name,
		Version:     m.Metadata.Version,
		Description: m.Metadata.Description,
		Parameters:  sortedKeys(m.Parameters),
		Features:    sortedKeys(m.Features),
		Entrypoints: eps,
	}
}

func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	if h.p.Catalog == nil {
		unavailable(w)
		return
	}
	ms, err := h.p.Catalog.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]manifestDTO, 0, len(ms))
	for _, m := range ms {
		out = append(out, toManifestDTO(m))
	}
	writeJSON(w, http.StatusOK, map[string]any{"blueprints": out, "total_count": len(out)})
}

func (h *Handler) get(w http.ResponseWriter, r *http.Request) {
	if h.p.Catalog == nil {
		unavailable(w)
		return
	}
	m, err := h.p.Catalog.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toManifestDTO(m))
}

type applyRequest struct {
	Params     map[string]string `json:"params"`
	Enable     []string          `json:"enable"`
	Disable    []string          `json:"disable"`
	As         string            `json:"as"`
	Entrypoint string            `json:"entrypoint"`
}

func (h *Handler) apply(w http.ResponseWriter, r *http.Request) {
	if h.p.Applier == nil {
		unavailable(w)
		return
	}
	var req applyRequest
	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
			return
		}
	}
	res, err := h.p.Applier.Apply(r.Context(), r.PathValue("name"), bp.ApplyOptions{
		Inputs:     req.Params,
		Enable:     req.Enable,
		Disable:    req.Disable,
		As:         req.As,
		Entrypoint: req.Entrypoint,
	})
	if res == nil && err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	dto := applyResultDTO{}
	if res != nil {
		dto.RunID = res.RunID
		dto.Status = res.Status
		dto.Outputs = res.Outputs
		if res.Report != nil {
			dto.Report = &reportDTO{Total: res.Report.Total, Changed: res.Report.Changed, Failed: res.Report.Failed}
		}
	}
	// A failed apply still returns the result envelope (caller reads
	// status); only a setup error with no result is 5xx.
	if err != nil && res == nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	status := http.StatusOK
	if err != nil {
		status = http.StatusConflict // apply ran but ended failed
	}
	writeJSON(w, status, dto)
}

func unavailable(w http.ResponseWriter) {
	writeErr(w, http.StatusServiceUnavailable, "blueprint subsystem not wired")
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
