package webhook

import (
	"net/http"
	"testing"
)

func TestHandlerRegistry(t *testing.T) {
	registry := NewHandlerRegistry()

	// Register handlers
	argoHandler := &ArgoCDHandler{}
	fluxHandler := &FluxHandler{}
	githubHandler := &GitHubHandler{}
	gitlabHandler := &GitLabHandler{}

	registry.Register(argoHandler)
	registry.Register(fluxHandler)
	registry.Register(githubHandler)
	registry.Register(gitlabHandler)

	// Test retrieval
	tests := []struct {
		name        string
		webhookType Type
		wantFound   bool
	}{
		{"get argocd", WebhookTypeArgoCD, true},
		{"get flux", WebhookTypeFlux, true},
		{"get github", WebhookTypeGitHub, true},
		{"get gitlab", WebhookTypeGitLab, true},
		{"get unknown", "unknown", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, found := registry.Get(tt.webhookType)
			if found != tt.wantFound {
				t.Errorf("Get(%s) found = %v, want %v", tt.webhookType, found, tt.wantFound)
			}
		})
	}
}

func TestDetectType(t *testing.T) {
	registry := NewHandlerRegistry()

	tests := []struct {
		name    string
		headers map[string]string
		want    Type
		wantErr bool
	}{
		{
			name:    "detect argocd via header",
			headers: map[string]string{"X-Argo-CD-Webhook": "true"},
			want:    WebhookTypeArgoCD,
			wantErr: false,
		},
		{
			name:    "detect flux via header",
			headers: map[string]string{"X-Flux-Event": "reconciliation"},
			want:    WebhookTypeFlux,
			wantErr: false,
		},
		{
			name:    "detect github via header",
			headers: map[string]string{"X-GitHub-Event": "push"},
			want:    WebhookTypeGitHub,
			wantErr: false,
		},
		{
			name:    "detect gitlab via header",
			headers: map[string]string{"X-Gitlab-Event": "Push Hook"},
			want:    WebhookTypeGitLab,
			wantErr: false,
		},
		{
			name:    "detect github via user-agent",
			headers: map[string]string{"User-Agent": "GitHub-Hookshot/abc123"},
			want:    WebhookTypeGitHub,
			wantErr: false,
		},
		{
			name:    "unknown webhook",
			headers: map[string]string{"User-Agent": "Unknown"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &http.Request{
				Header: http.Header{},
			}
			for k, v := range tt.headers {
				req.Header.Set(k, v)
			}

			got, err := registry.DetectType(req)
			if (err != nil) != tt.wantErr {
				t.Errorf("DetectType() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != tt.want {
				t.Errorf("DetectType() = %v, want %v", got, tt.want)
			}
		})
	}
}
