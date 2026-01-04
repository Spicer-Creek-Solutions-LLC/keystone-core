package registry

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/module/manifest"
	"github.com/shawnbutts/keystone-core/pkg/module/resolver"
	"github.com/shawnbutts/keystone-core/pkg/module/verify"
)

// HTTPClient implements PublishableRegistry using HTTP
type HTTPClient struct {
	config     *RegistryConfig
	httpClient *http.Client
	hasher     verify.HashVerifier
}

// NewHTTPClient creates a new HTTP registry client
func NewHTTPClient(config *RegistryConfig) (*HTTPClient, error) {
	transport := &http.Transport{}

	// InsecureSkipVerify - blocked by default unless KSCORE_ALLOW_INSECURE_TLS=1 is set
	if config.InsecureSkipVerify {
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			return nil, fmt.Errorf("registry: insecure_skip_verify is not allowed in production (allows MITM attacks). " +
				"Set KSCORE_ALLOW_INSECURE_TLS=1 to override for development/testing only")
		}
		log.Printf("WARNING: Registry HTTP client InsecureSkipVerify is enabled - this allows man-in-the-middle attacks")
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}

	client := &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
	}

	return &HTTPClient{
		config:     config,
		httpClient: client,
		hasher:     verify.NewDefaultHashVerifier(),
	}, nil
}

// SetAuth sets authentication credentials
func (c *HTTPClient) SetAuth(auth *AuthConfig) {
	c.config.Auth = auth
}

// ListVersions returns all available versions for a module
func (c *HTTPClient) ListVersions(moduleName string) ([]string, error) {
	// Go-mod style: /<module>/@v/list
	url := fmt.Sprintf("%s/%s/@v/list", c.config.URL, moduleName)

	resp, err := c.doRequest("GET", url, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &RegistryError{
			StatusCode: 404,
			Code:       ErrCodeModuleNotFound,
			Message:    fmt.Sprintf("module not found: %s", moduleName),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Parse line-separated version list
	lines := strings.Split(strings.TrimSpace(string(body)), "\n")
	var versions []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			versions = append(versions, line)
		}
	}

	return versions, nil
}

