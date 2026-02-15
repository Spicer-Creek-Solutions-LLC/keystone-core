package backend

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"time"
)

// HTTPConfig configures the HTTP backend.
type HTTPConfig struct {
	Config `yaml:",inline"`

	// BaseURL is the HTTP(S) server base URL.
	BaseURL string `yaml:"base_url"`

	// Timeout for HTTP requests (default: 30s).
	Timeout time.Duration `yaml:"timeout,omitempty"`

	// Headers are extra HTTP headers sent with every request.
	Headers map[string]string `yaml:"headers,omitempty"`

	// TLSInsecureSkipVerify skips TLS certificate verification.
	TLSInsecureSkipVerify bool `yaml:"tls_insecure_skip_verify,omitempty"`

	// Username for HTTP Basic authentication.
	Username string `yaml:"username,omitempty"`

	// Password for HTTP Basic authentication.
	Password string `yaml:"password,omitempty"`

	// BearerToken for Authorization: Bearer header.
	BearerToken string `yaml:"bearer_token,omitempty"`
}

// HTTPBackend implements the Backend interface for HTTP(S) file servers.
// This is a read-only backend that fetches files via HTTP GET/HEAD requests.
type HTTPBackend struct {
	config *HTTPConfig
	client *http.Client
}

// NewHTTPBackend creates a new HTTP backend.
func NewHTTPBackend(config *HTTPConfig) (*HTTPBackend, error) {
	if config.BaseURL == "" {
		return nil, fmt.Errorf("http backend: base_url is required")
	}

	parsed, err := url.Parse(config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("http backend: invalid base_url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("http backend: base_url must use http or https scheme")
	}

	// Normalize: strip trailing slash
	config.BaseURL = strings.TrimRight(config.BaseURL, "/")

	timeout := config.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	if config.TLSInsecureSkipVerify {
		if os.Getenv("KSCORE_ALLOW_INSECURE_TLS") != "1" {
			return nil, fmt.Errorf("http backend: tls_insecure_skip_verify is not allowed in production (allows MITM attacks). " +
				"Set KSCORE_ALLOW_INSECURE_TLS=1 to override for development/testing only")
		}
		transport.TLSClientConfig = &tls.Config{
			MinVersion:         tls.VersionTLS12,
			InsecureSkipVerify: true, //nolint:gosec // gated by KSCORE_ALLOW_INSECURE_TLS
		}
	}

	config.ReadOnly = true

	return &HTTPBackend{
		config: config,
		client: &http.Client{
			Timeout:   timeout,
			Transport: transport,
		},
	}, nil
}

// Name returns the backend name.
func (b *HTTPBackend) Name() string {
	return b.config.Name
}

// Type returns the backend type.
func (b *HTTPBackend) Type() Type {
	return TypeHTTP
}

// BaseConfig returns the base configuration.
func (b *HTTPBackend) BaseConfig() *Config {
	return &b.config.Config
}

// Get retrieves a file via HTTP GET.
func (b *HTTPBackend) Get(ctx context.Context, filePath string, opts *GetOptions) (*GetResult, error) {
	reqURL := b.fileURL(filePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, http.NoBody)
	if err != nil {
		return nil, &Error{Backend: b.config.Name, Op: "get", Path: filePath, Err: err}
	}
	b.applyAuth(req)
	b.applyHeaders(req)

	if opts != nil && opts.IfNoneMatch != "" {
		req.Header.Set("If-None-Match", opts.IfNoneMatch)
	}
	if opts != nil && opts.Range != nil {
		end := ""
		if opts.Range.End > 0 {
			end = strconv.FormatInt(opts.Range.End-1, 10)
		}
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%s", opts.Range.Start, end))
	}

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, &Error{Backend: b.config.Name, Op: "get", Path: filePath, Err: err}
	}

	switch resp.StatusCode {
	case http.StatusOK, http.StatusPartialContent:
		// success
	case http.StatusNotModified:
		resp.Body.Close()
		return &GetResult{NotModified: true}, nil
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, &Error{Backend: b.config.Name, Op: "get", Path: filePath, Err: notFoundError{}}
	case http.StatusForbidden, http.StatusUnauthorized:
		resp.Body.Close()
		return nil, &Error{Backend: b.config.Name, Op: "get", Path: filePath, Err: accessDeniedError{}}
	default:
		resp.Body.Close()
		return nil, &Error{Backend: b.config.Name, Op: "get", Path: filePath, Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}

	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	etag := resp.Header.Get("ETag")
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = detectContentType(filePath)
	}

	var modTime time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		modTime, _ = http.ParseTime(lm)
	}

	return &GetResult{
		Reader: resp.Body,
		Info: &FileInfo{
			Path:         filePath,
			Name:         path.Base(filePath),
			Size:         size,
			ContentType:  contentType,
			ModifiedTime: modTime,
			ETag:         etag,
		},
	}, nil
}

// Put is not supported (read-only backend).
func (b *HTTPBackend) Put(_ context.Context, filePath string, _ io.Reader, _ *PutOptions) (*PutResult, error) {
	return nil, &Error{Backend: b.config.Name, Op: "put", Path: filePath, Err: readOnlyError{}}
}

