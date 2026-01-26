package policy

import (
	"context"
	"testing"
)

func TestNewRBACContext(t *testing.T) {
	principal := &Principal{
		ID:   "user-1",
		Type: "user",
		Name: "Test User",
	}

	ctx := NewRBACContext(principal)

	if ctx.Principal != principal {
		t.Error("Expected principal to be set")
	}
	if ctx.EffectiveRoles == nil {
		t.Error("Expected effective roles to be initialized")
	}
	if ctx.EffectivePermissions == nil {
		t.Error("Expected effective permissions to be initialized")
	}
	if ctx.EvaluatedAt.IsZero() {
		t.Error("Expected EvaluatedAt to be set")
	}
}

func TestRBACContext_HasPermission(t *testing.T) {
	ctx := NewRBACContext(&Principal{ID: "user-1", Type: "user"})
	ctx.EffectivePermissions = []Permission{
		{Resource: "agents", Action: "read"},
		{Resource: "agents", Action: "write"},
		{Resource: "commands", Action: "*"},
	}

	tests := []struct {
		resource string
		action   string
		expected bool
	}{
		{"agents", "read", true},
		{"agents", "write", true},
		{"agents", "delete", false},
		{"commands", "read", true},
		{"commands", "execute", true},
		{"states", "read", false},
	}

	for _, tt := range tests {
		result := ctx.HasPermission(tt.resource, tt.action)
		if result != tt.expected {
			t.Errorf("HasPermission(%s, %s) = %v, want %v",
				tt.resource, tt.action, result, tt.expected)
		}
	}
}

func TestRBACContext_HasRole(t *testing.T) {
	ctx := NewRBACContext(&Principal{ID: "user-1", Type: "user"})
	ctx.EffectiveRoles = []*Role{
		{ID: "admin", Name: "Admin"},
		{ID: "operator", Name: "Operator"},
	}

	if !ctx.HasRole("admin") {
		t.Error("Expected HasRole(admin) to be true")
	}
	if !ctx.HasRole("operator") {
		t.Error("Expected HasRole(operator) to be true")
	}
	if ctx.HasRole("viewer") {
		t.Error("Expected HasRole(viewer) to be false")
	}
}

func TestRBACContext_HasAnyRole(t *testing.T) {
	ctx := NewRBACContext(&Principal{ID: "user-1", Type: "user"})
	ctx.EffectiveRoles = []*Role{
		{ID: "operator", Name: "Operator"},
	}

	if !ctx.HasAnyRole("admin", "operator") {
		t.Error("Expected HasAnyRole(admin, operator) to be true")
	}
	if ctx.HasAnyRole("admin", "viewer") {
		t.Error("Expected HasAnyRole(admin, viewer) to be false")
	}
}

func TestRBACContext_HasAllRoles(t *testing.T) {
	ctx := NewRBACContext(&Principal{ID: "user-1", Type: "user"})
	ctx.EffectiveRoles = []*Role{
		{ID: "admin", Name: "Admin"},
		{ID: "operator", Name: "Operator"},
	}

	if !ctx.HasAllRoles("admin", "operator") {
		t.Error("Expected HasAllRoles(admin, operator) to be true")
	}
	if ctx.HasAllRoles("admin", "viewer") {
		t.Error("Expected HasAllRoles(admin, viewer) to be false")
	}
}

func TestRBACContext_GetAttribute(t *testing.T) {
	ctx := NewRBACContext(&Principal{
		ID:   "user-1",
		Type: "user",
		Attributes: map[string]interface{}{
			"department": "engineering",
		},
	})
	ctx.SessionAttributes = map[string]interface{}{
		"session_id": "sess-123",
	}

	// Session attribute takes precedence
	val, ok := ctx.GetAttribute("session_id")
	if !ok || val != "sess-123" {
		t.Error("Expected session attribute")
	}

	// Principal attribute
	val, ok = ctx.GetAttribute("department")
	if !ok || val != "engineering" {
		t.Error("Expected principal attribute")
	}

	// Non-existent attribute
	_, ok = ctx.GetAttribute("unknown")
	if ok {
		t.Error("Expected unknown attribute to not exist")
	}
}

