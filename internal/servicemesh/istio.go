package servicemesh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

const (
	// Istio Envoy proxy admin port
	istioEnvoyAdminPort = "15000"
	istioEnvoyAdminURL  = "http://localhost:15000"

	// Istio metadata file injected by sidecar injector
	istioMetadataFile = "/var/run/secrets/istio/labels"

	// Istio environment variables
	istioServiceEnv      = "ISTIO_SERVICE"
	istioNamespaceEnv    = "POD_NAMESPACE"
	istioPodNameEnv      = "POD_NAME"
	istioMetaMeshIDEnv   = "ISTIO_META_MESH_ID"
	istioMetaClusterEnv  = "ISTIO_META_CLUSTER_ID"
	istioMetaTrustDomain = "TRUST_DOMAIN"
)

// IstioDetector detects Istio service mesh
type IstioDetector struct {
	config     *Config
	httpClient *http.Client
}

// NewIstioDetector creates a new Istio detector
func NewIstioDetector(config *Config) *IstioDetector {
	if config == nil {
		config = DefaultConfig()
	}

	return &IstioDetector{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Detect attempts to detect Istio and collect metadata
func (d *IstioDetector) Detect() (*Metadata, error) {
	if !d.IsServiceMesh() {
		return nil, fmt.Errorf("not running in Istio service mesh")
	}

	metadata := &Metadata{
		MeshType:    MeshTypeIstio,
		ProxyType:   "envoy",
		DetectedAt:  time.Now(),
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	// Collect from environment variables
	metadata.ServiceName = os.Getenv(istioServiceEnv)
	if metadata.ServiceName == "" {
		metadata.ServiceName = os.Getenv("SERVICE_NAME")
	}

	metadata.ServiceNamespace = os.Getenv(istioNamespaceEnv)
	metadata.MeshID = os.Getenv(istioMetaMeshIDEnv)
	metadata.ClusterName = os.Getenv(istioMetaClusterEnv)
	metadata.TrustDomain = os.Getenv(istioMetaTrustDomain)

	// Get service version from environment
	metadata.ServiceVersion = os.Getenv("SERVICE_VERSION")
	if metadata.ServiceVersion == "" {
		metadata.ServiceVersion = os.Getenv("VERSION")
	}

	// Try to read Istio metadata file
	if labels, err := d.readIstioLabels(); err == nil {
		for k, v := range labels {
			metadata.Labels[k] = v
		}
	}

	// Get proxy version from Envoy admin API
	if proxyInfo, err := d.getEnvoyServerInfo(); err == nil {
		metadata.ProxyVersion = proxyInfo.Version
		metadata.Version = extractIstioVersion(proxyInfo.Version)
	}

	// Get proxy configuration
	metadata.ProxyConfig = d.getProxyConfig()

	// Get TLS configuration
	if tlsConfig, err := d.getTLSConfig(); err == nil {
		metadata.TLSConfig = tlsConfig
	}

	// Parse workload name from pod name or labels
	if podName := os.Getenv(istioPodNameEnv); podName != "" {
		// Pod name format: <workload>-<hash>-<id>
		parts := strings.Split(podName, "-")
		if len(parts) >= 3 {
			// Workload name is everything except last 2 parts (hash and id)
			metadata.WorkloadName = strings.Join(parts[:len(parts)-2], "-")
		}
	}

	return metadata, nil
}

// IsServiceMesh checks if running in Istio service mesh
func (d *IstioDetector) IsServiceMesh() bool {
	// Check for Envoy proxy admin endpoint
	resp, err := d.httpClient.Get(istioEnvoyAdminURL + "/server_info")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}

	// Check for Istio metadata file
	if _, err := os.Stat(istioMetadataFile); err == nil {
		return true
	}

	// Check for Istio environment variables
	if os.Getenv(istioMetaMeshIDEnv) != "" {
		return true
	}

	return false
}

// GetMeshType returns Istio as the mesh type
func (d *IstioDetector) GetMeshType() MeshType {
	return MeshTypeIstio
}

// readIstioLabels reads Istio labels from metadata file
func (d *IstioDetector) readIstioLabels() (map[string]string, error) {
	content, err := os.ReadFile(istioMetadataFile)
	if err != nil {
		return nil, err
	}

	labels := make(map[string]string)
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			labels[strings.TrimSpace(parts[0])] = strings.Trim(strings.TrimSpace(parts[1]), "\"")
		}
	}

	return labels, nil
}

