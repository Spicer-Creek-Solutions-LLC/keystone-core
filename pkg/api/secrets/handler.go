// Package secrets exposes REST routes for the secrets domain (Epic 10).
//
// Routes:
//
//	GET    /api/v1/secrets/{path...}     read one secret
//	PUT    /api/v1/secrets/{path...}     write a secret
//	DELETE /api/v1/secrets/{path...}     delete a secret
//	GET    /api/v1/secrets               list (?prefix=...&page_size=...&page_token=...)
//
//	GET    /api/v1/leases                list
//	GET    /api/v1/leases/{id}           get one
//	POST   /api/v1/leases/{id}/renew     renew
//	POST   /api/v1/leases/{id}/revoke    revoke
//
//	POST   /api/v1/transit/encrypt       encrypt
//	POST   /api/v1/transit/decrypt       decrypt
//	POST   /api/v1/transit/sign          sign
//	POST   /api/v1/transit/verify        verify
//
// Secret paths are hierarchical (e.g., `production/db/postgres`) so
// the path-segment routes use Go 1.22+ ServeMux's wildcard pattern
// `{path...}` to match multi-segment paths.
//
// Authentication + RBAC are enforced by the auth interceptor /
// middleware (Epic 03 task 4); this handler trusts incoming requests
// to have already passed those checks.
//
// When the broker / transit / lease manager are not configured
// (`secrets.enabled: false`), the handler returns HTTP 503 Service
// Unavailable on every route.
package secrets

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"go.keystone-core.io/keystone-core/internal/secrets"
	"go.keystone-core.io/keystone-core/internal/state"
)

// Handler exposes REST routes for the secrets domain. broker is the
// task-3 [secrets.Broker]; transit is the task-7 [secrets.TransitBackend];
// leases is the task-6 [secrets.LeaseManager]. Any may be nil — the
// corresponding routes return 503 then.
type Handler struct {
	broker  *secrets.Broker
	transit secrets.TransitBackend
	leases  *secrets.LeaseManager
}

// NewHandler returns a Handler. Pass nil for components that aren't
// configured.
func NewHandler(broker *secrets.Broker, transit secrets.TransitBackend, leases *secrets.LeaseManager) *Handler {
	return &Handler{broker: broker, transit: transit, leases: leases}
}

// Register installs the routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	// KV ops.
	mux.HandleFunc("GET /api/v1/secrets", h.handleListSecrets)
	mux.HandleFunc("GET /api/v1/secrets/{path...}", h.handleGetSecret)
	mux.HandleFunc("PUT /api/v1/secrets/{path...}", h.handleWriteSecret)
	mux.HandleFunc("DELETE /api/v1/secrets/{path...}", h.handleDeleteSecret)

	// Lease ops.
	mux.HandleFunc("GET /api/v1/leases", h.handleListLeases)
	mux.HandleFunc("GET /api/v1/leases/{id}", h.handleGetLease)
	mux.HandleFunc("POST /api/v1/leases/{id}/renew", h.handleRenewLease)
	mux.HandleFunc("POST /api/v1/leases/{id}/revoke", h.handleRevokeLease)

	// Transit ops.
	mux.HandleFunc("POST /api/v1/transit/encrypt", h.handleEncrypt)
	mux.HandleFunc("POST /api/v1/transit/decrypt", h.handleDecrypt)
	mux.HandleFunc("POST /api/v1/transit/sign", h.handleSign)
	mux.HandleFunc("POST /api/v1/transit/verify", h.handleVerify)
}

// ---- request / response shapes -----------------------------------

type secretResponse struct {
	Path          string            `json:"path"`
	Data          map[string]any    `json:"data,omitempty"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	Version       uint64            `json:"version,omitempty"`
	LeaseID       string            `json:"lease_id,omitempty"`
	LeaseDuration string            `json:"lease_duration,omitempty"`
	Renewable     bool              `json:"renewable,omitempty"`
	CreatedAt     time.Time         `json:"created_at,omitempty"`
	UpdatedAt     time.Time         `json:"updated_at,omitempty"`
}

type writeRequest struct {
	Data       map[string]any    `json:"data"`
	Labels     map[string]string `json:"labels,omitempty"`
	TTLSeconds int               `json:"ttl_seconds,omitempty"`
}

type listResponse struct {
	Entries    []listEntryDTO `json:"entries"`
	NextCursor string         `json:"next_cursor,omitempty"`
}

type listEntryDTO struct {
	Path      string            `json:"path"`
	Version   uint64            `json:"version,omitempty"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	UpdatedAt time.Time         `json:"updated_at,omitempty"`
}

