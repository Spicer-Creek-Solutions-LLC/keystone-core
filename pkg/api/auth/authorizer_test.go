package auth

import (
	"context"
	"testing"
)

func TestRBACAuthorizer_DefaultPermissions(t *testing.T) {
	auth := NewRBACAuthorizer(nil)

	tests := []struct {
		name          string
		method        string
		role          Role
		shouldSucceed bool
	}{
		// ExecuteCommand requires operator
		{"admin can execute command", "/kscore.v1.ControlPlaneService/ExecuteCommand", RoleAdmin, true},
		{"operator can execute command", "/kscore.v1.ControlPlaneService/ExecuteCommand", RoleOperator, true},
		{"readonly cannot execute command", "/kscore.v1.ControlPlaneService/ExecuteCommand", RoleReadonly, false},

		// ListAgents requires readonly
		{"admin can list agents", "/kscore.v1.ControlPlaneService/ListAgents", RoleAdmin, true},
		{"operator can list agents", "/kscore.v1.ControlPlaneService/ListAgents", RoleOperator, true},
		{"readonly can list agents", "/kscore.v1.ControlPlaneService/ListAgents", RoleReadonly, true},

		// Cluster rebalance requires admin
		{"admin can rebalance", "/kscore.cluster.v1.ClusterService/Rebalance", RoleAdmin, true},
		{"operator cannot rebalance", "/kscore.cluster.v1.ClusterService/Rebalance", RoleOperator, false},
		{"readonly cannot rebalance", "/kscore.cluster.v1.ClusterService/Rebalance", RoleReadonly, false},

		// Unknown method defaults to admin
		{"admin can unknown method", "/unknown.Service/Unknown", RoleAdmin, true},
		{"operator cannot unknown method", "/unknown.Service/Unknown", RoleOperator, false},
		{"readonly cannot unknown method", "/unknown.Service/Unknown", RoleReadonly, false},
	}

	ctx := context.Background()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			principal := &Principal{Role: tt.role}
			err := auth.Authorize(ctx, principal, tt.method)

			if tt.shouldSucceed && err != nil {
				t.Errorf("Authorize() error = %v, want nil", err)
			}
			if !tt.shouldSucceed && err == nil {
				t.Error("Authorize() should have failed")
			}
		})
	}
}

func TestRBACAuthorizer_BypassMethods(t *testing.T) {
	bypassMethods := []string{
		"/health.HealthService/Check",
		"/health.HealthService/Watch",
	}

	auth := NewRBACAuthorizer(bypassMethods)
	ctx := context.Background()

	// Bypass methods should work without principal
	err := auth.Authorize(ctx, nil, "/health.HealthService/Check")
	if err != nil {
		t.Errorf("Authorize() bypass method error = %v, want nil", err)
	}

	// Non-bypass method without principal should fail
	err = auth.Authorize(ctx, nil, "/kscore.v1.ControlPlaneService/ListAgents")
	if err == nil {
		t.Error("Authorize() non-bypass without principal should fail")
	}
}

func TestRBACAuthorizer_SetPermission(t *testing.T) {
	auth := NewRBACAuthorizer(nil)

	// Add a custom permission
	auth.SetPermission("/custom.Service/Method", RoleReadonly)

	ctx := context.Background()
	principal := &Principal{Role: RoleReadonly}

	// Should work for readonly now
	err := auth.Authorize(ctx, principal, "/custom.Service/Method")
	if err != nil {
		t.Errorf("Authorize() custom method error = %v, want nil", err)
	}

	// Verify with GetPermission
	role, ok := auth.GetPermission("/custom.Service/Method")
	if !ok {
		t.Error("GetPermission() should return true for custom method")
	}
	if role != RoleReadonly {
		t.Errorf("GetPermission() role = %v, want readonly", role)
	}
}

func TestRBACAuthorizer_AddBypassMethod(t *testing.T) {
	auth := NewRBACAuthorizer(nil)
	ctx := context.Background()

	method := "/custom.Service/Bypass"

	// Initially should require auth
	err := auth.Authorize(ctx, nil, method)
	if err == nil {
		t.Error("Authorize() should fail before adding bypass")
	}

	// Add bypass
	auth.AddBypassMethod(method)

	// Now should work without principal
	err = auth.Authorize(ctx, nil, method)
	if err != nil {
		t.Errorf("Authorize() after bypass error = %v", err)
	}

	// Verify with IsBypassMethod
	if !auth.IsBypassMethod(method) {
		t.Error("IsBypassMethod() should return true")
	}
}

func TestParseMethodName(t *testing.T) {
	tests := []struct {
		fullMethod      string
		expectedService string
		expectedMethod  string
	}{
		{"/kscore.v1.ControlPlaneService/ListAgents", "kscore.v1.ControlPlaneService", "ListAgents"},
		{"/package.Service/Method", "package.Service", "Method"},
		{"Service/Method", "Service", "Method"},
		{"NoSlash", "NoSlash", ""},
	}

	for _, tt := range tests {
		t.Run(tt.fullMethod, func(t *testing.T) {
			service, method := ParseMethodName(tt.fullMethod)
			if service != tt.expectedService {
				t.Errorf("service = %v, want %v", service, tt.expectedService)
			}
			if method != tt.expectedMethod {
				t.Errorf("method = %v, want %v", method, tt.expectedMethod)
			}
		})
	}
}

func TestIsBypassMethod(t *testing.T) {
	tests := []struct {
		name          string
		method        string
		bypassMethods []string
		expected      bool
	}{
		{
			name:          "exact match",
			method:        "/health.Service/Check",
			bypassMethods: []string{"/health.Service/Check"},
			expected:      true,
		},
		{
			name:          "no match",
			method:        "/other.Service/Method",
			bypassMethods: []string{"/health.Service/Check"},
			expected:      false,
		},
		{
			name:          "wildcard match",
			method:        "/health.Service/Check",
			bypassMethods: []string{"/health.Service/*"},
			expected:      true,
		},
		{
			name:          "wildcard other method",
			method:        "/health.Service/Watch",
			bypassMethods: []string{"/health.Service/*"},
			expected:      true,
		},
		{
			name:          "wildcard no match",
			method:        "/other.Service/Method",
			bypassMethods: []string{"/health.Service/*"},
			expected:      false,
		},
		{
			name:          "empty bypass list",
			method:        "/any.Service/Method",
			bypassMethods: nil,
			expected:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isBypassMethod(tt.method, tt.bypassMethods)
			if result != tt.expected {
				t.Errorf("isBypassMethod() = %v, want %v", result, tt.expected)
			}
		})
	}
}
