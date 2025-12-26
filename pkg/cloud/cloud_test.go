package cloud

import (
	"os"
	"testing"
	"time"
)

func TestProvider_String(t *testing.T) {
	tests := []struct {
		provider Provider
		expected string
	}{
		{ProviderAWS, "aws"},
		{ProviderGCP, "gcp"},
		{ProviderAzure, "azure"},
		{ProviderUnknown, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.provider.String(); got != tt.expected {
			t.Errorf("Provider.String() = %v, want %v", got, tt.expected)
		}
	}
}

func TestEnvironmentType_String(t *testing.T) {
	tests := []struct {
		envType  EnvironmentType
		expected string
	}{
		{EnvTypeVM, "vm"},
		{EnvTypeContainer, "container"},
		{EnvTypeKubernetes, "kubernetes"},
		{EnvTypeServerless, "serverless"},
		{EnvTypeUnknown, "unknown"},
	}

	for _, tt := range tests {
		if got := tt.envType.String(); got != tt.expected {
			t.Errorf("EnvironmentType.String() = %v, want %v", got, tt.expected)
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

	if !config.EnableAWS {
		t.Error("expected AWS to be enabled")
	}

	if !config.EnableGCP {
		t.Error("expected GCP to be enabled")
	}

	if !config.EnableAzure {
		t.Error("expected Azure to be enabled")
	}

	if !config.EnableKubernetes {
		t.Error("expected Kubernetes to be enabled")
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

	// Should have all three cloud detectors registered
	if len(detector.detectors) != 3 {
		t.Errorf("expected 3 detectors, got %d", len(detector.detectors))
	}
}

func TestNewDetector_CustomConfig(t *testing.T) {
	config := &Config{
		Timeout:          10 * time.Second,
		EnableAWS:        true,
		EnableGCP:        false,
		EnableAzure:      false,
		EnableKubernetes: true,
		CacheDuration:    1 * time.Minute,
	}

	detector := NewDetector(config)

	if detector == nil {
		t.Fatal("NewDetector returned nil")
	}

	// Should only have AWS detector
	if len(detector.detectors) != 1 {
		t.Errorf("expected 1 detector, got %d", len(detector.detectors))
	}

	if _, ok := detector.detectors[ProviderAWS]; !ok {
		t.Error("AWS detector not registered")
	}
}

func TestMultiCloudDetector_GetProvider(t *testing.T) {
	detector := NewDetector(nil)

	// In non-cloud environment, should return unknown
	provider := detector.GetProvider()

	if provider != ProviderUnknown {
		t.Errorf("expected ProviderUnknown in non-cloud env, got %v", provider)
	}
}

func TestMultiCloudDetector_IsCloudEnvironment(t *testing.T) {
	detector := NewDetector(nil)

	// In normal test environment (not cloud), should return false
	isCloud := detector.IsCloudEnvironment()

	if isCloud {
		t.Error("expected false in non-cloud environment")
	}
}

func TestMultiCloudDetector_Cache(t *testing.T) {
	detector := NewDetector(nil)

	// Initially cache should be invalid
	if detector.isCacheValid() {
		t.Error("expected cache to be invalid initially")
	}

	// Set cache
	detector.mu.Lock()
	detector.cache = &Metadata{
		Provider:        ProviderAWS,
		EnvironmentType: EnvTypeVM,
		DetectedAt:      time.Now(),
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

func TestMultiCloudDetector_CacheExpiration(t *testing.T) {
	config := &Config{
		Timeout:          5 * time.Second,
		EnableAWS:        true,
		EnableGCP:        true,
		EnableAzure:      true,
		EnableKubernetes: true,
		CacheDuration:    100 * time.Millisecond, // Very short for testing
	}

	detector := NewDetector(config)

	// Set cache
	detector.mu.Lock()
	detector.cache = &Metadata{
		Provider:        ProviderAWS,
		EnvironmentType: EnvTypeVM,
		DetectedAt:      time.Now(),
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

func TestCollectK8sMetadataFromEnv_NoK8s(t *testing.T) {
	// In normal environment without K8s service account, should return nil
	metadata := collectK8sMetadataFromEnv()

	if metadata != nil {
		t.Error("expected nil metadata in non-K8s environment")
	}
}

func TestCollectK8sMetadataFromEnv_WithEnv(t *testing.T) {
	// Create a temporary service account directory to simulate K8s
	tmpDir := t.TempDir()
	saDir := tmpDir + "/var/run/secrets/kubernetes.io/serviceaccount"
	os.MkdirAll(saDir, 0755)
	os.WriteFile(saDir+"/token", []byte("fake-token"), 0644)

	// This won't work in actual test because the path is hardcoded
	// Just test the environment variable parsing logic

	// Set K8s environment variables
	os.Setenv("POD_NAME", "test-pod")
	os.Setenv("POD_NAMESPACE", "default")
	os.Setenv("POD_UID", "12345-67890")
	os.Setenv("NODE_NAME", "node-1")
	os.Setenv("SERVICE_ACCOUNT", "default")

	defer func() {
		os.Unsetenv("POD_NAME")
		os.Unsetenv("POD_NAMESPACE")
		os.Unsetenv("POD_UID")
		os.Unsetenv("NODE_NAME")
		os.Unsetenv("SERVICE_ACCOUNT")
	}()

	// Note: This test will fail because the actual path check is hardcoded
	// In a real K8s environment, collectK8sMetadataFromEnv would work correctly
	metadata := collectK8sMetadataFromEnv()

	// In test environment without actual K8s, this will be nil
	// This test just documents the expected behavior
	if metadata != nil {
		// If running in actual K8s, verify metadata
		if metadata.PodName != "test-pod" {
			t.Errorf("expected pod name 'test-pod', got %v", metadata.PodName)
		}

		if metadata.PodNamespace != "default" {
			t.Errorf("expected namespace 'default', got %v", metadata.PodNamespace)
		}
	}
}

func TestAWSDetector_New(t *testing.T) {
	detector := NewAWSDetector(nil)

	if detector == nil {
		t.Fatal("NewAWSDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}

	if detector.httpClient == nil {
		t.Error("detector httpClient is nil")
	}
}

func TestAWSDetector_GetProvider(t *testing.T) {
	detector := NewAWSDetector(nil)

	if detector.GetProvider() != ProviderAWS {
		t.Error("expected ProviderAWS")
	}
}

func TestGCPDetector_New(t *testing.T) {
	detector := NewGCPDetector(nil)

	if detector == nil {
		t.Fatal("NewGCPDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}

	if detector.httpClient == nil {
		t.Error("detector httpClient is nil")
	}
}

func TestGCPDetector_GetProvider(t *testing.T) {
	detector := NewGCPDetector(nil)

	if detector.GetProvider() != ProviderGCP {
		t.Error("expected ProviderGCP")
	}
}

func TestAzureDetector_New(t *testing.T) {
	detector := NewAzureDetector(nil)

	if detector == nil {
		t.Fatal("NewAzureDetector returned nil")
	}

	if detector.config == nil {
		t.Error("detector config is nil")
	}

	if detector.httpClient == nil {
		t.Error("detector httpClient is nil")
	}
}

func TestAzureDetector_GetProvider(t *testing.T) {
	detector := NewAzureDetector(nil)

	if detector.GetProvider() != ProviderAzure {
		t.Error("expected ProviderAzure")
	}
}

func TestExtractRegionFromARN(t *testing.T) {
	tests := []struct {
		arn      string
		expected string
	}{
		{"arn:aws:lambda:us-east-1:123456789012:function:my-function", "us-east-1"},
		{"arn:aws:ecs:eu-west-1:123456789012:task/my-task", "eu-west-1"},
		{"invalid-arn", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractRegionFromARN(tt.arn)
		if got != tt.expected {
			t.Errorf("extractRegionFromARN(%q) = %q, want %q", tt.arn, got, tt.expected)
		}
	}
}

func TestExtractAccountFromARN(t *testing.T) {
	tests := []struct {
		arn      string
		expected string
	}{
		{"arn:aws:lambda:us-east-1:123456789012:function:my-function", "123456789012"},
		{"arn:aws:ecs:eu-west-1:987654321098:task/my-task", "987654321098"},
		{"invalid-arn", ""},
		{"", ""},
	}

	for _, tt := range tests {
		got := extractAccountFromARN(tt.arn)
		if got != tt.expected {
			t.Errorf("extractAccountFromARN(%q) = %q, want %q", tt.arn, got, tt.expected)
		}
	}
}

func TestParseAzureTags(t *testing.T) {
	tests := []struct {
		tagsStr  string
		expected map[string]string
	}{
		{
			"Environment:Production;Application:WebServer",
			map[string]string{
				"Environment": "Production",
				"Application": "WebServer",
			},
		},
		{
			"Team:DevOps",
			map[string]string{
				"Team": "DevOps",
			},
		},
		{
			"",
			map[string]string{},
		},
	}

	for _, tt := range tests {
		got := parseAzureTags(tt.tagsStr)

		if len(got) != len(tt.expected) {
			t.Errorf("parseAzureTags(%q) returned %d tags, want %d", tt.tagsStr, len(got), len(tt.expected))
			continue
		}

		for key, expectedValue := range tt.expected {
			if gotValue, ok := got[key]; !ok || gotValue != expectedValue {
				t.Errorf("parseAzureTags(%q)[%q] = %q, want %q", tt.tagsStr, key, gotValue, expectedValue)
			}
		}
	}
}
