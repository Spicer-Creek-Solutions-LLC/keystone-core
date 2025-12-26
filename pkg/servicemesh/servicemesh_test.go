package servicemesh

import (
	"testing"
	"time"
)

func TestMeshType_String(t *testing.T) {
	tests := []struct {
		meshType MeshType
		expected string
	}{
		{MeshTypeIstio, "istio"},
		{MeshTypeLinkerd, "linkerd"},
		{MeshTypeConsul, "consul"},
		{MeshTypeKuma, "kuma"},
		{MeshTypeOSM, "osm"},
		{MeshTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.meshType.String(); got != tt.expected {
			t.Errorf("MeshType.String() = %v, want %v", got, tt.expected)
		}
	}
}

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config == nil {
		t.Fatal("DefaultConfig returned nil")
	}

	if config.Timeout != 5*time.Second {
		t.Errorf("expected timeout 5s, got %v", config.Timeout)
	}

	if !config.EnableIstio {
		t.Error("expected Istio to be enabled")
	}

	if !config.EnableLinkerd {
		t.Error("expected Linkerd to be enabled")
	}

	if !config.EnableConsul {
		t.Error("expected Consul to be enabled")
	}

	if config.CacheDuration != 5*time.Minute {
		t.Errorf("expected cache duration 5m, got %v", config.CacheDuration)
	}
}

