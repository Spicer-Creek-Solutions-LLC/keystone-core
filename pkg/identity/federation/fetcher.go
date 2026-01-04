// Package federation provides trust federation between identity providers.
package federation

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/identity"
)

// HTTPBundleFetcher fetches trust bundles over HTTP/HTTPS.
type HTTPBundleFetcher struct {
	client *http.Client

	// TLSConfig is the TLS configuration for HTTPS requests.
	TLSConfig *tls.Config

	// Timeout is the request timeout.
	Timeout time.Duration

	// MaxBundleSize is the maximum bundle size to download.
	MaxBundleSize int64

	// UserAgent is the User-Agent header for requests.
	UserAgent string
}

// BundleFetcherConfig configures the bundle fetcher.
type BundleFetcherConfig struct {
	// Timeout is the request timeout.
	// Default: 30 seconds
	Timeout time.Duration

	// MaxBundleSize is the maximum bundle size to download.
	// Default: 10MB
	MaxBundleSize int64

	// TLSConfig is the TLS configuration for HTTPS requests.
	TLSConfig *tls.Config

	// UserAgent is the User-Agent header for requests.
	UserAgent string
}

// DefaultBundleFetcherConfig returns default configuration.
func DefaultBundleFetcherConfig() *BundleFetcherConfig {
	return &BundleFetcherConfig{
		Timeout:       30 * time.Second,
		MaxBundleSize: 10 * 1024 * 1024, // 10MB
		UserAgent:     "keystone-core/1.0",
	}
}

// NewHTTPBundleFetcher creates a new HTTP bundle fetcher.
func NewHTTPBundleFetcher(config *BundleFetcherConfig) *HTTPBundleFetcher {
	if config == nil {
		config = DefaultBundleFetcherConfig()
	}

	transport := &http.Transport{
		TLSClientConfig: config.TLSConfig,
	}

	return &HTTPBundleFetcher{
		client: &http.Client{
			Transport: transport,
			Timeout:   config.Timeout,
		},
		TLSConfig:     config.TLSConfig,
		Timeout:       config.Timeout,
		MaxBundleSize: config.MaxBundleSize,
		UserAgent:     config.UserAgent,
	}
}

// Fetch retrieves a trust bundle from the given endpoint.
func (f *HTTPBundleFetcher) Fetch(ctx context.Context, endpoint string, profile string) (*identity.TrustBundle, error) {
	switch profile {
	case "https_web", "":
		return f.fetchHTTPSWeb(ctx, endpoint)
	case "https_spiffe":
		return f.fetchHTTPSSPIFFE(ctx, endpoint)
	case "spiffe_bundle_endpoint":
		return f.fetchSPIFFEBundleEndpoint(ctx, endpoint)
	default:
		return nil, fmt.Errorf("unknown bundle endpoint profile: %s", profile)
	}
}

// fetchHTTPSWeb fetches a trust bundle using standard HTTPS with web PKI.
func (f *HTTPBundleFetcher) fetchHTTPSWeb(ctx context.Context, endpoint string) (*identity.TrustBundle, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	req.Header.Set("Accept", "application/json, application/x-pem-file")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bundle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bundle endpoint returned status %d", resp.StatusCode)
	}

	// Read limited response
	reader := io.LimitReader(resp.Body, f.MaxBundleSize)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read bundle: %w", err)
	}

	// Try to parse based on content type
	contentType := resp.Header.Get("Content-Type")
	return f.parseBundleData(data, contentType)
}

// fetchHTTPSSPIFFE fetches a trust bundle using SPIFFE authentication.
func (f *HTTPBundleFetcher) fetchHTTPSSPIFFE(ctx context.Context, endpoint string) (*identity.TrustBundle, error) {
	// SPIFFE profile requires mutual TLS authentication
	// For now, fall back to HTTPS Web
	// Full implementation would use SVID for client authentication
	return f.fetchHTTPSWeb(ctx, endpoint)
}

// fetchSPIFFEBundleEndpoint fetches from a SPIFFE Federation Bundle Endpoint.
// Implements the SPIFFE Federation API.
func (f *HTTPBundleFetcher) fetchSPIFFEBundleEndpoint(ctx context.Context, endpoint string) (*identity.TrustBundle, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	if f.UserAgent != "" {
		req.Header.Set("User-Agent", f.UserAgent)
	}
	// SPIFFE Bundle Endpoint returns JWK Set
	req.Header.Set("Accept", "application/json")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch bundle: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bundle endpoint returned status %d", resp.StatusCode)
	}

	reader := io.LimitReader(resp.Body, f.MaxBundleSize)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("failed to read bundle: %w", err)
	}

	return f.parseSPIFFEBundleFormat(data)
}

// parseBundleData parses bundle data based on content type.
func (f *HTTPBundleFetcher) parseBundleData(data []byte, contentType string) (*identity.TrustBundle, error) {
	// Try JSON first
	if contentType == "application/json" || contentType == "" {
		bundle, err := f.parseJSONBundle(data)
		if err == nil {
			return bundle, nil
		}
	}

	// Try PEM
	if contentType == "application/x-pem-file" || contentType == "application/pem-certificate-chain" {
		return f.parsePEMBundle(data)
	}

	// Try to auto-detect
	if data[0] == '{' || data[0] == '[' {
		return f.parseJSONBundle(data)
	}
	if string(data[:min(len(data), 5)]) == "-----" {
		return f.parsePEMBundle(data)
	}

	return nil, fmt.Errorf("unable to parse bundle data")
}

