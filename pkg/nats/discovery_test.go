package nats

import (
	"context"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/pkg/testing/helpers"
)

// ============================================================================
// Discovery Types Tests
// ============================================================================

func TestDiscoveryMethod_String(t *testing.T) {
	tests := []struct {
		method   DiscoveryMethod
		expected string
	}{
		{DiscoveryMethodDNS, "dns"},
		{DiscoveryMethodMDNS, "mdns"},
		{DiscoveryMethodKubernetes, "kubernetes"},
		{DiscoveryMethodConsul, "consul"},
		{DiscoveryMethodEtcd, "etcd"},
		{DiscoveryMethodStatic, "static"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			if got := tt.method.String(); got != tt.expected {
				t.Errorf("String() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDiscoveredEndpoint_IsExpired(t *testing.T) {
	tests := []struct {
		name     string
		ttl      time.Duration
		age      time.Duration
		expected bool
	}{
		{
			name:     "no TTL never expires",
			ttl:      0,
			age:      time.Hour,
			expected: false,
		},
		{
			name:     "negative TTL never expires",
			ttl:      -1 * time.Second,
			age:      time.Hour,
			expected: false,
		},
		{
			name:     "not expired",
			ttl:      time.Minute,
			age:      30 * time.Second,
			expected: false,
		},
		{
			name:     "expired",
			ttl:      time.Minute,
			age:      2 * time.Minute,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ep := &DiscoveredEndpoint{
				TTL:          tt.ttl,
				DiscoveredAt: time.Now().Add(-tt.age),
			}
			if got := ep.IsExpired(); got != tt.expected {
				t.Errorf("IsExpired() = %v, want %v", got, tt.expected)
			}
		})
	}
}

func TestDiscoveredEndpoint_ToEndpoint(t *testing.T) {
	ep := &DiscoveredEndpoint{
		URL:      "nats://localhost:4222",
		Host:     "localhost",
		Port:     4222,
		Priority: 1,
		Scheme:   SchemeNATS,
	}

	result := ep.ToEndpoint()

	if result.URL != ep.URL {
		t.Errorf("URL = %v, want %v", result.URL, ep.URL)
	}
	if result.Host != ep.Host {
		t.Errorf("Host = %v, want %v", result.Host, ep.Host)
	}
	if result.Port != ep.Port {
		t.Errorf("Port = %v, want %v", result.Port, ep.Port)
	}
	if result.Priority != ep.Priority {
		t.Errorf("Priority = %v, want %v", result.Priority, ep.Priority)
	}
	if result.Scheme != ep.Scheme {
		t.Errorf("Scheme = %v, want %v", result.Scheme, ep.Scheme)
	}
}

// ============================================================================
// DNS Discovery Tests
// ============================================================================

func TestDefaultDNSDiscoveryConfig(t *testing.T) {
	cfg := DefaultDNSDiscoveryConfig()

	if cfg.ServiceName != "_nats._tcp" {
		t.Errorf("ServiceName = %v, want _nats._tcp", cfg.ServiceName)
	}
	if cfg.RefreshInterval != 30*time.Second {
		t.Errorf("RefreshInterval = %v, want 30s", cfg.RefreshInterval)
	}
	if cfg.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", cfg.Timeout)
	}
	if !cfg.FallbackToA {
		t.Error("FallbackToA should be true")
	}
	if cfg.DefaultPort != 4222 {
		t.Errorf("DefaultPort = %v, want 4222", cfg.DefaultPort)
	}
	if cfg.DefaultScheme != SchemeNATS {
		t.Errorf("DefaultScheme = %v, want nats", cfg.DefaultScheme)
	}
}

func TestDNSDiscoveryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *DNSDiscoveryConfig
		wantErr bool
	}{
		{
			name:    "valid with domain",
			config:  &DNSDiscoveryConfig{Domain: "example.com"},
			wantErr: false,
		},
		{
			name:    "valid with service name",
			config:  &DNSDiscoveryConfig{ServiceName: "_nats._tcp"},
			wantErr: false,
		},
		{
			name:    "missing domain and service name",
			config:  &DNSDiscoveryConfig{},
			wantErr: true,
		},
		{
			name:    "negative refresh interval",
			config:  &DNSDiscoveryConfig{Domain: "example.com", RefreshInterval: -1 * time.Second},
			wantErr: true,
		},
		{
			name:    "negative timeout",
			config:  &DNSDiscoveryConfig{Domain: "example.com", Timeout: -1 * time.Second},
			wantErr: true,
		},
		{
			name:    "invalid port negative",
			config:  &DNSDiscoveryConfig{Domain: "example.com", DefaultPort: -1},
			wantErr: true,
		},
		{
			name:    "invalid port too high",
			config:  &DNSDiscoveryConfig{Domain: "example.com", DefaultPort: 70000},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewDNSDiscoverer(t *testing.T) {
	// Test with valid config
	cfg := &DNSDiscoveryConfig{
		Domain:          "example.com",
		ServiceName:     "_nats._tcp",
		RefreshInterval: time.Minute,
		Timeout:         5 * time.Second,
		DefaultPort:     4222,
	}

	d, err := NewDNSDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewDNSDiscoverer() error = %v", err)
	}
	defer d.Close()

	if d.Method() != DiscoveryMethodDNS {
		t.Errorf("Method() = %v, want dns", d.Method())
	}

	// Test with invalid config (missing domain and service name)
	invalidCfg := &DNSDiscoveryConfig{}
	_, err = NewDNSDiscoverer(invalidCfg)
	if err == nil {
		t.Error("expected error with invalid config (missing domain and service name)")
	}
}

func TestDNSDiscoverer_GetCachedEndpoints(t *testing.T) {
	cfg := &DNSDiscoveryConfig{
		Domain:      "example.com",
		DefaultPort: 4222,
	}

	d, err := NewDNSDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewDNSDiscoverer() error = %v", err)
	}
	defer d.Close()

	// Initially should be empty
	endpoints := d.GetCachedEndpoints()
	if len(endpoints) != 0 {
		t.Errorf("GetCachedEndpoints() = %v, want empty", endpoints)
	}
}

func TestDNSDiscoverer_Close(t *testing.T) {
	cfg := &DNSDiscoveryConfig{
		Domain:      "example.com",
		DefaultPort: 4222,
	}

	d, err := NewDNSDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewDNSDiscoverer() error = %v", err)
	}

	// Close should not error
	if err := d.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// ============================================================================
// mDNS Discovery Tests
// ============================================================================

func TestDefaultMDNSDiscoveryConfig(t *testing.T) {
	cfg := DefaultMDNSDiscoveryConfig()

	if cfg.ServiceType != "_nats._tcp" {
		t.Errorf("ServiceType = %v, want _nats._tcp", cfg.ServiceType)
	}
	if cfg.Domain != "local." {
		t.Errorf("Domain = %v, want local.", cfg.Domain)
	}
	if cfg.BrowseTimeout != 3*time.Second {
		t.Errorf("BrowseTimeout = %v, want 3s", cfg.BrowseTimeout)
	}
	if cfg.RefreshInterval != 30*time.Second {
		t.Errorf("RefreshInterval = %v, want 30s", cfg.RefreshInterval)
	}
}

func TestMDNSDiscoveryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *MDNSDiscoveryConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  &MDNSDiscoveryConfig{ServiceType: "_nats._tcp"},
			wantErr: false,
		},
		{
			name:    "missing service type",
			config:  &MDNSDiscoveryConfig{},
			wantErr: true,
		},
		{
			name:    "negative browse timeout",
			config:  &MDNSDiscoveryConfig{ServiceType: "_nats._tcp", BrowseTimeout: -1 * time.Second},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewMDNSDiscoverer(t *testing.T) {
	// Test with nil config - should use defaults
	d, err := NewMDNSDiscoverer(nil)
	if err != nil {
		t.Fatalf("NewMDNSDiscoverer() error = %v", err)
	}
	defer d.Close()

	if d.Method() != DiscoveryMethodMDNS {
		t.Errorf("Method() = %v, want mdns", d.Method())
	}

	// Discover should complete without error.
	endpoints, err := d.Discover(context.Background())
	if err != nil {
		t.Errorf("Discover() error = %v", err)
	}
	if endpoints == nil {
		t.Error("Discover() returned nil endpoints")
	}
}

// ============================================================================
// Kubernetes Discovery Tests
// ============================================================================

func TestDefaultKubernetesDiscoveryConfig(t *testing.T) {
	cfg := DefaultKubernetesDiscoveryConfig()

	if cfg.ServiceName != "nats" {
		t.Errorf("ServiceName = %v, want nats", cfg.ServiceName)
	}
	if cfg.PortName != "nats" {
		t.Errorf("PortName = %v, want nats", cfg.PortName)
	}
	if !cfg.InCluster {
		t.Error("InCluster should be true")
	}
	if cfg.RefreshInterval != 30*time.Second {
		t.Errorf("RefreshInterval = %v, want 30s", cfg.RefreshInterval)
	}
}

func TestKubernetesDiscoveryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *KubernetesDiscoveryConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  &KubernetesDiscoveryConfig{ServiceName: "nats"},
			wantErr: false,
		},
		{
			name:    "missing service name",
			config:  &KubernetesDiscoveryConfig{},
			wantErr: true,
		},
		{
			name:    "negative refresh interval",
			config:  &KubernetesDiscoveryConfig{ServiceName: "nats", RefreshInterval: -1 * time.Second},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewKubernetesDiscoverer(t *testing.T) {
	// Skip if kubeconfig is not available (not running in Kubernetes or without kubeconfig)
	if os.Getenv("KUBECONFIG") == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			if _, err := os.Stat(home + "/.kube/config"); os.IsNotExist(err) {
				t.Skip("Skipping: no kubeconfig available")
			}
		}
	}

	cfg := &KubernetesDiscoveryConfig{
		ServiceName: "nats",
		Namespace:   "default",
	}

	d, err := NewKubernetesDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewKubernetesDiscoverer() error = %v", err)
	}
	defer d.Close()

	if d.Method() != DiscoveryMethodKubernetes {
		t.Errorf("Method() = %v, want kubernetes", d.Method())
	}
}

