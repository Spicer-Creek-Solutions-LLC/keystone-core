package config

import (
	"slices"
	"strings"
	"testing"
)

func TestGitOpsConfig_Defaults(t *testing.T) {
	t.Parallel()
	c := defaultConfig()
	if c.GitOps.Webhook.Enabled {
		t.Errorf("Webhook.Enabled = true, want false (opt-in: opens a port)")
	}
	if c.GitOps.Webhook.Addr != ":8081" {
		t.Errorf("Webhook.Addr = %q, want \":8081\" (must not collide with :8080 REST API)", c.GitOps.Webhook.Addr)
	}
	if c.GitOps.Webhook.Path != "/webhooks" {
		t.Errorf("Webhook.Path = %q, want \"/webhooks\"", c.GitOps.Webhook.Path)
	}
	if c.GitOps.Webhook.MaxBodyBytes != 1<<20 {
		t.Errorf("Webhook.MaxBodyBytes = %d, want %d", c.GitOps.Webhook.MaxBodyBytes, 1<<20)
	}
}

func TestGitOpsConfig_DefaultAddrDoesNotCollideWithRESTAPI(t *testing.T) {
	t.Parallel()
	c := defaultConfig()
	if c.GitOps.Webhook.Addr == ":8080" || c.Server.HTTPPort != 8080 {
		t.Fatalf("guard: HTTPPort=%d webhookAddr=%q — receiver must not bind the REST API port",
			c.Server.HTTPPort, c.GitOps.Webhook.Addr)
	}
}

func TestGitOpsConfig_Validate_DisabledIsAlwaysOK(t *testing.T) {
	t.Parallel()
	// Disabled with otherwise-invalid fields still validates.
	c := GitOpsConfig{Webhook: GitOpsWebhookConfig{Enabled: false, Path: "no-slash", MaxBodyBytes: -1}}
	if err := c.Validate(); err != nil {
		t.Errorf("disabled validates: got %v", err)
	}
}

func TestGitOpsConfig_Validate_Enabled(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		cfg     GitOpsWebhookConfig
		wantErr string
	}{
		{
			name: "valid",
			cfg:  GitOpsWebhookConfig{Enabled: true, Addr: ":8081", Path: "/webhooks", MaxBodyBytes: 1024},
		},
		{
			name:    "empty addr",
			cfg:     GitOpsWebhookConfig{Enabled: true, Addr: "", Path: "/webhooks", MaxBodyBytes: 1024},
			wantErr: "gitops.webhook.addr",
		},
		{
			name:    "relative path",
			cfg:     GitOpsWebhookConfig{Enabled: true, Addr: ":8081", Path: "webhooks", MaxBodyBytes: 1024},
			wantErr: "absolute path",
		},
		{
			name:    "empty path",
			cfg:     GitOpsWebhookConfig{Enabled: true, Addr: ":8081", Path: "", MaxBodyBytes: 1024},
			wantErr: "absolute path",
		},
		{
			name:    "non-positive body cap",
			cfg:     GitOpsWebhookConfig{Enabled: true, Addr: ":8081", Path: "/webhooks", MaxBodyBytes: 0},
			wantErr: "max_body_bytes",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := (&GitOpsConfig{Webhook: tc.cfg}).Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && err == nil:
				t.Fatalf("want error containing %q, got nil", tc.wantErr)
			case tc.wantErr != "" && !strings.Contains(err.Error(), tc.wantErr):
				t.Fatalf("error %q does not contain %q", err, tc.wantErr)
			}
		})
	}
}

func TestGitOpsConfig_Validate_Sources(t *testing.T) {
	t.Parallel()
	base := GitOpsWebhookConfig{Enabled: true, Addr: ":8081", Path: "/webhooks", MaxBodyBytes: 1024}
	cases := []struct {
		name    string
		sources map[string]GitOpsSourceAuthConfig
		wantErr string
	}{
		{"valid hmac", map[string]GitOpsSourceAuthConfig{"github": {Method: "hmac", Secret: "x"}}, ""},
		{"valid none no secret", map[string]GitOpsSourceAuthConfig{"flux": {Method: "none"}}, ""},
		{"unknown provider", map[string]GitOpsSourceAuthConfig{"bitbucket": {Method: "none"}}, "unknown provider"},
		{"missing method", map[string]GitOpsSourceAuthConfig{"github": {Method: ""}}, "method is required"},
		{"unknown method", map[string]GitOpsSourceAuthConfig{"github": {Method: "mtls"}}, "unknown method"},
		{"hmac no secret", map[string]GitOpsSourceAuthConfig{"github": {Method: "hmac"}}, "requires a non-empty secret"},
		{"bearer no secret", map[string]GitOpsSourceAuthConfig{"gitlab": {Method: "bearer"}}, "requires a non-empty secret"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			wc := base
			wc.Sources = tc.sources
			err := (&GitOpsConfig{Webhook: wc}).Validate()
			switch {
			case tc.wantErr == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)):
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestGitOpsConfig_UnauthenticatedWebhookSources(t *testing.T) {
	t.Parallel()
	if got := (&GitOpsConfig{Webhook: GitOpsWebhookConfig{Enabled: false}}).UnauthenticatedWebhookSources(); got != nil {
		t.Errorf("disabled = %v, want nil", got)
	}

	c := &GitOpsConfig{Webhook: GitOpsWebhookConfig{
		Enabled: true,
		Sources: map[string]GitOpsSourceAuthConfig{
			"github": {Method: "hmac", Secret: "x"},
			"gitlab": {Method: "none"},
		},
	}}
	got := c.UnauthenticatedWebhookSources()
	slices.Sort(got)
	want := []string{"argocd", "flux", "gitlab"} // github authed; gitlab none; argocd/flux absent
	if !slices.Equal(got, want) {
		t.Errorf("open = %v, want %v", got, want)
	}
}

func TestProductionWarnings_GitOpsUnauthenticated(t *testing.T) {
	t.Parallel()
	c := defaultConfig()
	c.Mode = ModeProduction
	c.GitOps.Webhook.Enabled = true
	c.GitOps.Webhook.Sources = map[string]GitOpsSourceAuthConfig{
		"github": {Method: "hmac", Secret: "x"},
		"gitlab": {Method: "hmac", Secret: "y"},
		"argocd": {Method: "hmac", Secret: "z"},
		"flux":   {Method: "hmac", Secret: "w"},
	}
	for _, msg := range c.ProductionWarnings() {
		if strings.Contains(msg, "gitops webhook") {
			t.Fatalf("unexpected gitops warning when all sources authed: %q", msg)
		}
	}

	c.GitOps.Webhook.Sources["flux"] = GitOpsSourceAuthConfig{Method: "none"}
	var found bool
	for _, msg := range c.ProductionWarnings() {
		if strings.Contains(msg, "gitops webhook receiver enabled with unauthenticated sources") &&
			strings.Contains(msg, "flux") {
			found = true
		}
	}
	if !found {
		t.Errorf("want gitops unauthenticated warning naming flux, warnings = %v", c.ProductionWarnings())
	}
}
