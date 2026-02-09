// Package registry provides container registry authentication support for Keystone.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GCRCredentialProvider provides GCP Container Registry credentials using instance metadata.
type GCRCredentialProvider struct {
	httpClient *http.Client
}

// NewGCRCredentialProvider creates a new GCR credential provider.
func NewGCRCredentialProvider() *GCRCredentialProvider {
	return &GCRCredentialProvider{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewGCRCredentialProviderWithClient creates a GCR provider with a custom HTTP client.
func NewGCRCredentialProviderWithClient(client *http.Client) *GCRCredentialProvider {
	return &GCRCredentialProvider{
		httpClient: client,
	}
}

// GetCredential retrieves GCR credentials using GCP instance metadata.
func (p *GCRCredentialProvider) GetCredential(ctx context.Context, registry string) (*Credential, error) {
	// Get access token from GCP metadata service
	token, expiresAt, err := p.getAccessToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get GCP access token: %w", err)
	}

	// Get project ID for metadata
	projectID, _ := p.getMetadata(ctx, "/project/project-id")

	return &Credential{
		Type:      TypeGCR,
		Registry:  registry,
		Username:  "_token",
		Password:  token,
		Token:     token,
		ExpiresAt: expiresAt,
		ProjectID: projectID,
	}, nil
}

// IsAvailable checks if GCR credentials are available (running on GCP).
func (p *GCRCredentialProvider) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.getMetadata(ctx, "/project/project-id")
	return err == nil
}

// Type returns the registry type this provider handles.
func (p *GCRCredentialProvider) Type() Type {
	return TypeGCR
}

// getAccessToken retrieves an access token from GCP metadata service.
func (p *GCRCredentialProvider) getAccessToken(ctx context.Context) (string, time.Time, error) {
	url := "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token"

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("token request failed: %d - %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", time.Time{}, err
	}

	expiresAt := time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	return tokenResp.AccessToken, expiresAt, nil
}

// getMetadata retrieves metadata from GCP metadata service.
func (p *GCRCredentialProvider) getMetadata(ctx context.Context, path string) (string, error) {
	url := "http://metadata.google.internal/computeMetadata/v1" + path

	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata request failed: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	return strings.TrimSpace(string(data)), err
}

// MatchesRegistry checks if this provider can handle the given registry.
func (p *GCRCredentialProvider) MatchesRegistry(registry string) bool {
	registry = strings.ToLower(registry)
	return strings.Contains(registry, "gcr.io") ||
		strings.Contains(registry, "pkg.dev") ||
		strings.Contains(registry, "-docker.pkg.dev")
}

// GCRArtifactRegistryCredentialProvider provides Google Artifact Registry credentials.
// This is an alias for GCRCredentialProvider as they use the same authentication mechanism.
type GCRArtifactRegistryCredentialProvider = GCRCredentialProvider

// NewGCRArtifactRegistryCredentialProvider creates a provider for Google Artifact Registry.
func NewGCRArtifactRegistryCredentialProvider() *GCRArtifactRegistryCredentialProvider {
	return NewGCRCredentialProvider()
}