type leaseDTO struct {
	ID            string    `json:"id"`
	SecretPath    string    `json:"secret_path,omitempty"`
	Backend       string    `json:"backend,omitempty"`
	IssuedFor     string    `json:"issued_for,omitempty"`
	IssuedAt      time.Time `json:"issued_at,omitempty"`
	ExpiresAt     time.Time `json:"expires_at,omitempty"`
	Duration      string    `json:"duration,omitempty"`
	Renewable     bool      `json:"renewable"`
	State         string    `json:"state,omitempty"`
	RenewCount    int       `json:"renew_count,omitempty"`
	LastRenewedAt time.Time `json:"last_renewed_at,omitempty"`
}

type transitRequest struct {
	Key        string `json:"key"`
	Plaintext  []byte `json:"plaintext,omitempty"`
	Ciphertext string `json:"ciphertext,omitempty"`
	Message    []byte `json:"message,omitempty"`
	Signature  string `json:"signature,omitempty"`
	Context    []byte `json:"context,omitempty"`
	Algorithm  string `json:"algorithm,omitempty"`
}

type encryptResponseDTO struct {
	Ciphertext string `json:"ciphertext"`
	KeyVersion int    `json:"key_version"`
}

type decryptResponseDTO struct {
	Plaintext []byte `json:"plaintext"`
}

type signResponseDTO struct {
	Signature  string `json:"signature"`
	KeyVersion int    `json:"key_version"`
}

type verifyResponseDTO struct {
	Valid bool `json:"valid"`
}

type errorDTO struct {
	Error string `json:"error"`
}

// ---- KV handlers -------------------------------------------------

func (h *Handler) handleGetSecret(w http.ResponseWriter, r *http.Request) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets disabled")
		return
	}
	path := r.PathValue("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := secrets.GetSecretRequest{Path: path}
	if v := r.URL.Query().Get("version"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid version %q: %v", v, err))
			return
		}
		req.Version = parsed
	}
	if r.URL.Query().Get("refresh") == "true" {
		req.Refresh = true
	}
	secret, err := h.broker.GetSecret(r.Context(), req)
	if err != nil {
		writeSecretsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, secretToDTO(secret))
}

func (h *Handler) handleWriteSecret(w http.ResponseWriter, r *http.Request) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets disabled")
		return
	}
	path := r.PathValue("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	var req writeRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	meta := make(map[string]string, len(req.Labels)+1)
	for k, v := range req.Labels {
		meta[k] = v
	}
	if req.TTLSeconds > 0 {
		meta["ttl_seconds"] = strconv.Itoa(req.TTLSeconds)
	}
	out, err := h.broker.WriteSecret(r.Context(), secrets.WriteSecretRequest{
		Path:     path,
		Data:     req.Data,
		Metadata: meta,
	})
	if err != nil {
		writeSecretsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, secretToDTO(out))
}

