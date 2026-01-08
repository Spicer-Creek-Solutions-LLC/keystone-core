// Package rest provides a REST/HTTP protocol adapter for proxy agents.
package rest

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Authenticator is the interface for REST authentication methods.
type Authenticator interface {
	// Authenticate adds authentication to a request.
	Authenticate(req *http.Request) error

	// Refresh refreshes authentication credentials if needed.
	Refresh(ctx context.Context) error

	// Type returns the authentication type name.
	Type() string
}

// NoAuth implements no authentication.
type NoAuth struct{}

// Authenticate implements Authenticator.
func (a *NoAuth) Authenticate(req *http.Request) error {
	return nil
}

// Refresh implements Authenticator.
func (a *NoAuth) Refresh(ctx context.Context) error {
	return nil
}

// Type implements Authenticator.
func (a *NoAuth) Type() string {
	return "none"
}

// BasicAuth implements HTTP Basic authentication.
type BasicAuth struct {
	username string
	password string
}

// NewBasicAuth creates a new Basic authenticator.
func NewBasicAuth(username, password string) *BasicAuth {
	return &BasicAuth{
		username: username,
		password: password,
	}
}

// Authenticate implements Authenticator.
func (a *BasicAuth) Authenticate(req *http.Request) error {
	credentials := base64.StdEncoding.EncodeToString([]byte(a.username + ":" + a.password))
	req.Header.Set("Authorization", "Basic "+credentials)
	return nil
}

// Refresh implements Authenticator.
func (a *BasicAuth) Refresh(ctx context.Context) error {
	return nil // Basic auth doesn't need refresh
}

// Type implements Authenticator.
func (a *BasicAuth) Type() string {
	return "basic"
}

// BearerAuth implements Bearer token authentication.
type BearerAuth struct {
	token string
	mu    sync.RWMutex
}

// NewBearerAuth creates a new Bearer authenticator.
func NewBearerAuth(token string) *BearerAuth {
	return &BearerAuth{
		token: token,
	}
}

// Authenticate implements Authenticator.
func (a *BearerAuth) Authenticate(req *http.Request) error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	req.Header.Set("Authorization", "Bearer "+a.token)
	return nil
}

// Refresh implements Authenticator.
func (a *BearerAuth) Refresh(ctx context.Context) error {
	return nil // Static token doesn't need refresh
}

// Type implements Authenticator.
func (a *BearerAuth) Type() string {
	return "bearer"
}

// SetToken updates the bearer token.
func (a *BearerAuth) SetToken(token string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.token = token
}

// APIKeyLocation specifies where the API key should be placed.
type APIKeyLocation string

const (
	// APIKeyHeader places the API key in a header.
	APIKeyHeader APIKeyLocation = "header"
	// APIKeyQuery places the API key in a query parameter.
	APIKeyQuery APIKeyLocation = "query"
	// APIKeyCookie places the API key in a cookie.
	APIKeyCookie APIKeyLocation = "cookie"
)

// APIKeyAuth implements API key authentication.
type APIKeyAuth struct {
	key      string
	value    string
	location APIKeyLocation
}

// NewAPIKeyAuth creates a new API key authenticator.
func NewAPIKeyAuth(key, value string, location APIKeyLocation) *APIKeyAuth {
	if location == "" {
		location = APIKeyHeader
	}
	return &APIKeyAuth{
		key:      key,
		value:    value,
		location: location,
	}
}

// Authenticate implements Authenticator.
func (a *APIKeyAuth) Authenticate(req *http.Request) error {
	switch a.location {
	case APIKeyHeader:
		req.Header.Set(a.key, a.value)
	case APIKeyQuery:
		q := req.URL.Query()
		q.Set(a.key, a.value)
		req.URL.RawQuery = q.Encode()
	case APIKeyCookie:
		req.AddCookie(&http.Cookie{Name: a.key, Value: a.value})
	default:
		req.Header.Set(a.key, a.value)
	}
	return nil
}

// Refresh implements Authenticator.
func (a *APIKeyAuth) Refresh(ctx context.Context) error {
	return nil // API key doesn't need refresh
}

// Type implements Authenticator.
func (a *APIKeyAuth) Type() string {
	return "api_key"
}

