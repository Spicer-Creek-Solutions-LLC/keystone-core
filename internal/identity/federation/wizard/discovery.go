// Package wizard provides an interactive TUI for trust federation setup.
package wizard

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// EndpointInfo contains information about a discovered bundle endpoint.
type EndpointInfo struct {
	// URL is the bundle endpoint URL.
	URL string

	// Profile is the detected endpoint profile (https_web, https_spiffe, spiffe_bundle_endpoint).
	Profile string

	// Reachable indicates whether the endpoint is reachable.
	Reachable bool

	// CertCount is the number of certificates in the bundle.
	CertCount int

	// ExpiresAt is when the earliest certificate expires.
	ExpiresAt time.Time

	// TrustDomain is the trust domain from the bundle (if available).
	TrustDomain string

	// SequenceNumber is the bundle sequence number (if available).
	SequenceNumber uint64

	// RefreshHint is the suggested refresh interval.
	RefreshHint time.Duration

	// Error contains any error encountered during discovery.
	Error string

	// ResponseTime is how long the endpoint took to respond.
	ResponseTime time.Duration
}

// EndpointDiscoveryResult contains the results of endpoint discovery.
type EndpointDiscoveryResult struct {
	// Domain is the trust domain being discovered.
	Domain string

	// Endpoints are the discovered endpoints.
	Endpoints []*EndpointInfo

	// BestEndpoint is the recommended endpoint to use.
	BestEndpoint *EndpointInfo

	// DNSSRVRecords indicates whether DNS SRV records were found.
	DNSSRVRecords bool
}

// DiscoveryOptions configures endpoint discovery.
type DiscoveryOptions struct {
	// Timeout is the timeout for each endpoint probe.
	Timeout time.Duration

	// MinTLSVersion is the minimum TLS version (1.2 or 1.3).
	MinTLSVersion string

	// TryDNSSRV indicates whether to try DNS SRV discovery.
	TryDNSSRV bool

	// MaxRedirects is the maximum number of redirects to follow.
	MaxRedirects int

	// SkipTLSVerify skips TLS certificate verification.
	SkipTLSVerify bool
}

// DefaultDiscoveryOptions returns default discovery options.
func DefaultDiscoveryOptions() *DiscoveryOptions {
	return &DiscoveryOptions{
		Timeout:       10 * time.Second,
		MinTLSVersion: "1.3",
		TryDNSSRV:     true,
		MaxRedirects:  3,
		SkipTLSVerify: false,
	}
}

// DiscoverBundleEndpoint attempts to discover the bundle endpoint for a trust domain.
func DiscoverBundleEndpoint(ctx context.Context, domain string, opts *DiscoveryOptions) (*EndpointDiscoveryResult, error) {
	if opts == nil {
		opts = DefaultDiscoveryOptions()
	}

	result := &EndpointDiscoveryResult{
		Domain:    domain,
		Endpoints: make([]*EndpointInfo, 0),
	}

	// Well-known URLs to try
	wellKnownURLs := []string{
		"https://" + domain + "/.well-known/spiffe-bundle",
		"https://" + domain + "/.well-known/spiffe/bundle",
		"https://" + domain + "/bundle.json",
	}

	// Try DNS SRV discovery
	if opts.TryDNSSRV {
		srvURLs, err := discoverViaDNSSRV(ctx, domain)
		if err == nil && len(srvURLs) > 0 {
			result.DNSSRVRecords = true
			// Prepend SRV URLs
			wellKnownURLs = append(srvURLs, wellKnownURLs...)
		}
	}

	// Probe each URL
	for _, url := range wellKnownURLs {
		info := probeEndpoint(ctx, url, opts)
		result.Endpoints = append(result.Endpoints, info)

		// Use first successful endpoint as best
		if result.BestEndpoint == nil && info.Reachable {
			result.BestEndpoint = info
		}
	}

	return result, nil
}