// ============================================================================
// Service Registry Discovery Tests
// ============================================================================

func TestDefaultServiceRegistryConfig(t *testing.T) {
	cfg := DefaultServiceRegistryConfig()

	if cfg.Type != "consul" {
		t.Errorf("Type = %v, want consul", cfg.Type)
	}
	if cfg.Address != "localhost:8500" {
		t.Errorf("Address = %v, want localhost:8500", cfg.Address)
	}
	if cfg.ServiceName != "nats" {
		t.Errorf("ServiceName = %v, want nats", cfg.ServiceName)
	}
}

func TestServiceRegistryConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		config  *ServiceRegistryConfig
		wantErr bool
	}{
		{
			name:    "valid consul config",
			config:  &ServiceRegistryConfig{Type: "consul", Address: "localhost:8500", ServiceName: "nats"},
			wantErr: false,
		},
		{
			name:    "valid etcd config",
			config:  &ServiceRegistryConfig{Type: "etcd", Address: "localhost:2379", Prefix: "/nats"},
			wantErr: false,
		},
		{
			name:    "missing type",
			config:  &ServiceRegistryConfig{Address: "localhost:8500", ServiceName: "nats"},
			wantErr: true,
		},
		{
			name:    "unsupported type",
			config:  &ServiceRegistryConfig{Type: "zookeeper", Address: "localhost:2181", ServiceName: "nats"},
			wantErr: true,
		},
		{
			name:    "missing address",
			config:  &ServiceRegistryConfig{Type: "consul", ServiceName: "nats"},
			wantErr: true,
		},
		{
			name:    "missing service name and prefix",
			config:  &ServiceRegistryConfig{Type: "consul", Address: "localhost:8500"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewServiceRegistryDiscoverer(t *testing.T) {
	cfg := &ServiceRegistryConfig{
		Type:        "consul",
		Address:     "localhost:8500",
		ServiceName: "nats",
	}

	d, err := NewServiceRegistryDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewServiceRegistryDiscoverer() error = %v", err)
	}
	defer d.Close()

	if d.Method() != DiscoveryMethodConsul {
		t.Errorf("Method() = %v, want consul", d.Method())
	}

	// Test etcd method
	cfgEtcd := &ServiceRegistryConfig{
		Type:    "etcd",
		Address: "localhost:2379",
		Prefix:  "/nats",
	}

	d2, err := NewServiceRegistryDiscoverer(cfgEtcd)
	if err != nil {
		t.Fatalf("NewServiceRegistryDiscoverer() error = %v", err)
	}
	defer d2.Close()

	if d2.Method() != DiscoveryMethodEtcd {
		t.Errorf("Method() = %v, want etcd", d2.Method())
	}
}

func TestServiceRegistryDiscoverer_Discover(t *testing.T) {
	// Skip if Consul is not running locally
	conn, err := net.DialTimeout("tcp", "localhost:8500", 100*time.Millisecond)
	if err != nil {
		t.Skip("Skipping: Consul not available at localhost:8500")
	}
	conn.Close()

	cfg := &ServiceRegistryConfig{
		Type:        "consul",
		Address:     "localhost:8500",
		ServiceName: "nats",
	}

	d, err := NewServiceRegistryDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewServiceRegistryDiscoverer() error = %v", err)
	}
	defer d.Close()

	// Discover may return empty if no NATS services are registered
	endpoints, err := d.Discover(context.Background())
	if err != nil {
		t.Errorf("Discover() error = %v", err)
	}
	// Just verify we got a valid response (empty is OK)
	_ = endpoints
}

// ============================================================================
// Static Discovery Tests
// ============================================================================

func TestNewStaticDiscoverer(t *testing.T) {
	urls := []string{
		"nats://server1:4222",
		"nats://server2:4222",
		"tls://server3:4222",
	}

	d := NewStaticDiscoverer(urls)

	if d.Method() != DiscoveryMethodStatic {
		t.Errorf("Method() = %v, want static", d.Method())
	}

	endpoints, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if len(endpoints) != 3 {
		t.Errorf("Discover() returned %d endpoints, want 3", len(endpoints))
	}

	// Check first endpoint
	if endpoints[0].Host != "server1" {
		t.Errorf("endpoints[0].Host = %v, want server1", endpoints[0].Host)
	}
	if endpoints[0].Port != 4222 {
		t.Errorf("endpoints[0].Port = %v, want 4222", endpoints[0].Port)
	}
	if endpoints[0].Scheme != SchemeNATS {
		t.Errorf("endpoints[0].Scheme = %v, want nats", endpoints[0].Scheme)
	}

	// Check TLS endpoint
	if endpoints[2].Scheme != SchemeTLS {
		t.Errorf("endpoints[2].Scheme = %v, want tls", endpoints[2].Scheme)
	}
	if !endpoints[2].TLS {
		t.Error("endpoints[2].TLS should be true")
	}
}

func TestStaticDiscoverer_InvalidURLs(t *testing.T) {
	urls := []string{
		"nats://valid:4222",
		"://invalid",
		"nats://also-valid:4222",
	}

	d := NewStaticDiscoverer(urls)

	endpoints, err := d.Discover(context.Background())
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	// Should have 2 valid endpoints
	if len(endpoints) != 2 {
		t.Errorf("Discover() returned %d endpoints, want 2", len(endpoints))
	}
}

func TestStaticDiscoverer_Watch(t *testing.T) {
	d := NewStaticDiscoverer([]string{"nats://server:4222"})

	// Watch should return nil (static endpoints don't change)
	err := d.Watch(context.Background(), func(endpoints []*DiscoveredEndpoint) {
		t.Error("callback should not be called")
	})
	if err != nil {
		t.Errorf("Watch() error = %v", err)
	}
}

func TestStaticDiscoverer_Close(t *testing.T) {
	d := NewStaticDiscoverer([]string{"nats://server:4222"})

	if err := d.Close(); err != nil {
		t.Errorf("Close() error = %v", err)
	}
}

// ============================================================================
// Discovery Manager Tests
// ============================================================================

func TestDefaultDiscoveryManagerConfig(t *testing.T) {
	cfg := DefaultDiscoveryManagerConfig()

	if cfg.RefreshInterval != 30*time.Second {
		t.Errorf("RefreshInterval = %v, want 30s", cfg.RefreshInterval)
	}
	if cfg.CacheExpiry != 5*time.Minute {
		t.Errorf("CacheExpiry = %v, want 5m", cfg.CacheExpiry)
	}
	if cfg.HealthCheckInterval != 10*time.Second {
		t.Errorf("HealthCheckInterval = %v, want 10s", cfg.HealthCheckInterval)
	}
	if len(cfg.PreferMethods) == 0 {
		t.Error("PreferMethods should not be empty")
	}
}

func TestNewDiscoveryManager(t *testing.T) {
	m := NewDiscoveryManager(nil)

	if m == nil {
		t.Fatal("NewDiscoveryManager() = nil")
	}

	// Check initial state
	endpoints := m.GetEndpoints()
	if len(endpoints) != 0 {
		t.Errorf("GetEndpoints() = %v, want empty", endpoints)
	}
}

func TestDiscoveryManager_AddRemoveDiscoverer(t *testing.T) {
	m := NewDiscoveryManager(nil)

	// Add a static discoverer
	d := NewStaticDiscoverer([]string{"nats://server:4222"})
	m.AddDiscoverer(d)

	// Remove it
	m.RemoveDiscoverer(DiscoveryMethodStatic)

	// Should be empty now (nothing to discover)
	endpoints := m.GetEndpoints()
	if len(endpoints) != 0 {
		t.Errorf("GetEndpoints() after remove = %v, want empty", endpoints)
	}
}

func TestDiscoveryManager_StartStop(t *testing.T) {
	cfg := &DiscoveryManagerConfig{
		RefreshInterval:     100 * time.Millisecond,
		CacheExpiry:         time.Minute,
		HealthCheckInterval: 0, // Disable health checks
	}
	m := NewDiscoveryManager(cfg)

	// Add a static discoverer
	m.AddDiscoverer(NewStaticDiscoverer([]string{"nats://server:4222"}))

	// Start
	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	// Start again should error
	if err := m.Start(); err == nil {
		t.Error("Start() should error when already running")
	}

	// Wait for initial discovery
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return len(m.GetEndpoints()) == 1, nil
	}); err != nil {
		t.Fatalf("expected initial discovery: %v", err)
	}

	// Should have endpoints
	endpoints := m.GetEndpoints()
	if len(endpoints) != 1 {
		t.Errorf("GetEndpoints() = %d, want 1", len(endpoints))
	}

	// Stop
	if err := m.Stop(); err != nil {
		t.Errorf("Stop() error = %v", err)
	}

	// Stop again should not error
	if err := m.Stop(); err != nil {
		t.Errorf("Stop() again error = %v", err)
	}
}

