package secrets

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/shawnbutts/keystone-core/internal/secrets"
)

// handleSecrets routes GET/POST/DELETE for /api/v1/secrets/{path}.
func (h *Handler) handleSecrets(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/secrets/")
	if path == "" {
		writeError(w, http.StatusBadRequest, "secret path is required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		if r.URL.Query().Get("list") == "true" {
			h.handleSecretList(w, r, path)
		} else {
			h.handleSecretRead(w, r, path)
		}
	case http.MethodPost:
		h.handleSecretWrite(w, r, path)
	case http.MethodDelete:
		h.handleSecretDelete(w, r, path)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

func (h *Handler) handleSecretRead(w http.ResponseWriter, r *http.Request, path string) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets backend not available")
		return
	}

	req := &secrets.SecretRequest{Path: path}

	if v := r.URL.Query().Get("version"); v != "" {
		version, err := strconv.Atoi(v)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid version parameter")
			return
		}
		req.Version = version
	}

	secret, err := h.broker.Read(r.Context(), req)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	resp := SecretResponse{
		Path:      secret.Path,
		Backend:   string(secret.Backend),
		Type:      string(secret.Type),
		Data:      secret.Data,
		Metadata:  secret.Metadata,
		Version:   secret.Version,
		CreatedAt: secret.CreatedAt,
		ExpiresAt: secret.ExpiresAt,
		Renewable: secret.Renewable,
	}
	if secret.Lease != nil {
		resp.LeaseID = secret.Lease.ID
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) handleSecretList(w http.ResponseWriter, r *http.Request, path string) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets backend not available")
		return
	}

	keys, err := h.broker.List(r.Context(), path)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	if keys == nil {
		keys = []string{}
	}

	writeJSON(w, http.StatusOK, SecretListResponse{
		Path: path,
		Keys: keys,
	})
}

// WritableBackend is the interface for backends that support writes.
type WritableBackend interface {
	Write(path string, data map[string]interface{}, metadata map[string]string) error
}

// DeletableBackend is the interface for backends that support deletes.
type DeletableBackend interface {
	Delete(path string) error
}

// resolveBackendByPath attempts to find a backend by extracting the first
// path segment and looking it up via the broker's public GetBackend method.
func (h *Handler) resolveBackendByPath(path string) (secrets.SecretBackend, error) {
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 || parts[0] == "" {
		return nil, secrets.ErrBackendNotFound
	}
	backend, err := h.broker.GetBackend(parts[0])
	if err != nil {
		// Try with the full path as the backend name
		backend, err = h.broker.GetBackend(path)
		if err != nil {
			return nil, secrets.ErrBackendNotFound
		}
	}
	return backend, nil
}

func (h *Handler) handleSecretWrite(w http.ResponseWriter, r *http.Request, path string) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets backend not available")
		return
	}

	var req SecretWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Data == nil {
		writeError(w, http.StatusBadRequest, "data field is required")
		return
	}

	backend, err := h.resolveBackendByPath(path)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	wb, ok := backend.(WritableBackend)
	if !ok {
		writeError(w, http.StatusNotImplemented, "backend does not support writes")
		return
	}

	if err := wb.Write(path, req.Data, req.Metadata); err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"path":    path,
		"written": true,
	})
}

func (h *Handler) handleSecretDelete(w http.ResponseWriter, r *http.Request, path string) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets backend not available")
		return
	}

	backend, err := h.resolveBackendByPath(path)
	if err != nil {
		writeSecretError(w, err)
		return
	}

	db, ok := backend.(DeletableBackend)
	if !ok {
		writeError(w, http.StatusNotImplemented, "backend does not support deletes")
		return
	}

	if err := db.Delete(path); err != nil {
		writeSecretError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"path":    path,
		"deleted": true,
	})
}
