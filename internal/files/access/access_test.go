package access

import (
	"context"
	"regexp"
	"testing"
	"time"
)

func TestIdentity_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		identity *Identity
		expected bool
	}{
		{
			name: "not expired - zero time",
			identity: &Identity{
				ExpiresAt: time.Time{},
			},
			expected: false,
		},
		{
			name: "not expired - future time",
			identity: &Identity{
				ExpiresAt: time.Now().Add(time.Hour),
			},
			expected: false,
		},
		{
			name: "expired - past time",
			identity: &Identity{
				ExpiresAt: time.Now().Add(-time.Hour),
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.identity.IsExpired(); got != tt.expected {
				t.Errorf("IsExpired() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestIdentity_HasRole(t *testing.T) {
	identity := &Identity{
		Roles: []string{"admin", "reader"},
	}

	if !identity.HasRole("admin") {
		t.Error("expected HasRole('admin') to be true")
	}

	if !identity.HasRole("reader") {
		t.Error("expected HasRole('reader') to be true")
	}

	if identity.HasRole("writer") {
		t.Error("expected HasRole('writer') to be false")
	}
}

func TestACLEntry_Compile(t *testing.T) {
	tests := []struct {
		name    string
		entry   *ACLEntry
		wantErr bool
	}{
		{
			name: "valid patterns",
			entry: &ACLEntry{
				IdentityPattern:  "spiffe://example.com/*",
				NamespacePattern: "packages",
				PathPattern:      "/nginx-*.deb",
			},
			wantErr: false,
		},
		{
			name: "empty patterns",
			entry: &ACLEntry{
				IdentityPattern:  "",
				NamespacePattern: "",
				PathPattern:      "",
			},
			wantErr: false,
		},
		{
			name: "special characters escaped",
			entry: &ACLEntry{
				// globToRegex escapes special characters, so [invalid becomes \[invalid
				// which is valid regex - this tests that escaping works
				IdentityPattern: "[invalid",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.entry.Compile()
			if (err != nil) != tt.wantErr {
				t.Errorf("Compile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestACLEntry_Matches(t *testing.T) {
	entry := &ACLEntry{
		IdentityPattern:  "spiffe://example.com/*",
		IdentityType:     "agent",
		Roles:            []string{"reader"},
		NamespacePattern: "packages",
		PathPattern:      "/nginx-*",
		Actions:          []Action{ActionGet},
		Effect:           "allow",
	}
	_ = entry.Compile()

	tests := []struct {
		name     string
		req      *Request
		expected bool
	}{
		{
			name: "matches all criteria",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://example.com/agent1",
					Type:  "agent",
					Roles: []string{"reader"},
				},
				Namespace: "packages",
				Path:      "/nginx-1.24.deb",
				Action:    ActionGet,
			},
			expected: true,
		},
		{
			name: "identity pattern mismatch",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://other.com/agent1",
					Type:  "agent",
					Roles: []string{"reader"},
				},
				Namespace: "packages",
				Path:      "/nginx-1.24.deb",
				Action:    ActionGet,
			},
			expected: false,
		},
		{
			name: "identity type mismatch",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://example.com/user1",
					Type:  "user",
					Roles: []string{"reader"},
				},
				Namespace: "packages",
				Path:      "/nginx-1.24.deb",
				Action:    ActionGet,
			},
			expected: false,
		},
		{
			name: "role mismatch",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://example.com/agent1",
					Type:  "agent",
					Roles: []string{"writer"},
				},
				Namespace: "packages",
				Path:      "/nginx-1.24.deb",
				Action:    ActionGet,
			},
			expected: false,
		},
		{
			name: "namespace mismatch",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://example.com/agent1",
					Type:  "agent",
					Roles: []string{"reader"},
				},
				Namespace: "configs",
				Path:      "/nginx-1.24.deb",
				Action:    ActionGet,
			},
			expected: false,
		},
		{
			name: "path mismatch",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://example.com/agent1",
					Type:  "agent",
					Roles: []string{"reader"},
				},
				Namespace: "packages",
				Path:      "/apache-2.4.deb",
				Action:    ActionGet,
			},
			expected: false,
		},
		{
			name: "action mismatch",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://example.com/agent1",
					Type:  "agent",
					Roles: []string{"reader"},
				},
				Namespace: "packages",
				Path:      "/nginx-1.24.deb",
				Action:    ActionPut,
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := entry.Matches(tt.req); got != tt.expected {
				t.Errorf("Matches() = %v, expected %v", got, tt.expected)
			}
		})
	}
}