func (h *Handler) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets disabled")
		return
	}
	req := secrets.ListSecretsRequest{
		Prefix: r.URL.Query().Get("prefix"),
		Cursor: r.URL.Query().Get("page_token"),
	}
	if pageSize := r.URL.Query().Get("page_size"); pageSize != "" {
		n, err := strconv.Atoi(pageSize)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid page_size %q", pageSize))
			return
		}
		req.Limit = n
	}
	resp, err := h.broker.ListSecrets(r.Context(), req)
	if err != nil {
		writeSecretsError(w, err)
		return
	}
	out := listResponse{NextCursor: resp.NextCursor}
	for _, e := range resp.Entries {
		out.Entries = append(out.Entries, listEntryDTO{
			Path: e.Path, Version: e.Version, Metadata: e.Metadata, UpdatedAt: e.UpdatedAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets disabled")
		return
	}
	path := r.PathValue("path")
	if path == "" {
		writeError(w, http.StatusBadRequest, "path is required")
		return
	}
	req := secrets.DeleteSecretRequest{Path: path}
	if r.URL.Query().Get("destroy") == "true" {
		req.Destroy = true
	}
	if v := r.URL.Query().Get("version"); v != "" {
		parsed, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid version %q", v))
			return
		}
		req.Version = parsed
	}
	if err := h.broker.DeleteSecret(r.Context(), req); err != nil {
		writeSecretsError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Lease handlers ----------------------------------------------

func (h *Handler) handleListLeases(w http.ResponseWriter, r *http.Request) {
	if h.leases == nil {
		writeError(w, http.StatusServiceUnavailable, "lease manager disabled")
		return
	}
	filter := state.LeaseFilter{
		Backend:    r.URL.Query().Get("backend"),
		State:      r.URL.Query().Get("state"),
		PathPrefix: r.URL.Query().Get("prefix"),
	}
	if v := r.URL.Query().Get("page_size"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid page_size %q", v))
			return
		}
		filter.Limit = n
	}
	if r.URL.Query().Get("include_revoked") == "true" {
		filter.IncludeRevoked = true
	}
	leases, err := h.leases.List(r.Context(), filter)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	out := make([]leaseDTO, 0, len(leases))
	for _, l := range leases {
		out = append(out, leaseToDTO(l))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *Handler) handleGetLease(w http.ResponseWriter, r *http.Request) {
	if h.leases == nil {
		writeError(w, http.StatusServiceUnavailable, "lease manager disabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "lease id is required")
		return
	}
	lease, err := h.leases.GetLease(r.Context(), id)
	if err != nil {
		if errors.Is(err, state.ErrNotFound) {
			writeError(w, http.StatusNotFound, fmt.Sprintf("lease %q not found", id))
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, leaseToDTO(lease))
}

func (h *Handler) handleRenewLease(w http.ResponseWriter, r *http.Request) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets disabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "lease id is required")
		return
	}
	info, err := h.broker.RenewLease(r.Context(), secrets.RenewLeaseRequest{LeaseID: id})
	if err != nil {
		writeSecretsError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, leaseInfoToDTO(info))
}

func (h *Handler) handleRevokeLease(w http.ResponseWriter, r *http.Request) {
	if h.broker == nil {
		writeError(w, http.StatusServiceUnavailable, "secrets disabled")
		return
	}
	id := r.PathValue("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "lease id is required")
		return
	}
	if err := h.broker.RevokeLease(r.Context(), secrets.RevokeLeaseRequest{LeaseID: id}); err != nil {
		writeSecretsError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- Transit handlers --------------------------------------------

func (h *Handler) handleEncrypt(w http.ResponseWriter, r *http.Request) {
	if h.transit == nil {
		writeError(w, http.StatusServiceUnavailable, "transit disabled")
		return
	}
	var req transitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	resp, err := h.transit.Encrypt(r.Context(), secrets.EncryptRequest{
		Key:   req.Key,
		Items: []secrets.EncryptInput{{Plaintext: req.Plaintext, Context: req.Context}},
	})
	if err != nil {
		writeSecretsError(w, err)
		return
	}
	if len(resp.Results) == 0 || resp.Results[0].Err != "" {
		writeError(w, http.StatusInternalServerError, transitFirstErr(resp.Results, "empty response"))
		return
	}
	writeJSON(w, http.StatusOK, encryptResponseDTO{
		Ciphertext: resp.Results[0].Ciphertext,
		KeyVersion: resp.Results[0].KeyVersion,
	})
}

func (h *Handler) handleDecrypt(w http.ResponseWriter, r *http.Request) {
	if h.transit == nil {
		writeError(w, http.StatusServiceUnavailable, "transit disabled")
		return
	}
	var req transitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	resp, err := h.transit.Decrypt(r.Context(), secrets.DecryptRequest{
		Key:   req.Key,
		Items: []secrets.DecryptInput{{Ciphertext: req.Ciphertext, Context: req.Context}},
	})
	if err != nil {
		writeSecretsError(w, err)
		return
	}
	if len(resp.Results) == 0 || resp.Results[0].Err != "" {
		writeError(w, http.StatusInternalServerError, decryptFirstErr(resp.Results, "empty response"))
		return
	}
	writeJSON(w, http.StatusOK, decryptResponseDTO{Plaintext: resp.Results[0].Plaintext})
}

func (h *Handler) handleSign(w http.ResponseWriter, r *http.Request) {
	if h.transit == nil {
		writeError(w, http.StatusServiceUnavailable, "transit disabled")
		return
	}
	var req transitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	resp, err := h.transit.Sign(r.Context(), secrets.SignRequest{
		Key:           req.Key,
		HashAlgorithm: req.Algorithm,
		Items:         []secrets.SignInput{{Input: req.Message}},
	})
	if err != nil {
		writeSecretsError(w, err)
		return
	}
	if len(resp.Results) == 0 || resp.Results[0].Err != "" {
		writeError(w, http.StatusInternalServerError, signFirstErr(resp.Results, "empty response"))
		return
	}
	writeJSON(w, http.StatusOK, signResponseDTO{
		Signature:  resp.Results[0].Signature,
		KeyVersion: resp.Results[0].KeyVersion,
	})
}

func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	if h.transit == nil {
		writeError(w, http.StatusServiceUnavailable, "transit disabled")
		return
	}
	var req transitRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.Key == "" {
		writeError(w, http.StatusBadRequest, "key is required")
		return
	}
	resp, err := h.transit.Verify(r.Context(), secrets.VerifyRequest{
		Key:           req.Key,
		HashAlgorithm: req.Algorithm,
		Items: []secrets.VerifyInput{
			{Input: req.Message, Signature: req.Signature},
		},
	})
	if err != nil {
		writeSecretsError(w, err)
		return
	}
	if len(resp.Results) == 0 {
		writeError(w, http.StatusInternalServerError, "empty response")
		return
	}
	writeJSON(w, http.StatusOK, verifyResponseDTO{Valid: resp.Results[0].Valid})
}

// ---- helpers -----------------------------------------------------

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

// writeSecretsError funnels [secrets] sentinels into the right HTTP
// status code. Mirrors the gRPC error-translation table in
// controlplane/grpc_secrets_server.go.
func writeSecretsError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, secrets.ErrSecretNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, secrets.ErrLeaseNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, secrets.ErrLeaseExpired):
		writeError(w, http.StatusPreconditionFailed, err.Error())
	case errors.Is(err, secrets.ErrLeaseNotRenewable):
		writeError(w, http.StatusPreconditionFailed, err.Error())
	case errors.Is(err, secrets.ErrBackendNotStarted):
		writeError(w, http.StatusServiceUnavailable, err.Error())
	case errors.Is(err, secrets.ErrInvalidBackend):
		writeError(w, http.StatusBadRequest, err.Error())
	default:
		writeError(w, http.StatusInternalServerError, err.Error())
	}
}

