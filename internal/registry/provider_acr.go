// Package registry provides container registry authentication support for Keystone.
package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ACRCredentialProvider provides Azure Container Registry credentials using managed identity.
type ACRCredentialProvider struct {
	httpClient *http.Client
}

// NewACRCredentialProvider creates a new ACR credential provider.
func NewACRCredentialProvider() *ACRCredentialProvider {
	return &ACRCredentialProvider{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// NewACRCredentialProviderWithClient creates an ACR provider with a custom HTTP client.
func NewACRCredentialProviderWithClient(client *http.Client) *ACRCredentialProvider {
	return &ACRCredentialProvider{
		httpClient: client,
	}
}

// GetCredential retrieves ACR credentials using Azure managed identity.
func (p *ACRCredentialProvider) GetCredential(ctx context.Context, registry string) (*Credential, error) {
	// Normalize registry URL
	loginServer := normalizeACRRegistry(registry)

	// Get AAD access token for ACR
	aadToken, err := p.getAADToken(ctx, loginServer)
	if err != nil {
		return nil, fmt.Errorf("failed to get AAD token: %w", err)
	}

	// Exchange AAD token for ACR refresh token
	refreshToken, expiresAt, err := p.exchangeAADTokenForACR(ctx, loginServer, aadToken)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token for ACR: %w", err)
	}

	// Get subscription info for metadata
	subscriptionID, _ := p.getSubscriptionID(ctx)

	return &Credential{
		Type:           RegistryTypeACR,
		Registry:       registry,
		Username:       "00000000-0000-0000-0000-000000000000",
		Password:       refreshToken,
		Token:          refreshToken,
		ExpiresAt:      expiresAt,
		SubscriptionID: subscriptionID,
	}, nil
}

// IsAvailable checks if ACR credentials are available (running on Azure).
func (p *ACRCredentialProvider) IsAvailable() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := p.getSubscriptionID(ctx)
	return err == nil
}

// RegistryType returns the registry type this provider handles.
func (p *ACRCredentialProvider) RegistryType() RegistryType {
	return RegistryTypeACR
}

// getAADToken retrieves an AAD access token for ACR from Azure managed identity.
func (p *ACRCredentialProvider) getAADToken(ctx context.Context, loginServer string) (string, error) {
	// The resource for ACR is the login server URL
	resource := fmt.Sprintf("https://%s", loginServer)

	tokenURL := fmt.Sprintf(
		"http://169.254.169.254/metadata/identity/oauth2/token?api-version=2018-02-01&resource=%s",
		url.QueryEscape(resource),
	)

	req, err := http.NewRequestWithContext(ctx, "GET", tokenURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata", "true")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("token request failed: %d - %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   string `json:"expires_in"`
		ExpiresOn   string `json:"expires_on"`
		Resource    string `json:"resource"`
		TokenType   string `json:"token_type"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	return tokenResp.AccessToken, nil
}

// exchangeAADTokenForACR exchanges an AAD token for an ACR refresh token.
func (p *ACRCredentialProvider) exchangeAADTokenForACR(ctx context.Context, loginServer, aadToken string) (string, time.Time, error) {
	// ACR OAuth2 token exchange endpoint
	exchangeURL := fmt.Sprintf("https://%s/oauth2/exchange", loginServer)

	// Build form data
	formData := url.Values{}
	formData.Set("grant_type", "access_token")
	formData.Set("service", loginServer)
	formData.Set("access_token", aadToken)

	req, err := http.NewRequestWithContext(ctx, "POST", exchangeURL, strings.NewReader(formData.Encode()))
	if err != nil {
		return "", time.Time{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", time.Time{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", time.Time{}, fmt.Errorf("token exchange failed: %d - %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		RefreshToken string `json:"refresh_token"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", time.Time{}, err
	}

	// ACR refresh tokens are valid for about 3 hours
	expiresAt := time.Now().Add(3 * time.Hour)

	return tokenResp.RefreshToken, expiresAt, nil
}

// getSubscriptionID retrieves the Azure subscription ID from instance metadata.
func (p *ACRCredentialProvider) getSubscriptionID(ctx context.Context) (string, error) {
	url := "http://169.254.169.254/metadata/instance/compute?api-version=2021-02-01"

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Metadata", "true")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata request failed: %d", resp.StatusCode)
	}

	var metadata struct {
		SubscriptionID string `json:"subscriptionId"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return "", err
	}

	return metadata.SubscriptionID, nil
}

// MatchesRegistry checks if this provider can handle the given registry.
func (p *ACRCredentialProvider) MatchesRegistry(registry string) bool {
	return strings.Contains(strings.ToLower(registry), ".azurecr.io")
}

// normalizeACRRegistry normalizes an ACR registry URL to just the login server.
func normalizeACRRegistry(registry string) string {
	// Remove protocol if present
	registry = strings.TrimPrefix(registry, "https://")
	registry = strings.TrimPrefix(registry, "http://")

	// Remove trailing slashes and paths
	if idx := strings.Index(registry, "/"); idx != -1 {
		registry = registry[:idx]
	}

	return registry
}
