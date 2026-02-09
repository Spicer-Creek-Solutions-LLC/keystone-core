package registry

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/internal/blueprint"
	"github.com/shawnbutts/keystone-core/pkg/wait"
)

// HTTPClient implements Client using HTTP with Go-mod style endpoints.
type HTTPClient struct {
	config     *Config
	httpClient *http.Client
	auth       *AuthConfig
}

// NewHTTPClient creates a new HTTP registry client.
func NewHTTPClient(config *Config) (*HTTPClient, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if config.URL == "" {
		return nil, fmt.Errorf("registry URL is required")
	}

	// Normalize URL
	config.URL = strings.TrimSuffix(config.URL, "/")

	// Apply defaults
	if config.Timeout == 0 {
		config.Timeout = 30 * time.Second
	}
	if config.RetryAttempts == 0 {
		config.RetryAttempts = 3
	}
	if config.RetryDelay == 0 {
		config.RetryDelay = 1 * time.Second
	}
	if config.Namespace == "" {
		config.Namespace = "blueprints"
	}

	// Create HTTP client
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: config.InsecureSkipVerify, // #nosec G402 -- user-configured for development/testing
		},
	}

	client := &HTTPClient{
		config: config,
		httpClient: &http.Client{
			Timeout:   config.Timeout,
			Transport: transport,
		},
		auth: config.Auth,
	}

	return client, nil
}

// SetAuth sets the authentication configuration.
func (c *HTTPClient) SetAuth(auth *AuthConfig) {
	c.auth = auth
}

// ListVersions returns all available versions for a blueprint.
// Endpoint: GET /{namespace}/{name}/@v/list
func (c *HTTPClient) ListVersions(name string) ([]string, error) {
	endpoint := fmt.Sprintf("/%s/%s/@v/list", c.config.Namespace, name)

	resp, err := c.doRequest("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrBlueprintNotFound
	}

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Response is one version per line
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	var versions []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			versions = append(versions, line)
		}
	}

	// Sort versions (newest first)
	sort.Slice(versions, func(i, j int) bool {
		return versions[i] > versions[j]
	})

	return versions, nil
}

// GetBlueprintInfo returns information about a specific blueprint version.
// Endpoint: GET /{namespace}/{name}/@v/{version}.info
func (c *HTTPClient) GetBlueprintInfo(name, version string) (*BlueprintInfo, error) {
	endpoint := fmt.Sprintf("/%s/%s/@v/%s.info", c.config.Namespace, name, version)

	resp, err := c.doRequest("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrVersionNotFound
	}

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	var info BlueprintInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &info, nil
}

// GetLatestVersion returns the latest version of a blueprint.
// Endpoint: GET /{namespace}/{name}/@latest
func (c *HTTPClient) GetLatestVersion(name string) (string, error) {
	endpoint := fmt.Sprintf("/%s/%s/@latest", c.config.Namespace, name)

	resp, err := c.doRequest("GET", endpoint, nil, nil)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", ErrBlueprintNotFound
	}

	if err := c.checkResponse(resp); err != nil {
		return "", err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	return strings.TrimSpace(string(body)), nil
}

// DownloadBlueprint downloads a blueprint archive.
// Endpoint: GET /{namespace}/{name}/@v/{version}.zip
func (c *HTTPClient) DownloadBlueprint(name, version string) ([]byte, error) {
	endpoint := fmt.Sprintf("/%s/%s/@v/%s.zip", c.config.Namespace, name, version)

	resp, err := c.doRequest("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrVersionNotFound
	}

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	return data, nil
}