// discoverViaDNSSRV attempts DNS SRV discovery for SPIFFE bundle endpoint.
func discoverViaDNSSRV(ctx context.Context, domain string) ([]string, error) {
	// Try SPIFFE federation SRV record
	_, addrs, err := net.DefaultResolver.LookupSRV(ctx, "spiffe-bundle", "tcp", domain)
	if err != nil {
		return nil, err
	}

	var urls []string
	for _, addr := range addrs {
		// Construct URL from SRV record
		host := strings.TrimSuffix(addr.Target, ".")
		port := addr.Port

		var url string
		if port == 443 {
			url = fmt.Sprintf("https://%s/.well-known/spiffe-bundle", host)
		} else {
			url = fmt.Sprintf("https://%s:%d/.well-known/spiffe-bundle", host, port)
		}
		urls = append(urls, url)
	}

	return urls, nil
}

func newDiscoveryTLSConfig(opts *DiscoveryOptions) *tls.Config {
	if opts == nil {
		opts = DefaultDiscoveryOptions()
	}

	// #nosec G402 -- SkipTLSVerify is user-configured for discovery against dev/test endpoints.
	return &tls.Config{
		InsecureSkipVerify: opts.SkipTLSVerify,
		MinVersion:         parseTLSMinVersion(opts.MinTLSVersion),
	}
}

func parseTLSMinVersion(value string) uint16 {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "1.3", "tls1.3", "tls13":
		return tls.VersionTLS13
	case "1.2", "tls1.2", "tls12":
		return tls.VersionTLS12
	default:
		return tls.VersionTLS13
	}
}

// probeEndpoint probes a single endpoint URL.
func probeEndpoint(ctx context.Context, url string, opts *DiscoveryOptions) *EndpointInfo {
	info := &EndpointInfo{
		URL: url,
	}

	start := time.Now()

	// Create HTTP client with transport cleanup
	transport := &http.Transport{
		TLSClientConfig: newDiscoveryTLSConfig(opts),
	}
	defer transport.CloseIdleConnections()

	client := &http.Client{
		Transport: transport,
		Timeout:   opts.Timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= opts.MaxRedirects {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	// Make request
	req, err := http.NewRequestWithContext(ctx, "GET", url, http.NoBody)
	if err != nil {
		info.Error = fmt.Sprintf("Failed to create request: %v", err)
		return info
	}

	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "keystone-core/federation-wizard")

	resp, err := client.Do(req)
	if err != nil {
		info.Error = fmt.Sprintf("Request failed: %v", err)
		return info
	}
	defer resp.Body.Close()

	info.ResponseTime = time.Since(start)

	if resp.StatusCode != http.StatusOK {
		info.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return info
	}

	// Read response body (limited to 10MB)
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		info.Error = fmt.Sprintf("Failed to read response: %v", err)
		return info
	}

	// Detect profile and parse bundle
	contentType := resp.Header.Get("Content-Type")
	info.Profile = detectProfile(contentType, body)

	bundleInfo, err := parseBundleInfo(body)
	if err != nil {
		info.Error = fmt.Sprintf("Failed to parse bundle: %v", err)
		return info
	}

	info.Reachable = true
	info.CertCount = bundleInfo.CertCount
	info.ExpiresAt = bundleInfo.EarliestExpiry
	info.TrustDomain = bundleInfo.TrustDomain
	info.SequenceNumber = bundleInfo.SequenceNumber
	info.RefreshHint = bundleInfo.RefreshHint

	return info
}

// BundleInfo contains parsed information from a trust bundle.
type BundleInfo struct {
	TrustDomain    string
	CertCount      int
	EarliestExpiry time.Time
	SequenceNumber uint64
	RefreshHint    time.Duration
}