func TestDiscoveryManager_EndpointsChangedCallback(t *testing.T) {
	cfg := &DiscoveryManagerConfig{
		RefreshInterval:     50 * time.Millisecond,
		HealthCheckInterval: 0,
	}
	m := NewDiscoveryManager(cfg)

	var callCount int32
	var mu sync.Mutex
	var lastEndpoints []*DiscoveredEndpoint

	m.SetEndpointsChangedCallback(func(endpoints []*DiscoveredEndpoint) {
		atomic.AddInt32(&callCount, 1)
		mu.Lock()
		lastEndpoints = endpoints
		mu.Unlock()
	})

	m.AddDiscoverer(NewStaticDiscoverer([]string{"nats://server:4222"}))

	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer m.Stop()

	// Wait for callback
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return atomic.LoadInt32(&callCount) > 0, nil
	}); err != nil {
		t.Fatalf("callback should have been called: %v", err)
	}

	if atomic.LoadInt32(&callCount) == 0 {
		t.Error("callback should have been called")
	}

	mu.Lock()
	if len(lastEndpoints) != 1 {
		t.Errorf("lastEndpoints = %d, want 1", len(lastEndpoints))
	}
	mu.Unlock()
}

func TestDiscoveryManager_GetBestEndpoint(t *testing.T) {
	m := NewDiscoveryManager(nil)

	// No endpoints
	best := m.GetBestEndpoint()
	if best != nil {
		t.Errorf("GetBestEndpoint() = %v, want nil", best)
	}

	// Add static discoverer and start
	m.AddDiscoverer(NewStaticDiscoverer([]string{
		"nats://server1:4222",
		"nats://server2:4222",
	}))

	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer m.Stop()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		best = m.GetBestEndpoint()
		return best != nil, nil
	}); err != nil {
		t.Fatalf("GetBestEndpoint() = nil, want endpoint: %v", err)
	}
	if best == nil {
		t.Fatal("GetBestEndpoint() = nil, want endpoint")
	}

	if best.Host != "server1" {
		t.Errorf("GetBestEndpoint().Host = %v, want server1", best.Host)
	}
}