func TestRBACContext_GetHighestPriorityRole(t *testing.T) {
	ctx := NewRBACContext(&Principal{ID: "user-1", Type: "user"})
	ctx.EffectiveRoles = []*Role{
		{ID: "viewer", Name: "Viewer", Priority: 10},
		{ID: "admin", Name: "Admin", Priority: 100},
		{ID: "operator", Name: "Operator", Priority: 50},
	}

	highest := ctx.GetHighestPriorityRole()
	if highest == nil || highest.ID != "admin" {
		t.Errorf("Expected admin to be highest priority, got %v", highest)
	}

	// Empty roles
	ctx.EffectiveRoles = nil
	if ctx.GetHighestPriorityRole() != nil {
		t.Error("Expected nil for empty roles")
	}
}

func TestRBACContext_ToMap(t *testing.T) {
	ctx := NewRBACContext(&Principal{
		ID:    "user-1",
		Type:  "user",
		Name:  "Test User",
		Roles: []string{"admin"},
		Attributes: map[string]interface{}{
			"department": "engineering",
		},
	})
	ctx.EffectiveRoles = []*Role{
		{ID: "admin", Name: "Admin"},
	}
	ctx.EffectivePermissions = []Permission{
		{Resource: "agents", Action: "read"},
	}
	ctx.SessionAttributes = map[string]interface{}{
		"session_id": "sess-123",
	}

	m := ctx.ToMap()

	if m["principal_id"] != "user-1" {
		t.Error("Expected principal_id")
	}
	if m["principal_type"] != "user" {
		t.Error("Expected principal_type")
	}

	roles, ok := m["roles"].([]string)
	if !ok || len(roles) != 1 || roles[0] != "admin" {
		t.Error("Expected roles")
	}

	permissions, ok := m["permissions"].([]map[string]interface{})
	if !ok || len(permissions) != 1 {
		t.Error("Expected permissions")
	}
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		pattern  string
		value    string
		expected bool
	}{
		{"*", "anything", true},
		{"agents", "agents", true},
		{"agents", "commands", false},
		{"agents:*", "agents:read", true},
		{"agents:*", "agents:write", true},
		{"agents:*", "commands:read", false},
		{"*:read", "agents:read", true},
		{"*:read", "commands:read", true},
		{"*:read", "agents:write", false},
	}

	for _, tt := range tests {
		result := matchesPattern(tt.pattern, tt.value)
		if result != tt.expected {
			t.Errorf("matchesPattern(%q, %q) = %v, want %v",
				tt.pattern, tt.value, result, tt.expected)
		}
	}
}

func TestNewRBACStore(t *testing.T) {
	store := NewRBACStore()

	if store == nil {
		t.Fatal("Expected store to be created")
	}
	if store.roles == nil {
		t.Error("Expected roles map to be initialized")
	}
	if store.principals == nil {
		t.Error("Expected principals map to be initialized")
	}
}

func TestRBACStore_RegisterGetRole(t *testing.T) {
	store := NewRBACStore()

	role := &Role{
		ID:       "admin",
		Name:     "Administrator",
		Priority: 100,
		Permissions: []Permission{
			{Resource: "*", Action: "*"},
		},
	}

	if err := store.RegisterRole(role); err != nil {
		t.Fatalf("Failed to register role: %v", err)
	}

	retrieved, ok := store.GetRole("admin")
	if !ok {
		t.Fatal("Expected role to be found")
	}
	if retrieved.Name != "Administrator" {
		t.Error("Expected role name to match")
	}
	if retrieved.CreatedAt.IsZero() {
		t.Error("Expected CreatedAt to be set")
	}
}

func TestRBACStore_RegisterRole_Validation(t *testing.T) {
	store := NewRBACStore()

	// Empty ID should fail
	err := store.RegisterRole(&Role{Name: "Test"})
	if err == nil {
		t.Error("Expected error for empty role ID")
	}
}