func TestACL_Evaluate(t *testing.T) {
	acl := NewACL()

	// Add deny rule with higher priority
	_ = acl.AddEntry(&ACLEntry{
		ID:          "deny-secrets",
		Priority:    100,
		PathPattern: "/secrets/**",
		Effect:      "deny",
		Description: "deny access to secrets",
	})

	// Add allow rule with lower priority
	_ = acl.AddEntry(&ACLEntry{
		ID:              "allow-agents",
		Priority:        50,
		IdentityPattern: "spiffe://example.com/*",
		Effect:          "allow",
		Description:     "allow example.com agents",
	})

	tests := []struct {
		name        string
		req         *Request
		wantAllowed bool
		wantRule    string
	}{
		{
			name: "denied by higher priority rule",
			req: &Request{
				Identity: &Identity{
					ID: "spiffe://example.com/agent1",
				},
				Path: "/secrets/api-key.txt",
			},
			wantAllowed: false,
			wantRule:    "deny-secrets",
		},
		{
			name: "allowed by lower priority rule",
			req: &Request{
				Identity: &Identity{
					ID: "spiffe://example.com/agent1",
				},
				Path: "/packages/nginx.deb",
			},
			wantAllowed: true,
			wantRule:    "allow-agents",
		},
		{
			name: "no matching rule",
			req: &Request{
				Identity: &Identity{
					ID: "spiffe://other.com/agent1",
				},
				Path: "/packages/nginx.deb",
			},
			wantAllowed: false,
			wantRule:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := acl.Evaluate(tt.req)
			if result.Allowed != tt.wantAllowed {
				t.Errorf("Evaluate() Allowed = %v, want %v", result.Allowed, tt.wantAllowed)
			}
			if result.MatchedRule != tt.wantRule {
				t.Errorf("Evaluate() MatchedRule = %v, want %v", result.MatchedRule, tt.wantRule)
			}
		})
	}
}

func TestACL_RemoveEntry(t *testing.T) {
	acl := NewACL()

	_ = acl.AddEntry(&ACLEntry{
		ID:          "test-entry",
		Effect:      "allow",
		Description: "test",
	})

	// Remove existing entry
	if !acl.RemoveEntry("test-entry") {
		t.Error("expected RemoveEntry to return true for existing entry")
	}

	// Remove non-existing entry
	if acl.RemoveEntry("nonexistent") {
		t.Error("expected RemoveEntry to return false for non-existing entry")
	}
}

func TestAuthorizer_Authorize(t *testing.T) {
	authorizer := NewAuthorizer(&AuthorizerConfig{
		DefaultDeny: true,
	})

	// Set up namespace config
	authorizer.SetNamespaceConfig(&NamespaceConfig{
		Name:                  "packages",
		AllowedRoles:          []string{"admin", "reader"},
		RequireAuthentication: true,
	})

	// Add ACL entry
	_ = authorizer.AddACLEntry(&ACLEntry{
		ID:               "allow-readers",
		IdentityPattern:  "spiffe://example.com/*",
		Roles:            []string{"reader"},
		NamespacePattern: "packages",
		Actions:          []Action{ActionGet},
		Effect:           "allow",
		Description:      "allow readers to get packages",
	})

	ctx := context.Background()

	tests := []struct {
		name        string
		req         *Request
		wantAllowed bool
	}{
		{
			name: "allowed - matches ACL",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://example.com/agent1",
					Type:  "agent",
					Roles: []string{"reader"},
				},
				Namespace: "packages",
				Path:      "/nginx.deb",
				Action:    ActionGet,
			},
			wantAllowed: true,
		},
		{
			name: "denied - no identity",
			req: &Request{
				Identity:  nil,
				Namespace: "packages",
				Path:      "/nginx.deb",
				Action:    ActionGet,
			},
			wantAllowed: false,
		},
		{
			name: "denied - expired identity",
			req: &Request{
				Identity: &Identity{
					ID:        "spiffe://example.com/agent1",
					ExpiresAt: time.Now().Add(-time.Hour),
				},
				Namespace: "packages",
				Path:      "/nginx.deb",
				Action:    ActionGet,
			},
			wantAllowed: false,
		},
		{
			name: "denied - wrong role",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://example.com/agent1",
					Type:  "agent",
					Roles: []string{"writer"},
				},
				Namespace: "packages",
				Path:      "/nginx.deb",
				Action:    ActionGet,
			},
			wantAllowed: false,
		},
		{
			name: "denied - wrong action",
			req: &Request{
				Identity: &Identity{
					ID:    "spiffe://example.com/agent1",
					Type:  "agent",
					Roles: []string{"reader"},
				},
				Namespace: "packages",
				Path:      "/nginx.deb",
				Action:    ActionPut,
			},
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := authorizer.Authorize(ctx, tt.req)
			if err != nil {
				t.Fatalf("Authorize() error = %v", err)
			}
			if result.Allowed != tt.wantAllowed {
				t.Errorf("Authorize() Allowed = %v, want %v (reason: %s)", result.Allowed, tt.wantAllowed, result.Reason)
			}
		})
	}
}