func TestDiscoveryManager_HealthCheck(t *testing.T) {
	// Start a test server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start test server: %v", err)
	}
	defer listener.Close()

	addr := listener.Addr().String()
	host, portStr, _ := net.SplitHostPort(addr)
	url := "nats://" + addr

	cfg := &DiscoveryManagerConfig{
		RefreshInterval:     50 * time.Millisecond,
		HealthCheckInterval: 50 * time.Millisecond,
		HealthCheckTimeout:  time.Second,
	}
	m := NewDiscoveryManager(cfg)

	m.AddDiscoverer(NewStaticDiscoverer([]string{
		url,
		"nats://invalid-host-that-does-not-exist:4222",
	}))

	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer m.Stop()

	// Wait for health checks to complete
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return m.IsEndpointHealthy(url), nil
	}); err != nil {
		t.Fatalf("health checks did not complete: %v", err)
	}

	// The test server endpoint should be healthy
	if !m.IsEndpointHealthy(url) {
		t.Errorf("IsEndpointHealthy(%s) = false, want true", url)
	}

	// The invalid endpoint should not be healthy
	invalidURL := "nats://invalid-host-that-does-not-exist:4222"
	if m.IsEndpointHealthy(invalidURL) {
		t.Errorf("IsEndpointHealthy(%s) = true, want false", invalidURL)
	}

	// Check total endpoints
	endpoints := m.GetEndpoints()
	if len(endpoints) != 2 {
		t.Errorf("GetEndpoints() = %d, want 2", len(endpoints))
	}

	_ = portStr // Used to construct the URL
	_ = host    // Verified via IsEndpointHealthy
}

