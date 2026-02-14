// Package rbac provides HTTP handlers for RBAC role management REST API endpoints.
package rbac

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/shawnbutts/keystone-core/internal/policy"
	"github.com/shawnbutts/keystone-core/pkg/api/apierror"
)

// Handler provides HTTP handlers for RBAC API endpoints.
type Handler struct {
	store *policy.RBACStore
}

// NewHandler creates a new RBAC API handler with standard roles pre-loaded.
func NewHandler() (*Handler, error) {
	store := policy.NewRBACStore()
	if err := policy.CreateStandardRoles(store); err != nil {
		return nil, fmt.Errorf("failed to create standard RBAC roles: %w", err)
	}
	return &Handler{store: store}, nil
}

// RegisterRoutes registers the RBAC API routes with the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/v1/rbac/roles", h.handleListRoles)
	mux.HandleFunc("/api/v1/rbac/export", h.handleExport)
}

// RoleResponse is the JSON response for a single role.
type RoleResponse struct {
	ID          string               `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description,omitempty"`
	Priority    int                  `json:"priority"`
	Permissions []policy.Permission  `json:"permissions,omitempty"`
}

func (h *Handler) handleListRoles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apierror.Write(w, http.StatusMethodNotAllowed, "only GET is allowed")
		return
	}

	showPerms := r.URL.Query().Get("show_permissions") == "true"

	roles := h.store.ListRoles()
	resp := make([]RoleResponse, len(roles))
	for i, role := range roles {
		resp[i] = RoleResponse{
			ID:          role.ID,
			Name:        role.Name,
			Description: role.Description,
			Priority:    role.Priority,
		}
		if showPerms {
			resp[i].Permissions = role.Permissions
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(resp)
}

// ExportResponse is the full RBAC export for backup/audit.
type ExportResponse struct {
	Roles      []*policy.Role      `json:"roles"`
	Principals []*policy.Principal `json:"principals"`
}

func (h *Handler) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apierror.Write(w, http.StatusMethodNotAllowed, "only GET is allowed")
		return
	}

	resp := ExportResponse{
		Roles:      h.store.ListRoles(),
		Principals: h.store.ListPrincipals(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(resp)
}