// parseBundleInfo parses bundle data to extract metadata.
func parseBundleInfo(data []byte) (*BundleInfo, error) {
	info := &BundleInfo{}

	// Try SPIFFE bundle format (JWK Set)
	var bundle struct {
		Keys []struct {
			Use string   `json:"use"`
			X5c []string `json:"x5c"`
		} `json:"keys"`
		SpiffeRefreshHint    int64  `json:"spiffe_refresh_hint"`
		SpiffeSequenceNumber uint64 `json:"spiffe_sequence_number"`
	}

	if err := json.Unmarshal(data, &bundle); err != nil {
		return nil, fmt.Errorf("failed to parse JSON: %w", err)
	}

	info.SequenceNumber = bundle.SpiffeSequenceNumber
	if bundle.SpiffeRefreshHint > 0 {
		info.RefreshHint = time.Duration(bundle.SpiffeRefreshHint) * time.Second
	}

	// Count and analyze certificates
	var earliestExpiry time.Time
	for _, key := range bundle.Keys {
		if key.Use == "x509-svid" || key.Use == "" {
			for _, certB64 := range key.X5c {
				certDER, err := base64.StdEncoding.DecodeString(certB64)
				if err != nil {
					// Try URL-safe base64
					certDER, err = base64.URLEncoding.DecodeString(certB64)
					if err != nil {
						continue
					}
				}

				cert, err := x509.ParseCertificate(certDER)
				if err != nil {
					continue
				}

				info.CertCount++
				if earliestExpiry.IsZero() || cert.NotAfter.Before(earliestExpiry) {
					earliestExpiry = cert.NotAfter
				}
			}
		}
	}

	info.EarliestExpiry = earliestExpiry
	return info, nil
}

// detectProfile detects the bundle endpoint profile.
func detectProfile(contentType string, body []byte) string {
	// Check content type
	if strings.Contains(contentType, "application/json") {
		// Try to determine if it's SPIFFE Bundle format
		var bundle struct {
			Keys []struct {
				Use string `json:"use"`
			} `json:"keys"`
		}
		if json.Unmarshal(body, &bundle) == nil && len(bundle.Keys) > 0 {
			// Has keys array, likely SPIFFE bundle endpoint format
			for _, key := range bundle.Keys {
				if key.Use == "x509-svid" || key.Use == "jwt-svid" {
					return "spiffe_bundle_endpoint"
				}
			}
		}
		return "https_web"
	}

	if strings.Contains(contentType, "application/x-pem") {
		return "https_web"
	}

	// Default to https_web
	return "https_web"
}

// ProbeEndpointDirect probes a specific endpoint URL with default options.
func ProbeEndpointDirect(ctx context.Context, url string) *EndpointInfo {
	return probeEndpoint(ctx, url, DefaultDiscoveryOptions())
}

// ValidateTrustDomain validates a trust domain string.
func ValidateTrustDomain(domain string) error {
	if domain == "" {
		return fmt.Errorf("trust domain cannot be empty")
	}

	// Must not contain scheme
	if strings.Contains(domain, "://") {
		return fmt.Errorf("trust domain should not include scheme (e.g., use 'example.org' not 'https://example.org')")
	}

	// Must not contain path
	if strings.Contains(domain, "/") {
		return fmt.Errorf("trust domain should not include path (e.g., use 'example.org' not 'example.org/path')")
	}

	// Must contain at least one dot (basic domain check)
	if !strings.Contains(domain, ".") {
		return fmt.Errorf("trust domain must be a valid domain name (e.g., 'example.org')")
	}

	// Should not start or end with dot
	if strings.HasPrefix(domain, ".") || strings.HasSuffix(domain, ".") {
		return fmt.Errorf("trust domain cannot start or end with a dot")
	}

	return nil
}

// ValidateEndpointURL validates a bundle endpoint URL.
func ValidateEndpointURL(url string) error {
	if url == "" {
		return fmt.Errorf("endpoint URL cannot be empty")
	}

	// Must start with https://
	if !strings.HasPrefix(url, "https://") {
		return fmt.Errorf("endpoint URL must use HTTPS (start with 'https://')")
	}

	return nil
}