// Delete is not supported (read-only backend).
func (b *HTTPBackend) Delete(_ context.Context, filePath string) error {
	return &Error{Backend: b.config.Name, Op: "delete", Path: filePath, Err: readOnlyError{}}
}

// Exists checks if a file exists via HTTP HEAD.
func (b *HTTPBackend) Exists(ctx context.Context, filePath string) (bool, error) {
	reqURL := b.fileURL(filePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, reqURL, http.NoBody)
	if err != nil {
		return false, &Error{Backend: b.config.Name, Op: "exists", Path: filePath, Err: err}
	}
	b.applyAuth(req)
	b.applyHeaders(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return false, &Error{Backend: b.config.Name, Op: "exists", Path: filePath, Err: err}
	}
	resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	case http.StatusForbidden, http.StatusUnauthorized:
		return false, &Error{Backend: b.config.Name, Op: "exists", Path: filePath, Err: accessDeniedError{}}
	default:
		return false, &Error{Backend: b.config.Name, Op: "exists", Path: filePath, Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}
}

// Stat returns file metadata via HTTP HEAD.
func (b *HTTPBackend) Stat(ctx context.Context, filePath string) (*FileInfo, error) {
	reqURL := b.fileURL(filePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodHead, reqURL, http.NoBody)
	if err != nil {
		return nil, &Error{Backend: b.config.Name, Op: "stat", Path: filePath, Err: err}
	}
	b.applyAuth(req)
	b.applyHeaders(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return nil, &Error{Backend: b.config.Name, Op: "stat", Path: filePath, Err: err}
	}
	resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// success
	case http.StatusNotFound:
		return nil, &Error{Backend: b.config.Name, Op: "stat", Path: filePath, Err: notFoundError{}}
	case http.StatusForbidden, http.StatusUnauthorized:
		return nil, &Error{Backend: b.config.Name, Op: "stat", Path: filePath, Err: accessDeniedError{}}
	default:
		return nil, &Error{Backend: b.config.Name, Op: "stat", Path: filePath, Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}

	size, _ := strconv.ParseInt(resp.Header.Get("Content-Length"), 10, 64)
	etag := resp.Header.Get("ETag")
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = detectContentType(filePath)
	}

	var modTime time.Time
	if lm := resp.Header.Get("Last-Modified"); lm != "" {
		modTime, _ = http.ParseTime(lm)
	}

	return &FileInfo{
		Path:         filePath,
		Name:         path.Base(filePath),
		Size:         size,
		ContentType:  contentType,
		ModifiedTime: modTime,
		ETag:         etag,
	}, nil
}

// List is not supported for HTTP backends.
// HTTP servers don't provide a standard directory listing API.
func (b *HTTPBackend) List(_ context.Context, filePath string, _ *ListOptions) (*ListResult, error) {
	return nil, &Error{Backend: b.config.Name, Op: "list", Path: filePath, Err: fmt.Errorf("list not supported by HTTP backend")}
}

// Health checks if the base URL is reachable via HTTP HEAD.
func (b *HTTPBackend) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, b.config.BaseURL, http.NoBody)
	if err != nil {
		return &Error{Backend: b.config.Name, Op: "health", Err: err}
	}
	b.applyAuth(req)
	b.applyHeaders(req)

	resp, err := b.client.Do(req)
	if err != nil {
		return &Error{Backend: b.config.Name, Op: "health", Err: err}
	}
	resp.Body.Close()

	if resp.StatusCode >= 500 {
		return &Error{Backend: b.config.Name, Op: "health", Err: fmt.Errorf("HTTP %d", resp.StatusCode)}
	}

	return nil
}

// Close releases resources.
func (b *HTTPBackend) Close() error {
	b.client.CloseIdleConnections()
	return nil
}

// fileURL joins the base URL with the file path.
func (b *HTTPBackend) fileURL(filePath string) string {
	// Trim leading slash since base URL has no trailing slash
	trimmed := strings.TrimPrefix(filePath, "/")
	return b.config.BaseURL + "/" + trimmed
}

func (b *HTTPBackend) applyAuth(req *http.Request) {
	if b.config.BearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+b.config.BearerToken)
	} else if b.config.Username != "" {
		req.SetBasicAuth(b.config.Username, b.config.Password)
	}
}

func (b *HTTPBackend) applyHeaders(req *http.Request) {
	for k, v := range b.config.Headers {
		req.Header.Set(k, v)
	}
}

// GetWithChecksum retrieves a file and computes its SHA-256 checksum.
// This is a convenience method that reads the full body to compute the hash.
func (b *HTTPBackend) GetWithChecksum(ctx context.Context, filePath string) (*GetResult, error) {
	result, err := b.Get(ctx, filePath, nil)
	if err != nil {
		return nil, err
	}
	if result.NotModified {
		return result, nil
	}

	body, err := io.ReadAll(result.Reader)
	result.Reader.Close()
	if err != nil {
		return nil, &Error{Backend: b.config.Name, Op: "get", Path: filePath, Err: err}
	}

	hash := sha256.Sum256(body)
	result.Info.Checksum = hex.EncodeToString(hash[:])
	result.Info.Size = int64(len(body))
	result.Reader = io.NopCloser(strings.NewReader(string(body)))

	return result, nil
}
