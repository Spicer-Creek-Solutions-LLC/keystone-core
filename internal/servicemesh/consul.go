package servicemesh

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	// Consul Connect Envoy proxy admin port (configurable, default 19000)
	consulEnvoyAdminPort = "19000"
	consulEnvoyAdminURL  = "http://localhost:19000"

	// Consul environment variables
	consulServiceEnv         = "CONSUL_SERVICE"
	consulServiceNameEnv     = "CONSUL_SERVICE_NAME"
	consulServiceIDEnv       = "CONSUL_SERVICE_ID"
	consulNamespaceEnv       = "CONSUL_NAMESPACE"
	consulDatacenterEnv      = "CONSUL_DATACENTER"
	consulHTTPAddrEnv        = "CONSUL_HTTP_ADDR"
	consulGRPCAddrEnv        = "CONSUL_GRPC_ADDR"
	consulCACertEnv          = "CONSUL_CACERT"
	consulClientCertEnv      = "CONSUL_CLIENT_CERT"
	consulClientKeyEnv       = "CONSUL_CLIENT_KEY"
)

// ConsulDetector detects Consul Connect service mesh
type ConsulDetector struct {
	config     *Config
	httpClient *http.Client
}

// NewConsulDetector creates a new Consul detector
func NewConsulDetector(config *Config) *ConsulDetector {
	if config == nil {
		config = DefaultConfig()
	}

	return &ConsulDetector{
		config: config,
		httpClient: &http.Client{
			Timeout: config.Timeout,
		},
	}
}

// Detect attempts to detect Consul Connect and collect metadata
func (d *ConsulDetector) Detect() (*Metadata, error) {
	if !d.IsServiceMesh() {
		return nil, fmt.Errorf("not running in Consul Connect service mesh")
	}

	metadata := &Metadata{
		MeshType:    MeshTypeConsul,
		ProxyType:   "envoy",
		DetectedAt:  time.Now(),
		Labels:      make(map[string]string),
		Annotations: make(map[string]string),
	}

	// Collect from environment variables
	metadata.ServiceName = os.Getenv(consulServiceNameEnv)
	if metadata.ServiceName == "" {
		metadata.ServiceName = os.Getenv(consulServiceEnv)
	}

	metadata.ServiceNamespace = os.Getenv(consulNamespaceEnv)
	metadata.ClusterName = os.Getenv(consulDatacenterEnv)

	// Get service ID (unique identifier)
	if serviceID := os.Getenv(consulServiceIDEnv); serviceID != "" {
		metadata.Labels["service-id"] = serviceID
	}

	// Get Consul agent addresses
	if httpAddr := os.Getenv(consulHTTPAddrEnv); httpAddr != "" {
		metadata.Annotations["consul-http-addr"] = httpAddr
	}

	if grpcAddr := os.Getenv(consulGRPCAddrEnv); grpcAddr != "" {
		metadata.Annotations["consul-grpc-addr"] = grpcAddr
	}

	// Try to get proxy version from Envoy admin API
	if envoyInfo, err := d.getEnvoyServerInfo(); err == nil {
		metadata.ProxyVersion = envoyInfo.Version
	}

	// Get proxy configuration
	metadata.ProxyConfig = d.getProxyConfig()

	// Get TLS configuration
	if tlsConfig, err := d.getTLSConfig(); err == nil {
		metadata.TLSConfig = tlsConfig
	}

	return metadata, nil
}

// IsServiceMesh checks if running in Consul Connect
func (d *ConsulDetector) IsServiceMesh() bool {
	// Check for Consul Envoy proxy admin endpoint
	// Note: Port might be configured differently
	resp, err := d.httpClient.Get(consulEnvoyAdminURL + "/server_info")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			return true
		}
	}

	// Check for Consul environment variables
	if os.Getenv(consulServiceNameEnv) != "" || os.Getenv(consulServiceEnv) != "" {
		return true
	}

	// Check for Consul CA cert (indicates Connect is configured)
	if caCert := os.Getenv(consulCACertEnv); caCert != "" {
		if _, err := os.Stat(caCert); err == nil {
			return true
		}
	}

	return false
}

// GetMeshType returns Consul as the mesh type
func (d *ConsulDetector) GetMeshType() MeshType {
	return MeshTypeConsul
}

// getEnvoyServerInfo gets Envoy server info from admin API
func (d *ConsulDetector) getEnvoyServerInfo() (*consulEnvoyServerInfo, error) {
	resp, err := d.httpClient.Get(consulEnvoyAdminURL + "/server_info")
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

	var info consulEnvoyServerInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return nil, err
	}

	return &info, nil
}

// getProxyConfig returns the proxy configuration
func (d *ConsulDetector) getProxyConfig() *ProxyConfig {
	// Consul Connect uses Envoy with different default ports than Istio
	return &ProxyConfig{
		AdminPort:    19000, // Configurable via ADMIN_BIND
		InboundPort:  20000, // Configurable
		OutboundPort: 21000, // Configurable
		MetricsPort:  9102,  // Prometheus metrics endpoint
		HealthPort:   19000, // Same as admin
		StatsPath:    "/stats/prometheus",
		ReadyPath:    "/ready",
		LivePath:     "/server_info",
		LogLevel:     os.Getenv("CONSUL_LOG_LEVEL"),
	}
}

// getTLSConfig gets the TLS configuration
func (d *ConsulDetector) getTLSConfig() (*TLSConfig, error) {
	caCert := os.Getenv(consulCACertEnv)
	clientCert := os.Getenv(consulClientCertEnv)
	clientKey := os.Getenv(consulClientKeyEnv)

	if caCert == "" && clientCert == "" {
		// Try default paths
		caCert = "/consul/connect-inject/ca-certificates.crt"
		clientCert = "/consul/connect-inject/service.crt"
		clientKey = "/consul/connect-inject/service.key"
	}

	// Check if cert files exist
	if _, err := os.Stat(caCert); err != nil {
		if _, err := os.Stat(clientCert); err != nil {
			return nil, fmt.Errorf("no certificate files found")
		}
	}

	config := &TLSConfig{
		Enabled:        true,
		Mode:           "STRICT", // Consul Connect uses strict mTLS
		CertChainFile:  clientCert,
		PrivateKeyFile: clientKey,
		CAFile:         caCert,
		CertProvider:   "consul-ca", // Consul's built-in CA
	}

	// Build SPIFFE ID
	// Consul Connect format: spiffe://<trust-domain>/ns/<namespace>/dc/<datacenter>/svc/<service>
	trustDomain := "consul" // Consul's default trust domain
	namespace := os.Getenv(consulNamespaceEnv)
	if namespace == "" {
		namespace = "default"
	}
	datacenter := os.Getenv(consulDatacenterEnv)
	if datacenter == "" {
		datacenter = "dc1"
	}
	serviceName := os.Getenv(consulServiceNameEnv)

	if serviceName != "" {
		config.SPIFFEID = fmt.Sprintf("spiffe://%s/ns/%s/dc/%s/svc/%s",
			trustDomain, namespace, datacenter, serviceName)
	}

	return config, nil
}

// Helper types

type consulEnvoyServerInfo struct {
	Version            string                 `json:"version"`
	State              string                 `json:"state"`
	UptimeCurrentEpoch int64                  `json:"uptime_current_epoch"`
	UptimeAllEpochs    int64                  `json:"uptime_all_epochs"`
	CommandLineOptions map[string]interface{} `json:"command_line_options"`
}
