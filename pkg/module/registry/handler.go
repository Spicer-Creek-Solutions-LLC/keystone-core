package registry

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"go.keystone-core.io/keystone-core/pkg/module/manifest"
)

// Handler serves the Go module-proxy protocol for a Registry.
//
// Module names contain '/', so a single catch-all handler splits
// the request path on the "/@v/" infix (the Go proxy convention):
// left = module, right = list | <ver>.info | <ver>.mod | <ver>.zip.
type Handler struct{ reg *Registry }

// NewHandler returns an HTTP handler over reg.
func NewHandler(reg *Registry) *Handler { return &Handler{reg: reg} }

// Register mounts the catch-all proxy route.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /", h.serve)
}

const vSep = "/@v/"

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	p := strings.TrimPrefix(r.URL.Path, "/")
	i := strings.Index(p, vSep)
	if i <= 0 {
		http.Error(w, "not a module-proxy path", http.StatusBadRequest)
		return
	}
	module := p[:i]
	action := p[i+len(vSep):]
	if module == "" || action == "" || !manifest.ValidModuleName(module) {
		http.Error(w, "invalid module name", http.StatusBadRequest)
		return
	}
	ctx := r.Context()

	switch {
	case action == "list":
		vs, err := h.reg.versions(ctx, module)
		if err != nil {
			h.fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		for _, v := range vs {
			// v is a semver.Version parsed from stored metadata
			// (digits/dots/hyphens only) and the response is
			// text/plain — not an HTML/XSS sink.
			_, _ = w.Write([]byte(v.String() + "\n")) //nolint:gosec // G705: semver-validated, text/plain
		}

	case strings.HasSuffix(action, ".info"):
		ver := strings.TrimSuffix(action, ".info")
		vi, err := h.reg.readInfo(ctx, module, ver)
		if err != nil {
			h.fail(w, err)
			return
		}
		// §4.18 contract: expose only {Version, Time}.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(struct {
			Version string    `json:"Version"`
			Time    time.Time `json:"Time"`
		}{vi.Version, vi.Time})

	case strings.HasSuffix(action, ".mod"):
		ver := strings.TrimSuffix(action, ".mod")
		b, err := h.reg.getBytes(ctx, manifestKey(module, ver))
		if err != nil {
			h.fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/x-yaml")
		_, _ = w.Write(b)

	case strings.HasSuffix(action, ".zip"):
		ver := strings.TrimSuffix(action, ".zip")
		b, err := h.reg.getBytes(ctx, zipKey(module, ver))
		if err != nil {
			h.fail(w, err)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(b)

	default:
		http.Error(w, "unknown action", http.StatusBadRequest)
	}
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	if errors.Is(err, ErrNotFound) {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	http.Error(w, "registry error", http.StatusInternalServerError)
}