// getEnvoyServerInfo gets Envoy server info from admin API
func (d *IstioDetector) getEnvoyServerInfo() (*envoyServerInfo, error) {
	resp, err := d.httpClient.Get(istioEnvoyAdminURL + "/server_info")
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server_info request failed: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var info envoyServerInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// getProxyConfig returns the proxy configuration
func (d *IstioDetector) getProxyConfig() *ProxyConfig {
	return &ProxyConfig{
		AdminPort:    15000,
		InboundPort:  15006,
		OutboundPort: 15001,
		MetricsPort:  15020,
		HealthPort:   15021,
		StatsPath:    "/stats/prometheus",
		ReadyPath:    "/healthz/ready",
		LivePath:     "/healthz/live",
		LogLevel:     os.Getenv("ISTIO_LOG_LEVEL"),
	}
}

// getTLSConfig gets the TLS configuration
func (d *IstioDetector) getTLSConfig() (*TLSConfig, error) {
	// Istio stores certs in /etc/certs or /var/run/secrets/istio
	certPaths := []string{
		"/etc/certs/cert-chain.pem",
		"/var/run/secrets/istio/cert-chain.pem",
	}

	var certChainFile string
	for _, path := range certPaths {
		if _, err := os.Stat(path); err == nil {
			certChainFile = path
			break
		}
	}

	if certChainFile == "" {
		return nil, fmt.Errorf("no certificate chain found")
	}

	// Determine private key path
	privateKeyFile := strings.Replace(certChainFile, "cert-chain.pem", "key.pem", 1)
	caFile := strings.Replace(certChainFile, "cert-chain.pem", "root-cert.pem", 1)

	config := &TLSConfig{
		Enabled:        true,
		Mode:           os.Getenv("ISTIO_MTLS_MODE"),
		CertChainFile:  certChainFile,
		PrivateKeyFile: privateKeyFile,
		CAFile:         caFile,
		CertProvider:   "istiod", // Istio's certificate authority
	}

	// Default to STRICT if not set
	if config.Mode == "" {
		config.Mode = "STRICT"
	}

	// Try to get SPIFFE ID from environment or cert
	config.SPIFFEID = os.Getenv("SPIFFE_ID")
	if config.SPIFFEID == "" {
		// Format: spiffe://<trust-domain>/ns/<namespace>/sa/<service-account>
		trustDomain := os.Getenv(istioMetaTrustDomain)
		namespace := os.Getenv(istioNamespaceEnv)
		serviceAccount := os.Getenv("SERVICE_ACCOUNT")

		if trustDomain != "" && namespace != "" && serviceAccount != "" {
			config.SPIFFEID = fmt.Sprintf("spiffe://%s/ns/%s/sa/%s",
				trustDomain, namespace, serviceAccount)
		}
	}

	return config, nil
}

// Helper types

type envoyServerInfo struct {
	Version              string `json:"version"`
	State                string `json:"state"`
	UptimeCurrentEpoch   int64  `json:"uptime_current_epoch"`
	UptimeAllEpochs      int64  `json:"uptime_all_epochs"`
	CommandLineOptions   map[string]interface{} `json:"command_line_options"`
}

// extractIstioVersion extracts Istio version from Envoy version string
// Envoy version format: "1.18.3/1.18.3/Clean/RELEASE/BoringSSL"
// Istio typically uses a custom build tag
func extractIstioVersion(envoyVersion string) string {
	// For now, return the Envoy version
	// In production, would parse from build metadata
	parts := strings.Split(envoyVersion, "/")
	if len(parts) > 0 {
		return parts[0]
	}
	return envoyVersion
}