// OAuth2ClientCredentials implements OAuth2 client credentials flow.
type OAuth2ClientCredentials struct {
	ClientID     string
	ClientSecret string
	TokenURL     string
	Scopes       []string

	// Token state
	accessToken  string
	tokenType    string
	expiresAt    time.Time
	refreshToken string
	mu           sync.RWMutex

	// HTTP client for token requests
	httpClient *http.Client
}

// NewOAuth2ClientCredentials creates a new OAuth2 client credentials authenticator.
func NewOAuth2ClientCredentials(clientID, clientSecret, tokenURL string) *OAuth2ClientCredentials {
	return &OAuth2ClientCredentials{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Authenticate implements Authenticator.
func (a *OAuth2ClientCredentials) Authenticate(req *http.Request) error {
	// Check if we need to refresh the token
	a.mu.RLock()
	needsRefresh := a.accessToken == "" || time.Now().After(a.expiresAt.Add(-time.Minute))
	a.mu.RUnlock()

	if needsRefresh {
		if err := a.Refresh(req.Context()); err != nil {
			return err
		}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	tokenType := a.tokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	req.Header.Set("Authorization", tokenType+" "+a.accessToken)
	return nil
}

// Refresh implements Authenticator.
func (a *OAuth2ClientCredentials) Refresh(ctx context.Context) error {
	// Build token request
	data := url.Values{}
	data.Set("grant_type", "client_credentials")
	if len(a.Scopes) > 0 {
		data.Set("scope", strings.Join(a.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(a.ClientID, a.ClientSecret)

	// Execute request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp oauth2TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	// Update token state
	a.mu.Lock()
	defer a.mu.Unlock()

	a.accessToken = tokenResp.AccessToken
	a.tokenType = tokenResp.TokenType
	a.refreshToken = tokenResp.RefreshToken

	if tokenResp.ExpiresIn > 0 {
		a.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		// Default to 1 hour if not specified
		a.expiresAt = time.Now().Add(time.Hour)
	}

	return nil
}

// Type implements Authenticator.
func (a *OAuth2ClientCredentials) Type() string {
	return "oauth2_client_credentials"
}

// GetAccessToken returns the current access token.
func (a *OAuth2ClientCredentials) GetAccessToken() string {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.accessToken
}

// oauth2TokenResponse represents an OAuth2 token response.
type oauth2TokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token,omitempty"`
	Scope        string `json:"scope,omitempty"`
}

// OAuth2ResourceOwner implements OAuth2 resource owner password flow.
type OAuth2ResourceOwner struct {
	ClientID     string
	ClientSecret string
	TokenURL     string
	Username     string
	Password     string
	Scopes       []string

	// Token state
	accessToken  string
	tokenType    string
	expiresAt    time.Time
	refreshToken string
	mu           sync.RWMutex

	// HTTP client for token requests
	httpClient *http.Client
}

// NewOAuth2ResourceOwner creates a new OAuth2 resource owner authenticator.
func NewOAuth2ResourceOwner(clientID, clientSecret, tokenURL, username, password string) *OAuth2ResourceOwner {
	return &OAuth2ResourceOwner{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		TokenURL:     tokenURL,
		Username:     username,
		Password:     password,
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

// Authenticate implements Authenticator.
func (a *OAuth2ResourceOwner) Authenticate(req *http.Request) error {
	// Check if we need to refresh the token
	a.mu.RLock()
	needsRefresh := a.accessToken == "" || time.Now().After(a.expiresAt.Add(-time.Minute))
	a.mu.RUnlock()

	if needsRefresh {
		if err := a.Refresh(req.Context()); err != nil {
			return err
		}
	}

	a.mu.RLock()
	defer a.mu.RUnlock()

	tokenType := a.tokenType
	if tokenType == "" {
		tokenType = "Bearer"
	}
	req.Header.Set("Authorization", tokenType+" "+a.accessToken)
	return nil
}

// Refresh implements Authenticator.
func (a *OAuth2ResourceOwner) Refresh(ctx context.Context) error {
	// Build token request
	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("username", a.Username)
	data.Set("password", a.Password)
	if len(a.Scopes) > 0 {
		data.Set("scope", strings.Join(a.Scopes, " "))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.TokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if a.ClientID != "" {
		req.SetBasicAuth(a.ClientID, a.ClientSecret)
	}

	// Execute request
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var tokenResp oauth2TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return fmt.Errorf("failed to parse token response: %w", err)
	}

	// Update token state
	a.mu.Lock()
	defer a.mu.Unlock()

	a.accessToken = tokenResp.AccessToken
	a.tokenType = tokenResp.TokenType
	a.refreshToken = tokenResp.RefreshToken

	if tokenResp.ExpiresIn > 0 {
		a.expiresAt = time.Now().Add(time.Duration(tokenResp.ExpiresIn) * time.Second)
	} else {
		a.expiresAt = time.Now().Add(time.Hour)
	}

	return nil
}

// Type implements Authenticator.
func (a *OAuth2ResourceOwner) Type() string {
	return "oauth2_password"
}

// DigestAuth implements HTTP Digest authentication.
type DigestAuth struct {
	username string
	password string
	realm    string
	nonce    string
	qop      string
	nc       int
	mu       sync.RWMutex
}

// NewDigestAuth creates a new Digest authenticator.
func NewDigestAuth(username, password string) *DigestAuth {
	return &DigestAuth{
		username: username,
		password: password,
	}
}

// Authenticate implements Authenticator.
// Note: Digest auth is complex and typically handled by the HTTP client.
// This is a simplified implementation.
func (a *DigestAuth) Authenticate(req *http.Request) error {
	// Basic digest implementation - in practice, this would need to handle
	// the 401 challenge/response cycle
	a.mu.RLock()
	defer a.mu.RUnlock()

	// If we don't have nonce yet, just return (will get 401 challenge)
	if a.nonce == "" {
		return nil
	}

	// Build digest header
	// This is a simplified version - a full implementation would compute
	// proper response hash
	return nil
}

// Refresh implements Authenticator.
func (a *DigestAuth) Refresh(ctx context.Context) error {
	return nil
}

// Type implements Authenticator.
func (a *DigestAuth) Type() string {
	return "digest"
}

// SetChallenge sets the digest challenge parameters from a 401 response.
func (a *DigestAuth) SetChallenge(realm, nonce, qop string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.realm = realm
	a.nonce = nonce
	a.qop = qop
	a.nc = 0
}

// CustomHeaderAuth implements custom header authentication.
type CustomHeaderAuth struct {
	headers map[string]string
	mu      sync.RWMutex
}

// NewCustomHeaderAuth creates a new custom header authenticator.
func NewCustomHeaderAuth(headers map[string]string) *CustomHeaderAuth {
	return &CustomHeaderAuth{
		headers: headers,
	}
}

// Authenticate implements Authenticator.
func (a *CustomHeaderAuth) Authenticate(req *http.Request) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for k, v := range a.headers {
		req.Header.Set(k, v)
	}
	return nil
}

// Refresh implements Authenticator.
func (a *CustomHeaderAuth) Refresh(ctx context.Context) error {
	return nil
}

// Type implements Authenticator.
func (a *CustomHeaderAuth) Type() string {
	return "custom_header"
}

// SetHeader sets a custom header.
func (a *CustomHeaderAuth) SetHeader(key, value string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.headers[key] = value
}

// RemoveHeader removes a custom header.
func (a *CustomHeaderAuth) RemoveHeader(key string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	delete(a.headers, key)
}

// ChainAuth chains multiple authenticators together.
type ChainAuth struct {
	authenticators []Authenticator
}

// NewChainAuth creates a new chain authenticator.
func NewChainAuth(authenticators ...Authenticator) *ChainAuth {
	return &ChainAuth{
		authenticators: authenticators,
	}
}

// Authenticate implements Authenticator.
func (a *ChainAuth) Authenticate(req *http.Request) error {
	for _, auth := range a.authenticators {
		if err := auth.Authenticate(req); err != nil {
			return err
		}
	}
	return nil
}

// Refresh implements Authenticator.
func (a *ChainAuth) Refresh(ctx context.Context) error {
	for _, auth := range a.authenticators {
		if err := auth.Refresh(ctx); err != nil {
			return err
		}
	}
	return nil
}

// Type implements Authenticator.
func (a *ChainAuth) Type() string {
	return "chain"
}

// Add adds an authenticator to the chain.
func (a *ChainAuth) Add(auth Authenticator) {
	a.authenticators = append(a.authenticators, auth)
}
