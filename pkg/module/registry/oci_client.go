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
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// OCIClient implements OCIRegistry using the OCI Distribution Specification
type OCIClient struct {
	config     *OCIRegistryConfig
	httpClient *http.Client
}

// NewOCIClient creates a new OCI registry client
func NewOCIClient(config *OCIRegistryConfig) (*OCIClient, error) {
	transport := &http.Transport{}

	// InsecureSkipVerify - blocked by default unless KSCORE_ALLOW_INSECURE_TLS=1 is set
	if config.InsecureSkipVerify {
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			return nil, fmt.Errorf("oci registry: insecure_skip_verify is not allowed in production (allows MITM attacks). " +
				"Set KSCORE_ALLOW_INSECURE_TLS=1 to override for development/testing only")
		}
		log.Printf("WARNING: OCI registry client InsecureSkipVerify is enabled - this allows man-in-the-middle attacks")
		transport.TLSClientConfig = &tls.Config{MinVersion: tls.VersionTLS12, InsecureSkipVerify: true} // #nosec G402 -- gated by KSCORE_ALLOW_INSECURE_TLS // nosemgrep: problem-based-packs.insecure-transport.go-stdlib.bypass-tls-verification.bypass-tls-verification -- InsecureSkipVerify only allowed with KSCORE_ALLOW_INSECURE_TLS=1 for dev/test
		// nosemgrep: problem-based-packs.insecure-transport.go-stdlib.bypass-tls-verification.bypass-tls-verification -- gated by KSCORE_ALLOW_INSECURE_TLS
	}

	client := &http.Client{
		Timeout:   config.Timeout,
		Transport: transport,
	}

	return &OCIClient{
		config:     config,
		httpClient: client,
	}, nil
}

// baseURL returns the base URL for the registry
func (c *OCIClient) baseURL() string {
	scheme := "https"
	if c.config.PlainHTTP {
		scheme = "http"
	}
	return fmt.Sprintf("%s://%s", scheme, c.config.Registry)
}

// repoName returns the full repository name
func (c *OCIClient) repoName(moduleName string) string {
	if c.config.Namespace != "" {
		return fmt.Sprintf("%s/%s", c.config.Namespace, moduleName)
	}
	return moduleName
}

// Ping checks if the registry is accessible
func (c *OCIClient) Ping() error {
	url := fmt.Sprintf("%s/v2/", c.baseURL())
	// Use context.Background() since Ping has no parent context and relies on HTTP client timeout
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to ping registry: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		// Handle WWW-Authenticate challenge if needed
		return &Error{
			StatusCode: resp.StatusCode,
			Code:       ErrCodeUnauthorized,
			Message:    "authentication required",
		}
	}

	if resp.StatusCode != http.StatusOK {
		return &Error{
			StatusCode: resp.StatusCode,
			Code:       ErrCodeServerError,
			Message:    fmt.Sprintf("registry returned status %d", resp.StatusCode),
		}
	}

	return nil
}

// ListTags lists all tags for a module
func (c *OCIClient) ListTags(moduleName string) ([]string, error) {
	repo := c.repoName(moduleName)
	url := fmt.Sprintf("%s/v2/%s/tags/list", c.baseURL(), repo)

	// Use context.Background() since ListTags has no parent context and relies on HTTP client timeout
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, http.NoBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list tags: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, &Error{
			StatusCode: 404,
			Code:       ErrCodeModuleNotFound,
			Message:    fmt.Sprintf("module not found: %s", moduleName),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, c.parseError(resp)
	}

	var tagsList OCITagsList
	if err := json.NewDecoder(resp.Body).Decode(&tagsList); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return tagsList.Tags, nil
}

