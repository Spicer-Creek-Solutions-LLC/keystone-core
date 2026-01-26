package servicemesh

import (
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
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

	if err := helpers.WaitForTimeout(2*time.Second, 10*time.Millisecond, func() (bool, error) {
		return !detector.isCacheValid(), nil
	}); err != nil {
		t.Fatalf("expected cache to be invalid after expiration: %v", err)
	}

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
		Enabled:        true,
		Mode:           "STRICT",
		CertChainFile:  "/etc/certs/cert-chain.pem",
		PrivateKeyFile: "/etc/certs/key.pem",
		CAFile:         "/etc/certs/root-cert.pem",
		CertProvider:   "istiod",
		SPIFFEID:       "spiffe://cluster.local/ns/default/sa/default",
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

func TestMeshTypeValues(t *testing.T) {
	// Test iota values
	if MeshTypeUnknown != 0 {
		t.Errorf("MeshTypeUnknown = %d, want 0", MeshTypeUnknown)
	}

	if MeshTypeIstio != 1 {
		t.Errorf("MeshTypeIstio = %d, want 1", MeshTypeIstio)
	}

	if MeshTypeLinkerd != 2 {
		t.Errorf("MeshTypeLinkerd = %d, want 2", MeshTypeLinkerd)
	}

	if MeshTypeConsul != 3 {
		t.Errorf("MeshTypeConsul = %d, want 3", MeshTypeConsul)
	}

	if MeshTypeKuma != 4 {
		t.Errorf("MeshTypeKuma = %d, want 4", MeshTypeKuma)
	}

	if MeshTypeOSM != 5 {
		t.Errorf("MeshTypeOSM = %d, want 5", MeshTypeOSM)
	}
}

func TestMetadata(t *testing.T) {
	now := time.Now()
	metadata := &Metadata{
		MeshType:         MeshTypeIstio,
		Version:          "1.18.3",
		ProxyType:        "envoy",
		ProxyVersion:     "1.25.0",
		ServiceName:      "frontend",
		ServiceNamespace: "production",
		ServiceVersion:   "v1.2.0",
		ClusterName:      "main-cluster",
		MeshID:           "mesh-id-123",
		TrustDomain:      "cluster.local",
		WorkloadName:     "frontend-deployment",
		Labels: map[string]string{
			"app":     "frontend",
			"version": "v1",
		},
		Annotations: map[string]string{
			"sidecar.istio.io/inject": "true",
		},
		DetectedAt: now,
	}

	if metadata.MeshType != MeshTypeIstio {
		t.Errorf("MeshType = %v, want %v", metadata.MeshType, MeshTypeIstio)
	}

	if metadata.Version != "1.18.3" {
		t.Errorf("Version = %v, want 1.18.3", metadata.Version)
	}

	if metadata.ProxyType != "envoy" {
		t.Errorf("ProxyType = %v, want envoy", metadata.ProxyType)
	}

	if metadata.ServiceName != "frontend" {
		t.Errorf("ServiceName = %v, want frontend", metadata.ServiceName)
	}

	if metadata.ServiceNamespace != "production" {
		t.Errorf("ServiceNamespace = %v, want production", metadata.ServiceNamespace)
	}

	if metadata.TrustDomain != "cluster.local" {
		t.Errorf("TrustDomain = %v, want cluster.local", metadata.TrustDomain)
	}

	if len(metadata.Labels) != 2 {
		t.Errorf("Labels length = %d, want 2", len(metadata.Labels))
	}

	if metadata.Labels["app"] != "frontend" {
		t.Errorf("Labels['app'] = %v, want frontend", metadata.Labels["app"])
	}
}

func TestMetadata_WithProxyConfig(t *testing.T) {
	metadata := &Metadata{
		MeshType:    MeshTypeIstio,
		ServiceName: "backend",
		ProxyConfig: &ProxyConfig{
			AdminPort:    15000,
			InboundPort:  15006,
			OutboundPort: 15001,
			MetricsPort:  15020,
			HealthPort:   15021,
			StatsPath:    "/stats/prometheus",
			ReadyPath:    "/healthz/ready",
			LivePath:     "/healthz/live",
			ConfigPath:   "/etc/istio/proxy/envoy.yaml",
			LogLevel:     "warning",
		},
		DetectedAt: time.Now(),
	}

	if metadata.ProxyConfig == nil {
		t.Fatal("ProxyConfig is nil")
	}

	if metadata.ProxyConfig.AdminPort != 15000 {
		t.Errorf("AdminPort = %d, want 15000", metadata.ProxyConfig.AdminPort)
	}

	if metadata.ProxyConfig.InboundPort != 15006 {
		t.Errorf("InboundPort = %d, want 15006", metadata.ProxyConfig.InboundPort)
	}

	if metadata.ProxyConfig.OutboundPort != 15001 {
		t.Errorf("OutboundPort = %d, want 15001", metadata.ProxyConfig.OutboundPort)
	}

	if metadata.ProxyConfig.StatsPath != "/stats/prometheus" {
		t.Errorf("StatsPath = %v, want /stats/prometheus", metadata.ProxyConfig.StatsPath)
	}

	if metadata.ProxyConfig.LogLevel != "warning" {
		t.Errorf("LogLevel = %v, want warning", metadata.ProxyConfig.LogLevel)
	}
}

func TestMetadata_WithTLSConfig(t *testing.T) {
	now := time.Now()
	validFrom := now.Add(-24 * time.Hour)
	validUntil := now.Add(30 * 24 * time.Hour)

	metadata := &Metadata{
		MeshType:    MeshTypeIstio,
		ServiceName: "api-gateway",
		TLSConfig: &TLSConfig{
			Enabled:        true,
			Mode:           "STRICT",
			CertChainFile:  "/etc/certs/cert-chain.pem",
			PrivateKeyFile: "/etc/certs/key.pem",
			CAFile:         "/etc/certs/root-cert.pem",
			CertProvider:   "istiod",
			SPIFFEID:       "spiffe://cluster.local/ns/production/sa/api-gateway",
			ValidFrom:      validFrom,
			ValidUntil:     validUntil,
		},
		DetectedAt: time.Now(),
	}

	if metadata.TLSConfig == nil {
		t.Fatal("TLSConfig is nil")
	}

	if !metadata.TLSConfig.Enabled {
		t.Error("TLSConfig.Enabled = false, want true")
	}

	if metadata.TLSConfig.Mode != "STRICT" {
		t.Errorf("TLSConfig.Mode = %v, want STRICT", metadata.TLSConfig.Mode)
	}

	if metadata.TLSConfig.CertProvider != "istiod" {
		t.Errorf("TLSConfig.CertProvider = %v, want istiod", metadata.TLSConfig.CertProvider)
	}

	if metadata.TLSConfig.SPIFFEID != "spiffe://cluster.local/ns/production/sa/api-gateway" {
		t.Errorf("TLSConfig.SPIFFEID = %v, want spiffe://cluster.local/ns/production/sa/api-gateway", metadata.TLSConfig.SPIFFEID)
	}

	if metadata.TLSConfig.ValidFrom.IsZero() {
		t.Error("TLSConfig.ValidFrom is zero")
	}

	if metadata.TLSConfig.ValidUntil.IsZero() {
		t.Error("TLSConfig.ValidUntil is zero")
	}
}

func TestMetricsInfo(t *testing.T) {
	metrics := &MetricsInfo{
		RequestsTotal:      1000000,
		RequestsSuccessful: 995000,
		RequestsFailed:     5000,
		ActiveConnections:  150,
		BytesSent:          1024 * 1024 * 1024, // 1GB
		BytesReceived:      512 * 1024 * 1024,  // 512MB
		RequestDuration: map[string]float64{
			"p50": 0.005, // 5ms
			"p90": 0.015, // 15ms
			"p95": 0.025, // 25ms
			"p99": 0.100, // 100ms
		},
	}

	if metrics.RequestsTotal != 1000000 {
		t.Errorf("RequestsTotal = %d, want 1000000", metrics.RequestsTotal)
	}

	if metrics.RequestsSuccessful != 995000 {
		t.Errorf("RequestsSuccessful = %d, want 995000", metrics.RequestsSuccessful)
	}

	if metrics.RequestsFailed != 5000 {
		t.Errorf("RequestsFailed = %d, want 5000", metrics.RequestsFailed)
	}

	if metrics.ActiveConnections != 150 {
		t.Errorf("ActiveConnections = %d, want 150", metrics.ActiveConnections)
	}

	if len(metrics.RequestDuration) != 4 {
		t.Errorf("RequestDuration length = %d, want 4", len(metrics.RequestDuration))
	}

	if metrics.RequestDuration["p50"] != 0.005 {
		t.Errorf("RequestDuration['p50'] = %v, want 0.005", metrics.RequestDuration["p50"])
	}

	if metrics.RequestDuration["p99"] != 0.100 {
		t.Errorf("RequestDuration['p99'] = %v, want 0.100", metrics.RequestDuration["p99"])
	}

	// Calculate error rate
	errorRate := float64(metrics.RequestsFailed) / float64(metrics.RequestsTotal) * 100
	if errorRate != 0.5 {
		t.Errorf("Error rate = %.2f%%, want 0.50%%", errorRate)
	}
}

func TestCircuitBreakerInfo(t *testing.T) {
	cb := &CircuitBreakerInfo{
		Enabled:            true,
		ConsecutiveErrors:  5,
		Interval:           10 * time.Second,
		BaseEjectionTime:   30 * time.Second,
		MaxEjectionPercent: 10,
	}

	if !cb.Enabled {
		t.Error("Enabled = false, want true")
	}

	if cb.ConsecutiveErrors != 5 {
		t.Errorf("ConsecutiveErrors = %d, want 5", cb.ConsecutiveErrors)
	}

	if cb.Interval != 10*time.Second {
		t.Errorf("Interval = %v, want 10s", cb.Interval)
	}

	if cb.BaseEjectionTime != 30*time.Second {
		t.Errorf("BaseEjectionTime = %v, want 30s", cb.BaseEjectionTime)
	}

	if cb.MaxEjectionPercent != 10 {
		t.Errorf("MaxEjectionPercent = %d, want 10", cb.MaxEjectionPercent)
	}
}

func TestCircuitBreakerInfo_Disabled(t *testing.T) {
	cb := &CircuitBreakerInfo{
		Enabled:            false,
		ConsecutiveErrors:  0,
		Interval:           0,
		BaseEjectionTime:   0,
		MaxEjectionPercent: 0,
	}

	if cb.Enabled {
		t.Error("Enabled = true, want false")
	}
}

func TestRetryPolicyInfo(t *testing.T) {
	retry := &RetryPolicyInfo{
		Enabled:       true,
		Attempts:      3,
		PerTryTimeout: 5 * time.Second,
		RetryOn:       []string{"5xx", "reset", "connect-failure", "retriable-4xx"},
	}

	if !retry.Enabled {
		t.Error("Enabled = false, want true")
	}

	if retry.Attempts != 3 {
		t.Errorf("Attempts = %d, want 3", retry.Attempts)
	}

	if retry.PerTryTimeout != 5*time.Second {
		t.Errorf("PerTryTimeout = %v, want 5s", retry.PerTryTimeout)
	}

	if len(retry.RetryOn) != 4 {
		t.Errorf("RetryOn length = %d, want 4", len(retry.RetryOn))
	}

	if retry.RetryOn[0] != "5xx" {
		t.Errorf("RetryOn[0] = %v, want 5xx", retry.RetryOn[0])
	}
}

func TestRetryPolicyInfo_Disabled(t *testing.T) {
	retry := &RetryPolicyInfo{
		Enabled:       false,
		Attempts:      0,
		PerTryTimeout: 0,
		RetryOn:       nil,
	}

	if retry.Enabled {
		t.Error("Enabled = true, want false")
	}

	if retry.RetryOn != nil {
		t.Errorf("RetryOn = %v, want nil", retry.RetryOn)
	}
}

func TestConfig_Custom(t *testing.T) {
	config := &Config{
		Timeout:       15 * time.Second,
		EnableIstio:   true,
		EnableLinkerd: false,
		EnableConsul:  true,
		EnableKuma:    false,
		EnableOSM:     false,
		CacheDuration: 10 * time.Minute,
	}

	if config.Timeout != 15*time.Second {
		t.Errorf("Timeout = %v, want 15s", config.Timeout)
	}

	if !config.EnableIstio {
		t.Error("EnableIstio = false, want true")
	}

	if config.EnableLinkerd {
		t.Error("EnableLinkerd = true, want false")
	}

	if !config.EnableConsul {
		t.Error("EnableConsul = false, want true")
	}

	if config.EnableKuma {
		t.Error("EnableKuma = true, want false")
	}

	if config.EnableOSM {
		t.Error("EnableOSM = true, want false")
	}

	if config.CacheDuration != 10*time.Minute {
		t.Errorf("CacheDuration = %v, want 10m", config.CacheDuration)
	}
}

func TestIstioDetector_IsServiceMesh(t *testing.T) {
	detector := NewIstioDetector(nil)

	// In test environment (not Istio), should return false
	if detector.IsServiceMesh() {
		t.Error("expected false in non-Istio environment")
	}
}

func TestLinkerdDetector_IsServiceMesh(t *testing.T) {
	detector := NewLinkerdDetector(nil)

	// In test environment (not Linkerd), should return false
	if detector.IsServiceMesh() {
		t.Error("expected false in non-Linkerd environment")
	}
}

func TestConsulDetector_IsServiceMesh(t *testing.T) {
	detector := NewConsulDetector(nil)

	// In test environment (not Consul), should return false
	if detector.IsServiceMesh() {
		t.Error("expected false in non-Consul environment")
	}
}

func TestIstioDetector_Detect_NotInIstio(t *testing.T) {
	detector := NewIstioDetector(nil)

	_, err := detector.Detect()
	if err == nil {
		t.Error("expected error when not in Istio")
	}
}

func TestLinkerdDetector_Detect_NotInLinkerd(t *testing.T) {
	detector := NewLinkerdDetector(nil)

	_, err := detector.Detect()
	if err == nil {
		t.Error("expected error when not in Linkerd")
	}
}

func TestConsulDetector_Detect_NotInConsul(t *testing.T) {
	detector := NewConsulDetector(nil)

	_, err := detector.Detect()
	if err == nil {
		t.Error("expected error when not in Consul")
	}
}

func TestMultiMeshDetector_Detect_NotInMesh(t *testing.T) {
	detector := NewDetector(nil)

	_, err := detector.Detect()
	if err == nil {
		t.Error("expected error when not in any service mesh")
	}
}

func TestProxyConfig_Complete(t *testing.T) {
	config := &ProxyConfig{
		AdminPort:    15000,
		InboundPort:  15006,
		OutboundPort: 15001,
		MetricsPort:  15020,
		HealthPort:   15021,
		StatsPath:    "/stats/prometheus",
		ReadyPath:    "/healthz/ready",
		LivePath:     "/healthz/live",
		ConfigPath:   "/etc/istio/proxy/envoy.yaml",
		LogLevel:     "info",
	}

	if config.AdminPort != 15000 {
		t.Errorf("AdminPort = %d, want 15000", config.AdminPort)
	}

	if config.InboundPort != 15006 {
		t.Errorf("InboundPort = %d, want 15006", config.InboundPort)
	}

	if config.OutboundPort != 15001 {
		t.Errorf("OutboundPort = %d, want 15001", config.OutboundPort)
	}

	if config.MetricsPort != 15020 {
		t.Errorf("MetricsPort = %d, want 15020", config.MetricsPort)
	}

	if config.HealthPort != 15021 {
		t.Errorf("HealthPort = %d, want 15021", config.HealthPort)
	}

	if config.LivePath != "/healthz/live" {
		t.Errorf("LivePath = %v, want /healthz/live", config.LivePath)
	}

	if config.ConfigPath != "/etc/istio/proxy/envoy.yaml" {
		t.Errorf("ConfigPath = %v, want /etc/istio/proxy/envoy.yaml", config.ConfigPath)
	}

	if config.LogLevel != "info" {
		t.Errorf("LogLevel = %v, want info", config.LogLevel)
	}
}

func TestTLSConfig_Permissive(t *testing.T) {
	config := &TLSConfig{
		Enabled:        true,
		Mode:           "PERMISSIVE",
		CertChainFile:  "/etc/certs/cert-chain.pem",
		PrivateKeyFile: "/etc/certs/key.pem",
		CAFile:         "/etc/certs/root-cert.pem",
		CertProvider:   "istiod",
		SPIFFEID:       "spiffe://cluster.local/ns/default/sa/default",
	}

	if !config.Enabled {
		t.Error("Enabled = false, want true")
	}

	if config.Mode != "PERMISSIVE" {
		t.Errorf("Mode = %v, want PERMISSIVE", config.Mode)
	}
}

func TestTLSConfig_Disabled(t *testing.T) {
	config := &TLSConfig{
		Enabled: false,
		Mode:    "DISABLED",
	}

	if config.Enabled {
		t.Error("Enabled = true, want false")
	}

	if config.Mode != "DISABLED" {
		t.Errorf("Mode = %v, want DISABLED", config.Mode)
	}
}