func TestRBACStore_ListRoles(t *testing.T) {
	store := NewRBACStore()

	store.RegisterRole(&Role{ID: "viewer", Name: "Viewer", Priority: 10})
	store.RegisterRole(&Role{ID: "admin", Name: "Admin", Priority: 100})
	store.RegisterRole(&Role{ID: "operator", Name: "Operator", Priority: 50})

	roles := store.ListRoles()

	if len(roles) != 3 {
		t.Errorf("Expected 3 roles, got %d", len(roles))
	}

	// Should be sorted by priority (descending)
	if roles[0].ID != "admin" {
		t.Error("Expected admin to be first (highest priority)")
	}
}

func TestRBACStore_DeleteRole(t *testing.T) {
	store := NewRBACStore()

	store.RegisterRole(&Role{ID: "admin", Name: "Admin"})

	if _, ok := store.GetRole("admin"); !ok {
		t.Fatal("Expected role to exist")
	}

	store.DeleteRole("admin")

	if _, ok := store.GetRole("admin"); ok {
		t.Error("Expected role to be deleted")
	}
}

func TestRBACStore_RegisterGetPrincipal(t *testing.T) {
	store := NewRBACStore()

	principal := &Principal{
		ID:    "user-1",
		Type:  "user",
		Name:  "Test User",
		Roles: []string{"admin"},
	}

	if err := store.RegisterPrincipal(principal); err != nil {
		t.Fatalf("Failed to register principal: %v", err)
	}

	retrieved, ok := store.GetPrincipal("user-1")
	if !ok {
		t.Fatal("Expected principal to be found")
	}
	if retrieved.Name != "Test User" {
		t.Error("Expected principal name to match")
	}
}

func TestRBACStore_RegisterPrincipal_Validation(t *testing.T) {
	store := NewRBACStore()

	err := store.RegisterPrincipal(&Principal{Name: "Test"})
	if err == nil {
		t.Error("Expected error for empty principal ID")
	}
}

func TestRBACStore_BuildRBACContext(t *testing.T) {
	store := NewRBACStore()

	// Register roles with inheritance
	store.RegisterRole(&Role{
		ID:       "base",
		Name:     "Base",
		Priority: 10,
		Permissions: []Permission{
			{Resource: "agents", Action: "read"},
		},
	})

	store.RegisterRole(&Role{
		ID:             "operator",
		Name:           "Operator",
		Priority:       50,
		InheritedRoles: []string{"base"},
		Permissions: []Permission{
			{Resource: "agents", Action: "write"},
		},
	})

	// Register principal
	store.RegisterPrincipal(&Principal{
		ID:    "user-1",
		Type:  "user",
		Roles: []string{"operator"},
		DirectPermissions: []Permission{
			{Resource: "admin", Action: "login"},
		},
	})

	ctx, err := store.BuildRBACContext("user-1")
	if err != nil {
		t.Fatalf("Failed to build RBAC context: %v", err)
	}

	// Should have both roles (operator and inherited base)
	if len(ctx.EffectiveRoles) != 2 {
		t.Errorf("Expected 2 effective roles, got %d", len(ctx.EffectiveRoles))
	}

	// Should have all permissions
	if len(ctx.EffectivePermissions) != 3 {
		t.Errorf("Expected 3 effective permissions, got %d", len(ctx.EffectivePermissions))
	}

	// Verify permissions
	if !ctx.HasPermission("agents", "read") {
		t.Error("Expected agents:read permission (inherited)")
	}
	if !ctx.HasPermission("agents", "write") {
		t.Error("Expected agents:write permission")
	}
	if !ctx.HasPermission("admin", "login") {
		t.Error("Expected admin:login permission (direct)")
	}
}

func TestRBACStore_BuildRBACContext_NotFound(t *testing.T) {
	store := NewRBACStore()

	_, err := store.BuildRBACContext("unknown")
	if err == nil {
		t.Error("Expected error for unknown principal")
	}
}