// Push pushes a module to the registry
func (c *OCIClient) Push(req *OCIPushRequest) (*OCIPushResult, error) {
	// Validate request
	if req.ModulePath == "" {
		return nil, fmt.Errorf("module path is required")
	}
	if req.ManifestPath == "" {
		return nil, fmt.Errorf("manifest path is required")
	}
	if req.ModuleName == "" {
		return nil, fmt.Errorf("module name is required")
	}
	if req.Version == "" {
		return nil, fmt.Errorf("version is required")
	}

	// Read module ZIP
	moduleData, err := os.ReadFile(req.ModulePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read module: %w", err)
	}
	moduleDigest := c.computeDigest(moduleData)

	// Read manifest
	manifestData, err := os.ReadFile(req.ManifestPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read manifest: %w", err)
	}
	manifestDigest := c.computeDigest(manifestData)

	// Read signature if present
	var signatureData []byte
	var signatureDigest string
	if req.SignaturePath != "" {
		signatureData, err = os.ReadFile(req.SignaturePath)
		if err != nil {
			return nil, fmt.Errorf("failed to read signature: %w", err)
		}
		signatureDigest = c.computeDigest(signatureData)
	}

	repo := c.repoName(req.ModuleName)

	// Upload module blob
	if err := c.uploadBlob(repo, moduleDigest, moduleData); err != nil {
		return nil, fmt.Errorf("failed to upload module blob: %w", err)
	}

	// Upload manifest blob (as config)
	if err := c.uploadBlob(repo, manifestDigest, manifestData); err != nil {
		return nil, fmt.Errorf("failed to upload manifest blob: %w", err)
	}

	// Upload signature blob if present
	if signatureData != nil {
		if err := c.uploadBlob(repo, signatureDigest, signatureData); err != nil {
			return nil, fmt.Errorf("failed to upload signature blob: %w", err)
		}
	}

	// Build OCI config
	config := OCIConfig{
		Created:      time.Now(),
		Architecture: "any",
		OS:           "any",
	}
	configData, err := json.Marshal(config)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal config: %w", err)
	}
	configDigest := c.computeDigest(configData)

	// Upload config blob
	if err := c.uploadBlob(repo, configDigest, configData); err != nil {
		return nil, fmt.Errorf("failed to upload config blob: %w", err)
	}

	// Build layers
	layers := []OCIDescriptor{
		{
			MediaType: KscoreModuleMediaType,
			Digest:    moduleDigest,
			Size:      int64(len(moduleData)),
			Annotations: map[string]string{
				"org.opencontainers.image.title": filepath.Base(req.ModulePath),
			},
		},
		{
			MediaType: KscoreManifestMediaType,
			Digest:    manifestDigest,
			Size:      int64(len(manifestData)),
			Annotations: map[string]string{
				"org.opencontainers.image.title": "module.yaml",
			},
		},
	}

	if signatureData != nil {
		layers = append(layers, OCIDescriptor{
			MediaType: KscoreSignatureMediaType,
			Digest:    signatureDigest,
			Size:      int64(len(signatureData)),
			Annotations: map[string]string{
				"org.opencontainers.image.title": "module.sig",
			},
		})
	}

	// Build OCI manifest
	ociManifest := OCIManifest{
		SchemaVersion: 2,
		MediaType:     OCIManifestMediaType,
		ArtifactType:  KscoreModuleMediaType,
		Config: OCIDescriptor{
			MediaType: OCIConfigMediaType,
			Digest:    configDigest,
			Size:      int64(len(configData)),
		},
		Layers: layers,
		Annotations: map[string]string{
			"org.opencontainers.image.created": time.Now().Format(time.RFC3339),
			"org.opencontainers.image.version": req.Version,
			"io.kscore.module.name":            req.ModuleName,
			"io.kscore.module.version":         req.Version,
		},
	}

	// Add custom annotations
	for k, v := range req.Annotations {
		ociManifest.Annotations[k] = v
	}

	// Push manifest
	manifestJSON, err := json.Marshal(ociManifest)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal OCI manifest: %w", err)
	}

	digest, err := c.pushManifest(repo, req.Version, manifestJSON)
	if err != nil {
		return nil, fmt.Errorf("failed to push manifest: %w", err)
	}

	totalSize := int64(len(moduleData) + len(manifestData) + len(signatureData) + len(configData))

	return &OCIPushResult{
		Reference:  fmt.Sprintf("%s/%s:%s", c.config.Registry, repo, req.Version),
		Digest:     digest,
		ModuleName: req.ModuleName,
		Version:    req.Version,
		Size:       totalSize,
		PushedAt:   time.Now(),
	}, nil
}