func secretToDTO(s *secrets.Secret) secretResponse {
	if s == nil {
		return secretResponse{}
	}
	out := secretResponse{
		Path:      s.Path,
		Data:      s.Data,
		Metadata:  s.Metadata,
		Version:   s.Version,
		LeaseID:   s.LeaseID,
		Renewable: s.Renewable,
		CreatedAt: s.CreatedAt,
		UpdatedAt: s.UpdatedAt,
	}
	if s.LeaseDuration > 0 {
		out.LeaseDuration = s.LeaseDuration.String()
	}
	return out
}

func leaseToDTO(l secrets.Lease) leaseDTO {
	out := leaseDTO{
		ID:            l.ID,
		SecretPath:    l.SecretPath,
		Backend:       l.Backend,
		IssuedFor:     l.IssuedFor,
		IssuedAt:      l.IssuedAt,
		ExpiresAt:     l.ExpiresAt,
		Renewable:     l.Renewable,
		State:         l.State.String(),
		RenewCount:    l.RenewCount,
		LastRenewedAt: l.LastRenewedAt,
	}
	if l.Duration > 0 {
		out.Duration = l.Duration.String()
	}
	return out
}

func leaseInfoToDTO(info *secrets.LeaseInfo) leaseDTO {
	if info == nil {
		return leaseDTO{}
	}
	out := leaseDTO{
		ID:            info.ID,
		SecretPath:    info.SecretPath,
		Backend:       info.Backend,
		IssuedAt:      info.IssuedAt,
		ExpiresAt:     info.ExpiresAt,
		Renewable:     info.Renewable,
		State:         info.State.String(),
		RenewCount:    info.RenewCount,
		LastRenewedAt: info.LastRenewedAt,
	}
	if info.Duration > 0 {
		out.Duration = info.Duration.String()
	}
	return out
}

func transitFirstErr(rs []secrets.EncryptResult, fallback string) string {
	for _, r := range rs {
		if r.Err != "" {
			return r.Err
		}
	}
	return fallback
}

func decryptFirstErr(rs []secrets.DecryptResult, fallback string) string {
	for _, r := range rs {
		if r.Err != "" {
			return r.Err
		}
	}
	return fallback
}

func signFirstErr(rs []secrets.SignResult, fallback string) string {
	for _, r := range rs {
		if r.Err != "" {
			return r.Err
		}
	}
	return fallback
}