// ============================================================================
// Auto-Configurator Tests
// ============================================================================

func TestNewAutoConfigurator(t *testing.T) {
	ac := NewAutoConfigurator(nil)

	if ac == nil {
		t.Fatal("NewAutoConfigurator() = nil")
	}

	// Check defaults
	if ac.cacheExpiry != 5*time.Minute {
		t.Errorf("cacheExpiry = %v, want 5m", ac.cacheExpiry)
	}
	if ac.connectTimeout != 5*time.Second {
		t.Errorf("connectTimeout = %v, want 5s", ac.connectTimeout)
	}
}

func TestAutoConfigurator_WithStaticURLs(t *testing.T) {
	opts := &AutoConfiguratorOptions{
		StaticURLs: []string{"nats://server:4222"},
	}

	ac := NewAutoConfigurator(opts)

	if err := ac.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ac.Stop()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return len(ac.GetEndpoints()) == 1, nil
	}); err != nil {
		t.Fatalf("expected endpoints: %v", err)
	}

	endpoints := ac.GetEndpoints()
	if len(endpoints) != 1 {
		t.Errorf("GetEndpoints() = %d, want 1", len(endpoints))
	}
}

func TestAutoConfigurator_Configure(t *testing.T) {
	opts := &AutoConfiguratorOptions{
		StaticURLs:  []string{"nats://server:4222"},
		CacheExpiry: 100 * time.Millisecond,
	}

	ac := NewAutoConfigurator(opts)

	if err := ac.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ac.Stop()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return len(ac.GetEndpoints()) == 1, nil
	}); err != nil {
		t.Fatalf("expected endpoints: %v", err)
	}

	ctx := context.Background()
	result, err := ac.Configure(ctx)
	if err != nil {
		t.Fatalf("Configure() error = %v", err)
	}

	if result == nil {
		t.Fatal("Configure() result = nil")
	}

	if result.Mode != AutoConfigModeHybrid {
		t.Errorf("Mode = %v, want hybrid (has static URLs)", result.Mode)
	}

	if len(result.Endpoints) != 1 {
		t.Errorf("Endpoints = %d, want 1", len(result.Endpoints))
	}

	// Cached result should be returned quickly
	result2, err := ac.Configure(ctx)
	if err != nil {
		t.Fatalf("Configure() cached error = %v", err)
	}

	if result2.DiscoveredAt != result.DiscoveredAt {
		t.Error("expected cached result")
	}

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		result3, err := ac.Configure(ctx)
		if err != nil {
			return false, nil
		}
		return result3.DiscoveredAt != result.DiscoveredAt, nil
	}); err != nil {
		t.Fatalf("expected fresh result after cache expiry: %v", err)
	}
}

