package cluster

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/shawnbutts/keystone-core/internal/audit"
	"github.com/shawnbutts/keystone-core/internal/cluster"
	"github.com/shawnbutts/keystone-core/internal/cluster/token"
)

// GenerateTokenRequest is the request body for POST /api/v1/cluster/tokens.
type GenerateTokenRequest struct {
	Label   string `json:"label"`
	TTL     string `json:"ttl"`
	MaxUses int    `json:"max_uses"`
}

// GenerateTokenResponse is returned once at token creation time and includes
// the raw plaintext token.
type GenerateTokenResponse struct {
	ID        string    `json:"id"`
	Token     string    `json:"token"`
	Label     string    `json:"label"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   int       `json:"max_uses"`
	CreatedAt time.Time `json:"created_at"`
}

// TokenResponse represents a token in list/get responses (no raw token).
type TokenResponse struct {
	ID        string    `json:"id"`
	Label     string    `json:"label"`
	ExpiresAt time.Time `json:"expires_at"`
	MaxUses   int       `json:"max_uses"`
	UsedCount int       `json:"used_count"`
	CreatedBy string    `json:"created_by"`
	Revoked   bool      `json:"revoked"`
	CreatedAt time.Time `json:"created_at"`
	Valid     bool      `json:"valid"`
}

// JoinRequest is the request body for POST /api/v1/cluster/join.
type JoinRequest struct {
	ClusterAddress string `json:"cluster_address"`
	Token          string `json:"token"`
	AdvertiseAddr  string `json:"advertise_addr"`
}

// JoinResponse is the response body for POST /api/v1/cluster/join.
type JoinResponse struct {
	Message  string `json:"message"`
	MemberID string `json:"member_id"`
}

// handleTokens dispatches GET/POST on /api/v1/cluster/tokens.
func (h *Handler) handleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		h.listTokens(w, r)
	case http.MethodPost:
		h.createToken(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleTokenResource dispatches GET/DELETE on /api/v1/cluster/tokens/{id}.
func (h *Handler) handleTokenResource(w http.ResponseWriter, r *http.Request) {
	tokenID := r.URL.Path[len("/api/v1/cluster/tokens/"):]
	if tokenID == "" {
		writeError(w, http.StatusBadRequest, "Token ID required")
		return
	}

	switch r.Method {
	case http.MethodGet:
		h.getToken(w, r, tokenID)
	case http.MethodDelete:
		h.revokeToken(w, r, tokenID)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func (h *Handler) createToken(w http.ResponseWriter, r *http.Request) {
	if h.tokenStore == nil {
		writeError(w, http.StatusServiceUnavailable, "Token store not configured")
		return
	}

	var req GenerateTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	var ttl time.Duration
	if req.TTL != "" {
		var err error
		ttl, err = time.ParseDuration(req.TTL)
		if err != nil {
			writeError(w, http.StatusBadRequest, "Invalid TTL: "+err.Error())
			return
		}
	}

	jt, rawToken, err := token.NewJoinToken(req.Label, "", ttl, req.MaxUses)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to generate token")
		return
	}

	if err := h.tokenStore.Create(r.Context(), jt); err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to store token")
		return
	}

	h.auditTokenCreate(r, jt)

	writeJSON(w, http.StatusCreated, GenerateTokenResponse{
		ID:        jt.ID,
		Token:     rawToken,
		Label:     jt.Label,
		ExpiresAt: jt.ExpiresAt,
		MaxUses:   jt.MaxUses,
		CreatedAt: jt.CreatedAt,
	})
}

func (h *Handler) listTokens(w http.ResponseWriter, r *http.Request) {
	if h.tokenStore == nil {
		writeError(w, http.StatusServiceUnavailable, "Token store not configured")
		return
	}

	tokens, err := h.tokenStore.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "Failed to list tokens")
		return
	}

	resp := make([]TokenResponse, 0, len(tokens))
	for _, t := range tokens {
		resp = append(resp, tokenToResponse(t))
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) getToken(w http.ResponseWriter, r *http.Request, id string) {
	if h.tokenStore == nil {
		writeError(w, http.StatusServiceUnavailable, "Token store not configured")
		return
	}

	jt, err := h.tokenStore.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, token.ErrTokenNotFound) {
			writeError(w, http.StatusNotFound, "Token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to get token")
		return
	}

	writeJSON(w, http.StatusOK, tokenToResponse(jt))
}

func (h *Handler) revokeToken(w http.ResponseWriter, r *http.Request, id string) {
	if h.tokenStore == nil {
		writeError(w, http.StatusServiceUnavailable, "Token store not configured")
		return
	}

	if err := h.tokenStore.Revoke(r.Context(), id); err != nil {
		if errors.Is(err, token.ErrTokenNotFound) {
			writeError(w, http.StatusNotFound, "Token not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "Failed to revoke token")
		return
	}

	h.auditTokenRevoke(r, id)

	w.WriteHeader(http.StatusNoContent)
}

// handleJoin handles POST /api/v1/cluster/join.
func (h *Handler) handleJoin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req JoinRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Validate token if token store is configured.
	var tokenID string
	if h.tokenStore != nil {
		if req.Token == "" {
			h.auditJoin(r, "", audit.OutcomeDenied)
			writeError(w, http.StatusUnauthorized, "Join token required")
			return
		}

		jt, err := h.tokenStore.Lookup(r.Context(), req.Token)
		if err != nil {
			if errors.Is(err, token.ErrTokenNotFound) {
				h.auditJoin(r, "", audit.OutcomeDenied)
				writeError(w, http.StatusUnauthorized, "Invalid join token")
				return
			}
			writeError(w, http.StatusInternalServerError, "Failed to validate token")
			return
		}

		if !jt.IsValid() {
			h.auditJoin(r, jt.ID, audit.OutcomeFailure)
			writeTokenError(w, jt)
			return
		}

		if err := h.tokenStore.IncrementUses(r.Context(), jt.ID); err != nil {
			status := tokenErrorStatus(err)
			if status != http.StatusInternalServerError {
				h.auditJoin(r, jt.ID, audit.OutcomeFailure)
				writeError(w, status, err.Error())
				return
			}
			writeError(w, http.StatusInternalServerError, "Failed to record token use")
			return
		}

		tokenID = jt.ID
	}

	// Add member if membership manager is available.
	var memberID string
	if h.membership != nil {
		addReq := &cluster.AddMemberRequest{
			Address: req.AdvertiseAddr,
		}
		member, err := h.membership.AddMember(r.Context(), addReq)
		if err != nil {
			h.auditJoin(r, tokenID, audit.OutcomeFailure)
			writeError(w, http.StatusInternalServerError, "Failed to add member: "+err.Error())
			return
		}
		memberID = member.ID
	}

	h.auditJoin(r, tokenID, audit.OutcomeSuccess)

	writeJSON(w, http.StatusOK, JoinResponse{
		Message:  "Successfully joined cluster",
		MemberID: memberID,
	})
}

func tokenToResponse(jt *token.JoinToken) TokenResponse {
	return TokenResponse{
		ID:        jt.ID,
		Label:     jt.Label,
		ExpiresAt: jt.ExpiresAt,
		MaxUses:   jt.MaxUses,
		UsedCount: jt.UsedCount,
		CreatedBy: jt.CreatedBy,
		Revoked:   jt.Revoked,
		CreatedAt: jt.CreatedAt,
		Valid:     jt.IsValid(),
	}
}

// writeTokenError maps a token's invalid state to the appropriate HTTP error.
func writeTokenError(w http.ResponseWriter, jt *token.JoinToken) {
	switch {
	case jt.IsExpired():
		writeError(w, http.StatusGone, "Token has expired")
	case jt.Revoked:
		writeError(w, http.StatusForbidden, "Token has been revoked")
	case !jt.HasUsesRemaining():
		writeError(w, http.StatusForbidden, "Token has reached max uses")
	default:
		writeError(w, http.StatusForbidden, "Token is invalid")
	}
}

// tokenErrorStatus maps token sentinel errors to HTTP status codes.
func tokenErrorStatus(err error) int {
	switch {
	case errors.Is(err, token.ErrTokenNotFound):
		return http.StatusNotFound
	case errors.Is(err, token.ErrTokenExpired):
		return http.StatusGone
	case errors.Is(err, token.ErrTokenRevoked):
		return http.StatusForbidden
	case errors.Is(err, token.ErrTokenExhausted):
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

// Audit helpers — all silently skip if auditLogger is nil.

func (h *Handler) auditTokenCreate(r *http.Request, jt *token.JoinToken) {
	if h.auditLogger == nil {
		return
	}
	_ = h.auditLogger.LogAdmin(r.Context(), audit.ActionCreate, audit.OutcomeSuccess,
		&audit.Actor{Type: "api", IP: r.RemoteAddr},
		&audit.Resource{Type: "cluster_join_token", ID: jt.ID},
		map[string]interface{}{
			"label":      jt.Label,
			"max_uses":   jt.MaxUses,
			"expires_at": jt.ExpiresAt.Format(time.RFC3339),
		},
	)
}

func (h *Handler) auditTokenRevoke(r *http.Request, tokenID string) {
	if h.auditLogger == nil {
		return
	}
	_ = h.auditLogger.LogAdmin(r.Context(), audit.ActionRevoke, audit.OutcomeSuccess,
		&audit.Actor{Type: "api", IP: r.RemoteAddr},
		&audit.Resource{Type: "cluster_join_token", ID: tokenID},
		nil,
	)
}

func (h *Handler) auditJoin(r *http.Request, tokenID string, outcome audit.Outcome) {
	if h.auditLogger == nil {
		return
	}
	details := map[string]interface{}{
		"remote_addr": r.RemoteAddr,
	}
	if tokenID != "" {
		details["token_id"] = tokenID
	}
	_ = h.auditLogger.LogAuth(r.Context(), audit.ActionLogin, outcome,
		&audit.Actor{Type: "node", IP: r.RemoteAddr},
		details,
	)
}
