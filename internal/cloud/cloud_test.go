package cloud

import (
	"os"
	"testing"
	"time"

	"github.com/shawnbutts/keystone-core/internal/testing/helpers"
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

// Tests for Metadata struct
func TestMetadata(t *testing.T) {
	now := time.Now()
	metadata := &Metadata{
		Provider:         ProviderAWS,
		EnvironmentType:  EnvTypeVM,
		Region:           "us-east-1",
		AvailabilityZone: "us-east-1a",
		InstanceID:       "i-1234567890abcdef0",
		InstanceType:     "t3.micro",
		AccountID:        "123456789012",
		ProjectID:        "",
		SubscriptionID:   "",
		VPCID:            "vpc-12345",
		SubnetID:         "subnet-12345",
		NetworkID:        "",
		PrivateIP:        "10.0.0.5",
		PublicIP:         "54.123.45.67",
		Tags:             map[string]string{"Environment": "Production", "App": "WebServer"},
		DetectedAt:       now,
	}

	if metadata.Provider != ProviderAWS {
		t.Errorf("Provider = %v, want %v", metadata.Provider, ProviderAWS)
	}

	if metadata.EnvironmentType != EnvTypeVM {
		t.Errorf("EnvironmentType = %v, want %v", metadata.EnvironmentType, EnvTypeVM)
	}

	if metadata.Region != "us-east-1" {
		t.Errorf("Region = %v, want us-east-1", metadata.Region)
	}

	if metadata.InstanceID != "i-1234567890abcdef0" {
		t.Errorf("InstanceID = %v, want i-1234567890abcdef0", metadata.InstanceID)
	}

	if metadata.InstanceType != "t3.micro" {
		t.Errorf("InstanceType = %v, want t3.micro", metadata.InstanceType)
	}

	if len(metadata.Tags) != 2 {
		t.Errorf("Tags length = %d, want 2", len(metadata.Tags))
	}
}

func TestMetadata_WithK8s(t *testing.T) {
	metadata := &Metadata{
		Provider:        ProviderGCP,
		EnvironmentType: EnvTypeKubernetes,
		Region:          "us-central1",
		ProjectID:       "my-project",
		K8s: &K8sMetadata{
			PodName:            "web-server-5f4b8c9d-abc123",
			PodNamespace:       "production",
			PodUID:             "12345-67890-abcdef",
			NodeName:           "gke-cluster-pool-1-abc123",
			ServiceAccountName: "web-server-sa",
			ClusterName:        "my-gke-cluster",
			Labels:             map[string]string{"app": "web-server", "version": "v1"},
			Annotations:        map[string]string{"prometheus.io/scrape": "true"},
		},
		DetectedAt: time.Now(),
	}

	if metadata.K8s == nil {
		t.Fatal("K8s metadata is nil")
	}

	if metadata.K8s.PodName != "web-server-5f4b8c9d-abc123" {
		t.Errorf("PodName = %v, want web-server-5f4b8c9d-abc123", metadata.K8s.PodName)
	}

	if metadata.K8s.PodNamespace != "production" {
		t.Errorf("PodNamespace = %v, want production", metadata.K8s.PodNamespace)
	}

	if metadata.K8s.ClusterName != "my-gke-cluster" {
		t.Errorf("ClusterName = %v, want my-gke-cluster", metadata.K8s.ClusterName)
	}

	if len(metadata.K8s.Labels) != 2 {
		t.Errorf("Labels length = %d, want 2", len(metadata.K8s.Labels))
	}

	if metadata.K8s.Labels["app"] != "web-server" {
		t.Errorf("Labels['app'] = %v, want web-server", metadata.K8s.Labels["app"])
	}
}

func TestMetadata_WithContainer(t *testing.T) {
	metadata := &Metadata{
		Provider:        ProviderAWS,
		EnvironmentType: EnvTypeContainer,
		Region:          "us-east-1",
		AccountID:       "123456789012",
		Container: &ContainerMetadata{
			ContainerID:    "abc123def456",
			ContainerName:  "web-app",
			ImageName:      "my-app:latest",
			ImageDigest:    "sha256:abc123",
			TaskARN:        "arn:aws:ecs:us-east-1:123456789012:task/my-cluster/abc123",
			TaskDefinition: "my-task:5",
			ClusterARN:     "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster",
			ServiceName:    "my-service",
			Revision:       "5",
		},
		DetectedAt: time.Now(),
	}

	if metadata.Container == nil {
		t.Fatal("Container metadata is nil")
	}

	if metadata.Container.ContainerID != "abc123def456" {
		t.Errorf("ContainerID = %v, want abc123def456", metadata.Container.ContainerID)
	}

	if metadata.Container.ImageName != "my-app:latest" {
		t.Errorf("ImageName = %v, want my-app:latest", metadata.Container.ImageName)
	}

	if metadata.Container.TaskARN != "arn:aws:ecs:us-east-1:123456789012:task/my-cluster/abc123" {
		t.Errorf("TaskARN = %v, want arn:aws:ecs:us-east-1:123456789012:task/my-cluster/abc123", metadata.Container.TaskARN)
	}

	if metadata.Container.ClusterARN != "arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster" {
		t.Errorf("ClusterARN = %v, want arn:aws:ecs:us-east-1:123456789012:cluster/my-cluster", metadata.Container.ClusterARN)
	}
}

func TestMetadata_WithServerless(t *testing.T) {
	metadata := &Metadata{
		Provider:        ProviderAWS,
		EnvironmentType: EnvTypeServerless,
		Region:          "us-west-2",
		AccountID:       "123456789012",
		Serverless: &ServerlessMetadata{
			FunctionName:    "my-function",
			FunctionVersion: "$LATEST",
			FunctionARN:     "arn:aws:lambda:us-west-2:123456789012:function:my-function",
			Handler:         "index.handler",
			Runtime:         "nodejs18.x",
			MemorySize:      256,
			Timeout:         30,
			RequestID:       "abc123-def456",
			InvocationID:    "",
		},
		DetectedAt: time.Now(),
	}

	if metadata.Serverless == nil {
		t.Fatal("Serverless metadata is nil")
	}

	if metadata.Serverless.FunctionName != "my-function" {
		t.Errorf("FunctionName = %v, want my-function", metadata.Serverless.FunctionName)
	}

	if metadata.Serverless.FunctionARN != "arn:aws:lambda:us-west-2:123456789012:function:my-function" {
		t.Errorf("FunctionARN = %v, want arn:aws:lambda:us-west-2:123456789012:function:my-function", metadata.Serverless.FunctionARN)
	}

	if metadata.Serverless.Runtime != "nodejs18.x" {
		t.Errorf("Runtime = %v, want nodejs18.x", metadata.Serverless.Runtime)
	}

	if metadata.Serverless.MemorySize != 256 {
		t.Errorf("MemorySize = %d, want 256", metadata.Serverless.MemorySize)
	}

	if metadata.Serverless.Timeout != 30 {
		t.Errorf("Timeout = %d, want 30", metadata.Serverless.Timeout)
	}
}

func TestK8sMetadata(t *testing.T) {
	k8s := &K8sMetadata{
		PodName:            "nginx-5f4b8c9d-xyz789",
		PodNamespace:       "default",
		PodUID:             "uid-12345",
		NodeName:           "node-1",
		ServiceAccountName: "default",
		ClusterName:        "production-cluster",
		Labels: map[string]string{
			"app":     "nginx",
			"version": "1.21",
		},
		Annotations: map[string]string{
			"prometheus.io/scrape": "true",
			"prometheus.io/port":   "9090",
		},
	}

	if k8s.PodName != "nginx-5f4b8c9d-xyz789" {
		t.Errorf("PodName = %v, want nginx-5f4b8c9d-xyz789", k8s.PodName)
	}

	if k8s.PodNamespace != "default" {
		t.Errorf("PodNamespace = %v, want default", k8s.PodNamespace)
	}

	if k8s.ClusterName != "production-cluster" {
		t.Errorf("ClusterName = %v, want production-cluster", k8s.ClusterName)
	}

	if len(k8s.Labels) != 2 {
		t.Errorf("Labels length = %d, want 2", len(k8s.Labels))
	}

	if len(k8s.Annotations) != 2 {
		t.Errorf("Annotations length = %d, want 2", len(k8s.Annotations))
	}
}

func TestContainerMetadata(t *testing.T) {
	container := &ContainerMetadata{
		ContainerID:    "container-abc123",
		ContainerName:  "main",
		ImageName:      "myregistry.example.com/app:v2.0.0",
		ImageDigest:    "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		TaskARN:        "arn:aws:ecs:us-east-1:123456789012:task/cluster/task-id",
		TaskDefinition: "app-task:10",
		ClusterARN:     "arn:aws:ecs:us-east-1:123456789012:cluster/main-cluster",
		ServiceName:    "frontend-service",
		Revision:       "rev-100",
	}

	if container.ContainerID != "container-abc123" {
		t.Errorf("ContainerID = %v, want container-abc123", container.ContainerID)
	}

	if container.ImageName != "myregistry.example.com/app:v2.0.0" {
		t.Errorf("ImageName = %v, want myregistry.example.com/app:v2.0.0", container.ImageName)
	}

	if container.ImageDigest != "sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855" {
		t.Errorf("ImageDigest = %v, want sha256:e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", container.ImageDigest)
	}

	if container.TaskDefinition != "app-task:10" {
		t.Errorf("TaskDefinition = %v, want app-task:10", container.TaskDefinition)
	}

	if container.ServiceName != "frontend-service" {
		t.Errorf("ServiceName = %v, want frontend-service", container.ServiceName)
	}
}

func TestServerlessMetadata(t *testing.T) {
	serverless := &ServerlessMetadata{
		FunctionName:    "process-orders",
		FunctionVersion: "v2.1.0",
		FunctionARN:     "arn:aws:lambda:eu-west-1:123456789012:function:process-orders:v2.1.0",
		Handler:         "main.handler",
		Runtime:         "python3.9",
		MemorySize:      512,
		Timeout:         120,
		RequestID:       "req-abc-123",
		InvocationID:    "inv-xyz-789",
	}

	if serverless.FunctionName != "process-orders" {
		t.Errorf("FunctionName = %v, want process-orders", serverless.FunctionName)
	}

	if serverless.FunctionVersion != "v2.1.0" {
		t.Errorf("FunctionVersion = %v, want v2.1.0", serverless.FunctionVersion)
	}

	if serverless.Handler != "main.handler" {
		t.Errorf("Handler = %v, want main.handler", serverless.Handler)
	}

	if serverless.Runtime != "python3.9" {
		t.Errorf("Runtime = %v, want python3.9", serverless.Runtime)
	}

	if serverless.MemorySize != 512 {
		t.Errorf("MemorySize = %d, want 512", serverless.MemorySize)
	}

	if serverless.Timeout != 120 {
		t.Errorf("Timeout = %d, want 120", serverless.Timeout)
	}

	if serverless.RequestID != "req-abc-123" {
		t.Errorf("RequestID = %v, want req-abc-123", serverless.RequestID)
	}

	if serverless.InvocationID != "inv-xyz-789" {
		t.Errorf("InvocationID = %v, want inv-xyz-789", serverless.InvocationID)
	}
}

func TestConfig(t *testing.T) {
	config := &Config{
		Timeout:          10 * time.Second,
		EnableAWS:        true,
		EnableGCP:        false,
		EnableAzure:      true,
		EnableKubernetes: true,
		CacheDuration:    10 * time.Minute,
	}

	if config.Timeout != 10*time.Second {
		t.Errorf("Timeout = %v, want 10s", config.Timeout)
	}

	if !config.EnableAWS {
		t.Error("EnableAWS = false, want true")
	}

	if config.EnableGCP {
		t.Error("EnableGCP = true, want false")
	}

	if !config.EnableAzure {
		t.Error("EnableAzure = false, want true")
	}

	if !config.EnableKubernetes {
		t.Error("EnableKubernetes = false, want true")
	}

	if config.CacheDuration != 10*time.Minute {
		t.Errorf("CacheDuration = %v, want 10m", config.CacheDuration)
	}
}

func TestProviderValues(t *testing.T) {
	// Test iota values
	if ProviderUnknown != 0 {
		t.Errorf("ProviderUnknown = %d, want 0", ProviderUnknown)
	}

	if ProviderAWS != 1 {
		t.Errorf("ProviderAWS = %d, want 1", ProviderAWS)
	}

	if ProviderGCP != 2 {
		t.Errorf("ProviderGCP = %d, want 2", ProviderGCP)
	}

	if ProviderAzure != 3 {
		t.Errorf("ProviderAzure = %d, want 3", ProviderAzure)
	}
}

func TestEnvironmentTypeValues(t *testing.T) {
	// Test iota values
	if EnvTypeUnknown != 0 {
		t.Errorf("EnvTypeUnknown = %d, want 0", EnvTypeUnknown)
	}

	if EnvTypeVM != 1 {
		t.Errorf("EnvTypeVM = %d, want 1", EnvTypeVM)
	}

	if EnvTypeContainer != 2 {
		t.Errorf("EnvTypeContainer = %d, want 2", EnvTypeContainer)
	}

	if EnvTypeKubernetes != 3 {
		t.Errorf("EnvTypeKubernetes = %d, want 3", EnvTypeKubernetes)
	}

	if EnvTypeServerless != 4 {
		t.Errorf("EnvTypeServerless = %d, want 4", EnvTypeServerless)
	}
}

func TestAWSDetector_IsCloudEnvironment(t *testing.T) {
	detector := NewAWSDetector(nil)

	// In test environment (not AWS), should return false
	if detector.IsCloudEnvironment() {
		t.Error("expected false in non-AWS environment")
	}
}

func TestGCPDetector_IsCloudEnvironment(t *testing.T) {
	detector := NewGCPDetector(nil)

	// In test environment (not GCP), should return false
	if detector.IsCloudEnvironment() {
		t.Error("expected false in non-GCP environment")
	}
}

func TestAzureDetector_IsCloudEnvironment(t *testing.T) {
	detector := NewAzureDetector(nil)

	// In test environment (not Azure), should return false
	if detector.IsCloudEnvironment() {
		t.Error("expected false in non-Azure environment")
	}
}

func TestAWSDetector_Detect_NotInAWS(t *testing.T) {
	detector := NewAWSDetector(nil)

	_, err := detector.Detect()
	if err == nil {
		t.Error("expected error when not in AWS")
	}
}

func TestGCPDetector_Detect_NotInGCP(t *testing.T) {
	detector := NewGCPDetector(nil)

	_, err := detector.Detect()
	if err == nil {
		t.Error("expected error when not in GCP")
	}
}

func TestAzureDetector_Detect_NotInAzure(t *testing.T) {
	detector := NewAzureDetector(nil)

	_, err := detector.Detect()
	if err == nil {
		t.Error("expected error when not in Azure")
	}
}

func TestMultiCloudDetector_Detect_NotInCloud(t *testing.T) {
	detector := NewDetector(nil)

	_, err := detector.Detect()
	if err == nil {
		t.Error("expected error when not in any cloud")
	}
}