func TestAutoConfigurator_ConfigureNoEndpoints(t *testing.T) {
	ac := NewAutoConfigurator(nil)

	if err := ac.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ac.Stop()

	ctx := context.Background()
	_, err := ac.Configure(ctx)
	if err == nil {
		t.Error("Configure() should error with no endpoints")
	}
}

func TestAutoConfigurator_SelectStrategy(t *testing.T) {
	ac := NewAutoConfigurator(nil)

	tests := []struct {
		networkType     NetworkType
		preferWebSocket bool
		wantStrategy    string
	}{
		{NetworkTypeDirect, false, "tls"},
		{NetworkTypeDirect, true, "websocket"},
		{NetworkTypeNAT, false, "direct"},
		{NetworkTypeSymmetricNAT, false, "websocket"},
		{NetworkTypeFirewall, false, "websocket"},
		{NetworkTypeUnknown, false, "tls"},
	}

	for _, tt := range tests {
		t.Run(string(tt.networkType), func(t *testing.T) {
			ac.preferWebSocket = tt.preferWebSocket
			strategy, _ := ac.selectStrategy(tt.networkType)
			if strategy != tt.wantStrategy {
				t.Errorf("selectStrategy(%v) = %v, want %v", tt.networkType, strategy, tt.wantStrategy)
			}
		})
	}
}