func TestAuthorizer_NamespaceConfig_ReadOnly(t *testing.T) {
	authorizer := NewAuthorizer(&AuthorizerConfig{
		DefaultDeny: false,
	})

	authorizer.SetNamespaceConfig(&NamespaceConfig{
		Name:     "readonly",
		ReadOnly: true,
	})

	ctx := context.Background()

	// Read should be allowed
	result, _ := authorizer.Authorize(ctx, &Request{
		Identity:  &Identity{ID: "test"},
		Namespace: "readonly",
		Path:      "/file.txt",
		Action:    ActionGet,
	})
	if !result.Allowed {
		t.Error("expected read to be allowed on read-only namespace")
	}

	// Write should be denied
	result, _ = authorizer.Authorize(ctx, &Request{
		Identity:  &Identity{ID: "test"},
		Namespace: "readonly",
		Path:      "/file.txt",
		Action:    ActionPut,
	})
	if result.Allowed {
		t.Error("expected write to be denied on read-only namespace")
	}
}

func TestAuthorizer_NamespaceConfig_Extensions(t *testing.T) {
	authorizer := NewAuthorizer(&AuthorizerConfig{
		DefaultDeny: false,
	})

	authorizer.SetNamespaceConfig(&NamespaceConfig{
		Name:              "packages",
		AllowedExtensions: []string{"deb", "rpm"},
		DeniedExtensions:  []string{"exe"},
	})

	ctx := context.Background()

	tests := []struct {
		name        string
		path        string
		wantAllowed bool
	}{
		{
			name:        "allowed extension",
			path:        "/nginx.deb",
			wantAllowed: true,
		},
		{
			name:        "denied extension",
			path:        "/malware.exe",
			wantAllowed: false,
		},
		{
			name:        "not in allowed list",
			path:        "/file.tar.gz",
			wantAllowed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, _ := authorizer.Authorize(ctx, &Request{
				Identity:  &Identity{ID: "test"},
				Namespace: "packages",
				Path:      tt.path,
				Action:    ActionGet,
			})
			if result.Allowed != tt.wantAllowed {
				t.Errorf("Authorize() Allowed = %v, want %v", result.Allowed, tt.wantAllowed)
			}
		})
	}
}

func TestRequestSigner(t *testing.T) {
	signer := NewRequestSigner("my-secret-key")

	req := &Request{
		Identity: &Identity{
			ID: "spiffe://example.com/agent1",
		},
		Namespace: "packages",
		Path:      "/nginx.deb",
		Action:    ActionGet,
	}

	// Sign request
	signature := signer.Sign(req)
	if signature == "" {
		t.Error("expected non-empty signature")
	}

	// Verify valid signature
	if !signer.Verify(req, signature) {
		t.Error("expected valid signature to verify")
	}

	// Verify invalid signature
	if signer.Verify(req, "invalid-signature") {
		t.Error("expected invalid signature to fail verification")
	}

	// Modified request should have different signature
	modifiedReq := &Request{
		Identity: &Identity{
			ID: "spiffe://example.com/agent2",
		},
		Namespace: "packages",
		Path:      "/nginx.deb",
		Action:    ActionGet,
	}
	if signer.Verify(modifiedReq, signature) {
		t.Error("expected modified request to fail verification")
	}
}

func TestGlobToRegex(t *testing.T) {
	tests := []struct {
		glob     string
		input    string
		expected bool
	}{
		{"*", "anything", true},
		{"*.txt", "file.txt", true},
		{"*.txt", "file.doc", false},
		{"file*", "file.txt", true},
		{"file*", "other.txt", false},
		{"/path/*", "/path/file", true},
		{"/path/*", "/path/sub/file", false},
		{"/path/**", "/path/sub/file", true},
		{"?at", "cat", true},
		{"?at", "boat", false},
		{"file.txt", "file.txt", true},
		{"file.txt", "file2.txt", false},
	}

	for _, tt := range tests {
		t.Run(tt.glob+"_"+tt.input, func(t *testing.T) {
			pattern := globToRegex(tt.glob)
			matched, err := matchRegex(pattern, tt.input)
			if err != nil {
				t.Fatalf("invalid regex: %v", err)
			}
			if matched != tt.expected {
				t.Errorf("glob=%q input=%q: got %v, want %v (pattern=%q)", tt.glob, tt.input, matched, tt.expected, pattern)
			}
		})
	}
}

func matchRegex(pattern, input string) (bool, error) {
	r, err := compileRegex(pattern)
	if err != nil {
		return false, err
	}
	return r.MatchString(input), nil
}

func compileRegex(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}

func TestGetExtension(t *testing.T) {
	tests := []struct {
		path     string
		expected string
	}{
		{"/file.txt", "txt"},
		{"/path/file.tar.gz", "gz"},
		{"/noext", ""},
		{"/path/", ""},
		{"file.DEB", "DEB"},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := getExtension(tt.path); got != tt.expected {
				t.Errorf("getExtension(%q) = %q, want %q", tt.path, got, tt.expected)
			}
		})
	}
}