// Pull pulls a module from the registry
func (c *OCIClient) Pull(moduleName, version, destDir string) (*OCIPullResult, error) {
	repo := c.repoName(moduleName)

	// Get manifest
	manifest, manifestDigest, err := c.getManifestWithDigest(repo, version)
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest: %w", err)
	}

	// Create destination directory
	//nolint:gosec // G301: module directory needs to be accessible by service user
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create directory: %w", err)
	}

	var modulePath, manifestPath, signaturePath string
	var totalSize int64

	// Download layers
	for _, layer := range manifest.Layers {
		var destPath string
		switch layer.MediaType {
		case KscoreModuleMediaType:
			destPath = filepath.Join(destDir, "module.zip")
			modulePath = destPath
		case KscoreManifestMediaType:
			destPath = filepath.Join(destDir, "module.yaml")
			manifestPath = destPath
		case KscoreSignatureMediaType:
			destPath = filepath.Join(destDir, "module.sig")
			signaturePath = destPath
		default:
			// Skip unknown layer types
			continue
		}

		if err := c.downloadBlob(repo, layer.Digest, destPath); err != nil {
			return nil, fmt.Errorf("failed to download blob %s: %w", layer.Digest, err)
		}
		totalSize += layer.Size
	}

	if modulePath == "" {
		return nil, fmt.Errorf("module ZIP not found in manifest")
	}
	if manifestPath == "" {
		return nil, fmt.Errorf("module manifest not found in manifest")
	}

	return &OCIPullResult{
		Reference:     fmt.Sprintf("%s/%s:%s", c.config.Registry, repo, version),
		Digest:        manifestDigest,
		ModulePath:    modulePath,
		ManifestPath:  manifestPath,
		SignaturePath: signaturePath,
		Size:          totalSize,
		PulledAt:      time.Now(),
	}, nil
}

// Delete deletes a module version from the registry
func (c *OCIClient) Delete(moduleName, version string) error {
	repo := c.repoName(moduleName)

	// Get manifest to get digest
	_, digest, err := c.getManifestWithDigest(repo, version)
	if err != nil {
		return fmt.Errorf("failed to get manifest: %w", err)
	}

	// Delete manifest by digest
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", c.baseURL(), repo, digest)
	// Use context.Background() since Delete has no parent context and relies on HTTP client timeout
	req, err := http.NewRequestWithContext(context.Background(), "DELETE", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &Error{
			StatusCode: 404,
			Code:       ErrCodeVersionNotFound,
			Message:    fmt.Sprintf("version not found: %s@%s", moduleName, version),
		}
	}

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

	return nil
}

// GetManifest retrieves the OCI manifest for a module
func (c *OCIClient) GetManifest(moduleName, reference string) (*OCIManifest, error) {
	repo := c.repoName(moduleName)
	manifest, _, err := c.getManifestWithDigest(repo, reference)
	return manifest, err
}

// getManifestWithDigest gets manifest and its digest
func (c *OCIClient) getManifestWithDigest(repo, reference string) (*OCIManifest, string, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", c.baseURL(), repo, reference)

	// Use context.Background() since this is called from methods with no parent context and relies on HTTP client timeout
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, http.NoBody)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Accept", OCIManifestMediaType)
	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("failed to get manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, "", &Error{
			StatusCode: 404,
			Code:       ErrCodeVersionNotFound,
			Message:    fmt.Sprintf("reference not found: %s", reference),
		}
	}

	if resp.StatusCode != http.StatusOK {
		return nil, "", c.parseError(resp)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("failed to read response: %w", err)
	}

	var manifest OCIManifest
	if err := json.Unmarshal(body, &manifest); err != nil {
		return nil, "", fmt.Errorf("failed to decode manifest: %w", err)
	}

	// Get digest from header or compute
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		digest = c.computeDigest(body)
	}

	return &manifest, digest, nil
}