func TestAutoConfigurator_GetBestEndpoint(t *testing.T) {
	opts := &AutoConfiguratorOptions{
		StaticURLs: []string{"nats://server:4222"},
	}

	ac := NewAutoConfigurator(opts)

	if err := ac.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer ac.Stop()

	var best *DiscoveredEndpoint
	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		best = ac.GetBestEndpoint()
		return best != nil, nil
	}); err != nil {
		t.Fatalf("GetBestEndpoint() = nil, want endpoint: %v", err)
	}
	if best == nil {
		t.Fatal("GetBestEndpoint() = nil, want endpoint")
	}

	if best.Host != "server" {
		t.Errorf("GetBestEndpoint().Host = %v, want server", best.Host)
	}
}

// ============================================================================
// Integration Tests
// ============================================================================

func TestDiscoveryManager_MultipleDiscoverers(t *testing.T) {
	cfg := &DiscoveryManagerConfig{
		RefreshInterval:     50 * time.Millisecond,
		HealthCheckInterval: 0,
		PreferMethods: []DiscoveryMethod{
			DiscoveryMethodStatic,
			DiscoveryMethodDNS,
		},
	}
	m := NewDiscoveryManager(cfg)

	// Add two static discoverers (simulating multiple methods)
	m.AddDiscoverer(NewStaticDiscoverer([]string{
		"nats://static1:4222",
		"nats://static2:4222",
	}))

	if err := m.Start(); err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	defer m.Stop()

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return len(m.GetEndpoints()) == 2, nil
	}); err != nil {
		t.Fatalf("expected endpoints: %v", err)
	}

	endpoints := m.GetEndpoints()
	if len(endpoints) != 2 {
		t.Errorf("GetEndpoints() = %d, want 2", len(endpoints))
	}

	// Verify ordering (static should come first based on PreferMethods)
	if len(endpoints) > 0 && endpoints[0].Method != DiscoveryMethodStatic {
		t.Errorf("endpoints[0].Method = %v, want static", endpoints[0].Method)
	}
}