func TestRBACStore_AssignRole(t *testing.T) {
	store := NewRBACStore()

	store.RegisterRole(&Role{ID: "admin", Name: "Admin"})
	store.RegisterPrincipal(&Principal{ID: "user-1", Type: "user"})

	if err := store.AssignRole("user-1", "admin"); err != nil {
		t.Fatalf("Failed to assign role: %v", err)
	}

	principal, _ := store.GetPrincipal("user-1")
	if len(principal.Roles) != 1 || principal.Roles[0] != "admin" {
		t.Error("Expected admin role to be assigned")
	}

	// Duplicate assignment should be no-op
	if err := store.AssignRole("user-1", "admin"); err != nil {
		t.Fatalf("Duplicate assignment should not error: %v", err)
	}
	if len(principal.Roles) != 1 {
		t.Error("Expected still only 1 role after duplicate assignment")
	}
}

func TestRBACStore_AssignRole_NotFound(t *testing.T) {
	store := NewRBACStore()

	store.RegisterPrincipal(&Principal{ID: "user-1", Type: "user"})

	err := store.AssignRole("user-1", "unknown-role")
	if err == nil {
		t.Error("Expected error for unknown role")
	}

	err = store.AssignRole("unknown-user", "admin")
	if err == nil {
		t.Error("Expected error for unknown principal")
	}
}

func TestRBACStore_RemoveRole(t *testing.T) {
	store := NewRBACStore()

	store.RegisterRole(&Role{ID: "admin", Name: "Admin"})
	store.RegisterPrincipal(&Principal{
		ID:    "user-1",
		Type:  "user",
		Roles: []string{"admin"},
	})

	if err := store.RemoveRole("user-1", "admin"); err != nil {
		t.Fatalf("Failed to remove role: %v", err)
	}

	principal, _ := store.GetPrincipal("user-1")
	if len(principal.Roles) != 0 {
		t.Error("Expected no roles after removal")
	}
}

func TestRBACStore_GrantRevokePermission(t *testing.T) {
	store := NewRBACStore()

	store.RegisterPrincipal(&Principal{ID: "user-1", Type: "user"})

	// Grant permission
	if err := store.GrantPermission("user-1", Permission{Resource: "agents", Action: "read"}); err != nil {
		t.Fatalf("Failed to grant permission: %v", err)
	}

	principal, _ := store.GetPrincipal("user-1")
	if len(principal.DirectPermissions) != 1 {
		t.Error("Expected 1 direct permission")
	}

	// Revoke permission
	if err := store.RevokePermission("user-1", "agents", "read"); err != nil {
		t.Fatalf("Failed to revoke permission: %v", err)
	}

	principal, _ = store.GetPrincipal("user-1")
	if len(principal.DirectPermissions) != 0 {
		t.Error("Expected no direct permissions after revoke")
	}
}

func TestWithRBACContext(t *testing.T) {
	ctx := context.Background()
	rbac := NewRBACContext(&Principal{ID: "user-1", Type: "user"})

	ctx = WithRBACContext(ctx, rbac)

	retrieved := RBACContextFromContext(ctx)
	if retrieved == nil {
		t.Fatal("Expected RBAC context from context")
	}
	if retrieved.Principal.ID != "user-1" {
		t.Error("Expected principal ID to match")
	}
}

func TestRBACContextFromContext_Empty(t *testing.T) {
	ctx := context.Background()

	retrieved := RBACContextFromContext(ctx)
	if retrieved != nil {
		t.Error("Expected nil RBAC context from empty context")
	}
}

func TestNewRBACEvaluationInput(t *testing.T) {
	input := &EvaluationInput{
		Action: "read",
		User:   "user-1",
	}
	rbac := NewRBACContext(&Principal{ID: "user-1", Type: "user"})

	rbacInput := NewRBACEvaluationInput(input, rbac)

	if rbacInput.EvaluationInput != input {
		t.Error("Expected input to be set")
	}
	if rbacInput.RBACContext != rbac {
		t.Error("Expected RBAC context to be set")
	}
}

