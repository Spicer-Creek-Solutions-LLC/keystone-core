package nats

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ============================================================================
// Discovery Types & Interfaces
// ============================================================================

// DiscoveryMethod represents the type of discovery mechanism
type DiscoveryMethod string

const (
	// DiscoveryMethodDNS uses DNS SRV/A/AAAA records
	DiscoveryMethodDNS DiscoveryMethod = "dns"
	// DiscoveryMethodMDNS uses multicast DNS (Bonjour/Avahi)
	DiscoveryMethodMDNS DiscoveryMethod = "mdns"
	// DiscoveryMethodKubernetes uses Kubernetes service discovery
	DiscoveryMethodKubernetes DiscoveryMethod = "kubernetes"
	// DiscoveryMethodConsul uses Consul service discovery
	DiscoveryMethodConsul DiscoveryMethod = "consul"
	// DiscoveryMethodEtcd uses etcd service discovery
	DiscoveryMethodEtcd DiscoveryMethod = "etcd"
	// DiscoveryMethodStatic uses static configuration
	DiscoveryMethodStatic DiscoveryMethod = "static"
)

func (m DiscoveryMethod) String() string {
	return string(m)
}

// DiscoveredEndpoint represents a discovered NATS endpoint
type DiscoveredEndpoint struct {
	// URL is the NATS connection URL
	URL string

	// Host is the hostname or IP
	Host string

	// Port is the NATS port
	Port int

	// Priority for connection preference (lower = preferred)
	Priority int

	// Weight for load balancing within same priority
	Weight int

	// TLS indicates if TLS is required
	TLS bool

	// Scheme is the connection scheme (nats, tls, ws, wss)
	Scheme Scheme

	// Method is how this endpoint was discovered
	Method DiscoveryMethod

	// TTL is how long this endpoint is valid
	TTL time.Duration

	// DiscoveredAt is when this endpoint was discovered
	DiscoveredAt time.Time

	// Metadata contains additional discovery-specific metadata
	Metadata map[string]string
}

// IsExpired returns true if the endpoint has expired
func (e *DiscoveredEndpoint) IsExpired() bool {
	if e.TTL <= 0 {
		return false // No TTL means never expires
	}
	return time.Since(e.DiscoveredAt) > e.TTL
}

// ToEndpoint converts to an Endpoint
func (e *DiscoveredEndpoint) ToEndpoint() *Endpoint {
	return &Endpoint{
		URL:      e.URL,
		Host:     e.Host,
		Port:     e.Port,
		Priority: e.Priority,
		Scheme:   e.Scheme,
	}
}

// Discoverer is the interface for endpoint discovery mechanisms
type Discoverer interface {
	// Method returns the discovery method
	Method() DiscoveryMethod

	// Discover performs endpoint discovery
	Discover(ctx context.Context) ([]*DiscoveredEndpoint, error)

	// Watch watches for endpoint changes (optional, returns nil if not supported)
	Watch(ctx context.Context, callback func([]*DiscoveredEndpoint)) error

	// Close stops the discoverer
	Close() error
}

// ============================================================================
// DNS-Based Discovery - T7.1
// ============================================================================

// DNSDiscoveryConfig holds DNS discovery configuration
type DNSDiscoveryConfig struct {
	// ServiceName is the service name for SRV lookup (e.g., "_nats._tcp.example.com")
	ServiceName string

	// Domain is the domain for SRV lookup
	Domain string

	// Resolver is a custom DNS resolver (nil = system default)
	Resolver *net.Resolver

	// RefreshInterval is how often to refresh DNS records
	RefreshInterval time.Duration

	// Timeout is the DNS lookup timeout
	Timeout time.Duration

	// FallbackToA enables fallback to A/AAAA records if SRV fails
	FallbackToA bool

	// DefaultPort is used when no port is in DNS records
	DefaultPort int

	// DefaultScheme is the default connection scheme
	DefaultScheme Scheme

	// UseTLS forces TLS for discovered endpoints
	UseTLS bool
}

// DefaultDNSDiscoveryConfig returns default DNS discovery configuration
func DefaultDNSDiscoveryConfig() *DNSDiscoveryConfig {
	return &DNSDiscoveryConfig{
		ServiceName:     "_nats._tcp",
		RefreshInterval: 30 * time.Second,
		Timeout:         5 * time.Second,
		FallbackToA:     true,
		DefaultPort:     4222,
		DefaultScheme:   SchemeNATS,
		UseTLS:          false,
	}
}

// Validate validates the DNS discovery configuration
func (c *DNSDiscoveryConfig) Validate() error {
	if c.Domain == "" && c.ServiceName == "" {
		return errors.New("domain or service name required")
	}
	if c.RefreshInterval < 0 {
		return errors.New("refresh interval must be non-negative")
	}
	if c.Timeout < 0 {
		return errors.New("timeout must be non-negative")
	}
	if c.DefaultPort < 0 || c.DefaultPort > 65535 {
		return fmt.Errorf("invalid default port: %d", c.DefaultPort)
	}
	return nil
}