// GetModuleInfo returns metadata for a specific version
func (c *HTTPClient) GetModuleInfo(moduleName, version string) (*resolver.ModuleInfo, error) {
	// Go-mod style: /<module>/@v/<version>.info
	url := fmt.Sprintf("%s/%s/@v/%s.info", c.config.URL, moduleName, version)

	resp, err := c.doRequest("GET", url, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &RegistryError{
			StatusCode: 404,
			Code:       ErrCodeVersionNotFound,
			Message:    fmt.Sprintf("version not found: %s@%s", moduleName, version),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var info resolver.ModuleInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &info, nil
}

// GetModuleManifest returns the manifest for a specific version
func (c *HTTPClient) GetModuleManifest(moduleName, version string) (*manifest.Manifest, error) {
	// Go-mod style: /<module>/@v/<version>.mod
	url := fmt.Sprintf("%s/%s/@v/%s.mod", c.config.URL, moduleName, version)

	resp, err := c.doRequest("GET", url, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &RegistryError{
			StatusCode: 404,
			Code:       ErrCodeVersionNotFound,
			Message:    fmt.Sprintf("version not found: %s@%s", moduleName, version),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	m, err := manifest.Parse(body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse manifest: %w", err)
	}

	return m, nil
}

// DownloadModule downloads a module to the specified path
func (c *HTTPClient) DownloadModule(moduleName, version, destPath string) error {
	// Go-mod style: /<module>/@v/<version>.zip
	url := fmt.Sprintf("%s/%s/@v/%s.zip", c.config.URL, moduleName, version)

	resp, err := c.doRequest("GET", url, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &RegistryError{
			StatusCode: 404,
			Code:       ErrCodeVersionNotFound,
			Message:    fmt.Sprintf("version not found: %s@%s", moduleName, version),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	// Create destination directory
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	// Write to file
	f, err := os.Create(destPath)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer f.Close()

	if _, err := io.Copy(f, resp.Body); err != nil {
		return fmt.Errorf("failed to write file: %w", err)
	}

	return nil
}

// PublishModule uploads a module to the registry
func (c *HTTPClient) PublishModule(req *PublishRequest) (*PublishResult, error) {
	// Validate request
	if req.ModulePath == "" {
		return nil, fmt.Errorf("module path is required")
	}
	if req.Manifest == nil {
		return nil, fmt.Errorf("manifest is required")
	}

	// Check if file exists
	info, err := os.Stat(req.ModulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to stat module: %w", err)
	}

	// Compute hash if not provided
	hash := req.Hash
	if hash == "" {
		hash, err = c.hasher.ComputeHash(req.ModulePath)
		if err != nil {
			return nil, fmt.Errorf("failed to compute hash: %w", err)
		}
	}

	// Build multipart request
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add module file
	moduleFile, err := os.Open(req.ModulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open module: %w", err)
	}
	defer moduleFile.Close()

	part, err := writer.CreateFormFile("module", filepath.Base(req.ModulePath))
	if err != nil {
		return nil, fmt.Errorf("failed to create form file: %w", err)
	}
	if _, err := io.Copy(part, moduleFile); err != nil {
		return nil, fmt.Errorf("failed to copy module: %w", err)
	}

	// Add manifest
	manifestBytes, err := req.Manifest.ToYAML()
	if err != nil {
		return nil, fmt.Errorf("failed to serialize manifest: %w", err)
	}
	if err := writer.WriteField("manifest", string(manifestBytes)); err != nil {
		return nil, fmt.Errorf("failed to write manifest: %w", err)
	}

	// Add hash
	if err := writer.WriteField("hash", hash); err != nil {
		return nil, fmt.Errorf("failed to write hash: %w", err)
	}

	// Add signature if present
	if req.SignaturePath != "" {
		sigData, err := os.ReadFile(req.SignaturePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read signature: %w", err)
		}
		if err := writer.WriteField("signature", string(sigData)); err != nil {
			return nil, fmt.Errorf("failed to write signature: %w", err)
		}
	}

	// Add optional metadata
	if req.Force {
		if err := writer.WriteField("force", "true"); err != nil {
			return nil, fmt.Errorf("failed to write force flag: %w", err)
		}
	}
	if req.ReleaseNotes != "" {
		if err := writer.WriteField("release_notes", req.ReleaseNotes); err != nil {
			return nil, fmt.Errorf("failed to write release notes: %w", err)
		}
	}
	if len(req.Tags) > 0 {
		tagsJSON, _ := json.Marshal(req.Tags)
		if err := writer.WriteField("tags", string(tagsJSON)); err != nil {
			return nil, fmt.Errorf("failed to write tags: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("failed to close writer: %w", err)
	}

	// Build URL: POST /<module>/@v/<version>
	url := fmt.Sprintf("%s/%s/@v/%s", c.config.URL, req.Manifest.Name, req.Manifest.Version)

	headers := map[string]string{
		"Content-Type": writer.FormDataContentType(),
	}

	resp, err := c.doRequestWithRetry("POST", url, headers, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	// Handle errors
	if resp.StatusCode == http.StatusConflict {
		return nil, &RegistryError{
			StatusCode: 409,
			Code:       ErrCodeVersionExists,
			Message:    fmt.Sprintf("version already exists: %s@%s", req.Manifest.Name, req.Manifest.Version),
		}
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, c.parseError(resp)
	}

	// Parse response
	var result PublishResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// If response doesn't decode, build minimal result
		result = PublishResult{
			ModuleName:  req.Manifest.Name,
			Version:     req.Manifest.Version,
			Hash:        hash,
			PublishedAt: time.Now(),
			Size:        info.Size(),
		}
	}

	// Ensure required fields are set
	if result.ModuleName == "" {
		result.ModuleName = req.Manifest.Name
	}
	if result.Version == "" {
		result.Version = req.Manifest.Version
	}
	if result.Hash == "" {
		result.Hash = hash
	}
	if result.URL == "" {
		result.URL = fmt.Sprintf("%s/%s/@v/%s.zip", c.config.URL, result.ModuleName, result.Version)
	}

	return &result, nil
}

// DeleteModule removes a module version from the registry
func (c *HTTPClient) DeleteModule(moduleName, version string) error {
	url := fmt.Sprintf("%s/%s/@v/%s", c.config.URL, moduleName, version)

	resp, err := c.doRequest("DELETE", url, nil, nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &RegistryError{
			StatusCode: 404,
			Code:       ErrCodeVersionNotFound,
			Message:    fmt.Sprintf("version not found: %s@%s", moduleName, version),
		}
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return c.parseError(resp)
	}

	return nil
}

// doRequest performs an HTTP request with authentication
func (c *HTTPClient) doRequest(method, url string, headers map[string]string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Add authentication
	if c.config.Auth != nil {
		switch c.config.Auth.Type {
		case AuthTypeBasic:
			req.SetBasicAuth(c.config.Auth.Username, c.config.Auth.Password)
		case AuthTypeBearer:
			req.Header.Set("Authorization", "Bearer "+c.config.Auth.Token)
		case AuthTypeAPIKey:
			header := c.config.Auth.Header
			if header == "" {
				header = "Authorization"
			}
			req.Header.Set(header, c.config.Auth.Token)
		}
	}

	// Add custom headers
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return c.httpClient.Do(req)
}

// doRequestWithRetry performs an HTTP request with retry logic
func (c *HTTPClient) doRequestWithRetry(method, url string, headers map[string]string, body io.Reader) (*http.Response, error) {
	var lastErr error

	// Read body into buffer for retries
	var bodyBytes []byte
	if body != nil {
		var err error
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return nil, fmt.Errorf("failed to read body: %w", err)
		}
	}

	attempts := c.config.RetryAttempts
	if attempts <= 0 {
		attempts = 1
	}

	for i := 0; i < attempts; i++ {
		var bodyReader io.Reader
		if bodyBytes != nil {
			bodyReader = bytes.NewReader(bodyBytes)
		}

		resp, err := c.doRequest(method, url, headers, bodyReader)
		if err != nil {
			lastErr = err
			time.Sleep(c.config.RetryDelay)
			continue
		}

		// Don't retry on client errors (4xx) except rate limiting
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			return resp, nil
		}

		// Retry on server errors (5xx) and rate limiting (429)
		if resp.StatusCode >= 500 || resp.StatusCode == 429 {
			resp.Body.Close()
			lastErr = fmt.Errorf("server returned %d", resp.StatusCode)
			time.Sleep(c.config.RetryDelay * time.Duration(i+1)) // Exponential backoff
			continue
		}

		return resp, nil
	}

	return nil, fmt.Errorf("request failed after %d attempts: %w", attempts, lastErr)
}

// parseError parses an error response from the registry
func (c *HTTPClient) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse as JSON error
	var errResp struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Error   string `json:"error"`
	}
	if err := json.Unmarshal(body, &errResp); err == nil {
		msg := errResp.Message
		if msg == "" {
			msg = errResp.Error
		}
		if msg == "" {
			msg = string(body)
		}
		return &RegistryError{
			StatusCode: resp.StatusCode,
			Code:       errResp.Code,
			Message:    msg,
		}
	}

	// Fall back to plain text
	return &RegistryError{
		StatusCode: resp.StatusCode,
		Message:    strings.TrimSpace(string(body)),
	}
}

// Ensure HTTPClient implements PublishableRegistry
var _ PublishableRegistry = (*HTTPClient)(nil)
