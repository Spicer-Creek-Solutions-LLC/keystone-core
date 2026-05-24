// SPDX-License-Identifier: Apache-2.0

package files

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	internalfiles "go.keystone-core.io/keystone-core/internal/files"
	"go.keystone-core.io/keystone-core/internal/files/acl"
	"go.keystone-core.io/keystone-core/internal/files/backend"
	"go.keystone-core.io/keystone-core/pkg/api/auth"
)

// Handler exposes REST routes for the file-distribution domain.
// A nil store disables every route (returns 503 — matches the
// pkg/api/secrets convention). A nil ACL means "no gating" so
// operator deployments wire one explicitly; tests + dev mode pass
// nil.
type Handler struct {
	store  backend.Store
	acl    acl.ACL
	logger *slog.Logger
}

// NewHandler returns a Handler. store may be nil (disabled state).
// acl may be nil (no gating). logger nil → [slog.Default].
func NewHandler(store backend.Store, a acl.ACL, logger *slog.Logger) *Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return &Handler{store: store, acl: a, logger: logger}
}

// Register installs the file-distribution routes onto mux. The
// metadata route is registered first so the more-specific pattern
// matches before the catch-all body route — Go 1.22+ ServeMux
// gives the more-specific pattern priority automatically, but
// explicit ordering keeps intent visible.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/files", h.handleList)
	mux.HandleFunc("GET /api/v1/files/metadata/{path...}", h.handleStat)
	mux.HandleFunc("GET /api/v1/files/{path...}", h.handleGet)
	mux.HandleFunc("PUT /api/v1/files/{path...}", h.handlePut)
	mux.HandleFunc("DELETE /api/v1/files/{path...}", h.handleDelete)
}

// ---- DTOs ------------------------------------------------------------------

type fileMetadataDTO struct {
	Path        string            `json:"path"`
	Size        int64             `json:"size"`
	Hash        string            `json:"hash"`
	ContentType string            `json:"content_type,omitempty"`
	CreatedAt   time.Time         `json:"created_at"`
	Version     int64             `json:"version"`
	Tags        map[string]string `json:"tags,omitempty"`
}

type listResponseDTO struct {
	Files []fileMetadataDTO `json:"files"`
}

type errorDTO struct {
	Error string `json:"error"`
}

func toDTO(m internalfiles.FileMetadata) fileMetadataDTO {
	return fileMetadataDTO{
		Path:        m.Path,
		Size:        m.Size,
		Hash:        m.Hash,
		ContentType: m.ContentType,
		CreatedAt:   m.CreatedAt,
		Version:     m.Version,
		Tags:        m.Tags,
	}
}

// ---- route handlers --------------------------------------------------------

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	prefix := r.URL.Query().Get("prefix")
	if !h.checkACL(w, r, internalfiles.FileOpList, prefix) {
		return
	}
	list, err := h.store.List(r.Context(), prefix)
	if err != nil {
		h.writeBackendErr(w, err)
		return
	}
	out := listResponseDTO{Files: make([]fileMetadataDTO, 0, len(list))}
	for _, m := range list {
		out.Files = append(out.Files, toDTO(m))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleStat(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	path := r.PathValue("path")
	if !h.checkACL(w, r, internalfiles.FileOpGet, path) {
		return
	}
	meta, err := h.store.Stat(r.Context(), path)
	if err != nil {
		h.writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDTO(meta))
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	path := r.PathValue("path")
	if !h.checkACL(w, r, internalfiles.FileOpGet, path) {
		return
	}
	meta, body, err := h.store.Get(r.Context(), path)
	if err != nil {
		h.writeBackendErr(w, err)
		return
	}
	defer func() { _ = body.Close() }()

	ct := meta.ContentType
	if ct == "" {
		ct = "application/octet-stream"
	}
	w.Header().Set("Content-Type", ct)
	if meta.Size > 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", meta.Size))
	}
	w.Header().Set("X-Kscore-File-Hash", meta.Hash)
	w.Header().Set("X-Kscore-File-Version", fmt.Sprintf("%d", meta.Version))
	w.WriteHeader(http.StatusOK)
	if _, err := io.Copy(w, body); err != nil {
		// Body was partially sent; we cannot rewrite the status.
		// Log and return — the client sees a truncated response.
		h.logger.Warn("files: copy body failed", "err", err, "path", path)
	}
}