// DNSDiscoverer discovers NATS endpoints via DNS
type DNSDiscoverer struct {
	config   *DNSDiscoveryConfig
	resolver *net.Resolver

	// State
	mu        sync.RWMutex
	endpoints []*DiscoveredEndpoint
	lastRefresh time.Time

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewDNSDiscoverer creates a new DNS discoverer
func NewDNSDiscoverer(config *DNSDiscoveryConfig) (*DNSDiscoverer, error) {
	if config == nil {
		config = DefaultDNSDiscoveryConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	resolver := config.Resolver
	if resolver == nil {
		resolver = net.DefaultResolver
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DNSDiscoverer{
		config:   config,
		resolver: resolver,
		ctx:      ctx,
		cancel:   cancel,
	}, nil
}

func (d *DNSDiscoverer) Method() DiscoveryMethod {
	return DiscoveryMethodDNS
}

func (d *DNSDiscoverer) Discover(ctx context.Context) ([]*DiscoveredEndpoint, error) {
	// Try SRV records first
	endpoints, err := d.discoverSRV(ctx)
	if err == nil && len(endpoints) > 0 {
		d.updateEndpoints(endpoints)
		return endpoints, nil
	}

	// Fallback to A/AAAA records if enabled
	if d.config.FallbackToA && d.config.Domain != "" {
		endpoints, err = d.discoverA(ctx)
		if err == nil && len(endpoints) > 0 {
			d.updateEndpoints(endpoints)
			return endpoints, nil
		}
	}

	if err != nil {
		return nil, err
	}
	return nil, errors.New("no endpoints discovered")
}

func (d *DNSDiscoverer) discoverSRV(ctx context.Context) ([]*DiscoveredEndpoint, error) {
	if d.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.config.Timeout)
		defer cancel()
	}

	// Build SRV record name
	srvName := d.config.ServiceName
	if d.config.Domain != "" {
		srvName = fmt.Sprintf("%s.%s", d.config.ServiceName, d.config.Domain)
	}

	// Lookup SRV records
	_, addrs, err := d.resolver.LookupSRV(ctx, "", "", srvName)
	if err != nil {
		return nil, fmt.Errorf("SRV lookup failed: %w", err)
	}

	endpoints := make([]*DiscoveredEndpoint, 0, len(addrs))
	for _, srv := range addrs {
		host := strings.TrimSuffix(srv.Target, ".")
		port := int(srv.Port)
		if port == 0 {
			port = d.config.DefaultPort
		}

		scheme := d.config.DefaultScheme
		if d.config.UseTLS {
			scheme = SchemeTLS
		}

		url := fmt.Sprintf("%s://%s:%d", scheme, host, port)

		endpoints = append(endpoints, &DiscoveredEndpoint{
			URL:          url,
			Host:         host,
			Port:         port,
			Priority:     int(srv.Priority),
			Weight:       int(srv.Weight),
			TLS:          d.config.UseTLS,
			Scheme:       scheme,
			Method:       DiscoveryMethodDNS,
			TTL:          d.config.RefreshInterval,
			DiscoveredAt: time.Now(),
			Metadata: map[string]string{
				"record_type": "SRV",
				"srv_name":    srvName,
			},
		})
	}

	// Sort by priority (ascending), then by weight (descending)
	sort.Slice(endpoints, func(i, j int) bool {
		if endpoints[i].Priority != endpoints[j].Priority {
			return endpoints[i].Priority < endpoints[j].Priority
		}
		return endpoints[i].Weight > endpoints[j].Weight
	})

	return endpoints, nil
}

func (d *DNSDiscoverer) discoverA(ctx context.Context) ([]*DiscoveredEndpoint, error) {
	if d.config.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, d.config.Timeout)
		defer cancel()
	}

	// Lookup A/AAAA records
	ips, err := d.resolver.LookupIPAddr(ctx, d.config.Domain)
	if err != nil {
		return nil, fmt.Errorf("A/AAAA lookup failed: %w", err)
	}

	endpoints := make([]*DiscoveredEndpoint, 0, len(ips))
	for i, ip := range ips {
		host := ip.IP.String()
		port := d.config.DefaultPort

		scheme := d.config.DefaultScheme
		if d.config.UseTLS {
			scheme = SchemeTLS
		}

		url := fmt.Sprintf("%s://%s:%d", scheme, host, port)

		endpoints = append(endpoints, &DiscoveredEndpoint{
			URL:          url,
			Host:         host,
			Port:         port,
			Priority:     i, // Use index as priority
			Weight:       1,
			TLS:          d.config.UseTLS,
			Scheme:       scheme,
			Method:       DiscoveryMethodDNS,
			TTL:          d.config.RefreshInterval,
			DiscoveredAt: time.Now(),
			Metadata: map[string]string{
				"record_type": "A/AAAA",
				"domain":      d.config.Domain,
			},
		})
	}

	return endpoints, nil
}

func (d *DNSDiscoverer) updateEndpoints(endpoints []*DiscoveredEndpoint) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.endpoints = endpoints
	d.lastRefresh = time.Now()
}

func (d *DNSDiscoverer) Watch(ctx context.Context, callback func([]*DiscoveredEndpoint)) error {
	if d.config.RefreshInterval <= 0 {
		return errors.New("refresh interval must be positive for watching")
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		ticker := time.NewTicker(d.config.RefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-d.ctx.Done():
				return
			case <-ticker.C:
				endpoints, err := d.Discover(ctx)
				if err == nil && callback != nil {
					callback(endpoints)
				}
			}
		}
	}()

	return nil
}

func (d *DNSDiscoverer) Close() error {
	d.cancel()
	d.wg.Wait()
	return nil
}

// GetCachedEndpoints returns the last discovered endpoints
func (d *DNSDiscoverer) GetCachedEndpoints() []*DiscoveredEndpoint {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.endpoints
}