func TestDiscoverer_Watch(t *testing.T) {
	// Use a static discoverer for reliable watch testing
	d := NewStaticDiscoverer([]string{"nats://server:4222"})
	defer d.Close()

	// Static discoverer Watch returns nil immediately (no changes to watch)
	err := d.Watch(context.Background(), func(endpoints []*DiscoveredEndpoint) {})
	if err != nil {
		t.Errorf("Watch() error = %v", err)
	}
}

func TestDNSDiscoverer_Watch(t *testing.T) {
	cfg := &DNSDiscoveryConfig{
		Domain:          "example.com",
		RefreshInterval: 50 * time.Millisecond,
		Timeout:         100 * time.Millisecond,
	}

	d, err := NewDNSDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewDNSDiscoverer() error = %v", err)
	}
	defer d.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	// Start watching - callback may or may not be called depending on DNS availability
	err = d.Watch(ctx, func(endpoints []*DiscoveredEndpoint) {
		// Just verify callback format is correct
	})
	if err != nil {
		t.Errorf("Watch() error = %v", err)
	}

	// Let watch run briefly
	start := time.Now()
	if err := helpers.WaitForTimeout(2*time.Second, 5*time.Millisecond, func() (bool, error) {
		return time.Since(start) >= 100*time.Millisecond, nil
	}); err != nil {
		t.Fatalf("watch wait did not elapse: %v", err)
	}
}

func TestMDNSDiscoverer_Watch(t *testing.T) {
	d, err := NewMDNSDiscoverer(nil)
	if err != nil {
		t.Fatalf("NewMDNSDiscoverer() error = %v", err)
	}
	defer d.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = d.Watch(ctx, func(endpoints []*DiscoveredEndpoint) {})
	if err != nil {
		t.Errorf("Watch() error = %v", err)
	}
}

func TestKubernetesDiscoverer_Watch(t *testing.T) {
	// Skip if kubeconfig is not available (not running in Kubernetes or without kubeconfig)
	if os.Getenv("KUBECONFIG") == "" {
		home, _ := os.UserHomeDir()
		if home != "" {
			if _, err := os.Stat(home + "/.kube/config"); os.IsNotExist(err) {
				t.Skip("Skipping: no kubeconfig available")
			}
		}
	}

	cfg := &KubernetesDiscoveryConfig{
		ServiceName:     "nats",
		RefreshInterval: 50 * time.Millisecond,
	}

	d, err := NewKubernetesDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewKubernetesDiscoverer() error = %v", err)
	}
	defer d.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = d.Watch(ctx, func(endpoints []*DiscoveredEndpoint) {})
	if err != nil {
		t.Errorf("Watch() error = %v", err)
	}
}

func TestServiceRegistryDiscoverer_Watch(t *testing.T) {
	cfg := &ServiceRegistryConfig{
		Type:            "consul",
		Address:         "localhost:8500",
		ServiceName:     "nats",
		RefreshInterval: 50 * time.Millisecond,
	}

	d, err := NewServiceRegistryDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewServiceRegistryDiscoverer() error = %v", err)
	}
	defer d.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err = d.Watch(ctx, func(endpoints []*DiscoveredEndpoint) {})
	if err != nil {
		t.Errorf("Watch() error = %v", err)
	}
}

func TestWatch_ZeroRefreshInterval(t *testing.T) {
	// DNS discoverer with zero refresh interval should fail Watch
	cfg := &DNSDiscoveryConfig{
		Domain:          "example.com",
		RefreshInterval: 0,
	}

	d, err := NewDNSDiscoverer(cfg)
	if err != nil {
		t.Fatalf("NewDNSDiscoverer() error = %v", err)
	}
	defer d.Close()

	err = d.Watch(context.Background(), func(endpoints []*DiscoveredEndpoint) {})
	if err == nil {
		t.Error("Watch() should error with zero refresh interval")
	}
}
