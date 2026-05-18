package registry

import (
	"encoding/json"
	"errors"
	"io"
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
// POST /publish (task 9) accepts a multipart upload.
type Handler struct {
	reg       *Registry
	maxUpload int64
}

// DefaultMaxUpload bounds a publish request body (64 MiB).
const DefaultMaxUpload int64 = 64 << 20

// NewHandler returns an HTTP handler over reg with the default
// upload cap.
func NewHandler(reg *Registry) *Handler { return &Handler{reg: reg, maxUpload: DefaultMaxUpload} }

// NewHandlerWithLimit is NewHandler with an explicit publish body
// cap (≤0 → the default).
func NewHandlerWithLimit(reg *Registry, maxUpload int64) *Handler {
	if maxUpload <= 0 {
		maxUpload = DefaultMaxUpload
	}
	return &Handler{reg: reg, maxUpload: maxUpload}
}

// Register mounts the read-only Go-proxy routes plus POST /publish
// (Epic 14 task 9 — the kscore-module publish target).
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /publish", h.publish)
	mux.HandleFunc("GET /", h.serve)
}

// publish accepts a multipart/form-data upload with a `manifest`
// (YAML) part and a `module` (ZIP) part. v1.0 publish is
// unauthenticated — trust is the TLS-trusted registry transport +
// Cosign verification at load time (deferred auth: see the
// "Module registry publish authentication" ROADMAP entry).
func (h *Handler) publish(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, h.maxUpload)
	// Body is bounded by MaxBytesReader above; ParseMultipartForm
	// reads through that cap and errors if exceeded.
	if err := r.ParseMultipartForm(h.maxUpload); err != nil { //nolint:gosec // G120: bounded by MaxBytesReader
		http.Error(w, "invalid multipart body (or too large)", http.StatusBadRequest)
		return
	}
	manifestYAML, ok := h.formPart(w, r, "manifest")
	if !ok {
		return
	}
	zip, ok := h.formPart(w, r, "module")
	if !ok {
		return
	}
	switch err := h.reg.Publish(r.Context(), manifestYAML, zip); {
	case err == nil:
		w.WriteHeader(http.StatusCreated)
	case errors.Is(err, ErrVersionExists):
		http.Error(w, "version already exists", http.StatusConflict)
	case errors.Is(err, ErrInvalidModule):
		http.Error(w, "invalid module: "+err.Error(), http.StatusBadRequest)
	default:
		http.Error(w, "registry error", http.StatusInternalServerError)
	}
}

func (h *Handler) formPart(w http.ResponseWriter, r *http.Request, name string) ([]byte, bool) {
	f, _, err := r.FormFile(name)
	if err != nil {
		http.Error(w, "missing "+name+" part", http.StatusBadRequest)
		return nil, false
	}
	defer func() { _ = f.Close() }()
	b, err := io.ReadAll(f)
	if err != nil {
		http.Error(w, "read "+name+" part", http.StatusBadRequest)
		return nil, false
	}
	return b, true
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
