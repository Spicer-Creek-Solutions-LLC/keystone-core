// Package servicemesh detects and integrates with service mesh environments.
// It supports:
//   - Automatic mesh type detection (Istio, Linkerd, Consul, Kuma, OSM)
//   - SPIFFE identity extraction from mesh certificates
//   - Proxy configuration discovery
//   - mTLS configuration access
//   - Mesh metadata collection for targeting
//
// When running in a service mesh environment, agents can use mesh identity
// for authentication and expose mesh-aware metadata for targeting expressions.
package servicemesh

import "time"

// MeshType represents a service mesh type
type MeshType int

const (
	// MeshTypeUnknown represents an unknown or no service mesh
	MeshTypeUnknown MeshType = iota
	// MeshTypeIstio represents Istio service mesh
	MeshTypeIstio
	// MeshTypeLinkerd represents Linkerd service mesh
	MeshTypeLinkerd
	// MeshTypeConsul represents Consul Connect service mesh
	MeshTypeConsul
	// MeshTypeKuma represents Kuma service mesh
	MeshTypeKuma
	// MeshTypeOSM represents Open Service Mesh
	MeshTypeOSM
)

func (m MeshType) String() string {
	switch m {
	case MeshTypeIstio:
		return "istio"
	case MeshTypeLinkerd:
		return "linkerd"
	case MeshTypeConsul:
		return "consul"
	case MeshTypeKuma:
		return "kuma"
	case MeshTypeOSM:
		return "osm"
	default:
		return "unknown"
	}
}

// Metadata represents service mesh metadata
type Metadata struct {
	// MeshType is the detected service mesh
	MeshType MeshType

	// Version is the mesh version
	Version string

	// ProxyType is the sidecar proxy type (envoy, linkerd-proxy, consul-proxy)
	ProxyType string

	// ProxyVersion is the proxy version
	ProxyVersion string

	// ServiceName is the service name
	ServiceName string

	// ServiceNamespace is the service namespace
	ServiceNamespace string

	// ServiceVersion is the service version label
	ServiceVersion string

	// ClusterName is the mesh cluster name
	ClusterName string

	// MeshID is the mesh identifier
	MeshID string

	// TrustDomain is the SPIFFE trust domain
	TrustDomain string

	// WorkloadName is the workload/deployment name
	WorkloadName string

	// Labels are service mesh labels
	Labels map[string]string

	// Annotations are service mesh annotations
	Annotations map[string]string

	// ProxyConfig is the proxy configuration
	ProxyConfig *ProxyConfig

	// TLSConfig is the mTLS configuration
	TLSConfig *TLSConfig

	// DetectedAt is when the metadata was collected
	DetectedAt time.Time
}

// ProxyConfig represents sidecar proxy configuration
type ProxyConfig struct {
	// AdminPort is the proxy admin port (e.g., 15000 for Envoy)
	AdminPort int

	// InboundPort is the inbound traffic port
	InboundPort int

	// OutboundPort is the outbound traffic port
	OutboundPort int

	// MetricsPort is the metrics endpoint port
	MetricsPort int

	// HealthPort is the health check port
	HealthPort int

	// StatsPath is the stats endpoint path
	StatsPath string

	// ReadyPath is the readiness endpoint path
	ReadyPath string

	// LivePath is the liveness endpoint path
	LivePath string

	// ConfigPath is the proxy config file path
	ConfigPath string

	// LogLevel is the proxy log level
	LogLevel string
}

// TLSConfig represents mTLS configuration
type TLSConfig struct {
	// Enabled indicates if mTLS is enabled
	Enabled bool

	// Mode is the mTLS mode (STRICT, PERMISSIVE, DISABLED)
	Mode string

	// CertChainFile is the certificate chain file path
	CertChainFile string

	// PrivateKeyFile is the private key file path
	PrivateKeyFile string

	// CAFile is the CA certificate file path
	CAFile string

	// CertProvider is the certificate provider (Citadel, cert-manager, etc.)
	CertProvider string

	// SPIFFEID is the SPIFFE identity
	SPIFFEID string

	// ValidFrom is when the certificate is valid from
	ValidFrom time.Time

	// ValidUntil is when the certificate expires
	ValidUntil time.Time
}

// Detector is the interface for service mesh detection
type Detector interface {
	// Detect attempts to detect service mesh and collect metadata
	Detect() (*Metadata, error)

	// IsServiceMesh checks if running in a service mesh
	IsServiceMesh() bool

	// GetMeshType returns the detected mesh type
	GetMeshType() MeshType
}

// Config holds configuration for service mesh detection
type Config struct {
	// Timeout for API requests
	Timeout time.Duration

	// EnableIstio enables Istio detection
	EnableIstio bool

	// EnableLinkerd enables Linkerd detection
	EnableLinkerd bool

	// EnableConsul enables Consul detection
	EnableConsul bool

	// EnableKuma enables Kuma detection
	EnableKuma bool

	// EnableOSM enables Open Service Mesh detection
	EnableOSM bool

	// CacheDuration is how long to cache metadata
	CacheDuration time.Duration
}

// DefaultConfig returns the default service mesh detection configuration
func DefaultConfig() *Config {
	return &Config{
		Timeout:       5 * time.Second,
		EnableIstio:   true,
		EnableLinkerd: true,
		EnableConsul:  true,
		EnableKuma:    true,
		EnableOSM:     true,
		CacheDuration: 5 * time.Minute,
	}
}

// MetricsInfo represents service mesh metrics information
type MetricsInfo struct {
	// RequestsTotal is the total number of requests
	RequestsTotal int64

	// RequestDuration is the request duration histogram
	RequestDuration map[string]float64 // percentile -> duration

	// RequestsSuccessful is the number of successful requests
	RequestsSuccessful int64

	// RequestsFailed is the number of failed requests
	RequestsFailed int64

	// ActiveConnections is the number of active connections
	ActiveConnections int64

	// BytesSent is the total bytes sent
	BytesSent int64

	// BytesReceived is the total bytes received
	BytesReceived int64
}

// CircuitBreakerInfo represents circuit breaker configuration
type CircuitBreakerInfo struct {
	// Enabled indicates if circuit breaker is enabled
	Enabled bool

	// ConsecutiveErrors is the number of consecutive errors before tripping
	ConsecutiveErrors int

	// Interval is the time window for counting errors
	Interval time.Duration

	// BaseEjectionTime is the base time for ejecting a host
	BaseEjectionTime time.Duration

	// MaxEjectionPercent is the maximum % of hosts that can be ejected
	MaxEjectionPercent int
}

// RetryPolicyInfo represents retry policy configuration
type RetryPolicyInfo struct {
	// Enabled indicates if retries are enabled
	Enabled bool

	// Attempts is the maximum number of retry attempts
	Attempts int

	// PerTryTimeout is the timeout for each retry attempt
	PerTryTimeout time.Duration

	// RetryOn is the conditions that trigger retries (e.g., "5xx,reset,connect-failure")
	RetryOn []string
}