// GetManifest returns the blueprint manifest.
// Endpoint: GET /{namespace}/{name}/@v/{version}.mod
func (c *HTTPClient) GetManifest(name, version string) (*blueprint.Blueprint, error) {
	endpoint := fmt.Sprintf("/%s/%s/@v/%s.mod", c.config.Namespace, name, version)

	resp, err := c.doRequest("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrVersionNotFound
	}

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	bp, err := blueprint.ParseManifest(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return bp, nil
}

// PublishBlueprint publishes a new blueprint version.
// Endpoint: POST /{namespace}/{name}/@v/{version}
func (c *HTTPClient) PublishBlueprint(req *PublishRequest) (*PublishResult, error) {
	if req.Name == "" || req.Version == "" {
		return nil, fmt.Errorf("name and version are required")
	}

	if len(req.Archive) == 0 {
		return nil, fmt.Errorf("archive is required")
	}

	// Verify checksum if provided
	if req.Checksum != "" {
		hash := sha256.Sum256(req.Archive)
		computed := hex.EncodeToString(hash[:])
		if !strings.EqualFold(computed, req.Checksum) {
			return nil, ErrChecksumMismatch
		}
	}

	endpoint := fmt.Sprintf("/%s/%s/@v/%s", c.config.Namespace, req.Name, req.Version)

	// Build multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add archive
	archivePart, err := writer.CreateFormFile("archive", "blueprint.zip")
	if err != nil {
		return nil, fmt.Errorf("failed to create archive part: %w", err)
	}
	if _, err := archivePart.Write(req.Archive); err != nil {
		return nil, fmt.Errorf("failed to write archive: %w", err)
	}

	// Add manifest
	if len(req.Manifest) > 0 {
		manifestPart, err := writer.CreateFormFile("manifest", "blueprint.yaml")
		if err != nil {
			return nil, fmt.Errorf("failed to create manifest part: %w", err)
		}
		if _, err := manifestPart.Write(req.Manifest); err != nil {
			return nil, fmt.Errorf("failed to write manifest: %w", err)
		}
	}

	// Add checksum
	if req.Checksum != "" {
		if err := writer.WriteField("checksum", req.Checksum); err != nil {
			return nil, fmt.Errorf("failed to write checksum: %w", err)
		}
	}

	// Add signature
	if len(req.Signature) > 0 {
		sigPart, err := writer.CreateFormFile("signature", "blueprint.sig")
		if err != nil {
			return nil, fmt.Errorf("failed to create signature part: %w", err)
		}
		if _, err := sigPart.Write(req.Signature); err != nil {
			return nil, fmt.Errorf("failed to write signature: %w", err)
		}
	}

	// Add certificate
	if len(req.Certificate) > 0 {
		certPart, err := writer.CreateFormFile("certificate", "blueprint.crt")
		if err != nil {
			return nil, fmt.Errorf("failed to create certificate part: %w", err)
		}
		if _, err := certPart.Write(req.Certificate); err != nil {
			return nil, fmt.Errorf("failed to write certificate: %w", err)
		}
	}

	// Add force flag
	if req.Force {
		if err := writer.WriteField("force", "true"); err != nil {
			return nil, fmt.Errorf("failed to write force flag: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close multipart writer: %w", err)
	}

	headers := map[string]string{
		"Content-Type": writer.FormDataContentType(),
	}

	resp, err := c.doRequest("POST", endpoint, &buf, headers)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusConflict && !req.Force {
		return nil, ErrVersionExists
	}

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	var result PublishResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// DeleteBlueprint deletes a blueprint version.
// Endpoint: DELETE /{namespace}/{name}/@v/{version}
func (c *HTTPClient) DeleteBlueprint(name, version string) error {
	endpoint := fmt.Sprintf("/%s/%s/@v/%s", c.config.Namespace, name, version)

	resp, err := c.doRequest("DELETE", endpoint, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrVersionNotFound
	}

	return c.checkResponse(resp)
}

// DeprecateVersion marks a version as deprecated.
// Endpoint: POST /{namespace}/{name}/@v/{version}/deprecate
func (c *HTTPClient) DeprecateVersion(name, version, message string) error {
	endpoint := fmt.Sprintf("/%s/%s/@v/%s/deprecate", c.config.Namespace, name, version)

	body := map[string]string{"message": message}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := c.doRequest("POST", endpoint, bytes.NewReader(data), headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrVersionNotFound
	}

	return c.checkResponse(resp)
}

// RetractVersion retracts a version.
// Endpoint: POST /{namespace}/{name}/@v/{version}/retract
func (c *HTTPClient) RetractVersion(name, version, reason string) error {
	endpoint := fmt.Sprintf("/%s/%s/@v/%s/retract", c.config.Namespace, name, version)

	body := map[string]string{"reason": reason}
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := c.doRequest("POST", endpoint, bytes.NewReader(data), headers)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return ErrVersionNotFound
	}

	return c.checkResponse(resp)
}

// Search searches for blueprints matching the query.
// Endpoint: GET /{namespace}/@search?q=...
func (c *HTTPClient) Search(query *SearchQuery) (*SearchResult, error) {
	endpoint := fmt.Sprintf("/%s/@search", c.config.Namespace)

	// Build query parameters
	params := url.Values{}
	if query.Query != "" {
		params.Set("q", query.Query)
	}
	if query.Vendor != "" {
		params.Set("vendor", query.Vendor)
	}
	if len(query.Tags) > 0 {
		params.Set("tags", strings.Join(query.Tags, ","))
	}
	if query.MinKSCoreVersion != "" {
		params.Set("min_kscore_version", query.MinKSCoreVersion)
	}
	if query.Platform != "" {
		params.Set("platform", query.Platform)
	}
	if query.Limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", query.Limit))
	}
	if query.Offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", query.Offset))
	}
	if query.SortBy != "" {
		params.Set("sort_by", query.SortBy)
	}
	if query.SortOrder != "" {
		params.Set("sort_order", query.SortOrder)
	}

	if len(params) > 0 {
		endpoint += "?" + params.Encode()
	}

	resp, err := c.doRequest("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	var result SearchResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &result, nil
}

// GetIndex returns the full registry index.
// Endpoint: GET /{namespace}/@index
func (c *HTTPClient) GetIndex() ([]*IndexEntry, error) {
	endpoint := fmt.Sprintf("/%s/@index", c.config.Namespace)

	resp, err := c.doRequest("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	var entries []*IndexEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return entries, nil
}

// GetVendors returns all vendors in the registry.
// Endpoint: GET /{namespace}/@vendors
func (c *HTTPClient) GetVendors() ([]string, error) {
	endpoint := fmt.Sprintf("/%s/@vendors", c.config.Namespace)

	resp, err := c.doRequest("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	var vendors []string
	if err := json.NewDecoder(resp.Body).Decode(&vendors); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return vendors, nil
}

// GetTags returns all tags used in the registry.
// Endpoint: GET /{namespace}/@tags
func (c *HTTPClient) GetTags() ([]string, error) {
	endpoint := fmt.Sprintf("/%s/@tags", c.config.Namespace)

	resp, err := c.doRequest("GET", endpoint, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if err := c.checkResponse(resp); err != nil {
		return nil, err
	}

	var tags []string
	if err := json.NewDecoder(resp.Body).Decode(&tags); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return tags, nil
}

// doRequest performs an HTTP request with retries.
func (c *HTTPClient) doRequest(method, endpoint string, body io.Reader, headers map[string]string) (*http.Response, error) {
	fullURL := c.config.URL + endpoint
	ctx := context.Background()
	if c.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.config.Timeout)
		defer cancel()
	}

	var lastErr error
	for attempt := 0; attempt < c.config.RetryAttempts; attempt++ {
		if attempt > 0 {
			if err := waitForRetry(ctx, c.config.RetryDelay*time.Duration(attempt)); err != nil {
				return nil, err
			}
		}

		// Create request
		var bodyReader io.Reader
		if body != nil {
			// Read body into buffer for retry
			switch b := body.(type) {
			case *bytes.Buffer:
				bodyReader = bytes.NewReader(b.Bytes())
			case *bytes.Reader:
				b.Seek(0, io.SeekStart)
				bodyReader = b
			default:
				bodyReader = body
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, fullURL, bodyReader)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		// Set headers
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		// Set auth
		c.setAuth(req)

		// Execute request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("request failed: %w", err)
			continue
		}

		// Don't retry on client errors (4xx)
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			return resp, nil
		}

		// Retry on server errors (5xx)
		if resp.StatusCode >= 500 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server error: %d", resp.StatusCode)
			continue
		}

		return resp, nil
	}

	if lastErr != nil {
		return nil, lastErr
	}

	return nil, ErrRegistryUnavailable
}

