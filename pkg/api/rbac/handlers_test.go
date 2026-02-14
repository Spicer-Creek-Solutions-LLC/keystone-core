package rbac

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func mustNewHandler(t *testing.T) *Handler {
	t.Helper()
	h, err := NewHandler()
	if err != nil {
		t.Fatalf("NewHandler() unexpected error: %v", err)
	}
	return h
}

func TestNewHandler_ReturnsNoError(t *testing.T) {
	h, err := NewHandler()
	if err != nil {
		t.Fatalf("NewHandler() returned error: %v", err)
	}
	if h == nil {
		t.Fatal("NewHandler() returned nil handler")
	}
}

func TestListRoles_Success(t *testing.T) {
	h := mustNewHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rbac/roles", nil)
	w := httptest.NewRecorder()

	h.handleListRoles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var roles []RoleResponse
	if err := json.Unmarshal(w.Body.Bytes(), &roles); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(roles) != 4 {
		t.Fatalf("expected 4 standard roles, got %d", len(roles))
	}

	// Roles should be sorted by priority descending: admin(100), operator(50), service(30), viewer(10)
	expectedOrder := []string{"admin", "operator", "service", "viewer"}
	for i, expected := range expectedOrder {
		if roles[i].ID != expected {
			t.Errorf("role[%d]: expected %s, got %s", i, expected, roles[i].ID)
		}
	}

	// Without show_permissions, permissions should be empty
	for _, role := range roles {
		if len(role.Permissions) > 0 {
			t.Errorf("role %s: expected no permissions without show_permissions param, got %d", role.ID, len(role.Permissions))
		}
	}
}

func TestListRoles_ShowPermissions(t *testing.T) {
	h := mustNewHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rbac/roles?show_permissions=true", nil)
	w := httptest.NewRecorder()

	h.handleListRoles(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var roles []RoleResponse
	if err := json.Unmarshal(w.Body.Bytes(), &roles); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Admin should have wildcard permission
	for _, role := range roles {
		if role.ID == "admin" {
			if len(role.Permissions) == 0 {
				t.Error("admin role should have permissions when show_permissions=true")
			}
			if role.Permissions[0].Resource != "*" || role.Permissions[0].Action != "*" {
				t.Errorf("admin role expected *:* permission, got %s:%s", role.Permissions[0].Resource, role.Permissions[0].Action)
			}
		}
	}
}

func TestListRoles_MethodNotAllowed(t *testing.T) {
	h := mustNewHandler(t)

	for _, method := range []string{http.MethodPost, http.MethodPut, http.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			req := httptest.NewRequest(method, "/api/v1/rbac/roles", nil)
			w := httptest.NewRecorder()

			h.handleListRoles(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("expected 405 for %s, got %d", method, w.Code)
			}
		})
	}
}

func TestListRoles_RegisterRoutes(t *testing.T) {
	h := mustNewHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rbac/roles", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 via mux, got %d", w.Code)
	}
}

func TestExport_Success(t *testing.T) {
	h := mustNewHandler(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rbac/export", nil)
	w := httptest.NewRecorder()

	h.handleExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var export ExportResponse
	if err := json.Unmarshal(w.Body.Bytes(), &export); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(export.Roles) != 4 {
		t.Errorf("expected 4 roles in export, got %d", len(export.Roles))
	}

	// Principals should be empty (no principals registered in standard setup)
	if len(export.Principals) != 0 {
		t.Errorf("expected 0 principals in export, got %d", len(export.Principals))
	}

	// Roles should include permissions (full export)
	for _, role := range export.Roles {
		if role.ID == "admin" && len(role.Permissions) == 0 {
			t.Error("admin role should have permissions in export")
		}
	}
}

func TestExport_MethodNotAllowed(t *testing.T) {
	h := mustNewHandler(t)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/rbac/export", nil)
	w := httptest.NewRecorder()

	h.handleExport(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestExport_ViaRoutes(t *testing.T) {
	h := mustNewHandler(t)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/rbac/export", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 via mux, got %d", w.Code)
	}
}