// ============================================================================
// mDNS/Bonjour Discovery - T7.2
// ============================================================================

// MDNSDiscoveryConfig holds mDNS discovery configuration
type MDNSDiscoveryConfig struct {
	// ServiceType is the mDNS service type (default: "_nats._tcp")
	ServiceType string

	// Domain is the mDNS domain (default: "local.")
	Domain string

	// BrowseTimeout is how long to browse for services
	BrowseTimeout time.Duration

	// RefreshInterval is how often to refresh discovery
	RefreshInterval time.Duration

	// Interface is the network interface to use (nil = all)
	Interface *net.Interface

	// DefaultScheme is the default connection scheme
	DefaultScheme Scheme
}

// DefaultMDNSDiscoveryConfig returns default mDNS discovery configuration
func DefaultMDNSDiscoveryConfig() *MDNSDiscoveryConfig {
	return &MDNSDiscoveryConfig{
		ServiceType:     "_nats._tcp",
		Domain:          "local.",
		BrowseTimeout:   3 * time.Second,
		RefreshInterval: 30 * time.Second,
		DefaultScheme:   SchemeNATS,
	}
}

// Validate validates the mDNS discovery configuration
func (c *MDNSDiscoveryConfig) Validate() error {
	if c.ServiceType == "" {
		return errors.New("service type required")
	}
	if c.BrowseTimeout < 0 {
		return errors.New("browse timeout must be non-negative")
	}
	return nil
}