func TestRBACEvaluationInput_ToEvaluationInput(t *testing.T) {
	input := &EvaluationInput{
		Action: "read",
		User:   "user-1",
	}
	rbac := NewRBACContext(&Principal{ID: "user-1", Type: "user"})
	rbac.EffectiveRoles = []*Role{{ID: "admin", Name: "Admin"}}

	rbacInput := NewRBACEvaluationInput(input, rbac)
	enriched := rbacInput.ToEvaluationInput()

	if enriched.Context == nil {
		t.Fatal("Expected context to be set")
	}

	rbacMap, ok := enriched.Context["rbac"].(map[string]interface{})
	if !ok {
		t.Fatal("Expected rbac in context")
	}

	if rbacMap["principal_id"] != "user-1" {
		t.Error("Expected principal_id in rbac context")
	}
}

func TestNewRBACEnrichedEngine(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	store := NewRBACStore()

	enriched := NewRBACEnrichedEngine(engine, store)

	if enriched.engine != engine {
		t.Error("Expected engine to be set")
	}
	if enriched.rbacStore != store {
		t.Error("Expected store to be set")
	}
}

func TestRBACEnrichedEngine_CheckPermission(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	store := NewRBACStore()

	store.RegisterRole(&Role{
		ID:   "admin",
		Name: "Admin",
		Permissions: []Permission{
			{Resource: "*", Action: "*"},
		},
	})
	store.RegisterPrincipal(&Principal{
		ID:    "user-1",
		Type:  "user",
		Roles: []string{"admin"},
	})

	enriched := NewRBACEnrichedEngine(engine, store)

	hasPermission, err := enriched.CheckPermission("user-1", "agents", "read")
	if err != nil {
		t.Fatalf("Failed to check permission: %v", err)
	}
	if !hasPermission {
		t.Error("Expected permission to be granted")
	}
}

func TestRBACEnrichedEngine_GetPrincipalRoles(t *testing.T) {
	registry := NewRegistry()
	engine := NewPolicyEngine(registry)
	store := NewRBACStore()

	store.RegisterRole(&Role{ID: "admin", Name: "Admin"})
	store.RegisterRole(&Role{ID: "operator", Name: "Operator"})
	store.RegisterPrincipal(&Principal{
		ID:    "user-1",
		Type:  "user",
		Roles: []string{"admin", "operator"},
	})

	enriched := NewRBACEnrichedEngine(engine, store)

	roles, err := enriched.GetPrincipalRoles("user-1")
	if err != nil {
		t.Fatalf("Failed to get roles: %v", err)
	}
	if len(roles) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(roles))
	}
}

func TestCreateStandardRoles(t *testing.T) {
	store := NewRBACStore()

	if err := CreateStandardRoles(store); err != nil {
		t.Fatalf("Failed to create standard roles: %v", err)
	}

	// Verify admin role
	admin, ok := store.GetRole(StandardRoles.Admin)
	if !ok {
		t.Fatal("Expected admin role")
	}
	if admin.Priority != 100 {
		t.Errorf("Expected admin priority 100, got %d", admin.Priority)
	}

	// Verify viewer role
	viewer, ok := store.GetRole(StandardRoles.Viewer)
	if !ok {
		t.Fatal("Expected viewer role")
	}
	if viewer.Priority != 10 {
		t.Errorf("Expected viewer priority 10, got %d", viewer.Priority)
	}

	// Verify operator role
	operator, ok := store.GetRole(StandardRoles.Operator)
	if !ok {
		t.Fatal("Expected operator role")
	}
	if operator.Priority != 50 {
		t.Errorf("Expected operator priority 50, got %d", operator.Priority)
	}

	// Verify service role
	service, ok := store.GetRole(StandardRoles.Service)
	if !ok {
		t.Fatal("Expected service role")
	}
	if service.Priority != 30 {
		t.Errorf("Expected service priority 30, got %d", service.Priority)
	}
}