func TestNewDetector(t *testing.T) {
	detector := NewDetector(nil)

	if detector == nil {
		t.Fatal("NewDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}

	if detector.detectors == nil {
		t.Error("detector detectors map is nil")
	}

	// Should have Istio, Linkerd, and Consul detectors registered
	if len(detector.detectors) != 3 {
		t.Errorf("expected 3 detectors, got %d", len(detector.detectors))
	}
}

func TestNewDetector_CustomConfig(t *testing.T) {
	config := &Config{
		Timeout:       10 * time.Second,
		EnableIstio:   true,
		EnableLinkerd: false,
		EnableConsul:  false,
		CacheDuration: 1 * time.Minute,
	}

	detector := NewDetector(config)

	if detector == nil {
		t.Fatal("NewDetector returned nil")
	}

	// Should only have Istio detector
	if len(detector.detectors) != 1 {
		t.Errorf("expected 1 detector, got %d", len(detector.detectors))
	}

	if _, ok := detector.detectors[MeshTypeIstio]; !ok {
		t.Error("Istio detector not registered")
	}
}

func TestMultiMeshDetector_GetMeshType(t *testing.T) {
	detector := NewDetector(nil)

	// In non-mesh environment, should return unknown
	meshType := detector.GetMeshType()

	if meshType != MeshTypeUnknown {
		t.Errorf("expected MeshTypeUnknown in non-mesh env, got %v", meshType)
	}
}

func TestMultiMeshDetector_IsServiceMesh(t *testing.T) {
	detector := NewDetector(nil)

	// In normal test environment (not service mesh), should return false
	isMesh := detector.IsServiceMesh()

	if isMesh {
		t.Error("expected false in non-service mesh environment")
	}
}

func TestMultiMeshDetector_Cache(t *testing.T) {
	detector := NewDetector(nil)

	// Initially cache should be invalid
	if detector.isCacheValid() {
		t.Error("expected cache to be invalid initially")
	}

	// Set cache
	detector.mu.Lock()
	detector.cache = &Metadata{
		MeshType:   MeshTypeIstio,
		DetectedAt: time.Now(),
	}
	detector.cacheTime = time.Now()
	detector.mu.Unlock()

	// Cache should be valid now
	if !detector.isCacheValid() {
		t.Error("expected cache to be valid")
	}

	// Clear cache
	detector.ClearCache()

	// Cache should be invalid again
	if detector.isCacheValid() {
		t.Error("expected cache to be invalid after clear")
	}
}

func TestMultiMeshDetector_CacheExpiration(t *testing.T) {
	config := &Config{
		Timeout:       5 * time.Second,
		EnableIstio:   true,
		EnableLinkerd: true,
		EnableConsul:  true,
		CacheDuration: 100 * time.Millisecond, // Very short for testing
	}

	detector := NewDetector(config)

	// Set cache
	detector.mu.Lock()
	detector.cache = &Metadata{
		MeshType:   MeshTypeIstio,
		DetectedAt: time.Now(),
	}
	detector.cacheTime = time.Now()
	detector.mu.Unlock()

	// Cache should be valid immediately
	if !detector.isCacheValid() {
		t.Error("expected cache to be valid immediately")
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Cache should be invalid now
	if detector.isCacheValid() {
		t.Error("expected cache to be invalid after expiration")
	}
}

func TestGetDefaultDetector(t *testing.T) {
	detector1 := GetDefaultDetector()
	detector2 := GetDefaultDetector()

	// Should return the same instance
	if detector1 != detector2 {
		t.Error("GetDefaultDetector should return the same instance")
	}
}

func TestIstioDetector_New(t *testing.T) {
	detector := NewIstioDetector(nil)

	if detector == nil {
		t.Fatal("NewIstioDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}

	if detector.httpClient == nil {
		t.Error("detector httpClient is nil")
	}
}

func TestIstioDetector_GetMeshType(t *testing.T) {
	detector := NewIstioDetector(nil)

	if detector.GetMeshType() != MeshTypeIstio {
		t.Error("expected MeshTypeIstio")
	}
}

func TestLinkerdDetector_New(t *testing.T) {
	detector := NewLinkerdDetector(nil)

	if detector == nil {
		t.Fatal("NewLinkerdDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}

	if detector.httpClient == nil {
		t.Error("detector httpClient is nil")
	}
}

func TestLinkerdDetector_GetMeshType(t *testing.T) {
	detector := NewLinkerdDetector(nil)

	if detector.GetMeshType() != MeshTypeLinkerd {
		t.Error("expected MeshTypeLinkerd")
	}
}

func TestConsulDetector_New(t *testing.T) {
	detector := NewConsulDetector(nil)

	if detector == nil {
		t.Fatal("NewConsulDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}

	if detector.httpClient == nil {
		t.Error("detector httpClient is nil")
	}
}

func TestConsulDetector_GetMeshType(t *testing.T) {
	detector := NewConsulDetector(nil)

	if detector.GetMeshType() != MeshTypeConsul {
		t.Error("expected MeshTypeConsul")
	}
}

func TestProxyConfig(t *testing.T) {
	config := &ProxyConfig{
		AdminPort:    15000,
		InboundPort:  15006,
		OutboundPort: 15001,
		MetricsPort:  15020,
		HealthPort:   15021,
		StatsPath:    "/stats/prometheus",
		ReadyPath:    "/healthz/ready",
	}

	if config.AdminPort != 15000 {
		t.Errorf("unexpected admin port: %d", config.AdminPort)
	}

	if config.StatsPath != "/stats/prometheus" {
		t.Errorf("unexpected stats path: %s", config.StatsPath)
	}
}

func TestTLSConfig(t *testing.T) {
	config := &TLSConfig{
		Enabled:       true,
		Mode:          "STRICT",
		CertChainFile: "/etc/certs/cert-chain.pem",
		PrivateKeyFile: "/etc/certs/key.pem",
		CAFile:        "/etc/certs/root-cert.pem",
		CertProvider:  "istiod",
		SPIFFEID:      "spiffe://cluster.local/ns/default/sa/default",
	}

	if !config.Enabled {
		t.Error("expected TLS to be enabled")
	}

	if config.Mode != "STRICT" {
		t.Errorf("unexpected mode: %s", config.Mode)
	}

	if config.CertProvider != "istiod" {
		t.Errorf("unexpected cert provider: %s", config.CertProvider)
	}
}

func TestExtractIstioVersion(t *testing.T) {
	tests := []struct {
		envoyVersion string
		expected     string
	}{
		{"1.18.3/1.18.3/Clean/RELEASE/BoringSSL", "1.18.3"},
		{"1.19.0", "1.19.0"},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractIstioVersion(tt.envoyVersion)
		if got != tt.expected {
			t.Errorf("extractIstioVersion(%q) = %q, want %q", tt.envoyVersion, got, tt.expected)
		}
	}
}