// MDNSDiscoverer discovers NATS endpoints via mDNS
type MDNSDiscoverer struct {
	config *MDNSDiscoveryConfig

	// State
	mu        sync.RWMutex
	endpoints []*DiscoveredEndpoint

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewMDNSDiscoverer creates a new mDNS discoverer
func NewMDNSDiscoverer(config *MDNSDiscoveryConfig) (*MDNSDiscoverer, error) {
	if config == nil {
		config = DefaultMDNSDiscoveryConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &MDNSDiscoverer{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (d *MDNSDiscoverer) Method() DiscoveryMethod {
	return DiscoveryMethodMDNS
}

func (d *MDNSDiscoverer) Discover(ctx context.Context) ([]*DiscoveredEndpoint, error) {
	// Note: Full mDNS implementation requires a library like github.com/hashicorp/mdns
	// This is a placeholder that demonstrates the interface
	// In production, use the mdns library for actual mDNS browsing

	// For now, return empty - actual implementation would use mDNS library
	return []*DiscoveredEndpoint{}, nil
}

func (d *MDNSDiscoverer) Watch(ctx context.Context, callback func([]*DiscoveredEndpoint)) error {
	if d.config.RefreshInterval <= 0 {
		return errors.New("refresh interval must be positive for watching")
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		ticker := time.NewTicker(d.config.RefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-d.ctx.Done():
				return
			case <-ticker.C:
				endpoints, err := d.Discover(ctx)
				if err == nil && callback != nil {
					callback(endpoints)
				}
			}
		}
	}()

	return nil
}

func (d *MDNSDiscoverer) Close() error {
	d.cancel()
	d.wg.Wait()
	return nil
}

// ============================================================================
// Kubernetes Discovery - T7.3
// ============================================================================

// KubernetesDiscoveryConfig holds Kubernetes discovery configuration
type KubernetesDiscoveryConfig struct {
	// ServiceName is the Kubernetes service name
	ServiceName string

	// Namespace is the Kubernetes namespace (empty = current namespace)
	Namespace string

	// PortName is the port name to use (empty = first port)
	PortName string

	// LabelSelector filters services by labels
	LabelSelector string

	// InCluster indicates if running inside a Kubernetes cluster
	InCluster bool

	// Kubeconfig is the path to kubeconfig file (for out-of-cluster)
	Kubeconfig string

	// RefreshInterval is how often to refresh endpoints
	RefreshInterval time.Duration

	// DefaultScheme is the default connection scheme
	DefaultScheme Scheme

	// UseTLS forces TLS for discovered endpoints
	UseTLS bool
}

// DefaultKubernetesDiscoveryConfig returns default Kubernetes discovery configuration
func DefaultKubernetesDiscoveryConfig() *KubernetesDiscoveryConfig {
	return &KubernetesDiscoveryConfig{
		ServiceName:     "nats",
		Namespace:       "",
		PortName:        "nats",
		InCluster:       true,
		RefreshInterval: 30 * time.Second,
		DefaultScheme:   SchemeNATS,
		UseTLS:          false,
	}
}

// Validate validates the Kubernetes discovery configuration
func (c *KubernetesDiscoveryConfig) Validate() error {
	if c.ServiceName == "" {
		return errors.New("service name required")
	}
	if c.RefreshInterval < 0 {
		return errors.New("refresh interval must be non-negative")
	}
	return nil
}

// KubernetesDiscoverer discovers NATS endpoints via Kubernetes
type KubernetesDiscoverer struct {
	config *KubernetesDiscoveryConfig

	// State
	mu        sync.RWMutex
	endpoints []*DiscoveredEndpoint

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewKubernetesDiscoverer creates a new Kubernetes discoverer
func NewKubernetesDiscoverer(config *KubernetesDiscoveryConfig) (*KubernetesDiscoverer, error) {
	if config == nil {
		config = DefaultKubernetesDiscoveryConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &KubernetesDiscoverer{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (d *KubernetesDiscoverer) Method() DiscoveryMethod {
	return DiscoveryMethodKubernetes
}

func (d *KubernetesDiscoverer) Discover(ctx context.Context) ([]*DiscoveredEndpoint, error) {
	// Note: Full Kubernetes implementation requires k8s.io/client-go
	// This is a placeholder that demonstrates the interface

	// For Kubernetes headless service, we would:
	// 1. Get the Endpoints or EndpointSlice for the service
	// 2. Extract IP addresses and ports
	// 3. Build endpoint list

	// Placeholder: try DNS-based service discovery within Kubernetes
	// Format: <service>.<namespace>.svc.cluster.local
	namespace := d.config.Namespace
	if namespace == "" {
		namespace = "default"
	}

	dnsName := fmt.Sprintf("%s.%s.svc.cluster.local", d.config.ServiceName, namespace)

	// Lookup the DNS name
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, dnsName)
	if err != nil {
		return nil, fmt.Errorf("kubernetes DNS lookup failed: %w", err)
	}

	endpoints := make([]*DiscoveredEndpoint, 0, len(ips))
	for i, ip := range ips {
		host := ip.IP.String()
		port := 4222 // Default NATS port

		scheme := d.config.DefaultScheme
		if d.config.UseTLS {
			scheme = SchemeTLS
		}

		url := fmt.Sprintf("%s://%s:%d", scheme, host, port)

		endpoints = append(endpoints, &DiscoveredEndpoint{
			URL:          url,
			Host:         host,
			Port:         port,
			Priority:     i,
			Weight:       1,
			TLS:          d.config.UseTLS,
			Scheme:       scheme,
			Method:       DiscoveryMethodKubernetes,
			TTL:          d.config.RefreshInterval,
			DiscoveredAt: time.Now(),
			Metadata: map[string]string{
				"service":   d.config.ServiceName,
				"namespace": namespace,
				"dns_name":  dnsName,
			},
		})
	}

	d.mu.Lock()
	d.endpoints = endpoints
	d.mu.Unlock()

	return endpoints, nil
}

func (d *KubernetesDiscoverer) Watch(ctx context.Context, callback func([]*DiscoveredEndpoint)) error {
	if d.config.RefreshInterval <= 0 {
		return errors.New("refresh interval must be positive for watching")
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		ticker := time.NewTicker(d.config.RefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-d.ctx.Done():
				return
			case <-ticker.C:
				endpoints, err := d.Discover(ctx)
				if err == nil && callback != nil {
					callback(endpoints)
				}
			}
		}
	}()

	return nil
}

func (d *KubernetesDiscoverer) Close() error {
	d.cancel()
	d.wg.Wait()
	return nil
}

// ============================================================================
// Service Registry Discovery - T7.4
// ============================================================================

// ServiceRegistryConfig holds service registry discovery configuration
type ServiceRegistryConfig struct {
	// Type is the registry type (consul, etcd)
	Type string

	// Address is the registry address
	Address string

	// ServiceName is the service to look up
	ServiceName string

	// Tags filters services by tags (Consul)
	Tags []string

	// Prefix is the key prefix (etcd)
	Prefix string

	// RefreshInterval is how often to refresh
	RefreshInterval time.Duration

	// Timeout is the lookup timeout
	Timeout time.Duration

	// TLS configuration for registry connection
	TLS *TLSStrategyConfig

	// Token for authentication
	Token string

	// DefaultScheme is the default connection scheme
	DefaultScheme Scheme

	// UseTLS forces TLS for discovered endpoints
	UseTLS bool
}

// DefaultServiceRegistryConfig returns default service registry configuration
func DefaultServiceRegistryConfig() *ServiceRegistryConfig {
	return &ServiceRegistryConfig{
		Type:            "consul",
		Address:         "localhost:8500",
		ServiceName:     "nats",
		RefreshInterval: 30 * time.Second,
		Timeout:         5 * time.Second,
		DefaultScheme:   SchemeNATS,
		UseTLS:          false,
	}
}

// Validate validates the service registry configuration
func (c *ServiceRegistryConfig) Validate() error {
	if c.Type == "" {
		return errors.New("registry type required")
	}
	if c.Type != "consul" && c.Type != "etcd" {
		return fmt.Errorf("unsupported registry type: %s", c.Type)
	}
	if c.Address == "" {
		return errors.New("registry address required")
	}
	if c.ServiceName == "" && c.Prefix == "" {
		return errors.New("service name or prefix required")
	}
	return nil
}

// ServiceRegistryDiscoverer discovers NATS endpoints via service registry
type ServiceRegistryDiscoverer struct {
	config *ServiceRegistryConfig

	// State
	mu        sync.RWMutex
	endpoints []*DiscoveredEndpoint

	// Lifecycle
	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewServiceRegistryDiscoverer creates a new service registry discoverer
func NewServiceRegistryDiscoverer(config *ServiceRegistryConfig) (*ServiceRegistryDiscoverer, error) {
	if config == nil {
		config = DefaultServiceRegistryConfig()
	}
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &ServiceRegistryDiscoverer{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}, nil
}

func (d *ServiceRegistryDiscoverer) Method() DiscoveryMethod {
	switch d.config.Type {
	case "consul":
		return DiscoveryMethodConsul
	case "etcd":
		return DiscoveryMethodEtcd
	default:
		return DiscoveryMethod(d.config.Type)
	}
}

func (d *ServiceRegistryDiscoverer) Discover(ctx context.Context) ([]*DiscoveredEndpoint, error) {
	// Note: Full implementation requires consul-api or etcd client libraries
	// This is a placeholder that demonstrates the interface

	switch d.config.Type {
	case "consul":
		return d.discoverConsul(ctx)
	case "etcd":
		return d.discoverEtcd(ctx)
	default:
		return nil, fmt.Errorf("unsupported registry type: %s", d.config.Type)
	}
}

func (d *ServiceRegistryDiscoverer) discoverConsul(ctx context.Context) ([]*DiscoveredEndpoint, error) {
	// Placeholder for Consul discovery
	// Full implementation would use github.com/hashicorp/consul/api
	return []*DiscoveredEndpoint{}, nil
}

func (d *ServiceRegistryDiscoverer) discoverEtcd(ctx context.Context) ([]*DiscoveredEndpoint, error) {
	// Placeholder for etcd discovery
	// Full implementation would use go.etcd.io/etcd/client/v3
	return []*DiscoveredEndpoint{}, nil
}

func (d *ServiceRegistryDiscoverer) Watch(ctx context.Context, callback func([]*DiscoveredEndpoint)) error {
	if d.config.RefreshInterval <= 0 {
		return errors.New("refresh interval must be positive for watching")
	}

	d.wg.Add(1)
	go func() {
		defer d.wg.Done()

		ticker := time.NewTicker(d.config.RefreshInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-d.ctx.Done():
				return
			case <-ticker.C:
				endpoints, err := d.Discover(ctx)
				if err == nil && callback != nil {
					callback(endpoints)
				}
			}
		}
	}()

	return nil
}

func (d *ServiceRegistryDiscoverer) Close() error {
	d.cancel()
	d.wg.Wait()
	return nil
}

// ============================================================================
// Static Discovery
// ============================================================================

// StaticDiscoveryConfig holds static discovery configuration
type StaticDiscoveryConfig struct {
	// URLs are the static NATS URLs
	URLs []string

	// DefaultScheme is used when URL doesn't have a scheme
	DefaultScheme Scheme
}

// StaticDiscoverer returns statically configured endpoints
type StaticDiscoverer struct {
	config    *StaticDiscoveryConfig
	endpoints []*DiscoveredEndpoint
}

// NewStaticDiscoverer creates a new static discoverer
func NewStaticDiscoverer(urls []string) *StaticDiscoverer {
	endpoints := make([]*DiscoveredEndpoint, 0, len(urls))

	for i, rawURL := range urls {
		ep, err := ParseEndpoint(rawURL)
		if err != nil {
			continue
		}

		endpoints = append(endpoints, &DiscoveredEndpoint{
			URL:          ep.URL,
			Host:         ep.Host,
			Port:         ep.Port,
			Priority:     i,
			Weight:       1,
			TLS:          ep.IsTLS(),
			Scheme:       ep.Scheme,
			Method:       DiscoveryMethodStatic,
			TTL:          0, // Never expires
			DiscoveredAt: time.Now(),
		})
	}

	return &StaticDiscoverer{
		config: &StaticDiscoveryConfig{
			URLs:          urls,
			DefaultScheme: SchemeNATS,
		},
		endpoints: endpoints,
	}
}

func (d *StaticDiscoverer) Method() DiscoveryMethod {
	return DiscoveryMethodStatic
}

func (d *StaticDiscoverer) Discover(ctx context.Context) ([]*DiscoveredEndpoint, error) {
	return d.endpoints, nil
}

func (d *StaticDiscoverer) Watch(ctx context.Context, callback func([]*DiscoveredEndpoint)) error {
	// Static endpoints don't change
	return nil
}

func (d *StaticDiscoverer) Close() error {
	return nil
}

// ============================================================================
// Discovery Manager - Aggregates Multiple Discoverers
// ============================================================================

// DiscoveryManagerConfig holds discovery manager configuration
type DiscoveryManagerConfig struct {
	// RefreshInterval is how often to refresh discovery
	RefreshInterval time.Duration

	// CacheExpiry is how long to cache endpoints
	CacheExpiry time.Duration

	// PreferMethods orders discovery methods by preference
	PreferMethods []DiscoveryMethod

	// HealthCheckInterval is how often to health check endpoints
	HealthCheckInterval time.Duration

	// HealthCheckTimeout is the timeout for health checks
	HealthCheckTimeout time.Duration
}

// DefaultDiscoveryManagerConfig returns default discovery manager configuration
func DefaultDiscoveryManagerConfig() *DiscoveryManagerConfig {
	return &DiscoveryManagerConfig{
		RefreshInterval: 30 * time.Second,
		CacheExpiry:     5 * time.Minute,
		PreferMethods: []DiscoveryMethod{
			DiscoveryMethodStatic,
			DiscoveryMethodDNS,
			DiscoveryMethodKubernetes,
			DiscoveryMethodConsul,
			DiscoveryMethodMDNS,
		},
		HealthCheckInterval: 10 * time.Second,
		HealthCheckTimeout:  5 * time.Second,
	}
}

// DiscoveryManager aggregates multiple discoverers
type DiscoveryManager struct {
	config *DiscoveryManagerConfig

	// Discoverers
	discoverers []Discoverer
	discMu      sync.RWMutex

	// Endpoints cache
	endpoints   []*DiscoveredEndpoint
	endpointsMu sync.RWMutex
	lastRefresh time.Time

	// Health tracking
	healthyEndpoints map[string]bool
	healthMu         sync.RWMutex

	// Callbacks
	onEndpointsChanged func([]*DiscoveredEndpoint)

	// Lifecycle
	running atomic.Bool
	ctx     context.Context
	cancel  context.CancelFunc
	wg      sync.WaitGroup
}

// NewDiscoveryManager creates a new discovery manager
func NewDiscoveryManager(config *DiscoveryManagerConfig) *DiscoveryManager {
	if config == nil {
		config = DefaultDiscoveryManagerConfig()
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &DiscoveryManager{
		config:           config,
		discoverers:      make([]Discoverer, 0),
		endpoints:        make([]*DiscoveredEndpoint, 0),
		healthyEndpoints: make(map[string]bool),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// AddDiscoverer adds a discoverer
func (m *DiscoveryManager) AddDiscoverer(d Discoverer) {
	m.discMu.Lock()
	defer m.discMu.Unlock()
	m.discoverers = append(m.discoverers, d)
}

// RemoveDiscoverer removes a discoverer by method
func (m *DiscoveryManager) RemoveDiscoverer(method DiscoveryMethod) {
	m.discMu.Lock()
	defer m.discMu.Unlock()

	for i, d := range m.discoverers {
		if d.Method() == method {
			d.Close()
			m.discoverers = append(m.discoverers[:i], m.discoverers[i+1:]...)
			return
		}
	}
}

// SetEndpointsChangedCallback sets the callback for endpoint changes
func (m *DiscoveryManager) SetEndpointsChangedCallback(cb func([]*DiscoveredEndpoint)) {
	m.onEndpointsChanged = cb
}

// Start starts the discovery manager
func (m *DiscoveryManager) Start() error {
	if m.running.Load() {
		return errors.New("already running")
	}

	m.running.Store(true)

	// Initial discovery
	if err := m.refresh(); err != nil {
		// Log but don't fail - we may get endpoints later
	}

	// Start refresh loop
	m.wg.Add(1)
	go m.refreshLoop()

	// Start health check loop if configured
	if m.config.HealthCheckInterval > 0 {
		m.wg.Add(1)
		go m.healthCheckLoop()
	}

	return nil
}

// Stop stops the discovery manager
func (m *DiscoveryManager) Stop() error {
	if !m.running.Load() {
		return nil
	}

	m.cancel()
	m.wg.Wait()

	// Close all discoverers
	m.discMu.Lock()
	for _, d := range m.discoverers {
		d.Close()
	}
	m.discMu.Unlock()

	m.running.Store(false)
	return nil
}

func (m *DiscoveryManager) refreshLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.RefreshInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.refresh()
		}
	}
}

func (m *DiscoveryManager) refresh() error {
	m.discMu.RLock()
	discoverers := make([]Discoverer, len(m.discoverers))
	copy(discoverers, m.discoverers)
	m.discMu.RUnlock()

	var allEndpoints []*DiscoveredEndpoint

	for _, d := range discoverers {
		ctx, cancel := context.WithTimeout(m.ctx, 10*time.Second)
		endpoints, err := d.Discover(ctx)
		cancel()

		if err != nil {
			continue
		}

		allEndpoints = append(allEndpoints, endpoints...)
	}

	// Sort by method preference, then priority
	m.sortEndpoints(allEndpoints)

	// Check if endpoints changed
	changed := m.endpointsChanged(allEndpoints)

	m.endpointsMu.Lock()
	m.endpoints = allEndpoints
	m.lastRefresh = time.Now()
	m.endpointsMu.Unlock()

	if changed && m.onEndpointsChanged != nil {
		m.onEndpointsChanged(allEndpoints)
	}

	return nil
}

func (m *DiscoveryManager) sortEndpoints(endpoints []*DiscoveredEndpoint) {
	methodPriority := make(map[DiscoveryMethod]int)
	for i, method := range m.config.PreferMethods {
		methodPriority[method] = i
	}

	sort.Slice(endpoints, func(i, j int) bool {
		// First by method preference
		pi := methodPriority[endpoints[i].Method]
		pj := methodPriority[endpoints[j].Method]
		if pi != pj {
			return pi < pj
		}
		// Then by priority
		if endpoints[i].Priority != endpoints[j].Priority {
			return endpoints[i].Priority < endpoints[j].Priority
		}
		// Then by weight (descending)
		return endpoints[i].Weight > endpoints[j].Weight
	})
}

func (m *DiscoveryManager) endpointsChanged(newEndpoints []*DiscoveredEndpoint) bool {
	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()

	if len(m.endpoints) != len(newEndpoints) {
		return true
	}

	oldURLs := make(map[string]bool)
	for _, ep := range m.endpoints {
		oldURLs[ep.URL] = true
	}

	for _, ep := range newEndpoints {
		if !oldURLs[ep.URL] {
			return true
		}
	}

	return false
}

func (m *DiscoveryManager) healthCheckLoop() {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.checkHealth()
		}
	}
}

func (m *DiscoveryManager) checkHealth() {
	m.endpointsMu.RLock()
	endpoints := make([]*DiscoveredEndpoint, len(m.endpoints))
	copy(endpoints, m.endpoints)
	m.endpointsMu.RUnlock()

	for _, ep := range endpoints {
		healthy := m.checkEndpointHealth(ep)
		m.healthMu.Lock()
		m.healthyEndpoints[ep.URL] = healthy
		m.healthMu.Unlock()
	}
}

func (m *DiscoveryManager) checkEndpointHealth(ep *DiscoveredEndpoint) bool {
	// Simple TCP connectivity check
	addr := fmt.Sprintf("%s:%d", ep.Host, ep.Port)
	conn, err := net.DialTimeout("tcp", addr, m.config.HealthCheckTimeout)
	if err != nil {
		return false
	}
	conn.Close()
	return true
}

// GetEndpoints returns all discovered endpoints
func (m *DiscoveryManager) GetEndpoints() []*DiscoveredEndpoint {
	m.endpointsMu.RLock()
	defer m.endpointsMu.RUnlock()

	endpoints := make([]*DiscoveredEndpoint, len(m.endpoints))
	copy(endpoints, m.endpoints)
	return endpoints
}

// GetHealthyEndpoints returns only healthy endpoints
func (m *DiscoveryManager) GetHealthyEndpoints() []*DiscoveredEndpoint {
	m.endpointsMu.RLock()
	endpoints := make([]*DiscoveredEndpoint, 0, len(m.endpoints))
	copy(endpoints, m.endpoints)
	m.endpointsMu.RUnlock()

	m.healthMu.RLock()
	defer m.healthMu.RUnlock()

	healthy := make([]*DiscoveredEndpoint, 0)
	for _, ep := range endpoints {
		if m.healthyEndpoints[ep.URL] {
			healthy = append(healthy, ep)
		}
	}
	return healthy
}

// GetBestEndpoint returns the best available endpoint
func (m *DiscoveryManager) GetBestEndpoint() *DiscoveredEndpoint {
	healthy := m.GetHealthyEndpoints()
	if len(healthy) > 0 {
		return healthy[0]
	}

	// Fall back to any endpoint
	endpoints := m.GetEndpoints()
	if len(endpoints) > 0 {
		return endpoints[0]
	}

	return nil
}

// IsEndpointHealthy returns true if the endpoint is healthy
func (m *DiscoveryManager) IsEndpointHealthy(url string) bool {
	m.healthMu.RLock()
	defer m.healthMu.RUnlock()
	return m.healthyEndpoints[url]
}

// ============================================================================
// Auto-Configuration - T7.5
// ============================================================================

// AutoConfigMode represents the auto-configuration mode
type AutoConfigMode string

const (
	// AutoConfigModeAuto automatically selects the best configuration
	AutoConfigModeAuto AutoConfigMode = "auto"
	// AutoConfigModeManual uses manual configuration only
	AutoConfigModeManual AutoConfigMode = "manual"
	// AutoConfigModeHybrid combines auto and manual configuration
	AutoConfigModeHybrid AutoConfigMode = "hybrid"
)

// NetworkType represents detected network type
type NetworkType string

const (
	// NetworkTypeDirect indicates direct connectivity
	NetworkTypeDirect NetworkType = "direct"
	// NetworkTypeNAT indicates NAT network
	NetworkTypeNAT NetworkType = "nat"
	// NetworkTypeSymmetricNAT indicates symmetric NAT
	NetworkTypeSymmetricNAT NetworkType = "symmetric_nat"
	// NetworkTypeFirewall indicates firewall restrictions
	NetworkTypeFirewall NetworkType = "firewall"
	// NetworkTypeUnknown indicates unknown network type
	NetworkTypeUnknown NetworkType = "unknown"
)

// AutoConfigResult represents auto-configuration results
type AutoConfigResult struct {
	// Mode is the selected configuration mode
	Mode AutoConfigMode

	// NetworkType is the detected network type
	NetworkType NetworkType

	// RecommendedStrategy is the recommended connection strategy
	RecommendedStrategy string

	// Endpoints are the selected endpoints
	Endpoints []*DiscoveredEndpoint

	// FallbackChain is the fallback strategy chain
	FallbackChain []string

	// Warnings are any configuration warnings
	Warnings []string

	// DiscoveredAt is when this configuration was determined
	DiscoveredAt time.Time
}

// AutoConfigurator automatically configures NATS connections
type AutoConfigurator struct {
	// Discovery manager
	discovery *DiscoveryManager

	// Static configuration (takes precedence)
	staticURLs []string

	// Configuration cache
	mu         sync.RWMutex
	lastConfig *AutoConfigResult
	lastCheck  time.Time

	// Settings
	cacheExpiry     time.Duration
	connectTimeout  time.Duration
	preferWebSocket bool
}

// AutoConfiguratorOptions holds auto-configurator options
type AutoConfiguratorOptions struct {
	// StaticURLs are manually configured URLs (highest priority)
	StaticURLs []string

	// CacheExpiry is how long to cache configuration
	CacheExpiry time.Duration

	// ConnectTimeout is the timeout for connectivity tests
	ConnectTimeout time.Duration

	// PreferWebSocket prefers WebSocket connections
	PreferWebSocket bool

	// Discovery configuration
	DNSConfig        *DNSDiscoveryConfig
	KubernetesConfig *KubernetesDiscoveryConfig
	ConsulConfig     *ServiceRegistryConfig
}

// NewAutoConfigurator creates a new auto-configurator
func NewAutoConfigurator(opts *AutoConfiguratorOptions) *AutoConfigurator {
	if opts == nil {
		opts = &AutoConfiguratorOptions{}
	}

	ac := &AutoConfigurator{
		staticURLs:     opts.StaticURLs,
		cacheExpiry:    opts.CacheExpiry,
		connectTimeout: opts.ConnectTimeout,
		preferWebSocket: opts.PreferWebSocket,
	}

	if ac.cacheExpiry == 0 {
		ac.cacheExpiry = 5 * time.Minute
	}
	if ac.connectTimeout == 0 {
		ac.connectTimeout = 5 * time.Second
	}

	// Create discovery manager
	ac.discovery = NewDiscoveryManager(DefaultDiscoveryManagerConfig())

	// Add static discoverer if URLs provided
	if len(opts.StaticURLs) > 0 {
		ac.discovery.AddDiscoverer(NewStaticDiscoverer(opts.StaticURLs))
	}

	// Add DNS discoverer if configured
	if opts.DNSConfig != nil {
		if d, err := NewDNSDiscoverer(opts.DNSConfig); err == nil {
			ac.discovery.AddDiscoverer(d)
		}
	}

	// Add Kubernetes discoverer if configured
	if opts.KubernetesConfig != nil {
		if d, err := NewKubernetesDiscoverer(opts.KubernetesConfig); err == nil {
			ac.discovery.AddDiscoverer(d)
		}
	}

	// Add Consul discoverer if configured
	if opts.ConsulConfig != nil {
		if d, err := NewServiceRegistryDiscoverer(opts.ConsulConfig); err == nil {
			ac.discovery.AddDiscoverer(d)
		}
	}

	return ac
}

// Start starts the auto-configurator
func (ac *AutoConfigurator) Start() error {
	return ac.discovery.Start()
}

// Stop stops the auto-configurator
func (ac *AutoConfigurator) Stop() error {
	return ac.discovery.Stop()
}

// Configure performs auto-configuration
func (ac *AutoConfigurator) Configure(ctx context.Context) (*AutoConfigResult, error) {
	// Check cache
	ac.mu.RLock()
	if ac.lastConfig != nil && time.Since(ac.lastCheck) < ac.cacheExpiry {
		result := ac.lastConfig
		ac.mu.RUnlock()
		return result, nil
	}
	ac.mu.RUnlock()

	// Perform discovery
	endpoints := ac.discovery.GetEndpoints()
	if len(endpoints) == 0 {
		return nil, errors.New("no endpoints discovered")
	}

	// Detect network type
	networkType := ac.detectNetworkType(ctx)

	// Select strategy based on network type
	strategy, fallback := ac.selectStrategy(networkType)

	// Build result
	result := &AutoConfigResult{
		Mode:                AutoConfigModeAuto,
		NetworkType:         networkType,
		RecommendedStrategy: strategy,
		Endpoints:           endpoints,
		FallbackChain:       fallback,
		Warnings:            make([]string, 0),
		DiscoveredAt:        time.Now(),
	}

	// Add warnings
	if len(ac.staticURLs) > 0 {
		result.Mode = AutoConfigModeHybrid
	}

	if networkType == NetworkTypeSymmetricNAT {
		result.Warnings = append(result.Warnings, "symmetric NAT detected - WebSocket recommended")
	}

	if networkType == NetworkTypeFirewall {
		result.Warnings = append(result.Warnings, "firewall detected - WebSocket on port 443 recommended")
	}

	// Cache result
	ac.mu.Lock()
	ac.lastConfig = result
	ac.lastCheck = time.Now()
	ac.mu.Unlock()

	return result, nil
}

func (ac *AutoConfigurator) detectNetworkType(ctx context.Context) NetworkType {
	endpoints := ac.discovery.GetEndpoints()
	if len(endpoints) == 0 {
		return NetworkTypeUnknown
	}

	// Try direct TCP connection
	for _, ep := range endpoints {
		if ep.Scheme == SchemeNATS || ep.Scheme == SchemeTLS {
			addr := fmt.Sprintf("%s:%d", ep.Host, ep.Port)
			conn, err := net.DialTimeout("tcp", addr, ac.connectTimeout)
			if err == nil {
				conn.Close()
				return NetworkTypeDirect
			}
		}
	}

	// Try WebSocket connection
	for _, ep := range endpoints {
		if ep.Scheme == SchemeWS || ep.Scheme == SchemeWSS {
			addr := fmt.Sprintf("%s:%d", ep.Host, ep.Port)
			conn, err := net.DialTimeout("tcp", addr, ac.connectTimeout)
			if err == nil {
				conn.Close()
				return NetworkTypeFirewall
			}
		}
	}

	return NetworkTypeNAT
}

func (ac *AutoConfigurator) selectStrategy(networkType NetworkType) (string, []string) {
	switch networkType {
	case NetworkTypeDirect:
		if ac.preferWebSocket {
			return "websocket", []string{"tls", "direct"}
		}
		return "tls", []string{"direct", "websocket"}

	case NetworkTypeNAT:
		return "direct", []string{"websocket", "leafnode"}

	case NetworkTypeSymmetricNAT:
		return "websocket", []string{"leafnode"}

	case NetworkTypeFirewall:
		return "websocket", []string{}

	default:
		return "tls", []string{"direct", "websocket", "leafnode"}
	}
}

// GetEndpoints returns discovered endpoints
func (ac *AutoConfigurator) GetEndpoints() []*DiscoveredEndpoint {
	return ac.discovery.GetEndpoints()
}

// GetHealthyEndpoints returns healthy endpoints
func (ac *AutoConfigurator) GetHealthyEndpoints() []*DiscoveredEndpoint {
	return ac.discovery.GetHealthyEndpoints()
}

// GetBestEndpoint returns the best endpoint
func (ac *AutoConfigurator) GetBestEndpoint() *DiscoveredEndpoint {
	return ac.discovery.GetBestEndpoint()
}
