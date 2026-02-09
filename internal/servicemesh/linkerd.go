package servicemesh

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// Linkerd proxy admin port
	linkerdProxyAdminPort = "4191"
	linkerdProxyAdminURL  = "http://localhost:4191"

	// Linkerd proxy environment variables
	linkerdProxyVersionEnv     = "LINKERD2_PROXY_VERSION"
	linkerdProxyNamespaceEnv   = "LINKERD2_PROXY_NAMESPACE"
	linkerdProxyPodEnv         = "LINKERD2_PROXY_POD"
	linkerdProxyServiceEnv     = "LINKERD2_PROXY_SERVICE"
	linkerdProxyControlURLEnv  = "LINKERD2_PROXY_CONTROL_URL"
	linkerdProxyIdentityDirEnv = "LINKERD2_PROXY_IDENTITY_DIR"
	linkerdProxyTrustDomainEnv = "LINKERD2_PROXY_IDENTITY_TRUST_DOMAIN"
	linkerdProxyLogLevelEnv    = "LINKERD2_PROXY_LOG"
)

// LinkerdDetector detects Linkerd service mesh
type LinkerdDetector struct {
	config     *Config
	httpClient *http.Client
}

// NewLinkerdDetector creates a new Linkerd detector
func NewLinkerdDetector(config *Config) *LinkerdDetector {
	if config == nil {
		config = DefaultConfig()
	}

	return &LinkerdDetector{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Detect attempts to detect Linkerd and collect metadata
func (d *LinkerdDetector) Detect() (*Metadata, error) {
	if !d.IsServiceMesh() {
		return nil, fmt.Errorf("not running in Linkerd service mesh")
	}

	metadata := &Metadata{
		MeshType:    MeshTypeLinkerd,
		ProxyType:   "linkerd-proxy",
		DetectedAt:  time.Now(),
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	// Collect from environment variables
	metadata.ProxyVersion = os.Getenv(linkerdProxyVersionEnv)
	metadata.Version = metadata.ProxyVersion // Linkerd proxy version == Linkerd version
	metadata.ServiceNamespace = os.Getenv(linkerdProxyNamespaceEnv)
	metadata.TrustDomain = os.Getenv(linkerdProxyTrustDomainEnv)

	// Parse service name from LINKERD2_PROXY_SERVICE
	// Format: <service-name>.<namespace>.svc.cluster.local
	if svc := os.Getenv(linkerdProxyServiceEnv); svc != "" {
		parts := strings.Split(svc, ".")
		if len(parts) > 0 {
			metadata.ServiceName = parts[0]
		}
	}

	// Parse pod name
	if podName := os.Getenv(linkerdProxyPodEnv); podName != "" {
		metadata.Labels["pod"] = podName

		// Extract workload name from pod name
		// Format: <workload>-<hash>-<id>
		parts := strings.Split(podName, "-")
		if len(parts) >= 3 {
			metadata.WorkloadName = strings.Join(parts[:len(parts)-2], "-")
		}
	}

	// Get proxy configuration
	metadata.ProxyConfig = d.getProxyConfig()

	// Get TLS configuration
	if tlsConfig, err := d.getTLSConfig(); err == nil {
		metadata.TLSConfig = tlsConfig
	}

	return metadata, nil
}

// IsServiceMesh checks if running in Linkerd service mesh
func (d *LinkerdDetector) IsServiceMesh() bool {
	// Check for Linkerd proxy admin endpoint
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, linkerdProxyAdminURL+"/metrics", http.NoBody)
	if err == nil {
		resp, err := d.httpClient.Do(req)
		if err == nil {
			defer resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return true
			}
		}
	}

	// Check for Linkerd environment variables
	if os.Getenv(linkerdProxyVersionEnv) != "" {
		return true
	}

	// Check for Linkerd identity directory
	if identityDir := os.Getenv(linkerdProxyIdentityDirEnv); identityDir != "" {
		if _, err := os.Stat(identityDir); err == nil {
			return true
		}
	}

	return false
}

// GetMeshType returns Linkerd as the mesh type
func (d *LinkerdDetector) GetMeshType() MeshType {
	return MeshTypeLinkerd
}

// getProxyConfig returns the proxy configuration
func (d *LinkerdDetector) getProxyConfig() *ProxyConfig {
	return &ProxyConfig{
		AdminPort:    4191,
		InboundPort:  4143, // Linkerd inbound proxy port
		OutboundPort: 4140, // Linkerd outbound proxy port
		MetricsPort:  4191, // Same as admin port
		HealthPort:   4191, // Same as admin port
		StatsPath:    "/metrics",
		ReadyPath:    "/ready",
		LivePath:     "/live",
		LogLevel:     os.Getenv(linkerdProxyLogLevelEnv),
	}
}

// getTLSConfig gets the TLS configuration
func (d *LinkerdDetector) getTLSConfig() (*TLSConfig, error) {
	identityDir := os.Getenv(linkerdProxyIdentityDirEnv)
	if identityDir == "" {
		identityDir = "/var/run/linkerd/identity/end-entity"
	}

	certChainFile := identityDir + "/cert.crt"
	privateKeyFile := identityDir + "/key.p8"
	caFile := identityDir + "/trust-anchors.crt"

	// Check if cert files exist
	if _, err := os.Stat(certChainFile); err != nil {
		return nil, fmt.Errorf("no certificate chain found")
	}

	config := &TLSConfig{
		Enabled:        true,
		Mode:           "STRICT", // Linkerd always uses strict mTLS
		CertChainFile:  certChainFile,
		PrivateKeyFile: privateKeyFile,
		CAFile:         caFile,
		CertProvider:   "linkerd-identity", // Linkerd's identity controller
	}

	// Build SPIFFE ID
	// Format: spiffe://<trust-domain>/ns/<namespace>/sa/<service-account>
	trustDomain := os.Getenv(linkerdProxyTrustDomainEnv)
	namespace := os.Getenv(linkerdProxyNamespaceEnv)
	serviceAccount := os.Getenv("SERVICE_ACCOUNT")

	if trustDomain != "" && namespace != "" && serviceAccount != "" {
		config.SPIFFEID = fmt.Sprintf("spiffe://%s/%s/%s",
			trustDomain, namespace, serviceAccount)
	}

	return config, nil
}