// uploadBlob uploads a blob to the registry
func (c *OCIClient) uploadBlob(repo, digest string, data []byte) error {
	// Check if blob exists
	exists, err := c.blobExists(repo, digest)
	if err != nil {
		return err
	}
	if exists {
		return nil // Already uploaded
	}

	// Start upload
	uploadURL, err := c.startUpload(repo)
	if err != nil {
		return err
	}

	// Upload blob in single request (monolithic upload)
	// Add digest query parameter
	if strings.Contains(uploadURL, "?") {
		uploadURL += "&digest=" + digest
	} else {
		uploadURL += "?digest=" + digest
	}

	req, err := http.NewRequestWithContext(context.Background(), "PUT", uploadURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Content-Length", fmt.Sprintf("%d", len(data)))
	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to upload blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return c.parseError(resp)
	}

	return nil
}

// blobExists checks if a blob exists
func (c *OCIClient) blobExists(repo, digest string) (bool, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", c.baseURL(), repo, digest)

	// Use context.Background() since this is called from methods with no parent context and relies on HTTP client timeout
	req, err := http.NewRequestWithContext(context.Background(), "HEAD", url, http.NoBody)
	if err != nil {
		return false, fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("failed to check blob: %w", err)
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK, nil
}

// startUpload initiates a blob upload and returns the upload URL
func (c *OCIClient) startUpload(repo string) (string, error) {
	url := fmt.Sprintf("%s/v2/%s/blobs/uploads/", c.baseURL(), repo)

	// Use context.Background() since this is called from methods with no parent context and relies on HTTP client timeout
	req, err := http.NewRequestWithContext(context.Background(), "POST", url, http.NoBody)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to start upload: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted {
		return "", c.parseError(resp)
	}

	location := resp.Header.Get("Location")
	if location == "" {
		return "", fmt.Errorf("no upload location in response")
	}

	// Handle relative URLs
	if !strings.HasPrefix(location, "http") {
		location = c.baseURL() + location
	}

	return location, nil
}

// downloadBlob downloads a blob to a file
func (c *OCIClient) downloadBlob(repo, digest, destPath string) error {
	url := fmt.Sprintf("%s/v2/%s/blobs/%s", c.baseURL(), repo, digest)

	// Use context.Background() since this is called from methods with no parent context and relies on HTTP client timeout
	req, err := http.NewRequestWithContext(context.Background(), "GET", url, http.NoBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download blob: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return c.parseError(resp)
	}

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

// pushManifest pushes a manifest and returns its digest
func (c *OCIClient) pushManifest(repo, reference string, manifestJSON []byte) (string, error) {
	url := fmt.Sprintf("%s/v2/%s/manifests/%s", c.baseURL(), repo, reference)

	req, err := http.NewRequestWithContext(context.Background(), "PUT", url, bytes.NewReader(manifestJSON))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", OCIManifestMediaType)
	c.addAuth(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to push manifest: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return "", c.parseError(resp)
	}

	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		digest = c.computeDigest(manifestJSON)
	}

	return digest, nil
}

// computeDigest computes the sha256 digest of data
func (c *OCIClient) computeDigest(data []byte) string {
	hash := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(hash[:])
}

// addAuth adds authentication to a request
func (c *OCIClient) addAuth(req *http.Request) {
	if c.config.Auth == nil {
		return
	}

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
	default:
	}
}

// parseError parses an error response
func (c *OCIClient) parseError(resp *http.Response) error {
	body, _ := io.ReadAll(resp.Body)

	// Try to parse as OCI error
	var ociErr struct {
		Errors []struct {
			Code    string `json:"code"`
			Message string `json:"message"`
			Detail  any    `json:"detail"`
		} `json:"errors"`
	}
	if err := json.Unmarshal(body, &ociErr); err == nil && len(ociErr.Errors) > 0 {
		return &Error{
			StatusCode: resp.StatusCode,
			Code:       ociErr.Errors[0].Code,
			Message:    ociErr.Errors[0].Message,
		}
	}

	return &Error{
		StatusCode: resp.StatusCode,
		Message:    strings.TrimSpace(string(body)),
	}
}

// Ensure OCIClient implements OCIRegistry
var _ OCIRegistry = (*OCIClient)(nil)