// parseJSONBundle parses a JSON-formatted trust bundle.
func (f *HTTPBundleFetcher) parseJSONBundle(data []byte) (*identity.TrustBundle, error) {
	// Try SPIFFE bundle format first
	bundle, err := f.parseSPIFFEBundleFormat(data)
	if err == nil {
		return bundle, nil
	}

	// Try simple JSON with certificates array
	var simpleBundle struct {
		TrustDomain  string   `json:"trust_domain"`
		Certificates []string `json:"certificates"`
	}
	if err := json.Unmarshal(data, &simpleBundle); err == nil && len(simpleBundle.Certificates) > 0 {
		var certs []*x509.Certificate
		for _, certPEM := range simpleBundle.Certificates {
			parsed, err := f.parsePEMBundle([]byte(certPEM))
			if err != nil {
				continue
			}
			certs = append(certs, parsed.X509Authorities...)
		}
		return &identity.TrustBundle{
			TrustDomain:     simpleBundle.TrustDomain,
			X509Authorities: certs,
			UpdatedAt:       time.Now(),
		}, nil
	}

	return nil, fmt.Errorf("failed to parse JSON bundle")
}

// parsePEMBundle parses a PEM-formatted trust bundle.
func (f *HTTPBundleFetcher) parsePEMBundle(data []byte) (*identity.TrustBundle, error) {
	var certs []*x509.Certificate

	for {
		block, rest := pem.Decode(data)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("failed to parse certificate: %w", err)
			}
			certs = append(certs, cert)
		}
		data = rest
	}

	if len(certs) == 0 {
		return nil, fmt.Errorf("no certificates found in PEM data")
	}

	return &identity.TrustBundle{
		X509Authorities: certs,
		UpdatedAt:       time.Now(),
	}, nil
}

// SPIFFEBundle represents a SPIFFE Bundle as defined by the spec.
type SPIFFEBundle struct {
	// Keys contains the JWK Set keys (X.509 CAs and JWT signing keys).
	Keys []SPIFFEBundleKey `json:"keys"`

	// SpiffeRefreshHint is the suggested refresh interval in seconds.
	SpiffeRefreshHint int64 `json:"spiffe_refresh_hint,omitempty"`

	// SpiffeSequenceNumber is the bundle sequence number.
	SpiffeSequenceNumber uint64 `json:"spiffe_sequence_number,omitempty"`
}

// SPIFFEBundleKey represents a key in a SPIFFE Bundle.
type SPIFFEBundleKey struct {
	// Kty is the key type (e.g., "RSA", "EC").
	Kty string `json:"kty"`

	// Use is the key use (e.g., "x509-svid", "jwt-svid").
	Use string `json:"use"`

	// Kid is the key ID.
	Kid string `json:"kid,omitempty"`

	// X5c is the X.509 certificate chain (base64 DER).
	X5c []string `json:"x5c,omitempty"`

	// N is the RSA public key modulus (base64url).
	N string `json:"n,omitempty"`

	// E is the RSA public key exponent (base64url).
	E string `json:"e,omitempty"`

	// Crv is the EC curve (e.g., "P-256").
	Crv string `json:"crv,omitempty"`

	// X is the EC x coordinate (base64url).
	X string `json:"x,omitempty"`

	// Y is the EC y coordinate (base64url).
	Y string `json:"y,omitempty"`
}

// parseSPIFFEBundleFormat parses a SPIFFE Bundle (JWK Set) format.
func (f *HTTPBundleFetcher) parseSPIFFEBundleFormat(data []byte) (*identity.TrustBundle, error) {
	var bundle SPIFFEBundle
	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("failed to parse SPIFFE bundle: %w", err)
	}

	var certs []*x509.Certificate
	var jwtAuthorities []identity.JWTAuthority

	for _, key := range bundle.Keys {
		switch key.Use {
		case "x509-svid":
			// Parse X.509 certificates
			for _, certB64 := range key.X5c {
				certDER, err := decodeBase64(certB64)
				if err != nil {
					continue
				}
				cert, err := x509.ParseCertificate(certDER)
				if err != nil {
					continue
				}
				certs = append(certs, cert)
			}
		case "jwt-svid":
			// Parse JWT signing keys
			// Note: Full implementation would parse the JWK public key
			jwtAuthorities = append(jwtAuthorities, identity.JWTAuthority{
				KeyID: key.Kid,
			})
		}
	}

	var refreshHint time.Duration
	if bundle.SpiffeRefreshHint > 0 {
		refreshHint = time.Duration(bundle.SpiffeRefreshHint) * time.Second
	}

	return &identity.TrustBundle{
		X509Authorities: certs,
		JWTAuthorities:  jwtAuthorities,
		RefreshHint:     refreshHint,
		SequenceNumber:  bundle.SpiffeSequenceNumber,
		UpdatedAt:       time.Now(),
	}, nil
}

// decodeBase64 decodes standard or URL-safe base64.
func decodeBase64(s string) ([]byte, error) {
	// Try standard base64 first
	data, err := base64.StdEncoding.DecodeString(s)
	if err == nil {
		return data, nil
	}
	// Try URL-safe base64
	return base64.URLEncoding.DecodeString(s)
}

// min returns the smaller of two integers.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Verify HTTPBundleFetcher implements BundleFetcher
var _ BundleFetcher = (*HTTPBundleFetcher)(nil)