func waitForRetry(ctx context.Context, delay time.Duration) error {
	return wait.ForContext(ctx, delay)
}

// setAuth sets authentication headers on the request.
func (c *HTTPClient) setAuth(req *http.Request) {
	if c.auth == nil {
		return
	}

	switch c.auth.Type {
	case AuthTypeBasic:
		req.SetBasicAuth(c.auth.Username, c.auth.Password)
	case AuthTypeBearer:
		req.Header.Set("Authorization", "Bearer "+c.auth.Token)
	case AuthTypeAPIKey:
		header := c.auth.Header
		if header == "" {
			header = "X-API-Key"
		}
		req.Header.Set(header, c.auth.Token)
	default:
	}
}

// checkResponse checks for error responses.
func (c *HTTPClient) checkResponse(resp *http.Response) error {
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}

	// Try to parse error response
	var regErr Error
	if err := json.NewDecoder(resp.Body).Decode(&regErr); err == nil {
		regErr.StatusCode = resp.StatusCode
		return &regErr
	}

	// Generic error based on status code
	switch resp.StatusCode {
	case http.StatusUnauthorized:
		return ErrUnauthorized
	case http.StatusForbidden:
		return ErrForbidden
	case http.StatusNotFound:
		return ErrBlueprintNotFound
	case http.StatusConflict:
		return ErrVersionExists
	default:
		return fmt.Errorf("request failed with status %d", resp.StatusCode)
	}
}

// BlueprintPath returns the path for a blueprint in the registry.
func BlueprintPath(namespace, name, version string) string {
	return path.Join(namespace, name, "@v", version)
}

// ParseBlueprintName parses a full blueprint name into vendor and name.
func ParseBlueprintName(fullName string) (vendor, name string, err error) {
	parts := strings.SplitN(fullName, "/", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid blueprint name: %s (expected vendor/name)", fullName)
	}
	return parts[0], parts[1], nil
}