func (h *Handler) handlePut(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	path := r.PathValue("path")
	if !h.checkACL(w, r, internalfiles.FileOpPut, path) {
		return
	}
	tags := parseTagParams(r)
	meta := internalfiles.FileMetadata{
		Path:        path,
		ContentType: r.Header.Get("Content-Type"),
		Tags:        tags,
	}
	if err := validatePutMetadata(meta); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	final, err := h.store.Put(r.Context(), meta, r.Body)
	if err != nil {
		h.writeBackendErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toDTO(final))
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	if !h.ready(w) {
		return
	}
	path := r.PathValue("path")
	if !h.checkACL(w, r, internalfiles.FileOpDelete, path) {
		return
	}
	if err := h.store.Delete(r.Context(), path); err != nil {
		h.writeBackendErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- helpers ---------------------------------------------------------------

// ready returns true if the store is configured. When disabled,
// it writes a 503 + JSON error and returns false so the caller
// short-circuits.
func (h *Handler) ready(w http.ResponseWriter) bool {
	if h.store == nil {
		writeError(w, http.StatusServiceUnavailable, "files disabled")
		return false
	}
	return true
}

// checkACL runs the configured ACL (if any) against the principal
// from the request context. On deny it writes 403 and returns
// false so the route handler short-circuits.
func (h *Handler) checkACL(w http.ResponseWriter, r *http.Request, op internalfiles.FileOperation, path string) bool {
	if h.acl == nil {
		return true
	}
	p := auth.PrincipalFromContext(r.Context())
	ns := internalfiles.Namespace(path)
	if err := h.acl.Authorize(r.Context(), p, op, ns); err != nil {
		writeError(w, http.StatusForbidden, err.Error())
		return false
	}
	return true
}

// writeBackendErr maps a backend.Store error to the right HTTP
// status. ErrNotFound → 404; everything else → 500. Validation
// errors are caught earlier (the backend's validatePath check
// surfaces as a generic error string — we route those to 400 by
// looking for the "invalid path" / "must not" prefix).
func (h *Handler) writeBackendErr(w http.ResponseWriter, err error) {
	if errors.Is(err, backend.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if isPathValidationErr(err) {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}

// isPathValidationErr detects the backend's validatePath strings
// so we surface them as 400s instead of 500s. The matched
// substrings are the canonical phrases [internal/files/backend]
// emits.
func isPathValidationErr(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	for _, frag := range []string{
		"path must not be empty",
		"path must not start with",
		"path must not end with",
		"path must not contain",
	} {
		if containsSubstr(s, frag) {
			return true
		}
	}
	return false
}

// containsSubstr keeps the helper dependency-free (no strings
// import is gained for one call site).
func containsSubstr(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// parseTagParams reads repeating ?tag=key=value query params into
// a map. A tag without "=" is ignored — silently dropping malformed
// tags keeps the upload path lenient (operators sometimes script
// these wrong; rejecting the whole upload over a stray tag would
// be obnoxious).
func parseTagParams(r *http.Request) map[string]string {
	raw := r.URL.Query()["tag"]
	if len(raw) == 0 {
		return nil
	}
	out := make(map[string]string, len(raw))
	for _, kv := range raw {
		for i := 0; i < len(kv); i++ {
			if kv[i] == '=' {
				out[kv[:i]] = kv[i+1:]
				break
			}
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// validatePutMetadata is a defense-in-depth check; the backend
// also re-validates. Catches the obvious cases at the REST layer
// so a malformed request gets a 400 instead of bouncing through
// the store.
func validatePutMetadata(m internalfiles.FileMetadata) error {
	if m.Path == "" {
		return errors.New("path is required")
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
